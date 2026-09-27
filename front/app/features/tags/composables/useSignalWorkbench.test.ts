/**
 * useSignalWorkbench — 信号工作台轮询状态机（board-signal-reports 5.2/5.4）。
 *
 * FE-1：发现只发 discovery 一个请求，终态只重拉列表，绝不串发 research
 * FE-4：no_signal 安静空态（不弹消息）；failed 错误态可重试
 * FE-12：job 404（后端重启）→ 停轮询 + 提示可重试 + 重拉候选/报告列表
 *        （派生「研究中」来自 live job，重启不卡死）
 * 5.4：409 恢复原 job；200 already_reported 零费用直接打开报告
 *      succeeded → 用 result_id 打开报告详情
 *
 * API 全 mock + fake timers 驱动串行轮询（3s 周期、迟到响应身份守卫）。
 */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { useSignalWorkbench } from './useSignalWorkbench'

const apiMocks = vi.hoisted(() => ({
  triggerSignalDiscovery: vi.fn(),
  listSignalCandidates: vi.fn(),
  triggerSignalResearch: vi.fn(),
  listSignalReports: vi.fn(),
  getSignalReport: vi.fn(),
  getSignalJobStatus: vi.fn(),
  getSignalResearchProgress: vi.fn(),
  getBoardAnalysisStatus: vi.fn(),
}))
const notifyMocks = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warn: vi.fn(),
}))

vi.mock('~/api/boardSignals', () => ({
  useBoardSignalsApi: () => apiMocks,
}))
vi.mock('~/composables/useNotify', () => ({
  useNotify: () => notifyMocks,
}))

function startedDiscovery(jobId = 'dj1') {
  return { success: true, data: { status: 'started', job_id: jobId, job_kind: 'board_signal_discovery', scope: 'board', target_id: 9, granularity: 'month', period: '2026-09' } }
}
function startedResearch(jobId = 'rj1', candidateId = 31) {
  return { success: true, data: { status: 'started', job_id: jobId, job_kind: 'board_signal_report', scope: 'board', target_id: 9, candidate_id: candidateId, granularity: 'month', period: '2026-09' } }
}
function jobStatus(overrides: Record<string, unknown> = {}) {
  return { success: true, data: { job_id: 'dj1', job_kind: 'board_signal_discovery', scope: 'board', target_id: 9, running: false, finished: true, ...overrides } }
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  apiMocks.listSignalCandidates.mockResolvedValue({ success: true, data: [] })
  apiMocks.listSignalReports.mockResolvedValue({ success: true, data: [] })
  apiMocks.getSignalResearchProgress.mockResolvedValue({ success: true, data: null })
  apiMocks.getBoardAnalysisStatus.mockResolvedValue({ success: true, data: { scope: 'board', target_id: 9, running: false, finished: false } })
})
afterEach(() => {
  vi.useRealTimers()
})

