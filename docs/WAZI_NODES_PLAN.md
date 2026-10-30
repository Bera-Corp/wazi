# Wazi Nodes and Fabric

Design integration: 30 October 2026. Source: the owner's supplied
[Wazi Nodes research](research/WAZI_NODES_RESEARCH_2026-10-30.md). The report
is research input, not operational instructions or implementation evidence.

## Adopted product direction

**Wazi Nodes** makes owner-controlled hardware a first-class application
destination. **Wazi Fabric** is the proposed common view of those nodes,
existing VPS resources and future managed compute. Capacity and permission
determine eligibility; provider names do not establish compatibility.

Start with the owner running their own workloads on one Linux machine.
The verified [bera-003 development host](DEVELOPMENT_ENVIRONMENTS.md) is the
first test target. It is reachable by SSH/sudo. Local Go agent/inventory,
mTLS pairing (one-time codes, revocation epochs, durable registry, renewal)
and authenticated intake are implemented in `services/` with adversarial
tests; pairing acceptance remains open. Local code now renders signed
per-service CPU/RAM ceilings, checks those ceilings against the approved
application allocation, checks signed per-volume storage ceilings, and fetches
the referenced policy bundle over paired mTLS. The live executor, per-volume
quota application, service/helper role enforcement and lifecycle runtime are
not integrated. The dedicated rootless runtime boundary passed reboot/
reconnect acceptance; the most recent managed-volume qualification passed at
2026-10-30T17:15:43Z and expired at 17:30:43Z. Deployment remains disabled
(501).

## Smallest useful build

```text
Pair a node -> observe capabilities -> choose contribution limits
  -> approve the node -> select one compatible application
  -> verify signed plan -> run with enforced limits
  -> retain data through container restart -> report disconnect/recovery
```

This extends the deterministic Phase 1 slice rather than creating a second
agent stack. Reuse its device identity, mutual TLS, registry, policy,
approval and evidence components. Developer SSH is a bootstrap channel;
the product agent only accepts authenticated, typed capabilities.

| Work package | Phase / status | Acceptance evidence |
|---|---|---|
| NODE-000 Development target | Access verified | SSH key auth, sudo, OS/architecture and current tools recorded; no claim of installed runtime |
| NODE-001 Identity and inventory | Phase 1, implemented (acceptance open) | Node-generated private key with CSR-only enrollment; one-time expiring codes bound to (tenant, node) and invalidated by revocation; wrong-tenant/replay/revoked/renewal/in-flight-commit tests in `services/`; durable hashed-code registry and operator token; pairing sign-off remains open |
| NODE-002 Contribution contract | Phase 1, contracts, budget and live storage qualification recorded | `contribution-budget` and `managed-volume-qualification` v1 schemas are embedded by the Go contract package; Python and Go enforce resource nesting, unique allocation roles, local-daemon constraints and exact validity-window ordering. Owner-authorized budget revision 2 preserves the 4 CPU / 2048 MiB RAM / 98304 MiB storage cap. The post-reboot live rootless named-volume project-quota test passed at 2026-10-30T17:15:43Z and expires at 17:30:43Z. Local paired mTLS policy retrieval and resource binding are implemented and tested, but live configuration and executor consumption remain open |
| NODE-003 One application | Phase 1, predeployment runtime qualified | Separate rootless Docker identity, rootful-socket denial, aggregate CPU/RAM cgroup limits, filesystem quota behavior, and local per-service CPU/RAM rendering are verified. Signed Gitea image/provenance, per-role/helper limits, per-volume quota application, app health and lifecycle evidence remain open |
| NODE-004 Availability/reconnect | Phase 1 extension, planned | Heartbeat expires to unavailable; UI says unreachable/unknown rather than deleted; reconnect re-inventories and reconciles the same deployment without duplicates |
| NODE-005 Selection and private access | Phase 2, planned | Inspectable eligible/ineligible destination reasons, availability classes, per-owner policy, private connectivity and atomic resource reservation |
| NODE-006 Durable operations | Phase 4/alpha, planned | Encrypted backups, restore evidence, quota accounting, drain/revocation and safe removal; actual storage reservation strategy |
| NODE-007 Broader Fabric | Later research | GPU qualification, organisational pools, cluster adapters, hardened VM boundaries and policy-controlled migration/failover |

