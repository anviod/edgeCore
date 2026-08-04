<template>
  <div class="ai-mcp-help">
    <!-- MCP Server 状态 -->
    <div class="ai-workbench-section">
      <h3 class="ai-workbench-section__title">MCP Server 状态</h3>
      <div class="ai-mcp-status">
        <span class="ai-mcp-status__dot" :class="mcpStatus ? 'ai-mcp-status__dot--online' : 'ai-mcp-status__dot--offline'"></span>
        <span class="ai-mcp-status__label">{{ mcpStatus ? '运行中' : '检查中...' }}</span>
        <span v-if="mcpInfo" class="ai-mcp-status__version">MCP {{ mcpInfo.protocol }}</span>
      </div>
      <div v-if="mcpInfo" class="ai-mcp-info-grid">
        <div class="ai-mcp-info-item">
          <span class="ai-mcp-info-item__label">传输协议</span>
          <code>{{ mcpInfo.transport }}</code>
        </div>
        <div class="ai-mcp-info-item">
          <span class="ai-mcp-info-item__label">端点</span>
          <code>{{ mcpInfo.endpoint }}</code>
        </div>
        <div class="ai-mcp-info-item">
          <span class="ai-mcp-info-item__label">工具数</span>
          <code>{{ mcpInfo.tools || 0 }}</code>
        </div>
        <div class="ai-mcp-info-item">
          <span class="ai-mcp-info-item__label">认证方式</span>
          <code>{{ mcpInfo.auth_mode || 'api_key' }}</code>
        </div>
      </div>
    </div>

    <!-- 接入方式 -->
    <div class="ai-workbench-section">
      <h3 class="ai-workbench-section__title">接入方式</h3>
      <p class="ai-workbench-section__hint">
        外部 LLM 应用通过 MCP 协议安全操作 EdgeX 工业网关。使用 MCP API Key 简化认证（无需 JWT）。
      </p>

      <div class="ai-mcp-client-tabs">
        <button
          v-for="c in clients"
          :key="c.name"
          type="button"
          class="ai-mcp-client-tab"
          :class="{ 'ai-mcp-client-tab--active': activeClient === c.name }"
          @click="activeClient = c.name"
        >
          {{ c.name }}
        </button>
      </div>

      <div class="ai-mcp-config-wrap">
        <div class="ai-mcp-config-head">
          <span class="ai-mcp-config-head__label">配置示例</span>
          <button
            type="button"
            class="ai-mcp-config-copy"
            @click="copyConfig"
            :title="copied ? '已复制' : '复制配置'"
          >
            <svg v-if="copied" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12" /></svg>
            <svg v-else width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2" /><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" /></svg>
            <span>{{ copied ? '已复制' : '复制' }}</span>
          </button>
        </div>
        <pre class="ai-mcp-config-code"><code>{{ currentConfig }}</code></pre>
      </div>
    </div>

    <!-- MCP 工具清单 -->
    <div class="ai-workbench-section">
      <div class="ai-mcp-section-head">
        <h3 class="ai-workbench-section__title">MCP 工具清单 ({{ toolList.length }} 个)</h3>
        <span class="ai-mcp-section-hint">按权限分组，点击标题折叠</span>
      </div>

      <!-- 只读工具 -->
      <div class="ai-mcp-foldable" :class="{ 'ai-mcp-foldable--open': expandedReadTools }">
        <button class="ai-mcp-foldable__head" @click="toggleReadTools">
          <span class="ai-mcp-tool-category__label">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" /><circle cx="12" cy="12" r="3" /></svg>
            只读查询（{{ readTools.length }} 个）
          </span>
          <span class="ai-mcp-foldable__chevron">
            <svg viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 4.5L6 7.5L9 4.5" /></svg>
          </span>
        </button>
        <transition name="ai-mcp-fold">
          <div v-show="expandedReadTools" class="ai-mcp-foldable__body">
            <div class="ai-mcp-tools-list">
              <div
                v-for="tool in readTools"
                :key="tool.name"
                class="ai-mcp-tool-card"
              >
                <div class="ai-mcp-tool-card__head">
                  <code class="ai-mcp-tool-card__name">{{ tool.name }}</code>
                </div>
                <p class="ai-mcp-tool-card__desc">{{ tool.desc }}</p>
              </div>
            </div>
          </div>
        </transition>
      </div>

      <!-- 全功能工具 -->
      <div class="ai-mcp-foldable" :class="{ 'ai-mcp-foldable--open': expandedWriteTools }">
        <button class="ai-mcp-foldable__head" @click="toggleWriteTools">
          <span class="ai-mcp-tool-category__label">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" /></svg>
            全功能 CRUD（{{ writeTools.length }} 个）
          </span>
          <span class="ai-mcp-foldable__chevron">
            <svg viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 4.5L6 7.5L9 4.5" /></svg>
          </span>
        </button>
        <transition name="ai-mcp-fold">
          <div v-show="expandedWriteTools" class="ai-mcp-foldable__body">
            <div class="ai-mcp-tools-list">
              <div
                v-for="tool in writeTools"
                :key="tool.name"
                class="ai-mcp-tool-card"
                :class="{ 'ai-mcp-tool-card--locked': !mcpFullAccess }"
              >
                <div class="ai-mcp-tool-card__head">
                  <code class="ai-mcp-tool-card__name">{{ tool.name }}</code>
                  <span v-if="!mcpFullAccess" class="ai-mcp-tool-card__badge ai-mcp-tool-card__badge--locked">
                    <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2" ry="2" /><path d="M7 11V7a5 5 0 0 1 10 0v4" /></svg>
                  </span>
                </div>
                <p class="ai-mcp-tool-card__desc">{{ tool.desc }}</p>
              </div>
            </div>
          </div>
        </transition>
      </div>
    </div>

    <!-- 安全说明 -->
    <div class="ai-workbench-section">
      <div class="ai-mcp-foldable" :class="{ 'ai-mcp-foldable--open': expandedSecurity }">
        <button class="ai-mcp-foldable__head ai-mcp-foldable__head--title" @click="toggleSecurity">
          <h3 class="ai-workbench-section__title" style="margin: 0;">安全说明</h3>
          <span class="ai-mcp-foldable__chevron">
            <svg viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 4.5L6 7.5L9 4.5" /></svg>
          </span>
        </button>
        <transition name="ai-mcp-fold">
          <div v-show="expandedSecurity" class="ai-mcp-foldable__body">
            <ul class="ai-mcp-security">
              <li>全功能 CRUD 操作（创建/删除/写入）需要用户在 UI 中确认激活</li>
              <li>所有操作通过 MCP API Key 认证（<code>Authorization: Bearer &lt;key&gt;</code> 或 <code>X-MCP-API-Key</code>）</li>
              <li>MCP API Key 独立于系统 JWT，可随时更换</li>
              <li>敏感配置信息（API Key、密码）已脱敏处理</li>
              <li>MCP 端点仅在内网暴露，建议配合防火墙规则使用</li>
            </ul>
          </div>
        </transition>
      </div>
    </div>

    <div class="ai-mcp-footer">
      <a-button type="primary" size="small" @click="refreshStatus">
        {{ loading ? '检查中...' : '刷新状态' }}
      </a-button>
      <a-button size="small" @click="openMCPDocs">
        查看完整文档
      </a-button>
    </div>

    <!-- MCP 完整文档抽屉 -->
    <a-drawer
      v-model:visible="docsVisible"
      title="MCP 接入完整文档"
      :width="980"
      :footer="false"
      unmount-on-close
      render-to-body
      class="ai-mcp-docs-drawer"
    >
      <div class="ai-mcp-docs-content" v-html="docsHtml"></div>
    </a-drawer>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Message } from '@arco-design/web-vue'

