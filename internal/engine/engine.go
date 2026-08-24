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
	// Shadows is the VSS backend's own state: the shadow copies this handle
	// owns and where each is mounted. Every other backend leaves it empty.
	Shadows []shadow `json:"shadows,omitempty"`
}

// shadow is 1 Volume Shadow Copy and its mount inside the handle.
type shadow struct {
	ID     string `json:"id"`     // the provider's identifier
	Volume string `json:"volume"` // the volume it copies, such as `C:\`
	Dir    string `json:"dir"`    // the mount's name inside the handle
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
		if err := CheckSource(root, p); err != nil {
			return err
		}
	}
	return nil
}

// CheckSource reports whether one path can be snapshotted into root. The API
// uses it to find the paths a partial safety snapshot can still cover.
func CheckSource(root, p string) error {
	if !filepath.IsAbs(p) {
		return fmt.Errorf("%w: path %q is not absolute", ErrBadSource, p)
	}
	if err := checkSourcePath(p); err != nil {
		return err
	}
	return checkNesting(root, p)
}

// checkSourcePath rejects a path a snapshot cannot read. It must be there.
func checkSourcePath(p string) error {
	fi, err := os.Lstat(p)
	if err != nil {
		return fmt.Errorf("%w: stat %q: %v", ErrBadSource, p, err)
	}
	return checkKind(fi, p)
}

// checkDestination rejects a path a restore must not write over. A path that
// is not there is the normal case: an agent deleted the working set, and the
// restore puts it back.
func checkDestination(p string) error {
	fi, err := os.Lstat(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("%w: stat %q: %v", ErrBadSource, p, err)
	}
	return checkKind(fi, p)
}

// checkKind holds the one rule both directions share. Lstat, not Stat: a
// symlink to a directory passes Stat, but a tree walk does not descend
// through it, so a snapshot of it would be empty and a restore over it would
// delete the link.
func checkKind(fi os.FileInfo, p string) error {
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: path %q is a symlink; declare the directory it points at", ErrBadSource, p)
	}
	if !fi.IsDir() {
		return fmt.Errorf("%w: path %q is not a directory", ErrBadSource, p)
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
	// Compare the paths the filesystem uses. The workset path is already
	// resolved, so a data directory reached through a symlink would otherwise
	// be spelled differently and slip past the check.
	absRoot = resolve(absRoot)
	source = resolve(source)
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
	if rel == "." {
		return true
	}
	// A plain prefix test would read a directory named "..foo" as an escape.
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// resolve follows symlinks as far as the path exists. A path that is not
// there yet resolves through its nearest existing parent, so the comparison
// still uses real directory names.
func resolve(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path
	}
	return filepath.Join(resolve(parent), filepath.Base(path))
}

// mkdirAll creates a handle directory.
func mkdirAll(path string) error { return os.MkdirAll(path, 0o755) }

// subtreeName keeps handles readable while staying collision-free.
func subtreeName(i int, path string) string {
	return fmt.Sprintf("p%d-%s", i, filepath.Base(path))
}
