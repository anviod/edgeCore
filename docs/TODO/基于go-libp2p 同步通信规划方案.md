---
layout: default
---

# 基于 go-libp2p 的工业边缘配置同步通信规划方案（工业级完整版）

> **文档定位：** 本文为 **多节点 libp2p 同步**规划权威版本。联机验收流程见 [联机测试方案](./联机测试方案.html)；development_plan/sync/ 目录保留短索引副本。

---

## 1. 方案概述

本方案基于 `go-libp2p` 构建一套**面向工业局域网（LAN）的去中心化配置与控制权同步系统**，专用于多台边缘网关之间的自动发现与配置一致性维护。

### 🔥 核心定位：Hybrid Sync Model（工业边缘推荐架构）

> **这是一个"分布式配置 + 控制权系统"，而不是"数据同步系统"**

```text
1️⃣ Config → CRDT-like（最终一致）
2️⃣ Ownership → Lease（强约束）  
3️⃣ Runtime → 单点主控（Owner Only）
```

**为什么这是最优选择（针对工业场景）**：

| 方案 | 结论 | 原因 |
| --- | --- | --- |
| ❌ Raft（强一致） | 不推荐 | 太重 + leader瓶颈 + ARM跑不动 |
| ❌ 全CRDT | 不推荐 | 实现复杂 + 不适合设备控制 |
| ❌ 文件同步(rsync) | 不推荐 | 无语义，必炸 |
| ✅ Hybrid（本方案） | **推荐** | 可控 + 轻量 + 工业适配 |

### ✅ 核心定位澄清

> **❌ 不是"文件镜像同步"（rsync式）**  
> **✅ 是"逻辑状态同步"（CRDT-like + Eventual Consistency）**

**关键区别**：

| 维度 | 文件镜像同步 | 逻辑状态同步（本方案） |
| --- | --- | --- |
| 同步单元 | 文件 | ConfigSnapshot（结构化对象） |
| 冲突处理 | 直接覆盖/失败 | Vector Clock + Diff |
| 一致性 | 强一致（阻塞） | 最终一致（异步） |
| 适用场景 | 备份/分发 | 分布式配置共享 |

**conf 只是存储载体，真正同步的是"结构化配置状态"**：

```text
conf/                → 文件存储
    ↓ parse
ConfigSnapshot       → 内存结构（核心同步单元）
    ↓ hash/version
SyncObject           → 网络传输单元
```

**三层一致性模型**：

| 层 | 一致性类型 | 同步方式 | 特点 |
| --- | --- | --- | --- |
| **Config** | Eventual Consistency | Announce + Pull | 最终一致、允许延迟、可冲突、低频 |
| **Ownership** | Bounded Consistency | Owner Announce + Lease 心跳 | 弱一致 + 强约束、不允许长期冲突 |
| **Runtime** | Single Writer | Owner Only Write | 去分布式、避免数据同步爆炸 |

核心特性：

* **0配置启动**（无 bootstrap / 无证书 / 无手动配置）
* **局域网自动发现 + 自动组网**
* **Hybrid Sync Model（三层模型）**
* **最终一致性（Eventual Consistency）**
* **ARMv7 友好（轻量、限流、压缩）**

适用于：

* ARMv7 边缘网关（PLC / OPC-UA / BACnet / ModbusRTU 混合）
* 工业 OT 网络环境（广播受限 / 丢包 / 延迟）
* 分布式采集 + 配置共享 + 设备接管

---

## 2. 工业层级模型（核心架构）

### ✅ 唯一正确层级（强约束）

系统 UI 必须收敛为 **单一主路径**（避免认知混乱）：

```text
Gateway（节点）
  └── Channels（通道）
        └── Devices（设备）
              └── Points（点位列表）
                    └── Point Detail（点位详情）
```

**工业语义路径对应关系**：

