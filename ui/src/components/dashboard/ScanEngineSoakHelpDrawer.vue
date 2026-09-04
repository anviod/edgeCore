<template>
  <a-drawer
    :visible="visible"
    :width="760"
    :footer="false"
    unmount-on-close
    class="help-drawer soak-help-drawer"
    render-to-body
    @update:visible="(v) => emit('update:visible', v)"
    @cancel="onCancel"
  >
    <template #title>
      <span class="help-drawer__title">ScanEngine Soak 监控说明</span>
    </template>

    <article class="help-doc">
      <header class="help-doc__hero">
        <span class="protocol-tag protocol-tag--accent">SLA / Soak</span>
        <p class="help-doc__lead">
          本面板每 15 秒采样一次 ScanEngine 运行指标，汇总 Release Gate 达标情况、当前快照、会话极值与趋势，用于 soak 测试与发布前验收。门禁判定标准详见
          <a href="/docs/RELEASE_GATE.html" target="_blank" class="help-doc-link">版本发布门禁</a>。
        </p>
      </header>

      <div class="help-doc__sections">
        <ChannelHelpBlock title="Release Gate（发布门禁）">
          <p class="help-doc-section__text">
            六项检查全部通过时显示「全部达标」。任一未达标则 Release Gate 不通过，需排查后再发布。
          </p>
          <a-collapse class="help-doc-faq" :bordered="false" expand-icon-position="right" :default-active-key="['running']">
            <a-collapse-item
              v-for="item in releaseGateItems"
              :key="item.key"
              :header="item.header"
            >
              <div class="help-doc-section__text">
                <p><strong>判定标准</strong>：{{ item.criteria }}</p>
                <p v-if="item.detail"><strong>说明</strong>：{{ item.detail }}</p>
                <p class="help-doc-example"><strong>示例</strong>：{{ item.example }}</p>
              </div>
            </a-collapse-item>
          </a-collapse>
        </ChannelHelpBlock>

        <ChannelHelpBlock title="当前快照">
          <p class="help-doc-section__text">最新一次采样的即时指标，反映 ScanEngine 此刻的运行状态。</p>
          <ChannelHelpParamList :items="snapshotItems" />
        </ChannelHelpBlock>

        <ChannelHelpBlock title="会话汇总">
          <p class="help-doc-section__text">
            自进程启动 Soak 监控以来，会话内各指标的极值与累计状态（非当前瞬时值）。
          </p>
          <ChannelHelpParamList :items="sessionSummaryItems" />
        </ChannelHelpBlock>

        <ChannelHelpBlock title="Soak 趋势">
          <p class="help-doc-section__text">
            会话内每次采样（15s 间隔）的历史序列，以迷你柱状图展示变化走势；右侧数字为最近一次采样值。
          </p>
          <ChannelHelpParamList :items="trendItems" />
        </ChannelHelpBlock>

        <ChannelHelpBlock id="soak-help-scan-class" title="Scan Class 明细">
          <p class="help-doc-section__text">
            按<strong>扫描间隔</strong>（如 100ms、1s、5s）分组展示各组内任务的运行状况，并非 fast / normal / slow 三类固定行。
            每台设备、每个 Scan Class 会注册独立扫描任务；不同设备若配置了不同采集周期，或 fast / normal / slow 映射到不同 Interval，表格就会出现多行——例如 11 种间隔表示当前共有 11 种不同的任务周期并存，属于正常现象。
          </p>
          <p class="help-doc-section__text">
            行背景色：绿色正常、黄色预警（积压或成功率偏低）、红色异常（有迟到）。
          </p>
          <ChannelHelpParamList :items="scanClassItems" />
          <p class="help-doc-example">
            <strong>总积压 vs 周期积压</strong>：快照中的「总积压」= 全局待执行队列 + 正在执行的任务数 + 串行队列深度，是引擎级汇总。
            表格中「积压」仅统计该周期内<strong>正在运行</strong>的任务数。例如 12 个任务、总积压 12、超出基线 0 → 积压稳定 PASS；
            若 5s 周期有 2 个任务正在执行，则该行积压 = 2，不代表全局只有 2 个待处理。
          </p>
        </ChannelHelpBlock>

        <ChannelHelpBlock title="运行时长">
          <ChannelHelpParamList :items="runtimeInfoItems" />
          <p class="help-doc-example">
            <strong>示例</strong>：「运行时长 2天3小时15分钟」表示 edgeCore 进程自启动以来已连续运行约 2 天 3 小时；仅分钟级时显示「45分钟」。面板每 30 秒刷新一次显示，Soak 指标仍按 15 秒间隔采样。
          </p>
        </ChannelHelpBlock>
      </div>
    </article>
  </a-drawer>