const mcpStatus = ref(false)
const mcpInfo = ref(null)
const loading = ref(false)
const activeClient = ref('Claude Desktop')
const copied = ref(false)

// MCP 激活状态（只读，由 refreshStatus 刷新）
const mcpFullAccess = ref(false)
const mcpApiKeySet = ref(false)

// 文档抽屉
const docsVisible = ref(false)
const docsHtml = ref('')

// 折叠面板状态
const expandedReadTools = ref(true)
const expandedWriteTools = ref(false)
const expandedSecurity = ref(true)
const toggleReadTools = () => { expandedReadTools.value = !expandedReadTools.value }
const toggleWriteTools = () => { expandedWriteTools.value = !expandedWriteTools.value }
const toggleSecurity = () => { expandedSecurity.value = !expandedSecurity.value }

// 客户端列表
const clients = [
  { name: 'Claude Desktop', config: '{"mcpServers":{"edgex":{"url":"<host>/api/mcp","headers":{"Authorization":"Bearer <mcp_api_key>"}}}}' },
  { name: 'Cursor', config: '{"mcpServers":{"edgex":{"url":"<host>/api/mcp","headers":{"Authorization":"Bearer <mcp_api_key>"}}}}' },
  { name: 'Windsurf', config: '{"mcpServers":{"edgex":{"url":"<host>/api/mcp","headers":{"Authorization":"Bearer <mcp_api_key>"}}}}' },
  { name: 'Continue.dev', config: '{"mcpServers":{"edgex":{"transport":{"type":"http","url":"<host>/api/mcp"},"auth":{"type":"bearer","token":"<mcp_api_key>"}}}}' }
]

