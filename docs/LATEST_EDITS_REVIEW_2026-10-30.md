# Latest edits and environment review — 30 October 2026

Scope: existing uncommitted judging/video/roadmap work, revised Phase 1
backlog, the supplied Nodes report and read-only access checks of bera-003.
User work was preserved. No Phase 1 service or application was deployed.

## Findings and disposition

| Area | Review result | Action |
|---|---|---|
| Phase 1 order | Matches the accepted deterministic sequence | Preserve Go -> inventory -> pairing -> verification/registry -> typed execution -> lifecycle evidence -> adversarial tests |
| Trust boundary | Registry verification and agent re-verification precede mutations; synthetic fixtures remain fixtures | Require real signer trust configuration and fresh revocation before real deployment; do not treat payload hashes as authenticity |
| Fresh installs | Correctly separated from existing-data operations in planning | Add versioned attestation/checkpoint amendments and semantic negative tests before agent execution |
| Linux readiness | Key authentication and sudo work; Go/Docker are absent | Mark SSH/bootstrap usable and runtime qualification pending |
| Backlog status | Older partial deployment-graph wording and suite counts conflicted with later resolution addenda | Reconcile contract status and remove stale numeric summaries; preserve runtime caveats |
| Judging/video | Four pillars and three-minute brief are integrated | Preserve the existing transcript and its concept-versus-implemented labels |
| Nodes report | Fits the existing agent/host slice | Integrate a focused own-hardware path; larger Fabric features remain planned |
| Credential discovery | No existing local connection environment file | Add ignored .env.local with metadata/key path, portable .env.example and a checked SSH helper; no private key copied |

## Important implementation follow-ups

- `signing.trustedSigners` currently identifies allowed signers; Phase 1
  must decide how identities resolve to trusted public keys or keyless
  issuer/identity constraints before using cosign. Public keys are not secrets.
- Define the fresh-install attestation's observation time, tenant/node/
  deployment scope and absence check. An upgrade cannot opt into the
  fresh path merely because its selected volume name is new.
- The host is also a tailnet exit node/subnet router. Development tests
  must preserve those separately authorised services and the management path.
- Node qualification must use current runtime facts and owner contribution
  limits. Today's total/free memory is not an allocatable resource policy.

## New Go scaffolding: actionable findings

The untracked `services/` module now contains an inventory agent and HTTP
control-plane skeleton. Local `go test ./...`, `go vet ./...` and a Linux
amd64 cross-build pass. The current tests exercise ID sanitisation and two
version examples; they do not validate a real emitted host profile against
the JSON schema or prove live Linux execution. Existing Phase 0 CI does not
yet qualify this new module.

1. **P1 — Unauthenticated control plane binds every interface.**
   [main.go](../services/cmd/controlplane/main.go#L55) binds `:8080`;
   unauthenticated callers can list all profiles and replace profiles by
   host ID. A development-network warning does not constrain the listener.
   Default to loopback before starting it; require mTLS and tenant-scoped
   identity/storage before permitting remote intake.
2. **P1 — Inventory loses version identity.**
   [probe.go](../services/internal/probe/probe.go#L17) extracts the first
   numeric triple, so `27.3.1-rc1` becomes `27.3.1`, defeating the existing
   prerelease rejection. The `docker --version` call reports the client,
   not the daemon executing workloads. Query the server explicitly and
   preserve/reject prerelease versions; test differing client/server
   versions and prerelease output before trusting compatibility results.
3. **P2 — Intake does not enforce the contract.**
   [main.go](../services/cmd/controlplane/main.go#L33) only checks JSON decode,
   host ID presence and schema version. It accepts missing tenant/resources,
   unknown fields and trailing JSON, without a request-size bound; the map
   also lacks a tenant key and retention limit. Enforce the schema, freshness,
   authenticated tenant binding, body limits and bounded storage before
   treating intake as a source of deployment facts.
4. **P2 — Observations can imply unsupported guarantees.**
   [probe_linux.go](../services/internal/probe/probe_linux.go#L130) treats an
   existing Docker data directory as writable without proving write access;
   connected UDP sockets are omitted by the state filter, although they can
   retain bound ports. Probe the actual required operation and preserve
   uncertainty/failure; test connected UDP sockets and read-only storage.
   Bound external Docker commands with timeouts so inventory cannot hang.

These are review findings; the service source was preserved. Pairing,
cryptographic verification and the Wazi Node execution runtime remain
Phase 1 work. No service was started or deployed by this review.

## Verification of this documentation/access change

The complete `scripts/validate.ps1` suite passes, including contract,
semantic adversarial, S-Print, secret-scanner and renderer checks. The SSH
helper passes PowerShell parsing and live `-Check` (hostname, identity,
passwordless sudo and Linux architecture). `.env.local` is ignored by Git;
only metadata placeholders are in `.env.example`. The research source is
archived unchanged below its provenance header, with its source SHA-256.

## Resolution addendum — 30 October 2026 (later sessions)

The findings in this review are historical and are preserved above.
Their dispositions, implemented in `services/` after this review:

| Finding | Resolution |
|---|---|
| P1 unauthenticated listener on every interface | Loopback default; non-loopback binds refused without mTLS; profile intake authenticated and tenant/node-bound |
| P1 inventory loses version identity (client, prerelease) | Daemon (`Server.Version`) queried; prerelease preserved and rejected; distro build metadata normalised; subprocess timeouts |
| P2 intake does not enforce the contract | Raw exact-property allowlists with duplicate/alias rejection, presence/nullability checks, freshness, body size and read deadlines, bounded tenant-keyed storage |
| P2 observations over-claim (writable dir, UDP, storage, context) | Writability proven by an actual write; connected UDP counted; storage measured at the Docker root; remote Docker contexts refused; unreadable port tables fail the probe |

Pairing followed in the same sequence: one-time codes bound to
(tenant, node), CSR-only enrollment with node-generated keys,
revocation epochs that invalidate spare codes, atomic
authorize-and-commit against revocation (both lock orderings
tested), durable hashed-code registry, operator token, certificate
renewal, and HTTPS-only/no-redirect agent transport with separate
server/issuing trust. Pairing acceptance remains open and
deployment remains disabled (501) until signature verification
and live host qualification pass. No service was deployed by
these changes either.
