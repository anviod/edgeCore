/*
 * edgeCore v3.0 — MCP 接入完整文档渲染器
 * 解析 /api/mcp/help 返回的工具描述（中英混合大段文本），
 * 提炼为「名称 / 描述 / 参数 / 返回」的结构化排版。
 */

export function esc(s) {
  if (s === null || s === undefined) return ''
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

export function summarizeText(text, maxLen = 120) {
  if (!text) return ''
  if (text.length <= maxLen) return text
  const stopChars = ['。', '！', '？', '；', '. ', '\n', '; ']
  for (const ch of stopChars) {
    const idx = text.indexOf(ch, Math.min(maxLen * 0.6, maxLen - 20))
    if (idx > 0 && idx < maxLen) return text.slice(0, idx + ch.length)
  }
  return text.slice(0, maxLen) + '...'
}

// 解析 "name(type, required 描述), name(type, optional 描述)" 形式的参数/返回片段
function parseNamedList(text) {
  if (!text) return []
  const items = []
  const re = /([a-zA-Z_][a-zA-Z0-9_]*)\s*\(\s*([^)]*)\)/g
  let m
  while ((m = re.exec(text)) !== null) {
    const name = m[1]
    const meta = m[2].trim()
    const typeMatch = meta.match(/^([a-zA-Z\[\]<>]+)\s*,?\s*(.*)$/)
    const type = typeMatch ? typeMatch[1].trim() : meta
    const note = typeMatch ? typeMatch[2].trim() : ''
    items.push({ name, type, note })
  }
  return items
}

// 提炼工具描述文本为结构化字段：nameZh / nameEn / summary / params[] / returns[]
export function parseToolDescription(raw) {
  const out = { nameZh: '', nameEn: '', summary: '', params: [], returns: [] }
  let text = String(raw || '')
    .trim()
    .replace(/^\[EAN Capability\]\s*/i, '')
  if (!text) return out

  // 1) 提取双语名称："中文 | English."
  let rest = text
  const nameMatch = text.match(/^([^|\n]{1,48}?)\s*\|\s*([^。]{1,120}?)\.\s/)
  if (nameMatch) {
    out.nameZh = nameMatch[1].replace(/[.。]\s*$/, '').trim()
    out.nameEn = nameMatch[2].trim()
    rest = text.slice(nameMatch[0].length)
  }

  // 2) 按「参数 Params:」「返回 Returns:」标记切分
  const pm = rest.search(/参数\s*Params\s*:/i)
  const rm = rest.search(/返回\s*Returns\s*:/i)
  let desc = rest
  let paramsText = ''
  let returnsText = ''
  if (pm >= 0) {
    desc = rest.slice(0, pm)
    paramsText = rest.slice(pm, rm > pm ? rm : rest.length)
    if (rm > pm) returnsText = rest.slice(rm)
  } else if (rm >= 0) {
    desc = rest.slice(0, rm)
    returnsText = rest.slice(rm)
  }

  out.summary = desc.replace(/\s+/g, ' ').trim()
  out.params = parseNamedList(paramsText.replace(/^参数\s*Params\s*:/i, ''))
  out.returns = parseNamedList(returnsText.replace(/^返回\s*Returns\s*:/i, ''))
  return out
}

function cleanNote(note) {
  return note
    .replace(/\b(?:required|optional|必需|可选)\s*[:\-－]?\s*/gi, '')
    .trim()
}

function renderParams(items) {
  const list = items
    .map((p) => {
      const req = /required|必需/i.test(p.note)
        ? '<span class="ai-mcp-docs-tool-required">*</span>'
        : ''
      const opt = /optional|可选/i.test(p.note)
        ? ' <span class="ai-mcp-docs-tool-type ai-mcp-docs-tool-type--opt">可选</span>'
        : ''
      const note = cleanNote(p.note)
      return `<li>
        <div class="ai-mcp-docs-tool-param__name">
          <code>${esc(p.name)}</code>${req} <code class="ai-mcp-docs-tool-type">${esc(p.type)}</code>${opt}
        </div>
        ${note ? `<div class="ai-mcp-docs-tool-param__desc">${esc(note)}</div>` : ''}
      </li>`
    })
    .join('')
  return `<div class="ai-mcp-docs-tool-section"><h5>参数</h5><ul class="ai-mcp-docs-tool-list">${list}</ul></div>`
}

function renderReturns(items) {
  const list = items
    .map((p) => {
      const note = cleanNote(p.note)
      return `<li>
        <div class="ai-mcp-docs-tool-param__name">
          <code>${esc(p.name)}</code> <code class="ai-mcp-docs-tool-type">${esc(p.type)}</code>
        </div>
        ${note ? `<div class="ai-mcp-docs-tool-param__desc">${esc(note)}</div>` : ''}
      </li>`
    })
    .join('')
  return `<div class="ai-mcp-docs-tool-section"><h5>返回</h5><ul class="ai-mcp-docs-tool-list">${list}</ul></div>`
}

