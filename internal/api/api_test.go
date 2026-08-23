package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ThyFriendlyFox/snapshot-contain-protect/internal/engine"
	"github.com/ThyFriendlyFox/snapshot-contain-protect/internal/store"
)

type fixture struct {
	t    *testing.T
	srv  *httptest.Server
	svc  *Service
	work string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	base := t.TempDir()
	work := filepath.Join(base, "work")
	writeFile(t, filepath.Join(work, "config.json"), "{}")
	writeFile(t, filepath.Join(work, "keep.txt"), "keep")

	cfg := DefaultConfig()
	cfg.DataDir = filepath.Join(base, "data")
	cfg.Backend = "copy"
	cfg.PruneInterval = 0

	svc, err := NewService(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })

	srv := httptest.NewServer(svc.Handler(slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(srv.Close)

	f := &fixture{t: t, srv: srv, svc: svc, work: work}
	f.post("/worksets", worksetRequest{Name: "proj-a", Paths: []string{work}}, http.StatusCreated, nil)
	return f
}

func (f *fixture) do(method, path string, body any, wantStatus int, into any) {
	f.t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			f.t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, f.srv.URL+path, reader)
	if err != nil {
		f.t.Fatal(err)
	}
	resp, err := f.srv.Client().Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		f.t.Fatalf("%s %s = %d, want %d: %s", method, path, resp.StatusCode, wantStatus, raw)
	}
	if into != nil {
		if err := json.Unmarshal(raw, into); err != nil {
			f.t.Fatalf("decode %s: %v: %s", path, err, raw)
		}
	}
}

func (f *fixture) post(path string, body any, wantStatus int, into any) {
	f.t.Helper()
	f.do(http.MethodPost, path, body, wantStatus, into)
}

func (f *fixture) get(path string, wantStatus int, into any) {
	f.t.Helper()
	f.do(http.MethodGet, path, nil, wantStatus, into)
}

