# EdgeX / edgeCore 构建与质量门禁
#
# 发布版本（含 UI 构建）使用 goreleaser：
#   goreleaser release --snapshot --clean
# 详见 .goreleaser.yml

GO              ?= go
CGO_ENABLED     ?= 0
PKGS            ?= ./...
SOAK_DURATION   ?= 30s

# 默认目标：静态检查 + 短测试 + 构建冒烟
.PHONY: all
all: vet test-short build-smoke

# 静态检查（仅编译期分析，不运行测试）
.PHONY: vet
vet:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) vet $(PKGS)

# 短单元测试：跳过 soak 与 10k benchmark 等长时测试
# 对应 CI gates 作业的 "Run short unit tests"
.PHONY: test-short
test-short:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) test -short -count=1 $(PKGS)

# 短 soak 门禁（CI 友好，无需 //go:build soak）
# 对应 CI gates 作业的 "Run short soak gate"
# 文档：SOAK_DURATION=30s go test ./internal/integration/ -run TestSoak -count=1 -timeout=5m
.PHONY: test-soak-short
test-soak-short:
	CGO_ENABLED=$(CGO_ENABLED) SOAK_DURATION=$(SOAK_DURATION) $(GO) test ./internal/integration/ -run TestSoak -count=1 -timeout=5m

# 长 soak（手动 / nightly，需要 //go:build soak）
# 文档：SOAK_DURATION=72h go test -tags=soak ./internal/integration/ -run TestSoak_ScanEngineStability -count=1 -timeout=80h
.PHONY: test-soak
test-soak:
	CGO_ENABLED=$(CGO_ENABLED) SOAK_DURATION=72h $(GO) test -tags=soak ./internal/integration/ -run TestSoak_ScanEngineStability -count=1 -timeout=80h

# Q3 万 Tag 吞吐 / 延迟门禁
# 对应 CI bench-q3 作业的 "Run Q3 benchmark gate"
# 默认 60s 运行时（可用 Q3_BENCH_DURATION 覆盖，单位秒）
.PHONY: bench-q3
bench-q3:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) test ./internal/core/ -run '^TestQ3_TenThousandTagBenchmark$$' -count=1 -timeout=10m

# 完整构建产物（用于部署 / 发布验证）
.PHONY: build
build:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build -trimpath -o dist/edgeCore ./cmd/

# 构建冒烟：仅确认可编译（CI gates 作业的 "Build smoke"）
# 对应 CI：CGO_ENABLED=0 go build -o /dev/null ./cmd/main.go
.PHONY: build-smoke
build-smoke:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build -o /dev/null ./cmd/main.go

.PHONY: fmt
fmt:
	$(GO) fmt $(PKGS)
