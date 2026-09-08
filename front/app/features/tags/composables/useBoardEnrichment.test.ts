/**
 * useBoardEnrichment — board 档异步任务轮询（board-level-deep-analysis 5.5）。
 *
 * 覆盖 M9 契约：
 *  - 202 触发：保存 job_id/job_kind，按 job_id 精确轮询（不再只按 board 猜任务）
 *  - 409 冲突：从 error.data 恢复当前任务轮询，不重复启动；kind 文案区分
 *  - brief 完成：reload + 选中新简报
 *  - investigation 完成：不重拉列表、不覆盖当前简报选择（不冒充）
 *  - 重进恢复：scope/id 入口发现 running job → 转 job_id 精确轮询；idle 不轮询
 *  - error/404：停止轮询并提示；瞬时错误继续轮询
 *  - unmount 清 timer
 *  - 旧 legacy 数据：默认选中最新 board_brief；无 kind 行降级不崩
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import type { BoardAnalysisResultRow, ResultSummaryRow, ResultDetailRow, ReviewRow, StockDebateResult, ContextRow, TopicEnrichmentQA } from '~/api/boardEnrichment'
import type { BoardTopicListItem } from '~/api/dailyReports'

// ── mocks（先声明后导入，Vitest hoist）─────────────────────────────────
const apiMocks = vi.hoisted(() => ({
  triggerBoardAnalysis: vi.fn(),
  triggerBoardInvestigation: vi.fn(),
  getAnalysisStatusByJobId: vi.fn(),
  getAnalysisStatus: vi.fn(),
  listBoardAnalysisResults: vi.fn(),
  // board qa（6.2：版块报告追问）
  askBoardQA: vi.fn(),
  listBoardQA: vi.fn(),
  sedimentBoardQA: vi.fn(),
  // topic qa（causal-analysis-agent 阶段3，6.x review：视图身份守卫）
  askQA: vi.fn(),
  listQA: vi.fn(),
  sedimentQA: vi.fn(),
  // topic 档 / debate（5.5 review 剩余 Important：切板块/换 topic 隔离）
  triggerEnrichment: vi.fn(),
  listContexts: vi.fn(),
  listResults: vi.fn(),
  listReviews: vi.fn(),
  getResult: vi.fn(),
  listDebates: vi.fn(),
  triggerDebate: vi.fn(),
}))

const notifyMocks = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warn: vi.fn(),
}))

vi.mock('~/api/boardEnrichment', () => ({
  useBoardEnrichmentApi: () => apiMocks,
}))
vi.mock('~/composables/useNotify', () => ({
  useNotify: () => notifyMocks,
}))
// loadTopics 走 reportsApi.listBoardTopics（5.4/5.5 终审 Important：失败语义/
// 切板块清场测试需手动控制该响应）
const reportsMocks = vi.hoisted(() => ({
  listBoardTopics: vi.fn(),
}))
vi.mock('~/api/dailyReports', () => ({
  useDailyReportsApi: () => reportsMocks,
}))

const { useBoardEnrichment } = await import('~/features/tags/composables/useBoardEnrichment')

type En = ReturnType<typeof useBoardEnrichment>

/** 挂一个 harness 组件让 onUnmounted 生效（composable 在 setup 内调用）。 */
let en: En | null = null
let wrapper: ReturnType<typeof mount> | null = null
const Harness = defineComponent({
  setup() {
    en = useBoardEnrichment()
    return () => h('div')
  },
})
function setup(): En {
  en = null
  wrapper = mount(Harness)
  return en!
}

function makeRow(id: number, kind?: string): BoardAnalysisResultRow {
  return { id, analysis_scope: 'board', result_kind: kind, sectors: null, created_at: '2026-09-01T10:00:00Z' }
}

function runningSt(jobId: string, jobKind: string) {
  return { success: true, data: { job_id: jobId, job_kind: jobKind, scope: 'board', target_id: 7701, running: true, started_at: '2026-09-01T00:00:00Z', finished: false } }
}
function finishedSt(jobId: string, jobKind: string, extra: Record<string, unknown> = {}) {
  return { success: true, data: { job_id: jobId, job_kind: jobKind, scope: 'board', target_id: 7701, running: false, finished: true, ...extra } }
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  apiMocks.getAnalysisStatusByJobId.mockResolvedValue({ success: true, data: undefined })
  apiMocks.listBoardAnalysisResults.mockResolvedValue({ success: true, data: [] })
  apiMocks.getAnalysisStatus.mockResolvedValue({ success: true, data: undefined })
  apiMocks.triggerEnrichment.mockResolvedValue({ success: true, data: { status: 'started', scope: 'topic', target_id: 101 } })
  apiMocks.listContexts.mockResolvedValue({ success: true, data: [] })
  apiMocks.listResults.mockResolvedValue({ success: true, data: [] })
  apiMocks.listReviews.mockResolvedValue({ success: true, data: [] })
  apiMocks.getResult.mockResolvedValue({ success: true, data: null })
  apiMocks.listDebates.mockResolvedValue({ success: true, data: [] })
  apiMocks.askBoardQA.mockResolvedValue({ success: true, data: { answer: '答', tool_calls: [], refs: [] } })
  apiMocks.listBoardQA.mockResolvedValue({ success: true, data: [] })
  apiMocks.sedimentBoardQA.mockResolvedValue({ success: true, data: null })
  apiMocks.askQA.mockResolvedValue({ success: true, data: { answer: '答', tool_calls: [], refs: [] } })
  apiMocks.listQA.mockResolvedValue({ success: true, data: [] })
  apiMocks.sedimentQA.mockResolvedValue({ success: true, data: null })
  reportsMocks.listBoardTopics.mockResolvedValue({ success: true, data: { topics: [] } })
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  en = null
  vi.useRealTimers()
})

describe('board 档按 job_id 轮询（5.5）', () => {
  it('202 触发：保存 job 身份并立即按 job_id 精确轮询', async () => {
    const c = setup()
    apiMocks.triggerBoardAnalysis.mockResolvedValue({
      success: true,
      data: { status: 'started', job_id: 'j1', job_kind: 'board_brief', scope: 'board', target_id: 7701 },
    })
    apiMocks.getAnalysisStatusByJobId.mockResolvedValueOnce(runningSt('j1', 'board_brief'))

    await c.triggerBoardAnalysis(7701)
    await vi.advanceTimersByTimeAsync(0)

    expect(apiMocks.getAnalysisStatusByJobId).toHaveBeenCalledWith('j1')
    expect(c.activeBoardJob.value).toEqual({ jobId: 'j1', jobKind: 'board_brief' })
    expect(c.boardAnalysisTriggering.value).toBe(true)
  })

  it('brief 完成：重拉列表并选中新简报 result_id', async () => {
    const c = setup()
    apiMocks.triggerBoardAnalysis.mockResolvedValue({
      success: true,
      data: { status: 'started', job_id: 'j1', job_kind: 'board_brief', scope: 'board', target_id: 7701 },
    })
    apiMocks.getAnalysisStatusByJobId
      .mockResolvedValueOnce(runningSt('j1', 'board_brief'))
      .mockResolvedValueOnce(runningSt('j1', 'board_brief'))
      .mockResolvedValueOnce(finishedSt('j1', 'board_brief', { result_id: 55 }))
    apiMocks.listBoardAnalysisResults.mockResolvedValue({
      success: true,
      data: [makeRow(55, 'board_brief'), makeRow(42, 'board_brief')],
    })

    await c.triggerBoardAnalysis(7701)
    await vi.advanceTimersByTimeAsync(0) // 第一次 poll：running
    await vi.advanceTimersByTimeAsync(3000) // 第二次 poll：running
    await vi.advanceTimersByTimeAsync(3000) // 第三次 poll：finished

    expect(notifyMocks.success).toHaveBeenCalledWith('简报已生成')
    expect(c.boardResults.value.map(r => r.id)).toEqual([55, 42])
    expect(c.selectedBoardResultId.value).toBe(55)
    expect(c.selectedBoardResult.value?.result_kind).toBe('board_brief')
    expect(c.boardAnalysisTriggering.value).toBe(false)
    expect(c.activeBoardJob.value).toBeNull()
  })

  it('409 冲突：从 error.data 恢复当前任务轮询（不重复启动），kind 文案区分', async () => {
    const c = setup()
    apiMocks.triggerBoardAnalysis.mockResolvedValue({
      success: false,
      status: 409,
      error: 'board analysis already running',
      data: { job_id: 'j9', job_kind: 'board_investigation', scope: 'board', target_id: 7701, running: true },
    })
    apiMocks.getAnalysisStatusByJobId.mockResolvedValue(runningSt('j9', 'board_investigation'))

    const ok = await c.triggerBoardAnalysis(7701)
    await vi.advanceTimersByTimeAsync(0)

    expect(ok).toBe(true)
    expect(apiMocks.triggerBoardAnalysis).toHaveBeenCalledTimes(1) // 不重复启动
    expect(apiMocks.getAnalysisStatusByJobId).toHaveBeenCalledWith('j9')
    expect(c.activeBoardJob.value).toEqual({ jobId: 'j9', jobKind: 'board_investigation' })
    expect(notifyMocks.warn).toHaveBeenCalledWith('该板块正在调查中，已恢复进度显示')
  })

  it('409 冲突 + board_brief 在跑：文案为「正在生成简报」', async () => {
    const c = setup()
    apiMocks.triggerBoardAnalysis.mockResolvedValue({
      success: false,
      status: 409,
      error: 'board analysis already running',
      data: { job_id: 'j2', job_kind: 'board_brief', scope: 'board', target_id: 7701, running: true },
    })
    apiMocks.getAnalysisStatusByJobId.mockResolvedValue(runningSt('j2', 'board_brief'))

    await c.triggerBoardAnalysis(7701)
    await vi.advanceTimersByTimeAsync(0)
    expect(notifyMocks.warn).toHaveBeenCalledWith('该板块正在生成简报，已恢复进度显示')
  })

  it('investigation 完成：重拉列表并选中调查行（报告立即出现），但不冒充简报（latestBoardBrief 仍认 board_brief）', async () => {
    const c = setup()
    // 先有旧简报被选中
    apiMocks.listBoardAnalysisResults.mockResolvedValue({
      success: true,
      data: [makeRow(42, 'board_brief')],
    })
    await c.loadBoardAnalysisResults(7701)
    expect(c.selectedBoardResultId.value).toBe(42)

    // 409 恢复到在跑的 investigation
    apiMocks.triggerBoardAnalysis.mockResolvedValue({
      success: false,
      status: 409,
      error: 'board analysis already running',
      data: { job_id: 'j9', job_kind: 'board_investigation', scope: 'board', target_id: 7701, running: true },
    })
    apiMocks.getAnalysisStatusByJobId
      .mockResolvedValueOnce(runningSt('j9', 'board_investigation'))
      .mockResolvedValueOnce(finishedSt('j9', 'board_investigation', { result_id: 77 }))
    apiMocks.listBoardAnalysisResults.mockResolvedValue({
      success: true,
      // 重拉后最新一行是调查（id 更大），简报 42 仍在列表里
      data: [makeRow(77, 'board_investigation'), makeRow(42, 'board_brief')],
    })

    await c.triggerBoardAnalysis(7701)
    await vi.advanceTimersByTimeAsync(0) // running
    await vi.advanceTimersByTimeAsync(3000) // finished（investigation）

    expect(apiMocks.listBoardAnalysisResults).toHaveBeenCalledTimes(2) // 手动拉 1 次 + 完成 reload 1 次
    expect(c.selectedBoardResultId.value).toBe(77) // 调查行被选中 → 调查报告立即出现
    expect(c.selectedBoardResult.value?.result_kind).toBe('board_investigation')
    // 「最新简报」计算不把调查当 brief
    expect(c.latestBoardBrief.value?.id).toBe(42)
    expect(notifyMocks.success).toHaveBeenCalledWith('调查已完成')
    expect(c.boardAnalysisTriggering.value).toBe(false)
  })

  it('重进恢复：scope/id 入口发现 running job → 转 job_id 精确轮询', async () => {
    const c = setup()
    apiMocks.getAnalysisStatus.mockResolvedValue({
      success: true,
      data: { scope: 'board', target_id: 7701, running: true, job_id: 'j5', job_kind: 'board_brief' },
    })

    await c.syncBoardAnalysisStatus(7701)
    await vi.advanceTimersByTimeAsync(0)

    expect(apiMocks.getAnalysisStatusByJobId).toHaveBeenCalledWith('j5')
    expect(c.activeBoardJob.value).toEqual({ jobId: 'j5', jobKind: 'board_brief' })
  })

  it('重进恢复：idle（无 running）不启动轮询', async () => {
    const c = setup()
    apiMocks.getAnalysisStatus.mockResolvedValue({
      success: true,
      data: { scope: 'board', target_id: 7701, running: false, finished: false },
    })

    await c.syncBoardAnalysisStatus(7701)
    await vi.advanceTimersByTimeAsync(3000)

    expect(apiMocks.getAnalysisStatusByJobId).not.toHaveBeenCalled()
    expect(c.boardAnalysisTriggering.value).toBe(false)
  })

  it('job error：停止轮询并提示（brief 文案）', async () => {
    const c = setup()
    apiMocks.triggerBoardAnalysis.mockResolvedValue({
      success: true,
      data: { status: 'started', job_id: 'j1', job_kind: 'board_brief', scope: 'board', target_id: 7701 },
    })
    apiMocks.getAnalysisStatusByJobId
      .mockResolvedValueOnce(runningSt('j1', 'board_brief'))
      .mockResolvedValueOnce(finishedSt('j1', 'board_brief', { error: 'LLM 超时' }))

    await c.triggerBoardAnalysis(7701)
    await vi.advanceTimersByTimeAsync(0)
    await vi.advanceTimersByTimeAsync(3000) // finished + error

    expect(notifyMocks.error).toHaveBeenCalledWith('简报生成失败：LLM 超时')
    expect(c.boardAnalysisTriggering.value).toBe(false)
    const calls = apiMocks.getAnalysisStatusByJobId.mock.calls.length
    await vi.advanceTimersByTimeAsync(6000)
    expect(apiMocks.getAnalysisStatusByJobId.mock.calls.length).toBe(calls) // 已停
  })

  it('unknown job_id（404）：停止轮询并如实提示', async () => {
    const c = setup()
    apiMocks.triggerBoardAnalysis.mockResolvedValue({
      success: true,
      data: { status: 'started', job_id: 'j1', job_kind: 'board_brief', scope: 'board', target_id: 7701 },
    })
    apiMocks.getAnalysisStatusByJobId
      .mockResolvedValueOnce(runningSt('j1', 'board_brief'))
      .mockResolvedValueOnce({ success: false, status: 404, error: 'unknown job_id' })

    await c.triggerBoardAnalysis(7701)
    await vi.advanceTimersByTimeAsync(0)
    await vi.advanceTimersByTimeAsync(3000)

    expect(notifyMocks.error).toHaveBeenCalledWith('任务已失效（服务可能重启过），请重新触发')
    expect(c.boardAnalysisTriggering.value).toBe(false)
  })

  it('瞬时网络错误：继续轮询不停止', async () => {
    const c = setup()
    apiMocks.triggerBoardAnalysis.mockResolvedValue({
      success: true,
      data: { status: 'started', job_id: 'j1', job_kind: 'board_brief', scope: 'board', target_id: 7701 },
    })
    apiMocks.getAnalysisStatusByJobId
      .mockResolvedValueOnce(runningSt('j1', 'board_brief'))
      .mockResolvedValueOnce({ success: false, error: '网络错误' }) // 无 status
      .mockResolvedValueOnce(finishedSt('j1', 'board_brief', { result_id: 55 }))
    apiMocks.listBoardAnalysisResults.mockResolvedValue({
      success: true,
      data: [makeRow(55, 'board_brief')],
    })

    await c.triggerBoardAnalysis(7701)
    await vi.advanceTimersByTimeAsync(0)
    await vi.advanceTimersByTimeAsync(3000) // 瞬时错误
    expect(c.boardAnalysisTriggering.value).toBe(true) // 未停
    await vi.advanceTimersByTimeAsync(3000) // finished
    expect(notifyMocks.success).toHaveBeenCalledWith('简报已生成')
  })

  it('unmount：清掉轮询 timer，不再发请求', async () => {
    const c = setup()
    apiMocks.triggerBoardAnalysis.mockResolvedValue({
      success: true,
      data: { status: 'started', job_id: 'j1', job_kind: 'board_brief', scope: 'board', target_id: 7701 },
    })
    apiMocks.getAnalysisStatusByJobId.mockResolvedValue(runningSt('j1', 'board_brief'))

    await c.triggerBoardAnalysis(7701)
    await vi.advanceTimersByTimeAsync(0)
    wrapper!.unmount()
    wrapper = null // 防 afterEach 重复 unmount

    const calls = apiMocks.getAnalysisStatusByJobId.mock.calls.length
    await vi.advanceTimersByTimeAsync(9000)
    expect(apiMocks.getAnalysisStatusByJobId.mock.calls.length).toBe(calls)
  })
})

