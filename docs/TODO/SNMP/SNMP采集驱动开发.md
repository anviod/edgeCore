
# SNMP 采集驱动开发方案

> **实现状态（2026-06-27）**：✅ **v2c / v3 已交付** — 后端 `internal/driver/snmp/`，协议 ID `snmp`，前端通道配置与帮助已注册。基于纯 Go 库 `github.com/gosnmp/gosnmp`（CGO_ENABLED=0）。

| 能力 | v2c | v3 | 说明 |
|------|-----|-----|------|
| GET 单 OID | ✅ | ✅ | 单点或批量 GET |
| GETBULK | ✅ | ✅ | 同 community/用户分组批量读 |
| GETNEXT / WALK | ✅ | ✅ | `ScanObjects` MIB 扫描 |
| SET 写 OID | ✅ | ✅ | `WritePoint` |
| Community 认证 | ✅ | — | 点位地址 `community\|OID` |
| USM 认证 | — | ✅ | MD5/SHA/SHA224/SHA256/SHA384/SHA512 |
| 加密 | — | ✅ | DES/AES128/AES192/AES256 |
| 安全级别 | — | ✅ | noAuthNoPriv / authNoPriv / authPriv |
| contextName / contextEngineID | — | ✅ | 可选 |
| 超时 / 重试 / 发送间隔 | ✅ | ✅ | timeout、retries、sendInterval |
| ScanEngine 集成 | ✅ | ✅ | ReadPoints / WritePoint，无独立采集 goroutine |
| 设备扫描 | ✅ | ✅ | ObjectScanner.ScanObjects |

**通道配置 JSON 示例（v2c）**：

```json
{
  "ip": "192.168.1.1",
  "port": 161,
  "snmpVersion": "v2c",
  "community": "public",
  "timeout": 3000,
  "retries": 3,
  "maxBulkSize": 10,
  "sendInterval": 100
}
```

**通道配置 JSON 示例（v3）**：

```json
{
  "ip": "192.168.1.1",
  "port": 161,
  "snmpVersion": "v3",
  "securityName": "admin",
  "securityLevel": "authPriv",
  "authProtocol": "SHA256",
  "authPassword": "AuthPass123",
  "privProtocol": "AES128",
  "privPassword": "PrivPass123",
  "timeout": 3000,
  "retries": 3
}
```

---

## 1. 概述

### 1.1 协议简介

SNMP（Simple Network Management Protocol，简单网络管理协议）是一种用于网络设备管理的标准协议。通过 SNMP，网络管理员可以监控和管理网络设备的状态和性能，如路由器、交换机、服务器等。

本驱动同时支持 SNMP v2c 和 SNMP v3 版本：

**SNMP v2c**（社区字符串认证）:
- **GET**: 获取单个 OID 的值
- **GETNEXT**: 获取下一个 OID 的值
- **GETBULK**: 批量获取多个 OID 的值
- **SET**: 设置 OID 的值

**SNMP v3**（用户认证和加密）:
- 支持认证协议：MD5、SHA-1、SHA-224、SHA-256、SHA-384、SHA-512
- 支持加密协议：DES、AES-128、AES-192、AES-256
- 提供更强的安全性：身份认证、数据完整性、数据加密

### 1.2 功能定位

| 功能类别 | 功能描述 | 支持范围 |
| :--- | :--- | :--- |
| 数据采集 | 读取单个 OID 值 | GET 操作 |
| 数据采集 | 读取多个 OID 值 | GETBULK 操作 |
| 数据采集 | 遍历 MIB 树 | GETNEXT 操作 |
| 数据写入 | 设置 OID 值 | SET 操作 |
| 设备发现 | 扫描网络中的 SNMP 设备 | 支持 |
| 安全认证 | 用户认证和数据加密 | SNMP v3 |

### 1.3 设计原则

