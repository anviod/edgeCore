<template>
  <div class="database-management settings-stack">
    <a-card title="数据库概览" :bordered="false" class="settings-panel">
      <div class="overview-grid">
        <div class="overview-item config-db">
          <div class="overview-header">
            <span class="db-type">配置库 (config.db)</span>
            <a-tag color="red" size="small">受保护 · 不可清理</a-tag>
          </div>
          <div class="overview-fields-row">
            <div class="overview-field">
              <div class="overview-label">数据库路径</div>
              <div class="overview-value">{{ stats.config_db?.path || '-' }}</div>
            </div>
            <div class="overview-field">
              <div class="overview-label">配置 Bucket 数</div>
              <div class="overview-value">{{ configBucketCount }} 个</div>
            </div>
          </div>
        </div>
        <div class="overview-item runtime-db">
          <div class="overview-header">
            <span class="db-type">运行时库 (runtime.db)</span>
            <a-tag color="green" size="small">可清理 · 可压缩</a-tag>
          </div>
          <div class="overview-fields-row">
            <div class="overview-field">
              <div class="overview-label">数据库路径</div>
              <div class="overview-value">{{ stats.runtime_db?.path || '-' }}</div>
            </div>
            <div class="overview-field">
              <div class="overview-label">运行时 Bucket 数</div>
              <div class="overview-value">{{ runtimeBucketCount }} 个</div>
            </div>
          </div>
        </div>
        <div class="overview-item overview-item--stats">
          <div class="overview-field">
            <div class="overview-label">总大小</div>
            <div class="overview-value">{{ formatSize(stats.total_size) }}</div>
          </div>
          <div class="overview-field">
            <div class="overview-label">可清理数据大小</div>
            <div class="overview-value text-warning">{{ formatSize(clearableSize) }}</div>
          </div>
        </div>
      </div>
    </a-card>

    <a-card title="配置库管理" :bordered="false" class="settings-panel">
      <template #extra>
        <a-tag color="red" size="small">优先备份</a-tag>
      </template>
      <a-alert type="info" show-icon :closable="false" class="mb-block">
        配置库包含设备配置、通道、点位、用户、系统设置等关键数据。
        <strong>配置库不可清理</strong>，建议定期备份或导出。
      </a-alert>

      <a-collapse :bordered="false" class="help-collapse mb-block">
        <a-collapse-item header="导入/导出说明" key="import-export-help">
          <div class="help-content">
            <p><strong>导出配置数据库</strong>：将 <code>config.db</code> 打包为 <code>.tar.gz</code> 下载，包含通道、设备、点位、北向、边缘规则、系统设置等。</p>
            <p><strong>导出运行时数据</strong>：将 <code>runtime.db</code> 打包为 <code>.tar.gz</code> 下载，包含实时值快照、缓存、历史数据等（不含配置）。</p>
            <p><strong>导入配置数据库</strong>：上传此前导出的配置库 <code>.tar.gz</code> 文件，将覆盖当前网关的采集与业务配置。</p>
            <p><strong>强制拉取远程覆盖本地</strong>：从另一台 edgeCore 网关 HTTP 拉取其配置库并<strong>完全覆盖</strong>本机（含用户密码与端口）。</p>
            <ul>
              <li>普通导入时<strong>不会覆盖</strong>本机已有<strong>用户账号与密码</strong>，避免导入后无法登录。</li>
              <li>普通导入时<strong>不会覆盖</strong>本机当前<strong>服务器端口</strong>，避免 Web 服务端口冲突导致无法访问。</li>
              <li>勾选「强制覆盖」或执行远程拉取时，将覆盖用户账号/密码与服务器端口，请谨慎操作。</li>
              <li>远程拉取需目标网关可访问，且提供有效登录 Token（若目标启用了鉴权）。</li>
              <li>导入或拉取完成后建议<strong>重启网关服务</strong>，使驱动与采集任务加载新配置。</li>
              <li>操作前建议先执行「备份配置库」或「导出配置数据库」，以便回滚。</li>
            </ul>
          </div>
        </a-collapse-item>
      </a-collapse>

      <div class="action-group">
        <a-button type="primary" status="success" @click="handleExportConfigDB" :loading="exportConfigLoading">
          <template #icon><icon-download /></template>
          导出配置数据库
        </a-button>
        <a-button type="outline" status="success" @click="handleBackupConfig" :loading="backupLoading">
          <template #icon><icon-save /></template>
          备份到本地目录
        </a-button>
        <a-upload
          :show-file-list="false"
          accept=".tar.gz,.tgz"
          :custom-request="handleImportConfigDB"
          :disabled="importConfigLoading"
        >
          <a-button type="outline" status="warning" :loading="importConfigLoading">
            <template #icon><icon-upload /></template>
            导入配置数据库
          </a-button>
        </a-upload>
        <a-checkbox v-model="importForceOverwrite" class="force-overwrite-check">
          强制覆盖（含用户密码与端口）
        </a-checkbox>
        <a-button type="outline" @click="handleRefresh" :loading="loading">
          <template #icon><icon-refresh /></template>
          刷新统计
        </a-button>
      </div>
      <div v-if="backupResult" class="backup-result">
        <a-alert type="success" show-icon :closable="true" @close="backupResult = null">
          <template #title>备份成功</template>
          {{ backupResult.message }}<br />
          备份文件：{{ backupResult.backup_path }}<br />
          文件大小：{{ backupResult.size_display }}
        </a-alert>
      </div>
      <div v-if="importResult" class="import-result">
        <a-alert type="success" show-icon :closable="true" @close="importResult = null">
          <template #title>导入成功</template>
          {{ importResult.message }}<br />
          已导入 {{ importResult.channel_count }} 个通道、{{ importResult.device_count }} 个设备配置<br />
          <span v-if="importResult.remote_source">远程来源：{{ importResult.remote_source }}<br /></span>
          服务器端口：{{ importResult.preserved_port }}
        </a-alert>
      </div>

      <a-divider orientation="left" class="section-divider">远程配置拉取</a-divider>
      <a-alert type="warning" show-icon :closable="false" class="mb-block">
        从远程 edgeCore 网关拉取配置库并<strong>强制覆盖</strong>本机全部配置（含用户账号/密码与服务器端口）。请确保目标地址正确，操作前请先备份。
      </a-alert>
      <div class="remote-pull-form">
        <a-input v-model="remotePullForm.host" placeholder="远程网关 IP 或域名" allow-clear />
        <a-input-number v-model="remotePullForm.port" :min="1" :max="65535" placeholder="端口" />
        <a-input-password v-model="remotePullForm.token" placeholder="远程 Token（可选）" allow-clear />
        <a-checkbox v-model="remotePullForm.useHttps">使用 HTTPS</a-checkbox>
        <a-button
          type="primary"
          status="danger"
          :loading="remotePullLoading"
          @click="openRemotePullConfirm"
        >
          <template #icon><icon-download /></template>
          强制拉取远程覆盖本地
        </a-button>
      </div>
    </a-card>

    <a-card title="运行时库管理" :bordered="false" class="settings-panel">
      <template #extra>
        <a-tag color="orange" size="small">可清理 · 可压缩 · 可重建</a-tag>
      </template>
      <a-alert type="warning" show-icon :closable="false" class="mb-block">
        运行时库包含实时值、缓存、历史数据、边缘计算日志、WAL 等。清理运行时数据
        <strong>不影响采集配置</strong>，但会丢失历史记录、边缘规则日志与缓存。
      </a-alert>

      <div class="table-wrap">
        <a-table
          :columns="columns"
          :data="runtimeBuckets"
          :pagination="pagination"
          row-key="name"
          :loading="loading"
          size="small"
          :bordered="false"
        >
          <template #category="{ record }">
            <a-tag :color="getCategoryColor(record.category)" size="small">
              {{ getCategoryLabel(record.category) }}
            </a-tag>
          </template>
          <template #total_size="{ record }">
            {{ formatSize(record.total_size) }}
          </template>
          <template #clearable="{ record }">
            <a-badge v-if="record.clearable" status="success" text="可清理" />
            <a-badge v-else status="danger" text="受保护" />
          </template>
        </a-table>
      </div>

      <div class="action-group">
        <a-button type="primary" status="warning" @click="handleExportRuntimeDB" :loading="exportRuntimeLoading">
          <template #icon><icon-download /></template>
          导出运行时数据
        </a-button>
        <a-button type="primary" status="warning" @click="handleClearCache" :loading="loading">
          <template #icon><icon-delete /></template>
          清空运行缓存
        </a-button>
        <a-button type="outline" status="warning" @click="handleClearRuntime" :loading="loading" :disabled="!hasRuntime">
          <template #icon><icon-delete /></template>
          清空实时值快照
        </a-button>
        <a-button type="outline" status="warning" @click="handleClearHistory" :loading="loading" :disabled="!hasHistory">
          <template #icon><icon-delete /></template>
          清空历史数据
        </a-button>
      </div>

      <a-divider orientation="left" class="section-divider">高级操作</a-divider>
      <div class="action-group">
        <a-button type="outline" @click="handleCompactRuntime" :loading="compactLoading">
          <template #icon><icon-shrink /></template>
          压缩运行时库
        </a-button>
      </div>
      <div class="action-desc">
        <a-alert type="info" show-icon :closable="false">
          <strong>压缩运行时库</strong>：回收已删除数据占用的磁盘空间，配置库不受影响。清空运行缓存/历史/实时值等操作会自动压缩 runtime.db；也可在此手动执行。
        </a-alert>
      </div>

      <div v-if="compactResult" class="compact-result">
        <a-alert type="success" show-icon :closable="true" @close="compactResult = null">
          <template #title>压缩完成</template>
          {{ compactResult.message }}<br />
          压缩前：{{ compactResult.before_size }} → 压缩后：{{ compactResult.after_size }}<br />
          节省空间：{{ compactResult.saved_size }}
        </a-alert>
      </div>
    </a-card>

    <a-card title="配置库 Bucket 详情（只读）" :bordered="false" class="settings-panel">
      <template #extra>
        <a-tag color="red" size="small">受保护</a-tag>
      </template>
      <div class="table-wrap">
        <a-table
          :columns="columns"
          :data="configBuckets"
          :pagination="false"
          row-key="name"
          :loading="loading"
          size="small"
          :bordered="false"
        >
          <template #category>
            <a-tag color="red" size="small">配置</a-tag>
          </template>
          <template #total_size="{ record }">
            {{ formatSize(record.total_size) }}
          </template>
          <template #clearable>
            <a-badge status="danger" text="不可清理" />
          </template>
        </a-table>
      </div>
    </a-card>

    <a-card title="一次性迁移工具" :bordered="false" class="settings-panel">
      <template #extra>
        <a-tag color="arcoblue" size="small">edgeCore.db → config.db + runtime.db</a-tag>
      </template>
      <a-alert type="info" show-icon :closable="false" class="mb-block">
        将旧版 <code>data/edgeCore.db</code> 中的配置 bucket 迁移到 <code>data/config.db</code>，
        运行时 bucket 迁移到 <code>data/runtime.db</code>。启动时已自动执行，此工具用于手动迁移。
      </a-alert>
      <div class="action-group">
        <a-button type="outline" @click="handleMigrateLegacy" :loading="migrateLoading">
          <template #icon><icon-swap /></template>
          执行迁移
        </a-button>
      </div>
      <div v-if="migrateResult" class="migrate-result">
        <a-alert
          :type="migrateResult.status === 'success' ? 'success' : 'error'"
          show-icon
          :closable="true"
          @close="migrateResult = null"
        >
          <template #title>迁移结果</template>
          {{ migrateResult.message }}<br />
          <span v-if="migrateResult.migrated?.length">
            已迁移：{{ migrateResult.migrated.join(', ') }}
          </span>
          <span v-if="migrateResult.skipped?.length" class="ml-2">
            跳过：{{ migrateResult.skipped.join(', ') }}
          </span>
        </a-alert>
      </div>
    </a-card>

    <a-modal
      v-model:visible="confirmVisible"
      :title="confirmTitle"
      @ok="handleConfirm"
      @cancel="confirmVisible = false"
      ok-text="确认"
      cancel-text="取消"
      :ok-button-props="{ status: 'danger' }"
    >
      <p>{{ confirmMessage }}</p>
      <div v-if="confirmBuckets.length > 0" class="confirm-buckets">
        <a-tag v-for="b in confirmBuckets" :key="b" color="red" size="small">
          {{ b }}
        </a-tag>
      </div>
      <a-checkbox v-model="confirmChecked" class="mt-3">
        我已了解此操作的影响
      </a-checkbox>
    </a-modal>

    <a-modal
      v-model:visible="remotePullConfirmVisible"
      title="确认强制拉取远程配置"
      @ok="handlePullRemoteConfig"
      @cancel="remotePullConfirmVisible = false"
      ok-text="确认拉取"
      cancel-text="取消"
      :ok-button-props="{ status: 'danger' }"
    >
      <p>
        将从 <strong>{{ remotePullForm.host }}:{{ remotePullForm.port || 8080 }}</strong>
        拉取配置并<strong>完全覆盖</strong>本机，包括用户账号/密码与服务器端口。
      </p>
      <p>此操作不可自动撤销，请确认已备份本机配置。</p>
      <a-checkbox v-model="remotePullConfirmChecked" class="mt-3">
        我已了解此操作的影响
      </a-checkbox>
    </a-modal>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import axios from 'axios'
