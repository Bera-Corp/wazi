# Threat Model

## Protected assets

- host control capabilities;
- user and tenant data;
- credentials and generated secrets;
- signed manifests and catalogue trust;
- deployment, backup, restore, and rollback evidence;
- application data and infrastructure metadata.

## Main adversaries

- malicious upstream repository or compromised maintainer;
- malicious catalogue contributor;
- attacker controlling a container image or dependency;
- tenant attempting cross-tenant context access;
- prompt injection embedded in documentation or logs;
- compromised host or control-plane account;
- insider abusing deployment privileges.

## Critical controls

- treat upstream text as untrusted data;
- pin image digests and capture SBOMs;
- scan candidates in isolated environments;
- sign and revoke manifests;
- enforce tenant-scoped context retrieval;
- keep secret values outside prompts and telemetry;
- expose typed capabilities instead of a generic shell;
- require approvals for public exposure and destructive actions;
- create backup checkpoints before migrations;
- verify restore and rollback procedures;
- preserve tamper-evident audit evidence;
- expire host observations and version-sensitive context.

## Failure posture

When context is missing, contradictory, stale, unsigned, or incompatible, Wazi must stop rather than guess. AI confidence never overrides policy or compatibility failures.

## Concrete abuse cases (Phase 0 contract view)

Each case names the Phase 0 mitigation and the adversarial proof in
`evals/` or `scripts/test_adversarial.py`. Runtime enforcement in
Phase 1+ must not weaken these.

1. **Shell smuggling through parameters.** A planner emits
   `compose.pull` with `command`/`password` parameters.
   Mitigation: closed per-capability parameter schemas. Proof:
   `adv_plan-command-and-password-params.json`.
2. **Cross-tenant reference.** An envelope or approval from tenant A is
   presented for tenant B's host. Mitigation: tenant scopes must agree
   across envelope, host, policy, case, and approvals; approvals also
   bind host and envelope IDs. Proof: semantic tests "cross-tenant host
   profile" and "cross-tenant approval replay".
3. **Approval replay after plan edit.** A user approves a plan, then a
   step target is changed and the old approval is replayed.
   Mitigation: approvals bind the canonical digest of the exact plan;
   any edit changes the digest. Proof: "plan changed after approval".
4. **Approval replay after expiry.** An old approval is reused days
   later. Mitigation: approval `expiresAt` checked against the
   evaluation clock. Proof: "approval expired at evaluation time".
5. **Revoked manifest reuse.** A previously signed manifest is revoked
   after a vulnerability disclosure but an old envelope still names it.
   Mitigation: registry state is checked in hard filters; envelope
   expiry bounds the window. Proof: `eval_revoked-manifest.json`.
6. **Host changes between preflight and execution (TOCTOU).** RAM,
   ports, or Docker version change after the profile was taken.
   Mitigation: profiles expire, envelopes are digest-bound to the exact
   profile, and Phase 1 must re-probe before mutation. Proof:
   "host changed between preflight and execution",
   `eval_host-profile-expired.json`.
7. **Secret value smuggled in a default, URL, or parameter.** A
   candidate manifest puts a password in an environment default.
   Mitigation: sensitive names must use `secretRef`; closed parameters
   restrict field names but free-text values remain possible; scanning is a backstop,
   not the primary control. Proof: `adv_manifest-sensitive-default.json`,
   `adv_envelope-secret-value.json`.
8. **Prompt injection in upstream docs.** Documentation instructs the
   planner to skip verification. Mitigation: upstream text is data;
   plans can only use allowlisted capabilities with citations, and
   deterministic mode needs no model at all. Proof: capability and
   citation checks in `validate.py`.
9. **Rollback to nowhere.** A plan references a rollback step that does
   not exist. Mitigation: rollback steps are a separate typed sequence
   and every reference must resolve there. Proof: "rollbackStepId
   pointing nowhere".
10. **Privileged container by default.** A package requests a privileged
    container. Mitigation: `privileged` is `const: false` for the
    initial catalogue. Proof: `adv_manifest-privileged.json`.

## Experimental S-Print boundaries

S-Print packages may describe executable components or weights. The current
offline resolver never fetches or executes them. World interfaces are not
host capabilities. Tests reject missing/incompatible interfaces, dependency
pin changes, composition cycles, ambiguous providers, aggregate resource
overcommit, ungranted permissions, traversal paths and non-simulation scope.
Network/device/physical-control permissions are outside this schema version.
Training metadata is rejected for composition.

Future loaders need artifact-byte verification, licence/dataset review,
signature/revocation checks, isolation, quotas and atomic rollback. Prevent
package instructions from changing runtime policy, reject unsafe weight
deserialisation, and measure simulator exploitation, reward hacking and
long-horizon regressions. Metadata compatibility does not prove physical
safety or model quality. See [the extension plan](WAZI_SPRINTS_PLAN.md).
