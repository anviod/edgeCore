# Industrial Edge Gateway

[中文文档站点](https://anviod.github.io/edgeCore/) | [English](README.en.md)

edgeCore（工业边缘网关）是一款轻量级工业边缘计算网关软件，面向制造、能源、楼宇等现场部署。 AI特性支持MCP协议，实现与LLM模型的通信。

<div align="center">
  <img src="./docs/img/dataScanEngineCN.svg" width="100%" alt="" />
  <p><small>图 1：数据流向总览</small></p>
</div>

## 产品概览

edgeCore 部署在工业现场，使命是打通 **OT 设备 ↔ IT 系统** 的数据通道 — 南向统一接入、边缘就地处理、北向灵活对接，一机完成采集到上报的闭环。**以南向 13 协议采集写入 ShadowCore 影子真源，联动虚拟设备、边缘规则、持久化与北向通道；并以工业级 SLA 与 Soak 长稳验证保障现场长期可靠运行。**


能力与指标详见 [产品优势](#产品优势)；精简宣传见 [产品手册](docs/guide/PRODUCT.zh-CN.md)；完整说明见 [产品说明](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#产品优势)（[源码](docs/guide/产品说明.md#产品优势)）。


## 产品优势

**工业级稳定性**是 edgeCore 的核心点：在功能丰富的同时，通过内置指标门控、Soak 长稳回归与 CI 五 gate 持续验证 SLA，保障现场长期可靠运行。

| 维度 | 核心价值 | 关键指标 / 交付物 |
| --- | --- | --- |
| **保障** | 内置指标门控 + Soak 长稳回归 + CI 五 gate |  1万 Tag 长期压测稳定,无内存泄露  |
| **南向** | 13 种工业协议，异构 OT 统一采集 | 设备发现 · 对象扫描 · 批量点位注册 |
| **采集** | 10ms 级调度内核，内存影子真源 | P99 调度 lag **<150ms**（≤10k tag，统计 SLA） |
| **边缘** | 规则引擎 + 虚拟影子派生计算 | 跨设备映射 · 公式聚合 · 本地联动控制 |
| **北向** | 多协议对接云平台、SCADA 与企业应用 | MQTT · Sparkplug B · OPC UA · BACnet · EdgeOS |

完整说明见 [产品说明 — 产品优势](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#产品优势)（[源码](docs/guide/产品说明.md#产品优势)）。

### 工业级 SLA 与稳定性验证

统计 SLA（非硬实时 PLC），内置门控与 `GET /api/diagnostics/scan-engine` 可观测。验证链路覆盖：**运行时指标监测** → **诊断 API 可观测** → **CI 五 gate 回归** → **Soak 长稳验证**。阈值定义、万 Tag 压测、Soak 长稳与部署建议见 [产品说明 — 工业级 SLA](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#工业级-sla-与稳定性验证)（[源码](docs/guide/产品说明.md#工业级-sla-与稳定性验证)）。

<div align="center">
  <img src="./docs/img/productCapabilityLayers.svg" width="100%" alt="" />
  <p><small>图 2：产品能力维度</small></p>
</div>



### 轻量灵活部署

| 项目 | 规格 |
| --- | --- |
| **交付形态** | 单二进制 · `CGO_ENABLED=0` 静态编译 · 无运行时依赖 |
| **最低配置** | 128MB 内存 · 1GB 存储 |
| **支持架构** | x86_64 · ARMv7 · ARM64 |
| **部署方式** | 裸机 / systemd · 嵌入式设备 |

### 功能索引

| 模块 | 说明 |
| --- | --- |
| **南向采集** | [工业协议](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#丰富的工业协议支持) · [调度与影子](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#低延迟数据采集与处理) · [采集优化](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#智能采集优化) |
| **北向上报** | [北向平台集成](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#北向平台集成) |
| **虚拟影子** | [虚拟影子设备](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#虚拟影子设备) |
| **边缘计算** | [规则与联动](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#边缘规则与联动控制) |
| **系统管理** | Vue 3 管理 UI · JWT / LDAP · 安装向导 · config.db · 日志 · WebSocket 实时推送 |
| **质量保障** | [工业级 SLA](#工业级-sla-与稳定性验证) · [轻量部署](#轻量灵活部署) |
| **AI / MCP** | [MCP 接入指南](https://anviod.github.io/edgeCore/guide/mcp-access-guide.html) · [源码](docs/guide/mcp-access-guide.md) | MCP 协议接入，33 个工具 · 6 资源 · 13 提示词 |

## 文档导航

架构图、组件职责与数据路径等运行时细节见 [文档站点](https://anviod.github.io/edgeCore/)（本地源码见 `docs/`）：

| 文档 | 说明 |
| --- | --- |
| [架构设计](https://anviod.github.io/edgeCore/architecture/index.html) · [源码](docs/architecture/index.md) | ScanEngine 调度内核、ShadowCore、系统架构图 |
| [边缘网关架构设计总览](https://anviod.github.io/edgeCore/edge/%E8%BE%B9%E7%BC%98%E7%BD%91%E5%85%B3%E6%9E%B6%E6%9E%84%E8%AE%BE%E8%AE%A1%E6%80%BB%E8%A7%88.html) · [源码](docs/edge/边缘网关架构设计总览.md) | 南向→影子→边缘/北向热路径（v3.0） |
| [Architecture Overview (EN)](https://anviod.github.io/edgeCore/en/architecture-overview.html) · [源码](docs/en/architecture-overview.md) | English architecture summary |
| [产品手册](docs/guide/PRODUCT.zh-CN.md) · [PRODUCT (EN)](docs/guide/PRODUCT.md) | 精简宣传向产品说明 |
| [产品说明](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html) · [源码](docs/guide/产品说明.md) | 能力详解、SLA 指标与功能说明 |
| [更新记录](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#%E6%9B%B4%E6%96%B0%E8%AE%B0%E5%BD%95) · [源码](docs/guide/产品说明.md#更新记录) | 最近重点：EtherCAT M1、驱动 22/22、Mac 万 Tag 复测、AI Copilot MVP |
| [用户手册](docs/guide/USER_MANUAL.md) · [User Manual (EN)](docs/guide/USER_MANUAL.en.md) | 协议、部署、操作与最佳实践 |
| [南向驱动矩阵](https://anviod.github.io/edgeCore/drivers/index.html) · [源码](docs/drivers/index.md) | 13 种协议驱动与开发规范 |
| [开发计划](https://anviod.github.io/edgeCore/development_plan/index.html) · [源码](docs/development_plan/index.md) | Q3/Q4 交付跟踪与已交付里程碑 |
| [测试验证](https://anviod.github.io/edgeCore/testing/index.html) · [源码](docs/testing/index.md) · [test/ 维护](test/README.md) | SLA 压测、Soak 与 `go test` 维护原则 |
| [MCP 接入指南](https://anviod.github.io/edgeCore/guide/mcp-access-guide.html) · [源码](docs/guide/mcp-access-guide.md) | MCP 协议接入、工具清单与客户端配置 |

---

## 部署流程

### 1. 编译方式

#### 一键多平台打包（推荐）

项目使用 [GoReleaser](https://goreleaser.com/)（`.goreleaser.yml`）统一构建前端与后端，并生成分平台安装包。GoReleaser 内置 nfpm，用于生成 `.deb` / `.rpm` 包，无需单独安装 nfpm。

```bash
go install github.com/goreleaser/goreleaser/v2@latest

# 在仓库根目录执行：清理 dist/ 并构建（snapshot 模式，不发布 GitHub Release）
goreleaser release --snapshot --clean
```

**构建流水线**（`before.hooks`）：

1. `go mod tidy` — 整理 Go 依赖
2. `npm run build --prefix ./ui` — 构建 Vue 前端至 `ui/dist/`

**交叉编译目标**（`CGO_ENABLED=0`，入口 `./cmd/main.go`，二进制名 `edgeCore`）：

| 平台 | 架构 |
| --- | --- |
| Linux | amd64、arm64、arm/v7 |
| Windows | amd64（不含 arm/arm64） |

**产物目录** `./dist/`：

| 类型 | 说明 |
| --- | --- |
| **tar.gz** | `edgeCore-{version}-{os}-{arch}.tar.gz`，含二进制、`conf/`、`scripts/`、`edgeCore.service`、`edgeCore.sh`、`ui/dist/` 等 |
| **deb** | `edgeCore-v{version}-{arch}.deb`，安装至 `/usr/local/bin/edgeCore`，含 systemd 单元与前端静态资源 |
| **rpm** | `edgeCore-v{version}-{arch}.rpm`，安装路径与 deb 相同，适用于 RHEL / CentOS / Fedora 等 |
| **SHA256SUMS** | 各产物校验和 |

#### 手动编译（开发 / 单平台）

```bash
git clone https://github.com/anviod/edgeCore.git
cd edgeCore
go mod tidy

# 后端：静态编译，无 CGO 依赖
CGO_ENABLED=0 go build -o edgeCore ./cmd/main.go

# 前端（生产环境推荐，产物由后端托管 ui/dist）
cd ui && npm install && npm run build && cd ..
```

### 2. 系统包安装与升级（Linux）

从 [GitHub Releases](https://github.com/anviod/edgeCore/releases) 或本地 `dist/` 目录获取对应架构的安装包（`amd64` / `arm64` / `arm`），文件名形如 `edgeCore-v{version}-{arch}.deb` 或 `.rpm`。

包内会安装二进制至 `/usr/local/bin/edgeCore/`，注册 systemd 服务 `edgeCore`；升级时 `preinstall` / `postinstall` 脚本会自动备份并恢复 `config/` 与 `data/`，无需手工迁移配置。

#### Debian / Ubuntu（`.deb`）

```bash
# 首次安装
sudo dpkg -i edgeCore-v{version}-amd64.deb
sudo apt-get install -f -y

# 升级
sudo dpkg -i edgeCore-v{new-version}-amd64.deb
# 或
sudo apt install ./edgeCore-v{new-version}-amd64.deb

# 卸载
sudo apt remove -y edgeCore
```

#### RHEL / CentOS / Fedora（`.rpm`）

```bash
# 首次安装
sudo rpm -ivh edgeCore-v{version}-amd64.rpm
# 或
sudo dnf install ./edgeCore-v{version}-amd64.rpm

# 升级
sudo rpm -Uvh edgeCore-v{new-version}-amd64.rpm
# 或
sudo dnf upgrade ./edgeCore-v{new-version}-amd64.rpm

# 卸载
sudo rpm -e edgeCore
```

#### 安装后验证

```bash
sudo systemctl status edgeCore
sudo systemctl enable --now edgeCore   # 若未自动启动
```

浏览器访问 `http://<主机>:<port>` 进入管理界面；首次启动若 `data/config.db` 不存在，将进入 Web 安装向导。

> **tar.gz 裸机部署**：若使用 `edgeCore-{version}-linux-{arch}.tar.gz`，解压后参考下方「目录与配置」与「启动服务」，或自行配置 systemd（包内附带 `edgeCore.service` 示例）。

### 3. 目录与配置

| 路径 | 说明 |
| --- | --- |
| `data/` | 运行时数据目录；`config.db` 存储配置（首次启动为空则进入安装向导） |
| `logs/` | 运行日志（默认 `logs/gateway.edgeCore.log`） |
| `conf/` | 遗留 YAML 配置，用于一次性迁移（`-conf` 参数指定，默认 `./conf`） |

首次启动若 `data/config.db` 不存在，网关以安装模式启动，通过 Web UI 完成初始化。

### 4. 启动服务

```bash
# 默认配置目录 ./conf，HTTP 端口见 server 配置（常见 8080/8082）
./edgeCore

# 指定遗留 YAML 迁移目录
./edgeCore -conf ./conf/
```

访问 `http://localhost:<port>` 进入管理界面。默认账号见安装向导或 `conf/users.yaml`。

### 5. 生产部署参考

- **systemd 服务、防火墙端口、配置初始化**：见 [用户手册 — 部署流程](https://anviod.github.io/edgeCore/guide/USER_MANUAL.html#部署流程)（[源码](docs/guide/USER_MANUAL.md#部署流程)）
- **系统包安装与升级（deb / rpm）**：见 [用户手册 — 安装指南](https://anviod.github.io/edgeCore/guide/USER_MANUAL.html#安装指南)（[源码](docs/guide/USER_MANUAL.md#安装指南)）
- **EdgeOS 集成与北向通道**：见 [EdgeOS 快速入门](https://anviod.github.io/edgeCore/deployment/edgeos-quickstart.html)（[源码](docs/deployment/edgeos-quickstart.md)）
- **多从站 Modbus 配置**：见 [多从站快速入门](https://anviod.github.io/edgeCore/deployment/QUICK_START_MULTI_SLAVE.html)（[源码](docs/deployment/QUICK_START_MULTI_SLAVE.md)）

---

## 文档索引

| 类别 | 链接 |
| --- | --- |
| 文档站点（GitHub Pages） | [https://anviod.github.io/edgeCore/](https://anviod.github.io/edgeCore/) |
| **开发原则与验收标准** | [在线阅读](https://anviod.github.io/edgeCore/DEVELOPMENT_PRINCIPLES.html) · [源码](docs/DEVELOPMENT_PRINCIPLES.md) |
| **分阶段路线图** | [在线阅读](https://anviod.github.io/edgeCore/ROADMAP.html) · [源码](docs/ROADMAP.md) |
| **版本发布门禁** | [在线阅读](https://anviod.github.io/edgeCore/RELEASE_GATE.html) · [源码](docs/RELEASE_GATE.md) |
| 驱动文档 | [在线阅读](https://anviod.github.io/edgeCore/drivers/index.html) · [源码](docs/drivers/index.md) |
| 用户手册（安装部署） | [在线阅读](https://anviod.github.io/edgeCore/guide/USER_MANUAL.html#安装指南) · [源码](docs/guide/USER_MANUAL.md#安装指南) |
| 架构设计 | [在线阅读](https://anviod.github.io/edgeCore/architecture/index.html) · [源码](docs/architecture/index.md) |
| 开发计划 / TODO | [docs/TODO/](docs/TODO/index.md)（未发布至 Pages） |
| Q3 南向采集优化方案 | [docs/[TODO]边缘计算南向采集优化方案2026第三季度.md](docs/[TODO]边缘计算南向采集优化方案2026第三季度.md)（未发布至 Pages） |
| 测试验证 | [在线阅读](https://anviod.github.io/edgeCore/testing/index.html) · [源码](docs/testing/index.md) · [test/ 维护](test/README.md) |
| 运维参考 | [在线阅读](https://anviod.github.io/edgeCore/operations/index.html) · [源码](docs/operations/index.md) |
| MCP 接入指南 | [在线阅读](https://anviod.github.io/edgeCore/guide/mcp-access-guide.html) · [源码](docs/guide/mcp-access-guide.md) |

---

## 分支与协作

日常开发在 `**dev**` 分支进行；功能与修复 PR 请以 `**dev**` 为合并目标。`main` 保留为稳定发布线，由维护者从 `dev` 按需合并。

## 快速开始（开发）

```bash
go mod tidy
go run cmd/main.go          # 后端，默认 -conf ./conf
cd ui && npm install && npm run build && cd ..   # 前端（可选）

# 短测门控（详见 test/README.md）
make test-short
```

默认管理端口取决于 `server` 配置；BACnet 需开放 UDP 47808。

---

## 相关文档

- **产品说明**：[在线阅读](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html) · [源码](docs/guide/产品说明.md)
- **用户手册**：[在线阅读](https://anviod.github.io/edgeCore/guide/USER_MANUAL.html) · [源码](docs/guide/USER_MANUAL.md)

---

English documentation: [README.en.md](README.en.md)