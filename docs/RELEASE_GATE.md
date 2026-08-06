---
layout: default
title: 版本发布门禁
description: edgeCore 每版本发布前的四道门禁 — 稳定性、工业验证、性能与轻量化
version: v1.0
date: 2026-07-03
status: 现行
---

# 版本发布门禁

> **工程铁律：** 任何性能优化不得以牺牲稳定性为代价；任何架构优化不得增加系统恢复复杂度。

每个 edgeCore 版本在标记 **生产就绪** 或发布 Release 之前，须依次通过以下四道门禁。**任一门禁未通过，不得对外宣称该版本可用于工业现场。**

> 相关文档：
> - [开发原则与验收标准](DEVELOPMENT_PRINCIPLES.html) — 优先级与量化标准
> - [分阶段路线图](ROADMAP.html) — Phase 1–4 与当前阶段约束
> - [CI 与发布门禁对照](testing/CI与发布门禁对照.html) — 文档门禁 vs 当前 CI 覆盖差距
> - [测试矩阵](testing/test_matrix.html) — 回归测试范围
> - [ScanEngine SLA 评估](TODO/SLA评估.html) — 性能阈值细节

---

## 门禁总览

| 门禁 | 代号 | 核心问题 | 未通过后果 |
| --- | --- | --- | --- |
| 稳定性门禁 | **G-Stability** | 长时运行是否可靠、故障是否可恢复 | **阻塞发布** |
| 工业验证门禁 | **G-Industrial** | 真实设备与异常场景是否验证 | **阻塞发布** |
| 性能门禁 | **G-Performance** | Benchmark 是否满足 SLA | **阻塞合并/发布** |
| 轻量化门禁 | **G-Lightweight** | 交付形态是否符合产品定位 | **阻塞发布** |

---

## G-Stability — 稳定性门禁

**目标：** 证明 ScanEngine 运行时基础稳定，单点故障不扩散，系统可长期自治运行。

| 检查项 | 验收标准 | 验证方式 |
| --- | --- | --- |
| 连续运行 | **24–72h** 无死锁、无 panic、无 goroutine 泄漏 | soak test（`//go:build soak`） |
| 内存 | 无明显内存持续增长；heap drift 在阈值内 | 60s/1h/24h 采样 |
| 故障恢复 | 全部 fault recovery 用例通过 | E2E CB、故障传播、混合故障注入 |
| 设备/通道隔离 | 单设备/单通道故障不影响其他采集 | 并行 95/100、串行 6/7 等集成测试 |
| 可观测 | diagnostics 可反映 CB、lag、warnings、通道事件 | API + 日志抽检 |

