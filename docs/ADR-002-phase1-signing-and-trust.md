# ADR-002: Phase 1 offline signing, policy trust, and revocation

Status: offline signature, registry, and revocation controls implemented as
local development gates, 30 October 2026. Gitea is the owner-selected reference
application; a bounded numeric resource budget is recorded from the owner's
delegated sizing instruction. No production signing key or deployment approval
has been established.
This protocol is for the private development qualification path; it is not
a general-purpose Sigstore deployment or a production trust service.

## Decision

For this single-host pilot, this decision supersedes ADR-001's proposed
certificate-identity profile for Phase 1; it does not change the Phase 0
placeholder semantics or the v1 signature schema. Use Cosign's key-based
blob-signing command to sign the exact ASCII bytes of
the Phase 0 signature.payloadDigest string. Pin Cosign v3.1.4 for signing
and interoperability fixtures. The supported Phase 1 signature is ECDSA
P-256 with SHA-256. The Go verifier implements only that named profile with
the Go standard library and fails closed on other key types, hashes, or
signature encodings.

Use the raw base64 signature output in signature.value. Do not accept a
certificate, public key, signer identity, bundle key, or trust root supplied
by the signed artifact. The record's signedBy is a signer ID; the
authenticated policy maps that ID to a PEM public key and an SPKI SHA-256
key ID. The verifier checks the ID, key fingerprint, algorithm, payload
digest, record state, and signature before accepting a record.

The key-based offline pilot does not require Fulcio, OIDC, Rekor, a
transparency-log upload, or an online verification service. Cosign's
protobuf bundle format is not the Phase 1 record format. The exact CLI
flags and resulting raw signature bytes are pinned by the interop fixture
and documented in services/internal/verify/README.md. The pinned command
explicitly sets tlog-upload=false and use-signing-config=false; it does not
upload signing material or fetch a remote signing configuration.

## Policy and revocation authority

The current control-plane implementation loads the versioned signer policy
from a local operator-managed file at startup. The file maps signer IDs to
public PEM keys and SPKI SHA-256 fingerprints. The service exposes no policy
write route; a trust change requires an operator file change and service
restart. Manifest ingest requires a loopback caller and WAZI_OPERATOR_TOKEN.
Manifest reads require a paired node certificate when mTLS is enabled; the
loopback development mode requires the operator token.

This is a single-host development boundary. The local filesystem writer and
control-plane process are trusted. The pairing CA does not authenticate the
signing-policy file, and the service currently has no policy hot reload,
policy update API, or tenant-specific trust mapping.

Signed revocation lists are operator-published through POST /v1/revocations
and paired nodes read the current list through GET /v1/revocations/current.
The registry enforces one list identity and a strictly increasing sequence
for changed content across restart; an identical sequence and digest is
idempotent. The agent verifies the list signature and freshness, then
persists its accepted list ID, sequence, and digest before it can return a
release from the revocation-aware preflight. The preflight requires the caller
to supply a positive maximum age; no deployment freshness bound is configured
yet, and no executor invokes this preflight today. Registry storage and reads
do not themselves revoke or purge an app record. Deployment remains disabled.

The pilot does not claim protection against compromise of host root, the
operator token, or the control-plane process. A future distributed update
channel must authenticate policy and revocation changes with the configured
control-plane CA and operator authority before it can replace this local-file
boundary.

## Contract changes

Keep existing v1 records and their fixture semantics unchanged. Implemented:

- versioned signer policy schema with key ID derived from DER
  SubjectPublicKeyInfo, PEM public key, and the Cosign v3.1.4 profile;
- strict canonical JSON, duplicate-key rejection, signer allowlist, and
  cryptographic verification at manifest ingest; and
- full app-manifest Draft 2020-12 schema validation in the Go registry,
  plus procedure schema validation and signature verification, with embedded
  schema copies checked against the root contract files; and
- agent-side HTTPS/mTLS retrieval and independent signature/schema/identity
  checks for a matching manifest and procedure pair; and
- signed revocation-record schema with a monotonic sequence, operator-token
  publication, paired-node reads, registry anti-rollback, and an agent-local
  durable anti-rollback cursor plus caller-configured freshness validation;
- embedded execution-envelope and deployment-plan schemas, plus a read-only
  agent preflight that binds the exact signed manifest, main and rollback
  procedures, envelope, plan, caller-authenticated tenant and policy snapshot,
  capability allow/deny sets, parameter contracts, approval declarations,
  and expiry windows. The preflight returns canonical digests for a future
  approval record but does not authorize mutations; and
