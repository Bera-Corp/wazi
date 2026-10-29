# Phase 0 follow-up review — 29 October 2026

Scope: local working tree, including untracked contracts. This is not proof
of publication, GitHub CI or deployment success.

## Improvements confirmed

The baseline passed before this review's edits. The original findings now
have substantial coverage: closed parameters, shared types, required schema
inventory, typed procedures, separate rollback steps, real-record fixtures,
active timestamp formats, digest-bound references, approval records,
synthetic secret scanning, privileged-container rejection, validation
wrappers and CI action SHA pinning.

## Additional defects fixed

| Defect | Correction | Targeted proof in test_adversarial.py |
|---|---|---|
| Public proxy accepted despite policy denial | Denial overrides approval; positive example explicitly permits public exposure | Policy public denial |
| Procedure membership allowed reordering | Exact forward/rollback sequences and unique step IDs | Procedure order, duplicate step IDs |
| Quorum counted expired maintainers | Count distinct approvers after live-time and role filtering | Two-maintainer live quorum |
| Future approval accepted; envelope edits retained approval | approvedAt <= clock < expiresAt; required envelopeDigest | Future approval, envelope edit, boundary tests |
| Semantic path omitted max age/future observations | Enforce policy max age, future rejection and exclusive expiry | Host max-age/future tests |
| Rollback could target another Compose project | Validate both forward and rollback service/project bindings | Rollback project test |
| Citation IDs resolved without verifying hashes | Compare hashes for known source records | Forged cited hash |
| Parser stripped prerelease text into stable versions | Accept initial stable numeric matrix; reject unsupported formats | Three Docker version probes |
| Secret diagnostic echoed detected content | Suppress detected values | Secret diagnostic test |
| Existing virtualenv skipped dependency updates | Reinstall pinned requirements each run; propagate setup failure | PowerShell wrapper run; Linux wrapper reviewed only |

The example rollback now describes its actual install compensation: stop
the newly installed service and preserve volumes. It does not claim
upgrade/restore rollback coverage. Proxy compensation still needs a typed
lifecycle contract.

## Remaining Phase 0 completion blockers

- Evidence is schema-checked individually but not part of graph reference
  validation. Add missing/wrong-case/failing-evidence and app/release/plan
  mismatch tests.
- Bind executable procedure parameters and artifact bytes to signed inputs;
  sequence order alone is not a complete signed program. Define checkpoints
  and compensation for every mutation, including proxy configuration.
- Complete required-action derivation, licence-policy semantics and deny
  precedence across lifecycle operations. These checks are not a production
  policy engine.
- Specify signature payloads, signer trust, revocation freshness and
  cross-language canonical JSON before Go services consume these records.
  Demo signatures are placeholders.
- Strengthen older negative tests to assert their intended error. Several
  cross-tenant tests also fail because of an incidental stale digest. New
  tests rebuild unrelated bindings and assert a specific diagnostic.
- Free-text fields can carry sensitive strings. Reference schemas and
  heuristic scanning cannot prove all strings are secret-free; runtime
  redaction and broker tests remain necessary.
- Dedicated secret scanning and remote CI remain pending.

## S-Print integration

Experimental contracts, offline composition and its adversarial tests are
implemented. Real model execution, package loading, signature verification,
evaluation scores, training, actuation and marketplace UI remain planned.
See [Worlds and S-Prints](WAZI_SPRINTS_PLAN.md) for features and gates.

## Resolution addendum — 29 October 2026 (later session)

The blockers above were closed in a follow-up change, each with a
focused regression test in `scripts/test_adversarial.py` or a fixture:

| Blocker | Resolution |
|---|---|
| Evidence not graph-validated | Evidence records are in the semantic graph: case refs must resolve, belong to the case, match the canonical manifest digest, and pass for success; successful installs require passing health-check evidence. Five new semantic tests. |
| Procedure parameters / artifact bytes not bound; proxy compensation | Procedures declare `parameterContracts` (required + fixed parameters), checked per step and bound by the procedure `payloadDigest`. Compose bytes are rendered deterministically by `scripts/render_compose.py`; `composeDigest` is the byte hash and tampering is detected. Every forward mutation (including proxy) requires a resolving `rollbackStepId`; proxy compensation uses the new typed `proxy.remove` capability. |
| Required-action derivation, licence semantics, deny precedence | Licence allowlists enforced semantically and in hard filters (`eval_licence-not-allowed.json`). Deny precedence and destructive-action derivation were verified by targeted tests; remaining lifecycle derivation is exercised per operation in the semantic approval pass. |
| Signature payloads, signer trust, revocation freshness, canonical JSON | [ADR-001](ADR-001-canonical-json-and-signing.md) specifies the canonical form, payload (`payloadDigest`), signer allowlist in policy, and freshness-bound revocation lists that fail closed when stale or missing. Cross-language vectors live in `schemas/canonicalization-vectors.json` and are checked every run. Cryptography remains Phase 1; Phase 0 checks binding, not authenticity. |
| Weak negative tests | New cases assert specific diagnostics (see test names in `scripts/test_adversarial.py` output). |
| Free-text secrets | Structural controls plus the new dedicated scanner; the honest limit stands — runtime redaction/broker tests are Phase 1 work, recorded below. |
| Dedicated secret scanning | `scripts/secret_scan.py` + `scripts/test_secret_scan.py`, wired into `validate.ps1`/`validate.sh` and CI. Remote CI remains unverified until this work is published. |

