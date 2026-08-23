package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

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

	if err := os.Symlink("keep.txt", filepath.Join(work, "link")); err != nil {
		t.Fatal(err)
	}
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
