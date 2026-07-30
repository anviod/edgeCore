# V1 命令协议迁移至 EAN 2.0 评估方案

| 属性 | 值 |
|---|---|
| 文档版本 | 2.14 |
| 日期 | 2026-07-30 |
| 适用范围 | EdgeX + EdgeOS 联合架构 |
| 状态 | **Phase 1 已完成 / Phase 2 跨系统路径已打通并实机复验通过（MQTT + NATS）/ Phase 3 EdgeX 侧全部完成（EX-P3-01~09）/ EdgeOS 侧 OS-P3-01/02 完成 / v2.14：MCP 工具合并优化（94→32）+ 地址语义统一修复（point_id/address/name 三形式自动解析）+ BACnet 设备扫描硬指标通过 / v2.13：EdgeX 侧 `device_report` 连接/重注册/超时兜底与 publisher 刷新竞态收尾** |
| 变更说明 | v2.14（EdgeX 侧）: **工具合并与地址语义**——① 将 15 协议×4 操作矩阵（60 工具）合并为 4 通用工具（read_points/write_points/scan_devices/list_points），MCP 工具总数从 94 降至 32（62% 压缩），消除 ToolSearch 索引抖动；② `DriverExecutor.resolvePointIDs()` 实现 point_id/address/name 三形式自动解析，修复 list_points 返回 PDU 偏移而 read_points 期望 point_id 的语义割裂 bug；③ MCP 工具 schema 增加 point_id 参数和地址语义说明文档标注；④ BACnet 设备 2228316/2228317/2228318/2228319 扫描硬指标全部通过（quality=Good）；⑤ goreleaser arm64 deb 包部署到 192.168.3.230 实机回归验证通过。v2.13（EdgeX 侧）: **收尾残留**——① `notifyEANRuntimeChanged` 改为同步刷新 publisher，`refreshEANEventPublishers` 在持 `nm.mu` 时 TryRLock 失败则延迟阻塞刷新（消竞态、防死锁）；② 连接成功即发 V1 `device_report` + 5s 未成功则重试，与重注册/`register_response` 路径对齐；③ 改造指南明确 EdgeOS 须双订 MQTT `edgex/devices/report` 与 NATS `edgex.devices.report`。v2.12（EdgeOS 侧）: **设备数/上报一致性**——根因：V1 `edgex/devices/report` 全量快照在 EdgeOS 只 Upsert 不剪枝（实机 EdgeX=4 / EdgeOS 曾=14）；非 Discovery/EAN Event/双传输重复计数。修复：`ReconcileDevices`、EAN→Registry 镜像、`POST /api/nodes/:id/devices/reconcile`；实机恢复 devices=4、nodes=1、agents=1、native_caps=63；`go test ./...` 全绿 + UI build 通过。**仍属 EdgeOS 侧**：若 messaging 未订 NATS `edgex.devices.report`，NATS 传输上的 V1 清单仍可能空窗（见改造指南 OS-23）。v2.11（双侧）: NATS 传输对称联调端到端复验——EdgeX NATS stats API 返回 `ean_metrics`（total_invokes=8, success_rate=100%），对齐 MQTT；EdgeOS 3 个 Invoke 测试全部 completed（system.diagnostics/modbus_tcp.list_points/bacnet_ip.list_points），P50=3ms P99=6ms v1_fallback=0；OS-P3-01 V1 Fallback 移除（v1_bridge_caps=0）；OS-P3-02 前端写操作切换 EAN Invoke（ControlView/PointListView）。v2.10（EdgeOS 侧）: NATS 对称补齐——NATS 延迟订阅/Connect·Reconnect 补订对齐 MQTT；Health 新增 `transport_details[{name,connected,endpoint}]`，`northbound_runtime` 改为 `mqttBus+natsBus`；UI Overview/DebugHelp/Dashboard 展示分传输连接态与 broker 地址；联调帮助拆分 MQTT/NATS 步骤并补充 NATS Subject 用例；`go test ./internal/ean/` + UI build 通过；实机复验（2026-07-28）：4222 可达、`registered_transports=2`、Agent `transport=["nats"]`/`northbound=edgeos_nats`、63 native Cap、`system.diagnostics` Invoke completed（avg≈5ms，`v1_fallback=0`）。v2.9（EdgeOS 侧）: NATS 传输对称联调——启用 `ean.nats.enabled=true`（`nats://127.0.0.1:4222`），EdgeOS `registered_transports=2`（mqtt+nats），63 条原生 Cap 通过 NATS 通道索引（`v1_bridge_caps=0`），`system.diagnostics` 通过双传输 Invoke 端到端成功（6ms / `v1_fallback=0`）；EdgeX NATS 北向通道 `EAN-NATS` 已启用（`ean_enabled=true`）。v2.8（EdgeOS 侧）: Governance 权限修复——default 租户无策略时放行 admin/write/ai（修复 system.diagnostics 502）；实机复验 63 条原生 Cap + system.diagnostics Invoke 端到端成功。v2.7（EdgeOS 侧）: 启动韧性对齐 messaging.Manager——MQTT ConnectRetry/AutoReconnect + 订阅延后补订，broker 不可用时不 fatal；Health 增加 `invoke_metrics`/`registered_transports`；默认 MQTT broker 改为 `18083`；UI 对齐 `source`/原生 Cap 计数/Invoke 指标；诚实勾选 Phase 3 EdgeOS 进度。v2.6（EdgeX 侧）: 深化双 Runtime 战略定位。v2.5（EdgeX 侧）: 明确双 Runtime。v2.4（EdgeOS 侧功能）: 仅对接北向 EAN Runtime；V1 purge/AgentSource |

---

## 1. 执行摘要

**结论：Phase 1 基础能力已全部落地，Phase 2 跨系统 Invoke 验证通过。当前处于 Phase 2→Phase 3 过渡期。**

EdgeX 内部维持双 Capability Runtime 架构，这是面向不同场景的互补设计：

- **MCP Runtime（基础能力层）**：Server 启动即就绪，零外部依赖。通过 LLM 接入即可完成设备读写、协议逆向、文档解析等本地智能操作，无需 MQTT/NATS 连接，无需 EdgeOS 参与。统一模式下通过 `GenerateUnifiedCapabilities` 生成 7 条统一 Capability，对应约 32 个 MCP 工具（7 `ean_*` unified + 25 `edgex_*`）始终可用。v2.14 工具合并优化后，15 协议×4 操作矩阵合并为 4 通用工具，工具数从 94 降至 32（62% 压缩）。
- **北向 EAN Runtime（高级协作层）**：依赖 EdgeOS(MQTT/NATS) 北向通道连接，启用后 EdgeOS 可远程发现并调用本设备 Capability，支持跨系统 Agent 编排和分布式调用。属于高级功能，不是基础功能。

EAN 配置（`EANEnabled` / `EANHeartbeatSec` / `EANEventAutoPublish`）**已合并**到 EdgeOS(MQTT/NATS) 北向通道配置字段，由北向管理器统一持久化和热更新。用户在北向通道弹窗中即可控制 EAN 启停。EdgeOS 侧独立 `ean:` 段仍用于 Coordination Platform 自身连接参数，与 EdgeX 通道字段分工不同。

V1 协议中，设备映射（`Devices` 字段）和数据上报路径（`edgex/points/*`、`edgex/data/*`）因 pub/sub 批量推送效率优势和外部集成依赖而长期保留。其余功能（节点注册、心跳、设备发现、命令下发/响应、Capability 发现）全部迁移到 EAN 2.0。

---

## 2. 当前架构状态（与代码一致）

### 2.1 EAN 设置现状（EdgeX）

EdgeX 侧 EAN 启停已并入北向 EdgeOS 通道配置（EX-P3-02/08 已落地）：

| 位置 | 现状 |
|---|---|
| `EdgeOSMQTTConfig` / `EdgeOSNATSConfig` | 含 `ean_enabled` / `ean_heartbeat_sec` / `ean_event_auto_publish`；热更新启停 Runtime |
| 北向通道 UI | 通道配置中控制 EAN 能力层 |
| MCP Runtime | 进程级独立，不受 `EANEnabled` 影响 |

EdgeOS Coordination Platform 仍可有自身 `ean:` / messaging 配置（订阅 `$edgeos/*` 与 V1 Topic），与 EdgeX 通道字段不是同一配置面。

### 2.2 V1 与 EAN 双路径现状

| 双路径 | V1 路径 | EAN 路径 | 当前状态 |
|---|---|---|---|
| 写命令 | `handleWriteCommand` -> `sb.WritePoint()` | `Dispatcher` -> `CapabilityMapper` -> `DriverExecutor` -> `sb.WritePoint()` | EAN 为主，V1 Fallback 保留（40%/60% 超时分配） |
| 设备发现 | `handleDiscoverCommand`（仅返回确认） | `*.scan_devices` Capability Invoke | EAN 已实现，V1 标记 deprecated |
| 节点注册 | `edgex/nodes/register` + 心跳 | `$edgeos/discovery/agent` + Heartbeat | EAN 已替代，V1 保留兼容 |
| Capability 发现 | V1 Bridge 合成 `{node}/{device}/read-write` | 北向 mqttBus 发布原生 EAN Cap | **已隔离+purge**：原生到达后清除 V1 伪 Cap |

### 2.3 V1 Bridge 隔离机制（当前实现）

**代码位置**：`edgeOS/internal/ean/discovery.go`、`edgeOS/internal/ean/bridge.go`

```
V1 Bridge 不再"完全移除"，而是"智能隔离"（对接 EdgeX 北向 mqttBus，非 MCP Runtime）：
- 若 Agent 已有原生 EAN Cap / Agent（北向 discovery）
  → 跳过设备级 Cap 合成，且不覆盖原生 Agent 描述符
  → 首条原生 Cap 到达时 purge 该 Agent 下全部残留 …/read-write
- 若无原生 EAN（EdgeX 未升级 / 北向未发布）
  → 继续合成 {nodeID}/{deviceID}/read-write，确保兼容性
- 心跳兜底与点位 Event 同步仍可执行
```

**隔离规则**（`discovery.go`）：
- Cap：`native-ean` 拒绝同名 `v1-bridge` 覆盖；可升级覆盖同名 `v1-bridge`；原生到达 purge 全量 v1 Cap
- Agent：`agentSources` 标记，原生不被 v1-bridge 覆盖

---

## 3. 迁移范围界定

### 3.1 保留 V1（不迁移）

| 功能 | V1 Topic / 机制 | 保留原因 |
|---|---|---|
| **设备映射** | `Devices map[string]DevicePublishConfig` | 设备级上报策略，属于通道配置语义 |
| **点位数据上报** | `edgex/data/{node}/{device}` | pub/sub 批量推送效率高于 Invoke request/response |
| **点位元数据上报** | `edgex/points/{node}/{device}` | 全量同步效率高于按需查询 |
| **设备状态上报** | `edgex/devices/{node}/{device}/online` | 高频状态变化，pub/sub 模型更合适 |
| **告警/事件** | `edgex/events/alert` | 已被外部监控系统订阅 |
| **V1 Bridge 隔离** | `bridge.go` 轮询合成 | 过渡期兼容未升级 EdgeX 节点 |
| **V1 Fallback** | `v1_invoke_bridge.go` | 过渡期 EAN Invoke 失败降级 |

### 3.2 迁移到 EAN 2.0

| 功能 | V1 Topic / 机制 | EAN 等价方案 | 状态 |
|---|---|---|---|
| **节点注册** | `edgex/nodes/register` | `$edgeos/discovery/agent`（retained） | 已完成 |
| **节点心跳** | `edgex/heartbeat/{node}` | `$edgeos/heartbeat/{agent}` | 已完成 |
| **节点离线** | `edgex/nodes/{node}/offline`（LWT） | `$edgeos/discovery/agent/offline` | 已完成 |
| **设备发现命令** | `edgex/cmd/{node}/discover` | `*.scan_devices` Capability Invoke | 已完成 |
| **写命令** | `edgex/cmd/{node}/{device}/write` | `*.write_register` Capability Invoke | 已完成 |
| **命令响应** | `edgex/cmd/responses/{node}/{device}` | `$edgeos/reply/{agent}` | 已完成 |
| **Capability 发现** | V1 Bridge 合成 | 原生 63 条 EAN Capability | 已完成 |
| **读命令** | V1 无（仅写命令） | `*.read_holding_register` Capability Invoke | 已完成 |
| **点位列表查询** | V1 无 | `*.list_points` Capability Invoke | 已完成 |
| **系统诊断** | V1 无 | `system.diagnostics` Capability Invoke | 已完成 |
| **AI 协议逆向** | V1 无 | `ai.protocol_reverse` Capability Invoke | 已完成 |
| **AI 文档解析** | V1 无 | `ai.doc_parse` Capability Invoke | 已完成 |

