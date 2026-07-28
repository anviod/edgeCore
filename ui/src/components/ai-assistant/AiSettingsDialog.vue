<template>
  <a-modal
    v-model:visible="visible"
    title="AI 助手设置"
    :width="960"
    modal-class="ai-settings-modal"
    unmount-on-close
    :mask-closable="false"
    render-to-body
    @cancel="handleCancel"
  >
    <!-- 顶部横幅 -->
    <div class="ai-settings-banner">
      <div class="ai-settings-banner__icon">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="3"/><path d="M12 2v3"/><path d="M12 19v3"/><path d="M4.93 4.93l2.12 2.12"/><path d="M16.95 16.95l2.12 2.12"/><path d="M2 12h3"/><path d="M19 12h3"/><path d="M4.93 19.07l2.12-2.12"/><path d="M16.95 7.05l2.12-2.12"/>
        </svg>
      </div>
      <div class="ai-settings-banner__body">
        <div class="ai-settings-banner__title">Industrial Protocol Copilot</div>
        <div class="ai-settings-banner__desc">
          配置保存至 <code>config.db</code> · <code>ai_copilot</code> bucket，立即生效
        </div>
      </div>
    </div>

    <!-- 三大分类 Tabs -->
    <a-tabs v-model:active-key="activeTab" class="ai-settings-tabs" @change="onTabChange">
      <!-- ── Tab 1: MCP 接入 ── -->
      <a-tab-pane key="mcp" title="MCP 接入">
        <div class="ai-settings-tab-body">
          <!-- 单卡片：开关 + 配置一体化 -->
          <div class="ai-settings-card ai-settings-card--vertical">
            <div class="ai-settings-card__row">
              <div class="ai-settings-card__label">
                <span class="ai-settings-card__title">MCP 服务</span>
                <span class="ai-settings-card__desc">
                  {{ form.mcp_enabled ? '30 个工具 · 13 个提示词模板，外部 LLM 可安全操作网关' : '外部客户端无法连接' }}
                </span>
              </div>
              <a-switch v-model="form.mcp_enabled" />
            </div>

            <!-- 开启后展开的配置区域 -->
            <template v-if="form.mcp_enabled">
              <div class="ai-settings-card__divider"></div>

              <!-- 全功能读写 -->
              <div class="ai-settings-status-item">
                <span class="ai-settings-status-dot" :class="form.mcp_full_access ? 'ai-settings-status-dot--on' : 'ai-settings-status-dot--off'"></span>
                <div class="ai-settings-status-item__text">
                  <span class="ai-settings-status-item__label">全功能读写</span>
                  <span class="ai-settings-status-item__desc">
                    {{ form.mcp_full_access ? '所有 CRUD 操作可用' : '仅只读查询' }}
                  </span>
                </div>
                <a-switch v-model="form.mcp_full_access" class="ai-settings-status-item__action" />
              </div>

              <!-- MCP API Key -->
              <div class="ai-settings-status-item">
                <span class="ai-settings-status-dot" :class="form.mcp_api_key_set ? 'ai-settings-status-dot--on' : 'ai-settings-status-dot--off'"></span>
                <div class="ai-settings-status-item__text">
                  <span class="ai-settings-status-item__label">API Key</span>
                  <span class="ai-settings-status-item__desc">
                    {{ form.mcp_api_key_set ? '已设置（' + mcpKeyMasked + '）' : '未设置' }}
                  </span>
                </div>
                <div class="ai-settings-status-item__action">
                  <a-button
                    v-if="form.mcp_api_key_set"
                    size="small"
                    @click="copyMcpKey"
                  >
                    复制
                  </a-button>
                  <a-button size="small" @click="toggleMcpKeyInput">
                    {{ showMcpKeyInput ? '取消' : '设置' }}
                  </a-button>
                </div>
              </div>

              <!-- API Key 输入区 -->
              <div v-if="showMcpKeyInput" class="ai-settings-key-input">
                <a-input-password
                  v-model="mcpKeyDraft"
                  placeholder="输入 MCP API Key（至少 8 位）"
                  allow-clear
                  autocomplete="new-password"
                  name="mcp-api-key-field"
                  :readonly="mcpInputReadonly"
                  @focus="onMcpKeyFocus"
                />
                <div class="ai-settings-key-input__actions">
                  <a-button size="small" :loading="genKeyLoading" @click="generateMcpKey">
                    生成密钥
                  </a-button>
                  <a-button type="primary" size="small" :loading="saveKeyLoading" @click="saveMcpKey">
                    保存
                  </a-button>
                </div>
              </div>

              <!-- 端点信息 -->
              <div class="ai-settings-endpoint ai-settings-endpoint--indented">
                <span class="ai-settings-endpoint__label">端点</span>
                <code class="ai-settings-endpoint__url">POST /api/mcp</code>
                <span class="ai-settings-endpoint__status ai-settings-endpoint__status--on">就绪</span>
              </div>
            </template>

            <!-- 关闭时显示提示文字 -->
            <template v-else>
              <div class="ai-settings-card__divider"></div>
              <div class="ai-settings-card__closed-hint">
                <span>开启后查看客户端配置示例</span>
              </div>
            </template>

            <!-- 帮助按钮：始终居中显示 -->
            <div class="ai-settings-card__divider"></div>
            <div class="ai-settings-card__help-bar">
              <a-button size="small" type="text" @click="openMcpDocs">
                <template #icon>
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
                </template>
                查看接入帮助
              </a-button>
            </div>
          </div>
        </div>
      </a-tab-pane>

      <!-- ── Tab 2: 本地 AI 模型 ── -->
      <a-tab-pane key="remote" title="本地 AI 模型">
        <div class="ai-settings-tab-body">
          <div class="ai-settings-intro">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2" ry="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
            <span>局域网 gRPC 对接 EdgeX AI Model Center，提供协议逆向、文档解析等高阶 AI 能力</span>
          </div>

          <div class="ai-settings-card ai-settings-card--vertical">
            <div class="ai-settings-card__field">
              <label class="ai-settings-card__field-label">gRPC 端点 <span class="ai-settings-card__required">*</span></label>
              <a-input
                v-model="form.grpc_endpoint"
                placeholder="192.168.1.10:50051"
                class="mono-text"
              />
              <div class="ai-settings-card__hint">AI Model Center 的 gRPC 服务地址，格式 IP:Port</div>
            </div>

            <div class="ai-settings-card__divider"></div>

            <div class="ai-settings-card__field">
              <label class="ai-settings-card__field-label">模型</label>
              <a-select
                v-model="form.model"
                allow-create
                allow-search
                placeholder="选择或输入模型 ID"
                popup-container=".ai-settings-modal"
              >
                <a-option v-for="m in remoteModels" :key="m" :value="m" :label="m" />
              </a-select>
              <div class="ai-settings-card__hint">AI Model Center 上注册的服务/模型名称</div>
            </div>
          </div>
        </div>
      </a-tab-pane>

      <!-- ── Tab 3: 云端大模型 ── -->
      <a-tab-pane key="cloud" title="云端大模型">
        <div class="ai-settings-tab-body">
          <div class="ai-settings-intro">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9Z"/></svg>
            <span>直连公网或私有 LLM API — OpenAI、DeepSeek、通义千问、文心一言等</span>
          </div>

          <div class="ai-settings-card">
            <div class="ai-settings-card__row">
              <div class="ai-settings-card__label">
                <span class="ai-settings-card__title">启用云端调用</span>
                <span class="ai-settings-card__desc">未启用时不允许保存云端配置</span>
              </div>
              <a-switch v-model="form.enable_cloud" />
            </div>

            <template v-if="form.enable_cloud">
              <div class="ai-settings-card__divider"></div>

              <!-- 第一行：平台 + Base URL -->
              <div class="ai-settings-card__row--fields">
                <div class="ai-settings-card__field" style="flex:1">
                  <label class="ai-settings-card__field-label">平台 / 提供商 <span class="ai-settings-card__required">*</span></label>
                  <a-select
                    v-model="form.provider"
                    placeholder="选择 AI 平台"
                    allow-search
                    popup-container=".ai-settings-modal"
                    @change="onProviderChange"
                  >
                    <a-option v-for="p in cloudProviders" :key="p.value" :value="p.value" :label="p.label" />
                  </a-select>
                </div>
                <div class="ai-settings-card__field" style="flex:1">
                  <label class="ai-settings-card__field-label">API Base URL <span class="ai-settings-card__required">*</span></label>
                  <a-input v-model="form.base_url" placeholder="https://api.openai.com/v1" class="mono-text" />
                </div>
              </div>

              <!-- 第二行：认证方式 + 模型 -->
              <div class="ai-settings-card__row--fields">
                <div class="ai-settings-card__field" style="flex:1">
                  <label class="ai-settings-card__field-label">认证方式</label>
                  <a-select v-model="form.auth_type" popup-container=".ai-settings-modal">
                    <a-option v-for="a in AI_AUTH_TYPES" :key="a.value" :value="a.value" :label="a.label" />
                  </a-select>
                  <div v-if="authHint" class="ai-settings-card__hint">{{ authHint }}</div>
                </div>
                <div class="ai-settings-card__field" style="flex:1">
                  <label class="ai-settings-card__field-label">{{ form.provider === 'azure-openai' ? '部署名称 (Model)' : '模型' }}</label>
                  <a-select v-if="form.provider !== 'azure-openai'" v-model="form.model" allow-create allow-search placeholder="选择或输入模型 ID" popup-container=".ai-settings-modal">
                    <a-option v-for="m in modelOptions" :key="m" :value="m" :label="m" />
                  </a-select>
                  <a-input v-else v-model="form.model" placeholder="gpt-4o-deployment" class="mono-text" />
                </div>
              </div>

              <!-- 条件字段：API Key -->
              <div class="ai-settings-card__divider" v-if="form.auth_type === 'bearer' || form.auth_type === 'azure_key' || form.auth_type === 'api_key' || form.auth_type === 'basic'"></div>

              <template v-if="form.auth_type === 'bearer' || form.auth_type === 'azure_key'">
                <div class="ai-settings-card__field">
                  <label class="ai-settings-card__field-label">{{ form.auth_type === 'azure_key' ? 'API Key (Azure)' : 'API Key / Token' }}</label>
                  <a-input-password v-model="form.api_key" :placeholder="form.api_key_set ? '已设置，留空保持不变' : '输入 API Key'" allow-clear />
                </div>
              </template>

              <template v-if="form.auth_type === 'api_key'">
                <div class="ai-settings-card__row--fields">
                  <div class="ai-settings-card__field" style="flex:1">
                    <label class="ai-settings-card__field-label">Header 名称</label>
                    <a-input v-model="form.api_key_header" placeholder="X-API-Key" class="mono-text" />
                  </div>
                  <div class="ai-settings-card__field" style="flex:1">
                    <label class="ai-settings-card__field-label">Header 值</label>
                    <a-input-password v-model="form.api_key" :placeholder="form.api_key_set ? '已设置' : '输入密钥'" allow-clear />
                  </div>
                </div>
              </template>

              <template v-if="form.auth_type === 'basic'">
                <div class="ai-settings-card__row--fields">
                  <div class="ai-settings-card__field" style="flex:1">
                    <label class="ai-settings-card__field-label">用户名</label>
                    <a-input v-model="form.username" />
                  </div>
                  <div class="ai-settings-card__field" style="flex:1">
                    <label class="ai-settings-card__field-label">密码</label>
                    <a-input-password v-model="form.password" :placeholder="form.password_set ? '已设置' : '输入密码'" allow-clear />
                  </div>
                </div>
              </template>

              <div v-if="form.auth_type === 'azure_key'" class="ai-settings-card__field">
                <label class="ai-settings-card__field-label">Azure API Version</label>
                <a-input v-model="form.azure_api_version" placeholder="2024-02-15-preview" class="mono-text" />
              </div>
            </template>
          </div>
        </div>
      </a-tab-pane>

      <!-- ── Tab 4: EAN 接入 ── -->
      <a-tab-pane key="ean" title="EAN 接入">
        <div class="ai-settings-tab-body">
          <div class="ai-settings-intro">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M12 2v3"/><path d="M12 19v3"/><path d="M4.93 4.93l2.12 2.12"/><path d="M16.95 16.95l2.12 2.12"/><path d="M2 12h3"/><path d="M19 12h3"/><path d="M4.93 19.07l2.12-2.12"/><path d="M16.95 7.05l2.12-2.12"/></svg>
            <span>Edge Agent Network — EAN 复用北向 edgeOS 通道作为传输层，不单独创建 MQTT/NATS 客户端</span>
          </div>

          <!-- 无北向通道提示 -->
          <div v-if="!eanNorthboundAvailable" class="ai-settings-ean-alert">
            <div class="ai-settings-ean-alert__icon">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
            </div>
            <div class="ai-settings-ean-alert__body">
              <div class="ai-settings-ean-alert__title">未检测到 edgeOS 北向通道</div>
              <div class="ai-settings-ean-alert__desc">
                EAN 依赖北向 edgeOS(MQTT) 或 edgeOS(NATS) 通道进行 Agent 注册、能力发现和事件发布。
                请先创建并启用至少一个 edgeOS 北向通道。
              </div>
              <a-button size="small" type="primary" @click="goToNorthbound">前往北向通道配置</a-button>
            </div>
          </div>

          <!-- 北向通道信息卡片 -->
          <div v-else class="ai-settings-card ai-settings-card--vertical">
            <div class="ai-settings-card__row">
              <div class="ai-settings-card__label">
                <span class="ai-settings-card__title">复用北向通道</span>
                <span class="ai-settings-card__desc">
                  EAN 直接复用已创建的 edgeOS 通道，不额外占用连接资源
                </span>
              </div>
              <span class="ai-settings-status-dot ai-settings-status-dot--on"></span>
            </div>

            <div class="ai-settings-card__divider"></div>

            <!-- 通道详情 -->
            <div class="ai-settings-ean-nb">
              <div class="ai-settings-ean-nb__item">
                <span class="ai-settings-ean-nb__label">通道名称</span>
                <span class="ai-settings-ean-nb__value">{{ eanNorthboundChannel?.name || '—' }}</span>
              </div>
              <div class="ai-settings-ean-nb__item">
                <span class="ai-settings-ean-nb__label">通道类型</span>
                <span class="ai-settings-ean-nb__value">{{ eanNorthboundChannel?.type || '—' }}</span>
              </div>
              <div class="ai-settings-ean-nb__item">
                <span class="ai-settings-ean-nb__label">接入地址</span>
                <code class="ai-settings-ean-mono">{{ eanNorthboundChannel?.broker || eanNorthboundChannel?.url || '—' }}</code>
              </div>
              <div class="ai-settings-ean-nb__item">
                <span class="ai-settings-ean-nb__label">节点 ID</span>
                <code class="ai-settings-ean-mono">{{ eanNorthboundChannel?.node_id || '—' }}</code>
              </div>
              <div class="ai-settings-ean-nb__item">
                <span class="ai-settings-ean-nb__label">运行状态</span>
                <span class="ai-settings-ean-nb__status" :class="eanNorthboundChannel?.status === 'Running' ? 'ok' : 'warn'">
                  {{ eanNorthboundChannel?.status || 'stopped' }}
                </span>
              </div>
            </div>
          </div>

          <div class="ai-settings-card ai-settings-card--vertical" style="margin-top: 12px;">
            <!-- Agent 信息 (只读) -->
            <div class="ai-settings-card__row">
              <div class="ai-settings-card__label">
                <span class="ai-settings-card__title">EAN 服务</span>
                <span class="ai-settings-card__desc">
                  {{ eanSettings.enabled ? 'Agent 已注册 · 传输层就绪' : 'EAN 服务未启用' }}
                </span>
              </div>
              <span class="ai-settings-status-dot" :class="eanSettings.enabled ? 'ai-settings-status-dot--on' : 'ai-settings-status-dot--off'"></span>
            </div>

            <div class="ai-settings-card__divider"></div>

            <!-- Agent ID -->
            <div class="ai-settings-card__row">
              <div class="ai-settings-card__label">
                <span class="ai-settings-card__title">Agent ID</span>
                <span class="ai-settings-card__desc">全局唯一标识符，由 EdgeX 自动生成</span>
              </div>
              <code class="ai-settings-ean-mono">{{ eanSettings.agent_id || '—' }}</code>
            </div>

            <div class="ai-settings-card__divider"></div>

            <!-- 传输协议 + 能力数量 -->
            <div class="ai-settings-card__row--fields">
              <div class="ai-settings-card__field" style="flex:1">
                <label class="ai-settings-card__field-label">传输协议</label>
                <code class="ai-settings-ean-mono">{{ (eanSettings.transport || 'mqtt').toUpperCase() }}</code>
              </div>
              <div class="ai-settings-card__field" style="flex:1">
                <label class="ai-settings-card__field-label">已注册能力</label>
                <span class="ai-settings-ean-count">{{ eanSettings.capabilities_count ?? 0 }}</span>
                <span class="ai-settings-card__hint">由 Driver Commands 自动生成</span>
              </div>
            </div>

            <div class="ai-settings-card__divider"></div>

            <!-- 心跳间隔 -->
            <div class="ai-settings-card__field">
              <label class="ai-settings-card__field-label">心跳间隔 (秒)</label>
              <code class="ai-settings-ean-mono">{{ eanSettings.heartbeat_sec || 60 }}s</code>
              <div class="ai-settings-card__hint">Agent 向 EdgeOS 发送心跳的间隔，默认 60 秒</div>
            </div>

            <div class="ai-settings-card__divider"></div>

            <!-- 事件自动发布 -->
            <div class="ai-settings-card__row">
              <div class="ai-settings-card__label">
                <span class="ai-settings-card__title">事件自动发布</span>
                <span class="ai-settings-card__desc">
                  {{ eanSettings.event_auto_publish ? '设备状态变化时自动发布 EAN Event' : '事件发布已暂停' }}
                </span>
              </div>
              <span class="ai-settings-status-dot" :class="eanSettings.event_auto_publish ? 'ai-settings-status-dot--on' : 'ai-settings-status-dot--off'"></span>
            </div>
          </div>
          <div style="margin-top: 16px; padding-top: 12px; border-top: 1px solid var(--color-border-2);">
            <a-button size="small" type="outline" @click="goToNorthbound">前往北向通道配置</a-button>
            <span style="margin-left: 8px; font-size: 12px; color: var(--color-text-3);">EAN 启停和心跳配置已迁移至北向通道弹窗</span>
          </div>
        </div>
      </a-tab-pane>
    </a-tabs>

    <!-- 接入方式（仅 MCP Tab 可见） -->
    <div v-if="activeTab === 'mcp'" class="ai-settings-access">
      <div class="ai-settings-access__head">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>
        <span>客户端接入</span>
      </div>

      <!-- 客户端 Tabs -->
      <div class="ai-settings-access__tabs">
        <button
          v-for="c in mcpClients"
          :key="c.key"
          class="ai-settings-access__tab"
          :class="{ 'ai-settings-access__tab--active': mcpClient === c.key }"
          @click="mcpClient = c.key"
        >{{ c.label }}</button>
      </div>

      <!-- 配置示例 -->
      <div class="ai-settings-access__codeblock">
        <div class="ai-settings-access__codeblock-header">
          <div class="ai-settings-access__dots">
            <span></span><span></span><span></span>
          </div>
          <span class="ai-settings-access__filename">mcp_config.json</span>
          <button class="ai-settings-access__copy" @click="copyMcpConfig">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
            <span>{{ copyLabel }}</span>
          </button>
        </div>
        <pre class="ai-settings-access__code"><code>{{ mcpClientConfig }}</code></pre>
      </div>
    </div>

    <template #footer>
      <a-button @click="handleCancel">取消</a-button>
      <a-button type="primary" :loading="saving" @click="handleSave">保存配置</a-button>
    </template>
  </a-modal>

  <!-- MCP 帮助文档抽屉 -->
  <a-drawer
    v-model:visible="docsVisible"
    title="MCP 接入帮助"
    :width="720"
    :footer="false"
    unmount-on-close
    render-to-body
    class="ai-mcp-docs-drawer"
  >
    <div v-if="docsLoading" class="ai-mcp-docs-loading">
      <a-spin />
      <span>加载中...</span>
    </div>
    <div v-else class="ai-mcp-docs-content" v-html="docsHtml"></div>
  </a-drawer>
