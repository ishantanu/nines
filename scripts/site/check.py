#!/usr/bin/env python3
"""Verify the static product site's local asset and deployment boundaries."""

import json
import re
import xml.etree.ElementTree as ET
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[2]
SITE = ROOT / "dist/site"


class Assets(HTMLParser):
    def __init__(self):
        super().__init__()
        self.urls = []

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        for attr in ["src", "href"]:
            if attr in attrs:
                self.urls.append(attrs[attr])


page = SITE / "index.html"
assert page.is_file(), "Build the site first: make site"
parser = Assets()
parser.feed(page.read_text())
checked = []
for url in parser.urls:
    u = urlsplit(url)
    if u.scheme or u.netloc or not u.path:
        continue
    assert not u.path.startswith("/"), (
        "Root-relative URLs break GitHub project sites: " + url
    )
    dest = (SITE / unquote(u.path)).resolve()
    assert dest.is_relative_to(SITE.resolve()) and dest.is_file(), (
        "Missing static asset: " + url
    )
    checked.append(url)
assert checked, "No assets found"
assert "./nines-architecture.svg" in checked, "Architecture image must be prerendered"
assert (SITE / "nines-architecture.svg").read_bytes() == (
    ROOT / "docs/public/brand/nines-architecture.svg"
).read_bytes(), "Publish the canonical architecture SVG without drift"
assert "<h1" in page.read_text() and "Critical user journey" in page.read_text(), (
    "The page must be prerendered for readers without JavaScript"
)
assert (SITE / ".nojekyll").is_file()
assert "shadcn/ui" in (SITE / "third-party-notices.txt").read_text()
assert "Nines application license" in (SITE / "third-party-notices.txt").read_text()
for p in (ROOT / "docs/public/brand").glob("*.svg"):
    tree = ET.parse(p)
    for element in tree.iter():
        assert element.tag.rsplit("}", 1)[-1] not in {"script", "foreignObject"}
        assert all(not attr.lower().startswith("on") for attr in element.attrib)
        assert all(
            not ("href" in attr and (":" in value or value.startswith("//")))
            for attr, value in element.attrib.items()
        )
assert not re.search(r"\u2014", (ROOT / "web/site/App.tsx").read_text()), (
    "Use plain punctuation in product copy"
)
print(
    json.dumps(
        {
            "status": "passed",
            "staticAssets": len(checked),
            "projectSubpath": "relative",
            "svgChecks": "passed",
            "deployed": False,
        }
    )
)
