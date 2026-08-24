package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ThyFriendlyFox/snapshot-contain-protect/internal/store"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestReconcileFindsNothingInAHealthyGraph(t *testing.T) {
	f := newFixture(t)
	f.snapshot("first", false)
	f.snapshot("second", false)

	rep, err := f.svc.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Empty() {
		t.Fatalf("a healthy graph reported %s", rep)
	}
}

func TestReconcileRemovesAHandleNoRowNames(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.snapshot("kept", false)

	// A daemon killed after the engine call and before the store write leaves
	// exactly this: a snapshot on disk that the graph does not know about.
	orphan := filepath.Join(f.svc.cfg.SnapshotRoot(), "01ORPHANHANDLE0000000000AA")
	if err := os.MkdirAll(filepath.Join(orphan, "p0-work"), 0o755); err != nil {
		t.Fatal(err)
	}

	rep, err := f.svc.Reconcile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OrphanHandles != 1 || rep.OrphanRows != 0 {
		t.Fatalf("repair = %s, want 1 orphan handle", rep)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("the orphan handle survived")
	}

	var list []store.Snapshot
	f.get("/snapshots?workset=proj-a", http.StatusOK, &list)
	if len(list) != 1 {
		t.Fatalf("graph size = %d, want the 1 healthy node untouched", len(list))
	}
}

func TestReconcileRemovesARowWhoseSnapshotIsGone(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	first := f.snapshot("first", false)
	second := f.snapshot("second", false)

	sn, err := f.svc.store.SnapshotByID(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(sn.FSHandle); err != nil {
		t.Fatal(err)
	}

	rep, err := f.svc.Reconcile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OrphanRows != 1 {
		t.Fatalf("repair = %s, want 1 orphan row", rep)
	}

	// The child survives and joins its grandparent, so the chain stays whole.
	var list []store.Snapshot
	f.get("/snapshots?workset=proj-a", http.StatusOK, &list)
	if len(list) != 1 || list[0].ID != second.ID {
		t.Fatalf("graph = %v, want only %s", list, second.ID)
	}
	if list[0].ParentID != nil {
		t.Fatalf("the survivor points at a deleted parent: %v", list[0].ParentID)
	}
}

func TestReconcileRefusesToEmptyTheGraphOnAWrongDataDirectory(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.snapshot("first", false)
	f.snapshot("second", false)

	// A typo in -data-dir looks exactly like "every snapshot is gone". The
	// worst answer to a typo is deleting the whole graph.
	if err := os.RemoveAll(f.svc.cfg.SnapshotRoot()); err != nil {
		t.Fatal(err)
	}

	_, err := f.svc.Reconcile(ctx)
	if err == nil {
		t.Fatal("reconciliation emptied the graph instead of refusing")
	}
	if !strings.Contains(err.Error(), "data-dir") {
		t.Fatalf("the error does not name the likely cause: %v", err)
	}

	var list []store.Snapshot
	f.get("/snapshots?workset=proj-a", http.StatusOK, &list)
	if len(list) != 2 {
		t.Fatalf("graph size = %d, want both rows kept", len(list))
	}
}

func TestReconcileLeavesForeignDirectoriesAlone(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.snapshot("first", false)

	foreign := filepath.Join(f.svc.cfg.SnapshotRoot(), "somebody-elses-data")
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}

	rep, err := f.svc.Reconcile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OrphanHandles != 0 {
		t.Fatalf("repair = %s, want nothing touched", rep)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal("a directory that is not ours was deleted")
	}
}

func TestReconcileAtStartReportsAndSucceeds(t *testing.T) {
	f := newFixture(t)
	f.snapshot("first", false)
	if err := f.svc.ReconcileAtStart(context.Background(), quietLogger()); err != nil {
		t.Fatal(err)
	}
}
