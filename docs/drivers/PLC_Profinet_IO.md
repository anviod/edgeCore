# Profinet IO 采集驱动

> 规划与扩展项见 [TODO/Profinet IO 采集驱动](../TODO/Profinet%20IO/采集驱动Profinet%20IO.html)。

edgeCore 南向 PROFINET IO 驱动，作为 IO 控制器通过 TCP（默认端口 34964）对 IO 设备进行非循环读写。

## 架构

```text
Channel (profinet-io)
  → local_interface, timeout, simulation
  → Device (ip, port, slot, subslot, device_name, ...)
    → Point (SLOT:SUB_SLOT:INDEX[.BIT][#ENDIAN])
  → ProfinetTransport (TCP + RPC)
  → ProfinetScheduler (ReadPoints / WritePoint)
```

## 协议注册

- 注册名：`profinet-io`
- 包路径：`internal/driver/profinetio/`
- 扫描引擎类型：`ProtocolTypeLimited`（单通道共享连接，互斥访问）

## 通道配置

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `local_interface` | 本地网卡名（如 eth0） | — |
| `timeout` | 超时 (ms) | 3000 |
| `max_retries` | 重试次数 | 3 |
| `heartbeat_interval` | 心跳间隔 (ms) | 30000 |
| `simulation` | 模拟模式（无真实设备） | false |

## 设备配置

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `device_name` | IO 设备名称 | — |
| `ip` | 设备 IPv4 地址 | — |
| `port` | 设备端口 | 34964 |
| `api` | API 列表 | — |
| `slot` | 模块槽号 | 0 |
| `subslot` | 子槽号 | 1 |
| `ident` | 模块标识 | — |
| `sub_ident` | 模组标识 | — |
| `properties` | 模组属性 | — |
| `input_length` | 输入数据长度（字节） | 0 |
| `output_length` | 输出数据长度（字节） | 0 |

## 点位地址

格式：`SLOT:SUB_SLOT:INDEX[.BIT][#ENDIAN]`

| 地址 | 数据类型 | 说明 |
|------|----------|------|
| 3:1:0 | int16 | 槽 3 子槽 1 第 0,1 字节 |
| 3:1:1 | uint16 | 槽 3 子槽 1 第 1,2 字节 |
| 3:2:3 | uint32 | 槽 3 子槽 2 第 3–6 字节 |
| 3:2:10 | float | 槽 3 子槽 2 第 10–13 字节 |
| 3:2:5.3 | bit | 槽 3 子槽 2 第 5 字节第 3 位 |

支持数据类型：INT8、UINT8、INT16、UINT16、INT32、UINT32、INT64、UINT64、FLOAT、DOUBLE、BIT

## 部署限制

PROFINET IO 实时报文基于以太网帧传输，需将 edgeCore 部署在物理设备上并绑定真实网卡。不建议在 Docker 镜像或虚拟机中使用。

## 前端集成清单

- [x] `protocolLabel.js` — 协议列表
- [x] `ChannelList.vue` — 通道配置表单
- [x] `DeviceList.vue` — 设备配置表单
- [x] `PointList.vue` — 地址标签/提示
- [x] `channelDefaultConfig.js` — 默认配置
- [x] `ProfinetHelp.vue` — 帮助文档
- [x] `Dashboard.vue` / CSS — 协议图标

## 测试

单元测试（11 项，覆盖率 ~49%）：地址解析、编解码、仿真模式读写、`scenario_test.go` 边界场景（无效地址、重连指标、并发读、ConnectionManager 退避）。编解码基准见 `decoder_benchmark_test.go`。

```bash
CGO_ENABLED=0 go test ./internal/driver/profinetio/... -count=1 -v
CGO_ENABLED=0 go test -bench=. -benchmem ./internal/driver/profinetio -run=^$ -count=1
CGO_ENABLED=0 go test ./internal/integration/... -count=1 -v -run Profinet
```

模拟模式测试无需真实 PROFINET 设备：

```json
{
  "protocol": "profinet-io",
  "config": { "simulation": true, "local_interface": "eth0" }
}
```
