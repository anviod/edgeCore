---
layout: default
title: BACnet 驱动采集测试与验收标准清单
description: edgeCore BACnet 驱动采集测试与验收标准清单
---

# BACnet 驱动采集测试与验收标准清单
基于配置修改 `conf/channels.yaml`
使用最新配置文件来实现设备读取 BACnet 点位
例如 `conf/devices/bacnet-ip/bacnet-2228316.yaml`（路径相对于仓库根目录）
严格按照配置文件中的点位进行读取 配置文件不可修改的原则进行代码调整
特别注意：
* **DeviceID**：系统内唯一标识（Edge/平台侧）
* **Instance ID**：BACnet 网络唯一标识
* **ObjectID**：对象标识（Type + Instance）
* **Property**：对象属性标识

当前设备清单（验收范围 ：设备点位不可串流到一起）：

| 设备 Key | Instance ID | IP | 点位数量 | 验收状态 |
|---------|------------|-----|---------|---------|
| bacnet-2228316 | 2228316 | 192.168.3.115 | 11 | ✅ |
| bacnet-2228317 | 2228317 | 192.168.3.115 | 11 | ✅ |
| bacnet-2228318 | 2228318 | 192.168.3.115 | 11 | ✅ |
| bacnet-2228319 | 2228319 | 192.168.3.115 | 11 | ✅ |

> ⚠ 验收前提：已确认所有设备物理运行正常，网络正常
> ⚠ 最终要求：**全部设备质量等级必须为 Good（≥85分）**

## 4-Phase 验收结果（2026-07-20）

| Phase | 描述 | 验收标准 | 实际结果 | 状态 |
|-------|------|---------|---------|------|
| Phase 1 | 广播 WhoIs | 4/4 设备发现 | 5/5 设备发现 (含 2228320) | ✅ |
| Phase 2 | 对象扫描 | 4/4 设备成功 | 4/4 设备，每设备 11 对象 | ✅ |
| Phase 3 | 点位读取 | 全部成功，0 失败 | 102/102 任务成功 | ✅ |
| Phase 4 | 可写点写入 | 可写点 100% 成功 | 20/20 可写点成功 | ✅ |

**测试环境**: ARM64 (aarch64), Linux, 192.168.3.230
**BACnet 模拟器**: Yabe 4 设备实例 (192.168.3.115)
**依赖版本**: `github.com/anviod/bacnet@v0.0.6`

如果使用token 可以利用下面的例子 (当前token为有效token)
curl ^"http://127.0.0.1:8082/api/channels/jxy3kvpohmetzct0^" ^
  -H ^"Accept: application/json, text/plain, */*^" ^
  -H ^"Accept-Language: zh,zh-CN;q=0.9,en;q=0.8^" ^
  -H ^"Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYW1lIjoiYWRtaW4iLCJlbWFpbCI6IiIsImlzcyI6IkluZHVzdHJpYWxFZGdlR2F0ZXdheSIsImV4cCI6MTc3Mjg2NDA3NywibmJmIjoxNzcyMjU5Mjc3fQ.m0k3SQ-B9n7sfSSYnzXjT0X0Vmq_cxjqNM1jw0w03vg^" ^
  -H ^"Connection: keep-alive^" ^
  -H ^"DNT: 1^" ^
  -H ^"Referer: http://127.0.0.1:8082/^" ^
  -H ^"Sec-Fetch-Dest: empty^" ^
  -H ^"Sec-Fetch-Mode: cors^" ^
  -H ^"Sec-Fetch-Site: same-origin^" ^
  -H ^"User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36^" ^
  -H ^"sec-ch-ua: ^\^"Not:A-Brand^\^";v=^\^"99^\^", ^\^"Google Chrome^\^";v=^\^"145^\^", ^\^"Chromium^\^";v=^\^"145^\^"^" ^
  -H ^"sec-ch-ua-mobile: ?0^" ^
  -H ^"sec-ch-ua-platform: ^\^"Windows^\^"^" ^
  -H ^"sec-gpc: 1^" ^
  -H ^"token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYW1lIjoiYWRtaW4iLCJlbWFpbCI6IiIsImlzcyI6IkluZHVzdHJpYWxFZGdlR2F0ZXdheSIsImV4cCI6MTc3Mjg2NDA3NywibmJmIjoxNzcyMjU5Mjc3fQ.m0k3SQ-B9n7sfSSYnzXjT0X0Vmq_cxjqNM1jw0w03vg^"
  
---

# 《BACnet 驱动采集测试与验收标准清单》

---

# 一、点位读取（ReadProperty）验收标准

## 3.1 基础属性支持范围

### 必须支持对象

| 类型     | 必须属性                                    |
| ------ | --------------------------------------- |
| AI     | Present_Value, Units, Status_Flags      |
| AO     | Present_Value, Units, Status_Flags      |
| AV     | Present_Value, Units                    |
| BI     | Present_Value, Status_Flags             |
| BO     | Present_Value, Polarity                 |
| Device | Object_Name, Vendor_Name, System_Status |

---

