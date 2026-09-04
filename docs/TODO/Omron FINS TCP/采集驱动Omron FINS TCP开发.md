
# Omron FINS TCP 采集驱动开发方案

## 1. 概述

### 1.1 协议简介

欧姆龙 FINS（Factory Interface Network Service）是一种用于工业自动化领域的通信协议，是欧姆龙公司开发的专有协议。该协议提供了高效、可靠的通信方式，用于在欧姆龙 PLC、传感器等设备之间进行数据交换。FINS 协议支持多种通信方式，包括串口、TCP、UDP 等。

本驱动实现 FINS TCP 协议的 Client 端功能，支持与欧姆龙 PLC 进行通信。

### 1.2 功能定位

| 功能类别 | 功能描述 | 支持区域 |
| :--- | :--- | :--- |
| 数据采集 | 读取 PLC 各区域数据 | CIO、W、H、D、A、P、F、EM |
| 数据写入 | 写入数据到 PLC 各区域 | CIO、W、H、D、EM、P |
| 位操作 | 读取/写入单个位 | CIO、W、H、D、A、P、EM |
| 字符串操作 | 读取/写入字符串数据 | 支持 H/L 字节顺序 |

### 1.3 设计原则

- **一致性**: 遵循项目统一的驱动接口规范，与 S7、EtherNet/IP 等驱动保持一致的设计风格
- **可扩展性**: 模块化设计，便于后续功能扩展
- **可靠性**: 完善的错误处理和重连机制
- **可测试性**: 支持依赖注入，便于单元测试

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
│          │                                                      │
├──────────┼──────────────────────────────────────────────────────┤
│                    Protocol Driver Layer                        │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │                  FinsTCPDriver                          │   │
│   │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────┐  │   │
│   │  │ transport│◄─│ scheduler│◄─│ decoder  │  │ config │  │   │
│   │  │ (TCP连接) │  │ (读写调度)│  │ (编解码) │  │        │  │   │
│   │  └────┬─────┘  └──────────┘  └──────────┘  └────────┘  │   │
│   │       │                                                 │   │
│   └───────┼─────────────────────────────────────────────────┘   │
├───────────┼──────────────────────────────────────────────────────┤
│                    Network Layer                                │
│                        TCP/IP                                  │
└───────────┴──────────────────────────────────────────────────────┘
```

### 2.2 模块划分

| 模块 | 文件 | 职责 | 对应参考 |
| :--- | :--- | :--- | :--- |
| **驱动主模块** | `fins.go` | 实现 Driver 接口，协调整体流程 | `s7.go`, `ethernetip.go` |
| **传输层** | `transport.go` | TCP 连接管理、FINS 帧收发、心跳检测 | `transport.go` |
| **调度器** | `scheduler.go` | 批量点位读写调度、请求分组 | `scheduler.go` |
| **解码器** | `decoder.go` | 地址解析、数据编解码 | `decoder.go` |

### 2.3 核心类/结构体设计

#### 2.3.1 FinsTCPDriver（驱动主类）

```go
type FinsTCPDriver struct {
    config    model.DriverConfig
    transport *FinsTransport
    decoder   *FinsDecoder
    scheduler *FinsScheduler
}
```

#### 2.3.2 FinsTransport（传输层）

```go
type FinsTransport struct {
    cfg               map[string]any
    conn              net.Conn
    connected         atomic.Bool
    connectTime       time.Time
    lastDisconnectTime time.Time
    reconnectCount    atomic.Int32
    localAddr         string
    remoteAddr        string
    
    // 配置参数
    plcIP             string
    plcPort           int
    maxFrameLength    int
    timeout           time.Duration
    
    // 连接状态
    heartbeatInterval time.Duration
    heartbeatTicker   *time.Ticker
    stopHeartbeat     chan struct{}
    
    // FINS Header 配置
    srcNetworkAddr    uint8
    srcNodeAddr       uint8
    srcUnitAddr       uint8
    dstNetworkAddr    uint8
    dstNodeAddr       uint8
    dstUnitAddr       uint8
}
```

#### 2.3.3 FinsScheduler（调度器）

```go
type FinsScheduler struct {
    transport     *FinsTransport
    decoder       *FinsDecoder
    
    // 配置
    maxFrameLength int           // 单次请求最大字节数
    minInterval    time.Duration // 指令最小间隔
    
    // 统计
    totalRequests int64
    successCount  int64
    failureCount  int64
    mu            sync.Mutex
}
```

#### 2.3.4 FinsDecoder（解码器）

```go
type FinsDecoder struct {
    // 区域定义映射
    areaMap map[string]AreaInfo
}

