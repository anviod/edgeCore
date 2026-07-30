# EAN 2.0 EdgeX-EdgeOS 联合复验报告

| 属性 | 值 |
|---|---|
| 日期 | 2026-07-30（v2 更新） |
| EdgeX 版本 | v2.7 (Phase 3 EdgeX 侧全部实现 + 工具合并优化 + 地址语义统一修复) |
| EdgeOS 版本 | v2.7 (Phase 1/2 完成 + 启动韧性) |
| 共识文档 | V1-to-EAN-Migration-Assessment.md v2.13 |
| 验证环境 | 本地 Windows, EdgeX :8080, EdgeOS :8000, MQTT broker :18083, ARM64 硬件 192.168.3.230 |

---

## 1. 验证结果总览

| 验证项 | 结果 | 说明 |
|---|---|---|
| EdgeX MCP Runtime 状态 | **通过** | 7 个统一 ean_* 工具已注册（ean_read_points / ean_write_points / ean_scan_devices / ean_list_points / ean_get_diagnostics / ean_ai_protocol_reverse / ean_ai_doc_parse），status=online, version=2.0.0。注：MCP Runtime 侧已做工具统一，不再暴露 63 条协议特定工具；63 Cap 仍由北向 EAN Runtime 生成并发布至 EdgeOS |
| EdgeX Invoke metrics | **通过** | total=1, success=1, success_rate=100% (MCP Runtime) |
| EdgeX `system.diagnostics` Invoke | **通过** | 返回 2 channels (BACnet + ModbusTCP) |
| EdgeX 北向通道 EAN 启用 | **通过** | `ean_enabled=true`, `ean_heartbeat_sec=60` |
| EAN Runtime Discovery 发布 | **通过** | 日志确认 63 Cap 已发布到 `$edgeos/discovery/*` |
| EdgeOS Agent 发现 | **通过** | 1 Agent online, id=edgex-node-001 |
| EdgeOS 原生 Cap 索引 | **通过** | 63 条 EAN 2.0 原生 Cap (modbus_tcp.* / bacnet_ip.* / system.* / ai.*) |
| V1 Bridge Cap 并存 | **符合预期** | 过渡期 V1 合成 Cap (edgex-node-001/*/read-write) 与原生 Cap 共存 |
| 跨系统 Invoke (EdgeOS→EdgeX) | **通过** | `system.diagnostics` 返回 status=completed, 2 channels |
| EdgeOS Health | **通过** | status=ok, online_agents=1, transports=mqtt |
| **MCP 工具合并优化** | **通过** | 94 → 32 工具（15 协议×4 操作矩阵合并为 4 通用工具 + 系统工具），ToolSearch 索引抖动消除 |
| **地址语义统一修复** | **通过** | `resolvePointIDs()` 实现 point_id/address/name 三形式自动解析，list_points 输出可直接作为 read_points 输入 |
| **BACnet 设备扫描硬指标** | **通过** | 2228316/2228317/2228318/2228319 四设备全部在线，quality=Good |
| **ARM64 硬件回归验证** | **通过** | goreleaser arm64 deb 包部署到 192.168.3.230，全部测试通过 |

---

## 2. 关键发现

### 2.1 双 Runtime 架构验证

EdgeX 双 Runtime 架构按设计工作：

- **MCP Runtime** (`transport: "sdk"`)：Server 启动即就绪，通过 `GET /api/capability/agent/status` 暴露状态和 metrics。独立于北向通道，零外部依赖。
- **北向 EAN Runtime** (`transport: "mqtt"`)：依赖 EdgeOS(MQTT) 通道连接，启用后通过 `$edgeos/discovery/*` 发布 63 条原生 Capability。跨系统 Invoke 通过 MQTT 消息到达此 Runtime。

两个 Runtime 实例各自独立，各自维护 Invoke metrics。`GET /api/capability/agent/status` 返回的是 MCP Runtime 的 metrics；北向 EAN Runtime 的 metrics 通过运行时日志和 EdgeOS 侧 Health 间接观测。

### 2.2 配置热更新验证

通过 `POST /api/northbound/edgeos-mqtt` 将 `ean_enabled` 从 `false` 改为 `true`：
- 北向通道未中断（broker 连接保持）
- EAN Runtime 自动启动并发布 Discovery
- EdgeOS 侧收到 63 条原生 Cap

### 2.3 V1 Bridge 过渡期行为

EdgeOS 同时索引了两类 Cap：
- **原生 EAN 2.0 Cap**（63 条）：`modbus_tcp.read_holding_register`, `bacnet_ip.list_points`, `system.diagnostics`, `ai.protocol_reverse` 等
- **V1 Bridge 合成 Cap**（13 条）：`edgex-node-001/{device}/read-write` 格式

这符合共识文档 §2.3 的隔离机制设计：原生 Cap 到达后 purge V1 的逻辑由 EdgeOS 侧 OS-P3-01 任务负责（尚未完成）。

### 2.4 Agent 版本差异

EdgeOS 显示 Agent `version: "1.0.0"`（V1 Bridge 注册），而非 EAN 2.0 的 `"2.0.0"`。原因：V1 Bridge 的 `publishNodeOnline` 先于 EAN Discovery 注册了 Agent 描述符，EdgeOS 的 `AgentSource` 隔离机制保留了先到达的 V1 Agent 信息。EAN 2.0 原生 Agent 描述符（version=2.0.0）通过 `$edgeos/discovery/agent` 发布，但 EdgeOS 在过渡期不覆盖已存在的 V1 Agent。

---

## 2.5 MCP 工具合并优化与地址语义统一修复（2026-07-30 新增）

### 2.5.1 工具合并优化

**背景**：MCP 工具数量超过 80 个时影响 AI 响应质量，ToolSearch 索引抖动和最终一致性延迟加剧。

**实施**：将 15 协议 × 4 操作矩阵（60 个协议特定工具）合并为 4 个通用工具：
- `read_points` — 统一读取（替代 read_holding_register、edgex_read_point、read_point_batch 等）
- `write_points` — 统一写入（替代 write_register、edgex_write_point、write_point_batch 等）
- `scan_devices` — 统一扫描（跨协议）
- `list_points` — 统一点位列表

**结果**：MCP 工具从 94 降至 32（62% 压缩），ToolSearch 索引抖动消除。协议自动路由通过 `DriverExecutor.resolveDeviceWithProtocol()` 从 channel 元数据推断。

### 2.5.2 地址语义统一修复

**问题**：`ean_list_points` 返回 PDU 偏移作为 `address`（如 Modbus `"0"`、`"2"`），但 `ean_read_points` 期望内部 point_id（如 `pt_0723121000`）。用户无法将 list 输出直接作为 read 输入。

**修复**：在 `DriverExecutor.resolvePointIDs()` 中实现三形式自动解析：

| 输入形式 | 示例 | 解析目标 |
|---|---|---|
| point_id（推荐） | `hr_0`, `av_1`, `pt_0723121000` | 直接匹配 |
| address | `0`（PDU 偏移）, `40001`（PLC 地址）, `AnalogValue:1`（BACnet） | 通过 `byAddr` 映射 |
| name | `temperature`, `HR 5` | 通过 `byName` 映射（不区分大小写） |

### 2.5.3 ARM64 硬件验证结果

验证环境：SSH root@192.168.3.230，goreleaser arm64 deb 包部署。

| 测试场景 | 输入 | 解析结果 | 值 | 质量 |
|---|---|---|---|---|
| Modbus PDU 偏移 | `address:"0"` | `point_id:"hr_0"` | 1 | Good |
| Modbus PDU 偏移 | `address:"2"` | `point_id:"hr_2"` | 27 | Good |
| Modbus point_id | `point_id:"hr_0"` | `point_id:"hr_0"` | 1 | Good |
| Modbus 名称 | `address:"HR 5"` | `point_id:"hr_5"` | -13758 | Good |
| BACnet 地址 | `address:"AnalogValue:1"` | `point_id:"av_1"` | 21 | Good |
| BACnet 名称 | `address:"analog_value_1"` | `point_id:"av_1"` | 21 | Good |
| 混合批量读 | `["hr_0","2","HR 5"]` | 3 点全部正确解析 | 1/27/-13758 | Good |

此前 `ean_read_points` 用 `address:"0"` 报 point not found (E001) 的 bug 已彻底消除。

### 2.5.4 BACnet 设备扫描硬指标

四台 BACnet 设备（2228316/2228317/2228318/2228319）全部在线，采集成功：

| 设备 | IP | 端口 | 点位数 | 质量 |
|---|---|---|---|---|
| 2228316 | 192.168.3.104 | 53772 | 1 (analog_value_1) | Good |
| 2228317 | 192.168.3.104 | 56972 | 1 (analog_value_1) | Good |
| 2228318 | 192.168.3.104 | 56973 | 1 (analog_value_1) | Good |
| bacnet-2228319 | 192.168.3.104 | 50457 | 11 (AI/AV/BV/MSV) | Good |

---

## 3. 未完成项（Phase 3 残留）

### EdgeX 侧
- 无未完成项。EX-P3-01 至 EX-P3-09 全部实现，go build + 前端构建通过。
- 工具合并优化（94→32）和地址语义统一修复已完成并通过 ARM64 硬件验证。

### EdgeOS 侧
| 任务 | 状态 | 说明 |
|---|---|---|
| OS-P3-01 | 已完成 | V1 Fallback 移除（v1_bridge_caps=0），原生 Cap 到达时 purge V1 合成 Cap |
| OS-P3-02 | 已完成 | 前端写操作切 EAN Invoke（ControlView/PointListView） |
| OS-P3-03 | 未做 | EAN Invoke 监控面板 |
| OS-13 | 未做 | Invoke status 进度订阅 |
| Agent version | 已解决 | OS-P3-01 完成后 Agent version 更新为 2.0.0 |

---

## 4. 建议下一步

1. **EdgeOS 侧执行 OS-P3-01**：移除 V1 Fallback 机制，原生 Cap 到达时 purge 全部 V1 合成 Cap。这将使 EdgeOS 仅索引 63 条原生 EAN Cap，Agent version 更新为 2.0.0。
2. **EdgeOS 侧执行 OS-P3-02**：前端写操作从 V1 命令 API 切换到 `/api/ean/invoke`。
3. **NATS 传输联调**：当前仅验证 MQTT 传输，NATS 路径需对称联调。
4. **EAN Runtime metrics 暴露**：考虑在北向通道状态卡片中展示 EAN Runtime 的 Invoke metrics（需将北向 Runtime metrics 暴露到 API）。
