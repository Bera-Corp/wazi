"""Bounded GitHub-only metadata and identity transport. No repository write scopes."""
import base64
import hashlib
import json
import secrets
import time
import urllib.error
import urllib.parse
import urllib.request

from .contracts import canonical


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def request_json(url, *, headers=None, data=None, method=None):
    parsed = urllib.parse.urlsplit(url)
    if parsed.scheme != "https" or parsed.hostname not in {"github.com", "api.github.com"} or parsed.username or parsed.password or parsed.port not in {None, 443}:
        raise ValueError("GitHub HTTPS transport only")
    request = urllib.request.Request(url, data, headers={"Accept": "application/json", "User-Agent": "Wazi-community/1", **(headers or {})}, method=method)
    try:
        with urllib.request.build_opener(NoRedirect).open(request, timeout=15) as response:
            raw = response.read((1 << 20) + 1)
            if len(raw) > 1 << 20:
                raise ValueError("upstream response too large")
            return response.status, dict(response.headers), json.loads(raw)
    except urllib.error.HTTPError as error:
        if error.code in {304, 403, 404, 429}:
            return error.code, dict(error.headers), {}
        raise ValueError("GitHub request failed") from None


def refresh(store, now=None, transport=request_json):
    now = time.time() if now is None else now
    with store.connect() as db:
        pause = db.execute("SELECT next_check FROM upstream WHERE name='github'").fetchone()
        if pause and pause[0] > now:
            return 0
    updated = 0
    for app in store.applications(now):
        app_id = app["applicationId"]
        with store.connect() as db:
            cached = db.execute("SELECT * FROM metadata WHERE id=?", (app_id,)).fetchone()
        if cached and cached["next_check"] > now:
            continue
        headers = {"Accept": "application/vnd.github+json"}
        if cached and cached["etag"]:
            headers["If-None-Match"] = cached["etag"]
        failures = cached["failures"] if cached else 0
        cooldown = 0
        try:
            status, response_headers, body = transport("https://api.github.com/repos/" + app["repository"], headers=headers)
            if status in {403, 429}:
                cooldown = rate_delay(response_headers, now)
            if status == 304 and cached:
                safe = json.loads(cached["body"])
                etag = cached["etag"]
            else:
                if status != 200 or body.get("full_name", "").lower() != app["repository"].lower():
                    raise ValueError("unexpected GitHub metadata")
                stars = body["stargazers_count"]
                if type(stars) is not int or stars < 0:
                    raise ValueError("invalid star count")
                license_id = (body.get("license") or {}).get("spdx_id")
                if license_id is not None and (not isinstance(license_id, str) or len(license_id) > 96):
                    raise ValueError("invalid license")
                safe = {"stars": stars, "license": license_id, "archived": bool(body.get("archived")),
                        "upstreamUpdatedAt": body.get("pushed_at")}
                etag = response_headers.get("ETag", response_headers.get("Etag", ""))[:256]
                previous = json.loads(cached["body"]) if cached else None
                if isinstance(previous, dict):
                    for name in ("latestRelease", "releaseObservedAt"):
                        if name in previous:
                            safe[name] = previous[name]
            # Repository and release validators are different resources. Never
            # send a repository ETag to the release endpoint.
            release_status, release_headers, release = transport("https://api.github.com/repos/" + app["repository"] + "/releases/latest",
                                                   headers={"Accept": "application/vnd.github+json"})
            if release_status in {403, 429}:
                cooldown = rate_delay(release_headers, now)
            if release_status in {200, 404}:
                tag = release.get("tag_name") if release_status == 200 else None
                if tag is not None and (not isinstance(tag, str) or len(tag) > 96):
                    raise ValueError("invalid upstream release")
                safe["latestRelease"] = tag
                safe["releaseObservedAt"] = now
            with store.connect() as db:
                db.execute("INSERT INTO metadata VALUES(?,?,?,?,?,0) ON CONFLICT(id) DO UPDATE SET body=excluded.body,etag=excluded.etag,checked=excluded.checked,next_check=excluded.next_check,failures=0",
                           (app_id, canonical(safe).decode(), etag, now, now + 86400))
                if cooldown:
                    db.execute("UPDATE metadata SET next_check=max(next_check,?)", (now + cooldown,))
                    db.execute("INSERT INTO upstream VALUES('github',?) ON CONFLICT(name) DO UPDATE SET next_check=excluded.next_check", (now + cooldown,))
            updated += 1
            if cooldown:
                break
        except (ValueError, KeyError, TypeError, OSError):
            failures += 1
            delay = cooldown or min(86400, 60 * 2 ** min(failures, 10))
            # A failed refresh never marks old metadata fresh.
            with store.connect() as db:
                if cached:
                    db.execute("UPDATE metadata SET next_check=?,failures=? WHERE id=?", (now + delay, failures, app_id))
                else:
                    db.execute("INSERT INTO metadata VALUES(?, 'null', '', 0, ?, ?)", (app_id, now + delay, failures))
                if cooldown:
                    db.execute("UPDATE metadata SET next_check=max(next_check,?)", (now + cooldown,))
                    db.execute("INSERT INTO upstream VALUES('github',?) ON CONFLICT(name) DO UPDATE SET next_check=excluded.next_check", (now + cooldown,))
            if cooldown:
                break
    return updated


