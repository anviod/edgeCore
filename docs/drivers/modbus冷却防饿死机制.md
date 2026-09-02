***

layout: default
title: Modbus 冷却防饿死机制
description: edgeCore Modbus 点位冷却与设备级故障隔离（参考 Kepware 的最优解）
----------------------------------------------------------

# Modbus 冷却防饿死机制

> 适用驱动：`modbus-tcp` / `modbus-rtu` / `modbus-rtu-over-tcp`（含 `*` `-simple` 兼容别名）
>
> 核心思想（参考 Kepware GE OPC / KEPServerEX）：
> 通信层故障归因于**设备（Device / Channel）**，而非**点位（Tag）**。
> 设备故障按扫描周期轮询恢复，点位不做永久跳过；只有点位的确定性错误（如非法数据地址）才在点位层面冷却。

***

## 1. 问题背景

### 1.1 现场现象

设备状态条显示：

```
连接状态:已断开 | 协议: modbus-tcp | 连续通信:0 次 | 最近失败:2026-09-02 18:00:58
```

连接的 modbus-tcp 设备离线后，扫描任务对**每一个点位**的读请求都失败。
旧实现把这类失败统一计入点位失败计数并触发**点位级 SKIPPED 冷却**
（连续 3 次失败 → 冷却 60s，连续 10 次失败 → 冷却 5 分钟）。

### 1.2 缺陷（饿死 / Starvation）

- 设备离线时：所有点位被逐个打入 SKIPPED 冷却；

- 设备恢复在线时：这些点位仍处于冷却期，`prepareRuntimes()` 会将其过滤掉而**不发起读取**；

- 结果：连接已恢复，但设备呈现"全点位 Bad / 无数据"，直到冷却逐一过期——即**冷却机制反向饿死了健康采集**。

### 1.3 根因

冷却机制只考虑了"隔离坏点位，防止其拖垮批次查询"，却没有区分**失败归因**：
把"设备离线/链路超时"这一**设备级故障**错误地当成了"点位故障"来冷却。

***

## 2. 设计目标（参照 Kepware）

| 维度   | Kepware 做法          | 本项目改造                                  |
| ---- | ------------------- | -------------------------------------- |
| 故障归因 | 通信/超时失败归因于设备        | 连接级错误不再计入点位冷却                          |
| 点位隔离 | 仅确定性点位错误（非法地址）做点位处理 | 非法地址永久跳过（人工重置），其余短暂阶梯（60s / 5min）      |
| 恢复   | 按设备扫描周期轮询，恢复即读      | 连接恢复瞬间全点位可读（0 饿死等待）                    |
| 全局保护 | 设备错误不阻塞其他设备         | 依旧由 ConnectionManager 退避 / 冷却 / 全局限流兜底 |

***

## 3. 两点因分离（本文核心）

在 `PointScheduler.markPointFailed(pointID, err)` 入口对错误做一级分类：

```go
func (s *PointScheduler) markPointFailed(pointID string, err error) {
    if rt, ok := s.pointStates[pointID]; ok {
        // 连接/传输级失败 = 设备级故障：点位解冻，保持 OK，不计冷却。
        // 恢复瞬间全点位立即可读，避免饿死。
        if err != nil && isConnectionError(err) {
            rt.FailCount = 0
            rt.State = "OK"
            rt.CooldownUntil = time.Time{}
            return
        }
        rt.FailCount++
        // 点位级确定性错误 → 保留原 SKIPPED 阶梯
        if isIllegalAddress(err) { ... permanent skip (人工重置) ... }
        else if rt.FailCount >= 10 { ... 5min ... }
        else if rt.FailCount >= 3 { ... 60s ... }
    }
}
```

### 3.1 连接级错误（视为设备级，点位不冷却）

`isConnectionError(err)` 命中以下子串即判断为链路/传输级故障：

- `not connected` / `modbus: not connected`

- `timeout` / `i/o timeout` / `request timed out`

- `connection refused` / `connection reset` / `connection closed`

- `broken pipe` / `network unreachable` / `no route to host`

- `dial `     / `tls handshake` / `cannot assign requested address`

行为：

- **不累计** `FailCount`（避免把"设备离线"误判成"点位坏"）；