export function renderToolCard(t) {
  const parsed = parseToolDescription(t.description)
  const hasStructure = parsed.params.length || parsed.returns.length
  const title =
    parsed.nameZh && parsed.nameEn
      ? `${esc(parsed.nameZh)}<span class="ai-mcp-docs-tool-card__sep">·</span>${esc(parsed.nameEn)}`
      : parsed.nameZh
        ? esc(parsed.nameZh)
        : ''

  const summarySrc = parsed.summary || t.description || ''
  const summary = esc(summarizeText(summarySrc, 110))
  const hasLongDesc = (summarySrc || '').length > 110

  // 简短且无结构 → 极简卡片
  if (!hasStructure && !hasLongDesc && !title) {
    return `<div class="ai-mcp-docs-tool-card">
      <code class="ai-mcp-docs-tool-card__name">${esc(t.name)}</code>
      <p class="ai-mcp-docs-tool-card__desc">${summary}</p>
    </div>`
  }

  let body = ''
  if (parsed.summary) {
    body += `<p class="ai-mcp-docs-tool-card__desc">${esc(parsed.summary)}</p>`
  } else if (t.description) {
    body += `<p class="ai-mcp-docs-tool-card__desc">${esc(summarizeText(t.description, 400))}</p>`
  }
  if (parsed.params.length) body += renderParams(parsed.params)
  if (parsed.returns.length) body += renderReturns(parsed.returns)

  const titleHtml = title
    ? `<div class="ai-mcp-docs-tool-card__title">${title}</div>`
    : ''

  return `<details class="ai-mcp-docs-tool-card">
    <summary>
      <code class="ai-mcp-docs-tool-card__name">${esc(t.name)}</code>
      <span class="ai-mcp-docs-tool-summary">${summary}</span>
      <span class="ai-mcp-docs-tool-toggle"><span class="show-when-closed">展开</span><span class="show-when-open">收起</span></span>
    </summary>
    <div class="ai-mcp-docs-tool-body">
      ${titleHtml}
      ${body}
    </div>
  </details>`
}

