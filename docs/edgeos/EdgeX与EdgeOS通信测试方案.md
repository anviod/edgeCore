---
layout: default
title: EdgeX-EdgeOS MQTT/NATS 通信测试方案
description: EdgeX EdgeX-EdgeOS MQTT/NATS 通信测试方案
---

# EdgeX-EdgeOS MQTT/NATS 通信测试方案

## 概述

本文档详细说明了 EdgeX 边缘采集网关与 edgeOS 节点之间基于 MQTT/NATS 消息中间件的通信测试方案。

## 文档对照表

本文档为 **EdgeX 端测试方案**，与 **EdgeOS 端验证文档** 相互配合。请配合使用：

| EdgeX 测试用例 | 测试内容 | EdgeOS 验证章节 | 说明 |
|---------------|---------|----------------|------|
| 测试 3.1.1 | MQTT 节点注册 | `测试一：节点注册流程` | EdgeX 发送注册请求，EdgeOS 存储并响应 |
| 测试 3.1.2 | 子设备上报 | `测试二：子设备同步流程` | EdgeX 上报设备列表，EdgeOS 同步到 BoltDB |
| **测试 3.1.3** | **点位元数据上报** | `测试二：子设备同步流程` | EdgeX 上报点位定义列表，EdgeOS 存储点位元数据 |
| 测试 3.1.4 | 设备点位值同步 | `测试三：设备点位同步流程` | EdgeX 上报点位全量数据，EdgeOS 存储并响应 |
| 测试 3.1.5 | 实时数据(变化)推送 | `测试四：实时数据推送流程` | EdgeX 推送变化数据，EdgeOS 更新并推送 WebSocket |
| 测试 3.1.6 | 心跳保活 | `测试五：心跳维持流程` | EdgeX 发送心跳，EdgeOS 更新最后活跃时间 |
| 测试 3.1.7 | 控制命令 | `测试七：控制命令下发流程` | EdgeOS 下发命令，EdgeX 执行并响应 |
| 测试 3.2.1 | NATS 节点注册 | `测试一：节点注册流程` | 与 MQTT 版本对应，使用 NATS 协议 |
| 测试 3.2.2 | NATS 设备点位全量同步 | `测试三：设备点位同步流程` | 与 MQTT 版本对应，使用 NATS 协议 |
| 测试 3.2.3 | NATS 实时数据 | `测试四：实时数据推送流程` | 与 MQTT 版本对应，使用 NATS 协议 |
| 测试 3.2.4 | NATS 请求/响应 | — | NATS 特有模式 |

### MQTT 主题对照

| 操作 | EdgeX 发布 | EdgeOS 发布 | 说明 |
|------|-----------|-------------|------|
| 节点注册 | `edgex/nodes/register` | `edgex/nodes/{node_id}/response` | |
| 设备上报 | `edgex/devices/report` | — | 无需响应 |
| 点位元数据上报 | `edgex/points/report` | — | 上报点位定义列表 |
| 设备点位值同步 | `edgex/points/{node_id}/{device_id}` | — | 首次/全量同步 |
| 心跳 | `edgex/nodes/{node_id}/heartbeat` | — | 无需响应 |
| 实时数据(变化) | `edgex/data/{node_id}/{device_id}` | — | 无需响应 |
| 命令下发 | — | `edgex/cmd/{node_id}/{command}` | EdgeOS 下发 |
| 命令响应 | `edgex/cmd/{node_id}/response` | — | EdgeX 响应 |
| 主动发现 | `edgex/discover/request` | `edgex/discover/response/{node_id}` | EdgeX 发起 |

### NATS 主题对照

| 操作 | EdgeX 发布 | EdgeOS 发布 | 说明 |
|------|-----------|-------------|------|
| 节点注册 | `edgex.nodes.register` | `edgex.nodes.{node_id}.response` | 点分隔替代斜杠 |
| 设备上报 | `edgex.devices.report` | — | **EdgeOS 必须订阅**（与 MQTT `edgex/devices/report` 对称） |
| 点位元数据上报 | `edgex.points.report` | — | 上报点位定义列表 |
| 设备点位值同步 | `edgex.points.{node_id}.{device_id}` | — | 首次/全量同步 |
| 心跳 | `edgex.nodes.{node_id}.heartbeat` | — | |
| 实时数据(变化) | `edgex.data.{node_id}.{device_id}` | — | |
| 请求/响应 | — | `edgex.req.node.info` / `edgex.res.node.info` | NATS 特有 |

## 测试环境

### 1.1 硬件环境

| 组件 | 配置 | 数量 |
|------|------|------|
| 服务器 | 4核8G内存，100G硬盘 | 1台 |
| 网络 | 千兆以太网 | 1个 |

### 1.2 软件环境

| 软件 | 版本 | 说明 |
|------|------|------|
| 操作系统 | Ubuntu 22.04 LTS / Windows 11 | 宿主操作系统 |
| Docker | 24.x | 容器运行环境 |
| MQTT Broker | 本地已经启动 | MQTT 消息中间件 |
| NATS Server | 2.10.x | NATS 消息中间件 |
| Go | 1.21+ | 开发语言 |
| edgeOS | v1.0 | 节点 |
| EdgeX | v1.0 | 边缘采集网关 |

### 1.3 网络拓扑

```
┌─────────────────────────────────────────────────────────────┐
│                     测试网络 已经启动MQTT                              │
│                                                             │
│  ┌──────────────┐                                         │
│  │  MQTT Broker │ (tcp://127.0.0.1:1883)                 │
│  │  Mosquitto   │                                         │
│  └──────────────┘                                         │
│         ▲                                                 │
│         │                                                 │
│    ┌────┴────┐                                           │
│    │         │                                           │
│ ┌──▼───┐  ┌─▼────┐                                    │
│ │edgeOS │  │EdgeX │                                    │
│ │  │  │节点   │                                    │
│ │clientID1 │  ││clientID2 │                                    │
│ └──────┘  └──────┘                                    │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### 1.4 消息订阅/发布角色说明

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           MQTT/NATS Broker                             │
│                                                                         │
│   EdgeX ──订阅──> edgex/cmd/{node_id}/+                                │
│   EdgeX ──发布──> edgex/nodes/register         (节点注册请求)           │
│   EdgeX ──发布──> edgex/nodes/{node_id}/heartbeat  (心跳)              │
│   EdgeX ──发布──> edgex/devices/report           (设备上报)             │
│   EdgeX ──发布──> edgex/data/{node_id}/{device_id}  (实时数据)        │
│   EdgeX ──发布──> edgex/cmd/{node_id}/response  (命令响应)             │
│                                                                         │
│   edgeOS ──订阅──> edgex/nodes/register        (接收节点注册)           │
│   edgeOS ──订阅──> edgex/nodes/{node_id}/heartbeat  (接收心跳)        │
│   edgeOS ──订阅──> edgex/devices/report        (接收设备上报)           │
│   edgeOS ──订阅──> edgex/data/{node_id}/{device_id}  (接收实时数据)    │
│   edgeOS ──发布──> edgex/nodes/{node_id}/response  (注册响应)          │
│   edgeOS ──发布──> edgex/cmd/{node_id}/{command}  (下发命令)           │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 1.5 订阅/发布主题对照表

#### EdgeX 角色

| 操作 | 行为 | Topic | 说明 |
|------|------|-------|------|
| 连接时订阅 | **订阅** | `edgex/cmd/<node_id>/+` | 接收来自 edgeOS 的所有命令 |
| 节点注册 | **发布** | `edgex/nodes/register` | 发送节点注册请求 |
| 心跳 | **发布** | `edgex/nodes/<node_id>/heartbeat` | 定期发送心跳保活 |
| 设备上报 | **发布** | `edgex/devices/report` | 上报设备列表 |
| 点位元数据上报 | **发布** | `edgex/points/report` | 上报点位定义列表 |
| 点位数据同步 | **发布** | `edgex/points/<node_id>/<device_id>` | 上报点位全量数据 |
| 实时数据(变化) | **发布** | `edgex/data/<node_id>/<device_id>` | 上报变化数据 |
| 命令响应 | **发布** | `edgex/cmd/<node_id>/response` | 响应命令执行结果 |

#### edgeOS 角色

| 操作 | 行为 | Topic | 说明 |
|------|------|-------|------|
| 启动时订阅 | **订阅** | `edgex/nodes/register` | 接收节点注册请求 |
| 启动时订阅 | **订阅** | `edgex/nodes/+` | 接收所有节点级消息 |
| 启动时订阅 | **订阅** | `edgex/devices/report` | 接收设备上报 |
| 启动时订阅 | **订阅** | `edgex/points/report` | 接收点位元数据上报 |
| 启动时订阅 | **订阅** | `edgex/points/+/+` | 接收点位全量同步 |
| 启动时订阅 | **订阅** | `edgex/data/+/+` | 接收所有设备数据 |
| 注册响应 | **发布** | `edgex/nodes/<node_id>/response` | 返回注册结果和 Token |
| 下发命令 | **发布** | `edgex/cmd/<node_id>/<command>` | 下发设备控制命令 |
| WebSocket | **推送** | WebSocket 连接 | 推送状态变更到 Web UI |

### 1.6 交互逻辑流程说明

#### 流程一：节点注册 (Node Registration)

```
EdgeX                              edgeOS                            Broker
 │                                   │                                 │
 │  1. 连接到 Broker                 │                                 │
 │──────────────────────────────────>│                                 │
 │                                   │                                 │
 │  2. 订阅 edgex/cmd/{node_id}/+   │                                 │
 │──────────────────────────────────>│                                 │
 │                                   │ 3. 订阅 edgex/nodes/register    │
 │                                   │<────────────────────────────────│
 │                                   │                                 │
 │  4. 发布 node_register           │                                 │
 │────────────────────────────────────────────────────────>│
 │                                   │ 5. 转发注册消息                  │
 │                                   │<────────────────────────────────│
 │                                   │                                 │
 │                                   │ 6. 解析并存储节点信息            │
 │                                   │    (UpsertNode → BoltDB)        │
 │                                   │                                 │
 │  7. 接收 register_response       │ 8. 发布响应                      │
 │<─────────────────────────────────────────────────────────│
 │                                   │                                 │
 │                                   │ 9. WebSocket 推送 node_status    │
 │                                   │────────────────────────────────>│ (Web UI)