describe('useSignalWorkbench 发现链路（FE-1/FE-4）', () => {
  it('FE-1：发现 202 → 只轮询 discovery job；终态 discovered 只重拉候选/报告，绝不串发 research', async () => {
    apiMocks.triggerSignalDiscovery.mockResolvedValue(startedDiscovery())
    apiMocks.getSignalJobStatus
      .mockResolvedValueOnce(jobStatus({ running: true, phase: 'detect' }))
      .mockResolvedValue(jobStatus({ outcome: 'discovered', discovery_id: 8, candidate_count: 2 }))

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.discover(9)
    expect(apiMocks.triggerSignalDiscovery).toHaveBeenCalledWith(9, { granularity: 'month', period: '2026-09' })
    expect(ws.discoveryRunning.value).toBe(true)

    await vi.advanceTimersByTimeAsync(0) // 首发轮询：running → 排下一发
    expect(apiMocks.getSignalJobStatus).toHaveBeenCalledTimes(1)
    expect(ws.discoveryPhase.value).toBe('detect')
    expect(apiMocks.triggerSignalResearch).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(3000) // 终态：discovered
    expect(ws.discoveryRunning.value).toBe(false)
    // 终态只重拉列表
    expect(apiMocks.listSignalCandidates).toHaveBeenCalledWith(9, { granularity: 'month', period: '2026-09' })
    expect(apiMocks.listSignalReports).toHaveBeenCalled()
    // FE-1 核心：全链零 research 请求
    expect(apiMocks.triggerSignalResearch).not.toHaveBeenCalled()
    ws.closeReport()
  })

  it('FE-4：no_signal 安静空态——不弹消息、终态只重拉列表（不清旧记录由服务端保证）', async () => {
    apiMocks.triggerSignalDiscovery.mockResolvedValue(startedDiscovery())
    apiMocks.getSignalJobStatus.mockResolvedValue(jobStatus({ outcome: 'no_signal' }))

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.discover(9)
    await vi.advanceTimersByTimeAsync(0)
    expect(ws.discoveryRunning.value).toBe(false)
    expect(ws.discoveryError.value).toBeNull()
    expect(notifyMocks.success).not.toHaveBeenCalled()
    expect(notifyMocks.warn).not.toHaveBeenCalled()
    expect(notifyMocks.error).not.toHaveBeenCalled()
    ws.closeReport()
  })

  it('FE-4：failed → discoveryError 置错（可重试），不伪装零信号', async () => {
    apiMocks.triggerSignalDiscovery.mockResolvedValue(startedDiscovery())
    apiMocks.getSignalJobStatus.mockResolvedValue(jobStatus({ outcome: 'failed', error: 'detect 两次尝试仍不可解析', error_stage: 'detect' }))

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.discover(9)
    await vi.advanceTimersByTimeAsync(0)
    expect(ws.discoveryError.value).toContain('detect 两次尝试仍不可解析')
    ws.closeReport()
  })

  it('周期格式非法：不发任何请求，直接显错', async () => {
    const ws = useSignalWorkbench()
    ws.period.value = '2026-13'
    const ok = await ws.discover(9)
    expect(ok).toBe(false)
    expect(apiMocks.triggerSignalDiscovery).not.toHaveBeenCalled()
  })
})

