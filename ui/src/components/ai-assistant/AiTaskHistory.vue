<template>
  <div class="ai-task-history">
    <header class="ai-task-history__header">
      <span class="ai-task-history__label">
        <icon-history :size="12" class="ai-task-history__label-icon" />
        最近任务
      </span>
      <button
        v-if="tasks.length"
        type="button"
        class="ai-task-history__refresh"
        title="刷新任务列表"
        :disabled="loading"
        @click="$emit('refresh')"
      >
        <icon-refresh :size="14" :spin="loading" />
      </button>
    </header>

    <div v-if="loading && !tasks.length" class="ai-task-history__skeleton">
      <div v-for="i in 3" :key="i" class="ai-skeleton ai-skeleton--row"></div>
    </div>

    <AiEmptyState
      v-else-if="!tasks.length"
      :title="emptyTitle"
      :description="emptyDescription"
    >
      <template #icon>
        <icon-unordered-list :size="22" />
      </template>
    </AiEmptyState>

    <ul v-else class="ai-task-history__list" role="list">
      <li
        v-for="t in tasks.slice(0, 8)"
        :key="t.id"
        class="ai-task-history__item"
        :class="{ 'ai-task-history__item--active': activeId === t.id }"
      >
        <button
          type="button"
          class="ai-task-history__btn"
          :aria-current="activeId === t.id ? 'true' : undefined"
          @click="$emit('select', t.id)"
        >
          <span class="ai-task-history__id">{{ shortId(t.id) }}</span>
          <span class="ai-task-history__meta">
            {{ skillLabel(t.skill) }}
            <span v-if="t.input_files?.[0]" class="ai-task-history__file">{{ t.input_files[0] }}</span>
          </span>
          <AiStatusBadge :status="t.status" />
        </button>
      </li>
    </ul>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { IconHistory, IconRefresh, IconUnorderedList } from '@arco-design/web-vue/es/icon'
import AiEmptyState from './AiEmptyState.vue'
import AiStatusBadge from './AiStatusBadge.vue'

const props = defineProps({
  tasks: { type: Array, default: () => [] },
  activeId: { type: String, default: '' },
  loading: { type: Boolean, default: false },
  workspace: { type: String, default: '' }
})

defineEmits(['select', 'refresh'])

const emptyMeta = computed(() => {
  const map = {
    protocol: {
      title: '暂无协议分析任务',
      description: '上传 PCAP、协议文档或监控表开始协议逆向'
    },
    validation: {
      title: '暂无校验任务',
      description: '上传 JSON/Excel 配置或在对话区描述待校验内容'
    },
    cases: {
      title: '暂无验证用例',
      description: '完成协议分析后可沉淀为可回放用例'
    },
    edge: {
      title: '暂无边缘场景任务',
      description: '描述场景需求，AI 将生成 EdgeRule 草稿包'
    },
    diagnostics: {
      title: '暂无诊断任务',
      description: '描述问题或上传日志、诊断快照开始联调诊断'
    }
  }
  return map[props.workspace] || {
    title: '暂无任务',
    description: '在对话区描述需求或上传文件开始任务'
  }
})

const emptyTitle = computed(() => emptyMeta.value.title)
const emptyDescription = computed(() => emptyMeta.value.description)

const shortId = (id) => id?.slice(-10) || '—'

const skillLabel = (skill) => {
  const map = {
    'protocol-reverse': '逆向',
    'doc-parse': '文档',
    'config-gen': '配置',
    'edge-rule-draft': '边缘',
    diagnostics: '诊断'
  }
  return map[skill] || skill || '任务'
}
</script>
