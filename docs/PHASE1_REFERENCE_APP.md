# Phase 1 reference app: Gitea

**Current pilot status — 31 October 2026:** A/B lifecycle, pairing acceptance,
reboot/service recovery and the declared live adversarial subset are recorded
in [PHASE1_LIVE_EVIDENCE.md](PHASE1_LIVE_EVIDENCE.md). Gitea A is serving on
bera-003. The committed implementation has passing remote CI; independent-human quorum,
automatic certificate renewal and automatic app recovery remain outside this
pilot acceptance. The dated build observations below are historical.

Status (30 October 2026): Bera-Corp's Gitea 28.2.0 image was rebuilt from the
verified upstream v28.2.0 source and its exact digest was bound by the Wazi
source-attestation workflow. A local, Wazi-signed pilot release manifest for
that digest passes the Wazi schema/signature verifier and Cosign blob
verification; details are in
releases/phase1/gitea/28.2.0/build-evidence.json. This satisfies the B
image/provenance portion of Gate 1 for that earlier digest. On commit
`3217408b0e6026638af4561276c6e8950cdd5d27`, release A run 41113830342 passed
provenance and archive-attestation checks for digest
`sha256:afd05701641f238dc198f98d7733b1d3b09cb6ae1a3f1cd9b3bc2058e706570b`, and
the refreshed B run 41113830385 passed for
`sha256:7b7f6f93ac9f4d0a50abd1c0c5f204631f0813723e13529d5e14754288d53ce2`.
These new A/B digests do not yet have matching Wazi-signed release records. The
pilot signer policy is not installed on bera-003, and image transfer/pull has
not been verified. The pilot has one approver and does not claim
independent-human quorum. A typed orchestration interface and durable journal
exist locally, but the rootless Docker boundary and control-plane dispatch are
not integrated. [Superseded 2026-10-30: integrated and executing for the pinned pilot; see [Phase 1 live evidence](PHASE1_LIVE_EVIDENCE.md).] The latest managed-volume qualification expired at 17:30:43Z.
A fresh read-only host preflight at 19:09:32Z found no rootless containers or
volumes, no configured GHCR pull credential, and no host mutation; see
`artifacts/bera-003/20261030T190932Z/phase1-execution-preflight.json`. Pairing
sign-off, the live digest-bound lifecycle and live denial matrix remain open.
Remote CI passed run 41113830567. The deployment endpoint remains HTTP 501 and
no Gitea workload has been created. [Superseded 2026-10-30: lifecycle, denial matrix, and reconciliation passed live; the generic deployment route stays disabled outside the pinned pilot.]

