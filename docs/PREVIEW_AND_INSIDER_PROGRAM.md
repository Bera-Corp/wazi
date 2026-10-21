# Wazi Preview & Insider Program
> **Status:** Proposed architecture / specification (not implemented)  
> **Date:** 2026-10-21  
> **Owner:** Wazi product and engineering  
> **Related:** [Wazi README](../README.md), [Architecture](ARCHITECTURE.md), [Context Engineering](CONTEXT_ENGINEERING_PLAN.md), [Threat Model](THREAT_MODEL.md), [MVP Roadmap](MVP_ROADMAP.md)  
> **Decision required:** Approve product boundaries, consent model, data processing roles, and phased scope before implementation.

## 1. Executive decision

Add an optional **Wazi Preview & Insider Program** to Wazi's marketplace and release lifecycle. Maintainers can invite people to test signed prerelease builds, ask users to help test particular features, and receive structured, privacy-preserving diagnostics and reproducible bug reports across supported devices. Testers join or leave through a Wazi application listing and can control diagnostic collection separately from participating in a preview.

**Core promise to users:** "Try upcoming versions, understand the risks, and control the diagnostics you share."

**Core promise to developers:** "Know which release failed, on which compatible environment, with enough relevant evidence to reproduce and fix the problem."

**Architectural rule:** Wazi manages release-channel intent, eligibility, consent, metadata, evidence and feedback. Native installers/package managers and approved external distribution channels remain responsible for installation where applicable.

**Delivery rule:** Preserve the current project's narrower Linux x86_64 Docker Compose, signed-manifest, local/BYO VPS deployment wedge. The preview program initially layers onto that lifecycle; generalized native desktop crash telemetry is a later SDK/adaptor capability. The hackathon demo can show channel enrollment, explicit diagnostic preferences and one real sanitized issue report, but must not fake machine instrumentation or build a production telemetry ingestion system.

## 2. Product context and positioning

Existing Wazi is a curated, security-focused self-hosted app marketplace, deployment system and governed context platform, not yet a finished universal desktop app store. Its existing README prohibits arbitrary repository execution and insists on signed manifests, typed capabilities, compatibility checks, review and recovery.

The Preview Program supports Wazi's wider aims:
- make application adoption more informative than GitHub star counts;
- make maintenance feasible for independent and AI-assisted developers;
- collect *consented and reproducible* installation, upgrade, crash and compatibility evidence;
- surface an application's maturity and known instability honestly on Wazi listings;
- make bug-reporting accessible to nontechnical users while preserving upstream GitHub Issues/Discussions as maintainers' preferred source-of-truth;
- provide a channel bridge to platform-native testing systems where the publisher authorizes it.

The program must not imply association with the Microsoft Windows Insider Program. Market it as **Wazi Preview** or **Wazi Insider Track**, with a notice that each preview is published by its own independent developer.

### Audience and jobs-to-be-done

| Persona | Need | Wazi response |
| --- | --- | --- |
| Everyday end user | A reliable default application | Stable channel; no preview nagging or surprise migration |
| Volunteer tester | Early access; ability to help without coding | Explicit join, build notes, issue wizard, diagnostic controls and exit |
| Indie maintainer | Reproduce hard-to-diagnose field failures | Release/build-specific reports, environment cohorts and issue links |
| Open-source project team | Validate compatibility across OS/CPU versions | Test matrix and evidence dashboard |
| Security/privacy reviewer | Ensure no excessive collection | Data schemas, privacy notices, redaction, access audit and deletions |
| External app-store publisher | Release to downstream test channels | Readiness/reporting adapters, not bypassed store review |

## 3. Release-channel specification

Channels are **per application**, scoped to publisher, package identity, signed release and compatible update route. The default is always Stable.

| Channel | Expected maturity | Enrollment | Update policy | Default diagnostic posture |
| --- | --- | --- | --- | --- |
| Stable | Tested for documented support matrix | Public | Normal verified releases | Optional minimum diagnostics |
| Preview / Beta | Feature complete but subject to change | Opt-in; publisher-defined quota/eligibility | Signed prerelease, explicit changelog | Crash/performance reporting separately opt-in |
| Insider / Canary | Rapid engineering feedback, potentially unstable | Explicit opt-in, possibly invite-only | More frequent; rollback capability verified | Extended tracing separately opt-in |
| Internal / Nightly | Team-only development artifacts | Closed, trusted testers | Frequent/expiring | Organization policy, with lawful and explicit notice |

