package engine

import (
	"fmt"
	"os"
	"sort"
)

// VolumeOf reports the mount root of the volume that holds path: `C:\` on
// Windows, `/` or the nearest mount point on unix.
//
// It exists because VSS is volume-scoped. A shadow copy covers a whole
// volume, not a directory, so the Windows backend must know which volumes a
// workset touches before it can snapshot one. Btrfs does not care, and for
// that backend this is reporting only.
// The path must exist. Windows would answer without it — GetVolumePathNameW
// parses the string and never touches the disk — while the unix walk needs a
// device number to compare. Rather than let the 2 platforms disagree, both
// require the path, and the answer is a fact instead of a prediction.
func VolumeOf(path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("volume of %q: %w", path, err)
	}
	return volumeOf(path)
}

// VolumesOf returns the distinct volumes a set of paths covers, sorted. A
// workset spanning 2 volumes needs 2 shadow copies, which is why the count
// matters to the caller and not just to the backend.
func VolumesOf(paths []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, 1)
	for _, p := range paths {
		v, err := VolumeOf(p)
		if err != nil {
			return nil, err
		}
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out, nil
}
