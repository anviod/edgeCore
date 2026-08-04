<template>
  <v-card class="glass-card" :class="{ 'metrics-card': true, 'expanded': showDetails }">
    <v-card-text class="pa-4">
      <!-- 顶部：大仪表 + 状态信息 -->
      <div class="d-flex align-center mb-4">
        <!-- 🔵 大圆形质量仪表 -->
        <div class="quality-score-wrapper mr-6">
          <v-progress-circular
            :model-value="qualityScore"
            :color="getQualityColor(qualityScore)"
            :size="120"
            :width="10"
            bg-color="grey-darken-3"
            class="quality-ring"
          >
            <div class="quality-inner">
              <div
                class="quality-value"
                :class="`text-${getQualityColor(qualityScore)}`"
              >
                {{ qualityScore }}
              </div>
              <div class="quality-label">
                质量评分
              </div>
              <div
                class="quality-level"
                :class="`text-${getQualityColor(qualityScore)}`"
              >
                {{ getQualityLabel(qualityScore) }}
              </div>
            </div>
          </v-progress-circular>
        </div>

        <!-- 右侧状态信息 -->
        <div class="flex-grow-1">
          <div class="d-flex align-center mb-2">
            <v-chip
              size="small"
              :color="getQualityColor(qualityScore)"
              variant="flat"
              class="font-weight-medium"
            >
              通道状态: {{ getQualityLabel(qualityScore) }}
            </v-chip>

            <span v-if="metrics?.reconnectCount > 0" class="text-caption text-warning ml-3">
              <v-icon size="x-small">mdi-refresh-alert</v-icon>
              重连 {{ metrics.reconnectCount }} 次
            </span>
          </div>

          <div class="text-caption text-grey-darken-1 mb-1">
            <v-icon size="x-small">mdi-clock-outline</v-icon>
            {{ connectionDuration }}
          </div>

          <div class="text-caption text-grey-darken-1">
            <v-icon size="x-small">mdi-lan-connect</v-icon>
            {{ getNetworkConnectionText() }}
          </div>
        </div>

        <!-- 展开按钮 -->
        <v-btn
          size="small"
          variant="text"
          :icon="showDetails ? 'mdi-chevron-up' : 'mdi-chevron-down'"
          @click="showDetails = !showDetails"
        />
      </div>

      <!-- 核心指标 -->
      <v-row dense class="metrics-summary">
        <v-col cols="4">
          <div class="metric-item text-center">
            <div class="text-caption text-grey-darken-1">成功率</div>
            <div
              class="text-body-1 font-weight-bold"
              :class="getSuccessRateColor(metrics?.successRate)"
            >
              {{ formatPercent(metrics?.successRate) }}
            </div>
          </div>
        </v-col>

        <v-col cols="4">
          <div class="metric-item text-center">
            <div class="text-caption text-grey-darken-1">平均 RTT</div>
            <div class="text-body-1 font-weight-bold">
              {{ formatDuration(metrics?.avgRtt) }}
            </div>
          </div>
        </v-col>

        <v-col cols="4">
          <div class="metric-item text-center">
            <div class="text-caption text-grey-darken-1">丢包率</div>
            <div
              class="text-body-1 font-weight-bold"
              :class="getPacketLossColor(metrics?.packetLoss)"
            >
              {{ formatPercent(metrics?.packetLoss) }}
            </div>
          </div>
        </v-col>
      </v-row>

      <!-- 通信计数指标 -->
      <v-row dense class="metrics-counts" v-if="showDetails">
        <v-col cols="4">
          <div class="metric-item text-center">
            <div class="text-caption text-grey-darken-1">总请求数</div>
            <div class="text-body-1 font-weight-bold">
              {{ metrics?.totalRequests || 0 }}
            </div>
          </div>
        </v-col>

        <v-col cols="4">
          <div class="metric-item text-center">
            <div class="text-caption text-grey-darken-1">成功次数</div>
            <div class="text-body-1 font-weight-bold text-success">
              {{ metrics?.successCount || 0 }}
            </div>
          </div>
        </v-col>

        <v-col cols="4">
          <div class="metric-item text-center">
            <div class="text-caption text-grey-darken-1">失败次数</div>
            <div class="text-body-1 font-weight-bold text-error">
              {{ metrics?.failureCount || 0 }}
            </div>
          </div>
        </v-col>
      </v-row>
    </v-card-text>
  </v-card>
