#!/usr/bin/env sh
# The acceptance test from START.md section 10. It needs a btrfs filesystem.
#
#   export SNAPSHOT_BTRFS_TEST_ROOT=/mnt/btrfs/snapshot-test
#   ./verify/acceptance.sh
#
# It checks 5 claims on a 5 GB working set:
#   1. A snapshot completes in under 1 second.
#   2. A diff of 2 snapshots differing by 1 file returns in under 2 seconds.
#   3. A restore returns the working set to its exact prior file state.
#   4. The snapshot graph survives a daemon restart.
#   5. 100 sequential snapshots consume less than 100 MB of extra disk space.
set -eu

root=${SNAPSHOT_BTRFS_TEST_ROOT:?set it to a writable directory on a btrfs filesystem}
size_mb=${ACCEPTANCE_SIZE_MB:-5120}
addr=${ACCEPTANCE_ADDR:-127.0.0.1:7123}
cd "$(dirname "$0")/.."

work="$root/work"
data="$root/data"
bin="$root/bin"
fail=0

say()  { printf '\n== %s ==\n' "$1"; }
pass() { printf 'PASS  %s\n' "$1"; }
bad()  { printf 'FAIL  %s\n' "$1"; fail=1; }

cleanup() {
  [ -n "${daemon_pid:-}" ] && kill "$daemon_pid" 2>/dev/null || true
  wait 2>/dev/null || true
}
trap cleanup EXIT

say "build"
mkdir -p "$bin"
go build -o "$bin/snapshotd" ./cmd/snapshotd
go build -o "$bin/snapctl" ./cmd/snapctl
export SNAPSHOT_ADDR="$addr"

say "make a $size_mb MB working set on btrfs"
btrfs subvolume delete "$work" 2>/dev/null || rm -rf "$work"
rm -rf "$data"
btrfs subvolume create "$work" >/dev/null
dd if=/dev/urandom of="$work/seed.bin" bs=1M count=10 status=none
i=0
while [ "$i" -lt $((size_mb / 10)) ]; do
  cp "$work/seed.bin" "$work/file$i.bin"
  i=$((i + 1))
done
sync
printf 'working set: %s\n' "$(du -sh "$work" | cut -f1)"

start_daemon() {
  "$bin/snapshotd" -addr "$addr" -data-dir "$data" -backend btrfs >"$root/daemon.log" 2>&1 &
  daemon_pid=$!
  until curl -sf --noproxy 127.0.0.1 "http://$addr/healthz" >/dev/null 2>&1; do
    kill -0 "$daemon_pid" 2>/dev/null || { echo "daemon died:"; cat "$root/daemon.log"; exit 1; }
  done
}

say "start the daemon"
start_daemon
"$bin/snapctl" workset acceptance "$work"

say "1. a snapshot completes in under 1 second"
first_json=$("$bin/snapctl" -json snapshot acceptance "acceptance base")
first=$(printf '%s' "$first_json" | tr -d ' \n' | sed 's/.*"id":"\([^"]*\)".*/\1/')
took=$(printf '%s' "$first_json" | tr -d ' \n' | sed 's/.*"duration_ms":\([0-9]*\).*/\1/')
printf 'snapshot of %s MB took %s ms\n' "$size_mb" "$took"
[ "$took" -lt 1000 ] && pass "snapshot under 1 s ($took ms)" || bad "snapshot took $took ms, want under 1000"

say "2. a diff of 2 snapshots differing by 1 file returns in under 2 seconds"
dd if=/dev/urandom of="$work/file1.bin.tmp" bs=1M count=10 status=none
mv "$work/file1.bin.tmp" "$work/file1.bin"
second=$("$bin/snapctl" snapshot acceptance "one file changed" | cut -d' ' -f1)
diff_start=$(date +%s%N)
diff_out=$("$bin/snapctl" diff "$first" "$second")
diff_ms=$((($(date +%s%N) - diff_start) / 1000000))
printf 'diff took %s ms\n%s\n' "$diff_ms" "$diff_out"
[ "$diff_ms" -lt 2000 ] && pass "diff under 2 s ($diff_ms ms)" || bad "diff took $diff_ms ms, want under 2000"
[ "$(printf '%s' "$diff_out" | grep -c '^M ')" = "1" ] && pass "diff names exactly 1 modified file" \
  || bad "diff did not name exactly 1 modified file"

say "3. a restore returns the working set to its exact prior file state"
before=$(cd "$work" && find . -type f -exec md5sum {} + | sort | md5sum)
dd if=/dev/urandom of="$work/wrecked.bin" bs=1M count=10 status=none
rm -f "$work/file2.bin"
"$bin/snapctl" restore "$second" >/dev/null
after=$(cd "$work" && find . -type f -exec md5sum {} + | sort | md5sum)
[ "$before" = "$after" ] && pass "the working set matches its prior state exactly" \
  || bad "the restored working set differs from its prior state"

say "4. the snapshot graph survives a daemon restart"
graph_before=$("$bin/snapctl" list acceptance | wc -l)
kill "$daemon_pid"; wait "$daemon_pid" 2>/dev/null || true
start_daemon
graph_after=$("$bin/snapctl" list acceptance | wc -l)
[ "$graph_before" = "$graph_after" ] && pass "the graph holds $graph_after nodes across a restart" \
  || bad "the graph held $graph_before nodes and now holds $graph_after"

say "5. 100 sequential snapshots consume less than 100 MB"
used_before=$(df -k --output=used "$root" | tail -1)
i=0
while [ "$i" -lt 100 ]; do
  "$bin/snapctl" snapshot acceptance "sequential $i" >/dev/null
  i=$((i + 1))
done
sync
used_after=$(df -k --output=used "$root" | tail -1)
grew_mb=$(( (used_after - used_before) / 1024 ))
printf '100 snapshots grew the filesystem by %s MB\n' "$grew_mb"
[ "$grew_mb" -lt 100 ] && pass "100 snapshots under 100 MB ($grew_mb MB)" \
  || bad "100 snapshots used $grew_mb MB, want under 100"

say "result"
if [ "$fail" = 0 ]; then
  echo "acceptance: all 5 claims hold"
else
  echo "acceptance: at least 1 claim failed"
fi
exit "$fail"
