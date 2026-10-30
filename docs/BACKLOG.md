# Backlog

Status key: `done` · `partial` · `next` · `later`. This tracks the
[MVP roadmap](MVP_ROADMAP.md) phases, not the hackathon schedule.

## Four-pillar delivery rule

Use [the judging rubric](JUDGING_PILLARS.md) in feature selection, acceptance
and rehearsals: Impact 30%, Use of Meta AI Tooling 25%, Technical Depth &
Execution 20%, Demo 25%. Each competition item records its pillar(s),
beneficiary, acceptance evidence, implementation/verification state and demo
moment. Weighted readiness cannot override mandatory safety gates.

- [done] Integrate the owner-supplied rubric into roadmap, feature decisions,
  hackathon acceptance and demo narrative
- [next] JUDGE-001: exact official problem-statement mapping, one beneficiary
  and reproducible before/after task baseline (Impact)
- [next] JUDGE-002: accessible eligible Meta model; one typed, cited proposal
  with useful test/rescan evidence and bounded calls/cost (Meta AI)
- [next] JUDGE-003: Phase 1 signed-artifact verification and one disposable
  host journey with health/rollback receipts (Engineering and Demo)
- [next] JUDGE-004: review/approval/progress/result UI and timed end-to-end
  rehearsal; video, evidence links and labelled fallback (Demo and Impact)

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
- [done] Phase 0 deployment-graph contracts: typed procedures, separate
  rollback steps, parameter/compensation checks, artifact-byte binding and
  evidence linkage. Cryptographic authenticity and runtime execution remain Phase 1
- [done] Cross-record authorisation: plan ⊆ envelope ⊆ policy, tenant
  agreement, citation resolution, ApprovalRecord bound to canonical
  plan digest + tenant + host + envelope + expiry
- [done] Compatibility on real records at an injected clock: CPU,
  Docker, Compose, filesystem, ports, RAM, storage, arch, OS, derived
  freshness/expiry, revoked/unsigned, privileged, missing-fact —
  `evals/fixtures/`; the current suite is the source of case counts
- [partial] Source provenance and freshness model; timestamp format
  checking is required-active, ordering checked semantically
- [done] Secret-reference design — [SECRET_REFERENCES.md](SECRET_REFERENCES.md);
  sensitive env entries must use references (schema), examples and
  fixtures scanned for secret-bearing keys and synthetic values
- [done] Threat model with concrete abuse cases — [THREAT_MODEL.md](THREAT_MODEL.md)
- [done] Evaluation + adversarial fixtures — `evals/fixtures/`,
  `evals/adversarial/`, `scripts/test_adversarial.py`
- [done] One validation command on PowerShell and Linux
  (`scripts/validate.ps1` / `scripts/validate.sh`), pinned dependencies
  in `requirements-dev.txt`, CI actions pinned to commit SHAs
- [done] Dedicated in-repo secret scanner (`scripts/secret_scan.py`),
  wired into validation wrappers and CI; external scanners may be added
  only pinned by SHA/checksum, and GitHub push protection stays on
- [next] ADR: secret broker choice and per-tenant key isolation
  (needed before Phase 1 stores any tenant secret material)
- [done] Publish Phase 0 to GitHub: `9e52ff8`, followed by CI correction
  `3ae8345`; run 40928854658 passed both contracts and secret-scan jobs
  (independently verified in the preceding project review)

**Gate check:** a complete deployment (install → proxy → health →
evidence → case, with rollback defined) is representable with the
records in `context/examples/`, every reference resolves and is
digest-bound, and the graph is semantically valid at the injected
evaluation time. Verified by `scripts/validate.py` +
`scripts/test_adversarial.py`.

**Follow-up status, 29 October:** the blockers recorded in
[the review](PHASE_0_REVIEW_2026-10-29.md) were closed the same day;
see its resolution addendum. Remaining Phase 1 items: cryptographic
verification and runtime redaction/broker tests. Publication and remote CI
were subsequently verified at `3ae8345`.

- [done] Follow-up fixes: policy denial precedence, ordered procedure
  checks, unique steps, live maintainer quorum, envelope-bound approvals,
  future/expiry checks, service/project binding and safe diagnostics
- [done] Evidence-to-case/app/release/plan validation in the semantic graph
- [done] Compensation contract for every mutation, including proxy (`proxy.remove`)
- [done] Licence-policy rules enforced semantically and in hard filters; deny precedence tested
- [done] Canonical JSON, signature payloads and cross-language vectors (ADR-001)

