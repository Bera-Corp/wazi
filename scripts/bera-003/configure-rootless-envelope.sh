#!/usr/bin/env bash
set -euo pipefail
umask 077

test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
run_id="${1:-}"
[[ "$run_id" =~ ^[0-9]{8}T[0-9]{6}Z$ ]]

runtime_user=wazi-runtime
home_dir=/srv/wazi-runtime/home
unit_dropin_dir="$home_dir/.config/systemd/user/docker.service.d"
unit_dropin="$unit_dropin_dir/10-wazi-resource-envelope.conf"
cpu_quota_percent=262.5
memory_max_mib=1536
memory_max_bytes=$((memory_max_mib * 1024 * 1024))
runtime_uid="$(id -u "$runtime_user")"

as_runtime() {
  runuser -u "$runtime_user" -- env \
    HOME="$home_dir" \
    XDG_RUNTIME_DIR="/run/user/${runtime_uid}" \
    DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/${runtime_uid}/bus" \
    PATH=/usr/bin:/bin \
    "$@"
}

systemctl is-active --quiet docker.service
test "$(docker version --format '{{.Server.Version}}')" = 29.1.3
test "$(docker info --format '{{.DockerRootDir}}')" = /var/lib/docker
as_runtime systemctl --user is-active --quiet docker.service
as_runtime docker --context rootless info >/dev/null
if [ -n "$(as_runtime docker --context rootless ps -aq)" ] ||
  [ -n "$(as_runtime docker --context rootless volume ls -q)" ]; then
  echo "rootless daemon has a container or volume; refusing to restart it" >&2
  exit 1
fi

rootful_link_before="$(ip -o link show | sha256sum | awk '{ print $1 }')"
rootful_routes_before="$( { ip route show table all; ip -6 route show table all; } | sha256sum | awk '{ print $1 }')"
rootful_listeners_before="$(ss -lnt | tail -n +2)"
rootful_objects_before="$( { docker ps -aq; docker volume ls -q; } | sort | sha256sum | awk '{ print $1 }')"

mkdir -p "$unit_dropin_dir"
chown "$runtime_user:$runtime_user" "$unit_dropin_dir"
chmod 0700 "$unit_dropin_dir"
expected_dropin="$(printf '[Service]\nCPUQuota=%s%%\nMemoryMax=%sM' "$cpu_quota_percent" "$memory_max_mib")"
if [ -e "$unit_dropin" ] || [ -L "$unit_dropin" ]; then
  test -f "$unit_dropin" && [ ! -L "$unit_dropin" ]
  test "$(cat "$unit_dropin")" = "$expected_dropin"
else
  printf '%s\n' "$expected_dropin" > "$unit_dropin"
  chown "$runtime_user:$runtime_user" "$unit_dropin"
  chmod 0600 "$unit_dropin"
fi

as_runtime systemctl --user daemon-reload
as_runtime systemctl --user restart docker.service
as_runtime systemctl --user is-active --quiet docker.service
as_runtime systemctl --user is-enabled --quiet docker.service

unit_properties="$(as_runtime systemctl --user show docker.service -p CPUQuotaPerSecUSec -p MemoryMax -p MemoryCurrent -p MemoryPeak)"
printf '%s\n' "$unit_properties"
cpu_quota_duration="$(printf '%s\n' "$unit_properties" | awk -F= '$1 == "CPUQuotaPerSecUSec" { print $2 }')"
memory_max="$(printf '%s\n' "$unit_properties" | awk -F= '$1 == "MemoryMax" { print $2 }')"
test "$cpu_quota_duration" = 2.625000s
test "$memory_max" -eq "$memory_max_bytes"

rootless_info="$(as_runtime docker --context rootless info --format 'SERVER={{.ServerVersion}} ROOT={{.DockerRootDir}} SECURITY={{json .SecurityOptions}}')"
printf '%s\n' "$rootless_info"
case "$rootless_info" in *rootless*) ;; *) echo "rootless mode not reported" >&2; exit 1 ;; esac
rootless_root="$(as_runtime docker --context rootless info --format '{{.DockerRootDir}}')"
test "$rootless_root" = "$home_dir/.local/share/docker"
test -z "$(as_runtime docker --context rootless ps -aq)"
test -z "$(as_runtime docker --context rootless volume ls -q)"

systemctl is-active --quiet docker.service
test "$(docker version --format '{{.Server.Version}}')" = 29.1.3
test "$(docker info --format '{{.DockerRootDir}}')" = /var/lib/docker
rootful_objects_after="$( { docker ps -aq; docker volume ls -q; } | sort | sha256sum | awk '{ print $1 }')"
test "$rootful_objects_before" = "$rootful_objects_after"
rootful_link_after="$(ip -o link show | sha256sum | awk '{ print $1 }')"
rootful_routes_after="$( { ip route show table all; ip -6 route show table all; } | sha256sum | awk '{ print $1 }')"
rootful_listeners_after="$(ss -lnt | tail -n +2)"
test "$rootful_link_before" = "$rootful_link_after"
test "$rootful_routes_before" = "$rootful_routes_after"
test "$rootful_listeners_before" = "$rootful_listeners_after"

printf 'ROOTLESS_AGGREGATE_CPU_QUOTA_PERCENT=%s\n' "$cpu_quota_percent"
printf 'ROOTLESS_AGGREGATE_MEMORY_LIMIT_MIB=%s\n' "$memory_max_mib"
printf 'ROOTFUL_DOCKER_UNCHANGED=true\nHOST_NETWORK_INVENTORY_UNCHANGED=true\n'
