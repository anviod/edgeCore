<template>
  <div v-if="deviceKind === 'real'" class="nb-report-strategy-panel">
    <div class="table-header table-header--strategy">
      <div class="table-header__hint">
        <span>启用设备默认周期上报 {{ defaultInterval }}</span>
        <span class="nb-strategy-hint">周期上报＝全量上报 · 变化上报＝增量上报</span>
      </div>
      <div class="table-header__actions">
        <a-button type="outline" size="small" @click="batchEnableDevices('real')">批量启用</a-button>
        <a-input
          v-model="batchInterval"
          size="small"
          :placeholder="defaultInterval"
          class="mono-text batch-interval-input"
          @press-enter="batchSetInterval('real')"
        />
        <a-button type="outline" size="small" @click="batchSetInterval('real')">批量设置周期</a-button>
        <a-dropdown trigger="click" @select="(key) => batchSetStrategy('real', key)">
          <a-button type="outline" size="small">
            批量上报模式
            <template #icon><icon-down :size="12" /></template>
          </a-button>
          <template #content>
            <a-doption value="periodic">周期上报（全量）</a-doption>
            <a-doption value="change">变化上报（增量）</a-doption>
          </template>
        </a-dropdown>
      </div>
    </div>
    <div class="table-container saas-table nb-device-table">
      <a-table
        ref="realTableRef"
        row-key="id"
        :columns="realDeviceColumns"
        :data="realDeviceTableData"
        :row-selection="realRowSelection"
        @selection-change="onRealSelectionChange"
        :virtual-list-props="realVirtualListProps"
        :scroll="realTableScroll"
        size="small"
        :bordered="false"
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
          <a-switch v-model="record._enable" size="small" @change="updateRealDeviceEnable(record)" />
        </template>
        <template #strategy="{ record }">
          <a-select
            v-model="record._strategy"
            size="small"
            :disabled="!record._enable"
            class="mono-text strategy-select"
            @change="updateRealDeviceStrategy(record)"
          >
            <a-option value="periodic">周期上报</a-option>
            <a-option value="change">变化上报</a-option>
          </a-select>
        </template>
        <template #interval="{ record }">
          <a-input
            v-if="record._strategy === 'periodic'"
            v-model="record._interval"
            size="small"
            :disabled="!record._enable"
            :placeholder="defaultInterval"
            class="mono-text strategy-interval-input"
            @change="updateRealDeviceInterval(record)"
          />
          <span v-else class="strategy-interval-dash">—</span>
        </template>
      </a-table>
    </div>
  </div>

  <div v-else class="nb-report-strategy-panel">
    <div class="table-header table-header--strategy">
      <div class="table-header__hint">
        <span>启用虚拟影子设备默认周期上报 {{ defaultInterval }}</span>
        <span class="nb-strategy-hint">周期上报＝全量上报 · 变化上报＝增量上报</span>
      </div>
      <div class="table-header__actions">
        <a-button type="outline" size="small" @click="batchEnableDevices('virtual')">批量启用</a-button>
        <a-input
          v-model="virtualBatchInterval"
          size="small"
          :placeholder="defaultInterval"
          class="mono-text batch-interval-input"
          @press-enter="batchSetInterval('virtual')"
        />
        <a-button type="outline" size="small" @click="batchSetInterval('virtual')">批量设置周期</a-button>
        <a-dropdown trigger="click" @select="(key) => batchSetStrategy('virtual', key)">
          <a-button type="outline" size="small">
            批量上报模式
            <template #icon><icon-down :size="12" /></template>
          </a-button>
          <template #content>
            <a-doption value="periodic">周期上报（全量）</a-doption>
            <a-doption value="change">变化上报（增量）</a-doption>
          </template>
        </a-dropdown>
      </div>
    </div>
    <div class="table-container saas-table nb-device-table">
      <a-table
        ref="virtualTableRef"
        row-key="id"
        :columns="virtualDeviceColumns"
        :data="virtualDeviceTableData"
        :row-selection="virtualRowSelection"
        @selection-change="onVirtualSelectionChange"
        :virtual-list-props="virtualVirtualListProps"
        :scroll="virtualTableScroll"
        size="small"
        :bordered="false"
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
            @change="updateVirtualDeviceEnable(record)"
          />
        </template>
        <template #strategy="{ record }">
          <a-select
            v-model="record._strategy"
            size="small"
            :disabled="!record._enable || !record.enable"
            class="mono-text strategy-select"
            @change="updateVirtualDeviceStrategy(record)"
          >
            <a-option value="periodic">周期上报</a-option>
            <a-option value="change">变化上报</a-option>
          </a-select>
        </template>
        <template #interval="{ record }">
          <a-input
            v-if="record._strategy === 'periodic'"
            v-model="record._interval"
            size="small"
            :disabled="!record._enable || !record.enable"
            :placeholder="defaultInterval"
            class="mono-text strategy-interval-input"
            @change="updateVirtualDeviceInterval(record)"
          />
          <span v-else class="strategy-interval-dash">—</span>
        </template>
      </a-table>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, nextTick } from 'vue'
