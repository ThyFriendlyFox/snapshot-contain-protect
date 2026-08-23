// Package store keeps the snapshot graph in SQLite. One file, next to the
// snapshot root. The graph lives entirely in parent_id, and history is
// append-only: a restore adds a node, it never removes one.
package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// ErrNotFound means the row does not exist.
var ErrNotFound = errors.New("not found")

// ErrHasChildren means a prune would orphan part of the graph.
var ErrHasChildren = errors.New("snapshot has children")

// Workset is a declared set of paths that snapshots cover together.
type Workset struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Paths     []string `json:"paths"`
	Container bool     `json:"container"`
}

// Snapshot is one node of the graph.
type Snapshot struct {
	ID        string  `json:"id"`
	ParentID  *string `json:"parent_id"`
	Label     string  `json:"label"`
	CreatedAt int64   `json:"created_at"`
	WorksetID string  `json:"workset_id"`
	FSHandle  string  `json:"fs_handle"`
	Backend   string  `json:"backend"`
	Container *string `json:"container"`
	Auto      bool    `json:"auto"`
}

// Store is the metadata database.
type Store struct {
	db *sql.DB
}

// Open opens the database at path and applies every migration.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	// One writer keeps the graph consistent without a transaction around every
	// read. Snapshot traffic is low; contention is not the problem here.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		var seen string
		err := s.db.QueryRowContext(ctx, `SELECT name FROM schema_migrations WHERE name = ?`, name).Scan(&seen)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		body, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (name) VALUES (?)`, name); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// PutWorkset inserts or replaces a workset.
func (s *Store) PutWorkset(ctx context.Context, w Workset) error {
	if len(w.Paths) == 0 {
		return errors.New("workset needs at least one path")
	}
	paths, err := json.Marshal(w.Paths)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO worksets (id, name, paths, container) VALUES (?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET name = excluded.name, paths = excluded.paths, container = excluded.container`,
		w.ID, w.Name, string(paths), boolToInt(w.Container))
	return err
}

// WorksetByName finds a workset by its human name, which is what the API takes.
func (s *Store) WorksetByName(ctx context.Context, name string) (Workset, error) {
	return s.workset(ctx, `SELECT id, name, paths, container FROM worksets WHERE name = ?`, name)
}

// WorksetByID finds a workset by identifier.
func (s *Store) WorksetByID(ctx context.Context, id string) (Workset, error) {
	return s.workset(ctx, `SELECT id, name, paths, container FROM worksets WHERE id = ?`, id)
}

func (s *Store) workset(ctx context.Context, query string, arg any) (Workset, error) {
	var w Workset
	var paths string
	var container int
	err := s.db.QueryRowContext(ctx, query, arg).Scan(&w.ID, &w.Name, &paths, &container)
	if errors.Is(err, sql.ErrNoRows) {
		return w, ErrNotFound
	}
	if err != nil {
		return w, err
	}
	w.Container = container == 1
	if err := json.Unmarshal([]byte(paths), &w.Paths); err != nil {
		return w, fmt.Errorf("workset %s has damaged paths: %w", w.ID, err)
	}
	return w, nil
}

// ListWorksets returns every workset, ordered by name.
func (s *Store) ListWorksets(ctx context.Context) ([]Workset, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, paths, container FROM worksets ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Workset{}
	for rows.Next() {
		var w Workset
		var paths string
		var container int
		if err := rows.Scan(&w.ID, &w.Name, &paths, &container); err != nil {
			return nil, err
		}
		w.Container = container == 1
		if err := json.Unmarshal([]byte(paths), &w.Paths); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// PutSnapshot inserts one node.
func (s *Store) PutSnapshot(ctx context.Context, sn Snapshot) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO snapshots (id, parent_id, label, created_at, workset_id, fs_handle, backend, container, auto)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sn.ID, sn.ParentID, sn.Label, sn.CreatedAt, sn.WorksetID, sn.FSHandle, sn.Backend, sn.Container, boolToInt(sn.Auto))
	return err
}

// SnapshotByID returns one node.
func (s *Store) SnapshotByID(ctx context.Context, id string) (Snapshot, error) {
	rows, err := s.db.QueryContext(ctx, snapshotSelect+` WHERE id = ?`, id)
	if err != nil {
		return Snapshot{}, err
	}
	defer rows.Close()
	list, err := scanSnapshots(rows)
	if err != nil {
		return Snapshot{}, err
	}
	if len(list) == 0 {
		return Snapshot{}, ErrNotFound
	}
	return list[0], nil
}

const snapshotSelect = `SELECT id, parent_id, label, created_at, workset_id, fs_handle, backend, container, auto FROM snapshots`

