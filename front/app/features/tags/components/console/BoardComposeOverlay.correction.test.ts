import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import BoardComposeOverlay from './BoardComposeOverlay.vue'

/**
 * 泳道修正工作面（修正模式）外壳用例（tasks 5.1 / 5.8 / 5.10 + A-1 / A-8 / A-5）。
 *
 * unify-lane-workbench 3.3 起修正模式为**三栏**：左 380（已选侧：成员/更早/已移出/首义）
 * + 中央画布 + 右 300（待选侧：关联推荐）；两侧内容不重复（左栏无推荐、右栏无成员）。
 *
 * 判据（机械断言）：
 * - 三栏均渲染；左栏无推荐分组、右栏无成员清单（S1.2 三栏内容不重复）；
 * - 节点卡快捷加入/移出/加回当场生效（S1.5/S1.6：一次点击一次 assignment 写）；
 * - 泳道名就地编辑：点标题 → 输入框 → Enter 保存 → toast；**全程零弹窗**；
 * - 窄屏（<768px）：只提示「需要宽屏或横屏」，不渲染残缺工作台；
 * - 新建模式行为不变（仍是自己的三栏 + 命名弹窗；OQ-4：栏位与保存出口不变）。
 */

const { persistentMock } = vi.hoisted(() => ({
  persistentMock: { getComposeCandidates: vi.fn(), createManualLane: vi.fn(), embedQuery: vi.fn() },
}))

const { reportsMock } = vi.hoisted(() => ({
  reportsMock: {
    listBoardTopics: vi.fn(),
    updateTopic: vi.fn(),
    getTopicLifeline: vi.fn(),
    getTopicAnchor: vi.fn(),
    setSectionAssignment: vi.fn(),
    updateTopicAnchor: vi.fn(),
  },
}))

const { notifyMock } = vi.hoisted(() => ({
  notifyMock: { success: vi.fn(), error: vi.fn(), warn: vi.fn() },
}))

const { narrowState } = vi.hoisted(() => ({ narrowState: { value: false } }))

vi.mock('~/api/persistentTopics', () => ({ usePersistentTopicsApi: () => persistentMock }))
vi.mock('~/api/dailyReports', () => ({ useDailyReportsApi: () => reportsMock }))
vi.mock('~/api/laneDynamicsWindow', () => ({ useLaneDynamicsWindowApi: () => dynamicsMock }))
vi.mock('~/composables/useNotify', () => ({ useNotify: () => notifyMock }))
vi.mock('~/composables/useMediaQuery', () => ({ useIsNarrowViewport: () => narrowState }))

const { dynamicsMock } = vi.hoisted(() => ({ dynamicsMock: { loadLaneDynamics: vi.fn() } }))

const SECTIONS = [
  { id: '101', reportId: '1', periodDate: '2026-11-10', clusterLabel: '成员一', embedding: [1, 0], persistentTopicId: '7' },
  { id: '103', reportId: '1', periodDate: '2026-11-11', clusterLabel: '候选节点', embedding: [0.8, 0.2] },
]

async function mountCorrection(overrides: Record<string, unknown> = {}) {
  const w = mount(BoardComposeOverlay, {
    props: { boardId: 1974, boardLabel: '测试版块', mode: 'correction', topicId: 7, laneLabel: 'AI 前沿', ...overrides },
    attachTo: document.body,
    global: { stubs: { teleport: true } },
  })
  await flushPromises()
  await flushPromises()
  return w
}

