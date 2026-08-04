<template>
  <a-modal 
    :visible="props.visible" 
    @update:visible="handleClose" 
    :width="1200" 
    title="点位扫描发现"
    title-align="start" 
    :footer="false" 
    unmount-on-close 
    modal-class="industrial-white-modal"
    :align-center="true"
  >
    <div class="scanner-content">
      <div class="scanner-header-banner">
        <div class="endpoint-info">
          <span class="label">DEVICE</span>
          <span class="value font-mono">{{ deviceInstanceLabel }}</span>
        </div>
        <div class="protocol-badge">
          <span class="protocol-tag-simple">{{ formatProtocolTag(props.channelProtocol) }}</span>
        </div>
      </div>

      <div class="scan-container">
        <div class="scanner-toolbar">
          <div class="toolbar-left">
            <a-input-search 
              v-model="state.filterText" 
              placeholder="搜索 NodeID / 名称" 
              size="small" 
              style="width: 240px" 
              allow-clear 
              class="industrial-input"
            />
            <div class="toolbar-divider"></div>
            <!-- 状态筛选 - 极简符号点+文字 -->
            <div class="status-filters">
              <div 
                class="status-filter-item" 
                :class="{ active: state.filterStatus === 'all' }"
                @click="state.filterStatus = 'all'"
              >
                <span class="status-dot dot-all"></span>
                <span class="status-label">全部</span>
              </div>
              <div 
                class="status-filter-item" 
                :class="{ active: state.filterStatus === 'new' }"
                @click="state.filterStatus = 'new'"
              >
                <span class="status-dot dot-new"></span>
                <span class="status-label">新增</span>
              </div>
              <div 
                class="status-filter-item" 
                :class="{ active: state.filterStatus === 'exists' }"
                @click="state.filterStatus = 'exists'"
              >
                <span class="status-dot dot-existing"></span>
                <span class="status-label">存量</span>
              </div>
            </div>
          </div>
          <div class="toolbar-right">
            <a-button 
              @click="handleScan" 
              :loading="state.loading" 
              size="small" 
              class="scan-btn"
            >
              <template #icon><IconScan /></template> 重新扫描
            </a-button>
            <a-button 
              type="primary" 
              :disabled="!state.selectedKeys.length" 
              @click="handleAddSelected" 
              size="small" 
              class="scan-btn"
              style="margin-left: 8px"
            >
              导入选中点位 ({{ state.selectedKeys.length }})
            </a-button>
          </div>
        </div>

        <a-table 
          row-key="unique_id" 
          :loading="state.loading" 
          :columns="scanColumns" 
          :data="filteredScanResults" 
          :pagination="{ pageSize: 100, size: 'small' }" 
          :row-selection="{ type: 'checkbox', showCheckedAll: true }" 
          v-model:selected-keys="state.selectedKeys" 
          :bordered="{ wrapper: true, cell: true }" 
          :scroll="{ y: 550 }" 
          class="industrial-table-fluid"
        >
          <template #status="{ record }">
            <a-tag v-if="record.is_exists" color="gray" size="mini" class="rect-tag">
              <template #icon><IconCheckCircle /></template>存量
            </a-tag>
            <a-tag v-else color="green" size="mini" class="rect-tag">
              <template #icon><IconPlus /></template>新增
            </a-tag>
          </template>

          <template #address="{ record }">
            <span class="font-mono text-[13px]">
              <template v-if="props.channelProtocol === 'bacnet-ip'">
                {{ record.type }}:{{ record.instance }}
              </template>
              <template v-else-if="props.channelProtocol === 'opc-ua'">
                {{ record.node_id }}
              </template>
            </span>
          </template>
        </a-table>
      </div>
    </div>
  </a-modal>
</template>

<script setup>
import { ref, reactive, computed } from 'vue'
import { 
  IconScan, IconCheckCircle, IconPlus 
} from '@arco-design/web-vue/es/icon'
import request from '@/utils/request'
import { showMessage } from '../composables/useGlobalState'
import { formatProtocolTag } from '../utils/protocolLabel'
import { postScanAndWait } from '@/utils/asyncJob'

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
  channelProtocol: {
    type: String,
    required: true
  },
  existingAddresses: {
    type: Set,
    required: true
  },
  deviceInfo: {
    type: Object,
    default: null
  }
})

const emit = defineEmits(['update:visible', 'refresh-points'])

