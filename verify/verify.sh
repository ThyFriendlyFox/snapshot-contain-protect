#!/usr/bin/env sh
# The health gate. CI runs this exact command. See agent-kit/VERIFICATION.md.
set -eu

cd "$(dirname "$0")/.."

echo "== 1/6 format =="
unformatted=$(gofmt -l cmd internal)
if [ -n "$unformatted" ]; then
  echo "gofmt needs to run on:"
  echo "$unformatted"
  exit 1
fi

echo "== 2/6 vet =="
# A workflow that does not parse is invisible until GitHub rejects it, and
# the gate that would have caught it does not run. Check them here.
if command -v python3 >/dev/null 2>&1; then
  python3 - <<'PYEOF'
import glob, sys
try:
    import yaml
except ImportError:
    print("SKIPPED LOUDLY: no pyyaml; workflow files are unchecked")
    sys.exit(0)
bad = 0
for path in sorted(glob.glob(".github/workflows/*.yml")) + sorted(glob.glob(".github/*.yml")):
    try:
        yaml.safe_load(open(path))
    except Exception as err:
        print("%s does not parse: %s" % (path, err))
        bad = 1
sys.exit(bad)
PYEOF
else
  echo "SKIPPED LOUDLY: no python3; workflow files are unchecked"
fi
go vet ./... >/dev/null
echo "== 3/6 build =="
go build ./...

echo "== 4/6 test =="
go test ./...

echo "== 5/6 btrfs gate =="
# Two things are needed, and the tooling alone is not enough: a host can hold
# btrfs-progs on a kernel with no btrfs support at all.
if ! command -v btrfs >/dev/null 2>&1; then
  echo "SKIPPED LOUDLY: no btrfs command on this host; the btrfs backend is unproven here."
elif ! grep -qw btrfs /proc/filesystems 2>/dev/null; then
  echo "SKIPPED LOUDLY: btrfs-progs is installed but this kernel cannot mount btrfs; the btrfs backend is unproven here."
elif [ -z "${SNAPSHOT_BTRFS_TEST_ROOT:-}" ]; then
  echo "SKIPPED LOUDLY: this host can run btrfs, but SNAPSHOT_BTRFS_TEST_ROOT is not set."
  echo "                Set it to a writable directory on a btrfs filesystem to prove the backend."
else
  go test -tags btrfs_live ./internal/engine/ -run TestBtrfsLive -v
fi

echo "== 6/6 unprivileged gate =="
# The daemon's documented deployment is a normal user. As root every
# permission check passes, so the read-only-directory cases prove nothing.
if [ "$(id -u)" = "0" ] && command -v setpriv >/dev/null 2>&1; then
  unpriv=$(mktemp -d)
  chmod 777 "$unpriv"
  go test -c -o "$unpriv/engine.test" ./internal/engine
  go test -c -o "$unpriv/api.test" ./internal/api
  chmod 755 "$unpriv/engine.test" "$unpriv/api.test"
  for suite in engine api; do
    setpriv --reuid=65534 --regid=65534 --clear-groups \
      env TMPDIR="$unpriv" HOME="$unpriv" "$unpriv/$suite.test" -test.count=1 >"$unpriv/$suite.log" 2>&1 || {
        echo "unprivileged $suite suite failed:"
        tail -30 "$unpriv/$suite.log"
        rm -rf "$unpriv"
        exit 1
      }
    echo "ok  unprivileged $suite suite"
  done
  rm -rf "$unpriv"
elif [ "$(uname -s)" != "Linux" ] && [ "$(uname -s)" != "Darwin" ]; then
  echo "not a unix host; there are no permission bits to drop. Steps 1 to 4 covered what applies."
elif [ "$(id -u)" != "0" ]; then
  echo "already running as a normal user; steps 1 to 4 covered this."
else
  echo "SKIPPED LOUDLY: running as root with no setpriv; permission cases are unproven here."
fi

echo "verify: green"
