---
layout: default
title: SNMP 驱动
description: edgeCore SNMP v2c/v3 采集驱动说明
---

# SNMP 驱动

edgeCore SNMP 驱动支持 **SNMP v2c** 与 **SNMP v3**，通过 UDP（默认 161）采集网络设备 OID 点位，集成 ScanEngine 周期采集与 Shadow 数据面。

## 协议 ID

`snmp`

## 通道配置

### SNMP v2c

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `ip` / `targetIP` | string | — | 设备 IP（必填） |
| `port` / `targetPort` | int | 161 | SNMP 端口 |
| `snmpVersion` | string | `v2c` | 版本 |
| `community` | string | `public` | 社区字符串 |
| `timeout` | int | 3000 | 超时（毫秒） |
| `retries` | int | 3 | 重试次数 |
| `maxBulkSize` | int | 10 | GETBULK 重复数 |
| `sendInterval` | int | 100 | 请求间隔（毫秒） |

```json
{
  "ip": "192.168.1.1",
  "port": 161,
  "snmpVersion": "v2c",
  "community": "public",
  "timeout": 3000,
  "retries": 3
}
```

### SNMP v3

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `securityName` | string | — | USM 用户名（必填） |
| `securityLevel` | string | `authPriv` | `noAuthNoPriv` / `authNoPriv` / `authPriv` |
| `authProtocol` | string | `SHA256` | MD5 / SHA1 / SHA224 / SHA256 / SHA384 / SHA512 |
| `authPassword` | string | — | authNoPriv / authPriv 必填 |
| `privProtocol` | string | `AES128` | DES / AES128 / AES192 / AES256 |
| `privPassword` | string | — | authPriv 必填 |
| `contextName` | string | — | 可选 |
| `contextEngineID` | string | — | 可选 |

```json
{
  "ip": "192.168.1.1",
  "port": 161,
  "snmpVersion": "v3",
  "securityName": "admin",
  "securityLevel": "authPriv",
  "authProtocol": "SHA256",
  "authPassword": "AuthPass123",
  "privProtocol": "AES128",
  "privPassword": "PrivPass123"
}
```

## 点位地址

| 版本 | 格式 | 示例 |
|------|------|------|
| v2c | `community\|OID` | `public\|1.3.6.1.2.1.1.1.0` |
| v3 | `securityName\|OID` | `admin\|1.3.6.1.2.1.1.5.0` |

## 支持操作

- **GET**：单 OID 或同组批量读取
- **GETBULK / GETNEXT / WALK**：批量读与 MIB 扫描（`ScanObjects`）
- **SET**：可写 OID 写入（需 RW 权限 community / v3 用户）

## 数据类型

STRING、BYTES、BOOL/BIT、UINT8–UINT64、INT8–INT64、FLOAT、DOUBLE

## 标准 OID 参考

| OID | 名称 |
|-----|------|
| 1.3.6.1.2.1.1.1.0 | sysDescr |
| 1.3.6.1.2.1.1.3.0 | sysUpTime |
| 1.3.6.1.2.1.1.5.0 | sysName |
| 1.3.6.1.2.1.2.2.1.10.{n} | ifInOctets |
| 1.3.6.1.2.1.2.2.1.16.{n} | ifOutOctets |

## 代码位置

```
internal/driver/snmp/
├── snmp.go
├── transport.go
├── scheduler.go
├── decoder.go
├── config.go
└── protocol.go
```

## 相关文档

- [SNMP 驱动开发方案（TODO）](../TODO/SNMP/SNMP采集驱动开发.html)
- [南向采集 TODO 索引](../TODO/index.html)
