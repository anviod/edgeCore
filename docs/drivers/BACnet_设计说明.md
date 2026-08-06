---
layout: default
title: BACnet/IP 通信模块设计说明
description: edgeCore BACnet 设计说明
---

# BACnet/IP 通信模块设计说明

> **依赖版本**: `github.com/anviod/bacnet@v0.0.6+`（Windows 需此版本以获得 SO_BROADCAST 支持）

## 架构概览

- 南向驱动：BACnetDriver（设备发现、对象扫描、读写、恢复）
- 调度器：PointScheduler（批量读、分批回退、失败冷却）
- 数据管线：统一 Value 模型下发到存储、WebSocket、OPC UA、MQTT 等
- 快照缓存：ChannelManager snapshots 提供 API 即时返回

## 设备发现（Yabe 风格三步法）

BACnet 设备发现严格遵循 Yabe 最佳实践，简化冗余逻辑：

1. **绑定 INADDR_ANY**：`ClientBuilder{Ip: "0.0.0.0", Port: 47808}` — 绑定所有物理网卡，确保广播可覆盖全部子网
2. **广播 WhoIs（无范围限制）**：`WhoIsOpts{Low: 0, High: 0}` — 0 表示不限制设备 ID 范围，匹配 Yabe 约定
3. **收集 IAm + 富化名称**：通过 `ReadProperty(Object_Name)` 获取设备名称

**关键实现细节**：
- 使用独立临时 Client（非复用 driver client），避免阻塞读写操作
- interface_ip 默认为 `0.0.0.0`（空字符串自动归一化），确保所有物理网卡广播
- 必须使用标准 BACnet 发现端口 47808（设备只响应此端口的 WhoIs）
- Windows 平台必须设置 `SO_BROADCAST` 套接字选项（v0.0.6+ 自动处理）

## 三种读取模式

- **手动添加（Manual-Add）**：现场工程推荐方式。用户提供 DeviceID + IP + Port，驱动通过 ReadProperty(Object_Name) 验证可达性（超时 10s），验证通过后直接注册设备，跳过 WhoIs 发现流程。
- **WhoIs 扫描（Supplement）**：端口未知时的补充手段。使用 Yabe 风格三步法进行全网广播发现。
- **轮询**：ReadPropertyMultiple 优先，失败回退单点 ReadProperty；并发隔离
- **订阅**：配置 report_mode=cov 的点位优先尝试订阅；不支持则回退为轮询

## 采集验证流程（4-Phase）

| Phase | 描述 | 验收标准 | 状态 |
|-------|------|---------|------|
| Phase 1 | 广播 WhoIs | 4/4 设备发现 (2228316-2228319) | ✅ |
| Phase 2 | 对象扫描 | 4/4 设备成功，每设备 11 个对象 | ✅ |
| Phase 3 | 点位读取 | 全部点位读取成功，0 失败 | ✅ |
| Phase 4 | 可写点写入 | 可写点 100% 成功，只读点跳过 | ✅ |

## 统一数据模型

```json
{
  "channel_id": "CH-1",
  "device_id": "bacnet-18",
  "instance_id": 18,
  "point_id": "AI1",
  "value": 23.5,
  "quality": "Good",
  "timestamp": "2026-02-28T12:00:00Z",
  "meta": {
    "objectType": 0,
    "objectId": 1,
    "propertyId": 85,
    "statusFlags": null
  }
}
```

## 可靠性与恢复

- 设备级超时隔离：每设备独立 3s 超时，不影响其他设备
- 离线判定与冻结：失败退化为 DEGRADED，连续失败置 OFFLINE 并冻结调度
- 恢复流程：周期触发 Who-Is，恢复后自动解冻并重建调度器
- 恢复前检查隔离期：避免快速 WhoIs 请求循环

## 性能优化

- 批量读取：默认分组阈值 20；避免 APDU 过大
- MaxPDU：ClientBuilder 设置 MaxPDU=1476 (MaxAPDU)，避免分包开销
- 并行化：设备级并发，互不阻塞
- 快照返回：API 从快照返回，UI 无阻塞

## 锁优化

- `WritePoint`/`ReadPoint`/`GetDevicePoints` 锁范围仅限数据查找，网络 I/O 在锁外执行
- `ScanChannel` 使用独立 Client 进行网络广播 I/O，避免阻塞读写操作
- `connectOnce` 三步流程：加锁关闭旧连接 → 解锁创建新连接 → 加锁保存新连接
- `saveChannels()` 在 goroutine 中异步执行磁盘 I/O，避免阻塞 API 读路径

## 对外暴露

- JSON：REST 与 WebSocket 广播统一 Value JSON
- OPC UA：北向 OPC UA Server 动态映射 Channel/Device/Point，实时更新

## 安全与质量

- 无敏感配置暴露；证书信任目录与可选认证
- 通过单元与集成测试；遵循项目编码规范

