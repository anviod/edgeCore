# edgeCore端 通信协议规范 (MQTT/NATS) — EAN 2.0

> **文档版本**: V2.0  
> **最后更新**: 2026-07-27  
> **维护者**: edgeOS 团队  
> **文档定位**: 本文档定义 **Edge Agent Network（EAN）2.0** 基于 MQTT v5 / NATS 2.x 的统一工业智能体协作协议。涵盖共性协议层（Agent / Capability / Discovery / Invoke / Event）、edgeCore Capability Runtime 接入规范、EdgeOS Coordination Platform 平台规范，以及 V1.0 兼容层 Topic 保留。  
> **适用对象**: edgeCore 边缘网关（Capability Runtime）、EdgeOS 蜂群网络（Coordination Platform）、第三方 Runtime 实现者。

---

## 版本变更摘要

| 版本 | 日期 | 变更说明 |
|------|------|---------|
| v1.0 | 2026-04-21 | 初始版本：edgeCore ↔ EdgeOS 专用 Topic/Subject 与消息体（节点注册、设备上报、下行控制等） |
| **v2.0** | **2026-07-27** | **全面升级至 EAN 2.0**：新增统一 Agent 模型、Capability 模型、Discovery/Invoke/Event 协议；edgeCore 作为 Capability Runtime 接入；EdgeOS 作为 Coordination Platform；保留 V1.0 Topic 作为兼容层 |

---

## 0. 架构总览

### 0.1 设计原则

> **不是重新设计，而是在现有 edgeCore + EdgeOS 架构上增加一层统一的 Agent 协作能力。**

- **edgeCore 改动最小** — 复用已有 AI、MCP、Execution Mapper、ShadowCore
- **EdgeOS 增加平台能力** — 发现、编排、治理
- **协议统一** — Capability、Discovery、Invoke、Event

### 0.2 统一能力模型

```text
Device    AI    Workflow    Service    Cloud
         \      |      /      /
          \     |     /      /
           \    |    /      /
            \   |   /      /
             \  |  /      /
              Agent
                 |
            Capability
                 |
              Invoke
                 |
            Execution
```

### 0.3 EAN 2.0 三层架构

```text
         EdgeOS Agent Network（EAN）2.0
    ─────────────────────────────────────

    ┌────────── 共性协议（Protocol） ──────────┐
    │ Agent │ Capability │ Discovery │ Invoke  │
    │ Event │ Registry   │ Workflow  │ QoS     │
    │ Shadow│ Security   │ Metrics   │         │
    └──────────────────────────────────────────┘
                     │
      ┌──────────────┴──────────────┐
      ▼                             ▼
EdgeOS Coordination      edgeCore Capability
    Platform                Runtime
───────────────────    ───────────────────
Registry Center        Capability Registry
Discovery Center       Invoke Dispatcher
Workflow Center        MCP Adapter
AI Planner             AI Planner Adapter
Scheduler              Execution Mapper
Resource Manager       ShadowCore
Event Center           ScanEngine
Security               Device Drivers
Metrics
    └──────────────┬──────────────┘
                   ▼
        MQTT v5 / NATS 2.x Message Bus
```

### 0.4 协议职责边界

| 职责 | 共性协议 | edgeCore Runtime | EdgeOS Platform |
|------|---------|---------------|-----------------|
| Agent 注册/发现 | 定义模型与 Topic | 发布 Descriptor | 聚合索引 |
| Capability 描述 | 统一 Schema | 自动生成/注册/执行 | 聚合/检索/跨节点发现 |
| Invoke 调用 | 统一请求/响应格式 | 接收请求并执行 | 发起跨 Agent 调用、编排 |
| Event 通知 | 统一 Event 模型 | 发布设备事件 | 订阅、路由、规则处理 |
| Execution | 定义状态机 | Execution Mapper → Driver | 不直接执行设备操作 |
| Shadow | 统一状态模型 | ShadowCore 维护 | 聚合状态、跨节点同步 |
| Workflow | 定义节点规范 | 提供 Capability 节点 | 负责流程编排与调度 |
| AI | 定义 Planner 接口 | MCP、Planner、Tool Adapter | 多 Agent 任务规划 |

---

# 第一章 共性部分（Protocol & Common Specification）

> 本章所有 Runtime 共用。不涉及 edgeCore 具体实现，不涉及 EdgeOS 具体实现。第三方 Runtime 也可据此实现互操作。

---

## 1.1 设计目标

统一所有能力模型，协议只负责以下六件事：

1. **Agent 注册** — Agent 上线时发布自身描述
2. **Agent 发现** — 查询网络中可用的 Agent
3. **Capability 描述** — 统一能力元数据 Schema
4. **Capability 调用** — 统一的请求/响应/状态机
5. **Event 通知** — 统一事件发布/订阅/回放
6. **状态同步** — Shadow 状态读写

**协议不负责**：AI 推理、设备驱动实现、Workflow 具体实现、业务逻辑。

---

## 1.2 Agent Model

### 1.2.1 Agent 定义

```yaml
agent:
  id: "edgeCore-node-001"           # 全局唯一标识
  kind: "device"                 # agent 类型
  version: "2.0.0"               # agent 版本
  status: "online"               # online | offline | degraded | error
  transport: "mqtt"              # mqtt | nats | http | sdk
  heartbeat_interval_sec: 30     # 心跳间隔（秒）
  metadata:                      # 扩展元数据
    os: "linux"
    arch: "arm64"
    hostname: "edgeCore-node-001.local"
  capabilities: []               # Capability 列表（见 1.3）
```

### 1.2.2 Agent 类型（kind）

| kind | 说明 | 示例 |
|------|------|------|
| `device` | 设备采集 Agent | edgeCore 网关 |
| `ai` | AI 推理 Agent | AI Model Center |
| `workflow` | 工作流 Agent | EdgeOS Workflow Engine |
| `service` | 通用服务 Agent | 日志服务、监控服务 |
| `cloud` | 云端 Agent | 云平台连接器 |

### 1.2.3 Agent 生命周期

