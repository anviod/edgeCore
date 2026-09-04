---
layout: default
title: 工业边缘网关 API 文档
description: edgeCore API 文档（中文）
---

# 工业边缘网关 API 文档

欢迎使用工业边缘网关 API 文档。本 API 符合 RESTful 规范，使用 JSON 作为请求和响应体。

## 基础 URL (Base URL)

`http://<gateway-ip>:<port>/api`

默认端口通常为 8080 或 9090，具体取决于配置。

## 认证鉴权 (Authentication)

大多数端点需要 JWT 认证。
1. 调用 `GET /auth/nonce` 获取一次性随机数 (nonce)。
2. 调用 `POST /auth/login` 使用用户名和加密密码登录 (加密方式：SHA256(密码原文 + nonce))。
3. 在 `Authorization` 请求头中使用返回的 `token`: `Bearer <token>`。

## 文档模块

*   [认证与用户管理](Authentication_CN.html)
*   [系统管理](System_Management_CN.html)
*   [通道与设备管理 (南向)](Channel_Device_Management_CN.html)
*   [边缘计算](Edge_Computing_CN.html)
*   [北向配置](Northbound_Configuration_CN.html)

## 通用响应格式

成功:
```json
{
  "code": "0", // 或 HTTP 200 直接返回数据
  "msg": "Success",
  "data": { ... }
}
```

错误:
```json
{
  "code": "1", // 或 HTTP 4xx/5xx
  "error": "错误信息描述"
}
```
