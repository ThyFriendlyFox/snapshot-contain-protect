# VERIFICATION.md — one command answers "is this repo healthy"

`./verify/verify.sh` runs, in order:

1. Format check — `gofmt -l cmd internal`
2. Vet — `go vet ./...`
3. Build — `go build ./...`
4. Tests — `go test ./...`
5. The live Btrfs gate — `go test -tags btrfs_live ./internal/engine/ -run TestBtrfsLive -v`
6. The unprivileged gate — the engine and API suites, run again as user 65534

## The Btrfs gate

Step 5 runs only when the host has the `btrfs` command. On any other host it
prints "SKIPPED LOUDLY" and states that the Btrfs backend is unproven there.
It never passes silently.

To run it, point it at a writable directory on a Btrfs filesystem:

```sh
export SNAPSHOT_BTRFS_TEST_ROOT=/mnt/btrfs/snapshot-test
./verify/verify.sh
```

The gate creates a subvolume, snapshots it, changes a file, restores it, and
checks that the file came back. It removes what it made.

## The unprivileged gate

Step 6 exists because the daemon runs as a normal user and a working set can
hold a read-only directory. As root every permission check passes, so a whole
class of defect hides. The step compiles the engine and API suites, then runs
them with `setpriv --reuid=65534`.

It runs only when the suite runs as root and `setpriv` is present. As a normal
user it states that steps 1 to 4 already covered the case. As root with no
`setpriv` it skips loudly.

## Rules

- CI runs **the same command** as local. No CI-only logic.
- A gate that can't run in some environment **skips loudly**, never
  passes silently.
- New behavior lands with its gate in the same PR whenever feasible.
- A feature's completion promise (ROADMAP.md) should be backed by a gate
  here whenever it can be — evidence that keeps proving itself beats
  evidence produced once.
- Fixing a flaky or broken gate is always in scope, for any task.