describe('调查触发 triggerBoardInvestigation（board-level-deep-analysis 5.7 接线）', () => {
  it('202 触发：按 job_id 精确轮询，任务身份为 board_investigation', async () => {
    const c = setup()
    apiMocks.triggerBoardInvestigation.mockResolvedValue({
      success: true,
      data: { status: 'started', job_id: 'inv1', job_kind: 'board_investigation', scope: 'board', target_id: 7701 },
    })
    apiMocks.getAnalysisStatusByJobId.mockResolvedValue(runningSt('inv1', 'board_investigation'))

    const ok = await c.triggerBoardInvestigation(7701, { briefing_result_id: 42, question_id: 'q1' })
    await vi.advanceTimersByTimeAsync(0)

    expect(ok).toBe(true)
    expect(apiMocks.triggerBoardInvestigation).toHaveBeenCalledWith(7701, { briefing_result_id: 42, question_id: 'q1' })
    expect(notifyMocks.success).toHaveBeenCalledWith('调查已在后台开始，可离开页面')
    expect(apiMocks.getAnalysisStatusByJobId).toHaveBeenCalledWith('inv1')
    expect(c.activeBoardJob.value).toEqual({ jobId: 'inv1', jobKind: 'board_investigation' })
    expect(c.boardAnalysisTriggering.value).toBe(true)
  })

  it('202 触发（custom 问题）：payload 原样传 question 文本', async () => {
    const c = setup()
    apiMocks.triggerBoardInvestigation.mockResolvedValue({
      success: true,
      data: { status: 'started', job_id: 'inv2', job_kind: 'board_investigation', scope: 'board', target_id: 7701 },
    })
    apiMocks.getAnalysisStatusByJobId.mockResolvedValue(runningSt('inv2', 'board_investigation'))

    await c.triggerBoardInvestigation(7701, { briefing_result_id: 42, question: '自填的问题？' })
    await vi.advanceTimersByTimeAsync(0)

    expect(apiMocks.triggerBoardInvestigation).toHaveBeenCalledWith(7701, { briefing_result_id: 42, question: '自填的问题？' })
  })

  it('409 冲突：从 error.data 恢复当前任务轮询（在跑的是 brief 也照常恢复）', async () => {
    const c = setup()
    apiMocks.triggerBoardInvestigation.mockResolvedValue({
      success: false,
      status: 409,
      error: 'board analysis already running',
      data: { job_id: 'jb1', job_kind: 'board_brief', scope: 'board', target_id: 7701, running: true },
    })
    apiMocks.getAnalysisStatusByJobId.mockResolvedValue(runningSt('jb1', 'board_brief'))

    const ok = await c.triggerBoardInvestigation(7701, { briefing_result_id: 42, question_id: 'q1' })
    await vi.advanceTimersByTimeAsync(0)

    expect(ok).toBe(true)
    expect(apiMocks.triggerBoardInvestigation).toHaveBeenCalledTimes(1) // 不重复启动
    expect(apiMocks.getAnalysisStatusByJobId).toHaveBeenCalledWith('jb1')
    expect(c.activeBoardJob.value).toEqual({ jobId: 'jb1', jobKind: 'board_brief' })
    expect(notifyMocks.warn).toHaveBeenCalledWith('该板块正在生成简报，已恢复进度显示')
  })

  it('同步 400（父结果不是 board_brief）：明确报错、不启动轮询、释放 triggering', async () => {
    const c = setup()
    apiMocks.triggerBoardInvestigation.mockResolvedValue({
      success: false,
      status: 400,
      error: 'briefing result is legacy_board_analysis, not a board_brief',
    })

    const ok = await c.triggerBoardInvestigation(7701, { briefing_result_id: 7, question_id: 'q1' })
    await vi.advanceTimersByTimeAsync(3000)

    expect(ok).toBe(false)
    expect(notifyMocks.error).toHaveBeenCalledWith('briefing result is legacy_board_analysis, not a board_brief')
    expect(apiMocks.getAnalysisStatusByJobId).not.toHaveBeenCalled() // 不启动 poll
    expect(c.activeBoardJob.value).toBeNull()
    expect(c.boardAnalysisTriggering.value).toBe(false)
  })

  it('同步 404（父简报不存在）：明确报错、不启动轮询', async () => {
    const c = setup()
    apiMocks.triggerBoardInvestigation.mockResolvedValue({
      success: false,
      status: 404,
      error: 'briefing result not found',
    })

    const ok = await c.triggerBoardInvestigation(7701, { briefing_result_id: 999, question: '？' })
    await vi.advanceTimersByTimeAsync(3000)

    expect(ok).toBe(false)
    expect(notifyMocks.error).toHaveBeenCalledWith('briefing result not found')
    expect(apiMocks.getAnalysisStatusByJobId).not.toHaveBeenCalled()
    expect(c.boardAnalysisTriggering.value).toBe(false)
  })

  it('调查完成：reload + 选中调查行，报告立即出现（不冒充简报）', async () => {
    const c = setup()
    apiMocks.listBoardAnalysisResults.mockResolvedValue({
      success: true,
      data: [makeRow(42, 'board_brief')],
    })
    await c.loadBoardAnalysisResults(7701)

    apiMocks.triggerBoardInvestigation.mockResolvedValue({
      success: true,
      data: { status: 'started', job_id: 'inv3', job_kind: 'board_investigation', scope: 'board', target_id: 7701 },
    })
    apiMocks.getAnalysisStatusByJobId
      .mockResolvedValueOnce(runningSt('inv3', 'board_investigation'))
      .mockResolvedValueOnce(finishedSt('inv3', 'board_investigation', { result_id: 88 }))
    apiMocks.listBoardAnalysisResults.mockResolvedValue({
      success: true,
      data: [makeRow(88, 'board_investigation'), makeRow(42, 'board_brief')],
    })

    await c.triggerBoardInvestigation(7701, { briefing_result_id: 42, question: '自填的问题？' })
    await vi.advanceTimersByTimeAsync(0) // running
    await vi.advanceTimersByTimeAsync(3000) // finished

    expect(notifyMocks.success).toHaveBeenCalledWith('调查已完成')
    expect(c.selectedBoardResultId.value).toBe(88)
    expect(c.selectedBoardResult.value?.result_kind).toBe('board_investigation')
    expect(c.latestBoardBrief.value?.id).toBe(42) // 简报计算不把调查当 brief
    expect(c.boardAnalysisTriggering.value).toBe(false)
  })

  it('迟到守卫：A 触发调查 202 在途切 B → 迟到响应不跟 A 的 job、不污染 B 任务态', async () => {
    const c = setup()
    let resolveTrigger!: (v: unknown) => void
    apiMocks.triggerBoardInvestigation.mockReturnValueOnce(
      new Promise((r) => { resolveTrigger = r }),
    )

    const p = c.triggerBoardInvestigation(7701, { briefing_result_id: 42, question: '？' })
    // 在途切板块（bootstrap：activate → epoch++）
    c.activateBoardContext(8802)
    resolveTrigger({
      success: true,
      data: { status: 'started', job_id: 'invA', job_kind: 'board_investigation', scope: 'board', target_id: 7701 },
    })
    expect(await p).toBe(false)

    await vi.advanceTimersByTimeAsync(9000)
    expect(apiMocks.getAnalysisStatusByJobId).not.toHaveBeenCalled() // 不跟 A 的 job
    expect(c.activeBoardJob.value).toBeNull()
    expect(c.boardAnalysisTriggering.value).toBe(false)
    expect(notifyMocks.success).not.toHaveBeenCalled() // 迟到响应不 toast
  })
})

