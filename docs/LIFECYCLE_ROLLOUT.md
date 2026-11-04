# Migration, canary and downgrade implementation

Status: code implementation, 4 November 2026. Later native qualification and
selected Uptime live operations are recorded in LINUX_INTEGRATION_2026-11-04.md;
cross-version canary/downgrade acceptance and platform-wide qualification remain open.

## Version transitions

PackageInstanceConfig now accepts protected operator-owned Transitions. Each
entry binds FromManifestDigest, ToManifestDigest, ProcedureDigest,
QualificationDigest, Operation (upgrade or restore), and Mode
(uptime-kuma-startup). Both releases must be admitted in AllowedManifests and
their immutable image config identities loaded through the normal image path.
The operator must retain the actual migration qualification evidence matching
QualificationDigest; naming a digest in configuration does not prove execution.
The transition policy digest is covered by renderer inputs and plan approvals.

The first supported adapter is Uptime Kuma's own SQLite startup migration.
This adapter never accepts raw migration SQL, scripts or commands in a job.
The upgraded signed image performs its built-in migration after a quiesced,
encrypted volume checkpoint. PostgreSQL/MySQL engine major migrations remain
blocked until separate adapters and data-conversion procedures exist.

Uptime Kuma procedures generated from the current package build use version 1.5.0.
Upgrade sequences now checkpoint before image startup and declare that same
checkpoint in the envelope. The backup binds the source release/data format,
even though the upgrade plan targets the new release. Before changing versions,
the runtime verifies the ciphertext, decrypts and validates the retained tar.

instance-transition.json records the pending source, target, approved plan and
checkpoint before startup or recovery changes data. While pending, unrelated
jobs are rejected. A successful health step commits instance-release.json and
clears the intent; failure leaves intent for approved recovery/reconciliation.
Crashes and ambiguous executor mutations retain the journal's existing blocks.

## Downgrade recovery

Protected instance policy may supply RecoveryReleaseVersion and
RecoveryProcedureVersion alongside the explicitly qualified reverse transition.
The catalog then exposes downgrade. The proposal API resolves downgrade into a
normal destructive-storage restore job for that older signed release and the
operator-selected old checkpoint. It retains current data before recovery,
restores the old data format, starts the old pinned image, and checks health.
Old binaries are never simply started against a newly migrated database.

Automatic recovery requires these exact recovery jobs to be approved in advance.
Approvals are not renewable by the coordinator. Expiry, revocation, changed
budgets and unknown operation outcomes still stop execution. Recovery may
discard changes made since the selected checkpoint; the authorized restore
plan must reflect that data-loss boundary.

## Canary coordinator

scripts/lifecycle_rollout.py persists an immutable rollout plan and append-only
state events in a private SQLite database. Commands are init --plan, tick
--policy and status, all with --database and --rollout. A scheduler can invoke
tick repeatedly; each invocation advances at most one action. No command in
this implementation was executed against a host.

A plan has rolloutId, automaticRecovery and waves. Every wave has holdSeconds
and targets. Each target names targetId, tenantScope, hostId, applicationId,
fromManifestDigest, toManifestDigest, forward and recovery actions. An action
contains kind and a complete, already approved PilotJob. Forward starts with
checkpoint, contains upgrade, and ends in a read-only health job. A recovery
path includes restore/downgrade and ends in a read-only health job. A migration
action can carry a separately reviewed typed upgrade procedure; it cannot
introduce a shell capability or make an unsupported runtime adapter executable.

The private transport policy supplies targets (targetId to HTTPS origin),
caFile, certificateFile, keyFile and credentialEnvironment. The credential value
is read only from the environment. Requests use the existing /v1/deployments
API with mTLS, verified server CA and no redirects. Each executor performs its
normal signature, schema, tenant, approval, revocation and resource checks.

Only a completed canary wave and its hold interval release the next wave.
Health observation is limited to the preapproved read-only jobs in the plan;
a hold interval alone is not continuous monitoring. On a confirmed failed job,
automaticRecovery enters recovery for the affected and completed targets in
reverse order. Pending intent, reconciliation, cancellation, rejection or an
ambiguous outcome blocks rollout instead of guessing a recovery action. A
failed recovery blocks further mutation. Requests use immutable job IDs and
query before submission, allowing retry after a lost response or local crash.

## Platform and owner requirements

The controller and planning tooling target Python on Windows and Linux. Docker
runtime execution still follows the existing qualified Linux boundary. Windows
WSL2/Docker and native macOS runtime integration are not inferred from this code.
Mac access will be needed when we begin that platform's runtime integration
and qualification, after the Linux path is ready for the deferred test pass.

Before enabling real transitions, select two pinned Uptime Kuma releases,
retain their image/transport provenance, configure qualified forward/reverse
paths, and approve the recovery checkpoint/data-loss policy. Independent human
review and protected runtime/off-host identities remain required acceptance
inputs. This pass does not raise phase acceptance percentages.
