//go:build !linux

package agentclient

func socketNotWritableBy(string, int, func() ([]int, error)) bool { return false }
