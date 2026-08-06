---
layout: section-index
title: 测试验证
description: edgeCore 测试验证文档 — 单元测试、回归验证、压力测试与验收报告
hero_eyebrow: Testing & Verification
hero_lead: edgeCore 测试验证文档 — 测试矩阵、回归验证、压力测试、南向驱动测试报告、2026-07-12 Mac 万 Tag/ScanEngine 复测与热路径单测补强、2026-07-10 ARM64 vs x86 跨架构双向对比测试。
hero_buttons:
  - text: 跨架构对比报告
    url: arch-cross-platform-benchmark-2026Q3/arch-cross-platform-benchmark-2026Q3.html
    style: primary
  - text: 南向驱动测试报告
    url: 南向驱动测试报告.html
    style: secondary
  - text: Test Report (EN)
    url: southbound-driver-test-report.html
    style: secondary
  - text: 返回首页
    url: ../index.html
    style: secondary
---

## 目录

### 发布与验收
- [版本发布门禁](../RELEASE_GATE.html) — 稳定性 / 工业验证 / 性能 / 轻量化四道门禁
- [CI 与发布门禁对照](CI与发布门禁对照.html) — 文档门禁 vs 当前 GitHub Actions / Makefile 覆盖差距
- [开发原则与验收标准](../DEVELOPMENT_PRINCIPLES.html) — 量化验收标准

### 测试计划
- [测试矩阵](test_matrix.html)
- [验收测试](acceptance_test.html)

### 点位测试
- [API 点位测试报告](API_Points_Test_Report.html)

### 回归验证
- [南向采集通道回归验证测试方案](南向采集通道回归验证测试方案.html) — 联机/压测步骤；§一/§八 含 2026-07-04 回归状态与排期；D 组 10ms 见 [B1 核验](b1_poll_interval_verification.html)

### 压力测试
- [压力测试报告](压力测试报告.html) — 含 **2026-07-12 Mac** 万 Tag / G007 / Soak / EdgeCompute 复测
- [Q3 万 Tag 压测报告](Q3_10k_tag_benchmark_2026Q3.html) — 10k tag @1s；§5 为 2026-07-12 Mac 复测

### 验证报告
- [验证报告](VERIFICATION_REPORT.html)
- [工业验证测试报告模板](工业验证测试报告模板.html) — Phase 2 各协议联机验证统一模板（G-Industrial）

### 南向驱动测试

> **2026-07-12 热路径补强**：`CGO_ENABLED=0 go test ./internal/{core,ai_agent,driver/modbus,driver/ethernetip}/ -short` PASS。覆盖率：`internal/core` **80.0%**、`internal/ai_agent` **91.4%**、`modbus` **65.9%**、`ethernetip` **62.2%**（均 ≥60%）。`TestScenario_RecoveryFromDead` 在 `-short` 下跳过 2 分钟冷却等待。

> **2026-07-11 / 07-12 全量回归**：`CGO_ENABLED=0 go test ./internal/driver/... -short` — 主驱动包 **22/22 PASS**（含 EtherCAT **87.8%**）；ConnectionManager **87.4%**，DL/T645 **76.5%**、KNXnet/IP **77.2%**、Mitsubishi **70.7%** 达 ≥70%。

- [南向驱动测试报告](南向驱动测试报告.html) — 单元测试、性能基准、边界场景矩阵（2026-07-12）
- [Southbound Driver Test Report (EN)](southbound-driver-test-report.html)

### 热路径单元测试（2026-07-12）

| 模块 | 新增/增强测试文件 | 覆盖重点 | `-short` 覆盖率 |
| :--- | :--- | :--- | ---: |
| `internal/ai_agent` | `manager_test.go` | Create/Confirm/配额/EdgeRule/Diagnostics | **91.4%** |
| `internal/core` | `channel_manager_hotpath_test.go` | EtherCAT 地址校验、ScanEngine 指标快照、RemoveDevice | **80.0%** |
| `internal/driver/modbus` | `scheduler_hotpath_test.go` | `Read`/`Write`/`readGroup`/`groupPoints`/`markPointFailed` | **65.9%** |
| `internal/driver/ethernetip` | `process_tag_test.go` | `processTagValue`、Tag 解析、batch 分组 | **62.2%** |

### 代码测试维护

- [test/ 目录说明](https://github.com/anviod/edgeCore/blob/dev/test/README.md) — 联调资产、`go test` 常用命令与测试腐化处理原则
- [B1 采集间隔核验报告](b1_poll_interval_verification.html) — Modbus 多从站 PollStart Δt 现场样本

### ScanEngine / ShadowCore SLA 验证

- [Q3 Phase A–D 验收复测 2026-07-04](q3_phase_abcd_verification_2026-07-04.html) — 完整命令、实测指标与 gate 结论
- [SLA 完成报告 2026Q3](sla_completion_report_2026Q3.html) — Phase A–D 测试与指标（含 2026-07-04 复测）
- [确定性 SLA 报告](deterministic_sla_report.html) — EDF + hard jitter；P99 承诺
- [Shadow 性能优化报告 2026Q3](shadow_optimization_report_2026Q3.html) — COW / Worker Pool / Ingress
- [ARM64 vs x86 跨架构双向对比测试 2026Q3](arch-cross-platform-benchmark-2026Q3/arch-cross-platform-benchmark-2026Q3.html) — 真实硬件双向对比：Cortex-A55 (RK3588s) vs i5-13500H (Golden Cove)
- [ScanEngine SLA 评估](../TODO/SLA评估.html) — 工业边界 B1–B6 达标矩阵
- [SLA 轻量化运维手册](../deployment/sla_monitoring.html) — diagnostics 巡检

### ScanEngine / ShadowCore 测试（权威版 edge/）

> testing/ 目录下编号测试文档为归档副本，正文以 edge/ 为准。

- [影子设备与采集优化集成测试](../edge/影子设备与采集优化集成测试文档.html)
- [2. 智能画像方案设计_测试](../edge/2. 智能画像方案设计_测试文档.html)
- [3. 核心结构体定义_测试](../edge/3. 核心结构体定义_测试文档.html)
- [4. 核心设计_测试](../edge/4. 核心设计_测试文档.html)
- [6. 影子设备设计_测试](../edge/6. 影子设备设计_测试文档.html)
- [8. RTT 管理器_测试](../edge/8. RTT管理器实现_测试文档.html)
- [9. MTU 管理器_测试](../edge/9. MTU管理器实现_测试文档.html)
- [10. Gap 优化器_测试](../edge/10. Gap优化器实现_测试文档.html)
