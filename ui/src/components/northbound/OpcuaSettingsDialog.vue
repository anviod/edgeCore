<template>
  <a-modal
    v-model:visible="visible"
    title="OPC UA 服务端"
    :width="720"
    modal-class="northbound-settings-modal"
    unmount-on-close
    :footer="true"
    :mask-closable="false"
  >
    <div class="nb-mode-banner nb-mode-banner--passive">
      <span class="nb-mode-banner__tag">被动读取</span>
      <span>网关作为 OPC UA Server 运行，SCADA / MES 连接后主动订阅读取点位数据</span>
    </div>

    <a-tabs v-model:active-key="activeTab" type="rounded" size="small">
      <a-tab-pane key="basic">
        <template #title>服务配置</template>
        <a-form :model="form" layout="vertical" class="industrial-form form-controls-md">
          <a-row :gutter="16">
            <a-col :span="16">
              <a-form-item label="通道名称" required>
                <a-input v-model="form.name" placeholder="例如: 工厂 SCADA OPC UA" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item label="启用">
                <a-switch v-model="form.enable" />
              </a-form-item>
            </a-col>
          </a-row>

          <div class="nb-form-section">
            <div class="nb-form-section__title">监听地址</div>
            <a-row :gutter="16">
              <a-col :span="8">
                <a-form-item label="端口" required>
                  <a-input-number v-model="form.port" :min="1" :max="65535" placeholder="4840" style="width: 100%" />
                </a-form-item>
              </a-col>
              <a-col :span="16">
                <a-form-item label="Endpoint 路径" required>
                  <a-input v-model="form.endpoint" placeholder="/ipp/opcua/server" class="mono-text" />
                </a-form-item>
              </a-col>
            </a-row>
            <a-alert type="info" style="margin-bottom: 12px">
              连接地址: <code class="mono-text">opc.tcp://&lt;网关IP&gt;:{{ form.port || 4840 }}{{ form.endpoint || '' }}</code>
            </a-alert>
          </div>

          <a-collapse :bordered="false">
            <a-collapse-item header="安全与认证（可选）" key="security">
              <a-form-item label="安全策略">
                <a-select v-model="form.security_policy">
                  <a-option value="Auto">自动（发布全部常用策略，推荐）</a-option>
                  <a-option value="None">None（不加密，仅调试）</a-option>
                  <a-option value="Basic256Sha256">Basic256Sha256</a-option>
                  <a-option value="Basic256">Basic256</a-option>
                  <a-option value="Basic128Rsa15">Basic128Rsa15</a-option>
                  <a-option value="Aes128_Sha256_RsaOaep">Aes128_Sha256_RsaOaep</a-option>
                  <a-option value="Aes256Sha256RsaPss">Aes256Sha256RsaPss</a-option>
                </a-select>
              </a-form-item>
              <a-form-item label="推荐 Security Mode">
                <a-select v-model="form.security_mode">
                  <a-option value="SignAndEncrypt">SignAndEncrypt（推荐）</a-option>
                  <a-option value="Sign">Sign</a-option>
                  <a-option value="None">None</a-option>
                  <a-option value="Auto">自动</a-option>
                </a-select>
                <template #extra>客户端连接时的首选模式；Auto 模式下服务端仍发布 Sign 与 SignAndEncrypt 端点。</template>
              </a-form-item>
              <a-form-item label="认证方式">
                <a-checkbox-group v-model="form.auth_methods">
                  <a-checkbox value="Anonymous">匿名</a-checkbox>
                  <a-checkbox value="UserName">用户名/密码</a-checkbox>
                  <a-checkbox value="Certificate">证书</a-checkbox>
                </a-checkbox-group>
              </a-form-item>
              <template v-if="form.auth_methods?.includes('UserName')">
                <div class="user-list-header">
                  <span>用户列表</span>
                  <a-button type="outline" size="mini" @click="addUser"><template #icon><icon-plus /></template>添加</a-button>
                </div>
                <div v-for="(user, index) in userList" :key="index" class="user-item">
                  <a-input v-model="user.username" placeholder="用户名" size="small" />
                  <a-input-password v-model="user.password" placeholder="密码" size="small" />
                  <a-button type="text" status="danger" size="mini" @click="userList.splice(index, 1)">
                    <template #icon><icon-delete /></template>
                  </a-button>
                </div>
              </template>
              <a-form-item label="服务端证书（Sign / SignAndEncrypt）">
                <div class="cert-upload-row">
                  <input ref="serverCertInput" type="file" accept=".pem,.crt,.cer" hidden @change="onServerCertFile" />
                  <a-button type="outline" size="small" @click="serverCertInput?.click()">上传证书 (.pem/.crt)</a-button>
                  <a-tag v-if="form.has_server_cert || pendingServerCert" color="green" size="small">
                    {{ pendingServerCert ? '待保存（新证书）' : '已配置' }}
                  </a-tag>
                  <a-tag v-else color="gray" size="small">未配置（启动时自动生成）</a-tag>
                </div>
              </a-form-item>
              <a-form-item label="服务端私钥">
                <div class="cert-upload-row">
                  <input ref="serverKeyInput" type="file" accept=".pem,.key" hidden @change="onServerKeyFile" />
                  <a-button type="outline" size="small" @click="serverKeyInput?.click()">上传私钥 (.pem/.key)</a-button>
                  <a-tag v-if="form.has_server_key || pendingServerKey" color="green" size="small">
                    {{ pendingServerKey ? '待保存（新私钥）' : '已配置' }}
                  </a-tag>
                  <a-tag v-else color="gray" size="small">未配置</a-tag>
                </div>
                <template #extra>私钥保存在数据库中，不会在 API 响应中返回；重新上传才会覆盖。</template>
              </a-form-item>
              <template v-if="form.auth_methods?.includes('Certificate')">
                <a-alert type="info" style="margin-bottom: 12px">
                  X509 客户端认证已启用。未上传「受信任客户端证书」时，默认信任自签名客户端证书（便于 Prosys / UaExpert 连接）。
                </a-alert>
              </template>
              <a-form-item label="受信任客户端证书（可选，严格 PKI）">
                <div class="cert-upload-row" style="margin-bottom: 8px">
                  <input ref="trustedCertInput" type="file" accept=".pem,.crt,.cer" multiple hidden @change="onTrustedCertFiles" />
                  <a-button type="outline" size="small" @click="trustedCertInput?.click()">添加信任证书</a-button>
                  <a-button v-if="trustedCertList.length" type="text" status="danger" size="small" @click="clearTrustedCerts">清空</a-button>
                </div>
                <div v-if="trustedCertList.length" class="trusted-cert-list">
                  <div v-for="(item, index) in trustedCertList" :key="index" class="trusted-cert-item">
                    <span class="mono-text">{{ item.label }}</span>
                    <a-button type="text" status="danger" size="mini" @click="trustedCertList.splice(index, 1); trustedCertsModified = true">
                      <template #icon><icon-delete /></template>
                    </a-button>
                  </div>
                </div>
                <template #extra>上传后仅接受列表中的客户端证书；留空则信任任意自签名客户端证书。</template>
              </a-form-item>
              <a-collapse :bordered="false" style="margin-top: 8px">
                <a-collapse-item header="高级：文件路径（兼容旧配置）" key="legacy-paths">
                  <a-row :gutter="16">
                    <a-col :span="12">
                      <a-form-item label="证书路径"><a-input v-model="form.cert_file" class="mono-text" placeholder="可选" /></a-form-item>
                    </a-col>
                    <a-col :span="12">
                      <a-form-item label="私钥路径"><a-input v-model="form.key_file" class="mono-text" placeholder="可选" /></a-form-item>
                    </a-col>
                  </a-row>
                  <a-form-item label="信任证书目录">
                    <a-input v-model="form.trusted_cert_path" placeholder="可选" class="mono-text" />
                  </a-form-item>
                </a-collapse-item>
              </a-collapse>
            </a-collapse-item>
          </a-collapse>
        </a-form>
      </a-tab-pane>

      <a-tab-pane key="real-devices">
        <template #title>映射真实设备</template>
        <div class="table-header">
          <span class="table-header__hint">按通道树形选择 OPC UA 地址空间中暴露的南向采集设备</span>
        </div>
        <div class="table-container saas-table nb-device-table">
          <a-table
            ref="realTreeRef"
            row-key="rowKey"
            :columns="deviceColumns"
            :data="deviceTableTree"
            :expanded-keys="expandedKeys"
            @expand-change="onExpandChange"
            size="small"
            :pagination="false"
            class="industrial-table-inline"
          >
            <template #empty>
              <a-empty description="暂无南向设备，请先在通道管理中创建设备" />
            </template>
            <template #name="{ record }">
              <span v-if="record.children" class="nb-tree-channel-name">
                <span class="nb-tree-channel-badge">{{ record.enabledCount }}/{{ record.count }} 已启用</span>
                {{ record.channelName }}
              </span>
              <span v-else>{{ record.name || record.id }}</span>
            </template>
            <template #channel="{ record }">
              <span v-if="record.children">—</span>
              <span v-else>{{ record.channelName }}</span>
            </template>
            <template #state="{ record }">
              <a-tag v-if="record.children" color="arcoblue" size="small">通道</a-tag>
              <a-tag v-else-if="record.state === 0" color="green" size="small">在线</a-tag>
              <a-tag v-else-if="record.state === 1" color="orangered" size="small">不稳定</a-tag>
              <a-tag v-else color="red" size="small">离线</a-tag>
            </template>
            <template #enable="{ record }">
              <a-checkbox
                v-if="record.children"
                :model-value="record.enabledCount > 0 && record.enabledCount >= record.count"
                :indeterminate="record.enabledCount > 0 && record.enabledCount < record.count"
                @change="(checked) => setChannelAll(record, checked)"
              />
              <a-switch v-else v-model="record._enable" size="small" @change="onDeviceToggle" />
            </template>
          </a-table>
        </div>
      </a-tab-pane>

      <a-tab-pane key="virtual-devices">
        <template #title>映射虚拟设备</template>
        <div class="table-header">
          <span class="table-header__hint">选择在 OPC UA 地址空间中暴露的虚拟影子设备</span>
          <a-button type="outline" size="small" @click="autoFillVirtualDevices">
            <template #icon><icon-check /></template>全部启用
          </a-button>
        </div>
        <div class="table-container saas-table nb-device-table">
          <a-table
            row-key="id"
            :columns="virtualDeviceColumns"
            :data="virtualDeviceTableData"
            size="small"
            :pagination="false"
            class="industrial-table-inline"
          >
            <template #empty>
              <a-empty description="暂无虚拟影子设备，请先在虚拟影子页面创建" />
            </template>
            <template #name="{ record }">
              <span>{{ record.name || record.id }}</span>
            </template>
            <template #configEnable="{ record }">
              <a-tag :color="record.enable ? 'green' : 'gray'" size="small">
                {{ record.enable ? '启用' : '禁用' }}
              </a-tag>
            </template>
            <template #enable="{ record }">
              <a-switch
                v-model="record._enable"
                size="small"
                :disabled="!record.enable"
              />
            </template>
          </a-table>
        </div>
      </a-tab-pane>
    </a-tabs>

    <template #footer>
      <div class="industrial-modal-footer">
        <a-button v-if="form.id" type="outline" :loading="syncing" @click="syncPointMapping" class="btn-secondary">
          <template #icon><icon-sync /></template>同步点位映射
        </a-button>
        <div style="flex: 1" />
        <a-button @click="visible = false" class="btn-secondary">取消</a-button>
        <a-button type="primary" :loading="loading" @click="saveSettings" class="btn-primary">
          <template #icon><icon-plus /></template>保存通道配置
        </a-button>
      </div>
    </template>
  </a-modal>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { IconPlus, IconDelete, IconCheck, IconSync } from '@arco-design/web-vue/es/icon'
