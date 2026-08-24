package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runner executes a command. The Btrfs backend routes every shell-out through
// it so tests can record the commands without a Btrfs host.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

func execRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// Btrfs snapshots subvolumes. Creation is copy-on-write and copies no data.
// Every source path in the workset must be its own subvolume.
type Btrfs struct {
	root string
	run  Runner
}

// NewBtrfs returns a Btrfs backend that keeps snapshots under root. The root
// must be on the same Btrfs filesystem as the working set.
func NewBtrfs(root string) *Btrfs { return &Btrfs{root: root, run: execRunner} }

func (b *Btrfs) Name() string { return "btrfs" }

func (b *Btrfs) Available() error {
	if _, err := exec.LookPath("btrfs"); err != nil {
		return fmt.Errorf("%w: btrfs command not found", ErrUnavailable)
	}
	if err := os.MkdirAll(b.root, 0o755); err != nil {
		return fmt.Errorf("%w: snapshot root %s: %v", ErrUnavailable, b.root, err)
	}
	if _, err := b.run(context.Background(), "btrfs", "subvolume", "show", b.root); err != nil {
		// The root itself need not be a subvolume, but its filesystem must be
		// Btrfs. `filesystem show` fails on any other filesystem.
		if _, ferr := b.run(context.Background(), "btrfs", "filesystem", "df", b.root); ferr != nil {
			return fmt.Errorf("%w: %s is not on a btrfs filesystem", ErrUnavailable, b.root)
		}
	}
	return nil
}

func (b *Btrfs) Create(ctx context.Context, id string, sources []string) (string, error) {
	if err := checkSources(b.root, sources); err != nil {
		return "", err
	}
	handle := filepath.Join(b.root, id)
	if err := os.MkdirAll(handle, 0o755); err != nil {
		return "", err
	}

	m := manifest{}
	for i, src := range sources {
		// btrfs snapshots a subvolume, never a plain directory. A directory
		// inside one is snapshotted through its subvolume and addressed as a
		// subpath, the way the VSS backend addresses a path inside a volume.
		subvol, err := b.enclosingSubvolume(ctx, src)
		if err != nil {
			b.cleanup(ctx, handle, m)
			return "", err
		}
		dir := subtreeName(i, subvol)
		dest := filepath.Join(handle, dir)
		if _, err := b.run(ctx, "btrfs", "subvolume", "snapshot", "-r", subvol, dest); err != nil {
			b.cleanup(ctx, handle, m)
			return "", err
		}

		entry := source{Path: src, Dir: dir, Swappable: subvol == src}
		if !entry.Swappable {
			rel, err := filepath.Rel(subvol, src)
			if err != nil {
				b.cleanup(ctx, handle, m)
				return "", fmt.Errorf("%w: %q is not inside %q", ErrBadSource, src, subvol)
			}
			entry.Dir = filepath.Join(dir, rel)
		}
		m.Sources = append(m.Sources, entry)
		m.Shadows = append(m.Shadows, shadow{Volume: subvol, Dir: dir})
	}
	if err := writeManifest(handle, m); err != nil {
		b.cleanup(ctx, handle, m)
		return "", err
	}
	return handle, nil
}

func (b *Btrfs) Delete(ctx context.Context, handle string) error {
	m, err := readManifest(handle)
	if err != nil {
		// A handle without a manifest is already broken. Remove what is there.
		return os.RemoveAll(handle)
	}
	// Shadows names 1 entry per snapshot taken. Sources can name several
	// paths inside 1 of them, so deleting per source would delete twice.
	for _, s := range m.Shadows {
		if _, err := b.run(ctx, "btrfs", "subvolume", "delete", filepath.Join(handle, s.Dir)); err != nil {
			return err
		}
	}
	return os.RemoveAll(handle)
}

func (b *Btrfs) Diff(ctx context.Context, from, to string) (Change, error) {
	// `btrfs send --no-data` is faster on large sets but needs both snapshots
	// to share a parent and needs root. The tree walk answers the same
	// question on any pair. ROADMAP item 3 replaces this.
	return diffHandles(ctx, from, to)
}

// Restore replaces each live subvolume with a writable snapshot of the stored
// read-only one. The live subvolume is deleted, so open file descriptors in
// running processes keep pointing at the old data. Restart the process tree
// after a restore.
func (b *Btrfs) Restore(ctx context.Context, handle string) error {
	m, err := readManifest(handle)
	if err != nil {
		return err
	}
	for _, s := range m.Sources {
		if !s.Swappable {
			// The snapshot holds more than the caller declared. Swapping the
			// subvolume would restore every sibling they never named, so
			// copy just the declared path back out.
			if err := checkDestination(s.Path); err != nil {
				return err
			}
			if err := swapIn(ctx, filepath.Join(handle, s.Dir), s.Path, copyBytes); err != nil {
				return fmt.Errorf("restore %s: %w", s.Path, err)
			}
			continue
		}
		src := filepath.Join(handle, s.Dir)
		staged := s.Path + ".snapshot-restore"

		// A staged subvolume left by an interrupted restore is not a
		// directory: rmdir refuses it, so only `subvolume delete` clears it.
		// Without this, every later restore of this path fails on "File
		// exists".
		b.clearStaged(ctx, staged)

		if _, err := b.run(ctx, "btrfs", "subvolume", "snapshot", src, staged); err != nil {
			return fmt.Errorf("restore %s: %w", s.Path, err)
		}
		if _, err := b.run(ctx, "btrfs", "subvolume", "delete", s.Path); err != nil {
			b.clearStaged(ctx, staged)
			return fmt.Errorf("restore %s: %w", s.Path, err)
		}
		if err := os.Rename(staged, s.Path); err != nil {
			// The live subvolume is already gone. Put the snapshot back at the
			// declared path directly, so the working set is not left missing.
			if _, rerr := b.run(ctx, "btrfs", "subvolume", "snapshot", src, s.Path); rerr != nil {
				return fmt.Errorf("restore %s: rename staged subvolume: %w; the path is now missing and %s holds the data: %v",
					s.Path, err, staged, rerr)
			}
			b.clearStaged(ctx, staged)
		}
	}
	return nil
}

// clearStaged removes a staged subvolume, or a plain directory if that is
// what is there. Both failures are ignored: the caller reports the operation
// that matters.
func (b *Btrfs) clearStaged(ctx context.Context, staged string) {
	if _, err := os.Lstat(staged); err != nil {
		return
	}
	if _, err := b.run(ctx, "btrfs", "subvolume", "delete", staged); err == nil {
		return
	}
	_ = os.RemoveAll(staged)
}

func (b *Btrfs) cleanup(ctx context.Context, handle string, m manifest) {
	for _, s := range m.Shadows {
		_, _ = b.run(ctx, "btrfs", "subvolume", "delete", filepath.Join(handle, s.Dir))
	}
	_ = os.RemoveAll(handle)
}

// enclosingSubvolume returns path itself when it is a subvolume, or the
// nearest ancestor that is one. A btrfs filesystem always has a subvolume at
// its root, so the walk terminates there.
func (b *Btrfs) enclosingSubvolume(ctx context.Context, path string) (string, error) {
	current := filepath.Clean(path)
	for {
		if _, err := b.run(ctx, "btrfs", "subvolume", "show", current); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("%w: no btrfs subvolume contains %q", ErrBadSource, path)
		}
		current = parent
	}
}
