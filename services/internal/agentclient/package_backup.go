package agentclient

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type packageCheckpoint struct {
	Format, Profile, Project, Tenant, ManifestDigest, BackupID, PayloadDigest, CreatedAt string
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
	return filepath.Join(root, id+".dump"), nil
}

func (b *DockerPackageBoundary) checkReleaseAnchor(p PreparedExecution, create bool) error {
	image, err := b.imageConfig(p)
	if err != nil {
		return err
	}
	expected := map[string]string{"profile": b.config.Profile, "project": b.config.Runtime.Project, "manifestDigest": p.Binding().Release.ManifestDigest, "imageConfig": image}
	path := filepath.Join(b.config.Runtime.StateDirectory, "instance-release.json")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) && create {
		raw, err := json.Marshal(expected)
		if err != nil {
			return err
		}
		if err := writeExclusiveCheckpoint(path, raw); err != nil {
			return err
		}
		return syncCheckpointDirectory(filepath.Dir(path))
	}
	if err != nil {
		return fmt.Errorf("instance data format has not been qualified")
	}
	var actual map[string]string
	if json.Unmarshal(raw, &actual) != nil || len(actual) != len(expected) {
		return fmt.Errorf("instance release anchor invalid")
	}
	for key, value := range expected {
		if actual[key] != value {
			return fmt.Errorf("cross-version package change requires a separately qualified migration")
		}
	}
	return nil
}

func (b *DockerPackageBoundary) backup(ctx context.Context, p PreparedExecution, id, kind string) error {
	path, err := b.checkpointPath(id, kind)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("package backup identity already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".package-dump-")
	if err != nil {
		return err
	}
	temporary := f.Name()
	defer os.Remove(temporary)
	defer f.Close()
	h := sha256.New()
	writer := &limitedPackageDump{writer: io.MultiWriter(f, h), remaining: int64(b.shape.Storage) * 1024 * 1024}
	format := "postgres-custom"
	switch b.config.Profile {
	case "postgresql":
		_, err = b.client(ctx, "pg_dump", nil, writer, "--dbname=service=wazi", "--no-password", "--format=custom")
	case "mysql":
		format = "mysql-logical"
		_, err = b.client(ctx, "mysqldump", nil, writer, "--defaults-extra-file=/run/secrets/client.conf", "--single-transaction", "--skip-lock-tables", "--no-tablespaces", "--set-gtid-purged=OFF", "wazi")
	case "nginx":
		format = "nginx-config-json"
		var raw []byte
		raw, err = b.readNginxConfig()
		if err == nil {
			_, err = writer.Write(raw)
		}
	}
	if err != nil {
		return fmt.Errorf("package checkpoint creation failed: %w", err)
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
	metadata := packageCheckpoint{Format: format, Profile: b.config.Profile, Project: b.config.Runtime.Project, Tenant: p.TenantScope(), ManifestDigest: p.Binding().Release.ManifestDigest, BackupID: id,
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
	if err != nil || !info.Mode().IsRegular() || info.Size() > int64(b.shape.Storage)*1024*1024 {
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
	if json.Unmarshal(raw, &record) != nil || record.Profile != b.config.Profile || record.Project != b.config.Runtime.Project || record.Tenant != p.TenantScope() || record.ManifestDigest != p.Binding().Release.ManifestDigest || record.BackupID != id || record.Bytes != info.Size() {
		return nil, record, fmt.Errorf("package checkpoint binding mismatch; cross-version restore requires a separately qualified migration")
	}
	expectedFormat := map[string]string{"postgresql": "postgres-custom", "mysql": "mysql-logical", "nginx": "nginx-config-json"}[b.config.Profile]
	if record.Format != expectedFormat {
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

func (b *DockerPackageBoundary) restore(ctx context.Context, p PreparedExecution, id string, compose []byte) error {
	file, record, err := b.verifiedCheckpoint(p, id)
	if err != nil {
		return err
	}
	defer file.Close()
	if b.config.Profile == "postgresql" {
		if _, err := b.client(ctx, "pg_restore", file, nil, "--list"); err != nil {
			return fmt.Errorf("PostgreSQL checkpoint does not parse")
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
	}
	beforeID := fmt.Sprintf("bak_pre_%x", sha256.Sum256([]byte(p.Binding().PlanDigest+id)))
	beforeID = beforeID[:48]
	if err := b.backup(ctx, p, beforeID, "pre-restore"); err != nil {
		return fmt.Errorf("pre-restore checkpoint required: %w", err)
	}
	switch b.config.Profile {
	case "postgresql":
		if record.Format != "postgres-custom" {
			return fmt.Errorf("checkpoint format mismatch")
		}
		if _, err = b.client(ctx, "dropdb", nil, nil, "--maintenance-db=service=wazi dbname=postgres", "--no-password", "--force", "wazi"); err != nil {
			return err
		}
		if _, err = b.client(ctx, "createdb", nil, nil, "--maintenance-db=service=wazi dbname=postgres", "--no-password", "--owner=wazi", "wazi"); err != nil {
			return err
		}
		_, err = b.client(ctx, "pg_restore", file, nil, "--dbname=service=wazi", "--no-password", "--clean", "--if-exists", "--single-transaction", "--exit-on-error")
	case "mysql":
		if record.Format != "mysql-logical" {
			return fmt.Errorf("checkpoint format mismatch")
		}
		if _, err = b.client(ctx, "mysql", nil, nil, "--defaults-extra-file=/run/secrets/client.conf", "--execute", "DROP DATABASE wazi; CREATE DATABASE wazi"); err != nil {
			return err
		}
		_, err = b.client(ctx, "mysql", file, nil, "--defaults-extra-file=/run/secrets/client.conf", "wazi")
	case "nginx":
		if record.Format != "nginx-config-json" {
			return fmt.Errorf("checkpoint format mismatch")
		}
		var data map[string]string
		raw, readErr := io.ReadAll(io.LimitReader(file, (4<<20)+1))
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
		_, err = b.core.run(ctx, nil, nil, "run", "--rm", "--network", "none", "--memory", "134217728", "--cpus", "0.25", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true", "--pull", "never",
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
