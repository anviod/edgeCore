<template>
  <div class="page-shell log-viewer-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">系统日志</h2>

        <p class="page-subtitle">WebSocket 实时日志流 · 支持分类/通道/设备筛选与 CSV 导出</p>
      </div>
    </div>



    <div class="log-viewer-body">
      <div class="log-toolbar form-controls-md">
        <div class="toolbar-left">
          <div class="log-meta">
            <span class="log-meta__count">{{ filteredLogs.length }} 条</span>

            <span class="toolbar-divider" aria-hidden="true"></span>

            <span

              class="stream-status"

              :class="isStreaming ? 'is-live' : 'is-paused'"
            >

              <span class="stream-status__dot"></span>

              {{ isStreaming ? '刷新中' : '已暂停' }}

            </span>
          </div>
        </div>

        <div class="toolbar-right">
          <div class="log-toolbar-group log-toolbar-group--filters" aria-label="日志筛选">
            <a-radio-group
              v-model="filters.level"
              type="button"
              size="small"
              class="log-level-toggle"
            >
              <a-radio
                v-for="opt in logLevelOptions"
                :key="opt.value"
                :value="opt.value"
              >
                {{ opt.label }}
              </a-radio>
            </a-radio-group>

            <span class="toolbar-divider toolbar-divider--inset" aria-hidden="true"></span>

            <div class="log-toolbar-selects">
              <a-select
                v-model="filters.categories"
                :options="LOG_CATEGORY_OPTIONS"
                placeholder="日志分类"
                multiple
                allow-clear
                :max-tag-count="1"
                class="log-toolbar-select log-toolbar-select--category"
              />

              <a-select
                v-model="filters.channelId"
                :options="channelOptions"
                placeholder="采集通道"
                allow-clear
                allow-search
                :loading="loadingChannels"
                class="log-toolbar-select log-toolbar-select--channel"
              />

              <a-select
                v-model="filters.deviceId"
                :options="deviceOptions"
                placeholder="设备"
                allow-clear
                allow-search
                :disabled="!filters.channelId"
                :loading="loadingDevices"
                class="log-toolbar-select log-toolbar-select--device"
              />
            </div>
          </div>

          <div class="log-toolbar-group log-toolbar-group--actions" aria-label="日志操作">
            <div
              class="log-stream-toggle"
              :class="{ 'is-streaming': isStreaming, 'is-paused': !isStreaming }"
              role="group"
              aria-label="实时打印控制"
            >
              <span class="log-stream-toggle__indicator" aria-hidden="true"></span>
              <button
                type="button"
                class="log-stream-toggle__btn log-stream-toggle__btn--update"
                :class="{ 'is-active': isStreaming }"
                :aria-pressed="isStreaming"
                aria-label="更新实时日志"
                @click="startStreaming"
              >
                <icon-loading v-if="isStreaming" class="log-stream-toggle__icon log-stream-toggle__icon--spin" />
                <icon-play-arrow v-else class="log-stream-toggle__icon" />
                更新
              </button>
              <button
                type="button"
                class="log-stream-toggle__btn log-stream-toggle__btn--pause"
                :class="{ 'is-active': !isStreaming }"
                :aria-pressed="!isStreaming"
                aria-label="暂停实时日志"
                @click="pauseStreaming"
              >
                <icon-pause />
                暂停
              </button>
            </div>

            <span class="toolbar-divider toolbar-divider--inset" aria-hidden="true"></span>

            <div class="log-toolbar-buttons">
              <a-popconfirm content="确定清空当前屏幕的日志吗？" position="br" @ok="clearLogs">
                <a-button type="outline" size="small">
                  <template #icon><icon-delete /></template>
                  清空屏幕
                </a-button>
              </a-popconfirm>

              <a-button type="primary" size="small" @click="downloadLogs">
                <template #icon><icon-download /></template>
                导出 CSV
              </a-button>
            </div>
          </div>
        </div>
      </div>



      <div v-if="pinnedLogs.length > 0" class="log-pinned-panel">
        <div class="log-pinned-panel__header">
          <span class="log-pinned-panel__badge">已钉住 {{ pinnedLogs.length }}</span>
          <a-button type="text" size="mini" status="danger" @click="unpinAllLogs">
            取消全部钉住
          </a-button>
        </div>
        <div class="log-pinned-panel__entries">
          <div
            v-for="pin in pinnedLogs"
            :key="getLogEntryKey(pin)"
            class="log-line log-pinned-entry is-pinned"
            :class="{ 'is-selected': isActiveDetailLog(pin) && detailVisible }"
            role="button"
            tabindex="0"
            @click="openPinnedDetail(pin)"
            @keydown.enter.prevent="openPinnedDetail(pin)"
          >
            <div class="log-pinned-entry__row">
              <div class="log-line__head">
                <span class="log-time">{{ formatLogTime(pin.ts) }}</span>
                <span :class="['log-level', getLogLevelClass(pin.level)]">
                  {{ (pin.level || 'INFO').toUpperCase() }}
                </span>
                <span :class="['log-category', getLogCategoryClass(getLogCategory(pin))]">
                  {{ LOG_CATEGORY_LABELS[getLogCategory(pin)] || getLogCategory(pin) }}
                </span>
                <span v-if="formatLogScopeLabel(pin, channelNameMap, deviceNameMap)" class="log-scope">
                  {{ formatLogScopeLabel(pin, channelNameMap, deviceNameMap) }}
                </span>
                <span class="log-message">{{ pin.msg }}</span>
              </div>
              <button
                type="button"
                class="log-pinned-entry__unpin"
                aria-label="取消钉住"
                @click.stop="unpinLog(pin)"
              >
                ×
              </button>
            </div>
            <div v-if="hasExtraFields(pin)" class="log-line__fields">
              <span
                v-for="(val, key) in getLogExtraFields(pin)"
                :key="key"
                class="log-extra"
              >
                <span class="log-extra__key">{{ key }}</span>
                <span class="log-extra__sep">=</span>
                <span class="log-extra__val">{{ val }}</span>
              </span>
            </div>
          </div>
        </div>
      </div>

      <div class="log-terminal" ref="terminalRef">
        <div v-if="displayLogs.length === 0" class="log-empty">
          <icon-file class="log-empty__icon" />

          <p class="log-empty__title">暂无日志</p>

          <p class="log-empty__hint">
            {{ isStreaming ? '等待服务端推送日志…' : '实时打印已关闭，开启后将显示新日志' }}
          </p>
        </div>

        <div
          v-for="log in displayLogs"
          :key="getLogEntryKey(log)"
          class="log-line"
          :class="{ 'is-pinned': isPinnedLog(log), 'is-selected': isActiveDetailLog(log) && detailVisible }"
          role="button"
          tabindex="0"
          @click="pinLog(log)"
          @keydown.enter.prevent="pinLog(log)"
        >
          <div class="log-line__head">
            <span class="log-time">{{ formatLogTime(log.ts) }}</span>

            <span :class="['log-level', getLogLevelClass(log.level)]">

              {{ (log.level || 'INFO').toUpperCase() }}

            </span>

            <span :class="['log-category', getLogCategoryClass(getLogCategory(log))]">

              {{ LOG_CATEGORY_LABELS[getLogCategory(log)] || getLogCategory(log) }}

            </span>

            <span v-if="formatLogScopeLabel(log, channelNameMap, deviceNameMap)" class="log-scope">

              {{ formatLogScopeLabel(log, channelNameMap, deviceNameMap) }}

            </span>

            <span class="log-message">{{ log.msg }}</span>
          </div>

          <div v-if="hasExtraFields(log)" class="log-line__fields">
            <span

              v-for="(val, key) in getLogExtraFields(log)"

              :key="key"

              class="log-extra"
            >

              <span class="log-extra__key">{{ key }}</span>

              <span class="log-extra__sep">=</span>

              <span class="log-extra__val">{{ val }}</span>

            </span>
          </div>
        </div>
      </div>



      <div v-if="filteredLogs.length > 0" class="log-pagination">
        <a-pagination

          v-model:current="page"

          :page-size="perPage"

          :total="filteredLogs.length"

          size="small"

          show-total
        />
      </div>
    </div>

    <LogDetailModal
      v-model:visible="detailVisible"
      :log="activeDetailLog"
      :pinned-logs="pinnedLogs"
      :channel-name-map="channelNameMap"
      :device-name-map="deviceNameMap"
      @unpin="unpinLog(activeDetailLog)"
      @prev="navigateDetail(-1)"
      @next="navigateDetail(1)"
    />
  </div>
