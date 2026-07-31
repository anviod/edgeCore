<script setup>
import { ref, computed } from 'vue'

const activeStep = ref(0)

const debugSteps = [
  {
    id: 0,
    title: '检查北向通道',
    description: '确认 EdgeX 已配置 edgeOS MQTT 或 edgeOS NATS 北向通道并成功连接',
    checklist: [
      '进入「节点同步」页面，确认 edgeOS 连接状态为「已连接」',
      '北向通道协议选择 edgeOS(MQTT) 或 edgeOS(NATS)',
      'Broker 地址正确且网络可达',
      'MQTT 默认端口 1883，NATS 默认端口 4222'
    ],
    api: 'GET /api/northbound/edgeos-mqtt/:id/stats',
    expected: '连接状态 connected = true'
  },
  {
    id: 1,
    title: '验证 Agent 注册',
    description: 'EdgeX 启动后自动发布 Agent Descriptor 到 $edgeos/discovery/agent',
    checklist: [
      '打开 AI 助手面板 → EAN Tab → Agent 子 Tab',
      'Agent 状态显示「在线」且 Agent ID 正确',
      'Capability 数 > 0（自动生成）',
      'Transport 与北向通道一致（MQTT/NATS）'
    ],
    api: 'GET /api/capability/agent/status',
    expected: 'status: "online", capabilities.length > 0'
  },
  {
    id: 2,
    title: '查看 Capability 注册表',
    description: 'Driver Commands 自动映射为 Capability 并注册',
    checklist: [
      '切换到 Capability 子 Tab',
      '确认包含 device 类能力（如 modbus_tcp.read_holding_register）',
      '确认包含 system.diagnostics 系统能力',
      '点击 Capability 行展开查看 input/output schema'
    ],
    api: 'GET /api/capability/list',
    expected: '返回 capabilities 数组，每个含 id/category/permission'
  },
  {
    id: 3,
    title: '测试 Capability 调用',
    description: '通过 Invoke Console 手动调用 Capability 验证执行链路',
    checklist: [
      '切换到 Invoke 子 Tab',
      '选择一个 read 类 Capability（如 modbus_tcp.read_holding_register）',
      '填写 Arguments JSON（device_id + address）',
      '点击「执行调用」，确认状态变为 completed',
      '检查 Result 包含正确的 values 数组'
    ],
    api: 'POST /api/capability/invoke',
    expected: 'status: "completed", result.success: true'
  },
  {
    id: 4,
    title: '验证 Event 发布',
    description: '设备状态变化时 EdgeX 自动发布 EAN Event',
    checklist: [
      '切换到 Event 子 Tab',
      '触发设备状态变化（如写入点位、设备上下线）',
      '确认事件列表出现 temperature.changed / device.online 等',
      '点击事件行展开查看完整 payload'
    ],
    api: 'GET /api/capability/events/history',
    expected: '返回 events 数组，含 event_type/timestamp'
  },
  {
    id: 5,
    title: 'EdgeOS Discovery 验证',
    description: '确认 EdgeOS 能发现 EdgeX Agent 及其 Capability',
    checklist: [
      '切换到 Discovery 子 Tab',
      '确认本机 Agent 出现在列表中',
      '如有其他 EdgeX 节点，确认也出现在列表',
      '离线 Agent 显示灰色状态'
    ],
    api: 'GET /api/capability/discovery/agents',
    expected: '返回 agents 数组，含 id/status/capabilities_count'
  },
  {
    id: 6,
    title: 'MCP 桥接验证',
    description: 'EAN Capability 通过 MCP Adapter 自动生成 MCP Tool',
    checklist: [
      '进入 AI 助手设置 → MCP 接入 Tab',
      '确认 MCP 服务状态为「就绪」',
      '使用 MCP 客户端（如 Claude Desktop）连接',
      '调用 ean_* 前缀的工具（如 ean_modbus_tcp_read_holding_register）',
      '确认返回结果与 Invoke Console 一致'
    ],
    api: 'POST /api/mcp (JSON-RPC tools/call)',
    expected: 'MCP 工具返回与 REST Invoke 相同的结果'
  }
]

