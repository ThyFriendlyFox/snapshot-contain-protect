package engine

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// entry is the comparable state of one path. Contents are never read: a diff
// answers "what moved", not "what changed inside".
type entry struct {
	mode    fs.FileMode
	size    int64
	modNsec int64
	link    string // symlink target, empty otherwise
}

func (e entry) equal(o entry) bool {
	if e.mode.Type() != o.mode.Type() {
		return false
	}
	if e.mode&fs.ModeSymlink != 0 {
		return e.link == o.link
	}
	if e.mode.IsDir() {
		return true
	}
	return e.size == o.size && e.modNsec == o.modNsec
}

// diffHandles compares two handles path by path. It is filesystem-agnostic, so
// every backend shares it.
func diffHandles(ctx context.Context, from, to string) (Change, error) {
	fromM, err := readManifest(from)
	if err != nil {
		return Change{}, err
	}
	toM, err := readManifest(to)
	if err != nil {
		return Change{}, err
	}

	var c Change
	seen := map[string]bool{}

	for _, s := range toM.Sources {
		seen[s.Path] = true
		old, ok := findSource(fromM, s.Path)
		if !ok {
			// The path joined the working set. Everything under it is new.
			added, err := listTree(ctx, filepath.Join(to, s.Dir), s.Path)
			if err != nil {
				return Change{}, err
			}
			c.Added = append(c.Added, added...)
			continue
		}
		if err := diffTree(ctx, filepath.Join(from, old.Dir), filepath.Join(to, s.Dir), s.Path, &c); err != nil {
			return Change{}, err
		}
	}

	for _, s := range fromM.Sources {
		if seen[s.Path] {
			continue
		}
		// The path left the working set. Everything under it is gone.
		deleted, err := listTree(ctx, filepath.Join(from, s.Dir), s.Path)
		if err != nil {
			return Change{}, err
		}
		c.Deleted = append(c.Deleted, deleted...)
	}

	c.Added, c.Truncated = capPaths(c.Added, c.Truncated)
	c.Modified, c.Truncated = capPaths(c.Modified, c.Truncated)
	c.Deleted, c.Truncated = capPaths(c.Deleted, c.Truncated)
	return c, nil
}

func findSource(m manifest, path string) (source, bool) {
	for _, s := range m.Sources {
		if s.Path == path {
			return s, true
		}
	}
	return source{}, false
}

func diffTree(ctx context.Context, fromDir, toDir, base string, c *Change) error {
	fromSet, err := scanTree(ctx, fromDir)
	if err != nil {
		return err
	}
	toSet, err := scanTree(ctx, toDir)
	if err != nil {
		return err
	}
	for rel, te := range toSet {
		fe, ok := fromSet[rel]
		switch {
		case !ok:
			c.Added = append(c.Added, filepath.Join(base, rel))
		case !fe.equal(te):
			c.Modified = append(c.Modified, filepath.Join(base, rel))
		}
	}
	for rel := range fromSet {
		if _, ok := toSet[rel]; !ok {
			c.Deleted = append(c.Deleted, filepath.Join(base, rel))
		}
	}
	return nil
}

// scanTree records every path under dir except the directory itself.
func scanTree(ctx context.Context, dir string) (map[string]entry, error) {
	out := map[string]entry{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		e, err := statEntry(p, d)
		if err != nil {
			return err
		}
		out[rel] = e
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", dir, err)
	}
	return out, nil
}

func statEntry(p string, d fs.DirEntry) (entry, error) {
	fi, err := d.Info()
	if err != nil {
		return entry{}, err
	}
	e := entry{mode: fi.Mode(), size: fi.Size(), modNsec: fi.ModTime().UnixNano()}
	if fi.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(p)
		if err != nil {
			return entry{}, err
		}
		e.link = target
	}
	return e, nil
}

func listTree(ctx context.Context, dir, base string) ([]string, error) {
	set, err := scanTree(ctx, dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(set))
	for rel := range set {
		out = append(out, filepath.Join(base, rel))
	}
	return out, nil
}

// capPaths sorts for a stable answer and enforces DiffCap.
func capPaths(in []string, truncated bool) ([]string, bool) {
	sort.Strings(in)
	if len(in) > DiffCap {
		return in[:DiffCap], true
	}
	if in == nil {
		return []string{}, truncated
	}
	return in, truncated
}
