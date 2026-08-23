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
