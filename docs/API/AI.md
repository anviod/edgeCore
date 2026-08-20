---
layout: default
title: AI 能力专题 — MCP 与 EAN 2.0
description: edgeCore 工业边缘网关 AI 能力体系 — MCP 协议操作接口与 EAN 2.0 能力自治网络的技术拆解
---

# AI 能力专题 — MCP 与 EAN 2.0

[产品指南](../guide/PRODUCT.zh-CN.html) · [MCP 接入指南](../guide/mcp-access-guide.html) · [AI 协同规划](../TODO/AI协同组件规划.html) · [EAN 改造指南](../edgeos/EAN2.0-edgeCore-EdgeOS改造指南.html)

edgeCore v2.x 在工业采集内核之上叠加了两层 AI 能力：**MCP Server** 把网关的通道、设备、点位暴露给外部 LLM 客户端操作；**EAN 2.0 Capability Runtime** 把 15 种协议驱动的读写扫描能力统一为可发现、可调用的 Capability，再通过 MCP 桥接层暴露为 `ean_*` 工具。两层合计 94 个 MCP 工具，覆盖从只读诊断到全功能 CRUD 的完整操作链路。

这套能力的设计前提只有一条：**AI 可以辅助工程师，但不能替工程师按下确认键**。所有写操作——创建通道、写入点位、删除设备——都卡在 Human-in-the-loop 门禁后面，LLM 能生成配置，但不能直接落库。AI 故障时，ScanEngine、ShadowCore、Execution Mapper 照常运行，采集不中断。

---

## MCP：LLM 操作工业网关的标准通道

MCP（Model Context Protocol）是 Anthropic 在 2024 年提出的开放协议，定义了 LLM 客户端与外部工具服务器之间的通信规范。edgeCore MCP Server 实现了该协议的 2024-11-05 和 2025-11-25 两个版本，传输层为 JSON-RPC 2.0 over Streamable HTTP / SSE。

### 四层架构

```
LLM 客户端              MCP 协议层              edgeCore 网关              工业设备
─────────────         ──────────────         ──────────────         ──────────────
Claude Desktop        JSON-RPC 2.0           JWT 认证                Modbus RTU/TCP
Cursor                SSE Stream             API Key 权限检查         BACnet IP
Windsurf              Streamable HTTP        工具分发                 OPC UA
Continue.dev                                 数据读写                 S7 / EtherNet/IP
                                                                      IEC 104 / SNMP ...
```

LLM 客户端发起自然语言指令，MCP 协议层将其转化为结构化的 `tools/call` 请求，edgeCore 网关收到后走 JWT 认证和 API Key 权限检查，再分发到对应工具处理器。处理器调用 ScanEngine、ShadowCore 或 Execution Mapper 拿到结果，原路返回。

### 35 个原生工具

工具按权限分三级，从严到松：

| 权限层级 | 工具数 | 激活方式 | 典型工具 |
|---------|--------|---------|---------|
| 只读查询 | 8 | 默认可用 | `edgeCore_list_channels` `edgeCore_read_point` `edgeCore_get_system_info` |
| 写操作 | 1 | 需人工确认 | `edgeCore_write_point` |
| 全功能 CRUD | 26 | 需 UI 显式激活 | `edgeCore_create_channel` `edgeCore_delete_device` `edgeCore_create_edge_rule` |

只读工具覆盖了通道列表、设备列表、点位实时值、系统诊断、协议特征分析。写操作工具 `edgeCore_write_point` 向 R/W 点位写入控制值，每次调用都需要人工确认，LLM 无法自动执行。全功能 CRUD 工具覆盖通道管理（6）、设备管理（5）、点位管理（6）、边缘规则（3）、虚拟设备（2）和扩展工具（4），用户必须在管理 UI 的「MCP 接入」面板里手动开启全功能开关才能解锁。

### 6 个资源端点

资源是 MCP 协议中的结构化数据 URI，LLM 客户端可通过 `resources/read` 一次性拉取完整数据快照：

| URI | 内容 |
|-----|------|
| `edgeCore://channels` | 所有采集通道完整配置 |
| `edgeCore://system` | 网关系统状态 |
| `edgeCore://diagnostics` | 通道和设备诊断汇总 |
| `edgeCore://protocols` | 15 种工业协议完整列表 |
| `edgeCore://edge-rules` | 所有边缘计算规则配置和状态 |
| `edgeCore://config` | edgeCore 完整配置导出 |

### 13 个提示词模板

提示词模板是预置的工业场景指令，LLM 客户端通过 `prompts/get` 获取结构化提示后执行：

