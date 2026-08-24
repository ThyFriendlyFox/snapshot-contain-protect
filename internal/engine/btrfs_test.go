package engine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeRunner records commands instead of running them. It lets the Btrfs
// backend be tested on a host with no Btrfs filesystem.
type fakeRunner struct {
	calls []string
	fail  string // fail the first command containing this text
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) ([]byte, error) {
	line := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, line)
	if f.fail != "" && strings.Contains(line, f.fail) {
		return nil, os.ErrPermission
	}
	// A real snapshot creates the destination directory. Fake that much, so
	// the manifest and the differ have something to read.
	if len(args) >= 3 && args[0] == "subvolume" && args[1] == "snapshot" {
		_ = os.MkdirAll(args[len(args)-1], 0o755)
	}
	if len(args) == 3 && args[0] == "subvolume" && args[1] == "delete" {
		_ = os.RemoveAll(args[2])
	}
	return nil, nil
}

func newFakeBtrfs(t *testing.T) (*Btrfs, *fakeRunner, string) {
	t.Helper()
	base := t.TempDir()
	work := filepath.Join(base, "work")
	writeFile(t, filepath.Join(work, "a.txt"), "a")

	f := &fakeRunner{}
	b := NewBtrfs(filepath.Join(base, "snapshots"))
	b.run = f.run
	if err := os.MkdirAll(b.root, 0o755); err != nil {
		t.Fatal(err)
	}
	return b, f, work
}

func TestBtrfsCreateCallsSubvolumeSnapshot(t *testing.T) {
	b, f, work := newFakeBtrfs(t)
	handle, err := b.Create(context.Background(), "01AAA", []string{work})
	if err != nil {
		t.Fatal(err)
	}
	want := "btrfs subvolume snapshot -r " + work + " " + filepath.Join(handle, subtreeName(0, work))
	if len(f.calls) != 1 || f.calls[0] != want {
		t.Fatalf("calls = %v, want [%s]", f.calls, want)
	}
	if _, err := os.Stat(filepath.Join(handle, manifestName)); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
}

func TestBtrfsCreateCleansUpAfterFailure(t *testing.T) {
	b, f, work := newFakeBtrfs(t)
	second := filepath.Join(filepath.Dir(work), "work2")
	writeFile(t, filepath.Join(second, "b.txt"), "b")
	f.fail = "work2"

	if _, err := b.Create(context.Background(), "01AAA", []string{work, second}); err == nil {
		t.Fatal("a failed snapshot reported success")
	}
	if _, err := os.Stat(filepath.Join(b.root, "01AAA")); !os.IsNotExist(err) {
		t.Fatal("the failed handle was left behind")
	}
}

func TestBtrfsDeleteRemovesEverySubvolume(t *testing.T) {
	b, f, work := newFakeBtrfs(t)
	handle, err := b.Create(context.Background(), "01AAA", []string{work})
	if err != nil {
		t.Fatal(err)
	}
	f.calls = nil
	if err := b.Delete(context.Background(), handle); err != nil {
		t.Fatal(err)
	}
	want := "btrfs subvolume delete " + filepath.Join(handle, subtreeName(0, work))
	if len(f.calls) != 1 || f.calls[0] != want {
		t.Fatalf("calls = %v, want [%s]", f.calls, want)
	}
}

func TestBtrfsRestoreSwapsTheSubvolume(t *testing.T) {
	b, f, work := newFakeBtrfs(t)
	handle, err := b.Create(context.Background(), "01AAA", []string{work})
	if err != nil {
		t.Fatal(err)
	}
	f.calls = nil
	if err := b.Restore(context.Background(), handle); err != nil {
		t.Fatal(err)
	}
	stored := filepath.Join(handle, subtreeName(0, work))
	want := []string{
		"btrfs subvolume snapshot " + stored + " " + work + ".snapshot-restore",
		"btrfs subvolume delete " + work,
	}
	if len(f.calls) != 2 || f.calls[0] != want[0] || f.calls[1] != want[1] {
		t.Fatalf("calls = %v, want %v", f.calls, want)
	}
}

func TestBtrfsUnavailableWithoutTheTool(t *testing.T) {
	// Look the command up the way the backend does. A hardcoded path misses
	// an install under /usr/bin and turns this into a false pass.
	if _, err := exec.LookPath("btrfs"); err == nil {
		t.Skip("this host has btrfs tooling")
	}
	b := NewBtrfs(t.TempDir())
	if err := b.Available(); err == nil {
		t.Fatal("Available reported a btrfs host with no btrfs command")
	}
}

func TestSelectRejectsAnUnknownBackend(t *testing.T) {
	if _, err := Select("zfs", t.TempDir()); err == nil {
		t.Fatal("an unknown backend was accepted")
	}
}

func TestSelectAutoFallsBackToCopy(t *testing.T) {
	e, err := Select("auto", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if e.Name() != "btrfs" && e.Name() != "copy" {
		t.Fatalf("auto chose %q", e.Name())
	}
}

func TestStubBackendsRefuseCleanly(t *testing.T) {
	if err := NewAPFS().Available(); err == nil {
		t.Fatal("apfs reported itself available")
	}
	if _, err := NewAPFS().Create(context.Background(), "01AAA", nil); err == nil {
		t.Fatal("apfs created a snapshot")
	}
}

func TestVSSRefusesOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this host is windows; the live gate covers it")
	}
	v := NewVSS(t.TempDir())
	err := v.Available()
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Available = %v, want ErrUnavailable", err)
	}
	if !strings.Contains(err.Error(), "windows") {
		t.Fatalf("the error does not say why: %v", err)
	}
	if err := v.Restore(context.Background(), "handle"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Restore = %v, want ErrUnavailable until item 5", err)
	}
}
