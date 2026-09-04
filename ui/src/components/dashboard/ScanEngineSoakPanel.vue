<template>
  <div class="soak-panel">
    <div class="soak-panel__header">
      <div class="soak-panel__heading">
        <div class="soak-panel__title-row">
          <h3 class="soak-panel__title">系统概览</h3>
          <a-button
            type="text"
            size="mini"
            class="help-trigger-btn soak-panel__help-btn"
            aria-label="Soak 监控帮助"
            @click="openGeneralHelp"
          >
            <template #icon><IconQuestionCircle /></template>
          </a-button>
        </div>
        <p class="soak-panel__subtitle">ScanEngine 运行监控 · SLA / Soak · Release Gate</p>
      </div>
      <div class="soak-panel__meta">
        <span class="dashboard-status-chip is-live">
          <span class="status-dot"></span>
          实时
        </span>
        <div class="soak-panel__status-strip">
          <span class="dashboard-status-chip is-online">
            <span class="status-dot"></span>
            在线 {{ onlineDevices }}
          </span>
          <span class="dashboard-status-chip is-offline">
            <span class="status-dot"></span>
            离线 {{ offlineDevices }}
          </span>
          <span class="dashboard-status-chip is-neutral">
            <span class="status-dot"></span>
            {{ channelCount }} 通道
          </span>
        </div>
        <div class="soak-panel__session">
          <span v-if="uptimeDisplay">运行时长 {{ uptimeDisplay }}</span>
        </div>
      </div>
    </div>

    <div v-if="loading && !hasData" class="soak-panel__loading">
      <a-spin tip="加载 SLA 监控..." />
    </div>

    <div v-else-if="fetchError && !hasData" class="soak-panel__error">
      <span>{{ fetchError }}</span>
      <a-button size="mini" @click="retryFetch">重试</a-button>
    </div>

    <template v-else>
      <div class="soak-scan-classes soak-card">
        <div class="soak-scan-classes__header">
          <div class="soak-scan-classes__heading">
            <div class="soak-scan-classes__title-row">
              <h4 class="soak-card__title">Scan Class 明细</h4>
              <a-button
                type="text"
                size="mini"
                class="help-trigger-btn soak-scan-classes__help-btn"
                aria-label="Scan Class 明细帮助"
                @click="openScanClassHelp"
              >
                <template #icon><IconQuestionCircle /></template>
              </a-button>
            </div>
            <p class="soak-scan-classes__subtitle">
              按扫描间隔分组
              <span v-if="scanClasses.length">· {{ scanClasses.length }} 种间隔</span>
            </p>
          </div>
          <div class="soak-scan-classes__actions">
            <span
              v-if="snapshot.scan_class_late > 0"
              class="soak-scan-classes__alert"
            >
              {{ snapshot.scan_class_late }} 迟到
            </span>
            <button
              v-if="scanClasses.length"
              class="soak-collapse-toggle"
              :aria-expanded="!effectiveScanCollapsed"
              aria-controls="scan-class-list"
              @click="scanCollapsed = !effectiveScanCollapsed"
            >
              {{ effectiveScanCollapsed ? '展开' : '收起' }}
              <icon-down v-if="effectiveScanCollapsed" :size="12" />
              <icon-up v-else :size="12" />
            </button>
          </div>
        </div>

        <div v-if="scanClasses.length" id="scan-class-list" class="soak-scan-class-grid" :class="{ 'is-collapsed': effectiveScanCollapsed }">
          <div class="soak-scan-class-grid__head" aria-hidden="true">
            <span>周期</span>
            <span>任务</span>
            <span>积压</span>
            <span>队列</span>
            <span>迟到</span>
            <span>成功率</span>
          </div>
          <div
            v-for="row in scanClasses"
            :key="row.class"
            class="soak-scan-class-row"
            :class="scanClassRowClass(row)"
          >
            <span class="soak-scan-class-row__period">{{ formatScanClassPeriod(row.class) }}</span>
            <span class="soak-scan-class-metric">
              <span class="soak-scan-class-metric__value">{{ row.tasks }}</span>
            </span>
            <span
              class="soak-scan-class-metric"
              :class="{ 'is-warn': row.backlog > 0 }"
            >
              <span class="soak-scan-class-metric__value">{{ row.backlog }}</span>
            </span>
            <span class="soak-scan-class-metric">
              <span class="soak-scan-class-metric__value">{{ row.queue }}</span>
            </span>
            <span
              class="soak-scan-class-metric"
              :class="{ 'is-fail': row.late > 0 }"
            >
              <span class="soak-scan-class-metric__value">{{ row.late }}</span>
            </span>
            <span
              class="soak-scan-class-metric"
              :class="successMetricClass(row.success)"
            >
              <span class="soak-scan-class-metric__value">{{ formatRate(row.success) }}</span>
            </span>
          </div>
        </div>

        <div v-else class="soak-scan-classes__empty">暂无 Scan Class 数据</div>
      </div>

      <!-- Release Gate hero — scannable pass/fail -->
      <div class="soak-hero">
        <div class="soak-gate-summary" :class="gateSummaryClass">
          <div class="soak-gate-summary__main">
            <span class="soak-gate-summary__icon">{{ releaseGate.all_passed !== false ? '✓' : '✗' }}</span>
            <div>
              <span class="soak-gate-summary__label">Release Gate</span>
              <span class="soak-gate-summary__status">{{ gateSummaryText }}</span>
            </div>
          </div>
          <div class="soak-gate-summary__counts" v-if="releaseGateItems.length">
            <span class="soak-gate-count is-pass">{{ passCount }} 达标</span>
            <span class="soak-gate-count is-fail" v-if="failCount">{{ failCount }} 未达标</span>
            <button
              class="soak-collapse-toggle"
              :aria-expanded="!effectiveGateCollapsed"
              aria-controls="gate-list"
              @click="gateCollapsed = !effectiveGateCollapsed"
            >
              {{ effectiveGateCollapsed ? '展开' : '收起' }} 明细
              <icon-down v-if="effectiveGateCollapsed" :size="12" />
              <icon-up v-else :size="12" />
            </button>
          </div>
        </div>

        <div id="gate-list" class="soak-gate-list" :class="{ 'is-collapsed': effectiveGateCollapsed }">
          <div
            v-for="item in releaseGateItems"
            :key="item.id"
            class="soak-gate-item"
            :class="{ 'is-fail': !item.passed, 'is-warn': item.warning }"
          >
            <span class="soak-gate-item__icon">{{ item.passed ? '✓' : '✗' }}</span>
            <div class="soak-gate-item__body">
              <div class="soak-gate-item__label">{{ item.label }}</div>
              <div class="soak-gate-item__detail">{{ item.detail }}</div>
            </div>
          </div>
        </div>
      </div>

      <!-- Metrics snapshot row -->
      <div class="soak-card soak-card--merged">
        <div class="metric-head">
          <h4 class="soak-card__title">运行指标</h4>
          <p class="soak-card__subtitle">当前运行 · 会话峰值 · 迷你趋势</p>
        </div>

        <div class="soak-metric-split">
          <div class="soak-metric-group">
            <div class="soak-metric-group__head">
              <span class="soak-metric-label"><i class="metric-dot metric-dot--ingress"></i>当前</span>
            </div>
            <div class="soak-metric-grid">
              <div class="soak-kv">
                <span class="soak-kv__label">任务数</span>
                <strong class="soak-kv__txt">{{ snapshot.task_count ?? 0 }}</strong>
              </div>
              <div class="soak-kv">
                <span class="soak-kv__label">总积压</span>
                <div class="soak-kv__row">
                  <strong class="soak-kv__txt" :class="(snapshot.total_backlog ?? 0) > 0 ? 'is-warn' : 'is-pass'">{{ snapshot.total_backlog ?? 0 }}</strong>
                  <span v-if="trendMap.total_backlog && trendMap.total_backlog.length" class="soak-inline-spark">
                    <span v-for="(v, i) in trendMap.total_backlog" :key="i" class="soak-inline-spark__bar" :style="{ height: barHeight(v, trendMap.total_backlog) + '%' }" />
                  </span>
                </div>
              </div>
              <div class="soak-kv">
                <span class="soak-kv__label">断路器打开</span>
                <div class="soak-kv__row">
                  <strong class="soak-kv__txt" :class="(snapshot.circuit_breaker_open ?? 0) > 0 ? 'is-fail' : 'is-pass'">{{ snapshot.circuit_breaker_open ?? 0 }}</strong>
                  <span v-if="trendMap.circuit_breaker_open && trendMap.circuit_breaker_open.length" class="soak-inline-spark">
                    <span v-for="(v, i) in trendMap.circuit_breaker_open" :key="i" class="soak-inline-spark__bar" :style="{ height: barHeight(v, trendMap.circuit_breaker_open) + '%' }" />
                  </span>
                </div>
              </div>
              <div class="soak-kv">
                <span class="soak-kv__label">节流状态</span>
                <strong class="soak-kv__txt" :class="throttleTone(snapshot.throttle_status)">{{ snapshot.throttle_status || '正常' }}</strong>
              </div>
              <div class="soak-kv">
                <span class="soak-kv__label">全局队列</span>
                <div class="soak-kv__row">
                  <strong class="soak-kv__txt" :class="globalQueueTone()">{{ snapshot.global_queue ?? 0 }} / {{ snapshot.global_queue_limit ?? 10000 }}</strong>
                  <span v-if="trendMap.global_queue && trendMap.global_queue.length" class="soak-inline-spark">
                    <span v-for="(v, i) in trendMap.global_queue" :key="i" class="soak-inline-spark__bar" :style="{ height: barHeight(v, trendMap.global_queue) + '%' }" />
                  </span>
                </div>
              </div>
              <div class="soak-kv">
                <span class="soak-kv__label">Scan Class 迟到</span>
                <div class="soak-kv__row">
                  <strong class="soak-kv__txt" :class="(snapshot.scan_class_late ?? 0) > 0 ? 'is-fail' : 'is-pass'">{{ snapshot.scan_class_late ?? 0 }}</strong>
                  <span v-if="trendMap.scan_class_late && trendMap.scan_class_late.length" class="soak-inline-spark">
                    <span v-for="(v, i) in trendMap.scan_class_late" :key="i" class="soak-inline-spark__bar" :style="{ height: barHeight(v, trendMap.scan_class_late) + '%' }" />
                  </span>
                </div>
              </div>
            </div>
          </div>

          <div class="soak-metric-group">
            <div class="soak-metric-group__head">
              <span class="soak-metric-label"><i class="metric-dot metric-dot--egress"></i>会话峰值</span>
            </div>
            <div class="soak-metric-grid">
              <div class="soak-kv">
                <span class="soak-kv__label">最大积压</span>
                <strong class="soak-kv__txt" :class="(sessionSummary.max_backlog ?? 0) > 0 ? 'is-warn' : 'is-pass'">{{ sessionSummary.max_backlog ?? 0 }}</strong>
              </div>
              <div class="soak-kv">
                <span class="soak-kv__label">最大断路器打开</span>
                <strong class="soak-kv__txt" :class="(sessionSummary.max_circuit_breaker_open ?? 0) > 0 ? 'is-fail' : 'is-pass'">{{ sessionSummary.max_circuit_breaker_open ?? 0 }}</strong>
              </div>
              <div class="soak-kv">
                <span class="soak-kv__label">曾出现节流</span>
                <strong class="soak-kv__txt" :class="sessionSummary.ever_throttled ? 'is-fail' : 'is-pass'">{{ sessionSummary.ever_throttled ? '是' : '否' }}</strong>
              </div>
              <div class="soak-kv">
                <span class="soak-kv__label">最低点位成功率</span>
                <strong class="soak-kv__txt" :class="minRateTone()">{{ formatRate(sessionSummary.min_point_success_rate) }}</strong>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>

    <ScanEngineSoakHelpDrawer v-model:visible="helpVisible" :focus-section="helpFocusSection" />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { IconQuestionCircle, IconDown, IconUp } from '@arco-design/web-vue/es/icon'