describe('useSignalWorkbench 研究链路（5.4）', () => {
  it('research 202 → 轮询 research job；succeeded → 用 result_id 打开报告详情 + 重拉列表', async () => {
    apiMocks.triggerSignalResearch.mockResolvedValue(startedResearch('rj1', 31))
    apiMocks.getSignalJobStatus
      .mockResolvedValueOnce(jobStatus({ job_kind: 'board_signal_report', running: true, phase: 'compose', candidate_id: 31 }))
      .mockResolvedValue(jobStatus({ job_kind: 'board_signal_report', outcome: 'succeeded', result_id: 501, candidate_id: 31 }))
    apiMocks.getSignalReport.mockResolvedValue({ success: true, data: { id: 501, sectors: { schema_version: 2 } } })

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.research(9, 31)
    expect(ws.researchCandidateId.value).toBe(31)

    await vi.advanceTimersByTimeAsync(0)
    expect(ws.researchPhase.value).toBe('compose')
    await vi.advanceTimersByTimeAsync(3000)
    expect(ws.researchCandidateId.value).toBeNull() // 终态停轮询
    expect(apiMocks.getSignalReport).toHaveBeenCalledWith(9, 501)
    expect(ws.activeReport.value).not.toBeNull()
    expect(notifyMocks.success).toHaveBeenCalledWith('研究完成，可阅读报告')
  })

  it('research failed → researchError 可重试 + 重拉列表（无 result 写入）', async () => {
    apiMocks.triggerSignalResearch.mockResolvedValue(startedResearch())
    apiMocks.getSignalJobStatus.mockResolvedValue(jobStatus({ job_kind: 'board_signal_report', outcome: 'failed', error: 'compose 三次校验未通过', error_stage: 'compose', candidate_id: 31 }))

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.research(9, 31)
    await vi.advanceTimersByTimeAsync(0)
    expect(ws.researchError.value).toContain('compose 三次校验未通过')
    expect(ws.researchCandidateId.value).toBeNull() // 候选回待研究
    expect(apiMocks.getSignalReport).not.toHaveBeenCalled()
  })

  it('4.7：research failed 错误文案携带已保留进展摘要（从重拉后的候选行读）', async () => {
    apiMocks.triggerSignalResearch.mockResolvedValue(startedResearch('rj1', 31))
    apiMocks.getSignalJobStatus.mockResolvedValue(jobStatus({ job_kind: 'board_signal_report', outcome: 'failed', error: '研究被取消或超时', error_stage: 'research', candidate_id: 31 }))
    // 重拉后的候选行携带 abandoned 进展行（后端每轮滚动 upsert + 终态保留）。
    apiMocks.listSignalCandidates.mockResolvedValue({
      success: true,
      data: [
        {
          id: 31, discovery_id: 2, granularity: 'month', period: '2026-09',
          signal: '库存超季节性去库', why_it_matters: 'w', research_question: 'q',
          evidence_refs: ['31'], score: 8, rationale: 'r', discovery_created_at: '2026-09-01T00:00:00Z',
          status: 'pending', latest_result_id: null,
          last_research_progress: {
            rounds_done: 8, source_calls: 9, calculation_calls: 2,
            status: 'abandoned', stop_reason: 'timeout', updated_at: '2026-09-22T10:00:00Z',
          },
        },
      ],
    })

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.research(9, 31)
    await vi.runAllTimersAsync() // 摘要在重拉列表之后追加，等全部定时器/微任务落定
    expect(ws.researchError.value).toContain('研究被取消或超时')
    expect(ws.researchError.value).toContain('已保留 8 轮进展（9 次取数）')
  })

  it('4.7：无进展（rounds_done=0/无摘要）时不附加进展片段', async () => {
    apiMocks.triggerSignalResearch.mockResolvedValue(startedResearch('rj1', 31))
    apiMocks.getSignalJobStatus.mockResolvedValue(jobStatus({ job_kind: 'board_signal_report', outcome: 'failed', error: '研究循环异常终止', candidate_id: 31 }))
    apiMocks.listSignalCandidates.mockResolvedValue({ success: true, data: [] })

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.research(9, 31)
    await vi.runAllTimersAsync()
    expect(ws.researchError.value).toBe('研究循环异常终止')
    expect(ws.researchError.value).not.toContain('已保留')
  })

  it('5.4：重复点击 409 → 按冲突体身份恢复原 job 轮询（不重复启动）', async () => {
    apiMocks.triggerSignalResearch.mockResolvedValue({
      success: false,
      status: 409,
      error: 'board analysis already running',
      data: { job_id: 'rj-running', job_kind: 'board_signal_report', scope: 'board', target_id: 9, running: true, candidate_id: 31 },
    })
    apiMocks.getSignalJobStatus
      .mockResolvedValueOnce(jobStatus({ job_id: 'rj-running', job_kind: 'board_signal_report', running: true, phase: 'research', candidate_id: 31 }))
      .mockResolvedValue(jobStatus({ job_id: 'rj-running', job_kind: 'board_signal_report', outcome: 'succeeded', result_id: 502, candidate_id: 31 }))
    apiMocks.getSignalReport.mockResolvedValue({ success: true, data: { id: 502, sectors: { schema_version: 2 } } })

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.research(9, 31)
    expect(ws.researchJobId.value).toBe('rj-running') // 恢复原 job
    await vi.advanceTimersByTimeAsync(0)
    await vi.advanceTimersByTimeAsync(3000)
    expect(apiMocks.triggerSignalResearch).toHaveBeenCalledTimes(1) // 未重复启动
    expect(apiMocks.getSignalReport).toHaveBeenCalledWith(9, 502)
  })

  it('5.4：完成后默认点击 → 200 already_reported 直接打开报告（零新请求零费用）', async () => {
    apiMocks.triggerSignalResearch.mockResolvedValue({ success: true, data: { status: 'already_reported', result_id: 500 } })
    apiMocks.getSignalReport.mockResolvedValue({ success: true, data: { id: 500, sectors: { schema_version: 2 } } })

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.research(9, 31)
    expect(apiMocks.triggerSignalResearch).toHaveBeenCalledTimes(1)
    expect(apiMocks.getSignalReport).toHaveBeenCalledWith(9, 500) // 直接打开已有报告
    expect(apiMocks.getSignalJobStatus).not.toHaveBeenCalled()
  })

  it('FE-12：job 404（后端重启）→ 停轮询 + 提示可重试 + 重拉候选/报告列表', async () => {
    apiMocks.triggerSignalResearch.mockResolvedValue(startedResearch())
    apiMocks.getSignalJobStatus.mockResolvedValue({ success: false, status: 404, error: 'signal candidate not found' })

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.research(9, 31)
    await vi.advanceTimersByTimeAsync(0)
    expect(ws.researchError.value).toContain('已失效')
    expect(ws.researchError.value).toContain('重试')
    expect(ws.researchCandidateId.value).toBeNull() // 不永久卡「研究中」
    expect(apiMocks.listSignalCandidates).toHaveBeenCalled() // 重新拉列表
    expect(apiMocks.listSignalReports).toHaveBeenCalled()
    // 停止轮询：再推进无新请求
    const calls = apiMocks.getSignalJobStatus.mock.calls.length
    await vi.advanceTimersByTimeAsync(9000)
    expect(apiMocks.getSignalJobStatus.mock.calls.length).toBe(calls)
  })
})