beforeEach(() => {
  narrowState.value = false
  document.body.innerHTML = ''
  notifyMock.success.mockReset()
  notifyMock.error.mockReset()
  persistentMock.getComposeCandidates.mockReset()
  reportsMock.getTopicLifeline.mockReset()
  reportsMock.getTopicAnchor.mockReset()
  reportsMock.updateTopic.mockReset()
  reportsMock.setSectionAssignment.mockReset()
  reportsMock.updateTopicAnchor.mockReset()
  dynamicsMock.loadLaneDynamics.mockReset().mockResolvedValue({
    success: true,
    data: {
      window: {
        windowDays: 365,
        hasReports: true,
        anchorDate: '2026-11-13',
        lanes: [{ topicId: '7', label: 'AI 前沿', timeline: [{ date: '2026-11-12', clueCount: 3 }] }],
        candidates: [],
      },
    },
  })

  persistentMock.getComposeCandidates.mockResolvedValue({
    success: true,
    data: {
      sections: SECTIONS,
      relations: [],
      candidateChains: [],
      matchThreshold: 0.3,
      window: { from: '2026-11-10', to: '2026-11-13', days: 4 },
    },
  })
  reportsMock.getTopicLifeline.mockResolvedValue({
    success: true,
    data: { sections: [{ id: 101, period_date: '2026-11-10', cluster_label: '成员一', topic_match_distance: 0.09 }], relations: [] },
  })
  reportsMock.getTopicAnchor.mockResolvedValue({
    success: true,
    data: { topic_id: 7, source: 'auto', section_ids: [], override_count: 0, effective_from: '' },
  })
  reportsMock.updateTopic.mockResolvedValue({ success: true, data: null })
  reportsMock.setSectionAssignment.mockResolvedValue({ success: true, data: null })
  reportsMock.updateTopicAnchor.mockResolvedValue({ success: true, data: null })
})

describe('BoardComposeOverlay · 修正模式三栏（unify-lane-workbench 3.3 / S1.1 / S1.2）', () => {
  it('三栏均渲染：左 380 已选侧 / 中央画布 / 右 300 待选侧；三栏栅格', async () => {
    const w = await mountCorrection()
    expect(w.find('[data-testid="lane-correction-left"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-correction-canvas"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-correction-right"]').exists()).toBe(true)
    expect(w.find('.bco__workbench--correction').exists()).toBe(true)
    // 新建右栏（检视）不在修正模式出现
    expect(w.find('.bco__right').exists()).toBe(false)
    expect(w.find('[data-testid="lane-compose-inspector"]').exists()).toBe(false)
  })

  it('三栏内容不重复：左栏无关联推荐分组，右栏无成员清单（S1.2）', async () => {
    const w = await mountCorrection()
    const left = w.find('[data-testid="lane-correction-left"]')
    expect(left.find('[data-testid="lane-members-panel"]').exists()).toBe(true)
    expect(left.find('[data-testid="lane-anchor-panel"]').exists()).toBe(true)
    expect(left.find('[data-testid="lane-recommend-panel"]').exists()).toBe(false) // 左栏 SHALL NOT 出现推荐
    const right = w.find('[data-testid="lane-correction-right"]')
    expect(right.find('[data-testid="lane-recommend-panel"]').exists()).toBe(true) // 推荐只在右栏
    expect(right.find('[data-testid="lane-members-panel"]').exists()).toBe(false) // 右栏无成员清单
    expect(right.text()).not.toContain('本泳道成员')
  })

  it('左栏承载四块：成员 / 更早 / 已移出 / 首义范围', async () => {
    const w = await mountCorrection()
    const left = w.find('[data-testid="lane-correction-left"]')
    expect(left.find('[data-testid="lane-members-panel"]').exists()).toBe(true)
    expect(left.find('[data-testid="lane-removed-group"]').exists()).toBe(false) // 无移出时不渲染
    expect(left.find('[data-testid="lane-anchor-panel"]').exists()).toBe(true)
  })

  it('生效时机只以结论型徽标表达（页脚两枚）', async () => {
    const w = await mountCorrection()
    const foot = w.find('[data-testid="lane-correction-foot"]')
    expect(foot.text()).toContain('归属 · 立即生效')
    expect(foot.text()).toContain('次日日报生效')
  })
})