```

**交互说明**:
1. EdgeX 首先连接到 MQTT Broker，并订阅命令接收主题
2. EdgeX 发布 `node_register` 到 `edgex/nodes/register`
3. Broker 转发消息给已订阅的 edgeOS
4. edgeOS 解析消息、验证节点信息、存储到 BoltDB
5. edgeOS 发布 `register_response` 到 `edgex/nodes/<node_id>/response`
6. EdgeX 接收响应，获取 `access_token` 和 `expires_at`
7. edgeOS 通过 WebSocket 通知前端更新节点状态

#### 流程二：设备同步 (Device Sync)

```
EdgeX                              edgeOS                            Broker
 │                                   │                                 │
 │  1. 发布 device_report           │                                 │
 │  (设备列表)                      │                                 │
 │────────────────────────────────────────────────────────>│
 │                                   │ 2. 转发设备报告                  │
 │                                   │<────────────────────────────────│
 │                                   │                                 │
 │                                   │ 3. 解析并存储设备信息            │
 │                                   │    (UpsertDevices → BoltDB)     │
 │                                   │                                 │
 │  4. 发布 point_report            │                                 │
 │  (点位元数据定义)                 │                                 │
 │────────────────────────────────────────────────────────>│
 │                                   │ 5. 转发点位报告                 │
 │                                   │<────────────────────────────────│
 │                                   │                                 │
 │                                   │ 6. 解析并存储点位定义            │
 │                                   │    (UpsertPointDefinitions)     │
 │                                   │                                 │
 │  7. 发布 point_sync              │                                 │
 │  (点位全量数据值)                 │                                 │
 │────────────────────────────────────────────────────────>│
 │                                   │ 8. 转发点位同步消息             │
 │                                   │<────────────────────────────────│
 │                                   │                                 │
 │                                   │ 9. 解析并存储点位数据           │
 │                                   │    (UpsertPoints → BoltDB)     │
 │                                   │                                 │
 │                                   │ 10. WebSocket 推送 device_synced│
 │                                   │    & point_report & point_synced│
 │                                   │────────────────────────────────>│ (Web UI)
```

**交互说明**:
1. EdgeX 在节点注册成功后，自动或手动发布设备列表到 `edgex/devices/report`
2. edgeOS 接收并解析设备信息，按 `node_id` 关联存储
3. EdgeX 发布点位元数据到 `edgex/points/report`，包含所有点位定义（point_id, point_name, data_type, unit, address 等）
4. edgeOS 接收并解析点位元数据，更新 BoltDB 中对应设备的点位定义
5. EdgeX 发布点位值同步到 `edgex/points/<node_id>/<device_id>`，包含所有点位当前值
6. edgeOS 接收并解析点位值数据，更新 BoltDB 中对应设备的点位值
7. edgeOS 通过 WebSocket 推送设备同步和点位同步事件，前端刷新设备列表和点位数据

#### 流程三：变化数据推送 (Change-based Data Push)

```
EdgeX                              edgeOS                            Broker
 │                                   │                                 │
 │  1. 发布 data_report             │                                 │
 │  (变化数据/按需推送)             │                                 │
 │────────────────────────────────────────────────────────>│
 │                                   │ 2. 转发数据消息                  │
 │                                   │<────────────────────────────────│
 │                                   │                                 │
 │                                   │ 3. 更新点位值                    │
 │                                   │    (UpdateValue → BoltDB)       │
 │                                   │                                 │
 │                                   │ 4. WebSocket 推送 data_update   │
 │                                   │────────────────────────────────>│ (Web UI)
```

**交互说明**:
1. EdgeX 持续采集设备数据，仅当点位值发生变化或达到推送周期时，发布到 `edgex/data/<node_id>/<device_id>`
2. edgeOS 接收数据，更新 BoltDB 中对应点位的 `current_value`
3. edgeOS 通过 WebSocket 推送数据更新事件，前端实时刷新显示

#### 流程四：心跳保活 (Heartbeat)

```
EdgeX                              edgeOS                            Broker
 │                                   │                                 │
 │  1. 发布 heartbeat (每30秒)       │                                 │
 │────────────────────────────────────────────────────────>│
 │                                   │ 2. 转发心跳消息                  │
 │                                   │<────────────────────────────────│
 │                                   │                                 │
 │                                   │ 3. 更新节点最后活跃时间          │
 │                                   │    (UpdateLastSeen)             │
 │                                   │                                 │
 │                                   │ 4. WebSocket 推送 node_status   │
 │                                   │    (status: online)            │
 │                                   │────────────────────────────────>│ (Web UI)
 │
 │  ... (正常心跳中，节点保持在线)    │                                 │
 │
 │                                   │                                 │
 │  5. 心跳超时 (90秒无心跳)         │                                 │
 │                                   │ 6. WebSocket 推送 node_status   │
 │                                   │    (status: offline)            │
 │                                   │────────────────────────────────>│ (Web UI)
```

**交互说明**:
1. EdgeX 每 30 秒发送一次心跳到 `edgex/nodes/<node_id>/heartbeat`
2. edgeOS 收到心跳后更新节点的 `last_seen` 时间戳
3. 若 90 秒内未收到心跳，edgeOS 判定节点离线，更新状态并通知前端

#### 流程五：命令下发 (Command Dispatch)

```
edgeOS                             EdgeX                             Broker
 │                                   │                                 │
 │  1. 发布 discover_command        │                                 │
 │────────────────────────────────────────────────────────>│
 │                                   │ 2. 转发命令消息                  │
 │                                   │<────────────────────────────────│
 │                                   │                                 │
 │                                   │ 3. 订阅 edgex/cmd/{node_id}/+  │
 │                                   │    (已订阅，等待消息)            │
 │                                   │                                 │
 │                                   │ 4. 接收并解析命令                │
 │                                   │                                 │
 │                                   │ 5. 执行设备发现/数据写入         │
 │                                   │                                 │
 │  6. 发布 device_report           │ 7. 执行结果上报                  │
 │<─────────────────────────────────────────────────────────────────────│
 │                                   │                                 │
 │  8. 接收设备列表并同步            │                                 │
 │                                   │                                 │
 │  9. WebSocket 推送 device_synced  │                                 │
 │────────────────────────────────>│ (Web UI)                        │
```

**交互说明**:
1. edgeOS 通过 Web UI 或 API 触发设备发现，发布命令到 `edgex/cmd/<node_id>/discover`
2. EdgeX 订阅了命令主题，收到消息后执行相应操作
3. 执行完成后，EdgeX 发布 `device_report` 或响应消息
4. edgeOS 接收执行结果，更新设备列表或状态

---

## 测试准备

### 2.1 本地已经启动 MQTT Broker

### 2.2 本地已经启动 NATS Server

---

## 功能测试

### 3.1 MQTT 功能测试

#### 测试 3.1.1: 节点注册

**目的**: 验证 EdgeX 节点能够通过 MQTT 成功注册到 edgeOS

##### 3.1.1.1 时序流程图

```
┌──────────────┐     ┌────────────────┐     ┌──────────────┐     ┌──────────────────┐
│    EdgeX     │     │  MQTT Broker   │     │    EdgeOS    │     │     Web UI       │
└──────┬───────┘     └───────┬────────┘     └───────┬──────┘     └────────┬─────────┘
       │                     │                      │                      │
       │  1. Connect &       │                      │                      │
       │     Subscribe       │                      │                      │
       │────────────────────>│                      │                      │
       │                     │  2. Subscribe        │                      │
       │                     │<─────────────────────│                      │
       │                     │  (edgex/nodes/register)                    │
       │                     │                      │                      │
       │  3. Publish         │                      │                      │
       │  node_register      │                      │                      │
       │────────────────────>│  4. Forward          │                      │
       │                     │─────────────────────>│                      │
       │                     │                      │  5. UpsertNode()     │
       │                     │                      │  ─────────────────>  │
       │                     │                      │  (BoltDB)            │
       │                     │  6. Publish          │                      │
       │                     │  register_response   │                      │
       │  7. Receive         │<─────────────────────│                      │
       │<────────────────────│                      │                      │
       │                     │                      │  8. WebSocket        │
       │                     │                      │  (node_status)      │
       │                     │                      │─────────────────────>│
       │                     │                      │                      │
       │                     │                      │  9. HTTP GET /nodes  │
       │                     │                      │<─────────────────────│
       │                     │                      │                      │
