---
layout: default
title: EdgeOS 升级说明
description: edgeCore EdgeOS 升级说明
---

# EdgeOS 升级说明

## 版本信息
- **升级日期**: 2026-04-16
- **影响模块**: EdgeOS(MQTT) 和 EdgeOS(NATS) 北向数据推送

## 核心改进

### 1. 设备级数据推送
从**单点推送**升级为**设备级推送**:

**改进前**: 每个点位数据单独推送一条消息
```json
{
  "body": {
    "points": {
      "Temperature": 25.5  // ❌ 只有一个点
    }
  }
}
```

**改进后**: 设备的所有点位数据在一条消息中推送
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

### 2. 推送策略支持
支持两种推送策略:
- **realtime**: 实时推送 (数据到达立即推送)
- **periodic**: 周期推送 (聚合数据,按配置周期批量推送)

### 3. 独立推送周期
每个设备可以配置独立的推送周期:
```yaml
devices:
  device-001:
    enable: true
    strategy: realtime   # 实时推送
    interval: "0s"      # 不适用
  device-002:
    enable: true
    strategy: periodic   # 周期推送
    interval: "10s"     # 每10秒推送一次
  device-003:
    enable: false       # 禁用
    strategy: realtime
    interval: "0s"
```

## 配置迁移

### 自动迁移
使用迁移脚本自动更新配置格式:

```bash
# Linux/Mac
bash scripts/migrate_edgeos_config.sh conf/northbound.yaml

# Windows (使用 Git Bash)
bash scripts/migrate_edgeos_config.sh conf/northbound.yaml
```

### 手动迁移
如果自动迁移失败,可以手动更新配置:

**旧格式**:
```yaml
edgeos_mqtt:
  - id: "mqtt-edgeos-1"
    devices:
      device-001: true
      device-002: false
```

**新格式**:
```yaml
edgeos_mqtt:
  - id: "mqtt-edgeos-1"
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

### 推荐配置策略

#### 实时推送 (实时监控场景)
```yaml
device-001:
  enable: true
  strategy: "realtime"
  interval: "0s"
```
**适用场景**: 报警、关键参数监控

#### 周期推送 (数据归档场景)
```yaml
device-001:
  enable: true
  strategy: "periodic"
  interval: "60s"  # 1分钟
```
**适用场景**: 历史数据记录、趋势分析

#### 混合配置
```yaml
devices:
  # 关键设备 - 实时推送
  alarm-device:
    enable: true
    strategy: "realtime"
    interval: "0s"
  
  # 监控设备 - 10秒周期
  sensor-device:
    enable: true
    strategy: "periodic"
    interval: "10s"
  
  # 记录设备 - 1分钟周期
  logger-device:
    enable: true
    strategy: "periodic"
    interval: "60s"
  
  # 测试设备 - 禁用
  test-device:
    enable: false
    strategy: "realtime"
    interval: "0s"
```

## 升级步骤

### 1. 备份配置
```bash
cp conf/northbound.yaml conf/northbound.yaml.backup
```

### 2. 更新代码
```bash
git pull
```

### 3. 迁移配置
```bash
bash scripts/migrate_edgeos_config.sh conf/northbound.yaml
```

### 4. 根据需求调整配置
根据业务需求调整每个设备的:
- `enable`: 是否启用
- `strategy`: 推送策略 (realtime/periodic)
- `interval`: 推送周期 (仅 periodic 模式有效)

### 5. 重新编译
```bash
go build -o gateway ./cmd/main.go
```

### 6. 重启服务
```bash
./gateway.sh restart
```

### 7. 验证升级
- 检查服务是否正常启动
- 查看 edgeCore-EdgeOS 通信日志
- 使用 MQTT Explorer / NATS Subscriber 检查消息格式
- 确认统计数据正常更新

## 回滚方案

如果升级后出现问题:

1. **停止服务**
   ```bash
   ./gateway.sh stop
   ```

2. **恢复旧代码**
   ```bash
   git checkout <previous_commit>
   ```

3. **恢复旧配置**
   ```bash
   cp conf/northbound.yaml.backup conf/northbound.yaml
   ```

4. **重新编译并启动**
   ```bash
   go build -o gateway ./cmd/main.go
   ./gateway.sh start
   ```

## 技术细节

### 数据流变化

**改进前**:
```
采集 → 管道 → NorthboundManager → EdgeOS Client
                              ↓ (每个点)
                         Publish(v)
                              ↓
                    MQTT/NATS (单点消息)
```

**改进后**:
```
采集 → 管道 → NorthboundManager → EdgeOS Client
                              ↓ (每个点)
                         Publish(v)
                              ↓
                   ┌───────────┴───────────┐
               Realtime Mode    Periodic Mode
                   ↓                  ↓
          publishDeviceData    aggregatePoint
                   ↓                  ↓
               MQTT/NATS         检查周期推送
                               ↓
                         publishAggregatedDevice
                               ↓
                         MQTT/NATS (设备级消息)
```

### 性能优化

1. **减少消息数量**: 
   - 设备有 N 个点,推送消息从 N 条减少到 1 条
   - 实时模式: N → 1
   - 周期模式: N × M → 1 (M = 周期内更新次数)

2. **降低网络开销**:
   - 减少协议头开销
   - 减少连接频繁建立/断开

3. **提高可靠性**:
   - 设备级聚合避免部分数据丢失
   - 周期模式降低峰值流量压力

### 兼容性

- ✅ 协议格式完全符合 edgeCore-EdgeOS 通信协议规范
- ✅ 支持旧版配置格式自动迁移
- ✅ 向后兼容未配置设备的默认行为
- ✅ 与现有 MQTT/NATS 客户端完全兼容

## 常见问题

### Q1: 配置迁移失败怎么办?
A: 使用手动迁移方式,参考上面的"手动迁移"章节。

### Q2: 如何选择推送策略?
A: 
- 实时监控场景: 使用 `realtime`
- 数据归档场景: 使用 `periodic`
- 混合需求: 不同设备使用不同策略

### Q3: 周期模式的 interval 支持哪些值?
A: 支持 Go Duration 格式,例如:
- `"1s"` - 1秒
- `"10s"` - 10秒
- `"1m"` - 1分钟
- `"5m"` - 5分钟
- `"1h"` - 1小时

### Q4: 实时模式会有性能问题吗?
A: 不会。改进后的实时模式虽然每次都推送,但一条消息包含设备的所有点,而不是每个点一条消息,反而降低了网络开销。

### Q5: 如何查看推送统计数据?
A: 在 UI 的北向通道页面可以看到:
- success_count: 成功推送次数
- fail_count: 失败推送次数
- publish_count: 总推送次数

### Q6: 能否禁用某个设备的数据推送?
A: 可以。设置 `enable: false` 即可:
```yaml
device-001:
  enable: false
  strategy: "realtime"
  interval: "0s"
```

## 技术支持

如有问题,请参考:
- [edgeCore 通信协议规范](../edgeos/edgeCore通信协议规范%28MQTT-NATS%29.html)
- [EdgeOS 设备级数据推送验证方案](../edgeos/EdgeOS设备级数据推送验证方案.html)
