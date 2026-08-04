<template>
  <div class="node-tree-container">
    <div class="sync-tab-toolbar">
      <span class="sync-tab-toolbar__hint">浏览各节点配置树，支持单项同步</span>
      <div class="sync-tab-toolbar__actions">
        <a-button type="outline" size="small" @click="refreshTree">刷新</a-button>
        <a-button type="primary" size="small" @click="syncAll">同步全部</a-button>
      </div>
    </div>

    <div class="tree-layout">
      <!-- 左侧：节点选择 + 配置树 -->
      <aside class="sidebar sync-tree-sidebar">
        <div class="sync-tree-card node-selector">
          <div class="selector-header">
            <span>已发现节点</span>
            <span class="node-count">{{ nodes.length }}</span>
          </div>
          <div class="node-list">
            <div 
              v-for="node in nodes" 
              :key="node.id"
              class="node-item"
              :class="{ 
                active: selectedNode?.id === node.id,
                'is-local': node.isLocal 
              }"
              @click="selectNode(node)"
            >
              <div class="node-status-dot" :class="node.status"></div>
              <div class="node-info">
                <span class="node-name">{{ node.name }}</span>
                <span class="node-address">{{ node.address }}</span>
              </div>
              <span v-if="node.isLocal" class="local-tag">本节点</span>
            </div>
          </div>
        </div>

        <div class="sync-tree-card config-tree">
          <div class="tree-header-text">
            <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 16V8a2 2 0 00-1-1.73l-7-4a2 2 0 00-2 0l-7 4A2 2 0 003 8v8a2 2 0 001 1.73l7 4a2 2 0 002 0l7-4A2 2 0 0021 16z" />
              <polyline points="3.27 6.96 12 12.01 20.73 6.96" />
              <line x1="12" y1="22.08" x2="12" y2="12" />
            </svg>
            {{ selectedNode?.name || '选择节点' }}
          </div>
          <div class="tree-scroll">
            <div v-if="selectedNode" class="tree-content">
              <!-- Gateway Root -->
              <div class="tree-node root">
                <div class="tree-node-header" @click="toggleExpand('root')">
                  <svg 
                    viewBox="0 0 24 24" 
                    width="14" 
                    height="14" 
                    fill="none" 
                    stroke="currentColor" 
                    stroke-width="2"
                    :class="{ rotated: expandedNodes.includes('root') }"
                  >
                    <polyline points="5 9 12 15 19 9" />
                  </svg>
                  <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
                    <circle cx="12" cy="12" r="10" />
                    <path d="M8 15s1.5-2 4-2 4 2 4 2" />
                    <circle cx="12" cy="9" r="3" />
                  </svg>
                  <span>{{ selectedNode.name }}</span>
                  <span class="node-status-badge" :class="selectedNode.status">
                    {{ statusText(selectedNode.status) }}
                  </span>
                </div>

                <!-- Channels -->
                <div v-if="expandedNodes.includes('root')" class="tree-children">
                  <!-- Channels Section -->
                  <div class="tree-node section">
                    <div class="tree-node-header" @click="toggleExpand('channels')">
                      <svg 
                        viewBox="0 0 24 24" 
                        width="12" 
                        height="12" 
                        fill="none" 
                        stroke="currentColor" 
                        stroke-width="2"
                        :class="{ rotated: expandedNodes.includes('channels') }"
                      >
                        <polyline points="5 9 12 15 19 9" />
                      </svg>
                      <svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
                        <rect x="3" y="3" width="7" height="7" />
                        <rect x="14" y="3" width="7" height="7" />
                        <rect x="14" y="14" width="7" height="7" />
                        <rect x="3" y="14" width="7" height="7" />
                      </svg>
                      <span>通道</span>
                      <span class="count-badge">{{ treeData.channels?.length || 0 }}</span>
                    </div>

                    <!-- Channel List -->
                    <div v-if="expandedNodes.includes('channels')" class="tree-children">
                      <div 
                        v-for="channel in treeData.channels" 
                        :key="channel.id"
                        class="tree-node item channel"
                        :class="{ 
                          selected: selectedItem?.type === 'channel' && selectedItem?.id === channel.id,
                          'has-diff': channel.hasDiff 
                        }"
                        @click="selectTreeItem('channel', channel)"
                      >
                        <div class="tree-node-row">
                          <svg 
                            v-if="channel.deviceCount > 0" 
                            viewBox="0 0 24 24" 
                            width="10" 
                            height="10" 
                            fill="none" 
                            stroke="currentColor" 
                            stroke-width="2"
                            :class="{ rotated: expandedNodes.includes('channel-' + channel.id) }"
                            @click.stop="loadChannelDevices(channel)"
                          >
                            <polyline points="5 9 12 15 19 9" />
                          </svg>
                          <span v-else class="spacer"></span>
                          <svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
                            <rect x="3" y="3" width="7" height="7" />
                            <rect x="14" y="3" width="7" height="7" />
                            <rect x="14" y="14" width="7" height="7" />
                            <rect x="3" y="14" width="7" height="7" />
                          </svg>
                          <span class="item-name">{{ channel.name }}</span>
                          <span v-if="channel.hasDiff" class="diff-badge">差异</span>
                          <span class="status-badge" :class="channel.status">{{ statusText(channel.status) }}</span>
                        </div>

                        <!-- Devices under Channel -->
                        <div v-if="expandedNodes.includes('channel-' + channel.id) && channel.devices" class="tree-children">
                          <div 
                            v-for="device in channel.devices" 
                            :key="device.id"
                            class="tree-node item device"
                            :class="{ 
                              selected: selectedItem?.type === 'device' && selectedItem?.id === device.id,
                              'has-diff': device.hasDiff 
                            }"
                            @click="selectTreeItem('device', device, channel)"
                          >
                            <div class="tree-node-row">
                              <svg 
                                v-if="device.pointCount > 0" 
                                viewBox="0 0 24 24" 
                                width="10" 
                                height="10" 
                                fill="none" 
                                stroke="currentColor" 
                                stroke-width="2"
                                :class="{ rotated: expandedNodes.includes('device-' + device.id) }"
                                @click.stop="loadDevicePoints(device, channel)"
                              >
                                <polyline points="5 9 12 15 19 9" />
                              </svg>
                              <span v-else class="spacer"></span>
                              <svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
                                <rect x="2" y="3" width="20" height="14" rx="2" />
                                <line x1="8" y1="21" x2="16" y2="21" />
                                <line x1="12" y1="17" x2="12" y2="21" />
                              </svg>
                              <span class="item-name">{{ device.name }}</span>
                              <span v-if="device.hasDiff" class="diff-badge">差异</span>
                              <span class="status-badge" :class="device.status">{{ statusText(device.status) }}</span>
                            </div>

                            <!-- Points under Device -->
                            <div v-if="expandedNodes.includes('device-' + device.id) && device.points" class="tree-children">
                              <div 
                                v-for="point in device.points" 
                                :key="point.id"
                                class="tree-node item point"
                                :class="{ 
                                  selected: selectedItem?.type === 'point' && selectedItem?.id === point.id,
                                  'has-diff': point.hasDiff,
                                  warning: point.status === 'warning'
                                }"
                                @click="selectTreeItem('point', point, device, channel)"
                              >
                                <svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2">
                                  <circle cx="12" cy="12" r="3" />
                                </svg>
                                <span class="item-name">{{ point.name }}</span>
                                <span class="point-address">{{ point.address }}</span>
                                <span v-if="point.hasDiff" class="diff-badge">差异</span>
                                <span v-if="point.status === 'warning'" class="warning-badge">
                                  <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" /><line x1="12" y1="9" x2="12" y2="13" /><line x1="12" y1="17" x2="12.01" y2="17" /></svg>
                                </span>
                              </div>
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
            <div v-else class="empty-tree">
              <svg viewBox="0 0 24 24" width="32" height="32" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10" />
                <polyline points="12 6 12 12 16 14" />
              </svg>
              <p>请选择一个节点</p>
            </div>
          </div>
        </div>
      </aside>

      <main class="detail-panel sync-tree-detail">
        <div v-if="selectedItem" class="detail-content">
          <!-- 详情头部 -->
          <div class="detail-header">
            <div class="header-left">
              <h3>{{ selectedItem.label }}</h3>
              <div class="breadcrumbs">
                <span v-for="(crumb, index) in breadcrumbs" :key="index">
                  <span v-if="index > 0">/</span>
                  <span :class="{ active: index === breadcrumbs.length - 1 }">{{ crumb }}</span>
                </span>
              </div>
            </div>
            <div class="header-right">
              <a-button
                type="outline"
                size="small"
                :class="{ 'is-active': viewMode === 'raw' }"
                @click="viewMode = viewMode === 'structured' ? 'raw' : 'structured'"
              >
                {{ viewMode === 'structured' ? '原始视图' : '结构视图' }}
              </a-button>
            </div>
          </div>

          <div class="detail-toolbar">
            <a-space wrap :size="8">
              <a-button type="outline" size="small" @click="viewItem">查看</a-button>
              <a-button type="outline" size="small" @click="compareItem">对比差异</a-button>
              <a-button type="primary" size="small" :disabled="!selectedItem.hasDiff" @click="syncItem">同步</a-button>
              <a-button type="outline" size="small" @click="toggleLock">
                {{ selectedItem.locked ? '解锁' : '锁定' }}
              </a-button>
            </a-space>
          </div>

          <!-- 详情内容 -->
          <div class="detail-body">
            <div v-if="viewMode === 'structured'" class="structured-view">
              <div class="form-grid">
                <div 
                  v-for="(value, key) in selectedItem.content" 
                  :key="key"
                  class="form-item"
                >
                  <label>{{ formatKey(key) }}</label>
                  <span class="value">{{ formatValue(value) }}</span>
                </div>
              </div>
            </div>
            <div v-else class="raw-view">
              <pre class="yaml-content">{{ formatYaml(selectedItem.content) }}</pre>
            </div>
          </div>

          <div v-if="selectedItem.hasDiff" class="diff-notification">
            <span class="diff-notification__text">此配置与本地存在差异</span>
            <a-button type="primary" size="small" @click="syncItem">立即同步</a-button>
          </div>
        </div>
        <div v-else-if="selectedNode" class="node-overview">
          <div class="overview-header">
            <div class="node-icon">
              <svg viewBox="0 0 24 24" width="48" height="48" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10" />
                <path d="M8 15s1.5-2 4-2 4 2 4 2" />
                <circle cx="12" cy="9" r="3" />
              </svg>
            </div>
            <div class="node-info">
              <h2>{{ selectedNode.name }}</h2>
              <div class="info-row">
                <span class="label">NodeID</span>
                <span class="value mono">{{ truncateId(selectedNode.id) }}</span>
              </div>
              <div class="info-row">
                <span class="label">IP</span>
                <span class="value">{{ selectedNode.address }}</span>
              </div>
              <div class="info-row">
                <span class="label">版本</span>
                <span class="value">{{ selectedNode.version || 'N/A' }}</span>
              </div>
              <div class="info-row">
                <span class="label">延迟</span>
                <span class="value" :class="{ high: selectedNode.latency > 100 }">{{ selectedNode.latency || 0 }}ms</span>
              </div>
            </div>
            <div class="node-status-card" :class="selectedNode.status">
              <a-tag :color="selectedNode.status === 'online' ? 'green' : selectedNode.status === 'degraded' ? 'orangered' : 'red'" size="small">
                {{ statusText(selectedNode.status) }}
              </a-tag>
            </div>
          </div>
          <div class="overview-stats">
            <div class="stat-item">
              <span class="stat-value">{{ treeData.channels?.length || 0 }}</span>
              <span class="stat-label">通道数</span>
            </div>
            <div class="stat-item">
              <span class="stat-value">{{ totalDevices }}</span>
              <span class="stat-label">设备数</span>
            </div>
            <div class="stat-item">
              <span class="stat-value">{{ totalPoints }}</span>
              <span class="stat-label">点位总数</span>
            </div>
            <div class="stat-item diff">
              <span class="stat-value">{{ totalDiffs }}</span>
              <span class="stat-label">配置差异</span>
            </div>
          </div>
          <div class="overview-actions">
            <a-space :size="12">
              <a-button type="primary" @click="syncAll">同步全部配置</a-button>
              <a-button type="outline" @click="viewDiff">查看差异</a-button>
            </a-space>
          </div>
        </div>
        <div v-else class="empty-detail">
          <svg viewBox="0 0 24 24" width="48" height="48" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10" />
            <polyline points="12 6 12 12 16 14" />
          </svg>
          <p>选择左侧配置项查看详情</p>
        </div>
      </main>
    </div>

    <a-modal
      v-model:visible="infoDialog.visible"
      :title="infoDialog.title"
      ok-text="确定"
      hide-cancel
      @ok="infoDialog.visible = false"
    >
      <p>{{ infoDialog.message }}</p>
    </a-modal>
  </div>