## Phase 1 — Deterministic vertical slice (local trust/resource gates; deployment disabled)

- [done] Verify development-host SSH and sudo access; save local metadata
  and key path in ignored `.env.local`. See [development environments](DEVELOPMENT_ENVIRONMENTS.md)
- [done] Install/verify pinned Go and supported Docker/Compose; native
  compilation, race checks and a real schema-valid profile are qualified.
  This does not qualify application execution
- [partial] Implement the [Wazi Nodes](WAZI_NODES_PLAN.md) inventory/contribution
  path within this slice. The owner budget and local resource-bound contracts
  are recorded; runtime executor and per-role enforcement remain open

The 29 October build order is superseded by the contract, trust, and runtime
prerequisites below. Do not enable application mutation until those prerequisites
have passing contract and adversarial evidence.

- [partial] Go module scaffolding exists in `services/` (control plane,
  host agent, shared contracts); local tests/vet and Linux cross-build pass.
  The [30 October review findings](LATEST_EDITS_REVIEW_2026-10-30.md)
  (P1 listener/version identity, P2 intake/probe accuracy) are resolved
  in code with regression tests; see `services/README.md`. The private
  control plane is now deployed on bera-003; native and live evidence is
  in [the 30 October deployment record](BERA_003_LIVE_TEST_2026-10-30.md)
- [partial] Linux inventory and preflight probes produce a real
  schema-valid `host-profile` on bera-003. Re-probe immediately before
  mutation remains unqualified because application execution is disabled.
- [partial] Secure pairing: mutual TLS enrollment implemented -
  one-time expiring codes bound to (tenant, node), node-generated
  keys with CSR-only enrollment, certificates bound to the authorised
  tenant/node, revocation (which also invalidates outstanding
  pairing codes), atomic commit-vs-revocation authorisation,
  exact-property/presence/nullability intake checks (not full
  schema parity), HTTPS-only no-redirect
  agent transport, and the full mTLS acceptance matrix
  (missing/untrusted/expired certs, wrong server identity, replay,
  revoked, cross-tenant/node) in `services/internal/controlplane`.
  Durable hashed-code single-writer registry, operator token,
  body deadlines, two-phase renewal and transactional failed-save
  rollback are implemented; a dedicated operator
  identity/admin transport and overall pairing acceptance remain open;
  real enrollment/intake, renewal, isolation, revocation and service-restart
  checks pass on bera-003 (26 deployed-binary checks);
  deployment stays disabled (501) until registry and agent signature checks,
  revocation, approval and runtime acceptance pass
- [partial] Offline signature verification and verified manifest registry:
  the Go verifier matches the ADR-001 canonical vectors, loads a versioned
  key-pinned signer policy, and passes a checked-in Cosign v3.1.4
  interoperability fixture. The control plane rejects invalid signatures
  before writing, uses an operator token for ingest, and re-verifies persisted
  records at startup. Registry ingest now applies the full app-manifest
  Draft 2020-12 schema after cryptographic verification and rejects a validly
  signed invalid record before writing. Separately signed procedures are also
  schema-checked and signature-verified at ingest and restart. The agent now has
  a read-only binding preflight for canonical envelope/plan schemas, signed main
  and rollback procedures, tenant and caller-authenticated policy references,
  capability allow/deny sets, parameter contracts, approval declarations, and
  expiry windows; it returns their canonical digests for a future approval
  record. A standalone case-bound approval API and agent verifier now check
  exact digests, identities, time bounds, role rules, and one-job consumption.
  The legacy API path maps one operator token to one configured identity; new
  local code supports separately configured credential principals and an
  agent consume/verify helper for quorum. Paired mTLS policy-bundle retrieval
  now exists locally and binds policy ID, version, tenant and envelope digest.
  Live policy configuration, human credential custody, executor integration
  and authenticated policy updates remain open. Signed
  revocation lists now have a required sequence and signature, operator-token
  publication, paired-node reads, registry rollback rejection, and a
  persistent agent cursor. The agent's revocation-aware release preflight
  checks freshness with an explicit caller-supplied maximum age and persists
  the cursor before returning; a deployment maximum age is not configured.
  This read-only preflight is not connected to an executor. Fresh host
  qualification and immediate pre-mutation authorization remain open. Private
  test keys are not committed; fixtures contain only public verification
  material.
