# Development environments

Status updated: 31 October 2026, Africa/Nairobi. This inventory records
development access and the bounded pilot, not production approval.

## Linux development host: bera-003

**The approved Gitea Phase 1 pilot is closed; expanded Phase 2 is in progress.**
The private mTLS listener is https://127.0.0.1:9443; its systemd service
is active and enabled. The one-shot inventory agent was tested from
the existing development runner account using separate product credentials.
Application execution is enabled only through reviewed pilot profiles and
case-bound approvals. Pairing acceptance and reboot recovery are recorded in
[Phase 1 evidence](PHASE1_LIVE_EVIDENCE.md). The older
[initial deployment record](BERA_003_LIVE_TEST_2026-10-30.md) is historical.

| Item | Verified state |
|---|---|
| Host | bera-003, reached over the existing private tailnet |
| Login | Dedicated wazi-dev development account |
| Authentication | Local Ed25519 key; noninteractive SSH succeeds |
| Sudo | Passwordless sudo verified with `sudo -n id` |
| OS / architecture | Ubuntu 26.04.1 LTS / Linux x86_64 |
| CPU observation | 8 available logical CPUs; approved Phase 2 owner cap 4.5 CPU, aggregate runtime cap 4 CPU |
| Memory observation | 7,163 MiB total; available memory is volatile. Approved owner cap 4,096 MiB, aggregate runtime cap 3,072 MiB |
| Storage allocation | Approved Phase 2 owner cap 48 GiB: application volumes 25 GiB, shared cache/checkpoints 13 GiB, protected reserve 10 GiB; runtime filesystem is separate from the host root |
| Tools | Python 3.14.4, Git 2.53.0, Docker Engine 29.1.3 / Compose 2.40.3 (daemon-verified); Go 1.23.12 and gcc 15.2.0 installed 30 Oct 2026 (below) |
| Boot configuration | SSH and tailscaled enabled; persistent Tailscale state present |
| Routing | IPv4/IPv6 forwarding enabled persistently; exit routes and the connected LAN subnet distributed to the tailnet |
| Device key expiry | Disabled in the latest Tailscale status |
| Restart evidence | Full reboot and unattended Gitea recovery qualified in Phase 1; journal bytes preserved |

Any new OS restart request requires a fresh maintenance/reconnection check;
the Phase 1 reboot receipt does not authorize another reboot.

### Qualification record — 30 October 2026

Installation choices (bootstrap, recorded per the scope notes):

- Docker Engine 29.1.3 and Compose 2.40.3 were already present
  (Ubuntu packages); the daemon reports Server 29.1.3.
- Go 1.23.12 (matching `services/go.mod`) was installed from the
  official `go.dev/dl/go1.23.12.linux-amd64.tar.gz` into
  `/usr/local/go`, verified against the published SHA-256
  `d3847fef834e9db11bf64e3fb34db9c04db14e068eeb064f49af747010454f90`.
  Ubuntu's `golang-go` (1.26) was not used, to stay on the module's
  pinned language line. Rollback: remove `/usr/local/go`.
- `gcc`/`libc6-dev` (15.2.0) were installed from Ubuntu apt for
  cgo race builds. Rollback: `apt-get remove gcc libc6-dev`.
- Services source was copied to the dedicated `~/wazi-qualification/`
  workspace only. No existing workloads, containers (none running),
  network, routing or Tailscale settings were altered, and no Wazi
  service was started.

Evidence obtained natively on this host:

- `CGO_ENABLED=1 go test -race ./internal/controlplane/
  ./internal/pairing/ ./internal/agentclient/ -count=1`: all three
  packages pass with no race reports (this is the native Linux
  race evidence the Windows workstation could not produce).
- `go test ./...` passes all packages; `go vet` passes natively
  and for `GOOS=windows`; `gofmt -l` is clean.
- The natively built `wazi-agent -tenant tenant_demo` (print-only,
  no control plane) produced a real profile: host_bera-003,
  Ubuntu 26.04, x86_64, 8 CPUs, 7,163 MiB RAM, 434,659 MiB free at
  the Docker-root filesystem (ext4), daemon 29.1.3, Compose
  2.40.3, 18 occupied ports. The profile validates against the
  canonical host-profile schema.