- a standalone approval pilot with a case-bound approval schema, a durable
  single-writer registry, loopback/operator-token issuance, paired-node
  consumption bound to the exact case/plan/envelope/tenant/host/application,
  expiry and one-job use; plus agent-side approval digest/time/role/quorum
  validation. The configured pilot has one operator identity and does not
  provide distinct-human quorum or executor integration; and
- a standalone durable operation journal that binds one job to the case,
  plan/envelope/manifest digests, tenant, node, app, and operation. It records
  allowlisted typed mutation intent before adapter invocation, keeps parameter
  values out of the log, blocks already-started jobs after restart, and sends
  unresolved intent to explicit resource reconciliation without automatic
  replay. The journal is not connected to an executor and does not authorize
  or evaluate lifecycle evidence.

Still required before deployment:

- execution integration: the agent helpers and approval API are standalone and
  are not combined into one immediate pre-mutation decision;
- authenticated retrieval or update of the capability policy snapshot. The
  local binding helper accepts that snapshot only from its caller and checks
  its identity, digest, tenant, and allow/deny rules against the envelope;
- deployment-configured revocation maximum age and integration of the
  revocation-aware fetch plus execution-binding preflight immediately before
  any future mutation;
- owner contribution budget revision 2 is recorded under the delegated sizing
  instruction. Per-role runtime reservations and enforcement still require
  executor integration;
- the live rootless managed-volume qualification is recorded separately from
  the truthful direct-filesystem dockerRootWritable observation and expires at
  2026-10-30T14:47:26Z; refresh it immediately before a later mutation;
- fresh-install absence attestation bound to tenant, node, deployment,
  manifest digest and fresh host-profile digest;
- distinct authenticated approver identities and policy-driven quorum. The
  current pilot maps one local operator token to one configured ID/role and
  stores caseId on the approval, but does not maintain a separate case store;
- operation-journal integration with the agent's immediate pre-mutation gate,
  capability adapter, and lifecycle evidence. The standalone store records
  intended resource references and external observation digests, but the
  adapter that verifies owned resources and the recovery review remain open.

The implemented signing-policy input is the local versioned signer file
described above. For distributed policy updates, the authenticated
control-plane channel and operator write authority are still required. No
artifact may carry a key that establishes trust for itself or for the policy
that authorizes it. Procedure signatures are checked against the same signer
policy at registry ingest and restart. The agent's plan-binding helper checks
the capability policy snapshot supplied by its caller, but the caller's
policy-snapshot authentication and its immediate pre-mutation integration
remain open.

## Runtime boundary implementation status

bera-003 now has a dedicated rootless Docker daemon owned by `wazi-runtime`,
separate from the existing rootful daemon. The runtime identity is denied the
rootful socket. The rootless service enforces an aggregate 2.625 CPU / 1,536
MiB cgroup ceiling and uses the separately quota-qualified ext4 filesystem.
The host profile's direct-filesystem dockerRootWritable=false remains
unchanged. This qualifies the development boundary only: per-role resource
limits, execution through typed capabilities, and reboot/reconnect evidence
remain open. If those checks fail, deployment stays disabled.

## Required proof

- A Cosign v3.1.4-generated signature over a payload-digest string verifies
  in Go with the pinned public key; altered payload, altered signature,
  wrong key, wrong signer ID, wrong algorithm and non-canonical record
  are rejected.
- Python/Go canonicalization vectors cover Unicode, large integers,
  float formatting, duplicate keys, key order and signature-field removal.
- Registry ingest and agent pre-mutation checks both verify the signature;
  policy and revocation rollback are rejected after restart. A stale or
  expired list, reused sequence, and changed list identity fail closed.
- An authenticated node cannot update policy, revocation or human
  approval. An operator update persists across restart.
- The live runtime cannot access the rootful Docker socket; the rootful daemon
  remained active at the same version and data root, with unchanged empty
  container/volume inventory and unchanged host network inventory during the
  recorded setup and qualification checks.

## Sources

- [Cosign v3.1.4 release](https://github.com/sigstore/cosign/releases/tag/v3.1.4)
- [Cosign blob signing](https://docs.sigstore.dev/cosign/signing/signing_with_blobs/)
- [Cosign verify-blob options](https://github.com/sigstore/cosign/blob/v3.1.4/doc/cosign_verify-blob.md)
- [Docker Rootless mode](https://docs.docker.com/engine/security/rootless/)
- [Docker daemon security](https://docs.docker.com/engine/security/)
