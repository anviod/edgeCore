/**
 * ECharts 主题联动助手
 * 从 CSS 变量读取主题色，并在主题切换时自动更新所有已注册的 ECharts 实例。
 */
;(function () {
  var CSS_VARS = [
    '--accent', '--accent-2', '--accent-3',
    '--text-1', '--text-2', '--text-3',
    '--border',
    '--bg', '--surface',
    '--good', '--warn', '--bad'
  ];

  var registry = new Map();

  function readThemeColors() {
    var cs = getComputedStyle(document.documentElement);
    var colors = {};
    CSS_VARS.forEach(function (v) {
      colors[v] = (cs.getPropertyValue(v) || '').trim();
    });
    return colors;
  }

  function refreshAll() {
    var colors = readThemeColors();
    registry.forEach(function (fn, chart) {
      try { chart.setOption(fn(colors)); } catch (_) {}
    });
  }

  function register(chart, getColorOption) {
    registry.set(chart, getColorOption);
    // tooltip 挂载到 body，避免被 .card/.slide 的 overflow:hidden 裁剪
    try { chart.setOption({ tooltip: { appendToBody: true } }); } catch (_) {}
    // 立即应用一次当前主题色
    try { chart.setOption(getColorOption(readThemeColors())); } catch (_) {}
  }

  function unregister(chart) {
    registry.delete(chart);
  }

  // 等待 CSS 加载完成后刷新
  function onThemeChange(link) {
    var check = function () {
      try { if (link.sheet) { requestAnimationFrame(refreshAll); return; } } catch (_) {}
      setTimeout(function () { requestAnimationFrame(refreshAll); }, 50);
    };
    check();
  }

  // 监听 <link id="theme-link"> 的 href 变化
  var themeLink = document.getElementById('theme-link');
  if (themeLink) {
    new MutationObserver(function () {
      onThemeChange(themeLink);
    }).observe(themeLink, { attributes: true, attributeFilter: ['href'] });
  }

  window.__deckECharts = {
    readThemeColors: readThemeColors,
    register: register,
    unregister: unregister
  };
})();
