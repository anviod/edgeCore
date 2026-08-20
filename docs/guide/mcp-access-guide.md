---
layout: default
title: MCP 接入指南
description: edgeCore MCP Server 接入指南 — 通过 Model Context Protocol 安全操作工业网关
---

# edgeCore MCP 接入指南

[产品手册](PRODUCT.zh-CN.html) · [用户手册](USER_MANUAL.html) · [AI 协同规划](../TODO/AI协同组件规划.html)

## 概述

edgeCore MCP Server 通过 **Model Context Protocol (MCP)** 协议，让外部 LLM 应用（Claude Desktop、Cursor、Windsurf、Continue.dev 等）安全操作工业网关。

- 33 个工具（8 只读 + 1 写操作 + 24 全功能 CRUD）
- 6 个资源端点
- 13 个提示词模板
- MCP 协议版本：2024-11-05 / 2025-11-25（Streamable HTTP）
- 传输方式：JSON-RPC 2.0 over HTTP/SSE

## 系统架构

四层架构：LLM 客户端 → MCP 协议层 → edgeCore 网关 → 工业设备

- LLM 客户端层：Claude Desktop / Cursor / Windsurf / Continue.dev
- MCP 协议层：JSON-RPC 2.0 / SSE / Streamable HTTP
- edgeCore 网关层：认证 → 权限检查 → 工具分发 → 数据读写
- 工业设备层：Modbus / S7 / BACnet / OPC UA / SNMP / IEC 104 等 13 种协议

## 快速开始

### 1. 启用 MCP 服务

在 edgeCore 管理 UI 中：
1. 打开 AI 助手面板 → 点击「设置」
2. 选择「MCP 接入」Tab
3. 开启「MCP 服务」开关
4. 设置 API Key（可点击「生成密钥」自动生成 256 位随机密钥）

### 2. 客户端配置

将以下 JSON 配置添加到 MCP 客户端配置文件中，替换 `<host>` 和 `<mcp_api_key>`：

**Claude Desktop / Cursor / Windsurf**：
```json
{
  "mcpServers": {
    "edgeCore": {
      "url": "http://<gateway-ip>:8080/api/mcp",
      "headers": {
        "Authorization": "Bearer <mcp_api_key>"
      }
    }
  }
}
```

**Continue.dev**：
```json
{
  "mcpServers": {
    "edgeCore": {
      "transport": {
        "type": "http",
        "url": "http://<gateway-ip>:8080/api/mcp"
      },
      "auth": {
        "type": "bearer",
        "token": "<mcp_api_key>"
      }
    }
  }
}
```

### 3. 验证连接

启动 LLM 客户端后，尝试询问：
- "列出所有采集通道"（调用 `edgeCore_list_channels`）
- "网关系统信息"（调用 `edgeCore_get_system_info`）

## 认证与权限

### 认证方式

MCP 使用独立于系统 JWT 的 API Key 认证，支持两种 Header：

| Header | 格式 | 说明 |
|--------|------|------|
| `Authorization` | `Bearer <mcp_api_key>` | 标准 Bearer Token |
| `X-MCP-API-Key` | `<mcp_api_key>` | 专用 Header |

API Key 为 64 字符十六进制字符串（256 位熵），在 UI 中生成和管理。

### 权限层级

| 层级 | 说明 | 工具数 |
|------|------|--------|
| 只读权限 | 默认状态，查询类工具可用 | 8 |
| 写操作 | 需人工确认，不自动执行 | 1 |
| 全功能权限 | 需在 UI 显式激活后可用 | 24 |

### 安全说明

- 全功能 CRUD 操作必须经用户 UI 确认激活
- MCP API Key 未设置时，MCP 端点拒绝所有请求
- 写操作需人工确认，不自动执行
- API Key 独立于系统 JWT，可随时更换
- 敏感信息已脱敏处理，端点仅在内网暴露

## 工具清单

### 只读查询工具（8 个）

| 工具名 | 功能 | 必填参数 |
|--------|------|----------|
| `edgeCore_list_channels` | 列出所有采集通道及状态 | 无 |
| `edgeCore_list_devices` | 列出指定通道下所有设备 | channel_id |
| `edgeCore_list_points` | 列出指定设备下所有点位（含当前值） | channel_id, device_id |
| `edgeCore_read_point` | 读取点位实时值 | channel_id, device_id, point_id |
| `edgeCore_get_system_info` | 系统信息（CPU/内存/协议列表） | 无 |
| `edgeCore_get_diagnostics` | 通道/设备诊断信息 | 无（channel_id/device_id 可选） |
| `edgeCore_analyze_protocol` | 协议特征分析 | 无（protocol_hint/port 可选） |
| `edgeCore_get_protocol_help` | 协议接入帮助（地址格式/功能码/示例） | protocol |

### 写操作工具（1 个，需人工确认）

| 工具名 | 功能 | 必填参数 |
|--------|------|----------|
| `edgeCore_write_point` | 向 R/W 点位写入控制值 | channel_id, device_id, point_id, value |

