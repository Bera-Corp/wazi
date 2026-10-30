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
fstab_backup="/var/backups/fstab.wazi-runtime.${run_id}"
root_project_id=30000
root_project_limit_kib=1048576
subuid_start=231072
subuid_end=296607
partial_image_uuid=a1609a69-6322-451a-a168-be87eadec878

for package in docker.io uidmap quota rootlesskit slirp4netns fuse-overlayfs; do
  dpkg-query -W -f='${Status}' "$package" 2>/dev/null | grep -qx 'install ok installed'
done
test -x /usr/share/docker.io/contrib/dockerd-rootless-setuptool.sh
test -x /usr/share/docker.io/contrib/dockerd-rootless.sh
systemctl is-active --quiet docker.service
test "$(docker version --format '{{.Server.Version}}')" = 29.1.3
test "$(docker info --format '{{.DockerRootDir}}')" = /var/lib/docker

if getent passwd wazi-runtime >/dev/null; then
  echo "wazi-runtime already exists; refusing to reuse or reconfigure it" >&2
  exit 1
fi
if grep -Fq "$mount_dir" /etc/fstab; then
  echo "an fstab entry already references $mount_dir" >&2
  exit 1
fi
if [ -e "$fstab_backup" ] || [ -L "$fstab_backup" ]; then
  echo "fstab backup target already exists; refusing to overwrite: $fstab_backup" >&2
  exit 1
fi
if awk -F: -v start="$subuid_start" -v end="$subuid_end" \
  '($2 + 0) < end + 1 && (($2 + 0) + ($3 + 0)) > start { overlap = 1 } END { exit overlap ? 0 : 1 }' \
  /etc/subuid /etc/subgid; then
  echo "subordinate UID/GID range ${subuid_start}-${subuid_end} is already in use" >&2
  exit 1
fi

available_kib="$(df -Pk /srv | awk 'NR == 2 { print $4 }')"
minimum_available_kib=$(((98304 + 20480) * 1024))
if [ "$available_kib" -lt "$minimum_available_kib" ]; then
  echo "less than 116 GiB is available on the filesystem backing /srv" >&2
  exit 1
fi

rootful_link_before="$(ip -o link show | sha256sum | awk '{ print $1 }')"
rootful_routes_before="$( { ip route show table all; ip -6 route show table all; } | sha256sum | awk '{ print $1 }')"
rootful_listeners_before="$(ss -lnt | tail -n +2)"
rootful_objects_before="$( { docker ps -aq; docker volume ls -q; } | sort | sha256sum | awk '{ print $1 }')"

if [ -e "$image" ] || [ -L "$image" ] || [ -e "$mount_dir" ] || [ -L "$mount_dir" ]; then
  if [ ! -f "$image" ] || [ -L "$image" ] || ! mountpoint -q "$mount_dir"; then
    echo "partial runtime storage exists in an unexpected state; refusing to format or mount it" >&2
    exit 1
  fi
  test "$(stat -c %s "$image")" -eq 107374182400
  test "$(blkid -s UUID -o value "$image")" = "$partial_image_uuid"
  test "$(findmnt -T "$mount_dir" -n -o FSTYPE)" = ext4
  case ",$(findmnt -T "$mount_dir" -n -o OPTIONS)," in
    *,prjquota,*) ;;
    *) echo "partial runtime filesystem is not mounted with prjquota" >&2; exit 1 ;;
  esac
  echo "RESUMING_FORMATTED_FILESYSTEM_UUID=$partial_image_uuid"
else
  install -d -o root -g root -m 0700 "$mount_dir"
  truncate -s 100G "$image"
  chown root:root "$image"
  chmod 0600 "$image"
  mkfs.ext4 -F -m 0 -O project,quota "$image"
  mount -o loop,prjquota,noatime "$image" "$mount_dir"
  echo "CREATED_FORMATTED_FILESYSTEM_UUID=$(blkid -s UUID -o value "$image")"
fi

chmod 0711 "$mount_dir"
chattr -p 0 -P "$mount_dir"
install -d -o root -g root -m 0700 "$home_dir"
chattr -p "$root_project_id" +P "$home_dir"
setquota -P "$root_project_id" 0 "$root_project_limit_kib" 0 0 "$mount_dir"
repquota -P -s "$mount_dir"

free_mib="$(df -Pm "$mount_dir" | awk 'NR == 2 { print $4 }')"
if [ "$free_mib" -lt 98304 ]; then
  echo "qualified filesystem exposes less than the 96 GiB owner budget" >&2
  exit 1
fi

cp -p /etc/fstab "$fstab_backup"
printf '\n# Wazi Phase 1 rootless runtime storage\n%s %s ext4 loop,prjquota,noatime,nofail 0 2\n' \
  "$image" "$mount_dir" >> /etc/fstab
systemctl daemon-reload
findmnt -T "$mount_dir" -n -o SOURCE,FSTYPE,OPTIONS
printf 'STORAGE_FREE_MIB=%s\n' "$free_mib"

