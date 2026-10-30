package pairing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Durable registry storage. When a store path is configured, every
// mutation (code minted/consumed, enrollment, revocation, rotation)
// is persisted atomically (stage + rename) before success is
// reported, so a restart cannot resurrect a revoked node, replay a
// consumed code, or lose a revocation epoch. Pairing codes are
// stored only as SHA-256 hashes: the file is not a credential list.

type codeState struct {
	Hash        string    `json:"hash"`
	Tenant      string    `json:"tenantScope"`
	HostID      string    `json:"hostId"`
	CreatedAt   time.Time `json:"createdAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
	Generation  uint64    `json:"generation"`
	Used        bool      `json:"used"`
	Invalidated bool      `json:"invalidated"`
}

type registryState struct {
	Codes       []codeState       `json:"codes"`
	Nodes       []Node            `json:"nodes"`
	Generations map[string]uint64 `json:"generations"`
}

func hashCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// NewRegistryWithStore creates a registry backed by path, taking
// the single-writer lock for that store first: the registry mutex
// only coordinates one process, so a second control plane over
// the same file is refused until the first Closes (or an operator
// removes a stale lock after proving no writer is alive). An
// existing file is loaded; a missing file starts empty and is
// created on first mutation. Load failures are fatal to the
// caller: starting empty over a corrupt store could resurrect
// revoked identities, so the control plane must refuse to start.
func NewRegistryWithStore(now func() time.Time, path string) (*Registry, error) {
	r := NewRegistry(now)
	r.storePath = path
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	lockPath := path + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("registry store %s is already held by another writer (lock %s); if no control plane is running, verify that and remove the lock file", path, lockPath)
		}
		return nil, err
	}
	_, _ = lockFile.WriteString(fmt.Sprintf("pid %d\n", os.Getpid()))
	_ = lockFile.Sync()
	r.storeLock = lockFile
	r.lockPath = lockPath
	releaseOnError := func(err error) (*Registry, error) {
		_ = r.Close()
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return releaseOnError(err)
	}
	var state registryState
	if err := json.Unmarshal(data, &state); err != nil {
		return releaseOnError(fmt.Errorf("registry store %s is corrupt, refusing to start empty: %w", path, err))
	}
	for _, cs := range state.Codes {
		r.codes[cs.Hash] = &pairingCode{
			tenant: cs.Tenant, hostID: cs.HostID, createdAt: cs.CreatedAt,
			expiresAt: cs.ExpiresAt, generation: cs.Generation,
			used: cs.Used, invalidated: cs.Invalidated,
		}
	}
	for i := range state.Nodes {
		node := state.Nodes[i]
		r.nodes[nodeKey(node.TenantScope, node.HostID)] = &node
		r.byFingerprint[node.Fingerprint] = nodeKey(node.TenantScope, node.HostID)
	}
	for key, gen := range state.Generations {
		r.generations[key] = gen
	}
	return r, nil
}

// StorePath returns the configured persistence path ("" = memory only).
func (r *Registry) StorePath() string { return r.storePath }

// Close releases the single-writer store lock. In-memory
// registries have nothing to release.
func (r *Registry) Close() error {
	if r.storeLock == nil {
		return nil
	}
	lockPath := r.lockPath
	err := r.storeLock.Close()
	r.storeLock = nil
	r.lockPath = ""
	if removeErr := os.Remove(lockPath); err == nil {
		err = removeErr
	}
	return err
}

// syncDirBestEffort fsyncs a directory so a rename inside it is
// durable across power loss where the platform supports it.
// Windows does not support directory sync; there it is a no-op and
// crash durability relies on the file sync plus rename alone,
// which is a documented qualification limit, not a guarantee.
func syncDirBestEffort(dir string) {
	handle, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = handle.Sync()
	_ = handle.Close()
}

// saveLocked persists the registry. Caller must hold r.mu.
func (r *Registry) saveLocked() error {
	if r.storePath == "" {
		return nil
	}
	state := registryState{Generations: map[string]uint64{}}
	for hash, entry := range r.codes {
		state.Codes = append(state.Codes, codeState{
			Hash: hash, Tenant: entry.tenant, HostID: entry.hostID,
			CreatedAt: entry.createdAt, ExpiresAt: entry.expiresAt,
			Generation: entry.generation, Used: entry.used, Invalidated: entry.invalidated,
		})
	}
	sort.Slice(state.Codes, func(i, j int) bool { return state.Codes[i].Hash < state.Codes[j].Hash })
	for key, node := range r.nodes {
		state.Nodes = append(state.Nodes, *node)
		_ = key
	}
	sort.Slice(state.Nodes, func(i, j int) bool {
		return nodeKey(state.Nodes[i].TenantScope, state.Nodes[i].HostID) < nodeKey(state.Nodes[j].TenantScope, state.Nodes[j].HostID)
	})
	for key, gen := range r.generations {
		state.Generations[key] = gen
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(r.storePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".wazi-registry-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, r.storePath); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	syncDirBestEffort(dir)
	return nil
}