### 全功能 CRUD 工具（26 个，需 UI 激活）

**通道管理（4 个）**

| 工具名 | 功能 |
|--------|------|
| `edgeCore_create_channel` | 创建南向采集通道 |
| `edgeCore_delete_channel` | 删除通道（含设备和点位） |
| `edgeCore_start_channel` | 启动通道采集引擎 |
| `edgeCore_stop_channel` | 停止通道采集引擎 |

**设备管理（5 个）**

| 工具名 | 功能 |
|--------|------|
| `edgeCore_create_device` | 创建设备 |
| `edgeCore_batch_create_devices` | 批量创建设备（适用于扫描结果导入） |
| `edgeCore_delete_device` | 删除设备（含点位） |
| `edgeCore_update_device` | 更新设备配置 |
| `edgeCore_enable_device` | 启用/禁用设备 |

**点位管理（6 个）**

| 工具名 | 功能 |
|--------|------|
| `edgeCore_create_point` | 创建采集点位 |
| `edgeCore_batch_create_points` | 批量创建点位（适用于扫描/对象浏览结果导入） |
| `edgeCore_delete_point` | 删除点位 |
| `edgeCore_update_point` | 更新点位配置 |
| `edgeCore_read_point_batch` | 批量读取点位实时值 |
| `edgeCore_write_point_batch` | 批量写入点位值 |

**边缘规则（3 个）**

| 工具名 | 功能 |
|--------|------|
| `edgeCore_create_edge_rule` | 创建边缘计算规则 |
| `edgeCore_delete_edge_rule` | 删除边缘规则 |
| `edgeCore_list_edge_rules` | 列出所有边缘规则 |

**虚拟设备（2 个）**

| 工具名 | 功能 |
|--------|------|
| `edgeCore_create_virtual_device` | 创建虚拟设备（公式计算） |
| `edgeCore_delete_virtual_device` | 删除虚拟设备 |

**扩展工具（6 个）**

| 工具名 | 功能 |
|--------|------|
| `edgeCore_restart_channel` | 重启通道采集引擎 |
| `edgeCore_get_channel_config` | 获取通道完整配置 |
| `edgeCore_get_point_history` | 获取点位历史数据 |
| `edgeCore_export_config` | 导出完整配置（json/yaml） |

## 资源端点

| URI | 名称 | 说明 |
|-----|------|------|
| `edgeCore://channels` | 通道列表 | 所有采集通道完整配置 |
| `edgeCore://system` | 系统信息 | 网关系统状态 |
| `edgeCore://diagnostics` | 诊断快照 | 通道和设备诊断汇总 |
| `edgeCore://protocols` | 协议支持列表 | 15 种工业协议完整列表 |
| `edgeCore://edge-rules` | 边缘规则 | 所有边缘计算规则配置和状态 |
| `edgeCore://config` | 完整配置 | edgeCore 完整配置导出 |

## 提示词模板

| 名称 | 描述 | 必填参数 |
|------|------|----------|
| `protocol-reverse` | 工业协议逆向工程 | protocol* |
| `channel-config` | 生成通道配置 JSON | protocol*, ip* |
| `diagnostics-analyze` | 诊断分析 | channel_id* |
| `modbus-quick-start` | Modbus TCP/RTU 快速接入 | ip* |
| `s7-quick-start` | Siemens S7 快速接入 | ip* |
| `bacnet-quick-start` | BACnet/IP 接入 | device_id |
| `opcua-quick-start` | OPC UA 接入 | endpoint* |
| `point-batch-generator` | 点位批量生成 | protocol*, start_address*, count* |
| `edge-rule-builder` | 边缘规则构建 | rule_type* |
| `troubleshooting-guide` | 故障排查流程 | issue_type* |
| `data-flow-architect` | 数据流架构设计 | target |
| `gateway-health-check` | 网关健康检查 | 无 |
| `protocol-migration` | 协议迁移指南 | from*, to* |

## API 端点

### MCP 协议端点（API Key 认证）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/mcp` | JSON-RPC 2.0 请求入口 |
| GET | `/api/mcp` | MCP Streamable HTTP SSE 流 |
| DELETE | `/api/mcp` | 终止 MCP 会话 |

### MCP 管理端点（JWT 认证）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/mcp/help` | MCP 接入帮助文档 |
| POST | `/api/mcp/activate` | 激活/关闭全功能读写 |
| GET | `/api/mcp/status` | 查询 MCP 激活状态 |
| GET | `/api/mcp/key` | 获取 MCP API Key（仅 JWT 用户） |
| POST | `/api/mcp/generate-key` | 生成 256 位随机 API Key |

## 会话管理

MCP 2025-11-25 Streamable HTTP 会话通过 `Mcp-Session-Id` 头管理：
- 会话在首次 POST 请求时创建
- SSE 流每 30 秒发送心跳
- DELETE 请求终止会话
