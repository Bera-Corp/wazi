# Phase 1–5 closeout ledger

**Status:** active, 4 November 2026. This is a planning estimate and gate
tracker, not a claim that the phases are accepted or production-ready.

## Baseline and scoring

The repository does not define phase percentages. These estimates compare the
documented deliverables with implementation and retained acceptance evidence.
Local code without the required live or human evidence earns partial credit.
Recalculate the estimates when a gate below changes state.

The requested source branch was fast-forwarded to GitHub main at
`56b18e514b33fc79e2676cbaf0ec235de5447728`. The commit immediately before it,
`25351dc`, is the last recorded Bera-003 deployment; the main commit adds only
an agent instruction. That deployment is historical evidence, not a fresh
host-health observation. GitHub started CI run
[41870010011](https://github.com/Bera-Corp/wazi/actions/runs/41870010011);
its result is deliberately left for the later validation pass.

| Phase | Estimate | Accepted evidence | Remaining closeout gates |
|---|---:|---|---|
| 1 | 90% | All five closure gates for the bounded Gitea pilot are recorded as accepted. | Independent-human quorum, automatic certificate renewal, and automatic application recovery. |
| 2 | 70% | Offline retrieval; live Gitea, PostgreSQL, MySQL, and Nginx package journeys; native qualification. | Correctly staged fresh registry/revocation publication, paired beneficiary baseline, Immich lifecycle, and a real authorized Meta model run. |
| 3 | 40% | Tenant-scoped candidate, receipt-validation, review/promotion, catalog, and workbench foundations. | Isolated runner, genuine signed receipt, independent review, signed publication, install, and live workbench acceptance. |
| 4 | 30% | Local age-encrypted PostgreSQL/MySQL checkpoint implementation and regressions. | MySQL disposable-database preflight, Nginx checkpoint coverage, protected live identities, off-host restore, lifecycle matrix, and platform qualification. |
| 5 | 20% | Existing private pilot, catalog, and consent-bound contribution foundations. | Five to ten qualified applications across local/VPS paths, support workflow, and measured alpha reliability gates. |

The Phase 1 pilot is accepted within its explicitly bounded scope. The 90%
estimate includes the wider reliability and independent-review work listed
above.

## Selected Phase 3 application

Use Uptime Kuma as the first new catalog adapter and end-to-end candidate.
Upstream release 2.5.9 resolves to source commit
`df601c58d9205562f34cfe5ef35264b341019d19`; the release asset
`dist.tar.gz` reports SHA-256
`f979084bbc74dc80a142d91d09b807f0a4ed52c06830c0e44c36adc9e77c064b`.
The pinned release reports an MIT license. Its upstream documentation names
container port 3001 and persistent data at `/app/data`.

Build a Wazi image from the pinned source and record its immutable image,
archive, and provenance digests. The upstream `:2` container tag is mutable
and is not an approved Wazi release. Resource limits, network policy, health
contract, backup behavior, and installation eligibility still need review and
qualification before this candidate can execute.

## Closeout order

1. **Phase 1:** add a second independent maintainer; qualify renewal and
   application recovery using an approved test boundary.
2. **Phase 2:** stage the complete package tree beside signed records before
   registry publication; refresh and verify the package and revocation set.
   Record the same beneficiary task before and after Wazi, including time,
   interventions, outcome, and sample limits. Qualify Immich after its
   capacity and recovery plan is approved. Run Meta authoring only after an
   eligible provider/model, credentials, and a hard spend cap are authorized.
3. **Phase 3:** freeze the Uptime Kuma scope and policy; build a separate
   resource-bounded runner with restricted networking and a distinct signer;
   capture success and denial receipts; complete independent review,
   publication, install, and live workbench acceptance.
4. **Phase 4:** implement MySQL semantic preflight before target mutation;
   provision private age identities through the protected host process; prove
   ciphertext restore, invalid-input rejection, and off-host recovery. Cover
   Nginx, upgrades, migrations, canary, revocation, SBOM, and vulnerability
   evidence. Claim Linux, Windows/WSL2, or macOS only after that exact runtime
   passes its qualification.
5. **Phase 5:** expand the qualified catalog to five to ten applications,
   verify local and VPS user paths, then measure each alpha threshold in
   `MVP_ROADMAP.md` on its declared support matrix.

## Owner actions and dependencies

- A second independent maintainer is not available yet. Phase 1 production
  closure and Phase 3 promotion acceptance stay open until one can review.
- Provider credentials, model selection, spending approval, and expanded
  Immich capacity are not provisioned by this ledger. No real call or
  additional live capacity use is authorized here.
- The owner asked to postpone testing and review. No local tests or review
  were run for this closeout update; the GitHub workflow result above remains
  for the later validation pass.
- The owner’s hands-on pass is pending. After the candidate is available in
  the workbench, exercise catalog discovery, inspect source and environment
  evidence, review/approve the candidate, follow install progress, confirm
  health, and observe one documented failure state. Record a paired manual
  baseline for the same task.
- After that pass and evidence reconciliation, update the existing 180-second
  film transcript and capture the real journey. Keep concept footage distinct
  from implemented behavior and preserve the four judging weights.

No phase reaches 100% until every in-scope gate has implementation and
acceptance evidence. Deferred credentials, spending, capacity, reviewer, and
owner testing remain explicit dependencies rather than inferred completions.
