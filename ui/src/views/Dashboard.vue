<template>
  <div class="page-shell dashboard-page">
    <!-- Primary: ScanEngine SLA / Soak monitoring -->

    <section class="dashboard-zone dashboard-zone--primary" aria-label="运行监控">
      <ScanEngineSoakPanel
        :online-devices="totalOnlineDevices"
        :offline-devices="totalOfflineDevices"
        :channel-count="channels.length"
      />
    </section>

    <div v-if="dashboardError" class="dashboard-error-banner">
      <icon-refresh :size="14" />
      <span>{{ dashboardError }}</span>
      <a-button size="mini" @click="fetchData">重试</a-button>
    </div>



    <!-- Secondary: system resource metrics -->

    <section class="dashboard-zone dashboard-zone--secondary" aria-label="系统资源">
      <div class="dashboard-zone-header">
        <h3 class="dashboard-zone-title">系统资源</h3>
        <a-spin v-if="!firstLoaded" :size="14" />
      </div>

      <div class="stats-grid stats-grid--compact">
        <div class="stat-card">
          <div class="stat-label">CPU 使用率</div>

          <div class="stat-value" :style="{ color: getCpuColor(system.cpu_usage) }">
            {{ (system.cpu_usage || 0).toFixed(1) }}%
          </div>

          <div class="stat-bar">
            <div class="stat-progress" :style="{ width: (system.cpu_usage || 0) + '%', background: getCpuColor(system.cpu_usage) }"></div>
          </div>
        </div>

        <div class="stat-card">
          <div class="stat-label">内存使用</div>

          <div class="stat-value" :style="{ color: getMemoryColor(system.memory_usage) }">
            {{ formatMemory(system.memory_usage) }}
          </div>

          <div class="stat-bar">
            <div class="stat-progress" :style="{ width: getMemoryPercent(system.memory_usage) + '%', background: getMemoryColor(system.memory_usage) }"></div>
          </div>
        </div>

        <div class="stat-card">
          <div class="stat-label">协程数量</div>

          <div class="stat-value" style="color: var(--edgeCore-info);">
            {{ system.goroutines || 0 }}
          </div>

          <div class="stat-bar">
            <div class="stat-progress stat-progress--idle"></div>
          </div>
        </div>

        <div class="stat-card">
          <div class="stat-label">磁盘使用率</div>

          <div class="stat-value" :style="{ color: getDiskColor(system.disk_usage) }">
            {{ (system.disk_usage || 0).toFixed(1) }}%
          </div>

          <div class="stat-bar">
            <div class="stat-progress" :style="{ width: (system.disk_usage || 0) + '%', background: getDiskColor(system.disk_usage) }"></div>
          </div>
        </div>
      </div>
    </section>



    <!-- Tertiary: data collection & reporting -->

    <section class="dashboard-zone dashboard-zone--tertiary" aria-label="数据采集与上报">
      <div class="dashboard-zone-header">
        <h3 class="dashboard-zone-title">数据采集与上报</h3>
      </div>



      <!-- Collection channels — full-width block -->

      <div class="dashboard-tertiary-block dashboard-tertiary-block--channels">
        <div class="section">
          <div class="section-header">
            <h3 class="section-title">采集通道</h3>

            <div class="section-status">
              <span class="status-badge online">

                <span class="status-dot"></span>

                在线: {{ totalOnlineDevices }}

              </span>

              <span class="status-badge offline">

                <span class="status-dot"></span>

                离线: {{ totalOfflineDevices }}

              </span>
            </div>
          </div>



          <div class="channels-grid">
            <div v-for="ch in channels" :key="ch.id" class="channel-card" @click="$router.push(`/channels/${ch.id}/devices`)">
              <div class="channel-header">
                <div class="channel-icon" :class="getProtocolClass(ch.protocol)">
                  <icon-link v-if="ch.protocol === 'bacnet-ip'" :size="20" />

                  <icon-link v-else-if="ch.protocol === 'modbus-rtu'" :size="20" />

                  <icon-link v-else-if="ch.protocol === 'modbus-tcp'" :size="20" />

                  <icon-tool v-else-if="ch.protocol === 'opc-ua'" :size="20" />

                  <icon-settings v-else-if="ch.protocol === 's7'" :size="20" />

                  <icon-link v-else :size="20" />
                </div>

                <div class="channel-info">
                  <div class="channel-name">
                    {{ ch.name }}

                    <span class="quality-score" :class="getQualityClass(ch.qualityScore)">{{ ch.qualityScore || '-' }}</span>
                  </div>

                  <div class="channel-meta">
                    {{ formatProtocolTag(ch.protocol) }}

                    <span class="divider">|</span>

                    <span :class="['status-text', ch.enable ? 'enabled' : 'disabled']">{{ ch.enable ? '启用' : '禁用' }}</span>
                  </div>
                </div>

                <icon-arrow-right :size="14" class="arrow-icon" />
              </div>



              <div class="channel-stats">
                <div class="stat-item">
                  <div class="stat-item-label">设备</div>

                  <div class="stat-item-value">{{ ch.device_count || 0 }}</div>
                </div>

                <div class="stat-item">
                  <div class="stat-item-label online">在线</div>

                  <div class="stat-item-value online">{{ ch.online_count || 0 }}</div>
                </div>

                <div class="stat-item">
                  <div class="stat-item-label offline">离线</div>

                  <div class="stat-item-value offline">{{ ch.offline_count || 0 }}</div>
                </div>

                <div class="stat-item">
                  <div class="stat-item-label">成功率</div>

                  <div class="stat-item-value" :class="getSuccessRateClass(ch.successRate)">{{ formatPercent(ch.successRate) }}</div>
                </div>
              </div>



              <div class="channel-metrics" v-if="ch.metrics">
                <div class="metrics-header">
                  <span class="metrics-label">通信质量</span>

                  <span class="metrics-rtt">RTT: {{ formatDuration(ch.metrics.avgRtt) }}</span>
                </div>

                <div class="quality-bar-container">
                  <div class="quality-bar" :class="getQualityBarClass(ch.qualityScore)" :style="{ width: (ch.qualityScore || 0) + '%' }"></div>
                </div>

                <div v-if="ch.metrics.reconnectCount > 0" class="reconnect-info">
                  <icon-refresh :size="12" />

                  重连: {{ ch.metrics.reconnectCount }}
                </div>
              </div>
            </div>



            <div v-if="channels.length === 0" class="empty-card">
              <div class="empty-content">
                <icon-apps :size="48" style="margin-bottom: 12px;" />

                <p>暂无采集通道配置</p>

                <button class="btn-primary" @click="$router.push('/channels')">添加通道</button>
              </div>
            </div>
          </div>
        </div>
      </div>



      <!-- Northbound + edge compute — separate row below channels -->

      <div class="dashboard-tertiary-row">
        <div class="dashboard-tertiary-block dashboard-tertiary-block--northbound">
          <div class="section">
            <div class="section-header">
              <h3 class="section-title">北向数据上报</h3>
            </div>

            <div class="northbound-grid">
              <div v-for="nb in northbound" :key="nb.id" class="northbound-card">
                <div class="northbound-header">
                  <h4 class="northbound-name">{{ nb.name }}</h4>

                  <span class="status-badge" :class="nb.status === 'Running' ? 'online' : (nb.status === 'Disabled' ? 'disabled' : 'offline')">

                    {{ nb.status }}

                  </span>
                </div>

                <div class="northbound-type">{{ nb.type }}</div>

                <div class="northbound-actions">
                  <button class="btn-outline" @click="$router.push('/northbound')">配置</button>
                </div>
              </div>

              <div v-if="northbound.length === 0" class="empty-card">
                <div class="empty-content">
                  <p>暂无北向数据上报配置</p>

                  <button class="btn-primary" @click="$router.push('/northbound')">配置北向</button>
                </div>
              </div>
            </div>
          </div>
        </div>



        <div class="dashboard-tertiary-block dashboard-tertiary-block--edge">
          <div class="section">
            <div class="section-header">
              <h3 class="section-title">边缘计算</h3>
            </div>

            <div class="edge-compute-card" @click="$router.push({ path: '/edge-compute', query: { tab: 'metrics' } })">
              <div class="edge-stats">
                <div class="edge-stat-item">
                  <div class="edge-stat-label">规则数</div>

                  <div class="edge-stat-value">{{ edgeRules.rule_count || 0 }}</div>
                </div>

                <div class="edge-stat-item">
                  <div class="edge-stat-label">已触发</div>

                  <div class="edge-stat-value primary">{{ edgeRules.rules_triggered || 0 }}</div>
                </div>

                <div class="edge-stat-item">
                  <div class="edge-stat-label">已执行</div>

                  <div class="edge-stat-value success">{{ edgeRules.rules_executed || 0 }}</div>
                </div>

                <div class="edge-stat-item">
                  <div class="edge-stat-label">工作池负载</div>

                  <div class="edge-stat-bar">
                    <div class="edge-progress" :style="{ width: getWorkerPoolPercent() + '%' }"></div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>



