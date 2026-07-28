<template>
  <Teleport to="body">
    <template v-if="!isLoginPage">
      <!-- FAB -->
      <button
        v-if="!state.expanded"
        type="button"
        class="ai-fab"
        :class="{ 'mcp-glow': mcpConnected }"
        title="打开 AI助手"
        aria-label="打开 AI助手"
        popovertarget="ai-assistant-hint"
        @click="openPanel"
      >
        <span class="ai-fab__pulse" aria-hidden="true"></span>
        <AiAssistantIcon />
        <span class="ai-fab__label">AI助手</span>
      </button>

      <div id="ai-assistant-hint" popover="hint" class="ai-fab-hint">
        AI助手 — 协议逆向 · 生产配置 · 校验 · 诊断
      </div>

      <!-- Main panel -->
      <dialog
        ref="dialogRef"
        class="ai-panel"
        :class="{
          'ai-panel--open': state.expanded,
          'ai-panel--mini': state.miniMode,
          'ai-panel--dragging': dragging
        }"
        :open="state.expanded || undefined"
        :style="panelStyle"
        aria-labelledby="ai-panel-title"
        @close="onDialogClose"
        @click.stop
      >
        <div class="ai-panel__shell">
          <!-- Header -->
          <header
            class="ai-panel__header"
            :class="{ 'ai-panel__header--mini': state.miniMode }"
            @pointerdown="onDragStart"
          >
            <div class="ai-panel__title">
              <AiAssistantIcon v-if="!state.miniMode" />
              <div class="ai-panel__title-text">
                <span id="ai-panel-title" class="ai-panel__title-main">AI助手</span>
                <span v-if="!state.miniMode" class="ai-panel__title-sub">{{ statusLabel }}</span>
              </div>
            </div>
            <div class="ai-panel__actions">
              <button
                type="button"
                class="ai-panel__action"
                title="AI 设置"
                aria-label="打开 AI 设置"
                @click.stop="openSettings"
              >
                <icon-settings :size="14" />
              </button>
              <button
                type="button"
                class="ai-panel__action"
                :title="state.miniMode ? '展开' : '迷你'"
                :aria-label="state.miniMode ? '展开面板' : '迷你模式'"
                @click.stop="toggleMini"
              >
                <icon-shrink v-if="!state.miniMode" :size="14" />
                <icon-expand v-else :size="14" />
              </button>
              <button
                type="button"
                class="ai-panel__action"
                title="收起 (Esc)"
                aria-label="收起面板"
                @click.stop="handleCollapse"
              >
                <icon-minus :size="14" />
              </button>
            </div>
          </header>

          <!-- Mini mode body -->
          <div v-if="state.miniMode" class="ai-panel__mini-body" @click="toggleMini">
            <AiAssistantIcon />
            <span class="ai-panel__mini-hint">点击展开</span>
          </div>

          <!-- Full mode body -->
          <template v-else>
            <AiQuotaBar :quota="quota" :mode="aiStatus?.mode || 'remote'" />

            <nav class="ai-panel__tabs" role="tablist" aria-label="工作台">
              <button
                v-for="ws in AI_WORKSPACES"
                :key="ws.id"
                type="button"
                role="tab"
                class="ai-tab"
                :class="{ 'ai-tab--active': state.workspace === ws.id }"
                :aria-selected="state.workspace === ws.id"
                @click="setWorkspace(ws.id)"
              >
                <span class="ai-tab__goal">{{ ws.goal }}</span>
                <span class="ai-tab__label">{{ ws.label }}</span>
              </button>
            </nav>

            <div class="ai-panel__body">
              <div
                class="ai-split"
                :class="{
                  'ai-split--workspace-collapsed': state.splitMode === 'chat',
                  'ai-split--chat-hidden': state.splitMode === 'workspace'
                }"
              >
                <!-- Workspace pane -->
                <section
                  v-show="state.splitMode !== 'chat'"
                  class="ai-split__workspace"
                  aria-label="工作台"
                >
                  <div class="ai-split__workspace-toolbar">
                    <span class="ai-split__workspace-label">工作台</span>
                  </div>
                  <div class="ai-split__workspace-content">
                    <AiTaskHistory
                      :tasks="tasks"
                      :active-id="activeTask?.id"
                      :loading="copilotLoading"
                      @select="selectTask"
                      @refresh="fetchTasks"
                    />
                    <AiWorkbenchProtocol
                      v-if="state.workspace === 'protocol'"
                      :task="activeTask"
                      :stages="stages"
                      :deliverables="activeDeliverables"
                      :loading="copilotLoading"
                      :upload-progress="uploadProgress"
                      @upload="handleUpload"
                      @export="exportDeliverable"
                      @export-all="exportAll"
                      @confirm="handleConfirm"
                    />
                    <AiWorkbenchValidation
                      v-else-if="state.workspace === 'validation'"
                      :deliverables="activeDeliverables"
                      :validation="validation"
                      :loading="copilotLoading"
                      @validate="handleValidate"
                    />
                    <AiWorkbenchCases
                      v-else-if="state.workspace === 'cases'"
                      :deliverables="activeDeliverables"
                    />
                    <AiWorkbenchEdge v-else-if="state.workspace === 'edge'" />
                    <AiWorkbenchDiagnostics v-else-if="state.workspace === 'diagnostics'" />
                    <AiMcpHelp v-else-if="state.workspace === 'mcp'" />
                    <AiEanPanel v-else-if="state.workspace === 'ean'" />
                  </div>
                </section>

                <!-- Splitter button -->
                <button
                  type="button"
                  class="ai-split__divider"
                  :title="splitterTitle"
                  :aria-label="splitterTitle"
                  @click="toggleSplitMode"
                >
                  <icon-left v-if="state.splitMode === 'both'" :size="10" />
                  <icon-right v-else :size="10" />
                </button>

                <!-- Chat pane -->
                <AiChatSidebar
                  v-show="state.splitMode !== 'workspace'"
                  :workspace="state.workspace"
                  :task-id="activeTask?.id || ''"
                  :task-status="activeTask?.status || ''"
                  class="ai-split__chat"
                />
              </div>
            </div>
          </template>

          <div
            v-if="!state.miniMode"
            class="ai-panel__resize-handle"
            aria-hidden="true"
            @pointerdown="onResizeStart"
          ></div>
        </div>
      </dialog>
    </template>
  </Teleport>

  <AiSettingsDialog
    v-if="!isLoginPage"
    v-model="settingsOpen"
    :settings="aiSettings"
    :saving="settingsSaving"
    @save="handleSaveSettings"
    @refresh="fetchSettings"
  />
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import {
  IconShrink, IconExpand, IconMinus, IconLeft, IconRight, IconSettings
} from '@arco-design/web-vue/es/icon'
import { useAiAssistant, AI_WORKSPACES } from '@/composables/useAiAssistant'
import { useAiCopilot } from '@/composables/useAiCopilot'
import AiAssistantIcon from './AiAssistantIcon.vue'
import AiQuotaBar from './AiQuotaBar.vue'
import AiChatSidebar from './AiChatSidebar.vue'
import AiTaskHistory from './AiTaskHistory.vue'
import AiWorkbenchProtocol from './AiWorkbenchProtocol.vue'
import AiWorkbenchValidation from './AiWorkbenchValidation.vue'
import AiWorkbenchCases from './AiWorkbenchCases.vue'
import AiWorkbenchEdge from './AiWorkbenchEdge.vue'
import AiWorkbenchDiagnostics from './AiWorkbenchDiagnostics.vue'
import AiMcpHelp from './AiMcpHelp.vue'
import AiEanPanel from './AiEanPanel.vue'
import AiSettingsDialog from './AiSettingsDialog.vue'

