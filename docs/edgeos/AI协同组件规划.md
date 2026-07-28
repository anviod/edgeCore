# EdgeX AI协同组件规划 — EAN 2.0 Capability Runtime

> **产品定位**：**EAN 2.0 EdgeX Capability Runtime** — Edge Agent Network 2.0 中 EdgeX 边缘网关的统一能力运行时。在现有 EdgeX + EdgeOS 架构上增加一层统一的 Agent 协作能力，**不是重新设计**。复用已有 AI、MCP、Execution Mapper、ShadowCore，新增 Capability Runtime 层。  
> **核心价值**：「工程师花 2 天分析协议」→「AI 30 分钟生成候选配置，工程师确认上线」；同时 Capability 统一接入 EAN 2.0 网络，支持跨 Agent 发现、编排、调用。  
> **工程铁律**：任何 AI 能力不得以牺牲稳定性为代价；**禁止** AI 调用进入 ScanEngine / Pipeline Worker 热路径；所有写配置须经 **Human-in-the-loop** 确认后落库。  
> **架构结论**：**工业边缘自治 + AI 协同中心 + Capability Runtime** — RK3588/3688 等边缘网关运行 EdgeX 采集内核、Capability Runtime、报文捕获/解码与 AI Agent Client；LLM/VLM/Embedding 与 **protocol_knowledge.db（bbolt）** 统一由远端 **AI Model Center** 承担；**AI 故障不得影响工业采集与规则执行**。


| 项       | 内容                                                                                                            |
| ------- | ------------------------------------------------------------------------------------------------------------- |
| 版本      | **V2.0**                                                                                                      |
| 更新      | 2026-07-27                                                                                                    |
| 状态      | **EAN-MVP 已落地**（Capability Runtime + MQTT/NATS Bridge + DriverExecutor + AI Adapter + MCP Adapter + Shadow→Event/`previous_value`；详见 [EAN2.0-EdgeX-EdgeOS改造指南](../edgeos/EAN2.0-EdgeX-EdgeOS改造指南.md)） |
| 产品名     | **EdgeX Industrial Protocol Copilot + EAN 2.0 Capability Runtime**（代码路径 `internal/ai_agent/` · `internal/capability/`） |
| 架构基线    | [TODO 索引 §1 新架构约束](./index.md) · [边缘网关架构设计总览](../edge/边缘网关架构设计总览.md) · [EAN 2.0 通信协议规范](../edgeos/EdgeX通信协议规范(MQTT-NATS).md) |
| 关联 TODO | [设备点位读写系统升级改造计划](./设备点位读写系统升级改造计划.md) · [边缘计算优化升级 2.0](./边缘计算优化升级2.0.md)                                      |
| 用户文档    | [边缘计算场景手册](../edge/EDGE_COMPUTING_SCENARIO_MANUAL.md) · [边缘计算最佳实践](../guide/EDGE_COMPUTING_BEST_PRACTICES.md) |


---
<div align="center">
  <img src="../img/AI助手.png" width="100%" />
  <p><small>图 1: 边缘AI 助手</small></p>
</div>
---
<div align="center">
  <img src="../img/AI助手配置.png" width="100%" />
  <p><small>图 2: 支持本地模型和在线接口</small></p>
</div>

---

## EAN 2.0 概述（V2.0 新增）

### EAN 2.0 设计原则

> **不是重新设计，而是在现有 EdgeX + EdgeOS 架构上增加一层统一的 Agent 协作能力。**

| 原则 | 说明 |
|------|------|
| **EdgeX 改动最小** | 复用已有 AI、MCP、Execution Mapper、ShadowCore、ScanEngine、Driver |
| **EdgeOS 增加平台能力** | 发现、编排、治理、调度、资源管理 |
| **协议统一** | Capability、Discovery、Invoke、Event 统一模型 |
| **Capability 为核心** | 所有能力（Driver Command / AI Skill / MCP Tool / Workflow Node）统一映射到 Capability |

### EAN 2.0 统一能力模型

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

### EdgeX 在 EAN 2.0 中的角色：Capability Runtime

EdgeX 继续承担工业通信和能力执行，在现有 AI/MCP 基础上统一到 **Capability Runtime**：

