"""Private-first catalog HTTP surface. No execution, SSH, or Docker endpoints."""
import argparse
import base64
import hashlib
import http.cookies
import io
import json
import os
import secrets
import sqlite3
import socket
import ssl
import threading
import time
import urllib.parse
import zipfile
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

from .contracts import canonical, closed, strict_json
from .github import OAuth, refresh
from .store import Store
from .schema import contracts

ROOT = Path(__file__).resolve().parent
BRAND = ROOT.parent / "services/internal/controlplane/assets"


class BoundedServer(ThreadingHTTPServer):
    daemon_threads = True
    request_queue_size = 16

    def __init__(self, *args, **kwargs):
        self.slots = threading.BoundedSemaphore(32)
        super().__init__(*args, **kwargs)

    def process_request(self, request, client_address):
        if not self.slots.acquire(blocking=False):
            self.shutdown_request(request)
            return
        try:
            super().process_request(request, client_address)
        except Exception:
            self.slots.release()
            raise

    def process_request_thread(self, request, client_address):
        try:
            super().process_request_thread(request, client_address)
        finally:
            self.slots.release()


class Application:
    def __init__(self, store, origin, objects, oauth=None):
        self.store, self.origin, self.objects, self.oauth = store, origin, Path(objects), oauth
        self.authority = urllib.parse.urlsplit(origin).netloc

    def handler(self):
        app = self

        class Handler(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def setup(self):
                super().setup()
                self.connection.settimeout(10)

            def log_message(self, fmt, *args):
                # Query strings can contain OAuth authorization codes.
                pass

            def cookies(self):
                cookie = http.cookies.SimpleCookie()
                try:
                    cookie.load(self.headers.get("Cookie", ""))
                except http.cookies.CookieError:
                    pass
                return cookie

            def cookie(self, name):
                item = self.cookies().get(name)
                return item.value if item else ""

            def response(self, status, value=b"", content_type="application/json", headers=None):
                body = canonical(value) if not isinstance(value, bytes) else value
                self.send_response(status)
                self.send_header("Content-Type", content_type)
                self.send_header("Content-Length", str(len(body)))
                self.send_header("Cache-Control", "no-store")
                self.send_header("X-Content-Type-Options", "nosniff")
                self.send_header("Referrer-Policy", "no-referrer")
                self.send_header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
                for key, item in (headers or {}).items():
                    self.send_header(key, item)
                self.end_headers()
                self.wfile.write(body)

            def user(self, write=False):
                user = app.store.authenticate(self.cookie("__Host-wazi-community"))
                if not user:
                    raise PermissionError("sign in required")
                if write and (self.headers.get("Origin") != app.origin or
                              not secrets.compare_digest(self.headers.get("X-CSRF-Token", ""), user["csrf"])):
                    raise PermissionError("origin or CSRF rejected")
                return user

            def body(self):
                if self.headers.get("Transfer-Encoding") or len(self.headers.get_all("Content-Length", [])) != 1 or self.headers.get("Content-Type", "").split(";")[0] != "application/json":
                    raise ValueError("bounded JSON body required")
                size = int(self.headers.get("Content-Length", "0"))
                if not 1 <= size <= 65536:
                    raise ValueError("body size rejected")
                raw = self.rfile.read(size)
                if len(raw) != size:
                    raise ValueError("incomplete body")
                return strict_json(raw)

            def do_GET(self):
                self.route(False)

            def do_POST(self):
                self.route(True)

            def route(self, write):
                try:
                    if self.headers.get("Host") != app.authority:
                        self.response(421, {"error": "unexpected authority"})
                        return
                    self.dispatch(write)
                except PermissionError:
                    self.close_connection = True
                    self.response(403, {"error": "authentication, consent or request binding rejected"})
                except (ValueError, KeyError, TypeError, json.JSONDecodeError, zipfile.BadZipFile):
                    self.close_connection = True
                    self.response(400, {"error": "request contract rejected"})
                except LookupError:
                    self.response(404, {"error": "not found"})
                except (OSError, TimeoutError, sqlite3.Error):
                    self.close_connection = True
                    self.response(503, {"error": "service temporarily unavailable"})

            def dispatch(self, write):
                url = urllib.parse.urlsplit(self.path)
                path = url.path
                query = urllib.parse.parse_qs(url.query)
                if not write and path == "/healthz":
                    self.response(200, {"status": "ok", "scope": "catalog only; no host execution"})
                elif not write and path in {"/", "/apps", "/dashboard"}:
                    self.response(200, (ROOT / "web/index.html").read_bytes(), "text/html; charset=utf-8")
                elif not write and path.startswith("/assets/"):
                    assets = {"catalog.css": ROOT / "web/catalog.css", "catalog.js": ROOT / "web/catalog.js",
                              "tokens.css": BRAND / "tokens.css", "wordmark.png": BRAND / "wazi-wordmark-open.png", "favicon.ico": BRAND / "favicon.ico"}
                    name = path.removeprefix("/assets/")
                    if name not in assets:
                        raise LookupError()
                    mime = "text/css" if name.endswith(".css") else "text/javascript" if name.endswith(".js") else "image/png" if name.endswith(".png") else "image/x-icon"
                    self.response(200, assets[name].read_bytes(), mime)
                elif not write and path == "/api/applications":
                    self.response(200, {"applications": app.store.applications(), "observedAt": time.time()})
                elif not write and path == "/api/contracts":
                    self.response(200, contracts())
                elif not write and path == "/api/me":
                    user = app.store.authenticate(self.cookie("__Host-wazi-community"))
                    self.response(200, {"user": user, "signInConfigured": app.oauth is not None,
                                        "consent": app.store.consent(user["owner"]) if user else None})
                elif not write and path == "/api/agent/consent":
                    authorization = self.headers.get("Authorization", "")
                    owner = app.store.agent(authorization.removeprefix("Bearer ")) if authorization.startswith("Bearer ") else None
                    if owner is None:
                        raise PermissionError()
                    self.response(200, app.store.consent(owner))
                elif not write and path == "/api/contributions":
                    user = self.user()
                    self.response(200, {"contributions": app.store.reports(user["owner"]), "candidates": app.store.candidates(user["owner"])})
                elif write and path == "/api/consent":
                    user, body = self.user(True), self.body()
                    closed(body, "enabled version licenseAccepted")
                    if body["enabled"] and body["licenseAccepted"] is not True:
                        raise ValueError("explicit original recipe license permission required")
                    app.store.set_consent(user["owner"], body["enabled"], body["version"])
                    self.response(200, app.store.consent(user["owner"]))
                elif write and path == "/api/contributions":
                    user = self.user(True)
                    self.response(201, app.store.submit(user["owner"], self.body()))
                elif write and path == "/api/agent/contributions":
                    authorization = self.headers.get("Authorization", "")
                    owner = app.store.agent(authorization.removeprefix("Bearer ")) if authorization.startswith("Bearer ") else None
                    if owner is None:
                        raise PermissionError()
                    revision = float(self.headers["X-Wazi-Consent-Revision"])
                    self.response(201, app.store.submit(owner, self.body(), revision))
                elif write and path in {"/api/agent/recipe-candidates", "/api/recipe-candidates"}:
                    if path.startswith("/api/agent/"):
                        authorization = self.headers.get("Authorization", "")
                        owner = app.store.agent(authorization.removeprefix("Bearer ")) if authorization.startswith("Bearer ") else None
                        if owner is None:
                            raise PermissionError()
                    else:
                        owner = self.user(True)["owner"]
                    if self.headers.get("Transfer-Encoding") or len(self.headers.get_all("Content-Length", [])) != 1 or self.headers.get("Content-Type") != "application/zip":
                        raise ValueError("bounded ZIP body required")
                    size = int(self.headers.get("Content-Length", "0"))
                    if not 1 <= size <= 4 << 20:
                        raise ValueError("candidate size rejected")
                    raw = self.rfile.read(size)
                    if len(raw) != size:
                        raise ValueError("incomplete archive")
                    revision = float(self.headers["X-Wazi-Consent-Revision"]) if path.startswith("/api/agent/") else None
                    self.response(201, app.store.submit_candidate(owner, raw, revision))
                elif write and path == "/api/recipe-candidates/withdraw":
                    user, body = self.user(True), self.body()
                    closed(body, "id")
                    app.store.withdraw_candidate(user["owner"], body["id"])
                    self.response(200, {"state": "withdrawn"})
                elif write and path == "/api/device/start":
                    self.body()
                    self.response(201, app.store.start_device())
                elif write and path == "/api/device/poll":
                    body = self.body()
                    closed(body, "deviceCode")
                    self.response(200, app.store.poll_device(body["deviceCode"]))
                elif write and path == "/api/device/approve":
                    user, body = self.user(True), self.body()
                    closed(body, "userCode")
                    app.store.approve_device(user["owner"], body["userCode"])
                    self.response(200, {"authorized": True})
                elif write and path == "/api/agents/revoke":
                    user = self.user(True)
                    self.body()
                    app.store.revoke_agents(user["owner"])
                    self.response(200, {"revoked": True})
                elif write and path == "/api/contributions/withdraw":
                    user, body = self.user(True), self.body()
                    closed(body, "id")
                    app.store.withdraw(user["owner"], body["id"])
                    self.response(200, {"state": "withdrawn"})
                elif write and path == "/auth/logout":
                    self.user(True)
                    app.store.logout(self.cookie("__Host-wazi-community"))
                    self.response(200, {"signedOut": True}, headers={"Set-Cookie": "__Host-wazi-community=; Path=/; Secure; HttpOnly; SameSite=Lax; Max-Age=0"})
                elif not write and path == "/auth/login":
                    if app.oauth is None:
                        self.response(503, {"error": "GitHub sign-in is not configured"})
                        return
                    state, destination = app.oauth.begin()
                    self.response(303, b"", headers={"Location": destination, "Set-Cookie": "__Host-wazi-oauth=" + state + "; Path=/; Secure; HttpOnly; SameSite=Lax; Max-Age=300"})
                elif not write and path == "/auth/callback":
                    if app.oauth is None:
                        raise PermissionError()
                    session = app.oauth.finish(query.get("state", [""])[0], self.cookie("__Host-wazi-oauth"), query.get("code", [""])[0])
                    self.response(303, b"", headers={"Location": "/dashboard", "Set-Cookie": "__Host-wazi-community=" + session + "; Path=/; Secure; HttpOnly; SameSite=Lax; Max-Age=3600"})
                elif not write and path == "/api/starter":
                    selected = query.get("application", [""])[0]
                    project = next((item for item in app.store.applications() if item["applicationId"] == selected), None)
                    if project is None:
                        raise LookupError()
                    # Discovery metadata is not a pinned executable package.
                    starter = {"schemaVersion": "1.0.0", "applicationId": selected, "repository": project["repository"],
                               "state": "discovery-starter", "executionAllowed": False,
                               "nextSteps": ["Pin upstream commit and image digests", "Preserve source licenses", "Create typed configuration and procedures", "Qualify in an isolated synthetic environment", "Request independent review and signing"]}
                    self.response(200, starter, headers={"Content-Disposition": 'attachment; filename="wazi-discovery-starter.json"'})
                elif not write and path.startswith("/api/downloads/"):
                    artifact = path.removeprefix("/api/downloads/")
                    if len(artifact) != 64 or any(c not in "0123456789abcdef" for c in artifact):
                        raise LookupError()
                    with app.store.connect() as db:
                        row = db.execute("SELECT 1 FROM recipes WHERE artifact=? AND revoked=0", (artifact,)).fetchone()
                    if not row:
                        raise LookupError()
                    raw = (app.objects / artifact).read_bytes()
                    if hashlib.sha256(raw).hexdigest() != artifact:
                        raise OSError("object integrity failure")
                    self.response(200, raw, "application/zip", {"Content-Disposition": 'attachment; filename="wazi-package.zip"'})
                else:
                    self.close_connection = True
                    self.response(404, {"error": "route unavailable"})

        return Handler


def main():
    from .packages import record_validator
    record_validator("app-manifest.schema.json")
    record_validator("procedure.schema.json")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--database", required=True)
    parser.add_argument("--objects", required=True)
    parser.add_argument("--origin", default="https://127.0.0.1:9444")
    parser.add_argument("--bind", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=9444)
    parser.add_argument("--certificate", required=True)
    parser.add_argument("--key", required=True)
    args = parser.parse_args()
    if args.bind not in {"127.0.0.1", "::1"}:
        parser.error("private pilot listener must be loopback; public ingress is a separate release gate")
    parsed_origin = urllib.parse.urlsplit(args.origin)
    if parsed_origin.scheme != "https" or not parsed_origin.netloc or parsed_origin.path or parsed_origin.query or parsed_origin.fragment or parsed_origin.username or parsed_origin.password:
        parser.error("fixed HTTPS origin required")
    store = Store(args.database)
    store.seed(strict_json((ROOT / "seeds.json").read_bytes()))
    client_id = os.environ.get("WAZI_GITHUB_CLIENT_ID", "")
    secret_value = os.environ.get("WAZI_GITHUB_CLIENT_SECRET", "")
    if bool(client_id) != bool(secret_value):
        parser.error("configure both GitHub OAuth fields or neither")
    oauth = OAuth(store, client_id, secret_value, args.origin) if client_id and secret_value else None
    app = Application(store, args.origin, args.objects, oauth)
    if args.bind == "::1":
        class IPv6Server(BoundedServer):
            address_family = socket.AF_INET6
        server = IPv6Server((args.bind, args.port), app.handler())
    else:
        server = BoundedServer((args.bind, args.port), app.handler())
    server.daemon_threads = True
    tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    tls.minimum_version = ssl.TLSVersion.TLSv1_2
    tls.load_cert_chain(args.certificate, args.key)
    server.socket = tls.wrap_socket(server.socket, server_side=True)
    def metadata_loop():
        while True:
            try:
                refresh(store)
            except Exception:
                pass  # Cached entries retain their stale marker.
            time.sleep(60)
    threading.Thread(target=metadata_loop, daemon=True).start()
    server.serve_forever(poll_interval=0.5)


if __name__ == "__main__":
    main()