| 模板 | 用途 |
|------|------|
| `protocol-reverse` | 工业协议逆向工程 |
| `channel-config` | 生成通道配置 JSON |
| `diagnostics-analyze` | 诊断分析 |
| `modbus-quick-start` | Modbus TCP/RTU 快速接入 |
| `s7-quick-start` | Siemens S7 快速接入 |
| `bacnet-quick-start` | BACnet/IP 接入 |
| `opcua-quick-start` | OPC UA 接入 |
| `point-batch-generator` | 点位批量生成 |
| `edge-rule-builder` | 边缘规则构建 |
| `troubleshooting-guide` | 故障排查流程 |
| `data-flow-architect` | 数据流架构设计 |
| `gateway-health-check` | 网关健康检查 |
| `protocol-migration` | 协议迁移指南 |

### 认证模型

MCP 使用独立于系统 JWT 的 API Key 认证体系。API Key 为 64 字符十六进制字符串（256 位熵），在管理 UI 中一键生成。客户端通过以下任一 Header 传递：

| Header | 格式 |
|--------|------|
| `Authorization` | `Bearer <mcp_api_key>` |
| `X-MCP-API-Key` | `<mcp_api_key>` |

API Key 未设置时，MCP 端点拒绝所有请求。全功能 CRUD 操作还需用户在 UI 中显式激活开关。写操作 `edgeCore_write_point` 在激活全功能后仍需人工确认，不自动执行。

### 客户端配置示例

Claude Desktop / Cursor / Windsurf：

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

连接后可直接用自然语言操作：

- "列出所有采集通道" → 调用 `edgeCore_list_channels`
- "读取 1 号设备的温度点位" → 调用 `edgeCore_read_point`
- "分析 3 号通道的 Modbus 报文特征" → 调用 `edgeCore_analyze_protocol`
- "网关系统信息" → 调用 `edgeCore_get_system_info`

---

## EAN 2.0：能力自治网络

EAN（Edge Agent Network）2.0 是 edgeCore 与 EdgeOS 之间的统一能力协作层。edgeCore 作为 Agent 注册到 EAN 网络，把 15 种协议驱动的读写扫描能力封装为标准 Capability；EdgeOS 作为协调平台，负责发现、编排和治理。

### 设计原则

EAN 2.0 不是重新设计——它在现有 edgeCore + EdgeOS 架构上增加一层 Agent 协作能力。四条原则贯穿整个实现：

- **edgeCore 改动最小**：复用已有 AI、MCP、Execution Mapper、ShadowCore、ScanEngine、Driver
- **EdgeOS 增加平台能力**：发现、编排、治理、调度、资源管理
- **协议统一**：Capability、Discovery、Invoke、Event 统一模型
- **Capability 为核心**：所有能力——Driver Command、AI Skill、MCP Tool、Workflow Node——统一映射到 Capability

### 统一能力模型

```
  Device    AI    Workflow    Service    Cloud
     \       |       /          /          /
      \      |      /          /          /
       \     |     /          /          /
        \    |    /          /          /
         \   |   /          /          /
          \  |  /          /          /
           \ | /          /          /
            Agent
               |
          Capability
               |
            Invoke
               |
          Execution
```

五种 Agent 类型（Device / AI / Workflow / Service / Cloud）统一到 Agent 模型，每个 Agent 持有若干 Capability，外部通过 Invoke 调用 Capability，Execution 层将 Capability 映射到底层 Driver Command 执行。

### 15 种协议 × 4 种能力 = 60 个设备能力

EAN 2.0 覆盖 15 种工业协议，每种协议自动生成 4 个标准能力：

| 能力 | 说明 | 权限 |
|------|------|------|
| `read_holding_register` | 读取设备保持寄存器/点位值 | read |
| `write_register` | 写入单个寄存器/点位值 | write |
| `scan_devices` | 扫描/发现指定通道下的设备 | read |
| `list_points` | 列出指定设备的所有点位 | read |

15 种协议清单：

