/**
 * BoardRadarStatusBlock 测试（add-radar-pipeline-status D9/D10 + R-17）。
 *
 * 覆盖：四项事实 / 状态矩阵（loading / empty / error / success）/
 * 按钮状态机（idle → picker-open → submitting → done / failed）/
 * 勾选交互（默认全选、取消勾选、全不选禁用确认、全选恢复）/
 * 队列门槛不可绕过（清单展开期间积压增长）/ 今日已出的重生成警示 /
 * 终态三态计数（「当日无内容」的板块不计入失败）/ 折叠状态机（默认折叠
 * 一行摘要 + 异常自动展开，matrix-clue-fallback-and-rail-merge D5）。
 *
 * 对账走真实 `usePollBundle`（只 mock `~/api/poll` 端点），因此「常驻状态块
 * 不新建周期性请求」这条契约在用例里可直接断言（请求次数 = 1）。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import BoardRadarStatusBlock from './BoardRadarStatusBlock.vue'
import { usePollBundle } from '~/composables/usePollBundle'
import { todayKey } from '~/utils/todayKey'
import type { RadarStatusBoard } from '../../composables/useRadarPipelineStatus'
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

function row(over: Partial<RadarStatusBoard> = {}): RadarStatusBoard {
  return {
    boardId: '1',
    label: '中东地缘政治与美伊关系',
    reportDate: '2026-10-24',
    todayClues: 8,
    ...over,
  }
}

function schedulers(over: Partial<SchedulerStatus> = {}): SchedulerStatus[] {
  return [{
    name: 'daily_report',
    check_interval: 0,
    schedule_time: '21:00',
    schedule_deadline: '23:30',
    ...over,
  }]
}

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
  /** 旧后端：响应里完全没有 embedding_queue 段。 */
  omitEmbeddingQueue?: boolean
  schedulers?: SchedulerStatus[]
}

function pollOk(over: PollOverrides = {}) {
  const embeddingPending = over.embeddingPending ?? 0
  const embeddingProcessing = over.embeddingProcessing ?? 0
  const embeddingCompletedToday = over.embeddingCompletedToday ?? 0
  return {
    success: true,
    data: {
      schedulers: over.schedulers ?? schedulers(),
      tag_queue: {
        pending: over.pending ?? 0,
        processing: over.processing ?? 0,
        completed: over.completedToday ?? 0,
        total: (over.pending ?? 0) + (over.processing ?? 0) + (over.completedToday ?? 0),
        completed_today: over.completedToday ?? 0,
        failed: over.failed ?? 0,
      },
      ...(over.omitEmbeddingQueue
        ? {}
        : {
            embedding_queue: {
              pending: embeddingPending,
              processing: embeddingProcessing,
              completed: embeddingCompletedToday,
              total: embeddingPending + embeddingProcessing + embeddingCompletedToday,
              completed_today: embeddingCompletedToday,
              failed: over.embeddingFailed ?? 0,
            },
          }),
      notifications: { unread: 0 },
    },
  }
}

/**
 * matrix-clue-fallback-and-rail-merge：状态块默认折叠成一行摘要——既有用例
 * 断言的是展开态五项事实，`collapsed: true` 时不代点（折叠行为专测用）。
 * 异常态挂载即自动展开（watch immediate），代点逻辑不会把它收起。
 */
async function mountBlock(props: {
  rows?: RadarStatusBoard[]
  overviewLoading?: boolean
  overviewError?: string
  /** 不代点展开：折叠/展开专测用。 */
  collapsed?: boolean
} = {}) {
  const wrapper = mount(BoardRadarStatusBlock, {
    props: {
      rows: props.rows ?? [row()],
      overviewLoading: props.overviewLoading ?? false,
      overviewError: props.overviewError ?? '',
    },
  })
  // 等常驻对账分发落地（fake timers 下用 advanceTimersByTimeAsync 冲微任务）
  await vi.advanceTimersByTimeAsync(0)
  if (!props.collapsed && !wrapper.find('[data-testid="radar-status-queue"]').exists()) {
    await wrapper.find('[data-testid="radar-status-summary"]').trigger('click')
    await vi.advanceTimersByTimeAsync(0)
  }
  return wrapper
}

function isDisabled(wrapper: Awaited<ReturnType<typeof mountBlock>>, testid: string): boolean {
  return wrapper.find(`[data-testid="${testid}"]`).attributes('disabled') !== undefined
}

function emitWs(type: string, data: Record<string, unknown>): void {
  wsHandlers.get(type)?.(data)
}

beforeEach(() => {
  vi.useFakeTimers()
  // 固定基准时钟（本地中午）：「今日」与「是否已过生成时刻」的断言可重复
  vi.setSystemTime(new Date(2026, 9, 24, 12, 0, 0))
  wsHandlers.clear()
  getPollBundleMock.mockReset()
  generateDailyReportMock.mockReset()
  getPollBundleMock.mockResolvedValue(pollOk())
})

