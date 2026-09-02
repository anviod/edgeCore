<template>
  <a-card class="metrics-card-glass device-metrics-card" :class="{ 'degraded': isDegraded }">
    <div class="px-3 py-3">
      <!-- 顶部：健康度 + 设备状态 -->
      <div class="flex items-center mb-3">
        <!-- 健康度圆环 -->
        <div class="health-indicator mr-3">
          <div class="health-ring" :style="ringStyle">
            <svg :width="ringSize" :height="ringSize" :viewBox="`0 0 ${ringSize} ${ringSize}`" aria-hidden="true">
              <circle :cx="ringSize / 2" :cy="ringSize / 2" :r="ringRadius" fill="none"
                stroke="var(--color-fill-3, #e5e6eb)" :stroke-width="ringStroke" />
              <circle :cx="ringSize / 2" :cy="ringSize / 2" :r="ringRadius" fill="none"
                :stroke="ringStrokeColor" :stroke-width="ringStroke" stroke-linecap="round"
                :stroke-dasharray="ringCircumference" :stroke-dashoffset="ringOffset"
                :transform="`rotate(-90 ${ringSize / 2} ${ringSize / 2})`" />
            </svg>
            <div class="health-icon" :class="`text-${healthTone}`">
              <icon-heart-fill v-if="healthScore >= 90" />
              <icon-heart v-else-if="healthScore >= 50" />
              <icon-close v-else />
            </div>
          </div>
        </div>

        <!-- 设备状态文字 -->
        <div class="flex-1 min-w-0">
          <div class="flex items-center mb-1">
            <a-tag size="small" :color="stateTone" class="mr-2">
              {{ getStateText(device?.state) }}
            </a-tag>
            <a-tag v-if="isDegraded" size="small" color="orange">
              已降级
            </a-tag>
          </div>
          <div class="vd-caption text-gray-500 flex items-center gap-1">
            <icon-clock-circle />
            <span>{{ lastCollectTime }}</span>
          </div>
        </div>

        <!-- 失败计数徽章 -->
        <a-badge v-if="metrics?.consecutiveFailures > 0" :text="String(metrics.consecutiveFailures)">
          <icon-exclamation-circle class="text-xl text-red-500" />
        </a-badge>
      </div>

      <!-- 核心采集指标 -->
      <a-row :gutter="[8, 8]" class="metrics-row">
        <a-col :span="8">
          <div class="metric-box text-center" :class="{ 'has-issue': metrics?.pointSuccessRate < 0.95 }">
            <div class="vd-caption text-gray-500">点位成功率</div>
            <div class="text-lg font-semibold" :class="pointSuccessTone">
              {{ formatPercent(metrics?.pointSuccessRate) }}
            </div>
          </div>
        </a-col>
        <a-col :span="8">
          <div class="metric-box text-center">
            <div class="vd-caption text-gray-500">采集耗时</div>
            <div class="text-lg font-semibold">
              {{ formatDuration(metrics?.avgCollectTime) }}
            </div>
          </div>
        </a-col>
        <a-col :span="8">
          <div class="metric-box text-center" :class="{ 'has-issue': metrics?.nullValueRate > 0.05 }">
            <div class="vd-caption text-gray-500">Null值比例</div>
            <div class="text-lg font-semibold" :class="nullRateTone">
              {{ formatPercent(metrics?.nullValueRate) }}
            </div>
          </div>
        </a-col>
      </a-row>

      <!-- 扩展详情 -->
      <Transition name="expand">
        <div v-show="showDetails" class="mt-3 pt-3 border-t border-gray-200">
          <a-row :gutter="[8, 8]">
            <a-col :span="12">
              <div class="detail-row">
                <span class="vd-caption text-gray-500">调度周期</span>
                <span class="text-sm">{{ device?.interval || '-' }}</span>
              </div>
            </a-col>
            <a-col :span="12">
              <div class="detail-row">
                <span class="vd-caption text-gray-500">健康评分</span>
                <span class="text-sm font-semibold" :class="`text-${healthTone}`">{{ healthScore }}</span>
              </div>
            </a-col>
            <a-col :span="12">
              <div class="detail-row">
                <span class="vd-caption text-gray-500">EWMARTT</span>
                <span class="text-sm">{{ formatRtt(metrics?.communicationProfile?.rtt) }}</span>
              </div>
            </a-col>
            <a-col :span="12">
              <div class="detail-row">
                <span class="vd-caption text-gray-500">Gap / MTU</span>
                <span class="text-sm">
                  {{ metrics?.communicationProfile?.gap ?? '-' }} / {{ metrics?.communicationProfile?.mtu ?? '-' }}
                </span>
              </div>
            </a-col>
            <a-col :span="12">
              <div class="detail-row">
                <span class="vd-caption text-gray-500">异常点位</span>
                <span class="text-sm" :class="metrics?.abnormalPoints > 0 ? 'text-amber-500' : ''">
                  {{ metrics?.abnormalPoints || 0 }}
                </span>
              </div>
            </a-col>
            <a-col :span="12">
              <div class="detail-row">
                <span class="vd-caption text-gray-500">无效值</span>
                <span class="text-sm" :class="metrics?.invalidValues > 0 ? 'text-red-600' : ''">
                  {{ metrics?.invalidValues || 0 }}
                </span>
              </div>
            </a-col>
          </a-row>

          <!-- 连续失败警告 -->
          <a-alert v-if="metrics?.consecutiveFailures >= 3" type="warning" class="mt-2" show-icon>
            连续失败 {{ metrics.consecutiveFailures }} 次，设备已降级
          </a-alert>

          <!-- 恢复中提示 -->
          <a-alert v-if="device?.recovering" type="info" class="mt-2" show-icon>
            <template #icon><icon-loading /></template>
            设备恢复中...
          </a-alert>
        </div>
      </Transition>
    </div>
  </a-card>
