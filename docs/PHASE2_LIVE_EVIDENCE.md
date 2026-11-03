# Phase 2 implementation and acceptance evidence

Updated 3 November 2026. Phase 2 remains in progress. The offline prototype is
complete; the expanded live/package/AI milestone is not closed.

## Acceptance matrix

| Milestone | Implemented | Verified evidence | Still required |
|---|---|---|---|
| Trusted live retrieval | Paired Go bridge, signature verification, durable revocation cursor and resource/plan gates; bounded Python ranking without approval consumption | Live revoked release, invalid signature, modified budget and expired context denied without journal changes on 1 Nov | Selection is bounded to reviewed catalog profiles; arbitrary repository discovery is not implemented |
| Cases and inspector | SQLite transactions, immutable scoped case/evidence records; operation/procedure/release/host history; authenticated app choice, exact-case approval and real journal observations | Browser approval-to-completion passed; 35 cases, 41 evidence records and 28 contexts at 10:12Z survived agent restart. Relevant MySQL success/failure cases affected fresh retrieval | Beneficiary measurement and final film; historical receipt formats remain immutable |
| PostgreSQL/MySQL/Nginx | Closed profiles, instance-bound renderer 0.5, protected runtime secret references, authenticated SQL readiness, logical checkpoints and validated Nginx restore | All three completed install/recovery and separate-instance reuse. PG 17.11.0+wazi.1, MySQL 8.4.13+wazi.1, Nginx 1.30.6+wazi.2. Signed bundles, build attestations, native qualification and exact-source CI linked below | Final demonstration; no cross-version database migration is qualified |
| Immich | Candidate composition constraints require its specific PostgreSQL/cache/proxy dependencies and coordinated recovery | Owner deferred live qualification on 31 October | Revised capacity/budget and full media/database lifecycle receipts |
| AI authoring and reuse | Disabled provider provision, explicit opt-in runner, candidate validation and durable cost reservations | Seven fake-provider/setup tests; zero real calls. Owner deferred connection on 1 Nov | Later provider authorization; actual Meta output, model-generated promotion, cost/latency and inference-free reuse remain unproven |

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
database recovery and a second application installation now have the bounded
live receipts below. Real model results remain deferred.

## Package provenance and next acceptance

The source lock records exact official recipe commits, recipe SHA-256 values,
upstream package versions and base-image digests. Those upstream commits do
not have verified commit signatures. The Wazi build workflow records that
limitation and attests its own source-to-image and image-to-archive bindings;
it must never label these as signed upstream releases.

PostgreSQL, MySQL and Nginx qualification completed in that order, preserving
the Gitea pilot. Admission requires the signed package, reviewed runtime
profile and aggregate authorized resource headroom. A successful standalone
Docker command is not acceptance through the Wazi execution journey.

Measure maintainer installation time and manual interventions against a
documented baseline. The three-minute demonstration must show useful Meta
output, review, actual progress and verified recovery. Preserve the judging
weights: Impact 30%, Meta tooling 25%, Technical execution 20%, Demo 25%.

## Smaller-package scope and budget

The owner explicitly approved 4.5 CPU / 4096 MiB RAM / 49152 MiB storage,
with an enforced aggregate runtime limit of 4 CPU / 3072 MiB RAM. Within
that cap, application volume reservations are limited to 25 GiB, the shared
image/checkpoint/scratch pool to 13 GiB, and the protected reserve remains
10 GiB. The package-budget proposal records the allocation breakdown.

Only the reviewed PostgreSQL, MySQL and Nginx profiles are enabled by the
operator registry. Each has two fixed instance slots. Reuse is sequential:
both slots use the signed private port, so one must stop before the other
starts. Uninstall retains data. Synthetic qualification data may be removed
only through an explicit scoped operator step; Gitea data is preserved.

These package procedures support same-release install/reapply, backup,
restore and container removal. Cross-version migrations and database
downgrades are rejected by an instance release anchor. No automatic rollback
is claimed. Signed operation compensation declarations remain mandatory;
they do not automatically execute. Restores retain a digest-bound pre-restore
checkpoint. Nginx validates staged configuration before stopping its instance.

Renderer inputs bind the instance project, protected secret file references
and image mapping. Secret values remain outside contracts, model inputs,
plans and receipts. SQL clients read protected client files rather than
password arguments or environment values.