describe('board 轮询守卫（5.5 review：切板块隔离 / 串行无重叠 / 409 兑底）', () => {
  /** 手动可控的迟到响应。 */
  function deferred<T>() {
    let resolve!: (v: T) => void
    const promise = new Promise<T>((r) => {
      resolve = r
    })
    return { promise, resolve }
  }

  function scopeRunning(jobId: string, jobKind: string) {
    return {
      success: true,
      data: { scope: 'board', target_id: 7701, running: true, job_id: jobId, job_kind: jobKind },
    }
  }

  it('切板块：stopBoardPoll 停 timer 清任务态，A 迟到 finished 不改 B 列表/选中/triggering', async () => {
    const c = setup()
    // board A 有 running job j1，首 Poll 挂起（迟到响应可控）
    apiMocks.getAnalysisStatus.mockResolvedValue(scopeRunning('j1', 'board_brief'))
    const lateA = deferred<ReturnType<typeof finishedSt>>()
    apiMocks.getAnalysisStatusByJobId.mockReturnValueOnce(lateA.promise)
    await c.syncBoardAnalysisStatus(7701)
    await vi.advanceTimersByTimeAsync(0)
    expect(apiMocks.getAnalysisStatusByJobId).toHaveBeenCalledTimes(1) // 在途

    // 切板块 B（面板 bootstrap 第一步 stopBoardPoll）
    c.stopBoardPoll()
    expect(c.boardAnalysisTriggering.value).toBe(false)
    expect(c.activeBoardJob.value).toBeNull()

    // B 加载自己的历史列表并选中自己的简报
    apiMocks.listBoardAnalysisResults.mockResolvedValue({
      success: true,
      data: [makeRow(88, 'board_brief')],
    })
    await c.loadBoardAnalysisResults(8802)
    expect(c.selectedBoardResultId.value).toBe(88)

    // A 的迟到 finished 响应到达：不得通知、不得重拉/改选中
    lateA.resolve(finishedSt('j1', 'board_brief', { result_id: 55 }))
    await vi.advanceTimersByTimeAsync(0)
    expect(notifyMocks.success).not.toHaveBeenCalled()
    expect(apiMocks.listBoardAnalysisResults).toHaveBeenCalledTimes(1) // 未被 A 触发重拉
    expect(c.boardResults.value.map(r => r.id)).toEqual([88])
    expect(c.selectedBoardResultId.value).toBe(88)
    expect(c.boardAnalysisTriggering.value).toBe(false)
  })

  it('旧 job 迟到响应不得杀新 job：A 的 finished 迟到，B 的轮询照常推进到完成', async () => {
    const c = setup()
    // A 在跑 j1，首 poll 挂起
    apiMocks.getAnalysisStatus.mockResolvedValue(scopeRunning('j1', 'board_brief'))
    const lateA = deferred<ReturnType<typeof finishedSt>>()
    apiMocks.getAnalysisStatusByJobId.mockReturnValueOnce(lateA.promise)
    await c.syncBoardAnalysisStatus(7701)
    await vi.advanceTimersByTimeAsync(0)

    // 切板块 B 并触发新任务 j2
    c.stopBoardPoll()
    apiMocks.triggerBoardAnalysis.mockResolvedValue({
      success: true,
      data: { status: 'started', job_id: 'j2', job_kind: 'board_brief', scope: 'board', target_id: 8802 },
    })
    apiMocks.getAnalysisStatusByJobId.mockResolvedValue(runningSt('j2', 'board_brief'))
    await c.triggerBoardAnalysis(8802)
    await vi.advanceTimersByTimeAsync(0)
    expect(c.activeBoardJob.value).toEqual({ jobId: 'j2', jobKind: 'board_brief' })
    expect(c.boardAnalysisTriggering.value).toBe(true)

    // A 的迟到 finished 到达：不得 stop B、不得重拉（「简报已在后台生成」是 B 触发时的合法提示，不算）
    lateA.resolve(finishedSt('j1', 'board_brief', { result_id: 55 }))
    await vi.advanceTimersByTimeAsync(0)
    expect(notifyMocks.success).not.toHaveBeenCalledWith('简报已生成')
    expect(apiMocks.listBoardAnalysisResults).not.toHaveBeenCalled()
    expect(c.boardAnalysisTriggering.value).toBe(true) // B 未被杀
    expect(c.activeBoardJob.value?.jobId).toBe('j2')

    // B 的轮询继续推进并正常完成
    apiMocks.getAnalysisStatusByJobId.mockResolvedValue(finishedSt('j2', 'board_brief', { result_id: 66 }))
    apiMocks.listBoardAnalysisResults.mockResolvedValue({
      success: true,
      data: [makeRow(66, 'board_brief')],
    })
    await vi.advanceTimersByTimeAsync(3000)
    expect(notifyMocks.success).toHaveBeenCalledWith('简报已生成')
    expect(c.selectedBoardResultId.value).toBe(66)
    expect(c.boardAnalysisTriggering.value).toBe(false)
  })

  it('串行轮询：在途请求期间不排下一发（无 interval 重叠并发）', async () => {
    const c = setup()
    apiMocks.triggerBoardAnalysis.mockResolvedValue({
      success: true,
      data: { status: 'started', job_id: 'j1', job_kind: 'board_brief', scope: 'board', target_id: 7701 },
    })
    const slow = deferred<ReturnType<typeof runningSt>>()
    apiMocks.getAnalysisStatusByJobId
      .mockReturnValueOnce(slow.promise)
      .mockResolvedValue(runningSt('j1', 'board_brief'))

    await c.triggerBoardAnalysis(7701)
    await vi.advanceTimersByTimeAsync(0) // 首发在途
    expect(apiMocks.getAnalysisStatusByJobId).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(9000) // 在途期间无新请求（无并发重叠）
    expect(apiMocks.getAnalysisStatusByJobId).toHaveBeenCalledTimes(1)

    slow.resolve(runningSt('j1', 'board_brief'))
    await vi.advanceTimersByTimeAsync(3000) // 响应处理完才排下一发
    expect(apiMocks.getAnalysisStatusByJobId).toHaveBeenCalledTimes(2)
  })

  it('409 无 data 兑底：scope 状态 idle → 解除 triggering，按钮不卡', async () => {
    const c = setup()
    apiMocks.triggerBoardAnalysis.mockResolvedValue({
      success: false,
      status: 409,
      error: 'board analysis already running',
      data: undefined,
    })
    apiMocks.getAnalysisStatus.mockResolvedValue({
      success: true,
      data: { scope: 'board', target_id: 7701, running: false, finished: false },
    })

    const ok = await c.triggerBoardAnalysis(7701)

    expect(ok).toBe(true)
    expect(apiMocks.getAnalysisStatusByJobId).not.toHaveBeenCalled() // 未误入轮询
    expect(c.boardAnalysisTriggering.value).toBe(false)
    expect(c.activeBoardJob.value).toBeNull()
  })

  it('409 无 data 兑底：scope 发现最近任务已 finished → 不误当 running、不启动轮询', async () => {
    const c = setup()
    apiMocks.triggerBoardAnalysis.mockResolvedValue({
      success: false,
      status: 409,
      error: 'board analysis already running',
      data: undefined,
    })
    apiMocks.getAnalysisStatus.mockResolvedValue({
      success: true,
      data: { scope: 'board', target_id: 7701, running: false, finished: true, job_id: 'j-done', job_kind: 'board_brief' },
    })

    const ok = await c.triggerBoardAnalysis(7701)
    await vi.advanceTimersByTimeAsync(3000)

    expect(ok).toBe(true)
    expect(apiMocks.getAnalysisStatusByJobId).not.toHaveBeenCalled() // finished 不当 running
    expect(c.boardAnalysisTriggering.value).toBe(false)
  })
})

