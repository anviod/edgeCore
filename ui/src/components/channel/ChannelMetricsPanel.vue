<template>
  <div class="channel-metrics-panel">
    <div v-if="loading" class="channel-metrics-panel__loading">
      <a-spin tip="加载监控指标中..." size="large" />
    </div>

    <div v-else-if="error" class="channel-metrics-panel__error">
      <a-alert type="error" :title="error" show-icon />
    </div>

    <div v-else-if="metrics" class="channel-metrics-panel__body">
      <section class="channel-metrics-hero">
        <div class="channel-metrics-score" :class="qualityTier.className">
          <div
            class="channel-metrics-score__ring"
            role="progressbar"
            :aria-valuenow="showScoreRing ? qualityScore : 0"
            aria-valuemin="0"
            aria-valuemax="100"
            :aria-label="`质量评分 ${qualityScoreDisplay}，${qualityTier.label}`"
          >
            <svg class="channel-metrics-score__svg" viewBox="0 0 96 96" aria-hidden="true">
              <defs>
                <linearGradient :id="scoreGradientId" x1="0%" y1="0%" x2="100%" y2="100%">
                  <stop offset="0%" :stop-color="qualityTier.gradientStart" />
                  <stop offset="100%" :stop-color="qualityTier.gradientEnd" />
                </linearGradient>
              </defs>
              <circle
                class="channel-metrics-score__inner"
                cx="48"
                cy="48"
                r="36"
              />
              <circle
                class="channel-metrics-score__track"
                cx="48"
                cy="48"
                :r="scoreRingRadius"
                fill="none"
              />
              <circle
                v-if="showScoreRing"
                class="channel-metrics-score__bar"
                cx="48"
                cy="48"
                :r="scoreRingRadius"
                fill="none"
                :stroke="`url(#${scoreGradientId})`"
                :stroke-dasharray="scoreRingCircumference"
                :stroke-dashoffset="scoreRingOffset"
              />
            </svg>
            <div class="channel-metrics-score__center">
              <span class="channel-metrics-score__value">{{ qualityScoreDisplay }}</span>
              <span class="channel-metrics-score__label">质量评分</span>
            </div>
          </div>
          <span class="channel-metrics-score__badge">{{ qualityTier.label }}</span>
        </div>

        <div class="channel-metrics-meta">
          <div class="channel-metrics-meta__item">
            <span class="channel-metrics-meta__label">通道状态</span>
            <a-badge :status="qualityTier.status" :text="qualityTier.label" />
          </div>
          <div class="channel-metrics-meta__item">
            <span class="channel-metrics-meta__label">连接时长</span>
            <span class="channel-metrics-meta__value">{{ connectionDuration }}</span>
          </div>
          <div class="channel-metrics-meta__item channel-metrics-meta__item--wide">
            <span class="channel-metrics-meta__label">本地地址</span>
            <code class="channel-metrics-meta__mono">{{ network.local }}</code>
          </div>
          <div class="channel-metrics-meta__item channel-metrics-meta__item--wide">
            <span class="channel-metrics-meta__label">目标地址</span>
            <code class="channel-metrics-meta__mono">{{ network.remote }}</code>
          </div>
          <div v-if="metrics.reconnectCount > 0" class="channel-metrics-meta__item">
            <span class="channel-metrics-meta__label">重连次数</span>
            <a-tag color="orange" size="small">{{ metrics.reconnectCount }} 次</a-tag>
          </div>
        </div>
      </section>

      <section class="channel-metrics-detail channel-metrics-transport">
        <h3 class="channel-metrics-detail__title">通信质量</h3>
        <p class="channel-metrics-detail__hint">传输层指标：Modbus/协议 RTT、成功率与丢包，反映链路与报文质量。</p>
        <div class="channel-metrics-metrics-grid">
          <div class="channel-metric-stat">
            <span class="channel-metric-stat__label">成功率</span>
            <span class="channel-metric-stat__value" :class="successRateClass">
              {{ formatMetricsPercent(metrics.successRate, metrics) }}
            </span>
          </div>
          <div class="channel-metric-stat">
            <span class="channel-metric-stat__label">平均 RTT</span>
            <span class="channel-metric-stat__value">{{ formatMetricsDuration(metrics.avgRtt, metrics) }}</span>
          </div>
          <div class="channel-metric-stat">
            <span class="channel-metric-stat__label">丢包率</span>
            <span class="channel-metric-stat__value" :class="packetLossClass">
              {{ formatMetricsPercent(metrics.packetLoss, metrics) }}
            </span>
          </div>
          <div class="channel-metric-stat">
            <span class="channel-metric-stat__label">CRC 错误率</span>
            <span class="channel-metric-stat__value">{{ formatMetricsPercent(metrics.crcErrorRate, metrics) }}</span>
          </div>
          <div class="channel-metric-stat">
            <span class="channel-metric-stat__label">重试率</span>
            <span class="channel-metric-stat__value">{{ formatMetricsPercent(metrics.retryRate, metrics) }}</span>
          </div>
          <div class="channel-metric-stat">
            <span class="channel-metric-stat__label">总请求数</span>
            <span class="channel-metric-stat__value">{{ totalRequests }}</span>
          </div>
          <div class="channel-metric-stat">
            <span class="channel-metric-stat__label">成功请求数</span>
            <span class="channel-metric-stat__value is-success">{{ successCount }}</span>
          </div>
          <div class="channel-metric-stat">
            <span class="channel-metric-stat__label">失败请求数</span>
            <span class="channel-metric-stat__value is-danger">{{ failureCount }}</span>
          </div>
        </div>
      </section>

      <section v-if="hasScanSLA" class="channel-metrics-detail channel-metrics-sla">
        <h3 class="channel-metrics-detail__title">调度 SLA</h3>
        <p class="channel-metrics-detail__hint">ScanEngine 调度指标：采集周期 lag/drift/miss（5 分钟滑动窗口），与本通道任务相关。</p>
        <div class="channel-metrics-detail__grid">
          <div class="channel-metrics-detail__cell">
            <span class="channel-metrics-detail__label">Lag P95</span>
            <span class="channel-metrics-detail__val" :class="lagClass">
              {{ formatLag(scanSLA.scanLagP95Ms) }}
            </span>
          </div>
          <div class="channel-metrics-detail__cell">
            <span class="channel-metrics-detail__label">Drift 均值 (5m)</span>
            <span class="channel-metrics-detail__val" :class="driftClass">
              {{ formatLag(scanSLA.scanDriftAvgMsWindow) }}
            </span>
          </div>
          <div class="channel-metrics-detail__cell">
            <span class="channel-metrics-detail__label">Miss Deadline (5m)</span>
            <span class="channel-metrics-detail__val" :class="missClass">
              {{ scanSLA.scanMissDeadlineWindow ?? 0 }}
            </span>
          </div>
          <div class="channel-metrics-detail__cell">
            <span class="channel-metrics-detail__label">CB Open</span>
            <span class="channel-metrics-detail__val" :class="cbOpenClass">
              {{ scanSLA.circuitBreakerOpen ?? 0 }}
            </span>
          </div>
        </div>
        <a-alert
          v-if="slaWarnings.length"
          type="warning"
          show-icon
          class="channel-metrics-sla__warnings"
          :title="`SLA 告警 (${slaWarnings.length})`"
        >
          <ul class="channel-metrics-sla__warning-list">
            <li v-for="(w, idx) in slaWarnings" :key="idx">{{ w.message || w.code }}</li>
          </ul>
        </a-alert>
      </section>
    </div>

    <a-empty v-else class="channel-metrics-panel__empty" description="暂无监控指标数据" />
  </div>
