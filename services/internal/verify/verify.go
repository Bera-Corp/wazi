package verify

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

const CosignBlobAlgorithm = "sigstore-cosign"

var digestPattern = regexp.MustCompile("^sha256:[0-9a-f]{64}$")

// TrustedKey is operator-provisioned policy material. KeyID is sha256 of the
// DER SubjectPublicKeyInfo; PublicKeyPEM must contain exactly one PUBLIC KEY
// block. It is never read from the artifact being verified.
type TrustedKey struct {
	KeyID        string
	PublicKeyPEM []byte
}

// Policy is an authenticated policy snapshot supplied by the caller.
type Policy struct {
	Algorithms     []string
	TrustedSigners map[string]TrustedKey
}

// VerifyRecord verifies one canonical signed record using only key material
// from policy. The cryptographic input is the ASCII payloadDigest value,
// matching the Phase 1 profile selected in ADR-002.
func VerifyRecord(raw []byte, policy Policy) error {
	return verifySignedRecord(raw, policy, true)
}

// VerifySignedRecord verifies a canonical signature without imposing a
// top-level reviewState field. The record's own schema remains responsible
// for requiring its signature and defining any record-specific state.
func VerifySignedRecord(raw []byte, policy Policy) error {
	return verifySignedRecord(raw, policy, false)
}

func verifySignedRecord(raw []byte, policy Policy, requireReviewState bool) error {
	value, err := ParseJSON(raw)
	if err != nil {
		return fmt.Errorf("parse signed record: %w", err)
	}
	canonical, err := CanonicalJSON(value)
	if err != nil {
		return fmt.Errorf("canonicalize signed record: %w", err)
	}
	if !bytes.Equal(raw, canonical) {
		return fmt.Errorf("signed record is not in canonical JSON form")
	}
	record, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("signed record must be a JSON object")
	}
	reviewStateValue, hasReviewState := record["reviewState"]
	if requireReviewState && !hasReviewState {
		return fmt.Errorf("record reviewState must be signed, canary, or stable")
	}
	if hasReviewState {
		reviewState, ok := reviewStateValue.(string)
		if !ok || !isSignedReviewState(reviewState) {
			return fmt.Errorf("record reviewState must be signed, canary, or stable")
		}
	}
	signature, ok := record["signature"].(map[string]any)
	if !ok {
		return fmt.Errorf("record signature must be an object")
	}
	if err := validateSignatureFields(signature); err != nil {
		return err
	}

	algorithm := signature["algorithm"].(string)
	if !contains(policy.Algorithms, algorithm) {
		return fmt.Errorf("signature algorithm %q is not allowed by policy", algorithm)
	}
	if algorithm != CosignBlobAlgorithm {
		return fmt.Errorf("signature algorithm %q is not implemented", algorithm)
	}
	signerID := signature["signedBy"].(string)
	trusted, exists := policy.TrustedSigners[signerID]
	if !exists {
		return fmt.Errorf("signer %q is not trusted by policy", signerID)
	}
	publicKey, keyID, err := parseTrustedKey(trusted.PublicKeyPEM)
	if err != nil {
		return fmt.Errorf("trusted signer %q has invalid key material: %w", signerID, err)
	}
	if trusted.KeyID == "" || trusted.KeyID != keyID {
		return fmt.Errorf("trusted signer %q key ID does not match its public key", signerID)
	}

	payloadDigest, err := PayloadDigest(raw)
	if err != nil {
		return fmt.Errorf("compute signed payload digest: %w", err)
	}
	declaredDigest := signature["payloadDigest"].(string)
	if !digestPattern.MatchString(declaredDigest) {
		return fmt.Errorf("signature payloadDigest must be a lowercase sha256 digest")
	}
	if declaredDigest != payloadDigest {
		return fmt.Errorf("signature payloadDigest does not match signed record content")
	}

	encoded, ok := signature["value"].(string)
	if !ok {
		return fmt.Errorf("signature value must be a base64 string")
	}
	signatureBytes, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || base64.StdEncoding.EncodeToString(signatureBytes) != encoded {
		return fmt.Errorf("signature value must be canonical standard base64")
	}
	if err := verifyECDSASignature(publicKey, []byte(declaredDigest), signatureBytes); err != nil {
		return err
	}
	return nil
}

func validateSignatureFields(signature map[string]any) error {
	allowed := map[string]bool{
		"algorithm": true, "value": true, "signedBy": true,
		"payloadDigest": true, "signedAt": true,
	}
	for name := range signature {
		if !allowed[name] {
			return fmt.Errorf("signature property %q is not allowed", name)
		}
	}
	for _, name := range []string{"algorithm", "value", "signedBy", "payloadDigest"} {
		if _, exists := signature[name]; !exists {
			return fmt.Errorf("signature.%s is required", name)
		}
	}
	for _, name := range []string{"algorithm", "value", "signedBy", "payloadDigest"} {
		if _, ok := signature[name].(string); !ok {
			return fmt.Errorf("signature.%s must be a string", name)
		}
	}
	if signedAt, exists := signature["signedAt"]; exists {
		if _, ok := signedAt.(string); !ok {
			return fmt.Errorf("signature.signedAt must be a string")
		}
	}
	return nil
}

func parseTrustedKey(publicPEM []byte) (*ecdsa.PublicKey, string, error) {
	block, rest := pem.Decode(publicPEM)
	if block == nil || block.Type != "PUBLIC KEY" || strings.TrimSpace(string(rest)) != "" {
		return nil, "", fmt.Errorf("expected exactly one PUBLIC KEY PEM block")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, "", err
	}
	publicKey, ok := parsed.(*ecdsa.PublicKey)
	if !ok || publicKey.Curve.Params().Name != elliptic.P256().Params().Name {
		return nil, "", fmt.Errorf("key must be ECDSA P-256")
	}
	spki, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil, "", err
	}
	keyDigest := sha256.Sum256(spki)
	return publicKey, "sha256:" + hex.EncodeToString(keyDigest[:]), nil
}

func verifyECDSASignature(publicKey *ecdsa.PublicKey, message, signature []byte) error {
	var parsed struct {
		R *big.Int
		S *big.Int
	}
	rest, err := asn1.Unmarshal(signature, &parsed)
	if err != nil || len(rest) != 0 || parsed.R == nil || parsed.S == nil {
		return fmt.Errorf("signature value is not a complete ASN.1 ECDSA signature")
	}
	if parsed.R.Sign() <= 0 || parsed.S.Sign() <= 0 ||
		parsed.R.Cmp(publicKey.Params().N) >= 0 || parsed.S.Cmp(publicKey.Params().N) >= 0 {
		return fmt.Errorf("signature values are outside the ECDSA curve range")
	}
	canonical, err := asn1.Marshal(parsed)
	if err != nil || !bytes.Equal(canonical, signature) {
		return fmt.Errorf("signature value is not canonical ASN.1")
	}
	digest := sha256.Sum256(message)
	if !ecdsa.VerifyASN1(publicKey, digest[:], signature) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func isSignedReviewState(value string) bool {
	switch value {
	case "signed", "canary", "stable":
		return true
	default:
		return false
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