describe('board view context 守卫（5.4/5.5 review 修复：activate/deactivate + 迟到副作用丢弃）', () => {
  /** 手动可控的迟到响应。 */
  function deferred<T>() {
    let resolve!: (v: T) => void
    const promise = new Promise<T>((r) => {
      resolve = r
    })
    return { promise, resolve }
  }

  function startedRes(jobId: string) {
    return { success: true, data: { status: 'started', job_id: jobId, job_kind: 'board_brief', scope: 'board', target_id: 7701 } }
  }

  function scopeRunningRes(jobId: string) {
    return { success: true, data: { scope: 'board', target_id: 7701, running: true, job_id: jobId, job_kind: 'board_brief' } }
  }

  it('unmount 后 trigger 202 迟到：不重建 timer、不 notify、不写任务态', async () => {
    const c = setup()
    c.activateBoardContext(7701) // 面板 bootstrap 先激活
    const late = deferred<ReturnType<typeof startedRes>>()
    apiMocks.triggerBoardAnalysis.mockReturnValueOnce(late.promise)

    const p = c.triggerBoardAnalysis(7701)
    wrapper!.unmount()
    wrapper = null // 防 afterEach 重复 unmount

    late.resolve(startedRes('j1')) // 后端 job 已启动，响应迟到
    await p
    await vi.advanceTimersByTimeAsync(9000)

    expect(apiMocks.getAnalysisStatusByJobId).not.toHaveBeenCalled() // 不跟 A 的 job（无 timer 重建）
    expect(notifyMocks.success).not.toHaveBeenCalled() // 「简报已在后台生成」不弹
    expect(notifyMocks.warn).not.toHaveBeenCalled()
    expect(c.activeBoardJob.value).toBeNull()
    expect(c.boardAnalysisTriggering.value).toBe(false)
  })

  it('unmount 后 sync running 迟到：不启动轮询、不写任务态', async () => {
    const c = setup()
    c.activateBoardContext(7701)
    const late = deferred<ReturnType<typeof scopeRunningRes>>()
    apiMocks.getAnalysisStatus.mockReturnValueOnce(late.promise)

    const p = c.syncBoardAnalysisStatus(7701)
    wrapper!.unmount()
    wrapper = null

    late.resolve(scopeRunningRes('j5'))
    await p
    await vi.advanceTimersByTimeAsync(9000)

    expect(apiMocks.getAnalysisStatusByJobId).not.toHaveBeenCalled() // 迟到 running 不启动轮询
    expect(c.activeBoardJob.value).toBeNull()
    expect(c.boardAnalysisTriggering.value).toBe(false)
  })

  it('A trigger 202 迟到切 B：不跟 A 的 job、不污染 B 的任务态/列表/选中', async () => {
    const c = setup()
    c.activateBoardContext(7701)
    const late = deferred<ReturnType<typeof startedRes>>()
    apiMocks.triggerBoardAnalysis.mockReturnValueOnce(late.promise)
    const p = c.triggerBoardAnalysis(7701)

    // 切 B（面板 bootstrap：activate 停旧 poll + epoch++），B 加载自己的列表
    c.activateBoardContext(8802)
    apiMocks.listBoardAnalysisResults.mockResolvedValue({ success: true, data: [makeRow(88, 'board_brief')] })
    await c.loadBoardAnalysisResults(8802)
    expect(c.selectedBoardResultId.value).toBe(88)

    // A 的 202 迟到到达：后端 job 已启动，但前端不跟 A、不弹提示（回 A 由 sync 恢复）
    late.resolve(startedRes('jA'))
    await p
    await vi.advanceTimersByTimeAsync(9000)

    expect(apiMocks.getAnalysisStatusByJobId).not.toHaveBeenCalled()
    expect(notifyMocks.success).not.toHaveBeenCalled()
    expect(notifyMocks.warn).not.toHaveBeenCalled()
    expect(c.activeBoardJob.value).toBeNull()
    expect(c.boardAnalysisTriggering.value).toBe(false)
    expect(c.boardResults.value.map(r => r.id)).toEqual([88]) // B 列表不被 A 污染
    expect(c.selectedBoardResultId.value).toBe(88)
  })

  it('A sync running 迟到切 B：不跟 A 的 job、不写 B 的任务态', async () => {
    const c = setup()
    c.activateBoardContext(7701)
    const late = deferred<ReturnType<typeof scopeRunningRes>>()
    apiMocks.getAnalysisStatus.mockReturnValueOnce(late.promise)
    const p = c.syncBoardAnalysisStatus(7701)

    c.activateBoardContext(8802)

    late.resolve(scopeRunningRes('jA'))
    await p
    await vi.advanceTimersByTimeAsync(9000)

    expect(apiMocks.getAnalysisStatusByJobId).not.toHaveBeenCalled()
    expect(c.activeBoardJob.value).toBeNull()
    expect(c.boardAnalysisTriggering.value).toBe(false)
  })

  it('A finished reload 迟到切 B：loader 自身拒写，外层不 select/不新增 notify', async () => {
    const c = setup()
    c.activateBoardContext(7701)
    apiMocks.triggerBoardAnalysis.mockResolvedValue(startedRes('jA'))
    apiMocks.getAnalysisStatusByJobId
      .mockResolvedValueOnce(runningSt('jA', 'board_brief'))
      .mockResolvedValueOnce(finishedSt('jA', 'board_brief', { result_id: 55 }))
    const lateReload = deferred<{ success: true, data: ReturnType<typeof makeRow>[] }>()
    apiMocks.listBoardAnalysisResults.mockReturnValueOnce(lateReload.promise)

    await c.triggerBoardAnalysis(7701)
    await vi.advanceTimersByTimeAsync(0) // running
    await vi.advanceTimersByTimeAsync(3000) // finished：当时还在 A，notify 合法，reload 挂起
    expect(notifyMocks.success).toHaveBeenCalledWith('简报已生成')
    const notifyCount = notifyMocks.success.mock.calls.length

    // reload 在途切 B
    c.activateBoardContext(8802)
    apiMocks.listBoardAnalysisResults.mockResolvedValue({ success: true, data: [makeRow(88, 'board_brief')] })
    await c.loadBoardAnalysisResults(8802)
    expect(c.selectedBoardResultId.value).toBe(88)

    // A 的 reload 迟到到达（携带 A 的列表）：loader 拒写，外层不 select 55
    lateReload.resolve({ success: true, data: [makeRow(55, 'board_brief')] })
    await vi.advanceTimersByTimeAsync(0)

    expect(c.boardResults.value.map(r => r.id)).toEqual([88])
    expect(c.selectedBoardResultId.value).toBe(88)
    expect(notifyMocks.success.mock.calls.length).toBe(notifyCount) // 无新增 notify
    const calls = apiMocks.getAnalysisStatusByJobId.mock.calls.length
    await vi.advanceTimersByTimeAsync(9000)
    expect(apiMocks.getAnalysisStatusByJobId.mock.calls.length).toBe(calls) // 无 timer 重建
  })
})

describe('board 结果列表与选择（按 result_kind）', () => {
  it('默认选中最新 board_brief；legacy/investigation 留在历史里', async () => {
    const c = setup()
    apiMocks.listBoardAnalysisResults.mockResolvedValue({
      success: true,
      data: [makeRow(9, 'board_investigation'), makeRow(7, 'legacy_board_analysis'), makeRow(11, 'board_brief')],
    })

    await c.loadBoardAnalysisResults(7701)

    expect(c.selectedBoardResultId.value).toBe(11)
    expect(c.selectedBoardResult.value?.id).toBe(11)
  })

  it('只有 legacy 行：默认不选中任何简报，选中 legacy 可渲染旧视图（不崩）', async () => {
    const c = setup()
    apiMocks.listBoardAnalysisResults.mockResolvedValue({
      success: true,
      data: [makeRow(7, 'legacy_board_analysis')],
    })

    await c.loadBoardAnalysisResults(7701)

    expect(c.selectedBoardResultId.value).toBeNull()
    // 手动选中 legacy 历史行
    c.selectBoardResult(7)
    expect(c.selectedBoardResult.value?.id).toBe(7)
  })

  it('旧数据无 result_kind：默认兜底首行，不崩', async () => {
    const c = setup()
    apiMocks.listBoardAnalysisResults.mockResolvedValue({
      success: true,
      data: [makeRow(3)],
    })

    await c.loadBoardAnalysisResults(7701)

    expect(c.selectedBoardResultId.value).toBeNull()
    expect(c.selectedBoardResult.value?.id).toBe(3)
  })
})

