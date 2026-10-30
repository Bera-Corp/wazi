#!/usr/bin/env bash
set -euo pipefail
umask 077

test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
run_id="${1:-}"
[[ "$run_id" =~ ^[0-9]{8}T[0-9]{6}Z$ ]]

runtime_user=wazi-runtime
home_dir=/srv/wazi-runtime/home
mount_dir=/srv/wazi-runtime
runtime_uid="$(id -u "$runtime_user")"
project_id=30001
quota_kib=8192
volume_name="wz-qual-${run_id}"
expected_prefix="$home_dir/.local/share/docker/volumes/"
volume_created=false
quota_configured=false
volume_mount=
observed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

as_runtime() {
  runuser -u "$runtime_user" -- env \
    HOME="$home_dir" \
    XDG_RUNTIME_DIR="/run/user/${runtime_uid}" \
    DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/${runtime_uid}/bus" \
    PATH=/usr/bin:/bin \
    "$@"
}

cleanup() {
  status=$?
  trap - EXIT
  cleanup_failed=false
  if [ "$volume_created" = true ]; then
    if ! as_runtime docker --context rootless volume rm "$volume_name" >/dev/null; then
      echo "cleanup failed to remove qualification volume $volume_name" >&2
      cleanup_failed=true
    fi
  fi
  if [ "$quota_configured" = true ]; then
    if ! setquota -P "$project_id" 0 0 0 0 "$mount_dir"; then
      echo "cleanup failed to clear project quota $project_id" >&2
      cleanup_failed=true
    fi
  fi
  if [ -n "$volume_mount" ] && [ -e "$volume_mount" ]; then
    echo "cleanup left qualification volume path $volume_mount" >&2
    cleanup_failed=true
  fi
  if [ "$quota_configured" = true ]; then
    quota_after="$(repquota -P -s "$mount_dir" 2>&1 || true)"
    if printf '%s\n' "$quota_after" | grep -q "^#${project_id}[[:space:]]"; then
      echo "cleanup left project quota $project_id configured" >&2
      cleanup_failed=true
    fi
  fi
  if [ "$cleanup_failed" = true ]; then
    exit 1
  fi
  exit "$status"
}
trap cleanup EXIT

test "$(findmnt -T "$mount_dir" -n -o FSTYPE)" = ext4
case ",$(findmnt -T "$mount_dir" -n -o OPTIONS)," in
  *,prjquota,*) ;;
  *) echo "runtime filesystem is not mounted with prjquota" >&2; exit 1 ;;
esac
systemctl is-active --quiet docker.service
test "$(docker version --format '{{.Server.Version}}')" = 29.1.3
test "$(docker info --format '{{.DockerRootDir}}')" = /var/lib/docker
as_runtime systemctl --user is-active --quiet docker.service
context_endpoint="$(as_runtime docker --context rootless context inspect rootless --format '{{json .Endpoints.docker}}')"
rootless_info="$(as_runtime docker --context rootless info --format 'SERVER={{.ServerVersion}} ROOT={{.DockerRootDir}} SECURITY={{json .SecurityOptions}}')"
case "$rootless_info" in *rootless*) ;; *) echo "rootless mode not reported" >&2; exit 1 ;; esac
test -z "$(as_runtime docker --context rootless ps -aq)"
test -z "$(as_runtime docker --context rootless volume ls -q)"
test -z "$(docker ps -aq)"
test -z "$(docker volume ls -q)"

rootful_link_before="$(ip -o link show | sha256sum | awk '{ print $1 }')"
rootful_routes_before="$( { ip route show table all; ip -6 route show table all; } | sha256sum | awk '{ print $1 }')"
rootful_listeners_before="$(ss -lnt | tail -n +2)"
rootful_objects_before="$( { docker ps -aq; docker volume ls -q; } | sort | sha256sum | awk '{ print $1 }')"

quota_before="$(repquota -P -s "$mount_dir" 2>&1 || true)"
if printf '%s\n' "$quota_before" | grep -q "^#${project_id}[[:space:]]"; then
  echo "project quota ID is already present; refusing to reuse it" >&2
  exit 1
fi

as_runtime docker --context rootless volume create "$volume_name" >/dev/null
volume_created=true
driver="$(as_runtime docker --context rootless volume inspect --format '{{.Driver}}' "$volume_name")"
test "$driver" = local
volume_mount="$(as_runtime docker --context rootless volume inspect --format '{{.Mountpoint}}' "$volume_name")"
case "$volume_mount" in
  "$expected_prefix"*/_data) ;;
  *) echo "managed volume path is outside the selected rootless data root" >&2; exit 1 ;;