A channel is not a security bypass: every artifact still requires provenance, signature/digest verification, malware/supply-chain checks, license metadata, safe installation plan and tenant isolation. Security scan findings may be acknowledged on experimental builds but may not be hidden.

Publisher controls: audience (public, invite-only, allowlist), capacity, architecture/OS constraints, minimum requirements, target features to test, session expiry, release notes, feedback endpoints, supported diagnostic types, release freeze, revoke and withdrawal. Client controls: enroll, pin version where supported, pause preview updates, leave/return to stable, choose diagnostics, inspect pending report and revoke consent.

Prefer separate preview and stable installations only where the app supports separate identities/data locations. Never promise simultaneous installation when OS package identity makes that impossible.

### Versioning and migration safety

All channel artifacts carry immutable artifact digest, semantic version/build ID, platform/architecture and native package identity. Never silently switch a stable user to preview. Changing channels is a stateful upgrade/downgrade operation, not just a label. A preview build may migrate a database to a schema that cannot be downgraded. Before exit to Stable, Wazi must check minimum compatible data/schema versions, create a validated backup/checkpoint when supported, and either execute a proven migration, offer reinstall with a clear data warning, or block a destructive downgrade. Revoked compromised builds must remain distinguishable from routine experimental failures.

## 4. Consent and user experience

The marketplace app listing offers **Stable** as the default with an optional **Try Preview** or **Join Insider** action. Enrollment should display:
1. Publisher, channel, exact build and what is being tested.
2. Known limitations, instability, potential data loss and backup recommendation.
3. Supported devices/OS versions and estimated resource impact.
4. Separate diagnostic choices with plain-language examples, destination/recipient and retention.
5. Update frequency, exit limitations, and support/bug-report options.
6. Explicit agreement to join and confirmation of preferences.

Do not bundle "send all logs" with the channel-enrollment checkbox. Consent must be purpose-specific, informed, recorded, easy to revoke and not coerced by making optional analytics mandatory. A publisher may define a testing program that requires a narrow essential diagnostic event set, but that must be disclosed before enrollment, justified, and offered only to people who choose that program; do not pretend a checkbox authorizes arbitrary future collection.

**Illustrative user choices — actual interface copy requires UX/legal review:**

- Join Wazi Preview for Example App [OFF by default].
- Send basic installation and crash status to developer [OFF by default for voluntary analytics].
- Send limited performance and feature diagnostics [OFF by default].
- Include detailed diagnostic logs in bug reports [OFF by default; review required].
- Attach screenshots or files [manual selection only].
- Share my contact address for developer follow-up [OFF by default].

Users can review consent status on an app's Settings > Preview & Diagnostics screen. "Leave Preview" must stop future preview upgrades and initiate a supported path back to Stable; "Stop sharing diagnostics" must stop subsequent optional collection immediately; "Manage my data" exposes download/deletion request controls with clear limitations on previously anonymized aggregates and legally required security/audit records.

### Privacy notices and jurisdiction

Treat each Wazi developer's independent telemetry as processing of users' data, with Wazi as controller and/or processor depending on concrete contracts and purposes. Clarify these roles before launch and publish a developer data-processing agreement where needed. Apply applicable privacy laws, including Kenya's Data Protection Act for relevant users/data and other local regimes. Notice must specify data categories, purposes, lawful basis, recipients, retention, transfers and user rights. Use geographically appropriate processing/storage safeguards when transferring data across borders.

The Office of the Data Protection Commissioner explains Kenyan data-subject rights and principles of purpose limitation, minimization, retention and cross-border protections: https://www.odpc.go.ke/rights-of-a-data-subject/ . This specification is a product-control plan, not legal advice.

## 5. Telemetry contract: what is and is not collected

Telemetry has four levels, each requiring capability support from the actual application/package and the applicable consent:

| Category | Examples of useful data | Capture method | Safety constraint |
| --- | --- | --- | --- |
| Installer lifecycle | Download/install attempted, success/failure stage, signed release/build, adapter, coarse architecture | Wazi installer/host agent | Avoid identifying host/network paths |
| Coarse environment | OS family and major/minor version, x64/arm64, coarse RAM tier, supported dependency versions | Wazi preflight, only for permitted scope | No hostname, MAC, raw serial, precise device fingerprint |
| Application health | Exit/crash event, redacted exception/stack, startup failure, health-check duration | Opt-in publisher SDK or approved runtime instrumentation | No arbitrary process scraping or full memory dumps |
| Feature/performance | Feature success/failure counters, sampled latency buckets, resource tiers | Opt-in app instrumentation | Never capture prompts, document contents or keystrokes |