```text
AI / HTTP / SDK / MCP / Workflow
           │
           ▼
    Capability Invoke
           │
           ▼
   Invoke Dispatcher（新增）
           │
           ▼
  Capability Registry（新增）
           │
           ▼
  Execution Mapper（少量升级）
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

### V1.5 → V2.0 能力映射

| V1.5 模块 | V2.0 定位 | 变更程度 | 代码路径 |
|-----------|-----------|---------|---------|
| AI Agent Client | Capability Runtime 组件 | 少量升级 | `internal/ai_agent/` |
| MCP Handler | Capability → Tool → MCP Adapter | 少量升级 | `internal/server/mcp_handler.go` |
| Execution Mapper | Capability → Driver Command 映射 | 少量升级 | `internal/execution/` |
| ShadowCore | Capability 状态缓存 | 保持 | `internal/shadow/` |
| ScanEngine / Driver | 底层执行 | 保持 | `internal/driver/*` |
| Invoke Dispatcher | **新增** | 新增 | `internal/capability/dispatcher.go` |
| Capability Registry | **新增** | 新增 | `internal/capability/registry.go` |
| Event Publisher | **新增** | 新增 | `internal/capability/event_publisher.go` |
| Discovery Publisher | **新增** | 新增 | `internal/capability/discovery_publisher.go` |

### EAN 2.0 与 EdgeOS 协作边界

| 模块 | EdgeX（执行层） | EdgeOS（平台层） |
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

## §E1 EAN 2.0 Capability Runtime 详细设计（V2.0 新增）

### §E1.1 Capability Registry

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

**自动生成规则**：

| Driver 命令 | 自动生成 Capability ID | 参数映射 |
|------------|----------------------|---------|
| `ReadPoints` | `{protocol}.read_{register_type}` | device_id, addresses[] |
| `WritePoint` | `{protocol}.write_{register_type}` | device_id, address, value |
| `ScanDevices` | `{protocol}.scan_devices` | channel_id, network |
| `GetDevicePoints` | `{protocol}.list_points` | device_id |
| `Diagnostics` | `system.diagnostics` | — |
| `AI.protocol_reverse` | `ai.protocol_reverse` | payload |
| `AI.doc_parse` | `ai.doc_parse` | payload |

**Capability Descriptor 发布**：

EdgeX 启动时，Capability Registry 收集所有 Capability，通过 MQTT/NATS 发布到 `$edgeos/discovery/capability`。

### §E1.2 Invoke Dispatcher（新增）

统一入口。所有 MQTT、HTTP、SDK、MCP、Workflow 调用统一为 `Capability Invoke`。

```text
MQTT    HTTP    SDK    MCP    Workflow
    \      |      /      /      /
     \     |     /      /      /
      \    |    /      /      /
       \   |   /      /      /
        \  |  /      /      /
      Invoke Dispatcher
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

**Dispatcher 路由逻辑**（伪代码）：

```go
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

### §E1.3 AI Adapter 升级

**V1.5 架构**：

```text
AI → MCP Tool
```

**V2.0 架构**：

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

### §E1.4 MCP Adapter 升级

**V1.5 架构**：

```text
MCP Tool → Command（直接调用）
```

**V2.0 架构**：

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

### §E1.5 Execution Mapper 升级

增加 Capability → Driver Command 映射：

```text
Capability: modbus_tcp.read_holding_register
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

### §E1.6 Event Publisher（新增）

Capability 执行导致设备状态变化时，自动发布 Event：

```text
Capability: modbus_tcp.write_register
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
$edgeos/event/edgex-node-001
```

**自动 Event 类型映射**：

| 操作 | 自动发布 Event |
|------|---------------|
| 点位值变化 | `{point_id}.changed` |
| 设备上线 | `device.online` |
| 设备离线 | `device.offline` |
| 告警触发 | `alarm.created` |
| Capability 执行完成 | `capability.invoked` |

### §E1.7 Discovery Publisher（新增）

**启动流程**：

```text
EdgeX 启动
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
启动 Heartbeat 定时器 → $edgeos/heartbeat/edgex-node-001
```

**关闭流程**：

```text
EdgeX 关闭（Graceful Shutdown）
    │
    ▼
DiscoveryPublisher 发布 Offline
    │
    ▼
$edgeos/discovery/agent/offline
```

### §E1.8 ShadowCore 升级

ShadowCore 继续维护设备状态，同时支持按 Capability ID 查询缓存：

```json
{
  "capability_cache": {
    "modbus_tcp.read_holding_register": {
      "device_slave_1": {
        "last_result": [...],
        "last_updated": 1776787200000
      }
    }
  }
}
```

Execution 优先读取 Shadow，减少 Driver 调用。

### §E1.9 EAN 2.0 Topic 规范（EdgeX 侧）

保留所有 V1.0 `edgex/*` Topic 作为兼容层，新增 EAN 2.0 Topic：

| Topic/Subject | 方向 | QoS | 说明 |
|---------------|------|-----|------|
| `$edgeos/discovery/agent` | EdgeX → EdgeOS | 1 | Agent 注册/更新 |
| `$edgeos/discovery/agent/offline` | EdgeX → EdgeOS | 1 | Agent 下线 |
| `$edgeos/discovery/capability` | EdgeX → EdgeOS | 1 | Capability 注册 |
| `$edgeos/invoke/{agent_id}` | EdgeOS → EdgeX | 1 | Capability 调用请求 |
| `$edgeos/reply/{agent_id}` | EdgeX → EdgeOS | 1 | Capability 调用响应 |
| `$edgeos/event/{agent_id}` | EdgeX → EdgeOS | 1 | 事件上报 |
| `$edgeos/state/{agent_id}` | EdgeX → EdgeOS | 1 | Shadow 全量上报 |
| `$edgeos/state/{agent_id}/delta` | EdgeX → EdgeOS | 1 | Shadow 增量更新 |
| `$edgeos/heartbeat/{agent_id}` | EdgeX → EdgeOS | 0 | 心跳 |

详细协议规范参见 [EdgeX通信协议规范(MQTT-NATS) V2.0](../edgeos/EdgeX通信协议规范(MQTT-NATS).md)。

### §E1.10 Capability SDK（新增）

SDK 面向 Capability 编程，而非面向 Driver 编程：

```go
// 调用 Capability（不直接调用 Driver）
result, err := sdk.InvokeCapability(ctx, InvokeRequest{
    Target:     "edgex-node-001",
    Capability: "modbus_tcp.read_holding_register",
    Arguments: map[string]interface{}{
        "device_id": "slave-1",
        "address":   "40001",
    },
})
```

---

## §0 背景与目标



### 0.1 背景

EdgeX 南向已支持 Modbus / OPC UA / S7 / BACnet / EIP / SNMP / IEC104 等 12+ 协议，采集内核以 **ScanEngine → ShadowCore → DataPipeline** 为统一数据面。现场集成仍高度依赖人工：

1. **阅读厂家 PDF / DOC / 寄存器表 / 点表 Excel / 上位机监控表 / 抓包文件**，手工录入 `model.Point`（地址、数据类型、缩放、读写属性）
2. **对照协议差异**（Modbus 功能码、S7 DB 块、BACnet ObjectType 等）反复试错；**仅有 HMI 显示值而无寄存器地址**时，需对照 PCAP 人工逆向
3. **编写通道驱动参数**（从站号、IP/端口、字节序、扫描类）与 EdgeRule 场景骨架
4. **联调排障**依赖 `/api/diagnostics/`* 与日志，缺少上下文化建议

上述工作重复、易错，且与协议栈知识强耦合。本模块以 **工业协议逆向工程引擎（Industrial Protocol Reverse Engineering Engine）** 为核心，在**冷路径**将厂家资料与报文分析转化为 **可生产部署的 EdgeX 配置**，而非生成可读性报告或文档摘要。

### 0.2 目标


| #      | 目标                        | 可度量结果                                                                       |
| ------ | ------------------------- | --------------------------------------------------------------------------- |
| **G0** | **协议逆向 → 生产配置**（模块核心）     | 无文档设备：PCAP + 显示值 → 30min 内产出候选点位 + Channel/Point JSON；工程师确认后 import         |
| G1     | 厂家文档 / 监控表 → 结构化点位 + 驱动参数 | 单设备 50～200 点表，人工校对时间 ↓ 60%+                                                 |
| G2     | 输出与驱动规范对齐                 | Protocol Model / Point Definition / Driver Parameter 导入前 Schema 校验通过率 ≥ 95% |
| G3     | 附带 Validation Case        | 每批候选含可回放验证用例（期望读数、容差、证据链）                                                   |
| G4     | 边缘垂直场景草稿（辅助）              | 从场景描述生成 EdgeRule / 场景模版 JSON 草案                                             |
| G5     | 联调诊断辅助（辅助）                | 结合 diagnostics + 日志给出可执行排查步骤                                                |
| G6     | Token 成本可控                | 网关侧配额可限、可审计；**本地 Mock UI 仅展示每日任务配额**，Token 用量在 **AI Model Center（remote）** 模式下可见 |




### 0.3 非目标（明确边界）

- **不**做通用聊天助手或文档摘要工具——AI 输出 **不是** Markdown 报告，而是 **EdgeX 可导入 JSON**
- **不**在 ScanEngine / ExecutionLayer / Pipeline Worker 循环内调用 LLM
- **不**在 RK3588 / ARM64 边缘网关上运行 LLM 推理（无 Ollama / vLLM 本地模型）
- **不**让 LLM 解码字节——字节解析由 **Decoder（确定性）** 与 **Rule Engine** 完成
- **不**自动写入 `config.db` 或下发写点指令（须 UI/API 显式确认）
- **不**将完整现场配置、凭证、北向密钥上传至公网（默认脱敏 + 可选私有 AI Server）
- **不**替代驱动 `decoder_test.go` 与联机测试报告



### 0.4 最终交付物（四类产出）


| 交付物                  | 说明                                                      | 落库路径                     |
| -------------------- | ------------------------------------------------------- | ------------------------ |
| **Protocol Model**   | `protocol_id`、帧特征、地址模型、字节序规则、功能码惯例                      | 校验 + 通道配置参考              |
| **Point Definition** | `[]ImportPoint` 对齐 `model.Point`：地址、类型、scale、scan_class | `POST .../points/import` |
| **Driver Parameter** | Channel JSON：协议、IP、端口、slave、rack/slot、device_instance 等 | 通道配置 API                 |
| **Validation Case**  | 期望读数、容差 ε、证据（帧偏移/寄存器/时间戳）、置信度                           | 联调回放；不入 config.db        |


---



## §1 核心能力定位



### 1.1 模块 centerpiece：工业协议逆向工程引擎

**Industrial Protocol Reverse Engineering Engine** 是本模块 **P0+ 核心能力**，而非文档解析的附属功能。引擎贯通 **协议识别 → 报文结构解析 → 物理量推理 → 生产配置生成** 四阶段流水线（见 §2），支撑 **Scenario B（无文档）** 高价值场景，并由 **Scenario A（有文档）** 提供地址真源与交叉验证。

### 1.2 双输入场景



#### Scenario A — 有文档（Supporting · P0）

厂家已提供可解析资料，AI 抽取结构化字段后生成生产配置。

```text
PDF / Excel / DOC / PLC变量表 / HMI点表
    ↓  文档解析（AI Server：OCR/RAG/LLM 结构化）
协议 ID 推断 → 地址模型 → datatype → 换算规则（scale/offset/byte_order）
    ↓  Rule Engine + Schema 校验
Point Definition + Driver Parameter + Validation Case
    ↓  Human Confirm
EdgeX Point Model + Channel JSON → import
```


| 输入类型             | 典型来源        | 产出                   |
| ---------------- | ----------- | -------------------- |
| 厂家 PDF / 协议手册    | 寄存器表章节      | 地址 + 类型 + scale      |
| Excel / CSV 点表   | 厂家交付、HMI 导出 | 批量 Point JSON        |
| PLC 变量表 / TIA 导出 | `.xml`、符号表  | S7 DB 偏移映射           |
| 上位机监控表           | WinCC、组态王等  | 标签名 + 描述 + 可选 I/O 地址 |




#### Scenario B — 无文档（Core · P0+ · 更高价值）

现场**无可靠寄存器表**，仅有 HMI 显示值与 PCAP/串口 HEX 抓包。引擎被动分析报文，推理候选点位，经人工确认后上线。

```text
PCAP / 串口 HEX 抓包
    ↓  网关本地：Capture + Decoder（确定性，复用驱动 decoder）
协议 ID（Rule Engine 优先）→ 通信行为分析（轮询周期、FC 序列）
    ↓  字段候选：datatype × byte_order × scale 组合
物理量推理（LLM：数值关联，如 220.5/221.1/219.8 → Uab/Ubc/Uca）
    ↓  candidate points + confidence
Human Confirm → Production Config（Channel + Point JSON）→ import
```

> **Scenario B 是产品差异化核心**：解决「有显示值、无地址」的现场痛点，将 2 天人工逆向压缩至 30 分钟候选生成。



### 1.3 能力价值表


| 能力          | 价值                  |
| ----------- | ------------------- |
| 厂家 PDF 解析   | 自动生成点表与驱动参数         |
| Excel 点表解析  | 快速批量导入              |
| **PCAP 逆向** | **无文档设备接入（核心）**     |
| 串口 HEX 分析   | 老设备 / Modbus RTU 兼容 |
| 协议识别        | 自动选择驱动与解码策略         |
| 点位推理        | 减少工程调试与试错           |
| 配置生成        | 直接生产部署，非可读报告        |




### 1.4 场景对照

```text
┌─────────────────────────────────────────────────────────────────────────┐
│           EdgeX Industrial Protocol Copilot                              │
├──────────────────────────────┬──────────────────────────────────────────┤
│  Scenario A（有文档）         │  Scenario B（无文档 · 核心）              │
│  PDF/Excel/DOC/PLC表/HMI点表  │  PCAP / 串口 HEX + HMI 显示值            │
│         ↓                    │         ↓                                │
│  文档解析 + RAG              │  网关 Decoder + Rule Engine              │
│         ↓                    │         ↓                                │
│  结构化字段抽取              │  报文结构 + 字段候选                      │
│         ↓                    │         ↓                                │
│  ───────────── 四阶段流水线（§2）────────────────────────────────────  │
│         ↓                    │         ↓                                │
│  Protocol Model + Points + Driver Param + Validation Case              │
│         ↓                                                                │
│  Human Confirm → EdgeX config.db（Channel + Point import）              │
└─────────────────────────────────────────────────────────────────────────┘
```

---



## §2 四阶段分析流水线

所有协议分析任务（Scenario A/B）统一经过四阶段流水线。**阶段 1～2 以确定性解码为主；阶段 3 为 LLM 最高价值区；阶段 4 输出生产 JSON。**

### 2.1 阶段一：协议识别（Protocol Identification）

**Rule Engine 优先，非 LLM 猜测。**


| 协议             | 识别特征                                                     | 置信度来源                        |
| -------------- | -------------------------------------------------------- | ---------------------------- |
| **Modbus TCP** | TCP 502 + MBAP 头（TransactionId/ProtocolId/Length/UnitId） | 端口 + PDU 结构匹配                |
| **S7**         | TCP 102 + TPKT/COTP + S7 PDU                             | 握手 Setup Communication       |
| **BACnet/IP**  | UDP 47808 + BVLC（type 0x81）                              | Who-Is / I-Am / ReadProperty |
| Modbus RTU     | 串口透传 / RTU-over-TCP + CRC                                | 从站地址 + FC + CRC 校验           |


```text
输入帧 / PCAP 摘要
    ↓
Rule Engine：端口 + 魔数 + 帧头模式匹配
    ↓
protocol_id + confidence_score（0～1）
    ↓
confidence < 0.7 → UI 提示人工选择协议；不进入阶段二
```



### 2.2 阶段二：报文结构解析（Message Structure Parsing）

**Decoder 提取确定性字段**（复用 `internal/driver/*/decoder.go`）：


| 协议     | 提取字段                                                                                |
| ------ | ----------------------------------------------------------------------------------- |
| Modbus | `slave_id`, `function_code`, `start_address`, `quantity`, response `raw[]`          |
| BACnet | `device_instance`, `object_type`, `instance`, `property_id`, `present-value` octets |
| S7     | `area`, `db`, `offset`, response raw                                                |


对 response raw 尝试 **UINT16 / INT32 / UINT32 / FLOAT32** 及 **ABCD / CDAB / BADC / DCBA** 字节序变体，产出 **candidate_fields[]**（每项含解码值、偏移、datatype 假设）。

### 2.3 阶段三：物理量推理（Physical Quantity Inference）

**LLM 最高价值环节**——仅做语义理解与关联推理，**不**解码字节：


| LLM 职责  | 示例                                            |
| ------- | --------------------------------------------- |
| 数值关联    | 220.5、221.1、219.8 → 推断为 Uab / Ubc / Uca 三相线电压 |
| 单位与量纲   | 描述「线电压」+ 数值域 200～250 → V                      |
| 多标签联合评分 | Ia/Ib/Ic 同时匹配 → 提升置信度                         |
| 命名建议    | `CHILLER_P1_TEMP1` → 中文名「冷机1蒸发侧进水温度」          |


输入：`candidate_fields[]` + 监控表观测值（可选）+ `protocol_knowledge.db`（bbolt）检索片段。  
输出：`point_candidates[]` 含 `confidence`、`evidence`、`semantic_label`。

### 2.4 阶段四：生产配置生成（Production Config Generation）

输出 **EdgeX 可直接导入的 JSON**，非人类可读报告：

- **Channel JSON**：`protocol_id`, `ip`, `port`, `slave_id`, 协议专属参数
- **Point JSON**：`id`, `name`, `address`, `datatype`, `scale`, `scan_class`, `function_code`, `byte_order`
- **Validation Case**：期望读数、容差、关联帧证据

```text
阶段一 ──► 阶段二 ──► 阶段三 ──► 阶段四
Rule ID     Decoder      LLM 语义     Channel + Point JSON
            候选字段      物理量关联    + Validation Case
                                              ↓
                                    Result Validator → UI Diff → Confirm → import
