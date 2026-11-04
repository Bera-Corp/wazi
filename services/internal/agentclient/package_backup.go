package agentclient

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/age"
)

type packageCheckpoint struct {
	Format, Profile, Project, Tenant, ManifestDigest, BackupID, PayloadDigest, CreatedAt string
	PayloadFormat                                                                        string
	Bytes                                                                                int64
}

type limitedPackageDump struct {
	writer    io.Writer
	remaining int64
}

func (w *limitedPackageDump) Write(raw []byte) (int, error) {
	if int64(len(raw)) > w.remaining {
		return 0, fmt.Errorf("package checkpoint exceeds approved storage bound")
	}
	n, err := w.writer.Write(raw)
	w.remaining -= int64(n)
	return n, err
}

func (b *DockerPackageBoundary) checkpointPath(id, kind string) (string, error) {
	if !pilotBackupID.MatchString(id) || (kind != "backups" && kind != "pre-restore") {
		return "", fmt.Errorf("invalid package checkpoint identity")
	}
	root := filepath.Join(b.config.Runtime.StateDirectory, kind)
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("package checkpoint directory must be private")
	}
	extension := ".dump"
	if b.config.Profile == "postgresql" || b.config.Profile == "mysql" || b.config.Profile == "nginx" || b.config.Profile == "uptime-kuma" {
		extension = ".age"
	}
	return filepath.Join(root, id+extension), nil
}

func (b *DockerPackageBoundary) checkReleaseAnchor(p PreparedExecution, create bool) error {
	image, err := b.imageConfig(p)
	if err != nil {
		return err
	}
	expected := map[string]string{"profile": b.config.Profile, "project": b.config.Runtime.Project, "manifestDigest": p.Binding().Release.ManifestDigest, "imageConfig": image}
	actual, pending, err := b.readPackageAnchor()
	if os.IsNotExist(err) && create {
		plan, _, parseErr := decodePrepared(p)
		if parseErr != nil {
			return parseErr
		}
		if plan.Operation != "install" {
			return fmt.Errorf("missing release anchor requires explicit reconciliation")
		}
		raw, marshalErr := json.Marshal(expected)
		if marshalErr != nil {
			return marshalErr
		}
		if err = writeExclusiveCheckpoint(filepath.Join(b.config.Runtime.StateDirectory, "instance-release.json"), raw); err != nil {
			return err
		}
		return syncCheckpointDirectory(b.config.Runtime.StateDirectory)
	}
	if err != nil {
		return err
	}
	same := true
	for key, value := range expected {
		if actual[key] != value {
			same = false
		}
	}
	if same {
		if pending != nil && pending.PlanDigest != p.Binding().PlanDigest {
			return fmt.Errorf("unfinished migration requires its original job or approved recovery")
		}
		return nil
	}
	return b.admitPackageTransition(p, actual, expected, create)
}

func (b *DockerPackageBoundary) checkpointManifestDigest(p PreparedExecution) (string, error) {
	anchor, pending, err := b.readPackageAnchor()
	if err != nil {
		return "", err
	}
	if pending != nil && pending.PlanDigest == p.Binding().PlanDigest {
		return pending.From["manifestDigest"], nil
	}
	return anchor["manifestDigest"], nil
}

