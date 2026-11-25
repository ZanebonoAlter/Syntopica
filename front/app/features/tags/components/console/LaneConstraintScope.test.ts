import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import LaneConstraintScope from './LaneConstraintScope.vue'

/**
 * 左栏 · 辅助标签约束（topic-lane-auxiliary-label-constraint 第 6 组）。
 *
 * 判据（机械断言，见 change 的 ui-design.md「验收映射」）：
 * - 块头常驻：标题 + 计数徽标 + 生效徽标「次日日报生效」；**默认折叠**（aria-expanded=false）
 * - 折叠时**不发请求**（懒加载：展开才 load），展开后渲染空态
 * - 添加 → 提交全量覆盖载荷 → 行出现、计数同步
 * - required / preferred 切换走同一 PUT（term 未变，后端复用锚）
 * - 移除 → 提交省略该条
 * - 计算中禁用该行操作；失败保留短词并可重试
 * - 重复短词在前端拦下，不发请求
 * - **界面不得出现派生机制解释句**（「不影响生命周期」这类）
 */

const { reportsMock } = vi.hoisted(() => ({
  reportsMock: {
    getTopicLaneConstraints: vi.fn(),
    putTopicLaneConstraints: vi.fn(),
  },
}))

vi.mock('~/api/dailyReports', () => ({
  useDailyReportsApi: () => reportsMock,
}))

function constraint(term: string, mode: 'required' | 'preferred' = 'required') {
  return {
    id: 1,
    topicId: 2,
    term,
    mode,
    margin: null,
    maxDistance: null,
    embeddingModel: 'qwen3-embedding',
    createdAt: '2026-11-24T00:00:00Z',
    updatedAt: '2026-11-24T00:00:00Z',
  }
}

describe('LaneConstraintScope', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    reportsMock.getTopicLaneConstraints.mockResolvedValue({ success: true, data: { constraints: [] } })
    reportsMock.putTopicLaneConstraints.mockImplementation((_id: number, rows: Array<{ term: string, mode: 'required' | 'preferred' }>) => Promise.resolve({
      success: true,
      data: { constraints: rows.map((row, i: number) => ({ ...constraint(row.term, row.mode), id: i + 1 })) },
    }))
  })

  it('块头常驻且默认折叠，计数徽标仍需加载数据', async () => {
    const wrapper = mount(LaneConstraintScope, { props: { topicId: 2 } })
    await flushPromises()

    expect(wrapper.find('[data-testid="lane-constraint-panel"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('辅助标签')
    expect(wrapper.text()).toContain('次日日报生效')
    expect(wrapper.find('[data-testid="lane-constraint-toggle"]').attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('[data-testid="lane-constraint-count"]').text()).toBe('0')
    // 折叠态徽标要能显示真实条数 → 挂载即加载（不装载的话徽标永远是 0）。
    expect(reportsMock.getTopicLaneConstraints).toHaveBeenCalledWith(2)
  })

  it('展开后加载并渲染空态', async () => {
    const wrapper = mount(LaneConstraintScope, { props: { topicId: 2 } })
    await wrapper.find('[data-testid="lane-constraint-toggle"]').trigger('click')
    await flushPromises()

    expect(reportsMock.getTopicLaneConstraints).toHaveBeenCalledWith(2)
    expect(wrapper.find('[data-testid="lane-constraint-empty"]').text()).toContain('未绑定辅助标签')
    expect(wrapper.find('[data-testid="lane-constraint-toggle"]').attributes('aria-expanded')).toBe('true')
  })

  it('添加约束提交全量覆盖并渲染行与计数', async () => {
    const wrapper = mount(LaneConstraintScope, { props: { topicId: 2 } })
    await wrapper.find('[data-testid="lane-constraint-toggle"]').trigger('click')
    await flushPromises()

    await wrapper.find('#lane-constraint-term').setValue('以色列')
    await wrapper.find('[data-testid="lane-constraint-submit"]').trigger('click')
    await flushPromises()

    expect(reportsMock.putTopicLaneConstraints).toHaveBeenCalledWith(2, [{ term: '以色列', mode: 'required' }])
    expect(wrapper.findAll('[data-testid="lane-constraint-row"]')).toHaveLength(1)
    expect(wrapper.find('[data-testid="lane-constraint-count"]').text()).toBe('1')
  })

  it('切换模式走同一 PUT 全量覆盖', async () => {
    reportsMock.getTopicLaneConstraints.mockResolvedValue({
      success: true,
      data: { constraints: [constraint('以色列')] },
    })
    const wrapper = mount(LaneConstraintScope, { props: { topicId: 2 } })
    await wrapper.find('[data-testid="lane-constraint-toggle"]').trigger('click')
    await flushPromises()

    await wrapper.find('[data-testid="lane-constraint-mode"]').trigger('click')
    await flushPromises()

    expect(reportsMock.putTopicLaneConstraints).toHaveBeenLastCalledWith(2, [{ term: '以色列', mode: 'preferred' }])
  })

  it('移除约束提交省略该条', async () => {
    reportsMock.getTopicLaneConstraints.mockResolvedValue({
      success: true,
      data: { constraints: [constraint('以色列'), constraint('黎巴嫩')] },
    })
    const wrapper = mount(LaneConstraintScope, { props: { topicId: 2 } })
    await wrapper.find('[data-testid="lane-constraint-toggle"]').trigger('click')
    await flushPromises()

    await wrapper.findAll('[data-testid="lane-constraint-remove"]')[0]!.trigger('click')
    await flushPromises()

    expect(reportsMock.putTopicLaneConstraints).toHaveBeenLastCalledWith(2, [{ term: '黎巴嫩', mode: 'required' }])
    expect(wrapper.find('[data-testid="lane-constraint-count"]').text()).toBe('1')
  })

  it('失败时回滚并提示，保留草稿短词', async () => {
    reportsMock.putTopicLaneConstraints.mockRejectedValueOnce(new Error('embedding unavailable'))
    const wrapper = mount(LaneConstraintScope, { props: { topicId: 2 } })
    await wrapper.find('[data-testid="lane-constraint-toggle"]').trigger('click')
    await flushPromises()

    await wrapper.find('#lane-constraint-term').setValue('以色列')
    await wrapper.find('[data-testid="lane-constraint-submit"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="lane-constraint-error"]').text()).toContain('embedding unavailable')
    expect(wrapper.findAll('[data-testid="lane-constraint-row"]')).toHaveLength(0)
    expect((wrapper.find('#lane-constraint-term').element as HTMLInputElement).value).toBe('以色列')
  })

  it('重复短词在前端拦下，不发请求', async () => {
    reportsMock.getTopicLaneConstraints.mockResolvedValue({
      success: true,
      data: { constraints: [constraint('以色列')] },
    })
    const wrapper = mount(LaneConstraintScope, { props: { topicId: 2 } })
    await wrapper.find('[data-testid="lane-constraint-toggle"]').trigger('click')
    await flushPromises()

    await wrapper.find('#lane-constraint-term').setValue('以色列')
    await wrapper.find('[data-testid="lane-constraint-submit"]').trigger('click')
    await flushPromises()

    expect(reportsMock.putTopicLaneConstraints).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="lane-constraint-error"]').text()).toContain('已存在')
  })

  it('界面不出现派生机制解释句', async () => {
    reportsMock.getTopicLaneConstraints.mockResolvedValue({
      success: true,
      data: { constraints: [constraint('以色列')] },
    })
    const wrapper = mount(LaneConstraintScope, { props: { topicId: 2 } })
    await wrapper.find('[data-testid="lane-constraint-toggle"]').trigger('click')
    await flushPromises()

    const text = wrapper.text()
    expect(text).not.toContain('不影响生命周期')
    expect(text).not.toContain('只影响吸附')
    expect(text).not.toContain('重心')
  })
})

