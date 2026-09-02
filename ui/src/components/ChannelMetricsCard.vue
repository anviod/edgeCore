<template>
  <a-card class="metrics-card-glass" :class="{ 'metrics-card': true, 'expanded': showDetails }">
    <div class="px-4 py-4">
      <!-- 顶部：大质量仪表 + 状态信息 -->
      <div class="flex items-center mb-4">
        <!-- 圆形质量评分（纯 SVG，避免依赖 a-progress 圆形的中部 slot） -->
        <div class="quality-score-wrapper mr-6">
          <div class="quality-ring" :style="ringStyle">
            <svg :width="ringSize" :height="ringSize" :viewBox="`0 0 ${ringSize} ${ringSize}`" aria-hidden="true">
              <circle
                :cx="ringSize / 2"
                :cy="ringSize / 2"
                :r="ringRadius"
                fill="none"
                stroke="var(--color-fill-3, #e5e6eb)"
                :stroke-width="ringStroke"
              />
              <circle
                :cx="ringSize / 2"
                :cy="ringSize / 2"
                :r="ringRadius"
                fill="none"
                :stroke="ringStrokeColor"
                :stroke-width="ringStroke"
                stroke-linecap="round"
                :stroke-dasharray="ringCircumference"
                :stroke-dashoffset="ringOffset"
                :transform="`rotate(-90 ${ringSize / 2} ${ringSize / 2})`"
              />
            </svg>
            <div class="quality-inner">
              <div class="quality-value" :class="`quality-value--${qualityTone}`">{{ qualityScore }}</div>
              <div class="quality-label">质量评分</div>
              <div class="quality-level" :class="`quality-level--${qualityTone}`">{{ getQualityLabel(qualityScore) }}</div>
            </div>
          </div>
        </div>

        <!-- 右侧状态信息 -->
        <div class="flex-1 min-w-0">
          <div class="flex items-center mb-2">
            <a-tag size="small" :color="qualityTone" class="font-medium">
              通道状态: {{ getQualityLabel(qualityScore) }}
            </a-tag>
            <span v-if="metrics?.reconnectCount > 0" class="vd-caption text-amber-500 ml-3 flex items-center gap-1">
              <icon-refresh /> 重连 {{ metrics.reconnectCount }} 次
            </span>
          </div>

          <div class="vd-caption text-gray-500 mb-1 flex items-center gap-1">
            <icon-clock-circle /> {{ connectionDuration }}
          </div>

          <div class="vd-caption text-gray-500 flex items-center gap-1">
            <icon-cloud /> {{ getNetworkConnectionText() }}
          </div>
        </div>

        <!-- 展开按钮 -->
        <a-button size="small" type="text" @click="showDetails = !showDetails">
          <template #icon>
            <icon-up v-if="showDetails" />
            <icon-down v-else />
          </template>
        </a-button>
      </div>

      <!-- 核心指标 -->
      <a-row :gutter="[8, 8]" class="metrics-summary">
        <a-col :span="8">
          <div class="metric-item text-center">
            <div class="vd-caption text-gray-500">成功率</div>
            <div class="text-base font-semibold" :class="getSuccessRateColor(metrics?.successRate)">
              {{ formatPercent(metrics?.successRate) }}
            </div>
          </div>
        </a-col>
        <a-col :span="8">
          <div class="metric-item text-center">
            <div class="vd-caption text-gray-500">平均 RTT</div>
            <div class="text-base font-semibold">
              {{ formatDuration(metrics?.avgRtt) }}
            </div>
          </div>
        </a-col>
        <a-col :span="8">
          <div class="metric-item text-center">
            <div class="vd-caption text-gray-500">丢包率</div>
            <div class="text-base font-semibold" :class="getPacketLossColor(metrics?.packetLoss)">
              {{ formatPercent(metrics?.packetLoss) }}
            </div>
          </div>
        </a-col>
      </a-row>

      <!-- 通信计数指标 -->
      <a-row v-if="showDetails" :gutter="[8, 8]" class="metrics-counts mt-2">
        <a-col :span="8">
          <div class="metric-item text-center">
            <div class="vd-caption text-gray-500">总请求数</div>
            <div class="text-base font-semibold">{{ metrics?.totalRequests || 0 }}</div>
          </div>
        </a-col>
        <a-col :span="8">
          <div class="metric-item text-center">
            <div class="vd-caption text-gray-500">成功次数</div>
            <div class="text-base font-semibold text-green-600">{{ metrics?.successCount || 0 }}</div>
          </div>
        </a-col>
        <a-col :span="8">
          <div class="metric-item text-center">
            <div class="vd-caption text-gray-500">失败次数</div>
            <div class="text-base font-semibold text-red-600">{{ metrics?.failureCount || 0 }}</div>
          </div>
        </a-col>
      </a-row>
    </div>
  </a-card>
