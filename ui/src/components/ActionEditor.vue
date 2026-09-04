<template>
  <div class="action-editor">
    <a-row :gutter="12">
      <!-- Type Selection -->
      <a-col :span="24" :md="6">
        <a-form-item label="动作类型">
          <a-select
            v-model="action.type"
            :options="actionTypes"
            placeholder="请选择动作类型"
            class="rect-input"
            @change="onTypeChange"
          />
        </a-form-item>
      </a-col>
      
      <!-- Interval (Rate Limit) -->
      <a-col :span="24" :md="6">
        <a-form-item label="频率限制 (Interval)">
          <a-input
            v-model="action.config.interval"
            placeholder="e.g. 1s"
            class="rect-input"
          />
        </a-form-item>
      </a-col>
      
      <!-- Remove Button -->
      <a-col :span="24" :md="12" style="text-align: right;">
        <a-button 
          type="text" 
          status="danger" 
          @click="$emit('remove')"
          size="small"
        >
          <template #icon><IconDelete /></template>
          删除动作
        </a-button>
      </a-col>

      <!-- Config Area -->
      <a-col :span="24">
        <!-- 1. Sequence -->
        <div v-if="action.type === 'sequence'" class="pl-2">
          <div class="flex items-center mb-2">
            <span class="text-sm font-semibold mr-2">执行步骤 (Steps)</span>
            <a-tag size="small" color="arcoblue">{{ (action.config.steps || []).length }}</a-tag>
          </div>
          <div class="pl-3 border-s-md" style="border-color: #eee;">
            <div v-for="(step, idx) in (action.config.steps || [])" :key="idx" class="mb-2">
              <ActionEditor 
                v-model="action.config.steps[idx]" 
                :channels="channels" 
                @remove="removeStep(idx)"
              />
            </div>
            <a-button size="small" type="primary" @click="addStep">
              <template #icon><IconPlus /></template>
              添加步骤
            </a-button>
          </div>
        </div>

        <!-- 2. Check -->
        <div v-if="action.type === 'check'" class="pl-2">
          <a-row :gutter="12">
            <!-- Device Selection -->
            <a-col :span="24" :md="6">
              <a-form-item label="通道">
                <a-select 
                  v-model="action.config.channel_id" 
                  :options="channels" 
                  placeholder="选择通道"
                  class="rect-input"
                  @change="onChannelChange(action.config)"
                />
              </a-form-item>
            </a-col>
            <a-col :span="24" :md="6">
              <a-form-item label="设备">
                <a-select 
                  v-model="action.config.device_id" 
                  :options="deviceList" 
                  placeholder="选择设备"
                  class="rect-input"
                  :disabled="!action.config.channel_id"
                  @change="onDeviceChange(action.config)"
                />
              </a-form-item>
            </a-col>
            <a-col :span="24" :md="6">
              <a-form-item label="点位">
                <a-select 
                  v-model="action.config.point_id" 
                  :options="pointList" 
                  placeholder="选择点位"
                  class="rect-input"
                  :disabled="!action.config.device_id"
                />
              </a-form-item>
            </a-col>

            <a-col :span="24" :md="12">
              <a-form-item label="校验表达式">
                <a-input 
                  v-model="action.config.expression" 
                  placeholder="v == 1" 
                  class="rect-input"
                />
                <template #extra>v 代表当前点位值</template>
              </a-form-item>
            </a-col>
            <a-col :span="24" :md="4">
              <a-form-item label="超时">
                <a-input 
                  v-model="action.config.timeout" 
                  placeholder="5s" 
                  class="rect-input"
                />
              </a-form-item>
            </a-col>
            <a-col :span="24" :md="4">
              <a-form-item label="重试次数">
                <a-input-number 
                  v-model="action.config.retry" 
                  class="rect-input"
                />
              </a-form-item>
            </a-col>
            <a-col :span="24" :md="4">
              <a-form-item label="重试间隔">
                <a-input 
                  v-model="action.config.interval" 
                  placeholder="1s" 
                  class="rect-input"
                />
              </a-form-item>
            </a-col>
          </a-row>
           
          <!-- On Fail -->
          <div class="mt-2">
            <div class="text-sm font-semibold text-red-600 mb-2">失败回退 (On Fail):</div>
            <div class="pl-3 border-s-md border-error" style="border-color: #ff5252;">
              <div v-for="(step, idx) in (action.config.on_fail || [])" :key="idx" class="mb-2">
                <ActionEditor 
                  v-model="action.config.on_fail[idx]" 
                  :channels="channels" 
                  @remove="removeFailStep(idx)"
                />
              </div>
              <a-button size="small" type="outline" status="danger" @click="addFailStep">
                <template #icon><IconPlus /></template>
                添加回退动作
              </a-button>
            </div>
          </div>
        </div>

        <!-- 3. Delay -->
        <div v-if="action.type === 'delay'" class="pl-2">
          <a-form-item label="延时时长 (Duration)">
            <a-input 
              v-model="action.config.duration" 
              placeholder="e.g. 30s, 1m" 
              class="rect-input"
            />
          </a-form-item>
        </div>

        <!-- 4. Log -->
        <div v-if="action.type === 'log'" class="pl-2">
          <a-row :gutter="12">
            <a-col :span="24" :md="6">
              <a-form-item label="日志级别">
                <a-select 
                  v-model="action.config.level" 
                  :options="[
                    {label: 'Info', value: 'info'},
                    {label: 'Warn', value: 'warn'},
                    {label: 'Error', value: 'error'}
                  ]" 
                  placeholder="选择级别"
                  class="rect-input"
                />
              </a-form-item>
            </a-col>
            <a-col :span="24" :md="18">
              <a-form-item label="日志内容">
                <a-input 
                  v-model="action.config.message" 
                  placeholder="支持模板变量 ${v}" 
                  class="rect-input"
                />
              </a-form-item>
            </a-col>
          </a-row>
        </div>

        <!-- 5. Device Control -->
        <div v-if="action.type === 'device_control'" class="pl-2">
          <div class="flex items-center justify-end mb-2">
            <a-switch
              v-model="isBatchMode"
              type="round"
              class="mr-4"
              @change="toggleBatchMode"
            >
              <template #checked>批量控制 (Batch)</template>
              <template #unchecked>单点控制</template>
            </a-switch>
          </div>

          <!-- Single Mode -->
          <div v-if="!isBatchMode">
            <a-row :gutter="12">
              <a-col :span="24" :md="6">
                <a-form-item label="通道">
                  <a-select 
                    v-model="action.config.channel_id" 
                    :options="channels" 
                    placeholder="选择通道"
                    class="rect-input"
                    @change="onChannelChange(action.config)"
                  />
                </a-form-item>
              </a-col>
              <a-col :span="24" :md="6">
                <a-form-item label="设备">
                  <a-select 
                    v-model="action.config.device_id" 
                    :options="deviceList" 
                    placeholder="选择设备"
                    class="rect-input"
                    :disabled="!action.config.channel_id"
                    @change="onDeviceChange(action.config)"
                  />
                </a-form-item>
              </a-col>
              <a-col :span="24" :md="6">
                <a-form-item label="点位(仅可写类型)">
                  <a-select 
                    v-model="action.config.point_id" 
                    :options="pointList" 
                    placeholder="选择点位"
                    class="rect-input"
                    :disabled="!action.config.device_id"
                  />
                </a-form-item>
              </a-col>
              <a-col :span="24">
                <a-form-item label="写入值 (Value Template)">
                  <a-input 
                    v-model="action.config.value" 
                    placeholder="可以是固定值(1) 或 模板(${v})" 
                    class="rect-input"
                  />
                </a-form-item>
              </a-col>
            </a-row>
          </div>

          <!-- Batch Mode -->
          <div v-else>
            <div v-for="(target, tIdx) in (action.config.targets || [])" :key="tIdx" class="mb-2 px-2 py-2 border border-gray-200 rounded">
              <div class="flex justify-between mb-1">
                <span class="text-xs text-gray-500">目标 {{ tIdx + 1 }}</span>
                <a-button type="text" size="mini" @click="removeTarget(tIdx)">
                  <template #icon><IconClose /></template>
                </a-button>
              </div>
              <TargetEditor :target="target" :channels="channels" />
            </div>
            <a-button size="small" type="primary" @click="addTarget">
              <template #icon><IconPlus /></template>
              添加控制目标
            </a-button>
          </div>
        </div>

        <!-- 6. MQTT -->
        <div v-if="action.type === 'mqtt'" class="pl-2">
          <a-row :gutter="12">
            <a-col :span="24" :md="8">
              <a-form-item label="北向通道">
                <a-select
                  v-model="action.config.mqtt_id"
                  :options="mqttOptions"
                  placeholder="选择 MQTT 通道"
                  class="rect-input"
                  allow-clear
                />
              </a-form-item>
            </a-col>
            <a-col :span="24" :md="8">
              <a-form-item label="Topic">
                <a-input 
                  v-model="action.config.topic" 
                  placeholder="可选 (默认使用配置Topic)" 
                  class="rect-input"
                />
              </a-form-item>
            </a-col>
            <a-col :span="24" :md="8">
              <a-form-item label="发送策略">
                <a-select 
                  v-model="action.config.send_strategy" 
                  :options="[
                    {label: 'Single', value: 'single'},
                    {label: 'Batch', value: 'batch'}
                  ]" 
                  placeholder="选择策略"
                  class="rect-input"
                />
              </a-form-item>
            </a-col>
            <a-col :span="24">
              <a-form-item label="消息内容 (Message Template)">
                <a-textarea 
                  v-model="action.config.message" 
                  placeholder="留空则发送默认 JSON" 
                  :rows="2"
                  class="rect-input"
                />
              </a-form-item>
            </a-col>
          </a-row>
        </div>

        <!-- 7. HTTP -->
        <div v-if="action.type === 'http'" class="pl-2">
          <a-row :gutter="12">
            <a-col :span="24" :md="12">
              <a-form-item label="北向通道 (Northbound Channel)">
                <a-select
                  v-model="action.config.http_id"
                  :options="httpOptions"
                  placeholder="选择 HTTP 通道"
                  class="rect-input"
                  allow-clear
                />
              </a-form-item>
            </a-col>
            <a-col :span="24" :md="6" v-if="!action.config.http_id">
              <a-form-item label="Method">
                <a-select 
                  v-model="action.config.method" 
                  :options="[
                    {label: 'POST', value: 'POST'},
                    {label: 'PUT', value: 'PUT'},
                    {label: 'GET', value: 'GET'}
                  ]" 
                  placeholder="选择方法"
                  class="rect-input"
                />
              </a-form-item>
            </a-col>
            <a-col :span="24" :md="18" v-if="!action.config.http_config_id">
              <a-form-item label="URL">
                <a-input 
                  v-model="action.config.url" 
                  placeholder="输入URL" 
                  class="rect-input"
                />
              </a-form-item>
            </a-col>
            <a-col :span="24">
              <a-form-item label="Body Template">
                <a-textarea 
                  v-model="action.config.body" 
                  placeholder="输入Body模板" 
                  :rows="2"
                  class="rect-input"
                />
              </a-form-item>
            </a-col>
          </a-row>
        </div>

        <!-- 8. Database -->
        <div v-if="action.type === 'database'" class="pl-2">
          <a-form-item label="Bucket Name">
            <a-input 
              v-model="action.config.bucket" 
              placeholder="rule_events" 
              class="rect-input"
            />
          </a-form-item>
        </div>
      </a-col>
    </a-row>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, inject } from 'vue'
