import { describe, it, expect, beforeEach } from 'vitest'
import { nextTick } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import NorthboundReportStrategyPanel from './NorthboundReportStrategyPanel.vue'
import { globalState } from '@/composables/useGlobalState'

const dev = (id, name, overrides = {}) => ({
  id,
  name,
  channelName: 'Ch-1',
  state: 0,
  ...overrides,
})

const tableStub = {
  props: ['data', 'selectedKeys', 'rowSelection'],
  emits: ['selection-change', 'update:selectedKeys'],
  template: '<div class="table-stub"><slot /></div>',
}

const selectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  template: '<select class="select-stub" v-bind="$attrs"><slot /></select>',
}

const optionStub = {
  props: ['value'],
  template: '<option class="option-stub" :value="value"><slot /></option>',
}

const inputStub = {
  props: ['modelValue', 'placeholder'],
  emits: ['update:modelValue', 'change', 'press-enter'],
  template: '<input class="input-stub" :value="modelValue" v-bind="$attrs" />',
}

const switchStub = {
  props: ['modelValue', 'disabled'],
  emits: ['update:modelValue', 'change'],
  template: '<button class="switch-stub" :disabled="disabled" @click="$emit(\'update:modelValue\', !modelValue); $emit(\'change\', !modelValue)">{{ modelValue ? "on" : "off" }}</button>',
}

const dropdownStub = {
  emits: ['select'],
  template: '<div class="dropdown-stub" @click="$emit(\'select\', \'periodic\')"><slot /><slot name="content" /></div>',
}

const doptionStub = {
  props: ['value'],
  template: '<span class="doption-stub" :data-value="value"><slot /></span>',
}

const emptyStub = { props: ['description'], template: '<div class="empty-stub">{{ description }}</div>' }
const tagStub = { props: ['color'], template: '<span class="tag-stub"><slot /></span>' }

let devices
let devicesMap

beforeEach(() => {
  devices = [dev('dev-1', '电表A'), dev('dev-2', '电表B')]
  devicesMap = {}
  globalState.snackbar.show = false
  globalState.snackbar.text = ''
})

const mountPanel = () => mount(NorthboundReportStrategyPanel, {
  props: {
    deviceKind: 'real',
    visible: true,
    allDevices: devices,
    devices: devicesMap,
    virtualDevices: {},
    defaultInterval: '10s',
  },
  global: {
    stubs: {
      'a-table': tableStub,
      'a-select': selectStub,
      'a-option': optionStub,
      'a-input': inputStub,
      'a-switch': switchStub,
      'a-button': { emits: ['click'], template: '<button type="button" v-bind="$attrs" @click="$emit(\'click\', $event)"><slot /></button>' },
      'a-dropdown': dropdownStub,
      'a-doption': doptionStub,
      'a-empty': emptyStub,
      'a-tag': tagStub,
    },
  },
})

describe('NorthboundReportStrategyPanel batch linkage', () => {
  it('prompts when batch mode is clicked with no selection', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await nextTick()

    const dropdowns = wrapper.findAll('.dropdown-stub')
    expect(dropdowns.length).toBeGreaterThan(0)
    dropdowns[0].trigger('click')
    await nextTick()

    expect(globalState.snackbar.show).toBe(true)
    expect(globalState.snackbar.text).toContain('没有勾选设备')
  })

  it('batch mode changes period and strategy for selected devices', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await nextTick()

    // enable both devices first (direct row state, mirrors switch ON)
    const rows = wrapper.vm.realDeviceTableData
    rows[0]._enable = true
    rows[1]._enable = true
    await nextTick()

    // simulate selecting dev-1 + dev-2 via Arco selection-change event
    const tables = wrapper.findAllComponents(tableStub)
    const realTable = tables[0]
    realTable.vm.$emit('selection-change', ['dev-1', 'dev-2'])
    await nextTick()

    // simulate choosing 周期上报 in the batch-mode dropdown
    const dropdowns = wrapper.findAll('.dropdown-stub')
    dropdowns[0].trigger('click')
    await nextTick()

    expect(globalState.snackbar.show).toBe(true)
    expect(globalState.snackbar.text).toContain('周期上报（全量）')

    // the batch operation emits update:devices for each selected device
    const updates = wrapper.emitted('update:devices') || []
    const merged = {}
    for (const [payload] of updates) {
      Object.assign(merged, payload)
    }
    expect(merged['dev-1']).toMatchObject({ enable: true, strategy: 'periodic', interval: '10s' })
    expect(merged['dev-2']).toMatchObject({ enable: true, strategy: 'periodic', interval: '10s' })
  })
})
