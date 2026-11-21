"""Minimal TCP connection demo; intentionally not a real database client."""

import os
import socket
import sys

host = os.environ["DATABASE_HOST"]
port = int(os.environ.get("DATABASE_PORT", "5432"))
print(f"[api] Connecting to database at {host}:{port}", flush=True)
try:
    socket.create_connection((host, port), timeout=3).close()
except OSError as exc:
    print(f"[api] ERROR: connection to {host}:{port} failed: {exc}", file=sys.stderr, flush=True)
    raise SystemExit(1)
print(f"[api] Connected to {host}:{port}", flush=True)
