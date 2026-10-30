package agentclient

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
)

// VerifyHostCompatibility checks actual host facts against a verified release.
// It does not establish provenance, freshness, approval, or runtime ownership.
func VerifyHostCompatibility(manifestRaw []byte, host contracts.HostProfile) error {
	manifest, err := parseCanonicalObject(manifestRaw, "signed manifest")
	if err != nil {
		return err
	}
	return verifyHostCompatibility(manifest, host)
}

func verifyHostCompatibility(manifest map[string]any, host contracts.HostProfile) error {
	requirements, ok := objectValue(manifest["requirements"])
	if !ok {
		return fmt.Errorf("manifest requirements missing")
	}
	contains := func(raw any, value string) bool {
		values, ok := raw.([]any)
		if !ok {
			return false
		}
		for _, candidate := range values {
			if candidate == value {
				return true
			}
		}
		return false
	}
	if !contains(requirements["architectures"], host.Architecture) || !contains(requirements["osFamilies"], host.OS.Family) {
		return fmt.Errorf("host architecture or OS is outside the signed supported matrix")
	}
	if requirements["privileged"] == true {
		return fmt.Errorf("privileged execution is forbidden in the pilot")
	}
	for _, field := range []struct{ name, available string }{
		{"minCpuCores", strconv.FormatFloat(host.Resources.CPUCores, 'f', -1, 64)},
		{"minRamMb", strconv.FormatInt(host.Resources.RAMMb, 10)},
		{"minStorageMb", strconv.FormatInt(host.Resources.StorageFreeMb, 10)},
	} {
		minimum, ok := requirements[field.name].(json.Number)
		if !ok {
			return fmt.Errorf("missing numeric requirement %s", field.name)
		}
		if err := requireResourceLimit(minimum, field.available, field.name); err != nil {
			return err
		}
	}
	for _, version := range []struct {
		minimum any
		actual  string
	}{
		{requirements["minDockerVersion"], host.Docker.EngineVersion},
		{requirements["minComposeVersion"], host.Docker.ComposeVersion},
	} {
		minimum, ok := version.minimum.(string)
		if !ok || !stableVersionAtLeast(version.actual, minimum) {
			return fmt.Errorf("host runtime version is outside the signed supported matrix")
		}
	}
	return nil
}

func stableVersionAtLeast(actual, minimum string) bool {
	parse := func(value string) ([3]uint64, bool) {
		var parsed [3]uint64
		parts := strings.Split(value, ".")
		if len(parts) != 3 {
			return parsed, false
		}
		for i, part := range parts {
			if part == "" {
				return parsed, false
			}
			for _, c := range part {
				if c < '0' || c > '9' {
					return parsed, false
				}
			}
			value, err := strconv.ParseUint(part, 10, 64)
			if err != nil {
				return parsed, false
			}
			parsed[i] = value
		}
		return parsed, true
	}
	a, okA := parse(actual)
	b, okB := parse(minimum)
	if !okA || !okB {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return true
}
