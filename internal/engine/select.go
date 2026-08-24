package engine

import "fmt"

// Select returns the backend named by id, or the best backend for this host
// when id is "auto". A named backend that cannot run here is an error: a
// silent downgrade would put snapshots on a filesystem the caller did not ask
// for.
func Select(id, root string) (Engine, error) {
	switch id {
	case "auto":
		b := NewBtrfs(root)
		if err := b.Available(); err == nil {
			return b, nil
		}
		c := NewCopy(root)
		if err := c.Available(); err != nil {
			return nil, err
		}
		return c, nil
	case "btrfs":
		return withCheck(NewBtrfs(root))
	case "copy":
		return withCheck(NewCopy(root))
	case "apfs":
		return withCheck(NewAPFS())
	case "vss":
		return withCheck(NewVSS(root))
	default:
		return nil, fmt.Errorf("unknown backend %q: use auto, btrfs, copy, apfs or vss", id)
	}
}

func withCheck(e Engine) (Engine, error) {
	if err := e.Available(); err != nil {
		return nil, err
	}
	return e, nil
}