type AreaInfo struct {
    Code         uint8   // FINS区域代码
    Name         string  // 区域名称
    ReadOnly     bool    // 是否只读
    SupportBit   bool    // 是否支持位操作
    SupportString bool   // 是否支持字符串
}

type Address struct {
    Area        string  // 区域标识 (CIO, W, H, D, A, P, F, EM)
    AreaCode    uint8   // 区域代码
    EMNumber    int     // EM区域号（仅EM区域有效）
    Address     int     // 地址
    Bit         int     // 位偏移（-1表示不使用）
    StringLen   int     // 字符串长度（0表示非字符串）
    ByteOrder   ByteOrder // 字节顺序
}

type ByteOrder int
const (
    ByteOrderHigh ByteOrder = iota // H - 高字节在前
    ByteOrderLow                   // L - 低字节在前
)
```

---

## 3. 接口定义

### 3.1 驱动接口（实现 `driver.Driver`）

| 方法 | 功能 | 参数 | 返回值 |
| :--- | :--- | :--- | :--- |
| `Init(cfg model.DriverConfig) error` | 初始化驱动 | `cfg`: 驱动配置 | `error`: 错误信息 |
| `Connect(ctx context.Context) error` | 建立TCP连接 | `ctx`: 上下文 | `error`: 错误信息 |
| `Disconnect() error` | 断开连接 | 无 | `error`: 错误信息 |
| `ReadPoints(ctx, points) (map, error)` | 批量读取点位 | `points`: 点位列表 | `map[string]model.Value`: 读取结果 |
| `WritePoint(ctx, point, value) error` | 写入单个点位 | `point`: 点位, `value`: 值 | `error`: 错误信息 |
| `Health() driver.HealthStatus` | 获取健康状态 | 无 | `HealthStatus`: 健康状态 |
| `SetSlaveID(slaveID uint8) error` | 设置从站ID | `slaveID`: 从站地址 | `error`: 错误信息 |
| `SetDeviceConfig(config map[string]any) error` | 设置设备配置 | `config`: 配置参数 | `error`: 错误信息 |
| `GetConnectionMetrics() (...)` | 获取连接指标 | 无 | 连接时长、重连次数、地址信息 |

### 3.2 内部接口

#### 3.2.1 传输层接口

| 方法 | 功能 |
| :--- | :--- |
| `SendCommand(cmd *FinsCommand) (*FinsResponse, error)` | 发送FINS命令并等待响应 |
| `ReadMemory(area uint8, address int, count int, dataType model.DataType) ([]byte, error)` | 读取内存区域 |
| `WriteMemory(area uint8, address int, data []byte, dataType model.DataType) error` | 写入内存区域 |
| `ReadBit(area uint8, address int, bit int) (bool, error)` | 读取单个位 |
| `WriteBit(area uint8, address int, bit int, value bool) error` | 写入单个位 |

#### 3.2.2 调度器接口

| 方法 | 功能 |
| :--- | :--- |
| `ReadPoints(ctx, points)` | 批量读取点位 |
| `WritePoint(ctx, point, value)` | 写入点位 |
| `GetStats()` | 获取统计信息 |

#### 3.2.3 解码器接口

| 方法 | 功能 |
| :--- | :--- |
| `ParseAddress(addr string) (*Address, error)` | 解析地址字符串 |
| `EncodeValue(value any, dataType model.DataType) ([]byte, error)` | 编码值为字节数组 |
| `DecodeValue(data []byte, dataType model.DataType, addr *Address) (any, error)` | 解码字节数组为值 |
| `GetAreaCode(areaName string) (uint8, error)` | 获取区域代码 |

---

## 4. 配置参数

### 4.1 设备配置项

| 参数名 | 类型 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| `plcModel` | string | - | PLC 设备型号（可选） |
| `plcIP` | string | - | PLC IPv4 地址（必填） |
| `plcPort` | int | 9600 | PLC 端口号 |
| `maxFrameLength` | int | 64 | 查询数据包最大字节数 |
| `timeout` | int | 3000 | 通信超时时间（毫秒） |
| `heartbeatInterval` | int | 30000 | 心跳间隔（毫秒），0表示禁用 |
| `maxRetries` | int | 3 | 连接重试次数 |
| `retryInterval` | int | 1000 | 重试间隔（毫秒） |
| `srcNetworkAddr` | int | 0 | 源网络地址 |
| `srcNodeAddr` | int | 1 | 源节点地址 |
| `srcUnitAddr` | int | 255 | 源单元地址 |
| `dstNetworkAddr` | int | 0 | 目标网络地址 |
| `dstNodeAddr` | int | 1 | 目标节点地址 |
| `dstUnitAddr` | int | 0 | 目标单元地址 |

### 4.2 点位配置

#### 4.2.1 地址格式

```
AREA ADDRESS[.BIT][.LEN[H][L]]
```

#### 4.2.2 区域定义

| 区域 | 代码 | 数据类型支持 | 属性 | 备注 |
| :--- | :--- | :--- | :--- | :--- |
| CIO | 0x30 | 除 uint8/int8 外所有类型 | 读/写 | CIO 区 |
| A | 0x31 | 除 uint8/int8 外所有类型 | 读 | 辅助区 |
| W | 0x32 | 除 uint8/int8 外所有类型 | 读/写 | 工作区 |
| H | 0x33 | 除 uint8/int8 外所有类型 | 读/写 | 保持区 |
| D | 0x34 | 除 uint8/int8 外所有类型 | 读/写 | 数据存储区 |
| P | 0x35 | 除 uint8/int8 外所有类型 | 读/写 | PVs（bit只读） |
| F | 0x0B | int8/uint8 | 读 | 标志区域 |
| EM | 0x8n | 除 uint8/int8 外所有类型 | 读/写 | 扩展内存（n为EM号） |

#### 4.2.3 支持的数据类型

| 数据类型 | 说明 | 字节数 |
| :--- | :--- | :--- |
| BIT | 单个位 | - |
| UINT8 | 无符号8位整数 | 1 |
| INT8 | 有符号8位整数 | 1 |
| UINT16 | 无符号16位整数 | 2 |
| INT16 | 有符号16位整数 | 2 |
| UINT32 | 无符号32位整数 | 4 |
| INT32 | 有符号32位整数 | 4 |
| UINT64 | 无符号64位整数 | 8 |
| INT64 | 有符号64位整数 | 8 |
| FLOAT | 单精度浮点数 | 4 |
| DOUBLE | 双精度浮点数 | 8 |
| STRING | 字符串 | 可变 |

#### 4.2.4 地址示例

| 地址 | 数据类型 | 说明 |
| :--- | :--- | :--- |
| F0 | uint8 | F 区域，地址为 0 |
| F1 | int8 | F 区域，地址为 1 |
| CIO1 | int16 | CIO 区域，地址为 1 |
| CIO2 | uint16 | CIO 区域，地址为 2 |
| A2 | int32 | A 区域，地址为 2 |
| A4 | uint32 | A 区域，地址为 4 |
| W5 | float | W 区域，地址为 5 |
| H20 | double | H 区域，地址为 20 |
| D10 | int32 | D 区域，地址为 10 |
| EM10W100 | float | EM10 区域，地址为 100 |
| CIO0.0 | bit | CIO 区域，地址为 0，第 0 位 |
| CIO1.2 | bit | CIO 区域，地址为 1，第 2 位 |
| EM10W100.0 | bit | EM10 区域，地址为 100，第 0 位 |
| CIO0.20 | string | CIO 区域，地址为 0，字符串长度 20，字节顺序 L |
| CIO1.20H | string | CIO 区域，地址为 1，字符串长度 20，字节顺序 H |

---

## 5. 数据处理流程

### 5.1 连接建立流程

```
edgeCore                          Omron PLC
  |                               |
  |--- TCP Connect -------------->|
  |                               |
  |<-- TCP ACK -------------------|
  |                               |
  |--- FINS Command (No. 0601) --->|  连接测试
  |                               |
  |<-- FINS Response -------------|  响应正常，连接建立
