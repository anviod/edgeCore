---
layout: section-index
title: 开发计划
description: edgeCore 开发计划与路线图 — Q3/Q4 驱动交付、ScanEngine、ShadowCore、多节点同步
hero_eyebrow: Roadmap & Planning
hero_lead: edgeCore 项目开发规划、路线图与待实现功能 — 驱动扩展、ScanEngine 采集优化、ShadowCore 影子设备与多节点同步通信。
hero_buttons:
  - text: 返回首页
    url: ../index.html
    style: primary
  - text: 驱动总览
    url: ../drivers/index.html
    style: secondary
  - text: 架构设计
    url: ../architecture/index.html
    style: secondary
---

## 战略路线图

正式分阶段路线图与发布门禁（**优先于本页 Q3/Q4 功能列表**）：

- [**分阶段开发路线图**](../ROADMAP.html) — Phase 1 稳定性 → Phase 2 工业验证 → Phase 3 性能 → Phase 4 可观测
- [**开发原则与验收标准**](../DEVELOPMENT_PRINCIPLES.html) — 量化验收与工程铁律
- [**版本发布门禁**](../RELEASE_GATE.html) — G-Stability / G-Industrial / G-Performance / G-Lightweight

---

## 路线图总览

### 已交付

#### 驱动支持
- **Modbus TCP/RTU**: 完整支持，智能 MTU 探测与指数退避
- **BACnet IP**: 设备发现、对象扫描、点位读写、半开探测
- **OPC UA 客户端**: 订阅与监控，断线自动重连
- **Siemens S7**: S7-200Smart/1200/1500/300/400 全系列
- **EtherNet/IP (ODVA)**: Rockwell PLC 全系列支持
- **Omron FINS TCP/UDP**: CIO/D/W/H/EM 区域批量读写
- **SNMP v2c/v3**: Community / USM 认证，OID 批量采集
- **IEC 60870-5-104 M1**: 总召唤、自发上报、单点遥控
- **EtherCAT M1** (v0.0.8): PDO 周期数据交换、CoE SDO 邮箱、模拟模式、87.8% 覆盖率

#### 核心特性
- **ScanEngine 调度驱动内核**（2026-06 已落地）：10ms Tick + PriorityQueue + ExecutionLayer 三路分发
- ConnectionManager 公共连接管理组件（唯一 dial Owner）
- 全驱动采集健康检测（取消独立心跳）
- 指数退避 + 冷却期策略 + single-flight 重连
- ShadowCore 影子设备系统 + ShadowBridge → DataPipeline 扇出
- RTT/MTU/Gap 画像模块（ExecutionLayer 读路径闭环 — Q3-B 已交付）

### 进行中

| 模块 | 状态 | 预计完成 | 说明 |
| :--- | :--- | :--- | :--- |
| **工业验证 Phase 2** | 进行中 | Q3 2026 | 各协议联机长跑、断网恢复与统一验证报告 |
| **ARMv7 板端验收** | 进行中 | Q3 2026 | 目标硬件 2h/72h 长跑与 Shadow/SLA 板端复验 |
| **ShadowCore 跨通道聚合** | 进行中 | Q3 2026 | 虚拟影子跨通道引用与多源点位聚合 |
| **多节点同步通信** | 预研中 | Q3 2026 | 基于 go-libp2p 的分布式配置同步 |
| **高可用接管** | 预研中 | Q3 2026 | 故障自动接管与租约机制 |
| **IEC 104 M2** | 待启动 | Q4 2026 | 遥调、时钟同步、SOE、双点遥控 |

### 2026年7月（新增已交付）

- [已交付] **EtherCAT M1（v0.0.8）** — PDO/SDO、模拟主站、87.8% 覆盖；依赖 v1.0.3
- [已交付] **Q3 南向采集闭环** — 统一数据面、Scan Class、块读、点位降级、Diagnostics API
- [已交付] **SLA 调度达标** — Phase A–C 综合达标率 ≥95%，EDF/硬抖动钳制
- [已交付] **ShadowCore 性能优化** — COW 快照、Worker Pool、ShadowIngress 批量写入
- [已交付] **北向统一重连** — MQTT/NATS/Sparkplug B 公共 reconnect 模块
- [已交付] **BACnet Server 北向从机模式** — Who-Is/I-Am 设备发现、ReadProperty/WriteProperty 双向读写、COV 订阅；将 edgeCore 南向点位映射为 BACnet 标准对象暴露给 BMS/SCADA
- [已交付] **Dashboard v3 UI** — Linear 级 SaaS 样式改版与 Soak 监控面板
- [已交付] **版本发布门禁** — G-Stability/Industrial/Performance/Lightweight 四道门禁
- [已交付] **虚拟影子设备体验** — 编辑流程、帮助文档与跨页面样式统一
- [已交付] **南向驱动全量回归（2026-07-11/12）** — 主驱动包 **22/22 PASS**（含 EtherCAT）；覆盖率矩阵见 [测试报告](../testing/南向驱动测试报告.html)
- [已交付] **Mac 万 Tag / ScanEngine 复测（2026-07-12）** — lag P95 1.56ms · G007 986 设备/s · Soak 五 gate；见 [压力测试报告](../testing/压力测试报告.html)
- [已交付] **热路径单测补强（2026-07-12）** — core 80% · ai_agent 91.4% · Modbus/ENIP ≥60%
- [已交付] **Industrial Protocol Copilot MVP** — AI 助手面板与热路径单测；工业联调验收进行中
- [已交付] **产品手册** — [PRODUCT.zh-CN](../guide/PRODUCT.zh-CN.html) / [PRODUCT](../guide/PRODUCT.html)