```text
启动
  │
  ▼
发布 Agent Descriptor  →  $edgeos/discovery/agent
  │
  ▼
发送 Heartbeat（周期性） →  $edgeos/heartbeat/{agent_id}
  │
  ▼
发布 Capability Descriptor  →  $edgeos/discovery/capability
  │
  ▼
接收 Invoke 请求 / 发布 Event
  │
  ▼
下线（Graceful Shutdown）
  │
  ▼
发布 Agent Offline  →  $edgeos/discovery/agent/offline
```

### 1.2.4 Agent Descriptor 消息格式

**Topic**: `$edgeos/discovery/agent`

```json
{
  "header": {
    "message_id": "msg-agent-desc-001",
    "timestamp": 1776787200000,
    "source": "edgeCore-node-001",
    "message_type": "agent_descriptor",
    "version": "2.0"
  },
  "body": {
    "agent": {
      "id": "edgeCore-node-001",
      "kind": "device",
      "version": "2.0.0",
      "status": "online",
      "transport": "mqtt",
      "heartbeat_interval_sec": 30,
      "endpoint": {
        "host": "192.168.1.100",
        "port": 8082
      },
      "metadata": {
        "os": "linux",
        "arch": "arm64",
        "hostname": "edgeCore-node-001.local",
        "model": "edgeCore-gateway-pro"
      }
    }
  }
}
```

---

## 1.3 Capability Model

### 1.3.1 Capability 定义

Capability 是 EAN 2.0 协议唯一的能力模型。以后 MCP Tool、HTTP API、SDK 函数、Workflow Node 全部映射自 Capability。

```yaml
capability:
  id: "modbus_tcp.read_point"   # 全局唯一标识
  agent_id: "edgeCore-node-001"               # 所属 Agent
  description: "读取 Modbus TCP 保持寄存器"
  category: "device"                       # device | ai | workflow | system
  input_schema: {}                         # JSON Schema 输入参数定义
  output_schema: {}                        # JSON Schema 输出结果定义
  timeout_sec: 10                          # 默认超时（秒）
  permission: "read"                       # read | write | readwrite | admin
  metadata: {}                             # 扩展元数据
```

### 1.3.2 Capability Descriptor 消息格式

**Topic**: `$edgeos/discovery/capability`

```json
{
  "header": {
    "message_id": "msg-cap-desc-001",
    "timestamp": 1776787200000,
    "source": "edgeCore-node-001",
    "message_type": "capability_descriptor",
    "version": "2.0"
  },
  "body": {
    "capabilities": [
      {
        "id": "modbus_tcp.read_point",
        "agent_id": "edgeCore-node-001",
        "description": "读取 Modbus TCP 保持寄存器",
        "category": "device",
        "input_schema": {
          "type": "object",
          "properties": {
            "device_id": {"type": "string"},
            "address": {"type": "string"},
            "quantity": {"type": "integer", "default": 1}
          },
          "required": ["device_id", "address"]
        },
        "output_schema": {
          "type": "object",
          "properties": {
            "values": {"type": "array"},
            "timestamp": {"type": "integer"}
          }
        },
        "timeout_sec": 10,
        "permission": "read"
      },
      {
        "id": "modbus_tcp.write_point",
        "agent_id": "edgeCore-node-001",
        "description": "写入 Modbus TCP 寄存器",
        "category": "device",
        "input_schema": {
          "type": "object",
          "properties": {
            "device_id": {"type": "string"},
            "address": {"type": "string"},
            "value": {"type": "number"}
          },
          "required": ["device_id", "address", "value"]
        },
        "timeout_sec": 10,
        "permission": "write"
      }
    ]
  }
}
```

### 1.3.3 自动生成的 Capability 命名规范

edgeCore Capability Runtime 中，Capability 由 Driver / Commands 自动生成，命名遵循：

```
{protocol_id}.{command_name}

示例：
  modbus_tcp.read_point
  modbus_tcp.write_point
  bacnet.read_property
  s7.read_db
  ai.protocol_reverse             # AI 能力
  ai.doc_parse                    # AI 能力
  system.diagnostics              # 系统能力
```

---

## 1.4 Discovery

### 1.4.1 Discovery Topic 规范

| Topic/Subject | 方向 | QoS | 说明 |
|---------------|------|-----|------|
| `$edgeos/discovery/agent` | Agent → EdgeOS | 1 | Agent 注册/更新 Descriptor |
| `$edgeos/discovery/agent/offline` | Agent → EdgeOS | 1 | Agent 下线通知 |
| `$edgeos/discovery/capability` | Agent → EdgeOS | 1 | Capability 注册/更新 |
| `$edgeos/discovery/service` | Agent → EdgeOS | 1 | Service Agent 注册 |
| `$edgeos/discovery/query` | EdgeOS → Agent | 0 | 主动查询 Discovery |
| `$edgeos/discovery/response` | Agent → EdgeOS | 1 | Discovery 查询响应 |

### 1.4.2 Discovery 流程

```text
Agent 启动
    │
    ▼
发布 Agent Descriptor → $edgeos/discovery/agent
    │
    ▼
发布 Capability Descriptor → $edgeos/discovery/capability
    │
    ▼
周期性 Heartbeat → $edgeos/heartbeat/{agent_id}
    │
    ▼
EdgeOS Registry Center 维护 Agent 在线状态
    │
    ▼
EdgeOS Discovery Center 建立 Capability 索引
```

---

## 1.5 Registry

### 1.5.1 Registry 职责

统一 Capability Registry：

```text
Agent
    │
    ▼
发布 Capability Descriptor
    │
    ▼
Registry（EdgeOS 侧聚合 / Agent 侧本地缓存）
    │
    ▼
Discovery 查询 / 调度决策
```

### 1.5.2 Registry 数据模型

```json
{
  "registry": {
    "agent_id": "edgeCore-node-001",
    "last_seen": 1776787200000,
    "capabilities_count": 15,
    "capabilities": ["modbus_tcp.read_point", "modbus_tcp.write_point", ...],
    "status": "online",
    "version": "2.0.0"
  }
}
```

---

## 1.6 Invoke Protocol

### 1.6.1 Invoke 请求格式

**Topic**: `$edgeos/invoke/{target_agent_id}`