import { Message } from '@arco-design/web-vue'
import { IconRefresh, IconDelete, IconDownload, IconShrink, IconSwap, IconUpload, IconSave } from '@arco-design/web-vue/es/icon'
import request from '../../utils/request'
import { downloadBytes } from '../../utils/decode'

const stats = ref({})
const loading = ref(false)
const backupLoading = ref(false)
const exportConfigLoading = ref(false)
const exportRuntimeLoading = ref(false)
const importConfigLoading = ref(false)
const remotePullLoading = ref(false)
const importForceOverwrite = ref(false)
const compactLoading = ref(false)
const migrateLoading = ref(false)
const backupResult = ref(null)
const importResult = ref(null)
const compactResult = ref(null)
const migrateResult = ref(null)

const confirmVisible = ref(false)
const confirmTitle = ref('')
const confirmMessage = ref('')
const confirmBuckets = ref([])
const confirmMode = ref('')
const confirmChecked = ref(false)

const remotePullConfirmVisible = ref(false)
const remotePullConfirmChecked = ref(false)
const remotePullForm = ref({
  host: '',
  port: 8080,
  token: '',
  useHttps: false,
})

const pagination = false

const columns = [
  { title: '名称', dataIndex: 'name', width: 200, ellipsis: true, tooltip: true },
  { title: '分类', dataIndex: 'category', slotName: 'category', width: 100 },
  { title: '记录数', dataIndex: 'record_count', width: 100 },
  { title: '大小', dataIndex: 'total_size', slotName: 'total_size', width: 120 },
  { title: '状态', dataIndex: 'clearable', slotName: 'clearable', width: 100 },
]

