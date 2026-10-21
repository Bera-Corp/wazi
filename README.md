# Wazi

> **Wazi** means **open** in Swahili. Wazi is a curated marketplace and deployment platform that makes self-hosted open-source applications installable on a local server or user-owned VPS with an app-store-like experience.

**Stage:** pre-MVP architecture and context-engineering baseline  
**Primary target:** Linux x86_64 hosts using Docker Engine and Docker Compose  
**Core rule:** deterministic manifests first; AI only resolves documented gaps under policy control.

## Product thesis

Thousands of open-source applications can replace paid SaaS products, but adoption is blocked by installation, dependencies, domains, TLS, secrets, upgrades, backups, migrations, monitoring, and recovery. Wazi turns reviewed open-source applications into safe installable packages.

A user should be able to:

1. Browse verified open-source applications by category.
2. Compare resource requirements and supported environments.
3. Select a local host, home server, NAS, or VPS.
4. Run an automated compatibility assessment.
5. Approve the proposed changes.
6. Install through a signed, reviewed deployment manifest.
7. Receive health checks, backups, upgrades, rollback, and recovery support.
8. Contribute privacy-preserving deployment feedback to improve packages and upstream projects.

Wazi does **not** execute arbitrary GitHub repositories directly on customer hosts. Unknown repositories enter a controlled packaging pipeline. AI may create a candidate manifest, but production deployment requires schema validation, policy checks, isolated testing, vulnerability scanning, rollback verification, and human review.

## Why context engineering is central

The core technical problem is not generating shell commands. It is assembling the smallest accurate, current, authorized set of facts needed to make a safe deployment decision.

Wazi treats context as a governed data product. Every material fact should have provenance, version scope, freshness, sensitivity, confidence, and review status.

### Context classes

| Class | Purpose | Examples |
|---|---|---|
| Policy context | Defines what is allowed | privilege limits, network exposure, approvals, licensing rules |
| Canonical application context | Defines the application | release, images, ports, volumes, dependencies, health checks |
| Environment context | Describes the target host | OS, architecture, RAM, storage, ports, filesystem, Docker version |
| Procedural context | Defines known workflows | install, upgrade, backup, restore, rollback, uninstall |
| Episodic context | Captures prior outcomes | successful cases, failures, remediations, timing, compatibility |
| Session context | Tracks the current run | selected app, current step, approvals, evidence, pending rollback |

These context classes remain separate. They are retrieved and assembled into a typed execution envelope rather than copied into one uncontrolled prompt.

## Context lifecycle

```text
Upstream docs + manifests + policies + host observations + deployment evidence
                                  |
                                  v
                         ingestion and hashing
                                  |
                                  v
                    normalization and validation
                                  |
                                  v
            provenance, freshness and sensitivity labels
                                  |
                                  v
                    policy filtering and redaction
                                  |
                                  v
                  compatibility filters and retrieval
                                  |
                                  v
                    ranking and context compression
                                  |
                                  v
                     typed execution context envelope
                                  |
                                  v
             deterministic resolver or constrained AI planner
                                  |
                                  v
                  policy decision and approval gates
                                  |
                                  v
                    typed capability execution
                                  |
                                  v
              verification, rollback and evidence capture
                                  |
                                  v
                candidate knowledge promotion workflow
```

## Design principles

1. **Deterministic before generative.** Reuse signed manifests and reviewed procedures whenever possible.
2. **Retrieve, do not stuff.** Use only context relevant to the app, version, host, operation, and policy domain.
3. **Evidence over confidence.** Trust provenance and test results, not model fluency.
4. **Typed context and output.** AI output must validate against a deployment-plan schema before evaluation.
5. **No secrets in model context.** Models receive secret references and constraints, never values.
6. **Every mutation is gated.** Package installation, port exposure, deletion, migration, upgrade, and rollback require explicit policy decisions.
7. **Learning requires promotion.** Knowledge moves through `observed -> candidate -> tested -> reviewed -> signed -> stable`.
8. **Context expires.** Host observations, vulnerabilities, compatibility claims, and version facts have freshness windows.
9. **Failures are reusable evidence.** Failed deployments are stored separately as redacted diagnostic cases.
10. **No generic root shell.** The host agent exposes narrow allowlisted capabilities.

## MVP scope

