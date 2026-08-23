package engine

import (
	"context"
	"fmt"
)

// VSS is the Windows backend. It is a stub: the MVP host is Linux on Btrfs.
// The real implementation calls the Volume Shadow Copy Service API directly.
// It never calls System Restore, which is minutes slow and records registry
// state a rollback does not need.
type VSS struct{}

func NewVSS() *VSS { return &VSS{} }

func (v *VSS) Name() string { return "vss" }

func (v *VSS) Available() error {
	return fmt.Errorf("%w: the vss backend is not implemented", ErrUnavailable)
}

func (v *VSS) Create(context.Context, string, []string) (string, error) {
	return "", v.Available()
}

func (v *VSS) Delete(context.Context, string) error { return v.Available() }

func (v *VSS) Restore(context.Context, string) error { return v.Available() }

func (v *VSS) Diff(context.Context, string, string) (Change, error) {
	return Change{}, v.Available()
}
