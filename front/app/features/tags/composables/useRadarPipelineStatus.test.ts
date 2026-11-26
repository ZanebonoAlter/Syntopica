/**
 * useRadarPipelineStatus 测试（add-radar-pipeline-status D6/D7/D8/D11）。
 *
 * 覆盖：队列派生口径（activeCount / roundTotal / 进度）/ 生成与兜底两个时刻 /
 * 旧后端缺 schedule_deadline 的降级 / 当日判定（本地日期）/ 首次对账未成功
 * 不产出结论 / 对账失败保留旧值 / 手动生成状态机（submitting → done、failed）/
 * WS 消息按 job_id 归属（并发管线的终态不被吸收）/ 队列门槛在入口同样把关 /
 * 终态三态计数（直接读 failed_count / empty_count，「当日无内容」不算失败；
 * 旧后端两级回落：逐板进度失败数 → total_boards - total_saved）。
 *
 * 对账走真实 `usePollBundle`（只 mock `~/api/poll` 端点），因此「单一合并入口、
 * 不新建轮询」这条契约在用例里可直接断言（请求次数 = 1）。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useRadarPipelineStatus, type RadarStatusBoard } from './useRadarPipelineStatus'
import { usePollBundle } from '~/composables/usePollBundle'
import { todayKey } from '~/utils/todayKey'
import type { SchedulerStatus } from '~/types/scheduler'

const { getPollBundleMock } = vi.hoisted(() => ({ getPollBundleMock: vi.fn() }))
vi.mock('~/api/poll', () => ({
  usePollApi: () => ({ getPollBundle: getPollBundleMock }),
}))

const { generateDailyReportMock, getDailyReportGapsMock, triggerGapBackfillMock } = vi.hoisted(() => ({
  generateDailyReportMock: vi.fn(),
  getDailyReportGapsMock: vi.fn(),
  triggerGapBackfillMock: vi.fn(),
}))
vi.mock('~/api/dailyReports', () => ({
  useDailyReportsApi: () => ({
    generateDailyReport: generateDailyReportMock,
    getDailyReportGaps: getDailyReportGapsMock,
    triggerGapBackfill: triggerGapBackfillMock,
  }),
}))

const wsHandlers = vi.hoisted(() => new Map<string, (data: unknown) => void>())
vi.mock('~/composables/useEventStream', () => ({
  useEventStream: () => ({
    on: (type: string, handler: (data: unknown) => void) => {
      wsHandlers.set(type, handler)
      return () => { wsHandlers.delete(type) }
    },
    off: () => {},
    get connected() { return false },
  }),
}))

const rows = ref<RadarStatusBoard[]>([])

interface PollOverrides {
  pending?: number
  processing?: number
  completedToday?: number
  failed?: number
  /** embedding 队列（单位：标签）。默认 0 → 双队列均清空。 */
  embeddingPending?: number
  embeddingProcessing?: number
  embeddingCompletedToday?: number
  embeddingFailed?: number
  schedulers?: SchedulerStatus[]
}

function pollOk(over: PollOverrides = {}) {
  return {
    success: true,
    data: {
      schedulers: over.schedulers ?? [{
        name: 'daily_report',
        check_interval: 0,
        schedule_time: '21:00',
        schedule_deadline: '23:30',
      }],
      tag_queue: {
        pending: over.pending ?? 0,
        processing: over.processing ?? 0,
        completed: over.completedToday ?? 0,
        total: (over.pending ?? 0) + (over.processing ?? 0) + (over.completedToday ?? 0),
        completed_today: over.completedToday ?? 0,
        failed: over.failed ?? 0,
      },
      // embedding 队列（单位：标签）——双队列判据的第二环；默认清空。
      embedding_queue: {
        pending: over.embeddingPending ?? 0,
        processing: over.embeddingProcessing ?? 0,
        completed: over.embeddingCompletedToday ?? 0,
        total: (over.embeddingPending ?? 0) + (over.embeddingProcessing ?? 0) + (over.embeddingCompletedToday ?? 0),
        completed_today: over.embeddingCompletedToday ?? 0,
        failed: over.embeddingFailed ?? 0,
      },
      notifications: { unread: 0 },
    },
  }
}

