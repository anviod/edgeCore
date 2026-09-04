<template>
  <a-drawer
    :visible="visible"
    :width="760"
    :footer="false"
    unmount-on-close
    class="help-drawer"
    render-to-body
    @update:visible="(v) => emit('update:visible', v)"
    @cancel="onCancel"
  >
    <template #title>
      <span class="help-drawer__title">边缘计算规则帮助</span>
    </template>

    <article class="help-doc">
      <header class="help-doc__hero">
        <span class="protocol-tag protocol-tag--accent">边缘计算</span>
        <p class="help-doc__lead">
          规则类型、触发条件、动作链路与表达式语法说明，便于配置阈值报警、联动控制与数据计算。
        </p>
      </header>

      <div class="help-doc__sections">
        <ChannelHelpBlock title="基础概念">
          <HelpDocFlow :steps="basicConceptFlow" />
          <p class="help-doc-example">
            <strong>示例</strong>：温度点位别名 <code>t1</code>，每 5s 检查；条件 <code>t1 &gt; 80</code> 满足时发送 MQTT 告警。
          </p>
          <ChannelHelpParamList :items="basicConcepts" />
        </ChannelHelpBlock>

        <ChannelHelpBlock title="规则类型详解">
          <a-collapse
            class="help-doc-faq"
            :bordered="false"
            expand-icon-position="right"
            :default-active-key="['threshold']"
          >
            <a-collapse-item
              v-for="item in ruleTypes"
              :key="item.key"
              :header="item.header"
            >
              <div class="help-doc-section__text">
                <HelpDocFlow :steps="item.flow" />
                <p class="help-doc-example"><strong>示例</strong>：{{ item.example }}</p>
                <p><strong>说明</strong>：{{ item.intro }}</p>
                <ul>
                  <li v-for="(bullet, idx) in item.bullets" :key="idx">
                    <strong>{{ bullet.term }}</strong>：<template v-if="bullet.desc">{{ bullet.desc }}</template><code v-if="bullet.code">{{ bullet.code }}</code><template v-if="bullet.suffix">{{ bullet.suffix }}</template>
                  </li>
                </ul>
              </div>
            </a-collapse-item>
          </a-collapse>
        </ChannelHelpBlock>

        <ChannelHelpBlock title="常见场景最佳实践">
          <a-collapse class="help-doc-faq" :bordered="false" expand-icon-position="right">
            <a-collapse-item
              v-for="scene in practiceScenes"
              :key="scene.key"
              :header="scene.header"
            >
              <div class="help-doc-section__text">
                <HelpDocFlow :steps="scene.flow" />
                <p class="help-doc-example"><strong>示例</strong>：{{ scene.example }}</p>
                <p><strong>目标</strong>：{{ scene.goal }}</p>
                <ul>
                  <li v-for="(line, idx) in scene.lines" :key="idx">
                    <strong v-if="line.term">{{ line.term }}</strong><template v-if="line.term">：</template>{{ line.desc }}<code v-if="line.code">{{ line.code }}</code><template v-if="line.suffix">{{ line.suffix }}</template>
                  </li>
                </ul>
                <p v-if="scene.note"><strong>注意</strong>：{{ scene.note }}</p>
              </div>
            </a-collapse-item>
          </a-collapse>
        </ChannelHelpBlock>

        <ChannelHelpBlock title="表达式语法参考">
          <HelpDocFlow :steps="syntaxFlow" />
          <p class="help-doc-example">
            <strong>示例</strong>：条件 <code>bitget(v, 3) == 1</code> 表示状态字第 4 位为 1 时触发；计算 <code>t1 * 1.8 + 32</code> 将摄氏度转为华氏度。
          </p>
          <ChannelHelpParamList :items="syntaxItems" />
        </ChannelHelpBlock>

        <ChannelHelpBlock title="动作类型详解">
          <a-collapse class="help-doc-faq" :bordered="false" expand-icon-position="right">
            <a-collapse-item
              v-for="action in actionTypes"
              :key="action.key"
              :header="action.header"
            >
              <div class="help-doc-section__text">
                <HelpDocFlow :steps="action.flow" />
                <p class="help-doc-example"><strong>示例</strong>：{{ action.example }}</p>
                <template v-if="action.desc">{{ action.desc }}</template>
                <ul v-if="action.bullets?.length">
                  <li v-for="(bullet, idx) in action.bullets" :key="idx">
                    <strong>{{ bullet.term }}</strong>：<template v-if="bullet.desc">{{ bullet.desc }}</template><code v-if="bullet.code">{{ bullet.code }}</code>
                  </li>
                </ul>
              </div>
            </a-collapse-item>
          </a-collapse>
        </ChannelHelpBlock>

        <ChannelHelpBlock title="配置建议">
          <HelpDocFlow :steps="configAdviceFlow" />
          <p class="help-doc-example">
            <strong>示例</strong>：先配置 Threshold 越限告警并验证 MQTT 推送，确认无误后再叠加 Sequence 多步联动与 On Fail 回退。
          </p>
          <ol class="help-doc-steps">
            <li><strong>一规则一职责</strong>：每条规则只负责一个具体功能，便于维护和排查。</li>
            <li><strong>告警去重</strong>：越限报警建议使用「仅状态改变时触发」，避免重复推送。</li>
            <li><strong>防抖动</strong>：对波动较大的传感器，使用 State 类型或设置持续时间/连续次数。</li>
            <li><strong>复杂联动</strong>：多步骤设备启停使用 Sequence + Check + On Fail 实现安全回退。</li>
            <li><strong>性能优化</strong>：非紧急规则使用较长检查频率（如 30s、1m）；合并功能相似的规则。</li>
            <li><strong>定期维护</strong>：禁用或删除不再使用的规则；通过「记录与日志」排查异常。</li>
          </ol>
        </ChannelHelpBlock>
      </div>
    </article>
  </a-drawer>