- **保持 / 强制恢复为** `State="OK"`，清空 `CooldownUntil`；

- 若点位此前已被误判为 SKIPPED，也一并**解冻**。

### 3.2 点位级错误（Kepware 简化：地址错永久跳过，人工重置）

仅当**单点隔离**且协议**显式返回** **`Illegal Data Address`（异常码2）时，判定为真正的非法地址，
永久跳过**（不再自动 24h 重试），直到人工重置才恢复：

| 错误 / 场景                                 | 处理                           |
| --------------------------------------- | ---------------------------- |
| 单点读 → 协议显式 `Illegal Data Address`（异常码2） | **永久跳过**，仅人工重置恢复             |
| 点位级其它错误（`read length mismatch`、解码失败等）   | 短暂冷却阶梯（3次→60s，10次→5min），自动重探 |
| **整组批量读失败**（设备离线/批量异常文本即使含 "illegal"）   | **绝不判为非法地址**，走短暂阶梯或由连接级解冻恢复  |
| `illegal data value`（异常码3）等其它文本         | 不判非法地址                       |

> 设计要点：
>
> 1. `isIllegalDataAddress()` 只匹配协议显式地址异常，`illegal data value`/`busy` 等不再误命中；
> 2. 多点点位的**批量失败属于整组/设备级**问题，`markPointFailed(..., pointLevel=false)`
>    不进入非法地址分支，防止"设备离线全点位 Bad"被误降级为永久非法地址；
> 3. **人工重置**：编辑点位关键配置（地址 / 寄存器类型 / 数据类型）后，
>    `prepareRuntimes` 检测到配置变更即自动解冻该点位重新采集，无需重启进程
>    （参照 Kepware：改配置即恢复）。

### 3.3 采集裁决隔离（设备在线 ≠ 点位全部可读）

点位冷却必须与**设备连通性判定**解耦。若不加隔离就会上演经典饿死：

- 设备所有点位都被点位冷却（如协议显式 `illegal address` 永久跳过）；

- `scheduler.Read` 因无活跃点位而**未做任何 I/O**，返回 `空结果 + nil error`；

- 旧裁决把空结果等价为"全部失败 / 设备不可达" → `onCollectUnreachable` → 设备 **Offline**；

- 冷却门控让下轮只会在冷却期后采集，点位不到冷却到期不恢复 → 设备永久离线，**只有重启进程清空内存态才重新上线**。

修复：`collectContextFromExecuteResult` 对 **`nil error + 空结果`** 判为"空闲采集"（链路健康、点位在冷却），按一次成功计入——
设备保持在线，冷却点位仍在点位列表单独展示。**设备在线状态只由真实失败裁定，点位冷却不再反向拖垮设备。**

| 裁决输入                   | 结果                                 |
| ---------------------- | ---------------------------------- |
| 链路 error（断连/超时）        | `onCollectUnreachable` → 离线（真实故障）  |
| 空结果 + nil error（点位全冷却） | 空闲成功 → 设备保持/恢复在线                   |
| 有值但全 Bad（从站响应超时）       | `onCollectUnreachable` → 离线（从站无应答） |
| ≥30% 点位 Good           | `onCollectSuccess` → 在线并复位失败计数     |

***

## 4. 三层冷却职责划分（全链路）

| 层级  | 组件                  | 职责                                           | 决定因素    |
| --- | ------------------- | -------------------------------------------- | ------- |
| 驱动级 | `ConnectionManager` | 连接建立/重连的指数退避 + Dead 冷却 + Half-Open 探测 + 全局限流 | 是否真的建连  |
| 点位级 | `PointScheduler`    | 点位失败隔离（仅点位级错误冷却）                             | 错误归因（新） |
| 任务级 | `ScanEngine`        | 任务优先级 / 降级 / 防饿死（300s 强派发）                   | 任务是否被调度 |

连接级错误由**驱动级**Drop：点位调度不再被连接故障拖入冷却，连接健康由
`ConnectionManager` 单独管理的退避/冷却节奏负责，二者解耦。

***

## 5. 状态转移（点位视角）

