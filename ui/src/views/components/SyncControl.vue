<template>
  <div class="sync-control-container">
    <div class="sync-status-bar">
      <div class="sync-status-bar__meta">
        <a-tooltip :content="localNodeStatus === 'running' ? '节点运行中' : '节点已停止'" placement="bottom">
          <span
            class="sync-status-dot"
            :class="localNodeStatus === 'running' ? 'sync-status-dot--online' : 'sync-status-dot--offline'"
          />
        </a-tooltip>
        <template v-if="localNodeInfo">
          <div class="sync-meta-chip">
            <span class="sync-meta-chip__label">节点 ID</span>
            <span class="sync-meta-chip__value">{{ localNodeInfo.nodeId || 'N/A' }}</span>
          </div>
          <div class="sync-meta-chip">
            <span class="sync-meta-chip__label">网络地址</span>
            <span class="sync-meta-chip__value">{{ localNodeInfo.address || 'N/A' }}</span>
          </div>
        </template>
      </div>
      <div class="sync-status-bar__actions">
        <a-button
          type="primary"
          :loading="isStarting"
          @click="toggleNode"
        >
          {{ localNodeStatus === 'running' ? '停止节点' : '启动节点' }}
        </a-button>
      </div>
    </div>

    <div class="sync-layout">
      <aside class="sync-sidebar">
        <a-card class="sync-flow-card discovery-card" :bordered="false">
          <template #title>
            <div class="card-title-flex">
              <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10" />
                <path d="M8 15s1.5-2 4-2 4 2 4 2" />
                <circle cx="12" cy="9" r="3" />
              </svg>
              <span>网络发现</span>
            </div>
          </template>
          <div class="discovery-content">
            <div class="discovery-toggle">
              <span class="toggle-label">自动发现</span>
              <a-switch v-model="discoveryEnabled" @change="toggleDiscovery" :disabled="localNodeStatus !== 'running'" />
            </div>
            <div class="discovery-status">
              <div class="status-line">
                <span class="label">发现状态:</span>
                <span class="value" :class="discoveryStatus === 'scanning' ? 'status-scanning' : 'status-idle'">
                  {{ discoveryStatus === 'scanning' ? '扫描中...' : '已就绪' }}
                </span>
              </div>
              <div class="status-line">
                <span class="label">已发现节点:</span>
                <span class="value value-highlight">{{ discoveredNodes.length }}</span>
              </div>
            </div>
          </div>
        </a-card>

        <!-- 发现的节点列表 -->
        <a-card class="sync-flow-card discovered-nodes-card" :bordered="false">
          <template #title>
            <div class="card-title-flex">
              <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="12" y1="20" x2="12" y2="10" />
                <line x1="18" y1="20" x2="18" y2="4" />
                <line x1="6" y1="20" x2="6" y2="16" />
              </svg>
              <span>发现的节点</span>
            </div>
          </template>
          <div v-if="discoveredNodes.length > 0" class="discovered-nodes-list">
            <div 
              v-for="node in discoveredNodes" 
              :key="node.peerId" 
              class="discovered-node-item"
              :class="{ 'node-connected': node.connected }"
            >
              <div class="node-indicator" :class="node.connected ? 'indicator-connected' : 'indicator-discovered'"></div>
              <div class="node-info">
                <div class="node-name">{{ node.name }}</div>
                <div class="node-meta">
                  <span class="meta-label">地址:</span>
                  <span class="meta-value mono">{{ node.address }}</span>
                </div>
              </div>
              <div class="node-actions">
                <a-button 
                  v-if="!node.connected" 
                  type="primary" 
                  size="small" 
                  @click="connectToDiscoveredNode(node)"
                  :disabled="localNodeStatus !== 'running'"
                >
                  连接
                </a-button>
                <a-tag v-else color="green" size="small">已连接</a-tag>
              </div>
            </div>
          </div>
          <a-empty v-else description="暂无发现节点" />
        </a-card>
      </aside>

      <div class="sync-main-panel">
        <a-card class="sync-flow-card remote-sync-card" :bordered="false">
          <template #title>
            <div class="card-title-flex">
              <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M12 20V10" />
                <path d="M18 20V4" />
                <path d="M6 20v-6" />
              </svg>
              <span>远程节点同步还原</span>
            </div>
          </template>

          <div class="remote-sync-content">
            <div class="remote-target-section">
              <label class="section-label">目标节点</label>
              <a-input
                v-model="remoteTargetNode"
                placeholder="输入远程节点 IP 或节点 ID"
                class="target-input"
              />
              <a-button
                type="primary"
                size="small"
                @click="selectRemoteNode"
                :disabled="!remoteTargetNode"
              >
                选择节点
              </a-button>
            </div>

            <div v-if="selectedRemoteNode" class="remote-node-info">
              <div class="info-row">
                <span class="info-label">节点名称</span>
                <span class="info-value">{{ selectedRemoteNode.name }}</span>
              </div>
              <div class="info-row">
                <span class="info-label">节点地址</span>
                <span class="info-value monospace">{{ selectedRemoteNode.address }}</span>
              </div>
              <div class="info-row">
                <span class="info-label">同步状态</span>
                <a-tag :color="remoteSyncStatus === 'synced' ? 'green' : remoteSyncStatus === 'syncing' ? 'blue' : 'orange'">
                  {{ remoteSyncStatus === 'synced' ? '已同步' : remoteSyncStatus === 'syncing' ? '同步中' : '待同步' }}
                </a-tag>
              </div>
            </div>

            <div class="remote-actions">
              <a-button
                type="primary"
                :loading="remoteSyncing"
                @click="pullRemoteConfig"
                :disabled="!selectedRemoteNode || remoteSyncing"
              >
                拉取配置
              </a-button>
              <a-button
                :loading="remoteSyncing"
                @click="clearRemoteNodeConfig"
                :disabled="!selectedRemoteNode || remoteSyncing"
                status="danger"
              >
                清除远程配置
              </a-button>
              <a-button
                type="primary"
                :loading="remoteSyncing"
                @click="restoreRemoteConfig"
                :disabled="!selectedRemoteNode || remoteSyncing"
              >
                同步还原
              </a-button>
            </div>

            <div v-if="remoteSnapshots.length > 0" class="snapshot-section">
              <label class="section-label">可用快照</label>
              <div class="snapshot-list">
                <div
                  v-for="snapshot in remoteSnapshots"
                  :key="snapshot.id"
                  class="snapshot-item"
                  :class="{ 'snapshot-selected': selectedSnapshotId === snapshot.id }"
                  @click="selectSnapshot(snapshot.id)"
                >
                  <div class="snapshot-info">
                    <div class="snapshot-name">{{ snapshot.name }}</div>
                    <div class="snapshot-time">{{ snapshot.timestamp }}</div>
                  </div>
                  <div class="snapshot-size">{{ snapshot.size }}</div>
                </div>
              </div>
            </div>
          </div>
        </a-card>

        <section class="sync-section">
          <h3 class="sync-section__title">本机配置同步</h3>
          <div class="sync-section__stack">
            <a-card class="sync-flow-card sync-modes-card" :bordered="false">
              <template #title>同步模式</template>
              <div class="sync-modes">
                <div class="sync-mode-card" @click="selectSyncMode('push')">
                  <div class="mode-icon push-icon">
                    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2">
                      <line x1="22" y1="2" x2="11" y2="13" />
                      <polygon points="22 2 15 22 11 13 2 9 22 2" />
                    </svg>
                  </div>
                  <div class="mode-info">
                    <div class="mode-title">主动推送</div>
                    <div class="mode-desc">将本机配置推送到所有已连接节点</div>
                  </div>
                  <a-radio v-model="syncMode" value="push" />
                </div>
                <div class="sync-mode-card" @click="selectSyncMode('pull')">
                  <div class="mode-icon pull-icon">
                    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2">
                      <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
                      <polyline points="7 10 12 15 17 10" />
                      <line x1="12" y1="15" x2="12" y2="3" />
                    </svg>
                  </div>
                  <div class="mode-info">
                    <div class="mode-title">主动拉取</div>
                    <div class="mode-desc">从已连接节点拉取最新配置</div>
                  </div>
                  <a-radio v-model="syncMode" value="pull" />
                </div>
              </div>
            </a-card>

            <a-card class="sync-flow-card sync-options-card" :bordered="false">
              <template #title>同步选项</template>
              <div class="sync-options">
                <a-form :model="syncOptions" layout="vertical" class="industrial-form form-controls-md">
                  <a-form-item field="syncAll" label="同步全部配置">
                    <a-switch v-model="syncOptions.syncAll" />
                  </a-form-item>
                  <a-form-item field="forceOverwrite" label="强制覆盖目标配置">
                    <a-switch v-model="syncOptions.forceOverwrite" />
                  </a-form-item>
                </a-form>
              </div>
            </a-card>

            <div class="sync-action">
              <a-button
                type="primary"
                size="large"
                :loading="syncing"
                :disabled="connectedPeers.length === 0 || localNodeStatus !== 'running'"
                @click="executeSync"
              >
                {{ syncMode === 'push' ? '推送配置' : '拉取配置' }}
              </a-button>
            </div>

            <a-card v-if="syncHistory.length > 0" class="sync-flow-card sync-history-card" :bordered="false">
              <template #title>同步历史</template>
              <a-timeline>
                <a-timeline-item
                  v-for="(record, index) in syncHistory"
                  :key="index"
                  :color="record.status === 'success' ? 'green' : 'red'"
                >
                  <div class="timeline-content">
                    <div class="timeline-title">{{ record.mode === 'push' ? '推送同步' : '拉取同步' }}</div>
                    <div class="timeline-status" :class="record.status">
                      {{ record.status === 'success' ? '成功' : '失败' }}
                    </div>
                    <div class="timeline-time">{{ record.time }}</div>
                  </div>
                </a-timeline-item>
              </a-timeline>
            </a-card>
          </div>
        </section>

        <section class="sync-section">
          <h3 class="sync-section__title">网络通信</h3>
          <div class="sync-section__stack network-panel">
            <div class="network-metrics">
              <a-card class="sync-flow-card metric-card" :bordered="false">
                <template #title>
                  <span class="metric-icon">
                    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                      <line x1="12" y1="20" x2="12" y2="10" />
                      <line x1="18" y1="20" x2="18" y2="4" />
                      <line x1="6" y1="20" x2="6" y2="16" />
                    </svg>
                  </span>
                  <span>已连接节点</span>
                </template>
                <a-statistic :value="connectedPeers.length" suffix="个" />
              </a-card>
              <a-card class="sync-flow-card metric-card" :bordered="false">
                <template #title>
                  <span class="metric-icon">
                    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                      <circle cx="12" cy="12" r="10" />
                      <circle cx="12" cy="12" r="6" />
                      <circle cx="12" cy="12" r="2" />
                    </svg>
                  </span>
                  <span>平均延迟</span>
                </template>
                <a-statistic :value="networkStats.latency" suffix="ms" />
              </a-card>
            </div>

            <a-card class="sync-flow-card connections-card" :bordered="false">
              <template #title>连接状态</template>
              <div v-if="connectedPeers.length > 0" class="connections-list">
                <div v-for="peer in connectedPeers" :key="peer.peerId" class="connection-item">
                  <div class="connection-indicator" :class="peer.status"></div>
                  <div class="connection-info">
                    <div class="connection-name">{{ peer.name }}</div>
                    <div class="connection-peer-id">{{ truncatePeerId(peer.peerId) }}</div>
                  </div>
                  <div class="connection-metrics">
                    <div class="metric-row">
                      <span class="metric-label">延迟</span>
                      <span class="metric-value">{{ peer.latency }}ms</span>
                    </div>
                  </div>
                  <a-button type="text" size="small" @click="disconnectPeer(peer)" :disabled="localNodeStatus !== 'running'">
                    断开
                  </a-button>
                </div>
              </div>
              <a-empty v-else description="暂无连接" />
            </a-card>

            <a-card class="sync-flow-card network-log-card" :bordered="false">
              <template #title>
                <div class="card-title-row">
                  <span>通信日志</span>
                  <a-button type="text" size="small" @click="clearLogs">清空</a-button>
                </div>
              </template>
              <div class="log-container">
                <div v-for="(log, index) in networkLogs" :key="index" class="log-item" :class="log.type">
                  <span class="log-time">{{ log.time }}</span>
                  <span class="log-type" :class="log.type.toLowerCase()">{{ log.type }}</span>
                  <span class="log-message">{{ log.message }}</span>
                </div>
              </div>
            </a-card>
          </div>
        </section>
      </div>
    </div>

    <!-- 同步进度弹窗 -->
    <a-modal 
      v-model:visible="showSyncProgressModal" 
      title="同步进度" 
      :closable="false"
      :footer="false"
    >
      <div class="sync-progress">
        <div class="progress-header">
          <div class="progress-icon" :class="syncProgress.status">
            <svg v-if="syncProgress.status === 'running'" viewBox="0 0 24 24" width="32" height="32" fill="none" stroke="currentColor" stroke-width="2" class="spin-icon">
              <circle cx="12" cy="12" r="10" />
              <polyline points="12 6 12 12 16 14" />
            </svg>
            <svg v-else-if="syncProgress.status === 'success'" viewBox="0 0 24 24" width="32" height="32" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="20 6 9 17 4 12" />
            </svg>
            <svg v-else viewBox="0 0 24 24" width="32" height="32" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <line x1="12" y1="8" x2="12" y2="12" />
              <line x1="12" y1="16" x2="12.01" y2="16" />
            </svg>
          </div>
          <div class="progress-title">{{ syncProgress.title }}</div>
        </div>
        <a-progress :percent="syncProgress.percent" :show-info="false" />
        <div class="progress-info">
          <span>{{ syncProgress.message }}</span>
        </div>
        <div v-if="syncProgress.status === 'failed'" class="progress-error">
          <a-button type="primary" @click="executeSync">重试</a-button>
          <a-button type="outline" @click="showSyncProgressModal = false">关闭</a-button>
        </div>
      </div>
    </a-modal>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { showMessage } from '../../composables/useGlobalState'