- **一致性**: 遵循项目统一的驱动接口规范，与 S7、EtherNet/IP 等驱动保持一致的设计风格
- **安全性**: SNMP v3 支持认证和加密，提供企业级安全保障
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
│   │                    SNMPDriver                           │   │
│   │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────┐  │   │
│   │  │ transport│◄─│ scheduler│◄─│ decoder  │  │ config │  │   │
│   │  │ (UDP/TCP) │  │ (读写调度)│  │ (编解码) │  │        │  │   │
│   │  └────┬─────┘  └──────────┘  └──────────┘  └────────┘  │   │
│   │       │                                                 │   │
│   └───────┼─────────────────────────────────────────────────┘   │
├───────────┼──────────────────────────────────────────────────────┤
│                    Network Layer                                │
│                        UDP/TCP                                 │
└───────────┴──────────────────────────────────────────────────────┘
```

### 2.2 模块划分

| 模块 | 文件 | 职责 | 对应参考 |
| :--- | :--- | :--- | :--- |
| **驱动主模块** | `snmp.go` | 实现 Driver 接口，协调整体流程 | `s7.go`, `ethernetip.go` |
| **传输层** | `transport.go` | UDP/TCP连接管理、SNMP 消息收发 | `transport.go` |
| **调度器** | `scheduler.go` | 批量点位读写调度、请求分组 | `scheduler.go` |
| **解码器** | `decoder.go` | OID 解析、数据编解码、PDU 处理 | `decoder.go` |

### 2.3 核心类/结构体设计

#### 2.3.1 SNMPDriver（驱动主类）

```go
type SNMPDriver struct {
    config    model.DriverConfig
    transport *SNMPTransport
    decoder   *SNMPDecoder
    scheduler *SNMPScheduler
}
```

#### 2.3.2 SNMPTransport（传输层）

```go
type SNMPTransport struct {
    cfg               map[string]any
    conn              net.Conn
    connected         atomic.Bool
    connectTime       time.Time
    lastDisconnectTime time.Time
    reconnectCount    atomic.Int32
    localAddr         string
    remoteAddr        string
    
    // SNMP 版本
    snmpVersion       SNMPVersion
    
    // 通用配置参数
    targetIP          string
    targetPort        int
    localPort         int
    timeout           time.Duration
    retries           int
    
    // SNMP v2c 配置
    community         string
    
    // SNMP v3 配置
    securityName      string        // 安全名称（用户名）
    securityLevel     SecurityLevel // 安全级别
    authProtocol      AuthProtocol  // 认证协议
    authPassword      string        // 认证密码
    privProtocol      PrivProtocol  // 加密协议
    privPassword      string        // 加密密码
    contextName       string        // 上下文名称
    contextEngineID   string        // 上下文引擎ID
}

type SNMPVersion int
const (
    SNMPVersionV2C SNMPVersion = iota
    SNMPVersionV3
)

type SecurityLevel int
const (
    SecurityLevelNoAuthNoPriv SecurityLevel = iota // 无认证无加密
    SecurityLevelAuthNoPriv                         // 有认证无加密
    SecurityLevelAuthPriv                           // 有认证有加密
)

type AuthProtocol int
const (
    AuthProtocolNone AuthProtocol = iota
    AuthProtocolMD5
    AuthProtocolSHA1
    AuthProtocolSHA224
    AuthProtocolSHA256
    AuthProtocolSHA384
    AuthProtocolSHA512
)

type PrivProtocol int
const (
    PrivProtocolNone PrivProtocol = iota
    PrivProtocolDES
    PrivProtocolAES128
    PrivProtocolAES192
    PrivProtocolAES256
)
```

#### 2.3.3 SNMPScheduler（调度器）

```go
type SNMPScheduler struct {
    transport     *SNMPTransport
    decoder       *SNMPDecoder
    
    // 配置
    maxBulkSize   int           // GETBULK 最大数量
    minInterval   time.Duration // 指令最小间隔
    
    // 统计
    totalRequests int64
    successCount  int64
    failureCount  int64
    mu            sync.Mutex
}
```

#### 2.3.4 SNMPDecoder（解码器）

```go
type SNMPDecoder struct {
    // MIB 信息缓存
    mibCache    map[string]MIBInfo
}

