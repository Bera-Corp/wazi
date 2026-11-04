package agentclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestUptimeHealthRecognizesOnlyFixedDashboardRedirect(t *testing.T) {
	for _, location := range []string{"/dashboard", "/setup-database", "https://example.invalid/dashboard", "//example.invalid/dashboard"} {
		t.Run(location, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Location", location)
				w.WriteHeader(http.StatusFound)
			}))
			defer server.Close()
			err := waitForUptimeHealth(context.Background(), server.URL, 0, time.Millisecond)
			if (err == nil) != (location == "/dashboard") || requests.Load() != 1 {
				t.Fatalf("unexpected redirect acceptance or follow: %v; requests=%d", err, requests.Load())
			}
			if err := waitForHealth(context.Background(), server.URL, 0, time.Millisecond); err == nil {
				t.Fatal("generic profile accepted redirect")
			}
		})
	}
}
