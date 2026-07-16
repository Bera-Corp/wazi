# Architecture

Wazi separates control, context, policy, and execution.

## Services

- **Web client:** catalogue, host setup, deployment review, approvals, evidence, lifecycle operations.
- **Control plane:** identity, tenancy, catalogue, deployments, orchestration, audit, manifest distribution.
- **Context engine:** ingestion, normalization, provenance, compatibility, retrieval, redaction, ranking, envelope assembly.
- **Policy engine:** capability authorization, approvals, network exposure, destructive-action and promotion rules.
- **Planner gateway:** constrained model invocation and schema validation; no execution privileges.
- **Host agent:** mutual authentication, preflight probes, typed capabilities, Compose operations, backups, health checks, evidence.

## Trust boundaries

The model cannot access secret values or a generic shell. The control plane cannot directly execute host commands. The host agent accepts only signed requests for allowlisted capabilities. Manifests are signed, versioned, revocable, and bound to evidence.

## Storage

- PostgreSQL: canonical records, deployments, approvals, audit metadata.
- Object storage: manifests, SBOMs, scans, evidence, redacted artifacts.
- Optional pgvector: retrieval index only, never canonical truth.
- Secret broker: credentials and generated values outside context storage.
