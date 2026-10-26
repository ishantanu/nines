#!/usr/bin/env python3
"""Allowlisted public-source export. This never initializes Git or publishes."""

import argparse
import hashlib
import json
import re
import shutil
import sys
from pathlib import Path
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[2]
TREES = {
    ".github",
    "api",
    "client",
    "cmd",
    "deploy",
    "examples",
    "integration",
    "internal",
    "observability",
    "reviewskills",
    "scripts",
    "skills",
    "web",
    "third_party",
}
FILES = {
    "README.md",
    "LICENSE",
    "NOTICE",
    "THIRD_PARTY_NOTICES.md",
    "CONTRIBUTING.md",
    "SECURITY.md",
    "CHANGELOG.md",
    "Makefile",
    "Dockerfile",
    "compose.yaml",
    "compose.playground.yaml",
    "compose.playground.mimir.yaml",
    "compose.playground.ai.yaml",
    ".env.example",
    ".gitignore",
    ".dockerignore",
    "go.mod",
    "go.sum",
    ".golangci.yml",
    "pyproject.toml",
    "requirements-lint.txt",
}
EXCLUDED = {
    ".git",
    "node_modules",
    ".vite",
    ".cache",
    "__pycache__",
    ".DS_Store",
    "bin",
    "dist",
    "evidence",
    ".ruff_cache",
    ".venv-lint",
}
LOCAL = re.compile(
    r"[/]Users/[^\s/]+/|[/]home/[^\s/]+/(?:\.config|\.ssh)/|[/]private/(?:tmp|var)/"
)
PRIVATE = re.compile(
    r"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----|\b(?:ghp_|github_pat_|sk-ant-api\d+-|sk-proj-)[A-Za-z0-9_-]{20,}"
)
LINK = re.compile(r'!?\[[^\]]*\]\(([^\s)]+)(?:\s+"[^"]*")?\)')


def included(rel):
    if any(
        p in EXCLUDED or p == ".env" or p.startswith(".env.") and p != ".env.example"
        for p in rel.parts
    ):
        return False
    if rel.suffix in {".test", ".log", ".jpg", ".png", ".mp4", ".webm"}:
        return False
    if rel.parts[0] == "docs":
        return rel.as_posix() == "docs/cli.md" or rel.parts[:2] == ("docs", "public")
    if rel.parts[0] == "integration" and rel.name.startswith("result-"):
        return False
    return rel.as_posix() in FILES or rel.parts[0] in TREES


def sources(root):
    # Do not traverse excluded trees, especially dependency caches or evidence.
    import os

    for directory, names, files in os.walk(root, followlinks=False):
        for name in names:
            child = Path(directory) / name
            if (
                name not in EXCLUDED
                and child.is_symlink()
                and included(child.relative_to(root))
            ):
                raise ValueError(
                    "Refusing symlink directory: " + str(child.relative_to(root))
                )
        names[:] = sorted(
            n
            for n in names
            if n not in EXCLUDED and not (Path(directory) / n).is_symlink()
        )
        for name in sorted(files):
            p = Path(directory) / name
            rel = p.relative_to(root)
            if included(rel):
                if p.is_symlink() or not p.is_file():
                    raise ValueError("Refusing non-regular source: " + str(rel))
                yield rel, p


def inspect(root):
    selected = dict(sources(root))
    problems = []
    for name in FILES:
        if name not in {r.as_posix() for r in selected}:
            problems.append("Missing release file: " + name)
    for rel, p in selected.items():
        data = p.read_bytes()
        if b"\x00" in data:
            problems.append("Unexpected binary file: " + str(rel))
            continue
        try:
            body = data.decode("utf-8")
        except UnicodeDecodeError:
            problems.append("Non-UTF-8 release source: " + str(rel))
            continue
        if LOCAL.search(body):
            problems.append("Private absolute path: " + str(rel))
        if PRIVATE.search(body):
            problems.append("Credential marker: " + str(rel))
        if rel.suffix == ".md":
            for match in LINK.finditer(body):
                raw = match.group(1).strip("<>")
                url = urlsplit(raw)
                if url.scheme or not url.path or raw.startswith("#"):
                    continue
                dest = (root / rel.parent / unquote(url.path)).resolve()
                try:
                    destrel = dest.relative_to(root.resolve())
                except ValueError:
                    problems.append("Link leaves public root: " + str(rel))
                    continue
                if dest.is_dir():
                    exists = any(r.is_relative_to(destrel) for r in selected)
                else:
                    exists = destrel in selected
                if not exists:
                    problems.append(
                        "Link excluded or missing: " + str(rel) + " -> " + raw
                    )
    return selected, sorted(set(problems))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=ROOT)
    parser.add_argument("--out", type=Path)
    parser.add_argument(
        "--publication",
        action="store_true",
        help="also require a public GitHub Go module path",
    )
    args = parser.parse_args()
    root = args.root.resolve()
    selected, problems = inspect(root)
    if args.publication:
        module = (root / "go.mod").read_text().splitlines()[0]
        if not re.fullmatch(
            r"module github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+(?:/v\d+)?", module
        ):
            problems.append(
                "Set the public GitHub module/import path before publication"
            )
    if problems:
        print(json.dumps({"status": "blocked", "problems": problems}, indent=2))
        return 1
    if args.out:
        dest = args.out.resolve()
        if dest.exists():
            parser.error(
                "output must not exist; review or remove the old generated export first"
            )
        if (
            root == dest
            or dest.is_relative_to(root)
            and dest.parts[len(root.parts)] != "dist"
        ):
            parser.error(
                "use a new dist/ subdirectory or a directory outside the source tree"
            )
        dest.mkdir(parents=True)
        manifest = []
        for rel, src in selected.items():
            dst = dest / rel
            dst.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(src, dst)
            manifest.append(
                {
                    "path": rel.as_posix(),
                    "bytes": dst.stat().st_size,
                    "sha256": hashlib.sha256(dst.read_bytes()).hexdigest(),
                }
            )
        dest.with_name(dest.name + "-manifest.json").write_text(
            json.dumps(manifest, indent=2) + "\n"
        )
    print(
        json.dumps(
            {
                "status": "passed",
                "sourceFiles": len(selected),
                "publication": args.publication,
                "published": False,
            }
        )
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
