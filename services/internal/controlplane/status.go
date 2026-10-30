package controlplane

import (
	"net/http"
	"strings"
)

// The development browser entry point contains no tenant or credential data.
// Protected APIs retain their existing authentication and execution gates.
func handleStatusPage(w http.ResponseWriter, _ *http.Request) {
	renderStatusPage(w, false)
}

func renderStatusPage(w http.ResponseWriter, pilot bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	page := statusPage
	if pilot {
		page = strings.ReplaceAll(page, "Disabled · safety gates open", "Pilot enabled · Gitea only")
		page = strings.ReplaceAll(page, "Application execution remains disabled until its verification gates pass.", "The Gitea pilot requires authenticated approval and verification. General application deployment remains disabled.")
	}
	_, _ = w.Write([]byte(page))
}

const statusPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="theme-color" content="#101b17">
<link rel="icon" type="image/x-icon" href="/assets/favicon.ico" sizes="any">
<title>Wazi | Development status</title>
<style>
:root{color-scheme:dark;font-family:system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#101b17;color:#edf5ee}
*{box-sizing:border-box}body{margin:0;padding:clamp(24px,7vw,80px)}main{max-width:800px;margin:auto}
header{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:64px}
.brand{display:block;width:109px;height:40px;object-fit:contain;flex:none}.badge{border:1px solid #556a5c;border-radius:99px;padding:8px 12px;font-size:13px;color:#b6cabc}
.eyebrow{color:#b6d098;font-size:13px;font-weight:650;letter-spacing:2px;text-transform:uppercase}h1{font-size:clamp(36px,6vw,56px);line-height:1.08;letter-spacing:-2px;margin:16px 0 24px}
p{font-size:17px;line-height:1.7;color:#bfd0c5;max-width:650px}.checks{border:1px solid #354c3d;border-radius:18px;margin:36px 0;overflow:hidden}
.row{display:flex;justify-content:space-between;gap:16px;padding:20px 24px;background:#17261e}.row+.row{border-top:1px solid #354c3d}
.good{color:#c6e7a6}.pending{color:#f1d294}a{display:inline-block;background:#d0e9a6;color:#172117;padding:14px 20px;border-radius:9px;font-weight:650;text-decoration:none}
a:focus-visible{outline:3px solid #fff;outline-offset:5px}footer{margin-top:48px;font-size:14px;line-height:1.7;color:#9db3a5}
@media(max-width:480px){header{margin-bottom:42px}.row{padding:18px;flex-direction:column;gap:7px}}
</style>
</head>
<body><main>
<header><img class="brand" src="/assets/wazi-wordmark-open.png" width="109" height="40" alt="wazi."><span class="badge">Private development</span></header>
<div class="eyebrow">Development status</div>
<h1>The control plane<br>is running.</h1>
<p>This is Wazi's development API for pairing a node and receiving its host inventory. The application dashboard and app catalogue are still upcoming.</p>
<div class="checks" aria-label="Available capabilities">
<div class="row"><span>Control plane</span><strong class="good">Online</strong></div>
<div class="row"><span>Node pairing and inventory API</span><strong class="good">Available with authentication</strong></div>
<div class="row"><span>Application deployment</span><strong class="pending">Disabled · safety gates open</strong></div>
<div class="row"><span>Application dashboard</span><strong>Upcoming</strong></div>
</div>
<a href="/healthz">Check service health</a>
<footer>Use HTTPS for this address. Node inventory requires a paired device certificate; operator actions require authorization. Application execution remains disabled until its verification gates pass.</footer>
</main></body></html>
`