```

##### 3.1.1.2 EdgeX 发布消息

**Topic**: `edgex/nodes/register`

**消息格式**:
```json
{
  "header": {
    "message_id": "<唯一消息ID>",
    "timestamp": <Unix毫秒时间戳>,
    "source": "<node_id>",
    "destination": "edgeos",
    "message_type": "node_register",
    "version": "1.0"
  },
  "body": {
    "node_id": "<节点ID>",
    "node_name": "<节点名称>",
    "model": "edgex",
    "version": "<版本号>",
    "api_version": "v1",
    "capabilities": ["shadow-sync", "heartbeat", "device-control", "task-execution"],
    "protocol": "edgeOS(MQTT)",
    "endpoint": {
      "host": "<本机IP>",
      "port": <API端口>
    },
    "metadata": {
      "os": "linux",
      "arch": "amd64",
      "hostname": "<主机名>"
    }
  }
}
```

**示例**:
```bash
mosquitto_pub -h 127.0.0.1 -p 1883 \
  -t "edgex/nodes/register" \
  -m '{
    "header": {
      "message_id": "node-reg-001",
      "timestamp": 1744680000000,
      "source": "edgex-node-001",
      "destination": "edgeos",
      "message_type": "node_register",
      "version": "1.0"
    },
    "body": {
      "node_id": "edgex-node-001",
      "node_name": "EdgeX Gateway Node 001",
      "model": "edgex",
      "version": "1.0.0",
      "api_version": "v1",
      "capabilities": ["shadow-sync", "heartbeat", "device-control", "task-execution"],
      "protocol": "edgeOS(MQTT)",
      "endpoint": {"host": "192.168.1.100", "port": 8082},
      "metadata": {"os": "linux", "arch": "amd64", "hostname": "edgex-node-001.local"}
    }
  }'
```

##### 3.1.1.3 EdgeOS 预期响应

**Topic**: `edgex/nodes/<node_id>/response`

**响应消息格式**:
```json
{
  "header": {
    "message_id": "<响应消息ID>",
    "timestamp": <响应时间戳>,
    "source": "edgeos",
    "destination": "<node_id>",
    "message_type": "register_response",
    "version": "1.0"
  },
  "body": {
    "node_id": "<节点ID>",
    "status": "success",
    "access_token": "<访问令牌>",
    "expires_at": <令牌过期时间戳>
  }
}
```

**EdgeX 接收响应验证**:
```bash
# 订阅注册响应
mosquitto_sub -h 127.0.0.1 -p 1883 -t "edgex/nodes/edgex-node-001/response" -v
```

**步骤**:

1. 启动 edgeOS 节点
2. 启动 EdgeX 节点
3. EdgeX 发布节点注册消息到 `edgex/nodes/register`
4. 验证 EdgeX 接收到 `edgex/nodes/<node_id>/response` 响应
5. 验证响应中 `status` 为 `success`
6. 验证响应中包含 `access_token` 和 `expires_at`
7. 验证 Web UI 显示节点状态为 "Online"

**预期结果**:
- ✅ EdgeOS 成功接收节点注册消息
- ✅ EdgeX 收到 `register_response` 响应
- ✅ 节点状态显示为 "Online"
- ✅ 节点信息正确显示在 edgeOS 中

**交叉验证** (与 EdgeOS 验证文档对应):
- 验证文档: `测试一：节点注册流程` 第 1.4 节
- 验证项: MQTT 消息接收、节点存储、注册响应、WebSocket 推送、UI 显示

#### 测试 3.1.2: 子设备上报

**目的**: 验证设备信息能够通过 MQTT 上报到 edgeOS

##### 3.1.2.1 时序流程图

```
┌──────────────┐     ┌────────────────┐     ┌────────────────┐     ┌──────────────────┐
│    EdgeX     │     │  MQTT Broker   │     │    EdgeOS      │     │     Web UI       │
└──────┬───────┘     └───────┬────────┘     └───────┬────────┘     └───────┬──────────┘
       │                     │                      │                      │
       │  1. Publish         │                      │                      │
       │  device_report      │                      │                      │
       │────────────────────>│  2. Forward          │                      │
       │                     │─────────────────────>│                      │
       │                     │                      │  3. UpsertDevices()  │
       │                     │                      │  ─────────────────>  │
       │                     │                      │  (BoltDB)            │
       │                     │                      │  4. WebSocket        │
       │                     │                      │  (device_synced)    │
       │                     │                      │─────────────────────>│
       │                     │                      │                      │
       │                     │                      │  5. HTTP GET /nodes/│
       │                     │                      │     {node_id}/devices│
       │                     │                      │<─────────────────────│
       │                     │                      │                      │
```

##### 3.1.2.2 EdgeX 发布消息

**Topic**: `edgex/devices/report`

**消息格式**:
```json
{
  "header": {
    "message_id": "<唯一消息ID>",
    "timestamp": <Unix毫秒时间戳>,
    "source": "<node_id>",
    "message_type": "device_report",
    "version": "1.0"
  },
  "body": {
    "node_id": "<节点ID>",
    "devices": [
      {
        "device_id": "<设备ID>",
        "device_name": "<设备名称>",
        "device_profile": "<设备配置名称>",
        "service_name": "<服务名称>",
        "labels": ["<标签1>", "<标签2>"],
        "description": "<设备描述>",
        "admin_state": "ENABLED",
        "operating_state": "ENABLED",
        "properties": {
          "<自定义属性>": "<属性值>"
        }
      }
    ]
  }
}
```

**示例**:
```bash
mosquitto_pub -h 127.0.0.1 -p 1883 \
  -t "edgex/devices/report" \
  -m '{
    "header": {
      "message_id": "dev-report-001",
      "timestamp": 1744680010000,
      "source": "edgex-node-001",
      "message_type": "device_report",
      "version": "1.0"
    },
    "body": {
      "node_id": "edgex-node-001",
      "devices": [
        {
          "device_id": "Room_FC_2014_19",
          "device_name": "Room FC 2014 HVAC Controller",
          "device_profile": "hvac-controller",
          "service_name": "bacnet-service",
          "labels": ["hvac", "room-control"],
          "description": "HVAC Controller for Room 2014",
          "admin_state": "ENABLED",
          "operating_state": "ENABLED",
          "properties": {"protocol": "bacnet-ip", "address": "192.168.1.50:47808"}
        },
        {
          "device_id": "Chiller_Plant_A",
          "device_name": "Chiller Plant A Controller",
          "device_profile": "chiller-controller",
          "service_name": "modbus-tcp-service",
          "labels": ["chiller", "hvac", "cooling"],
          "description": "Main Chiller Plant Controller",
          "admin_state": "ENABLED",
          "operating_state": "ENABLED",
          "properties": {"protocol": "modbus-tcp", "address": "192.168.1.100:502", "unit_id": 1}
        }
      ]
    }
  }'
```

**步骤**:

1. 确保节点已成功注册（参考测试 3.1.1）
2. EdgeX 发布设备上报消息到 `edgex/devices/report`
3. 验证 WebSocket 推送 `device_synced` 事件
4. 验证 edgeOS 接收到设备信息
5. 验证设备在 edgeOS 中正确显示

**预期结果**:
- ✅ EdgeOS 成功接收设备上报消息
- ✅ WebSocket 推送 `device_synced` 事件
- ✅ 设备信息正确显示在 edgeOS 中
- ✅ API 返回设备列表包含上报的设备

**交叉验证** (与 EdgeOS 验证文档对应):
- 验证文档: `测试二：子设备同步流程` 第 2.4 节
- 验证项: WebSocket 推送、API 返回设备列表

#### 测试 3.1.3: 点位元数据上报

**目的**: 验证设备点位元数据（点位定义）能够通过 MQTT 上报到 edgeOS，包含点位的 ID、名称、数据类型、单位等属性信息

##### 3.1.3.1 时序流程图

```
┌──────────────┐     ┌────────────────┐     ┌────────────────┐     ┌──────────────────┐
│    EdgeX     │     │  MQTT Broker   │     │    EdgeOS     │     │     Web UI       │
└──────┬───────┘     └───────┬────────┘     └───────┬──────┘     └────────┬─────────┘
       │                     │                      │                      │
       │  1. Publish         │                      │                      │
       │  point_report      │                      │                      │
       │  edgex/points/     │                      │                      │
       │  report             │                      │                      │
       │────────────────────>│  2. Forward          │                      │
       │                     │─────────────────────>│                      │
       │                     │                      │  3. Parse and store  │
       │                     │                      │     point metadata   │
       │                     │                      │  ─────────────────>  │
       │                     │                      │  (BoltDB)            │
       │                     │                      │  4. WebSocket        │
       │                     │                      │  (point_report)     │
       │                     │                      │─────────────────────>│
       │                     │                      │                      │