Third-party workload rental, Windows/macOS hardware providers, TPM
attestation, high availability and automated cloud failover are later design
work. None is implied by successful SSH access or a container starting.

## Contract amendments before runtime implementation

Do not add unrecognised fields to v1 records, which reject additional
properties. Make versioned contract changes with compatibility/migration
tests before the Go agent consumes new resource or lifecycle records.

| Contract | Required decisions |
|---|---|
| Node identity/qualification | Tenant/node ID, credential references, observation source, freshness, approval, supported OS/runtime and explicit qualification result; re-probe and bind the result immediately before the first mutation |
| Capacity and contribution | Logical CPU units, memory/storage bytes, physical totals, owner-approved budget, system reserve, active reservations, available capacity and expiry; include backups, helpers and runtime overhead |
| Workload resources | Minimum versus enforced maximum CPU/RAM; storage requirements versus enforceable quotas; GPU/device requests only when supported; never treat free-space observations as a quota |
| Availability | Best-effort/scheduled/always-on/interactive declaration separated from measured heartbeat and observed uptime; no invented reliability percentage |
| Placement | Owner/tenant boundary, operation, signed release, current resource snapshot and allocation reservation bound into the approved plan |
| Lifecycle/checkpoint | Fresh install attests absence of the deployment's existing data; upgrade/restore/rollback require a verified checkpoint of affected existing volumes |
| Evidence | Pairing, inventory, policy decisions, enforced limits, health, backup/restore, disconnect/recovery and cleanup tied to exact node/deployment/plan |
| Human authority and recovery | The local approval API and durable journal cover case/digest binding, one-job consumption, typed mutation intent and restart reconciliation. The pilot uses one configured approver principal; the credential loader accepts this configuration, while policy rules requiring two maintainers still require two distinct live approvals. This does not prove independent human custody. Live configuration, authenticated policy integration, executor wiring, owned-resource verification and recovery/revocation acceptance remain open. |
| Policy and revocation trust | Policy/revocation come from an authenticated authority, never from the artifact; signer identities map to actual public keys; revocation generations persist monotonically and reject rollback |

The current profile's dockerRootWritable=false is an accurate direct-filesystem
observation. The Phase 0 validator rejects a manifest with volumes when that
flag is false. Do not change the observation or weaken that check to make
execution pass. The versioned daemon-managed-storage qualification contract
now exists (`schemas/managed-volume-qualification.schema.json`): it records
the selected Docker daemon (local endpoints only), managed-volume capability,
available space, and enforcement method; unenforceable quotas must declare
`preflight-only`, so their capacity is a preflight constraint, not a hard
limit. The owner-authorized numeric budget is recorded in
`artifacts/bera-003/20261030T142519Z/contribution-budget-owner-authorized.json`.
A live rootless-daemon qualification is recorded in
`artifacts/bera-003/20261030T143226Z/managed-volume-qualification.json` and
expires at 2026-10-30T14:47:26Z. Both records remain separate from the host
profile and do not authenticate release approval or authorize execution; the
checked-in budget example remains a synthetic draft.
The current post-reboot qualification is recorded at
`artifacts/bera-003/20261030T171532Z/managed-volume-qualification.json` and
expires at 2026-10-30T17:30:43Z. It remains separate from the host profile
and does not authorize workload execution.

Resource units and overcommit policy must be explicit. Do not treat total
host RAM as allocatable RAM, a sample of free RAM as a permanent guarantee,
or a root-filesystem free-space check as an application quota. A fresh-install
attestation cannot bypass checkpoints for an existing deployment.

## Isolation, identity and networking

The first adapter uses Docker Compose against a dedicated rootless Docker
daemon owned by the Wazi runtime identity, isolated from the host's existing
rootful daemon. The Wazi agent must not have permission to open the existing
rootful Docker socket. Rootless daemon access is still powerful within its
own workload domain, so keep that daemon exclusive to the approved Phase 1
application and expose only the typed capability interface to deployment
requests. Add a negative live test proving the Wazi identity cannot list,
mutate, stop or remove unrelated rootful Docker resources.

