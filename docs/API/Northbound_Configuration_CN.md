---
layout: default
title: 北向配置 API
description: edgeCore 北向配置 API 文档（中文）
---

# 北向配置 API (Northbound Configuration)

所有端点均需 JWT 认证。

## 支持协议

| 协议 | 端点和配置 | 数据类型 |
|------|-----------|---------|
| MQTT | `/northbound/mqtt` | `MQTTConfig` |
| Sparkplug B | `/northbound/sparkplugb` | `SparkplugBConfig` |
| OPC UA Server | `/northbound/opcua` | `OPCUAConfig` |
| BACnet Server | `/northbound/bacnet` | `BACnetServerConfig` |
| HTTP | `/northbound/http` | `HTTPConfig` |
| EdgeOS | `/northbound/edgeos` | `EdgeOSConfig` |

## 1. 获取配置
获取所有北向配置（MQTT, OPC UA, SparkplugB, HTTP, BACnet Server, EdgeOS）。

*   **URL**: `/northbound/config`
*   **Method**: `GET`
*   **响应**: `NorthboundConfig` 对象。

## 2. 更新 MQTT 配置
创建或更新 MQTT 客户端配置。

*   **URL**: `/northbound/mqtt`
*   **Method**: `POST`
*   **请求体**: `MQTTConfig` 对象。

### 关键特性配置
*   **离线缓存 (Offline Cache)**:
    ```json
    "cache": {
      "enable": true,
      "max_count": 1000,
      "flush_interval": "1m"
    }
    ```
    *   启用后，当连接断开或发送失败时，数据将持久化到本地数据库 (bboltDB)。
    *   恢复连接后，按 FIFO 顺序重发，**发送成功后删除本地缓存**。

*   **事件上报 (Events)**:
    *   `device_status_topic`: 子设备上下线状态 (Payload: `{"event":"status", "status":"online" ...}`)
    *   `device_lifecycle_topic`: 子设备添加/移除事件 (Payload: `{"event":"add", "details":{...}}`)
    *   **触发机制**:
        *   **添加/移除**: 保存配置时，自动比较新旧配置的设备映射列表，差异部分触发事件。
        *   **上下线**: 实时监听设备连接状态变化触发。

## 3. 更新 HTTP 配置
创建或更新 HTTP 推送配置。

*   **URL**: `/northbound/http`
*   **Method**: `POST`
*   **请求体**: `HTTPConfig` 对象。
    ```json
    {
      "id": "http-01",
      "enable": true,
      "url": "http://remote-server:8080",
      "method": "POST",
      "data_endpoint": "/api/data",
      "device_event_endpoint": "/api/events",
      "headers": { "Authorization": "Bearer token" },
      "cache": { "enable": true, "max_count": 1000 }
    }
    ```

## 4. 删除 HTTP 配置
*   **URL**: `/northbound/http/:id`
*   **Method**: `DELETE`

## 5. 更新 OPC UA 配置
创建或更新 OPC UA 服务端配置。

*   **URL**: `/northbound/opcua`
*   **Method**: `POST`
*   **请求体**: `OPCUAConfig` 对象。

## 6. 获取运行时统计
*   MQTT: `/northbound/mqtt/:id/stats`
*   OPC UA: `/northbound/opcua/:id/stats`
*   BACnet Server: `/northbound/bacnet/:id/stats`

## 7. 更新 BACnet Server 配置
创建或更新 BACnet Server 从机模式配置。

*   **URL**: `/northbound/bacnet`
*   **Method**: `POST`
*   **请求体**: `BACnetServerConfig` 对象。

### 关键字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 配置唯一标识（UUID） |
| `name` | string | 服务名称 |
| `enable` | bool | 是否启用 |
| `interface` | string | 绑定的网络接口（留空绑定所有接口） |
| `ip` | string | 绑定的 IP 地址 |
| `port` | int | BACnet 端口（默认 47808） |
| `subnet_cidr` | int | 子网 CIDR（默认 24） |
| `device_id` | int | BACnet 设备实例 ID（0 时自动生成，范围 1000-4194303） |
| `device_name` | string | BACnet 设备名称 |
| `vendor_id` | int | BACnet 厂商 ID（默认 999） |
| `vendor_name` | string | BACnet 厂商名称 |
| `max_pdu` | int | 最大 PDU 大小（默认 1476） |
| `devices` | object | 真实设备映射（`{deviceID: {enable: bool}}`） |
| `virtual_devices` | object | 虚拟设备映射（`{deviceID: {enable: bool}}`） |

### 请求示例

```json
{
  "id": "bacnet-server-001",
  "name": "BACnet 北向从机",
  "enable": true,
  "port": 47810,
  "device_id": 47810,
  "device_name": "edgeCore-Gateway",
  "vendor_id": 999,
  "devices": {
    "2228316": { "enable": true },
    "2228317": { "enable": true }
  }
}
```

### 运行时统计

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

### 写入历史

*   **URL**: `/northbound/bacnet/:id/history`
*   **Method**: `GET`
*   **Query**: `?limit=100`（默认 100，最多 100 条）
*   **响应**: 最近的外部写入历史记录数组。

## 8. 删除 BACnet Server 配置
*   **URL**: `/northbound/bacnet/:id`
*   **Method**: `DELETE`