```

### 5.2 FINS 帧结构

```
┌─────────────────────────────────────────────────────────┐
│ FINS TCP Frame                                         │
├────────────┬────────────┬──────────────────────────────┤
│ Header     │ Command    │ Data                         │
│ (10 bytes) │ (2 bytes)  │ (variable)                   │
├────────────┼────────────┼──────────────────────────────┤
│ 0x46494E53 │ Cmd Code   │ 参数/数据                    │
│ "FINS"     │            │                              │
└────────────┴────────────┴──────────────────────────────┘

FINS Command Structure:
┌────────┬────────┬────────┬────────┬────────┬────────┐
│ IC     │ RC     │ SNA    │ DNA    │ SA     │ DA     │
│ (1)    │ (1)    │ (1)    │ (1)    │ (1)    │ (1)    │
├────────┼────────┼────────┼────────┼────────┼────────┤
│ SSU    │ DSU    │ CMD    │ SUB    │ DATA...│        │
│ (1)    │ (1)    │ (1)    │ (1)    │        │        │
└────────┴────────┴────────┴────────┴────────┴────────┘

IC: 信息控制字段
RC: 响应码
SNA: 源网络地址
DNA: 目标网络地址
SA: 源节点地址
DA: 目标节点地址
SSU: 源单元地址
DSU: 目标单元地址
CMD: 主命令码
SUB: 子命令码
```

### 5.3 点位读取流程

```
┌─────────────────────────────────────────────────────────────┐
│                    采集组定时触发                           │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  Scheduler: 按区域分组点位                                 │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  Transport: 发送 FINS 读命令 (0101)                        │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  Transport: 接收 FINS 响应                                 │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  Decoder: 解码数据，转换为 model.Value                      │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  返回结果到 Device Service                                  │
└─────────────────────────────────────────────────────────────┘
```

### 5.4 点位写入流程

```
┌─────────────────────────────────────────────────────────────┐
│                    收到写入请求                             │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  Decoder: 编码值为字节数组                                  │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  Transport: 发送 FINS 写命令 (0102)                        │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  Transport: 接收 FINS 响应，检查执行结果                    │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  返回执行结果                                               │
└─────────────────────────────────────────────────────────────┘
```

---

## 6. 错误处理机制

### 6.1 错误分类

| 错误类型 | 触发条件 | 处理策略 |
| :--- | :--- | :--- |
| **连接错误** | TCP连接失败、超时 | 自动重连（带指数退避） |
| **帧错误** | 帧格式错误、校验失败 | 记录日志，丢弃错误帧 |
| **协议错误** | 区域不支持、地址格式错误 | 返回错误，不发送请求 |
| **设备错误** | PLC返回错误码 | 记录日志，标记点位质量为Bad |
| **超时错误** | 响应超时 | 重试或标记失败 |

### 6.2 FINS 错误码

| 错误码 | 说明 |
| :--- | :--- |
| 0x0000 | 正常 |
| 0x0101 | 服务未执行（设备忙） |
| 0x0201 | 不存在的命令 |
| 0x0202 | 参数错误 |
| 0x0301 | 不存在的地址 |
| 0x0302 | 不存在的数据 |
| 0x0401 | 访问权限错误 |
| 0x0501 | 运行错误 |

### 6.3 重连机制

```go
// 重连策略
maxRetries    int           // 最大重试次数
retryInterval time.Duration // 基础重试间隔
maxBackoff    time.Duration // 最大退避时间

