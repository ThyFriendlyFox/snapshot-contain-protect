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
	if err := checkSources(sources); err != nil {
		return "", err
	}
	handle := filepath.Join(b.root, id)
	if err := os.MkdirAll(handle, 0o755); err != nil {
		return "", err
	}

	m := manifest{}
	for i, src := range sources {
		dir := subtreeName(i, src)
		dest := filepath.Join(handle, dir)
		if _, err := b.run(ctx, "btrfs", "subvolume", "snapshot", "-r", src, dest); err != nil {
			b.cleanup(ctx, handle, m)
			return "", err
		}
		m.Sources = append(m.Sources, source{Path: src, Dir: dir})
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
	for _, s := range m.Sources {
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
		src := filepath.Join(handle, s.Dir)
		staged := s.Path + ".snapshot-restore"
		_ = os.RemoveAll(staged)

		if _, err := b.run(ctx, "btrfs", "subvolume", "snapshot", src, staged); err != nil {
			return fmt.Errorf("restore %s: %w", s.Path, err)
		}
		if _, err := b.run(ctx, "btrfs", "subvolume", "delete", s.Path); err != nil {
			_, _ = b.run(ctx, "btrfs", "subvolume", "delete", staged)
			return fmt.Errorf("restore %s: %w", s.Path, err)
		}
		if err := os.Rename(staged, s.Path); err != nil {
			return fmt.Errorf("restore %s: rename staged subvolume: %w", s.Path, err)
		}
	}
	return nil
}

func (b *Btrfs) cleanup(ctx context.Context, handle string, m manifest) {
	for _, s := range m.Sources {
		_, _ = b.run(ctx, "btrfs", "subvolume", "delete", filepath.Join(handle, s.Dir))
	}
	_ = os.RemoveAll(handle)
}
