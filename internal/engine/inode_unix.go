//go:build unix

package engine

import (
	"os"
	"syscall"
)

// inodeOf reports the inode number so a hardlinked file is counted once.
func inodeOf(fi os.FileInfo) (uint64, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return st.Ino, true
}
