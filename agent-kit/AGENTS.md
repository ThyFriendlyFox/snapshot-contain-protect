# AGENTS.md — the binding contract

In effect whenever code in this repo is touched, by agent or human.
When this conflicts with intuition, this wins.

## Commands

```sh
go build ./...      # build
go test ./...       # tests
gofmt -l cmd internal && go vet ./...       # lint / format check
./verify/verify.sh     # full health gate — must pass before any push
```

Requires Go 1.25.

## Invariants — never regress these

1. **The engine interface is the only way to touch a filesystem.** No
   package above `internal/engine` runs `btrfs`, links a file, or walks
   a snapshot tree. A new filesystem is a new backend, not a branch in
   the daemon.
2. **The daemon binds loopback only.** The service has no
   authentication. `checkLoopback` refuses any other address, and that
   check never becomes a flag.
3. **History is append-only.** A restore adds a node whose parent is the
   restored snapshot. Only an explicit prune removes a node.
4. **A restore needs `confirm: true`.** An agent must not roll back by
   accident.
5. **Retention never removes a manual snapshot, and never removes an
   ancestor of one.** The 50-snapshot budget applies to `auto` nodes
   only.
6. **A pruned node hands its children to its parent.** The graph stays
   connected; a dangling `parent_id` is a defect.
7. **Snapshot creation copies no data.** Btrfs snapshots and the copy
   backend's hardlinks both cost inodes, not bytes. A backend that
   copies a working set fails the point of the tool.
8. **The five verbs keep their shapes.** START.md section 5 defines the
   request and response JSON. A change there breaks every agent that
   calls the service.
9. **The store owns the schema.** Migrations are append-only files under
   `internal/store/migrations/`; an existing migration is never edited.
10. **A restore returns the exact prior file state.** Modes, including
    setuid, setgid and sticky, survive. `open(2)` and `mkdir(2)` apply the
    umask, so the mode is set after the write, never by the create call.
11. **A workset never contains the snapshot root.** Otherwise a snapshot
    walks into the handle it is writing. Compare resolved paths: a data
    directory reached through a symlink is spelled differently.
12. **Anything that discards a tree uses `removeTree`.** A snapshot
    reproduces a read-only directory, and `os.RemoveAll` cannot unlink its
    children as a normal user. A handle that cannot be deleted can never be
    pruned.
13. **Retention survives one stuck snapshot.** A handle that will not go
    keeps its row, logs, and does not stop the pass or the worksets after
    it.
14. **A restore that landed on disk reports success.** Housekeeping after
    the rename never turns a successful restore into an error.

## Landmine map

| Area | Why it bites |
|---|---|
| `internal/engine/copy.go` | Hardlinks share inodes. An in-place write to a live file also rewrites the snapshot. Restore copies bytes on purpose; do not "optimise" it into a link. |
| `internal/api/retention.go` | The prune loop deletes rows as it goes. Re-read each node before reparenting, or the foreign key fails on a parent that is already gone. |
| `internal/engine/diff.go` | The differ compares size and modification time, never content. A test that writes two files in the same millisecond can see them as equal; set the time explicitly. |
| `internal/engine/btrfs.go` | Every source path must be its own subvolume. `btrfs subvolume snapshot` on a plain directory fails, and the error names the path, not the cause. |
| `internal/store/store.go` | The database runs with one connection on purpose. Adding parallelism reintroduces `SQLITE_BUSY` under snapshot bursts. |
| `internal/api/service.go` | Every write path takes the per-workset lock. A restore and a snapshot on the same paths at once tangles the graph. |
| Workset paths | A symlinked directory passes `os.Stat` but a tree walk does not descend through it. Validate with `os.Lstat`, and resolve the link when the workset is declared. |
| Prune and retention | Free the handle before the row. A row without a handle breaks every later diff and restore of that node. |
| The safety snapshot | It must never block a restore. The state that most needs a rollback is often the state that cannot be snapshotted. It is also never all-or-nothing: it covers the paths it can and reports what it missed in `safety_warning`. |
| `internal/engine/copy.go` `swapIn` | The final cleanup runs after both renames. A failure there is housekeeping, not a failed restore. A stale `.snapshot-previous` wedges every later restore of that path. |
| Read-only directories | They appear in real working sets, and as root every permission check passes. Gate step 6 runs the suites as a normal user; do not let it rot. |

## House style

- Match the surrounding code's idiom, naming, and comment density.
- No demo scaffolding, no leftover diagnostics, no dead flags.
- Comments state constraints the code can't show — never narration.
- User-facing copy states the thing plainly; no reassurance microcopy.

## Process rules

- Branch from `main`; never commit to it directly.
- `./verify/verify.sh` green before every push. Flaky gate → fix or
  quarantine in the same PR; never route around it.
- After adding/removing/renaming source files, run the stack's
  regeneration step (project gen, lockfile, tidy) and commit the result.
- Commit at boundaries; message says what changed and cites evidence.
- Docs move with behavior — same commit or PR.
- Report outcomes faithfully; failing is failing, with output.