afterEach(async () => {
  usePollBundle().__resetPollBundleForTest()
  await vi.advanceTimersByTimeAsync(0)
  vi.useRealTimers()
})

describe('BoardRadarStatusBlock — 四项事实', () => {
  it('对账成功 + 总览成功：队列数、完成进度、两个时刻、最新落点、当日状态同时呈现', async () => {
    const wrapper = await mountBlock({
      rows: [
        row({ boardId: '1', reportDate: todayKey() }),
        row({ boardId: '2', label: '另一个板块', reportDate: '2026-10-20' }),
      ],
    })
    expect(wrapper.find('[data-testid="radar-status-queue"]').text()).toBe('队列空闲')
    expect(wrapper.find('[data-testid="radar-status-schedule-time"]').text()).toContain('21:00')
    expect(wrapper.find('[data-testid="radar-status-schedule-deadline"]').text()).toContain('23:30')
    expect(wrapper.find('[data-testid="radar-status-latest"]').text()).toBe(todayKey())
    expect(wrapper.find('[data-testid="radar-status-today"]').text()).toBe('今日日报已出（1/2 板块）')
  })

  it('今日完成进度按「今日完成 / 本轮总量」呈现', async () => {
    getPollBundleMock.mockResolvedValue(pollOk({ pending: 3, processing: 1, completedToday: 62 }))
    const wrapper = await mountBlock()

    expect(wrapper.find('[data-testid="radar-status-queue"]').text()).toBe('待打标 4')
    expect(wrapper.find('[data-testid="radar-status-queue-progress"]').text()).toContain('今日完成 62 / 66')
  })

  it('旧后端缺 schedule_deadline：兜底时刻降级为 23:30，不留空', async () => {
    getPollBundleMock.mockResolvedValue(pollOk({ schedulers: [{ name: 'daily_report', check_interval: 0, schedule_time: '21:00' }] }))
    const wrapper = await mountBlock()
    expect(wrapper.find('[data-testid="radar-status-schedule-deadline"]').text()).toContain('23:30')
  })
})

describe('BoardRadarStatusBlock — 状态矩阵（R-17）', () => {
  it('首次对账未成功：呈现「读取中」，不出现业务结论', async () => {
    getPollBundleMock.mockResolvedValue({ success: false, error: '接口挂了' })
    const wrapper = await mountBlock()

    expect(wrapper.find('[data-testid="radar-status-queue"]').text()).toBe('读取中')
    expect(wrapper.find('[data-testid="radar-status-window-loading"]').text()).toBe('读取中')
    expect(wrapper.find('[data-testid="radar-status-today"]').text()).toBe('读取中')
    expect(wrapper.text()).not.toContain('今日日报未生成')
    expect(wrapper.text()).not.toContain('队列空闲')
    // 默认时刻不是本站真实配置，未取到值前不当事实呈现
    expect(wrapper.find('[data-testid="radar-status-schedule-deadline"]').exists()).toBe(false)
    expect(isDisabled(wrapper, 'radar-status-generate')).toBe(true)
  })

  it('对账成功但无任何板块有日报：成功空集 → 「暂无」+「今日日报未生成」', async () => {
    const wrapper = await mountBlock({ rows: [row({ reportDate: '' })] })
    expect(wrapper.find('[data-testid="radar-status-latest"]').text()).toBe('暂无')
    expect(wrapper.find('[data-testid="radar-status-today"]').text()).toBe('今日日报未生成')
  })

  it('队列清空 + 今日未出：给出缺料原因（未到生成时刻）', async () => {
    const wrapper = await mountBlock({ rows: [row({ reportDate: '2026-10-20' })] })
    expect(wrapper.find('[data-testid="radar-status-today"]').text()).toBe('今日日报未生成')
    expect(wrapper.find('[data-testid="radar-status-not-generated-hint"]').text()).toContain('尚未到生成时刻')
  })

  it('已过生成时刻仍无当日日报：缺料原因改为「今日暂无可用素材或生成未完成」', async () => {
    vi.setSystemTime(new Date(2026, 9, 24, 23, 0, 0))
    const wrapper = await mountBlock({ rows: [row({ reportDate: '2026-10-20' })] })
    expect(wrapper.find('[data-testid="radar-status-today"]').text()).toBe('今日日报未生成')
    expect(wrapper.find('[data-testid="radar-status-not-generated-hint"]').text()).toContain('已过生成时刻')
  })

  it('今日已出时不给缺料原因', async () => {
    const wrapper = await mountBlock({ rows: [row({ reportDate: todayKey() })] })
    expect(wrapper.find('[data-testid="radar-status-today"]').text()).toContain('今日日报已出')
    expect(wrapper.find('[data-testid="radar-status-not-generated-hint"]').exists()).toBe(false)
  })

  it('总览取数失败：队列行照常，日报行「读取失败」，不判定「今日日报未生成」', async () => {
    const wrapper = await mountBlock({ overviewError: '接口挂了' })
    expect(wrapper.find('[data-testid="radar-status-queue"]').text()).toBe('队列空闲')
    expect(wrapper.find('[data-testid="radar-status-today"]').text()).toBe('读取失败')
    expect(wrapper.find('[data-testid="radar-status-latest"]').text()).toBe('读取失败')
    expect(wrapper.text()).not.toContain('今日日报未生成')
  })

  it('总览取数中：日报行「读取中」', async () => {
    const wrapper = await mountBlock({ overviewLoading: true })
    expect(wrapper.find('[data-testid="radar-status-today"]').text()).toBe('读取中')
  })

  it('曾成功后对账失败：保留旧值并标注更新时刻，不清零、不转错误态', async () => {
    const wrapper = await mountBlock()
    const updated = wrapper.find('[data-testid="radar-status-updated"]')
    expect(updated.exists()).toBe(true)
    expect(updated.text()).toMatch(/更新于 \d{2}:\d{2}/)

    getPollBundleMock.mockResolvedValue({ success: false, error: '接口挂了' })
    await usePollBundle().reconcileNow()
    await vi.advanceTimersByTimeAsync(0)
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-testid="radar-status-queue"]').text()).toBe('队列空闲')
    expect(wrapper.find('[data-testid="radar-status-today"]').text()).not.toBe('读取失败')
    expect(wrapper.find('[data-testid="radar-status-updated"]').text()).toBe(updated.text())
  })
})

