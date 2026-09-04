---
layout: default
title: API 文档：BACnet 模块
description: edgeCore BACnet API 文档
---

# API 文档：BACnet 模块

## 设备点位

- `GET /api/channels/:channelId/devices/:deviceId/points`
  - 即时返回快照数据

## 实时值

- `GET /api/values/realtime`
- `GET /api/ws/values` WebSocket 广播

## 北向 OPC UA

- 见 `internal/northbound/opcua/server.go`

## 统一数据模型

见《BACnet_设计说明.md》