## 3.2 读取性能要求

* 成功率 ≥ 99%
* 单次读取 RTT ≤ 500ms（局域网）
* 连续采集周期支持 5~30 秒
* 非法点位读取不得影响其他点位

---

## 3.3 多设备隔离要求（关键）

* 单设备异常不影响其他设备轮询
* 设备级调度线程独立
* 超时仅影响当前设备

---

# 五、COV 订阅机制验收标准

## 5.1 功能要求

* 支持 SubscribeCOV
* 支持订阅过期自动重订
* 支持设备重启后自动恢复
* 订阅失败自动回退轮询
* 轮询周期默认 10s 可配置

---

## 5.2 验收判定

* 数据变化实时上报
* 不出现重复订阅
* 网络恢复后自动重建订阅
* COV 与轮询互不干扰

---

# 六、异常与健壮性验收标准（必须达标）

## 6.1 必测异常场景

* 设备断电
* 设备重启
* 网络抖动
* UDP 丢包
* 单对象错误
* Abort / Reject 报文

---

## 6.2 判定标准

* 单设备离线不影响其他设备
* 设备进入 DEGRADED 状态后质量下降
* 恢复后自动回到 ONLINE
* 连续失败达到阈值自动冻结
* 恢复成功后自动解冻

---

# 七、质量等级验收标准（必须全部 Good）

## 7.1 设备级质量评分规则

| 指标          | 要求      |
| ----------- | ------- |
| SuccessRate | ≥ 98%   |
| TimeoutRate | ≤ 1%    |
| AvgRTT      | ≤ 200ms |
| 连续失败        | ≤ 3 次   |
| Flap        | 0       |

最终等级必须：

```
QualityScore ≥ 85
QualityLevel = Good
```

---

## 7.2 通道级要求

* 所有设备 Quality = Good
* 无设备 Offline
* 无设备 Degraded
* WorstDeviceQuality ≥ 85

---

# 八、性能与压力测试验收标准

## 8.1 基准指标

* 支持 ≥ 256 设备
* 每设备 ≥ 500 点
* 采集周期 10 秒
* 连续运行 72 小时

---

## 8.2 判定标准

* CPU 无异常飙升
* 内存无泄漏
* Goroutine 不增长
* 无死锁
* 平均延迟稳定

---

# 九、自动建模与持久化验收

## 9.1 必须支持

* 自动注册设备
* 自动生成点位模型
* 模型持久化 JSON/DB
* 重启自动恢复

---

# 十、完整验收表（Markdown 模板）

```markdown
# BACnet 驱动功能验收表

## 一、设备发现

| 项目 | 标准 | 结果 | 是否通过 |
|------|------|------|----------|
| Who-Is 广播 | 正常 |      | ☐ |
| I-Am 解析 | 100%成功 |      | ☐ |
| 自动注册 | 正常 |      | ☐ |

## 二、对象发现

| 项目 | 标准 | 结果 | 是否通过 |
|------|------|------|----------|
| objectList 完整性 | 无丢失 |      | ☐ |
| 分段支持 | 正常 |      | ☐ |

## 三、点位读取

| 设备 | SuccessRate | AvgRTT | Quality | 是否Good |
|------|------------|--------|---------|----------|
| bacnet-16 |      |        |         | ☐ |
| bacnet-17 |      |        |         | ☐ |
| bacnet-18 |      |        |         | ☐ |
| Room_FC_2014_19 |      |        |         | ☐ |

## 四、写入控制

| 测试项 | 结果 | 是否通过 |
|--------|------|----------|
| AV写入 |      | ☐ |
| BO写入 |      | ☐ |
| 优先级释放 |      | ☐ |

## 五、稳定性测试

| 项目 | 标准 | 是否通过 |
|------|------|----------|
| 72小时运行 | 无异常 | ☐ |
| 断网恢复 | 自动恢复 | ☐ |
| 单设备异常隔离 | 正常 | ☐ |
```

---

# 十一、Go 驱动自测 Checklist

* [ ] Who-Is 解析正确
* [ ] objectList 分段正确
* [ ] ReadProperty 并发安全
* [ ] WriteProperty 优先级支持
* [ ] COV 自动重订
* [ ] 设备级 metrics 正确更新
* [ ] 状态机融合质量算法
* [ ] 单设备异常不影响全局
* [ ] 自动建模恢复

---

# 十二、自动化测试结构建议

```
/test
   bacnet_discovery_test.go
   bacnet_objectlist_test.go
   bacnet_read_test.go
   bacnet_write_test.go
   bacnet_cov_test.go
   bacnet_stability_test.go
   bacnet_performance_test.go
```

每个测试应包含：

* 正常场景
* 异常场景
* 恢复场景
* 压测场景

---

# 最终验收判定条件（签字级）

✔ 设备发现率 100%
✔ 对象完整率 100%
✔ 点位成功率 ≥ 99%
✔ 全部设备 Quality ≥ Good
✔ 单设备异常不影响其他设备
✔ 72 小时稳定运行无异常
✔ 自动恢复能力验证通过