const useCases = [
  {
    title: '场景一：Modbus 设备读取',
    tag: 'device',
    steps: [
      '配置 Modbus TCP 通道和设备点位',
      '在 Invoke Console 选择 modbus_tcp.read_holding_register',
      'Arguments: {"device_id":"slave-1","address":"40001","quantity":1}',
      '执行调用，验证返回值与 ScanEngine 实时数据一致'
    ]
  },
  {
    title: '场景二：跨 Agent 写入',
    tag: 'write',
    steps: [
      '在 Discovery 中确认目标 Agent 在线',
      '在 Invoke Console 选择 modbus_tcp.write_register',
      'Arguments: {"device_id":"slave-1","address":"40001","value":25.5}',
      '执行调用，确认 Event Monitor 出现 capability.invoked 事件',
      '在设备端验证寄存器值已更新'
    ]
  },
  {
    title: '场景三：AI 协议逆向',
    tag: 'ai',
    steps: [
      '在 Invoke Console 选择 ai.protocol_reverse',
      'Arguments: {"payload":"<PCAP摘要JSON>"}',
      '执行调用，等待 AI 返回候选点位',
      '在协议工作台 Tab 确认产出物',
      'Human Confirm 后导入 config.db'
    ]
  },
  {
    title: '场景四：EdgeOS 编排验证',
    tag: 'system',
    steps: [
      '确认 EdgeOS Workflow Center 已配置',
      'EdgeOS 通过 $edgeos/invoke 调用 EdgeX Capability',
      'EdgeX Invoke Dispatcher 路由到 Execution Mapper',
      '执行结果通过 $edgeos/reply 返回 EdgeOS',
      'EdgeOS Event Center 接收 EdgeX 发布的事件'
    ]
  }
]

const faqItems = [
  {
    q: 'Agent 状态显示「未连接」',
    a: '检查北向 edgeOS 通道是否已配置并连接。进入「节点同步」页面确认连接状态。若使用 MQTT，确认 Broker 地址和端口正确。'
  },
  {
    q: 'Capability 列表为空',
    a: '确认已配置至少一个采集通道和设备点位。Capability 由 Driver Commands 自动生成，无通道则无能力。点击「刷新」重新拉取。'
  },
  {
    q: 'Invoke 调用返回 failed',
    a: '检查 Arguments JSON 格式是否正确，device_id 和 address 是否存在。查看调用历史中的 error 字段获取详细错误信息。确认目标设备在线。'
  },
  {
    q: 'Event 列表无新事件',
    a: '确认 Event 自动发布已启用（AI 助手设置 → EAN 接入）。触发设备状态变化（如写入点位）后等待 5 秒刷新。检查 Event 是否被过滤器隐藏。'
  },
  {
    q: 'Discovery 无其他 Agent',
    a: 'EAN Discovery 依赖 EdgeOS 平台聚合。确认 EdgeOS 已连接且 Registry Center 正常运行。单个 EdgeX 节点只能看到自身。'
  },
  {
    q: 'MCP 工具未出现 ean_* 前缀',
    a: '确认 MCP 服务已激活全功能（AI 助手设置 → MCP 接入 → 全功能开关）。EAN Capability 自动映射为 MCP 工具需要 MCP 服务运行中。'
  }
]

// ── 可折叠 FAQ 状态 ──
const expandedFaq = ref(0)
const toggleFaq = (idx) => {
  expandedFaq.value = expandedFaq.value === idx ? -1 : idx
}

// ── 步骤控制 ──
const goToStep = (idx) => {
  if (idx >= 0 && idx < debugSteps.length) activeStep.value = idx
}
const nextStep = () => {
  if (activeStep.value < debugSteps.length - 1) activeStep.value++
}
const prevStep = () => {
  if (activeStep.value > 0) activeStep.value--
}

