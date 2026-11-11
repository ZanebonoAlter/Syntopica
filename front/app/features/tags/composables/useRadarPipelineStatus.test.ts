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

const { generateDailyReportMock } = vi.hoisted(() => ({ generateDailyReportMock: vi.fn() }))
vi.mock('~/api/dailyReports', () => ({
  useDailyReportsApi: () => ({ generateDailyReport: generateDailyReportMock }),
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
