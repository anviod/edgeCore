<template>
  <a-modal
    v-model:visible="visible"
    :title="title"
    width="620px"
    :footer="false"
    unmount-on-close
  >
    <!-- 无更新 -->
    <template v-if="mode === 'no-update'">
      <div class="upd-empty">
        <icon-check-circle class="upd-empty__icon" :size="44" />
        <p class="upd-empty__title">当前已是最新版本</p>
        <p class="upd-empty__desc">
          当前版本 <span class="upd-tag">{{ current }}</span>
          <template v-if="latestTag">，已优于最新发布 <span class="upd-tag">{{ latestTag }}</span></template>
        </p>
      </div>
    </template>

    <!-- 有新版本 -->
    <template v-else-if="mode === 'has-update'">
      <div class="upd-head">
        <div class="upd-head__badge">
          <icon-thunderbolt :size="18" />
        </div>
        <div class="upd-head__info">
          <div class="upd-head__row">
            <span class="upd-head__tag">{{ latest.tag }}</span>
            <span v-if="latest.prerelease" class="upd-flag">预发布</span>
          </div>
          <div v-if="latest.name && latest.name !== latest.tag" class="upd-head__name">{{ latest.name }}</div>
        </div>
        <div class="upd-head__meta">
          <span class="upd-muted">当前 {{ current }}</span>
          <a-button v-if="latest.htmlUrl" type="text" size="mini" @click="openGithub">
            查看发布页
            <icon-link :size="12" />
          </a-button>
        </div>
      </div>

      <div v-if="latest.publishedAt" class="upd-time">
        发布于 {{ formatDate(latest.publishedAt) }}
      </div>

      <div class="upd-notes">
        <div class="upd-notes__title">更新内容</div>
        <div
          v-if="hasBody"
          class="upd-notes__body"
          v-html="releaseHtml"
        ></div>
        <div v-else class="upd-muted">本次发布未填写更新说明。</div>
      </div>

      <div class="upd-actions">
        <a-button @click="visible = false">暂不升级</a-button>
        <a-button
          type="primary"
          status="success"
          :loading="running"
          :disabled="running"
          @click="startUpgrade"
        >
          <icon-thunderbolt v-if="!running" :size="14" />
          {{ running ? '升级中...' : '立即升级' }}
        </a-button>
      </div>
      <p class="upd-warn">
        <icon-info-circle :size="14" />
        升级过程将自动下载升级包、校验完整性并重启服务，期间网关可能短暂不可用。
      </p>
    </template>

    <!-- 升级进行中 / 失败 -->
    <template v-else-if="mode === 'upgrading'">
      <div class="upd-progress">
        <div class="upd-progress__head">
          <span class="upd-progress__title">{{ upgradeTitle }}</span>
          <span class="upd-progress__stage">{{ stageLabel }}</span>
        </div>
        <a-progress
          :percent="percentage"
          :status="progressStatus"
          :stroke-width="8"
        />
        <div class="upd-progress__detail">
          <span>目标版本 <span class="upd-tag">{{ targetVersion }}</span></span>
          <span class="upd-muted">{{ stageHint }}</span>
        </div>
        <div v-if="stage === 'failed'" class="upd-error">
          <icon-close-circle :size="16" />
          <span>{{ stageError || '升级失败，请查看系统日志' }}</span>
        </div>
      </div>
      <div class="upd-actions">
        <template v-if="stage === 'failed'">
          <a-button @click="close">关闭</a-button>
          <a-button type="primary" @click="checkNow">重新检查</a-button>
        </template>
        <template v-else-if="stage === 'upstaging'">
          <a-button type="primary" @click="reloadPage">刷新以应用新版本</a-button>
        </template>
      </div>
    </template>

    <!-- 检查失败 -->
    <template v-else>
      <div class="upd-empty">
        <icon-close-circle class="upd-empty__icon upd-empty__icon--error" :size="44" />
        <p class="upd-empty__title">检查更新失败</p>
        <p class="upd-empty__desc">{{ checkError || '无法连接更新服务器，请检查网络后重试' }}</p>
      </div>
      <div class="upd-actions">
        <a-button @click="visible = false">关闭</a-button>
        <a-button type="primary" @click="checkNow">重新检查</a-button>
      </div>
    </template>
  </a-modal>
</template>

<script setup>
import { ref, computed } from 'vue'
import {
  IconCheckCircle, IconCloseCircle, IconThunderbolt,
  IconLink, IconInfoCircle
} from '@arco-design/web-vue/es/icon'
import UpdateApi from '@/api/update'
import { formatMarkdownLite } from '@/utils/markdownLite'
import { showMessage } from '@/composables/useGlobalState'

