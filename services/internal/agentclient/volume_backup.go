package agentclient

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Digest-bound volume checkpoints for the one-app pilot. A backup is an
// uncompressed tar of exactly one approved volume plus a JSON sidecar
// binding its SHA-256. Restore verifies the sidecar before swapping
// volume contents and retains the pre-restore tree under the state
// directory. Backup IDs are immutable: reuse with different bytes fails.

var pilotBackupID = regexp.MustCompile(`^bak_[A-Za-z0-9_-]{1,64}$`)

var pilotVolumes = map[string]bool{"gitea-data": true, "gitea-config": true}

type volumeBackupRecord struct {
	Format   string `json:"format"`
	Volume   string `json:"volume"`
	BackupID string `json:"backupId"`
	SHA256   string `json:"sha256"`
	Bytes    int64  `json:"bytes"`
}

func validatePilotVolume(volume, backupID string) error {
	if err := validatePilotVolumeName(volume); err != nil {
		return err
	}
	if !pilotBackupID.MatchString(backupID) {
		return fmt.Errorf("backupId %q is not an approved checkpoint identity", backupID)
	}
	return nil
}

func validatePilotVolumeName(volume string) error {
	if !pilotVolumes[volume] {
		return fmt.Errorf("volume %q is outside the approved pilot allocation", volume)
	}
	return nil
}

func writeVolumeTar(volumeDir string, out io.Writer) (string, int64, error) {
	hash := sha256.New()
	counting := &countingWriter{w: io.MultiWriter(out, hash)}
	tw := tar.NewWriter(counting)
	var entries []string
	err := filepath.WalkDir(volumeDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != volumeDir {
			rel, relErr := filepath.Rel(volumeDir, path)
			if relErr != nil {
				return relErr
			}
			entries = append(entries, rel)
		}
		return nil
	})
	if err != nil {
		return "", 0, err
	}
	sort.Strings(entries)
	for _, rel := range entries {
		path := filepath.Join(volumeDir, rel)
		info, err := os.Lstat(path)
		if err != nil {
			return "", 0, err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return "", 0, err
		}
		header.Name = filepath.ToSlash(rel)
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return "", 0, err
			}
			if filepath.IsAbs(target) {
				return "", 0, fmt.Errorf("volume contains an absolute symlink %q", rel)
			}
			header.Linkname = target
		}
		if err := tw.WriteHeader(header); err != nil {
			return "", 0, err
		}
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return "", 0, err
			}
			_, copyErr := io.Copy(tw, file)
			closeErr := file.Close()
			if copyErr != nil {
				return "", 0, copyErr
			}
			if closeErr != nil {
				return "", 0, closeErr
			}
		}
	}
	if err := tw.Close(); err != nil {
		return "", 0, err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), counting.n, nil
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// pilotVolumeStreamer moves one approved volume's bytes as a tar
// stream. The Docker boundary implements it with a one-off helper
// container: in rootless Docker the volume files belong to the
// user-namespace-mapped application UID, which the agent process can
// neither read nor write directly.
type pilotVolumeStreamer interface {
	StreamOut(ctx context.Context, volume string, w io.Writer) error
	StreamIn(ctx context.Context, volume string, r io.Reader) error
}

