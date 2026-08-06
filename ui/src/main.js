import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import { createPinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import ArcoVue from '@arco-design/web-vue'
import '@arco-design/web-vue/dist/arco.css'
import './assets/css/fonts.css'
import './styles/theme.css'
import './styles/animations/index.css'
import './styles/shell.css'
import './styles/globals.css'
import './styles/form-controls.css'
import './styles/page-layout.css'
import './styles/spacing.css'
import './styles/views-shared.css'
import './styles/lists-views.css'
import './styles/components-shared.css'
import './styles/install.css'
import './styles/northbound-form.css'
import './styles/northbound.css'
import './styles/arco-overrides.css'
import './styles/config-modal.css'
import './styles/help-drawer.css'
import './styles/virtual-shadow.css'
import './styles/dark-arco.css'
import './styles/node-sync.css'
import './styles/system-settings.css'
import './styles/edge-compute.css'
import './styles/channel-list.css'
import './styles/log-viewer.css'
import './styles/channel-metrics.css'
import './styles/dashboard.css'
import './styles/dashboard-soak.css'
import './styles/history-modal.css'
import './styles/ai-assistant.css'
import './styles/breathing.css'

const i18n = createI18n({
  legacy: false,
  locale: 'zh',
  messages: {
    zh: {}
  }
})

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(ArcoVue)
app.use(i18n)

app.mount('#app')
