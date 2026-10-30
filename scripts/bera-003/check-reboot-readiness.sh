#!/usr/bin/env bash
set -euo pipefail

test "$(hostname)" = bera-003
runtime_user=wazi-runtime
runtime_uid="$(id -u "$runtime_user")"
runtime_home=/srv/wazi-runtime/home
as_runtime() {
  sudo -n -u "$runtime_user" env \
    HOME="$runtime_home" \
    XDG_RUNTIME_DIR="/run/user/${runtime_uid}" \
    DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/${runtime_uid}/bus" \
    PATH=/usr/bin:/bin \
    "$@"
}

printf 'OBSERVED_AT='; date -u +%Y-%m-%dT%H:%M:%SZ
printf 'BOOT_ID='; cat /proc/sys/kernel/random/boot_id
for unit in ssh.service tailscaled.service docker.service wazi-controlplane-dev.service; do
  printf 'UNIT=%s ENABLED=' "$unit"
  systemctl is-enabled "$unit" 2>&1 || true
  printf 'UNIT=%s ACTIVE=' "$unit"
  systemctl is-active "$unit" 2>&1 || true
done
printf 'ROOTLESS_DOCKER_ENABLED='
as_runtime systemctl --user is-enabled docker.service
printf 'ROOTLESS_DOCKER_ACTIVE='
as_runtime systemctl --user is-active docker.service
printf 'ROOTLESS_CONTAINER_IDS_BEGIN\n'
as_runtime docker --context rootless ps -aq
printf 'ROOTLESS_CONTAINER_IDS_END\n'
printf 'ROOTLESS_VOLUME_NAMES_BEGIN\n'
as_runtime docker --context rootless volume ls -q
printf 'ROOTLESS_VOLUME_NAMES_END\n'
printf 'CONTROLPLANE_HEALTH='
curl --fail --silent --show-error --cacert /etc/wazi-controlplane-dev/server-ca.crt https://127.0.0.1:9443/healthz
