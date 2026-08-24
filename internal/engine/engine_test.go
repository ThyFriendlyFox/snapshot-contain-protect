package engine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"
)

// tempDir is t.TempDir plus a cleanup that makes every directory writable
// again. A test that stores a read-only directory would otherwise defeat Go's
// own cleanup when the suite runs as a normal user.
func tempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Registered after TempDir's own cleanup, so it runs before it.
	t.Cleanup(func() {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				_ = os.Chmod(p, 0o700)
			}
			return nil
		})
	})
	return dir
}

// mustSymlink creates a symlink, or skips the test with the reason the host
// refused. Windows needs Developer Mode or elevation to make one, and a test
// that cannot run must say so rather than fail or pass quietly.
func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("this host refuses symlink creation: %v", err)
	}
}

// requirePOSIXModes skips a test that asserts permission bits. On Windows
// os.Chmod only toggles the read-only attribute, so mode preservation is a
// guarantee this project makes on Unix alone. docs/BACKENDS.md states it.
func requirePOSIXModes(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("windows has no POSIX permission bits; mode preservation is a unix guarantee")
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// The differ compares modification time. Tests write faster than the
	// clock's resolution on some filesystems, so set the time explicitly.
	stamp := time.Unix(1755900000, int64(len(body))*1000)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
}

// replace writes a file the way a careful writer does: new file, then rename.
// The copy backend depends on this, and so does every editor.
func replace(t *testing.T, path, body string) {
	t.Helper()
	tmp := path + ".tmp"
	writeFile(t, tmp, body)
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
}

func newCopyFixture(t *testing.T) (*Copy, string) {
	t.Helper()
	base := t.TempDir()
	work := filepath.Join(base, "work")
	writeFile(t, filepath.Join(work, "keep.txt"), "keep")
	writeFile(t, filepath.Join(work, "config.json"), "{}")
	writeFile(t, filepath.Join(work, "logs", "old.log"), "old")

	e := NewCopy(filepath.Join(base, "snapshots"))
	if err := e.Available(); err != nil {
		t.Fatal(err)
	}
	return e, work
}

func TestCopyCreateAndDiff(t *testing.T) {
	ctx := context.Background()
	e, work := newCopyFixture(t)

	first, err := e.Create(ctx, "01AAA", []string{work})
	if err != nil {
		t.Fatal(err)
	}

	replace(t, filepath.Join(work, "config.json"), `{"changed":true}`)
	writeFile(t, filepath.Join(work, "new.txt"), "new")
	if err := os.Remove(filepath.Join(work, "logs", "old.log")); err != nil {
		t.Fatal(err)
	}

	second, err := e.Create(ctx, "01BBB", []string{work})
	if err != nil {
		t.Fatal(err)
	}

	c, err := e.Diff(ctx, first, second)
	if err != nil {
		t.Fatal(err)
	}
	wantAdded := []string{filepath.Join(work, "new.txt")}
	wantModified := []string{filepath.Join(work, "config.json")}
	wantDeleted := []string{filepath.Join(work, "logs", "old.log")}
	if !slices.Equal(c.Added, wantAdded) {
		t.Errorf("added = %v, want %v", c.Added, wantAdded)
	}
	if !slices.Equal(c.Modified, wantModified) {
		t.Errorf("modified = %v, want %v", c.Modified, wantModified)
	}
	if !slices.Equal(c.Deleted, wantDeleted) {
		t.Errorf("deleted = %v, want %v", c.Deleted, wantDeleted)
	}
	if c.Truncated {
		t.Error("truncated is true for a 3-path diff")
	}
}

func TestCopyDiffOfIdenticalSnapshotsIsEmpty(t *testing.T) {
	ctx := context.Background()
	e, work := newCopyFixture(t)

	a, err := e.Create(ctx, "01AAA", []string{work})
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.Create(ctx, "01BBB", []string{work})
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.Diff(ctx, a, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Added)+len(c.Modified)+len(c.Deleted) != 0 {
		t.Fatalf("unchanged working set diffs as %+v", c)
	}
}

