# Phase 3 and 4 completion plan

**Status:** updated 5 November 2026. This plan records work and acceptance
evidence still required; neither phase is complete.

## Target outcome

Complete one supported package journey from cited upstream material through
isolated qualification, owner review, signed publication, installation,
operation and recoverable restore. Bind evidence to exact source, runner,
package, policy and application-release digests. Keep candidate authoring,
release signing, host execution and recovery authority separate.

Use the owner-supplied judging weights throughout prioritisation and rehearsal:
Impact 30%, Use of Meta AI Tooling 25%, Technical Depth & Execution 20%, and
Demo 25%. Engineering safety gates remain mandatory at every score.

## Current evidence boundary

- **Phase 3:** the isolated Linux runner passed its five recorded checks and
  produced a separately signed receipt. Candidate
  cand_244ed888373fa82d86d5778b698232dd is pending owner review and has not
  been promoted. Owner review, signed promotion and the hands-on workbench
  journey remain open. Independent reviewer participation is deferred for
  this buildout; owner approval and signatures remain required.
- **Phase 4:** Uptime Kuma has a recorded same-release Linux upgrade and
  age-encrypted backup/restore. Windows decrypted the off-host ciphertext and
  verified SQLite integrity; it did not restore or run the application.
  PostgreSQL/MySQL encrypted checkpoint code and MySQL disposable-database
  preflight exist, but their new paths remain untested. Engine-major migration,
  Nginx coverage and the full lifecycle/platform matrix remain open.
- **Deployment boundary:** Bera-003 is a private development host. A successful
  control-plane deployment there is not customer production acceptance.

See the detailed [Phase 3 workflow](PHASE3_CANDIDATE_WORKFLOW.md),
[Phase 4 checkpoint status](PHASE4_ENCRYPTED_BACKUPS.md), and
[development host record](DEVELOPMENT_ENVIRONMENTS.md).

## Work sequence

### Gate 1 — Freeze the supported journey and evidence contract

Use Uptime Kuma as the first new catalog adapter and end-to-end candidate.
The upstream 2.5.9 release resolves to
df601c58d9205562f34cfe5ef35264b341019d19, reports an MIT license, and publishes
dist.tar.gz with SHA-256
f979084bbc74dc80a142d91d09b807f0a4ed52c06830c0e44c36adc9e77c064b. Record the
beneficiary task, repeatable baseline, supported host matrix, rollback point,
resource and network policy, and evidence locations in the scope card. Build a
Wazi image from pinned source; do not use the mutable upstream :2 image tag.
Keep proposed, implemented, locally verified, CI verified and live states
distinct.

**Acceptance:** an owner-reviewed scope card names the package, exact source
digests, beneficiary, baseline method, supported platforms, policy profile and
success/failure evidence for every later gate. The current pin and closeout
dependencies are recorded in [the Phase 1–5 closeout ledger](PHASE1_5_CLOSEOUT.md).

**Pillars:** Impact 30% uses a measured task and honest sample limits. Demo 25%
gets a stable, narrow story before backend work expands.

### Gate 2 — Qualify a separate Phase 3 runner

Build the runner as a disposable, least-privilege boundary outside the control
plane. Use a dedicated qualification signer that cannot authorize release
promotion. Enforce synthetic-project-only inputs, denied egress, no live
volume access, no privileged mode, and hard CPU, memory, storage and time
limits. Bind each signed receipt to the candidate digest, runner and suite
digests, policy tenant, limits, test results and timestamps.

Exercise valid, failed, tampered, wrong-signer, stale-digest, cross-tenant and
limit-exceeded receipts. Confirm a rejection leaves candidate, review and
release state unchanged. A receipt's assertions alone are not proof that the
runner enforced isolation; retain runner configuration and execution evidence
from the isolated Linux environment.

**Acceptance:** one real signed receipt from the pinned runner qualifies the
selected candidate, and the negative cases are retained against the exact
candidate digest.

**Pillars:** Technical Depth & Execution 20% requires enforced isolation and
negative evidence. Demo 25% shows a clear qualification result and failure
state.

