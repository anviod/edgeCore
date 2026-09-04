---
layout: default
title: 边缘计算场景手册
description: edgeCore 边缘计算场景手册 — 典型工业场景配置示例
---

# 边缘计算场景手册

> **文档定位：** 典型工业场景配置示例。场景骨架与 UI「场景模版」一致（`ui/src/utils/edgeSceneTemplates.js`），下列 JSON 可直接用于 `POST /api/edge/rules`（绑定真实 `channel_id` / `device_id` / `point_id` 后启用）。

---

## 一、使用说明

### 1.1 套用步骤

1. 在 UI **边缘计算 → 场景模版** 选择场景，或复制本文 JSON
2. 替换 `sources` 与各动作中的通道、设备、点位 ID
3. 配置北向 `mqtt_config_id` / `http_config_id`（若使用 MQTT/HTTP 动作）
4. 保持 `enable: false` 完成绑定后，在 UI 启用或通过 API 将 `enable` 设为 `true`
5. 用 `log` 动作或 `/api/edge/states` 验证触发

### 1.2 场景分类

| 分类 | 说明 | 模版数 |
|------|------|--------|
| 告警联动 | 越限联动、边缘实时决策、环境温湿度、冷链物流、网络设备、预测性维护 | 6 |
| 群控策略 | 多设备联动控制、电力自动化遥控 | 2 |
| 数据聚合 | 跨设备聚合、联合抄表、光伏逆变器、楼宇能耗、产线节拍 | 5 |
| 其他 | 设备远程监控、Modbus 透传、断点续传、多协议网关、时序本地存储 | 5 |

UI 场景模版筛选栏为 **全部场景 / 告警联动 / 群控策略 / 数据聚合 / 其他**，共 **18 个模版**，与 [产品说明](../guide/产品说明.html) 中 18 类典型场景一一对应。各模版 `category` 与产品文档分类一致；卡片标签展示细粒度 `sceneType`。

---

## 二、告警联动

### 2.1 温度越限联动冷却

监测温度超过阈值且冷却未运行时，写入冷却设备启停点。

```json
{
  "name": "温度越限联动冷却",
  "type": "threshold",
  "enable": false,
  "priority": 10,
  "trigger_mode": "on_change",
  "check_interval": "1s",
  "sources": [
    { "alias": "temp", "channel_id": "CH_MODBUS", "device_id": "sensor_01", "point_id": "temp" },
    { "alias": "cooling_run", "channel_id": "CH_MODBUS", "device_id": "hvac_01", "point_id": "run_fb" }
  ],
  "condition": "temp > 35 && cooling_run == 0",
  "state": { "duration": "3s", "count": 0 },
  "actions": [
    {
      "type": "device_control",
      "config": {
        "channel_id": "CH_MODBUS",
        "device_id": "hvac_01",
        "point_id": "start",
        "value": "1"
      }
    },
    {
      "type": "log",
      "config": { "level": "warn", "message": "温度 ${temp}°C 越限，已启动冷却" }
    }
  ]
}
```

### 2.2 设备故障切换备用通道

主设备故障且备用就绪时，顺序停用主通道、延时后启用备用。

```json
{
  "name": "设备故障切换备用通道",
  "type": "threshold",
  "priority": 20,
  "trigger_mode": "on_change",
  "check_interval": "500ms",
  "sources": [
    { "alias": "fault", "channel_id": "CH_S7", "device_id": "line_a", "point_id": "fault_bit" },
    { "alias": "backup_ready", "channel_id": "CH_S7", "device_id": "line_b", "point_id": "ready" }
  ],
  "condition": "fault == 1 && backup_ready == 1",
  "state": { "duration": "2s" },
  "actions": [
    {
      "type": "sequence",
      "config": {
        "steps": [
          { "type": "device_control", "config": { "channel_id": "CH_S7", "device_id": "line_a", "point_id": "enable", "value": "0" } },
          { "type": "delay", "config": { "duration": "1s" } },
          { "type": "device_control", "config": { "channel_id": "CH_S7", "device_id": "line_b", "point_id": "enable", "value": "1" } },
          { "type": "log", "config": { "level": "error", "message": "主设备故障，已切换备用通道" } }
        ]
      }
    }
  ]
}
```

