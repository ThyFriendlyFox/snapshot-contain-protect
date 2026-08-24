package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ThyFriendlyFox/snapshot-contain-protect/internal/engine"
	"github.com/ThyFriendlyFox/snapshot-contain-protect/internal/store"
)

// Retention keeps the last AutoKeep auto snapshots per workset and keeps every
// manual one. START.md section 6 states the rules:
//
//   - Prune auto snapshots oldest first.
//   - Never prune a manual snapshot.
//   - Never prune a snapshot that is an ancestor of a manual snapshot.
//
// A pruned node in the middle of the graph hands its children to its parent,
// so the chain an agent reads stays connected.
func (s *Service) pruneAutoLocked(ctx context.Context, worksetID string) error {
	if s.cfg.AutoKeep <= 0 {
		return nil
	}
	list, err := s.store.ListSnapshots(ctx, worksetID)
	if err != nil {
		return err
	}
	protected, err := s.store.ManualAncestors(ctx, worksetID)
	if err != nil {
		return err
	}

	candidates := make([]store.Snapshot, 0, len(list))
	for _, sn := range list {
		if sn.Auto && !protected[sn.ID] {
			candidates = append(candidates, sn)
		}
	}
	// ListSnapshots orders oldest first, so the excess is at the front.
	keep := s.budgetFor(ctx, worksetID)
	excess := len(candidates) - keep
	if excess <= 0 {
		return nil
	}
	var stuck int
	for _, sn := range candidates[:excess] {
		// Re-read the node. An earlier pass of this loop may have moved its
		// parent, and reparenting onto a deleted row breaks the foreign key.
		current, err := s.store.SnapshotByID(ctx, sn.ID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		// Free the handle first. A row that outlives its handle breaks every
		// later diff and restore of that node; a handle that outlives its row
		// is disk this pass can never find again.
		//
		// A handle that will not go keeps its row and does not stop the pass.
		// One stuck snapshot must not freeze retention for the whole workset.
		if err := s.engine.Delete(ctx, current.FSHandle); err != nil {
			s.log.Warn("retention could not free a snapshot; keeping its row",
				"snapshot", current.ID, "handle", current.FSHandle, "error", err)
			stuck++
			continue
		}
		if err := s.store.Reparent(ctx, current.ID, current.ParentID); err != nil {
			return err
		}
		if _, err := s.store.DeleteSnapshot(ctx, current.ID, false); err != nil {
			return err
		}
	}
	if stuck > 0 {
		return fmt.Errorf("retention kept %d snapshot(s) whose handle could not be freed", stuck)
	}
	return nil
}

// budgetFor returns the number of auto snapshots to keep: the configured
// budget, or the provider's, whichever is smaller.
//
// A provider that reaches its cap does not refuse the next snapshot. It
// deletes the oldest one, and the graph would go on naming it. Pruning first
// keeps the graph true.
func (s *Service) budgetFor(ctx context.Context, worksetID string) int {
	keep := s.cfg.AutoKeep
	budgeter, ok := s.engine.(engine.Budgeter)
	if !ok {
		return keep
	}
	w, err := s.store.WorksetByID(ctx, worksetID)
	if err != nil {
		return keep
	}
	usable, _ := s.usablePaths(w)
	if len(usable) == 0 {
		return keep
	}
	max, ok, err := budgeter.Budget(ctx, usable)
	if err != nil {
		s.log.Warn("cannot read the provider budget; keeping the configured one",
			"workset", w.Name, "auto_keep", keep, "error", err)
		return keep
	}
	if !ok || max >= keep {
		return keep
	}
	// Leave 1 slot free, so the next snapshot does not force an eviction
	// between this prune and that write.
	if max > 0 {
		max--
	}
	s.log.Info("the provider is the binding limit, not auto-keep",
		"workset", w.Name, "auto_keep", keep, "provider_allows", max)
	return max
}

// PruneAll runs retention over every workset. The timer calls it, and so does
// a caller that wants it now.
func (s *Service) PruneAll(ctx context.Context) error {
	worksets, err := s.store.ListWorksets(ctx)
	if err != nil {
		return err
	}
	// One workset that cannot be pruned must not stop the others.
	var failed []string
	for _, w := range worksets {
		m := s.lock(w.ID)
		m.Lock()
		err := s.pruneAutoLocked(ctx, w.ID)
		m.Unlock()
		if err != nil {
			s.log.Warn("retention pass failed for a workset", "workset", w.Name, "error", err)
			failed = append(failed, w.Name)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("retention failed for %s", strings.Join(failed, ", "))
	}
	return nil
}

// RunRetention prunes on a timer until ctx is cancelled. A failed pass logs
// and waits for the next tick: retention is maintenance, not a request.
func (s *Service) RunRetention(ctx context.Context, log *slog.Logger) {
	if s.cfg.PruneInterval <= 0 || s.cfg.AutoKeep <= 0 {
		log.Info("retention off", "prune_interval", s.cfg.PruneInterval, "auto_keep", s.cfg.AutoKeep)
		return
	}
	ticker := time.NewTicker(s.cfg.PruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.PruneAll(ctx); err != nil && ctx.Err() == nil {
				log.Error("retention pass failed", "error", err)
			}
		}
	}
}