</template>

<script setup>
import { ref, computed, watch, reactive } from 'vue'
import { Message } from '@arco-design/web-vue'
import {
  AI_AUTH_TYPES,
  AI_PROVIDERS,
  defaultAiSettings,
  findProvider,
  applyProviderPreset
} from '@/constants/aiProviders'
import eanApi from '@/api/ean'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  settings: { type: Object, default: null },
  saving: { type: Boolean, default: false }
})

const emit = defineEmits(['update:modelValue', 'save', 'refresh'])

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

const form = ref(defaultAiSettings())
const activeTab = ref('remote')

/* ── EAN 设置 ── */
const eanSettings = reactive({
  enabled: false,
  agent_id: '',
  transport: 'mqtt',
  heartbeat_sec: 60,
  event_auto_publish: true,
  capabilities_count: 0
})
const eanNorthboundAvailable = ref(false)
const eanNorthboundChannel = ref(null)

async function fetchEanSettings() {
  try {
    const res = await eanApi.getSettings()
    if (res?.code === '0' || res?.code === 0) {
      const d = res.data || {}
      eanSettings.enabled = d.enabled ?? false
      eanSettings.agent_id = d.agent_id || ''
      eanSettings.transport = d.transport || 'mqtt'
      eanSettings.heartbeat_sec = d.heartbeat_sec ?? 60
      eanSettings.event_auto_publish = d.event_auto_publish ?? true
      eanSettings.capabilities_count = d.capabilities_count ?? 0
      eanNorthboundAvailable.value = d.northbound_available ?? false
      eanNorthboundChannel.value = d.northbound_channel || null
    }
  } catch {
    // EAN 后端不可用时保持默认值
  }
}