### 2.3 安全联锁自动停机

入侵检测且设备运行、未急停时，高优先级停机并 MQTT 上报。

```json
{
  "name": "安全联锁自动停机",
  "type": "state",
  "priority": 100,
  "trigger_mode": "on_change",
  "check_interval": "500ms",
  "sources": [
    { "alias": "intrusion", "channel_id": "CH_MODBUS", "device_id": "safety", "point_id": "light_curtain" },
    { "alias": "running", "channel_id": "CH_MODBUS", "device_id": "motor", "point_id": "run" },
    { "alias": "estop", "channel_id": "CH_MODBUS", "device_id": "safety", "point_id": "estop" }
  ],
  "condition": "intrusion == 1 && running == 1 && estop == 0",
  "state": { "duration": "1s", "count": 2 },
  "actions": [
    { "type": "device_control", "config": { "channel_id": "CH_MODBUS", "device_id": "motor", "point_id": "stop", "value": "0" } },
    {
      "type": "mqtt",
      "config": {
        "mqtt_config_id": "nb-mqtt-main",
        "topic": "edge/safety/interlock",
        "message": "{\"event\":\"stop\",\"intrusion\":${intrusion}}"
      }
    },
    { "type": "log", "config": { "level": "error", "message": "安全联锁触发，设备已停机" } }
  ]
}
```

### 2.4 温湿度越限告警

```json
{
  "name": "温湿度越限告警",
  "type": "threshold",
  "priority": 8,
  "trigger_mode": "on_change",
  "check_interval": "5s",
  "sources": [
    { "alias": "temp", "channel_id": "CH_BACNET", "device_id": "ahu_01", "point_id": "space_temp" },
    { "alias": "humidity", "channel_id": "CH_BACNET", "device_id": "ahu_01", "point_id": "space_rh" }
  ],
  "condition": "temp > 28 || temp < 18 || humidity > 70 || humidity < 30",
  "state": { "duration": "30s" },
  "actions": [
    {
      "type": "mqtt",
      "config": {
        "mqtt_config_id": "nb-mqtt-main",
        "topic": "edge/alarm/env",
        "message": "{\"temp\":${temp},\"humidity\":${humidity}}"
      }
    },
    { "type": "log", "config": { "level": "warn", "message": "环境越限: T=${temp} RH=${humidity}" } }
  ]
}
```

---

## 三、数据聚合

### 3.1 多泵流量汇总 (Calculation)

```json
{
  "name": "多泵流量汇总",
  "type": "calculation",
  "priority": 5,
  "trigger_mode": "always",
  "check_interval": "1s",
  "sources": [
    { "alias": "pump_a", "channel_id": "CH_MODBUS", "device_id": "pump1", "point_id": "flow" },
    { "alias": "pump_b", "channel_id": "CH_MODBUS", "device_id": "pump2", "point_id": "flow" },
    { "alias": "pump_c", "channel_id": "CH_MODBUS", "device_id": "pump3", "point_id": "flow" }
  ],
  "expression": "pump_a + pump_b + pump_c",
  "actions": [
    {
      "type": "device_control",
      "config": {
        "channel_id": "CH_VIRTUAL",
        "device_id": "metrics",
        "point_id": "total_flow",
        "value": "${value}"
      }
    },
    {
      "type": "mqtt",
      "config": {
        "mqtt_config_id": "nb-mqtt-main",
        "topic": "edge/metrics/total_flow",
        "message": "{\"total_flow\":${value}}"
      }
    }
  ]
}
```

### 3.2 多路温度平均值

