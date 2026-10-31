package agentclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type runningPackageResource struct {
	Project          string
	Memory, NanoCPUs int64
}

func packageCapacityCheck(memory int64, cpus float64, existing []runningPackageResource, project string, shape packageShape) error {
	// One serial helper plus the bounded agent/daemon allowance. Stopped
	// application instances retain storage but do not reserve running RAM/CPU.
	neededMemory := int64(shape.RAM+640) * 1024 * 1024
	shapeCPU, _ := strconv.ParseFloat(shape.CPU, 64)
	neededCPU := shapeCPU + 0.75
	for _, resource := range existing {
		if resource.Project == project {
			continue
		}
		if resource.Memory <= 0 || resource.NanoCPUs <= 0 {
			return fmt.Errorf("cannot admit beside an unbounded container")
		}
		neededMemory += resource.Memory
		neededCPU += float64(resource.NanoCPUs) / 1e9
	}
	if memory <= 0 || cpus <= 0 || neededMemory > memory || neededCPU > cpus+0.000001 {
		return fmt.Errorf("aggregate package demand exceeds the enforced runtime budget")
	}
	return nil
}

func (b *DockerPackageBoundary) verifyCapacity(ctx context.Context) error {
	root := filepath.Join("/sys/fs/cgroup/user.slice", "user-"+strconv.Itoa(b.config.Runtime.RuntimeUID)+".slice")
	memoryRaw, err := os.ReadFile(filepath.Join(root, "memory.max"))
	if err != nil {
		return fmt.Errorf("aggregate memory limit unavailable")
	}
	memory, err := strconv.ParseInt(strings.TrimSpace(string(memoryRaw)), 10, 64)
	if err != nil {
		return fmt.Errorf("aggregate memory limit must be finite")
	}
	cpuRaw, err := os.ReadFile(filepath.Join(root, "cpu.max"))
	if err != nil {
		return fmt.Errorf("aggregate CPU limit unavailable")
	}
	parts := strings.Fields(string(cpuRaw))
	if len(parts) != 2 {
		return fmt.Errorf("aggregate CPU limit invalid")
	}
	quota, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return fmt.Errorf("aggregate CPU limit must be finite")
	}
	period, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || period <= 0 {
		return fmt.Errorf("aggregate CPU period invalid")
	}
	if memory > 3072*1024*1024 || quota/period > 4.000001 {
		return fmt.Errorf("runtime cap exceeds the approved Phase 2 allocation")
	}
	command := exec.CommandContext(ctx, "/usr/bin/sudo", "-n", b.config.Runtime.QuotaHelper, "capacity")
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin"}
	var capacity boundedOutput
	command.Stdout = &capacity
	if err := command.Run(); err != nil {
		return fmt.Errorf("shared storage quota cannot be verified")
	}
	var pool struct{ UsedKiB, HardKiB int64 }
	if json.Unmarshal(capacity.Bytes(), &pool) != nil || pool.HardKiB != 13*1024*1024 || pool.UsedKiB < 0 ||
		pool.HardKiB-pool.UsedKiB < int64(b.shape.Storage+256)*1024 {
		return fmt.Errorf("shared checkpoint/cache pool has insufficient enforced headroom")
	}
	raw, err := b.core.run(ctx, nil, nil, "ps", "--filter", "status=running", "--format", "{{.ID}}")
	if err != nil {
		return err
	}
	ids := strings.Fields(string(raw))
	if len(ids) > 16 {
		return fmt.Errorf("runtime container capacity exceeded")
	}
	existing := []runningPackageResource{}
	for _, id := range ids {
		raw, err := b.core.run(ctx, nil, nil, "inspect", id, "--format", "{{json .}}")
		if err != nil {
			return err
		}
		var value struct {
			Config     struct{ Labels map[string]string }
			State      struct{ Running bool }
			HostConfig struct{ Memory, NanoCpus int64 }
		}
		if json.Unmarshal(raw, &value) != nil {
			return fmt.Errorf("runtime allocation unreadable")
		}
		if value.State.Running {
			existing = append(existing, runningPackageResource{Project: value.Config.Labels["com.docker.compose.project"], Memory: value.HostConfig.Memory, NanoCPUs: value.HostConfig.NanoCpus})
		}
	}
	if err := packageCapacityCheck(memory, quota/period, existing, b.config.Runtime.Project, b.shape); err != nil {
		return err
	}
	return b.verifyStorageReservations(ctx)
}

func (b *DockerPackageBoundary) verifyStorageReservations(ctx context.Context) error {
	raw, err := b.core.run(ctx, nil, nil, "volume", "ls", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	sizes := map[string]int64{"wazi-gitea-phase1_gitea-data": 18432, "wazi-gitea-phase1_gitea-config": 2048}
	for _, profile := range []string{"postgresql", "mysql", "nginx"} {
		shape, _ := shapeFor(profile, false)
		for _, suffix := range []string{"01", "02"} {
			sizes["wazi-"+profile+"-"+suffix+"_"+shape.Volume] = int64(shape.Storage)
		}
	}
	var total int64
	present := false
	for _, name := range strings.Fields(string(raw)) {
		size, known := sizes[name]
		if !known {
			return fmt.Errorf("cannot admit beside an unqualified managed volume")
		}
		total += size
		if name == b.config.Runtime.Project+"_"+b.shape.Volume {
			present = true
		}
	}
	if !present {
		total += int64(b.shape.Storage)
	}
	if total > b.config.ApplicationStorageBudgetMb {
		return fmt.Errorf("managed volume reservations exceed the approved application allocation")
	}
	return nil
}
