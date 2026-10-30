package contracts

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Presence/nullability parity with the JSON schema: decoding into
// value structs turned omitted fields into zero values, several of
// which are individually valid. These cases must fail at decode.
func TestDecodeHostProfilePresenceAndNullability(t *testing.T) {
	now := time.Now().UTC()
	valid, err := json.Marshal(validProfile(now))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeHostProfile(valid); err != nil {
		t.Fatalf("valid profile failed decode: %v", err)
	}

	mutate := func(t *testing.T, fn func(map[string]any)) []byte {
		t.Helper()
		var doc map[string]any
		if err := json.Unmarshal(valid, &doc); err != nil {
			t.Fatal(err)
		}
		fn(doc)
		out, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	nested := func(doc map[string]any, parent string) map[string]any {
		child, _ := doc[parent].(map[string]any)
		return child
	}

	cases := []struct {
		name string
		fn   func(map[string]any)
		want string
	}{
		{"omit ramMb", func(d map[string]any) { delete(nested(d, "resources"), "ramMb") }, "resources.ramMb is required"},
		{"omit storageFreeMb", func(d map[string]any) { delete(nested(d, "resources"), "storageFreeMb") }, "resources.storageFreeMb is required"},
		{"omit cpuCores", func(d map[string]any) { delete(nested(d, "resources"), "cpuCores") }, "resources.cpuCores is required"},
		{"null ramMb", func(d map[string]any) { nested(d, "resources")["ramMb"] = nil }, "must not be null"},
		{"omit secretValuesPresent", func(d map[string]any) { delete(nested(d, "redaction"), "secretValuesPresent") }, "redaction.secretValuesPresent is required"},
		{"null secretValuesPresent", func(d map[string]any) { nested(d, "redaction")["secretValuesPresent"] = nil }, "must not be null"},
		{"omit applied", func(d map[string]any) { delete(nested(d, "redaction"), "applied") }, "redaction.applied is required"},
		{"null network", func(d map[string]any) { d["network"] = nil }, "network must not be null"},
		{"null resources", func(d map[string]any) { d["resources"] = nil }, "resources must not be null"},
		{"null redaction", func(d map[string]any) { d["redaction"] = nil }, "redaction must not be null"},
		{"null docker", func(d map[string]any) { d["docker"] = nil }, "docker must not be null"},
		{"null occupiedPorts", func(d map[string]any) { d["occupiedPorts"] = nil }, "occupiedPorts must not be null"},
		{"null filesystem", func(d map[string]any) { d["filesystem"] = nil }, "filesystem must not be null"},
		{"omit os.family", func(d map[string]any) { delete(nested(d, "os"), "family") }, "os.family is required"},
		{"omit expiresAt", func(d map[string]any) { delete(d, "expiresAt") }, "expiresAt is required"},
		{"omit tenantScope", func(d map[string]any) { delete(d, "tenantScope") }, "tenantScope is required"},
		{"omit docker.engineVersion", func(d map[string]any) { delete(nested(d, "docker"), "engineVersion") }, "docker.engineVersion is required"},
		{"omit filesystem.type when present", func(d map[string]any) { delete(nested(d, "filesystem"), "type") }, "filesystem.type is required"},
		{"null os.version", func(d map[string]any) { nested(d, "os")["version"] = nil }, "must not be null"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeHostProfile(mutate(t, tc.fn))
			if err == nil {
				t.Fatalf("%s: accepted, want decode error", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%s: error %q should mention %q", tc.name, err, tc.want)
			}
		})
	}

	for name, raw := range map[string]string{
		"top-level null":  `null`,
		"top-level array": `[]`,
		"empty body":      ``,
		"trailing data":   string(valid) + ` {}`,
	} {
		if _, err := DecodeHostProfile([]byte(raw)); err == nil {
			t.Errorf("%s: accepted, want error", name)
		}
	}
}

// Case aliases must never reach struct decoding: encoding/json
// matches fields case-insensitively, so a raw allowlist is the only
// reliable gate. Duplicate keys are rejected at every object too.
func TestDecodeHostProfileRejectsAliasesAndDuplicates(t *testing.T) {
	now := time.Now().UTC()
	valid, err := json.Marshal(validProfile(now))
	if err != nil {
		t.Fatal(err)
	}
	raw := string(valid)
	cases := []struct {
		name string
		body string
		want string
	}{
		{"top-level Redaction alias", strings.Replace(raw, `"redaction":`, `"Redaction":{"secretValuesPresent":null},"redaction":`, 1), `"Redaction" is not allowed`},
		{"top-level OS alias", strings.Replace(raw, `"os":`, `"OS":{},"os":`, 1), `"OS" is not allowed`},
		{"nested SecretValuesPresent alias", strings.Replace(raw, `"secretValuesPresent":false`, `"secretValuesPresent":false,"SecretValuesPresent":null`, 1), `"SecretValuesPresent" is not allowed`},
		{"nested RAMMB alias", strings.Replace(raw, `"ramMb":8192`, `"ramMb":8192,"RAMMB":1`, 1), `"RAMMB" is not allowed`},
		{"duplicate top-level key", strings.Replace(raw, `"hostId":"host_demo01"`, `"hostId":"host_demo01","hostId":"host_demo01"`, 1), "duplicate property"},
		{"duplicate nested key", strings.Replace(raw, `"ramMb":8192`, `"ramMb":8192,"ramMb":8192`, 1), "duplicate property"},
		{"unknown nested key", strings.Replace(raw, `"ramMb":8192`, `"ramMb":8192,"bogus":1`, 1), `"bogus" is not allowed`},
		{"explicit empty mode", strings.Replace(raw, `"mode":"byo-vps"`, `"mode":""`, 1), "network.mode"},
		{"explicit invalid mode", strings.Replace(raw, `"mode":"byo-vps"`, `"mode":"public"`, 1), "network.mode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeHostProfile([]byte(tc.body))
			if err == nil {
				t.Fatalf("%s: accepted, want error", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%s: error %q should mention %q", tc.name, err, tc.want)
			}
		})
	}
}

// Omitted optional fields must stay valid and serialise back
// without schema-invalid nulls or empty enum strings.
func TestDecodeHostProfileOptionalRoundTrip(t *testing.T) {
	now := time.Now().UTC()
	valid, err := json.Marshal(validProfile(now))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(valid, &doc); err != nil {
		t.Fatal(err)
	}
	delete(doc, "occupiedPorts")
	doc["network"] = map[string]any{}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := DecodeHostProfile(raw)
	if err != nil {
		t.Fatalf("profile with omitted optionals rejected: %v", err)
	}
	if profile.OccupiedPorts == nil {
		t.Error("occupiedPorts not normalised to an empty array")
	}
	out, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `"occupiedPorts":null`) {
		t.Errorf("round-trip emitted occupiedPorts null: %s", out)
	}
	if strings.Contains(string(out), `"mode":""`) {
		t.Errorf("round-trip emitted empty mode: %s", out)
	}
	// The re-serialised record must itself decode and validate.
	again, err := DecodeHostProfile(out)
	if err != nil {
		t.Fatalf("round-tripped profile failed decode: %v (%s)", err, out)
	}
	if err := ValidateHostProfile(again, now); err != nil {
		t.Fatalf("round-tripped profile failed validation: %v", err)
	}
}
