"""Reload only this playground's Prometheus when shared rule files change."""

import hashlib
import time
import urllib.request
from pathlib import Path

previous = None
while True:
    try:
        digest = hashlib.sha256()
        for file in sorted(Path("/rules").glob("*.yaml")):
            digest.update(file.name.encode())
            digest.update(file.read_bytes())
        current = digest.digest()
        if current != previous:
            req = urllib.request.Request("http://prometheus:9090/-/reload", data=b"")
            with urllib.request.urlopen(req, timeout=5) as response:
                assert response.status == 200
            previous = current
            print("Prometheus accepted the current rule files", flush=True)
    except (OSError, AssertionError) as error:
        print(f"Reload pending: {type(error).__name__}", flush=True)
    time.sleep(2)