const currentStep = computed(() => debugSteps[activeStep.value])
const isFirst = computed(() => activeStep.value === 0)
const isLast = computed(() => activeStep.value === debugSteps.length - 1)

// ── 用例展开 ──
const expandedCase = ref(0)
const toggleCase = (idx) => {
  expandedCase.value = expandedCase.value === idx ? -1 : idx
}
</script>

<template>
  <div class="ai-dg">
    <!-- ── 联合调试流程 ── -->
    <section class="ai-dg-section">
      <div class="ai-dg-section__head">
        <span class="ai-dg-section__icon">
          <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <path d="M2 8h2l1.5-5 3 12 2-7 1.5 3h2" />
          </svg>
        </span>
        <h4 class="ai-dg-section__title">联合调试流程</h4>
        <span class="ai-dg-section__count">{{ activeStep + 1 }} / {{ debugSteps.length }}</span>
      </div>

      <!-- 步骤条 -->
      <div class="ai-dg-steps">
        <template v-for="(step, idx) in debugSteps" :key="step.id">
          <button
            class="ai-dg-step"
            :class="{
              'ai-dg-step--active': activeStep === idx,
              'ai-dg-step--done': activeStep > idx
            }"
            :title="step.title"
            @click="goToStep(idx)"
          >
            <span class="ai-dg-step__num">{{ idx + 1 }}</span>
            <span class="ai-dg-step__label">{{ step.title }}</span>
          </button>
          <span v-if="idx < debugSteps.length - 1" class="ai-dg-step__connector" :class="{ 'ai-dg-step__connector--done': activeStep > idx }"></span>
        </template>
      </div>

      <!-- 当前步骤详情 -->
      <div class="ai-dg-detail" v-if="currentStep">
        <div class="ai-dg-detail__head">
          <h5 class="ai-dg-detail__title">{{ currentStep.title }}</h5>
          <span class="ai-dg-detail__badge">步骤 {{ activeStep + 1 }}</span>
        </div>
        <p class="ai-dg-detail__desc">{{ currentStep.description }}</p>

        <div class="ai-dg-checklist">
          <div class="ai-dg-checklist__item" v-for="(item, ci) in currentStep.checklist" :key="ci">
            <span class="ai-dg-checklist__dot"></span>
            <span class="ai-dg-checklist__text">{{ item }}</span>
          </div>
        </div>

        <div class="ai-dg-codes">
          <div class="ai-dg-code">
            <span class="ai-dg-code__label">API</span>
            <code class="ai-dg-code__value">{{ currentStep.api }}</code>
          </div>
          <div class="ai-dg-code">
            <span class="ai-dg-code__label">期望</span>
            <code class="ai-dg-code__value ai-dg-code__value--expected">{{ currentStep.expected }}</code>
          </div>
        </div>

        <!-- 原生按钮：不依赖 Arco 弹出层，dialog 内 100% 可用 -->
        <div class="ai-dg-nav">
          <button
            class="ai-dg-btn ai-dg-btn--ghost"
            :disabled="isFirst"
            @click="prevStep"
          >
            <svg viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M8 2L4 6l4 4" /></svg>
            上一步
          </button>
          <button
            class="ai-dg-btn ai-dg-btn--primary"
            :disabled="isLast"
            @click="nextStep"
          >
            下一步
            <svg viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 2l4 4-4 4" /></svg>
          </button>
        </div>
      </div>
    </section>

    <!-- ── 指导用例 ── -->
    <section class="ai-dg-section">
      <div class="ai-dg-section__head">
        <span class="ai-dg-section__icon">
          <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <path d="M3 2.5h7l3 3v8a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V3.5a1 1 0 0 1 1-1z" />
            <path d="M9 2.5V5.5h3.5M5 8.5h6M5 11h4" />
          </svg>
        </span>
        <h4 class="ai-dg-section__title">指导用例</h4>
        <span class="ai-dg-section__count">{{ useCases.length }} 个场景</span>
      </div>

      <div class="ai-dg-cases">
        <div
          v-for="(uc, idx) in useCases"
          :key="uc.title"
          class="ai-dg-case"
          :class="{ 'ai-dg-case--open': expandedCase === idx }"
        >
          <button class="ai-dg-case__head" @click="toggleCase(idx)">
            <div class="ai-dg-case__left">
              <span class="ai-dg-case__tag" :class="`ai-dg-case__tag--${uc.tag}`">{{ uc.tag }}</span>
              <span class="ai-dg-case__title">{{ uc.title }}</span>
            </div>
            <span class="ai-dg-case__chevron">
              <svg viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 4.5L6 7.5L9 4.5" /></svg>
            </span>
          </button>
          <transition name="ai-dg-collapse">
            <div class="ai-dg-case__body" v-show="expandedCase === idx">
              <div class="ai-dg-case__step" v-for="(s, si) in uc.steps" :key="si">
                <span class="ai-dg-case__step-num">{{ si + 1 }}</span>
                <span class="ai-dg-case__step-text">{{ s }}</span>
              </div>
            </div>
          </transition>
        </div>
      </div>
    </section>

    <!-- ── 常见问题排查 ── -->
    <section class="ai-dg-section">
      <div class="ai-dg-section__head">
        <span class="ai-dg-section__icon">
          <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="8" cy="8" r="6.5" />
            <path d="M6.5 6.5a1.5 1.5 0 1 1 2.5 1.1c-.5.4-1 .7-1 1.4M8 11v.5" />
          </svg>
        </span>
        <h4 class="ai-dg-section__title">常见问题排查</h4>
        <span class="ai-dg-section__count">{{ faqItems.length }} 条</span>
      </div>

      <div class="ai-dg-faq">
        <div
          v-for="(item, idx) in faqItems"
          :key="idx"
          class="ai-dg-faq-item"
          :class="{ 'ai-dg-faq-item--open': expandedFaq === idx }"
        >
          <button class="ai-dg-faq-item__head" @click="toggleFaq(idx)">
            <span class="ai-dg-faq-item__q">{{ item.q }}</span>
            <span class="ai-dg-faq-item__chevron">
              <svg viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 4.5L6 7.5L9 4.5" /></svg>
            </span>
          </button>
          <transition name="ai-dg-collapse">
            <div class="ai-dg-faq-item__body" v-show="expandedFaq === idx">
              <p class="ai-dg-faq-item__a">{{ item.a }}</p>
            </div>
          </transition>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
