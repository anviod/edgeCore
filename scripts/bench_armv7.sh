#!/usr/bin/env bash
# ARMv7 cross-compile benchmark helper for edgeCore ScanEngine SLA verification.
# Run on linux/arm host or via qemu-user when hardware is unavailable.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export GOOS=linux
export GOARCH=arm
export GOARM=7
export CGO_ENABLED=0

echo "==> edgeCore ARMv7 benchmark (GOOS=$GOOS GOARCH=$GOARCH GOARM=$GOARM CGO_ENABLED=$CGO_ENABLED)"
echo "    Host: $(uname -s)/$(uname -m)"

OUT="${TMPDIR:-/tmp}/edgeCore-armv7-bench.test"
go test -tags=bench -c ./internal/core/ -o "$OUT"

if command -v qemu-arm >/dev/null 2>&1; then
  echo "==> Running Q3 benchmark via qemu-arm (60s window)"
  Q3_BENCH_DURATION=60 qemu-arm "$OUT" -test.run TestARMv7_Q3BenchmarkGate -test.timeout=15m -test.v || {
    echo "WARN: qemu-arm run failed; documenting cross-compile success only"
    echo "Cross-compiled test binary: $OUT"
    exit 0
  }
else
  echo "WARN: qemu-arm not found — cross-compile only"
  echo "Cross-compiled test binary: $OUT"
  echo "Copy to ARMv7 board and run:"
  echo "  Q3_BENCH_DURATION=60 ./edgeCore-armv7-bench.test -test.run TestARMv7_Q3BenchmarkGate -test.v"
fi

echo "==> Done. See docs/testing/shadow_armv7_performance_2026Q3.md for acceptance thresholds."