describe('BoardRadarStatusBlock — 队列门槛与失败提示（D8）', () => {
  it('队列有积压：生成按钮禁用 + 缺料说明', async () => {
    getPollBundleMock.mockResolvedValue(pollOk({ pending: 3, processing: 1 }))
    const wrapper = await mountBlock()

    expect(isDisabled(wrapper, 'radar-status-generate')).toBe(true)
    const shortage = wrapper.find('[data-testid="radar-status-shortage"]')
    expect(shortage.exists()).toBe(true)
    expect(shortage.text()).toContain('不等队列')
    expect(shortage.text()).toContain('当日才完成打标的文章不会进入当日日报')
  })

  it('队列清空：生成按钮可用、缺料说明消失', async () => {
    const wrapper = await mountBlock()
    expect(isDisabled(wrapper, 'radar-status-generate')).toBe(false)
    expect(wrapper.find('[data-testid="radar-status-shortage"]').exists()).toBe(false)
  })

  it('失败任务单独红色提示，且不因此禁用生成', async () => {
    getPollBundleMock.mockResolvedValue(pollOk({ failed: 2 }))
    const wrapper = await mountBlock()
    expect(wrapper.find('[data-testid="radar-status-queue-failed"]').text()).toContain('2 个失败')
    expect(isDisabled(wrapper, 'radar-status-generate')).toBe(false)
  })
})

describe('BoardRadarStatusBlock — embedding 队列（第二环，D14）', () => {
  it('两行并列：标签队列与 Embedding 队列各自呈现自己的计数', async () => {
    getPollBundleMock.mockResolvedValue(pollOk({
      pending: 3,
      processing: 1,
      embeddingPending: 12,
      embeddingProcessing: 2,
    }))
    const wrapper = await mountBlock()

    expect(wrapper.find('[data-testid="radar-status-queue"]').text()).toBe('待打标 4')
    expect(wrapper.find('[data-testid="radar-status-embedding-queue"]').text()).toBe('待向量化 14')
  })

  it('两个队列各带自己的今日完成进度条（量纲不同：文章 vs 标签）', async () => {
    getPollBundleMock.mockResolvedValue(pollOk({
      pending: 3,
      processing: 1,
      completedToday: 62,
      embeddingPending: 2,
      embeddingCompletedToday: 10,
    }))
    const wrapper = await mountBlock()

    expect(wrapper.find('[data-testid="radar-status-queue-progress"]').text()).toContain('今日完成 62 / 66')
    expect(wrapper.find('[data-testid="radar-status-embedding-progress"]').text()).toContain('今日完成 10 / 12 个标签')
  })

  it('标签队列清空但 embedding 队列有积压：仍挡住生成（双队列判据）', async () => {
    getPollBundleMock.mockResolvedValue(pollOk({ embeddingPending: 166 }))
    const wrapper = await mountBlock()

    expect(wrapper.find('[data-testid="radar-status-queue"]').text()).toBe('队列空闲')
    expect(wrapper.find('[data-testid="radar-status-embedding-queue"]').text()).toBe('待向量化 166')
    expect(isDisabled(wrapper, 'radar-status-generate')).toBe(true)
    const shortage = wrapper.find('[data-testid="radar-status-shortage"]')
    expect(shortage.exists()).toBe(true)
    expect(shortage.text()).toContain('待向量化')
    expect(shortage.text()).toContain('泳道分桶')
  })

  it('embedding 队列计数未知（旧后端无该段）：挡住生成且不断言「已清空」', async () => {
    getPollBundleMock.mockResolvedValue(pollOk({ omitEmbeddingQueue: true }))
    const wrapper = await mountBlock()

    expect(wrapper.find('[data-testid="radar-status-embedding-queue"]').text()).toBe('状态未知')
    expect(isDisabled(wrapper, 'radar-status-generate')).toBe(true)
    expect(wrapper.find('[data-testid="radar-status-shortage"]').text()).toContain('状态未知')
  })

  it('双队列均清空：生成按钮可用、缺料说明消失', async () => {
    const wrapper = await mountBlock()
    expect(isDisabled(wrapper, 'radar-status-generate')).toBe(false)
    expect(wrapper.find('[data-testid="radar-status-shortage"]').exists()).toBe(false)
  })

  it('embedding 队列的失败任务单独红色提示，不因此禁用生成', async () => {
    getPollBundleMock.mockResolvedValue(pollOk({ embeddingFailed: 3 }))
    const wrapper = await mountBlock()
    expect(wrapper.find('[data-testid="radar-status-embedding-failed"]').text()).toContain('3 个失败')
    expect(isDisabled(wrapper, 'radar-status-generate')).toBe(false)
  })

  it('清单展开期间 embedding 队列复堵：确认按钮同样禁用（门槛不可绕过）', async () => {
    const wrapper = await mountBlock()
    await wrapper.find('[data-testid="radar-status-generate"]').trigger('click')
    expect(isDisabled(wrapper, 'radar-status-confirm')).toBe(false)

    getPollBundleMock.mockResolvedValue(pollOk({ embeddingPending: 5 }))
    await usePollBundle().reconcileNow()
    await vi.advanceTimersByTimeAsync(0)
    await wrapper.vm.$nextTick()

    expect(isDisabled(wrapper, 'radar-status-confirm')).toBe(true)
  })
})

