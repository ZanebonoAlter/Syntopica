import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useLaneCompose } from './useLaneCompose'
import type { ComposeCandidatesData } from '~/api/persistentTopics'

/**
 * 白盒分支表 §3.4（选择状态与命名，S1–S10）。判据来自主线程 2026-10-31 定稿。
 * 另附窗口/草稿确认/日期非法的状态层用例（spec「编排日期范围」）。
 */

const { apiMock } = vi.hoisted(() => ({
  apiMock: {
    getComposeCandidates: vi.fn(),
    createManualLane: vi.fn(),
    embedQuery: vi.fn(),
  },
}))

const { reportsMock } = vi.hoisted(() => ({
  reportsMock: {
    listBoardTopics: vi.fn(),
    updateTopic: vi.fn(),
  },
}))

vi.mock('~/api/persistentTopics', () => ({ usePersistentTopicsApi: () => apiMock }))
vi.mock('~/api/dailyReports', () => ({ useDailyReportsApi: () => reportsMock }))

const WINDOW = { from: '2026-10-01', to: '2026-10-30', days: 30, latestCompletedDate: '2026-10-30' }

function candidate(id: string, periodDate: string, clusterLabel: string, embedding: number[], topicId?: string, status?: string) {
  return {
    id,
    reportId: '1',
    periodDate,
    clusterLabel,
    embedding,
    persistentTopicId: topicId,
    persistentTopic: topicId ? { id: topicId, label: '同名', status: status ?? 'candidate' } : undefined,
  }
}

const SECTIONS = [
  candidate('1', '2026-10-27', '候选一', [1, 0], '1984'),
  candidate('2', '2026-10-28', '候选二', [0.99, 0.1], '1984'),
  candidate('3', '2026-10-30', '候选三', [0.98, 0.2], '1984'),
  candidate('4', '2026-10-26', '散料近邻', [0.97, 0.15]),
  candidate('5', '2026-10-27', '已有泳道', [1, 0], '2000', 'active'),
  candidate('6', '2026-10-29', '散料远点', [0, 1]),
]

const CHAINS = [{
  key: '1984:1',
  topicId: '1984',
  topicLabel: '候选泳道 A',
  topicStatus: 'candidate',
  sectionIds: ['1', '2', '3'],
  firstDate: '2026-10-27',
  lastDate: '2026-10-30',
  spanDays: 3,
}]

function response(overrides: Partial<ComposeCandidatesData> = {}): ComposeCandidatesData {
  return {
    sections: SECTIONS,
    relations: [],
    candidateChains: CHAINS,
    matchThreshold: 0.3,
    window: WINDOW,
    ...overrides,
  }
}

interface Harness {
  compose: ReturnType<typeof useLaneCompose>
  confirm: ReturnType<typeof vi.fn>
}

/** 服务端管理名单（snake_case 原样，服务端可见性已按累计命中门槛过滤）。 */
const HISTORY_TOPICS = [
  {
    id: 1984,
    semantic_board_id: 1974,
    label: '候选泳道 A',
    description: '',
    status: 'candidate',
    first_seen_date: '2026-10-01',
    last_seen_date: '2026-10-30',
    hit_count: 5,
    consecutive_hits: 5,
    section_count: 3,
    color: '#abc',
    can_activate: true,
  },
  {
    id: 1985,
    semantic_board_id: 1974,
    label: '未达门槛话题',
    description: '',
    status: 'candidate',
    first_seen_date: '2026-10-05',
    last_seen_date: '2026-10-12',
    hit_count: 1,
    consecutive_hits: 1,
    section_count: 1,
    color: '#def',
    can_activate: false,
  },
  {
    id: 2000,
    semantic_board_id: 1974,
    label: '已有泳道',
    description: '',
    status: 'active',
    first_seen_date: '2026-09-01',
    last_seen_date: '2026-10-27',
    hit_count: 9,
    consecutive_hits: 2,
    section_count: 4,
    color: '#123',
    can_activate: false,
  },
]

async function setup(confirmResult = true): Promise<Harness> {
  const confirm = vi.fn(async () => confirmResult)
  const compose = useLaneCompose({
    boardId: ref(1974),
    requestConfirm: confirm as never,
    onSaved: vi.fn(),
  })
  await compose.load()
  return { compose, confirm }
}