function board(over: Partial<RadarStatusBoard> = {}): RadarStatusBoard {
  return {
    boardId: '1',
    label: '中东地缘政治与美伊关系',
    reportDate: '2026-10-24',
    todayClues: 8,
    ...over,
  }
}

/** 缺口查询响应（默认：窗口内无缺口）。 */
function gapsOk(over: {
  gaps?: Array<{ date: string; board_id: number; board_label: string }>
  missing_days?: number
  total_missing?: number
} = {}) {
  return {
    success: true,
    data: {
      window_start: '2026-10-18',
      window_end: '2026-10-24',
      days: 7,
      retention_days: 7,
      gaps: over.gaps ?? [],
      missing_days: over.missing_days ?? 0,
      total_missing: over.total_missing ?? 0,
    },
  }
}

/** 挂载 composable 并等对账分发落地。 */
async function setup(over: PollOverrides = {}) {
  getPollBundleMock.mockResolvedValue(pollOk(over))
  const status = useRadarPipelineStatus({
    rows: () => rows.value,
    loading: () => false,
    error: () => '',
  })
  await vi.advanceTimersByTimeAsync(0)
  return status
}

function emitWs(type: string, data: Record<string, unknown>): void {
  wsHandlers.get(type)?.(data)
}

beforeEach(() => {
  vi.useFakeTimers()
  wsHandlers.clear()
  rows.value = []
  getPollBundleMock.mockReset()
  generateDailyReportMock.mockReset()
  getDailyReportGapsMock.mockReset()
  triggerGapBackfillMock.mockReset()
  getDailyReportGapsMock.mockResolvedValue(gapsOk())
  triggerGapBackfillMock.mockResolvedValue({
    success: true,
    data: { job_id: 'bj1', run_id: 'br1', status: 'processing', window_start: '2026-10-18', window_end: '2026-10-24' },
  })
})

afterEach(async () => {
  usePollBundle().__resetPollBundleForTest()
  await vi.advanceTimersByTimeAsync(0)
  vi.useRealTimers()
})

describe('useRadarPipelineStatus — 队列派生口径（D8）', () => {
  it('activeCount = pending + processing；本轮总量 = 活跃量 + 今日完成；进度按同口径', async () => {
    const status = await setup({ pending: 3, processing: 1, completedToday: 62 })
    expect(status.activeCount.value).toBe(4)
    expect(status.roundTotal.value).toBe(66)
    expect(status.progressPercent.value).toBe(94)
    expect(status.completedToday.value).toBe(62)
    expect(status.queueBlocked.value).toBe(true)
    expect(status.queueIdle.value).toBe(false)
  })

  it('失败任务不算积压：failed > 0 不禁用生成', async () => {
    const status = await setup({ pending: 0, processing: 0, completedToday: 0, failed: 2 })
    expect(status.failedCount.value).toBe(2)
    expect(status.activeCount.value).toBe(0)
    expect(status.queueBlocked.value).toBe(false)
    expect(status.queueIdle.value).toBe(true)
  })

  it('分母为 0 时进度为 0%（不除零）', async () => {
    const status = await setup({ pending: 0, processing: 0, completedToday: 0 })
    expect(status.roundTotal.value).toBe(0)
    expect(status.progressPercent.value).toBe(0)
  })
})

