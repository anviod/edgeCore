<script setup>
import { ref, computed } from 'vue'
import AiEanAgentStatus from './AiEanAgentStatus.vue'
import AiCapabilityList from './AiCapabilityList.vue'
import AiInvokeConsole from './AiInvokeConsole.vue'
import AiEventMonitor from './AiEventMonitor.vue'
import AiDiscoveryView from './AiDiscoveryView.vue'
import AiEanDebugGuide from './AiEanDebugGuide.vue'
import { useEan } from '@/composables/useEan'

// 子组件各自在 onMounted 中拉取数据，panel 仅管理 Tab 状态
const subTab = ref('status')
const presetCapability = ref('')

const { capabilityCount, eventCount, isOnline, discoveredAgents } = useEan()

const SUB_TABS = computed(() => [
  { id: 'status', label: 'Agent', icon: 'pulse', badge: isOnline.value ? 'on' : 'off' },
  { id: 'capability', label: 'Capability', icon: 'grid', badge: capabilityCount.value || null },
  { id: 'invoke', label: 'Invoke', icon: 'play' },
  { id: 'event', label: 'Event', icon: 'bell', badge: eventCount.value || null },
  { id: 'discovery', label: 'Discovery', icon: 'network', badge: discoveredAgents.value.length || null },
  { id: 'debug', label: '调试指南', icon: 'book' }
])

// Capability 列表点击「测试调用」时切换到 Invoke Tab 并预选能力
const handleTestInvoke = (cap) => {
  presetCapability.value = cap.id
  subTab.value = 'invoke'
}
</script>

<template>
  <div class="ai-ean-panel">
    <nav class="ai-ean-subtabs">
      <button
        v-for="tab in SUB_TABS"
        :key="tab.id"
        class="ai-ean-subtab"
        :class="{ 'ai-ean-subtab--active': subTab === tab.id }"
        @click="subTab = tab.id"
      >
        <span class="ai-ean-subtab__icon" v-html="
          tab.icon === 'pulse' ? '<svg viewBox=\'0 0 16 16\' fill=\'none\' stroke=\'currentColor\' stroke-width=\'1.5\' stroke-linecap=\'round\' stroke-linejoin=\'round\'><path d=\'M1.5 8h2l1.5-5 3 12 2-7 1.5 3h3\'/></svg>' :
          tab.icon === 'grid' ? '<svg viewBox=\'0 0 16 16\' fill=\'none\' stroke=\'currentColor\' stroke-width=\'1.5\'><rect x=\'2\' y=\'2\' width=\'5\' height=\'5\' rx=\'1\'/><rect x=\'9\' y=\'2\' width=\'5\' height=\'5\' rx=\'1\'/><rect x=\'2\' y=\'9\' width=\'5\' height=\'5\' rx=\'1\'/><rect x=\'9\' y=\'9\' width=\'5\' height=\'5\' rx=\'1\'/></svg>' :
          tab.icon === 'play' ? '<svg viewBox=\'0 0 16 16\' fill=\'none\' stroke=\'currentColor\' stroke-width=\'1.5\' stroke-linejoin=\'round\'><path d=\'M3 2.5v11l9.5-5.5z\'/></svg>' :
          tab.icon === 'bell' ? '<svg viewBox=\'0 0 16 16\' fill=\'none\' stroke=\'currentColor\' stroke-width=\'1.5\' stroke-linecap=\'round\' stroke-linejoin=\'round\'><path d=\'M3 6.5a5 5 0 0 1 10 0v3l1.5 2H1.5l1.5-2z\'/><path d=\'M6 13.5a2 2 0 0 0 4 0\'/></svg>' :
          tab.icon === 'network' ? '<svg viewBox=\'0 0 16 16\' fill=\'none\' stroke=\'currentColor\' stroke-width=\'1.5\' stroke-linecap=\'round\' stroke-linejoin=\'round\'><circle cx=\'8\' cy=\'3\' r=\'2\'/><circle cx=\'3\' cy=\'13\' r=\'2\'/><circle cx=\'13\' cy=\'13\' r=\'2\'/><path d=\'M8 5v3M8 8L4.5 11M8 8l3.5 3\'/></svg>' :
          '<svg viewBox=\'0 0 16 16\' fill=\'none\' stroke=\'currentColor\' stroke-width=\'1.5\' stroke-linecap=\'round\' stroke-linejoin=\'round\'><path d=\'M3 2.5h7l3 3v8a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V3.5a1 1 0 0 1 1-1z\'/><path d=\'M9 2.5V5.5h3.5M5 8.5h6M5 11h4\'/></svg>'
        "></span>
        <span class="ai-ean-subtab__label">{{ tab.label }}</span>
        <span
          v-if="tab.badge"
          class="ai-ean-subtab__badge"
          :class="{
            'ai-ean-subtab__badge--on': tab.badge === 'on',
            'ai-ean-subtab__badge--off': tab.badge === 'off'
          }"
        >{{ tab.badge === 'on' ? '在线' : tab.badge === 'off' ? '离线' : tab.badge }}</span>
      </button>
    </nav>
    <div class="ai-ean-content">
      <AiEanAgentStatus v-if="subTab === 'status'" />
      <AiCapabilityList v-else-if="subTab === 'capability'" @test-invoke="handleTestInvoke" />
      <AiInvokeConsole v-else-if="subTab === 'invoke'" :preset-capability="presetCapability" />
      <AiEventMonitor v-else-if="subTab === 'event'" />
      <AiDiscoveryView v-else-if="subTab === 'discovery'" />
      <AiEanDebugGuide v-else-if="subTab === 'debug'" />
    </div>
  </div>
