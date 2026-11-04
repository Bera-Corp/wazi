package agentclient

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// PackageTransition is protected operator policy, never candidate/job input.
// This initial adapter permits the application's own SQLite startup migration;
// it does not run arbitrary SQL or authorize database-engine major upgrades.
type PackageTransition struct {
	FromManifestDigest, ToManifestDigest, ProcedureDigest, QualificationDigest string
	Operation, Mode                                                            string
}

type packageTransitionIntent struct {
	From, To                 map[string]string
	PlanDigest, CheckpointID string
}

func validatePackageTransitions(config PackageInstanceConfig) error {
	if (config.RecoveryReleaseVersion == "") != (config.RecoveryProcedureVersion == "") {
		return fmt.Errorf("recovery release and procedure versions must be configured together")
	}
	if len(config.Transitions) > 16 {
		return fmt.Errorf("too many package transitions")
	}
	pattern := regexp.MustCompile("^sha256:[0-9a-f]{64}$")
	seen := map[string]bool{}
	for _, rule := range config.Transitions {
		if !config.Runtime.AllowedManifests[rule.FromManifestDigest] || !config.Runtime.AllowedManifests[rule.ToManifestDigest] {
			return fmt.Errorf("both migration releases must be admitted by operator policy")
		}
		if config.Profile != "uptime-kuma" || rule.Mode != "uptime-kuma-startup" || (rule.Operation != "upgrade" && rule.Operation != "restore") || rule.FromManifestDigest == rule.ToManifestDigest {
			return fmt.Errorf("unsupported package migration adapter")
		}
		for _, digest := range []string{rule.FromManifestDigest, rule.ToManifestDigest, rule.ProcedureDigest, rule.QualificationDigest} {
			if !pattern.MatchString(digest) {
				return fmt.Errorf("transition requires pinned qualification and release digests")
			}
		}
		key := rule.FromManifestDigest + rule.ToManifestDigest + rule.Operation
		if seen[key] {
			return fmt.Errorf("duplicate package transition")
		}
		seen[key] = true
	}
	return nil
}

func (b *DockerPackageBoundary) readPackageAnchor() (map[string]string, *packageTransitionIntent, error) {
	var anchor map[string]string
	raw, err := os.ReadFile(filepath.Join(b.config.Runtime.StateDirectory, "instance-release.json"))
	if err != nil {
		return nil, nil, err
	}
	if json.Unmarshal(raw, &anchor) != nil || len(anchor) != 4 || anchor["profile"] != b.config.Profile || anchor["project"] != b.config.Runtime.Project {
		return nil, nil, fmt.Errorf("package anchor invalid")
	}
	var pending packageTransitionIntent
	raw, err = os.ReadFile(filepath.Join(b.config.Runtime.StateDirectory, "instance-transition.json"))
	if os.IsNotExist(err) {
		return anchor, nil, nil
	}
	if err != nil || json.Unmarshal(raw, &pending) != nil || pending.To["project"] != b.config.Runtime.Project || pending.To["profile"] != b.config.Profile || pending.PlanDigest == "" {
		return nil, nil, fmt.Errorf("package transition intent invalid")
	}
	return pending.To, &pending, nil
}

func (b *DockerPackageBoundary) admitPackageTransition(p PreparedExecution, from, to map[string]string, persist bool) error {
	plan, _, err := decodePrepared(p)
	if err != nil {
		return err
	}
	matched := false
	for _, rule := range b.config.Transitions {
		if rule.FromManifestDigest == from["manifestDigest"] && rule.ToManifestDigest == to["manifestDigest"] && rule.ProcedureDigest == p.Binding().Release.ProcedureDigest && rule.Operation == plan.Operation {
			matched = true
		}
	}
	if !matched {
		return fmt.Errorf("cross-version package change requires a qualified operator transition")
	}
	envelope, err := parseCanonicalObject(p.Envelope(), "migration envelope")
	if err != nil {
		return err
	}
	strategy, _ := objectValue(envelope["rollbackStrategy"])
	checkpoint, _ := objectValue(strategy["checkpoint"])
	id, _ := checkpoint["backupId"].(string)
	expected := from["manifestDigest"]
	if plan.Operation == "restore" {
		expected = to["manifestDigest"]
		for _, step := range plan.Steps {
			if step.Capability == "volume.restore" {
				id = step.Parameters.VolumeRestore.BackupID
			}
		}
	}
	if !pilotBackupID.MatchString(id) || checkpoint["volume"] != b.shape.Volume {
		return fmt.Errorf("version transition requires a bound checkpoint")
	}
	if !persist {
		return nil
	}
	file, record, err := b.verifiedCheckpointForDigest(p, id, expected)
	if err != nil {
		return fmt.Errorf("version transition checkpoint unavailable: %w", err)
	}
	defer file.Close()
	if record.Format != "age-v1" || record.PayloadFormat != "volume-tar" {
		return fmt.Errorf("encrypted volume transition checkpoint required")
	}
	staged, cleanup, _, err := stageAgeDecryption(file, filepath.Dir(file.Name()), b.config.BackupEncryption.IdentityFile, int64(b.shape.Storage)*1024*1024)
	if err != nil {
		return err
	}
	defer cleanup()
	if err = b.validatePackageTar(staged); err != nil {
		return err
	}
	intent := packageTransitionIntent{From: from, To: to, PlanDigest: p.Binding().PlanDigest, CheckpointID: id}
	return b.writePackageState("instance-transition.json", intent)
}

func (b *DockerPackageBoundary) writePackageState(name string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(b.config.Runtime.StateDirectory, ".package-state-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err = file.Write(raw); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Rename(file.Name(), filepath.Join(b.config.Runtime.StateDirectory, name)); err != nil {
		return err
	}
	return syncCheckpointDirectory(b.config.Runtime.StateDirectory)
}

func (b *DockerPackageBoundary) finishPackageTransition(p PreparedExecution) error {
	anchor, pending, err := b.readPackageAnchor()
	if err != nil {
		return err
	}
	if pending == nil {
		return nil
	}
	if pending.PlanDigest != p.Binding().PlanDigest || anchor["manifestDigest"] != p.Binding().Release.ManifestDigest {
		return fmt.Errorf("another package transition requires reconciliation")
	}
	if err = b.writePackageState("instance-release.json", pending.To); err != nil {
		return err
	}
	if err = os.Remove(filepath.Join(b.config.Runtime.StateDirectory, "instance-transition.json")); err != nil {
		return err
	}
	return syncCheckpointDirectory(b.config.Runtime.StateDirectory)
}
