# ADR-001: Canonical JSON, signature payloads, signer trust, revocation

Status: accepted for Phase 0 contracts, 29 October 2026. This ADR must be
implemented identically by every language that produces or verifies Wazi
records (Python today; Go services in Phase 1) before those services may
consume signed records.

## Canonical JSON form

The canonical form of a record is defined operationally by the test
vectors in [canonicalization-vectors.json](../schemas/canonicalization-vectors.json):

- UTF-8 encoding, no byte-order mark.
- Object keys sorted by Unicode code point; arrays keep their order.
- No insignificant whitespace: `,` and `:` separators only.
- Strings: only `"`, `\`, and control characters are escaped; non-ASCII
  is emitted as raw UTF-8 (never `\uXXXX` for printable characters).
- Numbers: integers without decimal point; non-integers in shortest
  round-trip decimal form (Python `json` semantics). Records should
  prefer integers and strings over floats for signed material.
- Digests are `sha256:<lowercase hex>` over the canonical UTF-8 bytes.

Any implementation must reproduce every vector's canonical string and
digest before it may sign or verify. `scripts/validate.py` checks the
vectors on every run.

## Signature payload

- The signed payload is the canonical form of the record with its
  top-level `signature` field removed.
- `signature.payloadDigest` is the `sha256:` digest of that payload.
  Changing any signed field without re-signing changes the payload
  digest and is detected semantically (see `check_signed` in
  `scripts/validate.py`).
- Phase 1: `signature.value` is a signature over the payloadDigest
  string bytes, produced with the declared `algorithm`
  (sigstore-cosign preferred). Phase 0 values are placeholders; the
  binding is checked, the cryptography is not, and no Phase 0 record may
  be presented as cryptographically verified.

## Signer trust

- A tenant policy bundle declares `signing.trustedSigners` and allowed
  `algorithms`. Signed manifests and procedures from anyone else, or
  with a disallowed algorithm, are rejected semantically.
- Phase 1 replaces the `signedBy` string with a verifiable identity
  (Sigstore certificate identity/issuer) and checks it against the same
  allowlist concept. The allowlist lives in policy, never in the record
  being verified.

## Revocation

- Revocation is registry data, not manifest state:
  [revocation-record.schema.json](../schemas/revocation-record.schema.json)
  lists revoked application IDs (optionally release-scoped) with reasons.
- Lists are freshness-bound (`generatedAt`/`expiresAt`). When policy
  sets `rejectRevokedManifests`, semantic validation fails closed if
  the list is missing, stale, or future-dated.
- Phase 1 registries must serve revocation lists with a maximum age set
  by policy and must re-check revocation at deployment time, not only
  at envelope creation.

## Consequences

- Go and TypeScript implementations need canonical-JSON libraries or a
  small verified encoder; both must pass the vectors in CI.
- Records that embed floats in signed material are discouraged; future
  schema revisions may forbid non-integer numbers in signed records.
- Demo Phase 0 signatures prove tamper evidence of the *binding*, not
  authenticity. Public documentation must keep saying so until Phase 1
  cryptographic verification ships.
