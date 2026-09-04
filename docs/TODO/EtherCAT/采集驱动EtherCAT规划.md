# EtherCAT 采集驱动实现规划

EtherCAT（Ethernet for Control Automation Technology）是一种基于标准以太网物理层的实时工业现场总线协议，由 Beckhoff 提出并由 EtherCAT Technology Group（ETG）维护。EtherCAT 采用「飞读（Processing on the fly）」机制：主站发出的以太网帧依次穿过各从站，从站在帧经过时完成数据读写，从而实现微秒级周期通信。

> **状态**：**v0.0.8 已实现 M1 里程碑**。核心驱动框架（`internal/driver/ethercat/`）、协议注册、前端通道/设备表单、帮助组件已交付。模拟模式可无硬件验证全流程。后续 M2/M3 里程碑（实时网卡绑定、DC 分布式时钟、CoE 完整对象字典）待规划。

EtherCAT 系统包括几个角色：**主站（Master）** 负责网络扫描、从站配置与过程数据调度；**从站（Slave）** 为现场 IO 模块、伺服驱动器、传感器等，以菊花链或树形拓扑挂接；**配置工具**（如 TwinCAT、SOEM slaveinfo、IgH 工具链）用于导入 ESI/XML、生成 PDO 映射与对象字典。edgeCore 本驱动规划为 **EtherCAT 主站侧采集驱动**，通过过程数据对象（PDO）周期性采集、通过服务数据对象（SDO）非周期性读写参数。

> **架构依据**：本文档以 [ScanEngine 重构方案](../ScanEngine重构方案.html)（v5.0 调度驱动内核规范）与 [边缘网关架构设计总览](../../edge/边缘网关架构设计总览.html)（v2.2 全生命周期架构）为准绳。EtherCAT 驱动须遵循 **调度→执行→数据→状态** 闭环，Driver 作为纯执行函数接入 ScanEngine 统一调度。

::: tip 部署约束

EtherCAT 主站需直接访问以太网 MAC 层（原始帧或专用内核模块），对帧时序与网卡驱动有严格要求。与 [Profinet IO 驱动](../../drivers/PLC_Profinet_IO.html) 类似，**须将 edgeCore 部署在物理工业网关上并绑定真实网卡**；不建议在普通 Docker 容器或无专用网卡驱动的虚拟机中运行主站功能。具体网卡白名单与实时性指标**待联调确认**。

:::

---

## 添加通道

在 **配置 → 南向通道**，点击添加通道，协议类型选择 **EtherCAT**（前端选项**待实现**）。

通道级配置对应整网 EtherCAT 主站实例：一个通道绑定一块网卡，管理该网段上所有从站设备。

## 设备配置

点击通道进入设备列表，添加从站设备。配置 edgeCore 与从站建立映射所需的参数，下表为**规划中的**设备相关配置项。

| 参数 | 说明 |
|------|------|
| 设备名称 | 从站别名，便于 UI 展示 |
| 从站位置 | 网络拓扑中的物理顺序（1…N），对应 SOEM/IgH 的 `slave position` |
| 从站别名 | 可选，配置别名寻址时使用 |
| 厂商 ID | 从站 `Vendor ID`（十六进制，如 `0x00000002`） |
| 产品代码 | 从站 `Product Code` |
| 修订号 | 从站 `Revision`，可选，用于精确匹配 ESI |
| ESI 标识 | 关联从站描述文件（ESI/XML）名称或哈希，**待确认**是否首版支持 |
| 输入 PDO 长度 | 从站 TxPDO 映射后的输入字节数（主站视角「读」） |
| 输出 PDO 长度 | 从站 RxPDO 映射后的输出字节数（主站视角「写」） |
| 运行模式 | `pdo`（过程数据，默认）或 `sdo`（仅对象字典访问，用于参数类点位） |

> **说明**：与 Profinet IO 按 IP 寻址不同，EtherCAT 从站通常无独立 IP，设备配置以**拓扑位置 + 身份标识**为主。若现场通过 EtherCAT 转以太网网关接入，是否支持「网关模式」**待确认**（首版建议仅支持原生主站拓扑）。

## 设置组和点位

完成通道和设备的添加与配置后，进入设备点位页添加采集点位。点位地址指向从站 PDO 映像区或 SDO 对象字典条目。

### 数据类型

规划支持的数据类型（与 Profinet IO、EtherNet/IP 等驱动对齐）：

- INT8 / UINT8
- INT16 / UINT16
- INT32 / UINT32
- INT64 / UINT64
- FLOAT / DOUBLE
- BIT / BOOL

### 地址格式

#### PDO 过程数据（默认，适合 ScanEngine 周期读）

```
POSITION:PDO:OFFSET[.BIT][#ENDIAN]
```

| 字段 | 说明 |
|------|------|
| `POSITION` | 必填，从站位置（与设备配置「从站位置」一致） |
| `PDO` | 必填，PDO 索引：`Tx`（主站读输入）或 `Rx`（主站写输出），也接受 `0`/`1` 表示 Tx/Rx 映像区编号，**具体枚举待实现时与主站库对齐** |
| `OFFSET` | 必填，PDO 映像区内字节偏移（从 0 开始） |
| `.BIT` | 可选，位偏移 0–7 |
| `#ENDIAN` | 可选，`BE`（默认）或 `LE` |

#### SDO 非过程数据（适合参数、诊断、非周期读）

```
POSITION:SDO:0xINDEX:0xSUBINDEX[#ENDIAN]
```

| 字段 | 说明 |
|------|------|
| `POSITION` | 必填，从站位置 |
| `SDO` | 固定关键字，表示走 CoE SDO 访问 |
| `0xINDEX` / `0xSUBINDEX` | 必填，对象字典索引与子索引（十六进制） |

### 地址示例

| 地址 | 数据类型 | 说明 |
|------|----------|------|
| `1:Tx:0` | int16 | 1 号从站 TxPDO 映像第 0、1 字节 |
| `1:Tx:2.3` | bit | 1 号从站 TxPDO 第 2 字节第 3 位 |
| `2:Rx:4` | uint32 | 2 号从站 RxPDO 映像第 4–7 字节（反控） |
| `3:Tx:10` | float | 3 号从站 TxPDO 第 10–13 字节 |
| `1:SDO:0x6041:0` | uint16 | 1 号从站 CiA402 状态字 |
| `1:SDO:0x6064:0` | int32 | 1 号从站实际位置值 |

## 数据监控

完成点位配置后，可点击 **监控 → 数据监控** 查看从站过程数据并反控输出 PDO，交互与现有驱动一致。EtherCAT 主站未进入 `OP` 状态时，点位应显示 Bad/Offline，具体文案**待 UI 联调时统一**。

---

## 1. 背景与目标

### 1.1 背景

