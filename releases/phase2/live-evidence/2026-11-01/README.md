# Phase 2 bounded live evidence — 1 November 2026

Phase 2 is in progress. This bundle records the reviewed smaller-package pilot
on bera-003; it does not declare production readiness or overall closure.

Deployed source: 2547e84127b23398f68b2d0b3ffab75ccdc876c0.
[Exact-source CI passed](https://github.com/Bera-Corp/wazi/actions/runs/41295560077).
Committed-source archive SHA-256:
462b4e401142abcc8a28f2595a3ceadd6e14ec836a4683606267d5e1421eaa97.
Native Linux tests/race, vet, formatting, builds and maintained Python suites
passed before activation at 10:03:04Z. The image build and verified archive
attestations came from [run 41191007408](https://github.com/Bera-Corp/wazi/actions/runs/41191007408).
Signed package manifests and procedures remain in the adjacent release folders.

| Acceptance | Receipt | Result |
|---|---|---|
| PostgreSQL install, backup, mutation, restore, second instance | [Proof](postgresql-proof.json), [operation digests](host-report.json) | Original row recovered; post-backup table absent |
| MySQL equivalent lifecycle | [Proof](mysql-proof.json), [operation digests](host-report.json) | Original row recovered; post-backup table absent |
| Nginx config validation/recovery and second instance | [Proof](nginx-proof.json), [operation digests](host-report.json) | Original response/config hashes restored |
| Trusted retrieval denials | [Negative retrieval](negative-retrieval.json), [revocation](revoked-release-denial.json) | No journal changes |
| Real outcome reuse in ranking | [Learning check](learning-check.json) | Relevant success +2 and readiness failure -2; execution remains unapproved |
| Browser review, approval and actual progress | [Inspector capture](inspector.png), [completed job](host-report.json) | Nginx backup job_fc7795c988a122ef39db99e5 completed at 09:58:37Z |
| Restart durability and preserved Gitea | [Host report](host-report.json) | 41 journals unchanged; zero pending reconciliation; 35 cases, 41 evidence records, 28 contexts |

The screenshot shows an immutable historical proposal and a real terminal job,
not a fresh approval. The final host report was captured at 10:12:40Z and
includes the source identity, application limits and journal SHA-256 values.
Observed application ceilings total 2304 MiB RAM / 3.25 CPU, within the
approved aggregate 3072 MiB / 4 CPU cap. Gitea's repository proof stayed exact.

Initial PostgreSQL and Nginx qualification failures and the intentional MySQL
readiness failure remain recorded; see [the narrative](../../../../docs/PHASE2_LIVE_EVIDENCE.md).
Same-release SQL recovery is qualified; migrations/downgrades and automatic
compensation are not. Deliberately interrupted new SQL restores and physical
disk-full recovery have no live acceptance receipt in this bundle.

Owner-deferred: Immich live qualification and model/provider connection.
The AI provision is disabled, with zero real calls or spending in this run.
Actual model-authored promotion/reuse, beneficiary/manual-baseline measurement
and the final three-minute film remain open. Independent-human quorum remains
a production blocker. The node certificate expires 2 November at 08:26:55Z;
fresh signed revocation state is required before new mutations.
