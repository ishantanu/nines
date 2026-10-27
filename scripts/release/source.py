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
    "compose.playground.jev.yaml",
    "compose.playground.topology.yaml",
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


# Only these vendored, licensed font bytes may bypass text-source inspection.
# The embedded build is allowed to rename them with Vite's content hash.
FONT_ASSETS = {
    "atkinson-hyperlegible-next-latin.woff2": (
        "1e4cea71d75ec427581d6259fc07148a2e60d60d16cabf4b4f5360487b3f9dc3",
        "OFL-Atkinson-Hyperlegible-Next.txt",
    ),
    "barlow-condensed-400-latin.woff2": (
        "0e2a408159b32d8105bcd1a0a4d6222fcee375a84fb62703c614ce34d8bb68bb",
        "OFL-Barlow-Condensed.txt",
    ),
    "barlow-condensed-600-latin.woff2": (
        "5bc49d41f32810a114765314392332ce2acae2276635eee265a50319a12c1b49",
        "OFL-Barlow-Condensed.txt",
    ),
    "barlow-condensed-700-latin.woff2": (
        "e3e520cb7468f2efd60bc2ce96694567c3312f4ef414fffbe605e3ad146b24e6",
        "OFL-Barlow-Condensed.txt",
    ),
}


# Reviewed synthetic product media; arbitrary screenshots remain excluded.
PRODUCT_IMAGES = {
    "docs/public/brand/nines-overview.jpg": "2d71e77e100cb9184d88e70b375c29f683954a0bc60668a91039e6e6c546a239",
}


def approved_font(rel, data, root):
    if rel.parent not in {Path("web/site/fonts"), Path("internal/httpapi/web/assets")}:
        return False
    if not data.startswith(b"wOF2"):
        return False
    for name, (digest, license_name) in FONT_ASSETS.items():
        source_name = rel.parent == Path("web/site/fonts") and rel.name == name
        built_name = rel.parent == Path("internal/httpapi/web/assets") and re.fullmatch(
            re.escape(Path(name).stem) + r"-[A-Za-z0-9_-]+\.woff2", rel.name
        )
        if (source_name or built_name) and hashlib.sha256(data).hexdigest() == digest:
            return (root / "web/site/fonts" / license_name).is_file()
    return False


def included(rel):
    if any(
        p in EXCLUDED or p == ".env" or p.startswith(".env.") and p != ".env.example"
        for p in rel.parts
    ):
        return False
    if rel.suffix in {".test", ".log", ".jpg", ".png", ".mp4", ".webm"}:
        return rel.as_posix() in PRODUCT_IMAGES
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
        if rel.as_posix() in PRODUCT_IMAGES:
            if (
                data.startswith(b"\xff\xd8\xff")
                and hashlib.sha256(data).hexdigest() == PRODUCT_IMAGES[rel.as_posix()]
            ):
                continue
            problems.append("Unapproved product image: " + str(rel))
            continue
        if approved_font(rel, data, root):
            continue
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
