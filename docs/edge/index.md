---
layout: section-index
title: 边缘计算
description: edgeCore 边缘计算 — 规则引擎、ShadowCore 影子设备与场景编排
hero_eyebrow: Edge Computing
hero_lead: edgeCore 边缘计算文档 — 规则引擎、数据流转、ShadowCore 影子设备、ScanEngine 调度与场景编排。
hero_buttons:
  - text: 基础功能
    url: 边缘计算基础功能.html
    style: primary
  - text: 规则帮助
    url: 边缘计算规则帮助.html
    style: secondary
  - text: 返回首页
    url: ../index.html
    style: secondary
---

> **工程铁律：** 任何性能优化不得以牺牲稳定性为代价；任何架构优化不得增加系统恢复复杂度。

## 文档与参考

边缘计算页面底部「文档与参考」与下列文档一一对应，均以当前 Go 实现（`EdgeComputeManager` + `DataPipeline`）为准。

| 文档 | 说明 |
|------|------|
| [边缘计算基础功能](边缘计算基础功能.html) | 架构能力、数据闭环与规则引擎总览（**规则引擎与数据流转说明**） |
| [边缘计算规则帮助](边缘计算规则帮助.html) | 规则类型、表达式、动作链配置详解（与 UI 帮助抽屉同步） |
| [边缘计算最佳实践](../guide/EDGE_COMPUTING_BEST_PRACTICES.html) | 部署与运维建议（**场景编排与性能建议**） |
| [场景手册](EDGE_COMPUTING_SCENARIO_MANUAL.html) | 典型工业场景与 JSON 配置示例 |
| [边缘计算 API](../API/Edge_Computing_CN.html) | REST 接口（**规则、指标与日志接口**） |

<div align="center">
  <img src="../img/dataScanEngineCN.svg" width="100%" alt="edgeCore 架构 · ScanEngine 统一调度" />
</div>

> **数据主路径：** 南向驱动 → **ScanEngine** → **ShadowIngress** → **ShadowCore** → **ShadowBridge** → **DataPipeline** → **EdgeComputeManager**（规则）/ 北向 / 历史落库。详见 [边缘计算基础功能](边缘计算基础功能.html)。

## 功能说明

- [边缘计算基础功能](边缘计算基础功能.html) — 规则引擎与数据流转
- [边缘计算规则帮助](边缘计算规则帮助.html) — 规则配置用户手册
- [边缘计算首页监控](边缘计算首页监控.html) — UI 监控页说明
- [边缘计算 Pipeline 配置指南](边缘计算Pipeline配置指南.html) — 历史对照（已归档，新部署请以上述文档为准）

## 最佳实践与场景

- [边缘计算最佳实践](../guide/EDGE_COMPUTING_BEST_PRACTICES.html)
- [场景手册](EDGE_COMPUTING_SCENARIO_MANUAL.html)

## ShadowCore / ScanEngine 设计参考

- [影子设备设计](6. 影子设备设计.html)
- [边缘网关架构设计总览](边缘网关架构设计总览.html)
- [RTT 管理器](8. RTT管理器实现.html) · [MTU 管理器](9. MTU管理器实现.html) · [Gap 优化器](10. Gap优化器实现.html)

## 相关文档

- [开发原则与验收标准](../DEVELOPMENT_PRINCIPLES.html) · [分阶段路线图](../ROADMAP.html)
- [架构设计](../architecture/index.html) · [北向数据](../northbound/index.html) · [测试验证](../testing/index.html)
