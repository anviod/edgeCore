---
layout: default
title: 系统管理 API
description: edgeCore 系统管理 API 文档（中文）
---

# 系统管理 API (System Management)

所有端点均需 JWT 认证。

## 1. 仪表盘概览
获取系统状态、通道状态、北向状态及资源使用概览。

*   **URL**: `/dashboard/summary`
*   **Method**: `GET`

**响应**:
```json
{
  "channels": [ ... ],
  "northbound": [ ... ],
  "edge_rules": { ... },
  "system": {
    "cpu_usage": 12.5,
    "memory_usage": 512,
    "disk_usage": 45.5,
    "goroutines": 120
  }
}
```

## 2. 获取系统配置
获取完整系统配置，包括网络、时间、HA 和 LDAP 设置。

*   **URL**: `/system`
*   **Method**: `GET`

**响应**: `SystemConfig` 对象 (参考 `internal/model/system.go`)。

## 3. 更新系统配置
更新系统配置。

*   **URL**: `/system`
*   **Method**: `PUT`

**请求体**: `SystemConfig` 对象。

**响应**:
```json
{
  "status": "success",
  "message": "System configuration updated"
}
```

## 4. 重启系统
触发系统重启（网关进程退出）。

*   **URL**: `/system/restart`
*   **Method**: `POST`

**响应**:
```json
{
  "status": "success",
  "message": "System is restarting..."
}
```

## 5. 网络接口
获取可用网络接口列表。

*   **URL**: `/system/network/interfaces`
*   **Method**: `GET`

**响应**: `NetworkInterface` 对象数组。

## 6. 网络路由
获取静态路由列表。

*   **URL**: `/system/network/routes`
*   **Method**: `GET`

**响应**: `StaticRoute` 对象数组。
