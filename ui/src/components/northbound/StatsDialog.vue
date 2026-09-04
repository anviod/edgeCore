<template>
  <a-modal v-model:visible="visible" :title="title" :width="900" :footer="false" modal-class="northbound-stats-modal" unmount-on-close>
    <template v-if="isClientPushMode">
      <a-row :gutter="16" class="nb-stats-grid">
        <a-col :span="6">
          <a-card class="nb-stat-card" :bordered="false">
            <div class="nb-stat-card__label">发送成功</div>
            <div class="nb-stat-card__value nb-stat-card__value--success">{{ stats.success_count || 0 }}</div>
          </a-card>
        </a-col>
        <a-col :span="6">
          <a-card class="nb-stat-card" :bordered="false">
            <div class="nb-stat-card__label">发送失败</div>
            <div class="nb-stat-card__value nb-stat-card__value--danger">{{ stats.fail_count || 0 }}</div>
          </a-card>
        </a-col>
        <a-col :span="6">
          <a-card class="nb-stat-card" :bordered="false">
            <div class="nb-stat-card__label">重连次数</div>
            <div class="nb-stat-card__value nb-stat-card__value--warning">{{ stats.reconnect_count || 0 }}</div>
          </a-card>
        </a-col>
        <a-col :span="6">
          <a-card class="nb-stat-card" :bordered="false">
            <div class="nb-stat-card__label">断线时长</div>
            <div class="nb-stat-card__value">{{ disconnectDuration }}</div>
          </a-card>
        </a-col>
      </a-row>
      <a-row :gutter="16" class="nb-stats-grid">
        <a-col :span="12">
          <a-card class="nb-stat-card" :bordered="false">
            <div class="nb-stat-card__label">运行时长</div>
            <div class="nb-stat-card__value">{{ formatUptime(stats.uptime || 0) }}</div>
          </a-card>
        </a-col>
        <a-col :span="12">
          <a-card class="nb-stat-card" :bordered="false">
            <div class="nb-stat-card__label">推送成功率</div>
            <div class="nb-stat-card__value nb-stat-card__value--success">{{ successRate }}%</div>
          </a-card>
        </a-col>
      </a-row>
      <!-- EAN Runtime 指标 -->
      <template v-if="stats.ean_metrics">
        <a-divider class="nb-stats-divider">EAN Runtime 指标</a-divider>
        <a-row :gutter="16" class="nb-stats-grid">
          <a-col :span="6">
            <a-card class="nb-stat-card" :bordered="false">
              <div class="nb-stat-card__label">Invoke 总数</div>
              <div class="nb-stat-card__value">{{ stats.ean_metrics.total_invokes || 0 }}</div>
            </a-card>
          </a-col>
          <a-col :span="6">
            <a-card class="nb-stat-card" :bordered="false">
              <div class="nb-stat-card__label">成功率</div>
              <div class="nb-stat-card__value nb-stat-card__value--success">{{ eanSuccessRate }}%</div>
            </a-card>
          </a-col>
          <a-col :span="6">
            <a-card class="nb-stat-card" :bordered="false">
              <div class="nb-stat-card__label">P50 延迟</div>
              <div class="nb-stat-card__value">{{ stats.ean_metrics.p50_latency_ms || 0 }}ms</div>
            </a-card>
          </a-col>
          <a-col :span="6">
            <a-card class="nb-stat-card" :bordered="false">
              <div class="nb-stat-card__label">P99 延迟</div>
              <div class="nb-stat-card__value nb-stat-card__value--warning">{{ stats.ean_metrics.p99_latency_ms || 0 }}ms</div>
            </a-card>
          </a-col>
        </a-row>
        <a-row :gutter="16" class="nb-stats-grid">
          <a-col :span="6">
            <a-card class="nb-stat-card" :bordered="false">
              <div class="nb-stat-card__label">成功</div>
              <div class="nb-stat-card__value nb-stat-card__value--success">{{ stats.ean_metrics.success_count || 0 }}</div>
            </a-card>
          </a-col>
          <a-col :span="6">
            <a-card class="nb-stat-card" :bordered="false">
              <div class="nb-stat-card__label">失败</div>
              <div class="nb-stat-card__value nb-stat-card__value--danger">{{ stats.ean_metrics.failed_count || 0 }}</div>
            </a-card>
          </a-col>
          <a-col :span="6">
            <a-card class="nb-stat-card" :bordered="false">
              <div class="nb-stat-card__label">超时</div>
              <div class="nb-stat-card__value nb-stat-card__value--warning">{{ stats.ean_metrics.timeout_count || 0 }}</div>
            </a-card>
          </a-col>
          <a-col :span="6">
            <a-card class="nb-stat-card" :bordered="false">
              <div class="nb-stat-card__label">平均延迟</div>
              <div class="nb-stat-card__value">{{ (stats.ean_metrics.avg_latency_ms || 0).toFixed(1) }}ms</div>
            </a-card>
          </a-col>
        </a-row>
      </template>
    </template>

    <template v-else-if="isOpcuaServerMode">
      <a-row :gutter="16" class="nb-stats-grid">
        <a-col :span="6">
          <a-card class="nb-stat-card" :bordered="false">
            <div class="nb-stat-card__label">当前连接客户端</div>
            <div class="nb-stat-card__value nb-stat-card__value--primary">{{ stats.client_count || 0 }}</div>
          </a-card>
        </a-col>
        <a-col :span="6">
          <a-card class="nb-stat-card" :bordered="false">
            <div class="nb-stat-card__label">当前订阅数量</div>
            <div class="nb-stat-card__value nb-stat-card__value--info">{{ stats.subscription_count || 0 }}</div>
          </a-card>
        </a-col>
        <a-col :span="6">
          <a-card class="nb-stat-card" :bordered="false">
            <div class="nb-stat-card__label">最近读操作</div>
            <div class="nb-stat-card__value nb-stat-card__value--success">{{ stats.read_count || 0 }}</div>
          </a-card>
        </a-col>
        <a-col :span="6">
          <a-card class="nb-stat-card" :bordered="false">
            <div class="nb-stat-card__label">最近写操作</div>
            <div class="nb-stat-card__value nb-stat-card__value--primary">{{ stats.write_count || 0 }}</div>
          </a-card>
        </a-col>
      </a-row>
      <a-row :gutter="16" class="nb-stats-grid">
        <a-col :span="12">
          <a-card class="nb-stat-card" :bordered="false">
            <div class="nb-stat-card__label">运行时长</div>
            <div class="nb-stat-card__value">{{ formatUptime(stats.uptime || 0) }}</div>
          </a-card>
        </a-col>
        <a-col :span="12">
          <a-card class="nb-stat-card" :bordered="false">
            <div class="nb-stat-card__label">数据吞吐量</div>
            <div class="nb-stat-card__value nb-stat-card__value--info">{{ formatThroughput(stats.throughput || 0) }}</div>
          </a-card>
        </a-col>
      </a-row>
    </template>

    <template v-else-if="isBacnetServerMode">
      <a-row :gutter="16" class="nb-stats-grid">
        <a-col :span="6">
          <a-card class="nb-stat-card" :bordered="false">
            <div class="nb-stat-card__label">已映射对象</div>
            <div class="nb-stat-card__value nb-stat-card__value--primary">{{ stats.object_count || 0 }}</div>
          </a-card>
        </a-col>
        <a-col :span="6">
          <a-card class="nb-stat-card" :bordered="false">
            <div class="nb-stat-card__label">已映射点位</div>
            <div class="nb-stat-card__value nb-stat-card__value--info">{{ stats.point_count || 0 }}</div>
          </a-card>
        </a-col>
        <a-col :span="6">
          <a-card class="nb-stat-card" :bordered="false">
            <div class="nb-stat-card__label">数据更新次数</div>
            <div class="nb-stat-card__value nb-stat-card__value--success">{{ stats.update_count || 0 }}</div>
          </a-card>
        </a-col>
        <a-col :span="6">
          <a-card class="nb-stat-card" :bordered="false">
            <div class="nb-stat-card__label">外部写入次数</div>
            <div class="nb-stat-card__value nb-stat-card__value--warning">{{ stats.write_count || 0 }}</div>
          </a-card>
        </a-col>
      </a-row>
      <a-row :gutter="16" class="nb-stats-grid">
        <a-col :span="12">
          <a-card class="nb-stat-card" :bordered="false">
            <div class="nb-stat-card__label">运行时长</div>
            <div class="nb-stat-card__value">{{ formatUptime(stats.uptime || 0) }}</div>
          </a-card>
        </a-col>
        <a-col :span="12">
          <a-card class="nb-stat-card" :bordered="false">
            <div class="nb-stat-card__label">最近写入时间</div>
            <div class="nb-stat-card__value nb-stat-card__value--info">{{ stats.last_write_time ? formatTime(stats.last_write_time) : '-' }}</div>
          </a-card>
        </a-col>
      </a-row>
    </template>

    <a-divider :margin="12" />

    <div class="nb-stats-toolbar">
      <span class="nb-stats-toolbar__title">实时日志 ({{ logTitle }})</span>
      <div class="nb-stats-toolbar__spacer" />
      <a-switch v-model="isStreaming" size="small" />
      <span class="nb-stats-toolbar__hint">实时滚动</span>
      <a-button type="outline" size="small" @click="downloadLogs">
        <template #icon><icon-download :size="12" /></template>
        下载日志
      </a-button>
    </div>

    <div class="log-viewer">
      <div v-if="paginatedLogs.length === 0" class="log-empty">暂无日志...</div>
      <div v-for="(log, idx) in paginatedLogs" :key="idx" class="log-line">
        <span class="log-line__time">[{{ formatTime(log.ts) }}]</span>
        <span :style="{ color: getLevelColor(log.level), fontWeight: 'bold', marginRight: '8px' }">{{ (log.level || 'INFO').toUpperCase() }}</span>
        <span>{{ log.msg }}</span>
        <span v-for="(val, key) in getExtraFields(log)" :key="key" class="log-line__time" style="margin-left: 8px">
          {{ key }}={{ val }}
        </span>
      </div>
    </div>

    <div class="nb-stats-pagination">
      <a-pagination v-model:current="page" :page-size="20" :total="logs.length" size="small" show-page-size />
    </div>
  </a-modal>
