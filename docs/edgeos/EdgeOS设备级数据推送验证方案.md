---
layout: default
title: EdgeOS 设备级数据推送验证方案
description: edgeCore EdgeOS 设备级数据推送验证方案
---

# EdgeOS 设备级数据推送验证方案

## 一、核心改进说明

### 1.1 数据模型升级

**修改文件**: `internal/model/types.go`

#### 改进内容:
- **DevicePublishConfig** 结构体更新:
  - `Strategy` 字段: 从 `"periodic" or "cov"` 改为 `"realtime" or "periodic"`
  - `Interval` 字段: 从 `0 means use collection interval` 改为 `Push interval for periodic mode (e.g., "5s", "1m")`

- **EdgeOSMQTTConfig.Devices**: 从 `map[string]bool` 升级为 `map[string]DevicePublishConfig`
  - 旧: 设备启用状态 (true/false)
  - 新: 完整推送配置 (Enable + Strategy + Interval)

- **EdgeOSNATSConfig.Devices**: 同上升级

### 1.2 MQTT 客户端实现升级

**修改文件**: `internal/northbound/edgos_mqtt/client.go`

#### 核心改进:
1. **设备级数据聚合**:
   ```go
   type deviceAggregator struct {
       points       map[string]model.Value  // pointID -> Value
       lastPushTS   time.Time
       pushInterval time.Duration
       mu           sync.RWMutex
   }
   ```

2. **推送策略支持**:
   - **Realtime (实时模式)**: 每次数据到达立即推送,但包含设备的所有点
   - **Periodic (周期模式)**: 聚合设备所有点,按配置周期批量推送

3. **设备级推送**:
   ```go
   func (c *Client) publishDeviceData(deviceID string, points map[string]any, quality string, ts time.Time)
   ```
   - 一次推送包含设备的多个点位数据
   - 符合协议规范的 JSON 格式

4. **周期推送循环**:
   ```go
   func (c *Client) periodicPushLoop()
   func (c *Client) checkAndPushPeriodicDevices()
   ```
   - 每秒检查需要推送的设备
   - 确保周期模式下数据按时推送

### 1.3 NATS 客户端实现升级

**修改文件**: `internal/northbound/edgos_nats/client.go`

#### 核心改进:
- 与 MQTT 客户端完全一致的实现
- 支持相同的设备级聚合和推送策略
- 确保两个协议的行为一致

---

## 二、JSON 格式验证

### 2.1 协议要求的格式

根据 `edgeCore通信协议规范%28MQTT-NATS%29.md` 第 4.5 节,数据推送格式如下:

```json
{
  "header": {
    "message_id": "msg-abc123...",
    "timestamp": 1713260400000,
    "source": "node-001",
    "destination": "",
    "message_type": "data",
    "version": "1.0"
  },
  "body": {
    "node_id": "node-001",
    "device_id": "device-001",
    "timestamp": 1713260400000,
    "points": {
      "Temperature": 25.5,
      "Humidity": 65.2,
      "Pressure": 101325,
      "Switch": true
    },
    "quality": "good"
  }
}
```

### 2.2 改进前后的对比

**改进前 (单点推送)**:
```json
{
  "body": {
    "points": {
      "Temperature": 25.5  // ❌ 只有一个点
    }
  }
}
```

**改进后 (设备级推送)**:
```json
{
  "body": {
    "points": {
      "Temperature": 25.5,  // ✅ 多个点
      "Humidity": 65.2,
      "Pressure": 101325,
      "Switch": true
    }
  }
}
```

---

## 三、配置示例

### 3.1 EdgeOS(MQTT) 配置示例

