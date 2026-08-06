<template>
  <a-modal
    v-model:visible="visible"
    title="BACnet 服务端"
    :width="720"
    modal-class="northbound-settings-modal"
    unmount-on-close
    :footer="true"
    :mask-closable="false"
  >
    <div class="nb-mode-banner nb-mode-banner--passive">
      <span class="nb-mode-banner__tag">被动读取</span>
      <span>网关作为 BACnet Server 从机运行，BMS / SCADA 连接后读取点位数据或写入控制指令</span>
    </div>

    <a-tabs v-model:active-key="activeTab" type="rounded" size="small">
      <a-tab-pane key="basic">
        <template #title>服务配置</template>
        <a-form :model="form" layout="vertical" class="industrial-form form-controls-md">
          <a-row :gutter="16">
            <a-col :span="16">
              <a-form-item label="通道名称" required>
                <a-input v-model="form.name" placeholder="例如：工厂 BMS BACnet Server" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item label="启用">
                <a-switch v-model="form.enable" />
              </a-form-item>
            </a-col>
          </a-row>
          <a-row :gutter="16">
            <a-col :span="16">
              <a-form-item
                label="通道 ID"
                extra="仅允许英文字母、数字、下划线或横线，禁止空格；点击「生成」一键生成"
              >
                <a-input
                  v-model="form.id"
                  placeholder="bacnet_xxx"
                  class="mono-text"
                  @input="onChannelIdInput"
                >
                  <template #suffix>
                    <a-button type="text" size="mini" @click="generateChannelId">
                      <template #icon><icon-refresh /></template>生成
                    </a-button>
                  </template>
                </a-input>
              </a-form-item>
            </a-col>
            <a-col :span="8" />
          </a-row>

          <div class="nb-form-section">
            <div class="nb-form-section__title">网络配置</div>
            <a-row :gutter="16">
              <a-col :span="8">
                <a-form-item label="端口" required>
                  <a-input-number v-model="form.port" :min="1" :max="65535" placeholder="47808" style="width: 100%" />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item label="子网 CIDR">
                  <a-input-number v-model="form.subnet_cidr" :min="8" :max="30" placeholder="24" style="width: 100%" />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item label="最大 PDU">
                  <a-input-number v-model="form.max_pdu" :min="128" :max="65535" placeholder="1476" style="width: 100%" />
                </a-form-item>
              </a-col>
            </a-row>
            <a-row :gutter="16">
              <a-col :span="12">
                <a-form-item label="网络接口">
                  <a-input v-model="form.interface" placeholder="eth0（留空自动选择）" />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item label="绑定 IP">
                  <a-input v-model="form.ip" placeholder="192.168.1.100（留空自动选择）" />
                </a-form-item>
              </a-col>
            </a-row>
            <a-alert type="info" style="margin-bottom: 12px">
              BACnet 标准端口 <code class="mono-text">47808</code> (0xBAC0)，与南向 BACnet 驱动端口 47809 分离避免冲突
            </a-alert>
          </div>

          <div class="nb-form-section">
            <div class="nb-form-section__title">设备标识</div>
            <a-row :gutter="16">
              <a-col :span="8">
                <a-form-item label="设备实例 ID">
                  <a-input-number v-model="form.device_id" :min="0" :max="4194303" placeholder="自动" style="width: 100%" />
                  <template #extra>0 表示自动生成</template>
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item label="设备名称">
                  <a-input v-model="form.device_name" placeholder="edgeCore-Gateway" />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item label="厂商 ID">
                  <a-input-number v-model="form.vendor_id" :min="0" :max="65535" placeholder="999" style="width: 100%" />
                </a-form-item>
              </a-col>
            </a-row>
            <a-row :gutter="16">
              <a-col :span="12">
                <a-form-item label="厂商名称">
                  <a-input v-model="form.vendor_name" placeholder="edgeCore Foundry" />
                </a-form-item>
              </a-col>
            </a-row>
          </div>
        </a-form>
      </a-tab-pane>

      <a-tab-pane key="real-devices">
        <template #title>映射真实设备</template>
        <div class="table-header">
          <span class="table-header__hint">选择在 BACnet 地址空间中暴露的南向采集设备</span>
          <a-button type="outline" size="small" @click="autoFillDevices">
            <template #icon><icon-check /></template>全部启用
          </a-button>
        </div>
        <div class="table-container saas-table nb-device-table">
          <a-table
            row-key="id"
            :columns="deviceColumns"
            :data="deviceTableData"
            size="small"
            :pagination="false"
            class="industrial-table-inline"
          >
            <template #empty>
              <a-empty description="暂无南向设备，请先在通道管理中创建设备" />
            </template>
            <template #state="{ record }">
              <a-tag v-if="record.state === 0" color="green" size="small">在线</a-tag>
              <a-tag v-else-if="record.state === 1" color="orangered" size="small">不稳定</a-tag>
              <a-tag v-else color="red" size="small">离线</a-tag>
            </template>
            <template #enable="{ record }">
              <a-switch v-model="record._enable" size="small" />
            </template>
          </a-table>
        </div>
      </a-tab-pane>

      <a-tab-pane key="virtual-devices">
        <template #title>映射虚拟设备</template>
        <div class="table-header">
          <span class="table-header__hint">选择在 BACnet 地址空间中暴露的虚拟影子设备</span>
          <a-button type="outline" size="small" @click="autoFillVirtualDevices">
            <template #icon><icon-check /></template>全部启用
          </a-button>
        </div>
        <div class="table-container saas-table nb-device-table">
          <a-table
            row-key="id"
            :columns="virtualDeviceColumns"
            :data="virtualDeviceTableData"
            size="small"
            :pagination="false"
            class="industrial-table-inline"
          >
            <template #empty>
              <a-empty description="暂无虚拟影子设备，请先在虚拟影子页面创建" />
            </template>
            <template #name="{ record }">
              <span>{{ record.name || record.id }}</span>
            </template>
            <template #configEnable="{ record }">
              <a-tag :color="record.enable ? 'green' : 'gray'" size="small">
                {{ record.enable ? '启用' : '禁用' }}
              </a-tag>
            </template>
            <template #enable="{ record }">
              <a-switch
                v-model="record._enable"
                size="small"
                :disabled="!record.enable"
              />
            </template>
          </a-table>
        </div>
      </a-tab-pane>
    </a-tabs>

    <template #footer>
      <div class="industrial-modal-footer">
        <a-button v-if="form.id" type="outline" :loading="syncing" @click="syncPointMapping" class="btn-secondary">
          <template #icon><icon-sync /></template>同步点位映射
        </a-button>
        <div style="flex: 1" />
        <a-button @click="visible = false" class="btn-secondary">取消</a-button>
        <a-button type="primary" :loading="loading" @click="saveSettings" class="btn-primary">
          <template #icon><icon-plus /></template>保存通道配置
        </a-button>
      </div>
    </template>
  </a-modal>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { IconPlus, IconCheck, IconSync, IconRefresh } from '@arco-design/web-vue/es/icon'