</template>

<script setup>
import { watch, nextTick } from 'vue'
import ChannelHelpBlock from '@/components/channel-help/ChannelHelpBlock.vue'
import ChannelHelpParamList from '@/components/channel-help/ChannelHelpParamList.vue'

const props = defineProps({
  visible: { type: Boolean, default: false },
  focusSection: { type: String, default: '' },
})

const emit = defineEmits(['update:visible', 'cancel'])

const onCancel = () => emit('update:visible', false)

watch(
  () => props.visible,
  async (open) => {
    if (!open || props.focusSection !== 'scan-class') return
    await nextTick()
    document.getElementById('soak-help-scan-class')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }
)

const releaseGateItems = [
  {
    key: 'running',
    header: 'ScanEngine 运行中',
    criteria: 'ScanEngine 处于运行状态（running=true）',
    detail: '引擎未启动时所有扫描任务停止调度，Release Gate 不通过。',
    example: 'running=true → PASS；服务刚重启、引擎尚未 Start → FAIL',
  },
  {
    key: 'cb',
    header: '断路器关闭',
    criteria: '当前打开的设备断路器 = 0，且会话内历史最大也为 0',
    detail: '设备连续通信失败后断路器会打开（Open），暂停对该设备的请求，防止故障扩散。',
    example: '3 台设备中 1 台断路器打开 → 当前 1 / 会话最大 1 → FAIL；全部关闭 → PASS',
  },
  {
    key: 'throttle',
    header: '无节流',
    criteria: '当前未节流，且会话内从未出现过节流',
    detail: '自适应降速因子 > 1.0 时触发节流，ScanEngine 会放慢扫描节奏以保护系统。会话内曾出现即 FAIL，即使当前已恢复。',
    example: 'factor=1.00、throttled=false → PASS；factor=1.50（节流 ×1.50）→ FAIL；曾节流现已恢复 → FAIL（会话内曾出现）',
  },
  {
    key: 'backlog',
    header: '积压稳定',
    criteria: '超出基线 ≤ 10，且会话内最大超出基线 ≤ 10',
    detail: '超出基线 = max(0, 总积压 − 任务数)。任务数可视为正常占用（每个任务至少占 1 个槽位），超出部分才是异常积压。详情中还显示串行队列深度。',
    example: '12 任务、total_backlog=12 → 超出 0 → PASS；12 任务、total_backlog=25 → 超出 13 → FAIL（阈值 ≤10）',
  },
  {
    key: 'late',
    header: 'Scan Class 无迟到',
    criteria: '所有扫描任务的 Scan Class 迟到合计 = 0',
    detail: '任务处于 Idle 且当前时间已超过 NextRun 计划执行时刻，计为 1 次迟到。',
    example: '3 个 1s 周期任务均按时调度 → 迟到 0 → PASS；2 个 5s 任务超时未跑 → 迟到 2 → FAIL',
  },
  {
    key: 'success',
    header: '点位成功率 ≥ 99%',
    criteria: '所有已启用通道中，最低点位请求成功率 ≥ 99%（0.99）',
    detail: '按通道维度统计采集请求成功/失败比，取最低值作为门禁依据；详情中会标注最低成功率对应的通道名称。',
    example: '通道 A 99.8%、通道 B 98.5% → 最低 98.5% → FAIL；全部 ≥ 99.0% → PASS',
  },
]

