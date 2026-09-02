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
  props: ['data', 'selectedKeys', 'rowSelection', 'defaultExpandedKeys'],
  provides: {},
  emits: ['selection-change', 'update:selectedKeys', 'expand-change'],
  template: '<div class="table-stub"><slot /></div>',
}

const checkboxStub = {
  props: ['modelValue', 'indeterminate'],
  emits: ['update:modelValue', 'change'],
  template: '<input class="checkbox-stub" type="checkbox" :checked="modelValue" @change="$emit(\'change\', !modelValue)" />',
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
      'a-checkbox': checkboxStub,
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

describe('NorthboundReportStrategyPanel batch linkage (tree grouped by channel)', () => {
  it('builds a tree with one channel parent row containing both devices', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await nextTick()

    const rows = wrapper.vm.realDeviceTableData
    expect(rows).toHaveLength(1)
    const channel = rows[0]
    expect(channel.channelName).toBe('Ch-1')
    expect(channel.children).toHaveLength(2)
    expect(channel.count).toBe(2)
    expect(channel.enabledCount).toBe(0)
  })

  it('prompts when batch mode is clicked with no enabled device', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await nextTick()

    const dropdowns = wrapper.findAll('.dropdown-stub')
    expect(dropdowns.length).toBeGreaterThan(0)
    dropdowns[0].trigger('click')
    await nextTick()

    expect(globalState.snackbar.show).toBe(true)
    expect(globalState.snackbar.text).toContain('已启用')
  })

  it('channel parent toggle enables all its devices and batch period/strategy apply to them', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await nextTick()

    // 勾选通道父行 → 批量启用该通道下全部设备（走 setChannelAll）
    const channel = wrapper.vm.realDeviceTableData[0]
    wrapper.vm.setChannelAll(channel, true)
    await nextTick()

    expect(channel.enabledCount).toBe(2)
    const updatesAfterChannel = wrapper.emitted('update:devices') || []
    const mergedAfterChannel = {}
    for (const [payload] of updatesAfterChannel) {
      Object.assign(mergedAfterChannel, payload)
    }
    expect(mergedAfterChannel['dev-1']).toMatchObject({ enable: true, strategy: 'periodic', interval: '10s' })
    expect(mergedAfterChannel['dev-2']).toMatchObject({ enable: true, strategy: 'periodic', interval: '10s' })

    // 模拟批量下拉选择 周期上报
    globalState.snackbar.show = false
    globalState.snackbar.text = ''
    const dropdowns = wrapper.findAll('.dropdown-stub')
    dropdowns[0].trigger('click')
    await nextTick()

    expect(globalState.snackbar.show).toBe(true)
    expect(globalState.snackbar.text).toContain('周期上报（全量）')

    // 已启用设备的批量操作通过 update:devices 逐设备发出
    const updates = wrapper.emitted('update:devices') || []
    const merged = {}
    for (const [payload] of updates) {
      Object.assign(merged, payload)
    }
    expect(merged['dev-1']).toMatchObject({ enable: true, strategy: 'periodic', interval: '10s' })
    expect(merged['dev-2']).toMatchObject({ enable: true, strategy: 'periodic', interval: '10s' })
  })
})