# Wazi Context Engineering Plan

## Objective

Build a provenance-aware context system that converts upstream application knowledge, host observations, policy, prior deployment evidence, and current session state into the smallest safe execution packet required for installation and lifecycle operations.

## Context layers

1. **Policy context** — capabilities, approvals, licensing, privacy, retention, destructive-action controls.
2. **Canonical app context** — repository, release, image digests, ports, volumes, dependencies, health checks, backup and upgrade rules.
3. **Host context** — OS, architecture, CPU/RAM/storage buckets, ports, filesystem, runtime versions, network mode.
4. **Procedural context** — signed install, upgrade, backup, restore, rollback, and uninstall workflows.
5. **Episodic context** — prior successes, failures, remediation steps, timings, and environment similarity.
6. **Session context** — current operation, approvals, action state, evidence, and rollback checkpoint.

## Data contracts

Every context record must carry:

- stable record ID;
- schema version;
- tenant scope;
- source ID and source type;
- content hash;
- extraction method;
- application and version scope;
- created, observed, and expiry timestamps;
- confidence and authority level;
- sensitivity classification;
- review and promotion state.

## Ingestion

Sources include official repositories, release notes, installation documentation, Dockerfiles, Compose files, container registries, licence files, vulnerability scans, Wazi manifests, host-agent probes, deployment evidence, maintainer notes, and user constraints.

Raw upstream text is untrusted. It is parsed as data and may never override system policy or planner instructions.

## Normalization

Normalize raw sources into:

- `Application`
- `ApplicationRelease`
- `DeploymentManifest`
- `HostProfile`
- `PolicyBundle`
- `DeploymentCase`
- `FailureSignature`
- `ApprovalRecord`
- `EvidenceRecord`
- `FeedbackBundle`

Contradictory facts are retained with their provenance and resolved by authority, version scope, freshness, and review state rather than silently merged.

## Retrieval pipeline

1. Bind tenant, app, release, operation, and target host.
2. Load the active policy bundle.
3. Reject stale or revoked manifests.
4. Apply hard compatibility filters.
5. Retrieve the signed procedure for the exact operation.
6. Retrieve a small number of comparable successful cases.
7. Retrieve known failure signatures relevant to the host and release.
8. Redact sensitive data.
9. Rank by exact version match, environment similarity, evidence quality, and freshness.
10. Assemble a typed `ExecutionContextEnvelope`.

Hard filters must run before semantic search. Semantic similarity cannot override incompatible architecture, insufficient resources, occupied required ports, unsupported filesystems, revoked releases, or missing approvals.

## Context envelope

An execution envelope contains:

- operation and deployment mode;
- application ID, release, and manifest digest;
- host profile reference;
- policy bundle reference;
- procedure reference;
- selected success and failure case references;
- secret references without values;
- allowed capabilities;
- forbidden operations;
- required approvals;
- verification checks;
- rollback strategy;
- cited sources and hashes;
- creation and expiry times.

The complete envelope must be inspectable in the UI and included in the audit record.

## Memory model

- Durable canonical facts live in versioned relational records.
- Documents, SBOMs, and scan artifacts live in object storage.
- Semantic indexes store embeddings plus authoritative metadata, never as the source of truth.
- Host observations expire quickly and are refreshed before mutation.
- Session state is ephemeral but summarized into an immutable deployment record.
- Success and failure cases remain separate.
- Raw customer logs are not promoted into shared memory.

## AI planner boundary

The planner receives only the typed context envelope and a schema for `DeploymentPlan`.

It must:

- cite context IDs for assumptions and actions;
- use only allowlisted capabilities;
- identify unresolved questions instead of inventing values;
- define validation, failure conditions, and rollback for every state change;
- stop when required approval is missing;
- never request or output secret values;
- never output unrestricted shell commands.

Planner output passes schema validation, policy evaluation, deterministic safety checks, and approval gates before execution.

## Secret handling

Secrets are generated or retrieved through a separate secret broker. The planner sees references such as `secret://deployment/db-password`, expected format, rotation requirements, and target injection points. Values never enter prompts, model logs, telemetry, deployment cases, or Git history.

## Learning and promotion

Deployment evidence follows:

`observed -> candidate -> sandbox-tested -> reviewed -> signed -> canary -> stable`

Promotion requires:

- replay in isolated test environments;
- supported-host matrix coverage;
- vulnerability and licence review;
- backup and restore test;
- rollback test;
- deterministic output comparison;
- human approval;
- signed manifest and revocation metadata.

No single successful customer deployment becomes a stable template automatically.

## Evaluation suite

Offline fixtures must test:

- supported and unsupported operating systems;
- wrong architecture;
- insufficient RAM or storage;
- occupied ports;
- stale host profiles;
- revoked manifests;
- malicious prompt injection in upstream docs;
- secrets in logs;
- contradictory requirements;
- known upgrade failures;
- tenant-isolation attempts.

Metrics:

- correct manifest retrieval at top 1 and top 3;
- false-compatible rate;
- stale-context rejection;
- secret leakage;
- uncited action rate;
- policy-violation proposal rate;
- schema-valid output rate;
- first-attempt deployment success;
- rollback and restore success;
- deterministic reuse ratio;
- median intervention time.

## Implementation sequence

### Stage 1 — Contracts

Implement schemas, provenance, policy, secret references, deployment records, and evaluation fixtures.

### Stage 2 — Deterministic reference path

Pair a Linux host, build a host profile, select a signed manifest, install one reference app, verify health, back up, restore, and roll back without AI.

### Stage 3 — Retrieval

Add compatibility filters, ranking, failure signatures, freshness enforcement, and context inspection.

### Stage 4 — Controlled AI packaging

Add documentation extraction, candidate manifests, sandbox tests, typed planning, diagnosis, and human review.

### Stage 5 — Lifecycle and alpha

Add upgrades, canary releases, revocation, five to ten applications, consent-based telemetry, and reliability reporting.
