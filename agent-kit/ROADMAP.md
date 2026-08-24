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

**Windows is the first platform.** The human ranked it on 2026-08-23. The
reasoning: START.md targets desktop operating systems, computer-use agents
mostly drive Windows desktops, and a Linux agent usually runs in a container
whose layer already rolls back. Windows is where an agent has no undo at all.

| Release | Theme | Promise | State |
|---|---|---|---|
| v0.1.0 | A person on Windows can use it | A person installs a release binary on Windows, declares a workset, and snapshots, diffs and restores. A snapshot completes in under 2 seconds, and System Restore is never called. | next |
| v0.2.0 | Linux, proven | The same 5 verbs on Btrfs, with the live gate and the START.md section 10 acceptance numbers recorded. | planned |
| v0.3.0 | Fast and bounded | A diff of 2 snapshots that differ by 1 file in a set of 100000 files returns in under 2 seconds, and disk use stays inside the provider's shadow storage budget. | planned |
| v0.4.0 | Built for agents | An agent adds Snapshot with 1 MCP configuration block and gets a checkpoint before each of its actions. | planned |
| v0.5.0 | macOS | The same 5 verbs, the same gate, on APFS. A snapshot completes in under 1 second. | planned |
| v0.6.0 | Total rollback | An agent inside a container restores, and its process tree resumes at the checkpoint. | planned |
| v1.0.0 | Stable | The 5 verbs carry a compatibility guarantee. 3 platforms are green in CI. An upgrade path is documented. | planned |

### What the Windows release costs

VSS is not Btrfs, and the model does not transfer. These 4 facts shape every
item in the queue. State them in the README before v0.1.0 ships.

| | Btrfs, built | VSS, to build |
|---|---|---|
| Scope | 1 subvolume per workset path | 1 shadow copy per **volume**. A workset scopes what diff and restore touch, not what the snapshot holds. |
| Privilege | root for subvolume operations | Administrator. A per-user daemon cannot make a shadow copy. |
| Count | disk-bound | About 64 shadow copies per volume, inside a shadow storage quota. The 50-snapshot budget must respect both. |
| Restore | atomic subvolume swap, O(1) | Copy out of the shadow copy, O(changed bytes). The sub-second restore promise does not survive. |

The 5 verbs survive unchanged. The graph, the store, the daemon, the client
and retention are all backend-agnostic already, which is what the engine
seam was for.

### What each release holds

**v0.1.0 — Windows, the platform where an agent has no undo.** The VSS
backend, the volume-scoped workset model it forces, restore by copy, an
elevation story, and a release a person can install.

**v0.2.0 — Linux, proven.** The Btrfs backend is written and gated but has
never run on Btrfs. The loopback CI job and `verify/acceptance.sh` are in
place; this release is where their numbers get recorded. The work is small
because the code exists.

**v0.3.0 — fast and bounded.** `btrfs send --no-data` on Linux, shadow
storage accounting on Windows, retention by age and disk budget, `GET /stats`
and metrics.

**v0.4.0 — built for agents.** An MCP server over the 5 verbs, Go and Python
clients, a diff against the live working set, labels and search.

**v0.5.0 — macOS.** 1 backend plus its gate, and no change above the seam.

**v0.6.0 — total rollback.** The container layer with CRIU.

**v1.0.0 — stable.** No new capability. The API freezes and 3 platforms run
in 1 CI matrix.

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

The human ranked Windows first on 2026-08-23. Items 1 to 6 are the whole of
v0.1.0, in the order they unblock each other. Items 7 and 8 are v0.2.0 and
carry work that is already written and only needs proving.

### 1. Make CI run, and require it

- **Promise:** A pull request against `main` runs `./verify/verify.sh` in
  GitHub Actions and cannot merge red.
- **Evidence:** A green run linked from pull request 1, and branch protection
  requiring the `verify` check.
- **Use case:** Serves every case: the gate is what keeps them true.
- **Scope guard:** No new checks. The workflows exist; they have never run.
- **Release:** v0.1.0
- **Status:** in progress. Pull request 1 ran CI for the first time in this
  repository's history, on 2026-08-24: `verify` and `verify-btrfs` both
  green. What remains is branch protection, which is a repository setting
  the human applies.

### 2. A Windows gate that runs on every pull request

- **Promise:** `./verify/verify.sh` runs on `windows-latest` in CI and the
  engine, store, API and client suites pass there.
- **Evidence:** A green `verify-windows` job on pull request 1's successor.
- **Use case:** Serves every case, on the platform v0.1.0 targets. No Windows
  host exists in this project, so CI is the only place Windows is real.
- **Scope guard:** The existing backends only. No VSS yet. A PowerShell entry
  point may replace `verify.sh` on Windows, but it runs the same steps.
- **Release:** v0.1.0
- **Status:** ready
- **Note:** Both binaries already cross-compile clean for `windows/amd64`,
  vet included. What is unknown is what fails at run time: `os.Symlink`
  needs Developer Mode or elevation, and `os.Chmod` on Windows only flips
  the read-only attribute, so the mode invariant weakens there.

### 3. The volume-scoped workset model

- **Promise:** A workset resolves to the set of volumes its paths live on,
  `GET /worksets` states them, and a workset spanning 2 volumes is either
  handled or refused with a sentence saying why.
- **Evidence:** Unit tests over the path-to-volume mapping, run in the
  Windows CI job.
- **Use case:** "Undo a file operation" — the user still declares paths; the
  backend decides what a snapshot must cover.
