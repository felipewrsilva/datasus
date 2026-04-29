#!/usr/bin/env python3
"""One-shot Metabase first-time setup via /api/setup (local Docker stack).

Run after: docker compose up -d
Defaults suit docker-compose.yml (admin user + DATASUS Pipeline DB on service `db`).

Env overrides:
  METABASE_URL              default http://localhost:3001
  METABASE_ADMIN_EMAIL      default admin@datasus.local
  METABASE_ADMIN_PASSWORD   default MetabaseLocal#2026 (change in production)
  METABASE_SITE_NAME        default DATASUS Analytics
"""

from __future__ import annotations

import json
import os
import sys
import time
import urllib.error
import urllib.request

BASE = os.environ.get("METABASE_URL", "http://localhost:3001").rstrip("/")
ADMIN_EMAIL = os.environ.get("METABASE_ADMIN_EMAIL", "admin@datasus.local")
ADMIN_PASSWORD = os.environ.get("METABASE_ADMIN_PASSWORD", "MetabaseLocal#2026")
SITE_NAME = os.environ.get("METABASE_SITE_NAME", "DATASUS Analytics")

MAX_WAIT_S = 120
POLL_S = 2


def http_json(method: str, path: str, body: dict | None = None) -> dict:
    url = f"{BASE}{path}"
    data = None
    headers = {"Accept": "application/json"}
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            raw = resp.read().decode("utf-8")
            return json.loads(raw) if raw else {}
    except urllib.error.HTTPError as e:
        err = e.read().decode("utf-8", errors="replace")
        raise SystemExit(f"HTTP {e.code} {path}: {err}") from e


def wait_for_ready() -> None:
    deadline = time.monotonic() + MAX_WAIT_S
    last_err: str | None = None
    while time.monotonic() < deadline:
        try:
            req = urllib.request.Request(
                f"{BASE}/api/session/properties",
                headers={"Accept": "application/json"},
                method="GET",
            )
            with urllib.request.urlopen(req, timeout=5) as resp:
                if resp.status == 200:
                    return
        except Exception as e:
            last_err = str(e)
        time.sleep(POLL_S)
    raise SystemExit(
        f"Metabase not reachable at {BASE} within {MAX_WAIT_S}s. Last error: {last_err}"
    )


def main() -> None:
    wait_for_ready()
    props = http_json("GET", "/api/session/properties")
    token = props.get("setup_token") or props.get("setup-token")
    if not token:
        print("Metabase is already initialized (no setup token). Nothing to do.")
        return

    payload = {
        "token": token,
        "user": {
            "first_name": "Admin",
            "last_name": "Local",
            "email": ADMIN_EMAIL,
            "password": ADMIN_PASSWORD,
        },
        "prefs": {"site_name": SITE_NAME, "allow_tracking": False},
        "database": {
            "name": "DATASUS Pipeline",
            "engine": "postgres",
            "details": {
                "host": "db",
                "port": 5432,
                "dbname": "datasus",
                "user": "datasus",
                "password": "datasus",
                "ssl": False,
                "tunnel-enabled": False,
            },
        },
    }
    try:
        http_json("POST", "/api/setup", payload)
    except SystemExit as exc:
        err = str(exc).lower()
        if "403" in str(exc) and "first user" in err:
            print(
                "Metabase already has an admin (setup was done earlier). "
                "If the pipeline DB is missing: Admin → Databases → Add database → PostgreSQL, "
                "host db, DB name datasus, user/password datasus, SSL off.",
                file=sys.stderr,
            )
            return
        raise
    print(f"Metabase setup complete. Sign in at {BASE} as {ADMIN_EMAIL}")


if __name__ == "__main__":
    main()
