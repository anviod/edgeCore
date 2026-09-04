---
layout: default
---

# edgeOS 北向通道部署清单

## 部署前检查

### 1. 代码检查
- [x] 后端代码实现完成
- [x] 前端组件创建完成
- [x] API 端点定义完成
- [x] 配置模型定义完成
- [ ] 运行 `go mod tidy` 更新依赖
- [ ] 运行 `go build` 编译检查
- [ ] 运行单元测试

### 2. 依赖检查
- [ ] NATS 依赖已安装: `github.com/nats-io/nats v1.31.0`
- [ ] MQTT 依赖已安装: `github.com/eclipse/paho.mqtt.golang v1.5.1`
- [ ] 前端依赖检查

### 3. 环境准备
- [ ] MQTT Broker 已部署
- [ ] NATS Server 已部署
- [ ] 网络连通性检查

## 部署步骤

### 步骤 1: 更新依赖
```bash
cd /path/to/edgeCore   # 仓库根目录
go mod tidy
go mod download
```

### 步骤 2: 编译检查
```bash
# 编译后端
go build -o gateway.exe cmd/main.go

# 检查编译结果
# 如果成功，继续；如果失败，修复错误
```

### 步骤 3: 运行测试
```bash
# 运行所有测试
go test ./...

# 或仅测试 edgeOS 相关
go test ./internal/northbound/edgos_mqtt/...
go test ./internal/northbound/edgos_nats/...
```

### 步骤 4: 前端构建
```bash
cd ui
npm install
npm run build
```

### 步骤 5: 配置更新
```bash
# 复制示例配置
cp conf/edgeos.example.yaml conf/edgeos.yaml

# 根据实际环境修改配置
# 编辑 conf/edgeos.yaml
```

### 步骤 6: 启动服务
```bash
# 启动 edgeCore 网关
./gateway.exe

# 或使用脚本
./gateway.sh start
```

## 功能验证

### 1. 基础连接验证

#### edgeOS(MQTT)
```bash
# 检查连接状态
curl http://localhost:8082/api/northbound/config

# 预期输出中包含 edgeos_mqtt 数组
```

#### edgeOS(NATS)
```bash
# 检查连接状态
curl http://localhost:8082/api/northbound/config

# 预期输出中包含 edgeos_nats 数组
```

### 2. 消息收发验证

#### MQTT 测试
```bash
# 订阅消息
mosquitto_sub -h 127.0.0.1 -p 1883 -t "edgeCore/#" -v

# 应该看到:
# - edgeCore/nodes/register (节点注册)
# - edgeCore/nodes/{node_id}/heartbeat (心跳)
# - edgeCore/data/{node_id}/{device_id} (数据上报)
```

#### NATS 测试
```bash
# 订阅消息
nats sub "edgeCore.>"

# 应该看到:
# - edgeCore.nodes.register (节点注册)
# - edgeCore.nodes.{node_id}.heartbeat (心跳)
# - edgeCore.data.{node_id}.{device_id} (数据上报)
```

### 3. 命令响应验证

#### 设备发现命令
```bash
# MQTT
mosquitto_pub -h 127.0.0.1 -p 1883 \
  -t "edgeCore/cmd/edgeCore-node-001/discover" \
  -m '{"header":{"message_type":"discover_command",...},"body":{...}}'

# 应该在响应主题收到响应
# edgeCore/responses/edgeCore-node-001/{message_id}
```

#### 写入命令
```bash
# MQTT
mosquitto_pub -h 127.0.0.1 -p 1883 \
  -t "edgeCore/cmd/edgeCore-node-001/device-001/write" \
  -m '{"header":{"message_type":"write_command",...},"body":{"points":{...}}}'

# 应该在响应主题收到响应
# edgeCore/responses/edgeCore-node-001/{message_id}
```

### 4. 统计信息验证

```bash
# 获取 edgeOS(MQTT) 统计
curl http://localhost:8082/api/northbound/edgeos-mqtt/{id}/stats

# 预期输出:
{
  "success_count": 1234,
  "fail_count": 5,
  "reconnect_count": 2,
  "publish_count": 1239,
  "last_offline_time": 0,
  "last_online_time": 1744680000000
}
```

## 性能验证

