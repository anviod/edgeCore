<template>
  <div class="modbus-slave-view modbus-point-tabs">
    <a-tabs
      v-model:active-key="activeTab"
      type="rounded"
      size="small"
      data-testid="modbus-tabs"
      :data-active-key="activeTab"
      @keydown="onTabsKeydown"
    >
      <a-tab-pane
        v-for="group in tabs"
        :key="group.key"
        :title="`${group.tabLabel} (${group.points.length})`"
      >
        <template #title>
          <span
            :data-testid="`register-tab-${group.key}`"
            :class="{ 'modbus-tab--empty': group.points.length === 0 && group.allPoints.length === 0 }"
          >{{ group.tabLabel }} <span class="modbus-tab-count" :class="{ 'modbus-tab-count--active': group.points.length > 0 }">({{ group.points.length }})</span></span>
        </template>
      </a-tab-pane>
    </a-tabs>

    <div class="modbus-point-tabs__content">
      <div
        v-if="loadError && allPoints.length === 0"
        data-testid="modbus-load-error-blocking"
        class="modbus-load-error"
      >
        <a-alert type="error" :title="loadError">
          <button type="button" class="modbus-action-btn modbus-action-btn--primary" @click="$emit('retry')">
            重试
          </button>
        </a-alert>
      </div>

      <template v-else>
        <a-alert
          v-if="loadError"
          type="error"
          :title="loadError"
          data-testid="modbus-load-error-inline"
          class="modbus-load-error modbus-load-error--inline"
        >
          <button type="button" class="modbus-action-btn modbus-action-btn--primary" @click="$emit('retry')">
            重试
          </button>
        </a-alert>

        <div
          v-if="activeGroup.allPoints.length === 0"
          data-testid="modbus-empty-category"
          class="group-empty font-mono"
        >
          {{ allGroupsEmpty ? '该设备暂无点位配置，可点击右上角「新增点位」开始添加' : '该寄存器类型暂无点位' }}
        </div>

        <div
          v-else-if="activeGroup.points.length === 0"
          data-testid="modbus-empty-filtered"
          class="group-empty font-mono"
        >
          <div>当前筛选条件下无匹配点位</div>
          <button
            type="button"
            class="modbus-action-btn modbus-action-btn--outline mt-2"
            @click="$emit('clear-filters')"
          >
            清除全局筛选
          </button>
        </div>

        <a-table
          v-else
          data-testid="modbus-point-table"
          :columns="columns"
          :data="activeGroup.points"
          :row-selection="rowSelectionConfig"
          :selected-keys="activeSelectedKeys"
          row-key="id"
          :pagination="activePagination"
          size="small"
          class="industrial-table-fluid modbus-group-table"
          :bordered="{ wrapper: true }"
          :scroll="{ x: 1126 }"
          @selection-change="onSelectionChange"
          @page-change="onPageChange"
          @page-size-change="onPageSizeChange"
        >
          <template #offset="{ record }">
            <span class="cell-numeric">{{ formatOffsetAddress(record.address) }}</span>
          </template>

          <template #plc="{ record }">
            <span class="cell-numeric">{{ record.plc_address ?? formatPlcAddress(record.register_key, record.address) }}</span>
          </template>

          <template #value="{ record }">
            <a-tooltip :content="valueTooltip(record)">
              <div class="value-cell cursor-pointer" @click="$emit('show-value', record)">
                <span class="value-text font-mono">{{ formatValue(record.value) }}</span>
                <span v-if="record.unit" class="value-unit">{{ record.unit }}</span>
              </div>
            </a-tooltip>
          </template>

          <template #quality="{ record }">
            <div class="status-display">
              <IconCheckCircle v-if="isQualityGood(record.quality)" class="status-icon status-icon--good" />
              <IconCloseCircle v-else class="status-icon status-icon--bad" />
              <span class="cell-text">{{ record.quality || 'Bad' }}</span>
            </div>
          </template>

          <template #readwrite="{ record }">
            <span class="cell-numeric">{{ record.readwrite || 'R' }}</span>
          </template>

          <template #timestamp="{ record }">
            <a-tooltip v-if="record?.updated_at" :content="`更新 ${formatDate(record.updated_at)}`">
              <div class="cell-time cursor-default">
                {{ record && (record.collected_at || record.timestamp) ? formatDate(record.collected_at || record.timestamp) : 'N/A' }}
              </div>
            </a-tooltip>
            <div v-else class="cell-time">
              {{ record && (record.collected_at || record.timestamp) ? formatDate(record.collected_at || record.timestamp) : 'N/A' }}
            </div>
          </template>

          <template #actions="{ record }">
            <div class="actions-container">
              <a-button
                v-if="record.readwrite === 'RW' || record.readwrite === 'W'"
                type="text"
                size="mini"
                @click="$emit('write', record)"
              >
                写入
              </a-button>
              <a-button type="text" size="mini" @click="$emit('edit', record)">编辑</a-button>
              <a-button type="text" size="mini" status="danger" @click="$emit('delete', record)">删除</a-button>
              <a-button type="text" size="mini" status="info" @click="$emit('debug', record)">调试</a-button>
            </div>
          </template>
        </a-table>
      </template>
    </div>
  </div>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { IconCheckCircle, IconCloseCircle } from '@arco-design/web-vue/es/icon'
