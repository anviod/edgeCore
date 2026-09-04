import { describe, it, expect } from 'vitest'
import {
  buildNorthboundDeviceTree,
  syncNorthboundDevicesFromTree,
  buildNorthboundVirtualDeviceRows,
  syncNorthboundVirtualDevicesFromRows
} from './southboundDevices'

const dev = (id, name, channelName, overrides = {}) => ({
  id,
  name,
  channelName,
  state: 0,
  ...overrides
})

describe('southboundDevices', () => {
  describe('buildNorthboundDeviceTree', () => {
    it('按通道分组为树形，父行带 count/enabledCount，白名单默认不启用', () => {
      const all = [
        dev('d1', '电表A', 'BACNET'),
        dev('d2', '电表B', 'BACNET'),
        dev('d3', '电表C', 'MODBUS')
      ]
      const tree = buildNorthboundDeviceTree(all, {})
      expect(tree.length).toBe(2)
      const bacnet = tree.find((g) => g.channelKey === 'BACNET')
      const modbus = tree.find((g) => g.channelKey === 'MODBUS')
      expect(bacnet.count).toBe(2)
      expect(bacnet.enabledCount).toBe(0)
      expect(bacnet.children.map((c) => c.rowKey)).toEqual(['d1', 'd2'])
      expect(modbus.count).toBe(1)
      // 空映射 → 全部不启用（严格白名单）
      expect(tree.flatMap((g) => g.children).every((c) => c._enable === false)).toBe(true)
    })

    it('已启用设备反映到 enabledCount 与 _enable', () => {
      const all = [dev('d1', '电表A', 'BACNET'), dev('d2', '电表B', 'BACNET')]
      const config = { d1: { enable: true, strategy: 'periodic', interval: '5s' } }
      const tree = buildNorthboundDeviceTree(all, config)
      const bacnet = tree.find((g) => g.channelKey === 'BACNET')
      expect(bacnet.enabledCount).toBe(1)
      expect(bacnet.children.find((c) => c.id === 'd1')._enable).toBe(true)
    })

    it('未分组设备排最后', () => {
      const all = [dev('d1', 'A', undefined), dev('d2', 'B', 'CHAN-1')]
      const tree = buildNorthboundDeviceTree(all, {})
      expect(tree[tree.length - 1].channelKey).toBe('未分组')
    })
  })

  describe('syncNorthboundDevicesFromTree', () => {
    it('从树收集白名单映射：启用写 true，未启用显式写 false', () => {
      const all = [dev('d1', 'A', 'CHAN-1'), dev('d2', 'B', 'CHAN-1')]
      const config = { d1: { enable: true } }
      const tree = buildNorthboundDeviceTree(all, config)
      // 手动启用 d2
      tree[0].children.find((c) => c.id === 'd2')._enable = true
      const devices = syncNorthboundDevicesFromTree(tree)
      expect(devices).toEqual({
        d1: { enable: true },
        d2: { enable: true }
      })
    })

    it('空树同步为空映射；未启用设备也写入 enable:false', () => {
      const all = [dev('d1', 'A', 'CHAN-1')]
      const devices = syncNorthboundDevicesFromTree(buildNorthboundDeviceTree(all, {}))
      expect(devices).toEqual({ d1: { enable: false } })
      expect(syncNorthboundDevicesFromTree([])).toEqual({})
    })
  })

  describe('buildNorthboundVirtualDeviceRows / sync', () => {
    it('虚拟设备为白名单：空映射默认不暴露', () => {
      const vdevs = [{ id: 'v1', name: '影子1', enable: true, points: [] }]
      const rows = buildNorthboundVirtualDeviceRows(vdevs, {})
      expect(rows[0]._enable).toBe(false)
    })

    it('同步时启用写 true、未启用写 false', () => {
      const rows = [{ id: 'v1', _enable: true }, { id: 'v2', _enable: false }]
      expect(syncNorthboundVirtualDevicesFromRows(rows)).toEqual({
        v1: { enable: true },
        v2: { enable: false }
      })
    })
  })
})