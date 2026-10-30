# Wazi — Meta Hackathon Execution Plan and Missing-Requirements Audit
> **Status:** Proposed and actionable; not proof of implementation. **Date:** 21 October 2026. **Track:** Indie. **Owner:** Bera-Corp / Wazi.  
> **Companion documents:** [Master research](WAZI_MASTER_PLAN.md) · [Installer and store bridge](WAZI_INSTALLER_AND_STORE_BRIDGE.md) · [Preview and Insider](PREVIEW_AND_INSIDER_PROGRAM.md) · [Existing architecture](ARCHITECTURE.md) · [Context engineering](CONTEXT_ENGINEERING_PLAN.md) · [Existing MVP roadmap](MVP_ROADMAP.md) · [Threat model](THREAT_MODEL.md).

## 1. Decision

**Judging rubric, integrated 29 October:** prioritise and accept work against
Impact **30%**, Use of Meta AI Tooling **25%**, Technical Depth & Execution
**20%**, and Demo **25%**, supplied by the project owner. The
[judging pillars and delivery rubric](JUDGING_PILLARS.md) defines feature
evidence, readiness review and submission gates. These weights apply to
product decisions and rehearsals as well as the pitch.

**29 October scope addition:** [Worlds and S-Prints](WAZI_SPRINTS_PLAN.md)
is a long-term feature track. Its experimental offline contracts/resolver
are locally implemented; model execution, GPU deployment and physical
actuation are outside this hackathon acceptance path. The repository ->
security -> remediation -> approved deployment story remains the build priority.

For the hackathon, demonstrate **one reproducible, safe, Meta-model-powered journey**: source repository → governed audit and Wazi package manifest → real vulnerability finding → reviewed AI repair proposal → isolated tests and rescan → signed/pinned deployment plan → one supported Linux Docker Compose target → a public-facing app listing and feedback pathway → optional MCP status inspection.

**Do not promise** arbitrary GitHub repository installation, universal native desktop support, multiple infrastructure providers, fully autonomous remediation, live marketplace revenue, a production crash SDK, or external app-store partnerships in this competition build. These belong in the master-plan phases.

The master research and install-bridge reports are design documents, not a guarantee these features already ship. The original 21 October inspection was documentation-only; by 29 October, Phase 0 contracts and the offline Worlds prototype were shipped with green CI at `3ae8345`. The live product journey and Meta integration remain buildout work.

## 2. Hackathon framing and rules

**Positioning:** "Wazi takes promising open-source repositories from code to trustworthy applications—with policy-gated Meta AI for packaging, maintenance and release."

**Tagline:** "From repository to trusted application."

**Evaluation focus:** Working functionality first; demonstrate why Meta AI is materially necessary instead of attaching a superficial chatbot. Capture reproducible test results, screenshots, a concise video, and clear safety boundaries. Verify the **final official contest rules**, Indie/solo eligibility, model/API access and quotas, jurisdiction eligibility, allowable pre-existing code, team approvals, project originality, submission format, intellectual-property terms, prize stacking, and exact cutoff time before submission. As of this document's date, some details circulated online are unverified. Do not quote unofficial prize tiers as confirmed.

**Pre-existing reusable work:** Fleet Control Plane v3.0.0-beta.1 includes repository doctor, bootstrap, test-first gates, branch migrator, Blueprints and context/agent governance. It is a separately governed repository; reuse its pinned public interfaces and document which work predates the hackathon. A team member must have their own competition approval if required. Avoid accidental competition-rule violations through copying or presenting prior code as newly built.

Official competition entry/rules: https://cerebralvalley.ai/e/meta-ai-developer-hackathon/waiver and https://dev.meta.ai/events/global-hackathon ; model docs: https://dev.meta.ai/docs/models .

## 3. Success definition and live 110–130 second demo

**Current film brief:** the owner's later three-minute master is developed
in [video/](../video/README.md), starting with the gladiator/Meta AI opening.
Its transcript supersedes the shorter timing below for this creative edit;
check the final submission-length rules before picture lock.

