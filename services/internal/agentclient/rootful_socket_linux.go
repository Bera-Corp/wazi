//go:build linux

package agentclient

import (
	"os"
	"syscall"
)

// socketNotWritableBy reports whether the socket at path exists and its
// owner/group/other mode bits deny write access to the given identity.
func socketNotWritableBy(path string, uid int, groups func() ([]int, error)) bool {
	info, err := os.Stat(path)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	mode := info.Mode().Perm()
	if int(stat.Uid) == uid {
		return mode&0o200 == 0
	}
	if groups != nil {
		if gids, err := groups(); err == nil {
			for _, gid := range gids {
				if uint32(gid) == stat.Gid {
					return mode&0o020 == 0
				}
			}
		}
	}
	return mode&0o002 == 0
}
