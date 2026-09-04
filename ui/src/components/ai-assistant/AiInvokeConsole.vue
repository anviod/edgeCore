<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { Message } from '@arco-design/web-vue'
import { useEan } from '@/composables/useEan'
import AiJsonPreview from './AiJsonPreview.vue'
import AiStatusBadge from './AiStatusBadge.vue'

const props = defineProps({
  presetCapability: { type: String, default: '' }
})

const { capabilities, invokeHistory, activeInvoke, invokeCapability, agentStatus, fetchCapabilities } = useEan()

const selectedCapId = ref('')
const argumentsText = ref('{}')
const timeoutSec = ref(10)
const priority = ref('normal')
const retryCount = ref(0)
const executing = ref(false)

const selectedCap = computed(() => {
  return capabilities.value.find((c) => c.id === selectedCapId.value) || null
})

const groupedCaps = computed(() => {
  const groups = {}
  capabilities.value.forEach((c) => {
    const cat = c.category || 'system'
    if (!groups[cat]) groups[cat] = []
    groups[cat].push(c)
  })
  return groups
})

const selectOptions = computed(() => {
  return Object.entries(groupedCaps.value).map(([cat, caps]) => ({
    label: cat,
    value: cat,
    options: caps.map((c) => ({ label: c.id, value: c.id }))
  }))
})

watch(() => props.presetCapability, (val) => {
  if (val) {
    selectedCapId.value = val
    const cap = capabilities.value.find((c) => c.id === val)
    if (cap?.input_schema?.properties) {
      const template = {}
      Object.keys(cap.input_schema.properties).forEach((k) => {
        template[k] = ''
      })
      argumentsText.value = JSON.stringify(template, null, 2)
    }
  }
})

watch(selectedCapId, (val) => {
  if (!val) return
  const cap = capabilities.value.find((c) => c.id === val)
  if (cap?.input_schema?.properties) {
    const template = {}
    Object.keys(cap.input_schema.properties).forEach((k) => {
      template[k] = ''
    })
    argumentsText.value = JSON.stringify(template, null, 2)
  }
})

const handleExecute = async () => {
  if (!selectedCapId.value) return
  let args = {}
  try {
    args = JSON.parse(argumentsText.value)
  } catch {
    Message.error('Arguments JSON 格式错误')
    return
  }
  executing.value = true
  try {
    await invokeCapability(selectedCapId.value, args, {
      timeout_sec: timeoutSec.value,
      priority: priority.value,
      retry: retryCount.value
    })
  } finally {
    executing.value = false
  }
}

const formatTime = (ts) => {
  if (!ts) return ''
  const d = new Date(ts)
  return `${String(d.getHours()).padStart(2,'0')}:${String(d.getMinutes()).padStart(2,'0')}:${String(d.getSeconds()).padStart(2,'0')}`
}

const fillFromHistory = (record) => {
  selectedCapId.value = record.capability
  argumentsText.value = JSON.stringify(record.arguments, null, 2)
}

// capabilities 为空时自动拉取（跨 Tab 切换可能尚未加载）
onMounted(() => {
  if (!capabilities.value.length) {
    fetchCapabilities()
  }
})
</script>