- [partial] Agent preflight verifies signed manifest/procedure identities and
  revocation freshness as a read-only API. The authenticated policy and
  resource-bound envelope/plan preflight is now locally composed and checks
  approved CPU/RAM limits for every app service plus hard storage ceilings
  for managed volumes. These helpers are still not connected to an executor.
  A standalone durable journal now binds
  case/job and signed-input digests, persists typed mutation intent before a
  capability call, and blocks or requires resource reconciliation after
  restart. It is not wired into the agent or executor. Fresh host-profile
  validation, human identity/quorum, runtime authority, and the first mutating
  operation remain open. Execution stays disabled (501).
- [partial] Approval pilot issues digest-bound `approval-record` records and
  binds them to a durable case ID, exact plan/envelope digests, tenant, host,
  app, action and expiry. The paired node consumes the record once with
  same-job idempotent retry and cross-job replay rejection. This is a single
  configured operator token/identity in the deployed pilot. Local code now
  accepts separately provisioned approver credentials, issues records under
  distinct configured identities, and provides an agent client that consumes
  multiple approvals and applies the authenticated role/quorum rule. Distinct
  human custody is not independently established, the credential mode is not
  configured on bera-003, and no executor calls this client yet. A separate
  case store and policy/executor integration remain open.
- Docker Compose typed capability adapter (capabilities and parameter
  schemas already fixed by Phase 0; no shell capability). Establish and
  negatively test the runtime boundary before mutation; raw access to the
  existing rootful Docker socket does not scope authority to the demo app.
  The agent re-verifies signature, revocation, approval, and a fresh host
  profile immediately before execution. Integrate the standalone operation
  journal so intent is persisted before the first mutation and owned resources
  are reconciled after restart or duplicate delivery.
- **Owner-selected real reference package, replacing the synthetic Phase 0 example:**
  the owner selected Gitea. Gitea 28.2.0 rootless is the local qualification
  candidate and registry digests are pinned only as candidates; queried
  upstream attestation endpoints did not provide a cryptographically verified
  provenance record. Owner-authorized budget revision 2 is versioned within
  the approved cap. The remaining package gate is verified image provenance,
  a genuine signature from the Wazi demo signer, reproducible
  install/upgrade/backup/restore/rollback procedures, and measured health.
  Phase 0 example digests/signatures remain demo-only fixtures.
- **Fresh installs are a distinct path:** lifecycle state is detected
  before execution. A fresh install (no existing application volume)
  records a fresh-install attestation instead of a backup checkpoint;
  backup checkpoints are mandatory only for operations over existing
  data (upgrade, migration, restore, rollback). Contract support for
  this distinction is Phase 1 work on the Phase 0 checkpoint types.
- Lifecycle evidence: evidence records produced and linked at install,
  health, backup, restore and rollback, closing a deployment case
- Adversarial end-to-end tests on the real host: unsigned manifest,
  untrusted signer, stale revocation, revoked manifest, stale host
  profile, wrong architecture, incompatible resources, disk-full, daemon
  loss, interrupted backup/restore, evidence-write failure, duplicate jobs,
  and restart mid-operation. Assert zero unauthorized workload mutations:
  containers, volumes, networks, image pulls, and application data within
  the declared scope. Rejection and audit evidence may still be written.

**Gate:** Phase 1 remains open and the deployment endpoint remains HTTP 501
until every owner-confirmed closure gate below passes in dependency order.
Only the approved tenant, node, application, and artifact digests may execute.

### Owner-confirmed Phase 1 closure order

Complete these gates in order; later evidence depends on the earlier trust and execution gates.

