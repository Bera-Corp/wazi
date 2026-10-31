package controlplane

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"time"
)

func (s *Server) pilotEnabled(w http.ResponseWriter, r *http.Request) bool {
	if s.cfg.PilotAgentSocket != "/run/wazi-phase1/agent.sock" || s.cfg.OperatorToken == "" {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "deployment_disabled", "detail": "the bounded pilot runtime is not configured"})
		return false
	}
	return s.requireOperator(w, r)
}

func (s *Server) handlePilotDeployment(w http.ResponseWriter, r *http.Request) {
	if !s.pilotEnabled(w, r) {
		return
	}
	s.beginBodyRead(w)
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_job_body"})
		return
	}
	s.forwardPilot(w, r, http.MethodPost, "/jobs", raw)
}

func (s *Server) handlePilotDeploymentStatus(w http.ResponseWriter, r *http.Request) {
	if !s.pilotEnabled(w, r) {
		return
	}
	id := r.PathValue("jobID")
	if !regexp.MustCompile("^job_[A-Za-z0-9_-]{1,96}$").MatchString(id) {
		writeJSON(w, 400, map[string]string{"error": "invalid_job_id"})
		return
	}
	s.forwardPilot(w, r, http.MethodGet, "/jobs/"+id, nil)
}

func (s *Server) forwardPilot(w http.ResponseWriter, r *http.Request, method, path string, body []byte) {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", s.cfg.PilotAgentSocket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("agent redirects forbidden") }}
	request, err := http.NewRequestWithContext(r.Context(), method, "http://pilot"+path, bytes.NewReader(body))
	if err != nil {
		writeJSON(w, 503, map[string]string{"error": "agent_request_unavailable"})
		return
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		writeJSON(w, 503, map[string]string{"error": "pilot_agent_unavailable"})
		return
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		writeJSON(w, 503, map[string]string{"error": "invalid_agent_response"})
		return
	}
	w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(raw)
}