</template>

<script setup>
import ChannelHelpBlock from '@/components/channel-help/ChannelHelpBlock.vue'
import ChannelHelpParamList from '@/components/channel-help/ChannelHelpParamList.vue'
import HelpDocFlow from '@/components/channel-help/HelpDocFlow.vue'

defineProps({
  visible: { type: Boolean, default: false },
})

const emit = defineEmits(['update:visible', 'cancel'])

const basicConceptFlow = [
  { type: 'node', text: '数据源 (Sources)' },
  { type: 'arrow' },
  { type: 'node', text: '按检查频率轮询', variant: 'muted' },
  { type: 'arrow' },
  { type: 'node', text: '评估触发条件', variant: 'cond' },
  { type: 'arrow' },
  { type: 'node', text: '执行动作链 (Actions)', variant: 'action' },
]

const basicConcepts = [
  {
    label: '数据源 (Sources)',
    desc: '规则的输入变量。为每个源设置简短别名（如 t1、p1），以便在表达式中引用。',
  },
  {
    label: '触发条件 (Condition)',
    desc: '返回 true/false 的布尔表达式。仅当条件满足时触发动作（Calculation 类型除外）。',
  },
  {
    label: '动作 (Actions)',
    desc: '规则触发后执行的一系列操作，可串联多个步骤。',
  },
  {
    label: '触发模式',
    desc: '「始终触发」每次检查满足条件即执行；「仅状态改变时触发」仅在 false→true 或 true→false 时执行，适合告警去重。',
  },
  {
    label: '检查频率',
    desc: '规则评估周期（如 1s、5s、1m）；频率越高响应越快，CPU 占用也越高。',
  },
  {
    label: '优先级',
    desc: '数值越大优先级越高；多条规则同时触发时按优先级排序执行。',
  },
]