// 重试间隔计算（指数退避）
wait = retryInterval * (2^attempt)
if wait > maxBackoff:
    wait = maxBackoff
```

---

## 7. 与其他系统集成

### 7.1 edgeCore Device Service 集成

驱动通过 `driver.RegisterDriver` 注册，Device Service 通过统一接口调用：

```go
func init() {
    driver.RegisterDriver("omron-fins-tcp", func() driver.Driver {
        return NewFinsTCPDriver()
    })
}
```

### 7.2 数据流向

```
┌──────────────┐      ┌──────────────┐      ┌──────────────┐
│  Omron PLC   │─────>│  FinsTCP     │─────>│  edgeCore       │
│              │      │  Driver      │      │  Core Data   │
└──────────────┘      └──────────────┘      └──────────────┘
     ^                      │                      │
     │                      │                      │
     │<─────────────────────│<─────────────────────│
     │    写入命令           │    写入请求          │    北向指令
```

### 7.3 北向数据格式

驱动上报的数据点包含以下信息：

| 字段 | 类型 | 说明 |
| :--- | :--- | :--- |
| `PointID` | string | 点位唯一标识 |
| `Value` | any | 数据值 |
| `Quality` | string | 品质（Good/Bad/Uncertain） |
| `TS` | time.Time | 时间戳 |

---

## 8. 代码结构

```
internal/driver/omron/
├── fins.go             # 驱动主模块
├── transport.go        # 传输层（TCP连接、FINS帧处理）
├── scheduler.go        # 调度器（批量读写）
├── decoder.go          # 解码器（地址解析、数据编解码）
├── protocol.go         # 协议常量和数据结构
├── transport_test.go   # 传输层测试
├── decoder_test.go     # 解码器测试
└── scheduler_test.go   # 调度器测试
```

### 8.1 文件职责说明

| 文件 | 职责 | 关键功能 |
| :--- | :--- | :--- |
| `fins.go` | 驱动入口 | 实现 Driver 接口，初始化各模块 |
| `transport.go` | 传输层 | TCP连接管理、FINS帧收发、心跳检测 |
| `scheduler.go` | 调度层 | 点位分组、批量读写、请求合并 |
| `decoder.go` | 编解码层 | 地址解析、数据编解码 |
| `protocol.go` | 协议定义 | FINS命令码、区域代码、帧结构 |

---

## 9. 安全性考虑

### 9.1 注意事项

| 风险点 | 描述 | 关联模块 |
| :--- | :--- | :--- |
| **未授权访问** | 无认证机制，任何人可连接 | transport |
| **数据篡改** | 无消息完整性校验 | decoder |
| **拒绝服务** | 恶意设备可发送大量数据 | transport |
| **地址伪造** | 区域地址无验证机制 | decoder |

### 9.2 建议措施

1. **网络层面**: 部署防火墙，限制访问IP
2. **传输加密**: 考虑使用 TLS 加密传输（需设备支持）
3. **输入验证**: 严格校验区域和地址范围
4. **流量控制**: 实现请求频率限制

---

## 10. 部署与集成

### 10.1 驱动注册

在 `cmd/driver/registry.go` 中添加导入：

```go
import (
    _ "github.com/anviod/edgeCore/internal/driver/omron"
)
```

### 10.2 配置示例

```yaml
device:
  name: "Omron-PLC-01"
  protocol: "omron-fins-tcp"
  config:
    plcModel: "CJ2M"
    plcIP: "192.168.1.100"
    plcPort: 9600
    maxFrameLength: 64
    timeout: 3000
    heartbeatInterval: 30000
    maxRetries: 3
    retryInterval: 1000