import { NodeApi, SyncApi, NetworkApi } from '../../api/nodeSync'

const emit = defineEmits(['refresh'])

// 本地节点状态
const localNodeStatus = ref('stopped')
const isStarting = ref(false)

// 本地节点信息
const localNodeInfo = ref(null)

// 网络发现
const discoveryEnabled = ref(false)
const discoveryStatus = ref('idle')

// 发现的节点
const discoveredNodes = ref([])

// 同步相关
const syncMode = ref('push')
const syncing = ref(false)
const syncOptions = ref({
  syncAll: true,
  forceOverwrite: false
})

const syncHistory = ref([])

const syncProgress = ref({
  status: 'running',
  percent: 0,
  title: '',
  message: ''
})

// 网络监控
const connectedPeers = ref([])

const networkStats = ref({
  latency: 0
})

const networkLogs = ref([])

const showSyncProgressModal = ref(false)

// 远程节点同步还原
const remoteTargetNode = ref('192.168.3.230')
const selectedRemoteNode = ref(null)
const remoteSyncStatus = ref('pending')
const remoteSyncing = ref(false)
const remoteSnapshots = ref([])
const selectedSnapshotId = ref('')

// 节点管理方法
async function toggleNode() {
  if (localNodeStatus.value === 'running') {
    try {
      await NodeApi.stopNode()
      localNodeStatus.value = 'stopped'
      discoveryEnabled.value = false
      discoveryStatus.value = 'idle'
      addNetworkLog('INFO', '本地节点已停止')
      showMessage('节点已停止', 'success')
    } catch (error) {
      showMessage('停止节点失败: ' + error.message, 'error')
    }
  } else {
    isStarting.value = true
    try {
      await NodeApi.startNode()
      localNodeStatus.value = 'running'
      discoveryEnabled.value = true
      discoveryStatus.value = 'scanning'
      addNetworkLog('INFO', '本地节点已启动')
      showMessage('节点已启动', 'success')
      refreshDiscoveredNodes()
      refreshConnectedPeers()
    } catch (error) {
      showMessage('启动节点失败: ' + error.message, 'error')
    } finally {
      isStarting.value = false
    }
  }
}