import { Message } from '@arco-design/web-vue'
import { showMessage } from '@/composables/useGlobalState'
import request from '@/utils/request'
import {
  closeNorthboundSettingsDialog,
  extractNorthboundSaveWarning,
  northboundSaveRequestConfig,
  notifyNorthboundSaveError,
  notifyNorthboundSaveSuccess,
  notifyNorthboundValidationError,
  resolveNorthboundSaveError,
  validateNorthboundChannelName
} from '@/utils/northboundSave'
import { buildNorthboundDeviceTree, buildNorthboundVirtualDeviceRows, syncNorthboundDevicesFromTree, syncNorthboundVirtualDevicesFromRows } from '@/utils/southboundDevices'
import { listVirtualShadows } from '@/api/virtualShadow'

const props = defineProps({
  visible: { type: Boolean, default: false },
  config: { type: Object, default: null },
  allDevices: { type: Array, default: () => [] },
  northboundConfig: { type: Object, default: () => ({}) }
})

const emit = defineEmits(['update:visible', 'saved'])

const visible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val)
})
const loading = ref(false)
const syncing = ref(false)
const form = ref({})
const userList = ref([])
const deviceTableTree = ref([])
const virtualDeviceTableData = ref([])
const expandedKeys = ref([])
const realTreeRef = ref(null)
const activeTab = ref('basic')
const isNewMode = ref(false)
const serverCertInput = ref(null)
const serverKeyInput = ref(null)
const trustedCertInput = ref(null)
const pendingServerCert = ref('')
const pendingServerKey = ref('')
const trustedCertList = ref([])
const trustedCertsModified = ref(false)