| 层级          | 对应 conf 文件          | 同步 Key 格式      |
| ----------- | -------------------- | ----------------- |
| Gateway     | 节点                   | 全量               |
| Channels    | channels.yaml        | channel.{id}      |
| Devices     | devices/*.yaml       | device.{id}       |
| Points      | models/*.yaml / 点位定义 | point.{id}        |
| PointDetail | 单点配置                 | point.{device}.{id}|

### ✅ UI整体结构

```text
[顶部导航]
  集群总览 | 节点管理 | 配置中心 | 接管 | 事件

[主体布局]
  ├── 左侧：节点选择 + 配置树（核心）
  └── 右侧：详情面板（动态）
```

### ✅ 配置树结构（必须严格遵循）

```text
📁 Gateway-A
   └── 📂 channels
         ├── Modbus-1
         │     └── 📂 devices
         │           ├── PLC-01
         │           │     └── 📂 points
         │           │           ├── temp
         │           │           └── pressure
         │           │
         │           └── PLC-02
         │
         └── OPCUA-1
               └── devices
```

**关键约束**：
* ❌ 不允许 "设备平铺"
* ❌ 不允许 "点位独立列表"  
* ✅ 必须走 **Channel → Device → Points**

---

## 3. 核心交互流程

### ✅ 节点选择（入口）

```text
已发现节点：

🟢 Gateway-A
🟢 Gateway-B  
🟡 Gateway-C
```

点击 Gateway-A 进入配置树浏览。

### ✅ 加载配置树（关键API）

```http
GET /api/sync/node/{id}/tree
```

返回结构：

```json
{
  "channels": [
    {
      "id": "modbus-1",
      "name": "Modbus-1",
      "protocol": "modbus",
      "status": "online",
      "deviceCount": 2,
      "hasDiff": false
    }
  ],
  "northbound": [...]
}
```

### ✅ 设备懒加载

```http
GET /api/sync/node/{id}/channel/{channelId}/devices
```

### ✅ 点位懒加载

```http
GET /api/sync/node/{id}/device/{deviceId}/points
```

### ✅ 点位详情（最终目标）

点击路径：`Gateway-A → Modbus-1 → PLC-01 → temp`

右侧展示：

```json
{
  "name": "temp",
  "address": "40001",
  "type": "float",
  "scale": 0.1,
  "unit": "°C"
}
```

---

## 4. 右侧详情面板（分层动态切换）

### ✅ Gateway层

```text
NodeID: xxx
IP: 192.168.1.100
状态: 🟢
版本: v1.2.3

操作: [同步全部] [拉取配置] [查看差异]
```

### ✅ Channel层

```text
协议: Modbus TCP
端口: 502
设备数: 5

操作: [同步该通道] [禁用]
```

### ✅ Device层

```text
设备: PLC-01
IP: 192.168.1.10
状态: 🟢
点位数: 128

操作: [同步设备] [查看差异] [锁定配置]
```

### ✅ Points列表层

```text
点位列表：
temp        float   40001
pressure    float   40002

操作: [批量同步] [对比差异]
```

### ✅ Point详情层（最细粒度）

```text
Point: temp
地址: 40001
类型: float
倍率: 0.1
单位: °C

操作: [同步此点] [推送] [回滚]
```

---

## 5. 配置来源映射（conf → UI）

```text
conf/
├── channels.yaml        → Channels
├── devices/*.yaml       → Devices  
├── models/*.yaml        → Points
├── northbound.yaml      → 挂在 Gateway 层
├── edge_rules.yaml      → 全局规则
```

### ✅ 关键设计：Points引用模型

必须支持：

```text
Device → 引用 Model → Model定义Points
```

否则扩展TSL会崩溃。

---

## 6. 配置同步粒度

| UI层级    | Sync Key 格式      | 说明         |
| ------- | ----------------- | ---------- |
| Gateway | 全量同步             | 同步所有配置     |
| Channel | channel.{id}      | 同步指定通道    |
| Device  | device.{id}       | 同步指定设备    |
| Point   | point.{device}.{id}| 同步单个点位    |

### ✅ Sync Record 示例

```json
{
  "key": "device.PLC-01.point.temp",
  "version": 12,
  "hash": "abc123..."
}
```

---

## 6.1 ConfigSnapshot 中间抽象层（核心）

### ✅ 设计原则

> **❌ 不同步文件**  
> **✅ 同步 Snapshot（结构 + hash）**

### ✅ 映射规则

```text
conf/                → 文件存储
    ↓ parse
ConfigSnapshot       → 内存结构
    ↓ hash/version
SyncObject           → 网络传输单元
```

### ✅ Go 数据结构

```go
type ConfigSnapshot struct {
    GatewayID string
    Timestamp int64

    Channels map[string]*Channel
    Devices  map[string]*Device
    Points   map[string]*Point

    // 版本控制
    Version  int64
    Hash     string  // 全量Hash

    // 分层Hash（用于局部更新）
    ChannelHashes map[string]string
    DeviceHashes  map[string]string
    PointHashes   map[string]string
}
```

### ✅ conf → Snapshot 映射

| conf 文件 | → Snapshot 字段 |
| --- | --- |
| channels.yaml | Channels |
| devices/*.yaml | Devices |
| models/*.yaml | Points |

---

## 6.2 Vector Clock 版本控制（关键）

### ❗ 问题场景

如果只有简单 version：

```text
A节点：temp.scale = 0.1（version: 12）
B节点：temp.scale = 1.0（version: 12）

→ 谁覆盖谁？（不可控）
```

### ✅ 解决方案：向量时钟

```go
type VersionVector map[string]int64  // nodeID → counter
```

```json
{
  "nodeA": 12,
  "nodeB": 9
}
```

### ✅ 合并规则

| 情况 | 结果 |
| --- | --- |
| A > B（所有维度） | 接受 A |
| B > A（所有维度） | 接受 B |
| A 和 B 冲突 | 标记 CONFLICT |

### ✅ UI 必须显示冲突状态

```text
temp ⚠️ CONFLICT
```

---

## 6.3 两阶段同步（Announce + Pull）

### ❗ 问题：全量广播不可行

```text
100节点 × 全量广播 = 网络炸
```

### ✅ 解决方案：两阶段同步

#### 阶段1：Announce（广播 Hash）

```json
{
  "type": "announce",
  "key": "device.PLC-01",
  "hash": "abc123",
  "versionVector": {"nodeA": 12, "nodeB": 9}
}
```

#### 阶段2：Pull（按需拉取）

```text
如果 hash 不同 → 请求完整数据
```

### ✅ 流程图

```text
节点A 修改配置
    ↓
计算新 Hash
    ↓
广播 Announce（只发 hash）
    ↓
节点B 发现 hash 不同
    ↓
发送 Pull 请求
    ↓
节点A 返回完整 Snapshot
    ↓
节点B 合并/冲突检测
```

---

## 6.4 Diff 引擎升级（三层）

### ✅ Diff 分层

| 层级 | 类型 | 说明 |
| --- | --- | --- |
| 结构 Diff | add/remove | 新增设备/删除点位 |
| 属性 Diff | update | 字段值变化 |
| 语义 Diff | conflict | 类型变更（破坏性） |

### ✅ 示例

**结构 Diff**：
```text
+ 新设备 PLC-03
- 删除点位 temp
```

**属性 Diff**：
```text
scale: 0.1 → 1.0
```

**语义 Diff（危险）**：
```text
40001(float) → 40001(int)

⚠️ 类型变更（破坏性）
```

### ✅ Diff 数据结构

```go
type Diff struct {
    Type   string        // add/remove/update/conflict
    Path   string        // e.g. "device.PLC-01.point.temp.scale"
    Before interface{}
    After  interface{}
    Level  string        // info/warn/critical
}
```

---

## 6.5 AccessMode 工业级接管能力模型（核心）

### ❗ 现有描述的问题

```text
RTU直连 → 不可接管
TCP/UDP网络 → 可接管
```

**维度不对！** 正确维度应该是：

> **设备访问路径是否可被多节点重入（Re-entrant Access）**

### ✅ AccessMode 定义

```go
type AccessMode string

const (
    AccessExclusive AccessMode = "exclusive" // 独占（不可接管）
    AccessShared    AccessMode = "shared"   // 共享（可接管）
    AccessLease     AccessMode = "lease"    // 租约（可接管但需锁）
)
```

### ✅ 设备分类表

| 设备类型 | AccessMode | 是否可接管 | 典型协议 |
| --- | --- | --- | --- |
| 串口直连 | exclusive | ❌ | Modbus RTU / IO / DIDO |
| 单连接TCP | lease | ⚠️（需抢占） | PLC / 私有TCP |
| 多客户端协议 | shared | ✅ | OPC UA / BACnet / MQTT |

### ✅ 协议级细化（关键！）

| 协议 | AccessMode | 原因 |
| --- | --- | --- |
| Modbus RTU | exclusive | 串口独占，无法多连接 |
| Modbus TCP | lease | 通常只允许 1~2 个连接 |
| OPC UA | shared | 天然支持多客户端 |
| BACnet/IP | shared | 支持多客户端 + 广播（需限流） |
| 私有TCP协议 | lease | 单连接，后连踢前连 |
| MQTT | shared | 发布订阅模式天然共享 |

### ❗ 反例警示

```text
Modbus TCP（很多PLC）
只能允许 1~2 个连接

→ 不是 shared，而是 lease
→ 如果多节点同时接入会直接冲突
```

### ✅ 设备访问模式推断（建议实现）

```go
func InferAccessMode(protocol string) AccessMode {
    switch protocol {
    case "modbus-rtu", "io", "dido":
        return AccessExclusive
    case "modbus-tcp":
        return AccessLease
    case "opcua", "bacnet", "mqtt":
        return AccessShared
    case "tcp", "私有协议":
        return AccessLease
    default:
        return AccessLease  // 保守默认值
    }
}
```

---

## 6.6 Takeover Lease 租约机制

### ❗ 问题：多节点同时接管

```text
A 接管 PLC-01
B 也接管 PLC-01
→ 冲突
```

### ✅ 解决方案：Lease 租约（仅用于 AccessLease 设备）

```go
type Lease struct {
    DeviceID string
    Owner    string    // GatewayID
    ExpireAt time.Time
    Version  int64
}
```

### ✅ 状态机升级（按 AccessMode 区分）

**Exclusive 设备**：
```text
OFFLINE → DETECTED → (禁止接管) → OFFLINE
```

**Shared 设备**：
```text
OFFLINE → DETECTED → TAKEOVER → RUNNING
```

**Lease 设备**：
```text
OFFLINE
    → DETECTED
    → TRY_LEASE        ⭐ 尝试获取租约
    → LEASE_GRANTED    ⭐ 租约获批
    → TAKEOVER
    → CONFIG_APPLIED
    → RUNNING
```

### ✅ Lease 续约

```text
持有者必须在 LeaseTTL 内续约
否则其他节点可以抢占到 Lease
```

---

## 7. Diff UI（嵌入右侧）

### ✅ 点位级差异示例

```text
temp

本地：
scale: 0.1

远程：
scale: 1.0 ⚠️

操作: [同步到本地] [推送远程]
```

---

## 8. 设备接管 UI（嵌入 Device层）

### ✅ 设备信息展示（含 AccessMode）

```text
设备: PLC-01
IP: 192.168.1.10
协议: Modbus TCP

访问模式: 🔁 租约（Lease）

当前控制:
  Gateway-A

状态:
  Lease 剩余 18s
```

### ✅ 三种 AccessMode 的 UI 展示

**Exclusive 设备**：
```text
设备: IO-Module-1
协议: Modbus RTU

访问模式: 🔒 独占（Exclusive）

状态: 本地独占设备

[不可接管]（按钮禁用）
```

**Shared 设备**：
```text
设备: OPC-UA-Server-1
协议: OPC UA

访问模式: 🌐 共享（Shared）

当前控制: Gateway-A（逻辑归属）

[直接接管]（无确认弹窗）
```

**Lease 设备**：
```text
设备: PLC-01
协议: Modbus TCP

访问模式: 🔁 租约（Lease）

当前控制: Gateway-B
Lease 剩余: 45s

[接管]（弹确认：会踢掉对方）
```

### ✅ 接管按钮行为（按 AccessMode 区分）

| 设备类型 | 按钮状态 | 行为 |
| --- | --- | --- |
| exclusive | ❌ 禁用 | 不允许接管 |
| shared | ✅ 直接接管 | 无确认弹窗 |
| lease | ⚠️ 弹确认 | 显示"会踢掉对方"警告 |

### ✅ 接管状态流程

```text
Exclusive:
OFFLINE → DETECTED → (禁止接管)

Shared:
OFFLINE → DETECTED → TAKEOVER → RUNNING

Lease:
OFFLINE → DETECTED → TRY_LEASE → LEASE_GRANTED → TAKEOVER → RUNNING
```

### ✅ 设备详情数据结构

```go
type DeviceDetail struct {
    ID          string
    Name        string
    Protocol    string
    AccessMode  AccessMode  // exclusive / shared / lease
    Owner       string      // 当前控制者
    LeaseTTL    int         // Lease 剩余秒数（仅 lease 设备）
}
```

---

## 8.1 设备配置层（devices/*.yaml）

### ✅ 必须增加的 access 配置

```yaml
# devices/plc-01.yaml
name: PLC-01
protocol: modbus-tcp
address: 192.168.1.10
port: 502

access:
  mode: lease        # exclusive / shared / lease
  timeout: 30s       # lease 专用租约超时
```

### ✅ 按协议自动推断

系统可以根据协议类型自动填入默认 access 配置：

```go
var defaultAccessModes = map[string]AccessMode{
    "modbus-rtu": AccessExclusive,
    "io":         AccessExclusive,
    "dido":       AccessExclusive,
    "modbus-tcp": AccessLease,
    "opcua":      AccessShared,
    "bacnet":     AccessShared,
    "mqtt":       AccessShared,
}
```

### ✅ 配置示例

**Exclusive 设备**：
```yaml
access:
  mode: exclusive
```

**Shared 设备**：
```yaml
access:
  mode: shared
```

**Lease 设备**：
```yaml
access:
  mode: lease
  timeout: 30s  # 租约超时，建议 30s~60s
```

---

## 8.2 调度器感知

### ❗ 问题场景

```text
A 节点正在采集 PLC-01
B 节点接管了 PLC-01
→ 双采集冲突
```

### ✅ 采集调度必须检查 Owner

```go
func (s *Scheduler) ScheduleCollect(deviceID string) {
    // 如果不是当前节点 owner，暂停采集
    if !s.IsOwner(deviceID) {
        s.pauseCollect(deviceID)
        return
    }

    // 是 owner，开始采集
    s.startCollect(deviceID)
}
```

### ✅ Lease 续约期间

```go
func (s *Scheduler) OnLeaseRenewed(deviceID string) {
    // Lease 续约成功，继续采集
    s.resumeCollect(deviceID)
}

func (s *Scheduler) OnLeaseExpired(deviceID string) {
    // Lease 过期，暂停采集
    s.pauseCollect(deviceID)
}
```

---

## 8.3 同步系统过滤规则

### ❗ 问题场景

```text
节点同步配置
→ 错误接管 RTU 设备
→ 导致本地采集中断
```

### ✅ Sync Takeover 过滤

```go
func (sm *SyncManager) CanTakeover(deviceID string) (bool, string) {
    device := sm.getDevice(deviceID)

    switch device.AccessMode {
    case AccessExclusive:
        return false, "独占设备不允许接管"

    case AccessShared:
        return true, "共享设备可直接接管"

    case AccessLease:
        // 检查当前 Lease 状态
        if sm.HasActiveLease(deviceID) {
            owner := sm.GetLeaseOwner(deviceID)
            if owner == sm.localNodeID {
                return false, "当前节点已持有租约"
            }
            return true, "需要抢占租约"
        }
        return true, "可获取租约"
    }

    return false, "未知访问模式"
}
```

### ✅ 配置同步时的设备过滤

```go
func (sm *SyncManager) SyncDeviceConfig(deviceID string) error {
    // Exclusive 设备不参与 takeover sync
    if device.AccessMode == AccessExclusive {
        return nil  // 跳过
    }

    // Shared / Lease 设备正常处理
    return sm.doSyncDevice(deviceID)
}
```

---

## 9. 事件流（联动层级）

### ✅ 事件展示示例

点击某个点位时：

```text
10:01 temp 被修改
10:02 同步到 Gateway-B
10:03 生效
```

---

## 10. 前端实现（Vue结构）

### ✅ 目录结构

```text
views/
└── sync/
    ├── Cluster.vue          # 集群总览
    ├── NodeTree.vue         # ⭐核心配置树组件
    ├── NodeDetail.vue       # 节点详情
    ├── ChannelDetail.vue    # 通道详情
    ├── DeviceDetail.vue     # 设备详情
    ├── PointList.vue        # 点位列表
    ├── PointDetail.vue      # 点位详情
    └── ConfigDiff.vue       # 配置差异
```

### ✅ Pinia Store 拆分

```ts
clusterStore   // 节点列表
treeStore      // 树结构（核心）
configStore    // 配置内容
diffStore      // 差异对比
eventStore     // 事件流
```

### ✅ TreeNode 接口定义

```ts
interface TreeNode {
  type: 'gateway' | 'channel' | 'device' | 'point'
  id: string
  label: string
  status: 'online' | 'offline' | 'degraded' | 'warning'
  hasDiff?: boolean
  children?: TreeNode[]
}
```

### ✅ 侧边栏底部版本信息展示（关键）

版本信息通过 goreleaser ldflags 注入二进制，由后端 API 暴露，前端在侧边栏底部实时展示：

**后端注入机制**：
```go
// internal/model/buildinfo.go
var (
    Version   = "dev"      // goreleaser: {{.Version}}
    BuildTime = "unknown"  // goreleaser: {{.Date}}
    CommitID  = "unknown"  // goreleaser: {{.ShortCommit}}
)
```

**API 暴露**：
```http
GET /api/auth/system-info
```
```json
{
  "code": "0",
  "data": {
    "name": "NODE-LAPTOP-5E3D21EG",
    "softVer": "0.0.4",
    "buildTime": "2026-06-05T14:23:00Z",
    "commitID": "efa219a"
  }
}
```

**UI 侧边栏底部展示**：
```text
┌──────────────────────────┐
│ ● v0.0.4                │  ← 运行状态指示 + 版本号
│                          │
│ Build  2026-06-05 14:23 │  ← 构建时间
│ Commit efa219a           │  ← Git 短哈希
│                          │
│ [◀ 收起]                 │
└──────────────────────────┘
```

**goreleaser 构建指令**：
```bash
goreleaser release --snapshot --clean --config .goreleaser.yml
```

ldflags 配置（`.goreleaser.yml`）：
```yaml
ldflags: >
  -s -w
  -X github.com/anviod/edgeCore/internal/model.Version={{.Version}}
  -X 'github.com/anviod/edgeCore/internal/model.BuildTime={{.Date}}'
  -X github.com/anviod/edgeCore/internal/model.CommitID={{.ShortCommit}}
```

**输出产物（自动多平台）**：
```
dist/
├── edgeCore-v0.0.4-arm64.deb          # 远程 ARM64 节点安装包
├── edgeCore-v0.0.4-amd64.deb          # AMD64 安装包
├── edgeCore-0.0.4-windows-amd64.tar.gz # Windows x64
├── edgeCore-0.0.4-linux-arm64.tar.gz   # Linux ARM64
└── SHA256SUMS
```

### ✅ 远程部署流程（goreleaser + deploy-remote.sh）

**一键远程部署**：
```bash
# 1. 构建所有平台安装包
goreleaser release --snapshot --clean --config .goreleaser.yml

# 2. 一键部署到 ARM64 远程节点（root@192.168.3.230）
bash scripts/deploy-remote.sh root@192.168.3.230 NODE-HNE_GATEWAY

# 3. 验证远程节点服务
ssh root@192.168.3.230 "systemctl status edgeCore --no-pager"
ssh root@192.168.3.230 "curl -s http://localhost:8082/api/auth/system-info"
```

**deploy-remote.sh 自动化流程**：
```
1. SSH连接验证 ──► 架构自动检测(aarch64→arm64)
2. 自动查找匹配的 .deb 包
3. scp 传输 ──► 配置备份 ──► 停止旧服务
4. dpkg安装 ──► 配置节点名 ──► systemctl 启动
5. 验证服务状态
```

**本机 - 远程双节点测试架构**：
```
┌──────────────────────┐         ┌──────────────────────┐
│ 本机 (Windows/Linux) │  libp2p │ 远程 (root@192.168.3.230) │
│                      │◄───────►│ ARM64 Linux           │
│ goreleaser 构建      │  :4001  │ systemctl edgeCore       │
│ 本地 run/debug       │         │ .deb 安装部署          │
└──────────────────────┘         └──────────────────────┘
```

---

### 10.1 联机测试方案对齐

本文档所有设计必须与 [联机测试方案](./联机测试方案.html) 中的 35 个测试用例严格对齐。

**方案文档 vs 测试用例映射**：

| 方案章节 | 对应测试用例 | 验证内容 |
|---------|-------------|----------|
| §3 核心交互流程 (NodeTree + 懒加载) | TC-27~TC-29, TC-32, TC-33 | UI 四级树结构、懒加载、Diff UI |
| §4 右侧详情面板 | TC-31 | 接管按钮按 AccessMode 分叉 |
| §6 配置同步粒度 | TC-04~TC-06 | 全量/增量/点位级同步 |
| §6.2 Vector Clock | TC-14, TC-15 | 版本比较、冲突检测 |
| §6.3 两阶段同步 (Announce+Pull) | TC-04~TC-06 | Announce 广播+Pull 拉取 |
| §6.4 Diff 引擎 | TC-18~TC-20 | 结构/属性/语义三层 Diff |
| §6.5 AccessMode | TC-07~TC-09 | exclusive/shared/lease 三态接管 |
| §6.6 Takeover Lease | TC-09, TC-10 | 租约抢占、续约、过期 |
| §7 Diff UI | TC-29 | Diff 对比展示 |
| §8 设备接管 UI | TC-31 | AccessMode 分叉按钮行为 |
| §8.2 调度器感知 | TC-09, TC-10 | 采集暂停/恢复 |
| §8.3 同步系统过滤 | TC-07 | Exclusive 设备跳过 |
| §10 版本信息展示 | TC-30 | sidebar-footer 版本号/构建时间/Commit |
| §10 远程部署 | 第4节部署流程 | goreleaser 构建 + .deb 部署 |
| §12 后端 API | TC-25, TC-26 | 所有权/租约 API |
| §18.1 libp2p 优化 | TC-01, TC-02 | 节点发现 (mDNS/静态) |
| §18.4 最终定位修正 | TC-17 | 一致性检查 |

**测试覆盖完整性**：
- P0 测试用例：15 项（节点发现/同步/控制权/设备迁移/一致性）
- P1 测试用例：20 项（版本管理/Diff/错误处理/UI/并发）
- 异常测试：8 项（断电/网络分区/崩溃/磁盘满等）
- 边界测试：6 项（空配置/极长名称等）
- 性能测试：6 项基准 + 3 项压力

**双节点测试拓扑**：
```
本机 (NODE-1):  go run cmd/main.go        ← API: localhost:8082
远程 (NODE-REMOTE): systemctl edgeCore        ← API: 192.168.3.230:8082
              ↕ libp2p:4001 (mDNS + 静态种子)
```

---

## 11. 关键优化（工业级）

### ✅ 懒加载（必须）

```text
点击 Channel 才加载 Devices
点击 Device 才加载 Points
```

防止：300设备 × 100点 = 性能问题

### ✅ 状态叠加（必须）

```text
PLC-01   🟢
PLC-02   🔴
```

点位警告：
```text
temp   ⚠️
```

### ✅ 一致性标记

```text
PLC-01   🟢一致
PLC-02   ⚠️差异
```

---

## 12. 后端 API 设计

### ✅ 核心接口

| API 路径                          | 方法 | 说明              |
| ------------------------------- | ---- | --------------- |
| /api/sync/node/{id}/tree        | GET  | 获取配置树结构        |
| /api/sync/node/{id}/devices     | GET  | 获取设备列表（懒加载）   |
| /api/sync/node/{id}/points      | GET  | 获取点位列表（懒加载）   |
| /api/sync/node/{id}/diff        | GET  | 获取差异列表         |
| /api/sync/node/{id}/sync        | POST | 触发同步           |
| /api/sync/node/{id}/takeover    | POST | 触发设备接管         |

### ✅ 响应格式规范

```json
{
  "code": 0,
  "message": "success",
  "data": {},
  "timestamp": 1710000000
}
```

---

## 13. 本质总结

### ✅ 系统定位

这套 UI 最终不是：

❌ "文件浏览器"
❌ "配置管理工具"

而是：

> **🔥 工业边缘配置控制平面（Config Control Plane）**

### ✅ 核心能力

```text
1️⃣ 可视化配置拓扑（树）
2️⃣ 精确到点位的控制
3️⃣ 跨节点同步
4️⃣ 可解释（事件流）
5️⃣ 可控（接管 + diff）
```

---

## 14. 落地优先级

| 优先级 | 任务 | 说明 | 对应测试 |
| --- | --- | --- | --- |
| P0 | AccessMode 定义与数据结构 | exclusive/shared/lease 三态 | TC-07~TC-09 |
| P0 | devices/*.yaml access 配置 | mode + timeout 字段 | TC-11, TC-12 |
| P0 | Takeover 状态机升级 | 按 AccessMode 分支 | TC-08~TC-10 |
| P0 | Sync Takeover 过滤规则 | Exclusive 设备跳过 | TC-07 |
| P0 | Scheduler Owner 检查 | IsOwner + 采集暂停 | TC-09, TC-10 |
| P0 | 全量/增量/点位同步 | 两阶段 Announce+Pull | TC-04~TC-06 |
| P0 | 节点发现 (mDNS + 静态) | Discovery 多级 Fallback | TC-01, TC-02 |
| P0 | goreleaser 构建 + 远程部署 | 多平台 .deb + deploy-remote.sh | [联机测试 §3-4] |
| P0 | UI 版本信息展示 | sidebar-footer Build/Commit | TC-30 |
| P1 | UI 按 AccessMode 分叉 | 禁用/直接/弹确认 | TC-31 |
| P1 | Lease 续约机制 | 定时续约 + 过期检测 | TC-10 |
| P1 | Protocol → AccessMode 推断 | 自动填充默认值 | TC-11 |
| P1 | Vector Clock 版本控制 | 版本比较 + 冲突检测 | TC-14, TC-15 |
| P1 | 三层 Diff 引擎 | 结构/属性/语义 Diff | TC-18~TC-20 |
| P1 | 配置树 UI + 懒加载 | GATEWAY→Channel→Device→Point | TC-27/28/32/33 |
| P1 | 错误处理与故障恢复 | 离线检测/断点续传/重试 | TC-21~TC-24 |
| P2 | Event Log 集成 | 事件流展示 | — |
| P2 | 并发安全测试 | 多节点并发修改/接管 | TC-34, TC-35 |

---

## 15. 技术栈要求

### ✅ 前端
- Vue 3 + Composition API
- Pinia 状态管理
- Vue Router 路由
- Ant Design Vue 组件库

### ✅ 后端
- Go 1.21+
- go-libp2p
- YAML 解析（gopkg.in/yaml.v3）
- BadgerDB 持久化（可选）

---

## 16. 安全保障

### ✅ 安全模型

| 层级   | 实现    | 说明       |
| :--- | :---- | -------- |
| 节点认证 | PSK   | 局域网轻量安全 |
| 加密   | Noise | 端到端加密   |
| 完整性  | Hash  | 内容校验     |
| 网络隔离 | 局域网   | 物理隔离     |

### ✅ 配置示例

```yaml
security:
  mode: lan
  psk: auto-generate
```

---

## 17. 部署与运维（0配置 + goreleaser）

### ✅ 配置示例（极简）

```yaml
sync:
  enable: true
  mode: lan
```

无需：bootstrap、peer配置、证书

### ✅ 完整编译部署流程

详见 [联机测试方案 §3-4](./联机测试方案.html#3-编译与构建)，摘要：

```bash
# 本地开发
go run cmd/main.go

# 生产构建（多平台）
goreleaser release --snapshot --clean --config .goreleaser.yml

# 一键远程部署（ARM64）
bash scripts/deploy-remote.sh root@192.168.3.230 NODE-REMOTE
```

**部署架构**：

```
┌──────────────────────┐    goreleaser 构建    ┌────────────────────────┐
│ 本机 (开发机)         │ ──────────────────► │ 远程 root@192.168.3.230 │
│                      │     .deb + scp       │ ARM64 Linux            │
│ go run cmd/main.go   │                      │ systemctl edgeCore        │
│ goreleaser build     │ ◄──── libp2p ──────► │ .deb 安装部署          │
│ API: :8082           │       :4001          │ API: :8082             │
└──────────────────────┘                      └────────────────────────┘
```

### ✅ 运维命令

```bash
gateway-cli sync peers      # 查看节点列表
gateway-cli sync status     # 查看同步状态
gateway-cli sync diff       # 查看配置差异
```

### ✅ 远程服务管理

```bash
# 远程状态
ssh root@192.168.3.230 "systemctl status edgeCore --no-pager"

# 远程日志
ssh root@192.168.3.230 "journalctl -u edgeCore -n 100 --no-pager"

# 远程重启
ssh root@192.168.3.230 "systemctl restart edgeCore"
```

---

## 18. 代码安全性

### ✅ 风险缓解

| 风险     | 措施   |
| ------ | ---- |
| 广播风暴   | 限流 + Gossip控制 |
| 配置错误传播 | 版本控制 + 灰度发布 |
| 非授权节点  | PSK认证 + 角色控制 |

---

## 18.1 libp2p 层工业级优化

### ❗ 问题清单

| 问题 | 影响 |
| --- | --- |
| mDNS 在工厂被禁 | 节点发现失败 |
| UDP 广播受限 | 组网不稳定 |
| Gossip 风暴 | 网络阻塞 |

### ✅ Discovery 多级 Fallback

```text
Discovery 优先级：
1. mDNS（优先，局域网内）
2. UDP Broadcast（可选，企业网）
3. 静态种子节点（最后兜底）
```

### ✅ Topic 必须拆分（禁止全量混用）

```text
/config/announce    → 配置变更通知（只发 Hash）
/config/request     → 拉取请求
/config/fullsync   → 全量同步响应
/config/diff        → 差异推送
/takeover          → 设备接管相关
/hello             → 节点心跳/握手
```

### ✅ Gossip 限流配置

```go
pubsub.WithPeerOutboundQueueSize(32)    // 出队缓冲
pubsub.WithValidateQueueSize(64)         // 验证队列
pubsub.WithMaxMessageSize(64 * 1024)    // 消息大小限制
pubsub.WithStrictSignatureVerification(true)  // 签名校验
```

### ✅ 心跳保活

```go
// 节点超时配置
connectionManager.WithPeerTimeout(30 * time.Second)
keepalive.WithPeriod(15 * time.Second)
```

---

## 18.2 性能优化（大规模节点）

### ❗ 规模问题

```text
100网关 × 200设备 × 200点位 = 4,000,000 点
```

### ✅ 分层 Hash（局部更新）

```text
Gateway Hash
├── Channel Hash
│   ├── Device Hash
│   │   ├── Point Hash
```

当 only `device.PLC-01` 变化时：
- 重新计算 `Device Hash`
- 重新计算 `Channel Hash`
- 重新计算 `Gateway Hash`
- **只广播 Announce**（hash 变化）

### ✅ Debounce 防抖

```go
// 500ms 内多次修改合并一次同步
debounceInterval := 500 * time.Millisecond
```

### ✅ Snapshot 分片（可选，大规模时启用）

```text
按 Channel 分片同步
避免全量 Gateway Snapshot 过大
```

### ✅ 压缩（可选）

```go
// Snappy 压缩（网络传输时）
snappy.Encode(snapshot)
```

---

## 18.3 conf 镜像目录结构

### ✅ 正确结构

```text
data/
└── edgeCore/
    ├── local/              # 本地节点配置（可写）
    │   └── conf/
    ├── peers/             # 远端节点配置镜像（只读）
    │   ├── NODE-A/
    │   │   └── conf/
    │   └── NODE-B/
    │       └── conf/
    └── wal/               # Write-Ahead Log
```

### ⚠️ 关键约束

> **peers 目录必须只读**，否则会污染远端配置

---

# ✅ 最终架构图（v4.0 工业级完整版）

```text
┌─────────────────────────────────────────────────────────────────────────────────┐
│                    Distributed OT Config Sync System (v4.0)                    │
│         分布式 OT 配置状态同步系统（CRDT-like + AccessMode + Eventual）        │
├─────────────────────────────────────────────────────────────────────────────────┤
│  UI Layer                                                                         │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐           │
│  │ NodeTree    │  │ ConfigDiff │  │ Takeover   │  │ EventLog    │           │
│  │ (懒加载树)   │  │ (三层Diff)  │  │(AccessMode)│  │ (事件流)    │           │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘           │
├─────────┼────────────────┼────────────────┼────────────────┼───────────────────┤
│  Store Layer                                                                      │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐           │
│  │ treeStore   │  │ configStore │  │ diffStore   │  │ leaseStore  │           │
│  │             │  │ (Snapshot)  │  │ (三层Diff)  │  │ (租约管理)   │           │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘           │
├─────────┼────────────────┼────────────────┼────────────────┼───────────────────┤
│  API Layer                                                                        │
│  ┌─────────────────────────────────────────────────────────────────────────┐     │
│  │ /api/sync/node/{id}/tree  /diff  /sync  /takeover                    │     │
│  └──────┬───────────────────────────────────────────────────────────────┘     │
├─────────┼─────────────────────────────────────────────────────────────────────┤
│  Sync Engine (go-libp2p)                                                       │
│                                                                                 │
│  ┌─────────────────────────────────────────────────────────────────────────┐     │
│  │ AccessMode Filter                                                        │     │
│  │ exclusive → skip | shared → direct | lease → try_lease                 │     │
│  └──────┬───────────────────────────────────────────────────────────────┘     │
│                                                                                 │
│  ┌─────────────────────────────────────────────────────────────────────────┐     │
│  │ Discovery（多级 Fallback）                                               │     │
│  │ 1. mDNS → 2. UDP Broadcast → 3. 静态种子                              │     │
│  └──────┬───────────────────────────────────────────────────────────────┘     │
│                                                                                 │
│  ┌─────────────────────────────────────────────────────────────────────────┐     │
│  │ PubSub Topics（严格拆分）                                               │     │
│  │ /config/announce | /config/request | /config/fullsync                  │     │
│  │ /config/diff | /takeover | /hello                                      │     │
│  └──────┬───────────────────────────────────────────────────────────────┘     │
│                                                                                 │
│  ┌─────────────────────────────────────────────────────────────────────────┐     │
│  │ Two-Phase Sync                                                          │     │
│  │ Phase1: Announce（Hash only）→ Phase2: Pull（按需拉取）                 │     │
│  └──────┬───────────────────────────────────────────────────────────────┘     │
│                                                                                 │
│  ┌─────────────────────────────────────────────────────────────────────────┐     │
│  │ Version Control                                                         │     │
│  │ Vector Clock + Diff 三层（结构/属性/语义）                               │     │
│  └─────────────────────────────────────────────────────────────────────────┘     │
├─────────────────────────────────────────────────────────────────────────────────┤
│  Scheduler Layer                                                                 │
│  ┌─────────────────────────────────────────────────────────────────────────┐     │
│  │ Owner-aware Scheduler                                                   │     │
│  │ IsOwner() → pause/resume collect | Lease renewal → collect control     │     │
│  └─────────────────────────────────────────────────────────────────────────┘     │
├─────────────────────────────────────────────────────────────────────────────────┤
│  Storage Layer                                                                   │
│  ┌───────────────────────────────────────────────────────────────────────┐       │
│  │ ConfigStore                                                             │       │
│  │  ├── ConfigSnapshot（内存结构）                                          │       │
│  │  ├── WAL（Write-Ahead Log）                                             │       │
│  │  └── BadgerDB（持久化）                                                 │       │
│  │                                                                          │       │
│  │ peers/（只读镜像）                                                       │       │
│  └───────────────────────────────────────────────────────────────────────┘       │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## 18.4 最终定位修正

### ❌ 之前定位

> Config Control Plane（配置控制平面）

### ✅ 正确工业级定位

> **分布式 OT 配置状态同步系统（Hybrid Sync Model + CRDT-like + Eventual Consistency + Edge Gossip Bus）**

---

## 18.5 设备接管工业级定义

> **设备接管（Takeover）并非简单的"连接切换"，而是基于设备访问模式（AccessMode）的控制权转移机制。**

系统将设备划分为三类：

| 类别 | AccessMode | 接管行为 | 典型协议 |
| --- | --- | --- | --- |
| 独占设备 | exclusive | ❌ 不允许接管 | Modbus RTU / IO / DIDO |
| 共享设备 | shared | ✅ 直接接管（逻辑归属） | OPC UA / BACnet / MQTT |
| 租约设备 | lease | ⚠️ 抢占租约 | Modbus TCP / 私有TCP |

---

## 18.6 ARMv7 优化专题（工业级关键）

### ✅ 针对 ARMv7 的核心优化

| 优化项 | 实现方式 | 目的 |
| --- | --- | --- |
| **分层 Hash** | Gateway → Channel → Device → Point | 增量同步，减少数据量 |
| **二阶段同步** | Announce(hash) → Pull(data) | 避免全量广播 |
| **消息压缩** | Snappy（优先）> Gzip | 减少网络传输，ARM友好 |
| **流量限流** | 100 msg/sec | 保护弱网络 |
| **Debounce** | 500ms 合并多次修改 | 减少同步频率 |

### ✅ 分层 Hash 结构

```text
GatewayHash
    ├── ChannelHash (channel.modbus-1)
    │   ├── DeviceHash (device.PLC-01)
    │   │   ├── PointHash (point.temp)
    │   │   └── PointHash (point.pressure)
    │   └── DeviceHash (device.PLC-02)
    └── ChannelHash (channel.opcua-1)
        └── DeviceHash (device.OPC-Server-01)
```

### ✅ 二阶段同步流程

```text
节点A 修改配置
    ↓
计算新的 PointHash → DeviceHash → ChannelHash → GatewayHash
    ↓
广播 Announce（只发 hash，几十字节）
    ↓
节点B 比对本地 Hash
    ↓
如果不同 → 发送 Pull 请求（按需）
    ↓
节点A 返回变化部分的完整数据
    ↓
节点B 合并更新
```

### ✅ 明确不建议的方案

| 方案 | 问题 |
| --- | --- |
| ❌ 全 Raft | Leader 挂了影响全局，网络抖动触发选举风暴，ARM 跑不动 |
| ❌ 全量文件同步 | 无语义，冲突不可控 |
| ❌ 全量广播同步 | 网络炸，CPU 炸 |

---

## 18.7 最终推荐架构

```text
Sync Engine（最终形态）

1. Discovery（mDNS + UDP Broadcast + 静态种子）
2. Gossip（轻量广播，限流）
3. Snapshot Sync（配置层，Announce + Pull）
4. Ownership Sync（控制权，Owner Announce）
5. Lease Manager（租约管理，TTL + 心跳）
6. Runtime Isolation（运行隔离，Owner Only Write）
```

---

## 18.8 P0 实现顺序（强建议）

| 顺序 | 任务 | 说明 | 验证方式 |
| --- | --- | --- | --- |
| 1 | Snapshot + Hash | 配置层核心 | TC-04 全量同步 |
| 2 | Gossip Announce | 轻量广播 | TC-01 节点发现 |
| 3 | Pull Sync | 按需拉取 | TC-05 增量同步 |
| 4 | Ownership + Lease | 控制权管理 | TC-07~TC-10 |
| 5 | Scheduler 接入 | 停止非 Owner 采集 | TC-09, TC-10 |
| 6 | goreleaser + 远程部署 | 构建 + .deb 部署 | [联机测试 §3-4] |

**联机测试入口**：完成每个步骤后，执行 [联机测试方案](./联机测试方案.html) 中对应 Phase 的测试用例进行验证。

---

**版本**: v3.0（工业级完整版 + 版本注入 + 远程部署 + 联机测试方案）  
**日期**: 2026-06-05  
**状态**: ✅ 工业级可运行模型完成，版本信息已实现 UI 底部展示，配套完整的 35 项联机测试方案  
**核心特性**: Hybrid Sync Model + ARMv7 优化 + 三层一致性 + goreleaser 多平台构建 + 一键远程部署 + 联机测试全覆盖  
**配套文档**: [联机测试方案](./联机测试方案.html) — 编译部署 + 功能测试覆盖 + 35个测试用例