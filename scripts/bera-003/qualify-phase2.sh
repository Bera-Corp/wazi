#!/usr/bin/env bash
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
run_dir="$1"
archive_hash="$2"
case "$run_dir" in /home/wazi-dev/wazi-phase2/20*T*Z) ;; *) exit 1 ;; esac
[[ "$archive_hash" =~ ^[0-9a-f]{64}$ ]]
cd "$run_dir"
printf '%s  source.tar.gz\n' "$archive_hash" | sha256sum -c -
tar -xzf source.tar.gz
export PATH=/usr/local/go/bin:/usr/bin:/bin
cd services
go version
go test ./... -count=1
CGO_ENABLED=1 go test -race ./internal/controlplane ./internal/agentclient ./internal/pilotqueue -count=1
go vet ./...
GOOS=windows go vet ./...
test -z "$(gofmt -l .)"
mkdir -p ../bin
for name in controlplane pilot-agent agent recordtool; do
  go build -trimpath -buildvcs=false -o "../bin/wazi-$name" "./cmd/$name"
done
cd ..
# Bera-003's distribution-managed Python 3.14 runtime is qualified separately
# from the pinned Python 3.11 development/CI environment. Do not silently build
# old rpds/PyYAML pins against an unsupported interpreter.
python3 - <<'PY'
import importlib.metadata as metadata
expected = {'jsonschema':'4.19.2','referencing':'0.36.2','rpds-py':'0.27.1','PyYAML':'6.0.3','rfc3339-validator':'0.1.4'}
actual = {name:metadata.version(name) for name in expected}
assert actual == expected, ('runtime dependency versions changed', actual)
print('BERA003_RUNTIME_DEPENDENCIES', actual)
PY
for suite in validate test_adversarial test_sprints test_secret_scan test_render_compose test_render_compose_v1 test_context_engine test_context_live test_package_composition test_candidate_author test_prepare_package_build test_prepare_phase2_release; do
  python3 "scripts/$suite.py"
done
python3 scripts/secret_scan.py
printf 'QUALIFICATION_COMPLETED_AT='
date -u +%Y-%m-%dT%H:%M:%SZ