// 只读工具
const readTools = [
  { name: 'edgex_list_channels', desc: '列出所有采集通道及其状态', category: 'read' },
  { name: 'edgex_list_devices', desc: '列出指定通道下的所有设备', category: 'read' },
  { name: 'edgex_list_points', desc: '列出指定设备下的所有点位（含当前值）', category: 'read' },
  { name: 'edgex_read_point', desc: '读取指定点位的当前实时值', category: 'read' },
  { name: 'edgex_get_system_info', desc: '获取 EdgeX 网关系统信息', category: 'read' },
  { name: 'edgex_get_diagnostics', desc: '获取通道或设备的诊断信息', category: 'read' },
  { name: 'edgex_analyze_protocol', desc: '分析工业协议特征（端口/名称匹配）', category: 'read' },
  { name: 'edgex_get_protocol_help', desc: '获取指定工业协议的接入帮助', category: 'read' }
]

// 全功能 CRUD 工具
const writeTools = [
  { name: 'edgex_write_point', desc: '向指定点位写入控制值', category: 'write' },
  { name: 'edgex_read_point_batch', desc: '批量读取多个点位实时值（测试验证）', category: 'write' },
  { name: 'edgex_write_point_batch', desc: '批量写入多个点位值（测试验证）', category: 'write' },
  { name: 'edgex_create_channel', desc: '创建南向采集通道（自动配置协议驱动）', category: 'write' },
  { name: 'edgex_delete_channel', desc: '删除指定通道（含设备/点位）', category: 'write' },
  { name: 'edgex_start_channel', desc: '启动通道采集引擎', category: 'write' },
  { name: 'edgex_stop_channel', desc: '停止通道采集引擎', category: 'write' },
  { name: 'edgex_create_device', desc: '在通道下创建设备（自动配置从站地址）', category: 'write' },
  { name: 'edgex_delete_device', desc: '删除指定设备（含点位）', category: 'write' },
  { name: 'edgex_create_point', desc: '创建设备采集点位（自动配置地址/类型/缩放）', category: 'write' },
  { name: 'edgex_delete_point', desc: '删除指定点位', category: 'write' },
  { name: 'edgex_create_edge_rule', desc: '创建边缘计算规则（阈值/计算/状态/窗口）', category: 'write' },
  { name: 'edgex_delete_edge_rule', desc: '删除边缘计算规则', category: 'write' },
  { name: 'edgex_create_virtual_device', desc: '创建虚拟设备（公式计算，不占用物理连接）', category: 'write' }
]

