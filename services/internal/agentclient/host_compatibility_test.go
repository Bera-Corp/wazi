package agentclient

import (
	"testing"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
)

func TestResourceBindingRejectsUnsupportedHostBeforeApproval(t *testing.T) {
	fixture, records := newResourceBindingFixture(t)
	host, err := contracts.DecodeHostProfile(records.HostProfileRaw)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyHostCompatibility(fixture.manifest, host); err != nil {
		t.Fatalf("baseline host: %v", err)
	}
	for _, name := range []string{"architecture", "os", "docker", "prerelease", "compose", "ram", "storage", "cpu"} {
		t.Run(name, func(t *testing.T) {
			bad := host
			switch name {
			case "architecture":
				bad.Architecture = "unsupported"
			case "os":
				bad.OS.Family = "unsupported"
			case "docker":
				bad.Docker.EngineVersion = "0.1.0"
			case "prerelease":
				bad.Docker.EngineVersion = "99.1.0-rc1"
			case "compose":
				bad.Docker.ComposeVersion = "0.1.0"
			case "ram":
				bad.Resources.RAMMb = 0
			case "storage":
				bad.Resources.StorageFreeMb = 0
			case "cpu":
				bad.Resources.CPUCores = 0
			}
			if VerifyHostCompatibility(fixture.manifest, bad) == nil {
				t.Fatal("incompatible host accepted")
			}
		})
	}
}