func (f *fixture) snapshot(label string, auto bool) snapshotResponse {
	f.t.Helper()
	var got snapshotResponse
	f.post("/snapshot", snapshotRequest{Workset: "proj-a", Label: label, Auto: auto}, http.StatusCreated, &got)
	return got
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func replace(t *testing.T, path, body string) {
	t.Helper()
	tmp := path + ".tmp"
	writeFile(t, tmp, body)
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotReturnsAnIdentifierAndDuration(t *testing.T) {
	f := newFixture(t)
	got := f.snapshot("before file cleanup", false)
	if len(got.ID) != 26 {
		t.Fatalf("id = %q, want a 26-character ULID", got.ID)
	}
	if got.CreatedAt == 0 {
		t.Error("created_at is zero")
	}
	if got.DurationM < 0 {
		t.Errorf("duration_ms = %d", got.DurationM)
	}
}

func TestSnapshotsListIsTheGraph(t *testing.T) {
	f := newFixture(t)
	first := f.snapshot("first", false)
	second := f.snapshot("second", true)

	var list []store.Snapshot
	f.get("/snapshots?workset=proj-a", http.StatusOK, &list)
	if len(list) != 2 {
		t.Fatalf("list length = %d, want 2", len(list))
	}
	if list[0].ID != first.ID || list[1].ID != second.ID {
		t.Fatalf("order = %s,%s; want %s,%s", list[0].ID, list[1].ID, first.ID, second.ID)
	}
	if list[1].ParentID == nil || *list[1].ParentID != first.ID {
		t.Fatalf("second parent = %v, want %s", list[1].ParentID, first.ID)
	}
	if !list[1].Auto || list[0].Auto {
		t.Error("the auto flag did not survive the round trip")
	}
}

func TestDiffReportsPathsByCategory(t *testing.T) {
	f := newFixture(t)
	first := f.snapshot("first", false)
	replace(t, filepath.Join(f.work, "config.json"), `{"a":1}`)
	writeFile(t, filepath.Join(f.work, "new.txt"), "new")
	if err := os.Remove(filepath.Join(f.work, "keep.txt")); err != nil {
		t.Fatal(err)
	}
	second := f.snapshot("second", false)

	var change engine.Change
	f.get("/diff?from="+first.ID+"&to="+second.ID, http.StatusOK, &change)
	if len(change.Added) != 1 || change.Added[0] != filepath.Join(f.work, "new.txt") {
		t.Errorf("added = %v", change.Added)
	}
	if len(change.Modified) != 1 || change.Modified[0] != filepath.Join(f.work, "config.json") {
		t.Errorf("modified = %v", change.Modified)
	}
	if len(change.Deleted) != 1 || change.Deleted[0] != filepath.Join(f.work, "keep.txt") {
		t.Errorf("deleted = %v", change.Deleted)
	}
	if change.Truncated {
		t.Error("truncated is true for a 3-path diff")
	}
}

func TestRestoreNeedsConfirm(t *testing.T) {
	f := newFixture(t)
	first := f.snapshot("first", false)
	f.post("/restore", restoreRequest{ID: first.ID}, http.StatusBadRequest, nil)
}

func TestRestoreReturnsTheWorkingSetAndAppendsANode(t *testing.T) {
	f := newFixture(t)
	first := f.snapshot("first", false)
	replace(t, filepath.Join(f.work, "config.json"), "wrecked")
	writeFile(t, filepath.Join(f.work, "junk.txt"), "junk")

	var created store.Snapshot
	f.post("/restore", restoreRequest{ID: first.ID, Confirm: true}, http.StatusCreated, &created)

	body, err := os.ReadFile(filepath.Join(f.work, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "{}" {
		t.Errorf("config.json = %q, want %q", body, "{}")
	}
	if _, err := os.Stat(filepath.Join(f.work, "junk.txt")); !os.IsNotExist(err) {
		t.Error("junk.txt survived the restore")
	}
	if created.ParentID == nil || *created.ParentID != first.ID {
		t.Fatalf("new node parent = %v, want %s", created.ParentID, first.ID)
	}

	// The restore keeps history: the safety snapshot and the new node join it.
	var list []store.Snapshot
	f.get("/snapshots?workset=proj-a", http.StatusOK, &list)
	if len(list) != 3 {
		t.Fatalf("graph size = %d, want 3", len(list))
	}
}

func TestPruneRefusesANodeWithChildren(t *testing.T) {
	f := newFixture(t)
	first := f.snapshot("first", false)
	f.snapshot("second", false)

	f.do(http.MethodDelete, "/snapshots/"+first.ID, nil, http.StatusConflict, nil)

	var pruned pruneResponse
	f.do(http.MethodDelete, "/snapshots/"+first.ID+"?cascade=true", nil, http.StatusOK, &pruned)
	if len(pruned.Removed) != 2 {
		t.Fatalf("removed = %v, want 2 identifiers", pruned.Removed)
	}
	var list []store.Snapshot
	f.get("/snapshots?workset=proj-a", http.StatusOK, &list)
	if len(list) != 0 {
		t.Fatalf("graph size after cascade = %d, want 0", len(list))
	}
}

func TestPruneRemovesTheHandle(t *testing.T) {
	f := newFixture(t)
	only := f.snapshot("only", false)
	sn, err := f.svc.store.SnapshotByID(context.Background(), only.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.do(http.MethodDelete, "/snapshots/"+only.ID, nil, http.StatusOK, nil)
	if _, err := os.Stat(sn.FSHandle); !os.IsNotExist(err) {
		t.Fatal("the snapshot handle survived the prune")
	}
}

func TestUnknownWorksetAndSnapshotAre404(t *testing.T) {
	f := newFixture(t)
	f.post("/snapshot", snapshotRequest{Workset: "absent"}, http.StatusNotFound, nil)
	f.get("/snapshots?workset=absent", http.StatusNotFound, nil)
	f.get("/diff?from=01ABSENT&to=01ABSENT", http.StatusNotFound, nil)
	f.do(http.MethodDelete, "/snapshots/01ABSENT", nil, http.StatusNotFound, nil)
}

func TestMissingQueryParametersAre400(t *testing.T) {
	f := newFixture(t)
	f.get("/snapshots", http.StatusBadRequest, nil)
	f.get("/diff?from=01A", http.StatusBadRequest, nil)
	f.post("/worksets", worksetRequest{Name: "relative", Paths: []string{"work"}}, http.StatusBadRequest, nil)
}

func TestContainerWorksetRefusesUntilTheLayerExists(t *testing.T) {
	f := newFixture(t)
	f.post("/worksets", worksetRequest{Name: "boxed", Paths: []string{f.work}, Container: true}, http.StatusCreated, nil)
	f.post("/snapshot", snapshotRequest{Workset: "boxed"}, http.StatusNotImplemented, nil)
}

func TestRedeclaringAWorksetKeepsItsGraph(t *testing.T) {
	f := newFixture(t)
	first := f.snapshot("first", false)
	f.post("/worksets", worksetRequest{Name: "proj-a", Paths: []string{f.work}}, http.StatusCreated, nil)

	var list []store.Snapshot
	f.get("/snapshots?workset=proj-a", http.StatusOK, &list)
	if len(list) != 1 || list[0].ID != first.ID {
		t.Fatalf("graph after redeclaring = %v", list)
	}
}

func TestHealthReportsTheBackend(t *testing.T) {
	f := newFixture(t)
	var got map[string]string
	f.get("/healthz", http.StatusOK, &got)
	if got["backend"] != "copy" {
		t.Fatalf("backend = %q, want copy", got["backend"])
	}
}

func TestServeRefusesANonLoopbackAddress(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:7099", "192.168.1.10:7099", "nonsense"} {
		if err := checkLoopback(addr); err == nil {
			t.Errorf("checkLoopback(%q) allowed a reachable address", addr)
		}
	}
	for _, addr := range []string{"127.0.0.1:7099", "[::1]:7099", "localhost:7099"} {
		if err := checkLoopback(addr); err != nil {
			t.Errorf("checkLoopback(%q) = %v", addr, err)
		}
	}
}

func TestRestoreWorksAfterTheWorksetPathIsDeleted(t *testing.T) {
	f := newFixture(t)
	first := f.snapshot("first", false)

	// The case a rollback exists for: the agent deleted the working set.
	if err := os.RemoveAll(f.work); err != nil {
		t.Fatal(err)
	}

	var created store.Snapshot
	f.post("/restore", restoreRequest{ID: first.ID, Confirm: true}, http.StatusCreated, &created)

	body, err := os.ReadFile(filepath.Join(f.work, "config.json"))
	if err != nil {
		t.Fatalf("the working set did not come back: %v", err)
	}
	if string(body) != "{}" {
		t.Errorf("config.json = %q, want %q", body, "{}")
	}
}

func TestAMissingWorksetPathIs400NotAServerError(t *testing.T) {
	f := newFixture(t)
	if err := os.RemoveAll(f.work); err != nil {
		t.Fatal(err)
	}
	// The caller declared the path, so the caller is the one who can fix it.
	f.post("/snapshot", snapshotRequest{Workset: "proj-a"}, http.StatusBadRequest, nil)
}

func TestWorksetResolvesASymlinkedPath(t *testing.T) {
	f := newFixture(t)
	link := filepath.Join(filepath.Dir(f.work), "link")
	if err := os.Symlink(f.work, link); err != nil {
		t.Fatal(err)
	}
	var ws store.Workset
	f.post("/worksets", worksetRequest{Name: "linked", Paths: []string{link}}, http.StatusCreated, &ws)

	resolved, err := filepath.EvalSymlinks(f.work)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Paths) != 1 || ws.Paths[0] != resolved {
		t.Fatalf("stored paths = %v, want [%s]", ws.Paths, resolved)
	}
	// The snapshot must hold the files, not an empty directory.
	var got snapshotResponse
	f.post("/snapshot", snapshotRequest{Workset: "linked"}, http.StatusCreated, &got)
	sn, err := f.svc.store.SnapshotByID(context.Background(), got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count := countFiles(t, sn.FSHandle); count < 2 {
		t.Fatalf("the snapshot holds %d files, want the 2 in the working set", count)
	}
}

func TestWorksetRefusesAPathThatHoldsTheSnapshotRoot(t *testing.T) {
	f := newFixture(t)
	// The data directory is inside the fixture's base directory.
	base := filepath.Dir(f.work)
	f.post("/worksets", worksetRequest{Name: "everything", Paths: []string{base}}, http.StatusCreated, nil)
	f.post("/snapshot", snapshotRequest{Workset: "everything"}, http.StatusBadRequest, nil)
}

// failingEngine refuses every delete, which is how a busy Btrfs subvolume
// behaves.
type failingEngine struct{ engine.Engine }

func (failingEngine) Delete(context.Context, string) error {
	return errors.New("subvolume is busy")
}

func TestPruneKeepsTheRowWhenTheHandleWillNotGo(t *testing.T) {
	f := newFixture(t)
	only := f.snapshot("only", false)
	f.svc.engine = failingEngine{f.svc.engine}

	f.do(http.MethodDelete, "/snapshots/"+only.ID, nil, http.StatusInternalServerError, nil)

	// The graph still names the snapshot, so the next prune can try again.
	var list []store.Snapshot
	f.get("/snapshots?workset=proj-a", http.StatusOK, &list)
	if len(list) != 1 || list[0].ID != only.ID {
		t.Fatalf("graph = %v, want the snapshot whose handle survived", list)
	}
}

func TestRetentionKeepsTheRowWhenTheHandleWillNotGo(t *testing.T) {
	f := newFixture(t)
	f.setKeep(1)
	f.snapshot("auto", true)
	f.svc.engine = failingEngine{f.svc.engine}
	f.snapshot("auto", true)

	var list []store.Snapshot
	f.get("/snapshots?workset=proj-a", http.StatusOK, &list)
	if len(list) != 2 {
		t.Fatalf("graph size = %d, want 2: retention must not drop a row whose handle stayed", len(list))
	}
}

func countFiles(t *testing.T, root string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func TestWorksetMayNameAPathThatDoesNotExistYet(t *testing.T) {
	f := newFixture(t)
	later := filepath.Join(filepath.Dir(f.work), "not-yet")
	f.post("/worksets", worksetRequest{Name: "later", Paths: []string{later}}, http.StatusCreated, nil)

	// The snapshot is where a missing path is refused, and it says so as a
	// caller error.
	f.post("/snapshot", snapshotRequest{Workset: "later"}, http.StatusBadRequest, nil)

	if err := os.MkdirAll(later, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(later, "a.txt"), "a")
	f.post("/snapshot", snapshotRequest{Workset: "later"}, http.StatusCreated, nil)
}