</template>

<script setup>
import { ref, watch, computed, onUnmounted } from 'vue'
import { IconDownload } from '@arco-design/web-vue/es/icon'
import request from '@/utils/request'

const props = defineProps({
  visible: { type: Boolean, default: false },
  type: { type: String, default: 'mqtt' },
  itemId: { type: String, default: '' }
})

const emit = defineEmits(['update:visible'])

const visible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val)
})
const isStreaming = ref(true)
const stats = ref({})
const logs = ref([])
const page = ref(1)
let timer = null
let ws = null

const title = computed(() => {
  if (props.type === 'mqtt') return 'MQTT 运行监控'
  if (props.type === 'http') return 'HTTP 运行监控'
  if (props.type === 'sparkplug_b') return 'Sparkplug B 运行监控'
  if (props.type === 'opcua') return 'OPC UA 运行监控'
  if (props.type === 'bacnet_server') return 'BACnet Server 运行监控'
  if (props.type === 'edgeos-mqtt') return 'edgeOS(MQTT) 运行监控'
  if (props.type === 'edgeos-nats') return 'edgeOS(NATS) 运行监控'
  return '运行监控'
})

const isClientPushMode = computed(() => {
  return props.type === 'mqtt' || props.type === 'http' || props.type === 'sparkplug_b' || props.type === 'edgeos-mqtt' || props.type === 'edgeos-nats'
})

