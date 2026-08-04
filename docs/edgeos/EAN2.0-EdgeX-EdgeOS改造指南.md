# EAN 2.0 EdgeX ↔ EdgeOS 改造指南

> **文档版本**: 1.12  
> **日期**: 2026-08-04  
> **适用范围**: EdgeX Capability Runtime（本仓库）与 EdgeOS Coordination Platform（对端必须实现）  
> **协议基线**: [EdgeX通信协议规范(MQTT-NATS).md](./EdgeX通信协议规范(MQTT-NATS).md)
> **规划基线**: [AI协同组件规划.md](./AI协同组件规划.md)
> **迁移评估**: [V1-to-EAN-Migration-Assessment.md](../TODO/V1-to-EAN-Migration-Assessment.md)

---

## 1. 背景与目标

### 1.1 背景

Edge Agent Network（EAN）2.0 在现有 EdgeX + EdgeOS 架构上增加统一 Agent 协作层：

- **EdgeX**：Capability Runtime（能力注册、发现发布、Invoke 执行、Event 上报）
- **EdgeOS**：Coordination Platform（全局发现索引、跨节点编排、Invoke 发起、Event 订阅与规则）

协议层统一为：`Agent` / `Capability` / `Discovery` / `Invoke` / `Event`，传输同时支持 **MQTT** 与 **NATS**，Topic/Subject 使用相同的 `$edgeos/...` 字符串形式。

### 1.2 本轮目标（已落地）

| 目标 | 状态 |
|------|------|
| Capability Runtime + MQTT/NATS Bridge | ✅ |
| DriverExecutor（读/写/扫描/诊断） | ✅ |
| MCP Capability → Tool Adapter | ✅ |
| Shadow → EAN Event（含 `previous_value`） | ✅ |
| AI Adapter（`ai.protocol_reverse` / `ai.doc_parse`） | ✅ |
| NATS 与 MQTT 对称 + 本机联调 | ✅ |
| EAN 启停并入 EdgeOS 通道（`EANEnabled` / 热更新） | ✅ |
| V1 `device_report` 连接/重注册兜底 | ✅ |
| EdgeOS 必做功能清单（本文档） | ✅ |

### 1.3 非目标

- 不替换 V1.0 `edgex/*`（MQTT）与 `edgex.*`（NATS）兼容层
- AI 产出仍须 Human-in-the-loop 确认后落库（禁止自动写 config）
- AI Invoke 不得进入 ScanEngine / Pipeline Worker 热路径

---

## 2. EdgeX 侧已实现能力总览

### 2.1 模块地图

```text
MQTT/NATS/MCP/HTTP
        │
        ▼
 Capability Runtime          internal/capability/
   ├─ Registry               registry.go / generator.go
   ├─ Invoke Dispatcher      dispatcher.go
   ├─ Discovery Publisher    discovery_publisher.go
   └─ Event Publisher        event_publisher.go
        │
        ▼
 CapabilityMapper            internal/execution/capability_mapper.go
        │
        ├─ DriverExecutor    internal/execution/driver_executor.go
        │     └─ Southbound (Shadow / Driver / Scan)
        └─ AIAdapter         internal/execution/ai_adapter.go
              └─ ai_agent.Agent（协议逆向 / 文档解析流水线）
        │
        ▼
 ShadowCore (COW)            internal/core/shadow_*.go
        │ notify(delta + previous_value)
        ▼
 ShadowEventBridge           internal/capability/shadow_events.go
        │
        ▼
 MQTT / NATS Event topics
```

### 2.2 关键代码路径

| 能力 | 路径 |
|------|------|
| Runtime / Topics / Message | `internal/capability/` |
| MQTT Bridge | `internal/northbound/edgos_mqtt/ean_bridge.go` |
| NATS Bridge | `internal/northbound/edgos_nats/ean_bridge.go` |
| Driver + AI 执行 | `internal/execution/` |
| MCP Tool 自动生成 | `internal/mcp/capability_adapter.go` |
| Shadow→Event 绑定 | `internal/core/northbound_manager_ext.go` |

> Shadow→Event 的 `EventPublisher` 列表在 **EAN Runtime 启停/连接生命周期**刷新（`SetOnEANRuntimeChanged` → `refreshEANEventPublishers`），**不**在每次 Shadow delta 热路径上刷新。通知回调同步执行；若北向管理器正在持写锁则延迟阻塞刷新，避免死锁并消除异步窗口内仍用旧 publisher 列表的竞态。

