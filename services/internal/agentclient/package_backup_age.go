package agentclient

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"filippo.io/age"
)

const (
	maxAgeBackupRecipients = 16
	maxAgeIdentityFile     = 64 << 10
	ageHeaderAllowance     = 64 << 10
)

// The age STREAM payload uses 64 KiB plaintext chunks and a 16-byte tag
// per chunk. Keep a separate bounded allowance for recipient headers and nonce.
func packageAgeCiphertextLimit(plaintextBytes int64) int64 {
	chunks := (plaintextBytes + (64 << 10) - 1) / (64 << 10)
	if chunks == 0 {
		chunks = 1
	}
	return plaintextBytes + chunks*16 + ageHeaderAllowance
}

// PackageBackupEncryptionConfig contains public age recipients and a
// protected reference to the private restore identity. Private key bytes
// never belong in operator JSON or a request.
type PackageBackupEncryptionConfig struct {
	Recipients   []string
	IdentityFile string
}

func validatePackageBackupEncryption(config PackageBackupEncryptionConfig, project string) error {
	if len(config.Recipients) == 0 && config.IdentityFile == "" {
		return nil
	}
	if len(config.Recipients) == 0 || config.IdentityFile == "" {
		return fmt.Errorf("package backup encryption requires recipients and a protected identity reference")
	}
	root := filepath.Clean("/srv/wazi-runtime/secrets/" + project)
	path := filepath.Clean(config.IdentityFile)
	prefix := root + string(os.PathSeparator)
	if path != config.IdentityFile || !filepath.IsAbs(path) || !strings.HasPrefix(path, prefix) {
		return fmt.Errorf("package backup identity reference is outside its protected instance")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil || filepath.Clean(resolvedRoot) != root {
		return fmt.Errorf("package backup secret directory must not traverse symlinks")
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil || filepath.Clean(resolvedPath) != path {
		return fmt.Errorf("package backup identity reference must not traverse symlinks")
	}
	if err := validatePackageAgeIdentityFile(path); err != nil {
		return err
	}
	return validatePackageAgeKeyPair(config)
}

func validatePackageAgeKeyPair(config PackageBackupEncryptionConfig) error {
	if config.IdentityFile == "" {
		return fmt.Errorf("package backup identity reference is missing")
	}
	if _, err := parsePackageAgeRecipients(config.Recipients); err != nil {
		return err
	}
	identities, err := readPackageAgeIdentities(config.IdentityFile)
	if err != nil {
		return err
	}
	matchingRecipients := make(map[string]bool, len(config.Recipients))
	for _, value := range config.Recipients {
		matchingRecipients[value] = true
	}
	for _, identity := range identities {
		if native, ok := identity.(*age.X25519Identity); ok && matchingRecipients[native.Recipient().String()] {
			return nil
		}
	}
	return fmt.Errorf("package backup identity does not match any configured recipient")
}

func parsePackageAgeRecipients(values []string) ([]age.Recipient, error) {
	if len(values) == 0 || len(values) > maxAgeBackupRecipients {
		return nil, fmt.Errorf("package backup requires between 1 and %d age recipients", maxAgeBackupRecipients)
	}
	recipients := make([]age.Recipient, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != value || value == "" {
			return nil, fmt.Errorf("package backup recipient is invalid")
		}
		recipient, err := age.ParseX25519Recipient(value)
		if err != nil || recipient.String() != value || seen[value] {
			return nil, fmt.Errorf("package backup recipient is invalid or duplicated")
		}
		seen[value] = true
		recipients = append(recipients, recipient)
	}
	return recipients, nil
}

func validatePackageAgeIdentityFile(path string) error {
	info, err := os.Lstat(path)
	unsafePermissions := err == nil && runtime.GOOS != "windows" && info.Mode().Perm()&0027 != 0
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || unsafePermissions || info.Size() <= 0 || info.Size() > maxAgeIdentityFile {
		return fmt.Errorf("package backup identity file must be a protected, bounded regular file")
	}
	return nil
}

func readPackageAgeIdentities(path string) ([]age.Identity, error) {
	if err := validatePackageAgeIdentityFile(path); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("package backup identity is unavailable")
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, maxAgeIdentityFile+1))
	scanner.Buffer(make([]byte, 1024), maxAgeIdentityFile)
	identities := []age.Identity{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		identity, err := age.ParseX25519Identity(line)
		if err != nil || identity.String() != line {
			return nil, fmt.Errorf("package backup identity file contains an unsupported identity")
		}
		identities = append(identities, identity)
		if len(identities) > maxAgeBackupRecipients {
			return nil, fmt.Errorf("package backup identity file exceeds its key bound")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("package backup identity file is unreadable")
	}
	if len(identities) == 0 {
		return nil, fmt.Errorf("package backup identity file has no native X25519 identity")
	}
	return identities, nil
}

// stageAgeDecryption fully authenticates a ciphertext before returning a
// private, bounded plaintext file to a restore caller.
func stageAgeDecryption(source io.Reader, directory, identityFile string, maxBytes int64) (*os.File, func(), int64, error) {
	if maxBytes <= 0 {
		return nil, nil, 0, fmt.Errorf("package restore plaintext limit is invalid")
	}
	identities, err := readPackageAgeIdentities(identityFile)
	if err != nil {
		return nil, nil, 0, err
	}
	plaintext, err := age.Decrypt(source, identities...)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("package backup cannot be decrypted with the configured identities")
	}
	staged, err := os.CreateTemp(directory, ".restore-plaintext-")
	if err != nil {
		return nil, nil, 0, err
	}
	cleanup := func() {
		_ = staged.Close()
		_ = os.Remove(staged.Name())
	}
	limited := &limitedPackageDump{writer: staged, remaining: maxBytes}
	n, err := io.Copy(limited, plaintext)
	if err != nil {
		cleanup()
		return nil, nil, 0, fmt.Errorf("package backup authentication or plaintext bound failed")
	}
	if n == 0 {
		cleanup()
		return nil, nil, 0, fmt.Errorf("decrypted package backup is empty")
	}
	if err := staged.Sync(); err != nil {
		cleanup()
		return nil, nil, 0, err
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, nil, 0, err
	}
	return staged, cleanup, n, nil
}