function goToNorthbound() {
  visible.value = false
  if (typeof window !== 'undefined') {
    window.location.href = '/#/northbound'
  }
}

/* ── 接入方式 ── */
const mcpClient = ref('claude')
const copyLabel = ref('复制')
const mcpClients = [
  { key: 'claude', label: 'Claude Desktop' },
  { key: 'cursor', label: 'Cursor' },
  { key: 'windsurf', label: 'Windsurf' },
  { key: 'continue', label: 'Continue.dev' }
]

const mcpEndpoint = computed(() => {
  if (typeof window !== 'undefined') {
    return window.location.origin + '/api/mcp'
  }
  return 'http://localhost:5173/api/mcp'
})

const mcpClientConfig = computed(() => {
  const key = lastMcpKeyForCopy.value || form.value.mcp_api_key || '替换为真实的 API Key'
  const configs = {
    claude: {
      mcpServers: {
        edgex: {
          url: mcpEndpoint.value,
          headers: { Authorization: `Bearer ${key}` }
        }
      }
    },
    cursor: {
      mcpServers: {
        edgex: {
          url: mcpEndpoint.value,
          headers: { Authorization: `Bearer ${key}` }
        }
      }
    },
    windsurf: {
      mcpServers: {
        edgex: {
          url: mcpEndpoint.value,
          headers: { Authorization: `Bearer ${key}` }
        }
      }
    },
    continue: {
      mcpServers: {
        edgex: {
          transport: { type: 'http', url: mcpEndpoint.value },
          auth: { type: 'bearer', token: key }
        }
      }
    }
  }
  return JSON.stringify(configs[mcpClient.value] || configs.claude, null, 2)
})