type MIBInfo struct {
    OID         string
    Name        string
    Syntax      string
    Access      string
    Description string
}

type Address struct {
    Community      string // 社区字符串
    OID            string // 对象标识符
    Instance       string // 实例后缀（可选）
}
```

---

## 3. 接口定义

### 3.1 驱动接口（实现 `driver.Driver`）

| 方法 | 功能 | 参数 | 返回值 |
| :--- | :--- | :--- | :--- |
| `Init(cfg model.DriverConfig) error` | 初始化驱动 | `cfg`: 驱动配置 | `error`: 错误信息 |
| `Connect(ctx context.Context) error` | 建立连接 | `ctx`: 上下文 | `error`: 错误信息 |
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
| `SendPDU(pdu *SNMPPDU) (*SNMPPDU, error)` | 发送 SNMP PDU 并等待响应 |
| `Get(oid string) (*SNMPVarBind, error)` | 获取单个 OID 值 |
| `GetBulk(oids []string, maxRepetitions int) ([]SNMPVarBind, error)` | 批量获取多个 OID 值 |
| `GetNext(oid string) (*SNMPVarBind, error)` | 获取下一个 OID 值 |
| `Set(oid string, value interface{}, dataType model.DataType) error` | 设置 OID 值 |

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
| `DecodePDU(data []byte) (*SNMPPDU, error)` | 解码 SNMP PDU |
| `EncodePDU(pdu *SNMPPDU) ([]byte, error)` | 编码 SNMP PDU |
| `DecodeValue(vb *SNMPVarBind, dataType model.DataType) (any, error)` | 解码变量绑定值 |
| `EncodeValue(value any, dataType model.DataType) (*SNMPVarBind, error)` | 编码值为变量绑定 |

---

## 4. 配置参数

### 4.1 设备配置项

| 参数名 | 类型 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| `snmpVersion` | string | "v2c" | SNMP 版本："v2c" 或 "v3" |
| `targetIP` | string | - | 目标设备的 IP 地址（必填） |
| `localPort` | int | 0 | 本地端口号，0 表示自动分配 |
| `targetPort` | int | 161 | 目标设备的 SNMP 端口号 |
| `timeout` | int | 3000 | 响应超时时间（毫秒） |
| `retries` | int | 3 | 重试次数 |
| `maxBulkSize` | int | 10 | GETBULK 最大请求数量 |
| `sendInterval` | int | 100 | 指令发送间隔（毫秒） |

#### 4.1.1 SNMP v2c 配置项

| 参数名 | 类型 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| `community` | string | "public" | SNMP 社区字符串 |

#### 4.1.2 SNMP v3 配置项

| 参数名 | 类型 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| `securityName` | string | - | 安全名称（用户名，必填） |
| `securityLevel` | string | "authPriv" | 安全级别："noAuthNoPriv"、"authNoPriv"、"authPriv" |
| `authProtocol` | string | "SHA256" | 认证协议："MD5"、"SHA1"、"SHA224"、"SHA256"、"SHA384"、"SHA512" |
| `authPassword` | string | - | 认证密码（安全级别为 authNoPriv 或 authPriv 时必填） |
| `privProtocol` | string | "AES128" | 加密协议："DES"、"AES128"、"AES192"、"AES256" |
| `privPassword` | string | - | 加密密码（安全级别为 authPriv 时必填） |
| `contextName` | string | "" | 上下文名称（可选） |
| `contextEngineID` | string | "" | 上下文引擎ID（可选） |

### 4.2 点位配置

#### 4.2.1 地址格式

**SNMP v2c**:
```
community|object_identifier[.instance]
```

**SNMP v3**:
```
securityName|object_identifier[.instance]
```

- `community`: SNMP 社区字符串（必填）
- `object_identifier`: 对象标识符（OID），由点分隔的数字序列（必填）
- `instance`: 实例后缀（可选）

#### 4.2.2 支持的数据类型

| 数据类型 | 说明 | SNMP ASN.1 类型 |
| :--- | :--- | :--- |
| BIT | 单个位 | INTEGER (0/1) |
| BOOL | 布尔值 | INTEGER (0/1) |
| UINT8 | 无符号8位整数 | INTEGER |
| INT8 | 有符号8位整数 | INTEGER |
| UINT16 | 无符号16位整数 | INTEGER |
| INT16 | 有符号16位整数 | INTEGER |
| UINT32 | 无符号32位整数 | INTEGER |
| INT32 | 有符号32位整数 | INTEGER |
| UINT64 | 无符号64位整数 | Counter64 |
| INT64 | 有符号64位整数 | INTEGER |
| FLOAT | 单精度浮点数 | OCTET STRING (解析) |
| DOUBLE | 双精度浮点数 | OCTET STRING (解析) |
| STRING | 字符串 | OCTET STRING |
| BYTES | 字节数组 | OCTET STRING |

#### 4.2.3 标准 OID 示例

| OID | 说明 | 数据类型 |
| :--- | :--- | :--- |
| 1.3.6.1.2.1.1.1 | 系统描述 | STRING |
| 1.3.6.1.2.1.1.2 | 系统对象ID | STRING |
| 1.3.6.1.2.1.1.3 | 系统运行时间 | UINT32 |
| 1.3.6.1.2.1.1.4 | 系统联系人 | STRING |
| 1.3.6.1.2.1.1.5 | 系统名称 | STRING |
| 1.3.6.1.2.1.1.6 | 系统位置 | STRING |
| 1.3.6.1.2.1.2.1 | 接口数量 | UINT32 |
| 1.3.6.1.2.1.2.2.1.2 | 接口名称 | STRING |
| 1.3.6.1.2.1.2.2.1.3 | 接口类型 | UINT32 |
| 1.3.6.1.2.1.2.2.1.10 | 入站字节数 | COUNTER64 |
| 1.3.6.1.2.1.2.2.1.16 | 出站字节数 | COUNTER64 |

#### 4.2.4 地址示例

**SNMP v2c**:

| 地址 | 数据类型 | 说明 |
| :--- | :--- | :--- |
| public\|1.3.6.1.2.1.1.1 | STRING | 系统描述 |
| public\|1.3.6.1.2.1.1.5 | STRING | 系统名称 |
| private\|1.3.6.1.2.1.1.3 | UINT32 | 系统运行时间 |
| public\|1.3.6.1.2.1.2.2.1.10.1 | UINT64 | 接口1入站字节数 |
| public\|1.3.6.1.2.1.2.2.1.16.2 | UINT64 | 接口2出站字节数 |

**SNMP v3**:

| 地址 | 数据类型 | 说明 |
| :--- | :--- | :--- |
| admin\|1.3.6.1.2.1.1.1 | STRING | 系统描述（安全名称为 admin） |
| admin\|1.3.6.1.2.1.1.5 | STRING | 系统名称（安全名称为 admin） |
| snmpuser\|1.3.6.1.2.1.1.3 | UINT32 | 系统运行时间（安全名称为 snmpuser） |
| admin\|1.3.6.1.2.1.2.2.1.10.1 | UINT64 | 接口1入站字节数 |
| admin\|1.3.6.1.2.1.2.2.1.16.2 | UINT64 | 接口2出站字节数 |

---

## 5. 数据处理流程

### 5.1 连接建立流程

```
edgeCore                          SNMP Agent
  |                               |
  |--- UDP Socket -------------->|  建立 UDP socket
  |                               |
  |--- SNMP GET Request -------->|  发送请求 PDU
  |                               |
  |<-- SNMP Response ------------|  接收响应 PDU
