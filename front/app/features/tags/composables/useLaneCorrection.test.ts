import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useLaneCorrection } from './useLaneCorrection'
import { ref } from 'vue'

/**
 * 泳道修正工作面状态机用例（test-cases 故事 A / B / D 的取数与写路径落点）。
 *
 * 判据（机械断言）：
 * - 取数：窗口内成员、无归属候选、当前首义来源与节点集；
 * - 移除：写 `{topic_id: null}`（直接解除归属，不选去处）+ 节点进「已移出（本次）」；
 * - 加回：写 `{topic_id: 本泳道}` + toast「已加回」；
 * - 定首义：写 `{section_ids}` + 来源变「人工（选定的 N 个节点）」+ toast 含「次日」；
 * - 选择池边界：窗口内本泳道节点 + **全部更早成员**（数据全量、展示分层）；
 *   未属本话题的候选不入池；
 * - 自动首义无节点集 → 无「更早」节点（不虚构）。
 */

const { persistentMock } = vi.hoisted(() => ({
  persistentMock: { getComposeCandidates: vi.fn() },
}))

const { reportsMock } = vi.hoisted(() => ({
  reportsMock: {
    getTopicLifeline: vi.fn(),
    getTopicAnchor: vi.fn(),
    setSectionAssignment: vi.fn(),
    updateTopicAnchor: vi.fn(),
    updateTopic: vi.fn(),
  },
}))

const { notifyMock } = vi.hoisted(() => ({
  notifyMock: { success: vi.fn(), error: vi.fn(), warn: vi.fn() },
}))

vi.mock('~/api/persistentTopics', () => ({ usePersistentTopicsApi: () => persistentMock }))
vi.mock('~/api/dailyReports', () => ({ useDailyReportsApi: () => reportsMock }))
vi.mock('~/composables/useNotify', () => ({ useNotify: () => notifyMock }))

const { dynamicsMock } = vi.hoisted(() => ({
  dynamicsMock: { loadLaneDynamics: vi.fn() },
}))

vi.mock('~/api/laneDynamicsWindow', () => ({ useLaneDynamicsWindowApi: () => dynamicsMock }))

function section(
  id: string,
  date: string,
  label: string,
  topicId?: string,
  embedding: number[] = [1, 0],
  owner?: { label: string, status: string },
) {
  return {
    id,
    reportId: '1',
    periodDate: date,
    clusterLabel: label,
    embedding,
    persistentTopicId: topicId,
    persistentTopic: topicId ? { id: topicId, label: owner?.label ?? 'AI 前沿', status: owner?.status ?? 'active' } : undefined,
  }
}

/** 窗口内：m1 / m2 属本泳道，c1 无归属，r1 属别的泳道。 */
const WINDOW_SECTIONS = [
  section('101', '2026-11-10', '成员一', '7'),
  section('102', '2026-11-12', '成员二', '7', [0.9, 0.1]),
  section('103', '2026-11-11', '候选节点', undefined, [0.8, 0.2]),
  section('104', '2026-11-11', '别的泳道节点', '9', [0.7, 0.3]),
]

/** 泳道全量成员：含窗外节点 f1（也是当前首义集的一员）。 */
const LIFELINE_SECTIONS = [
  { id: 101, period_date: '2026-11-10', cluster_label: '成员一', topic_match_distance: 0.09 },
  { id: 102, period_date: '2026-11-12', cluster_label: '成员二', topic_match_distance: 0.16 },
  { id: 109, period_date: '2026-10-21', cluster_label: '窗外首义节点', topic_match_distance: 0.31 },
  { id: 150, period_date: '2026-10-05', cluster_label: '更早的其它历史节点', topic_match_distance: 0.4 },
]

/** 后端「更早成员」全量口径（窗口外的本话题成员，升序）。 */
const EARLY_MEMBERS = [
  { id: '109', reportId: '2', periodDate: '2026-10-21', clusterLabel: '窗外首义节点', persistentTopicId: '7', topicMatchDistance: 0.31, embedding: [0.9, 0.1] },
  { id: '150', reportId: '3', periodDate: '2026-10-05', clusterLabel: '更早的其它历史节点', persistentTopicId: '7', topicMatchDistance: 0.4, embedding: [0.9, 0.1] },
]

function makeComposable() {
  const laneLabel = ref('AI 前沿')
  const state = useLaneCorrection({
    boardId: ref(3),
    topicId: ref(7),
    laneLabel,
    onRenamed: () => { /* 宿主刷新名单 */ },
  })
  return { state, laneLabel }
}

