# Wazi services (Phase 1)

Go workspace for the deterministic vertical slice. Signature verification uses
the standard library; canonical schema validation uses jsonschema/v6.

**31 October 2026:** the bounded Gitea pilot runs on bera-003, including the
signed A/B lifecycle, approvals, journal recovery and the documented live denial
subset. See [the live evidence](../docs/PHASE1_LIVE_EVIDENCE.md) for current
scope, limits and CI status. General deployment remains disabled by default.
The incremental implementation notes below predate live pilot qualification.

| Path | Responsibility |
|---|---|
| internal/verify | Strict canonical JSON, versioned signer-policy loading, and offline Cosign v3.1.4 ECDSA verification using policy-pinned keys |
| `cmd/agent` | Host agent: live inventory → host-profile, optional mTLS pairing + submission. Later: typed capability execution, evidence reporting |
| `cmd/controlplane` | Control plane: pairing (one-time codes, CSR issuance, revocation), authenticated host-profile intake, signed manifest/procedure registry, and configured approval pilot |
| `cmd/pilot-agent` | Operator-configured rootless Gitea worker: persistent queue, immediate preflight, typed capabilities, backup/restore, and journal reconciliation |
| `internal/pilotqueue` | Durable private Unix-socket job transport, identity conflict rejection and retained results |
| `internal/agentclient` | Agent-side enrollment (node key stays on the node; only a CSR is sent), mTLS profile submission, signed release fetch/verification, read-only execution binding, and approval checks |
| `internal/controlplane` | HTTP surface: strict host-profile intake, operator-authenticated manifest/procedure ingest, paired-node reads, and approval issue/consume routes; deployment remains disabled (501) |
| `internal/pairing` | Pairing registry and CA: one-time expiring codes bound to (tenant, node), certificate binding, revocation |
| `internal/approvalregistry` | Private single-writer durable approval records with exact-binding, expiry, and one-job consumption |
| `internal/operationjournal` | Private single-writer durable operation journal; records typed mutation intent and forces explicit recovery after interruption |
| `internal/contracts` | Go contract decoders and embedded schema validation, including NODE-002 budget/storage semantics; validation does not authenticate approval or prove live host qualification. The JSON schemas in `../schemas/` remain the source of truth |
| `internal/probe` | Linux host inventory (OS, arch, resources, daemon/Compose versions, ports, filesystem writability) |
| internal/manifestregistry | Private single-writer manifest and procedure store; verifies signatures and schemas before atomic persistence and re-verifies records at startup |

## Build and probe a host

```sh
# On the target Linux host (or cross-compiled, see below)
go build -o /tmp/wazi-agent ./cmd/agent
/tmp/wazi-agent -tenant tenant_demo
```

Cross-compile from a developer machine:

```powershell
$env:GOOS = "linux"; $env:GOARCH = "amd64"
go build -o wazi-agent ./cmd/agent
```

The emitted profile is validated against
`../schemas/host-profile.schema.json` by the Phase 0 validator tooling.

## Pairing and authenticated intake

The control plane binds `127.0.0.1:8080` by default and refuses a
non-loopback bind without mTLS (`-tls-cert`/`-tls-key`, plus
`-ca-cert`/`-ca-key` or an ephemeral dev CA). An operator mints a
one-time pairing code for a (tenant, node); the agent generates its
private key locally and enrolls with only a CSR:

```sh
go build -o /tmp/wazi-controlplane ./cmd/controlplane
/tmp/wazi-controlplane -addr 127.0.0.1:8080 -dev-allow-plaintext-profiles # dev only, loopback

go build -o /tmp/wazi-agent ./cmd/agent
/tmp/wazi-agent -tenant tenant_demo \
  -controlplane https://cp.example:8443 \
  -pairing-code <one-time-code> -ca ca.crt
# later runs reuse .wazi-agent/ identity and submit over mTLS
```