```

### 5.2 SNMP PDU 结构

```
┌─────────────────────────────────────────────────────────┐
│ SNMP Message                                          │
├──────────────────┬────────────────────────────────────┤
│ Version(1)       │ Community(n)  │ PDU(variable)      │
│                  │                │                    │
├──────────────────┼────────────────┼────────────────────┤
│ 0x01 (v1)       │ "public"      │ GetRequest PDU     │
│ 0x02 (v2c)      │               │ GetNextRequest PDU │
│                  │               │ GetBulkRequest PDU │
│                  │               │ SetRequest PDU     │
│                  │               │ Response PDU       │
└──────────────────┴────────────────┴────────────────────┘

PDU Structure:
┌────────┬────────┬──────────────┬─────────────────┐
│ Type   │ Request│ Error Status │ Error Index     │
│ (1)    │ ID (4) │ (1)          │ (1)             │
├────────┼────────┼──────────────┼─────────────────┤
│ VarBindList (variable)                              │
│ ┌───────────────────────────────────────────────┐   │
│ │ OID Length │ OID Bytes │ Value Type │ Value  │   │
│ └───────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────┘
```

### 5.3 点位读取流程

```
┌─────────────────────────────────────────────────────────────┐
│                    采集组定时触发                           │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  Scheduler: 按社区字符串分组点位                           │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  Transport: 发送 GET/GETBULK 请求                        │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  Transport: 接收响应，解析 PDU                            │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  Decoder: 解码 VarBind，转换为 model.Value                 │
└──────────────────────────┬──────────────────────────────────┘
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  返回结果到 Device Service                                  │
└─────────────────────────────────────────────────────────────┘
```

---

## 6. 错误处理机制

### 6.1 错误分类

| 错误类型 | 触发条件 | 处理策略 |
| :--- | :--- | :--- |
| **连接错误** | UDP socket 创建失败 | 记录日志，尝试重新创建 |
| **超时错误** | 响应超时 | 重试或标记失败 |
| **协议错误** | PDU 格式错误、版本不支持 | 返回错误，不发送请求 |
| **认证错误** | 社区字符串错误 | 返回错误，标记点位质量为Bad |
| **设备错误** | 设备返回错误状态 | 记录日志，标记点位质量为Bad |

### 6.2 SNMP 错误状态码

| 错误码 | 说明 |
| :--- | :--- |
| 0 | noError | 无错误 |
| 1 | tooBig | 响应过大 |
| 2 | noSuchName | OID 不存在 |
| 3 | badValue | 值无效 |
| 4 | readOnly | 只读 |
| 5 | genErr | 通用错误 |

### 6.3 重试机制

```go
// 重试策略
retries        int           // 重试次数
timeout        time.Duration // 超时时间
sendInterval   time.Duration // 发送间隔

