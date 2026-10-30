#!/usr/bin/env bash
# Private development deployment only; application execution remains disabled.
set -euo pipefail
umask 077
test "$(hostname)" = bera-003
run_id="$1"
[[ "$run_id" =~ ^[0-9]{8}T[0-9]{6}Z$ ]]
run_dir="$(pwd)"
export PATH=/usr/local/go/bin:/usr/bin:/bin
test "$(go env GOOS)" = linux
cd services
go version
go test ./... -count=1
CGO_ENABLED=1 go test -race ./internal/controlplane ./internal/pairing ./internal/agentclient -count=1
go vet ./...
test -z "$(gofmt -l .)"
mkdir -p "$run_dir/bin"
go build -trimpath -buildvcs=false -o "$run_dir/bin/wazi-controlplane" ./cmd/controlplane
go build -trimpath -buildvcs=false -o "$run_dir/bin/wazi-agent" ./cmd/agent
sha256sum "$run_dir/bin/"*
cd "$run_dir"
sudo -n bash -s -- "$run_dir" "$run_id" <<'ROOT'
set -euo pipefail
umask 077
run_dir="$1"
run_id="$2"
account=wazi-controlplane-dev
config=/etc/wazi-controlplane-dev
state=/var/lib/wazi-controlplane-dev
release="/opt/wazi-dev/releases/$run_id"
if ! id "$account" >/dev/null 2>&1; then
  useradd --system --home-dir "$state" --no-create-home --shell /usr/sbin/nologin "$account"
fi
install -d -o root -g root -m 0755 /opt/wazi-dev /opt/wazi-dev/releases "$release"
install -m 0555 "$run_dir/bin/wazi-controlplane" "$run_dir/bin/wazi-agent" "$release/"
install -d -o root -g "$account" -m 0750 "$config"
install -d -o "$account" -g "$account" -m 0700 "$state"
if [ ! -f "$config/operator.env" ]; then
  printf 'WAZI_OPERATOR_TOKEN=%s\n' "$(openssl rand -hex 32)" > "$config/operator.env"
  chmod 0600 "$config/operator.env"
fi
if [ ! -f "$config/server.crt" ]; then
  openssl ecparam -name prime256v1 -genkey -noout -out "$config/server-ca.key"
  openssl req -new -x509 -sha256 -days 30 -key "$config/server-ca.key" -subj /CN=wazi-bera003-dev-server-ca -addext 'basicConstraints=critical,CA:TRUE' -addext 'keyUsage=critical,keyCertSign,cRLSign' -out "$config/server-ca.crt"
  openssl ecparam -name prime256v1 -genkey -noout -out "$config/server.key"
  openssl req -new -key "$config/server.key" -subj /CN=localhost -out "$config/server.csr"
  printf 'subjectAltName=DNS:localhost,DNS:bera-003,IP:127.0.0.1\nextendedKeyUsage=serverAuth\nkeyUsage=critical,digitalSignature\nbasicConstraints=critical,CA:FALSE\nauthorityKeyIdentifier=keyid,issuer\nsubjectKeyIdentifier=hash\n' > "$config/server.ext"
  openssl x509 -req -sha256 -days 7 -in "$config/server.csr" -CA "$config/server-ca.crt" -CAkey "$config/server-ca.key" -CAcreateserial -extfile "$config/server.ext" -out "$config/server.crt"
  openssl ecparam -name prime256v1 -genkey -noout -out "$config/pairing-ca.key"
  openssl req -new -x509 -sha256 -days 30 -key "$config/pairing-ca.key" -subj /CN=wazi-bera003-dev-pairing-ca -addext 'basicConstraints=critical,CA:TRUE' -addext 'keyUsage=critical,keyCertSign,cRLSign' -out "$config/pairing-ca.crt"
fi
for file in server.crt server.key server-ca.crt pairing-ca.crt pairing-ca.key; do
  test -s "$config/$file"
  chown root:"$account" "$config/$file"
  chmod 0640 "$config/$file"
done
chmod 0600 "$config/server-ca.key"
ln -s "$release" /opt/wazi-dev/current.next
mv -Tf /opt/wazi-dev/current.next /opt/wazi-dev/current
cat > /etc/systemd/system/wazi-controlplane-dev.service <<'UNIT'
[Unit]
Description=Wazi development pairing control plane (application execution disabled)
After=network.target

[Service]
Type=simple
User=wazi-controlplane-dev
Group=wazi-controlplane-dev
EnvironmentFile=/etc/wazi-controlplane-dev/operator.env
ExecStart=/opt/wazi-dev/current/wazi-controlplane -addr 127.0.0.1:9443 -tls-cert /etc/wazi-controlplane-dev/server.crt -tls-key /etc/wazi-controlplane-dev/server.key -ca-cert /etc/wazi-controlplane-dev/pairing-ca.crt -ca-key /etc/wazi-controlplane-dev/pairing-ca.key -registry-file /var/lib/wazi-controlplane-dev/registry.json
WorkingDirectory=/var/lib/wazi-controlplane-dev
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/wazi-controlplane-dev
CapabilityBoundingSet=
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
MemoryMax=128M
CPUQuota=25%
TasksMax=64
Restart=no
TimeoutStopSec=15

[Install]
WantedBy=multi-user.target
UNIT
systemd-analyze verify /etc/systemd/system/wazi-controlplane-dev.service
systemctl daemon-reload
systemctl restart wazi-controlplane-dev.service
systemctl is-active wazi-controlplane-dev.service
ROOT
printf 'DEPLOY_RUN=%s\n' "$run_id"
printf 'LOOPBACK_ENDPOINT=https://127.0.0.1:9443\n'
