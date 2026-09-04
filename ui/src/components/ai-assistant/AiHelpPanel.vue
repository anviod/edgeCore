<template>
  <a-drawer
    v-model:visible="visible"
    title="AI 助手使用指南"
    :width="720"
    :footer="false"
    unmount-on-close
    render-to-body
    class="ai-help-drawer"
  >
    <div class="ai-help">
      <section class="ai-help-section">
        <div class="ai-help-section__head">
          <span class="ai-help-section__icon">✦</span>
          <h4 class="ai-help-section__title">快速开始</h4>
        </div>
        <p class="ai-help-text">
          AI 助手基于四阶段流水线工作：协议识别 → 配置生成 → 验证校验 → 导出确认。
          你可以通过左侧工作台选择任务类型，或在右侧聊天区直接用自然语言描述需求。
        </p>
        <div class="ai-help-steps">
          <div class="ai-help-step">
            <span class="ai-help-step__num">1</span>
            <span class="ai-help-step__text">选择工作台（协议逆向 / 生产配置 / 案例生成 / 诊断）</span>
          </div>
          <div class="ai-help-step">
            <span class="ai-help-step__num">2</span>
            <span class="ai-help-step__text">上传抓包文件、填写参数或与 AI 对话描述目标</span>
          </div>
          <div class="ai-help-step">
            <span class="ai-help-step__num">3</span>
            <span class="ai-help-step__text">查看 AI 生成的配置、校验结果与导出物</span>
          </div>
          <div class="ai-help-step">
            <span class="ai-help-step__num">4</span>
            <span class="ai-help-step__text">确认无误后一键导入系统</span>
          </div>
        </div>
      </section>

      <section class="ai-help-section">
        <div class="ai-help-section__head">
          <span class="ai-help-section__icon">⚡</span>
          <h4 class="ai-help-section__title">工作台说明</h4>
        </div>
        <div class="ai-help-cards">
          <div class="ai-help-card">
            <strong>协议逆向</strong>
            <span>上传工业协议抓包，自动识别寄存器、点位与数据类型。</span>
          </div>
          <div class="ai-help-card">
            <strong>生产配置</strong>
            <span>根据设备型号或描述批量生成通道、设备、点位配置。</span>
          </div>
          <div class="ai-help-card">
            <strong>案例生成</strong>
            <span>将历史配置沉淀为可复用的场景模板。</span>
          </div>
          <div class="ai-help-card">
            <strong>诊断助手</strong>
            <span>分析通道质量、设备离线原因并给出优化建议。</span>
          </div>
          <div class="ai-help-card">
            <strong>MCP 接入</strong>
            <span>将 edgeCore 能力以 MCP 工具形式提供给外部 LLM 客户端。</span>
          </div>
          <div class="ai-help-card">
            <strong>EAN 联合调试</strong>
            <span>验证 Agent、Capability、Event、Discovery 与 MCP 桥接链路。</span>
          </div>
        </div>
      </section>

      <section class="ai-help-section">
        <div class="ai-help-section__head">
          <span class="ai-help-section__icon">💡</span>
          <h4 class="ai-help-section__title">使用技巧</h4>
        </div>
        <ul class="ai-help-tips">
          <li>描述需求时尽量包含设备型号、协议类型、寄存器范围等关键信息</li>
          <li>上传抓包文件前可先用「诊断」确认通道通信质量</li>
          <li>AI 生成的配置务必在「验证」页校验后再导入</li>
          <li>点击面板右上角设置可切换本地模型与云端模型</li>
          <li>使用 <kbd>Esc</kbd> 快速收起面板，点击标题栏可拖拽位置</li>
        </ul>
      </section>
    </div>
  </a-drawer>
</template>

<script setup>
const visible = defineModel('visible', { type: Boolean, default: false })
</script>

<style scoped>
.ai-help {
  display: flex;
  flex-direction: column;
  gap: 24px;
  padding-bottom: 8px;
}

.ai-help-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.ai-help-section__head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.ai-help-section__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 6px;
  background: color-mix(in srgb, var(--primary) 10%, transparent);
  color: var(--primary);
  font-size: 13px;
}

.ai-help-section__title {
  margin: 0;
  font-size: 14px;
  font-weight: 700;
  color: var(--text-primary);
}

.ai-help-text {
  margin: 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--text-secondary);
}

.ai-help-steps {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.ai-help-step {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 12px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 10px;
}

.ai-help-step__num {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--primary);
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  flex-shrink: 0;
}

.ai-help-step__text {
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--text-secondary);
}

.ai-help-cards {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 10px;
}

.ai-help-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 10px;
  transition: all var(--motion-duration-fast) var(--motion-ease-standard);
}

.ai-help-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-md);
  border-color: color-mix(in srgb, var(--primary) 25%, var(--border));
}

.ai-help-card strong {
  font-size: 12.5px;
  color: var(--text-primary);
}

.ai-help-card span {
  font-size: 11.5px;
  line-height: 1.5;
  color: var(--text-secondary);
}

.ai-help-tips {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.ai-help-tips li {
  position: relative;
  padding: 10px 12px 10px 30px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--text-secondary);
}

.ai-help-tips li::before {
  content: '';
  position: absolute;
  left: 12px;
  top: 15px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--primary);
}

.ai-help-tips kbd {
  display: inline-block;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  font-family: ui-monospace, monospace;
  font-size: 11px;
  color: var(--text-primary);
}

@media (max-width: 640px) {
  .ai-help-cards {
    grid-template-columns: 1fr;
  }
}
</style>