const ruleTypes = [
  {
    key: 'threshold',
    header: 'Threshold (阈值触发)',
    intro: '当数据源数值满足布尔条件表达式时触发动作。最常用的规则类型。',
    example: '温度 t1 > 80 → 发送 MQTT 告警到 alarm/temp',
    flow: [
      { type: 'node', text: '读取数据源' },
      { type: 'arrow' },
      { type: 'node', text: '条件为 true？', variant: 'cond' },
      { type: 'arrow' },
      { type: 'node', text: '执行动作链', variant: 'action' },
    ],
    bullets: [
      { term: '适用场景', desc: '温度/压力越限报警、开关量状态检测、多点位组合逻辑判断。' },
      { term: '核心配置', desc: '数据源 + 触发条件（如 ', code: 't1 > 80', suffix: '）。' },
      { term: '支持运算', desc: '数值比较（>、<、≥、≤、==、!=）、逻辑组合（&&、||、!）、位操作（bitget、bitset、bitand、bitor）。' },
      { term: '可选防抖动', desc: '在「状态维持」中设置持续时间或连续次数，避免瞬时波动误触发。' },
    ],
  },
  {
    key: 'calculation',
    header: 'Calculation (计算公式)',
    intro: '对输入数据执行数学表达式计算，输出派生值。每次检查周期都会执行计算。',
    example: 't1 为摄氏温度 → 计算 t1 * 1.8 + 32 → MQTT 推送华氏温度',
    flow: [
      { type: 'node', text: '读取数据源' },
      { type: 'arrow' },
      { type: 'node', text: '执行计算公式', variant: 'cond' },
      { type: 'arrow' },
      { type: 'node', text: '通过动作输出结果', variant: 'action' },
    ],
    bullets: [
      { term: '适用场景', desc: '单位换算（℃→℉）、能耗折算、多传感器加权平均、数据预处理。' },
      { term: '核心配置', desc: '数据源 + 计算公式（如 ', code: 't1 * 1.8 + 32', suffix: '）。' },
      { term: '支持运算', desc: '四则运算（+、-、*、/、%、^）、函数调用、复杂嵌套表达式。' },
      { term: '注意', desc: '无触发条件字段；计算结果通过动作（如 MQTT 推送、数据库存储）输出。' },
    ],
  },
  {
    key: 'window',
    header: 'Window (时间/计数窗口)',
    intro: '在指定时间窗口或计数窗口内对数据进行聚合统计，再对聚合结果评估触发条件。',
    example: '最近 10s 温度平均值 avg > 50 → 记录 Warn 日志',
    flow: [
      { type: 'node', text: '持续采集数据点' },
      { type: 'arrow' },
      { type: 'node', text: '窗口聚合 avg/min/max…', variant: 'cond' },
      { type: 'arrow' },
      { type: 'node', text: '聚合值满足条件？', variant: 'cond' },
      { type: 'arrow' },
      { type: 'node', text: '执行动作链', variant: 'action' },
    ],
    bullets: [
      { term: '适用场景', desc: '滑动平均监控、峰值检测、流量速率统计、时段能耗汇总。' },
      { term: '窗口类型', desc: 'sliding（滑动窗口）/ tumbling（跳跃窗口）。' },
      { term: '窗口大小', desc: '时间格式如 ', code: '10s', suffix: '、5m，或计数格式如 100。' },
      { term: '聚合函数', desc: 'avg、min、max、sum、count、rate（变化率）。' },
      { term: '示例', desc: '窗口 avg > 50 表示最近 10 秒内平均值超过 50 时触发。' },
    ],
  },
  {
    key: 'state',
    header: 'State (状态持续)',
    intro: '当触发条件持续满足指定时间或连续次数后才触发动作，用于防抖动和持续异常检测。',
    example: 't1 > 80 连续 30s → 发送 Error 级日志与 MQTT 告警',
    flow: [
      { type: 'node', text: '读取数据源' },
      { type: 'arrow' },
      { type: 'node', text: '条件为 true？', variant: 'cond' },
      { type: 'arrow' },
      { type: 'node', text: '累计持续时间/次数', variant: 'muted' },
      { type: 'arrow' },
      { type: 'node', text: '达到阈值后触发', variant: 'action' },
    ],
    bullets: [
      { term: '适用场景', desc: '设备持续过热报警、振动异常持续检测、避免瞬时干扰触发。' },
      { term: '核心配置', desc: '触发条件 + 状态维持（持续时间 ', code: 'duration', suffix: ' 或连续次数 count）。' },
      { term: '持续时间', desc: '如 ', code: '30s', suffix: ' 表示条件需连续满足 30 秒才触发。' },
      { term: '连续次数', desc: '如 ', code: '5', suffix: ' 表示条件需连续 5 次检查均满足才触发。' },
      { term: '与 Threshold 区别', desc: 'Threshold 条件满足即触发（可配可选防抖）；State 以持续时间为核心语义。' },
    ],
  },
]

