package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "snapshot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func seedWorkset(t *testing.T, s *Store) Workset {
	t.Helper()
	w := Workset{ID: "01WS", Name: "proj-a", Paths: []string{"/home/u/proj"}}
	if err := s.PutWorkset(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	return w
}

func node(id string, parent *string, auto bool) Snapshot {
	return Snapshot{
		ID: id, ParentID: parent, Label: "n-" + id, CreatedAt: 1755900000,
		WorksetID: "01WS", FSHandle: "/snap/" + id, Backend: "copy", Auto: auto,
	}
}

func TestWorksetRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	w := Workset{ID: "01WS", Name: "proj-a", Paths: []string{"/a", "/b"}, Container: true}
	if err := s.PutWorkset(ctx, w); err != nil {
		t.Fatal(err)
	}
	got, err := s.WorksetByName(ctx, "proj-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != w.ID || len(got.Paths) != 2 || got.Paths[1] != "/b" || !got.Container {
		t.Fatalf("workset = %+v, want %+v", got, w)
	}
	if _, err := s.WorksetByName(ctx, "absent"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing workset error = %v, want ErrNotFound", err)
	}
}

func TestWorksetNeedsAPath(t *testing.T) {
	if err := open(t).PutWorkset(context.Background(), Workset{ID: "01WS", Name: "empty"}); err == nil {
		t.Fatal("a workset with no path was accepted")
	}
}

func TestParentGraphSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "snapshot.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	seedWorkset(t, s)
	root := "01A"
	if err := s.PutSnapshot(ctx, node(root, nil, false)); err != nil {
		t.Fatal(err)
	}
	if err := s.PutSnapshot(ctx, node("01B", &root, true)); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// A daemon restart must not lose the graph.
	again, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()

	list, err := again.ListSnapshots(ctx, "01WS")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("snapshot count = %d, want 2", len(list))
	}
	if list[1].ParentID == nil || *list[1].ParentID != root {
		t.Fatalf("child parent = %v, want %s", list[1].ParentID, root)
	}
	if !list[1].Auto {
		t.Error("the auto flag did not survive the restart")
	}
}

func TestRestoreAppendsInsteadOfRewriting(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	seedWorkset(t, s)
	root := "01A"
	mustPut(t, s, node(root, nil, false))
	mustPut(t, s, node("01B", &root, false))
	// A restore of 01A creates a new node whose parent is 01A. 01B stays.
	mustPut(t, s, node("01C", &root, false))

	kids, err := s.Children(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(kids) != 2 {
		t.Fatalf("children of the restored node = %d, want 2", len(kids))
	}
}

func TestDeleteRefusesANodeWithChildren(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	seedWorkset(t, s)
	root := "01A"
	mustPut(t, s, node(root, nil, false))
	mustPut(t, s, node("01B", &root, false))

	if _, err := s.DeleteSnapshot(ctx, root, false); !errors.Is(err, ErrHasChildren) {
		t.Fatalf("delete error = %v, want ErrHasChildren", err)
	}
	removed, err := s.DeleteSnapshot(ctx, root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Fatalf("cascade removed %d nodes, want 2", len(removed))
	}
	if _, err := s.SnapshotByID(ctx, "01B"); !errors.Is(err, ErrNotFound) {
		t.Fatal("the child survived a cascade delete")
	}
}

func TestAncestorsWalkToTheRoot(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	seedWorkset(t, s)
	a, b := "01A", "01B"
	mustPut(t, s, node(a, nil, false))
	mustPut(t, s, node(b, &a, false))
	mustPut(t, s, node("01C", &b, false))

	chain, err := s.Ancestors(ctx, "01C")
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) != 2 || chain[0].ID != b || chain[1].ID != a {
		t.Fatalf("ancestors = %v, want [01B 01A]", ids(chain))
	}
}

func TestLatestSnapshotIsTheNewest(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	seedWorkset(t, s)
	first := node("01A", nil, false)
	first.CreatedAt = 100
	mustPut(t, s, first)
	second := node("01B", nil, false)
	second.CreatedAt = 200
	mustPut(t, s, second)

	got, err := s.LatestSnapshot(ctx, "01WS")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "01B" {
		t.Fatalf("latest = %s, want 01B", got.ID)
	}
}

func TestMigrationsRunOnce(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "snapshot.db")
	for i := 0; i < 3; i++ {
		s, err := Open(ctx, path)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		s.Close()
	}
}

func mustPut(t *testing.T, s *Store, sn Snapshot) {
	t.Helper()
	if err := s.PutSnapshot(context.Background(), sn); err != nil {
		t.Fatal(err)
	}
}

func ids(list []Snapshot) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = s.ID
	}
	return out
}

func TestReparentMovesChildren(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	seedWorkset(t, s)
	a, b := "01A", "01B"
	mustPut(t, s, node(a, nil, true))
	mustPut(t, s, node(b, &a, true))
	mustPut(t, s, node("01C", &b, true))

	// Prune 01B out of the middle: its child joins its parent.
	if err := s.Reparent(ctx, b, &a); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteSnapshot(ctx, b, false); err != nil {
		t.Fatal(err)
	}
	c, err := s.SnapshotByID(ctx, "01C")
	if err != nil {
		t.Fatal(err)
	}
	if c.ParentID == nil || *c.ParentID != a {
		t.Fatalf("01C parent = %v, want %s", c.ParentID, a)
	}
}

func TestManualAncestorsProtectsTheChain(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	seedWorkset(t, s)
	a, b, c := "01A", "01B", "01C"
	mustPut(t, s, node(a, nil, true)) // auto, ancestor of a manual node
	mustPut(t, s, node(b, &a, false)) // manual
	mustPut(t, s, node(c, &b, true))  // auto leaf
	mustPut(t, s, node("01D", &a, true))

	protected, err := s.ManualAncestors(ctx, "01WS")
	if err != nil {
		t.Fatal(err)
	}
	if !protected[a] || !protected[b] {
		t.Fatalf("protected = %v, want 01A and 01B", protected)
	}
	if protected[c] || protected["01D"] {
		t.Fatalf("protected = %v, want auto leaves unprotected", protected)
	}
}