**Not allowed by default:** filesystem traversal, system-wide log collection, browser history, clipboard, unrelated apps, raw IP in reports, exact GPS location, names, contact lists, login tokens, environment variables containing secrets, unredacted HTTP headers/URLs, AI prompts, private document text, or raw core/full-memory dumps. User-selected screenshots and arbitrary diagnostic files are never collected in the background.

Wazi cannot magically see application-internal crashes without an instrumented SDK, approved crash handler, platform-native authorized report, or explicit user-supplied file. Label a metric "not available" rather than infer it from incomplete installer data.

For Wazi-managed server apps, container and health telemetry should be limited to Wazi-owned resources. Never claim whole-host telemetry access or enable host-level collection merely because a tester selected Insider.

### Local preview and sanitization

Before transmission, the client presents a manifest of report sections: app/version, OS/CPU tier, error type, log excerpt, attachments and recipient. Apply client-side structured field allowlists, token/key patterns, filesystem path and email redaction, log bounding, event sampling and rate limits. Masking alone is not foolproof; users can inspect/omit entire attachments. Server-side sanitization is a second line of defense, not permission to collect everything.

Synthetic example of a *proposed* sanitized diagnostic event:

~~~json
{
  "schema": "wazi.telemetry/v1",
  "app_id": "org.example.notes",
  "channel": "preview",
  "release": "1.4.0-beta.2",
  "build_digest": "sha256:REPLACE_WITH_REAL_DIGEST",
  "platform": {"os_family": "windows", "os_version": "11", "arch": "x86_64"},
  "kind": "crash",
  "fingerprint": "hash-of-normalized-error",
  "error_code": "STARTUP_DB_MIGRATION_FAILED",
  "duration_bucket_ms": "1000-5000",
  "consent_receipt_id": "opaque-id",
  "occurred_at": "2026-10-21T09:00:00Z",
  "redaction_version": "v1",
  "attachments": []
}
~~~

No event schema field should become a hidden catch-all for arbitrary logs or unsanitized JSON. The collector must validate schema, event size, release/consent binding, content type, origin and tenant.

## 6. Bug-report workflow

1. Tester opens Report a Problem from the Wazi app page, Wazi Setup or an integrated application's Help menu.
2. Wazi pre-fills release, channel, signed build ID, app ID and *available* coarse compatibility facts.
3. Tester enters issue summary, reproduction steps, expected/actual outcome and impact; chooses optional screenshot and/or explicitly scoped log excerpt.
4. Client locally previews report and highlights likely sensitive values. User can remove any section.
5. Wazi submits an encrypted, access-controlled report; assigns a report ID and correlates with an existing crash fingerprint without automatically revealing the tester's identity.
6. Developer views sanitized details, asks a follow-up question in Wazi, links to a new or existing upstream GitHub Issue/Discussions post, or marks it duplicate/resolved. Reporter opts in separately to cross-service account disclosure.
7. The GitHub issue includes summary, public safe environment data, anonymized counts and link to a Wazi report available only to authorized maintainer roles. Do **not** paste raw logs, screenshots, signed upload URLs or personal data into public GitHub Issues by default.
8. When a fix is proposed, Wazi records advisory/commit, relevant tests, release association and whether the reporting tester received a corrected build.

Feedback categories: Bug, Crash, Installation, Performance, Accessibility, Compatibility, Documentation, Feature suggestion and Security vulnerability. Security vulnerability reports must use private coordinated disclosure; never automatically create a public issue for secrets or exploitable findings.

Bug reports should remain useful offline: queue locally with encryption and bounded retention, request confirmation before upload, and permit deletion.

### Developer dashboard

Present grouped failure signatures, affected release ranges, OS/architecture cohorts, error frequency and trend, crash-free session rate *only if session instrumentation exists*, install/update/rollback success, response/triage times, outstanding accessibility/compatibility problems, and explicit coverage gaps. Aggregate small or sensitive cohorts; provide no raw unique-device leaderboard or deanonymizing export.

Allow maintainer triage, deduplication, label/split, private discussion, export sanitized issue, and public release-note acknowledgement. Mark all AI-generated diagnosis and draft replies as suggestions requiring developer approval. AI must treat logs and issue text as untrusted instructions.

## 7. Backend architecture

