import importlib.util
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location(
    "chart", Path(__file__).with_name("chart.py")
)
chart = importlib.util.module_from_spec(spec)
spec.loader.exec_module(chart)

ROOT = Path(__file__).resolve().parents[2]
SERVER = "sha256:" + "a" * 64
OPERATOR = "sha256:" + "b" * 64


class ChartDigestTests(unittest.TestCase):
    def test_pins_the_server_and_operator_images(self):
        values = (ROOT / "charts/nines/values.yaml").read_text()
        pinned = chart.pin(values, SERVER, OPERATOR)
        self.assertIn(f"\n  digest: {SERVER}\n", pinned)
        self.assertIn(f"\n    digest: {OPERATOR}\n", pinned)
        self.assertNotIn("digest: ''", pinned)

    def test_refuses_values_it_cannot_pin_exactly(self):
        with self.assertRaises(SystemExit):
            chart.pin("image:\n  tag: ''\n", SERVER, OPERATOR)


if __name__ == "__main__":
    unittest.main()
