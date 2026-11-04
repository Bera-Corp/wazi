package agentclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUptimeHealthRecognizesOnlyFixedDashboardRedirect(t *testing.T) {
	for _, location := range []string{"/dashboard", "/setup-database", "https://example.invalid/dashboard", "//example.invalid/dashboard"} {
		t.Run(location, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Location", location)
				w.WriteHeader(http.StatusFound)
			}))
			defer server.Close()
			err := waitForUptimeHealth(context.Background(), server.URL, 0, time.Millisecond)
			if (err == nil) != (location == "/dashboard") || requests != 1 {
				t.Fatalf("unexpected redirect acceptance or follow: %v; requests=%d", err, requests)
			}
			if err := waitForHealth(context.Background(), server.URL, 0, time.Millisecond); err == nil {
				t.Fatal("generic profile accepted redirect")
			}
		})
	}
}