### 1. 压力测试
```bash
# 使用 MQTT 压力测试工具
# mosquitto_pub -t "edgeCore/data/node-001/device-001" -l -p 10

# 使用 NATS 压力测试工具
# nats bench publish "edgeCore.data.node-001.device-001" -p 10
```

### 2. 监控指标
- CPU 使用率
- 内存使用率
- 网络流量
- 消息发送速率
- 消息接收速率
- 重连次数

## 常见问题排查

### 问题 1: 无法连接到 MQTT Broker
```bash
# 检查 Broker 是否运行
docker ps | grep mosquitto

# 检查端口
netstat -an | grep 1883

# 查看 MQTT Broker 日志
docker logs mosquitto
```

### 问题 2: 无法连接到 NATS Server
```bash
# 检查 Server 是否运行
docker ps | grep nats

# 检查端口
netstat -an | grep 4222

# 查看 NATS Server 日志
docker logs nats
```

### 问题 3: 消息丢失
- 检查 QoS 级别设置
- 检查网络稳定性
- 查看 edgeCore 网关日志
- 检查设备映射配置

### 问题 4: 前端无法加载
```bash
# 检查 API 可访问性
curl http://localhost:8082/api/northbound/config

# 检查浏览器控制台错误
# 检查网络请求

# 清除浏览器缓存
```

## 回滚方案

### 1. 配置回滚
```bash
# 备份当前配置
cp conf/system.yaml conf/system.yaml.backup

# 如果出现问题，恢复配置
cp conf/system.yaml.backup conf/system.yaml

# 重启服务
./gateway.sh restart
```

### 2. 代码回滚
```bash
# Git 回滚
git log --oneline
git revert <commit-id>

# 重新部署
go build
./gateway.sh restart
```

## 监控与告警

### 1. 关键指标
- 连接状态 (0/1/2/3)
- 消息发送成功率
- 消息接收成功率
- 平均延迟
- 重连次数
- 错误率

### 2. 告警阈值
- 连接断开 > 1 分钟
- 错误率 > 5%
- 重连次数 > 10/分钟
- 消息延迟 > 1 秒

### 3. 日志监控
```bash
# 实时查看日志
tail -f logs/edgeCore-gateway.edgeCore.log | grep edgeOS

# 过滤错误
tail -f logs/edgeCore-gateway.edgeCore.log | grep "error\|Error\|ERROR"
```

## 验收标准

### 基础功能
- [ ] edgeOS(MQTT) 可以成功连接
- [ ] edgeOS(NATS) 可以成功连接
- [ ] 可以发送节点注册消息
- [ ] 可以发送心跳消息
- [ ] 可以上报实时数据
- [ ] 可以接收设备发现命令
- [ ] 可以接收写入命令
- [ ] 可以响应命令

### 稳定性
- [ ] 自动重连功能正常
- [ ] 连接断开后可以自动恢复
- [ ] 消息不会丢失
- [ ] 长时间运行稳定

### 性能
- [ ] 消息发送延迟 < 100ms
- [ ] 可以支持 1000+ msg/s
- [ ] CPU 使用率 < 50%
- [ ] 内存使用率 < 500MB

### 易用性
- [ ] Web UI 可以配置 edgeOS
- [ ] 可以查看连接状态
- [ ] 可以查看统计信息
- [ ] 可以删除配置
- [ ] 可以更新配置
- [ ] 有完整的帮助文档

## 部署后支持

### 1. 文档
- [ ] 部署文档完整
- [ ] API 文档完整
- [ ] 故障排查文档完整
- [ ] 常见问题文档完整

### 2. 支持
- [ ] 用户手册
- [ ] 视频教程
- [ ] 示例代码
- [ ] 技术支持联系方式

## 完成确认

### 开发完成
- [x] 后端代码完成
- [x] 前端组件完成
- [x] API 端点完成
- [x] 文档完成

### 测试完成
- [ ] 单元测试完成
- [ ] 集成测试完成
- [ ] 功能测试完成
- [ ] 性能测试完成

### 部署完成
- [ ] 依赖更新完成
- [ ] 编译检查通过
- [ ] 环境部署完成
- [ ] 功能验证通过
- [ ] 性能验证通过

## 签字确认

- [ ] 开发人员: _______________ 日期: ______
- [ ] 测试人员: _______________ 日期: ______
- [ ] 运维人员: _______________ 日期: ______
- [ ] 产品负责人: _______________ 日期: ______
