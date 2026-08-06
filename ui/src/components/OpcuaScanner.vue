<template>
  <a-modal
    v-model:visible="localVisible"
    :title="'OPC-UA 点位扫描'"
    :width="1200"
    :mask-closable="false"
    unmount-on-close
    modal-class="industrial-white-modal"
    :align-center="true"
    @cancel="onCancel"
  >
    <div class="scanner-content">
      <!-- 头部 Banner -->
      <div class="scanner-header-banner">
        <div class="endpoint-info">
          <span class="label">ENDPOINT</span>
          <span class="value font-mono">{{ effectiveEndpoint || '-' }}</span>
        </div>
        <div class="protocol-badge">
          <span class="protocol-tag-simple">{{ formatProtocolTag('opc-ua') }}</span>
        </div>
      </div>

      <!-- 工具栏 - 水平对齐，扫描按钮靠右 -->
      <div class="scanner-toolbar">
        <div class="toolbar-left">
          <a-input
            v-model="filters.keyword"
            placeholder="搜索 NodeID / 名称"
            size="small"
            class="industrial-input"
            allow-clear
          >
            <template #prefix><IconSearch /></template>
          </a-input>

          <div class="toolbar-divider"></div>

          <!-- 状态筛选 - 极简符号点+文字 -->
          <div class="status-filters">
            <div 
              v-for="opt in diffStatusOptions" 
              :key="opt.value"
              class="status-filter-item"
              :class="{ active: filters.diffStatus === opt.value }"
              @click="filters.diffStatus = opt.value"
            >
              <span :class="['status-dot', opt.value === 'new' ? 'dot-new' : opt.value === 'existing' ? 'dot-existing' : 'dot-all']"></span>
              <span class="status-label">{{ opt.label }}</span>
            </div>
          </div>

          <div class="toolbar-divider"></div>

          <a-checkbox v-model="filters.varsOnly" class="industrial-checkbox">
            仅显示变量
          </a-checkbox>
        </div>

        <div class="toolbar-right">
          <a-button
            type="primary"
            size="small"
            :loading="loading"
            @click="startScan"
            :disabled="!effectiveEndpoint"
            class="scan-btn"
          >
            <template #icon><IconScan /></template>
            重新扫描
          </a-button>
        </div>
      </div>

      <!-- 统计信息栏 -->
      <div v-if="filteredResults.length > 0" class="stats-bar">
        <div class="stats-item">
          <span class="stats-number">{{ filteredResults.length }}</span>
          <span class="stats-unit">个节点</span>
        </div>
        <div class="stats-divider"></div>
        <div class="stats-item">
          <span class="status-dot dot-new"></span>
          <span class="stats-number">{{ filteredResults.filter(r => r.diff_status === 'new').length }}</span>
          <span class="stats-unit">新增</span>
        </div>
        <div class="stats-item">
          <span class="status-dot dot-existing"></span>
          <span class="stats-number">{{ filteredResults.filter(r => r.diff_status === 'existing').length }}</span>
          <span class="stats-unit">存量</span>
        </div>
      </div>

      <!-- 表格区域 - 改为左右分栏 -->
      <div v-if="flatNodes.length > 0 || !loading" class="table-wrapper">
        <a-spin :loading="loading" class="w-full">
          <div v-if="flatNodes.length > 0" class="split-view">
            <!-- 左侧：地址空间树 -->
            <div class="tree-panel">
              <div class="panel-header">
                <span class="panel-title">地址空间</span>
                <span class="panel-count">{{ treeNodeCount }} 个节点</span>
              </div>
              <div class="tree-content">
                <a-tree
                  v-if="treeData.length > 0"
                  :data="treeData"
                  :default-expand-all="true"
                  :selected-keys="[activeBranchKey]"
                  :show-line="true"
                  block-node
                  @select="onTreeSelect"
                  class="opc-tree"
                />
                <div v-else class="empty-tree">
                  暂无树数据
                </div>
              </div>
            </div>

            <!-- 右侧：点位列表 -->
            <div class="list-panel">
              <!-- 列表工具栏 -->
              <div class="list-toolbar">
                <div class="list-toolbar-left">
                  <span class="panel-title">点位列表</span>
                  <span class="panel-count">{{ filteredResults.length }} 个点位</span>
                </div>
                <div class="list-toolbar-right">
                  <a-button 
                    size="mini" 
                    @click="selectAll"
                    :disabled="selectableAllNodes.length === 0"
                    class="mini-btn"
                  >
                    全部
                  </a-button>
                  <a-button
                    size="mini"
                    @click="clearSelection"
                    :disabled="selected.length === 0"
                    class="mini-btn"
                  >
                    清空选择
                  </a-button>
                  <a-button
                    size="mini"
                    type="primary"
                    @click="selectCurrentLevel"
                    :disabled="selectableBranchNodes.length === 0"
                    class="mini-btn primary"
                  >
                    选择当前层级点位 ({{ getCurrentLevelSelectableCount() }})
                  </a-button>
                </div>
              </div>

              <!-- 表格 -->
              <a-table
                :columns="columns"
                :data="filteredResults"
                size="small"
                :bordered="{ cell: true, wrapper: true }"
                :pagination="{ pageSize: 20, showPageSize: true, pageSizeOptions: [10, 20, 50, 100] }"
                :scroll="{ y: 400 }"
                row-key="node_id"
                class="industrial-table"
              >
                <template #selection="{ record }">
                  <div class="row-checkbox-wrapper">
                    <a-checkbox 
                      :model-value="selected.includes(record.node_id)"
                      @change="(checked) => onRowCheck(checked, record)"
                      :disabled="!isSelectableNode(record)"
                      class="industrial-checkbox"
                    />
                  </div>
                </template>
                
                <template #status="{ record }">
                  <div class="status-cell">
                    <span :class="['status-dot', record.diff_status === 'new' ? 'dot-new' : 'dot-existing']" :title="record.diff_status === 'new' ? '新增' : '存量'"></span>
                  </div>
                </template>

                <template #name="{ record }">
                  <div class="node-name-cell">
                    <span class="node-icon">{{ record.type === 'Folder' ? '📁' : '◇' }}</span>
                    <span class="node-name font-mono">{{ record.display_name }}</span>
                  </div>
                </template>
              </a-table>
            </div>
          </div>
          
          <div v-else-if="!loading" class="empty-placeholder">
            <IconSearch size="28" class="empty-icon" />
            <div class="empty-text">点击"重新扫描"获取点位</div>
          </div>
        </a-spin>
      </div>
    </div>

    <!-- Footer 区域 -->
    <template #footer>
      <div class="modal-footer">
        <div class="footer-info">
          <span class="footer-label">已选择</span>
          <span class="footer-number">{{ selected.length }}</span>
          <span class="footer-unit">个点位</span>
        </div>
        <div class="footer-actions">
          <a-button @click="onCancel" size="small" class="cancel-btn">取消</a-button>
          <a-button 
            type="primary" 
            @click="addSelected" 
            :disabled="selected.length === 0" 
            :loading="adding" 
            size="small" 
            class="confirm-btn"
          >
            确认添加
          </a-button>
        </div>
      </div>
    </template>
  </a-modal>
