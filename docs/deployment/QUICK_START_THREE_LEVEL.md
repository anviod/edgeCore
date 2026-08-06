---
layout: default
---

# 三级架构快速入门

## 前提条件

1. Go 1.18+
2. BoltDB（自动初始化）
3. 已编译的二进制文件：`./main.exe`

## 快速启动

### 1. 编译

```bash
cd edgeCore
go build ./cmd/main.go -o main.exe
```

### 2. 准备配置文件

使用本文件「配置文件详解」中的 YAML 片段，或通过 Web UI/API 导入；全量快照见 [test/README.md](https://github.com/anviod/edgeCore/blob/dev/test/README.md) 与 `test/legacy/config_full_snapshot.yaml.bak`。参考 [ARCHITECTURE_V2](../architecture/ARCHITECTURE_V2.html)。

### 3. 运行

```bash
# 使用默认配置
./main.exe

# 启动网关（运行时以 data/config.db 为配置源；可经 UI 或导入 YAML 初始化）
./main.exe
```

### 4. 访问 Web UI

打开浏览器访问：`http://localhost:8080`

## API 使用示例

### 获取所有采集通道

```bash
curl http://localhost:8080/api/channels
```

响应示例：
```json
[
  {
    "id": "modbus-tcp-1",
    "name": "Modbus TCP Channel 1",
    "protocol": "modbus-tcp",
    "enable": true,
    "config": {
      "url": "tcp://127.0.0.1:502"
    }
  }
]
```

### 获取通道下的所有设备

```bash
curl http://localhost:8080/api/channels/modbus-tcp-1/devices
```

响应示例：
```json
[
  {
    "id": "device-1",
    "name": "Device 1",
    "enable": true,
    "interval": 5000000000
  },
  {
    "id": "device-2",
    "name": "Device 2",
    "enable": true,
    "interval": 5000000000
  }
]
```

### 获取设备的点位数据

```bash
curl http://localhost:8080/api/channels/modbus-tcp-1/devices/device-1/points
```

响应示例：
```json
[
  {
    "id": "temp",
    "name": "Temperature",
    "address": "40001",
    "datatype": "int16",
    "scale": 0.1,
    "offset": 0,
    "unit": "°C",
    "readwrite": "R"
  },
  {
    "id": "humidity",
    "name": "Humidity",
    "address": "40002",
    "datatype": "int16",
    "scale": 0.1,
    "offset": 0,
    "unit": "%",
    "readwrite": "R"
  }
]
```

## WebSocket 实时数据

连接 WebSocket 端点获取实时的点位数据更新：

```bash
# 使用 wscat 工具
npm install -g wscat
wscat -c ws://localhost:8080/api/ws/values
```

## 配置文件详解

完整的配置文件结构（YAML）：

### 最小配置

```yaml
version: "1.0"

server:
  port: 8080

storage:
  path: "./data/gateway.db"

channels:
  - id: "ch-1"
    name: "Channel 1"
    protocol: "modbus-tcp"
    enable: true
    config:
      url: "tcp://192.168.1.100:502"
    devices:
      - id: "dev-1"
        name: "Device 1"
        enable: true
        interval: 5s
        config:
          slave_id: 1
        points:
          - id: "pt-1"
            name: "Point 1"
            address: "40001"
            datatype: "int16"
            scale: 0.1
            offset: 0
```

### 完整配置

详见 [ARCHITECTURE_V2](../architecture/ARCHITECTURE_V2.html) 中的配置文件格式部分。

## 常见问题

### Q1: 如何添加新的采集通道？

A: 编辑配置文件的 `channels` 数组，添加新的通道配置，然后重启应用。

### Q2: 如何修改采集周期？

A: 修改 `device.interval` 字段，支持 Go 的 `time.Duration` 格式（如 `5s`、`1m`）。

### Q3: 如何支持多个从机？

A: 在同一通道下添加多个 `devices`，每个设备通过 `config.slave_id` 区分。

```yaml
channels:
  - id: "modbus-tcp-1"
    protocol: "modbus-tcp"
    config:
      url: "tcp://192.168.1.100:502"
    devices:
      - id: "device-1"
        config:
          slave_id: 1
        points: [...]
      - id: "device-2"
        config:
          slave_id: 6
        points: [...]
```

### Q4: 如何监控采集状态？

A: 使用 WebSocket 端点 `/api/ws/values` 或查看前端 UI 的设备详情页面。

### Q5: 如何处理采集失败？

A: 系统支持自动重试和失败转移机制（通过状态机管理）。可以通过 API 查询设备状态获取失败信息。

## 文件结构

```
edgeCore/
├── cmd/
│   └── main.go                  # 应用入口
├── internal/
│   ├── config/
│   │   └── config.go            # 配置加载
│   ├── core/
│   │   ├── channel_manager.go   # 通道/设备/点位 CRUD、驱动生命周期
│   │   ├── channel_device_state.go  # 通道-设备联动状态
│   │   ├── scan_engine.go       # 采集调度内核（ScanEngine）
│   │   ├── execution_layer.go   # 执行层（串行/并行/限流）
│   │   ├── resource_controller.go   # 资源控制
│   │   ├── shadow_core.go       # 影子真源（SoT）
│   │   ├── pipeline.go          # 数据管道
│   │   └── node_status.go       # 状态机
│   ├── driver/
│   │   ├── interface.go         # 驱动接口
│   │   └── modbus/
│   │       └── modbus.go        # Modbus 驱动实现
│   ├── model/
│   │   └── types.go             # 数据模型
│   ├── server/
│   │   └── server.go            # Web 服务器
│   └── storage/
│       └── boltdb.go            # BoltDB 存储
├── ui/
│   └── index.html               # Web UI
├── test/README.md                    # 测试目录与 v2 配置参考
├── test/legacy/config_full_snapshot.yaml.bak   # 联调全量 v2 快照（非 schema 模板）
├── test/legacy/config_multi_slave_legacy.yaml  # 2026-01 slaves: 嵌套格式（已废弃）
├── docs/deployment/ARCHITECTURE_V2.md   # 架构文档
└── docs/deployment/QUICK_START_THREE_LEVEL.md   # 本文件
```

## 日志输出示例

```
2026/01/22 08:52:32 Starting Industrial Edge Gateway...
2026/01/22 08:52:32 Channel modbus-tcp-1 added
2026/01/22 08:52:32 ModbusDriver connected to tcp://127.0.0.1:502 (MaxPacketSize: 125, GroupThreshold: 50)
2026/01/22 08:52:32 Channel modbus-tcp-1 started
2026/01/22 08:52:32 Web server starting on :8080
...
```

## 下一步

1. 查看 [ARCHITECTURE_V2](../architecture/ARCHITECTURE_V2.html) 了解详细的架构设计
2. 通过 Web UI/API 或 [产品说明 — 配置结构](../guide/产品说明.html#配置结构) 调整 channels/devices/points
3. 将 UI 更新为使用新的三级 API 端点
4. 测试与实际设备的连接

## 技术支持

- 查看日志文件了解运行状态
- 使用 API 端点查询系统状态
- 检查 WebSocket 连接是否正常

---

**版本信息**
- 架构版本：V2 (三级)
- 更新日期：2026-01-22
- 最后修订：Backend Restructuring Phase