```

### 10.3 点位配置示例

```yaml
points:
  - name: "DI-001"
    address: "CIO0.0"
    dataType: "BIT"
    attribute: "Read"
  
  - name: "AI-001"
    address: "D100"
    dataType: "FLOAT"
    attribute: "Read"
  
  - name: "DO-001"
    address: "CIO100.0"
    dataType: "BIT"
    attribute: "Write"
  
  - name: "AO-001"
    address: "D200"
    dataType: "FLOAT"
    attribute: "Write"
  
  - name: "Status"
    address: "F0"
    dataType: "UINT8"
    attribute: "Read"
```

---

## 11. 测试方案

### 11.1 CP2E PLC 连接示例

本章节介绍如何使用 Omron FINS TCP 插件连接欧姆龙 CP2E PLC，实现读写 PLC 中的点位值。

Omron FINS TCP 插件可以通过本地局域网或者 Internet 连接到欧姆龙 PLC，但是需要注意的是，如果 PLC 与 edgeCore Gateway 不在同一局域网，需要在 PLC 上配置端口转发。

#### 11.1.1 前置准备

已通过欧姆龙编程软件 **CX-Programmer** 连接到 CP2E PLC，可以查看 PLC 中的点位。

#### 11.1.2 查看 PLC 点位

1. **配置以太网参数**
   - 左侧菜单栏选择 **设置**，打开 PLC 设定窗口
   - 找到 **内置以太网** 选项卡
   - 为 PLC 配置 IP 地址、子网掩码等网络参数

2. **查看数据区域**
   - 左侧菜单选择 **内存**，打开 PLC 内存窗口
   - 可以查看到 PLC 支持的数据区域以及地址范围
   - CP2E PLC 支持多个数据区域，包括 CIO、A、W、H、D、F、EM 等

#### 11.1.3 PLC 配置示例

| 配置项 | 值 | 说明 |
| :--- | :--- | :--- |
| IP 地址 | 192.168.1.10 | PLC 静态IP地址 |
| 子网掩码 | 255.255.255.0 | 子网掩码 |
| 默认网关 | 192.168.1.1 | 默认网关 |
| FINS 端口 | 9600 | FINS TCP 默认端口 |

#### 11.1.4 edgeCore 驱动配置

```yaml
device:
  name: "CP2E-PLC-01"
  protocol: "omron-fins-tcp"
  config:
    plcModel: "CP2E"
    plcIP: "192.168.1.10"
    plcPort: 9600
    maxFrameLength: 64
    timeout: 3000
    heartbeatInterval: 30000
    maxRetries: 3
    retryInterval: 1000
    srcNetworkAddr: 0
    srcNodeAddr: 1
    srcUnitAddr: 255
    dstNetworkAddr: 0
    dstNodeAddr: 1
    dstUnitAddr: 0

