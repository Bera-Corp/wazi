package contracts

import (
	"strings"
	"testing"
	"time"
)

func validProfile(now time.Time) HostProfile {
	var p HostProfile
	p.SchemaVersion = "1.0.0"
	p.HostID = "host_demo01"
	p.TenantScope = "tenant_demo"
	p.ObservedAt = now.Add(-time.Minute).Format(time.RFC3339)
	p.ExpiresAt = now.Add(29 * time.Minute).Format(time.RFC3339)
	p.OS.Family = "linux"
	p.OS.Distribution = "ubuntu"
	p.OS.Version = "24.04"
	p.Architecture = "x86_64"
	p.Resources.CPUCores = 4
	p.Resources.RAMMb = 8192
	p.Resources.StorageFreeMb = 51200
	p.Docker.EngineVersion = "29.1.3"
	p.Docker.ComposeVersion = "2.40.3"
	p.OccupiedPorts = []int{22, 443}
	p.Filesystem.Type = "ext4"
	p.Filesystem.DockerRootWritable = true
	p.Network.Mode = "byo-vps"
	p.Provenance.SourceType = "host-agent-probe"
	p.Provenance.ExtractionMethod = "host-probe"
	p.Redaction.Applied = true
	p.Redaction.SecretValuesPresent = false
	return p
}

func TestValidateHostProfileAcceptsValid(t *testing.T) {
	now := time.Now().UTC()
	if err := ValidateHostProfile(validProfile(now), now); err != nil {
		t.Fatalf("valid profile rejected: %v", err)
	}
}

func TestValidateHostProfileRejects(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name   string
		mutate func(*HostProfile)
		wantIn string
	}{
		{"wrong schema", func(p *HostProfile) { p.SchemaVersion = "0.9.0" }, "schemaVersion"},
		{"missing tenant", func(p *HostProfile) { p.TenantScope = "" }, "tenantScope"},
		{"bad host id", func(p *HostProfile) { p.HostID = "demo01" }, "hostId"},
		{"missing resources", func(p *HostProfile) { p.Resources.CPUCores = 0 }, "cpuCores"},
		{"prerelease engine", func(p *HostProfile) { p.Docker.EngineVersion = "27.3.1-rc1" }, "engineVersion"},
		{"prerelease compose", func(p *HostProfile) { p.Docker.ComposeVersion = "2.40.3-rc1" }, "composeVersion"},
		{"expired", func(p *HostProfile) {
			p.ObservedAt = now.Add(-2 * time.Hour).Format(time.RFC3339)
			p.ExpiresAt = now.Add(-time.Hour).Format(time.RFC3339)
		}, "expired"},
		{"future observed", func(p *HostProfile) {
			p.ObservedAt = now.Add(time.Hour).Format(time.RFC3339)
			p.ExpiresAt = now.Add(90 * time.Minute).Format(time.RFC3339)
		}, "future"},
		{"expires before observed", func(p *HostProfile) { p.ExpiresAt = p.ObservedAt }, "after observedAt"},
		{"duplicate port", func(p *HostProfile) { p.OccupiedPorts = []int{443, 443} }, "duplicate"},
		{"port out of range", func(p *HostProfile) { p.OccupiedPorts = []int{70000} }, "out of range"},
		{"secret present", func(p *HostProfile) { p.Redaction.SecretValuesPresent = true }, "secretValuesPresent"},
		{"redaction not applied", func(p *HostProfile) { p.Redaction.Applied = false }, "redaction"},
		{"bad provenance", func(p *HostProfile) { p.Provenance.SourceType = "user-supplied" }, "sourceType"},
		{"bad network mode", func(p *HostProfile) { p.Network.Mode = "public" }, "network.mode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			profile := validProfile(now)
			tc.mutate(&profile)
			err := ValidateHostProfile(profile, now)
			if err == nil {
				t.Fatalf("invalid profile (%s) accepted", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Errorf("error %q should mention %q", err, tc.wantIn)
			}
		})
	}
}