### 2.3 EAN Topic / Subject（MQTT 与 NATS 相同字符串）

| 用途 | Topic/Subject | 方向 | QoS(MQTT) |
|------|---------------|------|-----------|
| Agent 发现 | `$edgeos/discovery/agent` | EdgeX → EdgeOS | 1 |
| Agent 下线 | `$edgeos/discovery/agent/offline` | EdgeX → EdgeOS | 1 |
| Capability 发现 | `$edgeos/discovery/capability` | EdgeX → EdgeOS | 1 |
| 发现查询 | `$edgeos/discovery/query` | EdgeOS → EdgeX | 0 |
| 发现响应 | `$edgeos/discovery/response` | EdgeX → EdgeOS | 0 |
| Invoke 请求 | `$edgeos/invoke/{agent_id}` | EdgeOS → EdgeX | 1 |
| Invoke 状态 | `$edgeos/invoke/{agent_id}/status` | EdgeX → EdgeOS | 1 |
| Invoke 回复 | `$edgeos/reply/{source_agent_id}` | EdgeX → EdgeOS | 1 |
| Event | `$edgeos/event/{agent_id}` | EdgeX → EdgeOS | 1 |
| Event（设备） | `$edgeos/event/{agent_id}/{device_id}` | EdgeX → EdgeOS | 1 |
| Event 广播 | `$edgeos/event/broadcast` | EdgeX → EdgeOS | 1 |
| Heartbeat | `$edgeos/heartbeat/{agent_id}` | EdgeX → EdgeOS | 0 |

> NATS 侧 **不** 把 `/` 改成 `.`：EAN 2.0 统一使用 `$edgeos/...` 斜杠形式；V1.0 NATS 仍用 `edgex.*`。

### 2.4 默认 Capability 清单（EdgeX 自动注册）

**MCP Runtime（统一模式，7 条）**：

| Capability ID | 类别 | 执行后端 |
|---------------|------|----------|
| `read_points` | driver | DriverExecutor.ReadPoints |
| `write_points` | driver | DriverExecutor.WritePoint |
| `scan_devices` | driver | DriverExecutor.ScanDevices |
| `list_points` | driver | DriverExecutor.GetDevicePoints |
| `get_diagnostics` | system | DriverExecutor.Diagnostics |
| `ai.protocol_reverse` | ai | AIAdapter → `ai_agent` skill `protocol-reverse` |
| `ai.doc_parse` | ai | AIAdapter → `ai_agent` skill `doc-parse` |

**北向 EAN Runtime（协议特定模式，63 条）**：15 协议 × 4 操作 + 3 系统/AI，Capability ID 格式 `{protocol}.read_point`（读单个数据）/ `{protocol}.write_point`（写单个数据）/ `{protocol}.scan_devices` / `{protocol}.list_points`。**中性命名（v1.12）**：所有协议统一用 `read_point`/`write_point`，不再用 Modbus 专属的 `read_holding_register`/`write_register`（对 Ethernet/IP、PROFINET 等无"保持寄存器"概念的协议语义统一）。协议集合见 `KnownDriverProtocols`（modbus-tcp、modbus-rtu、opc-ua、bacnet-ip、s7 等 15 种）。

### 2.5 Invoke 流程

1. EdgeOS 向 `$edgeos/invoke/{agent_id}` 发布 `invoke_capability` 信封  
2. EdgeX Runtime 校验 target / Capability / 权限  
3. `CapabilityMapper` → `DriverCommand`  
4. `DriverExecutor` 或 `AIAdapter` 执行  
5. 向 `$edgeos/reply/{source}` 回 `invoke_response`（含 `invoke_id` / `status` / `result`）  
6. 可选发布 `capability.invoked` Event

**AI Invoke 约定**：

```json
{
  "invoke_id": "inv-ai-1",
  "target": "edgex-node-001",
  "capability": "ai.protocol_reverse",
  "arguments": {
    "payload": {
      "protocol_id": "modbus-tcp",
      "filename": "sample.pcap"
    },
    "wait": true,
    "wait_timeout_sec": 30
  }
}
```

- 无 `wait`：立即返回 `task_id` + `status=queued/processing...`  
- `wait=true`：阻塞至 `waiting_confirm` / `failed` / 超时，并带回 `deliverables`  
- **落库仍需** 通过 AI 任务 Confirm API（Human-in-the-loop）

