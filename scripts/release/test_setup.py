import os
import re
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


class LocalSetupTests(unittest.TestCase):
    def test_private_credentials_and_mode_specific_login_instructions(self):
        for playground in (False, True):
            with (
                self.subTest(playground=playground),
                tempfile.TemporaryDirectory() as folder,
            ):
                path = Path(folder) / ".env"
                command = [
                    sys.executable,
                    str(ROOT / "scripts/setup-local.py"),
                    "--out",
                    str(path),
                ]
                if playground:
                    command.append("--playground")
                result = subprocess.run(
                    command, capture_output=True, text=True, check=True
                )
                original = path.read_bytes()
                values = dict(
                    line.split("=", 1)
                    for line in original.decode().splitlines()
                    if line and not line.startswith("#") and "=" in line
                )
                self.assertRegex(
                    values["POSTGRES_PASSWORD"], re.compile(r"^[0-9a-f]{48}$")
                )
                self.assertEqual(os.stat(path).st_mode & 0o777, 0o600)
                self.assertIn("Web username: admin", result.stdout)
                self.assertIn("POSTGRES_PASSWORD", result.stdout)
                for name in (
                    "POSTGRES_PASSWORD",
                    "API_TOKEN",
                    "NINES_BOOTSTRAP_PASSWORD",
                ):
                    self.assertNotIn(values[name], result.stdout)
                if playground:
                    self.assertIn("Web password: admin ", result.stdout)
                    self.assertIn("compose.playground.yaml", result.stdout)
                    self.assertNotIn("NINES_BOOTSTRAP_PASSWORD", result.stdout)
                else:
                    self.assertIn("NINES_BOOTSTRAP_PASSWORD", result.stdout)
                    self.assertNotIn("Web password: admin ", result.stdout)
                refused = subprocess.run(command, capture_output=True, text=True)
                self.assertNotEqual(refused.returncode, 0)
                self.assertEqual(path.read_bytes(), original)
