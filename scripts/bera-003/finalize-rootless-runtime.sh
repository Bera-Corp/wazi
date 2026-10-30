#!/usr/bin/env bash
set -euo pipefail
umask 077

test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
run_id="${1:-}"
[[ "$run_id" =~ ^[0-9]{8}T[0-9]{6}Z$ ]]

image=/srv/wazi-runtime.img
mount_dir=/srv/wazi-runtime
home_dir="$mount_dir/home"
wrapper_dir="$home_dir/.local/libexec/wazi-rootless-cli"
root_project_id=30000
root_project_limit=1024M
expected_uuid=a1609a69-6322-451a-a168-be87eadec878
runtime_user=wazi-runtime

test "$(stat -c %s "$image")" -eq 107374182400
test "$(blkid -s UUID -o value "$image")" = "$expected_uuid"
test "$(findmnt -T "$mount_dir" -n -o FSTYPE)" = ext4
case ",$(findmnt -T "$mount_dir" -n -o OPTIONS)," in
  *,prjquota,*) ;;
  *) echo "runtime filesystem is not mounted with prjquota" >&2; exit 1 ;;
esac
grep -Fq "$image $mount_dir ext4 loop,prjquota,noatime,nofail 0 2" /etc/fstab
test -f "/var/backups/fstab.wazi-runtime.${run_id}"
test "$(id -u "$runtime_user")" -eq 1002
test "$(stat -c '%U:%G:%a' "$home_dir")" = "$runtime_user:$runtime_user:700"
test -d "$home_dir/.local/share/docker"
test -S "/run/user/$(id -u "$runtime_user")/docker.sock"

root_project="$(lsattr -pd "$mount_dir")"
home_project="$(lsattr -pd "$home_dir")"
printf '%s\n' "$root_project" | awk 'NR == 1 && $1 == "0" && $2 !~ /P/ { ok = 1 } END { exit !ok }'
printf '%s\n' "$home_project" | awk -v id="$root_project_id" 'NR == 1 && $1 == id && $2 ~ /P/ { ok = 1 } END { exit !ok }'
quota_report="$(repquota -P -s "$mount_dir" 2>&1 || true)"
printf '%s\n' "$quota_report" | awk -v limit="$root_project_limit" '$1 == "#30000" && $5 == limit { ok = 1 } END { exit !ok }'
free_mib="$(df -Pm "$mount_dir" | awk 'NR == 2 { print $4 }')"
test "$free_mib" -ge 98304

systemctl is-active --quiet docker.service
test "$(docker version --format '{{.Server.Version}}')" = 29.1.3
test "$(docker info --format '{{.DockerRootDir}}')" = /var/lib/docker
rootful_containers_before="$(docker ps -aq)"
rootful_volumes_before="$(docker volume ls -q)"
if [ -n "$rootful_containers_before" ] || [ -n "$rootful_volumes_before" ]; then
  echo "existing rootful Docker workloads or volumes appeared; refusing finalization" >&2
  exit 1
fi
rootful_objects_before="$( { docker ps -aq; docker volume ls -q; } | sort | sha256sum | awk '{ print $1 }')"

rootful_link_before="$(ip -o link show | sha256sum | awk '{ print $1 }')"
rootful_routes_before="$( { ip route show table all; ip -6 route show table all; } | sha256sum | awk '{ print $1 }')"
rootful_listeners_before="$(ss -lnt | tail -n +2)"

mkdir -p "$wrapper_dir"
chown root:root "$home_dir/.local/libexec"
chmod 0755 "$home_dir/.local/libexec"
chown root:root "$wrapper_dir"
chmod 0755 "$wrapper_dir"
if [ -e "$wrapper_dir/docker" ] || [ -L "$wrapper_dir/docker" ]; then
  test "$(readlink -f "$wrapper_dir/docker")" = /usr/bin/docker
else
  ln -s /usr/bin/docker "$wrapper_dir/docker"
fi
if [ -e "$wrapper_dir/dockerd-rootless.sh" ] || [ -L "$wrapper_dir/dockerd-rootless.sh" ]; then
  test "$(readlink -f "$wrapper_dir/dockerd-rootless.sh")" = /usr/share/docker.io/contrib/dockerd-rootless.sh
else
  ln -s /usr/share/docker.io/contrib/dockerd-rootless.sh "$wrapper_dir/dockerd-rootless.sh"
fi

runtime_uid="$(id -u "$runtime_user")"
as_runtime() {
  runuser -u "$runtime_user" -- env \
    HOME="$home_dir" \
    XDG_RUNTIME_DIR="/run/user/${runtime_uid}" \
    DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/${runtime_uid}/bus" \
    PATH="$wrapper_dir:/usr/bin:/bin" \
    "$@"
}

as_runtime /usr/share/docker.io/contrib/dockerd-rootless-setuptool.sh install --force
as_runtime systemctl --user is-active --quiet docker.service
as_runtime systemctl --user is-enabled --quiet docker.service

rootless_info="$(as_runtime docker --context rootless info --format 'SERVER={{.ServerVersion}} ROOT={{.DockerRootDir}} SECURITY={{json .SecurityOptions}}')"
printf '%s\n' "$rootless_info"
case "$rootless_info" in
  *rootless*) ;;
  *) echo "selected Docker context does not report rootless security mode" >&2; exit 1 ;;
esac
expected_data_root="$home_dir/.local/share/docker"
actual_data_root="$(as_runtime docker --context rootless info --format '{{.DockerRootDir}}')"
test "$actual_data_root" = "$expected_data_root"
rootless_containers="$(as_runtime docker --context rootless ps -aq)"
rootless_volumes="$(as_runtime docker --context rootless volume ls -q)"
if [ -n "$rootless_containers" ] || [ -n "$rootless_volumes" ]; then
  echo "rootless daemon unexpectedly contains app containers or volumes" >&2
  exit 1
fi

rootful_socket_error="$(mktemp)"
if runuser -u "$runtime_user" -- env PATH=/usr/bin:/bin \
  docker -H unix:///var/run/docker.sock ps -aq >/dev/null 2>"$rootful_socket_error"; then
  echo "runtime identity unexpectedly accessed the rootful Docker socket" >&2
  rm -f -- "$rootful_socket_error"
  exit 1
fi
if ! grep -qi 'permission denied' "$rootful_socket_error"; then
  echo "rootful socket probe failed for a reason other than permission denial" >&2
  cat "$rootful_socket_error" >&2
  rm -f -- "$rootful_socket_error"
  exit 1
fi
rm -f -- "$rootful_socket_error"

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

printf 'RUNTIME_UID=%s\nRUNTIME_STORAGE_FREE_MIB=%s\n' "$runtime_uid" "$free_mib"
printf 'ROOTLESS_CONTEXT_READY=true\nROOTFUL_SOCKET_DENIED=true\n'
printf 'ROOTFUL_DOCKER_UNCHANGED=true\nHOST_NETWORK_INVENTORY_UNCHANGED_DURING_FINALIZATION=true\n'