export function renderHelpDoc(data) {
  if (!data) return '<p class="ai-mcp-docs-error">无数据</p>'

  let html = ''

  html += `<header class="ai-mcp-docs-hero">
    <h2 class="ai-mcp-docs-hero__title">${esc(data.title || '')}</h2>
    <p class="ai-mcp-docs-hero__desc">${esc(data.description || '')}</p>
  </header>`

  // 架构流程
  if (data.architecture?.layers?.length) {
    const colorMap = { purple: '#8b5cf6', blue: '#3b82f6', green: '#22c55e', orange: '#f59e0b' }
    const bgMap = {
      purple: 'rgba(139,92,246,0.12)',
      blue: 'rgba(59,130,246,0.12)',
      green: 'rgba(34,197,94,0.12)',
      orange: 'rgba(245,158,11,0.12)',
    }
    const nodes = data.architecture.layers
      .map((l, i) => {
        const c = colorMap[l.color] || '#6b7280'
        const bg = bgMap[l.color] || 'rgba(107,114,128,0.12)'
        const node = `<div class="ai-mcp-docs-arch__node" style="border-color:${c};background:${bg}">
          <strong>${esc(l.name)}</strong><br><small>${esc(l.desc)}</small>
        </div>`
        const arrow = i < data.architecture.layers.length - 1
          ? '<div class="ai-mcp-docs-arch__arrow">&#x2193;</div>'
          : ''
        return node + arrow
      })
      .join('')
    html += `<section class="ai-mcp-docs-section">
      <h3 class="ai-mcp-docs-section__title">系统架构</h3>
      <div class="ai-mcp-docs-arch">${nodes}</div>
    </section>`
  }

  // 传输协议
  html += `<section class="ai-mcp-docs-section">
    <h3 class="ai-mcp-docs-section__title">传输协议</h3>
    <div class="ai-mcp-docs-grid">
      <div class="ai-mcp-docs-grid__item"><span class="ai-mcp-docs-grid__label">传输方式</span><code>${esc(data.transport || '')}</code></div>
      <div class="ai-mcp-docs-grid__item"><span class="ai-mcp-docs-grid__label">端点</span><code>${esc(data.endpoint || '')}</code></div>
      <div class="ai-mcp-docs-grid__item"><span class="ai-mcp-docs-grid__label">认证方式</span><code>${esc(data.auth_mode || '')}</code></div>
    </div>
  </section>`

  // 客户端配置
  if (data.clients?.length) {
    html += `<section class="ai-mcp-docs-section">
      <h3 class="ai-mcp-docs-section__title">客户端接入配置</h3>
      <p class="ai-mcp-docs-section__text">将以下 JSON 配置添加到对应 MCP 客户端的配置文件中，替换 <code>&lt;host&gt;</code> 和 <code>&lt;mcp_api_key&gt;</code>。</p>`
    for (const c of data.clients) {
      html += `<div class="ai-mcp-docs-card">
        <h4>${esc(c.name)}</h4>
        <pre class="ai-mcp-docs-code"><code>${esc(c.config)}</code></pre>
      </div>`
    }
    html += `</section>`
  }

  // 工具清单
  if (data.tools?.length) {
    const readTools = data.tools.filter((t) => t.category === 'read')
    const writeTools = data.tools.filter((t) => t.category === 'write')
    html += `<section class="ai-mcp-docs-section">
      <h3 class="ai-mcp-docs-section__title">MCP 工具清单 (${data.tools.length} 个)</h3>`
    html += `<h4 class="ai-mcp-docs-subtitle">
      <span class="ai-mcp-docs-dot" style="background:#22c55e"></span> 只读查询 (${readTools.length} 个)
      <span class="ai-mcp-docs-subtitle__hint">无需全功能激活，默认可用</span>
    </h4>`
    html += '<div class="ai-mcp-docs-tool-grid">'
    for (const t of readTools) html += renderToolCard(t)
    html += '</div>'
    html += `<h4 class="ai-mcp-docs-subtitle">
      <span class="ai-mcp-docs-dot" style="background:#f59e0b"></span> 全功能 CRUD (${writeTools.length} 个)
      <span class="ai-mcp-docs-subtitle__hint">需激活全功能读写</span>
    </h4>`
    html += '<div class="ai-mcp-docs-tool-grid">'
    for (const t of writeTools) html += renderToolCard(t)
    html += '</div></section>'
  }

  // 提示词模板
  if (data.prompts?.length) {
    html += `<section class="ai-mcp-docs-section">
      <h3 class="ai-mcp-docs-section__title">提示词模板 (${data.prompts.length} 个)</h3>
      <div class="ai-mcp-docs-prompt-grid">`
    for (const p of data.prompts) {
      const args = (p.arguments || []).map((a) => esc(a.name) + (a.required ? '*' : '')).join(', ')
      html += `<div class="ai-mcp-docs-prompt-card">
        <div class="ai-mcp-docs-prompt-card__head">
          <code class="ai-mcp-docs-prompt-card__name">${esc(p.name)}</code>
          ${args ? `<span class="ai-mcp-docs-prompt-card__args">参数: ${args}</span>` : ''}
        </div>
        <p class="ai-mcp-docs-prompt-card__desc">${esc(p.description)}</p>
      </div>`
    }
    html += '</div></section>'
  }

  // 资源端点
  if (data.resources?.length) {
    html += `<section class="ai-mcp-docs-section">
      <h3 class="ai-mcp-docs-section__title">资源端点 (${data.resources.length} 个)</h3>
      <div class="ai-mcp-docs-resource-grid">`
    for (const r of data.resources) {
      html += `<div class="ai-mcp-docs-resource-card">
        <code class="ai-mcp-docs-resource-card__uri">${esc(r.uri)}</code>
        <span class="ai-mcp-docs-resource-card__name">${esc(r.name)}</span>
        <span class="ai-mcp-docs-resource-card__mime">${esc(r.mimeType || 'application/json')}</span>
        <p class="ai-mcp-docs-resource-card__desc">${esc(r.description)}</p>
      </div>`
    }
    html += '</div></section>'
  }

  // 安全说明
  html += `<section class="ai-mcp-docs-section">
    <h3 class="ai-mcp-docs-section__title">安全说明</h3>
    <div class="ai-mcp-docs-card">
      <ul class="ai-mcp-docs-security-list">
        <li>全功能 CRUD 操作（创建/删除/写入）需要用户在 UI 中确认激活</li>
        <li>所有操作通过 <strong>MCP API Key</strong> 认证（<code>Authorization: Bearer &lt;key&gt;</code> 或 <code>X-MCP-API-Key</code>）</li>
        <li>MCP API Key 独立于系统 JWT，可随时在 UI 中更换</li>
        <li>敏感配置信息（API Key、密码）已脱敏处理</li>
        <li>MCP 端点仅在内网暴露，建议配合防火墙规则使用</li>
        <li>全功能激活状态会持久化保存，重启后保持</li>
      </ul>
    </div>
  </section>`

  // API 端点
  html += `<section class="ai-mcp-docs-section">
    <h3 class="ai-mcp-docs-section__title">API 端点</h3>
    <div class="ai-mcp-docs-grid">
      <div class="ai-mcp-docs-grid__item"><span class="ai-mcp-docs-grid__label">MCP 协议接入</span><code>POST ${esc(data.endpoint || '/api/mcp')}</code></div>
      <div class="ai-mcp-docs-grid__item"><span class="ai-mcp-docs-grid__label">激活全功能</span><code>POST /api/mcp/activate</code></div>
      <div class="ai-mcp-docs-grid__item"><span class="ai-mcp-docs-grid__label">查询状态</span><code>GET /api/mcp/status</code></div>
      <div class="ai-mcp-docs-grid__item"><span class="ai-mcp-docs-grid__label">帮助文档</span><code>GET /api/mcp/help</code></div>
    </div>
  </section>`

  return html
}
