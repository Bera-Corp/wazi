#!/usr/bin/env bash
set -euo pipefail

test "$(hostname)" = bera-003
printf 'UTC='; date -u +%Y-%m-%dT%H:%M:%SZ
printf 'HOST='; hostname -f
printf 'RUNTIME_IMAGE='; stat -c '%F %s bytes' /srv/wazi-runtime.img
printf 'RUNTIME_IMAGE_UUID='; blkid -s UUID -o value /srv/wazi-runtime.img
printf 'RUNTIME_MOUNT='; findmnt -T /srv/wazi-runtime -n -o SOURCE,FSTYPE,OPTIONS
printf 'RUNTIME_ROOT_PROJECT='; lsattr -pd /srv/wazi-runtime
if [ -d /srv/wazi-runtime/home ]; then
  printf 'RUNTIME_HOME_PROJECT='; lsattr -pd /srv/wazi-runtime/home
else
  printf 'RUNTIME_HOME_PROJECT=absent\n'
fi
printf 'RUNTIME_FREE_MIB='; df -Pm /srv/wazi-runtime | awk 'NR == 2 { print $4 }'
printf 'RUNTIME_PROJECT_QUOTAS_BEGIN\n'; repquota -P -s /srv/wazi-runtime || true
printf 'RUNTIME_PROJECT_QUOTAS_END\n'
printf 'RUNTIME_FSTAB='; grep -F /srv/wazi-runtime /etc/fstab || true
printf 'RUNTIME_ACCOUNT='; getent passwd wazi-runtime || true
printf 'ROOTLESS_USER_SERVICE='; systemctl --user status docker.service --no-pager 2>&1 | head -n 8 || true
printf 'ROOTFUL_SERVICE='; systemctl is-active docker.service
printf 'ROOTFUL_DOCKER_VERSION='; docker version --format '{{.Server.Version}}'
printf 'ROOTFUL_DOCKER_ROOT='; docker info --format '{{.DockerRootDir}}'
printf 'ROOTFUL_CONTAINER_IDS_BEGIN\n'; docker ps -aq
printf 'ROOTFUL_CONTAINER_IDS_END\n'
printf 'ROOTFUL_VOLUME_NAMES_BEGIN\n'; docker volume ls -q
printf 'ROOTFUL_VOLUME_NAMES_END\n'