const configBuckets = computed(() => {
  return (stats.value.buckets || []).filter(b => b.database === 'config')
})

const runtimeBuckets = computed(() => {
  return (stats.value.buckets || []).filter(b => b.database === 'runtime')
})

const configBucketCount = computed(() => configBuckets.value.length)

const runtimeBucketCount = computed(() => runtimeBuckets.value.length)

const clearableSize = computed(() => {
  return (stats.value.buckets || []).reduce((sum, bucket) => {
    if (bucket.clearable) return sum + bucket.total_size
    return sum
  }, 0)
})

const hasRuntime = computed(() => {
  return (stats.value.buckets || []).some(b => b.name === 'values' || b.name === 'shadow_values')
})

const hasHistory = computed(() => {
  return (stats.value.buckets || []).some(b => b.category === 'history')
})

const formatSize = (bytes) => {
  const n = Number(bytes)
  if (!n || n <= 0) return '0 MB'
  const mb = n / (1024 * 1024)
  const digits = mb < 0.01 ? 4 : 2
  return `${mb.toFixed(digits)} MB`
}

const getCategoryColor = (category) => {
  const colors = {
    config: 'red',
    cache: 'blue',
    runtime: 'orange',
    history: 'purple',
    legacy: 'gray',
    edge_log: 'cyan',
    unknown: 'gray',
  }
  return colors[category] || 'gray'
}