describe('BoardComposeOverlay · 节点卡快捷加入与移除（unify 4.2 / S1.5 / S1.6）', () => {
  it('点候选节点只聚焦；点卡上「＋加入本泳道」一次 assignment 写并即时生效', async () => {
    const w = await mountCorrection()
    // 点节点本体：0 次写（查看与选入分离在宿主层保留）
    await w.find('[data-testid="lane-node-103"]').trigger('click')
    await flushPromises()
    expect(reportsMock.setSectionAssignment).not.toHaveBeenCalled()
    expect(w.find('[data-testid="lane-node-card"]').exists()).toBe(true)
    // 卡上加入：一次写，写对目标
    await w.find('[data-testid="lane-card-add"]').trigger('click')
    await flushPromises()
    expect(reportsMock.setSectionAssignment).toHaveBeenCalledTimes(1)
    expect(reportsMock.setSectionAssignment).toHaveBeenCalledWith(103, 7)
    expect(notifyMock.success).toHaveBeenCalled()
  })

  it('成员卡上「⊘移出本泳道」一次写 topic_id:null（不弹去处选择器/二次确认）', async () => {
    const w = await mountCorrection()
    await w.find('[data-testid="lane-node-101"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="lane-card-remove"]').trigger('click')
    await flushPromises()
    expect(reportsMock.setSectionAssignment).toHaveBeenCalledWith(101, null)
    expect(w.find('.app-dialog').exists()).toBe(false)
    expect(w.find('select').exists()).toBe(false)
    // 移出后落「已移出（本次）」可加回
    expect(w.find('[data-testid="lane-restore-101"]').exists()).toBe(true)
  })

  it('写失败：集合与卡片保留、无成功 toast，可重试（B6）', async () => {
    reportsMock.setSectionAssignment.mockResolvedValueOnce({ success: false, error: '写入被拒' })
    const w = await mountCorrection()
    await w.find('[data-testid="lane-node-101"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="lane-card-remove"]').trigger('click')
    await flushPromises()
    expect(reportsMock.setSectionAssignment).toHaveBeenCalledTimes(1)
    expect(notifyMock.error).toHaveBeenCalled()
    // 成员未被移除（事实集合不变），卡片仍在
    expect(w.find('[data-testid="lane-node-card"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-restore-101"]').exists()).toBe(false)
    // 重试成功
    await w.find('[data-testid="lane-card-remove"]').trigger('click')
    await flushPromises()
    expect(reportsMock.setSectionAssignment).toHaveBeenCalledTimes(2)
  })

  it('右栏推荐行「＋加入」与节点卡同一语义（同一份状态/同一写路径）', async () => {
    const w = await mountCorrection()
    // 候选 103 距离成员质心近 → 出现在右栏推荐
    const row = w.find('[data-testid="lane-recommend-add-103"]')
    expect(row.exists()).toBe(true)
    await row.trigger('click')
    await flushPromises()
    expect(reportsMock.setSectionAssignment).toHaveBeenCalledTimes(1)
    expect(reportsMock.setSectionAssignment).toHaveBeenCalledWith(103, 7)
  })
})

describe('BoardComposeOverlay · 修正模式时间视窗（unify 6.4 / S3.1）', () => {
  it('顶部时间视窗条在场：档位 7/14/30/365、本泳道曲线、无 URL query 写回', async () => {
    const w = await mountCorrection()
    const range = w.find('[data-testid="lane-correction-range"]')
    expect(range.exists()).toBe(true)
    expect(w.findAll('.orc-tier').map(b => b.text())).toEqual(['7 天', '14 天', '30 天', '近 1 年'])
    expect(range.text()).toContain('本泳道逐日线索数')
    expect(dynamicsMock.loadLaneDynamics).toHaveBeenCalledWith(1974, 365)
    // 模态内本地 state：不写 URL query（无 router 交互痕迹可断，断控件不受路由）
    expect(w.find('[data-testid="observation-range-controls"]').exists()).toBe(true)
  })

  it('档位切换取数一次；失败保旧（档位回退）', async () => {
    const w = await mountCorrection()
    persistentMock.getComposeCandidates.mockClear()
    persistentMock.getComposeCandidates.mockResolvedValueOnce({ success: false, error: '网络抖动' })
    await w.findAll('.orc-tier')[1]!.trigger('click')
    await flushPromises()
    await flushPromises()
    expect(persistentMock.getComposeCandidates).toHaveBeenCalledWith(1974, { days: 14, allMembersTopicId: 7 })
    // 失败保旧：当前窗口档仍在 7（无未生效激活态）
    expect(w.findAll('.orc-tier')[0]!.classes()).toContain('is-active')
  })
})