const route = useRoute()
const isLoginPage = computed(() => route.path === '/login' || route.path === '/install')

const {
  state, mcpConnected, setExpanded, setMiniMode, setWorkspace, toggleSplitMode,
  setPosition, setSize, collapseToFab
} = useAiAssistant()

const {
  tasks, activeTask, activeDeliverables, validation, stages, quota, aiStatus, aiSettings,
  loading: copilotLoading, uploadProgress,
  fetchStatus, fetchQuota, fetchTasks, fetchSettings, saveSettings, uploadAndCreate, selectTask, confirmTask, runValidation,
  exportDeliverable, stopPolling
} = useAiCopilot()

const settingsOpen = ref(false)
const settingsSaving = ref(false)

const dialogRef = ref(null)
const dragging = ref(false)
const initialized = ref(false)

const statusLabel = computed(() => {
  if (!aiStatus.value) return '连接中…'
  return `${aiStatus.value.provider || 'AI Model Center'} · 四阶段流水线`
})

const splitterTitle = computed(() => {
  const map = {
    both: '收起工作台',
    chat: '收起对话',
    workspace: '展开分栏'
  }
  return map[state.value.splitMode] || '切换分栏'
})

const openSettings = async () => {
  await fetchSettings()
  settingsOpen.value = true
}

const handleSaveSettings = async (payload) => {
  settingsSaving.value = true
  try {
    await saveSettings(payload)
    await fetchSettings()
    await Promise.all([fetchStatus(), fetchQuota()])
    settingsOpen.value = false
    Message.success('AI 设置已保存')
  } catch (e) {
    Message.error(e.message || '保存失败')
  } finally {
    settingsSaving.value = false
  }
}

const panelStyle = computed(() => {
  const { position, size, miniMode } = state.value
  const style = {}
  if (position.x != null && position.y != null) {
    const maxX = Math.max(8, window.innerWidth - Math.min(size.width, 200))
    const maxY = Math.max(8, window.innerHeight - Math.min(size.height, 80))
    style.left = `${Math.max(8, Math.min(position.x, maxX))}px`
    style.top = `${Math.max(8, Math.min(position.y, maxY))}px`
    style.right = 'auto'
    style.bottom = 'auto'
  }
  if (!miniMode) {
    style.width = `${size.width}px`
    style.height = `${size.height}px`
  }
  return style
})

