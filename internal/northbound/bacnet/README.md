# BACnet Server 北向通道

## 概述

BACnet Server 北向通道以**从机模式**运行，将 edgeCore 网关南向设备的点位数据映射为 BACnet 标准对象，对外暴露给 BACnet 主站（如 BMS 楼宇管理系统、SCADA 系统）进行监控和写入。

### 参考实现

本模块参考 OPC UA Server (`internal/northbound/opcua/`) 的架构模式实现，遵循相同的生命周期管理、数据流处理和热更新机制。

## 架构

```
                      BACnet 主站 (BMS/SCADA)
                              │
                     Who-Is / ReadProperty
                     WriteProperty / COV
                              │
                              ▼
┌──────────────────────────────────────────────┐
│              BACnet Server (从机)             │
│  ┌────────────────────────────────────────┐  │
│  │         server.Server (anviod/bacnet)  │  │
│  │  ┌──────────────────────────────────┐  │  │
│  │  │  WhoIs → IAm                     │  │  │
│  │  │  ReadProperty → 返回点位值       │  │  │
│  │  │  WriteProperty → 写回南向设备    │  │  │
│  │  │  ReadPropertyMultiple            │  │  │
│  │  │  WritePropertyMultiple           │  │  │
│  │  │  SubscribeCOV / COVNotification  │  │  │
│  │  └──────────────────────────────────┘  │  │
│  └────────────────────────────────────────┘  │
│                      │                       │
│            pointMap (点位映射)                │
│                      │                       │
│          SouthboundManager (南向管理)         │
└──────────────────────────────────────────────┘
                      │
          ┌───────────┼───────────┐
          ▼           ▼           ▼
      BACnet       Modbus      OPC UA
      Driver       Driver      Driver
```

## 点位类型映射

edgeCore 点位 DataType 到 BACnet 对象类型的映射规则：

| edgeCore DataType | ReadWrite | BACnet 对象类型    | 对象类型编号 |
|---------------|-----------|-------------------|------------|
| float32/float64/int | R   | AnalogInput       | 0          |
| float32/float64/int | RW  | AnalogValue       | 2          |
| bool/boolean       | R    | BinaryInput       | 3          |
| bool/boolean       | RW   | BinaryValue       | 5          |
| string             | R    | MultiStateInput   | 13         |
| string             | RW   | MultiStateValue   | 19         |

## 配置

### 配置结构体

```go
type BACnetServerConfig struct {
    ID             string         // 唯一标识符
    Name           string         // 显示名称
    Enable         bool           // 是否启用
    Interface      string         // 网络接口名 (如 eth0)，空则自动选择
    IP             string         // 绑定的 IP 地址，空则自动选择
    Port           int            // BACnet 端口，默认 47808 (0xBAC0)
    SubnetCIDR     int            // 子网 CIDR，默认 24
    DeviceID       int            // BACnet 设备实例 ID，默认自动生成
    DeviceName     string         // BACnet 设备名称
    VendorID       uint32         // 厂商 ID，默认 999
    VendorName     string         // 厂商名称
    MaxPDU         uint16         // 最大 PDU 大小，默认 1476
    Devices        OpcUaDeviceMap // 暴露的设备列表，空则全部暴露
    VirtualDevices OpcUaDeviceMap // 虚拟设备
}
```

### JSON 配置示例

```json
{
  "bacnet_server": [
    {
      "id": "bacnet-srv-1",
      "name": "BACnet Server 1",
      "enable": true,
      "port": 47808,
      "device_id": 1000,
      "device_name": "edgeCore-Gateway",
      "vendor_id": 999,
      "vendor_name": "edgeCore Foundry",
      "max_pdu": 1476,
      "devices": {
        "dev1": { "enable": true },
        "dev2": { "enable": true }
      }
    }
  ]
}
```

## 支持的 BACnet 服务

