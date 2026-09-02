<template>
  <div class="app-background">
    <aside
      v-if="!isLoginPage"
      class="industrial-sidebar"
      :class="{ 'is-collapsed': drawerRail }"
    >
      <div class="sidebar-header">
        <div class="logo-icon">
          <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
          </svg>
        </div>
        <span v-if="!drawerRail" class="logo-text">edgeCore</span>
      </div>

      <nav class="sidebar-nav">
        <router-link to="/" class="nav-item" active-class="nav-item-active" exact>
          <span class="nav-icon"><icon-apps /></span>
          <span v-if="!drawerRail" class="nav-text">首页监控</span>
        </router-link>
        <router-link to="/channels" class="nav-item" active-class="nav-item-active">
          <span class="nav-icon"><icon-link /></span>
          <span v-if="!drawerRail" class="nav-text">采集通道</span>
        </router-link>
        <router-link to="/edge-compute" class="nav-item" active-class="nav-item-active">
          <span class="nav-icon"><icon-tool /></span>
          <span v-if="!drawerRail" class="nav-text">边缘计算</span>
        </router-link>
        <router-link to="/virtual-shadows" class="nav-item" active-class="nav-item-active">
          <span class="nav-icon"><icon-thunderbolt /></span>
          <span v-if="!drawerRail" class="nav-text">虚拟影子</span>
        </router-link>
        <router-link to="/northbound" class="nav-item" active-class="nav-item-active">
          <span class="nav-icon"><icon-arrow-up /></span>
          <span v-if="!drawerRail" class="nav-text">北向接口</span>
        </router-link>
        <router-link to="/logs" class="nav-item" active-class="nav-item-active">
          <span class="nav-icon"><icon-file /></span>
          <span v-if="!drawerRail" class="nav-text">系统日志</span>
        </router-link>
        <router-link to="/system" class="nav-item" active-class="nav-item-active">
          <span class="nav-icon"><icon-settings /></span>
          <span v-if="!drawerRail" class="nav-text">系统设置</span>
        </router-link>
      </nav>

      <div class="sidebar-footer">
        <div v-if="!drawerRail" class="sidebar-status">
          <span class="status-indicator"></span>
          <span class="status-text">已连接</span>
        </div>
        <div v-if="!drawerRail" class="version-info">
          <span class="version-value">{{ systemVersion }}</span>
          <span v-if="buildTime" class="version-buildtime">{{ buildTime }}</span>
        </div>
        <button class="collapse-btn" @click="drawerRail = !drawerRail">
          <icon-arrow-left
            class="collapse-icon"
            :class="{ 'is-collapsed': drawerRail }"
            :size="14"
          />
          <span v-if="!drawerRail">收起</span>
        </button>
      </div>
    </aside>

    <header v-if="!isLoginPage" class="industrial-header" :class="{ 'is-collapsed': drawerRail }">
      <div class="header-title">
        <span class="title-main">边缘计算网关</span>
        <span v-if="breadcrumb" class="title-sub">{{ breadcrumb }}</span>
      </div>
      <div class="header-actions">
        <button
          class="ai-assistant-trigger"
          :class="{ 'mcp-glow': mcpConnected }"
          title="AI 助手"
          @click="openAiAssistant"
        >
          <AiAssistantIcon />
          <span class="ai-assistant-trigger__label">AI助手</span>
        </button>
        <button class="theme-toggle" @click="toggleTheme" title="切换主题">
          <icon-sun-fill v-if="isDarkTheme" :size="18" />
          <icon-moon-fill v-else :size="18" />
        </button>
        <div class="user-menu" @click="toggleUserMenu" ref="userMenuRef">
          <div class="user-avatar">
            <span>{{ userInitials }}</span>
          </div>
          <span class="user-name">{{ user.username || 'Admin' }}</span>
          <icon-caret-down class="dropdown-icon" :class="{ 'is-open': userMenuOpen }" :size="14" />
          <div v-if="userMenuOpen" class="dropdown-menu">
            <div class="dropdown-item" @click.stop="openChangePassword">
              <icon-lock :size="14" />
              <span>修改密码</span>
            </div>
            <div class="dropdown-divider"></div>
            <div class="dropdown-item text-warning" @click.stop="handleRestart">
              <icon-refresh :size="14" />
              <span>软件重启</span>
            </div>
            <div class="dropdown-item text-error" @click.stop="handleLogout">
              <icon-arrow-right :size="14" />
              <span>退出登录</span>
            </div>
          </div>
        </div>
      </div>
    </header>

    <main class="main-content" :class="{ 'has-sidebar': !isLoginPage, 'is-collapsed': drawerRail }">
      <div v-if="!isLoginPage" class="page-container">
        <router-view v-slot="{ Component }">
          <transition :name="transitionName" mode="out-in">
            <component v-if="Component" :is="Component" :key="$route.fullPath" />
          </transition>
        </router-view>
      </div>
      <router-view v-else v-slot="{ Component }">
        <transition name="fade" mode="out-in">
          <component v-if="Component" :is="Component" :key="$route.fullPath" />
        </transition>
      </router-view>
    </main>

    <change-password-dialog ref="changePwdRef" />
    <AiAssistantPanel ref="aiAssistantRef" />

    <a-modal
      v-model:visible="restartModalVisible"
      title="重启系统"
      ok-text="确认重启"
      cancel-text="取消"
      status="warning"
      @ok="confirmRestart"
    >
      <p>确定要重启系统吗？</p>
      <p class="text-secondary">服务将暂时不可用，重启过程可能需要几分钟时间。</p>
    </a-modal>

    <a-notification
      v-model:visible="snackbar.show"
      :type="snackbar.color === 'error' ? 'error' : snackbar.color === 'warning' ? 'warning' : snackbar.color === 'success' ? 'success' : 'info'"
      :title="snackbar.text"
      :duration="3000"
      style="position: fixed; top: 20px; right: 20px; z-index: 1000"
    >
      <template #extra>
        <a-button type="text" size="small" @click="snackbar.show = false">关闭</a-button>
      </template>
    </a-notification>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { globalState, showMessage } from './composables/useGlobalState'
