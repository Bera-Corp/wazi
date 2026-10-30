// Package contracts mirrors the Phase 0 JSON contracts in Go.
// Field names and values must stay aligned with schemas/*.schema.json;
// the JSON schema remains the source of truth and these structs are
// validated against it in tests and by scripts/validate.py gates.
package contracts

// HostProfile mirrors schemas/host-profile.schema.json (schemaVersion 1.0.0).
type HostProfile struct {
	SchemaVersion string `json:"schemaVersion"`
	HostID        string `json:"hostId"`
	TenantScope   string `json:"tenantScope"`
	ObservedAt    string `json:"observedAt"`
	ExpiresAt     string `json:"expiresAt"`
	OS            struct {
		Family       string `json:"family"`
		Distribution string `json:"distribution,omitempty"`
		Version      string `json:"version"`
	} `json:"os"`
	Architecture string `json:"architecture"`
	Resources    struct {
		CPUCores      float64 `json:"cpuCores"`
		RAMMb         int64   `json:"ramMb"`
		StorageFreeMb int64   `json:"storageFreeMb"`
	} `json:"resources"`
	Docker struct {
		EngineVersion  string `json:"engineVersion"`
		ComposeVersion string `json:"composeVersion"`
	} `json:"docker"`
	// omitempty keeps an omitted optional array omitted (rather
	// than serialising as null) when a stored profile is listed.
	OccupiedPorts []int `json:"occupiedPorts,omitempty"`
	Filesystem    struct {
		Type               string `json:"type"`
		DockerRootWritable bool   `json:"dockerRootWritable"`
	} `json:"filesystem"`
	Network struct {
		// omitempty: an omitted mode (network:{} or no explicit
		// mode) must round-trip as omitted, not as the schema-
		// invalid empty string. DecodeHostProfile rejects an
		// explicitly supplied empty/invalid mode at the raw layer.
		Mode                  string `json:"mode,omitempty"`
		PublicExposureCapable bool   `json:"publicExposureCapable"`
	} `json:"network"`
	Provenance struct {
		SourceType       string `json:"sourceType"`
		ExtractionMethod string `json:"extractionMethod"`
	} `json:"provenance"`
	Redaction struct {
		Applied             bool `json:"applied"`
		SecretValuesPresent bool `json:"secretValuesPresent"`
	} `json:"redaction"`
}
