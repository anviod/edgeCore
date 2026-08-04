<template>
  <a-modal
    v-model:visible="historyDialog"
    title="历史数据"
    width="90%"
    :modal-style="{ maxWidth: '1440px' }"
    modal-class="history-modal"
    :align-center="true"
    :mask-closable="false"
    @cancel="onClose"
  >
    <template #footer>
      <a-space>
        <a-button @click="onClose">关闭</a-button>
        <a-button @click="downloadHistoryCSV" :disabled="historyData.length === 0">导出 CSV</a-button>
        <a-button type="primary" :loading="historyLoading" @click="fetchHistory">查询</a-button>
      </a-space>
    </template>

    <div class="history-modal-body">
      <div class="history-info-bar">
        <div class="history-info-bar__main">
          <span class="history-info-bar__label">设备</span>
          <span class="history-info-bar__name">{{ device?.name || '-' }}</span>
          <span
            v-if="storageHint"
            class="history-info-bar__meta"
            :class="{ 'is-warning': storageHint === '历史存储未启用' }"
          >
            {{ storageHint }}
          </span>
        </div>
        <a-dropdown trigger="click">
          <a-button size="small" type="outline">
            列筛选
            <icon-filter />
          </a-button>
          <template #content>
            <div class="column-filter-panel scrollbar-industrial">
              <div class="column-filter-item disabled">
                <a-checkbox :model-value="true" disabled>时间</a-checkbox>
              </div>
              <a-divider :margin="8" />
              <div v-for="header in historyHeaders" :key="header.key" class="column-filter-item">
                <a-checkbox
                  :model-value="selectedColumns.includes(header.key)"
                  @change="checked => toggleColumn(header.key, checked)"
                >
                  {{ header.title }}
                </a-checkbox>
              </div>
            </div>
          </template>
        </a-dropdown>
      </div>

      <div class="history-query-panel">
        <div class="history-query-row">
          <div class="history-query-field history-query-field--mode">
            <span class="history-query-label">查询模式</span>
            <a-select
              v-model="historyMode"
              :options="historyModeOptions"
              placeholder="查询模式"
            />
          </div>
          <div v-if="historyMode === 'limit'" class="history-query-field history-query-field--limit">
            <span class="history-query-label">记录数量</span>
            <a-input-number
              v-model="historyLimit"
              :min="1"
              :max="1000"
              placeholder="记录数量"
            />
          </div>
          <div v-if="historyMode === 'range'" class="history-query-field history-query-field--range">
            <span class="history-query-label">时间范围</span>
            <a-range-picker
              v-model="historyDateRange"
              show-time
              format="YYYY-MM-DD HH:mm:ss"
              value-format="YYYY-MM-DD HH:mm:ss"
              :placeholder="['开始时间', '结束时间']"
            />
          </div>
          <div v-if="historyMode === 'range'" class="history-query-presets">
            <span class="history-query-presets__label">快捷</span>
            <a-button size="mini" type="outline" @click="setRangePreset('1h')">1 小时</a-button>
            <a-button size="mini" type="outline" @click="setRangePreset('24h')">24 小时</a-button>
            <a-button size="mini" type="outline" @click="setRangePreset('7d')">7 天</a-button>
          </div>
        </div>
      </div>

      <div class="history-summary">
        <span>查询结果：<strong>{{ historyData.length }}</strong> 条</span>
        <span v-if="historyMode === 'limit'" class="summary-hint">最多 1000 条 · 每分钟 1 条约 16.7 小时</span>
        <span v-else-if="historyDateRange?.length === 2" class="summary-hint">
          范围：{{ historyDateRange[0] }} ~ {{ historyDateRange[1] }}
        </span>
      </div>

      <div class="table-container saas-table history-table-wrap scrollbar-industrial">
        <a-table
          :columns="tableColumns"
          :data="paginatedData"
          :loading="historyLoading"
          :pagination="paginationConfig"
          size="small"
          class="history-table"
          row-key="rowKey"
          :scroll="{ x: 'max-content', y: 420 }"
          @page-change="handlePageChange"
          @page-size-change="handlePageSizeChange"
        >
          <template #ts="{ record }">
            <span
              class="cell-content"
              :title="'点击复制 ' + formatHistoryTime(record.ts)"
              @click="copyToClipboard(formatHistoryTime(record.ts))"
            >
              {{ formatHistoryTime(record.ts) }}
            </span>
          </template>
          <template #empty>
            <a-empty :description="historyLoading ? '查询中...' : (historySearched ? '未查询到历史数据' : '请设置查询条件后点击「查询」')">
              <template #image>
                <icon-search :size="40" class="empty-icon-muted" />
              </template>
            </a-empty>
          </template>
        </a-table>
      </div>
    </div>
  </a-modal>
