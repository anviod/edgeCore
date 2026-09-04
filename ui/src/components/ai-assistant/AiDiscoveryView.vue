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
      <a-button size="mini" type="text" @click="fetchDiscoveredAgents()">
        <svg viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="width:10px;height:10px;margin-right:3px"><path d="M2 6a4 4 0 0 1 7-2.5M10 6a4 4 0 0 1-7 2.5" /><path d="M8.5 1.5v2h-2M3.5 10.5v-2h2" /></svg>
        刷新
      </a-button>
    </div>

    <div class="ai-disc-grid" v-if="discoveredAgents.length">
      <div
        v-for="agent in discoveredAgents"
        :key="agent.id"
        class="ai-disc-card"
        :class="{ offline: agent.status !== 'online' }"
      >
        <div class="ai-disc-card__header" @click="toggleAgent(agent.id)">
          <div class="ai-disc-card__left">
            <span class="ai-disc-card__status-dot" :class="agent.status === 'online' ? 'ok' : 'off'"></span>
            <span class="ai-disc-card__id">{{ agent.id }}</span>
          </div>
          <span class="ai-disc-card__status" :class="agent.status === 'online' ? 'ok' : 'off'">
            {{ agent.status === 'online' ? 'online' : 'offline' }}
          </span>
        </div>
        <div class="ai-disc-card__meta">
          <span class="meta-item">{{ agent.kind || 'device' }}</span>
          <span class="meta-item">{{ agent.version || '-' }}</span>
          <span class="meta-item">{{ agent.transport || 'mqtt' }}</span>
        </div>
        <div class="ai-disc-card__stats">
          <div class="stat">
            <span class="stat-label">Cap</span>
            <span class="stat-value">{{ agent.capabilities_count || 0 }}</span>
          </div>
          <div class="stat">
            <span class="stat-label">Last</span>
            <span class="stat-value">{{ formatLastSeen(agent.last_seen_seconds_ago) }}</span>
          </div>
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
      <div class="ai-disc-empty__icon">
        <svg viewBox="0 0 32 32" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="16" cy="6" r="3" opacity="0.4" />
          <circle cx="6" cy="24" r="3" opacity="0.4" />
          <circle cx="26" cy="24" r="3" opacity="0.4" />
          <path d="M16 9v4M16 13L8 21M16 13l8 8" opacity="0.3" />
        </svg>
      </div>
      <p>未发现其他 Agent</p>
      <p class="hint">确保 EdgeOS 已连接且 Discovery 已启用</p>
      <a-button size="small" type="outline" @click="fetchDiscoveredAgents()">刷新</a-button>
    </div>
  </div>
</template>

<style scoped>
.ai-disc {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.ai-disc-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.ai-disc-header .title {
  font-size: 13px;
  font-weight: 600;
  color: var(--ai-text, var(--text-primary));
  letter-spacing: 0.2px;
}

/* ── 卡片网格 ── */
.ai-disc-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}
.ai-disc-card {
  border: 1px solid var(--ai-glass-border, var(--border));
  border-radius: 12px;
  padding: 12px 14px;
  background: var(--ai-glass-bg-card, var(--bg));
  backdrop-filter: blur(20px);
  transition: all 200ms cubic-bezier(0.16, 1, 0.3, 1);
  box-shadow:
    0 1px 3px rgba(0, 0, 0, 0.03),
    inset 0 1px 0 var(--ai-glass-highlight, transparent);
}
.ai-disc-card:hover {
  border-color: color-mix(in srgb, var(--primary) 30%, var(--border));
  box-shadow:
    0 4px 12px rgba(0, 0, 0, 0.06),
    inset 0 1px 0 var(--ai-glass-highlight, transparent);
  transform: translateY(-1px);
}
.ai-disc-card.offline {
  opacity: 0.55;
}
.ai-disc-card.offline:hover {
  opacity: 0.75;
}

.ai-disc-card__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  cursor: pointer;
}
.ai-disc-card__left {
  display: flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
}
.ai-disc-card__status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.ai-disc-card__status-dot.ok {
  background: #10b981;
  box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.15);
}
.ai-disc-card__status-dot.off {
  background: #94a3b8;
}
.ai-disc-card__id {
  font-size: 13px;
  font-weight: 600;
  color: var(--ai-text, var(--text-primary));
  font-family: 'SF Mono', 'Cascadia Code', 'Consolas', monospace;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ai-disc-card__status {
  font-size: 10.5px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.3px;
  flex-shrink: 0;
}
.ai-disc-card__status.ok {
  color: #10b981;
}
.ai-disc-card__status.off {
  color: #94a3b8;
}

.ai-disc-card__meta {
  display: flex;
  gap: 6px;
  margin-top: 8px;
  flex-wrap: wrap;
}
.meta-item {
  font-size: 10.5px;
  color: var(--ai-text-muted, var(--text-secondary));
  padding: 2px 7px;
  border-radius: 4px;
  background: var(--ai-glass-bg-subtle, rgba(0, 0, 0, 0.04));
  font-weight: 500;
}

.ai-disc-card__stats {
  display: flex;
  gap: 16px;
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--ai-glass-border-subtle, var(--border));
}
.stat {
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.stat-label {
  font-size: 10px;
  color: var(--ai-text-faint, var(--text-tertiary));
  text-transform: uppercase;
  letter-spacing: 0.3px;
  font-weight: 600;
}
.stat-value {
  font-size: 12.5px;
  color: var(--ai-text, var(--text-primary));
  font-weight: 600;
}

.ai-disc-card__caps {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--ai-glass-border-subtle, var(--border));
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.cap-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 11px;
}
.cap-id {
  color: var(--ai-text-muted, var(--text-secondary));
  font-family: 'SF Mono', 'Cascadia Code', 'Consolas', monospace;
}
.cap-cat {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 3px;
  font-weight: 600;
}
.cat-device { color: #3b82f6; background: rgba(59, 130, 246, 0.1); }
.cat-ai { color: #a78bfa; background: rgba(167, 139, 250, 0.1); }
.cat-system { color: #64748b; background: rgba(100, 116, 139, 0.1); }

/* ── 空状态 ── */
.ai-disc-empty {
  text-align: center;
  padding: 40px 20px;
  color: var(--ai-text-muted, var(--text-secondary));
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}
.ai-disc-empty__icon {
  color: var(--ai-text-faint, var(--text-tertiary));
  opacity: 0.4;
  margin-bottom: 8px;
}
.ai-disc-empty__icon svg {
  width: 40px;
  height: 40px;
}
.ai-disc-empty .hint {
  font-size: 12px;
  color: var(--ai-text-faint, var(--text-tertiary));
  margin: 4px 0 12px;
}

/* ── 暗色主题 ── */
body.dark-theme .ai-disc-card {
  background: rgba(13, 18, 34, 0.6);
  border-color: rgba(255, 255, 255, 0.08);
}
</style>
