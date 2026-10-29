# MVP Roadmap

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

## Months 5–6: context retrieval

- context ingestion and normalization;
- hard compatibility engine;
- freshness and revocation enforcement;
- success and failure case storage;
- ranking and context-envelope assembly;
- UI context inspector.

**Gate:** Wazi selects and explains the correct stable procedure.

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
See [the follow-up review](PHASE_0_REVIEW_2026-10-29.md) for remaining Phase 0
completion blockers.