describe('useRadarPipelineStatus — embedding 队列（第二环，D14）', () => {
  it('embeddingActiveCount = pending + processing；本轮总量与进度按同口径', async () => {
    const status = await setup({ embeddingPending: 12, embeddingProcessing: 2, embeddingCompletedToday: 10 })
    expect(status.embeddingActiveCount.value).toBe(14)
    expect(status.embeddingRoundTotal.value).toBe(24)
    expect(status.embeddingProgressPercent.value).toBe(42)
    expect(status.embeddingCompletedToday.value).toBe(10)
  })

  it('标签队列清空但 embedding 队列有积压：queuesDrained 为假（双队列判据）', async () => {
    const status = await setup({ pending: 0, embeddingPending: 166 })
    expect(status.queueBlocked.value).toBe(false)
    expect(status.embeddingBlocked.value).toBe(true)
    expect(status.queuesDrained.value).toBe(false)
  })

  it('双队列均清空：queuesDrained 为真', async () => {
    const status = await setup({ pending: 0, embeddingPending: 0 })
    expect(status.embeddingBlocked.value).toBe(false)
    expect(status.queuesDrained.value).toBe(true)
  })

  it('embedding 队列的失败任务不算积压（failed 不阻断生成）', async () => {
    const status = await setup({ embeddingFailed: 3 })
    expect(status.embeddingFailedCount.value).toBe(3)
    expect(status.embeddingBlocked.value).toBe(false)
    expect(status.queuesDrained.value).toBe(true)
  })

  it('embedding 队列有积压时手动生成入口不放行：绕过 UI 直接调用也不发请求', async () => {
    const status = await setup({ embeddingPending: 5 })
    await status.generate([3])
    expect(generateDailyReportMock).not.toHaveBeenCalled()
    expect(status.phase.value).toBe('idle')
  })
})

describe('useRadarPipelineStatus — 日报生成窗口（V4/V13）', () => {
  it('生成时刻与兜底时刻两个都读得出', async () => {
    const status = await setup({
      schedulers: [{ name: 'daily_report', check_interval: 0, schedule_time: '20:15', schedule_deadline: '22:45' }],
    })
    expect(status.scheduleTime.value).toBe('20:15')
    expect(status.scheduleDeadline.value).toBe('22:45')
  })

  it('旧后端缺 schedule_deadline：降级到默认 23:30，不留空', async () => {
    const status = await setup({
      schedulers: [{ name: 'daily_report', check_interval: 0, schedule_time: '21:00' }],
    })
    expect(status.scheduleTime.value).toBe('21:00')
    expect(status.scheduleDeadline.value).toBe('23:30')
  })

  it('没有 daily_report 条目时也降级到默认时刻', async () => {
    const status = await setup({ schedulers: [{ name: 'auto_refresh', check_interval: 0 }] })
    expect(status.scheduleTime.value).toBe('21:00')
    expect(status.scheduleDeadline.value).toBe('23:30')
  })
})

describe('useRadarPipelineStatus — 日报落点与当日判定（D7）', () => {
  it('最新落点取各板 report_date 最大值；当日判定用本地今天', async () => {
    rows.value = [
      board({ boardId: '1', reportDate: '2026-10-23' }),
      board({ boardId: '2', reportDate: todayKey() }),
      board({ boardId: '3', reportDate: '2026-10-01' }),
    ]
    const status = await setup()
    expect(status.latestReportDate.value).toBe(todayKey())
    expect(status.todayReportCount.value).toBe(1)
    expect(status.todayReady.value).toBe(true)
    expect(status.boardTotal.value).toBe(3)
  })

  it('没有板块有日报时落点为空', async () => {
    rows.value = [board({ boardId: '1', reportDate: '' })]
    const status = await setup()
    expect(status.latestReportDate.value).toBe('')
    expect(status.todayReportCount.value).toBe(0)
    expect(status.todayReady.value).toBe(false)
  })
})

describe('useRadarPipelineStatus — 对账语义（R-17）', () => {
  it('首次对账从未成功：pollReady 为假（上层据此呈现「读取中」，不产出结论）', async () => {
    getPollBundleMock.mockResolvedValue({ success: false, error: '接口挂了' })
    const status = useRadarPipelineStatus({ rows: () => rows.value })
    await vi.advanceTimersByTimeAsync(0)
    expect(status.pollReady.value).toBe(false)
    expect(status.updatedAtText.value).toBe('')
  })

  it('对账失败保留旧值：不清零、更新时间戳不动', async () => {
    const status = await setup({ pending: 3, processing: 1, completedToday: 62 })
    const stamp = status.updatedAtText.value
    expect(stamp).not.toBe('')

    getPollBundleMock.mockResolvedValue({ success: false, error: '接口挂了' })
    await usePollBundle().reconcileNow()
    await vi.advanceTimersByTimeAsync(0)

    expect(status.activeCount.value).toBe(4)
    expect(status.completedToday.value).toBe(62)
    expect(status.updatedAtText.value).toBe(stamp)
  })

  it('常驻对账只走单一合并入口：状态块不新建周期性请求', async () => {
    await setup()
    expect(getPollBundleMock).toHaveBeenCalledTimes(1)
  })
})

