package agentclient

import "testing"

func TestApprovedSharedPoolReallocation(t *testing.T) {
	for _, allocation := range []struct{ application, shared int64 }{{25600, 13}, {27648, 11}} {
		if err := packageSharedPoolCheck(allocation.application, 3*1024*1024, allocation.shared*1024*1024, 2048); err != nil {
			t.Fatal(err)
		}
	}
	for _, input := range []struct{ application, used, hard, checkpoint int64 }{
		{27648, 0, 13 * 1024 * 1024, 2048},
		{25600, 0, 11 * 1024 * 1024, 2048},
		{27649, 0, 11 * 1024 * 1024, 2048},
		{27648, -1, 11 * 1024 * 1024, 2048},
		{27648, 9 * 1024 * 1024, 11 * 1024 * 1024, 2048},
	} {
		if err := packageSharedPoolCheck(input.application, input.used, input.hard, input.checkpoint); err == nil {
			t.Fatalf("invalid allocation or inadequate headroom accepted: %+v", input)
		}
	}
}
