package agentclient

import "testing"

func TestAuthenticatedLicenseRulesPreserveExplicitEmptyAllowlistSemantics(t *testing.T) {
	policy := VerifiedCapabilityPolicy{
		snapshot:     AuthenticatedCapabilityPolicy{BundleID: "pol_test", Version: "1.0.0", Digest: "sha256:test", TenantScope: "tenant_test", AllowedCapabilities: []string{"manifest.verify"}},
		requirements: map[string]string{"install": "user"},
	}
	for _, test := range []struct {
		name, license string
		allowed       []string
		valid         bool
	}{
		{"allowed", "PostgreSQL", []string{"MIT", "PostgreSQL"}, true},
		{"denied", "GPL-2.0-only", []string{"MIT"}, false},
		{"empty-allows-explicit", "GPL-2.0-only", []string{}, true},
		{"empty-denies-missing", "", []string{}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var err error
			policy.raw, err = canonicalPayload(map[string]any{"licensing": map[string]any{"requireExplicitLicense": true, "allowedLicenses": test.allowed}})
			if err != nil {
				t.Fatal(err)
			}
			err = policy.ValidateManifestLicense(canonical(t, map[string]any{"license": test.license}))
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, error=%v", test.valid, err)
			}
		})
	}
}