useradd --no-create-home --home-dir "$home_dir" --shell /usr/sbin/nologin \
  --comment 'Wazi rootless runtime' wazi-runtime
usermod --add-subuids "${subuid_start}-${subuid_end}" \
  --add-subgids "${subuid_start}-${subuid_end}" wazi-runtime
chown wazi-runtime:wazi-runtime "$home_dir"
chmod 0700 "$home_dir"
passwd -l wazi-runtime >/dev/null
loginctl enable-linger wazi-runtime

runtime_uid="$(id -u wazi-runtime)"
systemctl start "user@${runtime_uid}.service"
for attempt in $(seq 1 20); do
  if [ -S "/run/user/${runtime_uid}/bus" ]; then break; fi
  sleep 1
done
test -S "/run/user/${runtime_uid}/bus"

runuser -u wazi-runtime -- env \
  HOME="$home_dir" \
  XDG_RUNTIME_DIR="/run/user/${runtime_uid}" \
  DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/${runtime_uid}/bus" \
  PATH="/usr/share/docker.io/contrib:/usr/bin:/bin" \
  /usr/share/docker.io/contrib/dockerd-rootless-setuptool.sh install --force

rootless_info="$(runuser -u wazi-runtime -- env \
  HOME="$home_dir" \
  XDG_RUNTIME_DIR="/run/user/${runtime_uid}" \
  DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/${runtime_uid}/bus" \
  docker --context rootless info --format 'SERVER={{.ServerVersion}} ROOT={{.DockerRootDir}} SECURITY={{json .SecurityOptions}}')"
printf '%s\n' "$rootless_info"
case "$rootless_info" in
  *rootless*) ;;
  *) echo "selected Docker context does not report rootless security mode" >&2; exit 1 ;;
esac
expected_data_root="$home_dir/.local/share/docker"
actual_data_root="$(runuser -u wazi-runtime -- env \
  HOME="$home_dir" \
  XDG_RUNTIME_DIR="/run/user/${runtime_uid}" \
  DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/${runtime_uid}/bus" \
  docker --context rootless info --format '{{.DockerRootDir}}')"
test "$actual_data_root" = "$expected_data_root"
rootless_containers="$(runuser -u wazi-runtime -- env \
  HOME="$home_dir" \
  XDG_RUNTIME_DIR="/run/user/${runtime_uid}" \
  DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/${runtime_uid}/bus" \
  docker --context rootless ps -aq)"
if [ -n "$rootless_containers" ]; then
  echo "new rootless daemon unexpectedly contains containers" >&2
  exit 1
fi
rootless_volumes="$(runuser -u wazi-runtime -- env \
  HOME="$home_dir" \
  XDG_RUNTIME_DIR="/run/user/${runtime_uid}" \
  DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/${runtime_uid}/bus" \
  docker --context rootless volume ls -q)"
if [ -n "$rootless_volumes" ]; then
  echo "new rootless daemon unexpectedly contains volumes" >&2
  exit 1
fi

rootful_socket_error="$(mktemp)"
if runuser -u wazi-runtime -- docker -H unix:///var/run/docker.sock ps -aq \
  >/dev/null 2>"$rootful_socket_error"; then
  echo "runtime identity unexpectedly accessed the existing rootful Docker socket" >&2
  rm -f -- "$rootful_socket_error"
  exit 1
fi
if ! grep -qi 'permission denied' "$rootful_socket_error"; then
  echo "rootful Docker socket probe failed for a reason other than permission denial" >&2
  cat "$rootful_socket_error" >&2
  rm -f -- "$rootful_socket_error"
  exit 1
fi
rm -f -- "$rootful_socket_error"
printf 'ROOTFUL_SOCKET_DENIED=true\n'

systemctl is-active --quiet docker.service
test "$(docker version --format '{{.Server.Version}}')" = 29.1.3
test "$(docker info --format '{{.DockerRootDir}}')" = /var/lib/docker
rootful_objects_after="$( { docker ps -aq; docker volume ls -q; } | sort | sha256sum | awk '{ print $1 }')"
test "$rootful_objects_before" = "$rootful_objects_after"
rootful_link_after="$(ip -o link show | sha256sum | awk '{ print $1 }')"
rootful_routes_after="$( { ip route show table all; ip -6 route show table all; } | sha256sum | awk '{ print $1 }')"
rootful_listeners_after="$(ss -lnt | tail -n +2)"
if [ "$rootful_link_before" != "$rootful_link_after" ] ||
  [ "$rootful_routes_before" != "$rootful_routes_after" ] ||
  [ "$rootful_listeners_before" != "$rootful_listeners_after" ]; then
  echo "host link, route, or listening-port inventory changed during rootless setup" >&2
  exit 1
fi
printf 'ROOTFUL_DOCKER_UNCHANGED=true\nHOST_NETWORK_INVENTORY_UNCHANGED=true\n'
