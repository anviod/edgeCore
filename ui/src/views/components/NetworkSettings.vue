<template>
  <!-- 网络接口 -->
  <a-card v-if="activeTab === 'network'" class="settings-panel">
    <a-card-header>
      <div class="card-title-row">
        <div class="card-title">网络接口</div>
        <span v-if="networkBackend.label" class="backend-badge">管理工具: {{ networkBackend.label }}</span>
      </div>
    </a-card-header>
    <a-card-body>
      <div class="table-container saas-table">
        <a-table 
          :columns="networkColumns" 
          :data="networkInterfaces" 
          size="small"
          :bordered="false"
          class="industrial-table-inline"
        >
          <template #name="{ record }">
            <span class="mono-text bold">{{ record.name }}</span>
          </template>
          <template #status="{ record }">
            <a-tag :color="record.status === 'UP' ? 'green' : 'red'" size="small">
              <template #icon>
                <icon-check-circle v-if="record.status === 'UP'" />
                <icon-close-circle v-else />
              </template>
              {{ record.status }}
            </a-tag>
          </template>
          <template #ip_configs="{ record }">
            <div class="subscribers-line">
              <template v-if="safeIPs(record).length">
                <span
                  v-for="(ipConf, idx) in safeIPs(record)"
                  :key="idx"
                  class="sub-item mono-text"
                >
                  {{ ipConf.address }}/{{ ipConf.prefix }} ({{ ipConf.version }})
                </span>
              </template>

              <span v-else class="text-gray-400 text-xs">
                --
              </span>
            </div>
          </template>
          <template #actions="{ record }">
            <a-button type="text" size="small" @click="editInterface(record)">
              <template #icon><icon-edit /></template>
              配置
            </a-button>
          </template>
        </a-table>
      </div>

      <a-divider />

      <div class="table-toolbar-industrial">
        <span class="toolbar-title">连通性验证（配置变更时自动检查）</span>
        <div class="toolbar-actions">
          <a-button size="small" :loading="connectivityChecking" @click="$emit('check-connectivity')">
            立即检测
          </a-button>
          <a-button type="primary" size="small" @click="addConnectivityTarget">
            <template #icon><icon-plus /></template>
            添加检查目标
          </a-button>
        </div>
      </div>
      <div v-if="connectivityReport" class="connectivity-report" :class="connectivityReport.success ? 'is-ok' : 'is-fail'">
        <span>{{ connectivityReport.success ? '连通性验证通过' : '连通性验证失败' }}</span>
        <span v-for="(item, idx) in connectivityReport.details || []" :key="idx" class="report-item">
          {{ item.target }}: {{ item.message }}
        </span>
      </div>
      <div class="table-container saas-table">
        <a-table :columns="connectivityColumns" :data="connectivityTargets" size="small" :bordered="false" class="industrial-table-inline">
          <template #type="{ index }">
            <a-select v-model="connectivityTargets[index].type" :options="connectivityTypeOptions" size="small" class="rect-input" @change="onConnectivityChange" />
          </template>
          <template #target="{ index }">
            <a-input v-model="connectivityTargets[index].target" placeholder="例如: 8.8.8.8 或 http://baidu.com" size="small" class="rect-input" @change="onConnectivityChange" />
          </template>
          <template #timeout="{ index }">
            <a-input-number v-model="connectivityTargets[index].timeout" :min="1" size="small" class="rect-input" @change="onConnectivityChange" />
          </template>
          <template #actions="{ index }">
            <a-button type="text" size="small" status="danger" @click="removeConnectivityTarget(index)">
              <template #icon><icon-delete /></template>
            </a-button>
          </template>
        </a-table>
      </div>
    </a-card-body>
  </a-card>

  <a-card v-if="activeTab === 'routes'" class="settings-panel">
    <a-card-header class="card-title-row">
      <div class="card-title">静态路由</div>
      <a-button type="primary" @click="openRouteDialog()">
        <template #icon><icon-plus /></template>
        添加路由
      </a-button>
    </a-card-header>
    <a-card-body>
      <div class="table-container saas-table">
        <a-table :columns="routesColumns" :data="routeRows" size="small" :bordered="false" class="industrial-table-inline">
          <template #destination="{ record }">
            {{ record.destination }}/{{ record.prefix }}
          </template>
          <template #enabled="{ index }">
            <a-switch :model-value="Boolean(routeRows[index]?.enabled)" @change="(value) => setRouteEnabled(index, value)" />
          </template>
          <template #actions="{ record, index }">
            <a-button type="text" size="small" @click="openRouteDialog(record, index)">
              <template #icon><icon-edit /></template>
            </a-button>
            <a-button type="text" size="small" status="danger" @click="deleteRoute(index)">
              <template #icon><icon-delete /></template>
            </a-button>
          </template>
        </a-table>
      </div>
    </a-card-body>
  </a-card>
  <a-modal 
    v-model:visible="interfaceDialog.visible" 
    :title="`编辑接口: ${interfaceDialog.form.name}`" 
    width="800px"
    modal-class="industrial-white-modal"
  >
    <a-tabs v-model:active-key="interfaceDialog.activeTab" class="mb-4">
      <a-tab-pane key="ip" title="IP 地址" />
      <a-tab-pane key="gateway" title="网关设置" />
      <a-tab-pane key="advanced" title="链路参数" />
    </a-tabs>

    <!-- IP 地址配置 -->
    <div v-if="interfaceDialog.activeTab === 'ip'">
      <div class="table-toolbar-industrial mb-4">
        <div class="flex items-center gap-2">
          <a-button type="primary" size="small" @click="addIpConfig">
            <template #icon><icon-plus /></template>
            添加新条目
          </a-button>
        </div>
      </div>
      <a-table 
        :key="interfaceDialog.form.name"
        :columns="ipConfigColumns" 
        :data="interfaceDialog.form.ip_configs" 
        size="small"
        :bordered="false"
      >
        <template #address="{ record }">
          <a-input v-model="record.address" size="small" class="rect-input" />
        </template>
        <template #prefix="{ record }">
          <a-input-number v-model="record.prefix" :min="1" :max="128" size="small" class="rect-input" />
        </template>
        <template #version="{ record }">
          <a-select v-model="record.version" :options="[{ label: 'IPv4', value: 'IPv4' }, { label: 'IPv6', value: 'IPv6' }]" size="small" class="rect-input" />
        </template>
        <template #source="{ record }">
          <a-select v-model="record.source" :options="[{ label: '静态', value: 'Static' }, { label: 'DHCP', value: 'DHCP' }]" size="small" class="rect-input" />
        </template>
        <template #enabled="{ record }">
          <a-switch v-model="record.enabled" />
        </template>
        <template #actions="{ index }">
          <a-button type="text" status="danger" size="small" @click="removeIpConfig(index)">
            <template #icon><icon-delete /></template>
          </a-button>
        </template>
      </a-table>
    </div>

    <!-- 网关设置 -->
    <div v-if="interfaceDialog.activeTab === 'gateway'">
      <div class="table-toolbar-industrial mb-4">
        <div class="flex items-center gap-2">
          <a-button type="primary" size="small" @click="addGatewayConfig">
            <template #icon><icon-plus /></template>
            添加网关
          </a-button>
        </div>
      </div>
      <a-table 
        :key="interfaceDialog.form.name"
        :columns="gatewayConfigColumns" 
        :data="interfaceDialog.form.gateways" 
        size="small" 
        :bordered="false"
      >
        <template #gateway="{ record }">
          <a-input v-model="record.gateway" size="small" class="rect-input" />
        </template>
        <template #metric="{ record }">
          <a-input-number v-model="record.metric" :min="1" size="small" class="rect-input" />
        </template>
        <template #scope="{ record }">
          <a-select v-model="record.scope" :options="[{ label: '全局', value: 'Global' }, { label: '链路', value: 'Link' }]" size="small" class="rect-input" />
        </template>
        <template #enabled="{ record }">
          <a-switch v-model="record.enabled" />
        </template>
        <template #actions="{ index }">
          <a-button type="text" size="small" status="danger" @click="removeGatewayConfig(index)">
            <template #icon><icon-delete /></template>
          </a-button>
        </template>
      </a-table>
    </div>

    <!-- 链路参数 -->
    <div v-if="interfaceDialog.activeTab === 'advanced'">
      <a-form :model="interfaceDialog.form" layout="vertical">
        <a-form-item field="mtu" label="MTU 大小">
          <a-input-number v-model="interfaceDialog.form.mtu" :min="64" :max="9000" class="rect-input" />
          <div class="text-gray-500 text-sm">范围: 64-9000 字节</div>
        </a-form-item>
        <a-form-item field="interface_metric" label="优先级">
          <a-input-number v-model="interfaceDialog.form.interface_metric" class="rect-input" />
        </a-form-item>
        <a-form-item field="mac" label="MAC 地址">
          <a-input v-model="interfaceDialog.form.mac" disabled class="rect-input" />
        </a-form-item>
      </a-form>
    </div>

    <template #footer>
      <a-button @click="interfaceDialog.visible = false">取消</a-button>
      <a-button type="primary" @click="saveInterface">提交接口更改</a-button>
    </template>
  </a-modal>

  <!-- Route Edit Dialog -->
  <a-modal 
    v-model:visible="routeDialog.visible" 
    :title="routeDialog.index === -1 ? '添加路由' : '编辑路由'" 
    width="600px"
    modal-class="industrial-white-modal"
  >
    <a-form :model="routeDialog.form" layout="vertical">
      <a-form-item field="destination" label="目标网段">
        <a-input v-model="routeDialog.form.destination" placeholder="例如: 192.168.2.0" class="rect-input" />
      </a-form-item>
      <a-form-item field="prefix" label="掩码长度">
        <a-input-number v-model="routeDialog.form.prefix" :min="1" :max="128" class="rect-input" />
      </a-form-item>
      <a-form-item field="gateway" label="下一跳">
        <a-input v-model="routeDialog.form.gateway" class="rect-input" />
      </a-form-item>
      <a-form-item field="interface" label="出接口">
        <a-select v-model="routeDialog.form.interface" :options="interfaceOptions" class="rect-input" />
      </a-form-item>
      <a-form-item field="metric" label="优先级">
        <a-input-number v-model="routeDialog.form.metric" :min="1" class="rect-input" />
      </a-form-item>
      <a-form-item field="enabled" label="启用">
        <a-switch v-model="routeDialog.form.enabled" type="round" />
      </a-form-item>
    </a-form>
    <template #footer>
      <a-button @click="routeDialog.visible = false">取消</a-button>
      <a-button type="primary" @click="saveRoute">保存路由</a-button>
    </template>
  </a-modal>