<script setup>

import { ref, onMounted, onUnmounted, computed } from 'vue'

import request from '@/utils/request'

import ScanEngineSoakPanel from '@/components/dashboard/ScanEngineSoakPanel.vue'

import { formatProtocolTag } from '@/utils/protocolLabel'

import {

  IconRefresh,

  IconApps, IconLink, IconSettings, IconTool,

  IconArrowRight

} from '@arco-design/web-vue/es/icon'



const system = ref({

    cpu_usage: 0,

    memory_usage: 0,

    disk_usage: 0,

    goroutines: 0

})

const channels = ref([])

const northbound = ref([])

const edgeRules = ref({})

const dashboardError = ref('')

const firstLoaded = ref(false)



let timer = null



const totalOnlineDevices = computed(() => {

  return channels.value.reduce((sum, ch) => sum + (ch.online_count || 0), 0)

})



const totalOfflineDevices = computed(() => {

  return channels.value.reduce((sum, ch) => sum + (ch.offline_count || 0), 0)

})



const getCpuColor = (val) => {

  if (val >= 80) return '#ef4444'

  if (val >= 60) return '#f59e0b'

  return '#6366f1'

}



const getMemoryColor = (val) => {

  const pct = getMemoryPercent(val)

  if (pct >= 80) return '#ef4444'

  if (pct >= 60) return '#f59e0b'

  return '#3b82f6'

}



