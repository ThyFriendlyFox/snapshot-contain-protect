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

## Release ladder

Each release has 1 promise. A release ships when a gate proves its promise,
per `VERIFICATION.md`. Versions follow `RELEASING.md`: before 1.0, a
breaking change is a minor and everything else is a patch.

| Release | Theme | Promise | State |
|---|---|---|---|
| v0.1.0 | A person on Btrfs can use it | A person installs a release binary on a Btrfs host, declares a workset, and snapshots, diffs and restores. The acceptance numbers from START.md section 10 are recorded. | next |
| v0.2.0 | Fast and bounded | A diff of 2 snapshots that differ by 1 file in a set of 100000 files returns in under 2 seconds, and disk use stays under a declared budget. | planned |
| v0.3.0 | Built for agents | An agent adds Snapshot with 1 MCP configuration block and gets a checkpoint before each of its actions. | planned |
| v0.4.0 | macOS | The same 5 verbs, the same gate, on APFS. A snapshot completes in under 1 second. | planned |
| v0.5.0 | Windows | The same 5 verbs, the same gate, on VSS. A snapshot completes in under 2 seconds, and System Restore is never called. | planned |
| v0.6.0 | Total rollback | An agent inside a container restores, and its process tree resumes at the checkpoint. | planned |
| v1.0.0 | Stable | The 5 verbs carry a compatibility guarantee. 3 platforms are green in CI. An upgrade path is documented. | planned |

### What each release holds

**v0.1.0 — a working product people can use.** Today the code is gated but
unreachable: the Btrfs backend has never run on Btrfs, no binary is
published, and a user must create subvolumes by hand. This release closes
the distance between "the tests pass" and "somebody installed it".

- Prove the Btrfs backend on a Btrfs host, and run the acceptance test.
- Accept a plain directory as a workset path.
- Reconcile the graph and the snapshot root at start.
- Publish binaries, a service unit, install instructions and a license.
- Make CI run for real, and require it.

**v0.2.0 — fast and bounded.** The MVP is cheap to write and expensive to
read at scale, and its only disk limit is a snapshot count.

- Diff through `btrfs send --no-data`, with the tree walk as the fallback.
- Retention by age and disk budget, not only by count.
- `GET /stats` and `snapctl status`: disk used and snapshot count per workset.
- Metrics an operator can scrape.

**v0.3.0 — built for agents.** The API is callable by an agent today, but
nothing makes it the obvious choice. This release is the distribution work.

- An MCP server exposing the 5 verbs, so an agent gets them natively.
- Client libraries for Go and Python.
- Diff a snapshot against the live working set, so an agent can preview a
  restore without taking a snapshot first.
- Labels and search over the graph.

**v0.4.0 and v0.5.0 — the other 2 platforms.** The engine interface exists
for this. Each is 1 backend plus its gate, and no change above the seam.

**v0.6.0 — total rollback.** The container layer with CRIU. It is the only
path that rolls back processes and sockets. START.md puts it after step 5
and says to use the tool for a week first.

**v1.0.0 — stable.** No new capability. The API freezes, the 3 platforms
run in 1 CI matrix, and the upgrade path is written down.

## Feature Queue — ordered; top unblocked item ships next

<!-- RULES:
     · Always ≥3 ready items. Refilling the queue is part of every weekly
       cycle (WEEKLY.md step 7) — a starving queue is a failed cycle.
     · Order is priority. The agent takes the TOP unblocked item and may
       not reorder without recording why (below, under "Queue changes").
     · Every item carries a completion promise: ONE testable sentence
       that is unambiguously true or false. No promise, not ready.
     · "Evidence" names how the promise will be proven: which gate,
       screenshot, benchmark, or user-visible behavior. -->

`provisional: true` — the human has not ranked this queue. The 5 items below
are the whole of v0.1.0, in the order they unblock each other.

### 1. Prove the Btrfs backend on a Btrfs host

- **Promise:** On a Btrfs host, `go test -tags btrfs_live ./internal/engine/`
  passes, and the acceptance run in START.md section 10 records a snapshot
  under 1 second and a diff under 2 seconds on a 5 GB working set.
- **Evidence:** The live gate output and the acceptance numbers in
  `DEVLOG.md`, plus a CI job on a Btrfs loopback image.
- **Use case:** "Undo a file operation" — the case is only proven on the
  filesystem the MVP targets.
- **Scope guard:** No macOS and no Windows work. No new verbs.
- **Release:** v0.1.0
- **Status:** in progress (week of 2026-08-23) — mechanised, not yet run.
  The live gate and the acceptance test are both wired into CI on a
  loopback btrfs image. Neither has executed: no CI run exists yet, which
  is item 5. This item cannot finish before item 5 does.

### 2. A workset that covers a plain directory on Btrfs

