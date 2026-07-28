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
            <a-input-number v-model="timeoutSec" size="small" :min="1" :max="300" style="width: 70px" />
            <span class="unit">s</span>
          </div>
          <div class="ai-invoke-opt">
            <label>优先级</label>
            <a-select v-model="priority" size="small" style="width: 90px"
              :options="[{label:'normal',value:'normal'},{label:'high',value:'high'},{label:'low',value:'low'}]" />
          </div>
          <div class="ai-invoke-opt">
            <label>重试</label>
            <a-input-number v-model="retryCount" size="small" :min="0" :max="5" style="width: 50px" />
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
        <div class="ai-invoke-history" v-if="invokeHistory.length">
          <span class="history-title">调用历史</span>
          <div class="ai-invoke-history-list">
            <div
              v-for="rec in invokeHistory.slice(0, 10)"
              :key="rec.invoke_id"
              class="history-item"
              @click="fillFromHistory(rec)"
            >
              <span class="time">{{ formatTime(rec.timestamp) }}</span>
              <span class="cap">{{ rec.capability }}</span>
              <span class="status" :class="rec.status">{{ rec.status === 'completed' ? '✓' : rec.status === 'failed' ? '✗' : '··' }}</span>
            </div>
          </div>
        </div>
        <div class="ai-invoke-empty" v-if="!invokeHistory.length">
          <p>暂无调用记录</p>
          <p class="hint">选择 Capability 并执行调用</p>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ai-invoke { height: 100%; }
.ai-invoke-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; height: 100%; }
.ai-invoke-left, .ai-invoke-right { display: flex; flex-direction: column; gap: 10px; }
.ai-invoke-section { display: flex; flex-direction: column; gap: 4px; }
.ai-invoke-label { font-size: 11px; color: var(--ai-text-faint, var(--text-tertiary)); text-transform: uppercase; letter-spacing: 0.5px; }
.ai-invoke-desc { font-size: 12px; color: var(--ai-text-muted, var(--text-secondary)); margin: 0; }
.ai-invoke-textarea {
  width: 100%; font-family: monospace; font-size: 12px; line-height: 1.5;
  padding: 8px 10px; border: 1px solid var(--ai-glass-border, var(--border));
  border-radius: 8px; background: var(--ai-glass-bg-subtle, var(--bg));
  color: var(--ai-text, var(--text-primary)); resize: vertical;
}
.ai-invoke-textarea:focus { outline: none; border-color: var(--primary); }
.ai-invoke-options { display: flex; gap: 12px; align-items: center; }
.ai-invoke-opt { display: flex; align-items: center; gap: 4px; font-size: 12px; color: var(--ai-text-muted, var(--text-secondary)); }
.ai-invoke-opt .unit { font-size: 11px; color: var(--ai-text-faint, var(--text-tertiary)); }
.ai-invoke-result {
  border: 1px solid var(--ai-glass-border, var(--border)); border-radius: 10px;
  padding: 10px 12px; background: var(--ai-glass-bg-card, var(--bg));
}
.ai-invoke-result-header { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.ai-invoke-result-header .label { font-size: 12px; font-weight: 500; color: var(--ai-text, var(--text-primary)); }
.ai-invoke-result-header .latency { font-size: 11px; color: var(--ai-text-faint, var(--text-tertiary)); margin-left: auto; }
.ai-invoke-noresult { text-align: center; padding: 16px; color: var(--ai-text-muted, var(--text-secondary)); font-size: 12px; }
.ai-invoke-history { margin-top: 4px; }
.history-title { font-size: 11px; color: var(--ai-text-faint, var(--text-tertiary)); text-transform: uppercase; }
.ai-invoke-history-list { display: flex; flex-direction: column; gap: 2px; margin-top: 4px; }
.history-item {
  display: flex; align-items: center; gap: 8px; padding: 4px 8px;
  border-radius: 6px; cursor: pointer; transition: background 120ms;
}
.history-item:hover { background: var(--ai-glass-bg-subtle, rgba(0,0,0,0.03)); }
.history-item .time { font-size: 11px; color: var(--ai-text-faint, var(--text-tertiary)); font-family: monospace; }
.history-item .cap { font-size: 12px; color: var(--ai-text-muted, var(--text-secondary)); flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.history-item .status { font-size: 12px; }
.history-item .status.completed { color: #10b981; }
.history-item .status.failed { color: #ef4444; }
.ai-invoke-empty { text-align: center; padding: 40px 20px; color: var(--ai-text-muted, var(--text-secondary)); }
.ai-invoke-empty .hint { font-size: 12px; color: var(--ai-text-faint, var(--text-tertiary)); }
body.dark-theme .ai-invoke-textarea { background: rgba(13,18,34,0.6); }
</style>
