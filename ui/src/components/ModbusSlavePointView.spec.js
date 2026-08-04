import { describe, it, expect } from 'vitest'
import { nextTick } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import ArcoVue from '@arco-design/web-vue'
import ModbusSlavePointView from './ModbusSlavePointView.vue'

const point = (id, registerType, address = 0, overrides = {}) => ({
  id,
  name: id,
  register_type: registerType,
  address,
  datatype: 'UINT16',
  readwrite: 'R',
  quality: 'Good',
  value: 1,
  ...overrides,
})

const dataComponentStub = {
  props: ['activeKey', 'data', 'pagination', 'selectedKeys', 'scroll'],
  emits: [
    'change',
    'selection-change',
    'update:selectedKeys',
    'update:activeKey',
    'page-change',
    'page-size-change',
  ],
  template: '<div v-bind="$attrs"><slot /><slot name="title" /></div>',
}

const buttonStub = {
  emits: ['click'],
  template: '<button type="button" v-bind="$attrs" @click="$emit(\'click\', $event)"><slot /></button>',
}

const alertStub = {
  props: ['type', 'title'],
  template: '<div role="alert" :data-type="type" v-bind="$attrs"><div>{{ title }}</div><slot /></div>',
}

const mountView = (props = {}) => mount(ModbusSlavePointView, {
  props: {
    allPoints: [],
    points: [],
    selectedIds: [],
    filterKey: '["",[]]',
    loadError: '',
    ...props,
  },
  global: {
    stubs: {
      'a-button': buttonStub,
      'a-alert': alertStub,
      'a-collapse': true,
      'a-collapse-item': true,
      'a-table': dataComponentStub,
      'a-tabs': dataComponentStub,
      'a-tab-pane': dataComponentStub,
      'a-tooltip': true,
      'a-empty': true,
    },
  },
})

