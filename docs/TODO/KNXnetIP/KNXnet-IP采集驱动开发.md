# KNXnet/IP 采集驱动开发方案

## 1. 概述

### 1.1 协议简介

KNXnet/IP 是基于标准 IP 网络（以太网、Wi-Fi）的 KNX 隧道协议，用于楼宇与家居自动化。edgeCore 本驱动作为 **KNXnet/IP 隧道客户端**，通过 KNX IP 接口（网关）访问总线上的组地址（Group Address, GA）。

典型部署：edgeCore 网关与 KNX IP 接口在同一局域网，使用 **隧道模式（Tunneling）** 建立连接，发送 `GroupValueRead` / `GroupValueWrite` 采集或反控点位。

### 1.2 功能定位

| 功能类别 | 功能描述 | 状态 |
| :--- | :--- | :--- |
| 数据采集 | 读取组地址当前值（GroupValueRead） | ✅ 已实现 |
| 数据写入 | 写入组地址（GroupValueWrite） | ✅ 已实现 |
| 位/子字节 | 支持 DPT 子字节类型（地址带 BIT 参数） | ✅ 已实现 |
| 组播发现 | SEARCH_REQUEST 自动发现网关 | ✅ 已实现 |
| 路由模式 | KNXnet/IP Routing 连接 | ⏳ 后续 |

### 1.3 设计原则

- **一致性**：遵循项目统一 `driver.Driver` 接口，模块划分对齐 Omron FINS、DL/T 645 驱动
- **可靠性**：连接重试、隧道保活（ConnectionState）、组地址值缓存
- **可测试性**：内置 UDP 协议模拟器，单元测试不依赖真实 KNX 硬件

---

## 2. 技术架构

### 2.1 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                      edgeCore Gateway                              │
├─────────────────────────────────────────────────────────────────┤
│                    Device Service Layer                         │
│   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐           │
│   │  DriverMgr  │  │  Schedule   │  │  ConfigMgr  │           │
│   └──────┬──────┘  └─────────────┘  └─────────────┘           │
├──────────┼──────────────────────────────────────────────────────┤
│                    Protocol Driver Layer                        │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │                  KNXnetIPDriver                         │   │
│   │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────┐  │   │
│   │  │ transport│◄─│ scheduler│◄─│ decoder  │  │ config │  │   │
│   │  │ UDP/TCP  │  │ 读写调度  │  │ 编解码   │  │        │  │   │
│   │  └────┬─────┘  └──────────┘  └──────────┘  └────────┘  │   │
│   └───────┼─────────────────────────────────────────────────┘   │
├───────────┼──────────────────────────────────────────────────────┤
│                    Network Layer                                │
│              UDP/TCP 端口 3671（KNXnet/IP）                      │
└───────────┴──────────────────────────────────────────────────────┘
                              │
                              ▼
                    KNX IP 接口 / 网关
                              │
                              ▼
                         KNX 总线设备