func (b *DockerPackageBoundary) backup(ctx context.Context, p PreparedExecution, id, kind string) error {
	checkpointDigest, err := b.checkpointManifestDigest(p)
	if err != nil {
		return err
	}
	path, err := b.checkpointPath(id, kind)
	if err != nil {
		return err
	}
	if b.config.Profile == "postgresql" || b.config.Profile == "mysql" || b.config.Profile == "nginx" || b.config.Profile == "uptime-kuma" {
		legacyPath := strings.TrimSuffix(path, ".age") + ".dump"
		if _, err := os.Lstat(legacyPath); err == nil {
			return fmt.Errorf("legacy package checkpoint identity is retained; choose a new identity")
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("package backup identity already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	format := "nginx-config-json"
	payloadFormat := ""
	var recipients []age.Recipient
	switch b.config.Profile {
	case "postgresql":
		format, payloadFormat = "age-v1", "postgres-custom"
		recipients, err = parsePackageAgeRecipients(b.config.BackupEncryption.Recipients)
	case "mysql":
		format, payloadFormat = "age-v1", "mysql-logical"
		recipients, err = parsePackageAgeRecipients(b.config.BackupEncryption.Recipients)
	case "uptime-kuma":
		format, payloadFormat = "age-v1", "volume-tar"
		recipients, err = parsePackageAgeRecipients(b.config.BackupEncryption.Recipients)
	case "nginx":
		format, payloadFormat = "age-v1", "nginx-config-json"
		recipients, err = parsePackageAgeRecipients(b.config.BackupEncryption.Recipients)
	default:
		return fmt.Errorf("package checkpoint profile is not supported")
	}
	if err != nil {
		return fmt.Errorf("package backup encryption is not configured")
	}
	if format == "age-v1" {
		if b.config.BackupEncryption.IdentityFile == "" {
			return fmt.Errorf("package backup encryption is not configured")
		}
		if err := validatePackageAgeKeyPair(b.config.BackupEncryption); err != nil {
			return err
		}
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".package-dump-")
	if err != nil {
		return err
	}
	temporary := f.Name()
	defer os.Remove(temporary)
	defer f.Close()
	h := sha256.New()
	ciphertextWriter := io.MultiWriter(f, h)
	limit := int64(b.shape.Storage) * 1024 * 1024
	var encryptedWriter io.WriteCloser
	var writer *limitedPackageDump
	if format == "age-v1" {
		encryptedWriter, err = age.Encrypt(ciphertextWriter, recipients...)
		if err != nil {
			return fmt.Errorf("package backup encryption could not start")
		}
		writer = &limitedPackageDump{writer: encryptedWriter, remaining: limit}
	} else {
		writer = &limitedPackageDump{writer: ciphertextWriter, remaining: limit}
	}
	switch b.config.Profile {
	case "postgresql":
		_, err = b.client(ctx, "pg_dump", nil, writer, "--dbname=service=wazi", "--no-password", "--format=custom")
	case "mysql":
		_, err = b.client(ctx, "mysqldump", nil, writer, "--defaults-extra-file=/run/secrets/client.conf", "--single-transaction", "--skip-lock-tables", "--no-tablespaces", "--set-gtid-purged=OFF", "wazi")
	case "uptime-kuma":
		err = b.packageVolumeOut(ctx, p, writer)
	case "nginx":
		var raw []byte
		raw, err = b.readNginxConfig()
		if err == nil {
			_, err = writer.Write(raw)
		}
	}
	if err != nil {
		if encryptedWriter != nil {
			_ = encryptedWriter.Close()
		}
		return fmt.Errorf("package checkpoint creation failed: %w", err)
	}
	if encryptedWriter != nil {
		if err := encryptedWriter.Close(); err != nil {
			return fmt.Errorf("package checkpoint encryption finalization failed")
		}
	}
	if err = f.Sync(); err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return fmt.Errorf("empty package checkpoint")
	}
	if err = f.Close(); err != nil {
		return err
	}
	metadata := packageCheckpoint{Format: format, PayloadFormat: payloadFormat, Profile: b.config.Profile, Project: b.config.Runtime.Project, Tenant: p.TenantScope(), ManifestDigest: checkpointDigest, BackupID: id,
		PayloadDigest: fmt.Sprintf("sha256:%x", h.Sum(nil)), CreatedAt: time.Now().UTC().Format(time.RFC3339), Bytes: info.Size()}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	// A data file without a sidecar is never restorable. Persist data first;
	// a crash leaves an occupied ID requiring operator inspection, not reuse.
	if err = os.Rename(temporary, path); err != nil {
		return err
	}
	if err := writeExclusiveCheckpoint(path+".json", raw); err != nil {
		return err
	}
	return syncCheckpointDirectory(filepath.Dir(path))
}

func writeExclusiveCheckpoint(path string, raw []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(raw); err != nil {
		return err
	}
	return f.Sync()
}
func syncCheckpointDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (b *DockerPackageBoundary) verifiedCheckpoint(p PreparedExecution, id string) (*os.File, packageCheckpoint, error) {
	return b.verifiedCheckpointForDigest(p, id, p.Binding().Release.ManifestDigest)
}

func (b *DockerPackageBoundary) verifiedCheckpointForDigest(p PreparedExecution, id, expectedManifest string) (*os.File, packageCheckpoint, error) {
	var record packageCheckpoint
	kind := "backups"
	if strings.HasPrefix(id, "bak_pre_") {
		kind = "pre-restore"
	}
	path, err := b.checkpointPath(id, kind)
	if err != nil {
		return nil, record, err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) && (b.config.Profile == "postgresql" || b.config.Profile == "mysql" || b.config.Profile == "nginx" || b.config.Profile == "uptime-kuma") {
		legacyPath := strings.TrimSuffix(path, ".age") + ".dump"
		if legacyInfo, legacyErr := os.Lstat(legacyPath); legacyErr == nil {
			path, info, err = legacyPath, legacyInfo, nil
		}
	}
	maxBytes := int64(b.shape.Storage) * 1024 * 1024
	if b.config.Profile == "postgresql" || b.config.Profile == "mysql" || b.config.Profile == "nginx" || b.config.Profile == "uptime-kuma" {
		maxBytes = packageAgeCiphertextLimit(maxBytes)
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, record, fmt.Errorf("package checkpoint file invalid")
	}
	meta, err := os.Lstat(path + ".json")
	if err != nil || !meta.Mode().IsRegular() || meta.Size() > 8192 {
		return nil, record, fmt.Errorf("package checkpoint metadata invalid")
	}
	raw, err := os.ReadFile(path + ".json")
	if err != nil {
		return nil, record, err
	}
	if json.Unmarshal(raw, &record) != nil || record.Profile != b.config.Profile || record.Project != b.config.Runtime.Project || record.Tenant != p.TenantScope() || record.ManifestDigest != expectedManifest || record.BackupID != id || record.Bytes != info.Size() {
		return nil, record, fmt.Errorf("package checkpoint binding mismatch; cross-version restore requires a separately qualified migration")
	}
	expectedPayload := map[string]string{"postgresql": "postgres-custom", "mysql": "mysql-logical", "nginx": "nginx-config-json", "uptime-kuma": "volume-tar"}[b.config.Profile]
	if expectedPayload != "" && record.Format == "age-v1" && record.PayloadFormat != expectedPayload {
		return nil, record, fmt.Errorf("encrypted checkpoint payload format mismatch")
	}
	if expectedPayload != "" && record.Format != "age-v1" && record.Format != expectedPayload {
		return nil, record, fmt.Errorf("checkpoint format mismatch")
	}
	if expectedPayload == "" && record.Format != "nginx-config-json" {
		return nil, record, fmt.Errorf("checkpoint format mismatch")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, record, err
	}
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		file.Close()
		return nil, record, err
	}
	if record.PayloadDigest != fmt.Sprintf("sha256:%x", h.Sum(nil)) {
		file.Close()
		return nil, record, fmt.Errorf("package checkpoint payload digest mismatch")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return nil, record, err
	}
	return file, record, nil
}

func (b *DockerPackageBoundary) preflightMySQLImport(ctx context.Context, restoreFile *os.File) error {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return fmt.Errorf("MySQL checkpoint preflight identity could not be created")
	}
	database := "wazi_preflight_" + hex.EncodeToString(nonce[:])
	cleanup := func() error {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		_, err := b.client(cleanupCtx, "mysql", nil, nil,
			"--defaults-extra-file=/run/secrets/client.conf",
			"--execute", "DROP DATABASE IF EXISTS "+database)
		return err
	}
	if _, err := b.client(ctx, "mysql", nil, nil,
		"--defaults-extra-file=/run/secrets/client.conf",
		"--execute", "CREATE DATABASE "+database); err != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return fmt.Errorf("MySQL checkpoint preflight setup and cleanup failed")
		}
		return fmt.Errorf("MySQL checkpoint preflight database could not be created")
	}
	if _, err := b.client(ctx, "mysql", restoreFile, nil,
		"--defaults-extra-file=/run/secrets/client.conf", database); err != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return fmt.Errorf("MySQL checkpoint import preflight and cleanup failed")
		}
		return fmt.Errorf("MySQL checkpoint import preflight failed")
	}
	if _, err := restoreFile.Seek(0, io.SeekStart); err != nil {
		_ = cleanup()
		return err
	}
	if err := cleanup(); err != nil {
		return fmt.Errorf("MySQL checkpoint preflight database cleanup failed")
	}
	return nil
}