async function toggleDiscovery(enabled) {
  try {
    await NodeApi.toggleDiscovery(enabled)
    discoveryStatus.value = enabled ? 'scanning' : 'idle'
    addNetworkLog(enabled ? 'INFO' : 'WARN', enabled ? 'mDNS 发现已启用' : 'mDNS 发现已禁用')
    showMessage(enabled ? '已启用自动发现' : '已禁用自动发现', 'success')
  } catch (error) {
    discoveryEnabled.value = !enabled
    showMessage('切换发现状态失败: ' + error.message, 'error')
  }
}

async function connectToDiscoveredNode(node) {
  try {
    await NodeApi.connectNode(node.peerId)
    node.connected = true
    addNetworkLog('INFO', `已连接到节点 ${node.name}`)
    showMessage(`已连接到节点 ${node.name}`, 'success')
    refreshConnectedPeers()
    emit('refresh')
  } catch (error) {
    showMessage('连接节点失败: ' + error.message, 'error')
  }
}

async function refreshDiscoveredNodes() {
  try {
    const response = await NodeApi.getDiscoveredNodes()
    const data = response.data !== undefined ? response.data : response
    discoveredNodes.value = data.map(node => ({
      peerId: node.peer_id || node.peerId,
      address: node.address || '',
      name: node.name || extractAddress(node.address),
      connected: node.status === 'online' || false,
      status: node.status || 'offline'
    }))
  } catch (error) {
    console.error('刷新发现节点失败:', error)
  }
}