describe('useRadarPipelineStatus — 手动生成（D11）', () => {
  it('提交后进入 submitting；收到进度与终态后给出成功/失败板块数', async () => {
    const status = await setup()
    generateDailyReportMock.mockResolvedValue({ success: true, data: { job_id: 'j1', status: 'processing' } })

    const pending = status.generate([3, 7, 9])
    expect(status.phase.value).toBe('submitting')
    expect(generateDailyReportMock).toHaveBeenCalledWith({ date: todayKey(), board_ids: [3, 7, 9] })

    // 响应到达 → job_id 落定（之后的 WS 消息按 job_id 归属）
    await pending

    emitWs('daily_report_progress', { job_id: 'j1', board_id: 3, status: 'completed', progress: '1/3' })
    emitWs('daily_report_progress', { job_id: 'j1', board_id: 7, status: 'failed', progress: '2/3' })
    emitWs('daily_report_progress', { job_id: 'j1', board_id: 9, status: 'failed', progress: '3/3' })
    expect(status.progressText.value).toBe('3/3')

    emitWs('daily_report_done', { job_id: 'j1', total_saved: 1, total_boards: 3 })
    expect(status.phase.value).toBe('done')
    expect(status.result.value).toEqual({ successBoards: 1, failedBoards: 2, emptyBoards: 0 })
  })

  it('并发管线的终态消息不被本次生成吸收（按 job_id 归属）', async () => {
    const status = await setup()
    generateDailyReportMock.mockResolvedValue({ success: true, data: { job_id: 'j1', status: 'processing' } })

    await status.generate([3])
    expect(status.phase.value).toBe('submitting')

    // 别的 job 的终态：不吸收（不得把并发管线的结果当成本次的）
    emitWs('daily_report_done', { job_id: 'other-job', total_saved: 9, total_boards: 9 })
    expect(status.phase.value).toBe('submitting')
    expect(status.result.value).toBeNull()

    // 本次 job 的终态：吸收
    emitWs('daily_report_done', { job_id: 'j1', total_saved: 1, total_boards: 1 })
    expect(status.phase.value).toBe('done')
    expect(status.result.value).toEqual({ successBoards: 1, failedBoards: 0, emptyBoards: 0 })
  })

  it('进度消息全丢：终态按 total_saved 换算成功/失败数（不把失败板算成成功）', async () => {
    const status = await setup()
    generateDailyReportMock.mockResolvedValue({ success: true, data: { job_id: 'j1', status: 'processing' } })

    await status.generate([3, 7, 9])
    emitWs('daily_report_done', { job_id: 'j1', total_saved: 1, total_boards: 3 })

    expect(status.result.value).toEqual({ successBoards: 1, failedBoards: 2, emptyBoards: 0 })
  })

  it('终态自带三态计数：直接读 failed_count / empty_count，不拿总数反推失败数', async () => {
    const status = await setup()
    generateDailyReportMock.mockResolvedValue({ success: true, data: { job_id: 'j1', status: 'processing' } })

    await status.generate([3, 7])
    // 1 个成功板 + 1 个当日无内容的板（报告为空，不算失败）
    emitWs('daily_report_done', {
      job_id: 'j1',
      total_saved: 1,
      total_boards: 2,
      failed_count: 0,
      empty_count: 1,
    })

    expect(status.result.value).toEqual({ successBoards: 1, failedBoards: 0, emptyBoards: 1 })
  })

  it('终态自带三态计数：真失败数直接读 failed_count（不用 total_boards - total_saved）', async () => {
    const status = await setup()
    generateDailyReportMock.mockResolvedValue({ success: true, data: { job_id: 'j1', status: 'processing' } })

    await status.generate([3, 7, 9])
    // 1 成功 + 1 真失败 + 1 当日无内容：旧口径会把无内容的板算成失败（2 个）
    emitWs('daily_report_done', {
      job_id: 'j1',
      total_saved: 1,
      total_boards: 3,
      failed_count: 1,
      empty_count: 1,
    })

    expect(status.result.value).toEqual({ successBoards: 1, failedBoards: 1, emptyBoards: 1 })
  })

  it('旧后端（无 failed_count）：回落逐板进度消息里的失败数', async () => {
    const status = await setup()
    generateDailyReportMock.mockResolvedValue({ success: true, data: { job_id: 'j1', status: 'processing' } })

    await status.generate([3, 7, 9])
    emitWs('daily_report_progress', { job_id: 'j1', board_id: 3, status: 'completed', progress: '1/3' })
    emitWs('daily_report_progress', { job_id: 'j1', board_id: 7, status: 'failed', progress: '2/3' })
    emitWs('daily_report_progress', { job_id: 'j1', board_id: 9, status: 'failed', progress: '3/3' })
    emitWs('daily_report_done', { job_id: 'j1', total_saved: 1, total_boards: 3 })

    expect(status.result.value).toEqual({ successBoards: 1, failedBoards: 2, emptyBoards: 0 })
  })

  it('旧后端 + 进度消息全丢：回落旧口径 total_boards - total_saved', async () => {
    const status = await setup()
    generateDailyReportMock.mockResolvedValue({ success: true, data: { job_id: 'j1', status: 'processing' } })

    await status.generate([3, 7, 9])
    emitWs('daily_report_done', { job_id: 'j1', total_saved: 1, total_boards: 3 })

    expect(status.result.value).toEqual({ successBoards: 1, failedBoards: 2, emptyBoards: 0 })
  })

  it('请求被 409 拒绝：恢复可用并给出互斥文案（不静默）', async () => {
    const status = await setup()
    generateDailyReportMock.mockResolvedValue({ success: false, status: 409, error: '日报生成正在执行中，请稍后再试。' })

    await status.generate([3])
    expect(status.phase.value).toBe('failed')
    expect(status.failureMessage.value).toBe('日报生成正在执行中，请稍后再试。')
  })

  it('请求被 400 拒绝：显示后端错误文案', async () => {
    const status = await setup()
    generateDailyReportMock.mockResolvedValue({ success: false, status: 400, error: '超出重建窗口' })

    await status.generate([3])
    expect(status.phase.value).toBe('failed')
    expect(status.failureMessage.value).toBe('超出重建窗口')
  })

  it('未勾选任何板块时不发请求', async () => {
    const status = await setup()
    await status.generate([])
    expect(generateDailyReportMock).not.toHaveBeenCalled()
    expect(status.phase.value).toBe('idle')
  })

  it('队列有积压时入口不放行：绕过 UI 直接调用也不发请求', async () => {
    const status = await setup({ pending: 3, processing: 1 })
    expect(status.queueBlocked.value).toBe(true)

    await status.generate([3])
    expect(generateDailyReportMock).not.toHaveBeenCalled()
    expect(status.phase.value).toBe('idle')
  })

  it('对账从未成功时入口不放行（队列状态未知）', async () => {
    getPollBundleMock.mockResolvedValue({ success: false, error: '接口挂了' })
    const status = useRadarPipelineStatus({ rows: () => rows.value })
    await vi.advanceTimersByTimeAsync(0)
    expect(status.pollReady.value).toBe(false)

    await status.generate([3])
    expect(generateDailyReportMock).not.toHaveBeenCalled()
    expect(status.phase.value).toBe('idle')
  })
})