</template>



<script setup>

import { ref, computed, watch, onMounted, onUnmounted } from 'vue'

import { IconDelete, IconDownload, IconFile, IconLoading, IconPlayArrow, IconPause } from '@arco-design/web-vue/es/icon'

import LogDetailModal from '@/components/log-viewer/LogDetailModal.vue'

import { useLogViewerFilters } from '@/composables/useLogViewerFilters'

import {

    LOG_CATEGORY_LABELS,

    LOG_CATEGORY_OPTIONS,

    formatLogScopeLabel,

    formatLogTime,

    getLogCategory,

    getLogCategoryClass,

    getLogExtraFields,

    getLogLevelClass,

    findLogEntryIndex,

    getLogEntryKey,

    isSameLogEntry,

    matchesLogViewerFilters,

    normalizeLogEntry,

} from '@/utils/logFormat'



const logs = ref([])

const isStreaming = ref(true)

const terminalRef = ref(null)

const pinnedLogs = ref([])

const activeDetailLog = ref(null)

const detailVisible = ref(false)

let ws = null

const maxLogs = 1000

const perPage = 30

const page = ref(1)



const {

    filters,

    channelOptions,

    deviceOptions,

    channelNameMap,

    deviceNameMap,

    loadingChannels,

    loadingDevices,

    loadChannels,

} = useLogViewerFilters()