func (b *DockerPackageBoundary) restore(ctx context.Context, p PreparedExecution, id string, compose []byte) error {
	file, record, err := b.verifiedCheckpoint(p, id)
	if err != nil {
		return err
	}
	defer file.Close()
	restoreFile := file
	cleanupRestore := func() {}
	if record.Format == "age-v1" {
		restoreFile, cleanupRestore, _, err = stageAgeDecryption(file, filepath.Dir(file.Name()), b.config.BackupEncryption.IdentityFile, int64(b.shape.Storage)*1024*1024)
		if err != nil {
			return fmt.Errorf("package checkpoint decryption failed: %w", err)
		}
		defer cleanupRestore()
	}
	if b.config.Profile == "postgresql" {
		if record.Format != "age-v1" && record.Format != "postgres-custom" {
			return fmt.Errorf("checkpoint format mismatch")
		}
		if _, err := b.client(ctx, "pg_restore", restoreFile, nil, "--list"); err != nil {
			return fmt.Errorf("PostgreSQL checkpoint does not parse")
		}
		if _, err := restoreFile.Seek(0, io.SeekStart); err != nil {
			return err
		}
	}
	envelope, parseErr := parseCanonicalObject(p.Envelope(), "restore envelope")
	if parseErr != nil {
		return parseErr
	}
	strategy, _ := objectValue(envelope["rollbackStrategy"])
	checkpoint, declared := objectValue(strategy["checkpoint"])
	if declared {
		beforeID, _ := checkpoint["backupId"].(string)
		if !strings.HasPrefix(beforeID, "bak_pre_") || beforeID == id || checkpoint["volume"] != b.shape.Volume {
			return fmt.Errorf("pre-restore checkpoint reference invalid")
		}
		beforeDigest, digestErr := b.checkpointManifestDigest(p)
		if digestErr != nil {
			return digestErr
		}
		before, beforeRecord, err := b.verifiedCheckpointForDigest(p, beforeID, beforeDigest)
		if err != nil {
			return fmt.Errorf("declared pre-restore checkpoint required: %w", err)
		}
		if beforeRecord.Format == "age-v1" {
			staged, cleanup, _, decryptErr := stageAgeDecryption(before, filepath.Dir(before.Name()), b.config.BackupEncryption.IdentityFile, int64(b.shape.Storage)*1024*1024)
			if decryptErr != nil {
				before.Close()
				return fmt.Errorf("declared pre-restore checkpoint is not decryptable: %w", decryptErr)
			}
			if b.config.Profile == "postgresql" {
				if _, err := b.client(ctx, "pg_restore", staged, nil, "--list"); err != nil {
					cleanup()
					before.Close()
					return fmt.Errorf("declared pre-restore PostgreSQL checkpoint does not parse")
				}
			}
			cleanup()
		}
		before.Close()
	} else {
		// Retain the adapter's defensive checkpoint for older internal callers.
		// New package plans always declare and create it as an approved step.
		beforeID := fmt.Sprintf("bak_pre_%x", sha256.Sum256([]byte(p.Binding().PlanDigest+id)))
		beforeID = beforeID[:48]
		if err := b.backup(ctx, p, beforeID, "pre-restore"); err != nil {
			return fmt.Errorf("pre-restore checkpoint required: %w", err)
		}
	}
	switch b.config.Profile {
	case "postgresql":
		if record.Format != "age-v1" && record.Format != "postgres-custom" {
			return fmt.Errorf("checkpoint format mismatch")
		}
		if _, err = b.client(ctx, "dropdb", nil, nil, "--maintenance-db=service=wazi dbname=postgres", "--no-password", "--force", "wazi"); err != nil {
			return err
		}
		if _, err = b.client(ctx, "createdb", nil, nil, "--maintenance-db=service=wazi dbname=postgres", "--no-password", "--owner=wazi", "wazi"); err != nil {
			return err
		}
		_, err = b.client(ctx, "pg_restore", restoreFile, nil, "--dbname=service=wazi", "--no-password", "--clean", "--if-exists", "--single-transaction", "--exit-on-error")
	case "mysql":
		if record.Format != "age-v1" && record.Format != "mysql-logical" {
			return fmt.Errorf("checkpoint format mismatch")
		}
		if err = b.preflightMySQLImport(ctx, restoreFile); err != nil {
			return err
		}
		if _, err = b.client(ctx, "mysql", nil, nil, "--defaults-extra-file=/run/secrets/client.conf", "--execute", "DROP DATABASE wazi; CREATE DATABASE wazi"); err != nil {
			return err
		}
		_, err = b.client(ctx, "mysql", restoreFile, nil, "--defaults-extra-file=/run/secrets/client.conf", "wazi")
	case "uptime-kuma":
		if record.Format != "age-v1" {
			return fmt.Errorf("encrypted package volume checkpoint required")
		}
		err = b.packageVolumeIn(ctx, p, restoreFile, compose)
	case "nginx":
		if record.Format != "nginx-config-json" && record.Format != "age-v1" {
			return fmt.Errorf("checkpoint format mismatch")
		}
		var data map[string]string
		raw, readErr := io.ReadAll(io.LimitReader(restoreFile, (4<<20)+1))
		if readErr != nil || len(raw) > 4<<20 || json.Unmarshal(raw, &data) != nil {
			return fmt.Errorf("configuration checkpoint invalid")
		}
		staged, stageErr := os.MkdirTemp(b.config.Runtime.StateDirectory, "nginx-restore-")
		if stageErr != nil {
			return stageErr
		}
		defer os.RemoveAll(staged)
		if err := writeNginxConfig(staged, data); err != nil {
			return err
		}
		// Validate staged bytes before stopping the live instance or replacing
		// any configuration. The bind source is agent-created private state.
		image, resolveErr := b.imageConfig(p)
		if resolveErr != nil {
			return resolveErr
		}
		_, err = b.core.run(ctx, nil, nil, "run", "--rm", "--network", "none", "--memory", "134217728", "--cpus", "0.25", "--cap-drop", "ALL", "--cap-add", "CHOWN", "--security-opt", "no-new-privileges:true", "--pull", "never",
			"--mount", "type=bind,source="+staged+",target=/etc/nginx,readonly", "--entrypoint", "nginx", image, "-t")
		if err != nil {
			return fmt.Errorf("restored nginx configuration does not validate")
		}
		if _, err = b.core.compose(ctx, compose, "down", "--timeout", "30"); err != nil {
			return err
		}
		root := b.core.volumeDir(b.shape.Volume)
		entries, readErr := os.ReadDir(root)
		if readErr != nil {
			return readErr
		}
		for _, entry := range entries {
			if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
				return err
			}
		}
		err = writeNginxConfig(root, data)
	}
	if err != nil {
		return fmt.Errorf("package restore failed; pre-restore checkpoint retained: %w", err)
	}
	return nil
}

