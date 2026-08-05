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

describe('NorthboundReportStrategyPanel real-Arco batch linkage', () => {
  it('selecting a row then 周期上报（全量）changes the strategy in the list', async () => {
    const devices = [dev('dev-1', '电表A'), dev('dev-2', '电表B')]
    const wrapper = mountPanel(devices)
    await flushPromises()
    await nextTick()

    // enable both devices via their row switches so strategy select is active
    const switches = wrapper.findAll('.arco-switch')
    for (const sw of switches) {
      await sw.trigger('click')
    }
    await nextTick()

    // select the dev-1 row via its checkbox
    const trs = wrapper.findAll('tr.arco-table-tr')
    const targetTr = trs.find(tr => tr.text().includes('电表A'))
    expect(targetTr).toBeTruthy()
    const checkbox = targetTr.find('.arco-checkbox')
    await checkbox.trigger('click')
    await nextTick()

    // open the batch-mode dropdown and pick 周期上报（全量）
    const modeButton = wrapper.findAll('button').find(b => b.text().includes('批量上报模式'))
    expect(modeButton).toBeTruthy()
    await modeButton.trigger('click')
    await nextTick()

    const options = Array.from(document.body.querySelectorAll('.arco-dropdown-option'))
    const periodicOption = options.find(o => o.textContent.includes('周期上报（全量）'))
    expect(periodicOption, 'dropdown 周期上报 option should render').toBeTruthy()
    periodicOption.click()
    await nextTick()

    // the strategy select for dev-1 should now show 周期上报 with 10s interval
    const dev1Tr = wrapper.findAll('tr.arco-table-tr').find(tr => tr.text().includes('电表A'))
    expect(dev1Tr.text()).toContain('周期上报')
    const intervalInput = dev1Tr.find('td:last-child input')
    expect(intervalInput.element.value).toBe('10s')

    wrapper.unmount()
  })

  it('selecting a not-enabled row and choosing 周期上报（全量）auto-enables it and updates the list', async () => {
    const devices = [dev('dev-1', '电表A'), dev('dev-2', '电表B')]
    const wrapper = mountPanel(devices)
    await flushPromises()
    await nextTick()

    // do NOT enable switches first — batch mode should auto-enable the selected row

    // select the dev-1 row via its checkbox
    const trs = wrapper.findAll('tr.arco-table-tr')
    const targetTr = trs.find(tr => tr.text().includes('电表A'))
    const checkbox = targetTr.find('.arco-checkbox')
    await checkbox.trigger('click')
    await nextTick()

    // open the batch-mode dropdown and pick 周期上报（全量）
    const modeButton = wrapper.findAll('button').find(b => b.text().includes('批量上报模式'))
    await modeButton.trigger('click')
    await nextTick()

    const options = Array.from(document.body.querySelectorAll('.arco-dropdown-option'))
    const periodicOption = options.find(o => o.textContent.includes('周期上报（全量）'))
    expect(periodicOption, 'dropdown 周期上报 option should render').toBeTruthy()
    periodicOption.click()
    await nextTick()

    const dev1Tr = wrapper.findAll('tr.arco-table-tr').find(tr => tr.text().includes('电表A'))
    expect(dev1Tr.text()).toContain('周期上报')
    const intervalInput = dev1Tr.find('td:last-child input')
    expect(intervalInput.element.value).toBe('10s')

    wrapper.unmount()
  })
})