// 后端升级阶段 → 中文文案
const STAGE_LABELS = {
  idle: '空闲',
  downloading: '下载升级包',
  verifying: '校验完整性',
  installing: '安装升级包',
  restarting: '重启服务',
  upstaging: '升级完成',
  failed: '升级失败'
}
// 进度百分比（阶段粗粒度映射）
const STAGE_PCT = {
  idle: 0,
  downloading: 30,
  verifying: 55,
  installing: 75,
  restarting: 92,
  upstaging: 100,
  failed: 100
}

const visible = ref(false)
// 展示模式：no-update | has-update | upgrading | error
const mode = ref('no-update')
const current = ref('')
const latestTag = ref('')
const latest = ref(null)
const checkError = ref('')
const hasBody = computed(() => !!(latest.value && latest.value.body))

/**
 * GitHub 自动生成的发布说明为 `* <完整哈希> <提交摘要>` 形式。
 * 先转为 `-` 无序列表并截断为 7 位短哈希，再由 markdownLite 渲染为列表项，
 * 避免整段 40 位字符串撑破布局。
 */
function beautifyReleaseBody(body) {
  if (!body) return body
  return body.replace(
    /^\*\s+([0-9a-fA-F]{7,40})(.*)$/gm,
    (_m, hash, rest) => `- \`${hash.slice(0, 7)}\`${rest}`
  )
}

const releaseHtml = computed(() => {
  const body = latest.value?.body || ''
  return formatMarkdownLite(beautifyReleaseBody(body))
})

const stage = ref('idle')
const targetVersion = ref('')
const startedAt = ref('')
const stageError = ref('')
let pollTimer = null

const title = computed(() => {
  if (mode.value === 'upgrading') return '软件升级'
  if (mode.value === 'has-update') return '发现新版本'
  if (mode.value === 'no-update') return '软件更新'
  return '软件更新'
})

const running = computed(() => stage.value !== 'idle')

const stageLabel = computed(() => STAGE_LABELS[stage.value] || stage.value)

const stageHint = computed(() => {
  switch (stage.value) {
    case 'downloading': return '正在从发布源下载升级包，请勿关闭页面'
    case 'verifying': return '正在校验文件完整性与安全性'
    case 'installing': return '正在备份并替换系统文件'
    case 'restarting': return '服务重启中，连接可能短暂中断'
    case 'failed': return ''
    case 'upstaging': return '新版本已就绪'
    default: return ''
  }
})

const percentage = computed(() => STAGE_PCT[stage.value] ?? 0)

const progressStatus = computed(() => {
  if (stage.value === 'failed') return 'error'
  if (stage.value === 'upstaging') return 'success'
  return 'normal'
})

const upgradeTitle = computed(() => {
  if (stage.value === 'failed') return '升级失败'
  if (stage.value === 'upstaging') return '升级成功'
  return '正在升级到 ' + targetVersion.value
})

const open = () => {
  visible.value = true
}

const close = () => {
  visible.value = false
  stopPoll()
}

const formatDate = (s) => {
  if (!s) return ''
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return s
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

const openGithub = () => {
  if (latest.value?.htmlUrl) {
    window.open(latest.value.htmlUrl, '_blank', 'noopener')
  }
}

// 本地保存在当前会话是否已验证过新版本（避免无更新时反复打扰）。
// checkNow(true) 由手动检查触发，总弹窗展示结果。
const checkNow = async (showResult = true) => {
  stopPoll()
  try {
    const res = await UpdateApi.checkUpdate()
    if (res.code !== '0') {
      throw new Error(res.message || '检查更新失败')
    }
    current.value = res.data.current || ''
    if (res.data.hasUpdate && res.data.latest) {
      latest.value = res.data.latest
      latestTag.value = res.data.latest.tag || ''
      mode.value = 'has-update'
      stage.value = 'idle'
    } else {
      mode.value = 'no-update'
      latestTag.value = res.data.latest ? res.data.latest.tag : ''
    }
  } catch (e) {
    checkError.value = e.message || e
    mode.value = 'error'
  }
  if (showResult) visible.value = true
}

// 开始一键升级
const startUpgrade = async () => {
  if (!latest.value) return
  const tag = latest.value.tag
  try {
    const res = await UpdateApi.performUpdate(tag)
    if (res.code && res.code !== '0') {
      throw new Error(res.message || '触发升级失败')
    }
    mode.value = 'upgrading'
    stage.value = 'downloading'
    targetVersion.value = tag
    startedAt.value = new Date().toISOString()
    showMessage('已开始升级，正在下载升级包...', 'warning')
    startPoll()
  } catch (e) {
    showMessage(e.message || '触发升级失败', 'error')
    // 后端 409/400 等场景：可能是已在升级或无需升级，同步一次真实状态
    refreshStatus()
  }
}

// 轮询升级状态
const startPoll = () => {
  stopPoll()
  pollTimer = window.setInterval(refreshStatus, 2500)
  refreshStatus()
}

const stopPoll = () => {
  if (pollTimer) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
}

const refreshStatus = async () => {
  try {
    const res = await UpdateApi.updateStatus()
    if (res.code !== '0') return
    const st = res.data || {}
    const prev = stage.value
    stage.value = st.stage || 'idle'
    if (st.target_version) targetVersion.value = st.target_version
    stageError.value = st.error || ''
    if (st.started_at) startedAt.value = st.started_at

    if (prev !== 'upgrading' && stage.value === 'upstaging') {
      showMessage('升级成功，请刷新页面体验新版本', 'success')
    }
    // 失败时停止轮询但保留弹窗展示错误
    const runningStages = ['downloading', 'verifying', 'installing', 'restarting']
    if (stage.value === 'failed' || stage.value === 'upstaging') {
      stopPoll()
    } else if (runningStages.includes(stage.value)) {
      mode.value = 'upgrading'
    }
  } catch (e) {
    // 升级重启期间接口短暂不可达属正常，保持轮询
    console.error('查询升级状态失败:', e)
  }
}

const reloadPage = () => {
  stopPoll()
  window.location.reload()
}

defineExpose({ open, checkNow })
</script>

<style scoped>
.upd-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 28px 0 8px;
  text-align: center;
}
.upd-empty__icon {
  color: var(--color-success-6);
}
.upd-empty__icon--error {
  color: var(--color-danger-6);
}
.upd-empty__title {
  margin: 12px 0 4px;
  font-size: 16px;
  font-weight: 600;
  color: var(--color-text-1);
}
.upd-empty__desc {
  margin: 0;
  font-size: 13px;
  color: var(--color-text-2);
}
.upd-tag {
  padding: 1px 8px;
  margin: 0 2px;
  border-radius: 4px;
  background: var(--color-fill-2);
  font-family: ui-monospace, monospace;
  font-size: 12px;
}
.upd-muted {
  font-size: 12px;
  color: var(--color-text-3);
}