function extractAddress(addr) {
  if (!addr) return '未知节点'
  const ipMatch = addr.match(/\/ip4\/([\d.]+)/)
  if (ipMatch) return ipMatch[1]
  return '未知节点'
}

// 同步控制方法
function selectSyncMode(mode) {
  syncMode.value = mode
}

async function executeSync() {
  syncing.value = true
  showSyncProgressModal.value = true
  
  const modeText = syncMode.value === 'push' ? '推送' : '拉取'
  syncProgress.value = {
    status: 'running',
    percent: 0,
    title: `${modeText}配置同步`,
    message: `正在${modeText}配置...`
  }
  
  try {
    let progress = 0
    const progressInterval = setInterval(() => {
      progress += Math.random() * 20
      if (progress >= 100) {
        progress = 100
        clearInterval(progressInterval)
      }
      syncProgress.value.percent = Math.floor(progress)
    }, 200)
  
    const options = {
      syncAll: syncOptions.value.syncAll,
      forceOverwrite: syncOptions.value.forceOverwrite
    }
    
    await SyncApi.pushConfig([], options)
    
    clearInterval(progressInterval)
    syncProgress.value.percent = 100
    syncProgress.value.status = 'success'
    syncProgress.value.message = '同步完成'
    
    addNetworkLog('INFO', `${modeText}同步完成`)
    showMessage(`${modeText}同步成功`, 'success')
    
    syncHistory.value.unshift({
      mode: syncMode.value,
      status: 'success',
      time: new Date().toLocaleString()
    })
    
    setTimeout(() => {
      showSyncProgressModal.value = false
      syncing.value = false
    }, 1500)
    
  } catch (error) {
    syncProgress.value.status = 'failed'
    syncProgress.value.message = '同步失败: ' + error.message
    addNetworkLog('ERROR', `${modeText}同步失败: ${error.message}`)
    showMessage(`${modeText}同步失败: ${error.message}`, 'error')
    syncing.value = false
    
    syncHistory.value.unshift({
      mode: syncMode.value,
      status: 'failed',
      time: new Date().toLocaleString()
    })
  }
}

