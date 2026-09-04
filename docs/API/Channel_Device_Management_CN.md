---
layout: default
title: 通道与设备管理 (南向) API
description: edgeCore 通道与设备管理 API 文档（中文）
---

# 通道与设备管理 (南向) API

所有端点均需 JWT 认证。

## 通道 (Channels)

### 1. 获取通道列表
*   **URL**: `/channels`
*   **Method**: `GET`
*   **响应**: `Channel` 对象数组。

### 2. 添加通道
*   **URL**: `/channels`
*   **Method**: `POST`
*   **请求体**: `Channel` 对象。
    ```json
    {
      "id": "modbus-01",
      "name": "Modbus TCP Channel",
      "protocol": "modbus-tcp",
      "enable": true,
      "config": {
        "ip": "192.168.1.100",
        "port": 502
      }
    }
    ```

### 3. 获取通道详情
*   **URL**: `/channels/:channelId`
*   **Method**: `GET`

### 4. 更新通道
*   **URL**: `/channels/:channelId`
*   **Method**: `PUT`
*   **请求体**: `Channel` 对象。

### 5. 删除通道
*   **URL**: `/channels/:channelId`
*   **Method**: `DELETE`

### 6. 扫描通道
触发通道上的设备发现（例如 BACnet WhoIs）。

*   **URL**: `/channels/:channelId/scan`
*   **Method**: `POST`
*   **请求体**: (可选) 扫描参数。

## 设备 (Devices)

### 1. 获取通道设备
*   **URL**: `/channels/:channelId/devices`
*   **Method**: `GET`

### 2. 添加设备
支持添加单个设备或设备数组。

*   **URL**: `/channels/:channelId/devices`
*   **Method**: `POST`
*   **请求体**: `Device` 对象 或 `[]Device`。
    ```json
    {
      "name": "Device 1",
      "interval": "1s",
      "enable": true,
      "config": { "slave_id": 1 }
    }
    ```

### 3. 更新设备
*   **URL**: `/channels/:channelId/devices/:deviceId`
*   **Method**: `PUT`
*   **请求体**: `Device` 对象。

### 4. 删除设备
*   **URL**: `/channels/:channelId/devices/:deviceId`
*   **Method**: `DELETE`

### 5. 批量删除设备
*   **URL**: `/channels/:channelId/devices`
*   **Method**: `DELETE`
*   **请求体**: `["deviceId1", "deviceId2"]`

### 6. 获取设备历史数据
查询设备的历史数据。

*   **URL**: `/devices/:deviceId/history`
*   **Method**: `GET`
*   **查询参数**:
    *   `start`: 开始时间 (RFC3339 或 `YYYY-MM-DD HH:mm:ss`)
    *   `end`: 结束时间
    *   `limit`: 记录限制 (默认 100，如果未指定时间范围)

## 点位 (Points)

### 1. 获取设备点位
*   **URL**: `/channels/:channelId/devices/:deviceId/points`
*   **Method**: `GET`

### 2. 添加点位
*   **URL**: `/channels/:channelId/devices/:deviceId/points`
*   **Method**: `POST`
*   **请求体**: `Point` 对象。

### 3. 更新点位
*   **URL**: `/channels/:channelId/devices/:deviceId/points/:pointId`
*   **Method**: `PUT`

### 4. 删除点位
*   **URL**: `/channels/:channelId/devices/:deviceId/points/:pointId`
*   **Method**: `DELETE`

### 5. 写入点位值
向可写点位写入值。

*   **URL**: `/write`
*   **Method**: `POST`
*   **请求体**:
    ```json
    {
      "channel_id": "ch1",
      "device_id": "dev1",
      "point_id": "p1",
      "value": 123
    }
    ```

### 6. 获取实时值
获取内存中所有最新值的快照。

*   **URL**: `/values/realtime`
*   **Method**: `GET`
