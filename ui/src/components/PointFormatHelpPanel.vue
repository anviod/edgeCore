<template>
  <div class="vd-card-box mb-4">
    <div class="vd-card-title-row" @click="toggle">
      <icon-question-circle class="vd-title-icon text-blue-600" />
      <span class="vd-section-title font-semibold">{{ t('panel.panelTitle') }}</span>
      <div class="vd-spacer"></div>
      <a-tag size="small" class="mr-2">{{ flatFormats.length }}</a-tag>
      <a-button size="mini" type="text">
        <template #icon>
          <icon-up v-if="expanded" />
          <icon-down v-else />
        </template>
      </a-button>
    </div>
    <Transition name="expand">
      <div v-show="expanded">
        <a-divider class="my-0" />
        <div class="p-3">
          <a-table
            :columns="columns"
            :data="tableData"
            :pagination="false"
            :bordered="{ wrapper: false, cell: false }"
            :stripe="false"
            size="small"
            class="help-table"
          >
            <template #format="{ record }">
              <div>
                <div class="font-medium">
                  <span class="text-xs text-gray-500 mr-1">{{ record.groupLabel }}</span>
                  <span v-html="sanitize(t(`formats.${record.name}.title`))"></span>
                </div>
                <div class="text-xs text-gray-500" v-html="sanitize(t(`formats.${record.name}.subtitle`))"></div>
              </div>
            </template>
            <template #range="{ record }">
              <div v-html="sanitize(t(`formats.${record.name}.range`))"></div>
              <div class="text-xs text-gray-500" v-html="sanitize(t(`formats.${record.name}.registers`))"></div>
            </template>
            <template #shortcut="{ record }">
              <span class="font-mono" v-html="sanitize(t(`formats.${record.name}.shortcut`))"></span>
            </template>
            <template #example="{ record }">
              <div v-html="sanitize(t(`formats.${record.name}.example`))"></div>
            </template>
          </a-table>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup>
import { computed, ref, watch, h } from 'vue'
import { useI18n } from 'vue-i18n'
import helpDefs from '@/i18n/pointFormatHelp.json'
import { sanitizeHtml } from '@/utils/sanitizeHtml'
import {
  IconQuestionCircle,
  IconUp,
  IconDown
} from '@arco-design/web-vue/es/icon'

const props = defineProps({
  lang: { type: String, default: 'zh' }
})

const buildMessages = (defs) => {
  const en = { panel: {}, formats: {} }
  const zh = { panel: {}, formats: {} }
  const metaFields = [
    'panelTitle', 'columnFormat', 'columnRange', 'columnShortcut', 'columnExample',
    'quickSwitchMenu', 'resetButton', 'resetTooltip', 'recentToggle', 'recentToggleTooltip'
  ]
  metaFields.forEach((field) => {
    const enVal = defs._meta?.en?.[field] || ''
    const zhVal = defs._meta?.zh?.[field]
    en.panel[field] = enVal
    if (!zhVal && enVal) {
      console.warn(`Missing zh translation for panel.${field}, fallback to en`)
      zh.panel[field] = enVal
    } else {
      zh.panel[field] = zhVal || ''
    }
  })
  const formatFields = ['title', 'subtitle', 'range', 'registers', 'shortcut', 'example']
  Object.keys(defs).forEach((key) => {
    if (key === '_meta') return
    const item = defs[key]
    en.formats[key] = {}
    zh.formats[key] = {}
    formatFields.forEach((field) => {
      const enVal = item.en?.[field] || ''
      const zhVal = item.zh?.[field]
      en.formats[key][field] = enVal
      if (!zhVal && enVal && field !== 'subtitle') {
        console.warn(`Missing zh translation for ${key}.${field}, fallback to en`)
        zh.formats[key][field] = enVal
      } else {
        zh.formats[key][field] = zhVal || ''
      }
    })
  })
  return { en, zh }
}

const messages = buildMessages(helpDefs)

const { t, locale } = useI18n({
  legacy: false,
  useScope: 'local',
  messages,
  inheritLocale: false,
  locale: props.lang || 'zh'
})

watch(() => props.lang, (val) => { locale.value = val || 'zh' })

const groupMeta = {
  one:   { bytes: 1, label: '1 字节' },
  two:   { bytes: 2, label: '2 字节 / 1 寄存器' },
  four:  { bytes: 4, label: '4 字节 / 2 寄存器' },
  eight: { bytes: 8, label: '8 字节 / 4 寄存器' }
}

const formatGroups = [
  { key: 'two',   names: ['Signed', 'Unsigned', 'Hex', 'Binary'] },
  { key: 'four',  names: ['Long AB CD', 'Long CD AB', 'Long BA DC', 'Long DC BA', 'Float AB CD', 'Float CD AB', 'Float BA DC', 'Float DC BA'] },
  { key: 'eight', names: ['Double AB CDEF GH', 'Double GH EFCD AB', 'Double BA DC FE HG', 'Double HG FE DC BA'] }
]

const flatFormats = computed(() => {
  const result = []
  formatGroups.forEach((g) => {
    const meta = groupMeta[g.key]
    const groupLabel = meta ? meta.label : ''
    g.names.forEach((name) => {
      if (helpDefs[name]) result.push({ name, groupLabel })
    })
  })
  return result
})

const expanded = ref(false)
const toggle = () => { expanded.value = !expanded.value }

const sanitize = (html) => sanitizeHtml(html)

const tableData = computed(() => flatFormats.value)

const columns = computed(() => [
  { title: t('panel.columnFormat'),  slotName: 'format',   width: 220 },
  { title: t('panel.columnRange'),   slotName: 'range',    width: 220 },
  { title: t('panel.columnShortcut'),slotName: 'shortcut', width: 150 },
  { title: t('panel.columnExample'), slotName: 'example' }
])
</script>

<style scoped>
/* 组件特有样式；通用 utility class (flex / items-center / gap-* / text-* / 颜色 / mb-? 等) 已集中在 src/styles/form-controls.css */
.vd-card-box {
  border: 1px solid var(--color-border-2, #e5e6eb);
  border-radius: 8px;
  background: var(--color-bg-1, #fff);
  overflow: hidden;
}
.vd-card-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  cursor: pointer;
  user-select: none;
}
.vd-card-title-row:hover {
  background: var(--color-fill-2, #f7f8fa);
}
.vd-section-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--color-text-1, #1d2129);
}
.vd-title-icon {
  font-size: 16px;
  color: var(--color-primary-6, #165dff);
}
.vd-spacer { flex: 1 1 auto; }

.help-table :deep(.arco-table-th),
.help-table :deep(.arco-table-td) {
  background: transparent !important;
  border-bottom: 1px solid var(--color-border-2, #f0f1f3) !important;
}

/* Expand 过渡 */
.expand-enter-active, .expand-leave-active {
  transition: opacity 0.2s ease, max-height 0.3s ease;
  overflow: hidden;
}
.expand-enter-from, .expand-leave-to {
  opacity: 0;
  max-height: 0;
}
.expand-enter-to, .expand-leave-from {
  opacity: 1;
  max-height: 1000px;
}
</style>
