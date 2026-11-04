# Uptime Kuma image and runtime implementation

Implementation date: 4 November 2026. Build execution, testing, security review
and live acceptance are pending. No image digest or passing receipt is claimed.

## Gitea reference pipeline

The manual uptime-kuma-image workflow implements the reference image path:
pinned upstream source, Buildx provenance/SBOM, GitHub image attestation, source
predicate, immutable image/config identities, Docker archive digest, archive
attestation and retained transport artifacts. A separate qualification image
and suite identity are built in the same workflow. Qualification is not run.

prepare_package_build.py supports the pinned Uptime Kuma 2.5.9 commit and exact
upstream Dockerfile hash. External Docker stages use pinned base/helper digests.
The application uses upstream's rootless target with its slimmer base. Release
dist assets must match their published SHA-256 before bounded extraction.

Gitea has a verified signed upstream release. Uptime Kuma's source lock records
commitSignatureVerified false: exact pins and Wazi attestations do not establish
an upstream signing claim.

## Package and runtime

prepare_phase2_release.py now generates unsigned Uptime Kuma manifests and typed
procedures from image exports. Existing operator signing/publication remains
separate. Its package version is 2.5.9+wazi.1; procedure version is 1.2.0.
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
- Existing pilot volumes plus Uptime Kuma need about 26.125 GiB, above the
  current 25 GiB application-volume reservation. Before enabling it live, the
  owner must choose capacity reallocation or retirement of an existing retained
  volume. The code does not raise the current limit.
- Independent maintainer review, eligible Meta provider access/spend cap,
  runtime/off-host recovery identities and Mac access remain dependencies.
- Testing and review are deferred by the owner. These files have not been run
  or deployed. This selected-package buildout does not yet justify 90% across
  the full documented scope of every phase.