Certificates are bound to exactly one tenant and node. The agent
only sends pairing codes and profiles to `https://` control-plane
URLs and never follows redirects. One node identity (flag, existing
certificate binding, or hostname) is resolved per agent run and used
for enrollment, inventory and submission. Intake enforces the
host-profile intake checks (exact property allowlists, required-field
presence and nullability, freshness, body limits/deadlines, and that
binding); this is not a claim of full canonical-schema parity, and
filesystem/provenance are additionally required at runtime although
optional in the schema. Listings are tenant-scoped. The final profile store is authorised atomically
with revocation, so an in-flight upload cannot land after
revocation completes. Revoking a node also invalidates every
outstanding pairing code for it; re-enrollment requires a freshly
minted code. With `-registry-file` (or `WAZI_REGISTRY_FILE`) the
pairing registry is durable: nodes, revocation epochs and consumed
codes persist atomically with codes stored hashed only, a single
writer holds the store lock (a stale lock must be verified and
removed by an operator), failed snapshot writes roll live state
back, and a corrupt store refuses startup rather than
resurrecting state; without it the registry is in-memory and a
restart fails closed. File and directory are synced on writes
where the platform supports directory sync.
Operator code-minting/revocation requires a loopback caller and,
when `WAZI_OPERATOR_TOKEN` is set, that token. Server trust (`-ca`)
is operator-supplied and never overwritten; the issuing CA is
validated (chain, binding, key match) and persisted separately to
`issuer.crt`. Identity persistence stages every file, captures
backups before moving anything, and rolls committed files back on
error; instantaneous power loss mid-commit remains a filesystem
ordering limit, not a guaranteed restore. Renewal is two-phase
with `-renew`: the server records the replacement as pending
(old certificate keeps working), the agent persists it, then
activation with the new certificate retires the old one; a lost
activation leaves a `renewal.pending` marker the agent completes
automatically before submission. After expiry a node re-pairs
with a fresh code (expired registrations are replaceable; live
ones are not), and after revocation it re-pairs with a fresh code
in the new epoch. Deployment routes return
`501 deployment_disabled` until registry and agent signature
checks, revocation, approval, runtime boundary, journal recovery, and
stateful lifecycle gates pass.

## Phase 1 order

Go scaffolding → real inventory → secure (mTLS) pairing → offline signed-record
verification → control-plane manifest/procedure/revocation ingest → agent
read-only trust preflight → typed execution → lifecycle evidence → adversarial
end-to-end tests.
See docs/BACKLOG.md and [the selected Gitea reference app](../docs/PHASE1_REFERENCE_APP.md).
Pairing is implemented with the mTLS acceptance matrix in the control-plane
server tests. The verifier and persistent registry are implemented, including
full manifest and procedure schema/signature validation. The control plane
also stores signed revocation lists under an operator token and serves them to
paired nodes. It rejects sequence rollback and changed content at a reused
sequence across restart. The agent client fetches and verifies the manifest,
procedure, and current revocation list, checks the caller-supplied freshness
maximum, and durably advances a local anti-rollback cursor before returning.
The caller must set a positive revocation maximum age; no deployment value is
configured. The paired client can fetch an authenticated capability-policy
bundle, but the live service is not configured to serve it and no executor
consumes it yet. A local read-only VerifyExecutionBinding helper validates the
envelope and plan schemas, exact signed procedure and rollback references,
tenant and policy-bundle references, capability limits, parameter contracts,
approval declarations, and record expiry. It returns envelope, plan, and
procedure digests for a future approval record. Release retrieval can also
return defensive copies of the exact canonical manifest and procedure bytes
whose signatures were checked, so an executor can bind those same bytes
without refetching. VerifyApprovalRecords checks
consumed approval records against exact case/plan/envelope/tenant/host/app
identities, time bounds, and caller-supplied role/quorum rules. The caller
must obtain the policy snapshot through the paired fetch helper; live policy
configuration, invocation of the approval-consumption helper from an executor,
and distinct human credential custody remain open.

