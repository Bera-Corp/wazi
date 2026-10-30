package contracts

import (
	"fmt"
	"regexp"
	"time"
)

// Validation mirrors schemas/host-profile.schema.json plus the
// freshness rules in scripts/validate.py (decide/semantic_errors).
// The JSON schema remains the source of truth; this is the Go intake
// enforcement of the same contract so the control plane never stores
// a profile the Phase 0 validator would reject.

var (
	hostIDPattern   = regexp.MustCompile(`^host_[A-Za-z0-9_-]+$`)
	tenantPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
	stableVersionRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
)

// Freshness policy for intake. Profiles are issued for 30 minutes;
// intake rejects expired, future-dated, over-aged and implausibly
// long-lived observations. ClockSkew tolerates small clock drift.
const (
	MaxProfileLifetime = time.Hour
	MaxClockSkew       = 2 * time.Minute
)

// ValidateHostProfile checks the full host-profile contract and its
// freshness at now. It returns the first violation; callers map any
// error to a 400 response and store nothing.
func ValidateHostProfile(p HostProfile, now time.Time) error {
	if p.SchemaVersion != "1.0.0" {
		return fmt.Errorf("schemaVersion must be 1.0.0")
	}
	if !hostIDPattern.MatchString(p.HostID) {
		return fmt.Errorf("hostId must match ^host_[A-Za-z0-9_-]+$")
	}
	if !tenantPattern.MatchString(p.TenantScope) {
		return fmt.Errorf("tenantScope is required and must be an identifier")
	}
	observed, err := time.Parse(time.RFC3339, p.ObservedAt)
	if err != nil {
		return fmt.Errorf("observedAt must be RFC3339: %w", err)
	}
	expires, err := time.Parse(time.RFC3339, p.ExpiresAt)
	if err != nil {
		return fmt.Errorf("expiresAt must be RFC3339: %w", err)
	}
	if !expires.After(observed) {
		return fmt.Errorf("expiresAt must be after observedAt")
	}
	if expires.Sub(observed) > MaxProfileLifetime {
		return fmt.Errorf("profile lifetime exceeds %s", MaxProfileLifetime)
	}
	now = now.UTC()
	if observed.After(now.Add(MaxClockSkew)) {
		return fmt.Errorf("observedAt is in the future")
	}
	if !now.Before(expires) {
		return fmt.Errorf("host profile expired")
	}
	if p.OS.Family == "" {
		return fmt.Errorf("os.family is required")
	}
	if p.OS.Version == "" {
		return fmt.Errorf("os.version is required")
	}
	if p.Architecture == "" {
		return fmt.Errorf("architecture is required")
	}
	if !(p.Resources.CPUCores > 0) {
		return fmt.Errorf("resources.cpuCores must be > 0")
	}
	if p.Resources.RAMMb < 0 {
		return fmt.Errorf("resources.ramMb must be >= 0")
	}
	if p.Resources.StorageFreeMb < 0 {
		return fmt.Errorf("resources.storageFreeMb must be >= 0")
	}
	if !stableVersionRe.MatchString(p.Docker.EngineVersion) {
		return fmt.Errorf("docker.engineVersion must be a stable x.y.z version, got %q", p.Docker.EngineVersion)
	}
	if !stableVersionRe.MatchString(p.Docker.ComposeVersion) {
		return fmt.Errorf("docker.composeVersion must be a stable x.y.z version, got %q", p.Docker.ComposeVersion)
	}
	seenPorts := map[int]bool{}
	for _, port := range p.OccupiedPorts {
		if port < 1 || port > 65535 {
			return fmt.Errorf("occupiedPorts entry %d out of range", port)
		}
		if seenPorts[port] {
			return fmt.Errorf("occupiedPorts contains duplicate %d", port)
		}
		seenPorts[port] = true
	}
	// Stricter than the schema at runtime: filesystem/provenance
	// are optional in the schema, but a profile the executor will
	// rely on must carry them. This is a runtime requirement, not
	// a schema-parity claim.
	if p.Filesystem.Type == "" {
		return fmt.Errorf("filesystem.type is required")
	}
	// "" here means the optional mode was omitted (the struct
	// cannot distinguish that from an explicit ""). An explicitly
	// supplied empty mode is rejected earlier, at the raw layer in
	// DecodeHostProfile, where presence is still visible.
	switch p.Network.Mode {
	case "", "local", "byo-vps", "nas":
	default:
		return fmt.Errorf("network.mode %q is not supported", p.Network.Mode)
	}
	if p.Provenance.SourceType != "host-agent-probe" {
		return fmt.Errorf("provenance.sourceType must be host-agent-probe")
	}
	if p.Provenance.ExtractionMethod != "host-probe" {
		return fmt.Errorf("provenance.extractionMethod must be host-probe")
	}
	if !p.Redaction.Applied {
		return fmt.Errorf("redaction.applied must be true")
	}
	if p.Redaction.SecretValuesPresent {
		return fmt.Errorf("redaction.secretValuesPresent must be false")
	}
	return nil
}
