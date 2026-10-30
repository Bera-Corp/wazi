package verify

import (
	"fmt"
	"os"
	"regexp"
)

const (
	SigningPolicySchemaVersion = "1.0.0"
	MaxSigningPolicyBytes      = 256 * 1024
)

var signerIDPattern = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")

// LoadPolicyFile reads one operator-managed signing policy. Symlinks and
// oversized or malformed files are rejected; the policy file contains only
// public verification material.
func LoadPolicyFile(path string) (Policy, error) {
	var empty Policy
	info, err := os.Lstat(path)
	if err != nil {
		return empty, fmt.Errorf("stat signing policy: %w", err)
	}
	if !info.Mode().IsRegular() {
		return empty, fmt.Errorf("signing policy must be a regular file, not a symlink or special file")
	}
	if info.Size() <= 0 || info.Size() > MaxSigningPolicyBytes {
		return empty, fmt.Errorf("signing policy size must be between 1 and %d bytes", MaxSigningPolicyBytes)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return empty, fmt.Errorf("read signing policy: %w", err)
	}
	return ParsePolicy(raw)
}

// ParsePolicy loads the versioned, strict JSON form of a signing policy.
// Each signer ID maps to one operator-pinned PEM key and its SPKI fingerprint.
func ParsePolicy(raw []byte) (Policy, error) {
	var empty Policy
	value, err := ParseJSON(raw)
	if err != nil {
		return empty, fmt.Errorf("parse signing policy: %w", err)
	}
	root, ok := value.(map[string]any)
	if !ok {
		return empty, fmt.Errorf("signing policy must be a JSON object")
	}
	if err := exactProperties(root, "schemaVersion", "algorithms", "trustedSigners"); err != nil {
		return empty, fmt.Errorf("signing policy: %w", err)
	}
	version, ok := root["schemaVersion"].(string)
	if !ok || version != SigningPolicySchemaVersion {
		return empty, fmt.Errorf("signing policy schemaVersion must be %s", SigningPolicySchemaVersion)
	}
	algorithmValues, ok := root["algorithms"].([]any)
	if !ok || len(algorithmValues) == 0 {
		return empty, fmt.Errorf("signing policy algorithms must be a non-empty array")
	}
	algorithms := make([]string, 0, len(algorithmValues))
	seenAlgorithms := map[string]bool{}
	for _, item := range algorithmValues {
		algorithm, ok := item.(string)
		if !ok || algorithm != CosignBlobAlgorithm {
			return empty, fmt.Errorf("signing policy contains an unsupported algorithm")
		}
		if seenAlgorithms[algorithm] {
			return empty, fmt.Errorf("signing policy contains duplicate algorithm %q", algorithm)
		}
		seenAlgorithms[algorithm] = true
		algorithms = append(algorithms, algorithm)
	}
	signerValues, ok := root["trustedSigners"].(map[string]any)
	if !ok || len(signerValues) == 0 {
		return empty, fmt.Errorf("signing policy trustedSigners must be a non-empty object")
	}
	trustedSigners := make(map[string]TrustedKey, len(signerValues))
	seenKeyIDs := map[string]string{}
	for signerID, rawKey := range signerValues {
		if !signerIDPattern.MatchString(signerID) {
			return empty, fmt.Errorf("signer ID %q is invalid", signerID)
		}
		keyFields, ok := rawKey.(map[string]any)
		if !ok {
			return empty, fmt.Errorf("signer %q entry must be an object", signerID)
		}
		if err := exactProperties(keyFields, "keyId", "publicKeyPem"); err != nil {
			return empty, fmt.Errorf("signer %q: %w", signerID, err)
		}
		keyID, ok := keyFields["keyId"].(string)
		if !ok {
			return empty, fmt.Errorf("signer %q keyId must be a string", signerID)
		}
		publicPEM, ok := keyFields["publicKeyPem"].(string)
		if !ok || publicPEM == "" {
			return empty, fmt.Errorf("signer %q publicKeyPem must be a non-empty string", signerID)
		}
		_, derivedKeyID, err := parseTrustedKey([]byte(publicPEM))
		if err != nil {
			return empty, fmt.Errorf("signer %q publicKeyPem: %w", signerID, err)
		}
		if keyID != derivedKeyID {
			return empty, fmt.Errorf("signer %q keyId does not match publicKeyPem", signerID)
		}
		if previous, duplicate := seenKeyIDs[keyID]; duplicate {
			return empty, fmt.Errorf("signer %q duplicates key material already assigned to %q", signerID, previous)
		}
		seenKeyIDs[keyID] = signerID
		trustedSigners[signerID] = TrustedKey{KeyID: keyID, PublicKeyPEM: []byte(publicPEM)}
	}
	return Policy{Algorithms: algorithms, TrustedSigners: trustedSigners}, nil
}

func exactProperties(object map[string]any, required ...string) error {
	allowed := make(map[string]bool, len(required))
	for _, name := range required {
		allowed[name] = true
		if _, exists := object[name]; !exists {
			return fmt.Errorf("property %q is required", name)
		}
	}
	for name := range object {
		if !allowed[name] {
			return fmt.Errorf("property %q is not allowed", name)
		}
	}
	return nil
}

// ValidatePolicy checks in-memory policy values before a registry begins
// serving records. Prefer ParsePolicy for operator-provided JSON.
func ValidatePolicy(policy Policy) error {
	if len(policy.Algorithms) == 0 {
		return fmt.Errorf("policy algorithms must be non-empty")
	}
	seenAlgorithms := map[string]bool{}
	for _, algorithm := range policy.Algorithms {
		if algorithm != CosignBlobAlgorithm {
			return fmt.Errorf("policy algorithm %q is not implemented", algorithm)
		}
		if seenAlgorithms[algorithm] {
			return fmt.Errorf("policy contains duplicate algorithm %q", algorithm)
		}
		seenAlgorithms[algorithm] = true
	}
	if len(policy.TrustedSigners) == 0 {
		return fmt.Errorf("policy trustedSigners must be non-empty")
	}
	seenKeyIDs := map[string]string{}
	for signerID, trusted := range policy.TrustedSigners {
		if !signerIDPattern.MatchString(signerID) {
			return fmt.Errorf("signer ID %q is invalid", signerID)
		}
		_, keyID, err := parseTrustedKey(trusted.PublicKeyPEM)
		if err != nil {
			return fmt.Errorf("signer %q public key is invalid: %w", signerID, err)
		}
		if trusted.KeyID == "" || trusted.KeyID != keyID {
			return fmt.Errorf("signer %q key ID does not match its public key", signerID)
		}
		if previous, duplicate := seenKeyIDs[keyID]; duplicate {
			return fmt.Errorf("signer %q duplicates key material already assigned to %q", signerID, previous)
		}
		seenKeyIDs[keyID] = signerID
	}
	return nil
}