- 制造与运动控制现场大量采用 EtherCAT 伺服、IO 耦合器、安全模块；网关若缺少原生主站能力，往往需额外 PLC 或协议转换网关，增加成本与延迟。
- edgeCore 已在 Go 侧交付 Modbus、S7、EtherNet/IP、Profinet IO 等工业协议驱动，架构上具备 `ScanEngine → Driver.ReadPoints/WritePoint → ShadowCore` 统一数据面（见 [南向采集 TODO 索引](../index.html) §1）。
- 产品说明已对外列出 EtherCAT，需补齐实现规划与里程碑，避免能力表述与代码长期脱节。

### 1.2 目标

| 目标 | 说明 | 首版范围 |
|------|------|----------|
| 周期采集 | 主站 OP 状态下按 ScanEngine 调度读取 TxPDO | ✅ 规划 |
| 反控写入 | 写入 RxPDO 指定偏移 | ✅ 规划 |
| 参数访问 | CoE SDO 读写（非实时参数） | 🟡 可选（M2） |
| 拓扑发现 | 扫描总线、枚举从站身份 | ✅ 规划（M1） |
| 热插拔 | 从站掉线重扫、位置变化告警 | 🟡 M2 |
| ESI 自动映射 | 导入 XML 自动生成 PDO 点位 | ⏳ 后续 |
| 冗余环网 | 电缆冗余 / 主备主站 | ❌ 不在首版范围 |

### 1.3 设计原则

- **一致性**：遵循 `internal/driver/interface.go` 统一 `Driver` 接口；模块划分对齐 [Profinet IO](../../drivers/PLC_Profinet_IO.html)、[KNXnet/IP](../KNXnetIP/KNXnet-IP采集驱动开发.html) 驱动的 `transport / scheduler / decoder / config` 分层。
- **可靠性**：主站状态机（INIT → PREOP → SAFEOP → OP）失败可观测；链路 Down 时 `ChannelManager` 将同通道设备标记 Offline（现有 `channel_device_state.go` 行为）。
- **可测试性**：提供模拟从站或录制的 PDO 帧回放，单元测试默认 `CGO_ENABLED=0` 可编译部分纯逻辑（地址解析、解码）；主站 IO **预计依赖 CGO**，集成测试单独门禁。
- **实时性边界**：edgeCore 作为网关采集，目标周期 **≥1 ms** 量级、毫秒级抖动可接受；**不**与硬实时运动控制主站竞争同一网口，现场方案需评审。

---

## 2. 架构定位：在 edgeCore V2.0 数据面中的位置

### 2.1 edgeCore 调度驱动架构回顾

edgeCore V2.0 已完成从「组件驱动」到「调度驱动」的迁移。ScanEngine 作为 Mini OS Scheduler，统一掌控时间（10ms Tick）、资源（IO/Conn）、执行（Serial/Parallel/Limited）与状态（优先级/退避/熔断）。所有南向驱动必须以**纯执行函数**身份接入，不得自行管理时间、并发或连接。

```text
config.db → ChannelManager → ScanEngine → ExecutionLayer → EtherCATDriver.ReadPoints
                                    ↓
                              ShadowCore (SoT) → ShadowBridge → DataPipeline
```

EtherCAT 驱动在此闭环中的职责边界：

| 环节 | ScanEngine / 框架职责 | EtherCAT 驱动职责 |
|------|----------------------|-------------------|
| 调度时机 | EDF 出队 + 10ms Tick + Scan Class 分组 | 无（被动等待 `ReadPoints` 调用） |
| 执行路径 | `ProtocolTypeLimited` → Backpressure(2) + Serial 队列 | 实现 `ReadPoints` / `WritePoint` |
| 连接管理 | `ConnectionManager` 统一重连 Owner | `Connect` / `Disconnect` 委托 Transport |
| 数据写入 | `ShadowIngress` → `ShadowCore` COW 快照 | 返回 `map[string]model.Value` |
| 状态反馈 | `finalizeScanCollect` → 设备 Online/Offline | `Health()` 返回主站状态 |
| 故障隔离 | 每设备断路器 + 串行队列 | 返回 error 触发 CB 计数 |
| 背压降速 | AdaptiveThrottle + ProtocolCongestion | 无（被动接受调度间隔调整） |

### 2.2 核心架构挑战：PDO 周期线程与 Driver 约束的协调

ScanEngine 对 Driver 的强约束明确规定：**Driver 内部禁止 ticker、goroutine、retry loop、connection management**。然而 EtherCAT 主站库（SOEM / IgH）通常需要一个持续运行的 PDO 周期线程，以固定间隔调用 `ec_send_processdata` / `ec_receive_processdata` 来维持总线通信。

这一矛盾是本驱动设计的核心决策点。解决方案是将 PDO 周期线程**下沉到 Transport 层**，使其成为连接生命周期的一部分，而非 Driver 调度逻辑的一部分：

```text
┌─────────────────────────────────────────────────────────────────┐
│                    EtherCATDriver（纯执行函数）                    │
│  ReadPoints()  →  从 PDO 快照内存读取（无 IO 等待）                │
│  WritePoint()  →  写入 RxPDO 待发缓冲（下一周期下发）              │
│  Health()      →  查询主站状态机 + 周期线程存活                    │
└──────────────────────────┬──────────────────────────────────────┘
                           │ 委托
┌──────────────────────────▼──────────────────────────────────────┐
│                  EtherCATTransport（传输层）                      │
│  Connect()    →  初始化主站 → 状态机推进至 OP → 启动 PDO 周期线程  │
│  Disconnect() →  停止周期线程 → 关闭主站                          │
│  ┌─────────────────────────────────────────────────────────┐     │
│  │  PDO 周期线程（goroutine，Transport 持有，非 Driver 持有） │     │
│  │  loop: ec_send_processdata → ec_receive_processdata     │     │
│  │        → 更新 PDO 快照内存（atomic.Pointer / mutex）     │     │
│  │        → 刷新 RxPDO 待发缓冲                              │     │
│  │  间隔: cycleTimeUs（默认 1ms）                            │     │
│  └─────────────────────────────────────────────────────────┘     │
│  ┌─────────────────────────────────────────────────────────┐     │
│  │  PDO 快照内存（Transport 维护）                           │     │
│  │  txSnapshot map[position][]byte  ← 周期线程写入           │     │
│  │  rxBuffers  map[position][]byte  ← WritePoint 写入       │     │
│  └─────────────────────────────────────────────────────────┘     │
└──────────────────────────────────────────────────────────────────┘
```

**设计要点**：

