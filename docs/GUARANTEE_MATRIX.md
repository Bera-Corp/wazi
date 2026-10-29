# Guarantee Matrix — Phase 0

Each invariant maps to the contract or check that enforces it, the
adversarial case that proves enforcement, and the phase where a runtime
must enforce it again (contracts do not replace runtime checks).

| Invariant | Enforced by (Phase 0) | Adversarial proof | Runtime phase |
|---|---|---|---|
| No shell execution is representable | Closed capability enum in [common.schema.json](../schemas/common.schema.json); closed per-capability parameters via `oneOf` + `capability: const` in [deployment-plan.schema.json](../schemas/deployment-plan.schema.json) | `evals/adversarial/adv_plan-shell-capability.json`, `adv_plan-command-and-password-params.json` | Phase 1 agent: capability dispatch only |
| Every material step is cited | `citations` minItems 1 in plan schema; citations must resolve to graph records in `semantic_errors` | `adv_plan-uncited-step.json`; semantic test "unresolved citation" | Phase 1 planner gateway |
| Images are pinned | `digest` required, `sha256:` pattern, in app-manifest schema | `adv_manifest-unpinned-image.json` | Phase 1 registry + agent pull-by-digest |
| Deterministic deploy needs signed manifest | Envelope `allOf`: deterministic ⇒ reviewState signed/canary/stable; compatibility `decide` rejects candidate/revoked | `eval_unsigned-manifest.json`, `eval_revoked-manifest.json` | Phase 1 registry revocation checks |
| Signed states carry a signature | Manifest/procedure `allOf`: signed/canary/stable ⇒ `signature` required | Semantic + schema checks in `validate.py` | Phase 1 cosign verification |
| Rollback path exists and is separate | `rollbackSteps` is a distinct sequence; `rollbackStepId` must resolve there, never into `steps` | Semantic tests "rollbackStepId pointing into steps / nowhere" | Phase 1 executor rollback runbook |
| Procedures are typed records | [procedure.schema.json](../schemas/procedure.schema.json); envelope refs must resolve + digest-match | Semantic test "undefined procedure reference" | Phase 1 procedure registry |
| Compose input is immutable | [compose-artifact.schema.json](../schemas/compose-artifact.schema.json): manifest digest + pinned renderer + compose digest; plan `compose.up` must match artifact | Semantic digest checks in `validate.py` | Phase 1 renderer + agent fetch-by-digest |
| Approvals bind the plan and envelope | ApprovalRecord: plan + envelope digests, tenant, host, action and live validity interval | Plan/envelope edit, future/expired approval and live maintainer-quorum tests | Phase 1 approval service |
| Plan capabilities are authorised | `semantic_errors`: plan ⊆ envelope ⊆ policy, minus denials/forbidden | Semantic test "capability outside envelope allowlist" | Phase 1 policy engine |
| Tenant isolation in the graph | Tenant scopes must agree across envelope/host/policy/case/approvals | Semantic tests "cross-tenant host profile", "cross-tenant approval replay" | Phase 1 tenant-scoped retrieval |
| Host facts are fresh | Freshness derived from `observedAt`/`expiresAt` vs injected clock and policy max age; no `stale` boolean is trusted | `eval_stale-host-profile.json`, `eval_host-profile-expired.json` | Phase 1 preflight refresh + TOCTOU re-probe |
| Host cannot change undetected after preflight | Envelope `hostProfileRef` is digest-bound (`recordRef`) | Semantic test "host changed between preflight and execution" | Phase 1 agent re-probe before mutation |
| Compatibility uses real facts | Fixtures carry real manifest/host/policy records; CPU, Docker, Compose, filesystem, ports, RAM, storage, arch, OS all checked | `eval_insufficient-cpu.json`, `eval_unsupported-docker-version.json`, `eval_unsupported-compose-version.json`, `eval_filesystem-not-writable.json`, `eval_missing-mandatory-fact.json`, others | Phase 2 compatibility engine |
| Missing facts fail closed | Record schema errors map to `missing-mandatory-fact`; required schema inventory is fail-closed | `eval_missing-mandatory-fact.json`; deleting a schema fails `validate.py` (checked in review) | All phases |
| Timestamps are real and ordered | `FormatChecker` (date-time, rfc3339) is required to be active; ordering + expiry checked semantically | `adv_host-invalid-timestamp.json`; semantic test "host profile expires before it was observed" | All phases (server clock) |
| Sensitive environment defaults are rejected | Sensitive env names must use refs; key + synthetic-value scan over examples AND fixtures; detected values suppressed in diagnostics | `adv_manifest-sensitive-default.json`, `adv_envelope-secret-value.json`, safe-diagnostic test | Phase 1 secret broker and redaction |
| Privileged containers are out | `requirements.privileged` is `const: false` in the manifest schema | `adv_manifest-privileged.json`, `eval_privileged-not-supported.json` | Phase 1 agent refuses privileged specs |
| Policy cannot request single-success auto-promotion | Policy `autoPromoteSingleSuccess` is `const: false`; promotion workflow remains unimplemented | Policy schema; deployment-case example alone does not prove transition enforcement | Phase 4 promotion workflow |

