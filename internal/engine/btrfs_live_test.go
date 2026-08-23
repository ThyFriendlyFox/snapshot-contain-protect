//go:build btrfs_live

// The live Btrfs gate. It runs only with `-tags btrfs_live` on a host that has
// a Btrfs filesystem and the rights to make subvolumes. verify/verify.sh runs
// it when the btrfs command exists and skips loudly when it does not.
package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBtrfsLive(t *testing.T) {
	root := os.Getenv("SNAPSHOT_BTRFS_TEST_ROOT")
	if root == "" {
		t.Fatal("set SNAPSHOT_BTRFS_TEST_ROOT to a writable directory on a btrfs filesystem")
	}
	ctx := context.Background()

	b := NewBtrfs(filepath.Join(root, "snapshots"))
	if err := b.Available(); err != nil {
		t.Fatalf("btrfs backend unavailable: %v", err)
	}

	work := filepath.Join(root, "work")
	_ = os.RemoveAll(work)
	if _, err := b.run(ctx, "btrfs", "subvolume", "create", work); err != nil {
		t.Fatalf("create test subvolume: %v", err)
	}
	t.Cleanup(func() { _, _ = b.run(ctx, "btrfs", "subvolume", "delete", work) })

	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	handle, err := b.Create(ctx, "01LIVE", []string{work})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Delete(ctx, handle) })

	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(ctx, handle); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(work, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "first" {
		t.Fatalf("after restore a.txt = %q, want %q", got, "first")
	}
}
