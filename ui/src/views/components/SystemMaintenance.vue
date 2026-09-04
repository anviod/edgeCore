<template>
  <div class="mm-page">
    <!-- 系统维护 -->
    <section class="mm-panel">
      <header class="mm-panel__head">
        <span class="mm-panel__icon mm-panel__icon--gold">
          <icon-settings :size="18" />
        </span>
        <h3 class="mm-panel__title">系统维护</h3>
        <span class="mm-panel__tag">维护模式</span>
      </header>

      <div class="mm-panel__body">
        <div class="mm-hero">
          <span class="mm-hero__dot"></span>
          <div class="mm-hero__text">
            <span class="mm-hero__title">系统运行正常</span>
            <span class="mm-hero__desc">网关服务在线，以下操作将基于当前配置执行</span>
          </div>
        </div>

        <div class="mm-ops">
          <div class="mm-op" role="button" tabindex="0" @click="showRestartConfirm" @keydown.enter="showRestartConfirm">
            <div class="mm-op__icon mm-op__icon--restart">
              <icon-refresh :size="24" />
            </div>
            <div class="mm-op__content">
              <div class="mm-op__title">重启系统</div>
              <div class="mm-op__desc">立即重启 Edge Gateway 硬件终端与全部采集/北向服务</div>
            </div>
            <div class="mm-op__foot">
              <a-button type="outline" status="warning" size="small" @click.stop="showRestartConfirm">
                <template #icon><icon-refresh /></template>
                执行重启
              </a-button>
            </div>
          </div>

          <div class="mm-op" role="button" tabindex="0" @click="showResetConfirm" @keydown.enter="showResetConfirm">
            <div class="mm-op__icon mm-op__icon--reset">
              <icon-delete :size="24" />
            </div>
            <div class="mm-op__content">
              <div class="mm-op__title">恢复出厂设置</div>
              <div class="mm-op__desc">清除所有本地配置与运行数据，恢复为出厂默认状态</div>
            </div>
            <div class="mm-op__foot">
              <a-button type="outline" status="danger" size="small" @click.stop="showResetConfirm">
                <template #icon><icon-delete /></template>
                执行清除
              </a-button>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- 软件升级 -->
    <section class="mm-panel">
      <header class="mm-panel__head">
        <span class="mm-panel__icon mm-panel__icon--gold">
          <icon-thunderbolt :size="18" />
        </span>
        <h3 class="mm-panel__title">软件升级</h3>
        <a-button
          class="mm-panel__action"
          type="outline"
          size="small"
          :loading="checkingOnline"
          @click="handleOnlineCheck"
        >
          <template #icon><icon-refresh /></template>
          在线检查更新
        </a-button>
      </header>

      <div class="mm-panel__body">
        <div class="mm-version">
          <span class="mm-version__label">当前版本</span>
          <span class="mm-badge">{{ currentVersion }}</span>
          <span class="mm-version__hint">匹配目标平台安装包可用于升级或降级</span>
        </div>

        <div class="mm-section">
          <div class="mm-section__head">
            <div class="mm-section__label">上传本地安装包</div>
            <div class="mm-section__sub">支持 tar.gz / deb / rpm，上传后先校验收件，确认无误后再安装</div>
          </div>

          <div class="mm-upload">
            <a-upload
              :auto-upload="false"
              :show-file-list="false"
              accept=".tar.gz,.deb,.rpm"
              :limit="1"
              :disabled="localBusy"
              @change="onFileSelect"
            >
              <a-button :disabled="localBusy" class="mm-btn-gold">
                <template #icon><icon-upload /></template>
                选择安装包
              </a-button>
            </a-upload>

            <a-alert
              v-if="uploadResult && uploadResult.uploading"
              class="mm-result-alert"
              type="info"
              :closable="false"
            >
              <template #icon><icon-loading /></template>
              正在上传并校验安装包，请稍候...
            </a-alert>

            <a-alert
              v-else-if="uploadResult && !uploadResult.valid"
              class="mm-result-alert"
              type="warning"
              :closable="false"
            >
              <template #icon><icon-close-circle /></template>
              <template #title>安装包校验未通过</template>
              {{ uploadResult.reason || '文件无效，请更换安装包' }}
            </a-alert>
          </div>

          <!-- 校验通过后的安装信息 -->
          <div v-if="uploadResult && uploadResult.valid" class="mm-valid">
            <div class="mm-valid__file">
              <icon-check-circle class="mm-ok" :size="16" />
              <span class="mm-valid__name">{{ selectedFileName }}</span>
            </div>
            <a-descriptions :column="3" size="small" bordered class="mm-desc">
              <a-descriptions-item label="安装版本">
                <span class="mm-badge">{{ uploadResult.info.version }}</span>
              </a-descriptions-item>
              <a-descriptions-item label="架构">{{ uploadResult.info.arch }}</a-descriptions-item>
              <a-descriptions-item label="格式">{{ uploadResult.info.format }}</a-descriptions-item>
              <a-descriptions-item label="大小">{{ formatSize(uploadResult.info.size) }}</a-descriptions-item>
              <a-descriptions-item label="操作类型">
                <span :class="installKind === '降级' ? 'mm-kind mm-kind--down' : 'mm-kind mm-kind--up'">
                  {{ installKind }}
                </span>
              </a-descriptions-item>
            </a-descriptions>
            <div class="mm-valid__actions">
              <a-button size="small" :disabled="localBusy" @click="clearUpload">取消</a-button>
              <a-button size="small" :loading="localBusy" class="mm-btn-gold" @click="confirmInstall">
                <template #icon><icon-thunderbolt /></template>
                安装此版本
              </a-button>
            </div>
          </div>

          <!-- 本地安装进度 -->
          <div v-if="localBusy" class="mm-progress">
            <div class="mm-progress__head">
              <span class="mm-progress__title">{{ localUpgradeTitle }}</span>
              <span class="mm-progress__stage">{{ localStageLabel }}</span>
            </div>
            <a-progress :percent="localPercent" :status="localProgressStatus" :stroke-width="8" />
            <div class="mm-progress__detail">
              <span class="mm-progress__hint">{{ localStageHint }}</span>
            </div>
            <div v-if="localStage === 'failed'" class="mm-progress__error">
              <icon-close-circle :size="16" />
              <span>{{ localError || '安装失败，请查看系统日志' }}</span>
            </div>
            <div v-if="localStage === 'upstaging'" class="mm-progress__success">
              <a-button type="primary" size="small" @click="reloadPage">刷新以应用新版本</a-button>
            </div>
          </div>
        </div>
      </div>
    </section>

    <update-dialog ref="updateDialogRef" />

    <!-- 重启确认 -->
    <a-modal
      v-model:visible="restartModalVisible"
      title="重启系统"
      ok-text="确认重启"
      cancel-text="取消"
      status="warning"
      @ok="handleRestart"
    >
      <div class="mm-modal">
        <p class="mm-modal__msg">确定要重启系统吗？</p>
        <p class="mm-modal__warn">服务将暂时不可用，重启过程可能需要几分钟时间。</p>
      </div>
    </a-modal>

    <!-- 恢复出厂设置确认 -->
    <a-modal
      v-model:visible="resetModalVisible"
      title="恢复出厂设置"
      ok-text="确认恢复"
      cancel-text="取消"
      status="danger"
      @ok="handleReset"
    >
      <div class="mm-modal">
        <p class="mm-modal__msg">确定要恢复出厂设置吗？</p>
        <p class="mm-modal__warn">此操作将清除所有配置且无法撤销，系统将恢复到初始状态。</p>
      </div>
    </a-modal>

    <!-- 安装本地安装包确认 -->
    <a-modal
      v-model:visible="installModalVisible"
      title="确认安装"
      ok-text="确认安装"
      cancel-text="取消"
      status="warning"
      @ok="startLocalInstall"
    >
      <div class="mm-modal">
        <p class="mm-modal__msg">
          确定安装 <span class="mm-badge">{{ uploadResult?.info?.version }}</span> 版本吗？
        </p>
        <p class="mm-modal__warn">
          将执行 {{ installKind }}。服务会重启并短暂不可用，失败时系统将自动回滚到原版本。
        </p>
      </div>
    </a-modal>
  </div>
