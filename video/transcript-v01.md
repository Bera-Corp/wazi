# Wazi film transcript — v01

Working title: **Small gladiator. Serious power.**

Format: 16:9 master; planned edit 03:00. Warm, cinematic and playful opening;
clear, confident product explanation. The gladiator is a petite adult female
AI character with large expressive eyes and oversized, practical armour.
The opening is an animated metaphor. It does not portray implemented robot
control or a working Wazi Worlds runtime.

This first draft describes the product experience we are building. During
unimplemented product scenes, keep a readable `Concept preview` label. The
Phase 0 evidence scene uses actual software captures. See
[production notes](production-notes.md) before turning future-tense lines
into present-tense demo claims.

## 01 · 00:00–00:07 · The entrance

**Picture / motion:** Ground-level close-up: a huge mechanical warhammer
scrapes a furrow through arena sand. Pull back to reveal its tiny determined
owner dragging it with both hands. Her helmet slips; she nudges it back up.
Other gladiators are warming up in the distance. One hears the scrape.

**Gladiator in the background:**
> She brought that?

**On-screen text:** None. Let the visual joke land.

**Sound:** Heavy scrape, small footsteps, loose armour clink. Music begins
with an inquisitive plucked motif. No narration over the reveal.

## 02 · 00:07–00:15 · The power source

**Picture / motion:** She plants her feet and exhales. A blue energy core
on her wrist lights up, visibly labelled `Meta AI`. A thin light trail
connects the core, her temple and the hammer. Her eyes focus; she raises
one finger. Sand falls away as the weapon rises cleanly into the air.
Keep her expression calm and a little cheeky.

**Heroine:**
> Let's make this easy.

**On-screen text:** `Meta AI` on the energy core, readable during activation.

**Sound:** Scraping stops. A bright charging tone blooms into a deep hum;
the score gains confidence. Give the lift a beat of silence before the reveal.

## 03 · 00:15–00:22 · The reaction and match cut

**Picture / motion:** The hammer floats into position beside her. Three
gladiators look over their shoulders and stop mid-motion; one slowly lowers
his wooden practice sword. No strike, explosion or combat. She gives a tiny
smile. Push into the hammer's glowing core; its ring becomes a Wazi loading
ring on an application screen. The palette and sound bridge the cut.

**Narrator:**
> A small team. A huge idea. The right power behind it.

**On-screen text:** `Wazi` appears as we enter the product world.

**Sound:** One comic armour clink, then a clean musical transition. Avoid
an abrupt trailer-to-presentation change.

## 04 · 00:22–00:38 · The real problem

**Picture / motion:** The loading ring opens around a repository card.
Next to it, an illustrative small-team workspace: one builder, several jobs.
Installation instructions, dependency warnings, server setup and backup tasks
stack up. Show the time and attention burden without fabricated statistics.
Move toward a single clear task: get this application safely running.

**Narrator:**
> For small teams, a promising open-source app can become another job: setting up servers, fixing dependencies, managing secrets, and keeping everything running. We want the benefits of open software without needing an entire operations team.

**On-screen text:** `A useful app. A lot to manage.` Then
`Install · Secure · Maintain · Recover`.

**Sound:** Brief overlapping notification sounds resolve into the main rhythm.

## 05 · 00:38–00:53 · What Wazi is

**Picture / motion:** Tasks gather into one Wazi application card. It shows
purpose, requirements, permissions and release evidence. A repository on the
left connects to a reviewed application on the right. Use a simple picture,
not an architecture chart.

**Narrator:**
> We're building Wazi: a trusted path from repository to running application. Discover useful software, understand what it needs, review the evidence, and deploy it on infrastructure you control.

**On-screen text:** `From repository to trusted application.` A small
`Concept preview` label begins and persists through proposed product screens.

**Sound:** The same blue activation tone returns, now restrained and precise.

## 06 · 00:53–01:10 · One understandable journey

**Picture / motion:** Follow one application, keeping its name and icon
consistent. Import its repository. Host checks reveal requirements and
compatibility. A package recipe shows images, storage and permissions in
plain language. The flow pauses on a specific dependency problem rather
than displaying an unexplained red security score.

**Narrator:**
> Start with one app and your own server. Wazi will check compatibility, explain permissions, and turn reviewed deployment instructions into a repeatable package. When a dependency problem appears, you can see the finding and where it came from.

**On-screen text:** `Repository → Check → Package`. Then
`Finding · Affected version · Source`.

**Sound:** Three light confirmation beats; the finding gets a gentle stop cue.

## 07 · 01:10–01:36 · Meta AI does meaningful work