### 2.6 Event 与 `previous_value`

ShadowCore COW 写入时，在 **通知克隆** 中附加变更前值（不写入持久快照）：

```json
{
  "event_type": "temperature.changed",
  "device_id": "slave-1",
  "point_id": "temperature",
  "value": 45.2,
  "previous_value": 42.1,
  "metadata": { "quality": "good", "channel_id": "ch-1" }
}
```

首次出现的点位 `previous_value` 可省略（`null`/缺省）。

### 2.7 V1.0 兼容层（仍保留）

| 传输 | V1 Topic/Subject 示例 |
|------|----------------------|
| MQTT | `edgex/nodes/register`、`edgex/devices/report`、`edgex/heartbeat/{node}`、`edgex/cmd/{node}/{device}/write` |
| NATS | `edgex.nodes.register`、`edgex.devices.report`、`edgex.heartbeat.{node}`、`edgex.cmd.{node}.{device}.write` |

EAN 与 V1 并行：新功能用 `$edgeos/*`；旧 EdgeOS 可继续用 V1。

**设备清单 `device_report`（EdgeX 行为）**：

1. **连接成功后主动发布**（与重注册命令路径一致），不唯依赖 `register_response`  
2. **`register_response` status=success** 时再发一次（EdgeOS 确认后对齐）  
3. **5s 超时兜底**：若本连接周期内尚未成功发出过 report，再重试一次（覆盖 publish 丢失 / 从未收到 success 的空窗）  
4. 重注册命令（`node_register`）路径同样立即 `publishDeviceReport`（节点已存在时 EdgeOS 可能不再回 `register_response`）
5. **动态变更重新上报（v1.10）**：通道/设备**增删改**（`addChannel`/`updateChannel`/`removeChannel`/`addDevice`/`updateDevice`/`removeDevice`）或北向 `Devices` 映射变更时，EdgeX 重新发布 `edgex/devices/report`，EdgeOS `ReconcileDevices` 据此增删设备——设备列表无需重启即可同步（联机复测：新增设备 8→9、删除 9→8 自动更新）。

EdgeOS 若仍依赖 V1 设备清单对账，**必须同时订阅** MQTT `edgex/devices/report` 与 NATS `edgex.devices.report`（双传输对称）；漏订任一侧会导致该传输上的设备数空窗或滞后。

---

## 3. EdgeOS 端必须实现的功能

以下为 **EdgeOS 开发落地清单**（缺一则无法完成 EAN 2.0 闭环）。

### 3.1 消息总线与连接

| # | 功能 | 要求 |
|---|------|------|
| OS-1 | MQTT Broker 接入 | 支持订阅/发布 `$edgeos/#`；建议 QoS1 对 Discovery/Invoke/Event |
| OS-2 | NATS 接入 | 订阅/发布相同 `$edgeos/...` subject（保留斜杠）；与 MQTT 语义对称 |
| OS-3 | 双传输对称 | 同一业务逻辑应对 MQTT/NATS 复用编解码，仅替换 transport adapter |

本机联调默认：

- MQTT: `tcp://127.0.0.1:18083`
- NATS: `nats://127.0.0.1:4222`

### 3.2 Discovery Center（必须）

| # | 功能 | 接口约定 |
|---|------|----------|
| OS-4 | 订阅 Agent 上线 | Sub `$edgeos/discovery/agent`，解析 Agent Descriptor，写入全局 Agent 索引 |
| OS-5 | 订阅 Agent 下线 | Sub `$edgeos/discovery/agent/offline`，**`reason=graceful_shutdown`（北向关闭 EAN）时彻底移除 Agent**；心跳超时/异常掉线标记 offline（v1.11） |
| OS-6 | 订阅 Capability | Sub `$edgeos/discovery/capability`，按 `agent_id` 聚合 Capability 列表 |
| OS-7 | 主动查询（可选增强） | Pub `$edgeos/discovery/query`，收 `$edgeos/discovery/response` |
| OS-8 | 索引 API | 对外提供：`ListAgents` / `GetAgent` / `ListCapabilities(filter)` |

**Agent Descriptor 关键字段**（EdgeX 发布）：`id`、`kind`、`version`、`status`、`transport`、`heartbeat_interval_sec`、`metadata`。

