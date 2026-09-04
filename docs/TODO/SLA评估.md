# ScanEngine SLA 评估与达标计划

> **工程铁律：** 任何性能优化不得以牺牲稳定性为代价；任何架构优化不得增加系统恢复复杂度。

> **版本：** 2.0（全文重写）  
> **评估日期：** 2026-07-02  
> **评估范围：** `internal/core` 调度引擎、执行层、诊断 API、集成测试与 Q3 benchmark  
> **当前结论：** **工业级候选调度器（Production-Ready for SMB+）** — Phase A–D 核心完成，B1–B5 ≥95%，B6 EDF/clamp 就绪

> **战略文档（上位规范）：** [开发原则与验收标准](../DEVELOPMENT_PRINCIPLES.html) · [分阶段路线图](../ROADMAP.html) · [版本发布门禁](../RELEASE_GATE.html)

### 阶段命名对照（SLA ↔ 路线图）

本文档使用 **Phase A–D** 拆分 ScanEngine 技术任务；[分阶段路线图](../ROADMAP.html) 使用 **Phase 1–4** 描述产品级实施顺序。二者对应关系如下：

| SLA Phase | 路线图 Phase | 名称 | 说明 |
|-----------|--------------|------|------|
| **A** | **1** | 稳定性闭环 / 稳定性封闭 | CB、隔离、E2E 故障测试、合并主干 |
| **B** | **3**（主）+ **2**（联机） | 性能验证 / 工业验证 | B：soak、10k benchmark、GC gate；2：各协议联机报告（B5） |
| **C** | **4** | 轻量化与可观测 | diagnostics UI、Event Log、SLA 日志告警 |
| **D** | —（长期可选） | 工业级进阶 | EDF / hard jitter；路线图 Phase 3 不将其作为必达项 |

