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
    <!-- 工具栏 -->
    <div class="ai-event-toolbar">
      <div class="ai-event-toolbar__left">
        <a-button size="mini" :type="paused ? 'outline' : 'text'" @click="togglePause">
          <template v-if="paused">
            <svg viewBox="0 0 12 12" fill="currentColor" style="width:10px;height:10px;margin-right:3px"><path d="M3 2v8L10 6z" /></svg>
            继续
          </template>
          <template v-else>
            <svg viewBox="0 0 12 12" fill="currentColor" style="width:10px;height:10px;margin-right:3px"><rect x="3" y="2" width="2.5" height="8" rx="0.5" /><rect x="6.5" y="2" width="2.5" height="8" rx="0.5" /></svg>
            暂停
          </template>
        </a-button>
        <a-button size="mini" type="text" @click="clearEvents">
          <svg viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" style="width:10px;height:10px;margin-right:3px"><path d="M2.5 3.5h7M5 1.5h2M4.5 3.5v6.5M7.5 3.5v6.5M3 3.5l.5 7a1 1 0 0 0 1 .9h3a1 1 0 0 0 1-.9l.5-7" /></svg>
          清除
        </a-button>
      </div>
      <div class="ai-event-toolbar__right">
        <a-select v-model="filterType" size="mini" style="width: 100px" :options="typeOptions" />
        <a-select v-model="filterSeverity" size="mini" style="width: 100px" :options="severityOptions" />
      </div>
    </div>

    <!-- 事件列表 -->
    <div class="ai-event-list">
      <div
        v-for="evt in filteredEvents"
        :key="evt.event_id"
        class="ai-event-item"
        :class="`sev-${evt.severity || 'info'}`"
        @click="toggleExpand(evt.event_id)"
      >
        <div class="ai-event-item__header">
          <span class="sev-dot"></span>
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
        <div class="ai-event-empty__icon">
          <svg viewBox="0 0 32 32" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <path d="M8 14a8 8 0 0 1 16 0v5l2 3H6l2-3z" opacity="0.4" />
            <path d="M13 25a3 3 0 0 0 6 0" opacity="0.4" />
          </svg>
        </div>
        <p>暂无事件</p>
      </div>
    </div>

    <div class="ai-event-footer">
      <span class="ai-event-footer__count" v-if="!paused">
        <span class="live-dot"></span>
        实时监听中
      </span>
      <span>已接收 {{ eventCount }} 条事件</span>
    </div>
  </div>
</template>

<style scoped>
.ai-event-mon {
  display: flex;
  flex-direction: column;
  gap: 8px;
  height: 100%;
}

/* ── 工具栏 ── */
.ai-event-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}
.ai-event-toolbar__left,
.ai-event-toolbar__right {
  display: flex;
  gap: 6px;
  align-items: center;
}

/* ── 事件列表 ── */
.ai-event-list {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.ai-event-item {
  border: 1px solid var(--ai-glass-border-subtle, var(--border));
  border-radius: 8px;
  padding: 6px 10px;
  cursor: pointer;
  transition: all 160ms cubic-bezier(0.16, 1, 0.3, 1);
  background: var(--ai-glass-bg-card, transparent);
}
.ai-event-item:hover {
  border-color: color-mix(in srgb, var(--primary) 25%, var(--border));
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.03);
}

/* ── 严重级别 ── */
.ai-event-item.sev-info .sev-dot { background: var(--primary); }
.ai-event-item.sev-info { border-left: 3px solid var(--primary); }
.ai-event-item.sev-warning { border-left: 3px solid #f59e0b; }
.ai-event-item.sev-warning .sev-dot { background: #f59e0b; }
.ai-event-item.sev-error { border-left: 3px solid #ef4444; }
.ai-event-item.sev-error .sev-dot { background: #ef4444; }
.ai-event-item.sev-critical {
  border-left: 3px solid #ef4444;
  background: rgba(239, 68, 68, 0.03);
}
.ai-event-item.sev-critical .sev-dot {
  background: #ef4444;
  animation: ean-blink 1s ease-in-out infinite;
}
@keyframes ean-blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.3; }
}

.ai-event-item__header {
  display: flex;
  align-items: center;
  gap: 8px;
}
.sev-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  flex-shrink: 0;
  flex-shrink: 0;
}
.time {
  font-size: 11px;
  color: var(--ai-text-faint, var(--text-tertiary));
  font-family: 'SF Mono', 'Cascadia Code', 'Consolas', monospace;
  flex-shrink: 0;
}
.type {
  font-size: 12px;
  color: var(--ai-text, var(--text-primary));
  font-weight: 500;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sev-tag {
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 3px;
  color: var(--ai-text-faint, var(--text-tertiary));
  background: var(--ai-glass-bg-subtle, rgba(0, 0, 0, 0.04));
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.3px;
  flex-shrink: 0;
}
.ai-event-item.sev-warning .sev-tag {
  color: #f59e0b;
  background: rgba(245, 158, 11, 0.08);
}
.ai-event-item.sev-error .sev-tag,
.ai-event-item.sev-critical .sev-tag {
  color: #ef4444;
  background: rgba(239, 68, 68, 0.08);
}

.ai-event-item__body {
  padding-top: 6px;
  border-top: 1px solid var(--ai-glass-border-subtle, var(--border));
  margin-top: 4px;
}
.ai-event-field {
  display: flex;
  gap: 8px;
  font-size: 11px;
  margin: 2px 0;
}
.ai-event-field .k {
  color: var(--ai-text-faint, var(--text-tertiary));
  min-width: 60px;
  font-weight: 500;
}
.ai-event-field .v {
  color: var(--ai-text-muted, var(--text-secondary));
  font-family: 'SF Mono', 'Cascadia Code', 'Consolas', monospace;
}

/* ── 空状态 ── */
.ai-event-empty {
  text-align: center;
  padding: 36px 20px;
  color: var(--ai-text-muted, var(--text-secondary));
  font-size: 13px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}
.ai-event-empty__icon {
  color: var(--ai-text-faint, var(--text-tertiary));
  opacity: 0.4;
}
.ai-event-empty__icon svg {
  width: 32px;
  height: 32px;
}

/* ── 页脚 ── */
.ai-event-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 11px;
  color: var(--ai-text-faint, var(--text-tertiary));
}
.ai-event-footer__count {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: #10b981;
}
.live-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #10b981;
  animation: ean-pulse 2s ease-in-out infinite;
}
@keyframes ean-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

/* ── 暗色主题 ── */
body.dark-theme .ai-event-item {
  background: rgba(13, 18, 34, 0.4);
}
body.dark-theme .ai-event-item.sev-critical {
  background: rgba(239, 68, 68, 0.06);
}
</style>