describe('LaneConstraintScope · 草稿模式（新建泳道）', () => {
  it('草稿模式不发任何请求，只维护本地行集并上报', async () => {
    vi.clearAllMocks()
    const wrapper = mount(LaneConstraintScope, { props: { topicId: null, draft: true } })
    await flushPromises()

    // 不加载、不落库
    expect(reportsMock.getTopicLaneConstraints).not.toHaveBeenCalled()
    expect(reportsMock.putTopicLaneConstraints).not.toHaveBeenCalled()

    await wrapper.find('[data-testid="lane-constraint-toggle"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="lane-constraint-empty"]').exists()).toBe(true)

    await wrapper.find('#lane-constraint-term').setValue('以色列')
    await wrapper.find('[data-testid="lane-constraint-submit"]').trigger('click')
    await flushPromises()

    // 依然零请求；行出现在本地
    expect(reportsMock.getTopicLaneConstraints).not.toHaveBeenCalled()
    expect(reportsMock.putTopicLaneConstraints).not.toHaveBeenCalled()
    expect(wrapper.findAll('[data-testid="lane-constraint-row"]')).toHaveLength(1)
    expect(wrapper.find('[data-testid="lane-constraint-count"]').text()).toBe('1')

    const emitted = wrapper.emitted('update:draftConstraints')
    expect(emitted).toBeTruthy()
    expect(emitted!.at(-1)).toEqual([[{ term: '以色列', mode: 'required' }]])
  })

  it('草稿模式可删行，变更继续上报', async () => {
    vi.clearAllMocks()
    const wrapper = mount(LaneConstraintScope, { props: { topicId: null, draft: true } })
    await wrapper.find('[data-testid="lane-constraint-toggle"]').trigger('click')
    await flushPromises()

    await wrapper.find('#lane-constraint-term').setValue('以色列')
    await wrapper.find('[data-testid="lane-constraint-submit"]').trigger('click')
    await flushPromises()
    await wrapper.find('#lane-constraint-term').setValue('黎巴嫩')
    await wrapper.find('[data-testid="lane-constraint-submit"]').trigger('click')
    await flushPromises()

    await wrapper.findAll('[data-testid="lane-constraint-remove"]')[0]!.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="lane-constraint-count"]').text()).toBe('1')
    const emitted = wrapper.emitted('update:draftConstraints')
    expect(emitted!.at(-1)).toEqual([[{ term: '黎巴嫩', mode: 'required' }]])
  })
})
