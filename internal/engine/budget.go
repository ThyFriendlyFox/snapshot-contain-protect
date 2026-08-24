package engine

import "context"

// Budgeter is implemented by a backend whose provider caps how many snapshots
// can exist. Retention asks, and keeps the smaller of its own budget and the
// provider's.
//
// It matters because a provider that hits its cap does not refuse the next
// snapshot: it silently deletes the oldest one. Counting to 50 while Windows
// deletes behind us would leave the graph naming snapshots that are gone.
type Budgeter interface {
	// Budget reports how many more snapshots the provider will hold for
	// these paths. ok is false when the provider has no cap worth applying.
	Budget(ctx context.Context, sources []string) (max int, ok bool, err error)
}

// Verifier is implemented by a backend that can be asked whether the snapshot
// behind a handle still exists. A handle directory outliving its snapshot is
// possible on any backend whose storage is owned by somebody else: a VSS
// mount stays a directory after the provider evicts the shadow copy under it.
type Verifier interface {
	// Exists reports whether the snapshot behind the handle is still real.
	Exists(ctx context.Context, handle string) (bool, error)
}