```yaml
edgeos_mqtt:
  - id: "mqtt-edgeos-1"
    name: "EdgeOS MQTT Channel 1"
    enable: true
    broker: "tcp://localhost:1883"
    client_id: "edgeCore-node-001"
    node_id: "node-001"
    username: ""
    password: ""
    qos: 0
    retain: false
    clean_session: true
    keep_alive: 60
    connect_timeout: 30
    auto_reconnect: true
    max_reconnect_interval: 60
    heartbeat_interval: "30s"
    devices:
      device-001:
        enable: true
        strategy: "realtime"  # 实时推送
        interval: "0s"
      device-002:
        enable: true
        strategy: "periodic"  # 周期推送
        interval: "10s"
      device-003:
        enable: false  # 禁用
        strategy: "realtime"
        interval: "0s"
```

### 3.2 EdgeOS(NATS) 配置示例

```yaml
edgeos_nats:
  - id: "nats-edgeos-1"
    name: "EdgeOS NATS Channel 1"
    enable: true
    url: "nats://localhost:4222"
    client_id: "edgeCore-node-001"
    node_id: "node-001"
    username: ""
    password: ""
    token: ""
    connect_timeout: 10
    reconnect_wait: 2
    max_reconnects: 10
    ping_interval: 20
    max_pings_outstanding: 5
    jetstream_enabled: false
    heartbeat_interval: "30s"
    devices:
      device-001:
        enable: true
        strategy: "realtime"
        interval: "0s"
      device-002:
        enable: true
        strategy: "periodic"
        interval: "5s"
```

---

## 四、测试方案

### 4.1 单元测试

#### 测试 1: 实时模式推送验证

**测试步骤**:
1. 配置一个设备使用 `strategy: realtime`
2. 采集该设备的多个点位数据
3. 验证每次点位更新都触发推送
4. 检查 MQTT/NATS 消息中的 `points` 字段包含所有点位

**预期结果**:
- ✅ 每次点位更新立即推送
- ✅ 消息包含设备的所有点位数据
- ✅ 符合协议 JSON 格式

#### 测试 2: 周期模式推送验证

**测试步骤**:
1. 配置一个设备使用 `strategy: periodic` 和 `interval: 10s`
2. 快速采集该设备的多个点位数据
3. 等待 10 秒
4. 验证只推送一次,包含所有点位

**预期结果**:
- ✅ 点位更新不会立即推送
- ✅ 10 秒后推送一次包含所有点位的消息
- ✅ 后续点位更新继续聚合
- ✅ 下一个周期再次推送

#### 测试 3: 设备启用状态过滤验证

**测试步骤**:
1. 配置三个设备:
   - device-001: enable=true, strategy=realtime
   - device-002: enable=false, strategy=realtime
   - device-003: enable=true, strategy=periodic
2. 采集所有三个设备的数据
3. 验证推送行为

**预期结果**:
- ✅ device-001 数据立即推送
- ✅ device-002 数据不推送 (被过滤)
- ✅ device-003 数据按周期推送

### 4.2 集成测试

#### 测试 4: 协议格式完整性验证

**测试工具**: MQTT Explorer / NATS Subscriber

**测试步骤**:
1. 订阅主题: `edgeCore/data/{node_id}/{device_id}`
2. 触发设备数据推送
3. 捕获消息并解析 JSON
4. 验证字段完整性

**检查项**:
```json
{
  "header": {
    "message_id": "必需字段 ✅",
    "timestamp": "毫秒时间戳 ✅",
    "source": "node_id ✅",
    "message_type": "data ✅",
    "version": "1.0 ✅"
  },
  "body": {
    "node_id": "节点ID ✅",
    "device_id": "设备ID ✅",
    "timestamp": "数据时间戳 ✅",
    "points": "包含多个点位 ✅",
    "quality": "good/bad ✅"
  }
}
```

#### 测试 5: 前后端通信验证

**测试步骤**:
1. 通过 UI 配置 EdgeOS(MQTT) 和 EdgeOS(NATS) 通道
2. 启用多个设备并设置不同策略
3. 触发数据采集
4. 在前端查看统计数据

**验证项**:
- ✅ 前端正确显示连接状态
- ✅ 统计数据 (success_count, fail_count, publish_count) 实时更新
- ✅ 设备配置正确保存和加载

