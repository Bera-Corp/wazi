//go:build linux

package probe

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

func osRelease() (distribution, version string) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "linux", "unknown"
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			fields[key] = strings.Trim(value, "\"")
		}
	}
	distribution = strings.ToLower(fields["ID"])
	version = fields["VERSION_ID"]
	if distribution == "" {
		distribution = "linux"
	}
	if version == "" {
		version = "unknown"
	}
	return distribution, version
}

func totalRAMMb() int64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.ParseInt(fields[1], 10, 64)
				if err == nil {
					return kb / 1024
				}
			}
		}
	}
	return 0
}

func storageFreeMb(path string) int64 {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return 0
	}
	info, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return 0
	}
	mount, err := mountpointForPath(resolved, info)
	if err != nil {
		return 0
	}
	var stat, directory syscall.Statfs_t
	if err := syscall.Statfs(mount, &stat); err != nil {
		return 0
	}
	if err := syscall.Statfs(resolved, &directory); err != nil || stat.Fsid != directory.Fsid || stat.Type != directory.Type {
		return 0
	}
	// Host inventory reports backing-filesystem capacity. An inode's project
	// quota is a different allocation: package admission independently verifies
	// the cache/checkpoint pool and every application volume's hard quota.
	return int64(stat.Bavail) * int64(stat.Bsize) / (1024 * 1024)
}

func mountpointForPath(path string, info []byte) (string, error) {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("storage path must be absolute")
	}
	best := ""
	decode := strings.NewReplacer("\\040", " ", "\\011", "\t", "\\012", "\n", "\\134", "\\")
	for _, line := range strings.Split(string(info), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		target := decode.Replace(fields[4])
		if !filepath.IsAbs(target) || filepath.Clean(target) != target {
			continue
		}
		if path == target || target == "/" || strings.HasPrefix(path, target+"/") {
			if len(target) > len(best) {
				best = target
			}
		}
	}
	if best == "" {
		return "", fmt.Errorf("storage filesystem mount unavailable")
	}
	return best, nil
}

// occupiedPorts reads the kernel socket tables. The IPv4 tables
// are mandatory evidence: if they cannot be read, the probe is
// uncertain and reports an error rather than silently claiming
// ports are free. IPv6 tables are optional (absent when IPv6 is
// disabled) but any other read error is reported too.
func occupiedPorts() ([]int, error) {
	seen := map[int]bool{}
	for _, file := range []string{"/proc/net/tcp", "/proc/net/tcp6", "/proc/net/udp", "/proc/net/udp6"} {
		data, err := os.ReadFile(file)
		if err != nil {
			optional := strings.HasSuffix(file, "6") && os.IsNotExist(err)
			if optional {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", file, err)
		}
		isTCP := strings.Contains(file, "/tcp")
		for _, port := range parseOccupiedPorts(string(data), isTCP) {
			seen[port] = true
		}
	}
	ports := make([]int, 0, len(seen))
	for port := range seen {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports, nil
}

// filesystemTypeFor reports the filesystem type of the mount
// that contains path, so the Docker root's filesystem is
// measured rather than always the root filesystem's.
func filesystemTypeFor(path string) string {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return "unknown"
	}
	return parseFilesystemType(string(data), path)
}

// Writability is proven by probeWritableDir in probe.go (an actual
// create/write/remove), called from Build with the daemon root.
