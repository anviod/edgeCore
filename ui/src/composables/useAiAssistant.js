import { ref, watch, onMounted, onUnmounted } from 'vue'

const STORAGE_KEY = 'edgeCore-ai-assistant'

// MCP 连接状态（全局共享，供顶部按钮和悬浮 FAB 使用）
const mcpConnected = ref(false)
let mcpStatusTimer = null

function getAuthToken() {
  try {
    const raw = localStorage.getItem('loginInfo')
    if (raw) {
      const parsed = JSON.parse(raw)
      return parsed.token || (parsed.data && parsed.data.token) || ''
    }
  } catch { /* ignore */ }
  return ''
}

async function checkMcpStatus() {
  try {
    const token = getAuthToken()
    const headers = { 'Content-Type': 'application/json' }
    if (token) headers['Authorization'] = `Bearer ${token}`
    const resp = await fetch('/api/mcp', { method: 'POST', headers })
    mcpConnected.value = resp.ok
  } catch {
    mcpConnected.value = false
  }
}

function startMcpStatusPolling() {
  checkMcpStatus()
  if (mcpStatusTimer) clearInterval(mcpStatusTimer)
  mcpStatusTimer = setInterval(checkMcpStatus, 30000)
}

function stopMcpStatusPolling() {
  if (mcpStatusTimer) {
    clearInterval(mcpStatusTimer)
    mcpStatusTimer = null
  }
}

// splitMode: 'both' = 分栏, 'workspace' = 仅工作台, 'chat' = 仅对话
const defaultState = () => ({
  expanded: false,
  miniMode: false,
  workspace: 'protocol',
  splitMode: 'both',
  position: { x: null, y: null },
  size: { width: 860, height: 620 }
})

const clampPosition = (position, size) => {
  if (position?.x == null || position?.y == null) return { x: null, y: null }
  const w = size?.width || 860
  const h = size?.height || 620
  // 确保面板至少 80px 可见，但不限制最大位置
  const maxX = Math.max(8, (typeof window !== 'undefined' ? window.innerWidth : 1200) - Math.min(w, 80))
  const maxY = Math.max(8, (typeof window !== 'undefined' ? window.innerHeight : 800) - Math.min(h, 40))
  return {
    x: Math.max(8, Math.min(position.x, maxX)),
    y: Math.max(8, Math.min(position.y, maxY))
  }
}

const loadState = () => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw)
      const merged = { ...defaultState(), ...parsed, mode: undefined }
      // Never restore open/collapsed UI flags — always start with FAB visible
      merged.expanded = false
      merged.miniMode = false
      merged.position = clampPosition(merged.position, merged.size)
      return merged
    }
  } catch (e) {
    console.warn('AI assistant state restore failed', e)
  }
  return defaultState()
}

const state = ref(loadState())

watch(
  state,
  (val) => {
    try {
      // Persist layout prefs only — not expanded/miniMode (avoids invisible FAB on reload)
      const toSave = {
        workspace: val.workspace,
        splitMode: val.splitMode,
        position: val.position,
        size: val.size
      }
      localStorage.setItem(STORAGE_KEY, JSON.stringify(toSave))
    } catch (e) {
      console.warn('AI assistant state persist failed', e)
    }
  },
  { deep: true }
)

export const AI_WORKSPACES = [
  {
    id: 'protocol',
    label: '协议工作台',
    goal: 'G0/G1',
    description: 'PCAP / 文档 / 监控表 → 四类生产配置',
    icon: 'protocol'
  },
  {
    id: 'validation',
    label: 'Schema 校验',
    goal: 'G2',
    description: '导入前 Protocol / Point / Driver 规范校验',
    icon: 'validate'
  },
  {
    id: 'cases',
    label: '验证用例',
    goal: 'G3',
    description: '可回放 Validation Case 与证据链',
    icon: 'cases'
  },
  {
    id: 'edge',
    label: '边缘场景',
    goal: 'G4',
    description: '场景描述 → EdgeRule 草稿包',
    icon: 'edge'
  },
  {
    id: 'diagnostics',
    label: '联调诊断',
    goal: 'G5',
    description: '通道健康卡 + 诊断快照',
    icon: 'diagnostics'
  },
  {
    id: 'mcp',
    label: 'MCP 接入',
    goal: 'G6',
    description: 'MCP Server · 外部 LLM 操作接口',
    icon: 'mcp'
  },
  {
    id: 'ean',
    label: 'EAN 协作',
    goal: 'G7',
    description: 'Edge Agent Network · 能力注册与调度',
    icon: 'ean'
  }
]

export const DELIVERABLE_TYPES = [
  { id: 'protocol_model', label: 'Protocol Model', desc: '协议帧特征与地址模型' },
  { id: 'point_definition', label: 'Point Definition', desc: '可导入点位 JSON' },
  { id: 'driver_parameter', label: 'Driver Parameter', desc: 'Channel 连接参数' },
  { id: 'validation_case', label: 'Validation Case', desc: '期望读数与帧证据' }
]

export const PIPELINE_STAGES = [
  { id: 'capture', label: 'Capture', sub: '抓包解帧' },
  { id: 'decode', label: 'Decode', sub: '报文结构' },
  { id: 'semantic', label: 'Semantic', sub: '物理量推理' },
  { id: 'output', label: 'Output', sub: '生产配置' }
]

export function useAiAssistant() {
  onMounted(() => {
    startMcpStatusPolling()
  })
  onUnmounted(() => {
    stopMcpStatusPolling()
  })

  const setExpanded = (value) => {
    state.value.expanded = value
    if (value) state.value.miniMode = false
  }

  const setMiniMode = (value) => {
    state.value.miniMode = value
    if (value) state.value.expanded = true
  }

  const setWorkspace = (id) => {
    state.value.workspace = id
  }

  const setSplitMode = (mode) => {
    state.value.splitMode = mode
  }

  // 循环切换: both → chat → workspace → both
  const toggleSplitMode = () => {
    const map = { both: 'chat', chat: 'workspace', workspace: 'both' }
    state.value.splitMode = map[state.value.splitMode] || 'both'
  }

  const setPosition = (x, y) => {
    state.value.position = clampPosition({ x, y }, state.value.size)
  }

  const setSize = (width, height) => {
    state.value.size = { width, height }
  }

  const collapseToFab = () => {
    state.value.expanded = false
    state.value.miniMode = false
  }

  const getWorkspaceConfig = (id) => {
    return AI_WORKSPACES.find((w) => w.id === id) || AI_WORKSPACES[0]
  }

  return {
    state,
    mcpConnected,
    setExpanded,
    setMiniMode,
    setWorkspace,
    setSplitMode,
    toggleSplitMode,
    setPosition,
    setSize,
    collapseToFab,
    getWorkspaceConfig,
    AI_WORKSPACES,
    DELIVERABLE_TYPES,
    PIPELINE_STAGES
  }
}
