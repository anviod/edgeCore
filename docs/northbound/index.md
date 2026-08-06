---
layout: section-index
title: 北向数据
description: edgeCore 北向数据文档 — MQTT 数据格式、Sparkplug B、HTTP、OPC UA 与 BACnet 服务端
hero_eyebrow: Northbound Integration
hero_lead: edgeCore 北向数据文档 — MQTT 上下行数据格式、Sparkplug B、HTTP 推送、OPC UA 与 BACnet 服务端集成参考。
hero_buttons:
  - text: MQTT 数据格式
    url: MQTT数据上下行格式.html
    style: primary
  - text: 北向配置 API
    url: ../API/Northbound_Configuration_CN.html
    style: secondary
  - text: OPC UA 服务端
    url: ../drivers/OPC-UA_Server_Functionality.html
    style: secondary
  - text: BACnet Server
    url: BACnet_Server.html
    style: secondary
  - text: EdgeOS 通信规范
    url: ../edgeos/edgeCore通信协议规范%28MQTT-NATS%29.html
    style: secondary
  - text: 返回首页
    url: ../index.html
    style: secondary
---

## 目录

### 数据格式与 API
- [MQTT 数据上下行格式](MQTT数据上下行格式.html) — MQTT / Sparkplug B Topic 与 Payload（权威）
- [北向配置 API](../API/Northbound_Configuration_CN.html) — MQTT / HTTP / Sparkplug B / OPC UA / BACnet REST 配置

### 协议与服务
- [OPC UA 服务端功能](../drivers/OPC-UA_Server_Functionality.html) — 功能设计与验收（权威）；UI Help Dialog 为精简接入说明
- [BACnet Server 北向从机模式](BACnet_Server.html) — 将 edgeCore 南向点位映射为 BACnet 标准对象，对外暴露给 BMS/SCADA 主站
- [edgeCore 通信协议规范 (MQTT-NATS)](../edgeos/edgeCore通信协议规范%28MQTT-NATS%29.html) — EdgeOS 北向协议

> UI 内 Help Dialog（MQTT / Sparkplug / OPC UA / BACnet / EdgeOS）为现场速查；完整字段以本目录与 API 文档为准。