const getCategoryLabel = (category) => {
  const labels = {
    config: '配置',
    cache: '缓存',
    runtime: '运行时',
    history: '历史',
    legacy: '遗留',
    edge_log: '边缘日志',
    unknown: '未知',
  }
  return labels[category] || category
}

const showMessage = (content, type = 'info') => {
  const msgMap = {
    success: 'success',
    error: 'error',
    warning: 'warning',
    info: 'info',
  }
  Message[msgMap[type] || 'info']({ content })
}

const getAuthHeaders = () => {
  const headers = {}
  try {
    const raw = localStorage.getItem('loginInfo')
    if (raw) {
      const parsed = JSON.parse(raw)
      const token = parsed.token || (parsed.data && parsed.data.token) || ''
      if (token) {
        headers.token = token
        headers.Authorization = `Bearer ${token}`
      }
    }
  } catch (e) {
    console.error('Failed to get token', e)
  }
  return headers
}

const parseFilename = (contentDisposition, fallback) => {
  if (!contentDisposition) return fallback
  const match = /filename="?([^";]+)"?/i.exec(contentDisposition)
  return match?.[1] || fallback
}

const downloadArchive = async (url, fallbackFilename) => {
  const res = await axios.get(url, {
    responseType: 'blob',
    headers: getAuthHeaders(),
  })
  const filename = parseFilename(res.headers['content-disposition'], fallbackFilename)
  downloadBytes(res.data, filename)
}