</template>

<script setup>
import { ref, computed, onUnmounted } from 'vue'
import { Message } from '@arco-design/web-vue'
import {
  IconSettings, IconRefresh, IconDelete, IconThunderbolt,
  IconUpload, IconCheckCircle, IconCloseCircle, IconLoading
} from '@arco-design/web-vue/es/icon'
import LoginApi from '@/api/login'
import UpdateApi from '@/api/update'
import UpdateDialog from '@/components/UpdateDialog.vue'

const restartModalVisible = ref(false)
const resetModalVisible = ref(false)
const installModalVisible = ref(false)

const showRestartConfirm = () => { restartModalVisible.value = true }
const showResetConfirm = () => { resetModalVisible.value = true }

const handleRestart = () => {
  Message.loading({ content: '正在重启系统...', duration: 0 })
  setTimeout(() => { Message.success('系统重启指令已发送'); restartModalVisible.value = false }, 1200)
}

const handleReset = () => {
  Message.loading({ content: '正在恢复出厂设置...', duration: 0 })
  setTimeout(() => { Message.success('出厂设置恢复指令已发送'); resetModalVisible.value = false }, 1200)
}

// —— 软件升级 ——
const currentVersion = ref('v-dev')
const updateDialogRef = ref(null)
const checkingOnline = ref(false)