```

##### 3.1.3.2 EdgeX 发布消息

**Topic**: `edgex/points/report`

**消息格式**:
```json
{
  "header": {
    "message_id": "<唯一消息ID>",
    "timestamp": <Unix毫秒时间戳>,
    "source": "<node_id>",
    "message_type": "point_report",
    "version": "1.0"
  },
  "body": {
    "node_id": "<节点ID>",
    "device_id": "<设备ID>",
    "points": [
      {
        "point_id": "<点位ID>",
        "point_name": "<点位名称>",
        "data_type": "<数据类型: Float32|Float64|Int16|Int32|String|Bool>",
        "access_mode": "<访问模式: R|RW|W>",
        "unit": "<单位>",
        "minimum": <最小值>,
        "maximum": <最大值>,
        "address": "<点位地址>",
        "description": "<点位描述>",
        "scale": <比例因子>,
        "offset": <偏移量>
      }
    ]
  }
}
```

**示例**:
```bash
mosquitto_pub -h 127.0.0.1 -p 1883 \
  -t "edgex/points/report" \
  -m '{
    "header": {
      "message_id": "point-report-001",
      "timestamp": 1744680012000,
      "source": "edgex-node-001",
      "message_type": "point_report",
      "version": "1.0"
    },
    "body": {
      "node_id": "edgex-node-001",
      "device_id": "Room_FC_2014_19",
      "points": [
        {
          "point_id": "SupplyWaterTemp",
          "point_name": "供水温度",
          "data_type": "Float32",
          "access_mode": "R",
          "unit": "°C",
          "minimum": -50.0,
          "maximum": 150.0,
          "address": "AI-30001",
          "description": "AHU Supply Water Temperature Sensor",
          "scale": 0.1,
          "offset": 0
        },
        {
          "point_id": "ReturnWaterTemp",
          "point_name": "回水温度",
          "data_type": "Float32",
          "access_mode": "R",
          "unit": "°C",
          "minimum": -50.0,
          "maximum": 150.0,
          "address": "AI-30002",
          "description": "AHU Return Water Temperature Sensor",
          "scale": 0.1,
          "offset": 0
        },
        {
          "point_id": "ValvePosition",
          "point_name": "阀门开度",
          "data_type": "Int16",
          "access_mode": "RW",
          "unit": "%",
          "minimum": 0,
          "maximum": 100,
          "address": "AO-10001",
          "description": "Control Valve Position",
          "scale": 1,
          "offset": 0
        }
      ]
    }
  }'
```

**步骤**:

1. 确保节点和设备已成功上报（参考测试 3.1.1 和 3.1.2）
2. EdgeX 发布点位元数据消息到 `edgex/points/report`
3. 验证 edgeOS 接收到点位元数据
4. 验证点位定义正确存储到 BoltDB
5. 验证 WebSocket 推送 `point_report` 事件
6. 验证点位定义在 edgeOS 中正确显示

**预期结果**:
- ✅ EdgeOS 成功接收点位元数据消息
- ✅ 点位定义包含完整信息（point_id, point_name, data_type, access_mode, unit, address 等）
- ✅ 点位元数据正确存储到 BoltDB
- ✅ WebSocket 推送 `point_report` 事件
- ✅ 点位定义正确显示在 edgeOS 中

**交叉验证** (与 EdgeOS 验证文档对应):
- 验证文档: `测试二：子设备同步流程` 第 2.4 节
- 验证项: 点位元数据存储、WebSocket 推送、API 返回点位定义列表

#### 测试 3.1.4: 设备点位值同步

**目的**: 验证设备点位全量数据能够通过 MQTT 同步到 edgeOS（首次同步或全量更新）

##### 3.1.4.1 时序流程图

```
┌──────────────┐     ┌────────────────┐     ┌────────────────┐     ┌──────────────────┐
│    EdgeX     │     │  MQTT Broker   │     │    EdgeOS     │     │     Web UI       │
└──────┬───────┘     └───────┬────────┘     └───────┬──────┘     └────────┬─────────┘
       │                     │                      │                      │
       │  1. Publish         │                      │                      │
       │  point_sync         │                      │                      │
       │  edgex/points/      │                      │                      │
       │  {node_id}/         │                      │                      │
       │  {device_id}        │                      │                      │
       │────────────────────>│  2. Forward          │                      │
       │                     │─────────────────────>│                      │
       │                     │                      │  3. UpsertPoints()   │
       │                     │                      │  ─────────────────>  │
       │                     │                      │  (BoltDB)            │
       │                     │                      │  4. WebSocket        │
       │                     │                      │  (point_synced)     │
       │                     │                      │─────────────────────>│
       │                     │                      │                      │
```

##### 3.1.4.2 EdgeX 发布消息

**Topic**: `edgex/points/<node_id>/<device_id>`

**消息格式**:
```json
{
  "header": {
    "message_id": "<唯一消息ID>",
    "timestamp": <Unix毫秒时间戳>,
    "source": "<node_id>",
    "message_type": "point_sync",
    "version": "1.0"
  },
  "body": {
    "node_id": "<节点ID>",
    "device_id": "<设备ID>",
    "timestamp": <数据时间戳>,
    "points": {
      "<点位名称1>": {
        "value": <当前值>,
        "quality": "good|uncertain|bad",
        "timestamp": <点位时间戳>
      },
      "<点位名称2>": {
        "value": <当前值>,
        "quality": "good|uncertain|bad",
        "timestamp": <点位时间戳>
      }
    },
    "quality": "good"
  }
}
```

**示例**:
```bash
mosquitto_pub -h 127.0.0.1 -p 1883 \
  -t "edgex/points/edgex-node-001/Room_FC_2014_19" \
  -m '{
    "header": {
      "message_id": "point-sync-001",
      "timestamp": 1744680015000,
      "source": "edgex-node-001",
      "message_type": "point_sync",
      "version": "1.0"
    },
    "body": {
      "node_id": "edgex-node-001",
      "device_id": "Room_FC_2014_19",
      "timestamp": 1744680015000,
      "points": {
        "Temperature": {
          "value": 25.5,
          "quality": "good",
          "timestamp": 1744680010000
        },
        "Humidity": {
          "value": 65.2,
          "quality": "good",
          "timestamp": 1744680010000
        },
        "Setpoint": {
          "value": 24.0,
          "quality": "good",
          "timestamp": 1744680012000
        },
        "FanSpeed": {
          "value": 75,
          "quality": "good",
          "timestamp": 1744680010000
        }
      },
      "quality": "good"
    }
  }'
```

**步骤**:

1. 确保节点和设备已成功同步（参考测试 3.1.1 和 3.1.2）
2. EdgeX 发布点位全量同步到 `edgex/points/<node_id>/<device_id>`
3. 验证 WebSocket 推送 `point_synced` 事件
4. 验证 edgeOS 接收到所有点位数据
5. 验证点位数据在 Web UI 中正确显示

**预期结果**:
- ✅ EdgeOS 成功接收点位全量同步消息
- ✅ WebSocket 推送 `point_synced` 事件
- ✅ 所有点位数据正确存储到 BoltDB
- ✅ 点位数据正确显示在 Web UI 中
- ✅ 点位数据包含完整信息（value, quality, timestamp）

**交叉验证** (与 EdgeOS 验证文档对应):
- 验证文档: `测试三：设备点位同步流程` 第 3.4 节
- 验证项: WebSocket 推送、API 返回点位列表、UI 点位数据显示

#### 测试 3.1.5: 实时数据(变化)推送

**目的**: 验证变化数据能够通过 MQTT 推送到 edgeOS（仅推送发生变化的点位数据）

##### 3.1.5.1 时序流程图

```
┌──────────────┐     ┌────────────────┐     ┌────────────────┐     ┌──────────────────┐
│    EdgeX     │     │  MQTT Broker   │     │    EdgeOS     │     │     Web UI       │
└──────┬───────┘     └───────┬────────┘     └───────┬──────┘     └────────┬─────────┘
       │                     │                      │                      │
       │  1. Publish         │                      │                      │
       │  data_report        │                      │                      │
       │  edgex/data/        │                      │                      │
       │  {node_id}/         │                      │                      │
       │  {device_id}        │                      │                      │
       │────────────────────>│  2. Forward          │                      │
       │                     │─────────────────────>│                      │
       │                     │                      │  3. UpdateValue()    │
       │                     │                      │  ─────────────────>  │
       │                     │                      │  (BoltDB)            │
       │                     │                      │  4. WebSocket        │
       │                     │                      │  (data_update)       │
       │                     │                      │─────────────────────>│
       │                     │                      │                      │
       │                     │                      │  5. HTTP GET /nodes/ │
       │                     │                      │     {node_id}/       │
       │                     │                      │     devices/{device} │
       │                     │                      │     /points          │
       │                     │                      │<─────────────────────│
       │                     │                      │                      │