.upd-head {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 6px 0;
}
.upd-head__badge {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  border-radius: 14px;
  color: #fff;
  background: linear-gradient(135deg, var(--primary), var(--primary-hover));
  box-shadow: 0 6px 16px rgba(14, 165, 233, 0.28);
  flex-shrink: 0;
}
.upd-head__row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.upd-head__tag {
  font-family: ui-monospace, monospace;
  font-size: 18px;
  font-weight: 700;
  color: var(--color-text-1);
}
.upd-flag {
  padding: 0 6px;
  font-size: 11px;
  border-radius: 4px;
  color: var(--color-warning-6);
  border: 1px solid var(--color-warning-6);
}
.upd-head__name {
  font-size: 12px;
  color: var(--color-text-2);
  margin-top: 2px;
}
.upd-head__meta {
  margin-left: auto;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 2px;
}
.upd-time {
  font-size: 12px;
  color: var(--color-text-3);
  margin: 4px 0 10px;
}

.upd-notes {
  border: 1px solid var(--color-border-2);
  border-radius: 8px;
  padding: 12px 14px;
  max-height: 260px;
  overflow: auto;
  background: var(--color-fill-1);
}
.upd-notes__title {
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text-1);
  margin-bottom: 8px;
}
.upd-notes__body {
  font-size: 13px;
  line-height: 1.7;
  color: var(--color-text-2);
}
.upd-notes__body :deep(.ai-md-ul) {
  margin: 0;
  padding-left: 2px;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.upd-notes__body :deep(.ai-md-li) {
  position: relative;
  line-height: 1.45;
  color: var(--color-text-2);
  padding-left: 16px;
}
.upd-notes__body :deep(.ai-md-li)::before {
  content: '';
  position: absolute;
  left: 2px;
  top: 8px;
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--primary);
  opacity: 0.7;
}
.upd-notes__body :deep(.ai-md-code) {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11.5px;
  padding: 1px 6px;
  border-radius: 5px;
  background: var(--color-fill-2);
  color: var(--color-text-1);
  letter-spacing: 0.2px;
  white-space: nowrap;
  vertical-align: baseline;
}
.upd-notes__body :deep(strong) {
  color: var(--color-text-1);
}

.upd-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 20px;
}
.upd-warn {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  margin: 12px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--color-text-3);
}

.upd-progress {
  padding: 12px 4px 4px;
}
.upd-progress__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}
.upd-progress__title {
  font-size: 15px;
  font-weight: 600;
  color: var(--color-text-1);
}
.upd-progress__stage {
  font-size: 13px;
  color: var(--color-primary-6);
  font-weight: 600;
}
.upd-progress__detail {
  display: flex;
  justify-content: space-between;
  margin-top: 10px;
  font-size: 12px;
}
.upd-error {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  margin-top: 14px;
  padding: 10px 12px;
  border-radius: 6px;
  font-size: 13px;
  color: var(--color-danger-6);
  background: var(--color-danger-light-1);
}
</style>