import { Message } from '@arco-design/web-vue'
import { showMessage } from '@/composables/useGlobalState'
import request from '@/utils/request'
import {
  closeNorthboundSettingsDialog,
  extractNorthboundSaveWarning,
  generateNorthboundID,
  northboundSaveRequestConfig,
  notifyNorthboundSaveError,
  notifyNorthboundSaveSuccess,
  notifyNorthboundValidationError,
  resolveNorthboundSaveError,
  sanitizeNorthboundID,
  validateNorthboundChannelName,
  validateNorthboundID
} from '@/utils/northboundSave'
import { buildNorthboundVirtualDeviceRows, syncNorthboundVirtualDevicesFromRows } from '@/utils/southboundDevices'
import { listVirtualShadows } from '@/api/virtualShadow'

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
const syncing = ref(false)
const form = ref({})
const deviceTableData = ref([])
const virtualDeviceTableData = ref([])
const activeTab = ref('basic')
const isNewMode = ref(false)

const deviceColumns = [
  { title: '设备名称', dataIndex: 'name' },
  { title: '采集通道', dataIndex: 'channelName', width: 120 },
  { title: '状态', slotName: 'state', width: 80, align: 'center' },
  { title: '暴露', slotName: 'enable', width: 70, align: 'center' }
]

const virtualDeviceColumns = [
  { title: '设备名称', slotName: 'name', width: 180, ellipsis: true, tooltip: true },
  { title: '归属通道', dataIndex: 'channel_id', width: 120, ellipsis: true, tooltip: true },
  { title: '点位', dataIndex: 'pointCount', width: 70, align: 'center' },
  { title: '配置', slotName: 'configEnable', width: 80, align: 'center' },
  { title: '暴露', slotName: 'enable', width: 70, align: 'center' }
]

