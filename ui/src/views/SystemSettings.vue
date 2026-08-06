<template>
  <div class="page-shell page-shell--wide system-settings-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">系统设置</h2>
        <p class="page-subtitle">时间、网络、高可用、认证与数据维护</p>
      </div>
    </div>

    <a-tabs v-model:active-key="activeTab" type="rounded" size="small" class="main-tabs">
      <a-tab-pane key="time" title="时间同步" />
      <a-tab-pane key="network" title="网络接口" />
      <a-tab-pane key="routes" title="静态路由" />
      <a-tab-pane key="ha" title="高可用集群" />
      <a-tab-pane key="hostname" title="主机设置" />
      <a-tab-pane key="ldap" title="LDAP 认证" />
      <a-tab-pane key="status" title="系统维护" />
      <a-tab-pane key="data" title="数据管理" />
    </a-tabs>

    <div class="settings-body">
      <TimeSettings
        v-if="activeTab === 'time'"
        v-model="timeConfig"
        @save="saveConfig"
      />

      <NetworkSettings
        v-if="activeTab === 'network' || activeTab === 'routes'"
        :active-tab="activeTab"
        :network-interfaces="networkInterfaces"
        :connectivity-targets="connectivityTargets"
        :static-routes="staticRoutes"
        :network-backend="networkBackend"
        :connectivity-report="connectivityReport"
        :connectivity-checking="connectivityChecking"
        @update:network-interfaces="(value) => networkInterfaces = value"
        @update:connectivity-targets="(value) => connectivityTargets = value"
        @update:static-routes="(value) => staticRoutes = value"
        @save="saveConfig"
        @check-connectivity="runConnectivityCheck"
        :on-add-route="addRoute"
        :on-delete-route="deleteRoute"
      />

      <HASettings
        v-if="activeTab === 'ha'"
        v-model="haConfig"
        @save="saveConfig"
      />

      <HostnameSettings
        v-if="activeTab === 'hostname'"
        v-model="hostnameConfig"
        :network-interfaces="networkInterfaces"
        :access-status="hostnameAccessStatus"
        @save="saveConfig"
      />

      <LDAPSettings
        v-if="activeTab === 'ldap'"
        v-model="ldapConfig"
        @save="saveConfig"
      />

      <SystemMaintenance v-if="activeTab === 'status'" />

      <DatabaseManagement v-if="activeTab === 'data'" />
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { Message } from '@arco-design/web-vue'
import request from '@/utils/request'

import TimeSettings from './components/TimeSettings.vue'
import NetworkSettings from './components/NetworkSettings.vue'
import HASettings from './components/HASettings.vue'
import HostnameSettings from './components/HostnameSettings.vue'
import LDAPSettings from './components/LDAPSettings.vue'
import SystemMaintenance from './components/SystemMaintenance.vue'
import DatabaseManagement from './components/DatabaseManagement.vue'

const activeTab = ref('time')

const timeConfig = reactive({
  mode: 'manual',
  manual: {
    datetime: '',
    timezone: 'Asia/Shanghai',
    sync_rtc: true
  },
  ntp: {
    servers: ['pool.ntp.org'],
    interval: 1,
    enabled: true
  }
})

const networkInterfaces = ref([])
const connectivityTargets = ref([])
const staticRoutes = ref([])
const networkBackend = ref({ type: '', label: '' })
const connectivityReport = ref(null)
const connectivityChecking = ref(false)

const haConfig = reactive({
  role: 'master',
  heartbeat_type: 'UDP',
  interval: 2,
  timeout: 5,
  retries: 3
})

const hostnameConfig = reactive({
  name: 'edgeCore',
  enable_mdns: true,
  enable_bare: true,
  http_port: 8080,
  https_port: 443,
  interfaces: []
})
const hostnameAccessStatus = ref(null)

const ldapConfig = reactive({
  enabled: false,
  server: '',
  port: 389,
  base_dn: '',
  bind_dn: '',
  bind_password: '',
  user_filter: '(uid=%s)',
  attributes: '',
  use_ssl: false,
  skip_verify: false
})

const API_BASE = '/api/system'

const safeArray = (arr) => Array.isArray(arr) ? arr : []

const loadRoutes = async () => {
  try {
    const routesRes = await request.get(API_BASE + '/network/routes')
    if (Array.isArray(routesRes)) {
      staticRoutes.value = routesRes
      return
    }
  } catch (e) {
    console.error('Failed to load routes', e)
  }
}

