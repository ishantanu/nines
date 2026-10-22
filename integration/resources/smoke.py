#!/usr/bin/env python3
"""Exercise YAML CLI/API round trips and restart persistence in a disposable DB.

Requires TEST_DATABASE_URL (loopback PostgreSQL), NINES_SERVER_BINARY and
NINES_CTL_BINARY. Writes synthetic resources; use a dedicated test database.
"""

import json
import os
import secrets
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
database = os.environ["TEST_DATABASE_URL"]
if urllib.parse.urlparse(database).hostname not in ("127.0.0.1", "localhost", "::1"):
    raise SystemExit("Use a disposable loopback PostgreSQL database")
server_binary = os.environ["NINES_SERVER_BINARY"]
ctl_binary = os.environ["NINES_CTL_BINARY"]
token = secrets.token_hex(24)
with socket.socket() as listener:
    listener.bind(("127.0.0.1", 0))
    port = listener.getsockname()[1]
origin = f"http://127.0.0.1:{port}"
env = {
    k: v
    for k, v in os.environ.items()
    if not k.startswith(("PROMETHEUS_", "MIMIR_", "ALERT_WEBHOOK_", "TENANTS_FILE"))
}
env.update(DATABASE_URL=database, API_TOKEN=token)
process = None


def request(path, body=None, media="application/json", match=None, expected=200):
    headers = {"Authorization": "Bearer " + token, "X-Nines-Tenant": "default"}
    if body is not None:
        headers["Content-Type"] = media
    if match is not None:
        headers["If-Match"] = f'"{match}"'
    req = urllib.request.Request(origin + path, data=body, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=5) as response:
            status, content = response.status, response.read()
    except urllib.error.HTTPError as error:
        status, content = error.code, error.read()
    assert status == expected, (path, status, content)
    return content


def start(log):
    global process
    process = subprocess.Popen(
        [server_binary, "-addr", f"127.0.0.1:{port}"], env=env, stdout=log, stderr=log
    )
    for _ in range(100):
        if process.poll() is not None:
            raise RuntimeError("server exited; inspect smoke log")
        try:
            request("/healthz")
            return
        except (OSError, AssertionError):
            time.sleep(0.05)
    raise RuntimeError("server readiness timed out")


def stop():
    global process
    if process is not None:
        process.terminate()
        process.wait(timeout=10)
        process = None


with tempfile.TemporaryDirectory(prefix="nines-yaml-smoke-") as folder:
    folder = Path(folder)
    with (folder / "server.log").open("wb") as log:
        try:
            start(log)
            prefix = "yaml-" + secrets.token_hex(4)
            ids = {
                name: prefix + "-" + name
                for name in (
                    "api-availability",
                    "checkout-success",
                    "background-jobs",
                    "complete-checkout",
                )
            }
            slo = (ROOT / "examples/api-availability.slo.yaml").read_text()
            # Verify existing JSON-created resources remain exportable without migration.
            legacy_id = prefix + "-legacy"
            legacy = dict(
                id=legacy_id,
                name="Legacy objective",
                project="test",
                service="api",
                target=0.99,
                windowSeconds=3600,
                stepSeconds=60,
            )
            request("/api/slos", json.dumps(legacy).encode(), expected=201)
            exported = request("/api/slos/" + legacy_id + "/manifest")
            assert (
                b"kind: ServiceLevelObjective" in exported
                and b"createdAt" not in exported
            )
            old_digest = json.loads(
                request("/api/resources/plan", exported, "application/yaml")
            )["digest"]
            for name in ("api-availability", "checkout-success", "background-jobs"):
                file = folder / (name + ".yaml")
                file.write_text(
                    slo.replace("name: api-availability", "name: " + ids[name])
                )
                for action, args in [
                    ("validate", []),
                    ("plan", []),
                    ("apply", ["-expected-revision", "0"]),
                ]:
                    result = subprocess.run(
                        [
                            ctl_binary,
                            action,
                            "-f",
                            str(file),
                            "-server",
                            origin,
                            "-tenant",
                            "default",
                        ]
                        + args,
                        env=env,
                        check=True,
                        capture_output=True,
                        text=True,
                    )
                    output = json.loads(result.stdout)
                    manifest = output if action == "validate" else output["manifest"]
                    assert manifest["kind"] == "ServiceLevelObjective"
            journey = (ROOT / "examples/checkout.journey.yaml").read_text()
            for old, new in ids.items():
                journey = journey.replace(old, new)
            file = folder / "checkout.yaml"
            file.write_text(journey)
            subprocess.run(
                [
                    ctl_binary,
                    "apply",
                    "-f",
                    str(file),
                    "-server",
                    origin,
                    "-tenant",
                    "default",
                    "-expected-revision",
                    "0",
                ],
                env=env,
                check=True,
                capture_output=True,
            )
            manifests = {}
            for path in (
                "/api/slos/" + ids["api-availability"] + "/manifest",
                "/api/journeys/" + ids["complete-checkout"] + "/manifest",
            ):
                manifests[path] = request(path)
                plan = json.loads(
                    request("/api/resources/plan", manifests[path], "application/yaml")
                )
                assert plan["action"] == "unchanged" and plan["expectedRevision"] == 1
                request(
                    "/api/resources/apply",
                    manifests[path],
                    "application/yaml",
                    match=0,
                    expected=409,
                )
                request(
                    "/api/resources/apply", manifests[path], "application/yaml", match=1
                )
            timestamp = time.strftime("%Y-%m-%dT%H:%M:00Z", time.gmtime())
            request(
                "/api/slos/" + ids["api-availability"] + "/samples",
                json.dumps(
                    {"samples": [{"time": timestamp, "good": 99, "total": 100}]}
                ).encode(),
            )
            stop()
            start(log)
            for path, content in manifests.items():
                assert request(path) == content, "manifest changed across restart"
            assert (
                json.loads(
                    request("/api/resources/plan", exported, "application/yaml")
                )["digest"]
                == old_digest
            )
            snapshot = json.loads(
                request("/api/slos/" + ids["api-availability"] + "/status")
            )
            assert snapshot["good"] == 99 and snapshot["total"] == 100
            assert (
                len(
                    json.loads(
                        request("/api/slos/" + ids["api-availability"] + "/journeys")
                    )
                )
                == 1
            )
            print(
                "PASS: both YAML kinds, CLI/API CAS, legacy export, backlinks, samples and manifests after PostgreSQL restart"
            )
        finally:
            stop()
