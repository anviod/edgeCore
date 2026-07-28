import { ref, computed } from 'vue'
import eanApi from '@/api/ean'

/**
 * EAN 2.0 Capability Runtime — 全局状态管理
 * 采用与 useAiCopilot 相同的模块级单例模式
 */

// ── 模块级单例状态 ──
const agentStatus = ref(null)
const capabilities = ref([])
const discoveredAgents = ref([])
const events = ref([])
const settings = ref(null)
const loading = ref(false)
const error = ref(null)

// Invoke 历史（内存，最多 20 条）
const invokeHistory = ref([])
const activeInvoke = ref(null)

// Event 轮询
let eventTimer = null
let eventPaused = false

// ── computed ──
const capabilityCount = computed(() => capabilities.value.length)

const capabilityByCategory = computed(() => {
  const map = { device: 0, ai: 0, system: 0, workflow: 0 }
  capabilities.value.forEach((c) => {
    const cat = c.category || 'system'
    map[cat] = (map[cat] || 0) + 1
  })
  return map
})

const isOnline = computed(() => agentStatus.value?.status === 'online')

const eventCount = computed(() => events.value.length)

// ── 方法 ──
async function fetchAgentStatus() {
  try {
    const res = await eanApi.getAgentStatus()
    if (res?.code === '0' || res?.code === 0) {
      agentStatus.value = res.data
    }
  } catch (e) {
    // 降级：REST 不可用时返回 null，UI 显示未连接
    agentStatus.value = null
  }
}

async function fetchCapabilities(params = {}) {
  loading.value = true
  try {
    const res = await eanApi.listCapabilities(params)
    if (res?.code === '0' || res?.code === 0) {
      capabilities.value = res.data?.capabilities || res.data || []
    }
  } catch {
    capabilities.value = []
  } finally {
    loading.value = false
  }
}

async function invokeCapability(capability, arguments_, options = {}) {
  const invokeId = `inv-${Date.now()}`
  const record = {
    invoke_id: invokeId,
    capability,
    arguments: arguments_,
    status: 'queued',
    timestamp: Date.now(),
    result: null,
    latency_ms: 0
  }
  activeInvoke.value = record
  invokeHistory.value.unshift(record)
  if (invokeHistory.value.length > 20) invokeHistory.value.pop()

  try {
    record.status = 'running'
    const res = await eanApi.invokeCapability({
      invoke_id: invokeId,
      target: agentStatus.value?.id || '',
      capability,
      arguments: arguments_,
      options: {
        timeout_sec: options.timeout_sec || 10,
        priority: options.priority || 'normal',
        retry: options.retry || 0
      }
    })
    if (res?.code === '0' || res?.code === 0) {
      const data = res.data || {}
      record.status = data.status || 'completed'
      record.result = data.result || data
      record.latency_ms = data.latency_ms || 0
    } else {
      record.status = 'failed'
      record.result = { error: res?.message || 'Invoke failed' }
    }
  } catch (e) {
    record.status = 'failed'
    record.result = { error: e?.message || 'Network error' }
  } finally {
    activeInvoke.value = null
  }
  return record
}

async function fetchDiscoveredAgents() {
  try {
    const res = await eanApi.listDiscoveredAgents()
    if (res?.code === '0' || res?.code === 0) {
      discoveredAgents.value = res.data?.agents || res.data || []
    }
  } catch {
    discoveredAgents.value = []
  }
}

async function fetchEventHistory(limit = 50) {
  try {
    const res = await eanApi.getEventHistory({ limit })
    if (res?.code === '0' || res?.code === 0) {
      events.value = res.data?.events || res.data || []
    }
  } catch {
    events.value = []
  }
}

function pushEvent(event) {
  if (eventPaused) return
  events.value.unshift(event)
  if (events.value.length > 200) events.value.length = 200
}

function pauseEvents() {
  eventPaused = true
}

function resumeEvents() {
  eventPaused = false
}

async function clearEvents() {
  events.value = []
  try {
    await eanApi.clearEventHistory()
  } catch {
    // 后端清除失败时仅清空本地状态
  }
}

async function fetchSettings() {
  try {
    const res = await eanApi.getSettings()
    if (res?.code === '0' || res?.code === 0) {
      settings.value = res.data
    }
  } catch {
    settings.value = null
  }
}

// ── 导出 ──
export function useEan() {
  return {
    // 状态
    agentStatus,
    capabilities,
    discoveredAgents,
    events,
    settings,
    loading,
    error,
    invokeHistory,
    activeInvoke,
    // computed
    capabilityCount,
    capabilityByCategory,
    isOnline,
    eventCount,
    // 方法
    fetchAgentStatus,
    fetchCapabilities,
    invokeCapability,
    fetchDiscoveredAgents,
    fetchEventHistory,
    pushEvent,
    pauseEvents,
    resumeEvents,
    clearEvents,
    fetchSettings
  }
}
