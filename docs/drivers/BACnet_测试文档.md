# BACnet 驱动测试文档

## 测试策略

基于「手动添加为主，WhoIs 扫描为辅」的架构设计，测试分为三个层次：
1. **单元测试**：核心函数逻辑验证
2. **集成测试**：手动添加 + WhoIs 扫描全链路
3. **端到端测试**：远程部署后实际设备验证

---

## 一、单元测试

### 1.1 手动添加 - discoverDevice

| 测试用例 | 输入 | 预期输出 |
|---------|------|----------|
| 正常手动添加 | DeviceID=1234, IP=192.168.3.115, Port=47810 | ReadProperty 成功 → 设备注册 |
| IP 不可达 | DeviceID=9999, IP=192.168.1.1, Port=47808 | ReadProperty 超时 → WhoIs 广播 → 直连回退 |
| 端口错误 | DeviceID=1234, IP=192.168.3.115, Port=9999 | ReadProperty 超时 → WhoIs 广播 → 直连回退 |

### 1.2 WhoIs 扫描 - Scan

| 测试用例 | 输入 | 预期输出 |
|---------|------|----------|
| 单播 WhoIs | target_ip=192.168.3.115 | 单播到 47808 → 返回设备列表 |
| 广播 WhoIs | 无 target_ip | 广播 WhoIs → 返回同网段设备 |
| 无 interface_ip | interface_ip 为空 | 返回错误 |
| ObjectName 富化 | 发现设备后 | ReadProperty(Object_Name) 成功 → object_name 非空 |

### 1.3 连接 - connectOnce

| 测试用例 | 输入 | 预期输出 |
|---------|------|----------|
| MaxPDU 设置 | 常规连接 | ClientBuilder.MaxPDU = 1476 |
| 端口分离 | 连接端口 47809 | 发现/扫描端口 47808 独立 |

### 1.4 设备隔离 - isolation

| 测试用例 | 输入 | 预期输出 |
|---------|------|----------|
| 连续失败 3 次 | 设备不可达 | 触发隔离，指数退避 |
| 每日重置 | 跨日 | 隔离计数重置 |

---

## 二、集成测试

### 2.1 手动添加全链路

```bash
# 1. 创建 BACnet 通道
POST /api/channels
{
  "name": "BACnet",
  "protocol": "bacnet-ip",
  "enable": true,
  "config": {
    "interface_ip": "192.168.3.230",
    "interface_port": 47809
  }
}

# 2. 手动添加设备
POST /api/channels/BACnet/devices
[{
  "id": "bacnet-1234",
  "name": "Room Simulator",
  "enable": true,
  "config": {
    "bacnet_device_id": 1234,
    "ip": "192.168.3.115",
    "port": 47810
  }
}]

# 3. 验证设备注册
GET /api/channels/BACnet/devices
# 预期: 设备 bacnet-1234 状态 = online
```

### 2.2 WhoIs 扫描全链路

```bash
# 1. 扫描设备（补充手段）
POST /api/channels/BACnet/scan?sync=1
{
  "target_ip": "192.168.3.115",
  "interface_ip": "192.168.3.230"
}

# 2. 添加扫描到的设备
POST /api/channels/BACnet/devices
[{
  "id": "bacnet-{device_id}",
  "name": "{object_name}",
  "config": {
    "bacnet_device_id": {device_id},
    "ip": "{ip}",
    "port": {port}
  }
}]
```

### 2.3 点位扫描

```bash
# 扫描指定设备的点位
POST /api/channels/BACnet/scan?sync=1
{
  "device_id": 1234
}

# 预期: 返回该设备的所有对象列表
```

---

## 三、端到端测试

### 3.1 环境准备

- 采集端：192.168.3.230（RK3588 ARM64，运行 edgeCore）
- 目标端：192.168.3.115（Yabe BACnet 模拟器）
- 网络：同一子网 192.168.3.0/24

### 3.2 测试步骤

1. **构建部署**：`goreleaser release --snapshot --clean` → 生成 ARM64 deb 包
2. **远程安装**：`dpkg -i edgeCore-v0.0.9~SNAPSHOT-arm64.deb`
3. **服务验证**：`systemctl is-active edgeCore` → active
4. **创建通道**：POST /api/channels → BACnet 通道
5. **手动添加**：POST /api/channels/BACnet/devices → 设备注册成功
6. **WhoIs 扫描**：POST /api/channels/BACnet/scan → 返回设备列表
7. **点位扫描**：POST /api/channels/BACnet/scan (device_id=1234) → 返回对象列表
8. **实时轮询**：验证点位值更新
9. **写入验证**：写入点位值 → 二次验证确认

### 3.3 验收标准

| 验收项 | 通过标准 |
|--------|----------|
| 手动添加 | 设备注册成功，状态 online |
| WhoIs 单播 | 返回目标 IP 设备列表 |
| WhoIs 广播 | 返回同网段设备列表 |
| ObjectName 富化 | 设备名称非空 |
| 点位扫描 | 返回完整对象列表 |
| 实时轮询 | 80%+ 成功率 |
| 写入验证 | 写入后二次验证通过 |
| 超时恢复 | 设备离线后自动恢复 |

---

## 四、运行测试

```bash
# 单元测试
make test-short

# 集成测试 (需要 BACnet 模拟器)
go test -v -run TestBACnetManualAdd ./internal/driver/bacnet/
go test -v -run TestBACnetScan ./internal/driver/bacnet/

# 端到端测试 (需要远程设备)
go test -v -run TestDriverWorkflow ./internal/driver/bacnet/
```

---

## 五、关键验证点

1. **手动添加优先**：`discoverDevice` 先调用 `locateDeviceAddress` (ReadProperty)，失败后 WhoIs，最后直连回退
2. **端口分离**：连接端口 47809（长连接），发现端口 47808（临时客户端）
3. **MaxPDU**：所有 ClientBuilder 设置 MaxPDU=1476
4. **超时隔离**：每设备独立 3s 超时，3 次失败触发隔离
5. **ObjectName 富化**：Scan 结果通过 ReadProperty(Object_Name) 获取真实设备名称（10s 超时）