func TestCopyRestoreReturnsExactPriorState(t *testing.T) {
	ctx := context.Background()
	e, work := newCopyFixture(t)

	mustSymlink(t, "keep.txt", filepath.Join(work, "link"))
	handle, err := e.Create(ctx, "01AAA", []string{work})
	if err != nil {
		t.Fatal(err)
	}

	replace(t, filepath.Join(work, "config.json"), "wrecked")
	writeFile(t, filepath.Join(work, "junk", "a.txt"), "junk")
	if err := os.RemoveAll(filepath.Join(work, "logs")); err != nil {
		t.Fatal(err)
	}

	if err := e.Restore(ctx, handle); err != nil {
		t.Fatal(err)
	}

	if got := read(t, filepath.Join(work, "config.json")); got != "{}" {
		t.Errorf("config.json = %q, want %q", got, "{}")
	}
	if got := read(t, filepath.Join(work, "logs", "old.log")); got != "old" {
		t.Errorf("logs/old.log = %q, want %q", got, "old")
	}
	if _, err := os.Lstat(filepath.Join(work, "junk")); !os.IsNotExist(err) {
		t.Error("junk/ survived the restore")
	}
	target, err := os.Readlink(filepath.Join(work, "link"))
	if err != nil || target != "keep.txt" {
		t.Errorf("symlink = %q, %v; want keep.txt", target, err)
	}

	// The restored tree must diff clean against the snapshot it came from.
	after, err := e.Create(ctx, "01CCC", []string{work})
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.Diff(ctx, handle, after)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Added)+len(c.Modified)+len(c.Deleted) != 0 {
		t.Fatalf("restored tree differs from its snapshot: %+v", c)
	}
}

