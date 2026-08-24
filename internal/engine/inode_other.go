//go:build !unix

package engine

import "os"

func inodeOf(os.FileInfo) (uint64, bool) { return 0, false }
