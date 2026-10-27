#!/usr/bin/env python3
"""Reject failures or skipped required integrations in go test -json output."""

import argparse
import json
import sys
from collections import deque
from pathlib import Path

p = argparse.ArgumentParser(description=__doc__)
p.add_argument("result", type=Path)
p.add_argument(
    "--required",
    choices=["database", "mimir", "operator", "prometheus", "identity", "topology"],
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
    "topology": "TestRealInstrumentedDependency",
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
    "topology": ["TestRealInstrumentedDependencyEvidence"],
    "database": [
        "TestPostgresIdentitySessionSurvivesRestart",
        "TestPostgresRestartPreservesSamplesStateAndOutbox",
    ],
    "mimir": [
        "TestRealMimirRetirement",
        "TestRealMimirTenantCollection",
        "TestRealMimirRuleDestinations",
        "TestRealMimirLifecycleAndMinEvents",
        "TestRealMimirIndependentAlertLifecycle",
    ],
    "prometheus": [
        "TestRealPrometheusRetirement",
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
        "TestRealOperatorCleanupOutageAndCredentialRotation",
        "TestRealOperatorIndependentSeverityEvaluation",
    ],
}[a.required]
passed = {r.get("Test") for r in rows if r.get("Action") == "pass"}
missing = [name for name in anchors if name not in passed]
if failures or skips or not seen or missing:
    # Preserve the failure explanation before disposable fixture cleanup. Keep
    # diagnostics bounded; successful tests remain in the JSON report only.
    failed_tests = {(r.get("Package"), r.get("Test")) for r in failures}
    diagnostics = {key: deque(maxlen=40) for key in failed_tests}
    for row in rows:
        key = (row.get("Package"), row.get("Test"))
        if key in diagnostics and row.get("Action") == "output":
            diagnostics[key].append(row.get("Output", ""))
    for key, output in diagnostics.items():
        print(
            "Failure diagnostics: " + "/".join(part for part in key if part),
            file=sys.stderr,
        )
        for line in output:
            print(line[:4000], end="" if line.endswith("\n") else "\n", file=sys.stderr)
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
