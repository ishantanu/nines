#!/usr/bin/env python3
"""Checks the Helm chart without a cluster: it lints and renders in every
supported configuration, refuses unsafe or incomplete values, renders no
duplicate keys, ships the operator CRDs byte for byte, and its version
matches the release the manifests reference. HELM may name the binary."""

import os
import re
import subprocess
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[2]
CHART = ROOT / "charts" / "nines"
HELM = os.environ.get("HELM", "helm")

BASE = ["--set", "database.existingSecret=db"]
CONFIGURATIONS = {
    "defaults": BASE,
    "ingress with TLS": BASE
    + [
        "--set",
        "ingress.enabled=true",
        "--set",
        "ingress.host=nines.example.com",
        "--set",
        "ingress.tlsSecretName=tls",
    ],
    "operator publishing": BASE
    + [
        "--set",
        "operator.enabled=true",
        "--set",
        "operator.publishFromPortal=true",
        "--set",
        "operator.credentialsSecret=ruler",
    ],
    "connections": BASE
    + ["--set", "connections.allowedHosts={prometheus.monitoring.svc:9090}"],
    "telemetry": BASE
    + [
        "--set",
        "operator.enabled=true",
        "--set",
        "telemetry.otlpEndpoint=http://otel-collector.observability.svc:4318",
        "--set",
        "telemetry.resourceAttributes.team=sre",
    ],
    "own credentials and digest": BASE
    + [
        "--set",
        "credentials.existingSecret=mine",
        "--set",
        "image.digest=sha256:" + "a" * 64,
    ],
}
REFUSED = {
    "no database": [],
    "ingress without host": BASE + ["--set", "ingress.enabled=true"],
    "misspelled value": BASE + ["--set", "replicas=2"],
    "unknown source kind": BASE + ["--set", "operator.sources.primary.kind=thanos"],
    "telemetry endpoint with credentials": BASE
    + ["--set", "telemetry.otlpEndpoint=http://user:pass@collector:4318"],
    "resource attribute with a separator": BASE
    + ["--set", "telemetry.resourceAttributes.team=a,b"],
}


class Strict(yaml.SafeLoader):
    pass


def no_duplicates(loader, node, deep=False):
    keys = [loader.construct_object(k, deep=deep) for k, _ in node.value]
    dupes = {k for k in keys if keys.count(k) > 1}
    if dupes:
        raise ValueError(f"duplicate keys {sorted(dupes)}")
    return yaml.SafeLoader.construct_mapping(loader, node, deep)


Strict.add_constructor(yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, no_duplicates)


def helm(*args):
    return subprocess.run([HELM, *args], capture_output=True, text=True)


def render(name, flags):
    out = helm("template", "nines", str(CHART), "--namespace", "nines", *flags)
    if out.returncode:
        raise SystemExit(f"{name}: render failed: {out.stderr.strip()}")
    return [d for d in yaml.load_all(out.stdout, Loader=Strict) if d]


def main():
    failures = []
    for name, flags in CONFIGURATIONS.items():
        lint = helm("lint", "--strict", str(CHART), *flags)
        if lint.returncode and "icon is recommended" not in lint.stdout:
            failures.append(f"{name}: lint: {lint.stdout.strip()}")
        docs = render(name, flags)
        server = next(
            d
            for d in docs
            if d["kind"] == "Deployment" and d["metadata"]["name"] == "nines"
        )
        pod = server["spec"]["template"]["spec"]
        if (
            server["spec"]["replicas"] != 1
            or server["spec"]["strategy"]["type"] != "Recreate"
        ):
            failures.append(
                f"{name}: the server must run one replica, replaced not rolled"
            )
        if pod["securityContext"].get("runAsNonRoot") is not True:
            failures.append(f"{name}: the server must not run as root")
        publishing = "operator publishing" == name
        if pod["automountServiceAccountToken"] is not publishing:
            failures.append(
                f"{name}: the server token must be mounted only for operator publishing"
            )
        for doc in docs:
            if doc["kind"] == "Secret" and "stringData" in doc:
                failures.append(f"{name}: secrets must not be rendered from values")
    for name, flags in REFUSED.items():
        if helm("template", "nines", str(CHART), *flags).returncode == 0:
            failures.append(f"{name}: should be refused")

    rendered = render("crds", CONFIGURATIONS["operator publishing"])
    crds = [d for d in rendered if d["kind"] == "CustomResourceDefinition"]
    shipped = [
        d
        for d in yaml.safe_load_all((ROOT / "deploy/operator/crds.yaml").read_text())
        if d
    ]
    if (CHART / "files/crds.yaml").read_text() != (
        ROOT / "deploy/operator/crds.yaml"
    ).read_text():
        failures.append(
            "charts/nines/files/crds.yaml must equal deploy/operator/crds.yaml"
        )
    for crd in crds:
        if (
            crd["metadata"].pop("annotations", {}).get("helm.sh/resource-policy")
            != "keep"
        ):
            failures.append(
                f"{crd['metadata']['name']}: CRDs must be kept on uninstall"
            )
    if crds != shipped:
        failures.append("rendered CRDs differ from deploy/operator/crds.yaml")

    chart = yaml.safe_load((CHART / "Chart.yaml").read_text())
    tag = re.search(
        r"image: ghcr\.io/[^/]+/nines:(\S+)",
        (ROOT / "deploy/kubernetes/deployment.yaml").read_text(),
    ).group(1)
    if chart["appVersion"] != tag or chart["version"] != tag.removeprefix("v"):
        failures.append(
            f"Chart.yaml version {chart['version']} / appVersion {chart['appVersion']} must match the manifests' {tag}"
        )

    if failures:
        print("\n".join(failures), file=sys.stderr)
        raise SystemExit(1)
    print(
        f"chart checked: {len(CONFIGURATIONS)} configurations render, {len(REFUSED)} refused, {len(crds)} CRDs in sync, version {chart['version']}"
    )


if __name__ == "__main__":
    main()