func (b *DockerPackageBoundary) imageConfig(p PreparedExecution) (string, error) {
	m, err := parseCanonicalObject(p.Manifest(), "package manifest")
	if err != nil {
		return "", err
	}
	r, _ := objectValue(m["release"])
	images, err := stringObjects(r["images"], "images")
	if err != nil || len(images) != 1 {
		return "", fmt.Errorf("package image binding missing")
	}
	digest, _ := images[0]["digest"].(string)
	id := b.config.Runtime.ImageConfigDigests[digest]
	if id == "" {
		return "", fmt.Errorf("package image not admitted")
	}
	return id, nil
}

func (b *DockerPackageBoundary) readNginxConfig() ([]byte, error) {
	root := b.core.volumeDir(b.shape.Volume)
	files := map[string]string{}
	total := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("configuration contains non-regular entries")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > int64((4<<20)-total) {
			return fmt.Errorf("configuration snapshot exceeds bound")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		total += len(raw)
		if total > 4<<20 || len(files) >= 64 {
			return fmt.Errorf("configuration snapshot exceeds bound")
		}
		files[filepath.ToSlash(relative)] = string(raw)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(files)
}
func writeNginxConfig(root string, files map[string]string) error {
	if len(files) == 0 || len(files) > 64 || files["nginx.conf"] == "" {
		return fmt.Errorf("configuration snapshot missing root config")
	}
	total := 0
	for name, content := range files {
		clean := filepath.ToSlash(filepath.Clean(name))
		if name == "" || clean == "." || clean != name || filepath.IsAbs(name) || strings.HasPrefix(name, "/") || clean == ".." || strings.HasPrefix(clean, "../") || strings.ContainsAny(name, "\\:\x00") {
			return fmt.Errorf("configuration path escapes instance")
		}
		total += len(content)
		if total > 4<<20 {
			return fmt.Errorf("configuration snapshot too large")
		}
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return err
		}
	}
	return nil
}