beforeEach(() => {
  persistentMock.getComposeCandidates.mockReset()
  reportsMock.getTopicLifeline.mockReset()
  reportsMock.getTopicAnchor.mockReset()
  reportsMock.setSectionAssignment.mockReset()
  reportsMock.updateTopicAnchor.mockReset()
  reportsMock.updateTopic.mockReset()
  notifyMock.success.mockReset()
  notifyMock.error.mockReset()

  persistentMock.getComposeCandidates.mockResolvedValue({
    success: true,
    data: {
      sections: WINDOW_SECTIONS,
      relations: [],
      candidateChains: [],
      matchThreshold: 0.3,
      window: { from: '2026-11-10', to: '2026-11-13', days: 4 },
    },
  })
  reportsMock.getTopicLifeline.mockResolvedValue({ success: true, data: { sections: LIFELINE_SECTIONS, relations: [] } })
  reportsMock.getTopicAnchor.mockResolvedValue({
    success: true,
    data: { topic_id: 7, source: 'auto', section_ids: [], override_count: 0, effective_from: '' },
  })
  reportsMock.setSectionAssignment.mockResolvedValue({ success: true, data: { section_id: 1, topic_id: null } })
  reportsMock.updateTopicAnchor.mockResolvedValue({ success: true, data: null })
  reportsMock.updateTopic.mockResolvedValue({ success: true, data: null })
  dynamicsMock.loadLaneDynamics.mockReset().mockResolvedValue({ success: false, error: 'not seeded' })
})

describe('useLaneCorrection · 取数与派生', () => {
  it('成员按窗口内归属计数，候选只取无归属节点，其它泳道只读', async () => {
    const { state } = makeComposable()
    await state.load()
    expect(state.memberNodes.value.map(node => node.id)).toEqual(['101', '102'])
    expect(state.candidateNodes.value.map(node => node.id)).toEqual(['103'])
    expect(state.readonlyNodes.value.map(node => node.id)).toEqual(['104'])
    expect(state.emptyKind.value).toBeNull()
  })

  it('候选口径：无归属 + 属 candidate（动态）泳道的节点进候选；属 active 泳道的只读', async () => {
    persistentMock.getComposeCandidates.mockResolvedValue({
      success: true,
      data: {
        sections: [
          section('101', '2026-11-10', '成员一', '7'),
          section('102', '2026-11-12', '成员二', '7', [0.9, 0.1]),
          section('103', '2026-11-11', '无归属候选', undefined, [0.8, 0.2]),
          section('104', '2026-11-11', '动态泳道节点', '9', [0.7, 0.3], { label: '动态泳道', status: 'candidate' }),
          section('105', '2026-11-11', '正式泳道节点', '11', [0.6, 0.4], { label: '正式泳道', status: 'active' }),
        ],
        relations: [],
        candidateChains: [],
        matchThreshold: 0.3,
        window: { from: '2026-11-10', to: '2026-11-13', days: 4 },
      },
    })
    const { state } = makeComposable()
    await state.load()

    // 候选 = 无归属（103）+ 动态泳道（104）；正式泳道（105）不作候选。
    expect(state.candidateNodes.value.map(node => node.id).sort()).toEqual(['103', '104'])
    // 正式泳道节点仍作只读参照；动态泳道节点不作只读（它已是候选）。
    expect(state.readonlyNodes.value.map(node => node.id)).toEqual(['105'])
    // 候选节点带来源泳道名（改挂来源要看得见）。
    expect(state.candidateNodes.value.find(n => n.id === '104')!.topicLabel).toBe('动态泳道')
  })

  it('成员接近度 = 1 − topic_match_distance；候选接近度按质心实时算', async () => {
    const { state } = makeComposable()
    await state.load()
    expect(state.memberNodes.value[0]!.proximity).toBeCloseTo(0.91, 5)
    const candidate = state.candidateNodes.value[0]!
    expect(candidate.proximity).not.toBeNull()
    expect(candidate.proximity!).toBeGreaterThan(0)
  })

  it('接近度两把尺子可区分：成员 = 归属快照，关联推荐 = 实时估算', async () => {
    persistentMock.getComposeCandidates.mockResolvedValue({
      success: true,
      data: {
        sections: WINDOW_SECTIONS,
        relations: [],
        candidateChains: [],
        matchThreshold: 0.3,
        window: { from: '2026-11-10', to: '2026-11-13', days: 4 },
        earlyMembers: EARLY_MEMBERS,
      },
    })
    const { state } = makeComposable()
    await state.load()
    // 成员行：归属快照（topic_match_distance 换算，入库即冻结）。
    expect(state.memberNodes.value[0]!.proximityKind).toBe('snapshot')
    expect(state.memberNodes.value[0]!.proximity).toBeCloseTo(0.91, 5)
    // 更早成员：同样是快照。
    expect(state.earlierMemberNodes.value[0]!.proximityKind).toBe('snapshot')
    // 关联推荐：对当前成员质心的实时估算。
    expect(state.candidateNodes.value[0]!.proximityKind).toBe('estimate')
  })

  it('「全部 M」唯一来源 = 成员 section 总数（窗口内 + 窗口外）', async () => {
    persistentMock.getComposeCandidates.mockResolvedValue({
      success: true,
      data: {
        sections: WINDOW_SECTIONS,
        relations: [],
        candidateChains: [],
        matchThreshold: 0.3,
        window: { from: '2026-11-10', to: '2026-11-13', days: 4 },
        earlyMembers: EARLY_MEMBERS,
      },
    })
    const { state } = makeComposable()
    await state.load()
    // 窗口内成员 2 + 窗口外成员 2 = 全部 4（左栏徽标与首义面板同数）。
    expect(state.memberNodes.value).toHaveLength(2)
    expect(state.earlierMemberNodes.value).toHaveLength(2)
    expect(state.totalMemberCount.value).toBe(4)
  })

  it('旧后端（无全量口径）→ 「全部 M」为 null（徽标回退只显示窗口数，不造假）', async () => {
    const { state } = makeComposable()
    await state.load()
    expect(state.totalMemberCount.value).toBeNull()
  })

  it('首次加载失败 → 错误态，不呈现业务空态', async () => {
    persistentMock.getComposeCandidates.mockResolvedValue({ success: false, error: '泳道成员读取失败' })
    const { state } = makeComposable()
    await state.load()
    expect(state.loadError.value).toBe('泳道成员读取失败')
    expect(state.emptyKind.value).toBeNull()
  })
})