</template>

<script setup>
import { computed } from 'vue'
import {
  computeQualityScore,
  getQualityTier,
  getQualityScoreDisplay,
  formatMetricsPercent,
  formatMetricsDuration,
  formatConnectionDuration,
  parseNetworkInfo,
  getNetworkConnectionText,
  getSuccessRateClass,
  getPacketLossClass,
  getTotalRequests,
  getSuccessCount,
  getFailureCount,
  hasObservedTraffic,
  isChannelLinkUp,
  getChannelScanSLA,
} from '@/utils/channelMetrics'

const props = defineProps({
  metrics: { type: Object, default: null },
  loading: { type: Boolean, default: false },
  error: { type: String, default: '' },
})

const SCORE_RING_RADIUS = 42
const scoreRingCircumference = 2 * Math.PI * SCORE_RING_RADIUS

const qualityScore = computed(() => computeQualityScore(props.metrics))
const qualityTier = computed(() => getQualityTier(qualityScore.value, props.metrics))
const qualityScoreDisplay = computed(() => getQualityScoreDisplay(qualityScore.value, props.metrics))
const showScoreRing = computed(() => hasObservedTraffic(props.metrics) && isChannelLinkUp(props.metrics))
const scoreRingRadius = SCORE_RING_RADIUS
const scoreGradientId = computed(() => `channel-score-gradient-${qualityTier.value.className}`)
const scoreRingOffset = computed(() => {
  if (!showScoreRing.value) return scoreRingCircumference
  const progress = Math.min(100, Math.max(0, qualityScore.value)) / 100
  return scoreRingCircumference * (1 - progress)
})

const connectionDuration = computed(() => {
  if (!isChannelLinkUp(props.metrics)) {
    return formatConnectionDuration(undefined, props.metrics)
  }
  const text = formatConnectionDuration(props.metrics?.connectionSeconds, props.metrics)
  return text === '未连接' ? text : `已连接 · ${text}`
})

const network = computed(() => {
  const info = parseNetworkInfo(props.metrics)
  return getNetworkConnectionText(props.metrics, info)
})

const totalRequests = computed(() => getTotalRequests(props.metrics))
const successCount = computed(() => getSuccessCount(props.metrics))
const failureCount = computed(() => getFailureCount(props.metrics))

const successRateClass = computed(() => getSuccessRateClass(props.metrics?.successRate, props.metrics))
const packetLossClass = computed(() => getPacketLossClass(props.metrics?.packetLoss, props.metrics))

const scanSLA = computed(() => getChannelScanSLA(props.metrics))
const hasScanSLA = computed(() => scanSLA.value != null)

const slaWarnings = computed(() => {
  const list = scanSLA.value?.slaWarnings
  return Array.isArray(list) ? list : []
})

const lagClass = computed(() => {
  const lag = scanSLA.value?.scanLagP95Ms
  if (lag == null) return ''
  if (lag <= 100) return 'is-success'
  if (lag <= 200) return 'is-warning'
  return 'is-danger'
})

const driftClass = computed(() => {
  const drift = scanSLA.value?.scanDriftAvgMsWindow
  if (drift == null) return ''
  if (drift <= 50) return 'is-success'
  if (drift <= 100) return 'is-warning'
  return 'is-danger'
})

const missClass = computed(() => {
  const miss = scanSLA.value?.scanMissDeadlineWindow
  if (miss == null || miss === 0) return 'is-success'
  return 'is-danger'
})

const cbOpenClass = computed(() => ((scanSLA.value?.circuitBreakerOpen ?? 0) > 0 ? 'is-danger' : 'is-success'))

function formatLag(ms) {
  if (ms == null || ms === undefined) return '—'
  if (ms < 1) return '<1ms'
  return `${Number(ms).toFixed(2)}ms`
}
</script>

<style scoped>
/* v3.0 — src/styles/channel-metrics.css */
</style>
