# Offline Cosign v3.1.4 blob profile

This package verifies signed Wazi records using the policy-supplied ECDSA
P-256 public key. It does not import Cosign or Sigstore libraries at runtime.
The signed message is the exact ASCII text of signature.payloadDigest, with no
newline. The digest itself covers the ADR-001 canonical form of the record
after removing its top-level signature property.

## Pinned interoperability command

Use Cosign v3.1.4 and an encrypted local key. The explicit flags below disable
transparency-log upload, remote signing configuration, and the new bundle
format. Do not omit tlog-upload=false. The deprecated compatibility flags are
part of this version-pinned fixture protocol and require requalification if
the Cosign version changes.

Create the payload file as ASCII without a trailing newline:

~~~powershell
Set-Content -LiteralPath payloadDigest.txt -Value $payloadDigest -NoNewline -Encoding ascii
~~~

Sign it offline:

~~~sh
cosign sign-blob --key cosign.key --new-bundle-format=false --use-signing-config=false --tlog-upload=false --output-signature payloadDigest.sig payloadDigest.txt
~~~

The output-signature file contains standard base64 of the ASN.1 ECDSA
signature. Store that text in signature.value. Keep private keys out of the
repository and off the target host; the authenticated policy carries only
the matching public PEM and its SPKI SHA-256 key ID.

Cosign's own offline check uses the same local key and explicitly skips
transparency-log proof:

~~~sh
cosign verify-blob --key cosign.pub --signature payloadDigest.sig --insecure-ignore-tlog payloadDigest.txt
~~~

This profile relies on the policy-pinned key for signer trust. It does not
provide a public transparency proof or protect against compromise of the
operator that provisions policy. The test-only fixture in testdata was
generated and accepted by these commands; its private fixture key is not
checked in. TestCosignV314InteroperabilityFixture proves the Go verifier
accepts the same signature bytes.

## Verification rules

- Input must be one valid UTF-8 JSON value with no duplicate keys, unpaired
  surrogate escapes, or trailing values.
- The full record must already be in canonical JSON form. The Go encoder
  matches the repository vectors, including Unicode, arbitrary-size integers
  and Python float exponent thresholds.
- The signature algorithm, signed review state, signer ID, key fingerprint,
  payload digest, base64 encoding, ASN.1 encoding and ECDSA signature are
  checked before acceptance.
- Policy keys are operator material. The record cannot provide a key, trust
  root, certificate, or identity to establish its own trust.

Run the focused checks from services with go test ./internal/verify.
