export function makePointRef(channelId, deviceId, pointId) {
  return `${channelId}.${deviceId}.${pointId}`
}

export function parsePointRef(ref) {
  if (!ref || typeof ref !== 'string') return null
  const parts = ref.split('.')
  if (parts.length < 3) return null
  return {
    channelId: parts[0],
    deviceId: parts[1],
    pointId: parts.slice(2).join('.')
  }
}

export function formatSourceLabel(source) {
  if (!source) return ''
  const name = source.point_name || source.point_id
  const dev = source.device_name || source.device_id
  const ch = source.channel_name || source.channel_id
  return `${ch} / ${dev} / ${name}`
}

export const FORMULA_OPERATORS = ['+', '-', '*', '/', '(', ')']

export function newVirtualPoint(mode = 'map') {
  return {
    point_id: '',
    name: '',
    unit: '',
    mode,
    source_ref: '',
    formula: ''
  }
}

export function newVirtualDeviceForm() {
  return {
    id: '',
    name: '',
    description: '',
    enable: true,
    points: []
  }
}

/** 返回重复的非空 point_id -> 积木索引列表（仅 length > 1 的项） */
export function findDuplicatePointIds(points) {
  const byId = new Map()
  for (let i = 0; i < (points || []).length; i++) {
    const id = points[i]?.point_id?.trim()
    if (!id) continue
    if (!byId.has(id)) byId.set(id, [])
    byId.get(id).push(i)
  }
  const dupes = new Map()
  for (const [id, indices] of byId) {
    if (indices.length > 1) dupes.set(id, indices)
  }
  return dupes
}

/** 所有涉及重复的积木索引（升序） */
export function flattenDuplicatePointIndices(dupesMap) {
  const set = new Set()
  for (const indices of dupesMap.values()) {
    for (const i of indices) set.add(i)
  }
  return [...set].sort((a, b) => a - b)
}

/** 将扁平点位源列表聚合为设备维度 */
export function buildSourceDeviceList(sources) {
  const map = new Map()
  for (const src of sources || []) {
    const key = `${src.channel_id}::${src.device_id}`
    if (!map.has(key)) {
      map.set(key, {
        key,
        channel_id: src.channel_id,
        channel_name: src.channel_name || src.channel_id,
        device_id: src.device_id,
        device_name: src.device_name || src.device_id,
        pointCount: 0,
        points: []
      })
    }
    const dev = map.get(key)
    dev.points.push(src)
    dev.pointCount++
  }
  return Array.from(map.values()).sort((a, b) => {
    const la = `${a.channel_name}/${a.device_name}`.toLowerCase()
    const lb = `${b.channel_name}/${b.device_name}`.toLowerCase()
    return la.localeCompare(lb)
  })
}

/** 模糊匹配：子串 + 字符顺序匹配 */
export function fuzzyMatch(text, query) {
  if (!query) return true
  const t = String(text || '').toLowerCase()
  const q = String(query).toLowerCase().trim()
  if (!q) return true
  if (t.includes(q)) return true
  let ti = 0
  for (let i = 0; i < q.length; i++) {
    const idx = t.indexOf(q[i], ti)
    if (idx === -1) return false
    ti = idx + 1
  }
  return true
}

export function sourceFromRef(ref, sources) {
  const found = (sources || []).find(s => s.ref === ref)
  if (found) return found
  const parsed = parsePointRef(ref)
  if (!parsed) return null
  return {
    ref,
    channel_id: parsed.channelId,
    device_id: parsed.deviceId,
    point_id: parsed.pointId,
    point_name: parsed.pointId,
    device_name: parsed.deviceId,
    channel_name: parsed.channelId
  }
}

export const DRAG_MIME = 'application/vnd.edgeCore.virtual-shadow-points+json'

export function encodeDragRefs(refs) {
  return encodeDragPayload({ refs })
}

export function decodeDragRefs(dataTransfer) {
  return decodeDragPayload(dataTransfer).refs
}

/** @param {{ refs?: string[], device?: object|null }} payload */
export function encodeDragPayload({ refs, device } = {}) {
  const list = Array.isArray(refs) ? refs : refs ? [refs] : []
  return JSON.stringify({ refs: list, device: device || null })
}

function parseDragPayloadJson(raw) {
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object') return null
    const refs = Array.isArray(parsed.refs) ? parsed.refs : []
    const device = parsed.device && typeof parsed.device === 'object' ? parsed.device : null
    if (refs.length || device) return { refs, device }
  } catch (_) {
    /* ignore */
  }
  return null
}

/** @returns {{ refs: string[], device: object|null }} */
export function decodeDragPayload(dataTransfer) {
  if (!dataTransfer) return { refs: [], device: null }
  const fromMime = parseDragPayloadJson(dataTransfer.getData(DRAG_MIME))
  if (fromMime) return fromMime
  const fromPlain = parseDragPayloadJson(dataTransfer.getData('text/plain'))
  if (fromPlain) return fromPlain
  const plain = dataTransfer.getData('text/plain')
  return { refs: plain ? [plain] : [], device: null }
}

/** 兼容直接数组或 { data: [] } 响应 */
export function normalizeArrayResponse(res) {
  if (Array.isArray(res)) return res
  if (res?.data && Array.isArray(res.data)) return res.data
  return []
}

/** Device.state: 0=Online, 1=Unstable, 2=Offline, 3=Quarantine */
export function isDeviceOnline(dev) {
  if (!dev) return false
  if (typeof dev.online === 'boolean') return dev.online
  const state = dev.state
  return state === 0 || state === 1
}

export function mapDeviceToSummary(dev, channelId, channelName) {
  const points = dev?.points
  return {
    key: `${channelId}::${dev.id}`,
    channel_id: channelId,
    channel_name: channelName || channelId,
    device_id: dev.id,
    device_name: dev.name || dev.id,
    point_count: Array.isArray(points) ? points.length : 0,
    online: isDeviceOnline(dev),
    state: dev?.state ?? 2
  }
}

export function mapPointToSource(pt, channelId, channelName, deviceId, deviceName) {
  return {
    channel_id: channelId,
    channel_name: channelName || channelId,
    device_id: deviceId,
    device_name: deviceName || deviceId,
    point_id: pt.id,
    point_name: pt.name || pt.id,
    ref: makePointRef(channelId, deviceId, pt.id)
  }
}
