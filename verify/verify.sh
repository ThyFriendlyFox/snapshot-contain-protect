#!/usr/bin/env sh
# The health gate. CI runs this exact command. See agent-kit/VERIFICATION.md.
set -eu

cd "$(dirname "$0")/.."

echo "== 1/5 format =="
unformatted=$(gofmt -l cmd internal)
if [ -n "$unformatted" ]; then
  echo "gofmt needs to run on:"
  echo "$unformatted"
  exit 1
fi

echo "== 2/5 vet =="
go vet ./...

echo "== 3/5 build =="
go build ./...

echo "== 4/5 test =="
go test ./...

echo "== 5/5 btrfs gate =="
if command -v btrfs >/dev/null 2>&1; then
  go test -tags btrfs_live ./internal/engine/ -run TestBtrfsLive -v
else
  echo "SKIPPED LOUDLY: btrfs tooling absent on this host; the btrfs backend is unproven here."
fi

echo "verify: green"