const practiceScenes = [
  {
    key: 'scene-a',
    header: '场景 A: 简单越限报警 (Threshold)',
    goal: '当温度 (t1) 超过 50 度时，记录日志并发送 MQTT 告警。',
    example: 't1 = 52.3 → 条件成立 → Log(Warn) + MQTT(alarm/temp)',
    flow: [
      { type: 'node', text: 't1 实时采集' },
      { type: 'arrow' },
      { type: 'node', text: 't1 > 50？', variant: 'cond' },
      { type: 'arrow' },
      { type: 'node', text: 'Log + MQTT 告警', variant: 'action' },
    ],
    lines: [
      { term: '类型', desc: 'Threshold' },
      { term: '数据源', desc: '添加温度点位，别名设为 ', code: 't1' },
      { term: '触发条件', code: 't1 > 50' },
      { term: '动作', desc: '① Log 级别 Warn「温度过高: ${t1}」② MQTT Topic「alarm/temp」' },
    ],
  },
  {
    key: 'scene-b',
    header: '场景 B: 顺序联动控制 (Sequence Workflow)',
    goal: '启动设备 A，等待 30 秒，确认 A 已启动后再启动设备 B；若 A 启动失败则回退关闭 A。',
    example: 'start_signal == 1 → 开 A → 等 30s → Check A → 开 B（失败则关 A）',
    flow: [
      { type: 'node', text: 'start_signal == 1', variant: 'cond' },
      { type: 'arrow' },
      { type: 'node', text: 'Device Control 开 A', variant: 'action' },
      { type: 'arrow' },
      { type: 'node', text: 'Delay 30s', variant: 'muted' },
      { type: 'arrow' },
      { type: 'node', text: 'Check A 状态 v==1', variant: 'cond' },
      { type: 'arrow' },
      { type: 'node', text: '成功开 B / 失败关 A', variant: 'action' },
    ],
    lines: [
      { term: '类型', desc: 'Threshold（或 State）' },
      { term: '触发条件', code: 'start_signal == 1' },
      { term: '动作', desc: 'Sequence → Device Control(A) → Delay 30s → Check(A) → Device Control(B)' },
    ],
    note: 'Sequence 中 Check 失败且未在 On Fail 中处理时，整个序列终止，后续步骤不会执行。',
  },
  {
    key: 'scene-c',
    header: '场景 C: 批量设备控制 (Batch Control)',
    goal: '一键关闭所有相关设备 (A, B, C)。',
    example: '手动触发 → 并行写入 A/B/C 开关点位 = 0',
    flow: [
      { type: 'node', text: '规则触发' },
      { type: 'arrow' },
      { type: 'node', text: 'Batch Control 并行下发', variant: 'action' },
      { type: 'arrow' },
      { type: 'node', text: '设备 A / B / C 同时关闭', variant: 'muted' },
    ],
    lines: [
      { term: '动作', desc: 'Device Control，开启 Batch Control' },
      { term: '目标列表', desc: '设备 A/B/C 开关点位，值均为 0' },
      { desc: '批量控制并行下发写入请求，响应速度优于连续单点控制。' },
    ],
  },
  {
    key: 'scene-d',
    header: '场景 D: 位运算与状态字控制 (Bitwise)',
    goal: '仅修改状态字的第 4 位（置 1），保持其他位不变。',
    example: '读当前值 v → bitset(v, 4) → 写回（RMW）',
    flow: [
      { type: 'node', text: '读取当前值 v' },
      { type: 'arrow' },
      { type: 'node', text: 'bitset(v, 4) 计算', variant: 'cond' },
      { type: 'arrow' },
      { type: 'node', text: '写入新值 (RMW)', variant: 'action' },
    ],
    lines: [
      { term: '动作', desc: 'Device Control' },
      { term: 'Expr', code: 'bitset(v, 4)', desc: ' 或 ', suffix: 'v | 8（0-based index）' },
      { term: '说明', desc: '系统自动读取当前值 → 计算新值 → 写入（Read-Modify-Write）。' },
      { term: 'RMW 机制', desc: '网关处理并发冲突，避免修改某位时覆盖其他位的同期变化。' },
    ],
  },
]

