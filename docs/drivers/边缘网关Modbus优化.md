---
layout: default
title: 边缘网关 Modbus 优化
description: edgeCore 边缘网关 Modbus 优化
---

# 边缘网关 Modbus 优化

> ✅ 通信层（Transport）
> ✅ 调度层（Scheduler）
> ✅ 解析层（Decoder）
> ✅ 设备状态机（DeviceState）
> ✅ 驱动整合层（Driver Facade）

---

# 一、总体架构图

```
┌────────────────────────────────────────────┐
│                ModbusDriver                │  ← 对外 Driver 接口
├────────────────────────────────────────────┤
│            DeviceStateMachine              │  ← 设备状态、降级、恢复
├────────────────────────────────────────────┤
│               PointScheduler               │  ← 分组、调度、重试、跳点
├────────────────────────────────────────────┤
│               ModbusTransport              │  ← TCP/RTU 通信、重连、超时
├────────────────────────────────────────────┤
│               PointDecoder                 │  ← 字节序、类型、缩放、异常
└────────────────────────────────────────────┘
```

---

# 二、通信层（Transport）模板

### 🎯 目标

* 屏蔽 TCP / RTU 差异
* 提供：

  * 自动重连
  * 错误分类
  * 心跳检测
  * 超时 / 重试

---

### 1️⃣ 接口定义

```go
type Transport interface {
    Connect(ctx context.Context) error
    Disconnect() error
    IsConnected() bool

    ReadRegisters(ctx context.Context, regType string, offset uint16, count uint16) ([]byte, error)
    ReadCoil(ctx context.Context, offset uint16) (bool, error)
    ReadDiscreteInput(ctx context.Context, offset uint16) (bool, error)

    WriteRegister(ctx context.Context, offset uint16, value uint16) error
    WriteRegisters(ctx context.Context, offset uint16, values []uint16) error
    WriteCoil(ctx context.Context, offset uint16, value bool) error

    SetUnitID(id uint8)
}
```

---

### 2️⃣ ModbusTransport 实现骨架

```go
type ModbusTransport struct {
    cfg       model.DriverConfig
    client    *modbus.ModbusClient
    connected atomic.Bool
    mu        sync.Mutex

    timeout        time.Duration
    maxRetries     int
    retryInterval  time.Duration
    heartbeatAddr  *uint16
    heartbeatTimer *time.Ticker
}
```

---

### 3️⃣ 核心方法模板

```go
func (t *ModbusTransport) Connect(ctx context.Context) error {
    t.mu.Lock()
    defer t.mu.Unlock()

    if t.connected.Load() {
        return nil
    }

    client, err := newClientFromConfig(t.cfg)
    if err != nil {
        return err
    }

    if err := client.Open(); err != nil {
        return err
    }

    t.client = client
    t.connected.Store(true)

    if hb := t.heartbeatAddr; hb != nil {
        go t.startHeartbeat()
    }
    return nil
}

func (t *ModbusTransport) Disconnect() error {
    t.mu.Lock()
    defer t.mu.Unlock()

    if t.client != nil {
        _ = t.client.Close()
    }
    t.connected.Store(false)
    return nil
}

func (t *ModbusTransport) ReadRegisters(ctx context.Context, regType string, offset uint16, count uint16) ([]byte, error) {
    return t.withRetry(ctx, func() ([]byte, error) {
        switch regType {
        case "HOLDING_REGISTER":
            return t.client.ReadBytes(offset, count*2, modbus.HOLDING_REGISTER)
        case "INPUT_REGISTER":
            return t.client.ReadBytes(offset, count*2, modbus.INPUT_REGISTER)
        default:
            return nil, fmt.Errorf("unsupported regType: %s", regType)
        }
    })
}

func (t *ModbusTransport) withRetry(ctx context.Context, fn func() ([]byte, error)) ([]byte, error) {
    var lastErr error
    for i := 0; i <= t.maxRetries; i++ {
        if i > 0 {
            time.Sleep(t.retryInterval)
        }
        data, err := fn()
        if err == nil {
            return data, nil
        }
        lastErr = err
        if isFatalError(err) {
            _ = t.Disconnect()
            _ = t.Connect(ctx)
        }
    }
    return nil, lastErr
}
```