~~~text
Wazi marketplace / app page
    |
    +-- Release channel registry -> signed immutable build + native updater adapter
    +-- Preview enrollment -> eligibility + consent receipts + revocation
    +-- Report wizard -> user review + redaction + secure upload
                                   |
                                   v
                       Telemetry ingest API
                       - auth / tenant isolation
                       - schema / size / rate gates
                       - consent & build checks
                       - quarantine / redaction
                                   |
                   +---------------+----------------+
                   |                                |
                   v                                v
          Aggregate events                 Private encrypted attachments
          (Postgres/time series)           (bounded object storage)
                   |                                |
                   +----------------+---------------+
                                    v
                        Maintainer QA dashboard
                                    |
                                    v
                Sanitized issue/PR/discussion bridge to GitHub
~~~

Use existing Wazi control plane, typed context/evidence design and policy gates. The telemetry pipeline is an isolated, lower-trust subsystem: it must never gain the host agent's installation privileges or mutate deployed apps. Treat uploaded logs as attacker-controlled content. Parse without shelling out or executing code, quarantine malicious attachments, validate declared MIME/type and enforce resource budgets. Access to telemetry and attachments is restricted by tenant, app, maintainer role, need-to-know and audit events.

### Minimal data model

- PreviewChannel(app_id, name, audience, eligible_platforms, status, release_policy)
- PreviewRelease(channel_id, release_id, artifact_digest, provenance, signed_status, tested_matrix, expiry)
- Enrollment(user_or_pseudonym, app_id, channel_id, joined_at, revoked_at, compatible_exit_state)
- ConsentReceipt(enrollment_id, purpose_version, data_categories, recipient, signed_timestamp, revoked_at)
- TelemetryEvent(tenant_id, app_id, release_id, coarse_platform, category, normalized_fingerprint, redaction_version, received_at)
- DiagnosticArtifact(event_id, encrypted_object_ref, sensitivity, expiration, uploader_confirmation)
- FeedbackReport(enrollment_id, release_id, safe_text, reproduction_steps, status, visibility)
- IssueLink(feedback_id, external_tracker, url, public_payload_hash)
- ReleasePromotion(from_channel, to_channel, evidence_digest, approver, time)
- DeleteRequest(subject_scope, state, completed_at, exception_reason)

Never use stable telemetry pseudonyms for cross-app tracking. Prefer per-app random identifiers with rotation and server-side aggregation. Avoid device fingerprinting; deduplicate via normalized crash signatures rather than permanent hardware identifiers.

### Retention and governance: design proposal, not a legal safe harbor

- Raw diagnostic attachments: 14 days by default, configurable shorter for sensitive apps.
- Redacted individual telemetry events: 30 days by default.
- Aggregated metrics with appropriate de-identification: 90 days or a documented shorter/longer retention when justified.
- Consent/audit metadata: retention determined by legitimate legal and security requirements, with minimization.
- Access logs and issue-tracker links: separately scheduled; review with counsel.
- Immediate prospective effect of opt-out; past personal data deletion/handling according to applicable rights and narrowly documented exceptions.

Retention values are preliminary product defaults, not statutory requirements; establish final policy only after data-protection review and processor arrangements. Authenticate export/deletion requests and maintain evidence of fulfillment without keeping unnecessary PII.

## 8. Cross-platform preview delivery and distribution bridge

Wazi must distinguish **Wazi-direct packages** from **packages managed by other app stores**. Do not hijack an app's updater or silently replace Apple/Microsoft/Flatpak-managed updates.

| Target | Preview delivery path | Constraint |
| --- | --- | --- |
| Wazi self-hosted Docker/Compose | Pinned prerelease manifest + signed image digest; isolated test host | App data backup and supported migration required |
| Windows direct MSI/MSIX | Wazi channel metadata and verified native package | Upgrade codes/product identities and rollback differ; silent mode needed for some downstream routes |
| Microsoft Store | Publisher-owned Partner Center package flights and known user groups when eligible | Store controls package review and delivery; Wazi cannot guarantee flight availability |
| macOS direct Developer ID | Notarized prerelease app/pkg and authorized update mechanism | Signature/notarization and bundle identity must remain valid |
| Mac App Store | Publisher-owned TestFlight (macOS support) | Apple review/90-day TestFlight build limits and account-specific flow |
| Linux Flatpak | Publisher beta branch/remote, potentially Flathub Beta where eligible | Signed remote and Flatpak branch identity; store rules still apply |
| Homebrew | Publisher-supported prerelease cask/formula or separate tap when appropriate | Homebrew contribution policy; do not mislabel as centrally supported beta service |