| Follow-up invariant | Enforced locally | Proof | Runtime phase |
|---|---|---|---|
| Approval cannot override public-exposure denial | Semantic policy check | Policy public-denial test with rebuilt bindings | Phase 1 policy engine |
| Signed procedure order is respected | Exact capability sequence and unique step IDs | Reordered procedure and duplicate-step tests | Phase 1 executor |
| S-Print compatibility is metadata only | Separate experimental schemas and offline resolver always return executionAuthorized false | `scripts/test_sprints.py` | Later Worlds loader |

| Evidence supports the claimed outcome | Case evidenceRefs must resolve to evidence records for that case, bound to the canonical manifest digest, passing for success; successful installs require passing health-check evidence | Semantic tests "case references missing evidence record", "evidence belongs to a different case", "failing evidence", "evidence bound to a different release", "case bound to a different plan" | Phase 1 evidence store |
| Every mutation has compensation | Forward steps with mutating capabilities must declare a rollbackStepId identifying their specific compensation step; capability and resource must correspond per the compensation contract (proxy.configure on a hostname -> proxy.remove on that hostname). Renderer: structured Compose object via safe YAML emitter, exposure-aware bindings (internal unpublished, loopback/private on 127.0.0.1, public derived to an approval requirement); artifact bytes must equal a fresh render(manifest) | Semantic test "mutating proxy step without compensation" | Phase 1 executor compensation runbook |
| Signed program surface | Procedures declare parameterContracts (required + fixed parameters per capability); plan steps are checked against them and the procedure signature payload binds them | Semantic test "procedure-fixed parameter changed" | Phase 1 procedure registry |
| Compose bytes are the signed input | Deterministic renderer (scripts/render_compose.py) renders artifact bytes from the manifest; composeDigest is the SHA-256 of the exact bytes at the artifact localPath | Semantic test "compose artifact bytes tampered"; digest chain in validate.py | Phase 1 fetch-by-digest agent |
| Signature payload and signer trust | signature.payloadDigest binds record content minus the signature field (ADR-001); policy declares trustedSigners and algorithms | Semantic test "untrusted signer rejected even with rebuilt payload digest" | Phase 1 cosign verification |
| Revocation is fresh or deployment stops | Revocation list is freshness-bound; missing/stale/future lists fail closed when policy requires revocation checks | Semantic tests "revoked manifest in graph", "stale revocation list fails closed" | Phase 1 registry revocation service |
| Licence policy is enforced | Policy allowedLicenses (when non-empty) checked semantically and in compatibility hard filters | Semantic licence test; eval_licence-not-allowed.json fixture | Phase 2 catalogue licensing review |
| Canonical JSON is cross-language safe | ADR-001 defines the canonical form; vectors in schemas/canonicalization-vectors.json checked on every validation run | Vector checks in validate.py | Phase 1 Go/TS implementations must pass vectors |
| Dedicated secret scanning | scripts/secret_scan.py scans the tree for credential shapes and high-entropy assignments, wired into validate wrappers and CI | scripts/test_secret_scan.py | Runtime redaction/broker tests in Phase 1 |
| Renderer cannot be injected by manifest strings | Structured Compose object serialized with a safe YAML emitter; tests parse the output | `scripts/test_render_compose.py`: privileged injection, multiline values, four exposure bindings | Phase 1 agent consumes artifact by digest |
| Artifact provenance, not just self-consistency | Bytes are required, must match composeDigest and a fresh render(manifest) from the pinned renderer | Semantic tests "location removed", "bytes that are not render(manifest)" | Phase 1 fetch-by-digest agent |
## Honest limits

- The secret scan catches synthetic patterns and forbidden keys. It
  cannot prove an arbitrary string contains no secret. Free-text fields
  still exist; broker and runtime redaction must enforce value handling.
- Canonical digests detect change; they are not signatures. Authenticity
  still requires cosign verification in Phase 1.
- Fixtures prove the documented hard-filter rules on real-shaped
  records. They do not prove a live host behaves as its profile claims;
  that is the Phase 1 agent's re-probe job.
- Evidence is not yet linked through semantic graph checks; complete
  compensation, policy and signing contracts remain open. See the
  [follow-up review](PHASE_0_REVIEW_2026-10-29.md). Passing local checks
  does not close these gaps.
