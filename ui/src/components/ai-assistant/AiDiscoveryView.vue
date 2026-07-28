<script setup>
import { onMounted, ref } from 'vue'
import { useEan } from '@/composables/useEan'

const { discoveredAgents, fetchDiscoveredAgents } = useEan()

const expandedAgent = ref(null)

const toggleAgent = (id) => {
  expandedAgent.value = expandedAgent.value === id ? null : id
}

const formatLastSeen = (seconds) => {
  if (!seconds && seconds !== 0) return '-'
  if (seconds < 60) return `${seconds}s ago`
  if (seconds < 3600) return `${Math.floor(seconds/60)}m ago`
  return `${Math.floor(seconds/3600)}h ago`
}

onMounted(() => {
  fetchDiscoveredAgents()
})
</script>

<template>
  <div class="ai-disc">
    <div class="ai-disc-header">
      <span class="title">Agent 发现</span>
      <a-button size="mini" type="text" @click="fetchDiscoveredAgents()">刷新</a-button>
    </div>
    <div class="ai-disc-grid" v-if="discoveredAgents.length">
      <div
        v-for="agent in discoveredAgents"
        :key="agent.id"
        class="ai-disc-card"
        :class="{ offline: agent.status !== 'online' }"
      >
        <div class="ai-disc-card__header" @click="toggleAgent(agent.id)">
          <span class="ai-disc-card__id">{{ agent.id }}</span>
          <span class="ai-disc-card__status" :class="agent.status === 'online' ? 'ok' : 'off'">
            <span class="dot"></span>{{ agent.status === 'online' ? 'online' : 'offline' }}
          </span>
        </div>
        <div class="ai-disc-card__meta">
          <span class="meta-item">{{ agent.kind || 'device' }}</span>
          <span class="meta-item">{{ agent.version || '-' }}</span>
          <span class="meta-item">{{ agent.transport || 'mqtt' }}</span>
        </div>
        <div class="ai-disc-card__stats">
          <span class="stat">Cap: {{ agent.capabilities_count || 0 }}</span>
          <span class="stat">Last: {{ formatLastSeen(agent.last_seen_seconds_ago) }}</span>
        </div>
        <div class="ai-disc-card__caps" v-if="expandedAgent === agent.id && agent.capabilities">
          <div v-for="cap in agent.capabilities" :key="cap.id" class="cap-item">
            <span class="cap-id">{{ cap.id }}</span>
            <span class="cap-cat" :class="`cat-${cap.category}`">{{ cap.category }}</span>
          </div>
        </div>
      </div>
    </div>
    <div class="ai-disc-empty" v-else>
      <p>未发现其他 Agent</p>
      <p class="hint">确保 EdgeOS 已连接且 Discovery 已启用</p>
      <a-button size="small" type="outline" @click="fetchDiscoveredAgents()">刷新</a-button>
    </div>
  </div>
</template>

<style scoped>
.ai-disc { display: flex; flex-direction: column; gap: 10px; }
.ai-disc-header { display: flex; justify-content: space-between; align-items: center; }
.ai-disc-header .title { font-size: 14px; font-weight: 600; color: var(--ai-text, var(--text-primary)); }
.ai-disc-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
.ai-disc-card {
  border: 1px solid var(--ai-glass-border, var(--border)); border-radius: 10px;
  padding: 12px; background: var(--ai-glass-bg-card, var(--bg));
  transition: border-color 160ms, opacity 160ms;
}
.ai-disc-card:hover { border-color: var(--primary); }
.ai-disc-card.offline { opacity: 0.5; }
.ai-disc-card__header { display: flex; justify-content: space-between; align-items: center; cursor: pointer; }
.ai-disc-card__id { font-size: 13px; font-weight: 600; color: var(--ai-text, var(--text-primary)); font-family: monospace; }
.ai-disc-card__status { display: inline-flex; align-items: center; gap: 4px; font-size: 11px; }
.ai-disc-card__status .dot { width: 6px; height: 6px; border-radius: 50%; }
.ai-disc-card__status.ok { color: #10b981; }
.ai-disc-card__status.ok .dot { background: #10b981; }
.ai-disc-card__status.off { color: #94a3b8; }
.ai-disc-card__status.off .dot { background: #94a3b8; }
.ai-disc-card__meta { display: flex; gap: 8px; margin-top: 8px; }
.meta-item { font-size: 11px; color: var(--ai-text-muted, var(--text-secondary)); padding: 2px 6px; border-radius: 4px; background: var(--ai-glass-bg-subtle, rgba(0,0,0,0.04)); }
.ai-disc-card__stats { display: flex; gap: 12px; margin-top: 6px; }
.stat { font-size: 11px; color: var(--ai-text-faint, var(--text-tertiary)); }
.ai-disc-card__caps { margin-top: 8px; padding-top: 8px; border-top: 1px solid var(--ai-glass-border-subtle, var(--border)); display: flex; flex-direction: column; gap: 4px; }
.cap-item { display: flex; justify-content: space-between; align-items: center; font-size: 11px; }
.cap-id { color: var(--ai-text-muted, var(--text-secondary)); font-family: monospace; }
.cap-cat { font-size: 10px; padding: 1px 5px; border-radius: 3px; }
.cat-device { color: #3b82f6; background: rgba(59,130,246,0.1); }
.cat-ai { color: #a78bfa; background: rgba(167,139,250,0.1); }
.cat-system { color: #64748b; background: rgba(100,116,139,0.1); }
.ai-disc-empty { text-align: center; padding: 40px 20px; color: var(--ai-text-muted, var(--text-secondary)); }
.ai-disc-empty .hint { font-size: 12px; color: var(--ai-text-faint, var(--text-tertiary)); margin: 4px 0 12px; }
</style>