1. **PDO 周期线程属于 Transport 层**，在 `Connect` 时启动、`Disconnect` 时停止。它不参与调度决策，仅维护总线通信与 PDO 快照。这与 Profinet IO 的 Transport 持有 `rpcClient` 连接、KNX 的 Transport 持有 UDP 监听是同一层级——Transport 管理传输资源，Driver 管理数据语义。
2. **ReadPoints 为零等待内存读取**：从 Transport 维护的 PDO 快照中按偏移切片，不阻塞等待下一个 PDO 周期。这使得 ScanEngine 的 `executeTimeout`（`max(interval×2, 5s)`）几乎不会因 PDO 读取超时。
3. **WritePoint 写入待发缓冲**：修改 RxPDO 映像区内存，由周期线程在下一帧自然下发。写入语义为「最终一致」——不保证立即生效，但在下一个 PDO 周期（≤1ms）内下发。
4. **SDO 操作走独立邮箱通道**：SDO 请求经 CoE 邮箱协议异步完成，超时独立配置（默认 3000ms），不阻塞 PDO 周期线程，也不阻塞 ReadPoints 路径。

### 2.3 整体架构图

```text
┌─────────────────────────────────────────────────────────────────────────┐
│                          edgeCore Gateway                                   │
├─────────────────────────────────────────────────────────────────────────┤
│  Web UI · REST API · WebSocket (/api/ws/values)                         │
├─────────────────────────────────────────────────────────────────────────┤
│  ChannelManager                                                          │
│    registerProtocolToScanEngine("ethercat", ProtocolTypeLimited)        │
│    validateEtherCATPoint(point)                                         │
├───────────────────────────┬─────────────────────────────────────────────┤
│  ScanEngine（内核调度器）   │                                             │
│  EDF · CB · Anti-starvation│  ShadowCore (SoT)                          │
│  Adaptive throttle          │  ShadowIngress → COW 快照                  │
│  10ms Tick                  │  ShadowBridge → DataPipeline               │
├───────────────────────────┘                        └────────────────────┤
│  ExecutionLayer — ProtocolTypeLimited                                    │
│  Backpressure(2) + Serial Queue + channelMu                              │
├───────────────────────────┬─────────────────────────────────────────────┤
│                           │                                             │
│  ┌────────────────────────▼────────────────────────────────────────┐    │
│  │              EtherCATDriver（纯执行函数）                         │    │
│  │  Init / ReadPoints / WritePoint / Health                        │    │
│  │  Connect / Disconnect / IsConnected                             │    │
│  │  SetDeviceConfig / GetConnectionMetrics                         │    │
│  └────────────────────────┬────────────────────────────────────────┘    │
│                           │                                             │
│  ┌────────────────────────▼────────────────────────────────────────┐    │
│  │              EtherCATTransport（传输层）                          │    │
│  │  主站生命周期 · 状态机 · PDO 周期线程 · PDO 快照                  │    │
│  │  SDO 邮箱通道 · ConnectionManager 集成                            │    │
│  └────────────┬───────────────────┬──────────────────┬─────────────┘    │
│  ┌────────────▼─────┐  ┌──────────▼──────────┐  ┌────▼──────────────┐  │
│  │ EtherCATScheduler│  │  EtherCATDecoder    │  │  EtherCATAddress   │  │
│  │ 批量读编排        │  │  类型/字节序编解码  │  │  地址解析          │  │
│  └──────────────────┘  └─────────────────────┘  └────────────────────┘  │
├─────────────────────────────────────────────────────────────────────────┤
│  Network Layer                                                           │
│  原始以太网帧 / 专用网卡驱动（Linux SOEM/IgH via CGO）                    │
└────────────────────────────────────┬────────────────────────────────────┘
                                     │
                                     ▼
                    EtherCAT 菊花链 / 树形拓扑（从站 1…N）
```

---

## 3. Driver 接口实现规范

EtherCAT 驱动须完整实现 `internal/driver/interface.go` 中的 `Driver` 接口（8 个核心方法），并按需实现可选接口。

### 3.1 核心接口方法映射

| 接口方法 | EtherCAT 实现语义 | 约束 |
|----------|-------------------|------|
| `Init(cfg model.DriverConfig)` | 解析通道配置（网卡、周期、超时）；创建 Transport / Scheduler / Decoder | 不建立网络连接 |
| `Connect(ctx)` | 初始化主站 → 扫描从站 → 推进状态机至 OP → 启动 PDO 周期线程 | 经 ConnectionManager 单一入口 |
| `Disconnect()` | 停止周期线程 → 关闭主站 → 释放网卡资源 | 幂等，可重复调用 |
| `ReadPoints(ctx, points)` | 从 PDO 快照内存按地址切片 + 解码；SDO 点位走邮箱通道 | **零等待**内存读（PDO）；SDO 有独立超时 |
| `WritePoint(ctx, point, value)` | 编码值 → 写入 RxPDO 待发缓冲；SDO 写走邮箱通道 | 不立即发送，下一周期下发 |
| `Health()` | 主站状态机是否 OP + 周期线程是否存活 + 从站是否在线 | 返回 `HealthStatusGood` / `Bad` |
| `SetSlaveID(slaveID)` | 无意义（EtherCAT 按位置寻址，非 Slave ID） | 空实现或返回 nil |
| `SetDeviceConfig(config)` | 设置当前设备的从站位置、PDO 长度等 | 每设备切换上下文 |
| `GetConnectionMetrics()` | 返回连接时长、重连次数、本地/远端地址 | 供 diagnostics 采集 |

### 3.2 可选接口

| 接口 | 是否实现 | 说明 |
|------|----------|------|
| `Scanner` | ✅ M1 | `Scan(ctx, params)` 枚举总线上从站（位置、Vendor ID、Product Code），供 UI 设备向导使用 |
| `ObjectScanner` | ⏳ M3 | `ScanObjects(ctx, config)` 读取从站对象字典，辅助 PDO 自动映射 |
| `DeviceCollectionResetter` | ✅ M1 | `ResetDeviceCollection(deviceID)` 清理设备级 PDO 快照缓存，供 ScanEngine 在点位增删时调用 |

### 3.3 结构体定义（草案）

