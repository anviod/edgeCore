---
layout: default
title: MQTT数据上下行格式
description: edgeCore MQTT数据上下行格式
---

# MQTT 数据上下行格式

> **UI 接入说明：** 北向 MQTT 配置页 Help Dialog 为**精简接入示例**（默认 Values-format 上报 + 读写/状态主题），完整字段定义、多格式选项与自定义变量以**本文档**为准。
>
> **EdgeOS 专用协议：** edgeCore ↔ edgeOS 蜂群 Topic/消息体（节点注册、设备上报、下行控制）见 [edgeCore 通信协议规范](../edgeos/edgeCore通信协议规范%28MQTT-NATS%29.html)，与本文 scope 不同。

数据上下行格式支持 Neuron 兼容方式上报。以下内容描述 MQTT 插件如何上报采集数据，以及如何通过 MQTT 实现读写点位。

## 通用约定

- 采集**成功**时返回数据值；**失败**时返回错误码，不再返回值。
- 配置项「上报点位错误码」为 `False` 时，不上报错误码（Custom 格式中的 `${tag_errors}` / `${tag_error_values}` 亦无效）。
- ECP-format **不支持**错误码上报。

---

## 数据上报

MQTT 插件将采集数据以 JSON 发布到北向节点订阅主题（默认 `/things/{MQTT driver name}`）。格式由「上报数据格式」参数指定。

### Values-format（默认）

| 字段 | 说明 |
| --- | --- |
| `timestamp` | 采集 UNIX 时间戳 |
| `node` | 南向驱动名称 |
| `group` | 采集组名称 |
| `values` | 采集成功的点位值字典 |
| `errors` | 采集失败的错误码字典 |
| `metas` | 驱动元数据 |

```json
{
  "timestamp": 1650006388943,
  "node": "modbus",
  "group": "grp",
  "values": { "tag0": 123 },
  "errors": { "tag1": 2014 },
  "metas": {}
}
```

### Tags-format

`tags` 为数组，每项含 `name` + `value` 或 `name` + `error`：

```json
{
  "timestamp": 1647497389075,
  "node": "modbus",
  "group": "grp",
  "tags": [
    { "name": "tag0", "value": 123 },
    { "name": "tag1", "error": 2014 }
  ]
}
```

### ECP-format

`tags` 数组每项含 `name`、`value`、`type`（1=布尔，2=整型，3=浮点，4=字符串）。仅上报成功点位。

### Custom 自定义格式

支持内置变量：`${timestamp}`、`${node}`、`${group}`、`${tags}`、`${tag_values}`、`${tag_errors}`、`${tag_error_values}`、`${static_tags}`、`${static_tag_values}`。

> `${tags}` 与 `${tag_values}` 二选一；静态点位变量同理。

示例（数组格式 + 自定义键名）：

```json
{
  "timestamp": "${timestamp}",
  "node": "${node}",
  "group": "${group}",
  "custom_tag_name": "${tags}",
  "custom_tag_errors": "${tag_errors}"
}
```

更多 Custom 嵌套与网关静态信息示例见 UI Help「数据上报」Tab 中的文档链接。

### 静态点位

在北向应用组列表为采集组配置标准 JSON 静态属性（如 `location`、`sn_number`），随上报格式一并携带。命名不得与南向采集点位重复（Values-format 下会覆盖同名采集值）。

---

## 读 Tags

**请求主题：** `/things/{node_name}/read/req`

```json
{
  "uuid": "bca54fe7-a2b1-43e2-a4b4-1da715d28eab",
  "node": "modbus",
  "group": "grp"
}
```

**响应主题：** `/things/{node_name}/read/resp`

```json
{
  "uuid": "bca54fe7-a2b1-43e2-a4b4-1da715d28eab",
  "tags": [
    { "name": "tag0", "value": 4 },
    { "name": "tag1", "error": 2014 }
  ]
}
```

---

## 写 Tag

**请求主题：** 配置项「写请求主题」，默认 `/things/{random_str}/write/req`

单点写入：

```json
{
  "uuid": "cd32be1b-c8b1-3257-94af-77f847b1ed3e",
  "node": "modbus",
  "group": "grp",
  "tag": "tag0",
  "value": 1234
}
```

多点写入（`tags` 数组）：

```json
{
  "uuid": "cd32be1b-c8b1-3257-94af-77f847b1ed3e",
  "node": "modbus",
  "group": "grp",
  "tags": [
    { "tag": "tag0", "value": 1234 },
    { "tag": "tag1", "value": 5678 }
  ]
}
```

**响应主题：** 写响应主题（Write Response Topic），未配置时在写请求主题后追加 `/resp`。

```json
{
  "uuid": "cd32be1b-c8b1-3257-94af-77f847b1ed3e",
  "success": true
}
```

---

## 驱动状态上报

**主题：** 北向节点配置（默认 `/things/{random_str}/state/update`）  
**间隔：** 1–3600 秒（默认 1）

```json
{
  "timestamp": 1658134132237,
  "states": [
    { "node": "modbus-tcp", "link": 1, "running": 3 },
    { "node": "modbus-rtu", "link": 1, "running": 3 }
  ]
}
```

---

## 设备状态通知

设备上线/离线时发布状态消息，支持 LWT（遗嘱）。

- **状态主题：** 配置项 Status Topic，默认 `{topic}/status`，支持变量替换。
- **载荷模板：** 可自定义 Online / Offline JSON。

默认载荷：

```json
{
  "status": "online",
  "timestamp": 1658134132237,
  "device_id": "device1"
}
```

**变量替换：** `{device_id}`、`{device_name}`、`{status}`、`{timestamp}`
