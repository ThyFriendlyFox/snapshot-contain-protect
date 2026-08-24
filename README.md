# Snapshot

A Git-style checkpoint and rollback tool for desktop operating systems. It is
built for computer-use agents that need a cheap undo.

A code agent can undo its work because Git exists. An agent that moves files
on a desktop cannot. Operating system restore tools are slow, coarse and not
built for a program to call. The filesystem primitives are already fast; this
service exposes them in a usable shape.

Snapshot creates a filesystem snapshot of a declared set of paths in under 1
second, lists the snapshots as a graph, diffs any 2 of them, and restores any
one of them. It answers on `127.0.0.1` over HTTP, so an agent calls it the
way it calls any other tool.

## Requirements

- Linux. Btrfs for real work; any filesystem for development.
- Go 1.25 to build.

## Build

```sh
go build ./...
./verify/verify.sh    # the health gate
```

## Run

```sh
snapshotd &                                   # 127.0.0.1:7099
snapctl workset proj-a /home/u/proj           # declare the paths
snapctl snapshot proj-a "before file cleanup" # 01J7Z... in 12 ms
snapctl list proj-a
snapctl diff <from-id> <to-id>
snapctl restore <id>
snapctl prune <id> [--cascade]
```

The daemon picks Btrfs when it runs on a Btrfs host and the hardlink backend
when it does not. Pin one with `-backend btrfs`. See
`agent-kit/docs/BACKENDS.md`.

## The API

Local HTTP on `127.0.0.1`. JSON in, JSON out. 5 verbs, plus the workset
declaration they act on.

| Verb | Body or query | Returns |
|---|---|---|
| `POST /snapshot` | `{"workset":"proj-a","label":"before file cleanup","auto":false}` | `{"id","created_at","duration_ms"}` |
| `GET /snapshots` | `?workset=proj-a` | The graph as a flat list. Each item carries `id`, `parent_id`, `label`, `created_at`, `auto`. |
| `GET /diff` | `?from=<id>&to=<id>` | `{"added":[],"modified":[],"deleted":[],"truncated":false}`, capped at 500 paths per category |
| `POST /restore` | `{"id":"01J...","confirm":true}` | The new snapshot node the restore appended, plus `safety_snapshot` and, when something was missed, `safety_warning` |
| `DELETE /snapshots/<id>` | `?cascade=true` | `{"removed":["01J..."]}`. It refuses a node with children unless `cascade=true`. |
| `POST /worksets` | `{"name":"proj-a","paths":["/home/u/proj"]}` | The workset |
| `GET /worksets` | | Every workset |
| `GET /healthz` | | `{"status":"ok","backend":"btrfs"}` |

A restore needs `confirm: true`, so an agent cannot roll back by accident.
Before it restores, the service takes a safety snapshot of the current state
and returns its identifier as `safety_snapshot`.

The safety snapshot never blocks the restore: the state that most needs a
rollback is often the state that cannot be read. It covers every path it can,
so one deleted path in a workset of 5 does not discard the other 4. When it
missed something, or could take nothing at all, `safety_warning` says so and
`safety_snapshot` is `null`. Read that field before you rely on being able to
undo the undo.

A restore refuses a declared path that has become a symlink since the
snapshot, rather than deleting the link. A restore of a workset with several
paths restores them one at a time. If one path fails, the paths before it are
already back.

## The graph

A snapshot's parent is the newest snapshot of its workset. A restore does not
delete anything: it appends a node whose parent is the restored snapshot, so
the work that followed stays in the graph as a branch. History is
append-only. Only `DELETE /snapshots/<id>` removes a node.

Mark a snapshot `auto` when an agent takes it before an action. Snapshot
keeps the last 50 auto snapshots per workset and every manual one. It never
prunes a snapshot that a manual snapshot descends from.

## Known limits

These are real. They have no workaround at the filesystem layer.

1. **Processes do not roll back.** Files revert. A running process keeps its
   old file descriptors and memory. Restart the process tree after a restore.
2. **Sockets do not roll back.** Open connections break on restore.
3. **Remote state does not roll back.** A row written to a remote database
   stays written. So does a sent email and a called API.
4. **Partial restore is the default.** A total rollback needs the container
   layer, which this build does not have.
5. **The copy backend is not copy-on-write.** It hardlinks. A writer that
   opens an existing file and writes in place changes the snapshot too. A
   writer that creates a new file and renames it over the old one is safe,
   and that is what editors and most agents do. Use Btrfs for work that
   matters.
6. **A workset path must be a real directory, not a symlink.** A tree walk
   does not descend through a symlinked root. `snapctl workset` resolves the
   link for you and stores the directory it points at.
7. **A workset path must not contain the data directory.** A snapshot of it
   would contain itself. The daemon refuses the snapshot and says so.
8. **The Btrfs backend is proven in CI, not on a desktop.** The
   `verify-btrfs` job creates a loopback Btrfs filesystem and runs the live
   gate on every pull request. No physical Btrfs machine has run it. On a
   host without Btrfs the gate skips loudly, never silently.

9. **On Windows, modes do not survive a restore.** `chmod` there only toggles
   the read-only attribute, and ACLs are not preserved at all. Contents and
   layout come back; permissions do not. Creating a symlink also needs
   Developer Mode or elevation.

Limits 1 and 2 disappear inside a container with CRIU. Limit 3 never
disappears. Limits 6 and 7 are enforced: the daemon refuses the workset
rather than storing an empty snapshot.

## Layout

```
cmd/snapshotd/     daemon entry point
cmd/snapctl/       command line client
internal/engine/   backend interface, btrfs, copy, apfs and vss
internal/store/    SQLite access and migrations
internal/api/      the 5 verbs, worksets, retention
internal/container/ the seam for process rollback
verify/verify.sh   the health gate
agent-kit/         how this repository is worked on
```

## Working on it

`agent-kit/ROUTING.md` says which file governs which situation.
`agent-kit/AGENTS.md` is binding whenever code is touched.
`agent-kit/ROADMAP.md` decides what gets built next.

## License

MIT. See `LICENSE`.

## Security

Report a vulnerability privately. See `agent-kit/SECURITY.md`. The daemon has
no authentication and refuses to bind anything but loopback.