```go
type EtherCATDriver struct {
    config     model.DriverConfig
    channelCfg channelConfig
    transport  *EtherCATTransport
    decoder    *EtherCATDecoder
    scheduler  *EtherCATScheduler
}

type channelConfig struct {
    localInterface string        // 绑定网卡，如 eth0
    cycleTime      time.Duration // PDO 交换周期，默认 1ms
    timeout        time.Duration // SDO / 状态切换超时，默认 3s
    maxRetries     int           // 链路异常重试，默认 3
    simulation     bool          // 模拟模式（无真实网卡）
}

// Transport 持有 PDO 周期线程与快照内存
type EtherCATTransport struct {
    channelCfg  channelConfig
    master      etherCATMaster  // CGO 接口（SOEM/IgH/模拟器）
    connMgr     *driver.ConnectionManager

    cycleStopCh chan struct{}
    cycleWG     sync.WaitGroup
    cycleRunning atomic.Bool

    // PDO 快照（周期线程写，ReadPoints 读）
    txSnapshot  sync.Map  // map[position]*atomic.Pointer[[]byte]
    rxBuffers   sync.Map  // map[position]*[]byte（mutex 保护）

    connected   atomic.Bool
    reconnectCount int64
    connectTime    time.Time
}

// etherCATMaster 抽象主站后端
type etherCATMaster interface {
    init(iface string) error
    scanSlaves() ([]slaveInfo, error)
    bringToOP(positions []int) error
    sendProcessdata() error
    receiveProcessdata() error
    getTxPDO(position int) []byte
    setRxPDO(position int, data []byte)
    readSDO(position int, index, subindex uint16) ([]byte, error)
    writeSDO(position int, index, subindex uint16, data []byte) error
    close() error
}
```

### 3.4 模块划分

| 模块 | 文件 | 职责 | 参考 |
|------|------|------|------|
| 驱动主模块 | `ethercat.go` | `Driver` 接口实现、`init()` 注册 `ethercat`、委托 transport/scheduler/decoder | `profinetio.go` |
| 传输层 | `transport.go` | 主站生命周期、状态机、PDO 周期线程、快照内存、SDO 邮箱、ConnectionManager 集成 | `profinetio/transport.go` |
| 主站绑定 | `master_cgo.go` | SOEM CGO 封装（build tag `cgo && linux`） | **新建** |
| 主站绑定 | `master_igh.go` | IgH 用户态 API 封装（build tag `igh`） | **新建** |
| 主站模拟 | `master_sim.go` | 内存模拟主站（build tag `!cgo` 或 `simulation`） | `knxnetip/simulator.go` |
| 调度器 | `scheduler.go` | 批量 `ReadPoints` 编排、`WritePoint` 编码、SDO 队列 | `profinetio/scheduler.go` |
| 解码器 | `decoder.go` | 地址解析、字节序、数据类型转换 | `profinetio/decoder.go` |
| 地址 | `address.go` | `POSITION:PDO:OFFSET` / SDO 解析 | `profinetio/address.go` |
| 配置 | `config.go` | 通道网卡、周期、超时；设备从站参数 | `profinetio/config.go` |
| 模拟器 | `simulator.go` | 单元测试用虚拟从站 PDO 映像 | `knxnetip/simulator.go` |

---

## 4. ScanEngine 集成方案

### 4.1 协议注册

EtherCAT 注册为 `ProtocolTypeLimited`，与 `profinet-io`、`ethernet-ip`、`s7` 同类——单通道单主站连接互斥，低并发执行：

```go
// internal/core/channel_manager.go — registerProtocolToScanEngine
case "s7", "ethernet-ip", "profinet-io", "iec60870-5-104", "ethercat":
    cm.scanEngineAdapter.scanEngine.RegisterProtocol(protocol, ProtocolTypeLimited)
```

**Limited 路径执行流程**：

```text
ScanEngine.processReadyTasks()
  → ExecutionLayer.Execute(task)
    → BackpressureController.AllowWithReason(deviceLimit=2)  ← 全局512 + 单设备2
    → SerialQueueManager.Enqueue(task)                       ← 串行队列
    → readPoints(task, channelMu)                            ← channelMu 互斥 I/O
      → EtherCATDriver.ReadPoints(ctx, points)              ← 从 PDO 快照读
    → ExecuteResult{values, err}
  → ShadowIngress.IngestDirect(values)
  → updateTaskState(task, result)                            ← CB 计数 / RTT 更新
  → finalizeScanCollect → 设备状态
```

选择 Limited 而非 Serial 的理由：EtherCAT 主站实例绑定单块网卡，通道内所有从站共享同一主站连接。`ProtocolTypeLimited` 的单设备并发 2 允许 PDO 读与 SDO 读并行（若同一设备同时有 PDO 和 SDO 点位），而 `channelMu` 确保同一通道的主站 IO 不并发。

### 4.2 Scan Class 与 PDO 周期的关系

ScanEngine 按 Scan Class（fast=100ms / normal=设备默认 / slow=10s）将点位分组为独立 ScanTask。EtherCAT 的 PDO 读取为快照内存读取（微秒级），因此：

| Scan Class | ScanEngine 调度间隔 | EtherCAT 实际数据新鲜度 | 说明 |
|------------|---------------------|------------------------|------|
| fast | 100ms | ≤1ms（PDO 周期） | 快照已由周期线程持续刷新 |
| normal | 1s | ≤1ms | 同上 |
| slow | 10s | ≤1ms | SDO 点位可能走此 class |

**关键约束**：ScanEngine 调度间隔与 PDO 周期解耦。PDO 周期线程以 `cycleTime`（默认 1ms）独立运行，ScanEngine 的 `ReadPoints` 仅从快照读取最新值。这意味着即使 ScanEngine 调度间隔为 100ms，用户仍可获得 1ms 粒度的过程数据更新。

### 4.3 断路器交互

ScanEngine 的 `DriverCircuitBreaker` 对 EtherCAT 设备的行为：

| CB 参数 | 默认值 | EtherCAT 场景表现 |
|---------|--------|-------------------|
| 连续超时 | 5 次 | PDO 读为内存操作几乎不超时；SDO 读可能超时触发 |
| 失败率窗口 | 60s 内 ≥40%（≥10 样本） | 主站状态机退出 OP 或周期线程崩溃时快速触发 |
| Open 持续 | 30s | HalfOpen 探测 = `Health()` 检查主站状态 |
| 恢复 | HalfOpen → 自动恢复 | 主站重新进入 OP 后自动恢复采集 |

**主站级故障 vs 设备级故障**：当主站整体崩溃（网卡异常、SOEM 库 panic）时，通道内所有设备的 CB 会同时触发。`ChannelManager.finalizeScanCollect` 的链路级错误隔离逻辑应将此类故障标记为通道级 Offline，而非逐设备 CB 计数。这需要在 `finalizeScanCollect` 中识别 EtherCAT 主站级错误（如 `ErrMasterDown`）并走通道级降级路径。

### 4.4 自适应降速与背压

| 机制 | 对 EtherCAT 的影响 |
|------|-------------------|
| AdaptiveThrottle（≤4× 间隔） | ScanEngine 拉大调度间隔；PDO 周期线程不受影响（独立运行） |
| ProtocolCongestion（Token Bucket） | EtherCAT 无独立速率桶（Limited 路径），复用默认桶 |
| BackpressureController | 单设备并发 ≤2；PDO 读 + SDO 读可并行 |
| GC 反压 | GC pause >20ms 时 ScanEngine 降速；周期线程可能受 GC 影响导致 PDO 抖动 |

### 4.5 代码注册点清单

