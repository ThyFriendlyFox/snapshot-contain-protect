package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/ThyFriendlyFox/snapshot-contain-protect/internal/engine"
	"github.com/ThyFriendlyFox/snapshot-contain-protect/internal/store"
)

// setKeep rebuilds the fixture's service with a different retention budget.
func (f *fixture) setKeep(n int) {
	f.t.Helper()
	f.svc.cfg.AutoKeep = n
}

func TestRetentionKeepsTheLastAutoSnapshots(t *testing.T) {
	f := newFixture(t)
	f.setKeep(3)

	for i := 0; i < 8; i++ {
		f.snapshot("auto", true)
	}

	var list []store.Snapshot
	f.get("/snapshots?workset=proj-a", http.StatusOK, &list)
	if len(list) != 3 {
		t.Fatalf("graph size = %d, want 3", len(list))
	}
	// The survivors are the newest three, and the chain is still connected.
	if list[0].ParentID != nil {
		t.Errorf("oldest survivor has parent %v, want nil", list[0].ParentID)
	}
	for i := 1; i < len(list); i++ {
		if list[i].ParentID == nil || *list[i].ParentID != list[i-1].ID {
			t.Fatalf("node %d parent = %v, want %s", i, list[i].ParentID, list[i-1].ID)
		}
	}
}

func TestRetentionKeepsEveryManualSnapshot(t *testing.T) {
	f := newFixture(t)
	f.setKeep(2)

	for i := 0; i < 4; i++ {
		f.snapshot("manual", false)
	}
	for i := 0; i < 6; i++ {
		f.snapshot("auto", true)
	}

	var list []store.Snapshot
	f.get("/snapshots?workset=proj-a", http.StatusOK, &list)
	manual := 0
	for _, sn := range list {
		if !sn.Auto {
			manual++
		}
	}
	if manual != 4 {
		t.Fatalf("manual snapshots = %d, want 4", manual)
	}
	if len(list) != 6 {
		t.Fatalf("graph size = %d, want 6 (4 manual, 2 auto)", len(list))
	}
}

func TestRetentionNeverPrunesAnAncestorOfAManualSnapshot(t *testing.T) {
	f := newFixture(t)
	f.setKeep(1)

	// An auto snapshot with a manual descendant is load-bearing history.
	ancestor := f.snapshot("auto ancestor", true)
	f.snapshot("manual child", false)
	for i := 0; i < 5; i++ {
		f.snapshot("auto", true)
	}

	var list []store.Snapshot
	f.get("/snapshots?workset=proj-a", http.StatusOK, &list)
	found := false
	for _, sn := range list {
		if sn.ID == ancestor.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("retention pruned an ancestor of a manual snapshot")
	}
}

func TestRetentionRemovesTheHandlesItPrunes(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.setKeep(1)

	first := f.snapshot("auto", true)
	sn, err := f.svc.store.SnapshotByID(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.snapshot("auto", true)
	f.snapshot("auto", true)

	if _, err := os.Stat(sn.FSHandle); !os.IsNotExist(err) {
		t.Fatalf("handle %s survived retention", filepath.Base(sn.FSHandle))
	}
}

func TestRetentionOffKeepsEverything(t *testing.T) {
	f := newFixture(t)
	f.setKeep(0)
	for i := 0; i < 5; i++ {
		f.snapshot("auto", true)
	}
	var list []store.Snapshot
	f.get("/snapshots?workset=proj-a", http.StatusOK, &list)
	if len(list) != 5 {
		t.Fatalf("graph size = %d, want 5", len(list))
	}
}

func TestPruneAllCoversEveryWorkset(t *testing.T) {
	f := newFixture(t)
	f.setKeep(2)
	f.post("/worksets", worksetRequest{Name: "proj-b", Paths: []string{f.work}}, http.StatusCreated, nil)

	f.svc.cfg.AutoKeep = 0 // Fill both graphs with retention off.
	for i := 0; i < 5; i++ {
		f.snapshot("auto", true)
		f.post("/snapshot", snapshotRequest{Workset: "proj-b", Auto: true}, http.StatusCreated, nil)
	}

	f.svc.cfg.AutoKeep = 2
	if err := f.svc.PruneAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"proj-a", "proj-b"} {
		var list []store.Snapshot
		f.get("/snapshots?workset="+name, http.StatusOK, &list)
		if len(list) != 2 {
			t.Fatalf("%s graph size = %d, want 2", name, len(list))
		}
	}
}

// budgetedEngine reports a provider cap smaller than the configured budget.
type budgetedEngine struct {
	engine.Engine
	max int
	ok  bool
}

func (b budgetedEngine) Budget(context.Context, []string) (int, bool, error) {
	return b.max, b.ok, nil
}

func TestProviderBudgetBeatsAutoKeepWhenSmaller(t *testing.T) {
	f := newFixture(t)
	f.setKeep(50)
	// The provider will hold 4. Retention must prune to 3, leaving a slot so
	// the next snapshot does not force the provider to evict.
	f.svc.engine = budgetedEngine{Engine: f.svc.engine, max: 4, ok: true}

	for i := 0; i < 8; i++ {
		f.snapshot("auto", true)
	}
	var list []store.Snapshot
	f.get("/snapshots?workset=proj-a", http.StatusOK, &list)
	if len(list) != 3 {
		t.Fatalf("graph size = %d, want 3: the provider allows 4 and 1 slot stays free", len(list))
	}
}

func TestAutoKeepWinsWhenTheProviderIsRoomier(t *testing.T) {
	f := newFixture(t)
	f.setKeep(2)
	f.svc.engine = budgetedEngine{Engine: f.svc.engine, max: 100, ok: true}

	for i := 0; i < 6; i++ {
		f.snapshot("auto", true)
	}
	var list []store.Snapshot
	f.get("/snapshots?workset=proj-a", http.StatusOK, &list)
	if len(list) != 2 {
		t.Fatalf("graph size = %d, want the configured 2", len(list))
	}
}

func TestABackendWithNoBudgetKeepsTheConfiguredOne(t *testing.T) {
	f := newFixture(t)
	f.setKeep(2)
	f.svc.engine = budgetedEngine{Engine: f.svc.engine, max: 1, ok: false}

	for i := 0; i < 5; i++ {
		f.snapshot("auto", true)
	}
	var list []store.Snapshot
	f.get("/snapshots?workset=proj-a", http.StatusOK, &list)
	if len(list) != 2 {
		t.Fatalf("graph size = %d, want the configured 2 when the provider reports no cap", len(list))
	}
}