const syntaxFlow = [
  { type: 'node', text: '点位值 v / 别名 t1' },
  { type: 'arrow' },
  { type: 'node', text: '表达式运算 / 函数', variant: 'cond' },
  { type: 'arrow' },
  { type: 'node', text: '布尔条件 或 数值结果', variant: 'action' },
]

const syntaxItems = [
  { label: '当前点位值', example: 'v / value', desc: '当前触发点位的实时值' },
  { label: '数据源别名', example: 't1, p1', desc: '在规则中定义的 Sources 别名' },
  { label: '读取位', example: 'bitget(v, n)', desc: '获取第 n 位 (0/1)' },
  { label: '置位', example: 'bitset(v, n)', desc: '将第 n 位置 1' },
  { label: '清位', example: 'bitclr(v, n)', desc: '将第 n 位置 0' },
]

const actionTypes = [
  {
    key: 'action-log',
    header: 'Log (日志)',
    desc: '记录规则触发信息到系统日志。',
    example: '触发后写入 Warn 日志「温度过高: ${t1}」',
    flow: [
      { type: 'node', text: '规则触发' },
      { type: 'arrow' },
      { type: 'node', text: '渲染 Message 模板', variant: 'cond' },
      { type: 'arrow' },
      { type: 'node', text: '写入系统日志', variant: 'action' },
    ],
    bullets: [
      { term: 'Level', desc: '日志级别（Info/Warn/Error）' },
      { term: 'Message', desc: '支持 ', code: '${v}', suffix: ' 或 ${alias} 模板变量' },
    ],
  },
  {
    key: 'action-device',
    header: 'Device Control (设备控制)',
    desc: '向设备写入值。',
    example: '触发 → 计算 bitset(v,4) → 写入状态字点位',
    flow: [
      { type: 'node', text: '规则触发' },
      { type: 'arrow' },
      { type: 'node', text: '计算写入值 (可选 Expr)', variant: 'cond' },
      { type: 'arrow' },
      { type: 'node', text: '下发单点 / 批量写入', variant: 'action' },
    ],
    bullets: [
      { term: '单点模式', desc: '直接控制一个点位' },
      { term: '批量模式', desc: '同时控制多个点位，并行下发' },
      { term: 'Expression', desc: '可选，用于计算写入值（支持位操作 RMW）' },
    ],
  },
  {
    key: 'action-mqtt',
    header: 'MQTT Push (MQTT 推送)',
    example: 'Topic alarm/temp，Payload「温度异常: ${t1}」',
    flow: [
      { type: 'node', text: '规则触发' },
      { type: 'arrow' },
      { type: 'node', text: '组装 Topic / Payload', variant: 'cond' },
      { type: 'arrow' },
      { type: 'node', text: '北向 MQTT 通道发送', variant: 'action' },
    ],
    desc: '通过已配置的北向 MQTT 通道发送消息；Topic / Payload 支持 ${alias} 模板变量。',
  },
  {
    key: 'action-http',
    header: 'HTTP Push (HTTP 推送)',
    example: 'POST 北向 HTTP 接口，Body 携带 ${t1}、${p1} 实时值',
    flow: [
      { type: 'node', text: '规则触发' },
      { type: 'arrow' },
      { type: 'node', text: '构造请求体', variant: 'cond' },
      { type: 'arrow' },
      { type: 'node', text: '调用北向 HTTP 接口', variant: 'action' },
    ],
    desc: '调用已配置的北向 HTTP 接口上报数据或触发外部系统。',
  },
  {
    key: 'action-db',
    header: 'Database (存储)',
    example: '计算结果 avg_temp 写入本地库，供历史趋势查询',
    flow: [
      { type: 'node', text: '规则触发 / 计算完成' },
      { type: 'arrow' },
      { type: 'node', text: '整理字段与数值', variant: 'cond' },
      { type: 'arrow' },
      { type: 'node', text: '写入本地数据库', variant: 'action' },
    ],
    desc: '将规则计算结果或触发数据写入本地数据库，便于历史查询与分析。',
  },
  {
    key: 'action-seq',
    header: 'Sequence (顺序执行)',
    example: '开阀 → Delay 5s → Check 压力 → 开泵',
    flow: [
      { type: 'node', text: '规则触发' },
      { type: 'arrow' },
      { type: 'node', text: '步骤 1 → 2 → … → N', variant: 'cond' },
      { type: 'arrow' },
      { type: 'node', text: '任一步失败则终止', variant: 'muted' },
    ],
    desc: '严格按顺序执行子动作；任一步骤失败（如 Check 未处理）则整个序列终止。',
  },
  {
    key: 'action-delay',
    header: 'Delay (延时)',
    example: 'Sequence 中开设备 A 后 Delay 30s 再 Check 状态',
    flow: [
      { type: 'node', text: '前序步骤完成' },
      { type: 'arrow' },
      { type: 'node', text: '等待指定时长', variant: 'cond' },
      { type: 'arrow' },
      { type: 'node', text: '继续后续步骤', variant: 'action' },
    ],
    desc: '在 Sequence 中暂停指定时间后再执行后续步骤，常用于设备启动等待。',
  },
  {
    key: 'action-check',
    header: 'Check (校验)',
    example: '读 A 状态 v==1，重试 3 次；失败执行 On Fail 关 A',
    flow: [
      { type: 'node', text: '读取目标点位' },
      { type: 'arrow' },
      { type: 'node', text: '表达式校验 v==1', variant: 'cond' },
      { type: 'arrow' },
      { type: 'node', text: '通过 / On Fail 回退', variant: 'action' },
    ],
    bullets: [
      { term: 'Expression', desc: '校验公式（如 ', code: 'v == 1', suffix: '）' },
      { term: 'Retry', desc: '失败重试次数与间隔' },
      { term: 'On Fail', desc: '校验最终失败后执行的回退动作序列' },
    ],
  },
]

const configAdviceFlow = [
  { type: 'node', text: '规划规则职责' },
  { type: 'arrow' },
  { type: 'node', text: '配置并测试', variant: 'cond' },
  { type: 'arrow' },
  { type: 'node', text: '上线运行监控', variant: 'action' },
  { type: 'arrow' },
  { type: 'node', text: '定期维护优化', variant: 'muted' },
]

const onCancel = () => {
  emit('update:visible', false)
  emit('cancel')
}
</script>

<style scoped>
/* v3.0 — src/styles/help-drawer.css */
</style>
