# ARMv7 工业控制内核 — Pipeline Worker Go 参考实现

> **文档定位**：V2.2 Pipeline Worker 的 Go 参考设计——面向 ARMv7 / RK3588 嵌入式工业控制盒，与 [边缘计算优化升级 2.0](./边缘计算优化升级2.0.md) §0–§15 对齐。配置走 **bbolt + 内存**；**禁止 YAML 入代码**。

| 项 | 内容 |
|----|------|
| 版本 | V1.0 |
| 更新 | 2026-07-05 |
| 状态 | 参考设计（待 P0 实现） |
| 架构主文档 | [边缘计算优化升级 2.0](./边缘计算优化升级2.0.md) |
| 代码基线 | `internal/core/scan_engine.go` · `pipeline.go` · `edge_compute_manager.go` |

---

## 1. 设计目标

| 目标 | 指标 |
|------|------|
| 控制延迟 P95 | ≤ 20ms（不含 Modbus/MQTT/串口 IO） |
| 72h soak | 内存漂移 ≤ 5%；GC STW ≤ 5ms |
| 热路径分配 | **零堆分配**（`go test -benchmem -memprofile`） |
| 内存预算 | 256MB–2GB 设备可运行 |
| 稳定性原则 | 无对象流积压、无双缓冲四层驻留、Feedback 不回写 Intent |

---

## 2. 总体架构

```text
                    ┌──────────────────────────────────────┐
                    │           config.db (bbolt)           │
                    │  pipeline_rules · point_bindings      │
                    └─────────────────┬────────────────────┘
                                      │ init 编译（冷路径）
                                      ▼
ScanEngine ──► RingBuffer ──► Worker Loop ──► Execute ──► Drivers
                  │              │    │    │
                  │              │    │    └── Feedback ──► runtime.db (async)
                  │              │    └── Decide (RuleTable 只读)
                  │              └── Evaluate (EvalTable 只读)
                  └── PointCache (atomic 覆盖写)
```

**单 goroutine 原则**：Pipeline Worker 独占热路径；ScanEngine 采集 goroutine 仅 `Push` ring buffer；bblot 写入独立 goroutine。

---

## 3. Pipeline Worker 主循环

```go
// internal/pipeline/worker.go

type Worker struct {
    input    *RingBuffer
    cache    *pointcache.Cache
    eval     *EvalTable
    rules    *RuleTable
    exec     *Executor
    feedback *Feedback
}

func (w *Worker) Run(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
        }
        ev, ok := w.input.Pop()
        if !ok {
            runtime.Gosched()
            continue
        }
        if !w.allow(ev) {
            w.feedback.CountRejected()
            continue
        }
        val, ok := w.eval.Eval(ev, w.cache)
        if !ok {
            continue
        }
        slot, ok := w.rules.Match(ev.SourceIdx, val)
        if !ok {
            continue
        }
        res := w.exec.Run(slot.ActionIdx, val)
        w.feedback.Record(ev, slot, res)
    }
}
```

**与 1.x ECM 对比**：

| 1.x `edge_compute_manager.go` | Pipeline Worker |
|------------------------------|-----------------|
| `workerPool chan *ruleTask` | 单 loop，无 channel 任务 |
| `ruleIndex map[string][]string` | init 编译 `RuleTable` |
| `valueCache map[string]Value` | `PointCache []atomic` |
| `expr` runtime | init 预展开 |
| `executeRule` 混合决策+执行 | decide / execute 硬分离 |

---

## 4. Ring Buffer 输入流

```go
// internal/pipeline/ring_buffer.go

const defaultRingCap = 4096 // 固定，init 分配

type RingEvent struct {
    SourceIdx uint32
    Raw       int32
    Ts        int64
    Quality   uint8
}

type RingBuffer struct {
    buf  []RingEvent
    mask uint32
    head atomic.Uint32
    tail atomic.Uint32
    drop atomic.Uint64 // drop-oldest 计数
}

func (r *RingBuffer) Push(ev RingEvent) {
    h := r.head.Load()
    t := r.tail.Load()
    next := (h + 1) & r.mask
    if next == t {
        r.tail.Store((t + 1) & r.mask) // drop-oldest
        r.drop.Add(1)
    }
    r.buf[h&r.mask] = ev
    r.head.Store(next)
}
```

**对接 ScanEngine**：

- 在 `DataPipeline` 与 ECM handler **之外**注册 `ScanEngine` 回调（或 Shadow 写入后 hook）。
- 点位字符串 → `SourceIdx`：**init 时**建立 `point_bindings` 索引表，热路径只做 uint32 比较。
- 满则 drop-oldest，**永不阻塞** ScanEngine（对齐 `armv7_bench_test.go` 采集 lag 门禁）。