| 序号 | 协议 | Capability 前缀 | 典型场景 |
|------|------|----------------|---------|
| 1 | Modbus TCP | `modbus_tcp` | PLC、仪表、变频器 |
| 2 | Modbus RTU | `modbus_rtu` | 串口设备 |
| 3 | Modbus RTU over TCP | `modbus_rtu_over_tcp` | 串口转网关 |
| 4 | BACnet/IP | `bacnet_ip` | 楼宇自控 |
| 5 | Siemens S7 | `s7` | S7-200/300/400/1200/1500 |
| 6 | OPC UA | `opc_ua` | 统一互操作 |
| 7 | EtherNet/IP | `ethernet_ip` | Allen-Bradley PLC |
| 8 | Omron FINS | `omron_fins` | Omron CJ/CP 系列 |
| 9 | IEC 60870-5-104 | `iec60870_5_104` | 电力远动 |
| 10 | KNXnet/IP | `knxnet_ip` | 楼宇自动化 |
| 11 | SNMP | `snmp` | 网络设备监控 |
| 12 | DL/T 645-2007 | `dlt645` | 电力抄表 |
| 13 | Mitsubishi SLMP | `mitsubishi_slmp` | 三菱 PLC |
| 14 | Profinet IO | `profinet_io` | 实时以太网 |
| 15 | EtherCAT | `ethercat` | 运动控制 |

### 3 个系统与 AI 能力

除协议能力外，EAN 2.0 还提供 3 个跨协议的系统级能力：

| 能力 ID | 类别 | 说明 | 超时 |
|---------|------|------|------|
| `system.diagnostics` | system | 收集 edgeCore 系统诊断信息（通道状态、设备统计、资源使用） | 15s |
| `ai.protocol_reverse` | ai | AI 辅助协议逆向工程，输入抓包数据，输出候选协议结构 | 120s |
| `ai.doc_parse` | ai | AI 辅助协议文档解析，输入文档内容，输出点位配置列表 | 120s |

`ai.protocol_reverse` 解决的是现场工程师拿到未知设备时"花 2 天分析协议"的痛点——AI 30 分钟生成候选配置，工程师确认后上线。`ai.doc_parse` 解决的是手动逐条录入点位地址的低效问题——扔一份协议手册进去，批量生成点位配置。

### 传输与发现

EAN 2.0 支持四种传输方式：

| 传输类型 | 说明 | 使用场景 |
|---------|------|---------|
| MQTT | 标准 MQTT 消息总线 | EdgeOS 集成（推荐） |
| NATS | NATS Streaming | 高吞吐场景 |
| HTTP | REST API 调用 | 轻量集成 |
| SDK | 进程内直接调用 | 本地调试 |

Agent 启动时通过 Discovery Publisher 发布 Agent Descriptor（包含 Agent ID、版本、状态、能力列表）到 `discovery/agent` 主题。其他 Agent 或 EdgeOS 可通过 `discovery/query` 主题发起查询，按 Agent ID、Capability ID、Category 或关键词过滤。

心跳机制每 60 秒（可配置 10~600 秒）发布一次 `agent_heartbeat` 消息，每 60 秒周期性重发 Capability Descriptor，解决启动时序竞态——如果 EdgeOS 比 edgeCore 晚启动，心跳重发能保证能力发现不遗漏。

### Invoke 调度

Invoke 是 EAN 2.0 的核心调用模型。一次完整的 Invoke 流程：

```
InvokeRequest
  ├─ invoke_id     (唯一标识)
  ├─ target        (目标 Agent ID)
  ├─ capability    (能力 ID，如 modbus_tcp.read_holding_register)
  ├─ arguments     (调用参数 JSON)
  └─ options
       ├─ timeout_sec  (超时秒数，覆盖能力默认值)
       ├─ priority     (优先级)
       ├─ retry        (重试次数)
       └─ async        (是否异步)
     │
     ▼
  Dispatcher
     ├─ 验证 capability 是否存在
     ├─ 验证 target 是否匹配
     ├─ 创建超时 context
     ├─ 状态流转: queued → running → completed/failed/timeout
     │
     ▼
  Execution Mapper
     ├─ Capability → DriverCommand 映射
     │    ├─ protocol:  从 Capability.Metadata 提取
     │    ├─ command:   ReadPoints / WritePoint / ScanDevices / GetDevicePoints
     │    └─ args:      透传 InvokeRequest.Arguments
     │
     ▼
  Driver Executor
     ├─ 调用 ScanEngine / Driver / AI Adapter
     └─ 返回结果
     │
     ▼
  InvokeResponse
     ├─ invoke_id
     ├─ status:     completed / failed / timeout / rejected
     ├─ result:     { success, values, timestamp, error? }
     └─ latency_ms
```

Dispatcher 内部维护 Invoke 记录表，支持异步状态查询。每次 Invoke 结束后，Event Publisher 发布 `capability.invoked` 事件，包含能力 ID、状态和延迟数据。Invoke Metrics 收集器实时统计调用次数、成功率、延迟分布（P50/P99）和错误码频率。

---