describe('useSignalWorkbench 研究进展实时展示（4.8）', () => {
  function progressData(overrides: Record<string, unknown> = {}) {
    return {
      rounds_done: 5, source_calls: 6, calculation_calls: 1,
      status: 'running', stop_reason: null, updated_at: '2026-09-23T12:00:00Z',
      job_id: 'rj1', candidate_id: 31, semantic_board_id: 9,
      granularity: 'month', period: '2026-09',
      ledger: {
        calls: [
          { call_id: 'c1', question: '当前库存多少？', tool: 'eia_weekly', status: 'ok', observations: [{ observation_id: 'c1:o1' }] },
        ],
        calculations: [
          { calc_id: 'k1', op: 'difference', inputs: ['c1:o1'], expression: 'c1:o1-c1:o2', value: '1.5', unit: 'million barrels', precision: 4, status: 'ok', computed_at: '2026-09-23T12:00:00Z' },
        ],
        gaps: [{ reason: '炼厂开工率无数据源' }],
      },
      error: null, created_at: '2026-09-23T11:00:00Z',
      ...overrides,
    }
  }

  it('进行中：每轮轮询顺带拉 research-progress，label 显示真实计数（第N/40轮·取数·计算）', async () => {
    apiMocks.triggerSignalResearch.mockResolvedValue(startedResearch('rj1', 31))
    apiMocks.getSignalJobStatus
      .mockResolvedValue(jobStatus({ job_kind: 'board_signal_report', running: true, phase: 'research', candidate_id: 31 }))
    apiMocks.getSignalResearchProgress.mockResolvedValue({ success: true, data: progressData() })

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.research(9, 31)
    await vi.advanceTimersByTimeAsync(0) // 首发轮询：running → 拉进展 → 排下一发
    expect(apiMocks.getSignalResearchProgress).toHaveBeenCalledWith(9, 31)
    expect(ws.researchProgress.value?.rounds_done).toBe(5)
    expect(ws.researchProgressLabel.value).toContain('第 5/40 轮')
    expect(ws.researchProgressLabel.value).toContain('取数 6')
    expect(ws.researchProgressLabel.value).toContain('计算 1')
    // 轮询主链路不受进展拉取影响：下一发照排
    const statusCalls = apiMocks.getSignalJobStatus.mock.calls.length
    await vi.advanceTimersByTimeAsync(3000)
    expect(apiMocks.getSignalJobStatus.mock.calls.length).toBe(statusCalls + 1)
  })

  it('无进展数据（首轮未落库）时 label 回落「决策上限」文案，不显示 0/40', async () => {
    apiMocks.triggerSignalResearch.mockResolvedValue(startedResearch('rj1', 31))
    apiMocks.getSignalJobStatus.mockResolvedValue(jobStatus({ job_kind: 'board_signal_report', running: true, phase: 'research', candidate_id: 31 }))
    apiMocks.getSignalResearchProgress.mockResolvedValue({ success: true, data: null })

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.research(9, 31)
    await vi.advanceTimersByTimeAsync(0)
    expect(ws.researchProgress.value).toBeNull()
    expect(ws.researchProgressLabel.value).toContain('决策上限 40 轮')
    expect(ws.researchProgressLabel.value).not.toContain('0/40')
  })

  it('failed 终态：全量进展保留供错误区回看（断了不能白跑从摘要升级为全程）', async () => {
    apiMocks.triggerSignalResearch.mockResolvedValue(startedResearch('rj1', 31))
    apiMocks.getSignalJobStatus
      .mockResolvedValue(jobStatus({ job_kind: 'board_signal_report', running: true, phase: 'research', candidate_id: 31 }))
    apiMocks.getSignalResearchProgress
      .mockResolvedValueOnce({ success: true, data: progressData() }) // running 时拉到 running 态
      .mockResolvedValue({ success: true, data: progressData({ rounds_done: 8, source_calls: 9, status: 'abandoned', stop_reason: 'timeout' }) })
    apiMocks.getSignalJobStatus.mockResolvedValueOnce(jobStatus({ job_kind: 'board_signal_report', running: true, phase: 'research', candidate_id: 31 }))
      .mockResolvedValueOnce(jobStatus({ job_kind: 'board_signal_report', outcome: 'failed', error: '研究超时', error_stage: 'research', candidate_id: 31 }))

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.research(9, 31)
    await vi.advanceTimersByTimeAsync(0) // 首发 running + 拉进展
    await vi.advanceTimersByTimeAsync(3000) // 终态 failed + 再拉一次全量
    expect(ws.researchError.value).toContain('研究超时')
    expect(ws.researchProgress.value).not.toBeNull()
    expect(ws.researchProgress.value?.rounds_done).toBe(8)
    expect(ws.researchProgress.value?.ledger.calls.length).toBe(1)
  })

  it('succeeded 终态：researchProgress 清空（每步链路由报告 appendix 接管）', async () => {
    apiMocks.triggerSignalResearch.mockResolvedValue(startedResearch('rj1', 31))
    apiMocks.getSignalJobStatus
      .mockResolvedValue(jobStatus({ job_kind: 'board_signal_report', running: true, phase: 'research', candidate_id: 31 }))
    apiMocks.getSignalResearchProgress.mockResolvedValue({ success: true, data: progressData() })
    apiMocks.getSignalJobStatus.mockResolvedValueOnce(jobStatus({ job_kind: 'board_signal_report', running: true, phase: 'research', candidate_id: 31 }))
      .mockResolvedValueOnce(jobStatus({ job_kind: 'board_signal_report', outcome: 'succeeded', result_id: 501, candidate_id: 31 }))
    apiMocks.getSignalReport.mockResolvedValue({ success: true, data: { id: 501, sectors: { schema_version: 2 } } })

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.research(9, 31)
    await vi.advanceTimersByTimeAsync(0)
    expect(ws.researchProgress.value).not.toBeNull()
    await vi.advanceTimersByTimeAsync(3000)
    expect(ws.researchCandidateId.value).toBeNull() // 终态
    expect(ws.researchProgress.value).toBeNull()
  })

  it('404（后端重启）：停轮询后仍拉一次进展——已落库轮次在错误区可回看', async () => {
    apiMocks.triggerSignalResearch.mockResolvedValue(startedResearch())
    apiMocks.getSignalJobStatus.mockResolvedValue({ success: false, status: 404, error: 'job not found' })
    apiMocks.getSignalResearchProgress.mockResolvedValue({ success: true, data: progressData({ status: 'abandoned', stop_reason: 'timeout', rounds_done: 3 }) })

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.research(9, 31)
    await vi.advanceTimersByTimeAsync(0)
    expect(ws.researchError.value).toContain('已失效')
    expect(apiMocks.getSignalResearchProgress).toHaveBeenCalledWith(9, 31)
    expect(ws.researchProgress.value?.rounds_done).toBe(3)
  })

  it('进展端点瞬时失败：静默不炸轮询主链路（下轮再拉）', async () => {
    apiMocks.triggerSignalResearch.mockResolvedValue(startedResearch('rj1', 31))
    apiMocks.getSignalJobStatus.mockResolvedValue(jobStatus({ job_kind: 'board_signal_report', running: true, phase: 'research', candidate_id: 31 }))
    apiMocks.getSignalResearchProgress.mockRejectedValue(new Error('network hiccup'))

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.research(9, 31)
    await vi.advanceTimersByTimeAsync(0)
    expect(ws.researchCandidateId.value).toBe(31) // 轮询未中断
    const statusCalls = apiMocks.getSignalJobStatus.mock.calls.length
    await vi.advanceTimersByTimeAsync(3000)
    expect(apiMocks.getSignalJobStatus.mock.calls.length).toBe(statusCalls + 1)
  })

  it('刷新后自动恢复：analysis-status 报 running research → 接回轮询（进度不再静默丢失）', async () => {
    apiMocks.getBoardAnalysisStatus.mockResolvedValue({
      success: true,
      data: { scope: 'board', target_id: 9, running: true, finished: false, job_id: 'rj-live', job_kind: 'board_signal_report', candidate_id: 4, phase: 'research' },
    })
    apiMocks.getSignalJobStatus.mockResolvedValue(jobStatus({ job_kind: 'board_signal_report', running: true, phase: 'research', candidate_id: 4 }))
    apiMocks.getSignalResearchProgress.mockResolvedValue({
      success: true,
      data: progressData({ rounds_done: 6, source_calls: 4, job_id: 'rj-live', candidate_id: 4 }),
    })

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.loadPeriod(9) // bootstrap 入口：拉列表 + sync
    expect(ws.researchCandidateId.value).toBe(4) // 自动接管
    expect(ws.researchJobId.value).toBe('rj-live')
    expect(notifyMocks.warn).toHaveBeenCalledWith('研究进行中，已恢复进度显示')
    await vi.advanceTimersByTimeAsync(0) // 恢复的首发轮询 + 进展拉取
    expect(ws.researchProgressLabel.value).toContain('第 6/40 轮')
  })

  it('刷新恢复 discovery running → 接回发现轮询；idle 不接管不弹提示', async () => {
    apiMocks.getBoardAnalysisStatus.mockResolvedValue({
      success: true,
      data: { scope: 'board', target_id: 9, running: true, finished: false, job_id: 'dj-live', job_kind: 'board_signal_discovery' },
    })
    apiMocks.getSignalJobStatus.mockResolvedValue(jobStatus({ outcome: 'discovered', discovery_id: 8 }))

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.loadPeriod(9)
    expect(ws.discoveryRunning.value).toBe(true)
    expect(notifyMocks.warn).toHaveBeenCalledWith('信号发现进行中，已恢复进度显示')

    ws.setBoard(99) // 停掉 ws 的轮询，避免共享 fake timers 污染下一段断言
    const ws2 = useSignalWorkbench()
    apiMocks.getBoardAnalysisStatus.mockResolvedValue({ success: true, data: { scope: 'board', target_id: 9, running: false, finished: false } })
    apiMocks.getSignalJobStatus.mockClear()
    await ws2.loadPeriod(9)
    expect(ws2.discoveryRunning.value).toBe(false)
    expect(ws2.researchCandidateId.value).toBeNull()
    await vi.advanceTimersByTimeAsync(0)
    expect(apiMocks.getSignalJobStatus).not.toHaveBeenCalled() // idle 零轮询
    ws.closeReport(); ws2.closeReport()
  })

  it('openProgressHistory：拉历史进展供行内回看；进行中时拒绝（防覆盖实时数据）', async () => {
    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.openProgressHistory(9, 6)
    expect(apiMocks.getSignalResearchProgress).toHaveBeenCalledWith(9, 6)
    expect(ws.researchProgress.value).toBeNull() // 候选无进展 → null

    // 进行中拒绝
    apiMocks.getBoardAnalysisStatus.mockResolvedValue({
      success: true,
      data: { scope: 'board', target_id: 9, running: true, finished: false, job_id: 'rj-x', job_kind: 'board_signal_report', candidate_id: 4 },
    })
    apiMocks.getSignalJobStatus.mockResolvedValue(jobStatus({ job_kind: 'board_signal_report', running: true, phase: 'research', candidate_id: 4 }))
    await ws.loadPeriod(9) // 接管 running job
    await ws.openProgressHistory(9, 6)
    expect(notifyMocks.warn).toHaveBeenLastCalledWith('研究进行中，先看实时进度')
    ws.closeReport()
  })
})

