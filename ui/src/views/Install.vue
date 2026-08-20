<template>
  <div class="install-container">
    <div class="install-wrapper">
      <div class="install-header">
        <div class="logo-section">
          <div class="logo-icon">
            <svg viewBox="0 0 24 24" width="32" height="32" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
            </svg>
          </div>
          <div class="logo-text">
            <div class="brand-name">edgeCore</div>
            <div class="brand-version">系统初始化配置</div>
          </div>
        </div>
      </div>

      <div class="install-body">
        <div class="step-indicator">
          <div class="step" :class="{ active: installState === 'config', done: installState !== 'config' }">
            <span class="step-number">1</span>
            <span class="step-label">配置信息</span>
          </div>
          <div class="step-divider" :class="{ done: installState !== 'config' }"></div>
          <div class="step" :class="{ active: installState === 'installing', done: installState === 'completed' }">
            <span class="step-number">2</span>
            <span class="step-label">初始化</span>
          </div>
          <div class="step-divider" :class="{ done: installState === 'completed' }"></div>
          <div class="step" :class="{ active: installState === 'completed' }">
            <span class="step-number">3</span>
            <span class="step-label">完成</span>
          </div>
        </div>

        <div v-if="installState === 'config'" class="install-panel">
          <div class="config-form flow-form">
            <div class="form-group">
              <label class="form-label">
                <IconSettings class="label-icon" />
                Web服务端口
              </label>
              <div class="input-group">
                <a-input 
                  v-model="formData.port" 
                  type="number" 
                  :placeholder="'请输入端口号 (80-65535)'" 
                  size="large"
                  :status="portStatus"
                  @blur="validatePort"
                >
                  <template #suffix>
                    <span v-if="portChecking" class="checking">
                      <IconLoading class="loading" />
                    </span>
                    <span v-else-if="portAvailable !== null" :class="['status', portAvailable ? 'success' : 'error']">
                      <IconCheckCircle v-if="portAvailable" />
                      <IconCloseCircle v-else />
                    </span>
                  </template>
                </a-input>
              </div>
              <div v-if="portMessage" :class="['form-hint', portAvailable === false ? 'error' : 'success']">
                {{ portMessage }}
              </div>
            </div>

            <div class="form-group">
              <label class="form-label">
                <IconUser class="label-icon" />
                管理员用户名
              </label>
              <div class="input-group">
                <a-input 
                  v-model="formData.username" 
                  :placeholder="'请输入用户名'" 
                  size="large"
                  :status="usernameStatus"
                >
                  <template #prefix>
                    <IconUser />
                  </template>
                </a-input>
              </div>
            </div>

            <div class="form-group">
              <label class="form-label">
                <IconLock class="label-icon" />
                管理员密码
              </label>
              <div class="input-group">
                <a-input-password 
                  v-model="formData.password" 
                  :placeholder="'请输入密码'" 
                  size="large"
                  :status="passwordStatus"
                  @blur="validatePassword"
                >
                  <template #prefix>
                    <IconLock />
                  </template>
                </a-input-password>
              </div>
              <div v-if="passwordMessage" class="form-hint error">
                {{ passwordMessage }}
              </div>
              <div class="password-rules">
                <div class="rule" :class="{ pass: formData.password.length >= 8 }">
                  <IconCheckCircle v-if="formData.password.length >= 8" />
                  <IconCloseCircle v-else />
                  <span>至少8位</span>
                </div>
                <div class="rule" :class="{ pass: /[A-Z]/.test(formData.password) }">
                  <IconCheckCircle v-if="/[A-Z]/.test(formData.password)" />
                  <IconCloseCircle v-else />
                  <span>大写字母</span>
                </div>
                <div class="rule" :class="{ pass: /[a-z]/.test(formData.password) }">
                  <IconCheckCircle v-if="/[a-z]/.test(formData.password)" />
                  <IconCloseCircle v-else />
                  <span>小写字母</span>
                </div>
                <div class="rule" :class="{ pass: /[0-9]/.test(formData.password) }">
                  <IconCheckCircle v-if="/[0-9]/.test(formData.password)" />
                  <IconCloseCircle v-else />
                  <span>数字</span>
                </div>
                <div class="rule" :class="{ pass: hasSpecialChar }">
                  <IconCheckCircle v-if="hasSpecialChar" />
                  <IconCloseCircle v-else />
                  <span>特殊符号</span>
                </div>
              </div>
            </div>

            <div class="form-group">
              <label class="form-label">
                <IconLock class="label-icon" />
                确认密码
              </label>
              <div class="input-group">
                <a-input-password 
                  v-model="formData.confirmPassword" 
                  :placeholder="'请再次输入密码'" 
                  size="large"
                  :status="confirmPasswordStatus"
                  @blur="validateConfirmPassword"
                >
                  <template #prefix>
                    <IconLock />
                  </template>
                </a-input-password>
              </div>
              <div v-if="confirmPasswordMessage" class="form-hint error">
                {{ confirmPasswordMessage }}
              </div>
            </div>

            <div class="form-group">
              <label class="form-label">
                <IconNav class="label-icon" />
                网关名称
              </label>
              <div class="input-group">
                <a-input 
                  v-model="formData.gatewayName" 
                  :placeholder="'请输入网关名称'" 
                  size="large"
                  :status="gatewayNameStatus"
                >
                  <template #prefix>
                    <IconNav />
                  </template>
                </a-input>
              </div>
            </div>

            <div class="form-group">
              <label class="form-label">
                <IconLocation class="label-icon" />
                网关位置
              </label>
              <div class="input-group">
                <a-input 
                  v-model="formData.gatewayLocation" 
                  :placeholder="'例如：站点-1栋-4楼-201机房'" 
                  size="large"
                  :status="gatewayLocationStatus"
                >
                  <template #prefix>
                    <IconLocation />
                  </template>
                </a-input>
              </div>
              <div class="form-hint">填写网关所在物理位置，便于运维管理</div>
            </div>

            <div class="form-group info-group">
              <label class="form-label">
                <IconFolder class="label-icon" />
                数据存储目录
              </label>
              <div class="info-value">
                <IconFolder />
                <span>data (配置文件: data/config.db)</span>
              </div>
              <div class="form-hint">系统将自动在此目录下创建数据库文件，无需手动选择</div>
            </div>

            <div v-if="generalErrors.length > 0" class="error-list">
              <div v-for="(error, index) in generalErrors" :key="index" class="error-item">
                <IconCloseCircleFill />
                <span>{{ error }}</span>
              </div>
            </div>

            <div class="form-actions">
              <a-button 
                type="primary" 
                size="large" 
                long 
                :loading="startingInstall"
                @click="handleStartInstall"
                class="btn-install"
              >
                <template #icon>
                  <IconPlayCircle />
                </template>
                开始初始化安装
              </a-button>
            </div>
          </div>
        </div>

        <div v-else-if="installState === 'installing'" class="install-panel install-panel--center">
          <div class="progress-icon install-progress-icon">
            <IconLoading class="spinner" />
          </div>
          <div class="install-progress-title">正在初始化系统</div>
          <div class="install-progress-subtitle">{{ statusText }}</div>

          <div class="install-progress-bar-wrap">
            <div class="install-progress-bar">
              <div class="install-progress-fill" :style="{ width: progress + '%' }"></div>
            </div>
            <div class="install-progress-info">{{ currentStep }} / {{ totalSteps }} · {{ progress }}%</div>
          </div>

          <div class="install-progress-log">
            <div v-for="(log, index) in logMessages" :key="index" class="install-log-item">
              {{ log }}
            </div>
          </div>
        </div>

        <div v-else-if="installState === 'completed'" class="install-panel install-panel--center">
          <div class="complete-icon install-complete-icon">
            <IconCheckCircleFill />
          </div>
          <div class="install-complete-title">初始化完成</div>
          <div class="install-complete-subtitle">
            系统配置已完成，即将跳转到登录页面
            <span v-if="configPort">
              <br />服务端口: {{ configPort }}
            </span>
          </div>
          <div class="install-complete-countdown">
            自动跳转: <span class="countdown">{{ redirectCountdown }}s</span>
          </div>
        </div>
        <div v-else-if="installState === 'failed'" class="install-panel install-panel--center">
          <div class="complete-icon install-complete-icon install-fail-icon">
            <IconCloseCircle />
          </div>
          <div class="install-complete-title">初始化失败</div>
          <div class="install-progress-subtitle" style="color: var(--edgeCore-error);">
            {{ installErrorMsg }}
          </div>
          <div v-if="logMessages.length" class="install-progress-log">
            <div v-for="(log, index) in logMessages" :key="index" class="install-log-item">
              {{ log }}
            </div>
          </div>
          <div class="install-complete-countdown">
            <a-button type="primary" @click="installState = 'config'">返回重新配置</a-button>
          </div>
        </div>
      </div>

      <div class="install-footer">
        © {{ new Date().getFullYear() }} edgeCore
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import {
  IconSettings, IconUser, IconLock, IconFolder,
  IconNav, IconLocation, IconCheckCircle, IconCloseCircle,
  IconCheckCircleFill, IconCloseCircleFill, IconPlayCircle,
  IconLoading
} from '@arco-design/web-vue/es/icon'
import InstallApi from '../api/install'
import { configStore } from '../stores/app.js'

