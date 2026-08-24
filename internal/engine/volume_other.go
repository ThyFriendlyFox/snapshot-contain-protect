//go:build !unix && !windows

package engine

import "path/filepath"

// volumeOf has no way to ask this platform, so it answers with the root.
func volumeOf(string) (string, error) { return string(filepath.Separator), nil }
