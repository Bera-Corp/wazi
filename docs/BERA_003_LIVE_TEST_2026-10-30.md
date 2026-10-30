# bera-003 development deployment and live tests

Verified 30 October 2026, Africa/Nairobi. The owner authorized deployment
and actual testing. This is a private development deployment, not production
acceptance. Pairing acceptance remains open and application execution stays 501.

## Deployed state

- Service: wazi-controlplane-dev.service, active and enabled for normal boot.
- Endpoint: https://127.0.0.1:9443, loopback only; no firewall, routing,
  Tailscale, SSH or existing workload configuration was changed.
- Runtime account: wazi-controlplane-dev, system account with no login shell,
  no sudo and no Docker group. The service has NoNewPrivileges, a read-only
  system/home boundary and a private writable state directory.
- Pilot service limits: MemoryMax 128 MiB, CPUQuota 25% of one CPU, TasksMax 64.
  These bound this control plane; they are not an owner application contribution
  budget or a validated capacity promise.
- Initial tested release: /opt/wazi-dev/releases/20261030T064834Z.
  The browser-entry follow-up below records the newer release selected by
  /opt/wazi-dev/current. Binaries were built natively using Go 1.23.12.
- Configuration: /etc/wazi-controlplane-dev. Server and issuing CAs are
  distinct. The operator token is in a root-only environment file; CA private
  keys and credentials stayed on the host and were not copied into evidence.
- Registry: /var/lib/wazi-controlplane-dev/registry.json, private to the
  runtime account, with the single-writer lock enforced.
- Agent: installed as a one-shot binary. Live inventory/enrollment runs use
  wazi-dev as the supervised development runner, which already had Docker
  access. There is no unattended agent timer, and this does not establish a
  least-privilege production agent identity. No SSH/sudo keys enter Wazi identity.
- Final paired tenant: tenant_live_20261030T064834Z; node: host_bera-003.
  Product credentials remain in the private development test state directory.

## Evidence

The ignored artifacts/bera-003/20261030T064834Z directory contains:

- source-manifest.json: exact SHA-256 hashes of copied service source;
- deployment.json and deploy.log: archive identity, native tests/builds;
- results.json and live-checks.log: all 26 deployed-binary checks passed;
- host-profile.json and roundtrip-profile.json: real observations, independently
  validated against the canonical host-profile schema after retrieval;
- service-final.log: service identity/limits, loopback socket, binary hashes,
  key permissions, SSH/Tailscale active, no running application containers.

Final source archive SHA-256:
250744768bafa9cb0e8bd8c3b739f567597650b3afe552ed1f790a08b323e8a7

Deployed binary SHA-256:

- Agent: cc96c0cc432baec1db31e9e6533e19dd7056ab8678ef1a89b285cdc5ce095e40
- Control plane: 93250d6dff854dd1a469434aaa2fcc3bb971504e5d9c8690c5fff39ce08e84d0

## Verified behavior

| Path | Live result |
|---|---|
| Health | 200 |
| No client certificate on protected listing | 401 |
| No operator token on minting | 401 |
| Real agent CSR enrollment and inventory submission | Successful |
| Existing node identity reuse | Successful |
| Consumed-code replay | Rejected |
| Cross-tenant and cross-node submissions | 403 |
| Case alias and missing RAM field | 400 |
| Omitted ports and empty network object | Stored/listed without invalid null/empty enum; canonical schema passes |
| Two-phase renewal | Successful; old certificate 401, activated replacement 200 |
| Tenant listing isolation | Other tenant's host absent |
| Revoked node | 403 |
| Normal systemd restart | Successful; revocation still 403 |
| Spare code minted before revocation, retried after restart | Invalidated |
| Fresh-code re-pair | Successful |
| Application deployment before and after testing | 501 deployment_disabled |

The native full Go suite, core-package race tests, vet and formatting also
passed on the exact staged source before installation. This is independent
native evidence; it does not claim that remote GitHub CI ran.

## Issues discovered and corrected during this deployment

1. Initial development CA generation omitted explicit CA signing key usage.
   Python 3.14's strict TLS verification rejected it. The PKI setup now emits
   explicit critical CA/basic-constraint/key-usage extensions. Verification
   remains enabled; there is no insecure TLS bypass.
2. A normal SIGTERM stop left the registry lock behind. A real systemd restart
   failed with the single-writer diagnostic. The control-plane main now handles
   shutdown, drains HTTP and returns through registry cleanup, including startup
   error paths. A regression verifies writer release after TLS startup failure.
   The stale development lock was removed only after stopping the unit and
   verifying its recorded PID no longer existed. Subsequent deployed restart
   and persistent-revocation tests passed.

