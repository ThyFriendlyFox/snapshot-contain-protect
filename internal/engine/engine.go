// Package engine wraps the filesystem snapshot primitive. One interface, one
// backend per filesystem. The daemon never shells out; the backend does.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DiffCap is the maximum number of paths reported per category.
const DiffCap = 500

// ErrUnavailable means the backend cannot run on this host.
var ErrUnavailable = errors.New("backend unavailable on this host")

// Change is a path-level summary of the difference between two snapshots.
type Change struct {
	Added     []string `json:"added"`
	Modified  []string `json:"modified"`
	Deleted   []string `json:"deleted"`
	Truncated bool     `json:"truncated"`
}

// Engine is the seam. A backend implements these four operations and nothing
// else. Handles are opaque to every caller above this package.
type Engine interface {
	// Name is the backend identifier stored in the snapshots table.
	Name() string
	// Available reports whether this backend can run here.
	Available() error
	// Create snapshots every source path and returns the handle.
	Create(ctx context.Context, id string, sources []string) (string, error)
	// Delete removes the snapshot behind the handle.
	Delete(ctx context.Context, handle string) error
	// Restore returns every source path to the state in the handle.
	Restore(ctx context.Context, handle string) error
	// Diff summarises the difference between two handles.
	Diff(ctx context.Context, from, to string) (Change, error)
}

// manifest records which source path each subtree of a handle came from. It
// lives inside the handle so a restore works after a daemon restart.
type manifest struct {
	Sources []source `json:"sources"`
}

type source struct {
	Path string `json:"path"` // absolute source path
	Dir  string `json:"dir"`  // subtree name inside the handle
}

const manifestName = "manifest.json"

func writeManifest(handle string, m manifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(handle, manifestName), b, 0o644)
}

func readManifest(handle string) (manifest, error) {
	var m manifest
	b, err := os.ReadFile(filepath.Join(handle, manifestName))
	if err != nil {
		return m, fmt.Errorf("read manifest: %w", err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("parse manifest: %w", err)
	}
	return m, nil
}

// checkSources rejects a working set the backend cannot snapshot.
func checkSources(sources []string) error {
	if len(sources) == 0 {
		return errors.New("workset has no paths")
	}
	for _, p := range sources {
		if !filepath.IsAbs(p) {
			return fmt.Errorf("path %q is not absolute", p)
		}
		fi, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("stat %q: %w", p, err)
		}
		if !fi.IsDir() {
			return fmt.Errorf("path %q is not a directory", p)
		}
	}
	return nil
}

// subtreeName keeps handles readable while staying collision-free.
func subtreeName(i int, path string) string {
	return fmt.Sprintf("p%d-%s", i, filepath.Base(path))
}