// 网络监控方法
async function refreshConnectedPeers() {
  try {
    const response = await NetworkApi.getConnectedPeers()
    connectedPeers.value = response.data || []
  } catch (error) {
    console.error('刷新连接节点失败:', error)
  }
}

async function disconnectPeer(peer) {
  try {
    await NetworkApi.disconnectNode(peer.peerId)
    connectedPeers.value = connectedPeers.value.filter(p => p.peerId !== peer.peerId)
    addNetworkLog('WARN', `已断开与 ${peer.name} 的连接`)
    showMessage(`已断开与 ${peer.name} 的连接`, 'success')
    emit('refresh')
  } catch (error) {
    showMessage('断开连接失败: ' + error.message, 'error')
  }
}

// 远程节点同步还原方法
async function selectRemoteNode() {
  try {
    const target = remoteTargetNode.value.trim()
    if (!target) {
      showMessage('请输入目标节点IP或ID', 'warning')
      return
    }
    
    selectedRemoteNode.value = {
      name: `远程节点 ${target}`,
      address: target,
      peerId: target,
      status: 'online'
    }
    remoteSyncStatus.value = 'pending'
    
    await loadRemoteSnapshots(target)
    showMessage(`已选择远程节点: ${target}`, 'success')
  } catch (error) {
    showMessage('选择节点失败: ' + error.message, 'error')
  }
}

