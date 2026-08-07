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

describe('NorthboundReportStrategyPanel select-all / half-select', () => {
  const headerCheckbox = (wrapper) => wrapper.find('.arco-table-th.arco-table-checkbox .arco-checkbox')

  it('row checkbox toggles realSelectedKeys and header shows half-select', async () => {
    const wrapper = mountPanel([dev('dev-1', '电表A'), dev('dev-2', '电表B')])
    await flushPromises()
    await nextTick()

    expect(wrapper.vm.realSelectedKeys).toEqual([])

    const trs = wrapper.findAll('tr.arco-table-tr')
    const targetTr = trs.find(tr => tr.text().includes('电表A'))
    await targetTr.find('.arco-checkbox').trigger('click')
    await nextTick()

    expect(wrapper.vm.realSelectedKeys).toEqual(['dev-1'])
    expect(headerCheckbox(wrapper).classes()).toContain('arco-checkbox-indeterminate')

    // uncheck the same row → selection cleared
    await targetTr.find('.arco-checkbox').trigger('click')
    await nextTick()
    expect(wrapper.vm.realSelectedKeys).toEqual([])

    wrapper.unmount()
  })

  it('header select-all checks every row and batch enable applies to all', async () => {
    const wrapper = mountPanel([dev('dev-1', '电表A'), dev('dev-2', '电表B')])
    await flushPromises()
    await nextTick()

    await headerCheckbox(wrapper).trigger('click')
    await nextTick()
    expect(wrapper.vm.realSelectedKeys).toEqual(['dev-1', 'dev-2'])

    const batchButton = wrapper.findAll('button').find(b => b.text().includes('批量启用'))
    expect(batchButton).toBeTruthy()
    await batchButton.trigger('click')
    await nextTick()

    const updates = wrapper.emitted('update:devices') || []
    const merged = {}
    for (const [payload] of updates) Object.assign(merged, payload)
    expect(merged['dev-1']).toMatchObject({ enable: true, strategy: 'periodic', interval: '10s' })
    expect(merged['dev-2']).toMatchObject({ enable: true, strategy: 'periodic', interval: '10s' })

    // clicking select-all again clears the selection
    await headerCheckbox(wrapper).trigger('click')
    await nextTick()
    expect(wrapper.vm.realSelectedKeys).toEqual([])

    wrapper.unmount()
  })
})
