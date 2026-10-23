#!/usr/bin/env python3
"""Read-only demo smoke test against a task-owned binary or loopback URL."""

import argparse
import json
import os
import re
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
from pathlib import Path
from urllib.parse import urlsplit


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def check(origin, token=None):
    opener = urllib.request.build_opener(NoRedirect())

    def get(path):
        headers = {"X-Nines-Tenant": "default"}
        if token:
            headers["Authorization"] = "Bearer " + token
        req = urllib.request.Request(origin + path, headers=headers)
        with opener.open(req, timeout=3) as res:
            body = res.read((4 << 20) + 1)
            if res.status != 200 or len(body) > 4 << 20:
                raise ValueError("Invalid smoke response")
            return body

    html = get("/").decode()
    assets = re.findall(r'(?:src|href)="(/assets/[^"\s]+)"', html)
    if not assets:
        raise ValueError("UI has no embedded asset references")
    for asset in assets:
        get(asset)
    health = json.loads(get("/healthz"))
    slos = json.loads(get("/api/slos"))
    tenants = json.loads(get("/api/tenants"))
    if (
        health.get("status") != "ok"
        or not isinstance(slos, list)
        or not slos
        or not isinstance(tenants, list)
        or len(tenants) < 2
    ):
        raise ValueError("Missing healthy synthetic demo workspaces")
    routes = [
        "/login",
        "/tenants/default/home",
        "/tenants/default/slos",
        "/tenants/default/slos/" + slos[0]["slo"]["id"],
    ]
    for path in routes:
        if get(path).decode() != html:
            raise ValueError("Deep link did not serve the embedded portal")
    missing = [
        "/api/does-not-exist",
        "/assets/does-not-exist.js",
        "/tenants/default/does-not-exist",
    ]
    for path in missing:
        try:
            get(path)
        except urllib.error.HTTPError as error:
            error.read()
            if error.code != 404:
                raise ValueError("Missing route did not return 404") from error
        else:
            raise ValueError("Missing route received a successful response")
    print(
        json.dumps(
            {
                "status": "passed",
                "demoSLOs": len(slos),
                "demoTenants": len(tenants),
                "embeddedAssets": len(assets),
                "portalRoutes": len(routes),
                "missingRoutes": len(missing),
                "writes": False,
            }
        )
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--binary", type=Path)
    group.add_argument("--url")
    parser.add_argument(
        "--token-env",
        help="environment variable containing the token for a loopback URL check",
    )
    args = parser.parse_args()
    if args.url:
        u = urlsplit(args.url)
        if (
            u.scheme != "http"
            or u.hostname not in {"127.0.0.1", "localhost", "::1"}
            or u.path not in {"", "/"}
            or u.query
            or u.fragment
            or u.username is not None
        ):
            parser.error("smoke URL must be a loopback HTTP origin")
        token = os.environ.get(args.token_env) if args.token_env else None
        if args.token_env and not token:
            parser.error("smoke token environment variable is empty")
        check(args.url.rstrip("/"), token)
        return
    if args.token_env:
        parser.error("--token-env is only used with --url")
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    origin = f"http://127.0.0.1:{port}"
    with tempfile.TemporaryFile() as log:
        # No model/provider/telemetry credentials inherited by the demo proof.
        env = {
            key: value
            for key, value in os.environ.items()
            if key in {"PATH", "SYSTEMROOT", "TMPDIR"}
        }
        child = subprocess.Popen(
            [str(args.binary.resolve()), "-demo", "-addr", f"127.0.0.1:{port}"],
            env=env,
            stdout=log,
            stderr=log,
        )
        try:
            for _ in range(60):
                if child.poll() is not None:
                    raise ValueError("Demo process exited during startup")
                try:
                    with urllib.request.urlopen(
                        origin + "/healthz", timeout=1
                    ) as response:
                        if response.status == 200:
                            break
                except (urllib.error.URLError, TimeoutError):
                    time.sleep(0.1)
            else:
                raise ValueError("Demo did not become ready")
            check(origin)
        finally:
            child.terminate()
            try:
                child.wait(timeout=5)
            except subprocess.TimeoutExpired:
                child.kill()
                child.wait()


if __name__ == "__main__":
    main()