const logLevelOptions = [

  { label: 'ALL', value: 'ALL' },

  { label: 'INFO', value: 'INFO' },

  { label: 'WARN', value: 'WARN' },

  { label: 'ERROR', value: 'ERROR' },

  { label: 'DEBUG', value: 'DEBUG' }

]



const filteredLogs = computed(() =>

    logs.value.filter((log) => matchesLogViewerFilters(log, filters.value))

)



const displayLogs = computed(() => {

    const start = (page.value - 1) * perPage

    const end = start + perPage

    return filteredLogs.value.slice(start, end)

})



watch(() => logs.value.length, () => {

    if (isStreaming.value && page.value !== 1) {

        page.value = 1

    }

})



watch(filters, () => {

    page.value = 1

}, { deep: true })



watch(page, (newVal) => {

    if (newVal !== 1) {

        isStreaming.value = false

    }

})



const connectWs = () => {

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'

    const host = window.location.host



    let token = ''

    try {

        const raw = localStorage.getItem('loginInfo')

        if (raw) {

            const parsed = JSON.parse(raw)

            token = parsed.token || (parsed.data && parsed.data.token) || ''

        }

    } catch (e) {

        console.error('Failed to parse loginInfo', e)

    }



    ws = new WebSocket(`${protocol}//${host}/api/ws/logs${token ? `?token=${token}` : ''}`)



    ws.onopen = () => {

        console.log('Log WS connected')

    }



    ws.onmessage = (event) => {

        if (!isStreaming.value) return



        try {

            const log = normalizeLogEntry(JSON.parse(event.data))

            logs.value.unshift(log)

            if (logs.value.length > maxLogs) {

                logs.value.pop()

            }

            if (page.value !== 1) {

                page.value = 1

            }

        } catch (e) {

            if (!isStreaming.value) return



            logs.value.unshift(normalizeLogEntry({

                ts: new Date().toISOString(),

                level: 'INFO',

                msg: event.data,

            }))

            if (page.value !== 1) {

                page.value = 1

            }

        }

    }



    ws.onclose = () => {

        console.log('Log WS closed')

    }

}