// ── 缺口与补填（add-daily-report-gap-backfill D8/D10）────────────────────
describe('useRadarPipelineStatus — 缺口取数（一次性请求 + 独立降级）', () => {
  it('取数失败：只降级缺口区，不产出「无缺口」结论，其余事实不受影响', async () => {
    const status = await setup()
    getDailyReportGapsMock.mockResolvedValue({ success: false, error: '接口挂了' })

    await status.reloadGaps()

    expect(status.gapError.value).toBe('接口挂了')
    expect(status.gapLoaded.value).toBe(false)
    expect(status.gapsByDate.value).toEqual([])
    // 其余五项事实照常（缺口区降级不连坐）
    expect(status.pollReady.value).toBe(true)
    expect(status.queueIdle.value).toBe(true)
    expect(status.overviewError.value).toBe('')
  })

  it('取数成功：按日分组（日期升序）、板块名缺失回退、不含今天', async () => {
    const status = await setup()
    getDailyReportGapsMock.mockResolvedValue(gapsOk({
      gaps: [
        { date: '2026-10-23', board_id: 12, board_label: 'AI 前沿' },
        { date: '2026-10-22', board_id: 3, board_label: '' },
        { date: '2026-10-22', board_id: 4, board_label: '芯片' },
        { date: todayKey(), board_id: 9, board_label: '今天' },
      ],
      missing_days: 2,
      total_missing: 4,
    }))

    await status.reloadGaps()

    expect(status.gapLoaded.value).toBe(true)
    expect(status.gapError.value).toBe('')
    expect(status.missingDays.value).toBe(2)
    expect(status.totalMissing.value).toBe(4)
    expect(status.gapsByDate.value.map(d => d.date)).toEqual(['2026-10-22', '2026-10-23'])
    // 板块名缺失 → 回退 Board #N（不编造板块名）
    expect(status.gapsByDate.value[0]?.boards.map(b => b.label)).toEqual(['Board #3', '芯片'])
    // 今天不在缺口区（今日状态由「当日日报状态」行负责）
    expect(status.gapsByDate.value.some(d => d.date === todayKey())).toBe(false)
  })

  it('缺口取数是**一次性**请求：不随对账轮询重复发', async () => {
    const status = await setup()
    await status.reloadGaps()
    expect(getDailyReportGapsMock).toHaveBeenCalledTimes(1)

    // 对账再来一次（既有轮询）——缺口不跟着重取
    getPollBundleMock.mockResolvedValue(pollOk())
    await usePollBundle().reconcileNow()
    await vi.advanceTimersByTimeAsync(0)
    expect(getDailyReportGapsMock).toHaveBeenCalledTimes(1)
  })

  it('跨日切换：重取一次缺口（不新建定时器）', async () => {
    const status = await setup()
    // 本实例尚未取过缺口（gapLoaded 假）
    expect(status.gapLoaded.value).toBe(false)

    // 跨过本地日界：对账成功一次（对账时刻是跨日判据的时钟刻度）→ 缺口重取
    const tomorrow = new Date(Date.now() + 24 * 3600 * 1000)
    vi.setSystemTime(tomorrow)
    getPollBundleMock.mockResolvedValue(pollOk())
    await usePollBundle().reconcileNow()
    await vi.advanceTimersByTimeAsync(0)

    expect(status.gapLoaded.value).toBe(true)
    expect(getDailyReportGapsMock).toHaveBeenCalled()
  })
})