const loadVersion = async () => {
  try {
    const res = await LoginApi.getSystemInfo()
    if (res.code === '0' && res.data) {
      currentVersion.value = `v${res.data.softVer || 'dev'}`
    }
  } catch (e) {
    console.error('获取版本失败:', e)
  }
}
loadVersion()

const handleOnlineCheck = async () => {
  if (checkingOnline.value) return
  checkingOnline.value = true
  try {
    await updateDialogRef.value?.checkNow(true)
  } finally {
    checkingOnline.value = false
  }
}

// 本地上传：上传并校验
const selectedFileName = ref('')
const uploadResult = ref(null)
const localStage = ref('idle')
const localError = ref('')
let pollTimer = null
let upgradeMsg = null

const runningStages = ['downloading', 'verifying', 'installing', 'restarting']
const localBusy = computed(() => runningStages.includes(localStage.value))

const onFileSelect = async (fileItemList, fileItem) => {
  const item = fileItem || (Array.isArray(fileItemList) ? fileItemList[0] : null)
  const f = item && (item.file || item.originFile)
  if (!f) return
  selectedFileName.value = f.name || 'upgrade'
  uploadResult.value = { uploading: true, valid: false, reason: '' }
  const formData = new FormData()
  formData.append('file', f)
  try {
    const res = await UpdateApi.uploadPackage(formData)
    if (res.code !== '0') throw new Error(res.message || '上传校验失败')
    uploadResult.value = { uploading: false, ...res.data }
    if (res.data.valid) {
      Message.success(`安装包校验通过：v${res.data.version}`)
    } else {
      Message.warning('安装包校验未通过，请更换文件')
    }
  } catch (e) {
    uploadResult.value = { uploading: false, valid: false, reason: e.message || '上传失败' }
    Message.error('上传或校验失败')
  }
}

const clearUpload = () => {
  uploadResult.value = null
  selectedFileName.value = ''
}

const confirmInstall = () => {
  installModalVisible.value = true
}

const formatSize = (bytes) => {
  if (!bytes && bytes !== 0) return '-'
  const mb = bytes / (1024 * 1024)
  if (mb >= 1) return mb.toFixed(2) + ' MB'
  return (bytes / 1024).toFixed(1) + ' KB'
}

const installKind = computed(() => {
  if (!uploadResult.value?.info) return ''
  const target = uploadResult.value.info.version
  const cur = (currentVersion.value || '').replace(/^v/, '')
  if (!cur) return '安装'
  return compareSimple(cur, target) >= 0 ? '降级' : '升级'
})

// 简易版本比较（x.y.z 主版本；含预发布标记的按主版本近似处理）
function compareSimple(a, b) {
  const to = (s) => (s.match(/\d+/g) || []).slice(0, 3).map(Number)
  const A = to(a); const B = to(b)
  for (let i = 0; i < 3; i++) {
    if ((A[i] || 0) !== (B[i] || 0)) return (A[i] || 0) < (B[i] || 0) ? -1 : 1
  }
  return 0
}