---

## 5. Lock-free PointCache

```go
// internal/pointcache/cache.go

type slot struct {
    value atomic.Int32
    ts    atomic.Int64
}

type Cache struct {
    slots []slot // len = maxPoints，init 固定
}

func (c *Cache) Store(idx uint32, value int32, ts int64) {
    if int(idx) >= len(c.slots) {
        return
    }
    c.slots[idx].value.Store(value)
    c.slots[idx].ts.Store(ts)
}

func (c *Cache) Load(idx uint32) (int32, int64) {
    if int(idx) >= len(c.slots) {
        return 0, 0
    }
    return c.slots[idx].value.Load(), c.slots[idx].ts.Load()
}
```

**设计约束**：

- **禁止 map**：点位 ID 仅出现在 init / API 层。
- **覆盖写语义**：无历史窗口队列；窗口逻辑在 `EvalTable` 用环形样本区（固定长度 `[]int32`）实现。
- 与 1.x `virtual_shadow_engine.go` 公式：init 解析点位引用 → `EvalSlot` 绑定点索引数组。

---

## 6. 预展开规则表（bbolt → RuleTable）

### 6.1 bbolt 配置结构（JSON 序列化）

Bucket：`pipeline_rules`

```json
{
  "id": "rule-high-temp",
  "source_ref": "ch1.dev1.temp",
  "op": "gt",
  "threshold": 8000,
  "action_id": "mqtt-alarm",
  "min_interval_ms": 5000
}
```

Bucket：`point_bindings`

```json
{
  "ref": "ch1.dev1.temp",
  "idx": 42
}
```

### 6.2 编译后运行时结构

```go
// internal/pipeline/decide.go

type RuleSlot struct {
    SourceIdx   uint32
    Op          uint8  // opGT, opLT, opEq, opEdgeRise, opEdgeFall
    Threshold   int32
    ActionIdx   uint16
    MinInterval int64  // ns
    LastFire    int64  // ns，inline 节流
}

type RuleTable struct {
    slots []RuleSlot
}

func (t *RuleTable) Match(sourceIdx uint32, value int32, now int64) (RuleSlot, bool) {
    for i := range t.slots {
        s := &t.slots[i]
        if s.SourceIdx != sourceIdx {
            continue
        }
        if s.LastFire != 0 && now-s.LastFire < s.MinInterval {
            continue
        }
        if !evalOp(s.Op, value, s.Threshold) {
            continue
        }
        s.LastFire = now
        return *s, true
    }
    return RuleSlot{}, false
}
```

**迁移自 EdgeRule 子集**：

| EdgeRule 字段 | RuleSlot |
|---------------|----------|
| Condition / Threshold | Op + Threshold |
| CheckInterval | MinInterval |
| Actions[0] | ActionIdx |
| Source 点位 | SourceIdx（经 point_bindings） |

**不支持 runtime 编译**：配置变更 → API → bbolt → **Worker 暂停 → reload 编译 → 恢复**（冷路径，允许 GC）。

### 6.3 用户规则配置 → RuleTable 预展开

> **对齐文档**：[边缘计算优化升级 2.0](./边缘计算优化升级2.0.md) §8 · [边缘计算 Pipeline 配置指南](../edge/边缘计算Pipeline配置指南.md)

UI / `EdgeRules` bucket 中的用户配置在 **init 编译器**中一次性展开为 `RuleTable` + `EvalTable` + `ActionSlot[]`。**ARM 热路径零 expr**。

#### 6.3.1 编译流水线

```text
bbolt EdgeRules (用户 JSON)
    │
    ├─► point_bindings 编译器 ──► alias "t1" → SourceIdx 42
    │
    ├─► 表达式编译器 ──► EvalSlot (formula / window / debounce)
    │
    ├─► 条件编译器 ──► RuleSlot (op + threshold + flags)
    │
    └─► 动作编译器 ──► ActionSlot (control / stream / store)
              │
              ▼
        Worker.Run() 只读表
```

#### 6.3.2 Threshold：`t1 > 80` → RuleSlot

**用户 Condition**（`EdgeRule.condition`）：

```text
t1 > 80
```

**编译步骤**：