The first failure evidence remains in artifacts/bera-003/20261030T064501Z.

## Remaining limits and next tests

- dockerRootWritable is false under the development runner. This is the actual
  observation; direct Docker-root writes were not enabled or falsely reported.
- Inventory is in-memory in the control plane and is re-submitted after restart.
  Pairing/revocation state persists; profile persistence is a separate feature.
- Registry lock recovery after a crash/power loss remains operator-mediated.
  Never remove a lock without verifying that its writer is dead.
- Server certificate lifetime is seven days; development CA lifetime is 30 days;
  node credentials expire after 24 hours unless renewed. These are pilot defaults.
- The pending reboot was not performed. Reboot/reconnect and abrupt-crash
  recovery remain open. Owner-authorized contribution-budget revision 2 is
  recorded, while per-role enforcement and an unattended least-privilege agent
  remain open.
- There is no frontend or running reference application. Signature verification,
  approved execution, health, backup, restore and rollback remain gated work.
- This establishes engineering and a demo foundation. Impact 30%, Meta AI 25%,
  Engineering 20% and Demo 25% still require the beneficiary baseline, actual
  bounded Meta proposal and working review/approval/progress/application journey.

## Operating and rolling back

Use scripts/ssh-dev.ps1 for the existing verified SSH connection. Useful remote
commands are systemctl status wazi-controlplane-dev.service and, with sudo,
journalctl -u wazi-controlplane-dev.service. Do not dump the operator environment
file or private keys into logs.

To stop this development deployment while retaining evidence, credentials and
state, run sudo systemctl disable --now wazi-controlplane-dev.service.
There is no need to remove unrelated files, Docker resources or networking.

The reusable deployment and test sources are scripts/bera-003/deploy.ps1,
deploy.sh and live_checks.py. Running the checks deliberately mints/revokes
test identities and restarts only this development control-plane service.
Use a new timestamp/run ID for each fresh acceptance run.

## Browser entry follow-up — 30 October 2026

Opening the original HTTPS root returned 404 because the deployed service
provided API routes only. Plain HTTP on the TLS port returned 400 with
the HTTPS-required message. The browser entry point had not been covered
by the original live suite.

Release /opt/wazi-dev/releases/20261030T070238Z adds a public, static development
status page at the exact HTTPS root. It explains that pairing/inventory are
available, application execution remains disabled, and the application
dashboard is upcoming. It links to /healthz and exposes no tenant or
credential data. Use HTTPS; this change does not add plaintext transport.

Deployed checks pass: root 200 with HTML, health 200, unknown path 404,
protected inventory without a certificate 401, and application deployment 501.
Native tests, race tests, vet and formatting pass. Evidence is retained in
artifacts/bera-003/20261030T070238Z/browser-entry-checks.log and deploy.log.
The new control-plane binary SHA-256 is
d1bffcdad7b188d122aa4fe5f043ac3d6d233f145b7ce82117579ac6f7690fac.

## Brand assets follow-up — 30 October 2026

The owner-approved transparent wordmark and multi-resolution favicon were
deployed in release `/opt/wazi-dev/releases/20261030T081120Z`. The control
plane was built natively from the staged service source and restarted; the
existing agent binary and systemd policy were retained. This asset refresh did
not run the Go test, race, or vet suites. Application deployment remains
disabled (501).

The HTTPS root returned 200 with HTML, `/assets/favicon.ico` returned 200 as
an icon, and `/assets/wazi-wordmark-open.png` returned 200 as PNG. Source
archive SHA-256: `9689fddf3648820e11f6d0aea7bed351538eada7efd8dfc0fb469c67b1eddc1f`.
Control-plane binary SHA-256: `8e05a96e08fd04b65283f8b24eadb2d3b04aa455763e28c1000f08ce257eff2f`.

The service remains loopback-only on Bera-003 at `https://127.0.0.1:9443`.
A workstation SSH tunnel is listening only on `127.0.0.1:19443` for this
session; the browser URL is `https://localhost:19443/`. The TLS certificate
uses the private development CA, so a browser without that CA may show a trust
warning.

## Read-only managed-volume preflight — 30 October 2026