</template>

<script setup>
import { ref, computed, reactive } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()

// 节点列表
const nodes = ref([
  { id: 'local-' + Date.now(), name: 'Gateway-A', address: '192.168.1.100', status: 'online', version: 'v1.2.3', latency: 12, isLocal: true },
  { id: 'QmSZ1CuR2t4eWHUeBywbs46MgbCLUBo7cTLgyRn6u6TMr4', name: 'Gateway-B', address: '192.168.1.101', status: 'online', version: 'v1.2.3', latency: 25, isLocal: false },
  { id: 'QmXYZ1234567890abcdefghijklmnopqrstuvwxyz', name: 'Gateway-C', address: '192.168.1.102', status: 'degraded', version: 'v1.2.2', latency: 156, isLocal: false },
  { id: 'QmABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890', name: 'Gateway-D', address: '192.168.1.103', status: 'offline', version: 'v1.2.1', latency: 0, isLocal: false }
])

// 当前选中的节点
const selectedNode = ref(nodes.value[0])

// 展开的节点
const expandedNodes = ref(['root', 'channels'])

// 当前选中的树项
const selectedItem = ref(null)

// 视图模式
const viewMode = ref('structured')

// 加载状态
const loading = ref(false)

// 树数据
const treeData = ref({
  channels: [
    {
      id: 'ch1',
      name: 'Modbus-1',
      protocol: 'modbus',
      status: 'online',
      hasDiff: false,
      deviceCount: 2,
      devices: null
    },
    {
      id: 'ch2',
      name: 'OPC-UA-1',
      protocol: 'opcua',
      status: 'online',
      hasDiff: true,
      deviceCount: 1,
      devices: null
    },
    {
      id: 'ch3',
      name: 'BACnet-1',
      protocol: 'bacnet',
      status: 'offline',
      hasDiff: false,
      deviceCount: 1,
      devices: null
    }
  ]
})