describe('useSignalWorkbench 视图守卫与周期', () => {
  it('setBoard 切板块：epoch++ 使在途响应失效，任务态清零', async () => {
    apiMocks.triggerSignalDiscovery.mockResolvedValue(startedDiscovery())
    apiMocks.getSignalJobStatus.mockResolvedValue(jobStatus({ outcome: 'discovered', discovery_id: 8 }))

    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.discover(9)
    ws.setBoard(10) // 切板块（旧板块轮询停、视图重置）
    expect(ws.candidates.value).toEqual([])
    expect(ws.activeReportId.value).toBeNull()
    await vi.advanceTimersByTimeAsync(9000)
    expect(apiMocks.getSignalJobStatus).not.toHaveBeenCalled() // 旧轮询不复活
  })

  it('changePeriod：形状非法拒绝（不发请求）；合法则换周期重拉', async () => {
    const ws = useSignalWorkbench()
    const bad = await ws.changePeriod(9, 'month', '2026-13')
    expect(bad).toBe(false)
    expect(apiMocks.listSignalCandidates).not.toHaveBeenCalled()

    const ok = await ws.changePeriod(9, 'year', '2026')
    expect(ok).toBe(true)
    expect(apiMocks.listSignalCandidates).toHaveBeenCalledWith(9, { granularity: 'year', period: '2026' })
    expect(apiMocks.listSignalReports).toHaveBeenCalled()
  })

  it('list error：错误态不伪装无数据（candidatesError/reportsError 置错）', async () => {
    apiMocks.listSignalCandidates.mockResolvedValue({ success: false, error: 'db down' })
    apiMocks.listSignalReports.mockResolvedValue({ success: true, data: [] })
    const ws = useSignalWorkbench()
    ws.period.value = '2026-09'
    await ws.loadPeriod(9)
    expect(ws.candidatesError.value).toBe('db down')
    expect(ws.reportsError.value).toBeNull()
  })
})