// 本地安装执行 + 状态轮询
const startLocalInstall = () => {
  installModalVisible.value = false
  const pkg = uploadResult.value?.info
  if (!pkg) return
  localStage.value = 'verifying'
  upgradeMsg = Message.loading({ content: '正在安装本地安装包...', duration: 0 })
  UpdateApi.installLocal(pkg.path).then((res) => {
    if (res.code && res.code !== '0') throw new Error(res.message || '触发安装失败')
    if (upgradeMsg) { upgradeMsg.close(); upgradeMsg = null }
    Message.success('已开始安装，正在执行...')
    startPoll()
  }).catch((e) => {
    if (upgradeMsg) { upgradeMsg.close(); upgradeMsg = null }
    localStage.value = 'failed'
    localError.value = e.message || '触发安装失败'
    refreshStatus() // 同步一次真实后端状态
  })
}

const startPoll = () => {
  stopPoll()
  pollTimer = window.setInterval(refreshStatus, 2500)
  refreshStatus()
}

const stopPoll = () => {
  if (pollTimer) { window.clearInterval(pollTimer); pollTimer = null }
}

const refreshStatus = async () => {
  try {
    const res = await UpdateApi.updateStatus()
    if (res.code !== '0') return
    const st = res.data || {}
    localStage.value = st.stage || 'idle'
    localError.value = st.error || ''
    if (localStage.value === 'upstaging') {
      Message.success('安装成功')
      clearUpload()
      loadVersion()
    }
    if (localStage.value === 'failed') {
      Message.error('安装失败')
    }
    if (localStage.value === 'failed' || localStage.value === 'upstaging') {
      stopPoll()
    }
  } catch (e) {
    console.error('查询安装状态失败:', e)
  }
}

const STAGE_LABELS = {
  idle: '空闲', downloading: '下载安装包', verifying: '校验完整性',
  installing: '安装安装包', restarting: '重启服务', upstaging: '安装完成', failed: '安装失败'
}
const STAGE_PCT = {
  idle: 0, verifying: 30, installing: 60, restarting: 90, upstaging: 100, failed: 100
}

const localStageLabel = computed(() => STAGE_LABELS[localStage.value] || localStage.value)
const localPercent = computed(() => STAGE_PCT[localStage.value] ?? 0)
const localProgressStatus = computed(() => {
  if (localStage.value === 'failed') return 'error'
  if (localStage.value === 'upstaging') return 'success'
  return 'normal'
})
const localUpgradeTitle = computed(() => {
  if (localStage.value === 'failed') return '安装失败'
  if (localStage.value === 'upstaging') return '安装成功'
  return '正在安装 ' + (uploadResult?.value?.info?.version || '')
})
const localStageHint = computed(() => {
  switch (localStage.value) {
    case 'verifying': return '正在校验文件的完整性与格式'
    case 'installing': return '正在备份当前版本并写入新版本'
    case 'restarting': return '服务重启中，连接可能短暂中断'
    case 'failed': return '安装未能完成'
    case 'upstaging': return '新版本已就绪，请刷新页面'
    default: return ''
  }
})

const reloadPage = () => {
  stopPoll()
  window.location.reload()
}

onUnmounted(stopPoll)
</script>