</template>

<script setup>
import { ref, reactive, computed, watch, h, resolveComponent } from 'vue'
import { IconScan, IconSearch } from '@arco-design/web-vue/es/icon'
import request from '@/utils/request'
import { channelDeviceApiPath } from '@/utils/deviceRoute'
import { formatProtocolTag } from '@/utils/protocolLabel'
import { postScanAndWait } from '@/utils/asyncJob'

const ROOT_KEY = '__root__'

const props = defineProps({
  visible: {
    type: Boolean,
    default: false
  },
  channelId: {
    type: String,
    required: true
  },
  deviceId: {
    type: String,
    required: true
  },
  deviceConfig: {
    type: Object,
    default: () => ({})
  },
  channelConfig: {
    type: Object,
    default: () => ({})
  },
  existingPoints: {
    type: Array,
    default: () => []
  }
})

const emit = defineEmits(['update:visible', 'cancel', 'points-added'])

const effectiveEndpoint = computed(() => {
  const devEp = props.deviceConfig?.endpoint
  if (devEp && String(devEp).trim()) {
    return String(devEp).trim()
  }
  const chCfg = props.channelConfig || {}
  return chCfg.url || chCfg.endpoint || ''
})

const loading = ref(false)
const adding = ref(false)
const rawTree = ref([])
const flatNodes = ref([])
const activeBranchKey = ref(ROOT_KEY)
const selected = ref([])
const localVisible = ref(props.visible)