```

### 2.2 模块划分

| 模块 | 文件 | 职责 |
| :--- | :--- | :--- |
| 驱动主模块 | `knxnetip.go` | 实现 Driver 接口，注册协议 `knxnet-ip` |
| 传输层 | `transport.go` | 隧道连接、CONNECT/TUNNELING 帧收发、TCP/UDP、保活、缓存 |
| 发现 | `discovery.go` | SEARCH_REQUEST / SEARCH_RESPONSE 网关自动发现 |
| 调度器 | `scheduler.go` | 批量点位读写 |
| 解码器 | `decoder.go` | 组地址解析、DPT 值编解码 |
| 协议 | `protocol.go` | KNXnet/IP 与 cEMI 常量、帧构建 |
| 地址 | `address.go` | 组地址/个体地址字符串解析 |
| 模拟器 | `simulator.go` | 单元测试用 UDP 网关模拟 |

---

## 3. 配置参数

### 3.1 通道（设备）配置

| 参数名 | 类型 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| `ip` | string | — | KNX IP 网关地址（必填；直连网关 IP，非组播） |
| `gatewayIP` | string | — | `ip` 的别名 |
| `port` | int | 3671 | KNXnet/IP 端口 |
| `mode` | string | UDP | 传输模式：`UDP` 或 `TCP` |
| `timeout` | int | 3000 | 通信超时（毫秒） |
| `max_retries` | int | 3 | 连接重试次数 |
| `heartbeat_interval` | int | 60000 | ConnectionState 保活间隔（毫秒），0 禁用 |
| `local_ip` | string | — | 本地绑定 IP（可选） |
| `discovery` | bool | false | 启用 SEARCH 自动发现；`ip` 为空时选用首个响应网关 |
| `discovery_timeout` | int | 3000 | 发现超时（毫秒） |
| `discovery_multicast` | string | 224.0.23.12:3671 | SEARCH 组播地址（可改为单播以测试） |

> **说明**：标准组播地址 `224.0.23.12:3671` 用于 SEARCH 发现；直连采集请填写具体网关 IP，或启用 `discovery: true` 自动填充。

### 3.2 点位配置

#### 3.2.1 地址格式

**格式 1** — 组地址 + 可选个体地址：

```
main/middle/sub[,area.line.device]
```

**格式 2** — 组地址 + 个体地址 + 子字节位数（DPT B1/B2 等）：

```
main/middle/sub,area.line.device,BIT
```

| 示例 | 说明 |
| :--- | :--- |
| `1/2/3` | 三级组地址 1/2/3 |
| `1/34` | 二级组地址 1/34 |
| `0/0/1,1.1.1` | 组地址 0/0/1，个体地址 1.1.1 |
| `0/0/1,1.1.1,2` | 组地址 0/0/1，读取 2 比特子字段 |

#### 3.2.2 支持的数据类型

| 数据类型 | 说明 | KNX DPT 对应 |
| :--- | :--- | :--- |
| `BIT` / `BOOL` | 布尔 | DPT 1 |
| `INT8` / `UINT8` | 8 位整数 | DPT 5 等 |
| `INT16` / `UINT16` | 16 位整数 | DPT 7/8/9 等 |
| `INT32` / `UINT32` | 32 位整数 | DPT 12/13 等 |
| `FLOAT` | 浮点（4 字节 IEEE 或 2 字节 DPT 9） | DPT 9/14 |

---

## 4. 读写语义

### 4.1 读操作

1. 调度器解析点位组地址
2. 传输层发送 `TUNNELING_REQUEST`，cEMI 内嵌 `L_Data.req` + **GroupValueRead**
3. 等待 `TUNNELING_INDICATION`，解析 **GroupValueResponse** 载荷
4. 解码器按数据类型转换为 `model.Value`
5. 若读超时，尝试返回该组地址最近一次缓存值

### 4.2 写操作

1. 解码器将值编码为 KNX 数据字节
2. 发送 `L_Data.req` + **GroupValueWrite**
3. 等待隧道确认

### 4.3 连接流程

```
edgeCore                         KNX IP 接口
  |--- CONNECT_REQUEST -------->|
  |<-- CONNECT_RESPONSE --------|  分配 Channel ID
  |--- TUNNELING_REQUEST ------>|  GroupValueRead/Write
  |<-- TUNNELING_INDICATION ----|  GroupValueResp
  |--- TUNNELING_CONFIRM ------->|
  |--- CONNECTIONSTATE_REQUEST ->|  保活（可选）
```

---

## 5. 错误处理

| 错误类型 | 触发条件 | 处理策略 |
| :--- | :--- | :--- |
| 连接错误 | 网关不可达、CONNECT 被拒绝 | 指数退避重连（ConnectionManager） |
| 超时 | GroupValueRead 无响应 | 标记点位 Bad；尝试缓存值 |
| 地址错误 | 组地址格式非法 | 跳过请求，点位 Bad |
| 协议错误 | 非预期 Service Type / APCI | 记录日志，继续等待或失败 |

---

## 6. 部署与集成

### 6.1 驱动注册

协议名：`knxnet-ip`

```go
func init() {
    driver.RegisterDriver("knxnet-ip", func() driver.Driver {
        return NewKNXnetIPDriver()
    })
}
```

`cmd/main.go` 中 blank import：

```go
_ "github.com/anviod/edgeCore/internal/driver/knxnetip"
```

### 6.2 配置示例

```yaml
protocol: knxnet-ip
config:
  ip: "192.168.1.50"
  port: 3671
  mode: UDP
  timeout: 3000
  max_retries: 3
  heartbeat_interval: 60000
```

自动发现示例（无需预先填写 `ip`）：

```yaml
protocol: knxnet-ip
config:
  discovery: true
  discovery_timeout: 5000
  mode: UDP
  timeout: 3000
```

TCP 隧道示例：

```yaml
protocol: knxnet-ip
config:
  ip: "192.168.1.50"
  port: 3671
  mode: TCP
  timeout: 3000
  heartbeat_interval: 60000
```

### 6.3 点位示例

```yaml
points:
  - name: "living-light"
    address: "1/2/3"
    datatype: "BOOL"
    readwrite: "R"

  - name: "room-temp"
    address: "2/1/10"
    datatype: "FLOAT"
    readwrite: "R"

  - name: "dimmer"
    address: "3/0/5"
    datatype: "UINT8"
    readwrite: "RW"

  - name: "status-bits"
    address: "0/0/1,1.1.1,2"
    datatype: "UINT8"
    readwrite: "R"
