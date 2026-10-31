package agentclient

import (
	"fmt"
)

// PackageRegistry is the closed, operator-reviewed instance catalog. It never
// imports a profile from a job, a model response or an upstream Compose file.
type PackageRegistry struct {
	entries map[string]PackageInstanceConfig
}

type ReviewedRuntimeBoundary interface {
	Boundary
	ValidatePrepared(PreparedExecution) error
	ValidateContext(PreparedExecution) error
}

func ResolveRuntimeBoundary(legacy DockerPilotConfig, budget string, registry *PackageRegistry, artifact []byte) (ReviewedRuntimeBoundary, string, error) {
	value, err := parseCanonicalObject(artifact, "compose artifact")
	if err != nil {
		return nil, "", err
	}
	if value["projectName"] == legacy.Project {
		b, err := NewDockerPilotBoundary(legacy, artifact)
		return b, budget, err
	}
	return registry.Boundary(artifact)
}

func NewPackageRegistry(configs []PackageInstanceConfig) (*PackageRegistry, error) {
	if len(configs) > 6 {
		return nil, fmt.Errorf("package registry capacity exceeded")
	}
	registry := &PackageRegistry{entries: map[string]PackageInstanceConfig{}}
	for _, config := range configs {
		// Constructor validates policy without loading images or mutating data.
		if _, err := NewDockerPackageBoundary(config, []byte("{}")); err != nil {
			return nil, err
		}
		project := config.Runtime.Project
		if _, exists := registry.entries[project]; exists {
			return nil, fmt.Errorf("duplicate package instance")
		}
		config.Runtime.AllowedManifests = cloneBoolMap(config.Runtime.AllowedManifests)
		config.Runtime.ImageConfigDigests = cloneStringMap(config.Runtime.ImageConfigDigests)
		config.SecretFiles = cloneStringMap(config.SecretFiles)
		registry.entries[project] = config
	}
	return registry, nil
}

func (r *PackageRegistry) Boundary(artifact []byte) (*DockerPackageBoundary, string, error) {
	value, err := parseCanonicalObject(artifact, "package artifact")
	if err != nil {
		return nil, "", err
	}
	project, _ := value["projectName"].(string)
	if r == nil {
		return nil, "", fmt.Errorf("package registry disabled")
	}
	config, exists := r.entries[project]
	if !exists {
		return nil, "", fmt.Errorf("package instance has not been qualified and enabled")
	}
	boundary, err := NewDockerPackageBoundary(config, artifact)
	return boundary, config.BudgetDigest, err
}