def rate_delay(headers, now):
    lowered = {key.lower(): value for key, value in headers.items()}
    values = [3600]
    try:
        if "retry-after" in lowered:
            values.append(int(lowered["retry-after"]))
        if "x-ratelimit-reset" in lowered:
            values.append(int(lowered["x-ratelimit-reset"]) - now)
    except (TypeError, ValueError):
        pass
    return min(86400, max(values))


class OAuth:
    def __init__(self, store, client_id, client_secret, origin, transport=request_json):
        self.store, self.client_id, self.client_secret = store, client_id, client_secret
        parsed = urllib.parse.urlsplit(origin)
        if parsed.scheme != "https" or not parsed.netloc or parsed.path or parsed.query or parsed.fragment or parsed.username or parsed.password:
            raise ValueError("fixed HTTPS origin required")
        self.origin, self.transport = origin, transport

    def begin(self):
        if not self.client_id or not self.client_secret:
            raise ValueError("GitHub sign-in is not configured")
        state, verifier = secrets.token_urlsafe(32), secrets.token_urlsafe(48)
        challenge = base64.urlsafe_b64encode(hashlib.sha256(verifier.encode()).digest()).rstrip(b"=").decode()
        with self.store.connect() as db:
            db.execute("BEGIN IMMEDIATE")
            db.execute("DELETE FROM oauth WHERE expires<?", (time.time(),))
            if db.execute("SELECT count(*) FROM oauth").fetchone()[0] >= 256:
                raise ValueError("pending identity capacity exceeded")
            db.execute("INSERT INTO oauth VALUES(?,?,?)", (self.store.key(state), verifier, time.time() + 300))
        query = urllib.parse.urlencode({"client_id": self.client_id, "redirect_uri": self.origin + "/auth/callback",
                                      "state": state, "scope": "", "code_challenge": challenge, "code_challenge_method": "S256"})
        return state, "https://github.com/login/oauth/authorize?" + query

    def finish(self, state, cookie_state, code):
        if not state or not secrets.compare_digest(state, cookie_state) or not code or len(code) > 256:
            raise PermissionError("OAuth browser binding rejected")
        with self.store.connect() as db:
            db.execute("BEGIN IMMEDIATE")
            record = db.execute("SELECT verifier FROM oauth WHERE hash=? AND expires>?", (self.store.key(state), time.time())).fetchone()
            db.execute("DELETE FROM oauth WHERE hash=?", (self.store.key(state),))
        if not record:
            raise PermissionError("OAuth state expired or replayed")
        fields = {"client_id": self.client_id, "code": code,
                  "redirect_uri": self.origin + "/auth/callback", "code_verifier": record[0]}
        fields["client_secret"] = self.client_secret
        data = urllib.parse.urlencode(fields).encode()
        status, _, response = self.transport("https://github.com/login/oauth/access_token", data=data,
                                             headers={"Content-Type": "application/x-www-form-urlencoded"}, method="POST")
        access = response.get("access_token")
        if status != 200 or not isinstance(access, str) or not access:
            raise PermissionError("GitHub identity exchange failed")
        if response.get("scope", ""):
            raise PermissionError("unexpected OAuth scopes")
        status, _, user = self.transport("https://api.github.com/user", headers={"Authorization": "Bearer " + access})
        # Discard the GitHub access credential; store only our hashed session.
        if status != 200 or type(user.get("id")) is not int or user["id"] < 1:
            raise PermissionError("GitHub identity unavailable")
        login = user.get("login", "")
        if not isinstance(login, str) or not 1 <= len(login) <= 39 or not all(c.isascii() and (c.isalnum() or c == "-") for c in login):
            raise PermissionError("GitHub identity invalid")
        return self.store.session(str(user["id"]), login)
