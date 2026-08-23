package api

import (
	"context"
	"errors"
	"log/slog"
	"time"

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
	excess := len(candidates) - s.cfg.AutoKeep
	if excess <= 0 {
		return nil
	}
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
		if err := s.store.Reparent(ctx, current.ID, current.ParentID); err != nil {
			return err
		}
		// Free the handle first. A row that outlives its handle breaks every
		// later diff and restore of that node; a handle that outlives its row
		// is disk this pass can never find again.
		if err := s.engine.Delete(ctx, current.FSHandle); err != nil {
			return err
		}
		if _, err := s.store.DeleteSnapshot(ctx, current.ID, false); err != nil {
			return err
		}
	}
	return nil
}

// PruneAll runs retention over every workset. The timer calls it, and so does
// a caller that wants it now.
func (s *Service) PruneAll(ctx context.Context) error {
	worksets, err := s.store.ListWorksets(ctx)
	if err != nil {
		return err
	}
	for _, w := range worksets {
		m := s.lock(w.ID)
		m.Lock()
		err := s.pruneAutoLocked(ctx, w.ID)
		m.Unlock()
		if err != nil {
			return err
		}
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
