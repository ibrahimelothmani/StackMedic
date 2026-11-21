"""Minimal crash-loop demo; intentionally not a real worker."""

import os
import sys
import time

queue = os.environ.get("QUEUE_NAME", "")
print("[worker] starting", flush=True)
if not queue:
    print("[worker] FATAL: required setting QUEUE_NAME is empty", file=sys.stderr, flush=True)
    raise SystemExit(3)
print(f"[worker] consuming from {queue!r}", flush=True)
time.sleep(3600)
