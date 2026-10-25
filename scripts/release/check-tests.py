#!/usr/bin/env python3
"""Reject failures or skipped required integrations in go test -json output."""

import argparse
import json
import sys
from pathlib import Path

p = argparse.ArgumentParser(description=__doc__)
p.add_argument("result", type=Path)
p.add_argument(
    "--required",
    choices=["database", "mimir", "operator", "prometheus", "identity"],
    required=True,
)
a = p.parse_args()
rows = [
    json.loads(line)
    for line in a.result.read_text().splitlines()
    if line.startswith("{")
]
failures = [r for r in rows if r.get("Action") == "fail"]
skips = []
seen = False

prefixes = {
    "mimir": "TestRealMimir",
    "operator": "TestRealOperator",
    "prometheus": "TestRealPrometheus",
    "identity": "TestRealIdentity",
}
for row in rows:
    test = row.get("Test", "")
    package = row.get("Package", "")
    if not test:
        continue
    required = (
        (
            "postgres" in test.lower()
            or package.endswith("/internal/store")
            or package.endswith("/internal/identity")
            and "Postgres" in test
        )
        if a.required == "database"
        else test.startswith(prefixes[a.required])
    )
    if required:
        seen = True
        if row.get("Action") == "skip":
            skips.append(package + "/" + test)
anchors = {
    "database": [
        "TestPostgresIdentitySessionSurvivesRestart",
        "TestPostgresRestartPreservesSamplesStateAndOutbox",
    ],
    "mimir": [
        "TestRealMimirTenantCollection",
        "TestRealMimirRuleDestinations",
        "TestRealMimirLifecycleAndMinEvents",
        "TestRealMimirIndependentAlertLifecycle",
    ],
    "prometheus": [
        "TestRealPrometheusReviewedActivation",
        "TestRealPrometheusLifecycleAndMinEvents",
    ],
    "identity": [
        "TestRealIdentityOIDCBrowserCLIRevocation",
        "TestRealIdentityOIDCClaimsBoundaries",
        "TestRealIdentityOIDCClaimExpiry",
        "TestRealIdentityTeamsAndProviderBinding",
        "TestRealIdentityCLIApprovalCancellation",
        "TestRealIdentityOpenLDAPTLSAndFailureBoundaries",
    ],
    "operator": [
        "TestRealOperatorRuleSelectors",
        "TestRealOperatorControllerConformance",
        "TestRealOperatorLeaseFailover",
        "TestRealOperatorPortalPublication",
    ],
}[a.required]
passed = {r.get("Test") for r in rows if r.get("Action") == "pass"}
missing = [name for name in anchors if name not in passed]
if failures or skips or not seen or missing:
    print(
        json.dumps(
            {
                "status": "failed",
                "failures": len(failures),
                "requiredSkipped": skips,
                "requiredSeen": seen,
                "requiredMissing": missing,
            }
        )
    )
    sys.exit(1)
print(json.dumps({"status": "passed", "required": a.required, "noRequiredSkips": True}))
