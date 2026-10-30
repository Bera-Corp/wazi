package verify

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalizationVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../schemas/canonicalization-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	document, err := ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	root := document.(map[string]any)
	vectors := root["vectors"].([]any)
	for _, item := range vectors {
		vector := item.(map[string]any)
		t.Run(vector["name"].(string), func(t *testing.T) {
			canonical, err := CanonicalJSON(vector["input"])
			if err != nil {
				t.Fatal(err)
			}
			if string(canonical) != vector["canonical"].(string) {
				t.Fatalf("canonical mismatch:\n got %s\nwant %s", canonical, vector["canonical"])
			}
			digest, err := CanonicalDigest(vector["input"])
			if err != nil {
				t.Fatal(err)
			}
			if digest != vector["sha256"].(string) {
				t.Fatalf("digest mismatch: got %s want %s", digest, vector["sha256"])
			}
		})
	}
}

func TestParseJSONRejectsAmbiguousOrInvalidInput(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
	}{
		{name: "duplicate top-level key", raw: []byte("{\"a\":1,\"a\":2}")},
		{name: "duplicate nested key", raw: []byte("{\"outer\":{\"a\":1,\"a\":2}}")},
		{name: "unpaired high surrogate", raw: []byte("{\"x\":\"\\uD800\"}")},
		{name: "unpaired low surrogate", raw: []byte("{\"x\":\"\\uDC00\"}")},
		{name: "trailing value", raw: []byte("{} {}")},
		{name: "invalid UTF-8", raw: []byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseJSON(test.raw); err == nil {
				t.Fatal("ParseJSON accepted invalid or ambiguous input")
			}
		})
	}
}

func TestVerifyRecordRejectsUntrustedAndAlteredRecords(t *testing.T) {
	raw, policy, otherPolicy := signedTestRecord(t)
	if err := VerifyRecord(raw, policy); err != nil {
		t.Fatalf("valid test signature rejected: %v", err)
	}
	if err := VerifyRecord(raw, otherPolicy); err == nil {
		t.Fatal("signature verified with the wrong key")
	}
	if err := VerifyRecord(append(append([]byte(nil), raw...), ' '), policy); err == nil {
		t.Fatal("non-canonical record accepted")
	}

	alteredPayload := mustRecord(t, raw)
	alteredPayload["appId"] = "org.example.changed"
	alteredRaw := mustCanonical(t, alteredPayload)
	if err := VerifyRecord(alteredRaw, policy); err == nil {
		t.Fatal("altered payload accepted")
	}

	alteredSignature := mustRecord(t, raw)
	signature := alteredSignature["signature"].(map[string]any)
	encoded := signature["value"].(string)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	decoded[len(decoded)-1] ^= 1
	signature["value"] = base64.StdEncoding.EncodeToString(decoded)
	if err := VerifyRecord(mustCanonical(t, alteredSignature), policy); err == nil {
		t.Fatal("altered signature accepted")
	}

	untrustedSigner := mustRecord(t, raw)
	untrustedSigner["signature"].(map[string]any)["signedBy"] = "unknown-signer"
	if err := VerifyRecord(mustCanonical(t, untrustedSigner), policy); err == nil {
		t.Fatal("untrusted signer accepted")
	}

	disallowedAlgorithm := mustRecord(t, raw)
	disallowedAlgorithm["signature"].(map[string]any)["algorithm"] = "minisign"
	if err := VerifyRecord(mustCanonical(t, disallowedAlgorithm), policy); err == nil {
		t.Fatal("unimplemented signature algorithm accepted")
	}
}