```

---

## 7. 开发计划

### Phase 1 — 核心采集（✅ 已完成）

- [x] 协议常量与帧编解码（CONNECT / TUNNELING / cEMI）
- [x] UDP 隧道连接与重连
- [x] GroupValueRead / GroupValueWrite
- [x] 组地址解析（2/3 级）与子字节 BIT
- [x] 驱动注册与 Driver 接口实现
- [x] UDP 模拟器与单元测试

### Phase 2 — 增强（✅ 已完成）

- [x] TCP 隧道模式完善（统一重连、帧重组、ConnectionState 保活）
- [x] SEARCH_REQUEST 网关自动发现（224.0.23.12:3671，可配置）
- [x] UI 通道帮助组件 `KnxHelp.vue`（参考 FinsHelp.vue）
- [ ] 异步 TUNNELING_INDICATION 订阅（COV 式更新）— 后续
- [ ] 用户手册页 `docs/drivers/KNXnetIP.md` — 后续

### Phase 3 — 生产验证（部分完成）

- [x] 模拟器 TCP 模式与并发读压测（单元测试，无真实硬件）
- [ ] 真实 KNX IP 接口（如 Weinzierl BAOS）联调
- [ ] 多隧道通道并发压测（真实网关）
- [ ] 南向驱动测试报告条目

---

## 8. 测试方案

### 8.1 单元测试（模拟器）

`internal/driver/knxnetip/simulator.go` 提供 UDP/TCP 网关模拟与 SEARCH 响应：

```bash
go test ./internal/driver/knxnetip/... -v
```

覆盖：地址解析、值解码、Connect、ReadPoints、WritePoint、TCP 隧道、SEARCH 发现、并发读压测。当前 **10/10** 测试通过。

### 8.2 TCP 与发现说明

| 模式 | 配置 | 行为 |
| :--- | :--- | :--- |
| UDP 隧道 | `mode: UDP`（默认） | 本地 UDP 端口与网关 3671 通信 |
| TCP 隧道 | `mode: TCP` | 长连接 TCP，HPAI 使用 IPv4/TCP，支持帧重组与保活 |
| 自动发现 | `discovery: true` | Init 时发送 SEARCH，选用首个 SEARCH_RESPONSE 中的 Control Endpoint |

发现流程：绑定本地 UDP → 向 `discovery_multicast` 发送 SEARCH_REQUEST → 收集 SEARCH_RESPONSE 直至 `discovery_timeout` → 取 Control HPAI 的 IP/端口作为网关。

### 8.3 真实设备验证清单

| 测试项 | 方法 | 预期 |
| :--- | :--- | :--- |
| 连接 | 配置网关 IP，查看 Health | Good |
| 布尔读 | 组地址 DPT 1 | 正确 true/false |
| 数值读 | DPT 5/9/14 | 与 ETS 监视器一致 |
| 反控写 | GroupValueWrite BOOL | 总线设备响应 |

---

## 9. 代码结构

```
internal/driver/knxnetip/
├── knxnetip.go        # 驱动入口
├── transport.go       # KNXnet/IP 隧道传输（UDP/TCP）
├── discovery.go       # SEARCH 网关发现
├── scheduler.go       # 点位读写调度
├── decoder.go         # 值编解码
├── address.go         # 地址解析
├── protocol.go        # 协议帧
├── config.go          # 配置解析
├── simulator.go       # 测试模拟器（UDP/TCP/SEARCH）
└── knxnetip_test.go   # 单元测试
```

UI 帮助：`ui/src/components/channel-help/KnxHelp.vue`，已在 `ChannelProtocolHelpDrawer.vue` 注册协议键 `knxnet-ip`。

---

## 附录：主要 KNXnet/IP 服务类型

| 服务 | 代码 | 说明 |
| :--- | :--- | :--- |
| CONNECT_REQUEST | 0x0205 | 建立隧道 |
| CONNECT_RESPONSE | 0x0206 | 隧道响应 |
| CONNECTIONSTATE_REQUEST | 0x0207 | 保活 |
| DISCONNECT_REQUEST | 0x0209 | 断开 |
| TUNNELING_REQUEST | 0x0420 | 隧道数据请求 |
| TUNNELING_CONFIRM | 0x0421 | 隧道确认 |
| TUNNELING_INDICATION | 0x0422 | 隧道数据指示 |

| APCI | 值 | 说明 |
| :--- | :--- | :--- |
| GroupValueRead | 0x00 | 读组值 |
| GroupValueResponse | 0x01 | 读响应 |
| GroupValueWrite | 0x02 | 写组值 |
