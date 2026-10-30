package controlplane

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"runtime"

	"github.com/Bera-Corp/wazi/services/internal/verify"
)

const maxApproverCredentialFileBytes = 64 << 10

var (
	approverIDPattern     = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")
	approverDigestPattern = regexp.MustCompile("^sha256:[a-f0-9]{64}$")
)

// ApproverCredential maps one separately provisioned high-entropy credential
// digest to a named approval principal. It does not prove who controls that
// credential; deployment policy must keep credentials under distinct custody.
type ApproverCredential struct {
	ID               string
	Role             string
	CredentialSHA256 string
}

// LoadApproverCredentials reads a strict, private configuration file. The
// file stores credential digests, never the credentials themselves.
func LoadApproverCredentials(path string) ([]ApproverCredential, error) {
	if path == "" {
		return nil, fmt.Errorf("approver credential file path is required")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect approver credential file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("approver credential path must be a regular non-symlink file")
	}
	if info.Size() <= 0 || info.Size() > maxApproverCredentialFileBytes {
		return nil, fmt.Errorf("approver credential file size is invalid")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("approver credential file must not be accessible by group or other users")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read approver credential file: %w", err)
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("parse approver credential file: %w", err)
	}
	canonical, err := verify.CanonicalJSON(value)
	if err != nil || !bytes.Equal(raw, canonical) {
		return nil, fmt.Errorf("approver credential file must be canonical JSON")
	}
	root, ok := value.(map[string]any)
	if !ok || len(root) != 2 {
		return nil, fmt.Errorf("approver credential file has an invalid shape")
	}
	version, versionOK := root["schemaVersion"].(string)
	if !versionOK || version != "1.0.0" {
		return nil, fmt.Errorf("approver credential file schemaVersion must be 1.0.0")
	}
	items, ok := root["approvers"].([]any)
	if !ok {
		return nil, fmt.Errorf("approver credential file approvers must be an array")
	}
	credentials := make([]ApproverCredential, 0, len(items))
	for index, item := range items {
		fields, ok := item.(map[string]any)
		if !ok || len(fields) != 3 {
			return nil, fmt.Errorf("approver entry %d has an invalid shape", index)
		}
		id, idOK := fields["approverId"].(string)
		role, roleOK := fields["role"].(string)
		digest, digestOK := fields["credentialSha256"].(string)
		if !idOK || !roleOK || !digestOK {
			return nil, fmt.Errorf("approver entry %d has invalid field types", index)
		}
		credentials = append(credentials, ApproverCredential{
			ID: id, Role: role, CredentialSHA256: digest,
		})
	}
	if err := ValidateApproverCredentials(credentials); err != nil {
		return nil, err
	}
	return credentials, nil
}

// ValidateApproverCredentials accepts a pilot with one configured approver or
// a larger principal set. Credentials and IDs must be unique. A policy that
// requires two maintainers still fails closed unless two distinct configured
// maintainer credentials are available and used.
func ValidateApproverCredentials(credentials []ApproverCredential) error {
	if len(credentials) < 1 || len(credentials) > 32 {
		return fmt.Errorf("approver credentials must contain between 1 and 32 principals")
	}
	ids := make(map[string]struct{}, len(credentials))
	digests := make(map[string]struct{}, len(credentials))
	for _, credential := range credentials {
		if !approverIDPattern.MatchString(credential.ID) {
			return fmt.Errorf("approver ID is invalid")
		}
		if credential.Role != "user" && credential.Role != "maintainer" {
			return fmt.Errorf("approver role must be user or maintainer")
		}
		if !approverDigestPattern.MatchString(credential.CredentialSHA256) {
			return fmt.Errorf("approver credential digest is invalid")
		}
		if _, exists := ids[credential.ID]; exists {
			return fmt.Errorf("approver IDs must be unique")
		}
		if _, exists := digests[credential.CredentialSHA256]; exists {
			return fmt.Errorf("approver credentials must be unique")
		}
		ids[credential.ID] = struct{}{}
		digests[credential.CredentialSHA256] = struct{}{}
	}
	return nil
}

func matchApproverCredential(credentials []ApproverCredential, supplied string) (ApproverCredential, bool) {
	if len(supplied) < 32 || len(supplied) > 1024 {
		return ApproverCredential{}, false
	}
	digest := sha256.Sum256([]byte(supplied))
	candidate := "sha256:" + hex.EncodeToString(digest[:])
	for _, credential := range credentials {
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(credential.CredentialSHA256)) == 1 {
			return credential, true
		}
	}
	return ApproverCredential{}, false
}