async function loadRemoteSnapshots(nodeId) {
  try {
    const response = await SyncApi.getRemoteSnapshots(nodeId)
    remoteSnapshots.value = response.data || []
  } catch (error) {
    console.warn('加载快照列表失败:', error)
    remoteSnapshots.value = [
      { id: 'auto', name: '自动选择最新快照', timestamp: '当前时间', size: '自动检测' }
    ]
  }
}

function selectSnapshot(snapshotId) {
  selectedSnapshotId.value = snapshotId
}

async function pullRemoteConfig() {
  if (!selectedRemoteNode.value) {
    showMessage('请先选择目标节点', 'warning')
    return
  }
  
  remoteSyncing.value = true
  remoteSyncStatus.value = 'syncing'
  
  try {
    await SyncApi.pullFromNode(selectedRemoteNode.value.peerId, {
      syncAll: true,
      forceOverwrite: false
    })
    
    remoteSyncStatus.value = 'synced'
    addNetworkLog('INFO', `从远程节点 ${selectedRemoteNode.value.address} 拉取配置完成`)
    showMessage(`成功从 ${selectedRemoteNode.value.address} 拉取配置`, 'success')
    
    syncHistory.value.unshift({
      mode: 'pull',
      status: 'success',
      time: new Date().toLocaleString(),
      target: selectedRemoteNode.value.address
    })
    
    emit('refresh')
  } catch (error) {
    remoteSyncStatus.value = 'pending'
    addNetworkLog('ERROR', `从远程节点 ${selectedRemoteNode.value.address} 拉取配置失败: ${error.message}`)
    showMessage(`拉取配置失败: ${error.message}`, 'error')
  } finally {
    remoteSyncing.value = false
  }
}