const filters = reactive({
  keyword: '',
  varsOnly: true,
  diffStatus: 'all'
})

const diffStatusOptions = [
  { label: '全部', value: 'all' },
  { label: '新增', value: 'new' },
  { label: '存量', value: 'existing' }
]

watch(() => props.visible, (newValue) => {
  localVisible.value = newValue
})

watch(localVisible, (newValue) => {
  emit('update:visible', newValue)
})

const columns = [
  {
    title: () => {
      const Checkbox = resolveComponent('a-checkbox')
      return h('div', { class: 'selection-header' }, [
        h(Checkbox, {
          modelValue: isAllSelected.value,
          indeterminate: isIndeterminate.value,
          onChange: onToggleAll,
          disabled: selectableBranchNodes.value.length === 0,
          class: 'industrial-checkbox'
        })
      ])
    },
    key: 'selection',
    width: 60,
    align: 'center',
    slotName: 'selection'
  },
  {
    title: '状态',
    slotName: 'status',
    width: 80,
    align: 'center'
  },
  {
    title: '节点名称',
    slotName: 'name',
    ellipsis: true,
    tooltip: true
  },
  {
    title: '节点类型',
    dataIndex: 'type',
    width: 120
  },
  {
    title: '数据类型',
    dataIndex: 'data_type',
    width: 120,
    ellipsis: true
  },
  {
    title: '访问权限',
    dataIndex: 'access_level',
    width: 100,
    ellipsis: true
  },
  {
    title: 'NodeID',
    dataIndex: 'node_id',
    ellipsis: true
  }
]

const existingAddresses = computed(() => {
  const set = new Set()
  for (const p of props.existingPoints) {
    if (p && p.address) set.add(p.address)
  }
  return set
})

const normalizeOpcNodes = (nodes, parentId = null, pathIds = [], level = 0) => {
  let normalized = []

  for (const node of nodes || []) {
    const nodeId = node.node_id
    const currentPathIds = nodeId ? [...pathIds, nodeId] : [...pathIds]
    const normalizedNode = {
      ...node,
      node_id: nodeId,
      display_name: node.display_name || node.name || nodeId || '-',
      type: node.type || 'Unknown',
      data_type: node.data_type || '',
      access_level: node.access_level || '',
      diff_status: nodeId && existingAddresses.value.has(nodeId) ? 'existing' : 'new',
      level,
      parent_id: parentId,
      path_ids: currentPathIds,
      has_children: Array.isArray(node.children) && node.children.length > 0
    }

    normalized.push(normalizedNode)

    if (normalizedNode.has_children) {
      normalized = normalized.concat(
        normalizeOpcNodes(node.children, nodeId, currentPathIds, level + 1)
      )
    }
  }

  return normalized
}

const applyFilters = (nodes) => {
  let list = nodes || []

  if (filters.varsOnly) {
    list = list.filter(node => node.type === 'Variable')
  }

  if (filters.diffStatus !== 'all') {
    list = list.filter(node => node.diff_status === filters.diffStatus)
  }

  if (filters.keyword) {
    const keyword = filters.keyword.trim().toLowerCase()
    list = list.filter(node =>
      (node.node_id && node.node_id.toLowerCase().includes(keyword)) ||
      (node.display_name && node.display_name.toLowerCase().includes(keyword))
    )
  }

  return list
}

const branchNodes = computed(() => {
  if (activeBranchKey.value === ROOT_KEY) {
    return flatNodes.value
  }

  return flatNodes.value.filter(node => node.path_ids.includes(activeBranchKey.value))
})

