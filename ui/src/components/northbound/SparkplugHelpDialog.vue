<template>
  <a-modal v-model:visible="visible" title="Sparkplug B 接入文档" :width="900" :footer="false" modal-class="northbound-help-modal" unmount-on-close>
    <a-tabs v-model:active-key="activeTab" type="line">
      <a-tab-pane key="overview" title="协议概述">
        <div class="nb-help-tab">
          <header class="nb-help-hero">
            <h4 class="nb-help-hero__title">Sparkplug B 协议</h4>
            <p class="nb-help-hero__lead">
              基于 MQTT 的工业协议；Topic 结构 <code>namespace/group_id/message_type/edge_node_id/[device_id]</code>，Payload 为 Protobuf 编码。
              完整配置与 API 见
              <a href="/docs/API/Northbound_Configuration_CN.html" target="_blank" class="nb-help-link">北向配置 API</a>、
              <a href="/docs/northbound/MQTT数据上下行格式.html" target="_blank" class="nb-help-link">MQTT 数据上下行格式</a>。
            </p>
          </header>

          <div class="nb-help-block">
            <div class="nb-help-block-title">消息类型</div>
            <a-table :data="messageTypes" :bordered="false" size="small" :pagination="false">
              <template #columns>
                <a-table-column title="类型" data-index="type" :width="120" />
                <a-table-column title="说明" data-index="description" />
              </template>
            </a-table>
          </div>
        </div>
      </a-tab-pane>

      <a-tab-pane key="publish" title="数据上报">
        <div class="nb-help-tab">
          <header class="nb-help-hero">
            <h4 class="nb-help-hero__title">数据上报 (Data Publishing)</h4>
            <p class="nb-help-hero__lead">
              设备数据通过 NBIRTH/DBIRTH 消息上报出生声明，通过 NDATA/DDATA 消息上报数据更新。
              Topic 与 Payload 字段详见
              <a href="/docs/northbound/MQTT数据上下行格式.html" target="_blank" class="nb-help-link">MQTT 数据上下行格式</a>。
            </p>
          </header>

          <div class="nb-help-topic-card nb-help-topic-card--primary">
            <div class="nb-help-topic-card__body">
              <div class="nb-help-topic-card__label">NBIRTH Topic (节点出生)</div>
              <div class="nb-help-code-row">
                <span class="nb-help-code-row__text">
                  spBv1.0/{{ groupId || '采集通' }}/NBIRTH/{{ nodeId || 'edgeCore' }}
                </span>
                <a-button type="text" size="mini" @click="copyToClipboard(`spBv1.0/${groupId || '采集通'}/NBIRTH/${nodeId || 'edgeCore'}`)">
                  <template #icon><icon-copy :size="12" /></template>
                </a-button>
              </div>
            </div>
          </div>

          <div class="nb-help-topic-card nb-help-topic-card--blue">
            <div class="nb-help-topic-card__body">
              <div class="nb-help-topic-card__label">DBIRTH Topic (设备出生)</div>
              <div class="nb-help-code-row">
                <span class="nb-help-code-row__text">
                  spBv1.0/{{ groupId || '采集通' }}/DBIRTH/{{ nodeId || 'edgeCore' }}/{{ deviceId }}
                </span>
                <a-button type="text" size="mini" @click="copyToClipboard(`spBv1.0/${groupId || '采集通'}/DBIRTH/${nodeId || 'edgeCore'}/${deviceId}`)">
                  <template #icon><icon-copy :size="12" /></template>
                </a-button>
              </div>
            </div>
          </div>

          <div class="nb-help-block">
            <div class="nb-help-block-title">Payload 结构 (Protobuf)</div>
            <pre class="nb-help-pre">{
  "timestamp": 1678888888888,
  "metrics": [
    {
      "name": "temperature",
      "value": 25.5,
      "type": "Float"
    }
  ],
  "seq": 0
}</pre>
          </div>
        </div>
      </a-tab-pane>

      <a-tab-pane key="command" title="设备控制">
        <div class="nb-help-tab">
          <header class="nb-help-hero">
            <h4 class="nb-help-hero__title">设备控制 (Device Control)</h4>
            <p class="nb-help-hero__lead">
              通过 NCMD/DCMD 消息向设备发送控制指令。
            </p>
          </header>

          <div class="nb-help-topic-card nb-help-topic-card--orange">
            <div class="nb-help-topic-card__body">
              <div class="nb-help-topic-card__label">NCMD Topic (节点命令)</div>
              <div class="nb-help-code-row">
                <span class="nb-help-code-row__text">
                  spBv1.0/{{ groupId || '采集通' }}/NCMD/{{ nodeId || 'edgeCore' }}
                </span>
                <a-button type="text" size="mini" @click="copyToClipboard(`spBv1.0/${groupId || '采集通'}/NCMD/${nodeId || 'edgeCore'}`)">
                  <template #icon><icon-copy :size="12" /></template>
                </a-button>
              </div>
            </div>
          </div>

          <div class="nb-help-topic-card nb-help-topic-card--green">
            <div class="nb-help-topic-card__body">
              <div class="nb-help-topic-card__label">DCMD Topic (设备命令)</div>
              <div class="nb-help-code-row">
                <span class="nb-help-code-row__text">
                  spBv1.0/{{ groupId || '采集通' }}/DCMD/{{ nodeId || 'edgeCore' }}/{{ deviceId }}
                </span>
                <a-button type="text" size="mini" @click="copyToClipboard(`spBv1.0/${groupId || '采集通'}/DCMD/${nodeId || 'edgeCore'}/${deviceId}`)">
                  <template #icon><icon-copy :size="12" /></template>
                </a-button>
              </div>
            </div>
          </div>

          <div class="nb-help-block">
            <div class="nb-help-block-title">命令 Payload (Protobuf)</div>
            <pre class="nb-help-pre">{
  "timestamp": 1678888888888,
  "metrics": [
    {
      "name": "setpoint",
      "value": 100,
      "type": "Int32"
    }
  ],
  "seq": 1
}</pre>
          </div>
        </div>
      </a-tab-pane>

      <a-tab-pane key="status" title="状态管理">
        <div class="nb-help-tab">
          <header class="nb-help-hero">
            <h4 class="nb-help-hero__title">状态管理 (State Management)</h4>
            <p class="nb-help-hero__lead">
              Sparkplug B 通过 NDEATH/DDEATH 消息管理设备离线状态，支持遗嘱消息 (LWT)。
            </p>
          </header>

          <div class="nb-help-topic-card nb-help-topic-card--red">
            <div class="nb-help-topic-card__body">
              <div class="nb-help-topic-card__label">NDEATH Topic (节点死亡)</div>
              <div class="nb-help-code-row">
                <span class="nb-help-code-row__text">
                  spBv1.0/{{ groupId || '采集通' }}/NDEATH/{{ nodeId || 'edgeCore' }}
                </span>
                <a-button type="text" size="mini" @click="copyToClipboard(`spBv1.0/${groupId || '采集通'}/NDEATH/${nodeId || 'edgeCore'}`)">
                  <template #icon><icon-copy :size="12" /></template>
                </a-button>
              </div>
            </div>
          </div>

          <div class="nb-help-topic-card nb-help-topic-card--purple">
            <div class="nb-help-topic-card__body">
              <div class="nb-help-topic-card__label">DDEATH Topic (设备死亡)</div>
              <div class="nb-help-code-row">
                <span class="nb-help-code-row__text">
                  spBv1.0/{{ groupId || '采集通' }}/DDEATH/{{ nodeId || 'edgeCore' }}/{{ deviceId }}
                </span>
                <a-button type="text" size="mini" @click="copyToClipboard(`spBv1.0/${groupId || '采集通'}/DDEATH/${nodeId || 'edgeCore'}/${deviceId}`)">
                  <template #icon><icon-copy :size="12" /></template>
                </a-button>
              </div>
            </div>
          </div>

          <a-alert type="warning" class="nb-help-alert">
            NDEATH 消息应配置为 MQTT 遗嘱消息 (LWT)，确保异常断开时能自动发布。
          </a-alert>

          <div class="nb-help-block">
            <div class="nb-help-block-title">状态 Payload (Protobuf)</div>
            <pre class="nb-help-pre">{
  "timestamp": 1678888888888,
  "seq": 255
}</pre>
          </div>
        </div>
      </a-tab-pane>
    </a-tabs>
  </a-modal>