describe('topic/debate 轮询视图守卫（5.5 review 剩余 Important：切板块/换 topic 隔离）', () => {
  /** 手动可控的迟到响应。 */
  function deferred<T>() {
    let resolve!: (v: T) => void
    const promise = new Promise<T>((r) => {
      resolve = r
    })
    return { promise, resolve }
  }

  function topicRunning(topicId: number) {
    return { success: true, data: { scope: 'topic', target_id: topicId, running: true, job_id: 'tj1', job_kind: 'topic_analysis' } }
  }
  function topicFinished(topicId: number, extra: Record<string, unknown> = {}) {
    return { success: true, data: { scope: 'topic', target_id: topicId, running: false, finished: true, ...extra } }
  }
  function startedTopicRes(topicId: number) {
    return { success: true, data: { status: 'started' as const, scope: 'topic', target_id: topicId } }
  }
  function resultRow(id: number): ResultSummaryRow {
    return { id, evolution_assessment: 'assess', sectors: null, tool_calls_count: 0, session_id: 's', created_at: '2026-09-01T10:00:00Z' }
  }
  function detailRow(id: number): ResultDetailRow {
    return { id, evolution_assessment: 'assess', sectors: null, causal_chain: null, tool_calls: [], input_snapshot: {}, session_id: 's', created_at: '2026-09-01T10:00:00Z' }
  }
  function reviewRow(id: number): ReviewRow {
    return { id, prev_result_id: null, curr_result_id: 55, deviation_summary: 'd', affected_context: null, confidence: null, applied: false, source: 'manual', created_at: '2026-09-01T10:00:00Z' }
  }
  function debateRow(id: number, status: string): StockDebateResult {
    return { id, topic_enrichment_result_id: 55, sector: 'energy', code: 'XOM', distill_status: status }
  }
  function startedBoardRes(jobId: string) {
    return { success: true, data: { status: 'started' as const, job_id: jobId, job_kind: 'board_brief', scope: 'board', target_id: 7701 } }
  }

  /** 激活板块 A 并选中 topic 101（直写 ref，不触发任何加载）。 */
  function inBoardATopic101(c: En) {
    c.activateBoardContext(7701)
    c.selectedTopicId.value = 101
  }
  /** 模拟切到板块 B：bootstrap 的 activate（停三类轮询 + epoch++）+ loadTopics 换 topic。 */
  function switchToBoardBTopic202(c: En) {
    c.activateBoardContext(8802)
    c.selectedTopicId.value = 202
  }
  const topicStatusCalls = () => apiMocks.getAnalysisStatus.mock.calls.filter(a => a[0] === 'topic')

  it('A topic status 在途切 B 后 finished：不 toast「增强完成」、不重拉、不再发轮询', async () => {
    const c = setup()
    inBoardATopic101(c)
    apiMocks.triggerEnrichment.mockResolvedValue(startedTopicRes(101))
    await c.triggerEnrichment(101)
    expect(c.triggering.value).toBe(true)

    const late = deferred<ReturnType<typeof topicFinished>>()
    apiMocks.getAnalysisStatus.mockReturnValueOnce(late.promise)
    await vi.advanceTimersByTimeAsync(3000) // 首轮 topic 轮询发出，挂起
    expect(apiMocks.getAnalysisStatus).toHaveBeenCalledWith('topic', 101)

    switchToBoardBTopic202(c)

    late.resolve(topicFinished(101)) // A 的 finished 迟到到达（后端 job 照跑，前端丢弃）
    await vi.advanceTimersByTimeAsync(0)

    expect(notifyMocks.success).not.toHaveBeenCalledWith('增强完成')
    expect(notifyMocks.error).not.toHaveBeenCalled()
    expect(apiMocks.listResults).not.toHaveBeenCalled() // 终态 loader 未被触发
    expect(apiMocks.listReviews).not.toHaveBeenCalled()
    expect(apiMocks.getResult).not.toHaveBeenCalled()
    expect(c.triggering.value).toBe(false)
    const calls = topicStatusCalls().length
    await vi.advanceTimersByTimeAsync(9000)
    expect(topicStatusCalls().length).toBe(calls) // 旧 timer 已清，不再发 topic 101 状态请求
  })

  it('A topic error 终态在途切 B：不 toast 失败、不重拉', async () => {
    const c = setup()
    inBoardATopic101(c)
    apiMocks.triggerEnrichment.mockResolvedValue(startedTopicRes(101))
    await c.triggerEnrichment(101)

    const late = deferred<ReturnType<typeof topicFinished>>()
    apiMocks.getAnalysisStatus.mockReturnValueOnce(late.promise)
    await vi.advanceTimersByTimeAsync(3000)

    switchToBoardBTopic202(c)

    late.resolve(topicFinished(101, { error: 'boom' }))
    await vi.advanceTimersByTimeAsync(0)

    expect(notifyMocks.error).not.toHaveBeenCalled()
    expect(apiMocks.listResults).not.toHaveBeenCalled()
    expect(c.triggering.value).toBe(false)
  })

  it('A topic finished 后三个 loader 在途切 B：activate 已同步清场，迟到响应也不回填', async () => {
    const c = setup()
    inBoardATopic101(c)
    // A 已有结果与详情（哨兵）：切 B 时由 activate 同步清场，迟到响应不得回填
    c.results.value = [resultRow(55)]
    c.latestResultDetail.value = detailRow(55)

    apiMocks.triggerEnrichment.mockResolvedValue(startedTopicRes(101))
    await c.triggerEnrichment(101)

    const lateResults = deferred<{ success: true, data: ResultSummaryRow[] }>()
    const lateReviews = deferred<{ success: true, data: ReviewRow[] }>()
    const lateDetail = deferred<{ success: true, data: ResultDetailRow }>()
    apiMocks.listResults.mockReturnValueOnce(lateResults.promise)
    apiMocks.listReviews.mockReturnValueOnce(lateReviews.promise)
    apiMocks.getResult.mockReturnValueOnce(lateDetail.promise)
    apiMocks.getAnalysisStatus.mockResolvedValueOnce(topicFinished(101))

    await vi.advanceTimersByTimeAsync(3000) // finished：当时还在 A，notify 合法
    expect(notifyMocks.success).toHaveBeenCalledWith('增强完成')

    switchToBoardBTopic202(c) // 切 B：activate 同步清空旧 topic 展示 refs（终审 Important）
    expect(c.results.value).toEqual([]) // 清场即时生效，不等加载
    expect(c.latestResultDetail.value).toBeNull()

    lateResults.resolve({ success: true, data: [resultRow(900)] })
    await vi.advanceTimersByTimeAsync(0) // loadResults 拒写 → loadReviews 发出
    lateReviews.resolve({ success: true, data: [reviewRow(901)] })
    await vi.advanceTimersByTimeAsync(0) // loadReviews 拒写 → loadLatestResultDetail 发出
    lateDetail.resolve({ success: true, data: detailRow(900) })
    await vi.advanceTimersByTimeAsync(0)

    // [] / null 即「清场后未被 A 的迟到响应回填」（回填会出现 900/901）
    expect(c.results.value).toEqual([])
    expect(c.reviews.value).toEqual([])
    expect(c.latestResultDetail.value).toBeNull()
  })

  it('A debate 加载在途切 B：不写 debates、不启轮询、不残留 loading', async () => {
    const c = setup()
    inBoardATopic101(c)
    const late = deferred<{ success: true, data: StockDebateResult[] }>()
    apiMocks.listDebates.mockReturnValueOnce(late.promise)
    const p = c.loadDebates(55)

    switchToBoardBTopic202(c)

    late.resolve({ success: true, data: [debateRow(7, 'running')] }) // running 本应启轮询
    await p

    expect(c.debates.value).toEqual([]) // 未写
    expect(c.debatesLoading.value).toBe(false) // activate 停轮询时已清，不残留卡死
    const calls = apiMocks.listDebates.mock.calls.length
    await vi.advanceTimersByTimeAsync(15000)
    expect(apiMocks.listDebates.mock.calls.length).toBe(calls) // 未启轮询
  })

  it('A debate 轮询在途换 topic：迟到响应不写 debates、不误停 B 的新轮询', async () => {
    const c = setup()
    inBoardATopic101(c)
    c.results.value = [resultRow(55)]
    // A 初始加载：running → 启动 A 的 debate 轮询（捕获 101+55+epoch）
    apiMocks.listDebates.mockResolvedValueOnce({ success: true, data: [debateRow(7, 'running')] })
    await c.loadDebates(55)
    expect(c.debates.value.map(d => d.id)).toEqual([7])

    const lateA = deferred<{ success: true, data: StockDebateResult[] }>()
    apiMocks.listDebates.mockReturnValueOnce(lateA.promise)
    await vi.advanceTimersByTimeAsync(5000) // A 轮询首发出，挂起

    // 换 topic：selectTopic 停 A 的 topic/debate 轮询，装载 B（result 66，debate running → 新轮询）
    apiMocks.listResults.mockResolvedValue({ success: true, data: [resultRow(66)] })
    apiMocks.getResult.mockResolvedValue({ success: true, data: detailRow(66) })
    apiMocks.listDebates.mockResolvedValueOnce({ success: true, data: [debateRow(9, 'running')] })
    await c.selectTopic(202)

    // A 的迟到响应到达（全部 done）——不得写、不得 stopDebatePolling（B 的 timer 不能被误停）
    lateA.resolve({ success: true, data: [debateRow(7, 'done')] })
    await vi.advanceTimersByTimeAsync(0)
    expect(c.debates.value.map(d => d.id)).toEqual([9]) // 仍是 B 的数据
    expect(c.debates.value.every(d => d.distill_status === 'running')).toBe(true)

    // B 的轮询继续：下一轮 5s 后仍请求 (202, 66)
    await vi.advanceTimersByTimeAsync(5000)
    expect(apiMocks.listDebates).toHaveBeenCalledWith(202, 66)
  })

  it('A topic 轮询持续 running 中换 topic：旧轮询停、迟到/后续响应全丢', async () => {
    const c = setup()
    inBoardATopic101(c)
    apiMocks.triggerEnrichment.mockResolvedValue(startedTopicRes(101))
    await c.triggerEnrichment(101)
    apiMocks.getAnalysisStatus.mockResolvedValue(topicRunning(101))
    await vi.advanceTimersByTimeAsync(3000)
    expect(topicStatusCalls().length).toBeGreaterThan(0)

    // selectTopic(202)：停旧 topic poll；B 无 result → 不发 debate 请求
    await c.selectTopic(202)

    const calls = topicStatusCalls().length
    await vi.advanceTimersByTimeAsync(9000)
    expect(topicStatusCalls().length).toBe(calls) // 旧 topic 轮询已停
    expect(c.triggering.value).toBe(false)
  })

  it('activateBoardContext：board/topic/debate 三类 timer 全清、任务态归零', async () => {
    const c = setup()
    inBoardATopic101(c)
    c.results.value = [resultRow(55)]
    // 三类轮询全部在跑
    apiMocks.triggerBoardAnalysis.mockResolvedValue(startedBoardRes('j1'))
    await c.triggerBoardAnalysis(7701)
    apiMocks.getAnalysisStatusByJobId.mockResolvedValue(runningSt('j1', 'board_brief'))
    apiMocks.triggerEnrichment.mockResolvedValue(startedTopicRes(101))
    await c.triggerEnrichment(101)
    apiMocks.listDebates.mockResolvedValueOnce({ success: true, data: [debateRow(7, 'running')] })
    await c.loadDebates(55)
    await vi.advanceTimersByTimeAsync(3000) // board/topic 各发一轮
    expect(apiMocks.getAnalysisStatusByJobId).toHaveBeenCalled()
    expect(apiMocks.getAnalysisStatus).toHaveBeenCalledWith('topic', 101)
    expect(apiMocks.listDebates).toHaveBeenCalledWith(101, 55)

    // 切 B：activate 一并清三类 timer
    c.activateBoardContext(8802)

    const boardCalls = apiMocks.getAnalysisStatusByJobId.mock.calls.length
    const topicCalls = topicStatusCalls().length
    const debateCalls = apiMocks.listDebates.mock.calls.length
    await vi.advanceTimersByTimeAsync(12000)
    expect(apiMocks.getAnalysisStatusByJobId.mock.calls.length).toBe(boardCalls)
    expect(topicStatusCalls().length).toBe(topicCalls)
    expect(apiMocks.listDebates.mock.calls.length).toBe(debateCalls)
    expect(c.triggering.value).toBe(false)
    expect(c.boardAnalysisTriggering.value).toBe(false)
    expect(c.activeBoardJob.value).toBeNull()
  })

  it('deactivateBoardContext：三类 timer 全清（unmount 同语义）', async () => {
    const c = setup()
    inBoardATopic101(c)
    c.results.value = [resultRow(55)]
    apiMocks.triggerBoardAnalysis.mockResolvedValue(startedBoardRes('j1'))
    await c.triggerBoardAnalysis(7701)
    apiMocks.getAnalysisStatusByJobId.mockResolvedValue(runningSt('j1', 'board_brief'))
    apiMocks.triggerEnrichment.mockResolvedValue(startedTopicRes(101))
    await c.triggerEnrichment(101)
    apiMocks.listDebates.mockResolvedValueOnce({ success: true, data: [debateRow(7, 'running')] })
    await c.loadDebates(55)
    await vi.advanceTimersByTimeAsync(3000)

    c.deactivateBoardContext()

    const boardCalls = apiMocks.getAnalysisStatusByJobId.mock.calls.length
    const topicCalls = topicStatusCalls().length
    const debateCalls = apiMocks.listDebates.mock.calls.length
    await vi.advanceTimersByTimeAsync(12000)
    expect(apiMocks.getAnalysisStatusByJobId.mock.calls.length).toBe(boardCalls)
    expect(topicStatusCalls().length).toBe(topicCalls)
    expect(apiMocks.listDebates.mock.calls.length).toBe(debateCalls)
    expect(c.triggering.value).toBe(false)
    expect(c.boardAnalysisTriggering.value).toBe(false)
    expect(c.activeBoardJob.value).toBeNull()
  })

  it('selectTopic：停旧 topic/debate 轮询并失效化在途，board 轮询照常', async () => {
    const c = setup()
    inBoardATopic101(c)
    c.results.value = [resultRow(55)]
    apiMocks.triggerBoardAnalysis.mockResolvedValue(startedBoardRes('j1'))
    await c.triggerBoardAnalysis(7701)
    apiMocks.getAnalysisStatusByJobId.mockResolvedValue(runningSt('j1', 'board_brief'))
    apiMocks.triggerEnrichment.mockResolvedValue(startedTopicRes(101))
    await c.triggerEnrichment(101)
    apiMocks.listDebates.mockResolvedValueOnce({ success: true, data: [debateRow(7, 'running')] })
    await c.loadDebates(55)
    await vi.advanceTimersByTimeAsync(3000) // board/topic 各发一轮

    await c.selectTopic(202) // B 无 result：不发 debate 请求

    const boardCalls = apiMocks.getAnalysisStatusByJobId.mock.calls.length
    const topicCalls = topicStatusCalls().length
    const debateCalls = apiMocks.listDebates.mock.calls.length
    await vi.advanceTimersByTimeAsync(9000)

    // board 轮询继续（板块任务与 topic 无关）
    expect(apiMocks.getAnalysisStatusByJobId.mock.calls.length).toBeGreaterThan(boardCalls)
    expect(c.boardAnalysisTriggering.value).toBe(true)
    expect(c.activeBoardJob.value).toEqual({ jobId: 'j1', jobKind: 'board_brief' })
    // topic 轮询已停：topic scope 状态请求不再增长
    expect(topicStatusCalls().length).toBe(topicCalls)
    // 旧 debate 轮询已停
    expect(apiMocks.listDebates.mock.calls.length).toBe(debateCalls)
    expect(c.triggering.value).toBe(false)
  })

  // ── topic 档重进恢复（5.5 最终 review：syncTopicAnalysisStatus）─────────
  // 面板 bootstrap 在 loadTopics 确定 selectedTopicId 后调用 sync：同 board
  // 刷新停 poll 后恢复、切走再返回恢复、首次进入 running topic 恢复；旧 board
  // 的迟到 status 不得写入新 board。

  it('sync running：恢复 triggering 并启动 topic 轮询（同 board 刷新后 running topic 恢复）', async () => {
    const c = setup()
    inBoardATopic101(c)
    expect(c.triggering.value).toBe(false)

    apiMocks.getAnalysisStatus.mockResolvedValueOnce(topicRunning(101))
    await c.syncTopicAnalysisStatus(101)

    expect(c.triggering.value).toBe(true)
    await vi.advanceTimersByTimeAsync(3000)
    expect(apiMocks.getAnalysisStatus).toHaveBeenCalledWith('topic', 101)

    // 恢复的轮询终态语义照旧：finished → 提示 + 重拉三表
    apiMocks.getAnalysisStatus.mockResolvedValueOnce(topicFinished(101, { result_id: 55 }))
    await vi.advanceTimersByTimeAsync(3000)
    expect(notifyMocks.success).toHaveBeenCalledWith('增强完成')
    expect(apiMocks.listResults).toHaveBeenCalledWith(101)
    expect(apiMocks.listReviews).toHaveBeenCalledWith(101)
    expect(c.triggering.value).toBe(false)
  })

  // ── bootstrap 顺序契约（Critical 修复：panel bootstrap 为 loadAll 先、sync 最后）──
  // 用真实 loadAllTopicTables + syncTopicAnalysisStatus 组合验证：正确顺序下
  // sync 恢复的 running 轮询跨出 bootstrap 存活（timer 持续发出、triggering 保持），
  // 终态重拉照旧；反序（旧 bug 形态）则 loadAll 入口 stopTopicPoll 误杀恢复轮询。

  it('bootstrap 顺序契约：loadAll 先 + sync 最后——恢复的 running 轮询跨出 bootstrap 存活（timer/triggering 保持，终态重拉照旧）', async () => {
    const c = setup()
    inBoardATopic101(c)

    // 复刻新 bootstrap 顺序：loadTopics 定 topic → loadAllTopicTables → sync 收尾
    apiMocks.getAnalysisStatus.mockResolvedValue(topicRunning(101))
    await c.loadAllTopicTables(101) // 入口同步 stopTopicPoll：此时无恢复轮询可杀
    await c.syncTopicAnalysisStatus(101) // 最后恢复：running → startTopicPoll

    // bootstrap 结束后 poll 仍 active：triggering 保持 + 每个周期深 topic status
    expect(c.triggering.value).toBe(true)
    const base = topicStatusCalls().length
    await vi.advanceTimersByTimeAsync(3000)
    expect(topicStatusCalls().length).toBe(base + 1)
    await vi.advanceTimersByTimeAsync(3000)
    expect(topicStatusCalls().length).toBe(base + 2)
    expect(apiMocks.getAnalysisStatus).toHaveBeenLastCalledWith('topic', 101)

    // 终态语义照旧：finished → 提示 + 重拉三表 + 解除 triggering
    apiMocks.getAnalysisStatus.mockResolvedValueOnce(topicFinished(101, { result_id: 55 }))
    await vi.advanceTimersByTimeAsync(3000)
    expect(notifyMocks.success).toHaveBeenCalledWith('增强完成')
    expect(apiMocks.listResults).toHaveBeenCalledWith(101)
    expect(apiMocks.listReviews).toHaveBeenCalledWith(101)
    expect(c.triggering.value).toBe(false)
  })

  it('反证旧顺序（bug 形态）：sync 先 + loadAll 后——loadAll 入口 stopTopicPoll 误杀恢复轮询（顺序契约的根因，永不得回退）', async () => {
    const c = setup()
    inBoardATopic101(c)

    apiMocks.getAnalysisStatus.mockResolvedValueOnce(topicRunning(101))
    await c.syncTopicAnalysisStatus(101) // 旧顺序：先恢复 running 轮询
    expect(c.triggering.value).toBe(true)

    await c.loadAllTopicTables(101) // 随后 loadAll：入口同步 stopTopicPoll 杀掉刚恢复的轮询

    expect(c.triggering.value).toBe(false) // 轮询被杀：任务态归零
    const calls = topicStatusCalls().length
    await vi.advanceTimersByTimeAsync(9000)
    expect(topicStatusCalls().length).toBe(calls) // timer 已清：不再发出 topic status
    expect(notifyMocks.success).not.toHaveBeenCalledWith('增强完成') // 后台任务在前端失联
  })

  it('切 A→B→A：A 的 running topic 重进后恢复轮询', async () => {
    const c = setup()
    inBoardATopic101(c)
    apiMocks.triggerEnrichment.mockResolvedValue(startedTopicRes(101))
    await c.triggerEnrichment(101)
    apiMocks.getAnalysisStatus.mockResolvedValue(topicRunning(101))
    await vi.advanceTimersByTimeAsync(3000)
    expect(topicStatusCalls().length).toBeGreaterThan(0)

    // 切 B（bootstrap 语义：activate 停三类轮询 + 换 selectedTopicId）
    switchToBoardBTopic202(c)
    const callsAfterB = topicStatusCalls().length
    await vi.advanceTimersByTimeAsync(9000)
    expect(topicStatusCalls().length).toBe(callsAfterB) // 已停

    // 回 A（bootstrap：activate(A) + loadTopics 恢复 topic 101 + sync）
    c.activateBoardContext(7701)
    c.selectedTopicId.value = 101
    await c.syncTopicAnalysisStatus(101)
    expect(c.triggering.value).toBe(true)
    await vi.advanceTimersByTimeAsync(3000)
    expect(apiMocks.getAnalysisStatus).toHaveBeenLastCalledWith('topic', 101)
    expect(topicStatusCalls().length).toBeGreaterThan(callsAfterB)
  })

  it('sync A 在途切 B：迟到 running 不启动轮询、不写 B 的任务态', async () => {
    const c = setup()
    inBoardATopic101(c)
    const late = deferred<ReturnType<typeof topicRunning>>()
    apiMocks.getAnalysisStatus.mockReturnValueOnce(late.promise)
    const p = c.syncTopicAnalysisStatus(101)

    switchToBoardBTopic202(c)

    late.resolve(topicRunning(101)) // A 的 running 迟到到达：旧 board 的 status 不写入新 board
    await p
    await vi.advanceTimersByTimeAsync(0)

    expect(c.triggering.value).toBe(false)
    const calls = topicStatusCalls().length
    await vi.advanceTimersByTimeAsync(9000)
    expect(topicStatusCalls().length).toBe(calls) // 未启动任何 topic 轮询
  })

  it('sync finished：不启动轮询并解除 triggering（最近任务不误当 running）', async () => {
    const c = setup()
    inBoardATopic101(c)
    c.triggering.value = true // 模拟 409 兜底/旧轮询残留的任务态

    apiMocks.getAnalysisStatus.mockResolvedValueOnce(topicFinished(101))
    await c.syncTopicAnalysisStatus(101)

    expect(c.triggering.value).toBe(false)
    const calls = topicStatusCalls().length
    await vi.advanceTimersByTimeAsync(9000)
    expect(topicStatusCalls().length).toBe(calls)
    expect(notifyMocks.success).not.toHaveBeenCalledWith('增强完成') // 终态重拉不发生
  })

  it('sync idle（无任务骨架）：解除 triggering 不启动轮询', async () => {
    const c = setup()
    inBoardATopic101(c)
    c.triggering.value = true

    // beforeEach 默认 getAnalysisStatus → { success: true, data: undefined }（idle 骨架）
    await c.syncTopicAnalysisStatus(101)

    expect(c.triggering.value).toBe(false)
    const calls = topicStatusCalls().length
    await vi.advanceTimersByTimeAsync(9000)
    expect(topicStatusCalls().length).toBe(calls)
  })

  it('sync 与 trigger 并发（有在跟轮询）：idle/finished 不清 triggering、不误停新轮询', async () => {
    const c = setup()
    inBoardATopic101(c)
    apiMocks.triggerEnrichment.mockResolvedValue(startedTopicRes(101))
    await c.triggerEnrichment(101) // 启动在跟轮询，triggering=true

    apiMocks.getAnalysisStatus.mockResolvedValueOnce(topicFinished(101)) // sync 看到 finished
    await c.syncTopicAnalysisStatus(101)

    expect(c.triggering.value).toBe(true) // topicPollCtx 非空：不清（不误停刚启动的 poll）
    await vi.advanceTimersByTimeAsync(3000)
    expect(topicStatusCalls().length).toBeGreaterThan(0) // 轮询照常发出
  })

  it('unmount 后迟到 running：不重建 timer、不写任务态', async () => {
    const c = setup()
    inBoardATopic101(c)
    const late = deferred<ReturnType<typeof topicRunning>>()
    apiMocks.getAnalysisStatus.mockReturnValueOnce(late.promise)
    const p = c.syncTopicAnalysisStatus(101)

    wrapper!.unmount()

    late.resolve(topicRunning(101))
    await p
    await vi.advanceTimersByTimeAsync(0)

    expect(c.triggering.value).toBe(false)
    const calls = topicStatusCalls().length
    await vi.advanceTimersByTimeAsync(9000)
    expect(topicStatusCalls().length).toBe(calls) // 无 timer 重建
  })

  it('trigger 409（携 job 身份冲突体）：按 scope 入口恢复轮询，终态文案/重拉保持', async () => {
    const c = setup()
    inBoardATopic101(c)
    apiMocks.triggerEnrichment.mockResolvedValueOnce({
      success: false,
      status: 409,
      error: 'topic analysis already running: job tj9 (topic_analysis)',
      data: { job_id: 'tj9', job_kind: 'topic_analysis', scope: 'topic', target_id: 101, running: true },
    })

    const ok = await c.triggerEnrichment(101)

    expect(ok).toBe(true)
    expect(c.triggering.value).toBe(true) // 已恢复轮询显示
    apiMocks.getAnalysisStatus.mockResolvedValueOnce(topicFinished(101, { result_id: 55 }))
    await vi.advanceTimersByTimeAsync(3000)
    expect(notifyMocks.success).toHaveBeenCalledWith('增强完成') // 状态文案保持
    expect(apiMocks.listResults).toHaveBeenCalledWith(101) // 终态重拉保持
  })
})

