<template>
  <div class="config-diff-container">
    <div class="diff-header">
      <span class="sync-tab-toolbar__hint">选择源节点与目标节点，对比配置差异</span>
      <div class="header-right">
        <a-space size="middle">
          <a-select 
            v-model="sourceNode" 
            placeholder="选择源节点"
            class="node-select"
          >
            <a-option value="local">本地节点</a-option>
            <a-option value="node-a">边缘节点-A</a-option>
            <a-option value="node-b">边缘节点-B</a-option>
            <a-option value="node-c">边缘节点-C</a-option>
          </a-select>
          
          <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" class="swap-icon">
            <polyline points="9 18 15 12 9 6" />
          </svg>
          
          <a-select 
            v-model="targetNode" 
            placeholder="选择目标节点"
            class="node-select"
          >
            <a-option value="local">本地节点</a-option>
            <a-option value="node-a">边缘节点-A</a-option>
            <a-option value="node-b">边缘节点-B</a-option>
            <a-option value="node-c">边缘节点-C</a-option>
          </a-select>
          
          <a-button type="primary" @click="compareConfigs">
            <template #icon>
              <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M9 18h6" />
                <path d="M12 15h-2v4h2v-4z" />
                <path d="M5 9h14" />
                <path d="M12 6h-2v4h2V6z" />
                <path d="M5 2h14" />
                <path d="M12 -1h-2v4h2V-1z" />
              </svg>
            </template>
            对比
          </a-button>
        </a-space>
      </div>
    </div>

    <!-- 配置选择 -->
    <div class="config-select-panel">
      <div class="panel-header">
        <span>选择配置文件</span>
        <a-button type="text" size="small" @click="selectAllConfigs">全选</a-button>
      </div>
      <div class="config-list">
        <label 
          v-for="config in configFiles" 
          :key="config.name" 
          class="config-checkbox"
        >
          <a-checkbox v-model="config.selected" />
          <span class="config-name">{{ config.name }}</span>
          <a-tag :color="config.type === 'device' ? 'blue' : config.type === 'network' ? 'green' : 'orange'" size="small">
            {{ config.type === 'device' ? '设备' : config.type === 'network' ? '网络' : '系统' }}
          </a-tag>
        </label>
      </div>
    </div>

    <!-- 差异结果 -->
    <div class="diff-results" v-if="showDiff">
      <div class="results-header">
        <span class="results-title">差异对比结果</span>
        <div class="results-summary">
          <span class="summary-item">
            <span class="summary-label">相同:</span>
            <span class="summary-value same">{{ diffStats.same }}</span>
          </span>
          <span class="summary-item">
            <span class="summary-label">不同:</span>
            <span class="summary-value different">{{ diffStats.different }}</span>
          </span>
          <span class="summary-item">
            <span class="summary-label">缺失:</span>
            <span class="summary-value missing">{{ diffStats.missing }}</span>
          </span>
        </div>
      </div>

      <!-- 差异列表 -->
      <div class="diff-list">
        <a-card 
          v-for="diff in diffResults" 
          :key="diff.name" 
          class="diff-card"
          :class="diff.status"
        >
          <template #title>
            <div class="diff-title-row">
              <a-tag :color="getStatusColor(diff.status)">
                {{ getStatusText(diff.status) }}
              </a-tag>
              <span class="diff-name">{{ diff.name }}</span>
            </div>
          </template>
          
          <div class="diff-content">
            <div class="diff-panel source-panel">
              <div class="panel-label">{{ sourceNodeLabel }}</div>
              <pre class="diff-code">{{ diff.source }}</pre>
            </div>
            
            <div class="diff-arrow">
              <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M12 5v14M5 12l7 7 7-7" />
              </svg>
            </div>
            
            <div class="diff-panel target-panel">
              <div class="panel-label">{{ targetNodeLabel }}</div>
              <pre class="diff-code">{{ diff.target }}</pre>
            </div>
          </div>
          
          <div class="diff-actions">
            <a-button type="primary" size="small" @click="syncConfig(diff)">
              <template #icon>
                <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9" />
                </svg>
              </template>
              同步
            </a-button>
          </div>
        </a-card>
      </div>
    </div>

    <!-- 空状态 -->
    <div v-else class="empty-state">
      <div class="empty-icon">
        <svg viewBox="0 0 24 24" width="48" height="48" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M9 18h6" />
          <path d="M12 15h-2v4h2v-4z" />
          <path d="M5 9h14" />
          <path d="M12 6h-2v4h2V6z" />
        </svg>
      </div>
      <div class="empty-title">选择节点开始对比</div>
      <div class="empty-desc">从上方选择源节点和目标节点，点击对比查看配置差异</div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'

const sourceNode = ref('local')
const targetNode = ref('node-a')
const showDiff = ref(false)

// 配置文件列表
const configFiles = ref([
  { name: 'sensor-a.yaml', type: 'device', selected: true },
  { name: 'sensor-b.yaml', type: 'device', selected: false },
  { name: 'gateway.yaml', type: 'device', selected: true },
  { name: 'mqtt.yaml', type: 'network', selected: false },
  { name: 'sync.yaml', type: 'system', selected: false }
])

// 差异统计
const diffStats = ref({
  same: 1,
  different: 2,
  missing: 0
})

// 节点标签映射
const nodeLabels = {
  'local': '本地节点',
  'node-a': '边缘节点-A',
  'node-b': '边缘节点-B',
  'node-c': '边缘节点-C'
}

const sourceNodeLabel = computed(() => nodeLabels[sourceNode.value] || sourceNode.value)
const targetNodeLabel = computed(() => nodeLabels[targetNode.value] || targetNode.value)

// 差异结果
const diffResults = ref([
  {
    name: 'sensor-a.yaml',
    status: 'different',
    source: `device_id: sensor-001
device_name: 温度传感器A
interval: 5000
enabled: true`,
    target: `device_id: sensor-001
device_name: 温度传感器A
interval: 10000
enabled: true`
  },
  {
    name: 'gateway.yaml',
    status: 'different',
    source: `gateway_id: gw-001
name: Main Gateway
protocol: mqtt
port: 1883
timeout: 30`,
    target: `gateway_id: gw-001
name: Main Gateway
protocol: http
port: 8080
timeout: 60`
  },
  {
    name: 'mqtt.yaml',
    status: 'same',
    source: `broker: localhost
port: 1883
username: admin
password: secret
keepalive: 60`,
    target: `broker: localhost
port: 1883
username: admin
password: secret
keepalive: 60`
  }
])

// 方法
function selectAllConfigs() {
  configFiles.value.forEach(c => c.selected = true)
}

function compareConfigs() {
  if (sourceNode.value === targetNode.value) {
    console.log('请选择不同的节点进行对比')
    return
  }
  showDiff.value = true
}

function getStatusColor(status) {
  switch (status) {
    case 'same': return 'green'
    case 'different': return 'orange'
    case 'missing': return 'red'
    default: return 'gray'
  }
}

function getStatusText(status) {
  switch (status) {
    case 'same': return '相同'
    case 'different': return '存在差异'
    case 'missing': return '目标缺失'
    default: return '未知'
  }
}

function syncConfig(diff) {
  console.log('Syncing config:', diff.name)
}
</script>

<style scoped>
/* v3.0 — styles in src/styles/ */
</style>