**Capability Descriptor 关键字段**：`id`、`agent_id`、`description`、`category`、`input_schema`、`output_schema`、`timeout_sec`、`permission`、`metadata`。

### 3.3 Invoke Orchestrator（必须）

| # | 功能 | 接口约定 |
|---|------|----------|
| OS-9 | 发起 Invoke | Pub `$edgeos/invoke/{target_agent_id}`，信封 `message_type=invoke_capability` |
| OS-10 | 接收 Reply | Sub `$edgeos/reply/{edgeos_planner_id}`（`header.source` 须与 reply topic 一致） |
| OS-11 | Correlation | 使用 `header.correlation_id` + `body.invoke_id` 关联请求/响应 |
| OS-12 | 超时与重试 | 按 Capability `timeout_sec` 设置客户端超时；幂等键建议用 `invoke_id` |
| OS-13 | 状态订阅（可选） | Sub `$edgeos/invoke/{agent_id}/status` 做进度 UI |
| OS-14 | 编排 API | `Invoke(capability, arguments, target)` → 返回 `InvokeResponse` |

**InvokeRequest 最小字段**：

```json
{
  "invoke_id": "uuid",
  "target": "edgex-node-001",
  "capability": "system.diagnostics",
  "arguments": {}
}
```

**InvokeResponse 最小字段**：`invoke_id`、`status`（`completed|failed|...`）、`result.success`、`result.values` / `result.error`。

### 3.4 Event Center（必须）

| # | 功能 | 接口约定 |
|---|------|----------|
| OS-15 | 订阅节点 Event | Sub `$edgeos/event/{agent_id}` 和/或 `$edgeos/event/broadcast` |
| OS-16 | 解析点位变化 | 处理 `event_type={point_id}.changed`，读取 `value` **与** `previous_value` |
| OS-17 | 设备在离线 | 处理 `device.online` / `device.offline` |
| OS-18 | 规则路由 | 支持按 agent/device/point/event_type 过滤并触发规则/告警 |
| OS-19 | 存储/流 | 至少短期缓存最近 Event；生产建议落时序或消息队列 |

**点位变化 Event 契约（EdgeOS 必须兼容）**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `event_type` | string | `{point_id}.changed` |
| `agent_id` | string | 来源 Agent |
| `device_id` | string | 物理设备 ID |
| `point_id` | string | 点位 ID |
| `value` | any | 新值 |
| `previous_value` | any | 旧值（首次可缺省） |
| `timestamp` | int64 | 毫秒 |
| `metadata.quality` | string | 可选 |
| `metadata.channel_id` | string | 可选 |

### 3.5 平台治理（必须/强烈建议）

| # | 功能 | 说明 |
|---|------|------|
| OS-20 | Agent 生命周期视图 | online/offline/heartbeat 超时判定（建议 2～3 个心跳周期） |
| OS-21 | 权限与命名空间 | 按租户/项目限制可 Invoke 的 Capability（尤其 write/admin/AI） |
| OS-22 | 审计 | 记录跨节点 Invoke 的 initiator、target、capability、结果 |
| OS-23 | V1 兼容网关（过渡期） | 若仍有 V1 客户端：可保留 `edgex/*` 适配，但新功能禁止只做 V1；**V1 设备清单须订** MQTT `edgex/devices/report` **与** NATS `edgex.devices.report`（勿只订 MQTT） |

### 3.6 EdgeOS 侧「不要做」的事

- 不要在 EdgeOS 直接驱动南向设备（读/写走 EdgeX Capability Invoke）
- 不要假设 AI Invoke 会自动写配置（必须等待人工 Confirm 或显式 apply API）
- 不要把 NATS subject 擅自改成 `edgeos.discovery.agent` 这类点分形式（除非双方同时改规范）

---

## 4. MQTT 与 NATS 对称说明

| 能力 | MQTT | NATS | 对称性 |
|------|------|------|--------|
| Discovery agent/capability | ✅ | ✅ | 同 Topic 字符串 |
| Invoke + Reply | ✅ | ✅ | 同 |
| Event（含 previous_value） | ✅ | ✅ | 同 |
| Offline / Heartbeat | ✅ | ✅ | 同 |
| V1 兼容 | `edgex/...` | `edgex.*` | **仅 V1 路径分隔符不同** |
| Bridge 代码 | `edgos_mqtt/ean_bridge.go` | `edgos_nats/ean_bridge.go` | `Bus` 接口对称 |
| 执行器 | `NewWiredExecutor` | `NewWiredExecutor` | 同 Driver+AI |