const toolList = computed(() => [...readTools, ...writeTools])

const currentConfig = computed(() => {
  const client = clients.find(c => c.name === activeClient.value)
  if (!client) return ''

  const host = window.location.origin
  return client.config.replace('<host>', host).replace('<mcp_api_key>', '<your-mcp-api-key>')
})

// 复制配置
function copyConfig() {
  if (!currentConfig.value) return
  navigator.clipboard.writeText(currentConfig.value).then(() => {
    copied.value = true
    setTimeout(() => { copied.value = false }, 2000)
  }).catch(() => {
    Message.warning('复制失败，请手动选择复制')
  })
}

// 刷新状态
async function refreshStatus() {
  loading.value = true
  try {
    const token = getAuthToken()
    const headers = { 'Content-Type': 'application/json' }
    if (token) {
      headers['Authorization'] = `Bearer ${token}`
    }

    const resp = await fetch('/api/mcp', { method: 'POST', headers })
    if (resp.ok) {
      mcpInfo.value = await resp.json()
      mcpStatus.value = true
    } else {
      mcpStatus.value = false
    }

    const statusResp = await fetch('/api/mcp/status', { headers })
    if (statusResp.ok) {
      const statusData = await statusResp.json()
      if (statusData.code === '0') {
        mcpFullAccess.value = statusData.data.mcp_full_access
        mcpApiKeySet.value = statusData.data.mcp_api_key_set
      }
    }
  } catch {
    mcpStatus.value = false
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  refreshStatus()
})

// 获取 JWT token（与 request.js 一致）
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

// 内联打开 MCP 文档
async function openMCPDocs() {
  docsVisible.value = true
  if (docsHtml.value) return

  try {
    const token = getAuthToken()
    const headers = { 'Content-Type': 'application/json' }
    if (token) {
      headers['Authorization'] = `Bearer ${token}`
    }
    const resp = await fetch('/api/mcp/help', { headers })
    if (resp.ok) {
      const data = await resp.json()
      docsHtml.value = renderHelpDoc(data)
    } else {
      docsHtml.value = `<p class="ai-mcp-docs-error">请求失败 (${resp.status})：请确认已登录系统</p>`
    }
  } catch {
    docsHtml.value = '<p class="ai-mcp-docs-error">无法加载文档，请检查网络连接</p>'
  }
}

function summarizeText(text, maxLen) {
  if (!text) return ''
  if (text.length <= maxLen) return text
  const stopChars = ['。', '！', '？', '. ', '\n']
  for (const ch of stopChars) {
    const idx = text.indexOf(ch, Math.min(maxLen * 0.6, maxLen - 20))
    if (idx > 0 && idx < maxLen) return text.slice(0, idx + ch.length)
  }
  return text.slice(0, maxLen) + '...'
}

