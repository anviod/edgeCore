// EdgeStark Paper Charts - Performance Visualization
// 聚焦 EdgeStark：数据汇聚延迟、多终端渲染帧率、物理引擎步进性能
(function() {
  var style = getComputedStyle(document.documentElement);
  var accent = style.getPropertyValue('--accent').trim();
  var accent2 = style.getPropertyValue('--accent2').trim();
  var ink = style.getPropertyValue('--ink').trim();
  var muted = style.getPropertyValue('--muted').trim();
  var rule = style.getPropertyValue('--rule').trim();
  var bg2 = style.getPropertyValue('--bg2').trim();

  // ===== Chart 1: End-to-End Data Aggregation Latency Distribution =====
  var chart1 = echarts.init(document.getElementById('chart-e2e-latency'), null, { renderer: 'svg' });
  chart1.setOption({
    animation: false,
    tooltip: { trigger: 'axis', appendToBody: true, formatter: function(p) { return p[0].name + ' ms<br/>占比: ' + p[0].value + '%'; } },
    grid: { left: '8%', right: '5%', top: '10%', bottom: '12%' },
    xAxis: {
      type: 'category',
      name: '延迟区间 (ms)',
      nameLocation: 'middle',
      nameGap: 35,
      data: ['40-60', '60-80', '80-100', '100-120', '120-140', '140-160', '160-180'],
      axisLine: { lineStyle: { color: rule } },
      axisLabel: { color: muted, fontSize: 11 }
    },
    yAxis: {
      type: 'value',
      name: '占比 (%)',
      axisLine: { lineStyle: { color: rule } },
      axisLabel: { color: muted, fontSize: 11 },
      splitLine: { lineStyle: { color: rule, type: 'dashed' } }
    },
    series: [{
      type: 'bar',
      data: [12.5, 28.3, 31.2, 15.8, 7.4, 3.6, 1.2],
      itemStyle: {
        color: function(params) {
          return params.dataIndex < 3 ? accent2 : accent;
        }
      },
      barWidth: '60%',
      markLine: {
        symbol: 'none',
        data: [
          { xAxis: 4, lineStyle: { color: accent, type: 'dashed', width: 1.5 }, label: { formatter: 'P95=142ms', color: accent, fontSize: 10 } },
          { xAxis: 5, lineStyle: { color: accent, type: 'dashed', width: 1.5 }, label: { formatter: 'P99=163ms', color: accent, fontSize: 10 } }
        ]
      }
    }]
  });
  window.addEventListener('resize', function() { chart1.resize(); });

  // ===== Chart 2: 3D Rendering FPS vs Scene Complexity (Desktop + VR) =====
  var chart2 = echarts.init(document.getElementById('chart-fps'), null, { renderer: 'svg' });
  chart2.setOption({
    animation: false,
    legend: { data: ['桌面端优化', 'VR 立体渲染', '未优化对照'], top: 'top', textStyle: { color: muted, fontSize: 11 } },
    tooltip: { trigger: 'axis', appendToBody: true, formatter: function(params) {
      var s = params[0].axisValue + ' 组件<br/>';
      params.forEach(function(p) { s += p.marker + p.seriesName + ': ' + p.value + ' FPS<br/>'; });
      return s;
    }},
    grid: { left: '8%', right: '5%', top: '15%', bottom: '12%' },
    xAxis: {
      type: 'category',
      name: '组件节点数量',
      nameLocation: 'middle',
      nameGap: 35,
      data: ['2K', '5K', '10K', '20K', '30K', '50K'],
      axisLine: { lineStyle: { color: rule } },
      axisLabel: { color: muted, fontSize: 11 }
    },
    yAxis: {
      type: 'value',
      name: '帧率 (FPS)',
      min: 0,
      max: 100,
      axisLine: { lineStyle: { color: rule } },
      axisLabel: { color: muted, fontSize: 11 },
      splitLine: { lineStyle: { color: rule, type: 'dashed' } }
    },
    series: [
      {
        name: '桌面端优化',
        type: 'line',
        data: [60, 60, 60, 58, 52, 45],
        smooth: true,
        symbol: 'circle',
        symbolSize: 7,
        lineStyle: { color: accent, width: 2.5 },
        itemStyle: { color: accent },
        areaStyle: { color: accent, opacity: 0.08 },
        markLine: {
          symbol: 'none',
          data: [{ yAxis: 50, lineStyle: { color: accent2, type: 'dotted', width: 1 }, label: { formatter: '桌面 SLA: 50FPS', color: accent2, fontSize: 10 } }]
        }
      },
      {
        name: 'VR 立体渲染',
        type: 'line',
        data: [90, 90, 72, 55, 40, 28],
        smooth: true,
        symbol: 'triangle',
        symbolSize: 7,
        lineStyle: { color: accent2, width: 2.5 },
        itemStyle: { color: accent2 },
        markLine: {
          symbol: 'none',
          data: [{ yAxis: 80, lineStyle: { color: accent, type: 'dotted', width: 1 }, label: { formatter: 'VR SLA: 80FPS', color: accent, fontSize: 10 } }]
        }
      },
      {
        name: '未优化对照',
        type: 'line',
        data: [55, 32, 18, 12, 8, 5],
        smooth: true,
        symbol: 'diamond',
        symbolSize: 6,
        lineStyle: { color: muted, width: 2, type: 'dashed' },
        itemStyle: { color: muted }
      }
    ]
  });
  window.addEventListener('resize', function() { chart2.resize(); });

  // ===== Chart 3: Physics Engine Stepping Performance =====
  var chart3 = echarts.init(document.getElementById('chart-physics'), null, { renderer: 'svg' });
  chart3.setOption({
    animation: false,
    tooltip: { trigger: 'axis', appendToBody: true, formatter: function(params) {
      var s = params[0].axisValue + ' 动态刚体<br/>';
      params.forEach(function(p) { s += p.marker + p.seriesName + ': ' + p.value + ' ms<br/>'; });
      return s;
    }},
    grid: { left: '10%', right: '5%', top: '12%', bottom: '12%' },
    xAxis: {
      type: 'category',
      name: '动态刚体数量',
      nameLocation: 'middle',
      nameGap: 35,
      data: ['500', '1000', '2000', '3000', '5000', '8000', '10000'],
      axisLine: { lineStyle: { color: rule } },
      axisLabel: { color: muted, fontSize: 11 }
    },
    yAxis: {
      type: 'value',
      name: '单次步进耗时 (ms)',
      min: 0,
      max: 10,
      axisLine: { lineStyle: { color: rule } },
      axisLabel: { color: muted, fontSize: 11 },
      splitLine: { lineStyle: { color: rule, type: 'dashed' } }
    },
    series: [
      {
        name: 'Rapier WASM 步进耗时',
        type: 'line',
        data: [0.4, 0.8, 1.5, 2.1, 3.2, 4.8, 6.1],
        smooth: true,
        symbol: 'circle',
        symbolSize: 7,
        lineStyle: { color: accent, width: 2.5 },
        itemStyle: { color: accent },
        areaStyle: { color: accent, opacity: 0.08 }
      },
      {
        name: '120Hz 帧预算上限',
        type: 'line',
        data: [8.33, 8.33, 8.33, 8.33, 8.33, 8.33, 8.33],
        symbol: 'none',
        lineStyle: { color: accent2, width: 2, type: 'dotted' },
        itemStyle: { color: accent2 }
      }
    ],
    legend: {
      data: ['Rapier WASM 步进耗时', '120Hz 帧预算上限'],
      top: 'top',
      textStyle: { color: muted, fontSize: 11 }
    }
  });
  window.addEventListener('resize', function() { chart3.resize(); });
})();