The 12:17:52Z native qualification passed tests, race checks, vet, builds and
the maintained Python suites. Activation at 12:19:11Z preserved all 19 journal
files byte-for-byte and Gitea health. Later journal-observer and installer
changes require their own final source/CI qualification before closure.

## Live smaller-package qualification — 1 November

PostgreSQL completed install at 08:28:26Z, backup, deliberate row/table
mutation and restore at 08:29:30Z. The original proof returned and the
post-backup table was absent. A separate instance installed at 08:31:05Z.
MySQL repeated the same lifecycle, restoring at 08:38:30Z and installing
its second instance at 08:38:59Z. Both proof checks hashed to
f57ce7e53d8958188d270d92ae597c9bbf52405db9e79d2926eeb5272ddbb2a4.

Nginx restored the original configuration and HTTP response at 08:51:34Z
after a deliberate configuration mutation/reload. Its response hash was
5cdad38ccc660777700e25756f82f976c549de1f91a8ef633d5a9f0315b03a69;
configuration hash d0dde686126b7a61711ffc6c2a76552f906ce734675809d36673da8cf8db8683.
Clean first- and second-instance installs completed at 08:54:20Z and 08:54:41Z.
Uninstall retained data; explicit operator cleanup removed only the verified
synthetic first-instance volumes, retaining backups and pre-restore checkpoints.

Failures are retained: an initial PostgreSQL install never started because
RootlessKit could not see new /etc bind sources. Its empty volume was inspected
and removed, and the operation reconciled to blocked with observation digest
sha256:e02ccbd3fe37c499d25cbff80d2eec9ee1139c266c0471dfc8bcda623a888948.
Protected secret sources now reside under /srv/wazi-runtime/secrets.
Nginx's first runtime attempt failed readiness because its reviewed profile
needed CHOWN for cache initialization. A newly approved reapply recovered it;
both subsequent clean installations passed. Nginx revision 2 uses 128 MiB.
The 64 MiB allocation adjustment stayed within the approved owner/runtime caps.

A deliberately stopped MySQL instance caused an approved backup to fail
readiness after 93 seconds, before any mutation intent or checkpoint write.
A fresh approved reapply recovered it at 08:57:45Z. Live revocation sequence
52 temporarily denied the qualified Nginx release; sequence 53 removed only
that test entry and restored eligibility. All 40 journals were unchanged
during revoked, tampered-budget, expired-context and invalid-signature probes.

The 09:07:42Z report found four healthy running applications, 2304 MiB of
container RAM limits and 3.25 CPU in total, below the enforced 3072 MiB / 4 CPU
runtime cap. Both databases passed authenticated SELECT 1. The agent restart
preserved all journals, case/evidence counts and a stored inspector context,
with zero pending reconciliation. Gitea's original repository proof remained
728836b371192e623885770f40710bbe833d0f76d6cdbadfd9495621c25dc3e6.

The expired node certificate was replaced through a fresh operator pairing
code at 08:26:55Z, with the server anchor unchanged and all 20 then-existing
journals byte-identical. The replacement expires 2 Nov at 08:26:55Z.
The pilot still needs deliberate credential maintenance and fresh signed
revocation publication before mutations; acceptance does not imply unattended
production operation or an independent-human approval quorum.

Raw lifecycle receipts are under package-evidence/ in staging directories
20261031T132057Z (PostgreSQL), 20261101T083459Z (MySQL) and
20261101T084810Z (Nginx, negative tests and restart report). The final
committed-source qualification and CI receipt supersede intermediate builds.
The measurement protocol and video walkthrough remain capture plans;
no human-time savings, beneficiary outcome or actual Meta inference is claimed.

## Final committed-source snapshot

