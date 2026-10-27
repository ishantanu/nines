import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

CHECKER = Path(__file__).with_name("check-tests.py")


class IntegrationGateTests(unittest.TestCase):
    def run_gate(self, rows):
        with tempfile.TemporaryDirectory() as directory:
            report = Path(directory) / "results.json"
            report.write_text("\n".join(json.dumps(row) for row in rows))
            return subprocess.run(
                [sys.executable, str(CHECKER), str(report), "--required", "topology"],
                capture_output=True,
                text=True,
                check=False,
            )

    def test_failure_keeps_explanation_and_rejects_report(self):
        result = self.run_gate(
            [
                {
                    "Package": "fixture",
                    "Test": "TestRealInstrumentedDependencyEvidence",
                    "Action": "output",
                    "Output": "fixture timeout: independent recovery\n",
                },
                {
                    "Package": "fixture",
                    "Test": "TestRealInstrumentedDependencyEvidence",
                    "Action": "fail",
                },
            ]
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn("fixture timeout: independent recovery", result.stderr)
        self.assertEqual(json.loads(result.stdout)["status"], "failed")

    def test_diagnostics_are_bounded_and_exclude_successful_tests(self):
        rows = [
            {
                "Package": "fixture",
                "Test": "PassingTest",
                "Action": "output",
                "Output": "passing output",
            }
        ]
        rows += [
            {
                "Package": "fixture",
                "Test": "FailingTest",
                "Action": "output",
                "Output": f"diagnostic {index}\n",
            }
            for index in range(60)
        ]
        rows.append({"Package": "fixture", "Test": "FailingTest", "Action": "fail"})
        result = self.run_gate(rows)
        self.assertEqual(result.returncode, 1)
        self.assertNotIn("passing output", result.stderr)
        self.assertNotIn("diagnostic 0\n", result.stderr)
        self.assertIn("diagnostic 59\n", result.stderr)

    def test_required_success_remains_quiet(self):
        result = self.run_gate(
            [
                {
                    "Package": "fixture",
                    "Test": "TestRealInstrumentedDependencyEvidence",
                    "Action": "pass",
                }
            ]
        )
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stderr, "")
        self.assertEqual(json.loads(result.stdout)["status"], "passed")
