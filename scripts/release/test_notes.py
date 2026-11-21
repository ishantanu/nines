import importlib.util
import re
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location(
    "notes", Path(__file__).with_name("notes.py")
)
notes = importlib.util.module_from_spec(spec)
spec.loader.exec_module(notes)

SITE = "https://ishantanu.github.io/nines/docs/"
ROOT = Path(__file__).resolve().parents[2]


class ReleaseNotesLinkTests(unittest.TestCase):
    def test_relative_guides_become_site_pages(self):
        cases = {
            "[types](slo-types.md)": f"[types]({SITE}slo-types/)",
            "[table](slo-types.md#which-providers-support-which-types)": f"[table]({SITE}slo-types/#which-providers-support-which-types)",
            "[backup](deployment/backup-and-restore.md)": f"[backup]({SITE}deployment/backup-and-restore/)",
            "[home](index.md)": f"[home]({SITE})",
            "[deploy](deployment/index.md)": f"[deploy]({SITE}deployment/)",
            "[here](./intents.md)": f"[here]({SITE}intents/)",
        }
        for source, want in cases.items():
            self.assertEqual(notes.rewrite(source), want, source)

    def test_other_links_are_unchanged(self):
        for text in [
            "[issue](https://github.com/ishantanu/nines/issues/34)",
            "[guide](https://github.com/ishantanu/nines/blob/v1/docs/public/x.md)",
            "[top](#upgrade-notes)",
            "[mail](mailto:someone@example.com)",
            "a plain mention of slo-types.md in text",
        ]:
            self.assertEqual(notes.rewrite(text), text)

    def test_links_leaving_the_docs_are_refused(self):
        with self.assertRaises(ValueError):
            notes.rewrite("[readme](../../README.md)")

    def test_published_notes_have_no_relative_markdown_links(self):
        source = (ROOT / "docs/public/alpha-release-notes.md").read_text()
        out = notes.rewrite(source)
        self.assertGreater(out.count(SITE), 5)
        self.assertEqual(re.findall(r"\]\((?!https?://|#)[^)]*\)", out), [])
        # Every rewritten page exists as a guide in docs/public.
        for slug in re.findall(re.escape(SITE) + r"([^)#]*)", out):
            page = slug.rstrip("/") or "index"
            self.assertTrue(
                (ROOT / "docs/public" / f"{page}.md").exists()
                or (ROOT / "docs/public" / page / "index.md").exists(),
                page,
            )


if __name__ == "__main__":
    unittest.main()
