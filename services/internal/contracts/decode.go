package contracts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
)

// DecodeHostProfile parses a raw host-profile body enforcing the
// intake subset of schemas/host-profile.schema.json that value
// structs cannot express: required-field presence, nullability,
// exact property names, and uniqueness. It is not a claim of full
// schema parity; type/range/enum/freshness rules live in
// ValidateHostProfile and the canonical schema remains the source
// of truth.
//
// Raw checks must run before struct decoding because encoding/json
// matches struct fields case-insensitively: an extra
// "Redaction":{"secretValuesPresent":null} would otherwise alias
// onto the real redaction field despite DisallowUnknownFields.
// Every object is therefore checked against its exact property
// allowlist (additionalProperties: false in the schema), duplicate
// keys are rejected, and only then is the struct decoded.
var (
	topLevelProperties = []string{
		"schemaVersion", "hostId", "tenantScope", "observedAt", "expiresAt",
		"os", "architecture", "resources", "docker", "occupiedPorts",
		"filesystem", "network", "provenance", "redaction",
	}
	nestedProperties = map[string][]string{
		"os":         {"family", "distribution", "version"},
		"resources":  {"cpuCores", "ramMb", "storageFreeMb"},
		"docker":     {"engineVersion", "composeVersion"},
		"filesystem": {"type", "dockerRootWritable"},
		"network":    {"mode", "publicExposureCapable"},
		"provenance": {"sourceType", "extractionMethod"},
		"redaction":  {"applied", "secretValuesPresent"},
	}
)

func DecodeHostProfile(data []byte) (HostProfile, error) {
	var profile HostProfile
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return profile, fmt.Errorf("request body is required")
	}
	top, err := parseObject(trimmed, "host profile")
	if err != nil {
		return profile, err
	}
	if err := checkAllowed(top, topLevelProperties, "host profile"); err != nil {
		return profile, err
	}
	for _, field := range []string{
		"schemaVersion", "hostId", "tenantScope", "observedAt", "expiresAt",
		"os", "architecture", "resources", "docker", "redaction",
	} {
		if err := requirePresent(top, field, field); err != nil {
			return profile, err
		}
	}
	for field, raw := range top {
		if isNull(raw) {
			return profile, fmt.Errorf("%s must not be null", field)
		}
	}
	nestedRequired := map[string][]string{
		"os":        {"family", "version"},
		"resources": {"cpuCores", "ramMb", "storageFreeMb"},
		"docker":    {"engineVersion", "composeVersion"},
		"redaction": {"applied", "secretValuesPresent"},
	}
	// filesystem and provenance are optional at the top level but,
	// when present, all of their schema-required fields apply.
	optionalNestedRequired := map[string][]string{
		"filesystem": {"type", "dockerRootWritable"},
		"provenance": {"sourceType", "extractionMethod"},
	}
	for parent, fields := range nestedRequired {
		if err := checkNested(top, parent, fields); err != nil {
			return profile, err
		}
	}
	for parent, fields := range optionalNestedRequired {
		if _, present := top[parent]; present {
			if err := checkNested(top, parent, fields); err != nil {
				return profile, err
			}
		}
	}
	// Every present nested object is checked against its exact
	// property allowlist, and every present value inside it must be
	// non-null (all schema property types reject null).
	for _, parent := range []string{"os", "resources", "docker", "redaction", "filesystem", "provenance", "network"} {
		raw, present := top[parent]
		if !present {
			continue
		}
		fields, err := objectFields(raw, parent)
		if err != nil {
			return profile, err
		}
		if err := checkAllowed(fields, nestedProperties[parent], parent); err != nil {
			return profile, err
		}
		names := make([]string, 0, len(fields))
		for name := range fields {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if isNull(fields[name]) {
				return profile, fmt.Errorf("%s.%s must not be null", parent, name)
			}
		}
		// An explicitly supplied network.mode must satisfy the
		// schema enum; an omitted mode (network:{} or no network)
		// is valid and stays distinguishable only at this raw
		// layer, since the struct represents both as "".
		if parent == "network" {
			if modeRaw, supplied := fields["mode"]; supplied {
				var mode string
				if err := json.Unmarshal(modeRaw, &mode); err != nil {
					return profile, fmt.Errorf("network.mode must be a string")
				}
				switch mode {
				case "local", "byo-vps", "nas":
				default:
					return profile, fmt.Errorf("network.mode %q is not supported", mode)
				}
			}
		}
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&profile); err != nil {
		return profile, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return profile, fmt.Errorf("trailing data after JSON value")
		}
		return profile, err
	}
	// Normalise the optional array so stored records never carry a
	// nil slice into listing serialisation.
	if profile.OccupiedPorts == nil {
		profile.OccupiedPorts = []int{}
	}
	return profile, nil
}

// parseObject decodes one JSON object, rejecting duplicate keys.
// Plain json.Unmarshal into a map silently keeps the last duplicate,
// which would let a smuggled second value replace the checked one.
func parseObject(raw []byte, label string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("%s must be a JSON object", label)
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("%s: object key must be a string", label)
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, fmt.Errorf("%s: duplicate property %q", label, key)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	if _, err := decoder.Token(); err != nil { // closing '}'
		return nil, err
	}
	return fields, nil
}

// checkAllowed enforces additionalProperties: false with exact,
// case-sensitive property names at one object level.
func checkAllowed(fields map[string]json.RawMessage, allowed []string, label string) error {
	allowedSet := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = true
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !allowedSet[name] {
			return fmt.Errorf("%s: property %q is not allowed", label, name)
		}
	}
	return nil
}

func isNull(raw json.RawMessage) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}

func requirePresent(fields map[string]json.RawMessage, field, label string) error {
	raw, present := fields[field]
	if !present {
		return fmt.Errorf("%s is required", label)
	}
	if isNull(raw) {
		return fmt.Errorf("%s must not be null", label)
	}
	return nil
}

func objectFields(raw json.RawMessage, label string) (map[string]json.RawMessage, error) {
	fields, err := parseObject(bytes.TrimSpace(raw), label)
	if err != nil {
		return nil, err
	}
	return fields, nil
}

func checkNested(top map[string]json.RawMessage, parent string, required []string) error {
	fields, err := objectFields(top[parent], parent)
	if err != nil {
		return err
	}
	for _, field := range required {
		if err := requirePresent(fields, field, parent+"."+field); err != nil {
			return err
		}
	}
	return nil
}