const deviceColumns = [
  { title: '设备名称', slotName: 'name', width: 260, ellipsis: true, tooltip: true },
  { title: '采集通道', slotName: 'channel', width: 120 },
  { title: '状态', slotName: 'state', width: 84, align: 'center' },
  { title: '暴露', slotName: 'enable', width: 90, align: 'center' }
]

const virtualDeviceColumns = [
  { title: '设备名称', slotName: 'name', width: 180, ellipsis: true, tooltip: true },
  { title: '归属通道', dataIndex: 'channel_id', width: 120, ellipsis: true, tooltip: true },
  { title: '点位', dataIndex: 'pointCount', width: 70, align: 'center' },
  { title: '配置', slotName: 'configEnable', width: 80, align: 'center' },
  { title: '暴露', slotName: 'enable', width: 70, align: 'center' }
]

watch(() => props.visible, async (val) => {
  if (val) {
    activeTab.value = 'basic'
    isNewMode.value = !props.config
    if (props.config) {
      form.value = JSON.parse(JSON.stringify(props.config))
    } else {
      form.value = {
        enable: true,
        name: 'New OPC UA Server',
        port: 4840,
        endpoint: '/ipp/opcua/server',
        security_policy: 'Auto',
        security_mode: 'SignAndEncrypt',
        trusted_cert_path: '',
        devices: {},
        virtual_devices: {},
        auth_methods: ['Anonymous'],
        users: {},
        cert_file: '',
        key_file: ''
      }
    }
    if (!form.value.devices) form.value.devices = {}
    if (!form.value.virtual_devices) form.value.virtual_devices = {}
    if (!form.value.auth_methods) form.value.auth_methods = ['Anonymous']
    if (!form.value.users) form.value.users = {}

    userList.value = []
    if (form.value.users) {
      for (const [u, p] of Object.entries(form.value.users)) {
        userList.value.push({ username: u, password: p })
      }
    }

    buildDeviceTable()
    await buildVirtualDeviceTable()
    resetCertState()
  }
})