describe('BoardRadarStatusBlock — 按钮状态机', () => {
  it('idle：只有「立即生成」按钮，清单未展开', async () => {
    const wrapper = await mountBlock()
    expect(wrapper.find('[data-testid="radar-status-generate"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-status-picker"]').exists()).toBe(false)
  })

  it('picker-open：就地展开清单（不弹窗），默认全选并展示参与数 / 总数', async () => {
    const wrapper = await mountBlock({
      rows: [row({ boardId: '1' }), row({ boardId: '2', label: '板块二' })],
    })
    await wrapper.find('[data-testid="radar-status-generate"]').trigger('click')

    expect(wrapper.find('[data-testid="radar-status-picker"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-status-selected-count"]').text()).toContain('参与 2 / 2')
    expect((wrapper.find('[data-testid="radar-status-board-1"]').element as HTMLInputElement).checked).toBe(true)
    expect((wrapper.find('[data-testid="radar-status-board-2"]').element as HTMLInputElement).checked).toBe(true)
  })

  it('submitting → done：按钮进进度态、结果内联、并触发总览刷新', async () => {
    generateDailyReportMock.mockResolvedValue({ success: true, data: { job_id: 'j1', status: 'processing' } })
    const wrapper = await mountBlock()
    await wrapper.find('[data-testid="radar-status-generate"]').trigger('click')
    await wrapper.find('[data-testid="radar-status-confirm"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(isDisabled(wrapper, 'radar-status-confirm')).toBe(true)
    expect(wrapper.find('[data-testid="radar-status-progress"]').exists()).toBe(true)

    // 响应到达 → job_id 落定（之后的 WS 消息按 job_id 归属）
    await vi.advanceTimersByTimeAsync(0)

    emitWs('daily_report_progress', { job_id: 'j1', board_id: 1, status: 'completed', progress: '1/1' })
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="radar-status-progress"]').text()).toContain('1/1')

    emitWs('daily_report_done', { job_id: 'j1', total_saved: 1, total_boards: 1 })
    await wrapper.vm.$nextTick()

    expect(isDisabled(wrapper, 'radar-status-confirm')).toBe(false)
    expect(wrapper.find('[data-testid="radar-status-result"]').text()).toContain('成功 1 板块')
    expect(wrapper.find('[data-testid="radar-status-result"]').text()).toContain('失败 0 板块')
    expect(wrapper.emitted('generated')).toHaveLength(1)
  })

  it('终态三态计数：「当日无内容」的板块不算失败（成功 1 · 失败 0）', async () => {
    generateDailyReportMock.mockResolvedValue({ success: true, data: { job_id: 'j1', status: 'processing' } })
    const wrapper = await mountBlock()
    await wrapper.find('[data-testid="radar-status-generate"]').trigger('click')
    await wrapper.find('[data-testid="radar-status-confirm"]').trigger('click')
    await vi.advanceTimersByTimeAsync(0)

    // 终态消息自带三态计数：1 个成功板 + 1 个当日无内容的板（报告为空）
    emitWs('daily_report_done', {
      job_id: 'j1',
      total_saved: 1,
      total_boards: 2,
      failed_count: 0,
      empty_count: 1,
    })
    await wrapper.vm.$nextTick()

    const result = wrapper.find('[data-testid="radar-status-result"]')
    expect(result.text()).toContain('成功 1 板块')
    expect(result.text()).toContain('失败 0 板块')
    expect(result.text()).not.toContain('失败 1')
  })

  it('并发管线的终态不被吸收：结果仍属本次 job', async () => {
    generateDailyReportMock.mockResolvedValue({ success: true, data: { job_id: 'j1', status: 'processing' } })
    const wrapper = await mountBlock()
    await wrapper.find('[data-testid="radar-status-generate"]').trigger('click')
    await wrapper.find('[data-testid="radar-status-confirm"]').trigger('click')
    await vi.advanceTimersByTimeAsync(0)

    emitWs('daily_report_done', { job_id: 'other-job', total_saved: 9, total_boards: 9 })
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="radar-status-result"]').exists()).toBe(false)
    expect(isDisabled(wrapper, 'radar-status-confirm')).toBe(true)

    emitWs('daily_report_done', { job_id: 'j1', total_saved: 1, total_boards: 1 })
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="radar-status-result"]').text()).toContain('成功 1 板块')
  })

  it('failed（409）：按钮恢复可用 + 内联失败文案（不静默）', async () => {
    generateDailyReportMock.mockResolvedValue({ success: false, status: 409, error: '日报生成正在执行中，请稍后再试。' })
    const wrapper = await mountBlock()
    await wrapper.find('[data-testid="radar-status-generate"]').trigger('click')
    await wrapper.find('[data-testid="radar-status-confirm"]').trigger('click')
    await vi.advanceTimersByTimeAsync(0)
    await wrapper.vm.$nextTick()

    expect(isDisabled(wrapper, 'radar-status-confirm')).toBe(false)
    expect(wrapper.find('[data-testid="radar-status-error"]').text()).toContain('日报生成正在执行中，请稍后再试。')
  })
})