Microsoft package flights let publishers target known user groups: https://learn.microsoft.com/en-us/windows/apps/publish/package-flights and https://learn.microsoft.com/en-us/windows/apps/publish/create-known-user-groups . Apple TestFlight supports macOS beta builds, test groups and feedback: https://developer.apple.com/help/app-store-connect/test-a-beta-version/testflight-overview/ . Flathub documents a beta repository: https://docs.flathub.org/docs/for-users/installation . These are examples of platform integrations, not Wazi agreements.

Distribution Bridge should report channel eligibility and whether the developer must perform steps in the external store. Wazi publisher identity, review, privacy policy and telemetry recipients must remain clearly attributed.

## 9. Security, abuse, reliability, and review gates

Threats: malicious preview binaries, compromised publisher signing keys, forged compatibility metrics, spam bug reports, telemetry exfiltration, accidental secret upload, log prompt injection, cross-tenant attachment access, reidentification of rare device cohorts, forced risky downgrades, unavailable telemetry backend, and misleading security badges.

Required controls:
- no arbitrary unreviewed upstream code deployed through Wazi;
- signed, digest-pinned and immutable preview artifacts, release provenance and revocation;
- permissions and risk warnings are real and platform-enforced where claimed;
- no silent privilege escalation or irreversible data migration;
- per-app consent binding and server-side enforcement, independent of client UI state;
- local allowlist/redaction plus server verification, malware scanning, max upload sizes and time bounds;
- encrypted transport/storage, least-privilege maintainer access, audited access to attachments, expiring links;
- no raw logs shared with Meta AI or third-party models without explicit legitimate processing basis, disclosure, filtering and provider controls;
- security reports go through private disclosure by default;
- release rollbacks tested before preview promotion, stable users never enrolled silently;
- Wazi installer must keep working when telemetry infrastructure is offline; drop/queue optional diagnostics safely and never block application function purely to upload analytics;
- review complaint/abuse flow for testers and developers.

Evaluation fixtures should include deliberately injected credentials in logs, malicious text instructing an AI to leak secrets, oversized uploads, spoofed release digest, revoked consent, invalid native package identity, wrong OS/architecture, incompatible downgrade, cross-tenant attachment access, expired signing key, disabled network and unavailable GitHub.

## 10. Acceptance criteria and KPI framework

### User acceptance
1. Stable is preselected; Preview enrollment explicitly identifies publisher, app and channel.
2. Each optional diagnostic category is clearly explained and independently configurable.
3. User sees the exact diagnostic payload before a manually submitted report.
4. With all optional diagnostics disabled, no optional telemetry network requests occur.
5. A channel exit never silently destroys application data or downgrades an incompatible database.
6. The reported build, OS family, CPU architecture and Wazi manifest are validated against install receipts, not asserted by a free-text browser form.

### Developer acceptance
1. Maintainer can create/pause/revoke a prerelease channel for an eligible signed build.
2. Testers can join through a Wazi listing; private channels enforce eligibility.
3. One installed reference app emits a real structured crash or failed-health event through authorized instrumentation.
4. Maintainer dashboard can group at least two matching synthetic/real test incidents without leaking identity.
5. User-approved log excerpt is redacted, encrypted, access-scoped, expired by policy, and omitted from public GitHub issues.
6. Wazi can draft a sanitized GitHub issue and link evidence back to the private report without automatically publishing sensitive data.
7. Stable users' update paths are unaffected when the preview is withdrawn.
8. Consent revocation, deletion request, access controls and API failure behaviors are tested.

### Proposed outcome measures

- Preview-to-Stable release promotion time and number of bugs identified before stable.
- Percentage of new preview builds with verified signatures and completed baseline checks: 100% target.
- Bug report reproducibility rate: measured from maintainer triage, without pretending it is automatic.
- Percent of crash reports tied to an exact build and signed artifact: 100% for Wazi-managed release reporting.
- Opt-out effectiveness: 0 optional collection after revocation, verified by test.
- Accidental secret leakage in test fixtures: 0 accepted uploads/exports containing seeded secrets (defense in depth).
- Crash-free session percentage only for applications with consented, representative session instrumentation.
- Installation/update success by compatible host cohorts; disclose sample size and bias.
- Number of distinct volunteer testers and actioned feedback reports, not simply download counts.

## 11. Prioritized implementation roadmap

