# ROADMAP.md — the source of all work

**This file is not optional.** Every feature the agent builds flows down
from here. If it isn't on this roadmap, it doesn't get built; if it needs
building, it gets added here first. One item ships per weekly cycle
(see `WEEKLY.md`).

## North star

Snapshot is becoming the undo a computer-use agent can call. A code agent
undoes its work because Git exists; a computer-use agent has nothing. The
project ends when an agent on Linux, macOS or Windows can mark a filesystem
state in under 1 second, read what changed, and return to that state, all
through 5 local HTTP verbs. It stays small: no user interface, no network
snapshots, no rollback of state that lives somewhere else.

## Feature Queue — ordered; top unblocked item ships next

`provisional: true` — the human has not ranked this queue. The order below
follows START.md section 9 and the gaps the MVP left.

### 1. Prove the Btrfs backend on a Btrfs host

- **Promise:** On a Btrfs host, `go test -tags btrfs_live ./internal/engine/`
  passes, and the acceptance run in START.md section 10 records a snapshot
  under 1 second and a diff under 2 seconds on a 5 GB working set.
- **Evidence:** The live gate output and the acceptance numbers in
  `DEVLOG.md`, plus a CI job on a Btrfs loopback image.
- **Use case:** "Undo a file operation" — the case is only proven on the
  filesystem the MVP targets.
- **Scope guard:** No macOS and no Windows work. No new verbs.
- **Status:** ready

### 2. Diff through `btrfs send --no-data`

- **Promise:** A diff of two Btrfs snapshots that differ by 1 file in a
  working set of 100000 files returns in under 2 seconds, and returns the
  same paths as the tree walk.
- **Evidence:** A benchmark in `internal/engine/` that runs both differs over
  the same pair and compares the output.
- **Use case:** "Decide whether to roll back" — the diff must stay cheaper
  than the rollback it prevents.
- **Scope guard:** The tree walk stays as the fallback for backends without
  a native diff. No content-level diff.
- **Status:** ready

### 3. A workset that covers a plain directory on Btrfs

- **Promise:** A workset path that is a directory and not a subvolume is
  snapshotted, and `snapctl workset` states which paths it converted.
- **Evidence:** A live gate case that declares a plain directory and restores
  it.
- **Use case:** "Checkpoint every step of a long task" — an agent points at
  a project directory, not at a subvolume it must create first.
- **Scope guard:** No automatic conversion of a directory a person did not
  name.
- **Status:** ready

### 4. Snapshot the daemon's own crash recovery

- **Promise:** A daemon killed during `Create` leaves no handle without a
  row and no row without a handle, and says at start how many it repaired.
- **Evidence:** A test that kills the service between the engine call and
  the store write, then reopens it.
- **Use case:** "Recover the graph after a restart".
- **Scope guard:** No write-ahead journal of its own; the repair pass reads
  the snapshot root and the table.
- **Status:** ready

## Later — candidates, not yet specced

- APFS backend — the macOS half of the north star. Near-instant snapshots.
- VSS backend — the Windows half. 1 to 2 seconds, and never System Restore.
- Container layer with CRIU — START.md section 6. The only path that rolls
  back processes and sockets.
- A `GET /snapshots/<id>/paths` verb — an agent that wants the workset paths
  of one node without reading the workset.
- Retention by age and by disk budget, not only by count.

## Shipped

| Week | Feature | Release | Evidence |
|---|---|---|---|
| 2026-08-23 | MVP: engine, store, daemon, client, retention | unreleased | `./verify/verify.sh` green at `HEAD`; 53 tests, 0 failures |

## Explicitly not doing

- A graphical interface. The caller is an agent; the human uses `snapctl`.
- Remote or networked snapshots. The daemon binds loopback and has no
  authentication.
- Rollback of remote state. A written row, a sent email and a called API
  stay done. START.md section 7 states this and it never changes.
- Windows System Restore as the Windows backend. It takes minutes and
  records state a rollback does not need.

## Queue changes

- 2026-08-23 — Seeded the queue at install. Order follows START.md section 9:
  the Btrfs proof first, because the MVP gate skips it on a non-Btrfs host.