describe('BoardRadarStatusBlock — 勾选交互', () => {
  it('取消勾选某板：参与数下降，请求体只含勾选板块', async () => {
    generateDailyReportMock.mockResolvedValue({ success: true, data: { job_id: 'j1', status: 'processing' } })
    const wrapper = await mountBlock({
      rows: [row({ boardId: '1' }), row({ boardId: '2', label: '板块二' })],
    })
    await wrapper.find('[data-testid="radar-status-generate"]').trigger('click')
    await wrapper.find('[data-testid="radar-status-board-2"]').setValue(false)

    expect(wrapper.find('[data-testid="radar-status-selected-count"]').text()).toContain('参与 1 / 2')

    await wrapper.find('[data-testid="radar-status-confirm"]').trigger('click')
    expect(generateDailyReportMock).toHaveBeenCalledWith({ date: todayKey(), board_ids: [1] })
  })

  it('全不选：确认按钮禁用且不发请求；全选可恢复', async () => {
    const wrapper = await mountBlock({
      rows: [row({ boardId: '1' }), row({ boardId: '2', label: '板块二' })],
    })
    await wrapper.find('[data-testid="radar-status-generate"]').trigger('click')
    await wrapper.find('[data-testid="radar-status-select-none"]').trigger('click')

    expect(wrapper.find('[data-testid="radar-status-selected-count"]').text()).toContain('参与 0 / 2')
    expect(isDisabled(wrapper, 'radar-status-confirm')).toBe(true)

    await wrapper.find('[data-testid="radar-status-confirm"]').trigger('click')
    expect(generateDailyReportMock).not.toHaveBeenCalled()

    await wrapper.find('[data-testid="radar-status-select-all"]').trigger('click')
    expect(wrapper.find('[data-testid="radar-status-selected-count"]').text()).toContain('参与 2 / 2')
    expect(isDisabled(wrapper, 'radar-status-confirm')).toBe(false)
  })

  it('今日无线索的板块标注出来（提示哪些板今天没料）', async () => {
    const wrapper = await mountBlock({
      rows: [row({ boardId: '1', todayClues: 8 }), row({ boardId: '2', label: '板块二', todayClues: 0 })],
    })
    await wrapper.find('[data-testid="radar-status-generate"]').trigger('click')
    expect(wrapper.find('[data-testid="radar-status-board-2"]').element.closest('label')?.textContent).toContain('今日无线索')
  })
})

describe('BoardRadarStatusBlock — 队列门槛不可绕过（M1）', () => {
  it('picker 开启期间积压增长：确认按钮禁用且不发起请求', async () => {
    const wrapper = await mountBlock()
    await wrapper.find('[data-testid="radar-status-generate"]').trigger('click')
    expect(isDisabled(wrapper, 'radar-status-confirm')).toBe(false)

    // 清单展开期间有新文章入队 → 队列门槛重新生效（确认按钮与入口同款判据）
    getPollBundleMock.mockResolvedValue(pollOk({ pending: 3, processing: 1 }))
    await usePollBundle().reconcileNow()
    await vi.advanceTimersByTimeAsync(0)
    await wrapper.vm.$nextTick()

    expect(isDisabled(wrapper, 'radar-status-confirm')).toBe(true)
    await wrapper.find('[data-testid="radar-status-confirm"]').trigger('click')
    expect(generateDailyReportMock).not.toHaveBeenCalled()
  })
})