describe('loadTopics 失败语义与切板块清场（5.4/5.5 终审 Important）', () => {
  /** 手动可控的迟到响应。 */
  function deferred<T>() {
    let resolve!: (v: T) => void
    const promise = new Promise<T>((r) => {
      resolve = r
    })
    return { promise, resolve }
  }

  function startedTopicRes(topicId: number) {
    return { success: true, data: { status: 'started' as const, scope: 'topic', target_id: topicId } }
  }
  function topicListItem(id: number, status = 'active'): BoardTopicListItem {
    return {
      id,
      semantic_board_id: 7701,
      label: `话题${id}`,
      description: '',
      status,
      first_seen_date: '2026-09-01',
      last_seen_date: '2026-09-01',
      hit_count: 1,
      consecutive_hits: 1,
      section_count: 1,
      color: '#3b82f6',
      can_activate: true,
    }
  }
  function contextRow(id: number): ContextRow {
    return { id, persistent_topic_id: 101, granularity: 'week', period: '2026-W36', content: '哨兵内容', as_of_date: '2026-09-01', source: 'llm_assisted', created_at: '2026-09-01T10:00:00Z', updated_at: '2026-09-01T10:00:00Z' }
  }
  function resultRow(id: number): ResultSummaryRow {
    return { id, evolution_assessment: 'assess', sectors: null, tool_calls_count: 0, session_id: 's', created_at: '2026-09-01T10:00:00Z' }
  }
  function detailRow(id: number): ResultDetailRow {
    return { id, evolution_assessment: 'assess', sectors: null, causal_chain: null, tool_calls: [], input_snapshot: {}, session_id: 's', created_at: '2026-09-01T10:00:00Z' }
  }
  function reviewRow(id: number): ReviewRow {
    return { id, prev_result_id: null, curr_result_id: 55, deviation_summary: 'd', affected_context: null, confidence: null, applied: false, source: 'manual', created_at: '2026-09-01T10:00:00Z' }
  }
  function debateRow(id: number, status: string): StockDebateResult {
    return { id, topic_enrichment_result_id: 55, sector: 'energy', code: 'XOM', distill_status: status }
  }
  function qaRow(id: number): TopicEnrichmentQA {
    return { id, topic_enrichment_result_id: 55, question: 'q', answer: 'a', tool_calls: [], source: 'qa', sedimented: false, created_at: '2026-09-01T10:00:00Z' }
  }
  const topicStatusCalls = () => apiMocks.getAnalysisStatus.mock.calls.filter(a => a[0] === 'topic')

  /** 激活板块 A 并选中 topic 101，填满 topic 级展示哨兵（表1/2/3、详情、QA、辩论、error）。 */
  function inBoardAFilled(c: En) {
    c.activateBoardContext(7701)
    c.selectedTopicId.value = 101
    c.contexts.value = [contextRow(1)]
    c.results.value = [resultRow(55)]
    c.reviews.value = [reviewRow(66)]
    c.latestResultDetail.value = detailRow(55)
    c.qaList.value = [qaRow(7)]
    c.latestAnswer.value = { answer: '旧答案', tool_calls: [], refs: [] }
    c.qaError.value = '旧追问错误'
    c.debates.value = [debateRow(8, 'done')]
    c.debateError.value = '旧辩论错误'
  }

  it('A→B 切板块：activate 同步置空 selectedTopicId 并清空旧 topic 展示 refs；B topics 失败后不回填、无任何 topic 级请求/轮询', async () => {
    const c = setup()
    inBoardAFilled(c)

    // 切 B：activate 同步清场（不等 loadTopics 返回，加载期间无混合视图）
    c.activateBoardContext(8802)
    expect(c.selectedTopicId.value).toBeNull()
    expect(c.contexts.value).toEqual([])
    expect(c.results.value).toEqual([])
    expect(c.reviews.value).toEqual([])
    expect(c.latestResultDetail.value).toBeNull()
    expect(c.qaList.value).toEqual([])
    expect(c.latestAnswer.value).toBeNull()
    expect(c.debates.value).toEqual([])
    expect(c.qaError.value).toBeNull()
    expect(c.debateError.value).toBeNull()
    // loading/触发态一并归零，不残留卡死
    expect(c.contextsLoading.value).toBe(false)
    expect(c.resultsLoading.value).toBe(false)
    expect(c.reviewsLoading.value).toBe(false)
    expect(c.latestResultDetailLoading.value).toBe(false)
    expect(c.qaLoading.value).toBe(false)
    expect(c.debatesLoading.value).toBe(false)
    expect(c.triggering.value).toBe(false)
    expect(c.debateTriggering.value).toBe(false)

    // B 的 topics 失败：失败语义保持清场 + topics=[]/selected=null/error 置位
    reportsMocks.listBoardTopics.mockResolvedValueOnce({ success: false, error: 'B 板块话题接口炸了' })
    await c.loadTopics(8802)
    expect(c.topics.value).toEqual([])
    expect(c.selectedTopicId.value).toBeNull()
    expect(c.error.value).toBe('B 板块话题接口炸了')
    expect(c.topicsLoading.value).toBe(false)
    expect(c.contexts.value).toEqual([]) // 旧 topic 展示不回填
    expect(c.latestResultDetail.value).toBeNull()

    // 无 loadAll 等价（无 listContexts/listResults/listReviews/getResult/listDebates 请求）
    expect(apiMocks.listContexts).not.toHaveBeenCalled()
    expect(apiMocks.listResults).not.toHaveBeenCalled()
    expect(apiMocks.listReviews).not.toHaveBeenCalled()
    expect(apiMocks.getResult).not.toHaveBeenCalled()
    expect(apiMocks.listDebates).not.toHaveBeenCalled()
    // 无 sync/topic 轮询（无 topic scope 状态请求，含时间推进后）
    expect(topicStatusCalls().length).toBe(0)
    await vi.advanceTimersByTimeAsync(9000)
    expect(topicStatusCalls().length).toBe(0)
  })

  it('同 board 手动刷新 topics 失败：selected 置空 + 清 topic 级 refs + 停旧 topic 轮询（不得继续旧 topic）', async () => {
    const c = setup()
    inBoardAFilled(c)
    // 旧 topic 轮询在跑（同 board 刷新场景：刷新入口 bootstrap → loadTopics）
    apiMocks.triggerEnrichment.mockResolvedValue(startedTopicRes(101))
    await c.triggerEnrichment(101)
    expect(c.triggering.value).toBe(true)

    reportsMocks.listBoardTopics.mockResolvedValueOnce({ success: false, error: '网络炸了' })
    await c.loadTopics(7701) // 同 board（activate 同 id 不清场，由失败分支清）

    expect(c.topics.value).toEqual([])
    expect(c.selectedTopicId.value).toBeNull()
    expect(c.error.value).toBe('网络炸了')
    expect(c.contexts.value).toEqual([])
    expect(c.results.value).toEqual([])
    expect(c.reviews.value).toEqual([])
    expect(c.latestResultDetail.value).toBeNull()
    expect(c.qaList.value).toEqual([])
    expect(c.debates.value).toEqual([])
    expect(c.triggering.value).toBe(false) // 旧 topic 轮询已停

    // 失败后不再有任何 topic 级请求/轮询
    expect(apiMocks.listContexts).not.toHaveBeenCalled()
    expect(apiMocks.listResults).not.toHaveBeenCalled()
    const calls = topicStatusCalls().length
    await vi.advanceTimersByTimeAsync(9000)
    expect(topicStatusCalls().length).toBe(calls)
  })

  it('同 board 手动刷新 topics 成功：合法选择保留、topic 级 refs 不被清（由后续 loader 刷新）', async () => {
    const c = setup()
    inBoardAFilled(c)
    const sentinelCtx = c.contexts.value[0]!

    reportsMocks.listBoardTopics.mockResolvedValueOnce({ success: true, data: { topics: [topicListItem(101), topicListItem(102)] } })
    await c.loadTopics(7701) // 同 board（boardId 不变）：保留当前 topic 选择

    expect(c.selectedTopicId.value).toBe(101) // 仍合法：保留（不清、不重选）
    expect(c.topics.value.map(t => t.id)).toEqual([101, 102])
    expect(c.contexts.value).toEqual([sentinelCtx]) // 成功路径不清展示 refs
    expect(c.results.value.map(r => r.id)).toEqual([55])
    expect(c.latestResultDetail.value?.id).toBe(55)
    expect(c.debates.value.map(d => d.id)).toEqual([8])
    expect(c.error.value).toBeNull()
  })

  it('A 的 topics 失败响应迟到切 B 后到达：epoch 守卫丢弃，不清 B 的列表/选择/展示', async () => {
    const c = setup()
    inBoardAFilled(c)
    const late = deferred<{ success: false, error: string }>()
    reportsMocks.listBoardTopics.mockReturnValueOnce(late.promise)
    const p = c.loadTopics(7701) // A 的请求在途

    // 切 B 并已装载成功视图（B 的话题列表已返回、topic 202 选中、展示已就绪）
    c.activateBoardContext(8802)
    reportsMocks.listBoardTopics.mockResolvedValueOnce({ success: true, data: { topics: [topicListItem(202)] } })
    await c.loadTopics(8802)
    c.contexts.value = [contextRow(9)] // B 的展示哨兵
    expect(c.selectedTopicId.value).toBe(202)
    expect(c.error.value).toBeNull()

    // A 的失败迟到到达：不得清 B（不得 topics=[]/selected=null/清 refs/写 error）
    late.resolve({ success: false, error: 'A 炸了' })
    await p
    await vi.advanceTimersByTimeAsync(0)

    expect(c.topics.value.map(t => t.id)).toEqual([202])
    expect(c.selectedTopicId.value).toBe(202)
    expect(c.contexts.value.length).toBe(1) // B 的展示未被 A 的失败清空
    expect(c.error.value).toBeNull()
    expect(c.topicsLoading.value).toBe(false) // 不残留 loading
  })
})

