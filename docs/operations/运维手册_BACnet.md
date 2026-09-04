---
layout: default
title: 运维手册：BACnet 模块
description: edgeCore BACnet 运维手册
---

# 运维手册：BACnet 模块

## 启动与验证

1. 启动服务后观察日志中 `Driver connected for channel` 与 `Channel started`。
2. 访问 `/api/channels` 查看通道状态；设备应逐步转为 Online。
3. 进入 Web UI → 设备 → 点位页，确认数据刷新。

## 常见问题

- 点位超时：后端 API 直接读取快照；若值长期不变，请检查物理网络或设备。
- 设备离线：驱动将自动恢复；必要时检查网段广播与 47808 端口策略。
- 值异常：确认点位地址格式 `Type:Instance[:Property]` 是否正确。

## 日志定位

- 采集：`internal/core/channel_manager.go`
- 驱动：`internal/driver/bacnet/*`
- 北向：`internal/northbound/*`

## 回滚

见《回滚方案.md》，使用上一版本二进制覆盖并重启。