import request from '@/utils/request'
import { IconDelete, IconPlus, IconClose } from '@arco-design/web-vue/es/icon'
import {
  normalizeDeviceOptions,
  fetchWritablePointOptions
} from '@/utils/southboundPointOptions'
// Recursive component self-reference
import ActionEditor from './ActionEditor.vue'
import TargetEditor from './TargetEditor.vue'

const props = defineProps({
  modelValue: {
    type: Object,
    required: true
  },
  channels: {
    type: Array,
    default: () => []
  }
})

const emit = defineEmits(['update:modelValue', 'remove'])

const action = ref(props.modelValue)
const deviceList = ref([])
const pointList = ref([])
const loadedChannelId = ref('')
const pointLoadToken = ref(0)

// Inject Options
const mqttOptions = inject('mqttOptions', ref([]))
const httpOptions = inject('httpOptions', ref([]))

// Sync props to local state
watch(() => props.modelValue, (val) => {
  if (val === action.value) return

  action.value = val
  // Load devices/points if needed
  if (action.value?.type === 'device_control' && !isBatchMode.value) {
     loadDevices(action.value.config || {})
  } else if (action.value?.type === 'check') {
     loadDevices(action.value.config || {})
  }
}, { immediate: true })



// Sync local state to props
watch(action, (val) => {
  emit('update:modelValue', val)
}, { deep: true })

