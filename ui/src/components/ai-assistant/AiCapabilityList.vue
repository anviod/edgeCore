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
    <!-- 工具栏 -->
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
      <!-- Capability 列表 -->
      <div class="ai-cap-table" v-if="filtered.length">
        <div
          class="ai-cap-row"
          v-for="cap in filtered"
          :key="cap.id"
          :class="{ 'ai-cap-row--expanded': expandedId === cap.id }"
        >
          <div class="ai-cap-row__header" @click="toggleExpand(cap.id)">
            <div class="ai-cap-row__left">
              <span class="ai-cap-row__chevron" :class="{ 'ai-cap-row__chevron--open': expandedId === cap.id }">
                <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M6 4l4 4-4 4" />
                </svg>
              </span>
              <span class="ai-cap-row__id">{{ cap.id }}</span>
            </div>
            <div class="ai-cap-row__meta">
              <span class="tag" :class="`tag-${cap.category}`">{{ cap.category }}</span>
              <span class="tag tag-perm">{{ permissionLabel(cap.permission) }}</span>
            </div>
          </div>

          <!-- 展开详情 -->
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
              <span class="ai-cap-timeout">
                <svg viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                  <circle cx="6" cy="6" r="5" /><path d="M6 3v3l2 1.5" />
                </svg>
                {{ cap.timeout_sec || 10 }}s
              </span>
              <a-button size="mini" type="primary" @click.stop="emit('test-invoke', cap)">
                测试调用
              </a-button>
            </div>
          </div>
        </div>
      </div>

      <!-- 空状态 -->
      <div class="ai-cap-empty" v-else-if="!loading">
        <div class="ai-cap-empty__icon">
          <svg viewBox="0 0 32 32" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <rect x="4" y="4" width="10" height="10" rx="2" opacity="0.4" />
            <rect x="18" y="4" width="10" height="10" rx="2" opacity="0.4" />
            <rect x="4" y="18" width="10" height="10" rx="2" opacity="0.4" />
            <rect x="18" y="18" width="10" height="10" rx="2" opacity="0.4" />
          </svg>
        </div>
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
.ai-cap-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  height: 100%;
}

/* ── 工具栏 ── */
.ai-cap-toolbar {
  display: flex;
  gap: 8px;
}

/* ── 列表 ── */
.ai-cap-table {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.ai-cap-row {
  border: 1px solid var(--ai-glass-border-subtle, var(--border));
  border-radius: 10px;
  overflow: hidden;
  transition: all 200ms cubic-bezier(0.16, 1, 0.3, 1);
}
.ai-cap-row:hover {
  border-color: color-mix(in srgb, var(--primary) 30%, var(--border));
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}
.ai-cap-row--expanded {
  border-color: color-mix(in srgb, var(--primary) 35%, var(--border));
  box-shadow: 0 2px 12px color-mix(in srgb, var(--primary) 8%, transparent);
}

.ai-cap-row__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 12px;
  cursor: pointer;
  user-select: none;
}
.ai-cap-row__left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.ai-cap-row__chevron {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  flex-shrink: 0;
  color: var(--ai-text-faint, var(--text-tertiary));
  transition: transform 200ms cubic-bezier(0.16, 1, 0.3, 1);
}
.ai-cap-row__chevron svg {
  width: 12px;
  height: 12px;
}
.ai-cap-row__chevron--open {
  transform: rotate(90deg);
  color: var(--primary);
}
.ai-cap-row__id {
  font-size: 12.5px;
  font-weight: 500;
  color: var(--ai-text, var(--text-primary));
  font-family: 'SF Mono', 'Cascadia Code', 'Consolas', monospace;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ai-cap-row__meta {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

/* ── 标签 ── */
.tag {
  font-size: 10.5px;
  padding: 2px 8px;
  border-radius: 4px;
  font-weight: 600;
  letter-spacing: 0.2px;
  border: 1px solid transparent;
}
.tag-device {
  color: #3b82f6;
  background: rgba(59, 130, 246, 0.08);
  border-color: rgba(59, 130, 246, 0.15);
}
.tag-ai {
  color: #a78bfa;
  background: rgba(167, 139, 250, 0.08);
  border-color: rgba(167, 139, 250, 0.15);
}
.tag-system {
  color: #64748b;
  background: rgba(100, 116, 139, 0.08);
  border-color: rgba(100, 116, 139, 0.15);
}
.tag-workflow {
  color: #f59e0b;
  background: rgba(245, 158, 11, 0.08);
  border-color: rgba(245, 158, 11, 0.15);
}
.tag-perm {
  color: #f59e0b;
  background: rgba(245, 158, 11, 0.08);
  border-color: rgba(245, 158, 11, 0.12);
}

/* ── 详情 ── */
.ai-cap-row__detail {
  padding: 8px 12px 12px;
  border-top: 1px solid var(--ai-glass-border-subtle, var(--border));
  background: var(--ai-glass-bg-subtle, rgba(0, 0, 0, 0.02));
}
.ai-cap-desc {
  font-size: 12px;
  color: var(--ai-text-muted, var(--text-secondary));
  margin: 0 0 8px;
  line-height: 1.5;
}
.ai-cap-schemas {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 8px;
}
.ai-cap-schema {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.schema-label {
  font-size: 10.5px;
  color: var(--ai-text-faint, var(--text-tertiary));
  text-transform: uppercase;
  letter-spacing: 0.5px;
  font-weight: 600;
}
.ai-cap-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-top: 4px;
}
.ai-cap-timeout {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: var(--ai-text-faint, var(--text-tertiary));
}
.ai-cap-timeout svg {
  width: 11px;
  height: 11px;
}

/* ── 空状态 ── */
.ai-cap-empty {
  text-align: center;
  padding: 40px 20px;
  color: var(--ai-text-muted, var(--text-secondary));
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}
.ai-cap-empty__icon {
  color: var(--ai-text-faint, var(--text-tertiary));
  opacity: 0.4;
  margin-bottom: 4px;
}
.ai-cap-empty__icon svg {
  width: 32px;
  height: 32px;
}

/* ── 页脚 ── */
.ai-cap-footer {
  text-align: right;
  font-size: 11px;
  color: var(--ai-text-faint, var(--text-tertiary));
  padding: 4px 0;
}

/* ── 暗色主题 ── */
body.dark-theme .ai-cap-row__detail {
  background: rgba(13, 18, 34, 0.4);
}
</style>
