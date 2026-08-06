# Industrial Edge Gateway

[中文文档](README.md) | [Documentation Site](https://anviod.github.io/edgeCore/)

Industrial Edge Gateway is a lightweight industrial edge computing gateway for manufacturing, energy, building automation, and other field deployments. The backend is Go; the management UI is Vue 3 + Vuetify.

## Product Overview

Industrial Edge Gateway runs at the industrial edge to bridge **OT devices ↔ IT systems** — unified southbound access, local edge processing, and flexible northbound integration in a single gateway from acquisition to reporting. **13 southbound protocols write into ShadowCore (runtime SoT), then fan out to virtual devices, edge rules, persistence, and northbound channels — backed by industrial-grade SLA and Soak long-stability verification.**

- **Unified access**: heterogeneous PLCs, meters, building and network devices — one gateway for collection
- **Shadow SoT**: in-memory ShadowCore shared by UI, edge compute, and northbound
- **Edge intelligence**: local rules and derived tags for control linkage and reduced uplink traffic
- **Open integration**: cloud platforms, SCADA, and enterprise apps with reverse write/control
- **Industrial-grade stability**: built-in metric gates, Soak regression, and CI five-gate verification — SLA observable and verifiable

> Designed for the industrial field, built on stability-first principles, unified architecture, and long-term maintainability — a future-ready industrial edge computing platform.

See [Product Advantages](#product-advantages) below; concise guide: [PRODUCT.md](docs/guide/PRODUCT.md); full details in the [Product Guide (中文)](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#产品优势) ([source](docs/guide/产品说明.md#产品优势)).


## Product Advantages

**Industrial-grade stability** is edgeCore's core differentiator: delivering rich functionality while sustaining SLA in production through built-in metric gates, Soak long-stability regression, and CI five-gate verification.

| Capability | Core Value | Key Metrics / Deliverables |
| :--- | :--- | :--- |
| **Quality assurance** | Built-in metric gates + Soak regression + CI five-gate | lag P95 **<100ms** · miss deadline **=0** · 10k-tag benchmark · observable diagnostics |
| **Southbound access** | 13 industrial protocols, unified OT collection | device discovery · object scan · batch tag registration |
| **Collection scheduling** | 10ms-class scheduling kernel, in-memory shadow as source of truth | P99 scheduling lag **<150ms** (≤10k tags, statistical SLA) |
| **Edge intelligence** | rule engine + virtual shadow derived computation | cross-device mapping · formula aggregation · local control linkage |
| **Northbound integration** | multi-protocol cloud, SCADA, and enterprise connectivity | MQTT · Sparkplug B · OPC UA · BACnet · EdgeOS |

Full details: [Product Guide — Product Advantages (中文)](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#产品优势) ([source](docs/guide/产品说明.md#产品优势)).

### Industrial-Grade SLA & Stability Verification

Statistical SLA (not hard real-time PLC). Built-in gates and `GET /diagnostics/scan-engine` for observability. Verification chain: **runtime metric monitoring** → **diagnostics API** → **CI five-gate regression** → **Soak long-stability verification**. Thresholds, 10k-tag benchmark, Soak reports, and deployment guidance: [Product Guide — Industrial SLA (中文)](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#工业级-sla-与稳定性验证) ([source](docs/guide/产品说明.md#工业级-sla-与稳定性验证)).

### Lightweight Deployment

| Item | Specification |
| :--- | :--- |
| **Delivery** | single binary · `CGO_ENABLED=0` static build · no runtime dependencies |
| **Minimum** | 128MB RAM · 1GB storage |
| **Architectures** | x86_64 · ARMv7 · ARM64 |
| **Deployment** | bare metal / systemd · embedded devices |

### Feature Index

| Module | Description |
| :--- | :--- |
| **Southbound collection** | [Industrial protocols](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#丰富的工业协议支持) · [Scheduling & shadow](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#低延迟数据采集与处理) · [Collection optimization](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#智能采集优化) |
| **Northbound reporting** | [Northbound integration](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#北向平台集成) |
| **Virtual shadow** | [Virtual shadow devices](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#虚拟影子设备) |
| **Edge computing** | [Rules & linkage](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#边缘规则与联动控制) |
| **System management** | Vue 3 admin UI · JWT / LDAP · install wizard · config.db · logging · WebSocket live push |
| **Quality assurance** | [Industrial SLA](#industrial-grade-sla--stability-verification) · [Lightweight deployment](#lightweight-deployment) |

## Documentation

Runtime details — architecture, component responsibilities, data paths — see the [documentation site](https://anviod.github.io/edgeCore/) (local sources in `docs/`):

| Document | Description |
| :--- | :--- |
| [Architecture](https://anviod.github.io/edgeCore/architecture/index.html) · [source](docs/architecture/index.md) | ScanEngine scheduling kernel, ShadowCore, system diagrams |
| [Architecture Overview (EN)](https://anviod.github.io/edgeCore/en/architecture-overview.html) · [source](docs/en/architecture-overview.md) | English hot-path architecture summary |
| [Product Guide (EN)](docs/guide/PRODUCT.md) · [产品手册](docs/guide/PRODUCT.zh-CN.md) | Concise product positioning |
| [Product Guide (中文)](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html) · [source](docs/guide/产品说明.md) | capabilities, SLA metrics, feature details |
| [Changelog (更新记录)](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html#%E6%9B%B4%E6%96%B0%E8%AE%B0%E5%BD%95) · [source](docs/guide/产品说明.md#更新记录) | Jul 2026 highlights: EtherCAT M1, drivers 22/22, Mac 10k-tag retest, AI Copilot MVP |
| [Edge Gateway Architecture Overview (中文)](https://anviod.github.io/edgeCore/edge/%E8%BE%B9%E7%BC%98%E7%BD%91%E5%85%B3%E6%9E%B6%E6%9E%84%E8%AE%BE%E8%AE%A1%E6%80%BB%E8%A7%88.html) · [source](docs/edge/边缘网关架构设计总览.md) | authoritative component layout and hot path (v3.0) |
| [User Manual (EN)](docs/guide/USER_MANUAL.en.md) · [用户手册](docs/guide/USER_MANUAL.md) | protocols, deploy, ops, best practices |
| [Southbound Driver Matrix](https://anviod.github.io/edgeCore/drivers/index.html) · [source](docs/drivers/index.md) | 13 protocol drivers and development standards |
| [Development plan](https://anviod.github.io/edgeCore/development_plan/index.html) · [source](docs/development_plan/index.md) | Q3/Q4 delivery tracking |
| [Testing & Verification](https://anviod.github.io/edgeCore/testing/index.html) · [source](docs/testing/index.md) | SLA benchmarks, Soak, and regression reports |

English driver matrix: [online](https://anviod.github.io/edgeCore/drivers/index_en.html) · [source](docs/drivers/index_en.md) · English docs hub: [online](https://anviod.github.io/edgeCore/en/index.html) · [source](docs/en/index.md)

---

## Deployment

### 1. Build

#### Multi-platform release (recommended)

The project uses [GoReleaser](https://goreleaser.com/) (`.goreleaser.yml`) to build frontend and backend and produce per-platform packages.

```bash
go install github.com/goreleaser/goreleaser/v2@latest

# From repo root: clean dist/ and build (snapshot, no GitHub Release)
goreleaser release --snapshot --clean
```

**Build pipeline** (`before.hooks`): `go mod tidy` → `npm run build --prefix ./ui`

**Cross-compile targets** (`CGO_ENABLED=0`, entry `./cmd/main.go`, binary `edgeCore`): Linux amd64/arm64/arm/v7; Windows amd64.

Artifacts under `./dist/`: tar.gz bundles, `.deb` packages, SHA256SUMS.

#### Manual build (development / single platform)

```bash
git clone https://github.com/anviod/edgeCore.git
cd edgeCore
go mod tidy

# Backend: static build, no CGO
CGO_ENABLED=0 go build -o edgeCore ./cmd/main.go

# Frontend (recommended for production; served from ui/dist)
cd ui && npm install && npm run build && cd ..
```

### 2. Directories & Configuration

| Path | Description |
| :--- | :--- |
| `data/` | runtime data; `config.db` stores config (install wizard on first start if empty) |
| `logs/` | runtime logs (default `logs/gateway.edgeCore.log`) |
| `conf/` | legacy YAML for one-time migration (`-conf` flag, default `./conf`) |

### 3. Start

```bash
# Default config dir ./conf; HTTP port from server config (commonly 8080/8082)
./edgeCore

# Specify legacy YAML migration directory
./edgeCore -conf ./conf/
```

Open `http://localhost:<port>` for the admin UI. Default credentials via install wizard or `conf/users.yaml`.

### 4. Production References

- **systemd, firewall, initialization**: [User Manual — Deployment (中文)](https://anviod.github.io/edgeCore/guide/USER_MANUAL.html#部署流程) ([source](docs/guide/USER_MANUAL.md#部署流程))
- **EdgeOS integration**: [EdgeOS Quick Start](https://anviod.github.io/edgeCore/deployment/edgeos-quickstart.html) ([source](docs/deployment/edgeos-quickstart.md))
- **Multi-slave Modbus**: [Multi-Slave Quick Start](https://anviod.github.io/edgeCore/deployment/QUICK_START_MULTI_SLAVE.html) ([source](docs/deployment/QUICK_START_MULTI_SLAVE.md))
- **Full deployment index**: [GitHub Pages](https://anviod.github.io/edgeCore/deployment/) · [source](docs/deployment/index.md)

---

## Document Index

| Category | Link |
| :--- | :--- |
| Documentation site (GitHub Pages) | [https://anviod.github.io/edgeCore/](https://anviod.github.io/edgeCore/) |
| **Development principles & acceptance** | [online](https://anviod.github.io/edgeCore/DEVELOPMENT_PRINCIPLES.html) · [source](docs/DEVELOPMENT_PRINCIPLES.md) |
| **Roadmap** | [online](https://anviod.github.io/edgeCore/ROADMAP.html) · [source](docs/ROADMAP.md) |
| **Release gates** | [online](https://anviod.github.io/edgeCore/RELEASE_GATE.html) · [source](docs/RELEASE_GATE.md) |
| Driver docs | [online](https://anviod.github.io/edgeCore/drivers/index.html) · [source](docs/drivers/index.md) |
| Deployment guides | [online](https://anviod.github.io/edgeCore/deployment/) · [source](docs/deployment/index.md) |
| Architecture | [online](https://anviod.github.io/edgeCore/architecture/index.html) · [source](docs/architecture/index.md) |
| Development plan / TODO | [online](https://anviod.github.io/edgeCore/development_plan/index.html) · [TODO index](https://anviod.github.io/edgeCore/TODO/index.html) · [source](docs/TODO/index.md) |
| Testing & verification | [online](https://anviod.github.io/edgeCore/testing/index.html) · [source](docs/testing/index.md) |
| Operations reference | [online](https://anviod.github.io/edgeCore/operations/index.html) · [source](docs/operations/index.md) |

---

## Branch & Collaboration

Day-to-day development on **`dev`**; target PRs to **`dev`**. **`main`** is the stable release line, merged from `dev` by maintainers as needed.

## Quick Start (Development)

```bash
go mod tidy
go run cmd/main.go          # backend, default -conf ./conf
cd ui && npm install && npm run build && cd ..   # frontend (optional)
```

Default admin port depends on `server` config; BACnet requires UDP 47808 open.

## Build & Tests

```bash
CGO_ENABLED=0 go build -o edgeCore ./cmd/main.go

# Short gate (see test/README.md for soak / Q3 benchmarks)
make test-short
```

Test assets and maintenance rules: [test/README.md](test/README.md) · Reports: [docs/testing/index.md](docs/testing/index.md)

---

## Related Documents

- **Product Guide (中文)**: [online](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html) · [source](docs/guide/产品说明.md)
- **User Manual**: [online](https://anviod.github.io/edgeCore/guide/USER_MANUAL.html) · [source](docs/guide/USER_MANUAL.md)
- **Driver matrix**: [online](https://anviod.github.io/edgeCore/drivers/index.html) · [source](docs/drivers/index.md) · [English](https://anviod.github.io/edgeCore/drivers/index_en.html) · [source](docs/drivers/index_en.md)
- **中文 README**: [README.md](README.md)