---

# 三、调度层（Scheduler）模板

### 🎯 目标

* 点位分组
* 点位失败隔离
* 支持优先级与多周期
* 不因单点失败阻塞整体

---

### 1️⃣ 点位运行态结构

```go
type PointRuntime struct {
    Point         model.Point
    FailCount     int
    LastSuccess   time.Time
    State         string // OK, DEGRADED, SKIPPED
    CooldownUntil time.Time
}
```

---

### 2️⃣ 调度器接口

```go
type Scheduler interface {
    Read(ctx context.Context, points []model.Point) (map[string]model.Value, error)
}
```

---

### 3️⃣ Scheduler 实现骨架

```go
type PointScheduler struct {
    transport Transport
    decoder   Decoder

    maxPacketSize  uint16
    groupThreshold uint16

    pointStates map[string]*PointRuntime
    mu          sync.Mutex
}
```

---

### 4️⃣ 核心调度流程

```go
func (s *PointScheduler) Read(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
    now := time.Now()
    result := make(map[string]model.Value)

    runtimes := s.prepareRuntimes(points)
    groups := s.groupPoints(runtimes)

    for _, group := range groups {
        values, err := s.readGroup(ctx, group)
        if err != nil {
            s.markGroupFailed(group, now)
            continue
        }

        for id, val := range values {
            result[id] = model.Value{
                PointID: id,
                Value:   val,
                Quality: "Good",
                TS:      now,
            }
            s.markPointSuccess(id, now)
        }
    }
    return result, nil
}
```

---

### 5️⃣ 点位失败隔离机制（核心）

> ⚠️ 更新（2026-09-02）：失败已做**两点因分离**——
> 连接级错误（timeout / not connected / 链路故障）视为**设备级故障**，
> 点位**解冻保持 OK、不计冷却**，避免设备恢复后被 SKIPPED 点位饿死；
> 仅点位级错误（如非法地址）进入冷却阶梯。详见
> [modbus_冷却防饿死机制](./modbus冷却防饿死机制.md)。

```go
func (s *PointScheduler) markPointFailed(pointID string, err error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    rt := s.pointStates[pointID]
    if err != nil && isConnectionError(err) {
        // 设备级故障：解冻点位，恢复即全量可读（防饿死）
        rt.FailCount = 0
        rt.State = "OK"
        rt.CooldownUntil = time.Time{}
        return
    }

    rt.FailCount++
    if isIllegalAddress(err) {
        rt.State = "SKIPPED"
        rt.CooldownUntil = time.Now().Add(24 * time.Hour) // 非法地址长隔离
    } else if rt.FailCount >= 10 {
        rt.State = "SKIPPED"
        rt.CooldownUntil = time.Now().Add(5 * time.Minute)
    } else if rt.FailCount >= 3 {
        rt.State = "SKIPPED"
        rt.CooldownUntil = time.Now().Add(60 * time.Second)
    }
}
```

---

# 四、解析层（Decoder）模板

### 🎯 目标

* 支持多数据类型
* 支持 bit / bcd / string
* 支持字节序覆盖
* 支持异常策略与质量码

---

### 1️⃣ Decoder 接口

```go
type Decoder interface {
    Decode(point model.Point, raw []byte) (any, string, error)
    Encode(point model.Point, value any) ([]uint16, error)
}
```

---

### 2️⃣ Decoder 实现骨架

```go
type PointDecoder struct {
    defaultByteOrder string
}
```

---

### 3️⃣ Decode 模板

```go
func (d *PointDecoder) Decode(point model.Point, raw []byte) (any, string, error) {
    val, err := d.decodeRaw(point, raw)
    if err != nil {
        return nil, "Bad", err
    }

    val = d.applyScaleOffset(point, val)
    quality := d.applyRangeCheck(point, val)

    return val, quality, nil
}
```

