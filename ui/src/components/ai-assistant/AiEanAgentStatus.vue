<script setup>
import { onMounted, computed } from 'vue'
import { useEan } from '@/composables/useEan'

const { agentStatus, capabilityCount, capabilityByCategory, isOnline, fetchAgentStatus } = useEan()

const statusText = computed(() => {
  if (!agentStatus.value) return '未连接'
  const s = agentStatus.value.status
  const map = { online: '在线', offline: '离线', degraded: '降级', error: '错误' }
  return map[s] || s
})

onMounted(() => {
  fetchAgentStatus()
})
</script>

<template>
  <div class="ai-ean-status">
    <div class="ai-ean-card" v-if="agentStatus">
      <div class="ai-ean-status__header">
        <span class="ai-ean-status__title">Agent 状态</span>
        <span class="ai-ean-status__badge" :class="isOnline ? 'ok' : 'err'">
          <span class="dot"></span>{{ statusText }}
        </span>
      </div>
      <div class="ai-ean-status__grid">
        <div class="ai-ean-field">
          <span class="label">Agent ID</span>
          <span class="value">{{ agentStatus.id || '-' }}</span>
        </div>
        <div class="ai-ean-field">
          <span class="label">Kind</span>
          <span class="value">{{ agentStatus.kind || 'device' }}</span>
        </div>
        <div class="ai-ean-field">
          <span class="label">Version</span>
          <span class="value">{{ agentStatus.version || '-' }}</span>
        </div>
        <div class="ai-ean-field">
          <span class="label">Transport</span>
          <span class="value">{{ agentStatus.transport || 'mqtt' }}</span>
        </div>
        <div class="ai-ean-field">
          <span class="label">心跳间隔</span>
          <span class="value">{{ agentStatus.heartbeat_interval_sec || 30 }}s</span>
        </div>
        <div class="ai-ean-field">
          <span class="label">Capability 数</span>
          <span class="value highlight">{{ capabilityCount }}</span>
        </div>
      </div>
      <div class="ai-ean-status__cats">
        <span class="cat cat-device">device: {{ capabilityByCategory.device }}</span>
        <span class="cat cat-ai">ai: {{ capabilityByCategory.ai }}</span>
        <span class="cat cat-system">system: {{ capabilityByCategory.system }}</span>
      </div>
    </div>
    <div class="ai-ean-empty" v-else>
      <div class="ai-ean-empty__icon">⚠</div>
      <p class="ai-ean-empty__text">EAN Capability Runtime 未连接</p>
      <p class="ai-ean-empty__hint">请检查北向 edgeOS 通道配置或 MCP 服务状态</p>
      <a-button size="small" type="outline" @click="fetchAgentStatus">重试</a-button>
    </div>
  </div>
</template>

<style scoped>
.ai-ean-status { display: flex; flex-direction: column; gap: 12px; }
.ai-ean-card {
  background: var(--ai-glass-bg-card, var(--bg));
  border: 1px solid var(--ai-glass-border, var(--border));
  border-radius: 14px;
  padding: 16px 20px;
}
.ai-ean-status__header {
  display: flex; justify-content: space-between; align-items: center;
  margin-bottom: 16px;
}
.ai-ean-status__title { font-size: 15px; font-weight: 600; color: var(--ai-text, var(--text-primary)); }
.ai-ean-status__badge {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 3px 10px; border-radius: 9999px; font-size: 12px; font-weight: 500;
}
.ai-ean-status__badge.ok { color: #10b981; background: rgba(16,185,129,0.12); }
.ai-ean-status__badge.err { color: #ef4444; background: rgba(239,68,68,0.12); }
.ai-ean-status__badge .dot { width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
.ai-ean-status__badge.ok .dot { animation: pulse 2s ease-in-out infinite; }
@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }
.ai-ean-status__grid {
  display: grid; grid-template-columns: 1fr 1fr; gap: 12px 24px;
  padding-bottom: 14px; border-bottom: 1px solid var(--ai-glass-border-subtle, var(--border));
}
.ai-ean-field { display: flex; flex-direction: column; gap: 2px; }
.ai-ean-field .label { font-size: 11px; color: var(--ai-text-faint, var(--text-tertiary)); text-transform: uppercase; letter-spacing: 0.5px; }
.ai-ean-field .value { font-size: 14px; color: var(--ai-text, var(--text-primary)); font-weight: 500; }
.ai-ean-field .value.highlight { color: var(--primary); font-size: 16px; }
.ai-ean-status__cats { display: flex; gap: 12px; padding-top: 14px; }
.cat { font-size: 12px; padding: 4px 10px; border-radius: 6px; }
.cat-device { color: #3b82f6; background: rgba(59,130,246,0.1); }
.cat-ai { color: #a78bfa; background: rgba(167,139,250,0.1); }
.cat-system { color: #64748b; background: rgba(100,116,139,0.1); }
.ai-ean-empty { text-align: center; padding: 40px 20px; }
.ai-ean-empty__icon { font-size: 32px; margin-bottom: 8px; }
.ai-ean-empty__text { font-size: 14px; color: var(--ai-text, var(--text-primary)); margin: 0 0 4px; }
.ai-ean-empty__hint { font-size: 12px; color: var(--ai-text-muted, var(--text-secondary)); margin: 0 0 12px; }
</style>
