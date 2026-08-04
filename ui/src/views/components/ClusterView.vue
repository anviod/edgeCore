<template>
  <div class="cluster-view-container">
    <div class="sync-tab-toolbar">
      <span class="sync-tab-toolbar__hint">监控和管理集群节点状态</span>
      <a-button type="outline" size="small" @click="refreshCluster">
        <template #icon>
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="23 4 23 10 17 10" />
            <polyline points="1 20 1 14 7 14" />
            <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
          </svg>
        </template>
        刷新
      </a-button>
    </div>

    <!-- 统计卡片 -->
    <div class="stats-row">
      <a-card class="stat-card">
        <template #title>
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="12" y1="20" x2="12" y2="10" />
            <line x1="18" y1="20" x2="18" y2="4" />
            <line x1="6" y1="20" x2="6" y2="16" />
          </svg>
          <span>节点总数</span>
        </template>
        <a-statistic :value="stats.totalNodes" :suffix="stats.totalNodes > 1 ? '个节点' : '个节点'" />
      </a-card>
      <a-card class="stat-card stat-active">
        <template #title>
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
            <polyline points="22 4 12 14.01 9 11.01" />
          </svg>
          <span>在线节点</span>
        </template>
        <a-statistic :value="stats.onlineNodes" :suffix="stats.onlineNodes > 1 ? '个在线' : '个在线'" />
      </a-card>
      <a-card class="stat-card stat-leader">
        <template #title>
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
            <polygon points="12 2 22 8.5 22 15.5 12 22 2 15.5 2 8.5 12 2" />
            <line x1="12" y1="22" x2="12" y2="15.5" />
          </svg>
          <span>主节点</span>
        </template>
        <a-statistic :value="stats.leaderCount" :suffix="stats.leaderCount > 1 ? '个主节点' : '个主节点'" />
      </a-card>
      <a-card class="stat-card stat-config">
        <template #title>
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z" />
            <polyline points="14 2 14 8 20 8" />
            <line x1="16" y1="13" x2="8" y2="13" />
            <line x1="16" y1="17" x2="8" y2="17" />
            <polyline points="10 9 9 9 8 9" />
          </svg>
          <span>配置总数</span>
        </template>
        <a-statistic :value="stats.totalConfigs" :suffix="stats.totalConfigs > 1 ? '条配置' : '条配置'" />
      </a-card>
    </div>

    <!-- 主内容区 -->
    <div class="main-content">
      <!-- 节点列表 -->
      <a-card class="nodes-card">
        <template #title>
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="12" y1="20" x2="12" y2="10" />
            <line x1="18" y1="20" x2="18" y2="4" />
            <line x1="6" y1="20" x2="6" y2="16" />
          </svg>
          <span>集群节点</span>
        </template>
        
        <a-table 
          :columns="nodeColumns" 
          :data="nodes"
          :pagination="false"
          row-key="peerId"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'status'">
              <a-tag :color="record.status === 'online' ? 'green' : 'red'">
                {{ record.status === 'online' ? '在线' : '离线' }}
              </a-tag>
            </template>
            <template v-else-if="column.key === 'role'">
              <a-tag :color="record.role === 'leader' ? 'orange' : 'blue'">
                {{ record.role === 'leader' ? '主节点' : '从节点' }}
              </a-tag>
            </template>
            <template v-else-if="column.key === 'actions'">
              <a-space>
                <a-button type="text" size="small" @click="viewNodeDetail(record)">
                  <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                    <path d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                  </svg>
                </a-button>
                <a-button type="text" size="small" @click="syncWithNode(record)">
                  <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9" />
                  </svg>
                </a-button>
              </a-space>
            </template>
            <template v-else-if="column.key === 'peerId'">
              <span class="mono-text">{{ truncatePeerId(record.peerId) }}</span>
            </template>
          </template>
        </a-table>
      </a-card>

      <!-- 集群状态 -->
      <div class="status-section">
        <a-card class="cluster-status-card">
          <template #title>
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <polyline points="12 6 12 12 16 14" />
            </svg>
            <span>集群状态</span>
          </template>
          
          <div class="cluster-status-content">
            <div class="status-item">
              <div class="status-label">集群健康度</div>
              <div class="status-value">
                <a-progress :percent="clusterHealth" :show-info="false" />
                <span class="health-percent">{{ clusterHealth }}%</span>
              </div>
            </div>
            <div class="status-item">
              <div class="status-label">同步一致性</div>
              <div class="status-value">
                <a-tag :color="consistencyStatus === 'consistent' ? 'green' : 'orange'">
                  {{ consistencyStatus === 'consistent' ? '完全一致' : '存在差异' }}
                </a-tag>
              </div>
            </div>
            <div class="status-item">
              <div class="status-label">当前主节点</div>
              <div class="status-value">
                <span class="mono-text">{{ leaderNodeId }}</span>
              </div>
            </div>
          </div>
        </a-card>

        <!-- 网络拓扑图 -->
        <a-card class="network-topology-card">
          <template #title>
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M18 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2" />
              <circle cx="12" cy="7" r="4" />
            </svg>
            <span>网络拓扑</span>
          </template>
          
          <div class="topology-container">
            <div class="topology-center" :class="localNodeStatus === 'online' ? 'online' : 'offline'">
              <div class="center-icon">
                <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M12 20V10" />
                  <path d="M18 20V4" />
                  <path d="M6 20v-6" />
                </svg>
              </div>
              <div class="center-label">本地节点</div>
            </div>
            
            <div class="topology-peers">
              <div 
                v-for="(peer, index) in displayPeers" 
                :key="peer.peerId"
                class="peer-node"
                :class="peer.status"
                :style="getPeerPosition(index)"
              >
                <div class="peer-icon">
                  <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                    <circle cx="12" cy="12" r="10" />
                    <path d="M8 15s1.5-2 4-2 4 2 4 2" />
                    <circle cx="12" cy="9" r="3" />
                  </svg>
                </div>
                <div class="peer-label">{{ peer.name }}</div>
              </div>
            </div>
            
            <!-- 连接线 -->
            <svg class="connection-lines">
              <line 
                v-for="(peer, index) in displayPeers" 
                :key="'line-' + peer.peerId"
                :x1="150" :y1="100"
                :x2="getPeerPosition(index).x"
                :y2="getPeerPosition(index).y"
                stroke="#e2e8f0"
                stroke-width="1"
                stroke-dasharray="4"
              />
            </svg>
          </div>
        </a-card>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'