describe('useRadarPipelineStatus — 补填状态流转', () => {
  it('提交 → running → 进度 → 终态 done，并重取一次缺口', async () => {
    const status = await setup()

    const pending = status.backfillAll()
    expect(status.backfillPhase.value).toBe('submitting')
    await pending
    expect(status.backfillPhase.value).toBe('running')
    // 一次补齐：窗口与缺口清单同口径
    expect(triggerGapBackfillMock).toHaveBeenCalledWith({ days: 7 })

    // 逐格进度：run_scope=backfill + job_id 匹配
    emitWs('daily_report_progress', {
      job_id: 'bj1', run_scope: 'backfill', date: '2026-10-22', board_id: 12, status: 'completed', saved: 2, progress: '2/5',
    })
    expect(status.backfillProgress.value).toEqual({ filled: 2, total: 5, date: '2026-10-22' })

    // 终态（独立事件类型）→ 重取缺口
    getDailyReportGapsMock.mockClear()
    emitWs('daily_report_backfill_done', {
      job_id: 'bj1', run_id: 'br1', status: 'partial', filled: 3, failed: 1, remaining: 1,
      window_start: '2026-10-18', window_end: '2026-10-24',
    })
    await vi.advanceTimersByTimeAsync(0)

    expect(status.backfillPhase.value).toBe('done')
    expect(status.backfillResult.value).toEqual({
      status: 'partial', filled: 3, failed: 1, remaining: 1, windowStart: '2026-10-18', windowEnd: '2026-10-24',
    })
    expect(getDailyReportGapsMock).toHaveBeenCalledTimes(1)
  })

  it('补填进度只认 run_scope=backfill：当日生成的进度不被吸收', async () => {
    const status = await setup()
    await status.backfillAll()

    emitWs('daily_report_progress', {
      job_id: 'bj1', run_scope: 'generate', date: todayKey(), saved: 9, progress: '9/9',
    })
    expect(status.backfillProgress.value).toEqual({ filled: 0, total: 0, date: '' })
  })

  it('补填进度按 job_id 归属：别的补填 job 不被吸收', async () => {
    const status = await setup()
    await status.backfillAll()

    emitWs('daily_report_progress', {
      job_id: 'other-job', run_scope: 'backfill', date: '2026-10-22', saved: 7, progress: '7/7',
    })
    expect(status.backfillProgress.value).toEqual({ filled: 0, total: 0, date: '' })
  })

  it('补填终态只认本 job：别的 job 的终态不吸收', async () => {
    const status = await setup()
    await status.backfillAll()

    emitWs('daily_report_backfill_done', { job_id: 'other-job', status: 'success', filled: 9, failed: 0, remaining: 0 })
    expect(status.backfillPhase.value).toBe('running')
    expect(status.backfillResult.value).toBeNull()
  })

  it('补这天只提交该日（不为该日之外的缺口生成日报）', async () => {
    const status = await setup()
    await status.backfillDate('2026-10-22')
    expect(triggerGapBackfillMock).toHaveBeenCalledWith({ date: '2026-10-22' })
  })

  it('补填被拒（409）：phase failed + 后端文案（不静默）', async () => {
    const status = await setup()
    triggerGapBackfillMock.mockResolvedValue({
      success: false,
      status: 409,
      error: '打标/向量化队列未排空，补出的候选不全，请稍后再试。',
    })

    await status.backfillDate('2026-10-22')
    expect(status.backfillPhase.value).toBe('failed')
    expect(status.backfillError.value).toContain('队列未排空')
  })

  it('补填进行中：不重复受理', async () => {
    const status = await setup()
    const first = status.backfillAll()
    const second = status.backfillAll()
    await Promise.all([first, second])
    expect(triggerGapBackfillMock).toHaveBeenCalledTimes(1)
  })

  it('队列积压时补填入口禁用（与生成同款双队列判据）', async () => {
    const status = await setup({ pending: 5 })
    expect(status.backfillBlocked.value).toBe(true)
  })

  it('双队列清空时补填入口可用', async () => {
    const status = await setup({ pending: 0, processing: 0, embeddingPending: 0 })
    expect(status.backfillBlocked.value).toBe(false)
  })

  it('补填终态先于受理响应到达：缓冲后回放，不永久卡 running', async () => {
    const status = await setup()
    let resolveTrigger: (value: unknown) => void = () => {}
    triggerGapBackfillMock.mockImplementation(() => new Promise((resolve) => {
      resolveTrigger = resolve
    }))

    const pending = status.backfillAll()
    // 响应尚未返回，终态 WS 先到（快失败场景）
    emitWs('daily_report_backfill_done', {
      job_id: 'bj1', run_id: 'br1', status: 'success', filled: 2, failed: 0, remaining: 0,
    })
    expect(status.backfillPhase.value).toBe('submitting')

    resolveTrigger({
      success: true,
      data: { job_id: 'bj1', run_id: 'br1', status: 'processing', window_start: '', window_end: '' },
    })
    await pending
    await vi.advanceTimersByTimeAsync(0)

    expect(status.backfillPhase.value).toBe('done')
    expect(status.backfillResult.value?.filled).toBe(2)
  })
})
