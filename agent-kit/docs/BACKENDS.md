# Backends

A backend translates four operations — create, delete, diff, restore — into
one filesystem's snapshot primitive. Snapshot has 4 backends. Select one with
the `-backend` flag. The default, `auto`, picks Btrfs when it runs on this
host and `copy` when it does not.

## btrfs

Snapshots Btrfs subvolumes. Creation is copy-on-write and copies no data, so
a checkpoint of a 5 GB working set costs milliseconds. Every path in the
workset must be its own subvolume. This is the backend the MVP targets.

Restore creates a writable snapshot beside the live subvolume, deletes the
live subvolume, and renames the new one into place. Root rights are required,
as they are for every Btrfs subvolume operation.

```sh
snapshotd -backend btrfs -data-dir /mnt/data/snapshot
```

## copy

Snapshots by hardlinking every file into the snapshot root. It runs on any
filesystem, which is why development and CI use it.

It is not copy-on-write. A writer that opens an existing file and writes in
place changes the snapshot too, because both names point at one inode. A
writer that creates a new file and renames it over the old one is safe, and
that is what editors and most agents do. Restore copies bytes instead of
linking, so a restored tree never shares an inode with the snapshot it came
from.

```sh
snapshotd -backend copy
```

## apfs

The macOS backend. It is a stub. Every call returns "the apfs backend is not
implemented". The real implementation calls `fs_snapshot_create`.

## vss

The Windows backend. It is a stub. The real implementation calls the Volume
Shadow Copy Service API directly. It does not call System Restore, which
takes minutes and records registry state a rollback does not need.

## Writing a new backend

Implement `engine.Engine` in `internal/engine/`:

| Method | Contract |
|---|---|
| `Name` | The string written to the `backend` column. |
| `Available` | Returns `nil` only if this backend can run on this host. Wrap `ErrUnavailable`. |
| `Create` | Snapshots every source path, writes the handle manifest, returns the handle. Cleans up after a partial failure. |
| `Delete` | Removes the snapshot behind a handle. |
| `Restore` | Returns every source path to the state in the handle. A failed restore leaves the working set untouched. |
| `Diff` | Summarises two handles. Reuse `diffHandles` unless the filesystem answers faster. |

Register it in `select.go`. A new backend lands with: the implementation, its
section in this file, its `-backend` value in `docs/CONFIGURATION.md`, and a
gate in `VERIFICATION.md` that skips loudly when the filesystem is absent.