**Picture / motion:** The dependency finding expands into a bounded evidence
packet. Blue light from the opening core becomes a restrained connection to
a panel labelled `Meta AI`. Reveal a short proposed change, source references
and a plain-language explanation. Show a relevant test before and after,
plus the new scan, only when genuine evidence is available. Avoid scrolling
pages of chat or displaying private model chain-of-thought.

**Narrator:**
> Meta AI is the reasoning layer we're building into that workflow. It will use the repository and security evidence to propose a focused repair, explain the change, and point back to its sources. Tests and a fresh scan will check the proposal. You review what changed before anything is approved.

**On-screen text:** `Meta AI proposes` → `Tests verify` → `You approve`.
One selected diff and one clear test result, not a wall of code.

**Sound:** Blue pulse follows the evidence-to-proposal path. Leave a short
breath after “You approve.” No fake instant response or hidden loading cut.

## 08 · 01:36–01:57 · Power with control

**Picture / motion:** The proposed release moves through three visible gates:
identity/verification, permissions, approval. The intended deployment starts
on a disposable host. A tampered package example takes a separate red path
marked `Blocked`; it never reaches the host. Then return to the approved
path and the app's health view.

**Narrator:**
> That power needs clear boundaries. Wazi's deployment design uses verified packages, explicit permissions, and approval tied to the exact plan. Secret values stay outside the AI context. The goal is a healthy application, with evidence of what ran and a clear recovery path.

**On-screen text:** `Verify · Review · Approve · Run`.
`Verified` and `Healthy` require real captures before final submission.

**Sound:** A brief denied tone for the invalid path; warm resolution for the
healthy app. Keep denial and success visually distinguishable without sound.

## 09 · 01:57–02:16 · The connected product

**Picture / motion:** Pull back from that same app to four connected surfaces:
Store, Guardian, Cloud, Agents. Use one benefit per surface: discover,
understand risk, operate on your infrastructure, inspect through tools.
A small lifecycle loop adds backup, restore and updates around the app.
These are planned screens, not a claim of an operating marketplace.

**Narrator:**
> The bigger vision connects discovery in Wazi Store, security in Guardian, and deployment and maintenance across your own servers and future managed compute. Backups, updates, recovery, and agent integrations should belong to the same understandable application lifecycle.

**On-screen text:** `Store: discover` · `Guardian: assess` ·
`Cloud: operate` · `Agents: connect`. Keep `Roadmap` visible.

**Sound:** Four soft notes build a single chord, linking the surfaces.

## 10 · 02:16–02:32 · What works today

**Picture / motion:** Cut to genuine captures of the current repository and
validation. Show readable compatibility rejection, wrong-service secret
rejection and passing CI for a pinned commit. Two or three highlights only.
Replace `Concept preview` with `Implemented: Phase 0 contracts`.

**Narrator:**
> Today, the foundation is working: typed packages, compatibility and policy checks, deployment-plan validation, and repeatable tests in green CI. Live deployment and Meta-powered repair are the next pieces we're building on that foundation.

**On-screen text:** `Implemented: contracts + validation`.
`Next: live deployment + Meta repair`.

**Sound:** Reduce the score so the proof and narration carry the moment.

## 11 · 02:32–02:46 · Where the idea can grow

**Picture / motion:** The app recipe becomes three modular cards: environment,
skill, planner. They connect in the order produced by the actual offline
S-Print resolver. A faint arena silhouette echoes the opening, but do not
show a robot being controlled or imply learned-model improvement.

**Narrator:**
> We're also exploring S-Prints: composable packages for AI environments, skills, and adapters. An experimental resolver already checks how those pieces fit. Actual world-model execution is future work.

**On-screen text:** `S-Prints · Experimental`.
`Offline compatibility prototype · No model execution`.

**Sound:** A short variation on the opening blue-core motif.

## 12 · 02:46–03:00 · The promise and payoff

**Picture / motion:** Return to the tiny gladiator. Her oversized weapon floats
steadily at her side; she is in control, not struggling. In the core, briefly
see the Wazi app card. The other gladiators lean in with curiosity. She
welcomes them into frame. Resolve to the Wazi wordmark and tagline; reserve
the last two seconds for a clean, readable end card.

**Narrator:**
> Wazi means open. Our ambition is to give small teams the power to turn useful code into applications people can trust and use. Bring the idea. Keep control. Build with Wazi.

**On-screen text:** `Wazi` · `From repository to trusted application.`
Add the final project/demo URL only when it is accessible to judges.

**Sound:** Warm orchestral/electronic resolve and a final small armour clink.
No new feature list or extra narration over the final card.