const appConfig = configStore()

const installState = ref('config')
const installErrorMsg = ref('')
const formData = reactive({
  port: 8080,
  username: 'admin',
  password: '',
  confirmPassword: '',
  gatewayName: '',
  gatewayLocation: ''
})

const portAvailable = ref(null)
const portChecking = ref(false)
const portMessage = ref('')
const portStatus = computed(() => {
  if (portAvailable.value === true) return 'success'
  if (portAvailable.value === false) return 'error'
  return ''
})

const passwordStatus = ref('')
const passwordMessage = ref('')
const confirmPasswordStatus = ref('')
const confirmPasswordMessage = ref('')
const usernameStatus = ref('')
const gatewayNameStatus = ref('')
const gatewayLocationStatus = ref('')

const hasSpecialChar = computed(() => {
  const specialChars = /[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>/?~`]/
  return specialChars.test(formData.password)
})

const generalErrors = ref([])
const startingInstall = ref(false)

const currentStep = ref(0)
const totalSteps = ref(4)
const progress = ref(0)
const statusText = ref('')
const logMessages = ref([])

const redirectCountdown = ref(5)
let redirectTimer = null
let statusPollTimer = null
let configPort = ref(0)

const validatePort = async () => {
  if (!formData.port) {
    portAvailable.value = false
    portMessage.value = '端口号不能为空'
    return
  }
  if (formData.port < 80 || formData.port > 65535) {
    portAvailable.value = false
    portMessage.value = '端口号必须在80-65535范围内'
    return
  }

  portChecking.value = true
  try {
    const res = await InstallApi.checkPort(formData.port)
    if (res.code === '0') {
      portAvailable.value = res.data.available
      portMessage.value = res.data.error || (res.data.available ? '端口可用' : '端口已被占用')
    }
  } catch (error) {
    portAvailable.value = false
    portMessage.value = '端口检查失败: ' + error.message
  }
  portChecking.value = false
}

const validatePassword = () => {
  if (!formData.password) {
    passwordStatus.value = 'error'
    passwordMessage.value = '密码不能为空'
    return
  }
  if (formData.password.length < 8) {
    passwordStatus.value = 'error'
    passwordMessage.value = '密码长度至少8位'
    return
  }

  const hasUpperCase = /[A-Z]/.test(formData.password)
  const hasLowerCase = /[a-z]/.test(formData.password)
  const hasNumber = /[0-9]/.test(formData.password)
  const hasSpecialChar = /[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>/?~`]/.test(formData.password)

  const typesCount = [hasUpperCase, hasLowerCase, hasNumber, hasSpecialChar].filter(Boolean).length

  if (typesCount < 3) {
    passwordStatus.value = 'error'
    passwordMessage.value = '密码需包含大写字母、小写字母、数字、特殊符号中的至少三种组合'
    return
  }

  passwordStatus.value = 'success'
  passwordMessage.value = ''
}

const validateConfirmPassword = () => {
  if (!formData.confirmPassword) {
    confirmPasswordStatus.value = 'error'
    confirmPasswordMessage.value = '请确认密码'
    return
  }
  if (formData.confirmPassword !== formData.password) {
    confirmPasswordStatus.value = 'error'
    confirmPasswordMessage.value = '两次输入的密码不一致'
    return
  }
  confirmPasswordStatus.value = 'success'
  confirmPasswordMessage.value = ''
}

const validateForm = async () => {
  generalErrors.value = []

  await validatePort()
  validatePassword()
  validateConfirmPassword()

  if (!formData.username) {
    usernameStatus.value = 'error'
    generalErrors.value.push('用户名不能为空')
  } else {
    usernameStatus.value = 'success'
  }

  if (!formData.gatewayName) {
    gatewayNameStatus.value = 'error'
    generalErrors.value.push('网关名称不能为空')
  } else {
    gatewayNameStatus.value = 'success'
  }

  return generalErrors.value.length === 0 && 
         portAvailable.value === true && 
         passwordStatus.value === 'success' && 
         confirmPasswordStatus.value === 'success'
}

const handleStartInstall = async () => {
  if (!(await validateForm())) return

  startingInstall.value = true
  try {
    const config = {
      port: parseInt(formData.port) || 8080,
      username: formData.username,
      password: formData.password,
      storagePath: 'data',
      gatewayName: formData.gatewayName,
      gatewayLocation: formData.gatewayLocation,
      deviceSerial: ''
    }

    const res = await InstallApi.startInstall(config)
    if (res.code === '0') {
      installState.value = 'installing'
      startStatusPolling()
    } else {
      generalErrors.value.push(res.message || '启动初始化失败')
    }
  } catch (error) {
    if (error.response?.status === 409) {
      generalErrors.value.push('系统正在初始化中，请稍后再试')
    } else {
      generalErrors.value.push('启动初始化失败: ' + error.message)
    }
  } finally {
    startingInstall.value = false
  }
}

const startStatusPolling = () => {
  statusPollTimer = setInterval(async () => {
    try {
      const res = await InstallApi.getInstallStatus()
      if (res.code === '0') {
        const data = res.data
        currentStep.value = data.currentStep
        totalSteps.value = data.totalSteps
        progress.value = data.progress
        statusText.value = data.status
        logMessages.value = data.logMessages
        if (data.configPort) {
          configPort.value = data.configPort
        }

        nextTick(() => {
          const logContainer = document.querySelector('.install-progress-log')
          if (logContainer) {
            logContainer.scrollTop = logContainer.scrollHeight
          }
        })

        if (data.status === 'completed') {
          appConfig.markInstalled()
          installState.value = 'completed'
          stopStatusPolling()
          startRedirectCountdown()
        } else if (data.status === 'failed') {
          installState.value = 'failed'
          installErrorMsg.value = data.message || data.error || '初始化过程中发生未知错误'
          stopStatusPolling()
        }
      }
    } catch (error) {
      console.error('获取安装状态失败:', error)
    }
  }, 1000)
}

const stopStatusPolling = () => {
  if (statusPollTimer) {
    clearInterval(statusPollTimer)
    statusPollTimer = null
  }
}

const startRedirectCountdown = () => {
  redirectCountdown.value = 5
  redirectTimer = setInterval(() => {
    redirectCountdown.value--
    if (redirectCountdown.value <= 0) {
      clearInterval(redirectTimer)
      const currentPort = window.location.port
      const targetPort = configPort.value || formData.port
      if (targetPort && String(targetPort) !== currentPort) {
        const newUrl = `${window.location.protocol}//${window.location.hostname}:${targetPort}/#/login`
        window.location.href = newUrl
      } else {
        location.href = '/#/login'
      }
    }
  }, 1000)
}

onMounted(async () => {
  try {
    const res = await InstallApi.checkInstallStatus()
    if (res.code === '0') {
      if (res.data.isInstalled) {
        location.href = '/#/login'
        return
      }
    }
  } catch (error) {
    console.error('检查安装状态失败:', error)
  }
  await validatePort()
})

onUnmounted(() => {
  stopStatusPolling()
  if (redirectTimer) {
    clearInterval(redirectTimer)
  }
})
</script>

<style scoped>
/* v3.0 — styles in src/styles/ */
</style>