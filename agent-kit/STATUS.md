# STATUS.md — where the project actually stands

The single source of truth for project state. Claims require evidence: a
passing gate, a linked run, a tag. Updated in the same commit as the
behavior change. The weekly cycle (WEEKLY.md step 5) refreshes it.

| Area | State | Evidence |
|---|---|---|
| Engine interface | ✅ | `./verify/verify.sh` green; 4 operations, 4 backends |
| Copy backend | ✅ | 18 engine tests, also run as a normal user; 100 snapshots of 200 files stay under 100 MB; modes survive a restore |
| Btrfs backend | 🚧 | Written and unit-tested through a command seam. Unproven: this kernel cannot mount btrfs. The `verify-btrfs` CI job proves it on a loopback image, and CI has never run. ROADMAP items 1 and 5. |
| APFS backend | ❌ | Stub. Refuses with "not implemented". |
| VSS backend | ❌ | Stub. Refuses with "not implemented". |
| SQLite store | ✅ | 10 store tests; the graph survives a reopen |
| Daemon, 5 verbs | ✅ | 32 API tests; a manual run snapshotted in 1 ms and restored a working set |
| snapctl client | ✅ | 5 client tests against a real service |
| Implicit checkpoints | ✅ | 6 retention tests; the 50-snapshot budget holds |
| Container layer | 🧊 | Seam only. START.md puts it after step 5; the MVP stops at step 5. |
| CI | ✅ | `.github/workflows/ci.yml` runs the same command as local |
| Unprivileged gate | ✅ | Gate step 6 reruns the engine and API suites as user 65534 |
| Acceptance test (START.md section 10) | 🚧 | Mechanised as `verify/acceptance.sh` with a CI job. Never executed. ROADMAP items 1 and 5. |
| CI actually running | ❌ | The workflows have never fired. 0 runs exist: `ci.yml` needs a pull request or a push to `main`. ROADMAP item 5. |
| Installable release | ❌ | No tag and no published binary. MIT `LICENSE` is committed. ROADMAP item 4. |

States: ✅ done (gated) · 🚧 in progress · ❌ not started · 🧊 frozen/won't do.

## Current week

- **Shipping:** between cycles. The MVP is built and gated. The next 5 cycles are v0.1.0, the release that makes it usable: ROADMAP items 1 to 5.
- **Last release:** none. The MVP sits on `claude/build-agent-kit-mvp-rryrb6`, unmerged and untagged. `RELEASING.md` cuts `v0.1.0`.
- **Known red:** none. The gate is green. The Btrfs backend is untested on a
  Btrfs host, which the gate reports as a loud skip and not as a pass.