EtherCAT 驱动落地需在以下位置「登记」：

| # | 位置 | 文件 | 改动 |
|---|------|------|------|
| 1 | blank import | `cmd/main.go` | 增加 `_ "github.com/anviod/edgeCore/internal/driver/ethercat"` |
| 2 | 协议类型 | `internal/core/channel_manager.go` `registerProtocolToScanEngine` | `ProtocolTypeLimited` case 增加 `"ethercat"` |
| 3 | 点位校验 | `internal/core/channel_manager.go` `validatePoint` | 增加 `case "ethercat": return cm.validateEtherCATPoint(point)` |
| 4 | 驱动注册 | `internal/driver/ethercat/ethercat.go` `init()` | `driver.RegisterDriver("ethercat", ...)` |

---

## 5. 连接管理与主站状态机

### 5.1 ConnectionManager 集成

遵循 ScanEngine 重构方案 §5.3 的「单一 Owner」原则，EtherCAT 的所有连接操作（主站初始化、网卡绑定）必须经 `driver.ConnectionManager` 进入：

```go
// 同步路径：Connect 时初始化主站
func (t *EtherCATTransport) Connect(ctx context.Context) error {
    t.mu.Lock()
    defer t.mu.Unlock()
    return t.connMgr.EnsureConnected(ctx, t.connectOnce)
}

// connectOnce：实际主站初始化（唯一 dial 入口）
func (t *EtherCATTransport) connectOnce(ctx context.Context) error {
    if err := t.master.init(t.channelCfg.localInterface); err != nil {
        return err  // ConnectionManager.RecordFailure → 指数退避
    }
    slaves, err := t.master.scanSlaves()
    if err != nil {
        return err
    }
    if err := t.master.bringToOP(positions); err != nil {
        return err
    }
    t.startCycleThread()  // 启动 PDO 周期线程
    t.connected.Store(true)
    t.connectTime = time.Now()
    return nil
}

// 异步路径：周期线程检测到主站异常
func (t *EtherCATTransport) scheduleReconnect() {
    t.connMgr.ScheduleReconnect(ctx, timeout, func(ctx context.Context) error {
        t.mu.Lock()
        defer t.mu.Unlock()
        return t.connectOnce(ctx)
    })
}
```

**重连退避参数**（与全局一致）：

| 参数 | 值 | 说明 |
|------|-----|------|
| `baseDelay` | 100ms | 指数退避基数 |
| `maxDelay` | 30s | 单次退避上限 |
| `backoffFactor` | 2.0 | 指数因子 |
| `maxRetries` | 64 | 进入 Dead 前最大尝试 |
| `MaxGlobalReconnectRate` | 10/s | 全局重连令牌桶（与其他协议共享） |

### 5.2 EtherCAT 状态机

主站与从站需按顺序进入运行态：

```
INIT → PREOP → SAFEOP → OP
```

| 状态 | 能力 | 驱动行为 |
|------|------|----------|
| INIT | 扫描从站、分配地址 | `connectOnce` 阶段 1 |
| PREOP | 邮箱通信（SDO、FoE） | `connectOnce` 阶段 2：配置 PDO 映射 |
| SAFEOP | 传输 PDO，输出安全态 | `connectOnce` 阶段 3 |
| OP | 完整过程数据输入输出 | `connectOnce` 完成 → 启动周期线程 |

`Connect` 成功须达到 **OP** 状态。`Health()` 在主站非 OP 时返回 `HealthStatusBad`，通道内设备标记 Offline。

**状态机异常处理**：

| 异常 | 检测方式 | 处理 |
|------|----------|------|
| 从站掉线 | PDO WDT 超时 / `ec_slaveconfig.state` != OP | 周期线程标记该从站 Bad；其他从站不受影响 |
| 主站崩溃 | `ec_send_processdata` 返回错误 | 停止周期线程 → `scheduleReconnect` |
| 状态机回退 | SAFEOP → PREOP（从站异常） | `Health()` 返回 Bad；CB 开始计数 |

### 5.3 PDO 周期线程设计

```go
func (t *EtherCATTransport) startCycleThread() {
    t.cycleStopCh = make(chan struct{})
    t.cycleRunning.Store(true)
    t.cycleWG.Add(1)
    go t.pdoCycle()
}

func (t *EtherCATTransport) pdoCycle() {
    defer t.cycleWG.Done()
    ticker := time.NewTicker(t.channelCfg.cycleTime)  // 默认 1ms
    defer ticker.Stop()

    for {
        select {
        case <-t.cycleStopCh:
            return
        case <-ticker.C:
            // 1. 发送 PDO 帧（含 RxPDO 待发缓冲）
            if err := t.master.sendProcessdata(); err != nil {
                t.handleCycleError(err)
                return
            }
            // 2. 接收 PDO 帧
            if err := t.master.receiveProcessdata(); err != nil {
                t.handleCycleError(err)
                return
            }
            // 3. 刷新 TxPDO 快照（atomic 写，ReadPoints 无锁读）
            t.refreshTxSnapshot()
        }
    }
}

func (t *EtherCATTransport) refreshTxSnapshot() {
    t.txSnapshot.Range(func(key, val any) bool {
        position := key.(int)
        ptr := val.(*atomic.Pointer[[]byte])
        data := t.master.getTxPDO(position)
        if len(data) > 0 {
            snapshot := make([]byte, len(data))
            copy(snapshot, data)
            ptr.Store(&snapshot)
        }
        return true
    })
}
```

**线程安全模型**：

| 数据 | 写入者 | 读取者 | 同步机制 |
|------|--------|--------|----------|
| TxPDO 快照 | 周期线程 | ReadPoints（ScanEngine goroutine） | `atomic.Pointer[[]byte]`（无锁读） |
| RxPDO 缓冲 | WritePoint（ScanEngine goroutine） | 周期线程 | `sync.Mutex`（per-position） |
| 主站状态 | 周期线程 | Health() | `atomic.Bool` / `atomic.Int32` |

---

## 6. PDO / SDO 读写语义

### 6.1 PDO 读（ReadPoints 主路径）

