// Package api is the local HTTP service. It binds to the loopback address,
// takes JSON, and returns JSON. Five verbs do the work an agent needs:
// snapshot, list, diff, restore and prune.
package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ThyFriendlyFox/snapshot-contain-protect/internal/container"
	"github.com/ThyFriendlyFox/snapshot-contain-protect/internal/engine"
	"github.com/ThyFriendlyFox/snapshot-contain-protect/internal/store"
	"github.com/ThyFriendlyFox/snapshot-contain-protect/internal/ulid"
)

// Config holds every option the daemon reads. docs/CONFIGURATION.md lists the
// same fields with their defaults.
type Config struct {
	Addr          string        // loopback address to listen on
	DataDir       string        // holds snapshots/ and snapshot.db
	Backend       string        // auto | btrfs | copy | apfs | vss
	AutoKeep      int           // auto snapshots kept per workset
	PruneInterval time.Duration // 0 turns the pruner off
}

// DefaultConfig returns the configuration the daemon uses with no flags.
func DefaultConfig() Config {
	return Config{
		Addr:          "127.0.0.1:7099",
		DataDir:       defaultDataDir(),
		Backend:       "auto",
		AutoKeep:      50,
		PruneInterval: 10 * time.Minute,
	}
}

func defaultDataDir() string {
	if dir := os.Getenv("SNAPSHOT_DATA_DIR"); dir != "" {
		return dir
	}
	if dir, err := os.UserHomeDir(); err == nil {
		return filepath.Join(dir, ".local", "share", "snapshot")
	}
	return filepath.Join(os.TempDir(), "snapshot")
}

// SnapshotRoot is where backends keep snapshot handles.
func (c Config) SnapshotRoot() string { return filepath.Join(c.DataDir, "snapshots") }

// DBPath is the metadata database, next to the snapshot root.
func (c Config) DBPath() string { return filepath.Join(c.DataDir, "snapshot.db") }

// Service carries the state behind the handlers.
type Service struct {
	cfg       Config
	store     *store.Store
	engine    engine.Engine
	container container.Runtime
	now       func() time.Time
	log       *slog.Logger

	mu    sync.Mutex
	locks map[string]*sync.Mutex // one lock per workset
}

// NewService opens the store, selects a backend, and returns the service.
func NewService(ctx context.Context, cfg Config) (*Service, error) {
	if err := os.MkdirAll(cfg.SnapshotRoot(), 0o755); err != nil {
		return nil, err
	}
	eng, err := engine.Select(cfg.Backend, cfg.SnapshotRoot())
	if err != nil {
		return nil, err
	}
	st, err := store.Open(ctx, cfg.DBPath())
	if err != nil {
		return nil, err
	}
	return &Service{
		cfg:       cfg,
		store:     st,
		engine:    eng,
		container: container.Unavailable{},
		now:       time.Now,
		log:       slog.Default(),
		locks:     map[string]*sync.Mutex{},
	}, nil
}

// SetLogger points the service at the daemon's logger.
func (s *Service) SetLogger(l *slog.Logger) { s.log = l }

// Close releases the store.
func (s *Service) Close() error { return s.store.Close() }

// Backend reports the selected backend, which the daemon logs at start.
func (s *Service) Backend() string { return s.engine.Name() }

// lock serialises work on one workset. A restore and a snapshot on the same
// paths at the same time would tangle the graph.
func (s *Service) lock(worksetID string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.locks[worksetID]
	if !ok {
		m = &sync.Mutex{}
		s.locks[worksetID] = m
	}
	return m
}