const deviceInstanceLabel = computed(() => {
  const config = props.deviceInfo?.config
  if (!config) return '-'
  const id = config.bacnet_device_id ?? config.device_id ?? config.bacnetDeviceInstance ?? config.InstanceID ?? config.instance_id
  return id != null ? String(id) : '-'
})

const state = reactive({
  visible: false,
  loading: false,
  results: [],
  selectedKeys: [],
  filterText: '',
  filterStatus: 'all',
  mode: 'fast'
})

// 计算属性：实现多维度筛选
const filteredScanResults = computed(() => {
  return state.results.filter(item => {
    // 1. 文本搜索
    const matchText = !state.filterText || 
      (item.name && item.name.toLowerCase().includes(state.filterText.toLowerCase())) || 
      (item.object_name && item.object_name.toLowerCase().includes(state.filterText.toLowerCase())) ||
      (item.node_id && item.node_id.includes(state.filterText));
    
    // 2. 存量筛选 (基于 is_exists 属性)
    const matchStatus = 
      state.filterStatus === 'all' || 
      (state.filterStatus === 'new' && !item.is_exists) || 
      (state.filterStatus === 'exists' && item.is_exists);
      
    return matchText && matchStatus;
  });
});

// 计算新增点位数量
const countNewPoints = computed(() => 
  state.results.filter(i => !i.is_exists).length
);

// 统一的表格列定义
const scanColumns = computed(() => {
  const base = [
    { title: '状态', slotName: 'status', width: 90 },
    { title: '名称', dataIndex: 'name', ellipsis: true, tooltip: true },
    { title: '点位地址', slotName: 'address', width: 220 },
    { title: '类型', dataIndex: 'type', width: 120 },
    { title: '实例', dataIndex: 'instance', width: 80 },
    { title: '当前值', dataIndex: 'present_value', width: 100 },
    { title: '单位', dataIndex: 'units', width: 80 },
    { title: '描述/DataType', dataIndex: 'description', ellipsis: true, tooltip: true }
  ];

  return base;
});

// 处理扫描
const handleScan = async () => {
  state.loading = true
  state.results = []
  try {
    // 构建扫描请求参数
    const payload = {
      mode: state.mode
    }
    
    // 提取设备实例ID（如果有）
    // bacnet_device_id: BACnet 通信使用的真实设备实例 ID
    let targetDeviceId = null
    if (props.deviceInfo?.config?.bacnet_device_id) {
      targetDeviceId = props.deviceInfo.config.bacnet_device_id
    } else if (props.deviceInfo?.config?.device_id) {
      targetDeviceId = props.deviceInfo.config.device_id
    } else if (props.deviceInfo?.config?.instance_id) {
      targetDeviceId = props.deviceInfo.config.instance_id
    } else {
      // 尝试从设备配置中提取
      const deviceIdParts = props.deviceId.split('-')
      if (deviceIdParts.length > 1) {
        const lastPart = deviceIdParts[deviceIdParts.length - 1]
        if (!isNaN(lastPart)) {
          targetDeviceId = lastPart
        }
      }
    }

    if (targetDeviceId === undefined || targetDeviceId === null || targetDeviceId === '') {
      showMessage('无法获取设备实例ID (config.bacnet_device_id 或 device_id)', 'error')
      return
    }

    // 如果提取到了 device_id (BACnet Instance ID)，则显式传递给后端
    if (targetDeviceId !== null) {
      payload.device_id = parseInt(targetDeviceId)
    }

    const res = await postScanAndWait(
      request,
      `/api/channels/${props.channelId}/devices/${props.deviceId}/scan`,
      payload,
      { timeoutMs: 70000 }
    )
    
    // Normalize result: accept raw array, {devices: [...]}, or {data: [...]}
    let items = []
    if (Array.isArray(res)) {
      items = res
    } else if (res && Array.isArray(res.devices)) {
      items = res.devices
    } else if (res && Array.isArray(res.data)) {
      items = res.data
    } else if (res && Array.isArray(res.points)) {
      items = res.points
    } else {
      showMessage('扫描结果格式错误', 'error')
      return
    }

    if (items.length === 0) {
      showMessage('扫描结果为空', 'warning')
    }
    if (props.channelProtocol === 'opc-ua') {
      // Flatten OPC UA tree for display
      state.results = flattenOpcNodes(items)
    } else {
      // For BACnet (and others), process is_exists based on existing points in UI
      state.results = items.map(item => {
        if (props.channelProtocol === 'bacnet-ip') {
             const key = `${item.type}:${item.instance}`
             item.is_exists = props.existingAddresses.has(key)
             item.unique_id = key
             item.name = item.object_name || item.name
        }
        return item
      })
    }
  } catch (e) {
    showMessage('扫描失败: ' + e.message, 'error')
  } finally {
    state.loading = false
  }
}