The [public evidence bundle](../releases/phase2/live-evidence/2026-11-01/README.md)
links lifecycle proofs, denied requests, real case ranking and the inspector
capture. Deployed source is 2547e84127b23398f68b2d0b3ffab75ccdc876c0;
its [CI run 41295560077](https://github.com/Bera-Corp/wazi/actions/runs/41295560077)
passed. The committed-source archive SHA-256 is
462b4e401142abcc8a28f2595a3ceadd6e14ec836a4683606267d5e1421eaa97.
Native tests, race checks, vet, formatting, builds and maintained Python suites
passed before activation at 10:03:04Z.

The visual browser journey selected Nginx, prepared a live proposal, reviewed
and approved its backup, and observed job_fc7795c988a122ef39db99e5 complete
at 09:58:37Z. The final source presents readable journal events with the raw
receipt collapsed. Its screenshot preserves the historical proposal time;
it is not a fresh execution authorization.

At 10:08:41Z a fresh MySQL backup retrieval cited both the completed backup
and the deliberate database-not-ready failure, applying +2 and -2 ranking
contributions respectively. It did not approve or queue execution. At
10:12:40Z all four applications remained healthy, all 41 journals were
unchanged across restart, pending reconciliation was zero, and the store
retained 35 cases, 41 evidence records and 28 contexts. Gitea proof was exact.

Phase 2 remains open for beneficiary/manual-baseline measurements, actual
authorized model authoring/promotion/reuse and the final three-minute film.
Provider connection and live Immich qualification are explicitly owner-deferred.
The single-approver pilot and same-release recovery limits remain in force.

## Live pilot refresh — 3 November

The 07:45:34Z host report is a dated observation, not a continuing health
claim. The node certificate was renewed at 07:36:01Z and is valid through
4 November at 07:37:01Z. The server trust anchor did not change; 41 journal
files remained byte-identical, application health passed and the worker was
active.

The 07:45:34Z report observed four healthy applications, 2304 MiB of aggregate
container memory limits and 3.25 CPU, authenticated database readiness checks,
zero pending reconciliation, and unchanged case-store counts (35 cases,
41 evidence records and 28 contexts) across agent restart. The Gitea proof
remained
728836b371192e623885770f40710bbe833d0f76d6cdbadfd9495621c25dc3e6.
The host still reported source commit
2547e84127b23398f68b2d0b3ffab75ccdc876c0 and archive SHA-256
462b4e401142abcc8a28f2595a3ceadd6e14ec836a4683606267d5e1421eaa97.

Revocation sequence 57, with zero entries, was signed, verified locally with
Cosign and accepted by the control plane at 07:45:08Z. That record expired at
08:08:55Z. Its package-manifest refresh did not complete because the publisher
was run without the staged package tree. The corrected staging order is present
in the working tree but has not been rerun against the live registry. Sequence
57 is historical evidence only; refresh and verify the complete package and
revocation set before another live mutation.

This refresh does not close live acceptance. Beneficiary manual-versus-Wazi
measurements, an authorized Meta AI authoring/promotion/reuse run, fresh
registry publication evidence and the final three-minute film remain open.
Receipts for this observation are in
[the 3 November evidence directory](../releases/phase2/live-evidence/2026-11-03/).

## Native qualification of the current working tree — 3 November

At 09:10:20Z, Bera-003 qualified a staged working-tree archive with source
commit 00757075c2c032ccf1d3ca47eed58b6ff16d9948 and SHA-256
6c26cfa073b409d78a33e3eb9645cae3a46a6686c6de77bc6bfbebff1c1a93e1.
The archive is identified as uncommitted source. Its local staging receipt and
archive are under artifacts/phase2/20261103T091002Z; the remote copy is under
/home/wazi-dev/wazi-phase2/20261103T091002Z.

The full Go suite, native Linux race packages, Linux and Windows vet, gofmt,
four service builds, all maintained Python suites and the repository secret
scan passed. This qualified the staged implementation, including the
candidate-promotion recovery regression and the active .age checkpoint
corruption fixture.

This is native working-tree qualification, not CI on a committed final source
revision. It did not deploy binaries, restart services, publish revocation or
package records, or perform live application mutations. Live Phase 2
acceptance and the remaining beneficiary, Meta tooling and film evidence
remain open.

## Owner scope amendment — 3 November

Automated model authoring/provider integration is now deferred. Codex assists
through this chat and exported/imported candidate files. It is not a live
in-product model connection or evidence of Meta tooling use. The final film
is a release-wide deliverable rather than a retrieval-engine acceptance gate.
Phase 2 still needs refreshed live acceptance and the observed maintainer
manual-versus-Wazi comparison. Historical rows above retain their original
reported scope and timestamps; this amendment supersedes their closure wording.
