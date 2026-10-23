#!/usr/bin/env python3
"""Small read-only Prometheus exporter for active SignalForge archive files."""
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from threading import Lock

ARCHIVE_DIR = Path(os.environ.get("ARCHIVE_METRICS_PATH", "/archive"))
PORT = int(os.environ.get("ARCHIVE_METRICS_PORT", "9465"))
STREAMS = {"raw": "logs.jsonl", "optimized": "optimized.jsonl"}


class ArchiveState:
    def __init__(self):
        self.lock = Lock()
        self.state = {name: {"inode": None, "offset": 0, "lines": 0} for name in STREAMS}

    def sample(self):
        rows = []
        with self.lock:
            for stream, filename in STREAMS.items():
                path = ARCHIVE_DIR / filename
                size = 0
                present = 0
                try:
                    stat = path.stat()
                    size = stat.st_size
                    present = 1
                    current = self.state[stream]
                    if current["inode"] != stat.st_ino or size < current["offset"]:
                        current.update(inode=stat.st_ino, offset=0, lines=0)
                    with path.open("rb") as handle:
                        handle.seek(current["offset"])
                        while True:
                            chunk = handle.read(1024 * 1024)
                            if not chunk:
                                break
                            current["lines"] += chunk.count(b"\n")
                        current["offset"] = handle.tell()
                    lines = current["lines"]
                except FileNotFoundError:
                    self.state[stream].update(inode=None, offset=0, lines=0)
                    lines = 0
                rows.extend([
                    f'signalforge_archive_file_bytes{{stream="{stream}"}} {size}',
                    f'signalforge_archive_file_present{{stream="{stream}"}} {present}',
                    f'signalforge_archive_records_total{{stream="{stream}"}} {lines}',
                ])
        return "\n".join(rows) + "\n"


STATE = ArchiveState()


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/healthz":
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"ok\n")
            return
        if self.path != "/metrics":
            self.send_error(404)
            return
        payload = STATE.sample().encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, fmt, *args):
        return


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", PORT), Handler).serve_forever()