</template>

<script setup>
import { ref, reactive, computed, watch, onBeforeUnmount } from 'vue'
import { IconPlus, IconDelete, IconEdit, IconCheckCircle, IconCloseCircle } from '@arco-design/web-vue/es/icon'

const props = defineProps({
  activeTab: {
    type: String,
    default: 'network'
  },
  networkInterfaces: {
    type: Array,
    default: () => []
  },
  connectivityTargets: {
    type: Array,
    default: () => []
  },
  staticRoutes: {
    type: Array,
    default: () => []
  },
  networkBackend: {
    type: Object,
    default: () => ({ type: '', label: '' })
  },
  connectivityReport: {
    type: Object,
    default: null
  },
  connectivityChecking: {
    type: Boolean,
    default: false
  },
  onAddRoute: {
    type: Function,
    default: null
  },
  onDeleteRoute: {
    type: Function,
    default: null
  }
})

const emit = defineEmits(['update:networkInterfaces', 'update:connectivityTargets', 'update:staticRoutes', 'save', 'check-connectivity'])

// 监听 activeTab 变化，关闭弹窗
watch(() => props.activeTab, (newTab) => {
  if (newTab !== 'network' && newTab !== 'routes') {
    interfaceDialog.visible = false
    routeDialog.visible = false
  }
})