**出口条件：** Phase 1（稳定性闭环）交付物完成；详见 [ROADMAP — Phase 1](ROADMAP.html#phase-1-稳定性闭环必须完成)。

---

## G-Industrial — 工业验证门禁

**目标：** 各支持协议在真实或高保真仿真环境下通过异常场景验证，而非仅 Mock 单测。

| 检查项 | 验收标准 | 验证方式 |
| --- | --- | --- |
| 协议覆盖 | Modbus TCP、Modbus RTU、OPC UA、S7、DLT645、BACnet、EtherCAT 等**已支持协议**逐项验证 | 联机测试报告 |
| 异常场景 | 断网恢复、PLC 重启、网络抖动、超时、高丢包 | 故障注入 + 现场/仿真器 |
| 长时联机 | 每协议 **24h/72h** 长跑（按协议风险分级） | 统一测试报告 |
| 完成定义 | **设计 + 代码 + 测试报告** 三件套 | PR + 报告归档 |

**说明：** 本门禁为 **验证型**，不要求同期交付新功能。新协议须在 Phase 1 稳定后再进入工业验证队列。

> 报告填写：每协议使用 [工业验证测试报告模板](testing/工业验证测试报告模板.html) 归档；下文 **发布判定标准** 为门禁裁量依据。

### 发布判定标准（PASS / WARN / BLOCK）

| 级别 | 含义 | 对发布的影响 |
| --- | --- | --- |
| **PASS** | 本协议全部必测场景与硬指标达标 | 计入 G-Industrial 通过 |
| **WARN** | 无 BLOCK 项，但存在可解释偏差或仅仿真覆盖 | **有条件发布** — 须在 Release Notes 与报告 §7 注明范围与复测计划 |
| **BLOCK** | 任一硬门禁未满足 | **阻塞发布** — 该协议不得宣称「工业验证通过」 |

**版本级 G-Industrial 汇总：**

- 版本声明**支持**的每一协议，均须有一份结论为 **PASS** 或经签核的 **WARN** 报告。
- 任一**已支持协议**为 **BLOCK**，或必测场景（S1–S6）存在 **FAIL**，则 **G-Industrial 整体 BLOCK**。
- **WARN 不得用于**掩盖 panic、死锁、跨设备污染、不可恢复 CB 或指标持续超标。

### 各协议验证要求

Release 范围包含某协议时，须满足下表最低要求（验证重点见 [ROADMAP — Phase 2](ROADMAP.html#phase-2--工业验证)）：

| 协议 | 长跑时长 | 最低环境 | 额外必证能力 |
| --- | --- | --- | --- |
| **Modbus TCP** | 24h | 现场或多从站仿真 | 多从站块读；单从站离线不拖死同通道 |
| **Modbus RTU** | **72h** | 现场 RS485 或等效仿真 | 串口抖动、超时、从站离线隔离 |
| **OPC UA** | 24h | 现场 PLC 或高保真 Server | 订阅/监控；断线重连；PLC 重启恢复 |
| **Siemens S7** | 24h | 现场 PLC 或认证仿真 | 连接生命周期；读写稳定性；Half-Open 恢复 |
| **DL/T645** | **72h** | 表计或协议仿真 | 异常帧处理；长时抄表稳定 |
| **BACnet** | **72h** | 多设备现场或仿真 | 多设备隔离；发现与读写；单设备故障不扩散 |
| **EtherCAT** | **72h** | 真实 EtherCAT 主站 + 从站 | PDO 周期稳定性；SDO 读写；从站状态切换 |

未在上表列出的**新版本新增协议**：默认 **72h** + 全场景 S1–S6，直至 ROADMAP 另行分级。

### 必测场景与单项通过条件

与报告模板 [§3 测试场景清单](testing/工业验证测试报告模板.html#3-测试场景清单) 对齐；**S1–S6 全部为必测，单项结论须为「通过」方可 PASS**。

| 场景 | 代号 | 单项 PASS 条件（摘要） |
| --- | --- | --- |
| 长时联机 | **S1** | 达到协议要求 24h/72h；无 panic/死锁；goroutine 无持续增长；稳态 `scan_miss_deadline_total` = 0 |
| 网络断开恢复 | **S2** | 恢复后 **≤60s** 内目标设备/通道恢复有效采集；无 channel 级非预期 Offline |
| PLC / 设备重启 | **S3** | 设备侧重启后 **≤120s** 自动恢复采集；CB 可进入 Half-Open 并成功闭合 |
| 网络抖动 | **S4** | 延迟波动/间歇丢包下采集不中断全局调度；同通道其他设备采集成功率 ≥99.9% |
| 超时 | **S5** | 超时触发 CB 与退避；不误伤同通道健康设备；恢复后 lag 回归阈值内 |
| 高丢包 | **S6** | 极端丢包下故障隔离有效；恢复后无数据面永久僵死 |

**共享总线/链路协议**（Modbus RTU、RS485 等）：任一从站故障时，同通道其他从站须保持可采集（与 G-Stability 设备隔离一致）。

### 量化指标阈值（联机 / 板端）

长跑 **稳态窗口**（S1 末 4h 或故障场景恢复后 15min）须满足；与 [G-Performance](#g-performance--性能门禁) 联机列一致，细节见 [SLA评估 — 核心指标](TODO/SLA评估.html#三sla-指标定义)。

| 指标 | 字段 / 来源 | PASS | WARN | BLOCK |
| --- | --- | --- | --- | --- |
| Scan Lag P95 | `scan_lag_p95_ms` | **<150ms** | 150–200ms 且 S2–S6 全过 | **≥200ms** 或稳态 miss deadline >0 |
| Scan Lag P99 | diagnostics | 记录值，无硬门禁 | >300ms 须根因说明 | 持续 >500ms 且影响采集完整性 |
| Scan Drift 均值 | `scan_drift_avg_ms` | **<80ms** | 80–100ms | **≥100ms** |
| 任务失败率 | `tasks_failed/tasks_executed` | **<0.1%** | 0.1–0.5% 且可解释 | **≥0.5%** |
| Miss Deadline | `scan_miss_deadline_total` | 稳态 **=0** | — | 稳态 **>0** |
| 恢复时间 | S2–S6 各次 | ≤上表场景上限 | 超上限 ≤2× 且仅 1 次 | 超上限 >2× 或重复失败 |
| CB Open | `driver_circuit_open_total` | 可解释、可自动恢复 | 频繁 Open 有 documented 根因 | Open 后无法 Half-Open/永久僵死 |
| Heap drift | 60s / 1h / 长跑 | **<8%** | 8–12% 且无 OOM | **≥12%** 或内存持续增长 |
| GC pause max | `gc_pause_max_ms` | **<30ms** | 30–50ms | **≥50ms** |
| Goroutine 趋势 | 采样 | 无持续增长 | 缓增有解释 | 泄漏或 soak 后翻倍 |

### 与工业验证测试报告的关系

| 门禁项 | 报告章节 |
| --- | --- |
| 协议 / 环境 / 配置 | §1–§2 |
| S1–S6 执行与单项结论 | §3 |
| 指标与阈值对照 | §4 |
| 阻塞项与偏差 | §5 |
| **本协议 G-Industrial 结论** | §6 |
| Release 纳入建议 | §7 |

**裁量规则：** §6 结论须与上表 **PASS/WARN/BLOCK** 一致；§6 为 **BLOCK** 时 §7 不得勾选「准予纳入 Release」。

### 版本发布判定矩阵

| G-Stability | G-Industrial | G-Performance | G-Lightweight | 发布决策 |
| --- | --- | --- | --- | --- |
| PASS | 全部支持协议 PASS | PASS | PASS | **准予发布** |
| PASS | 全部 PASS；部分 WARN 已签核 | PASS | PASS | **有条件发布**（注明 WARN 协议与限制） |
| PASS | 任一 BLOCK 或缺报告 | — | — | **不得发布** |
| BLOCK | — | — | — | **不得发布** |
| PASS | PASS | FAIL | PASS | **不得发布**（性能回归） |
| PASS | PASS | PASS | FAIL | **不得发布** |

**不得对外宣称「可用于工业现场」的情形：** G-Industrial BLOCK；必测场景 FAIL；仅 Mock/单测无联机报告；仿真与现场混填为「已通过现场验证」。

---

## G-Performance — 性能门禁

**目标：** 合并主干与发布前，性能不退化且满足统计 SLA。

| 检查项 | 验收标准 | 验证方式 |
| --- | --- | --- |
| 统一 Benchmark | Q3 10k Tag benchmark 通过 | `q3_10k_tag_benchmark_test.go` |
| Scan Lag | P95 在阈值内；稳态 miss deadline = 0 | diagnostics + gate |
| GC / Heap | GC pause、heap drift 在阈值内 | benchmark + soak |
| Goroutine | 压测与 soak 后数量稳定 | runtime 采样 |
| 吞吐量 | 批量读/调度吞吐无回归 | benchmark 对比上一版本 |
| 合并策略 | **Benchmark 未通过不得合并性能相关 PR** | CI gate |

| 指标 | x86 mock 阈值 | 板端/联机阈值 |
| --- | --- | --- |
| Scan Lag P95 | <100ms | <150ms |
| Scan Drift 均值 | <50ms | <80ms |
| GC pause max（60s） | <20ms | <30ms |
| Heap drift（60s） | <5% | <8% |

详细常量见 [SLA评估 — 核心指标](TODO/SLA评估.html#三sla-指标定义)。

---

## G-Lightweight — 轻量化门禁

**目标：** 交付形态与工业边缘网关「轻量、可嵌入、零依赖」定位一致。

| 检查项 | 验收标准 | 验证方式 |
| --- | --- | --- |
| 单二进制 | `CGO_ENABLED=0` 静态编译，单 `edgeCore` 可运行 | GoReleaser / 构建产物 |
| 零外部依赖 | 运行时不依赖 Redis、Prometheus、Grafana、外部 TSDB 等 | 部署清单审查 |
| 诊断通路 | HTTP API + Web UI + 结构化日志覆盖运维所需 | diagnostics 端点走查 |
| 无重量级栈 | 不引入与产品定位冲突的重型运行时 | 依赖与镜像体积审查 |

---

## 发布检查清单（模板）

版本：`________`　日期：`________`　负责人：`________`

### G-Stability

- [ ] 24–72h soak 通过，无 goroutine 泄漏
- [ ] 内存无异常增长
- [ ] 故障恢复 E2E 全绿
- [ ] 设备/通道隔离测试通过

### G-Industrial

- [ ] 各**已支持协议**均有归档报告（[工业验证测试报告模板](testing/工业验证测试报告模板.html)）
- [ ] S1–S6 必测场景全部 **通过**（或 WARN 已签核并注明限制）
- [ ] 联机指标达 PASS 阈值（Lag P95 <150ms、失败率 <0.1%、稳态 miss deadline = 0 等）
- [ ] 无 BLOCK 级问题（panic、死锁、不可恢复 CB、跨设备污染）
- [ ] 版本发布判定矩阵结论：☐ 准予发布　☐ 有条件发布　☐ **不得发布**

### G-Performance

- [ ] 10k Tag benchmark 通过
- [ ] Scan Lag / GC / Heap gate 通过
- [ ] 与上一版本无性能回归

### G-Lightweight

- [ ] 单二进制、零外部运行时依赖
- [ ] 诊断经 HTTP/UI/日志可用
- [ ] 无新增重量级监控依赖

**结论：** ☐ 准予发布　☐ 有条件发布（注明项）　☐ 不予发布

---

## 与 CI 的关系

> **现状审计（2026-07-03）：** PR / `main` 由 [CI Workflow](https://github.com/anviod/edgeCore/blob/main/.github/workflows/ci.yml) 运行 P0 门控（`test-short`、`test-soak-short`、构建 smoke、`bench-q3`）；tag 发布仍走 [Release Workflow](https://github.com/anviod/edgeCore/blob/main/.github/workflows/release.yml)。G-Industrial 仍未自动化。详见 **[CI 与发布门禁对照](testing/CI与发布门禁对照.html)**。

| 门禁 | 目标 CI 行为 | 当前状态 |
| --- | --- | --- |
| G-Stability | PR：`make test-soak-short`；Nightly：`make test-soak`（1h–72h） | **PR CI 已跑** short soak + `test-short`；长跑 **未自动化** |
| G-Industrial | Release 前强制联机报告归档；可选仿真器 job | **未自动化** — 人工 + [报告模板](testing/工业验证测试报告模板.html) |
| G-Performance | PR：`make bench-q3`；合并前 benchmark 全绿 | **PR CI 已跑** `bench-q3`；**无回归对比** |
| G-Lightweight | 构建 job：`CGO_ENABLED=0`；goreleaser 多架构 | Release 仅 linux/windows amd64；**goreleaser 未接入** |

**建议演进：** P0 已落地；见 [CI 与发布门禁对照 — 建议的 CI 演进](testing/CI与发布门禁对照.html#建议的-ci-演进p1-起本文档不实施)（P1 nightly / goreleaser → P2 工业验证 artifact → P3 基线对比）。
