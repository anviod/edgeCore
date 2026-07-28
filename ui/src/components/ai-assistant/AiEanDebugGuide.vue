<script setup>
import { ref } from 'vue'

const activeStep = ref(0)

const debugSteps = [
  {
    id: 0,
    title: '检查北向通道',
    icon: '1',
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
    icon: '2',
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
    icon: '3',
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
    icon: '4',
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
    icon: '5',
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
    icon: '6',
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
    icon: '7',
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
    steps: [
      '配置 Modbus TCP 通道和设备点位',
      '在 Invoke Console 选择 modbus_tcp.read_holding_register',
      'Arguments: {"device_id":"slave-1","address":"40001","quantity":1}',
      '执行调用，验证返回值与 ScanEngine 实时数据一致'
    ]
  },
  {
    title: '场景二：跨 Agent 写入',
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
    steps: [
      '确认 EdgeOS Workflow Center 已配置',
      'EdgeOS 通过 $edgeos/invoke 调用 EdgeX Capability',
      'EdgeX Invoke Dispatcher 路由到 Execution Mapper',
      '执行结果通过 $edgeos/reply 返回 EdgeOS',
      'EdgeOS Event Center 接收 EdgeX 发布的事件'
    ]
  }
]

const nextStep = () => {
  if (activeStep.value < debugSteps.length - 1) activeStep.value++
}
const prevStep = () => {
  if (activeStep.value > 0) activeStep.value--
}
</script>

<template>
  <div class="ai-debug-guide">
    <!-- 调试流程 -->
    <div class="ai-debug-section">
      <h4 class="ai-debug-section__title">联合调试流程</h4>
      <div class="ai-debug-flow">
        <div class="ai-debug-step" v-for="(step, idx) in debugSteps" :key="step.id"
          :class="{ active: activeStep === idx, done: activeStep > idx }">
          <div class="ai-debug-step__num">{{ step.icon }}</div>
          <div class="ai-debug-step__line" v-if="idx < debugSteps.length - 1"></div>
        </div>
      </div>

      <div class="ai-debug-detail" v-if="debugSteps[activeStep]">
        <div class="ai-debug-detail__header">
          <h5>{{ debugSteps[activeStep].title }}</h5>
          <span class="ai-debug-detail__step">{{ activeStep + 1 }} / {{ debugSteps.length }}</span>
        </div>
        <p class="ai-debug-detail__desc">{{ debugSteps[activeStep].description }}</p>
        <ul class="ai-debug-checklist">
          <li v-for="item in debugSteps[activeStep].checklist" :key="item">{{ item }}</li>
        </ul>
        <div class="ai-debug-api">
          <span class="label">API</span>
          <code>{{ debugSteps[activeStep].api }}</code>
        </div>
        <div class="ai-debug-expected">
          <span class="label">期望结果</span>
          <code>{{ debugSteps[activeStep].expected }}</code>
        </div>
        <div class="ai-debug-nav">
          <a-button size="small" :disabled="activeStep === 0" @click="prevStep">上一步</a-button>
          <a-button size="small" type="primary" :disabled="activeStep === debugSteps.length - 1" @click="nextStep">下一步</a-button>
        </div>
      </div>
    </div>

    <!-- 指导用例 -->
    <div class="ai-debug-section">
      <h4 class="ai-debug-section__title">指导用例</h4>
      <div class="ai-debug-cases">
        <div class="ai-debug-case" v-for="uc in useCases" :key="uc.title">
          <h6 class="ai-debug-case__title">{{ uc.title }}</h6>
          <ol class="ai-debug-case__steps">
            <li v-for="(s, i) in uc.steps" :key="i">{{ s }}</li>
          </ol>
        </div>
      </div>
    </div>

    <!-- 故障排查 -->
    <div class="ai-debug-section">
      <h4 class="ai-debug-section__title">常见问题排查</h4>
      <div class="ai-debug-faq">
        <div class="ai-debug-faq-item">
          <span class="q">Agent 状态显示「未连接」</span>
          <p class="a">检查北向 edgeOS 通道是否已配置并连接。进入「节点同步」页面确认连接状态。若使用 MQTT，确认 Broker 地址和端口正确。</p>
        </div>
        <div class="ai-debug-faq-item">
          <span class="q">Capability 列表为空</span>
          <p class="a">确认已配置至少一个采集通道和设备点位。Capability 由 Driver Commands 自动生成，无通道则无能力。点击「刷新」重新拉取。</p>
        </div>
        <div class="ai-debug-faq-item">
          <span class="q">Invoke 调用返回 failed</span>
          <p class="a">检查 Arguments JSON 格式是否正确，device_id 和 address 是否存在。查看调用历史中的 error 字段获取详细错误信息。确认目标设备在线。</p>
        </div>
        <div class="ai-debug-faq-item">
          <span class="q">Event 列表无新事件</span>
          <p class="a">确认 Event 自动发布已启用（AI 助手设置 → EAN 接入）。触发设备状态变化（如写入点位）后等待 5 秒刷新。检查 Event 是否被过滤器隐藏。</p>
        </div>
        <div class="ai-debug-faq-item">
          <span class="q">Discovery 无其他 Agent</span>
          <p class="a">EAN Discovery 依赖 EdgeOS 平台聚合。确认 EdgeOS 已连接且 Registry Center 正常运行。单个 EdgeX 节点只能看到自身。</p>
        </div>
        <div class="ai-debug-faq-item">
          <span class="q">MCP 工具未出现 ean_* 前缀</span>
          <p class="a">确认 MCP 服务已激活全功能（AI 助手设置 → MCP 接入 → 全功能开关）。EAN Capability 自动映射为 MCP 工具需要 MCP 服务运行中。</p>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ai-debug-guide { display: flex; flex-direction: column; gap: 20px; }
