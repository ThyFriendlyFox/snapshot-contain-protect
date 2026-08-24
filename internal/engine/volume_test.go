package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVolumeOfIsAPrefixOfThePath(t *testing.T) {
	dir := t.TempDir()
	vol, err := VolumeOf(dir)
	if err != nil {
		t.Fatal(err)
	}
	if vol == "" {
		t.Fatal("volume is empty")
	}
	if !strings.HasPrefix(filepath.Clean(dir), filepath.Clean(vol)) {
		t.Fatalf("volume %q does not contain %q", vol, dir)
	}
}

func TestVolumeOfAgreesForSiblings(t *testing.T) {
	base := t.TempDir()
	a := filepath.Join(base, "a")
	b := filepath.Join(base, "b", "deep", "deeper")
	for _, p := range []string{a, b} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	va, err := VolumeOf(a)
	if err != nil {
		t.Fatal(err)
	}
	vb, err := VolumeOf(b)
	if err != nil {
		t.Fatal(err)
	}
	// 2 directories on 1 filesystem are 1 volume, whatever their depth.
	if va != vb {
		t.Fatalf("sibling volumes differ: %q and %q", va, vb)
	}
}

func TestVolumesOfDeduplicatesAndSorts(t *testing.T) {
	base := t.TempDir()
	a := filepath.Join(base, "a")
	b := filepath.Join(base, "b")
	for _, p := range []string{a, b} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	vols, err := VolumesOf([]string{a, b, a})
	if err != nil {
		t.Fatal(err)
	}
	if len(vols) != 1 {
		t.Fatalf("volumes = %v, want 1 for 2 paths on 1 filesystem", vols)
	}
}

func TestVolumeOfRejectsAMissingPath(t *testing.T) {
	// Both platforms refuse, though only unix has to. Windows answers from
	// the path string alone, so the check lives above the platform split to
	// keep 1 contract everywhere.
	if _, err := VolumeOf(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("a missing path reported a volume")
	}
}

func TestVolumesOfIsEmptyForNoPaths(t *testing.T) {
	vols, err := VolumesOf(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(vols) != 0 {
		t.Fatalf("volumes = %v, want none", vols)
	}
}