func backupVolumeStream(ctx context.Context, streamer pilotVolumeStreamer, backupsRoot, volume, backupID string) error {
	if err := validatePilotVolume(volume, backupID); err != nil {
		return err
	}
	dir := filepath.Join(backupsRoot, backupID)
	tarPath := filepath.Join(dir, volume+".tar")
	recordPath := filepath.Join(dir, volume+".json")
	if _, err := os.Stat(tarPath); err == nil {
		record, readErr := readBackupRecord(recordPath)
		if readErr != nil {
			return readErr
		}
		if record.Volume != volume || record.BackupID != backupID {
			return fmt.Errorf("existing checkpoint identity does not match its request")
		}
		if err := verifyFileDigest(tarPath, record.SHA256); err != nil {
			return err
		}
		return nil // immutable checkpoint already recorded; replay is a no-op
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeStreamedCheckpoint(ctx, streamer, volume, tarPath, recordPath, backupID)
}

func streamVolumeDigest(ctx context.Context, streamer pilotVolumeStreamer, volume string) (string, error) {
	hash := sha256.New()
	if err := streamer.StreamOut(ctx, volume, hash); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func readBackupRecord(path string) (volumeBackupRecord, error) {
	var record volumeBackupRecord
	raw, err := os.ReadFile(path)
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		return record, fmt.Errorf("checkpoint record is invalid: %w", err)
	}
	if record.Format != "wazi-volume-tar-v1" {
		return record, fmt.Errorf("checkpoint record format is not approved")
	}
	return record, nil
}

func verifyFileDigest(path, want string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if "sha256:"+hex.EncodeToString(hash.Sum(nil)) != want {
		return fmt.Errorf("checkpoint bytes do not match their recorded digest")
	}
	return nil
}

type volumeRestoreRequest struct {
	VolumeDir      string
	BackupsRoot    string
	PreRestoreRoot string
	Volume         string
	BackupID       string
}

// restoreVolumeStream verifies the checkpoint digest, retains the
// current volume bytes as a digest-bound pre-restore checkpoint, and
// streams the checkpoint into the volume through the streamer (which
// clears the volume first). A failed extraction streams the retained
// bytes back. The volume directory itself is never renamed: in
// rootless Docker it is a daemon-managed mountpoint.
func restoreVolumeStream(ctx context.Context, streamer pilotVolumeStreamer, req volumeRestoreRequest) error {
	if err := validatePilotVolume(req.Volume, req.BackupID); err != nil {
		return err
	}
	dir := filepath.Join(req.BackupsRoot, req.BackupID)
	record, err := readBackupRecord(filepath.Join(dir, req.Volume+".json"))
	if err != nil {
		return err
	}
	if record.Volume != req.Volume || record.BackupID != req.BackupID {
		return fmt.Errorf("checkpoint identity does not match its request")
	}
	tarPath := filepath.Join(dir, req.Volume+".tar")
	if err := verifyFileDigest(tarPath, record.SHA256); err != nil {
		return err
	}
	if _, err := os.Stat(req.VolumeDir); err != nil {
		return fmt.Errorf("approved volume %q is not present", req.Volume)
	}
	if current, err := streamVolumeDigest(ctx, streamer, req.Volume); err == nil && current == record.SHA256 {
		return nil // already restored; replay is a no-op
	}
	preTar := filepath.Join(req.PreRestoreRoot, req.BackupID+"-"+req.Volume+".tar")
	preRecord := filepath.Join(req.PreRestoreRoot, req.BackupID+"-"+req.Volume+".json")
	if _, err := os.Stat(preTar); err == nil {
		return fmt.Errorf("a pre-restore copy already exists for this checkpoint; operator reconciliation is required")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := writeStreamedCheckpoint(ctx, streamer, req.Volume, preTar, preRecord, req.BackupID+"-pre-restore"); err != nil {
		return fmt.Errorf("retain pre-restore checkpoint: %w", err)
	}
	putBack := func() error {
		retained, openErr := os.Open(preTar)
		if openErr != nil {
			return openErr
		}
		defer retained.Close()
		return streamer.StreamIn(ctx, req.Volume, retained)
	}
	checkpoint, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("open checkpoint: %w", err)
	}
	streamErr := streamer.StreamIn(ctx, req.Volume, checkpoint)
	closeErr := checkpoint.Close()
	if streamErr == nil {
		streamErr = closeErr
	}
	if streamErr != nil {
		if rollbackErr := putBack(); rollbackErr != nil {
			return fmt.Errorf("restore extraction failed (%v) and pre-restore recovery also failed: %w", streamErr, rollbackErr)
		}
		return fmt.Errorf("restore extraction failed; pre-restore checkpoint was streamed back: %w", streamErr)
	}
	return nil
}

// writeStreamedCheckpoint records one volume's streamed tar bytes as a
// digest-bound checkpoint file pair (tar + JSON sidecar).
func writeStreamedCheckpoint(ctx context.Context, streamer pilotVolumeStreamer, volume, tarPath, recordPath, backupID string) error {
	if err := os.MkdirAll(filepath.Dir(tarPath), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(tarPath), ".checkpoint-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	hash := sha256.New()
	counting := &countingWriter{w: io.MultiWriter(tmp, hash)}
	if err := streamer.StreamOut(ctx, volume, counting); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	record := volumeBackupRecord{Format: "wazi-volume-tar-v1", Volume: volume, BackupID: backupID, SHA256: "sha256:" + hex.EncodeToString(hash.Sum(nil)), Bytes: counting.n}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if err := os.WriteFile(recordPath, append(raw, '\n'), 0o640); err != nil {
		return err
	}
	return os.Rename(tmpName, tarPath)
}

func extractVolumeTar(tarPath, dest string) error {
	file, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer file.Close()
	reader := tar.NewReader(file)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("checkpoint tar is invalid: %w", err)
		}
		name := header.Name
		if name == "" || filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
			return fmt.Errorf("checkpoint contains an absolute or empty path")
		}
		clean := filepath.Clean(name)
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("checkpoint contains a path escaping the volume")
		}
		target := filepath.Join(dest, clean)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, fs.FileMode(header.Mode).Perm()); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fs.FileMode(header.Mode).Perm())
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, reader)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			if err := os.Chtimes(target, header.ModTime, header.ModTime); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if filepath.IsAbs(header.Linkname) {
				return fmt.Errorf("checkpoint contains an absolute symlink")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		default:
			return fmt.Errorf("checkpoint contains an unsupported entry type %d", header.Typeflag)
		}
	}
}
