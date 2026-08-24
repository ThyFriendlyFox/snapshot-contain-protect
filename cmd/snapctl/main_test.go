package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ThyFriendlyFox/snapshot-contain-protect/internal/api"
)

// newDaemon starts a real service on a loopback port. The client is tested
// against the daemon it ships with, not against a fake.
func newDaemon(t *testing.T) (addr, work string) {
	t.Helper()
	base := t.TempDir()
	work = filepath.Join(base, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The daemon stores the resolved path. A Windows temp directory arrives
	// as an 8.3 short name and resolves to its long form.
	if real, err := filepath.EvalSymlinks(work); err == nil {
		work = real
	}

	cfg := api.DefaultConfig()
	cfg.DataDir = filepath.Join(base, "data")
	cfg.Backend = "copy"
	svc, err := api.NewService(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })

	srv := httptest.NewServer(svc.Handler(slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://"), work
}

func snapctl(t *testing.T, addr string, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := run(append([]string{"-addr", addr}, args...), &out); err != nil {
		t.Fatalf("snapctl %s: %v (output: %s)", strings.Join(args, " "), err, out.String())
	}
	return out.String()
}

func TestClientRunsTheWholeCycle(t *testing.T) {
	addr, work := newDaemon(t)

	if got := snapctl(t, addr, "workset", "proj-a", work); !strings.Contains(got, "proj-a covers") {
		t.Fatalf("workset output = %q", got)
	}
	if got := snapctl(t, addr, "worksets"); !strings.Contains(got, "proj-a") {
		t.Fatalf("worksets output = %q", got)
	}

	first := idFrom(t, snapctl(t, addr, "snapshot", "proj-a", "before", "cleanup"))

	if err := os.WriteFile(filepath.Join(work, "b.txt"), []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	second := idFrom(t, snapctl(t, addr, "snapshot", "proj-a"))

	diff := snapctl(t, addr, "diff", first, second)
	if !strings.Contains(diff, "A "+filepath.Join(work, "b.txt")) {
		t.Fatalf("diff output = %q", diff)
	}

	list := snapctl(t, addr, "list", "proj-a")
	if !strings.Contains(list, first) || !strings.Contains(list, "before cleanup") {
		t.Fatalf("list output = %q", list)
	}
	// The child is indented under its parent.
	if !strings.Contains(list, "\n  "+second) {
		t.Fatalf("list did not nest the child: %q", list)
	}

	restore := snapctl(t, addr, "restore", first)
	if !strings.Contains(restore, "restored "+first) {
		t.Fatalf("restore output = %q", restore)
	}
	if _, err := os.Stat(filepath.Join(work, "b.txt")); !os.IsNotExist(err) {
		t.Error("b.txt survived the restore")
	}

	// The safety snapshot the restore took hangs off `second`, so the cascade
	// removes both.
	pruned := snapctl(t, addr, "prune", second, "--cascade")
	if !strings.Contains(pruned, "removed 2 snapshot") {
		t.Fatalf("prune output = %q", pruned)
	}
}

func TestClientPrintsJSONWhenAsked(t *testing.T) {
	addr, work := newDaemon(t)
	snapctl(t, addr, "workset", "proj-a", work)
	got := snapctl(t, addr, "-json", "snapshot", "proj-a", "raw")
	if !strings.Contains(got, `"duration_ms"`) {
		t.Fatalf("json output = %q", got)
	}
}

func TestClientReportsDaemonErrorsPlainly(t *testing.T) {
	addr, _ := newDaemon(t)
	var out bytes.Buffer
	err := run([]string{"-addr", addr, "snapshot", "absent"}, &out)
	if err == nil || !strings.Contains(err.Error(), `unknown workset "absent"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestClientReportsAMissingDaemon(t *testing.T) {
	var out bytes.Buffer
	err := run([]string{"-addr", "127.0.0.1:1", "worksets"}, &out)
	if err == nil || !strings.Contains(err.Error(), "no daemon at 127.0.0.1:1") {
		t.Fatalf("error = %v", err)
	}
}

func TestUnknownCommandFails(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"frobnicate"}, &out); err == nil {
		t.Fatal("an unknown command was accepted")
	}
}

func idFrom(t *testing.T, line string) string {
	t.Helper()
	fields := strings.Fields(line)
	if len(fields) == 0 {
		t.Fatalf("no identifier in %q", line)
	}
	return fields[0]
}