| Gate | Required exit evidence and current state |
|---|---|
| 1. Verified image provenance and Wazi release | Verify provenance for the exact Gitea image digest, then issue and verify a Wazi-signed release. Current: the Gitea 28.2.0 rootless digest is only a candidate; cryptographic provenance and Wazi release approval are absent. |
| 2. Typed executor on the approved rootless boundary | Wire the existing preflight, approval, and operation-journal helpers into one typed executor on the approved rootless boundary. Enforce the authenticated policy, the pilot's configured single-approver rule, resource and helper limits, and intent-before-mutation journal behavior. Do not claim independent-human quorum for this pilot. Current: paired preflight now returns exact verified records and approval evidence, and the credential loader accepts one configured pilot principal; no executor or runtime adapter integrates the helpers, and live policy/approval configuration remains open. |
| 3. Live digest-bound stateful lifecycle | Install release A with known application data → backup → upgrade to release B → restore → verify application data → rollback to A on the approved boundary. Bind evidence at every stage to the exact image/release digests and deployment case. Current: no Gitea workload or lifecycle has run; the endpoint remains HTTP 501. |
| 4. Live adversarial denial matrix | Exercise the unauthorized and failure cases on the live approved boundary and retain evidence proving zero unauthorized workload mutations. Current: local contract/adversarial tests pass, but the live matrix has not run. |
| 5. Reboot, pairing, and remote CI acceptance | Record reboot/reconnect recovery evidence, obtain pairing sign-off, and record a passing remote services CI run. Current: reboot/reconnect passed and is recorded in artifacts/bera-003/20261030T171532Z/reboot-reconnect.json; pairing sign-off and remote CI remain open. |

The competition workstream runs alongside these gates: measure the
beneficiary baseline early, prove one bounded real Meta proposal with
test/rescan evidence, and build review/approval/progress UX against the
deterministic APIs. The four judging weights and safety gates still apply.

## Wazi Nodes and Fabric

See [the integrated research/buildout plan](WAZI_NODES_PLAN.md). Reuse the
existing agent and control plane; do not introduce a second execution stack.

- [done] NODE-000: Linux SSH/sudo environment verified; runtime tooling pending
- [partial] NODE-001: device identity, one-time mTLS pairing, durable registry, operator token and renewal implemented in services (acceptance and live host qualification remain); real inventory probe hardened
- [partial] NODE-002: versioned contribution-budget and managed-volume-qualification contracts are checked by Python and the embedded Go contract package; owner-authorized budget revision 2 and the post-reboot rootless-volume qualification at `2026-10-30T17:15:43Z` (expires `17:30:43Z`) are recorded. Authenticated runtime consumption and executor binding remain open
- [partial] NODE-003: rootless engine and aggregate cgroup boundary are qualified; signed Gitea image, per-role limits, app volumes, health, and lifecycle evidence remain open
- [next] NODE-004: unavailable/reconnected states and reconciliation evidence
- [later] NODE-005: deterministic placement, availability policy and private access
- [later] NODE-006: quotas, durable backups, safe drain/removal and revocation
- [later] NODE-007: GPU, organisational pools, cluster/VM adapters and migration

## Later phases

Retrieval/compatibility engine, controlled AI packaging, lifecycle
operations, and private alpha follow the phase order in
[MVP_ROADMAP.md](MVP_ROADMAP.md). Do not start them before the Phase 1
gate passes.

- [partial] Phase 2 context retrieval (owner-directed parallel offline
  work, 2026-10-30): `scripts/context_engine.py` implements the
  retrieval pipeline from CONTEXT_ENGINEERING_PLAN.md - binding,
  tenant isolation, redaction gate, revocation freshness (stale record
  fails closed), hard compatibility before ranking, explainable scored
  procedure ranking, failure signatures, comparable-case citation, and
  schema-valid envelope assembly. `scripts/test_context_engine.py`
  covers the pipeline and adversarial denials; evaluation over the 17
  labelled fixtures reports hard-engine accuracy 1.0 (hard filters
  only); end-to-end adversarial retrieval (12 scenarios: tenant,
  procedure trust, revocation, envelope) reports accuracy 1.0 and
  false-accept rate 0. Review corrections landed 2026-10-30:
  tenant-scoped case retrieval (fail closed without scope), procedure
  eligibility (schema, signed/canary/stable, trusted signer, payload
  binding, exact release scope) before ranking, envelope rebound to
  current records with schema+binding validation before accept,
  live-mode revocation proof (verified, current, non-rolled-back
  snapshot mandatory; fixture mode explicitly labelled), and a
  free-text secret scan in addition to the host redaction flag.
  The demo at `demo/index.html` is generated from real pipeline
  output by `scripts/build_demo_data.py` and shows one accept plus two
  engine-produced denials. Still open: live host freshness, real
  deployment cases (only eval fixtures exist today), ranking against
  real deployments, and any AI planner (Phase 3). This does not advance
  the Phase 1 execution gate, which stays 501.

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