const filteredResults = computed(() => applyFilters(branchNodes.value))
const allFilteredNodes = computed(() => applyFilters(flatNodes.value))

const isSelectableNode = (node) => {
  return node.type === 'Variable' && node.diff_status !== 'existing'
}

const selectableBranchNodes = computed(() => {
  return filteredResults.value.filter(isSelectableNode)
})

const selectableAllNodes = computed(() => {
  return allFilteredNodes.value.filter(isSelectableNode)
})

const treeVariableCountMap = computed(() => {
  const map = new Map([[ROOT_KEY, 0]])

  for (const node of flatNodes.value) {
    if (node.type !== 'Variable') continue

    map.set(ROOT_KEY, (map.get(ROOT_KEY) || 0) + 1)

    for (const pathId of node.path_ids) {
      map.set(pathId, (map.get(pathId) || 0) + 1)
    }
  }

  return map
})

const treeData = computed(() => {
  const mapTreeNodes = (nodes) => {
    return (nodes || []).flatMap(node => {
      const nodeId = node.node_id
      const type = node.type || 'Unknown'
      const children = mapTreeNodes(node.children || [])

      if (!nodeId || type === 'Variable') {
        return children
      }

      const displayName = node.display_name || node.name || nodeId
      const variableCount = treeVariableCountMap.value.get(nodeId) || 0

      return [{
        key: nodeId,
        node_id: nodeId,
        title: variableCount > 0 ? `${displayName} (${variableCount})` : displayName,
        display_name: displayName,
        type,
        variable_count: variableCount,
        children: children.length > 0 ? children : undefined
      }]
    })
  }

  const rootVariableCount = treeVariableCountMap.value.get(ROOT_KEY) || 0

  return [{
    key: ROOT_KEY,
    node_id: ROOT_KEY,
    title: rootVariableCount > 0 ? `全部地址空间 (${rootVariableCount})` : '全部地址空间',
    display_name: '全部地址空间',
    type: 'Root',
    variable_count: rootVariableCount,
    children: mapTreeNodes(rawTree.value)
  }]
})

const treeNodeCount = computed(() => {
  const countNodes = (nodes) => {
    let count = 0

    for (const node of nodes || []) {
      if (node.type !== 'Root') {
        count++
      }

      if (node.children && node.children.length > 0) {
        count += countNodes(node.children)
      }
    }

    return count
  }

  return countNodes(treeData.value)
})

const getNodeVariableCount = (node) => {
  return treeVariableCountMap.value.get(node.node_id || node.key) || 0
}

const getCurrentLevelSelectableCount = () => {
  return selectableBranchNodes.value.length
}

const isAllSelected = computed(() => {
  const selectableIds = selectableBranchNodes.value.map(node => node.node_id)
  return selectableIds.length > 0 && selectableIds.every(id => selected.value.includes(id))
})

const isIndeterminate = computed(() => {
  const selectableIds = selectableBranchNodes.value.map(node => node.node_id)
  const selectedCount = selected.value.filter(id => selectableIds.includes(id)).length

  return selectedCount > 0 && selectedCount < selectableIds.length
})

const onToggleAll = () => {
  const selectableIds = selectableBranchNodes.value.map(node => node.node_id)

  if (isAllSelected.value) {
    selected.value = selected.value.filter(id => !selectableIds.includes(id))
    return
  }

  const nextSelected = new Set(selected.value)
  selectableIds.forEach(id => nextSelected.add(id))
  selected.value = Array.from(nextSelected)
}

const onCancel = () => {
  emit('update:visible', false)
  emit('cancel')
  resetState()
}

const resetState = () => {
  loading.value = false
  adding.value = false
  rawTree.value = []
  flatNodes.value = []
  selected.value = []
  activeBranchKey.value = ROOT_KEY
}

