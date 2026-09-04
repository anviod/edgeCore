<template>
  <a-modal
    v-model:visible="visible"
    :title="isEdit ? '编辑 edgeOS (MQTT)' : '新增 edgeOS (MQTT)'"
    :width="960"
    modal-class="northbound-settings-modal"
    unmount-on-close
    :footer="true"
    :mask-closable="false"
  >
    <div class="nb-mode-banner nb-mode-banner--push">
      <span class="nb-mode-banner__tag">主动上报</span>
      <span>edgeOS 平台 MQTT 3.1.1，节点注册与双向通信</span>
    </div>

    <a-tabs v-model:active-key="activeTab" type="rounded" size="small">
      <a-tab-pane key="basic">
        <template #title>连接配置</template>
        <a-form :model="form" layout="vertical" class="industrial-form form-controls-md">
          <a-row :gutter="16">
            <a-col :span="16">
              <a-form-item label="通道名称" required>
                <a-input v-model="form.name" placeholder="例如: edgeOS MQTT 生产通道" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item label="启用"><a-switch v-model="form.enable" /></a-form-item>
            </a-col>
          </a-row>

          <div class="nb-form-section">
            <div class="nb-form-section__title">Broker 连接</div>
            <a-form-item label="Broker 地址" required>
              <a-input v-model="form.broker" placeholder="tcp://127.0.0.1:1883" class="mono-text" />
            </a-form-item>
            <a-row :gutter="16">
              <a-col :span="12">
                <a-form-item label="Client ID" required>
                  <a-input v-model="form.client_id" placeholder="edgeCore-node-001" class="mono-text" />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item label="节点 ID" required>
                  <a-input v-model="form.node_id" placeholder="edgeCore-node-001" class="mono-text" />
                </a-form-item>
              </a-col>
            </a-row>
            <a-row :gutter="16">
              <a-col :span="12">
                <a-form-item label="用户名"><a-input v-model="form.username" placeholder="可选" /></a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item label="密码"><a-input-password v-model="form.password" placeholder="可选" /></a-form-item>
              </a-col>
            </a-row>
          </div>

          <div class="nb-form-section">
            <div class="nb-form-section__title">MQTT 选项</div>
            <a-row :gutter="16">
              <a-col :span="12">
                <a-form-item label="QoS 级别">
                  <a-select v-model="form.qos">
                    <a-option :value="0">0 - 最多一次</a-option>
                    <a-option :value="1">1 - 至少一次</a-option>
                    <a-option :value="2">2 - 恰好一次</a-option>
                  </a-select>
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item label="心跳周期">
                  <a-input v-model="form.heartbeat_interval" placeholder="30s" class="mono-text" />
                </a-form-item>
              </a-col>
            </a-row>
            <a-row :gutter="16">
              <a-col :span="8">
                <a-form-item label="心跳间隔 (秒)">
                  <a-input-number v-model="form.keep_alive" :min="10" :max="3600" placeholder="60" />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item label="保留消息"><a-switch v-model="form.retain" /></a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item label="自动重连"><a-switch v-model="form.auto_reconnect" /></a-form-item>
              </a-col>
            </a-row>
          </div>
        </a-form>
      </a-tab-pane>

      <a-tab-pane key="real-devices">
        <template #title>上报真实设备</template>
        <NorthboundReportStrategyPanel
          device-kind="real"
          :visible="visible"
          :all-devices="localDevices.length ? localDevices : allDevices"
          v-model:devices="form.devices"
          v-model:virtual-devices="form.virtual_devices"
        />
      </a-tab-pane>

      <a-tab-pane key="virtual-devices">
        <template #title>上报虚拟设备</template>
        <NorthboundReportStrategyPanel
          device-kind="virtual"
          :visible="visible"
          :all-devices="localDevices.length ? localDevices : allDevices"
          v-model:devices="form.devices"
          v-model:virtual-devices="form.virtual_devices"
        />
      </a-tab-pane>

      <!-- EAN 2.0 能力层 / EAN Capability Layer -->
      <a-tab-pane key="ean">
        <template #title>EAN 能力层</template>
        <a-form :model="form" layout="vertical" class="industrial-form form-controls-md">
          <div class="nb-form-section">
            <div class="nb-form-section__title">EAN 2.0 能力层</div>
            <!-- EAN 启用开关 / EAN Enable Switch -->
            <a-form-item label="启用 EAN 能力层">
              <a-switch v-model="form.ean_enabled" />
              <div class="form-hint">启用后此通道将承载 EAN 2.0 Agent 注册、能力发现和远程调用</div>
            </a-form-item>
            <!-- 心跳间隔 / Heartbeat Interval -->
            <a-form-item label="心跳间隔 (秒)">
              <a-input-number v-model="form.ean_heartbeat_sec" :min="10" :max="600" :step="10" style="width:180px" />
              <div class="form-hint">Agent 向 EdgeOS 发送心跳的间隔，默认 60 秒</div>
            </a-form-item>
            <!-- 事件自动发布 / Event Auto Publish -->
            <a-form-item label="事件自动发布">
              <a-switch v-model="form.ean_event_auto_publish" />
              <div class="form-hint">设备数据变化时自动发布 EAN Event</div>
            </a-form-item>
          </div>
        </a-form>
      </a-tab-pane>
    </a-tabs>

    <template #footer>
      <div class="industrial-modal-footer">
        <a-button @click="visible = false">取消</a-button>
        <a-button type="primary" :loading="loading" @click="saveSettings">保存</a-button>
      </div>
    </template>
  </a-modal>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import request from '@/utils/request'
