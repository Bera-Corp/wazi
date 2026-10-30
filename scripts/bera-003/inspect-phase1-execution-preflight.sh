#!/usr/bin/env bash
set -euo pipefail

test "$(hostname)" = bera-003
runtime_user=wazi-runtime
runtime_home=/srv/wazi-runtime/home
runtime_uid="$(id -u "$runtime_user")"

printf 'HOST='
hostname -f
printf 'CONTROLPLANE_ACTIVE='
systemctl is-active wazi-controlplane-dev.service
printf 'CONTROLPLANE_USER='
systemctl show wazi-controlplane-dev.service -p User --value
printf 'CONTROLPLANE_ENV_FILES_BEGIN\n'
systemctl show wazi-controlplane-dev.service -p EnvironmentFiles --value
printf 'CONTROLPLANE_ENV_FILES_END\n'
printf 'CONTROLPLANE_ENV_NAMES_BEGIN\n'
sudo -n awk -F= '/^[[:space:]]*[A-Za-z_][A-Za-z0-9_]*=/ {
  name = $1
  gsub(/^[[:space:]]+|[[:space:]]+$/, "", name)
  print name
}' /etc/wazi-controlplane-dev/operator.env | sort
printf 'CONTROLPLANE_ENV_NAMES_END\n'
printf 'CONTROLPLANE_EXEC_START_BEGIN\n'
systemctl show wazi-controlplane-dev.service -p ExecStart --value |
  sed -E 's/(WAZI_OPERATOR_TOKEN=)[^ ;]+/\1<redacted>/g'
printf 'CONTROLPLANE_EXEC_START_END\n'

as_runtime() {
  sudo -n -u "$runtime_user" env \
    HOME="$runtime_home" \
    XDG_RUNTIME_DIR="/run/user/$runtime_uid" \
    PATH=/usr/bin:/bin \
    "$@"
}

printf 'ROOTLESS_DOCKER_INFO='
as_runtime docker --context rootless info \
  --format 'SERVER={{.ServerVersion}} ROOT={{.DockerRootDir}} SECURITY={{json .SecurityOptions}}'
printf 'ROOTLESS_CONTAINERS_BEGIN\n'
as_runtime docker --context rootless ps -a --no-trunc \
  --format '{{.ID}} {{.Image}} {{.Names}} {{.Status}}'
printf 'ROOTLESS_CONTAINERS_END\n'
printf 'ROOTLESS_VOLUMES_BEGIN\n'
as_runtime docker --context rootless volume ls --format '{{.Name}} {{.Driver}}'
printf 'ROOTLESS_VOLUMES_END\n'
printf 'ROOTLESS_CONFIG_CREDENTIAL_METADATA_BEGIN\n'
as_runtime python3 - <<'PY'
import json
from pathlib import Path

path = Path("/srv/wazi-runtime/home/.docker/config.json")
if not path.is_file():
    print("config=absent")
else:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        print("config=unreadable-or-invalid")
    else:
        print("config=present")
        print("authRegistryNames=" + ",".join(sorted(value.get("auths", {}))))
        print("credentialHelperNames=" + ",".join(sorted(value.get("credHelpers", {}))))
        print("credentialStoreConfigured=" + str(bool(value.get("credsStore", ""))).lower())
PY
printf 'ROOTLESS_CONFIG_CREDENTIAL_METADATA_END\n'
printf 'UTC='
date -u +%Y-%m-%dT%H:%M:%SZ
