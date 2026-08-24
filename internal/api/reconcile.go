package api

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/ThyFriendlyFox/snapshot-contain-protect/internal/engine"
	"github.com/ThyFriendlyFox/snapshot-contain-protect/internal/store"
)

// Repair counts what reconciliation found and fixed.
type Repair struct {
	OrphanHandles int // snapshots on disk that no row names
	OrphanRows    int // rows whose snapshot is gone from disk
	Evicted       int // rows whose provider deleted the snapshot underneath
}

// Empty reports whether there was nothing to repair.
func (r Repair) Empty() bool {
	return r.OrphanHandles == 0 && r.OrphanRows == 0 && r.Evicted == 0
}

func (r Repair) String() string {
	return fmt.Sprintf("%d orphan handle(s), %d orphan row(s), %d evicted by the provider",
		r.OrphanHandles, r.OrphanRows, r.Evicted)
}

// Reconcile makes the graph and the snapshot root agree. A daemon killed
// between the engine call and the store write leaves one without the other:
//
//   - A handle no row names is unreferenced disk. It is removed.
//   - A row whose handle is gone breaks every later diff and restore of that
//     node with a confusing error. The row is removed.
//
// It refuses one case rather than repairing it. If every row is orphaned and
// there is more than 1 row, the likely cause is a data directory pointed at
// the wrong place, and deleting the whole graph would be the worst possible
// answer to a typo.
func (s *Service) Reconcile(ctx context.Context) (Repair, error) {
	var rep Repair

	worksets, err := s.store.ListWorksets(ctx)
	if err != nil {
		return rep, err
	}

	known := map[string]bool{}
	var rows []store.Snapshot
	for _, w := range worksets {
		list, err := s.store.ListSnapshots(ctx, w.ID)
		if err != nil {
			return rep, err
		}
		for _, sn := range list {
			known[filepath.Clean(sn.FSHandle)] = true
			rows = append(rows, sn)
		}
	}

	verifier, canVerify := s.engine.(engine.Verifier)

	missing := make([]store.Snapshot, 0)
	absent := 0
	for _, sn := range rows {
		if _, err := os.Stat(sn.FSHandle); err != nil {
			if !os.IsNotExist(err) {
				return rep, err
			}
			absent++
			missing = append(missing, sn)
			continue
		}
		if !canVerify {
			continue
		}
		// The handle is there, which proves nothing on a backend whose
		// storage belongs to somebody else. A VSS mount stays a directory
		// after the provider evicts the shadow copy under it, and the graph
		// would go on offering a snapshot that cannot be read.
		alive, err := verifier.Exists(ctx, sn.FSHandle)
		if err != nil {
			s.log.Warn("cannot verify a snapshot; keeping its row",
				"snapshot", sn.ID, "error", err)
			continue
		}
		if !alive {
			rep.Evicted++
			missing = append(missing, sn)
		}
	}
	// The refusal counts only the handles that are not on disk. A wrong
	// -data-dir looks exactly like that, and emptying the graph would be the
	// worst answer to a typo.
	//
	// Eviction is not the same condition and is not ambiguous: the handle is
	// there and the provider says the snapshot behind it is gone. Removing
	// those rows is the only honest answer, however many there are.
	if len(rows) > 1 && absent == len(rows) {
		return rep, fmt.Errorf(
			"every snapshot in the graph is missing from %s: refusing to delete %d rows. "+
				"check that -data-dir points at the right directory",
			s.cfg.SnapshotRoot(), len(rows))
	}

	// A row without its snapshot. Remove it, deepest first, so no child
	// outlives its parent's row.
	for i := len(missing) - 1; i >= 0; i-- {
		sn := missing[i]
		if err := s.store.Reparent(ctx, sn.ID, sn.ParentID); err != nil {
			return rep, err
		}
		if _, err := s.store.DeleteSnapshot(ctx, sn.ID, false); err != nil {
			return rep, err
		}
	}
	rep.OrphanRows = len(missing) - rep.Evicted

	// A snapshot on disk that no row names.
	entries, err := os.ReadDir(s.cfg.SnapshotRoot())
	if err != nil {
		if os.IsNotExist(err) {
			return rep, nil
		}
		return rep, err
	}
	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) != ulidLength {
			// Only ULID-shaped directories are ours. Anything else belongs to
			// somebody, and this is not the code to decide whose.
			continue
		}
		handle := filepath.Join(s.cfg.SnapshotRoot(), e.Name())
		if known[filepath.Clean(handle)] {
			continue
		}
		if err := s.engine.Delete(ctx, handle); err != nil {
			return rep, fmt.Errorf("remove orphan handle %s: %w", handle, err)
		}
		rep.OrphanHandles++
	}
	return rep, nil
}

// ulidLength is the length of the identifiers the engine names handles with.
const ulidLength = 26

// ReconcileAtStart runs reconciliation and says what it found. A failure here
// stops the daemon: starting with a graph that disagrees with the disk means
// every later answer is suspect.
func (s *Service) ReconcileAtStart(ctx context.Context, log *slog.Logger) error {
	rep, err := s.Reconcile(ctx)
	if err != nil {
		return err
	}
	if rep.Empty() {
		log.Info("graph and snapshot root agree")
		return nil
	}
	log.Warn("repaired the graph at start",
		"orphan_handles", rep.OrphanHandles, "orphan_rows", rep.OrphanRows,
		"evicted_by_provider", rep.Evicted)
	return nil
}