```json
{
  "header": {
    "message_id": "msg-invoke-001",
    "timestamp": 1776787200000,
    "source": "edgeos-planner-001",
    "destination": "edgeCore-node-001",
    "message_type": "invoke_capability",
    "version": "2.0",
    "correlation_id": "req-plan-001"
  },
  "body": {
    "invoke_id": "invoke-001",
    "target": "edgeCore-node-001",
    "capability": "modbus_tcp.write_point",
    "arguments": {
      "device_id": "slave-1",
      "address": "40001",
      "value": 25.5
    },
    "options": {
      "timeout_sec": 10,
      "priority": "normal",
      "retry": 2
    }
  }
}
```

### 1.6.2 Invoke 响应格式

**Topic**: `$edgeos/reply/{source_agent_id}`

```json
{
  "header": {
    "message_id": "msg-reply-001",
    "timestamp": 1776787200500,
    "source": "edgeCore-node-001",
    "destination": "edgeos-planner-001",
    "message_type": "invoke_response",
    "version": "2.0",
    "correlation_id": "req-plan-001"
  },
  "body": {
    "invoke_id": "invoke-001",
    "status": "completed",
    "result": {
      "success": true,
      "values": [{"address": "40001", "value": 25.5}],
      "timestamp": 1776787200450
    },
    "latency_ms": 120
  }
}
```

### 1.6.3 Invoke 状态机

```text
Queued → Running → Completed
   │         │
   │         ▼
   │      Failed
   │         │
   │         ▼
   │      Timeout
   ▼
Rejected（权限不足/目标离线）
```

| 状态 | 说明 |
|------|------|
| `queued` | 请求已接收，等待调度 |
| `running` | 正在执行 |
| `completed` | 执行成功 |
| `failed` | 执行失败（业务错误） |
| `timeout` | 执行超时 |
| `rejected` | 请求被拒绝（权限/离线/参数错误） |

### 1.6.4 异步 Invoke 与状态查询

支持异步模式：发送 Invoke 后通过 `invoke_id` 查询状态。

**状态查询 Topic**: `$edgeos/invoke/{target_agent_id}/status`

```json
{
  "body": {
    "invoke_id": "invoke-001"
  }
}
```

---

## 1.7 Event

### 1.7.1 Event Topic 规范

| Topic/Subject | 方向 | QoS | 说明 |
|---------------|------|-----|------|
| `$edgeos/event/{agent_id}` | Agent → EdgeOS | 1 | Agent 事件上报 |
| `$edgeos/event/{agent_id}/{device_id}` | Agent → EdgeOS | 1 | 子设备事件 |
| `$edgeos/event/broadcast` | Agent → EdgeOS | 1 | 广播事件 |
| `$edgeos/event/subscribe` | EdgeOS → Agent | 0 | 事件订阅请求 |

### 1.7.2 Event 消息格式

**Topic**: `$edgeos/event/edgeCore-node-001`

```json
{
  "header": {
    "message_id": "msg-event-001",
    "timestamp": 1776787200000,
    "source": "edgeCore-node-001",
    "message_type": "event",
    "version": "2.0"
  },
  "body": {
    "event_id": "evt-001",
    "event_type": "temperature.changed",
    "agent_id": "edgeCore-node-001",
    "device_id": "slave-1",
    "point_id": "temperature",
    "value": 45.2,
    "previous_value": 42.1,
    "timestamp": 1776787200000,
    "severity": "info",
    "metadata": {
      "quality": "good",
      "scan_class": "normal"
    }
  }
}
```

### 1.7.3 预定义 Event 类型

| Event 类型 | 说明 | 来源 |
|-----------|------|------|
| `{point_id}.changed` | 点位值变化 | edgeCore ShadowCore |
| `{point_id}.updated` | 点位更新（含相同值） | edgeCore ScanEngine |
| `device.online` | 设备上线 | edgeCore Driver |
| `device.offline` | 设备离线 | edgeCore Driver |
| `device.error` | 设备错误 | edgeCore Driver |
| `alarm.created` | 告警创建 | edgeCore EdgeRule |
| `alarm.cleared` | 告警清除 | edgeCore EdgeRule |
| `agent.heartbeat` | 心跳事件 | 所有 Agent |
| `capability.invoked` | Capability 被调用 | 所有 Agent |
| `workflow.step_completed` | 工作流步骤完成 | Workflow Agent |

---

## 1.8 Workflow

### 1.8.1 Workflow 设计原则

Workflow 只调用 Capability，不调用 Driver。

```text
Workflow Definition
    │
    ▼
Workflow Node → Capability Invoke
    │
    ▼
edgeCore Capability Runtime → Execution Mapper → Driver
```

### 1.8.2 Workflow 节点类型

| 节点类型 | 说明 | 映射到 Capability |
|---------|------|------------------|
| `action` | 执行单个 Capability | 直接映射 |
| `condition` | 条件判断 | `system.condition.evaluate` |
| `retry` | 重试包装 | 内置调度逻辑 |
| `delay` | 延迟执行 | 内置调度逻辑 |
| `timeout` | 超时包装 | 内置调度逻辑 |
| `parallel` | 并行执行 | 内置调度逻辑 |
| `event_wait` | 等待事件 | `system.event.subscribe` |

### 1.8.3 Workflow 通过 Capability 调用 edgeCore

```json
{
  "workflow_step": {
    "step_id": "step-001",
    "node_type": "action",
    "capability": "modbus_tcp.read_point",
    "target_agent": "edgeCore-node-001",
    "arguments": {
      "device_id": "slave-1",
      "address": "40001",
      "quantity": 1
    },
    "on_success": "step-002",
    "on_failure": "step-error"
  }
}
```

---

## 1.9 QoS

### 1.9.1 统一 QoS 模型

| 维度 | 说明 | 取值 |
|------|------|------|
| `priority` | 调度优先级 | `critical` > `high` > `normal` > `low` |
| `timeout_sec` | 执行超时 | 正整数，默认 10s |
| `retry` | 重试次数 | 0～5，默认 0 |
| `exclusive` | 独占执行 | `true` / `false` |
| `queue` | 队列策略 | `fifo` / `priority` / `drop_oldest` |

### 1.9.2 资源锁

Execution 必须支持资源锁，防止并发冲突：