// 重试流程
for attempt := 0; attempt <= retries; attempt++ {
    sendRequest()
    select {
    case <-response:
        return success
    case <-timeout:
        continue
    }
}
return error
```

---

## 7. 与其他系统集成

### 7.1 edgeCore Device Service 集成

驱动通过 `driver.RegisterDriver` 注册，Device Service 通过统一接口调用：

```go
func init() {
    driver.RegisterDriver("snmp", func() driver.Driver {
        return NewSNMPDriver()
    })
}
```

### 7.2 数据流向

```
┌──────────────┐      ┌──────────────┐      ┌──────────────┐
│  SNMP Agent  │─────>│  SNMP        │─────>│  edgeCore       │
│  (Router/    │      │  Driver      │      │  Core Data   │
│   Switch)    │      │              │      │              │
└──────────────┘      └──────────────┘      └──────────────┘
     ^                      │                      │
     │                      │                      │
     │<─────────────────────│<─────────────────────│
     │    SET 请求           │    写入请求          │    北向指令
```

### 7.3 北向数据格式

驱动上报的数据点包含以下信息：

| 字段 | 类型 | 说明 |
| :--- | :--- | :--- |
| `PointID` | string | 点位唯一标识 |
| `Value` | any | 数据值 |
| `Quality` | string | 品质（Good/Bad/Uncertain） |
| `TS` | time.Time | 时间戳 |
| `OID` | string | 原始 OID（可选） |

---

## 8. 代码结构

```
internal/driver/snmp/
├── snmp.go            # 驱动主模块
├── transport.go       # 传输层（UDP连接、SNMP消息处理）
├── scheduler.go       # 调度器（批量读写）
├── decoder.go         # 解码器（OID解析、数据编解码）
├── protocol.go        # 协议常量和数据结构
├── transport_test.go  # 传输层测试
├── decoder_test.go    # 解码器测试
└── scheduler_test.go  # 调度器测试
```

### 8.1 文件职责说明

| 文件 | 职责 | 关键功能 |
| :--- | :--- | :--- |
| `snmp.go` | 驱动入口 | 实现 Driver 接口，初始化各模块 |
| `transport.go` | 传输层 | UDP socket 管理、SNMP PDU 收发 |
| `scheduler.go` | 调度层 | 点位分组、批量请求合并 |
| `decoder.go` | 编解码层 | OID 解析、ASN.1 BER 编解码 |
| `protocol.go` | 协议定义 | SNMP 版本、PDU 类型、错误码 |

---

## 9. 安全性考虑

### 9.1 注意事项

| 风险点 | 描述 | 关联模块 |
| :--- | :--- | :--- |
| **未授权访问** | SNMP v2c 使用明文社区字符串认证 | transport |
| **数据篡改** | SNMP v2c 无消息完整性校验 | decoder |
| **拒绝服务** | 恶意设备可发送大量数据 | transport |
| **OID 枚举** | 攻击者可枚举所有 OID | decoder |

### 9.2 建议措施

1. **限制访问**: 部署防火墙，限制 SNMP 端口访问
2. **使用安全社区字符串**: 避免使用默认的 "public"
3. **考虑升级到 v3**: SNMP v3 支持认证和加密
4. **输入验证**: 严格校验 OID 格式

---

## 10. 部署与集成

### 10.1 驱动注册

在 `cmd/driver/registry.go` 中添加导入：

```go
import (
    _ "github.com/anviod/edgeCore/internal/driver/snmp"
)
```

### 10.2 配置示例

**SNMP v2c 配置**:

```yaml
device:
  name: "SNMP-v2c-Device-01"
  protocol: "snmp"
  config:
    snmpVersion: "v2c"
    targetIP: "192.168.1.1"
    localPort: 0
    targetPort: 161
    community: "public"
    timeout: 3000
    retries: 3
    maxBulkSize: 10
    sendInterval: 100