### Gate 3 — Produce, review and publish one assisted candidate

Capture official source URLs, content digests, licensing evidence, citations
and the assisted-origin digest. Keep secrets out of model context. The model
may produce a typed candidate and a minimal explanation; schema, policy,
runner results and people decide whether it advances. For this private
buildout, the owner defers independent reviewer participation. Keep owner
review and signatures required, and do not characterize this candidate as a
production release. Before any production release, review its human-quorum
requirements separately. Sign the reviewed manifest and required procedures
through the existing external trust path.

The owner previously deferred provider/model connection. Treat an accessible,
eligible Meta model and any spending approval as prerequisites; do not
represent Codex assistance as Meta model use. For the judging record, capture
the model identifier, redacted input provenance, typed output, bounded usage,
relevant before/after check and fresh rescan.

**Acceptance:** a reviewer can inspect provenance and qualification, reject a
candidate safely, and promote one unchanged reviewed digest. Install that
signed release without a second authoring step; preserve source, receipt,
review, signature, job, health and recovery evidence.

**Pillars:** Use of Meta AI Tooling 25% needs an actual material model action.
Technical Depth & Execution 20% requires independent authority and immutable
digest binding. Impact 30% measures the same user task before and after.

### Gate 4 — Close the Phase 4 lifecycle matrix

For the selected package, map every supported operation to a reviewed
procedure, checkpoint, approval, evidence record and recovery path:

- upgrade between pinned releases and validate version-specific migration;
- encrypted backup, restore verification and rollback checkpoint;
- canary manifest evaluation and emergency revocation;
- SBOM and vulnerability evidence tied to the signed release.

Keep a platform matrix for Linux, Windows/WSL2 and macOS Docker paths. Each
platform stays unsupported until its exact runtime, filesystem and failure
behavior are qualified. Do not infer parity from Linux results.

**Acceptance:** every claimed lifecycle operation has a repeatable success
case, a relevant denial/failure case and a recovery result bound to release
and evidence digests. Unsupported cells are explicit.

### Gate 5 — Prove encrypted recovery without risking live data

Provision instance-specific age identities through the protected operator
process. Keep private identities and recovery keys outside source control,
plans, receipts, chat and model context. Add the MySQL disposable-database
semantic preflight before target mutation. Define encryption and restore
coverage for Nginx configuration checkpoints separately.

On disposable databases, create and inspect PostgreSQL and MySQL ciphertext
checkpoints, restore with the correct identity, and reject wrong identity,
modified, truncated and over-bound files before target mutation. Store a
separate recovery recipient off-host; restore from it in an isolated
environment and record elapsed time, manual steps and result. Retain the
pre-restore checkpoint until the restored service passes its health checks.

**Acceptance:** a clean off-host recovery reproduces expected data for the
selected release, and every invalid-input case proves the target remained
unchanged. State measured recovery time and coverage limits.

### Gate 6 — Review, rehearse and close

Run the repository's native qualification and live acceptance procedures only
against the pinned source commit and disposable/package-specific state.
Reconcile every result into the phase documents and backlog. Resolve critical
security findings and open platform claims before closure. Independent reviewer
participation is deferred for this buildout and must not be described as complete.

Rehearse the owner's three-minute film brief and the live product journey.
Keep recorded, cached and replayed steps labelled. A green build, uploaded
brand asset or successful API request alone does not close either phase.

**Pillars:** Impact 30% shows the measured beneficiary outcome; Meta AI Tooling
25% shows a real eligible model contribution; Technical Depth & Execution
20% shows reproducible trust and recovery evidence; Demo 25% shows the whole
journey with clear approvals, health and failure handling.

## Phase exit criteria

Close Phase 3 only when the isolated runner, real signed qualification, owner
review, signed publication, install path and live workflow evidence all match
one pinned candidate and release. Record Meta-tooling status separately.

Close Phase 4 only when the supported lifecycle matrix is complete for every
claimed platform, invalid restore inputs are rejected before mutation, and an
off-host recovery is verified from protected key material. Keep any remaining
platform or application work explicitly partial.
