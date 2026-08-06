---
layout: default
title: CI 与发布门禁对照
description: edgeCore 四道发布门禁（G-Stability / G-Industrial / G-Performance / G-Lightweight）与当前 CI 自动化覆盖对照
version: v1.0
date: 2026-07-03
status: 现行
---

# CI 与发布门禁对照

> **用途：** 对照 [版本发布门禁](../RELEASE_GATE.html) 四道门禁与仓库 **实际 CI / 本地脚本** 的覆盖情况，明确「文档要求」与「自动化现状」之间的差距。  
> **审计基准：** `.github/workflows/ci.yml`、`.github/workflows/release.yml`、`Makefile`、`scripts/bench_armv7.sh`、`.goreleaser.yml`（2026-07-03）。

---

## 总览

| 维度 | 现状 |
| --- | --- |
| **PR / 主干 CI** | **有** — [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)：`test-short`、`test-soak-short`、构建 smoke、`bench-q3`（PR + `main` push） |
| **Release CI** | 打 `v*` tag 时运行 `go test ./...` + `CGO_ENABLED=0` 双平台构建 |
| **Makefile 门控** | `test-soak-short`、`bench-q3` 等已就绪，**未接入 GitHub Actions** |
| **工业验证** | 完全依赖人工联机 + [工业验证测试报告模板](工业验证测试报告模板.html) 归档 |

**结论：** P0 PR 门控已接入（见下）；**G-Industrial 全部门禁项** 与 **24–72h 长跑 soak** 仍未自动化。Release 打 tag 仍运行完整 `go test ./...`（含 Q3 / short soak 的间接覆盖）。

---

## 门禁逐项对照

### G-Stability — 稳定性门禁

| 文档要求 | 验收标准（摘要） | 当前 CI / 脚本 | 自动化状态 |
| --- | --- | --- | --- |
| 连续运行 | 24–72h 无 panic / 死锁 / goroutine 泄漏 | `make test-soak`（`-tags=soak`，默认 1h，可 `SOAK_DURATION=72h`） | **未自动化** — 仅本地 / 计划 nightly |
| 短 soak 门控 | 故障注入下 lag / 失败率 / 内存 drift | `make test-soak-short` → `TestSoak_ScanEngineShortGate`（~30s + warmup） | **部分** — PR CI `make test-soak-short`；Release `go test ./...` 亦会执行 |
| 内存 | heap drift 在阈值内 | 同上 short soak + 长跑 soak 内嵌 gate | short：**PR CI + Release**；长跑：**未自动化** |
| 故障恢复 E2E | CB、故障传播、混合注入 | `internal/integration/fault_propagation_test.go`、`modbus_protocol_test.go`（CB 恢复在非 `-short` 时运行） | **部分** — PR CI `make test-short`（`-short`）+ Release 全量 mock E2E |
| 设备/通道隔离 | 单设备故障不扩散 | `TestModbusProtocol_SerialScanEngineFaultPropagation` 等 | **部分** — 同上 |
| 可观测 | diagnostics 反映 CB / lag / warnings | 无专用 CI 断言；依赖单元 / 集成测试间接覆盖 | **未自动化**（无 diagnostics 端点走查 job） |

### G-Industrial — 工业验证门禁

| 文档要求 | 验收标准（摘要） | 当前 CI / 脚本 | 自动化状态 |
| --- | --- | --- | --- |
| 协议覆盖 | 各已支持协议联机验证 | `internal/integration/*_protocol_test.go`（多为框架 / mock；OPC UA / S7 需现场设备时 `t.Skip`） | **未自动化** |
| 异常场景 S1–S6 | 断网、重启、抖动、超时、高丢包 | 无联机 / 仿真器 CI job | **未自动化** |
| 长时联机 | 每协议 24h / 72h | 无 | **未自动化** |
| 三件套 | 设计 + 代码 + [测试报告](工业验证测试报告模板.html) | 报告为人工归档；CI 不校验报告存在或 PASS/WARN | **未自动化** |

### G-Performance — 性能门禁

| 文档要求 | 验收标准（摘要） | 当前 CI / 脚本 | 自动化状态 |
| --- | --- | --- | --- |
| Q3 10k Tag Benchmark | P95 / drift / GC / heap / miss deadline | `make bench-q3` → `TestQ3_TenThousandTagBenchmark`（默认 60s + 10s warmup） | **部分** — PR CI `make bench-q3`；Release 亦会跑 ~70s；**无版本间回归对比** |
| Throttle 压测 | P95 增幅 <30% | `TestScanEngine_ThrottlePressure_ClusterLag`（非 `-short` 时） | **部分** — Release 间接 |
| 零分配 LoadPoints | benchmark 0 allocs/op | `make bench-loadpoints` | **未自动化** |
| ARMv7 板端 / 交叉编译 | 板端或 qemu 验证 | `scripts/bench_armv7.sh`、`make bench-armv7` | **未自动化** |
| 合并策略 | Benchmark 未通过不得合并性能 PR | PR CI `bench-q3` job | **P0 已落实**（无基线对比） |

### G-Lightweight — 轻量化门禁

