"""Bootstrap and verify only the isolated Compose playground's synthetic data."""

import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

TOKEN = "nines-local-playground-owner-token"
TENANTS = tuple(os.environ.get("NINES_PLAYGROUND_TENANTS", "prometheus").split(","))
if TENANTS not in (("prometheus",), ("prometheus", "mimir")):
    raise SystemExit("Use prometheus or prometheus,mimir for NINES_PLAYGROUND_TENANTS")
SLO = "checkout-success"


def request(origin, path, body=None, headers=None, method=None):
    req = urllib.request.Request(
        origin + path, data=body, headers=headers or {}, method=method
    )
    with urllib.request.urlopen(req, timeout=12) as response:
        return json.load(response)


def api(tenant, path, body=None, media="application/json", match=None):
    headers = {"Authorization": "Bearer " + TOKEN, "X-Nines-Tenant": tenant}
    if body is not None:
        headers["Content-Type"] = media
    if match is not None:
        headers["If-Match"] = f'"{match}"'
    return request("http://nines:8080", path, body, headers)


def wait(label, check, timeout=150):
    end = time.monotonic() + timeout
    last = None
    while time.monotonic() < end:
        try:
            value = check()
            if value:
                print(label + ": passed", flush=True)
                return value
        except (OSError, ValueError, AssertionError) as error:
            last = type(error).__name__
        time.sleep(3)
    raise RuntimeError(f"{label}: timed out (last error: {last})")


def setup():
    wait("Nines ready", lambda: request("http://nines:8080", "/healthz"))
    manifest = Path("/app/sample.slo.yaml").read_bytes()
    for tenant in TENANTS:
        try:
            api(tenant, f"/api/slos/{SLO}")
        except urllib.error.HTTPError as error:
            if error.code != 404:
                raise
            try:
                api(tenant, "/api/resources/apply", manifest, "application/yaml", 0)
            except urllib.error.HTTPError as conflict:
                if conflict.code != 409:
                    raise
                # setup and verify may start together on a fresh stack.
                api(tenant, f"/api/slos/{SLO}")
        state = api(tenant, f"/api/slos/{SLO}/integration")
        print(
            json.dumps(
                {
                    "tenant": tenant,
                    "definition": state["definition"]["state"],
                    "deployment": state["deployment"]["state"],
                }
            ),
            flush=True,
        )
    print(
        "Definitions ready. Rule deployment is separate; review the Rules tab or run the verify service.",
        flush=True,
    )


def query(tenant, expression):
    origin = (
        "http://prometheus:9090"
        if tenant == "prometheus"
        else "http://mimir:9009/prometheus"
    )
    headers = {} if tenant == "prometheus" else {"X-Scope-OrgID": "playground"}
    data = request(
        origin,
        "/api/v1/query?" + urllib.parse.urlencode({"query": expression}),
        headers=headers,
    )
    assert data["status"] == "success" and not data.get("warnings"), data
    return data["data"]["result"]


def integration(tenant):
    return api(tenant, f"/api/slos/{SLO}/integration")


def status(tenant):
    return api(tenant, f"/api/slos/{SLO}/status")


def failure(percent):
    return request("http://sample:8000", f"/failure?percent={percent}", b"")


def verify():
    setup()
    from management import verify as verify_management

    verify_management(TENANTS)
    revisions = {}
    # Use HTTP API directly so the playground needs Docker only, no host CLI.
    for tenant in TENANTS:
        wait(
            tenant + " source metrics",
            lambda t=tenant: query(t, "playground_requests_total"),
        )
        wait(
            tenant + " measurements collected",
            lambda t=tenant: integration(t)["collection"]["state"] == "collected",
        )
        revisions[tenant] = api(tenant, f"/api/slos/{SLO}")["slo"]["revision"]
        plan = api(tenant, f"/api/slos/{SLO}/rules/plan", b"{}")
        result = api(
            tenant,
            f"/api/slos/{SLO}/rules/apply",
            json.dumps({"expectedDigest": plan["digest"]}).encode(),
        )
        assert result["state"] in ("written", "accepted", "unchanged"), result
        print(tenant + " reviewed rules: " + result["state"], flush=True)
        wait(
            tenant + " external evaluation",
            lambda t=tenant: integration(t)["evaluation"]["state"] == "evaluating",
        )
        assert integration(tenant)["deployment"]["state"] == "deployed"
        wait(
            tenant + " recorded SLI",
            lambda t=tenant: query(
                t, f'nines:sli:ratio{{nines_tenant="{t}",nines_slo="{SLO}"}}'
            ),
        )

    failure(0)
    for tenant in TENANTS:
        wait(
            tenant + " healthy alert baseline",
            lambda t=tenant: not query(
                t,
                f'ALERTS{{alertname="NinesSLOBurnRate",nines_tenant="{t}",nines_revision="{revisions[t]}",alertstate="firing"}}',
            )
            and status(t)["alert"]["phase"] == "inactive",
            300,
        )
    try:
        failure(10)
        print(
            "Synthetic failures set to 10%; waiting for coverage and burn alerts",
            flush=True,
        )
        for tenant in TENANTS:
            for policy, severity in [
                ("fast-burn", "critical"),
                ("sustained-burn", "warning"),
            ]:
                wait(
                    tenant + " " + severity + " external alert firing",
                    lambda t=tenant, p=policy, s=severity: query(
                        t,
                        f'ALERTS{{alertname="NinesSLOBurnRate",nines_tenant="{t}",nines_revision="{revisions[t]}",nines_policy="{p}",severity="{s}",alertstate="firing"}}',
                    ),
                    300,
                )
                wait(
                    tenant + " " + severity + " Nines alert firing",
                    lambda t=tenant, p=policy: status(t)["alert"]
                    .get("policies", {})
                    .get(p, {})
                    .get("phase")
                    == "firing",
                    90,
                )
            snapshot = status(tenant)
            assert snapshot["bad"] > 0 and snapshot["burnRate"] > 0, snapshot
    finally:
        failure(0)
        print("Synthetic failures restored to 0%", flush=True)
    for tenant in TENANTS:
        wait(
            tenant + " external burn alert recovered",
            lambda t=tenant: not query(
                t,
                f'ALERTS{{alertname="NinesSLOBurnRate",nines_tenant="{t}",nines_revision="{revisions[t]}",alertstate="firing"}}',
            ),
            300,
        )
        wait(
            tenant + " Nines burn alert recovered",
            lambda t=tenant: status(t)["alert"]["phase"] == "inactive"
            and all(
                p["phase"] == "inactive"
                for p in status(t)["alert"].get("policies", {}).values()
            ),
            90,
        )
    print(
        json.dumps(
            {
                "result": "passed",
                "tenants": list(TENANTS),
                "checks": [
                    "collection",
                    "reviewed deployment",
                    "external evaluation",
                    "recorded SLI",
                    "burn alert firing",
                    "recovery",
                ],
            }
        ),
        flush=True,
    )


if __name__ == "__main__":
    mode = sys.argv[1] if len(sys.argv) > 1 else "setup"
    if mode == "setup":
        setup()
    elif mode == "verify":
        verify()
    else:
        raise SystemExit("Use setup or verify")