```go
func (s *EtherCATScheduler) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
    results := make(map[string]model.Value, len(points))
    for _, p := range points {
        addr, err := ParseAddress(p.Address)
        if err != nil {
            results[p.ID] = model.Value{PointID: p.ID, Quality: "Bad", TS: time.Now()}
            continue
        }

        if addr.IsSDO {
            // SDO 路径：异步邮箱，独立超时
            data, err := s.transport.readSDO(ctx, addr.Position, addr.Index, addr.SubIndex)
            if err != nil {
                results[p.ID] = model.Value{PointID: p.ID, Quality: "Bad", TS: time.Now()}
                continue
            }
            val, _ := s.decoder.DecodeValue(data, p.DataType, addr)
            results[p.ID] = model.Value{PointID: p.ID, Value: val, Quality: "Good", TS: time.Now()}
        } else {
            // PDO 路径：从快照内存读取（零等待）
            data := s.transport.getTxPDOSnapshot(addr.Position, addr.Offset, s.decoder.ByteSize(p.DataType))
            if data == nil {
                results[p.ID] = model.Value{PointID: p.ID, Quality: "Bad", TS: time.Now()}
                continue
            }
            val, _ := s.decoder.DecodeValue(data, p.DataType, addr)
            if p.Scale != 0 || p.Offset != 0 {
                val = toFloat64(val)*p.Scale + p.Offset
            }
            results[p.ID] = model.Value{PointID: p.ID, Value: val, Quality: "Good", TS: time.Now()}
        }
    }
    return results, nil
}
```

### 6.2 PDO 写（WritePoint）

```go
func (s *EtherCATScheduler) WritePoint(ctx context.Context, point model.Point, value any) error {
    addr, err := ParseAddress(point.Address)
    if err != nil {
        return err
    }
    if addr.IsSDO {
        data, _ := s.decoder.EncodeValue(value, point.DataType, addr)
        return s.transport.writeSDO(ctx, addr.Position, addr.Index, addr.SubIndex, data)
    }
    // RxPDO 写：编码后写入待发缓冲，下一周期由周期线程下发
    data, _ := s.decoder.EncodeValue(value, point.DataType, addr)
    return s.transport.setRxPDOBuffer(addr.Position, addr.Offset, data)
}
```

### 6.3 SDO 邮箱通道

| 属性 | 值 | 说明 |
|------|-----|------|
| 触发方式 | ReadPoints 中 SDO 地址点位 / WritePoint SDO 写 | 与 PDO 点位混在同一 ReadPoints 调用 |
| 超时 | 3000ms（独立配置） | 不受 `executeTimeout` 的 `max(interval×2, 5s)` 约束 |
| 阻塞性 | 同步等待邮箱响应 | 单次 SDO 请求阻塞当前 ReadPoints 调用 |
| 并发保护 | channelMu + Serial Queue | 同通道 SDO 请求串行化 |
| 失败影响 | 仅该点位 Quality=Bad | 不影响 PDO 周期线程、不触发 CB（除非连续失败） |

**SDO 与 PDO 是否共用 channelMu**：首版建议共用。SDO 请求经 CoE 邮箱发送，主站库内部已有邮箱队列，但为避免 SOEM API 的线程安全问题，SDO 读写也应在 `channelMu` 保护下执行。这与 Profinet IO 的 `ProtocolTypeLimited` 单通道互斥模型一致。

---

## 7. 配置与数据模型

### 7.1 通道配置

| 参数名 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `local_interface` | string | — | 绑定网卡名（必填） |
| `cycle_time_us` | int | 1000 | PDO 周期（微秒） |
| `timeout` | int | 3000 | SDO / 状态切换超时（毫秒） |
| `max_retries` | int | 3 | 链路异常重试 |
| `simulation` | bool | false | 模拟模式（无真实网卡） |

### 7.2 设备配置

| 参数名 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `position` | int | — | 从站位置 1…N |
| `alias` | int | 0 | 可选别名 |
| `vendor_id` | string | — | 如 `0x00000002` |
| `product_code` | string | — | 如 `0x044c2c52` |
| `revision` | string | — | 可选 |
| `tx_pdo_size` | int | 0 | TxPDO 字节长度 |
| `rx_pdo_size` | int | 0 | RxPDO 字节长度 |

### 7.3 点位配置

- `address`：见上文 PDO / SDO 格式
- `datatype`：INT8 … DOUBLE / BIT
- `readwrite`：`R` 对应 TxPDO；`RW` 允许 RxPDO 或 SDO 写
- `scan_class`：fast / normal / slow（与全局 ScanEngine 一致；PDO 读仍受主站周期约束）

### 7.4 配置示例（规划）

```yaml
protocol: ethercat
config:
  local_interface: eth0
  cycle_time_us: 1000
  timeout: 3000
  max_retries: 3
  simulation: false
```

```yaml
# 设备
position: 1
vendor_id: "0x00000002"
product_code: "0x07d43052"
tx_pdo_size: 16
rx_pdo_size: 8
```

```yaml
# 点位
points:
  - name: "di-0"
    address: "1:Tx:0"
    datatype: "UINT16"
    readwrite: "R"
  - name: "target-speed"
    address: "1:Rx:4"
    datatype: "INT32"
    readwrite: "RW"
  - name: "drive-status"
    address: "1:SDO:0x6041:0"
    datatype: "UINT16"
    readwrite: "R"
```

---

## 8. 模拟器与测试方案

### 8.1 模拟器设计

参照 KNX `simulator.go` 模式，在 `master_sim.go` 中实现 `etherCATMaster` 接口的内存版本：

```go
type simulatorMaster struct {
    mu       sync.Mutex
    slaves   map[int]*slaveSim  // position → 从站模拟
    opState  atomic.Int32       // 0=INIT, 1=PREOP, 2=SAFEOP, 3=OP
}

type slaveSim struct {
    txPDO    []byte              // TxPDO 映像（主站读）
    rxPDO    []byte              // RxPDO 映像（主站写）
    sdoDict  map[uint32][]byte   // 0xINDEX<<16 | 0xSUBINDEX → value
}
```

模拟器在 `CGO_ENABLED=0` 时编译（build tag `!cgo`），使纯逻辑单测在 CI 无硬件环境下运行：

```bash
# 纯逻辑单测（无 CGO、无硬件）
CGO_ENABLED=0 go test ./internal/driver/ethercat/... -count=1 -v
```

### 8.2 单元测试（无硬件）

| 测试范围 | 方法 | 预期 |
|----------|------|------|
| 地址解析 | PDO / SDO 格式、位偏移、字节序 | 正则匹配所有合法格式 |
| 解码 | 各数据类型边界值 | 与 Profinet decoder 行为一致 |
| PDO 读 | 模拟器预置 TxPDO → ReadPoints 切片 | 值与偏移对应 |
| PDO 写 | WritePoint → 检查 RxPDO 缓冲 | 下一周期下发 |
| SDO 读写 | 模拟器预置对象字典 | 读写一致 |
| 状态机 | 模拟 INIT → OP 流程 | Health() 返回 Good |
| 周期线程 | 启动/停止/异常退出 | 无 goroutine 泄漏 |
| ScanEngine 闭环 | 模拟模式全链路 | 四路数据一致 |

### 8.3 集成测试（CGO + 硬件或 ESC 模拟器）