```

##### 3.1.5.2 EdgeX 发布消息

**Topic**: `edgex/data/<node_id>/<device_id>`

**消息格式**:
```json
{
  "header": {
    "message_id": "<唯一消息ID>",
    "timestamp": <Unix毫秒时间戳>,
    "source": "<node_id>",
    "message_type": "data",
    "version": "1.0"
  },
  "body": {
    "node_id": "<节点ID>",
    "device_id": "<设备ID>",
    "timestamp": <数据时间戳>,
    "points": {
      "<点位名称1>": <值>,
      "<点位名称2>": <值>
    },
    "quality": "good|uncertain|bad"
  }
}
```

**示例**:
```bash
# 发布实时数据
mosquitto_pub -h 127.0.0.1 -p 1883 \
  -t "edgex/data/edgex-node-001/Room_FC_2014_19" \
  -m '{
    "header": {
      "message_id": "data-001",
      "timestamp": 1744680020000,
      "source": "edgex-node-001",
      "message_type": "data",
      "version": "1.0"
    },
    "body": {
      "node_id": "edgex-node-001",
      "device_id": "Room_FC_2014_19",
      "timestamp": 1744680020000,
      "points": {
        "Temperature": 25.5,
        "Humidity": 65.2,
        "Setpoint": 24.0,
        "FanSpeed": 75
      },
      "quality": "good"
    }
  }'
```

**步骤**:

1. 确保节点、设备和点位已成功同步（参考测试 3.1.1、3.1.2 和 3.1.3）
2. EdgeX 发布实时数据（变化数据）到 `edgex/data/<node_id>/<device_id>`
3. 验证 WebSocket 推送 `data_update` 事件
4. 验证 edgeOS 接收到实时数据
5. 验证数据在 Web UI 中正确显示

**预期结果**:
- ✅ EdgeOS 成功接收实时数据
- ✅ WebSocket 推送 `data_update` 事件
- ✅ 数据值正确显示在 Web UI 中
- ✅ 数据质量为 `good`

**交叉验证** (与 EdgeOS 验证文档对应):
- 验证文档: `测试四：实时数据推送流程` 第 4.4 节
- 验证项: WebSocket 推送、UI 数据更新、API 数据查询

#### 测试 3.1.6: 心跳保活

**目的**: 验证心跳消息能够定期发送并被 edgeOS 接收，节点状态保持在线

##### 3.1.6.1 时序流程图

```
┌──────────────┐     ┌────────────────┐     ┌────────────────┐     ┌──────────────────┐
│    EdgeX     │     │  MQTT Broker   │     │    EdgeOS     │     │     Web UI       │
└──────┬───────┘     └───────┬────────┘     └───────┬──────┘     └────────┬─────────┘
       │                     │                      │                      │
       │  1. Publish         │                      │                      │
       │  heartbeat          │                      │                      │
       │  (周期性 30s)        │                      │                      │
       │────────────────────>│  2. Forward          │                      │
       │                     │─────────────────────>│                      │
       │                     │                      │  3. UpdateLastSeen() │
       │                     │                      │  ─────────────────>  │
       │                     │                      │  (BoltDB)            │
       │                     │                      │  4. WebSocket        │
       │                     │                      │  (node_status)       │
       │                     │                      │─────────────────────>│
       │                     │                      │                      │
       │  ... 重复心跳 ...    │                      │                      │
       │                     │                      │                      │
       │  5. 等待超时        │                      │                      │
       │     (无心跳 90s)     │                      │                      │
       │                     │                      │  6. WebSocket        │
       │                     │                      │  (node_status)       │
       │                     │                      │  status: offline    │
       │                     │                      │─────────────────────>│
       │                     │                      │                      │
```

##### 3.1.6.2 EdgeX 发布消息

**Topic**: `edgex/nodes/<node_id>/heartbeat`

**消息格式**:
```json
{
  "header": {
    "message_id": "<唯一消息ID>",
    "timestamp": <Unix毫秒时间戳>,
    "source": "<node_id>",
    "message_type": "heartbeat",
    "version": "1.0"
  },
  "body": {
    "node_id": "<节点ID>",
    "uptime": <运行时间(秒)>,
    "cpu_usage": <CPU使用率>,
    "memory_usage": <内存使用率>,
    "device_count": <设备数量>
  }
}
```

**示例**:
```bash
# 监听心跳消息
mosquitto_sub -h 127.0.0.1 -p 1883 -t "edgex/nodes/+/heartbeat" -v

# 发布心跳消息 (每 30 秒执行一次)
mosquitto_pub -h 127.0.0.1 -p 1883 \
  -t "edgex/nodes/edgex-node-001/heartbeat" \
  -m '{
    "header": {
      "message_id": "heartbeat-001",
      "timestamp": 1744680030000,
      "source": "edgex-node-001",
      "message_type": "heartbeat",
      "version": "1.0"
    },
    "body": {
      "node_id": "edgex-node-001",
      "uptime": 3600,
      "cpu_usage": 25.5,
      "memory_usage": 45.2,
      "device_count": 10
    }
  }'
```

##### 3.1.6.3 心跳超时机制

| 参数 | 默认值 | 说明 |
|------|--------|------|
| heartbeat_interval | 30秒 | 心跳发送间隔 |
| heartbeat_timeout | 90秒 | 节点离线判定时间 |
| offline_threshold | 3次 | 连续超时次数 |

**步骤**:

1. 确保节点已成功注册（参考测试 3.1.1）
2. 配置 EdgeX 每 30 秒发送心跳消息
3. 验证 edgeOS 接收到心跳消息
4. 验证 WebSocket 推送 `node_status` 事件（状态: online）
5. 等待心跳超时（90秒无心跳）
6. 验证节点状态变为 "Offline"
7. 恢复心跳后验证节点状态恢复为 "Online"

**预期结果**:
- ✅ 心跳消息定期发送（每 30 秒）
- ✅ EdgeOS 成功接收心跳消息
- ✅ WebSocket 推送 `node_status` 事件
- ✅ 节点状态保持为 "Online"
- ✅ 心跳超时后节点状态变为 "Offline"
- ✅ 恢复心跳后节点状态恢复为 "Online"

**交叉验证** (与 EdgeOS 验证文档对应):
- 验证文档: `测试五：心跳维持流程` 第 5.4 节
- 验证项: 心跳接收、状态更新、超时判定、状态恢复

#### 测试 3.1.7: 控制命令下发

**目的**: 验证 edgeOS 能够下发控制命令到 EdgeX，EdgeX 执行后返回响应

##### 3.1.7.1 时序流程图 (边缘发现命令)

```
┌──────────────┐     ┌────────────────┐     ┌────────────────┐     ┌──────────────────┐
│    EdgeX     │     │  MQTT Broker   │     │    EdgeOS     │     │     Web UI       │
└──────┬───────┘     └───────┬────────┘     └───────┬──────┘     └────────┬─────────┘
       │                     │                      │                      │
       │                     │  1. HTTP POST        │                      │
       │                     │  /api/edgex/discover │                      │
       │                     │<─────────────────────│                      │
       │                     │                      │  2. Publish          │
       │                     │                      │  discover_command   │
       │                     │                      │─────────────────────>│
       │  3. Subscribe       │                      │                      │
       │  edgex/cmd/{node_id} │                      │                      │
       │<────────────────────│                      │                      │
       │  4. Receive          │                      │                      │
       │  discover_command    │                      │                      │
       │<────────────────────│                      │                      │
       │                     │                      │                      │
       │  5. Execute          │                      │                      │
       │  Device Discovery    │                      │                      │
       │                     │                      │                      │
       │  6. Publish          │                      │                      │
       │  device_report       │                      │                      │
       │────────────────────>│  7. Forward          │                      │
       │                     │─────────────────────>│                      │
       │                     │                      │  8. UpsertDevices() │
       │                     │                      │  ─────────────────>  │
       │                     │                      │  9. WebSocket        │
       │                     │                      │  (device_synced)    │
       │                     │                      │─────────────────────>│
       │                     │                      │                      │
```

##### 3.1.7.2 EdgeOS 下发命令消息格式

**Topic**: `edgex/cmd/<node_id>/<command_type>`

**消息格式**:
```json
{
  "header": {
    "message_id": "<唯一消息ID>",
    "timestamp": <Unix毫秒时间戳>,
    "source": "edgeos",
    "destination": "<node_id>",
    "message_type": "<命令类型>",
    "version": "1.0",
    "correlation_id": "<关联ID>"
  },
  "body": {
    "command": "<命令类型>",
    "protocol": "<协议类型>",
    "network": "<网络地址>",
    "timeout_seconds": <超时时间>,
    "options": {
      "<选项键>": "<选项值>"
    }
  }
}
```

##### 3.1.7.3 边缘发现命令示例

**Topic**: `edgex/cmd/edgex-node-001/discover`

**示例**:
```bash
# 发布设备发现命令
mosquitto_pub -h 127.0.0.1 -p 1883 \
  -t "edgex/cmd/edgex-node-001/discover" \
  -m '{
    "header": {
      "message_id": "cmd-disc-001",
      "timestamp": 1744680100000,
      "source": "edgeos",
      "destination": "edgex-node-001",
      "message_type": "discover_command",
      "version": "1.0",
      "correlation_id": "req-disc-001"
    },
    "body": {
      "command": "discover",
      "protocol": "modbus-tcp",
      "network": "192.168.1.0/24",
      "timeout_seconds": 30,
      "options": {
        "auto_register": true,
        "sync_immediately": true
      }
    }
  }'
