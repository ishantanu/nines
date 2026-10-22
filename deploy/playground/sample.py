"""Synthetic checkout counters and a loopback-published failure control."""

import json
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlsplit

lock = threading.Lock()
good, bad, failure = 0, 0, 0


def tick():
    global good, bad
    while True:
        with lock:
            good += 100 - failure
            bad += failure
        time.sleep(1)


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/metrics":
            self.send_error(404)
            return
        with lock:
            text = (
                "# HELP playground_requests_total Synthetic checkout requests.\n"
                "# TYPE playground_requests_total counter\n"
                f'playground_requests_total{{code="200"}} {good}\n'
                f'playground_requests_total{{code="500"}} {bad}\n'
            )
        self.send_response(200)
        self.send_header("Content-Type", "text/plain; version=0.0.4")
        self.end_headers()
        self.wfile.write(text.encode())

    def do_POST(self):
        global failure
        url = urlsplit(self.path)
        try:
            value = int(parse_qs(url.query)["percent"][0])
            assert url.path == "/failure" and 0 <= value <= 100
        except (KeyError, ValueError, AssertionError):
            self.send_error(400, "Use /failure?percent=0..100")
            return
        with lock:
            failure = value
        self.send_response(200)
        self.end_headers()
        self.wfile.write(json.dumps({"failurePercent": value}).encode())

    def log_message(self, *_):
        pass


threading.Thread(target=tick, daemon=True).start()
ThreadingHTTPServer(("0.0.0.0", 8000), Handler).serve_forever()