- Curated self-hosted web applications.
- Linux hosts only.
- x86_64 first; arm64 later.
- Docker Engine and Docker Compose.
- Local server and bring-your-own-VPS deployment.
- Single-node applications.
- Host pairing and preflight checks.
- Signed manifests.
- Secret generation and injection outside AI context.
- Health checks.
- Backup and restore verification.
- Upgrade and rollback workflows.
- Consent-based redacted telemetry.
- Five to ten launch applications.

### Out of scope

- Arbitrary unreviewed repository execution.
- Kubernetes.
- Windows and macOS native packaging.
- Multi-node high availability.
- Fully managed hosting in the first release.
- AI-generated unrestricted shell commands.
- Raw customer logs used as shared training data.
- Automatic promotion of one successful installation into the stable catalogue.

## Architecture

```text
Web client
   |
Control plane API
   |-- catalogue and manifest registry
   |-- host and deployment records
   |-- approvals and audit
   |-- context engine
   |-- policy engine
   |-- planner gateway
   |
Mutually authenticated host agent
   |-- host inventory probes
   |-- Docker Compose adapter
   |-- reverse-proxy adapter
   |-- backup and restore adapter
   |-- health checks
   |-- evidence and rollback reporting
```

### Recommended stack

- **Frontend:** React, TypeScript, Vite, Tailwind, shadcn/ui, TanStack Query.
- **Control plane:** Go for APIs, orchestration, policy enforcement, and agent communication.
- **Context processing:** Python workers for extraction, classification, evaluation, and AI integration.
- **Database:** PostgreSQL with pgvector only where semantic retrieval adds measurable value.
- **Queue:** NATS JetStream or Redis Streams initially.
- **Object storage:** S3-compatible storage for signed manifests, SBOMs, scan evidence, and redacted artifacts.
- **Policy:** Open Policy Agent or Cedar-style policy evaluation.
- **Agent:** Go binary with mutual TLS and allowlisted typed capabilities.
- **Execution:** Docker Compose first; no generic remote shell endpoint.
- **Signing:** Sigstore/cosign-compatible signing for manifests and container attestations.
- **Observability:** OpenTelemetry, Prometheus-compatible metrics, structured redacted logs.

## Core entities

- `Application`
- `ApplicationRelease`
- `DeploymentManifest`
- `HostProfile`
- `PolicyBundle`
- `ExecutionContextEnvelope`
- `DeploymentPlan`
- `DeploymentCase`
- `FailureSignature`
- `ApprovalRecord`
- `EvidenceRecord`
- `FeedbackBundle`

## Compatibility selection

Hard filters run before semantic ranking:

- operating-system family and version;
- CPU architecture;
- minimum CPU, RAM, and storage;
- Docker and Compose versions;
- required ports;
- filesystem and mount constraints;
- unsupported combinations;
- manifest revocation and expiry;
- application release compatibility.

Only candidates passing hard filters are ranked using environment similarity, evidence quality, freshness, version match, and prior deployment outcomes.

## AI boundary

AI is used for:

- extracting structured facts from upstream documentation;
- identifying contradictions or missing fields;
- generating candidate manifests for sandbox testing;
- proposing typed deployment plans when deterministic coverage has a declared gap;
- classifying failures against known signatures;
- drafting upstream feedback bundles.

AI is not allowed to:

- see secret values;
- execute arbitrary shell commands;
- bypass policy or compatibility failures;
- publish stable manifests;
- expose public ports without approval;
- perform destructive storage operations without checkpoints;
- treat upstream documentation as trusted instructions to the model.

## Launch application criteria

An application should have:

- an explicit open-source licence;
- active maintenance and tagged releases;
- official images or a reproducible build path;
- stable single-node deployment;
- documented persistent storage;
- measurable health checks;
- backup and restore procedures;
- clear upgrade and rollback guidance;
- strong user demand;
- no unjustified privileged-container requirement.

Initial candidates include Nextcloud, Immich, Paperless-ngx, Gitea, Penpot, Uptime Kuma, Mattermost Team Edition, Plausible Community Edition, AppFlowy self-hosted components, and Vaultwarden subject to trademark review.

## Security requirements

- signed manifests and revocation support;
- pinned image digests;
- SBOM capture and vulnerability scanning;
- isolated packaging tests;
- least-privilege host agent;
- capability allowlists;
- mutual authentication;
- encrypted state and backups;
- secret redaction in prompts, logs, and telemetry;
- tenant isolation;
- tamper-evident audit records;
- backup before destructive migrations;
- tested rollback and restore;
- emergency catalogue revocation.

