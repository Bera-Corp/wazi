package agentclient

import (
	"archive/tar"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeVolumeStreamer moves volume bytes with the filesystem tar
// helpers, standing in for the Docker helper-container streamer so the
// checkpoint semantics are tested without a daemon.
type fakeVolumeStreamer struct {
	dirs map[string]string
}

func (f fakeVolumeStreamer) StreamOut(_ context.Context, volume string, w io.Writer) error {
	_, _, err := writeVolumeTar(f.dirs[volume], w)
	return err
}

func (f fakeVolumeStreamer) StreamIn(_ context.Context, volume string, r io.Reader) error {
	tmp, err := os.CreateTemp("", "wazi-streamin-*.tar")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Mirror the Docker streamer's clear-first semantics.
	entries, err := os.ReadDir(f.dirs[volume])
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(f.dirs[volume], entry.Name())); err != nil {
			return err
		}
	}
	return extractVolumeTar(tmp.Name(), f.dirs[volume])
}

func TestVolumeBackupRestoreRoundtrip(t *testing.T) {
	root := t.TempDir()
	volumeDir := filepath.Join(root, "volumes", "_data")
	backups := filepath.Join(root, "state", "backups")
	pre := filepath.Join(root, "state", "pre")
	if err := os.MkdirAll(filepath.Join(volumeDir, "data"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(volumeDir, "data", "gitea.db"), []byte("known-data-v1"), 0o640); err != nil {
		t.Fatal(err)
	}
	streamer := fakeVolumeStreamer{dirs: map[string]string{"gitea-data": volumeDir}}
	ctx := context.Background()
	if err := backupVolumeStream(ctx, streamer, backups, "gitea-data", "bak_pilot1"); err != nil {
		t.Fatal(err)
	}
	if err := backupVolumeStream(ctx, streamer, backups, "gitea-data", "bak_pilot1"); err != nil {
		t.Fatal("immutable backup replay must be a no-op:", err)
	}
	record, err := readBackupRecord(filepath.Join(backups, "bak_pilot1", "gitea-data.json"))
	if err != nil || record.SHA256 == "" || record.Bytes == 0 {
		t.Fatalf("checkpoint record not digest-bound: %+v, %v", record, err)
	}
	if err := os.WriteFile(filepath.Join(volumeDir, "data", "gitea.db"), []byte("mutated"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(volumeDir, "data", "stray.txt"), []byte("post-backup"), 0o640); err != nil {
		t.Fatal(err)
	}
	req := volumeRestoreRequest{VolumeDir: volumeDir, BackupsRoot: backups, PreRestoreRoot: pre, Volume: "gitea-data", BackupID: "bak_pilot1"}
	if err := restoreVolumeStream(ctx, streamer, req); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(volumeDir, "data", "gitea.db"))
	if err != nil || string(raw) != "known-data-v1" {
		t.Fatalf("restored data = %q, %v", raw, err)
	}
	if _, err := os.Stat(filepath.Join(volumeDir, "data", "stray.txt")); !os.IsNotExist(err) {
		t.Fatal("post-backup stray file survived the restore")
	}
	preRecord, err := readBackupRecord(filepath.Join(pre, "bak_pilot1-gitea-data.json"))
	if err != nil || preRecord.Volume != "gitea-data" || preRecord.SHA256 == "" {
		t.Fatalf("pre-restore checkpoint was not retained: %+v, %v", preRecord, err)
	}
	if err := verifyFileDigest(filepath.Join(pre, "bak_pilot1-gitea-data.tar"), preRecord.SHA256); err != nil {
		t.Fatal("pre-restore checkpoint is not digest-bound:", err)
	}
	if err := restoreVolumeStream(ctx, streamer, req); err != nil {
		t.Fatal("already-restored replay must be a no-op:", err)
	}
}

func TestVolumeBackupRejectsForeignVolumeAndBadID(t *testing.T) {
	streamer := fakeVolumeStreamer{dirs: map[string]string{}}
	if err := backupVolumeStream(context.Background(), streamer, t.TempDir(), "other-data", "bak_pilot1"); err == nil {
		t.Fatal("foreign volume accepted")
	}
	if err := backupVolumeStream(context.Background(), streamer, t.TempDir(), "gitea-data", "../escape"); err == nil {
		t.Fatal("traversal backupId accepted")
	}
}

func TestRestoreRejectsTamperedCheckpoint(t *testing.T) {
	root := t.TempDir()
	volumeDir := filepath.Join(root, "_data")
	backups := filepath.Join(root, "backups")
	if err := os.MkdirAll(volumeDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(volumeDir, "f"), []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	streamer := fakeVolumeStreamer{dirs: map[string]string{"gitea-config": volumeDir}}
	ctx := context.Background()
	if err := backupVolumeStream(ctx, streamer, backups, "gitea-config", "bak_pilot2"); err != nil {
		t.Fatal(err)
	}
	tarPath := filepath.Join(backups, "bak_pilot2", "gitea-config.tar")
	raw, _ := os.ReadFile(tarPath)
	if err := os.WriteFile(tarPath, append(raw, 0), 0o640); err != nil {
		t.Fatal(err)
	}
	req := volumeRestoreRequest{VolumeDir: volumeDir, BackupsRoot: backups, PreRestoreRoot: filepath.Join(root, "p"), Volume: "gitea-config", BackupID: "bak_pilot2"}
	if err := restoreVolumeStream(ctx, streamer, req); err == nil {
		t.Fatal("tampered checkpoint restored")
	}
	// The tampered restore must not have disturbed the live volume.
	rawF, err := os.ReadFile(filepath.Join(volumeDir, "f"))
	if err != nil || string(rawF) != "x" {
		t.Fatalf("live volume disturbed by rejected restore: %q, %v", rawF, err)
	}
}

func TestExtractRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	tarPath := filepath.Join(root, "bad.tar")
	file, err := os.Create(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(file)
	if err := tw.WriteHeader(&tar.Header{Name: "../escape", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	file.Close()
	err = extractVolumeTar(tarPath, filepath.Join(root, "dest"))
	if err == nil || !strings.Contains(err.Error(), "escaping") {
		t.Fatalf("traversal not rejected: %v", err)
	}
}
