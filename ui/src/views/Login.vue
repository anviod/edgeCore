<template>
  <div class="login-container">
    <div class="login-scene">
      <div
        class="login-panel" 
        :class="{ 'shake-animation': isShaking, 'login-card-exit': isLoginSuccess, 'countdown-10': ctxData.countdown <= 10 }"
      >
        <div class="panel-topbar">
          <div class="logo-box">
            <div class="logo-icon">
              <span>edgeCore</span>
            </div>
          </div>
          <div class="panel-header-side">
            <span class="version-tag">VER {{ ctxData.configInfo.softVer || '2.0' }}</span>
          </div>
        </div>

        <div class="panel-title">
          <div class="title-main">边缘计算网关</div>
          <div class="title-sub">IOT THINGS GATEWAY</div>
        </div>

        <div class="auth-row">
          <a-radio-group v-model="ctxData.loginMethod" type="button" class="industrial-radio">
            <a-radio value="local"><icon-user /> 本地登录</a-radio>
            <a-radio value="ldap"><icon-user-group /> LDAP 登录</a-radio>
          </a-radio-group>
          <span class="mode-indicator" :class="{ 'is-ldap': ctxData.loginMethod === 'ldap' }"></span>
        </div>

        <a-form :model="ctxData.loginForm" layout="vertical" @submit="handleLogin" class="flow-form custom-form">
          <div class="field">
            <div class="label">用户标识 / Username</div>
            <a-input v-model="ctxData.loginForm.userName" :placeholder="ctxData.loginMethod === 'ldap' ? 'LDAP 账号 / 邮箱' : '请输入用户名'" size="large" allow-clear>
              <template #prefix>
                <icon-user v-if="ctxData.loginMethod === 'local'" />
                <icon-at v-else />
              </template>
            </a-input>
          </div>

          <div class="field">
            <div class="label">访问密钥 / Password</div>
            <a-input-password v-model="ctxData.loginForm.password" :placeholder="ctxData.loginMethod === 'ldap' ? 'LDAP 域密码' : '请输入密码'" size="large" allow-clear @keyup.enter="handleLogin">
              <template #prefix><icon-lock /></template>
            </a-input-password>
          </div>

          <div class="options">
            <a-checkbox v-model="ctxData.rememberMe" class="remember-check">记住访问权限</a-checkbox>
            <a-link v-if="ctxData.loginMethod === 'local'" class="forgot-link" @click="handleForgotPassword">忘记密码?</a-link>
          </div>

          <div v-if="ctxData.loginMethod === 'ldap'" class="ldap-hint compact-hint">
            <icon-info-circle /> <span>通过企业域控服务进行身份验证</span>
          </div>
          <div v-else class="terminal-decorator compact-terminal">
            <span class="status-dot"></span>
            <span class="terminal-code">AUTH_MODE: LOCAL_DATABASE</span>
          </div>

          <div v-if="ctxData.errorMessage" class="error-message">
            <icon-close-circle-fill /> <span>{{ ctxData.errorMessage }}</span>
          </div>

          <a-button type="primary" html-type="submit" size="large" long :loading="ctxData.loading" :disabled="isLoginSuccess" class="login-submit-btn">
            <template #icon>
              <icon-check-circle-fill v-if="isLoginSuccess" />
              <icon-arrow-right v-else />
            </template>
            {{ isLoginSuccess ? '登录成功' : (ctxData.loginMethod === 'ldap' ? '域验证登录' : '立即登录') }}
          </a-button>

          <div v-if="ctxData.countdown <= 30" class="session-expiry-hint">
            <icon-info-circle /> 登录会话将在 {{ Math.ceil(ctxData.countdown) }} 秒后过期，请及时操作
          </div>

          <div class="copyright-text panel-copyright">© {{ new Date().getFullYear() }} {{ ctxData.configInfo.name || '系统' }}</div>
        </a-form>
      </div>
    </div>
  </div>
</template>

<script setup>
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import {
  IconUser, IconLock, IconArrowRight,
  IconUserGroup, IconAt, IconInfoCircle,
  IconCloseCircleFill, IconCheckCircleFill
} from '@arco-design/web-vue/es/icon'
import LoginApi from 'api/login.js'
import InstallApi from 'api/install.js'
import router from '@/router'
import { userStore } from 'stores/user.js'
import { configStore } from '@/stores/app.js'
import { useI18n } from 'vue-i18n'
import sha256 from 'crypto-js/sha256'
import encHex from 'crypto-js/enc-hex'
import { showMessage } from '@/composables/useGlobalState'

