# Phase 2–4 implementation handoff

Status: implementation in progress, 4 November 2026. Testing, bug hunting,
security review, live execution and acceptance are deferred by the owner.
The requested 90% target is a buildout target; no acceptance percentage is
increased merely because a source file has been added.

## Phase 2 measurement recording

scripts/phase2_measurement.py provides start, finish and report commands backed
by SQLite. A trial binds an opaque task ID, manual/Wazi mode, source commit
and release digest. Start records the clock; finish records outcome and human
interventions once. Reports group identical bindings and retain sample counts.
No beneficiary names, application data, logs or model prompts are accepted.

Use a private operator directory for --database. Start requires --task,
--mode, --source-commit and --release-digest. Finish requires --trial,
--interventions and --outcome. The report subcommand returns aggregate JSON.
No trial has been run in this implementation pass.

Live registry refresh, Immich capacity/qualification and provider access remain
separate work. Real model calls still require explicit spending authorization.

## Phase 3 runner and receipt path

scripts/qualification_runner.py takes --policy, --manifest and --output.
Run it on a native Linux rootless Docker development worker with an already
loaded, immutable suite image. It does not pull images or accept candidate
commands. The operator policy has exactly these fields:

- image: repository@sha256 digest of the trusted suite image;
- tenantScope, runnerId, runnerVersion and suiteDigest;
- testIds: the complete ordered suite inventory;
- cpuMilli, ramMb, storageMb and timeoutSeconds;
- endpoint: the worker's unix:///run/user/UID/docker.sock endpoint.

The suite image must contain /wazi/qualify, declare no volumes, and support
UID/GID 65534 with a read-only root and a writable /work tmpfs. Its fixed
entrypoint receives one JSON object on stdin containing manifest and
candidateDigest. It emits one JSON object with a tests array; each item has
exactly testId and result (passed or failed). Missing, duplicate, unknown or
oversized results fail the run. Candidate bytes never choose Docker arguments.

The orchestrator creates a disposable container with no network, privileges,
host mounts, devices, published ports or live data. CPU, memory, swap, process,
writable storage, time and output limits are supplied by the operator policy.
Container cleanup occurs after success, failure or timeout. Output is a private,
exclusive directory containing execution.json and qualification.unsigned.json.
Cleanup failure forces a failed result. Raw suite output is temporary.

scripts/uptime_kuma_qualification.mjs supplies the Uptime Kuma suite entrypoint.
It checks candidate identity, startup HTTP, SQLite creation, restart HTTP and
synthetic-file persistence in /work/data. Bake it into the operator's pinned
suite image as /wazi/qualify with an executable mode and Node at
/usr/local/bin/node. The image must contain the pinned app at /app and support
UID 65534. Suite image assembly and qualification remain open. These scenarios
do not establish monitor-record recovery, upgrades or backup correctness.

scripts/sign_qualification.py takes --input, --output, --key, --signer-id,
--policy, --recordtool and --cosign. It uses an external qualification-only
key, disables transparency-log publication, assembles the signed receipt and
uses recordtool's qualification schema/signature path before writing the final
record. The signer key never enters the suite container. Tenant release policy
must continue to exclude qualification signers. Upload the resulting signed
record through the existing candidate qualification API, then use the existing
review and promotion flow. No runner or signing command has been executed here.

## Phase 4 checkpoint buildout

New Nginx checkpoints use age-v1 with nginx-config-json as payloadFormat,
private identity references and the same ciphertext bounds as database
checkpoints. Legacy Nginx .dump checkpoints remain readable. Restores decrypt
before configuration parsing and existing staged nginx validation. Configure
recipients and a protected identity before creating new Nginx checkpoints.

The working tree also adds MySQL import preflight using a unique disposable
schema and cleanup before destructive target commands. This implementation
still uses the existing server/account. Database grants, dumps containing
explicit database references, cleanup failures, concurrent restores and large
imports are unresolved qualification cases. Do not claim that arbitrary SQL
is confined to the disposable schema solely because a default schema is set.

No source change here has been tested, security-reviewed or deployed. Off-host
key provisioning/recovery, version migrations, canary rollout, vulnerability
and SBOM evidence, and Windows/WSL2/macOS qualification remain open. The
existing Phase 3/4 completion plan continues to define the exit gates.
