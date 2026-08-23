package engine

import (
	"context"
	"fmt"
)

// APFS is the macOS backend. It is a stub: the MVP host is Linux on Btrfs.
// The real implementation calls fs_snapshot_create through the Darwin syscall
// interface. See agent-kit/docs/ADAPTERS.md for the contract a backend meets.
type APFS struct{}

func NewAPFS() *APFS { return &APFS{} }

func (a *APFS) Name() string { return "apfs" }

func (a *APFS) Available() error {
	return fmt.Errorf("%w: the apfs backend is not implemented", ErrUnavailable)
}

func (a *APFS) Create(context.Context, string, []string) (string, error) {
	return "", a.Available()
}

func (a *APFS) Delete(context.Context, string) error { return a.Available() }

func (a *APFS) Restore(context.Context, string) error { return a.Available() }

func (a *APFS) Diff(context.Context, string, string) (Change, error) {
	return Change{}, a.Available()
}
