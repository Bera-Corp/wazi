#!/usr/bin/env bash
set -euo pipefail
umask 077

test "$(hostname)" = bera-003
run_id="${1:-}"
[[ "$run_id" =~ ^[0-9]{8}T[0-9]{6}Z$ ]]

image="/tmp/wazi-project-quota-${run_id}.img"
mount_dir="/mnt/wazi-project-quota-${run_id}"
test ! -e "$image" && test ! -L "$image"
test ! -e "$mount_dir" && test ! -L "$mount_dir"
install -d -o root -g root -m 0700 "$mount_dir"

cleanup() {
  local status=$?
  trap - EXIT
  set +e
  if mountpoint -q "$mount_dir"; then
    if ! umount "$mount_dir"; then
      echo "quota probe cleanup could not unmount $mount_dir; preserving image" >&2
      exit 1
    fi
  fi
  if mountpoint -q "$mount_dir"; then
    echo "quota probe mount remains active; preserving image" >&2
    exit 1
  fi
  if [ -f "$image" ] && [ ! -L "$image" ]; then
    rm -f -- "$image"
  fi
  rmdir -- "$mount_dir"
  exit "$status"
}
trap cleanup EXIT

truncate -s 64M "$image"
mkfs.ext4 -F -m 0 -O project,quota "$image" >/dev/null
mount -o loop,prjquota "$image" "$mount_dir"
mkdir "$mount_dir/limited"
chown wazi-dev "$mount_dir/limited"
chmod 0700 "$mount_dir/limited"
chattr -p 42424 +P "$mount_dir/limited"
setquota -P 42424 0 8192 0 0 "$mount_dir"

runuser -u wazi-dev -- mkdir "$mount_dir/limited/child"
runuser -u wazi-dev -- dd if=/dev/zero of="$mount_dir/limited/child/within" bs=1M count=7 conv=fsync status=none
error_file="$mount_dir/over-limit.err"
if runuser -u wazi-dev -- dd if=/dev/zero of="$mount_dir/limited/child/over" bs=1M count=2 conv=fsync status=none 2>"$error_file"; then
  echo "project quota failed to reject a write beyond the 8 MiB hard limit" >&2
  exit 1
fi
if ! grep -qiE 'quota|No space left on device' "$error_file"; then
  echo "over-limit write failed for a reason other than quota enforcement" >&2
  cat "$error_file" >&2
  exit 1
fi
written_bytes="$(stat -c %s "$mount_dir/limited/child/over")"
if [ "$written_bytes" -ge 2097152 ]; then
  echo "over-limit write stored the full 2 MiB unexpectedly" >&2
  exit 1
fi

repquota -P -s "$mount_dir"
printf 'PROJECT_QUOTA_ENFORCED project_id=42424 hard_limit=8MiB over_limit_write=denied\n'
