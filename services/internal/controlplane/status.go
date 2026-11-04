package controlplane

import (
	_ "embed"
	"net/http"
	"strings"
)

// The overview contains no tenant data. Protected operator routes keep their
// existing authentication, approval and execution checks.
func handleStatusPage(w http.ResponseWriter, _ *http.Request) {
	renderStatusPage(w, false)
}

func renderStatusPage(w http.ResponseWriter, pilot bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; img-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	page := statusPage
	if pilot {
		page = strings.Replace(page, "id=\"pilot-state\">Not configured<", "id=\"pilot-state\">Private pilot configured<", 1)
	}
	_, _ = w.Write([]byte(page))
}

//go:embed assets/status.html
var statusPage string
