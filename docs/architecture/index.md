---
layout: section-index
title: 架构设计
description: edgeCore 架构设计 — ScanEngine 调度驱动内核、SLA 统计调度、ShadowCore 影子设备与系统架构
hero_eyebrow: Architecture
hero_lead: edgeCore 系统架构权威文档 — ScanEngine 调度驱动采集（EDF/CB/SLA diagnostics）、ExecutionLayer 背压、ShadowCore COW 快照与 ShadowIngress 批量写入。运行时架构细节以此目录与 edge/ 总览为准。
hero_buttons:
  - text: 核心设计
    url: "4. 核心设计.html"
    style: primary
  - text: 影子设备
    url: "6. 影子设备设计.html"
    style: secondary
  - text: 返回首页
    url: ../index.html
    style: secondary
---

> **工程铁律：** 任何性能优化不得以牺牲稳定性为代价；任何架构优化不得增加系统恢复复杂度。

## 战略开发文档

> 所有架构设计与实现方案须对齐以下文档。

- [**开发原则与验收标准**](../DEVELOPMENT_PRINCIPLES.html) — 稳定性优先、工业级验证、高性能、轻量化
- [**分阶段路线图**](../ROADMAP.html) — Phase 1–4 实施顺序
- [**版本发布门禁**](../RELEASE_GATE.html) — 每版本四道门禁

## 目录

### 基础架构
- [**边缘网关架构设计总览**](../edge/边缘网关架构设计总览.html) — 全生命周期架构、南向差异、影子热路径与工业级稳定性（**权威 · v3.0**）
- [Architecture Overview (EN)](../en/architecture-overview.html) — English summary of the hot path
- [ScanEngine 重构方案](../TODO/ScanEngine重构方案.html) — 四阶段实施规范
- [Q3 采集优化方案](../%5BTODO%5D边缘计算南向采集优化方案2026第三季度.html) — Q3 交付验收与进度跟踪
- [架构 V2](ARCHITECTURE_V2.html) — 三级模型索引（权威见 [edge/边缘网关架构设计总览](../edge/边缘网关架构设计总览.html)）
- [状态机 API](STATE_MACHINE_API.html)
- [后端重构完成报告](BACKEND_RESTRUCTURING_COMPLETE.html) — 历史归档
- [数据源与输出动作设计](数据源与输出动作设计.html)

### SLA 调度与可观测（2026 Q3）

| 文档 | 说明 |
|------|------|
| [ScanEngine SLA 评估](../TODO/SLA评估.html) | Phase A–D 达标矩阵与工业边界 B1–B6 |
| [SLA 完成报告 2026Q3](../testing/sla_completion_report_2026Q3.html) | Phase A–C 测试命令与关键指标 |
| [确定性 SLA 报告](../testing/deterministic_sla_report.html) | EDF + hard jitter；P99 书面承诺 |
| [Shadow 性能优化报告](../testing/shadow_optimization_report_2026Q3.html) | COW / Worker Pool / ShadowIngress |
| [SLA 轻量化运维手册](../deployment/sla_monitoring.html) | diagnostics 巡检与告警响应 |

### 智能采集优化系列（ScanEngine 调度驱动内核）

<div align="center">
  <img src="../img/dataScanEngineCN.svg" width="100%" alt="edgeCore V2.0 架构 · ScanEngine引擎" />
</div>

> **edgeCore V2.0 架构 · ScanEngine 统一调度**：13 种南向驱动经 ScanEngine（EDF + CB + SLA metrics）写入 ShadowIngress → ShadowCore 快照，再联通虚拟设备、边缘计算与北向接口。

> 规范：`docs/TODO/ScanEngine重构方案.md` · 总览：`docs/edge/边缘网关架构设计总览.md`

> **编号设计文档（2.–10.）权威版本**在 [edge/](../edge/index.html) 目录；本目录保留索引入口，正文已归档为短索引页。

#### 项目分析与设计
- [1. 项目现状分析](../edge/1. 项目现状分析.html)
- [2. 智能画像方案设计](../edge/2. 智能画像方案设计.html)
- [3. 核心结构体定义](../edge/3. 核心结构体定义.html)
- [4. 核心设计](../edge/4. 核心设计.html)
- [5. 实现架构](../edge/5. 实现架构.html)

#### ShadowCore 影子设备系统
- [6. 影子设备设计](../edge/6. 影子设备设计.html)
- [影子设备与采集优化集成测试文档](../edge/影子设备与采集优化集成测试文档.html)
- [影子设备系统联动关系文档](../edge/影子设备系统联动关系文档.html)

#### 管理器实现
- [8. RTT 管理器实现](../edge/8. RTT管理器实现.html)
- [9. MTU 管理器实现](../edge/9. MTU管理器实现.html)
- [10. Gap 优化器实现](../edge/10. Gap优化器实现.html)

#### 运维与设备替换
- [7. 边缘运维与设备替换](../edge/7. 边缘运维与设备替换.html)

## 相关文档

- [边缘计算](../edge/index.html) — 边缘计算功能与场景
- [设备驱动](../drivers/index.html) — 南向驱动文档
- [测试验证](../testing/index.html) — SLA 报告与各模块测试文档
- [SLA 轻量化运维](../deployment/sla_monitoring.html) — diagnostics 巡检手册
