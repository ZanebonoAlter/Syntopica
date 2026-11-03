/**
 * useInlineCompose — 就地编排态纯逻辑 composable 单测（inline-compose-lane 切片①）.
 *
 * 覆盖 section-lifecycle spec 三 Requirement 的纯逻辑 Scenario：
 *  - 就地勾选实时贴合度 + 渐进收敛（anchor 用剩余重算、全员 distance/tier 更新）
 *  - 离群标黄不自动删
 *  - moveOut 分类（勾走 active section → counts/originLabel/moveOutItems）
 *  - 聚类质量单卡实时（memberCount/meanDistance/outlierCount）
 *  - adopt 预填名 + 按 centroid 预勾 matchThreshold 内 unassigned + 截断上限
 *  - 语义搜索冷启动（queryVec 接管）+ 勾选后 anchor 接管 + 失败降级不阻断
 *  - save 流程（移出二次确认 / 校验 / 取消）
 *  - 候选侧边栏排序 + 激活
 *
 * mock 策略：只 mock 网络（usePersistentTopicsApi / useDailyReportsApi 的网络方法），
 * composeReport 纯函数全真跑（验证 composable 正确接线）。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick, ref } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { useInlineCompose } from './useInlineCompose'
import {
  aggregatePreview,
  cosineDistance,
  distanceTier,
  outlierFlags,
} from '~/features/tags/components/composeReport'
import type { ComposeCandidate } from '~/api/persistentTopics'
import type { BoardTopicListItem } from '~/api/dailyReports'

// ── 只 mock 网络方法；composeReport 不 mock（纯逻辑真跑） ─────────────────────
const api = vi.hoisted(() => ({
  getComposeCandidates: vi.fn(),
  createManualLane: vi.fn(),
  embedQuery: vi.fn(),
  updateTopic: vi.fn(),
}))

vi.mock('~/api/persistentTopics', () => ({
  usePersistentTopicsApi: () => ({
    getComposeCandidates: api.getComposeCandidates,
    createManualLane: api.createManualLane,
    embedQuery: api.embedQuery,
  }),
}))

vi.mock('~/api/dailyReports', () => ({
  useDailyReportsApi: () => ({ updateTopic: api.updateTopic }),
}))

// ── fixtures ───────────────────────────────────────────────────────────────

const THRESHOLD = 0.3

function cand(id: string, opts: Partial<ComposeCandidate> = {}): ComposeCandidate {
  return {
    id,
    periodDate: opts.periodDate ?? '2026-06-20T00:00:00Z',
    clusterLabel: opts.clusterLabel ?? `c${id}`,
    embedding: opts.embedding ?? [1, 0],
    persistentTopicId: opts.persistentTopicId,
    persistentTopic: opts.persistentTopic,
  }
}

function topic(id: number, over: Partial<BoardTopicListItem> = {}): BoardTopicListItem {
  return {
    id,
    semantic_board_id: 1,
    label: `话题${id}`,
    description: '',
    status: 'candidate',
    first_seen_date: '2026-06-01',
    last_seen_date: '2026-06-20',
    hit_count: 1,
    consecutive_hits: 1,
    section_count: 1,
    color: '#abc',
    can_activate: false,
    ...over,
  }
}

function defaultPool(): ComposeCandidate[] {
  return [
    cand('u1', { embedding: [1, 0], clusterLabel: '美伊博弈1' }),
    cand('u2', { embedding: [0.95, 0.31], clusterLabel: '美伊博弈2' }),
    cand('u3', { embedding: [0.8, 0.6], clusterLabel: '油价波动' }),
    cand('u4', { embedding: [0.5, 0.866], clusterLabel: '半导体管制' }),
    cand('m1', { embedding: [1, 0], clusterLabel: '中东战报A', persistentTopicId: '7', persistentTopic: { id: '7', label: '中东局势', status: 'active' } }),
    cand('m2', { embedding: [0.9, 0.4359], clusterLabel: '中东战报B', persistentTopicId: '7', persistentTopic: { id: '7', label: '中东局势', status: 'active' } }),
  ]
}

async function setup(
  poolOver: ComposeCandidate[] = defaultPool(),
  topics: BoardTopicListItem[] = [],
) {
  api.getComposeCandidates.mockResolvedValue({
    success: true,
    data: { sections: poolOver, matchThreshold: THRESHOLD },
  })
  api.createManualLane.mockResolvedValue({
    success: true,
    data: { topic: { id: 20, label: 'x', status: 'active', source: 'manual' }, skipped: [] },
  })
  api.embedQuery.mockResolvedValue({ success: true, data: { embedding: [1, 0] } })
  api.updateTopic.mockResolvedValue({ success: true, data: { id: 1 } })

  const boardId = ref(101)
  const candidateTopics = ref<BoardTopicListItem[]>(topics)
  const onSaved = vi.fn().mockResolvedValue(undefined)
  const requestMoveOutConfirm = vi.fn().mockResolvedValue(true)

  const c = useInlineCompose({ boardId, candidateTopics, onSaved, requestMoveOutConfirm })
  await c.enter()
  await flushPromises()
  return { c, onSaved, requestMoveOutConfirm, boardId, candidateTopics }
}

beforeEach(() => {
  vi.clearAllMocks()
})

// ── 进入 / 加载 ──────────────────────────────────────────────────────────────

describe('enter / load', () => {
  it('enter 设置 active=true 并按 embedDays(默认30)加载 pool 与 matchThreshold', async () => {
    const { c } = await setup()
    expect(c.active.value).toBe(true)
    expect(c.loading.value).toBe(false)
    expect(c.pool.value).toHaveLength(6)
    expect(c.matchThreshold.value).toBe(THRESHOLD)
    expect(api.getComposeCandidates).toHaveBeenCalledWith(101, 30)
  })

  it('embedDays 自定义透传', async () => {
    api.getComposeCandidates.mockResolvedValue({
      success: true,
      data: { sections: [], matchThreshold: THRESHOLD },
    })
    const c = useInlineCompose({
      boardId: ref(101),
      candidateTopics: ref<BoardTopicListItem[]>([]),
      onSaved: vi.fn(),
      requestMoveOutConfirm: vi.fn(),
      embedDays: 7,
    })
    await c.enter()
    await flushPromises()
    expect(api.getComposeCandidates).toHaveBeenCalledWith(101, 7)
  })

  it('load 失败 → error 置位、pool 空、loading 复位', async () => {
    api.getComposeCandidates.mockResolvedValue({ success: false, error: '网络错误' })
    const c = useInlineCompose({
      boardId: ref(101),
      candidateTopics: ref<BoardTopicListItem[]>([]),
      onSaved: vi.fn(),
      requestMoveOutConfirm: vi.fn(),
    })
    await c.enter()
    await flushPromises()
    expect(c.error.value).toBe('网络错误')
    expect(c.pool.value).toEqual([])
    expect(c.loading.value).toBe(false)
  })
})

// ── 就地勾选实时贴合度 + 渐进收敛 ───────────────────────────────────────────

describe('就地勾选实时贴合度（渐进收敛）', () => {
  it('取消勾选后 anchor 用剩余重算，全员 distance/tier/isOutlier 更新', async () => {
    const pool = [
      cand('a', { embedding: [1, 0] }),
      cand('b', { embedding: [0.95, 0.31] }),
      cand('c', { embedding: [0.8, 0.6] }),
      cand('far', { embedding: [-1, 0] }),
    ]
    const { c } = await setup(pool)

    c.toggle('a'); c.toggle('b'); c.toggle('c')
    await nextTick()
    const anchor123 = aggregatePreview([[1, 0], [0.95, 0.31], [0.8, 0.6]]).mean!
    expect(c.anchor.value).toEqual(anchor123)
    const dFarFrom123 = cosineDistance([-1, 0], anchor123)
    expect(c.nodeInfo.value.far!.distance).toBeCloseTo(dFarFrom123, 6)
    expect(c.nodeInfo.value.far!.tier).toBe(distanceTier(dFarFrom123, THRESHOLD))

    // 取消 c → anchor 用剩余 a,b 重算
    c.toggle('c')
    await nextTick()
    const anchor12 = aggregatePreview([[1, 0], [0.95, 0.31]]).mean!
    expect(c.anchor.value).toEqual(anchor12)
    expect(anchor12).not.toEqual(anchor123)
    const dFarFrom12 = cosineDistance([-1, 0], anchor12)
    expect(c.nodeInfo.value.far!.distance).toBeCloseTo(dFarFrom12, 6)
    expect(c.nodeInfo.value.far!.tier).toBe(distanceTier(dFarFrom12, THRESHOLD))
    // distance 随 anchor 改变而改变
    expect(dFarFrom12).not.toBeCloseTo(dFarFrom123, 6)
  })

  it('toggle 增删 selectedIds；isSelected 反映状态', async () => {
    const { c } = await setup()
    expect(c.isSelected('u1')).toBe(false)
    c.toggle('u1')
    expect(c.isSelected('u1')).toBe(true)
    expect(c.selectedIds.value.has('u1')).toBe(true)
    c.toggle('u1')
    expect(c.isSelected('u1')).toBe(false)
  })
})

// ── 离群标黄不自动删 ─────────────────────────────────────────────────────────

describe('离群标黄不自动删', () => {
  it('离群节点（distance > threshold×1.3）标黄但保持勾选状态', async () => {
    // a=[1,0], x=[-0.5,0.866] 同时勾选 → anchor 均值；x 到 anchor 距离 > 0.39
    const pool = [
      cand('a', { embedding: [1, 0] }),
      cand('x', { embedding: [-0.5, 0.866] }),
    ]
    const { c } = await setup(pool)
    c.toggle('a'); c.toggle('x')
    await nextTick()
    const anchor = aggregatePreview([[1, 0], [-0.5, 0.866]]).mean!
    const dX = cosineDistance([-0.5, 0.866], anchor)
    const info = c.nodeInfo.value.x!
    expect(info.distance).toBeCloseTo(dX, 6)
    expect(info.isOutlier).toBe(dX > THRESHOLD * 1.3)
    expect(info.tier).toBe(distanceTier(dX, THRESHOLD))
    // 关键：标黄（isOutlier）但仍在选中集，不自动移除
    expect(info.selected).toBe(true)
    expect(c.selectedIds.value.has('x')).toBe(true)
    expect(c.selectedCandidates.value.map(s => s.id)).toContain('x')
  })
})

// ── moveOut 分类 ─────────────────────────────────────────────────────────────

describe('moveOut 分类（勾走 active section 标移出提示）', () => {
  it('勾走 active section → counts.moveOut 增、originLabel 正确、moveOutItems 含原泳道名', async () => {
    const { c } = await setup()
    c.toggle('u1')
    c.toggle('m1')
    await nextTick()
    expect(c.counts.value).toEqual({ unassigned: 1, moveOut: 1 })
    // nodeInfo.moveOut / originLabel
    expect(c.nodeInfo.value.m1!.moveOut).toBe(true)
    expect(c.nodeInfo.value.m1!.originLabel).toBe('中东局势')
    expect(c.nodeInfo.value.u1!.moveOut).toBe(false)
    expect(c.nodeInfo.value.u1!.originLabel).toBeNull()
    // moveOutItems：每条选中 section 一项，含原泳道名
    expect(c.moveOutItems.value).toEqual([{ label: '中东战报A', origin: '中东局势' }])
  })

  it('勾走同一 active 泳道多条 → moveOutItems 逐条展开', async () => {
    const { c } = await setup()
    c.toggle('m1'); c.toggle('m2')
    await nextTick()
    expect(c.counts.value).toEqual({ unassigned: 0, moveOut: 2 })
    expect(c.moveOutItems.value).toEqual([
      { label: '中东战报A', origin: '中东局势' },
      { label: '中东战报B', origin: '中东局势' },
    ])
  })

  it('归属 candidate（非 active）topic 的 section 不算移出，归 unassigned（对齐 lanes 未分类口径）', async () => {
    const pool = [
      cand('c1', { embedding: [1, 0], clusterLabel: '候选归属', persistentTopicId: '8', persistentTopic: { id: '8', label: '候选话题', status: 'candidate' } }),
    ]
    const { c } = await setup(pool)
    c.toggle('c1')
    await nextTick()
    expect(c.nodeInfo.value.c1!.moveOut).toBe(false)
    expect(c.nodeInfo.value.c1!.originLabel).toBeNull()
    expect(c.counts.value).toEqual({ unassigned: 1, moveOut: 0 })
    expect(c.moveOutItems.value).toEqual([])
  })
})

// ── 聚类质量单卡实时 ─────────────────────────────────────────────────────────

describe('聚类质量单卡实时', () => {
  it('memberCount/meanDistance/outlierCount 随勾选实时更新', async () => {
    const pool = [
      cand('a', { embedding: [1, 0] }),
      cand('b', { embedding: [0.95, 0.31] }),
      cand('x', { embedding: [-0.5, 0.866] }),
    ]
    const { c } = await setup(pool)
    // 空选 → 全 0
    expect(c.quality.value).toEqual({ memberCount: 0, meanDistance: 0, outlierCount: 0 })

    c.toggle('a'); c.toggle('b')
    await nextTick()
    const anchor1 = aggregatePreview([[1, 0], [0.95, 0.31]]).mean!
    const d1 = [cosineDistance([1, 0], anchor1), cosineDistance([0.95, 0.31], anchor1)]
    expect(c.quality.value.memberCount).toBe(2)
    expect(c.quality.value.meanDistance).toBeCloseTo(d1.reduce((s, d) => s + d, 0) / 2, 6)
    expect(c.quality.value.outlierCount).toBe(0)

    // 加入离群 x
    c.toggle('x')
    await nextTick()
    const anchor2 = aggregatePreview([[1, 0], [0.95, 0.31], [-0.5, 0.866]]).mean!
    const d2 = [[1, 0], [0.95, 0.31], [-0.5, 0.866]].map(v => cosineDistance(v, anchor2))
    expect(c.quality.value.memberCount).toBe(3)
    expect(c.quality.value.meanDistance).toBeCloseTo(d2.reduce((s, d) => s + d, 0) / 3, 6)
    const flags = outlierFlags(d2, THRESHOLD)!
    expect(c.quality.value.outlierCount).toBe(flags.filter(Boolean).length)
    expect(c.quality.value.outlierCount).toBeGreaterThanOrEqual(1) // x 离群
  })
})

// ── adopt 预填名 + 预勾 ──────────────────────────────────────────────────────

describe('adopt（采纳预填名并预勾相关 section）', () => {
  it('预填 laneName + 按 centroid 预勾 matchThreshold 内 unassigned（升序），纯前端无 API', async () => {
    const pool = [
      cand('o1', { embedding: [1, 0], persistentTopicId: '20', persistentTopic: { id: '20', label: '美伊博弈', status: 'active' } }),
      cand('o2', { embedding: [0.95, 0.31], persistentTopicId: '20', persistentTopic: { id: '20', label: '美伊博弈', status: 'active' } }),
      cand('p1', { embedding: [1, 0] }),
      cand('p2', { embedding: [0.9, 0.4359] }),
      cand('p3', { embedding: [0, 1] }),
      cand('p4', { embedding: [-1, 0] }),
    ]
    const topics = [topic(20, { label: '美伊博弈', can_activate: true, consecutive_hits: 3, section_count: 2 })]
    const { c } = await setup(pool, topics)

    await c.adopt(c.sidebarItems.value[0]!)
    await nextTick()

    expect(c.laneName.value).toBe('美伊博弈')
    // centroid = mean(o1,o2)=[0.975,0.155]；p1/p2 在 0.3 内，p3/p4 外
    expect(c.selectedIds.value.has('p1')).toBe(true)
    expect(c.selectedIds.value.has('p2')).toBe(true)
    expect(c.selectedIds.value.has('p3')).toBe(false)
    expect(c.selectedIds.value.has('p4')).toBe(false)
    // 该候选自己的全部 section 先进入成员（spec adopt 场景），再补相似待确认
    expect(c.selectedIds.value.has('o1')).toBe(true)
    expect(c.selectedIds.value.has('o2')).toBe(true)
    expect(c.selectedIds.value.size).toBe(4)
    // 纯前端：不调任何 API
    expect(api.createManualLane).not.toHaveBeenCalled()
    expect(api.updateTopic).not.toHaveBeenCalled()
  })

  it('截断到 adoptPreselectCap（owned 优先，再取最近相似）', async () => {
    const pool = [
      cand('o1', { embedding: [1, 0], persistentTopicId: '20', persistentTopic: { id: '20', label: '美伊博弈', status: 'active' } }),
      cand('p1', { embedding: [1, 0] }),       // 距 centroid 0
      cand('p2', { embedding: [0.95, 0.31] }), // 距 centroid ~0.043
    ]
    api.getComposeCandidates.mockResolvedValue({
      success: true,
      data: { sections: pool, matchThreshold: THRESHOLD },
    })
    const c = useInlineCompose({
      boardId: ref(101),
      candidateTopics: ref<BoardTopicListItem[]>([topic(20, { label: '美伊博弈', can_activate: true })]),
      onSaved: vi.fn(),
      requestMoveOutConfirm: vi.fn(),
      adoptPreselectCap: 1,
    })
    await c.enter()
    await flushPromises()
    await c.adopt(c.sidebarItems.value[0]!)
    await nextTick()
    expect(c.selectedIds.value.size).toBe(1)
    expect(c.selectedIds.value.has('o1')).toBe(true) // owned 优先
  })

  it('在已有勾选基础上追加（不替换）', async () => {
    const pool = [
      cand('o1', { embedding: [1, 0], persistentTopicId: '20', persistentTopic: { id: '20', label: '美伊博弈', status: 'active' } }),
      cand('p1', { embedding: [1, 0] }),
      cand('p2', { embedding: [0.95, 0.31] }),
      cand('p3', { embedding: [0, 1] }),
    ]
    const topics = [topic(20, { label: '美伊博弈', can_activate: true })]
    const { c } = await setup(pool, topics)
    c.toggle('p2') // 预先勾选
    await nextTick()
    await c.adopt(c.sidebarItems.value[0]!)
    await nextTick()
    expect(c.selectedIds.value.has('p2')).toBe(true) // 原有保留
    expect(c.selectedIds.value.has('p1')).toBe(true) // 新增
    expect(c.selectedIds.value.has('o1')).toBe(true) // owned 并入
    expect(c.selectedIds.value.has('p3')).toBe(false)
  })
})

// ── 语义搜索 ─────────────────────────────────────────────────────────────────

describe('语义搜索（debounce + 冷启动 + 接管 + 降级）', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('冷启动：无勾选时 queryVec 接管分层', async () => {
    vi.useFakeTimers()
    const pool = [
      cand('n1', { embedding: [1, 0] }),
      cand('n2', { embedding: [0, 1] }),
    ]
    const { c } = await setup(pool)
    expect(c.activeSignal.value).toBeNull() // 无勾选无搜索

    api.embedQuery.mockResolvedValue({ success: true, data: { embedding: [1, 0] } })
    c.runSearch('半导体出口管制')
    expect(c.queryText.value).toBe('半导体出口管制')
    await vi.advanceTimersByTimeAsync(300)
    await nextTick()

    expect(c.queryVec.value).toEqual([1, 0])
    // 无勾选 → anchor null → activeSignal = queryVec
    expect(c.anchor.value).toBeNull()
    expect(c.activeSignal.value).toEqual([1, 0])
    expect(c.nodeInfo.value.n1!.distance).toBeCloseTo(0, 6)
    expect(c.nodeInfo.value.n2!.distance).toBeCloseTo(1, 6)
    expect(c.nodeInfo.value.n1!.tier).toBe('good')
  })

  it('勾选后 anchor 接管，queryVec 不再决定主信号', async () => {
    vi.useFakeTimers()
    const pool = [
      cand('n1', { embedding: [1, 0] }),
      cand('n2', { embedding: [0.9, 0.4359] }),
      cand('n3', { embedding: [0, 1] }),
    ]
    const { c } = await setup(pool)
    c.runSearch('半导体')
    await vi.advanceTimersByTimeAsync(300)
    await nextTick()
    expect(c.queryVec.value).toEqual([1, 0])
    expect(c.activeSignal.value).toEqual([1, 0]) // queryVec 接管

    // 勾选 n1,n2 → anchor 接管
    c.toggle('n1'); c.toggle('n2')
    await nextTick()
    const anchor = aggregatePreview([[1, 0], [0.9, 0.4359]]).mean!
    expect(c.anchor.value).toEqual(anchor)
    expect(c.activeSignal.value).toEqual(anchor) // anchor 优先于 queryVec
    expect(c.nodeInfo.value.n3!.distance).toBeCloseTo(cosineDistance([0, 1], anchor), 6)
  })

  it('空文本 → 清空 queryVec/searchError（clearSearch 语义），不调 embedQuery', async () => {
    vi.useFakeTimers()
    const { c } = await setup()
    c.runSearch('半导体')
    await vi.advanceTimersByTimeAsync(300)
    await nextTick()
    expect(c.queryVec.value).not.toBeNull()
    api.embedQuery.mockClear()

    c.runSearch('   ')
    await nextTick()
    expect(c.queryVec.value).toBeNull()
    expect(c.searchError.value).toBeNull()
    expect(api.embedQuery).not.toHaveBeenCalled()
  })

  it('clearSearch 清空文本与向量', async () => {
    vi.useFakeTimers()
    const { c } = await setup()
    c.runSearch('半导体')
    await vi.advanceTimersByTimeAsync(300)
    await nextTick()
    expect(c.queryText.value).toBe('半导体')
    c.clearSearch()
    expect(c.queryText.value).toBe('')
    expect(c.queryVec.value).toBeNull()
    expect(c.searchError.value).toBeNull()
  })

  it('搜索失败降级：queryVec=null + searchError，不污染主错误、不阻断勾选', async () => {
    vi.useFakeTimers()
    const { c } = await setup()
    api.embedQuery.mockResolvedValue({ success: false, error: '嵌入服务不可用' })
    c.runSearch('半导体')
    await vi.advanceTimersByTimeAsync(300)
    await nextTick()
    expect(c.queryVec.value).toBeNull()
    expect(c.searchError.value).toBe('嵌入服务不可用')
    expect(c.error.value).toBeNull() // 不污染主流程错误
    // 仍可勾选
    c.toggle('u1')
    await nextTick()
    expect(c.selectedIds.value.has('u1')).toBe(true)
  })
})

// ── rankedPool ───────────────────────────────────────────────────────────────

describe('rankedPool（渐进收敛排序）', () => {
  it('无信号保持原序；勾选后按 anchor 重排', async () => {
    const pool = [
      cand('c', { embedding: [-1, 0] }),
      cand('b', { embedding: [0, 1] }),
      cand('a', { embedding: [1, 0] }),
    ]
    const { c } = await setup(pool)
    // 无勾选无搜索 → 原序
    expect(c.rankedPool.value.map(x => x.id)).toEqual(['c', 'b', 'a'])
    c.toggle('a') // anchor=[1,0]
    await nextTick()
    // 按 anchor [1,0] 距离升序：a(0) < b(1) < c(2)
    expect(c.rankedPool.value.map(x => x.id)).toEqual(['a', 'b', 'c'])
  })
})

// ── save 流程（移出二次确认） ────────────────────────────────────────────────

describe('recommendations（相似 section 推荐）', () => {
  it('未勾选且未搜索 → 两组皆空（空态，侧边栏该区隐藏）', async () => {
    const { c } = await setup()
    expect(c.recommendations.value).toEqual({ unassigned: [], active: [] })
  })

  it('搜索冷启动（未勾选）也能推荐：activeSignal=queryVec，主组列 good 的 unassigned', async () => {
    vi.useFakeTimers()
    const pool = [
      cand('u1', { embedding: [1, 0], clusterLabel: '半导体1' }),
      cand('u2', { embedding: [0.95, 0.31], clusterLabel: '半导体2' }),
      cand('u3', { embedding: [-0.7, 0.7], clusterLabel: '无关' }),
    ]
    const { c } = await setup(pool)
    c.runSearch('半导体')
    vi.advanceTimersByTime(300)
    await flushPromises()
    // activeSignal=queryVec=[1,0]；u1(0)/u2(≈0.05) good 入主组升序，u3 far 排除
    expect(c.recommendations.value.unassigned.map(r => r.id)).toEqual(['u1', 'u2'])
    expect(c.recommendationTitle.value).toBe('与搜索词最相近')
    vi.useRealTimers()
  })

  it('勾选后按 anchor 推荐 ≤ threshold 的未勾选 section，分两组并按距离升序', async () => {
    const { c } = await setup()
    c.toggle('u1') // anchor = [1,0]
    await nextTick()
    const anchor = aggregatePreview([[1, 0]]).mean!
    expect(c.recommendations.value).toEqual({
      // u2(≈0.05) u3(0.20) ≤0.3 升序；u4(0.50) 超阈值排除
      unassigned: [
        { id: 'u2', clusterLabel: '美伊博弈2', originLabel: null, distance: cosineDistance([0.95, 0.31], anchor) },
        { id: 'u3', clusterLabel: '油价波动', originLabel: null, distance: cosineDistance([0.8, 0.6], anchor) },
      ],
      // m1(0) m2(0.10) ≤0.3 升序
      active: [
        { id: 'm1', clusterLabel: '中东战报A', originLabel: '中东局势', distance: cosineDistance([1, 0], anchor) },
        { id: 'm2', clusterLabel: '中东战报B', originLabel: '中东局势', distance: cosineDistance([0.9, 0.4359], anchor) },
      ],
    })
  })

  it('已勾选的 section 不出现在推荐里', async () => {
    const { c } = await setup()
    c.toggle('u1')
    c.toggle('u2')
    await nextTick()
    const ids = c.recommendations.value.unassigned.map(r => r.id)
    expect(ids).not.toContain('u1')
    expect(ids).not.toContain('u2')
  })

  it('top-N 截断：unassigned 取 5、active 取 3', async () => {
    const pool = [
      ...['s1', 's2', 's3', 's4', 's5', 's6', 's7'].map(id => cand(id, { embedding: [1, 0] })),
      ...['a1', 'a2', 'a3', 'a4', 'a5'].map(id => cand(id, { embedding: [1, 0], persistentTopicId: '7', persistentTopic: { id: '7', label: '中东局势', status: 'active' } })),
    ]
    const { c } = await setup(pool)
    c.toggle('s1') // anchor=[1,0]，其余全 dist=0 入选
    await nextTick()
    expect(c.recommendations.value.unassigned).toHaveLength(5)
    expect(c.recommendations.value.active).toHaveLength(3)
  })

  it('有 persistentTopicId 但无对象（无法确认 active）→ 归主组、originLabel null', async () => {
    const pool = [
      cand('u1', { embedding: [1, 0] }),
      cand('a1', { embedding: [1, 0], persistentTopicId: '7' }), // 无对象 → 非 active → 主组
    ]
    const { c } = await setup(pool)
    c.toggle('u1')
    await nextTick()
    expect(c.recommendations.value.unassigned.map(r => r.id)).toContain('a1')
    expect(c.recommendations.value.active).toEqual([])
  })
})

describe('save 流程（保存前移出二次确认）', () => {
  it('无 moveOut → 直接保存，不弹确认', async () => {
    const { c, onSaved, requestMoveOutConfirm } = await setup()
    c.toggle('u1')
    await nextTick()
    c.laneName.value = '美伊博弈'
    await c.save()
    await flushPromises()
    expect(requestMoveOutConfirm).not.toHaveBeenCalled()
    expect(api.createManualLane).toHaveBeenCalledWith(101, '美伊博弈', ['u1'])
    expect(onSaved).toHaveBeenCalledTimes(1)
    expect(c.active.value).toBe(false) // exit
  })

  it('有 moveOut → 先 requestMoveOutConfirm；返回 false 不调 createManualLane、不退出', async () => {
    const { c, onSaved, requestMoveOutConfirm } = await setup()
    c.toggle('m1')
    await nextTick()
    c.laneName.value = '美伊博弈'
    requestMoveOutConfirm.mockResolvedValue(false)
    await c.save()
    await flushPromises()
    expect(requestMoveOutConfirm).toHaveBeenCalledWith([{ label: '中东战报A', origin: '中东局势' }])
    expect(api.createManualLane).not.toHaveBeenCalled()
    expect(onSaved).not.toHaveBeenCalled()
    expect(c.active.value).toBe(true) // 未退出
    expect(c.saving.value).toBe(false)
  })

  it('有 moveOut → 确认 true → 保存 + 退出', async () => {
    const { c, onSaved } = await setup()
    c.toggle('m1')
    await nextTick()
    c.laneName.value = '美伊博弈'
    await c.save()
    await flushPromises()
    expect(api.createManualLane).toHaveBeenCalledWith(101, '美伊博弈', ['m1'])
    expect(onSaved).toHaveBeenCalledTimes(1)
    expect(c.active.value).toBe(false)
  })

  it('空名校验 → 不调 API、置 error', async () => {
    const { c } = await setup()
    c.toggle('u1')
    await nextTick()
    c.laneName.value = '   '
    await c.save()
    await flushPromises()
    expect(api.createManualLane).not.toHaveBeenCalled()
    expect(c.error.value).toBeTruthy()
    expect(c.active.value).toBe(true)
  })

  it('空选校验 → 不调 API、置 error', async () => {
    const { c } = await setup()
    c.laneName.value = '美伊博弈'
    await c.save()
    await flushPromises()
    expect(api.createManualLane).not.toHaveBeenCalled()
    expect(c.error.value).toBeTruthy()
  })

  it('createManualLane 失败 → 置 error、不退出', async () => {
    const { c } = await setup()
    // 在 setup() 之后覆盖（setup 内会重置默认成功实现）
    api.createManualLane.mockResolvedValue({ success: false, error: '保存失败' })
    c.toggle('u1')
    await nextTick()
    c.laneName.value = '美伊博弈'
    await c.save()
    await flushPromises()
    expect(c.error.value).toBe('保存失败')
    expect(c.active.value).toBe(true)
    expect(c.saving.value).toBe(false)
  })

  it('cancel → 清空勾选/名字并退出，无 API', async () => {
    const { c } = await setup()
    c.toggle('u1')
    c.laneName.value = '美伊博弈'
    c.queryText.value = 'xx'
    await nextTick()
    c.cancel()
    expect(c.selectedIds.value.size).toBe(0)
    expect(c.laneName.value).toBe('')
    expect(c.active.value).toBe(false)
    expect(api.createManualLane).not.toHaveBeenCalled()
  })
})

// ── 候选话题侧边栏 + 激活 ───────────────────────────────────────────────────

describe('候选话题侧边栏 + 激活', () => {
  it('sidebarItems：activatable 置顶 + consecutive_hits desc + brokenStreak 标记', async () => {
    const topics = [
      topic(3, { label: '油价', consecutive_hits: 1, can_activate: false }),
      topic(5, { label: '断连续', consecutive_hits: 0, can_activate: false }),
      topic(7, { label: '美伊', consecutive_hits: 3, can_activate: true }),
      topic(9, { label: '半导体', consecutive_hits: 5, can_activate: true }),
    ]
    const { c } = await setup([], topics)
    const items = c.sidebarItems.value
    // activatable 置顶（9 hits5, 7 hits3），再非 activatable（3 hits1, 5 hits0）
    expect(items.map(i => i.topic.id)).toEqual([9, 7, 3, 5])
    expect(items.find(i => i.topic.id === 9)!.activatable).toBe(true)
    expect(items.find(i => i.topic.id === 3)!.activatable).toBe(false)
    expect(items.find(i => i.topic.id === 5)!.brokenStreak).toBe(true)
    expect(items.find(i => i.topic.id === 3)!.brokenStreak).toBe(false)
  })

  it('无候选 → sidebarItems 空', async () => {
    const { c } = await setup([], [])
    expect(c.sidebarItems.value).toEqual([])
  })

  it('activate 达标候选 → updateTopic(active) + onSaved', async () => {
    const topics = [topic(7, { label: '美伊', can_activate: true })]
    const { c, onSaved } = await setup([], topics)
    await c.activate(c.sidebarItems.value[0]!)
    await flushPromises()
    expect(api.updateTopic).toHaveBeenCalledWith(7, { status: 'active' })
    expect(onSaved).toHaveBeenCalledTimes(1)
  })

  it('activate 未达标（!activatable）→ 不调 API', async () => {
    const topics = [topic(3, { label: '油价', can_activate: false })]
    const { c } = await setup([], topics)
    await c.activate(c.sidebarItems.value[0]!)
    await flushPromises()
    expect(api.updateTopic).not.toHaveBeenCalled()
  })

  it('activate 失败 → 置 error、不调 onSaved', async () => {
    const topics = [topic(7, { label: '美伊', can_activate: true })]
    const { c, onSaved } = await setup([], topics)
    api.updateTopic.mockResolvedValue({ success: false, error: '升级失败' })
    await c.activate(c.sidebarItems.value[0]!)
    await flushPromises()
    expect(c.error.value).toBe('升级失败')
    expect(onSaved).not.toHaveBeenCalled()
  })
})
