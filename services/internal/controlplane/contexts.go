package controlplane

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"time"
)

//go:embed assets/context.html assets/context.js
var contextAssets embed.FS

type contextSession struct {
	Expires   time.Time
	CSRF      string
	Bootstrap bool
}

func contextNonce() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
func contextKey(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

func (s *Server) contextAvailable(w http.ResponseWriter, r *http.Request) bool {
	if s.cfg.PilotAgentSocket != "/run/wazi-phase1/agent.sock" || s.cfg.OperatorToken == "" || r.TLS == nil || !isLoopbackRemote(r.RemoteAddr) {
		writeJSON(w, 403, map[string]string{"error": "private_https_context_required"})
		return false
	}
	return true
}

func (s *Server) contextAuthorized(w http.ResponseWriter, r *http.Request) bool {
	if !s.contextAvailable(w, r) {
		return false
	}
	if supplied := r.Header.Get("X-Wazi-Operator-Token"); supplied != "" {
		return s.requireOperator(w, r)
	}
	cookie, err := r.Cookie("__Host-wazi-context")
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "context_session_required"})
		return false
	}
	s.mu.Lock()
	value, found := s.contextSessions[contextKey(cookie.Value)]
	s.mu.Unlock()
	if !found || value.Bootstrap || !s.cfg.Now().Before(value.Expires) {
		writeJSON(w, 401, map[string]string{"error": "context_session_expired"})
		return false
	}
	if r.Method != http.MethodGet && (r.Header.Get("Origin") != "https://"+r.Host || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Wazi-CSRF")), []byte(value.CSRF)) != 1) {
		writeJSON(w, 403, map[string]string{"error": "context_csrf_rejected"})
		return false
	}
	return true
}

func (s *Server) registerContextRoutes(mux *http.ServeMux) {
	for _, item := range []struct{ Route, Path, Type string }{
		{"GET /context", "assets/context.html", "text/html; charset=utf-8"},
		{"GET /context.js", "assets/context.js", "text/javascript; charset=utf-8"},
	} {
		item := item
		mux.HandleFunc(item.Route, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", item.Type)
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; connect-src 'self'; img-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
			raw, _ := contextAssets.ReadFile(item.Path)
			_, _ = w.Write(raw)
		})
	}
	mux.HandleFunc("POST /v1/context-sessions", func(w http.ResponseWriter, r *http.Request) {
		if !s.contextAvailable(w, r) || !s.requireOperator(w, r) {
			return
		}
		nonce, err := contextNonce()
		if err != nil {
			writeJSON(w, 503, map[string]string{"error": "session_unavailable"})
			return
		}
		s.mu.Lock()
		for k, v := range s.contextSessions {
			if !s.cfg.Now().Before(v.Expires) {
				delete(s.contextSessions, k)
			}
		}
		if len(s.contextSessions) >= 100 {
			s.mu.Unlock()
			writeJSON(w, 429, map[string]string{"error": "session_capacity"})
			return
		}
		s.contextSessions[contextKey(nonce)] = contextSession{Expires: s.cfg.Now().Add(time.Minute), Bootstrap: true}
		s.mu.Unlock()
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 201, map[string]string{"bootstrapCode": nonce, "expiresIn": "60 seconds"})
	})
	mux.HandleFunc("POST /v1/context-sessions/exchange", func(w http.ResponseWriter, r *http.Request) {
		if !s.contextAvailable(w, r) {
			return
		}
		if r.Header.Get("Origin") != "https://"+r.Host {
			writeJSON(w, 403, map[string]string{"error": "origin_rejected"})
			return
		}
		s.beginBodyRead(w)
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
		var body struct{ Code string }
		if err != nil || json.Unmarshal(raw, &body) != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid_session_body"})
			return
		}
		nonce, err := contextNonce()
		if err != nil {
			writeJSON(w, 503, map[string]string{"error": "session_unavailable"})
			return
		}
		csrf, err := contextNonce()
		if err != nil {
			writeJSON(w, 503, map[string]string{"error": "session_unavailable"})
			return
		}
		s.mu.Lock()
		bootstrap, found := s.contextSessions[contextKey(body.Code)]
		if !found || !bootstrap.Bootstrap || !s.cfg.Now().Before(bootstrap.Expires) {
			s.mu.Unlock()
			writeJSON(w, 401, map[string]string{"error": "bootstrap_rejected"})
			return
		}
		delete(s.contextSessions, contextKey(body.Code))
		s.contextSessions[contextKey(nonce)] = contextSession{Expires: s.cfg.Now().Add(15 * time.Minute), CSRF: csrf}
		s.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "__Host-wazi-context", Value: nonce, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 900})
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]string{"csrf": csrf, "principal": "configured pilot operator", "expiresIn": "15 minutes"})
	})
	mux.HandleFunc("POST /v1/contexts/retrieve", func(w http.ResponseWriter, r *http.Request) {
		if !s.contextAuthorized(w, r) {
			return
		}
		s.beginBodyRead(w)
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid_context_body"})
			return
		}
		s.forwardPilot(w, r, http.MethodPost, "/contexts/retrieve", raw)
	})
	mux.HandleFunc("GET /v1/contexts/{contextID}", func(w http.ResponseWriter, r *http.Request) {
		if !s.contextAuthorized(w, r) {
			return
		}
		id := r.PathValue("contextID")
		if !regexp.MustCompile("^ctx_[0-9a-f]{64}$").MatchString(id) {
			writeJSON(w, 400, map[string]string{"error": "invalid_context_id"})
			return
		}
		s.forwardPilot(w, r, http.MethodGet, "/contexts/"+id, nil)
	})
	// Session authority is the same single pilot operator that issued its
	// bootstrap code. It cannot issue approvals on behalf of other principals.
	mux.HandleFunc("POST /v1/context-deployments", func(w http.ResponseWriter, r *http.Request) {
		if !s.contextAuthorized(w, r) {
			return
		}
		s.beginBodyRead(w)
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid_job_body"})
			return
		}
		s.forwardPilot(w, r, http.MethodPost, "/jobs", raw)
	})
	mux.HandleFunc("GET /v1/context-deployments/{jobID}", func(w http.ResponseWriter, r *http.Request) {
		if !s.contextAuthorized(w, r) {
			return
		}
		id := r.PathValue("jobID")
		if !regexp.MustCompile("^job_[A-Za-z0-9_-]{1,96}$").MatchString(id) {
			writeJSON(w, 400, map[string]string{"error": "invalid_job_id"})
			return
		}
		s.forwardPilot(w, r, http.MethodGet, "/jobs/"+id, nil)
	})
}
