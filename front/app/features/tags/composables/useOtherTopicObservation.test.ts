/**
 * useOtherTopicObservation（8C G3 同归属跨日观察）测试。
 *
 * 覆盖 OBV 组：
 *  - OBV-1 只按可靠同 persistentTopicId + 当前连续观察轴取真实记录日（窗外/其他 ID/无 ID 不进入）
 *  - OBV-2 无 topicId 不发请求；0 日/1 日/多日诚实分层
 *  - OBV-3 同日多分区取 threadCount 峰值；任一分区缺失则整日未知（不冒 0）
 *  - OBV-5 加载/失败重试、旧成功保留 stale、切板块与强刷晚到响应不写新状态、在途合乘
 *  - G3 收口：已有数据强刷时 `refreshing` 如实暴露（status 仍 ready 但不能当终态）；
 *    `invalidate()` 懒失效（不 GET、在途作废、下次 ensure 重读）
 *  - 观察拖选/轴变化只本地重投影，不新增 GET
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick, ref } from 'vue'
import type {
  ObservationResponse,
  ObservationSectionRecord,
  ObservationTimelineData,
} from '~/api/otherTopicObservation'
import { useOtherTopicObservation, projectObservationDays } from './useOtherTopicObservation'

vi.mock('~/api/otherTopicObservation', async (importOriginal) => {
  const actual = await importOriginal<typeof import('~/api/otherTopicObservation')>()
  return {
    ...actual,
    useOtherTopicObservationApi: () => ({ loadBoardSectionTimeline: vi.fn() }),
  }
})

const BOARD = 1974
const AXIS = ['2026-10-20', '2026-10-21', '2026-10-22', '2026-10-23']

function record(
  sectionId: string,
  date: string,
  persistentTopicId: string | null,
  threadCount: number | null,
  reportId = '900',
): ObservationSectionRecord {
  return {
    sectionId,
    reportId,
    date,
    clusterLabel: `分区${sectionId}`,
    threadCount,
    persistentTopicId,
  }
}

const RECORDS: ObservationSectionRecord[] = [
  record('1', '2026-10-20', '10', 2),
  record('2', '2026-10-20', '10', 5),
  record('3', '2026-10-22', '10', 4),
  record('4', '2026-10-21', '10', null),
  record('5', '2026-10-20', '11', 9),
  record('6', '2026-10-01', '10', 7),
  record('7', '2026-10-19', null, 3),
  record('8', '2026-10-23', '10', 0),
]

function payload(records: ObservationSectionRecord[], reportId = '900'): ObservationTimelineData {
  return { records: records.map(item => ({ ...item, reportId })), coverage: 'all' }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((res) => { resolve = res })
  return { promise, resolve }
}

function harness(options: {
  records?: ObservationSectionRecord[]
  axis?: string[]
  fetcher?: ReturnType<typeof vi.fn>
} = {}) {
  const boardId = ref(BOARD)
  let axis = options.axis ?? AXIS
  const timeline = options.fetcher ?? vi.fn(async () => ({ success: true as const, data: payload(options.records ?? RECORDS) }))
  const observation = useOtherTopicObservation({
    boardId,
    axis: () => axis,
    fetchers: { timeline },
  })
  return {
    boardId,
    observation,
    timeline,
    setAxis(next: string[]) { axis = next },
  }
}

beforeEach(() => {
  vi.clearAllMocks()
})
afterEach(() => {
  vi.clearAllMocks()
})

describe('projectObservationDays（纯投影）', () => {
  it('只取可靠同 ID + 窗内记录日；同日多分区峰值、原序首分区为目标、缺失即未知', () => {
    const nodes = projectObservationDays(RECORDS, { topicId: '10', axis: AXIS })
    expect(nodes.map(node => node.date)).toEqual(['2026-10-20', '2026-10-21', '2026-10-22', '2026-10-23'])
    expect(nodes[0]).toEqual({
      date: '2026-10-20',
      reportId: '900',
      sectionId: '1',
      sectionIds: ['1', '2'],
      sectionTitles: ['分区1', '分区2'],
      threadCount: 5,
    })
    // 同日有缺失计数 → 整日未知，不是 2+null 或拿另一分区冒峰值
    expect(nodes[1]!.threadCount).toBeNull()
    expect(nodes[2]!.threadCount).toBe(4)
    // 0 是合法已知值
    expect(nodes[3]!.threadCount).toBe(0)
  })

  it('窗外记录、其他 topicId、无 persistentTopicId、无 topicId 都不进入；1 日/0 日如实返回', () => {
    expect(projectObservationDays(RECORDS, { topicId: '10', axis: ['2026-10-22'] }).map(node => node.date))
      .toEqual(['2026-10-22'])
    expect(projectObservationDays(RECORDS, { topicId: '10', axis: ['2026-10-05'] })).toEqual([])
    expect(projectObservationDays(RECORDS, { topicId: '11', axis: AXIS }).map(node => node.date))
      .toEqual(['2026-10-20'])
    expect(projectObservationDays(RECORDS, { topicId: null, axis: AXIS })).toEqual([])
    expect(projectObservationDays(null, { topicId: '10', axis: AXIS })).toEqual([])
  })
})

describe('useOtherTopicObservation', () => {
  it('无可靠 ID 不发请求：missing-id 是终态，不是加载/无记录', async () => {
    const { observation, timeline } = harness()
    await observation.ensureObservation(null)
    expect(timeline).not.toHaveBeenCalled()
    expect(observation.view.value.status).toBe('missing-id')
    expect(observation.view.value.nodes).toEqual([])
    expect(observation.view.value.coverage).toBeNull()
  })

  it('固定 days=0（全部历史）只发一次 GET；同板块换 topic 命中缓存不再请求', async () => {
    const { observation, timeline, setAxis } = harness()
    await observation.ensureObservation('10')
    expect(timeline).toHaveBeenCalledTimes(1)
    expect(timeline).toHaveBeenCalledWith(BOARD)
    expect(observation.view.value.status).toBe('ready')
    expect(observation.view.value.coverage).toBe('all')
    expect(observation.view.value.refreshing).toBe(false)

    await observation.ensureObservation('11')
    expect(timeline).toHaveBeenCalledTimes(1)
    expect(observation.view.value.nodes.map(node => node.date)).toEqual(['2026-10-20'])

    // 拖范围/预览只本地裁投影，不新增 GET
    setAxis(['2026-10-20', '2026-10-21'])
    await nextTick()
    expect(timeline).toHaveBeenCalledTimes(1)
    expect(observation.view.value.nodes.map(node => node.date)).toEqual(['2026-10-20'])
  })

  it('失败可重试；失败不冒充成功缓存，成功后才不再请求', async () => {
    const { observation, timeline } = harness()
    timeline.mockResolvedValueOnce({ success: false, error: 'timeline 不可用' })
    await observation.ensureObservation('10')
    expect(observation.view.value.status).toBe('error')
    expect(observation.view.value.error).toBe('timeline 不可用')
    expect(observation.view.value.nodes).toEqual([])
    expect(observation.view.value.coverage).toBeNull()

    timeline.mockResolvedValueOnce({ success: true, data: payload(RECORDS) })
    await observation.retry()
    expect(timeline).toHaveBeenCalledTimes(2)
    expect(observation.view.value.status).toBe('ready')
    expect(observation.view.value.nodes).toHaveLength(4)
  })

  it('强刷失败保留旧成功数据并标 stale（不冒空、不冒无记录）', async () => {
    const { observation, timeline } = harness()
    await observation.ensureObservation('10')
    const before = observation.view.value.nodes
    timeline.mockResolvedValueOnce({ success: false, error: '刷新失败' })
    await observation.ensureObservation('10', { force: true })
    expect(observation.view.value.status).toBe('ready')
    expect(observation.view.value.stale).toBe(true)
    expect(observation.view.value.refreshing).toBe(false)
    expect(observation.view.value.error).toBe('刷新失败')
    expect(observation.view.value.nodes).toEqual(before)
    expect(observation.view.value.coverage).toBe('all')
  })

  it('在途合乘：同 tick 两次 ensure 只发一次列表请求', async () => {
    const pending = deferred<ObservationResponse<ObservationTimelineData>>()
    const timeline = vi.fn(() => pending.promise)
    const { observation } = harness({ fetcher: timeline })
    const first = observation.ensureObservation('10')
    const second = observation.ensureObservation('10')
    expect(timeline).toHaveBeenCalledTimes(1)
    pending.resolve({ success: true, data: payload(RECORDS) })
    await Promise.all([first, second])
    expect(observation.view.value.status).toBe('ready')
  })

  it('强刷晚到：旧响应不覆盖更新的强刷结果；loading/error 归最新请求', async () => {
    const { observation, timeline } = harness()
    await observation.ensureObservation('10')

    const older = deferred<ObservationResponse<ObservationTimelineData>>()
    const newer = deferred<ObservationResponse<ObservationTimelineData>>()
    timeline.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise)
    const olderRun = observation.ensureObservation('10', { force: true })
    const newerRun = observation.ensureObservation('10', { force: true })
    expect(timeline).toHaveBeenCalledTimes(3)

    newer.resolve({ success: true, data: payload([record('9', '2026-10-23', '10', 8)]) })
    await newerRun
    expect(observation.view.value.nodes.map(node => node.sectionId)).toEqual(['9'])

    const olderData = payload([record('1', '2026-10-20', '10', 99)])
    older.resolve({ success: true, data: olderData })
    await olderRun
    expect(observation.view.value.nodes.map(node => node.sectionId)).toEqual(['9'])
    expect(observation.view.value.stale).toBe(false)
  })

  it('强刷在途：已有数据 status 仍 ready 但 refreshing=true；终态后归 false', async () => {
    const { observation, timeline } = harness()
    await observation.ensureObservation('10')
    expect(observation.view.value.refreshing).toBe(false)

    const pending = deferred<ObservationResponse<ObservationTimelineData>>()
    timeline.mockReturnValueOnce(pending.promise)
    const run = observation.ensureObservation('10', { force: true })
    await nextTick()
    // 旧成功数据还在（保留可读），但刷新就是刷新——不能当成本轮终态
    expect(observation.view.value.status).toBe('ready')
    expect(observation.view.value.refreshing).toBe(true)

    pending.resolve({ success: true, data: payload(RECORDS) })
    await run
    expect(observation.view.value.refreshing).toBe(false)
  })

  it('invalidate 懒失效：不发请求、丢掉成功缓存（不发旧数据）；下次 ensure 真的重读', async () => {
    const { observation, timeline } = harness()
    await observation.ensureObservation('10')
    expect(timeline).toHaveBeenCalledTimes(1)

    observation.invalidate()
    await nextTick()
    // 不 GET、不冒旧节点；未选其他行时也不能留着「成功缓存」以后永读旧数据
    expect(timeline).toHaveBeenCalledTimes(1)
    expect(observation.hasDataset()).toBe(false)
    expect(observation.view.value.status).toBe('idle')
    expect(observation.view.value.nodes).toEqual([])
    expect(observation.view.value.coverage).toBeNull()

    await observation.ensureObservation('10')
    expect(timeline).toHaveBeenCalledTimes(2)
    expect(observation.view.value.status).toBe('ready')
  })

  it('invalidate 使在途旧响应失效：晚到不写数据；未选行时不发无谓请求', async () => {
    const pending = deferred<ObservationResponse<ObservationTimelineData>>()
    const timeline = vi.fn(() => pending.promise)
    const { observation } = harness({ fetcher: timeline })
    const run = observation.ensureObservation('10')
    expect(timeline).toHaveBeenCalledTimes(1)

    observation.invalidate()
    await nextTick()
    expect(timeline).toHaveBeenCalledTimes(1)
    expect(observation.view.value.status).toBe('idle')

    pending.resolve({ success: true, data: payload(RECORDS) })
    await run
    expect(observation.hasDataset()).toBe(false)
    expect(observation.view.value.status).toBe('idle')

    // 未选其他行时 invalidate/retry 都不该沿旧 topic 发请求
    observation.invalidate()
    expect(timeline).toHaveBeenCalledTimes(1)
  })

  it('切板块：旧响应不写新板块数据/错误/loading；新板块重新请求', async () => {
    const pending = deferred<ObservationResponse<ObservationTimelineData>>()
    const timeline = vi.fn(() => pending.promise)
    const { observation, boardId } = harness({ fetcher: timeline })
    const run = observation.ensureObservation('10')
    expect(observation.view.value.status).toBe('loading')

    boardId.value = 2077
    await nextTick()
    expect(observation.view.value.status).toBe('missing-id')

    pending.resolve({ success: true, data: payload(RECORDS) })
    await run
    expect(observation.view.value.status).toBe('missing-id')
    expect(observation.view.value.coverage).toBeNull()
    expect(observation.view.value.error).toBeNull()

    timeline.mockResolvedValueOnce({ success: true, data: payload([]) })
    await observation.ensureObservation('10')
    expect(timeline).toHaveBeenLastCalledWith(2077)
    expect(observation.view.value.status).toBe('ready')
    expect(observation.view.value.nodes).toEqual([])
  })

  it('跨日/换 topic 时先清当前投影（不把旧 ID 的节点挂在新 ID 名下）', async () => {
    const { observation } = harness()
    await observation.ensureObservation('10')
    expect(observation.view.value.nodes.length).toBeGreaterThan(0)
    await observation.ensureObservation('12')
    expect(observation.view.value.topicId).toBe('12')
    expect(observation.view.value.nodes).toEqual([])
    expect(observation.view.value.status).toBe('ready')
  })
})