| 测试项 | 方法 | 预期 |
|--------|------|------|
| 总线扫描 | 接入 1–3 个从站模块 | 枚举位置与 Vendor/Product 正确 |
| OP 进入 | 查看驱动 Health | Good |
| TxPDO 读 | 数字量输入模块 | 与 TwinCAT / slaveinfo 一致 |
| RxPDO 写 | 数字量输出模块 | 输出翻转 |
| 断线恢复 | 拔插网线 | 重连或通道 Offline，行为符合 `channel_device_state` |
| ScanEngine 闭环 | 周期读 → Shadow → UI | 四路数据一致 |
| 断路器验证 | 模拟从站掉线 | CB Open → 30s → HalfOpen → 恢复 |
| SDO 读写 | CiA402 状态字 `0x6041` | 读到正确状态 |

集成测试单独 CI 门禁（build tag `cgo && integration`），不阻塞纯逻辑单测流水线。

### 8.4 性能基线（待测）

| 指标 | 目标 |
|------|------|
| PDO 周期 | 可配置 1–10 ms |
| 单通道从站数 | ≥32（待压测） |
| 单通道点位 | ≥1000 PDO 点位（待压测） |
| ReadPoints 延迟 | <1ms（快照内存读，1000 点位） |
| 周期线程 CPU | <5%（单核，1ms 周期，32 从站） |

---

## 9. 诊断与 SLA 集成

### 9.1 诊断 API 集成

EtherCAT 驱动须接入 ScanEngine 现有诊断体系（零外部依赖）：

| 通路 | 机制 | EtherCAT 暴露字段 |
|------|------|-------------------|
| 读 | `GET /api/diagnostics/scan-engine` | `circuit_breaker`（每设备 CB 状态）、`serial_queue_depth` |
| 判 | `sla_warnings[]` | 主站非 OP 告警、周期线程停止告警 |
| 告 | zap WARN 日志 + channel Event Log | CB Open、从站掉线、状态机回退 |
| 看 | UI 通道监控 SLA 区块 | `ChannelMetricsPanel.vue` |

### 9.2 驱动级诊断指标

`GetConnectionMetrics()` 返回的指标经 ChannelManager 上报至 diagnostics：

| 指标 | 来源 | 说明 |
|------|------|------|
| `connection_seconds` | `time.Since(connectTime)` | 主站连接时长 |
| `reconnect_count` | `connMgr` | 重连次数 |
| `local_addr` | `localInterface` | 绑定网卡名 |
| `remote_addr` | — | EtherCAT 无远端 IP，留空或填 `ethercat-bus` |

### 9.3 SLA 阈值适用性

EtherCAT PDO 读为内存操作，以下 ScanEngine SLA 阈值天然满足：

| 指标 | 阈值 | EtherCAT 表现 |
|------|------|---------------|
| 调度 lag P95 | <100ms | PDO 读 <1ms，lag 主要来自 ScanEngine 调度开销 |
| 漂移均值 | <50ms | 同上 |
| miss deadline | 稳态 =0 | 内存读不超时 |
| GC pause max | <20ms | 周期线程可能受 GC 影响（需联调验证） |

**风险点**：GC pause >1ms 可能导致 PDO 周期线程丢帧（cycle miss）。若现场对周期稳定性要求高，需评估 GOGC 调优或 `runtime.LockOSThread` 隔离周期线程。

---

## 10. 技术选型

### 10.1 主站实现路线（待选型确认）

| 方案 | 说明 | 优点 | 风险 |
|------|------|------|------|
| **SOEM**（Simple Open EtherCAT Master） | C 开源主站库，LGPL | 社区广、示例多、跨平台 | 需 CGO；Windows 支持弱于 Linux |
| **IgH EtherCAT Master** | Linux 内核模块 + 用户态 API | 工业现场常用、实时性好 | 仅 Linux；内核模块部署复杂 |
| **商用 SDK**（如 acontis、Beckhoff ADS 网关） | 商业授权 | 厂商支持 | 授权成本、绑定供应商 |

**当前建议**：Linux 工业网关优先评估 **SOEM + CGO 薄封装** 实现 M1；若客户现场已标准化 IgH，可在 M2 增加 IgH 后端抽象。最终选型需在 spike（概念验证）后确认。

### 10.2 CGO 交叉编译策略

| 目标平台 | 编译方式 | 测试策略 |
|----------|----------|----------|
| x86_64 Linux | `CGO_ENABLED=1` + gcc | 集成测试 + 硬件 |
| ARM64 Linux | `CGO_ENABLED=1` + aarch64-linux-gnu-gcc | 板端集成测试 |
| ARMv7 Linux | `CGO_ENABLED=1` + arm-linux-gnueabihf-gcc | 板端 SLA 复验 |
| 任意平台（纯逻辑） | `CGO_ENABLED=0` + build tag `!cgo` | 单元测试（模拟器） |

build tag 隔离策略：

```go
//go:build cgo && linux
// +build cgo,linux

package ethercat
// master_cgo.go — SOEM CGO 封装
```

```go
//go:build !cgo || !linux
// +build !cgo !linux

package ethercat
// master_sim.go — 内存模拟（无 CGO 依赖）
```

---

## 11. 前端集成清单

| 项 | 路径 | 状态 |
|----|------|------|
| 协议列表 | `ui/src/utils/protocolLabel.js` | ⏳ |
| 通道默认配置 | `ui/src/utils/channelDefaultConfig.js` | ⏳ |
| 通道表单 | `ui/src/views/ChannelList.vue` | ⏳ |
| 设备表单 | `ui/src/views/DeviceList.vue` | ⏳ |
| 点位地址提示 | `ui/src/views/PointList.vue` | ⏳ |
| 帮助组件 | `ui/src/components/channel-help/EthercatHelp.vue` | ⏳ |
| 协议图标 | `ui/src/views/Dashboard.vue` / CSS | ⏳ |
| 用户手册 | `docs/drivers/EtherCAT.md` | ⏳ |

---

## 12. 实现计划与里程碑

### M0 — 选型验证（2 周，待启动）

- [ ] SOEM vs IgH spike：单网卡扫描 3 从站、1 ms 周期 PDO 交换
- [ ] 评估 CGO 交叉编译对 ARM64/ARMv7 网关镜像的影响
- [ ] 确认目标网卡型号与内核版本
- [ ] 验证 PDO 周期线程在 Go runtime 下的抖动基线（GC pause 影响）

### M1 — 最小可用主站（4–6 周）

