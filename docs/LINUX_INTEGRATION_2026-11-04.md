# Linux platform integration — 4 November 2026

The owner authorized live integration/testing of the four existing pilots plus
Uptime Kuma. This supersedes the earlier testing deferral for this workstream.
Customer-production migration and arbitrary additional applications are outside
this first integration scope.

## Current milestone

Committed source a5b571509a2ba186485c7b2de8e9673ce308d0f6 passed native Linux
qualification at 17:48:04 UTC: Go tests/race/vet, Linux/Windows vet, four service
builds, formatting, maintained Python suites, 31 community tests and secret
scanning. The later backup-keygen and allocation changes require their own
qualified source revision before deployment.

That source was deployed to the private control plane and pilot agent at
17:48:51 UTC. All 41 operation-journal files were preserved; private health,
root HTML, favicon and wordmark requests passed. The existing four application
containers remained running. The private community catalog subsequently started
on HTTPS localhost:9444 and its root/catalog API responded. OAuth is not yet
configured; authenticated contribution/install handoff is not claimed.

The Uptime Kuma image workflow is
[41880742775](https://github.com/Bera-Corp/wazi/actions/runs/41880742775).
It reports success and offers the named image transport artifact. Download,
independent provenance verification, signing, live installation and runner
qualification must be recorded separately before claiming acceptance.

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
package-budget-revision3.json; the runtime change must retain protected copies
of prior configuration, enforce quotas, and record a new owner-budget digest.

## Dependencies

GitHub OAuth configuration is needed for authenticated catalog use. A second
independent maintainer and Meta provider/model/spending authorization remain
outstanding. The runtime and off-host age identities must be provisioned via
protected files; private bytes never enter Git or chat. Mac access is needed
when its runtime integration begins, not for this Linux deployment.