func signedTestRecord(t *testing.T) ([]byte, Policy, Policy) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	unsigned := map[string]any{
		"appId":         "org.example.verifier-test",
		"reviewState":   "signed",
		"schemaVersion": "1.0.0",
	}
	payloadDigest, err := CanonicalDigest(unsigned)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(payloadDigest))
	signatureBytes, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	record := map[string]any{
		"appId":         unsigned["appId"],
		"reviewState":   unsigned["reviewState"],
		"schemaVersion": unsigned["schemaVersion"],
		"signature": map[string]any{
			"algorithm":     CosignBlobAlgorithm,
			"payloadDigest": payloadDigest,
			"signedBy":      "unit-test-signer",
			"value":         base64.StdEncoding.EncodeToString(signatureBytes),
		},
	}
	raw, err := CanonicalJSON(record)
	if err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	keyDigest := sha256.Sum256(publicDER)
	trustedKey := TrustedKey{
		KeyID:        "sha256:" + hex.EncodeToString(keyDigest[:]),
		PublicKeyPEM: publicPEM,
	}
	policy := Policy{
		Algorithms:     []string{CosignBlobAlgorithm},
		TrustedSigners: map[string]TrustedKey{"unit-test-signer": trustedKey},
	}
	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherDER, err := x509.MarshalPKIXPublicKey(&otherKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	otherPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: otherDER})
	otherDigest := sha256.Sum256(otherDER)
	otherPolicy := Policy{
		Algorithms: []string{CosignBlobAlgorithm},
		TrustedSigners: map[string]TrustedKey{
			"unit-test-signer": {
				KeyID:        "sha256:" + hex.EncodeToString(otherDigest[:]),
				PublicKeyPEM: otherPEM,
			},
		},
	}
	return raw, policy, otherPolicy
}

func mustRecord(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	value, err := ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value.(map[string]any)
}

func mustCanonical(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := CanonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestPayloadDigestOmitsOnlyTopLevelSignature(t *testing.T) {
	raw, _, _ := signedTestRecord(t)
	got, err := PayloadDigest(raw)
	if err != nil {
		t.Fatal(err)
	}
	record := mustRecord(t, raw)
	delete(record, "signature")
	want, err := CanonicalDigest(record)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("payload digest mismatch: got %s want %s", got, want)
	}
	if bytes.Contains([]byte(got), []byte("signature")) {
		t.Fatal("digest unexpectedly contains the signature field")
	}
}

func TestCanonicalJSONRejectsUnsupportedValues(t *testing.T) {
	if _, err := CanonicalJSON(struct{ A int }{A: 1}); err == nil {
		t.Fatal("CanonicalJSON accepted a non-JSON Go struct")
	}
}

func TestVectorDocumentIsValidJSON(t *testing.T) {
	raw, err := os.ReadFile("../../../schemas/canonicalization-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseJSON(raw); err != nil {
		t.Fatal(err)
	}
}

func TestCosignV314InteroperabilityFixture(t *testing.T) {
	record, err := os.ReadFile("testdata/cosign-v3.1.4-record.json")
	if err != nil {
		t.Fatal(err)
	}
	publicPEM, err := os.ReadFile("testdata/cosign-v3.1.4-public.pem")
	if err != nil {
		t.Fatal(err)
	}
	_, keyID, err := parseTrustedKey(publicPEM)
	if err != nil {
		t.Fatal(err)
	}
	policy := Policy{
		Algorithms: []string{CosignBlobAlgorithm},
		TrustedSigners: map[string]TrustedKey{
			"cosign-v3.1.4-fixture": {
				KeyID:        keyID,
				PublicKeyPEM: publicPEM,
			},
		},
	}
	if err := VerifyRecord(record, policy); err != nil {
		t.Fatalf("Go verifier rejected the pinned Cosign v3.1.4 fixture: %v", err)
	}
}