.ai-debug-section { display: flex; flex-direction: column; gap: 10px; }
.ai-debug-section__title { font-size: 14px; font-weight: 600; color: var(--ai-text, var(--text-primary)); margin: 0; }
.ai-debug-flow { display: flex; align-items: center; padding: 0 4px; }
.ai-debug-step { display: flex; align-items: center; flex-shrink: 0; }
.ai-debug-step__num {
  width: 28px; height: 28px; border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  font-size: 13px; font-weight: 600;
  border: 2px solid var(--ai-glass-border, var(--border));
  color: var(--ai-text-faint, var(--text-tertiary));
  background: var(--ai-glass-bg, var(--bg));
  transition: all 200ms cubic-bezier(0.16, 1, 0.3, 1);
}
.ai-debug-step__line { width: 24px; height: 2px; background: var(--ai-glass-border, var(--border)); }
.ai-debug-step.active .ai-debug-step__num {
  border-color: var(--primary); color: var(--primary);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--primary) 15%, transparent);
}
.ai-debug-step.done .ai-debug-step__num {
  border-color: #10b981; color: #10b981; background: rgba(16,185,129,0.1);
}
.ai-debug-step.done .ai-debug-step__line { background: #10b981; }
.ai-debug-detail {
  border: 1px solid var(--ai-glass-border, var(--border)); border-radius: 10px;
  padding: 14px 16px; background: var(--ai-glass-bg-card, var(--bg));
}
.ai-debug-detail__header { display: flex; justify-content: space-between; align-items: center; }
.ai-debug-detail__header h5 { font-size: 14px; font-weight: 600; color: var(--ai-text, var(--text-primary)); margin: 0; }
.ai-debug-detail__step { font-size: 11px; color: var(--ai-text-faint, var(--text-tertiary)); }
.ai-debug-detail__desc { font-size: 12px; color: var(--ai-text-muted, var(--text-secondary)); margin: 6px 0 8px; }
.ai-debug-checklist { list-style: none; padding: 0; margin: 0 0 10px; display: flex; flex-direction: column; gap: 4px; }
.ai-debug-checklist li { font-size: 12px; color: var(--ai-text-muted, var(--text-secondary)); padding-left: 16px; position: relative; }
.ai-debug-checklist li::before { content: '○'; position: absolute; left: 0; color: var(--primary); font-size: 11px; }
.ai-debug-api, .ai-debug-expected { display: flex; align-items: center; gap: 8px; margin: 4px 0; }
.ai-debug-api .label, .ai-debug-expected .label { font-size: 11px; color: var(--ai-text-faint, var(--text-tertiary)); min-width: 60px; }
.ai-debug-api code, .ai-debug-expected code { font-size: 11px; font-family: monospace; color: var(--primary); background: color-mix(in srgb, var(--primary) 8%, transparent); padding: 2px 8px; border-radius: 4px; }
.ai-debug-nav { display: flex; gap: 8px; margin-top: 12px; }
.ai-debug-cases { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
.ai-debug-case { border: 1px solid var(--ai-glass-border-subtle, var(--border)); border-radius: 8px; padding: 10px 12px; }
.ai-debug-case__title { font-size: 13px; font-weight: 600; color: var(--ai-text, var(--text-primary)); margin: 0 0 6px; }
.ai-debug-case__steps { padding-left: 18px; margin: 0; }
.ai-debug-case__steps li { font-size: 11px; color: var(--ai-text-muted, var(--text-secondary)); margin-bottom: 3px; line-height: 1.5; }
.ai-debug-faq { display: flex; flex-direction: column; gap: 8px; }
.ai-debug-faq-item { border: 1px solid var(--ai-glass-border-subtle, var(--border)); border-radius: 8px; padding: 8px 12px; }
.ai-debug-faq-item .q { font-size: 13px; font-weight: 500; color: var(--ai-text, var(--text-primary)); display: block; margin-bottom: 4px; }
.ai-debug-faq-item .a { font-size: 12px; color: var(--ai-text-muted, var(--text-secondary)); margin: 0; line-height: 1.6; }
body.dark-theme .ai-debug-step__num { background: rgba(13,18,34,0.8); }
body.dark-theme .ai-debug-detail { background: rgba(13,18,34,0.6); }
</style>
