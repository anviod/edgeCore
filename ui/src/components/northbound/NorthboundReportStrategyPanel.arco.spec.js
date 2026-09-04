import { describe, it, expect } from 'vitest'
import { nextTick } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import ArcoVue from '@arco-design/web-vue'
import NorthboundReportStrategyPanel from './NorthboundReportStrategyPanel.vue'

const dev = (id, name, overrides = {}) => ({
  id,
  name,
  channelName: 'Ch-1',
  state: 0,
  ...overrides,
})

const mountPanel = (devices) => mount(NorthboundReportStrategyPanel, {
  props: {
    deviceKind: 'real',
    visible: true,
    allDevices: devices,
    devices: {},
    virtualDevices: {},
    defaultInterval: '10s',
  },
  global: {
    plugins: [ArcoVue],
  },
  attachTo: document.body,
})

describe('NorthboundReportStrategyPanel real-Arco tree batch linkage', () => {
  it('renders a channel parent row and device child rows (tree grouping)', async () => {
    const wrapper = mountPanel([dev('dev-1', '电表A'), dev('dev-2', '电表B')])
    await flushPromises()
    await nextTick()

    // 通道父行含 badge 文本，子行为设备名
    const bodyText = wrapper.find('.arco-table').text()
    expect(bodyText).toContain('Ch-1')
    expect(bodyText).toContain('电表A')
    expect(bodyText).toContain('电表B')
    expect(bodyText).toContain('0/2 已启用')

    wrapper.unmount()
  })

  it('channel parent checkbox enables all devices, then 周期上报（全量）applies to them', async () => {
    const wrapper = mountPanel([dev('dev-1', '电表A'), dev('dev-2', '电表B')])
    await flushPromises()
    await nextTick()

    // 勾选通道父行启用列 → 批量启用该通道全部设备
    const channelRow = wrapper.findAll('tr.arco-table-tr')
      .find(tr => tr.text().includes('Ch-1'))
    const channelCheckbox = channelRow.find('.arco-checkbox')
    await channelCheckbox.trigger('click')
    await nextTick()

    const updatesAfterChannel = wrapper.emitted('update:devices') || []
    const mergedAfter = {}
    for (const [payload] of updatesAfterChannel) Object.assign(mergedAfter, payload)
    expect(mergedAfter['dev-1']).toMatchObject({ enable: true, strategy: 'periodic', interval: '10s' })
    expect(mergedAfter['dev-2']).toMatchObject({ enable: true, strategy: 'periodic', interval: '10s' })

    // 此时设备子行应显示 周期上报 + 10s
    const dev1Tr = wrapper.findAll('tr.arco-table-tr').find(tr => tr.text().includes('电表A'))
    expect(dev1Tr.text()).toContain('周期上报')
    const intervalInput = dev1Tr.find('td:last-child input')
    expect(intervalInput.element.value).toBe('10s')

    wrapper.unmount()
  })
})