</template>

<script setup>
import { ref, computed } from 'vue'

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

  let score = 100
  const m = props.metrics

  if (m.successRate !== undefined)
    score -= (1 - m.successRate) * 40

  if (m.crcErrorRate !== undefined)
    score -= m.crcErrorRate * 20

  if (m.retryRate !== undefined)
    score -= m.retryRate * 20

  if (m.avgRtt > 100)
    score -= Math.min(10, (m.avgRtt - 100) / 50)

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
  
  // 优先使用分开的字段
  let localIp = props.metrics.localIp || props.metrics.local_ip
  let localPort = props.metrics.localPort || props.metrics.local_port
  let remoteIp = props.metrics.remoteIp || props.metrics.remote_ip
  let remotePort = props.metrics.remotePort || props.metrics.remote_port
  
  // 辅助函数：解析地址字符串（处理IP:Port格式）
  const parseAddressString = (addrStr) => {
    if (!addrStr) return { ip: '-', port: '-' }
    
    // 去掉协议前缀
    let addr = addrStr
    if (addr.includes('://')) {
      addr = addr.split('://')[1] || addr
    }
    
    // 提取IP和端口
    // 处理IPv6 [::1]:8080 这种格式
    if (addr.startsWith('[')) {
      const bracketIdx = addr.indexOf(']')
      if (bracketIdx > 0) {
        const ip = addr.substring(1, bracketIdx)
        const rest = addr.substring(bracketIdx + 1)
        if (rest.startsWith(':')) {
          return { ip, port: rest.substring(1).split('/')[0] }
        }
        return { ip, port: '-' }
      }
    }
    
    // 处理普通IPv4格式 IP:Port 或 包含路径的格式 IP:Port/Path
    const colonIdx = addr.lastIndexOf(':')
    if (colonIdx > 0) {
      const ip = addr.substring(0, colonIdx)
      let port = addr.substring(colonIdx + 1)
      
      // 处理包含路径的情况
      const slashIdx = port.indexOf('/')
      if (slashIdx > 0) {
        port = port.substring(0, slashIdx)
      }
      
      return { ip, port }
    }
    
    // 如果找不到冒号，整个字符串作为IP
    return { ip: addr, port: '-' }
  }
  
  // 如果没有分开的字段，尝试解析 localAddr 和 remoteAddr
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

const getNetworkConnectionText = () => {
  if (!props.metrics) {
    return '暂无网络连接信息'
  }
  
  // 获取本地和远程地址信息，确保没有undefined
  const info = networkInfo.value
  const localIp = info.localIp || '-'
  const localPort = info.localPort || '-'
  const remoteIp = info.remoteIp || '-'
  const remotePort = info.remotePort || '-'
  
  const local = `${localIp}:${localPort}`
  
  // 检查remoteAddr是否是IP:Port格式
  const remoteAddr = props.metrics.remoteAddr
  if (remoteAddr && remoteAddr.includes(':')) {
    const parts = remoteAddr.split(':')
    if (parts.length >= 2 && !isNaN(parts[1])) {
      // 是IP:Port格式
      return `本地 ${local} → 目标 ${remoteIp}:${remotePort}`
    }
  }
  
  // 不是标准IP:Port格式，直接显示remoteAddr作为描述
  if (remoteAddr && remoteAddr !== '') {
    return `本地 ${local} → ${remoteAddr}`
  }
  
  return `本地 ${local} → 目标 -:-`
}

/* =======================
   颜色规则
======================= */
const getSuccessRateColor = (rate) => {
  if (rate >= 0.99) return 'text-success'
  if (rate >= 0.95) return 'text-warning'
  return 'text-error'
}

const getPacketLossColor = (rate) => {
  if (rate < 0.01) return 'text-success'
  if (rate < 0.05) return 'text-warning'
  return 'text-error'
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
</script>

<style scoped>
/* v3.0 — styles in src/styles/ */
</style>