/* ════════════════════════════════════════════
   AiEanDebugGuide — 纯原生元素，dialog 安全
   ════════════════════════════════════════════ */

.ai-dg {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding-bottom: 8px;
}

/* ── 通用 section ── */
.ai-dg-section {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.ai-dg-section__head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 2px;
}
.ai-dg-section__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 6px;
  color: var(--primary);
  background: color-mix(in srgb, var(--primary) 8%, transparent);
  flex-shrink: 0;
}
.ai-dg-section__icon svg {
  width: 13px;
  height: 13px;
}
.ai-dg-section__title {
  font-size: 13px;
  font-weight: 700;
  color: var(--ai-text, var(--text-primary));
  margin: 0;
  letter-spacing: 0.2px;
  flex: 1;
}
.ai-dg-section__count {
  font-size: 10.5px;
  color: var(--ai-text-faint, var(--text-tertiary));
  font-family: 'SF Mono', 'Cascadia Code', 'Consolas', monospace;
  padding: 2px 7px;
  border-radius: 4px;
  background: var(--ai-glass-bg-subtle, rgba(0, 0, 0, 0.04));
  font-weight: 600;
}

/* ── 步骤条 ── */
.ai-dg-steps {
  display: flex;
  align-items: flex-start;
  overflow-x: auto;
  scrollbar-width: none;
  padding: 4px 2px 6px;
}
.ai-dg-steps::-webkit-scrollbar { display: none; }

