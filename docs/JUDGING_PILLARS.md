# Wazi judging pillars and delivery rubric

Source: judging criteria supplied by the project owner on 29 October 2026.
These are the working competition weights. Exact official problem-statement
wording and final submission constraints still need to be linked by HACK-001;
this document does not claim an independent verification of the contest rules.

Every hackathon feature, build decision, acceptance review and demo rehearsal
must explain its contribution to these four pillars. Engineering safeguards
remain mandatory even when their judging weight is lower.

| Pillar | Weight | Judge's question | Evidence Wazi must deliver |
|---|---:|---|---|
| Impact | 30% | Who benefits, how much does it matter, could people use it, and which problem statement does it address? | One named user segment and concrete task; exact challenge mapping; observable before/after deployment friction; a usable path from repository to healthy application; genuine feedback where available |
| Use of Meta AI Tooling | 25% | Is Meta tooling used creatively beyond a basic integration, with a distinctive capability? | A real eligible Meta model turns bounded repository/advisory evidence into a typed, cited explanation and minimal repair proposal; relevant tests/rescan establish its value; the demo makes the model's contribution visible |
| Technical Depth & Execution | 20% | Is the engineering sound, refined and thoughtfully crafted? | Contracts, policy denial, cryptographic verification, pinned artifacts, approval binding, isolated execution, health and rollback evidence; reproducible CI and clean reruns; clear error states |
| Demo | 25% | Does the product work live, feel impressive and make a compelling video? | One coherent working journey with clear before/after stakes, visible Meta reasoning/repair, explicit approval and a healthy deployed app; concise video, rehearsed timing and honest failure/replay handling |

## Product and scope decisions

The competition story is: an open-source maintainer or small team has a
promising repository but faces deployment and maintenance friction. Wazi
uses evidence and bounded Meta AI assistance to produce a reviewed repair
and trustworthy release, then deploys one supported application.

- **Impact:** select a real beneficiary and one painful task. Measure
  elapsed time, manual interventions and successful completion on the same
  pinned task before/after. State sample size and conditions; a controlled
  example is not evidence of broad market adoption or production savings.
- **Meta AI:** one strong, visible model-assisted transformation is required.
  A generic chat panel or generated listing copy alone does not satisfy
  this pillar. Model failure must remain visible and must not bypass policy.
- **Engineering:** reuse the Phase 0 contracts and evidence pipeline. Build
  cryptographic verification and the deterministic host slice before giving
  a model any path to a mutating operation. The model proposes candidates;
  tests, policy and approvals decide whether execution may proceed.
- **Demo:** design the user-facing journey while building the backend.
  Show readiness, proposed changes, approval, progress, health and failure
  states. A green CI screenshot alone is not a product demonstration.

Worlds/S-Prints remain an experimental extension. Their offline resolver can
support a brief technical-depth explanation, but it does not prove Meta
tooling use, learned-model gains, adoption or a working world runtime. Do
not displace the core journey with this extension unless it demonstrably
improves the weighted submission and passes its own execution gates.

## Build sequence and acceptance

| Work item | Primary pillars | Acceptance evidence |
|---|---|---|
| Beneficiary, problem statement and baseline | Impact, Demo | A named user segment; exact official statement and source; one reproducible task and recorded baseline; assumptions labelled |
| Phase 1 verification and host slice | Technical Depth & Execution, Demo, Impact | Valid signed artifact executes on a disposable supported host; altered/revoked input is denied; health and rollback evidence tied to exact digests |
| Meta proposal path | Use of Meta AI Tooling, Technical Depth & Execution | Real model call using an accessible eligible model; redacted input provenance; typed cited output; bounded calls/cost; relevant failing-before/passing-after test and fresh rescan if claiming a repair |
| App review and approval UX | Demo, Impact, Technical Depth & Execution | A judge can inspect proposed changes and evidence, approve explicitly, see progress and reach the running app; failure is understandable and recoverable |
| Rehearsal and submission | All four | Timed end-to-end run, short video and reproducibility instructions; every material claim has an inspectable receipt; fallback recordings are labelled |