import { IconDown } from '@arco-design/web-vue/es/icon'
import { showMessage } from '@/composables/useGlobalState'
import { listVirtualShadows } from '@/api/virtualShadow'
import {
  buildNorthboundDeviceRows,
  buildNorthboundVirtualDeviceStrategyRows
} from '@/utils/southboundDevices'

const props = defineProps({
  visible: { type: Boolean, default: false },
  deviceKind: { type: String, default: 'real', validator: (v) => ['real', 'virtual'].includes(v) },
  allDevices: { type: Array, default: () => [] },
  devices: { type: Object, default: () => ({}) },
  virtualDevices: { type: Object, default: () => ({}) },
  defaultInterval: { type: String, default: '10s' }
})

const emit = defineEmits(['update:devices', 'update:virtualDevices'])

const realDeviceTableData = ref([])
const virtualDeviceTableData = ref([])
const batchInterval = ref(props.defaultInterval)
const virtualBatchInterval = ref(props.defaultInterval)

const realSelectedKeys = ref([])
const virtualSelectedKeys = ref([])
const realTableRef = ref(null)
const virtualTableRef = ref(null)

// Arco 受控选择：由表格内部维护勾选状态（点击即时响应，避免受控回传延迟），
// 通过 selection-change 事件同步到外部用于批量操作
const realRowSelection = computed(() => ({
  type: 'checkbox',
  showCheckedAll: true,
  onlyCurrent: false
}))

const virtualRowSelection = computed(() => ({
  type: 'checkbox',
  showCheckedAll: true,
  onlyCurrent: false
}))

const onRealSelectionChange = (keys) => {
  realSelectedKeys.value = keys || []
}

const onVirtualSelectionChange = (keys) => {
  virtualSelectedKeys.value = keys || []
}

// 大列表时启用虚拟滚动：仅渲染可视行，避免勾选 100+ 行时整表重渲染导致的卡顿
const VIRTUAL_ROW_THRESHOLD = 50
const virtualListConfig = { height: 320, estimatedSize: 40, buffer: 8 }

const realVirtualListProps = computed(() =>
  realDeviceTableData.value.length > VIRTUAL_ROW_THRESHOLD ? virtualListConfig : undefined
)
const realTableScroll = computed(() =>
  realDeviceTableData.value.length > VIRTUAL_ROW_THRESHOLD ? { y: 320 } : undefined
)

const virtualVirtualListProps = computed(() =>
  virtualDeviceTableData.value.length > VIRTUAL_ROW_THRESHOLD ? virtualListConfig : undefined
)
const virtualTableScroll = computed(() =>
  virtualDeviceTableData.value.length > VIRTUAL_ROW_THRESHOLD ? { y: 320 } : undefined
)

const realDeviceColumns = [
  { title: '设备', dataIndex: 'name', width: 180, ellipsis: true, tooltip: true },
  { title: '通道', dataIndex: 'channelName', width: 120 },
  { title: '状态', slotName: 'state', width: 80, align: 'center' },
  { title: '启用', slotName: 'enable', width: 70, align: 'center' },
  { title: '策略', slotName: 'strategy', width: 130 },
  { title: '上报周期', slotName: 'interval', width: 100 }
]