describe('useLaneCorrection · 移除与加回（故事 B）', () => {
  it('取消勾选 = 直接解除归属（写 topic_id: null，不选去处），节点进「已移出（本次）」', async () => {
    const { state } = makeComposable()
    await state.load()
    const ok = await state.removeMember('101')
    expect(ok).toBe(true)
    expect(reportsMock.setSectionAssignment).toHaveBeenCalledWith(101, null)
    expect(state.memberNodes.value.map(node => node.id)).toEqual(['102'])
    expect(state.removedNodes.value.map(node => node.id)).toEqual(['101'])
    expect(notifyMock.success.mock.calls[0]![0]).toContain('不再归属任何泳道')
    expect(notifyMock.success.mock.calls[0]![0]).toContain('仍可再参与候选')
  })

  it('移除节点 → 首义范围选择集同步剔除（按钮计数不偏大，不带非本话题节点）', async () => {
    const { state } = makeComposable()
    await state.load()
    state.toggleAnchorNode('101')
    state.toggleAnchorNode('102')
    expect(state.anchorSelection.value.size).toBe(2)
    await state.removeMember('101')
    expect(state.anchorSelection.value.has('101')).toBe(false)
    expect(state.anchorSelection.value.has('102')).toBe(true)
    expect(state.anchorSelection.value.size).toBe(1)
    expect(state.anchorPool.value.map(node => node.id)).not.toContain('101')
  })

  it('加回：写 topic_id: 本泳道，节点回到成员列表', async () => {
    const { state } = makeComposable()
    await state.load()
    await state.removeMember('101')
    const ok = await state.restoreMember('101')
    expect(ok).toBe(true)
    expect(reportsMock.setSectionAssignment).toHaveBeenLastCalledWith(101, 7)
    expect(state.memberNodes.value.map(node => node.id)).toContain('101')
    expect(state.removedNodes.value).toHaveLength(0)
    expect(notifyMock.success.mock.calls.at(-1)![0]).toContain('已加回')
  })

  it('写失败 → 保留本轮未提交改动（成员集合不变），并报错', async () => {
    reportsMock.setSectionAssignment.mockResolvedValue({ success: false, error: '移除失败' })
    const { state } = makeComposable()
    await state.load()
    const ok = await state.removeMember('101')
    expect(ok).toBe(false)
    expect(state.memberNodes.value.map(node => node.id)).toContain('101')
    expect(state.actionError.value).toBe('移除失败')
  })
})

