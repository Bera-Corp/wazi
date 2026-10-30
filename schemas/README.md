# Wazi Contracts (Phase 0)

Versioned JSON Schema (draft 2020-12) contracts. These are the Phase 0
deliverable from [the MVP roadmap](../docs/MVP_ROADMAP.md): a complete
deployment must be representable without hidden free-form state — and
the documented safety guarantees must be enforced, not just described.
See [the guarantee matrix](../docs/GUARANTEE_MATRIX.md) for the
invariant → enforcement → proof mapping.

Shared types live once in [common.schema.json](common.schema.json)
(capability enum, `secretRef`, `sha256Digest`, digest-bound `recordRef`,
provenance, review states, approval actions). Other contracts `$ref`
them; do not redefine them locally.

| Schema | Contract for |
|---|---|
| [app-manifest.schema.json](app-manifest.schema.json) | One application release: pinned digests, ports, volumes, secret references, health check, resource/runtime requirements, and optional Phase 1 per-service CPU/RAM and per-volume storage ceilings. Runtime enforcement still requires the resource-bound preflight and typed adapter. Privileged containers are `const: false` |
| [execution-context-envelope.schema.json](execution-context-envelope.schema.json) | Typed packet for one operation. All record references are digest-bound, so changing a referenced record invalidates the envelope |
| [deployment-plan.schema.json](deployment-plan.schema.json) | Resolver/planner output. Closed per-capability parameters via `oneOf` + `capability: const` — `command`/`password` fields are rejected. Forward `steps` and `rollbackSteps` are separate sequences |
| [host-profile.schema.json](host-profile.schema.json) | Redacted, expiring host observation. Records any observed OS/arch; compatibility filters enforce MVP support |
| [policy-bundle.schema.json](policy-bundle.schema.json) | Capability allowlist, approval rules, licensing, promotion and freshness gates. Generic shell is `const: false` |
| [procedure.schema.json](procedure.schema.json) | Typed lifecycle procedure (capability sequence per operation). Envelope procedure references must resolve to a record |
| [compose-artifact.schema.json](compose-artifact.schema.json) | Deterministic rendering contract: immutable Compose artifact rendered from one manifest by a pinned renderer, consumed by digest |
| [approval-record.schema.json](approval-record.schema.json) | Human approval bound to the canonical digest of one exact plan, plus tenant, host, envelope, action, and expiry. Editing the plan invalidates it |
| [evidence-record.schema.json](evidence-record.schema.json) | Hash-bound pointer to one verification artifact |
| [deployment-case.schema.json](deployment-case.schema.json) | Normalized outcome record. A single success stays `observed`; it never auto-promotes |
| [eval-fixture.schema.json](eval-fixture.schema.json) | Compatibility fixture carrying real manifest/host/policy records, replayed at an injected `evaluationTime` |
| [contribution-budget.schema.json](contribution-budget.schema.json) | Phase 1 NODE-002 owner contribution budget: CPU in logical cores; RAM and storage in MiB; physical storage is total capacity. Physical totals, owner budget, system reserve and per-role allocations stay distinct, with nesting enforced semantically |
| [managed-volume-qualification.schema.json](managed-volume-qualification.schema.json) | Phase 1 daemon-managed storage qualification, kept separate from the host profile so `dockerRootWritable=false` is never weakened; free storage is measured in MiB and is not a quota; preflight-only records cannot qualify, and qualified storage requires an enforceable quota |
| [execution-context-envelope-v2.schema.json](execution-context-envelope-v2.schema.json) | Version 2 requires digest-bound references to the owner contribution budget and daemon-managed volume qualification; v1 remains unchanged |

Examples live in [context/examples/](../context/examples/) and form one
complete graph (host → policy → procedures → manifest → compose
artifact → envelope → plan → approval → evidence → case). Compatibility
fixtures are in [evals/fixtures/](../evals/fixtures/) and records that
must fail are in [evals/adversarial/](../evals/adversarial/).

## Validation layers (all fail-closed)

1. Required schema inventory — a missing schema is an error, never a skip.
2. Schema validation with date-time format checking active.
3. Secret scan (keys + synthetic value patterns) over examples and fixtures.
4. Timestamp ordering and freshness against an injected clock.
5. Cross-record semantic validation: digest binding, tenant agreement,
   capability authorisation (plan ⊆ envelope ⊆ policy), citation and
   procedure resolution, rollback resolution, approval binding.
6. Compatibility hard filters on real records.
7. Adversarial fixtures must fail; semantic mutations must be rejected
   by `scripts/test_adversarial.py`.

## Run it

```powershell
# Windows
.\scripts\validate.ps1
```

```sh
# Linux/macOS
./scripts/validate.sh
```

Both create a project-local `.venv`, install the pinned dependencies
from [requirements-dev.txt](../requirements-dev.txt), and run every
gate. In CI the same commands run after `pip install -r requirements-dev.txt`.

## Experimental Worlds contracts

[sprint-manifest.schema.json](experimental/sprint-manifest.schema.json) and
[world-composition.schema.json](experimental/world-composition.schema.json)
use a separate experimental `0.1.0` namespace. They do not extend the
production host capability enum. `scripts/validate.py` checks the schemas
and example graph; `scripts/test_sprints.py` exercises rejection cases.
`scripts/sprints.py` resolves compatibility without executing assets.

See [the Worlds feature plan](../docs/WAZI_SPRINTS_PLAN.md) and
[the Phase 0 follow-up review](../docs/PHASE_0_REVIEW_2026-10-29.md).
Typed metadata and local tests do not yet satisfy every Phase 0 completion
claim: evidence linkage, complete compensation and signing contracts remain.

## Versioning

`schemaVersion` is `1.0.0` for record contracts (eval fixtures are
`2.0.0` after the real-record redesign). Breaking changes bump the
major version and add a new `$id` path (`/schemas/v2/…`); old versions
stay valid for already-signed manifests.