```json
{
  "options": {
    "resource_locks": [
      {"resource": "plc-001", "scope": "device"},
      {"resource": "com3", "scope": "channel"}
    ]
  }
}
```

---

## 1.10 Shadow

### 1.10.1 Shadow 统一状态模型

Capability 默认读取 Shadow。需要实时数据时，Execution Mapper 访问 Driver。

```json
{
  "shadow": {
    "agent_id": "edgeCore-node-001",
    "device_id": "slave-1",
    "points": {
      "temperature": {
        "value": 45.2,
        "timestamp": 1776787200000,
        "quality": "good",
        "source": "scan"
      },
      "pressure": {
        "value": 101325,
        "timestamp": 1776787200000,
        "quality": "good",
        "source": "scan"
      }
    },
    "device_status": "online",
    "last_updated": 1776787200000
  }
}
```

### 1.10.2 Shadow Topic

| Topic/Subject | 方向 | 说明 |
|---------------|------|------|
| `$edgeos/state/{agent_id}` | Agent → EdgeOS | Shadow 全量上报 |
| `$edgeos/state/{agent_id}/delta` | Agent → EdgeOS | Shadow 增量更新 |
| `$edgeos/state/{agent_id}/get` | EdgeOS → Agent | 请求 Shadow 快照 |

---

# 第二章 edgeCore Capability Runtime

> **目标：尽量少改动，复用现有 AI 与 MCP。**  
> 新增的是 **Capability Runtime**，不是新的 Runtime。

---

## 2.1 Capability Registry（新增）

### 2.1.1 自动生成 Capability

已有 Driver / Commands 自动生成 Capability，无需人工维护。

```text
Driver
    │
    ▼
Commands（读/写/扫描）
    │
    ▼
Capability（自动映射）
    │
    ▼
Registry（本地缓存 + 发布到 EdgeOS）
```

### 2.1.2 自动生成规则

| Driver 命令 | 自动生成 Capability ID | 参数映射 |
|------------|----------------------|---------|
| `ReadPoints` | `{protocol}.read_{register_type}` | device_id, addresses[] |
| `WritePoint` | `{protocol}.write_{register_type}` | device_id, address, value |
| `ScanDevices` | `{protocol}.scan_devices` | channel_id, network |
| `GetDevicePoints` | `{protocol}.list_points` | device_id |
| `Diagnostics` | `system.diagnostics` | — |

---

## 2.2 AI Adapter（升级现有 AI）

### 2.2.1 当前架构（V1.5）

```text
AI
    │
    ▼
MCP Tool
```

### 2.2.2 升级架构（V2.0）

```text
AI
    │
    ▼
Capability Planner（新增）
    │
    ▼
Capability Invoke（统一入口）
    │
    ▼
Invoke Dispatcher
    │
    ▼
Execution Mapper
```

AI 负责规划（Planning），Execution 继续由 Execution Mapper 执行。

---

## 2.3 MCP Adapter（升级现有 MCP）

### 2.3.1 当前架构（V1.5）

```text
MCP Tool
    │
    ▼
Command（直接调用）
```

### 2.3.2 升级架构（V2.0）

```text
Capability（统一能力模型）
    │
    ▼
Tool（自动生成 MCP Tool）
    │
    ▼
MCP Server
```

Tool 由 Capability 自动生成，无需人工维护 Tool 清单。

---

## 2.4 Invoke Dispatcher（新增）

### 2.4.1 统一入口

所有入口统一为 `Capability Invoke`：

```text
MQTT    HTTP    SDK    MCP    Workflow
    \      |      /      /      /
     \     |     /      /      /
      \    |    /      /      /
       \   |   /      /      /
        \  |  /      /      /
      Invoke Dispatcher（新增）
             │
             ▼
      Capability Registry
             │
             ▼
      Execution Mapper
             │
             ▼
         ShadowCore
             │
             ▼
         ScanEngine
             │
             ▼
          Driver
```

### 2.4.2 Dispatcher 路由逻辑

```go
// 伪代码
func Dispatch(invoke InvokeRequest) {
    capability := registry.Get(invoke.Capability)
    
    switch capability.Category {
    case "device":
        executionMapper.ExecuteDriverCommand(invoke)
    case "ai":
        aiAdapter.Execute(invoke)
    case "system":
        systemHandler.Execute(invoke)
    }
}
```

---

## 2.5 Execution Mapper（少量升级）

### 2.5.1 新增 Capability → Driver Command 映射

```text
Capability: modbus_tcp.read_point
    │
    ▼
Execution Mapper 解析 arguments
    │
    ▼
Driver Command: ReadPoints(device_id, addresses, function_code=3)
    │
    ▼
ScanEngine → Driver
```

无需修改驱动实现，仅在 Execution Mapper 增加 Capability 到 Driver Command 的映射层。

---

## 2.6 ShadowCore（保持）

### 2.6.1 增加 Capability 状态缓存

ShadowCore 继续维护设备状态，同时支持按 Capability ID 查询：

```json
{
  "capability_cache": {
    "modbus_tcp.read_point": {
      "device_slave_1": {
        "last_result": [...],
        "last_updated": 1776787200000
      }
    }
  }
}
```

Execution 优先读取 Shadow，减少 Driver 调用。

---

## 2.7 Event Publisher（新增）

### 2.7.1 Capability 自动发布 Event

当 Capability 执行导致设备状态变化时，自动发布 Event：

```text
Capability: modbus_tcp.write_point
    │
    ▼
写入成功
    │
    ▼
ShadowCore 更新
    │
    ▼
Event Publisher 发布事件
    │
    ▼
$edgeos/event/edgeCore-node-001
```

### 2.7.2 自动 Event 类型映射

| 操作 | 自动发布 Event |
|------|---------------|
| 点位值变化 | `{point_id}.changed` |
| 设备上线 | `device.online` |
| 设备离线 | `device.offline` |
| 告警触发 | `alarm.created` |
| Capability 执行完成 | `capability.invoked` |

---

## 2.8 Discovery Publisher（新增）

### 2.8.1 启动时发布 Agent Descriptor

```text
edgeCore 启动
    │
    ▼
初始化 ChannelManager / ScanEngine
    │
    ▼
DiscoveryPublisher 收集 Agent 信息
    │
    ▼
发布 Agent Descriptor → $edgeos/discovery/agent
    │
    ▼
发布 Capability Descriptor → $edgeos/discovery/capability
    │
    ▼
启动 Heartbeat 定时器
```

