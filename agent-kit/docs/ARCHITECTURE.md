# Architecture

Snapshot is a local HTTP service and a command line client, written in Go and
shipped as two binaries. The code has 6 parts.

| Part | Folder | Task |
|---|---|---|
| Daemon | `cmd/snapshotd/` | Reads flags, opens the service, serves until a signal |
| Client | `cmd/snapctl/` | Speaks the same HTTP API a computer-use agent speaks |
| API | `internal/api/` | The five verbs, the worksets they act on, and retention |
| Engine | `internal/engine/` | The filesystem primitive, one backend per filesystem |
| Store | `internal/store/` | SQLite metadata: the snapshot graph and the worksets |
| Container | `internal/container/` | The seam for process rollback; refuses in this build |

## Data flow

One checkpoint, from the agent's call to the row on disk.

1. The agent sends `POST /snapshot {"workset":"proj-a","auto":true}`.
2. `internal/api/handlers.go` decodes the body and calls `CreateSnapshot`.
3. `internal/api/service.go` reads the workset from the store and takes the
   per-workset lock.
4. The service asks the store for the newest node of that workset. That node
   becomes the parent.
5. The service calls `engine.Create` with a new ULID and the workset paths.
6. The backend snapshots each path into `<data-dir>/snapshots/<id>/` and
   writes `manifest.json`, which maps each subtree to its source path.
7. The service writes one row to `snapshots` with the handle, the parent and
   the `auto` flag.
8. Retention prunes the excess auto snapshots of that workset.
9. The handler returns `{"id","created_at","duration_ms"}`.

A restore runs steps 3 to 7 twice: once for the safety snapshot before the
rollback, once for the node the restore appends after it.

## API

| File | Task |
|---|---|
| `service.go` | Config, workset and snapshot operations, per-workset locks |
| `handlers.go` | Routes, JSON shapes, status codes, request log |
| `server.go` | Listener, loopback check, shutdown |
| `retention.go` | The 50-snapshot budget and the timer that applies it |
| `reconcile.go` | Makes the graph and the snapshot root agree at start |
| `errors.go` | Status codes for caller-visible failures |

## Engine

| File | Task |
|---|---|
| `engine.go` | The `Engine` interface, the handle manifest, source checks |
| `diff.go` | The shared differ: mode, size, modification time, symlink target |
| `btrfs.go` | Btrfs subvolume snapshots through a `Runner` seam |
| `copy.go` | Hardlink snapshots for development and CI hosts |
| `apfs.go`, `vss.go` | Stubs that refuse cleanly |
| `select.go` | Picks a backend by name, or the best one for the host |

## Store

| File | Task |
|---|---|
| `store.go` | Open, migrate, and every query the service makes |
| `migrations/0001_init.sql` | The snapshots and worksets tables |

## Boundaries

| Boundary | Rule |
|---|---|
| API ↔ Engine | The API passes paths and handles. It never runs a filesystem command and never reads a snapshot tree. |
| API ↔ Store | The store answers with rows. It never touches the filesystem, and it never deletes a handle. |
| Engine ↔ Store | No link. A backend does not know the database exists. |
| Daemon ↔ Network | The listener binds loopback. `checkLoopback` refuses every other address. |
| Client ↔ Daemon | HTTP and JSON only. The client never opens the database or the snapshot root. |
