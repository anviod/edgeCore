---
layout: default
title: Product Guide
description: edgeCore industrial edge gateway — capabilities, stability, southbound access, edge compute, northbound integration
---

# edgeCore Product Guide

[中文版](产品说明.html) · [User Manual](USER_MANUAL.en.html) · [Architecture](../en/architecture-overview.html)

Industrial Edge Gateway software for manufacturing, energy, and building sites. Go backend · Vue 3 admin UI · single static binary.

## Value proposition

| Pillar | What you get |
|--------|----------------|
| **Industrial stability** | Schedule-driven ScanEngine, per-device circuit breakers, backpressure, Soak + CI gates, observable SLA |
| **Easy device onboarding** | 13 southbound protocols, discovery/scan where applicable, batch point registration, hot config without restart |
| **Shadow-centric data plane** | In-memory ShadowCore as runtime SoT — UI, rules, history, and northbound share one snapshot |
| **Edge compute** | Threshold / expression / window rules; virtual shadow formulas; local write-back |
| **Northbound ready** | MQTT · Sparkplug B · OPC UA Server · BACnet Server · HTTP · EdgeOS — map shadow tags to SCADA/cloud |
| **AI assist (advanced)** | Protocol reverse / diagnostics copilot skills (`internal/ai_agent`) |

## Southbound protocols

| Protocol | Registry key | Status |
|----------|--------------|--------|
| Modbus TCP/RTU | `modbus-tcp`, `modbus-rtu`, `modbus-rtu-over-tcp` | Production |
| BACnet IP | `bacnet-ip` | Production |
| OPC UA Client | `opc-ua` | Production |
| Siemens S7 | `s7` | Production |
| EtherNet/IP | `ethernet-ip` | Production |
| Omron FINS | `omron-fins` | Production |
| SNMP v2c/v3 | `snmp` | Production |
| IEC 60870-5-104 | `iec60870-5-104` | M1 |
| DL/T645-2007 | `dlt645` | Production |
| Mitsubishi SLMP | `mitsubishi-slmp` | Production |
| Profinet IO | `profinet-io` | Production |
| KNXnet/IP | `knxnet-ip` | Production |
| EtherCAT | `ethercat` | M1 |

Details: [Driver Matrix](../drivers/index_en.html).

## Architecture in one glance

```text
Devices → ScanEngine → ShadowCore (SoT) → UI / Edge Rules / History / Northbound
                         ↑
                   Virtual Shadow (formulas)
```

Authoritative design: [Architecture Overview](../en/architecture-overview.html) · [中文总览](../edge/边缘网关架构设计总览.html).

## Deployment snapshot

| Item | Spec |
|------|------|
| Delivery | Single binary, `CGO_ENABLED=0` |
| Minimum | 128MB RAM · 1GB disk |
| Arch | x86_64 · ARM64 · ARMv7 |
| Install | deb / rpm / tar.gz / systemd |

See [User Manual — Install](USER_MANUAL.en.html#installation).

## SLA (statistical)

Not hard real-time PLC cycle. Typical mock gates (≤10k tags): lag P95 &lt;100ms · miss deadline =0 (steady) · diagnostics API. Full narrative: [产品说明 — SLA](产品说明.html#工业级-sla-与稳定性验证).

## Positioning vs peers

Comparable class to industrial collectors / edge platforms (Kepware-style multi-protocol collection, Ignition/ThingsBoard Edge-style edge processing, Node-RED industrial flows) — edgeCore emphasizes **unified shadow SoT**, **schedule-driven SLA**, and **lightweight single-binary** field deployment.