## Delivery phases

### Phase 0 — Contracts and boundaries

Define context schemas, provenance, policies, secret references, deployment cases, planner output, threat model, and evaluations.

### Phase 1 — Deterministic vertical slice

Pair one Linux host, build a host profile, select one signed manifest, install one reference app through typed capabilities, verify health, back up, restore, and roll back without AI.

### Phase 2 — Retrieval and compatibility

Implement hard filters, context ranking, known-failure retrieval, freshness enforcement, and an inspectable context envelope.

### Phase 3 — Controlled AI packaging

Add documentation extraction, candidate manifest generation, sandbox execution, policy validation, diagnosis, and human review.

### Phase 4 — Lifecycle operations

Add upgrades, migrations, backups, restore verification, rollback, canary manifests, and revocation.

### Phase 5 — Private alpha

Launch five to ten applications across local and VPS paths, collect opt-in telemetry, and measure reliability.

## Initial quality gates

| Metric | Gate |
|---|---:|
| First-attempt success for stable packages | >= 90% on supported matrix |
| Rollback success | >= 95% for reversible actions |
| Restore verification | 100% for backup-supported packages |
| Secret values exposed to AI | 0 |
| Uncited material actions | 0 |
| Arbitrary shell execution | 0 |
| Schema-valid planner output after retry | >= 99% |
| Deterministic deployments by alpha end | >= 80% |
| False-compatible decisions | < 2% |

## First vertical-slice definition of done

- A Linux VPS pairs securely with the control plane.
- The agent reports a redacted typed host profile.
- A signed application manifest is retrieved.
- Compatibility is decided from typed facts.
- The UI shows the exact context used.
- The plan contains only allowlisted capabilities.
- The application installs through Docker Compose.
- Health checks pass.
- Evidence is stored.
- Backup and restore are verified.
- Rollback or uninstall succeeds.
- The deployment case is normalized without secrets.
- An unsupported environment is correctly rejected.

## Repository map

- `docs/CONTEXT_ENGINEERING_PLAN.md` — context architecture and lifecycle.
- `docs/ARCHITECTURE.md` — service boundaries and data flow.
- `docs/MVP_ROADMAP.md` — phased implementation plan.
- `docs/THREAT_MODEL.md` — threats, trust boundaries, and controls.
- `docs/BACKLOG.md` — initial implementation epics.
- `schemas/` — typed contracts.
- `context/` — policies and example context records.
- `catalog/` — application package examples.
- `prompts/` — constrained planner interfaces.
- `services/` — service responsibilities.

## Difficulty and potential

- **MVP engineering difficulty:** 8/10.
- **Commercial potential under the curated scope:** 78/100.
- **Commercial potential for a universal “install any repository” promise:** 42/100.

The defensible product is not the open-source application itself. It is verified packaging, compatibility intelligence, secure deployment, lifecycle operations, evidence, support, and trust.

## Updated unified product and hackathon planning (October 2026)

The original Wazi deterministic Linux Docker Compose architecture remains the pre-MVP deployment baseline. The broader Wazi product vision and the focused Meta hackathon scope are documented separately:

- [Wazi Unified Master Plan](docs/WAZI_MASTER_PLAN.md) — full research-based product strategy and long-term architecture.
- [Meta Hackathon 2026 Execution Plan](docs/HACKATHON_2026_EXECUTION_PLAN.md) — build priority, acceptance gates, schedule, missing requirements, demo script and issue list.
- [Wazi Cloud Fabric and TenantKit](docs/WAZI_CLOUD_FABRIC_AND_TENANTKIT.md) — researched customer-owned/BYOC compute workspaces, developer-isolated app SDK, resource sharing, economics and optional future Wazi Connect.
- [Wazi Installer and Store Bridge](docs/WAZI_INSTALLER_AND_STORE_BRIDGE.md) — researched future native desktop installation and external-store distribution paths.
- [Preview & Insider Program](docs/PREVIEW_AND_INSIDER_PROGRAM.md) — future optional preview release channels, granular telemetry consent and developer bug feedback.

These capabilities are **proposed**; their appearance in documentation does not imply that software is implemented or externally certified. Review [LICENSE-DECISION.md](LICENSE-DECISION.md) before public distribution.