- **Promise:** A workset path that is a directory and not a subvolume is
  snapshotted, and `snapctl workset` states which paths it converted.
- **Evidence:** A live gate case that declares a plain directory and restores
  it.
- **Use case:** "Checkpoint every step of a long task" — an agent points at
  a project directory, not at a subvolume it must create first.
- **Scope guard:** No automatic conversion of a directory a person did not
  name.
- **Release:** v0.1.0
- **Status:** ready

### 3. Reconcile the graph and the snapshot root at start

- **Promise:** A daemon killed during `Create` leaves no handle without a
  row and no row without a handle, and says at start how many it repaired.
- **Evidence:** A test that kills the service between the engine call and
  the store write, then reopens it.
- **Use case:** "Recover the graph after a restart".
- **Scope guard:** No write-ahead journal of its own; the repair pass reads
  the snapshot root and the table.
- **Release:** v0.1.0
- **Status:** ready

### 4. Ship an installable release

- **Promise:** `snapshotd` and `snapctl` install from a published GitHub
  release on a clean Linux machine, the service unit starts the daemon, and
  `snapctl snapshot` works without the repository present.
- **Evidence:** A run of the published v0.1.0 artifact on a clean machine,
  recorded in `DEVLOG.md`. `RELEASING.md` step 6 requires this anyway.
- **Use case:** Serves every case in `docs/USE-CASES.md`. None of them is
  reachable without an install.
- **Scope guard:** Binaries, a systemd user unit, install text and a
  license. No distribution packages, no Homebrew, no container image.
- **Release:** v0.1.0
- **Status:** ready
- **Note:** Unblocked on 2026-08-23. The human chose MIT and `LICENSE` is
  committed.

### 5. Make CI run, and require it

- **Promise:** A pull request against `main` runs `./verify/verify.sh` in
  GitHub Actions and cannot merge red.
- **Evidence:** A green run linked from the first pull request, and branch
  protection requiring the `verify` check.
- **Use case:** Serves every case: the gate is what keeps them true.
- **Scope guard:** No new checks. The workflows exist; they have never run.
- **Release:** v0.1.0
- **Status:** ready
- **Note:** Zero workflow runs exist today. `ci.yml` fires on a pull request
  and on a push to `main`, and the MVP went to a branch with neither.

## Later — candidates, not yet specced

Grouped by the release they most likely serve.

**v0.2.0**
- Diff through `btrfs send --no-data` — the tree walk is O(files), not
  O(changes).
- Retention by age and by disk budget.
- `GET /stats`: disk used and snapshot count per workset.
- Metrics an operator can scrape.

**v0.3.0**
- An MCP server over the 5 verbs — the way an agent finds this tool.
- Go and Python client libraries.
- Diff a snapshot against the live working set, with no snapshot taken.
- Labels and search: `GET /snapshots?label=`.
- `GET /snapshots/<id>/paths` — the workset paths of 1 node.

**v0.4.0 and v0.5.0**
- APFS backend through `fs_snapshot_create`.
- VSS backend through the Volume Shadow Copy API.
- A CI matrix that runs the gate on 3 platforms.

**v0.6.0**
- The container layer with CRIU: freeze, restore, discard.
- A workset flag that puts the agent inside the container.

**Unplaced**
- Snapshot of a working set spanning 2 filesystems.
- A read-only mount of a snapshot, so a person can copy 1 file out of it
  instead of restoring the set.

## Shipped

| Week | Feature | Release | Evidence |
|---|---|---|---|
| 2026-08-23 | MVP: engine, store, daemon, client, retention | unreleased | `./verify/verify.sh` green at `HEAD`; 74 tests, 0 failures |

## Explicitly not doing

- A graphical interface. The caller is an agent; the human uses `snapctl`.
- Remote or networked snapshots. The daemon binds loopback and has no
  authentication.
- Rollback of remote state. A written row, a sent email and a called API
  stay done. START.md section 7 states this and it never changes.
- Windows System Restore as the Windows backend. It takes minutes and
  records state a rollback does not need.
- Content-level diff. The 5 verbs answer "which paths moved". A person who
  wants to see inside a file has Git, `diff` and the snapshot on disk.

## Queue changes

- 2026-08-23 — Seeded the queue at install. Order follows START.md section 9:
  the Btrfs proof first, because the MVP gate skips it on a non-Btrfs host.
- 2026-08-23 — Item 5 now blocks item 1. Item 1 is proven by a CI job on a
  loopback btrfs image, and CI has never run. Cycle item 5 first.
- 2026-08-23 — Added the release ladder and re-scoped the queue to v0.1.0.
  The queue held 4 engineering items and no path to a person using the tool.
  2 items were added: an installable release, and CI that actually runs.
  Both are gaps the MVP left, and neither was visible as work before.
