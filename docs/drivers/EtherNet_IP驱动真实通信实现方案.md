---
layout: default
title: EtherNet/IP 驱动真实通信实现方案
description: edgeCore EtherNet/IP 驱动真实通信实现方案
---

# EtherNet/IP 驱动真实通信实现方案

> **文档状态：** 本文为 **历史开发方案归档**（驱动已于 2026 Q2 交付，`ethernet-ip` 生产就绪）。现行能力矩阵与配置项见 [驱动总览 · EtherNet/IP](index.html#ethernetip)；实现代码见 `internal/driver/ethernetip/`。

## 1. 产品概述

将edgeCore边缘网关的EtherNet/IP协议采集通道从模拟实现升级为基于go-ethernet-ip库的真实Allen-Bradley PLC通信，支持ControlLogix、CompactLogix、Micro800等系列PLC。

## 2. 核心功能

- **EtherNet/IP真实通信**: 基于github.com/anviod/ethernet-ip库实现与Allen-Bradley系列PLC的TCP通信
- **前端通道配置增强**: 支持IP地址、端口、超时时间、重试次数、心跳间隔、缓冲区大小、QoS等级、连接类型(CIP Connection Type)、CPU插槽值
- **点位读写**: 支持EtherNet/IP Tag地址格式的批量读取和单点写入
- **数据类型支持**: bool, sint, int, dint, real, string, 数组类型
- **批量读取优化**: 使用go-ethernet-ip的批量Tag读取减少网络往返
- **连接管理**: 自动重连、心跳保活、健康状态检测、连接指标统计

## 3. 技术栈

- **后端**: Go + github.com/anviod/ethernet-ip
- **前端**: Vue 3 + Arco Design (现有技术栈)
- **数据模型**: 复用现有Channel/Device/Point模型

## 3.1 Go 库评估

### 3.1.1 库对比分析

| 评估维度 | anviod/ethernet-ip | danomagnum/gologix |
| :--- | :--- | :--- |
| **Star数** | 70 | 66 |
| **提交活跃度** | 85 commits (更新至2022) | 431 commits (更新至2026) |
| **Issue处理** | 1 issue | 3 issues |
| **功能完整性** | 完整CIP协议实现 | 专注于Rockwell PLC |
| **批量读写** | 支持MultipleServicePacket | 支持批量Tag读写 |
| **文档质量** | 基本文档 | 详细文档和示例 |
| **社区支持** | 较小 | 活跃 |
| **许可证** | WTFPL | MIT |

### 3.1.2 本项目选型

**确定使用**: `github.com/anviod/ethernet-ip`

**选型理由**:
1. **通用性更强**: 完整的CIP协议实现，支持多种厂商设备
2. **批量操作支持**: 支持MultipleServicePacket批量读写
3. **协议完整性**: 覆盖EtherNet/IP整个协议栈
4. **轻量级**: 更适合资源受限的边缘网关环境

### 3.1.3 依赖配置

```go
require github.com/anviod/ethernet-ip latest
```

## 4. 实现方案

### 4.1 后端EtherNet/IP驱动架构（参照S7三层模式）

#### 4.1.1 架构设计原则

参考S7驱动的三层架构模式，EtherNet/IP驱动采用以下分层设计：

| 层级 | 职责 | 对应文件 | 参考S7实现 |
| :--- | :--- | :--- | :--- |
| **传输层** | TCP连接管理、CIP会话管理、心跳保活 | `transport.go` | `s7/transport.go` |
| **解码器** | Tag地址解析、数据类型编解码 | `decoder.go` | `s7/decoder.go` |
| **调度器** | 批量读取优化、请求分组、限流控制 | `scheduler.go` | `s7/scheduler.go` |
| **驱动入口** | 接口实现、组件组合 | `ethernetip.go` | `s7/s7.go` |

#### 4.1.2 三层框架搭建步骤

**第一步：创建目录结构**
```bash
mkdir -p internal/driver/ethernetip
touch internal/driver/ethernetip/transport.go
touch internal/driver/ethernetip/decoder.go
touch internal/driver/ethernetip/scheduler.go
touch internal/driver/ethernetip/transport_test.go
touch internal/driver/ethernetip/decoder_test.go
touch internal/driver/ethernetip/scheduler_test.go
```

**第二步：定义核心接口**
```go
// ENIPClient 接口别名，方便mock测试
type ENIPClient interface {
    Connect() error
    Disconnect() error
    ReadTag(tag string, count int) ([]byte, error)
    WriteTag(tag string, data []byte) error
    ReadTags(tags []string) (map[string][]byte, error)
    Ping() error
}
```

**第三步：建立依赖注入机制**
```go
type ENIPTransport struct {
    clientFactory func(address string, slot int) ENIPClient
    // ...
}
```

**1. 传输层 - `internal/driver/ethernetip/transport.go`**

#### 4.1.3 传输层实现方案（优先实现）

传输层是整个驱动的基础，负责管理TCP连接和CIP会话。

##### 4.1.3.1 核心职责

- 封装gologix的Client和连接管理
- 管理TCP连接生命周期：Connect/Disconnect/Reconnect
- 心跳保活：定期发送Ping验证连接存活
- 连接指标：连接时长、重连次数、本地/远程地址
- 配置解析：从DriverConfig.Config map中提取ip/port/slot/timeout等参数
- 支持CIP连接类型：CIP Connection Type

##### 4.1.3.2 配置参数清单

| 参数名 | 类型 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| ip | string | - | PLC IP地址（必填） |
| port | int | 44818 | EtherNet/IP默认端口 |
| slot | int | 0 | CPU插槽号 |
| timeout | int | 2000 | 连接超时时间(ms) |
| max_retries | int | 3 | 最大重试次数 |
| retry_interval | int | 100 | 重试间隔(ms) |
| heartbeat_interval | int | 30000 | 心跳间隔(ms) |
| buffer_size | int | 4096 | 缓冲区大小 |

##### 4.1.3.3 结构体设计

```go
// ENIPTransport EtherNet/IP传输层
type ENIPTransport struct {
    cfg    map[string]any
    client gologix.Client

    // 依赖注入（用于测试）
    clientFactory func(address string, slot int) *gologix.Client

    // 配置参数
    ip           string
    port         int
    slot         int
    timeout      time.Duration
    maxRetries   int
    retryInterval time.Duration

    // 连接状态
    connected           bool
    mu                  sync.Mutex
    connectTime         time.Time
    lastDisconnectTime  time.Time
    reconnectCount      int64
    localAddr           string
    remoteAddr          string

    // 心跳
    heartbeatInterval   time.Duration
    heartbeatTicker     *time.Ticker
    stopHeartbeat       chan struct{}

    // 会话健康
    lastActivityTime    time.Time
    heartbeatFailCount  int32
    heartbeatFailMax    int32
}
```

##### 4.1.3.4 核心方法实现

**构造函数**:
```go
func NewENIPTransport(cfg map[string]any) *ENIPTransport {
    t := &ENIPTransport{
        cfg:                 cfg,
        port:                44818,
        slot:                0,
        timeout:             2 * time.Second,
        maxRetries:          3,
        retryInterval:       100 * time.Millisecond,
        heartbeatInterval:   30 * time.Second,
        heartbeatFailMax:    3,
        stopHeartbeat:       make(chan struct{}),
    }
    
    // 设置默认工厂函数
    t.clientFactory = func(address string, slot int) *gologix.Client {
        client := gologix.NewClient(address)
        client.Slot = slot
        return client
    }
    
    // 解析配置
    t.parseConfig()
    
    return t
}
```

**配置解析**:
```go
func (t *ENIPTransport) parseConfig() {
    // IP地址（必填）
    if v, ok := t.cfg["ip"].(string); ok {
        t.ip = v
    }
    
    // 端口
    if v, ok := t.cfg["port"].(float64); ok {
        t.port = int(v)
    } else if v, ok := t.cfg["port"].(int); ok {
        t.port = v
    }
    
    // 插槽号
    if v, ok := t.cfg["slot"].(float64); ok {
        t.slot = int(v)
    } else if v, ok := t.cfg["slot"].(int); ok {
        t.slot = v
    }
    
    // 超时时间
    if v, ok := t.cfg["timeout"].(float64); ok {
        t.timeout = time.Duration(v) * time.Millisecond
    }
    
    // 重试次数
    if v, ok := t.cfg["max_retries"].(float64); ok {
        t.maxRetries = int(v)
    }
    
    // 心跳间隔
    if v, ok := t.cfg["heartbeat_interval"].(float64); ok {
        t.heartbeatInterval = time.Duration(v) * time.Millisecond
    }
}
```

**连接管理**:
```go
func (t *ENIPTransport) Connect(ctx context.Context) error {
    t.mu.Lock()
    defer t.mu.Unlock()
    
    if t.connected {
        return nil
    }
    
    address := fmt.Sprintf("%s:%d", t.ip, t.port)
    t.client = t.clientFactory(address, t.slot)
    
    var err error
    for i := 0; i < t.maxRetries; i++ {
        err = t.client.Connect()
        if err == nil {
            break
        }
        
        if i < t.maxRetries-1 {
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(t.retryInterval):
            }
        }
    }
    
    if err != nil {
        return fmt.Errorf("ENIP connection failed after %d retries: %w", t.maxRetries, err)
    }
    
    t.connected = true
    t.connectTime = time.Now()
    t.remoteAddr = address
    t.reconnectCount++
    
    // 启动心跳
    t.startHeartbeat()
    
    return nil
}

func (t *ENIPTransport) Disconnect() error {
    t.mu.Lock()
    defer t.mu.Unlock()
    
    if !t.connected || t.client == nil {
        return nil
    }
    
    // 停止心跳
    t.stopHeartbeat <- struct{}{}
    
    err := t.client.Disconnect()
    t.connected = false
    t.lastDisconnectTime = time.Now()
    
    return err
}
```

**心跳机制**:
```go
func (t *ENIPTransport) startHeartbeat() {
    t.heartbeatTicker = time.NewTicker(t.heartbeatInterval)
    
    go func() {
        for {
            select {
            case <-t.stopHeartbeat:
                t.heartbeatTicker.Stop()
                return
            case <-t.heartbeatTicker.C:
                t.mu.Lock()
                if t.connected && t.client != nil {
                    if err := t.client.Ping(); err != nil {
                        atomic.AddInt32(&t.heartbeatFailCount, 1)
                        if atomic.LoadInt32(&t.heartbeatFailCount) >= t.heartbeatFailMax {
                            // 心跳失败次数达到上限，触发重连
                            go t.reconnect()
                        }
                    } else {
                        atomic.StoreInt32(&t.heartbeatFailCount, 0)
                    }
                }
                t.mu.Unlock()
            }
        }
    }()
}

func (t *ENIPTransport) reconnect() {
    t.mu.Lock()
    if !t.connected {
        t.mu.Unlock()
        return
    }
    
    _ = t.client.Disconnect()
    t.mu.Unlock()
    
    ctx, cancel := context.WithTimeout(context.Background(), t.timeout)
    defer cancel()
    
    for i := 0; i < t.maxRetries; i++ {
        if err := t.Connect(ctx); err == nil {
            return
        }
        
        time.Sleep(t.retryInterval * time.Duration(i+1))
    }
}
```

**连接指标**:
```go
func (t *ENIPTransport) GetConnectionMetrics() (
    connectionSeconds int64,
    reconnectCount int64,
    localAddr string,
    remoteAddr string,
    lastDisconnectTime time.Time,
) {
    t.mu.Lock()
    defer t.mu.Unlock()
    
    if t.connectTime.IsZero() {
        return 0, 0, "", "", time.Time{}
    }
    
    connectionSeconds = int64(time.Since(t.connectTime).Seconds())
    reconnectCount = t.reconnectCount
    localAddr = t.localAddr
    remoteAddr = t.remoteAddr
    lastDisconnectTime = t.lastDisconnectTime
    
    return
}

func (t *ENIPTransport) IsConnected() bool {
    t.mu.Lock()
    defer t.mu.Unlock()
    return t.connected
}
```

##### 4.1.3.5 依赖注入测试模式

为了支持单元测试，采用依赖注入模式：

```go
// MockENIPClient 用于测试的mock客户端
type MockENIPClient struct {
    connectFunc    func() error
    disconnectFunc func() error
    readTagFunc    func(tag string, count int) ([]byte, error)
    writeTagFunc   func(tag string, data []byte) error
    pingFunc       func() error
}

func (m *MockENIPClient) Connect() error {
    if m.connectFunc != nil {
        return m.connectFunc()
    }
    return nil
}

func (m *MockENIPClient) Disconnect() error {
    if m.disconnectFunc != nil {
        return m.disconnectFunc()
    }
    return nil
}

func (m *MockENIPClient) ReadTag(tag string, count int) ([]byte, error) {
    if m.readTagFunc != nil {
        return m.readTagFunc(tag, count)
    }
    return nil, nil
}

func (m *MockENIPClient) WriteTag(tag string, data []byte) error {
    if m.writeTagFunc != nil {
        return m.writeTagFunc(tag, data)
    }
    return nil
}

func (m *MockENIPClient) Ping() error {
    if m.pingFunc != nil {
        return m.pingFunc()
    }
    return nil
}
```

在测试中使用mock：

```go
func TestENIPTransport_Connect(t *testing.T) {
    mockClient := &MockENIPClient{
        connectFunc: func() error { return nil },
    }
    
    transport := NewENIPTransport(map[string]any{
        "ip":   "192.168.1.10",
        "port": 44818,
    })
    
    // 注入mock客户端
    transport.clientFactory = func(address string, slot int) *gologix.Client {
        // 返回mock包装的客户端
        return mockClient
    }
    
    err := transport.Connect(context.Background())
    assert.NoError(t, err)
    assert.True(t, transport.IsConnected())
}
```

**2. 解码器 - `internal/driver/ethernetip/decoder.go`**

- EtherNet/IP Tag地址解析：支持格式 `tag_name`（简单标签）、`tag_name[index]`（数组元素）、`tag_name.field`（结构体字段）
- 地址结构：TagName + ElementSize + ArrayIndex + Path
- 数据编解码：使用gologix的数据类型进行字节序转换
- 寄存器计数：根据DataType确定需要读取的字节数

**3. 调度器 - `internal/driver/ethernetip/scheduler.go`**

- 按Tag名称对点位分组
- 使用gologix的批量读取API，减少PDU往返
- 适配EtherNet/IP CMU (Connection Manager)限制，自动拆分大数据块
- 指令间隔控制，避免PLC过载

**4. 驱动主入口 - `internal/driver/ethernetip/ethernetip.go`（重构）**

- 组合transport/decoder/scheduler三层
- 实现Driver接口所有方法
- Init时根据配置创建三层组件
- Connect时建立真实TCP连接
- ReadPoints通过scheduler批量读取
- WritePoint通过单点写入
- Health基于真实连接状态判断

### 4.2 EtherNet/IP地址格式

```
// 简单标签
Motor_Speed          -> TagName="Motor_Speed"

// 数组元素
Temperatures[0]      -> TagName="Temperatures", Index=0

// 数组范围
Temperatures[0..9]   -> TagName="Temperatures", StartIndex=0, EndIndex=9

// 结构体字段
Controller.Scanner   -> TagName="Controller.Scanner"

// 位域访问 (部分数据类型支持)
Motor_Status.0       -> TagName="Motor_Status", BitPosition=0
```

### 4.3 支持的PLC类型与默认参数

| PLC类型 | 默认端口 | 默认插槽 | 连接类型 |
| :--- | :--- | :--- | :--- |
| ControlLogix | 44818 | 0 | CIP Direct |
| CompactLogix | 44818 | 0 | CIP Direct |
| Micro800 | 44818 | 0 | CIP Direct |
| SLC500 | 44818 | N/A | Backplane |
| PLC-5 | 44818 | N/A | Backplane |

### 4.4 数据类型映射

| Point Type | CIP Data Type | Size (bytes) |
| :--- | :--- | :--- |
| bool | BOOL | 1 |
| sint | SINT | 1 |
| int | INT | 2 |
| dint | DINT | 4 |
| real | REAL | 4 |
| string | STRING | 88 |

### 4.5 批量读取策略

- 按Tag名称分组点位
- 同一组内使用go-ethernet-ip的ReadTags批量读取
- 自动构建Tag列表，包含TagName和ElementSize
- 读取后逐项检查Status字段并解析Data缓冲区
- 超过最大连接数时自动分批处理
- 支持配置batch_read_max限制单次最大读取点数（默认50）

## 5. 前端配置增强

**ChannelList.vue EtherNet/IP配置区域扩展**

在现有IP/端口/插槽基础上增加：

- 超时时间(timeout): 默认2000ms
- 重试次数(max_retries): 默认1
- 心跳间隔(heartbeat_interval): 默认30000ms
- 缓冲区大小(buffer_size): 默认4096字节
- QoS等级(qos): 默认1
- 连接超时(connect_timeout): 毫秒
- 连接类型(connection_type): Direct/Backplane下拉选择
- 批量读取最大值(batch_read_max): 默认50

## 6. go.mod依赖

新增: `github.com/anviod/ethernet-ip`

## 7. 实现细节

### 7.1 CIP协议基础知识

**EtherNet/IP会话建立流程**：
1. TCP连接到44818端口
2. 发送RegisterSession请求
3. 接收Session ID
4. 使用Session ID进行后续通信
5. 通信结束时发送UnregisterSession

**CIP通用指令**：
- ReadTag - 读取单个标签
- ReadTagFragmented - 读取大标签（超过连接缓冲区）
- WriteTag - 写入单个标签
- WriteTagFragmented - 写入大标签
- MultipleServicePacket - 批量读写

### 7.2 go-ethernet-ip库结构


```
ethernet-ip/
├── bufferx/              # 字节缓冲区操作
│   ├── bufferx.go        # 支持小端/大端读写、缓冲区池化
│   ├── bufferx_test.go   # 单元测试
│   └── bufferx_benchmark_test.go # 性能测试
├── command/              # EIP 命令定义
│   ├── command.go        # 命令常量（注册会话、发送数据等）
│   └── command_test.go   # 单元测试
├── messages/             # 消息处理
│   ├── packet/          # 数据包编解码
│   │   ├── packet.go    # 数据包结构
│   │   ├── commonPacketFormat.go # CPF 格式
│   │   ├── messageRouter.go      # 消息路由器
│   │   ├── services.go  # 服务定义
│   │   ├── cmm.go       # CIP 消息管理
│   │   ├── ucmm.go      # 非连接消息管理
│   │   ├── data.go      # 数据项处理
│   │   ├── utils.go     # 工具函数
│   │   └── packet_test.go # 单元测试
│   ├── registerSession/  # 会话注册
│   ├── unRegisterSession/ # 会话注销
│   ├── listIdentity/     # 设备识别信息
│   ├── listInterface/    # 接口列表
│   ├── listServices/     # 服务列表
│   ├── sendRRData/       # 发送路由数据
│   ├── sendUnitData/     # 发送单元数据
│   └── nop/              # NOP 命令（空操作）
├── path/                # CIP 路径构建
│   ├── path.go          # 逻辑路径、端口路径、数据路径
│   └── path_test.go     # 单元测试
├── types/               # 类型定义
│   └── types.go         # 所有数据类型定义
├── utils/               # 工具函数
│   ├── len.go           # 长度计算
│   ├── mmap.go          # 内存映射
│   ├── mmap_unix.go     # Unix 平台内存映射
│   ├── mmap_windows.go  # Windows 平台内存映射
│   └── simd.go          # SIMD 优化
├── test/                # 集成测试
│   ├── cpppo/           # cpppo 兼容性测试
│   ├── protocol_verifier_test.go # 协议验证测试
│   └── access_mode_test.go # 访问模式测试
├── doc/                 # 文档
│   ├── PERFORMANCE_OPTIMIZATION*.md # 性能优化文档
│   └── performance_report.json      # 性能报告
├── config.go            # 配置结构
├── context.go           # 上下文生成器
├── doc.go               # Go 文档注释
├── tcp.go               # TCP 连接管理（含重连机制）
├── tcp_pool.go          # TCP 连接池
├── tag.go               # Tag 操作核心
├── request.go           # 请求处理
├── example.go           # 使用示例
└── go.mod               # Go 模块配置
```

### 7.3 关键实现代码模式

**连接管理**：
```go
// 创建TCP连接
tcp := ethernetip.NewTCP("192.168.1.10:44818")

// 创建会话
session, err := tcp.CreateSession()
if err != nil {
    return err
}

// 注册会话
err = session.Register()
if err != nil {
    return err
}

// 读取Tag
data, err := session.ReadTag("Motor_Speed", 1)
if err != nil {
    return err
}

// 写入Tag
err = session.WriteTag("Motor_Speed", data)

// 关闭会话
session.Unregister()
session.Close()
```

**依赖注入测试模式**：
```go
// 使用clientFactory注入mock对象进行单元测试
type ENIPTransport struct {
    clientFactory func(address string) ENIPClient
    // ...
}
```

## 8. 测试计划

### 8.1 单元测试

| 测试文件 | 测试用例数 | 覆盖范围 |
| :--- | :--- | :--- |
| decoder_test.go | 10 | Tag解析、数据类型编解码、配置解析 |
| transport_test.go | 8 | 连接管理、心跳控制、重试逻辑、指标统计 |
| scheduler_test.go | 6 | 批量读取、调度策略、分批处理 |

### 8.2 集成测试

| 测试用例 | 测试内容 | 预期结果 |
| :--- | :--- | :--- |
| TestEtherNetIPConnect | 连接真实PLC | 连接成功，获取Session ID |
| TestReadSingleTag | 读取单个Tag | 正确解析数据类型和值 |
| TestReadMultipleTags | 批量读取多个Tag | 减少网络往返，提高效率 |
| TestWriteTag | 写入Tag值 | 写入成功，PLC端数据更新 |
| TestConnectionRecovery | 连接断开后自动重连 | 自动重连，恢复通信 |
| TestHeartbeat | 心跳保活机制 | 定期发送心跳，保持连接 |

### 8.3 验收测试

| 检查项 | 标准 |
| :--- | :--- |
| 连接稳定性 | 连续运行24小时无断连 |
| 数据准确性 | 读取值与PLC端一致 |
| 批量读取效率 | 10个Tag批量读取比逐个读取快80%以上 |
| 重连恢复时间 | 断连后3秒内自动重连 |
| 内存占用 | 稳定状态下内存占用不超过50MB |

## 9. 文件结构

```
internal/driver/ethernetip/
├── ethernetip.go           # 驱动主入口
├── transport.go            # 传输层（TCP连接、会话管理）
├── decoder.go              # 地址解码器（Tag解析、数据类型）
├── scheduler.go            # 调度器（批量读取优化）
├── transport_test.go       # 传输层单元测试
├── decoder_test.go         # 解码器单元测试
├── scheduler_test.go       # 调度器单元测试
└── integration_test.go     # 集成测试
```

## 10. 风险评估

| 风险 | 影响 | 缓解措施 |
| :--- | :--- | :--- |
| go-ethernet-ip库不稳定 | 高 | 评估库的质量，考虑使用gologix作为备选 |
| 不同PLC兼容性 | 中 | 针对主流PLC型号测试，保留配置扩展性 |
| 批量读取限制 | 中 | 实现分批读取机制，处理超限情况 |
| 连接超时处理 | 中 | 实现智能超时和重试机制 |

## 11. 里程碑

1. **Phase 1**: 基础连接管理 - 完成transport.go实现
2. **Phase 2**: Tag读写功能 - 完成decoder.go和scheduler.go实现
3. **Phase 3**: 单元测试 - 完成各层单元测试
4. **Phase 4**: 集成测试 - 连接真实PLC验证功能
5. **Phase 5**: 前端集成 - 配置界面增强
6. **Phase 6**: 性能优化 - 批量读取调优

## 12. 参考资源

- [go-ethernet-ip库](https://github.com/anviod/ethernet-ip)
- [gologix库](https://github.com/danomagnum/gologix) (备选)
- [EtherNet/IP协议规范](https://literature.rockwellautomation.com/lc/73/Section1.htm)
- [CIP协议规范](https://www.odva.org/technology-standards/key-technologies/cip)
