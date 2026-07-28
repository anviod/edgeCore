import request from '@/utils/request'

/**
 * EAN 2.0 Capability Runtime API
 * 对应后端 /api/capability/* 端点（需后端新增 REST 路由）
 * 当前 EAN 能力通过 MCP JSON-RPC (/api/mcp) 暴露，
 * 此 API 层为 UI 专用 REST 封装，后端尚未实现时降级为 MCP 调用。
 */
export default {
  /* ── Agent 状态 ── */
  getAgentStatus() {
    return request({ url: '/api/capability/agent/status', method: 'get' })
  },

  /* ── Capability 注册表 ── */
  listCapabilities(params = {}) {
    return request({ url: '/api/capability/list', method: 'get', params })
  },

  getCapabilityDetail(id) {
    return request({ url: `/api/capability/list/${encodeURIComponent(id)}`, method: 'get' })
  },

  /* ── Capability 调用 ── */
  invokeCapability(data) {
    return request({ url: '/api/capability/invoke', method: 'post', data })
  },

  getInvokeStatus(invokeId) {
    return request({ url: `/api/capability/invoke/${encodeURIComponent(invokeId)}/status`, method: 'get' })
  },

  /* ── Discovery ── */
  listDiscoveredAgents() {
    return request({ url: '/api/capability/discovery/agents', method: 'get' })
  },

  getAgentCapabilities(agentId) {
    return request({ url: `/api/capability/discovery/agents/${encodeURIComponent(agentId)}/capabilities`, method: 'get' })
  },

  /* ── Event ── */
  getEventHistory(params = {}) {
    return request({ url: '/api/capability/events/history', method: 'get', params })
  },

  clearEventHistory() {
    return request({ url: '/api/capability/events/history', method: 'delete' })
  },

  /* ── 设置 ── */
  getSettings() {
    return request({ url: '/api/capability/settings', method: 'get' })
  },

  updateSettings(data) {
    return request({ url: '/api/capability/settings', method: 'put', data })
  },

  /* ── 通过 MCP JSON-RPC 降级调用（后端无 REST 时使用） ── */
  mcpCall(toolName, args = {}) {
    return request({
      url: '/api/mcp',
      method: 'post',
      data: {
        jsonrpc: '2.0',
        id: Date.now(),
        method: 'tools/call',
        params: { name: toolName, arguments: args }
      }
    })
  }
}
