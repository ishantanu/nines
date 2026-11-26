#!/usr/bin/env python3
"""Checks a Nines installed by the Helm chart, through a loopback URL: the
portal is served, the administrator signs in with the generated password,
and an SLO saved with --apply is present."""

import argparse
import http.cookiejar
import json
import os
import re
import urllib.request

SLO = """apiVersion: nines.dev/v1beta1
kind: ServiceLevelObjective
metadata:
  name: helm-check
  project: default
spec:
  displayName: Helm check
  service: checkout
  target: 0.99
  window: 1h
  step: 1m
"""


class Client:
    def __init__(self, origin):
        self.origin = origin
        self.opener = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar())
        )

    def call(self, method, path, body=None, headers=None):
        req = urllib.request.Request(
            self.origin + path, data=body, method=method, headers=headers or {}
        )
        with self.opener.open(req, timeout=10) as res:
            return res.read(4 << 20)

    def json(self, method, path, body=None, headers=None):
        return json.loads(self.call(method, path, body, headers))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", required=True)
    parser.add_argument("--apply", action="store_true")
    args = parser.parse_args()
    if not re.fullmatch(r"http://127\.0\.0\.1:\d+", args.url):
        parser.error("use a loopback origin")
    password = os.environ["NINES_PASSWORD"]
    c = Client(args.url)

    html = c.call("GET", "/").decode()
    assets = re.findall(r'(?:src|href)="(/assets/[^"\s]+)"', html)
    if not assets:
        raise SystemExit("the portal has no embedded assets")
    for asset in assets:
        c.call("GET", asset)

    login = c.json("GET", "/auth/session")
    if not login.get("enabled") or not any(
        p["id"] == "local" for p in login.get("providers", [])
    ):
        raise SystemExit("local sign-in is not configured")
    c.call(
        "POST",
        "/auth/local/local/login",
        json.dumps({"username": "admin", "password": password}).encode(),
        {"Content-Type": "application/json", "X-CSRF-Token": login["csrfToken"]},
    )
    session = c.json("GET", "/auth/session")
    actor = session.get("actor") or {}
    if not session.get("authenticated") or not actor.get("platformAdmin"):
        raise SystemExit("the administrator did not sign in")
    scope = {"X-Nines-Tenant": "default", "X-CSRF-Token": session["csrfToken"]}

    if args.apply:
        c.call(
            "POST",
            "/api/resources/apply",
            SLO.encode(),
            {**scope, "Content-Type": "application/yaml", "If-Match": '"0"'},
        )
    slos = c.json("GET", "/api/slos", headers=scope)
    if "helm-check" not in [s["slo"]["id"] for s in slos]:
        raise SystemExit("the saved SLO is missing")
    print(
        json.dumps(
            {
                "status": "passed",
                "signedIn": True,
                "slos": len(slos),
                "assets": len(assets),
            }
        )
    )


if __name__ == "__main__":
    main()