const resetCertState = () => {
  pendingServerCert.value = ''
  pendingServerKey.value = ''
  trustedCertsModified.value = false
  trustedCertList.value = []
  const trusted = form.value.trusted_certs_pem || []
  trusted.forEach((pem, i) => {
    trustedCertList.value.push({ label: `已存证书 #${i + 1}`, pem })
  })
}

const readFileAsText = (file) => new Promise((resolve, reject) => {
  const reader = new FileReader()
  reader.onload = () => resolve(String(reader.result || ''))
  reader.onerror = () => reject(reader.error)
  reader.readAsText(file)
})

const onServerCertFile = async (e) => {
  const file = e.target.files?.[0]
  if (!file) return
  try {
    pendingServerCert.value = await readFileAsText(file)
    showMessage(`已选择服务端证书: ${file.name}`, 'success')
  } catch (err) {
    showMessage('读取证书失败: ' + err.message, 'error')
  } finally {
    e.target.value = ''
  }
}

const onServerKeyFile = async (e) => {
  const file = e.target.files?.[0]
  if (!file) return
  try {
    pendingServerKey.value = await readFileAsText(file)
    showMessage(`已选择服务端私钥: ${file.name}`, 'success')
  } catch (err) {
    showMessage('读取私钥失败: ' + err.message, 'error')
  } finally {
    e.target.value = ''
  }
}

const onTrustedCertFiles = async (e) => {
  const files = Array.from(e.target.files || [])
  for (const file of files) {
    try {
      const pem = await readFileAsText(file)
      trustedCertList.value.push({ label: file.name, pem })
      trustedCertsModified.value = true
    } catch (err) {
      showMessage(`读取 ${file.name} 失败: ` + err.message, 'error')
    }
  }
  e.target.value = ''
}

const clearTrustedCerts = () => {
  trustedCertList.value = []
  trustedCertsModified.value = true
}

const onExpandChange = (keys) => {
  expandedKeys.value = keys || []
}

