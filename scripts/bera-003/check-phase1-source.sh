#!/usr/bin/env bash
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
run_dir="$1"
archive_hash="$2"
case "$run_dir" in /home/wazi-dev/wazi-phase1-closure/20*T*Z) ;; *) exit 1 ;; esac
[[ "$archive_hash" =~ ^[0-9a-f]{64}$ ]]
cd "$run_dir"
printf '%s  source.tar.gz\n' "$archive_hash" | sha256sum -c -
tar -xzf source.tar.gz
export PATH=/usr/local/go/bin:/usr/bin:/bin
cd services
go version
go test ./... -count=1
CGO_ENABLED=1 go test -race ./internal/controlplane ./internal/pairing ./internal/agentclient ./internal/operationjournal ./internal/pilotqueue -count=1
go vet ./...
test -z "$(gofmt -l .)"
mkdir -p ../bin
for name in controlplane agent pilot-agent recordtool; do
  go build -trimpath -buildvcs=false -o "../bin/wazi-$name" "./cmd/$name"
done
sha256sum ../bin/*
printf 'NATIVE_CHECKS_COMPLETED_AT='
date -u +%Y-%m-%dT%H:%M:%SZ
