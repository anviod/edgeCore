<template>
  <a-modal
    v-model:visible="visible"
    title="MQTT 客户端"
    :width="960"
    modal-class="northbound-settings-modal"
    unmount-on-close
    :footer="true"
    :mask-closable="false"
  >
    <div class="nb-mode-banner nb-mode-banner--push">
      <span class="nb-mode-banner__tag">主动上报</span>
      <span>网关连接 MQTT Broker，按配置策略主动推送采集数据</span>
    </div>

    <a-tabs v-model:active-key="activeTab" type="rounded" size="small">
      <a-tab-pane key="basic">
        <template #title>连接配置</template>
        <a-form :model="form" layout="vertical" class="industrial-form form-controls-md">
          <a-row :gutter="16">
            <a-col :span="16">
              <a-form-item label="通道名称" required>
                <a-input v-model="form.name" placeholder="例如: 云端生产环境 MQTT" />
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
                <a-form-item label="Client ID">
                  <a-input v-model="form.client_id" placeholder="edgeCore-01" class="mono-text">
                    <template #append>
                      <a-button type="text" size="mini" @click="autoFillTopics">生成推荐</a-button>
                    </template>
                  </a-input>
                </a-form-item>
              </a-col>
              <a-col :span="6">
                <a-form-item label="用户名"><a-input v-model="form.username" /></a-form-item>
              </a-col>
              <a-col :span="6">
                <a-form-item label="密码"><a-input-password v-model="form.password" /></a-form-item>
              </a-col>
            </a-row>
          </div>

          <div class="nb-form-section">
            <div class="nb-form-section__title">主题</div>
            <a-form-item label="上报主题 (Upstream)">
              <a-input v-model="form.topic" class="mono-text" />
            </a-form-item>
            <a-row :gutter="16">
              <a-col :span="12">
                <a-form-item label="订阅主题 (Downstream)">
                  <a-input v-model="form.subscribe_topic" placeholder="/things/{client_id}/write/req" class="mono-text" />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item label="写入响应主题">
                  <a-input v-model="form.write_response_topic" placeholder="默认: 订阅主题/resp" class="mono-text" />
                </a-form-item>
              </a-col>
            </a-row>
          </div>

          <a-collapse :bordered="false">
            <a-collapse-item header="设备状态与 LWT（可选）" key="advanced">
              <a-form-item label="状态上报主题"><a-input v-model="form.status_topic" class="mono-text" /></a-form-item>
              <a-form-item label="生命周期主题"><a-input v-model="form.device_lifecycle_topic" class="mono-text" /></a-form-item>
              <a-row :gutter="16">
                <a-col :span="12">
                  <a-form-item label="上线消息"><a-textarea v-model="form.online_payload" :auto-size="{ minRows: 2 }" class="mono-text" /></a-form-item>
                </a-col>
                <a-col :span="12">
                  <a-form-item label="离线消息"><a-textarea v-model="form.offline_payload" :auto-size="{ minRows: 2 }" class="mono-text" /></a-form-item>
                </a-col>
              </a-row>
              <a-form-item label="遗嘱主题"><a-input v-model="form.lwt_topic" placeholder="默认同状态主题" class="mono-text" /></a-form-item>
              <a-form-item label="遗嘱消息"><a-textarea v-model="form.lwt_payload" :auto-size="{ minRows: 2 }" class="mono-text" /></a-form-item>
              <a-checkbox v-model="form.ignore_offline_data">设备离线时不主动上报历史数据</a-checkbox>
            </a-collapse-item>
          </a-collapse>
        </a-form>
      </a-tab-pane>

      <a-tab-pane key="real-devices">
        <template #title>上报真实设备</template>
        <NorthboundReportStrategyPanel
          device-kind="real"
          :visible="visible"
          :all-devices="allDevices"
          v-model:devices="form.devices"
          v-model:virtual-devices="form.virtual_devices"
        />
      </a-tab-pane>

      <a-tab-pane key="virtual-devices">
        <template #title>上报虚拟设备</template>
        <NorthboundReportStrategyPanel
          device-kind="virtual"
          :visible="visible"
          :all-devices="allDevices"
          v-model:devices="form.devices"
          v-model:virtual-devices="form.virtual_devices"
        />
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
import { showMessage } from '@/composables/useGlobalState'
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
const isNewMode = ref(false)

watch(() => props.visible, (val) => {
  if (!val) return
  activeTab.value = 'basic'
  isNewMode.value = !props.config
  if (props.config) {
    form.value = JSON.parse(JSON.stringify(props.config))
  } else {
    form.value = {
      id: 'mqtt_' + Date.now(),
      enable: true,
      name: 'New MQTT',
      broker: 'tcp://127.0.0.1:1883',
      client_id: '',
      topic: '',
      subscribe_topic: '',
      write_response_topic: '',
      ignore_offline_data: false,
      device_lifecycle_topic: '',
      status_topic: '',
      online_payload: '',
      offline_payload: '',
      lwt_topic: '',
      lwt_payload: '',
      username: '',
      password: '',
      devices: {},
      virtual_devices: {}
    }
  }
  if (!form.value.devices) form.value.devices = {}
  if (!form.value.virtual_devices) form.value.virtual_devices = {}
})

const autoFillTopics = () => {
  if (!form.value.client_id) form.value.client_id = 'edgeCore'
  const root = 'things/{client_id}'
  form.value.topic = `${root}/up`
  form.value.subscribe_topic = `${root}/down/req`
  form.value.write_response_topic = `${root}/down/resp`
  form.value.status_topic = `${root}/{device_id}/status`
  form.value.device_lifecycle_topic = `${root}/lifecycle`
  form.value.lwt_topic = `${root}/lwt`
  form.value.online_payload = JSON.stringify({ status: 'online', device_id: '%device_id%', timestamp: '%timestamp%' })
  form.value.offline_payload = JSON.stringify({ status: 'offline', device_id: '%device_id%', timestamp: '%timestamp%' })
  form.value.lwt_payload = JSON.stringify({ status: 'lwt', device_id: '%device_id%', timestamp: '%timestamp%' })
  showMessage('已生成推荐主题', 'success')
}

const saveSettings = async () => {
  const missing = []
  if (!form.value.name?.trim()) missing.push('通道名称')
  if (!form.value.broker?.trim()) missing.push('Broker 地址')
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
    const res = await request.post('/api/northbound/mqtt', { ...form.value }, northboundSaveRequestConfig)
    notifyNorthboundSaveSuccess('MQTT 客户端', isNewMode.value, extractNorthboundSaveWarning(res))
    closeNorthboundSettingsDialog(emit)
    emit('saved')
  } catch (e) {
    notifyNorthboundSaveError(e, 'MQTT 客户端')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
/* v3.0 — styles in src/styles/ */
</style>