</template>

<script setup>
import { ref, watch, computed, reactive } from 'vue'
import { Message } from '@arco-design/web-vue'
import { IconFilter, IconSearch } from '@arco-design/web-vue/es/icon'
import request from '@/utils/request'

const MAX_HISTORY_LIMIT = 1000

const props = defineProps({
  visible: Boolean,
  device: {
    type: Object,
    default: () => null
  }
})

const emit = defineEmits(['update:visible'])

const historyDialog = ref(false)
const historyDevice = ref(null)
const historyLoading = ref(false)
const historyData = ref([])
const historySearched = ref(false)
const historyHeaders = ref([])
const historyDateRange = ref([])
const historyLimit = ref(MAX_HISTORY_LIMIT)
const historyMode = ref('limit')
const selectedColumns = ref([])

const pagination = reactive({
  current: 1,
  pageSize: 50,
  total: 0
})

const historyModeOptions = [
  { label: '最近记录', value: 'limit' },
  { label: '时间范围', value: 'range' }
]

// History headers use keys like "data.SetPoint.Value"; point IDs may contain dots.
const historyPointKey = (header) => header.title || header.key.replace(/^data\./, '')

const storageHint = computed(() => {
  const storage = props.device?.storage
  if (!storage?.enable) return '历史存储未启用'
  const interval = storage.interval || 1
  const maxRecords = storage.max_records || MAX_HISTORY_LIMIT
  const strategyMap = {
    interval: '定时间隔全量快照',
    minute_aligned: '整分钟全量快照',
    realtime: '实时全量快照'
  }
  const strategy = strategyMap[storage.strategy] || storage.strategy || '整分钟全量快照'
  return `${strategy} · 每 ${interval} 分钟一条 · 最多 ${maxRecords} 条快照`
})

const tableColumns = computed(() => {
  const columns = [
    {
      title: '时间',
      dataIndex: 'ts',
      width: 180,
      slotName: 'ts',
      ellipsis: true,
      fixed: 'left'
    }
  ]

  historyHeaders.value.forEach(header => {
    if (!selectedColumns.value.includes(header.key)) return
    const pointKey = historyPointKey(header)
    columns.push({
      title: header.title,
      dataIndex: pointKey,
      width: 120,
      ellipsis: true,
      tooltip: true,
      render: ({ record }) => {
        const val = record.data?.[pointKey]
        if (val === null || val === undefined || val === '') return '-'
        return String(val)
      }
    })
  })

  return columns
})

const paginatedData = computed(() => {
  const start = (pagination.current - 1) * pagination.pageSize
  return historyData.value.slice(start, start + pagination.pageSize).map((row, index) => ({
    ...row,
    rowKey: `${row.ts}-${start + index}`
  }))
})

const paginationConfig = computed(() => ({
  current: pagination.current,
  pageSize: pagination.pageSize,
  total: historyData.value.length,
  showTotal: true,
  showPageSize: true,
  pageSizeOptions: [20, 50, 100, 200, 500]
}))

