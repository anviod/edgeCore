<template>
  <a-modal
    v-model:visible="visible"
    title="edgeOS 通信协议帮助"
    width="800px"
    modal-class="northbound-help-modal"
    :footer="false"
    @cancel="handleCancel"
  >
    <a-tabs default-active-key="mqtt">
      <a-tab-pane key="mqtt" title="edgeOS(MQTT)">
        <div class="nb-help-doc">
          <header class="nb-help-hero">
            <h4 class="nb-help-hero__title">概述</h4>
            <p class="nb-help-hero__lead">
              edgeOS(MQTT) 北向通道，将 edgeCore 数据上报至 edgeOS 蜂群网络。完整协议见
              <a href="/docs/edgeos/edgeCore通信协议规范(MQTT-NATS).html" target="_blank" class="nb-help-link">edgeCore 通信协议规范</a>。
            </p>
          </header>

          <div class="nb-help-block">
            <div class="nb-help-block-title">消息主题 (Topics)</div>
            <div class="nb-help-table-wrap">
              <table class="nb-help-table">
                <thead>
                  <tr>
                    <th>Topic</th>
                    <th>方向</th>
                    <th>说明</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>edgeCore/nodes/register</code></td>
                    <td>edgeCore → EdgeOS</td>
                    <td>节点注册</td>
                  </tr>
                  <tr>
                    <td><code>edgeCore/data/{node_id}/{device_id}</code></td>
                    <td>edgeCore → EdgeOS</td>
                    <td>实时数据上报</td>
                  </tr>
                  <tr>
                    <td><code>edgeCore/nodes/{node_id}/heartbeat</code></td>
                    <td>edgeCore → EdgeOS</td>
                    <td>节点心跳</td>
                  </tr>
                  <tr>
                    <td><code>edgeCore/cmd/{node_id}/discover</code></td>
                    <td>EdgeOS → edgeCore</td>
                    <td>设备发现命令</td>
                  </tr>
                  <tr>
                    <td><code>edgeCore/cmd/{node_id}/{device_id}/write</code></td>
                    <td>EdgeOS → edgeCore</td>
                    <td>写入设备数据</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div class="nb-help-block">
            <div class="nb-help-block-title">消息格式</div>
            <pre class="nb-help-pre"><code>{
  "header": {
    "message_id": "msg-001",
    "timestamp": 1744680000000,
    "source": "edgeCore-node-001",
    "message_type": "data",
    "version": "1.0"
  },
  "body": {
    "node_id": "edgeCore-node-001",
    "device_id": "device-001",
    "timestamp": 1744680000000,
    "points": {
      "Temperature": 25.5,
      "Humidity": 65.2
    },
    "quality": "good"
  }
}</code></pre>
          </div>
        </div>
      </a-tab-pane>

      <a-tab-pane key="nats" title="edgeOS(NATS)">
        <div class="nb-help-doc">
          <header class="nb-help-hero">
            <h4 class="nb-help-hero__title">概述</h4>
            <p class="nb-help-hero__lead">
              edgeOS(NATS) 北向通道，Subject 命名与 MQTT 版对应（<code>.</code> 分隔）。协议细节见
              <a href="/docs/edgeos/edgeCore通信协议规范(MQTT-NATS).html" target="_blank" class="nb-help-link">edgeCore 通信协议规范</a>。
            </p>
          </header>

          <div class="nb-help-block">
            <div class="nb-help-block-title">消息主题 (Subjects)</div>
            <div class="nb-help-table-wrap">
              <table class="nb-help-table">
                <thead>
                  <tr>
                    <th>Subject</th>
                    <th>方向</th>
                    <th>说明</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>edgeCore.nodes.register</code></td>
                    <td>edgeCore → EdgeOS</td>
                    <td>节点注册</td>
                  </tr>
                  <tr>
                    <td><code>edgeCore.data.{node_id}.{device_id}</code></td>
                    <td>edgeCore → EdgeOS</td>
                    <td>实时数据上报</td>
                  </tr>
                  <tr>
                    <td><code>edgeCore.nodes.{node_id}.heartbeat</code></td>
                    <td>edgeCore → EdgeOS</td>
                    <td>节点心跳</td>
                  </tr>
                  <tr>
                    <td><code>edgeCore.cmd.{node_id}.discover</code></td>
                    <td>EdgeOS → edgeCore</td>
                    <td>设备发现命令</td>
                  </tr>
                  <tr>
                    <td><code>edgeCore.cmd.{node_id}.{device_id}.write</code></td>
                    <td>EdgeOS → edgeCore</td>
                    <td>写入设备数据</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div class="nb-help-block">
            <div class="nb-help-block-title">通配符</div>
            <ul class="nb-help-list">
              <li><code>*</code> - 匹配单个 token</li>
              <li><code>></code> - 匹配一个或多个 tokens</li>
            </ul>
          </div>

          <div class="nb-help-block">
            <div class="nb-help-block-title">示例</div>
            <pre class="nb-help-pre"><code>// 订阅所有设备的写入命令
