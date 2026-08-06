---
layout: default
title: 边缘网关用户手册
description: edgeCore 用户手册
---

# 边缘网关用户手册

> **文档定位：** 本手册侧重**协议说明、安装部署、操作步骤与最佳实践**。产品宣传见 [产品手册](PRODUCT.zh-CN.html) / [产品说明](产品说明.html)；架构与热路径见 [边缘网关架构设计总览](../edge/边缘网关架构设计总览.html)。  
> **English：** [USER_MANUAL.en.html](USER_MANUAL.en.html)

## 目录

1. [简介](#简介)
2. [系统架构](#系统架构)
3. [安装指南](#安装指南)
4. [部署流程](#部署流程)
5. [使用方式](#使用方式)
6. [南向采集](#南向采集)
7. [北向数据共享](#北向数据共享)
8. [边缘计算](#边缘计算)
9. [系统管理](#系统管理)
10. [最佳实践](#最佳实践)
11. [故障排查](#故障排查)
12. [附录](#附录)

---

## 简介

边缘网关是一个集成了数据采集、边缘计算、数据转发等功能的边缘设备管理平台。本用户手册旨在帮助用户快速了解和使用边缘网关的各项功能。

**数据主线：** 南向驱动采集 → **ShadowCore 影子设备（运行时真源）** → UI 实时展示 / 虚拟影子派生 / 边缘规则 / 历史落库 / 北向上报。配置落在 `config.db`，影子本身为内存快照，进程重启后由采集自动回填。

<div align="center">
  <img src="../img/edge_core_v2.4.svg" width="100%" />
  <p><small>图 1: 边缘计算数据流程图</small></p>
</div>

### 主要功能

- **多协议数据采集**：支持 Modbus、BACnet、OPC UA、S7、EtherNet/IP、FINS、SNMP、IEC 104、DL/T645、Mitsubishi MC、Profinet IO、KNXnet/IP、EtherCAT 等工业协议（详见 [南向驱动矩阵](../drivers/index.html)）
- **影子数据面**：真实影子 + 虚拟影子，UI / 规则 / 北向统一读路径
- **边缘计算**：支持阈值告警、状态管理、窗口计算等边缘智能分析
- **数据转发**：支持 MQTT、Sparkplug B、OPC UA Server、HTTP、EdgeOS 等北向方式
- **设备管理**：支持设备的添加、编辑、删除和监控
- **系统监控**：提供系统运行状态、资源使用情况与 ScanEngine SLA 诊断
- **高可用**：连接状态机、指数退避、冷却期、采集健康检测

---

## 系统架构

> 完整分层架构、数据流与 ScanEngine 调度内核见 [产品说明 · 系统架构](产品说明.html#系统架构) 与 [边缘网关架构设计总览](../edge/边缘网关架构设计总览.html)。

### 连接管理架构

所有驱动统一使用公共连接管理器 (ConnectionManager)，具备以下能力：

- **状态机管理**：Disconnected → Connecting → Connected → Retrying → Dead
- **指数退避**：失败重连采用指数退避算法，避免设备过载
- **冷却期策略**：连续失败后进入冷却期，防止持续打设备
- **采集健康检测**：通过采集成功/失败判断连接健康状态
- **每日清零**：每日零点自动重置重试计数

---

## 安装指南

### 系统要求

#### 硬件要求

| 配置项 | 最低配置 | 推荐配置 |
| :--- | :--- | :--- |
| **CPU** | 1GHz 单核 | 双核 1.5GHz+ |
| **内存** | 128MB | 512MB+ |
| **存储** | 1GB | 4GB+ |
| **网络** | 10/100Mbps 以太网 | 100/1000Mbps 以太网 |
| **串口** | 可选（RTU 协议需要） | 可选 |

#### 软件要求

| 项目 | 要求 |
| :--- | :--- |
| **操作系统** | Linux (Ubuntu 20.04+、Debian 10+、CentOS 7+)、Windows 10+ |
| **架构** | x86_64、ARMv7、ARM64 |
| **浏览器** | Chrome 80+、Firefox 75+、Safari 13+、Edge 80+ |

### 安装方式

从 [GitHub Releases](https://github.com/anviod/edgeCore/releases) 下载对应架构的安装包（`amd64` / `arm64` / `arm`）。生产环境 Linux 推荐使用 **deb / rpm 系统包**（自动注册 systemd、升级时保留配置）。

#### 方式一：系统包安装（Linux，推荐）

包内安装路径为 `/usr/local/bin/edgeCore/`，注册 systemd 服务 `edgeCore`；升级时 `preinstall` / `postinstall` 脚本会自动备份并恢复 `config/` 与 `data/`。

文件名形如 `edgeCore-v{version}-{arch}.deb` 或 `edgeCore-v{version}-{arch}.rpm`。

**Debian / Ubuntu（`.deb`）**

首次安装：

```bash
sudo dpkg -i edgeCore-v{version}-amd64.deb
sudo apt-get install -f -y    # 若提示依赖缺失
```

升级（覆盖安装，保留配置，服务自动重启）：

```bash
sudo dpkg -i edgeCore-v{new-version}-amd64.deb
# 或
sudo apt install ./edgeCore-v{new-version}-amd64.deb
```

卸载：

```bash
sudo apt remove -y edgeCore
```

**RHEL / CentOS / Fedora（`.rpm`）**

首次安装：

```bash
sudo rpm -ivh edgeCore-v{version}-amd64.rpm
# 或（Fedora / RHEL 8+）
sudo dnf install ./edgeCore-v{version}-amd64.rpm
```

升级：

```bash
sudo rpm -Uvh edgeCore-v{new-version}-amd64.rpm
# 或
sudo dnf upgrade ./edgeCore-v{new-version}-amd64.rpm
```

卸载：

```bash
sudo rpm -e edgeCore
# 或
sudo dnf remove edgeCore
```

安装后验证：

```bash
sudo systemctl status edgeCore
sudo systemctl enable --now edgeCore   # 若未自动启动
```

浏览器访问 `http://<主机>:<port>` 进入管理界面；首次启动若 `data/config.db` 不存在，将进入 Web 安装向导。

#### 方式二：tar.gz 二进制包

1. **下载安装包**
   - 文件名格式：`edgeCore-{version}-linux-{arch}.tar.gz`（含二进制、`conf/`、`edgeCore.service`、`ui/dist/` 等）

2. **解压安装**
   ```bash
   sudo mkdir -p /usr/local/bin/edgeCore
   sudo tar -xzf edgeCore-{version}-linux-amd64.tar.gz -C /usr/local/bin/edgeCore

   # 可选：配置 systemd（包内附带 edgeCore.service 示例）
   sudo cp edgeCore.service /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable --now edgeCore
   ```

3. **验证安装**
   ```bash
   sudo systemctl status edgeCore
   ```

#### 方式三：源码编译安装

1. **前置条件**
   - Go 1.21+
   - Node.js 16+（编译前端）
   - Git

2. **获取源码**
   ```bash
   git clone https://github.com/anviod/edgeCore.git
   cd edgeCore
   ```

3. **编译后端**
   ```bash
   go mod tidy
   go build -o edgeCore cmd/main.go
   ```

4. **编译前端**
   ```bash
   cd ui
   npm install
   npm run build
   cd ..
   ```

5. **运行**
   ```bash
   ./edgeCore
   ```

---

## 部署流程

### 标准部署流程

#### 第一步：环境准备

1. **检查系统环境**
   ```bash
   # 检查操作系统版本
   cat /etc/os-release

   # 检查内存
   free -h

   # 检查磁盘空间
   df -h
   ```

2. **开放防火墙端口**
   ```bash
   # HTTP 管理端口
   sudo ufw allow 8082/tcp

   # BACnet UDP 端口（如使用 BACnet）
   sudo ufw allow 47808/udp

   # Modbus TCP 端口（如使用 Modbus）
   sudo ufw allow 502/tcp
   ```

#### 第二步：配置文件准备

1. **创建配置目录**
   ```bash
   sudo mkdir -p /etc/edgeCore/conf
   sudo mkdir -p /var/lib/edgeCore/data
   ```

2. **初始化配置**
   ```bash
   # 从示例配置复制
   cp -r conf/* /etc/edgeCore/conf/
   ```

3. **修改核心配置**
   - `server.yaml`：HTTP 服务器端口、静态资源路径
   - `system.yaml`：系统级网络配置
   - `users.yaml`：用户账号管理

#### 第三步：服务部署

1. **创建 systemd 服务**
   ```bash
   sudo tee /etc/systemd/system/edgeCore.service > /dev/null <<EOF
   [Unit]
   Description=edgeCore Industrial Gateway
   After=network.target

   [Service]
   Type=simple
   User=edgeCore
   WorkingDirectory=/opt/edgeCore
   ExecStart=/opt/edgeCore/edgeCore -conf /etc/edgeCore/conf
   Restart=always
   RestartSec=10
   StandardOutput=journal
   StandardError=journal

   [Install]
   WantedBy=multi-user.target
   EOF
   ```

2. **创建运行用户**
   ```bash
   sudo useradd -r -s /bin/false edgeCore
   sudo chown -R edgeCore:edgeCore /var/lib/edgeCore
   ```

3. **启动服务**
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable edgeCore
   sudo systemctl start edgeCore
   ```

4. **验证服务状态**
   ```bash
   sudo systemctl status edgeCore
   sudo journalctl -u edgeCore -f
   ```

#### 第四步：初始配置

1. **访问管理界面**
   - 打开浏览器访问：`http://<设备IP>:8082`
   - 默认账号：`admin` / `passwd@123`

2. **修改默认密码**
   - 进入「系统设置」→「用户管理」
   - 选择 admin 用户，修改密码

3. **配置网络**
   - 进入「系统设置」→「网络配置」
   - 根据现场环境配置 IP 地址、网关、DNS

#### 第五步：设备接入

1. **添加通道**
   - 进入「南向采集」→「通道管理」
   - 点击「添加通道」
   - 选择协议类型（Modbus、BACnet、OPC UA、S7、EtherNet/IP、FINS、SNMP、IEC 104、DL/T645、Mitsubishi MC、Profinet IO、KNXnet/IP、EtherCAT 等）
   - 配置通道参数

2. **添加设备**
   - 在通道下添加设备
   - 配置设备地址、端口、采集周期等参数

3. **扫描点位**
   - 进入设备详情页
   - 点击「扫描点位」自动发现设备数据点
   - 勾选需要采集的点位并添加

#### 第六步：数据转发

1. **配置北向通道**
   - 进入「北向数据」→「通道管理」
   - 添加 MQTT/OPC UA 北向通道
   - 配置连接参数

2. **配置数据映射**
   - 选择需要转发的点位
   - 配置数据格式和转换规则
   - 设置转发频率

3. **测试验证**
   - 点击「测试连接」
   - 检查数据是否正常上报

### 高可用部署（可选）

多节点部署方案，实现故障自动接管：

1. **部署多个边缘网关节点**
2. **配置节点间同步通信**
3. **设置设备访问模式（Exclusive/Shared/Lease）**
4. **配置租约超时时间**

> 详细方案请参考 [TODO - 多节点 libp2p 同步](../TODO/基于go-libp2p%20同步通信规划方案.html)

---

## 使用方式

### 快速上手指南

#### 第一次使用

1. **登录系统**
   - 打开浏览器，输入网关 IP 地址和端口
   - 输入用户名和密码登录

2. **添加第一个设备**
   ```
   南向采集 → 通道管理 → 添加通道 → 选择协议 → 填写参数 → 保存
   → 设备列表 → 添加设备 → 填写设备信息 → 保存
   → 点位列表 → 扫描点位 → 勾选点位 → 添加选定点位
   ```

3. **查看实时数据**
   - 进入「设备详情」→「实时数据」
   - 查看点位当前值、质量状态、更新时间

4. **配置边缘计算规则**
   - 进入「边缘计算」→「规则管理」
   - 添加规则，配置触发条件和动作

#### 日常操作

##### 设备管理

- **添加设备**：通道 → 设备列表 → 添加设备
- **编辑设备**：设备列表 → 编辑 → 修改参数 → 保存
- **删除设备**：设备列表 → 删除 → 确认删除
- **设备启停**：设备列表 → 启用/禁用

##### 点位管理

- **批量添加**：扫描点位 → 批量选择 → 添加
- **手动添加**：点位列表 → 添加点位 → 填写地址和类型
- **点位分组**：支持按功能、区域等分组管理
- **数据监控**：实时值、历史趋势、质量状态

##### 数据查询

- **实时数据**：设备详情 → 实时数据
- **历史数据**：数据分析 → 历史趋势
- **数据导出**：支持 CSV 格式导出

### Web 界面说明

#### 主要菜单结构

```
首页
├── 南向采集
│   ├── 通道管理
│   ├── 设备监控
│   └── 数据查询
├── 边缘计算
│   ├── 规则管理
│   ├── 运行日志
│   └── 统计分析
├── 北向数据
│   ├── 通道管理
│   ├── 数据映射
│   └── 传输监控
├── 系统管理
│   ├── 用户管理
│   ├── 系统设置
│   └── 日志查询
```

---

## 南向采集

### 支持的协议

> 完整驱动矩阵见 [设备驱动](../drivers/index.html) 与 [产品说明 · 功能索引](产品说明.html#功能索引)。架构差异（Serial / Parallel / Limited）见 [架构设计总览 §2](../edge/边缘网关架构设计总览.html)。

| 协议 | 注册名 | 执行模式 | 要点 |
| :--- | :--- | :--- | :--- |
| Modbus TCP/RTU | `modbus-tcp` / `modbus-rtu` / `modbus-rtu-over-tcp` | Serial | Gap 块读；非法地址长冷却 |
| BACnet IP | `bacnet-ip` | Limited | 对象扫描 / 发现 |
| OPC UA | `opc-ua` | Parallel | 订阅或分批 Read |
| Siemens S7 | `s7` | Limited | rack/slot |
| EtherNet/IP | `ethernet-ip` | Limited | CIP Tag |
| Omron FINS | `omron-fins` | Serial | TCP/UDP |
| SNMP | `snmp` | — | v2c / v3 |
| IEC 104 | `iec60870-5-104` | — | 总召唤 + 自发 |
| DL/T645 | `dlt645` | Serial | 表地址 + DI |
| Mitsubishi SLMP | `mitsubishi-slmp` | Serial | MC 3E |
| Profinet IO | `profinet-io` | — | 槽位 IO |
| KNXnet/IP | `knxnet-ip` | — | 组地址 / 发现 |
| EtherCAT | `ethercat` | — | PDO + SDO |

### 影子设备与采集闭环

1. 启用通道后，ScanEngine 按 Scan Class 调度 `ReadPoints`
2. 结果经 **ShadowIngress** 批量写入 **ShadowCore**
3. UI 经 WebSocket/REST **优先读影子**；边缘规则与北向经 **ShadowBridge → DataPipeline** 消费
4. 虚拟影子由公式依赖真实影子派生

推荐操作顺序：建通道 → 建设备/点位 → 启通道 → 确认 UI 实时值 → 配置边缘/北向 → 查看 `GET /api/diagnostics/scan-engine`。

### 连接健康检测

所有驱动均集成采集健康检测机制，无需独立心跳：

- **检测原理**：采集成功 = 连接健康，采集失败 = 连接异常
- **触发条件**：连续采集失败达到阈值时触发重连
- **保护机制**：指数退避 + 冷却期，防止设备过载

各驱动失败阈值：

| 驱动/型号 | 最大失败次数 | 默认采集周期 |
| :--- | :--- | :--- |
| S7-200Smart | 3 次 | 60 秒 |
| S7-1200/1500 | 5 次 | 10 秒 |
| Modbus | 5 次 | 可配置 |
| EtherNet/IP | 5 次 | 可配置 |
| OPC UA | 5 次 | 订阅回调触发 |
| FINS | 5 次 | 可配置 |
| SNMP | 5 次 | 可配置 |
| DL/T645 | 5 次 | 可配置 |
| Profinet IO | 5 次 | 可配置 |
| KNXnet/IP | 5 次 | 可配置 |
| EtherCAT | 5 次 | 周期可配置 |

### 设备管理

1. **添加设备**：
   - 进入「设备管理」页面
   - 点击「添加设备」按钮
   - 选择协议类型并填写设备信息
   - 点击「保存」完成添加

2. **编辑设备**：
   - 在设备列表中找到需要编辑的设备
   - 点击「编辑」按钮
   - 修改设备信息
   - 点击「保存」完成编辑

3. **删除设备**：
   - 在设备列表中找到需要删除的设备
   - 点击「删除」按钮
   - 确认删除操作

### 数据点管理

- **自动发现**：支持自动发现设备的数据点
- **手动添加**：支持手动添加数据点
- **数据点映射**：支持数据点的重命名和类型映射
- **采集频率**：支持设置数据点的采集频率

---

## 北向数据共享

edgeCore 支持 6 种北向通道，将边缘数据灵活对接云平台、SCADA 与企业应用：

| 北向协议 | 模式 | 说明 |
|----------|------|------|
| **MQTT** | 客户端 | 标准 MQTT 3.1.1/5.0 推送，支持离线缓存、设备事件上报 |
| **Sparkplug B** | 客户端 | MQTT 之上的工业物联网互操作协议，支持 Birth/Death 证书 |
| **OPC UA Server** | 服务端 | 以 OPC UA 服务器对外暴露影子点位，支持 Browse/Read/Write/Subscribe |
| **BACnet Server** | 服务端（从机） | 将 edgeCore 南向点位映射为 BACnet 标准对象（AI/BI/AO/BO/MV），支持 Who-Is/I-Am 设备发现、ReadProperty/WriteProperty 双向读写、COV 订阅通知，对接 BMS/SCADA 主站 |
| **HTTP** | 客户端 | 自定义 URL/Method/Headers 推送，支持离线缓存 |
| **EdgeOS (MQTT/NATS)** | 客户端 | 专用 EdgeOS 平台协议，支持设备生命周期通知与点位元数据上报 |

### 配置步骤

1. 进入「北向数据」→「添加通道」，选择协议并填写连接参数
2. 在通道配置中勾选需上报的设备/虚拟影子，设置上报策略（周期/变化）
3. 保存后查看通道运行状态；MQTT/OPC UA/BACnet 可点击「接入文档」核对 Topic 或 Endpoint

### BACnet Server 配置要点

1. 进入「北向数据」→ 添加 BACnet Server 通道
2. 填写 BACnet 设备参数（设备 ID、设备名称、端口等）
3. 在「映射真实设备」标签页中勾选需要暴露给 BACnet 主站的设备
4. 保存后 BACnet Server 启动，对外暴露 BACnet 标准对象
5. 在 Yabe 或 BMS/SCADA 中输入网关 IP 和端口即可访问

Topic/Payload 格式与 API 字段见 [北向数据文档](../northbound/index.html)、[MQTT 数据格式](../northbound/MQTT数据上下行格式.html)、[BACnet Server 文档](../northbound/BACnet_Server.html) 与 [北向配置 API](../API/Northbound_Configuration_CN.html)。

---

## 边缘计算

> 规则类型、expr 语法、场景编排与 API 见 [边缘计算基础功能](../edge/边缘计算基础功能.html)、[场景手册](../edge/EDGE_COMPUTING_SCENARIO_MANUAL.html)、[边缘计算最佳实践](EDGE_COMPUTING_BEST_PRACTICES.html)、[边缘计算 API](../API/Edge_Computing_CN.html)。

### 配置步骤

1. **添加规则**：
   - 进入「边缘计算」页面
   - 点击「添加规则」按钮
   - 选择规则类型并填写规则信息
   - 配置触发条件和动作
   - 点击「保存」完成添加

2. **编辑规则**：
   - 在规则列表中找到需要编辑的规则
   - 点击「编辑」按钮
   - 修改规则信息
   - 点击「保存」完成编辑

3. **启用/禁用规则**：
   - 在规则列表中找到需要操作的规则
   - 点击「启用」或「禁用」按钮

4. **验证规则**：
   - 启用规则后在「记录与日志」查看运行状态
   - 或通过 `GET /api/edge/states` 确认 `current_status`

---

## 系统管理

### 用户管理

- **添加用户**：创建新的系统用户
- **编辑用户**：修改用户信息和权限
- **删除用户**：删除不需要的用户
- **权限管理**：设置用户的操作权限

### 系统配置

- **网络配置**：配置网络参数和连接
- **系统参数**：配置系统运行参数
- **存储配置**：配置数据存储参数
- **安全配置**：配置系统安全参数

### 系统监控

- **资源监控**：监控 CPU、内存、磁盘使用情况
- **服务状态**：监控系统服务的运行状态
- **网络状态**：监控网络连接和数据传输情况
- **日志管理**：查看系统日志和操作记录

### 运维诊断与 SLA 监控

edgeCore 采用**统计 SLA**（非硬实时 PLC），内置阈值门控与 diagnostics API，**无需 Prometheus/Grafana** 等外部监控栈。运维可通过 **HTTP JSON 巡检**、**Web UI 指标页** 与 **结构化日志 grep** 三条通路观察采集健康度。

#### 1. Diagnostics API 巡检

所有 diagnostics 端点挂载在 `/api` 下，需携带登录 JWT（`Authorization: Bearer <token>`）。默认网关端口 `8082`。

| 端点 | 说明 |
|------|------|
| `GET /api/diagnostics/scan-engine` | ScanEngine 全局调度指标，含 `sla_warnings[]` |
| `GET /api/diagnostics/soak` | Soak 长稳会话与 Release Gate 验收项 |
| `GET /api/devices/:deviceId/diagnostics` | 单设备 IO 画像、scan task lag、断路器状态 |
| `GET /api/channels/:channelId/diagnostics/events` | 通道 Event Log（CB Open/Reject 等最近事件） |
| `GET /api/channels/:channelId/metrics` | 通道级通信 KPI（成功率、RTT、丢包率等） |
| `GET /api/channels/:channelId/devices/:deviceId/metrics` | 设备级通信 KPI |
| `GET /api/points/:pointId/debug` | 点位调试（原始字节 + 解析值） |

**ScanEngine SLA 快照示例：**

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:8082/api/diagnostics/scan-engine | jq '{
  lag_p95: .scan_lag_p95_ms,
  drift: .scan_drift_avg_ms,
  miss: .scan_miss_deadline_total,
  cb_open: .driver_circuit_open_total,
  backpressure: .backpressure_reject_total,
  warnings: .sla_warnings
}'
```

**单设备 diagnostics 示例：**

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:8082/api/devices/modbus-slave-1/diagnostics | jq .
```

**通道 Event Log 示例：**

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:8082/api/channels/<channelId>/diagnostics/events | jq .
```

#### 2. 解读 sla_warnings

`sla_warnings` 为数组，由 ScanEngine 内置阈值自动生成。非空表示至少一项 SLA 指标超出稳态阈值，需进一步排查。

| code | 含义 | 常见原因 |
|------|------|----------|
| `scan_lag_p95_exceeded` | 调度 lag P95 超阈 | 点位过多、采集周期过短、设备响应慢 |
| `scan_drift_avg_exceeded` | 漂移均值超阈 | 调度拥塞或共享串口链路慢 |
| `scan_miss_deadline_exceeded` | miss deadline 累计 | 任务长期 overdue |
| `circuit_breaker_rejects` | 断路器拒绝请求 | 设备离线或频繁超时 |

**响应示例：**

```json
{
  "code": "scan_lag_p95_exceeded",
  "metric": "scan_lag_p95_ms",
  "value": 125.5,
  "threshold": 100,
  "message": "scan lag P95 125.50ms exceeds 100ms"
}
```

**告警响应建议：**

1. `sla_warnings` 非空 → 查对应通道 Event Log 定位 channel/device
2. `driver_circuit_open_total` 增加 → 检查设备网络或从站状态，等待 HalfOpen 探测
3. `backpressure_reject_total` 持续上升 → 降载或调大 scan interval

#### 3. Web UI 指标页

**首页 — ScanEngine Soak 面板**

- 浏览器路径：`http://<网关IP>:8082/`（登录后进入 Dashboard）
- 展示 ScanEngine 运行监控、SLA / Soak 状态、Release Gate 项
- 数据来源：`GET /api/diagnostics/soak`

**通道列表 — 监控弹窗**

1. 进入 **南向采集 → 通道列表**
2. 点击目标通道卡片上的 **监控** 图标（图表样式按钮）
3. 弹窗分区说明：
   - **质量评分**：综合成功率、RTT、丢包、lag P95、CB Open 的 0–100 分
   - **调度 SLA**：Lag P95、Drift 均值、CB Open 数、反压拒绝次数
   - **SLA 告警**：`sla_warnings` 列表（黄色告警条，逐条显示 `message`）
   - **详细指标**：CRC 错误率、重试率、请求计数等
4. 并行请求 `GET /api/channels/:id/metrics` 与 `GET /api/diagnostics/scan-engine`

**点位列表 — 点位调试**

1. 进入通道下 **点位列表**
2. 对目标点位执行 **调试** 操作
3. 查看原始字节、解析值与质量状态（`GET /api/points/:pointId/debug`）

#### 4. 结构化日志 grep

SLA 周期告警（约每 30s 扫描，有告警才输出 WARN）：

```bash
grep '\[SLA\]' /var/log/edgeCore/app.log
grep 'scan_lag_p95_exceeded\|circuit_breaker_open' /var/log/edgeCore/app.log
```

#### 5. UI 手动验证清单

| 步骤 | 预期 |
|------|------|
| 登录 Dashboard | 首页可见 ScanEngine Soak 面板 |
| 通道列表 → 监控 | 弹窗显示质量评分与调度 SLA 区块 |
| `sla_warnings` 非空时 | 弹窗出现黄色 SLA 告警列表 |
| 点位调试 | 返回原始字节与解析值 |

更多阈值对照与压测回归命令见 [SLA 运维手册](../deployment/sla_monitoring.html)。

### 备份与恢复

- **数据备份**：备份系统配置和数据
- **配置导出**：导出系统配置
- **恢复系统**：从备份恢复系统

---

## 最佳实践

### 热路径最佳实践（推荐）

完整路径：**南向 ReadPoints → ShadowIngress → ShadowCore → ShadowBridge → DataPipeline → 边缘 / 历史 / 北向；UI 直读影子。**

| 建议 | 说明 |
|------|------|
| 以影子为中心配置北向与规则 | 避免绕过 Shadow 的旁路回调，保证数据面一致 |
| 合理 Interval + Scan Class | fast/normal/slow 分离关键与慢变点，降低总线压力 |
| Serial 协议勿过度轮询 | Modbus RTU / DLT645 等共享链路，周期过短易饿死其他从站 |
| 观察 SLA | `GET /api/diagnostics/scan-engine` 与通道监控面板 |
| 弱网启用北向缓存 | Store & Forward / NorthboundCache 断网补发 |

详见 [架构设计总览 §4](../edge/边缘网关架构设计总览.html)。

### 设备接入最佳实践

#### 1. 通道规划

**原则：按网络分区和协议类型规划通道**

- 同一协议、同一网段的设备尽量放在同一通道
- 不同协议分通道管理，便于维护
- 设备数量较多时，按功能或区域分组

**示例：**
```
通道1: Modbus-TCP-车间A (192.168.1.0/24)
  - 设备1: PLC-01 (192.168.1.10)
  - 设备2: PLC-02 (192.168.1.11)

通道2: BACnet-楼宇B (192.168.2.0/24)
  - 设备1: BAC-01 (192.168.2.100)
  - 设备2: BAC-02 (192.168.2.101)
```

#### 2. 采集周期设置

**原则：根据设备能力和业务需求合理设置**

| 场景 | 推荐采集周期 | 说明 |
| :--- | :--- | :--- |
| 快速变化信号（转速、压力） | 1~5 秒 | 需要高实时性 |
| 慢速变化信号（温度、湿度） | 10~30 秒 | 变化缓慢，无需高频 |
| 状态监控（运行/停止） | 5~10 秒 | 状态变化不频繁 |
| 累计量（电能、流量） | 30~60 秒 | 避免太频繁影响累计精度 |

**S7-200Smart 特别注意：**
- 采集周期建议 ≥ 10 秒（默认 60 秒）
- 最大失败次数 3 次（保护弱 PLC）

#### 3. 点位命名规范

**原则：统一命名，便于管理和识别**

命名格式：`区域_设备_类型_序号`

示例：
- `workshopA_plc01_temp_01`
- `buildingB_bac01_humidity_02`
- `energy_meter01_power_total`

#### 4. 批量读取优化

**原则：尽量使用批量读取，减少通信次数**

- 连续地址的点位尽量放在同一个批量读取中
- 合理利用 MTU 探测功能，自动优化批量大小
- 间隔较远的点位分批次读取

### 边缘计算最佳实践

#### 1. 规则设计原则

- **简单优先**：能用简单规则实现的不要用复杂规则
- **避免死循环**：注意规则之间的依赖关系，避免循环触发
- **性能考虑**：复杂计算规则注意执行频率，避免影响系统性能

#### 2. 阈值设置建议

- 留有余量：阈值设置考虑一定的余量，避免频繁震荡
- 滞回设置：使用滞回阈值，避免临界值附近频繁触发
- 分级告警：设置多级阈值（预警、告警、严重）

#### 3. 规则测试

- **逐步启用**：先测试再启用，避免影响正常运行
- **日志观察**：启用后观察一段时间运行日志
- **性能监控**：注意规则执行时间和资源占用

### 系统运维最佳实践

#### 1. 定期维护

- **每周**：检查系统日志，排查异常
- **每月**：备份配置和数据，检查磁盘空间
- **每季度**：检查设备连接状态，清理无用配置

#### 2. 故障处理流程

```
发现故障 → 查看日志 → 定位问题 → 处理修复 → 验证恢复 → 记录总结
```

#### 3. 安全建议

- 立即修改默认密码
- 使用强密码策略
- 定期更换密码
- 限制管理界面访问 IP
- 启用 HTTPS（生产环境）

### 性能优化最佳实践

#### 1. 采集性能优化

- 合理设置采集周期，避免过高频率
- 启用批量读取，减少通信次数
- 利用智能采集优化功能（RTT/MTU/Gap）

#### 2. 存储优化

- 配置合理的数据保留策略
- 定期清理历史数据
- 使用高效的存储格式

#### 3. 网络优化

- 设备与网关尽量在同一网段
- 保证网络质量，减少丢包
- 合理规划网络拓扑

---

## 故障排查

### 常见问题

#### 1. 设备连接失败

**可能原因：**
- 设备电源和网络连接问题
- 设备地址和端口配置错误
- 设备未正常运行
- 防火墙阻止了连接

**排查步骤：**
```bash
# 1. 测试网络连通性
ping <设备IP>

# 2. 测试端口连通性
telnet <设备IP> <端口>
# 或
nc -zv <设备IP> <端口>

# 3. 检查网关日志
journalctl -u edgeCore -f
```

**解决方案：**
- 检查设备电源和网线
- 确认设备地址和端口配置正确
- 检查防火墙设置
- 确认设备协议版本兼容性

#### 2. 数据采集异常

**可能原因：**
- 设备通信状态不稳定
- 数据点配置错误（地址、类型）
- 采集频率设置过高
- 设备负载过重

**排查步骤：**
1. 检查设备连接状态
2. 查看采集质量状态
3. 检查点位配置是否正确
4. 查看系统日志中的错误信息

**解决方案：**
- 调整采集周期
- 核对点位地址和数据类型
- 降低采集频率
- 检查设备负载情况

#### 3. 规则执行失败

**可能原因：**
- 规则条件配置错误
- 数据源状态异常
- 动作配置错误
- 表达式语法错误

**排查步骤：**
1. 查看规则运行日志
2. 检查数据源状态
3. 验证规则表达式
4. 测试动作执行

**解决方案：**
- 修正规则条件配置
- 检查数据源连接
- 修正动作配置
- 使用测试功能验证规则

#### 4. 数据转发失败

**可能原因：**
- 网络连接问题
- 转发目标配置错误
- 数据格式配置错误
- 权限不足

**排查步骤：**
1. 检查网络连通性
2. 验证转发目标配置
3. 检查数据格式配置
4. 查看传输日志

**解决方案：**
- 检查网络连接
- 修正目标配置
- 验证数据格式
- 检查认证信息

### 诊断工具

- **Diagnostics API**：`GET /api/diagnostics/scan-engine` 与 `sla_warnings[]` 巡检 — 见 [运维诊断与 SLA 监控](#运维诊断与-sla-监控)
- **UI 指标页**：首页 Soak 面板、通道列表「监控」弹窗 — 同上
- **Ping 测试**：测试网络连接（系统管理 → 网络）
- **设备扫描**：扫描可用设备
- **日志查看**：查看系统和应用日志；SLA 告警 `grep '[SLA]'`
- **性能分析**：分析系统性能问题

### 技术支持

- **在线文档**：访问官方文档获取帮助
- **问题反馈**：通过 GitHub Issues 提交问题
- **社区支持**：参与技术论坛讨论

---

## 附录

### 术语表

| 术语 | 解释 |
| ---- | ---- |
| 边缘计算 | 在靠近数据源的边缘设备上进行数据处理和分析的技术 |
| 南向采集 | 从设备采集数据的过程 |
| 北向转发 | 将数据发送到云端或其他系统的过程 |
| 规则引擎 | 执行边缘计算规则的核心组件 |
| 数据管道 | 处理和传输数据的通道 |
| 阈值规则 | 基于设定阈值触发动作的规则 |
| 状态规则 | 基于持续状态判断的规则 |
| 窗口规则 | 基于时间或计数窗口的规则 |
| 指数退避 | 失败重试时逐渐增加等待时间的算法 |
| 冷却期 | 连续失败后暂停重试的时间段 |
| 半开探测 | 冷却期后发送轻量请求探测设备是否恢复 |

### 版本历史

| 版本 | 日期 | 变更内容 |
| ---- | ---- | ---- |
| v1.2 | 2026-07 | 新增运维诊断与 SLA 监控章节（Diagnostics API、sla_warnings、UI 指标页） |
| v1.1 | 2026-06 | 新增安装指南、部署流程、最佳实践章节；更新驱动支持矩阵 |
| v1.0 | 2026-03-27 | 初始版本 |

### 相关文档

- [产品说明](产品说明.html) - 产品介绍与技术栈
- [设备驱动](../drivers/index.html) - 各协议驱动详细文档
- [开发计划](../development_plan/index.html) - 路线图与待开发功能
- [边缘计算最佳实践](EDGE_COMPUTING_BEST_PRACTICES.html) - 边缘计算深度指南
