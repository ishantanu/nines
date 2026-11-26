#!/usr/bin/env python3
"""Copies the Helm chart for a release, pinning the server and operator image
digests the release published, so the chart installs exactly those images."""

import argparse
import re
import shutil
from pathlib import Path

DIGEST = re.compile(r"^sha256:[a-f0-9]{64}$")


def pin(values, server, operator):
    # The server image is the top-level image (two-space digest); the operator
    # image is nested under operator (four spaces).
    replacements = [
        (r"(?m)^(  digest: )''$", server),
        (r"(?m)^(    digest: )''$", operator),
    ]
    for pattern, digest in replacements:
        values, count = re.subn(pattern, lambda m, d=digest: m.group(1) + d, values)
        if count != 1:
            raise SystemExit(f"expected one unpinned digest matching {pattern!r}")
    return values


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--chart", type=Path, default=Path("charts/nines"))
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--server-digest", required=True)
    parser.add_argument("--operator-digest", required=True)
    args = parser.parse_args()
    for digest in (args.server_digest, args.operator_digest):
        if not DIGEST.match(digest):
            parser.error(f"invalid digest {digest!r}")
    shutil.copytree(args.chart, args.out)
    values = args.out / "values.yaml"
    values.write_text(pin(values.read_text(), args.server_digest, args.operator_digest))


if __name__ == "__main__":
    main()