The deterministic Phase 1 gate remains without AI. The competition's Meta
proposal path is a separate bounded workstream, integrated only after the
deterministic boundary works. Planning it now does not mark later runtime
or AI-packaging phases implemented.

Each hackathon backlog item must record:

1. Primary pillar(s) and expected contribution; respect the supplied weights.
2. User-visible outcome and beneficiary.
3. A concrete acceptance check and evidence location.
4. Current state: planned, implemented, locally verified, CI verified or
   demonstrated; do not collapse these into one completion claim.
5. The demo moment it enables, dependencies and fallback/failure behaviour.

## Weighted readiness review

At a build review or rehearsal, assess every pillar from 0 to 5:

| Score | Evidence level |
|---:|---|
| 0 | No demonstrated evidence |
| 1 | Defined in planning |
| 2 | Partial implementation or isolated example |
| 3 | Reproducible controlled result with inspectable evidence |
| 4 | Repeated end-to-end result with relevant failure handling |
| 5 | Submission-ready proof, usable journey and polished presentation |

Internal readiness index (0–100):

`100 * (0.30 * impact/5 + 0.25 * meta/5 + 0.20 * engineering/5 + 0.25 * demo/5)`

This is an internal prioritisation aid, not a predicted judging score. Attach
evidence and remaining gaps to every score. Do not average away a missing
working demo, absent real Meta usage, missing challenge fit or unsafe execution.
When scope competes for time, prioritise the largest evidenced contribution
to the weighted pillars after mandatory dependencies and safety gates.

Current evidence boundary: Phase 0 contracts and the offline Worlds prototype
are shipped with green CI at `3ae8345` (verified in the preceding project
review). That supports part of engineering; it does not establish measured
impact, actual Meta inference, a live deployment or a submission-ready demo.
No aggregate readiness score is assigned without those receipts.

## Demo beats

Target: 110–130 seconds, subject to final contest limits.

**Current creative brief:** the owner subsequently requested a three-minute
master with an animated gladiator opening. [The video workspace](../video/README.md)
and [transcript v01](../video/transcript-v01.md) govern that film's timing.
The shorter beats below remain an earlier planning outline; final submission
duration must still be checked against the contest rules.

| Time | Product moment | Pillar evidence |
|---|---|---|
| 0–15 s | Name the beneficiary and show the concrete repository problem | Impact: task and stakes, challenge fit |
| 15–35 s | Show readiness and a real advisory/blocked release with provenance | Impact and engineering: the actual obstacle |
| 35–65 s | Show the real Meta-generated minimal proposal and cited rationale | Meta: a distinctive, useful transformation rather than chat |
| 65–85 s | Relevant test and rescan evidence; human reviews and approves | Engineering: measured change and controlled authority |
| 85–115 s | Execute the approved deployment and open the healthy application | Demo and impact: a working outcome |
| 115–130 s | Show before/after evidence and one concise trust/rollback view | All four: benefits, craft and reproducibility |

Record elapsed installation/test time honestly. Long-running work may be
time-compressed or shown as a labelled replay; never imply an entire deploy
or model call completed within an edited screen transition. Recordings,
cached outputs and mock data must be distinguishable from live operations.

## Submission gate

- Impact: exact problem-statement mapping, beneficiary, measured task outcome
  and limitations are explicit. Genuine feedback requires consent.
- Meta: an eligible, actually accessible model is materially involved, with
  model ID, integration role, redacted evidence and output status recorded.
- Engineering: relevant CI, authenticity/denial checks, isolated execution,
  health and rollback receipts are available for the submitted revision.
- Demo: working end-to-end journey, judge-accessible run instructions,
  watchable video and an honest service-outage fallback are ready.

Review all four together before adding a stretch feature or declaring the
hackathon build complete. See [the execution plan](HACKATHON_2026_EXECUTION_PLAN.md)
and [the roadmap](MVP_ROADMAP.md).