const handleUpload = async ({ file, skill, protocol_id, observations }) => {
  try {
    await uploadAndCreate(file, { skill, protocol_id, observations })
    Message.success('任务已创建，流水线运行中…')
  } catch (e) {
    Message.error(e.message || '上传失败')
  }
}

const handleConfirm = async (applyMode = 'preview') => {
  try {
    await confirmTask(applyMode)
    const msg = applyMode === 'import'
      ? 'Human Confirm 完成（导入模式 · 本地 Mock 未写入 config.db）'
      : 'Human Confirm 完成（预览模式 · 产出已确认）'
    Message.success(msg)
  } catch (e) {
    Message.error(e.message || '确认失败')
  }
}

const handleValidate = async () => {
  if (!activeDeliverables.value) return
  try {
    await runValidation(activeDeliverables.value)
    if (activeTask.value?.id) await selectTask(activeTask.value.id)
    Message.success('校验完成')
  } catch (e) {
    Message.error(e.message || '校验失败')
  }
}

const exportAll = () => {
  ['protocol_model', 'point_definition', 'driver_parameter', 'validation_case'].forEach(exportDeliverable)
  Message.success('已导出全部 JSON')
}

const openPanel = async () => {
  setExpanded(true)
  if (!initialized.value) {
    await Promise.all([fetchStatus(), fetchQuota(), fetchTasks()])
    initialized.value = true
  }
}

const handleCollapse = () => {
  collapseToFab()
  dialogRef.value?.close()
}

const onDialogClose = () => {
  if (state.value.expanded) collapseToFab()
}

const toggleMini = () => setMiniMode(!state.value.miniMode)

const onKeyDown = (e) => {
  if (e.key === 'Escape' && state.value.expanded) {
    e.preventDefault()
    handleCollapse()
  }
}

let dragStart = null
const onDragStart = (e) => {
  if (e.target.closest('.ai-panel__action') || state.value.miniMode) return
  const dialog = dialogRef.value
  if (!dialog) return
  const rect = dialog.getBoundingClientRect()
  dragStart = { pointerX: e.clientX, pointerY: e.clientY, left: rect.left, top: rect.top }
  dragging.value = true
  e.currentTarget.setPointerCapture(e.pointerId)
  e.currentTarget.addEventListener('pointermove', onDragMove)
  e.currentTarget.addEventListener('pointerup', onDragEnd)
  e.currentTarget.addEventListener('pointercancel', onDragEnd)
}
const onDragMove = (e) => {
  if (!dragStart) return
  setPosition(
    Math.max(8, Math.min(window.innerWidth - 100, dragStart.left + e.clientX - dragStart.pointerX)),
    Math.max(8, Math.min(window.innerHeight - 60, dragStart.top + e.clientY - dragStart.pointerY))
  )
}
const onDragEnd = (e) => {
  dragging.value = false
  dragStart = null
  e.currentTarget.releasePointerCapture(e.pointerId)
  e.currentTarget.removeEventListener('pointermove', onDragMove)
  e.currentTarget.removeEventListener('pointerup', onDragEnd)
  e.currentTarget.removeEventListener('pointercancel', onDragEnd)
}

let resizeStart = null
const onResizeStart = (e) => {
  e.stopPropagation()
  const dialog = dialogRef.value
  if (!dialog) return
  const rect = dialog.getBoundingClientRect()
  resizeStart = { pointerX: e.clientX, pointerY: e.clientY, width: rect.width, height: rect.height }
  e.currentTarget.setPointerCapture(e.pointerId)
  e.currentTarget.addEventListener('pointermove', onResizeMove)
  e.currentTarget.addEventListener('pointerup', onResizeEnd)
  e.currentTarget.addEventListener('pointercancel', onResizeEnd)
}
const onResizeMove = (e) => {
  if (!resizeStart) return
  setSize(
    Math.max(320, resizeStart.width + e.clientX - resizeStart.pointerX),
    Math.max(240, resizeStart.height + e.clientY - resizeStart.pointerY)
  )
}
const onResizeEnd = (e) => {
  resizeStart = null
  e.currentTarget.releasePointerCapture(e.pointerId)
  e.currentTarget.removeEventListener('pointermove', onResizeMove)
  e.currentTarget.removeEventListener('pointerup', onResizeEnd)
  e.currentTarget.removeEventListener('pointercancel', onResizeEnd)
}

onMounted(() => {
  window.addEventListener('keydown', onKeyDown)
  fetchStatus()
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeyDown)
  stopPolling()
  if (dialogRef.value?.open) dialogRef.value.close()
})

defineExpose({
  open: openPanel,
  toggle: () => (state.value.expanded ? handleCollapse() : openPanel())
})
</script>