import request from '@/utils/request'
import ScanEngineSoakHelpDrawer from '@/components/dashboard/ScanEngineSoakHelpDrawer.vue'

defineProps({
  onlineDevices: { type: Number, default: 0 },
  offlineDevices: { type: Number, default: 0 },
  channelCount: { type: Number, default: 0 }
})

const loading = ref(true)
const hasData = ref(false)
const fetchError = ref('')
const helpVisible = ref(false)
const helpFocusSection = ref('')

const openGeneralHelp = () => {
  helpFocusSection.value = ''
  helpVisible.value = true
}

const openScanClassHelp = () => {
  helpFocusSection.value = 'scan-class'
  helpVisible.value = true
}

const releaseGate = ref({})
const runtimeStartTime = ref(null)
const uptimeDisplay = ref('')
const snapshot = ref({})
const sessionSummary = ref({})
const trends = ref({})
const scanClasses = ref([])

const releaseGateItems = computed(() => releaseGate.value.items || [])

const passCount = computed(() => releaseGateItems.value.filter(i => i.passed).length)
const failCount = computed(() => releaseGateItems.value.filter(i => !i.passed).length)

// 折叠状态：默认收起，可手动切换
const gateCollapsed = ref(null)
const scanCollapsed = ref(null)
const effectiveGateCollapsed = computed(() => (gateCollapsed.value !== null) ? gateCollapsed.value : true)
const effectiveScanCollapsed = computed(() => (scanCollapsed.value !== null) ? scanCollapsed.value : true)

