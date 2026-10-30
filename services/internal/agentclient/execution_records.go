package agentclient

import (
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

type ProcedureExpectation struct {
	ProcedureID   string
	Version       string
	ApplicationID string
	Operation     string
}

// FetchVerifiedProcedureRecord returns the exact canonical procedure bytes
// whose paired-TLS response digest, signature, schema, and identity were
// checked.
func FetchVerifiedProcedureRecord(
	ctx context.Context,
	baseURL string,
	tlsConfig *tls.Config,
	policy verify.Policy,
	expected ProcedureExpectation,
) ([]byte, error) {
	if expected.ProcedureID == "" || expected.Version == "" ||
		expected.ApplicationID == "" || expected.Operation == "" {
		return nil, fmt.Errorf("procedure expectation must identify procedure, version, application, and operation")
	}
	if err := requirePairedTLS(tlsConfig, "procedure fetch"); err != nil {
		return nil, err
	}
	base, err := secureBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	endpoint := base + "/v1/procedures/" +
		url.PathEscape(expected.ProcedureID) + "/" + url.PathEscape(expected.Version)
	raw, err := fetchCanonicalRecord(ctx, endpoint, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("fetch procedure: %w", err)
	}
	if err := verify.VerifyRecord(raw, policy); err != nil {
		return nil, fmt.Errorf("procedure signature rejected: %w", err)
	}
	if err := contracts.ValidateProcedure(raw); err != nil {
		return nil, fmt.Errorf("procedure schema rejected: %w", err)
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("parse verified procedure: %w", err)
	}
	record, ok := value.(map[string]any)
	if !ok || record["procedureId"] != expected.ProcedureID ||
		record["version"] != expected.Version ||
		record["applicationId"] != expected.ApplicationID ||
		record["operation"] != expected.Operation {
		return nil, fmt.Errorf("procedure identity does not match the requested operation")
	}
	return append([]byte(nil), raw...), nil
}

// FetchHostProfile returns the current paired-node profile as canonical bytes
// after selecting the exact authenticated tenant and host from the scoped
// profile listing.
func FetchHostProfile(
	ctx context.Context,
	baseURL string,
	tlsConfig *tls.Config,
	tenantScope string,
	hostID string,
) ([]byte, error) {
	if tenantScope == "" || hostID == "" {
		return nil, fmt.Errorf("host profile fetch requires tenant and host identity")
	}
	if err := requirePairedTLS(tlsConfig, "host profile fetch"); err != nil {
		return nil, err
	}
	base, err := secureBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/host-profiles", nil)
	if err != nil {
		return nil, fmt.Errorf("create host profile request: %w", err)
	}
	response, err := newSecureClient(tlsConfig).Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch host profile: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		return nil, fmt.Errorf("host profile fetch rejected (%s): %s", response.Status, strings.TrimSpace(string(body)))
	}
	if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		return nil, fmt.Errorf("host profile response content-type must be application/json")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxFetchedRecordBytes*8+1))
	if err != nil {
		return nil, fmt.Errorf("read host profile response: %w", err)
	}
	if len(raw) == 0 || len(raw) > maxFetchedRecordBytes*8 {
		return nil, fmt.Errorf("host profile response size is invalid")
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("host profile response is invalid JSON: %w", err)
	}
	profiles, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("host profile response must be an object")
	}
	profileValue, ok := profiles[hostID]
	if !ok {
		return nil, fmt.Errorf("authenticated host profile was not found")
	}
	profileRaw, err := verify.CanonicalJSON(profileValue)
	if err != nil {
		return nil, fmt.Errorf("canonicalize host profile: %w", err)
	}
	profile, err := contracts.DecodeHostProfile(profileRaw)
	if err != nil {
		return nil, fmt.Errorf("host profile rejected: %w", err)
	}
	if profile.HostID != hostID || profile.TenantScope != tenantScope {
		return nil, fmt.Errorf("host profile does not match the authenticated tenant and host")
	}
	return profileRaw, nil
}