const fetchStats = async () => {
  loading.value = true
  try {
    const res = await request.get('/api/data/stats')
    stats.value = res
  } catch (error) {
    console.error('Failed to fetch stats:', error)
    showMessage('获取统计信息失败', 'error')
  } finally {
    loading.value = false
  }
}

const handleRefresh = () => {
  fetchStats()
}

const handleBackupConfig = async () => {
  backupLoading.value = true
  backupResult.value = null
  try {
    const res = await request.post('/api/data/backup-config')
    backupResult.value = res
    showMessage('配置库备份成功', 'success')
  } catch (error) {
    console.error('Backup config failed:', error)
    showMessage(error?.response?.data?.error || '备份失败', 'error')
  } finally {
    backupLoading.value = false
  }
}

const handleExportConfigDB = async () => {
  exportConfigLoading.value = true
  try {
    await downloadArchive('/api/data/export-config-db', `edgeCore-config-${Date.now()}.tar.gz`)
    showMessage('配置库导出成功', 'success')
  } catch (error) {
    console.error('Export config db failed:', error)
    showMessage(error?.response?.data?.error || '导出失败', 'error')
  } finally {
    exportConfigLoading.value = false
  }
}

const handleExportRuntimeDB = async () => {
  exportRuntimeLoading.value = true
  try {
    await downloadArchive('/api/data/export-runtime-db', `edgeCore-runtime-${Date.now()}.tar.gz`)
    showMessage('运行时数据导出成功', 'success')
  } catch (error) {
    console.error('Export runtime db failed:', error)
    showMessage(error?.response?.data?.error || '导出失败', 'error')
  } finally {
    exportRuntimeLoading.value = false
  }
}

const handleImportConfigDB = async (option) => {
  const file = option?.fileItem?.file
  if (!file) {
    showMessage('请选择 .tar.gz 配置文件', 'warning')
    return
  }

  importConfigLoading.value = true
  importResult.value = null
  try {
    const formData = new FormData()
    formData.append('file', file)
    formData.append('force_overwrite', importForceOverwrite.value ? 'true' : 'false')
    const res = await axios.post('/api/data/import-config-db', formData, {
      headers: {
        ...getAuthHeaders(),
        'Content-Type': 'multipart/form-data',
      },
    })
    importResult.value = res.data
    showMessage('配置库导入成功', 'success')
    await fetchStats()
    option.onSuccess?.(res.data)
  } catch (error) {
    console.error('Import config db failed:', error)
    const msg = error?.response?.data?.error || '导入失败'
    showMessage(msg, 'error')
    option.onError?.(error)
  } finally {
    importConfigLoading.value = false
  }
}

const openRemotePullConfirm = () => {
  if (!remotePullForm.value.host?.trim()) {
    showMessage('请输入远程网关地址', 'warning')
    return
  }
  remotePullConfirmChecked.value = false
  remotePullConfirmVisible.value = true
}

const handlePullRemoteConfig = async () => {
  if (!remotePullConfirmChecked.value) {
    showMessage('请先勾选「我已了解此操作的影响」', 'warning')
    return
  }
  remotePullConfirmVisible.value = false
  remotePullLoading.value = true
  importResult.value = null
  try {
    const res = await request.post('/api/data/pull-remote-config', {
      host: remotePullForm.value.host.trim(),
      port: remotePullForm.value.port || 8080,
      token: remotePullForm.value.token || '',
      use_https: remotePullForm.value.useHttps,
    })
    importResult.value = res
    showMessage('远程配置拉取成功', 'success')
    await fetchStats()
  } catch (error) {
    console.error('Pull remote config failed:', error)
    showMessage(error?.response?.data?.error || '远程拉取失败', 'error')
  } finally {
    remotePullLoading.value = false
  }
}