const getDiskColor = (val) => {

  if (val >= 80) return '#ef4444'

  if (val >= 60) return '#f59e0b'

  return '#f97316'

}



const formatMemory = (mb) => {

  if (!mb) return '0 MB'

  if (mb >= 1024) return (mb / 1024).toFixed(1) + ' GB'

  return Math.round(mb) + ' MB'

}



const getMemoryPercent = (mb) => {

  const total = 8 * 1024

  return Math.min(((mb || 0) / total) * 100, 100)

}



const formatPercent = (val) => {

  if (val === undefined || val === null) return '-'

  return (val * 100).toFixed(0) + '%'

}



const formatDuration = (ms) => {

  if (ms === undefined || ms === null) return '-'

  if (ms < 1) return '<1ms'

  if (ms < 1000) return ms.toFixed(2) + 'ms'

  return (ms / 1000).toFixed(2) + 's'

}



const getWorkerPoolPercent = () => {

  const usage = edgeRules.value.worker_pool_usage || 0

  const size = edgeRules.value.worker_pool_size || 1

  return Math.min((usage / size) * 100, 100)

}



const getProtocolClass = (protocol) => {

  const classes = {

    'modbus-tcp': 'protocol-tcp',

    'modbus-rtu': 'protocol-rtu',

    'modbus-rtu-over-tcp': 'protocol-tcp',

    'bacnet-ip': 'protocol-bacnet',

    'opc-ua': 'protocol-opc',

    's7': 'protocol-s7',

    'profinet-io': 'protocol-profinet-io',

    'ethernet-ip': 'protocol-ip',

    'mitsubishi-slmp': 'protocol-mitsubishi',

    'omron-fins': 'protocol-omron'

  }

  return classes[protocol] || 'protocol-default'

}



const getQualityClass = (score) => {

  if (score === undefined || score === null || score === 0) return 'quality-none'

  if (score === 100) return 'quality-perfect'

  if (score >= 90) return 'quality-good'

  if (score >= 80) return 'quality-fair'

  return 'quality-poor'

}



const getQualityBarClass = (score) => {

  if (score === undefined || score === null || score === 0) return 'bar-none'

  if (score === 100) return 'bar-perfect'

  if (score >= 90) return 'bar-good'

  if (score >= 80) return 'bar-fair'

  return 'bar-poor'

}



const getSuccessRateClass = (rate) => {

  if (!rate && rate !== 0) return ''

  if (rate >= 0.99) return 'success'

  if (rate >= 0.95) return 'warning'

  return 'error'

}



const fetchData = async () => {

    try {

        const data = await request.get('/api/dashboard/summary')

        const sys = data.system || {}

        system.value = {

            cpu_usage: Number(sys.cpu_usage) || 0,

            memory_usage: Number(sys.memory_usage) || 0,

            disk_usage: Number(sys.disk_usage) || 0,

            goroutines: Number(sys.goroutines) || 0

        }

        dashboardError.value = ''

        firstLoaded.value = true



        channels.value = (data.channels || []).map(ch => {

          let qualityScore = 100

          if (ch.metrics) {

            const m = ch.metrics

            if (m.successRate !== undefined) qualityScore -= (1 - m.successRate) * 40

            if (m.crcErrorRate !== undefined) qualityScore -= m.crcErrorRate * 20

            if (m.retryRate !== undefined) qualityScore -= m.retryRate * 20

            qualityScore = Math.max(0, Math.round(qualityScore))

          }



          return {

            ...ch,

            qualityScore,

            successRate: ch.metrics?.successRate || ch.success_rate || 0

          }

        }).sort((a, b) => a.name.localeCompare(b.name))



        northbound.value = data.northbound || []

        edgeRules.value = data.edge_rules || {}

    } catch (e) {

        console.error(e)

        dashboardError.value = '系统资源数据加载失败'

    }

}



onMounted(() => {

    fetchData()

    timer = setInterval(fetchData, 2000)

})



onUnmounted(() => {

    if (timer) clearInterval(timer)

})

</script>



<style scoped>

/* v3.0 — styles in src/styles/dashboard.css */

</style>