// 计算属性：总设备数
const totalDevices = computed(() => {
  return treeData.value.channels?.reduce((sum, ch) => sum + (ch.deviceCount || 0), 0) || 0
})

// 计算属性：总点数
const totalPoints = computed(() => {
  let count = 0
  treeData.value.channels?.forEach(ch => {
    ch.devices?.forEach(d => {
      count += d.pointCount || 0
    })
  })
  return count
})

// 计算属性：总差异数
const totalDiffs = computed(() => {
  let count = 0
  treeData.value.channels?.forEach(ch => {
    if (ch.hasDiff) count++
    ch.devices?.forEach(d => {
      if (d.hasDiff) count++
      d.points?.forEach(p => {
        if (p.hasDiff) count++
      })
    })
  })
  return count
})

// 面包屑
const breadcrumbs = computed(() => {
  if (!selectedItem.value) return []
  const crumbs = [selectedNode.value?.name || '']
  switch (selectedItem.value.type) {
    case 'channel':
      crumbs.push('通道', selectedItem.value.name)
      break
    case 'device':
      crumbs.push('通道', selectedItem.value.channelName, '设备', selectedItem.value.name)
      break
    case 'point':
      crumbs.push('通道', selectedItem.value.channelName, '设备', selectedItem.value.deviceName, '点位', selectedItem.value.name)
      break
  }
  return crumbs
})