const startScan = async () => {
  if (!effectiveEndpoint.value) {
    emit('error', 'OPC UA 未配置 endpoint（可在通道或设备中设置 Endpoint URL）')
    return
  }

  loading.value = true
  rawTree.value = []
  flatNodes.value = []
  selected.value = []
  activeBranchKey.value = ROOT_KEY

  try {
    const payload = { mode: 'fast' }
    const res = await postScanAndWait(
      request,
      channelDeviceApiPath(props.channelId, props.deviceId, 'scan'),
      payload,
      { timeoutMs: 190000 }
    )

    if (!Array.isArray(res)) {
      emit('error', '扫描结果格式错误')
      return
    }

    if (res.length === 0) {
      emit('info', '扫描结果为空')
      return
    }

    rawTree.value = res
    flatNodes.value = normalizeOpcNodes(res)
    activeBranchKey.value = ROOT_KEY
  } catch (e) {
    emit('error', '扫描失败: ' + e.message)
  } finally {
    loading.value = false
  }
}

const onTreeSelect = (selectedKeys) => {
  activeBranchKey.value = selectedKeys[0] || ROOT_KEY
}

const selectCurrentLevel = () => {
  const nextSelected = new Set(selected.value)
  selectableBranchNodes.value.forEach(node => nextSelected.add(node.node_id))
  selected.value = Array.from(nextSelected)
}

const selectAll = () => {
  const nextSelected = new Set(selected.value)
  selectableAllNodes.value.forEach(node => nextSelected.add(node.node_id))
  selected.value = Array.from(nextSelected)
}

const clearSelection = () => {
  selected.value = []
}

const onRowCheck = (checked, record) => {
  if (!isSelectableNode(record)) {
    return
  }

  const id = record.node_id
  if (!id) {
    return
  }

  if (checked) {
    if (!selected.value.includes(id)) {
      selected.value.push(id)
    }
    return
  }

  selected.value = selected.value.filter(selectedId => selectedId !== id)
}

const getOpcUaReadWrite = (accessLevel) => {
  const normalized = String(accessLevel || '').toLowerCase()
  const canRead = normalized.includes('currentread')
  const canWrite = normalized.includes('currentwrite')

  if (canRead && canWrite) return 'RW'
  if (canWrite) return 'W'
  return 'R'
}

const normalizeOpcUaDataType = (dataType) => {
  const normalized = String(dataType || '').trim().toLowerCase()

  const mapping = {
    'bool': 'bool',
    'boolean': 'bool',
    'byte': 'uint8',
    'uint8': 'uint8',
    'sbyte': 'int8',
    'int8': 'int8',
    'int16': 'int16',
    'short': 'int16',
    'uint16': 'uint16',
    'unsignedshort': 'uint16',
    'int32': 'int32',
    'int': 'int32',
    'uint32': 'uint32',
    'unsignedint': 'uint32',
    'int64': 'int64',
    'long': 'int64',
    'uint64': 'uint64',
    'unsignedlong': 'uint64',
    'float': 'float32',
    'float32': 'float32',
    'double': 'float64',
    'float64': 'float64',
    'string': 'string',
    'bytestring': 'bytestring'
  }

  return mapping[normalized] || ''
}

const addSelected = async () => {
  if (selected.value.length === 0) return

  const selectedObjs = flatNodes.value.filter(node => {
    return selected.value.includes(node.node_id) && isSelectableNode(node)
  })

  adding.value = true
  let successCount = 0
  let failCount = 0

  const pointPayloads = []
  for (const obj of selectedObjs) {
    const rw = getOpcUaReadWrite(obj.access_level)
    const dt = normalizeOpcUaDataType(obj.data_type)

    if (!dt) {
      failCount++
      console.warn(`Unsupported OPC UA datatype for quick import: ${obj.data_type}`, obj)
      continue
    }

    pointPayloads.push({
      id: obj.node_id,
      name: obj.display_name || obj.node_id,
      address: obj.node_id,
      datatype: dt,
      readwrite: rw,
      unit: '',
      scale: 1.0,
      offset: 0.0
    })
  }

  if (pointPayloads.length > 0) {
    try {
      await request.post(
        channelDeviceApiPath(props.channelId, props.deviceId, 'points'),
        pointPayloads
      )
      successCount = pointPayloads.length
    } catch (e) {
      console.error(e)
      failCount += pointPayloads.length
    }
  }

  adding.value = false
  emit('points-added', { success: successCount, fail: failCount })
  onCancel()
}
</script>

