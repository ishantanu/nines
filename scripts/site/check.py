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
        self.ids = set()
        self.documentation = False
        self.repository_guide_links = []

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if "id" in attrs:
            self.ids.add(attrs["id"])
        if attrs.get("aria-label") == "Documentation chapters":
            self.documentation = True
        if (
            tag == "a"
            and "github.com/ishantanu/nines/blob/main/docs/" in attrs.get("href", "")
            and "data-doc-source" not in attrs
        ):
            self.repository_guide_links.append(attrs["href"])
        for attr in ["src", "href"]:
            if attr in attrs:
                self.urls.append(attrs[attr])


page = SITE / "index.html"
assert page.is_file(), "Build the site first: make site"
parsers = {}
for html in SITE.rglob("*.html"):
    parsed = Assets()
    parsed.feed(html.read_text())
    parsers[html.resolve()] = parsed
checked = []
for html, parsed in parsers.items():
    assert not parsed.repository_guide_links, (
        "Guide navigation must stay on the website: "
        + str(parsed.repository_guide_links)
    )
    for url in parsed.urls:
        u = urlsplit(url)
        if u.scheme or u.netloc:
            continue
        assert not u.path.startswith("/"), (
            "Root-relative URLs break GitHub project sites: " + str(html) + " " + url
        )
        dest = (html.parent / unquote(u.path)).resolve() if u.path else html
        if dest.is_dir():
            dest = dest / "index.html"
        assert dest.is_relative_to(SITE.resolve()) and dest.is_file(), (
            "Missing static asset/page: " + str(html.relative_to(SITE)) + " " + url
        )
        if u.fragment and dest in parsers:
            assert unquote(u.fragment) in parsers[dest].ids, (
                "Missing heading: " + str(html.relative_to(SITE)) + " " + url
            )
        if html == page.resolve():
            checked.append(url)
    if html != page.resolve():
        assert parsed.documentation, (
            "A documentation page is missing chapter navigation"
        )
        assert "doc-content" in parsed.ids, "A guide is missing prerendered content"
        assert "docs-page-data" in parsed.ids, "A guide is missing hydration metadata"
assert checked, "No assets found"
search = json.loads((SITE / "docs/search.json").read_text())
assert len(search) >= 30, "Public guides are missing from documentation search"
assert len({row["slug"] for row in search}) == len(search), (
    "Duplicate documentation route"
)
assert len(parsers) == len(search) + 1, "Search and published HTML routes differ"
for row in search:
    target = SITE / "docs" / row["slug"] / "index.html"
    assert target.is_file(), "Missing directly addressable guide: " + row["slug"]
assert "thresholdSeconds" in (SITE / "llms-full.txt").read_text()
assert (SITE / "examples/request-latency.slo.yaml").read_bytes() == (
    ROOT / "examples/request-latency.slo.yaml"
).read_bytes(), "Downloadable examples must match the source"
assert (
    "kind: ServiceLevelObjective" in (SITE / "docs/latency-slos/index.html").read_text()
)
assert ET.parse(SITE / "sitemap.xml").getroot().tag.endswith("urlset")
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
            "documentationPages": len(search),
            "localLinksAndFragments": "passed",
            "projectSubpath": "relative",
            "svgChecks": "passed",
            "deployed": False,
        }
    )
)
