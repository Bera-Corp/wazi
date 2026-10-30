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

## Owner-controlled compute: Nodes and Fabric

[Wazi Nodes](WAZI_NODES_PLAN.md) makes one owner-controlled Linux host a
first-class destination. Fabric is the proposed shared view of personal
nodes, existing VPS resources and future managed compute. Reuse the host
agent, mutual TLS, policy, registry, approvals and evidence components.

Keep physical capacity, owner contribution limits, active reservations and
current availability distinct. Placement must respect the owner/tenant,
fresh resource observations and supported isolation. The initial adapter
uses Docker Compose with explicit limits; clusters, GPUs, VM hardening and
failover are later capabilities.

[Development SSH access](DEVELOPMENT_ENVIRONMENTS.md) is operational
bootstrap access, separate from product node credentials. Existing tailnet
networking is a private transport, not proof of Wazi pairing or an application
public-exposure grant. Qualification, heartbeat expiry, data retention and
reconnect behaviour require typed records and runtime evidence.

## Experimental Worlds extension

The [S-Print feature plan](WAZI_SPRINTS_PLAN.md) extends registry and BYOC
concepts with runtime profiles, capability negotiation and component graphs.
The offline resolver matches interface IDs, versions and contract hashes,
pins explicit dependencies, rejects cycles and reserves composition RAM/VRAM.

World interfaces describe observation, state, dynamics, planning and actions.
They are separate from host-agent capabilities and confer no execution
permission. Prototype requests are simulation-only. Future loaders must
verify signatures, asset bytes, licences, policy, isolation and evaluation
evidence before activation. Base weights remain immutable; overlays and
activation locks are versioned. No real model adapter is implemented yet.
