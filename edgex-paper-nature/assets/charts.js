/**
 * charts.js — ECharts visualisations for the EdgeX Nature-style paper.
 *
 * Every chart:
 *   - reads colour tokens from CSS custom properties on :root
 *   - uses the SVG renderer (echarts.init(el, null, { renderer: 'svg' }))
 *   - disables animation (animation: false)
 *   - appends tooltip DOM to <body> (appendToBody: true)
 *   - shares a single debounced window resize listener
 *
 * The file is a self-contained IIFE — no globals are leaked.
 */
(function () {
  'use strict';

  /* ------------------------------------------------------------------
   * Boot helper — wait for DOM (and ECharts) before initialising.
   * ------------------------------------------------------------------ */
  function boot() {
    if (typeof echarts === 'undefined') {
      if (window.console) console.warn('[charts.js] ECharts not found — aborting.');
      return;
    }
    init();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }

  function init() {
    /* ----------------------------------------------------------------
     * 1. Read CSS custom properties from the page root
     * ---------------------------------------------------------------- */
    var style = getComputedStyle(document.documentElement);
    var accent  = style.getPropertyValue('--accent').trim()  || '#B31B1B';
    var accent2 = style.getPropertyValue('--accent2').trim() || '#1a4b8c';
    var ink     = style.getPropertyValue('--ink').trim()     || '#1a1a1a';
    var muted   = style.getPropertyValue('--muted').trim()   || '#6b6b6b';
    var rule    = style.getPropertyValue('--rule').trim()    || '#d4d4d0';
    var bg2     = style.getPropertyValue('--bg2').trim()     || '#f7f7f5';
    var sans    = style.getPropertyValue('--sans').trim()    || "'Helvetica Neue', Arial, sans-serif";

    /* ----------------------------------------------------------------
     * 2. Shared constants
     * ---------------------------------------------------------------- */
    var FZ       = 10;   /* axis & data-label font-size (px)  */
    var FZ_LEG   = 11;   /* legend font-size (px)             */
    var FZ_NAME  = 11;   /* axis-name font-size (px)          */

    /* ----------------------------------------------------------------
     * 3. Chart registry + unified resize
     * ---------------------------------------------------------------- */
    var charts = [];

    function makeChart(id, buildOption) {
      var el = document.getElementById(id);
      if (!el) return;
      var chart = echarts.init(el, null, { renderer: 'svg' });
      chart.setOption(buildOption());
      charts.push(chart);
    }

    var resizeTimer;
    window.addEventListener('resize', function () {
      clearTimeout(resizeTimer);
      resizeTimer = setTimeout(function () {
        charts.forEach(function (c) {
          if (c && !c.isDisposed()) c.resize();
        });
      }, 150);
    });

    /* ----------------------------------------------------------------
     * 4. Shared tooltip factory
     * ---------------------------------------------------------------- */
    function baseTooltip() {
      return {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        appendToBody: true,
        backgroundColor: bg2,
        borderColor: rule,
        borderWidth: 1,
        padding: [6, 10],
        textStyle: { color: ink, fontSize: FZ, fontFamily: sans },
        extraCssText: 'box-shadow:0 2px 8px rgba(0,0,0,.12);border-radius:2px;'
      };
    }

    /* ================================================================
     * Chart 1 — chart-perf-comparison
     * Cross-architecture performance bar chart.
     *   Series 1: ARM64 RK3588s  (accent)
     *   Series 2: x86 i5-13500H  (accent2)
     * ================================================================ */
    makeChart('chart-perf-comparison', function () {
      return {
        animation: false,
        grid: { top: 42, right: 28, bottom: 48, left: 68, containLabel: true },
        legend: {
          top: 4,
          itemWidth: 14,
          itemHeight: 10,
          textStyle: { color: ink, fontSize: FZ_LEG, fontFamily: sans }
        },
        tooltip: baseTooltip(),
        xAxis: {
          type: 'category',
          data: ['吞吐 (pts/s)', 'P95 延迟 (ms)', '最大延迟 (ms)', '内存漂移 (%)'],
          axisLine:  { lineStyle: { color: rule } },
          axisTick:  { lineStyle: { color: rule } },
          axisLabel: { color: ink, fontSize: FZ, fontFamily: sans, interval: 0 }
        },
        yAxis: {
          type: 'value',
          name: '数值',
          nameLocation: 'end',
          nameTextStyle: { color: muted, fontSize: FZ_NAME, fontFamily: sans },
          axisLine:  { show: false },
          axisTick:  { show: false },
          axisLabel: { color: ink, fontSize: FZ, fontFamily: sans },
          splitLine: { lineStyle: { color: rule, type: 'dashed' } }
        },
        series: [
          {
            name: 'ARM64 RK3588s',
            type: 'bar',
            data: [9890, 1.10, 3.04, -8.28],
            itemStyle: { color: accent },
            barGap: '20%',
            barCategoryGap: '45%'
          },
          {
            name: 'x86 i5-13500H',
            type: 'bar',
            data: [8988, 0.99, 53.80, -3.89],
            itemStyle: { color: accent2 }
          }
        ]
      };
    });

    /* ================================================================
     * Chart 2 — chart-protocol-coverage
     * Protocol driver test coverage (horizontal bars, top → bottom).
     *   Single series in accent colour.
     *   Vertical markLine at 70 % threshold.
     * ================================================================ */
    makeChart('chart-protocol-coverage', function () {
      var protocols = [
        'EtherCAT', 'ConnectionManager', 'KNXnet/IP', 'DL/T645',
        'Mitsubishi SLMP', 'BACnet IP', 'SNMP', 'Modbus TCP/RTU',
        'EtherNet/IP', 'Siemens S7', 'IEC 60870-5-104', 'Profinet IO',
        'OPC UA', 'Omron FINS'
      ];
      var coverage = [
        87.8, 87.4, 77.2, 76.5, 70.7, 66.1, 63.7,
        65.9, 62.2, 61.3, 60.2, 55.9, 47.9, 43.3
      ];

      var tip = baseTooltip();
      tip.formatter = function (params) {
        var p = params[0];
        return p.name + ': ' + p.value + '%';
      };

      return {
        animation: false,
        grid: { top: 16, right: 54, bottom: 44, left: 10, containLabel: true },
        tooltip: tip,
        xAxis: {
          type: 'value',
          name: '覆盖率 (%)',
          nameLocation: 'middle',
          nameGap: 26,
          nameTextStyle: { color: muted, fontSize: FZ_NAME, fontFamily: sans },
          min: 0,
          max: 100,
          axisLine:  { lineStyle: { color: rule } },
          axisTick:  { lineStyle: { color: rule } },
          axisLabel: { color: ink, fontSize: FZ, fontFamily: sans },
          splitLine: { lineStyle: { color: rule, type: 'dashed' } }
        },
        yAxis: {
          type: 'category',
          inverse: true,
          data: protocols,
          axisLine:  { lineStyle: { color: rule } },
          axisTick:  { show: false },
          axisLabel: { color: ink, fontSize: FZ, fontFamily: sans }
        },
        series: [{
          type: 'bar',
          data: coverage,
          itemStyle: { color: accent },
          barWidth: '62%',
          label: {
            show: true,
            position: 'right',
            color: ink,
            fontSize: FZ,
            fontFamily: sans,
            formatter: '{c}%'
          },
          markLine: {
            symbol: 'none',
            silent: true,
            label: {
              formatter: '70% 基准线',
              position: 'insideEndTop',
              color: ink,
              fontSize: FZ,
              fontFamily: sans
            },
            lineStyle: { color: ink, type: 'dashed', width: 1.5 },
            data: [{ xAxis: 70 }]
          }
        }]
      };
    });

    /* ================================================================
     * Chart 3 — chart-sla-gauge
     * ScanEngine SLA metrics vs. thresholds.
     *   Series 1: 实测值     (accent)
     *   Series 2: SLA 门限   (muted)
     * Mixed units → single linear axis, value labels on every bar.
     * ================================================================ */
    makeChart('chart-sla-gauge', function () {
      /* Format integers as-is, floats with 2 decimals. */
      function fmt(v) {
        return Number.isInteger(v) ? String(v) : v.toFixed(2);
      }

      var tip = baseTooltip();
      tip.formatter = function (params) {
        var s = params[0].name;
        params.forEach(function (p) {
          s += '<br/>' + p.marker + p.seriesName + ': ' + fmt(p.value);
        });
        return s;
      };

      var labelCfg = {
        show: true,
        position: 'top',
        color: ink,
        fontSize: FZ,
        fontFamily: sans,
        formatter: function (p) { return fmt(p.value); }
      };

      return {
        animation: false,
        grid: { top: 42, right: 28, bottom: 52, left: 58, containLabel: true },
        legend: {
          top: 4,
          itemWidth: 14,
          itemHeight: 10,
          textStyle: { color: ink, fontSize: FZ_LEG, fontFamily: sans }
        },
        tooltip: tip,
        xAxis: {
          type: 'category',
          data: ['P95 调度延迟', 'GC 最大暂停', '内存漂移', 'Miss Deadline'],
          axisLine:  { lineStyle: { color: rule } },
          axisTick:  { lineStyle: { color: rule } },
          axisLabel: { color: ink, fontSize: FZ, fontFamily: sans, interval: 0 }
        },
        yAxis: {
          type: 'value',
          axisLine:  { show: false },
          axisTick:  { show: false },
          axisLabel: { color: ink, fontSize: FZ, fontFamily: sans },
          splitLine: { lineStyle: { color: rule, type: 'dashed' } }
        },
        series: [
          {
            name: '实测值',
            type: 'bar',
            data: [1.56, 0.10, 0.00, 0],
            itemStyle: { color: accent },
            barGap: '20%',
            barCategoryGap: '45%',
            label: labelCfg
          },
          {
            name: 'SLA 门限',
            type: 'bar',
            data: [100, 20, 5, 0],
            itemStyle: { color: muted },
            label: labelCfg
          }
        ]
      };
    });
  }
})();
