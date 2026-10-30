package agentclient

import (
	"context"
	"io"
	"strings"
	"testing"
)

// The restore helper receives the checkpoint over stdin; without
// docker run -i the tar stream never reaches the container and the
// extraction fails after the volume was cleared. This pins the exact
// helper invocation shape.
func TestDockerVolumeStreamerRestoreForwardsStdin(t *testing.T) {
	var calls [][]string
	boundary := &DockerPilotBoundary{config: DockerPilotConfig{Project: "wazi-gitea-phase1"}}
	boundary.run = func(_ context.Context, _ io.Reader, _ io.Writer, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		return nil, nil
	}
	streamer := boundary.streamer("sha256:image")
	if err := streamer.StreamIn(context.Background(), "gitea-data", strings.NewReader("tar-bytes")); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected extract + chown helper calls, got %d", len(calls))
	}
	extract := strings.Join(calls[0], " ")
	if !strings.Contains(extract, "run --rm -i ") {
		t.Fatalf("restore helper does not forward stdin: %s", extract)
	}
	if !strings.Contains(extract, "--network none") || !strings.Contains(extract, "wazi-gitea-phase1_gitea-data:/wazi-volume") {
		t.Fatalf("restore helper escaped its boundary: %s", extract)
	}
	if err := streamer.StreamIn(context.Background(), "foreign-volume", strings.NewReader("x")); err == nil {
		t.Fatal("foreign volume accepted by streamer")
	}
	if err := streamer.StreamOut(context.Background(), "foreign-volume", io.Discard); err == nil {
		t.Fatal("foreign volume accepted by streamer")
	}
}