import {
  MODBUS_REGISTER_GROUPS,
  groupPointsByRegisterType,
  formatPlcAddress,
  formatOffsetAddress
} from '@/utils/modbusRegisterGroups'

const props = defineProps({
  allPoints: {
    type: Array,
    default: () => []
  },
  points: {
    type: Array,
    default: () => []
  },
  selectedIds: {
    type: Array,
    default: () => []
  },
  filterKey: {
    type: String,
    default: '["",[]]'
  },
  loadError: {
    type: String,
    default: ''
  }
})

const emit = defineEmits([
  'selection-change',
  'retry',
  'clear-filters',
  'write',
  'edit',
  'delete',
  'debug',
  'show-value'
])

const activeTab = ref('coil')

// 优先展示有数据的 tab：首次加载/数据到达时自动定位到第一个有点位的分组，
// 之后不再抢占用户的主动切换。
const autoSelected = ref(false)

const tabKeys = MODBUS_REGISTER_GROUPS.map(group => group.key)

const onTabsKeydown = (event) => {
  if (event.key !== 'ArrowRight' && event.key !== 'ArrowLeft') return
  const index = tabKeys.indexOf(activeTab.value)
  if (index < 0) return
  const nextIndex = event.key === 'ArrowRight'
    ? Math.min(tabKeys.length - 1, index + 1)
    : Math.max(0, index - 1)
  if (nextIndex === index) return
  activeTab.value = tabKeys[nextIndex]
  event.preventDefault()
}

const tabLabels = {
  coil: '线圈',
  discrete_input: '离散输入',
  input: '输入寄存器',
  holding: '保持寄存器',
}

const groupedAllPoints = computed(() => groupPointsByRegisterType(props.allPoints))
const groupedFilteredPoints = computed(() => groupPointsByRegisterType(props.points))

const tabs = computed(() => MODBUS_REGISTER_GROUPS.map(group => ({
  ...group,
  tabLabel: tabLabels[group.key],
  allPoints: groupedAllPoints.value[group.key] || [],
  points: groupedFilteredPoints.value[group.key] || [],
})))

const activeGroup = computed(() =>
  tabs.value.find(group => group.key === activeTab.value) || tabs.value[0]
)

const allGroupsEmpty = computed(() =>
  tabs.value.every(group => group.allPoints.length === 0)
)

// 首次加载/数据到达时自动定位到第一个有点位的分组，之后不再抢占用户主动切换
watch(groupedFilteredPoints, (val) => {
  if (autoSelected.value) return
  const firstPopulated = tabs.value.find(group => group.points.length > 0)
  if (firstPopulated && (val[activeTab.value] || []).length === 0) {
    activeTab.value = firstPopulated.key
  }
  if (Object.values(val).some(group => group.length > 0)) {
    autoSelected.value = true
  }
}, { immediate: true })

const columns = [
  { title: '点位 ID', dataIndex: 'id', width: 110, ellipsis: true, tooltip: true },
  { title: '名称', dataIndex: 'name', width: 110, ellipsis: true, tooltip: true },
  { title: 'PDU 偏移', slotName: 'offset', width: 82, align: 'right' },
  { title: 'PLC 地址', slotName: 'plc', width: 104, align: 'right' },
  { title: '类型', dataIndex: 'datatype', width: 78, ellipsis: true },
  { title: 'R/W', slotName: 'readwrite', width: 56, align: 'center' },
  { title: '数值', slotName: 'value', width: 132, align: 'right' },
  { title: '质量', slotName: 'quality', width: 92 },
  { title: '采集时间', slotName: 'timestamp', width: 152 },
  { title: '操作', slotName: 'actions', width: 210, align: 'right' }
]

const paginationByGroup = reactive(Object.fromEntries(
  MODBUS_REGISTER_GROUPS.map(group => [
    group.key,
    { current: 1, pageSize: 10 },
  ])
))

