package engine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// fakeRunner records commands instead of running them. It lets the Btrfs
// backend be tested on a host with no Btrfs filesystem.
type fakeRunner struct {
	calls      []string
	fail       string          // fail the first command containing this text
	subvolumes map[string]bool // paths that answer to `subvolume show`
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) ([]byte, error) {
	line := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, line)
	if f.fail != "" && strings.Contains(line, f.fail) {
		return nil, os.ErrPermission
	}
	// `subvolume show` is how the backend finds the subvolume enclosing a
	// path. It fails on a plain directory, which is the whole point.
	if len(args) == 3 && args[0] == "subvolume" && args[1] == "show" {
		if f.subvolumes[args[2]] {
			return nil, nil
		}
		return nil, os.ErrInvalid
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

	f := &fakeRunner{subvolumes: map[string]bool{work: true}}
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
	if !slices.Contains(f.calls, want) {
		t.Fatalf("calls = %v, want one to be %s", f.calls, want)
	}
	if _, err := os.Stat(filepath.Join(handle, manifestName)); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
}

func TestBtrfsCreateCleansUpAfterFailure(t *testing.T) {
	b, f, work := newFakeBtrfs(t)
	second := filepath.Join(filepath.Dir(work), "work2")
	writeFile(t, filepath.Join(second, "b.txt"), "b")
	f.subvolumes[second] = true
	f.fail = "snapshot -r " + second

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
}

func TestBtrfsSnapshotsAPlainDirectoryThroughItsSubvolume(t *testing.T) {
	b, f, work := newFakeBtrfs(t)
	// A project directory inside a subvolume, which is what an agent points
	// at. btrfs cannot snapshot it directly.
	project := filepath.Join(work, "projects", "alpha")
	writeFile(t, filepath.Join(project, "a.txt"), "a")

	handle, err := b.Create(context.Background(), "01AAA", []string{project})
	if err != nil {
		t.Fatal(err)
	}

	// The snapshot is of the subvolume, not of the directory.
	want := "btrfs subvolume snapshot -r " + work + " " + filepath.Join(handle, subtreeName(0, work))
	if !slices.Contains(f.calls, want) {
		t.Fatalf("calls = %v, want one to be %s", f.calls, want)
	}

	m, err := readManifest(handle)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Sources) != 1 {
		t.Fatalf("sources = %+v, want 1", m.Sources)
	}
	if m.Sources[0].Swappable {
		t.Fatal("a plain directory was marked swappable; a restore would take its siblings too")
	}
	wantDir := filepath.Join(subtreeName(0, work), "projects", "alpha")
	if m.Sources[0].Dir != wantDir {
		t.Fatalf("dir = %q, want %q", m.Sources[0].Dir, wantDir)
	}
}

func TestBtrfsRestoreOfAPlainDirectoryLeavesItsSiblingsAlone(t *testing.T) {
	ctx := context.Background()
	b, f, work := newFakeBtrfs(t)
	project := filepath.Join(work, "projects", "alpha")
	writeFile(t, filepath.Join(project, "a.txt"), "a")
	sibling := filepath.Join(work, "projects", "beta", "theirs.txt")
	writeFile(t, sibling, "theirs")

	handle, err := b.Create(ctx, "01AAA", []string{project})
	if err != nil {
		t.Fatal(err)
	}
	// The fake runner does not copy data, so stage what the snapshot holds.
	m, err := readManifest(handle)
	if err != nil {
		t.Fatal(err)
	}
	stored := filepath.Join(handle, m.Sources[0].Dir)
	if err := os.MkdirAll(stored, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(stored, "a.txt"), "a")

	replace(t, filepath.Join(project, "a.txt"), "wrecked")
	replace(t, sibling, "changed by somebody else")
	f.calls = nil

	if err := b.Restore(ctx, handle); err != nil {
		t.Fatal(err)
	}

	// The declared path comes back.
	if got := read(t, filepath.Join(project, "a.txt")); got != "a" {
		t.Errorf("a.txt = %q, want %q", got, "a")
	}
	// The sibling was never declared, so the restore must not have touched
	// it. Swapping the enclosing subvolume would have reverted it.
	if got := read(t, sibling); got != "changed by somebody else" {
		t.Fatalf("a path nobody declared was restored: %q", got)
	}
	for _, c := range f.calls {
		if strings.Contains(c, "subvolume delete "+work) {
			t.Fatalf("the restore deleted the enclosing subvolume: %v", f.calls)
		}
	}
}

func TestBtrfsStillSwapsWhenTheSourceIsASubvolume(t *testing.T) {
	ctx := context.Background()
	b, f, work := newFakeBtrfs(t)
	handle, err := b.Create(ctx, "01AAA", []string{work})
	if err != nil {
		t.Fatal(err)
	}
	m, err := readManifest(handle)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Sources[0].Swappable {
		t.Fatal("a subvolume source lost its constant-time restore")
	}
	f.calls = nil
	if err := b.Restore(ctx, handle); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.calls, "btrfs subvolume delete "+work) {
		t.Fatalf("calls = %v, want the swap path", f.calls)
	}
}