function copyMcpConfig() {
  if (typeof navigator !== 'undefined' && navigator.clipboard) {
    navigator.clipboard.writeText(mcpClientConfig.value).then(() => {
      copyLabel.value = '已复制'
      setTimeout(() => { copyLabel.value = '复制' }, 2000)
    }).catch(() => {
      fallbackCopy()
    })
  } else {
    fallbackCopy()
  }
}

function fallbackCopy() {
  const ta = document.createElement('textarea')
  ta.value = mcpClientConfig.value
  ta.style.position = 'fixed'; ta.style.opacity = '0'
  document.body.appendChild(ta)
  ta.select()
  document.execCommand('copy')
  document.body.removeChild(ta)
  copyLabel.value = '已复制'
  setTimeout(() => { copyLabel.value = '复制' }, 2000)
}

/* ── MCP 状态展示 ── */
const mcpKeyMasked = computed(() => {
  const key = lastMcpKeyForCopy.value || form.value.mcp_api_key
  if (!key) return '****'
  if (key.length <= 8) return key.substring(0, 2) + '****' + key.substring(key.length - 2)
  return key.substring(0, 4) + '****' + key.substring(key.length - 4)
})

/* ── MCP API Key 管理（独立保存，不依赖 form 保存） ── */
const showMcpKeyInput = ref(false)
const mcpKeyDraft = ref('')
const genKeyLoading = ref(false)
const saveKeyLoading = ref(false)
const mcpInputReadonly = ref(true)