// CreateWorkset declares a set of paths that snapshot together.
func (s *Service) CreateWorkset(ctx context.Context, name string, paths []string, useContainer bool) (store.Workset, error) {
	if name == "" {
		return store.Workset{}, badRequest("workset needs a name")
	}
	if len(paths) == 0 {
		return store.Workset{}, badRequest("workset needs at least one path")
	}
	clean := make([]string, 0, len(paths))
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			return store.Workset{}, badRequest(fmt.Sprintf("path %q is not absolute", p))
		}
		// Store the path the filesystem will actually snapshot. A tree walk
		// does not descend through a symlinked root, so a workset that names
		// one would snapshot nothing.
		resolved, err := filepath.EvalSymlinks(filepath.Clean(p))
		if err != nil {
			return store.Workset{}, badRequest(fmt.Sprintf("path %q: %v", p, err))
		}
		clean = append(clean, resolved)
	}

	existing, err := s.store.WorksetByName(ctx, name)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return store.Workset{}, err
	}
	w := store.Workset{ID: ulid.New(), Name: name, Paths: clean, Container: useContainer}
	if err == nil {
		w.ID = existing.ID // Redeclaring a workset keeps its identity and its graph.
	}
	if err := s.store.PutWorkset(ctx, w); err != nil {
		return store.Workset{}, err
	}
	return w, nil
}

// ListWorksets returns every declared workset.
func (s *Service) ListWorksets(ctx context.Context) ([]store.Workset, error) {
	return s.store.ListWorksets(ctx)
}

// CreateSnapshot checkpoints a workset. The parent is the newest node of that
// workset, so the graph grows in the order the work happened.
func (s *Service) CreateSnapshot(ctx context.Context, worksetName, label string, auto bool) (store.Snapshot, time.Duration, error) {
	w, err := s.store.WorksetByName(ctx, worksetName)
	if errors.Is(err, store.ErrNotFound) {
		return store.Snapshot{}, 0, notFound(fmt.Sprintf("unknown workset %q", worksetName))
	}
	if err != nil {
		return store.Snapshot{}, 0, err
	}
	if w.Container {
		return store.Snapshot{}, 0, unavailable(container.ErrNotImplemented.Error())
	}

	m := s.lock(w.ID)
	m.Lock()
	defer m.Unlock()

	start := s.now()
	sn, err := s.snapshotLocked(ctx, w, label, auto, nil)
	if err != nil {
		return store.Snapshot{}, 0, asAPIError(err)
	}
	took := s.now().Sub(start)

	// Retention runs after the snapshot the caller asked for, never before it.
	// A failed prune is maintenance debt, not a failed checkpoint; the timer
	// tries again.
	if err := s.pruneAutoLocked(ctx, w.ID); err != nil {
		s.log.Warn("retention after snapshot failed", "workset", w.Name, "error", err)
	}
	return sn, took, nil
}

// snapshotLocked writes one node. The caller holds the workset lock. parent
// overrides the default parent, which a restore needs.
func (s *Service) snapshotLocked(ctx context.Context, w store.Workset, label string, auto bool, parent *string) (store.Snapshot, error) {
	if parent == nil {
		if latest, err := s.store.LatestSnapshot(ctx, w.ID); err == nil {
			parent = &latest.ID
		} else if !errors.Is(err, store.ErrNotFound) {
			return store.Snapshot{}, err
		}
	}

	id := ulid.New()
	handle, err := s.engine.Create(ctx, id, w.Paths)
	if err != nil {
		return store.Snapshot{}, err
	}
	sn := store.Snapshot{
		ID:        id,
		ParentID:  parent,
		Label:     label,
		CreatedAt: s.now().Unix(),
		WorksetID: w.ID,
		FSHandle:  handle,
		Backend:   s.engine.Name(),
		Auto:      auto,
	}
	if err := s.store.PutSnapshot(ctx, sn); err != nil {
		_ = s.engine.Delete(ctx, handle)
		return store.Snapshot{}, err
	}
	return sn, nil
}

// ListSnapshots returns the graph of one workset as a flat list.
func (s *Service) ListSnapshots(ctx context.Context, worksetName string) ([]store.Snapshot, error) {
	w, err := s.store.WorksetByName(ctx, worksetName)
	if errors.Is(err, store.ErrNotFound) {
		return nil, notFound(fmt.Sprintf("unknown workset %q", worksetName))
	}
	if err != nil {
		return nil, err
	}
	return s.store.ListSnapshots(ctx, w.ID)
}