Use the [four-pillar demo beats](JUDGING_PILLARS.md#demo-beats)
as the updated narrative: beneficiary/problem -> visible Meta proposal ->
verified repair and approval -> working application -> measured benefit.
The timing table below is an implementation breakdown; it must serve that
story. The final contest's video limits take precedence over these targets.

| Timestamp | Live evidence shown | Fallback if external services are unavailable |
|---|---|---|
| 0–12 s | Choose a deliberately flawed, permissively licensed sample application; explain deployment friction | Known tested fixture; no fabricated success |
| 12–27 s | Import GitHub URL and view Repo Doctor findings; propose Solo profile without overwriting existing controls | Offline cached immutable fixture annotated as replay |
| 27–43 s | Generate/validate typed Wazi app manifest, app identity, version, permissions and license | Fixture generated from pinned commit |
| 43–58 s | Real known vulnerable dependency/advisory and OSV evidence, severity, affected version | Stored scan output with scan date |
| 58–80 s | Meta Muse or **actually accessible eligible Meta API model** explains smallest remediation; generates draft patch | A precomputed model output clearly identified as recorded |
| 80–96 s | Run reproducible test, fresh dependency rescan; human approves promotion | CI receipt with actual SHA/command/results |
| 96–113 s | Install signed/pinned Compose package onto disposable private host and show health/status | Recorded execution with timestamps and checked artifacts |
| 113–130 s | Marketplace listing with verified metadata; consented review/mock user input visibly labeled; MCP read-only inspection | Clickable pre-seeded genuine application record |

**Never silently replay fake security or deployment metrics as live.** If a step is simulated, label it in UI and video. A true end-to-end green path with one reference app is more persuasive than shallow demonstrations of five platforms.

## 4. Scope control: build vs defer

**Nodes addition, 30 October:** [Wazi Nodes](WAZI_NODES_PLAN.md) makes owned
hardware the first deployment destination in the existing story. The Linux
dev host is SSH/sudo-ready; node pairing, resource limits and the app runtime
are still planned. Target one node/app with persistent data and reconnect
evidence. Broader Fabric scheduling, Kubernetes and compute resale stay later.

| Capability | Hackathon | Longer-term |
|---|---|---|
| Wazi identity, searchable sample app listing, install/readiness status | **Must** | Catalog search, metadata quality, categories |
| GitHub repo import, webhook or manually triggered reconciliation | **Must** | Multi-source registry, rate-aware webhook ingestion |
| Fleet read-only audit and configurable Solo bootstrap proposal | **Must**, pinned interface | Team/Enterprise profiles, safe migration |
| Typed release/manifest schema & checks | **Must** | Standardization and OS adapters |
| SBOM + OSV findings, hard-coded evidence provenance | **Must** | EPSS, KEV, OpenSSF, independent security ratings |
| Meta model-assisted explanation and patch PR draft | **Must** where model accessible | Maintenance agents and controlled subscriptions |
| Human approval and tests/rescan | **Must** | Full compliance/attestation workflows |
| Single Docker Compose deployment to disposable Linux VM | **Must** | Remote agent, lifecycle and backup/restore across hosts |
| Minimal MCP inspect/status tool | **Should** after working path | Full guarded search/plan/approval/deployment tool set |
| Ratings/reviews/comment sample & maintainer reply | **Should** | Anti-fraud verification, moderation, adoption signals |
| AI-assisted listing/launch copy | **Should**, good second model value | Full Wazi Launch Studio |
| Preview enrollment + consent UX + manual sanitized bug report | **Could** | Native telemetry SDK, canary staging, cross-OS crash pipeline |
| Graphical Wazi Setup on Windows/macOS/Linux | **Defer** | InstallKit, native package adapters, quiet installer |
| Microsoft Store / App Store / Homebrew / Flathub submissions | **Defer** | Wazi Distribution Bridge |
| BYO-VPS production multi-tenant and dedicated VM plans | **Defer** | Monetized Wazi Cloud |
| Billing, one-time premium purchase, donations | **Defer** | Payments and compliance |
| Full Wazi automated maintenance / self-improving agents | **Defer** | Reviewed Fleet/MissionKit/Meciria integration |

## 5. Architecture and physical repository plan

Maintain clear product boundaries; use a monorepo for the Wazi app and independently versioned Fleet integration. Do **not** copy Fleet, MissionKit, or Meciria entire source trees into Wazi.

~~~text
wazi/
  README.md
  docs/
    WAZI_MASTER_PLAN.md
    WAZI_INSTALLER_AND_STORE_BRIDGE.md
    PREVIEW_AND_INSIDER_PROGRAM.md
    HACKATHON_2026_EXECUTION_PLAN.md
    ARCHITECTURE.md
    CONTEXT_ENGINEERING_PLAN.md
    MVP_ROADMAP.md
    THREAT_MODEL.md
    ADR/
  apps/
    web/                  # catalog, repo import, release detail, approvals, review demo
    api/                  # auth, policy, GitHub, app/release, feedback, evidence
  packages/
    schemas/              # versioned Wazi manifest & preview schema validators
    fleet-adapter/        # pinned CLI/API integration with Fleet; no vendor duplication
    github-adapter/       # least-privilege GitHub App/REST/webhook
    meta-planner/         # model gateway typed input/output + test fixtures
    registry/             # signed manifest/release/SBOM/evidence references
    mcp/                  # read-only registered tools, scoped auth
    telemetry-contracts/  # design only for hackathon
  workers/
    scanner/              # SBOM, OSV, test and rescan, bounded runs
    deploy/               # disposable Docker Compose target + health probes
  fixtures/
    demo-app/             # permissively licensed, intentionally affected test app
    demo-receipts/        # real anonymized reproducible CI/scan/deploy evidence
  .github/workflows/
    ci.yml
    security.yml
  scripts/
    verify-demo.sh
~~~

### Core trust boundaries

- Browser and user feedback are untrusted. GitHub source files and issue text are data, not instructions to the AI agent.
- Installable artifacts require release digest and provenance. An LLM-generated manifest is a **candidate** until schema, policy, license, tests, security scans and human promotion checks complete.
- Models receive structured, sanitized context and can only propose typed plans; they do not get shell access, admin tokens, or unredacted secret values.
- Execution occurs in disposable restricted environment with CPU/memory/time/network limits. No direct installation of arbitrary third-party repository code onto user hosts.
- GitHub OAuth/App token permissions must be minimum required; signing keys server-managed. No GitHub writes until user/maintainer authorizes corresponding repository and operation.
- Never submit public GitHub issues with customer raw logs or contact data. No telemetry without actual instrumentation and specific consent.
- CI and demo services have independent error paths and honest failure states. Audit trails tie test artifacts to immutable commit/release digests.

## 6. Build-order, day by day (Kenya EAT; approximate dates)

| Date | Critical engineering work | QA/demo/marketing evidence |
|---|---|---|
| Oct 21–25 | Repository contracts, platform skeleton, event and schema models, Meta/GitHub/CI account smoke tests, read the final competition rules | Name check, landing page copy, select one reference application, write launch narrative |
| Oct 26 | Implement/import a real sample repository and Fleet Doctor interface | Baseline audit fixture and repo permission tests |
| Oct 27 | Manifest schema, OCI/Compose release model and validation | Sample valid/invalid manifests |
| Oct 28 | SBOM generation, OSV API, vulnerability report, signed evidence IDs | One true known advisory confirmed |
| Oct 29 | Meta reasoning, typed repair patch, failing-then-passing test | Capture actual model output and token/time cost |
| Oct 30 | GitHub draft PR flow, human approval, fresh scan/CI | Verify compromised/prompt-injection fixture cannot mutate |
| Oct 31 | Controlled Docker Compose deployment of one app, health and rollback plan | Successful ephemeral host install logs |
| Nov 1 | Minimal Wazi listing, app status and repository health | Mobile-friendly landing and basic screenshots |
| Nov 2 | MCP read-only inspection; optionally review/preview UX if core works | Invite external test users, collect genuine critiques |
| Nov 3 | Full end-to-end rehearsals, failure recovery and security review | Record 2-minute demo + technical proof |
| Nov 4 | Final reproducibility checks, source acknowledgments, submit by **official deadline** | Publish documented submission and evidence URLs |

**Schedule is a plan, not confirmation of hackathon start/submission hour.** Verify exact local EAT cutoff from official rules and submit ahead of it. Stop adding new features 24–36 hours before deadline. Reserve last day for demonstration reliability.

### Hard stop / descope rules

- If Meta API access is not verified by first 48 hours: immediately select officially permitted accessible Meta model before building model-specific coupling; avoid promising an unavailable Muse SKU.
- If hosted provisioner not reliable by day 4: use a disposable Linux VM with pinned Compose and explicit controlled credentials.
- If GitHub App install flow delayed: support explicit URL/manual repo import read-only; GitHub PR draft behind owner-controlled OAuth/token, no elevated fallback.
- If a patch cannot be proven safe by tests and rescan: demonstrate a proposed patch with failure reason, not a claimed fix.
- If full rating system not ready: single comment/review backed by database and moderation flag, no fake verified-install badge.
- If installer work threatens core demo: leave Windows/macOS and store bridge as roadmap screenshots only, no misleading download links.
- If solo developer throughput is limited: prioritize Repo → manifest → scan → Meta patch → verified deploy. Eliminate everything else.

## 7. Missing requirements audit from broader Wazi discussions

This matrix explicitly checks earlier promises often lost when focusing on a showcase.

| Previously discussed element | Present in existing Wazi docs? | Placement | Required decision |
|---|---|---|---|
| Repository standard/agent-readable boilerplate, CI, branching | Not central in original Wazi README; Fleet exists | Hackathon Fleet interface, standard long-term | Avoid enforcing enterprise five-lane topology for solo users |
| Open-source core / marketplace business separation | Licensing undecided | Public architecture, later legal work | Select license, trademark and contribution policy before public release |
| Search by outcome/alternatives to commercial SaaS | Existing demand research | Basic demo plus product roadmap | No invented migration compatibility |
| Security grades, vulnerability alerts, OSS provenance | Existing security scope | Real scanner, evidence UI now | No blanket “safe” label from a single scan |
| GitHub stars vs actual app adoption metrics | Original research partially covers install outcomes | Later product metrics | Distinguish downloads/installs from active users |
| Consumer ratings, comments and developer replies | New product pillar | Minimal optional demo, full later | Anti-fraud, safety reporting and moderation |
| Marketing/launch support | New product pillar | Optional AI listing; full Launch Studio later | No fake testimonials, generated screenshots, or paid-review manipulation |
| AI maintenance and upstream issue/PR integration | Original constrained AI policy; expanded roadmap | Only reviewed one-PR demo | Never auto-merge or falsely report deployment |
| Third-party model APIs/AI usage billed through Wazi | Longer-term vision | Defer | Official API entitlements, model availability, privacy/markup economics |
| Per-user isolated private VM / BYOC/provider integration | Original BYO-VPS, newer cloud concept | Defer except disposable test VM | VM isolation and back-up economics |
| One-time desktop premium and donation choice | Expansion beyond initial wedge | Business model only | App payment support/legal/accounting |
| Cross-platform installer with Standard/Custom/Complete and registration | Separate installer study | Post-demo InstallKit | Windows registration, macOS bundle, Linux desktop/package manager |
| Microsoft/Apple/Homebrew/Flathub distribution bridge | New longer-term distribution vision | Phase later | Publisher controls submissions; no presumed partnership |
| Preview/Insider release tracks | Added PR spec | Optional opt-in channel UX | Separate telemetry consent and safe exit paths |
| Automatic crash/error reporting & attachments | Added PR spec | Manual sanitized report demo only | Instrumentation, retention, DPAs and anti-leak controls |
| Neutral specification + Wazi hosted commercial marketplace | Described in master research | Establish governance now | Avoid self-declared “standard” status before external adoption |
| Fleet/MissionKit/Meciria integrations | Existing Fleet, separate components | Fleet only initially | External versioned interfaces, no cross-DB writes |
| MCP tools for agent discovery/scan/deploy | Expanded vision | Read-only inspect/status MVP | Auth scopes and action approvals |
| Platform-dependent permission declarations | Installer/manifest concepts | Manifest metadata now | Honest “declared” versus OS-enforced status |
| GitHub outage resilience | Discussion, not valid direct GitHub outage fix | Webhooks/cache later | Do not claim abandoned repositories cause outages |
| Backup/restore/rollback validation | Strong original Wazi requirements | Simple rollback evidence now | Avoid unsafe production claims without tested restore |
| Revocation and supply-chain proofs | Original Wazi requirement | Baseline signed immutable artifact | Protect keys and revoke compromised builds |
| Privacy, consent and data deletion | Context plan + new Preview doc | No real telemetry MVP | Do not send full logs to AI |
| Accessibility, localization, offline behavior | Not thoroughly detailed in original documents | Basic keyboard/contrast now | Test keyboard/screen-reader, unknown translations labeled roadmap |
| Support / GitHub Issues / Discussions bridges | Mentioned in original upstream feedback | Issue-link demo, later full | GitHub Pages is web hosting, not issue tracking |

### Hidden hackathon-specific blockers to close

- **Eligibility/reused code**: track pre-event Fleet/Wazi source and new contributions; acknowledge collaborators.
- **Meta API**: validate account access, data locality, call quota, tool support and cost cap before pitch.
- **Budget controls**: maximum calls and spend; do not run uncapped background model or CI work.
- **Credential handling**: no PAT in frontend, no credentials in screenshots; scope Github App installation, protect secrets and use disposable demo credentials.
- **Supply chain**: pin third-party Action SHAs and container digests; generate SBOM and preserve license attribution.
- **Installer safety**: no OS-level privilege bypass; do not install real unreviewed repos simply for a demo.
- **Sandbox limits**: isolated job queue, no privileged container socket for untrusted builds, ephemeral runners and strict network egress.
- **Demo determinism**: controlled reference repo with exact commit, advisory ID, model output, expected tests, VM snapshot and rollback/cleanup script.
- **Claims**: publish what is truly implemented versus planned. Explain that initial signatures/attestations are not a universal guarantee of safety.
- **Developer rights**: don't mirror redistributable packages without checking individual license, attribution, trademark and hosting terms.
- **Review integrity**: no fake reviews/downloads/verified user indicators or ungrounded performance figures.
- **Reliability**: app can communicate temporary GitHub/Meta outages; cached records are timestamped, not mislabeled current.
- **Data protection**: avoid collecting machine-identifying telemetry without purpose and consent; seed test logs with secrets for redaction tests.
- **Public access**: if product cannot be safely trialed by reviewers, include reproducible run instructions and clear video evidence.
- **UX**: accessible contrast, keyboard navigation, feedback for denied permissions and explicit irreversible-action confirmation.

## 8. Objective acceptance gates

**Judging:** complete the [four-pillar submission gate](JUDGING_PILLARS.md#submission-gate).
No feature is competition-complete solely because its tests pass: record
beneficiary, pillar contribution, evidence and demo moment. Missing real
Meta use or a working demo cannot be compensated by extra schemas or slides.

**Engineering:** One real OSV advisory; one correctly bounded model-generated patch proposal; one failing-before/passing-after relevant test and fresh scan if claiming a fixed vulnerability; one actual reviewed deployment to isolated Linux; one health status check with evidence; one reproducible clean re-run; no raw secrets/privileged shell surfaced to model.

**Security:** Permission deny tests, external prompt injection test, invalid and revoked manifest checks, safe failure on stale host context, cleanup after aborted deployment. Simulated fixture-based checks must be labeled. Auth rules for every mutating endpoint.

**UX:** A judge unfamiliar with GitHub can follow the app page and outcome in under two minutes; provenance, scan time, model output status, approval boundary, and supported deployment target shown clearly.

**Marketing:** Real 100–130 sec video and a separate technical README; 3–10 genuine pilot testers desirable, not a condition of correctness. A dedicated landing page, 1 concise public demo clip, 1 architecture explainer, and 1 real developer quote only with consent. Focus marketing on technical proof, not manufactured metrics.

**Release:** Signed build/release hashes, successful smoke-test receipt, deployment teardown and reproducibility guide. Public documentation defines current feature support, known limitations, license status and security reporting route.

## 9. Recommended issues in priority order

Every issue carries the pillar/evidence fields in
[JUDGING_PILLARS.md](JUDGING_PILLARS.md#build-sequence-and-acceptance).
HACK-001/003 establish Impact and challenge fit; HACK-007/008 prove Meta value;
HACK-002/006/009/010 establish engineering and the working outcome; HACK-011/015
make the demo usable and compelling. Dependencies and mandatory safety
checks still precede optional features.

1. **HACK-001** Confirm final challenge rules, Meta API/model access, Indie team and existing-code requirements; record evidence.
2. **HACK-002** Wazi schemas: app identity, package manifest, permissions, security status, deployment plan and artifact receipt.
3. **HACK-003** Reference repo and reproducible test fixture with safe intentionally vulnerable dependency.
4. **HACK-004** Fleet Repo Doctor adapter pinned to versioned interface, Solo profile read-only proposal.
5. **HACK-005** GitHub URL importer with minimal required permissions and caching.
6. **HACK-006** OSV integration, SBOM capture, source/advisory traceability and scan freshness display.
7. **HACK-007** Meta model adapter with typed output and patch proposal; enforced no privileged execution.
8. **HACK-008** Automated relevant test and rescan with signed evidence receipts.
9. **HACK-009** Human approval and GitHub draft PR mechanism (owner authorized only).
10. **HACK-010** Disposable Linux Compose runtime, signed artifact verification, health and safe rollback demo.
11. **HACK-011** Marketplace listing/release readiness view, one real installation event and honest review widget.
12. **HACK-012** MCP scoped read-only inspect/status, issuer/tenant checks and denied mutation tests.
13. **HACK-013** Optional Wazi Preview enrollment/consent UI with one manual, sanitized feedback report.
14. **HACK-014** Launch Studio minimal Meta-generated, human-approved listing copy.
15. **HACK-015** Demo recorder, deterministic smoke script, architecture readme, privacy and limitation notes.
16. **HACK-016** Public pitch, launch website, organic community feedback, submission checklist and video.

**Sequence:** rules/model access → fixture/schema → import/Fleet → OSV → Meta patch/test → approval/deploy → listing/demo → MCP/reviews/preview if time. Keep no more than one repository under active agent mutation at a time; isolated execution needs explicit policy controls.

## 10. Operational ownership

**Solo:** founder owns API/Fleet/Meta/scanner/deploy and acceptance criteria; reserve significant dedicated time for tests and video. If a second approved participant joins, assign that person landing page, UX, accessibility, community, clip production and tester interviews, not duplicate agentic backend work. Separate public roadmap delivery from competition-submission priorities.

**PR governance:** This document and imported research reports are decisions/proposals. Finalize ADRs before implementing privileged operations or changing public licensing. For the user's currently private repository, do not make public/open-source claims until license selection and repo exposure are deliberate.

## 11. Source ledger

- Original Wazi technical baseline: [README](../README.md), [Architecture](ARCHITECTURE.md), [Context](CONTEXT_ENGINEERING_PLAN.md), [Roadmap](MVP_ROADMAP.md), [Threat Model](THREAT_MODEL.md), [License Decision](../LICENSE-DECISION.md).
- [Fleet Bootstrap distribution and versioning](https://github.com/Bera-Corp/Fleet-control-plane/blob/main/docs/DISTRIBUTION_MODEL.md).
- [Fleet standalone repository capabilities](https://github.com/Bera-Corp/Fleet-control-plane/blob/main/docs/products/README.md).
- [AGENTS.md](https://agents.md/) and [MCP specification](https://modelcontextprotocol.io/specification).
- [OSV advisory data](https://osv.dev/), [OpenSSF Scorecard](https://securityscorecards.dev/), [Sigstore](https://www.sigstore.dev/).
- [Meta hackathon event](https://dev.meta.ai/events/global-hackathon) and [Meta AI Model API documentation](https://dev.meta.ai/docs/).
- [GitHub Webhooks and Apps](https://docs.github.com/en/webhooks) and [GitHub API rate limits](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api).

> **Verification caveat:** Dates, award requirements, model names, quotas and permissions can change; this is an execution proposal based on material available by 21 October 2026 and must be reconciled against official rules before release.
