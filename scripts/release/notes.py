#!/usr/bin/env python3
"""Prepare release notes from docs/public/alpha-release-notes.md.

The guide links to other guides with relative paths, which work on the
documentation site but not in a GitHub release. This rewrites each relative
Markdown link to the published page, so the same source serves both.
"""

import argparse
import re
import sys

SITE = "https://ishantanu.github.io/nines/docs/"

# A Markdown link target that is a relative .md path, with an optional anchor.
# Absolute URLs, mail links and in-page anchors are left alone.
LINK = re.compile(r"\]\((?![a-z][a-z0-9+.-]*:|#|/)([^)\s#]+)\.md(#[^)\s]*)?\)")


def page_url(path: str, anchor: str, site: str) -> str:
    path = path.removeprefix("./")
    if ".." in path.split("/"):
        raise ValueError(f"release notes link leaves docs/public: {path}.md")
    if path == "index":
        slug = ""
    elif path.endswith("/index"):
        slug = path.removesuffix("index")
    else:
        slug = path + "/"
    return f"{site}{slug}{anchor or ''}"


def rewrite(text: str, site: str = SITE) -> str:
    return LINK.sub(lambda m: f"]({page_url(m.group(1), m.group(2), site)})", text)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source")
    parser.add_argument("--site", default=SITE)
    args = parser.parse_args()
    with open(args.source, encoding="utf-8") as f:
        sys.stdout.write(rewrite(f.read(), args.site))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
