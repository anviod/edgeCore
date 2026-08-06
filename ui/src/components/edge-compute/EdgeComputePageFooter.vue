<template>
  <section class="edge-compute-page-footer" aria-label="页面参考信息">
    <div class="edge-compute-footer-grid">
      <article class="edge-compute-footer-card">
        <h4 class="edge-compute-footer-card__title">
          <IconBulb class="edge-compute-footer-card__icon" />
          快速上手
        </h4>
        <ol class="edge-compute-footer-tip-list">
          <li>
            <strong>绑定数据源</strong> — 选择通道、设备与点位，为每个源设置别名（如 <code>t1</code>）
          </li>
          <li>
            <strong>编写触发条件</strong> — 使用表达式定义逻辑，如 <code>t1 &gt; 80 &amp;&amp; t2 &lt; 30</code>
          </li>
          <li>
            <strong>配置执行动作</strong> — 添加 MQTT 推送、设备控制或日志记录等动作链
          </li>
        </ol>
        <div class="edge-compute-footer-card__actions">
          <a-button type="outline" size="small" @click="emit('switch-tab', 'templates')">
            <template #icon><IconApps /></template>
            浏览场景模版
          </a-button>
          <a-button type="text" size="small" @click="emit('open-help')">
            <template #icon><IconQuestionCircle /></template>
            查看帮助说明
          </a-button>
        </div>
      </article>

      <article class="edge-compute-footer-card">
        <h4 class="edge-compute-footer-card__title">
          <IconThunderbolt class="edge-compute-footer-card__icon" />
          规则类型
        </h4>
        <ul class="edge-compute-footer-feature-list">
          <li v-for="item in ruleTypeHighlights" :key="item.key">
            <a-tag size="small" :color="item.color">{{ item.label }}</a-tag>
            <span>{{ item.desc }}</span>
          </li>
        </ul>
      </article>

      <article class="edge-compute-footer-card">
        <h4 class="edge-compute-footer-card__title">
          <IconBook class="edge-compute-footer-card__icon" />
          文档与参考
        </h4>
        <ul class="edge-compute-footer-doc-list">
          <li v-for="doc in docLinks" :key="doc.url">
            <a :href="doc.url" target="_blank" rel="noopener" class="edge-compute-footer-doc-link">
              {{ doc.label }}
            </a>
            <span class="edge-compute-footer-doc-desc">{{ doc.desc }}</span>
          </li>
        </ul>
      </article>
    </div>
  </section>
</template>

<script setup>
import {
  IconBulb, IconThunderbolt, IconBook, IconApps, IconQuestionCircle
} from '@arco-design/web-vue/es/icon'

const emit = defineEmits(['switch-tab', 'open-help'])

const ruleTypeHighlights = [
  { key: 'threshold', label: 'Threshold', color: 'red', desc: '数值越限报警，条件满足即触发' },
  { key: 'calculation', label: 'Calculation', color: 'arcoblue', desc: '实时计算公式，输出预处理结果' },
  { key: 'window', label: 'Window', color: 'orange', desc: '时间/计数窗口聚合，如滑动平均' },
  { key: 'state', label: 'State', color: 'green', desc: '状态持续检测，防抖动报警' },
]

const DOCS_BASE = 'https://anviod.github.io/edgeCore'

const docLinks = [
  { label: '边缘计算基础功能', url: `${DOCS_BASE}/edge/边缘计算基础功能.html`, desc: '规则引擎与数据流转说明' },
  { label: '边缘计算最佳实践', url: `${DOCS_BASE}/guide/EDGE_COMPUTING_BEST_PRACTICES.html`, desc: '场景编排与性能建议' },
  { label: '场景手册', url: `${DOCS_BASE}/edge/EDGE_COMPUTING_SCENARIO_MANUAL.html`, desc: '典型工业场景配置示例' },
  { label: 'API 参考', url: `${DOCS_BASE}/API/Edge_Computing_CN.html`, desc: '规则、指标与日志接口' },
]
</script>

<style scoped>
/* v3.0 — styles in src/styles/edge-compute.css */
</style>
