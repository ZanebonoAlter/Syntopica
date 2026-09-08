/**
 * BoardEnrichmentPanel — bootstrap 真实链路集成（Critical 修复回归）。
 *
 * 与 BoardEnrichmentPanel.test.ts（mock 整个 composable，锁组件侧调用契约）不同，
 * 本文件只 mock API 层（~/api/boardEnrichment / ~/api/dailyReports / useNotify），
 * 走真实 useBoardEnrichment 组合函数，覆盖 board-level-deep-analysis 5.5 review
 * Critical 修复的端到端语义：
 *
 *  - bootstrap 顺序：loadTopics 定 topic → loadAllTopicTables（拉表）→
 *    syncTopicAnalysisStatus（最后收尾）——topic 档 status 查询必须发生在
 *    loadAll 三表加载之后（loadAll 入口 stopTopicPoll，反序会误杀恢复轮询）
 *  - running 状态下 bootstrap 结束后 topic poll 仍 active：3s 周期持续发出
 *    topic status 请求（timer 未被 bootstrap 内任何后续步骤停掉）
 *  - 恢复的轮询终态语义照旧：finished → 「增强完成」+ 重拉结果表 + 停轮询
 *  - watch(selectedTopicId) 双发（挂载 null→id 触发 watch loadAll + bootstrap
 *    自身 loadAll）无害：两次装载都在 sync 之前完成入口 stop
 */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import BoardEnrichmentPanel from './BoardEnrichmentPanel.vue'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

const apiMocks = vi.hoisted(() => ({
  triggerRelationDiscovery: vi.fn(),
  listBoardRelations: vi.fn(),
  getBoardRelation: vi.fn(),
  confirmBoardRelation: vi.fn(),
  dismissBoardRelation: vi.fn(),
  reResolveBoardRelation: vi.fn(),
  triggerBoardAnalysis: vi.fn(),
  getAnalysisStatusByJobId: vi.fn(),
  getAnalysisStatus: vi.fn(),
  listBoardAnalysisResults: vi.fn(),
  listDataSources: vi.fn(),
  triggerEnrichment: vi.fn(),
  listContexts: vi.fn(),
  listResults: vi.fn(),
  listReviews: vi.fn(),
  getResult: vi.fn(),
  listDebates: vi.fn(),
  triggerDebate: vi.fn(),
}))
const listBoardTopics = vi.fn()
const notifyMocks = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), warn: vi.fn() }))

vi.mock('~/api/boardEnrichment', () => ({
  useBoardEnrichmentApi: () => apiMocks,
}))
vi.mock('~/api/dailyReports', () => ({
  useDailyReportsApi: () => ({ listBoardTopics }),
}))
vi.mock('~/composables/useNotify', () => ({
  useNotify: () => notifyMocks,
}))
vi.mock('~/utils/markdown', () => ({
  renderMarkdown: (s: string) => s,
}))

// 注意：不 mock ~/features/tags/composables/useBoardEnrichment —— 走真实组合函数。

const stubs = {
  CausalAnalysisReport: { name: 'CausalAnalysisReport', template: '<div class="causal-stub" />' },
  BoardAnalysisReport: { name: 'BoardAnalysisReport', props: ['result', 'loading'], template: '<div class="board-report-stub" />' },
  BoardBriefReport: { name: 'BoardBriefReport', props: ['result', 'loading', 'investigationRunning'], template: '<div class="brief-stub" />' },
  BoardInvestigationReport: { name: 'BoardInvestigationReport', props: ['result', 'loading'], template: '<div class="investigation-stub" />' },
  QAPanel: { name: 'QAPanel', template: '<div class="qa-stub" />' },
  DebateSection: { name: 'DebateSection', template: '<div class="debate-stub" />' },
  AppDialog: { name: 'AppDialog', props: ['modelValue', 'title'], template: '<div class="dialog-stub" />' },
  AppButton: { name: 'AppButton', template: '<button class="app-btn-stub"><slot /></button>' },
  AppToggle: { name: 'AppToggle', template: '<div class="toggle-stub" />' },
}

/**
 * bootstrap 完成的确定性观测（不拍微任务次数）：sync 是 bootstrap 的最后一
 * 步，等「topic 档 status 已发出」成立即可断言整条链完成——vi.waitFor 在
 * fake timers 下自动推进虚拟时钟并反复排空微任务，链多深都稳。
 */
async function waitForBootstrapSync(): Promise<void> {
  await vi.waitFor(() => {
    expect(topicStatusCalls().length).toBeGreaterThan(0)
  })
}

const topicStatusCalls = () => apiMocks.getAnalysisStatus.mock.calls.filter(a => a[0] === 'topic')
/** 首个 topic 档 status 调用的全局序号（与其它 mock 的 invocationCallOrder 可比）。 */
function firstTopicStatusOrder(): number {
  const idx = apiMocks.getAnalysisStatus.mock.calls.findIndex(a => a[0] === 'topic')
  return idx >= 0 ? (apiMocks.getAnalysisStatus.mock.invocationCallOrder[idx] ?? 0) : 0
}