function statusText(status) {
  const map = { online: '在线', offline: '离线', degraded: '延迟高', normal: '正常', warning: '警告' }
  return map[status] || status
}

function truncateId(id) {
  if (id && id.length > 12) {
    return id.slice(0, 6) + '...' + id.slice(-6)
  }
  return id
}

function toggleExpand(nodeId) {
  const index = expandedNodes.value.indexOf(nodeId)
  if (index === -1) {
    expandedNodes.value.push(nodeId)
  } else {
    expandedNodes.value.splice(index, 1)
  }
}

function selectNode(node) {
  selectedNode.value = node
  selectedItem.value = null
  expandedNodes.value = ['root', 'channels']
  loadNodeData(node.id)
}

function loadChannelDevices(channel) {
  const expandId = 'channel-' + channel.id
  const isExpanded = expandedNodes.value.includes(expandId)
  
  if (isExpanded && !channel.devices) {
    // 懒加载设备数据
    loading.value = true
    setTimeout(() => {
      const channelIndex = treeData.value.channels.findIndex(c => c.id === channel.id)
      if (channelIndex !== -1) {
        treeData.value.channels[channelIndex].devices = generateMockDevices(channel.name)
      }
      loading.value = false
    }, 500)
  }
  
  toggleExpand(expandId)
}

function loadDevicePoints(device, channel) {
  const expandId = 'device-' + device.id
  const isExpanded = expandedNodes.value.includes(expandId)
  
  if (isExpanded) {
    const channelIndex = treeData.value.channels.findIndex(c => c.id === channel.id)
    if (channelIndex !== -1) {
      const deviceIndex = treeData.value.channels[channelIndex].devices?.findIndex(d => d.id === device.id)
      if (deviceIndex !== -1 && !treeData.value.channels[channelIndex].devices[deviceIndex].points) {
        // 懒加载点位数据
        loading.value = true
        setTimeout(() => {
          treeData.value.channels[channelIndex].devices[deviceIndex].points = generateMockPoints(device.name)
          loading.value = false
        }, 300)
      }
    }
  }
  
  toggleExpand(expandId)
}