.ai-dg-step {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 5px;
  flex-shrink: 0;
  width: 56px;
  border: none;
  background: transparent;
  cursor: pointer;
  padding: 0;
  transition: all 200ms cubic-bezier(0.16, 1, 0.3, 1);
}
.ai-dg-step__num {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: 700;
  border: 2px solid var(--ai-glass-border, var(--border));
  color: var(--ai-text-faint, var(--text-tertiary));
  background: var(--ai-glass-bg, var(--bg));
  transition: all 300ms cubic-bezier(0.16, 1, 0.3, 1);
}
.ai-dg-step__label {
  font-size: 9.5px;
  color: var(--ai-text-faint, var(--text-tertiary));
  text-align: center;
  line-height: 1.2;
  max-width: 56px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition: color 200ms;
}
.ai-dg-step:hover .ai-dg-step__num {
  border-color: color-mix(in srgb, var(--primary) 50%, var(--border));
  color: var(--primary);
}
.ai-dg-step:hover .ai-dg-step__label {
  color: var(--ai-text-muted, var(--text-secondary));
}
.ai-dg-step--active .ai-dg-step__num {
  border-color: var(--primary);
  color: #fff;
  background: var(--primary);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--primary) 15%, transparent);
  transform: scale(1.15);
}
.ai-dg-step--active .ai-dg-step__label {
  color: var(--primary);
  font-weight: 600;
}
.ai-dg-step--done .ai-dg-step__num {
  border-color: #10b981;
  color: #10b981;
  background: rgba(16, 185, 129, 0.08);
}
.ai-dg-step--done .ai-dg-step__label {
  color: var(--ai-text-muted, var(--text-secondary));
}
.ai-dg-step__connector {
  width: 16px;
  height: 2px;
  background: var(--ai-glass-border, var(--border));
  margin-top: 11px;
  flex-shrink: 0;
  border-radius: 1px;
  transition: background 300ms;
}
.ai-dg-step__connector--done {
  background: #10b981;
}

/* ── 详情卡片 ── */
.ai-dg-detail {
  border: 1px solid var(--ai-glass-border, var(--border));
  border-radius: 12px;
  padding: 14px 16px;
  background: var(--ai-glass-bg-card, var(--bg));
  backdrop-filter: blur(20px);
  box-shadow:
    0 1px 3px rgba(0, 0, 0, 0.03),
    inset 0 1px 0 var(--ai-glass-highlight, transparent);
}
.ai-dg-detail__head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 4px;
}
.ai-dg-detail__title {
  font-size: 14px;
  font-weight: 700;
  color: var(--ai-text, var(--text-primary));
  margin: 0;
}
.ai-dg-detail__badge {
  font-size: 10px;
  color: var(--primary);
  background: color-mix(in srgb, var(--primary) 8%, transparent);
  padding: 2px 8px;
  border-radius: 4px;
  font-weight: 600;
  border: 1px solid color-mix(in srgb, var(--primary) 15%, transparent);
}
.ai-dg-detail__desc {
  font-size: 12px;
  color: var(--ai-text-muted, var(--text-secondary));
  margin: 4px 0 10px;
  line-height: 1.5;
}

/* ── checklist ── */
.ai-dg-checklist {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 12px;
}
.ai-dg-checklist__item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}
.ai-dg-checklist__dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--primary);
  margin-top: 7px;
  flex-shrink: 0;
  opacity: 0.5;
}
.ai-dg-checklist__text {
  font-size: 12px;
  color: var(--ai-text-muted, var(--text-secondary));
  line-height: 1.5;
}