<style scoped>
.scanner-content {
  padding: 0;
}

.scanner-header-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  background: #fafbfc;
  border-bottom: 1px solid #e9ecef;
  margin-bottom: 16px;
}

.endpoint-info {
  display: flex;
  align-items: baseline;
  gap: 12px;
}

.scanner-header-banner .label {
  font-size: 11px;
  color: #6c757d;
  font-weight: 600;
  letter-spacing: 0.5px;
}

.scanner-header-banner .value {
  font-size: 12px;
  color: #495057;
  font-family: 'JetBrains Mono', monospace;
}

.protocol-tag-simple {
  background: #e9ecef;
  color: #495057;
  padding: 4px 12px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.5px;
}

.scanner-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  background: var(--edgeCore-surface-raised);
  border: 1px solid #e9ecef;
  margin-bottom: 12px;
}

.toolbar-left {
  display: flex;
  align-items: center;
  gap: 16px;
  flex: 1;
}

.toolbar-right {
  display: flex;
  align-items: center;
}

.toolbar-divider {
  width: 1px;
  height: 20px;
  background: #e9ecef;
}

.industrial-input {
  width: 220px;
}

.industrial-input :deep(.arco-input-wrapper) {
  border-radius: 0;
  border-color: #dee2e6;
  background: var(--edgeCore-surface-raised);
}

.industrial-input :deep(.arco-input-wrapper:hover) {
  border-color: #adb5bd;
}

.industrial-input :deep(.arco-input-wrapper:focus-within) {
  border-color: #495057;
  box-shadow: none;
}

.status-filters {
  display: flex;
  align-items: center;
  gap: 4px;
}

.status-filter-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  cursor: pointer;
  font-size: 12px;
  color: #6c757d;
  transition: all 0.2s ease;
  background: transparent;
}

.status-filter-item:hover {
  color: #495057;
  background: #f8f9fa;
}

.status-filter-item.active {
  color: #212529;
  background: #f8f9fa;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
}

.dot-new {
  background: #2ecc71;
}

.dot-existing {
  background: #95a5a6;
}

.dot-all {
  background: #bdc3c7;
}

.status-label {
  font-size: 12px;
  font-weight: 500;
}

.industrial-checkbox :deep(.arco-checkbox) {
  border-radius: 0;
}

.industrial-checkbox :deep(.arco-checkbox-icon) {
  border-radius: 0;
}

.scan-btn {
  background: #212529 !important;
  border: none;
  border-radius: 0;
  padding: 4px 16px;
  font-size: 12px;
}

.scan-btn:hover {
  background: #343a40 !important;
}

.stats-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 8px 16px;
  background: #fafbfc;
  border: 1px solid #e9ecef;
  margin-bottom: 12px;
}

.stats-item {
  display: flex;
  align-items: center;
  gap: 6px;
}

.stats-number {
  font-size: 13px;
  font-weight: 600;
  color: #212529;
  font-family: 'JetBrains Mono', monospace;
}

.stats-unit {
  font-size: 11px;
  color: #6c757d;
}

.stats-divider {
  width: 1px;
  height: 14px;
  background: #dee2e6;
}

.table-wrapper {
  border: 1px solid #e9ecef;
  background: var(--edgeCore-surface-raised);
  display: flex;
  flex-direction: column;
  min-height: 450px;
}