---

## 4. EAN 合并到北向 EdgeOS 通道（设计态，待实现）

### 4.1 设计原则

EdgeX 北向 `EdgeOS(MQTT/NATS)` 通道承担三层职责，EAN 是其中最上层的能力层。MCP Runtime 完全独立于通道，始终随 Server 启动：

```
┌─ EdgeX 进程 ──────────────────────────────────────────────────┐
│                                                                │
│  ┌─ MCP Runtime（独立基础层，不依赖通道）────────────────────┐ │
│  │  TransportSDK + NoopBus                                     │ │
│  │  Server 启动即就绪，LLM 接入即可使用                        │ │
│  │  ├── 7 Unified Capability（`GenerateUnifiedCapabilities`）   │ │
│  │  ├── ~34 MCP 工具（9 `ean_*` unified + 25 `edgex_*`）      │ │
│  │  └── /api/capability/invoke（本地 HTTP）                    │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                │
│  ┌─ EdgeOS(MQTT) 北向通道 ──────────────────────────────────┐  │
│  │                                                           │  │
│  │  [1] 通道基础层（连接、认证、重连）                       │  │
│  │   ├── broker / client_id / node_id / username / password  │  │
│  │   ├── keep_alive / auto_reconnect / clean_session         │  │
│  │   └── 通道启停（Enable 字段）                             │  │
│  │                                                           │  │
│  │  [2] 数据上报层（V1 保留，长期共存）                      │  │
│  │   ├── 设备映射（Devices map）                             │  │
│  │   ├── 实时数据上报（edgex/data/*）                        │  │
│  │   ├── 点位元数据上报（edgex/points/*）                    │  │
│  │   ├── 设备状态上报（edgex/devices/*）                     │  │
│  │   └── 告警事件上报（edgex/events/*）                      │  │
│  │                                                           │  │
│  │  [3] EAN 能力层（高级功能，按需启用）                     │  │
│  │   ├── ean_enabled: bool           ← 控制北向 EAN Runtime  │  │
│  │   ├── ean_heartbeat_sec: int      ← 心跳周期（替代硬编码）│  │
│  │   ├── ean_event_auto_publish: bool ← 事件自动发布开关     │  │
│  │   │                                                       │  │
│  │   ├── $edgeos/discovery/*  (retained, 周期发布)           │  │
│  │   ├── $edgeos/invoke/*     (接收 EdgeOS 远程调用)         │  │
│  │   ├── $edgeos/reply/*      (返回调用结果)                 │  │
│  │   └── $edgeos/heartbeat/*  (Agent 心跳)                   │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                │
│  ┌─ 共享执行内核 ──────────────────────────────────────────┐  │
│  │  CapabilityMapper → DriverExecutor → Southbound          │  │
│  └──────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────┘
```

**启停规则**（四态）：

| 通道 Enable | EANEnabled | V1 数据上报 | EAN 能力层 | MCP Runtime |
|---|---|---|---|---|
| `false` | - | 不工作 | 不工作 | **始终运行** |
| `true` | `false` | 工作 | 不启动 | **始终运行** |
| `true` | `true` | 工作 | 启动 | **始终运行** |
| `true` | `true` → `false`（热更新） | 工作 | 停止 Runtime | **始终运行** |

MCP Runtime 不受通道配置影响，始终随 Server 启动。即使用户未创建任何北向通道，LLM 接入后仍可通过 MCP 工具调用 7 条统一 Capability（`GenerateUnifiedCapabilities`）完成设备操作。

### 4.2 数据结构变更

**当前 `EdgeOSMQTTConfig`**（`internal/model/types.go:421`，已验证）：

```go
type EdgeOSMQTTConfig struct {
    ID                   string                         `json:"id" yaml:"id"`
    Name                 string                         `json:"name" yaml:"name"`
    Enable               bool                           `json:"enable" yaml:"enable"`
    Broker               string                         `json:"broker" yaml:"broker"`
    ClientID             string                         `json:"client_id" yaml:"client_id"`
    NodeID               string                         `json:"node_id" yaml:"node_id"`
    Username             string                         `json:"username" yaml:"username"`
    Password             string                         `json:"password" yaml:"password"`
    QoS                  byte                           `json:"qos" yaml:"qos"`
    Retain               bool                           `json:"retain" yaml:"retain"`
    CleanSession         bool                           `json:"clean_session" yaml:"clean_session"`
    KeepAlive            int                            `json:"keep_alive" yaml:"keep_alive"`
    ConnectTimeout       int                            `json:"connect_timeout" yaml:"connect_timeout"`
    AutoReconnect        bool                           `json:"auto_reconnect" yaml:"auto_reconnect"`
    MaxReconnectInterval int                            `json:"max_reconnect_interval" yaml:"max_reconnect_interval"`
    HeartbeatInterval    string                         `json:"heartbeat_interval" yaml:"heartbeat_interval"` // V1 心跳
    Devices              map[string]DevicePublishConfig `json:"devices" yaml:"devices"`
    VirtualDevices       OpcUaDeviceMap                 `json:"virtual_devices" yaml:"virtual_devices"`
}
```

**新增 EAN 字段**（追加到上述 struct 末尾，不修改现有字段）：

```go
    // ── EAN 2.0 能力层配置 | EAN 2.0 capability layer config ──
    // EANEnabled 控制北向 EAN Runtime 启停；MCP Runtime 不受此字段影响
    EANEnabled          bool `json:"ean_enabled" yaml:"ean_enabled"`
    // EANHeartbeatSec EAN 心跳周期（秒），0 时使用默认值 60
    EANHeartbeatSec     int  `json:"ean_heartbeat_sec" yaml:"ean_heartbeat_sec"`
    // EANEventAutoPublish 设备数据变化时是否自动发布 EAN Event
    EANEventAutoPublish bool `json:"ean_event_auto_publish" yaml:"ean_event_auto_publish"`
```

`EdgeOSNATSConfig`（`types.go:443`）追加相同三个字段。`HeartbeatInterval string`（V1 心跳）保持不变，与 `EANHeartbeatSec int`（EAN 心跳）互不干扰——前者控制 V1 节点心跳 topic，后者控制 EAN Agent 心跳 topic。

### 4.3 后端实现方案

#### 4.3.1 `EnsureCapabilityRuntime` 改造

**当前代码**（`edgos_mqtt/ean_bridge.go:71`，已验证）：`OnConnect` 回调中无条件调用 `EnsureCapabilityRuntime` + `startEANLocked`，`HeartbeatIntervalSec` 硬编码为 30。

**改造后**：

```go
func (c *Client) EnsureCapabilityRuntime(agentVersion string) (*capability.Runtime, error) {
    c.eanMu.Lock()
    defer c.eanMu.Unlock()
    if c.eanRuntime != nil {
        return c.eanRuntime, nil
    }
    c.configMu.RLock()
    nodeID := c.config.NodeID
    eanEnabled := c.config.EANEnabled
    heartbeatSec := c.config.EANHeartbeatSec
    c.configMu.RUnlock()
    
    if !eanEnabled {
        // EAN 能力层未启用，不创建 Runtime | EAN capability layer disabled
        return nil, nil
    }
    if nodeID == "" {
        nodeID = c.nodeID
    }
    if heartbeatSec <= 0 {
        heartbeatSec = 60 // 默认 60s | default 60s
    }
    
    rt, err := capability.NewRuntime(capability.RuntimeConfig{
        AgentID:              nodeID,
        AgentVersion:         agentVersion,
        Transport:            capability.TransportMQTT,
        HeartbeatIntervalSec: heartbeatSec, // 替代硬编码 30
        Metadata: map[string]any{
            "northbound": "edgeos_mqtt",
            "compat":     "v1_topics_retained",
        },
    }, mqttBus{client: c})
    if err != nil {
        return nil, err
    }
    rt.SetMapper(execution.NewCapabilityMapper(execution.NewWiredExecutor(c.sb)))
    c.eanRuntime = rt
    return rt, nil
}
```

#### 4.3.2 `OnConnect` 回调改造

**当前代码**（`edgos_mqtt/client.go:303-312`，已验证）：

```go
// 当前：无条件创建并启动
if _, err := c.EnsureCapabilityRuntime(capability.RuntimeVersion); err != nil {
    zap.L().Warn("Failed to ensure EAN Capability Runtime", ...)
} else {
    c.startEANLocked(context.Background())
}
```

**改造后**：

```go
// 改造后：EnsureCapabilityRuntime 内部检查 EANEnabled
// 返回 nil 时说明 EAN 未启用，跳过 startEANLocked
rt, err := c.EnsureCapabilityRuntime(capability.RuntimeVersion)
if err != nil {
    zap.L().Warn("Failed to ensure EAN Capability Runtime", ...)
} else if rt != nil {
    c.startEANLocked(context.Background())
} else {
    zap.L().Info("EAN capability layer disabled, skipping Runtime start",
        zap.String("node_id", nodeID))
}
```

`edgos_nats/ean_bridge.go` 和 `edgos_nats/client.go` 做对称改造。

#### 4.3.3 调整项汇总

| 调整项 | 当前代码 | 目标代码 | 涉及文件 |
|---|---|---|---|
| EAN Runtime 启停 | `OnConnect` 无条件 `EnsureCapabilityRuntime` + `startEANLocked` | `EnsureCapabilityRuntime` 内部检查 `EANEnabled`，返回 nil 时跳过启动 | `edgos_mqtt/ean_bridge.go:71` / `edgos_nats/ean_bridge.go:66` |
| EAN 心跳周期 | 硬编码 `HeartbeatIntervalSec: 30`（`ean_bridge.go:87`） | 读取 `cfg.EANHeartbeatSec`，为 0 时默认 60 | 同上 |
| EAN Runtime 停止 | 无 `stopEAN()` 方法 | 新增 `StopCapabilityRuntime()`：调用 `rt.Stop()` + 置 nil + 清理订阅 | `edgos_mqtt/ean_bridge.go` / `edgos_nats/ean_bridge.go` |
| EAN 设置 PUT | `handleEanSettingsUpdate` 存根，不持久化 | **移除**该 handler 和路由 | `capability_handler.go` / `server.go` |
| EAN 设置 GET | 从北向配置派生 | 保留，增加 `ean_enabled` / `ean_heartbeat_sec` / `ean_event_auto_publish` | `capability_handler.go` |

### 4.4 配置热更新

**当前代码**：北向管理器 `updateEdgeOSMQTTClients`（`northbound_manager_edgos.go`）在配置变更时仅处理连接层参数（broker、认证等），不感知 EAN 字段变化。

**改造方案**：在 `updateEdgeOSMQTTClients` 中新增 EAN 字段 diff 检测：

```go
// 伪代码 | pseudocode
func (m *Manager) updateEdgeOSMQTTClients(old, new []model.EdgeOSMQTTConfig) {
    for i := range new {
        oldCfg := findOldByID(old, new[i].ID)
        if oldCfg == nil {
            continue // 新增通道，由连接逻辑处理
        }
        
        // 检测 EAN 字段变化 | Detect EAN field changes
        eanWasEnabled := oldCfg.EANEnabled
        eanNowEnabled := new[i].EANEnabled
        
        if !eanWasEnabled && eanNowEnabled {
            // false → true：启动 EAN Runtime
            // 仅在通道已连接时触发
            if client := m.getMQTTClient(new[i].ID); client != nil && client.IsConnected() {
                client.EnsureCapabilityRuntime(capability.RuntimeVersion)
                client.StartEAN(context.Background())
            }
        } else if eanWasEnabled && !eanNowEnabled {
            // true → false：停止 EAN Runtime
            if client := m.getMQTTClient(new[i].ID); client != nil {
                client.StopCapabilityRuntime()
            }
        }
        
        // 心跳间隔变化：重启 Runtime 以应用新参数
        if eanNowEnabled && oldCfg.EANHeartbeatSec != new[i].EANHeartbeatSec {
            if client := m.getMQTTClient(new[i].ID); client != nil {
                client.StopCapabilityRuntime()
                client.EnsureCapabilityRuntime(capability.RuntimeVersion)
                client.StartEAN(context.Background())
            }
        }
    }
}
```

NATS 通道（`updateEdgeOSNATSClients`）做对称改造。

### 4.5 前端实现方案