```

##### 3.1.7.4 写点位命令示例

**Topic**: `edgex/cmd/<node_id>/write`

**消息格式**:
```json
{
  "header": {
    "message_id": "<唯一消息ID>",
    "timestamp": <Unix毫秒时间戳>,
    "source": "edgeos",
    "destination": "<node_id>",
    "message_type": "write_command",
    "version": "1.0",
    "correlation_id": "<关联ID>"
  },
  "body": {
    "request_id": "<请求ID>",
    "device_id": "<设备ID>",
    "point_id": "<点位ID>",
    "value": <要写入的值>
  }
}
```

**步骤**:

1. 确保节点已成功注册（参考测试 3.1.1）
2. EdgeX 订阅命令主题 `edgex/cmd/<node_id>/+`
3. edgeOS 通过 HTTP API 或直接 MQTT 发布命令
4. 验证 EdgeX 接收到命令
5. 验证 EdgeX 执行命令并返回响应
6. 验证设备同步或数据更新（根据命令类型）

**预期结果**:
- ✅ EdgeX 成功接收控制命令
- ✅ EdgeX 正确执行命令
- ✅ EdgeX 返回设备报告响应
- ✅ edgeOS 接收到响应并更新设备列表

**交叉验证** (与 EdgeOS 验证文档对应):
- 验证文档: `测试七：控制命令下发流程` 第 7.4 节
- 验证项: 命令发布、设备发现、设备同步响应

##### 3.1.7.5 主动发现 (由 EdgeX 触发)

EdgeX 也可以主动发起设备发现，请求 edgeOS 广播到所有中间件通道：

**EdgeX 发布发现请求**:

**Topic**: `edgex/discover/request`

```json
{
  "header": {
    "message_id": "<唯一消息ID>",
    "timestamp": <Unix毫秒时间戳>,
    "source": "<node_id>",
    "message_type": "discover_request",
    "version": "1.0"
  },
  "body": {
    "protocol": "<协议类型>",
    "options": {}
  }
}
```

**edgeOS 响应**:

**Topic**: `edgex/discover/response/<node_id>`

```json
{
  "header": {
    "message_id": "<响应消息ID>",
    "timestamp": <响应时间戳>,
    "source": "edgeos",
    "destination": "<node_id>",
    "message_type": "discover_response",
    "version": "1.0"
  },
  "body": {
    "status": "accepted",
    "message": "Discovery request received"
  }
}
```

### 3.2 NATS 功能测试

> **说明**: NATS 使用点分隔的主题名称，与 MQTT 的斜杠分隔不同。例如:
> - MQTT: `edgex/nodes/register`
> - NATS: `edgex.nodes.register`

#### 测试 3.2.1: 节点注册

**目的**: 验证 EdgeX 节点能够通过 NATS 成功注册到 edgeOS

##### 3.2.1.1 时序流程图

```
┌──────────────┐     ┌────────────────┐     ┌────────────────┐     ┌──────────────────┐
│    EdgeX     │     │ NATS Server    │     │    EdgeOS     │     │     Web UI       │
└──────┬───────┘     └───────┬────────┘     └───────┬──────┘     └────────┬─────────┘
       │                     │                      │                      │
       │  1. Connect &       │                      │                      │
       │     Subscribe       │                      │                      │
       │────────────────────>│                      │                      │
       │                     │  2. Subscribe         │                      │
       │                     │<─────────────────────│                      │
       │                     │  (edgex.nodes.register)                    │
       │                     │                      │                      │
       │  3. Publish         │                      │                      │
       │  node_register      │                      │                      │
       │────────────────────>│  4. Forward          │                      │
       │                     │─────────────────────>│                      │
       │                     │                      │  5. UpsertNode()    │
       │                     │                      │  ─────────────────>  │
       │                     │                      │  (BoltDB)            │
       │                     │  6. Publish          │                      │
       │                     │  register_response   │                      │
       │  7. Receive         │<─────────────────────│                      │
       │<────────────────────│                      │                      │
       │                     │                      │  8. WebSocket        │
       │                     │                      │  (node_status)      │
       │                     │                      │─────────────────────>│
       │                     │                      │                      │
```

##### 3.2.1.2 EdgeX 发布消息

**Subject**: `edgex.nodes.register`

**消息格式** (与 MQTT 版本相同):
```json
{
  "header": {
    "message_id": "<唯一消息ID>",
    "timestamp": <Unix毫秒时间戳>,
    "source": "<node_id>",
    "destination": "edgeos",
    "message_type": "node_register",
    "version": "1.0"
  },
  "body": {
    "node_id": "<节点ID>",
    "node_name": "<节点名称>",
    "model": "edgex",
    "version": "<版本号>",
    "api_version": "v1",
    "capabilities": ["shadow-sync", "heartbeat", "device-control", "task-execution"],
    "protocol": "edgeOS(NATS)",
    "endpoint": {
      "host": "<本机IP>",
      "port": <API端口>
    },
    "metadata": {
      "os": "linux",
      "arch": "amd64",
      "hostname": "<主机名>"
    }
  }
}
```

**示例**:
```bash
# 订阅所有消息
nats sub "edgex.>" > nats_test.log &

# 发布节点注册消息
nats pub "edgex.nodes.register" '{
  "header": {
    "message_id": "nats-node-reg-001",
    "timestamp": 1744680000000,
    "source": "edgex-node-001",
    "destination": "edgeos",
    "message_type": "node_register",
    "version": "1.0"
  },
  "body": {
    "node_id": "edgex-node-001",
    "node_name": "EdgeX Gateway Node 001",
    "model": "edgex",
    "version": "1.0.0",
    "api_version": "v1",
    "capabilities": ["shadow-sync", "heartbeat", "device-control", "task-execution"],
    "protocol": "edgeOS(NATS)",
    "endpoint": {"host": "192.168.1.100", "port": 8082},
    "metadata": {"os": "linux", "arch": "amd64", "hostname": "edgex-node-001.local"}
  }
}'

# 检查日志
grep "node_register" nats_test.log
```

##### 3.2.1.3 EdgeOS 预期响应

**Subject**: `edgex.nodes.<node_id>.response`

**响应消息格式**:
```json
{
  "header": {
    "message_id": "<响应消息ID>",
    "timestamp": <响应时间戳>,
    "source": "edgeos",
    "destination": "<node_id>",
    "message_type": "register_response",
    "version": "1.0"
  },
  "body": {
    "node_id": "<节点ID>",
    "status": "success",
    "access_token": "<访问令牌>",
    "expires_at": <令牌过期时间戳>
  }
}
```

**步骤**:

1. 启动 edgeOS 节点（配置 NATS 中间件）
2. 启动 EdgeX 节点
3. EdgeX 发布节点注册消息到 `edgex.nodes.register`
4. 验证 EdgeX 接收到 `edgex.nodes.<node_id>.response` 响应
5. 验证节点状态为 "Online"

**预期结果**:
- ✅ EdgeOS 成功接收节点注册消息
- ✅ EdgeX 收到 `register_response` 响应
- ✅ 节点状态显示为 "Online"

#### 测试 3.2.2: 设备点位全量同步

**目的**: 验证设备点位全量数据能够通过 NATS 同步到 edgeOS（首次同步或全量更新）

##### 3.2.2.1 时序流程图

```
┌──────────────┐     ┌────────────────┐     ┌────────────────┐     ┌──────────────────┐
│    EdgeX     │     │ NATS Server    │     │    EdgeOS     │     │     Web UI       │
└──────┬───────┘     └───────┬────────┘     └───────┬──────┘     └────────┬─────────┘
       │                     │                      │                      │
       │  1. Publish         │                      │                      │
       │  point_sync         │                      │                      │
       │  edgex.points.     │                      │                      │
       │  {node_id}.        │                      │                      │
       │  {device_id}       │                      │                      │
       │────────────────────>│  2. Forward          │                      │
       │                     │─────────────────────>│                      │
       │                     │                      │  3. UpsertPoints()   │
       │                     │                      │  ─────────────────>  │
       │                     │                      │  (BoltDB)            │
       │                     │                      │  4. WebSocket        │
       │                     │                      │  (point_synced)     │
       │                     │                      │─────────────────────>│
       │                     │                      │                      │