const isOpcuaServerMode = computed(() => {
  return props.type === 'opcua'
})

const isBacnetServerMode = computed(() => {
  return props.type === 'bacnet_server'
})

const paginatedLogs = computed(() => {
  const start = (page.value - 1) * 20
  return logs.value.slice(start, start + 20)
})

const logTitle = computed(() => {
  if (props.type === 'mqtt') return 'MQTT'
  if (props.type === 'http') return 'HTTP'
  if (props.type === 'sparkplug_b') return 'Sparkplug B'
  if (props.type === 'opcua') return 'OPC UA'
  if (props.type === 'bacnet_server') return 'BACnet Server'
  if (props.type === 'edgeos-mqtt') return 'edgeOS(MQTT)'
  if (props.type === 'edgeos-nats') return 'edgeOS(NATS)'
  return '日志'
})

const disconnectDuration = computed(() => {
  const offlineTime = stats.value.last_offline_time
  const onlineTime = stats.value.last_online_time
  if (!offlineTime) return '0s'
  const now = Date.now()
  if (offlineTime > onlineTime) {
    return formatUptime(Math.floor((now - offlineTime) / 1000))
  }
  return '0s'
})

const successRate = computed(() => {
  const success = stats.value.success_count || 0
  const fail = stats.value.fail_count || 0
  const total = success + fail
  if (total === 0) return 0
  return Math.round((success / total) * 100)
})