- [ ] 创建 `internal/driver/ethercat/` 包与 `driver.RegisterDriver("ethercat", ...)`
- [ ] `cmd/main.go` blank import
- [ ] `channel_manager.go`：`registerProtocolToScanEngine` + `validateEtherCATPoint`
- [ ] Transport 层：主站 init、扫描从站、状态机推进至 OP、PDO 周期线程
- [ ] PDO 读：`ReadPoints` + 地址解析 + 解码单元测试
- [ ] RxPDO 写：`WritePoint`
- [ ] `simulation: true` 模拟模式（无硬件 CI）
- [ ] `master_sim.go` 内存模拟器（`CGO_ENABLED=0` 可编译）
- [ ] ScanEngine `ProtocolTypeLimited` 注册
- [ ] ConnectionManager 集成（`EnsureConnected` + `ScheduleReconnect`）
- [ ] `Scanner` 接口实现（总线扫描 → UI 设备向导）
- [ ] 前端：`protocolLabel.js`、`channelDefaultConfig.js`、基础表单

### M2 — 生产化（4 周）

- [ ] CoE SDO 读写（邮箱通道 + 独立超时）
- [ ] 拓扑变化检测与重连策略
- [ ] 主站级 vs 设备级故障隔离（`finalizeScanCollect` 适配）
- [ ] `EthercatHelp.vue` 帮助文档
- [ ] 用户手册 `docs/drivers/EtherCAT.md`
- [ ] 真实 IO 耦合器 / 伺服联调报告
- [ ] 诊断字段接入 `GET /api/diagnostics/scan-engine`

### M3 — 增强（后续）

- [ ] ESI 导入与 PDO 自动映射
- [ ] `ScanObjects` 从站对象扫描
- [ ] `ObjectScanner` 接口实现
- [ ] 南向驱动测试报告条目、Soak 长稳
- [ ] IgH 后端抽象（build tag 切换）

---

## 13. 风险与依赖

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| **CGO / 内核模块** | 交叉编译、CI 复杂度上升 | build tag 隔离；CI 分「逻辑单测」与「主站集成」job |
| **网卡与实时性** | 普通网卡抖动导致 PDO 超时 | 维护网卡白名单；文档明确部署约束 |
| **PDO 映射复杂** | 用户手工配置门槛高 | M1 文档 + 示例；M3 ESI 工具 |
| **与运动控制争用** | 同一主站承担硬实时控制 | 定位「网关采集」，不建议替代 PLC 主站 |
| **Windows 工控机** | SOEM/IgH 支持有限 | 首版仅支持 Linux 网关；Windows 待确认 |
| **GC 影响 PDO 周期** | Go GC pause >1ms 导致丢帧 | 评估 GOGC 调优 / `LockOSThread`；M0 spike 验证 |
| **周期线程 vs Driver 约束** | 架构评审可能质疑 goroutine | 周期线程属 Transport 层（连接资源），非 Driver 调度逻辑；类比 Profinet rpcClient |
| **产品说明表述** | 用户以为已支持 | 本页状态栏 + README 对齐 |

**外部依赖（规划）**：

- SOEM 或 IgH EtherCAT Master（C 库，许可证合规审查 **待法务确认**）
- Linux 原始套接字或 PF_PACKET 权限（容器需 `cap_net_raw` 或 host network，**待运维规范**）

---

## 14. 主站选型方案对比

### 14.1 SOEM + CGO 薄封装（推荐 M1）

**架构**：

```text
EtherCATTransport
  └─ master_cgo.go (CGO 封装)
       └─ SOEM C 库 (libsoem)
            └─ Linux PF_PACKET 原始套接字
```

**优点**：
- 纯用户态，无需内核模块
- 社区活跃，文档丰富（OpenEtherCATsociety）
- 跨平台（Linux/macOS，Windows 支持有限）
- LGPL 许可证，合规审查相对简单

**缺点**：
- 需 CGO 编译，交叉编译复杂度增加
- 实时性依赖用户态调度（无内核级保证）
- 周期线程受 Go GC 影响

**CGO 封装要点**：
- SOEM 的 `ec_init` / `ec_config_init` / `ec_config_map` / `ec_send_processdata` / `ec_receive_processdata` 封装为 Go 函数
- PDO 映像区通过 `ec_slave[].inputs` / `ec_slave[].outputs` 指针访问
- SDO 通过 `ec_SDOread` / `ec_SDOwrite` 封装
- 错误码映射为 Go error

### 14.2 IgH EtherCAT Master（M2 备选）

**架构**：

```text
EtherCATTransport
  └─ master_igh.go (CGO 封装)
       └─ IgH 用户态库 (libethercat)
            └─ IgH 内核模块 (ec_master)
```

**优点**：
- 内核模块提供更好的实时性（可选 Xenomai 补丁）
- 工业现场已有部署基础
- 支持域（Domain）抽象，PDO 映射管理更规范

**缺点**：
- 仅 Linux，需安装内核模块
- 部署复杂度高于 SOEM
- GPL 许可证（内核模块），合规审查更严格

### 14.3 商用 SDK（按需评估）

如 acontis EC-Master、Beckhoff ADS 网关等，提供厂商级技术支持，但涉及授权成本和供应商绑定。适用于客户指定方案的场景。

---

## 附录 A：与 Profinet IO 架构对照

| 维度 | Profinet IO | EtherCAT（本规划） |
|------|-------------|-------------------|
| 设备寻址 | IP + 槽/子槽 | 从站位置 + Vendor/Product |
| 过程数据 | `SLOT:SUB_SLOT:INDEX` | `POSITION:Tx\|Rx:OFFSET` |
| 非周期数据 | RPC 读写 | `POSITION:SDO:0xINDEX:0xSUBINDEX` |
| 部署 | 物理网卡 | 物理网卡 + 主站库 |
| 传输层连接 | TCP（RFC1006） | 原始以太网帧（PF_PACKET） |
| 周期机制 | Profinet 周期帧（IO Data） | PDO 飞读帧 |
| CGO 依赖 | 无（纯 Go） | 有（SOEM/IgH C 库） |
| ExecutionLayer | `ProtocolTypeLimited` | `ProtocolTypeLimited` |
| 模拟器 | `simulationStore`（内存 IO 映像） | `simulatorMaster`（内存 PDO 映像） |
| ConnectionManager | ✅ 已集成 | ✅ 规划集成 |

## 附录 B：参考资料

- [EtherCAT Technology Group 规范](https://www.ethercat.org/)
- [SOEM](https://github.com/OpenEtherCATsociety/SOEM)
- [IgH EtherCAT Master](https://etherlab.org/en/ethercat/)
- 项目内参考：[Profinet IO 驱动文档](../../drivers/PLC_Profinet_IO.html)、[Profinet IO TODO](../Profinet%20IO/采集驱动Profinet%20IO.html)、[南向 TODO 索引](../index.html)
- 架构依据：[ScanEngine 重构方案](../ScanEngine重构方案.html)、[边缘网关架构设计总览](../../edge/边缘网关架构设计总览.html)

---

*维护：南向驱动组 | 架构依据：ScanEngine 重构方案 v5.0 · 边缘网关架构设计总览 v2.2 | 下次审查：M0 spike 完成后*
