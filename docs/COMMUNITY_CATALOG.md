# Community catalog implementation

Status: implemented private-first development slice, 3 November 2026. This is
not Phase 3/4 closure, a public deployment, or a desktop compatibility claim.

## Try the UI

Run from the repository root:

~~~powershell
python -m community.preview
~~~

Open http://127.0.0.1:3690/apps. The read-only preview uses real SQLite catalog
data and denies every write. It has no sign-in, Docker, SSH or execution
connection. Ten curated projects include the existing four pilot applications
and the discovery shortlist. Immich remains deferred.

Historical pilot evidence is projected from the checked-in 1 November report;
it is not current host health or a portable download. Star counts are never
seeded from planning estimates. Refresh actual GitHub observations with:

~~~powershell
python -m community.cli refresh --database artifacts/community-preview/catalog.sqlite
~~~

The UI supports search, category, environment, release, operation, package
availability and popularity. Environment selection changes the coverage state;
it cannot grant execution eligibility. Detail views expose evidence, upstream
documentation and unsigned discovery starters. No package is published by
seeding an application.

## Run the authenticated service

Use the repository validation environment and requirements-dev.txt. The service
requires TLS and a fixed HTTPS origin; the bind remains loopback. Configure a
GitHub OAuth application with the exact callback ORIGIN/auth/callback. Set
WAZI_GITHUB_CLIENT_ID and WAZI_GITHUB_CLIENT_SECRET in the protected service
environment, never frontend files, command arguments or this repository.
Both fields are required together. Without them, browsing works and sign-in is
explicitly unavailable. Registering an OAuth app has not been performed here.

~~~text
python -m community.server --database PRIVATE/catalog.sqlite --objects PRIVATE/objects --origin https://localhost:9444 --certificate PRIVATE/server.crt --key PRIVATE/server.key
~~~

Use a protected private state directory. Trust the chosen server CA normally;
neither service nor CLI disables TLS verification. The server uses GitHub
authorization code/PKCE, one-use browser-bound OAuth state and no repository
write scopes. GitHub credentials are discarded after identity lookup. Local
session identifiers are hashed in SQLite; cookies are Secure, HttpOnly,
SameSite=Lax and host-prefixed. Mutations require an exact Origin and CSRF
value. Contributor workers use separately revocable, submission-only identities.

Private control-plane credentials never enter this service. Existing host
pairing, approvals and jobs remain in the private control plane. Dashboard
host/application/installation panes explain the absent connection instead of
fabricating records. Linking and listing private installations is still open.

## Portable packages

Version 1 packages are bounded ZIP archives of JSON files. recipe.json is a
closed index binding every file hash, app/release/revision, original-recipe
license, authors, pinned upstream commits, typed inputs, procedures, exact
environment observations and evidence references. licenses.json carries copies
of original/upstream license texts and attribution; it does not replace review
of the actual upstream licensing terms. Config inputs support
instance names, unprivileged ports and protected secret references.

The inspector rejects duplicate/traversing/symlink entries, oversized expanded
content, duplicate JSON properties, secret patterns, literal private home
paths in templates, unknown inputs, missing procedure bindings and manifest
architecture/runtime contradictions. It never extracts or executes archives.
Canonical manifest/procedure schemas are reused. Published indexes and their
manifest/procedures must verify through the existing Go recordtool and a
trusted operator policy. Environment coverage is an explicit maintainer-signed
claim with evidence citations; independent runner acceptance is still a gate.

Commands:

~~~text
python -m community.cli draft-index --directory CANDIDATE_FILES --metadata UNSIGNED_METADATA.json --output recipe-draft.json
python -m community.cli bundle-candidate --index recipe-draft.json --directory CANDIDATE_FILES --output candidate.zip
python -m community.cli inspect --archive reviewed.zip --recordtool RECORDTOOL --policy RELEASE_POLICY
python -m community.cli configure --archive reviewed.zip --recordtool RECORDTOOL --policy RELEASE_POLICY --inputs local-inputs.json --output NEW_INSTANCE_DIRECTORY
python -m community.cli publish --archive reviewed.zip --recordtool RECORDTOOL --policy RELEASE_POLICY --database PRIVATE/catalog.sqlite --objects PRIVATE/objects
~~~

draft-index does not invent pins or qualification: supply actual metadata first.
Unsigned candidates require candidate-state manifests and procedures without
signatures. Signed files need explicit review/signing before a release archive
can be published. Configuration generation writes inert JSON to an exclusive
new directory; it does not approve a profile or install an application.