- **Scope guard:** Model and validation only. No VSS calls in this item.
- **Release:** v0.1.0
- **Status:** ready
- **Note:** This is the model change VSS forces. A shadow copy covers a
  volume, not a directory. Getting it wrong here makes every later item wrong.

### 4. The VSS backend: create and delete

- **Promise:** On Windows, `POST /snapshot` makes a real shadow copy in under
  2 seconds and `DELETE /snapshots/<id>` removes it, with the shadow copy ID
  stored as the handle.
- **Evidence:** A live Windows gate that creates, lists and deletes a shadow
  copy, run in CI.
- **Use case:** "Undo a file operation".
- **Scope guard:** Create and delete only. Diff and restore are item 5. The
  Volume Shadow Copy API directly; System Restore is never called.
- **Release:** v0.1.0
- **Status:** blocked on item 3
- **Note:** Needs Administrator. The daemon must detect elevation and refuse
  with a sentence, not a stack trace. Decide in this item whether it ships as
  a Windows service running as LocalSystem.

### 5. VSS diff and restore

- **Promise:** A diff of 2 shadow copies returns the same path-level answer
  the tree walk gives, and a restore returns the workset paths to their exact
  prior contents.
- **Evidence:** The live Windows gate, extended to change a file, diff, and
  restore.
- **Use case:** "Decide whether to roll back" and "Undo a file operation".
- **Scope guard:** Restore copies out of the shadow copy into the live paths.
  No volume-level revert, which would take the whole disk back.
- **Release:** v0.1.0
- **Status:** blocked on item 4
- **Note:** Restore is O(changed bytes) here, not a swap. The README's
  performance claims must be split by backend before this ships.

### 6. Retention inside the shadow copy budget

- **Promise:** Retention keeps the last 50 auto snapshots per workset or as
  many as the provider allows, whichever is smaller, and says in the log when
  the provider is the binding limit.
- **Evidence:** A Windows gate case that fills the budget and shows retention
  holding the line.
- **Use case:** "Free disk space".
- **Scope guard:** Count and provider limits. Age and disk budget are v0.3.0.
- **Release:** v0.1.0
- **Status:** blocked on item 4
- **Note:** About 64 shadow copies per volume, inside a shadow storage quota.
  The current budget of 50 does not know either number exists.

### 7. Prove the Btrfs backend on a Btrfs host

- **Promise:** On a Btrfs host, `go test -tags btrfs_live ./internal/engine/`
  passes, and `verify/acceptance.sh` records a snapshot under 1 second and a
  diff under 2 seconds on a 5 GB working set.
- **Evidence:** The `verify-btrfs` and `acceptance` CI job output, with the
  numbers copied into `DEVLOG.md`.
- **Use case:** "Undo a file operation" on Linux.
- **Scope guard:** No new verbs. No Windows work.
- **Release:** v0.2.0
- **Status:** in progress. Half proven on 2026-08-24: `TestBtrfsLive` PASS
  on a real btrfs filesystem in CI run 32677729798. The acceptance numbers
  are still missing; that job skips on a pull request and runs on demand.

### 8. A workset that covers a plain directory on Btrfs

- **Promise:** A workset path that is a directory and not a subvolume is
  snapshotted, and `snapctl workset` states which paths it converted.
- **Evidence:** A live gate case that declares a plain directory and restores
  it.
- **Use case:** "Checkpoint every step of a long task".
- **Scope guard:** No automatic conversion of a directory a person did not
  name.
- **Release:** v0.2.0
- **Status:** ready

### 9. Reconcile the graph and the snapshot root at start

- **Promise:** A daemon killed during `Create` leaves no handle without a
  row and no row without a handle, and says at start how many it repaired.
- **Evidence:** A test that kills the service between the engine call and
  the store write, then reopens it.
- **Use case:** "Recover the graph after a restart".
- **Scope guard:** No write-ahead journal of its own.
- **Release:** v0.2.0
- **Status:** ready

### 10. Ship an installable release

- **Promise:** `snapshotd` and `snapctl` install from a published GitHub
  release on a clean machine, the service starts the daemon, and `snapctl
  snapshot` works without the repository present.
- **Evidence:** A run of the published artifact on a clean machine, recorded
  in `DEVLOG.md`. `RELEASING.md` step 6 requires this anyway.
- **Use case:** Serves every case in `docs/USE-CASES.md`. None of them is
  reachable without an install.
- **Scope guard:** Binaries for Windows and Linux, a service definition per
  platform, and install text. No distribution packages, no Homebrew, no
  container image.
- **Release:** v0.1.0 for the Windows half, v0.2.0 for Linux
- **Status:** blocked on item 5
- **Note:** MIT `LICENSE` is committed, so nothing legal blocks publishing.

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
- 2026-08-23 — Windows moved to v0.1.0 and Linux to v0.2.0, on the human's
  ranking. The reasoning is in the release ladder. The queue was rewritten:
  4 Windows items were added, the Btrfs proof moved from position 1 to 7,
  and "make CI run" moved to position 1 because every other item's evidence
  needs it. The Btrfs work stays gated and shipping; it is not discarded.
- 2026-08-23 — Item 5 now blocks item 1. Item 1 is proven by a CI job on a
  loopback btrfs image, and CI has never run. Cycle item 5 first.
- 2026-08-23 — Added the release ladder and re-scoped the queue to v0.1.0.
  The queue held 4 engineering items and no path to a person using the tool.
  2 items were added: an installable release, and CI that actually runs.
  Both are gaps the MVP left, and neither was visible as work before.