/* ── API / 期望 code ── */
.ai-dg-codes {
  display: flex;
  flex-direction: column;
  gap: 5px;
  margin-bottom: 12px;
}
.ai-dg-code {
  display: flex;
  align-items: center;
  gap: 8px;
}
.ai-dg-code__label {
  font-size: 10px;
  color: var(--ai-text-faint, var(--text-tertiary));
  min-width: 32px;
  text-transform: uppercase;
  letter-spacing: 0.3px;
  font-weight: 700;
  flex-shrink: 0;
}
.ai-dg-code__value {
  font-size: 11px;
  font-family: 'SF Mono', 'Cascadia Code', 'Consolas', monospace;
  color: var(--primary);
  background: color-mix(in srgb, var(--primary) 6%, transparent);
  padding: 3px 8px;
  border-radius: 5px;
  border: 1px solid color-mix(in srgb, var(--primary) 12%, transparent);
  word-break: break-all;
  line-height: 1.5;
}
.ai-dg-code__value--expected {
  color: #10b981;
  background: color-mix(in srgb, #10b981 6%, transparent);
  border-color: color-mix(in srgb, #10b981 12%, transparent);
}

/* ── 原生导航按钮 ── */
.ai-dg-nav {
  display: flex;
  gap: 8px;
  margin-top: 4px;
  padding-top: 12px;
  border-top: 1px solid var(--ai-glass-border-subtle, var(--border));
}
.ai-dg-btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 6px 14px;
  border-radius: 8px;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: all 200ms cubic-bezier(0.16, 1, 0.3, 1);
  border: 1px solid transparent;
  position: relative;
  z-index: 1;
}
.ai-dg-btn svg {
  width: 11px;
  height: 11px;
}
.ai-dg-btn--primary {
  background: var(--primary);
  color: #fff;
  border-color: var(--primary);
  box-shadow: 0 2px 8px color-mix(in srgb, var(--primary) 25%, transparent);
}
.ai-dg-btn--primary:hover:not(:disabled) {
  filter: brightness(1.08);
  box-shadow: 0 4px 12px color-mix(in srgb, var(--primary) 35%, transparent);
  transform: translateY(-1px);
}
.ai-dg-btn--primary:active:not(:disabled) {
  transform: translateY(0);
  filter: brightness(0.95);
}
.ai-dg-btn--ghost {
  background: transparent;
  color: var(--ai-text-muted, var(--text-secondary));
  border-color: var(--ai-glass-border, var(--border));
}
.ai-dg-btn--ghost:hover:not(:disabled) {
  color: var(--primary);
  border-color: color-mix(in srgb, var(--primary) 40%, var(--border));
  background: color-mix(in srgb, var(--primary) 4%, transparent);
}
.ai-dg-btn--ghost:active:not(:disabled) {
  background: color-mix(in srgb, var(--primary) 8%, transparent);
}
.ai-dg-btn:disabled {
  opacity: 0.38;
  cursor: not-allowed;
  transform: none !important;
  box-shadow: none !important;
}
.ai-dg-btn:focus-visible {
  outline: 2px solid var(--primary);
  outline-offset: 2px;
}

/* ── 用例卡片 ── */
.ai-dg-cases {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.ai-dg-case {
  border: 1px solid var(--ai-glass-border-subtle, var(--border));
  border-radius: 10px;
  overflow: hidden;
  background: var(--ai-glass-bg-card, var(--bg));
  transition: border-color 200ms;
}
.ai-dg-case--open {
  border-color: color-mix(in srgb, var(--primary) 30%, var(--border));
}
.ai-dg-case__head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 9px 12px;
  border: none;
  background: transparent;
  cursor: pointer;
  width: 100%;
  text-align: left;
  transition: background 160ms;
}
.ai-dg-case__head:hover {
  background: var(--ai-glass-bg-subtle, rgba(0, 0, 0, 0.02));
}
.ai-dg-case__left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.ai-dg-case__tag {
  font-size: 9.5px;
  padding: 2px 6px;
  border-radius: 3px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.3px;
  flex-shrink: 0;
}
.ai-dg-case__tag--device { color: #3b82f6; background: rgba(59,130,246,0.1); }
.ai-dg-case__tag--write { color: #f59e0b; background: rgba(245,158,11,0.1); }
.ai-dg-case__tag--ai { color: #a78bfa; background: rgba(167,139,250,0.1); }
.ai-dg-case__tag--system { color: #64748b; background: rgba(100,116,139,0.1); }
.ai-dg-case__title {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--ai-text, var(--text-primary));
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ai-dg-case__chevron {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  color: var(--ai-text-faint, var(--text-tertiary));
  transition: transform 200ms cubic-bezier(0.16, 1, 0.3, 1);
  flex-shrink: 0;
}
.ai-dg-case__chevron svg { width: 12px; height: 12px; }
.ai-dg-case--open .ai-dg-case__chevron { transform: rotate(180deg); }
.ai-dg-case__body {
  padding: 4px 12px 12px;
  border-top: 1px solid var(--ai-glass-border-subtle, var(--border));
}
.ai-dg-case__step {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 4px 0;
}
.ai-dg-case__step-num {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  font-size: 10px;
  font-weight: 700;
  color: var(--primary);
  background: color-mix(in srgb, var(--primary) 8%, transparent);
  flex-shrink: 0;
  margin-top: 1px;
}
.ai-dg-case__step-text {
  font-size: 11.5px;
  color: var(--ai-text-muted, var(--text-secondary));
  line-height: 1.5;
}

/* ── FAQ ── */
.ai-dg-faq {
  display: flex;
  flex-direction: column;
  gap: 5px;
}
.ai-dg-faq-item {
  border: 1px solid var(--ai-glass-border-subtle, var(--border));
  border-radius: 8px;
  overflow: hidden;
  background: var(--ai-glass-bg-card, var(--bg));
  transition: border-color 200ms;
}
.ai-dg-faq-item--open {
  border-color: color-mix(in srgb, var(--primary) 25%, var(--border));
}
.ai-dg-faq-item__head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 12px;
  border: none;
  background: transparent;
  cursor: pointer;
  width: 100%;
  text-align: left;
  transition: background 160ms;
}
.ai-dg-faq-item__head:hover {
  background: var(--ai-glass-bg-subtle, rgba(0, 0, 0, 0.02));
}
.ai-dg-faq-item__q {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--ai-text, var(--text-primary));
  flex: 1;
  padding-right: 8px;
}
.ai-dg-faq-item__chevron {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  color: var(--ai-text-faint, var(--text-tertiary));
  transition: transform 200ms cubic-bezier(0.16, 1, 0.3, 1);
  flex-shrink: 0;
}
.ai-dg-faq-item__chevron svg { width: 12px; height: 12px; }
.ai-dg-faq-item--open .ai-dg-faq-item__chevron { transform: rotate(180deg); }
.ai-dg-faq-item__body {
  padding: 0 12px 10px;
  border-top: 1px solid var(--ai-glass-border-subtle, var(--border));
}
.ai-dg-faq-item__a {
  font-size: 11.5px;
  color: var(--ai-text-muted, var(--text-secondary));
  margin: 6px 0 0;
  line-height: 1.6;
}

/* ── 折叠过渡 ── */
.ai-dg-collapse-enter-active,
.ai-dg-collapse-leave-active {
  transition: all 200ms cubic-bezier(0.16, 1, 0.3, 1);
  overflow: hidden;
}
.ai-dg-collapse-enter-from,
.ai-dg-collapse-leave-to {
  opacity: 0;
  max-height: 0;
  padding-top: 0;
  padding-bottom: 0;
}
.ai-dg-collapse-enter-to,
.ai-dg-collapse-leave-from {
  opacity: 1;
  max-height: 500px;
}

/* ── 暗色主题 ── */
body.dark-theme .ai-dg-step__num {
  background: rgba(13, 18, 34, 0.8);
}
body.dark-theme .ai-dg-detail {
  background: rgba(13, 18, 34, 0.6);
  border-color: rgba(255, 255, 255, 0.08);
}
body.dark-theme .ai-dg-case {
  background: rgba(13, 18, 34, 0.4);
}
body.dark-theme .ai-dg-faq-item {
  background: rgba(13, 18, 34, 0.4);
}
body.dark-theme .ai-dg-btn--primary {
  box-shadow: 0 2px 8px color-mix(in srgb, var(--primary) 30%, transparent);
}
</style>
