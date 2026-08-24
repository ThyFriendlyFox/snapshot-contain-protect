//go:build unix

package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// volumeOf walks up until the device number changes. That boundary is the
// mount point, which is the unix answer to "which volume holds this".
func volumeOf(path string) (string, error) {
	current := filepath.Clean(path)
	fi, err := os.Stat(current)
	if err != nil {
		return "", fmt.Errorf("volume of %q: %w", path, err)
	}
	dev, ok := deviceOf(fi)
	if !ok {
		return string(filepath.Separator), nil
	}

	for {
		parent := filepath.Dir(current)
		if parent == current {
			return current, nil
		}
		pfi, err := os.Stat(parent)
		if err != nil {
			// The walk stopped at something unreadable. The last directory
			// that answered is the best available answer.
			return current, nil
		}
		pdev, ok := deviceOf(pfi)
		if !ok || pdev != dev {
			return current, nil
		}
		current = parent
	}
}

func deviceOf(fi os.FileInfo) (uint64, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Dev), true
}
