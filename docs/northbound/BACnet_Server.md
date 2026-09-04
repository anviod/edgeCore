---
layout: default
title: BACnet Server 北向从机模式
description: edgeCore BACnet Server 北向通道 — 将 edgeCore 南向点位映射为 BACnet 标准对象，对外暴露给 BMS/SCADA 主站
---

# BACnet Server 北向从机模式

## 概述

BACnet Server 北向通道以**从机模式**运行，将 edgeCore 网关南向设备的点位数据映射为 BACnet 标准对象，对外暴露给 BACnet 主站（如 BMS 楼宇管理系统、SCADA 系统）进行监控和写入。

参考 OPC UA Server 的架构模式实现，支持双向读写控制。

## 架构

```
BACnet 主站 (BMS/SCADA)
    │
    │ BACnet/IP (UDP)
    ▼
┌─────────────────────┐
│  BACnet Server      │
│  (从机模式)          │
│  端口: 47808         │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  SouthboundManager   │
│  (南向设备管理)      │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  南向驱动 (BACnet/   │
│  Modbus/OPC UA/...)  │
└─────────────────────┘
```

## 支持的 BACnet 服务

| 服务 | 说明 |
|------|------|
| Who-Is / I-Am | 设备发现与广播应答 |
| ReadProperty | 单属性读取 |
| WriteProperty | 单属性写入（可写点位） |
| ReadPropertyMultiple | 批量属性读取 |
| WritePropertyMultiple | 批量属性写入 |
| SubscribeCOV | COV 变化订阅 |
| COVNotification | COV 变化通知 |

## 点位类型映射

edgeCore 点位根据数据类型和读写属性自动映射为 BACnet 标准对象：

| edgeCore 数据类型 | 读写属性 | BACnet 对象类型 | 编号 |
|--------------|----------|----------------|------|
| float32/float64/int | R | AnalogInput | 0 |
| float32/float64/int | RW | AnalogValue | 2 |
| bool/boolean | R | BinaryInput | 3 |
| bool/boolean | RW | BinaryValue | 5 |
| string | R | MultiStateInput | 13 |
| string | RW | MultiStateValue | 19 |

## 配置参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `name` | string | — | 服务名称（必填） |
| `enable` | bool | false | 是否启用 |
| `interface` | string | — | 绑定的网络接口（留空绑定所有接口） |
| `ip` | string | — | 绑定的 IP 地址 |
| `port` | int | 47808 | BACnet 端口 |
| `subnet_cidr` | int | 24 | 子网 CIDR |
| `device_id` | int | 自动生成 | BACnet 设备实例 ID（0 时 FNV-32a 哈希生成） |
| `device_name` | string | "edgeCore-Gateway" | BACnet 设备名称 |
| `vendor_id` | int | 999 | BACnet 厂商 ID |
| `vendor_name` | string | — | BACnet 厂商名称 |
| `max_pdu` | int | 1476 | 最大 PDU 大小 |
| `devices` | object | {} | 真实设备映射（空表示全部暴露） |
| `virtual_devices` | object | {} | 虚拟设备映射（空表示全部暴露） |

## 设备映射

通过 `devices` 和 `virtual_devices` 字段控制哪些设备暴露给 BACnet 主站：

- **空 map（默认）**：所有设备全部暴露
- **指定设备**：`{"deviceID": {"enable": true}}` 暴露指定设备，`{"deviceID": {"enable": false}}` 隐藏指定设备

## 热更新 vs 重启

| 变更类型 | 行为 |
|----------|------|
| 设备/点位映射变更 | 热更新（仅重建地址空间，不停 UDP 监听） |
| IP/Port/DeviceID/MaxPDU/SubnetCIDR 变更 | 完整重启 |

## 配置示例

### 基本配置

```json
{
  "name": "BACnet 北向从机",
  "enable": true,
  "port": 47810,
  "device_id": 47810,
  "device_name": "edgeCore-Gateway",
  "vendor_id": 999
}
```

### 选择性暴露设备

```json
{
  "name": "BACnet 北向从机",
  "enable": true,
  "port": 47810,
  "device_id": 47810,
  "devices": {
    "2228316": { "enable": true },
    "2228317": { "enable": true },
    "2228318": { "enable": false },
    "2228319": { "enable": false }
  }
}
```

## 接入 BACnet 主站

在 Yabe 或其他 BACnet 客户端中：

1. 添加设备：输入 edgeCore 网关的 IP 地址和 BACnet Server 端口
2. 或发送 Who-Is 广播：BACnet Server 会自动回复 I-Am 完成设备发现
3. 浏览 Object List：查看所有映射的 BACnet 对象
4. 读取 PresentValue：获取实时点位数据
5. 写入 PresentValue：下发控制指令到南向设备（仅可写点位）

## 写入历史

BACnet Server 保留最近 100 条外部写入记录，包含：

- 时间戳
- 通道/设备/点位 ID
- 写入值
- 成功/失败状态
- 错误信息（如有）

通过 API `GET /api/northbound/bacnet/:id/history?limit=100` 查询。

## 运行时统计

```json
{
  "object_count": 147,
  "point_count": 147,
  "write_count": 0,
  "update_count": 1234,
  "last_write_time": "2026-07-21T10:00:00Z",
  "start_time": "2026-07-21T09:00:00Z"
}
```

## 相关资源

- [北向配置 API](../API/Northbound_Configuration_CN.html) — REST API 文档
- [北向数据索引](index.html) — 所有北向通道文档
- [BACnet 驱动文档](../drivers/BACnet_设计说明.html) — 南向 BACnet 驱动
- [架构设计总览](../edge/边缘网关架构设计总览.html)