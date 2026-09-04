---
layout: default
---

# 质量评分规则文档 (Quality Score Rules)

本文档详细说明了 edgeCore 系统中通道 (Channel) 和设备 (Device) 的质量评分计算逻辑、评分等级定义以及改进建议。

## 1. 评分体系概述

系统通过实时采集通信指标，计算出 0-100 分的质量评分。
- **通道评分 (Channel Quality Score)**: 反映该通道下整体通信链路的稳定性。
- **设备健康分 (Device Health Score)**: 反映单个设备的连接状态和数据完整性。

## 2. 评分等级定义 (Rating Levels)

前端显示将根据分数映射到以下等级：

| 分数区间 (Score) | 等级 (Level) | 颜色 (Color) | 含义 |
| :--- | :--- | :--- | :--- |
| **100** | **Perfect** | <span style="color:blue">Primary (Blue)</span> | 各项指标完美，无任何错误或延迟扣分。 |
| **90 - 99** | **Excellent** | <span style="color:green">Success (Green)</span> | 极其稳定，偶有极少量重试或延迟波动。 |
| **80 - 89** | **Good** | <span style="color:#4CAF50">Success (Light Green)</span> | 状态良好，可能存在零星的超时或丢包，但不影响整体业务。 |
| **60 - 79** | **Average** | <span style="color:orange">Warning (Orange)</span> | 处于亚健康状态，存在明显的丢包、超时或连续失败，需关注。 |
| **< 60** | **Bad** | <span style="color:red">Error (Red)</span> | 连接严重不稳定或完全断开，急需维护。 |

---

## 3. 详细计算规则 (Backend Logic)

### 3.1 通道质量评分 (Channel Metrics)

源文件: `internal/model/metrics.go` -> `calculateQualityScore`

**基准分**: 100 分

**扣分项**:

1.  **成功率 (Success Rate)** (权重: 40%)
    *   公式: `扣分 = (1.0 - SuccessRate) * 40`
    *   示例: 95% 成功率 -> 扣 2 分; 50% 成功率 -> 扣 20 分。

2.  **CRC 错误率 (CRC Error Rate)** (权重: 20%)
    *   公式: `扣分 = CrcErrorRate * 20`
    *   示例: 10% 误码率 -> 扣 2 分。

3.  **超时率 (Timeout Rate)** (权重: 20%)
    *   公式: `扣分 = TimeoutRate * 20`
    *   示例: 10% 超时率 -> 扣 2 分。

4.  **响应时间 (RTT)** (阈值扣分)
    *   阈值: 平均 RTT > 100ms 开始扣分。
    *   公式: `扣分 = min(10, (AvgRtt - 100) / 50)`
    *   示例: 150ms -> 扣 1 分; 600ms+ -> 扣 10 分 (封顶)。

### 3.2 设备健康评分 (Device Metrics)

源文件: `internal/model/metrics.go` -> `calculateDeviceHealthScore`

**基准分**: 100 分

**扣分项**:

1.  **连续失败次数 (Consecutive Failures)**
    *   规则: 每次连续失败扣 10 分，最多扣 30 分。
    *   影响: 1 次失败=90分, 2 次=80分, 3 次及以上=70分 (此处已进入 Average 区间)。

2.  **点位成功率 (Point Success Rate)** (权重: 40%)
    *   公式: `扣分 = (1.0 - PointSuccessRate) * 40`
    *   影响: 若设备在线但部分点位读取失败，会拉低分数。

3.  **异常点位数量 (Abnormal Points)**
    *   规则: 每个异常点位扣 5 分，最多扣 20 分。
    *   定义: 点位值无效、类型错误或读取报错。

---

## 4. 改进清单与验证方法 (Actionable Items)

### 针对 "Bad" 或 "Average" 评分的排查步骤：

1.  **检查物理连接**:
    *   验证 IP/端口是否正确。
    *   检查网线/串口线是否松动。
    *   Ping 设备 IP 查看延迟和丢包率。

2.  **优化超时设置**:
    *   若 **RTT 扣分** 较高: 增加 `Timeout` 配置 (默认 2000ms 可能不足)。
    *   若 **连续失败** 频繁: 检查设备是否因请求过快而拒绝响应，增大 `Instruction Interval` (指令间隔)。

3.  **排查干扰 (CRC 错误)**:
    *   检查 RS485 终端电阻。
    *   检查接地和屏蔽线。

4.  **验证点位配置**:
    *   若 **异常点位** 扣分: 检查点位地址是否存在。使用 "扫描设备" 功能重新发现有效点位。

### 验证方法:

1.  **前端验证**:
    *   刷新页面，观察颜色变化。
    *   100 分应显示蓝色 "Perfect"。
    *   90-99 分应显示绿色 "Excellent"。

2.  **API 验证**:
    *   调用 `/api/channels/{id}/metrics` 查看原始 `qualityScore`。
    *   调用 `/api/channels/{id}/devices` 查看设备 `quality_score`。

3.  **日志验证**:
    *   查看系统日志中是否有 "timeout" 或 "CRC error" 警告。