// 使用 sessionStorage 持久化 MCP Key，避免 unmount-on-close 导致丢失
const MCP_KEY_STORAGE = 'edgex_mcp_api_key'
function loadMcpKeyFromStorage() {
  if (typeof sessionStorage !== 'undefined') {
    try { return sessionStorage.getItem(MCP_KEY_STORAGE) || '' } catch (e) {}
  }
  return ''
}
function saveMcpKeyToStorage(key) {
  if (typeof sessionStorage !== 'undefined') {
    try {
      if (key) sessionStorage.setItem(MCP_KEY_STORAGE, key)
      else sessionStorage.removeItem(MCP_KEY_STORAGE)
    } catch (e) {}
  }
}
const lastMcpKeyForCopy = ref(loadMcpKeyFromStorage())

function getAuthToken() {
  if (typeof localStorage !== 'undefined') {
    try {
      const raw = localStorage.getItem('loginInfo')
      if (raw) {
        const parsed = JSON.parse(raw)
        return parsed.token || (parsed.data && parsed.data.token) || ''
      }
    } catch (e) {}
  }
  return ''
}

async function generateMcpKey() {
  genKeyLoading.value = true
  try {
    const token = getAuthToken()
    const headers = { 'Content-Type': 'application/json' }
    if (token) headers['Authorization'] = `Bearer ${token}`

    // 1. 生成密钥
    const genResp = await fetch('/api/mcp/generate-key', { method: 'POST', headers })
    const genData = await genResp.json()
    if (genData.code !== '0') {
      Message.error(genData.message || '生成失败')
      return
    }
    const newKey = genData.data.api_key

    // 2. 立即通过 activate API 保存
    const saveResp = await fetch('/api/mcp/activate', {
      method: 'POST',
      headers,
      body: JSON.stringify({ api_key: newKey, full_access: form.value.mcp_full_access })
    })
    const saveData = await saveResp.json()
    if (saveData.code === '0') {
      mcpKeyDraft.value = newKey
      lastMcpKeyForCopy.value = newKey
      saveMcpKeyToStorage(newKey)
      form.value.mcp_api_key_set = true
      emit('refresh')
      Message.success('已生成并保存 64 位随机密钥')
    } else {
      Message.error(saveData.message || '保存失败')
    }
  } catch {
    Message.error('网络错误')
  } finally {
    genKeyLoading.value = false
  }
}