### 2.8.2 关闭时发布 Offline

```text
edgeCore 关闭（Graceful Shutdown）
    │
    ▼
DiscoveryPublisher 发布 Offline
    │
    ▼
$edgeos/discovery/agent/offline
```

---

## 2.9 MQTT/NATS Transport（少量升级）

### 2.9.1 新增 EAN 2.0 Topic

保留所有 V1.0 Topic 作为兼容层（见附录 A），新增以下 EAN 2.0 Topic：

| Topic/Subject | 方向 | QoS | 说明 |
|---------------|------|-----|------|
| `$edgeos/discovery/agent` | edgeCore → EdgeOS | 1 | Agent 注册/更新 |
| `$edgeos/discovery/agent/offline` | edgeCore → EdgeOS | 1 | Agent 下线 |
| `$edgeos/discovery/capability` | edgeCore → EdgeOS | 1 | Capability 注册 |
| `$edgeos/discovery/query` | EdgeOS → edgeCore | 0 | Discovery 查询 |
| `$edgeos/discovery/response` | edgeCore → EdgeOS | 1 | Discovery 响应 |
| `$edgeos/invoke/{agent_id}` | EdgeOS → edgeCore | 1 | Capability 调用请求 |
| `$edgeos/reply/{agent_id}` | edgeCore → EdgeOS | 1 | Capability 调用响应 |
| `$edgeos/invoke/{agent_id}/status` | EdgeOS → edgeCore | 0 | 查询 Invoke 状态 |
| `$edgeos/event/{agent_id}` | edgeCore → EdgeOS | 1 | 事件上报 |
| `$edgeos/event/broadcast` | edgeCore → EdgeOS | 1 | 广播事件 |
| `$edgeos/state/{agent_id}` | edgeCore → EdgeOS | 1 | Shadow 全量上报 |
| `$edgeos/state/{agent_id}/delta` | edgeCore → EdgeOS | 1 | Shadow 增量更新 |
| `$edgeos/state/{agent_id}/get` | EdgeOS → edgeCore | 0 | Shadow 查询请求 |
| `$edgeos/heartbeat/{agent_id}` | edgeCore → EdgeOS | 0 | 心跳 |

### 2.9.2 Transport 复用

继续复用现有 MQTT/NATS 北向通道配置，仅需订阅/发布新增 Topic。

---

## 2.10 Capability SDK（新增）

### 2.10.1 统一 Capability SDK

SDK 面向 Capability 编程，而非面向 Driver 编程：

```go
// 调用 Capability（不直接调用 Driver）
result, err := sdk.InvokeCapability(ctx, InvokeRequest{
    Target:     "edgeCore-node-001",
    Capability: "modbus_tcp.read_point",
    Arguments: map[string]interface{}{
        "device_id": "slave-1",
        "address":   "40001",
    },
})
```

---

# 第三章 EdgeOS Coordination Platform

> EdgeOS 不负责执行，负责平台治理。

---

## 3.1 Registry Center

维护所有 Agent：

```text
Online Agents
    ├── Agent ID
    ├── Kind
    ├── Version
    ├── Heartbeat Timestamp
    ├── Capabilities[]
    └── Metadata

Offline Agents
    └── Last Seen + Offline Reason
```

---

## 3.2 Discovery Center

缓存所有 Capability，支持查询：

| 查询方式 | 说明 |
|---------|------|
| 按 Agent ID | 查询某 Agent 的所有 Capability |
| 按 Capability ID | 全局搜索 Capability |
| 按 Category | 按类型筛选（device/ai/workflow） |
| 按 Keyword | 模糊匹配描述 |
| 按 Permission | 按权限筛选 |

---

## 3.3 Workflow Center

Workflow 编排只调用 Capability：

```text
用户：关闭所有空调
    │
    ▼
AI Planner → Capability List
    │
    ▼
Workflow Center 生成执行计划
    │
    ▼
Scheduler 调度 Capability Invoke
    │
    ▼
edgeCore Capability Runtime 执行
```

---

## 3.4 AI Planner

AI 负责 Capability 规划：

```text
自然语言需求
    │
    ▼
AI Planner 解析意图
    │
    ▼
Discovery Center 查询可用 Capability
    │
    ▼
生成 Capability 执行计划
    │
    ▼
Workflow Center / Scheduler 执行
```

---

## 3.5 Scheduler

统一调度参数：

| 维度 | 说明 |
|------|------|
| `priority` | `critical` / `high` / `normal` / `low` |
| `retry` | 失败重试次数与退避策略 |
| `timeout` | 全局超时控制 |
| `queue` | 队列长度与丢弃策略 |

---

## 3.6 Resource Manager

工业资源锁管理：

| 资源类型 | 示例 | 锁粒度 |
|---------|------|--------|
| PLC | PLC-001 | 设备级 |
| COM 口 | COM3 | 通道级 |
| Camera | CAM-001 | 设备级 |
| Robot | ROBOT-001 | 设备级 |

---

## 3.7 Event Center

统一 Event 管理：

```text
Publish    Subscribe    History    Replay
```

Workflow 可直接订阅 Event 触发步骤。

---

## 3.8 Security

| 安全维度 | 说明 |
|---------|------|
| Capability ACL | 按 Capability ID 控制调用权限 |
| Agent ACL | 按 Agent ID 控制通信权限 |
| Namespace | 多租户隔离 |
| Token | JWT / API Key 认证 |

---

## 3.9 Metrics

统一监控指标：

| 指标 | 说明 |
|------|------|
| `invoke_total` | Capability 调用总数 |
| `invoke_latency_ms` | 调用延迟分布 |
| `agent_busy` | Agent 忙碌状态 |
| `agent_offline` | Agent 离线次数 |
| `invoke_timeout` | 超时次数 |
| `invoke_failure` | 失败次数 |

---

## 3.10 Cluster

支持多个 EdgeOS 节点：

```text
EdgeOS Node A          EdgeOS Node B
    │                      │
    ▼                      ▼
Registry Sync  ←────→  Registry Sync
Discovery Sync ←────→  Discovery Sync
Workflow Sync  ←────→  Workflow Sync
```

