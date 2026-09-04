---
layout: default
title: OPC UA 客户端驱动设计方案
description: edgeCore OPC UA 客户端驱动设计方案
---

# OPC UA 客户端驱动设计方案

## 1. 概述
本方案旨在为工业边缘网关（Industrial Edge Gateway）增加 OPC UA 客户端支持，基于开源库 `github.com/gopcua/opcua` 实现。该驱动将遵循项目现有的统一驱动接口（Driver Interface），支持设备接入、安全认证、地址空间浏览（点位扫描）、数据采集（轮询与订阅）、异常恢复及数据持久化。

## 2. 架构设计

### 2.1 驱动集成
OPC UA 驱动将位于 `internal/driver/opcua` 包中，并实现 `internal/driver/interface.go` 中定义的以下接口：
- **Driver**: 基础生命周期管理（Init, Connect, Disconnect）及读写操作（ReadPoints, WritePoint）。
- **Scanner**: (可选) 实现端点（Endpoint）发现。
- **ObjectScanner**: 实现地址空间浏览，用于自动发现设备点位。

### 2.2 依赖库
- 核心库: `github.com/gopcua/opcua`
- 辅助库: `github.com/gopcua/opcua/ua` (OPC UA 数据类型与服务)

## 3. 功能实现细节

### 3.1 配置结构 (channels.yaml)
在 `channels.yaml` 中，OPC UA 通道配置将扩展以支持以下字段：

```yaml
channels:
  - id: "opcua-line-1"
    name: "OPC UA 产线1"
    protocol: "opc-ua"
    enable: true
    polling_interval: 1000 # 默认轮询周期 (ms)
    devices:
      - id: "plc-01"
        name: "西门子 PLC"
        config:
          endpoint: "opc.tcp://192.168.1.10:4840"
          security_policy: "Basic256Sha256" # None, Basic128Rsa15, Basic256, Basic256Sha256
          security_mode: "SignAndEncrypt" # None, Sign, SignAndEncrypt
          auth_method: "UserName" # Anonymous, UserName, Certificate
          username: "admin"
          password: "password123"
          certificate_file: "/path/to/cert.pem" # 仅当 auth_method 为 Certificate 时需要
          private_key_file: "/path/to/key.pem"
```

### 3.2 连接与安全认证
驱动将根据配置构建 `opcua.Client`：
- **安全策略**: 自动发现服务端支持的 Endpoint，并匹配配置的 Policy 和 Mode。
- **认证方式**: 支持匿名、用户名/密码、证书认证。
- **会话管理**: 建立 Session，并处理 KeepAlive。

### 3.3 地址空间浏览（点位扫描）
实现 `ObjectScanner` 接口的 `ScanObjects` 方法：
- **递归浏览**: 从 RootFolder (或指定起始节点) 开始，使用 `Browse` 服务遍历 Objects 文件夹。
- **点位模型同步**: 将发现的 Variable 节点转换为系统通用的 `Point` 模型。
  - `Address`: 节点的 NodeID (如 `ns=2;s=Demo.Static.Scalar.Double`)。
  - `Name`: BrowseName 或 DisplayName。
  - `DataType`: 映射 OPC UA 数据类型到系统类型 (Float, Int, Boolean, String)。
  - `Access`: 根据 AccessLevel 判断是否可读写。

### 3.4 数据采集

#### 3.4.1 周期轮询 (Polling)
- 实现 `ReadPoints` 接口。
- 使用 `Read` 服务批量读取 NodeID 的 Value 属性。
- 处理 `BadNodeIdUnknown` 等错误，并返回系统标准数据格式。

#### 3.4.2 实时订阅 (Subscription) - 推荐模式
- 在 `Connect` 成功后，建立 OPC UA Subscription。
- 为配置的点位创建 Monitored Items。
- **数据变更通知**: 监听 `Publish` 请求返回的 Notification，实时更新内存中的点位值，减少轮询开销。
- **数据质量与时间戳**: 提取 SourceTimestamp 和 StatusCode，用于数据质量判断。

### 3.5 异常恢复与会话重建
- **连接监控**: 监听 KeepAlive 失败或网络断开错误。
- **自动重连**: 启动后台 Goroutine 进行指数退避重连。
- **会话重建**: 重连成功后，自动重新创建 Session 和 Subscription，恢复 Monitored Items。

### 3.6 分钟级结果存储 (BBLOT)
复用并扩展现有的存储机制：
- **存储引擎**: 使用 `bbolt` (BoltDB)。
- **数据结构**: 扩展 `DeviceStorageManager` 或创建新的 `OpcUaHistoryManager`。
- **存储策略**:
  - **Minute Snapshot**: 每分钟对采集到的点位数据进行快照（Avg/Last/Min/Max）。
  - **Key Design**: `Bucket: opcua_history`, Key: `timestamp_deviceID`.
  - **API**: 提供 `/api/history/opcua` 接口供前端查询历史趋势。

## 4. 前端 UI 详细设计 (基于现有优化)

本章节基于现有的 UI 框架进行扩展，仅针对 OPC UA 协议特性进行增强，保持整体风格一致。

### 4.1 设备管理 UI（OPC UA 专区）
**页面路径**: 设备管理 → 添加设备 → 协议类型 = OPC UA

#### 4.1.1 表单字段（分组展示）
*   **基本信息**
    *   **设备名称**
    *   **设备ID**
    *   **协议类型**：OPC UA（只读）
    *   **启用状态**（开关）