// 处理添加选定点位 — 批量导入，单次请求
const handleAddSelected = async () => {
  if (state.selectedKeys.length === 0) return
  
  state.loading = true
  let successCount = 0
  let failCount = 0
  
  // Find selected objects based on selectedKeys
  const selectedObjects = state.results.filter(obj => state.selectedKeys.includes(obj.unique_id))
  
  // Build all point payloads into a single array for batch import
  const pointPayloads = []

  for (const obj of selectedObjects) {
    if (props.channelProtocol === 'opc-ua') {
      // OPC UA Point Mapping
      if (obj.type !== 'Variable') continue;
      
      let rw = 'R'
      if (obj.access_level && obj.access_level.includes('CurrentWrite')) {
        rw = 'RW'
      }
      
      let dt = (obj.data_type || 'Float').toLowerCase()
      if (dt.includes('bool')) dt = 'bool'
      else if (dt.includes('int16') || dt.includes('short')) dt = 'int16'
      else if (dt.includes('uint16') || dt.includes('unsignedshort')) dt = 'uint16'
      else if (dt.includes('int32') || dt.includes('int')) dt = 'int32'
      else if (dt.includes('uint32') || dt.includes('unsignedint')) dt = 'uint32'
      else if (dt.includes('float')) dt = 'float32'
      else if (dt.includes('double')) dt = 'float64'
      else if (dt.includes('string')) dt = 'string'
      else dt = 'float32'

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
    } else {
      // BACnet Point Mapping
      let datatype = 'float32'
      if (obj.type.includes('Binary') || obj.type.includes('Bit')) datatype = 'bool'
      if (obj.type.includes('MultiState')) datatype = 'uint16'
      
      let rw = 'R'
      if (obj.type.includes('Output') || obj.type.includes('Value')) rw = 'RW'
      
      // Use consistent ID format: type-instance (lowercase)
      const pointId = `${obj.type.toLowerCase()}-${obj.instance}`
      
      pointPayloads.push({
        id: pointId,
        name: obj.name || obj.description || `${obj.type} ${obj.instance}`,
        address: `${obj.type}:${obj.instance}`,
        datatype: datatype,
        readwrite: rw,
        unit: obj.units || '',
        scale: 1.0,
        offset: 0.0
      })
    }
  }

  if (pointPayloads.length > 0) {
    try {
      await request.post(
        `/api/channels/${props.channelId}/devices/${props.deviceId}/points`,
        pointPayloads
      )
      successCount = pointPayloads.length
    } catch (e) {
      console.error(e)
      failCount = pointPayloads.length
    }
  }
  
  state.loading = false
  showMessage(`已添加 ${successCount} 个点位${failCount > 0 ? `，${failCount} 个失败` : ''}`, failCount > 0 ? 'warning' : 'success')
  emit('update:visible', false)
  emit('refresh-points')
}

// 关闭弹窗
const handleClose = (value) => {
  if (!value) {
    emit('close')
    emit('refresh-points')
  }
}

// 扁平化OPC UA节点
const flattenOpcNodes = (nodes, level = 0) => {
  let result = []
  for (const node of nodes) {
    // Add current node
    const item = {
      ...node,
      level: level,
      isOpcNode: true,
      // Map to common fields for display
      device_id: node.node_id, // Use NodeID as ID
      object_name: node.name,
      name: node.name,
      type: node.type, // "Variable" or "Folder"
      description: node.node_id, // Show NodeID in description/extra
      unique_id: node.node_id
    }
    // Mark existing/new for sync status
    if (node.type === 'Variable' && node.node_id) {
      item.is_exists = props.existingAddresses.has(node.node_id)
    }
    result.push(item)
    
    // Process children
    if (node.children && node.children.length > 0) {
      result = result.concat(flattenOpcNodes(node.children, level + 1))
    }
  }
  return result
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
  background: var(--edgex-surface-raised);
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
  background: var(--edgex-surface-raised);
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
</style>