const loadHostnameStatus = async () => {
  try {
    hostnameAccessStatus.value = await request.get(API_BASE + '/hostname/status')
  } catch (e) {
    console.error('Failed to load hostname access status', e)
  }
}

const loadConfig = async () => {
  try {
    const [sysRes, netRes, routesRes, infoRes] = await Promise.all([
      request.get(API_BASE),
      request.get(API_BASE + '/network/interfaces'),
      request.get(API_BASE + '/network/routes'),
      request.get(API_BASE + '/network/info').catch(() => null)
    ])

    const configData = sysRes || {}
    if (configData.time) Object.assign(timeConfig, configData.time)
    if (configData.ha) Object.assign(haConfig, configData.ha)
    if (configData.hostname) Object.assign(hostnameConfig, configData.hostname)
    if (configData.ldap) Object.assign(ldapConfig, configData.ldap)
    if (Array.isArray(configData.connectivity_targets)) {
      connectivityTargets.value = configData.connectivity_targets
    }

    if (infoRes && infoRes.label) {
      networkBackend.value = infoRes
    }

    if (Array.isArray(netRes)) {
      networkInterfaces.value = normalizeInterfaces(netRes, configData.network)
    } else if (Array.isArray(configData.network)) {
      networkInterfaces.value = normalizeInterfaces(configData.network)
    }

    if (Array.isArray(routesRes)) {
      staticRoutes.value = routesRes
    } else if (Array.isArray(configData.routes)) {
      staticRoutes.value = configData.routes
    }

    await loadHostnameStatus()
  } catch (e) {
    console.error('Failed to load system config', e)
  }
}

const normalizeInterfaces = (interfaces, configured = []) => {
  const cfgMap = new Map((configured || []).map(item => [item.name, item]))
  return (interfaces || []).map(iface => {
    const cfg = cfgMap.get(iface.name)
    const cleanedIface = {
      ...iface,
      enabled: cfg?.enabled ?? iface.enabled ?? true,
      ip_configs: safeArray(iface.ip_configs).filter(ip => ip && ip.address && typeof ip.address === 'string'),
      gateways: safeArray(iface.gateways).filter(g => g && g.gateway)
    }
    if (cfg?.ip_configs?.length) {
      cleanedIface.ip_configs = safeArray(cfg.ip_configs).filter(ip => ip && ip.address)
    }
    if (cfg?.gateways?.length) {
      cleanedIface.gateways = safeArray(cfg.gateways).filter(g => g && g.gateway)
    }
    return cleanedIface
  })
}

const runConnectivityCheck = async () => {
  if (!connectivityTargets.value.length) {
    Message.info('请先添加连通性检查目标')
    return
  }
  connectivityChecking.value = true
  try {
    const report = await request.post(API_BASE + '/network/connectivity', connectivityTargets.value)
    connectivityReport.value = report
    if (report?.success) {
      Message.success('连通性验证通过')
    } else {
      Message.warning('连通性验证失败')
    }
  } catch (e) {
    Message.error('连通性检测失败')
    console.error('Connectivity check failed', e)
  } finally {
    connectivityChecking.value = false
  }
}

const addRoute = async (route) => {
  try {
    await request.post(API_BASE + '/network/routes', route)
    await loadRoutes()
    Message.success('路由已添加')
  } catch (e) {
    Message.error('添加路由失败')
    console.error('Failed to add route', e)
    throw e
  }
}

const deleteRoute = async (route) => {
  try {
    await request.delete(API_BASE + '/network/routes', { data: route })
    await loadRoutes()
    Message.success('路由已删除')
  } catch (e) {
    Message.error('删除路由失败')
    console.error('Failed to delete route', e)
    throw e
  }
}

const saveConfig = async () => {
  const fullConfig = {
    time: timeConfig,
    network: networkInterfaces.value,
    routes: staticRoutes.value,
    ha: haConfig,
    hostname: hostnameConfig,
    ldap: ldapConfig,
    connectivity_targets: connectivityTargets.value
  }

  try {
    await request.put(API_BASE, fullConfig)
    await loadRoutes()
    await loadHostnameStatus()
    Message.success('配置已保存')
    if (connectivityTargets.value.length) {
      await runConnectivityCheck()
    } else {
      connectivityReport.value = null
    }
  } catch (e) {
    Message.error('保存配置失败')
    console.error('Failed to save config', e)
  }
}

onMounted(loadConfig)
</script>

<style scoped>
/* v3.0 — styles in src/styles/system-settings.css */
</style>