import { fetchAllSouthboundDevices } from '@/utils/southboundDevices'
import {
  closeNorthboundSettingsDialog,
  extractNorthboundSaveWarning,
  northboundSaveRequestConfig,
  notifyNorthboundSaveError,
  notifyNorthboundSaveSuccess,
  notifyNorthboundValidationError,
  validateNorthboundChannelName
} from '@/utils/northboundSave'
import NorthboundReportStrategyPanel from '@/components/northbound/NorthboundReportStrategyPanel.vue'

const props = defineProps({
  visible: { type: Boolean, default: false },
  config: { type: Object, default: null },
  allDevices: { type: Array, default: () => [] },
  northboundConfig: { type: Object, default: () => ({}) }
})

const emit = defineEmits(['update:visible', 'saved'])

const visible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val)
})

const loading = ref(false)
const form = ref({})
const activeTab = ref('basic')
const localDevices = ref([])
const isNewMode = ref(false)

const isEdit = computed(() => props.config && props.config.id)

const initForm = () => {
  activeTab.value = 'basic'
  isNewMode.value = !props.config
  if (props.config) {
    form.value = JSON.parse(JSON.stringify(props.config))
  } else {
    form.value = {
      id: 'edgeos-mqtt_' + Date.now(),
      enable: true,
      name: 'New edgeOS MQTT',
      broker: 'tcp://127.0.0.1:1883',
      client_id: 'edgeCore-node-001',
      node_id: 'edgeCore-node-001',
      username: '',
      password: '',
      qos: 1,
      retain: false,
      keep_alive: 60,
      auto_reconnect: true,
      heartbeat_interval: '30s',
      devices: {},
      virtual_devices: {},
      ean_enabled: false,
      ean_heartbeat_sec: 60,
      ean_event_auto_publish: false
    }
  }
  if (!form.value.devices) form.value.devices = {}
  if (!form.value.virtual_devices) form.value.virtual_devices = {}
}

const resolveDevices = async () => {
  if (props.allDevices?.length) {
    localDevices.value = props.allDevices
    return
  }
  try {
    localDevices.value = await fetchAllSouthboundDevices(request)
  } catch (e) {
    console.error('Failed to fetch southbound devices', e)
    localDevices.value = []
  }
}

watch(() => props.visible, async (val) => {
  if (!val) return
  initForm()
  await resolveDevices()
})

watch(() => props.allDevices, (devs) => {
  if (!props.visible || !devs?.length) return
  localDevices.value = devs
}, { deep: true })

const saveSettings = async () => {
  const missing = []
  if (!form.value.name?.trim()) missing.push('通道名称')
  if (!form.value.broker?.trim()) missing.push('Broker 地址')
  if (!form.value.client_id?.trim()) missing.push('Client ID')
  if (!form.value.node_id?.trim()) missing.push('节点 ID')
  if (missing.length) {
    notifyNorthboundValidationError('请填写必填项：' + missing.join('、'))
    activeTab.value = 'basic'
    return
  }

  const nameError = validateNorthboundChannelName(form.value.name, form.value.id, props.northboundConfig)
  if (nameError) {
    notifyNorthboundValidationError(nameError)
    activeTab.value = 'basic'
    return
  }

  loading.value = true
  try {
    const res = await request.post('/api/northbound/edgeos-mqtt', { ...form.value }, northboundSaveRequestConfig)
    notifyNorthboundSaveSuccess('edgeOS (MQTT)', isNewMode.value, extractNorthboundSaveWarning(res))
    closeNorthboundSettingsDialog(emit)
    emit('saved')
  } catch (e) {
    notifyNorthboundSaveError(e, 'edgeOS (MQTT)')
  } finally {
    loading.value = false
  }
}
</script>