const { t } = useI18n()
const config = configStore()
const users = userStore()

const loginFormRef = ref(null)
const isShaking = ref(false)
const isLoginSuccess = ref(false)

const ctxData = reactive({
  loginForm: {
    userName: '',
    password: '',
  },
  loginMethod: 'local',
  loading: false,
  rememberMe: false,
  configInfo: config.configInfo || {},
  nonce: '',
  errorMessage: '',
  countdown: 60,
  countdownTimer: null
})

const clearCountdown = () => {
  if (ctxData.countdownTimer) {
    clearInterval(ctxData.countdownTimer)
    ctxData.countdownTimer = null
  }
}

const startCountdown = () => {
  if (ctxData.countdownTimer) {
    clearInterval(ctxData.countdownTimer)
  }

  ctxData.countdown = 60
  const interval = 20
  const step = 60 / (60 * 1000 / interval)

  ctxData.countdownTimer = setInterval(() => {
    ctxData.countdown -= step

    if (ctxData.countdown <= 0) {
      clearInterval(ctxData.countdownTimer)
      ctxData.countdownTimer = null
      showMessage('登录页面已过期，请刷新页面重新登录', 'warning')
      setTimeout(() => {
        window.location.reload()
      }, 3000)
    }
  }, interval)
}

onBeforeUnmount(() => {
  if (ctxData.countdownTimer) {
    clearInterval(ctxData.countdownTimer)
    ctxData.countdownTimer = null
  }
})

onMounted(async () => {
  try {
    const installRes = await InstallApi.checkInstallStatus()
    if (installRes.code === '0' && installRes.data) {
      if (!installRes.data.isInstalled) {
        router.push('/install')
        return
      }
    }
  } catch (error) {
    console.error('检查安装状态失败:', error)
  }

  const logout = localStorage.getItem('logout')
  if (logout && logout !== '') {
    try {
      const lo = JSON.parse(logout)
      showMessage(lo.message || '您已成功退出登录', lo.type || 'info')
    } catch (error) {
      console.error('解析登出信息失败:', error)
    }
    localStorage.setItem('logout', '')
  }

  loadRememberedAccount()
  getSystemInfo()
  getNonce()
  startCountdown()
})

const loadRememberedAccount = () => {
  try {
    const saved = localStorage.getItem('rememberedAccount')
    if (saved) {
      const account = JSON.parse(saved)
      ctxData.loginForm.userName = account.userName || ''
      ctxData.rememberMe = true
    }
  } catch (e) {
    console.error('加载保存的账号失败:', e)
  }
}

const getSystemInfo = async () => {
  try {
    const res = await LoginApi.getSystemInfo()
    if (res.code === '0' && res.data) {
      // 只更新版本号等信息，不更新系统标题
      const newConfigInfo = {
        ...ctxData.configInfo,
        softVer: res.data.softVer
      }
      ctxData.configInfo = newConfigInfo
      config.setConfigInfo(newConfigInfo)
    }
  } catch (error) {
    console.error('获取系统信息失败:', error)
  }
}

const getNonce = async () => {
  try {
    const res = await LoginApi.getNonce()
    if (res.code === '0' && res.data?.nonce) {
      ctxData.nonce = res.data.nonce
    } else {
      console.warn('获取nonce失败，使用本地生成')
      ctxData.nonce = Date.now().toString(36) + Math.random().toString(36).substr(2)
    }
  } catch (error) {
    console.error('获取nonce异常:', error)
    ctxData.nonce = Date.now().toString(36) + Math.random().toString(36).substr(2)
  }
}