describe('useLaneCorrection · 首义范围（故事 A）', () => {
  it('提交选定节点 → 写 section_ids，来源变「人工（选定的 N 个节点）」，toast 含「次日」', async () => {
    const { state } = makeComposable()
    await state.load()
    state.toggleAnchorNode('101')
    state.toggleAnchorNode('102')
    // 写成功后重读首义（纯读）会看到刚落库的 override → 来源转为人工、节点集为刚提交的两个
    reportsMock.getTopicAnchor.mockResolvedValue({
      success: true,
      data: {
        topic_id: 7,
        source: 'manual',
        section_ids: [101, 102],
        override_count: 1,
        effective_from: 'next_report',
        anchor_origin: { resolved: true, section_id: 101, title: '成员一', period_date: '2026-11-10' },
      },
    })
    const ok = await state.applyAnchor()
    expect(ok).toBe(true)
    expect(reportsMock.updateTopicAnchor).toHaveBeenCalledWith(7, [101, 102])
    expect(state.anchorSource.value).toBe('manual')
    expect(state.anchorSourceLabel.value).toBe('人工（选定的 2 个节点）')
    expect(notifyMock.success.mock.calls.at(-1)![0]).toContain('次日日报生效')
    // 写后重读一次（load 一次 + 写后一次），让只读块反映真实结果而不是写前的旧值
    expect(reportsMock.getTopicAnchor).toHaveBeenCalledTimes(2)
    expect(state.anchorOriginKind.value).toBe('resolved')
  })

  it('写成功后重读失败：不抹掉刚写入的写入结论，但也不伪造「可考」', async () => {
    const { state } = makeComposable()
    await state.load()
    state.toggleAnchorNode('101')
    reportsMock.getTopicAnchor.mockResolvedValue({ success: false, error: '读挂了' })
    const ok = await state.applyAnchor()
    expect(ok).toBe(true)
    expect(state.anchorSourceLabel.value).toBe('人工（选定的 1 个节点）')
    // 来源节点回到「读失败」态（保守），不得变成「创建时写入·来源不可考」
    expect(state.anchorOriginKind.value).toBe('error')
  })

  it('已有首义时先出画布内确认条（不叠弹窗）', async () => {
    reportsMock.getTopicAnchor.mockResolvedValue({
      success: true,
      data: { topic_id: 7, source: 'manual', section_ids: [109], override_count: 1, effective_from: 'next_report' },
    })
    const { state } = makeComposable()
    await state.load()
    state.toggleAnchorNode('101')
    state.requestAnchorApply()
    expect(state.anchorConfirmOpen.value).toBe(true)
    expect(reportsMock.updateTopicAnchor).not.toHaveBeenCalled()
    await state.applyAnchor()
    expect(state.anchorConfirmOpen.value).toBe(false)
    expect(reportsMock.updateTopicAnchor).toHaveBeenCalledWith(7, [101])
  })
})

