<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useEan } from '@/composables/useEan'
import AiJsonPreview from './AiJsonPreview.vue'

const { events, eventCount, fetchEventHistory, pauseEvents, resumeEvents, clearEvents } = useEan()

const paused = ref(false)
const filterType = ref('')
const filterSeverity = ref('')

const typeOptions = [
  { label: '全部类型', value: '' },
  { label: 'changed', value: 'changed' },
  { label: 'device', value: 'device' },
  { label: 'alarm', value: 'alarm' },
  { label: 'capability', value: 'capability' }
]

const severityOptions = [
  { label: '全部级别', value: '' },
  { label: 'info', value: 'info' },
  { label: 'warning', value: 'warning' },
  { label: 'error', value: 'error' },
  { label: 'critical', value: 'critical' }
]

const filteredEvents = computed(() => {
  return events.value.filter((e) => {
    const matchType = !filterType.value || (e.event_type || '').includes(filterType.value)
    const matchSev = !filterSeverity.value || e.severity === filterSeverity.value
    return matchType && matchSev
  })
})

const expandedId = ref(null)
const toggleExpand = (id) => {
  expandedId.value = expandedId.value === id ? null : id
}

const formatTime = (ts) => {
  if (!ts) return ''
  const d = new Date(ts)
  return `${String(d.getHours()).padStart(2,'0')}:${String(d.getMinutes()).padStart(2,'0')}:${String(d.getSeconds()).padStart(2,'0')}`
}

const togglePause = () => {
  paused.value = !paused.value
  if (paused.value) pauseEvents()
  else resumeEvents()
}

let pollTimer = null
onMounted(() => {
  fetchEventHistory(50)
  pollTimer = setInterval(() => fetchEventHistory(20), 5000)
})
onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<template>
  <div class="ai-event-mon">
    <div class="ai-event-toolbar">
      <a-button size="mini" :type="paused ? 'outline' : 'text'" @click="togglePause">
        {{ paused ? '继续' : '暂停' }}
      </a-button>
      <a-button size="mini" type="text" @click="clearEvents">清除</a-button>
      <a-select v-model="filterType" size="mini" style="width: 100px" :options="typeOptions" />
      <a-select v-model="filterSeverity" size="mini" style="width: 100px" :options="severityOptions" />
    </div>
    <div class="ai-event-list">
      <div
        v-for="evt in filteredEvents"
        :key="evt.event_id"
        class="ai-event-item"
        :class="`sev-${evt.severity || 'info'}`"
        @click="toggleExpand(evt.event_id)"
      >
        <div class="ai-event-item__header">
          <span class="dot"></span>
          <span class="time">{{ formatTime(evt.timestamp) }}</span>
          <span class="type">{{ evt.event_type }}</span>
          <span class="sev-tag">{{ evt.severity || 'info' }}</span>
        </div>
        <div class="ai-event-item__body" v-if="expandedId === evt.event_id">
          <div class="ai-event-field"><span class="k">agent_id</span><span class="v">{{ evt.agent_id }}</span></div>
          <div class="ai-event-field" v-if="evt.device_id"><span class="k">device_id</span><span class="v">{{ evt.device_id }}</span></div>
          <div class="ai-event-field" v-if="evt.value !== undefined"><span class="k">value</span><span class="v">{{ evt.value }}</span></div>
          <AiJsonPreview v-if="evt.metadata" :data="evt.metadata" :compact="true" :copyable="false" />
        </div>
      </div>
      <div class="ai-event-empty" v-if="!filteredEvents.length">
        <p>暂无事件</p>
      </div>
    </div>
    <div class="ai-event-footer">
      已接收 {{ eventCount }} 条事件
    </div>
  </div>
</template>

<style scoped>
.ai-event-mon { display: flex; flex-direction: column; gap: 8px; height: 100%; }
.ai-event-toolbar { display: flex; gap: 6px; align-items: center; }
.ai-event-list { flex: 1; overflow-y: auto; display: flex; flex-direction: column; gap: 4px; }
.ai-event-item {
  border: 1px solid var(--ai-glass-border-subtle, var(--border));
  border-radius: 8px; padding: 6px 10px; cursor: pointer;
  transition: border-color 120ms;
}
.ai-event-item:hover { border-color: var(--primary); }
.ai-event-item.sev-warning { border-left: 3px solid #f59e0b; }
.ai-event-item.sev-error { border-left: 3px solid #ef4444; }
.ai-event-item.sev-critical { border-left: 3px solid #ef4444; }
.ai-event-item__header { display: flex; align-items: center; gap: 8px; }
.dot { width: 6px; height: 6px; border-radius: 50%; background: var(--primary); flex-shrink: 0; }
.sev-warning .dot { background: #f59e0b; }
.sev-error .dot { background: #ef4444; }
.sev-critical .dot { background: #ef4444; }
.time { font-size: 11px; color: var(--ai-text-faint, var(--text-tertiary)); font-family: monospace; }
.type { font-size: 12px; color: var(--ai-text, var(--text-primary)); font-weight: 500; flex: 1; }
.sev-tag { font-size: 10px; padding: 1px 6px; border-radius: 3px; color: var(--ai-text-faint, var(--text-tertiary)); background: var(--ai-glass-bg-subtle, rgba(0,0,0,0.04)); }
.ai-event-item__body { padding-top: 6px; border-top: 1px solid var(--ai-glass-border-subtle, var(--border)); margin-top: 4px; }
.ai-event-field { display: flex; gap: 8px; font-size: 11px; margin: 2px 0; }
.ai-event-field .k { color: var(--ai-text-faint, var(--text-tertiary)); min-width: 60px; }
.ai-event-field .v { color: var(--ai-text-muted, var(--text-secondary)); }
.ai-event-empty { text-align: center; padding: 32px; color: var(--ai-text-muted, var(--text-secondary)); font-size: 13px; }
.ai-event-footer { font-size: 11px; color: var(--ai-text-faint, var(--text-tertiary)); text-align: right; }
</style>