async function saveMcpKey() {
  const key = mcpKeyDraft.value.trim()
  if (!key || key.length < 8) {
    Message.warning('API Key 至少需要 8 位字符')
    return
  }

  saveKeyLoading.value = true
  try {
    const token = getAuthToken()
    const headers = { 'Content-Type': 'application/json' }
    if (token) headers['Authorization'] = `Bearer ${token}`

    const resp = await fetch('/api/mcp/activate', {
      method: 'POST',
      headers,
      body: JSON.stringify({ api_key: key, full_access: form.value.mcp_full_access })
    })
    const data = await resp.json()
    if (data.code === '0') {
      form.value.mcp_api_key_set = true
      lastMcpKeyForCopy.value = key
      saveMcpKeyToStorage(key)
      showMcpKeyInput.value = false
      mcpKeyDraft.value = ''
      emit('refresh')
      Message.success('MCP API Key 已保存')
    } else {
      Message.error(data.message || '保存失败')
    }
  } catch {
    Message.error('网络错误')
  } finally {
    saveKeyLoading.value = false
  }
}

function toggleMcpKeyInput() {
  showMcpKeyInput.value = !showMcpKeyInput.value
  if (showMcpKeyInput.value) {
    mcpKeyDraft.value = ''
    mcpInputReadonly.value = true
    // 延迟二次清空，应对浏览器自动填充
    setTimeout(() => {
      if (mcpKeyDraft.value && (mcpKeyDraft.value.includes('passwd') || mcpKeyDraft.value.includes('password'))) {
        mcpKeyDraft.value = ''
      }
    }, 150)
  }
}

function onMcpKeyFocus() {
  mcpInputReadonly.value = false
  // 如果浏览器已自动填充非用户输入的内容，清空
  if (mcpKeyDraft.value && (mcpKeyDraft.value.includes('passwd') || mcpKeyDraft.value.includes('password'))) {
    mcpKeyDraft.value = ''
  }
}

function copyMcpKey() {
  const key = lastMcpKeyForCopy.value || mcpKeyDraft.value
  if (!key) {
    Message.warning('无可复制的 Key，请重新生成或设置')
    return
  }
  if (typeof navigator !== 'undefined' && navigator.clipboard) {
    navigator.clipboard.writeText(key).then(() => {
      Message.success('MCP API Key 已复制到剪贴板')
    }).catch(() => {
      fallbackCopyText(key)
    })
  } else {
    fallbackCopyText(key)
  }
}