## MCP × EAN：桥接层

MCP 桥接层是连接 EAN Capability Runtime 和 MCP 协议的适配器。它把 63 个 EAN Capability 自动转换为 MCP 工具，加上 31 个原生 edgeCore 工具，LLM 客户端一次连接就能访问 94 个工具。

### 63 + 31 = 94 个工具

| 来源 | 前缀 | 工具数 | 生成方式 |
|------|------|--------|---------|
| EAN Capability | `ean_` | 63 | 自动生成（协议能力 60 + 系统/AI 能力 3） |
| edgeCore 原生 | `edgeCore_` | 31 | 手写注册（只读 8 + 写操作 1 + CRUD 22） |

EAN Capability 到 MCP 工具的映射规则：

```
Capability ID                    →  MCP Tool Name
─────────────────────────────────    ──────────────────────────────────────
modbus_tcp.read_holding_register →  ean_modbus_tcp_read_holding_register
bacnet_ip.list_points            →  ean_bacnet_ip_list_points
system.diagnostics               →  ean_system_diagnostics
ai.protocol_reverse              →  ean_ai_protocol_reverse
```

Capability 的 `InputSchema` 自动转换为 MCP 工具的 `inputSchema`，包括参数类型、描述、必填项和默认值。LLM 客户端拿到的工具定义与直接调用 EAN REST API 的参数完全一致。

### 写操作门禁

桥接层在 `RegisterCapabilityTools` 中安装了写操作门禁。当 LLM 调用 `ean_*_write_register` 这类 write/admin 权限的工具时，桥接层先调用 `writeGate()` 检查：

- 全功能开关未激活 → 返回拦截结果，工具不执行
- 全功能开关已激活 → 透传到 Capability Runtime 的 Invoke 通道

门禁逻辑在 MCP 协议层完成，不侵入 Capability Runtime 内部。Runtime 本身只管调度和执行，不关心调用方是 LLM 还是 HTTP API。

---

## 安全边界

### Human-in-the-loop

所有写操作——无论来自 MCP 原生工具还是 EAN 桥接工具——都必须经过人工确认才能执行。LLM 能生成通道配置 JSON、能计算出点位地址、能推导出写入值，但不能按下"确认"键。这个约束不是配置项，是架构层面的硬限制：`edgeCore_write_point` 的处理器在返回结果前检查 `writeGate()`，未确认时直接返回拦截响应。

### AI 不进热路径

AI 能力（`ai.protocol_reverse`、`ai.doc_parse`）的超时设为 120 秒，远高于协议读写能力的 10 秒。这个差距是刻意的——AI 调用走的是远端 Model Center，网络延迟和推理时间不可控。如果 AI 服务宕机或超时，ScanEngine 的采集循环、ShadowCore 的状态更新、Execution Mapper 的规则执行不受影响。AI 故障时，网关退化为纯采集模式，工业数据照常流转。

### API Key 隔离

MCP API Key 与系统 JWT 完全独立。即使 API Key 泄露，攻击者也只能访问 MCP 暴露的工具集，无法触达 JWT 保护的系统管理 API。API Key 可随时在 UI 中重新生成，旧 Key 立即失效。

---

## 实战场景

### 场景 1：未知设备协议逆向

现场拿到一台未知 PLC，只有串口接线和一个抓包文件。

1. 工程师把抓包数据上传给 LLM 客户端
2. LLM 调用 `edgeCore_analyze_protocol` 分析报文特征
3. LLM 调用 `ean_ai_protocol_reverse`，传入抓包数据
4. AI 返回候选协议结构（可能是 Modbus RTU，地址从 0x01 开始）
5. 工程师确认协议类型，LLM 调用 `modbus-quick-start` 提示词生成通道配置
6. 工程师在 UI 中确认配置，创建通道
7. LLM 调用 `ean_modbus_rtu_scan_devices` 扫描设备
8. LLM 调用 `ean_modbus_rtu_list_points` 列出点位

整个过程从"2 天手动分析"压缩到"30 分钟 AI 辅助 + 工程师确认"。

### 场景 2：批量点位录入

一台 Modbus TCP 设备有 200 个点位，手动录入需要半天。

1. 工程师把设备协议手册的 PDF 内容给 LLM
2. LLM 调用 `ean_ai_doc_parse`，传入文档内容
3. AI 返回 200 条点位配置（地址、数据类型、缩放因子）
4. LLM 调用 `point-batch-generator` 提示词，指定起始地址和数量
5. 工程师在 UI 中确认批量导入
6. LLM 调用 `edgeCore_read_point_batch` 验证点位值

