#!/usr/bin/env python3
"""Collect supplied dependency notices from installed Go and npm modules."""

import argparse
import hashlib
import json
import re
import shutil
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
NAMES = re.compile(r"^(?:licen[cs]e|notice|copying|copyright)(?:[._-].*)?$", re.I)


def json_stream(raw):
    decoder = json.JSONDecoder()
    while raw.strip():
        raw = raw.lstrip()
        row, end = decoder.raw_decode(raw)
        yield row
        raw = raw[end:]


def collect(root, dest, targets=None, go_only=False, packages=None):
    if dest.exists():
        raise ValueError("license output must be a new directory")
    dest.mkdir(parents=True)
    inventory, missing = [], []
    overrides = json.loads((root / "third_party/license-sources.json").read_text())

    def record(
        kind,
        name,
        version,
        directory,
        license_id="",
        notice_source="",
        override=None,
        required=True,
    ):
        directory = Path(directory)
        files = sorted(
            p for p in directory.iterdir() if p.is_file() and NAMES.match(p.name)
        )
        if override:
            notice = root / "third_party" / override["file"]
            if (
                not notice.resolve().is_relative_to((root / "third_party").resolve())
                or hashlib.sha256(notice.read_bytes()).hexdigest() != override["sha256"]
            ):
                raise ValueError("Unreviewed or modified notice override: " + name)
            files = [notice]
        key = re.sub(r"[^A-Za-z0-9_.-]", "_", name) + "_" + version
        entry = {
            "ecosystem": kind,
            "name": name,
            "version": version,
            "declaredLicense": license_id,
            "noticeSourcePackage": notice_source or name + "_" + version,
            "notices": [],
        }
        for file in files:
            if file.stat().st_size > 2 << 20:
                raise ValueError("Oversized dependency notice: " + name)
            out = dest / kind / key / file.name
            out.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(file, out)
            entry["notices"].append(out.relative_to(dest).as_posix())
        if override:
            entry["noticeProvenance"] = override
        if not files and not required:
            entry["noticeStatus"] = (
                "No supplied notice in development-only package; not bundled in release runtime"
            )
        if not files and required:
            missing.append(kind + ":" + name + "_" + version)
        inventory.append(entry)

    import os

    modules = {}
    for target in targets or [None]:
        env = dict(os.environ, CGO_ENABLED="0")
        if target:
            env["GOOS"], env["GOARCH"] = target.split("/")
        raw = subprocess.check_output(
            [
                "go",
                "list",
                "-deps",
                "-json",
                *(
                    packages
                    or ["./cmd/nines", "./cmd/server", "./cmd/mcp", "./cmd/operator"]
                ),
            ],
            cwd=root,
            text=True,
            env=env,
        )
        for package in json_stream(raw):
            module = package.get("Module")
            if module and not module.get("Main"):
                module = module.get("Replace", module)
                modules[(module["Path"], module.get("Version", "local"))] = module
    for module in modules.values():
        record("go", module["Path"], module.get("Version", "local"), module["Dir"])
    goroot = subprocess.check_output(
        ["go", "env", "GOROOT"], cwd=root, text=True
    ).strip()
    goversion = subprocess.check_output(
        ["go", "env", "GOVERSION"], cwd=root, text=True
    ).strip()
    toolchain_dir = Path(goroot)
    if not any(f.is_file() and NAMES.match(f.name) for f in toolchain_dir.iterdir()):
        # Homebrew keeps the distribution license beside libexec, not inside it.
        toolchain_dir = toolchain_dir.parent
    record(
        "go-toolchain", "Go standard library", goversion, toolchain_dir, "BSD-3-Clause"
    )
    lock = (
        {"packages": {}}
        if go_only
        else json.loads((root / "web/package-lock.json").read_text())
    )
    installed = []
    for path, package in sorted(lock["packages"].items()):
        if not path:
            continue
        directory = root / "web" / path
        if not directory.is_dir():
            if package.get("optional"):
                continue
            raise ValueError("npm dependency not installed: " + path)
        metadata = json.loads((directory / "package.json").read_text())
        installed.append((directory, metadata, package))

    def repository(metadata):
        value = metadata.get("repository", "")
        if isinstance(value, dict):
            value = value.get("url", "")
        value = value.removeprefix("git+").removesuffix(".git")
        for prefix in ["ssh://git@github.com/", "git://github.com/"]:
            if value.startswith(prefix):
                value = "https://github.com/" + value[len(prefix) :]
        return value

    for directory, metadata, package in installed:
        notice_dir, notice_source = directory, ""
        if not any(f.is_file() and NAMES.match(f.name) for f in directory.iterdir()):
            # Platform-specific binary packages sometimes ship only a binary.
            # Use supplied notices from the exact-version/same-repository peer,
            # retaining explicit provenance for the mandatory maintainer review.
            for peer_dir, peer, peer_lock in installed:
                if (
                    repository(metadata)
                    and repository(peer) == repository(metadata)
                    and peer["version"] == metadata["version"]
                    and package.get("license") == peer_lock.get("license")
                    and any(
                        f.is_file() and NAMES.match(f.name) for f in peer_dir.iterdir()
                    )
                ):
                    notice_dir = peer_dir
                    notice_source = peer["name"] + "@" + peer["version"]
                    break
        override = None
        if not any(f.is_file() and NAMES.match(f.name) for f in notice_dir.iterdir()):
            override = next(
                (
                    o
                    for o in overrides
                    if o["version"] == metadata["version"]
                    and (
                        o.get("package") == metadata["name"]
                        or not o.get("package")
                        and o.get("repository") == repository(metadata)
                    )
                ),
                None,
            )
        record(
            "npm",
            metadata["name"],
            metadata["version"],
            notice_dir,
            package.get("license", ""),
            notice_source,
            override,
            not package.get("dev", False),
        )
    shadcn = root / "web/public/shadcn-license.txt"
    shadcn_out = dest / "incorporated/shadcn-ui/LICENSE"
    shadcn_out.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(shadcn, shadcn_out)
    inventory.append(
        {
            "ecosystem": "incorporated",
            "name": "shadcn/ui",
            "version": "vendored-components",
            "declaredLicense": "MIT",
            "sourceFile": "web/public/shadcn-license.txt",
            "notices": ["incorporated/shadcn-ui/LICENSE"],
        }
    )
    if missing:
        raise ValueError(
            "Review missing upstream notices before release: " + ", ".join(missing)
        )
    (dest / "DEPENDENCIES.json").write_text(
        json.dumps(
            sorted(inventory, key=lambda x: (x["ecosystem"], x["name"], x["version"])),
            indent=2,
        )
        + "\n"
    )
    return inventory


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=ROOT)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument(
        "--go-only",
        action="store_true",
        help="omit web dependencies for binaries without an embedded UI",
    )
    parser.add_argument(
        "--package",
        action="append",
        dest="packages",
        help="Go main package to inspect; may be repeated",
    )
    args = parser.parse_args()
    items = collect(
        args.root.resolve(),
        args.out.resolve(),
        go_only=args.go_only,
        packages=args.packages,
    )
    print(
        json.dumps(
            {
                "dependencies": len(items),
                "noticeCollection": "passed",
                "legalReview": "still required",
            }
        )
    )
