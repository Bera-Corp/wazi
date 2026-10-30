package agentclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// A freshly started application answers connection errors and non-200
// responses before its web server is ready; the health gate must keep
// polling and only succeed on a real 200.
func TestWaitForHealthPollsUntilReady(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if err := waitForHealth(context.Background(), server.URL, 5*time.Second, 10*time.Millisecond); err != nil {
		t.Fatalf("health eventually ready: %v", err)
	}
	if attempts.Load() < 3 {
		t.Fatalf("expected retries before success, got %d attempts", attempts.Load())
	}
}

func TestWaitForHealthNeverPassesOnPersistentFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	if err := waitForHealth(context.Background(), server.URL, 60*time.Millisecond, 10*time.Millisecond); err == nil {
		t.Fatal("persistent 503 reported healthy")
	}
}

func TestWaitForHealthRejectsRedirects(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirector.Close()
	if err := waitForHealth(context.Background(), redirector.URL, 60*time.Millisecond, 10*time.Millisecond); err == nil {
		t.Fatal("redirected health response reported healthy")
	}
}

func TestWaitForHealthHonoursContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	unreachable := fmt.Sprintf("http://127.0.0.1:%d/", 1)
	if err := waitForHealth(ctx, unreachable, time.Minute, time.Minute); err == nil {
		t.Fatal("cancelled context reported healthy")
	}
}
