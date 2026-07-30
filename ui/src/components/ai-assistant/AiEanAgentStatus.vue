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
      <!-- 头部：标题 + 状态徽章 -->
      <div class="ai-ean-status__header">
        <div class="ai-ean-status__title-group">
          <span class="ai-ean-status__icon" :class="isOnline ? 'ok' : 'err'">
            <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
              <path d="M1.5 8h2l1.5-5 3 12 2-7 1.5 3h3" />
            </svg>
          </span>
          <span class="ai-ean-status__title">Agent 状态</span>
        </div>
        <span class="ai-ean-status__badge" :class="isOnline ? 'ok' : 'err'">
          <span class="dot"></span>{{ statusText }}
        </span>
      </div>

      <!-- 信息网格 -->
      <div class="ai-ean-status__grid">
        <div class="ai-ean-field">
          <span class="label">Agent ID</span>
          <span class="value mono">{{ agentStatus.id || '-' }}</span>
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
        <div class="ai-ean-field featured">
          <span class="label">Capability 数</span>
          <span class="value highlight">{{ capabilityCount }}</span>
        </div>
      </div>

      <!-- 分类统计 -->
      <div class="ai-ean-status__cats">
        <span class="cat cat-device">
          <span class="cat-dot"></span>device: {{ capabilityByCategory.device }}
        </span>
        <span class="cat cat-ai">
          <span class="cat-dot"></span>ai: {{ capabilityByCategory.ai }}
        </span>
        <span class="cat cat-system">
          <span class="cat-dot"></span>system: {{ capabilityByCategory.system }}
        </span>
      </div>
    </div>

    <!-- 空状态 -->
    <div class="ai-ean-empty" v-else>
      <div class="ai-ean-empty__icon">
        <svg viewBox="0 0 48 48" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="24" cy="24" r="20" stroke-dasharray="4 3" opacity="0.4" />
          <path d="M24 14v12M24 32v2" />
        </svg>
      </div>
      <p class="ai-ean-empty__text">EAN Capability Runtime 未连接</p>
      <p class="ai-ean-empty__hint">请检查北向 edgeOS 通道配置或 MCP 服务状态</p>
      <a-button size="small" type="outline" @click="fetchAgentStatus">重试</a-button>
    </div>
  </div>
</template>

<style scoped>
.ai-ean-status {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

/* ── 玻璃态卡片 ── */
.ai-ean-card {
  background: var(--ai-glass-bg-card, var(--bg));
  border: 1px solid var(--ai-glass-border, var(--border));
  border-radius: 14px;
  padding: 16px 20px;
  backdrop-filter: blur(20px);
  box-shadow:
    0 1px 3px rgba(0, 0, 0, 0.04),
    inset 0 1px 0 var(--ai-glass-highlight, transparent);
  transition: border-color 200ms cubic-bezier(0.16, 1, 0.3, 1);
}
.ai-ean-card:hover {
  border-color: color-mix(in srgb, var(--primary) 25%, var(--ai-glass-border, var(--border)));
}

/* ── 头部 ── */
.ai-ean-status__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}
.ai-ean-status__title-group {
  display: flex;
  align-items: center;
  gap: 8px;
}
.ai-ean-status__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 6px;
  flex-shrink: 0;
}
.ai-ean-status__icon svg {
  width: 14px;
  height: 14px;
}
.ai-ean-status__icon.ok {
  color: #10b981;
  background: rgba(16, 185, 129, 0.1);
}
.ai-ean-status__icon.err {
  color: #ef4444;
  background: rgba(239, 68, 68, 0.1);
}
.ai-ean-status__title {
  font-size: 14px;
  font-weight: 600;
  color: var(--ai-text, var(--text-primary));
  letter-spacing: 0.2px;
}
.ai-ean-status__badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 10px;
  border-radius: 9999px;
  font-size: 12px;
  font-weight: 500;
}
.ai-ean-status__badge.ok {
  color: #10b981;
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.2);
}
.ai-ean-status__badge.err {
  color: #ef4444;
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.2);
}
.ai-ean-status__badge .dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}
.ai-ean-status__badge.ok .dot {
  animation: ean-pulse 2s ease-in-out infinite;
}
@keyframes ean-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

/* ── 信息网格 ── */
.ai-ean-status__grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px 24px;
  padding-bottom: 14px;
  border-bottom: 1px solid var(--ai-glass-border-subtle, var(--border));
}
.ai-ean-field {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.ai-ean-field.featured {
  /* Capability 数高亮字段 */
}
.ai-ean-field .label {
  font-size: 11px;
  color: var(--ai-text-faint, var(--text-tertiary));
  text-transform: uppercase;
  letter-spacing: 0.5px;
  font-weight: 500;
}
.ai-ean-field .value {
  font-size: 13.5px;
  color: var(--ai-text, var(--text-primary));
  font-weight: 500;
}
.ai-ean-field .value.mono {
  font-family: 'SF Mono', 'Cascadia Code', 'Consolas', monospace;
  font-size: 12.5px;
}
.ai-ean-field .value.highlight {
  color: var(--primary);
  font-size: 18px;
  font-weight: 700;
}

/* ── 分类统计 ── */
.ai-ean-status__cats {
  display: flex;
  gap: 8px;
  padding-top: 14px;
  flex-wrap: wrap;
}
.cat {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 11.5px;
  padding: 4px 10px;
  border-radius: 6px;
  font-weight: 500;
  border: 1px solid transparent;
  transition: all 160ms;
}
.cat-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}
.cat-device {
  color: #3b82f6;
  background: rgba(59, 130, 246, 0.08);
  border-color: rgba(59, 130, 246, 0.15);
}
.cat-ai {
  color: #a78bfa;
  background: rgba(167, 139, 250, 0.08);
  border-color: rgba(167, 139, 250, 0.15);
}
.cat-system {
  color: #64748b;
  background: rgba(100, 116, 139, 0.08);
  border-color: rgba(100, 116, 139, 0.15);
}

/* ── 空状态 ── */
.ai-ean-empty {
  text-align: center;
  padding: 48px 20px 40px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}
.ai-ean-empty__icon {
  color: var(--ai-text-faint, var(--text-tertiary));
  margin-bottom: 8px;
  opacity: 0.5;
}
.ai-ean-empty__icon svg {
  width: 48px;
  height: 48px;
}
.ai-ean-empty__text {
  font-size: 14px;
  color: var(--ai-text, var(--text-primary));
  margin: 0 0 4px;
  font-weight: 500;
}
.ai-ean-empty__hint {
  font-size: 12px;
  color: var(--ai-text-muted, var(--text-secondary));
  margin: 0 0 16px;
  max-width: 260px;
}

/* ── 暗色主题 ── */
body.dark-theme .ai-ean-card {
  background: rgba(13, 18, 34, 0.6);
  border-color: rgba(255, 255, 255, 0.08);
}
body.dark-theme .ai-ean-status__icon.ok {
  background: rgba(16, 185, 129, 0.15);
}
body.dark-theme .ai-ean-status__icon.err {
  background: rgba(239, 68, 68, 0.15);
}
</style>