// ── board qa（6.2：版块报告追问，独立 state + 双重迟到守卫）──────────────────
describe('board qa — 版块报告追问（board-level-deep-analysis 6.2，design D5）', () => {
  /** 手动可控的迟到响应。 */
  function deferredQA<T>() {
    let resolve!: (v: T) => void
    const promise = new Promise<T>((r) => {
      resolve = r
    })
    return { promise, resolve }
  }

  function qaRow(id: number, answer = '答（已验证）'): TopicEnrichmentQA {
    return { id, topic_enrichment_result_id: 88, question: 'q' + id, answer, tool_calls: [], source: 'qa', sedimented: false, created_at: '2026-09-01T10:00:00Z' }
  }

  /** 激活板块并装载结果列表（默认选中最新简报）。 */
  async function setupBoardWithResults(c: En, boardId: number, rows: BoardAnalysisResultRow[]) {
    c.activateBoardContext(boardId)
    apiMocks.listBoardAnalysisResults.mockResolvedValueOnce({ success: true, data: rows })
    await c.loadBoardAnalysisResults(boardId)
  }

  it('loadBoardQA：走 board 路由（boardId+resultId），写入独立 boardQa* state，不碰 topic QA refs', async () => {
    const c = setup()
    await setupBoardWithResults(c, 7701, [makeRow(88, 'board_brief')])
    expect(c.currentBoardResultId.value).toBe(88)
    apiMocks.listBoardQA.mockResolvedValueOnce({ success: true, data: [qaRow(1)] })

    await c.loadBoardQA(88)

    expect(apiMocks.listBoardQA).toHaveBeenCalledWith(7701, 88)
    expect(c.boardQaList.value).toHaveLength(1)
    expect(c.boardQaResultId.value).toBe(88)
    // 串台防护：topic 档 QA refs 原样（空），latestAnswer 也不动
    expect(c.qaList.value).toHaveLength(0)
    expect(c.latestAnswer.value).toBeNull()
  })

  it('askBoardQuestion：board 路由 ask（挂当前选中报告），成功后重拉列表并写 boardLatestAnswer', async () => {
    const c = setup()
    await setupBoardWithResults(c, 7701, [makeRow(88, 'board_brief')])
    apiMocks.askBoardQA.mockResolvedValueOnce({ success: true, data: { answer: '仍成立', tool_calls: [], refs: [] } })
    apiMocks.listBoardQA.mockResolvedValueOnce({ success: true, data: [qaRow(1, '仍成立')] })

    await c.askBoardQuestion('旧结论还成立吗')

    expect(apiMocks.askBoardQA).toHaveBeenCalledWith(7701, 88, '旧结论还成立吗')
    expect(apiMocks.listBoardQA).toHaveBeenCalledWith(7701, 88)
    expect(c.boardLatestAnswer.value?.answer).toBe('仍成立')
    expect(c.boardQaList.value).toHaveLength(1)
    expect(c.boardQaLoading.value).toBe(false)
  })

  it('askBoardQuestion：无版块报告时报错不动网络', async () => {
    const c = setup()
    c.activateBoardContext(7701)
    // 空列表 → selectedBoardResult null
    apiMocks.listBoardAnalysisResults.mockResolvedValueOnce({ success: true, data: [] })
    await c.loadBoardAnalysisResults(7701)
    expect(c.currentBoardResultId.value).toBeNull()

    const ok = await c.askBoardQuestion('q')

    expect(ok).toBe(false)
    expect(apiMocks.askBoardQA).not.toHaveBeenCalled()
    expect(notifyMocks.error).toHaveBeenCalled()
  })

  it('sedimentBoardAnswer：按 boardQaResultId 走 board 路由，原地替换回写行', async () => {
    const c = setup()
    await setupBoardWithResults(c, 7701, [makeRow(88, 'board_brief')])
    apiMocks.listBoardQA.mockResolvedValueOnce({ success: true, data: [qaRow(5)] })
    await c.loadBoardQA(88)
    apiMocks.sedimentBoardQA.mockResolvedValueOnce({ success: true, data: { ...qaRow(5), sedimented: true } })

    await c.sedimentBoardAnswer(5)

    expect(apiMocks.sedimentBoardQA).toHaveBeenCalledWith(7701, 88, 5)
    expect(c.boardQaList.value[0]?.sedimented).toBe(true)
    expect(notifyMocks.success).toHaveBeenCalledWith('已沉淀')
  })

  it('切历史报告后迟到 QA 响应：selectedBoardResultId 守卫丢弃，不写入新报告视图', async () => {
    const c = setup()
    await setupBoardWithResults(c, 7701, [makeRow(88, 'board_brief'), makeRow(99, 'legacy_board_analysis')])
    const late = deferredQA<{ success: true, data: TopicEnrichmentQA[] }>()
    apiMocks.listBoardQA.mockReturnValueOnce(late.promise)

    const p = c.loadBoardQA(88) // 88 的响应在途
    c.selectBoardResult(99) // 用户切到 legacy 报告
    late.resolve({ success: true, data: [qaRow(1)] }) // 88 的行迟到
    await p

    expect(c.boardQaList.value).toHaveLength(0) // 88 的 QA 不写入
    expect(c.boardQaResultId.value).toBeNull()
  })

  it('切板块后迟到 QA 响应：epoch 守卫丢弃 + 切板块即清 board QA refs', async () => {
    const c = setup()
    await setupBoardWithResults(c, 7701, [makeRow(88, 'board_brief')])
    apiMocks.listBoardQA.mockResolvedValueOnce({ success: true, data: [qaRow(1)] })
    await c.loadBoardQA(88)
    expect(c.boardQaList.value).toHaveLength(1)

    const late = deferredQA<{ success: true, data: TopicEnrichmentQA[] }>()
    apiMocks.listBoardQA.mockReturnValueOnce(late.promise)
    const p = c.loadBoardQA(88)
    c.activateBoardContext(8802) // A→B：同步清 board QA refs + epoch++
    expect(c.boardQaList.value).toHaveLength(0)
    expect(c.boardQaResultId.value).toBeNull()

    late.resolve({ success: true, data: [qaRow(2)] }) // A 的响应迟到
    await p

    expect(c.boardQaList.value).toHaveLength(0) // B 的 QA state 不被 A 污染
    expect(c.boardQaResultId.value).toBeNull()
  })

  it('loadBoardQA 失败：置 boardQaError + 清列表，不动 topic QA', async () => {
    const c = setup()
    await setupBoardWithResults(c, 7701, [makeRow(88, 'board_brief')])
    apiMocks.listBoardQA.mockResolvedValueOnce({ success: false, error: '加载追问历史失败' })

    await c.loadBoardQA(88)

    expect(c.boardQaList.value).toHaveLength(0)
    expect(c.boardQaError.value).toBe('加载追问历史失败')
    expect(c.qaError.value).toBeNull()
  })
})