---

# edgeCore 与 EdgeOS 职责边界（EAN 2.0）

| 模块 | edgeCore（执行层） | EdgeOS（平台层） |
|------|---------------|----------------|
| Agent 生命周期 | Agent 注册、上线、心跳、下线 | 全局 Agent 管理与查询 |
| Capability | 自动生成、注册、执行 | 聚合、检索、跨节点发现 |
| Discovery | 发布自身 Descriptor | 建立全局 Discovery 索引 |
| Invoke | 接收请求并执行 | 发起跨 Agent 调用、编排 |
| Execution | Execution Mapper、ScanEngine、Driver | 不直接执行设备操作 |
| Shadow | ShadowCore 状态维护 | 聚合状态、跨节点同步（可选） |
| Workflow | 提供 Capability 节点 | 负责流程编排与调度 |
| AI | MCP、Planner、Tool Adapter | 多 Agent 任务规划 |
| Event | 发布设备事件 | 订阅、路由、规则处理 |
| MQTT/NATS | 通信接入与协议实现 | 消息治理、集群协调 |
| Security | Capability 权限校验 | 全局认证、授权、命名空间 |
| Observability | 本地运行指标 | 全局监控、告警、审计 |

---

# 附录 A：V1.0 Topic 兼容层

> 以下为 edgeCore ↔ EdgeOS V1.0 通信协议 Topic，EAN 2.0 中继续保留作为兼容层。新开发建议优先使用 EAN 2.0 Topic（`$edgeos/*`）。

## A.1 MQTT Topic 命名规则（V1.0）

```
edgeCore/{layer}/{category}[/{node_id}[/{device_id}[/{point_id}]]]
```

## A.2 V1.0 Topic 列表

### A.2.1 节点管理 Topics

| Topic | 方向 | QoS | 说明 |
|-------|------|-----|------|
| `edgeCore/nodes/register` | edgeCore → EdgeOS | 1 | 节点注册 |
| `edgeCore/nodes/unregister` | edgeCore → EdgeOS | 1 | 节点注销 |
| `edgeCore/nodes/{node_id}/status` | edgeCore → EdgeOS | 1 | 节点状态更新 |
| `edgeCore/nodes/{node_id}/online` | edgeCore → EdgeOS | 2 | 节点上线上报 |
| `edgeCore/nodes/{node_id}/offline` | edgeCore → EdgeOS | 2 | 节点离线上报 |
| `edgeCore/heartbeat/{node_id}` | edgeCore → EdgeOS | 0 | 节点心跳（丰富版） |

### A.2.2 设备管理 Topics

| Topic | 方向 | QoS | 说明 |
|-------|------|-----|------|
| `edgeCore/devices/report` | edgeCore → EdgeOS | 1 | 设备信息上报 |
| `edgeCore/devices/{node_id}/list` | EdgeOS → edgeCore | 0 | 查询设备列表 |
| `edgeCore/devices/{node_id}/{device_id}/info` | EdgeOS → edgeCore | 0 | 查询设备详情 |
| `edgeCore/devices/{node_id}/{device_id}/bind` | EdgeOS → edgeCore | 1 | 绑定设备 |
| `edgeCore/devices/{node_id}/{device_id}/unbind` | EdgeOS → edgeCore | 1 | 解绑设备 |
| `edgeCore/devices/{node_id}/{device_id}/online` | edgeCore → EdgeOS | 2 | 子设备上线上报 |
| `edgeCore/devices/{node_id}/{device_id}/offline` | edgeCore → EdgeOS | 2 | 子设备离线上报 |

### A.2.3 点位管理 Topics

| Topic | 方向 | QoS | 说明 |
|-------|------|-----|------|
| `edgeCore/points/report` | edgeCore → EdgeOS | 1 | 点位信息上报 |
| `edgeCore/points/{node_id}/{device_id}` | edgeCore → EdgeOS | 1 | 点位全量数据同步 |
| `edgeCore/points/{node_id}/{device_id}/list` | EdgeOS → edgeCore | 0 | 查询点位列表 |
| `edgeCore/points/{node_id}/{device_id}/sync` | EdgeOS → edgeCore | 1 | 同步点位数据 |

### A.2.4 数据采集 Topics

| Topic | 方向 | QoS | 说明 |
|-------|------|-----|------|
| `edgeCore/data/{node_id}/{device_id}` | edgeCore → EdgeOS | 0 | 设备实时数据 |
| `edgeCore/data/{node_id}/{device_id}/batch` | edgeCore → EdgeOS | 1 | 批量数据上报 |
| `edgeCore/data/{node_id}/{device_id}/{point_id}` | edgeCore → EdgeOS | 0 | 单点位数据 |

### A.2.5 控制命令 Topics（EdgeOS 发布 → edgeCore 订阅）

| Topic | 方向 | QoS | 说明 |
|-------|------|-----|------|
| `edgeCore/cmd/nodes/register` | EdgeOS → edgeCore | 1 | 触发节点重新注册 |
| `edgeCore/cmd/{node_id}/discover` | EdgeOS → edgeCore | 0 | 设备发现命令 |
| `edgeCore/cmd/{node_id}/task/create` | EdgeOS → edgeCore | 1 | 创建任务 |
| `edgeCore/cmd/{node_id}/task/{task_id}/pause` | EdgeOS → edgeCore | 1 | 暂停任务 |
| `edgeCore/cmd/{node_id}/task/{task_id}/resume` | EdgeOS → edgeCore | 1 | 恢复任务 |
| `edgeCore/cmd/{node_id}/task/{task_id}/stop` | EdgeOS → edgeCore | 1 | 停止任务 |
| `edgeCore/cmd/{node_id}/{device_id}/write` | EdgeOS → edgeCore | 1 | 写入数据 |
| `edgeCore/cmd/{node_id}/config/update` | EdgeOS → edgeCore | 1 | 更新配置 |

### A.2.6 事件告警 Topics