const gateSummaryClass = computed(() => {
  if (releaseGate.value.all_passed === true) return 'is-pass'
  if (releaseGate.value.partial_failed) return 'is-partial'
  if (!hasData.value) return 'is-unknown'
  return 'is-fail'
})

const gateSummaryText = computed(() => {
  if (releaseGate.value.all_passed === true) return '全部达标'
  if (releaseGate.value.partial_failed) return '部分未达标'
  if (!hasData.value) return '数据未知'
  return '未达标'
})

const trendCards = computed(() => {
  const t = trends.value || {}
  return [
    { key: 'total_backlog', label: '总积压', values: t.total_backlog || [], latest: lastValue(t.total_backlog) },
    { key: 'circuit_breaker_open', label: '断路器打开', values: t.circuit_breaker_open || [], latest: lastValue(t.circuit_breaker_open) },
    { key: 'global_queue', label: '全局队列', values: t.global_queue || [], latest: lastValue(t.global_queue) },
    { key: 'scan_class_late', label: 'Scan Class 迟到', values: t.scan_class_late || [], latest: lastValue(t.scan_class_late) }
  ]
})

const trendMap = computed(() => {
  const out = {}
  trendCards.value.forEach(t => { out[t.key] = t.values || [] })
  return out
})

const scanClassRowClass = (row) => {
  if (!row) return ''
  if (row.late > 0) return 'is-fail'
  if (row.backlog > 0 || (row.success != null && row.success < 0.99)) return 'is-warn'
  return 'is-ok'
}

