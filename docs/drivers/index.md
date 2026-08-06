---
layout: section-index
title: 设备驱动
description: edgeCore 南向采集驱动文档 — Modbus、BACnet、OPC UA、S7、EtherNet/IP、FINS、SNMP、IEC 104、DL/T645、Mitsubishi MC、Profinet IO、KNXnet/IP、EtherCAT
hero_eyebrow: Southbound Drivers
hero_lead: 南向采集驱动的设计文档、测试报告、优化方案与故障分析 — 覆盖 Modbus、BACnet、OPC UA、S7、EtherNet/IP、Omron FINS、SNMP、IEC 104、DL/T645、Mitsubishi MC、Profinet IO、KNXnet/IP、EtherCAT 等工业协议。
hero_buttons:
  - text: 返回首页
    url: ../index.html
    style: primary
  - text: English
    url: index_en.html
    style: secondary
  - text: 测试报告
    url: ../testing/南向驱动测试报告.html
    style: secondary
  - text: 开发计划
    url: ../development_plan/index.html
    style: secondary
---

## 驱动支持矩阵

> 注册来源：`cmd/main.go` 空白导入 · 测试数据：**2026-07-12** · `CGO_ENABLED=0` · 详见 [南向驱动测试报告](../testing/南向驱动测试报告.html)

| 协议 | 注册名 | 状态 | 读 | 写 | 扫描/发现 | 连接管理 | 单元测试（`-short` 覆盖） |
| :--- | :--- | :--- | :---: | :---: | :---: | :---: | :--- |
| Modbus TCP/RTU | `modbus-tcp`, `modbus-rtu`, `modbus-rtu-over-tcp` | 生产就绪 | 是 | 是 | — | 是 | **65.9%** |
| **BACnet IP** | `bacnet-ip` | 生产就绪 | 是 | 是 | Scan + ScanObjects | 故障隔离 | **66.1%** |
| **OPC UA Client** | `opc-ua` | 生产就绪 | 是 | 是 | Scan + ScanObjects | 是 | **47.9%** |
| **Siemens S7** | `s7` | 生产就绪 | 是 | 是 | — | 是 | **61.3%** |
| **EtherNet/IP** | `ethernet-ip` | 生产就绪 | 是 | 是 | — | 是 | **62.2%** |
| **Omron FINS** | `omron-fins` | 生产就绪 | 是 | 是 | — | 是 | **43.3%** |
| **SNMP v2c/v3** | `snmp` | 生产就绪 | 是 | 是 | ScanObjects | 是 | **63.7%** |
| **IEC 60870-5-104** | `iec60870-5-104` | M1 已交付 | 是 | 是 单点遥控 | — | 是 | **60.2%** |
| **DL/T645-2007** | `dlt645` | 生产就绪 | 是 | 是 | — | 是 | **76.5%** ✅ |
| **Mitsubishi SLMP** | `mitsubishi-slmp` | 生产就绪 | 是 | 是 | — | 是 | **70.7%** ✅ |
| **Profinet IO** | `profinet-io` | 生产就绪 | 是 | 是 | — | 是 | **55.9%** |
| **KNXnet/IP** | `knxnet-ip` | 生产就绪 | 是 | 是 | 网关发现 | 是 | **77.2%** ✅ |
| **EtherCAT** | `ethercat` | M1 已交付 | PDO + SDO | PDO + SDO | 是 | 是 | **87.8%** ✅ |

> ConnectionManager（公共组件）：**87.4%** · 主驱动包 **22/22 PASS**（2026-07-12）

### 主要配置参数

| 驱动 | 关键配置项 |
| :--- | :--- |
| Modbus | `ip`, `port`, `slaveId`, `timeout`，连接类型 TCP/RTU/RTU-over-TCP |
| BACnet | `ip`, `port`, `deviceId`，广播网口、对象实例 |
| OPC UA | `endpoint`，安全策略/模式，凭证，订阅间隔 |
| S7 | `ip`, `port`, `rack`, `slot`，PLC 型号 (200Smart/1200/1500/300/400) |
| EtherNet/IP | `ip`, `port`, `slot`，Tag 路径，连接类型 |
| Omron FINS | `plcIP`/`ip`, `plcPort`/`port`, `timeout`，源/目的节点地址，TCP/UDP |
| Mitsubishi MC | `ip`, `port`, `frame_type`, `network_no`, `station_no`, `timeout` |
| SNMP | `snmpVersion`, `targetIP`, `community` (v2c)，USM 认证/加密 (v3)，`maxBulkSize` |
| DLT645 | `connectionType` (serial/tcp), `port`, `ip`, `baudRate`, `timeout`, 表地址 + DI |
| Profinet IO | `local_interface`, `timeout`, `simulation`；设备级 `ip`, `port`, `slot`, `subslot`, `device_name` |
| KNXnet/IP | `ip`, `port`, `mode` (TCP/UDP)，`discovery`, `discovery_timeout`, `discovery_multicast` |
| IEC 104 | `ip`, `port`, `commonAddress`，T0–T3 定时器，总召唤间隔 |
| EtherCAT | `local_interface`, `cycle_time_us`, `simulation`；设备级 `position`, `vendor_id`, `product_code`, `tx_pdo_size`, `rx_pdo_size` |

---

## 目录

### BACnet