---

### 4️⃣ 支持读取 Bit 点位 (也需要支持写)

```go
func decodeBit(raw []byte, bitIndex int) bool {
    v := binary.BigEndian.Uint16(raw)
    return ((v >> bitIndex) & 0x1) == 1
}
```

---

### 5️⃣ Encode（写入反算）

```go
func (d *PointDecoder) Encode(point model.Point, value any) ([]uint16, error) {
    rawValue := d.reverseScaleOffset(point, value)
    return d.encodeRaw(point, rawValue)
}
```

---

# 五、设备状态机模板

### 🎯 目标

* 管理 ONLINE / DEGRADED / OFFLINE / RECOVERING
* 与调度器联动

---

### 1️⃣ 状态定义

```go
type DeviceState string

const (
    StateOnline     DeviceState = "ONLINE"
    StateDegraded   DeviceState = "DEGRADED"
    StateOffline    DeviceState = "OFFLINE"
    StateRecovering DeviceState = "RECOVERING"
)
```

---

### 2️⃣ 状态机骨架

```go
type DeviceStateMachine struct {
    state           DeviceState
    failCount       int
    lastSuccess     time.Time
    degradeThreshold int
    recoverThreshold int
}
```

---

### 3️⃣ 状态迁移逻辑

```go
func (sm *DeviceStateMachine) OnFailure() {
    sm.failCount++
    if sm.failCount >= sm.degradeThreshold {
        sm.state = StateDegraded
    }
    if sm.failCount >= sm.degradeThreshold*2 {
        sm.state = StateOffline
    }
}

func (sm *DeviceStateMachine) OnSuccess() {
    sm.failCount = 0
    if sm.state == StateOffline || sm.state == StateDegraded {
        sm.state = StateRecovering
    } else {
        sm.state = StateOnline
    }
}
```

---

# 六、驱动整合层（Facade）

### 🎯 目标

* 对外保持你原有 Driver 接口不变
* 内部使用新架构组件

---

### ModbusDriver 新骨架

```go
type ModbusDriver struct {
    transport Transport
    scheduler Scheduler
    stateMachine *DeviceStateMachine
}
```

---

### ReadPoints 实现模板

```go
func (d *ModbusDriver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
    if err := d.transport.Connect(ctx); err != nil {
        d.stateMachine.OnFailure()
        return nil, err
    }

    values, err := d.scheduler.Read(ctx, points)
    if err != nil {
        d.stateMachine.OnFailure()
        return values, err
    }

    d.stateMachine.OnSuccess()
    return values, nil
}
```

---

### WritePoint 实现模板

```go
func (d *ModbusDriver) WritePoint(ctx context.Context, point model.Point, value any) error {
    regs, err := d.scheduler.GetDecoder().Encode(point, value)
    if err != nil {
        return err
    }
    // 由 scheduler 调度 write
    return d.scheduler.Write(ctx, point, regs)
}
```

---

# 七、落地实施建议

你可以：

### ✅ 第一步（低风险改造）

* 抽出：

  * `decodeValue → PointDecoder`
  * `readPointGroup → Scheduler`
  * `modbus.Client → Transport`

不改对外接口。

---

### ✅ 第二步（增强能力）

* 引入点位失败隔离
* 引入设备状态机
* 写入支持 Scale/Offset 反算 + bit 写

---

### ✅ 第三步（工业级增强）

* 多周期调度
* 高优先级点位
* 写入事务校验
* 多质量码支持

---

# 八、最终实现

1. 🔧 输出**通信层 ModbusTransport 完整可运行代码**
2. 🔧 输出**Scheduler 完整分组 + 跳点 + 重试实现**
3. 🔧 输出**Decoder 完整解析 + bit/bcd/string 实现**
4. 📄 输出**Modbus 点位配置规范文档（JSON/YAML 模板）**