describe('BoardRadarStatusBlock — 今日已出的重生成警示（M2）', () => {
  it('今日已出：入口按钮文案改为「重新生成今日日报」', async () => {
    const wrapper = await mountBlock({ rows: [row({ reportDate: todayKey() })] })
    expect(wrapper.find('[data-testid="radar-status-generate"]').text()).toBe('重新生成今日日报')
  })

  it('今日已出：清单内给出替换警示，确认按钮文案为「确认替换」', async () => {
    const wrapper = await mountBlock({ rows: [row({ reportDate: todayKey() })] })
    await wrapper.find('[data-testid="radar-status-generate"]').trigger('click')

    const warning = wrapper.find('[data-testid="radar-status-replace-warning"]')
    expect(warning.exists()).toBe(true)
    expect(warning.text()).toContain('将替换今天的日报')
    expect(warning.text()).toContain('标题、章节、线索全部重写')
    expect(wrapper.find('[data-testid="radar-status-confirm"]').text()).toBe('确认替换')
  })

  it('今日未出：入口按钮文案为「立即生成今日日报」，无替换警示', async () => {
    const wrapper = await mountBlock({ rows: [row({ reportDate: '2026-10-20' })] })
    expect(wrapper.find('[data-testid="radar-status-generate"]').text()).toBe('立即生成今日日报')

    await wrapper.find('[data-testid="radar-status-generate"]').trigger('click')
    expect(wrapper.find('[data-testid="radar-status-replace-warning"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="radar-status-confirm"]').text()).toBe('确认生成')
  })
})

describe('BoardRadarStatusBlock — 缺料文案随对账重算（L3）', () => {
  it('页面停驻跨过生成时刻：下一次对账后文案从「尚未到」变成「已过」', async () => {
    vi.setSystemTime(new Date(2026, 9, 24, 20, 59, 0))
    const wrapper = await mountBlock({ rows: [row({ reportDate: '2026-10-20' })] })
    expect(wrapper.find('[data-testid="radar-status-not-generated-hint"]').text()).toContain('尚未到生成时刻')

    vi.setSystemTime(new Date(2026, 9, 24, 21, 5, 0))
    getPollBundleMock.mockResolvedValue(pollOk())
    await usePollBundle().reconcileNow()
    await vi.advanceTimersByTimeAsync(0)
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-testid="radar-status-not-generated-hint"]').text()).toContain('已过生成时刻')
  })
})