| 文档要求 | 验收标准（摘要） | 当前 CI / 脚本 | 自动化状态 |
| --- | --- | --- | --- |
| 单二进制 | `CGO_ENABLED=0` 静态编译 | Release Workflow：`CGO_ENABLED=0` 构建 linux/windows amd64 | **部分** — PR CI 构建 smoke + Release amd64 双平台；未覆盖 arm / arm64 矩阵 |
| GoReleaser 产物 | 多架构 tar.gz / deb | `.goreleaser.yml` 配置完整 | **未接入 CI** — Release Workflow 未调用 goreleaser |
| 零外部依赖 | 无 Redis / Prometheus 等运行时依赖 | 无 dependency audit job | **未自动化** |
| 诊断通路 | HTTP + UI + 日志 | 无 smoke / 走查 job | **未自动化** |
| 镜像 / 体积 | 无重量级栈 | 无镜像构建或体积阈值 job | **未自动化** |

---

## 当前 CI 资产清单

| 资产 | 触发条件 | 实际执行内容 |
| --- | --- | --- |
| [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) | `pull_request` / `push` → `main`（及 `master`） | `make test-short` → `make test-soak-short` → `CGO_ENABLED=0` 构建 smoke；并行 job `make bench-q3` |
| [`.github/workflows/release.yml`](../../.github/workflows/release.yml) | 推送 `v*` tag | `go mod download` → `go test ./... -v -count=1`（`CGO_ENABLED=0`）→ Linux/Windows amd64 构建 → GitHub Release |
| [`Makefile`](../../Makefile) | 本地 / 待接入 CI | `test`、`test-short`、`test-soak-short`、`test-soak`、`bench-q3`、`bench-loadpoints`、`bench-armv7` |
| [`scripts/bench_armv7.sh`](../../scripts/bench_armv7.sh) | 本地 / 板端 | ARMv7 交叉编译 + 可选 qemu 跑 Q3 gate |
| [`.goreleaser.yml`](../../.goreleaser.yml) | 手动 `goreleaser release` | 多 OS/ARCH、`CGO_ENABLED=0`、deb 包 — **未在 Actions 中调用** |

**说明：** Release Workflow 使用 `go test ./...` 且 **不带 `-short`**，因此会顺带执行 Q3 benchmark（~70s）与 short soak gate（~55s），总时长显著长于 `make test-short`。但这 **仅发生在打 Release tag 时**，不能替代 PR 合并前的持续门控。

---

## 差距摘要

| 门禁 | 文档阻塞级别 | CI 覆盖度 | 主要缺口 |
| --- | --- | --- | --- |
| **G-Stability** | 阻塞发布 | 中 | PR CI 覆盖 short soak / test-short；24–72h soak 未 nightly；diagnostics 无自动化走查 |
| **G-Industrial** | 阻塞发布 | 无 | 联机 S1–S6、协议长跑、报告归档校验均未自动化 |
| **G-Performance** | 阻塞合并/发布 | 中 | PR CI `bench-q3`；无基线对比；ARMv7 / loadpoints 未接入 |
| **G-Lightweight** | 阻塞发布 | 低–中 | Release 仅 amd64 双平台；goreleaser / 依赖 audit / 诊断 smoke 缺失 |

---

## 建议的 CI 演进（P1 起，本文档不实施）

按优先级与实施成本排序，供后续 PR 参考：

### P0 — 主干 / PR 基础门控 — **已实施（2026-07-03）**

[`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)：`pull_request` 与 `main`/`master` 的 `push` 触发。

1. **Job `gates`**：`make test-short` → `make test-soak-short` → `CGO_ENABLED=0 go build -o /dev/null ./cmd/main.go`
2. **Job `bench-q3`**：并行运行 `make bench-q3`（Makefile 内 `-timeout=15m`），失败则 block merge。

### P1 — Release 与长跑

3. **Release Workflow 增强**：打 tag 前除 `go test ./...` 外，显式调用 `make test-soak-short` + `make bench-q3`，避免隐式依赖 `-short` 默认值变化。
4. **Nightly workflow**（`schedule: cron`）：`make test-soak`（1h）或 `-tags=soak` + `SOAK_DURATION=24h`（self-hosted / 长 timeout runner）。
5. **Release 构建**：改用 `goreleaser release` 或复用 `.goreleaser.yml` 矩阵，覆盖 linux/windows × amd64/arm64/arm。

### P2 — 工业验证与发布裁量

6. **Release 前 manual / workflow_dispatch job**： checklist 引用 [RELEASE_GATE 发布检查清单](../RELEASE_GATE.html#发布检查清单模板)，要求上传各协议 [工业验证报告](工业验证测试报告模板.html)（artifact 或 docs 路径校验）。
7. **可选联机 job**（self-hosted + 仿真器）：Modbus TCP 多从站、OPC UA server — 跑 S2–S5 子集，**不强制每 PR**。
8. **依赖 audit**：`go mod verify` + 禁止已知 heavyweight 依赖的脚本 gate（Redis client 等）。

### P3 — 回归与板端

9. **Benchmark 基线对比**：存储上一 tag 的 Q3 结果 JSON，PR 超阈值则 fail（可用 `benchstat` 或自定义阈值）。
10. **ARMv7**：CI 交叉编译 + 可选 qemu job；板端长跑保留 manual + 报告归档。

---

## 相关文档

- [版本发布门禁](../RELEASE_GATE.html) — 四道门禁定义与发布判定矩阵
- [开发原则与验收标准](../DEVELOPMENT_PRINCIPLES.html) — 上位工程原则
- [SLA 运维手册](../deployment/sla_monitoring.html) — Makefile 命令与 diagnostics 巡检
- [Q3 10k Benchmark 报告](Q3_10k_tag_benchmark_2026Q3.html) — 性能基准细节
- [工业验证测试报告模板](工业验证测试报告模板.html) — G-Industrial 归档模板