| Topic | 方向 | QoS | 说明 |
|-------|------|-----|------|
| `edgeCore/events/alert` | edgeCore → EdgeOS | 2 | 告警消息 |
| `edgeCore/events/error` | edgeCore → EdgeOS | 1 | 错误消息 |
| `edgeCore/events/info` | edgeCore → EdgeOS | 0 | 信息消息 |

### A.2.7 响应 Topics

| Topic | 方向 | QoS | 说明 |
|-------|------|-----|------|
| `edgeCore/cmd/responses/{node_id}/{device_id}` | edgeCore → EdgeOS | 1 | 命令响应 |

## A.3 NATS Subject 规范（V1.0）

```
edgeCore.{layer}.{category}.{node_id}.{device_id}.{point_id}
```

| 通配符 | 说明 |
|--------|------|
| `*` | 匹配单个 token |
| `>` | 匹配一个或多个 tokens |

### A.3.1 节点管理 Subjects

| Subject | 方向 | 说明 |
|---------|------|------|
| `edgeCore.nodes.register` | edgeCore → EdgeOS | 节点注册 |
| `edgeCore.nodes.unregister` | edgeCore → EdgeOS | 节点注销 |
| `edgeCore.nodes.heartbeat.>` | edgeCore → EdgeOS | 节点心跳 |
| `edgeCore.nodes.status.>` | edgeCore → EdgeOS | 节点状态 |
| `edgeCore.cmd.nodes.register` | EdgeOS → edgeCore | 触发节点重新注册 |
| `edgeCore.cmd.>.discover` | EdgeOS → edgeCore | 设备发现 |

### A.3.2 设备管理 Subjects

| Subject | 方向 | 说明 |
|---------|------|------|
| `edgeCore.devices.report` | edgeCore → EdgeOS | 设备上报 |
| `edgeCore.devices.>.list` | EdgeOS → edgeCore | 查询设备 |
| `edgeCore.devices.>.info.>` | EdgeOS → edgeCore | 设备详情 |
| `edgeCore.devices.>.online` | edgeCore → EdgeOS | 子设备上线 |
| `edgeCore.devices.>.offline` | edgeCore → EdgeOS | 子设备下线 |

### A.3.3 数据采集 Subjects

| Subject | 方向 | 说明 |
|---------|------|------|
| `edgeCore.data.>.>` | edgeCore → EdgeOS | 实时数据 |
| `edgeCore.data.>.batch` | edgeCore → EdgeOS | 批量数据 |

### A.3.4 请求/响应 Subjects

| Subject | 类型 | 说明 |
|---------|------|------|
| `edgeCore.req.>` | Request | 请求消息 |
| `edgeCore.res.>` | Response | 响应消息 |

---

# 附录 B：V1.0 消息格式参考

> V1.0 消息格式在 EAN 2.0 中继续兼容使用。以下为主要消息类型示例。

## B.1 通用消息头

```json
{
  "message_id": "msg-001",
  "timestamp": 1744680000000,
  "source": "edgeCore-node-001",
  "destination": "edgeos-queen",
  "message_type": "node_register",
  "version": "1.0"
}
```

## B.2 节点注册消息

**Topic**: `edgeCore/nodes/register`

```json
{
  "header": {
    "message_id": "msg-node-reg-001",
    "timestamp": 1744680000000,
    "source": "edgeCore-node-001",
    "destination": "edgeos-queen",
    "message_type": "node_register",
    "version": "1.0"
  },
  "body": {
    "node_id": "edgeCore-node-001",
    "node_name": "edgeCore Gateway Node",
    "model": "edgeCore",
    "version": "1.0.0",
    "api_version": "v1",
    "capabilities": ["shadow-sync", "heartbeat", "device-control", "task-execution"],
    "protocol": "edgeOS(MQTT)",
    "endpoint": {
      "host": "127.0.0.1",
      "port": 8082
    },
    "metadata": {
      "os": "linux",
      "arch": "amd64",
      "hostname": "edgeCore-node-001.local"
    }
  }
}
```

## B.3 设备上报消息

**Topic**: `edgeCore/devices/report`

```json
{
  "header": {
    "message_id": "msg-dev-report-001",
    "timestamp": 1744680000000,
    "source": "edgeCore-node-001",
    "message_type": "device_report",
    "version": "1.0"
  },
  "body": {
    "node_id": "edgeCore-node-001",
    "devices": [
      {
        "device_id": "device-001",
        "device_name": "Modbus TCP Device",
        "device_profile": "modbus-tcp-device",
        "service_name": "modbus-tcp-service",
        "labels": ["sensor", "modbus"],
        "description": "Test Modbus TCP device",
        "admin_state": "ENABLED",
        "operating_state": "ENABLED",
        "properties": {
          "protocol": "modbus-tcp",
          "address": "192.168.1.100:502",
          "unit_id": 1
        }
      }
    ]
  }
}
```

## B.4 实时数据消息

**Topic**: `edgeCore/data/{node_id}/{device_id}`

```json
{
  "header": {
    "message_id": "msg-data-001",
    "timestamp": 1744680000000,
    "source": "edgeCore-node-001",
    "message_type": "data",
    "version": "1.0"
  },
  "body": {
    "node_id": "edgeCore-node-001",
    "device_id": "device-001",
    "timestamp": 1744680000000,
    "points": {
      "Temperature": 25.5,
      "Humidity": 65.2,
      "Pressure": 101325,
      "Switch": true
    },
    "quality": "good"
  }
}
```

## B.5 心跳消息

**Topic**: `edgeCore/heartbeat/{node_id}`

```json
{
  "header": {
    "message_id": "msg-hb-001",
    "timestamp": 1744680000000,
    "source": "edgeCore-node-001",
    "message_type": "heartbeat",
    "version": "1.0"
  },
  "body": {
    "node_id": "edgeCore-node-001",
    "status": "active",
    "timestamp": 1744680000000,
    "sequence": 100,
    "uptime_seconds": 3600,
    "version": "1.0.0",
    "system_metrics": {
      "cpu_usage": 25.5,
      "memory_usage": 45.2,
      "memory_total": 8589934592,
      "memory_used": 3883921408,
      "disk_usage": 32.1,
      "disk_total": 107374182400,
      "disk_used": 34426873856,
      "load_average": 0.85,
      "network_rx_bytes": 1024000,
      "network_tx_bytes": 512000,
      "process_count": 45,
      "thread_count": 128
    },
    "device_summary": {
      "total_count": 10,
      "online_count": 8,
      "offline_count": 1,
      "error_count": 1,
      "degraded_count": 0,
      "recovering_count": 0
    },
    "channel_summary": {
      "total_count": 3,
      "connected_count": 3,
      "error_count": 0,
      "avg_success_rate": 0.985
    },
    "task_summary": {
      "total_count": 5,
      "running_count": 5,
      "paused_count": 0,
      "error_count": 0
    },
    "connection_stats": {
      "reconnect_count": 2,
      "last_online_time": 1744676400000,
      "last_offline_time": 1744672800000,
      "connected_since": 1744676400000,
      "publish_count": 15000,
      "protocol_version": "MQTTv3.1.1"
    }
  }
}
```

