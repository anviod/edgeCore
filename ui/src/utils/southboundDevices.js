/** 拉取全部南向设备（与 ChannelList 一致，兼容数组 / { data } 响应） */
export async function fetchAllSouthboundDevices(request) {
  const res = await request.get('/api/channels')
  const channels = Array.isArray(res) ? res : (res?.data || [])
  const devices = []

  for (const ch of channels) {
    let devs = []
    if (Array.isArray(ch.devices) && ch.devices.length > 0) {
      devs = ch.devices
    } else if (ch.id) {
      const devRes = await request.get(`/api/channels/${ch.id}/devices`)
      devs = Array.isArray(devRes) ? devRes : (devRes?.data || [])
    }
    devs.forEach((d) => {
      devices.push({ ...d, channelName: ch.name })
    })
  }

  return devices
}

/** 根据北向配置中的 devices 映射，构建扁平上报策略表格行。
 *  若无 channelName 需求，可用本函数；树形表格请用 buildNorthboundDeviceTree。 */
export function buildNorthboundDeviceRows(allDevices, deviceConfig, defaultInterval = '10s') {
  return (allDevices || []).map((dev) => buildNorthboundDeviceRow(dev, deviceConfig, defaultInterval))
}

/** 构建单个设备行（含 _enable/_strategy/_interval） */
function buildNorthboundDeviceRow(dev, deviceConfig, defaultInterval = '10s') {
  const current = deviceConfig?.[dev.id]
  let _enable = false
  let _strategy = 'periodic'
  let _interval = defaultInterval

  if (current === undefined || current === null) {
    _enable = false
  } else if (typeof current === 'boolean') {
    _enable = current
    if (_enable) {
      _strategy = 'periodic'
      _interval = defaultInterval
    }
  } else if (typeof current === 'object') {
    _enable = !!current.enable
    _strategy = current.strategy || 'periodic'
    _interval = current.interval || defaultInterval
  }

  return { ...dev, _enable, _strategy, _interval }
}

/** 将真实设备按通道分组构建为树形上报表格行。
 *  通道为父行（可批量启用/禁用整通道设备），设备为子行。
 *  返回结构：{ rowKey: channelKey, channelName, count, enabledCount, children: [设备行(rowKey=id)...] }
 *  顺序：有通道名的正常分组在前，未分组排最后。 */
export function buildNorthboundDeviceTree(allDevices, deviceConfig, defaultInterval = '10s') {
  const groups = new Map()
  for (const dev of allDevices || []) {
    const key = dev.channelName || dev.channel_id || '未分组'
    if (!groups.has(key)) {
      groups.set(key, { rowKey: key, channelKey: key, channelName: key, count: 0, children: [] })
    }
    const child = buildNorthboundDeviceRow(dev, deviceConfig, defaultInterval)
    child.rowKey = dev.id
    groups.get(key).children.push(child)
  }
  const rows = []
  for (const g of groups.values()) {
    g.count = g.children.length
    g.enabledCount = g.children.filter((c) => c._enable).length
    rows.push(g)
  }
  // 未分组通道排最后
  return rows.sort((a, b) => (a.channelKey === '未分组' ? 1 : b.channelKey === '未分组' ? -1 : 0))
}

/** 将真实设备树形表格行同步回配置的 devices 字段（稀疏存储，严格白名单语义）。
 *  空映射或未显式启用的设备一律不暴露；未勾选的设备也会显式写入 enable:false，
 *  与后端 AllowsDevice / northbound_publish 语义保持一致。 */
export function syncNorthboundDevicesFromTree(treeData) {
  if (!treeData?.length) {
    return {}
  }
  const devices = {}
  for (const group of treeData) {
    for (const child of group.children || []) {
      devices[child.id] = { enable: !!child._enable }
    }
  }
  return devices
}

/** 根据 OPC UA/BACnet 配置中的 virtual_devices 映射，构建虚拟设备暴露表格行。
 *  白名单语义：仅显式启用（Enable=true）的虚拟设备才暴露到北向地址空间；
 *  空映射或未列出的设备一律不暴露，与后端 AllowsDevice 一致。 */
export function buildNorthboundVirtualDeviceRows(allVirtualDevices, deviceConfig) {
  return (allVirtualDevices || []).map((dev) => {
    const current = deviceConfig?.[dev.id]
    let _enable = false
    if (current === undefined || current === null) {
      _enable = false
    } else if (typeof current === 'boolean') {
      _enable = current
    } else if (typeof current === 'object') {
      _enable = !!current.enable
    }
    return {
      ...dev,
      pointCount: dev.points?.length || 0,
      _enable
    }
  })
}

/** 根据北向配置中的 virtual_devices 映射，构建虚拟设备上报策略表格行 */
export function buildNorthboundVirtualDeviceStrategyRows(allVirtualDevices, deviceConfig, defaultInterval = '10s') {
  return (allVirtualDevices || []).map((dev) => {
    const current = deviceConfig?.[dev.id]
    let _enable = false
    let _strategy = 'periodic'
    let _interval = defaultInterval

    if (current === undefined || current === null) {
      _enable = false
    } else if (typeof current === 'boolean') {
      _enable = current
      if (_enable) {
        _strategy = 'periodic'
        _interval = defaultInterval
      }
    } else if (typeof current === 'object') {
      _enable = !!current.enable
      _strategy = current.strategy || 'periodic'
      _interval = current.interval || defaultInterval
    }

    return {
      ...dev,
      pointCount: dev.points?.length || 0,
      _enable,
      _strategy,
      _interval
    }
  })
}

/** 将虚拟设备表格行同步回 OPC UA/BACnet 配置的 virtual_devices 字段（稀疏存储，白名单语义） */
export function syncNorthboundVirtualDevicesFromRows(rows) {
  if (!rows?.length) {
    return {}
  }
  const devices = {}
  for (const record of rows) {
    if (record._enable) {
      devices[record.id] = { enable: true }
    } else {
      // 显式写入禁用，确保设备即使出现在映射中也保持不暴露
      devices[record.id] = { enable: false }
    }
  }
  return devices
}