</template>

<script setup>
import { ref, computed } from 'vue'
import {
  IconClockCircle,
  IconRefresh,
  IconCloud,
  IconUp,
  IconDown
} from '@arco-design/web-vue/es/icon'

const props = defineProps({
  metrics: {
    type: Object,
    default: () => ({})
  }
})

const showDetails = ref(false)

/* =======================
   质量评分计算（100分制）
======================= */
const qualityScore = computed(() => {
  if (!props.metrics) return 100
  const m = props.metrics
  let score = 100
  if (m.successRate !== undefined) score -= (1 - m.successRate) * 40
  if (m.crcErrorRate !== undefined) score -= m.crcErrorRate * 20
  if (m.retryRate !== undefined) score -= m.retryRate * 20
  if (m.avgRtt > 100) score -= Math.min(10, (m.avgRtt - 100) / 50)
  return Math.max(0, Math.round(score))
})

/* =======================
   连接时长
======================= */
const connectionDuration = computed(() => {
  const seconds = props.metrics?.connectionSeconds || 0
  if (seconds < 60) return `已连接 ${seconds}s`
  if (seconds < 3600) return `已连接 ${Math.floor(seconds / 60)}m`
  return `已连接 ${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`
})

/* =======================
   网络地址信息
======================= */
const networkInfo = computed(() => {
  if (!props.metrics) {
    return { localIp: '-', localPort: '-', remoteIp: '-', remotePort: '-' }
  }
  let localIp = props.metrics.localIp || props.metrics.local_ip
  let localPort = props.metrics.localPort || props.metrics.local_port
  let remoteIp = props.metrics.remoteIp || props.metrics.remote_ip
  let remotePort = props.metrics.remotePort || props.metrics.remote_port

  const parseAddressString = (addrStr) => {
    if (!addrStr) return { ip: '-', port: '-' }
    let addr = addrStr
    if (addr.includes('://')) addr = addr.split('://')[1] || addr
    if (addr.startsWith('[')) {
      const bracketIdx = addr.indexOf(']')
      if (bracketIdx > 0) {
        const ip = addr.substring(1, bracketIdx)
        const rest = addr.substring(bracketIdx + 1)
        if (rest.startsWith(':')) return { ip, port: rest.substring(1).split('/')[0] }
        return { ip, port: '-' }
      }
    }
    const colonIdx = addr.lastIndexOf(':')
    if (colonIdx > 0) {
      const ip = addr.substring(0, colonIdx)
      let port = addr.substring(colonIdx + 1)
      const slashIdx = port.indexOf('/')
      if (slashIdx > 0) port = port.substring(0, slashIdx)
      return { ip, port }
    }
    return { ip: addr, port: '-' }
  }

  if (!localIp && props.metrics.localAddr) {
    const parsed = parseAddressString(props.metrics.localAddr)
    localIp = parsed.ip
    localPort = parsed.port
  }
  if (!remoteIp && props.metrics.remoteAddr) {
    const parsed = parseAddressString(props.metrics.remoteAddr)
    remoteIp = parsed.ip
    remotePort = parsed.port
  }
  return {
    localIp: localIp || '-',
    localPort: localPort || '-',
    remoteIp: remoteIp || '-',
    remotePort: remotePort || '-'
  }
})

