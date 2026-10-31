import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location(
    "public_source", Path(__file__).with_name("source.py")
)
source = importlib.util.module_from_spec(spec)
spec.loader.exec_module(source)


class PublicSourceTests(unittest.TestCase):
    def fixture(self, root):
        for name in source.FILES:
            (root / name).write_text("")
        (root / "internal").mkdir()
        (root / "internal/example.go").write_text("package example\n")

    def test_private_inputs_are_not_exported(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.fixture(root)
            (root / ".env").write_text("private")
            (root / ".nines-playground.conf").write_text(
                "providers=/private/local/tenants.yaml\n"
            )
            (root / ".vscode").mkdir()
            (root / ".vscode/mcp.json").write_text("private")
            (root / "docs").mkdir()
            (root / "docs/old-proof.json").write_text("private")
            selected, errors = source.inspect(root)
            self.assertFalse(errors)
            self.assertNotIn(Path(".env"), selected)
            self.assertNotIn(Path(".nines-playground.conf"), selected)
            self.assertIn(Path("playground"), selected)
            self.assertNotIn(Path(".vscode/mcp.json"), selected)
            self.assertNotIn(Path("docs/old-proof.json"), selected)
            self.assertIn(Path("internal/example.go"), selected)

    def test_private_paths_credentials_and_broken_links_fail(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.fixture(root)
            cases = [
                "/" + "Users" + "/example/private",
                "ghp_" + "x" * 30,
                "[missing](missing.md)",
            ]
            for body in cases:
                (root / "README.md").write_text(body)
                self.assertTrue(source.inspect(root)[1])

    def test_only_reviewed_synthetic_product_media_is_exported(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.fixture(root)
            name = next(iter(source.PRODUCT_IMAGES))
            image = root / name
            image.parent.mkdir(parents=True)
            image.write_bytes((ROOT / name).read_bytes())
            selected, errors = source.inspect(root)
            self.assertFalse(errors)
            self.assertIn(Path(name), selected)
            arbitrary = image.with_name("private-screenshot.png")
            arbitrary.write_bytes(image.read_bytes())
            self.assertNotIn(arbitrary.relative_to(root), source.inspect(root)[0])
            image.write_bytes(image.read_bytes() + b"unreviewed payload")
            self.assertTrue(source.inspect(root)[1])

    def test_only_exact_licensed_font_assets_are_exported(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.fixture(root)
            name = next(iter(source.FONT_ASSETS))
            _, license_name = source.FONT_ASSETS[name]
            fonts = root / "web/site/fonts"
            fonts.mkdir(parents=True)
            font = fonts / name
            original = (ROOT / "web/site/fonts" / name).read_bytes()
            font.write_bytes(original)
            license_file = fonts / license_name
            license_file.write_text("Licensed typeface")
            built = root / "internal/httpapi/web/assets"
            built.mkdir(parents=True)
            (built / (Path(name).stem + "-buildhash.woff2")).write_bytes(original)
            self.assertFalse(source.inspect(root)[1])
            font.write_bytes(original + b"unexpected appended payload")
            self.assertTrue(source.inspect(root)[1])
            font.write_bytes(original)
            license_file.unlink()
            self.assertTrue(source.inspect(root)[1])
            license_file.write_text("Licensed typeface")
            (root / "internal/private.woff2").write_bytes(original)
            self.assertTrue(source.inspect(root)[1])

    def test_symlink_cannot_pull_external_content(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "source"
            root.mkdir()
            self.fixture(root)
            outside = root.parent / "private"
            outside.mkdir()
            (outside / "data.go").write_text("private")
            (root / "internal/link").symlink_to(outside, target_is_directory=True)
            with self.assertRaises(ValueError):
                source.inspect(root)


class RequiredIntegrationTests(unittest.TestCase):
    def run_gate(self, rows, required):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "tests.json"
            path.write_text("\n".join(json.dumps(r) for r in rows))
            return subprocess.run(
                [
                    sys.executable,
                    str(Path(__file__).with_name("check-tests.py")),
                    str(path),
                    "--required",
                    required,
                ],
                capture_output=True,
            ).returncode

    def test_database_skip_or_missing_anchor_is_not_success(self):
        rows = [
            {
                "Action": "pass",
                "Test": "TestPostgresIdentitySessionSurvivesRestart",
                "Package": "app/internal/identity",
            },
            {
                "Action": "pass",
                "Test": "TestPostgresRestartPreservesSamplesStateAndOutbox",
                "Package": "app/internal/store",
            },
        ]
        self.assertEqual(self.run_gate(rows, "database"), 0)
        self.assertNotEqual(self.run_gate(rows[:1], "database"), 0)
        rows.append(
            {
                "Action": "skip",
                "Test": "TestContract/postgres",
                "Package": "app/internal/store",
            }
        )
        self.assertNotEqual(self.run_gate(rows, "database"), 0)

    def test_mimir_requires_collection_destinations_and_alert_lifecycle(self):
        rows = [
            {"Action": "pass", "Test": n, "Package": "app/integration/mimir"}
            for n in [
                "TestRealMimirRetirement",
                "TestRealMimirTenantCollection",
                "TestRealMimirRuleDestinations",
                "TestRealMimirLifecycleAndMinEvents",
                "TestRealMimirIndependentAlertLifecycle",
            ]
        ]
        self.assertEqual(self.run_gate(rows, "mimir"), 0)
        for index in range(len(rows)):
            with self.subTest(missing=rows[index]["Test"]):
                self.assertNotEqual(
                    self.run_gate(rows[:index] + rows[index + 1 :], "mimir"), 0
                )
            with self.subTest(skipped=rows[index]["Test"]):
                skipped = [dict(row) for row in rows]
                skipped[index]["Action"] = "skip"
                self.assertNotEqual(self.run_gate(skipped, "mimir"), 0)

    def test_identity_requires_real_directory_and_native_cli_cases(self):
        rows = [
            {"Action": "pass", "Test": name, "Package": "app/integration/identity"}
            for name in [
                "TestRealIdentityOIDCBrowserCLIRevocation",
                "TestRealIdentityOIDCClaimsBoundaries",
                "TestRealIdentityOIDCClaimExpiry",
                "TestRealIdentityTeamsAndProviderBinding",
                "TestRealIdentityCLIApprovalCancellation",
                "TestRealIdentityOpenLDAPTLSAndFailureBoundaries",
            ]
        ]
        self.assertEqual(self.run_gate(rows, "identity"), 0)
        for index in range(len(rows)):
            with self.subTest(missing=rows[index]["Test"]):
                self.assertNotEqual(
                    self.run_gate(rows[:index] + rows[index + 1 :], "identity"), 0
                )
        rows.append(
            {
                "Action": "skip",
                "Test": "TestRealIdentityOpenLDAPTLSAndFailureBoundaries/starttls",
                "Package": "app/integration/identity",
            }
        )
        self.assertNotEqual(self.run_gate(rows, "identity"), 0)
        rows[-1] = {"Action": "fail", "Package": "app/integration/identity"}
        self.assertNotEqual(self.run_gate(rows, "identity"), 0)

    def test_operator_requires_real_selector_case(self):
        rows = [
            {"Action": "pass", "Test": name, "Package": "app/integration/operator"}
            for name in [
                "TestRealOperatorRuleSelectors",
                "TestRealOperatorControllerConformance",
                "TestRealOperatorLeaseFailover",
                "TestRealOperatorPortalPublication",
                "TestRealOperatorCleanupOutageAndCredentialRotation",
                "TestRealOperatorIndependentSeverityEvaluation",
            ]
        ]
        self.assertEqual(self.run_gate(rows, "operator"), 0)
        self.assertNotEqual(self.run_gate([], "operator"), 0)
        rows[0]["Action"] = "skip"
        self.assertNotEqual(self.run_gate(rows, "operator"), 0)


if __name__ == "__main__":
    unittest.main()
