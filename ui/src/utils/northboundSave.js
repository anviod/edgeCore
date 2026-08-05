import { Message } from '@arco-design/web-vue'
import { NORTHBOUND_PROTOCOLS } from '@/utils/northboundProtocols'

/** 解析北向保存 API 错误信息 */
export function resolveNorthboundSaveError(error) {
  return (
    error?.response?.data?.error ||
    error?.response?.data?.message ||
    error?.response?.data?.msg ||
    error?.message ||
    '未知错误'
  )
}

/** 从保存 API 响应中提取连接器启动 warning（若有） */
export function extractNorthboundSaveWarning(response) {
  return response?.warning || ''
}

/** 保存成功提示，区分新建/更新；可选展示连接器启动 warning */
export function notifyNorthboundSaveSuccess(label, isNew, warning) {
  const verb = isNew ? '已添加' : '已更新'
  Message.success(`${label} 配置${verb}`)
  if (warning) {
    Message.warning(warning)
  }
}

/** 保存失败提示，展示具体原因 */
export function notifyNorthboundSaveError(error, label) {
  Message.error(`${label} 保存失败：${resolveNorthboundSaveError(error)}`)
}

/** 表单校验失败提示 */
export function notifyNorthboundValidationError(message) {
  Message.warning(message)
}

/** 通道 ID 合法字符集：英文字母、数字、下划线、横线（禁止空格等其它字符） */
export const NORTHBOUND_ID_PATTERN = /^[A-Za-z0-9_-]+$/

/** 一键生成合法通道 ID（前缀 + 时间戳，如 bacnet_1785909123456） */
export function generateNorthboundID(prefix = 'nb') {
  return `${prefix}_${Date.now()}`
}

/** 清洗输入，仅保留英文字母、数字、下划线、横线 */
export function sanitizeNorthboundID(value) {
  return (value || '').replace(/[^A-Za-z0-9_-]/g, '')
}

/**
 * 校验通道 ID 格式。
 * @returns {string|null} 错误消息，通过时返回 null
 */
export function validateNorthboundID(id) {
  const trimmed = (id || '').trim()
  if (!trimmed) return null
  if (!NORTHBOUND_ID_PATTERN.test(trimmed)) {
    return '通道 ID 只能包含英文字母、数字、下划线或横线，且不能包含空格'
  }
  return null
}

/** 收集所有北向通道 { id, name }，用于名称唯一性校验 */
export function collectNorthboundChannels(config = {}) {
  const channels = []
  for (const meta of Object.values(NORTHBOUND_PROTOCOLS)) {
    for (const item of config[meta.key] || []) {
      channels.push({ id: item.id, name: item.name })
    }
  }
  return channels
}

/**
 * 校验北向通道名称唯一性（跨协议、忽略大小写）。
 * @returns {string|null} 错误消息，通过时返回 null
 */
export function validateNorthboundChannelName(name, channelId, config) {
  const trimmed = (name || '').trim()
  if (!trimmed) return null
  const normalized = trimmed.toLowerCase()
  for (const ch of collectNorthboundChannels(config)) {
    if (ch.id === channelId) continue
    if ((ch.name || '').trim().toLowerCase() === normalized) {
      return `通道名称「${trimmed}」已存在`
    }
  }
  return null
}

/** 关闭设置弹窗 */
export function closeNorthboundSettingsDialog(emit) {
  emit('update:visible', false)
}

/** 静默请求配置，避免与弹窗内错误提示重复 */
export const northboundSaveRequestConfig = { silent: true }