```

---



## §3 Decoder / Rule Engine / LLM 分工



### 3.1 流水线架构（CRITICAL）

```text
PCAP / HEX / 文档表格
    ↓
┌──────────────────────────────────────────────────────────────────┐
│  网关（RK3588）                                                    │
│  Capture（gopacket）→ Decoder（确定性 · 复用驱动 decoder）         │
│       ↓                                                            │
│  协议字段摘要 JSON（不上传原始 PCAP 全量）                          │
└───────────────────────────────┬──────────────────────────────────┘
                                │ gRPC / MQTT
┌───────────────────────────────▼──────────────────────────────────┐
│  AI Server（AI Model Center）                                     │
│  Rule Engine（协议识别 · 模式匹配 · 寄存器惯例）                   │
│       ↓                                                            │
│  candidate_fields[]（datatype/offset/scale 组合枚举结果）          │
│       ↓                                                            │
│  LLM（仅语义层：推理 · 关联 · 命名 · 置信度解释）                   │
│       ↓                                                            │
│  Point Candidate + Protocol Model + Driver Parameter               │
└───────────────────────────────┬──────────────────────────────────┘
                                │
┌───────────────────────────────▼──────────────────────────────────┐
│  网关 Result Validator → Human Confirm → config.db import          │
└──────────────────────────────────────────────────────────────────┘
```



### 3.2 职责矩阵


| 组件              | 执行位置                             | 做什么                            | **不**做什么 |
| --------------- | -------------------------------- | ------------------------------ | -------- |
| **Capture**     | 网关 `internal/ai_agent/pcap/`     | gopacket 解帧、串口 HEX 解析          | LLM 调用   |
| **Decoder**     | 网关（复用 `driver/*/decoder.go`）     | 字节 → 结构化字段；FC/地址/raw 提取        | 猜测物理量含义  |
| **Rule Engine** | AI Server（可同步规则至网关只读缓存）          | 协议 ID、端口模式、寄存器映射惯例、datatype 枚举 | 自然语言理解   |
| **LLM**         | AI Server                        | 语义理解、物理量推断、多值关联、命名             | **解码字节** |
| **Validator**   | 网关 `internal/ai_agent/validate/` | Schema、驱动规范、冲突检测               | 推理       |




### 3.3 设计原则

```text
确定性优先：能用 Rule + Decoder 解决的，不用 LLM
LLM 最小化：仅阶段三语义推理消耗 Token
知识沉淀：每次确认上线的映射写入 protocol_knowledge.db bbolt 桶（§8）
```

---



## §4 组件定位（EdgeX 架构中的位置）



### 4.1 架构分层：边缘网关 vs AI 推理中心

**EdgeX Industrial Protocol Copilot** 采用 **边缘网关 + AI 推理中心分离** 架构。网关侧运行 **Capture / Decoder / Task Agent**（`internal/ai_agent/`）；文档解析、RAG、LLM 路由、**protocol_knowledge.db（bbolt）** 在远端 **AI Model Center** 完成。

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│  UI：点位列表 · 设备配置 · Industrial Protocol Copilot 面板（新）            │
└───────────────────────────────┬─────────────────────────────────────────────┘
                                │ REST / WebSocket（异步任务）
┌───────────────────────────────▼─────────────────────────────────────────────┐
│  边缘网关（RK3588 / ARM64）— 工业自治域 · 热路径优先                          │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │  AI Agent Client  internal/ai_agent/                                     │ │
│  │  ┌──────────────┐ ┌─────────────────┐ ┌──────────────┐ ┌──────────────┐ │ │
│  │  │ Task Manager │ │ AI Gateway      │ │ Result       │ │ Human        │ │ │
│  │  │ 任务队列/状态 │ │ Client (gRPC/   │ │ Validator    │ │ Confirm      │ │ │
│  │  │ 机 · bbolt   │ │ MQTT/NATS)      │ │ Schema 校验  │ │ 确认落库     │ │ │
│  │  └──────┬───────┘ └────────┬────────┘ └──────┬───────┘ └──────┬───────┘ │ │
│  │  ┌──────▼───────────────────▼─────────────────▼────────────────┐         │ │
│  │  │ Capture + Decoder  internal/ai_agent/pcap/  （确定性 · 无 LLM）       │ │ │
│  │  └─────────────────────────────────────────────────────────────┘         │ │
│  │                         每日任务配额（UI）+ Token 计数（后端 · remote UI 可见）+ Audit Log   │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │  现有配置面（不变）                                                       │ │
│  │  ChannelManager · config.db · POST .../points/import                    │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │  数据面（热路径 · AI 禁止介入）                                           │ │
│  │  ScanEngine → ExecutionLayer → Driver.ReadPoints/WritePoint            │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
└───────────────────────────────┬─────────────────────────────────────────────┘
                                │ gRPC（主）/ MQTT·NATS（弱网）
┌───────────────────────────────▼─────────────────────────────────────────────┐
│  AI Model Center（独立服务器 / 私有云）                                       │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │  AI Service + Protocol Knowledge Base                                    │ │
│  │  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────────┐ │ │
│  │  │ Document     │ │ Rule Engine  │ │ LLM Router   │ │ protocol_        │ │ │
│  │  │ Parser       │ │ 协议识别/模式 │ │ 语义推理     │ │ knowledge.db     │ │ │
│  │  │ PDF/OCR/RAG  │ │ 寄存器惯例   │ │              │ │ (bbolt)          │ │ │
│  │  └──────────────┘ └──────────────┘ └──────────────┘ └──────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

**职责边界**：


| 侧                   | 模块路径                 | 职责                                                      | 禁止                            |
| ------------------- | -------------------- | ------------------------------------------------------- | ----------------------------- |
| **边缘网关**            | `internal/ai_agent/` | **Capture、Decoder**、任务编排、远端调用、结果校验、人工确认                 | LLM 推理、大文件 OCR、向量索引构建         |
| **AI Model Center** | AI Service（独立部署）     | 文档解析、Rule Engine、RAG、LLM 语义推理、**protocol_knowledge.db（bbolt 主库）** | 直接写 `config.db`、调度 ScanEngine |




### 4.2 与「Protocol Token Bucket」区分

ExecutionLayer 背压中的 **Token Rate** 指**协议 IO 令牌桶限流**，与 LLM **API Token 用量**无关。


| 术语                 | 含义                                  |
| ------------------ | ----------------------------------- |
| **LLM Token**      | 模型输入/输出计费单位；由 AI Model Center 统计    |
| **Protocol Token** | ScanEngine 执行层协议速率限制；**不**与 AI 模块共享 |




### 4.3 RK3588 / ARM64 资源约束


| 约束                     | 说明                                     |
| ---------------------- | -------------------------------------- |
| **ScanEngine 优先**      | 采集调度、驱动解码、Shadow 写入始终最高优先级             |
| **单 AI Worker**        | 每网关仅 **1** 个 AI Agent Worker goroutine |
| **无本地推理**              | RK3588 **不**部署 Ollama / vLLM           |
| **Capture/Decoder 本地** | PCAP 解帧、串口 HEX 解析在网关完成，**不经 LLM**      |
| **故障隔离**               | AI Model Center 不可达时，采集与规则 **零影响**     |


---



## §5 部署架构：边缘网关 + AI 推理中心分离



### 5.1 设计原则

```text
工业边缘自治 + AI 协同中心
├── 边缘网关：EdgeX 运行时 + Capture/Decoder + AI Agent Client
│   └── RK3588：ScanEngine / 驱动 / 报文解码 — 不跑 LLM
└── AI Model Center：PDF/OCR/RAG/LLM + protocol_knowledge.db（bbolt）
    └── 语义推理 + 文档结构化 — 不直接写 config.db
```


| 原则       | 说明                                      |
| -------- | --------------------------------------- |
| **推理外置** | LLM 仅在 AI Model Center                  |
| **解码下沉** | Capture + Decoder 在网关，复用驱动纯函数           |
| **工业优先** | AI 故障 **不得**影响 ScanEngine               |
| **默认禁云** | `enable_cloud=false`；仅 Mode C 显式开启公网 AI |




### 5.2 三种部署模式


| 模式         | 名称    | 拓扑                            | 适用场景         | 推荐度      |
| ---------- | ----- | ----------------------------- | ------------ | -------- |
| **Mode A** | 工业标准  | RK3588 网关 + **就近 AI Server**  | 单项目 1～20 台网关 | ⭐ **推荐** |
| **Mode B** | 企业私有  | 1 台 GPU Server 服务 **100+** 网关 | 集团多站点        | 企业级      |
| **Mode C** | 云端 AI | 网关 → 公网 AI API                | 演示、PoC       | 非工业默认    |


```text
Mode A（工业标准 · 推荐）
┌──────────────┐  gRPC/MQTT   ┌─────────────────────────────┐
│ RK3588 网关  │◄────────────►│ AI Server（同网段）          │
│ EdgeX        │  局域网      │ LLM + RAG + protocol_       │
│ Capture/     │              │ knowledge.db（bbolt）       │
│ Decoder/     │              │ + Rule Engine               │
│ Agent Client │              │                             │
└──────────────┘              └─────────────────────────────┘
```



### 5.3 通信协议（gRPC 主通道）

网关 `AI Gateway Client` ↔ AI Service `CopilotService`：

```protobuf
service CopilotService {
  rpc CreateTask(CreateTaskRequest) returns (CreateTaskResponse);
  rpc GetTask(GetTaskRequest) returns (GetTaskResponse);
  rpc StreamResult(StreamResultRequest) returns (stream StreamResultEvent);
  rpc CancelTask(CancelTaskRequest) returns (CancelTaskResponse);
}

message CreateTaskRequest {
  string gateway_id = 1;
  string skill = 2;            // protocol-reverse | doc-parse | point-gen | config-gen | ...
  string protocol_id = 3;      // 可选；留空则由 Rule Engine 识别
  bytes payload = 4;           // 解码摘要 JSON / 文档片段 / 观测值
  map<string, string> meta = 5;
}
```

- **默认端口**：`50051`；弱网降级 MQTT/NATS（见 V1.2 §2.3.2，保持不变）
- **任务状态机**：`pending → queued → processing → waiting_model → validating → waiting_confirm → applied | failed | cancelled`



### 5.4 网关侧模块结构


| 子模块                   | 路径                            | 职责                                  |
| --------------------- | ----------------------------- | ----------------------------------- |
| **Task Manager**      | `internal/ai_agent/task/`     | 任务队列、状态机、bbolt 持久化                  |
| **Capture + Decoder** | `internal/ai_agent/pcap/`     | gopacket 解帧、串口 HEX；**复用驱动 decoder** |
| **AI Gateway Client** | `internal/ai_agent/client/`   | gRPC / MQTT / NATS                  |
| **Result Validator**  | `internal/ai_agent/validate/` | 四类产出 Schema 校验                      |
| **Human Confirm**     | `internal/ai_agent/confirm/`  | Diff 预览、apply 审计                    |
| **Token 配额**          | `internal/ai_agent/quota/`    | 本地硬限 + AI Server 同步                 |




### 5.5 RK3588 资源保护（systemd）

```ini
CPUQuota=10%
MemoryMax=256M
Nice=10
IOSchedulingClass=idle
OOMScoreAdjust=500
```

验收：ScanEngine 1w Tag 压测时，AI Agent 满载 CPU ≤ 10%，lag P99 增幅 < 5%。

### 5.6 数据上传安全


| ✅ 允许                 | ❌ 禁止           |
| -------------------- | -------------- |
| 协议字段摘要（Decoder 输出）   | 原始 PCAP 全量（默认） |
| 用户选定文档分块             | 完整 `config.db` |
| 掩码或 IP（`192.168.1.*`） | 凭证、PLC 密码      |
| 待分析点位片段              | 已投产全量点表        |


---



## §6 Token 调用与配额管理

（配置项、`config.db` → `ai_copilot`、`runtime.db` → `ai_task` / `ai_token_usage` / `protocol_knowledge_cache` bbolt 结构、用量统计、任务分级路由矩阵 — 与 V1.2 §3 保持一致，知识库详见 §8。）

**UI 展示策略**（`AiQuotaBar.vue`）：

| 部署模式 | 配额栏展示 | 设置入口 |
| -------- | ---------- | -------- |
| **local**（本地 Mock） | 模式徽章 + 今日任务数 / 上限 + 进度条；**不展示 Token 环与用量数字** | 面板标题栏 ⚙ → `AiSettingsDialog` |
| **remote**（AI Model Center） | 模式徽章 + Token 环 + 用量 + 今日任务 | 同上；可配置 gRPC 端点 |
| **cloud**（直连 LLM API） | 同 remote（Token 可见） | 同上；需 `enable_cloud=true`；支持 OpenAI / Anthropic / Azure / DeepSeek / 通义 / 文心 / 智谱等 |

后端 `GET /api/ai/quota` 仍返回 `tokens_used` / `tokens_limit`（本地 Mock 用于硬限与审计）；仅 UI 在 local 模式下隐藏 Token 相关控件。


| 任务               | 执行位置                   | Token       |
| ---------------- | ---------------------- | ----------- |
| PCAP 解帧 / HEX 解析 | **网关 Capture+Decoder** | 无           |
| 协议识别 Rule Engine | AI Server              | 无           |
| datatype 枚举      | AI Server Rule Engine  | 无           |
| **物理量推理**        | AI Server LLM          | **有**（核心消耗） |
| 文档结构化            | AI Server LLM          | 有           |
| diagnostics 摘要   | AI Server 小模型或模板       | 低           |


---



## §7 核心技能规划

> **技能组织原则**：**§7.1 工业协议逆向工程引擎** 为模块 centerpiece；§7.2 文档解析为支撑能力；§7.3 生产配置生成为统一出口。



### 7.1 工业协议逆向工程引擎（Industrial Protocol Reverse Engineering Engine）— **P0+ 核心**

> **场景**：集成工程师**不知道精确点位地址**，但能从 HMI / 监控表获知**显示读数**（如 220.5 V、15.2 A），并持有 **PCAP / 串口 HEX**。引擎经四阶段流水线（§2）输出带置信度的 **Point Definition + Driver Parameter + Validation Case**。



#### 7.1.1 输入输出


| 输入                     | 必填  | 说明                   |
| ---------------------- | --- | -------------------- |
| PCAP / PCAPNG / 串口 HEX | ✅   | 网关本地 Capture+Decoder |
| 监控表或手动观测值              | 推荐  | 标签 + 显示值@时间 T        |
| 协议提示                   | 可选  | 可由 Rule Engine 从端口推断 |



| 输出（四类交付物）            | 说明                                                           |
| -------------------- | ------------------------------------------------------------ |
| **Protocol Model**   | `protocol_id`、帧特征、地址模型、byte_order 规则                         |
| **Point Definition** | `candidate_mappings[]`：地址、datatype、scale、confidence、evidence |
| **Driver Parameter** | Channel JSON：ip、port、slave_id 等                              |
| **Validation Case**  | 期望读数、容差、帧证据、unmatched_observations                           |




#### 7.1.2 工作流（与 §2/§3 对齐）

```text
① PCAP/HEX → 网关 Capture+Decoder → 协议字段摘要
② AI Server Rule Engine → protocol_id + candidate_fields[]
③ LLM 物理量推理 → point_candidates[]（语义关联，不解码字节）
④ 生产配置生成 → Channel JSON + Point JSON + Validation Case
⑤ 网关 Validator → UI Diff → Human Confirm → import
```



#### 7.1.3 协议覆盖


| 协议                 | MVP   | Decoder 要点                             |
| ------------------ | ----- | -------------------------------------- |
| **Modbus TCP/RTU** | ✅ P0+ | FC03/04 响应；MBAP/Unit ID；字节序变体          |
| **BACnet/IP**      | P1    | Who-Is/I-Am；ReadProperty present-value |
| **S7**             | P2    | Read Var DB 偏移                         |
| **EtherNet/IP**    | P2    | CIP Read Tag                           |
| **串口 HEX**         | P1    | Modbus RTU CRC + 透传帧                   |


**Modbus 主路径**：

1. Rule Engine 识别 TCP 502 + MBAP → `modbus-tcp`
2. Decoder 提取 FC03/04 请求-响应对 → `raw[]`
3. 枚举 datatype × endianness × scale → `candidate_fields[]`
4. LLM 关联 Uab/Ubc/Uca、Ia/Ib/Ic → 联合置信度
5. 生成 Point JSON + Channel JSON



#### 7.1.4 评分与关联


| 技术     | 说明                             |
| ------ | ------------------------------ |
| 数值指纹   | `                              |
| 轮询周期相关 | FC 请求间隔与 HMI 刷新对齐              |
| 多观测联合  | 六相电压/电流同时匹配 → 加分               |
| 寄存器邻域  | 连续地址合理物理量组合 → 加分               |
| 低置信度阻断 | `confidence < 0.6` 默认不勾选 apply |


```text
score = w1·value_match + w2·unit_plausibility + w3·polling_corr + w4·multi_tag_joint
confidence = sigmoid(score) · protocol_prior · knowledge_db_prior
```



#### 7.1.5 驱动集成


| 模块         | 路径                                  | 复用                          |
| ---------- | ----------------------------------- | --------------------------- |
| Modbus 解码  | `internal/driver/modbus/decoder.go` | `PointDecoder.Decode`、字节序   |
| BACnet 编解码 | `internal/driver/bacnet/encoding/*` | Who-Is/I-Am/ReadProp        |
| PCAP 解析    | `internal/ai_agent/pcap/`（新）        | gopacket；**不**链入 ScanEngine |
| 点位导入       | `POST .../points/import`            | Human Confirm 后             |




### 7.2 厂家文档与协议工件解析（Supporting · P0）

**输入**：PDF、DOC/DOCX、XLS/XLSX、CSV、PLC 变量表、HMI 点表、GSD/EDS 等。

#### 7.2.1 协议工件类型


| 类型              | 解析目标                                    | 场景             |
| --------------- | --------------------------------------- | -------------- |
| 寄存器表 / 点表       | `register_address`, `scale`, `datatype` | Scenario A     |
| 上位机监控表          | 标签名、描述、单位；有地址则直映；无地址则作 Scenario B 关联输入  | A + B          |
| PCAP            | 协议帧字段                                   | Scenario B 主输入 |
| PLC 符号 / TIA 导出 | S7 DB 偏移                                | Scenario A     |




#### 7.2.2 监控表示例（`test/上位机监控表PLC.csv`）

关键列映射：`Tag Name` → `id`；`I/O Address` → S7 地址；`Type` → `datatype`；`I/O Address` 缺失时归入 §7.1 逆向引擎。

#### 7.2.3 文档解析流水线

```text
Upload → MIME 探测
    ├─ 文档类 → 文本/表格/OCR → 分块 → AI Server 结构化 → Point + Driver Param
    ├─ PCAP → 网关 Decoder → 摘要 → §7.1 逆向引擎
    └─ 监控表 → 段头识别 → Common Variant 行解析 → 直映或关联输入
```



### 7.3 生产配置生成（统一出口 · P0）

所有技能最终产出对齐以下结构，经 Validator 后供 import。

#### 7.3.1 Protocol Model

```json
{
  "protocol_id": "modbus-tcp",
  "confidence": 0.95,
  "frame_pattern": {
    "transport": "tcp",
    "port": 502,
    "header": "mbap",
    "default_byte_order": "ABCD"
  },
  "address_model": "holding_register_4xxxx",
  "datatype_rules": ["float32@2regs", "uint16@1reg"],
  "conversion_rules": {"scale_range": [0.001, 0.01, 0.1, 1, 10]}
}
```



#### 7.3.2 Point Definition

```json
{
  "skill": "protocol-reverse",
  "protocol_id": "modbus-tcp",
  "points": [{
    "id": "uab",
    "name": "Uab线电压",
    "address": "40001",
    "register_type": "holding",
    "function_code": 3,
    "datatype": "float32",
    "byte_order": "ABCD",
    "scale": 0.1,
    "offset": 0,
    "unit": "V",
    "readwrite": "R",
    "scan_class": "normal",
    "slave_id": 1,
    "confidence": 0.87,
    "evidence": "FC03 rsp offset=0 raw=0x43DC6666 → 220.4V; polling 5s"
  }],
  "warnings": ["Ib 无唯一匹配，存在 3 个并列候选"]
}
```



#### 7.3.3 Driver Parameter（Channel JSON）

```json
{
  "protocol_id": "modbus-tcp",
  "name": "chiller-modbus-01",
  "connection": {
    "ip": "192.168.1.100",
    "port": 502,
    "slave_id": 1,
    "timeout_ms": 3000,
    "retries": 2
  },
  "scan_defaults": {
    "scan_class": "normal",
    "report_mode": "on_change"
  }
}
```



#### 7.3.4 Validation Case

```json
{
  "validation_cases": [{
    "point_id": "uab",
    "expected_value": 220.5,
    "tolerance_pct": 0.5,
    "observation_time": "2026-07-08T10:00:00+08:00",
    "frame_evidence": {
      "fc": 3,
      "start_addr": 0,
      "raw_hex": "43DC6666",
      "decoded": 220.4
    },
    "confidence": 0.87
  }]
}
```

**落库路径**（确认后）：

- `POST /api/channels/:channelId/devices/:deviceId/points/import`
- 通道配置经 ChannelManager API
- Validation Case 仅存 `ai_task` 审计，不入 `config.db`



### 7.4 点位校验与冲突检测

校验在 **AI 输出后、用户确认前** 由网关 `Result Validator` 执行：


| 校验项           | 规则                                  |
| ------------- | ----------------------------------- |
| 地址格式          | 对照 `protocol_id` 与驱动 `decoder_test` |
| 数据类型          | `datatype` ∈ 驱动支持集                  |
| 功能码           | Modbus FC 与 `register_type` 一致      |
| ID 唯一         | 同设备内不重复                             |
| ScanEngine 负载 | `fast` 类点位过多时警告                     |
| 低置信度          | `confidence < 0.6` 强制人工复核           |




### 7.5 边缘计算垂直场景辅助（P1 · 辅助）

EdgeRule 草稿、场景模版扩展、expr 子集检查 — 与 V1.2 §4.4 相同，**非模块核心**。

### 7.6 联调与诊断辅助（P1 · 辅助）

diagnostics 摘要 → 排查清单 — 与 V1.2 §4.5 相同，**非模块核心**。

---



## §8 protocol_knowledge.db（协议知识库 · bbolt）



### 8.1 定位

**protocol_knowledge.db** 存储跨项目的协议模式与设备映射经验，随每次 Human Confirm 上线 **持续沉淀**。采用项目既有 **bbolt 单文件 + Bucket + JSON 值** 模式（对齐 `internal/storage/config_store.go` · `boltdb.go`），**不引入 SQLite**。部署于 **AI Server** 读写主库；网关侧通过 `runtime.db` 只读缓存 bucket 供 Rule Engine 离线增强。

### 8.2 存储位置与双库分工


| 侧 | 文件 / Bucket | 模式 | 对齐现有基础设施 |
| --- | --- | --- | --- |
| **AI Server（主库）** | `data/protocol_knowledge.db`（**bbolt**） | 读写；`NoGrowSync: false` 强一致 | 复用 `openBoltDB` + `SaveData`/`GetData` JSON 序列化 |
| **网关 — Copilot 配置** | `config.db` → `ai_copilot` bucket | 读写；AI Server 端点、部署模式、配额 | 对齐 `ConfigStore.saveJSON` 单键模式 |
| **网关 — 任务审计** | `runtime.db` → `ai_task` bucket | 读写；任务状态机、Validation Case 审计 | 对齐 `edge_event_recorder` 异步写模式 |
| **网关 — Token 用量** | `runtime.db` → `ai_token_usage` bucket | 读写；本地配额计数 | 可清理 bucket |
| **网关（可选缓存）** | `runtime.db` → `protocol_knowledge_cache` bucket | **只读快照**；定期 pull | 对齐 `runtime.db` 可 compact 治理策略 |

> **设计原则**：知识主库独立于 `config.db` / `runtime.db`（跨项目共享、部署在 AI Server），但 **Bucket 命名、Key 编码、JSON Value 格式** 与网关双库完全一致，便于快照同步与 `internal/storage` 代码复用。

### 8.3 Bucket 布局（AI Server `protocol_knowledge.db`）

| Bucket | Key 格式 | Value（JSON） | 说明 |
| --- | --- | --- | --- |
| `KnowledgeVersion` | `version` | `"1.0"` 或单调递增 `uint64` 字符串 | 对齐 `ConfigVersion` bucket；快照版本号 |
| `protocol_pattern` | `{id}` | `ProtocolPattern` | 协议帧模式；`id` 建议 `modbus-tcp\|tcp\|502` |
| `manufacturer` | `{manufacturer}` | `Manufacturer` | 厂家元数据；key 小写归一化 |
| `device_model` | `{manufacturer}\|{model_name}` | `DeviceModel` | 设备型号；含 `default_params`、`project_count` |
| `register_mapping` | `{manufacturer}\|{model_name}\|{address}` | `RegisterMapping` | 沉淀自 confirm 的点位映射 |
| `datatype_rules` | `{protocol_id}\|{raw_pattern}` | `DatatypeRule` | 如 `modbus-tcp\|2_regs_float` |
| `byte_order_rules` | `{protocol_id}\|{manufacturer}` | `ByteOrderRule` | 厂家缺省用 `_default`：`modbus-tcp\|_default` |
| `conversion_rules` | `{protocol_id}\|{semantic_type}` | `ConversionRule` | 如 `modbus-tcp\|voltage` |

**Key 编码约定**（对齐 `Devices` bucket 的 `{device_id}` 与 `bblot` 的 `{ruleID}_{minute}` 模式）：

- 分隔符 `|`（管道符），字段内禁止含 `|`
- `manufacturer`、`model_name`、`address` 写入前 `strings.ToLower` + trim
- `address` 保留驱动原生格式（如 `40001`、`DB1.DBD0`）

**Value 示例**（`register_mapping`）：

```json
{
  "id": "rm_8f3a2b",
  "manufacturer": "siemens",
  "model_name": "s7-1200",
  "point_semantic": "Uab线电压",
  "protocol_id": "modbus-tcp",
  "address": "40001",
  "register_type": "holding",
  "function_code": 3,
  "datatype": "float32",
  "byte_order": "ABCD",
  "scale": 0.1,
  "offset": 0,
  "unit": "V",
  "confidence": 0.92,
  "source": "protocol-reverse",
  "evidence": "pcap_offset_12",
  "confirmed_at": "2026-07-08T10:30:00Z",
  "gateway_id": "gw-rk3588-01"
}
```

**写入 API**（AI Server 侧，复用项目模式）：

```go
// 对齐 internal/storage/boltdb.go SaveData
store.SaveData("register_mapping", "siemens|s7-1200|40001", mapping)
store.SaveData("KnowledgeVersion", "version", fmt.Sprintf("%d", newVersion))
```

### 8.4 网关侧 Bucket 集成

#### 8.4.1 `config.db` → `ai_copilot`

| Key | Value | 说明 |
| --- | --- | --- |
| `settings` | `AICopilotSettings` JSON | AI Server 端点、部署模式、配额上限 |
| `knowledge_snapshot_version` | `uint64` 字符串 | 本地已同步的知识库版本号 |

#### 8.4.2 `runtime.db` → `ai_task`

| Key | Value | 说明 |
| --- | --- | --- |
| `{task_id}` | `AITaskRecord` JSON | 任务状态机、四类产出摘要、Validation Case |
| `{task_id}_audit` | `ApplyAudit` JSON | Human Confirm apply 审计链 |

Key 示例：`task_20260708_abc123`（对齐任务 ID 唯一性）。

#### 8.4.3 `runtime.db` → `protocol_knowledge_cache`

结构与 AI Server 主库 **同 Bucket 名 + 同 Key 编码**，Value 为只读 JSON 快照副本。Rule Engine 在 AI Server 不可达时从此 bucket 检索先例。

### 8.5 快照同步机制（网关 ← AI Server）

```text
AI Server protocol_knowledge.db
    KnowledgeVersion.version = N
    ↓  gRPC SyncKnowledgeSnapshot（或 MQTT 增量）
网关 runtime.db → protocol_knowledge_cache
    同 Key 全量/增量 UPSERT
    ai_copilot.knowledge_snapshot_version = N
    ↓
Rule Engine 离线检索 → confidence_prior 加权
```

| 步骤 | 行为 |
| --- | --- |
| ① 版本探测 | 网关定时（默认 1h）或任务创建前调用 `GetKnowledgeVersion` |
| ② 增量拉取 | `version` 不一致时拉取变更 Key 列表 + Value JSON |
| ③ 本地写入 | `SaveData("protocol_knowledge_cache", key, value)` 批量事务 |
| ④ 版本落盘 | 更新 `ai_copilot` → `knowledge_snapshot_version` |
| ⑤ 治理 | 缓存随 `runtime.db` 可 `compact-runtime`；pull 可重建 |

> **铁律**：网关缓存 **只读**；知识沉淀 UPSERT 仅在 AI Server 主库执行（confirm apply 回流后）。

### 8.6 增长与检索

```text
Human Confirm → apply import（网关 config.db）
    ↓
ai_task 审计写入（runtime.db）
    ↓
gRPC 回流 → AI Server register_mapping UPSERT
    device_model.project_count++
    KnowledgeVersion.version++
    ↓
RAG 检索：新任务按 protocol_id + manufacturer + semantic 查 Top-K 先例
    ↓
Rule Engine confidence_prior 加权；LLM Prompt 注入映射片段
```

**检索路径**：

| 场景 | 数据源 |
| --- | --- |
| AI Server 在线 | `protocol_knowledge.db` 主库直接 `LoadRange` / 向量索引 |
| AI Server 离线 | 网关 `protocol_knowledge_cache` bucket |
| 冷启动种子 | `docs/drivers/*`、`pointTemplates.json` 导入主库 |

### 8.7 与现有 Bucket 关系总览

```text
config.db
├── Channels / Devices / …（现有配置面）
└── ai_copilot          ← Copilot 设置 + 知识快照版本

runtime.db
├── values / RuleState / …（现有运行时）
├── ai_task             ← 任务状态 + Validation Case 审计
├── ai_token_usage      ← Token 配额计数
└── protocol_knowledge_cache  ← 知识库只读快照（pull from AI Server）

protocol_knowledge.db（AI Server 独立 bbolt 文件）
├── KnowledgeVersion
├── protocol_pattern / manufacturer / device_model
├── register_mapping / datatype_rules / byte_order_rules
└── conversion_rules
```

---



## §9 与现有模块集成点


| 模块          | 路径 / API                              | 集成方式                                                |
| ----------- | ------------------------------------- | --------------------------------------------------- |
| **南向采集**    | `scan_engine.go`                      | 只读 diagnostics；AI 不调度采集                             |
| **通道与驱动**   | `channel_manager.go`                  | 读取 `protocol_id`；生成 Driver Parameter                |
| **点位读写**    | `ChannelManager.AddPoints`            | Confirm 后 `points/import`                           |
| **协议解码**    | `internal/driver/modbus/decoder.go` 等 | Capture+Decoder 冷路径复用                               |
| **PCAP 解析** | `internal/ai_agent/pcap/`             | gopacket 离线解帧                                       |
| **UI**      | `AiAssistantPanel.vue` / `AiQuotaBar.vue` / `AiSettingsDialog.vue` | Industrial Protocol Copilot 面板：上传 PCAP/文档、预览配置、确认导入；**local 模式配额栏仅显示今日任务**；**面板标题栏齿轮按钮**打开 AI 设置（部署模式、平台、认证、配额） |




### 9.1 建议 API 草案


| 方法   | 路径                              | 说明                                                                 |
| ---- | ------------------------------- | ------------------------------------------------------------------ |
| POST | `/api/ai-agent/tasks`           | 创建任务（`skill`: `**protocol-reverse**` / `doc-parse` / `config-gen`） |
| GET  | `/api/ai-agent/tasks/:id`       | 四类产出预览 + 置信度                                                       |
| POST | `/api/ai-agent/tasks/:id/apply` | Human Confirm → import Channel + Points                            |
| GET  | `/api/ai-agent/usage`           | Token 用量                                                           |
| GET  | `/api/ai/settings`              | 读取 AI Copilot 设置（密钥脱敏）                                         |
| PUT  | `/api/ai/settings`              | 保存部署模式 / 平台 / 端点 / 认证 / 配额                                   |
| PUT  | `/api/ai-agent/settings`        | 部署模式 / AI Server 端点（与 `/api/ai/settings` 对齐，后续合并）              |


---



## §10 技能清单与优先级

> **V1.4 变更**：协议知识库由 SQLite 改为 **bbolt**（§8），复用 `internal/storage` 双库模式；网关 `protocol_knowledge_cache` 只读快照同步。  
> **V1.3 变更**：S15 升格为模块 centerpiece；文档解析降为支撑；新增 S19 生产配置生成、S20 protocol_knowledge 沉淀。


| ID      | 技能                                                             | 优先级        | 依赖                                  |
| ------- | -------------------------------------------------------------- | ---------- | ----------------------------------- |
| **S15** | **工业协议逆向工程引擎（Modbus TCP/RTU + HEX + 监控表关联）**                   | **P0+ 核心** | Capture+Decoder + Rule Engine + LLM |
| **S19** | **生产配置生成（Protocol Model + Point + Channel + Validation Case）** | **P0+**    | S15 + Point/Channel Schema          |
| S3      | 点位 Schema 校验与冲突检测                                              | **P0**     | S19 + 驱动规范库                         |
| S4      | UI 预览 + import API 对接                                          | **P0**     | S3                                  |
| S5      | Token Manager + 配额 + 审计                                        | **P0**     | config.db                           |
| S1      | 文档上传与文本/表格抽取                                                   | **P0 支撑**  | AI Server                           |
| S2      | Modbus 寄存器表 → 点位 JSON                                          | **P0 支撑**  | S1 + S19                            |
| **S20** | **protocol_knowledge.db（bbolt）沉淀与检索**                          | **P1**     | S15 confirm 回流                      |
| S6      | 上位机监控表 CSV/Excel 解析                                            | **P1**     | S1                                  |
| **S16** | 协议逆向 — BACnet ReadProperty                                     | **P1**     | S15 + `bacnet/encoding`             |
| S7      | S7 / BACnet / OPC UA 点表生成                                      | **P1**     | 各驱动文档                               |
| S8      | EdgeRule / 场景模版草稿                                              | **P1 辅助**  | edgeSceneTemplates                  |
| S9      | diagnostics 联调助手                                               | **P1 辅助**  | diagnostics API                     |
| S10     | PDF 扫描件 OCR                                                    | **P2**     | AI Server VLM                       |
| S11     | gRPC Client + MQTT 弱网回退                                        | **P2**     | Mode A                              |
| S17     | 协议逆向扩展（S7 / EIP）                                               | **P2**     | S15                                 |
| S18     | Mode B 多网关队列调度                                                 | **P2**     | AI Service 扩展                       |


---



## §11 技术方案概要



### 11.1 RAG 与知识增强（AI Server）

```text
文档片段 / 解码摘要 / protocol_knowledge.db（bbolt）先例
    → Chunk + 协议标签（modbus_register_map · pcap_session · hmi_tag）
    → Embedding → 向量库
    → Top-K + 重排序 → 注入 LLM Prompt（仅阶段三语义推理）
```

冷启动语料：`docs/drivers/*`、`pointTemplates.json`、`protocol_knowledge.db`（bbolt）种子数据导入。

### 11.2 结构化输出约束

- AI Server **JSON Schema / 函数调用** 约束四类产出
- 网关 `Result Validator` **三次校验**
- 不通过：自动重试（≤2 次）或降级为「仅 Validation Case、不生成 Point」



### 11.3 Human-in-the-loop

```text
① 上传 PCAP / 文档 / 选择协议
② 网关 Capture+Decoder 预处理 → 提交 AI Server
③ 四阶段流水线 → 四类产出 JSON
④ Validator → UI Diff（新增/冲突/低置信度）
⑤ 用户编辑 / 确认
⑥ apply → Channel + Point import + protocol_knowledge.db bbolt 沉淀（AI Server）
```

**铁律**：步骤 ⑥ 之前 **零** `config.db` 写操作。

---



## §12 安全与合规

（与 V1.2 §8 保持一致，补充：）


| 要求            | 措施                                |
| ------------- | --------------------------------- |
| **LLM 不解码字节** | Decoder 与 Rule Engine 承担全部字节解析    |
| **被动抓包**      | 默认离线 PCAP；禁止未授权主动扫寄存器             |
| **知识库脱敏**     | `register_mapping` 不含客户项目名称；仅技术映射 |
| **AI 故障隔离**   | AI Server 不可达时采集零影响               |


---



## §13 分阶段实施路线图

### V2.0 新增：EAN 2.0 Capability Runtime 阶段

| 阶段 | 时间 | 交付 | 验收 |
|------|------|------|------|
| **EAN-MVP** | 2026 Q3 | Capability Registry（自动生成）+ Invoke Dispatcher + Discovery Publisher + Event Publisher；MQTT `$edgeos/*` Topic 接入；保留 V1.0 `edgex/*` 兼容层 | EdgeX 启动后自动发布 Agent + Capability Descriptor；EdgeOS 可发现 EdgeX Capability；Invoke 请求可路由到 Driver；V1.0 Topic 100% 兼容 |
| **EAN-增强** | 2026 Q4 | AI Adapter 升级（Capability Planner）；MCP Adapter 升级（Capability → Tool 自动生成）；ShadowCore Capability 缓存；EAN 2.0 完整协议实现（Discovery/Invoke/Event/State） | AI 规划输出 Capability Invoke 而非直接 MCP Tool；MCP Tool 清单自动同步 Capability；Shadow 优先命中 ≥ 90%；协议一致性测试通过 |
| **EAN-企业级** | 2027 Q1 | Capability SDK 发布；跨 EdgeOS 集群 Agent 发现；Workflow Center 集成；Resource Manager 工业资源锁 | 第三方可通过 SDK 调用 EdgeX Capability；多 EdgeOS 节点 Agent 注册同步；Workflow 可编排跨 Agent Capability；PLC/COM 资源锁无冲突 |
| **EAN-持续** | — | A2A / OpenAI Agent 协议适配器；Capability 市场索引 | 外部 AI Agent 可发现/调用 EdgeX Capability |

```text
EAN-MVP ──► EAN-增强 ──► EAN-企业级
    │           │            │
    │           │            └─ SDK / 跨集群 / Workflow
    │           │
    │           └─ AI Planner / MCP Auto-Tool / Shadow Cache
    │
    └─ Capability Registry / Dispatcher / Discovery / Event
       $edgeos/* Topic / V1.0 兼容
```

### V1.5 既有能力路线（保留）

| 阶段      | 时间      | 交付                                                                                     | 验收                                                                                                      |
| ------- | ------- | -------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| **MVP** | 2026 Q3 | **S15+S19+S3–S5** + Mode A：Modbus PCAP/HEX 逆向 → **30min 候选配置**；文档点表导入；gRPC；四类产出 import | 无文档设备：PCAP+6 点显示值 → 30min 内候选 Channel+Point JSON；confirm → Shadow 与 HMI 一致（ε≤1%）；`test/` 夹具 Top-1 ≥ 70% |
| **增强**  | 2026 Q4 | S6/S16/S20 + MQTT 弱网；BACnet 逆向；监控表多格式；protocol_knowledge.db bbolt 沉淀                                  | 3 项目试点；断网 `ai_task` 自动重投；`protocol_knowledge_cache` 离线检索 |
| **企业级** | 2027 Q1 | S10/S17/S18：OCR、S7/EIP 逆向、Mode B 多网关                                                   | 100+ 网关共享 GPU Server                                                                                    |
| **持续**  | —       | protocol_knowledge.db 随项目增长；ROADMAP Phase 4 对齐                                                  | 月度 Token 成本报告                                                                                           |


```text
MVP ──► 增强 ──► 企业级
 │        │          │
 逆向引擎  BACnet     S7/EIP
 30min    bbolt知识  Mode B
 生产配置  沉淀       多网关
 Mode A   MQTT弱网
```

---



## §14 验收标准与成功指标



### 14.1 架构验收


| #   | 标准                                     |
| --- | -------------------------------------- |
| A1  | AI Agent **零**注册为 DataPipeline handler |
| A2  | 未 confirm 的产出 **零** `config.db` 写入     |
| A3  | AI Server 不可达 **不**影响采集                |
| A4  | LLM **不**参与字节解码（代码审查 + 日志验证）           |
| A5  | Capture+Decoder 在网关本地完成，不上传原始 PCAP     |
| A6  | RK3588 **零** LLM 进程；AI Agent CPU ≤ 10% |




### 14.2 功能验收（生产配置导向）


| #   | 标准                                                                                 |
| --- | ---------------------------------------------------------------------------------- |
| F1  | Scenario A：Modbus 寄存器表 → Point+Channel JSON → import → Shadow 有值                   |
| F2  | **Scenario B**：Modbus PCAP + 显示值 → **30min** 内四类产出 → confirm → import → 读数与 HMI 一致 |
| F3  | 冲突检测：重复 address 阻断 apply                                                           |
| F4  | Validation Case 可回放：证据链含帧偏移与 decoded 值                                             |
| F5  | Mode A gRPC 小任务端到端 < 500ms（不含 LLM 等待）                                              |
| F6  | protocol_knowledge.db（bbolt）：confirm 后映射可检索并提升后续任务置信度                                     |




### 14.3 成功指标（试点 90 天）


| 指标             | 目标                                |
| -------------- | --------------------------------- |
| **无文档设备接入工时**  | 2 天 → **≤ 4h**（含 30min AI + 人工确认） |
| 有文档点表集成工时      | 较纯人工 ↓ ≥ 50%                      |
| 首次采集成功率        | AI 辅助导入设备 ≥ 85%（24h Quality Good） |
| 候选配置 apply 率   | ≥ 70%                             |
| 误配置事故          | 0 起「未确认自动写 PLC」                   |
| ScanEngine 稳定性 | AI 满载时 lag P99 增幅 < 5%            |


---



## §15 风险与依赖


| 风险               | 影响  | 缓解                               |
| ---------------- | --- | -------------------------------- |
| LLM 幻觉（语义层）      | 高   | 仅用于阶段三；Decoder 提供确定性证据；低置信度阻断    |
| 逆向误匹配            | 高   | 多观测联合 + protocol_knowledge_cache 先例 + 人工并列候选 |
| Rule Engine 覆盖不足 | 中   | 协议模式持续沉淀；人工协议选择兜底                |
| Token 成本         | 中   | LLM 最小化；本地 Decoder 优先            |
| 数据合规             | 高   | §5.6 白名单 + `enable_cloud=false`  |


**依赖**：点位读写 TODO、驱动 `decoder_test`、`test/*.pcap` · `test/上位机监控表PLC.csv`、AI Model Center 部署包。

---



## §16 与项目技能规划对齐（Cursor / 集成商交付）

建议在 `.cursor/skills/` 维护 **EdgeX Industrial Protocol Copilot Skills**：


| Skill                            | 触发场景                                     |
| -------------------------------- | ---------------------------------------- |
| `edgex-protocol-reverse`         | PCAP/HEX + 显示值 → 四类生产配置（**P0+ 核心**）      |
| `edgex-protocol-identify`        | 抓包摘要 → Rule Engine 协议识别                  |
| `edgex-doc-to-modbus-points`     | Modbus 手册 → Point+Channel JSON           |
| `edgex-hmi-table-parse`          | 上位机监控表 → 结构化标签 / 逆向关联输入                  |
| `edgex-config-gen`               | 候选点位 → Channel+Point+Validation Case 完整包 |
| `edgex-point-validate`           | 现有点表冲突与 Scan Class 审查                    |
| `edgex-protocol-knowledge-query` | 检索 protocol_knowledge.db（bbolt）先例              |
| `edgex-edge-rule-draft`          | 场景描述 → EdgeRule（辅助）                      |
| `edgex-diagnostics-explain`      | diagnostics JSON → 排查步骤（辅助）              |


技能定义应引用 **§9 API** 与 **四类交付物 Schema**，产品名统一为 **EdgeX Industrial Protocol Copilot**。

---



## 附录：交叉引用

- 架构数据面：[TODO 索引 §1](./index.md)
- bbolt 双库架构：[edgex-db-runtime-architecture.md](../operations/edgex-db-runtime-architecture.md)
- RK3588 约束：本文 §4.3 · §5.5
- 点位模型：`internal/model/types.go`
- Modbus 解码：`internal/driver/modbus/decoder.go`
- BACnet：`internal/driver/bacnet/encoding/whois.go`
- **EAN 2.0 通信协议规范**：[EdgeX通信协议规范(MQTT-NATS) V2.0](../edgeos/EdgeX通信协议规范(MQTT-NATS).md)

---

## 附录 E：EAN 2.0 新增技能清单（V2.0）

建议在 `.cursor/skills/` 维护 **EAN 2.0 Capability Runtime Skills**：

| Skill | 触发场景 |
|-------|---------|
| `ean-capability-registry` | 自动生成/注册 Capability |
| `ean-invoke-dispatcher` | 统一 Capability 调用路由 |
| `ean-discovery-publisher` | Agent/Capability Descriptor 发布 |
| `ean-event-publisher` | 设备事件自动发布到 EAN |
| `ean-mcp-adapter` | Capability → MCP Tool 自动映射 |
| `ean-ai-planner` | AI 规划输出 Capability Invoke |
| `ean-shadow-cache` | ShadowCore Capability 缓存查询 |

---

## 附录 F：EAN 2.0 代码路径规划

| 模块 | 代码路径 | 说明 |
|------|---------|------|
| Capability Registry | `internal/capability/registry.go` | Capability 自动生成与本地缓存 |
| Invoke Dispatcher | `internal/capability/dispatcher.go` | 统一调用入口与路由 |
| Discovery Publisher | `internal/capability/discovery_publisher.go` | Agent/Capability Descriptor 发布 |
| Event Publisher | `internal/capability/event_publisher.go` | 事件自动发布 |
| AI Adapter | `internal/ai_agent/planner/` | Capability Planner |
| MCP Adapter | `internal/server/mcp_capability_adapter.go` | Capability → Tool 映射 |
| Execution Mapper | `internal/execution/capability_mapper.go` | Capability → Driver Command |
| Shadow Cache | `internal/shadow/capability_cache.go` | Capability 结果缓存 |
| EAN SDK | `pkg/eansdk/` | 面向 Capability 的 SDK |
| 协议类型 | `internal/capability/types.go` | EAN Agent/Capability/Invoke/Event 类型（**不**放在 MCP protocol.go，避免与 MCP JSON-RPC 混淆） |

---

## 附录 G：EAN 2.0 验收标准（V2.0 新增）

### G.1 架构验收

| # | 标准 |
|---|------|
| E1 | Capability Registry 自动生成所有 Driver Command 对应的 Capability |
| E2 | Invoke Dispatcher 统一路由 MQTT/HTTP/SDK/MCP/Workflow 到 Driver |
| E3 | Discovery Publisher 启动时自动发布 Agent + Capability Descriptor |
| E4 | Event Publisher 点位变化时自动发布 `$edgeos/event/{agent_id}` |
| E5 | V1.0 `edgex/*` Topic 100% 兼容，业务无感知 |
| E6 | AI 故障 / EdgeOS 离线时，EdgeX 采集与规则执行零影响 |

### G.2 功能验收

| # | 标准 |
|---|------|
| EF1 | EdgeOS 可通过 `$edgeos/discovery/capability` 查询 EdgeX 的所有 Capability |
| EF2 | EdgeOS 可通过 `$edgeos/invoke/{agent_id}` 调用 EdgeX Capability 并收到响应 |
| EF3 | MCP Tool 清单自动同步 Capability，新增驱动无需手动维护 Tool |
| EF4 | AI Planner 输出 Capability Invoke 而非直接 MCP Tool 调用 |
| EF5 | ShadowCore Capability 缓存命中时，不触发 Driver 读取 |
| EF6 | Graceful Shutdown 时发布 Agent Offline，EdgeOS 正确感知 |

### G.3 性能验收

| # | 标准 |
|---|------|
| EP1 | Invoke Dispatcher 路由延迟 < 1ms（本地 Capability） |
| EP2 | Discovery Publisher 启动发布延迟 < 5s |
| EP3 | Event Publisher 发布延迟 < 5ms（异步） |
| EP4 | Shadow Capability 缓存命中时读延迟 < 1ms |
| EP5 | ScanEngine 稳定性：Capability Runtime 满载时 lag P99 增幅 < 3% |



### 附录：测试夹具（`test/`）


| 文件                        | 用途                |
| ------------------------- | ----------------- |
| `test/上位机监控表PLC.csv`      | 监控表解析 + S15 关联输入  |
| `test/BACnet发现报文.pcap`    | BACnet 协议识别 + S16 |
| `test/单播who-is.pcap`      | Who-Is/I-Am 逆向    |
| `test/slave1_points.json` | 逆向结果对照基准          |


---
<br>

## §17 MCP (Model Context Protocol) 接入

> **V1.5 新增 / V1.7 更新**：MCP 协议允许外部 LLM 应用（Claude Desktop、Cursor、Windsurf、Continue.dev 等）通过标准 JSON-RPC 2.0 协议安全操作 EdgeX 工业网关。MCP 提供 **33 个工具**（8 只读 + 1 写操作 + 24 全功能 CRUD）、**6 个资源端点**、**13 个提示词模板**。支持 MCP 2024-11-05 与 2025-11-25（Streamable HTTP）两个协议版本。全功能操作需用户显式确认激活。

### 17.1 定位

MCP 是 EdgeX 对外 AI 协同的**标准协议接口**。与 AI Agent 内部协同（§0-§16）不同，MCP 面向**外部 LLM 应用**，提供工业网关的远程操作能力。两者互补：

| 维度 | AI Agent（内部协同） | MCP（对外接口） |
|------|---------------------|----------------|
| 调用方 | EdgeX UI / 内部任务 | 外部 LLM 应用（Claude Desktop 等） |
| 协议 | gRPC / MQTT / REST | MCP (JSON-RPC 2.0 over HTTP/SSE) |
| 认证 | 系统 JWT | MCP API Key（独立密钥） |
| 能力范围 | 协议逆向、文档解析、配置生成 | 通道/设备/点位 CRUD、边缘规则、批量读写测试 |
| 安全门控 | Human-in-the-loop 确认 | 用户 UI 激活全功能 + API Key 认证 |

### 17.2 架构

```text
┌─────────────────────────────────────────────────────────────────────────┐
│  外部 LLM 应用（Claude Desktop / Cursor / Windsurf / Continue.dev）    │
│  MCP Client ── JSON-RPC 2.0 ── HTTP/SSE                                │
└───────────────────────────────┬─────────────────────────────────────────┘
                                │ Authorization: Bearer <mcp_api_key>
                                │ 或 X-MCP-API-Key: <mcp_api_key>
┌───────────────────────────────▼─────────────────────────────────────────┐
│  EdgeX 网关                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │  MCP Handler  internal/server/mcp_handler.go                        │ │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐  │ │
│  │  │ 只读工具 (8个)    │  │ 操作工具 (25个)   │  │ 资源 (6个)       │  │ │
│  │  │ list_channels    │  │ write_point      │  │ edgex://channels │  │ │
│  │  │ list_devices     │  │ create_channel   │  │ edgex://system   │  │ │
│  │  │ list_points      │  │ start/stop_chan  │  │ edgex://diag..   │  │ │
│  │  │ read_point       │  │ create_device    │  │ edgex://protocol │  │ │
│  │  │ get_system_info  │  │ update_device    │  │ edgex://rules    │  │ │
│  │  │ get_diagnostics  │  │ create_point     │  │ edgex://config   │  │ │
│  │  │ analyze_protocol │  │ read/write_batch │  └──────────────────┘  │ │
│  │  │ get_protocol_help│  │ create_edge_rule │                        │ │
│  │  └──────────────────┘  │ delete_edge_rule │  ┌──────────────────┐  │ │
│  │                        │ create_virtual   │  │ 提示词 (13个)     │  │ │
│  │                        │ restart_channel  │  │ protocol-reverse │  │ │
│  │                        │ export_config    │  │ channel-config   │  │ │
│  │                        │ ... +15 more     │  │ modbus-quick-str │  │ │
│  │                        └──────────────────┘  │ ... +10 more     │  │ │
│  │                                              └──────────────────┘  │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │  ChannelManager · EdgeComputeManager · VirtualShadowEngine         │ │
│  │  config.db · runtime.db（现有核心引擎）                              │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────┘
```

### 17.3 认证与权限模型

MCP 采用**独立于系统 JWT 的简化认证机制**：

| 层级 | 说明 |
|------|------|
| **API Key 认证** | MCP 客户端通过 `Authorization: Bearer <mcp_api_key>` 或 `X-MCP-API-Key: <mcp_api_key>` 认证 |
| **只读权限** | 默认状态：`edgex_list_*`、`edgex_read_point`、`edgex_get_*`、`edgex_analyze_*` 等 8 个工具可用 |
| **写操作** | `edgex_write_point` 需人工确认，不自动执行（1 个工具） |
| **全功能权限** | 用户通过 UI 显式激活后：创建/删除通道、设备、点位、边缘规则、虚拟设备、批量读写等 24 个工具可用 |
| **API Key 管理** | 在 EdgeX UI → AI 助手 → MCP 接入页面设置，支持随时更换；`POST /api/mcp/generate-key` 生成 256 位随机密钥 |
| **会话管理** | MCP 2025-11-25 Streamable HTTP 会话通过 `Mcp-Session-Id` 头管理，SSE 每 30s 心跳 |

**安全铁律**：
- 全功能 CRUD 操作**必须**经用户 UI 确认激活
- MCP API Key 未设置时，MCP 端点拒绝所有请求
- 创建/删除操作受 `mcpRequireFullAccess()` 门控，未激活时返回明确错误提示

### 17.4 工具清单

#### 只读查询工具（8 个，无需全功能激活）

| 工具名 | 功能 | 参数 |
|--------|------|------|
| `edgex_list_channels` | 列出所有采集通道及其状态 | 无 |
| `edgex_list_devices` | 列出指定通道下的所有设备 | `channel_id` |
| `edgex_list_points` | 列出指定设备下的所有点位（含当前值） | `channel_id`, `device_id` |
| `edgex_read_point` | 读取指定点位的当前实时值 | `channel_id`, `device_id`, `point_id` |
| `edgex_get_system_info` | 获取网关系统信息（CPU/内存/协议支持） | 无 |
| `edgex_get_diagnostics` | 获取通道或设备诊断信息 | `channel_id`（可选）, `device_id`（可选） |
| `edgex_analyze_protocol` | 分析工业协议特征（端口/名称匹配） | `protocol_hint`, `port`, `description` |
| `edgex_get_protocol_help` | 获取协议接入帮助（地址格式/功能码/配置示例） | `protocol` |

#### 写操作工具（1 个，需人工确认）

| 工具名 | 功能 | 参数 |
|--------|------|------|
| `edgex_write_point` | 向 R/W 点位写入控制值（需人工确认，不自动执行） | `channel_id`, `device_id`, `point_id`, `value` |

#### 全功能 CRUD 工具（24 个，需用户激活全功能）

**通道管理（4 个）**

| 工具名 | 功能 | 参数 |
|--------|------|------|
| `edgex_create_channel` | 创建南向采集通道 | `name`, `protocol`, `config` |
| `edgex_delete_channel` | 删除通道（含设备和点位） | `channel_id` |
| `edgex_start_channel` | 启动通道采集引擎 | `channel_id` |
| `edgex_stop_channel` | 停止通道采集引擎 | `channel_id` |

**设备管理（4 个）**

| 工具名 | 功能 | 参数 |
|--------|------|------|
| `edgex_create_device` | 在通道下创建设备 | `channel_id`, `name`, `config` |
| `edgex_delete_device` | 删除设备（含点位） | `channel_id`, `device_id` |
| `edgex_update_device` | 更新设备配置 | `channel_id`, `device_id` |
| `edgex_enable_device` | 启用/禁用设备 | `channel_id`, `device_id`, `enable` |

**点位管理（5 个）**

| 工具名 | 功能 | 参数 |
|--------|------|------|
| `edgex_create_point` | 创建设备采集点位 | `channel_id`, `device_id`, `name`, `address`, `datatype` |
| `edgex_delete_point` | 删除指定点位 | `channel_id`, `device_id`, `point_id` |
| `edgex_update_point` | 更新点位配置 | `channel_id`, `device_id`, `point_id` |
| `edgex_read_point_batch` | 批量读取多个点位实时值 | `channel_id`, `device_id`, `point_ids[]` |
| `edgex_write_point_batch` | 批量写入多个点位值 | `channel_id`, `device_id`, `writes[]` |

**边缘规则（3 个）**

| 工具名 | 功能 | 参数 |
|--------|------|------|
| `edgex_create_edge_rule` | 创建边缘计算规则 | `name`, `type`, `condition`, `actions[]`, `sources[]` |
| `edgex_delete_edge_rule` | 删除边缘计算规则 | `rule_id` |
| `edgex_list_edge_rules` | 列出所有边缘规则 | 无 |

**虚拟设备（2 个）**

| 工具名 | 功能 | 参数 |
|--------|------|------|
| `edgex_create_virtual_device` | 创建虚拟设备（公式计算） | `virtual_device_id`, `channel_id`, `formula_points` |
| `edgex_delete_virtual_device` | 删除虚拟设备 | `virtual_device_id` |

**扩展工具（6 个）**

| 工具名 | 功能 | 参数 |
|--------|------|------|
| `edgex_restart_channel` | 重启通道采集引擎 | `channel_id` |
| `edgex_get_channel_config` | 获取通道完整配置 | `channel_id` |
| `edgex_get_point_history` | 获取点位历史数据 | `channel_id`, `device_id`, `point_id` |
| `edgex_export_config` | 导出完整配置（json/yaml） | 无 |

### 17.5 MCP 资源

| URI | 名称 | 说明 |
|-----|------|------|
| `edgex://channels` | 通道列表 | 所有采集通道的完整配置信息（JSON） |
| `edgex://system` | 系统信息 | 网关系统状态信息（CPU/内存/运行时间/协议支持） |
| `edgex://diagnostics` | 诊断快照 | 所有通道和设备的诊断信息汇总 |
| `edgex://protocols` | 协议支持列表 | 12 种工业协议完整列表（含端口、传输层、特性） |
| `edgex://edge-rules` | 边缘规则 | 所有边缘计算规则配置和状态 |
| `edgex://config` | 完整配置 | EdgeX 完整配置导出（通道/设备/点位/规则） |

### 17.6 MCP 提示词模板

| 名称 | 描述 | 参数 |
|------|------|------|
| `protocol-reverse` | 工业协议逆向工程：PCAP + HMI 值分析协议结构 | `protocol`*, `observations` |
| `channel-config` | 生成通道配置 JSON | `protocol`*, `ip`*, `port` |
| `diagnostics-analyze` | 诊断分析：分析通道/设备异常并给排查建议 | `channel_id`* |
| `modbus-quick-start` | Modbus TCP/RTU 快速接入指南 | `ip`*, `port`, `slave_id` |
| `s7-quick-start` | Siemens S7 PLC 快速接入指南 | `ip`*, `rack`, `slot` |
| `bacnet-quick-start` | BACnet/IP 楼宇自控接入指南 | `device_id` |
| `opcua-quick-start` | OPC UA 接入指南（安全策略/节点浏览/订阅） | `endpoint`*, `security` |
| `point-batch-generator` | 点位批量生成模板 | `protocol`*, `start_address`*, `count`*, `datatype` |
| `edge-rule-builder` | 边缘计算规则构建助手 | `rule_type`*, `channel_id` |
| `troubleshooting-guide` | 工业协议故障排查流程 | `issue_type`* |
| `data-flow-architect` | 数据流架构设计（采集到云端端到端链路） | `target` |
| `gateway-health-check` | 网关健康检查清单 | 无 |
| `protocol-migration` | 协议迁移指南（配置转换/地址映射/数据类型对应） | `from`*, `to`* |

### 17.7 接入方式

外部 LLM 应用通过 MCP 配置文件接入：

**Claude Desktop / Cursor / Windsurf**：
```json
{
  "mcpServers": {
    "edgex": {
      "url": "http://<gateway>:8080/api/mcp",
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
    "edgex": {
      "transport": {
        "type": "http",
        "url": "http://<gateway>:8080/api/mcp"
      },
      "auth": {
        "type": "bearer",
        "token": "<mcp_api_key>"
      }
    }
  }
}
```

### 17.8 API 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/mcp` | MCP Server 信息（工具数、资源数、激活状态） |
| POST | `/api/mcp` | JSON-RPC 2.0 请求入口 |
| GET | `/api/mcp` (SSE) | MCP Streamable HTTP SSE 流 |
| DELETE | `/api/mcp` | 终止 MCP 会话（按 `Mcp-Session-Id`） |
| GET | `/api/mcp/help` | MCP 接入帮助文档（含工具列表、客户端配置示例） |
| POST | `/api/mcp/activate` | 激活/关闭全功能读写（需用户确认） |
| GET | `/api/mcp/status` | 查询 MCP 激活状态 |
| GET | `/api/mcp/key` | 获取 MCP API Key 明文（仅 JWT 用户） |
| POST | `/api/mcp/generate-key` | 生成 256 位随机 API Key（64 字符 hex） |

### 17.9 代码路径

| 模块 | 路径 | 说明 |
|------|------|------|
| MCP 协议 | `internal/mcp/protocol.go` | JSON-RPC 2.0 类型定义 |
| MCP 服务端 | `internal/mcp/server.go` | MCP Server 引擎（工具/资源/提示词注册） |
| MCP 工具实现 | `internal/server/mcp_handler.go` | 33 个 MCP 工具的 Handler 实现 |
| MCP 配置模型 | `internal/model/ai_copilot.go` | `McpEnabled`, `McpApiKey`, `McpFullAccess` |
| MCP 激活管理 | `internal/server/ai_settings_handler.go` | `handleMcpActivate`, `handleMcpStatus` |
| MCP 前端面板 | `ui/src/components/ai-assistant/AiMcpHelp.vue` | MCP 接入帮助页面 |
| MCP 设置入口 | `ui/src/components/ai-assistant/AiSettingsDialog.vue` | AI 设置对话框 MCP 配置区域 |
| MCP 样式 | `ui/src/styles/ai-assistant.css` | MCP 相关 CSS 样式 |

### 17.10 与 AI Agent 协同

MCP 和 AI Agent 是 EdgeX 对外 AI 协同的**双通道**：

```text
┌──────────────────────────────────────────────────────────────────┐
│  EdgeX 工业网关                                                    │
│                                                                    │
│  ┌─────────────────────┐    ┌─────────────────────┐              │
│  │  AI Agent（内部）    │    │  MCP Server（对外）   │              │
│  │  §7 协议逆向引擎     │    │  §17 MCP 工具        │              │
│  │  §8 协议知识库       │    │  33 个工具/6 资源    │              │
│  │  Scenario A/B       │    │  API Key 认证        │              │
│  └────────┬────────────┘    └────────┬────────────┘              │
│           │                          │                             │
│           ▼                          ▼                             │
│  ┌─────────────────────────────────────────────────────────────┐  │
│  │  ChannelManager · EdgeComputeManager · VirtualShadowEngine  │  │
│  │  config.db · runtime.db                                     │  │
│  └─────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────┘
```

- **AI Agent** 负责协议逆向、文档解析、配置生成等**智能分析**能力
- **MCP Server** 负责通道/设备/点位/规则/虚拟设备的**远程操作**能力
- 两者共享底层 ChannelManager、EdgeComputeManager、VirtualShadowEngine
- MCP 全功能激活状态和 API Key 存储在 `config.db → ai_copilot` bucket

---

*维护：架构组 · 产品组 | 下次审查：MVP 协议逆向引擎 + 30min 生产配置生成 + Mode A gRPC + MCP 全功能 首 PR 合并时*