const snapshotItems = [
  {
    label: '任务数',
    desc: 'ScanEngine 已注册的扫描任务总数（各通道点位扫描任务之和）',
    example: 'task_count=48 → 48 个活跃扫描任务',
  },
  {
    label: '总积压',
    desc: '全局待执行队列 + 正在执行任务数 + 串行队列深度之和，反映引擎整体负载',
    example: 'global_queue=5 + active=3 + serial=4 → total_backlog=12',
  },
  {
    label: '断路器打开',
    desc: '当前处于 Open 状态的设备级断路器数量',
    example: '0 → 全部设备通信正常；2 → 2 台设备被断路保护',
  },
  {
    label: '节流状态',
    desc: '自适应降速因子是否 > 1.0；正常显示「正常」，节流时显示「节流 (×因子)」',
    example: '正常 → factor=1.00；节流 (×1.25) → 扫描周期被拉长 25%',
  },
  {
    label: '全局队列',
    desc: '待执行扫描任务队列深度 / 队列上限（默认 10000）',
    example: '128 / 10000 → 队列占用 1.3%，健康',
  },
  {
    label: 'Scan Class 迟到',
    desc: '所有周期内，已超过 NextRun 但仍 Idle 的任务总数',
    example: '0 → 调度准时；5 → 5 个任务排队超时',
  },
]

const sessionSummaryItems = [
  {
    label: '最大积压',
    desc: '会话内 total_backlog 的历史峰值',
    example: '采样序列 [10,12,15,11] → 最大积压=15',
  },
  {
    label: '最大断路器打开',
    desc: '会话内 circuit_breaker_open 的历史峰值',
    example: '全程为 0 → 无设备触发断路；曾出现 3 → 显示 3',
  },
  {
    label: '曾出现节流',
    desc: '会话内是否任意一次采样检测到 throttled=true',
    example: '否 → 全程无节流；是 → 曾降速，Release Gate「无节流」FAIL',
  },
  {
    label: '最低点位成功率',
    desc: '会话内各通道成功率的最小值（与 Release Gate 点位成功率同源）',
    example: '99.2% → PASS 余量；97.8%（Modbus 通道）→ FAIL',
  },
]

const trendItems = [
  {
    label: '总积压',
    desc: '每次采样的 total_backlog，观察负载是否持续攀升',
    example: '趋势 [10,10,11,12] → 缓慢增长，需关注',
  },
  {
    label: '断路器打开',
    desc: '每次采样的断路器打开数，应长期为 0',
    example: '[0,0,1,1,0] → 中间曾故障后恢复',
  },
  {
    label: '全局队列',
    desc: '每次采样的 pending 任务队列深度',
    example: '[50,48,52,50] → 队列稳定',
  },
  {
    label: 'Scan Class 迟到',
    desc: '每次采样的迟到任务合计，理想情况全程为 0',
    example: '[0,0,2,1,0] → 某时段调度滞后',
  },
]

const scanClassItems = [
  {
    label: '周期',
    desc: '扫描间隔标签，来自任务 Interval（如 1s、100ms）或 ScanClass 名称',
    example: '1s、5s、100ms',
  },
  {
    label: '任务',
    desc: '该周期下的扫描任务数量',
    example: '1s 周期 20 个任务、5s 周期 8 个任务',
  },
  {
    label: '积压',
    desc: '该周期内 Status=Running 的任务数（正在执行中的任务）',
    example: '1s 周期积压=2 → 2 个 1s 任务正在跑',
  },
  {
    label: '队列',
    desc: '该周期内所有任务计数（含 Idle / Running），表示该周期的任务规模',
    example: '与「任务」相同，按周期汇总',
  },
  {
    label: '迟到',
    desc: '该周期内 NextRun 已过但仍 Idle 的任务数；>0 时行标红',
    example: '5s 周期迟到=1 → 有一个 5s 任务未按时启动',
  },
  {
    label: '成功率',
    desc: '该周期内各任务 (1 − FailRate) 的算术平均值；≥99% 绿色，95–99% 黄色，<95% 红色',
    example: '1s 周期 99.5% → 健康；5s 周期 94.0% → 通信质量差',
  },
]

const runtimeInfoItems = [
  {
    label: '运行时长',
    desc: 'edgeCore 应用自进程启动以来的累计运行时间，按天/小时/分钟友好展示（省略为零的单位）',
    example: '2天3小时15分钟、3小时20分钟、45分钟',
  },
  {
    label: '数据来源',
    desc: '后端记录的服务启动时刻（runtime.start_time），前端据此本地计算并定期刷新',
    example: '服务重启后从 0 重新计时',
  },
]
</script>