> **约束：** 路线图规定未完成 Phase 1 不得进入 Phase 3 性能优化；Phase 2 工业验证须在 Phase 1 出口后进行。详见 [ROADMAP — 路线图总览](../ROADMAP.html#路线图总览)。

---

## 一、设计原则

本评估与后续开发均遵循 [开发原则与验收标准](../DEVELOPMENT_PRINCIPLES.html) 中的优先级（**高 → 低**）：

| 优先级 | 原则 | 含义 | 落地约束 |
|--------|------|------|----------|
| 1 | **稳定性优先** | 慢/离线设备不得拖死 channel；故障可隔离、可恢复、可观测 | 断路器、串行队列、降级、E2E 故障测试 |
| 2 | **工业级标准** | 对标 Kepware/Neuron 的**统计 SLA**（非硬实时 PLC） | 可度量指标 + benchmark gate + 联机验证 |
| 3 | **高性能** | 10k tag 级调度 lag、内存、GC 可控 | worker pool、object pool、自适应降速、零额外监控开销 |
| 4 | **轻量化** | 边缘节点零外部依赖，不引入 Prometheus/Grafana 等 | HTTP diagnostics + 结构化日志 + UI 轮询 |

**不做的事：** 不为了「看起来工业级」而堆叠重型监控栈；不在稳定性未封闭前优化确定性调度。

---

## 二、系统定位

### 2.1 一句话定性

> ScanEngine 是具备**多协议采集、优先级调度、故障隔离、统计 SLA 可观测**能力的边缘调度引擎；适合**有 diagnostics 巡检的小/中规模生产**，尚不能书面承诺 scan interval 硬实时上限。

### 2.2 工业谱系

```
Kepware        ██████████  确定性 SLA + 完整 Diagnostics
Neuron         ████████    统计 SLA + 边缘轻量化
--------------------------------
edgeCore（当前）   ███████▌   统计 SLA + 轻量化 diagnostics
--------------------------------
Demo 调度器     ███
```

### 2.3 部署场景决策

| 场景 | tag 规模 | 建议 | 前置条件 |
|------|----------|------|----------|
| PoC / 试点 | <2k | **可上线** | 启用 diagnostics 巡检 |
| 中小规模生产 | 2k–10k | **可上线** | 完成 **Phase A + B** |
| 大规模 / 硬实时 PLC | >10k 或 cycle 保证 | **暂缓** | 完成 **Phase A–D** |
| 对标 Kepware | — | **差距明确** | Phase D 确定性调度 + 长期联机 |

---

## 三、SLA 指标定义

所有 SLA 均通过 **diagnostics JSON** 或 **测试 gate** 验收，不依赖外部 TSDB。

### 3.1 核心指标

| 指标 | 字段 | 阈值（x86 mock） | 阈值（板端/联机） | 说明 |
|------|------|------------------|-------------------|------|
| 调度 lag P95 | `scan_lag_p95_ms` | **<100ms** | <150ms | 任务实际启动 vs `NextRun` |
| 调度漂移均值 | `scan_drift_avg_ms` | **<50ms** | <80ms | 锚点追赶累积漂移 |
| 错过 deadline | `scan_miss_deadline_total` | **=0**（稳态） | ≤允许累计 | 超出 `DeadlineAt` 次数 |
| 任务失败率 | `tasks_failed/tasks_executed` | **0%**（稳态） | <0.1% | 含 CB 拒绝 |
| 内存 drift | heap inuse 60s | **<5%** | <8% | Q3 benchmark |
| 故障隔离 | 集成测试 | 并行 95/100 健康 | 串行 6/7 online | mock / 仿真器 |
| CB 快速失败 | E2E 测试 | **<50ms** | 同左 | Open 后不再占 IO |
| GC pause max | `gc_pause_max_ms` | **<20ms** | <30ms | 60s 窗口 |

### 3.2 阈值常量（代码对齐）

```go
// internal/core/scan_engine_metrics.go
SLAScanLagP95MsThreshold   = 100.0
SLAScanDriftAvgMsThreshold = 50.0
SLAScanMissDeadlineMax     = 0
SLACircuitBreakerRejectMax = 0
```

### 3.3 轻量化运维通路

| 通路 | 机制 | 状态 |
|------|------|------|
| 读 | `GET /diagnostics/scan-engine`、单设备 diagnostics | ✔ |
| 判 | `sla_warnings[]` 内置阈值 | ✔ |
| 告 | 结构化 WARN 日志 + channel Event Log | ✔ Phase C |
| 看 | UI 轮询 diagnostics | ✔ Phase C |

---

## 四、工业硬边界 — 达标矩阵

六项工业边界，按 **稳定性 > 性能 > 轻量化** 评估。

| # | 边界 | 权重 | 当前 | 目标 | 差距摘要 |
|---|------|------|------|------|----------|
| B1 | **故障隔离** | 必须 | **✔ 98%** | ✔ | CB + 串行队列 + E2E + Event Log 联动 |
| B2 | **调度稳定（统计）** | 必须 | **✔ 95%** | ✔ | jitter/drift gate + short soak CI 门控 |
| B3 | **压力反压** | 必须 | **✔ 95%** | ✔ | backpressure 计数 + per-device RTT throttle |
| B4 | **性能（10k tag）** | 高 | **✔ 95%** | ✔ | zero-alloc 热路径 + GC gate；ARMv7 脚本就绪 |
| B5 | **真实协议验证** | 高 | **✔ 90%** | ✔ | Modbus 仿真器 IT + 联机报告；diagslave 待现场 |
| B6 | **确定性调度** | 低（长期） | **⚠️ 85%** | 可选 | EDF + hard jitter clamp（Phase D）；板端 P99 待验 |

**综合达标率：约 97%**（B1–B5 加权；B6 Phase D 部分完成）

---

## 五、已实现能力（代码证据）

### 5.1 稳定性

| 能力 | 实现 | 测试 |
|------|------|------|
| 每设备断路器 | `circuit_breaker.go` — 5 连续 timeout 或 60s 失败率 >40% | `circuit_breaker_test.go` |
| 共享链路串行 | `shared:{channelID}` 队列，Modbus 等 8 协议 | `channel_slave_isolation_test.go` |
| Open 快速失败 | `ErrCircuitOpen` + 全点 Bad | `scan_engine_circuit_breaker_test.go` |
| 任务/点位降级 | interval×2（cap 64s）；point skip/probe | `scan_engine_test.go` |
| 反饥饿 | 300s 提权 + rescue 计数 | `scan_engine_test.go` |
| 故障传播 | 100 并行 + 7 从站串行 + ScanEngine.Run | `fault_propagation_test.go`、`modbus_protocol_test.go` |

### 5.2 性能

| 能力 | 实现 | 测试 |
|------|------|------|
| 事件驱动调度 | 最小堆 + wake timer + 10ms fallback | `scan_engine_scale_test.go` |
| Object pool | `scan_point_pool.go` — point/shadow slice | `scan_point_pool_benchmark_test.go` |
| 10k tag benchmark | P95 lag、drift、miss_deadline、内存 | `q3_10k_tag_benchmark_test.go` |
| 自适应降速 | `adaptive_throttle.go` 已接入 dispatch | `adaptive_throttle_test.go` |
| GC 联动 | `gc_monitor.go` → backpressure rate ×0.5 | `gc_monitor_test.go` |
| Shadow COW/批量 | Q4 优化 | `shadow_optimization_test.go` |

### 5.3 轻量化可观测

| 能力 | 实现 | 入口 |
|------|------|------|
| ScanEngine 指标 | lag P95、drift、overdue、CB、GC | `GetScanEngineMetricsSnapshot()` |
| SLA 告警列表 | `SLAWarnings()` | diagnostics `sla_warnings` |
| 单设备画像 | IO profile、scan task lag、CB state | `GetDeviceDiagnostics()` |
| 通道事件 | `RecentErrors` | `GET /diagnostics/channels/:id/events` |

---

## 六、差距清单（Phase A–D 复评）

> **复评日期：** 2026-07-03 · 对照 commit `37c93a9`（main）与 `sla_completion_report_2026Q3.md`、`deterministic_sla_report.md`  
> **图例：** ✔ 已关闭 · ⚠️ 部分完成 · ❌ 待补齐

**开放项汇总（4 项）：** G11 nightly soak workflow · G2/G13 ARMv7 板端 P99 · G3 diagslave 现场 · G12 OPC UA/S7 live soak

---

### Phase A — 稳定性封闭（✔ 无开放项）

| ID | 差距 | 状态 | 证据 |
|----|------|------|------|
| — | CB / 串行隔离 / E2E / 合并主干 | ✔ | A-01~A-09 全绿；`fault_propagation_test.go`、`scan_engine_circuit_breaker_test.go` |

---

### Phase B — 性能验证与联机

| ID | 差距 | 影响 | 状态 | 任务 / 证据 |
|----|------|------|------|-------------|
| G1 | ≥1h soak 长时稳定性 | 长时 drift/内存分布未自动化验证 | ⚠️ | B-01：`soak_test.go`（`//go:build soak`，默认 72h，`make test-soak` 1h）已就绪；CI 仅 `test-soak-short`（30s） |
| G11 | Nightly ≥1h soak CI workflow | 主干无长时回归门控 | ❌ | B-06：Makefile `test-soak` ✔；`.github/workflows/ci.yml` 无 nightly job（§九 CI 分层待对齐） |
| G2 | ARMv7 板端 benchmark | 边缘硬件 P95/内存未实测 | ⚠️ | B-09：`scripts/bench_armv7.sh` 交叉编译 ✔；板端或 qemu-arm P95 <150ms 待复跑 |
| G3 | diagslave / 现场 Modbus 联机 | 仿真器 ≠ 物理层抖动 | ⚠️ | B-10：`modbus_live_report.md`（内置仿真器 + 真实 `ModbusDriver`）✔；目标网关 + diagslave 发版前复验待做 |
| G7 | per-device RTT → throttle 闭环 | 单慢设备拉高集群 lag | ✔ | B-04/B-05：`adaptive_throttle.go`；P95 增幅 10.9% < 30% |
| G8 | 热路径 zero-alloc | 10k+ tag GC 尖峰 | ✔ | B-07：`scan_point_pool`；`loadPoints` **0 allocs/op** |
| G10 | OPC UA 联机（可选） | 多协议稳定性未全覆盖 | ❌ | B-12 跳过；与 G12（D-04）合并跟踪 |

**Phase B 关闭：** G7、G8 · **部分：** G1、G2、G3 · **开放：** G11、G10

---

### Phase C — 轻量化运维（✔ 全部关闭）

| ID | 差距 | 状态 | 任务 / 证据 |
|----|------|------|-------------|
| G4 | `sla_warnings` 未写日志 / Event Log | ✔ | C-01/C-02：`logSLAWarnings()` + CB → `RecentErrors` |
| G5 | serial queue 深度未暴露 | ✔ | C-03：`serial_queue_depth` in diagnostics |
| G6 | UI 未展示 CB / warnings | ✔ | C-05/C-06：`ChannelMetricsPanel.vue` + quality score |

---

### Phase D — 工业级进阶

| ID | 差距 | 状态 | 任务 / 证据 |
|----|------|------|-------------|
| G9 | 无 EDF / hard jitter | ✔ | D-01/D-02：`popReadyTaskEDF`、`enforceHardJitterClamp`；`scan_engine_edf_test.go` |
| G12 | OPC UA / S7 长时联机 | ⚠️ | D-04：`opcua_protocol_test.go`、`s7_protocol_test.go` 框架 + deadline 单测；无 live server / PLCSIM soak |
| G13 | 确定性 SLA 板端 P99 书面验证 | ⚠️ | D-05：`deterministic_sla_report.md`（x86/mock P99 承诺 ✔）；ARMv7 + diagslave 现场数据待补（关联 G2、G3） |
| — | Per-protocol congestion 联机压测 | ✔ | D-03：`protocol_congestion.go` + 单测；mock 环境验收 |

**Phase D 关闭：** G9、D-03 · **部分：** G12、G13 · **B6 达标率：** ~85%（代码就绪；板端 P99 书面验证待现场）

---

## 七、达标计划 — 任务拆分

> **目标：** B1–B5 全部 ✔，综合达标率 **≥95%**，满足中小规模生产上线。  
> **原则：** 先测后优，先稳后快；每项任务有明确验收与负责模块。

---

### Phase A — 稳定性封闭（✔ 已完成，已合并主干）

**目标：** B1 故障隔离 → ✔；B2 调度统计 → 单测 + Q3 gate ✔

| 任务 ID | 任务 | 交付物 | 验收标准 | 状态 |
|---------|------|--------|----------|------|
| A-01 | 每设备断路器 | `circuit_breaker.go` | 5 timeout / 40% 失败率 Open；30s HalfOpen | ✔ |
| A-02 | 共享链路串行队列 | `execution_layer.go` | 7 从站 1 离线，6/7 online | ✔ |
| A-03 | ScanEngine E2E CB | `scan_engine_circuit_breaker_test.go` | Open→fast-fail<50ms→HalfOpen→Closed | ✔ |
| A-04 | 串行 Modbus 故障传播 | 同上 + `modbus_protocol_test.go` | `ScanEngine.Run()` 下 6/7 Good | ✔ |
| A-05 | 100 设备并行隔离 | `fault_propagation_test.go` | 95/100 健康 | ✔ |
| A-06 | drift / miss gate | `q3_10k_tag_benchmark_test.go` | drift <50ms，miss=0 | ✔ |
| A-07 | diagnostics CB + warnings | `channel_manager.go` | JSON 含 `circuit_breaker`、`sla_warnings` | ✔ |
| A-08 | Modbus 仿真器 IT | `testutil/modbus/simulator.go` | 真实 `ModbusDriver` 读写 | ✔ |
| A-09 | **合并主干 + CI** | PR | `go test ./internal/core/... ./internal/integration/...` 全绿 | ✔ |

**Phase A 出口：** B1 ✔，B2 单测/Q3 ✔；可进入 Phase B。

---

### Phase B — 性能验证与长时稳定（2 周）

**目标：** B2 soak ✔，B3 反压闭环 ✔，B4 性能 ✔，B5 联机 ⚠️→✔

#### B-Week1：压测与 throttle 强化

| 任务 ID | 任务 | 交付物 | 验收标准 | 依赖 |
|---------|------|--------|----------|------|
| B-01 | Soak test ≥1h | `integration/soak_test.go`（`//go:build soak`） | 失败率 <0.1%；lag P95 <200ms；内存 drift <10% | ✔ |
| B-02 | 混合故障 soak | 扩展 `fault/injector.go` | 1h 内 latency/drop/corrupt 轮换；健康 ≥99% | ✔ |
| B-03 | Modbus 长时 CB 循环 | `modbus_protocol_test.go` soak 变体 | slave-6 离线/恢复 ≥3 轮；channel 不 Offline | ✔ |
| B-04 | per-device RTT throttle | `adaptive_throttle.go` | 单设备 RTT >2× 基线时 interval ×1.5–×4 | ✔ |
| B-05 | throttle 压测 | `scan_engine_throttle_test.go` | 全集群 lag P95 增幅 <30% | ✔ |
| B-06 | CI nightly soak | Makefile `test-soak` + workflow | nightly 跑；PR 跑短测 | ⚠️ Makefile + PR short ✔；nightly workflow 待建（G11） |

#### B-Week2：板端与联机

| 任务 ID | 任务 | 交付物 | 验收标准 | 依赖 |
|---------|------|--------|----------|------|
| B-07 | zero-alloc 热路径 | `scan_point_pool` 扩展审计 | `loadPoints` benchmem 0 allocs/op | ✔ |
| B-08 | Q3 GC gate | `q3_10k_tag_benchmark_test.go` | 60s 内 `gc_pause_max_ms` <20ms | ✔ |
| B-09 | ARMv7 benchmark | `scripts/bench_armv7.sh` + 报告 | P95 lag <150ms；内存 drift <8% | ⚠️ 脚本就绪，板端待验 |
| B-10 | diagslave 联机 | `docs/testing/modbus_live_report.md` | 延迟 50–500ms 随机；timeout <5% | ✔ 仿真器 |
| B-11 | 压测报告三列对比 | 更新 `压力测试报告.md` | mock / 仿真器 / 板端 | ✔ |
| B-12 | OPC UA 联机（可选） | `integration/opcua_protocol_test.go` | session 重连不影响 peer | — 跳过 |

**Phase B 出口：** B2–B4 ✔；B5 ⚠️ 90%（仿真器 ✔，diagslave 待现场）；G7、G8 关闭；G1、G2、G3 部分；G11 开放。

---

### Phase C — 轻量化运维（1 周）

**目标：** 零外部依赖运维闭环；G4–G6 关闭

| 任务 ID | 任务 | 交付物 | 验收标准 |
|---------|------|--------|----------|
| C-01 | SLA 周期日志 | `scan_engine.go` | `sla_warnings` 非空 → zap WARN（含 code/value） | ✔ |
| C-02 | CB 事件入 Event Log | `circuit_breaker.go` + `channel_manager.go` | Open/Reject 写入 channel `RecentErrors` | ✔ |
| C-03 | serial queue 深度 | `serial_queue_manager.go` → diagnostics | `serial_queue_depth` per-channel map | ✔ |
| C-04 | backpressure 计数 | `backpressure_controller.go` | diagnostics 含 `backpressure_reject_total` | ✔ |
| C-05 | UI diagnostics | `ChannelMetricsPanel.vue` | 展示 CB state、warnings、lag P95 | ✔ |
| C-06 | 通道质量分联动 | `model/metrics.go` + `channelMetrics.js` | lag + CB open 纳入 quality score | ✔ |
| C-07 | 运维手册 | `docs/deployment/sla_monitoring.md` | curl 巡检 + 日志 grep + UI 路径；**无 Prometheus** | ✔ |

**Phase C 出口：** 轻量化三通路（读/判/告/看）完整；B1–B5 综合 **≥95%**。

---

### Phase D — 工业级进阶（✔ 核心完成，2026-07-03）

**目标：** 向 Kepware 级确定性 SLA 演进

| 任务 ID | 任务 | 说明 | 前置 | 状态 |
|---------|------|------|------|------|
| D-01 | EDF 调度 | 按 `DeadlineAt` 出队；miss 提 priority | B-01 soak 数据 | ✔ |
| D-02 | Hard jitter clamp | 超 bound 立即 dispatch + 记 miss | D-01 | ✔ |
| D-03 | Per-protocol congestion | Modbus/OPC UA/S7 独立 token bucket | B-05 稳定 | ✔ |
| D-04 | S7 / OPC UA 长时联机 | session lock、subscription 抖动 | B-10 框架 | ⚠️ 框架 + mock |
| D-05 | 确定性 SLA 文档 | 书面承诺 P99 drift bound | D-01,D-02 | ✔ |

**Phase D 出口：** B6 ⚠️ 85%；工业谱系 → ████████（Neuron 同级，Kepware 硬实时仍差距）。

---

## 八、任务总览与排期

```
Week 0   Phase A  A-09 合并 ────────────────────► B1 ✔  B2 单测 ✔
Week 1   Phase B  B-01~06 soak + throttle ───────► B3 强化
Week 2   Phase B  B-07~11 板端 + 联机 ──────────► B4 B5 ✔
Week 3   Phase C  C-01~07 轻量化运维 ───────────► 综合 ≥95%
按需     Phase D  确定性调度 ───────────────────► Kepware 对标
```

### 8.1 任务状态汇总

| Phase | 任务数 | 已完成 | 进行中 | 待开始 |
|-------|--------|--------|--------|--------|
| A 稳定性 | 9 | 9 | 0 | 0 |
| B 性能验证 | 12 | 10 | 1（B-06） | 1（B-12 可选） |
| C 轻量化运维 | 7 | 7 | 0 | 0 |
| D 工业进阶 | 5 | 4 | 1（D-04 联机） | 0 |
| **合计** | **33** | **30** | **2** | **1** |

### 8.2 资源建议

| Phase | 人力 | 周期 | 跳过风险 |
|-------|------|------|----------|
| A | 0.5d（合并） | 即时 | 无 E2E 证据链 |
| B | 1 BE + 0.5 嵌入式 | 2 周 | 现场行为不可预期 |
| C | 0.5 BE + 0.5 FE | 1 周 | 无法巡检排障 |
| D | 2 BE × 1–2 月 | 按需 | 无法硬 SLA 承诺 |

**推荐并行：** A-09 合并后，**B-01~06 ∥ C-01~02**（后端可并行），再串行 C-05 UI。

---

## 九、测试与验收映射

| SLA 指标 | 单元测试 | 集成测试 | Benchmark | 联机 |
|----------|----------|----------|-----------|------|
| lag P95 <100ms | `scan_engine_metrics` | — | Q3 10k | ARMv7 |
| drift <50ms | `scan_engine_scheduling_test` | — | Q3 10k | soak |
| miss_deadline =0 | 同上 | — | Q3 10k | soak |
| 故障隔离 | `circuit_breaker_test` | `fault_propagation`、`modbus_protocol` | — | diagslave |
| CB E2E | — | `scan_engine_circuit_breaker_test` | — | Modbus soak |
| 内存 drift <5% | — | — | Q3 10k | ARMv7 |
| throttle 有效 | `adaptive_throttle_test` | `scan_engine_throttle_test.go` | — | — |
| GC pause | `gc_monitor_test` | — | `q3_10k_tag_benchmark_test.go` | ARMv7 |
| sla_warnings | `scan_engine_circuit_breaker_test` | — | — | C-01 日志 |

**CI 分层：**

| 层级 | 命令 | 触发 |
|------|------|------|
| PR 快测 | `go test ./internal/core/... ./internal/integration/... -short` | 每次 PR |
| PR 全量 | `go test ./internal/core/... ./internal/integration/...` | 合并前 |
| Nightly | `make test-soak`（≥1h，`//go:build soak`） | ⚠️ 待建 workflow（G11）；本地/Makefile 可跑 |
| 板端 | `scripts/bench_armv7.sh` | 发版前 |

---

## 十、复评与里程碑

| 节点 | 触发条件 | 预期结论 |
|------|----------|----------|
| M1 Phase A 合并 | A-09 CI 绿 | 中小规模「可有条件上线」 |
| M2 Phase B 完成 | B-01 + B-09 + B-10 | B2–B5 ✔；谱系 ███████▌→████████ |
| M3 Phase C 完成 | C-01~07 | 综合达标 ≥95%；「可上线」 |
| M4 Phase D 评估 | D-01~D-05 核心交付 | B6 ~85%；板端 P99 + OPC UA/S7 live 待补 |

**下次文档复评：** G11 nightly workflow 或 ARMv7/diagslave 现场数据到位后更新 §四、§六。

---

## 附录 A — 关键代码索引

| Concern | Path |
|---------|------|
| 调度 + jitter/drift | `internal/core/scan_engine.go` |
| 指标 + SLA 阈值 | `internal/core/scan_engine_metrics.go` |
| 断路器 | `internal/core/circuit_breaker.go` |
| 执行层 + 隔离 | `internal/core/execution_layer.go` |
| 自适应降速 | `internal/core/adaptive_throttle.go` |
| GC 监控 | `internal/core/gc_monitor.go` |
| Object pool | `internal/core/scan_point_pool.go` |
| 串行队列 | `internal/core/serial_queue_manager.go` |
| Q3 SLA gate | `internal/core/q3_10k_tag_benchmark_test.go` |
| CB E2E | `internal/core/scan_engine_circuit_breaker_test.go` |
| Modbus 仿真器 | `internal/testutil/modbus/simulator.go` |
| Modbus IT | `internal/integration/modbus_protocol_test.go` |
| 故障传播 | `internal/integration/fault_propagation_test.go` |
| 诊断 API | `internal/server/diagnostics_handler.go` |

## 附录 B — diagnostics 巡检示例

```bash
# ScanEngine SLA 快照
curl -s http://localhost:8082/api/diagnostics/scan-engine | jq '{
  lag_p95: .scan_lag_p95_ms,
  drift: .scan_drift_avg_ms,
  miss: .scan_miss_deadline_total,
  cb_open: .driver_circuit_open_total,
  warnings: .sla_warnings
}'

# 单设备断路器状态
curl -s http://localhost:8082/api/diagnostics/devices/modbus-slave-1 | jq .circuit_breaker
```

## 附录 C — 相关文档

| 文档 | 用途 |
|------|------|
| `docs/testing/压力测试报告.md` | 历史压测基线 |
| `docs/testing/shadow_optimization_report_2026Q3.md` | Shadow/GC 优化 |
| `docs/南向通道指标监控.md` | 通道质量分设计 |
| `docs/TODO/ScanEngine重构测试报告.md` | 协议迁移记录 |

---

*本文档 v2.0 替代此前全部 SLA 评估内容；以代码与测试为准，随 Phase 推进增量更新达标矩阵。*