集成测试：

- `internal/northbound/edgos_mqtt/ean_integration_test.go`
- `internal/northbound/edgos_nats/ean_integration_test.go`

> **NATS 订阅 V1 数据面警示（v1.6，联合调试发现）**：订阅 `edgex.*` 数据面时**勿用双 `>` 通配**（如 `edgex.data.>.>`）——nats-server（MQTT gateway 版）会拒绝并关闭连接（EdgeOS 曾因此 NATS 断连）。用单 `>`（`edgex.data.>`）即可匹配 `edgex.data.{node}.{device}`，语义与 MQTT `edgex/data/+/+` 一致。

---

## 5. V1.0 兼容策略

### 5.0 协议共识：数据面默认开放，EAN 能力层按需开启（EdgeOS 必读）

EdgeOS(MQTT/NATS) 北向通道是**两层协议**，由通道配置的 `ean_enabled` 开关控制：

| 层 | 开关 | 默认 | 行为 |
|---|---|---|---|
| **V1 数据面（基础上报）** | 通道 `enable=true` 即生效 | ✅ 开启 | 设备注册（`edgex/devices/report`）、点位元数据（`edgex/points/*`）、实时数据（`edgex/data/{node}/{device}`）、设备状态（`edgex/devices/*`）、告警（`edgex/events/*`）。**与 EAN 无关，始终工作**。*V1 节点面（`edgex/nodes/*`）在 `v1_command_enabled=false`（v1.9 全面下线）时移除，节点由 EAN Discovery 替代* |
| **EAN 能力层（读写/发现/编排）** | 通道内 `ean_enabled=true` | ❌ 关闭 | `$edgeos/discovery/*`（Agent/Capability 发现）、`$edgeos/invoke/*` + `$edgeos/reply/*`（Capability 读写调用）、`$edgeos/heartbeat/*`（Agent 心跳）、`$edgeos/event/*`（EAN Event，受 `ean_event_auto_publish` 控制） |

**约定**：
- EdgeOS 建北向通道时，**即使不开启 EAN**，也必须订阅 V1 数据面 Topic 以接收节点/设备/点位/实时数据。
- **开启 `ean_enabled=true` 后**，EdgeOS 才可发现 Capability 并发起 Invoke（读写设备）。未开启时不应向 `$edgeos/invoke/*` 发调用——EdgeX 无 Runtime 订阅，消息将静默丢弃。
- `ean_event_auto_publish=false`（默认）时，EAN Runtime 正常收发 Invoke/Discovery/Heartbeat，但**不自动广播点位变化 Event**；置 `true` 才发布 `$edgeos/event/{agent}` 点位变化事件。
- MCP Runtime 是进程内基础能力层，与北向通道无关，始终可用（供本地 LLM 调用，不走 MQTT/NATS）。

> **EdgeOS 对接验收**：`ean_enabled=false` 时验证 V1 数据面贯通 + `$edgeos/#` 无 Discovery 流量；`ean_enabled=true` 时验证 63 条 Capability 发现 + Invoke/Reply 闭环 + Heartbeat。

1. **并行运行**：EAN `$edgeos/*` 与 V1 `edgex/*`（MQTT）/`edgex.*`（NATS）同时可用。  
2. **新功能只加 EAN**：Capability / Discovery / Invoke / Event 新能力不回灌 V1。  
3. **迁移建议**：EdgeOS 先实现 EAN Discovery+Invoke+Event，再逐步将 V1 控制面切到 Invoke。  
4. **双写窗口**：过渡期 EdgeX 可同时发 V1 点位上报与 EAN Event；EdgeOS 应以 EAN Event 为准做新规则。

**Phase 4（v1.7→v1.9）V1 命令面状态**：

