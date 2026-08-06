---
layout: default
title: 产品手册
description: edgeCore 工业边缘网关产品手册 
---

# edgeCore 产品手册

[English](PRODUCT.html) · [完整产品说明](产品说明.html) · [用户手册](USER_MANUAL.html)

面向制造、能源、楼宇现场的**工业边缘采集与计算网关**。Go 后端 · Vue 3 管理界面 · 单二进制静态部署。

## 核心价值

| 能力 | 简洁 |
|------|--------|
| **工业级稳定** | ScanEngine 调度内核、每设备断路器、背压与点位降级；SLA 可观测，Soak / CI 门禁验证 |
| **设备接入便捷** | 13 种南向协议，发现/扫描与批量点位；热配置无需重启进程 |
| **影子数据面** | ShadowCore 内存真源，UI / 边缘规则 / 历史 / 北向共用同一快照 |
| **边缘计算** | 阈值、表达式、窗口规则；虚拟影子公式聚合；本地写控联动 |
| **北向无缝接入** | MQTT · Sparkplug B · OPC UA Server · BACnet Server · HTTP · EdgeOS，对接 SCADA / 云平台 |
| **AI / MCP 协同** | AI Copilot 协议逆向 + MCP Server 33 工具，外部 LLM 安全操作网关（`internal/mcp`） |

## 南向协议一览

Modbus TCP/RTU · BACnet IP · OPC UA · Siemens S7 · EtherNet/IP · Omron FINS · SNMP · IEC 104 · DL/T645 · Mitsubishi SLMP · Profinet IO · KNXnet/IP · EtherCAT

完整矩阵与覆盖率：[南向驱动](../drivers/index.html) · [测试报告](../testing/南向驱动测试报告.html)

## AI 协同与 MCP

edgeCore 提供双通道 AI 协同能力：

| 通道 | 定位 | 协议 | 认证 |
|------|------|------|------|
| **AI Agent**（内部） | 协议逆向、文档解析、配置生成 | gRPC / REST | 系统 JWT |
| **MCP Server**（对外） | 通道/设备/点位 CRUD、边缘规则 | MCP (JSON-RPC 2.0) | MCP API Key |

- MCP 提供 33 个工具（8 只读 + 1 写操作 + 24 全功能 CRUD）
- 支持 Claude Desktop、Cursor、Windsurf、Continue.dev
- 全功能操作需用户 UI 显式激活

详见 [MCP 接入指南](mcp-access-guide.html)。

## 架构主线（影子设备）

```text
南向采集 → ShadowCore（真源）→ UI / 虚拟影子 / 边缘计算 / 持久化 / 北向上报
```

权威设计：[边缘网关架构设计总览](../edge/边缘网关架构设计总览.html)

## 部署规格

| 项 | 规格 |
|----|------|
| 交付 | 单二进制，`CGO_ENABLED=0` |
| 最低 | 128MB 内存 · 1GB 存储 |
| 架构 | x86_64 · ARM64 · ARMv7 |
| 安装 | deb / rpm / tar.gz / systemd |

安装步骤见 [用户手册](USER_MANUAL.html#安装指南)。

## 对标定位

同类工业采集 / 边缘软件（Kepware 多协议采集、Ignition / ThingsBoard Edge 边缘处理、Node-RED industrial 流编排）中，edgeCore 强调：**统一影子真源**、**调度驱动的统计 SLA**、**轻量单机现场交付**。