The owner-confirmed, dependency-ordered Phase 1 exit gates and their current
evidence are tracked in [BACKLOG.md](BACKLOG.md#owner-confirmed-phase-1-closure-order).
This document records the Gitea-specific lifecycle and acceptance details.

## Selection and candidate release

The owner selected Gitea as the Phase 1 stateful reference application. The
current upstream release observed for qualification is Gitea 28.2.0, published
on 18 October 2026. Use the exact rootless image tag
docker.gitea.com/gitea:28.2.0-rootless for the local contract candidate.
The registry observations below record candidate immutable digests, but release
provenance has not been verified. Before signing a Wazi manifest, verify a
cryptographic statement that binds the publisher and source to the exact
platform digest. A tag alone is not accepted as the image identity.

### Registry observation

On 30 October 2026, both `docker.gitea.com` and `ghcr.io/go-gitea/gitea` returned
the same OCI index digest for `28.2.0-rootless`:

```text
sha256:94807314454f6525994a3b76286ba12bc7e7f8cd14f61d9e67431767f8103306
```

The Linux amd64 image manifest in that index is
`sha256:dbfe22e3478e391f0b3bd2eac8df024657aa3c52da2af23ea77530e73caf5f7d`.
The registry returned a BuildKit SLSA v1 provenance record whose subject matches
that image digest and whose source revision is
`829b49c1ae96015e345f9d2a0eb1ea7acd5f31e1`; it names the Gitea GitHub Actions
build run `39151647093`. Its metadata says resolved-dependency completeness is
false. I did not verify a cryptographic signature over the container image or
this provenance record, so upstream image provenance remains an open check.
These registry digests are immutable pin candidates, not owner approval to pull
or run the image. A follow-up attempt to verify a signed upstream attestation
on 30 October found no OCI attestation through the queried registry endpoint;
the GitHub attestation query returned 404. The available SLSA record is not
cryptographically verified, so release provenance remains an open gate.

A second read-only check at 2026-10-30T16:06:31Z queried OCI referrers for the
same index digest at both docker.gitea.com and Docker Hub; both returned an
empty referrer list. Docker Hub's legacy digest-signature tag returned 404, and
the GitHub Attestations API returned 404 for the Linux amd64 image digest. The
pinned [Gitea v28.2.0 release workflow](https://github.com/go-gitea/gitea/blob/v28.2.0/.github/workflows/release-tag-version.yml)
signs release binaries in its binary job, while its container job builds and
pushes images without an image-signing or attestation step. The signed Git tag
and binary signatures therefore do not authenticate this container digest.
The image remains unqualified until an image-level provenance statement can
be cryptographically verified.

At 2026-10-30T16:30Z, `gh release verify v28.2.0 --repo go-gitea/gitea`
successfully verified GitHub's release attestation. The signed annotated tag
object is `1dc1ecdddcdc60ab14ec14e30e7bb4d84242f21f`; its peeled source commit
is `829b49c1ae96015e345f9d2a0eb1ea7acd5f31e1`. The build workflow checks both
identities explicitly. The release attestation's subjects cover the signed
source tag and release assets; none is the Gitea OCI index digest. The same
CLI's API lookup for the exact index digest returned 404, and
`--bundle-from-oci` reported no registry attestations. This verifies the
upstream source release and release files, not the container image, so Gate 1
remains open. See the [GitHub CLI release verification command](https://cli.github.com/manual/gh_release_verify).

Official upstream material supports this candidate:

- Gitea's upstream project states that the project is MIT-licensed.
- Its rootless Docker guide demonstrates the 28.2.0 rootless image, a
  single-service SQLite setup, and persistent data and configuration volumes.
- The current upstream web router exposes GET /api/healthz; the upstream
  health-check documentation specifies HTTP 200 on success.

Sources:

- [Gitea 28.2.0 release](https://github.com/go-gitea/gitea/releases)
- [Gitea 28.2.0 release checklist](https://github.com/go-gitea/gitea/issues/39850)
- [Gitea rootless Docker installation](https://docs.gitea.com/installation/install-with-docker-rootless/)
- [docker.gitea.com OCI manifest](https://docker.gitea.com/v2/gitea/manifests/28.2.0-rootless)
- [GHCR OCI manifest](https://ghcr.io/v2/go-gitea/gitea/manifests/28.2.0-rootless)
- [Gitea binary signature verification](https://docs.gitea.com/installation/install-from-binary#verify-signature)
- [Gitea project and license](https://github.com/go-gitea/gitea)
- [Gitea health-check route](https://github.com/go-gitea/gitea/blob/main/routers/web/web.go)
- [Gitea health-check response](https://docs.gitea.com/1.27/installation/install-on-kubernetes/)

## Persistent state and lifecycle

The qualification candidate uses SQLite and two named volumes:

- application data mounted at /var/lib/gitea;
- configuration mounted at /etc/gitea.

Both volumes are lifecycle state. Backup and restore evidence must cover both,
including the Gitea configuration and generated installation secrets. The
exact backup method must be qualified for SQLite consistency before a
stateful-upgrade rehearsal.

The live acceptance flow is release A install, create a known repository and
issue, check /api/healthz, take a consistent backup of both volumes, upgrade
to the independently verified and Wazi-signed release B, restore into clean
target volumes, verify the repository and issue through Gitea, and roll back
to release A using its exact image digest and the correct checkpoint. Every
stage must emit evidence bound to the exact A/B image digests, manifest,
procedure, approval, and deployment case. The release-A and release-B digests
remain unset until the provenance and Wazi-release gate passes.

## Owner gate before host mutation

The owner delegated numeric budget sizing with the instruction “Use as much
as is necessary.” Budget revision 2 is recorded at
`artifacts/bera-003/20261030T142519Z/contribution-budget-owner-authorized.json`
(local and Git-ignored; valid through 2026-10-31T10:18:12Z). It records a cap,
not a target to consume:

| Role or boundary | CPU cores | RAM (MiB) | Storage (MiB) |
|---|---:|---:|---:|
| Owner budget cap | 4 | 2048 | 98304 |
| Protected system reserve | 1 | 512 | 20480 |
| Gitea application, including SQLite | 2 | 1024 | 20480 |
| Backup helper | 0.125 | 96 | 20480 |
| Restore helper | 0.125 | 96 | 20480 |
| Upgrade temporary space | 0.125 | 128 | 10240 |
| Runtime overhead | 0.25 | 192 | 1024 |

SQLite is embedded in the Gitea application allocation; no separate database
service is budgeted. The allocations plus reserve total 3.625 CPU cores,
2,048 MiB RAM and 93,184 MiB storage, within the approved cap; 5,120 MiB
storage remains unallocated. The aggregate rootless service enforces 2.625 CPU
cores and 1,536 MiB RAM, leaving the 1 CPU / 512 MiB system reserve. SSH port
2222 is not required for this first lifecycle; health/API checks use HTTP
port 3000, and no host port opening is approved. The budget record does not
authenticate execution approval.

The latest live managed-volume record at
`artifacts/bera-003/20261030T170212Z/managed-volume-qualification.json`
qualified the rootless Docker local-volume path with an enforced ext4 project
quota until `2026-10-30T17:17:12Z`. It binds to the Unix socket
`/run/user/1002/docker.sock`, rootless data root
`/srv/wazi-runtime/home/.local/share/docker`, and 100,203 MiB observed free
space. Its disposable volume and project quota were removed after an 8 MiB
quota test. The earlier default-daemon preflight records remain
`not-qualified`, `preflight-only`, and expired. No app image, app volume,
container, host port, or workload has been deployed. Before an application
mutation, follow all five ordered gates in [the Phase 1 closure sequence](BACKLOG.md#owner-confirmed-phase-1-closure-order).
Refresh the volume qualification immediately before the approved rootless
boundary is used for live lifecycle acceptance.

## Local Phase 1 contract and renderer progress

The app-manifest schema has additive per-service CPU/RAM ceilings and per-volume
storage ceilings. The resource-bound Go preflight requires a ceiling for each
image service, checks CPU/RAM totals against the approved application
allocation and manifest minima, and checks managed-volume storage ceilings
against that allocation. The versioned `render_compose_v1.py` renderer
(`wazi-compose-renderer 0.3.0`) emits Compose CPU and exact-byte memory limits;
the Phase 0 renderer refuses these manifests instead of silently omitting the
limits. These changes are locally validated contracts. The typed runtime
adapter still must apply and inspect per-volume quotas and helper limits on
bera-003.

Paired mTLS policy-bundle retrieval is implemented in the local control-plane
and agent client. `PrepareExecution` now chains policy retrieval, exact signed
release plus revocation checks, rollback-procedure and authenticated
host-profile retrieval, resource binding, and one-job approval consumption
into a read-only prepared result. Focused Go tests pass. The control plane is
not configured with the policy and approval registries on bera-003, and no
typed runtime executor consumes the prepared result. The pilot has one
configured approver and does not claim independent-human quorum.

Local validation on 30 October passed 17 schemas, 14 examples, 17 fixtures and
12 adversarial fixtures; both Compose renderer suites and the Go contracts,
agent-client and control-plane packages passed. The complete Go suite passed
with package/test parallelism set to 1, and `go vet ./...` passed. The default
parallel Go run reported intermittent Windows temporary-directory cleanup
failures; the serialized run passed. This evidence does not qualify the signed
Gitea image or a live app deployment.

### bera-003 managed-volume preflight — 30 October 2026

A fresh read-only inspection at `2026-10-30T11:13:44Z` found Docker context
`default` at the local Unix socket, Docker 29.1.3, root `/var/lib/docker`, and
the `local` volume plugin advertised. The backing filesystem is
`/dev/nvme0n1p4`, ext4, with 434,210 MiB available. Its feature list does not
include `project`, the mount options do not enable project quotas, and neither
`quotaon` nor `xfs_quota` is installed. No Docker volumes existed at inspection
time. Docker reports cgroup v2 with CPU and memory controllers, but effective
application container limits have not been tested. No volume or workload was
created or modified.

The machine-readable record is
`artifacts/bera-003/20261030T111344Z/managed-volume-qualification.json`; it is
`not-qualified`, uses `preflight-only` enforcement, and has
`quotaEnforceable: false`. The `local` plugin listing is advertised capability,
not a live volume creation test. This observation does not change the host
profile's `dockerRootWritable: false` or approve runtime use. The numeric owner
budget is recorded, but authenticated consumption, enforceable storage
boundaries, and a least-privilege execution boundary remain open.

## Judging evidence

The Gitea lifecycle is evidence for the four owner-supplied judging pillars:

- **Impact 30%:** measure a beneficiary's baseline and show the time and
  recovery outcome for the complete lifecycle.
- **Meta AI Tooling 25%:** demonstrate one bounded, real Meta AI-assisted
  proposal with tests and rescan evidence; Gitea itself does not count as
  Meta AI usage.
- **Technical Depth & Execution 20%:** show signature, policy, revocation,
  resource-boundary, backup-consistency, data-verification, and rollback
  evidence.
- **Demo 25%:** rehearse the visible install-to-rollback journey with known
  repository data and clear approval and progress states.

These weights do not reduce the mandatory safety gates in the Phase 1
acceptance criteria.

## Read-only Phase 1 recheck — 30 October 2026 (historical snapshot)

The SSH check succeeded and `wazi-controlplane-dev.service` was active. An
empty JSON request to the loopback deployment route returned HTTP 501. The
rootless runtime container and named-volume inventories were empty. These
checks made no host changes. The recorded managed-volume qualification expired
at `2026-10-30T14:47:26Z`; it was refreshed after reboot at `17:15:43Z` and expired
at `17:30:43Z`. That disposable quota test made no application mutation.
At the time of this snapshot, the Gitea image lacked verified image
provenance and the Wazi release had not been signed. The later Gate 1 B-image
evidence is recorded at the top of this document.
