package agentclient

import (
	"testing"
	"time"
)

func TestLiveSignedSourceWindows(t *testing.T) {
	now := time.Now().UTC()
	for _, test := range []struct {
		name             string
		observed, expiry time.Time
		valid            bool
	}{
		{"valid", now.Add(-time.Minute), now.Add(time.Minute), true},
		{"future", now.Add(time.Minute), now.Add(time.Hour), false},
		{"expired", now.Add(-time.Hour), now.Add(-time.Minute), false},
		{"reversed", now.Add(-time.Minute), now.Add(-time.Hour), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := canonical(t, map[string]any{"provenance": map[string]any{"observedAt": test.observed.Format(time.RFC3339), "expiresAt": test.expiry.Format(time.RFC3339)}})
			err := verifySourceFreshness(now, 0, raw)
			if (err == nil) != test.valid {
				t.Fatalf("source validity=%v, error=%v", test.valid, err)
			}
		})
	}
}