<template>
  <div class="ai-invoke">
    <div class="ai-invoke-grid">
      <!-- 左侧：请求面板 -->
      <div class="ai-invoke-left">
        <div class="ai-invoke-section">
          <label class="ai-invoke-label">Capability</label>
          <a-select
            v-model="selectedCapId"
            placeholder="选择 Capability..."
            size="small"
            style="width: 100%"
            allow-search
            :options="selectOptions"
          />
        </div>
        <div class="ai-invoke-section" v-if="selectedCap">
          <label class="ai-invoke-label">描述</label>
          <p class="ai-invoke-desc">{{ selectedCap.description || '无描述' }}</p>
        </div>
        <div class="ai-invoke-section">
          <label class="ai-invoke-label">Arguments (JSON)</label>
          <textarea
            v-model="argumentsText"
            class="ai-invoke-textarea"
            rows="8"
            spellcheck="false"
          ></textarea>
        </div>
        <div class="ai-invoke-options">
          <div class="ai-invoke-opt">
            <label>超时</label>
            <a-input-number v-model="timeoutSec" size="small" :min="1" :max="300" style="width: 64px" />
            <span class="unit">s</span>
          </div>
          <div class="ai-invoke-opt">
            <label>优先级</label>
            <a-select
              v-model="priority" size="small" style="width: 84px"
              :options="[{label:'normal',value:'normal'},{label:'high',value:'high'},{label:'low',value:'low'}]"
            />
          </div>
          <div class="ai-invoke-opt">
            <label>重试</label>
            <a-input-number v-model="retryCount" size="small" :min="0" :max="5" style="width: 48px" />
          </div>
        </div>
        <a-button
          type="primary"
          long
          :loading="executing"
          :disabled="!selectedCapId"
          @click="handleExecute"
        >
          {{ executing ? '执行中...' : '执行调用' }}
        </a-button>
      </div>

      <!-- 右侧：响应面板 -->
      <div class="ai-invoke-right">
        <div class="ai-invoke-result" v-if="invokeHistory.length">
          <div class="ai-invoke-result-header" v-if="invokeHistory[0]">
            <span class="label">最近调用</span>
            <AiStatusBadge :status="invokeHistory[0].status === 'completed' ? 'applied' : invokeHistory[0].status === 'failed' ? 'failed' : 'processing'" />
            <span class="latency" v-if="invokeHistory[0].latency_ms">{{ invokeHistory[0].latency_ms }}ms</span>
          </div>
          <AiJsonPreview
            v-if="invokeHistory[0]?.result"
            :data="invokeHistory[0].result"
            :compact="true"
          />
          <div class="ai-invoke-noresult" v-else>
            <p>等待执行结果...</p>
          </div>
        </div>
        <div class="ai-invoke-history" v-if="invokeHistory.length > 1">
          <span class="history-title">调用历史</span>
          <div class="ai-invoke-history-list">
            <div
              v-for="rec in invokeHistory.slice(1, 11)"
              :key="rec.invoke_id"
              class="history-item"
              @click="fillFromHistory(rec)"
            >
              <span class="time">{{ formatTime(rec.timestamp) }}</span>
              <span class="cap">{{ rec.capability }}</span>
              <span class="status-dot" :class="rec.status"></span>
            </div>
          </div>
        </div>
        <div class="ai-invoke-empty" v-if="!invokeHistory.length">
          <div class="ai-invoke-empty__icon">
            <svg viewBox="0 0 32 32" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
              <path d="M16 4l3 9h9l-7.5 5.5L23 28l-7-5.5L9 28l2.5-9.5L4 13h9z" opacity="0.3" />
            </svg>
          </div>
          <p>暂无调用记录</p>
          <p class="hint">选择 Capability 并执行调用</p>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ai-invoke { height: 100%; }
.ai-invoke-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  height: 100%;
}
.ai-invoke-left, .ai-invoke-right {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 0;
}
.ai-invoke-section {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.ai-invoke-label {
  font-size: 10.5px;
  color: var(--ai-text-faint, var(--text-tertiary));
  text-transform: uppercase;
  letter-spacing: 0.5px;
  font-weight: 600;
}
.ai-invoke-desc {
  font-size: 12px;
  color: var(--ai-text-muted, var(--text-secondary));
  margin: 0;
  line-height: 1.5;
}
.ai-invoke-textarea {
  width: 100%;
  font-family: 'SF Mono', 'Cascadia Code', 'Consolas', monospace;
  font-size: 12px;
  line-height: 1.6;
  padding: 10px 12px;
  border: 1px solid var(--ai-glass-border, var(--border));
  border-radius: 10px;
  background: var(--ai-glass-bg-subtle, var(--bg));
  color: var(--ai-text, var(--text-primary));
  resize: vertical;
  transition: border-color 200ms, box-shadow 200ms;
}
.ai-invoke-textarea:focus {
  outline: none;
  border-color: var(--primary);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--primary) 10%, transparent);
}