edgeCore.cmd.edgeCore-node-001.*.write

// 订阅所有数据上报
edgeCore.data.>></code></pre>
          </div>
        </div>
      </a-tab-pane>

      <a-tab-pane key="ean" title="EAN 2.0">
        <div class="nb-help-doc">
          <header class="nb-help-hero">
            <h4 class="nb-help-hero__title">概述</h4>
            <p class="nb-help-hero__lead">
              EAN 2.0（Edge Agent Network）能力层复用 edgeOS(MQTT/NATS) 北向通道作为传输层，以
              <code>$edgeos/*</code> 为主题空间承载 Agent 注册、能力发现、远程调用与事件发布。
              edgeCore 节点作为 Agent 加入蜂群网络，EdgeOS 可远程发现并调用本设备能力。协议细节见
              <a href="/docs/edgeos/AI协同组件规划.html" target="_blank" class="nb-help-link">AI 协同组件规划</a>。
            </p>
          </header>

          <a-alert class="nb-help-alert" type="info" show-icon>
            <template #title>启用条件</template>
            需在通道配置的「EAN 能力层」中开启 <code>ean_enabled</code>，并按需设置心跳间隔与事件自动发布；
            启停与心跳配置已合并到北向通道弹窗，不单独创建 MQTT/NATS 客户端。
          </a-alert>

          <div class="nb-help-block">
            <div class="nb-help-block-title">消息主题 (Topics)</div>
            <div class="nb-help-table-wrap">
              <table class="nb-help-table">
                <thead>
                  <tr>
                    <th>Topic</th>
                    <th>方向</th>
                    <th>说明</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>$edgeos/discovery/agent</code></td>
                    <td>edgeCore → EdgeOS</td>
                    <td>Agent 注册 / 上线公告</td>
                  </tr>
                  <tr>
                    <td><code>$edgeos/discovery/agent/offline</code></td>
                    <td>edgeCore → EdgeOS</td>
                    <td>Agent 离线公告</td>
                  </tr>
                  <tr>
                    <td><code>$edgeos/discovery/capability</code></td>
                    <td>edgeCore → EdgeOS</td>
                    <td>Capability 能力公告</td>
                  </tr>
                  <tr>
                    <td><code>$edgeos/discovery/query</code></td>
                    <td>EdgeOS → edgeCore</td>
                    <td>能力发现查询</td>
                  </tr>
                  <tr>
                    <td><code>$edgeos/discovery/response</code></td>
                    <td>edgeCore → EdgeOS</td>
                    <td>发现结果响应</td>
                  </tr>
                  <tr>
                    <td><code>$edgeos/invoke/{agent_id}</code></td>
                    <td>EdgeOS → edgeCore</td>
                    <td>能力调用请求</td>
                  </tr>
                  <tr>
                    <td><code>$edgeos/invoke/{agent_id}/status</code></td>
                    <td>EdgeOS → edgeCore</td>
                    <td>异步调用状态查询</td>
                  </tr>
                  <tr>
                    <td><code>$edgeos/reply/{agent_id}</code></td>
                    <td>edgeCore → EdgeOS</td>
                    <td>调用执行结果</td>
                  </tr>
                  <tr>
                    <td><code>$edgeos/event/{agent_id}</code></td>
                    <td>edgeCore → EdgeOS</td>
                    <td>事件上报 / 状态变化</td>
                  </tr>
                  <tr>
                    <td><code>$edgeos/event/broadcast</code></td>
                    <td>edgeCore → EdgeOS</td>
                    <td>事件广播</td>
                  </tr>
                  <tr>
                    <td><code>$edgeos/heartbeat/{agent_id}</code></td>
                    <td>edgeCore → EdgeOS</td>
                    <td>Agent 心跳（QoS 0）</td>
                  </tr>
                  <tr>
                    <td><code>$edgeos/state/{agent_id}</code></td>
                    <td>edgeCore → EdgeOS</td>
                    <td>设备状态同步</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div class="nb-help-block">
            <div class="nb-help-block-title">流程细节</div>

            <div class="nb-flow">
              <div class="nb-flow__head">
                <span class="nb-flow__actor">edgeCore Agent</span>
                <span class="nb-flow__headline">① 启动与注册流程</span>
                <span class="nb-flow__actor">EdgeOS 蜂群</span>
              </div>
              <div class="nb-flow__row">
                <span class="nb-flow__note">连接 MQTT/NATS 北向通道</span>
                <span class="nb-flow__mid">
                  <i class="nb-flow__arrow nb-flow__arrow--right"></i>
                  <span class="nb-flow__topic">通道连接</span>
                </span>
                <span class="nb-flow__note">就绪并鉴权</span>
              </div>
              <div class="nb-flow__row">
                <span class="nb-flow__note">发布 Agent 注册</span>
                <span class="nb-flow__mid">
                  <i class="nb-flow__arrow nb-flow__arrow--right"></i>
                  <code class="nb-flow__topic">$edgeos/discovery/agent</code>
                </span>
                <span class="nb-flow__note">记录 Agent 上线</span>
              </div>
              <div class="nb-flow__row">
                <span class="nb-flow__note">发布 Capability 公告</span>
                <span class="nb-flow__mid">
                  <i class="nb-flow__arrow nb-flow__arrow--right"></i>
                  <code class="nb-flow__topic">$edgeos/discovery/capability</code>
                </span>
                <span class="nb-flow__note">收录能力清单</span>
              </div>
              <div class="nb-flow__row">
                <span class="nb-flow__note">周期心跳</span>
                <span class="nb-flow__mid">
                  <i class="nb-flow__arrow nb-flow__arrow--right"></i>
                  <code class="nb-flow__topic">$edgeos/heartbeat/{agent_id}</code>
                </span>
                <span class="nb-flow__note">更新在线状态</span>
              </div>
            </div>

            <div class="nb-flow">
              <div class="nb-flow__head">
                <span class="nb-flow__actor">edgeCore Agent</span>
                <span class="nb-flow__headline">② 能力发现与调用流程</span>
                <span class="nb-flow__actor">EdgeOS 蜂群</span>
              </div>
              <div class="nb-flow__row">
                <span class="nb-flow__note">收到发现查询</span>
                <span class="nb-flow__mid">
                  <i class="nb-flow__arrow nb-flow__arrow--left"></i>
                  <code class="nb-flow__topic">$edgeos/discovery/query</code>
                </span>
                <span class="nb-flow__note">发起能力发现</span>
              </div>
              <div class="nb-flow__row">
                <span class="nb-flow__note">返回发现结果</span>
                <span class="nb-flow__mid">
                  <i class="nb-flow__arrow nb-flow__arrow--right"></i>
                  <code class="nb-flow__topic">$edgeos/discovery/response</code>
                </span>
                <span class="nb-flow__note">保存能力清单</span>
              </div>
              <div class="nb-flow__row">
                <span class="nb-flow__note">收到调用请求</span>
                <span class="nb-flow__mid">
                  <i class="nb-flow__arrow nb-flow__arrow--left"></i>
                  <code class="nb-flow__topic">$edgeos/invoke/{agent_id}</code>
                </span>
                <span class="nb-flow__note">下发能力调用</span>
              </div>
              <div class="nb-flow__row">
                <span class="nb-flow__note">执行 Capability 并回复</span>
                <span class="nb-flow__mid">
                  <i class="nb-flow__arrow nb-flow__arrow--right"></i>
                  <code class="nb-flow__topic">$edgeos/reply/{agent_id}</code>
                </span>
                <span class="nb-flow__note">获取调用结果</span>
              </div>
              <div class="nb-flow__row">
                <span class="nb-flow__note">状态变化发布事件</span>
                <span class="nb-flow__mid">
                  <i class="nb-flow__arrow nb-flow__arrow--right"></i>
                  <code class="nb-flow__topic">$edgeos/event/{agent_id}</code>
                </span>
                <span class="nb-flow__note">订阅 / 消费事件</span>
              </div>
            </div>

            <div class="nb-help-hint">
              心跳（QoS 0）与查询（QoS 0）使用 QoS 0；发现 / 调用 / 事件 / 回复默认 QoS 1。
              调用为异步模型：请求走 <code>invoke</code>，结果经 <code>reply</code> 返回，
              异步场景可经 <code>invoke/.../status</code> 查询执行状态。
            </div>
          </div>

          <div class="nb-help-block">
            <div class="nb-help-block-title">消息格式（invoke_capability）</div>
            <pre class="nb-help-pre"><code>{
  "header": {
    "message_id": "msg-001",
    "timestamp": 1744680000000,
    "source": "edgeCore-node-001",
    "destination": "edgeCore-node-001",
    "message_type": "invoke_capability",
    "version": "2.0",
    "correlation_id": "req-001"
  },
  "body": {
    "invoke_id": "inv-001",
    "target": "edgeCore-node-001",
    "capability": "device.read_points",
    "arguments": { "device_id": "device-001" },
    "options": { "timeout_sec": 10, "async": false }
  }
}</code></pre>
          </div>

          <div class="nb-help-block">
            <div class="nb-help-block-title">配置项</div>
            <div class="nb-help-table-wrap">
              <table class="nb-help-table">
                <thead>
                  <tr>
                    <th>配置项</th>
                    <th>类型</th>
                    <th>说明</th>
                    <th>默认值</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td>ean_enabled</td>
                    <td>bool</td>
                    <td>启用 EAN 能力层</td>
                    <td>false</td>
                  </tr>
                  <tr>
                    <td>ean_heartbeat_sec</td>
                    <td>int</td>
                    <td>Agent 心跳间隔（秒）</td>
                    <td>60</td>
                  </tr>
                  <tr>
                    <td>ean_event_auto_publish</td>
                    <td>bool</td>
                    <td>设备状态变化时自动发布 EAN Event</td>
                    <td>false</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </a-tab-pane>

      <a-tab-pane key="config" title="配置说明">
        <div class="nb-help-doc">
          <div class="nb-help-block">
            <div class="nb-help-block-title">edgeOS(MQTT) 配置项</div>
            <div class="nb-help-table-wrap">
              <table class="nb-help-table">
                <thead>
                  <tr>
                    <th>配置项</th>
                    <th>类型</th>
                    <th>说明</th>
                    <th>默认值</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td>broker</td>
                    <td>string</td>
                    <td>MQTT Broker 地址</td>
                    <td>tcp://127.0.0.1:1883</td>
                  </tr>
                  <tr>
                    <td>client_id</td>
                    <td>string</td>
                    <td>MQTT 客户端 ID</td>
                    <td>edgeCore-node-001</td>
                  </tr>
                  <tr>
                    <td>node_id</td>
                    <td>string</td>
                    <td>节点唯一标识</td>
                    <td>edgeCore-node-001</td>
                  </tr>
                  <tr>
                    <td>qos</td>
                    <td>byte</td>
                    <td>QoS 级别 (0/1/2)</td>
                    <td>1</td>
                  </tr>
                  <tr>
                    <td>keep_alive</td>
                    <td>int</td>
                    <td>心跳间隔(秒)</td>
                    <td>60</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div class="nb-help-block">
            <div class="nb-help-block-title">edgeOS(NATS) 配置项</div>
            <div class="nb-help-table-wrap">
              <table class="nb-help-table">
                <thead>
                  <tr>
                    <th>配置项</th>
                    <th>类型</th>
                    <th>说明</th>
                    <th>默认值</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td>url</td>
                    <td>string</td>
                    <td>NATS 服务器 URL</td>
                    <td>nats://127.0.0.1:4222</td>
                  </tr>
                  <tr>
                    <td>client_id</td>
                    <td>string</td>
                    <td>NATS 客户端名称</td>
                    <td>edgeCore-node-001</td>
                  </tr>
                  <tr>
                    <td>jetstream_enabled</td>
                    <td>bool</td>
                    <td>是否启用 JetStream</td>
                    <td>false</td>
                  </tr>
                  <tr>
                    <td>max_reconnects</td>
                    <td>int</td>
                    <td>最大重连次数</td>
                    <td>10</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </a-tab-pane>
    </a-tabs>
  </a-modal>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  visible: Boolean
})

const emit = defineEmits(['update:visible'])

const visible = computed({
  get: () => props.visible,
  set: (value) => emit('update:visible', value)
})

const handleCancel = () => {
  emit('update:visible', false)
}
</script>
