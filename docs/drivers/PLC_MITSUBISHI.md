---
layout: default
---

# 三菱 MC Protocol 南向采集驱动

## 概述

edgeCore 通过 `mitsubishi-slmp` 协议驱动与三菱 PLC 通信，实现 **MC Protocol 3E 二进制帧**（TCP），适用于 Q/L/iQ-R 系列等支持 SLMP / MELSEC Communication Protocol 的 CPU。

## 通道配置

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `ip` | string | — | PLC IP 地址（必填） |
| `port` | int | 5000 | MC Protocol 端口（iQ-R SLMP 常用 5007） |
| `frame_type` | string | 3E | 帧类型，当前支持 3E |
| `network_no` | int | 0 | 网络编号 |
| `pc_no` | int | 255 | PC 编号 (0xFF) |
| `station_no` | int | 0 | 目标站号 |
| `timeout` | int | 3000 | 通信超时（ms） |
| `max_retries` | int | 2 | TCP 连接重试次数 |
| `batch_read_max` | int | 64 | 调度分组上限 |

### 配置示例（YAML）

```yaml
protocol: mitsubishi-slmp
config:
  ip: "192.168.1.10"
  port: 5000
  frame_type: "3E"
  network_no: 0
  station_no: 0
  timeout: 3000
  batch_read_max: 64
```

## 点位地址

格式：`AREA ADDRESS[.BIT][.LEN[H|L]]`

| 示例 | 说明 |
|------|------|
| `D100` | D 区字地址 100 |
| `M0` | M 区位地址 0 |
| `X0` / `Y10` | 输入/输出继电器 |
| `D20.2` | D20 字的第 2 位 |
| `D100.16L` | D100 起 16 字节字符串，低字节在前 |
| `W50` | W 区链接寄存器 |

支持数据类型：`BIT`/`BOOL`、`INT16`、`UINT16`、`INT32`、`UINT32`、`FLOAT`、`DOUBLE`、`STRING` 等。

## 架构

```
MitsubishiDriver (edgeCore)
    ├── MCTransport   — TCP 连接、3E 帧收发
    ├── MCDecoder     — 地址解析、数值编解码
    └── MCScheduler   — 按设备区分组调度读写
```

## PLC 侧设置

1. 在 GX Works2 / GX Works3 中启用以太网 MC Protocol / SLMP 通信。
2. 确认 TCP 端口（默认 5000 或 5007）与通道配置一致。
3. 允许外部设备访问目标设备区（D/M/X/Y 等）。

## 相关文件

- 驱动实现：`internal/driver/mitsubishi/`
- UI 配置：`ui/src/views/ChannelList.vue`
- 帮助文档：`ui/src/components/channel-help/MitsubishiHelp.vue`

## 后续增强

- 4E 帧与 UDP 传输
- 随机读（0403）批量合并优化
- 读-改-写支持 D 区字内位写入