import { userStore } from '@/stores/user'
import LoginApi from '@/api/login'
import ChangePasswordDialog from '@/components/ChangePasswordDialog.vue'
import AiAssistantIcon from '@/components/ai-assistant/AiAssistantIcon.vue'
import AiAssistantPanel from '@/components/ai-assistant/AiAssistantPanel.vue'
import { useAiAssistant } from './composables/useAiAssistant'
import {
  IconApps, IconLink, IconSettings, IconArrowUp,
  IconFile, IconTool, IconThunderbolt, IconSunFill, IconMoonFill,
  IconLock, IconRefresh, IconArrowRight,
  IconArrowLeft, IconCaretDown
} from '@arco-design/web-vue/es/icon'

const route = useRoute()
const router = useRouter()
const drawerRail = ref(false)
const snackbar = globalState.snackbar
const user = userStore()
const previousLevel = ref(1)

const transitionName = computed(() => {
  const toLevel = route.meta?.level ?? 1
  if (toLevel > previousLevel.value) return 'page-slide'
  if (toLevel < previousLevel.value) return 'page-back'
  return 'fade'
})

watch(
  () => route.path,
  () => {
    previousLevel.value = route.meta?.level ?? 1
  }
)
const changePwdRef = ref(null)
const restartModalVisible = ref(false)
const isDarkTheme = ref(false)
const aiAssistantRef = ref(null)
const { mcpConnected } = useAiAssistant()

const openAiAssistant = () => {
  aiAssistantRef.value?.open()
}

const systemVersion = ref('dev')
const buildTime = ref('')

const isLoginPage = computed(() => route.path === '/login' || route.path === '/install')

const breadcrumb = computed(() => {
  const parts = []
  if (route.meta.title) parts.push(route.meta.title)
  if (globalState.navTitle) parts.push(globalState.navTitle)
  return parts.join(' / ')
})

const userInitials = computed(() => (user.username || 'A').charAt(0).toUpperCase())