| 组件 | 当前 | 目标 |
|---|---|---|
| `EdgeOSMQTTSettingsDialog.vue` | 仅通道基础配置 + 设备映射 | 新增 "EAN 能力层" Tab 页，含启用开关、心跳间隔输入、事件自动发布开关、只读状态展示 |
| `EdgeOSNATSSettingsDialog.vue` | 同上 | 同上 |
| `AiSettingsDialog.vue` "EAN 接入" Tab | 含启停控制（功能无效） | 移除启停控制，降级为只读状态展示卡片 + "前往北向通道配置" 跳转按钮 |
| `NorthboundChannelCard.vue` | 显示通道基础状态 | EAN 启用时显示 EAN 徽标（"EAN 已启用" + Capability 数量） |
| `useEan.js` | `saveSettings` 调用 `PUT /api/capability/settings` | 移除 `saveSettings`，EAN 状态从北向通道 store 派生 |

**北向通道弹窗 Tab 结构**：

```
┌─ EdgeOS(MQTT) 配置 ──────────────────────────────────────┐
│                                                           │
│  [基础配置]  [设备映射]  [EAN 能力层]                     │
│                                                           │
│  ── EAN 能力层 ──                                         │
│                                                           │
│  启用 EAN 能力层        [  ON  ]                         │
│  心跳间隔 (秒)          [  60  ]                          │
│  事件自动发布           [  ON  ]                          │
│                                                           │
│  ── 运行状态（只读）──                                    │
│  Agent ID:          edgex-node-001                       │
│  Capability 数量:   63                                    │
│  心跳状态:          正常 (最近 2s 前)                    │
│  MCP Runtime:       始终运行（独立于此开关）              │
│                                                           │
│  说明: EAN 启用后，EdgeOS 可远程调用本设备 Capability。   │
│        MCP Runtime 不受此开关影响，始终可用。              │
│                                                           │
└───────────────────────────────────────────────────────────┘
```

**AI 助手 "EAN 接入" Tab 降级后**：

```
┌─ EAN 接入（只读状态）────────────────────────────────────┐
│                                                           │
│  ┌─ 状态卡片 ─────────────────────────────────────────┐  │
│  │  EAN 能力层:    未启用 / 已启用                    │  │
│  │  绑定通道:      EdgeOS(MQTT) - edgex-node-001     │  │
│  │  Agent ID:      edgex-node-001                    │  │
│  │  Capability:    63 条                              │  │
│  │  心跳状态:      正常 / 未运行                       │  │
│  └───────────────────────────────────────────────────┘  │
│                                                           │
│  ┌─ MCP Runtime（始终运行）──────────────────────────┐  │
│  │  状态:          运行中                              │  │
│  │  工具数量:      ~34 个（9 ean_* unified + 25 edgex_*） │  │
│  │  说明: MCP Runtime 独立于 EAN，始终可用             │  │
│  └───────────────────────────────────────────────────┘  │
│                                                           │
│  [前往北向通道配置 →]                                     │
│                                                           │
└───────────────────────────────────────────────────────────┘
```

### 4.6 API 调整

| API | 当前 | 变更 |
|---|---|---|
| `POST /api/northbound/edgeos-mqtt` | 请求体无 EAN 字段 | 新增 `ean_enabled`、`ean_heartbeat_sec`、`ean_event_auto_publish` |
| `PUT /api/northbound/edgeos-mqtt/:id` | 同上 | 同上 |
| `POST /api/northbound/edgeos-nats` | 同上 | 同上 |
| `PUT /api/northbound/edgeos-nats/:id` | 同上 | 同上 |
| `GET /api/capability/settings` | 从北向配置派生 | 保留，增加 EAN 字段，标注来源通道 |
| `PUT /api/capability/settings` | 存根，不持久化 | **移除**，EAN 配置通过北向 API 提交 |
| `GET /api/capability/agent/status` | 返回 Agent 状态 | 保留，MCP Runtime 始终返回；北向 Runtime 按 `EANEnabled` 返回 |
| `GET /api/capability/list` | 返回 7 条 Unified Capability | 保留，MCP Runtime 提供（`GenerateUnifiedCapabilities`） |

### 4.7 Runtime 生命周期状态机

**MCP Runtime**（Server 级别，无状态转换）：

```
Server 启动 → ensureCapabilityRuntime()（sync.Once）→ 运行中 → Server 关闭
```

MCP Runtime 无启停状态转换，生命周期与 Server 进程绑定。`sync.Once` 保证只创建一次，不可停止或重建。

**北向 EAN Runtime**（通道级别，四态转换）：

```
                    ┌─────────┐
                    │  Idle   │ ← EANEnabled=false 或 通道未连接
                    └────┬────┘
                         │ 通道连接 + EANEnabled=true
                         ▼
                    ┌─────────┐
         ┌────────→ │ Running │ ←─────────┐
         │          └────┬────┘           │
         │               │                │
    EANEnabled       MQTT 断开        EANEnabled
    false→true           │            true→false
         │               ▼                │
         │          ┌─────────┐           │
         └───────── │ Stopped │ ──────────┘
                    └────┬────┘
                         │ MQTT 重连
                         ▼
                    检查 EANEnabled:
                    true → Running
                    false → Idle
```

| 状态 | 触发条件 | Runtime 实例 | Discovery 发布 | Invoke 订阅 |
|---|---|---|---|---|
| **Idle** | `EANEnabled=false` 或通道未创建 | nil | 无 | 无 |
| **Running** | 通道已连接 + `EANEnabled=true` | 活跃 | retained + 60s 周期 | 活跃 |
| **Stopped** | 通道连接中断（从 Running 转入） | 保留 | 停止 | 停止 |
| **Stopped → Running** | MQTT 重连 + `EANEnabled=true` | 复用 | 重新发布 retained | 重新订阅 |

### 4.8 MCP Runtime 独立工作场景

MCP Runtime 不依赖任何北向通道，以下场景在 EAN 未启用时完全可用：

| 场景 | 调用路径 | 示例 |
|---|---|---|
| LLM 设备读写 | LLM → MCP `tools/call` → `ean_unified.write_point` → `inferDriverCommand` → DriverExecutor → Southbound | AI 助手写入 Modbus 寄存器 |
| LLM 协议逆向 | LLM → MCP `tools/call` → `ean_unified.protocol_reverse` → AI 模块 | AI 分析未知协议数据帧 |
| LLM 文档解析 | LLM → MCP `tools/call` → `ean_unified.doc_parse` → AI 模块 | AI 解析设备说明书 PDF |
| 本地 HTTP Invoke | HTTP → `/api/capability/invoke` → MCP Runtime → `inferDriverCommand` → DriverExecutor | 前端直接调用 Capability |
| 设备扫描 | LLM → MCP `tools/call` → `ean_unified.scan_devices` → `inferDriverCommand` → DriverExecutor | AI 扫描 Modbus 总线设备 |
| 系统诊断 | LLM → MCP `tools/call` → `ean_unified.diagnostics` → Runtime | AI 查询通道和设备状态 |

**关键区别**：MCP Runtime 的调用是 in-process（进程内），不经过 MQTT/NATS 网络；北向 EAN Runtime 的调用经过 EdgeOS Governance 权限校验和 MQTT 消息传输。两者共享同一套 Capability 定义和 Execution Mapper，但传输层和安全边界完全独立。

---

## 5. EdgeX 侧状态

### 5.1 双 Runtime 架构（战略定位）

EdgeX 内部保留两个独立的 EAN Capability Runtime 实例，这是架构设计而非临时方案。两者面向不同场景，职责互补：

| Runtime | 战略定位 | 创建位置 | Transport | Bus | 心跳间隔 | 目标用户 |
|---|---|---|---|---|---|---|
| **MCP Runtime** | **基础能力层**：独立工作，LLM 接入即可完成设备读写、协议逆向、文档解析等本地智能操作 | `mcp_handler.go:2301` `ensureCapabilityRuntime()` | `TransportSDK` | `NoopBus{}` | 60s | AI 助手 / MCP 客户端 / LLM Agent |
| **北向 EAN Runtime** | **高级协作层**：通过 MQTT/NATS 与 EdgeOS 进行跨系统 Agent 协作，支持远程发现、分布式调用、事件广播 | `edgos_mqtt/ean_bridge.go:71` `EnsureCapabilityRuntime()` | `TransportMQTT` | `mqttBus` | 30s（硬编码，待可配置） | EdgeOS 平台 / 远程编排 / 多 Agent 网络 |

**保留双 Runtime 的四个理由：**

1. **MCP Runtime 零依赖启动**：`sync.Once` 单例，Server 启动即可用。不依赖 MQTT/NATS 连接，不需要北向通道配置。LLM 接入后通过 MCP `tools/call` 即可调用 7 条统一 Capability（`GenerateUnifiedCapabilities` 生成，含 `ean_unified.read_point`、`ean_unified.write_point`、`ean_unified.protocol_reverse`、`ean_unified.doc_parse` 等），无需 EdgeOS 参与。这意味着即使用户只配置了南向设备通道、未创建任何北向通道，AI 助手依然可以完成设备读写和智能分析。可通过 `RuntimeConfig.Unified` 控制统一模式开关。
2. **北向 EAN Runtime 按需启动**：依赖北向通道连接，仅在 `EdgeOS(MQTT/NATS)` 通道 `Enable=true` 且 `EANEnabled=true` 时启动。这是面向 EdgeOS 平台远程编排的高级功能，不是 EdgeX 的基础功能。用户不启用 EAN 时，EdgeX 的本地 AI 能力不受影响。
3. **故障隔离**：MQTT 连接闪断不影响 MCP 本地调用——AI 助手仍可通过 MCP 工具操作设备；MCP Runtime 故障不影响跨系统 EAN 通道——EdgeOS 仍可远程调用 Capability。
4. **安全边界**：MCP Runtime 的 in-process 调用不经过网络，不存在跨系统权限问题；北向 EAN Runtime 的 Invoke 经过 EdgeOS Governance 权限校验（read/write/admin/ai 四级权限 + 租户策略）。

**共享层与独立层：**

| 层次 | 共享/独立 | 说明 |
|---|---|---|
| Capability 定义 | **共享基础，模式不同** | 北向 EAN Runtime：`generator.go` 生成 63 条；MCP Runtime：`GenerateUnifiedCapabilities` 生成 7 条 Unified（`RuntimeConfig.Unified` 控制） |
| Execution Mapper | **共享** | `CapabilityMapper` → `DriverExecutor` → `Southbound`，调用路径完全一致 |
| Transport / Bus | **独立** | MCP: `TransportSDK` + `NoopBus`；北向: `TransportMQTT` + `mqttBus` |
| 生命周期管理 | **独立** | MCP: `sync.Once`（进程级）；北向: 通道级，随连接和 `EANEnabled` 变化 |
| 安全边界 | **独立** | MCP: 进程内调用无权限校验；北向: EdgeOS Governance 权限校验 |

```
┌─ EdgeX 进程 ──────────────────────────────────────────────┐
│                                                            │
│  ┌─ MCP Runtime（基础层，Server 启动即就绪）────────────┐ │
│  │  TransportSDK + NoopBus                               │ │
│  │  ├── 7 Unified Capability（`GenerateUnifiedCapabilities`） │ │
│  │  ├── MCP 工具注册（~34 个，9 `ean_*` unified + 25 `edgex_*`） │ │
│  │  ├── /api/capability/invoke（本地 HTTP）              │ │
│  │  └── LLM 接入 → 智能设备操作                          │ │
│  └───────────────────────────────────────────────────────┘ │
│                                                            │
│  ┌─ 北向 EAN Runtime（高级层，依赖通道连接）────────────┐ │
│  │  TransportMQTT + mqttBus                              │ │
│  │  ├── 63 Capability（共享定义）                        │ │
│  │  ├── $edgeos/discovery/*（retained 发布）             │ │
│  │  ├── $edgeos/invoke/* ← EdgeOS 远程调用              │ │
│  │  ├── $edgeos/reply/* → EdgeOS 响应                   │ │
│  │  └── $edgeos/heartbeat/*（周期心跳）                  │ │
│  └───────────────────────────────────────────────────────┘ │
│                                                            │
│  ┌─ 共享执行内核 ──────────────────────────────────────┐  │
│  │  CapabilityMapper → DriverExecutor → Southbound      │  │
│  └─────────────────────────────────────────────────────┘  │
│                                                            │
└────────────────────────────────────────────────────────────┘
```

### 5.2 北向通道（`edgos_mqtt` / `edgos_nats`）