func TestCosignV314AppManifestFixture(t *testing.T) {
	record, err := os.ReadFile("testdata/cosign-v3.1.4-app-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	publicPEM, err := os.ReadFile("testdata/cosign-v3.1.4-public.pem")
	if err != nil {
		t.Fatal(err)
	}
	_, keyID, err := parseTrustedKey(publicPEM)
	if err != nil {
		t.Fatal(err)
	}
	policy := Policy{
		Algorithms: []string{CosignBlobAlgorithm},
		TrustedSigners: map[string]TrustedKey{
			"cosign-v3.1.4-fixture": {
				KeyID:        keyID,
				PublicKeyPEM: publicPEM,
			},
		},
	}
	if err := VerifyRecord(record, policy); err != nil {
		t.Fatalf("Go verifier rejected the signed app-manifest fixture: %v", err)
	}
}

func TestParsePolicyValidatesVersionAndKeyBinding(t *testing.T) {
	raw, keyID, publicPEM := signingPolicyFixture(t)
	policy, err := ParsePolicy(raw)
	if err != nil {
		t.Fatalf("valid signing policy rejected: %v", err)
	}
	if policy.TrustedSigners["fixture-signer"].KeyID != keyID {
		t.Fatal("loaded policy lost the trusted key ID")
	}
	if !bytes.Equal(policy.TrustedSigners["fixture-signer"].PublicKeyPEM, publicPEM) {
		t.Fatal("loaded policy changed the public key bytes")
	}
}

func TestLoadPolicyFile(t *testing.T) {
	raw, keyID, _ := signingPolicyFixture(t)
	path := filepath.Join(t.TempDir(), "signing-policy.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	policy, err := LoadPolicyFile(path)
	if err != nil {
		t.Fatalf("valid policy file rejected: %v", err)
	}
	if policy.TrustedSigners["fixture-signer"].KeyID != keyID {
		t.Fatal("policy file lost the trusted signer mapping")
	}
}

func TestParsePolicyRejectsDuplicateAndUnsupportedTrustData(t *testing.T) {
	raw, _, _ := signingPolicyFixture(t)
	value, err := ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	root := value.(map[string]any)

	wrongKeyID := mustPolicyCopy(root)
	signer := wrongKeyID["trustedSigners"].(map[string]any)["fixture-signer"].(map[string]any)
	signer["keyId"] = "sha256:" + strings.Repeat("0", 64)
	if _, err := ParsePolicy(mustJSON(t, wrongKeyID)); err == nil {
		t.Fatal("policy with mismatched key fingerprint accepted")
	}

	unsupported := mustPolicyCopy(root)
	unsupported["algorithms"] = []any{"openpgp"}
	if _, err := ParsePolicy(mustJSON(t, unsupported)); err == nil {
		t.Fatal("policy with an unimplemented algorithm accepted")
	}

	duplicateKey := mustPolicyCopy(root)
	signers := duplicateKey["trustedSigners"].(map[string]any)
	signers["alias-signer"] = signers["fixture-signer"]
	if _, err := ParsePolicy(mustJSON(t, duplicateKey)); err == nil {
		t.Fatal("same key assigned to multiple signer IDs accepted")
	}

	duplicateProperty := []byte("{\"schemaVersion\":\"1.0.0\",\"schemaVersion\":\"1.0.0\",\"algorithms\":[\"sigstore-cosign\"],\"trustedSigners\":{\"fixture-signer\":{}}}")
	if _, err := ParsePolicy(duplicateProperty); err == nil {
		t.Fatal("duplicate policy property accepted")
	}
}

func signingPolicyFixture(t *testing.T) ([]byte, string, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	keyHash := sha256.Sum256(der)
	keyID := "sha256:" + hex.EncodeToString(keyHash[:])
	policy := map[string]any{
		"schemaVersion": SigningPolicySchemaVersion,
		"algorithms":    []string{CosignBlobAlgorithm},
		"trustedSigners": map[string]any{
			"fixture-signer": map[string]any{
				"keyId":        keyID,
				"publicKeyPem": string(publicPEM),
			},
		},
	}
	return mustJSON(t, policy), keyID, publicPEM
}

func mustPolicyCopy(value map[string]any) map[string]any {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var copy map[string]any
	if err := json.Unmarshal(data, &copy); err != nil {
		panic(err)
	}
	return copy
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