Publication is local maintainer tooling, not a contributor HTTP permission.
Recipes are immutable by recipe ID/revision and objects are content-addressed.
Revocation disables downloads; private execution also requires its existing
signed revocation publication. Downloads do not bypass private preflight.

The old three-way shape switch is now release-owned versioned JSON in
services/internal/agentclient/reviewed-profiles.v1.json. Existing renderer
bindings and closed adapters are retained. This does not yet provide a generic
adapter for arbitrary new applications: adding Uptime Kuma execution still
requires a reviewed adapter and qualification, not simply uploading a recipe.

## Opt-in automatic contribution

In My workspace / Contributions, accept the original-recipe license and enable
contribution intake. Then connect and explicitly enable a local worker:

~~~text
python -m community.cli connect --state PRIVATE/worker --origin https://localhost:9444 --ca-file SERVER_CA
python -m community.cli consent-on --state PRIVATE/worker --accept-apache-2-original-recipes
python -m community.cli watch --state PRIVATE/worker --origin https://localhost:9444 --ca-file SERVER_CA --case-database PRIVATE/context.sqlite --mapping PRIVATE/export-mapping.json --candidate-directory PREPARED_CANDIDATES
~~~

Approve only the displayed code from your own worker in the authenticated
dashboard. The worker's protected identity expires after 30 days and can be
revoked from the dashboard. It cannot create sessions or approve deployments.

The mapping is operator-local and keyed by application ID. Each entry contains
tenantScope, releaseVersion, manifestDigest, recipeDigest, templateDigest and
the closed environment descriptor. These bindings prevent unrelated releases
or tenants from entering this worker's export. The public record omits tenant,
host, user, paths, logs and application files. Only evidence digests leave the
case store. Cases older than local opt-in are not backfilled.

Prepared candidate ZIPs created after opt-in can also be queued automatically.
They are checked before queueing and again before delivery. This shares the
actual reusable templates, not application data. Uncertain content stays local.
The server quarantines candidates; it never enables a runtime profile.
Original attribution and version variants remain explicit, and different
outcomes are retained rather than replacing previous reports.

The durable outbox deduplicates, retries at most five times and checks remote
consent before collecting. Remote consent changes cancel unsent records and
require local opt-in again; submissions bind the current consent revision.
Offline workers collect nothing until they can check consent. Consent changes
are not instantaneous on disconnected machines; server intake rejects old
consent regardless. Turn local collection off explicitly with consent-off.

Owners can withdraw their reports and quarantined candidates. Previously
downloaded published releases cannot be recalled. Reports never automatically
promote a release or become cross-tenant private retrieval history.

## API and tests

Public: /api/applications, /api/contracts, /api/starter and /api/downloads/HASH.
Authenticated: /api/me, /api/contributions, /api/consent, candidate submission/
withdrawal and worker authorization/revocation. Worker routes accept only
submission credentials, not browser sessions. There is no deployment route.

The 1.0.0 contract inventory exposes contribution, environment, unsigned recipe
and signed recipe definitions. Runtime checks additionally enforce cross-record
bindings and archive integrity.

~~~powershell
& ./.venv/Scripts/python.exe -m unittest community.test_community -v
node community/test_web.mjs
~~~

The browser test requires a local Chromium executable and puppeteer-core through
HALLMARK_PUPPETEER. Hallmark render checks use the same running preview and
preserve real returned states. No OAuth network call or independent community
contributor has been claimed from mock tests.

## Remaining acceptance

Real GitHub OAuth setup/login; private installation handoff and host listings;
supervised isolated runner and independent signer receipts; one newly reviewed
application adapter/package; a second installation; desktop qualification and
the remaining Phase 4 upgrade/migration/canary/supply-chain lifecycle matrix.
The owner confirmed Windows and Mac test machines are available; Mac connection
details will be supplied later. Docker Desktop was not found on this machine's
PATH or standard installation path; desktop tests remain unverified.

The first implementation commit, 301beb789f3f4693e65b1f5427cf5cb94cfedd9f,
passed [CI 41640674876](https://github.com/Bera-Corp/wazi/actions/runs/41640674876)
and committed-source native Linux qualification at 13:02:17Z. It included
22 community tests, the maintained suites, Go tests/race/vet/builds and the
secret scan. Subsequent TLS handshake isolation adds a real certificate-based
regression; its final source qualification is recorded separately.

Public ingress and the Wazi software distribution license are separate release
gates. Only original recipe contributions are designated Apache-2.0; upstream
licenses and attribution remain in force. This change does not license the
whole Wazi application or publish it publicly.