// ── 折叠状态机（matrix-clue-fallback-and-rail-merge D5）────────────────────
describe('BoardRadarStatusBlock — 默认折叠与异常自动展开', () => {
  it('正常态默认折叠成一行摘要：五项事实与生成入口不渲染', async () => {
    const wrapper = await mountBlock({
      rows: [
        row({ boardId: '1', reportDate: todayKey() }),
        row({ boardId: '2', label: '另一个板块', reportDate: todayKey() }),
      ],
      collapsed: true,
    })
    const summary = wrapper.find('[data-testid="radar-status-summary"]')
    expect(summary.exists()).toBe(true)
    expect(summary.text()).toContain('管线正常')
    expect(summary.text()).toContain('日报 2/2')
    expect(summary.text()).toContain('队列 0')
    // 折叠 = 详情不在 DOM（不是 display:none）
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="radar-status-today"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="radar-status-generate"]').exists()).toBe(false)
  })

  it('点击摘要行展开五项事实，再点收起（aria-expanded 随态）', async () => {
    const wrapper = await mountBlock({ rows: [row({ reportDate: todayKey() })], collapsed: true })
    // 摘要行常驻（展开态也在顶端作为状态句 + 折叠入口）
    expect(wrapper.find('[data-testid="radar-status-summary"]').attributes('aria-expanded')).toBe('false')
    await wrapper.find('[data-testid="radar-status-summary"]').trigger('click')
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-status-summary"]').attributes('aria-expanded')).toBe('true')
    // 展开态另有「收起」入口
    await wrapper.find('[data-testid="radar-status-collapse"]').trigger('click')
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="radar-status-summary"]').attributes('aria-expanded')).toBe('false')
  })

  it('已过生成时刻仍未出当日日报：挂载即自动展开，摘要行带异常词', async () => {
    vi.setSystemTime(new Date(2026, 9, 24, 23, 0, 0))
    const wrapper = await mountBlock({ rows: [row({ reportDate: '2026-10-20' })], collapsed: true })
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-status-today"]').text()).toBe('今日日报未生成')
    const summary = wrapper.find('[data-testid="radar-status-summary"]')
    expect(summary.text()).toContain('日报未生成')
  })

  it('未到生成时刻的未出日报不算异常：保持折叠（正常态）', async () => {
    // 基准时钟 12:00 < schedule_time 21:00
    const wrapper = await mountBlock({ rows: [row({ reportDate: '2026-10-20' })], collapsed: true })
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="radar-status-summary"]').text()).not.toContain('日报未生成')
  })

  it('队列失败任务 > 0：自动展开，摘要行带失败词**与条数**（折叠不得藏异常计数）', async () => {
    getPollBundleMock.mockResolvedValue(pollOk({ failed: 2 }))
    const wrapper = await mountBlock({ rows: [row({ reportDate: todayKey() })], collapsed: true })
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-status-queue-failed"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-status-summary"]').text()).toContain('失败 2')
  })

  // 注意：对账状态是模块级单例，同一用例内连续 mount 不会重新取数——阈值两侧
  // 拆成两个用例，靠 afterEach 的 __resetPollBundleForTest 隔离。
  it('标签队列积压 99（未达 attention 阈值）：保持折叠', async () => {
    getPollBundleMock.mockResolvedValue(pollOk({ pending: 99 }))
    const wrapper = await mountBlock({ rows: [row({ reportDate: todayKey() })], collapsed: true })
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(false)
  })

  it('标签队列积压达 100（明显积压阈值）：自动展开，摘要行带「队列积压」', async () => {
    getPollBundleMock.mockResolvedValue(pollOk({ pending: 100 }))
    const wrapper = await mountBlock({ rows: [row({ reportDate: todayKey() })], collapsed: true })
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-status-summary"]').text()).toContain('队列积压')
  })

  it('异常消除后回到折叠摘要（用户未手动操作时不保持展开）', async () => {
    vi.setSystemTime(new Date(2026, 9, 24, 23, 0, 0))
    const wrapper = await mountBlock({ rows: [row({ boardId: '1', reportDate: '2026-10-20' })], collapsed: true })
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(true)
    // 当日日报落出 → 异常消除 → 折叠
    await wrapper.setProps({ rows: [row({ boardId: '1', reportDate: todayKey() })] })
    await vi.advanceTimersByTimeAsync(0)
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="radar-status-summary"]').text()).toContain('管线正常')
  })

  it('取数窗口内（overviewLoading）不撤销异常展开：不出现收起→再展开抖动（review R-3）', async () => {
    vi.setSystemTime(new Date(2026, 9, 24, 23, 0, 0))
    const wrapper = await mountBlock({ rows: [row({ reportDate: '2026-10-20' })], collapsed: true })
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(true)

    // 加载中 = 未知，不是「异常消除」：必须保持展开，不能先收起
    await wrapper.setProps({ overviewLoading: true })
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(true)

    await wrapper.setProps({ overviewLoading: false })
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(true)
  })

  it('用户手动收起后，后续异常状态跳变不覆盖手动选择（review R-3）', async () => {
    vi.setSystemTime(new Date(2026, 9, 24, 23, 0, 0))
    const wrapper = await mountBlock({ rows: [row({ reportDate: '2026-10-20' })], collapsed: true })
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(true)

    // 手动收起
    await wrapper.find('[data-testid="radar-status-collapse"]').trigger('click')
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(false)

    // 异常状态跳变（队列积压新触发）不应把手动收起改回展开
    getPollBundleMock.mockResolvedValue(pollOk({ pending: 100 }))
    await usePollBundle().reconcileNow()
    await vi.advanceTimersByTimeAsync(0)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(false)
  })

  it('对账从未成功：折叠摘要「读取中」，不把未知当业务结论（R-17）', async () => {
    getPollBundleMock.mockResolvedValue({ success: false, error: '接口挂了' })
    const wrapper = await mountBlock({ collapsed: true })
    const summary = wrapper.find('[data-testid="radar-status-summary"]')
    expect(summary.text()).toContain('读取中')
    expect(summary.text()).not.toContain('未生成')
    expect(wrapper.text()).not.toContain('今日日报未生成')
  })

  it('生成请求失败（phase failed）：异常即自动展开，摘要行带「生成失败」，失败文案内联', async () => {
    generateDailyReportMock.mockResolvedValue({ success: false, status: 409, error: '日报生成正在执行中，请稍后再试。' })
    const wrapper = await mountBlock({ rows: [row({ reportDate: todayKey() })], collapsed: true })
    // 正常态先折叠
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(false)

    await wrapper.find('[data-testid="radar-status-summary"]').trigger('click')
    await wrapper.find('[data-testid="radar-status-generate"]').trigger('click')
    await wrapper.find('[data-testid="radar-status-confirm"]').trigger('click')
    await vi.advanceTimersByTimeAsync(0)
    await wrapper.vm.$nextTick()

    // 生成失败属异常态：摘要行状态词为「生成失败」，且强制展开（详情在 DOM）
    expect(wrapper.find('[data-testid="radar-status-summary"]').text()).toContain('生成失败')
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-status-error"]').text()).toContain('日报生成正在执行中，请稍后再试。')
  })

  it('总览取数失败：摘要日报段「读取失败」，不判「未生成」', async () => {
    const wrapper = await mountBlock({ overviewError: '超时', collapsed: true })
    const summary = wrapper.find('[data-testid="radar-status-summary"]')
    expect(summary.text()).toContain('读取失败')
    expect(summary.text()).not.toContain('未生成')
  })

  // ── matrix-clue R-3：异常态变化不得在取数窗口内翻转折叠态；用户手动态粘滞 ──

  it('取数窗口内异常判据变「未知」：不收起（收起→展开抖动消失）', async () => {
    vi.setSystemTime(new Date(2026, 9, 24, 23, 0, 0))
    const wrapper = await mountBlock({ rows: [row({ reportDate: '2026-10-20' })], collapsed: true })
    // 挂载即异常（已过生成时刻未出日报）→ 自动展开
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(true)

    // 取数窗口（重试 / 手动生成后的 load()）：数据变「未知」，不是「异常消除」
    await wrapper.setProps({ overviewLoading: true })
    await vi.advanceTimersByTimeAsync(0)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-status-summary"]').attributes('aria-expanded')).toBe('true')

    // 数据回来、异常仍在 → 仍展开（全程没有收起→展开翻转）
    await wrapper.setProps({ overviewLoading: false })
    await vi.advanceTimersByTimeAsync(0)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-status-summary"]').attributes('aria-expanded')).toBe('true')
  })

  it('用户手动展开后：正常态不自动收回（手动态粘滞）', async () => {
    const wrapper = await mountBlock({ rows: [row({ reportDate: todayKey() })], collapsed: true })
    // 正常态默认折叠
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(false)
    await wrapper.find('[data-testid="radar-status-summary"]').trigger('click')
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(true)

    // 数据重算（重新取数 / 重新渲染）不得把用户展开收回
    await wrapper.setProps({ rows: [row({ reportDate: todayKey() })] })
    await vi.advanceTimersByTimeAsync(0)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-status-summary"]').attributes('aria-expanded')).toBe('true')
  })

  it('用户手动收起后真实异常出现：仍自动展开（异常态 SHALL 自动展开）', async () => {
    const wrapper = await mountBlock({ rows: [row({ reportDate: todayKey() })], collapsed: true })
    // 用户手动展开再收起（异常未变迁，手动意图生效）
    await wrapper.find('[data-testid="radar-status-summary"]').trigger('click')
    await wrapper.find('[data-testid="radar-status-collapse"]').trigger('click')
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(false)

    // 真实异常出现（标签队列积压达明显阈值）→ 自动展开（手动意图作废、交还自动态）
    getPollBundleMock.mockResolvedValue(pollOk({ pending: 100 }))
    await usePollBundle().reconcileNow()
    await vi.advanceTimersByTimeAsync(0)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-status-summary"]').attributes('aria-expanded')).toBe('true')
  })

  it('异常消除后回到折叠摘要：真实变迁交还控制权给自动态', async () => {
    vi.setSystemTime(new Date(2026, 9, 24, 23, 0, 0))
    const wrapper = await mountBlock({ rows: [row({ reportDate: '2026-10-20' })], collapsed: true })
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(true)
    // 用户在异常态手动收起 → 折叠（两次真实变迁之间用户意图生效）
    await wrapper.find('[data-testid="radar-status-collapse"]').trigger('click')
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(false)

    // 异常消除（当日日报落出）→ 真实变迁 → 回折叠摘要
    await wrapper.setProps({ rows: [row({ reportDate: todayKey() })] })
    await vi.advanceTimersByTimeAsync(0)
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="radar-status-summary"]').attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('[data-testid="radar-status-summary"]').text()).toContain('管线正常')
  })

  // ── matrix-clue R-4：折叠摘要行给出失败任务条数（折叠不藏异常计数）────────

  it('折叠摘要行给出失败任务条数（折叠态也不藏计数，两条队列不相加）', async () => {
    getPollBundleMock.mockResolvedValue(pollOk({ failed: 2, embeddingFailed: 1 }))
    const wrapper = await mountBlock({ rows: [row({ reportDate: todayKey() })], collapsed: true })
    // 失败属异常 → 自动展开
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(true)

    // 用户手动收起：折叠态摘要行仍带失败词与条数
    await wrapper.find('[data-testid="radar-status-collapse"]').trigger('click')
    const summary = wrapper.find('[data-testid="radar-status-summary"]')
    expect(summary.text()).toContain('队列有失败')
    // 标签队列 2 个失败（文章）+ Embedding 队列 1 个失败（标签）→ 分段写法 2+1
    expect(summary.text()).toContain('2+1')
    expect(summary.text()).not.toContain('队列有失败 3')
  })
})