watch(() => props.visible, async (val) => {
  if (val) {
    activeTab.value = 'basic'
    isNewMode.value = !props.config
    if (props.config) {
      form.value = JSON.parse(JSON.stringify(props.config))
    } else {
      form.value = {
        enable: true,
        id: generateNorthboundID('bacnet'),
        name: 'New BACnet Server',
        interface: '',
        ip: '',
        port: 47808,
        subnet_cidr: 24,
        device_id: 0,
        device_name: '',
        vendor_id: 999,
        vendor_name: '',
        max_pdu: 1476,
        devices: {},
        virtual_devices: {}
      }
    }
    if (!form.value.devices) form.value.devices = {}
    if (!form.value.virtual_devices) form.value.virtual_devices = {}

    buildDeviceTable()
    await buildVirtualDeviceTable()
  }
})

const buildDeviceTable = () => {
  const allowAll = !form.value.devices || Object.keys(form.value.devices).length === 0
  deviceTableData.value = props.allDevices.map(dev => {
    const current = form.value.devices[dev.id]
    let _enable = allowAll
    if (current === undefined || current === null) {
      _enable = allowAll
    } else if (typeof current === 'boolean') {
      _enable = current
    } else if (typeof current === 'object') {
      _enable = !!current.enable
    }
    return { ...dev, _enable }
  })
}

const buildVirtualDeviceTable = async () => {
  try {
    const res = await listVirtualShadows()
    const items = Array.isArray(res) ? res : (res?.data || [])
    virtualDeviceTableData.value = buildNorthboundVirtualDeviceRows(items, form.value.virtual_devices)
  } catch (e) {
    console.error('[BacnetSettings] load virtual shadows failed', e)
    virtualDeviceTableData.value = buildNorthboundVirtualDeviceRows([], form.value.virtual_devices)
  }
}

const syncDevicesFromTable = () => {
  if (deviceTableData.value.length === 0) {
    form.value.devices = {}
    return
  }
  const devices = {}
  let allEnabled = true
  for (const record of deviceTableData.value) {
    if (!record._enable) {
      allEnabled = false
    }
    devices[record.id] = { enable: record._enable }
  }
  // 全部启用时使用空 map，保持默认"全部暴露"语义
  form.value.devices = allEnabled ? {} : devices
}

const syncVirtualDevicesFromTable = () => {
  form.value.virtual_devices = syncNorthboundVirtualDevicesFromRows(virtualDeviceTableData.value)
}

const autoFillDevices = () => {
  deviceTableData.value.forEach(record => {
    record._enable = true
  })
  showMessage('已启用全部真实设备', 'success')
}

const autoFillVirtualDevices = () => {
  virtualDeviceTableData.value.forEach(record => {
    if (record.enable) {
      record._enable = true
    }
  })
  showMessage('已启用全部虚拟设备', 'success')
}

const syncPointMapping = async () => {
  if (!form.value.id) {
    showMessage('请先保存通道配置', 'warning')
    return
  }
  if (syncing.value) return

  syncing.value = true
  try {
    await request.post(
      `/api/northbound/bacnet_server/${encodeURIComponent(form.value.id)}/sync`,
      null,
      northboundSaveRequestConfig
    )
    Message.success('BACnet 点位映射已同步，地址空间已更新')
  } catch (e) {
    Message.error('同步失败：' + resolveNorthboundSaveError(e))
  } finally {
    syncing.value = false
  }
}

// 一键生成合法通道 ID（仅英文字母/数字/下划线/横线）
const generateChannelId = () => {
  form.value.id = generateNorthboundID('bacnet')
}

// 输入通道 ID 时即时清洗，禁止空格等非法字符
const onChannelIdInput = (value) => {
  form.value.id = sanitizeNorthboundID(value)
}

const saveSettings = async () => {
  if (!form.value.name?.trim()) {
    notifyNorthboundValidationError('请填写通道名称')
    activeTab.value = 'basic'
    return
  }
  if (!form.value.port) {
    notifyNorthboundValidationError('请填写端口号')
    activeTab.value = 'basic'
    return
  }

  const idError = validateNorthboundID(form.value.id)
  if (idError) {
    notifyNorthboundValidationError(idError)
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
  syncDevicesFromTable()
  syncVirtualDevicesFromTable()

  try {
    const res = await request.post('/api/northbound/bacnet_server', form.value, northboundSaveRequestConfig)
    const warning = extractNorthboundSaveWarning(res)
    if (res?.id) {
      form.value.id = res.id
    }
    const { warning: _ignored, ...savedCfg } = res || {}
    Object.assign(form.value, savedCfg)
    notifyNorthboundSaveSuccess('BACnet 服务端', isNewMode.value, warning)
    closeNorthboundSettingsDialog(emit)
    emit('saved')
  } catch (e) {
    notifyNorthboundSaveError(e, 'BACnet 服务端')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
/* v3.0 — styles in src/styles/ */
</style>