function renderToolCard(t) {
  const full = esc(t.description || '')
  const summary = esc(summarizeText(t.description || '', 120))
  const hasLongDesc = (t.description || '').length > 120

  let paramsHtml = ''
  if (Array.isArray(t.parameters) && t.parameters.length) {
    const items = t.parameters.map(p => {
      const req = p.required ? '<span class="ai-mcp-docs-tool-required">*</span>' : ''
      const type = p.type ? `<code class="ai-mcp-docs-tool-type">${esc(p.type)}</code>` : ''
      const def = p.default !== undefined ? ` <span class="ai-mcp-docs-tool-default">默认 ${esc(String(p.default))}</span>` : ''
      return `<li><div class="ai-mcp-docs-tool-param__name">${esc(p.name)}${req} ${type}${def}</div><div class="ai-mcp-docs-tool-param__desc">${esc(p.description || '')}</div></li>`
    }).join('')
    paramsHtml = `<div class="ai-mcp-docs-tool-section"><h5>参数</h5><ul class="ai-mcp-docs-tool-list">${items}</ul></div>`
  }

  let returnsHtml = ''
  const ret = t.returns || t.return
  if (ret) {
    const retText = typeof ret === 'string' ? ret : JSON.stringify(ret, null, 2)
    returnsHtml = `<div class="ai-mcp-docs-tool-section"><h5>返回</h5><pre class="ai-mcp-docs-tool-returns"><code>${esc(retText)}</code></pre></div>`
  }

  if (!hasLongDesc && !paramsHtml && !returnsHtml) {
    return `<div class="ai-mcp-docs-tool-card"><code class="ai-mcp-docs-tool-card__name">${esc(t.name)}</code><p class="ai-mcp-docs-tool-card__desc">${full}</p></div>`
  }

  return `<details class="ai-mcp-docs-tool-card">
    <summary>
      <code class="ai-mcp-docs-tool-card__name">${esc(t.name)}</code>
      <span class="ai-mcp-docs-tool-summary">${summary}</span>
      <span class="ai-mcp-docs-tool-toggle"><span class="show-when-closed">展开</span><span class="show-when-open">收起</span></span>
    </summary>
    <div class="ai-mcp-docs-tool-body">
      <p class="ai-mcp-docs-tool-card__desc">${full.replace(/\n/g, '<br>')}</p>
      ${paramsHtml}
      ${returnsHtml}
    </div>
  </details>`
}

