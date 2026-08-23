// Package container holds the optional container layer. START.md section 9
// puts it after step 5, so the MVP ships the seam and not the implementation.
//
// When it lands, the runtime freezes the process tree with CRIU, keeps the
// checkpoint identifier next to the filesystem snapshot, and discards the
// container layer instead of restoring the host. That is the only path that
// rolls back processes and sockets. See START.md section 7.
package container

import (
	"context"
	"errors"
)

// ErrNotImplemented means the container layer is not built yet.
var ErrNotImplemented = errors.New("the container layer is not implemented in this build")

// Runtime freezes, restores and discards a container checkpoint.
type Runtime interface {
	// Freeze checkpoints the process tree and returns the checkpoint ID.
	Freeze(ctx context.Context, container string) (string, error)
	// Restore returns the process tree to a checkpoint.
	Restore(ctx context.Context, checkpoint string) error
	// Discard drops a checkpoint and its layer.
	Discard(ctx context.Context, checkpoint string) error
}

// Unavailable is the runtime this build ships. Every call refuses.
type Unavailable struct{}

func (Unavailable) Freeze(context.Context, string) (string, error) { return "", ErrNotImplemented }
func (Unavailable) Restore(context.Context, string) error          { return ErrNotImplemented }
func (Unavailable) Discard(context.Context, string) error          { return ErrNotImplemented }
