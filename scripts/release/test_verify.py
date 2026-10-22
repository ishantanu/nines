import hashlib
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location(
    "release_verify", Path(__file__).with_name("verify.py")
)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
VERSION = "v0.1.0-alpha.1"
COMMIT = "a" * 40


class ReleaseVerificationTests(unittest.TestCase):
    def fixture(self, root):
        manifest = {
            "module": "github.com/ishantanu/nines",
            "version": VERSION,
            "commit": COMMIT,
            "cleanCheckout": True,
            "targets": list(module.TARGETS),
            "artifacts": [],
        }
        for target in module.TARGETS:
            name = f"nines_{VERSION}_{target.replace('/', '_')}.tar.gz"
            data = target.encode()
            (root / name).write_bytes(data)
            manifest["artifacts"].append(
                {
                    "file": name,
                    "bytes": len(data),
                    "sha256": hashlib.sha256(data).hexdigest(),
                }
            )
        (root / "build.json").write_text(json.dumps(manifest))
        self.checksums(root)
        return manifest

    def checksums(self, root):
        (root / "SHA256SUMS").write_text(
            "".join(
                hashlib.sha256(p.read_bytes()).hexdigest() + "  " + p.name + "\n"
                for p in sorted(root.iterdir())
                if p.name != "SHA256SUMS"
            )
        )

    def test_complete_release_matches_expected_commit(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.fixture(root)
            self.assertEqual(module.verify(root, VERSION, COMMIT)["commit"], COMMIT)

    def test_wrong_version_or_commit_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            self.fixture(root)
            for version, commit in [
                ("v0.1.0-alpha.2", COMMIT),
                (VERSION, "b" * 40),
                ("../alpha", COMMIT),
            ]:
                with (
                    self.subTest(version=version, commit=commit),
                    self.assertRaises(ValueError),
                ):
                    module.verify(root, version, commit)

    def test_archive_corruption_rejected_even_with_updated_checksum(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            manifest = self.fixture(root)
            (root / manifest["artifacts"][0]["file"]).write_bytes(b"changed")
            self.checksums(root)
            with self.assertRaises(ValueError):
                module.verify(root, VERSION, COMMIT)

    def test_untrusted_metadata_rejected(self):
        for field, value in [
            ("cleanCheckout", False),
            ("targets", ["linux/amd64"]),
            ("module", "example.com/other"),
        ]:
            with self.subTest(field=field), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                manifest = self.fixture(root)
                manifest[field] = value
                (root / "build.json").write_text(json.dumps(manifest))
                self.checksums(root)
                with self.assertRaises(ValueError):
                    module.verify(root, VERSION, COMMIT)

    def test_missing_extra_and_linked_assets_rejected(self):
        for mode in ["missing", "extra", "symlink"]:
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                manifest = self.fixture(root)
                asset = root / manifest["artifacts"][0]["file"]
                if mode == "extra":
                    (root / "unexpected.txt").write_text("extra")
                else:
                    asset.unlink()
                    if mode == "symlink":
                        asset.symlink_to(root / "build.json")
                with self.assertRaises(ValueError):
                    module.verify(root, VERSION, COMMIT)

    def test_duplicate_or_changed_checksums_rejected(self):
        for mode in ["duplicate", "changed"]:
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                self.fixture(root)
                checksums = root / "SHA256SUMS"
                body = checksums.read_text()
                checksums.write_text(
                    body + body.splitlines()[0] + "\n"
                    if mode == "duplicate"
                    else body.replace(body[:64], "0" * 64, 1)
                )
                with self.assertRaises(ValueError):
                    module.verify(root, VERSION, COMMIT)


if __name__ == "__main__":
    unittest.main()