// 组件销毁前关闭弹窗
onBeforeUnmount(() => {
  interfaceDialog.visible = false
  routeDialog.visible = false
})

// 表格列配置
const networkColumns = [
  { title: '接口名', dataIndex: 'name', key: 'name', slotName: 'name' },
  { title: 'MAC 地址', dataIndex: 'mac', key: 'mac' },
  { title: '链路状态', dataIndex: 'status', key: 'status', slotName: 'status' },
  { title: 'IP 地址', dataIndex: 'ip_configs', key: 'ip_configs', slotName: 'ip_configs' },
  { title: '操作', dataIndex: 'actions', key: 'actions', slotName: 'actions' }
]

const connectivityColumns = [
  { title: '类型', dataIndex: 'type', key: 'type', slotName: 'type' },
  { title: '目标 (IP/URL)', dataIndex: 'target', key: 'target', slotName: 'target' },
  { title: '超时 (秒)', dataIndex: 'timeout', key: 'timeout', slotName: 'timeout' },
  { title: '操作', dataIndex: 'actions', key: 'actions', slotName: 'actions' }
]

const routesColumns = [
  { title: '目标网段', dataIndex: 'destination', key: 'destination', slotName: 'destination' },
  { title: '网关', dataIndex: 'gateway', key: 'gateway' },
  { title: '出接口', dataIndex: 'interface', key: 'interface' },
  { title: '优先级', dataIndex: 'metric', key: 'metric' },
  { title: '状态', dataIndex: 'enabled', key: 'enabled', slotName: 'enabled' },
  { title: '操作', dataIndex: 'actions', key: 'actions', slotName: 'actions' }
]

