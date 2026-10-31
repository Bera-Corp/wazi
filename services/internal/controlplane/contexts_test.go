package controlplane

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestContextSessionExchangeIsolationAndCSRF(t *testing.T) {
	now := time.Now()
	config := Config{PilotAgentSocket: "/run/wazi-phase1/agent.sock", Now: func() time.Time { return now }}
	config.OperatorToken = "local-" + "fixture-" + "operator"
	s := New(config)
	handler := s.Handler()
	call := func(method, path string, body []byte, cookie *http.Cookie, origin, csrf string, operator bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://localhost"+path, bytes.NewReader(body))
		r.RemoteAddr = "127.0.0.1:1234"
		if cookie != nil {
			r.AddCookie(cookie)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if csrf != "" {
			r.Header.Set("X-Wazi-CSRF", csrf)
		}
		if operator {
			r.Header.Set("X-Wazi-Operator-Token", config.OperatorToken)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if got := call("POST", "/v1/context-sessions", nil, nil, "", "", false).Code; got != 401 {
		t.Fatalf("unauthorized bootstrap = %d", got)
	}
	created := call("POST", "/v1/context-sessions", nil, nil, "", "", true)
	if created.Code != 201 {
		t.Fatalf("bootstrap = %d", created.Code)
	}
	var bootstrap map[string]string
	if err := json.Unmarshal(created.Body.Bytes(), &bootstrap); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"code": bootstrap["bootstrapCode"]})
	if got := call("POST", "/v1/context-sessions/exchange", body, nil, "https://attacker.invalid", "", false).Code; got != 403 {
		t.Fatalf("cross-origin exchange = %d", got)
	}
	exchanged := call("POST", "/v1/context-sessions/exchange", body, nil, "https://localhost", "", false)
	if exchanged.Code != 200 {
		t.Fatalf("exchange = %d", exchanged.Code)
	}
	cookies := exchanged.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Path != "/" {
		t.Fatal("session cookie is not protected")
	}
	var session map[string]string
	if err := json.Unmarshal(exchanged.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if got := call("POST", "/v1/context-sessions/exchange", body, nil, "https://localhost", "", false).Code; got != 401 {
		t.Fatalf("bootstrap replay = %d", got)
	}
	for _, attack := range []struct{ origin, csrf string }{{"https://attacker.invalid", session["csrf"]}, {"https://localhost", ""}, {"https://localhost", "wrong"}} {
		if got := call("POST", "/v1/contexts/retrieve", []byte("{}"), cookies[0], attack.origin, attack.csrf, false).Code; got != 403 {
			t.Fatalf("CSRF accepted: %d", got)
		}
		for _, path := range []string{"/v1/context-proposals", "/v1/context-approvals", "/v1/context-deployments"} {
			if got := call("POST", path, []byte("{}"), cookies[0], attack.origin, attack.csrf, false).Code; got != 403 {
				t.Fatalf("CSRF accepted on %s: %d", path, got)
			}
		}
	}
	if got := call("GET", "/v1/contexts/invalid", nil, cookies[0], "", "", false).Code; got != 400 {
		t.Fatalf("authenticated context route = %d", got)
	}
	now = now.Add(16 * time.Minute)
	if got := call("GET", "/v1/contexts/invalid", nil, cookies[0], "", "", false).Code; got != 401 {
		t.Fatalf("expired session = %d", got)
	}
}

func TestContextRequiresHTTPSAndDoesNotExposeOperatorCredential(t *testing.T) {
	config := Config{PilotAgentSocket: "/run/wazi-phase1/agent.sock"}
	config.OperatorToken = "local-" + "fixture-" + "operator"
	handler := New(config).Handler()
	r := httptest.NewRequest("POST", "http://localhost/v1/context-sessions", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Wazi-Operator-Token", config.OperatorToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("plaintext session = %d", w.Code)
	}
	for _, path := range []string{"/context", "/context.js"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "https://localhost"+path, nil))
		if w.Code != 200 || bytes.Contains(w.Body.Bytes(), []byte(config.OperatorToken)) {
			t.Fatalf("unsafe browser asset: %s", path)
		}
		if w.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("missing CSP")
		}
	}
}
