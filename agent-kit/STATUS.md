# STATUS.md — where the project actually stands

The single source of truth for project state. Claims require evidence: a
passing gate, a linked run, a tag. Updated in the same commit as the
behavior change. The weekly cycle (WEEKLY.md step 5) refreshes it.

| Area | State | Evidence |
|---|---|---|
| Engine interface | ✅ | `./verify/verify.sh` green; 4 operations, 4 backends |
| Copy backend | ✅ | 18 engine tests, also run as a normal user; 100 snapshots of 200 files stay under 100 MB; modes survive a restore |
| Btrfs backend | ✅ | Proven. `TestBtrfsLive` PASS on a real btrfs filesystem in CI run 32677729798, 2026-08-24: subvolume created, snapshotted, file changed, restored, file came back. |
| APFS backend | ❌ | Stub. Refuses with "not implemented". |
| VSS backend | 🚧 | Create, delete and diff written; 6 unit tests through a Runner seam. Restore is item 5. Unproven against a real provider until the live gate runs. |
| SQLite store | ✅ | 10 store tests; the graph survives a reopen |
| Daemon, 5 verbs | ✅ | 32 API tests; a manual run snapshotted in 1 ms and restored a working set |
| snapctl client | ✅ | 5 client tests against a real service |
| Implicit checkpoints | ✅ | 6 retention tests; the 50-snapshot budget holds |
| Startup reconciliation | ✅ | 6 tests. Orphan handles removed, orphan rows removed, a wrong data directory refused rather than obeyed. |
| Container layer | 🧊 | Seam only. START.md puts it after step 5; the MVP stops at step 5. |
| CI | ✅ | `.github/workflows/ci.yml` runs the same command as local |
| Unprivileged gate | ✅ | Gate step 6 reruns the engine and API suites as user 65534 |
| Volume mapping | ✅ | `GetVolumePathNameW` on Windows, device-number walk on unix. Proven on both in CI run 32679318909. Reported by `GET /worksets`. |
| Windows gate | ✅ | `verify-windows` green on `windows-latest`, CI run 32678743583. It now also runs the live VSS gate. |
| Acceptance test (START.md section 10) | ✅ | All 5 claims hold. CI run 32677974928, 2026-08-24, 5120 MB working set: snapshot 8 ms, diff 12 ms, exact restore, graph survived a restart, 100 snapshots cost 21 MB. |
| CI actually running | 🚧 | First run green: `verify` and `verify-btrfs` both passed on pull request 1. Branch protection is not set, so red can still merge. ROADMAP item 1. |
| Installable release | ❌ | No tag and no published binary. MIT `LICENSE` is committed. ROADMAP item 4. |

States: ✅ done (gated) · 🚧 in progress · ❌ not started · 🧊 frozen/won't do.

## Current week

- **Shipping:** v0.1.0. Items 2, 3, 7 and 9 are done. Item 4 is written and waiting on its live gate. Item 1 needs branch protection, which is a repository setting the human applies.
- **Last release:** none. The MVP sits on pull request 1, unmerged and untagged.
- **Known red:** none. The gate is green locally and in CI. The Btrfs
  backend is proven in CI on a loopback image; no physical Btrfs machine has
  run it.