const ipConfigColumns = [
  { title: '地址', dataIndex: 'address', key: 'address', slotName: 'address' },
  { title: '掩码长度', dataIndex: 'prefix', key: 'prefix', slotName: 'prefix' },
  { title: '版本', dataIndex: 'version', key: 'version', slotName: 'version' },
  { title: '来源', dataIndex: 'source', key: 'source', slotName: 'source' },
  { title: '启用', dataIndex: 'enabled', key: 'enabled', slotName: 'enabled' },
  { title: '操作', dataIndex: 'actions', key: 'actions', slotName: 'actions' }
]

const gatewayConfigColumns = [
  { title: '网关地址', dataIndex: 'gateway', key: 'gateway', slotName: 'gateway' },
  { title: 'Metric', dataIndex: 'metric', key: 'metric', slotName: 'metric' },
  { title: '范围', dataIndex: 'scope', key: 'scope', slotName: 'scope' },
  { title: '启用', dataIndex: 'enabled', key: 'enabled', slotName: 'enabled' },
  { title: '操作', dataIndex: 'actions', key: 'actions', slotName: 'actions' }
]

const connectivityTypeOptions = [
  { label: 'gateway', value: 'gateway' },
  { label: 'ip', value: 'ip' },
  { label: 'http', value: 'http' }
]

const safeArray = (arr) => Array.isArray(arr) ? arr : []

const normalizeIP = (ip) => {
  if (!ip || typeof ip.address !== 'string' || !ip.address.trim()) return null

  const isIPv6 = ip.address.includes(':')
  return {
    address: ip.address,
    prefix: ip.prefix ?? (isIPv6 ? 64 : 24),
    version: ip.version ?? (isIPv6 ? 'IPv6' : 'IPv4'),
    source: ip.source ?? 'Static',
    enabled: ip.enabled ?? true
  }
}

const safeIPs = (record) => {
  if (!record) return []
  return safeArray(record.ip_configs)
    .map(normalizeIP)
    .filter(ip => ip)
}

const routeRows = computed(() => safeArray(props.staticRoutes).map((route) => {
  if (!route || typeof route !== 'object') {
    return { destination: '', prefix: 24, gateway: '', interface: '', metric: 100, enabled: false }
  }
  return {
    destination: typeof route.destination === 'string' ? route.destination : '',
    prefix: Number.isFinite(route.prefix) ? route.prefix : 24,
    gateway: typeof route.gateway === 'string' ? route.gateway : '',
    interface: typeof route.interface === 'string' ? route.interface : '',
    metric: Number.isFinite(route.metric) ? route.metric : 100,
    enabled: Boolean(route.enabled)
  }
}))

const interfaceOptions = computed(() => safeArray(props.networkInterfaces)
  .map((iface) => ({
    label: iface?.name || '',
    value: iface?.name || ''
  }))
  .filter((item) => item.value))

const interfaceDialog = reactive({
  visible: false,
  activeTab: 'ip',
  form: { 
    name: '', 
    mac: '',
    mtu: 1500,
    interface_metric: 100,
    ip_configs: [],
    gateways: []
  }
})

const routeDialog = reactive({
  visible: false,
  index: -1, // -1 for new
  form: {
    destination: '',
    prefix: 24,
    gateway: '',
    interface: '',
    metric: 100,
    enabled: true
  }
})

const editInterface = (iface) => {
  if (!iface) return
  
  // ✅ 强制结构标准化 + 深拷贝
  const ipConfigs = safeArray(iface.ip_configs).map(ip => {
    const n = normalizeIP(ip)
    if (!n) return null
    return {
      address: n.address,
      prefix: n.prefix,
      version: n.version,
      source: n.source,
      enabled: n.enabled
    }
  }).filter(Boolean)

  const gateways = safeArray(iface.gateways).map(g => ({
    gateway: g?.gateway || '',
    metric: g?.metric ?? 100,
    scope: g?.scope ?? 'Global',
    enabled: g?.enabled ?? true
  }))

  // ✅ 不保留任何旧引用
  interfaceDialog.form = {
    name: iface.name || '',
    mac: iface.mac || '',
    mtu: iface.mtu || 1500,
    interface_metric: iface.interface_metric || 100,
    ip_configs: ipConfigs,
    gateways: gateways
  }

  interfaceDialog.activeTab = 'ip'
  interfaceDialog.visible = true
}

