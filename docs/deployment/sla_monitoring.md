# ScanEngine SLA 轻量化运维手册

> **版本：** 1.0 · 2026-07-02  
> **原则：** 零外部依赖（无 Prometheus/Grafana），通过 HTTP diagnostics + 结构化日志 + UI 轮询完成巡检。

## 1. 三通路概览

| 通路 | 机制 | 入口 |
|------|------|------|
| **读** | REST diagnostics JSON | `GET /api/diagnostics/scan-engine` |
| **判** | 内置 `sla_warnings[]` 阈值 | 同上 + 单设备 diagnostics |
| **告** | zap WARN 日志 + channel Event Log | 日志 grep + `GET /api/channels/:id/diagnostics/events` |
| **看** | UI 通道监控弹窗 SLA 区块 | 通道列表 → 监控 |

## 2. 巡检命令

### 2.1 ScanEngine SLA 快照

```bash
curl -s http://localhost:8082/api/diagnostics/scan-engine | jq '{
  lag_p95: .scan_lag_p95_ms,
  drift: .scan_drift_avg_ms,
  miss: .scan_miss_deadline_total,
  cb_open: .driver_circuit_open_total,
  cb_reject: .driver_circuit_reject_total,
  backpressure_reject: .backpressure_reject_total,
  serial_queue: .serial_queue_depth,
  gc_pause_max_ms: .gc_pause_max_ms,
  warnings: .sla_warnings
}'
```

### 2.2 单设备断路器

```bash
curl -s http://localhost:8082/api/devices/modbus-slave-1/diagnostics | jq .
```

### 2.3 通道 Event Log（CB Open/Reject）

```bash
curl -s http://localhost:8082/api/channels/<channelId>/diagnostics/events | jq .
```

## 3. 日志 grep

SLA 周期告警（每 30s 扫描，有告警才输出）：

```bash
grep '\[SLA\]' /var/log/edgeCore/app.log
grep 'circuit_breaker_open\|circuit_breaker_reject' /var/log/edgeCore/app.log
grep 'scan_lag_p95_exceeded\|scan_drift_avg_exceeded' /var/log/edgeCore/app.log
```

## 4. 阈值对照

| 指标 | 字段 | x86 mock 阈值 |
|------|------|---------------|
| 调度 lag P95 | `scan_lag_p95_ms` | <100ms（soak 放宽 200ms） |
| 漂移均值 | `scan_drift_avg_ms` | <50ms |
| miss deadline | `scan_miss_deadline_total` | 稳态 =0 |
| GC pause max | `gc_pause_max_ms` | <20ms |
| 反压拒绝 | `backpressure_reject_total` | 趋势监控 |
| 串行队列深度 | `serial_queue_depth` | 共享链路拥塞预警 |

## 5. UI 路径

1. 打开 **通道列表**
2. 点击通道 **监控** 按钮
3. 查看 **调度 SLA** 区块：Lag P95、CB Open、反压拒绝、`sla_warnings` 列表
4. 质量评分已纳入 lag P95 与 CB Open 扣分（C-06）

## 6. 压测 / soak 回归

| 层级 | 命令 |
|------|------|
| PR 快测 | `make test-short` |
| PR soak 门控 | `make test-soak-short` |
| Nightly 1h | `make test-soak` |
| Q3 10k | `make bench-q3` |
| ARMv7 | `scripts/bench_armv7.sh` |

## 7. 升级与告警响应

1. `sla_warnings` 非空 → 查 Event Log 定位 channel/device
2. `serial_queue_depth` 某 key ≥ 50% 容量 → 检查共享链路上慢/离线从站
3. `driver_circuit_open_total` 增加 → 设备侧网络或 slave 异常，等待 30s HalfOpen 探测
4. `backpressure_reject_total` 持续升 → 降载或扩容 worker / 调大 scan interval

## 8. 相关文档

- `docs/TODO/SLA评估.md`
- `docs/testing/modbus_live_report.md`
- `docs/testing/shadow_optimization_report_2026Q3.md`