points:
  - name: "DI-001"
    address: "CIO0.0"
    dataType: "BIT"
    attribute: "Read"
  
  - name: "DI-002"
    address: "CIO0.1"
    dataType: "BIT"
    attribute: "Read"
  
  - name: "AI-001"
    address: "D100"
    dataType: "FLOAT"
    attribute: "Read"
  
  - name: "DO-001"
    address: "CIO100.0"
    dataType: "BIT"
    attribute: "Write"
  
  - name: "AO-001"
    address: "D200"
    dataType: "FLOAT"
    attribute: "Write"
  
  - name: "Work-001"
    address: "W0"
    dataType: "INT16"
    attribute: "Read"
  
  - name: "Hold-001"
    address: "H0"
    dataType: "INT16"
    attribute: "Read/Write"
```

#### 11.1.5 测试验证

| 测试项 | 验证方法 | 预期结果 |
| :--- | :--- | :--- |
| 连接测试 | 启动驱动后查看健康状态 | HealthStatusGood |
| 数字输入读取 | 观察 CIO0.0、CIO0.1 点位值 | 读取到 PLC 输入状态 |
| 模拟输入读取 | 读取 D100 浮点数值 | 读取到模拟量数值 |
| 数字输出写入 | 写入 TRUE/FALSE 到 CIO100.0 | PLC 输出点状态变化 |
| 模拟输出写入 | 写入浮点数值到 D200 | PLC 内部寄存器更新 |

#### 11.1.6 常见问题排查

| 问题现象 | 可能原因 | 解决方案 |
| :--- | :--- | :--- |
| 连接失败 | IP地址错误 | 检查 PLC IP 地址配置 |
| 连接失败 | 端口被占用 | 检查 9600 端口是否被其他程序占用 |
| 连接失败 | 网络不通 | 检查网络连接，确保 PLC 和 edgeCore 在同一网段 |
| 读取失败 | 地址不存在 | 检查点位地址是否在 PLC 支持范围内 |
| 写入失败 | 区域只读 | 确认点位所在区域支持写入操作 |
| 写入失败 | 权限不足 | 检查 PLC 权限设置 |

#### 11.1.7 CP2E 支持的数据区域

| 区域 | 类型 | 属性 | 地址范围 |
| :--- | :--- | :--- | :--- |
| CIO | 输入输出继电器 | 读/写 | CIO0 ~ CIO6143 |
| W | 工作继电器 | 读/写 | W0 ~ W511 |
| H | 保持继电器 | 读/写 | H0 ~ H511 |
| D | 数据寄存器 | 读/写 | D0 ~ D32767 |
| A | 辅助继电器 | 只读 | A0 ~ A511 |
| F | 标志继电器 | 只读 | F0 ~ F255 |
| EM | 扩展内存 | 读/写 | EM0 ~ EM15 |

---

## 附录：FINS 命令码

| 主命令 | 子命令 | 功能 |
| :--- | :--- | :--- |
| 0x01 | 0x01 | 读取内存区域 |
| 0x01 | 0x02 | 写入内存区域 |
| 0x02 | 0x01 | 读取多个区域 |
| 0x02 | 0x02 | 写入多个区域 |
| 0x03 | 0x01 | 批量读取 |
| 0x03 | 0x02 | 批量写入 |
| 0x04 | 0x01 | 读取位 |
| 0x04 | 0x02 | 写入位 |
| 0x06 | 0x01 | 连接测试 |
| 0x07 | 0x01 | 控制器状态读取 |
| 0x08 | 0x01 | 错误清除 |
