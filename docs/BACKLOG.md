# Backlog

Status key: `done` · `partial` · `next` · `later`. This tracks the
[MVP roadmap](MVP_ROADMAP.md) phases, not the hackathon schedule.

## Phase 0 — Contracts and boundaries

Revisited after adversarial review: `done` below means enforced by a
schema or validator check with an adversarial proof, not merely
documented. See [GUARANTEE_MATRIX.md](GUARANTEE_MATRIX.md).

- [done] Context, manifest, plan, host-profile, evidence, policy,
  procedure, approval, and compose-artifact schemas, consolidated on
  [common.schema.json](../schemas/common.schema.json) — enforced by
  `scripts/validate.py` (required inventory is fail-closed)
- [done] Closed typed parameters per capability (`oneOf` +
  `capability: const`); `command`/`password` parameters are rejected —
  proof: `evals/adversarial/adv_plan-command-and-password-params.json`
- [partial] Deployment graph: typed procedures, separate `rollbackSteps`
  and digest-bound compose-artifact metadata exist. Evidence linkage,
  full signed procedure parameters and compensation remain open
- [done] Cross-record authorisation: plan ⊆ envelope ⊆ policy, tenant
  agreement, citation resolution, ApprovalRecord bound to canonical
  plan digest + tenant + host + envelope + expiry
- [done] Compatibility on real records at an injected clock: CPU,
  Docker, Compose, filesystem, ports, RAM, storage, arch, OS, derived
  freshness/expiry, revoked/unsigned, privileged, missing-fact —
  `evals/fixtures/` (16 cases)
- [partial] Source provenance and freshness model; timestamp format
  checking is required-active, ordering checked semantically
- [done] Secret-reference design — [SECRET_REFERENCES.md](SECRET_REFERENCES.md);
  sensitive env entries must use references (schema), examples and
  fixtures scanned for secret-bearing keys and synthetic values
- [done] Threat model with concrete abuse cases — [THREAT_MODEL.md](THREAT_MODEL.md)
- [done] Evaluation + adversarial fixtures — `evals/fixtures/`,
  `evals/adversarial/`, `scripts/test_adversarial.py` (14 semantic cases)
- [done] One validation command on PowerShell and Linux
  (`scripts/validate.ps1` / `scripts/validate.sh`), pinned dependencies
  in `requirements-dev.txt`, CI actions pinned to commit SHAs
- [done] Dedicated in-repo secret scanner (`scripts/secret_scan.py`),
  wired into validation wrappers and CI; external scanners may be added
  only pinned by SHA/checksum, and GitHub push protection stays on
- [next] ADR: secret broker choice and per-tenant key isolation
  (needed before Phase 1 stores any tenant secret material)
- [next] Publish Phase 0 to GitHub and confirm CI green there; local
  validation does not establish remote CI status

**Gate check:** a complete deployment (install → proxy → health →
evidence → case, with rollback defined) is representable with the
records in `context/examples/`, every reference resolves and is
digest-bound, and the graph is semantically valid at the injected
evaluation time. Verified by `scripts/validate.py` +
`scripts/test_adversarial.py`.

**Follow-up status, 29 October:** the blockers recorded in
[the review](PHASE_0_REVIEW_2026-10-29.md) were closed the same day;
see its resolution addendum. Remaining Phase 1 items: cryptographic
verification, runtime redaction/broker tests, and publication.

- [done] Follow-up fixes: policy denial precedence, ordered procedure
  checks, unique steps, live maintainer quorum, envelope-bound approvals,
  future/expiry checks, service/project binding and safe diagnostics
- [done] Evidence-to-case/app/release/plan validation in the semantic graph
- [done] Compensation contract for every mutation, including proxy (`proxy.remove`)
- [done] Licence-policy rules enforced semantically and in hard filters; deny precedence tested
- [done] Canonical JSON, signature payloads and cross-language vectors (ADR-001)

## Phase 1 — Deterministic vertical slice (not started)

- Go control-plane skeleton
- Go host agent with mutual TLS pairing
- Linux inventory and preflight probes producing `host-profile` records
  (with re-probe immediately before mutation, per the TOCTOU abuse case)
- Docker Compose typed capability adapter (capabilities and parameter
  schemas already fixed by Phase 0; no shell capability)
- Signed manifest registry serving `app-manifest` records, with
  revocation checks at deploy time
- Approval service issuing digest-bound `approval-record` records
- One reference application through health, backup, restore, rollback

## Later phases

Retrieval/compatibility engine, controlled AI packaging, lifecycle
operations, and private alpha follow the phase order in
[MVP_ROADMAP.md](MVP_ROADMAP.md). Do not start them before the Phase 1
gate passes.

## Worlds and S-Prints extension

See [the feature/buildout plan](WAZI_SPRINTS_PLAN.md) for dependencies and
acceptance evidence. These epics do not change core application delivery gates.

- [done] WS-000: experimental package/composition schemas, three-package
  metadata example, offline resolver and adversarial tests
- [next] WS-001, after Phase 1: executable simulation proof, package
  activation/deactivation and before/after task evidence
- [later] WS-002, after Phase 2 + WS-001: one model adapter, immutable base
  checkpoint, exact interfaces, hardware probe and regression evidence
- [later] WS-003, after Phase 4 + WS-002: isolation, signing/revocation,
  quotas, atomic activation and verified rollback
- [later] WS-004, after core alpha + WS-003: Worlds preview, capability
  cards, verified publishers and read-only MCP
- [later] WS-005: training and additional bridges after a separate design
  review. Physical-device actuation is outside the implemented prototype