</template>

<style scoped>
.ai-ean-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  gap: 10px;
}

/* ── 子标签导航 ── */
.ai-ean-subtabs {
  display: flex;
  gap: 2px;
  padding: 2px;
  border-bottom: 1px solid var(--ai-glass-border-subtle, var(--border));
  overflow-x: auto;
  scrollbar-width: none;
}
.ai-ean-subtabs::-webkit-scrollbar { display: none; }

.ai-ean-subtab {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 7px 12px;
  border: none;
  background: transparent;
  color: var(--ai-text-muted, var(--text-secondary));
  font-size: 12.5px;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: all 200ms cubic-bezier(0.16, 1, 0.3, 1);
  white-space: nowrap;
  position: relative;
}

.ai-ean-subtab:hover {
  color: var(--primary);
  background: color-mix(in srgb, var(--primary) 4%, transparent);
  border-radius: 6px 6px 0 0;
}

.ai-ean-subtab--active {
  color: var(--primary);
  border-bottom-color: var(--primary);
  font-weight: 600;
}

.ai-ean-subtab__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  flex-shrink: 0;
  opacity: 0.7;
  transition: opacity 200ms;
}
.ai-ean-subtab__icon svg {
  width: 100%;
  height: 100%;
}
.ai-ean-subtab--active .ai-ean-subtab__icon {
  opacity: 1;
}

.ai-ean-subtab__badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 16px;
  padding: 0 5px;
  border-radius: 9999px;
  font-size: 10px;
  font-weight: 600;
  line-height: 1;
  color: var(--ai-text-faint, var(--text-tertiary));
  background: var(--ai-glass-bg-subtle, rgba(0, 0, 0, 0.04));
  transition: all 200ms;
}
.ai-ean-subtab--active .ai-ean-subtab__badge {
  background: color-mix(in srgb, var(--primary) 12%, transparent);
  color: var(--primary);
}
.ai-ean-subtab__badge--on {
  background: rgba(16, 185, 129, 0.12) !important;
  color: #10b981 !important;
}
.ai-ean-subtab__badge--off {
  background: rgba(239, 68, 68, 0.12) !important;
  color: #ef4444 !important;
}

.ai-ean-content {
  flex: 1;
  overflow-y: auto;
  padding: 2px 4px 4px;
}

/* ── 暗色主题适配 ── */
body.dark-theme .ai-ean-subtab {
  color: var(--ai-text-muted);
}
body.dark-theme .ai-ean-subtab:hover {
  background: color-mix(in srgb, var(--primary) 8%, transparent);
}
</style>