The standalone approval pilot issues records through `POST /v1/approvals` and
consumes them through POST /v1/approvals/{approvalID}/consume. The process
requires loopback access, WAZI_APPROVAL_REGISTRY_DIR, and WAZI_OPERATOR_TOKEN
for its administrative surface. Approval issuance uses either the legacy
WAZI_APPROVER_ID and WAZI_APPROVER_ROLE pair or WAZI_APPROVER_CREDENTIALS_FILE.
The service binds each record to the
canonical plan and envelope digests and a case ID, caps validity at 15 minutes
and at the context expiry, and persists single-use consumption. Consumption
requires the paired mTLS node for the approved tenant and host. A retry from
the same job is idempotent; another job is rejected.
The legacy configured token still maps to one operator ID and cannot satisfy a
two-maintainer rule. A multi-principal pilot can instead load
WAZI_APPROVER_CREDENTIALS_FILE, a canonical JSON file containing distinct
approver IDs, roles, and SHA-256 credential digests. Keep that file private to
the service identity with mode 0600; send credentials in the
X-Wazi-Approver-Credential header. Generate credentials with at least 32 random
bytes. The operator token remains a separate administrative credential and
does not bypass per-approver checks when the credential file is configured.

The agent client can consume several case-bound approvals over paired TLS,
check each response digest, and enforce a caller-authenticated role/quorum
rule. This proves distinct configured identities, not that separate people
control them; custody of the credentials remains an owner responsibility.
The approval API and agent client are not yet called by an executor. Fresh-host
validation, runtime authorization, Compose artifact verification, journal
integration, and mutation execution remain open; deployment stays disabled at
501.

The standalone `internal/operationjournal` package now stores exact case, job,
plan, envelope, tenant, host, app, and manifest bindings in a private,
single-writer directory. It appends a typed mutation intent and parameter
digest before a caller may invoke an adapter, records only bounded result
codes, and never stores parameter values. Duplicate delivery is idempotent by
operation/job identity; a second job cannot reuse a case. On restart, an
unresolved mutation is listed for explicit resource reconciliation and is
never replayed. Reconciliation blocks the old operation, while a started job
without an unresolved intent is blocked on startup. Corrupt state, stale locks,
or persistence errors fail closed. The package is not yet wired to the agent or
executor, does not verify authorization or lifecycle evidence, and cannot
authorize a mutation by itself.

## Verified manifest and procedure registry

Enable registry ingest by configuring both WAZI_SIGNING_POLICY_FILE and
WAZI_MANIFEST_REGISTRY_DIR (or their matching command-line flags). The policy
uses the versioned contract in schemas/phase1-signing-policy.schema.json.
WAZI_OPERATOR_TOKEN is required for manifest and procedure writes. Registry
reads require a paired node certificate; loopback development mode uses the
operator token. Procedures are stored separately from manifests and keyed by
procedure ID plus version.
The operator writes POST /v1/procedures; paired nodes read
GET /v1/procedures/{procedureID}/{version}.
The operator also writes POST /v1/revocations; paired nodes read
GET /v1/revocations/current. Revocation records are signed, schema-checked,
and sequence-bound. A list identity change requires operator inspection and
state migration; the API has no reset path.
The registry is a private directory with a single-writer lock; startup rejects
unverified or unexpected stored files. A stale lock after a crash requires
operator inspection.

The registry checks canonical JSON, the Cosign signature, signer key
fingerprint, and the complete app-manifest or procedure Draft 2020-12 schema
before persistence. Tests prove that a validly signed manifest with an
unknown field is rejected without writing a registry file; procedure
signatures and schemas are re-checked after restart. Revocation records require
a signature and sequence; newer signed records replace the current list
durably, and older or conflicting records are rejected. The agent cursor
survives restart and rejects rollback. Freshness is an explicit caller
parameter. The read-only plan-binding helper is implemented but not called by
an executor. The approval store and verifier are also separate from that
helper and from any executor. Fresh-host checks, multi-principal approval
policy, runtime authorization, and execution remain unimplemented. POST
/v1/deployments remains 501.
