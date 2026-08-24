# Configuration

Snapshot reads command line flags. Two flags also read an environment
variable, which is what a service manager sets. There is no configuration
file: the daemon holds 5 options, and a file would be a second source of
truth.

| Platform | Default data directory |
|---|---|
| Linux | `$HOME/.local/share/snapshot` |
| macOS | `$HOME/.local/share/snapshot` |
| Windows | `%USERPROFILE%\.local\share\snapshot` |

The data directory holds `snapshots/` and `snapshot.db`. The daemon creates
both at first start. A missing directory is created; it is not an error.

A damaged database stops the daemon at start with the SQLite error. The
daemon does not repair or delete it, because the graph is the only record of
what the snapshots on disk mean.

## Fields

| Field | Type | Default | Use |
|---|---|---|---|
| `-addr` | string | `127.0.0.1:7099` | Loopback address to listen on. A non-loopback address is refused. |
| `-data-dir` | path | `$HOME/.local/share/snapshot` | Holds `snapshots/` and `snapshot.db`. |
| `-backend` | string | `auto` | `auto`, `btrfs`, `copy`, `apfs` or `vss`. `auto` picks Btrfs when it runs here, otherwise `copy`. A named backend that cannot run is an error. |
| `-auto-keep` | integer | `50` | Auto snapshots kept per workset. `0` turns retention off. A backend whose provider holds fewer wins: on Windows the shadow storage cap decides, and the daemon logs which limit is binding. |
| `-prune-interval` | duration | `10m` | How often retention runs. `0` turns the timer off. |
| `-verbose` | boolean | `false` | Log at debug level. |

## Environment variables

| Variable | Replaces | Use |
|---|---|---|
| `SNAPSHOT_ADDR` | `-addr` | The daemon and `snapctl` both read it. |
| `SNAPSHOT_BACKEND` | `-backend` | Pin a backend for a service unit. |
| `SNAPSHOT_DATA_DIR` | `-data-dir` | Move the data directory. |
| `SNAPSHOT_BTRFS_TEST_ROOT` | none | The live Btrfs gate reads it. See VERIFICATION.md. |

A flag beats an environment variable. An environment variable beats the
default.

## Workset paths

`POST /worksets` stores the resolved spelling of every path, not the spelling
the caller sent. A symlink resolves to its target. On Windows an 8.3 short
name such as `C:\Users\RUNNER~1\work` resolves to `C:\Users\runneradmin\work`.

Every later answer uses the stored spelling. A caller that compares a diff
path against the string it declared must resolve its own path first.