- `dockerRootWritable` reports false: the probe proves writability
  by writing as the agent user, and `wazi-dev` cannot write
  `/var/lib/docker` directly although the daemon (root) can.
  That is the intended fail-closed under-report; executor storage
  checks must go through the daemon, not this flag alone.

The owner-authorized contribution-budget revision 2 is recorded; it is a cap
and does not establish per-role runtime enforcement. The expired 14:32Z
managed-volume qualification was refreshed at 2026-10-30T17:02:12Z and expires
at 17:17:12Z; refresh it immediately before any later mutation. Still open:
the deliberate reboot/reconnect test, pairing acceptance, an unattended
least-privilege agent, and the signed application lifecycle.
Deployment remains disabled (501). The subsequent live development deployment
and its 26 passed checks are recorded in
[BERA_003_LIVE_TEST_2026-10-30.md](BERA_003_LIVE_TEST_2026-10-30.md).

## Agent discovery and connection

The ignored root `.env.local` holds host, port, user and the absolute path
to the existing private key. It contains no private key bytes or passphrase.
The checked-in [.env.example](../.env.example) is a portable template, not
a working credential. Do not copy any key into this repository, prompts,
deployment manifests, model context or shared research artifacts.

From the local Windows workspace:

```powershell
.\scripts\ssh-dev.ps1 -Check
.\scripts\ssh-dev.ps1 'hostname'
.\scripts\ssh-dev.ps1 'uname -sm'
```

[ssh-dev.ps1](../scripts/ssh-dev.ps1) parses connection metadata without
evaluating expressions. It requires an existing trusted host-key entry,
uses the configured key/SSH agent, disables agent forwarding, and propagates
SSH's exit code. If the key needs unlocking after a local restart, load it
with `ssh-add` in the local user's session. Do not work around a failed
host-key check by disabling verification.

The public-key fingerprint in `.env.local` identifies the client key; it is
not the server's host-key fingerprint. Server identity remains checked
against the local OpenSSH known-hosts store.

## Scope and next preparation

`wazi-dev` is an authorised development administrator. It is separate from
the future Wazi Node Agent identity, tenant keys and application capabilities.
Its SSH access is not a product API or evidence that Wazi exposes shell access.

- ~~Install pinned Go and supported Docker Engine/Compose versions~~
  done 30 Oct 2026; choices and rollback recorded above.
- A bounded CPU/RAM/storage contribution budget was recorded from the owner's
  delegated sizing instruction on 30 Oct 2026. Treat it as a cap, preserve the
  system reserve, and require authenticated runtime binding before using it.
- Use a dedicated Wazi workspace and named application data. Do not alter
  existing host workloads or network settings as an incidental test step.
- Re-probe before deployment and validate against actual runtime requirements.
- Verify signing, revocation, pairing and fresh-install contracts before
  running an application; the synthetic Phase 0 package remains a fixture.
- Plan a deliberate reboot/reconnect and unavailable/recovered-node test.
  Avoid disconnecting the only management path during initial setup.

Routing changes were authorised separately by the owner. Wi-Fi remains DHCP;
agents should use the stable tailnet endpoint in `.env.local`. Application
public exposure still requires its own product policy and approval.

## Phase 2 runtime qualification — 31 October 2026

The context worker uses the distribution-managed Python 3.14 runtime,
qualified separately from the pinned Python 3.11 development/CI environment.
Installed python3.14-venv and its pip/setuptools wheel dependencies during
setup; no existing packages were upgraded. The old pinned development rpds
and PyYAML packages were not installed into the service runtime. Installed
python3-rfc3339-validator 0.1.4 to activate mandatory timestamp validation.
Rollback of these development dependencies is removal of those newly installed
packages after checking that no other user depends on them.

The tested runtime versions and live acceptance receipts are recorded in
[the Phase 2 evidence matrix](PHASE2_LIVE_EVIDENCE.md). The context upgrade
preserves the Gitea runtime allocation and application volumes; its systemd
drop-ins and agent configuration have retained rollback copies.
