# Uptime Kuma image and runtime implementation

Implementation began 4 November 2026. The current live and code status is
summarized below; the detailed dated receipts are in the
[Linux integration record](LINUX_INTEGRATION_2026-11-04.md).

## Current checkpoint — 5 November 2026

The attested Uptime Kuma image, signed 2.5.9+wazi.3 package and isolated Linux
runner receipt are recorded. The selected Linux instance completed install,
same-release upgrade, age-encrypted backup and restore. A transferred
ciphertext was decrypted on Windows and its recovered SQLite database passed
integrity verification; Windows did not run or restore the application.
Startup migration, canary and downgrade code exists but has no new execution
evidence in this closeout. No live host was queried during this pass.

## Gitea reference pipeline

The manual uptime-kuma-image workflow implements the reference image path:
pinned upstream source, Buildx provenance/SBOM, GitHub image attestation, source
predicate, immutable image/config identities, Docker archive digest, archive
attestation and retained transport artifacts. The separate qualification
image was built. The recorded five-check run used a suite derivative of the
verified application archive, not the registry qualification image.

prepare_package_build.py supports the pinned Uptime Kuma 2.5.9 commit and exact
upstream Dockerfile hash. External Docker stages use pinned base/helper digests.
The application uses upstream's rootless target with its slimmer base. Release
dist assets must match their published SHA-256 before bounded extraction.

Gitea has a verified signed upstream release. Uptime Kuma's source lock records
commitSignatureVerified false: exact pins and Wazi attestations do not establish
an upstream signing claim.

## Package and runtime

prepare_phase2_release.py initially generated unsigned Uptime Kuma manifests
and typed procedures from image exports. The signed package used for the live
pilot is 2.5.9+wazi.3; operator signing/publication remains a separate step.
prepare_phase2_transport.py accepts explicit run, attempt and profile inputs.

The reviewed profile defines 512 MiB RAM, 0.5 CPU, a 2 GiB managed data volume
at /app/data, no added application capabilities, and loopback port 13001 mapped
to container 3001. Slots 01/02 use the same port sequentially. Registry capacity,
composition validation, storage accounting and quota IDs include the new app.

prepare_uptime_instance.py generates inert private configuration from an
existing operator runtime template, signed manifest and exported image/config
digests. Protected backup identities and resource records remain references.
The generator does not establish trust, create keys, reserve space or install.

Backup stops Uptime Kuma, archives its complete volume through age encryption,
and attempts restart after success or failure. Restore authenticates/decrypts,
bounds tar contents, rejects escaping paths/special entries, stops the app,
extracts to its own volume, then uses the typed startup/health steps. Retained
pre-restore checkpoints support explicit recovery; partial extraction does not
automatically roll back.

stage_offhost_recovery.py verifies a separately retained ciphertext digest and
package bindings, decrypts within time/size limits, and writes a private staged
payload and receipt. Application restoration is a separate step.

## Remaining code and owner dependencies

- Uptime Kuma startup migration, checkpoint-aware downgrade and durable canary
  coordination now have an initial implementation described in
  [Lifecycle rollout](LIFECYCLE_ROLLOUT.md). Execution and validation remain pending.
- Database-engine major migrations and the full lifecycle/platform matrix remain
  incomplete; unqualified version changes continue to be rejected.
- On 4 November the owner authorized 27 GiB of application reservations,
  11 GiB shared cache/checkpoints and the existing 10 GiB reserve within the
  total 48 GiB cap. Revision 3 records this decision. PostgreSQL and Uptime
  Kuma must run sequentially under the unchanged 3 GiB / 4 CPU runtime cap;
  the owner approved temporary PostgreSQL stops with data preserved.
- Owner review and promotion of the pending candidate remain open. Independent
  reviewer participation is deferred for this buildout. Eligible Meta provider
  access/spend approval, protected recovery identities and Mac access remain
  separate owner dependencies.
- Further tests are deferred to the owner's later pass. The live Linux evidence
  above is bounded to the selected Uptime release and same-release lifecycle;
  it does not qualify database-engine conversions or the full OS matrix.