const activePagination = computed(() => ({
  ...paginationByGroup[activeTab.value],
  total: activeGroup.value.points.length,
  pageSizeOptions: [10, 20, 50, 100],
  showPageSize: true,
  showTotal: true,
  size: 'small',
}))

const onPageChange = (current) => {
  paginationByGroup[activeTab.value].current = current
}

const onPageSizeChange = (pageSize) => {
  const pagination = paginationByGroup[activeTab.value]
  pagination.pageSize = pageSize
  pagination.current = 1
}

const groupIdSignature = computed(() =>
  MODBUS_REGISTER_GROUPS.map(group =>
    (groupedFilteredPoints.value[group.key] || []).map(point => point.id).join('\0')
  ).join('\n')
)

watch(() => props.filterKey, () => {
  for (const group of MODBUS_REGISTER_GROUPS) {
    paginationByGroup[group.key].current = 1
  }
})

watch(groupIdSignature, () => {
  for (const group of MODBUS_REGISTER_GROUPS) {
    const total = (groupedFilteredPoints.value[group.key] || []).length
    const pagination = paginationByGroup[group.key]
    const maxPage = Math.max(1, Math.ceil(total / pagination.pageSize))
    if (pagination.current > maxPage) {
      pagination.current = maxPage
    }
  }
})

const rowSelectionConfig = {
  type: 'checkbox',
  showCheckedAll: true,
  onlyCurrent: false,
}

const activeFilteredIdSet = computed(() =>
  new Set(activeGroup.value.points.map(point => point.id))
)

const activeSelectedKeys = computed(() => [
  ...new Set(props.selectedIds.filter(id => activeFilteredIdSet.value.has(id))),
])

const onSelectionChange = (currentGroupIds) => {
  const retained = props.selectedIds.filter(id => !activeFilteredIdSet.value.has(id))
  emit('selection-change', [...new Set([...retained, ...currentGroupIds])])
}

const formatValue = (val) => {
  if (val === null || val === undefined) return 'N/A'
  if (typeof val === 'number') return Number.isInteger(val) ? String(val) : val.toFixed(2)
  return String(val)
}

