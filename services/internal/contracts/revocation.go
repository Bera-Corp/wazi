package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/Bera-Corp/wazi/services/internal/verify"
)

type RevocationRecord struct {
	SchemaVersion string            "json:\"schemaVersion\""
	ListID        string            "json:\"listId\""
	Sequence      int64             "json:\"sequence\""
	GeneratedAt   string            "json:\"generatedAt\""
	ExpiresAt     string            "json:\"expiresAt\""
	Entries       []RevocationEntry "json:\"entries\""
	Signature     json.RawMessage   "json:\"signature\""
}

type RevocationEntry struct {
	ApplicationID  string "json:\"applicationId\""
	ReleaseVersion string "json:\"releaseVersion\""
	ManifestDigest string "json:\"manifestDigest\""
	RevokedAt      string "json:\"revokedAt\""
	Reason         string "json:\"reason\""
}

// DecodeRevocationRecord applies the full embedded schema before decoding
// into bounded, typed fields. Duplicate keys are rejected by ParseJSON.
func DecodeRevocationRecord(raw []byte) (RevocationRecord, error) {
	var record RevocationRecord
	if err := ValidateRevocationRecord(raw); err != nil {
		return record, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return RevocationRecord{}, fmt.Errorf("decode revocation record: %w", err)
	}
	return record, nil
}

func RevocationRecordDigest(raw []byte) (string, error) {
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return "", err
	}
	return verify.CanonicalDigest(value)
}
