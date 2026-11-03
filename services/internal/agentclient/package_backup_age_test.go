package agentclient

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"filippo.io/age"
)

func packageAgeFixture(t *testing.T) (*age.X25519Identity, string, string) {
	t.Helper()
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "identity.txt")
	if err := os.WriteFile(path, []byte(identity.String()+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return identity, identity.Recipient().String(), path
}

func encryptPackageFixture(t *testing.T, recipient string, plaintext []byte) []byte {
	t.Helper()
	parsed, err := age.ParseX25519Recipient(recipient)
	if err != nil {
		t.Fatal(err)
	}
	var ciphertext bytes.Buffer
	encrypted, err := age.Encrypt(&ciphertext, parsed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encrypted.Write(plaintext); err != nil {
		t.Fatal(err)
	}
	if err := encrypted.Close(); err != nil {
		t.Fatal(err)
	}
	return ciphertext.Bytes()
}

func TestStageAgeDecryptionAuthenticatesAndCleansPlaintext(t *testing.T) {
	_, recipient, identityFile := packageAgeFixture(t)
	plaintext := []byte("synthetic database checkpoint")
	ciphertext := encryptPackageFixture(t, recipient, plaintext)
	directory := t.TempDir()
	staged, cleanup, size, err := stageAgeDecryption(bytes.NewReader(ciphertext), directory, identityFile, 1024)
	if err != nil {
		t.Fatal(err)
	}
	path := staged.Name()
	got, err := io.ReadAll(staged)
	if err != nil || !bytes.Equal(got, plaintext) || size != int64(len(plaintext)) {
		t.Fatalf("decrypted checkpoint mismatch: size=%d err=%v", size, err)
	}
	info, err := staged.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("plaintext staging mode is %04o", info.Mode().Perm())
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("plaintext staging file remained after cleanup")
	}
}

func TestStageAgeDecryptionRejectsWrongKeyTamperingTruncationAndOverflow(t *testing.T) {
	_, recipient, identityFile := packageAgeFixture(t)
	_, _, wrongIdentityFile := packageAgeFixture(t)
	plaintext := []byte("synthetic authenticated checkpoint payload")
	ciphertext := encryptPackageFixture(t, recipient, plaintext)

	cases := []struct {
		name       string
		data       []byte
		identity   string
		limitBytes int64
	}{
		{name: "wrong identity", data: ciphertext, identity: wrongIdentityFile, limitBytes: 1024},
		{name: "tampered ciphertext", data: func() []byte { value := append([]byte(nil), ciphertext...); value[len(value)-1] ^= 1; return value }(), identity: identityFile, limitBytes: 1024},
		{name: "truncated ciphertext", data: ciphertext[:len(ciphertext)-1], identity: identityFile, limitBytes: 1024},
		{name: "plaintext exceeds bound", data: ciphertext, identity: identityFile, limitBytes: int64(len(plaintext) - 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			staged, cleanup, _, err := stageAgeDecryption(bytes.NewReader(tc.data), directory, tc.identity, tc.limitBytes)
			if err == nil {
				if cleanup != nil {
					cleanup()
				}
				t.Fatal("invalid encrypted checkpoint was accepted")
			}
			_ = staged
			if entries, readErr := os.ReadDir(directory); readErr != nil || len(entries) != 0 {
				t.Fatal("failed decryption left plaintext staging data")
			}
		})
	}
}

func TestPackageBackupAgeIdentityAndRecipientBinding(t *testing.T) {
	identity, recipient, identityFile := packageAgeFixture(t)
	valid := PackageBackupEncryptionConfig{Recipients: []string{recipient}, IdentityFile: identityFile}
	if err := validatePackageAgeKeyPair(valid); err != nil {
		t.Fatal(err)
	}
	_, _, otherIdentityFile := packageAgeFixture(t)
	other := PackageBackupEncryptionConfig{Recipients: []string{identity.Recipient().String()}, IdentityFile: otherIdentityFile}
	if err := validatePackageAgeKeyPair(other); err == nil {
		t.Fatal("unmatched private identity accepted")
	}
	if _, err := parsePackageAgeRecipients([]string{"ssh-ed25519 AAAA"}); err == nil {
		t.Fatal("SSH recipient accepted where native age keys are required")
	}
}

func TestPackageAgeIdentityFileRejectsSymlinkAndWritableGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("protected identity permissions are qualified on Linux")
	}
	_, _, identityFile := packageAgeFixture(t)
	if err := os.Chmod(identityFile, 0660); err != nil {
		t.Fatal(err)
	}
	if err := validatePackageAgeIdentityFile(identityFile); err == nil {
		t.Fatal("group-writable private identity accepted")
	}
	if err := os.Chmod(identityFile, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "identity-link")
	if err := os.Symlink(identityFile, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := validatePackageAgeIdentityFile(link); err == nil {
		t.Fatal("symlinked private identity accepted")
	}
}