> **权威：** [BACnet 设计说明](BACnet_设计说明.html)（架构）· [BACnet 最佳实践](BACnet_最佳实践.html)（开发指南）· [BACnet 前端需求](BACnet_Frontend_UI_Requirements.html)（UI PRD）· [驱动采集验收清单](BACnet_Driver_Collection_Test_Acceptance_Checklist.html)（验收）

- [BACnet 设计说明](BACnet_设计说明.html)
- [BACnet 最佳实践](BACnet_最佳实践.html) — 开发指南与 4-Phase 验证
- [BACnet 驱动采集测试验收清单](BACnet_Driver_Collection_Test_Acceptance_Checklist.html)
- [BACnet 测试文档](BACnet_测试文档.html)
- [BACnet 故障隔离报告](BACnet_Fault_Isolation_Report.html) — 历史归档
- [BACnet 前端功能清单](BACnet_Frontend_UI_Functionality_Checklist.html) — 索引
- [BACnet 前端需求](BACnet_Frontend_UI_Requirements.html)
- [BACnet 多设备隔离测试计划](BACnet_Multi_Device_Isolation_Test_Plan.html) — 索引
- [BACnet 点位串流 bug](BACnet点位串流bug.html) — 历史归档
- [API BACnet](API_BACnet.html)

### Modbus

> **权威：** [边缘网关 Modbus 优化](边缘网关Modbus优化.html)

- [Modbus 优化](MODBUS_OPTIMIZATION.html) — 批量读索引
- [Modbus 优化最终报告](MODBUS_OPTIMIZATION_FINAL.html) — 历史归档
- [Modbus 优化报告](MODBUS_OPTIMIZATION_REPORT.html) — 历史归档
- [Modbus 心跳优化](MODBUS_HEARTBEAT_OPTIMIZATION.html) — 索引
- [Modbus 智能探测](Modbus智能探测.html)
- [边缘网关 Modbus 优化](边缘网关Modbus优化.html)

### OPC UA
- [OPC UA 设计](OPC_UA_Design.html)
- [OPC UA 服务端功能](OPC-UA_Server_Functionality.html)
- [OPC UA UI 审查](OPC_UA_UI审查.html)

### S7 协议
- [S7 协议](PLC_S7.html)
- [S7 连接生命周期系统](S7_Connection_Lifecycle.html)

### Profinet IO
- [Profinet IO 驱动](PLC_Profinet_IO.html)

### EtherNet/IP
- [EtherNet/IP 真实通信实现方案](EtherNet_IP驱动真实通信实现方案.html)

### Omron FINS
- [FINS 协议驱动](PLC_FINS.html)

### Mitsubishi MC
- [三菱 MC Protocol 驱动](PLC_MITSUBISHI.html)

### SNMP
- [SNMP 驱动说明](SNMP.html)

### DL/T 645
- [DL/T 645-2007 驱动](DLT645.html)
- [开发方案](../TODO/DLT-645-2007/DL-T-645-2007驱动开发.html)

### IEC 60870-5-104
- [ICE104 开发计划](../development_plan/drivers/采集驱动ICE104开发.html)

### EtherCAT
- [EtherCAT 采集驱动规划](../TODO/EtherCAT/采集驱动EtherCAT规划.html)

### 测试报告
- [南向驱动测试报告](../testing/南向驱动测试报告.html)
- [Southbound Driver Test Report (EN)](../testing/southbound-driver-test-report.html)

---

## 连接管理系统 (2026-06)

### ConnectionManager 公共组件

**核心特性**:
- 统一连接状态机：`Disconnected → Connecting → Connected → Retrying → Dead`
- 指数退避算法：`backoff = min(base_delay × 2^retry_count, max_delay) + jitter`
- 冷却期策略：基础冷却 1 分钟，指数增长，最大 1 小时
- 每日清零机制：每日零点自动重置重试计数与冷却次数

**适用驱动**: S7、Modbus、EtherNet/IP、OPC UA、FINS、SNMP、DL/T645、Profinet IO、KNXnet/IP、EtherCAT

### 采集健康检测

> 采集成功 = 连接健康 · 采集失败 = 连接异常 · 连续失败达到阈值 → 触发状态变更

| 驱动/型号 | 最大失败次数 | 默认采集周期 | 说明 |
| :--- | :--- | :--- | :--- |
| **S7-200Smart** | 3 次 | 60 秒 | 弱 PLC，保护设备 |
| **S7-1200/1500** | 5 次 | 10 秒 | 标准 PLC |
| **Modbus** | 5 次 | 可配置 | 通用设置 |
| **EtherNet/IP** | 5 次 | 可配置 | Rockwell 系列 |
| **OPC UA** | 5 次 | 订阅回调触发 | 订阅数据质量判断 |
| **FINS** | 5 次 | 可配置 | 欧姆龙 PLC |
| **SNMP** | 5 次 | 可配置 | 网络设备 |
| **EtherCAT** | 5 次 | 周期可配置 | PDO 周期性数据交换 |

### 低频采集补偿探测

当采集周期超过 3 倍阈值时，自动触发轻量探测请求：
- S7：读取 M 区 1 字节
- Modbus：读取单个寄存器
- EtherNet/IP：读取单个 Tag
- OPC UA：读取 ServerStatus 节点
- EtherCAT：读取从站 TxPDO 状态

---

## 相关文档

- [开发计划](../development_plan/index.html) — 待开发驱动规划
- [架构设计](../architecture/index.html) — ScanEngine 与 ShadowCore
- [测试验证](../testing/index.html) — 测试方案与报告
- [运维文档](../operations/index.html) — 驱动连接修复与运维