const formatDate = (ts) => {
  if (!ts) return 'N/A'
  const date = new Date(ts)
  if (Number.isNaN(date.getTime())) return 'N/A'
  const pad = (n) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

const isQualityGood = (q) => q === 'Good' || q === 'good'

const valueTooltip = (record) => {
  const dt = record.datatype || ''
  const addr = record.address ?? ''
  return `${formatValue(record.value)} ${record.unit || ''} · ${addr} · ${dt}`.trim()
}
</script>

<style scoped>
.modbus-point-tabs {
  min-width: 0;
}

.modbus-point-tabs :deep(.arco-tabs-nav-tab) {
  overflow-x: auto;
  scrollbar-width: thin;
}

.modbus-point-tabs :deep(.arco-tabs-nav-scroll) {
  min-width: max-content;
}

.modbus-point-tabs__content {
  margin-top: var(--space-3);
  min-width: 0;
}

/* 空 tab 弱化，有点位的 tab 计数更醒目 */
.modbus-tab-count {
  font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  color: var(--text-tertiary, #94a3b8);
  transition: color var(--transition-fast, 0.2s);
}

.modbus-tab-count--active {
  color: var(--text-secondary, #64748b);
}

.modbus-point-tabs :deep(.arco-tabs-tab-active) .modbus-tab-count,
.modbus-tab-count--active:hover {
  color: rgb(var(--primary-6, 22, 93, 255));
}

.modbus-tab--empty {
  opacity: 0.55;
}

.modbus-point-tabs :deep(.arco-tabs-tab-active) .modbus-tab--empty {
  opacity: 1;
}

.modbus-action-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0 12px;
  height: 28px;
  font-size: 14px;
  line-height: 1;
  border-radius: 2px;
  cursor: pointer;
}

.modbus-action-btn--primary {
  color: var(--color-white, #fff);
  background: rgb(var(--primary-6, 22, 93, 255));
  border: 1px solid transparent;
}

.modbus-action-btn--primary:hover {
  background: rgb(var(--primary-5, 64, 128, 255));
}

.modbus-action-btn--outline {
  color: var(--color-text-1, #1d2129);
  background: transparent;
  border: 1px solid var(--color-border-2, #e5e6eb);
}

.modbus-action-btn--outline:hover {
  border-color: rgb(var(--primary-6, 22, 93, 255));
  color: rgb(var(--primary-6, 22, 93, 255));
}

/* 覆盖仓库全局 outline:none，保证键盘焦点可见 */
button.modbus-action-btn:focus-visible {
  outline: 2px solid rgb(var(--primary-6, 22, 93, 255)) !important;
  outline-offset: 2px;
  box-shadow: none !important;
}

/* ═══ Modbus 点表美化：单元格不换行、表头/表身一体、悬停/选中反馈 ═══ */

/* 表格外壳：圆角 + 溢出裁剪，与页面卡片风格一致 */
.modbus-group-table :deep(.arco-table-container),
.modbus-group-table :deep(.arco-table-content) {
  border-radius: var(--radius-md, 8px);
  overflow: hidden;
}

/* 表头：独立底色 + 底部实线，与表身自然衔接 */
.modbus-group-table :deep(.arco-table-th) {
  background: var(--surface-1, #f1f5f9) !important;
  border: none !important;
  border-bottom: 2px solid var(--border, rgba(15, 23, 42, 0.08)) !important;
  height: 40px !important;
  min-height: 40px !important;
  max-height: 40px !important;
}

.modbus-group-table :deep(.arco-table-th .arco-table-cell) {
  white-space: nowrap !important;
  overflow: hidden;
  text-overflow: ellipsis;
  font-weight: 600;
  font-size: 11px;
  letter-spacing: 0.03em;
  text-transform: uppercase;
  color: var(--text-secondary, #475569);
}

/* 表头/表身统一：不换行、垂直居中、同高、同内边距（保证列对齐） */
.modbus-group-table :deep(.arco-table-th),
.modbus-group-table :deep(.arco-table-td) {
  white-space: nowrap !important;
  vertical-align: middle;
  overflow: hidden;
}

.modbus-group-table :deep(.arco-table-cell) {
  padding: 0 10px;
  line-height: 1.5;
}

.modbus-group-table :deep(.arco-table-tr) {
  height: 40px !important;
}

/* 表身行分隔线：细横线，弱化竖线，阅读更顺 */
.modbus-group-table :deep(.arco-table-td) {
  border: none !important;
  border-bottom: 1px solid var(--border, rgba(15, 23, 42, 0.08)) !important;
}

.modbus-group-table :deep(.arco-table-tr:last-child .arco-table-td) {
  border-bottom: none !important;
}

/* 行悬停与选中反馈 */
.modbus-group-table :deep(.arco-table-tr:hover .arco-table-td) {
  background: var(--edgeCore-surface-muted, #f8fafc) !important;
}

.modbus-group-table :deep(.arco-table-tr.arco-table-tr-selected .arco-table-td) {
  background: rgba(14, 165, 233, 0.07) !important;
}

/* 数字/地址类单元格：等宽字体、右对齐 */
.modbus-group-table .cell-numeric {
  font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 12px;
  color: var(--text-primary, #0f172a);
}

.modbus-group-table .cell-text {
  font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 12px;
  color: var(--text-secondary, #475569);
}

/* 时间列弱化但保持单行 */
.modbus-group-table .cell-time {
  font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 12px;
  color: var(--text-tertiary, #94a3b8);
  white-space: nowrap;
}

/* 数值列：主值 + 单位在同一行，超长截断 */
.modbus-group-table .value-cell {
  display: inline-flex;
  align-items: baseline;
  gap: 5px;
  max-width: 100%;
  white-space: nowrap;
  overflow: hidden;
}

.modbus-group-table .value-cell .value-text {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-primary, #0f172a);
  overflow: hidden;
  text-overflow: ellipsis;
}

.modbus-group-table .value-cell .value-unit {
  font-size: 11px;
  color: var(--text-tertiary, #94a3b8);
  flex-shrink: 0;
}

/* 质量状态图标对齐 */
.modbus-group-table .status-display {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  white-space: nowrap;
}

.modbus-group-table .status-display .status-icon {
  flex-shrink: 0;
}

.modbus-group-table .status-display .status-icon--good {
  color: var(--edgeCore-success, #16a34a);
}

.modbus-group-table .status-display .status-icon--bad {
  color: var(--edgeCore-error, #dc2626);
}

/* 操作列：单行不换行 */
.modbus-group-table .actions-container {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  white-space: nowrap;
}

.modbus-group-table .actions-container .arco-btn {
  padding: 0 4px;
}

/* 空状态提示 */
.modbus-point-tabs__content .group-empty {
  padding: var(--space-8) 0;
  text-align: center;
  color: var(--text-tertiary);
  font-size: 13px;
  line-height: 1.8;
}

.modbus-point-tabs__content .group-empty .modbus-action-btn {
  margin-top: var(--space-2);
}

@media (max-width: 900px) {
  .modbus-point-tabs :deep(.arco-tabs-nav-tab) {
    width: 100%;
  }
}
</style>