| 调整项 | 状态 | 说明 |
|---|---|---|
| `EdgeOSMQTTConfig` / `EdgeOSNATSConfig` 新增 EAN 字段 | **已落地** | `EANEnabled` / `EANHeartbeatSec` / `EANEventAutoPublish` 已在 `types.go` |
| 北向通道 `OnConnect` 中启动 Runtime | **已实现** | `client.go:305` 无条件调用 `EnsureCapabilityRuntime()` + `startEANLocked()` |
| EAN 启停控制 | **已落地** | `EnsureCapabilityRuntime` 检查 `EANEnabled`；热更新 true↔false 启停 Runtime |
| EAN 心跳间隔配置 | **未实现** | 硬编码 `HeartbeatIntervalSec: 30`（`ean_bridge.go:87`），不可配置 |
| EAN Runtime 停止方法 | **未实现** | 无 `StopCapabilityRuntime()` 方法，通道断连时 Runtime 仅暂停而非显式停止 |
| V1 命令处理标记 deprecated | **未实现** | 无 DEPRECATED 日志告警 |
| 配置热更新支持 EAN 启停 | **未实现** | `updateEdgeOSMQTTClients` 仅处理连接参数，不检测 EAN 字段变化 |
| 数据上报路径保留 | **已实现** | `PublishRealtimeData()` -> `edgex/data/*` 不变 |
| 设备映射保留 | **已实现** | `Devices` 字段及上报策略不变 |

### 5.3 北向 EAN Runtime（已完成）

| 调整项 | 状态 | 说明 |
|---|---|---|
| Runtime 创建与 MQTT bus 绑定 | 已完成 | `mqttBus{client: c}` 包装真实 MQTT 客户端 |
| Runtime 启停 | 已完成 | 随 MQTT 连接 `OnConnect` 自动启动，重连时重新发布 discovery |
| `heartbeatLoop` 周期性发布 Capability | 已完成 | 每 60s 重发 Capability Descriptor |
| discovery retained 消息 | 已完成 | `$edgeos/discovery/*` 自动 retained |
| Invoke 消息订阅与分发 | 已完成 | `handleInvokeMessage` -> `Dispatcher` -> `CapabilityMapper` |
| Reply 消息发布 | 已完成 | `$edgeos/reply/{source_agent_id}` |
| `scan_devices` required 修复 | 已完成 | `channel_id` 标记 required |
| 全部 Capability description | 已完成 | 63 条双语 description + property description |
| HandleDiscoveryQuery | 已完成 | 响应 EdgeOS 主动查询 |

### 5.4 MCP Runtime（已完成，独立基础能力层）

MCP Runtime 是 EdgeX 的基础能力层，Server 启动即就绪，不依赖任何北向通道。

| 调整项 | 状态 | 说明 |
|---|---|---|
| `ensureCapabilityRuntime` 创建 | 已完成 | `mcp_handler.go:2301`，`sync.Once` 单例，`TransportSDK` + `NoopBus`，心跳 60s |
| AgentID 来源 | 已完成 | 优先取北向配置 `EdgeOSMQTT[0].NodeID`，无北向配置时默认 `"edgex"` |
| MCP 工具注册 | 已完成 | ~32 个工具（7 `ean_*` unified + 25 `edgex_*`）注册到 MCP Server，6 个重叠工具已移除（`read_point`、`read_point_batch`、`write_point`、`write_point_batch`、`list_points`、`get_diagnostics`）。v2.14：15 协议×4 操作矩阵合并为 4 通用工具，总数从 94 降至 32 |
| 本地 HTTP Invoke | 已完成 | `/api/capability/invoke` 走 MCP Runtime in-process 调用 |
| MCP `tools/list` | 已完成 | JSON-RPC over HTTP POST，返回 ~34 个工具描述 |
| MCP `tools/call` | 已完成 | LLM 接入后可直接调用 7 条统一 Capability（`GenerateUnifiedCapabilities`），`execution/capability_mapper.go` 的 `inferDriverCommand` 负责映射到具体驱动命令 |
| EAN 设置 GET | 已完成 | 从北向配置派生状态展示 |
| EAN 设置 PUT | **存根** | `handleEanSettingsUpdate` 仅返回确认，不持久化任何字段（Phase 3 移除） |

**MCP Runtime 独立工作验证**：在未创建任何北向通道的场景下，`ensureCapabilityRuntime` 仍可通过 `sync.Once` 创建 Runtime 实例。AgentID 取默认值 `"edgex"`，LLM 接入后即可通过 MCP `tools/call` 调用 `ean_unified.read_point`、`ean_unified.write_point`、`ean_unified.protocol_reverse`、`ean_unified.doc_parse` 等 7 条统一 Capability（`GenerateUnifiedCapabilities` 生成）。调用路径为 in-process，不经过 MQTT/NATS 网络。`RuntimeConfig.Unified` 控制统一模式，`execution/capability_mapper.go` 的 `inferDriverCommand` 完成协议适配。

### 5.5 V1 命令处理（运行中，未标记 deprecated）

| V1 Handler | 状态 | Phase 3 动作 |
|---|---|---|
| `handleWriteCommand` | 运行中 | 标记 `DEPRECATED` 日志，引导使用 EAN `*.write_register` |
| `handleDiscoverCommand` | 运行中 | 标记 `DEPRECATED` 日志，引导使用 EAN `*.scan_devices` |
| `handleTaskCommand` | 运行中 | 标记 `DEPRECATED` 日志 |
| `handleNodeRegisterCommand` | 运行中 | 保留（EAN discovery 已覆盖，但 V1 节点注册兼容旧版本） |
| `subscribeToCommands` | 运行中 | 保留 V1 订阅，不再新增命令类型 |

### 5.6 Phase 3 EdgeX 必须完成功能

#### P0 优先级（阻塞 EAN 合并）

| 编号 | 功能 | 说明 | 涉及文件 | 关联设计 |
|---|---|---|---|---|
| EX-P3-01 | V1 命令处理标记 DEPRECATED | `handleWriteCommand` / `handleDiscoverCommand` / `handleTaskCommand` 增加 WARN 级 `DEPRECATED` 日志，提示迁移到 EAN Capability。不移除功能，仅告警 | `edgos_mqtt/client.go` / `edgos_nats/client.go` | §5.5 |
| EX-P3-02 | EAN 启用开关加入通道配置 | `EdgeOSMQTTConfig`（`types.go:421`）/ `EdgeOSNATSConfig`（`types.go:443`）追加 `EANEnabled bool` 字段；`EnsureCapabilityRuntime`（`ean_bridge.go:71`）读取该字段，为 false 时返回 nil 不创建 Runtime；`OnConnect`（`client.go:305`）检查返回值，nil 时跳过 `startEANLocked` | `model/types.go` / `edgos_mqtt/ean_bridge.go` / `edgos_nats/ean_bridge.go` / `edgos_mqtt/client.go` / `edgos_nats/client.go` | §4.2 §4.3.1 §4.3.2 |
| EX-P3-03 | EAN 心跳间隔可配置 | 追加 `EANHeartbeatSec int` 字段，传入 `RuntimeConfig.HeartbeatIntervalSec`，替代 `ean_bridge.go:87` 硬编码 30；为 0 时默认 60 | `model/types.go` / `edgos_mqtt/ean_bridge.go` / `edgos_nats/ean_bridge.go` | §4.2 §4.3.1 |
| EX-P3-04 | 移除 EAN 设置 PUT 存根 | 移除 `PUT /api/capability/settings` 路由和 `handleEanSettingsUpdate` handler；EAN 配置通过 `POST /api/northbound/edgeos-mqtt` 提交，由北向管理器持久化 | `capability_handler.go` / `server.go` | §4.6 |

#### P1 优先级（完善 EAN 合并体验）

| 编号 | 功能 | 说明 | 涉及文件 | 关联设计 |
|---|---|---|---|---|
| EX-P3-05 | 北向通道弹窗新增 EAN 能力层 Tab | `EdgeOSMQTTSettingsDialog.vue` / `EdgeOSNATSSettingsDialog.vue` 新增 "EAN 能力层" Tab：启用开关、心跳间隔输入、事件自动发布开关、只读状态展示（Agent ID、Capability 数量、心跳状态、MCP Runtime 状态） | `ui/src/components/northbound/` | §4.5 |
| EX-P3-06 | AI 助手 EAN Tab 降级为只读 | `AiSettingsDialog.vue` 移除启停控制，改为状态卡片（EAN 状态 + MCP Runtime 状态）+ "前往北向通道配置" 跳转按钮；`useEan.js` 移除 `saveSettings`，EAN 状态从北向通道 store 派生 | `ui/src/components/ai-assistant/` / `ui/src/composables/useEan.js` | §4.5 |
| EX-P3-07 | EAN Invoke metrics 采集 | 北向 EAN Runtime 增加 Invoke 延迟（P50/P99）、成功率、失败原因统计；供北向通道状态卡片和 `GET /api/capability/agent/status` 展示 | `edgos_mqtt/ean_bridge.go` / `capability/runtime.go` | - |
| EX-P3-08 | 配置热更新支持 EAN 启停 | `updateEdgeOSMQTTClients`（`northbound_manager_edgos.go`）新增 EAN 字段 diff 检测：`EANEnabled` false→true 且通道已连接时启动 Runtime；true→false 时调用 `StopCapabilityRuntime()` 停止；`EANHeartbeatSec` 变化时重启 Runtime。NATS 对称改造 | `northbound_manager_edgos.go` / `northbound_manager_edgos_nats.go` | §4.4 |
| EX-P3-09 | 新增 `StopCapabilityRuntime` 方法 | `edgos_mqtt/ean_bridge.go` / `edgos_nats/ean_bridge.go` 新增：调用 `rt.Stop()` + 置 `c.eanRuntime = nil` + 清理 `$edgeos/invoke/*` 订阅；供热更新和通道禁用时调用 | `edgos_mqtt/ean_bridge.go` / `edgos_nats/ean_bridge.go` | §4.3.3 §4.7 |

**已移除的任务（v2.5 调整）：**

| 原编号 | 原任务 | 移除原因 |
|---|---|---|
| EX-P3-08 (旧) | 双 Runtime 一致性验证，考虑合并为单 Runtime | 双 Runtime 是战略设计：MCP 独立基础能力 + EAN 高级协作功能，不合并。一致性由共享 Capability 定义和 Execution Mapper 保证 |

---

## 6. EdgeOS 侧状态

### 6.0 与 EdgeX 双 Runtime 的对接约定（EdgeOS 必须遵守）

> 交叉引用：EdgeX 双 Runtime 详见 **§5.1**。EdgeOS **只消费北向 EAN Runtime**，不得把 MCP Runtime 当作跨系统总线。

| 对接对象 | EdgeX 位置 | Transport / Bus | EdgeOS 行为 |
|---|---|---|---|
| **正式跨系统通道** | `edgos_mqtt/ean_bridge.go`（及 NATS 对称实现） | `TransportMQTT` + **mqttBus** | 订阅/发布 `$edgeos/discovery/*`、`$edgeos/invoke/*`、`$edgeos/reply/*`、`$edgeos/heartbeat/*`、`$edgeos/event/*` |
| **禁止误对接** | `mcp_handler.go` `ensureCapabilityRuntime()` | `TransportSDK` + `NoopBus{}` | **不**经 EdgeX 本地 HTTP `/api/capability/*` 或 MCP 工具枚举做跨系统发现；该路径仅 EdgeX 进程内 |

**验收含义**：EdgeOS 索引中的原生 Capability（`system.diagnostics`、`ai.*`、协议类）必须来自 MQTT/NATS 北向 discovery 信封，而非 V1 Bridge 合成，也非 MCP Runtime。

### 6.1 V1 Bridge（已隔离，非移除）

**代码位置**：`internal/ean/bridge.go`

| 调整项 | 当前实现 | 目标 | 优先级 |
|---|---|---|---|
| V1 Capability 合成 | **已隔离**：`HasNativeEANCaps()` **或** `HasNativeEANAgent()` 为真时跳过 `syncDevice()` | Phase 4 完全移除 | P2 |
| Agent 同步 | 无原生 Agent 时才 `HandleAgentOnline(..., "v1-bridge")`；已有 `native-ean` Agent 时**跳过**（由 DiscoveryCenter 拒绝覆盖） | Phase 4 移除 | P2 |
| 点位数据桥接 | 5s 轮询，`syncPointData()` → `$edgeos/event/{node}`，含 `previous_value` | Phase 4 移除 | P2 |
| 心跳模拟 | 仍可 `HandleHeartbeat` 兜底；原生心跳到达后不覆盖 Agent 描述符 | Phase 4 移除 | P2 |

