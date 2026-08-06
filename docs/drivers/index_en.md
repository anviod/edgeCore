---
layout: section-index
title: Device Drivers (English)
description: edgeCore Southbound Driver Documentation — Modbus, BACnet, OPC UA, S7, EtherNet/IP, FINS, SNMP, IEC 104, DL/T645, Mitsubishi MC, Profinet IO, KNXnet/IP, EtherCAT
hero_eyebrow: Southbound Drivers
hero_title: Device Drivers
hero_lead: Design docs, test reports, and optimization notes for edgeCore southbound drivers — Modbus, BACnet, OPC UA, S7, EtherNet/IP, Omron FINS, SNMP, IEC 104, DL/T645, Mitsubishi MC, Profinet IO, KNXnet/IP, and more.
hero_buttons:
  - text: Home
    url: ../index.html
    style: primary
  - text: 中文版
    url: index.html
    style: secondary
  - text: Test Report
    url: ../testing/southbound-driver-test-report.html
    style: secondary
---

## Driver Support Matrix

| Protocol | Registry Key | Status | Read | Write | Scan / Discover | ConnectionManager | Unit Tests (`-short` cov) |
| :--- | :--- | :--- | :---: | :---: | :---: | :---: | :--- |
| **Modbus TCP/RTU** | `modbus-tcp`, `modbus-rtu`, `modbus-rtu-over-tcp` | Production | Yes | Yes | — | Yes | **65.9%** |
| **BACnet IP** | `bacnet-ip` | Production | Yes | Yes | Scan + ScanObjects | Partial | **66.1%** |
| **OPC UA Client** | `opc-ua` | Production | Yes | Yes | Scan + ScanObjects | Yes | **47.9%** |
| **Siemens S7** | `s7` | Production | Yes | Yes | — | Yes | **61.3%** |
| **EtherNet/IP** | `ethernet-ip` | Production | Yes | Yes | — | Yes | **62.2%** |
| **Omron FINS** | `omron-fins` | Production | Yes | Yes | — | Yes | **43.3%** |
| **SNMP v2c/v3** | `snmp` | Production | Yes | Yes | ScanObjects | Yes | **63.7%** |
| **IEC 60870-5-104** | `iec60870-5-104` | M1 delivered | Yes | Yes (single command) | — | Yes | **60.2%** |
| **DL/T645-2007** | `dlt645` | Production | Yes | Yes | — | Yes | **76.5%** ✅ |
| **Mitsubishi SLMP** | `mitsubishi-slmp` | Production | Yes | Yes | — | Yes | **70.7%** ✅ |
| **Profinet IO** | `profinet-io` | Production | Yes | Yes | — | Yes | **55.9%** |
| **KNXnet/IP** | `knxnet-ip` | Production Ready | Yes | Yes | Gateway Discovery | Yes | **77.2%** ✅ |
| **EtherCAT** | `ethercat` | M1 Delivered | PDO + SDO | PDO + SDO | Yes | Yes | **87.8%** ✅ |

> ConnectionManager (shared component): **87.4%** · Main driver package **22/22 PASS** (2026-07-12)

---

## Key Configuration Parameters

| Driver | Main Config Keys |
| :--- | :--- |
| Modbus | `ip`, `port`, `slaveId`, `timeout`, connection type (TCP/RTU/RTU-over-TCP) |
| BACnet | `ip`, `port`, `deviceId`, broadcast interface, object instance |
| OPC UA | `endpoint`, security policy/mode, credentials, subscription interval |
| S7 | `ip`, `port`, `rack`, `slot`, PLC model (200Smart/1200/1500/300/400) |
| EtherNet/IP | `ip`, `port`, `slot`, tag path, connection type |
| Omron FINS | `plcIP`/`ip`, `plcPort`/`port`, `timeout`, src/dst node addresses, TCP/UDP mode |
| Mitsubishi MC | `ip`, `port`, `frame_type`, `network_no`, `station_no`, `timeout` |
| Profinet IO | `local_interface`, `timeout`, `simulation`; device `ip`, `port`, `slot`, `subslot` |
| KNXnet/IP | `ip`, `port`, `mode` (TCP/UDP), `discovery`, `discovery_timeout`, `discovery_multicast` |
| SNMP | `snmpVersion`, `targetIP`, `community` (v2c), USM auth/priv (v3), `maxBulkSize` |
| DLT645 | `connectionType` (serial/tcp), `port`, `ip`, `baudRate`, `timeout`, meter address + DI |
| IEC 104 | `ip`, `port`, `commonAddress`, T0–T3 timers, general call interval |

---

## Connection Management (2026-06)

Shared **ConnectionManager** (`internal/driver/connection_manager.go`):

- State machine: `Disconnected → Connecting → Connected → Retrying → Dead`
- Exponential backoff with jitter and cooldown (max 1 hour)
- Collection health: successful read = healthy; no separate heartbeat
- Low-frequency probe when scan interval exceeds 3× threshold

**Supported**: S7, Modbus, EtherNet/IP, OPC UA, FINS, SNMP, DL/T645, Profinet IO, KNXnet/IP

---

## Test Coverage

Full report: [Southbound Driver Test Report](../testing/southbound-driver-test-report.html)

```bash
CGO_ENABLED=0 go test ./internal/driver/modbus ./internal/driver/bacnet ... -count=1 -cover
```

Last run (**2026-07-04**): all 21 main driver packages **PASS** under `-short`; extended `coverage_test.go` across drivers. Integration/manual tags documented in the test report.

---

## Related Docs

- [Driver Index (中文)](index.html)
- [Modbus Optimization](MODBUS_OPTIMIZATION.html)
- [BACnet Design](BACnet_设计说明.html)
- [BACnet Best Practices](BACnet_最佳实践.html) — development guide & 4-phase verification
- [SNMP Driver](SNMP.html)
- [Architecture](../architecture/index.html)
- [Development Plan](../development_plan/index.html)
