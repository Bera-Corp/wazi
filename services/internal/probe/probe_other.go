//go:build !linux

package probe

import "runtime"

// Non-Linux fallbacks exist so the module builds and unit tests run on
// developer machines. The Phase 1 supported target is Linux; profiles
// built elsewhere will fail compatibility checks by design.
func osRelease() (string, string)     { return runtime.GOOS, "unknown" }
func totalRAMMb() int64               { return 0 }
func storageFreeMb(string) int64      { return 0 }
func occupiedPorts() ([]int, error)   { return []int{}, nil }
func filesystemTypeFor(string) string { return "unknown" }