function renderHelpDoc(data) {
  if (!data) return '<p class="ai-mcp-docs-error">无数据</p>'

  let html = ''

  // ── Hero 头部 ──
  html += `<header class="ai-mcp-docs-hero">
    <h2 class="ai-mcp-docs-hero__title">${esc(data.title || '')}</h2>
    <p class="ai-mcp-docs-hero__desc">${esc(data.description || '')}</p>
  </header>`

  // ── 架构流程图 ──
  if (data.architecture?.layers?.length) {
    html += `<section class="ai-mcp-docs-section">
      <h3 class="ai-mcp-docs-section__title">系统架构</h3>
      <div class="ai-mcp-docs-arch">
        ${data.architecture.layers.map((l, i) => {
          const colorMap = { purple: '#8b5cf6', blue: '#3b82f6', green: '#22c55e', orange: '#f59e0b' }
          const bgMap = { purple: 'rgba(139,92,246,0.12)', blue: 'rgba(59,130,246,0.12)', green: 'rgba(34,197,94,0.12)', orange: 'rgba(245,158,11,0.12)' }
          const c = colorMap[l.color] || '#6b7280'
          const bg = bgMap[l.color] || 'rgba(107,114,128,0.12)'
          let nodes = `<div class="ai-mcp-docs-arch__node" style="border-color:${c};background:${bg}">
            <strong>${esc(l.name)}</strong><br><small>${esc(l.desc)}</small>
          </div>`
          let arrow = i < data.architecture.layers.length - 1
            ? `<div class="ai-mcp-docs-arch__arrow">&#x2193;</div>` : ''
          return nodes + arrow
        }).join('')}
      </div>
    </section>`
  }

  // ── 传输协议 ──
  html += `<section class="ai-mcp-docs-section">
    <h3 class="ai-mcp-docs-section__title">传输协议</h3>
    <div class="ai-mcp-docs-grid">
      <div class="ai-mcp-docs-grid__item">
        <span class="ai-mcp-docs-grid__label">传输方式</span>
        <code>${esc(data.transport || '')}</code>
      </div>
      <div class="ai-mcp-docs-grid__item">
        <span class="ai-mcp-docs-grid__label">端点</span>
        <code>${esc(data.endpoint || '')}</code>
      </div>
      <div class="ai-mcp-docs-grid__item">
        <span class="ai-mcp-docs-grid__label">MCP 版本</span>
        <code>2024-11-05 / 2025-11-25</code>
      </div>
      <div class="ai-mcp-docs-grid__item">
        <span class="ai-mcp-docs-grid__label">认证方式</span>
        <code>${esc(data.auth_mode || '')}</code>
      </div>
    </div>
    <div class="ai-mcp-docs-card">
      <h4>认证头</h4>
      <pre class="ai-mcp-docs-code"><code>${esc(data.auth_header || '')}</code></pre>
    </div>
  </section>`

  // ── 服务状态 ──
  html += `<section class="ai-mcp-docs-section">
    <h3 class="ai-mcp-docs-section__title">服务状态</h3>
    <div class="ai-mcp-docs-grid">
      <div class="ai-mcp-docs-grid__item">
        <span class="ai-mcp-docs-grid__label">MCP 已启用</span>
        <span class="ai-mcp-docs-badge" style="background:${data.mcp_enabled ? 'rgba(34,197,94,0.12)' : 'rgba(107,114,128,0.12)'};color:${data.mcp_enabled ? '#16a34a' : '#6b7280'}">${data.mcp_enabled ? '是' : '否'}</span>
      </div>
      <div class="ai-mcp-docs-grid__item">
        <span class="ai-mcp-docs-grid__label">全功能激活</span>
        <span class="ai-mcp-docs-badge" style="background:${data.full_access ? 'rgba(34,197,94,0.12)' : 'rgba(245,158,11,0.12)'};color:${data.full_access ? '#16a34a' : '#d97706'}">${data.full_access ? '已激活' : '未激活'}</span>
      </div>
    </div>
  </section>`

  // ── 客户端配置 ──
  if (data.clients?.length) {
    html += `<section class="ai-mcp-docs-section">
      <h3 class="ai-mcp-docs-section__title">客户端接入配置</h3>
      <p class="ai-mcp-docs-section__text">将以下 JSON 配置添加到对应 MCP 客户端的配置文件中，替换 <code>&lt;host&gt;</code> 和 <code>&lt;mcp_api_key&gt;</code>。</p>`

    for (const c of data.clients) {
      html += `<div class="ai-mcp-docs-card">
        <h4>${esc(c.name)}</h4>
        <pre class="ai-mcp-docs-code"><code>${esc(c.config)}</code></pre>
      </div>`
    }
    html += `</section>`
  }

  // ── MCP 工具清单 ──
  if (data.tools?.length) {
    const readTools = data.tools.filter(t => t.category === 'read')
    const writeTools = data.tools.filter(t => t.category === 'write')

    html += `<section class="ai-mcp-docs-section">
      <h3 class="ai-mcp-docs-section__title">MCP 工具清单 (${data.tools.length} 个)</h3>`

    // 只读工具
    html += `<h4 class="ai-mcp-docs-subtitle">
      <span class="ai-mcp-docs-dot" style="background:#22c55e"></span> 只读查询 (${readTools.length} 个)
      <span class="ai-mcp-docs-subtitle__hint">无需全功能激活，默认可用</span>
    </h4>`
    html += `<div class="ai-mcp-docs-tool-grid">`
    for (const t of readTools) html += renderToolCard(t)
    html += `</div>`

    // 全功能工具
    html += `<h4 class="ai-mcp-docs-subtitle">
      <span class="ai-mcp-docs-dot" style="background:#f59e0b"></span> 全功能 CRUD (${writeTools.length} 个)
      <span class="ai-mcp-docs-subtitle__hint">需激活全功能读写</span>
    </h4>`
    html += `<div class="ai-mcp-docs-tool-grid">`
    for (const t of writeTools) html += renderToolCard(t)
    html += `</div>`
    html += `</section>`
  }

  // ── 提示词模板 ──
  if (data.prompts?.length) {
    html += `<section class="ai-mcp-docs-section">
      <h3 class="ai-mcp-docs-section__title">提示词模板 (${data.prompts.length} 个)</h3>
      <p class="ai-mcp-docs-section__text">提示词模板为 LLM 提供结构化的指导，帮助快速完成协议接入、点位配置、规则构建等任务。</p>
      <div class="ai-mcp-docs-prompt-grid">`
    for (const p of data.prompts) {
      const args = (p.arguments || []).map(a => esc(a.name) + (a.required ? '*' : '')).join(', ')
      html += `<div class="ai-mcp-docs-prompt-card">
        <div class="ai-mcp-docs-prompt-card__head">
          <code class="ai-mcp-docs-prompt-card__name">${esc(p.name)}</code>
          ${args ? `<span class="ai-mcp-docs-prompt-card__args">参数: ${args}</span>` : ''}
        </div>
        <p class="ai-mcp-docs-prompt-card__desc">${esc(p.description)}</p>
      </div>`
    }
    html += `</div></section>`
  }

  // ── 资源端点 ──
  if (data.resources?.length) {
    html += `<section class="ai-mcp-docs-section">
      <h3 class="ai-mcp-docs-section__title">资源端点 (${data.resources.length} 个)</h3>
      <p class="ai-mcp-docs-section__text">资源端点以结构化 JSON 格式提供 EdgeX 网关的实时配置数据，LLM 可通过 <code>resources/read</code> 直接读取。</p>
      <div class="ai-mcp-docs-resource-grid">`
    for (const r of data.resources) {
      html += `<div class="ai-mcp-docs-resource-card">
        <code class="ai-mcp-docs-resource-card__uri">${esc(r.uri)}</code>
        <span class="ai-mcp-docs-resource-card__name">${esc(r.name)}</span>
        <span class="ai-mcp-docs-resource-card__mime">${esc(r.mimeType || 'application/json')}</span>
        <p class="ai-mcp-docs-resource-card__desc">${esc(r.description)}</p>
      </div>`
    }
    html += `</div></section>`
  }

  // ── 安全说明 ──
  html += `<section class="ai-mcp-docs-section">
    <h3 class="ai-mcp-docs-section__title">安全说明</h3>
    <div class="ai-mcp-docs-card">
      <ul class="ai-mcp-docs-security-list">
        <li>全功能 CRUD 操作（创建/删除/写入）需要用户在 UI 中确认激活</li>
        <li>所有操作通过 <strong>MCP API Key</strong> 认证（<code>Authorization: Bearer &lt;key&gt;</code> 或 <code>X-MCP-API-Key</code>）</li>
        <li>MCP API Key 独立于系统 JWT，可随时在 UI 中更换</li>
        <li>敏感配置信息（API Key、密码）已脱敏处理</li>
        <li>MCP 端点仅在内网暴露，建议配合防火墙规则使用</li>
        <li>全功能激活状态会持久化保存，重启后保持</li>
      </ul>
    </div>
  </section>`

  // ── API 端点 ──
  html += `<section class="ai-mcp-docs-section">
    <h3 class="ai-mcp-docs-section__title">API 端点</h3>
    <div class="ai-mcp-docs-grid">
      <div class="ai-mcp-docs-grid__item">
        <span class="ai-mcp-docs-grid__label">MCP 协议接入</span>
        <code>POST ${esc(data.endpoint || '/api/mcp')}</code>
      </div>
      <div class="ai-mcp-docs-grid__item">
        <span class="ai-mcp-docs-grid__label">激活全功能</span>
        <code>POST /api/mcp/activate</code>
      </div>
      <div class="ai-mcp-docs-grid__item">
        <span class="ai-mcp-docs-grid__label">查询状态</span>
        <code>GET /api/mcp/status</code>
      </div>
      <div class="ai-mcp-docs-grid__item">
        <span class="ai-mcp-docs-grid__label">帮助文档</span>
        <code>GET /api/mcp/help</code>
      </div>
    </div>
  </section>`

  return html
}

function esc(s) {
  if (!s) return ''
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}
</script>