// Diff summarises what changed between two snapshots.
func (s *Service) Diff(ctx context.Context, fromID, toID string) (engine.Change, error) {
	from, err := s.snapshotOr404(ctx, fromID)
	if err != nil {
		return engine.Change{}, err
	}
	to, err := s.snapshotOr404(ctx, toID)
	if err != nil {
		return engine.Change{}, err
	}
	if from.Backend != to.Backend {
		return engine.Change{}, badRequest("the two snapshots use different backends")
	}
	return s.engine.Diff(ctx, from.FSHandle, to.FSHandle)
}

// Restore returns a workset to a snapshot. It takes a safety snapshot first,
// then writes a new node whose parent is the restored one. Nothing is
// deleted: history is append-only.
func (s *Service) Restore(ctx context.Context, id string) (store.Snapshot, error) {
	target, err := s.snapshotOr404(ctx, id)
	if err != nil {
		return store.Snapshot{}, err
	}
	w, err := s.store.WorksetByID(ctx, target.WorksetID)
	if err != nil {
		return store.Snapshot{}, err
	}

	m := s.lock(w.ID)
	m.Lock()
	defer m.Unlock()

	// The safety snapshot is the undo for the undo. An agent that rolls back
	// to the wrong node can still reach the state it left.
	//
	// It must not block the restore. The state that most needs a rollback is
	// the state that cannot be snapshotted: an agent that deleted a declared
	// path leaves nothing to capture.
	if _, err := s.snapshotLocked(ctx, w, "before restore of "+target.ID, true, nil); err != nil {
		s.log.Warn("safety snapshot failed; restoring anyway",
			"workset", w.Name, "target", target.ID, "error", err)
	}
	if err := s.engine.Restore(ctx, target.FSHandle); err != nil {
		return store.Snapshot{}, asAPIError(err)
	}
	sn, err := s.snapshotLocked(ctx, w, "restore of "+target.ID, true, &target.ID)
	return sn, asAPIError(err)
}

// Prune removes a snapshot and its handle. It refuses a node with children
// unless cascade is set.
func (s *Service) Prune(ctx context.Context, id string, cascade bool) ([]store.Snapshot, error) {
	target, err := s.snapshotOr404(ctx, id)
	if err != nil {
		return nil, err
	}
	m := s.lock(target.WorksetID)
	m.Lock()
	defer m.Unlock()

	kids, err := s.store.Children(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(kids) > 0 && !cascade {
		return nil, conflict("snapshot has children; set cascade=true to remove the subtree")
	}
	order, err := s.store.Subtree(ctx, target)
	if err != nil {
		return nil, err
	}

	// The handle goes first, then the row. A row without a handle breaks
	// every later diff and restore; a handle whose delete failed keeps its
	// row, so the next prune can try again.
	removed := make([]store.Snapshot, 0, len(order))
	for i := len(order) - 1; i >= 0; i-- {
		sn := order[i]
		if err := s.engine.Delete(ctx, sn.FSHandle); err != nil {
			return removed, fmt.Errorf("delete handle %s: %w", sn.FSHandle, err)
		}
		if _, err := s.store.DeleteSnapshot(ctx, sn.ID, false); err != nil {
			return removed, err
		}
		removed = append(removed, sn)
	}
	return removed, nil
}

// asAPIError turns an engine complaint about the working set into a 400. The
// caller declared those paths, so the caller is the one who can fix them.
func asAPIError(err error) error {
	if errors.Is(err, engine.ErrBadSource) {
		return badRequest(err.Error())
	}
	if errors.Is(err, engine.ErrUnavailable) {
		return unavailable(err.Error())
	}
	return err
}

func (s *Service) snapshotOr404(ctx context.Context, id string) (store.Snapshot, error) {
	sn, err := s.store.SnapshotByID(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return store.Snapshot{}, notFound(fmt.Sprintf("unknown snapshot %q", id))
	}
	return sn, err
}