/* =======================
   质量等级
======================= */
const getQualityLabel = (score) => {
  if (score >= 90) return 'Excellent'
  if (score >= 75) return 'Good'
  if (score >= 60) return 'Unstable'
  return 'Poor'
}

const getQualityColor = (score) => {
  if (score >= 90) return 'success'
  if (score >= 75) return 'info'
  if (score >= 60) return 'warning'
  return 'error'
}

const qualityTone = computed(() => {
  const c = getQualityColor(qualityScore.value)
  return c === 'success' ? 'green'
    : c === 'info' ? 'arcoblue'
    : c === 'warning' ? 'orange'
    : 'red'
})

const getNetworkConnectionText = () => {
  if (!props.metrics) return '暂无网络连接信息'
  const info = networkInfo.value
  const local = `${info.localIp || '-'}:${info.localPort || '-'}`
  const remoteAddr = props.metrics.remoteAddr
  if (remoteAddr && remoteAddr.includes(':')) {
    const parts = remoteAddr.split(':')
    if (parts.length >= 2 && !isNaN(parts[1])) {
      return `本地 ${local} → 目标 ${info.remoteIp}:${info.remotePort}`
    }
  }
  if (remoteAddr && remoteAddr !== '') return `本地 ${local} → ${remoteAddr}`
  return `本地 ${local} → 目标 -:-`
}

/* =======================
   颜色规则
======================= */
const getSuccessRateColor = (rate) => {
  if (rate >= 0.99) return 'text-green-600'
  if (rate >= 0.95) return 'text-amber-500'
  return 'text-red-600'
}
const getPacketLossColor = (rate) => {
  if (rate < 0.01) return 'text-green-600'
  if (rate < 0.05) return 'text-amber-500'
  return 'text-red-600'
}

/* =======================
   格式化
======================= */
const formatPercent = (val) => {
  if (val === undefined || val === null) return '-'
  return (val * 100).toFixed(1) + '%'
}
const formatDuration = (ms) => {
  if (ms === undefined || ms === null) return '-'
  if (ms < 1) return '<1ms'
  if (ms < 1000) return ms.toFixed(2) + 'ms'
  return (ms / 1000).toFixed(2) + 's'
}

/* =======================
   圆形进度环 (SVG)
======================= */
const ringSize = 120
const ringStroke = 10
const ringRadius = (ringSize - ringStroke) / 2
const ringCircumference = 2 * Math.PI * ringRadius
const ringOffset = computed(() => ringCircumference * (1 - qualityScore.value / 100))
const ringStrokeColor = computed(() => {
  const t = qualityTone.value
  return t === 'green' ? '#00b42a'
    : t === 'arcoblue' ? '#165dff'
    : t === 'orange' ? '#ff7d00'
    : '#f53f3f'
})
const ringStyle = {
  position: 'relative',
  width: `${ringSize}px`,
  height: `${ringSize}px`
}
</script>

<style scoped>
/* 组件特有样式；通用 utility class (flex / items-center / gap-* / text-* / 颜色 / mb-? 等) 已集中在 src/styles/form-controls.css */
.metrics-card-glass {
  border-radius: 12px;
}
.quality-score-wrapper {
  flex-shrink: 0;
}
.quality-ring {
  display: inline-block;
  position: relative;
}
.quality-ring svg {
  display: block;
}
.quality-inner {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
}
.quality-value {
  font-size: 28px;
  font-weight: 700;
  line-height: 1.1;
}
.quality-value--green { color: #00b42a; }
.quality-value--arcoblue { color: #165dff; }
.quality-value--orange { color: #ff7d00; }
.quality-value--red { color: #f53f3f; }
.quality-label {
  font-size: 11px;
  color: var(--color-text-3, #86909c);
  margin-top: 2px;
}
.quality-level {
  font-size: 12px;
  font-weight: 600;
  margin-top: 2px;
}
.quality-level--green { color: #00b42a; }
.quality-level--arcoblue { color: #165dff; }
.quality-level--orange { color: #ff7d00; }
.quality-level--red { color: #f53f3f; }
.metric-item {
  padding: 8px 4px;
  border-radius: 6px;
}
</style>