| 服务 | 类型 | 说明 |
|------|------|------|
| Who-Is / I-Am | 无确认 | 设备发现，主站发送 Who-Is 广播，Server 回复 I-Am |
| ReadProperty | 确认 | 读取单个对象属性（含 PresentValue） |
| WriteProperty | 确认 | 写入单个对象属性，值会转发到南向设备 |
| ReadPropertyMultiple | 确认 | 批量读取多个对象属性 |
| WritePropertyMultiple | 确认 | 批量写入多个对象属性 |
| SubscribeCOV | 确认 | COV 订阅，点位值变化时自动通知 |
| COVNotification | 确认/无确认 | COV 变更通知 |

## API 接口

### 生命周期管理

| 方法 | 说明 |
|------|------|
| `NewServer(cfg, sb)` | 创建 Server 实例 |
| `Start() error` | 启动 Server（UDP 监听 + 地址空间构建） |
| `Stop()` | 停止 Server（可安全多次调用） |
| `UpdateConfig(cfg) error` | 更新配置（结构性变更自动重启） |
| `SyncAddressSpace() error` | 热更新地址空间（不停 UDP 监听器） |
| `IsRunning() bool` | 检查运行状态 |

### 数据流

| 方法 | 说明 |
|------|------|
| `Update(v model.Value)` | 从数据管道接收南向数据更新，写入 BACnet PresentValue |
| `WriteViaBACnet(ch, dev, pt, value) error` | 外部 BACnet 写入请求，转发到南向设备 |

### 统计与诊断

| 方法 | 说明 |
|------|------|
| `GetStats() Stats` | 获取运行统计（对象数、写入次数、更新次数） |
| `GetWriteHistory(limit) []WriteHistoryItem` | 获取写入历史（最近 100 条） |

## 使用示例

### 基本使用

```go
import (
    "github.com/anviod/edgeCore/internal/model"
    "github.com/anviod/edgeCore/internal/northbound/bacnet"
)

cfg := model.BACnetServerConfig{
    ID:         "bacnet-1",
    Name:       "My BACnet Server",
    Enable:     true,
    Port:       47808,
    DeviceID:   1000,
    DeviceName: "edgeCore-Gateway",
}

srv := bacnet.NewServer(cfg, southboundManager)
if err := srv.Start(); err != nil {
    log.Fatal(err)
}
defer srv.Stop()

// Server 已启动，监听 BACnet 请求
```

### 通过 NorthboundManager 管理

```go
// 通过 NorthboundManager 的 UpsertBACnetServerConfig 管理
warning, err := northboundManager.UpsertBACnetServerConfig(cfg)

// 同步地址空间（热更新）
err := northboundManager.SyncBACnetServer("bacnet-1")

// 获取统计
stats, err := northboundManager.GetBACnetServerStats("bacnet-1")
```

## 设计决策

### 1. 端口选择

默认使用 BACnet 标准端口 47808 (0xBAC0)。与南向 BACnet 驱动使用 47809 端口分离，避免端口冲突。

### 2. 设备 ID 生成

当未指定 DeviceID 时，使用 FNV-32a 哈希算法根据 Name 生成确定性的设备实例 ID（范围 1000-4194303），确保相同配置产生相同 ID。

### 3. 热更新 vs 重启

- **热更新**：设备/点位映射变更时，仅重建地址空间，不停止 UDP 监听器
- **重启**：IP/Port/DeviceID/MaxPDU/SubnetCIDR 变更时，需要完整重启

### 4. 值类型转换

所有 edgeCore 值在写入 BACnet 对象前根据对象类型进行转换：
- AnalogInput/AnalogValue → float64
- BinaryInput/BinaryValue → bool
- MultiStateInput/MultiStateValue → uint32

### 5. 写入历史

保留最近 100 条外部写入记录，用于调试和审计。

## 文件结构

```
internal/northbound/bacnet/
├── server.go       # BACnet Server 核心实现
├── server_test.go  # 单元测试 (16 个测试用例)
└── README.md       # 本文档
```

## 依赖

- `github.com/anviod/bacnet v0.0.5` — BACnet 协议库（server 子包提供完整 BACnet 服务端实现）