```

**SNMP v3 配置**:

```yaml
device:
  name: "SNMP-v3-Device-01"
  protocol: "snmp"
  config:
    snmpVersion: "v3"
    targetIP: "192.168.1.1"
    localPort: 0
    targetPort: 161
    timeout: 3000
    retries: 3
    maxBulkSize: 10
    sendInterval: 100
    securityName: "admin"
    securityLevel: "authPriv"
    authProtocol: "SHA256"
    authPassword: "AuthPass123"
    privProtocol: "AES128"
    privPassword: "PrivPass123"
    contextName: ""
    contextEngineID: ""
```

### 10.3 点位配置示例

**SNMP v2c 点位配置**:

```yaml
points:
  - name: "System-Description"
    address: "public|1.3.6.1.2.1.1.1"
    dataType: "STRING"
    attribute: "Read"
  
  - name: "System-Name"
    address: "public|1.3.6.1.2.1.1.5"
    dataType: "STRING"
    attribute: "Read"
  
  - name: "System-Uptime"
    address: "public|1.3.6.1.2.1.1.3"
    dataType: "UINT32"
    attribute: "Read"
  
  - name: "Interface-Count"
    address: "public|1.3.6.1.2.1.2.1"
    dataType: "UINT32"
    attribute: "Read"
  
  - name: "IfInOctets-1"
    address: "public|1.3.6.1.2.1.2.2.1.10.1"
    dataType: "UINT64"
    attribute: "Read"
  
  - name: "IfOutOctets-1"
    address: "public|1.3.6.1.2.1.2.2.1.16.1"
    dataType: "UINT64"
    attribute: "Read"