const buildDeviceTable = () => {
  deviceTableTree.value = buildNorthboundDeviceTree(props.allDevices, form.value.devices)
  expandedKeys.value = deviceTableTree.value.map((r) => r.rowKey)
}

const buildVirtualDeviceTable = async () => {
  try {
    const res = await listVirtualShadows()
    const items = Array.isArray(res) ? res : (res?.data || [])
    virtualDeviceTableData.value = buildNorthboundVirtualDeviceRows(items, form.value.virtual_devices)
  } catch (e) {
    console.error('[OpcuaSettings] load virtual shadows failed', e)
    virtualDeviceTableData.value = buildNorthboundVirtualDeviceRows([], form.value.virtual_devices)
  }
}

const syncDevicesFromTable = () => {
  form.value.devices = syncNorthboundDevicesFromTree(deviceTableTree.value)
}

const syncVirtualDevicesFromTable = () => {
  form.value.virtual_devices = syncNorthboundVirtualDevicesFromRows(virtualDeviceTableData.value)
}

const addUser = () => {
  userList.value.push({ username: '', password: '' })
}

// 单个设备开关切换后联动父行启用计数
const onDeviceToggle = () => {
  for (const g of deviceTableTree.value) {
    g.enabledCount = g.children.filter((c) => c._enable).length
  }
}

// 通道父勾选：批量启用/禁用该通道下全部设备
const setChannelAll = (channel, checked) => {
  const enable = !!checked
  for (const child of channel.children) {
    child._enable = enable
  }
  channel.enabledCount = enable ? channel.count : 0
}

const autoFillVirtualDevices = () => {
  virtualDeviceTableData.value.forEach(record => {
    if (record.enable) {
      record._enable = true
    }
  })
  showMessage('已启用全部虚拟设备', 'success')
}

const syncPointMapping = async () => {
  if (!form.value.id) {
    showMessage('请先保存通道配置', 'warning')
    return
  }
  if (syncing.value) return

  syncing.value = true
  try {
    await request.post(
      `/api/northbound/opcua/${form.value.id}/sync`,
      null,
      northboundSaveRequestConfig
    )
    Message.success('点位映射已同步，读写权限已更新')
  } catch (e) {
    Message.error('同步失败：' + resolveNorthboundSaveError(e))
  } finally {
    syncing.value = false
  }
}

const saveSettings = async () => {
  const missing = []
  if (!form.value.name?.trim()) missing.push('通道名称')
  if (!form.value.port) missing.push('端口')
  if (!form.value.endpoint?.trim()) missing.push('Endpoint')
  if (missing.length) {
    notifyNorthboundValidationError('请填写必填项：' + missing.join('、'))
    activeTab.value = 'basic'
    return
  }

  const nameError = validateNorthboundChannelName(form.value.name, form.value.id, props.northboundConfig)
  if (nameError) {
    notifyNorthboundValidationError(nameError)
    activeTab.value = 'basic'
    return
  }

  loading.value = true
  syncDevicesFromTable()
  syncVirtualDevicesFromTable()
  form.value.users = {}
  if (userList.value) {
    userList.value.forEach(u => {
      if (u.username) form.value.users[u.username] = u.password
    })
  }
  try {
    const dataToSave = { ...form.value }
    delete dataToSave.server_key_pem
    delete dataToSave.has_server_cert
    delete dataToSave.has_server_key
    if (pendingServerCert.value) {
      dataToSave.server_cert_pem = pendingServerCert.value
    } else {
      delete dataToSave.server_cert_pem
    }
    if (pendingServerKey.value) {
      dataToSave.server_key_pem = pendingServerKey.value
    }
    if (trustedCertsModified.value) {
      dataToSave.trusted_certs_pem = trustedCertList.value.map(item => item.pem)
    } else {
      delete dataToSave.trusted_certs_pem
    }

    const res = await request.post('/api/northbound/opcua', dataToSave, northboundSaveRequestConfig)
    const warning = extractNorthboundSaveWarning(res)
    if (res?.id) {
      form.value.id = res.id
    }
    const { warning: _ignored, ...savedCfg } = res || {}
    Object.assign(form.value, savedCfg)
    notifyNorthboundSaveSuccess('OPC UA 服务端', isNewMode.value, warning)
    closeNorthboundSettingsDialog(emit)
    emit('saved')
  } catch (e) {
    notifyNorthboundSaveError(e, 'OPC UA 服务端')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
/* v3.0 — styles in src/styles/ */
</style>