/* ── 选项行 ── */
.ai-invoke-options {
  display: flex;
  gap: 14px;
  align-items: center;
  padding: 2px 0;
}
.ai-invoke-opt {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--ai-text-muted, var(--text-secondary));
}
.ai-invoke-opt label {
  font-size: 11px;
  color: var(--ai-text-faint, var(--text-tertiary));
  font-weight: 500;
}
.ai-invoke-opt .unit {
  font-size: 11px;
  color: var(--ai-text-faint, var(--text-tertiary));
}

/* ── 结果面板 ── */
.ai-invoke-result {
  border: 1px solid var(--ai-glass-border, var(--border));
  border-radius: 10px;
  padding: 10px 12px;
  background: var(--ai-glass-bg-card, var(--bg));
  backdrop-filter: blur(20px);
  box-shadow:
    0 1px 3px rgba(0, 0, 0, 0.03),
    inset 0 1px 0 var(--ai-glass-highlight, transparent);
}
.ai-invoke-result-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
.ai-invoke-result-header .label {
  font-size: 12px;
  font-weight: 600;
  color: var(--ai-text, var(--text-primary));
}
.ai-invoke-result-header .latency {
  font-size: 11px;
  color: var(--ai-text-faint, var(--text-tertiary));
  margin-left: auto;
  font-family: 'SF Mono', 'Cascadia Code', 'Consolas', monospace;
}
.ai-invoke-noresult {
  text-align: center;
  padding: 20px;
  color: var(--ai-text-muted, var(--text-secondary));
  font-size: 12px;
}

/* ── 调用历史 ── */
.ai-invoke-history {
  margin-top: 2px;
}
.history-title {
  font-size: 10.5px;
  color: var(--ai-text-faint, var(--text-tertiary));
  text-transform: uppercase;
  letter-spacing: 0.5px;
  font-weight: 600;
}
.ai-invoke-history-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-top: 4px;
}
.history-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 8px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 160ms cubic-bezier(0.16, 1, 0.3, 1);
}
.history-item:hover {
  background: var(--ai-glass-bg-subtle, rgba(0, 0, 0, 0.03));
  transform: translateX(2px);
}
.history-item .time {
  font-size: 11px;
  color: var(--ai-text-faint, var(--text-tertiary));
  font-family: 'SF Mono', 'Cascadia Code', 'Consolas', monospace;
  flex-shrink: 0;
}
.history-item .cap {
  font-size: 12px;
  color: var(--ai-text-muted, var(--text-secondary));
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.history-item .status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  flex-shrink: 0;
  background: var(--ai-text-faint, var(--text-tertiary));
}
.history-item .status-dot.completed {
  background: #10b981;
  box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.15);
}
.history-item .status-dot.failed {
  background: #ef4444;
  box-shadow: 0 0 0 2px rgba(239, 68, 68, 0.15);
}
.history-item .status-dot.running,
.history-item .status-dot.queued {
  background: var(--primary);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--primary) 15%, transparent);
}

/* ── 空状态 ── */
.ai-invoke-empty {
  text-align: center;
  padding: 40px 20px;
  color: var(--ai-text-muted, var(--text-secondary));
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}
.ai-invoke-empty__icon {
  color: var(--ai-text-faint, var(--text-tertiary));
  opacity: 0.3;
  margin-bottom: 8px;
}
.ai-invoke-empty__icon svg {
  width: 40px;
  height: 40px;
}
.ai-invoke-empty .hint {
  font-size: 12px;
  color: var(--ai-text-faint, var(--text-tertiary));
}

/* ── 暗色主题 ── */
body.dark-theme .ai-invoke-textarea {
  background: rgba(13, 18, 34, 0.6);
}
body.dark-theme .ai-invoke-result {
  background: rgba(13, 18, 34, 0.6);
  border-color: rgba(255, 255, 255, 0.08);
}
</style>