At `2026-10-30T09:50:27Z`, a read-only SSH inspection found Docker context
`default` using `unix:///var/run/docker.sock`, daemon `bera-003` running Docker
29.1.3, Docker root `/var/lib/docker`, and the `local` volume driver advertised.
The backing filesystem was `/dev/nvme0n1p4`, ext4, with 434,271 MiB available.
The `quotaon` utility was unavailable, and the ext4 feature list did not include
`project`. No Docker volume, container, image, port, or workload was created or
changed during this inspection. The qualification record at
`artifacts/bera-003/20261030T095027Z/managed-volume-qualification.json` is
`not-qualified` with `preflight-only` enforcement and `quotaEnforceable: false`.
The reported volume driver is not a live create/use test. The owner budget,
effective runtime limits, and least-privilege execution boundary remain open;
`dockerRootWritable: false` is unchanged.

## Read-only gate recheck and local acceptance — 30 October 2026

The SSH host check succeeded. At `2026-10-30T11:39:02Z`, a diagnostic POST
of an empty JSON object to the loopback `/v1/deployments` endpoint returned
HTTP 501 with `error: deployment_disabled`. No application deployment was
attempted. The request used `curl -k` because this check did not validate the
private development CA; it is endpoint status evidence only.

At the time of this `11:39:02Z` recheck, the latest managed-volume observation
was the `11:13:44Z` preflight above. It expired at `11:28:44Z`, was
`not-qualified`, and did not create or use a Docker volume. A later live
rootless-runtime qualification is recorded below.

After tightening the qualification contract so preflight-only storage cannot
be marked qualified, local checks passed: Python validation (17 schemas,
14 examples, 17 compatibility fixtures, 12 adversarial fixtures), all
Python adversarial, sprint, secret-scan and Compose-render suites, 64-file
JSON syntax gate, full Go suite, native/Linux/Windows `go vet`, `gofmt`,
Linux agent/control-plane cross-builds, and the Go profile round-trip schema
check. The native Linux Go race job could not run in this Windows environment
because CGO is disabled and no WSL distribution is installed. No hosted CI
run covers the uncommitted worktree.

## Runtime-boundary inventory — 30 October 2026

Read-only SSH inspection at `2026-10-30T11:47:42Z` found Docker 29.1.3 on
the default rootful daemon. `/var/lib/docker` is on `/dev/nvme0n1p4`, ext4,
with 434,200 MiB free; no Docker volumes existed. The local volume driver
is advertised, but this is not a live managed-volume test. The separate
`/dev/sda` disk contains NTFS partitions and was not touched.

At `2026-10-30T11:53:32Z`, the development control plane was active under
`wazi-controlplane-dev`; execution remains disabled. The host kernel is
7.0.0-42-generic with cgroup v2/systemd and unprivileged user namespaces
enabled. A dedicated `wazi-runtime` account, subordinate UID/GID ranges,
`newuidmap`/`newgidmap`, and rootless Docker setup tools were absent. The
rootful daemon remains active and unchanged; no runtime account, package,
filesystem, mount, service, or workload was created or modified.

## Read-only runtime gate refresh — 30 October 2026

At `2026-10-30T12:09:35Z`, SSH access and the separate development sudo
credential still worked. The `wazi-dev` login remains a member of the existing
`docker` group. The selected daemon's data root remains on `/dev/nvme0n1p4`
formatted as ext4 with mount options `rw,relatime`; `/srv/wazi-runtime.img`
does not exist. No `wazi-runtime` account or subordinate UID/GID ranges were
present, and `newuidmap`, `newgidmap`, `dockerd-rootless-setuptool.sh`,
`slirp4netns`, and `fuse-overlayfs` were not found. `docker volume ls -q`
returned no volumes. This read-only check confirms the existing rootful Docker
daemon is still the only available Docker boundary; no package, account,
filesystem, mount, daemon, network, volume, or workload was changed.

## Rootless runtime and managed-volume qualification — 30 October 2026

The isolated rootless Docker daemon started at `2026-10-30T12:27:47Z` under
the dedicated `wazi-runtime` UID 1002. It runs Docker 29.1.3 on the separate
ext4 loop filesystem `/srv/wazi-runtime.img` (UUID
`a1609a69-6322-451a-a168-be87eadec878`) mounted at `/srv/wazi-runtime` with
`prjquota`. The rootless data root is
`/srv/wazi-runtime/home/.local/share/docker`; the rootless security option is
reported. A 1 GiB hard project quota protects runtime metadata. The filesystem
reported 100,203 MiB free, above the 98,304 MiB owner storage cap.