const eanSuccessRate = computed(() => {
  const rate = stats.value.ean_metrics?.success_rate || 0
  return Math.round(rate * 100)
})

const formatThroughput = (bytes) => {
  if (!bytes || bytes === 0) return '0 B'
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(2) + ' KB'
  if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(2) + ' MB'
  return (bytes / (1024 * 1024 * 1024)).toFixed(2) + ' GB'
}

const cleanup = () => {
  if (timer) { clearInterval(timer); timer = null }
  if (ws) { ws.close(); ws = null }
}

onUnmounted(cleanup)

watch(() => props.visible, (val) => {
  if (val) {
    logs.value = []
    page.value = 1
    isStreaming.value = true
    refreshStats()
    timer = setInterval(refreshStats, props.type === 'mqtt' ? 1000 : 3000)
    connectWs()
  } else {
    cleanup()
  }
})

const refreshStats = async () => {
  if (!props.itemId) return
  try {
    let apiType = props.type
    if (apiType === 'edgeos-mqtt') apiType = 'edgeos-mqtt'
    if (apiType === 'edgeos-nats') apiType = 'edgeos-nats'
    if (apiType === 'sparkplug_b') apiType = 'sparkplugb'
    if (apiType === 'bacnet_server') apiType = 'bacnet_server'

    const data = await request.get(`/api/northbound/${apiType}/${props.itemId}/stats`, { silent: true })
    stats.value = data
  } catch (e) {}
}

const connectWs = () => {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  let token = ''
  try {
    const raw = localStorage.getItem('loginInfo')
    if (raw) {
      const parsed = JSON.parse(raw)
      token = parsed.token || (parsed.data && parsed.data.token) || ''
    }
  } catch (e) {}

  ws = new WebSocket(`${protocol}//${host}/api/ws/logs?token=${token}`)
  ws.onmessage = (event) => {
    if (!isStreaming.value) return
    try {
      const log = JSON.parse(event.data)
      let targetComponents = []

      if (isClientPushMode.value) {
        targetComponents = ['mqtt-client', 'http-client', 'sparkplugb-client', 'edgos-mqtt-client', 'edgos-nats-client']
      } else if (isOpcuaServerMode.value) {
        targetComponents = ['opcua-server']
      } else if (isBacnetServerMode.value) {
        targetComponents = ['bacnet-server']
      }

      if (targetComponents.includes(log.component)) {
        logs.value.unshift(log)
        if (logs.value.length > 500) logs.value.pop()
        if (page.value !== 1) page.value = 1
      }
    } catch (e) {}
  }
}

const formatTime = (ts) => {
  if (!ts) return ''
  return new Date(ts).toLocaleTimeString() + '.' + new Date(ts).getMilliseconds().toString().padStart(3, '0')
}

const formatUptime = (seconds) => {
  if (seconds < 60) return seconds + '秒'
  if (seconds < 3600) return Math.floor(seconds / 60) + '分' + (seconds % 60) + '秒'
  const hours = Math.floor(seconds / 3600)
  const mins = Math.floor((seconds % 3600) / 60)
  return hours + '小时' + mins + '分'
}

const getLevelColor = (level) => {
  const l = (level || '').toUpperCase()
  if (l === 'ERROR' || l === 'FATAL') return '#f53f3f'
  if (l === 'WARN') return '#ff7d00'
  return '#00b42a'
}

const getExtraFields = (log) => {
  const { ts, level, msg, caller, component, ...rest } = log
  return rest
}

const downloadLogs = () => {
  const rows = logs.value.map(log => {
    const ts = log.ts ? new Date(log.ts).toLocaleString() : ''
    const level = (log.level || 'INFO').toUpperCase()
    const msg = log.msg || ''
    return `[${ts}] [${level}] ${msg}`
  })
  const content = rows.join('\n')
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8;' })
  const link = document.createElement('a')
  link.href = URL.createObjectURL(blob)
  link.download = `${props.type}_logs_${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.log`
  link.click()
  URL.revokeObjectURL(link.href)
}
</script>
