# Changelog

All notable changes to Snapshot are documented here.
Format: [Keep a Changelog](https://keepachangelog.com/) · Versioning: [SemVer](https://semver.org/).

## [Unreleased]
### Added
### Changed
### Deprecated
### Removed
### Fixed
### Security

## [0.1.0] - unreleased

The MVP. It is not tagged yet: `RELEASING.md` step 4 cuts `v0.1.0` when the
human is ready for the release workflow to publish binaries.
### Added
- `snapshotd`, a local service on `127.0.0.1:7099` with 5 verbs:
  `POST /snapshot`, `GET /snapshots`, `GET /diff`, `POST /restore` and
  `DELETE /snapshots/{id}`.
- `POST /worksets` and `GET /worksets` declare the paths the 5 verbs act on.
- `snapctl`, a command line client for the same API. It draws the snapshot
  graph as a tree and prints a diff as A, M and D lines.
- A Btrfs backend that snapshots subvolumes with `btrfs subvolume snapshot
  -r`, and a portable copy backend that hardlinks the working set.
- APFS and VSS backend stubs that refuse with a clear message.
- A SQLite store for the snapshot graph. The graph survives a restart.
- Implicit checkpoints: a caller marks a snapshot `auto`, and retention keeps
  the last 50 per workset. Retention never removes a manual snapshot or an
  ancestor of one.
- A safety snapshot before every restore.
- `./verify/verify.sh`, the health gate. It runs a live Btrfs test when the
  host has Btrfs and skips loudly when it does not.

### Fixed before release
- A workset that named a symlinked directory stored an empty snapshot, and a
  restore of it replaced the symlink with an empty directory. `POST
  /worksets` now resolves the link, and the engine refuses a symlinked path.
- A restore failed when a declared path had been deleted, which is the case
  a rollback exists for. The safety snapshot now warns and the restore runs.
- A workset that contained the data directory made a snapshot walk into the
  handle it was writing. The daemon refuses that workset.
- A restore returned files with the daemon's umask applied and dropped
  setuid, setgid and sticky bits.
- A read-only directory in the working set made the whole snapshot fail.
- Prune and retention removed the graph row before the snapshot on disk. A
  failed delete left a snapshot no row could reach.
- A missing or unreadable workset path returned 500. It returns 400.
- On Btrfs, a staged subvolume left by an interrupted restore was cleaned up
  with `rmdir`, which cannot remove a subvolume, and every later restore of
  that path failed.
- A read-only directory in the working set made its snapshot impossible to
  delete, which stopped retention for that workset and every workset after
  it. Anything that discards a tree now makes its directories writable first.
- A restore over a working set holding a read-only directory landed on disk
  and then reported 500, and left a tree that made every later restore of
  that path fail.
- The safety snapshot was all or nothing: one unreadable path discarded the
  state of every other path in the workset. It now covers what it can, and
  `POST /restore` returns `safety_snapshot` and `safety_warning`.
- A restore overwrote a declared path that had become a symlink, deleting the
  link and orphaning the work at its target. It refuses with 400.
- The nesting guard missed a data directory reached through a symlink, and
  read a directory named `..foo` as an escape.
