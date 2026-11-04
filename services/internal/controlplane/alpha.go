package controlplane

import (
	"io"
	"net/http"
	"time"
)

func (s *Server) registerAlphaRoutes(mux *http.ServeMux) {
	for _, item := range []struct{ Route, Path, Type string }{
		{"GET /alpha", "assets/alpha.html", "text/html; charset=utf-8"},
		{"GET /alpha.js", "assets/alpha.js", "text/javascript; charset=utf-8"},
	} {
		item := item
		mux.HandleFunc(item.Route, func(w http.ResponseWriter, r *http.Request) {
			if !s.contextAvailable(w, r) {
				return
			}
			raw, err := brandAssets.ReadFile(item.Path)
			if err != nil {
				writeJSON(w, 503, map[string]string{"error": "alpha_view_unavailable"})
				return
			}
			w.Header().Set("Content-Type", item.Type)
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
			_, _ = w.Write(raw)
		})
	}
	mux.HandleFunc("GET /v1/context-session", func(w http.ResponseWriter, r *http.Request) {
		if !s.contextAuthorized(w, r) {
			return
		}
		cookie, err := r.Cookie("__Host-wazi-context")
		if err != nil {
			writeJSON(w, 401, map[string]string{"error": "context_session_required"})
			return
		}
		s.mu.Lock()
		value, found := s.contextSessions[contextKey(cookie.Value)]
		s.mu.Unlock()
		if !found || value.Bootstrap || !s.cfg.Now().Before(value.Expires) {
			writeJSON(w, 401, map[string]string{"error": "context_session_expired"})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]string{"csrf": value.CSRF, "expiresAt": value.Expires.UTC().Format(time.RFC3339)})
	})
	mux.HandleFunc("DELETE /v1/context-session", func(w http.ResponseWriter, r *http.Request) {
		if !s.contextAuthorized(w, r) {
			return
		}
		if cookie, err := r.Cookie("__Host-wazi-context"); err == nil {
			s.mu.Lock()
			delete(s.contextSessions, contextKey(cookie.Value))
			s.mu.Unlock()
		}
		http.SetCookie(w, &http.Cookie{Name: "__Host-wazi-context", Value: "", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]string{"state": "session-closed"})
	})
	mux.HandleFunc("POST /v1/alpha-actions", func(w http.ResponseWriter, r *http.Request) {
		if !s.contextAuthorized(w, r) {
			return
		}
		s.beginBodyRead(w)
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16384))
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid_alpha_request"})
			return
		}
		s.forwardPilot(w, r, http.MethodPost, "/contexts/alpha-actions", raw)
	})
}