- **EdgeX 侧**：V1 命令 Topic（`edgex/cmd/*`）订阅已**全面下线**（v1.9 `v1_command_enabled=false`）：不再订阅/发布 `edgex/cmd/*`、不再发布 V1 节点注册、不运行 V1 心跳循环；命令统一走 EAN Invoke。
- **EdgeOS 侧**：V1→EAN Bridge（轮询合成 Agent/心跳/点位 Event）已移除（OS-P4）；`edgex/cmd/responses/#` 订阅已移除，`PublishCommand` 跳过（`v1_command_enabled=false`）；命令统一 `$edgeos/invoke/*`。
- **保留**：V1 数据面（`edgex/data/*`、`edgex/points/*`、`edgex/devices/*`）与告警（`edgex/events/*`）长期保留。
- **全面下线开关（v1.8/1.9 `V1CommandEnabled`）**：EdgeX 北向通道（`EdgeOSMQTTConfig`/`EdgeOSNATSConfig`）与 EdgeOS `ean.v1_command_enabled` 开关——**v1.9 已置 `false`（全面下线）**：EdgeX 停止订阅/发布 `edgex/cmd/*` 与 `edgex/nodes/*`、V1 心跳循环，EdgeOS 跳过 `PublishCommand`/主动发现；V1 数据面/告警不受影响。

---

## 6. 联调步骤

### 6.1 前置

```bash
# NATS（本机）
# 确认监听 4222
nats server check connection --server=nats://127.0.0.1:4222
# 或
nc -vz 127.0.0.1 4222

# MQTT（本机 EMQX/Mosquitto 等）
nc -vz 127.0.0.1 18083
```

### 6.2 跑 EdgeX 集成测试

```bash
cd d:/code/edgex

# NATS：Discovery + Invoke(system.diagnostics) + Event(previous_value)
go test ./internal/northbound/edgos_nats/ -run TestEANIntegrationNATSDiscoveryInvokeEvent -v -count=1

# MQTT：同上（broker 不可达时自动 Skip）
go test ./internal/northbound/edgos_mqtt/ -run TestEANIntegrationMQTTDiscoveryInvokeEvent -v -count=1

# AI Adapter + Capability 路由
go test ./internal/execution/ -run 'AI|DriverExecutor' -v -count=1

# Shadow previous_value
go test ./internal/core/ -run TestShadowCore_NotifyIncludesPreviousValue -v -count=1

# 构建
go build -o /dev/null ./cmd/
```

### 6.3 手工联调（NATS 示例）

1. 启动 EdgeX，配置 `northbound.edgeos_nats`：`url=nats://127.0.0.1:4222`，`node_id=edgex-node-001`，`enable=true`  
2. EdgeOS（或 nats CLI）订阅：  
   - `$edgeos/discovery/agent`  
   - `$edgeos/discovery/capability`  
   - `$edgeos/event/edgex-node-001`  
   - `$edgeos/reply/edgeos-planner`  
3. 发布 Invoke：

```json
{
  "header": {
    "message_id": "msg-1",
    "timestamp": 0,
    "source": "edgeos-planner",
    "message_type": "invoke_capability",
    "version": "2.0",
    "correlation_id": "corr-1"
  },
  "body": {
    "invoke_id": "inv-1",
    "target": "edgex-node-001",
    "capability": "system.diagnostics",
    "arguments": {}
  }
}
```

Subject: `$edgeos/invoke/edgex-node-001`

4. 确认 Reply `status=completed`  
5. 制造点位变化（或调用 Runtime Event），确认 Event 含 `previous_value`

MQTT 手工步骤相同，仅把 NATS Publish/Subscribe 换成 MQTT，Broker `127.0.0.1:18083`。

---

## 7. 验收清单

### 7.1 EdgeX

- [x] Capability Runtime 随 MQTT/NATS 连接自动 Start / OnConnected  
- [x] Discovery 发布 agent + capability  
- [x] Invoke `system.diagnostics` 成功  
- [x] Invoke `ai.protocol_reverse` / `ai.doc_parse` 不再报「未接线」，返回 task / deliverables  
- [x] Shadow 点位变化 Event 含 `previous_value`  
- [x] NATS 集成测试通过（`127.0.0.1:4222`）  
- [x] MQTT 集成测试通过（`127.0.0.1:18083` 可达时）  
- [x] `go build ./cmd/` 通过  
- [x] V1 命令面标记 DEPRECATED（订阅/发布 WARN；EX-P4-01/02，v1.7）

### 7.2 EdgeOS（对端验收）

- [x] 双传输订阅 `$edgeos/discovery/*` 并建立索引
- [x] 能向任意 online Agent 发起 Invoke 并解析 Reply
- [x] 订阅 Event 并正确使用 `previous_value`（差值/告警/审计）
- [x] Agent 心跳超时标记 offline，与 `discovery/agent/offline` 一致
- [x] 权限：限制 write/admin/AI Capability 的调用方
- [x] 与 V1 并存时无 Topic 冲突 / 双处理重复副作用
- [x] V1 设备清单：MQTT 订 `edgex/devices/report`，NATS 订 `edgex.devices.report`（双传输对称）