const emit = defineEmits(['refresh'])

// 统计数据
const stats = ref({
  totalNodes: 5,
  onlineNodes: 4,
  leaderCount: 1,
  totalConfigs: 128
})

// 节点列表
const nodes = ref([
  {
    peerId: '12D3KooWMB71AGTizmRu2Ek2wkuBgF1yh2n7SPwVnAPqTvSYbrAs',
    name: '本地节点',
    address: '192.168.1.100:4001',
    status: 'online',
    role: 'leader',
    latency: 0,
    configCount: 128
  },
  {
    peerId: '12D3KooWJz9m7R1R5gH78n2Y3m1P2Q3R4S5T6U7V8W9X0Y1Z2',
    name: '边缘节点-A',
    address: '192.168.1.101:4001',
    status: 'online',
    role: 'follower',
    latency: 8,
    configCount: 128
  },
  {
    peerId: '12D3KooWQ9rQZ9m7R1R5gH78n2Y3m1P2Q3R4S5T6U7V8W9X0',
    name: '边缘节点-B',
    address: '192.168.1.102:4001',
    status: 'online',
    role: 'follower',
    latency: 12,
    configCount: 126
  },
  {
    peerId: '12D3KooWJz9m7R1R5gH78n2Y3m1P2Q3R4S5T6U7V8W9X0Y',
    name: '边缘节点-C',
    address: '192.168.1.103:4001',
    status: 'online',
    role: 'follower',
    latency: 15,
    configCount: 128
  },
  {
    peerId: '12D3KooWJz9m7R1R5gH78n2Y3m1P2Q3R4S5T6U7V8W9X0Y1',
    name: '边缘节点-D',
    address: '192.168.1.104:4001',
    status: 'offline',
    role: 'follower',
    latency: 0,
    configCount: 96
  }
])

// 本地节点状态
const localNodeStatus = ref('online')

// 集群健康度
const clusterHealth = computed(() => {
  return Math.round((stats.value.onlineNodes / stats.value.totalNodes) * 100)
})

// 一致性状态
const consistencyStatus = ref('consistent')

// 主节点ID
const leaderNodeId = ref('12D3KooWMB71AGTizmRu2Ek2wkuBgF1yh2n7SPwVnAPqTvSYbrAs')

// 列配置
const nodeColumns = [
  { title: '节点名称', dataIndex: 'name', key: 'name' },
  { title: '节点ID', dataIndex: 'peerId', key: 'peerId', width: 180 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 80 },
  { title: '角色', dataIndex: 'role', key: 'role', width: 80 },
  { title: '延迟', dataIndex: 'latency', key: 'latency', width: 80, render: (val) => `${val}ms` },
  { title: '配置数', dataIndex: 'configCount', key: 'configCount', width: 80 },
  { title: '操作', key: 'actions', width: 100 }
]

// 显示的 peers
const displayPeers = computed(() => nodes.value.filter(n => n.status === 'online' && n.peerId !== '12D3KooWMB71AGTizmRu2Ek2wkuBgF1yh2n7SPwVnAPqTvSYbrAs'))

// 获取 peer 位置
function getPeerPosition(index) {
  const positions = [
    { x: 280, y: 40 },
    { x: 280, y: 160 },
    { x: 20, y: 70 },
    { x: 20, y: 130 },
    { x: 150, y: 200 }
  ]
  return positions[index] || positions[0]
}

// 方法
function truncatePeerId(peerId) {
  if (!peerId) return ''
  return peerId.length > 12 ? peerId.substring(0, 6) + '...' + peerId.substring(peerId.length - 6) : peerId
}

function refreshCluster() {
  console.log('Refreshing cluster info...')
}

function viewNodeDetail(node) {
  console.log('Viewing node detail:', node.name)
}

function syncWithNode(node) {
  console.log('Syncing with node:', node.name)
}

onMounted(() => {
  console.log('ClusterView mounted')
})
</script>

<style scoped>
/* v3.0 — styles in src/styles/ */
</style>