```
        连接级错误                   点位级错误
   ┌─────────────────┐         ┌──────────────────────┐
   │ 解冻→OK（不计次数） │         │ FailCount++           │
   │ 恢复立即可读        │         │ 3次→SKIPPED 60s        │
   │                    │         │ 10次→SKIPPED 5min     │
   │                    │         else if 点位级重试超出 → SKIPPED 短暂阶梯  │
        │ illegal（单点显式）→ SKIPPED 永久(人工重置) │
   └─────────────────┘         └──────────────────────┘
          ▲                              │
          │ 连接恢复（解冻）               │ 冷却到期 / 成功
          └──────────────────────────────┘
                           ▼
                        State=OK
```

- 连接级错误不会让点位离开 OK；

- 点位级冷却到期后自动回 OK；

- 读到成功亦回 OK。

***

## 6. 关键代码位置

| 文件                                       | 说明                                                         |
| ---------------------------------------- | ---------------------------------------------------------- |
| `internal/driver/modbus/scheduler.go`    | `isConnectionError()`、`markPointFailed()` 解冻逻辑             |
| `internal/driver/modbus/scheduler.go`    | 点位 SKIPPED 阶梯、`prepareRuntimes()` 过滤                       |
| `internal/driver/connection_manager.go`  | 连接状态机、退避、冷却、限流（设备级兜底）                                      |
| `internal/driver/modbus/transport.go`    | `isDeviceLevelModbusError` / `RecordFailure`（通道级，TCP 复用保护） |
| `internal/core/connection_controller.go` | 观测层错误分类 / 健康分（只读，不触发重连）                                    |

***

## 7. 单元测试覆盖

`internal/driver/modbus/scheduler_cooling_test.go`

| 用例                                                                | 断言                            |
| ----------------------------------------------------------------- | ----------------------------- |
| `TestCooling_ConnectionLevelFailureDoesNotStarve`                 | 10 次连接级失败：点位始终 OK、FailCount=0 |
| `TestCooling_ConnectionLevelErrorThawsSkippedPoint`               | 连接级错误解冻已 SKIPPED 的点位          |
| `TestCooling_GroupLevelIllegalTextDoesNotPermanentSkip`           | 整组批量失败即使文本含异常2 也不永久跳过（仅短暂阶梯）  |
| `TestCooling_IsIllegalDataAddressRequiresExplicitAddressFault`    | 仅协议显式地址异常2 判非法；异常3/busy/超时否   |
| `TestCooling_IllegalAddressPermanentAndManualResetByConfigChange` | 单点非法地址永久跳过；改配置即人工重置           |
| `TestCooling_PointLevelErrorStillDegrades`                        | 点位级非非法错误 → 60s/5min 短暂阶梯      |
| `TestPointScheduler_markPointFailed_TimeoutDoesNotCool`           | timeout 不计点位失败，保持 OK          |

采集裁决层（`internal/core/channel_device_state_test.go`）：

| 用例                                                                 | 断言                        |
| ------------------------------------------------------------------ | ------------------------- |
| `TestCollectContextFromExecuteResult_AllSkippedIdleDoesNotOffline` | 空结果+成功 → 空闲成功，不判不可达       |
| `TestFinalizeScanCollect_AllSkippedKeepsDeviceOnline`              | 点位全冷却时空闲成功使设备从离线恢复在线（免重启） |
| `TestFinalizeScanCollect_AllBadPointsMarksOffline`                 | 有值全 Bad（从站无应答）→ 仍判离线（不回归） |

***

## 8. 效果对比

| 场景                     | 改进前                            | 改进后                          |
| ---------------------- | ------------------------------ | ---------------------------- |
| modbus-tcp 设备离线一段时间后恢复 | 点位多在 SKIPPED 冷却期，恢复后仍需逐个等待冷却过期 | 连接恢复瞬间全点位立即重读，0 饿死等待         |
| 设备离线期间                 | 所有点位被误判冷却，浪费大量重试               | 点位保持 OK，仅由驱动级退避/限流控制节奏       |
| 真·坏点位（协议显式非法地址）        | 自动 24h 重试，且字母宽松误判风险高           | 永久跳过，仅人工重置（改配置）；判定收紧为协议显式异常2 |
| 网络抖动引发的整条链路超时          | 连锁冷却整个设备                       | 点位不受牵连，只按设备级快速重试             |