beforeEach(() => {
  apiMock.getComposeCandidates.mockReset().mockResolvedValue({ success: true, data: response() })
  apiMock.createManualLane.mockReset().mockResolvedValue({
    success: true,
    data: { topic: { id: '300', label: '新泳道', status: 'active', source: 'manual' }, skipped: [] },
  })
  reportsMock.listBoardTopics.mockReset().mockResolvedValue({ success: true, data: { topics: HISTORY_TOPICS } })
  reportsMock.updateTopic.mockReset().mockResolvedValue({
    success: true,
    data: { id: 1984, semantic_board_id: 1974, label: '候选泳道 A', description: '', status: 'active', first_seen_date: '2026-10-01', last_seen_date: '2026-10-30', hit_count: 5, consecutive_hits: 5 },
  })
})

describe('useLaneCompose · 选择状态（S1–S6）', () => {
  it('S1 空组：unchecked（不是 indeterminate）', async () => {
    const { compose } = await setup()
    expect(compose.chainStates.value['1984:1']).toEqual({ count: 0, total: 3, checked: false, mixed: false })
  })

  it('S2 全选：checked', async () => {
    const { compose } = await setup()
    compose.setChainSelected('1984:1', true)
    expect(compose.chainStates.value['1984:1']!.checked).toBe(true)
    expect(compose.chainStates.value['1984:1']!.mixed).toBe(false)
  })

  it('S3 半选：indeterminate（逐个取消成员后可见）', async () => {
    const { compose } = await setup()
    compose.setChainSelected('1984:1', true)
    compose.toggleMember('2', false)
    const state = compose.chainStates.value['1984:1']!
    expect(state.count).toBe(2)
    expect(state.mixed).toBe(true)
    expect(state.checked).toBe(false)
  })

  it('S4 整组补齐：与组外人工补选合并去重，不截条数', async () => {
    const { compose } = await setup()
    compose.toggleMember('6', true)
    compose.setChainSelected('1984:1', true)
    expect(compose.memberCount.value).toBe(4)
    expect([...compose.selectedIds.value].sort()).toEqual(['1', '2', '3', '6'])
    // 再点一次全选标题 = 取消该组（不是幂等追加），组外补选保留
    compose.toggleChain('1984:1')
    expect([...compose.selectedIds.value]).toEqual(['6'])
  })

  it('S5 整组补齐不自动补相似推荐', async () => {
    const { compose } = await setup()
    compose.setChainSelected('1984:1', true)
    expect(compose.selectedIds.value.has('4')).toBe(false)
    // 推荐里仍然有它（待选，不是已选）
    expect(compose.recommendations.value.unassigned.map(r => r.section.id)).toContain('4')
  })

  it('S6 取消整组：只撤组内成员，组外人工补选保留', async () => {
    const { compose } = await setup()
    compose.toggleMember('6', true)
    compose.setChainSelected('1984:1', true)
    compose.cancelChain('1984:1')
    expect([...compose.selectedIds.value]).toEqual(['6'])
  })

  it('点候选标题：未全选一次补齐，全选后再点取消该组', async () => {
    const { compose } = await setup()
    compose.toggleChain('1984:1')
    expect(compose.memberCount.value).toBe(3)
    compose.toggleChain('1984:1')
    expect(compose.memberCount.value).toBe(0)
  })
})

describe('useLaneCompose · 命名（S7–S9）', () => {
  it('S7 自动名跟随来源组：取消该组且名字未被人工改写时一并清空', async () => {
    const { compose } = await setup()
    compose.setChainSelected('1984:1', true)
    expect(compose.laneName.value).toBe('候选泳道 A')
    compose.cancelChain('1984:1')
    expect(compose.laneName.value).toBe('')
  })

  it('S8 人工名保护：人工改名后取消组 / 再选组都不被覆盖', async () => {
    const { compose } = await setup()
    compose.setLaneName('我手动起的名字')
    compose.setChainSelected('1984:1', true)
    expect(compose.laneName.value).toBe('我手动起的名字')
    compose.cancelChain('1984:1')
    expect(compose.laneName.value).toBe('我手动起的名字')
  })

  it('S9 清空已选：成员清空、手写名保留', async () => {
    const { compose } = await setup()
    compose.setLaneName('我手动起的名字')
    compose.setChainSelected('1984:1', true)
    compose.clearSelection()
    expect(compose.memberCount.value).toBe(0)
    expect(compose.laneName.value).toBe('我手动起的名字')
  })
})