Generate explicit CPU and memory limits, then inspect actual cgroup/runtime
enforcement on the target. A schema minimum is not a limit. Rootless Docker
must be qualified for cgroup enforcement before proceeding; if it cannot
enforce the approved limits on bera-003, keep execution disabled and select
an alternative boundary through an explicit design review. Do not change the
rootful daemon, existing workload networking, or Tailscale configuration as
part of qualification.
[Docker's resource constraints](https://docs.docker.com/engine/containers/resource_constraints)
describe the controls and their limits; hardening profiles need corresponding
runtime evidence before being displayed as enforced.

The host's existing Tailscale connection provides private development
reachability. It does not replace Wazi's mutual authentication, node approval
or tenant authorisation. Tailscale's
[one-off authentication and device approval](https://tailscale.com/docs/features/access-control/auth-keys)
are onboarding references, not a claim that Wazi has implemented pairing.
Do not pass tailnet keys or development SSH credentials to planners.

Keep application access private initially. Exit-node/subnet-router service
on bera-003 was configured separately; it grants no authority to publish an
application, change tailnet ACLs or expose new LAN services. Public access
requires a separate approved gateway/proxy path and exposure evidence.

Distinguish “account approved”, “device identity authenticated”, “inventory
observed”, “runtime qualified” and “hardware attested”. Avoid a single
unqualified “Node verified” badge. TPM/secure-boot claims require actual
attestation support and evidence, not an agent-supplied boolean.

## Reliability and persistence

Define a heartbeat deadline and reconnect behaviour. A missed heartbeat
means current application state is unknown/unreachable; it does not prove
data loss or healthy operation. Retain desired state, expire old observations,
and re-verify identity/authorisation before reconciling on return.

Application data belongs to declared volumes. Demonstrate container restart
and data retention separately from backup and restore. Stop/removal must
specify what happens to running workloads, credentials, secrets, retained
data and queued operations. A private LAN route is not a backup channel.

Rollback acceptance must be stateful: install release A, write known
application data, take a consistent backup, upgrade to B, restore into a
clean target, verify the data through the application, then roll back to A
from the correct checkpoint. Returning to an empty pre-install host proves
fresh-install cleanup only; it does not prove upgrade rollback.

Before any execution, integrate the standalone operation journal with the
agent gate and persist its entry with the deployment case, exact
manifest/plan/envelope digests, operation ID, step state and owned-resource
references. Local journal tests reopen persisted pending intent, block restart
before the next mutation, reject duplicate delivery and pending cancellation,
and cover ambiguous outcomes and corrupt state. Abrupt-kill lock recovery is
still open. The live executor must test interruption after every mutation
boundary, revocation during execution, evidence-store failure and restart
against the host's actual owned resources. Recovery must reconcile those
resources and avoid repeating completed destructive steps.

## Report assessment and submission value

Adopt now: Linux-first agent, own hardware/own workloads, measured inventory,
explicit contribution budget, typed execution, private access, persistent
data and visible interruption/recovery. Keep multi-cluster orchestration,
resale and advanced attestation out of the first implementation.

The report's example specifications, reliability figures, hardware numbers,
pricing implications and uniqueness claims are illustrative/unverified.
Viewer citation IDs are archived as supplied and are not portable sources.
The two primary references above were checked on 30 October 2026; more
product-specific research is needed before choosing later adapters.

| Judging pillar | Nodes contribution | Proof needed |
|---|---|---|
| Impact 30% | Reuse owned hardware as a useful application destination | Same-task outcome and owner contribution budget; no invented cost savings |
| Meta AI Tooling 25% | Later bounded assistance explaining requirements/eligible targets | Actual eligible Meta call and cited input/output; inventory and policy remain deterministic |
| Technical Depth & Execution 20% | Authenticated node, real limits, persistence and reconnect | Target-host receipts plus pre-mutation rejection tests |
| Demo 25% | Show a real owned machine becoming a useful application target | App working, limits visible and honest unavailable/recovered states |

Within the three-minute film, make the owned machine the destination in the
existing app journey. The report's longer disconnect/reconnect sequence is
supporting proof or a labelled compressed capture, not an extra five-minute
segment. Preserve the transcript's current implementation/concept labels.