1. 解析 `sources[]`：`alias=t1` → `point_bindings` → `SourceIdx=42`
2. 解析比较式：`gt`，右值 `80` → 定点 `threshold=80000`（scale=1000）
3. 映射 `trigger_mode=on_change` → `RuleSlot.EdgeTrigger=true`
4. 映射 `check_interval=5s` → `MinInterval=5_000_000_000` ns
5. 映射 `actions[0].type=mqtt` → `ActionIdx=3`（`ActionStream`）
6. 按 `priority` 插入 `RuleTable.slots`（降序）

```go
// init 编译输出示例
RuleSlot{
    SourceIdx:   42,
    Op:          opGT,
    Threshold:   80000,
    ActionIdx:   3,
    MinInterval: 5_000_000_000,
    EdgeTrigger: true,
    Priority:    10,
}
```

**runtime `Match()`**（无字符串、无 expr）：

```go
func evalOp(op uint8, value, threshold int32) bool {
    switch op {
    case opGT:
        return value > threshold
    // ...
    }
}
```

#### 6.3.3 表达式语法 → EvalSlot（非 runtime expr）

| 用户写法 | 编译器输出 | 热路径 |
|---------|-----------|--------|
| `t1*1.8+32` | `EvalFormula{InIdx:[42], Coef:[1000,1800,32000]}` | 定点乘加 3 条指令 |
| `bitget(v,3)==1` | `EvalBitGet{n:3}` → `RuleSlot{opEq, threshold:1}` | AND + SHIFT + 比较 |
| `window avg 10` | `EvalWindowAvg{WindowN:10, buf:[10]int32}` | 环形累加 / 除法 |

**位运算 opcode**（init 展开，非函数表）：

```go
const (
    EvalBitGet EvalKind = iota + 10
    EvalBitSet
    EvalBitClr
    EvalBitAnd
    EvalBitOr
)
```

#### 6.3.4 动作链 → StepTable 预展开

**embedded profile**（PRIMARY）：单规则通常 1 个 `ActionIdx`；多动作在 init 展开为 **内联 Step 数组**（仍属 `control` / `stream` / `store` 三类）。

```go
// gateway profile 完整 StepChain
type StepSlot struct {
    Kind      ActionKind // control / stream / store / delay / check
    TargetIdx uint32
    DelayMs   uint32     // delay step
    CheckOp   uint8      // check step: 读点比较
    OnFailIdx uint16     // check 失败跳转
}

type StepChain struct {
    steps []StepSlot // init 预分配，runtime 顺序执行
}
```

| UI 动作链 | embedded | gateway |
|----------|----------|---------|
| `[mqtt]` | `ActionSlot[3]` 单步 | 同左 |
| `[control, delay, check, control]` | 简化为 `[control]` 或报错 | `StepChain` 4 步 |
| `[sequence → ...]` | 拆多条规则 | 完整 `StepChain` |

#### 6.3.5 TriggerFlags 与 allow() 门控

```go
type RuleSlot struct {
    // ...
    EdgeTrigger bool  // trigger_mode: on_change
    MinInterval int64 // check_interval
    LastFire    int64 // runtime 节流状态（唯一可写热路径字段）
}

func (w *Worker) allow(ev RingEvent) bool {
    // 1. 源级 check_interval：PointCache 上次处理 ts
    if w.cache.SinceLastProcess(ev.SourceIdx, ev.Ts) < w.sourceInterval[ev.SourceIdx] {
        w.feedback.CountRateLimit()
        return false
    }
    // 2. on_change 边沿：查 Feedback.lastState，未变化则 dedup
    // （在 match 前或 match 内，依 EdgeTrigger 标志）
    return true
}
```

| 用户字段 | RuleSlot / Worker | 行为 |
|---------|-------------------|------|
| `trigger_mode: always` | `EdgeTrigger=false` | 条件持续为真且过 `allow()` 则重复触发 |
| `trigger_mode: on_change` | `EdgeTrigger=true` | 仅 false→true 边沿触发一次 |
| `check_interval: 5s` | `MinInterval` + `sourceInterval` | `allow()` + `LastFire` 双重节流 |

#### 6.3.6 bbolt bucket 布局（V2.2）

| Bucket | 内容 | 说明 |
|--------|------|------|
| `EdgeRules` | 用户 UI JSON（1.x 兼容） | API 读写；init 编译器输入 |
| `pipeline_rules` | 编译后 RuleSlot JSON | 可选：直接存储或 init 自 EdgeRules 生成 |
| `point_bindings` | ref → idx + alias 映射 | init 固定，热路径 uint32 |
| `pipeline_actions` | ActionSlot / StepChain | stream topic 模板等于 init 编译 |
| `pipeline_stats` | counter 快照 | 诊断 API |

---

## 7. Evaluate 内联表

