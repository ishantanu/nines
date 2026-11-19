#!/usr/bin/env python3
"""Verify the static product site's local asset and deployment boundaries."""

import hashlib
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
        self.chapter_cache = None
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
        if tag == "div" and attrs.get("id") == "root":
            self.chapter_cache = attrs.get("data-doc-bundle")
        for attr in ["src", "href", "data-doc-bundle"]:
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
        assert "docs-page-data" not in parsed.ids, "Guides do not need hydration data"
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
    assert (target.parent / "index.md").is_file(), (
        "Missing Markdown guide: " + row["slug"]
    )
cache_files = list((SITE / "docs").glob("chapter-cache-*.json"))
assert len(cache_files) == 1, "Expected one content-addressed chapter cache"
cache_file = cache_files[0]
cache_bytes = cache_file.read_bytes()
assert len(cache_bytes) <= 3_000_000, "Chapter cache exceeded its size budget"
assert (
    cache_file.name
    == "chapter-cache-" + hashlib.sha256(cache_bytes).hexdigest()[:12] + ".json"
), "Chapter cache digest mismatch"
chapter_cache = json.loads(cache_bytes)
assert chapter_cache["version"] == 1
assert {row["slug"] for row in chapter_cache["pages"]} == {
    row["slug"] for row in search
}, "Chapter cache and published routes differ"
for row in chapter_cache["pages"]:
    target = (SITE / "docs" / row["slug"] / "index.html").resolve()
    assert parsers[target].chapter_cache, "Guide has no navigation-cache reference"
    assert (
        target.parent / parsers[target].chapter_cache
    ).resolve() == cache_file.resolve(), "Guide references the wrong chapter cache"
    assert row["main"] in target.read_text() and row["outline"] in target.read_text(), (
        "Cached fragments differ from published HTML"
    )
assert "thresholdSeconds" in (SITE / "llms-full.txt").read_text()
assert (SITE / "openapi/v1alpha1.yaml").read_bytes() == (
    ROOT / "api/openapi/v1alpha1.yaml"
).read_bytes(), "Published OpenAPI contract must match the source"
assert (SITE / "schemas/remote-mcp/v1alpha1.json").read_bytes() == (
    ROOT / "api/mcp/v1alpha1.schema.json"
).read_bytes(), "Published remote MCP schema must match the source"
assert (SITE / "examples/request-latency.slo.yaml").read_bytes() == (
    ROOT / "examples/request-latency.slo.yaml"
).read_bytes(), "Downloadable examples must match the source"
assert (
    "kind: ServiceLevelObjective" in (SITE / "docs/latency-slos/index.html").read_text()
)
sitemap = ET.parse(SITE / "sitemap.xml").getroot()
assert sitemap.tag.endswith("urlset")
canonical_base = "https://ishantanu.github.io/nines/"
locations = {element.text for element in sitemap.iter() if element.tag.endswith("loc")}
assert canonical_base in locations
for row in search:
    slug = row["slug"]
    canonical = canonical_base + "docs/" + (slug + "/" if slug else "")
    assert canonical in locations, "Missing documentation sitemap URL: " + canonical
    html = (SITE / "docs" / slug / "index.html").read_text()
    assert f'rel="canonical" href="{canonical}"' in html, (
        "Incorrect canonical URL: " + slug
    )
llms = (SITE / "llms.txt").read_text()
for url in re.findall(r"https://ishantanu\.github\.io/nines/docs/[^)\s]+", llms):
    assert url in locations, "Missing LLM index documentation URL: " + url
assert 'rel="canonical" href="' + canonical_base + '"' in page.read_text()
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
