# Snapshot MVP Spec

A Git-style checkpoint and rollback tool for desktop operating systems. Built for
computer-use agents that need a cheap undo.

---

## 1. Problem

Code agents can undo their work because Git exists. Computer-use agents cannot.
Operating system restore tools are slow, coarse, and not designed for programmatic
use. Windows System Restore takes minutes. It also records registry and system state
that an agent rollback does not need.

The filesystem primitives are already fast. They are not exposed in a usable shape.

---

## 2. Goal

Provide a local service that creates a filesystem checkpoint in under one second,
lists checkpoints as a graph, diffs any two checkpoints, and restores to any
checkpoint. Expose this as five verbs over a local API so an agent can call it.

---

## 3. Scope

**In scope for MVP:**

- Linux host with a Btrfs filesystem.
- Snapshot of a declared working set, not the whole volume.
- SQLite metadata store.
- Local HTTP API with five verbs.
- Command line client over the same API.
- Optional container target for full process rollback.

**Not in scope for MVP:**

- Windows and macOS backends.
- Graphical user interface.
- Remote or networked snapshots.
- Rollback of external state such as remote databases or sent email.

---

## 4. Architecture

Three layers. Each layer is replaceable.

### 4.1 Snapshot engine

Wraps the filesystem primitive. Btrfs first.

| Operation | Btrfs command |
|---|---|
| Create | `btrfs subvolume snapshot -r <src> <dest>` |
| Delete | `btrfs subvolume delete <path>` |
| Diff | `btrfs send --no-data -p <parent> <child>` |
| Restore | swap active subvolume, then remount |

Snapshots are copy-on-write. Creation does not copy data. Expect sub-second
creation time.

The engine is an interface. Later backends implement the same four operations:

- **macOS:** APFS snapshots via `fs_snapshot_create`. Near-instant.
- **Windows:** raw Volume Shadow Copy via the VSS API. One to two seconds.
  Do not use System Restore.

### 4.2 Metadata store

SQLite. One file. Lives next to the snapshot root.

```sql
CREATE TABLE snapshots (
  id          TEXT PRIMARY KEY,   -- ULID
  parent_id   TEXT REFERENCES snapshots(id),
  label       TEXT,
  created_at  INTEGER NOT NULL,   -- unix epoch
  workset_id  TEXT NOT NULL REFERENCES worksets(id),
  fs_handle   TEXT NOT NULL,      -- backend path or shadow copy ID
  backend     TEXT NOT NULL,      -- 'btrfs' | 'apfs' | 'vss'
  container   TEXT,               -- container checkpoint ID, nullable
  auto        INTEGER NOT NULL    -- 1 if created implicitly before an action
);

CREATE TABLE worksets (
  id      TEXT PRIMARY KEY,
  name    TEXT NOT NULL,
  paths   TEXT NOT NULL           -- JSON array of absolute paths
);

CREATE INDEX idx_snapshots_parent ON snapshots(parent_id);
CREATE INDEX idx_snapshots_workset ON snapshots(workset_id, created_at);
```

The graph lives entirely in `parent_id`. Branching is free. A restore does not
delete children. It creates a new node whose parent is the restored snapshot.
History is therefore append-only.

### 4.3 Container layer

Optional in MVP. Set a flag on the workset to enable it.

When enabled, the agent runs inside a container. The container filesystem layer is
already copy-on-write. Rollback discards the layer instead of restoring the host.

Use CRIU through the container runtime to freeze and restore the process tree.
This is the only path to a total rollback. See section 7.

---

## 5. API

Local HTTP on `127.0.0.1`. JSON in, JSON out. Five verbs.

### `POST /snapshot`

```json
{ "workset": "proj-a", "label": "before file cleanup", "auto": false }
```

Returns `{ "id": "01J...", "created_at": 1755900000, "duration_ms": 340 }`.

### `GET /snapshots?workset=proj-a`

Returns the graph as a flat list. Each item carries `id`, `parent_id`, `label`,
`created_at`, `auto`. The client builds the tree.

### `GET /diff?from=<id>&to=<id>`

Returns a path-level summary. Not file contents.

```json
{
  "added":    ["/home/u/proj/new.txt"],
  "modified": ["/home/u/proj/config.json"],
  "deleted":  ["/home/u/proj/old.log"],
  "truncated": false
}
```

Cap the list at 500 paths per category. Set `truncated` when the cap is hit.
An agent uses this to decide whether a rollback is warranted before it commits
to one.

### `POST /restore`

```json
{ "id": "01J...", "confirm": true }
```

Returns the new snapshot node created by the restore. Requires `confirm` so an
agent cannot roll back by accident.

### `DELETE /snapshots/<id>`

Prune. Refuses if the snapshot has children, unless `?cascade=true` is set.

---

## 6. Implicit checkpoints

Every agent action gets a snapshot before it runs. Mark it `auto = 1`.

Retention: keep the last 50 auto snapshots per workset. Keep all manual ones.
Prune auto snapshots oldest-first on a timer. Never prune a snapshot that is an
ancestor of a manual snapshot.

---

## 7. Known limits

State these in the README. They are real and they do not have a workaround at the
filesystem layer.

1. **Processes do not roll back.** Files revert. A running process keeps its old
   file descriptors and memory. Restart the process tree after a restore.
2. **Sockets do not roll back.** Open connections break on restore.
3. **Remote state does not roll back.** A row written to a remote database stays
   written. So does a sent email or an API call.
4. **Partial restore is the default.** A total rollback needs the container layer.

Limits 1 and 2 disappear inside a container with CRIU. Limit 3 never disappears.

---

## 8. Layout

```
snapshotd/
  cmd/
    snapshotd/          # daemon entry point
    snapctl/            # CLI client
  internal/
    engine/
      engine.go         # backend interface
      btrfs.go
      apfs.go           # stub for now
      vss.go            # stub for now
    store/
      store.go          # SQLite access
      migrations/
    container/
      runtime.go        # freeze, restore, discard
    api/
      server.go
      handlers.go
  README.md
```

Language: Go. It ships as one binary, it shells out cleanly, and the container
runtime libraries are native to it. Rust is a fine substitute if preferred.

---

## 9. Build order

1. **Engine interface plus Btrfs backend.** Create, delete, diff, restore. Test
   directly, with no daemon and no database.
2. **SQLite store and migrations.** Write the schema. Verify the parent graph
   survives a restore.
3. **Daemon plus the five endpoints.** No container support yet.
4. **CLI client.** `snapctl snapshot`, `list`, `diff`, `restore`, `prune`.
5. **Implicit checkpoints and retention.**
6. **Container layer behind a workset flag.**

Stop after step 5 and use the tool for a week before starting step 6.

---

## 10. Acceptance test

On a Btrfs host with a 5 GB working set:

- A snapshot completes in under one second.
- A diff of two snapshots that differ by one file returns in under two seconds.
- A restore returns the working set to its exact prior file state.
- The snapshot graph survives a daemon restart.
- 100 sequential snapshots consume less than 100 MB of extra disk space.