Still open (not Phase 0 contract blockers): cryptographic signature
verification, runtime redaction and secret-broker behaviour, registry
signing/revocation service, and GitHub publication/CI. Those are
Phase 1 delivery items, tracked in [BACKLOG.md](BACKLOG.md).
## Second resolution addendum — 29 October 2026

A further review reproduced six gaps beyond the tests above. All are
closed in the reference implementation and contracts:

| Gap | Resolution |
|---|---|
| Environment defaults could inject privileged Compose structure | Renderer builds a structured Compose object and serializes with a safe YAML emitter (PyYAML, pinned). Parsed-output tests in `scripts/test_render_compose.py` prove a newline/`privileged: true` default stays a plain value and multiline defaults survive intact. |
| Renderer ignored port exposure | Exposure contract: internal ports are not published (expose only), loopback/private bind `127.0.0.1`, public publishes and derives a public-exposure approval requirement in semantic validation. Parsed binding tests for all four exposures. |
| Artifact validation could be skipped or rebound to unrelated bytes | `localPath` is required, artifact bytes are required, and the bytes must match both `composeDigest` and a fresh `render(manifest)`. Missing-location and mismatched-render tests added; `bind_graph` now rebuilds artifact/procedure bindings so these tests fail for their intended reason. |
| Unsigned rollback procedures passed | Both procedures must carry a signed review state and signature; a signed deployment graph must also have policy signer-trust configuration (`trustedSigners` and `algorithms`). |
| Resolving rollback pointer did not prove compensation | Pointers identify the specific compensation step. Capability and resource must correspond per the compensation contract (e.g. `proxy.configure` on a hostname must be compensated by `proxy.remove` on that hostname; `compose.up` by `compose.down` on the same service/project). Wrong-capability compensation is rejected with a specific diagnostic. |
| Scanner blind spots | Exemptions apply only to the matched `secret://` value (not the line), quoted JSON keys are scanned, URI schemes are not mistaken for assignments, and exclusions are reviewed root-relative paths/prefixes, never basenames. Tests cover a `validate.py` outside `scripts/`. |

Full gate: `scripts/validate.ps1` exits 0, including the new renderer
and scanner cases.
## Third resolution addendum — 29 October 2026

Five adjacent cases from a further review are closed:

| Gap | Resolution |
|---|---|
| Public manifest ports bypassed policy denial | Denial precedence now applies to every public exposure source (manifest ports and proxy steps); approval cannot override it. Regression test uses a public port with a private proxy under `allowPublicExposure: false`. |
| YAML safety did not stop Compose interpolation | Literal defaults escape every `$` as `$$`, the documented Compose literal-dollar form, so `${HOST_VALUE}`-style defaults remain literal and cannot pull host/secret values implicitly. Parsed-output test included. |
| Env/volumes copied to every service | Environment entries and volumes accept explicit `service` ownership. With multiple services, unowned entries are rejected by the renderer; scoped entries reach only their owning service (the worker in the test gets neither `APP_SECRET` nor the app volume). |
| Checkpoint requirements unenforced | Typed checkpoint contract: envelope `rollbackStrategy.checkpoint` (`backupId`, `volume`) is required when checkpointing is required, and semantic validation requires a matching `volume.backup` step before the first mutation. The example install graph now carries `step_checkpoint`; removing it is a regression test. |
| Declared renderer versions unverified | Semantic validation requires the artifact's declared renderer version to equal the installed `RENDERER_VERSION`; version dispatch is explicitly unsupported. A `99.0.0` declaration is rejected with a specific diagnostic. |
## Fourth resolution addendum — 29 October 2026

Two end-to-end acceptance gaps are closed:

| Gap | Resolution |
|---|---|
| Complete graph bypassed compatibility | `semantic_errors` now runs the same `decide()` hard filters on the actual deployment graph (manifest, host, policy, injected clock). End-to-end tests prove a fully rebound graph with 1 MB RAM (`insufficient-ram`) or a Windows host (`unsupported-os`) is rejected despite valid schemas, fresh observations, and consistent bindings. |
| Secret injection ignored service owner | Secret injection is bound to the `(service, environment name, secretRef)` triple from a manifest environment entry owned by that service. Every environment/volume/port owner must resolve to a manifest service, unowned entries in multi-service manifests are rejected, and the renderer raises on unknown owners instead of silently omitting entries. A two-service test targets the secret at the non-owning worker and is rejected. |
Follow-up: the secret-ownership triple check now applies to forward and rollback steps alike; a rollback `secret.inject` targeted at a non-owning service is rejected with the same specific diagnostic (regression in `scripts/test_adversarial.py`).