const actionTypes = [
  { label: 'Log (日志)', value: 'log' },
  { label: 'Device Control (设备控制)', value: 'device_control' },
  { label: 'Sequence (顺序执行)', value: 'sequence' },
  { label: 'Check (校验)', value: 'check' },
  { label: 'Delay (延时)', value: 'delay' },
  { label: 'MQTT Push (MQTT推送)', value: 'mqtt' },
  { label: 'HTTP Push (HTTP推送)', value: 'http' },
  { label: 'Database (存储)', value: 'database' },
]

const onTypeChange = () => {
    if (!action.value.config) action.value.config = {}
    // Set defaults based on type
    if (action.value.type === 'sequence') {
        if (!action.value.config.steps) action.value.config.steps = []
    } else if (action.value.type === 'check') {
        if (!action.value.config.retry) action.value.config.retry = 3
        if (!action.value.config.interval) action.value.config.interval = '1s'
        if (!action.value.config.timeout) action.value.config.timeout = '5s'
    } else if (action.value.type === 'log') {
        if (!action.value.config.level) action.value.config.level = 'info'
    }
}

// --- Sequence / Check Steps Management ---
const addStep = () => {
    if (!action.value.config.steps) action.value.config.steps = []
    action.value.config.steps.push({ type: 'device_control', config: {} })
}
const removeStep = (idx) => {
    action.value.config.steps.splice(idx, 1)
}
const addFailStep = () => {
    if (!action.value.config.on_fail) action.value.config.on_fail = []
    action.value.config.on_fail.push({ type: 'log', config: { level: 'error', message: 'Check failed, rolling back...' } })
}
const removeFailStep = (idx) => {
    action.value.config.on_fail.splice(idx, 1)
}

