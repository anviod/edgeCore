.PHONY: test test-short test-soak test-soak-short bench-q3 bench-g007 bench-loadpoints bench-armv7

test:
	go test ./internal/core/... ./internal/integration/... -count=1

test-short:
	go test ./internal/core/... ./internal/integration/... -short -count=1

# Nightly ≥1h soak (requires //go:build soak)
test-soak:
	SOAK_DURATION=3600 go test -tags=soak ./internal/integration/... -run TestSoak_ScanEngineStability -count=1 -timeout=2h

# PR/CI short soak gate (~60s default, override with SOAK_DURATION=30s)
test-soak-short:
	SOAK_DURATION=30 go test ./internal/integration/... -run TestSoak -count=1 -timeout=5m

bench-q3:
	go test ./internal/core/ -run TestQ3_TenThousandTagBenchmark -count=1 -timeout=15m

bench-g007:
	go test ./internal/core/ -run TestG007_DeviceThroughputBenchmark -count=1 -timeout=5m

bench-loadpoints:
	go test ./internal/core/ -run '^$$' -bench BenchmarkExecutionLayer_LoadPoints_Pooled -benchmem -count=3

# ARMv7 cross-compile gate (compile-only; run binary on board or qemu-arm)
bench-armv7:
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go test -tags=bench -c ./internal/core/ -o $(TMPDIR)/edgeCore-armv7.test
	@echo "Cross-compiled: $(TMPDIR)/edgeCore-armv7.test"
	@echo "Run: Q3_BENCH_DURATION=60 $(TMPDIR)/edgeCore-armv7.test -test.run TestARMv7_Q3BenchmarkGate -test.v -test.timeout=15m"