const handleLogin = async () => {
  if (ctxData.loading || isLoginSuccess.value) return
  if (!ctxData.loginForm.userName) {
    ctxData.errorMessage = '请输入用户名'
    triggerShake()
    return
  }
  if (!ctxData.loginForm.password) {
    ctxData.errorMessage = '请输入密码'
    triggerShake()
    return
  }
  if (ctxData.loginMethod === 'local' && ctxData.loginForm.password.length < 8) {
    ctxData.errorMessage = '密码长度至少8位'
    triggerShake()
    return
  }

  ctxData.loading = true
  ctxData.errorMessage = ''

  try {
    if (!ctxData.nonce) {
      await getNonce()
    }

    let passwordToSend = ''
    if (ctxData.loginMethod === 'ldap') {
      passwordToSend = ctxData.loginForm.password
    } else {
      passwordToSend = sha256(ctxData.loginForm.password + ctxData.nonce).toString(encHex)
    }

    const loginData = {
      loginFlag: true,
      loginType: ctxData.loginMethod,
      data: {
        username: ctxData.loginForm.userName,
        password: passwordToSend,
        nonce: ctxData.nonce,
      },
      token: '',
    }

    const res = await LoginApi.login(loginData)

    if (res.code === '0') {
      await handleLoginSuccess(res)
    } else {
      handleLoginFailure(res)
      triggerShake()
      ctxData.loading = false
    }
  } catch (error) {
    handleLoginError(error)
    triggerShake()
    ctxData.loading = false
  }
}

const triggerShake = () => {
  isShaking.value = true
  setTimeout(() => {
    isShaking.value = false
  }, 500)
}

const handleLoginSuccess = async (res) => {
  clearCountdown()
  try {
    ctxData.errorMessage = ''
    isLoginSuccess.value = true

    const processedPermissions = processPermissions(res.data.permissions)

    users.setLoginInfo(
      { userName: res.data.username },
      processedPermissions,
      res.data.token
    )

    const storeData = {
      ...res.data,
      permissions: processedPermissions,
      loginTime: Date.now()
    }
    localStorage.setItem('loginInfo', JSON.stringify(storeData))

    if (ctxData.rememberMe) {
      localStorage.setItem('rememberedAccount', JSON.stringify({
        userName: ctxData.loginForm.userName,
        timestamp: Date.now()
      }))
    } else {
      localStorage.removeItem('rememberedAccount')
    }

    showMessage('登录成功')

    ctxData.loading = false
    await new Promise(resolve => setTimeout(resolve, 1000))
    await router.push('/')

  } catch (error) {
    console.error('处理登录成功数据失败:', error)
    ctxData.errorMessage = '处理用户数据失败，请稍后重试'
    ctxData.loading = false
  }
}

const processPermissions = (permissions) => {
  const perms = Array.isArray(permissions) ? [...permissions] : []

  const ensureTerminalGroup = (list) => {
    const edge = list.find(p =>
      p && (p.path === '/ruleEngine' || p.meta?.title === '边缘计算')
    )

    if (edge) {
      edge.children = edge.children || []
      const hasTerminalGroup = edge.children.some(c =>
        c && (c.path === '/terminalGroup' || c.meta?.title === '末端群控')
      )

      if (!hasTerminalGroup) {
        const terminalGroup = {
          path: '/terminalGroup',
          name: 'TerminalGroup',
          meta: { title: '末端群控', icon: 'terminal' }
        }

        const scriptIndex = edge.children.findIndex(c =>
          c && c.meta?.title === '规则脚本'
        )

        if (scriptIndex >= 0) {
          edge.children.splice(scriptIndex + 1, 0, terminalGroup)
        } else {
          edge.children.push(terminalGroup)
        }
      }
    } else {
      list.push({
        name: 'RuleEngine',
        path: '/ruleEngine',
        meta: { title: '边缘计算', icon: 'ruleEngine' },
        children: [{
          path: '/terminalGroup',
          name: 'TerminalGroup',
          meta: { title: '末端群控', icon: 'terminal' }
        }]
      })
    }

    return list
  }

  return ensureTerminalGroup(perms)
}

const handleLoginFailure = (res) => {
  ctxData.errorMessage = res.message || '登录失败，请检查用户名和密码'
  getNonce()
}

const handleLoginError = (error) => {
  console.error('登录错误:', error)

  if (error.code === 'ECONNABORTED' || error.code === 'ERR_NETWORK') {
    ctxData.errorMessage = '网络连接失败，请检查网络后重试'
  } else {
    ctxData.errorMessage = '登录异常，请稍后重试'
  }

  getNonce()
}

const handleForgotPassword = () => {
  showMessage('请联系系统管理员重置密码', 'info')
}
</script>

<style scoped>
/* v3.0 — styles in src/styles/ */
</style>


