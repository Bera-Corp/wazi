# Private alpha operations

Phase 5 implementation began on 5 November 2026. This is private operator
tooling; it does not report synthetic regression fixtures as alpha results.
The owner deferred independent review for this buildout. Owner authorization,
signatures, tenant isolation and runtime admission remain in place.

## Cohort and observations

Use scripts/alpha_ops.py with a private --database path outside the repository.
The curate command takes --input containing tenantScope and applications: five
to ten unique application IDs. Curation is immutable within a cohort; create a
new private database/cohort when changing membership.

The record command takes an observation containing schemaVersion 1.0.0,
eventId, trialId, tenantScope, applicationId, releaseDigest, environment, target
(local or vps), kind, outcome, attempt, deterministic, interventions, elapsedMs
and evidenceDigests. Environment uses the existing exact platform descriptor.
Retries retain the same trial, release, environment and target, advance attempts
consecutively and cannot overwrite successful trials. Conflicting event replay
is rejected; identical replay is idempotent.

Kinds/outcomes:

| Kind | Outcomes |
|---|---|
| deployment, rollback, restore | success, failure |
| planner | valid, invalid |
| compatibility | compatible, incompatible, false-compatible |
| safety | clear, secret-exposure, uncited-action, arbitrary-shell |

Safety and compatibility observations cannot be retried to erase a finding.
No raw logs, credentials, private host paths, beneficiary identities or prompts
are accepted. This ledger records operator-supplied evidence references; it is
not an independent assurance or cryptographic receipt-verification service.

## Import real private jobs

import-queue accepts --queue, --tenant, --environment, --target and --mapping.
Mapping is schemaVersion 1.0.0 and jobs, a job-ID dictionary. Each entry supplies
trialId, attempt, deterministic and interventions, so the importer never guesses
which separate jobs were retries or how much human help occurred.

Only matching terminal local queue requests/results are imported. Tenant, host,
case, envelope, plan and release bindings must match; unresolved mutations are
rejected. Install/upgrade, rollback and restore feed their respective measures.
Backups, uninstalls and read-only health checks do not inflate deployment counts.
Planner quality and safety audit coverage are never inferred from a successful
deployment. The importer performs no network requests and does not execute jobs.

## Metrics and private dashboard

The dedicated private page is /alpha. It shares navigation, typography and
branding with the overview and workbench. Open it through the existing private
HTTPS connection, then exchange an operator-issued one-use code. An unexpired
cookie session can move between /context and /alpha without entering another
code. End session clears that browser session; expiry clears private results.
The public catalog does not expose this ledger.

Readiness shows the five-app cohort, exact release digests, local/VPS recovery
coverage and measured reliability. Observations retains the latest twenty
records. Support offers receipt-backed ticket creation, evidence-bound state
changes and explicit-consent local feedback downloads. All actions display the
actual service response. Preview mode disables writes and labels supplied
snapshot data; absent data remains empty.

report --tenant TENANT emits numerator, denominator, threshold and state for
each metric. --html-output FILE writes an exclusive offline private dashboard.
The existing private workbench also exposes Refresh alpha progress through
authenticated GET /v1/alpha-report. Its database is alpha.sqlite beside the
operator-configured private context database, never the public catalog.

Thresholds follow the roadmap: first-attempt success >=90%, rollback >=95%,
restore verification 100%, schema-valid planner output after retry >=99%,
deterministic deployments >=80%, false-compatible decisions strictly <2%, and
zero secret exposure, uncited material actions or arbitrary shell execution.
An observed threshold violation is shown as a failure immediately, even before
the minimum sample count is reached. Passing a small sample remains insufficient.
First-attempt results remain failures even if a retry succeeds. Planner results
use the final attempt. Historical safety findings cannot be cleared by later
successes. Defaults require twenty observations per measure; change the reporting
minimum explicitly for exploratory reports. Zero samples means not measured.

Five applications must each have the same release deployed and restored on both
local and VPS targets. Deployment and restore must match the exact environment
within each target. Thresholds met means these recorded observations meet those
checks; it does not independently certify a release or award phase acceptance.

## Support and upstream feedback

ticket accepts schemaVersion, ticketId, tenantScope, applicationId,
releaseDigest, environment, category, severity and evidenceDigests. Categories
are installation, backup, restore, rollback, compatibility, usability and
metadata; severity is P0–P3. There are no free-text logs or identity fields.

transition takes eventId, tenantScope, ticketId, from, to and evidenceDigests.
The flow is open -> triaged -> resolved -> closed, with reopening supported.
Every change requires evidence and is durable/idempotent.

feedback --tenant TENANT --ticket ID --consent --output FILE produces a local
feedback file that omits tenant and ticket identities. Nothing is sent upstream.
Sending or posting remains a separate explicit owner instruction.

## Configure a private local target

Run the CLI as the account that owns the private context database. Choose a
private directory outside the checkout; create it with owner-only access. Use
the same directory as the configured context database so the worker can find
alpha.sqlite and its configuration. Do not point the public catalog at it.

~~~sh
python scripts/alpha_ops.py --database PRIVATE_DIRECTORY/alpha.sqlite configure --input PRIVATE_CONFIGURATION.json
~~~

The input has exactly tenantScope, applications (five to ten IDs), environment
(the existing exact platform descriptor) and target (local or vps). Obtain the
descriptor from that host's qualification record; do not copy a Linux descriptor
onto a desktop. This command records configuration and grants no qualification.
It creates alpha-cohort.json, alpha-environment.json and alpha-target.json with
exclusive owner-only writes. Conflicting existing configuration is rejected.
On Windows, restrict the directory ACL to the runtime account; POSIX mode bits
alone do not establish Windows privacy. The VPS setup helper supplies the
equivalent configuration from its existing qualified runtime.

The browser import form takes a terminal job ID, explicit trial ID, attempt,
intervention count and deterministic attribution. Tenant, application, release,
host environment and receipt digest come from protected server state. It never
submits or re-executes the referenced job. Ticket resolution and closure require
a successful matching application/environment receipt without pending recovery.

## Open alpha work

The five current pilots form the initial cohort. Discovery entries for the next
five applications are not qualified installations. Add curated releases through
the existing signed-package and runtime-profile path, then collect local/VPS
evidence through hands-on testing. OAuth setup, expanded app coverage, actual
reliability sample collection and the product walkthrough remain open.

Impact 30% uses the paired beneficiary measurement tool; Meta AI Tooling 25%
still requires an authorized real model contribution; Technical Depth 20% uses
the bound execution/recovery evidence; Demo 25% uses the demonstrated journey.