### 场景 3：故障诊断

3 号通道采集中断，需要快速定位。

1. 工程师问 LLM："3 号通道为什么没有数据"
2. LLM 调用 `edgeCore_get_diagnostics`，传入 `channel_id=3`
3. 诊断信息显示：通道运行中，但 3 号设备连接超时
4. LLM 调用 `edgeCore_list_devices`，确认设备配置正常
5. LLM 调用 `ean_modbus_tcp_read_holding_register`，传入设备地址
6. 返回连接拒绝错误，定位为网络层问题
7. LLM 调用 `troubleshooting-guide` 提示词，生成排查步骤

---

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

### EAN REST 端点（JWT 认证）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/capability/agent/status` | Agent 状态 |
| GET | `/api/capability/list` | 能力列表（支持 category/keyword 过滤） |
| GET | `/api/capability/list/:id` | 能力详情（含完整 input/output schema） |
| POST | `/api/capability/invoke` | 调用能力 |
| GET | `/api/capability/invoke/:id/status` | 查询调用状态 |
| GET | `/api/capability/discovery/agents` | Agent 发现 |
| GET | `/api/capability/events/history` | 事件历史（线程安全环形缓冲，容量 200） |
| DELETE | `/api/capability/events/history` | 清除事件历史 |
| GET | `/api/capability/settings` | 获取 EAN 配置 |
| PUT | `/api/capability/settings` | 更新 EAN 配置 |
| WS | `/api/capability/events/stream` | 事件实时推送（WebSocket） |

---

## 工程约束

| 约束 | 说明 |
|------|------|
| 写操作需人工确认 | `edgeCore_write_point` 和 `ean_*_write_register` 不自动执行 |
| AI 不进热路径 | ScanEngine / Pipeline Worker 不调用 AI，AI 故障不影响采集 |
| Capability Invoke 超时 5s | 协议读写能力默认 10s 超时，可通过 options 覆盖 |
| 事件缓冲容量 200 | 线程安全环形缓冲，超出自动淘汰旧事件 |
| API Key 256 位熵 | 64 字符十六进制，独立于系统 JWT |
| 全功能需 UI 激活 | 24 个 CRUD 工具默认锁定，用户手动开启 |
| 心跳 10~600s | 默认 60s，Agent 每周期重发 Capability Descriptor |
| Agent 版本 2.0.0 | EAN Runtime 版本号，上报到 Discovery |

---

## 代码路径

| 模块 | 路径 | 说明 |
|------|------|------|
| MCP Server | `internal/mcp/server.go` | JSON-RPC 引擎、工具/资源/提示词管理 |
| MCP 协议层 | `internal/mcp/protocol.go` | MCP 消息类型、错误码、版本协商 |
| EAN Capability 桥接 | `internal/mcp/capability_adapter.go` | Capability → MCP Tool 映射、写操作门禁 |
| EAN Runtime | `internal/capability/runtime.go` | Agent 生命周期、心跳、发现、Invoke |
| EAN Dispatcher | `internal/capability/dispatcher.go` | Invoke 调度、超时控制、状态机 |
| EAN Registry | `internal/capability/registry.go` | Capability 注册表 |
| Capability 生成器 | `internal/capability/generator.go` | 15 协议 × 4 能力 + 3 系统能力自动生成 |
| Execution Mapper | `internal/execution/capability_mapper.go` | Capability → DriverCommand 映射 |
| Driver Executor | `internal/execution/driver_executor.go` | DriverCommand → ScanEngine/Driver 执行 |
| AI Adapter | `internal/execution/ai_adapter.go` | AI 能力适配器 |
| AI Agent | `internal/ai_agent/` | AI Pipeline、配额、验证 |
| MCP Handler | `internal/server/mcp_handler.go` | HTTP 层 MCP 端点注册 |

---

## 相关文档

- [MCP 接入指南](../guide/mcp-access-guide.html) — 客户端配置、认证流程、工具清单
- [AI 协同组件规划](../TODO/AI协同组件规划.html) — EAN 2.0 架构设计、能力模型、V1.5→V2.0 映射
- [EAN 2.0 改造指南](../edgeos/EAN2.0-edgeCore-EdgeOS改造指南.html) — edgeCore/EdgeOS 联调方案
- [通信协议规范](../edgeos/edgeCore通信协议规范(MQTT-NATS).html) — MQTT/NATS 主题与消息格式
- [EAN 联合验证报告](../TODO/EAN2-Joint-Verification-Report.html) — 全链路测试结果
