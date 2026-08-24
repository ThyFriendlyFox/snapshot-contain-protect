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

### The copy backend on Windows

NTFS supports hardlinks, so the backend runs there. 2 guarantees weaken, and
neither has a workaround at this layer:

- `os.Chmod` on Windows only toggles the read-only attribute. Permission bits
  do not survive a restore, and ACLs are not preserved at all. Mode
  preservation is a Unix guarantee.
- Creating a symlink needs Developer Mode or elevation. A working set holding
  one may fail to snapshot for an unelevated daemon.

The Windows release targets VSS, not this backend. See `ROADMAP.md`.

## apfs

The macOS backend. It is a stub. Every call returns "the apfs backend is not
implemented". The real implementation calls `fs_snapshot_create`.

## vss

The Windows backend. It is a stub, and it is the v0.1.0 target.

A shadow copy covers a **volume**, not a directory. `GET /worksets` reports
the volumes each workset covers, so a caller can see how many shadow copies
a snapshot will need. A workset spanning 2 volumes needs 2. The workset
still scopes what diff and restore touch.

Every other backend ignores volumes; for them the field is reporting only.

### How a shadow copy becomes a handle

Measured on `windows-latest`, 2026-08-24.

A shadow copy answers as a device path such as
`\\?\GLOBALROOT\Device\HarddiskVolumeShadowCopy1`. Files cannot be read
through that path directly. A directory symlink to it can be read, and the
trailing backslash is required:

```
mklink /d C:\ProgramData\snapshot\mounts\<id> "\\?\GLOBALROOT\Device\HarddiskVolumeShadowCopyN\"
```

So a VSS handle is a mount. Create makes the shadow copy and then the
symlink. Delete removes the symlink and then the shadow copy. The mount
outlives a crash, so start-up reconciliation removes any mount with no row.

Once mounted, the shared differ and the byte-copy restore work through it
unchanged. `restoreTrees` serves the copy backend and VSS alike: once a
snapshot is a readable tree, putting it back is the same work.

A VSS restore copies the bytes, so it costs the size of the working set.
A Btrfs restore swaps a subvolume and costs nothing. State that difference
wherever the performance of a restore is claimed.

The current stub. The real implementation calls the Volume
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