esac
test -d "$volume_mount"
chattr -p "$project_id" +P "$volume_mount"
setquota -P "$project_id" 0 "$quota_kib" 0 0 "$mount_dir"
quota_configured=true

volume_project="$(lsattr -pd "$volume_mount")"
printf '%s\n' "$volume_project" | awk -v id="$project_id" 'NR == 1 && $1 == id && $2 ~ /P/ { ok = 1 } END { exit !ok }'
quota_report="$(repquota -P -s "$mount_dir" 2>&1 || true)"
printf '%s\n' "$quota_report" | awk -v id="#${project_id}" -v limit="8192K" '$1 == id && $5 == limit { ok = 1 } END { exit !ok }'

probe_file="$volume_mount/project-quota-probe.bin"
runuser -u "$runtime_user" -- dd if=/dev/zero of="$probe_file" bs=1M count=7 conv=fsync status=none
within_bytes="$(stat -c %s "$probe_file")"
test "$within_bytes" -eq 7340032
overflow_error="$(mktemp)"
if runuser -u "$runtime_user" -- dd if=/dev/zero of="$probe_file" bs=1M count=2 oflag=append conv=notrunc,fsync status=none 2>"$overflow_error"; then
  echo "write beyond the 8 MiB project quota unexpectedly succeeded" >&2
  rm -f -- "$overflow_error"
  exit 1
fi
if ! grep -qi 'Disk quota exceeded' "$overflow_error"; then
  echo "over-limit write failed for a reason other than quota enforcement" >&2
  cat "$overflow_error" >&2
  rm -f -- "$overflow_error"
  exit 1
fi
overflow_message="$(grep -i 'Disk quota exceeded' "$overflow_error" | head -n 1)"
rm -f -- "$overflow_error"
at_limit_bytes="$(stat -c %s "$probe_file")"
test "$at_limit_bytes" -eq 7340032
quota_report="$(repquota -P -s "$mount_dir" 2>&1 || true)"
printf '%s\n' "$quota_report" | awk -v id="#${project_id}" -v limit="8192K" '$1 == id && $5 == limit { ok = 1 } END { exit !ok }'

rm -f -- "$probe_file"
as_runtime docker --context rootless volume rm "$volume_name" >/dev/null
volume_created=false
setquota -P "$project_id" 0 0 0 0 "$mount_dir"
quota_after="$(repquota -P -s "$mount_dir" 2>&1 || true)"
if printf '%s\n' "$quota_after" | grep -q "^#${project_id}[[:space:]]"; then
  echo "project quota $project_id remained after cleanup" >&2
  exit 1
fi
quota_configured=false
test -z "$(as_runtime docker --context rootless volume ls -q)"
test -z "$(as_runtime docker --context rootless ps -aq)"

rootful_objects_after="$( { docker ps -aq; docker volume ls -q; } | sort | sha256sum | awk '{ print $1 }')"
test "$rootful_objects_before" = "$rootful_objects_after"
test "$(docker version --format '{{.Server.Version}}')" = 29.1.3
test "$(docker info --format '{{.DockerRootDir}}')" = /var/lib/docker
systemctl is-active --quiet docker.service
rootful_link_after="$(ip -o link show | sha256sum | awk '{ print $1 }')"
rootful_routes_after="$( { ip route show table all; ip -6 route show table all; } | sha256sum | awk '{ print $1 }')"
rootful_listeners_after="$(ss -lnt | tail -n +2)"
test "$rootful_link_before" = "$rootful_link_after"
test "$rootful_routes_before" = "$rootful_routes_after"
test "$rootful_listeners_before" = "$rootful_listeners_after"
test ! -e "$volume_mount"

free_mib="$(df -Pm "$mount_dir" | awk 'NR == 2 { print $4 }')"
printf '%s\n' "$rootless_info"
printf 'OBSERVED_AT=%s\nCONTEXT_NAME=rootless\nCONTEXT_ENDPOINT=%s\n' "$observed_at" "$context_endpoint"
printf 'QUALIFICATION_VOLUME=%s\nVOLUME_DRIVER=%s\n' "$volume_name" "$driver"
printf 'VOLUME_MOUNTPOINT=%s\nPROJECT_ID=%s\nQUOTA_LIMIT_KIB=%s\n' "$volume_mount" "$project_id" "$quota_kib"
printf 'WRITE_WITHIN_QUOTA_BYTES=%s\nOVERFLOW_WRITE_BYTES=2097152\nOVERFLOW_ERROR=%s\n' "$within_bytes" "$overflow_message"
printf 'CLEANUP_CONFIRMED=true\nSTORAGE_FREE_MIB=%s\n' "$free_mib"
printf 'ROOTFUL_DOCKER_UNCHANGED=true\nHOST_NETWORK_INVENTORY_UNCHANGED=true\n'