```go
// internal/pipeline/eval.go

type EvalKind uint8

const (
    EvalPassthrough EvalKind = iota
    EvalFormula
    EvalWindowAvg
    EvalEdge
)

type EvalSlot struct {
    Kind     EvalKind
    InIdx    []uint32 // 公式输入点索引
    Coef     []int32  // 定点系数（scale=1000）
    WindowN  uint16
    buf      []int32  // 窗口环形区，init 分配
    bufPos   uint16
}

type EvalTable struct {
    slots []EvalSlot
}

func (t *EvalTable) Eval(ev RingEvent, cache *pointcache.Cache) (int32, bool) {
    if int(ev.SourceIdx) >= len(t.slots) {
        return ev.Raw, true
    }
    return t.slots[ev.SourceIdx].eval(ev, cache)
}
```

**定点数**：嵌入式避免 `float64`；温度等物理量 ×1000 存 `int32`（与 Modbus 寄存器对齐）。

---

## 8. Execute 三类型

```go
// internal/pipeline/execute.go

type ActionKind uint8

const (
    ActionControl ActionKind = iota
    ActionStream
    ActionStore
)

type ActionSlot struct {
    Kind       ActionKind
    TargetIdx  uint32 // control: 写点索引；stream: connector 索引
    Payload    [64]byte
    PayloadLen uint8
}

type Executor struct {
    actions []ActionSlot
    cm      DeviceWriter // 复用 ChannelManager WritePoint
    nb      StreamWriter // 复用 NorthboundManager
    store   StoreWriter  // bblot / 文件
}

func (e *Executor) Run(actionIdx uint16, value int32) ResultCode {
    // switch actions[actionIdx].Kind → control/stream/store
    // 热路径：无 fmt.Sprintf、无 json.Marshal
}
```

### 8.1 PLC IO 调度模型

| 原则 | 实现 |
|------|------|
| 写串行 | 同一 channel 的 control 在 Executor 内 mutex（非全局） |
| 超时 | 每次 WritePoint context.WithTimeout（冷路径创建 timer 池） |
| 失败 | 返回 ResultCode，**不阻塞** Worker；counter + 可选 store |
| 读确认 | control 链内联：Write → 短 delay（busy wait ms 级）→ Read 同点 |

**与 ScanEngine 关系**：control 写走 `ChannelManager.WritePoint`；**不在驱动内**另起 goroutine 做规则逻辑。

### 8.2 MQTT 零拷贝 Pipeline

| 层级 | 策略 |
|------|------|
| Payload 模板 | init 编译固定 topic + 前缀字节 |
| 数值编码 | `strconv.AppendInt` 到 **预分配** `[]byte` scratch（Worker 栈或 Executor 字段） |
| 发布 | 复用 `NorthboundManager` 连接；避免每事件 `new([]byte)` |
| QoS | 默认 0（告警 stream）；可配置 1 在 ActionSlot |

---

## 9. Feedback 轻量层

```go
// internal/pipeline/feedback.go

type Feedback struct {
    lastState []atomic.Int32 // 每 source_idx 末态
    counters  struct {
        in, allowed, matched, execOK, execFail, dedup, rateLimit atomic.Uint64
    }
    recorder *edgeEventRecorder // 复用 internal/core
}

func (f *Feedback) Record(ev RingEvent, slot RuleSlot, res ResultCode) {
    f.lastState[ev.SourceIdx].Store(int32(res))
    // 非阻塞 enqueue 到 recorder（满则丢弃，不阻塞 Worker）
}
```

**禁止**：

- Feedback → 新 Pipeline 迭代
- Feedback → Intent / ExecutionContext
- 同步 bbolt 写

---

## 10. 零分配热路径指南

| 检查项 | 要求 |
|--------|------|
| 主循环 | 无 `make` / `new` / `append` 触达堆 |
| 字符串 | 热路径无 string 拼接；用 uint32 索引 |
| 接口 | 避免热路径 `interface{}` 装箱；Action 用 kind switch |
| JSON | 仅 init / API；bblot 批量写可分配 |
| 闭包 | Worker 不注册 per-event 闭包 handler |
| 日志 | 热路径无 `log.Printf`；counter 代替 |
| 验证 | `go test -benchmem -memprofile=mem.out`；`go tool pprof -alloc_space` |

**允许分配（冷路径）**：

- 配置 reload
- bblot 批量 flush
- REST API 诊断

---

## 11. 内存预算表

假设：1024 点位 · 256 条规则 · ring 4096

