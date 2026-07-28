<script setup>
import { ref } from 'vue'
import AiEanAgentStatus from './AiEanAgentStatus.vue'
import AiCapabilityList from './AiCapabilityList.vue'
import AiInvokeConsole from './AiInvokeConsole.vue'
import AiEventMonitor from './AiEventMonitor.vue'
import AiDiscoveryView from './AiDiscoveryView.vue'
import AiEanDebugGuide from './AiEanDebugGuide.vue'

// 子组件各自在 onMounted 中拉取数据，panel 仅管理 Tab 状态
const subTab = ref('status')
const presetCapability = ref('')

const SUB_TABS = [
  { id: 'status', label: 'Agent' },
  { id: 'capability', label: 'Capability' },
  { id: 'invoke', label: 'Invoke' },
  { id: 'event', label: 'Event' },
  { id: 'discovery', label: 'Discovery' },
  { id: 'debug', label: '调试指南' }
]

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
        {{ tab.label }}
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
  gap: 12px;
}
.ai-ean-subtabs {
  display: flex;
  gap: 4px;
  padding: 0 4px;
  border-bottom: 1px solid var(--ai-glass-border-subtle, var(--border));
}
.ai-ean-subtab {
  padding: 6px 14px;
  border: none;
  background: transparent;
  color: var(--ai-text-muted, var(--text-secondary));
  font-size: 13px;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: all 160ms cubic-bezier(0.16, 1, 0.3, 1);
}
.ai-ean-subtab:hover {
  color: var(--primary);
}
.ai-ean-subtab--active {
  color: var(--primary);
  border-bottom-color: var(--primary);
  font-weight: 500;
}
.ai-ean-content {
  flex: 1;
  overflow-y: auto;
  padding: 4px;
}
</style>
