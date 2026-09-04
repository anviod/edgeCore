---
layout: default
---

# Omron FINS 南向采集驱动

## 概述

edgeCore 通过 `omron-fins` 协议驱动与欧姆龙 PLC 通信，底层使用 [github.com/anviod/fins](https://github.com/anviod/fins) 库，支持 **TCP** 与 **UDP** 两种传输模式。

## 通道配置

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `ip` | string | — | PLC IP 地址（必填） |
| `port` | int | 9600 | FINS 端口 |
| `mode` | string | TCP | 传输模式：`TCP` 或 `UDP` |
| `timeout` | int | 3000 | 通信超时（ms） |
| `max_retries` | int | 3 | 最大重连次数 |
| `heartbeat_interval` | int | 30000 | TCP 心跳间隔（ms） |
| `maxFrameLength` | int | 64 | 单次批量读取最大字数 |
| `min_interval` | int | 0 | 连续指令最小间隔（ms） |
| `src_network_addr` | int | 0 | 源 FINS 网络地址 |
| `src_node_addr` | int | 1 | 源 FINS 节点地址 |
| `src_unit_addr` | int | 255 | 源 FINS 单元地址 |
| `dst_network_addr` | int | 0 | 目标 FINS 网络地址 |
| `dst_node_addr` | int | — | 目标节点（常为 PLC IP 末段） |
| `dst_unit_addr` | int | 0 | 目标 FINS 单元地址 |
| `local_port` | int | 0 | UDP 本地端口（0 为自动） |

### 配置示例（YAML）

```yaml
protocol: omron-fins
config:
  ip: "192.168.1.100"
  port: 9600
  mode: TCP
  timeout: 3000
  dst_node_addr: 100
  maxFrameLength: 64
```

## 点位地址

格式：`AREA ADDRESS[.BIT][.LEN[H|L]]`

| 示例 | 说明 |
|------|------|
| `D100` | D 区字地址 100 |
| `CIO1.2` | CIO 区地址 1 位 2 |
| `W3.4` | W 区位地址 |
| `EM10.100` | EM10 扩展区 |
| `D10.20H` | D 区字符串，长度 20，高字节在前 |

支持数据类型：`BIT`、`INT16`、`UINT16`、`INT32`、`UINT32`、`FLOAT`、`DOUBLE`、`STRING` 等。

## 架构

```
OmronFinsDriver (edgeCore)
    ├── TCP → fins.FinsTCPDriver (anviod/fins)
    └── UDP → udpBackend + udpScheduler (fins/udp + fins.Decoder)
```

TCP 模式直接使用 fins 库的 Transport + Scheduler 实现高性能批量读写；UDP 模式复用 fins 解码器与分组调度逻辑。

## 相关文件

- 驱动实现：`internal/driver/omron/`
- UI 配置：`ui/src/views/ChannelList.vue`
- 帮助文档：`ui/src/components/channel-help/FinsHelp.vue`