function fallbackCopyText(text) {
  const ta = document.createElement('textarea')
  ta.value = text
  ta.style.position = 'fixed'; ta.style.opacity = '0'
  document.body.appendChild(ta)
  ta.select()
  document.execCommand('copy')
  document.body.removeChild(ta)
  Message.success('MCP API Key 已复制到剪贴板')
}

// MCP 帮助文档抽屉
const docsVisible = ref(false)
const docsHtml = ref('')
const docsLoading = ref(false)

async function openMcpDocs() {
  docsVisible.value = true
  if (docsHtml.value) return
  docsLoading.value = true
  try {
    const token = getAuthToken()
    const headers = { 'Content-Type': 'application/json' }
    if (token) headers['Authorization'] = `Bearer ${token}`
    const resp = await fetch('/api/mcp/help', { headers })
    if (resp.ok) {
      const data = await resp.json()
      docsHtml.value = renderHelpDoc(data)
    } else {
      docsHtml.value = `<p style="padding:24px;color:var(--ai-text-muted)">请求失败 (${resp.status})：请确认已登录系统</p>`
    }
  } catch {
    docsHtml.value = '<p style="padding:24px;color:var(--ai-text-muted)">无法加载文档，请检查网络连接</p>'
  } finally {
    docsLoading.value = false
  }
}

function renderHelpDoc(data) {
  if (!data) return '<p style="padding:24px;color:var(--ai-text-muted)">无数据</p>'
  let html = ''
  html += `<header class="ai-mcp-docs-hero"><h2>${esc(data.title || '')}</h2><p>${esc(data.description || '')}</p></header>`
  if (data.architecture?.layers?.length) {
    html += `<section class="ai-mcp-docs-section"><h3>系统架构</h3><div class="ai-mcp-docs-arch">`
    html += data.architecture.layers.map((l, i) => {
      const colorMap = { purple: '#8b5cf6', blue: '#3b82f6', green: '#22c55e', orange: '#f59e0b' }
      const bgMap = { purple: 'rgba(139,92,246,0.12)', blue: 'rgba(59,130,246,0.12)', green: 'rgba(34,197,94,0.12)', orange: 'rgba(245,158,11,0.12)' }
      const c = colorMap[l.color] || '#6b7280'
      const bg = bgMap[l.color] || 'rgba(107,114,128,0.12)'
      let node = `<div class="ai-mcp-docs-arch__node" style="border-color:${c};background:${bg}"><strong>${esc(l.name)}</strong><br><small>${esc(l.desc)}</small></div>`
      let arrow = i < data.architecture.layers.length - 1 ? `<div class="ai-mcp-docs-arch__arrow">&#x2193;</div>` : ''
      return node + arrow
    }).join('')
    html += `</div></section>`
  }
  html += `<section class="ai-mcp-docs-section"><h3>传输协议</h3><div class="ai-mcp-docs-grid">`
  html += `<div class="ai-mcp-docs-grid__item"><span class="ai-mcp-docs-grid__label">传输方式</span><code>${esc(data.transport || '')}</code></div>`
  html += `<div class="ai-mcp-docs-grid__item"><span class="ai-mcp-docs-grid__label">端点</span><code>${esc(data.endpoint || '')}</code></div>`
  html += `<div class="ai-mcp-docs-grid__item"><span class="ai-mcp-docs-grid__label">认证方式</span><code>${esc(data.auth_mode || '')}</code></div>`
  html += `</div></section>`
  if (data.tools?.length) {
    const readTools = data.tools.filter(t => t.category === 'read')
    const writeTools = data.tools.filter(t => t.category === 'write')
    html += `<section class="ai-mcp-docs-section"><h3>MCP 工具清单 (${data.tools.length} 个)</h3>`
    html += `<h4 class="ai-mcp-docs-subtitle"><span class="ai-mcp-docs-dot" style="background:#22c55e"></span> 只读查询 (${readTools.length} 个)</h4>`
    html += `<div class="ai-mcp-docs-tool-grid">`
    for (const t of readTools) {
      html += `<div class="ai-mcp-docs-tool-card"><code>${esc(t.name)}</code><p>${esc(t.description)}</p></div>`
    }
    html += `</div>`
    html += `<h4 class="ai-mcp-docs-subtitle"><span class="ai-mcp-docs-dot" style="background:#f59e0b"></span> 全功能 CRUD (${writeTools.length} 个)</h4>`
    html += `<div class="ai-mcp-docs-tool-grid">`
    for (const t of writeTools) {
      html += `<div class="ai-mcp-docs-tool-card"><code>${esc(t.name)}</code><p>${esc(t.description)}</p></div>`
    }
    html += `</div></section>`
  }
  html += `<section class="ai-mcp-docs-section"><h3>安全说明</h3><div class="ai-mcp-docs-card"><ul class="ai-mcp-docs-security-list">`
  html += `<li>全功能 CRUD 操作需要用户在 UI 中确认激活</li>`
  html += `<li>所有操作通过 MCP API Key 认证（Bearer 或 X-MCP-API-Key）</li>`
  html += `<li>MCP API Key 独立于系统 JWT，可随时更换</li>`
  html += `<li>敏感信息已脱敏处理，端点仅在内网暴露</li>`
  html += `</ul></div></section>`
  html += `<section class="ai-mcp-docs-section"><h3>API 端点</h3><div class="ai-mcp-docs-grid">`
  html += `<div class="ai-mcp-docs-grid__item"><span class="ai-mcp-docs-grid__label">MCP 协议接入</span><code>POST ${esc(data.endpoint || '/api/mcp')}</code></div>`
  html += `<div class="ai-mcp-docs-grid__item"><span class="ai-mcp-docs-grid__label">激活全功能</span><code>POST /api/mcp/activate</code></div>`
  html += `<div class="ai-mcp-docs-grid__item"><span class="ai-mcp-docs-grid__label">查询状态</span><code>GET /api/mcp/status</code></div>`
  html += `<div class="ai-mcp-docs-grid__item"><span class="ai-mcp-docs-grid__label">帮助文档</span><code>GET /api/mcp/help</code></div>`
  html += `</div></section>`
  return html
}

