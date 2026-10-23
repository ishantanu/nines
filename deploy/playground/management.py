"""Verify only the synthetic playground: local users, RBAC, revocation and sources."""

import http.cookiejar
import json
import os
import urllib.error
import urllib.request
import uuid
from urllib.parse import urlsplit

BASE = os.environ.get("NINES_MANAGEMENT_TEST_URL", "http://nines:8080")
if urlsplit(BASE).hostname not in ("nines", "127.0.0.1", "localhost", "::1"):
    raise SystemExit(
        "Use only the synthetic playground or a disposable loopback test server"
    )
TOKEN = "nines-local-playground-owner-token"


def call(
    path,
    body=None,
    method=None,
    tenant="prometheus",
    client=None,
    key=True,
    csrf=None,
    match=None,
    want=200,
):
    headers = {"X-Nines-Tenant": tenant}
    if key:
        headers["Authorization"] = "Bearer " + TOKEN
    if body is not None:
        headers["Content-Type"] = "application/json"
    if csrf:
        headers["X-CSRF-Token"] = csrf
    if match is not None:
        headers["If-Match"] = f'"{match}"'
    request = urllib.request.Request(
        BASE + path,
        data=None if body is None else json.dumps(body).encode(),
        headers=headers,
        method=method,
    )
    try:
        with (
            client.open(request, timeout=10)
            if client
            else urllib.request.urlopen(request, timeout=10)
        ) as response:
            code = response.status
            data = json.load(response)
    except urllib.error.HTTPError as error:
        code = error.code
        data = json.load(error)
    assert code == want, (path, code, want, data)
    return data


def login(username, password):
    browser = urllib.request.build_opener(
        urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar())
    )
    info = call("/auth/session", client=browser, key=False)
    call(
        "/auth/local/local/login",
        {"username": username, "password": password},
        client=browser,
        key=False,
        csrf=info["csrfToken"],
    )
    info = call("/auth/session", client=browser, key=False)
    assert info["authenticated"]
    return browser, info["csrfToken"]


def verify(tenants=("prometheus",)):
    owner, csrf = login("admin", os.environ.get("NINES_TEST_PASSWORD", "admin"))
    assert {t["id"] for t in call("/api/tenants", client=owner, key=False)} == set(
        tenants
    )
    print("Local owner sign-in: passed", flush=True)
    suffix = uuid.uuid4().hex[:8]
    username = "smoke-" + suffix
    password = "fixture-local-password"
    user = call(
        "/api/team-access/users",
        {
            "provider": "local",
            "username": username,
            "name": "Smoke viewer",
            "email": "",
            "password": password,
            "role": "viewer",
            "expectedVersion": 0,
        },
        want=201,
    )
    assert "hash" not in json.dumps(user) and password not in json.dumps(user)
    viewer, vcsrf = login(username, password)
    call("/api/slos", client=viewer, key=False)
    call("/api/slos", client=viewer, key=False, tenant="mimir", want=403)
    call("/api/team-access", client=viewer, key=False, want=403)
    call(
        "/api/connections",
        {"name": "forbidden"},
        client=viewer,
        key=False,
        csrf=vcsrf,
        want=403,
    )
    updated = call(
        "/api/team-access/users/" + user["id"],
        {
            "name": "Smoke updated",
            "email": "",
            "password": "fixture-rotated-password",
            "expectedVersion": 1,
        },
        method="PUT",
    )
    assert updated["version"] == 2
    call("/api/slos", client=viewer, key=False, want=401)
    viewer, vcsrf = login(username, "fixture-rotated-password")
    directory = call("/api/team-access")
    member = next(m for m in directory["members"] if m["userId"] == user["id"])
    call(
        "/api/team-access/members/" + user["id"],
        method="DELETE",
        match=member["version"],
    )
    call("/api/slos", client=viewer, key=False, want=403)
    print(
        "User create/read/update/remove, RBAC and password session revocation: passed",
        flush=True,
    )
    name = "smoke-" + suffix
    source = {
        "name": name,
        "kind": "prometheus",
        "url": "http://prometheus:9090",
        "mimirOrgID": "",
        "bearerToken": "synthetic-provider-token",
        "rulesEnabled": True,
        "reload": True,
        "rulerURL": "",
        "expectedVersion": 0,
        "clearCredentials": False,
    }
    created = call("/api/connections", source)
    assert created["version"] == 1 and source["bearerToken"] not in json.dumps(created)
    call("/api/connections/" + name + "/test", method="POST")
    assert (
        next(c for c in call("/api/connections")["connections"] if c["id"] == name)[
            "configuration"
        ]
        == "managed"
    )
    source["expectedVersion"] = 1
    source["bearerToken"] = ""
    assert call("/api/connections/" + name, source, method="PUT")["version"] == 2
    call("/api/connections/" + name, method="DELETE", match=2)
    assert not any(c["id"] == name for c in call("/api/connections")["connections"])
    source["name"] = "blocked-" + suffix
    source["url"] = "http://metadata.invalid"
    source["expectedVersion"] = 0
    call("/api/connections", source, want=400)
    if "mimir" in tenants:
        source.update(
            name="cross-" + suffix,
            kind="mimir",
            url="http://mimir:9009/prometheus",
            mimirOrgID="unauthorized",
        )
        call("/api/connections", source, tenant="mimir", want=400)
        print("Mimir dataset bounds: passed", flush=True)
    print(
        "Source create/test/read/update/delete, credential redaction and host bounds: passed",
        flush=True,
    )


if __name__ == "__main__":
    verify(tuple(os.environ.get("NINES_PLAYGROUND_TENANTS", "prometheus").split(",")))