describe('BoardComposeOverlay · 泳道名就地编辑（A-8）', () => {
  it('点标题变输入框 → Enter 保存 → toast「泳道名已更新」，全程零弹窗', async () => {
    const w = await mountCorrection()
    expect(w.find('[data-testid="lane-correction-name"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-correction-name-input"]').exists()).toBe(false)

    await w.find('[data-testid="lane-correction-name"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-correction-name-input"]').exists()).toBe(true)

    const input = w.find('[data-testid="lane-correction-name-input"] input')
    await input.setValue('新泳道名')
    await input.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(reportsMock.updateTopic).toHaveBeenCalledWith(7, { label: '新泳道名' })
    expect(notifyMock.success.mock.calls.at(-1)![0]).toBe('泳道名已更新')
    // 零弹窗：没有对话框被渲染
    expect(w.find('.app-dialog').exists()).toBe(false)
    expect(document.querySelector('.app-dialog')).toBeNull()
  })

  it('Esc 取消就地编辑（不写库）', async () => {
    const w = await mountCorrection()
    await w.find('[data-testid="lane-correction-name"]').trigger('click')
    await flushPromises()
    const input = w.find('[data-testid="lane-correction-name-input"] input')
    await input.setValue('不该保存')
    await input.trigger('keydown', { key: 'Escape' })
    await flushPromises()
    expect(reportsMock.updateTopic).not.toHaveBeenCalled()
    expect(w.find('[data-testid="lane-correction-name"]').exists()).toBe(true)
  })

  // 回归（2026-11-15 实测）：就地编辑的 Esc 只取消编辑，不得连带关掉整个工作面
  // （window 级 Esc 监听会在 nameEditing 已置 false 后接手 → emit('close')）。
  it('就地编辑按 Esc：不写库、只取消编辑，工作面不退出（不 emit close）', async () => {
    const w = await mountCorrection()
    await w.find('[data-testid="lane-correction-name"]').trigger('click')
    await flushPromises()
    const input = w.find('[data-testid="lane-correction-name-input"] input')
    await input.setValue('不该保存')
    await input.trigger('keydown', { key: 'Escape' })
    await flushPromises()
    expect(reportsMock.updateTopic).not.toHaveBeenCalled()
    expect(w.emitted('close')).toBeUndefined()
    expect(w.find('[data-testid="lane-correction-left"]').exists()).toBe(true)
  })
})

describe('BoardComposeOverlay · 窄屏（A-5）', () => {
  it('窄屏只提示「需要宽屏或横屏」，不渲染残缺工作台', async () => {
    narrowState.value = true
    const w = await mountCorrection()
    expect(w.find('[data-testid="lane-correction-narrow"]').text()).toContain('泳道修正需要宽屏或横屏')
    expect(w.find('[data-testid="lane-correction-left"]').exists()).toBe(false)
    expect(w.find('[data-testid="lane-correction-canvas"]').exists()).toBe(false)
    expect(w.find('[data-testid="lane-correction-right"]').exists()).toBe(false)
    narrowState.value = false
  })
})

describe('BoardComposeOverlay · 新建模式不变', () => {
  it('新建模式仍是三栏（候选栏 / 画布 / 右栏），没有修正模式左栏', async () => {
    const w = mount(BoardComposeOverlay, {
      props: { boardId: 1974, boardLabel: '测试版块' },
      attachTo: document.body,
      global: { stubs: { teleport: true } },
    })
    await flushPromises()
    await flushPromises()
    expect(w.find('.bco__right').exists()).toBe(true)
    expect(w.find('[data-testid="lane-correction-left"]').exists()).toBe(false)
    expect(w.find('[data-testid="lane-compose-canvas"]').exists()).toBe(true)
  })
})
