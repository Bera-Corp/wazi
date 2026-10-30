#!/usr/bin/env bash
# Activate the bounded Gitea pilot worker on bera-003. Root operator only.
# Preserves the previous controlplane unit and binary for rollback.
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
test "$(id -u)" -eq 0
STAMP="${1:?staging timestamp}"
SRC="/home/wazi-dev/wazi-phase1-closure/${STAMP}"

# Bootstrap only. Re-running this script must not replace a renewed node key
# with the historical developer-runner identity.
if [ -e /etc/wazi-phase1/identity/node.crt ]; then
  echo "pilot identity already exists; use the renewal/upgrade path instead of bootstrap" >&2
  exit 1
fi
test -x "$SRC/bin/wazi-controlplane"
test -x "$SRC/bin/wazi-pilot-agent"

install -d -m 0755 "/opt/wazi-dev/${STAMP}"
install -m 0755 "$SRC/bin/wazi-controlplane" "$SRC/bin/wazi-agent" \
  "$SRC/bin/wazi-pilot-agent" "$SRC/bin/wazi-recordtool" "/opt/wazi-dev/${STAMP}/"

CP_USER=wazi-controlplane-dev
install -d -o "$CP_USER" -g "$CP_USER" -m 0700 /var/lib/wazi-controlplane-dev/manifests
install -d -o "$CP_USER" -g "$CP_USER" -m 0700 /var/lib/wazi-controlplane-dev/approvals
install -m 0640 -o root -g "$CP_USER" "$SRC/releases/phase1/gitea/28.2.0/signing-policy.json" \
  /etc/wazi-controlplane-dev/signing-policy.json
install -m 0640 -o root -g "$CP_USER" /tmp/policy-bundle.json \
  /etc/wazi-controlplane-dev/policy-bundle.json

install -d -o wazi-runtime -g wazi-runtime -m 0700 /srv/wazi-runtime/pilot-state
install -d -o wazi-runtime -g wazi-runtime -m 0700 /etc/wazi-phase1/identity
IDENT=/home/wazi-dev/wazi-live/checks/20261030T064834Z/primary
install -m 0600 -o wazi-runtime -g wazi-runtime "$IDENT/node.key" /etc/wazi-phase1/identity/node.key
install -m 0640 -o wazi-runtime -g wazi-runtime "$IDENT/node.crt" /etc/wazi-phase1/identity/node.crt
install -m 0640 -o wazi-runtime -g wazi-runtime "$IDENT/server-ca.crt" /etc/wazi-phase1/identity/ca.crt
install -m 0640 -o wazi-runtime -g wazi-runtime "$IDENT/issuer.crt" /etc/wazi-phase1/identity/issuer.crt
install -m 0640 -o root -g wazi-runtime "$SRC/releases/phase1/gitea/28.2.0/signing-policy.json" \
  /etc/wazi-phase1/signing-policy.json
install -m 0640 -o root -g wazi-runtime /tmp/agent.json /etc/wazi-phase1/agent.json

usermod -aG wazi-runtime "$CP_USER"
printf '%s\n' 'wazi-runtime ALL=(root) NOPASSWD: /usr/local/libexec/wazi-phase1-quota' > /etc/sudoers.d/wazi-phase1-quota # wazi-secret-scan: allow - sudoers rule for the one fixed quota helper; no secret value
chmod 0440 /etc/sudoers.d/wazi-phase1-quota
visudo -cf /etc/sudoers.d/wazi-phase1-quota >/dev/null

cp -a /etc/systemd/system/wazi-controlplane-dev.service "/etc/systemd/system/wazi-controlplane-dev.service.bak-${STAMP}"
cp -a /etc/systemd/system/wazi-pilot-agent.service "/etc/systemd/system/wazi-pilot-agent.service.bak-${STAMP}" 2>/dev/null || true
install -m 0644 /tmp/wazi-controlplane-dev.service /etc/systemd/system/wazi-controlplane-dev.service
install -m 0644 /tmp/wazi-pilot-agent.service /etc/systemd/system/wazi-pilot-agent.service
systemctl daemon-reload
systemctl restart wazi-controlplane-dev.service
systemctl is-active --quiet wazi-controlplane-dev.service
echo "controlplane activated from /opt/wazi-dev/${STAMP}"
