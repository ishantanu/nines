#!/usr/bin/env python3
"""Prepare reviewable CLI/server/MCP archives. Never publishes or tags."""

import argparse
import gzip
import hashlib
import io
import json
import os
import re
import subprocess
import tarfile
import tempfile
from pathlib import Path

from licenses import collect
from source import inspect

ROOT = Path(__file__).resolve().parents[2]
SUPPORTED = {"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--version", required=True)
    p.add_argument(
        "--targets", default="linux/amd64,linux/arm64,darwin/amd64,darwin/arm64"
    )
    p.add_argument("--out", type=Path, default=ROOT / "dist/releases")
    p.add_argument("--require-clean", action="store_true")
    a = p.parse_args()
    if not re.fullmatch(r"v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?", a.version):
        p.error("provide an explicit release version such as v0.1.0-alpha.1")
    targets = a.targets.split(",")
    if len(set(targets)) != len(targets) or not set(targets) <= SUPPORTED:
        p.error("unsupported or duplicate build target")
    _, errors = inspect(ROOT)
    if errors:
        raise SystemExit("\n".join(errors))
    output = a.out.resolve()
    if output.exists():
        p.error("output must be new; review or remove old generated assets first")
    try:
        commit = subprocess.check_output(
            ["git", "rev-parse", "HEAD"], cwd=ROOT, text=True, stderr=subprocess.DEVNULL
        ).strip()
        epoch = int(
            subprocess.check_output(
                ["git", "show", "-s", "--format=%ct", "HEAD"], cwd=ROOT, text=True
            )
        )
        clean = not subprocess.check_output(
            ["git", "status", "--porcelain"], cwd=ROOT, text=True
        ).strip()
    except subprocess.CalledProcessError:
        commit, epoch, clean = "unknown", 0, False
    from datetime import datetime, timezone

    if a.require_clean and (
        not clean
        or commit == "unknown"
        or Path(
            subprocess.check_output(
                ["git", "rev-parse", "--show-toplevel"], cwd=ROOT, text=True
            ).strip()
        ).resolve()
        != ROOT
    ):
        p.error(
            "candidate workflow requires a clean repository rooted at this source tree"
        )
    date = (
        datetime.fromtimestamp(epoch, timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        if epoch
        else "unknown"
    )
    module = subprocess.check_output(["go", "list", "-m"], cwd=ROOT, text=True).strip()
    goversion = subprocess.check_output(
        ["go", "env", "GOVERSION"], cwd=ROOT, text=True
    ).strip()
    # A preview with no Git metadata is allowed locally, never represented as clean.
    with tempfile.TemporaryDirectory(prefix="nines-release-") as tmp:
        work = Path(tmp)
        notices = work / "LICENSES"
        dependencies = collect(ROOT, notices, targets)
        output.mkdir(parents=True)
        manifest = {
            "module": module,
            "goVersion": goversion,
            "version": a.version,
            "commit": commit,
            "builtAt": date,
            "cleanCheckout": clean,
            "published": False,
            "targets": targets,
            "dependencies": len(dependencies),
            "artifacts": [],
        }
        for target in targets:
            goos, goarch = target.split("/")
            tree = work / (goos + "_" + goarch)
            tree.mkdir()
            env = dict(os.environ, GOOS=goos, GOARCH=goarch, CGO_ENABLED="0")
            flags = "-s -w " + " ".join(
                "-X " + module + "/internal/buildinfo." + k + "=" + v
                for k, v in [("Version", a.version), ("Commit", commit), ("Date", date)]
            )
            for binary, package in [
                ("nines", "./cmd/nines"),
                ("nines-server", "./cmd/server"),
                ("nines-mcp", "./cmd/mcp"),
            ]:
                subprocess.run(
                    [
                        "go",
                        "build",
                        "-trimpath",
                        "-buildvcs=false",
                        "-ldflags=" + flags,
                        "-o",
                        str(tree / binary),
                        package,
                    ],
                    cwd=ROOT,
                    env=env,
                    check=True,
                )
            name = f"nines_{a.version}_{goos}_{goarch}.tar.gz"
            archive = output / name
            sources = [(f.name, f) for f in sorted(tree.iterdir())]
            readme = work / "BINARY_README.md"
            readme.write_text(f"""# Nines {a.version}

This is a pre-release binary candidate. No production support or stable API compatibility is declared.

## Try the isolated demo

```sh
./nines version --agent
./nines-server -demo
```

Open http://127.0.0.1:8080 and use `admin` / `admin`. This sign-in is a local demonstration, not an access-control boundary. The demo resets on restart and contacts no external provider.

## Use the offline CLI

```sh
./nines init slo --name checkout --window 28d --out checkout.yaml
./nines validate --offline -f checkout.yaml --agent
./nines rules generate --offline -f examples/api-availability.slo.yaml --tenant commerce
```

## Source-based deployments

The Compose playground, container builds, Kubernetes operator image and development/site commands require a source checkout from `https://github.com/ishantanu/nines`. This archive contains native CLI/server/MCP binaries, configuration examples and reference documentation; it is not the full source tree. Do not run source-build commands from the extracted archive. For the native Linux service guide, install the archive's `nines-server` binary in place of the source-built `nines-server-linux-amd64`, after confirming the archive matches the host architecture.

For persistence, supply a PostgreSQL `DATABASE_URL` and random `API_TOKEN` in the server environment, then run `./nines-server`. Configure providers and identity explicitly; binary execution does not load `.env`. The collecting server is single-process for this MVP.

[CLI guide](docs/cli.md), [configuration](docs/public/configuration.md), [operational limits](docs/public/operations.md), [experimental features](docs/public/experimental.md) and [security reporting](SECURITY.md).

The source module is `github.com/ishantanu/nines`. Local previews are unpublished and may lack Git commit metadata. Build metadata and archive checksums accompany this candidate. Verify them before executing downloaded binaries. Dependency licenses are under `LICENSES/`.
""")
            sources += [("README.md", readme)]
            sources += [
                (f, ROOT / f)
                for f in [
                    "LICENSE",
                    "NOTICE",
                    "THIRD_PARTY_NOTICES.md",
                    "SECURITY.md",
                    "CONTRIBUTING.md",
                    "CHANGELOG.md",
                ]
            ]
            sources += [
                ("examples/" + f.relative_to(ROOT / "examples").as_posix(), f)
                for f in sorted((ROOT / "examples").rglob("*"))
                if f.is_file() and f.suffix in {".yaml", ".json"}
            ]
            sources += [
                ("deploy/" + f.relative_to(ROOT / "deploy").as_posix(), f)
                for f in sorted((ROOT / "deploy").rglob("*"))
                if f.is_file() and f.suffix in {".yaml", ".service"}
            ]
            sources += [
                ("LICENSES/" + f.relative_to(notices).as_posix(), f)
                for f in sorted(notices.rglob("*"))
                if f.is_file()
            ]
            sources += [("docs/cli.md", ROOT / "docs/cli.md")]
            sources += [
                ("docs/public/" + f.relative_to(ROOT / "docs/public").as_posix(), f)
                for f in sorted((ROOT / "docs/public").rglob("*"))
                if f.is_file() and f.suffix in {".md", ".svg"}
            ]
            # Stable ordering, timestamps, owner and gzip headers for identical inputs.
            with (
                archive.open("wb") as raw,
                gzip.GzipFile(
                    filename="", fileobj=raw, mode="wb", mtime=epoch
                ) as zipped,
                tarfile.open(fileobj=zipped, mode="w") as tar,
            ):
                for relative, path in sorted(sources):
                    body = path.read_bytes()
                    entry = tarfile.TarInfo(relative)
                    entry.size = len(body)
                    entry.mtime = epoch
                    entry.mode = (
                        0o755
                        if relative in {"nines", "nines-server", "nines-mcp"}
                        else 0o644
                    )
                    tar.addfile(entry, io.BytesIO(body))
            manifest["artifacts"].append(
                {
                    "file": name,
                    "sha256": hashlib.sha256(archive.read_bytes()).hexdigest(),
                    "bytes": archive.stat().st_size,
                }
            )
        (output / "build.json").write_text(json.dumps(manifest, indent=2) + "\n")
        checked = sorted(output.iterdir())
        (output / "SHA256SUMS").write_text(
            "".join(
                hashlib.sha256(f.read_bytes()).hexdigest() + "  " + f.name + "\n"
                for f in checked
                if f.is_file()
            )
        )
        print(
            json.dumps(
                {
                    "status": "prepared",
                    "directory": str(output),
                    "targets": targets,
                    "cleanCheckout": clean,
                    "published": False,
                }
            )
        )


if __name__ == "__main__":
    main()
