<template>
  <div v-if="deviceKind === 'real'" class="nb-report-strategy-panel">
    <div class="table-header table-header--strategy">
      <div class="table-header__hint">
        <span>按通道树形启用设备，通道勾选＝批量启用该通道全部设备</span>
        <span class="nb-strategy-hint">周期上报＝全量上报 · 变化上报＝增量上报</span>
      </div>
      <div class="table-header__actions">
        <a-input
          v-model="batchInterval"
          size="small"
          :placeholder="defaultInterval"
          class="mono-text batch-interval-input"
          @press-enter="batchSetIntervalForEnabled()"
        />
        <a-button type="outline" size="small" @click="batchSetIntervalForEnabled()">批量设置周期</a-button>
        <a-dropdown trigger="click" @select="(key) => batchSetStrategyForEnabled(key)">
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
        row-key="rowKey"
        :columns="realDeviceColumns"
        :data="realDeviceTableData"
        :expanded-keys="expandedChannelKeys"
        @expand-change="onExpandChange"
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
        <template #name="{ record }">
          <span v-if="record.children" class="nb-tree-channel-name">
            <span class="nb-tree-channel-badge">{{ record.enabledCount }}/{{ record.count }} 已启用</span>
            {{ record.channelName }}
          </span>
          <span v-else>{{ record.name || record.id }}</span>
        </template>
        <template #channel="{ record }">
          <span v-if="record.children">—</span>
          <span v-else>{{ record.channelName }}</span>
        </template>
        <template #state="{ record }">
          <a-tag v-if="record.children" color="arcoblue" size="small">通道</a-tag>
          <a-tag v-else-if="record.state === 0" color="green" size="small">在线</a-tag>
          <a-tag v-else-if="record.state === 1" color="orangered" size="small">不稳定</a-tag>
          <a-tag v-else color="red" size="small">离线</a-tag>
        </template>
        <template #enable="{ record }">
          <!-- 父行为通道：一键启用/禁用该通道下全部设备 -->
          <a-checkbox
            v-if="record.children"
            :model-value="record.enabledCount > 0 && record.enabledCount >= record.count"
            :indeterminate="record.enabledCount > 0 && record.enabledCount < record.count"
            @change="(checked) => setChannelAll(record, checked)"
          />
          <a-switch v-else v-model="record._enable" size="small" @change="updateRealDeviceEnable(record)" />
        </template>
        <template #strategy="{ record }">
          <span v-if="record.children" class="nb-tree-dash">—</span>
          <a-select
            v-else
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
          <span v-if="record.children" class="nb-tree-dash">—</span>
          <a-input
            v-else-if="record._strategy === 'periodic'"
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
  buildNorthboundDeviceTree,
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

const virtualSelectedKeys = ref([])
const realTableRef = ref(null)
const virtualTableRef = ref(null)
const expandedChannelKeys = ref([])

const onExpandChange = (keys) => {
  expandedChannelKeys.value = keys || []
}

const onVirtualSelectionChange = (keys) => {
  virtualSelectedKeys.value = keys || []
}

const virtualRowSelection = computed(() => ({
  type: 'checkbox',
  showCheckedAll: true,
  onlyCurrent: false
}))

const VIRTUAL_ROW_THRESHOLD = 50
const virtualListConfig = { height: 320, estimatedSize: 40, buffer: 8 }

// 树形表格行数少（通道数+设备数），通常无需虚拟滚动；超大通道仍开启
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
  { title: '设备', slotName: 'name', width: 260, ellipsis: true, tooltip: true },
  { title: '通道', slotName: 'channel', width: 130, ellipsis: true, tooltip: true },
  { title: '状态', slotName: 'state', width: 84, align: 'center' },
  { title: '启用', slotName: 'enable', width: 70, align: 'center' },
  { title: '策略', slotName: 'strategy', width: 120 },
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
  realDeviceTableData.value = buildNorthboundDeviceTree(
    props.allDevices,
    props.devices,
    props.defaultInterval
  )
  // 每次构建表格默认全部展开，让用户一眼看到所有设备归属
  expandedChannelKeys.value = realDeviceTableData.value.map((r) => r.rowKey)
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
  virtualTableRef.value?.selectAll?.(false)
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
  updateChannelEnrollment(parentOf(record))
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