const saveInterface = () => {
  const idx = props.networkInterfaces.findIndex(i => i.name === interfaceDialog.form.name)
  if (idx !== -1) {
    const updatedInterfaces = [...props.networkInterfaces]
    updatedInterfaces[idx] = { ...interfaceDialog.form }
    emit('update:networkInterfaces', updatedInterfaces)
    emit('save')
  }
  interfaceDialog.visible = false
}

const addIpConfig = () => {
  interfaceDialog.form.ip_configs.push({
    address: '', prefix: 24, version: 'IPv4', source: 'Static', enabled: true
  })
}

const removeIpConfig = (idx) => {
  interfaceDialog.form.ip_configs.splice(idx, 1)
}

const addGatewayConfig = () => {
  interfaceDialog.form.gateways.push({
    gateway: '', metric: 100, interface: interfaceDialog.form.name, scope: 'Global', enabled: true
  })
}

const addConnectivityTarget = () => {
  const updatedTargets = [...props.connectivityTargets]
  updatedTargets.push({
    type: 'ip', target: '', timeout: 2
  })
  emit('update:connectivityTargets', updatedTargets)
  emit('save')
}

const onConnectivityChange = () => {
  emit('save')
}

const removeConnectivityTarget = (idx) => {
  const updatedTargets = [...props.connectivityTargets]
  updatedTargets.splice(idx, 1)
  emit('update:connectivityTargets', updatedTargets)
  emit('save')
}

const removeGatewayConfig = (idx) => {
  interfaceDialog.form.gateways.splice(idx, 1)
}

const openRouteDialog = (route = null, index = -1) => {
  routeDialog.index = index
  if (route) {
    routeDialog.form = { ...route }
  } else {
    routeDialog.form = { destination: '', prefix: 24, gateway: '', interface: '', metric: 100, enabled: true }
  }
  routeDialog.visible = true
}

const saveRoute = async () => {
  const route = { ...routeDialog.form }
  if (!route.destination?.trim()) {
    return
  }
  try {
    if (routeDialog.index === -1) {
      if (props.onAddRoute) {
        await props.onAddRoute(route)
      } else {
        const updatedRoutes = [...props.staticRoutes, route]
        emit('update:staticRoutes', updatedRoutes)
        emit('save')
      }
    } else {
      const updatedRoutes = [...props.staticRoutes]
      updatedRoutes[routeDialog.index] = route
      emit('update:staticRoutes', updatedRoutes)
      emit('save')
    }
    routeDialog.visible = false
  } catch {
    // Parent shows error message.
  }
}

const deleteRoute = async (idx) => {
  const route = props.staticRoutes[idx]
  if (!route) return
  try {
    if (props.onDeleteRoute) {
      await props.onDeleteRoute(route)
    } else {
      const updatedRoutes = [...props.staticRoutes]
      updatedRoutes.splice(idx, 1)
      emit('update:staticRoutes', updatedRoutes)
      emit('save')
    }
  } catch {
    // Parent shows error message.
  }
}

const setRouteEnabled = (idx, enabled) => {
  const updatedRoutes = safeArray(props.staticRoutes).map((route) => {
    if (!route || typeof route !== 'object') return route
    return { ...route }
  })
  if (!updatedRoutes[idx] || typeof updatedRoutes[idx] !== 'object') return
  updatedRoutes[idx] = { ...updatedRoutes[idx], enabled: Boolean(enabled) }
  emit('update:staticRoutes', updatedRoutes)
  emit('save')
}
</script>

<style scoped>
.card-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.backend-badge {
  font-size: 12px;
  color: var(--color-text-3);
}

.toolbar-actions {
  display: flex;
  gap: 8px;
}

.connectivity-report {
  margin-bottom: 12px;
  padding: 8px 12px;
  border-radius: 4px;
  font-size: 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.connectivity-report.is-ok {
  background: rgba(0, 180, 42, 0.08);
  color: #00b42a;
}

.connectivity-report.is-fail {
  background: rgba(245, 63, 63, 0.08);
  color: #f53f3f;
}

.report-item {
  color: var(--color-text-2);
}
</style>
