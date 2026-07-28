<script setup>
import { ref, computed, onMounted } from 'vue'
import { useEan } from '@/composables/useEan'
import AiJsonPreview from './AiJsonPreview.vue'

const { capabilities, loading, fetchCapabilities } = useEan()

const searchText = ref('')
const categoryFilter = ref('')
const expandedId = ref(null)

const filtered = computed(() => {
  return capabilities.value.filter((c) => {
    const matchText = !searchText.value ||
      (c.id || '').toLowerCase().includes(searchText.value.toLowerCase()) ||
      (c.description || '').toLowerCase().includes(searchText.value.toLowerCase())
    const matchCat = !categoryFilter.value || c.category === categoryFilter.value
    return matchText && matchCat
  })
})

const categoryOptions = [
  { label: '全部分类', value: '' },
  { label: 'device', value: 'device' },
  { label: 'ai', value: 'ai' },
  { label: 'system', value: 'system' },
  { label: 'workflow', value: 'workflow' }
]

const permissionLabel = (p) => {
  const map = { read: '读', write: '写', readwrite: '读写', admin: '管理' }
  return map[p] || p
}

const toggleExpand = (id) => {
  expandedId.value = expandedId.value === id ? null : id
}

const emit = defineEmits(['test-invoke'])

onMounted(() => {
  fetchCapabilities()
})
</script>

<template>
  <div class="ai-cap-list">
    <div class="ai-cap-toolbar">
      <a-input
        v-model="searchText"
        placeholder="搜索 Capability..."
        size="small"
        allow-clear
        style="flex: 1"
      />
      <a-select
        v-model="categoryFilter"
        size="small"
        style="width: 120px"
        :options="categoryOptions"
      />
    </div>

    <a-spin :loading="loading" style="width: 100%">
      <div class="ai-cap-table" v-if="filtered.length">
        <div class="ai-cap-row" v-for="cap in filtered" :key="cap.id">
          <div class="ai-cap-row__header" @click="toggleExpand(cap.id)">
            <span class="ai-cap-row__id">{{ cap.id }}</span>
            <div class="ai-cap-row__meta">
              <span class="tag" :class="`tag-${cap.category}`">{{ cap.category }}</span>
              <span class="tag tag-perm">{{ permissionLabel(cap.permission) }}</span>
              <span class="ai-cap-row__expand">{{ expandedId === cap.id ? '▾' : '▸' }}</span>
            </div>
          </div>
          <div class="ai-cap-row__detail" v-if="expandedId === cap.id">
            <p class="ai-cap-desc">{{ cap.description || '无描述' }}</p>
            <div class="ai-cap-schemas">
              <div class="ai-cap-schema">
                <span class="schema-label">Input Schema</span>
                <AiJsonPreview :data="cap.input_schema" :compact="true" :copyable="false" />
              </div>
              <div class="ai-cap-schema">
                <span class="schema-label">Output Schema</span>
                <AiJsonPreview :data="cap.output_schema" :compact="true" :copyable="false" />
              </div>
            </div>
            <div class="ai-cap-actions">
              <span class="ai-cap-timeout">超时: {{ cap.timeout_sec || 10 }}s</span>
              <a-button size="mini" type="primary" @click.stop="emit('test-invoke', cap)">
                测试调用
              </a-button>
            </div>
          </div>
        </div>
      </div>
      <div class="ai-cap-empty" v-else-if="!loading">
        <p>暂无已注册的 Capability</p>
        <a-button size="small" type="outline" @click="fetchCapabilities()">刷新</a-button>
      </div>
    </a-spin>

    <div class="ai-cap-footer" v-if="filtered.length">
      共 {{ filtered.length }} 个 Capability
    </div>
  </div>
</template>

<style scoped>
.ai-cap-list { display: flex; flex-direction: column; gap: 10px; height: 100%; }
.ai-cap-toolbar { display: flex; gap: 8px; }
.ai-cap-table { display: flex; flex-direction: column; gap: 4px; }
.ai-cap-row {
  border: 1px solid var(--ai-glass-border-subtle, var(--border));
  border-radius: 10px;
  overflow: hidden;
  transition: border-color 160ms;
}
.ai-cap-row:hover { border-color: var(--primary); }
.ai-cap-row__header {
  display: flex; justify-content: space-between; align-items: center;
  padding: 8px 12px; cursor: pointer; user-select: none;
}
.ai-cap-row__id { font-size: 13px; font-weight: 500; color: var(--ai-text, var(--text-primary)); font-family: monospace; }
.ai-cap-row__meta { display: flex; align-items: center; gap: 6px; }
.tag { font-size: 11px; padding: 2px 8px; border-radius: 4px; font-weight: 500; }
.tag-device { color: #3b82f6; background: rgba(59,130,246,0.1); }
.tag-ai { color: #a78bfa; background: rgba(167,139,250,0.1); }
.tag-system { color: #64748b; background: rgba(100,116,139,0.1); }
.tag-perm { color: #f59e0b; background: rgba(245,158,11,0.1); }
.ai-cap-row__expand { color: var(--ai-text-faint, var(--text-tertiary)); font-size: 12px; }
.ai-cap-row__detail { padding: 8px 12px 12px; border-top: 1px solid var(--ai-glass-border-subtle, var(--border)); }
.ai-cap-desc { font-size: 12px; color: var(--ai-text-muted, var(--text-secondary)); margin: 0 0 8px; }
.ai-cap-schemas { display: flex; flex-direction: column; gap: 8px; margin-bottom: 8px; }
.ai-cap-schema { display: flex; flex-direction: column; gap: 4px; }
.schema-label { font-size: 11px; color: var(--ai-text-faint, var(--text-tertiary)); text-transform: uppercase; }
.ai-cap-actions { display: flex; justify-content: space-between; align-items: center; }
.ai-cap-timeout { font-size: 11px; color: var(--ai-text-faint, var(--text-tertiary)); }
.ai-cap-empty { text-align: center; padding: 32px 20px; color: var(--ai-text-muted, var(--text-secondary)); }
.ai-cap-footer { text-align: right; font-size: 11px; color: var(--ai-text-faint, var(--text-tertiary)); padding: 4px 0; }
</style>