describe('useLaneCorrection · 选择池边界（故事 D）', () => {
  it('自动首义无节点集 → 无「更早」节点（不虚构）', async () => {
    const { state } = makeComposable()
    await state.load()
    expect(state.anchorNodes.value).toHaveLength(0)
    expect(state.earlierAnchorNodes.value).toHaveLength(0)
    expect(state.anchorPool.value.map(node => node.id)).toEqual(['101', '102'])
  })

  it('人工首义集含窗外节点 → 进「更早」分组（日期如实），选择池 = 窗口内节点 + 当前首义集', async () => {
    reportsMock.getTopicAnchor.mockResolvedValue({
      success: true,
      data: { topic_id: 7, source: 'manual', section_ids: [109, 101], override_count: 1, effective_from: 'next_report' },
    })
    const { state } = makeComposable()
    await state.load()
    expect(state.earlierAnchorNodes.value.map(node => node.id)).toEqual(['109'])
    expect(state.earlierAnchorNodes.value[0]!.date).toBe('2026-10-21')
    expect(state.anchorPool.value.map(node => node.id).sort()).toEqual(['101', '102', '109'])
  })

  it('后端给了全量口径 → 更早成员进池（全部成员可选），计数 = 窗口 + 更早', async () => {
    persistentMock.getComposeCandidates.mockResolvedValue({
      success: true,
      data: {
        sections: WINDOW_SECTIONS,
        relations: [],
        candidateChains: [],
        matchThreshold: 0.3,
        window: { from: '2026-11-10', to: '2026-11-13', days: 4 },
        earlyMembers: EARLY_MEMBERS,
      },
    })
    const { state } = makeComposable()
    await state.load()

    // 更早成员 = 窗口外的本话题成员（接近度 = 1 − 锚定距离）。
    expect(state.earlierMemberNodes.value.map(node => node.id)).toEqual(['109', '150'])
    expect(state.earlierMemberNodes.value[0]!.proximity).toBeCloseTo(0.69, 5)
    expect(state.earlierMemberNodes.value[0]!.outsideWindow).toBe(true)

    // 选择池覆盖全部成员（窗口内 2 + 更早 2）；候选（103）不入池。
    expect(state.anchorPool.value.map(node => node.id).sort()).toEqual(['101', '102', '109', '150'])
    expect(state.anchorPool.value.map(node => node.id)).not.toContain('103')

    // 全量计数（徽标「窗口 N · 全部 M」的 M）。
    expect(state.totalMemberCount.value).toBe(4)
  })

  it('窗口内成员为 0 但更早成员有向量 → 质心仍可算，候选区非空（回归：质心空导致候选全空）', async () => {
    // 窗口内没有任何本话题成员；无归属候选 103 与质心同向（距离 0 ≤ 阈值）。
    const orphanWindow = WINDOW_SECTIONS.filter(section => section.id === '103')
    persistentMock.getComposeCandidates.mockResolvedValue({
      success: true,
      data: {
        sections: orphanWindow,
        relations: [],
        candidateChains: [],
        matchThreshold: 0.3,
        window: { from: '2026-11-10', to: '2026-11-13', days: 4 },
        earlyMembers: EARLY_MEMBERS,
      },
    })
    const { state } = makeComposable()
    await state.load()

    // 窗口内成员 0；质心由更早成员向量撑起；候选 103（无归属）按距离入推荐。
    expect(state.memberNodes.value).toHaveLength(0)
    expect(state.recommendations.value.map(node => node.id)).toContain('103')
  })

  it('后端未给全量口径（旧后端）→ 更早成员不可得，计数回退只显示窗口数', async () => {
    const { state } = makeComposable()
    await state.load()
    expect(state.earlyMembers.value).toBeNull()
    expect(state.totalMemberCount.value).toBeNull()
    expect(state.anchorPool.value.map(node => node.id).sort()).toEqual(['101', '102'])
  })

  it('更早的其它历史节点不在池内（未属本话题者不入池）', async () => {
    const { state } = makeComposable()
    await state.load()
    expect(state.anchorPool.value.map(node => node.id)).not.toContain('150')
  })
})

describe('useLaneCorrection · 泳道名就地编辑', () => {
  it('改名成功 → 写 label 并提示「泳道名已更新」', async () => {
    const { state, laneLabel } = makeComposable()
    await state.load()
    const ok = await state.commitRename('新名字')
    expect(ok).toBe(true)
    expect(reportsMock.updateTopic).toHaveBeenCalledWith(7, { label: '新名字' })
    expect(laneLabel.value).toBe('新名字')
    expect(notifyMock.success.mock.calls.at(-1)![0]).toBe('泳道名已更新')
  })

  it('空名拒绝（不发请求）', async () => {
    const { state } = makeComposable()
    await state.load()
    const ok = await state.commitRename('   ')
    expect(ok).toBe(false)
    expect(reportsMock.updateTopic).not.toHaveBeenCalled()
    expect(state.renameError.value).toBe('泳道名不能为空')
  })
})

