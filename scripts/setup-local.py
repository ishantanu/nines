#!/usr/bin/env python3
"""Create private local Compose credentials; never overwrite an existing file."""

import argparse
import base64
import os
import secrets
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", type=Path, default=Path(".env"))
    parser.add_argument(
        "--playground",
        action="store_true",
        help="Show playground login and startup instructions",
    )
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    values = {
        "API_TOKEN": secrets.token_hex(32),
        "POSTGRES_PASSWORD": secrets.token_hex(24),
        "NINES_BOOTSTRAP_PASSWORD": secrets.token_urlsafe(24),
        "NINES_SOURCE_ENCRYPTION_KEY": base64.b64encode(
            secrets.token_bytes(32)
        ).decode(),
    }
    lines = (root / ".env.example").read_text().splitlines()
    body = (
        "\n".join(
            line.split("=", 1)[0] + "=" + values[line.split("=", 1)[0]]
            if not line.startswith("#") and line.split("=", 1)[0] in values
            else line
            for line in lines
        )
        + "\n"
    )
    try:
        fd = os.open(args.out, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    except FileExistsError:
        parser.error(f"{args.out} already exists; refusing to replace credentials")
    with os.fdopen(fd, "w") as output:
        output.write(body)
    print(f"Created {args.out} with private permissions. Keep it out of Git.")
    print(f"Database password: POSTGRES_PASSWORD in {args.out}; not the web password.")
    print("Web username: admin")
    if args.playground:
        print("Web URL: http://127.0.0.1:18080")
        print("Web password: admin (or your NINES_PLAYGROUND_PASSWORD override)")
        print("Start with: ./playground up")
    else:
        print("Web URL: http://127.0.0.1:8080")
        print(f"Web password: NINES_BOOTSTRAP_PASSWORD in {args.out}")
        print("Start with: docker compose -p nines up --build -d")


if __name__ == "__main__":
    main()
