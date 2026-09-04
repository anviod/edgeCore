<template>
  <a-modal
    :visible="visible"
    title="克隆其它设备点位"
    width="100%"
    @ok="handleOk"
    @cancel="handleCancel"
    @update:visible="(val) => emit('update:visible', val)"
  >
    <a-space direction="vertical" :size="16" fill>
      <!-- 条件区 -->
      <a-row :gutter="16">
        <a-col :span="8">
          <a-select
            v-model="selectedChannel"
            :options="channels"
            placeholder="选择通道"
            :loading="loading"
            allow-clear
            @change="onChannelChange"
          />
        </a-col>

        <a-col :span="8">
          <a-select
            v-model="selectedDevice"
            :options="devices"
            placeholder="选择设备"
            :disabled="!selectedChannel"
            allow-clear
            @change="onDeviceChange"
          />
        </a-col>

        <a-col :span="8">
          <a-input v-model="search" allow-clear placeholder="过滤">
            <template #prefix><IconSearch /></template>
          </a-input>
        </a-col>
      </a-row>

      <!-- 统计 -->
      <div>
        已选择 {{ selectedRowKeys.length }} / {{ tableData.length }}
      </div>

      <!-- ✅ 表格（永远存在） -->
      <a-table
        row-key="id"
        :loading="loading"
        :columns="columns"
        :data="tableData"
        :pagination="false"
        :row-selection="rowSelection"
        :selected-keys="selectedRowKeys"
        @selection-change="onSelectionChange"
        :scroll="{ y: 360 }"
      />

      <!-- 空态只作为覆盖 -->
      <a-empty
        v-if="!loading && tableData.length === 0"
        description="暂无数据"
      />
    </a-space>
  </a-modal>
</template>

<script setup>
import { ref, reactive, watch } from 'vue'
import { IconSearch } from '@arco-design/web-vue/es/icon'
import request from '@/utils/request'

const props = defineProps({
  visible: Boolean,
  channelProtocol: String
})

const emit = defineEmits(['update:visible', 'success', 'error'])

const loading = ref(false)

const channels = ref([])
const devices = ref([])
const points = ref([])

const selectedChannel = ref(null)
const selectedDevice = ref(null)
const selectedRowKeys = ref([])
const search = ref('')
const tableData = ref([])

/* ✅ 列 */
const columns = [
  { title: '点位ID', dataIndex: 'id', width: 120 },
  { title: '名称', dataIndex: 'name', width: 150 },
  { title: '地址', dataIndex: 'address', width: 100 },
  { title: '类型', dataIndex: 'datatype', width: 100 },
  { title: '单位', dataIndex: 'unit', width: 80 },
  { title: '权限', dataIndex: 'readwrite', width: 80 }
]

/* ✅ 核心：完全参考官方例子 */
const rowSelection = reactive({
  type: 'checkbox',
  showCheckedAll: true,
})

const onSelectionChange = (keys) => {
  selectedRowKeys.value = keys || []
}

// 监听 search 和 points 变化，更新 tableData
watch([search, points], () => {
  const key = search.value.toLowerCase()
  tableData.value = (points.value || []).filter(p =>
    !key ||
    (p.name || '').toLowerCase().includes(key) ||
    (p.address || '').toLowerCase().includes(key)
  )
}, { deep: true })

/* 初始化 */
const init = async () => {
  loading.value = true
  try {
    selectedChannel.value = null
    selectedDevice.value = null
    selectedRowKeys.value = []
    channels.value = []
    devices.value = []
    points.value = []
    tableData.value = []

    const chs = await request.get('/api/channels')
    channels.value = (chs || [])
      .filter(c => c.protocol === props.channelProtocol)
      .map(c => ({
        label: `${c.id}(${c.name})`,
        value: c.id
      }))
  } finally {
    loading.value = false
  }
}

/* 通道 */
const onChannelChange = async (cid) => {
  selectedDevice.value = null
  devices.value = []
  points.value = []
  tableData.value = []
  selectedRowKeys.value = []

  if (!cid) return

  loading.value = true
  try {
    const devs = await request.get(`/api/channels/${cid}/devices`)
    devices.value = (devs || []).map(d => ({
      label: d.name,
      value: d.id
    }))
  } finally {
    loading.value = false
  }
}

/* 设备 */
const onDeviceChange = async (did) => {
  points.value = []
  tableData.value = []
  selectedRowKeys.value = []

  if (!did) return

  loading.value = true
  try {
    const pts = await request.get(
      `/api/channels/${selectedChannel.value}/devices/${did}/points`
    )

    // ✅ 强制 string key（关键）
    points.value = (pts || []).map(p => ({
      ...p,
      id: String(p.id)
    }))
    
    // 手动更新 tableData
    const key = search.value.toLowerCase()
    tableData.value = points.value.filter(p =>
      !key ||
      (p.name || '').toLowerCase().includes(key) ||
      (p.address || '').toLowerCase().includes(key)
    )
  } finally {
    loading.value = false
  }
}

/* 确认 */
const handleOk = () => {
  const set = new Set(selectedRowKeys.value)
  const result = points.value.filter(p => set.has(p.id))

  if (!result.length) {
    emit('error', '请选择点位')
    return
  }

  emit('success', result)
  handleCancel()
}

/* 关闭 */
const handleCancel = () => {
  emit('update:visible', false)

  selectedChannel.value = null
  selectedDevice.value = null
  selectedRowKeys.value = []
  points.value = []
  search.value = ''
}

/* 监听 */
watch(() => props.visible, v => v && init())
</script>