**隔离规则**（`discovery.go`）：
- Cap：`native-ean` 拒绝同名 `v1-bridge` 覆盖；`native-ean` 可升级覆盖同名 `v1-bridge`
- Cap：**首条原生 Cap 到达时 `purgeV1BridgeCapsLocked`**，清除该 Agent 下全部残留 `…/read-write`（ID 不同，仅靠同名覆盖不够）
- Agent：`agentSources` 标记；已有 `native-ean` 时拒绝 `v1-bridge` 覆盖（避免 kind 被改回 `edgex-gateway`）
- 点位 Event 同步不受 Cap 隔离影响

### 6.2 Invoke 编排与 V1 Fallback

**代码位置**：`internal/ean/invoke.go`、`internal/ean/v1_invoke_bridge.go`、`internal/ean/bus.go`

#### Invoke 编排器（`invoke.go`）

| 调整项 | 当前实现 | 目标 | 优先级 |
|---|---|---|---|
| Invoke 发布 | 构造 `Message` 信封（`message_type=invoke_capability`）→ `$edgeos/invoke/{target}`（北向 mqttBus） | 不变 | - |
| Reply 关联 | 订阅 `$edgeos/reply/{plannerID}`，`correlation_id` / `invoke_id` 关联 | 不变 | - |
| 超时处理 | `context.WithTimeout`，超时清理 pending | 不变 | - |

#### V1 Fallback（`v1_invoke_bridge.go` + `bus.go`）

| 调整项 | 当前实现 | 目标 | 优先级 |
|---|---|---|---|
| 原生 EAN Cap | **100% MQTT Invoke**，全额超时；**禁止** V1 Fallback（`system.diagnostics` / `ai.*` / 协议类） | 保持 | P0 |
| V1 合成 Cap Fallback | **仅** `source=v1-bridge` 或 ID 后缀 `/read-write`：超时/rejected/error 时 40%/60% 分配并 `InvokeViaV1` | Phase 3 移除 | P0 |
| Agent Kind 过滤 | V1 合成路径仍要求 `edgex-gateway` / `edgex` | Phase 3 移除 | P0 |
| ID / 路由隔离 | 真实 EAN ID 与 V1 伪 ID 分路，互不混用 | 保持 | P0 |

**V1 Fallback 竞态修复**（已实现，仅作用于 V1 合成 Cap）：
- `RegisterPending` → `PublishV1Command` → `WaitForChannel` 严格顺序

### 6.3 Messaging Manager（V1 Topic 订阅）

**代码位置**：`internal/messaging/manager.go`

| V1 Topic | 订阅模式 | Handler | 当前状态 | 计划 |
|---|---|---|---|---|
| `edgex/nodes/register` | 精确 | `nodeHandler.HandleRegister` | **保留**（V1 节点注册兼容） | Phase 4 移除 |
| `edgex/nodes/+/heartbeat` | 通配 | `nodeHandler.HandleHeartbeat` | **保留**（V1 心跳兼容） | Phase 4 移除 |
| `edgex/nodes/+/status` | 通配 | `nodeHandler.HandleHeartbeat` | **保留**（V1 状态兼容） | Phase 4 移除 |
| `edgex/nodes/unregister` | 精确 | `nodeHandler.HandleUnregister` | **保留**（V1 注销兼容） | Phase 4 移除 |
| `edgex/devices/report` | 精确 | `deviceHandler.HandleDeviceReport` | **保留**，V1 数据上报路径不迁移 | 长期保留 |
| `edgex/devices/+/+/online` | 通配 | `deviceHandler.HandleDeviceOnline` | **保留**，设备状态通过 V1 pub/sub | 长期保留 |
| `edgex/devices/+/+/offline` | 通配 | `deviceHandler.HandleDeviceOffline` | **保留**，设备状态通过 V1 pub/sub | 长期保留 |
| `edgex/points/report` | 精确 | `pointHandler.HandlePointReport` | **保留**，全量同步效率高于按需查询 | 长期保留 |
| `edgex/points/+/+` | 通配 | `pointHandler.HandlePointSync` | **保留**，点位元数据同步 | 长期保留 |
| `edgex/data/+/+` | 通配 | `pointHandler.HandleRealtimeData` | **保留**，实时数据批量推送 | 长期保留 |
| `edgex/events/alert` | 精确 | `handleAlert` | **保留**，外部监控系统依赖 | 长期保留 |
| `edgex/events/error` | 精确 | `handleAlert` | **保留**，外部监控系统依赖 | 长期保留 |
| `edgex/events/info` | 精确 | `handleAlert` | **保留**，外部监控系统依赖 | 长期保留 |
| `edgex/cmd/responses/#` | 通配 | `controlHandler.HandleCommandResponse` | **保留**，V1 Fallback 响应通道 | Phase 3 移除 |

**注意**：`edgex/cmd/responses/#` 同时服务于 V1 Fallback 机制。`HandleCommandResponse` 在 `UpdateCommandStatus` 后调用 `HandleResponse`，通知 pending channel。Phase 3 移除 V1 Fallback 后此订阅一并移除。

### 6.4 Discovery Center（北向 mqttBus 消费端）

**代码位置**：`internal/ean/discovery.go`

| 功能 | 实现 | 状态 |
|---|---|---|
| Topic 订阅 | `$edgeos/discovery/agent`、`…/capability`、`…/response`、`…/agent/offline` | P0（已实现） |
| CapSource / AgentSource | `native-ean`（mqtt/nats 北向）/ `v1-bridge`；Agent 与 Cap 双维度隔离 | P0（已实现） |
| `upsertCapability` + purge | 同名优先级 + **原生到达清除该 Agent 全部 v1 Cap** | P0（已实现） |
| `HasNativeEANCaps` / `HasNativeEANAgent` | 供 V1 Bridge 跳过合成与 Agent 覆盖 | P0（已实现） |
| `HandleDiscoveryResponse` | 解析北向 `discovery_response` 信封，Agent/Cap 均标 `native-ean` | P0（已实现） |
| 信封兼容 | EdgeX `NewEnvelope`：`body.capabilities[]` / `body.agent`；`transport` string/[]string；`metadata` 非 string 字符串化 | P0（已实现） |
| 主动 Query | Bus `discoveryQueryLoop`：2s → 30s → 原生后 5min | P0（已实现） |
| 可观测性 | `CountCapabilitiesBySource`；API/Health 暴露 `native_ean_caps` / `v1_bridge_caps` | P1（已实现） |

### 6.5 Event Center

**代码位置**：`internal/ean/event.go`

| 功能 | 实现 | 状态 |
|---|---|---|
| 点位变化事件 | `handlePointChange()` 读取 `value` 和 `previous_value`，首次 occurrence `previous_value` 为 nil | P0（已实现） |
| 设备上下线事件 | `handleDeviceOnline()` / `handleDeviceOffline()` 处理 `device.online` / `device.offline` 事件类型 | P0（已实现） |
| 事件路由 | `EventRule`（点位规则） + `DeviceRule`（设备规则），支持 AgentID/DeviceID/PointID/EventType 过滤 | P0（已实现） |
| 短期缓存 | 环形缓冲（默认 1024 条），`RecentEvents(n)` 按时间倒序查询，`QueryEvents()` 按条件过滤 | P0（已实现） |
| 订阅注册 | `TopicEventBroadcast` + `TopicEventPrefix+"#"`（按节点通配） | P0（已实现） |
| 协议信封兼容 | `unwrapBody()` 解包 `{header, body}` 信封后路由 | P0（已实现） |

### 6.6 Heartbeat Monitor

**代码位置**：`internal/ean/heartbeat.go`

| 功能 | 实现 | 状态 |
|---|---|---|
| 超时判定 | `timeout = timeoutMultiplier * heartbeat_interval_sec`（默认 3x），`checkTimeouts()` 周期检查 | P0（已实现） |
| 检查间隔 | 配置项 `check_interval_sec`（默认 5s），`checkLoop()` 后台 goroutine | P0（已实现） |
| `TouchLastSeen()` | 心跳到达时更新 `metadata.last_seen` 和 `last_heartbeat_seq`，持有写锁避免竞态 | P0（已实现） |
| Agent 离线 | 超时后调用 `discovery.RemoveAgent()` 标记离线 + 清理 Cap 来源标记 | P0（已实现） |
| 序列号检测 | `Sequence` 回退时 Warn 日志（不丢弃，仅告警） | P0（已实现） |
| 回调机制 | `onTimeout` 回调通知上层（Bus 中用于审计记录） | P0（已实现） |
| 订阅注册 | `TopicHeartbeatPrefix+"#"`（所有 Agent 心跳通配） | P0（已实现） |

### 6.7 Governance

**代码位置**：`internal/ean/governance.go`

| 功能 | 实现 | 状态 |
|---|---|---|
| 权限级别 | `read`（默认放行）/ `write` / `admin` / `ai`（需显式 allow） | P0（已实现） |
| 租户策略 | `TenantPolicy` 含 `AllowCap` / `DenyCap` / `AllowTarget` / `DenyTarget` 列表 | P0（已实现） |
| 检查顺序 | deny 优先 -> AI 显式 allow -> allow target -> allow cap | P0（已实现） |
| 通配符支持 | `matchPrefix()` 支持 `*`（全通配）、精确匹配、前缀匹配（如 `ai.` 匹配 `ai.protocol_reverse`） | P0（已实现） |
| 审计记录 | `RecordAudit()` 记录 initiator/target/capability/invokeID/status/tenantID，上限 10000 条 | P0（已实现） |
| 审计查询 | `QueryAuditRecords()` 按条件过滤，倒序返回 | P0（已实现） |
| 异步回调 | `OnAudit` 回调支持异步落库 | P0（已实现） |

### 6.8 EAN Bus 与双传输

**代码位置**：`internal/ean/bus.go`、`internal/ean/transport.go`、`internal/ean/envelope.go`

| 功能 | 实现 | 状态 |
|---|---|---|
| 双传输 | `DualTransport` 支持 MQTT + NATS 并行，`Add()` 注册传输层，`ConnectedNames()` 返回已连接列表；`Details()` 返回 `transport_details` | P0（已实现） |
| 降级运行 | MQTT/NATS broker 不可用时 **不 fatal**：创建传输实例 + 后台重连（MQTT `ConnectRetry`/`AutoReconnect`，NATS `RetryOnFailedConnect`）；订阅本地登记，OnConnect/Reconnect 补订；与 messaging.Manager 一致 | P0（已实现，v2.7 MQTT / v2.10 NATS 延迟订阅对称） |
| 主动 Discovery Query | `discoveryQueryLoop()`：延迟 2s 首发 -> 30s 周期 -> 收到原生 EAN Cap 后降为 5min | P0（已实现） |
| 子系统回调绑定 | `wireCallbacks()`：AgentOnline->心跳初始化、AgentOffline->心跳清理、HeartbeatTimeout->审计记录 | P0（已实现） |
| 编排 API | `InvokeCapability()`：在线检查 → Cap 查找 → 权限 → 审计 → MQTT Invoke；**仅 V1 合成 Cap** 可 Fallback | P0（已实现） |
| 健康状态 | `Health()` 含 transports/registered_transports/online_agents + `native_ean_caps` / `v1_bridge_caps` / `northbound_runtime=mqttBus` + `invoke_metrics` | P0（已实现，v2.7） |
| 生命周期 | `Start()` 注册订阅 + 心跳 + Discovery Query（订阅失败仅 Warn）；`Stop()` 优雅关闭 | P0（已实现） |

### 6.9 EAN API 路由

**代码位置**：`internal/server/ean_routes.go`

| API | 方法 | 功能 | 状态 |
|---|---|---|---|
| `/api/ean/agents` | GET | 列出所有 Agent | 运行中 |
| `/api/ean/agents/:id` | GET | 获取指定 Agent 详情 | 运行中 |
| `/api/ean/agents/:id/capabilities` | GET | 列出 Cap；附带每条 `source` 及 `native_ean_caps` / `v1_bridge_caps` | 运行中 |
| `/api/ean/invoke` | POST | 调用 Cap（原生走北向 MQTT；V1 合成才可 Fallback） | 运行中 |
| `/api/ean/events/recent` | GET | 查询最近 N 条事件（默认 100） | 运行中 |
| `/api/ean/audit` | GET | 查询审计记录（默认 100 条） | 运行中 |
| `/api/ean/governance/policies` | POST | 设置租户策略 | 运行中 |
| `/api/ean/health` | GET | EAN Bus 健康状态（含原生/V1 Cap 计数） | 运行中 |

**注意**：所有 EAN 路由受 JWT 保护。`eanBus` 为 nil 时返回 503 Service Unavailable。

### 6.10 EAN 配置