const handleClearCache = () => {
  confirmTitle.value = '确认清空运行缓存'
  confirmMessage.value = '将清理 runtime.db 中的全部运行时数据（含 values、各类缓存、历史 bucket、边缘计算日志 edge_events/edge_failures/bblot、遗留 WAL 等），并清空内存中的影子设备、边缘规则运行时状态与日志缓冲。采集配置不受影响；实时值将在下一轮采集后恢复。'
  confirmBuckets.value = ['runtime.db 全部 bucket（含边缘计算日志）']
  confirmMode.value = 'all_runtime'
  confirmChecked.value = false
  confirmVisible.value = true
}

const handleClearRuntime = () => {
  confirmTitle.value = '确认清空实时值快照'
  confirmMessage.value = '将清理 values bucket 中的遗留缓存（当前实时值以内存影子设备为准）。清空后 UI 仍由影子设备推送，无需等待落库。采集配置不会受影响。'
  confirmBuckets.value = ['values']
  confirmMode.value = 'runtime'
  confirmChecked.value = false
  confirmVisible.value = true
}

const handleClearHistory = () => {
  const historyBuckets = (stats.value.buckets || []).filter(b => b.category === 'history').map(b => b.name)
  confirmTitle.value = '确认清空历史数据'
  confirmMessage.value = `将清理 ${historyBuckets.length} 个历史数据 bucket。不影响采集配置。`
  confirmBuckets.value = historyBuckets
  confirmMode.value = 'history'
  confirmChecked.value = false
  confirmVisible.value = true
}

const handleConfirm = async () => {
  if (!confirmChecked.value) {
    showMessage('请先勾选「我已了解此操作的影响」', 'warning')
    return
  }
  confirmVisible.value = false
  loading.value = true
  try {
    let res
    if (confirmMode.value === 'all_runtime') {
      res = await request.post('/api/data/clear-all-runtime')
    } else {
      res = await request.post('/api/data/clear-cache', { mode: confirmMode.value })
    }
    if (res.status === 'success') {
      let msg = `成功清理 ${res.cleared?.length || 0} 个 bucket`
      if (res.compact?.saved_size && res.compact.saved_size !== '0 MB') {
        msg += `，runtime.db 已压缩（节省 ${res.compact.saved_size}）`
      } else if (res.compact) {
        msg += '，runtime.db 已压缩'
      }
      showMessage(msg, 'success')
      await fetchStats()
    } else {
      showMessage('清理失败', 'error')
    }
  } catch (error) {
    console.error('Clear failed:', error)
    showMessage(error?.response?.data?.error || '清理失败', 'error')
  } finally {
    loading.value = false
  }
}

const handleCompactRuntime = async () => {
  compactLoading.value = true
  compactResult.value = null
  try {
    const res = await request.post('/api/data/compact-runtime')
    compactResult.value = res
    showMessage('运行时库压缩成功', 'success')
    await fetchStats()
  } catch (error) {
    console.error('Compact failed:', error)
    showMessage(error?.response?.data?.error || '压缩失败', 'error')
  } finally {
    compactLoading.value = false
  }
}

const handleMigrateLegacy = async () => {
  migrateLoading.value = true
  migrateResult.value = null
  try {
    const res = await request.post('/api/data/migrate-legacy', { legacy_path: 'data/edgeCore.db' })
    migrateResult.value = res
    showMessage('迁移完成', 'success')
    await fetchStats()
  } catch (error) {
    console.error('Migrate failed:', error)
    migrateResult.value = error?.response?.data || { status: 'error', message: '迁移失败' }
    showMessage(error?.response?.data?.error || '迁移失败', 'error')
  } finally {
    migrateLoading.value = false
  }
}

onMounted(() => {
  fetchStats()
})
</script>

<style scoped>
/* v3.0 — styles in src/styles/system-settings.css */
.help-content p {
  margin: 0 0 8px;
}
.help-content ul {
  margin: 0;
  padding-left: 20px;
}
.help-content li {
  margin-bottom: 4px;
}
.backup-result,
.import-result,
.compact-result,
.migrate-result {
  margin-top: 12px;
}
.remote-pull-form {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
}
.remote-pull-form .arco-input-wrapper,
.remote-pull-form .arco-input-number {
  width: 220px;
}
.force-overwrite-check {
  margin-left: 4px;
}
</style>