```

##### 3.2.2.2 EdgeX 发布消息

**Subject**: `edgex.points.<node_id>.<device_id>`

**消息格式** (与 MQTT 版本相同):
```json
{
  "header": {
    "message_id": "<唯一消息ID>",
    "timestamp": <Unix毫秒时间戳>,
    "source": "<node_id>",
    "message_type": "point_sync",
    "version": "1.0"
  },
  "body": {
    "node_id": "<节点ID>",
    "device_id": "<设备ID>",
    "timestamp": <数据时间戳>,
    "points": {
      "<点位名称1>": {
        "value": <当前值>,
        "quality": "good|uncertain|bad",
        "timestamp": <点位时间戳>
      }
    },
    "quality": "good"
  }
}
```

**示例**:
```bash
# 发布点位全量同步
nats pub "edgex.points.edgex-node-001.Room_FC_2014_19" '{
  "header": {
    "message_id": "nats-point-sync-001",
    "timestamp": 1744680015000,
    "source": "edgex-node-001",
    "message_type": "point_sync",
    "version": "1.0"
  },
  "body": {
    "node_id": "edgex-node-001",
    "device_id": "Room_FC_2014_19",
    "timestamp": 1744680015000,
    "points": {
      "Temperature": {
        "value": 25.5,
        "quality": "good",
        "timestamp": 1744680010000
      },
      "Humidity": {
        "value": 65.2,
        "quality": "good",
        "timestamp": 1744680010000
      }
    },
    "quality": "good"
  }
}'
```

**步骤**:

1. 确保节点和设备已成功同步（参考测试 3.2.1）
2. EdgeX 发布点位全量同步到 `edgex.points.<node_id>.<device_id>`
3. 验证 WebSocket 推送 `point_synced` 事件
4. 验证 edgeOS 接收到所有点位数据
5. 验证点位数据在 Web UI 中正确显示

**预期结果**:
- ✅ EdgeOS 成功接收点位全量同步消息
- ✅ WebSocket 推送 `point_synced` 事件
- ✅ 所有点位数据正确存储到 BoltDB
- ✅ 点位数据正确显示在 Web UI 中

#### 测试 3.2.3: 实时数据推送

**目的**: 验证实时数据能够通过 NATS 推送到 edgeOS

##### 3.2.3.1 时序流程图

```
┌──────────────┐     ┌────────────────┐     ┌────────────────┐     ┌──────────────────┐
│    EdgeX     │     │ NATS Server    │     │    EdgeOS     │     │     Web UI       │
└──────┬───────┘     └───────┬────────┘     └───────┬──────┘     └────────┬─────────┘
       │                     │                      │                      │
       │  1. Publish         │                      │                      │
       │  data_report        │                      │                      │
       │  edgex.data.        │                      │                      │
       │  {node_id}.         │                      │                      │
       │  {device_id}        │                      │                      │
       │────────────────────>│  2. Forward          │                      │
       │                     │─────────────────────>│                      │
       │                     │                      │  3. UpdateValue()   │
       │                     │                      │  ─────────────────>  │
       │                     │                      │  (BoltDB)            │
       │                     │                      │  4. WebSocket        │
       │                     │                      │  (data_update)       │
       │                     │                      │─────────────────────>│
       │                     │                      │                      │
```

##### 3.2.3.2 EdgeX 发布消息

**Subject**: `edgex.data.<node_id>.<device_id>`

**消息格式** (与 MQTT 版本相同):
```json
{
  "header": {
    "message_id": "<唯一消息ID>",
    "timestamp": <Unix毫秒时间戳>,
    "source": "<node_id>",
    "message_type": "data",
    "version": "1.0"
  },
  "body": {
    "node_id": "<节点ID>",
    "device_id": "<设备ID>",
    "timestamp": <数据时间戳>,
    "points": {
      "<点位名称1>": <值>,
      "<点位名称2>": <值>
    },
    "quality": "good|uncertain|bad"
  }
}
```

**示例**:
```bash
# 发布实时数据
nats pub "edgex.data.edgex-node-001.Room_FC_2014_19" '{
  "header": {
    "message_id": "nats-data-001",
    "timestamp": 1744680020000,
    "source": "edgex-node-001",
    "message_type": "data",
    "version": "1.0"
  },
  "body": {
    "node_id": "edgex-node-001",
    "device_id": "Room_FC_2014_19",
    "timestamp": 1744680020000,
    "points": {
      "Temperature": 25.5,
      "Humidity": 65.2,
      "Setpoint": 24.0,
      "FanSpeed": 75
    },
    "quality": "good"
  }
}'
```

**步骤**:

1. 确保节点和设备已成功同步（参考测试 3.2.1 和 3.1.2）
2. EdgeX 发布实时数据到 `edgex.data.<node_id>.<device_id>`
3. 验证 WebSocket 推送 `data_update` 事件
4. 验证 edgeOS 接收到实时数据
5. 验证数据在 Web UI 中正确显示

**预期结果**:
- ✅ EdgeOS 成功接收实时数据
- ✅ WebSocket 推送 `data_update` 事件
- ✅ 数据值正确显示在 Web UI 中

#### 测试 3.2.4: 请求/响应模式

**目的**: 验证 NATS 请求/响应模式，用于 edgeOS 查询节点信息或下发命令

##### 3.2.4.1 时序流程图

```
┌──────────────┐     ┌────────────────┐     ┌────────────────┐     ┌──────────────────┐
│    EdgeX     │     │ NATS Server    │     │    EdgeOS     │     │     Web UI       │
└──────┬───────┘     └───────┬────────┘     └───────┬──────┘     └────────┬─────────┘
       │                     │                      │                      │
       │                     │  1. Request          │                      │
       │                     │  edgex.req.node.info  │                      │
       │                     │<─────────────────────│                      │
       │  2. Receive          │                      │                      │
       │  request            │                      │                      │
       │<────────────────────│                      │                      │
       │                     │                      │                      │
       │  3. Process          │                      │                      │
       │  & Respond          │                      │                      │
       │────────────────────>│  4. Deliver          │                      │
       │  edgex.res.node.info │─────────────────────>│                      │
       │                     │                      │  5. WebSocket        │
       │                     │                      │  (node_status)       │
       │                     │                      │─────────────────────>│
       │                     │                      │                      │
```

##### 3.2.4.2 请求消息格式

**Subject**: `edgex.req.node.info`

**请求消息格式**:
```json
{
  "header": {
    "message_id": "<唯一消息ID>",
    "timestamp": <Unix毫秒时间戳>,
    "source": "edgeos",
    "message_type": "node_info_request",
    "version": "1.0",
    "correlation_id": "<关联ID>"
  },
  "body": {
    "node_id": "<节点ID>"
  }
}
```

##### 3.2.4.3 响应消息格式

**Subject**: `edgex.res.node.info`

**响应消息格式**:
```json
{
  "header": {
    "message_id": "<响应消息ID>",
    "timestamp": <响应时间戳>,
    "source": "<node_id>",
    "destination": "edgeos",
    "message_type": "node_info_response",
    "version": "1.0",
    "correlation_id": "<关联ID>"
  },
  "body": {
    "node_id": "<节点ID>",
    "node_name": "<节点名称>",
    "status": "online",
    "device_count": <设备数量>,
    "uptime": <运行时间>
  }
}
```

**示例**:
```bash
# 使用 NATS 请求/响应模式
nats req "edgex.req.node.info" '{
  "header": {
    "message_id": "nats-req-001",
    "timestamp": 1744680050000,
    "source": "edgeos",
    "message_type": "node_info_request",
    "version": "1.0",
    "correlation_id": "req-info-001"
  },
  "body": {
    "node_id": "edgex-node-001"
  }
}'

# 订阅响应
nats sub "edgex.res.node.info"
```

**步骤**:

1. EdgeX 订阅请求主题 `edgex.req.node.info`
2. edgeOS 发送请求到 `edgex.req.node.info`
3. EdgeX 接收请求并处理
4. EdgeX 发送响应到 `edgex.res.node.info`
5. 验证 edgeOS 接收到响应

**预期结果**:
- ✅ EdgeX 成功接收请求
- ✅ EdgeX 正确处理请求
- ✅ EdgeX 发送响应
- ✅ edgeOS 成功接收响应

---

## 性能测试

### 4.1 MQTT 性能测试

#### 测试 4.1.1: 消息吞吐量测试

**目的**: 测试 MQTT 消息的吞吐量

**步骤**:

1. EdgeX 以不同速率发布消息
2. 测量 edgeOS 接收消息的速率
3. 记录消息丢失率

**测试场景**:

| 消息速率 | 持续时间 | 预期结果 |
|---------|---------|---------|
| 100 msg/s | 1 分钟 | ✅ 无消息丢失 |
| 1000 msg/s | 1 分钟 | ✅ 无消息丢失 |
| 5000 msg/s | 1 分钟 | ✅ 消息丢失率 < 0.01% |
| 10000 msg/s | 1 分钟 | ⚠️ 消息丢失率 < 1% |

**测试脚本**:

```bash
#!/bin/bash

# 性能测试脚本
RATE=1000  # 消息速率 (msg/s)
DURATION=60 # 持续时间 (秒)

