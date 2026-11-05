# Phase 3 candidate workflow — implementation slice

## Current checkpoint — 5 November 2026

The restricted Linux runner passed five recorded checks: candidate profile,
startup HTTP, SQLite creation, restart HTTP and synthetic-file persistence.
Its receipt is signed by the qualification-only key. The private store contains
candidate cand_244ed888373fa82d86d5778b698232dd with that passing receipt.
Owner review is pending and the candidate has not been promoted. The owner
deferred independent reviewer participation for this buildout. These dated
receipts do not replace the later hands-on contribution-to-install journey.
See the [Phase 1–5 closeout ledger](PHASE1_5_CLOSEOUT.md) for the current
phase estimate and remaining owner work.

## Status

The local control-plane implementation now stores unsigned package candidates
and their state transitions durably. The authenticated workbench has an import,
inspection, qualification-receipt, maintainer-review and signed-manifest
promotion flow.

This is a workflow foundation, not Phase 3 acceptance. The control plane
validates a signed qualification receipt; it does not launch or supervise the
isolated runner. Failed signed attempts remain in the append-only candidate
history and block review; a later passing receipt is required to continue.
Local regressions cover recovery when the release registry has persisted a
promotion before the candidate snapshot, and retries after the success response
is lost. They do not establish live runner or deployment acceptance.

## Tenant and session boundary

Candidate APIs require the existing private HTTPS context session, loopback
access, CSRF validation for writes, and the configured single maintainer
identity. The tenant scope is taken from the control plane's validated
capability-policy bundle. Callers cannot select another tenant in the request.

Enable the workflow only with the private pilot socket, mTLS, operator token,
manifest registry, approval registry, signer policy and a configured
maintainer identity. Configure a private candidate-store directory with
WAZI_CANDIDATE_STORE_DIR or the equivalent command-line flag. Also configure
WAZI_QUALIFICATION_SIGNERS as a comma-separated list of trusted signer IDs
reserved for qualification receipts. Startup rejects qualification signers
that are also named by the tenant policy as release-promotion signers.

The candidate store uses private append-only snapshots, one exclusive writer
lock, restart validation, and bounded record and disk capacity. A stale lock
fails startup for operator inspection.

## API flow

- GET /v1/package-candidates lists only summaries for the configured tenant.
- POST /v1/package-candidates imports an unsigned packet with a manifest,
  source IDs/URLs/content hashes, citation IDs and assisted-origin digest.
- GET /v1/package-candidates/{candidateID} returns that tenant's complete
  candidate and recorded evidence.
- POST /v1/package-candidates/{candidateID}/qualification records a signed
  runner receipt.
- POST /v1/package-candidates/{candidateID}/review records the configured
  maintainer's decision for the exact candidate digest.
- POST /v1/package-candidates/{candidateID}/promotion accepts an already
  signed manifest, verifies it under the configured release-signing policy,
  checks that its content matches the reviewed candidate, and writes it to
  the verified manifest registry. Registry writes are idempotent; repeating a
  promotion after a lost response returns the same persisted candidate state.
  The control plane never signs the release.

Import accepts only schema-valid manifests with reviewState candidate and no
signature. The candidate digest covers the canonical manifest; the packet
digest also binds source metadata, citations and origin. The manifest's
provenance source ID and content hash must match one of the cited sources.
Known credential patterns are rejected. Source text is not retained.

Promotion compares the signed manifest after removing its signature and
restoring reviewState candidate. Any release-content change therefore needs a
new candidate, qualification and review. Signed procedures remain a separate
registry operation and must still match the package release before execution.

## Qualification receipt contract

Receipts use the canonical
schemas/candidate-qualification.schema.json contract and are signed with a
trusted qualification signer. They are bound to the candidate digest and
policy tenant. Passing and failed results must agree with the per-test
results. At most 16 attempts are retained per candidate; list responses show
the result and digest but omit receipt bodies. Receipts include:

- runner ID/version, suite digest, qualification ID and a valid start/end time;
- unique test IDs; a passing receipt requires all tests to pass, while a failed
  receipt requires at least one failed test;
- syntheticProject true, network deny-egress, liveVolumeAccess false and
  privileged false;
- explicit CPU, RAM and storage ceilings within the configured hard bounds.

The control plane verifies the signature and every required field. It cannot
independently prove that the named runner actually enforced those isolation
properties. The service does not execute candidate data or claim sandbox
execution from a receipt alone.

## Remaining Phase 3 acceptance

- Review the pending candidate as owner, then publish its signed manifest and
  required procedures through the existing trust path if approved.
- On the later validation pass, exercise tampering, wrong-signer, stale-digest,
  failed-test and cross-tenant receipt rejection.
- Install from the signed release without another authoring step and preserve
  the exact source, qualification, review, release, job and recovery evidence.
- Run the workbench against the live control plane and complete responsive,
  accessibility, browser-security and Hallmark rendered review.
- Record actual Meta AI tooling evidence separately. Codex assistance does not
  satisfy that judging pillar; the provider integration remains deferred.

The current candidate record does not establish independent-human quorum.
Independent reviewer participation is deferred for this buildout; owner
review, release signing and the live install journey remain open.

The 3 Nov adversarial pass found that cryptographic qualification trust could
also authorize release promotion. Promotion now requires the intersection of
tenant release authorization and provisioned signing keys; generic registry
intake excludes qualification-only keys. A negative HTTP regression confirms
that a runner-signed manifest cannot change the registry or reviewed state.
Valid multi-attempt candidates also reopen beyond the former 4,000-snapshot
limit; the bound now matches the maximum permitted transition history.
