---
layout: default
title: 边缘计算最佳实践
description: edgeCore 边缘计算最佳实践 — 场景编排与性能建议
---

# 边缘计算最佳实践

> **文档定位：** 场景编排与性能建议。面向现场工程师与集成商，帮助在 edgeCore 网关上稳定部署规则引擎。架构说明见 [边缘计算基础功能](../edge/边缘计算基础功能.html)。

---

## 一、设计原则

1. **本地闭环优先** — 告警联动、安全停机应在网关完成，不依赖云端可达
2. **一规则一职责** — 每条规则只做一件事；复杂流程用 `sequence` 串联
3. **与采集周期对齐** — `check_interval` ≥ 相关点位 ScanEngine 采集周期
4. **可观测先于优化** — 先通过 `/api/edge/states` 与日志验证，再调频率与优先级
5. **失败可恢复** — 关键写点配合 `check`；MQTT/写点失败进入 `DataCache` 自动重试

---

## 二、场景编排

### 2.1 编排模式

| 模式 | 动作组合 | 适用 |
|------|----------|------|
| 简单告警 | `log` + `mqtt` | 环境越限、状态位变化 |
| 本地联动 | `device_control` + `log` | 温度启停冷却、阀门联动 |
| 顺序启停 | `sequence` + `delay` | 产线设备依次启动 |
| 安全联锁 | `threshold/state` + `device_control` + 高 `priority` | 光栅/急停 |
| 写后校验 | `device_control` + `check` + `on_fail` | 关键指令确认 |
| 聚合上报 | `window`/`calculation` + `mqtt` | 趋势预警、派生指标 |

### 2.2 Sequence 编排要点

```json
{
  "type": "sequence",
  "config": {
    "steps": [
      { "type": "device_control", "config": { "value": "1", "...": "..." } },
      { "type": "delay", "config": { "duration": "2s" } },
      { "type": "check", "config": { "expression": "v == 1", "on_fail": [...] } },
      { "type": "log", "config": { "level": "info", "message": "步骤完成" } }
    ]
  }
}
```

- 步骤失败时 sequence **整体中断**；关键步骤应加 `on_fail` 回滚或告警
- `delay` 会占用 Worker 协程，长延时序列不宜过多并行规则
- 启停类建议 `trigger_mode: on_change`，避免重复执行整条序列

### 2.3 多规则协作

- **优先级分层**：安全联锁 `priority: 100`，普通告警 `10`，聚合计算 `5`
- **避免循环写点**：规则 A 写点 X → 触发规则 B 读 X 再写 Y → 再触发 A，需用 `on_change` 或条件互斥
- **北向与边缘分离**：北向 MQTT 持续上报由 NorthboundManager 负责；边缘 MQTT 动作用于**事件型**告警

### 2.4 UI 场景模版

**边缘计算 → 场景模版** 提供可套用骨架（`ui/src/utils/edgeSceneTemplates.js`），套用后绑定真实通道/点位再启用。详见 [场景手册](../edge/EDGE_COMPUTING_SCENARIO_MANUAL.html)。

---

## 三、性能建议

### 3.1 规则数量与频率

| 场景规模 | 建议 |
|----------|------|
| &lt; 50 条规则 | `check_interval` 1s～5s 可接受 |
| 50～200 条 | 非关键规则 ≥ 10s；合并同源条件 |
| &gt; 200 条 | 审计禁用规则；Window 规则控制 `size` 样本上限（引擎单规则最多 10000 样本） |

引擎默认 **250ms 批合并**、**10 Worker**；突发触发过多时关注 `rules_dropped`、`rules_coalesced`（`GET /api/edge/metrics`）。

### 3.2 与 ScanEngine 协同

- 10k+ 点位场景：优先保证 ScanEngine SLA（`GET /api/diagnostics/scan-engine`）
- 规则索引按点位 O(1) 查找；**每条规则绑定点位越少越好**
- VirtualShadow 计算点位写入 Shadow 后同样进入 Pipeline，计入同一评估负载

### 3.3 Window / Calculation 开销

- Window 每次样本更新可能触发缓冲与磁盘 `WindowData` 异步保存
- `window.interval` 可降频聚合评估（如 `10s` 步长 + `60s` 窗）
- Calculation 每周期都执行表达式，避免在 `check_interval: 500ms` 下绑定大量三角函数/复杂公式

### 3.4 动作执行

- 顶层 `actions[]` **并行** — 无顺序保证；有顺序必须用 `sequence`
- `device_control` RMW 含读点 RTT，批量 `targets[]` 时注意驱动写吞吐
- MQTT `send_strategy: batch` 合并推送，减少北向连接压力

---

## 四、安全与可靠

### 4.1 安全联锁

- 使用 `type: state` 或 `state.count` 过滤瞬时干扰
- `priority` 高于普通规则；`check_interval: 500ms` 或更短
- 停机动作后避免自动重启逻辑在同一规则内与之冲突

### 4.2 告警风暴

- `trigger_mode: on_change` 防止持续越限重复 MQTT
- MQTT/HTTP 动作配置 `config.interval` 限速
- 北向引用 `mqtt_config_id` 复用连接，避免每规则新建客户端

### 4.3 失败处理

- `mqtt`、`device_control` 失败写入 `DataCache`，30s 重试，最多 10 次
- 持久错误查 `GET /api/edge/failures`、`GET /api/edge/logs`
- 运维可 `POST /api/edge/logs/clear` 清理错误日志并 compact runtime.db

---

## 五、部署检查清单

- [ ] 南向点位 Shadow 页数据质量为 Good
- [ ] 规则 `sources` 别名与 `condition` 一致
- [ ] 测试环境先用 `log` 动作验证触发
- [ ] 生产启用前确认 `mqtt_config_id` / `http_config_id` 有效
- [ ] 安全类规则单独高优先级并文档化
- [ ] 监控 `edge/metrics`：`worker_pool_usage`、`rules_dropped`
- [ ] 定期导出/清理历史错误日志

---

## 六、常见问题

| 现象 | 可能原因 | 处理 |
|------|----------|------|
| 规则从不触发 | 点位未更新、`check_interval` 过长、条件写错 | 查 Shadow 值与 `/api/edge/states` |
| 重复告警 | `trigger_mode: always` | 改为 `on_change` |
| 抖动误报 | 无 `state` 维持 | 加 `duration` 或 `count` |
| MQTT 无消息 | `mqtt_config_id` 错误或 UI 用了 `mqtt_id` | 统一用 `mqtt_config_id` |
| Window 不触发 | 未到 `interval` 步长 | 等待或减小 `interval` |
| Worker 满 | 规则过多/过频 | 降频、合并规则、查 `rules_dropped` |

---

## 相关文档

- [边缘计算基础功能](../edge/边缘计算基础功能.html)
- [边缘计算规则帮助](../edge/边缘计算规则帮助.html)
- [场景手册](../edge/EDGE_COMPUTING_SCENARIO_MANUAL.html)
- [边缘计算 API](../API/Edge_Computing_CN.html)