> 以上 EdgeOS 验收项已于 **2026-08-03 两端联合联调** 在本机（192.168.3.104，MQTT 18083 + NATS 4222）全部验证通过，详见 [V1-to-EAN-Migration-Assessment §7.0](../TODO/V1-to-EAN-Migration-Assessment.md)。

---

## 8. 已知限制与后续

| 项 | 说明 | 后续建议 |
|----|------|----------|
| AI 为本地 Mock/配额流水线 | 远端 Model Center 未强制接通 | 在 `ai_agent` 接入真实 LLM Provider；设置页已有 remote 模式骨架 |
| AI 不自动落库 | Confirm 后才 apply | EdgeOS 工作流节点增加「等待确认」状态 |
| Shadow Event 批内合并 | 同批多次写合同一点时，previous 取批前值 | 一般可接受；若需逐步差分可改通知不合并 |
| State 同步 Topic | `$edgeos/state/*` 已预留 | EdgeOS 需要全量/增量状态时可再实现 |
| Capability Planner | AI 规划输出仍以任务交付物为主 | EAN-增强阶段：Planner 直接产出 Capability Invoke 图 |
| 全量 `internal/core` stress | 长时间压力测试可能触达超时 | 与本改造无关；CI 建议 `-short` 或拆分 stress |
| V1 device_report 依赖 EdgeOS 订阅 | EdgeX 已连接即发 + 超时兜底；若 EdgeOS messaging 未订对应 Topic/Subject，清单仍无法入库 | EdgeOS 按 OS-23 补齐 MQTT/NATS 双订，并用 `ReconcileDevices` 对账 |
| V1 命令面（已下线） | `v1_command_enabled=false`（v1.9）：EdgeX 不再订阅/发布 `edgex/cmd/*`、`edgex/nodes/*`；EdgeOS `PublishCommand` 跳过；命令统一 EAN Invoke | 已全面下线；V1 数据面/告警长期保留 |

---

## 9. 附录：本轮 EdgeX 改动摘要

| 文件 | 变更 |
|------|------|
| `internal/execution/ai_adapter.go` | 新增 AIAdapter，对接 `ai_agent.Agent` |
| `internal/execution/driver_executor.go` | AI 命令委托 AIAdapter；`NewWiredExecutor` |
| `internal/execution/ai_adapter_test.go` | AI Invoke 单测 |
| `internal/model/types.go` | `ShadowPoint.PreviousValue`（notify-only） |
| `internal/core/shadow_pool.go` | `cloneShadowDeltaForNotify` |
| `internal/core/shadow_core.go` | 写路径携带 previous |
| `internal/core/northbound_manager_ext.go` | Event Bridge 传递 previous；publisher 生命周期刷新（同步 + 写锁下延迟） |
| `internal/core/shadow_previous_value_test.go` | previous_value 单测 |
| `internal/northbound/edgos_{mqtt,nats}/ean_bridge.go` | WiredExecutor；`notifyEANRuntimeChanged` 同步回调 |
| `internal/northbound/edgos_{mqtt,nats}/client.go` | 连接/重注册/`register_response`/5s 超时 `device_report` 兜底 |
| `internal/northbound/edgos_nats/ean_integration_test.go` | NATS 真实联调 |
| `internal/northbound/edgos_mqtt/ean_integration_test.go` | MQTT 校验 previous_value |
| `internal/server/mcp_handler.go` | MCP Runtime 注入 AIAdapter |
| `docs/edgeos/AI协同组件规划.md` | 状态同步 |

---

**维护者**: EdgeX / edgeOS 团队  
**下一步**: V1 命令面已全面下线（`v1_command_enabled=false`，v1.9）；命令路径完全由 EAN Invoke 承载，联机复测通过。V1 数据面与告警长期保留。  
**关联文档**: [AI协同组件规划.md](./AI协同组件规划.md) · [V1-to-EAN-Migration-Assessment.md](../TODO/V1-to-EAN-Migration-Assessment.md) · [EdgeX通信协议规范(MQTT-NATS).md](./EdgeX通信协议规范(MQTT-NATS).md)