| Phase | Scope | Exit gate |
| --- | --- | --- |
| Hackathon/demo | Preview channel listing, version metadata, consent UI, real manually reviewed sanitized bug report; integration with one reference app | Live flow demonstrable with no fabricated telemetry and no false privacy promises |
| 30 days after MVP | Stable/Preview signed release mapping; enrollment; manual feedback forms and GitHub issue bridge; consent receipt schema | Testers can join, submit, leave and retain stable users' updates |
| 60 days | Minimal optional Wazi SDK and collector; installer and managed-host coarse events; privacy dashboard and redaction | Real end-to-end consented event on a reference app |
| 90 days | Event grouping, maintainer dashboard, rate limiting, retention/deletion workflow and compatibility cohorts | Closed private alpha with security/privacy tests |
| Subsequent alpha | Insider/canary, rollback compatibility and staged promotion, native desktop integration where actual adapters ship | Release gates and rollback/restore evidence across a supported matrix |
| Later | TestFlight/Microsoft flights/Flathub Beta integrations and privacy-reviewed advanced diagnostics | Publisher authorization, conformance reports and actual tested external-store flows |

Do not expand hackathon MVP into a full crash-ingestion product. The early pitch can describe the preview program as a product roadmap capability, not deployed production functionality.

### Proposed first issues

- [ ] PREVIEW-001: ADR — channel ownership, stable default and OS update-provider precedence.
- [ ] PREVIEW-002: Typed preview release/channel/enrollment/consent schemas with expiry.
- [ ] PREVIEW-003: Marketplace join/leave UX and granular diagnostic preferences.
- [ ] PREVIEW-004: Secure sanitized manual issue report and private evidence storage.
- [ ] PREVIEW-005: Signed build-to-release-to-install-receipt binding.
- [ ] PREVIEW-006: Redaction test fixtures and forbidden-data denylist checks.
- [ ] PREVIEW-007: GitHub Issues draft bridge with no public raw telemetry.
- [ ] PREVIEW-008: Restricted pilot instrumentation SDK and structured event ingest.
- [ ] PREVIEW-009: Consent revocation, retention, access/export/deletion tests.
- [ ] PREVIEW-010: Promotion/rollback/downgrade evaluation and platform-flight readiness.

## 12. Decisions to resolve

1. Which legal entity controls first-party Wazi usage analytics, and which entity processes the independent publisher's data?
2. Can publishers require a narrow class of diagnostic reports for participation, or should the first product make every category opt-in?
3. Are there app categories (health, children's software, identity/security tools) where diagnostic collection should be further restricted or initially unavailable?
4. Should Wazi store private diagnostic attachments or issue short-lived uploads directly to a publisher's own processor? Default: Wazi access-controlled storage for narrow, revocable reports.
5. Does a preview inherit the stable application's native package identity or have a separate identity when supported? Must be decided per adapter/app.
6. Can developers invite users via email without creating a Wazi account? Consent and account-creation boundaries need a formal UX decision.
7. What are the exact production retention periods, data-transfer jurisdictions and publisher DPAs?
8. Which Meta AI analyses may be run on *sanitized* reports, and with what separate notices and retention policies?

## 13. Primary research references (checked 2026-10-21)

1. Microsoft, "Package flights" — https://learn.microsoft.com/en-us/windows/apps/publish/package-flights
2. Microsoft, "Create customer groups" — https://learn.microsoft.com/en-us/windows/apps/publish/create-known-user-groups
3. Apple, "TestFlight overview" — https://developer.apple.com/help/app-store-connect/test-a-beta-version/testflight-overview/
4. Apple, "Invite external testers" — https://developer.apple.com/help/app-store-connect/test-a-beta-version/invite-external-testers/
5. Apple, "Beta feedback crash submissions" — https://developer.apple.com/documentation/appstoreconnectapi/beta-feedback-crash-submissions
6. Flathub, "Installation / Flathub Beta repository" — https://docs.flathub.org/docs/for-users/installation
7. Kenya Office of the Data Protection Commissioner, "Rights of a Data Subject" — https://www.odpc.go.ke/rights-of-a-data-subject/
8. Wazi current repository design — https://github.com/Bera-Corp/wazi/blob/main/README.md and https://github.com/Bera-Corp/wazi/blob/main/docs/CONTEXT_ENGINEERING_PLAN.md

> **Status reminder:** This file documents a proposed design only. It does not attest that Wazi currently ships an Insider Program, a production telemetry SDK, private crash-processing infrastructure, or external-store partnerships.