| 组件 | 估算 | 说明 |
|------|------|------|
| PointCache | 1024 × 16B ≈ 16KB | value + ts atomic |
| RingBuffer | 4096 × 24B ≈ 96KB | 固定 |
| RuleTable | 256 × 32B ≈ 8KB | 固定 |
| EvalTable + 窗口 | ≤ 512KB | 按公式窗口配置 |
| ActionSlot | 256 × 80B ≈ 20KB | 含 payload |
| lastState | 1024 × 4B ≈ 4KB | |
| bblot 队列 | ≤ 256KB | 异步，有界 |
| ScanEngine + 驱动 | 视协议 | 主要常驻 |
| **Pipeline 合计** | **< 1MB** | 不含 ScanEngine |
| **设备总预算** | 256MB–2GB | Pipeline 占比 < 0.5% |

---

## 12. Benchmark 目标

| 基准 | 目标 | 命令 / 门禁 |
|------|------|-------------|
| Worker 单事件延迟 | P95 < 50µs（纯 CPU） | `BenchmarkPipelineWorker_Event` |
| allow+eval+match | 0 allocs/op | `-benchmem` |
| Ring buffer Push/Ppop | 0 allocs/op | `BenchmarkRingBuffer` |
| 端到端（含 Shadow 写入） | P95 < 20ms @ 1k eps | 扩展 `armv7_bench_test.go` |
| 72h soak | MemInuseDrift ≤ 5% | `TestARMv7_Q3BenchmarkGate` 扩展 |
| GC STW | max < 5ms | runtime/metrics |

**交叉编译门禁**（已有）：

```bash
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go test -c ./internal/pipeline/ -o /tmp/edgeCore-pipeline-armv7.test
```

---

## 13. Go Struct 布局速查

```go
// model/point_cache.go
type PointCache struct {
    Value int32 // 定点或原始值
    Ts    int64 // UnixNano
}

// model/trigger.go — 栈上传递，非持久化
type Trigger struct {
    Source uint32
    Value  int32
    Code   uint8
}

// RingEvent — ring buffer 单元
type RingEvent struct {
    SourceIdx uint32
    Raw       int32
    Ts        int64
    Quality   uint8
    _         [3]byte // 对齐至 24B
}
```

**包路径汇总**：

```text
internal/pipeline/worker.go
internal/pipeline/ring_buffer.go
internal/pipeline/eval.go
internal/pipeline/decide.go
internal/pipeline/execute.go
internal/pipeline/feedback.go
internal/pointcache/cache.go
model/trigger.go
model/point_cache.go
```

---

## 14. 从现有代码迁移清单

| 步骤 | 动作 |
|------|------|
| 1 | 新增 `internal/pipeline/`，build tag `embedded` |
| 2 | ScanEngine 采集后双写：Shadow（UI）+ RingBuffer（Pipeline） |
| 3 | `point_bindings` 编译：复用 `config_store` 点位列表 |
| 4 | EdgeRule 子集编译器：P1 从 `EdgeRules` bucket 生成 `pipeline_rules` |
| 5 | ECM handler 从 `DataPipeline` 移除（embedded profile） |
| 6 | 诊断 API `/api/diagnostics/pipeline` |
| 7 | 扩展 `armv7_bench_test.go` 含 Pipeline 路径 |

**保留 1.x 路径**：`build tag gateway` 继续编译 ECM + V2.1 六段链（见 [边缘计算优化升级 2.0](./边缘计算优化升级2.0.md) 附录 A）。

---

## 15. P0 实现顺序（与主文档对齐）

1. `ring_buffer.go` + 单测（0 alloc）
2. `pointcache/cache.go` + 单测
3. `decide.go` RuleTable + init 加载 mock 规则
4. `eval.go` passthrough + 阈值
5. `execute.go` control stub（WritePoint mock）
6. `feedback.go` counter + recorder 对接
7. `worker.go` 整合 + soak 测试
8. ScanEngine hook + `config_store` bucket

---

## 附录：交叉引用

- [边缘计算优化升级 2.0](./边缘计算优化升级2.0.md) — PRIMARY 架构与 P0–P3 · **§8 功能对齐映射**
- [边缘计算 Pipeline 配置指南](../edge/边缘计算Pipeline配置指南.md) — 用户配置术语与 Pipeline 对照
- [数据源与输出动作设计](../architecture/数据源与输出动作设计.md) — §1.0 嵌入式 Profile
- `internal/core/scan_engine.go` · `pipeline.go` · `edge_compute_manager.go`
- `internal/core/armv7_bench_test.go` — ARM 门禁基线

---

*维护：架构组 | 下次审查：P0-1 `worker.go` 首 PR 合并时*
