---
layout: default
title: 认证 API
description: edgeCore 认证 API 文档（中文）
---

# 认证 API (Authentication)

## 1. 获取系统信息
返回基础系统信息（如名称、软件版本）。

*   **URL**: `/auth/system-info`
*   **Method**: `GET`
*   **Auth Required**: 否

**响应**:
```json
{
  "code": "0",
  "data": {
    "name": "EdgeGateway-01",
    "softVer": "v1.0.0"
  }
}
```

## 2. 获取登录随机数 (Nonce)
获取用于密码加密的随机数 nonce。有效期 2 分钟。限流 (2次/秒)。

*   **URL**: `/auth/nonce`
*   **Method**: `GET`
*   **Auth Required**: 否

**响应**:
```json
{
  "code": "0",
  "data": {
    "nonce": "a1b2c3d4e5f6..."
  }
}
```

## 3. 登录
用户认证。支持本地和 LDAP 登录。

*   **URL**: `/auth/login`
*   **Method**: `POST`
*   **Auth Required**: 否

**请求体**:
```json
{
  "loginFlag": true,
  "loginType": "local", // "local" (本地) 或 "ldap" (LDAP)
  "data": {
    "username": "admin",
    "password": "<SHA256(raw_password + nonce)>", // Hex 编码字符串
    "nonce": "<上一步获取的 nonce>"
  }
}
```

**响应**:
```json
{
  "code": "0",
  "msg": "Success",
  "data": {
    "username": "admin",
    "token": "eyJhbGciOiJIUzI1Ni...",
    "permissions": ["admin"]
  }
}
```

## 4. 登出
*   **URL**: `/auth/logout`
*   **Method**: `POST`
*   **Auth Required**: 否 (客户端应丢弃 Token)

**响应**:
```json
{
  "code": "0",
  "msg": "Logged out"
}
```

## 5. 修改密码
修改当前用户的密码。

*   **URL**: `/auth/change-password`
*   **Method**: `POST`
*   **Auth Required**: 是 (JWT)

**请求体**:
```json
{
  "oldPassword": "<SHA256(old_raw_password + nonce)>",
  "newPassword": "new_raw_password",
  "nonce": "<新请求获取的 nonce>"
}
```

**响应**:
```json
{
  "code": "0",
  "msg": "密码修改成功"
}
```
