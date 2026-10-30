package agentclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

// ReleaseExpectation is the identity requested by the caller before it may
// consider a signed manifest and procedure for a typed operation.
type ReleaseExpectation struct {
	ApplicationID    string
	ReleaseVersion   string
	ProcedureID      string
	ProcedureVersion string
	Operation        string
}

// VerifiedRelease contains canonical digests only after both signed records
// pass the Phase 1 signature profile, full schema validation, and identity
// binding to one application release and operation.
type VerifiedRelease struct {
	ManifestDigest  string
	ProcedureDigest string
}

// VerifiedReleaseArtifacts retains defensive copies of the exact canonical
// bytes whose signatures, schemas, identities, and digests were verified.
// Accessors return fresh copies so downstream execution code cannot mutate the
// verifier's retained evidence.
type VerifiedReleaseArtifacts struct {
	Release      VerifiedRelease
	manifestRaw  []byte
	procedureRaw []byte
}

func (r VerifiedReleaseArtifacts) Manifest() []byte {
	return append([]byte(nil), r.manifestRaw...)
}

func (r VerifiedReleaseArtifacts) Procedure() []byte {
	return append([]byte(nil), r.procedureRaw...)
}

const maxFetchedRecordBytes = 1 << 20

// FetchVerifiedRelease retrieves one manifest and procedure over the
// configured HTTPS/mTLS client, validates each response digest against its
// ETag, then independently verifies both signatures, schemas, and identities.
func FetchVerifiedRelease(ctx context.Context, baseURL string, tlsConfig *tls.Config, policy verify.Policy, expected ReleaseExpectation) (VerifiedRelease, error) {
	artifacts, err := FetchVerifiedReleaseArtifacts(ctx, baseURL, tlsConfig, policy, expected)
	if err != nil {
		return VerifiedRelease{}, err
	}
	return artifacts.Release, nil
}

// FetchVerifiedReleaseArtifacts returns the exact records verified by the
// existing release preflight. This avoids a verify-then-refetch gap for an
// executor that must bind the same bytes into its plan and journal.
func FetchVerifiedReleaseArtifacts(ctx context.Context, baseURL string, tlsConfig *tls.Config, policy verify.Policy, expected ReleaseExpectation) (VerifiedReleaseArtifacts, error) {
	var empty VerifiedReleaseArtifacts
	if err := validateReleaseExpectation(expected); err != nil {
		return empty, err
	}
	if err := requirePairedTLS(tlsConfig, "release fetch"); err != nil {
		return empty, err
	}
	base, err := secureBaseURL(baseURL)
	if err != nil {
		return empty, err
	}
	manifestPath := "/v1/manifests/" + url.PathEscape(expected.ApplicationID) + "/" + url.PathEscape(expected.ReleaseVersion)
	procedurePath := "/v1/procedures/" + url.PathEscape(expected.ProcedureID) + "/" + url.PathEscape(expected.ProcedureVersion)
	manifestRaw, err := fetchCanonicalRecord(ctx, base+manifestPath, tlsConfig)
	if err != nil {
		return empty, fmt.Errorf("fetch manifest: %w", err)
	}
	procedureRaw, err := fetchCanonicalRecord(ctx, base+procedurePath, tlsConfig)
	if err != nil {
		return empty, fmt.Errorf("fetch procedure: %w", err)
	}
	return VerifyReleaseArtifacts(manifestRaw, procedureRaw, policy, expected)
}

func fetchCanonicalRecord(ctx context.Context, endpoint string, tlsConfig *tls.Config) ([]byte, error) {
	client := newSecureClient(tlsConfig)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		return nil, fmt.Errorf("record read rejected (%s): %s", response.Status, strings.TrimSpace(string(body)))
	}
	if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		return nil, fmt.Errorf("record response content-type must be application/json")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxFetchedRecordBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read record response: %w", err)
	}
	if len(raw) == 0 || len(raw) > maxFetchedRecordBytes {
		return nil, fmt.Errorf("record response size is invalid")
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("record response is invalid JSON: %w", err)
	}
	canonical, err := verify.CanonicalJSON(value)
	if err != nil {
		return nil, fmt.Errorf("canonicalize record response: %w", err)
	}
	if !bytes.Equal(raw, canonical) {
		return nil, fmt.Errorf("record response is not canonical JSON")
	}
	digest, err := verify.CanonicalDigest(value)
	if err != nil {
		return nil, fmt.Errorf("digest record response: %w", err)
	}
	if response.Header.Get("ETag") != fmt.Sprintf("\"%s\"", digest) {
		return nil, fmt.Errorf("record response ETag does not match its canonical digest")
	}
	return raw, nil
}