func TestCopyRestoreDoesNotShareInodesWithSnapshot(t *testing.T) {
	ctx := context.Background()
	e, work := newCopyFixture(t)
	handle, err := e.Create(ctx, "01AAA", []string{work})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Restore(ctx, handle); err != nil {
		t.Fatal(err)
	}
	// Write in place, the unsafe way. The snapshot must not follow.
	f, err := os.OpenFile(filepath.Join(work, "keep.txt"), os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("clobbered"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	stored := filepath.Join(handle, subtreeName(0, work), "keep.txt")
	if got := read(t, stored); got != "keep" {
		t.Fatalf("snapshot copy = %q, want %q", got, "keep")
	}
}

func TestCopyDeleteRemovesTheSnapshot(t *testing.T) {
	ctx := context.Background()
	e, work := newCopyFixture(t)
	handle, err := e.Create(ctx, "01AAA", []string{work})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Delete(ctx, handle); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(handle); !os.IsNotExist(err) {
		t.Fatal("handle survived delete")
	}
}

func TestDiffCapsEachCategory(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	work := filepath.Join(base, "work")
	writeFile(t, filepath.Join(work, "seed.txt"), "seed")
	e := NewCopy(filepath.Join(base, "snapshots"))

	first, err := e.Create(ctx, "01AAA", []string{work})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < DiffCap+10; i++ {
		writeFile(t, filepath.Join(work, fmt.Sprintf("f%04d.txt", i)), "x")
	}
	second, err := e.Create(ctx, "01BBB", []string{work})
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.Diff(ctx, first, second)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Added) != DiffCap {
		t.Errorf("added count = %d, want %d", len(c.Added), DiffCap)
	}
	if !c.Truncated {
		t.Error("truncated is false above the cap")
	}
}

func TestCopyRejectsRelativeAndMissingPaths(t *testing.T) {
	ctx := context.Background()
	e := NewCopy(t.TempDir())
	if _, err := e.Create(ctx, "01AAA", []string{"relative/path"}); err == nil {
		t.Error("a relative path was accepted")
	}
	if _, err := e.Create(ctx, "01AAA", nil); err == nil {
		t.Error("an empty workset was accepted")
	}
	if _, err := e.Create(ctx, "01AAA", []string{filepath.Join(t.TempDir(), "absent")}); err == nil {
		t.Error("a missing path was accepted")
	}
}

func TestSnapshotOfLargeSetStaysCheap(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	work := filepath.Join(base, "work")
	for i := 0; i < 200; i++ {
		writeFile(t, filepath.Join(work, fmt.Sprintf("f%03d.bin", i)), string(make([]byte, 4096)))
	}
	e := NewCopy(filepath.Join(base, "snapshots"))

	start := time.Now()
	for i := 0; i < 100; i++ {
		if _, err := e.Create(ctx, fmt.Sprintf("01SEQ%03d", i), []string{work}); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("100 snapshots of 200 files took %s", time.Since(start))

	used := diskUsage(t, filepath.Join(base, "snapshots"))
	// Hardlinked snapshots hold no data blocks. 100 snapshots of 800 KB of
	// files must stay far under the 100 MB acceptance budget.
	if used > 100<<20 {
		t.Fatalf("100 snapshots used %d bytes, want under 100 MB", used)
	}
}

// diskUsage counts each inode once, the way `du` does.
func diskUsage(t *testing.T, root string) int64 {
	t.Helper()
	var total int64
	seen := map[uint64]bool{}
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		ino, ok := inodeOf(fi)
		if ok {
			if seen[ino] {
				return nil
			}
			seen[ino] = true
		}
		total += fi.Size()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return total
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCopyRejectsASymlinkedWorksetPath(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	real := filepath.Join(base, "real")
	writeFile(t, filepath.Join(real, "important.txt"), "important")
	link := filepath.Join(base, "link")
	mustSymlink(t, real, link)

	e := NewCopy(filepath.Join(base, "snapshots"))
	// A tree walk does not descend through a symlinked root. Snapshotting it
	// would store nothing and a restore would replace the link with an empty
	// directory.
	_, err := e.Create(ctx, "01AAA", []string{link})
	if !errors.Is(err, ErrBadSource) {
		t.Fatalf("error = %v, want ErrBadSource", err)
	}
	if _, err := os.Readlink(link); err != nil {
		t.Fatalf("the symlink did not survive: %v", err)
	}
}

func TestCopyRejectsAPathThatContainsTheSnapshotRoot(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	// The default data directory sits under the home directory, so a workset
	// of the home directory would snapshot the snapshot root.
	root := filepath.Join(base, "data", "snapshots")
	e := NewCopy(root)
	if err := e.Available(); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Create(ctx, "01AAA", []string{base}); !errors.Is(err, ErrBadSource) {
		t.Fatalf("error = %v, want ErrBadSource", err)
	}
	if _, err := e.Create(ctx, "01AAA", []string{root}); !errors.Is(err, ErrBadSource) {
		t.Fatalf("error for the root itself = %v, want ErrBadSource", err)
	}
}

func TestCopyRestoreReturnsExactModes(t *testing.T) {
	requirePOSIXModes(t)
	ctx := context.Background()
	base := t.TempDir()
	work := filepath.Join(base, "work")
	writeFile(t, filepath.Join(work, "wide.sh"), "#!/bin/sh\n")
	writeFile(t, filepath.Join(work, "sticky", "a.txt"), "a")
	if err := os.Chmod(filepath.Join(work, "wide.sh"), 0o776); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(work, "sticky"), 0o777); err != nil {
		t.Fatal(err)
	}

	e := NewCopy(filepath.Join(base, "snapshots"))
	handle, err := e.Create(ctx, "01AAA", []string{work})
	if err != nil {
		t.Fatal(err)
	}
	// Delete the file and narrow the directory. A chmod of the live file would
	// not prove anything: the copy backend hardlinks, so the stored name
	// points at the same inode and the same mode.
	if err := os.Remove(filepath.Join(work, "wide.sh")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(work, "sticky"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := e.Restore(ctx, handle); err != nil {
		t.Fatal(err)
	}

	// open(2) and mkdir(2) apply the umask. A restore must not.
	if got := modeOf(t, filepath.Join(work, "wide.sh")); got != 0o776 {
		t.Errorf("file mode = %#o, want 0776", got)
	}
	if got := modeOf(t, filepath.Join(work, "sticky")); got != 0o777 {
		t.Errorf("directory mode = %#o, want 0777", got)
	}
}

func TestCopyHandlesAReadOnlyDirectory(t *testing.T) {
	requirePOSIXModes(t)
	ctx := context.Background()
	base := tempDir(t)
	work := filepath.Join(base, "work")
	locked := filepath.Join(work, "locked")
	writeFile(t, filepath.Join(locked, "a.txt"), "a")
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	e := NewCopy(filepath.Join(base, "snapshots"))
	handle, err := e.Create(ctx, "01AAA", []string{work})
	if err != nil {
		t.Fatalf("snapshot of a read-only directory failed: %v", err)
	}
	if got := modeOf(t, filepath.Join(handle, subtreeName(0, work), "locked")); got != 0o555 {
		t.Errorf("stored directory mode = %#o, want 0555", got)
	}
	if err := e.Restore(ctx, handle); err != nil {
		t.Fatalf("restore of a read-only directory failed: %v", err)
	}
	if got := modeOf(t, locked); got != 0o555 {
		t.Errorf("restored directory mode = %#o, want 0555", got)
	}
}

func modeOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

func TestDeleteRemovesAHandleHoldingAReadOnlyDirectory(t *testing.T) {
	requirePOSIXModes(t)
	ctx := context.Background()
	base := tempDir(t)
	work := filepath.Join(base, "work")
	locked := filepath.Join(work, "locked")
	writeFile(t, filepath.Join(locked, "a.txt"), "a")
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	e := NewCopy(filepath.Join(base, "snapshots"))
	handle, err := e.Create(ctx, "01AAA", []string{work})
	if err != nil {
		t.Fatal(err)
	}
	// The stored copy keeps mode 0555, and unlinking its children needs write
	// permission on it. A handle that cannot be deleted can never be pruned.
	if err := e.Delete(ctx, handle); err != nil {
		t.Fatalf("delete of a handle with a read-only directory failed: %v", err)
	}
	if _, err := os.Stat(handle); !os.IsNotExist(err) {
		t.Fatal("the handle survived delete")
	}
}

func TestRestoreOverAReadOnlyDirectoryLeavesNoStaleTree(t *testing.T) {
	requirePOSIXModes(t)
	ctx := context.Background()
	base := tempDir(t)
	work := filepath.Join(base, "work")
	locked := filepath.Join(work, "locked")
	writeFile(t, filepath.Join(locked, "a.txt"), "a")
	writeFile(t, filepath.Join(work, "config.json"), "{}")
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	e := NewCopy(filepath.Join(base, "snapshots"))
	handle, err := e.Create(ctx, "01AAA", []string{work})
	if err != nil {
		t.Fatal(err)
	}
	replace(t, filepath.Join(work, "config.json"), "wrecked")

	// The first restore must not leave <path>.snapshot-previous behind: a
	// stale one makes every later restore fail with "file exists".
	for i := 0; i < 2; i++ {
		if err := e.Restore(ctx, handle); err != nil {
			t.Fatalf("restore %d failed: %v", i+1, err)
		}
		if _, err := os.Lstat(work + ".snapshot-previous"); !os.IsNotExist(err) {
			t.Fatalf("restore %d left a stale previous tree", i+1)
		}
		if got := read(t, filepath.Join(work, "config.json")); got != "{}" {
			t.Fatalf("restore %d: config.json = %q, want %q", i+1, got, "{}")
		}
	}
}

func TestRestoreRefusesADestinationThatBecameASymlink(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	work := filepath.Join(base, "work")
	writeFile(t, filepath.Join(work, "a.txt"), "a")

	e := NewCopy(filepath.Join(base, "snapshots"))
	handle, err := e.Create(ctx, "01AAA", []string{work})
	if err != nil {
		t.Fatal(err)
	}

	// The declared path is now a link to somebody else's work.
	elsewhere := filepath.Join(base, "elsewhere")
	writeFile(t, filepath.Join(elsewhere, "theirs.txt"), "theirs")
	if err := os.RemoveAll(work); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, elsewhere, work)

	if err := e.Restore(ctx, handle); !errors.Is(err, ErrBadSource) {
		t.Fatalf("error = %v, want ErrBadSource", err)
	}
	if got := read(t, filepath.Join(elsewhere, "theirs.txt")); got != "theirs" {
		t.Fatalf("the link target was disturbed: %q", got)
	}
	if _, err := os.Readlink(work); err != nil {
		t.Fatalf("the symlink did not survive: %v", err)
	}
}

func TestNestingGuardSeesThroughASymlinkedDataDirectory(t *testing.T) {
	base := t.TempDir()
	realHome := filepath.Join(base, "realhome")
	if err := os.MkdirAll(filepath.Join(realHome, "data", "snapshots"), 0o755); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(base, "home")
	mustSymlink(t, realHome, home)

	// The data directory is reached through a symlink, so its spelling and the
	// resolved workset path differ. The guard must still see the nesting.
	e := NewCopy(filepath.Join(home, "data", "snapshots"))
	if _, err := e.Create(context.Background(), "01AAA", []string{realHome}); !errors.Is(err, ErrBadSource) {
		t.Fatalf("error = %v, want ErrBadSource", err)
	}
}

func TestNestingGuardIsNotFooledByADotDotName(t *testing.T) {
	// Real directories, because "/x" is not an absolute path on Windows and
	// the guard compares resolved paths.
	base := t.TempDir()
	work := filepath.Join(base, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}

	// filepath.Rel returns "..foo/snapshots" for this pair. A plain ".."
	// prefix test reads that as an escape and lets the nesting through.
	inside := filepath.Join(work, "..foo", "snapshots")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := checkNesting(inside, work); !errors.Is(err, ErrBadSource) {
		t.Fatalf("error = %v, want ErrBadSource", err)
	}

	elsewhere := filepath.Join(base, "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := checkNesting(filepath.Join(elsewhere, "snapshots"), work); err != nil {
		t.Fatalf("unrelated paths reported as nested: %v", err)
	}
}