const virtualDeviceColumns = [
  { title: '设备', slotName: 'name', width: 160, ellipsis: true, tooltip: true },
  { title: '归属通道', dataIndex: 'channel_id', width: 110, ellipsis: true, tooltip: true },
  { title: '点位', dataIndex: 'pointCount', width: 60, align: 'center' },
  { title: '配置', slotName: 'configEnable', width: 70, align: 'center' },
  { title: '启用', slotName: 'enable', width: 70, align: 'center' },
  { title: '策略', slotName: 'strategy', width: 120 },
  { title: '上报周期', slotName: 'interval', width: 90 }
]

const buildRealDeviceTable = () => {
  realDeviceTableData.value = buildNorthboundDeviceRows(
    props.allDevices,
    props.devices,
    props.defaultInterval
  )
}

const buildVirtualDeviceTable = async () => {
  try {
    const res = await listVirtualShadows()
    const items = Array.isArray(res) ? res : (res?.data || [])
    virtualDeviceTableData.value = buildNorthboundVirtualDeviceStrategyRows(
      items,
      props.virtualDevices,
      props.defaultInterval
    )
  } catch (e) {
    console.error('[NorthboundReportStrategy] load virtual shadows failed', e)
    virtualDeviceTableData.value = buildNorthboundVirtualDeviceStrategyRows(
      [],
      props.virtualDevices,
      props.defaultInterval
    )
  }
}

const rebuildTables = async () => {
  batchInterval.value = props.defaultInterval
  virtualBatchInterval.value = props.defaultInterval
  buildRealDeviceTable()
  await buildVirtualDeviceTable()
  await nextTick()
  // 表格内部维护勾选状态，重建后需同步清空（同时通过 selection-change 清空外部引用）
  realTableRef.value?.selectAll?.(false)
  virtualTableRef.value?.selectAll?.(false)
  realSelectedKeys.value = []
  virtualSelectedKeys.value = []
}

onMounted(() => {
  if (props.visible) {
    rebuildTables()
  }
})

watch(() => props.visible, async (val) => {
  if (!val) return
  await rebuildTables()
})

watch(() => props.allDevices, async () => {
  if (props.visible) buildRealDeviceTable()
}, { deep: true })

watch(() => props.devices, () => {
  if (props.visible) buildRealDeviceTable()
}, { deep: true })

watch(() => props.virtualDevices, async () => {
  if (props.visible) await buildVirtualDeviceTable()
}, { deep: true })

const syncRealRecordToForm = (record) => {
  const next = { ...props.devices }
  next[record.id] = {
    enable: record._enable,
    strategy: record._strategy,
    interval: record._interval || props.defaultInterval
  }
  emit('update:devices', next)
}

const syncVirtualRecordToForm = (record) => {
  const next = { ...props.virtualDevices }
  next[record.id] = {
    enable: record._enable,
    strategy: record._strategy,
    interval: record._interval || props.defaultInterval
  }
  emit('update:virtualDevices', next)
}

const updateRealDeviceEnable = (record) => {
  if (record._enable) {
    record._strategy = 'periodic'
    record._interval = record._interval || props.defaultInterval
  }
  const current = props.devices[record.id]
  if (!current || typeof current === 'boolean') {
    syncRealRecordToForm(record)
  } else {
    const next = { ...props.devices }
    next[record.id] = { ...current, enable: record._enable }
    if (record._enable) {
      next[record.id].strategy = 'periodic'
      next[record.id].interval = record._interval
    }
    emit('update:devices', next)
  }
}

const updateRealDeviceStrategy = (record) => {
  if (record._strategy === 'periodic' && !record._interval) {
    record._interval = props.defaultInterval
  }
  const current = props.devices[record.id]
  if (!current || typeof current === 'boolean') {
    syncRealRecordToForm(record)
  } else {
    const next = { ...props.devices }
    next[record.id] = { ...current, strategy: record._strategy }
    if (record._strategy === 'periodic') {
      next[record.id].interval = record._interval
    }
    emit('update:devices', next)
  }
}

const updateRealDeviceInterval = (record) => {
  record._interval = record._interval || props.defaultInterval
  const current = props.devices[record.id]
  if (!current || typeof current === 'boolean') {
    syncRealRecordToForm(record)
  } else {
    const next = { ...props.devices }
    next[record.id] = { ...current, interval: record._interval }
    emit('update:devices', next)
  }
}