function mountPanel(): VueWrapper {
  return mount(BoardEnrichmentPanel, { props: { boardId: 7701 }, global: { stubs } })
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  // board 档 idle（不启 board 轮询，聚焦 topic 维度）；topic 101 running（恢复对象）
  apiMocks.getAnalysisStatus.mockImplementation((scope: string) =>
    scope === 'topic'
      ? { success: true, data: { scope: 'topic', target_id: 101, running: true, job_id: 'tj1', job_kind: 'topic_analysis' } }
      : { success: true, data: undefined },
  )
  listBoardTopics.mockResolvedValue({
    success: true,
    data: { topics: [{ id: 101, label: '伊朗局势', status: 'active' }] },
  })
  apiMocks.listBoardAnalysisResults.mockResolvedValue({ success: true, data: [] })
  apiMocks.listBoardRelations.mockResolvedValue({ success: true, data: [] })
  apiMocks.getBoardRelation.mockResolvedValue({ success: true, data: null })
  apiMocks.listDataSources.mockResolvedValue({ success: true, data: [] })
  apiMocks.listContexts.mockResolvedValue({ success: true, data: [] })
  apiMocks.listResults.mockResolvedValue({ success: true, data: [] })
  apiMocks.listReviews.mockResolvedValue({ success: true, data: [] })
  apiMocks.getResult.mockResolvedValue({ success: true, data: null })
  apiMocks.listDebates.mockResolvedValue({ success: true, data: [] })
})

afterEach(() => {
  vi.useRealTimers()
})

describe('BoardEnrichmentPanel bootstrap 真实链路（Critical：loadAll 先 / sync 最后 / 恢复轮询存活）', () => {
  it('顺序契约：topic 档 status（sync）发生在 loadAll 三表加载之后；bootstrap 结束后 running topic poll 仍 active，终态语义照旧', async () => {
    const wrapper = mountPanel()
    await waitForBootstrapSync()

    // ① bootstrap 已跑完：loadTopics 确定 topic 101，三表已装载（watch 双发 + bootstrap 自身，≥2 次）
    expect(listBoardTopics).toHaveBeenCalledWith(7701)
    expect(apiMocks.listContexts.mock.calls.filter(c => c[0] === 101).length).toBeGreaterThanOrEqual(2)
    expect(apiMocks.listResults).toHaveBeenCalledWith(101)
    expect(apiMocks.listReviews).toHaveBeenCalledWith(101)

    // ② 顺序契约：首个 topic 档 status 调用（sync 收尾）晚于首个 loadAll 表加载
    expect(topicStatusCalls().length).toBeGreaterThan(0)
    const loadAllOrder = apiMocks.listContexts.mock.invocationCallOrder[0] ?? 0
    expect(firstTopicStatusOrder()).toBeGreaterThan(loadAllOrder)

    // ③ running 状态下 bootstrap 结束后 topic poll 仍 active：每个 3s 周期持续发出
    const base = topicStatusCalls().length
    await vi.advanceTimersByTimeAsync(3000)
    expect(topicStatusCalls().length).toBe(base + 1)
    await vi.advanceTimersByTimeAsync(3000)
    expect(topicStatusCalls().length).toBe(base + 2)
    expect(apiMocks.getAnalysisStatus).toHaveBeenLastCalledWith('topic', 101)

    // ④ 终态语义照旧：finished → 「增强完成」+ 重拉结果表 + 轮询停止
    const resultsBefore = apiMocks.listResults.mock.calls.length
    apiMocks.getAnalysisStatus.mockImplementation((scope: string) =>
      scope === 'topic'
        ? { success: true, data: { scope: 'topic', target_id: 101, running: false, finished: true, result_id: 55 } }
        : { success: true, data: undefined },
    )
    await vi.advanceTimersByTimeAsync(3000)
    expect(notifyMocks.success).toHaveBeenCalledWith('增强完成')
    expect(apiMocks.listResults.mock.calls.length).toBeGreaterThan(resultsBefore)
    const afterFinish = topicStatusCalls().length
    await vi.advanceTimersByTimeAsync(9000)
    expect(topicStatusCalls().length).toBe(afterFinish) // 轮询已按终态停止

    wrapper.unmount()
  })

  it('无 topic（板块无活跃泳道）：loadAll / topic sync 均不触发，不启动 topic 轮询', async () => {
    listBoardTopics.mockResolvedValue({ success: true, data: { topics: [] } })
    const wrapper = mountPanel()
    // 等 loadTopics 已发出（无 topic 时 bootstrap 无「最后一步 sync」可等）
    await vi.waitFor(() => expect(listBoardTopics).toHaveBeenCalledWith(7701))

    expect(apiMocks.listContexts).not.toHaveBeenCalled()
    expect(topicStatusCalls().length).toBe(0)
    const calls = topicStatusCalls().length
    await vi.advanceTimersByTimeAsync(9000)
    expect(topicStatusCalls().length).toBe(calls) // 无 timer

    wrapper.unmount()
  })
})