function esc(s) {
  if (!s) return ''
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

const cloudProviders = computed(() =>
  AI_PROVIDERS.filter((p) => p.deploymentMode === 'cloud')
)

const remoteModels = computed(() => {
  const preset = findProvider('edgex-center')
  return preset?.models || []
})

const modelOptions = computed(() => {
  const preset = findProvider(form.value.provider)
  return preset?.models || []
})

const authHint = computed(() => {
  const a = AI_AUTH_TYPES.find((t) => t.value === form.value.auth_type)
  return a?.desc || ''
})

// MCP Tab / EAN Tab 独立于 deployment_mode — 不会改变 deployment_mode 也不会被其控制
const onTabChange = (key) => {
  // MCP / EAN Tab 是独立配置区，不映射到 deployment_mode
  if (key !== 'mcp' && key !== 'ean') {
    form.value.deployment_mode = key
  }
}

const onProviderChange = (provider) => {
  applyProviderPreset(form.value, provider)
}

// syncForm — 弹窗打开时同步表单，并从后端拉取 MCP API Key 明文
// 必须为 async：后端 ToPublic() 会清空 mcp_api_key 字段，只有
// /api/mcp/key 端点能返回明文，必须 await 完成后 UI 才能正确显示
const syncForm = async (settings) => {
  form.value = { ...defaultAiSettings(), ...(settings || {}) }
  // 始终从后端获取 MCP Key 明文 — 不依赖 sessionStorage（可能过期/被清）
  await fetchMcpKeyFromBackend()
  // 并行拉取 EAN 设置
  fetchEanSettings()
  // MCP Tab 状态由自身 mcp_enabled 决定，不跟随 deployment_mode
  if (form.value.mcp_enabled) {
    activeTab.value = 'mcp'
  } else {
    activeTab.value = form.value.deployment_mode === 'cloud' ? 'cloud' : 'remote'
  }
}

// 通过 JWT 认证 API 获取已保存的 MCP API Key
// 后端 ToPublic() 出于安全会清空 mcp_api_key 字段，
// 只有 /api/mcp/key 端点能返回明文（需 JWT 认证）
async function fetchMcpKeyFromBackend() {
  try {
    const token = getAuthToken()
    const headers = {}
    if (token) headers['Authorization'] = `Bearer ${token}`
    const resp = await fetch('/api/mcp/key', { headers })
    if (!resp.ok) {
      // HTTP 错误（401 等）— 回退到 sessionStorage 缓存
      const cached = loadMcpKeyFromStorage()
      if (cached) lastMcpKeyForCopy.value = cached
      return
    }
    const data = await resp.json()
    if (data.code === '0' && data.data?.key_set && data.data?.api_key) {
      lastMcpKeyForCopy.value = data.data.api_key
      saveMcpKeyToStorage(data.data.api_key)
    }
    // key_set 为 false 时不主动清空 — 保留当前会话内的缓存值
  } catch (e) {
    // 网络错误 — 回退到 sessionStorage 缓存
    const cached = loadMcpKeyFromStorage()
    if (cached) lastMcpKeyForCopy.value = cached
  }
}

// 仅在弹窗打开时同步表单，不在弹窗打开期间响应 settings 变化
// 避免 emit('refresh') 后父组件 fetchSettings 导致表单被重置
watch(visible, async (open) => {
  if (open && props.settings) await syncForm(props.settings)
})

const handleCancel = () => {
  visible.value = false
}

const handleSave = () => {
  if (form.value.deployment_mode === 'cloud' && !form.value.enable_cloud) {
    Message.warning('云端模式需先启用「启用云端调用」')
    return
  }
  if (form.value.deployment_mode === 'remote' && !form.value.grpc_endpoint?.trim()) {
    Message.warning('请填写 AI Model Center gRPC 端点')
    return
  }
  if (form.value.deployment_mode === 'cloud' && !form.value.base_url?.trim()) {
    Message.warning('请填写 API Base URL')
    return
  }
  emit('save', { ...form.value })
}
</script>