</template>

<script setup>
import { ref, computed } from 'vue'
import { IconCopy } from '@arco-design/web-vue/es/icon'
import { showMessage } from '@/composables/useGlobalState'

const props = defineProps({
  visible: { type: Boolean, default: false },
  groupId: { type: String, default: '采集通道' },
  nodeId: { type: String, default: '' },
  deviceId: { type: String, default: 'device_1' }
})

const emit = defineEmits(['update:visible'])

const visible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val)
})
const activeTab = ref('overview')

const messageTypes = ref([
  { type: 'NBIRTH', description: '节点出生声明，包含节点所有指标初始值' },
  { type: 'DBIRTH', description: '设备出生声明，包含设备所有指标初始值' },
  { type: 'NDATA', description: '节点数据更新，仅包含变化的指标' },
  { type: 'DDATA', description: '设备数据更新，仅包含变化的指标' },
  { type: 'NCMD', description: '节点命令，向节点发送控制指令' },
  { type: 'DCMD', description: '设备命令，向设备发送控制指令' },
  { type: 'NDEATH', description: '节点死亡通知，通常作为遗嘱消息' },
  { type: 'DDEATH', description: '设备死亡通知' }
])

const copyToClipboard = (text) => {
  if (!text) return
  navigator.clipboard.writeText(text).then(() => {
    showMessage('已复制到剪贴板', 'success')
  }).catch(() => {
    showMessage('复制失败', 'error')
  })
}
</script>
