#!/usr/bin/env python3
"""Verify release assets against their expected version, commit and checksums."""

import argparse
import hashlib
import json
import re
import tarfile
import zipfile
from pathlib import Path, PurePosixPath

TARGETS = (
    "linux/amd64",
    "linux/arm64",
    "darwin/amd64",
    "darwin/arm64",
    "windows/amd64",
    "windows/arm64",
)


def archive_name(version, target):
    """Windows users expect .zip; everyone else gets a reproducible .tar.gz."""
    goos, goarch = target.split("/")
    ext = "zip" if goos == "windows" else "tar.gz"
    return f"nines_{version}_{goos}_{goarch}.{ext}"


def binaries(target):
    """Programs in each archive. The operator runs only in Linux containers,
    so Windows archives carry the CLI, server and MCP server."""
    if target.startswith("windows/"):
        return ["nines.exe", "nines-server.exe", "nines-mcp.exe"]
    return ["nines", "nines-server", "nines-mcp", "nines-operator"]


def entries(path):
    """(name, size, is_regular_file, is_executable) for each archive entry."""
    if path.name.endswith(".zip"):
        with zipfile.ZipFile(path) as archive:
            return [
                (i.filename, i.file_size, not i.is_dir(), i.filename.endswith(".exe"))
                for i in archive.infolist()
            ]
    with tarfile.open(path, "r:gz") as archive:
        return [
            (e.name, e.size, e.isfile(), bool(e.mode & 0o111))
            for e in archive.getmembers()
        ]


def verify(directory, version, commit):
    directory = Path(directory)
    if not re.fullmatch(r"v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?", version):
        raise ValueError("invalid release version")
    if not re.fullmatch(r"[0-9a-f]{40,64}", commit):
        raise ValueError("invalid expected commit")
    expected = {archive_name(version, target): target for target in TARGETS}
    files = set(expected) | {"build.json", "SHA256SUMS"}
    if {p.name for p in directory.iterdir()} != files:
        raise ValueError(
            "release must contain exactly one archive per supported platform, "
            "build.json and SHA256SUMS"
        )
    if any(
        not (directory / name).is_file() or (directory / name).is_symlink()
        for name in files
    ):
        raise ValueError("release assets must be regular files")
    manifest = json.loads((directory / "build.json").read_text())
    if (
        manifest.get("version") != version
        or manifest.get("commit") != commit
        or manifest.get("cleanCheckout") is not True
        or manifest.get("module") != "github.com/ishantanu/nines"
        or sorted(manifest.get("targets", [])) != sorted(TARGETS)
    ):
        raise ValueError(
            "build metadata does not match the release version, commit or targets"
        )
    artifacts = manifest.get("artifacts", [])
    if len(artifacts) != len(expected) or {a.get("file") for a in artifacts} != set(
        expected
    ):
        raise ValueError("archive inventory does not match supported targets")
    for artifact in artifacts:
        data = (directory / artifact["file"]).read_bytes()
        if len(data) != artifact.get("bytes") or hashlib.sha256(
            data
        ).hexdigest() != artifact.get("sha256"):
            raise ValueError("archive differs from build metadata: " + artifact["file"])
        listed = entries(directory / artifact["file"])
        names = [name for name, *_ in listed]
        if len(names) != len(set(names)):
            raise ValueError("duplicate archive entry")
        for name, size, regular, _ in listed:
            path = PurePosixPath(name)
            if (
                not regular
                or path.is_absolute()
                or ".." in path.parts
                or "\\" in name
                or ":" in name
                or size > 128 << 20
            ):
                raise ValueError("unsafe archive entry")
        executable = {name: runnable for name, _, _, runnable in listed}
        for binary in binaries(expected[artifact["file"]]):
            if not executable.get(binary):
                raise ValueError("missing or nonexecutable release binary: " + binary)
    checksums = {}
    for line in (directory / "SHA256SUMS").read_text().splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9_.-]+)", line)
        if not match or match[2] in checksums:
            raise ValueError("invalid or duplicate checksum entry")
        checksums[match[2]] = match[1]
    if set(checksums) != set(expected) | {"build.json"}:
        raise ValueError("checksum inventory does not match release assets")
    for name, digest in checksums.items():
        if hashlib.sha256((directory / name).read_bytes()).hexdigest() != digest:
            raise ValueError("checksum mismatch: " + name)
    return manifest


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--directory", type=Path, required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--commit", required=True)
    args = parser.parse_args()
    manifest = verify(args.directory, args.version, args.commit)
    print(
        json.dumps(
            {
                "status": "verified",
                "version": manifest["version"],
                "commit": manifest["commit"],
                "archives": len(manifest["artifacts"]),
            }
        )
    )


if __name__ == "__main__":
    main()
