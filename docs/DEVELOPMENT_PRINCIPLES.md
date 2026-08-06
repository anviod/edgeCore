---
layout: default
title: 开发原则与验收标准
description: edgeCore 工业边缘网关战略开发原则 — 稳定性优先、工业级验证、高性能与轻量化，含量化验收标准
version: v1.0
date: 2026-07-03
status: 现行
---

# 开发原则与验收标准

> **工程铁律：** 任何性能优化不得以牺牲稳定性为代价；任何架构优化不得增加系统恢复复杂度。

本文档定义 edgeCore 工业边缘网关的**战略开发优先级**与**量化验收标准**，适用于架构设计、功能开发、性能优化与版本发布。所有设计文档、实现方案与 PR 评审均须对齐本文档。

> 相关文档：
> - [分阶段路线图](ROADMAP.html) — Phase 1–4 实施顺序与交付物
> - [版本发布门禁](RELEASE_GATE.html) — 每版本四道门禁
> - [ScanEngine SLA 评估](TODO/SLA评估.html) — 指标阈值与达标矩阵（技术细节）
> - [边缘网关架构设计总览](edge/边缘网关架构设计总览.html) — 运行时架构与能力评估

---

## 1. 核心优先级原则

开发决策按以下优先级排序（**高 → 低**）。低优先级目标不得阻塞或削弱高优先级目标的达成。

| 优先级 | 原则 | 量化验收标准 |
| --- | --- | --- |
| **1** | **稳定性优先** | 单设备故障不得影响其他设备；通道须保持可调度；所有异常须可恢复；所有状态须可观测；每项失败须有自动恢复路径 |
| **2** | **工业级标准** | 每个版本须通过统一 Benchmark、真实 PLC 联机测试、故障注入测试 — **仅有理论设计不算完成** |
| **3** | **高性能** | 合并主干前 Benchmark 须通过；Scan Lag、GC、Heap、Goroutine、吞吐量须有回归测试 |
| **4** | **轻量化** | 单二进制交付、零外部运行时依赖；全部诊断经 HTTP API、Web UI、结构化日志完成 |

### 1.1 稳定性优先 — 验收细则

| 维度 | 要求 | 验证方式 |
| --- | --- | --- |
| 设备隔离 | 单设备离线/超时/CB Open 不影响同通道其他设备采集 | E2E 故障传播测试（如 7 从站 1 离线 → 6/7 正常） |
| 通道隔离 | 单通道故障不拖死全局调度与其他通道 | 多通道并行 + 单通道注入故障 |
| 可调度性 | 故障设备/通道降级后，其余任务仍按 Scan Class 入队执行 | ScanEngine 集成测试 + diagnostics |
| 可恢复性 | 断连、超时、PLC 重启后自动重连并恢复采集 | 故障注入 + 冷却/退避验证 |
| 可观测性 | CB 状态、lag、warnings、通道事件可通过 API/UI 查询 | `GET /diagnostics/*` + 结构化日志 |
| 自动恢复 | 每项失败路径有 Retry + Cooldown + HalfOpen 或等价机制 | 单元测试 + E2E CB 循环 |

### 1.2 工业级标准 — 验收细则

| 维度 | 要求 | 验证方式 |
| --- | --- | --- |
| 统一 Benchmark | 全版本共用 Q3 10k Tag / soak / GC gate | `go test` benchmark + CI gate |
| 真实设备联机 | 各支持协议须对接真实 PLC/仿真器/表计 | 联机测试报告（见 Phase 2）；裁量见 [G-Industrial 发布判定标准](RELEASE_GATE.html#g-industrial--工业验证门禁) |
| 故障注入 | 断网、抖动、高丢包、超时、PLC 重启 | `fault/injector` + 混合故障 soak；S2–S6 必测 |
| 完成定义 | 设计文档 + 代码 + 测试报告三件套 | PR 门禁 + [工业验证测试报告模板](testing/工业验证测试报告模板.html) |

### 1.3 高性能 — 验收细则

| 指标 | 回归要求 | 参考阈值 |
| --- | --- | --- |
| Scan Lag P95 | 每次合并须过 gate | x86 mock <100ms；板端/联机 <150ms |
| Scan Drift 均值 | 同上 | <50ms（x86）；<80ms（板端） |
| Miss Deadline | 稳态 = 0 | `scan_miss_deadline_total` |
| Heap / GC | 60s 窗口无异常增长 | heap drift <5%；GC pause max <20ms |
| Goroutine | 长时运行无泄漏 | 24–72h soak 门禁 |
| 吞吐量 | 10k Tag 场景 benchmark 不退化 | `q3_10k_tag_benchmark_test.go` |

### 1.4 轻量化 — 验收细则

| 维度 | 要求 | 禁止项 |
| --- | --- | --- |
| 交付形态 | 单二进制 `edgeCore`，`CGO_ENABLED=0` | 运行时依赖 Redis / Prometheus / Grafana 等 |
| 诊断入口 | HTTP JSON diagnostics、Web UI 轮询、结构化日志 | 强制引入外部 APM/TSDB |
| 运维模型 | 边缘节点可离线自治巡检 | 依赖云端监控才能判 SLA |

---

## 2. 重要约束

### 2.1 调度优化顺序

**在稳定性封闭（Phase 1）完成之前，不得优化确定性调度。**

工业边缘网关定位以**统计 SLA** 为准即可：

- P95/P99 Scan Lag 可度量、可告警
- 慢设备隔离、长时稳定、自动恢复
- 不书面承诺 scan interval 硬实时上限（对标 Kepware 确定性 SLA 为长期 Phase D 可选目标）

### 2.2 工程铁律的应用

| 场景 | 正确做法 | 错误做法 |
| --- | --- | --- |
| 性能优化 | Object Pool / 零分配在通过稳定性 gate 后引入 | 为降 lag 取消 CB 或串行隔离 |
| 架构优化 | 统一 ConnectionManager，单一恢复路径 | 各驱动各自重连，恢复逻辑分散 |
| 可观测性 | 轻量 diagnostics + Event Log | 引入重量级监控栈才能上线 |
| 功能扩展 | Phase 2 先验证现有协议 | Phase 1 未完成时新增协议 |

---

## 3. 与现有文档的关系

| 文档 | 关系 |
| --- | --- |
| [ROADMAP.md](ROADMAP.html) | 本文档原则的具体分阶段实施计划 |
| [RELEASE_GATE.md](RELEASE_GATE.html) | 每版本发布前的四道门禁检查清单；**G-Industrial 发布判定标准（PASS/WARN/BLOCK）** |
| [testing/工业验证测试报告模板.md](testing/工业验证测试报告模板.html) | Phase 2 各协议联机报告模板，与 G-Industrial 裁量对齐 |
| [TODO/SLA评估.md](TODO/SLA评估.html) | ScanEngine 指标阈值、Phase A–D 任务拆分（技术执行层） |
| [边缘网关架构设计总览](edge/边缘网关架构设计总览.html) | 架构能力评估与 ScanEngine 内核说明 |
| [testing/test_matrix.md](testing/test_matrix.html) | 测试矩阵与回归范围 |

---

## 4. 文档维护

- **变更流程**：战略原则变更须更新本文档、`ROADMAP.md`、`RELEASE_GATE.md` 及相关设计文档开头的工程铁律引用。
- **评审检查项**：PR 须说明对齐的优先级原则、所属 Phase、以及满足的验收标准或门禁项。
