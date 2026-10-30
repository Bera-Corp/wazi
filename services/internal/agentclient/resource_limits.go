package agentclient

import (
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
)

// validateApplicationResourceLimits binds the signed per-service CPU/RAM
// ceilings to the owner-approved application allocation. The renderer emits
// these values into Compose; this preflight proves their sum fits the signed
// budget before approvals may be requested.
func validateApplicationResourceLimits(manifest map[string]any, budget contracts.ContributionBudget) error {
	release, ok := objectValue(manifest["release"])
	if !ok {
		return fmt.Errorf("signed manifest release is invalid")
	}
	images, ok := release["images"].([]any)
	if !ok || len(images) == 0 {
		return fmt.Errorf("signed manifest images are invalid")
	}
	serviceNames := make(map[string]struct{}, len(images))
	for _, rawImage := range images {
		image, ok := rawImage.(map[string]any)
		if !ok {
			return fmt.Errorf("signed manifest image entry is invalid")
		}
		service, ok := image["service"].(string)
		if !ok || service == "" {
			return fmt.Errorf("signed manifest image service is invalid")
		}
		if _, duplicate := serviceNames[service]; duplicate {
			return fmt.Errorf("signed manifest contains duplicate image service %q", service)
		}
		serviceNames[service] = struct{}{}
	}
	limits, ok := release["resourceLimits"].([]any)
	if !ok || len(limits) != len(serviceNames) {
		return fmt.Errorf("Phase 1 signed manifest must declare one resource limit for every image service")
	}

	var applicationAllocation *contracts.ContributionResourceSet
	for i := range budget.Allocations {
		if budget.Allocations[i].Role == "application" {
			applicationAllocation = &budget.Allocations[i].Resources
			break
		}
	}
	if applicationAllocation == nil {
		return fmt.Errorf("approved contribution budget has no application allocation")
	}
	allocationCPU, ok := new(big.Rat).SetString(string(applicationAllocation.CPUCores))
	if !ok {
		return fmt.Errorf("application CPU allocation is invalid")
	}
	allocationRAM, ok := new(big.Int).SetString(string(applicationAllocation.RAMMb), 10)
	if !ok {
		return fmt.Errorf("application RAM allocation is invalid")
	}
	allocationStorage, ok := new(big.Int).SetString(string(applicationAllocation.StorageMb), 10)
	if !ok {
		return fmt.Errorf("application storage allocation is invalid")
	}

	seen := make(map[string]struct{}, len(limits))
	totalCPU := new(big.Rat)
	totalRAM := new(big.Int)
	for _, rawLimit := range limits {
		limit, ok := rawLimit.(map[string]any)
		if !ok {
			return fmt.Errorf("signed manifest resource limit is invalid")
		}
		service, ok := limit["service"].(string)
		if !ok || service == "" {
			return fmt.Errorf("signed manifest resource limit service is invalid")
		}
		if _, declared := serviceNames[service]; !declared {
			return fmt.Errorf("resource limit references unknown service %q", service)
		}
		if _, duplicate := seen[service]; duplicate {
			return fmt.Errorf("signed manifest has duplicate resource limit for service %q", service)
		}
		seen[service] = struct{}{}
		cpuText, ok := limit["cpuCores"].(json.Number)
		if !ok {
			return fmt.Errorf("service %q CPU limit is invalid", service)
		}
		cpu, ok := new(big.Rat).SetString(string(cpuText))
		if !ok || cpu.Sign() <= 0 {
			return fmt.Errorf("service %q CPU limit must be positive", service)
		}
		ramText, ok := limit["ramMb"].(json.Number)
		if !ok {
			return fmt.Errorf("service %q RAM limit is invalid", service)
		}
		ram, ok := new(big.Int).SetString(string(ramText), 10)
		if !ok || ram.Sign() <= 0 {
			return fmt.Errorf("service %q RAM limit must be a positive integer", service)
		}
		totalCPU.Add(totalCPU, cpu)
		totalRAM.Add(totalRAM, ram)
	}
	if len(seen) != len(serviceNames) {
		return fmt.Errorf("signed manifest is missing a service resource limit")
	}
	if totalCPU.Cmp(allocationCPU) > 0 {
		return fmt.Errorf("application service CPU limits exceed the approved application allocation")
	}
	if totalRAM.Cmp(allocationRAM) > 0 {
		return fmt.Errorf("application service RAM limits exceed the approved application allocation")
	}
	volumeStorage := new(big.Int)
	if rawVolumes, exists := release["volumes"]; exists {
		volumes, ok := rawVolumes.([]any)
		if !ok {
			return fmt.Errorf("signed manifest volumes are invalid")
		}
		seenNames := make(map[string]struct{}, len(volumes))
		for _, rawVolume := range volumes {
			volume, ok := rawVolume.(map[string]any)
			if !ok {
				return fmt.Errorf("signed manifest volume entry is invalid")
			}
			name, ok := volume["name"].(string)
			if !ok || name == "" {
				return fmt.Errorf("signed manifest volume name is invalid")
			}
			if _, duplicate := seenNames[name]; duplicate {
				return fmt.Errorf("signed manifest has duplicate managed volume %q", name)
			}
			seenNames[name] = struct{}{}
			if rawService, exists := volume["service"]; exists {
				service, ok := rawService.(string)
				if !ok {
					return fmt.Errorf("managed volume %q service owner is invalid", name)
				}
				if _, declared := serviceNames[service]; !declared {
					return fmt.Errorf("managed volume %q references unknown service %q", name, service)
				}
			} else if len(serviceNames) != 1 {
				return fmt.Errorf("managed volume %q must declare an owner when the manifest has multiple services", name)
			}
			storageText, ok := volume["storageLimitMb"].(json.Number)
			if !ok {
				return fmt.Errorf("managed volume %q is missing its hard storage limit", name)
			}
			storage, ok := new(big.Int).SetString(string(storageText), 10)
			if !ok || storage.Sign() <= 0 {
				return fmt.Errorf("managed volume %q storage limit must be a positive integer", name)
			}
			volumeStorage.Add(volumeStorage, storage)
		}
	}

	requirements, ok := objectValue(manifest["requirements"])
	if !ok {
		return fmt.Errorf("signed manifest requirements are invalid")
	}
	minCPUText, ok := requirements["minCpuCores"].(json.Number)
	if !ok {
		return fmt.Errorf("signed manifest minimum CPU requirement is invalid")
	}
	minCPU, ok := new(big.Rat).SetString(string(minCPUText))
	if !ok || totalCPU.Cmp(minCPU) < 0 {
		return fmt.Errorf("application CPU limits do not meet the signed minimum requirement")
	}
	minRAMText, ok := requirements["minRamMb"].(json.Number)
	if !ok {
		return fmt.Errorf("signed manifest minimum RAM requirement is invalid")
	}
	minRAM, ok := new(big.Int).SetString(string(minRAMText), 10)
	if !ok || totalRAM.Cmp(minRAM) < 0 {
		return fmt.Errorf("application RAM limits do not meet the signed minimum requirement")
	}
	minStorageText, ok := requirements["minStorageMb"].(json.Number)
	if !ok {
		return fmt.Errorf("signed manifest minimum storage requirement is invalid")
	}
	minStorage, ok := new(big.Int).SetString(string(minStorageText), 10)
	if !ok || allocationStorage.Cmp(minStorage) < 0 {
		return fmt.Errorf("application storage allocation does not meet the signed minimum requirement")
	}
	if volumeStorage.Sign() > 0 {
		if volumeStorage.Cmp(allocationStorage) > 0 {
			return fmt.Errorf("managed volume storage limits exceed the approved application allocation")
		}
		if volumeStorage.Cmp(minStorage) < 0 {
			return fmt.Errorf("managed volume storage limits do not meet the signed minimum requirement")
		}
	}
	return nil
}