describe('useLaneCompose · 只读边界（S10）', () => {
  it('S10 本编排态不改任何话题状态：加载/选择全程不触发写接口', async () => {
    const { compose } = await setup()
    compose.setChainSelected('1984:1', true)
    expect(apiMock.createManualLane).not.toHaveBeenCalled()
    // 编排态只调用取数接口；状态变更（转正/归档）不在这条路径上
    expect(Object.keys(apiMock)).not.toContain('updateTopic')
  })
})

describe('useLaneCompose · 质心推荐随增删重算', () => {
  it('焦点不影响质心；加入推荐项后按新成员均值重算', async () => {
    const { compose } = await setup()
    compose.setChainSelected('1984:1', true)
    const before = compose.recommendations.value.center!
    compose.setFocus('4')
    expect(compose.recommendations.value.center).toEqual(before)
    expect(compose.selectedIds.value.has('4')).toBe(false)

    compose.toggleMember('4', true)
    const after = compose.recommendations.value.center!
    expect(after).not.toEqual(before)
    expect(compose.recommendations.value.unassigned.map(r => r.section.id)).not.toContain('4')
  })
})

describe('useLaneCompose · 日期范围与草稿确认', () => {
  it('无草稿切窗口：直接取数并生效', async () => {
    const { compose, confirm } = await setup()
    await compose.setQuickDays(7)
    expect(confirm).not.toHaveBeenCalled()
    expect(apiMock.getComposeCandidates).toHaveBeenLastCalledWith(1974, { mode: 7, days: 7 })
    expect(compose.appliedWindow.value.mode).toBe(7)
  })

  it('有草稿切窗口：先确认；确认才清草稿并重算', async () => {
    const { compose, confirm } = await setup(true)
    compose.setChainSelected('1984:1', true)
    await compose.setQuickDays(7)
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(confirm.mock.calls[0]![0]).toMatchObject({ kind: 'window' })
    expect(compose.memberCount.value).toBe(0)
    expect(compose.laneName.value).toBe('')
  })

  it('取消确认：恢复原控件、成员与推荐', async () => {
    const { compose } = await setup(false)
    compose.setChainSelected('1984:1', true)
    const center = compose.recommendations.value.center
    await compose.setQuickDays(7)
    expect(compose.appliedWindow.value.mode).toBe(30)
    expect(compose.controlMode.value).toBe(30)
    expect(compose.memberCount.value).toBe(3)
    expect(compose.recommendations.value.center).toEqual(center)
  })

  it('日期非法（倒置）：报错、保留旧范围与草稿、控件恢复原值', async () => {
    const { compose } = await setup()
    compose.setChainSelected('1984:1', true)
    compose.startCustomRange()
    compose.controlFrom.value = '2026-10-30'
    compose.controlTo.value = '2026-10-01'
    await compose.applyCustomRange()
    expect(compose.dateError.value).toBeTruthy()
    expect(compose.appliedWindow.value.from).toBe('2026-10-01')
    expect(compose.appliedWindow.value.to).toBe('2026-10-30')
    expect(compose.memberCount.value).toBe(3)
    expect(compose.controlFrom.value).toBe('2026-10-01')
    expect(compose.controlTo.value).toBe('2026-10-30')
  })

  it('日期非法（超 90 天）：报错且不截断成 90 天', async () => {
    const { compose } = await setup()
    compose.startCustomRange()
    compose.controlFrom.value = '2026-01-01'
    compose.controlTo.value = '2026-10-30'
    await compose.applyCustomRange()
    expect(compose.dateError.value).toContain('90')
    expect(apiMock.getComposeCandidates).toHaveBeenCalledTimes(1)
  })

  it('日期非法（窗外起点）：报错并保留旧范围', async () => {
    const { compose } = await setup()
    compose.startCustomRange()
    compose.controlFrom.value = '2026-01-01'
    compose.controlTo.value = '2026-01-20'
    await compose.applyCustomRange()
    expect(compose.dateError.value).toBeTruthy()
    expect(compose.appliedWindow.value.from).toBe('2026-10-01')
  })

  it('合法自选范围生效：请求带 from/to，窗口回显服务端值', async () => {
    const { compose } = await setup()
    apiMock.getComposeCandidates.mockResolvedValue({
      success: true,
      data: response({ window: { from: '2026-10-27', to: '2026-10-30', days: 4, latestCompletedDate: '2026-10-30' } }),
    })
    compose.startCustomRange()
    compose.controlFrom.value = '2026-10-27'
    compose.controlTo.value = '2026-10-30'
    await compose.applyCustomRange()
    expect(apiMock.getComposeCandidates).toHaveBeenLastCalledWith(1974, { mode: 'custom', from: '2026-10-27', to: '2026-10-30' })
    expect(compose.appliedWindow.value.from).toBe('2026-10-27')
    expect(compose.dates.value).toEqual(['2026-10-27', '2026-10-28', '2026-10-29', '2026-10-30'])
  })
})