describe('useLaneCorrection · 时间视窗与本泳道曲线（unify-lane-workbench 6.2/6.3）', () => {
  it('S3.1 模态本地范围：preview 只改显示；commit 才取候选（带 from/to/allMembersTopicId）', async () => {
    const { state } = makeComposable()
    await state.load()
    persistentMock.getComposeCandidates.mockClear()
    state.previewWindow({ start: '2026-11-11', end: '2026-11-12' })
    expect(state.windowRange.value).toEqual({ start: '2026-11-11', end: '2026-11-12' })
    expect(persistentMock.getComposeCandidates).not.toHaveBeenCalled() // preview 0 请求
    persistentMock.getComposeCandidates.mockResolvedValueOnce({
      success: true,
      data: {
        sections: WINDOW_SECTIONS,
        relations: [],
        candidateChains: [],
        matchThreshold: 0.3,
        window: { from: '2026-11-11', to: '2026-11-12', days: 2, latestCompletedDate: '2026-11-13' },
      },
    })
    const ok = await state.commitWindow({ start: '2026-11-11', end: '2026-11-12' })
    expect(ok).toBe(true)
    expect(persistentMock.getComposeCandidates).toHaveBeenCalledWith(3, { from: '2026-11-11', to: '2026-11-12', allMembersTopicId: 7 })
    expect(state.windowFrom.value).toBe('2026-11-11')
    expect(state.windowRange.value).toEqual({ start: '2026-11-11', end: '2026-11-12' }) // 回显接管
  })

  it('非法范围（超 365 天）：行内报错并保旧（不静默截断）', async () => {
    const { state } = makeComposable()
    await state.load()
    const ok = await state.commitWindow({ start: '2025-11-13', end: '2026-11-13' }) // 366 天
    expect(ok).toBe(false)
    expect(state.windowError.value).toContain('365')
    expect(state.windowFrom.value).toBe('2026-11-10') // 保旧
    expect(state.windowRange.value).toEqual({ start: '2026-11-10', end: '2026-11-13' })
  })

  it('切档失败保旧：档位回退到生效值（不留在未生效范围）', async () => {
    const { state } = makeComposable()
    await state.load()
    persistentMock.getComposeCandidates.mockResolvedValueOnce({ success: false, error: '网络抖动' })
    const ok = await state.setWindow(30)
    expect(ok).toBe(false)
    expect(state.windowDays.value).toBe(7) // 保旧
  })

  it('S3.2 曲线只取目标泳道：anchor 定轴、断档为 null 不补 0、目标未返回为未知', async () => {
    const { state } = makeComposable()
    dynamicsMock.loadLaneDynamics.mockResolvedValueOnce({
      success: true,
      data: {
        window: {
          windowDays: 365,
          hasReports: true,
          anchorDate: '2026-11-13',
          lanes: [
            { topicId: '7', label: 'AI 前沿', timeline: [
              { date: '2026-11-12', clueCount: 3 },
              { date: '2026-11-13', clueCount: 0 },
            ] },
            { topicId: '9', label: '别的泳道', timeline: [{ date: '2026-11-13', clueCount: 99 }] },
          ],
          candidates: [],
        },
      },
    })
    const ok = await state.reloadCurve()
    expect(ok).toBe(true)
    expect(dynamicsMock.loadLaneDynamics).toHaveBeenCalledWith(3, 365)
    expect(state.curveTargetKnown.value).toBe(true)
    const series = state.curveSeries.value!
    // 轴长 365；末两天 3 / 已知 0；中间断档全部 null（别的泳道 99 不影响）
    expect(series).toHaveLength(365)
    expect(series.at(-2)).toEqual({ date: '2026-11-12', value: 3 })
    expect(series.at(-1)).toEqual({ date: '2026-11-13', value: 0 })
    expect(series.filter(p => p.value === null)).toHaveLength(363) // 断档 = 未知，不补 0
    expect(state.curveBounds.value).toEqual({ start: '2025-11-14', end: '2026-11-13' })
  })

  it('曲线目标泳道未出现在响应里：未知（不造全 0），画布/左栏仍可用', async () => {
    const { state } = makeComposable()
    dynamicsMock.loadLaneDynamics.mockResolvedValueOnce({
      success: true,
      data: { window: { windowDays: 365, hasReports: true, anchorDate: '2026-11-13', lanes: [], candidates: [] } },
    })
    await state.reloadCurve()
    expect(state.curveTargetKnown.value).toBe(false)
    expect(state.curveSeries.value).toBeNull()
    expect(state.curveError.value).toBeNull() // 未知 ≠ 错误
  })

  it('曲线失败保旧并可重试；写操作成功后自动重取', async () => {
    const { state } = makeComposable()
    dynamicsMock.loadLaneDynamics.mockResolvedValueOnce({
      success: true,
      data: { window: { windowDays: 365, hasReports: true, anchorDate: '2026-11-13', lanes: [
        { topicId: '7', label: 'AI 前沿', timeline: [{ date: '2026-11-13', clueCount: 2 }] },
      ], candidates: [] } },
    })
    await state.reloadCurve()
    expect(state.curveSeries.value!.at(-1)!.value).toBe(2)
    dynamicsMock.loadLaneDynamics.mockResolvedValueOnce({ success: false, error: '曲线失败' })
    await state.reloadCurve()
    expect(state.curveError.value).toBe('曲线失败')
    expect(state.curveSeries.value!.at(-1)!.value).toBe(2) // 保旧
    // 写操作成功 → 重取曲线（removeMember 成功后）
    dynamicsMock.loadLaneDynamics.mockResolvedValueOnce({
      success: true,
      data: { window: { windowDays: 365, hasReports: true, anchorDate: '2026-11-13', lanes: [
        { topicId: '7', label: 'AI 前沿', timeline: [{ date: '2026-11-13', clueCount: 5 }] },
      ], candidates: [] } },
    })
    await state.removeMember('101')
    await Promise.resolve() // fire-and-forget 重取落定
    await Promise.resolve()
    expect(dynamicsMock.loadLaneDynamics).toHaveBeenCalledTimes(3)
    expect(state.curveSeries.value!.at(-1)!.value).toBe(5)
  })

  it('5.1 候选响应 relations 保留（相关高亮共享入口的输入）', async () => {
    const { state } = makeComposable()
    persistentMock.getComposeCandidates.mockResolvedValueOnce({
      success: true,
      data: {
        sections: WINDOW_SECTIONS,
        relations: [
          { fromId: '103', toId: '104', distance: 0.1, relationType: 'similarity' },
        ],
        candidateChains: [],
        matchThreshold: 0.3,
        window: { from: '2026-11-10', to: '2026-11-13', days: 4 },
      },
    })
    await state.load()
    expect(state.relations.value).toEqual([
      { fromId: '103', toId: '104', distance: 0.1, relationType: 'similarity' },
    ])
  })

  it('B9 迟到候选响应丢弃：旧窗口的回来不改新清单', async () => {
    const { state } = makeComposable()
    await state.load()
    let resolveSlow!: (value: unknown) => void
    persistentMock.getComposeCandidates.mockImplementationOnce(() => new Promise(resolve => { resolveSlow = resolve }))
    const slow = state.setWindow(30)
    persistentMock.getComposeCandidates.mockResolvedValueOnce({
      success: true,
      data: {
        sections: [section('201', '2026-11-12', '新窗口节点', '7')],
        relations: [],
        candidateChains: [],
        matchThreshold: 0.3,
        window: { from: '2026-10-15', to: '2026-11-13', days: 30 },
      },
    })
    await state.setWindow(14)
    expect(state.memberNodes.value.map(n => n.id)).toEqual(['201'])
    resolveSlow({
      success: true,
      data: {
        sections: WINDOW_SECTIONS,
        relations: [],
        candidateChains: [],
        matchThreshold: 0.3,
        window: { from: '2026-11-10', to: '2026-11-13', days: 4 },
      },
    })
    await slow
    expect(state.memberNodes.value.map(n => n.id)).toEqual(['201']) // 旧响应被丢弃
  })
})

