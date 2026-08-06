---
layout: section-index
title: edgeCore Documentation (English)
description: English documentation hub for edgeCore industrial edge gateway — product overview, architecture, drivers, and testing
hero_eyebrow: Expert Knowledge Base
hero_title: edgeCore Documentation (English)
hero_lead: English entry for edgeCore industrial edge gateway. Primary technical content is maintained in Chinese; this hub mirrors README.en and links to published Pages.
hero_buttons:
  - text: README.en (GitHub)
    url: https://github.com/anviod/edgeCore/blob/dev/README.en.md
    style: primary
  - text: Home (中文)
    url: ../index.html
    style: secondary
  - text: Driver Matrix (EN)
    url: ../drivers/index_en.html
    style: secondary
  - text: Product Guide (中文)
    url: ../guide/产品说明.html
    style: secondary
---

## Product Overview

Industrial Edge Gateway runs at the industrial edge to bridge **OT devices ↔ IT systems** — unified southbound access, local edge processing, and flexible northbound integration in a single gateway. **13 southbound protocols write into ShadowCore (runtime SoT), then fan out to virtual devices, edge rules, persistence, and northbound — backed by industrial-grade SLA and Soak verification.**

- **Unified access**: heterogeneous PLCs, meters, building and network devices — one gateway for collection
- **Shadow SoT**: in-memory ShadowCore shared by UI, edge compute, and northbound
- **Edge intelligence**: local rules and derived tags for control linkage and reduced uplink traffic
- **Open integration**: cloud platforms, SCADA, and enterprise apps with reverse write/control
- **Industrial-grade stability**: built-in metric gates, Soak regression, and CI five-gate verification

Full narrative: [README.en.md](https://github.com/anviod/edgeCore/blob/dev/README.en.md) on GitHub. Concise product page: [PRODUCT](../guide/PRODUCT.html).

## Product Advantages

| Capability | Core Value | Key Metrics |
| :--- | :--- | :--- |
| **Quality assurance** | Metric gates + Soak + CI five-gate | lag P95 **<100ms** · miss deadline **=0** · 10k-tag benchmark |
| **Southbound access** | 13 industrial protocols | device discovery · object scan · batch tag registration |
| **Collection scheduling** | 10ms-class kernel, in-memory shadow SoT | P99 lag **<150ms** (≤10k tags, statistical SLA) |
| **Edge intelligence** | rules + virtual shadow derived computation | cross-device mapping · formula aggregation |
| **Northbound integration** | cloud, SCADA, enterprise | MQTT · Sparkplug B · OPC UA · EdgeOS |

Detailed SLA, protocols, and virtual shadow: [Product Guide (中文)](../guide/产品说明.html#产品优势).

## Documentation

| Document | Description |
| :--- | :--- |
| [Architecture Overview (EN)](architecture-overview.html) | Southbound → Shadow → edge / history / northbound hot path |
| [Architecture (中文)](../architecture/index.html) | ScanEngine scheduling kernel, ShadowCore, system diagrams |
| [Product Guide (EN)](../guide/PRODUCT.html) | Concise product positioning |
| [产品手册 / 产品说明](../guide/PRODUCT.zh-CN.html) · [产品说明](../guide/产品说明.html) | Chinese product docs |
| [User Manual (EN)](../guide/USER_MANUAL.en.html) | Protocols, deploy, ops, best practices |
| [Edge Gateway Architecture (中文)](../edge/边缘网关架构设计总览.html) | Authoritative architecture overview (v3.0) |
| [Southbound Driver Matrix (EN)](../drivers/index_en.html) | 13 protocol drivers and development standards |
| [Testing & Verification](../testing/index.html) | SLA benchmarks, Soak, regression, and driver test reports |
| [Southbound Driver Test Report](../testing/southbound-driver-test-report.html) | Coverage matrix and hot-path unit tests |
| [User Manual — Installation (中文)](../guide/USER_MANUAL.html#安装指南) | deb / rpm / tar.gz install and upgrade |

## Implemented Southbound Drivers

Registered in `cmd/main.go`:

| Protocol | Registry Key | Status |
| :--- | :--- | :--- |
| Modbus TCP/RTU | `modbus-tcp`, `modbus-rtu`, `modbus-rtu-over-tcp` | Production |
| BACnet IP | `bacnet-ip` | Production |
| OPC UA Client | `opc-ua` | Production |
| Siemens S7 | `s7` | Production |
| EtherNet/IP | `ethernet-ip` | Production |
| Omron FINS | `omron-fins` | Production |
| SNMP v2c/v3 | `snmp` | Production |
| IEC 60870-5-104 | `iec60870-5-104` | M1 delivered |
| DL/T645-2007 | `dlt645` | Implemented |
| Mitsubishi SLMP | `mitsubishi-slmp` | Production |
| Profinet IO | `profinet-io` | Implemented |
| KNXnet/IP | `knxnet-ip` | Production |
| EtherCAT | `ethercat` | M1 delivered |

See [Southbound Driver Test Report](../testing/southbound-driver-test-report.html) for coverage under `-short`.

## Related

- [中文首页](../index.html)
- [README.md](https://github.com/anviod/edgeCore/blob/dev/README.md)
- [README.en.md](https://github.com/anviod/edgeCore/blob/dev/README.en.md)
