package agentclient

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestRootfulDenialFromPermissionError(t *testing.T) {
	err := &net.OpError{Op: "dial", Err: &os.SyscallError{Syscall: "connect", Err: fs.ErrPermission}}
	if !rootfulDenialEstablishedFor(filepath.Join(t.TempDir(), "missing.sock"), err, 1002, nil) {
		t.Fatal("permission dial error must establish denial")
	}
}

func TestRootfulDenialNotEstablishedForOtherErrors(t *testing.T) {
	err := fmt.Errorf("connect: %w", errors.New("connection refused"))
	if rootfulDenialEstablishedFor(filepath.Join(t.TempDir(), "missing.sock"), err, 1002, nil) {
		t.Fatal("unrelated dial error without a socket must not establish denial")
	}
}