const successMetricClass = (rate) => {
  if (rate == null) return ''
  if (rate < 0.95) return 'is-fail'
  if (rate < 0.99) return 'is-warn'
  return 'is-pass'
}

let timer = null
let uptimeTimer = null

const formatRuntimeDuration = (totalSeconds) => {
  if (totalSeconds < 0) totalSeconds = 0
  const days = Math.floor(totalSeconds / 86400)
  const hours = Math.floor((totalSeconds % 86400) / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const parts = []
  if (days > 0) parts.push(`${days}天`)
  if (hours > 0) parts.push(`${hours}小时`)
  if (minutes > 0 || parts.length === 0) parts.push(`${minutes}分钟`)
  return parts.join('')
}

const updateUptimeDisplay = () => {
  if (!runtimeStartTime.value) {
    uptimeDisplay.value = ''
    return
  }
  const startMs = new Date(runtimeStartTime.value).getTime()
  if (Number.isNaN(startMs)) {
    uptimeDisplay.value = ''
    return
  }
  const seconds = Math.floor((Date.now() - startMs) / 1000)
  uptimeDisplay.value = formatRuntimeDuration(seconds)
}

const lastValue = (arr) => {
  if (!arr || arr.length === 0) return 0
  return arr[arr.length - 1]
}

const barHeight = (value, series) => {
  const max = Math.max(...series, 1)
  return Math.max(4, Math.round((value / max) * 100))
}

const formatRate = (rate) => {
  if (rate === undefined || rate === null) return '-'
  return (rate * 100).toFixed(1) + '%'
}

/* 运行指标语义着色：超阈值时给出警示/异常色 */
const throttleTone = (status) => (status && status !== '正常' ? 'is-warn' : 'is-pass')
const globalQueueTone = () => {
  const q = snapshot.value.global_queue ?? 0
  const lim = snapshot.value.global_queue_limit ?? 10000
  const r = lim > 0 ? q / lim : 0
  return r >= 0.7 ? 'is-warn' : r > 0 ? 'is-pass' : ''
}
const minRateTone = () => {
  const v = sessionSummary.value.min_point_success_rate
  if (v === undefined || v === null) return ''
  if (v < 0.95) return 'is-fail'
  if (v < 0.99) return 'is-warn'
  return 'is-pass'
}

/** 扫描间隔标签：整秒/整毫秒保持整数，小数间隔保留两位 */
const formatScanClassPeriod = (label) => {
  if (label == null || label === '') return '—'
  const text = String(label)
  const m = text.match(/^([\d.]+)(s|ms)$/)
  if (!m) return text
  const num = Number(m[1])
  if (Number.isNaN(num)) return text
  const unit = m[2]
  const rounded = Math.round(num * 100) / 100
  if (Number.isInteger(rounded)) return `${rounded}${unit}`
  return `${rounded.toFixed(2)}${unit}`
}

const retryFetch = () => {
  fetchError.value = ''
  loading.value = true
  fetchSoak()
}

const fetchSoak = async () => {
  try {
    const data = await request.get('/api/diagnostics/soak')
    if (!data) {
      fetchError.value = '暂无监控数据'
      return
    }
    fetchError.value = ''
    runtimeStartTime.value = data.runtime?.start_time || null
    updateUptimeDisplay()
    releaseGate.value = data.release_gate || {}
    snapshot.value = data.snapshot || {}
    sessionSummary.value = data.session_summary || {}
    trends.value = data.trends || {}
    scanClasses.value = data.scan_classes || []
    hasData.value = true
  } catch (e) {
    console.error(e)
    fetchError.value = '监控数据加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchSoak()
  timer = setInterval(fetchSoak, 15000)
  uptimeTimer = setInterval(updateUptimeDisplay, 30000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
  if (uptimeTimer) clearInterval(uptimeTimer)
})
</script>

<style scoped>
/* v3.0 — styles in src/styles/dashboard-soak.css */
</style>