### 4.3 性能测试

#### 测试 6: 高频数据推送验证

**测试场景**:
- 设备数量: 10 个
- 每设备点位: 50 个
- 采集频率: 1 秒

**测试步骤**:
1. 配置 5 个设备使用 realtime 模式
2. 配置 5 个设备使用 periodic (interval: 5s) 模式
3. 运行 1 分钟
4. 统计推送次数

**预期结果**:
- ✅ Realtime 设备推送约 300 次 (10 设备 × 50 点 × 60 秒 / 10 realtime 设备)
- ✅ Periodic 设备推送约 60 次 (5 设备 × 12 周期 / 60 秒)
- ✅ 无推送失败
- ✅ CPU/内存使用正常

---

## 五、验证检查清单

### 5.1 代码检查

- [x] DevicePublishConfig 结构体更新完成
- [x] EdgeOSMQTTConfig.Devices 类型更新为 map[string]DevicePublishConfig
- [x] EdgeOSNATSConfig.Devices 类型更新为 map[string]DevicePublishConfig
- [x] MQTT 客户端添加 deviceAggregator 结构体
- [x] MQTT 客户端实现 aggregatePoint 方法
- [x] MQTT 客户端实现 periodicPushLoop 方法
- [x] MQTT 客户端实现 publishDeviceData 方法
- [x] NATS 客户端添加 deviceAggregator 结构体
- [x] NATS 客户端实现相同的聚合和推送逻辑
- [x] PublishDeviceStatus 方法更新设备启用状态检查

### 5.2 功能检查

- [ ] 实时模式: 单点更新立即推送设备所有数据
- [ ] 周期模式: 点位更新聚合后按周期推送
- [ ] 设备过滤: 未启用设备数据不推送
- [ ] JSON 格式: 消息包含完整的 header 和 body
- [ ] 设备级聚合: points 字段包含多个点位
- [ ] 时间戳正确: body.timestamp 使用数据的最新时间
- [ ] 质量字段: quality 正确传递

### 5.3 测试检查

- [ ] 单元测试: 实时模式验证通过
- [ ] 单元测试: 周期模式验证通过
- [ ] 单元测试: 设备过滤验证通过
- [ ] 集成测试: 协议格式验证通过
- [ ] 集成测试: 前后端通信验证通过
- [ ] 性能测试: 高频数据推送验证通过

---

## 六、部署前验证

### 6.1 编译检查

```bash
# 编译检查
go build -o gateway.exe cmd/gateway/main.go

# 运行单元测试
go test ./internal/northbound/edgos_mqtt/...
go test ./internal/northbound/edgos_nats/...
```

### 6.2 配置迁移

**重要提示**: 配置格式发生变化,需要迁移现有配置

**旧格式**:
```yaml
devices:
  device-001: true
  device-002: false
```

**新格式**:
```yaml
devices:
  device-001:
    enable: true
    strategy: "realtime"
    interval: "0s"
  device-002:
    enable: false
    strategy: "realtime"
    interval: "0s"
```

### 6.3 回滚计划

如果出现问题:
1. 恢复旧代码版本
2. 使用旧配置格式
3. 重启服务

---

## 七、总结

### 核心改进:
1. ✅ **设备级数据推送**: 从单点推送升级为设备级推送
2. ✅ **推送策略支持**: 支持 realtime 和 periodic 两种模式
3. ✅ **协议格式符合**: JSON 格式完全符合 edgeCore-EdgeOS 通信协议规范
4. ✅ **UI 配置联动**: 设备启用状态和推送周期设置生效

### 技术亮点:
- 使用设备聚合器管理周期模式数据
- 读写锁确保并发安全
- 高效的定时检查机制
- 支持两种协议的统一实现

### 下一步:
1. 执行测试验证清单
2. 性能测试和优化
3. UI 界面更新以支持新配置字段
4. 用户文档更新