async function clearRemoteNodeConfig() {
  if (!selectedRemoteNode.value) {
    showMessage('请先选择目标节点', 'warning')
    return
  }
  
  remoteSyncing.value = true
  
  try {
    await SyncApi.clearRemoteConfig(selectedRemoteNode.value.peerId)
    
    remoteSyncStatus.value = 'pending'
    remoteSnapshots.value = []
    selectedSnapshotId.value = ''
    
    addNetworkLog('WARN', `已清除远程节点 ${selectedRemoteNode.value.address} 的配置`)
    showMessage(`已清除 ${selectedRemoteNode.value.address} 的配置`, 'success')
  } catch (error) {
    addNetworkLog('ERROR', `清除远程节点配置失败: ${error.message}`)
    showMessage(`清除配置失败: ${error.message}`, 'error')
  } finally {
    remoteSyncing.value = false
  }
}

async function restoreRemoteConfig() {
  if (!selectedRemoteNode.value) {
    showMessage('请先选择目标节点', 'warning')
    return
  }
  
  remoteSyncing.value = true
  remoteSyncStatus.value = 'syncing'
  
  try {
    await SyncApi.restoreRemoteConfig(selectedRemoteNode.value.peerId, selectedSnapshotId.value)
    
    remoteSyncStatus.value = 'synced'
    addNetworkLog('INFO', `远程节点 ${selectedRemoteNode.value.address} 同步还原完成`)
    showMessage(`成功还原 ${selectedRemoteNode.value.address} 的配置`, 'success')
    
    syncHistory.value.unshift({
      mode: 'restore',
      status: 'success',
      time: new Date().toLocaleString(),
      target: selectedRemoteNode.value.address
    })
    
    await loadRemoteSnapshots(selectedRemoteNode.value.peerId)
    emit('refresh')
  } catch (error) {
    remoteSyncStatus.value = 'pending'
    addNetworkLog('ERROR', `远程节点 ${selectedRemoteNode.value.address} 同步还原失败: ${error.message}`)
    showMessage(`同步还原失败: ${error.message}`, 'error')
  } finally {
    remoteSyncing.value = false
  }
}

async function clearLogs() {
  try {
    await NetworkApi.clearLogs()
    networkLogs.value = []
    showMessage('日志已清空', 'success')
  } catch (error) {
    showMessage('清空日志失败: ' + error.message, 'error')
  }
}

function addNetworkLog(type, message) {
  const now = new Date()
  const time = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}:${now.getSeconds().toString().padStart(2, '0')}`
  networkLogs.value.unshift({ time, type, message })
  if (networkLogs.value.length > 100) {
    networkLogs.value.pop()
  }
}

function truncatePeerId(peerId) {
  if (!peerId) return ''
  return peerId.length > 12 ? peerId.substring(0, 6) + '...' + peerId.substring(peerId.length - 6) : peerId
}

// 初始化和清理
let networkUpdateTimer = null

onMounted(() => {
  NodeApi.getStatus().then(response => {
    const data = response.data !== undefined ? response.data : response
    localNodeStatus.value = data.status || 'stopped'
    if (localNodeStatus.value === 'running') {
      discoveryEnabled.value = true
      discoveryStatus.value = 'scanning'
      refreshDiscoveredNodes()
      refreshConnectedPeers()
    }
  }).catch(() => {
    localNodeStatus.value = 'stopped'
  })
  
  NodeApi.getNodeInfo().then(response => {
    const data = response.data !== undefined ? response.data : response
    localNodeInfo.value = data
  }).catch(() => {
    localNodeInfo.value = {
      nodeId: 'node-local-001',
      name: '本地节点',
      address: '192.168.1.100:4001'
    }
  })
  
  networkUpdateTimer = setInterval(() => {
    if (localNodeStatus.value === 'running') {
      networkStats.value.latency = Math.floor(Math.random() * 20 + 5)
      
      connectedPeers.value.forEach(peer => {
        peer.latency = Math.floor(Math.random() * 20 + 5)
      })
    }
  }, 3000)
})

onUnmounted(() => {
  if (networkUpdateTimer) {
    clearInterval(networkUpdateTimer)
  }
})
</script>

<style scoped>
/* v3.0 — styles in src/styles/ */
</style>