describe('ModbusSlavePointView', () => {
  it('mounts with the coil tab active', () => {
    const wrapper = mountView()
    expect(wrapper.get('[data-testid="modbus-tabs"]').attributes('data-active-key')).toBe('coil')
  })

  it('renders four fixed tabs with filtered counts and a single active table', async () => {
    const allPoints = [
      point('c1', 'coil', 0),
      point('c2', 'coil', 1),
      point('di1', 'discrete_input', 0),
      point('ir1', 'input', 0),
      point('ir2', 'input', 1),
      point('hr1', 'holding', 0),
    ]
    const points = [
      point('c1', 'coil', 0),
      point('ir1', 'input', 0),
      point('hr1', 'holding', 0),
    ]

    const wrapper = mountView({ allPoints, points })

    const tabNodes = wrapper.findAll('[data-testid^="register-tab-"]')
    expect(tabNodes.map((node) => node.attributes('data-testid'))).toEqual([
      'register-tab-coil',
      'register-tab-discrete_input',
      'register-tab-input',
      'register-tab-holding',
    ])
    expect(tabNodes.map((node) => node.text())).toEqual([
      '线圈 (1)',
      '离散输入 (0)',
      '输入寄存器 (1)',
      '保持寄存器 (1)',
    ])

    expect(wrapper.get('[data-testid="modbus-tabs"]').attributes('data-active-key')).toBe('coil')
    expect(wrapper.findAll('[data-testid="modbus-point-table"]')).toHaveLength(1)
    expect(wrapper.getComponent('[data-testid="modbus-point-table"]').props('data').map((p) => p.id)).toEqual(['c1'])

    expect(wrapper.find('[data-testid="register-tab-discrete_input"]').exists()).toBe(true)

    await wrapper.getComponent('[data-testid="modbus-tabs"]').vm.$emit('update:activeKey', 'input')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="modbus-tabs"]').attributes('data-active-key')).toBe('input')
    expect(wrapper.findAll('[data-testid="modbus-point-table"]')).toHaveLength(1)
    expect(wrapper.getComponent('[data-testid="modbus-point-table"]').props('data').map((p) => p.id)).toEqual(['ir1'])
  })

  it('keeps independent pagination, resets current on filterKey, and clamps on structure shrink', async () => {
    const makeGroup = (prefix, registerType, count) =>
      Array.from({ length: count }, (_, i) => point(`${prefix}-${i}`, registerType, i))

    let allPoints = [
      ...makeGroup('c', 'coil', 25),
      ...makeGroup('di', 'discrete_input', 25),
      ...makeGroup('ir', 'input', 25),
      ...makeGroup('hr', 'holding', 25),
    ]
    let points = allPoints

    const wrapper = mountView({ allPoints, points, filterKey: '["",[]]' })

    const table = () => wrapper.getComponent('[data-testid="modbus-point-table"]')

    expect(table().props('pagination')).toMatchObject({
      current: 1,
      pageSize: 10,
      pageSizeOptions: [10, 20, 50, 100],
      showPageSize: true,
      showTotal: true,
    })

    await table().vm.$emit('page-change', 2)
    await table().vm.$emit('page-size-change', 20)
    await wrapper.vm.$nextTick()

    expect(table().props('pagination')).toMatchObject({ current: 1, pageSize: 20 })
    await table().vm.$emit('page-change', 2)
    await wrapper.vm.$nextTick()
    expect(table().props('pagination')).toMatchObject({ current: 2, pageSize: 20 })

    await wrapper.getComponent('[data-testid="modbus-tabs"]').vm.$emit('update:activeKey', 'holding')
    await wrapper.vm.$nextTick()

    await table().vm.$emit('page-change', 3)
    await table().vm.$emit('page-size-change', 10)
    await wrapper.vm.$nextTick()
    expect(table().props('pagination')).toMatchObject({ current: 1, pageSize: 10 })
    await table().vm.$emit('page-change', 3)
    await wrapper.vm.$nextTick()
    expect(table().props('pagination')).toMatchObject({ current: 3, pageSize: 10 })

    await wrapper.getComponent('[data-testid="modbus-tabs"]').vm.$emit('update:activeKey', 'coil')
    await wrapper.vm.$nextTick()
    expect(table().props('pagination')).toMatchObject({ current: 2, pageSize: 20 })

    await wrapper.setProps({ filterKey: '["temp",[]]' })
    await wrapper.vm.$nextTick()
    expect(table().props('pagination')).toMatchObject({ current: 1, pageSize: 20 })

    await wrapper.getComponent('[data-testid="modbus-tabs"]').vm.$emit('update:activeKey', 'holding')
    await wrapper.vm.$nextTick()
    expect(table().props('pagination')).toMatchObject({ current: 1, pageSize: 10 })

    await wrapper.setProps({ filterKey: '["",[]]' })
    await wrapper.vm.$nextTick()
    await table().vm.$emit('page-change', 3)
    await wrapper.vm.$nextTick()
    expect(table().props('pagination')).toMatchObject({ current: 3, pageSize: 10 })

    allPoints = [
      ...makeGroup('c', 'coil', 25),
      ...makeGroup('di', 'discrete_input', 25),
      ...makeGroup('ir', 'input', 25),
      ...makeGroup('hr', 'holding', 12),
    ]
    points = allPoints
    await wrapper.setProps({ allPoints, points })
    await wrapper.vm.$nextTick()
    expect(table().props('pagination')).toMatchObject({ current: 2, pageSize: 10 })

    // 缩减为 0：空态无表；恢复到 25 条后应仍为第 1 页（证明曾收敛到 1，而非停留在 2）
    allPoints = [
      ...makeGroup('c', 'coil', 25),
      ...makeGroup('di', 'discrete_input', 25),
      ...makeGroup('ir', 'input', 25),
    ]
    points = allPoints
    await wrapper.setProps({ allPoints, points })
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="modbus-point-table"]').exists()).toBe(false)

    allPoints = [
      ...makeGroup('c', 'coil', 25),
      ...makeGroup('di', 'discrete_input', 25),
      ...makeGroup('ir', 'input', 25),
      ...makeGroup('hr', 'holding', 25),
    ]
    points = allPoints
    await wrapper.setProps({ allPoints, points })
    await wrapper.vm.$nextTick()
    expect(table().props('pagination')).toMatchObject({ current: 1, pageSize: 10 })

    await wrapper.getComponent('[data-testid="modbus-tabs"]').vm.$emit('update:activeKey', 'coil')
    await wrapper.vm.$nextTick()
    await table().vm.$emit('page-change', 2)
    await wrapper.vm.$nextTick()
    expect(table().props('pagination')).toMatchObject({ current: 2, pageSize: 20 })

    const valueUpdated = points.map((p) =>
      p.id.startsWith('c-')
        ? { ...p, value: 99, updated_at: '2026-07-18T00:00:00Z' }
        : p
    )
    await wrapper.setProps({ allPoints: valueUpdated, points: valueUpdated })
    await wrapper.vm.$nextTick()
    expect(table().props('pagination')).toMatchObject({ current: 2, pageSize: 20 })
  })

  it('merges selection across tabs, pages, and filters from global selectedIds', async () => {
    const allPoints = [
      point('coil-1', 'coil', 0),
      point('coil-2', 'coil', 1),
      point('holding-1', 'holding', 0),
      point('holding-2', 'holding', 1),
    ]
    let points = [...allPoints]

    const wrapper = mountView({
      allPoints,
      points,
      selectedIds: [],
      filterKey: '["",[]]',
    })

    const table = () => wrapper.getComponent('[data-testid="modbus-point-table"]')

    await table().vm.$emit('selection-change', ['coil-1'])
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('selection-change')?.at(-1)?.[0]).toEqual(['coil-1'])

    await wrapper.setProps({ selectedIds: ['coil-1'] })
    await wrapper.vm.$nextTick()

    await wrapper.getComponent('[data-testid="modbus-tabs"]').vm.$emit('update:activeKey', 'holding')
    await wrapper.vm.$nextTick()
    await table().vm.$emit('selection-change', ['holding-1'])
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('selection-change')?.at(-1)?.[0]).toEqual(['coil-1', 'holding-1'])

    await wrapper.setProps({ selectedIds: ['coil-1', 'holding-1'] })
    await wrapper.vm.$nextTick()
    await table().vm.$emit('selection-change', [])
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('selection-change')?.at(-1)?.[0]).toEqual(['coil-1'])

    await wrapper.setProps({ selectedIds: ['coil-1'] })
    await wrapper.vm.$nextTick()
    points = [
      point('coil-2', 'coil', 1),
      point('holding-1', 'holding', 0),
      point('holding-2', 'holding', 1),
    ]
    await wrapper.setProps({
      points,
      filterKey: '["coil-hidden",[]]',
      selectedIds: ['coil-1'],
    })
    await wrapper.vm.$nextTick()

    await wrapper.getComponent('[data-testid="modbus-tabs"]').vm.$emit('update:activeKey', 'coil')
    await wrapper.vm.$nextTick()
    await table().vm.$emit('selection-change', ['coil-2'])
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('selection-change')?.at(-1)?.[0]).toEqual(['coil-1', 'coil-2'])

    await wrapper.setProps({
      allPoints,
      points: allPoints,
      selectedIds: ['coil-1', 'coil-2'],
      filterKey: '["",[]]',
    })
    await wrapper.vm.$nextTick()
    expect(table().props('selectedKeys')).toEqual(['coil-1', 'coil-2'])

    await table().vm.$emit('page-change', 1)
    await wrapper.vm.$nextTick()
    expect(table().props('selectedKeys')).toEqual(['coil-1', 'coil-2'])

    const valueUpdated = allPoints.map((p) =>
      p.id === 'coil-1' ? { ...p, value: 42, updated_at: '2026-07-18T01:00:00Z' } : p
    )
    await wrapper.setProps({ allPoints: valueUpdated, points: valueUpdated })
    await wrapper.vm.$nextTick()
    expect(table().props('selectedKeys')).toEqual(['coil-1', 'coil-2'])

    await wrapper.setProps({ selectedIds: ['coil-1', 'coil-1', 'holding-1'] })
    await wrapper.vm.$nextTick()
    await wrapper.getComponent('[data-testid="modbus-tabs"]').vm.$emit('update:activeKey', 'holding')
    await wrapper.vm.$nextTick()
    await table().vm.$emit('selection-change', ['holding-1', 'holding-2'])
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('selection-change')?.at(-1)?.[0]).toEqual([
      'coil-1',
      'holding-1',
      'holding-2',
    ])
  })

  it('supports keyboard a11y, labeled tabs, and responsive structure hooks', async () => {
    const allPoints = [
      point('coil-1', 'coil', 0),
      point('holding-1', 'holding', 0),
    ]

    const mountA11y = (props = {}) => mount(ModbusSlavePointView, {
      props: {
        allPoints: [],
        points: [],
        selectedIds: [],
        filterKey: '["",[]]',
        loadError: '',
        ...props,
      },
      attachTo: document.body,
      global: {
        plugins: [ArcoVue],
        stubs: {
          'a-table': dataComponentStub,
          'a-tooltip': true,
          'a-empty': true,
        },
      },
    })

    const wrapper = mountA11y({
      allPoints,
      points: allPoints,
    })
    await flushPromises()
    await nextTick()

    const tabLabels = wrapper.findAll('[data-testid^="register-tab-"]').map((node) => node.text())
    expect(tabLabels).toEqual([
      '线圈 (1)',
      '离散输入 (0)',
      '输入寄存器 (0)',
      '保持寄存器 (1)',
    ])
    tabLabels.forEach((label) => {
      expect(label).toMatch(/\S+ \(\d+\)/)
    })

    expect(wrapper.classes()).toContain('modbus-point-tabs')
    expect(wrapper.find('.arco-tabs-nav-tab').exists()).toBe(true)
    expect(wrapper.html()).not.toMatch(/outline\s*:\s*none/i)

    const table = wrapper.getComponent('[data-testid="modbus-point-table"]')
    expect(table.props('scroll')).toEqual({ x: 1126 })

    const activeTab = wrapper.find('.arco-tabs-tab-active')
    expect(activeTab.exists()).toBe(true)
    activeTab.element.focus()
    await activeTab.trigger('keydown', { key: 'ArrowRight' })
    await nextTick()
    expect(wrapper.get('[data-testid="modbus-tabs"]').attributes('data-active-key')).toBe('discrete_input')
    // Empty discrete_input tab: no table mounted; arrow did not require table row interaction.
    expect(wrapper.findAll('[data-testid="modbus-point-table"]')).toHaveLength(0)

    await wrapper.setProps({
      allPoints: [],
      points: [],
      loadError: '获取点位失败：network down',
    })
    await nextTick()
    const retryBtn = wrapper.findAll('button').find((node) => node.text().includes('重试'))
    expect(retryBtn).toBeTruthy()
    expect(retryBtn.element.tagName).toBe('BUTTON')
    expect(retryBtn.attributes('type')).toBe('button')
    // 原生 button 在浏览器中由 Enter 触发 click；jsdom 无该默认行为，此处用 click 验证单次 emit
    await retryBtn.trigger('click')
    await nextTick()
    expect(wrapper.emitted('retry')).toHaveLength(1)

    await wrapper.setProps({
      allPoints,
      points: [point('holding-1', 'holding', 0)],
      loadError: '',
      filterKey: '["no-coil",[]]',
    })
    await flushPromises()
    await nextTick()
    await wrapper.find('[data-testid="register-tab-coil"]').trigger('click')
    await nextTick()
    const clearBtn = wrapper.findAll('button').find((node) => node.text().includes('清除全局筛选'))
    expect(clearBtn).toBeTruthy()
    expect(clearBtn.element.tagName).toBe('BUTTON')
    expect(clearBtn.attributes('type')).toBe('button')
    await clearBtn.trigger('click')
    await nextTick()
    expect(wrapper.emitted('clear-filters')).toHaveLength(1)

    wrapper.unmount()
  })

  it('distinguishes category-empty, filter-empty, and loadError states', async () => {
    const wrapper = mountView({
      allPoints: [],
      points: [],
      loadError: '',
    })
    expect(wrapper.text()).toContain('该设备暂无点位配置')
    expect(wrapper.text()).not.toContain('清除全局筛选')
    expect(wrapper.find('[data-testid="modbus-point-table"]').exists()).toBe(false)

    const allPoints = [
      point('coil-1', 'coil', 0),
      point('holding-1', 'holding', 0),
    ]
    await wrapper.setProps({
      allPoints,
      points: [],
      filterKey: '["no-match",[]]',
      loadError: '',
    })
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('当前筛选条件下无匹配点位')
    expect(wrapper.text()).not.toContain('该设备暂无点位配置')
    expect(wrapper.text()).not.toContain('该寄存器类型暂无点位')
    const clearBtn = wrapper.findAll('button').find((node) => node.text().includes('清除全局筛选'))
    expect(clearBtn).toBeTruthy()
    await clearBtn.trigger('click')
    expect(wrapper.emitted('clear-filters')).toBeTruthy()

    await wrapper.setProps({
      allPoints: [],
      points: [],
      loadError: '获取点位失败：network down',
    })
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('获取点位失败：network down')
    expect(wrapper.text()).toContain('重试')
    expect(wrapper.text()).not.toContain('该寄存器类型暂无点位')
    expect(wrapper.text()).not.toContain('当前筛选条件下无匹配点位')
    expect(wrapper.find('[data-testid="modbus-point-table"]').exists()).toBe(false)
    const retryBlocking = wrapper.findAll('button').find((node) => node.text().includes('重试'))
    await retryBlocking.trigger('click')
    expect(wrapper.emitted('retry')).toBeTruthy()

    await wrapper.setProps({
      allPoints,
      points: allPoints,
      loadError: '获取点位失败：refresh failed',
    })
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="modbus-point-table"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('获取点位失败：refresh failed')
    const retryInline = wrapper.findAll('button').find((node) => node.text().includes('重试'))
    await retryInline.trigger('click')
    expect(wrapper.emitted('retry')?.length).toBeGreaterThanOrEqual(2)
  })
})

export { point }
