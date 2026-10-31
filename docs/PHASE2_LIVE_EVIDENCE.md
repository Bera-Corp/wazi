# Phase 2 implementation and acceptance evidence

Updated 31 October 2026. Phase 2 remains in progress. The offline prototype is
complete; the expanded live/package/AI milestone is not closed.

## Acceptance matrix

| Milestone | Implemented | Verified evidence | Still required |
|---|---|---|---|
| Trusted live retrieval | Private Go bridge reuses paired TLS, signature/revocation cursor and resource/plan checks without consuming approvals; bounded Python worker ranks verified proposals | bera-003 accepted a fresh Gitea proposal at 09:47:58Z, with three real case citations and executionApproved=false; modified owner budget and expired envelope denied | Complete catalog discovery and package selection beyond explicitly supplied proposals |
| Cases and inspector | SQLite transactions, tenant/case identity, immutable case/evidence records, conflict rejection, exact-operation/procedure history; private HTTPS operator sessions and actual job status | Imported 14 terminal cases; restart preserved a stored context; unauthorized reads, bootstrap replay, cross-origin requests and missing CSRF rejected at 09:49:18Z | Complete the application-choice and approval-authoring UX; the current inspector accepts typed job JSON |
| PostgreSQL/MySQL/Nginx | Composition validation and source locks; build/attestation workflow prepared for pinned official recipes and base images | Five composition tests and two recipe-lock tests; no live lifecycle qualification claimed | Reviewed runtime profiles, secret injection, authenticated readiness, database backup/restore, signed releases, separate-instance reuse |
| Immich | Candidate composition constraints require its specific PostgreSQL/cache/proxy dependencies and coordinated recovery | Owner deferred live qualification on 31 October | Revised capacity/budget and full media/database lifecycle receipts |
| AI authoring and reuse | Hosted JSON-chat adapter, pinned-source/candidate validation and durable conservative cost reservations | Six tests with a fake provider; no model call or Meta integration evidence claimed | Authorized provider/model, verified pricing and spending cap; real output, qualification, review/signing, second install without inference |

The browser entry is /context on the existing private HTTPS listener.
An authenticated operator issues a one-minute bootstrap code through
POST /v1/context-sessions. Exchange it in the browser for a 15-minute
Secure/HttpOnly/SameSite=Strict session. Credentials are never embedded in
browser assets or local storage. This remains the pilot's single configured
operator, not an independent-human quorum.

POST /v1/contexts/retrieve accepts either one typed job or an object containing
one to four candidates for the same application, release and operation.
GET /v1/contexts/{contextId} returns the original immutable observation.
Retrieval does not consume approvals. Case citations in the inspector do not
rewrite an approved envelope. Deployment uses the existing case-bound approval
and per-mutation revalidation boundary.

## Live receipts

The first deployed context build came from qualification archive
sha256:e431ef2a53629b168716d31ef3a2fa1617cc864b24feb589700683cb3090c313,
staged at /home/wazi-dev/wazi-phase2/20261031T094152Z and installed under
/opt/wazi-dev/phase2-20261031T094152Z.

- Activation: 2026-10-31T09:44:00Z; 14 cases imported, 25 receipts skipped,
  zero rejected imports. All 19 journal files remained byte-identical.
- Accepted context:
  ctx_919b9f64431e4626606e0f6257390cfc15a4257345a1985202fcd569271d6ca9.
  It is a historical snapshot and must not be presented as a current approval.
- A fresh disposable 8 MiB managed-volume probe proved an over-limit write
  failed with quota denial; the volume and quota were cleaned up.
- The revocation list advanced to sequence 40 with its entries preserved;
  no existing revocation was removed.
- Browser protocol checks covered root, health, HTML and script responses,
  session protections and stored-context recovery after agent restart.
  These are HTTP/protocol checks, not a visual-browser usability acceptance.
- Gitea health remained pass. No application budget or volume was changed.

Raw receipts remain under that staging directory's live/ folder and
context-activation.json. The latest source and CI identities must be recorded
before claiming closure; the working tree includes subsequent hardening.

## Verification and runtime

Native Go 1.23.12 tests, race checks, vet and builds passed on bera-003.
Python 3.11 development/CI dependencies remain pinned in requirements-dev.txt.
The separately tested distribution Python 3.14 runtime uses jsonschema 4.19.2,
referencing 0.36.2, rpds-py 0.27.1, PyYAML 6.0.3 and rfc3339-validator 0.1.4.
Validation refuses to proceed without active date-time checking.

The 87 offline context checks and fixture metrics retain their original
offline interpretation. Native qualification now also includes live-store,
composition, candidate-authoring and package-recipe tests. Real model results,
database recovery and a second application installation remain separate gates.

## Package provenance and next acceptance

The source lock records exact official recipe commits, recipe SHA-256 values,
upstream package versions and base-image digests. Those upstream commits do
not have verified commit signatures. The Wazi build workflow records that
limitation and attests its own source-to-image and image-to-archive bindings;
it must never label these as signed upstream releases.

Complete PostgreSQL, MySQL and Nginx qualification in that order, preserving
the Gitea pilot. Admission requires the signed package, reviewed runtime
profile and aggregate authorized resource headroom. A successful standalone
Docker command is not acceptance through the Wazi execution journey.

Measure maintainer installation time and manual interventions against a
documented baseline. The three-minute demonstration must show useful Meta
output, review, actual progress and verified recovery. Preserve the judging
weights: Impact 30%, Meta tooling 25%, Technical execution 20%, Demo 25%.