describe('useLaneCompose · 保存与取消', () => {
  it('名称空白：拒绝创建并保留成员', async () => {
    const { compose } = await setup()
    compose.setChainSelected('1984:1', true)
    const ok = await compose.save('   ')
    expect(ok).toBe(false)
    expect(compose.nameError.value).toBeTruthy()
    expect(apiMock.createManualLane).not.toHaveBeenCalled()
    expect(compose.memberCount.value).toBe(3)
  })

  it('保存成功：走既有 manual 端点并回调 onSaved', async () => {
    const { compose } = await setup()
    compose.setChainSelected('1984:1', true)
    const ok = await compose.save('新泳道')
    expect(ok).toBe(true)
    expect(apiMock.createManualLane).toHaveBeenCalledWith(1974, '新泳道', ['1', '2', '3'])
  })

  it('取消本次：有草稿先确认；取消确认保留草稿', async () => {
    const { compose } = await setup(false)
    compose.setChainSelected('1984:1', true)
    const cancelled = await compose.cancel()
    expect(cancelled).toBe(false)
    expect(compose.memberCount.value).toBe(3)
  })

  it('取消本次：确认后清本地草稿', async () => {
    const { compose } = await setup(true)
    compose.setChainSelected('1984:1', true)
    const cancelled = await compose.cancel()
    expect(cancelled).toBe(true)
    expect(compose.memberCount.value).toBe(0)
    expect(compose.laneName.value).toBe('')
  })
})