const startStreaming = () => {

    if (isStreaming.value) return

    isStreaming.value = true

    if (page.value !== 1) {

        page.value = 1

    }

}



const pauseStreaming = () => {

    if (!isStreaming.value) return

    isStreaming.value = false

}



const clearLogs = () => {

    logs.value = []

}



const downloadLogs = () => {

    const headers = ['Timestamp', 'Level', 'Category', 'Channel', 'Device', 'Message', 'Details']

    const rows = filteredLogs.value.map(log => {

        const ts = log.ts ? new Date(log.ts).toLocaleString() : ''

        const level = (log.level || 'INFO').toUpperCase()

        const category = LOG_CATEGORY_LABELS[getLogCategory(log)] || getLogCategory(log)

        const channel = formatLogScopeLabel(log, channelNameMap.value, deviceNameMap.value).split(' / ')[0] || ''

        const device = formatLogScopeLabel(log, channelNameMap.value, deviceNameMap.value).split(' / ')[1] || ''

        const msg = (log.msg || '').replace(/"/g, '""')

        const details = JSON.stringify(getLogExtraFields(log)).replace(/"/g, '""')

        return `"${ts}","${level}","${category}","${channel}","${device}","${msg}","${details}"`

    })



    const bom = '\uFEFF'

    const csvContent = bom + [headers.join(','), ...rows].join('\n')

    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' })

    const link = document.createElement('a')

    link.href = URL.createObjectURL(blob)

    link.download = `edge_logs_${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.csv`

    link.click()

    URL.revokeObjectURL(link.href)

}



const hasExtraFields = (log) => Object.keys(getLogExtraFields(log)).length > 0



const isPinnedLog = (log) => findLogEntryIndex(pinnedLogs.value, log) !== -1



const isActiveDetailLog = (log) => isSameLogEntry(log, activeDetailLog.value)



const pinLog = (log) => {

    const existing = pinnedLogs.value.find((entry) => isSameLogEntry(entry, log))

    if (existing) {

        activeDetailLog.value = existing

    } else {

        pinnedLogs.value.push(log)

        activeDetailLog.value = log

    }

    detailVisible.value = true

}



const openPinnedDetail = (log) => {

    activeDetailLog.value = log

    detailVisible.value = true

}



const unpinLog = (log) => {

    if (!log) return

    const idx = findLogEntryIndex(pinnedLogs.value, log)

    if (idx === -1) return

    pinnedLogs.value.splice(idx, 1)

    if (isSameLogEntry(activeDetailLog.value, log)) {

        if (pinnedLogs.value.length === 0) {

            activeDetailLog.value = null

            detailVisible.value = false

        } else {

            activeDetailLog.value = pinnedLogs.value[Math.min(idx, pinnedLogs.value.length - 1)]

        }

    }

}



const unpinAllLogs = () => {

    pinnedLogs.value = []

    activeDetailLog.value = null

    detailVisible.value = false

}



const navigateDetail = (delta) => {

    const idx = findLogEntryIndex(pinnedLogs.value, activeDetailLog.value)

    if (idx === -1) return

    const nextIdx = idx + delta

    if (nextIdx >= 0 && nextIdx < pinnedLogs.value.length) {

        activeDetailLog.value = pinnedLogs.value[nextIdx]

    }

}



onMounted(() => {

    loadChannels()

    connectWs()

})



onUnmounted(() => {

    if (ws) ws.close()

})

</script>



<style scoped>

/* v3.0 — styles in src/styles/log-viewer.css */

</style>