// VerifyReleaseRecords is the agent-side cryptographic and contract check
// for the signed manifest/procedure pair. The policy must come from an
// authenticated operator source; artifact fields never provide trust keys.
// Revocation freshness, host qualification, approval, and runtime checks are
// separate gates and are not implied by this function.
func VerifyReleaseRecords(manifestRaw, procedureRaw []byte, policy verify.Policy, expected ReleaseExpectation) (VerifiedRelease, error) {
	artifacts, err := VerifyReleaseArtifacts(manifestRaw, procedureRaw, policy, expected)
	if err != nil {
		return VerifiedRelease{}, err
	}
	return artifacts.Release, nil
}

// VerifyReleaseArtifacts validates and retains defensive copies of one exact
// manifest/procedure pair for downstream binding and evidence generation.
func VerifyReleaseArtifacts(manifestRaw, procedureRaw []byte, policy verify.Policy, expected ReleaseExpectation) (VerifiedReleaseArtifacts, error) {
	var empty VerifiedReleaseArtifacts
	release, err := verifyReleaseRecords(manifestRaw, procedureRaw, policy, expected)
	if err != nil {
		return empty, err
	}
	return VerifiedReleaseArtifacts{
		Release:      release,
		manifestRaw:  append([]byte(nil), manifestRaw...),
		procedureRaw: append([]byte(nil), procedureRaw...),
	}, nil
}

func verifyReleaseRecords(manifestRaw, procedureRaw []byte, policy verify.Policy, expected ReleaseExpectation) (VerifiedRelease, error) {
	var result VerifiedRelease
	if err := validateReleaseExpectation(expected); err != nil {
		return result, err
	}
	if err := verify.ValidatePolicy(policy); err != nil {
		return result, fmt.Errorf("signing policy is invalid: %w", err)
	}
	if err := verify.VerifyRecord(manifestRaw, policy); err != nil {
		return result, fmt.Errorf("manifest signature rejected: %w", err)
	}
	if err := contracts.ValidateAppManifest(manifestRaw); err != nil {
		return result, fmt.Errorf("manifest schema rejected: %w", err)
	}
	if err := verify.VerifyRecord(procedureRaw, policy); err != nil {
		return result, fmt.Errorf("procedure signature rejected: %w", err)
	}
	if err := contracts.ValidateProcedure(procedureRaw); err != nil {
		return result, fmt.Errorf("procedure schema rejected: %w", err)
	}

	manifest, err := verify.ParseJSON(manifestRaw)
	if err != nil {
		return result, fmt.Errorf("parse verified manifest: %w", err)
	}
	procedure, err := verify.ParseJSON(procedureRaw)
	if err != nil {
		return result, fmt.Errorf("parse verified procedure: %w", err)
	}
	manifestRecord := manifest.(map[string]any)
	procedureRecord := procedure.(map[string]any)
	if manifestRecord["appId"] != expected.ApplicationID ||
		manifestRecord["version"] != expected.ReleaseVersion {
		return result, fmt.Errorf("manifest identity does not match requested application release")
	}
	if procedureRecord["procedureId"] != expected.ProcedureID ||
		procedureRecord["version"] != expected.ProcedureVersion ||
		procedureRecord["applicationId"] != expected.ApplicationID ||
		procedureRecord["operation"] != expected.Operation {
		return result, fmt.Errorf("procedure identity does not match requested application operation")
	}
	result.ManifestDigest, err = verify.CanonicalDigest(manifestRecord)
	if err != nil {
		return VerifiedRelease{}, fmt.Errorf("digest verified manifest: %w", err)
	}
	result.ProcedureDigest, err = verify.CanonicalDigest(procedureRecord)
	if err != nil {
		return VerifiedRelease{}, fmt.Errorf("digest verified procedure: %w", err)
	}
	return result, nil
}

func validateReleaseExpectation(expected ReleaseExpectation) error {
	if expected.ApplicationID == "" ||
		expected.ReleaseVersion == "" ||
		expected.ProcedureID == "" ||
		expected.ProcedureVersion == "" ||
		expected.Operation == "" {
		return fmt.Errorf("release expectation must identify app, release, procedure, and operation")
	}
	return nil
}
