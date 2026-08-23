package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Copy is the portable backend. It snapshots by hardlinking every file into
// the snapshot root, so a snapshot costs inodes and not data.
//
// It exists because the acceptance host is Btrfs but development and CI hosts
// are not. It is not copy-on-write: a writer that opens an existing file and
// writes in place changes the snapshot too, because both names point at one
// inode. Writers that create a new file and rename over the old one — most
// editors and most agents — are safe. Use Btrfs for work that matters.
type Copy struct {
	root string
}

// NewCopy returns a Copy backend that keeps snapshots under root.
func NewCopy(root string) *Copy { return &Copy{root: root} }

func (c *Copy) Name() string { return "copy" }

func (c *Copy) Available() error {
	if err := os.MkdirAll(c.root, 0o755); err != nil {
		return fmt.Errorf("%w: snapshot root %s: %v", ErrUnavailable, c.root, err)
	}
	return nil
}

func (c *Copy) Create(ctx context.Context, id string, sources []string) (string, error) {
	if err := checkSources(c.root, sources); err != nil {
		return "", err
	}
	handle := filepath.Join(c.root, id)
	if err := os.MkdirAll(handle, 0o755); err != nil {
		return "", err
	}

	m := manifest{}
	for i, src := range sources {
		dir := subtreeName(i, src)
		if err := cloneTree(ctx, src, filepath.Join(handle, dir), link); err != nil {
			_ = os.RemoveAll(handle)
			return "", fmt.Errorf("snapshot %s: %w", src, err)
		}
		m.Sources = append(m.Sources, source{Path: src, Dir: dir})
	}
	if err := writeManifest(handle, m); err != nil {
		_ = os.RemoveAll(handle)
		return "", err
	}
	return handle, nil
}

func (c *Copy) Delete(_ context.Context, handle string) error {
	return os.RemoveAll(handle)
}

func (c *Copy) Diff(ctx context.Context, from, to string) (Change, error) {
	return diffHandles(ctx, from, to)
}

func (c *Copy) Restore(ctx context.Context, handle string) error {
	m, err := readManifest(handle)
	if err != nil {
		return err
	}
	for _, s := range m.Sources {
		// Restore copies bytes instead of hardlinking. The restored tree must
		// not share inodes with the snapshot, or the next in-place write would
		// rewrite history.
		if err := swapIn(ctx, filepath.Join(handle, s.Dir), s.Path, copyBytes); err != nil {
			return fmt.Errorf("restore %s: %w", s.Path, err)
		}
	}
	return nil
}

// cloneMode selects how a regular file reaches the destination tree.
type cloneMode int

const (
	link cloneMode = iota
	copyBytes
)

// cloneTree rebuilds src at dst. Sockets, devices and pipes are skipped: a
// working set is files, and an agent cannot roll one of those back anyway.
//
// Directories are made writable first and given their real mode at the end.
// A source tree may hold a read-only directory, and a destination copy of it
// cannot take children.
func cloneTree(ctx context.Context, src, dst string, mode cloneMode) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}

	dirs := []dirMode{{path: dst, mode: srcInfo.Mode()}}
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		fi, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case fi.IsDir():
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			dirs = append(dirs, dirMode{path: target, mode: fi.Mode()})
			return nil
		case fi.Mode()&fs.ModeSymlink != 0:
			dest, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(dest, target)
		case fi.Mode().IsRegular():
			return placeFile(p, target, fi, mode)
		default:
			return nil
		}
	})
	if err != nil {
		return err
	}
	return applyDirModes(dirs)
}

// dirMode remembers a directory's real mode until its children exist.
type dirMode struct {
	path string
	mode fs.FileMode
}

// applyDirModes sets directory modes deepest first, so a read-only parent
// never blocks a child that still needs its own mode.
func applyDirModes(dirs []dirMode) error {
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i].path) > len(dirs[j].path) })
	for _, d := range dirs {
		if err := os.Chmod(d.path, permOf(d.mode)); err != nil {
			return err
		}
	}
	return nil
}

// permOf keeps the permission bits and the setuid, setgid and sticky bits.
// Everything else is the file type, which chmod does not set.
func permOf(m fs.FileMode) fs.FileMode {
	return m & (fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky)
}

func placeFile(src, dst string, fi fs.FileInfo, mode cloneMode) error {
	if mode == link {
		err := os.Link(src, dst)
		if err == nil {
			return nil
		}
		if errors.Is(err, os.ErrExist) {
			return err
		}
		// A cross-device link, or a filesystem link limit, falls back to bytes.
	}
	return writeCopy(src, dst, fi)
}

func writeCopy(src, dst string, fi fs.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	// open(2) applies the umask and drops setuid, setgid and sticky. A restore
	// must return the exact prior file state, so set the mode after the write.
	if err := os.Chmod(dst, permOf(fi.Mode())); err != nil {
		return err
	}
	// The differ compares size and modification time, so the time must survive.
	return os.Chtimes(dst, fi.ModTime(), fi.ModTime())
}

// swapIn builds the tree beside its destination, then moves it into place. A
// failed restore leaves the working set untouched.
func swapIn(ctx context.Context, from, dst string, mode cloneMode) error {
	staged := dst + ".snapshot-restore"
	previous := dst + ".snapshot-previous"
	_ = os.RemoveAll(staged)
	_ = os.RemoveAll(previous)

	if err := cloneTree(ctx, from, staged, mode); err != nil {
		_ = os.RemoveAll(staged)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		_ = os.RemoveAll(staged)
		return err
	}

	existed := true
	if err := os.Rename(dst, previous); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			_ = os.RemoveAll(staged)
			return err
		}
		existed = false
	}
	if err := os.Rename(staged, dst); err != nil {
		if existed {
			_ = os.Rename(previous, dst)
		}
		_ = os.RemoveAll(staged)
		return err
	}
	return os.RemoveAll(previous)
}