```json
{
  "name": "多路温度平均值",
  "type": "calculation",
  "check_interval": "5s",
  "sources": [
    { "alias": "t1", "channel_id": "CH_MODBUS", "device_id": "zone1", "point_id": "temp" },
    { "alias": "t2", "channel_id": "CH_MODBUS", "device_id": "zone2", "point_id": "temp" },
    { "alias": "t3", "channel_id": "CH_MODBUS", "device_id": "zone3", "point_id": "temp" }
  ],
  "expression": "(t1 + t2 + t3) / 3",
  "actions": [
    { "type": "log", "config": { "level": "info", "message": "平均温度: ${value}°C" } }
  ]
}
```

### 3.3 振动滑动平均预警 (Window)

```json
{
  "name": "振动均值趋势检测",
  "type": "window",
  "priority": 12,
  "check_interval": "1s",
  "sources": [
    { "alias": "vibration", "channel_id": "CH_MODBUS", "device_id": "bearing", "point_id": "vib_rms" },
    { "alias": "rpm", "channel_id": "CH_MODBUS", "device_id": "motor", "point_id": "speed" }
  ],
  "window": { "size": "60s", "aggr_func": "avg", "interval": "10s" },
  "condition": "value > 5 && rpm > 100",
  "state": { "duration": "10s" },
  "actions": [
    { "type": "log", "config": { "level": "warn", "message": "振动均值 ${value} 超阈值" } },
    {
      "type": "mqtt",
      "config": {
        "mqtt_config_id": "nb-mqtt-main",
        "topic": "edge/pdm/vibration",
        "message": "{\"avg_vib\":${value},\"rpm\":${rpm}}"
      }
    }
  ]
}
```

---

## 四、群控策略

### 4.1 产线顺序启动

```json
{
  "name": "产线启停顺序控制",
  "type": "threshold",
  "priority": 15,
  "trigger_mode": "on_change",
  "check_interval": "1s",
  "sources": [
    { "alias": "start_cmd", "channel_id": "CH_OPCUA", "device_id": "hmi", "point_id": "start" },
    { "alias": "line_ready", "channel_id": "CH_S7", "device_id": "plc", "point_id": "ready" }
  ],
  "condition": "start_cmd == 1 && line_ready == 1",
  "actions": [
    {
      "type": "sequence",
      "config": {
        "steps": [
          { "type": "device_control", "config": { "channel_id": "CH_MODBUS", "device_id": "conv", "point_id": "run", "value": "1" } },
          { "type": "delay", "config": { "duration": "2s" } },
          { "type": "device_control", "config": { "channel_id": "CH_S7", "device_id": "cell", "point_id": "run", "value": "1" } },
          { "type": "delay", "config": { "duration": "2s" } },
          { "type": "device_control", "config": { "channel_id": "CH_MODBUS", "device_id": "pack", "point_id": "run", "value": "1" } },
          { "type": "log", "config": { "level": "info", "message": "产线顺序启动完成" } }
        ]
      }
    }
  ]
}
```

### 4.2 位操作群控（RMW）

写状态字特定位而不覆盖其它位：

```json
{
  "name": "置位运行标志",
  "type": "threshold",
  "condition": "cmd == 1",
  "sources": [
    { "alias": "cmd", "channel_id": "CH_MODBUS", "device_id": "plc", "point_id": "start_cmd" }
  ],
  "actions": [
    {
      "type": "device_control",
      "config": {
        "channel_id": "CH_MODBUS",
        "device_id": "plc",
        "point_id": "status_word",
        "expression": "bitset(v, 3)"
      }
    }
  ]
}
```

---

## 五、故障排查

| 场景 | 检查项 |
|------|--------|
| 联动未执行 | Shadow 点位是否更新；`enable` 是否为 true |
| Sequence 中断 | 查看 `/api/edge/failures` 中失败步骤 |
| Window 无输出 | `GET /api/edge/rules/:id/window` 查看缓冲 |
| MQTT 未发出 | 确认 `mqtt_config_id` 与北向连接状态 |

---

## 相关文档

- [边缘计算规则帮助](边缘计算规则帮助.html)
- [边缘计算最佳实践](../guide/EDGE_COMPUTING_BEST_PRACTICES.html)
- [边缘计算 API](../API/Edge_Computing_CN.html)