// ListSnapshots returns the graph for one workset as a flat list, oldest
// first. The client builds the tree from parent_id.
func (s *Store) ListSnapshots(ctx context.Context, worksetID string) ([]Snapshot, error) {
	rows, err := s.db.QueryContext(ctx, snapshotSelect+` WHERE workset_id = ? ORDER BY created_at, id`, worksetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSnapshots(rows)
}

// LatestSnapshot returns the newest node of a workset, which becomes the
// parent of the next one.
func (s *Store) LatestSnapshot(ctx context.Context, worksetID string) (Snapshot, error) {
	rows, err := s.db.QueryContext(ctx, snapshotSelect+` WHERE workset_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`, worksetID)
	if err != nil {
		return Snapshot{}, err
	}
	defer rows.Close()
	list, err := scanSnapshots(rows)
	if err != nil {
		return Snapshot{}, err
	}
	if len(list) == 0 {
		return Snapshot{}, ErrNotFound
	}
	return list[0], nil
}

// Children returns the direct children of a node.
func (s *Store) Children(ctx context.Context, id string) ([]Snapshot, error) {
	rows, err := s.db.QueryContext(ctx, snapshotSelect+` WHERE parent_id = ? ORDER BY created_at, id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSnapshots(rows)
}

// DeleteSnapshot removes one node. It refuses a node with children unless
// cascade is set, in which case the whole subtree goes, deepest first. The
// returned list is every node removed, so the caller can free the handles.
func (s *Store) DeleteSnapshot(ctx context.Context, id string, cascade bool) ([]Snapshot, error) {
	target, err := s.SnapshotByID(ctx, id)
	if err != nil {
		return nil, err
	}
	kids, err := s.Children(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(kids) > 0 && !cascade {
		return nil, ErrHasChildren
	}

	order, err := s.subtree(ctx, target)
	if err != nil {
		return nil, err
	}
	// Deepest first, so no row ever loses its parent before it is gone.
	for i := len(order) - 1; i >= 0; i-- {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM snapshots WHERE id = ?`, order[i].ID); err != nil {
			return nil, err
		}
	}
	return order, nil
}

// subtree returns the node and every descendant, parents before children.
func (s *Store) subtree(ctx context.Context, root Snapshot) ([]Snapshot, error) {
	out := []Snapshot{root}
	for i := 0; i < len(out); i++ {
		kids, err := s.Children(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out = append(out, kids...)
	}
	return out, nil
}

// Reparent moves every child of oldParent to newParent. Retention uses it: a
// pruned node in the middle of the graph must not orphan the work that came
// after it.
func (s *Store) Reparent(ctx context.Context, oldParent string, newParent *string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE snapshots SET parent_id = ? WHERE parent_id = ?`, newParent, oldParent)
	return err
}

// ManualAncestors returns the identifier of every snapshot that a manual
// snapshot descends from, including the manual snapshots themselves.
// Retention never removes one of these.
func (s *Store) ManualAncestors(ctx context.Context, worksetID string) (map[string]bool, error) {
	list, err := s.ListSnapshots(ctx, worksetID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]Snapshot, len(list))
	for _, sn := range list {
		byID[sn.ID] = sn
	}

	protected := map[string]bool{}
	for _, sn := range list {
		if sn.Auto {
			continue
		}
		for cur := sn; ; {
			if protected[cur.ID] {
				break // Already walked, or the graph has a cycle.
			}
			protected[cur.ID] = true
			if cur.ParentID == nil {
				break
			}
			parent, ok := byID[*cur.ParentID]
			if !ok {
				break
			}
			cur = parent
		}
	}
	return protected, nil
}

// Ancestors returns the parent chain of a node, nearest first.
func (s *Store) Ancestors(ctx context.Context, id string) ([]Snapshot, error) {
	var out []Snapshot
	seen := map[string]bool{id: true}
	current, err := s.SnapshotByID(ctx, id)
	if err != nil {
		return nil, err
	}
	for current.ParentID != nil {
		if seen[*current.ParentID] {
			return nil, fmt.Errorf("snapshot graph has a cycle at %s", *current.ParentID)
		}
		seen[*current.ParentID] = true
		parent, err := s.SnapshotByID(ctx, *current.ParentID)
		if errors.Is(err, ErrNotFound) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, parent)
		current = parent
	}
	return out, nil
}

func scanSnapshots(rows *sql.Rows) ([]Snapshot, error) {
	out := []Snapshot{}
	for rows.Next() {
		var sn Snapshot
		var label sql.NullString
		var auto int
		if err := rows.Scan(&sn.ID, &sn.ParentID, &label, &sn.CreatedAt, &sn.WorksetID, &sn.FSHandle, &sn.Backend, &sn.Container, &auto); err != nil {
			return nil, err
		}
		sn.Label = label.String
		sn.Auto = auto == 1
		out = append(out, sn)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