// 找到设备行所属的通道父行
const parentOf = (record) =>
  realDeviceTableData.value.find((g) => g.children.some((c) => c.rowKey === record.rowKey))

// 逐个联动父行启用计数
const updateChannelEnrollment = (channel) => {
  if (!channel) return
  const enabled = channel.children.filter((c) => c._enable).length
  channel.enabledCount = enabled
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

// 通道父勾选：启用/禁用该通道下全部设备
const setChannelAll = (channel, checked) => {
  const enable = !!checked
  const next = { ...props.devices }
  for (const child of channel.children) {
    child._enable = enable
    child._strategy = 'periodic'
    child._interval = child._interval || props.defaultInterval
    const cur = props.devices[child.id]
    if (!cur || typeof cur === 'boolean') {
      next[child.id] = {
        enable,
        strategy: 'periodic',
        interval: child._interval
      }
    } else {
      next[child.id] = { ...cur, enable, strategy: 'periodic', interval: child._interval }
    }
  }
  channel.enabledCount = enable ? channel.count : 0
  emit('update:devices', next)
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
  const keys = isVirtual ? virtualSelectedKeys.value : []
  const rows = isVirtual ? virtualDeviceTableData.value : []
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
    record._enable = true
    record._strategy = 'periodic'
    record._interval = interval
    if (isVirtual) syncVirtualRecordToForm(record)
    count++
  })
  if (count === 0) {
    showMessage('所选设备无法设置周期（虚拟设备配置为禁用）', 'warning')
    return
  }
  showMessage(`已为 ${count} 个设备启用并设置周期 ${interval}`, 'success')
}

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
    record._enable = true
    record._strategy = strategy
    if (isPeriodic) {
      record._interval = applyInterval
    }
    if (isVirtual) syncVirtualRecordToForm(record)
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

// 批量设置周期/模式作用域：真实设备树中批量作用于当前【已启用】设备，
// 避免树中误触整通道；批量启用以通道父行勾选为准。
const enabledLeafRows = () =>
  realDeviceTableData.value.flatMap((g) => g.children).filter((c) => c._enable)

const batchSetIntervalForEnabled = () => {
  const interval = (batchInterval.value || props.defaultInterval).trim()
  if (!interval) {
    showMessage('请输入上报周期', 'warning')
    return
  }
  const rows = enabledLeafRows()
  if (!rows.length) {
    showMessage('没有已启用的设备，请先在通道勾选启用设备', 'warning')
    return
  }
  rows.forEach(record => {
    record._strategy = 'periodic'
    record._interval = interval
    syncRealRecordToForm(record)
  })
  showMessage(`已为 ${rows.length} 个已启用设备设置周期 ${interval}`, 'success')
}

const batchSetStrategyForEnabled = (strategy) => {
  const isPeriodic = strategy === 'periodic'
  const rows = enabledLeafRows()
  if (!rows.length) {
    showMessage('没有已启用的设备，请先在通道勾选启用设备', 'warning')
    return
  }
  const intervalInput = (batchInterval.value || props.defaultInterval).trim()
  const applyInterval = intervalInput || props.defaultInterval
  rows.forEach(record => {
    record._strategy = strategy
    if (isPeriodic) record._interval = applyInterval
    syncRealRecordToForm(record)
  })
  showMessage(
    `已为 ${rows.length} 个已启用设备设置为${isPeriodic ? `周期上报（全量），周期 ${applyInterval}` : '变化上报（增量）'}`,
    'success'
  )
}

defineExpose({ rebuildTables })
</script>

<style scoped>
/* v3.0 — styles in src/styles/northbound-form.css */
</style>