echo "Starting MQTT performance test: $RATE msg/s for $DURATION seconds"

for ((i=1; i<=$RATE*$DURATION; i++)); do
    mosquitto_pub -h 127.0.0.1 -p 1883 -t "edgex/data/test/device" -m "{\"value\": $i}" &
    if ((i % $RATE == 0)); then
        sleep 1
    fi
done

echo "Performance test completed"
```

#### 测试 4.1.2: 消息延迟测试

**目的**: 测试 MQTT 消息的端到端延迟

**步骤**:

1. EdgeX 发布带有时间戳的消息
2. edgeOS 接收消息并计算延迟
3. 统计延迟分布

**预期结果**:

| QoS 级别 | 平均延迟 | P99 延迟 |
|---------|---------|---------|
| 0 | < 10ms | < 50ms |
| 1 | < 20ms | < 100ms |
| 2 | < 50ms | < 200ms |

#### 测试 4.1.3: 并发连接测试

**目的**: 测试 MQTT Broker 的并发连接能力

**步骤**:

1. 启动多个 EdgeX 节点（10、50、100、500、1000）
2. 每个节点定期发送心跳和数据
3. 监控 Broker 性能指标

**预期结果**:

| 并发连接数 | CPU 使用率 | 内存使用 | 结果 |
|-----------|----------|---------|------|
| 10 | < 5% | < 100MB | ✅ 正常 |
| 50 | < 10% | < 200MB | ✅ 正常 |
| 100 | < 20% | < 500MB | ✅ 正常 |
| 500 | < 50% | < 2GB | ⚠️ 可接受 |
| 1000 | < 80% | < 4GB | ⚠️ 可接受 |

### 4.2 NATS 性能测试

#### 测试 4.2.1: 消息吞吐量测试

**目的**: 测试 NATS 消息的吞吐量

**步骤**:

1. EdgeX 以不同速率发布消息
2. 测量 edgeOS 接收消息的速率
3. 记录消息丢失率

**预期结果**:

| 消息速率 | 持续时间 | 预期结果 |
|---------|---------|---------|
| 1000 msg/s | 1 分钟 | ✅ 无消息丢失 |
| 10000 msg/s | 1 分钟 | ✅ 无消息丢失 |
| 50000 msg/s | 1 分钟 | ✅ 无消息丢失 |
| 100000 msg/s | 1 分钟 | ✅ 无消息丢失 |

#### 测试 4.2.2: 消息延迟测试

**目的**: 测试 NATS 消息的端到端延迟

**预期结果**:

| 场景 | 平均延迟 | P99 延迟 |
|------|---------|---------|
| 本地 | < 1ms | < 5ms |
| 局域网 | < 5ms | < 20ms |

---

## 稳定性测试

### 5.1 长时间运行测试

**目的**: 验证系统在长时间运行下的稳定性

**步骤**:

1. 启动 edgeOS 和 EdgeX 节点
2. 持续运行 24 小时
3. EdgeX 定期发送心跳和数据
4. 监控内存、CPU 使用情况
5. 监控连接状态

**预期结果**:
- ✅ 系统稳定运行 24 小时
- ✅ 无内存泄漏
- ✅ 无连接泄漏
- ✅ 无消息丢失

### 5.2 故障恢复测试

#### 测试 5.2.1: 网络中断恢复

**目的**: 验证网络中断后的自动恢复能力

**步骤**:

1. 断开 MQTT/NAT 连接
2. 等待 30 秒
3. 恢复网络连接
4. 验证自动重连
5. 验证消息继续传输

**预期结果**:
- ✅ 自动重连成功
- ✅ 消息继续传输
- ✅ 数据不丢失

#### 测试 5.2.2: Broker 重启恢复

**目的**: 验证 Broker 重启后的自动恢复能力

**步骤**:

1. 重启 MQTT/NATS Broker
2. 等待 30 秒
3. 验证客户端自动重连
4. 验证消息继续传输

**预期结果**:
- ✅ 客户端自动重连
- ✅ 消息继续传输
- ✅ 数据不丢失

#### 测试 5.2.3: 节点重启恢复

**目的**: 验证节点重启后的自动恢复能力

**步骤**:

1. 重启 EdgeX 节点
2. 验证节点重新注册
3. 验证消息继续传输

**预期结果**:
- ✅ 节点重新注册成功
- ✅ 消息继续传输

---

## 压力测试

### 6.1 高负载测试

**目的**: 验证系统在高负载下的表现

**测试场景**:

1. **多节点并发**: 100 个 EdgeX 节点同时连接
2. **高频数据**: 每个节点每秒发送 100 条数据消息
3. **混合消息**: 心跳、数据、命令混合发送

**预期结果**:
- ✅ 系统稳定运行
- ✅ 消息丢失率 < 0.01%
- ✅ 平均延迟 < 100ms

### 6.2 极限测试

**目的**: 找出系统的性能极限

**测试场景**:

1. 逐步增加并发节点数（10、50、100、200、500、1000）
2. 逐步增加消息速率（100、1000、5000、10000 msg/s）
3. 记录系统性能指标
4. 找出系统崩溃点

**预期结果**:
- ✅ 找到系统极限
- ✅ 记录性能瓶颈

---

## 安全测试

### 7.1 认证测试

**目的**: 验证认证机制的有效性

**测试场景**:

1. **有效认证**: 使用正确的用户名密码连接
2. **无效认证**: 使用错误的用户名密码连接
3. **无认证**: 使用匿名连接

**预期结果**:
- ✅ 有效认证成功连接
- ✅ 无效认证被拒绝
- ✅ 无认证根据配置接受或拒绝

### 7.2 TLS 加密测试

**目的**: 验证 TLS 加密的有效性

**测试场景**:

1. 启用 TLS 连接
2. 验证通信加密
3. 验证证书验证

**预期结果**:
- ✅ TLS 连接成功
- ✅ 通信加密生效
- ✅ 证书验证生效

---

## 测试报告模板

### 测试环境

| 项目 | 内容 |
|------|------|
| 测试时间 | YYYY-MM-DD HH:MM:SS |
| 测试人员 | XXX |
| MQTT Broker 版本 | XXX |
| NATS Server 版本 | XXX |
| edgeOS 版本 | XXX |
| EdgeX 版本 | XXX |

### 测试结果

| 测试项 | 测试结果 | 备注 |
|--------|---------|------|
| MQTT 节点注册 | ✅/❌ | |
| MQTT 设备上报 | ✅/❌ | |
| MQTT 实时数据 | ✅/❌ | |
| MQTT 心跳保活 | ✅/❌ | |
| MQTT 控制命令 | ✅/❌ | |
| NATS 节点注册 | ✅/❌ | |
| NATS 实时数据 | ✅/❌ | |
| NATS 请求响应 | ✅/❌ | |
| 性能测试 | ✅/❌ | 吞吐量: XXX msg/s, 延迟: XXX ms |
| 稳定性测试 | ✅/❌ | 运行时长: XX 小时 |
| 压力测试 | ✅/❌ | 最大负载: XXX |
| 安全测试 | ✅/❌ | |

### 问题和建议

| 问题描述 | 严重程度 | 处理状态 |
|---------|---------|---------|
| | | |

---

## 附录

### A. 测试工具

#### A.1 MQTT 测试工具

- **mosquitto_pub**: MQTT 消息发布工具
- **mosquitto_sub**: MQTT 消息订阅工具
- **MQTTX**: MQTT 跨平台客户端工具

安装方法:

```bash
# Ubuntu/Debian
sudo apt-get install mosquitto-clients

# macOS
brew install mosquitto

# Windows
# 下载并安装 Mosquitto
```

#### A.2 NATS 测试工具

- **nats CLI**: NATS 命令行工具

安装方法:

```bash
# 使用 Go 安装
go install github.com/nats-io/natscli/nats@latest

# 使用 Scoop (Windows)
scoop bucket add nats-io https://github.com/nats-io/scoop-nats-io.git
scoop install nats

# 使用 Homebrew (macOS)
brew install nats
```

### B. 常见问题

#### B.1 MQTT 连接失败

**问题**: 无法连接到 MQTT Broker

**排查步骤**:

1. 检查 Broker 是否运行: `docker ps | grep mosquitto`
2. 检查端口是否监听: `netstat -an | grep 1883`
3. 检查防火墙设置
4. 检查用户名密码是否正确

#### B.2 NATS 连接失败

**问题**: 无法连接到 NATS Server

**排查步骤**:

1. 检查 Server 是否运行: `docker ps | grep nats`
2. 检查端口是否监听: `netstat -an | grep 4222`
3. 检查防火墙设置
4. 检查用户名密码是否正确

#### B.3 消息丢失

**问题**: 发送的消息未被接收

**排查步骤**:

1. 检查 Topic/Subject 是否正确
2. 检查 QoS 级别设置
3. 检查网络连接稳定性
4. 查看 Broker 日志

---

**文档版本**: v1.0  
**最后更新**: 2026-04-15  
**维护者**: edgeOS 团队