**代码位置**：`internal/config/ean_config.go`、`config/config.yaml`

当前 EAN 配置为独立段（`ean:`），与北向通道配置（`middlewares:`）分离。合并设计见第 4 章。

| 配置项 | 当前值 | 说明 |
|---|---|---|
| `ean.enabled` | `true`（`config.yaml`；`DefaultEANConfig` 仍为 false） | EAN Bus 启用开关 |
| `ean.planner_id` | `edgeos-planner` | Invoke reply topic 路由标识 |
| `ean.mqtt.broker` | `tcp://127.0.0.1:18083` | MQTT broker（默认已对齐联调端口；不再默认 1883） |
| `ean.mqtt.qos` | `1` | QoS 级别 |
| `ean.nats.enabled` | `true`（`config.yaml` 联调；`DefaultEANConfig` 仍为 false） | NATS 传输层；与 MQTT 对称订阅/发布同一 `$edgeos/...` |
| `ean.nats.url` | `nats://127.0.0.1:4222` | NATS Server |
| `ean.heartbeat.check_interval_sec` | `5` | 心跳检查间隔 |
| `ean.heartbeat.timeout_multiplier` | `3` | 超时倍数（3 个心跳周期） |

### 6.11 EdgeOS 验收标准与 EdgeX 阻塞项

| # | 验收项 | 通过条件 |
|---|---|---|
| OS-A1 | 北向 Capability 发现 | 收到 `$edgeos/discovery/capability` 或 `discovery/response` 后索引含原生 ID，且 `source=native-ean` |
| OS-A2 | 无 V1 污染 | 存在原生 Cap 时该 Agent `v1_bridge_caps == 0` |
| OS-A3 | 跨系统 Invoke | `POST /api/ean/invoke` `system.diagnostics` 走北向 Runtime 成功 |
| OS-A4 | 路由隔离 | 原生 Cap 失败/超时 **不** 调用 `InvokeViaV1` |
| OS-A5 | 单元测试 | `go test ./internal/ean/...` 含北向信封 fixtures（PASS） |

**EdgeX 侧阻塞（非 EdgeOS 缺陷）**：若监听窗口无 `$edgeos/discovery/capability` 流量、或仅 MCP Runtime 有 63 Cap，属北向 Runtime 未发布/Query 未响应——EdgeOS 已按线格式就绪，待 EdgeX §5.3 行为持续生效。

---

## 7. 联合测试验证结果

### 7.1 测试环境

| 组件 | 版本/配置 |
|---|---|
| EdgeX | `d:\code\edgex`，运行中 |
| EdgeOS | `d:\code\edgeOS`，运行中 |
| MQTT Broker | `127.0.0.1:18083` |
| NATS Server | `127.0.0.1:4222` |
| EdgeX AgentID | `edgex-node-001` |
| EdgeOS PlannerID | `edgeos-planner` |
| EdgeX NATS 北向通道 | `EAN-NATS`（`ean_enabled=true`，`node_id=edgex-node-001`） |
| EdgeOS NATS 传输 | `ean.nats.enabled=true`（`url=nats://127.0.0.1:4222`，`client_name=edgeos-ean`） |

### 7.2 编译验证

| 项目 | 结果 |
|---|---|
| EdgeX `go build ./...` | 通过 |
| EdgeX `go test ./internal/capability/...` | PASS |
| EdgeOS `go build ./cmd/` | 通过（v2.7） |
| EdgeOS `go test ./internal/ean/...` | **PASS**（含启动韧性/不可达 broker 用例，v2.7） |
| EdgeOS `go test ./internal/messaging/ ./internal/server/` | **PASS**（v2.7） |
| EdgeOS UI `npm run build` | **PASS**（v2.7） |

### 7.3 API 测试

| 测试项 | 请求 | 结果 |
|---|---|---|
| EdgeOS 登录 | `POST /api/auth/login` | token 获取成功 |
| EAN Agent 查询 | `GET /api/ean/agents` | `edgex-node-001` online |
| Capability 索引 | `GET /api/ean/agents/edgex-node-001/capabilities` | **63 条 `native-ean`**（含 `system.diagnostics` / `ai.*` / 协议类）；EdgeOS v2.3 起原生到达后应 `v1_bridge_caps=0`（需重启 EdgeOS 加载新隔离逻辑后复验） |
| scan_devices Invoke | `POST /api/ean/invoke` `modbus_tcp.scan_devices` | **链路贯通**（北向 MQTT），Driver 返回 `does not support scanning`（业务正常） |
| system.diagnostics Invoke | `POST /api/ean/invoke` `system.diagnostics` | **成功**（北向 Runtime），返回通道诊断数据 |
| 审计记录 | `GET /api/ean/audit` | pending → completed/failed 完整流转 |

### 7.4 `system.diagnostics` 返回数据

```json
{
  "channels": [
    {"channel_id": "BACnet", "devices": 3, "protocol": "bacnet-ip"},
    {"channel_id": "ch_0723120128", "devices": 1, "protocol": "modbus-tcp"}
  ],
  "count": 2
}
```

### 7.5 关键修复验证

- `scan_devices` 的 `required: ["channel_id"]` 已正确生效
- `channel_id` property 的 `description: "通道ID，必需 | Channel ID, required"` 已生效
- 全部 63 条 Capability 的双语 Description 和参数说明完整
- V1 Bridge 隔离（EdgeOS v2.3）：原生 Cap 到达后 purge `…/read-write`；原生 Agent 不被 V1 覆盖；原生 Invoke 不走 V1 Fallback
- EdgeOS 单元测试：`go test ./internal/ean/...` 含北向信封 fixtures（PASS）


### 7.6 NATS 传输对称联调（v2.9；v2.10 复验 + 代码对称补齐；v2.11 端到端复验含 EAN Metrics 暴露）