*   **连接配置**
    *   **Endpoint URL**（文本框，如 `opc.tcp://192.168.1.10:4840`）
    *   **安全策略**（下拉）：
        *   None
        *   Basic128Rsa15
        *   Basic256
        *   Basic256Sha256
    *   **安全模式**（单选）：
        *   None
        *   Sign
        *   SignAndEncrypt
    *   **认证方式**（切换式）：
        *   Anonymous
        *   Username / Password (显示用户名/密码输入框)
        *   Certificate (显示证书文件/私钥文件上传控件)

*   **高级配置（折叠区）**
    *   **命名空间过滤**（多选下拉）
    *   **自动扫描点位**（开关）
    *   **自动订阅点位**（开关）
    *   **默认采集质量过滤**（多选：Good / Uncertain / Bad）
    *   **默认采集模式**（订阅 / 轮询）

#### 4.1.2 设备列表展示字段增强
在设备列表中新增 OPC UA 专属状态列：

| 字段 | 说明 |
| :--- | :--- |
| **连接状态** | Connected / Disconnected / Reconnecting |
| **安全模式** | Sign / SignAndEncrypt |
| **会话状态** | Active / Expired |
| **Subscription 状态** | Active / Dropped |
| **最近心跳** | 时间戳 |
| **最近错误** | 简要错误信息 |

**支持操作**：
*   测试连接
*   重新连接
*   导出配置
*   查看会话详情
*   查看历史趋势
*   查看 bblot 统计

### 4.2 点位扫描与同步 UI（重点模块）
**页面路径**: 采集通道 → 设备详情列表 → 点位管理 → 扫描点位

#### 4.2.1 扫描对话框结构
*   **扫描配置区（顶部）**
    *   **起始节点**（默认 RootFolder）
    *   **最大深度**（默认 10）
    *   **命名空间过滤**（多选）
    *   **NodeClass 过滤**（默认 Variable）
    *   **访问权限过滤**（默认 Read + ReadWrite）

*   **扫描结果区（主体）**
    *   展示方式支持：
        *   🌲 **树状结构**（对象 → 子对象 → 点位）
        *   📋 **列表结构**（可分页、可搜索）
    *   每行字段：
        *   勾选框
        *   NodeId
        *   DisplayName
        *   DataType
        *   Access
        *   Unit
        *   Namespace
        *   Description
    *   支持功能：
        *   批量勾选
        *   按名称/类型/命名空间搜索
        *   快速筛选只显示可写点位
        *   预览点位实时值（可选）

#### 4.2.2 点位导入配置弹窗
*   **导入策略选择**：
    *   仅新增
    *   覆盖已有点位
    *   差异更新
    *   手动处理冲突
*   **字段映射确认**：
    *   `Point.Name` ← `DisplayName`
    *   `Point.Address` ← `NodeId`
    *   `Point.DataType` ← `DataType`
    *   `Point.Unit` ← `EngineeringUnit`
    *   `Point.Access` ← `AccessLevel`

### 4.3 实时采集状态 UI
**页面路径**: 设备详情 → 实时数据

*   **展示内容**：
    *   点位名称
    *   当前值
    *   时间戳
    *   质量码（Good / Uncertain / Bad）
    *   采集模式（Subscription / Polling）
    *   最近一次更新时间
    *   最近错误信息（如 Bad）

*   **支持操作**：
    *   手动刷新
    *   单点写入（Write）
    *   批量写入（Write 多点）
    *   切换采集模式（仅管理员）

### 4.4 历史趋势 UI（基于 bblot）
**页面路径**: 设备详情 → 历史趋势 → OPC UA

*   **功能支持**：
    *   按点位查询历史趋势
    *   支持分钟级粒度
    *   支持 Last / Min / Max / Avg 切换
    *   支持时间范围选择
    *   支持导出 CSV

### 4.5 运维诊断 UI（工程级增强）
**页面路径**: 设备详情 → 运维诊断 → OPC UA

*   **展示内容**：
    *   SecureChannel 状态
    *   Session ID
    *   Subscription ID
    *   最近重建时间
    *   会话重建次数
    *   Subscription 重建次数
    *   丢包率
    *   平均采集延迟
    *   最近 24 小时 bblot 统计曲线

## 5. 开发计划

1. **基础驱动框架**: 实现 Connect/Disconnect 及基础配置解析。
2. **点位扫描**: 实现 Browse 功能，能够列出服务端节点。
3. **读写功能**: 实现 Read/Write 接口。
4. **订阅功能**: 实现 Subscription 模式以优化性能。
5. **异常处理**: 完善断线重连机制。
6. **存储集成**: 对接 bbolt 进行历史数据存储。
7. **UI 开发**:
    - **设备管理**: 实现 OPC UA 专用配置表单及状态列增强。
    - **点位扫描**: 开发树状/列表扫描结果页及导入逻辑。
    - **实时数据**: 适配订阅/轮询模式展示及写入操作。
    - **历史趋势**: 集成 bblot 查询 API 进行趋势渲染。
    - **运维诊断**: 开发连接状态与统计指标看板。

## 6. 验证方案
- 使用 `open62541` 或 `Prosys OPC UA Simulation Server` 作为测试服务端。
- 验证不同安全策略（None, Basic256Sha256）的连接性。
- 验证网络中断后的自动恢复能力。
- 验证 1000+ 点位的订阅性能。