function generateMockDevices(channelName) {
  const devices = []
  const count = Math.floor(Math.random() * 3) + 1
  for (let i = 1; i <= count; i++) {
    devices.push({
      id: `dev-${channelName}-${i}`,
      name: `${channelName.split('-')[0]}-Device-${i}`,
      status: Math.random() > 0.1 ? 'online' : 'degraded',
      hasDiff: Math.random() > 0.7,
      pointCount: Math.floor(Math.random() * 20) + 5,
      points: null
    })
  }
  return devices
}

function generateMockPoints(deviceName) {
  const points = []
  const count = Math.floor(Math.random() * 10) + 3
  const pointNames = ['temp', 'pressure', 'flow', 'level', 'status', 'speed', 'voltage', 'current', 'power', 'frequency']
  for (let i = 0; i < count; i++) {
    points.push({
      id: `point-${deviceName}-${i}`,
      name: pointNames[i % pointNames.length],
      address: `4000${i + 1}`,
      type: ['float', 'int', 'bool', 'double'][Math.floor(Math.random() * 4)],
      scale: Math.random() * 10 + 0.1,
      unit: ['°C', 'kPa', 'L/min', '%', 'V', 'A', 'W'][Math.floor(Math.random() * 7)],
      status: Math.random() > 0.9 ? 'warning' : 'normal',
      hasDiff: Math.random() > 0.8
    })
  }
  return points
}

function selectTreeItem(type, item, parent = null, grandParent = null) {
  let content = {}
  let hasDiff = item.hasDiff || false
  let locked = false

  switch (type) {
    case 'channel':
      content = {
        id: item.id,
        name: item.name,
        protocol: item.protocol,
        status: item.status,
        deviceCount: item.deviceCount,
        hasDiff: item.hasDiff
      }
      break
    case 'device':
      content = {
        id: item.id,
        name: item.name,
        protocol: parent?.protocol || 'unknown',
        status: item.status,
        pointCount: item.pointCount,
        hasDiff: item.hasDiff
      }
      break
    case 'point':
      content = {
        id: item.id,
        name: item.name,
        address: item.address,
        type: item.type,
        scale: item.scale?.toFixed(2) || '-',
        unit: item.unit || '-',
        status: item.status,
        hasDiff: item.hasDiff
      }
      break
  }

  selectedItem.value = {
    type,
    id: item.id,
    name: item.name,
    label: item.name,
    channelName: grandParent?.name || parent?.name,
    deviceName: parent?.name,
    content,
    hasDiff,
    locked
  }
}

function formatKey(key) {
  const keyMap = {
    id: 'ID',
    name: '名称',
    protocol: '协议',
    status: '状态',
    deviceCount: '设备数量',
    pointCount: '点位数量',
    address: '地址',
    type: '类型',
    scale: '倍率',
    unit: '单位',
    enabled: '启用状态',
    hasDiff: '是否有差异'
  }
  return keyMap[key] || key
}

function formatValue(value) {
  if (typeof value === 'boolean') {
    return value ? '是' : '否'
  }
  return value
}

function formatYaml(obj, indent = 0) {
  let yaml = ''
  const prefix = ' '.repeat(indent)
  for (const [key, value] of Object.entries(obj)) {
    if (typeof value === 'object' && value !== null) {
      yaml += `${prefix}${key}:\n${formatYaml(value, indent + 2)}`
    } else {
      yaml += `${prefix}${key}: ${value}\n`
    }
  }
  return yaml
}

const infoDialog = reactive({
  visible: false,
  title: '',
  message: ''
})

function showInfo(title, message) {
  infoDialog.title = title
  infoDialog.message = message
  infoDialog.visible = true
}

function loadNodeData(nodeId) {
  console.log('Loading data for node:', nodeId)
}

function refreshTree() {
  loadNodeData(selectedNode.value?.id)
}

function syncAll() {
  showInfo('同步配置', `同步 ${selectedNode.value?.name} 的全部配置`)
}

function viewItem() {
  showInfo('配置详情', '查看配置详情')
}

function compareItem() {
  router.push('/config-diff')
}

function syncItem() {
  if (selectedItem.value?.hasDiff) {
    showInfo('同步配置', `同步 ${selectedItem.value.name}`)
  }
}

function viewDiff() {
  router.push('/config-diff')
}

function toggleLock() {
  if (selectedItem.value) {
    selectedItem.value.locked = !selectedItem.value.locked
  }
}
</script>

<style scoped>
/* v3.0 — styles in src/styles/ */
</style>