// --- Device Control Logic ---
const isBatchMode = ref(false)

const toggleBatchMode = () => {
    if (isBatchMode.value) {
        if (!action.value.config.targets) action.value.config.targets = []
        // Migrate single to batch target 1
        if (action.value.config.channel_id) {
            action.value.config.targets.push({
                channel_id: action.value.config.channel_id,
                device_id: action.value.config.device_id,
                point_id: action.value.config.point_id,
                value: action.value.config.value
            })
            action.value.config.channel_id = ''
        }
    }
}

const addTarget = () => {
    if (!action.value.config.targets) action.value.config.targets = []
    action.value.config.targets.push({ channel_id: '', device_id: '', point_id: '', value: '' })
}
const removeTarget = (idx) => {
    action.value.config.targets.splice(idx, 1)
}

// --- Device/Point Loading ---
const onChannelChange = async (cfg) => {
    cfg.device_id = ''
    cfg.point_id = ''
    deviceList.value = []
    pointList.value = []
    loadedChannelId.value = ''

    if (!cfg.channel_id) return

    const data = await request.get(`/api/channels/${encodeURIComponent(cfg.channel_id)}/devices`)
    deviceList.value = normalizeDeviceOptions(data)
    loadedChannelId.value = cfg.channel_id
}

const loadPointsForDevice = async (cfg) => {
    if (!cfg?.channel_id || !cfg?.device_id) {
        pointList.value = []
        return
    }

    const channelId = cfg.channel_id
    const deviceId = cfg.device_id
    const token = ++pointLoadToken.value
    const dev = deviceList.value.find((d) => String(d.value) === String(deviceId))
    const embeddedPoints = dev?.raw?.points

    const options = await fetchWritablePointOptions(
        request,
        channelId,
        deviceId,
        embeddedPoints
    )

    if (token !== pointLoadToken.value) return
    if (String(channelId) !== String(loadedChannelId.value)) return
    if (String(deviceId) !== String(cfg.device_id)) return

    pointList.value = options
}

const onDeviceChange = async (cfg) => {
    cfg.point_id = ''
    pointList.value = []

    if (!cfg.device_id) return

    if (deviceList.value.length === 0 && cfg.channel_id) {
        await loadDevices(cfg)
        return
    }

    await loadPointsForDevice(cfg)
}

const loadDevices = async (cfg) => {
    if (!cfg?.channel_id) {
        loadedChannelId.value = ''
        deviceList.value = []
        pointList.value = []
        return
    }

    const channelChanged = loadedChannelId.value !== cfg.channel_id

    if (channelChanged || deviceList.value.length === 0) {
        const data = await request.get(`/api/channels/${encodeURIComponent(cfg.channel_id)}/devices`)
        deviceList.value = normalizeDeviceOptions(data)
        loadedChannelId.value = cfg.channel_id
    }

    if (cfg.device_id) {
        await loadPointsForDevice(cfg)
    } else {
        pointList.value = []
    }
}

onMounted(() => {
    // Init batch mode check
    if (action.value.type === 'device_control' && action.value.config && action.value.config.targets && action.value.config.targets.length > 0) {
        isBatchMode.value = true
    }
    // Init device list loading
    if ((action.value.type === 'device_control' && !isBatchMode.value) || action.value.type === 'check') {
        loadDevices(action.value.config)
    }
})
</script>

<style scoped>
/* v3.0 — styles in src/styles/ */
</style>