const formatDateTime = (date) => {
  const pad = (n) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

const formatHistoryTime = (ts) => {
  if (ts === null || ts === undefined || ts === '') return '-'
  const date = new Date(Number(ts) * 1000)
  if (Number.isNaN(date.getTime())) return '-'
  return formatDateTime(date)
}

const setRangePreset = (preset) => {
  const end = new Date()
  const start = new Date(end)
  if (preset === '1h') start.setHours(end.getHours() - 1)
  else if (preset === '24h') start.setDate(end.getDate() - 1)
  else if (preset === '7d') start.setDate(end.getDate() - 7)
  historyDateRange.value = [formatDateTime(start), formatDateTime(end)]
}

const toggleColumn = (key, checked) => {
  if (checked) {
    if (!selectedColumns.value.includes(key)) {
      selectedColumns.value.push(key)
    }
  } else {
    selectedColumns.value = selectedColumns.value.filter(col => col !== key)
  }
}

watch(
  () => props.visible,
  (val) => {
    historyDialog.value = val
    if (val) {
      historyDevice.value = props.device
      resetHistoryQuery()
      fetchHistory()
    }
  }
)

watch(
  () => props.device,
  (val) => {
    historyDevice.value = val
    if (historyDialog.value) {
      resetHistoryQuery()
      fetchHistory()
    }
  }
)

const onClose = () => {
  historyDialog.value = false
}

watch(historyDialog, (val) => {
  if (!val) {
    emit('update:visible', false)
  }
})

const resetHistoryQuery = () => {
  historyData.value = []
  historyHeaders.value = []
  selectedColumns.value = []
  historyMode.value = 'limit'
  historyLimit.value = MAX_HISTORY_LIMIT
  pagination.current = 1
  pagination.pageSize = 50
  pagination.total = 0
  setRangePreset('24h')
}

const buildHistoryUrl = () => {
  const deviceId = historyDevice.value?.id
  if (!deviceId) return null

  const params = new URLSearchParams()
  if (historyMode.value === 'range') {
    if (!historyDateRange.value || historyDateRange.value.length !== 2) {
      throw new Error('请选择时间范围')
    }
    params.set('start', historyDateRange.value[0])
    params.set('end', historyDateRange.value[1])
    params.set('limit', String(MAX_HISTORY_LIMIT))
  } else {
    params.set('limit', String(Math.min(historyLimit.value || MAX_HISTORY_LIMIT, MAX_HISTORY_LIMIT)))
  }

  return `/api/devices/${deviceId}/history?${params.toString()}`
}

const fetchHistory = async () => {
  if (!historyDevice.value?.id) return

  historyLoading.value = true
  historySearched.value = true
  historyData.value = []
  historyHeaders.value = []
  pagination.current = 1

  try {
    const url = buildHistoryUrl()
    const res = await request.get(url, { timeout: 60000 })

    let rows = []
    if (Array.isArray(res)) {
      rows = res
    } else {
      rows = res?.data || []
    }

    historyData.value = rows
    pagination.total = rows.length

    if (rows.length > 0) {
      const keys = new Set()
      rows.forEach(row => {
        if (row.data) {
          Object.keys(row.data).forEach(k => keys.add(k))
        }
      })
      historyHeaders.value = Array.from(keys).sort().map(k => ({ title: k, key: `data.${k}` }))
      selectedColumns.value = historyHeaders.value.map(header => header.key)
    } else {
      Message.info('该时间范围内暂无历史数据')
    }
  } catch (e) {
    Message.error('获取历史数据失败: ' + (e.message || '未知错误'))
  } finally {
    historyLoading.value = false
  }
}

const handlePageChange = (page) => {
  pagination.current = page
}

const handlePageSizeChange = (pageSize) => {
  pagination.pageSize = pageSize
  pagination.current = 1
}

const copyToClipboard = (text) => {
  if (!text || text === '-') return
  navigator.clipboard.writeText(text).then(() => {
    Message.success('已复制到剪贴板')
  }).catch(() => {
    Message.error('复制失败')
  })
}

const escapeCsv = (value) => {
  const text = value == null ? '' : String(value)
  if (/[",\n]/.test(text)) {
    return `"${text.replace(/"/g, '""')}"`
  }
  return text
}

const downloadHistoryCSV = () => {
  if (historyData.value.length === 0) {
    Message.warning('无数据可导出')
    return
  }

  const visibleHeaders = historyHeaders.value.filter(h => selectedColumns.value.includes(h.key))
  const headers = ['时间', ...visibleHeaders.map(h => h.title)]
  const rows = historyData.value.map(row => {
    const line = [formatHistoryTime(row.ts)]
    visibleHeaders.forEach(header => {
      line.push(row.data?.[historyPointKey(header)] ?? '')
    })
    return line.map(escapeCsv).join(',')
  })

  const csvContent = '\uFEFF' + [headers.map(escapeCsv).join(','), ...rows].join('\n')
  const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' })
  const link = document.createElement('a')
  link.href = URL.createObjectURL(blob)
  link.download = `${historyDevice.value?.name || 'device'}_history_${new Date().toISOString().slice(0, 10)}.csv`
  link.click()
  URL.revokeObjectURL(link.href)
}
</script>

<style scoped>
/* v3.0 — styles in src/styles/history-modal.css */
</style>
