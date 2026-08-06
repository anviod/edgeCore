---
layout: default
title: User Manual
description: edgeCore user manual — protocols, deployment, operations, and best practices
---

# edgeCore User Manual

[中文用户手册](USER_MANUAL.html) · [Product Guide](PRODUCT.html) · [Architecture](../en/architecture-overview.html)

This English manual covers **protocols, deployment, day-to-day use, and hot-path best practices**. Deep Chinese reference remains in [USER_MANUAL.md](USER_MANUAL.html).

## 1. Protocols (southbound)

| Protocol | Key | Notes |
|----------|-----|-------|
| Modbus TCP/RTU | `modbus-tcp` / `modbus-rtu` / `modbus-rtu-over-tcp` | Serial execution; Gap batch read |
| BACnet IP | `bacnet-ip` | Scan + object scan |
| OPC UA | `opc-ua` | Parallel; subscribe / batch read |
| S7 | `s7` | rack/slot |
| EtherNet/IP | `ethernet-ip` | CIP tags |
| Omron FINS | `omron-fins` | TCP/UDP |
| SNMP | `snmp` | v2c / v3 |
| IEC 104 | `iec60870-5-104` | Interrogation + spontaneous |
| DL/T645 | `dlt645` | Meter DI |
| Mitsubishi SLMP | `mitsubishi-slmp` | MC 3E |
| Profinet IO | `profinet-io` | Slot IO |
| KNXnet/IP | `knxnet-ip` | Group address / discovery |
| EtherCAT | `ethercat` | PDO/SDO (M1) |

Driver docs: [index_en](../drivers/index_en.html).

## 2. Installation

### Requirements

| | Minimum | Recommended |
|--|---------|-------------|
| RAM | 128MB | 512MB+ |
| Disk | 1GB | 4GB+ |
| CPU | 1 core | 2+ |

### Packages

```bash
# Debian/Ubuntu
sudo dpkg -i edgeCore-v{version}-amd64.deb

# RHEL/Fedora
sudo rpm -ivh edgeCore-v{version}-amd64.rpm

sudo systemctl enable --now edgeCore
```

Open `http://<host>:<port>`. First boot runs the install wizard if `data/config.db` is missing.

Source build: see [README.en.md](../../README.en.md).

## 3. Operations (shadow-centric)

1. **Create channel** → choose protocol → set connection params  
2. **Add device + points** → set interval / Scan Class  
3. **Enable channel** → ScanEngine registers tasks  
4. **Verify live values** in UI (WebSocket from ShadowCore)  
5. **Optional:** virtual shadow formulas, edge rules, northbound MQTT/OPC UA/Sparkplug  
6. **Watch SLA:** `GET /api/diagnostics/scan-engine`

Data path:

```text
Driver ReadPoints → ShadowIngress → ShadowCore → UI / Edge / History / Northbound
```

## 4. Best practices

| Do | Don't |
|----|-------|
| Keep intervals realistic for the bus (especially Modbus RTU) | Flood Serial buses with 10ms polls on dozens of slaves |
| Use Scan Class (fast/normal/slow) for mixed criticality | Put all tags in one ultra-fast class |
| Map northbound from **shadow** tags | Bypass shadow with ad-hoc driver callbacks |
| Check `sla_warnings` and circuit-breaker state | Ignore Dead/Offline devices forever without cooldown |
| Enable northbound cache for weak networks | Assume always-on uplink |

Architecture detail: [Architecture Overview](../en/architecture-overview.html).

## 5. Troubleshooting quick list

| Symptom | Check |
|---------|-------|
| No live values | Channel enabled? Driver Connect? Shadow populated? |
| High lag / miss | diagnostics API; per-device CB; backpressure |
| Illegal Modbus address | Point enters long SKIPPED cooldown (24h) |
| Northbound silent | Pipeline handlers registered? Client connected? Cache full? |

Full Chinese ops chapters: [USER_MANUAL](USER_MANUAL.html).
