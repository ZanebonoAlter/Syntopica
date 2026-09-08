import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CompositeLabelPool from './CompositeLabelPool.vue'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

const getLabelsMock = vi.fn()
const disableLabelMock = vi.fn()
const enableLabelMock = vi.fn()

vi.mock('~/api/compositeLabels', () => ({
  useCompositeLabelsApi: () => ({ getLabels: getLabelsMock, createLabel: vi.fn(), disableLabel: disableLabelMock, enableLabel: enableLabelMock }),
}))

function compositeItem(id: number, label: string, status: string, components: { label_id: number, label: string, position: number }[], refCount = 3) {
  return {
    id, label, slug: `comp-${id}`, description: '', source: 'manual', status,
    ref_count: refCount, aliases: [], created_at: '2026-09-01T00:00:00Z', components,
  }
}

function mountPool() {
  return mount(CompositeLabelPool, { global: { stubs: { teleport: true } } })
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('CompositeLabelPool — 组合标签治理页', () => {
  it('列表渲染 label / 组件序列（position 排序）/ ref_count / status', async () => {
    getLabelsMock.mockResolvedValue({
      success: true,
      data: {
        items: [
          compositeItem(1, '美债收益率', 'active', [
            { label_id: 11, label: '收益率', position: 2 },
            { label_id: 10, label: '美国国债', position: 1 },
          ], 7),
          compositeItem(2, '中国CPI', 'disabled', [
            { label_id: 20, label: '中国', position: 1 },
            { label_id: 21, label: 'CPI', position: 2 },
          ]),
        ],
        total: 2,
      },
    })
    const w = mountPool()
    await flushPromises()
    const items = w.findAll('[data-testid^="composite-label-item-"]')
    expect(items.length).toBe(2)
    // 组件序列按 position 排序展示
    const chain = w.find('[data-testid="composite-label-chain"]')
    expect(chain.text()).toBe('美国国债 × 收益率')
    expect(w.find('[data-testid="composite-label-item-1"]').attributes('data-status')).toBe('active')
    expect(w.find('[data-testid="composite-label-item-2"]').attributes('data-status')).toBe('disabled')
    expect(w.find('[data-testid="composite-label-item-1"] .clp-item-ref').text()).toBe('7')
  })

  it('空态：明确文案不残留旧数据（变体2）', async () => {
    getLabelsMock.mockResolvedValue({ success: true, data: { items: [], total: 0 } })
    const w = mountPool()
    await flushPromises()
    const empty = w.find('[data-testid="composite-label-pool-empty"]')
    expect(empty.exists()).toBe(true)
    expect(empty.text()).toContain('暂无组合标签')
    expect(w.findAll('[data-testid^="composite-label-item-"]').length).toBe(0)
  })

  it('加载失败展示错误提示（变体3）', async () => {
    getLabelsMock.mockRejectedValue(new Error('network'))
    const w = mountPool()
    await flushPromises()
    expect(w.find('[data-testid="composite-label-pool-error"]').text()).toContain('加载组合标签失败')
  })

  it('禁用 active 组合调用 disable API 并刷新（成功提示）', async () => {
    getLabelsMock.mockResolvedValue({ success: true, data: { items: [compositeItem(1, '美债收益率', 'active', [{ label_id: 10, label: '美国国债', position: 1 }, { label_id: 11, label: '收益率', position: 2 }])], total: 1 } })
    disableLabelMock.mockResolvedValue({ success: true, data: { id: 1, status: 'disabled' } })
    const w = mountPool()
    await flushPromises()
    await w.find('[data-testid="composite-label-toggle-1"]').trigger('click')
    await flushPromises()
    expect(disableLabelMock).toHaveBeenCalledWith(1)
    expect(w.find('[data-testid="composite-label-notice"]').text()).toContain('已禁用')
    expect(getLabelsMock).toHaveBeenCalledTimes(2) // 初次加载 + 操作后刷新
  })

  it('启用 disabled 组合调用 enable API', async () => {
    getLabelsMock.mockResolvedValue({ success: true, data: { items: [compositeItem(2, '中国CPI', 'disabled', [{ label_id: 20, label: '中国', position: 1 }, { label_id: 21, label: 'CPI', position: 2 }])], total: 1 } })
    enableLabelMock.mockResolvedValue({ success: true, data: { id: 2, status: 'active' } })
    const w = mountPool()
    await flushPromises()
    await w.find('[data-testid="composite-label-toggle-2"]').trigger('click')
    await flushPromises()
    expect(enableLabelMock).toHaveBeenCalledWith(2)
    expect(w.find('[data-testid="composite-label-notice"]').text()).toContain('已启用')
  })

  it('状态过滤 tab 重新拉取（active/disabled）', async () => {
    getLabelsMock.mockResolvedValue({ success: true, data: { items: [], total: 0 } })
    const w = mountPool()
    await flushPromises()
    await w.findAll('.clp-tab')[1]!.trigger('click') // 启用中
    await flushPromises()
    expect(getLabelsMock).toHaveBeenLastCalledWith({ status: 'active' })
  })
})
