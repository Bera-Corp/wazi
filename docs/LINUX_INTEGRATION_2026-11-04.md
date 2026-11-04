# Linux platform integration — 4 November 2026

The owner authorized live integration/testing of the four existing pilots plus
Uptime Kuma. This supersedes the earlier testing deferral for this workstream.
Customer-production migration and arbitrary additional applications are outside
this first integration scope.

## Current milestone

Committed source a5b571509a2ba186485c7b2de8e9673ce308d0f6 passed native Linux
qualification at 17:48:04 UTC: Go tests/race/vet, Linux/Windows vet, four service
builds, formatting, maintained Python suites, 31 community tests and secret
scanning. Revision 6d3a248 subsequently passed the same native checks, including
the fifth backup-keygen binary, at 17:59:59 UTC and was deployed at 18:03:49 UTC.

That source was deployed to the private control plane and pilot agent at
17:48:51 UTC. All 41 operation-journal files were preserved; private health,
root HTML, favicon and wordmark requests passed. The existing four application
containers remained running. The private community catalog subsequently started
on HTTPS localhost:9444 and its root/catalog API responded. OAuth is not yet
configured; authenticated contribution/install handoff is not claimed.

The Uptime Kuma image workflow is
[41880742775](https://github.com/Bera-Corp/wazi/actions/runs/41880742775).
Its archive was downloaded and independently verified against the publisher's
archive digest and GitHub attestation for source a5b5715. The application image
digest is sha256:4388c18019f6eee3dbb4de9fafef4f3bd680586bf227e93aa336f0d8f2bac475.
The signed 2.5.9+wazi.3 package fixes SQLite selection and writable temporary
paths. Earlier local revisions are retained in private build artifacts.

The isolated Linux runner passed all five checks at 20:30:23 UTC: candidate
profile, startup HTTP, SQLite creation, restart HTTP, and synthetic-file
persistence. Its receipt is signed by the separate qualification-only key in
releases/phase2/uptime-kuma/qualification-20261104. This used a locally assembled
suite derivative of the verified application archive; the registry qualification
image was not used. The suite has no host mounts or egress and runs as UID 65534
with bounded CPU, RAM and temporary storage. PostgreSQL was restarted and its
authenticated SELECT 1 passed after each run. This is synthetic qualification,
not independent review, a persistent installation, or off-host recovery proof.

The approved storage allocation was applied at 18:03:51 UTC. Runtime and
off-host age recipients were configured for the six existing package slots;
private recovery identity material remains outside the repository. The expired
node certificate was repaired through the existing pairing flow at 18:05:41 UTC,
preserving the tenant and host identity. Automatic renewal remains open.

## Provisional code estimates

These remain engineering estimates against the documented scope, not objective
test percentages. They distinguish source coverage from accepted live behavior.

| Phase | Estimated code completion | Code remaining | Main gaps |
|---|---:|---:|---|
| 2 | 90% | 10% | Expanded Immich/runtime integration and remaining user-facing integration details; baseline measurements and provider setup also lack acceptance evidence. |
| 3 | 85% | 15% | Complete private installation handoff and integration of catalog, qualification, review and signed release as one user journey. Real runner/promoted-package receipts and independent review remain acceptance gates. |
| 4 | 80% | 20% | Database-engine major-version conversion adapters and complete Windows/WSL2/macOS lifecycle integration. Uptime startup migration/canary/downgrade code needs live qualification; source presence alone is not recovery proof. |

The full documented phases cannot honestly all be reported at 90% yet. The
selected Linux Uptime path is being integrated before broadening the remaining
adapters; existing release and resource checks remain mandatory.

## Approved allocation revision

The owner authorized 27 GiB application reservations, 11 GiB shared
cache/checkpoints, and the unchanged 10 GiB protected reserve, totaling 48 GiB.
The CPU/RAM runtime cap remains 4 CPU / 3 GiB. PostgreSQL may be stopped
temporarily for Uptime tests, with its data preserved and service restored
afterward. No existing volume deletion was authorized. The revised proposal is
package-budget-revision3.json. Protected prior configuration was retained and
the shared project quota reduced to 11 GiB. The current owner-budget digest is
sha256:332b4df657fb305c9e53493d5fac9f2346b390f139bdb4aadf54cccdbce31325.

## Dependencies

GitHub OAuth configuration is needed for authenticated catalog use. A second
independent maintainer and Meta provider/model/spending authorization remain
outstanding. Persistent Uptime installation, integrated review/promotion,
encrypted live backup/restore and off-host recovery still need their own
acceptance records. Private identity bytes never enter Git or chat. Mac access is needed
when its runtime integration begins, not for this Linux deployment.
