# Phase 1–5 closeout ledger

**Status:** updated 5 November 2026. This ledger separates provisional source
estimates from verified acceptance evidence. A 90% code estimate is not a
phase acceptance score, a security sign-off, or production approval.

## Repository snapshot and review boundary

The review snapshot was main commit
6ed13235912b8c9bd728cfd9d58764334d52321c, equal to origin/main before these
documentation edits. GitHub CI run
[41924688779](https://github.com/Bera-Corp/wazi/actions/runs/41924688779)
completed successfully for that exact SHA. It does not cover the documentation
edits made after that snapshot.

A standard static source and documentation review completed against that
snapshot. It recorded two unresolved findings below. This pass did not run
tests, execute the application, inspect current VPS state, inspect credentials,
or change a remote service. The review marked dynamic and live coverage partial.
The Hallmark workspaces and unrelated untracked files were preserved.

## Provisional phase status

| Phase | Code estimate | Recorded evidence | Remaining work |
|---|---:|---|---|
| 1 | 90% | The five gates for the bounded Gitea pilot are recorded as accepted, including pairing and reboot recovery. | Qualify automatic certificate renewal and automatic application recovery. Independent-human review is deferred for the current closeout; this does not change the bounded pilot evidence. |
| 2 | 90% | Offline retrieval and live Gitea, PostgreSQL, MySQL and Nginx journeys; signed Uptime Kuma package/image evidence; live same-release Uptime upgrade and encrypted backup/restore; signed registry publication and revocation sequence 60. | Capture the paired beneficiary task baseline; obtain owner-approved provider/model/spend details before any real Meta model call; decide whether to qualify Immich later; refresh live acceptance evidence when testing resumes. |
| 3 | 90% | Tenant-scoped candidate workflow, isolated Linux runner and signed passing receipt, catalog/private handoff, candidate review and promotion interfaces. Candidate cand_244ed888373fa82d86d5778b698232dd has a passing receipt and remains pending owner review; it is not promoted. | Complete owner review, signed candidate promotion and the hands-on catalog-to-install workbench journey. Independent reviewer participation is deferred for this closeout. |
| 4 | 90% | PostgreSQL/MySQL encrypted-checkpoint code and MySQL disposable-database preflight; live same-release Uptime upgrade, encrypted backup and restore; Windows off-host ciphertext decryption and recovered SQLite integrity check. | Test preflight and failure paths; qualify application restore on an independent host; finish Nginx and engine-major migration support; exercise canary, downgrade and cross-version recovery; qualify each claimed OS/runtime. Windows did not run the application restore. |
| 5 | 90% | Five-app alpha cohort configuration, private readiness and observation pages, receipt intake, support workflow, reliability ledger and reporting tools. | Run the owner's local and VPS product journey, record real observations, meet every alpha threshold with sufficient samples, configure required OAuth, and expand the qualified cohort toward ten applications. No alpha threshold is currently claimed as met. |

The Phase 2–5 percentages are engineering estimates of source coverage from
the buildout checkpoint. Phase 1's percentage is likewise provisional.
The remaining code estimate is 10% for each phase; the acceptance evidence
still required is listed separately above. No phase is at 100%.

For the detailed dated Linux receipts, see
[Linux integration](LINUX_INTEGRATION_2026-11-04.md). The historical
[Phase 2 evidence matrix](PHASE2_LIVE_EVIDENCE.md), [Phase 3 workflow](PHASE3_CANDIDATE_WORKFLOW.md),
[Phase 4 backup guide](PHASE4_ENCRYPTED_BACKUPS.md), and
[Phase 5 operations guide](PHASE5_ALPHA_OPERATIONS.md) retain their original
receipts and now link back to this status.

## Open static-review findings

These are source findings, not dynamically reproduced or fixed in this pass.
Keep both open until the later compatibility and security pass.

1. **Shared candidate-quarantine availability — medium.**
   The store applies a 100-row limit per owner but checks archive bytes only
   against a shared 64 MiB total. A consented contributor can consume that
   pool and block other contributors from submitting. The impact found is
   limited to candidate intake; catalog reads and package execution were not
   shown to be affected. Relevant source: [community/store.py](../community/store.py),
   [community/server.py](../community/server.py) and
   [community/packages.py](../community/packages.py). A per-owner byte
   allowance needs a deliberate resource-policy choice before implementation.

2. **Temporary artifact URL file permissions — low, conditional.**
   [scripts/prepare_phase2_transport.py](../scripts/prepare_phase2_transport.py)
   stores short-lived signed artifact download URLs in a caller-selected file
   without enforcing owner-only POSIX
   permissions or a Windows owner-only ACL. A local reader of a permissive
   file may use the URL while it is valid. The output location and actual
   permissions were not inspected. Enforce privacy before writing the URL and
   fail closed if the platform's protection cannot be established.

## Owner's next pass

1. Exercise the private UI journey: catalog discovery, package details,
   candidate review, approval, install progress, health and one visible failure
   state. Capture a before/after beneficiary task with elapsed time and
   interventions.
2. Review the pending Uptime Kuma candidate and promotion evidence. Record the
   owner decision; do not infer approval from its passing qualification receipt.
3. Collect alpha observations for the configured cohort on local and VPS
   targets. Report each numerator, denominator, threshold, release and
   environment. A small sample is not an alpha pass.
4. Decide whether to configure OAuth, authorize a Meta provider/model and spend
   cap, or defer those optional integrations. Immich capacity and recovery
   remain deferred.
5. Later, run the product, security, native Linux and platform recovery checks.
   Mac qualification needs access to the Mac machine. Keep the video work in
   its separate workspace and distinguish concept scenes from demonstrated
   behavior.

The owner deferred independent-review participation and hands-on testing for
the current buildout. The owner review/approval flow, signatures, tenant
isolation, runtime admission and mandatory recovery gates remain in force.