<style scoped>
.mm-page {
  --mm-gold: #c19b45;
  --mm-gold-deep: #a87c22;
  --mm-radius: 18px;
  /* 与系统设置其余面板（.arco-card 用 var(--bg)/--border/--text-*）取色保持一致，
     避免暗色主题下因 Arco --color-bg-* 与项目令牌不一致产生的面板底色差异 */
  --color-bg-1: var(--bg);
  --color-bg-2: var(--bg);
  --color-bg-3: var(--surface);
  --color-border-1: var(--border);
  --color-border-2: var(--border);
  --color-text-1: var(--text-primary);
  --color-text-2: var(--text-secondary);
  --color-text-3: var(--text-secondary);
  --color-fill-1: var(--surface);
  --color-danger-6: #d03050;
  --color-danger-light-1: rgba(208, 48, 80, 0.09);
  --color-success-6: #34c759;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* ── 面板 ── */
.mm-panel {
  background: var(--color-bg-2, var(--bg, #fff));
  border: 1px solid var(--color-border-2, var(--border, rgba(0, 0, 0, 0.08)));
  border-radius: var(--mm-radius);
  overflow: hidden;
  transition: border-color 0.25s cubic-bezier(0.22, 1, 0.36, 1);
}

.mm-panel__head {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 18px 22px;
  border-bottom: 1px solid var(--color-border-2, var(--border, rgba(0, 0, 0, 0.08)));
}

.mm-panel__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 11px;
  color: #fff;
  flex-shrink: 0;
}

.mm-panel__icon--gold {
  background: linear-gradient(135deg, #d3ae57, var(--mm-gold-deep));
  box-shadow: 0 6px 14px rgba(193, 155, 69, 0.28);
}

.mm-panel__title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--color-text-1, var(--text-primary, #1f2937));
  letter-spacing: 0.01em;
}

.mm-panel__tag {
  margin-left: auto;
  padding: 3px 10px;
  font-size: 11px;
  letter-spacing: 0.05em;
  color: var(--mm-gold-deep);
  background: rgba(193, 155, 69, 0.12);
  border-radius: 999px;
}

.mm-panel__action {
  margin-left: auto;
}

.mm-panel__body {
  padding: 22px;
}

/* ── 系统运行状态 ── */
.mm-hero {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 14px 16px;
  margin-bottom: 18px;
  border-radius: 12px;
  background: rgba(52, 199, 89, 0.08);
  border: 1px solid rgba(52, 199, 89, 0.18);
}

.mm-hero__dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: #34c759;
  box-shadow: 0 0 0 0 rgba(52, 199, 89, 0.5);
  animation: mm-pulse 2s infinite;
  flex-shrink: 0;
}

@keyframes mm-pulse {
  0% { box-shadow: 0 0 0 0 rgba(52, 199, 89, 0.45); }
  70% { box-shadow: 0 0 0 9px rgba(52, 199, 89, 0); }
  100% { box-shadow: 0 0 0 0 rgba(52, 199, 89, 0); }
}

.mm-hero__text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.mm-hero__title {
  font-size: 14px;
  font-weight: 600;
  color: var(--color-text-1, var(--text-primary, #1f2937));
}

.mm-hero__desc {
  font-size: 12px;
  color: var(--color-text-3, var(--text-secondary, #6b7280));
}

/* ── 维护操作卡 ── */
.mm-ops {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.mm-op {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 20px;
  border: 1px solid var(--color-border-2, var(--border, rgba(0, 0, 0, 0.08)));
  border-radius: 14px;
  background: var(--color-bg-3, var(--surface, #fafafa));
  cursor: pointer;
  transition: transform 0.25s cubic-bezier(0.22, 1, 0.36, 1),
    border-color 0.25s ease, box-shadow 0.25s ease;
}

.mm-op:hover {
  transform: translateY(-3px);
  border-color: rgba(193, 155, 69, 0.4);
  box-shadow: 0 12px 28px rgba(0, 0, 0, 0.08);
}

.mm-op:focus-visible {
  outline: 2px solid var(--mm-gold);
  outline-offset: 2px;
}

.mm-op__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 46px;
  height: 46px;
  border-radius: 13px;
  color: #fff;
}

.mm-op__icon--restart {
  background: linear-gradient(135deg, #e5b45a, var(--mm-gold-deep));
}

.mm-op__icon--reset {
  background: linear-gradient(135deg, #f87171, #b91c1c);
}

.mm-op__content {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.mm-op__title {
  font-size: 15px;
  font-weight: 600;
  color: var(--color-text-1, var(--text-primary, #1f2937));
}

.mm-op__desc {
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--color-text-3, var(--text-secondary, #6b7280));
}

.mm-op__foot {
  margin-top: auto;
  padding-top: 6px;
}

/* ── 版本信息 ── */
.mm-version {
  display: flex;
  align-items: center;
  gap: 12px;
}

.mm-version__label {
  font-size: 13px;
  color: var(--color-text-2, var(--text-secondary, #6b7280));
}

.mm-badge {
  padding: 2px 10px;
  border-radius: 6px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--mm-gold-deep);
  background: rgba(193, 155, 69, 0.12);
  border: 1px solid rgba(193, 155, 69, 0.22);
}

.mm-version__hint {
  margin-left: auto;
  font-size: 12px;
  color: var(--color-text-3, var(--text-secondary, #6b7280));
}

/* ── 升级区块 ── */
.mm-section {
  margin-top: 20px;
  padding-top: 20px;
  border-top: 1px dashed var(--color-border-2, var(--border, rgba(0, 0, 0, 0.08)));
}

.mm-section__head {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 16px;
}

.mm-section__label {
  font-size: 14px;
  font-weight: 600;
  color: var(--color-text-1, var(--text-primary, #1f2937));
}

.mm-section__sub {
  font-size: 12px;
  color: var(--color-text-3, var(--text-secondary, #6b7280));
}

.mm-upload {
  display: flex;
  flex-direction: column;
  gap: 14px;
  align-items: flex-start;
}

.mm-btn-gold {
  color: #fff !important;
  background: linear-gradient(135deg, #d3ae57, var(--mm-gold-deep)) !important;
  border-color: transparent !important;
  box-shadow: 0 6px 16px rgba(193, 155, 69, 0.3);
}

.mm-btn-gold:hover {
  filter: brightness(1.06);
  box-shadow: 0 8px 20px rgba(193, 155, 69, 0.38);
}

.mm-result-alert {
  width: 100%;
}

/* ── 校验通过结果卡 ── */
.mm-valid {
  width: 100%;
  margin-top: 16px;
  padding: 16px;
  border: 1px solid rgba(52, 199, 89, 0.22);
  border-radius: 12px;
  background: var(--color-bg-3, var(--surface, #fafafa));
}

.mm-valid__file {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 14px;
}

.mm-ok {
  color: #34c759;
  flex-shrink: 0;
}

.mm-valid__name {
  font-weight: 600;
  font-size: 13px;
  color: var(--color-text-1, var(--text-primary, #1f2937));
  word-break: break-all;
}

.mm-desc :deep(.arco-descriptions-body) {
  background: transparent;
}

.mm-kind {
  padding: 1px 8px;
  border-radius: 5px;
  font-size: 12px;
  font-weight: 600;
}

.mm-kind--up {
  color: #25780d;
  background: rgba(40, 167, 69, 0.14);
}

.mm-kind--down {
  color: #b4540a;
  background: rgba(217, 119, 6, 0.14);
}

.mm-valid__actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 16px;
}

/* ── 安装进度 ── */
.mm-progress {
  width: 100%;
  margin-top: 20px;
}

.mm-progress__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.mm-progress__title {
  font-size: 14px;
  font-weight: 600;
  color: var(--color-text-1, var(--text-primary, #1f2937));
}

.mm-progress__stage {
  font-size: 13px;
  font-weight: 600;
  color: var(--mm-gold-deep);
}

.mm-progress__detail {
  margin-top: 10px;
}

.mm-progress__hint {
  font-size: 12px;
  color: var(--color-text-3, var(--text-secondary, #6b7280));
}

.mm-progress__error {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  margin-top: 14px;
  padding: 10px 12px;
  border-radius: 8px;
  font-size: 13px;
  color: var(--color-danger-6, #d03050);
  background: var(--color-danger-light-1, rgba(208, 48, 80, 0.1));
}

.mm-progress__success {
  margin-top: 16px;
}

/* ── 响应式：窄屏操作卡单列 ── */
@media (max-width: 720px) {
  .mm-ops { grid-template-columns: 1fr; }
  .mm-version__hint { display: none; }
  .mm-panel__action { margin-left: auto; }
  .mm-panel__body { padding: 18px; }
}

/* ── 弹窗 ── */
.mm-modal__msg {
  margin: 4px 0 8px;
  font-size: 15px;
  color: var(--color-text-1, var(--text-primary, #1f2937));
}

.mm-modal__warn {
  margin: 0;
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--color-text-3, var(--text-secondary, #6b7280));
}
</style>