</template>

<script setup>
import { ref, computed } from 'vue'
import {
  IconClockCircle,
  IconExclamationCircle,
  IconHeartFill,
  IconHeart,
  IconClose,
  IconLoading
} from '@arco-design/web-vue/es/icon'

const props = defineProps({
  device: { type: Object, default: () => ({}) },
  metrics: { type: Object, default: () => ({}) },
  showDetails: { type: Boolean, default: false }
})

// 是否降级
const isDegraded = computed(() =>
  props.device?.degraded || props.metrics?.consecutiveFailures >= 3
)

// 健康评分计算
const healthScore = computed(() => {
  if (!props.metrics && !props.device) return 100
  let health = 100
  const m = props.metrics || {}
  if (m.consecutiveFailures) health -= m.consecutiveFailures * 10
  if (m.abnormalPointRate) health -= m.abnormalPointRate * 30
  if (m.timeoutRate) health -= m.timeoutRate * 30
  if (m.nullValueRate) health -= m.nullValueRate * 20
  return Math.max(0, Math.round(health))
})

// 最后采集时间
const lastCollectTime = computed(() => {
  const ts = props.metrics?.lastCollectTime || props.device?.lastCollectTime
  if (!ts) return '从未采集'
  const date = new Date(ts)
  const now = new Date()
  const diff = now - date
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)}分钟前`
  return date.toLocaleString()
})

// 状态文本
const getStateText = (state) => {
  switch (state) {
    case 0: return '在线'
    case 1: return '不稳定'
    case 2: return '离线'
    case 3: return '隔离'
    default: return '未知'
  }
}

const stateTone = computed(() => {
  switch (props.device?.state) {
    case 0: return 'green'
    case 1: return 'orange'
    case 2: return 'red'
    case 3: return 'gray'
    default: return 'gray'
  }
})

// 健康度
const healthTone = computed(() => {
  const s = healthScore.value
  if (s >= 90) return 'green-600'
  if (s >= 70) return 'amber-500'
  if (s >= 50) return 'orange'
  return 'red-600'
})
const ringStrokeColor = computed(() => {
  const t = healthTone.value
  return t === 'green-600' ? '#00b42a'
    : t === 'amber-500' ? '#ff7d00'
    : t === 'orange' ? '#ff7d00'
    : '#f53f3f'
})

// 点位成功率 / Null值比例 颜色
const pointSuccessTone = computed(() => {
  const r = props.metrics?.pointSuccessRate
  if (r === undefined || r === null) return ''
  if (r >= 0.98) return 'text-green-600'
  if (r >= 0.90) return 'text-amber-500'
  return 'text-red-600'
})
const nullRateTone = computed(() => {
  const r = props.metrics?.nullValueRate
  if (r === undefined || r === null) return ''
  if (r < 0.01) return 'text-green-600'
  if (r < 0.05) return 'text-amber-500'
  return 'text-red-600'
})

// 格式化
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
const formatRtt = (micros) => {
  if (micros === undefined || micros === null) return '-'
  if (micros < 1000) return micros + 'µs'
  return (micros / 1000).toFixed(2) + 'ms'
}

/* 圆形环 SVG 参数 */
const ringSize = 48
const ringStroke = 4
const ringRadius = (ringSize - ringStroke) / 2
const ringCircumference = 2 * Math.PI * ringRadius
const ringOffset = computed(() => ringCircumference * (1 - healthScore.value / 100))
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
.device-metrics-card.degraded {
  border-color: var(--color-warning-3, #ff7d00);
}
.health-indicator {
  flex-shrink: 0;
}
.health-ring {
  display: inline-block;
  position: relative;
}
.health-ring svg {
  display: block;
}
.health-icon {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
}
.metric-box {
  padding: 8px 4px;
  border-radius: 6px;
  background: var(--color-fill-2, #f7f8fa);
}
.metric-box.has-issue {
  background: rgba(255, 125, 0, 0.08);
}
.detail-row {
  display: flex;
  justify-content: space-between;
  padding: 4px 0;
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
  max-height: 600px;
}
</style>
