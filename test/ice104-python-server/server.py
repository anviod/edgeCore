#!/usr/bin/env python3
"""Headless IEC 60870-5-104 server for edgeCore ice104 driver M1 integration tests.

Uses Fraunhofer c104 (iec104-python / lib60870-C bindings, GPLv3).
Exposes M_ME_NA_1 IOA=1 CA=1 with normalized value 0.5 on TCP 2404.

Usage:
    pip install -r requirements.txt
    python server.py --bind 127.0.0.1 --port 2404
"""

from __future__ import annotations

import argparse
import signal
import sys
import time

try:
    import c104
except ImportError:
    print(
        "c104 not installed; run: pip install -r requirements.txt",
        file=sys.stderr,
    )
    sys.exit(1)

COMMON_ADDRESS = 1
IOA = 1
NORMALIZED_VALUE = 0.5


def main() -> int:
    parser = argparse.ArgumentParser(description="edgeCore ice104 M1 test server (c104)")
    parser.add_argument("--bind", default="127.0.0.1", help="listen address")
    parser.add_argument("--port", type=int, default=2404, help="TCP port")
    args = parser.parse_args()

    server = c104.Server(ip=args.bind, port=args.port, tick_rate_ms=100)
    station = server.add_station(common_address=COMMON_ADDRESS)
    point = station.add_point(
        io_address=IOA,
        type=c104.Type.M_ME_NA_1,
        report_ms=0,
    )
    point.value = c104.NormalizedFloat(NORMALIZED_VALUE)

    server.start()

    print(
        f"ice104-python-server listening on {args.bind}:{args.port} "
        f"(M_ME_NA_1 IOA={IOA} CA={COMMON_ADDRESS}, value={NORMALIZED_VALUE})",
        flush=True,
    )

    stop = False

    def handle_stop(*_args: object) -> None:
        nonlocal stop
        stop = True

    signal.signal(signal.SIGINT, handle_stop)
    if hasattr(signal, "SIGTERM"):
        signal.signal(signal.SIGTERM, handle_stop)

    try:
        while not stop:
            time.sleep(0.2)
    finally:
        server.stop()

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