```

**SNMP v3 点位配置**:

```yaml
points:
  - name: "System-Description"
    address: "admin|1.3.6.1.2.1.1.1"
    dataType: "STRING"
    attribute: "Read"
  
  - name: "System-Name"
    address: "admin|1.3.6.1.2.1.1.5"
    dataType: "STRING"
    attribute: "Read"
  
  - name: "System-Uptime"
    address: "admin|1.3.6.1.2.1.1.3"
    dataType: "UINT32"
    attribute: "Read"
  
  - name: "Interface-Count"
    address: "admin|1.3.6.1.2.1.2.1"
    dataType: "UINT32"
    attribute: "Read"
  
  - name: "IfInOctets-1"
    address: "admin|1.3.6.1.2.1.2.2.1.10.1"
    dataType: "UINT64"
    attribute: "Read"
  
  - name: "IfOutOctets-1"
    address: "admin|1.3.6.1.2.1.2.2.1.16.1"
    dataType: "UINT64"
    attribute: "Read"
```

---

## 11. 测试方案

### 11.1 网络设备连接示例

本章节介绍如何使用 SNMP 插件连接网络设备（如路由器、交换机），实现读取设备中的点位值。

#### 11.1.1 前置准备

1. 网络设备已配置 SNMP v2c 支持
2. 已知设备 IP 地址和社区字符串
3. 设备防火墙允许 SNMP 端口（UDP 161）访问

#### 11.1.2 设备配置

在网络设备上启用 SNMP：

```
# Cisco IOS 示例
snmp-server community public RO
snmp-server community private RW
snmp-server enable traps
```

#### 11.1.3 edgeCore 驱动配置

```yaml
device:
  name: "Router-Cisco-01"
  protocol: "snmp"
  config:
    targetIP: "192.168.1.1"
    targetPort: 161
    community: "public"
    timeout: 3000
    retries: 3
```

#### 11.1.4 测试验证

| 测试项 | 验证方法 | 预期结果 |
| :--- | :--- | :--- |
| 连接测试 | 启动驱动后查看健康状态 | HealthStatusGood |
| 系统信息读取 | 读取系统描述 OID | 成功获取设备描述 |
| 接口信息读取 | 读取接口数量 | 成功获取接口数 |
| 流量统计读取 | 读取入站/出站字节数 | 成功获取流量数据 |
| SET 测试 | 设置可写 OID | 成功设置值（需权限） |

#### 11.1.5 常见问题排查

| 问题现象 | 可能原因 | 解决方案 |
| :--- | :--- | :--- |
| 连接失败 | IP地址错误 | 检查设备IP地址配置 |
| 连接失败 | 端口被防火墙阻止 | 检查设备防火墙配置 |
| 读取失败 | 社区字符串错误 | 确认社区字符串配置 |
| 读取失败 | OID不存在 | 确认OID在设备MIB中存在 |
| 读取失败 | 权限不足 | 使用具有读取权限的社区字符串 |

#### 11.1.6 支持的设备类型

| 类型 | 说明 | 支持程度 |
| :--- | :--- | :--- |
| 路由器 | Cisco、华为、H3C 等 | 完全支持 |
| 交换机 | Cisco、华为、H3C、TP-Link 等 | 完全支持 |
| 服务器 | Linux、Windows SNMP Agent | 完全支持 |
| 网络设备 | 支持 SNMP v2c 的设备 | 完全支持 |

### 11.2 SNMP v3 网络设备连接示例

本章节介绍如何使用 SNMP v3 插件连接网络设备，实现安全的数据采集。

#### 11.2.1 前置准备

1. 网络设备已配置 SNMP v3 支持
2. 已知设备 IP 地址和 SNMP v3 用户信息
3. 设备防火墙允许 SNMP 端口（UDP 161）访问

#### 11.2.2 设备配置

在网络设备上配置 SNMP v3：

```
# Cisco IOS 示例
snmp-server group snmpgroup v3 auth
snmp-server user admin snmpgroup v3 auth sha AuthPass123 priv aes 128 PrivPass123
snmp-server enable traps
```

#### 11.2.3 edgeCore 驱动配置

```yaml
device:
  name: "Router-Cisco-v3-01"
  protocol: "snmp"
  config:
    snmpVersion: "v3"
    targetIP: "192.168.1.1"
    targetPort: 161
    securityName: "admin"
    securityLevel: "authPriv"
    authProtocol: "SHA256"
    authPassword: "AuthPass123"
    privProtocol: "AES128"
    privPassword: "PrivPass123"
    timeout: 3000
    retries: 3