---

## 目录

### 驱动开发计划
- [DL/T 645-2007 驱动开发](drivers/DL-T-645-2007驱动开发.html)
- [IEC 60870-5-104 驱动开发](drivers/采集驱动ICE104开发.html)
- [Omron FINS TCP 驱动开发](drivers/采集驱动Omron%20FINS%20TCP开发.html)
- [SNMP 驱动开发](drivers/SNMP采集驱动开发.html)

### 多节点同步通信
- [基于 go-libp2p 同步通信规划方案](../TODO/基于go-libp2p%20同步通信规划方案.html)（权威 · TODO）
- [联机测试方案](sync/联机测试方案.html)

### edgeCore Industrial Protocol Copilot（AI 协同 · MVP 已落地）
- [**AI 协同组件规划 / Industrial Protocol Copilot**](../TODO/AI协同组件规划.html)（权威 · **V1.4**；**MVP 已落地**，工业联调验收进行中）
  - 工业协议工程 Copilot：厂家资料 + 报文分析 → 生产可部署 Channel/Point/Driver/Validation 配置
  - **协议逆向工程引擎 P0+ 核心** — PCAP/监控表关联、无文档设备接入
  - 部署：**RK3588 边缘自治 + AI Model Center 分离**（gRPC / MQTT 弱网回退）
  - 关联：[设备点位读写升级](../TODO/设备点位读写系统升级改造计划.html) · [边缘计算 2.0](../TODO/边缘计算优化升级2.0.html)

---

## Q3 2026 重点交付（主体已完成）

1. **ScanEngine 南向采集优化（✅ 2026-07 交付）**
   - ScanEngine：事件驱动堆、EDF、hard jitter clamp、CB、Scan Class、diagnostics SLA
   - ExecutionLayer：Serial 硬隔离 + Parallel 背压 + Gap/MTU 块读分片
   - RTT 管理器：Execute 后 `UpdateDeviceRTT` + adaptive throttle
   - 点位降级：`point_degradation_manager.go` — 故障 Tag 隔离

2. **ShadowCore 影子设备（✅ 2026-07 性能优化）**
   - ShadowIngress 批量写入 + COW 快照 + Worker Pool
   - 统一内部数据模型；纯内存运行时 SoT
   - 真实/虚拟影子设备支持

3. **SLA 调度达标（✅ Phase A–D 核心）**
   - B1–B5 ≥95%；P95 lag ~7ms（10k mock）
   - diagnostics API + `sla_warnings` + UI 通道监控
   - 详见 [SLA 评估](../TODO/SLA评估.html)

4. **DL/T 645-2007 驱动**（2026-06 已交付）
   - 电能量采集（有功/无功电能）
   - 需量采集与变量采集

5. **多节点同步通信**（预研中）
   - 基于 go-libp2p 的 P2P 网络
   - 配置自动发现与同步
   - 设备控制权租约机制

---

## 核心特性规划

### 多节点同步通信 (Hybrid Sync Model)

**定位**: 分布式配置与控制权同步系统，而非数据同步系统

**三层一致性模型**:
- **Config 层** → 最终一致 (Eventual Consistency)
- **Ownership 层** → 租约约束 (Lease)
- **Runtime 层** → 单点主控 (Owner Only)

**核心价值**:
- 0 配置运维：节点接入网络自动组网
- 高可用保障：单点故障秒级接管
- 工业协议适配：Exclusive/Shared/Lease 三种访问模式
- 轻量级实现：内存占用 < 50MB，ARMv7 友好

> 详细方案请参考 [基于 go-libp2p 同步通信规划方案](../TODO/基于go-libp2p%20同步通信规划方案.html)

---

## 项目进度跟踪

### 2026年6月
- [已交付] IEC 104 M1 交付（总召唤、自发上报、遥控）
- [已交付] SNMP v2c/v3 与 Omron FINS 驱动完成
- [已交付] ConnectionManager 公共组件发布
- [已交付] 全驱动采集健康检测集成
- [已交付] CGO-free CI 流水线稳定
- [已交付] **ScanEngine 调度驱动内核**（10ms Tick + ExecutionLayer + 13 协议迁移）
- [已交付] DL/T645、Mitsubishi SLMP、Profinet IO、KNXnet/IP 驱动
- [预研中] 多节点同步通信方案设计

### 2026年5月
- [已交付] S7 协议完整支持
- [已交付] 连接生命周期系统构建
- [已交付] 指数退避与冷却期策略

---

## 相关文档

- [驱动总览](../drivers/index.html) — 已支持驱动的完整文档
- [架构设计](../architecture/index.html) — ScanEngine SLA 调度、ShadowCore COW 与 ShadowIngress 设计
- [边缘计算](../edge/index.html) — 边缘计算功能与场景
- [测试验证](../testing/index.html) — 测试方案与验证报告
- [Q3 采集优化方案](../%5BTODO%5D边缘计算南向采集优化方案2026第三季度.html) — ScanEngine 详细规划