const fetchSystemInfo = async () => {
  try {
    const res = await LoginApi.getSystemInfo()
    if (res.code === '0' && res.data) {
      systemVersion.value = `v${res.data.softVer || 'dev'}`
      buildTime.value = res.data.buildTime || ''
    }
  } catch (e) {
    console.error('获取系统信息失败:', e)
  }
}

const applyTheme = (dark) => {
  document.body.classList.toggle('dark-theme', dark)
  document.documentElement.classList.toggle('dark-theme', dark)
  if (dark) {
    document.body.setAttribute('arco-theme', 'dark')
  } else {
    document.body.removeAttribute('arco-theme')
  }
}

const toggleTheme = () => {
  isDarkTheme.value = !isDarkTheme.value
  localStorage.setItem('theme', isDarkTheme.value ? 'dark' : 'light')
  applyTheme(isDarkTheme.value)
}

const userMenuOpen = ref(false)
const userMenuRef = ref(null)

const toggleUserMenu = () => {
  userMenuOpen.value = !userMenuOpen.value
}

const handleClickOutside = (event) => {
  if (userMenuRef.value && !userMenuRef.value.contains(event.target)) {
    userMenuOpen.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  const savedTheme = localStorage.getItem('theme')
  isDarkTheme.value = savedTheme === 'dark'
  applyTheme(isDarkTheme.value)
  if (!user.username) {
    try {
      const loginInfo = localStorage.getItem('loginInfo')
      if (loginInfo) {
        const parsed = JSON.parse(loginInfo)
        if (parsed?.username) {
          user.setLoginInfo({ userName: parsed.username }, parsed.permissions || [], parsed.token || '')
        }
      }
    } catch (e) {
      console.error('Failed to restore user info', e)
    }
  }
  fetchSystemInfo()
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})

const openChangePassword = () => {
  changePwdRef.value?.open()
}

const handleLogout = async () => {
  try {
    await LoginApi.logout()
  } catch (e) {
    console.error(e)
  }
  localStorage.removeItem('loginInfo')
  user.setLoginInfo({}, [], '')
  router.push('/login')
  showMessage('已退出登录')
}

const handleRestart = () => {
  restartModalVisible.value = true
}

const confirmRestart = () => {
  restartModalVisible.value = false
  LoginApi.restartSystem().then(() => {
    showMessage('系统正在重启...', 'warning')
    setTimeout(() => window.location.reload(), 5000)
  }).catch(e => {
    showMessage('重启指令发送失败: ' + e.message, 'error')
  })
}
</script>

<style scoped>
.industrial-sidebar,
.industrial-header,
.main-content {
  will-change: width, left, margin-left;
}

.collapse-icon {
  transition: transform var(--motion-duration-normal) var(--motion-ease-bounce);
}

.collapse-icon.is-collapsed {
  transform: rotate(180deg);
}

.ai-assistant-trigger {
  transition:
    background-color var(--motion-duration-fast) var(--motion-ease-standard),
    box-shadow var(--motion-duration-fast) var(--motion-ease-standard),
    transform var(--motion-duration-fast) var(--motion-ease-standard);
}

.ai-assistant-trigger:hover {
  transform: translateY(-1px);
}

.ai-assistant-trigger.mcp-glow {
  animation: pulse-glow 2s var(--motion-ease-standard) infinite;
}

.dropdown-menu {
  animation: scale-soft-in var(--motion-duration-fast) var(--motion-ease-decelerate);
  transform-origin: top right;
}

.dropdown-item {
  transition:
    background-color var(--motion-duration-fast) var(--motion-ease-standard),
    color var(--motion-duration-fast) var(--motion-ease-standard);
}

.dropdown-icon {
  transition: transform var(--motion-duration-fast) var(--motion-ease-bounce);
}

.dropdown-icon.is-open {
  transform: rotate(180deg);
}

.user-avatar {
  transition:
    background-color var(--motion-duration-fast) var(--motion-ease-standard),
    transform var(--motion-duration-fast) var(--motion-ease-standard);
}

.user-menu:hover .user-avatar {
  transform: scale(1.05);
}
</style>