```

#### 11.2.4 测试验证

| 测试项 | 验证方法 | 预期结果 |
| :--- | :--- | :--- |
| 连接测试 | 启动驱动后查看健康状态 | HealthStatusGood |
| 系统信息读取 | 读取系统描述 OID | 成功获取设备描述 |
| 接口信息读取 | 读取接口数量 | 成功获取接口数 |
| 流量统计读取 | 读取入站/出站字节数 | 成功获取流量数据 |
| SET 测试 | 设置可写 OID | 成功设置值（需权限） |

#### 11.2.5 常见问题排查

| 问题现象 | 可能原因 | 解决方案 |
| :--- | :--- | :--- |
| 连接失败 | IP地址错误 | 检查设备IP地址配置 |
| 连接失败 | 端口被防火墙阻止 | 检查设备防火墙配置 |
| 读取失败 | 安全名称错误 | 确认安全名称配置 |
| 读取失败 | 认证密码错误 | 确认认证密码配置 |
| 读取失败 | 加密密码错误 | 确认加密密码配置 |
| 读取失败 | 认证协议不匹配 | 确认认证协议配置 |
| 读取失败 | 加密协议不匹配 | 确认加密协议配置 |
| 读取失败 | 安全级别不匹配 | 确认安全级别配置 |

#### 11.2.6 SNMP v3 安全级别说明

| 安全级别 | 说明 | 认证 | 加密 |
| :--- | :--- | :--- | :--- |
| noAuthNoPriv | 无认证无加密 | 否 | 否 |
| authNoPriv | 有认证无加密 | 是 | 否 |
| authPriv | 有认证有加密 | 是 | 是 |

---

## 附录：标准 OID 参考

### A.1 系统组（System Group）

| OID | 名称 | 说明 |
| :--- | :--- | :--- |
| 1.3.6.1.2.1.1.1 | sysDescr | 系统描述 |
| 1.3.6.1.2.1.1.2 | sysObjectID | 系统对象ID |
| 1.3.6.1.2.1.1.3 | sysUpTime | 系统运行时间 |
| 1.3.6.1.2.1.1.4 | sysContact | 系统联系人 |
| 1.3.6.1.2.1.1.5 | sysName | 系统名称 |
| 1.3.6.1.2.1.1.6 | sysLocation | 系统位置 |

### A.2 接口组（Interface Group）

| OID | 名称 | 说明 |
| :--- | :--- | :--- |
| 1.3.6.1.2.1.2.1 | ifNumber | 接口数量 |
| 1.3.6.1.2.1.2.2.1.1 | ifIndex | 接口索引 |
| 1.3.6.1.2.1.2.2.1.2 | ifDescr | 接口描述 |
| 1.3.6.1.2.1.2.2.1.3 | ifType | 接口类型 |
| 1.3.6.1.2.1.2.2.1.4 | ifMtu | MTU |
| 1.3.6.1.2.1.2.2.1.5 | ifSpeed | 接口速度 |
| 1.3.6.1.2.1.2.2.1.10 | ifInOctets | 入站字节数 |
| 1.3.6.1.2.1.2.2.1.16 | ifOutOctets | 出站字节数 |

### A.3 TCP/IP 组

| OID | 名称 | 说明 |
| :--- | :--- | :--- |
| 1.3.6.1.2.1.4.1 | ipForwarding | IP转发状态 |
| 1.3.6.1.2.1.4.20 | ipAddrTable | IP地址表 |
| 1.3.6.1.2.1.6.1 | tcpRtoAlgorithm | TCP重传算法 |
| 1.3.6.1.2.1.6.9 | tcpMaxConn | TCP最大连接数 |
| 1.3.6.1.2.1.7.1 | udpInDatagrams | UDP入站数据报 |
| 1.3.6.1.2.1.7.4 | udpOutDatagrams | UDP出站数据报 |
