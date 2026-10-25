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
            (root / ".vscode").mkdir()
            (root / ".vscode/mcp.json").write_text("private")
            (root / "docs").mkdir()
            (root / "docs/old-proof.json").write_text("private")
            selected, errors = source.inspect(root)
            self.assertFalse(errors)
            self.assertNotIn(Path(".env"), selected)
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

    def test_operator_requires_real_selector_case(self):
        rows = [
            {"Action": "pass", "Test": name, "Package": "app/integration/operator"}
            for name in [
                "TestRealOperatorRuleSelectors",
                "TestRealOperatorControllerConformance",
                "TestRealOperatorLeaseFailover",
                "TestRealOperatorPortalPublication",
            ]
        ]
        self.assertEqual(self.run_gate(rows, "operator"), 0)
        self.assertNotEqual(self.run_gate([], "operator"), 0)
        rows[0]["Action"] = "skip"
        self.assertNotEqual(self.run_gate(rows, "operator"), 0)


if __name__ == "__main__":
    unittest.main()