describe('useLaneCompose · 历史待启用话题（管理名单）', () => {
  it('H1 名单加载：snake_case 在 composable 层归一为 camelCase，可见性沿服务端（前端不自行放宽）', async () => {
    const { compose } = await setup()
    const ok = await compose.loadHistoryTopics()
    expect(ok).toBe(true)
    expect(reportsMock.listBoardTopics).toHaveBeenCalledWith(1974)
    expect(compose.historyTopics.value.map(t => t.id)).toEqual(['1984', '1985', '2000'])
    const first = compose.historyTopics.value[0]!
    expect(first.label).toBe('候选泳道 A')
    expect(first.hitCount).toBe(5)
    expect(first.lastSeenDate).toBe('2026-10-30')
    expect(first.sectionCount).toBe(3)
    expect(first.canActivate).toBe(true)
    // 未达门槛者：资格沿服务端 can_activate（前端不自造门槛，也不改写它）
    const blocked = compose.historyTopics.value[1]!
    expect(blocked.canActivate).toBe(false)
    expect(blocked.activateBlockReason).toContain('累计命中未达管理门槛')
  })

  it('H2 名单加载失败：报错并保留当前状态', async () => {
    const { compose } = await setup()
    reportsMock.listBoardTopics.mockResolvedValueOnce({ success: false, error: '名单接口炸了' })
    const ok = await compose.loadHistoryTopics()
    expect(ok).toBe(false)
    expect(compose.historyError.value).toBe('名单接口炸了')
  })

  it('H3 按名过滤：纯前端过滤，不调 AI / 不调 embed', async () => {
    const { compose } = await setup()
    await compose.loadHistoryTopics()
    apiMock.embedQuery.mockClear()
    compose.historyQuery.value = '门槛'
    expect(compose.filteredHistoryTopics.value.map(t => t.id)).toEqual(['1985'])
    compose.historyQuery.value = ' 泳道 a '
    expect(compose.filteredHistoryTopics.value.map(t => t.id)).toEqual(['1984'])
    expect(apiMock.embedQuery).not.toHaveBeenCalled()
  })

  it('H4 带入草稿：只带当前窗口内的 owned 成员（不补相似、不截条）', async () => {
    const { compose } = await setup()
    await compose.loadHistoryTopics()
    const outcome = compose.adoptHistoryTopic('1984')
    expect(outcome).not.toBeNull()
    expect(outcome!.ownedCount).toBe(3)
    expect(outcome!.addedCount).toBe(3)
    // 只带 1984 自己的成员：不带散料（4 / 6），也不带别的話題的成员（5）
    expect([...compose.selectedIds.value].sort()).toEqual(['1', '2', '3'])
  })

  it('H5 带入草稿：未手写名字 → 预填话题名并绑定来源；手写过 → 保留不覆盖', async () => {
    const { compose } = await setup()
    await compose.loadHistoryTopics()
    const outcome = compose.adoptHistoryTopic('1984')
    expect(outcome!.prefilledName).toBe(true)
    expect(outcome!.keptManualName).toBe(false)
    expect(compose.laneName.value).toBe('候选泳道 A')

    const { compose: manual } = await setup()
    await manual.loadHistoryTopics()
    manual.setLaneName('我自己写的名字')
    const manualOutcome = manual.adoptHistoryTopic('1984')
    expect(manualOutcome!.keptManualName).toBe(true)
    expect(manualOutcome!.prefilledName).toBe(false)
    expect(manual.laneName.value).toBe('我自己写的名字')
  })

  it('H6 历史来源被清空：自动名一并清空；人工改写过则保留', async () => {
    const { compose } = await setup()
    await compose.loadHistoryTopics()
    compose.adoptHistoryTopic('1984')
    expect(compose.laneName.value).toBe('候选泳道 A')
    // 组外成员先占位，避免「全部清空」分支提前清名
    compose.toggleMember('6', true)
    compose.toggleMember('1', false)
    compose.toggleMember('2', false)
    compose.toggleMember('3', false)
    expect(compose.laneName.value).toBe('')

    const { compose: manual } = await setup()
    await manual.loadHistoryTopics()
    manual.adoptHistoryTopic('1984')
    manual.setLaneName('我自己写的名字')
    manual.toggleMember('6', true)
    manual.toggleMember('1', false)
    manual.toggleMember('2', false)
    manual.toggleMember('3', false)
    expect(manual.laneName.value).toBe('我自己写的名字')
  })

  it('H7 直接启用原话题：资格沿服务端 can_activate，不达门槛不发请求', async () => {
    const { compose } = await setup()
    await compose.loadHistoryTopics()
    const denied = await compose.activateHistoryTopic('1985')
    expect(denied.ok).toBe(false)
    expect(denied.message).toContain('累计命中未达管理门槛')
    expect(reportsMock.updateTopic).not.toHaveBeenCalled()
  })

  it('H8 直接启用原话题：走既有 updateTopic(status=active)，不改全局命中与质心', async () => {
    const { compose } = await setup()
    await compose.loadHistoryTopics()
    const ok = await compose.activateHistoryTopic('1984')
    expect(ok.ok).toBe(true)
    expect(reportsMock.updateTopic).toHaveBeenCalledWith(1984, { status: 'active' })
    // 不碰编排态的保存接口（不新建、不重算质心）
    expect(apiMock.createManualLane).not.toHaveBeenCalled()
  })

  it('H9 直接启用失败：报错并保留当前状态（不假装成功）', async () => {
    const { compose } = await setup()
    await compose.loadHistoryTopics()
    reportsMock.updateTopic.mockResolvedValueOnce({ success: false, error: '启用被拒' })
    const res = await compose.activateHistoryTopic('1984')
    expect(res.ok).toBe(false)
    expect(res.message).toBe('启用被拒')
    expect(compose.historyError.value).toBe('启用被拒')
    expect(compose.memberCount.value).toBe(0)
  })
})