## B.6 写入命令

**Topic**: `edgeCore/cmd/{node_id}/{device_id}/write`

```json
{
  "header": {
    "message_id": "msg-cmd-write-001",
    "timestamp": 1744680000000,
    "source": "edgeos-queen",
    "destination": "edgeCore-node-001",
    "message_type": "write_command",
    "version": "1.0",
    "correlation_id": "req-write-001"
  },
  "body": {
    "request_id": "req-write-001",
    "device_id": "device-001",
    "timestamp": 1744680000000,
    "points": {
      "Switch": true,
      "Setpoint": 80.5
    },
    "options": {
      "confirm": true,
      "timeout_seconds": 10
    }
  }
}
```

## B.7 告警消息

**Topic**: `edgeCore/events/alert`

```json
{
  "header": {
    "message_id": "msg-alert-001",
    "timestamp": 1744680000000,
    "source": "edgeCore-node-001",
    "message_type": "alert",
    "version": "1.0"
  },
  "body": {
    "node_id": "edgeCore-node-001",
    "device_id": "device-001",
    "alert_id": "alert-001",
    "alert_type": "device_offline",
    "severity": "critical",
    "message": "Device device-001 went offline",
    "timestamp": 1744680000000,
    "details": {
      "last_seen": "2026-04-15T16:00:00Z",
      "retry_count": 3,
      "error": "Connection timeout"
    }
  }
}
```

---

# 附录 C：连接配置参考

## C.1 MQTT 连接配置

```yaml
mqtt:
  broker: "tcp://127.0.0.1:1883"
  client_id: "edgeCore-node-001"
  username: "edgeCore"
  password: "edgeCore-secret"
  qos: 1
  retain: false
  clean_session: true
  keep_alive: 60
  connect_timeout: 30
  write_timeout: 10
  read_timeout: 10
  auto_reconnect: true
  max_reconnect_interval: 300
```

## C.2 NATS 连接配置

```yaml
nats:
  url: "nats://127.0.0.1:4222"
  client_name: "edgeCore-node-001"
  username: "edgeCore"
  password: "edgeCore-secret"
  token: ""
  connect_timeout: 30
  reconnect_wait: 2
  max_reconnects: 10
  ping_interval: 20
  max_pings_outstanding: 5
  jetstream_enabled: true
```

## C.3 协议选择配置

```yaml
communication:
  protocol: "edgeOS(MQTT)"  # 或 "edgeOS(NATS)"
  mqtt_config:
    broker: "tcp://127.0.0.1:1883"
  nats_config:
    url: "nats://127.0.0.1:4222"
```

---

# 附录 D：QoS 和可靠性

## D.1 MQTT QoS 级别

| QoS | 含义 | 使用场景 | 性能影响 |
|-----|------|---------|---------|
| 0 | 最多一次 | 实时数据、心跳消息 | 最低 |
| 1 | 至少一次 | 设备上报、命令控制 | 中等 |
| 2 | 恰好一次 | 告警消息、重要状态 | 最高 |

## D.2 NATS 可靠性机制

| 机制 | 说明 | 配置 |
|------|------|------|
| ACK | 消息确认 | 默认开启 |
| JetStream | 消息持久化 | 可选 |
| Replication | 消息复制 | 可选 |
| Durable Subscriptions | 持久化订阅 | 可选 |

## D.3 重试策略

```yaml
retry:
  max_attempts: 3
  initial_interval: 1000  # 毫秒
  max_interval: 30000     # 毫秒
  multiplier: 2
  backoff_factor: 0.2
```

---

# 附录 E：安全性

## E.1 MQTT 安全

| 机制 | 说明 |
|------|------|
| TLS/SSL | 加密通信 |
| Username/Password | 基本认证 |
| Client Certificates | 双向认证 |
| ACL | 访问控制列表 |

## E.2 NATS 安全

| 机制 | 说明 |
|------|------|
| TLS | 加密通信 |
| User Authentication | 用户认证 |
| Account | 多租户隔离 |
| Permissions | 权限控制 |

---

# 附录 F：错误码

| 错误码 | 说明 | 处理建议 |
|--------|------|---------|
| `E001` | 消息格式错误 | 检查 JSON 格式 |
| `E002` | 消息类型不支持 | 检查消息类型 |
| `E003` | 节点未注册 | 先执行节点注册 |
| `E004` | 设备不存在 | 检查设备 ID |
| `E005` | 认证失败 | 检查凭证 |
| `E006` | 权限不足 | 检查权限配置 |
| `E007` | 超时 | 重试或增加超时时间 |
| `E008` | 重复消息 | 检查 message_id |
| `E009` | Capability 不存在 | 检查 Capability ID |
| `E010` | Agent 离线 | 检查 Agent 状态 |
| `E011` | 资源被锁定 | 等待或释放资源锁 |
| `E012` | 参数校验失败 | 检查输入参数 Schema |

---

# 附录 G：版本兼容性

| edgeOS 版本 | 协议版本 | 支持中间件 | EAN 版本 | 状态 |
|------------|---------|----------|---------|------|
| v1.0 | v1.0 | MQTT 3.1.1/5.0, NATS 2.x | — | 已发布 |
| **v2.0** | **v2.0** | **MQTT 5.0, NATS 2.x+** | **EAN 2.0** | **当前** |

---

**文档版本**: v2.0  
**最后更新**: 2026-07-27  
**维护者**: edgeOS 团队