The owner-authorized budget revision at
`artifacts/bera-003/20261030T142519Z/contribution-budget-owner-authorized.json`
keeps the cap at 4 CPU cores, 2,048 MiB RAM and 98,304 MiB storage. Its role
allocations are 2 CPU/1,024 MiB/20,480 MiB for Gitea, 0.125 CPU/96 MiB/20,480
MiB for each backup and restore helper, 0.125 CPU/128 MiB/10,240 MiB for
upgrade-temporary, and 0.25 CPU/192 MiB/1,024 MiB for runtime overhead. The
protected reserve remains 1 CPU/512 MiB/20,480 MiB. The aggregate rootless
service cgroup enforces 262.5% CPU and 1,536 MiB RAM; per-container role
limits still require executor integration.

At `2026-10-30T14:32:26Z`, the rootless daemon created a disposable `local`
named volume at its data root. Project ID 30001 was assigned an 8 MiB hard
quota. A 7 MiB write succeeded; a 2 MiB append was rejected with `Disk quota
exceeded` and left the file at 7 MiB. The volume was removed and the test
quota cleared. No application image, application container, persistent app
volume, host port or workload was created. The machine-readable
`volqual_bera003_rootless_20261030T143226Z` record is
`artifacts/bera-003/20261030T143226Z/managed-volume-qualification.json`; it
is `qualified` until `2026-10-30T14:47:26Z` for the observed local daemon and
filesystem.

The `wazi-runtime` identity was denied access to `/var/run/docker.sock`. The
existing rootful Docker 29.1.3 service and `/var/lib/docker` root remained
unchanged, its container and volume lists remained empty, and host link,
route and TCP-listener inventories were unchanged during runtime setup and
volume qualification. Rootless Docker is enabled in its user systemd manager
with linger. Reboot/reconnect behavior has not been exercised. The host is
still a development target; no production deployment is authorized by these
records.

## Refreshed rootless managed-volume qualification — 30 October 2026

The reusable `qualify-rootless-volume.sh` script passed again at
`2026-10-30T17:02:12Z` on bera-003. It bound the result to Docker 29.1.3,
context `rootless`, Unix endpoint `/run/user/1002/docker.sock`, and data root
`/srv/wazi-runtime/home/.local/share/docker`. The ext4 filesystem was mounted
with `prjquota` and reported 100,203 MiB free. A disposable named volume
received an 8 MiB hard project quota; a 7 MiB write succeeded, a further 2 MiB
was rejected with `Disk quota exceeded`, and the file remained 7 MiB. The
volume and project quota were removed and their absence verified. Rootful
container/volume inventories and host link, route, and listener inventories
were unchanged. No application image, persistent application volume,
container, host port, or workload was created.

The machine-readable record is
`artifacts/bera-003/20261030T170212Z/managed-volume-qualification.json` and
expires at `2026-10-30T17:17:12Z`. The run log is alongside it. The prior
16:33Z qualification expired at 16:48Z. `setquota` printed non-fatal warnings
that it could not stat mounted tmpfs devices; all exact ext4 quota,
over-limit denial, and cleanup assertions passed. The script now
refuses reused project quota IDs without clearing them and reports failed
cleanup instead of silently treating it as success. This is a fresh
qualification only; application execution remains gated by the Phase 1 exit
sequence.

## Reboot and reconnect acceptance — 30 October 2026

The host rebooted from boot ID 2584d9ef-6802-4b4a-a575-128b3e8d7be8 to
eae1e4f0-91c1-4b3c-8ac1-2b548f612323. SSH reconnected after two 10-second
connection timeouts. Post-boot checks at 2026-10-30T17:15:32Z confirmed
SSH, Tailscale, rootful Docker, the control plane, and the rootless Docker
user service enabled and active. The loopback control-plane health endpoint
returned status ok. Rootless container and named-volume inventories remained
empty. No application workload was mutated.

The observation is retained in
`artifacts/bera-003/20261030T171532Z/reboot-reconnect.json`.

## Post-reboot managed-volume qualification — 30 October 2026

The rootless managed-volume qualification passed again at
2026-10-30T17:15:43Z after reboot. The record binds Docker 29.1.3, the local
Unix socket, rootless data root, ext4 with prjquota, and 100,203 MiB free.
The disposable 8 MiB project quota accepted a 7 MiB write and rejected a
further 2 MiB write with Disk quota exceeded. The test volume and quota were
removed, rootful Docker state and host network inventory stayed unchanged,
and no app workload was created. The record expired at 17:30:43Z:
`artifacts/bera-003/20261030T171532Z/managed-volume-qualification.json`.
Four setquota warnings concerned mounted tmpfs devices; the targeted ext4
quota and cleanup assertions passed.
