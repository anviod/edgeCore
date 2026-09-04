/**
 * edgeCore Paper — Nature-style Charts & Diagrams
 * 跨架构性能对比图表 + Mermaid 初始化
 */
(function () {
  'use strict';

  /* ── Mermaid 初始化（Nature 风格主题） ── */
  if (typeof mermaid !== 'undefined') {
    mermaid.initialize({
      startOnLoad: true,
      theme: 'base',
      themeVariables: {
        primaryColor: '#f8f7f5',
        primaryTextColor: '#1a1a1a',
        primaryBorderColor: '#c8102e',
        lineColor: '#6b6b6b',
        secondaryColor: '#f2f0ec',
        tertiaryColor: '#ffffff',
        fontFamily: "'WorkSans', 'Source Han Sans SC', 'Noto Sans CJK SC', 'Microsoft YaHei', sans-serif",
        fontSize: '13px'
      },
      flowchart: { curve: 'basis', padding: 12, nodeSpacing: 30, rankSpacing: 30, useMaxWidth: true },
      sequence: { actorMargin: 40, boxMargin: 6, noteMargin: 6, messageMargin: 30 },
      securityLevel: 'loose'
    });
  }

  /* ── ECharts 图表 ── */
  if (typeof echarts === 'undefined') return;

  var style = getComputedStyle(document.documentElement);
  var accent = style.getPropertyValue('--accent').trim() || '#c8102e';
  var accent2 = style.getPropertyValue('--accent2').trim() || '#1a5490';
  var ink = style.getPropertyValue('--ink').trim() || '#1a1a1a';
  var muted = style.getPropertyValue('--muted').trim() || '#6b6b6b';
  var rule = style.getPropertyValue('--rule').trim() || '#e5e2dd';
  var bg2 = style.getPropertyValue('--bg2').trim() || '#f8f7f5';

  /* 通用网格样式 */
  var baseGrid = { left: '3%', right: '5%', bottom: '8%', top: '18%', containLabel: true };
  var baseAxisLine = { lineStyle: { color: rule } };
  var baseAxisLabel = { color: muted, fontSize: 11, fontFamily: "'WorkSans', sans-serif" };
  var baseSplitLine = { lineStyle: { color: rule, type: 'dashed' } };

  /* ── 图 6：跨架构性能对比（双 Y 轴分组柱状图） ── */
  var chartCrossArch = document.getElementById('chart-cross-arch');
  if (chartCrossArch) {
    var chart1 = echarts.init(chartCrossArch, null, { renderer: 'svg' });
    chart1.setOption({
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        appendToBody: true,
        textStyle: { fontSize: 12, fontFamily: "'WorkSans', sans-serif" }
      },
      legend: {
        data: ['ARM64 RK3588s', 'x86 i5-13500H'],
        top: 0,
        right: '2%',
        textStyle: { color: ink, fontSize: 11, fontFamily: "'WorkSans', sans-serif" },
        itemWidth: 14,
        itemHeight: 10
      },
      grid: baseGrid,
      xAxis: {
        type: 'category',
        data: ['吞吐 (pts/s)', 'P95 延迟 (ms)', '最大延迟 (ms)'],
        axisLine: baseAxisLine,
        axisLabel: { color: ink, fontSize: 11, fontFamily: "'WorkSans', sans-serif", fontWeight: 600 },
        axisTick: { show: false }
      },
      yAxis: [
        {
          type: 'value',
          name: '吞吐 (pts/s)',
          nameTextStyle: { color: muted, fontSize: 10, fontFamily: "'WorkSans', sans-serif" },
          min: 8000,
          max: 10500,
          axisLine: { show: false },
          axisLabel: { color: muted, fontSize: 10, fontFamily: "'WorkSans', sans-serif" },
          splitLine: baseSplitLine
        },
        {
          type: 'value',
          name: '延迟 (ms)',
          nameTextStyle: { color: muted, fontSize: 10, fontFamily: "'WorkSans', sans-serif" },
          min: 0,
          max: 60,
          axisLine: { show: false },
          axisLabel: { color: muted, fontSize: 10, fontFamily: "'WorkSans', sans-serif" },
          splitLine: { show: false }
        }
      ],
      series: [
        {
          name: 'ARM64 RK3588s',
          type: 'bar',
          data: [
            { value: 9890, yAxisIndex: 0 },
            { value: 1.10, yAxisIndex: 1 },
            { value: 3.04, yAxisIndex: 1 }
          ],
          itemStyle: {
            color: accent,
            borderRadius: [3, 3, 0, 0]
          },
          barWidth: '28%',
          label: {
            show: true,
            position: 'top',
            color: ink,
            fontSize: 10,
            fontFamily: "'IBMPlexMono', monospace",
            formatter: function (p) {
              if (p.value > 100) return p.value.toLocaleString();
              return p.value.toFixed(2);
            }
          }
        },
        {
          name: 'x86 i5-13500H',
          type: 'bar',
          data: [
            { value: 8988, yAxisIndex: 0 },
            { value: 0.99, yAxisIndex: 1 },
            { value: 53.80, yAxisIndex: 1 }
          ],
          itemStyle: {
            color: accent2,
            borderRadius: [3, 3, 0, 0]
          },
          barWidth: '28%',
          label: {
            show: true,
            position: 'top',
            color: ink,
            fontSize: 10,
            fontFamily: "'IBMPlexMono', monospace",
            formatter: function (p) {
              if (p.value > 100) return p.value.toLocaleString();
              return p.value.toFixed(2);
            }
          }
        }
      ],
      animation: true,
      animationDuration: 800,
      animationEasing: 'cubicOut'
    });
    window.addEventListener('resize', function () { chart1.resize(); });
  }

  /* ── 图 5：协议驱动测试覆盖率（水平柱状图） ── */
  var chartCoverage = document.getElementById('chart-protocol-coverage');
  if (chartCoverage) {
    var chart2 = echarts.init(chartCoverage, null, { renderer: 'svg' });
    var protocols = ['EtherCAT', 'KNXnet/IP', 'DL/T645', 'Mitsubishi SLMP', 'Modbus', 'BACnet IP', 'ConnectionManager', 'SNMP', 'Siemens S7', 'EtherNet/IP', 'IEC 60870-5-104', 'Profinet IO', 'Omron FINS', 'OPC UA'];
    var coverages = [87.8, 77.2, 76.5, 70.7, 65.9, 66.1, 87.4, 63.7, 61.3, 62.2, 60.2, 55.9, 43.3, 47.9];
    chart2.setOption({
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        appendToBody: true,
        formatter: function (p) { return p[0].name + ': ' + p[0].value + '%'; },
        textStyle: { fontSize: 12, fontFamily: "'WorkSans', sans-serif" }
      },
      grid: { left: '3%', right: '8%', bottom: '3%', top: '3%', containLabel: true },
      xAxis: {
        type: 'value',
        max: 100,
        axisLine: { show: false },
        axisLabel: { color: muted, fontSize: 10, fontFamily: "'WorkSans', sans-serif", formatter: '{value}%' },
        splitLine: baseSplitLine
      },
      yAxis: {
        type: 'category',
        data: protocols,
        axisLine: baseAxisLine,
        axisTick: { show: false },
        axisLabel: { color: ink, fontSize: 10, fontFamily: "'WorkSans', sans-serif" }
      },
      series: [{
        type: 'bar',
        data: coverages,
        barWidth: '55%',
        itemStyle: {
          color: function (p) { return p.value >= 60 ? accent : accent2; },
          borderRadius: [0, 3, 3, 0]
        },
        label: {
          show: true,
          position: 'right',
          color: ink,
          fontSize: 9,
          fontFamily: "'IBMPlexMono', monospace",
          formatter: '{c}%'
        },
        markLine: {
          silent: true,
          symbol: 'none',
          lineStyle: { color: muted, type: 'dashed', width: 1 },
          data: [{ xAxis: 60, label: { show: true, formatter: '门禁 60%', color: muted, fontSize: 9, fontFamily: "'WorkSans', sans-serif" } }]
        }
      }],
      animation: true,
      animationDuration: 1000,
      animationDelay: function (idx) { return idx * 60; },
      animationEasing: 'cubicOut'
    });
    window.addEventListener('resize', function () { chart2.resize(); });
  }

  /* ── 图 7：ScanEngine 调度延迟分布（仪表盘风格） ── */
  var chartSchedGauge = document.getElementById('chart-sched-gauge');
  if (chartSchedGauge) {
    var chart3 = echarts.init(chartSchedGauge, null, { renderer: 'svg' });
    chart3.setOption({
      series: [
        {
          type: 'gauge',
          min: 0,
          max: 150,
          splitNumber: 5,
          radius: '85%',
          center: ['25%', '55%'],
          axisLine: { lineStyle: { width: 12, color: [[0.0104, accent], [0.467, accent2], [1, '#e5e2dd']] } },
          pointer: { width: 4, length: '65%', itemStyle: { color: ink } },
          axisTick: { length: 6, lineStyle: { color: muted } },
          splitLine: { length: 12, lineStyle: { color: muted, width: 2 } },
          axisLabel: { color: muted, fontSize: 9, fontFamily: "'IBMPlexMono', monospace", distance: 4 },
          detail: {
            valueAnimation: true,
            formatter: '{value} ms',
            color: ink,
            fontSize: 16,
            fontFamily: "'WorkSans', sans-serif",
            fontWeight: 700,
            offsetCenter: [0, '35%']
          },
          title: {
            offsetCenter: [0, '60%'],
            color: muted,
            fontSize: 10,
            fontFamily: "'WorkSans', sans-serif"
          },
          data: [{ value: 1.56, name: 'P95 调度延迟' }]
        },
        {
          type: 'gauge',
          min: 0,
          max: 150,
          splitNumber: 5,
          radius: '85%',
          center: ['75%', '55%'],
          axisLine: { lineStyle: { width: 12, color: [[0.456, accent], [0.93, accent2], [1, '#e5e2dd']] } },
          pointer: { width: 4, length: '65%', itemStyle: { color: ink } },
          axisTick: { length: 6, lineStyle: { color: muted } },
          splitLine: { length: 12, lineStyle: { color: muted, width: 2 } },
          axisLabel: { color: muted, fontSize: 9, fontFamily: "'IBMPlexMono', monospace", distance: 4 },
          detail: {
            valueAnimation: true,
            formatter: '{value} ms',
            color: ink,
            fontSize: 16,
            fontFamily: "'WorkSans', sans-serif",
            fontWeight: 700,
            offsetCenter: [0, '35%']
          },
          title: {
            offsetCenter: [0, '60%'],
            color: muted,
            fontSize: 10,
            fontFamily: "'WorkSans', sans-serif"
          },
          data: [{ value: 68.35, name: '最大延迟' }]
        }
      ],
      animation: true,
      animationDuration: 1200,
      animationEasing: 'cubicOut'
    });
    window.addEventListener('resize', function () { chart3.resize(); });
  }

  /* ── 图 8：ShadowCore 性能加速比对比 ── */
  var chartShadowBoost = document.getElementById('chart-shadow-boost');
  if (chartShadowBoost) {
    var chart4 = echarts.init(chartShadowBoost, null, { renderer: 'svg' });
    chart4.setOption({
      tooltip: {
        trigger: 'item',
        appendToBody: true,
        textStyle: { fontSize: 12, fontFamily: "'WorkSans', sans-serif" }
      },
      legend: {
        bottom: 0,
        textStyle: { color: ink, fontSize: 11, fontFamily: "'WorkSans', sans-serif" },
        itemWidth: 12,
        itemHeight: 10
      },
      grid: { left: '3%', right: '5%', bottom: '15%', top: '8%', containLabel: true },
      xAxis: {
        type: 'category',
        data: ['直接访问 PLC', 'ShadowCore 快照读取'],
        axisLine: baseAxisLine,
        axisTick: { show: false },
        axisLabel: { color: ink, fontSize: 11, fontFamily: "'WorkSans', sans-serif", fontWeight: 600 }
      },
      yAxis: {
        type: 'value',
        name: '响应时间 (ms)',
        nameTextStyle: { color: muted, fontSize: 10, fontFamily: "'WorkSans', sans-serif" },
        axisLine: { show: false },
        axisLabel: { color: muted, fontSize: 10, fontFamily: "'WorkSans', sans-serif" },
        splitLine: baseSplitLine
      },
      series: [
        {
          name: '响应时间范围',
          type: 'bar',
          data: [
            { value: 50, name: '直接访问 PLC（均值）', itemStyle: { color: accent2, borderRadius: [3, 3, 0, 0] } },
            { value: 5, name: 'ShadowCore 快照', itemStyle: { color: accent, borderRadius: [3, 3, 0, 0] } }
          ],
          barWidth: '35%',
          label: {
            show: true,
            position: 'top',
            color: ink,
            fontSize: 11,
            fontFamily: "'IBMPlexMono', monospace",
            fontWeight: 700,
            formatter: function (p) {
              if (p.dataIndex === 0) return '数十~数百 ms';
              return '< 5 ms';
            }
          },
          markLine: {
            silent: true,
            symbol: 'none',
            lineStyle: { color: accent, type: 'dashed', width: 1.5 },
            data: [
              {
                yAxis: 5,
                label: {
                  show: true,
                  formatter: '4.3× 加速',
                  color: accent,
                  fontSize: 10,
                  fontFamily: "'WorkSans', sans-serif",
                  fontWeight: 700,
                  position: 'end'
                }
              }
            ]
          }
        }
      ],
      animation: true,
      animationDuration: 800,
      animationEasing: 'cubicOut'
    });
    window.addEventListener('resize', function () { chart4.resize(); });
  }

})();
