//go:build vss_live

// The live VSS gate. It runs only with `-tags vss_live` on an elevated
// Windows host. It makes a real shadow copy, reads the pre-snapshot contents
// back through the mount, and deletes both.
package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVSSLive(t *testing.T) {
	root := os.Getenv("SNAPSHOT_VSS_TEST_ROOT")
	if root == "" {
		t.Fatal("set SNAPSHOT_VSS_TEST_ROOT to a writable directory on the volume to snapshot")
	}
	ctx := context.Background()

	v := NewVSS(filepath.Join(root, "snapshots"))
	if err := v.Available(); err != nil {
		t.Fatalf("vss backend unavailable: %v", err)
	}

	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(work, "marker.txt")
	if err := os.WriteFile(marker, []byte("before the snapshot"), 0o644); err != nil {
		t.Fatal(err)
	}

	handle, err := v.Create(ctx, "01VSSLIVE", []string{work})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = v.Delete(ctx, handle) })

	if err := os.WriteFile(marker, []byte("after the snapshot"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The snapshot must hold what the file said before the change, read
	// through the mount the backend made.
	m, err := readManifest(handle)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Sources) != 1 {
		t.Fatalf("manifest sources = %+v, want 1", m.Sources)
	}
	stored := filepath.Join(handle, m.Sources[0].Dir, "marker.txt")
	body, err := os.ReadFile(stored)
	if err != nil {
		t.Fatalf("read %s through the mount: %v", stored, err)
	}
	if string(body) != "before the snapshot" {
		t.Fatalf("the snapshot holds %q, want %q", body, "before the snapshot")
	}

	// A diff of the snapshot against a second one must name the changed file.
	second, err := v.Create(ctx, "01VSSLIVE2", []string{work})
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	t.Cleanup(func() { _ = v.Delete(ctx, second) })

	change, err := v.Diff(ctx, handle, second)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(change.Modified) != 1 || filepath.Base(change.Modified[0]) != "marker.txt" {
		t.Fatalf("diff = %+v, want marker.txt modified", change)
	}

	// A restore must put the working set back to what the first snapshot
	// holds, copying out of the mount.
	extra := filepath.Join(work, "junk.txt")
	if err := os.WriteFile(extra, []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := v.Restore(ctx, handle); err != nil {
		t.Fatalf("restore: %v", err)
	}
	restored, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != "before the snapshot" {
		t.Fatalf("after restore marker.txt = %q, want %q", restored, "before the snapshot")
	}
	if _, err := os.Stat(extra); !os.IsNotExist(err) {
		t.Fatal("junk.txt survived the restore")
	}

	if err := v.Delete(ctx, second); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(second); !os.IsNotExist(err) {
		t.Fatal("the handle survived delete")
	}
}