const updateVirtualDeviceEnable = (record) => {
  if (!record.enable) {
    record._enable = false
    return
  }
  if (record._enable) {
    record._strategy = 'periodic'
    record._interval = record._interval || props.defaultInterval
  }
  syncVirtualRecordToForm(record)
}

const updateVirtualDeviceStrategy = (record) => {
  if (record._strategy === 'periodic' && !record._interval) {
    record._interval = props.defaultInterval
  }
  syncVirtualRecordToForm(record)
}

const updateVirtualDeviceInterval = (record) => {
  record._interval = record._interval || props.defaultInterval
  syncVirtualRecordToForm(record)
}

const getSelectedRows = (kind) => {
  const isVirtual = kind === 'virtual'
  const keys = isVirtual ? virtualSelectedKeys.value : realSelectedKeys.value
  const rows = isVirtual ? virtualDeviceTableData.value : realDeviceTableData.value
  return rows.filter(record => keys.includes(record.id))
}

const batchEnableDevices = (kind) => {
  const isVirtual = kind === 'virtual'
  const rows = getSelectedRows(kind)
  if (!rows.length) {
    showMessage('没有勾选设备，请先勾选要启用的设备', 'warning')
    return
  }
  let count = 0
  rows.forEach(record => {
    if (isVirtual && !record.enable) return
    record._enable = true
    record._strategy = 'periodic'
    record._interval = record._interval || props.defaultInterval
    if (isVirtual) syncVirtualRecordToForm(record)
    else syncRealRecordToForm(record)
    count++
  })
  if (!count) {
    showMessage('所选设备无法启用（虚拟设备配置为禁用）', 'warning')
    return
  }
  showMessage(`已启用 ${count} 个设备，周期 ${props.defaultInterval}`, 'success')
}

const batchSetInterval = (kind) => {
  const isVirtual = kind === 'virtual'
  const interval = ((isVirtual ? virtualBatchInterval.value : batchInterval.value) || props.defaultInterval).trim()
  if (!interval) {
    showMessage('请输入上报周期', 'warning')
    return
  }
  const rows = getSelectedRows(kind)
  if (!rows.length) {
    showMessage('没有勾选设备，请先勾选要设置周期的设备', 'warning')
    return
  }
  let count = 0
  rows.forEach(record => {
    if (isVirtual && !record.enable) return
    // 联动：设置周期时自动启用并切到周期上报
    record._enable = true
    record._strategy = 'periodic'
    record._interval = interval
    if (isVirtual) syncVirtualRecordToForm(record)
    else syncRealRecordToForm(record)
    count++
  })
  if (count === 0) {
    showMessage('所选设备无法设置周期（虚拟设备配置为禁用）', 'warning')
    return
  }
  showMessage(`已为 ${count} 个设备启用并设置周期 ${interval}`, 'success')
}

// 批量上报模式：联动生效 —— 自动启用所选设备，切换模式并应用批量周期
const batchSetStrategy = (kind, strategy) => {
  const isVirtual = kind === 'virtual'
  const rows = getSelectedRows(kind)
  if (!rows.length) {
    showMessage('没有勾选设备，请先勾选要设置上报模式的设备', 'warning')
    return
  }
  const isPeriodic = strategy === 'periodic'
  const intervalInput = ((isVirtual ? virtualBatchInterval.value : batchInterval.value) || props.defaultInterval).trim()
  const applyInterval = intervalInput || props.defaultInterval
  let count = 0
  rows.forEach(record => {
    if (isVirtual && !record.enable) return
    // 联动：切换模式时自动启用所选设备
    record._enable = true
    record._strategy = strategy
    if (isPeriodic) {
      record._interval = applyInterval
    }
    if (isVirtual) syncVirtualRecordToForm(record)
    else syncRealRecordToForm(record)
    count++
  })
  if (count === 0) {
    showMessage('所选设备无法设置上报模式（虚拟设备配置为禁用）', 'warning')
    return
  }
  showMessage(
    `已为 ${count} 个设备设置为${isPeriodic ? `周期上报（全量），周期 ${applyInterval}` : '变化上报（增量）'}`,
    'success'
  )
}

defineExpose({ rebuildTables })
</script>

<style scoped>
/* v3.0 — styles in src/styles/northbound-form.css */
</style>
