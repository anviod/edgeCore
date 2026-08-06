---
layout: default
title: 边缘计算 API
description: edgeCore 边缘计算 REST API — 规则、指标与日志接口
---

# 边缘计算 API

> **文档定位：** 规则、指标与日志接口。所有端点挂载于 `/api` 前缀，需 JWT 认证（`Authorization: Bearer <token>`）。路由注册见 `internal/server/server.go`。

**Base URL 示例：** `https://<gateway-host>:8080/api`

---

## 一、EdgeRule 数据模型

定义于 `internal/model/types.go`。

```json
{
  "id": "uuid",
  "name": "规则名称",
  "type": "threshold",
  "enable": true,
  "priority": 10,
  "check_interval": "5s",
  "trigger_mode": "always",
  "sources": [
    {
      "alias": "t1",
      "channel_id": "ch1",
      "device_id": "dev1",
      "point_id": "temp",
      "point_name": "温度"
    }
  ],
  "condition": "t1 > 80",
  "expression": "",
  "actions": [
    { "type": "log", "config": { "level": "warn", "message": "越限" } }
  ],
  "window": { "size": "60s", "interval": "10s", "aggr_func": "avg" },
  "state": { "duration": "10s", "count": 3 }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | string | `threshold` · `state` · `window` · `calculation` |
| `trigger_mode` | string | `always` · `on_change` |
| `trigger_logic` | string | UI 保留字段，**引擎未实现** |
| `actions[].type` | string | `log` · `device_control` · `mqtt` · `http` · `database` · `sequence` · `delay` · `check` |

---

## 二、规则管理

### GET /api/edge/rules

获取全部边缘规则。

**响应：** `EdgeRule[]`

**状态码：** `200` · `503`（EdgeComputeManager 未初始化）

---

### POST /api/edge/rules

创建或更新规则（Upsert）。`id` 为空时服务端自动生成 UUID。

**请求体：** `EdgeRule`

**响应：** 保存后的 `EdgeRule`

**状态码：** `200` · `400` · `500` · `503`

> 规则持久化至 `data/config.db` → `EdgeRules` 桶。

---

### DELETE /api/edge/rules/:id

删除指定规则。

**路径参数：** `id` — 规则 UUID

**响应：** `200` 空 body

---

## 三、运行时状态

### GET /api/edge/states

获取所有规则运行时状态。

**响应：** `RuleRuntimeState[]` 或 map（以实现为准）

**RuleRuntimeState 主要字段：**

| 字段 | 说明 |
|------|------|
| `rule_id` / `rule_name` | 规则标识 |
| `current_status` | `NORMAL` · `WARNING` · `ALARM` |
| `last_check_time` / `last_trigger` | 最近评估/触发时间 |
| `trigger_count` / `success_count` / `failure_count` | 计数 |
| `execution_phase` | `idle` · `window` · `evaluate` · `state_hold` · `trigger` · `action` · `completed` · `error` |
| `error_message` | 最近错误 |

持久化桶：`runtime.db` → `RuleState`

---

### GET /api/edge/rules/:id/window

获取 Window 规则当前样本缓冲。

**路径参数：** `id` — 规则 ID

**响应：** `model.Value[]`（含 `channel_id`、`device_id`、`point_id`、`value`、`ts`）

---

## 四、指标接口

### GET /api/edge/metrics

边缘引擎执行指标（`EdgeComputeMetrics`）。

**响应示例：**

```json
{
  "worker_pool_size": 10,
  "worker_pool_usage": 2,
  "rule_count": 45,
  "cache_size": 1200,
  "window_buffer_total": 380,
  "pending_scheduler_tasks": 0,
  "minute_cache_size": 12,
  "event_buffer_size": 3,
  "failure_buffer_size": 1,
  "batch_window_ms": 250,
  "rules_triggered": 1024,
  "rules_executed": 980,
  "rules_dropped": 0,
  "rules_coalesced": 44,
  "rules_debounced": 12
}
```

| 字段 | 说明 |
|------|------|
| `rules_coalesced` | 批窗口内合并的重复触发 |
| `rules_dropped` | 队列满或 pending _cap 淘汰 |
| `rules_debounced` | check_interval 节流跳过 |

**相关：** 仪表盘 `GET /api/dashboard/summary` 含 `edge_rules` 摘要；ScanEngine `GET /api/diagnostics/scan-engine` 为采集层指标。

---

### GET /api/edge/cache

获取失败动作重试队列（`DataCache` 桶）。

**响应：** `FailedAction[]`

```json
{
  "id": "1730000000000000000",
  "rule_id": "...",
  "action": { "type": "mqtt", "config": {} },
  "retry_count": 2,
  "last_error": "connection refused",
  "timestamp": "2026-07-08T10:00:00Z"
}
```

仅 `mqtt`、`device_control` 失败会入队；30s 周期重试，最多 10 次。

---

## 五、日志与事件接口

> **说明：** 持久化日志**仅记录错误与丢弃事件**，正常成功触发不写 bblot/events 桶。

### GET /api/edge/logs

分钟级错误快照（`bblot` 桶），UI「记录与日志」主数据源。

**查询参数：**

| 参数 | 格式 | 说明 |
|------|------|------|
| `rule_id` | string | 按规则筛选 |
| `category` | string | 错误分类 |
| `channel_id` / `device_id` | string | 关联设备 |
| `start_date` / `end_date` | `YYYY-MM-DD HH:mm` | 时间范围 |

**响应：** `RuleMinuteSnapshot[]`（最多 1000 条，按时间降序）

```json
{
  "rule_id": "...",
  "rule_name": "温度告警",
  "minute": "2026-07-08 10:05",
  "error_type": "execution_error",
  "error_message": "mqtt publish failed",
  "updated_at": "2026-07-08T10:05:32Z"
}
```

**错误类型：** `formula_error` · `execution_error` · `timeout` · `dispatch_error` · `other`

---

### POST /api/edge/logs/clear

清除边缘错误日志并 compact `runtime.db`。

**响应：**

```json
{
  "status": "success",
  "message": "edge logs cleared",
  "cleared": { "...": "..." },
  "compact": { "...": "..." }
}
```

---

### GET /api/edge/events

结构化规则执行事件（错误/丢弃生命周期）。

**查询参数：** `rule_id`（可选）· `limit`（默认 100）

**响应：** `EdgeRuleEvent[]`

---

### GET /api/edge/failures

结构化失败记录。

**查询参数：** `rule_id`（可选）· `limit`（默认 100）

**响应：** `EdgeFailureRecord[]`

含 `phase`、`action_type`、`action_index`、`error` 等字段。

---

### GET /api/edge-compute/logs（兼容）

旧版日志查询，调用 `EdgeComputeManager.QueryLogs`。

**查询参数：**

| 参数 | 格式 | 默认 |
|------|------|------|
| `rule_id` | string | — |
| `start` | `YYYY-MM-DD HH:mm` | 24h 前 |
| `end` | `YYYY-MM-DD HH:mm` | 当前 |

**响应：** `RuleMinuteSnapshot[]`

---

### GET /api/edge-compute/logs/export

同上筛选条件，导出 CSV。

**响应：** `Content-Type: text/csv` 附件

---

## 六、动作配置参考

### mqtt

```json
{
  "type": "mqtt",
  "config": {
    "mqtt_config_id": "nb-mqtt-01",
    "topic": "edge/alarm",
    "message": "{\"v\":${value}}",
    "send_strategy": "batch"
  }
}
```

| 字段 | 说明 |
|------|------|
| `mqtt_config_id` | **推荐** — 北向 MQTT 配置 ID |
| `client_id` | 无 config_id 时的备选 |
| `send_strategy` | `batch` · `single` |

### http

```json
{
  "type": "http",
  "config": {
    "http_config_id": "nb-http-01",
    "body": "{\"temp\":${t1}}"
  }
}
```

或内联 `url`、`method`、`body`。

### device_control

```json
{
  "type": "device_control",
  "config": {
    "channel_id": "ch1",
    "device_id": "dev1",
    "point_id": "coil",
    "value": "1",
    "expression": "bitset(v, 2)",
    "interval": "1s",
    "targets": [
      { "channel_id": "ch1", "device_id": "d1", "point_id": "p1", "value": "0" }
    ]
  }
}
```

### check

```json
{
  "type": "check",
  "config": {
    "channel_id": "ch1",
    "device_id": "dev1",
    "point_id": "status",
    "expression": "v == 1",
    "retry": 3,
    "interval": "500ms",
    "timeout": "5s",
    "on_fail": [{ "type": "log", "config": { "level": "error", "message": "fail" } }]
  }
}
```

---

## 七、存储位置汇总

| 文件 | 桶 | 内容 |
|------|-----|------|
| `data/config.db` | `EdgeRules` | 规则定义 JSON |
| `data/runtime.db` | `RuleState` | 运行时状态 |
| `data/runtime.db` | `WindowData` | 窗口缓冲 |
| `data/runtime.db` | `DataCache` | 失败动作 |
| `data/runtime.db` | `bblot` | 分钟错误日志 |
| `data/runtime.db` | `edge_events` / `edge_failures` | 结构化事件 |
| 文件系统 | `logs/gateway.edgeCore.log` | `[EdgeCompute]` / `[EdgeAction]` 系统日志 |

---

## 八、错误响应

| HTTP | 含义 |
|------|------|
| `400` | 请求体/查询参数无效 |
| `401` | 未认证 |
| `503` | EdgeComputeManager 未初始化 |

错误体示例：`{"error": "..."}`

---

## 相关文档

- [边缘计算基础功能](../edge/边缘计算基础功能.html)
- [边缘计算规则帮助](../edge/边缘计算规则帮助.html)
- [认证 API](Authentication_CN.html)
