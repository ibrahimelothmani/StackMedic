"""Minimal DNS diagnostic demo; intentionally not a real API server."""

import os
import socket
import sys

host = os.environ["DATABASE_HOST"]
port = int(os.environ.get("DATABASE_PORT", "5432"))
print(f"[api] Checking database hostname: {host}:{port}", flush=True)

try:
    socket.getaddrinfo(host, port, type=socket.SOCK_STREAM)
except OSError as exc:
    print(f"[api] ERROR: unable to resolve database hostname {host!r}: {exc}", file=sys.stderr, flush=True)
    raise SystemExit(1)

print(f"[api] Database hostname {host!r} resolves successfully", flush=True)
