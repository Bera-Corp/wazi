# MVP Roadmap

## Judging pillars across delivery

Hackathon-facing product and build decisions use the owner-supplied weights:
Impact **30%**, Use of Meta AI Tooling **25%**, Technical Depth & Execution
**20%**, Demo **25%**. [The shared rubric](JUDGING_PILLARS.md) maps features to
evidence, acceptance and demo beats. The phase sequence below remains the
engineering dependency order; the competition track is planned alongside it.

- Phase 1: prove the deterministic signed deployment, then show the running
  app in a usable review/approval/progress flow. This establishes execution
  evidence and a demo backbone; do not wait until the end to design the UX.
- Competition workstream: select a beneficiary and exact challenge, measure
  baseline friction, and add one material, bounded Meta proposal path with
  tests and rescan. Its execution remains behind the deterministic gates.
- Each release/rehearsal: review all four pillars using actual receipts.
  CI alone does not establish impact, Meta use or demo readiness.

## Months 1–2: contracts and security boundaries

- context, manifest, plan, host-profile, evidence, and policy schemas;
- source provenance and freshness model;
- secret-reference design;
- threat model and evaluation fixtures;
- monorepo CI, linting, schema validation, secret scanning.

**Gate:** a complete deployment can be represented without hidden free-form state.

## Months 3–4: deterministic vertical slice

- Go control-plane skeleton;
- Go host agent with mutual TLS pairing;
- Linux inventory and preflight probes;
- Docker Compose typed capability adapter;
- signed manifest registry;
- one reference application;
- health, uninstall, backup, restore, and rollback evidence.

**Gate:** reference app completes its lifecycle on the supported matrix without AI.

The [Linux development environment](DEVELOPMENT_ENVIRONMENTS.md) is reachable
for SSH/sudo bootstrap. Go and Docker prerequisites are pending. Integrate
the first [Wazi Node](WAZI_NODES_PLAN.md) through this same slice: authenticated
inventory, owner contribution limits, one signed application, enforced
CPU/RAM limits, persistent data and unavailable/reconnected evidence.
Apply versioned contracts before changing v1 resource/checkpoint semantics.

## Months 5–6: context retrieval

- context ingestion and normalization;
- hard compatibility engine;
- freshness and revocation enforcement;
- success and failure case storage;
- ranking and context-envelope assembly;
- UI context inspector.

**Gate:** Wazi selects and explains the correct stable procedure.

Nodes/Fabric selection adds explicit eligible/ineligible destination reasons,
availability policy and atomic capacity reservations after the node slice
has runtime evidence. Later lifecycle work adds durable drain/removal and
backup semantics. Cluster, GPU, VM and migration adapters remain later work.

## Months 7–8: controlled AI packaging

- official-doc extraction;
- candidate manifest generation;
- sandbox deployment runner;
- typed planner and policy validator;
- failure classification;
- maintainer review and promotion workflow.

**Gate:** AI can create a candidate package but cannot publish or deploy it directly.

## Months 9–10: lifecycle operations

- version-aware upgrades and migrations;
- encrypted backups and restore verification;
- rollback checkpoints;
- canary manifests and emergency revocation;
- SBOM and vulnerability evidence.

**Gate:** supported applications can be operated safely, not merely installed.

## Months 11–12: private alpha

- five to ten curated applications;
- local and bring-your-own-VPS flows;
- consent-based telemetry;
- support and upstream-feedback workflows;
- reliability and intervention dashboards.

**Alpha gates:** >=90% first-attempt success, >=95% rollback success, 100% restore verification for supported packages, zero secret exposure, and >=80% deterministic deployments.

## Worlds extension: S-Print packages

[Worlds and S-Prints](WAZI_SPRINTS_PLAN.md) adds environments, reusable skills,
action/model adapters, planners, evaluators, datasets, guardrails and
training recipes. The application roadmap remains the core delivery path.

- **Phase 0 extension, local:** experimental 0.1.0 contracts, interface and
  dependency negotiation, resource/permission checks and a synthetic
  three-package composition. No package execution or measured model gains.
- **After Phase 1:** WS-001 executable simulator proof with activation,
  deactivation and reproducible before/after evidence.
- **After Phase 2 and WS-001:** WS-002 one model integration with pinned
  weights, exact interfaces, hardware checks and regression evidence.
- **After Phase 4 and WS-002:** WS-003 signed, isolated, revocable activation
  with quotas and verified rollback.
- **After core private alpha:** WS-004 optional Worlds preview, capability
  cards, evidence-backed compatibility and read-only MCP inspection.
- **Research only:** WS-005 training and broader bridges. Physical control
  needs separate design, approval and validation.

The extension uses dependency gates, not unverified calendar commitments.
See [the follow-up review](PHASE_0_REVIEW_2026-10-29.md) for the resolved Phase 0
findings and the boundaries retained for Phase 1.