// ── 8.5：首义来源与滚动窗口的归一（align-lane-correction-workface R-3） ─────
//
// 本组是本 change 新增的归一逻辑（normalizeAnchorOrigin / normalizeCentroid）与
// 失败隔离路径（getTopicAnchor 失败不阻断写入口）的用例——此前只有 LaneAnchorScope
// 的纯 props 单测（测组件不测归一）。

describe('useLaneCorrection · 首义来源与滚动窗口归一（8.5）', () => {
  /** 后端 anchor 响应的最小骨架（source=auto，无节点集）。 */
  function anchorState(extra: Record<string, unknown> = {}) {
    return {
      success: true,
      data: { topic_id: 7, source: 'auto', section_ids: [], override_count: 0, effective_from: '', ...extra },
    }
  }

  it('8.5-a anchor_origin 可考：标题 + 日期如实透出（来源节点有值，不改造事实）', async () => {
    reportsMock.getTopicAnchor.mockResolvedValue(anchorState({
      source: 'manual',
      section_ids: [109],
      anchor_origin: { resolved: true, section_id: 109, title: '窗外首义节点', period_date: '2026-10-21' },
    }))
    const { state } = makeComposable()
    await state.load()
    expect(state.anchorOriginKind.value).toBe('resolved')
    expect(state.anchorOrigin.value).toEqual({
      resolved: true,
      sectionId: 109,
      title: '窗外首义节点',
      periodDate: '2026-10-21',
    })
    expect(state.anchorError.value).toBeNull()
  })

  it('8.5-b anchor_origin resolved=false：不可考且三个节点字段一律 null（不用推测节点顶替）', async () => {
    reportsMock.getTopicAnchor.mockResolvedValue(anchorState({
      anchor_origin: { resolved: false, section_id: null, title: null, period_date: null },
    }))
    const { state } = makeComposable()
    await state.load()
    expect(state.anchorOriginKind.value).toBe('unresolved')
    expect(state.anchorOrigin.value).toEqual({ resolved: false, sectionId: null, title: null, periodDate: null })
    // 不可考 ≠ 读失败：没有错误，也不弹重试。
    expect(state.anchorError.value).toBeNull()
  })

  it('8.5-c anchor_origin 整段缺失（旧后端）：按不可考处理，不造节点', async () => {
    reportsMock.getTopicAnchor.mockResolvedValue(anchorState())
    const { state } = makeComposable()
    await state.load()
    expect(state.anchorOriginKind.value).toBe('unresolved')
    expect(state.anchorOrigin.value).toBeNull()
    expect(state.anchorError.value).toBeNull()
  })

  it('8.5-d centroid 齐备：窗口 / α / 节点集合与明细按 camelCase 归一，degraded 原样透出', async () => {
    reportsMock.getTopicAnchor.mockResolvedValue(anchorState({
      centroid: {
        window_size: 30,
        anchor_weight: 0.4,
        section_ids: [11, 12],
        section_count: 2,
        members: [
          { section_id: 11, title: '滚动节点一', period_date: '2026-11-10' },
          { section_id: 12, title: '滚动节点二', period_date: '2026-11-11' },
        ],
        degraded: true,
      },
    }))
    const { state } = makeComposable()
    await state.load()
    const scope = state.centroidScope.value!
    expect(scope.windowSize).toBe(30)
    expect(scope.anchorWeight).toBeCloseTo(0.4, 6)
    expect(scope.sectionIds).toEqual([11, 12])
    expect(scope.sectionCount).toBe(2)
    expect(scope.members.map(m => m.sectionId)).toEqual([11, 12])
    expect(scope.members[0]!.title).toBe('滚动节点一')
    // degraded 是单一布尔标记，原样透出（文案分支在组件层，不在这里断言）。
    expect(scope.degraded).toBe(true)
    expect(state.centroidDegraded.value).toBe(true)
    expect(state.centroidSectionCount.value).toBe(2)
  })

  it('8.5-e centroid 缺失（旧后端）：按空处理不抛；section_count 缺席时按 section_ids.length 回退', async () => {
    reportsMock.getTopicAnchor.mockResolvedValue(anchorState())
    const { state } = makeComposable()
    await expect(state.load()).resolves.toBe(true)
    expect(state.centroidScope.value).toBeNull()
    expect(state.centroidDegraded.value).toBe(false)
    expect(state.centroidSectionCount.value).toBeNull() // 未知，不写 0

    // 齐备但无 section_count（旧后端）→ 回退到 section_ids 条数
    reportsMock.getTopicAnchor.mockResolvedValue(anchorState({
      centroid: {
        window_size: 14,
        anchor_weight: 0.5,
        section_ids: [21, 22, 23],
        members: [],
        degraded: false,
      },
    }))
    const second = makeComposable().state
    await second.load()
    expect(second.centroidScope.value!.sectionCount).toBe(3)
    expect(second.centroidSectionCount.value).toBe(3)
  })

  it('8.5-f getTopicAnchor reject：load() 仍成功（成员/候选照常落地），anchorError 有值且不输出「自动」断言', async () => {
    reportsMock.getTopicAnchor.mockRejectedValue(new Error('gateway timeout'))
    const { state } = makeComposable()
    await expect(state.load()).resolves.toBe(true)

    // 写入口不被只读块拖死：成员 / 候选照常落地。
    expect(state.memberNodes.value.map(node => node.id)).toEqual(['101', '102'])
    expect(state.candidateNodes.value.map(node => node.id)).toEqual(['103'])
    expect(state.loadError.value).toBeNull()

    expect(state.anchorError.value).toBe('gateway timeout')
    expect(state.anchorOriginKind.value).toBe('error')
    // 没读到就不能断言来源：标签为空串（不得回退成「自动」）。
    expect(state.anchorSourceLabel.value).toBe('')
    expect(state.anchorSourceLabel.value).not.toContain('自动')

    // 写入口实际可用：勾选 + 定首义照常发请求。
    state.toggleAnchorNode('101')
    await expect(state.applyAnchor()).resolves.toBe(true)
    expect(reportsMock.updateTopicAnchor).toHaveBeenCalledWith(7, [101])
  })

  it('8.5-g getTopicAnchor success:false：同样只落到只读块，不阻断 load()', async () => {
    reportsMock.getTopicAnchor.mockResolvedValue({ success: false, error: '话题不存在' })
    const { state } = makeComposable()
    await expect(state.load()).resolves.toBe(true)
    expect(state.anchorError.value).toBe('话题不存在')
    expect(state.anchorSourceLabel.value).toBe('')
    expect(state.memberNodes.value).toHaveLength(2)
  })
})
