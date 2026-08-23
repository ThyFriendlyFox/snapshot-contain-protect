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
	"strings"
)

// DiffCap is the maximum number of paths reported per category.
const DiffCap = 500

// ErrUnavailable means the backend cannot run on this host.
var ErrUnavailable = errors.New("backend unavailable on this host")

// ErrBadSource means the working set itself is wrong: a missing path, a
// relative path, a symlink, or a path that contains the snapshot root.
var ErrBadSource = errors.New("workset path cannot be snapshotted")

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

// checkSources rejects a working set the backend cannot snapshot. Every
// failure here is the caller's input, so it wraps ErrBadSource and the API
// answers 400.
func checkSources(root string, sources []string) error {
	if len(sources) == 0 {
		return fmt.Errorf("%w: workset has no paths", ErrBadSource)
	}
	for _, p := range sources {
		if !filepath.IsAbs(p) {
			return fmt.Errorf("%w: path %q is not absolute", ErrBadSource, p)
		}
		// Lstat, not Stat. A symlink to a directory passes Stat, but a tree
		// walk does not descend through it: the snapshot would be empty and
		// the restore would replace the link with an empty directory.
		fi, err := os.Lstat(p)
		if err != nil {
			return fmt.Errorf("%w: stat %q: %v", ErrBadSource, p, err)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: path %q is a symlink; declare the directory it points at", ErrBadSource, p)
		}
		if !fi.IsDir() {
			return fmt.Errorf("%w: path %q is not a directory", ErrBadSource, p)
		}
		if err := checkNesting(root, p); err != nil {
			return err
		}
	}
	return nil
}

// checkNesting refuses a working set that contains the snapshot root. Without
// this, a snapshot of a parent of the data directory walks into the handle it
// is writing and recurses until the path is too long.
func checkNesting(root, source string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if within(absRoot, source) {
		return fmt.Errorf("%w: path %q contains the snapshot root %s; a snapshot of it would contain itself",
			ErrBadSource, source, absRoot)
	}
	if within(source, absRoot) {
		return fmt.Errorf("%w: path %q is inside the snapshot root %s", ErrBadSource, source, absRoot)
	}
	return nil
}

// within reports whether child is dir itself or under it.
func within(dir, child string) bool {
	rel, err := filepath.Rel(filepath.Clean(child), filepath.Clean(dir))
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}

// subtreeName keeps handles readable while staying collision-free.
func subtreeName(i int, path string) string {
	return fmt.Sprintf("p%d-%s", i, filepath.Base(path))
}