.table-wrapper .arco-spin {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.table-wrapper .arco-spin .w-full {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.split-view {
  display: flex;
  height: 480px;
}

.tree-panel {
  width: 280px;
  border-right: 1px solid #e9ecef;
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  background: #fafbfc;
  border-bottom: 1px solid #e9ecef;
}

.panel-title {
  font-size: 12px;
  font-weight: 600;
  color: #495057;
}

.panel-count {
  font-size: 11px;
  color: #868e96;
  font-family: 'JetBrains Mono', monospace;
}

.tree-content {
  flex: 1;
  overflow: auto;
  padding: 8px 0;
}

.opc-tree :deep(.arco-tree-node) {
  padding: 2px 0;
}

.opc-tree :deep(.arco-tree-node-title) {
  padding: 4px 8px;
  border-radius: 0;
}

.opc-tree :deep(.arco-tree-node-title:hover) {
  background: #f8f9fa;
}

.opc-tree :deep(.arco-tree-node-selected .arco-tree-node-title) {
  background: #e9ecef;
}

.empty-tree {
  padding: 24px;
  text-align: center;
  color: #adb5bd;
  font-size: 12px;
}

.list-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.list-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  background: #fafbfc;
  border-bottom: 1px solid #e9ecef;
}

.list-toolbar-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.list-toolbar-right {
  display: flex;
  align-items: center;
  gap: 6px;
}

.mini-btn {
  font-size: 11px;
  padding: 2px 10px;
  border-radius: 0;
  border-color: #dee2e6;
  color: #495057;
}

.mini-btn:hover:not(:disabled) {
  border-color: #adb5bd;
  color: #212529;
}

.mini-btn.primary {
  background: #495057 !important;
  border-color: #495057;
  color: #fff;
}

.mini-btn.primary:hover:not(:disabled) {
  background: #343a40 !important;
  border-color: #343a40;
}

.industrial-table :deep(.arco-table-th) {
  background: #fafbfc;
  border-bottom: 1px solid #e9ecef;
  color: #495057;
  font-size: 11px;
  font-weight: 600;
  padding: 12px 12px;
}

.industrial-table :deep(.arco-table-td) {
  padding: 10px 12px;
  font-size: 12px;
  border-bottom: 1px solid #f1f3f5;
}

.industrial-table :deep(.arco-table-tr:hover .arco-table-td) {
  background: #f8f9fa;
}

.row-checkbox-wrapper {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
}

.selection-header {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  width: 100%;
}

.status-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}

.node-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.node-icon {
  font-size: 12px;
  color: #868e96;
  flex-shrink: 0;
}

.node-name {
  font-size: 12px;
  color: #212529;
  font-family: 'JetBrains Mono', monospace;
}

.font-mono {
  font-family: 'JetBrains Mono', monospace;
}

.empty-placeholder {
  flex: 1;
  min-height: 360px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  background: #fafbfc;
  width: 100%;
}

.empty-icon {
  color: #ced4da;
}

.empty-text {
  margin-top: 12px;
  font-size: 12px;
  color: #adb5bd;
}

.modal-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-top: 8px;
}

.footer-info {
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.footer-label {
  font-size: 11px;
  color: #6c757d;
}

.footer-number {
  font-size: 14px;
  font-weight: 600;
  color: #212529;
  font-family: 'JetBrains Mono', monospace;
}

.footer-unit {
  font-size: 11px;
  color: #868e96;
}

.footer-actions {
  display: flex;
  gap: 12px;
}

.cancel-btn {
  border-radius: 0;
  border-color: #dee2e6;
  color: #495057;
  font-size: 12px;
}

.cancel-btn:hover {
  border-color: #adb5bd;
  color: #212529;
}

.confirm-btn {
  background: #212529 !important;
  border: none;
  border-radius: 0;
  font-size: 12px;
}

.confirm-btn:hover:not(:disabled) {
  background: #343a40 !important;
}

.confirm-btn:disabled {
  background: #e9ecef !important;
  color: #adb5bd;
}

@media (max-width: 768px) {
  .scanner-toolbar {
    flex-direction: column;
    align-items: stretch;
    gap: 12px;
  }

  .toolbar-left {
    flex-wrap: wrap;
    gap: 12px;
  }

  .toolbar-divider {
    display: none;
  }

  .status-filters {
    flex-wrap: wrap;
  }

  .industrial-input {
    width: 100%;
  }

  .modal-footer {
    flex-direction: column;
    gap: 12px;
    align-items: stretch;
  }

  .footer-info {
    justify-content: center;
  }

  .footer-actions {
    justify-content: center;
  }

  .selection-header {
    flex-direction: column;
    gap: 4px;
  }
}
</style>

