#!/usr/bin/env bash
# Root operator helper: publish records, issue one approval, submit and
# poll one bounded pilot job. The operator token never leaves the host.
set -euo pipefail
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
set -a
. /etc/wazi-controlplane-dev/operator.env
set +a
: "${WAZI_OPERATOR_TOKEN:?operator token missing}"
CP=https://localhost:9443
CA=/etc/wazi-controlplane-dev/server.crt
AUTH="X-Wazi-Operator-Token: $WAZI_OPERATOR_TOKEN" # wazi-secret-scan: allow - value expands from the host-only operator env at runtime; no literal secret
CT="Content-Type: application/json"

cmd="${1:?command}"
case "$cmd" in
  publish-records)
    REVOCATION="${2:?signed revocation record}"
    for dir in /tmp/pilot-records/a /tmp/pilot-records/b; do
      curl -sk --cacert "$CA" -X POST "$CP/v1/manifests" -H "$AUTH" -H "$CT" \
        --data-binary "@$dir/manifest.json" | head -c 300; echo
      for proc in "$dir"/procedure-*.json; do
        curl -sk --cacert "$CA" -X POST "$CP/v1/procedures" -H "$AUTH" -H "$CT" \
          --data-binary "@$proc" | head -c 200; echo
      done
    done
    curl -sk --cacert "$CA" -X POST "$CP/v1/revocations" -H "$AUTH" -H "$CT" \
      --data-binary "@$REVOCATION" | head -c 300; echo
    ;;
  procedure)
    curl -sk --cacert "$CA" -X POST "$CP/v1/procedures" -H "$AUTH" -H "$CT" \
      --data-binary "@${2:?signed procedure record}"
    echo
    ;;
  revocation)
    curl -sk --cacert "$CA" -X POST "$CP/v1/revocations" -H "$AUTH" -H "$CT" \
      --data-binary "@${2:?signed revocation record}"
    echo
    ;;
  profile)
    curl -s --unix-socket /run/wazi-phase1/agent.sock -X POST http://pilot/profile \
      > /tmp/pilot-profile.json
    cat /tmp/pilot-profile.json | head -c 400; echo
    ;;
  get-profile)
    curl -sk --cacert "$CA" --cert /etc/wazi-phase1/identity/node.crt \
      --key /etc/wazi-phase1/identity/node.key "$CP/v1/host-profiles" \
      > /tmp/pilot-profiles.json
    cat /tmp/pilot-profiles.json | head -c 200; echo
    ;;
  approve)
    curl -sk --cacert "$CA" -X POST "$CP/v1/approvals" -H "$AUTH" -H "$CT" \
      --data-binary "@${2:?approval request}"
    echo
    ;;
  submit)
    curl -sk --cacert "$CA" -X POST "$CP/v1/deployments" -H "$AUTH" -H "$CT" \
      --data-binary "@${2:?job file}"
    echo
    ;;
  status)
    curl -sk --cacert "$CA" "$CP/v1/deployments/${2:?job id}" -H "$AUTH"
    echo
    ;;
  *) echo "unknown command" >&2; exit 2 ;;
esac