// ── topic qa（6.x review hardening：视图身份守卫，与 board QA 同级）─────────
describe('topic qa — 聚焦分析追问迟到守卫（board-level-deep-analysis 6.x review）', () => {
  /** 手动可控的迟到响应。 */
  function deferredQA<T>() {
    let resolve!: (v: T) => void
    const promise = new Promise<T>((r) => {
      resolve = r
    })
    return { promise, resolve }
  }

  function qaRow(id: number, answer = '答（已验证）'): TopicEnrichmentQA {
    return { id, topic_enrichment_result_id: 55, question: 'q' + id, answer, tool_calls: [], source: 'qa', sedimented: false, created_at: '2026-09-01T10:00:00Z' }
  }

  function topicResultRow(id: number): ResultSummaryRow {
    return { id, evolution_assessment: 'assess', sectors: null, tool_calls_count: 0, session_id: 's', created_at: '2026-09-01T10:00:00Z' }
  }

  /** 激活板块 A 选中 topic 101、results=[55]，QA 哨兵就绪（latestResultId=55）。 */
  function inTopicView(c: En) {
    c.activateBoardContext(7701)
    c.selectedTopicId.value = 101
    c.results.value = [topicResultRow(55)]
    c.qaList.value = [qaRow(7)]
    c.latestAnswer.value = { answer: '旧答案', tool_calls: [], refs: [] }
  }

  it('当前请求正常：loadQA 写列表、askQuestion 写 latestAnswer+重拉、sedimentAnswer 原地替换+toast', async () => {
    const c = setup()
    inTopicView(c)

    apiMocks.listQA.mockResolvedValueOnce({ success: true, data: [qaRow(7, '历史答')] })
    await c.loadQA(55)
    expect(apiMocks.listQA).toHaveBeenCalledWith(101, 55)
    expect(c.qaList.value).toHaveLength(1)
    expect(c.qaError.value).toBeNull()
    expect(c.qaLoading.value).toBe(false)

    apiMocks.askQA.mockResolvedValueOnce({ success: true, data: { answer: '新答', tool_calls: [], refs: [] } })
    apiMocks.listQA.mockResolvedValueOnce({ success: true, data: [qaRow(7), qaRow(8)] })
    await c.askQuestion('新问题')
    expect(apiMocks.askQA).toHaveBeenCalledWith(101, 55, '新问题')
    expect(c.latestAnswer.value?.answer).toBe('新答')
    expect(c.qaList.value).toHaveLength(2)
    expect(c.qaLoading.value).toBe(false)

    apiMocks.sedimentQA.mockResolvedValueOnce({ success: true, data: { ...qaRow(8), sedimented: true } })
    await c.sedimentAnswer(8)
    expect(apiMocks.sedimentQA).toHaveBeenCalledWith(101, 8)
    expect(c.qaList.value[1]?.sedimented).toBe(true)
    expect(notifyMocks.success).toHaveBeenCalledWith('已沉淀')
  })

  it('loadQA 迟到切新 topic：不写 qaList/qaError，不清新视图在途 loading；新请求正常完成', async () => {
    const c = setup()
    inTopicView(c)
    const late = deferredQA<{ success: true, data: TopicEnrichmentQA[] }>()
    apiMocks.listQA.mockReturnValueOnce(late.promise)
    const p = c.loadQA(55) // topic 101 的响应在途

    c.selectedTopicId.value = 102 // 切新 topic，新视图自己拉
    const fresh = deferredQA<{ success: true, data: TopicEnrichmentQA[] }>()
    apiMocks.listQA.mockReturnValueOnce(fresh.promise)
    const p2 = c.loadQA(55)
    expect(c.qaLoading.value).toBe(true) // B 在途

    late.resolve({ success: true, data: [qaRow(9)] }) // 101 的行迟到
    await p
    expect(c.qaList.value).toEqual([qaRow(7)]) // 旧视图哨兵未被 A 覆盖
    expect(c.qaError.value).toBeNull()
    expect(c.qaLoading.value).toBe(true) // 旧响应未清新视图在途 loading

    fresh.resolve({ success: true, data: [qaRow(10)] })
    await p2
    expect(c.qaList.value).toEqual([qaRow(10)])
    expect(c.qaLoading.value).toBe(false)
  })

  it('loadQA 迟到换最新报告：不写入，后续新请求照常', async () => {
    const c = setup()
    inTopicView(c)
    const late = deferredQA<{ success: true, data: TopicEnrichmentQA[] }>()
    apiMocks.listQA.mockReturnValueOnce(late.promise)
    const p = c.loadQA(55) // 55 的响应在途

    c.results.value = [topicResultRow(56)] // 最新报告换新
    late.resolve({ success: true, data: [qaRow(9)] }) // 55 的行迟到
    await p
    expect(c.qaList.value).toEqual([qaRow(7)]) // 不写入
    expect(c.qaError.value).toBeNull()

    apiMocks.listQA.mockResolvedValueOnce({ success: true, data: [qaRow(11)] })
    await c.loadQA(56)
    expect(c.qaList.value).toEqual([qaRow(11)])
    expect(c.qaLoading.value).toBe(false)
  })

  it('askQuestion 迟到切新 topic：不写 latestAnswer/qaError、不重拉不 toast，旧 finally 不清新视图在途 loading', async () => {
    const c = setup()
    inTopicView(c)
    const late = deferredQA<{ success: true, data: { answer: string, tool_calls: never[], refs: never[] } }>()
    apiMocks.askQA.mockReturnValueOnce(late.promise)
    const p = c.askQuestion('旧问题') // topic 101 的 ask 在途
    expect(c.qaLoading.value).toBe(true)

    c.selectedTopicId.value = 102 // 切新 topic，新视图自己问
    const fresh = deferredQA<{ success: true, data: { answer: string, tool_calls: never[], refs: never[] } }>()
    apiMocks.askQA.mockReturnValueOnce(fresh.promise)
    const p2 = c.askQuestion('新问题')

    late.resolve({ success: true, data: { answer: 'A答', tool_calls: [], refs: [] } }) // A 的 ask 迟到
    await p
    expect(c.latestAnswer.value?.answer).toBe('旧答案') // 哨兵未被 A 覆盖
    expect(c.qaError.value).toBeNull()
    expect(apiMocks.listQA).not.toHaveBeenCalled() // 迟到成功不重拉
    expect(notifyMocks.error).not.toHaveBeenCalled()
    expect(c.qaLoading.value).toBe(true) // B 在途 loading 未被旧 finally 清掉

    apiMocks.listQA.mockResolvedValueOnce({ success: true, data: [qaRow(7), qaRow(8)] })
    fresh.resolve({ success: true, data: { answer: 'B答', tool_calls: [], refs: [] } })
    await p2
    expect(c.latestAnswer.value?.answer).toBe('B答')
    expect(c.qaList.value).toHaveLength(2)
    expect(c.qaLoading.value).toBe(false)
  })

  it('askQuestion 迟到换最新报告：不写 latestAnswer、不重拉', async () => {
    const c = setup()
    inTopicView(c)
    const late = deferredQA<{ success: true, data: { answer: string, tool_calls: never[], refs: never[] } }>()
    apiMocks.askQA.mockReturnValueOnce(late.promise)
    const p = c.askQuestion('旧问题') // 挂在 55 上的 ask 在途

    c.results.value = [topicResultRow(56)] // 最新报告换新
    late.resolve({ success: true, data: { answer: 'A答', tool_calls: [], refs: [] } })
    await p
    expect(c.latestAnswer.value?.answer).toBe('旧答案') // 不写
    expect(apiMocks.listQA).not.toHaveBeenCalled() // 不重拉
    expect(notifyMocks.error).not.toHaveBeenCalled()
  })

  it('sedimentAnswer 迟到切新 topic / 换最新报告：不替换哨兵行、不 toast', async () => {
    const c = setup()
    inTopicView(c)

    // 切新 topic 场景
    const late1 = deferredQA<{ success: true, data: TopicEnrichmentQA }>()
    apiMocks.sedimentQA.mockReturnValueOnce(late1.promise)
    const p1 = c.sedimentAnswer(7)
    c.selectedTopicId.value = 102
    late1.resolve({ success: true, data: { ...qaRow(7), sedimented: true } })
    await p1
    expect(c.qaList.value[0]?.sedimented).toBe(false) // 哨兵行未被替换
    expect(notifyMocks.success).not.toHaveBeenCalled()

    // 换最新报告场景
    const late2 = deferredQA<{ success: true, data: TopicEnrichmentQA }>()
    apiMocks.sedimentQA.mockReturnValueOnce(late2.promise)
    const p2 = c.sedimentAnswer(7)
    c.results.value = [topicResultRow(56)]
    late2.resolve({ success: true, data: { ...qaRow(7), sedimented: true } })
    await p2
    expect(c.qaList.value[0]?.sedimented).toBe(false)
    expect(notifyMocks.success).not.toHaveBeenCalled()
  })
})