**目标**：验证 EdgeOS NATS 传输与 EdgeX NATS 北向通道的 EAN 2.0 协议对称性，确认 ``$edgeos/*`` Subject 在 NATS 上保持斜杠形式、Discovery/Invoke/Reply/Heartbeat 全链路贯通。

**配置**：

| 侧 | 配置项 | 值 |
|---|---|---|
| EdgeX | NATS 北向通道 ``EAN-NATS`` | ``enable=true``, ``ean_enabled=true``, ``url=nats://127.0.0.1:4222``, ``node_id=edgex-node-001``, ``ean_heartbeat_sec=60`` |
| EdgeOS | ``config.yaml`` ``ean.nats`` | ``enabled=true``, ``url=nats://127.0.0.1:4222``, ``client_name=edgeos-ean``, ``max_reconnects=5`` |

**验证结果（2026-07-28 v2.11 端到端复验）**：

| 测试项 | 请求/操作 | 结果 |
|---|---|---|
| NATS 服务端 | TCP ``127.0.0.1:4222`` | **可达**，双连接 ESTABLISHED（EdgeX PID 30676 + EdgeOS PID 15100） |
| EdgeX NATS 北向状态 | ``GET /api/northbound/config`` -> ``edgeos_nats[0]`` | ``enable=true``, ``ean_enabled=true``, ``name=EAN-NATS``, ``url=nats://127.0.0.1:4222`` |
| EdgeX NATS Stats + EAN Metrics | ``GET /api/northbound/edgeos-nats/:id/stats`` | ``publish_count=7288``, ``ean_metrics.total_invokes=8``, ``success_rate=100%`` |
| EdgeX MQTT Stats + EAN Metrics | ``GET /api/northbound/edgeos-mqtt/:id/stats`` | ``publish_count=4919``, ``ean_metrics.total_invokes=4``, ``success_rate=100%`` |
| EdgeX MCP/本地 status | ``GET /api/capability/agent/status`` | ``online``, ``capabilities_count=7``（MCP Runtime Unified 模式，``transport=sdk``） |
| EdgeOS 双传输注册 | ``GET /api/ean/health`` | ``transports`` 含 mqtt+nats，``registered_transports=2`` |
| NATS Discovery 索引 | ``GET /api/ean/agents`` | ``transport=["nats"]``, ``metadata.northbound="edgeos_nats"``, ``status=online`` |
| NATS Capability 索引 | ``GET /api/ean/agents/edgex-node-001/capabilities`` | **63 条 ``native-ean``**, ``v1_bridge_caps=0`` |
| Invoke: system.diagnostics | ``POST /api/ean/invoke`` | **``completed``** / 2 channels（BACnet 3 devices + ModbusTCP 1 device） |
| Invoke: modbus_tcp.list_points | ``POST /api/ean/invoke`` | **``completed``** / 20 points（HR_40001~HR_40020, quality=Good） |
| Invoke: bacnet_ip.list_points | ``POST /api/ean/invoke`` | **``completed``** / 11 points |
| EdgeOS Invoke Metrics | ``GET /api/ean/health`` -> ``invoke_metrics`` | ``total=8``, ``success=8``, ``success_rate=100%``, ``P50=3ms``, ``P99=6ms``, ``v1_fallback=0`` |
| EdgeOS 审计追踪 | ``GET /api/ean/audit?limit=6`` | 3 组 pending→completed 记录，initiator=edgeos-planner, target=edgex-node-001 |
| EdgeOS 事件监控 | ``GET /api/ean/events/recent?n=5`` | 实时接收点位变化事件（pt_0723121000.changed value=12345 等） |

**v2.10 代码对称补齐（相对 MQTT）**：

- NATS ``Subscribe`` 未连接时仅登记 pending，``ConnectHandler`` / ``ReconnectHandler`` 补订（对齐 MQTT deferred subscribe）
- Health 增加 ``transport_details[{name,connected,endpoint}]``；``northbound_runtime`` 按已注册传输拼为 ``mqttBus+natsBus``
- UI Overview / DebugHelp / Dashboard 分传输展示连接态与 endpoint；联调帮助拆分 MQTT(18083) / NATS(4222) 步骤
- 单测：``TestNewBus_UnreachableNATS_DoesNotFail``、``TestNewNATSTransport_DeferredSubscribe``；``go test ./internal/ean/`` PASS；UI ``npm run build`` PASS

**v2.11 EAN Metrics 暴露对称补齐**：

- EdgeX NATS ``GetStats()`` 返回 ``ean_metrics`` 字段（``EdgeOSNATSStats.EANMetrics``），对齐 MQTT ``EdgeOSMQTTStats.EANMetrics``
- EdgeX MQTT/NATS ``StatsDialog.vue`` 展示 EAN Runtime 指标（Invoke 总数、成功率、P50/P99 延迟、成功/失败/超时计数）
- EdgeX 重新编译重启后，NATS stats API 正确返回 ``ean_metrics.total_invokes=8, success_rate=100%``
- EdgeOS ``northbound_runtime`` 字段当前显示 ``mqttBus``（因 MQTT Agent 先于 NATS 到达 Discovery 索引），不影响 NATS Invoke 双向通信

**对称性结论**：

- NATS Subject 保持 ``$edgeos/...`` 斜杠形式（``mqttTopicToNatsSubject`` 仅转换通配符 ``+ -> *`` / ``# -> >``，不转换分隔符）
- EdgeX NATS 北向 ``natsBus``（``edgos_nats/ean_bridge.go``）正确实现 ``capability.Bus`` 接口，Publish/Subscribe 直接使用原始 Subject
- EdgeOS ``DualTransport`` 同时向 MQTT + NATS 发布 Invoke，EdgeX 双 Runtime 均可响应；EdgeOS ``InvokeOrchestrator`` 通过 ``invoke_id`` 去重，仅接受首个 Reply
- NATS 无 retained 消息机制，但 EdgeOS 主动 Discovery Query（2s 首发 -> 30s -> 5min 降频）+ EdgeX 60s 周期重发 capability 弥补了此差异
- **EAN Metrics 对称暴露**：EdgeX 双北向通道 stats API 均返回 ``ean_metrics``，前端 ``StatsDialog.vue`` 统一展示；EdgeOS ``health`` API 返回 ``invoke_metrics`` 含 P50/P99 延迟和成功率
---

## 8. 迁移路线图

### Phase 1：EAN 基础就绪（已完成）

- [x] EdgeX: EAN Capability Runtime 上线
- [x] EdgeX: 63 条 Capability 生成 + 周期性发布（每 60s）
- [x] EdgeX: discovery retained 消息
- [x] EdgeX: `scan_devices` required + description 修复
- [x] EdgeOS: DiscoveryCenter 订阅 + 索引
- [x] EdgeOS: V1 Bridge 隔离（原生 EAN 优先）
- [x] EdgeOS: 主动 Discovery Query（2s → 30s → 5min）
- [x] EdgeOS: Invoke + Reply 关联（correlation_id）
- [x] EdgeOS: V1 Fallback（40%/60% 超时分配）
- [x] EdgeOS: Event 消费（含 previous_value）
- [x] EdgeOS: 心跳超时监控
- [x] EdgeOS: 权限控制 + 审计记录
- [x] EdgeOS: EAN UI（Overview / Agents / Invoke / Events）

### Phase 2：跨系统 Invoke 验证（已完成，v2.9 MQTT + NATS 双传输对称联调通过）

- [x] EdgeOS: 索引 63 条原生 EAN Capability（无 V1 Bridge 污染）
- [x] EdgeOS: 跨系统 Invoke `system.diagnostics` → 200 成功
- [x] EdgeOS: 跨系统 Invoke `modbus_tcp.scan_devices` → 链路贯通
- [x] EdgeOS: 审计记录完整（pending → completed/failed）
- [x] EdgeX: `HandleDiscoveryQuery` 响应主动查询
- [x] 双方: MQTT 18083 连通性验证
- [x] 双方: NATS 4222 对称联调（v2.9）—— EdgeX NATS 北向 `EAN-NATS` 启用；EdgeOS `ean.nats.enabled=true`；双传输注册（mqtt+nats）；63 条原生 Cap 通过 NATS 索引；`system.diagnostics` NATS Invoke 端到端成功（6ms / `v1_fallback=0`）

### Phase 3：V1 命令迁移（当前阶段，双方必须完成）

**EdgeX 侧必须完成：**

- [x] EX-P3-01: V1 命令处理标记 `DEPRECATED` 日志（`handleWriteCommand` / `handleDiscoverCommand` / `handleTaskCommand`）
- [x] EX-P3-02: `EdgeOSMQTTConfig` / `EdgeOSNATSConfig` 新增 `EANEnabled bool` 字段；`EnsureCapabilityRuntime` 检查开关；`OnConnect` 检查返回值
- [x] EX-P3-03: EAN 心跳间隔可配置（`EANHeartbeatSec`，替代 `ean_bridge.go:87` 硬编码 30s，默认 60）
- [x] EX-P3-04: 移除 `PUT /api/capability/settings` 存根 API，EAN 配置通过北向 API 提交
- [x] EX-P3-05: 北向通道弹窗新增 "EAN 能力层" Tab（启用开关、心跳间隔、事件自动发布、只读状态）
- [x] EX-P3-06: AI 助手 EAN Tab 降级为只读状态展示 + "前往北向通道配置" 跳转
- [x] EX-P3-07: EAN Invoke metrics 采集（P50/P99 延迟、成功率、失败原因）
- [x] EX-P3-08: 配置热更新支持 EAN 启停（`updateEdgeOSMQTTClients` 检测 `EANEnabled` 变化）
- [x] EX-P3-09: 新增 `StopCapabilityRuntime()` 方法（`rt.Stop()` + 置 nil + 清理订阅）

**EdgeOS 侧必须完成（由 EdgeOS 侧维护）：**

- [x] OS-P2-FIX: 北向 discovery 解析/索引/Query；V1 Cap purge + AgentSource 隔离；原生 Cap 禁用 V1 Fallback；capabilities/health 暴露 source 计数（v2.4）
- [x] OS-P3-STARTUP: 启动韧性——EAN 启用但 broker 未起时不 fatal；MQTT/NATS 后台重连 + 订阅补订（v2.7 MQTT；v2.10 NATS 延迟订阅对称，**代码完成**）
- [x] OS-P3-NATS-UI: Health `transport_details` + UI 分传输状态/联调帮助 NATS 步骤（v2.10）
- [x] OS-P3-01: 移除 V1 Fallback 机制（含 V1 合成 Cap 路径；`v1_invoke_bridge.go` 删除；`InvokeCapability` 去掉 Fallback 分支）— **已完成**：V1 Invoke Bridge 代码移除，合成 Cap 路径清理，`v1_bridge_caps=0`
- [x] OS-P3-02: 前端命令下发接口切换为 EAN Invoke — **已完成**：`ControlView.vue` 和 `PointListView.vue` 写操作切换到 `eanStore.invokeCapability`，V1 命令 API 已移除
- [x] OS-P3-03: EAN Invoke 监控 — **代码完成/待联调**：`Health.invoke_metrics`（total/success/failed/timeout/v1_fallback/P50/P99）；Overview 展示成功率与延迟；独立监控面板可后续增强
- [x] OS-P3-DEVSYNC: V1 设备全量上报对账剪枝（`ReconcileDevices`）+ EAN Agent→节点注册镜像 + reconcile API — **已完成（v2.12）**；实机 EdgeX 4 = EdgeOS 4；全量 `go test ./...` 通过
- [ ] OS-P3-04: 移除 V1 命令响应 Topic 订阅（依赖 OS-P3-01，过渡期保留）
- [ ] OS-P3-05: EAN 配置合并到北向通道配置 — **设计态**（与 EdgeX §4 同步，双方尚未落地）
- [ ] OS-P3-06: 移除 V1 节点注册/心跳 Topic 订阅 — **未做**（与 §6.3 Phase 4 / 附录 B 一致，过渡期保留；当前靠 EAN mirror 维持 `/api/nodes` 可见性）

**双方共同：**

- [ ] 稳定运行 2-4 周，收集 EAN Invoke metrics
- [ ] 对比 V1 vs EAN 命令路径的延迟和可靠性

### Phase 4：V1 Bridge 下线

**EdgeOS 侧（由 EdgeOS 侧维护）：**

- [ ] 完全移除 V1 Bridge 轮询和 Capability 合成
- [ ] 移除 V1 命令 Topic 订阅（`edgex/cmd/*`）
- [ ] 保留 V1 数据 Topic 订阅（`edgex/data/*`、`edgex/points/*`、`edgex/devices/*`）
- [ ] 保留 V1 告警 Topic 订阅（`edgex/events/*`）

**EdgeX 侧：**

- [ ] EX-P4-01: V1 命令 Topic 订阅标记 deprecated（仅保留，不再维护）
- [ ] EX-P4-02: V1 节点注册/心跳 Topic 订阅标记 deprecated（EAN Discovery + Heartbeat 已完全替代）
- [ ] EX-P4-03: 评估移除 V1 命令 Topic 订阅（`edgex/cmd/*`），仅保留数据上报 Topic

> **注**：双 Runtime（MCP + 北向 EAN）是战略设计，不合并。MCP Runtime 作为基础能力层始终运行，北向 EAN Runtime 作为高级协作层按需启停。详见 §5.1。

### Phase 5：长期共存态

**EdgeX 侧长期保留：**

- [ ] V1 数据上报路径（`edgex/data/*`、`edgex/points/*`）长期保留
- [ ] V1 设备映射（`Devices` 字段）长期保留
- [ ] V1 告警 Topic 长期保留（外部集成依赖）
- [ ] V1 设备状态上报（`edgex/devices/*`）长期保留

**双方共同：**

- [ ] EAN 2.0 为命令/发现/Capability 唯一协议
- [ ] V1 仅保留数据上报 + 告警 + 设备状态的 pub/sub 单向流

---

## 9. 最终架构态（当前实现 + 设计态标注）

```
┌─ EdgeX ──────────────────────────────────────────────────────────┐
│                                                                   │
│  ┌─ 南向驱动 ─────────────────────────────────────────────────┐  │
│  │  Modbus / BACnet / OPC UA / S7 / ...                       │  │
│  └────────────────────────────────────────────────────────────┘  │
│                          │                                        │
│  ┌─ 共享执行内核 ────────┼────────────────────────────────────┐  │
│  │  DriverExecutor ←─────┘                                     │  │
│  │       ↑                                                     │  │
│  │  CapabilityMapper（共享）                                   │  │
│  │       ↑                        ↑                            │  │
│  │  EAN Dispatcher            MCP Handler                      │  │
│  └───────┼────────────────────────┼───────────────────────────┘  │
│          │                        │                                │
│  ┌───────┴────────┐  ┌───────────┴──────────────────────────┐    │
│  │                 │  │                                      │    │
│  │  ┌─ 北向 EAN ─┐ │  │  ┌─ MCP Runtime（基础层）─────────┐ │    │
│  │  │  Runtime   │ │  │  │  TransportSDK + NoopBus        │ │    │
│  │  │  (高级层)  │ │  │  │  sync.Once, Server 启动即就绪   │ │    │
│  │  │            │ │  │  │  不依赖通道连接                 │ │    │
│  │  │ TransportMQTT│  │  │                                │ │    │
│  │  │ + mqttBus  │ │  │  │  7 Unified Capability（`GenerateUnifiedCapabilities`） │ │    │
│  │  │            │ │  │  │  ~34 MCP 工具（9 ean_* unified + 25 edgex_*）         │ │    │
│  │  │ 63 Cap     │ │  │  │  /api/capability/invoke (HTTP) │ │    │
│  │  │ (共享定义) │ │  │  │  MCP tools/call (JSON-RPC)     │ │    │
│  │  │            │ │  │  │  LLM 接入 → 智能设备操作       │ │    │
│  │  └────────────┘ │  │  └────────────────────────────────┘ │    │
│  │       ↑         │  └──────────────────────────────────────┘    │
│  │  ┌─ EdgeOS(MQTT) 通道 ─────────────────────────────────────┐  │
│  │  │                                                        │  │
│  │  │  [1] 通道基础层                                        │  │
│  │  │   ├── broker / client_id / node_id / auth              │  │
│  │  │   └── Enable（通道启停）                               │  │
│  │  │                                                        │  │
│  │  │  [2] 数据上报层（V1 保留）                             │  │
│  │  │   ├── 设备映射 (Devices)                               │  │
│  │  │   ├── edgex/data/* (实时数据)                          │  │
│  │  │   ├── edgex/points/* (点位元数据)                      │  │
│  │  │   ├── edgex/devices/* (设备状态)                       │  │
│  │  │   └── edgex/events/* (告警事件)                        │  │
│  │  │                                                        │  │
│  │  │  [3] EAN 能力层（高级功能，设计态）                    │  │
│  │  │   ├── ean_enabled: bool     ← EAN Runtime 启停         │  │
│  │  │   ├── ean_heartbeat_sec: int ← 心跳周期                │  │
│  │  │   ├── ean_event_auto_publish ← 事件自动发布            │  │
│  │  │   │                                                    │  │
│  │  │   ├── $edgeos/discovery/*  (retained, 60s 周期)        │  │
│  │  │   ├── $edgeos/invoke/{agent}     (命令入口)            │  │
│  │  │   ├── $edgeos/reply/{agent}      (命令响应)            │  │
│  │  │   └── $edgeos/heartbeat/{agent}  (心跳)                │  │
│  │  └────────────────────────────────────────────────────────┘  │
│  │                                                              │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
          │ MQTT / NATS（仅北向 EAN Runtime 使用）
          ▼
┌─ EdgeOS ─────────────────────────────────────────────────────────┐
│                                                                   │
│  ┌─ Discovery Center ────────────────────────────────────────┐  │
│  │  订阅 $edgeos/discovery/*  → 索引 63 条 Capability        │  │
│  │  V1 Bridge 隔离: native-ean 优先于 v1-bridge              │  │
│  └──────────────────────────────────────────────────────────┘  │
│  ┌─ Invoke Orchestrator ─────────────────────────────────────┐  │
│  │  $edgeos/invoke/{agent} → EdgeX → $edgeos/reply/{os}     │  │
│  │  V1 Fallback: EAN 超时后转 V1 命令协议 (40%/60%)         │  │
│  └──────────────────────────────────────────────────────────┘  │
│  ┌─ V1 数据订阅（保留）─────────────────────────────────────┐  │
│  │  edgex/data/*       → 实时数据                            │  │
│  │  edgex/points/*     → 点位元数据                          │  │
│  │  edgex/devices/*    → 设备状态                            │  │
│  │  edgex/events/*     → 告警/事件                           │  │
│  └──────────────────────────────────────────────────────────┘  │
│  ┌─ V1 Bridge（隔离态）─────────────────────────────────────┐  │
│  │  轮询 ListNodes() → 仅对无原生 EAN Cap 的 Agent 合成     │  │
│  │  点位数据同步 → 生成 EAN Event（含 previous_value）      │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
```

**关键变化（v2.6）**：架构图新增 MCP Runtime 独立层，与北向 EAN Runtime 并列。MCP Runtime 不经过 MQTT/NATS 网络，直接通过 in-process 调用访问共享执行内核。北向 EAN Runtime 的 EAN 能力层配置（`ean_enabled` 等）从独立 `ean:` 段合并到通道配置字段中（设计态）。

---

## 10. 风险与回退策略

| 风险 | 影响 | 缓解措施 | 状态 |
|---|---|---|---|
| EAN MQTT 连接闪断导致 Invoke 失败 | 高 | retained discovery 确保重连恢复；30s 超时重试 | 已验证 |
| 外部系统依赖 V1 告警 topic | 中 | V1 告警 topic 长期保留，不迁移 | 已确认 |
| Capability 描述不完整导致调用失败 | 中 | `generator.go` 已修复 description 和 required 标记 | **已验证通过** |
| V1 Bridge 隔离不彻底导致索引污染 | 中 | `CapSource` 双来源标记 + `upsertCapability` 优先级规则 | **已验证通过** |
| EdgeOS 晚于 EdgeX 启动错过 Discovery | 高 | EdgeX 60s 周期重发 + EdgeOS 主动 Query + retained 消息 | **已验证通过** |
| EAN 设置合并后用户找不到入口 | 中 | 北向通道弹窗新增 "EAN 能力层" Tab；EAN 视图保留 | 设计态 |
| 数据上报路径保留导致双协议长期共存 | 低 | V1 仅保留数据上报（单向 pub/sub），命令路径统一为 EAN | 已确认 |

---

## 11. 配置字段对照

### 11.1 当前 EdgeOS 配置（独立 ean 段）

```yaml
ean:
  enabled: true
  planner_id: edgeos-planner
  mqtt:
    enabled: true
    broker: tcp://127.0.0.1:18083
    client_id: edgeos-ean
    qos: 1
  nats:
    enabled: false
    url: nats://127.0.0.1:4222
  heartbeat:
    check_interval_sec: 5
    timeout_multiplier: 3
```

### 11.2 设计态：合并后的 EdgeOSMQTTConfig 字段

| 字段 | 类型 | 说明 | 来源 |
|---|---|---|---|
| `ID` | string | 通道 ID | 现有 |
| `Name` | string | 通道名称 | 现有 |
| `Enable` | bool | 通道启停 | 现有 |
| `Broker` | string | MQTT broker 地址 | 现有 |
| `ClientID` | string | MQTT 客户端 ID | 现有 |
| `NodeID` | string | 节点 ID（同时作为 EAN AgentID） | 现有 |
| `Username` | string | 用户名 | 现有 |
| `Password` | string | 密码 | 现有 |
| `QoS` | byte | QoS 级别 | 现有 |
| `Retain` | bool | 是否保留消息 | 现有 |
| `CleanSession` | bool | 清除会话 | 现有 |
| `KeepAlive` | int | 心跳间隔（秒） | 现有 |
| `ConnectTimeout` | int | 连接超时 | 现有 |
| `AutoReconnect` | bool | 自动重连 | 现有 |
| `MaxReconnectInterval` | int | 最大重连间隔 | 现有 |
| `HeartbeatInterval` | string | V1 心跳间隔 | 现有（V1 保留） |
| `Devices` | map | 设备映射配置 | 现有（V1 保留） |
| `VirtualDevices` | OpcUaDeviceMap | 虚拟设备映射 | 现有 |
| `EANEnabled` | bool | EAN 能力层启用 | **已落地** |
| `EANHeartbeatSec` | int | EAN 心跳间隔（秒） | **已落地** |
| `EANEventAutoPublish` | bool | EAN 事件自动发布 | **已落地** |

---

## 附录 A：关键代码路径

### EdgeX

| 模块 | 路径 | 状态 | Phase 3 动作 |
|---|---|---|---|
| 北向 MQTT 客户端 | `internal/northbound/edgos_mqtt/client.go` | 运行中 | EX-P3-01: 标记 DEPRECATED；EX-P3-02: `OnConnect` 检查 `EANEnabled` |
| 北向 NATS 客户端 | `internal/northbound/edgos_nats/client.go` | 运行中 | EX-P3-01: 标记 DEPRECATED；EX-P3-02: 同上 |
| EAN MQTT 桥接（北向 Runtime） | `internal/northbound/edgos_mqtt/ean_bridge.go` | 运行中 | EX-P3-02/03: `EANEnabled` 检查 + 可配置心跳；EX-P3-09: 新增 `StopCapabilityRuntime` |
| EAN NATS 桥接（北向 Runtime） | `internal/northbound/edgos_nats/ean_bridge.go` | 运行中 | EX-P3-02/03/09: 同上 |
| Capability Runtime（核心库） | `internal/capability/runtime.go` | 运行中 | EX-P3-07: metrics 采集 |
| Capability 生成器 | `internal/capability/generator.go` | description + required 已修复 | - |
| Discovery 发布器 | `internal/capability/discovery_publisher.go` | HandleDiscoveryQuery 已实现 | - |
| Execution Mapper | `internal/execution/capability_mapper.go` | 运行中（双 Runtime 共享） | - |
| Driver Executor | `internal/execution/driver_executor.go` | 运行中（双 Runtime 共享） | - |
| MCP Runtime（基础能力层） | `internal/server/mcp_handler.go:2301` `ensureCapabilityRuntime` | 运行中，`sync.Once` 单例，TransportSDK + NoopBus | 不需改动（独立于 EAN 配置） |
| EAN HTTP API 处理器 | `internal/server/capability_handler.go` | 运行中，PUT 存根 | EX-P3-04: 移除 PUT 存根 |
| EAN API 路由 | `internal/server/server.go` `/api/capability/*` | 运行中 | EX-P3-04: 移除 PUT 路由 |
| 北向管理器（MQTT） | `internal/core/northbound_manager_edgos.go` | 运行中 | EX-P3-08: EAN 字段热更新 |
| 北向管理器（NATS） | `internal/core/northbound_manager_edgos_nats.go` | 运行中 | EX-P3-08: 同上 |
| 配置模型 | `internal/model/types.go` `EdgeOSMQTTConfig:421` / `EdgeOSNATSConfig:443` | 运行中 | EX-P3-02/03: 追加 EAN 字段 |
| 前端北向弹窗 | `ui/src/components/northbound/EdgeOSMQTTSettingsDialog.vue` | 运行中 | EX-P3-05: 新增 EAN 能力层 Tab |
| 前端北向弹窗 | `ui/src/components/northbound/EdgeOSNATSSettingsDialog.vue` | 运行中 | EX-P3-05: 同上 |
| 前端 AI 设置 | `ui/src/components/ai-assistant/AiSettingsDialog.vue` | 运行中 | EX-P3-06: EAN Tab 降级为只读 |
| 前端 EAN 状态 | `ui/src/composables/useEan.js` | 运行中 | EX-P3-06: 移除 `saveSettings` |

### EdgeOS

| 模块 | 路径 | 状态 | Phase 3 动作 |
|---|---|---|---|
| EAN 配置类型 | `internal/config/ean_config.go` | 运行中 | OS-P3-05: 合并到北向通道配置 |
| EAN 配置文件 | `config/config.yaml` `ean:` 段 | 运行中 | OS-P3-05: 合并到 `middlewares[]` |
| EAN Bus | `internal/ean/bus.go` | 原生 Cap 100% MQTT；仅 V1 合成可 Fallback | OS-P3-01: 移除全部 Fallback |
| 双传输层 | `internal/ean/transport.go` | 运行中 | - |
| 消息信封 | `internal/ean/envelope.go` | 运行中 | - |
| 类型定义 | `internal/ean/model.go` | FlexibleStringMap / TransportList 兼容北向 | - |
| V1 Bridge | `internal/ean/bridge.go` | **已隔离**（原生跳过 Agent/Cap 合成） | Phase 4 移除 |
| V1 Invoke Bridge | `internal/ean/v1_invoke_bridge.go` | **仅** `…/read-write` | OS-P3-01: 移除 |
| Discovery Center | `internal/ean/discovery.go` | 对接北向 mqttBus；Cap+Agent 隔离+purge | - |
| Event Center | `internal/ean/event.go` | 运行中 | - |
| Invoke Orchestrator | `internal/ean/invoke.go` | 运行中 | - |
| Heartbeat Monitor | `internal/ean/heartbeat.go` | 运行中 | - |
| Governance | `internal/ean/governance.go` | 运行中 | - |
| EAN API 路由 | `internal/server/ean_routes.go` | 返回 Cap `source` 与计数 | - |
| EAN UI 视图 | `ui/src/views/ean/*.vue` | 运行中 | OS-P3-02: 前端命令切换 |
| Messaging Manager | `internal/messaging/manager.go` | 保留数据/告警订阅 | OS-P3-04: 移除 `edgex/cmd/responses/#` |

---

## 附录 B：V1 与 EAN Topic 共存态

### 当前保留的 Topic

| Topic | 协议 | 方向 | 说明 |
|---|---|---|---|
| `edgex/data/{node}/{device}` | V1 | EdgeX -> EdgeOS | 实时数据上报（保留） |
| `edgex/points/{node}/{device}` | V1 | EdgeX -> EdgeOS | 点位元数据（保留） |
| `edgex/points/report` | V1 | EdgeX -> EdgeOS | 点位全量同步（保留） |
| `edgex/devices/{node}/{device}/online` | V1 | EdgeX -> EdgeOS | 设备上线（保留） |
| `edgex/devices/{node}/{device}/offline` | V1 | EdgeX -> EdgeOS | 设备离线（保留） |
| `edgex/devices/report` | V1 | EdgeX -> EdgeOS | 设备信息上报（保留） |
| `edgex/events/alert` | V1 | EdgeX -> EdgeOS | 告警（保留） |
| `edgex/events/error` | V1 | EdgeX -> EdgeOS | 错误事件（保留） |
| `edgex/events/info` | V1 | EdgeX -> EdgeOS | 信息事件（保留） |
| `$edgeos/discovery/agent` | EAN | EdgeX -> EdgeOS | Agent 描述符（retained） |
| `$edgeos/discovery/capability` | EAN | EdgeX -> EdgeOS | Capability 描述符（retained） |
| `$edgeos/discovery/query` | EAN | EdgeOS -> EdgeX | 主动发现查询 |
| `$edgeos/discovery/response` | EAN | EdgeX -> EdgeOS | 发现查询响应 |
| `$edgeos/discovery/agent/offline` | EAN | EdgeX -> EdgeOS | Agent 离线 |
| `$edgeos/heartbeat/{agent}` | EAN | EdgeX -> EdgeOS | 心跳 |
| `$edgeos/invoke/{agent}` | EAN | EdgeOS -> EdgeX | 命令调用 |
| `$edgeos/reply/{agent}` | EAN | EdgeX -> EdgeOS | 命令响应 |
| `$edgeos/event/{agent}` | EAN | EdgeX -> EdgeOS | 事件广播 |

### 计划移除的 Topic（Phase 3/4）

| Topic | 协议 | 说明 | 计划 |
|---|---|---|---|
| `edgex/nodes/register` | V1 | 被 `$edgeos/discovery/agent` 替代 | Phase 4 |
| `edgex/nodes/{node}/online` | V1 | 被 `$edgeos/discovery/agent` 替代 | Phase 4 |
| `edgex/nodes/{node}/offline` | V1 | 被 `$edgeos/discovery/agent/offline` 替代 | Phase 4 |
| `edgex/heartbeat/{node}` | V1 | 被 `$edgeos/heartbeat/{agent}` 替代 | Phase 4 |
| `edgex/cmd/{node}/discover` | V1 | 被 `*.scan_devices` Invoke 替代 | Phase 3 |
| `edgex/cmd/{node}/{device}/write` | V1 | 被 `*.write_register` Invoke 替代 | Phase 3 |
| `edgex/cmd/{node}/task/{type}/{id}` | V1 | 被 EAN Capability 替代 | Phase 3 |
| `edgex/cmd/responses/{node}/{device}` | V1 | 被 `$edgeos/reply/{agent}` 替代 | Phase 3 |
| `edgex/cmd/nodes/register` | V1 | 被 `$edgeos/discovery/agent` 替代 | Phase 4 |

---

*本文档基于 EdgeX (`d:\code\edgex`) 和 EdgeOS (`d:\code\edgeOS`) 代码库 2026-07-28 版本编制。*
*更新记录：v2.12 (EdgeOS侧) 设备数对账——V1 devices/report 全量剪枝 + EAN→节点镜像；实机 4=4；`go test ./...` 全绿。v2.11 (双侧) NATS 对称联调复验；OS-P3-01/02。v2.10 (EdgeOS侧) NATS 延迟订阅/Health transport_details。v2.9 NATS 启用。v2.8 Governance 权限。v2.7 启动韧性。v2.6～v2.1 见历史。*
