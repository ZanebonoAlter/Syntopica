/**
 * BoardEnrichmentPanel — 工作台信息架构收口（fix-board-analysis-material tasks 3.1）.
 *
 * Covers board-level-analysis spec scenarios:
 *  - 单一下拉选择泳道：泳道选择收敛于聚焦分析区，顶部旧话题选择条不再存在
 *  - 新闻背景入口保留：折叠 section 形态（非单 tab 栏），展开后周期筛选器可用
 *
 * board-level-deep-analysis 5.5：
 *  - 主视图默认是版块简报（BoardBriefReport），触发按钮为「生成简报」，
 *    不再出现自动论文分析文案（跨泳道命题论证/论文式长文）
 *  - legacy 结果 → 旧版分析标注 + BoardAnalysisReport
 *  - 历史下拉带 kind 标签（简报/调查/旧版分析）；在跑任务按 job_kind 区分文案
 *
 * board-level-deep-analysis 5.6/5.7/5.8：
 *  - investigation 结果 → BoardInvestigationReport（三 kind 路由）
 *  - investigate 事件 → triggerBoardInvestigation 接线（绝不自动触发）
 *  - lane 下钻：校验泳道属于当前 topics（幽灵 lane notify 不误选）；
 *    prefill 写入可编辑 textarea，用户可修改后再触发（payload 透传）
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { ref, nextTick } from 'vue'
import BoardEnrichmentPanel from './BoardEnrichmentPanel.vue'
import type { BoardAnalysisResultRow } from '~/api/boardEnrichment'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

const topics = ref<{ id: number; label: string; status: string }[]>([
  { id: 101, label: '伊朗局势', status: 'active' },
  { id: 102, label: '美联储政策', status: 'active' },
])
const selectedTopicId = ref<number | null>(101)
const loadTopics = vi.fn().mockResolvedValue(undefined)
const loadDataSources = vi.fn().mockResolvedValue(undefined)
const loadBoardAnalysisResults = vi.fn().mockResolvedValue(undefined)
const loadAllTopicTables = vi.fn().mockResolvedValue(undefined)
const syncTopicAnalysisStatus = vi.fn().mockResolvedValue(undefined)
const stopBoardPoll = vi.fn()
const activateBoardContext = vi.fn()
const triggerBoardInvestigation = vi.fn().mockResolvedValue(true)
const triggerEnrichmentMock = vi.fn().mockResolvedValue(true)

// notify 需可断言（幽灵 lane 提示）：hoist 捕获
const notifyMocks = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warn: vi.fn(),
}))

// board-level analysis：mock 为可写 ref，测试里直接驱动视图分派
const boardResults = ref<BoardAnalysisResultRow[]>([])
const selectedBoardResult = ref<BoardAnalysisResultRow | null>(null)
const activeBoardJob = ref<{ jobId: string; jobKind: string } | null>(null)
const boardAnalysisTriggering = ref(false)
// board qa（6.2：独立 state，mock 为可写 ref + 可断言 fn）
const boardQaList = ref<unknown[]>([])
const loadBoardQA = vi.fn().mockResolvedValue(true)
const askBoardQuestion = vi.fn().mockResolvedValue(true)
const sedimentBoardAnswer = vi.fn().mockResolvedValue(true)
// topic 档最新 result id（聚焦区 QAPanel 的 v-if 门）
const latestResultId = ref<number | null>(null)
// topic 档 QA 事件 fn（6.2 串台防护断言用）
const askQuestion = vi.fn().mockResolvedValue(true)
const sedimentAnswer = vi.fn().mockResolvedValue(true)
// topic 档 QA 列表（串台防护：board QA 行不得串入）
const qaList = ref<unknown[]>([])

vi.mock('~/features/tags/composables/useBoardEnrichment', () => ({
  useBoardEnrichment: () => ({
    // topic selector
    topics, topicsLoading: ref(false), selectedTopicId, loadTopics,
    // table 1
    contexts: ref([]), contextsLoading: ref(false), regenerating: ref(null),
    saveContext: vi.fn(), regenerateContext: vi.fn(),
    // table 2
    results: ref([]), triggering: ref(false), triggerEnrichment: triggerEnrichmentMock,
    latestResultId, latestResultDetail: ref(null), latestResultDetailLoading: ref(false),
    // table 3
    reviews: ref([]),
    // data sources
    dataSources: ref([]), loadDataSources, saveDataSource: vi.fn(), removeDataSource: vi.fn(),
    // stock debates
    debates: ref([]), debateTriggering: ref(false), debateError: ref(''), debateStage: ref(''),
    loadDebates: vi.fn(), triggerDebate: vi.fn(),
    // qa
    qaList, qaLoading: ref(false), qaError: ref(''), latestAnswer: ref(null),
    loadQA: vi.fn(), askQuestion, sedimentAnswer,
    // board qa（6.2：独立 state，mock 为可写 ref 驱动渲染）
    boardQaList, boardQaLoading: ref(false), boardQaError: ref(''), boardLatestAnswer: ref(null),
    boardQaResultId: ref<number | null>(null),
    loadBoardQA, askBoardQuestion, sedimentBoardAnswer,
    // board-level analysis
    boardResults, boardResultsLoading: ref(false),
    selectedBoardResult, latestBoardBrief: ref(null),
    selectBoardResult: vi.fn(), boardAnalysisTriggering, activeBoardJob,
    triggerBoardAnalysis: vi.fn(), triggerBoardInvestigation, loadBoardAnalysisResults, loadAllTopicTables,
    syncBoardAnalysisStatus: vi.fn().mockResolvedValue(undefined),
    syncTopicAnalysisStatus,
    stopBoardPoll,
    activateBoardContext,
    // workbench UI（周期筛选器）
    selectedGran: ref('week'), selectedPeriodIdx: ref(0), periodList: ref<string[]>([]),
    currentContext: ref(null), setGran: vi.fn(), shiftPeriod: vi.fn(), selectPeriod: vi.fn(),
  }),
}))
vi.mock('~/features/tags/composables/useBoardRelations', () => ({
  useBoardRelations: () => ({
    relations: ref([]),
    relationsLoading: ref(false),
    relationsError: ref(null),
    loadRelations: vi.fn(async () => true),
    relationDetail: ref(null),
    relationDetailLoading: ref(false),
    loadRelationDetail: vi.fn(async () => true),
    triggeringSource: ref(null),
    triggerDiscovery: vi.fn(async () => true),
    confirmingRelationId: ref(null),
    dismissingRelationId: ref(null),
    reResolvingRelationId: ref(null),
    confirmRelation: vi.fn(async () => true),
    dismissRelation: vi.fn(async () => true),
    reResolveRelation: vi.fn(async () => true),
    resetRelationView: vi.fn(),
    disposeRelationView: vi.fn(),
  }),
}))

vi.mock('~/composables/useNotify', () => ({
  useNotify: () => notifyMocks,
}))
vi.mock('~/utils/markdown', () => ({
  renderMarkdown: (s: string) => s,
}))

const stubs = {
  CausalAnalysisReport: { name: 'CausalAnalysisReport', template: '<div class="causal-stub" />' },
  BoardAnalysisReport: { name: 'BoardAnalysisReport', props: ['result', 'loading'], template: '<div class="board-report-stub" />' },
  BoardBriefReport: { name: 'BoardBriefReport', props: ['result', 'loading', 'investigationRunning'], template: '<div class="brief-stub" />' },
  BoardInvestigationReport: { name: 'BoardInvestigationReport', props: ['result', 'loading'], template: '<div class="investigation-stub" />' },
  QAPanel: {
    name: 'QAPanel',
    props: ['resultId', 'qaList', 'qaLoading', 'qaError', 'latestAnswer'],
    emits: ['ask', 'sediment', 'load'],
    template: `<div class="qa-stub" :data-result-id="resultId ?? null" :data-qa-count="qaList?.length ?? 0">
      <button class="qa-ask-stub" @click="$emit('ask', '追问问题')" />
      <button class="qa-sediment-stub" @click="$emit('sediment', 5)" />
      <button class="qa-load-stub" @click="$emit('load', resultId)" />
    </div>`,
  },
  DebateSection: { name: 'DebateSection', template: '<div class="debate-stub" />' },
  AppDialog: { name: 'AppDialog', props: ['modelValue', 'title'], template: '<div class="dialog-stub" />' },
  AppButton: { name: 'AppButton', template: '<button class="app-btn-stub"><slot /></button>' },
  AppToggle: { name: 'AppToggle', template: '<div class="toggle-stub" />' },
}

function makeRow(id: number, kind: string): BoardAnalysisResultRow {
  return { id, analysis_scope: 'board', result_kind: kind, sectors: null, created_at: '2026-09-01T10:00:00Z' }
}

function mountPanel(): VueWrapper {
  return mount(BoardEnrichmentPanel, { props: { boardId: 7701 }, global: { stubs } })
}

beforeEach(() => {
  vi.clearAllMocks()
  // 重置模块级可变 ref（185 会把 selectedTopicId 置 null 验证无 topic 分
  // 支，不重置会泄漏到后续测试——测试间不得顺序耦合）
  selectedTopicId.value = 101
  boardResults.value = []
  selectedBoardResult.value = null
  activeBoardJob.value = null
  boardAnalysisTriggering.value = false
  boardQaList.value = []
  qaList.value = []
  latestResultId.value = null
})
describe('BoardEnrichmentPanel 工作台收口', () => {
  it('单一下拉选择泳道：泳道下拉仅聚焦分析区一处，旧话题选择条不存在', async () => {
    const wrapper = mountPanel()
    // 聚焦分析区先展开（唯一泳道选择点在折叠区内）
    const focusToggle = wrapper.findAll('.focus-toggle').find(b => b.text().includes('聚焦分析'))
    expect(focusToggle).toBeDefined()
    await focusToggle!.trigger('click')

    // 泳道选择下拉（placeholder=选择泳道…）全页面仅一处
    const laneSelects = wrapper.findAll('select').filter(s => s.text().includes('选择泳道…'))
    expect(laneSelects.length).toBe(1)

    // 旧顶栏（话题选择条）不复存在：无 toolbar 容器、无「选择话题…」placeholder
    expect(wrapper.find('.ew-toolbar').exists()).toBe(false)
    const legacySelects = wrapper.findAll('select').filter(s => s.text().includes('选择话题…'))
    expect(legacySelects.length).toBe(0)
    wrapper.unmount()
  })

  it('新闻背景入口保留：折叠 section 形态（非单 tab 栏），展开后周期筛选器可用', async () => {
    const wrapper = mountPanel()
    // 无 subtabs 导航容器（单 tab 栏已移除）
    expect(wrapper.find('.subtabs').exists()).toBe(false)
    expect(wrapper.find('.subtab').exists()).toBe(false)

    // 新闻背景折叠 toggle 存在，默认收起
    const newsToggle = wrapper.findAll('.focus-toggle').find(b => b.text().includes('新闻背景'))
    expect(newsToggle).toBeDefined()
    expect(wrapper.find('.period-picker').exists()).toBe(false)

    // 展开后周期筛选器（粒度切换）可达
    await newsToggle!.trigger('click')
    expect(wrapper.find('.period-picker').exists()).toBe(true)
    expect(wrapper.find('.gran-select').exists()).toBe(true)
    wrapper.unmount()
  })

  it('刷新入口位于版块简报区头部', () => {
    const wrapper = mountPanel()
    const boardHead = wrapper.find('.board-head')
    expect(boardHead.exists()).toBe(true)
    const refreshBtn = boardHead.find('button[title*="刷新"]')
    expect(refreshBtn.exists()).toBe(true)
    wrapper.unmount()
  })

  it('bootstrap 在挂载时拉取板块数据', () => {
    const wrapper = mountPanel()
    expect(loadTopics).toHaveBeenCalledWith(7701)
    expect(loadBoardAnalysisResults).toHaveBeenCalledWith(7701)
    wrapper.unmount()
  })

  it('bootstrap：挂载与切板块都先激活 board 视图上下文（activateBoardContext(boardId) 先于加载，内部停旧轮询+epoch++）', async () => {
    const wrapper = mountPanel()
    expect(activateBoardContext).toHaveBeenCalledTimes(1) // 挂载 bootstrap 最前激活
    expect(activateBoardContext).toHaveBeenCalledWith(7701)
    const actOrder = activateBoardContext.mock.invocationCallOrder[0] ?? 0
    const loadOrder = loadTopics.mock.invocationCallOrder[0] ?? 0
    expect(actOrder).toBeLessThan(loadOrder)

    await wrapper.setProps({ boardId: 8802 })
    expect(activateBoardContext).toHaveBeenCalledTimes(2) // 切板块再激活一次（旧 epoch 全部失效）
    expect(activateBoardContext).toHaveBeenLastCalledWith(8802)
    const actOrder2 = activateBoardContext.mock.invocationCallOrder[1] ?? 0
    const loadOrder2 = loadTopics.mock.invocationCallOrder[1] ?? 0
    expect(actOrder2).toBeLessThan(loadOrder2)
    expect(loadTopics).toHaveBeenLastCalledWith(8802)
    wrapper.unmount()
  })

  it('bootstrap：loadTopics 确定 selectedTopicId 后对当前 topic 调 syncTopicAnalysisStatus（重进恢复接线）；无 topic 不调', async () => {
    const wrapper = mountPanel()
    // bootstrap 链同步步序已重排（sync 在 await boardLoads 之后），纯微任务
    // 链比一个 nextTick 深——用宏任务屏障确定性排空（不拍次数：mock 均
    // 立即 resolve，一次宏任务边界足以跑完整条 bootstrap 微任务链）
    await new Promise(r => setTimeout(r, 0))
    // 挂载：loadTopics 后 selectedTopicId=101 → 对当前 topic 恢复任务状态
    expect(syncTopicAnalysisStatus).toHaveBeenCalledTimes(1)
    expect(syncTopicAnalysisStatus).toHaveBeenCalledWith(101)
    const loadOrder = loadTopics.mock.invocationCallOrder[0] ?? 0
    const syncOrder = syncTopicAnalysisStatus.mock.invocationCallOrder[0] ?? 0
    expect(syncOrder).toBeGreaterThan(loadOrder) // 先确定 topic 再 sync

    // 切板块且无选中 topic：不误调 sync、不调 loadAll（无 topic 二者都不调）
    selectedTopicId.value = null
    await wrapper.setProps({ boardId: 8802 })
    await new Promise(r => setTimeout(r, 0)) // flush bootstrap 链（loadTopics resolve 后才判 sync）
    expect(syncTopicAnalysisStatus).toHaveBeenCalledTimes(1)
    expect(loadAllTopicTables).toHaveBeenCalledTimes(1) // 第二次 bootstrap 无 topic：loadAll 也不调
    expect(loadTopics).toHaveBeenLastCalledWith(8802)
    wrapper.unmount()
  })

  it('bootstrap 顺序契约（Critical 修复）：loadAllTopicTables 先于 syncTopicAnalysisStatus，sync 为 topic 维度最后一步（其后无 stopTopicPoll 路径）', async () => {
    const wrapper = mountPanel()
    await new Promise(r => setTimeout(r, 0)) // flush bootstrap 链
    expect(loadAllTopicTables).toHaveBeenCalledWith(101)
    expect(syncTopicAnalysisStatus).toHaveBeenCalledWith(101)
    const loadAllOrder = loadAllTopicTables.mock.invocationCallOrder[0] ?? 0
    const syncOrder = syncTopicAnalysisStatus.mock.invocationCallOrder[0] ?? 0
    expect(syncOrder).toBeGreaterThan(loadAllOrder) // loadAll 先、sync 最后（反序会误杀恢复轮询）
    // selectedTopicId 未变化（watch 不触发）→ 本次 bootstrap 各恰好一次
    expect(loadAllTopicTables).toHaveBeenCalledTimes(1)
    expect(syncTopicAnalysisStatus).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('bootstrap：切板块后同 topic 语义保持——新 bootstrap 自身重新 loadAll + sync（同 topic 刷新也加载表+恢复，不依赖 watch）', async () => {
    const wrapper = mountPanel()
    await new Promise(r => setTimeout(r, 0))
    expect(loadAllTopicTables).toHaveBeenCalledTimes(1)
    expect(syncTopicAnalysisStatus).toHaveBeenCalledTimes(1)

    // selectedTopicId 保持 101（watch 不触发）→ 第二次 bootstrap（切板块与头部刷新按钮同路径）自身完成 loadAll + sync
    await wrapper.setProps({ boardId: 8802 })
    await new Promise(r => setTimeout(r, 0))
    expect(loadTopics).toHaveBeenLastCalledWith(8802)
    expect(loadAllTopicTables).toHaveBeenCalledTimes(2)
    expect(loadAllTopicTables).toHaveBeenLastCalledWith(101)
    expect(syncTopicAnalysisStatus).toHaveBeenCalledTimes(2)
    expect(syncTopicAnalysisStatus).toHaveBeenLastCalledWith(101)
    const loadAllOrder2 = loadAllTopicTables.mock.invocationCallOrder[1] ?? 0
    const syncOrder2 = syncTopicAnalysisStatus.mock.invocationCallOrder[1] ?? 0
    expect(syncOrder2).toBeGreaterThan(loadAllOrder2)
    wrapper.unmount()
  })

  it('bootstrap：切板块 A→B loadTopics 失败（失败语义 selectedTopicId=null）——不调旧 topic loadAll/sync，无 topic 轮询接线', async () => {
    const wrapper = mountPanel()
    await new Promise(r => setTimeout(r, 0))
    expect(loadAllTopicTables).toHaveBeenCalledTimes(1) // 挂载正常路径：101
    expect(syncTopicAnalysisStatus).toHaveBeenCalledTimes(1)

    // 模拟 B 的 loadTopics 失败：composable 失败语义把 selectedTopicId 置 null（topics=[]）
    loadTopics.mockImplementationOnce(async () => { selectedTopicId.value = null })
    await wrapper.setProps({ boardId: 8802 })
    await new Promise(r => setTimeout(r, 0)) // flush bootstrap 链（loadTopics resolve 后才判 loadAll/sync）
    expect(loadTopics).toHaveBeenLastCalledWith(8802)
    expect(loadAllTopicTables).toHaveBeenCalledTimes(1) // 旧 topic(101) 的 loadAll 不调
    expect(syncTopicAnalysisStatus).toHaveBeenCalledTimes(1) // 旧 topic 的 sync 不调（无 topic 轮询接线）
    wrapper.unmount()
  })

  it('历史下拉带 aria-label="历史报告"', async () => {
    boardResults.value = [makeRow(11, 'board_brief'), makeRow(9, 'board_investigation')]
    selectedBoardResult.value = boardResults.value[0] ?? null
    const wrapper = mountPanel()
    await nextTick()
    const select = wrapper.find('select.board-history')
    expect(select.exists()).toBe(true)
    expect(select.attributes('aria-label')).toBe('历史报告')
    wrapper.unmount()
  })
})

describe('BoardEnrichmentPanel 简报主视图（board-level-deep-analysis 5.5）', () => {
  it('主视图默认渲染 BoardBriefReport；触发按钮为「生成简报」，无自动论文分析文案', () => {
    const wrapper = mountPanel()
    expect(wrapper.find('.brief-stub').exists()).toBe(true)
    expect(wrapper.find('.board-report-stub').exists()).toBe(false)
    const btn = wrapper.find('.board-head .btn-primary')
    expect(btn.text()).toContain('生成简报')
    // 旧「分析板块」按钮与论文式文案不再出现
    expect(wrapper.find('.board-head').text()).not.toContain('分析板块')
    expect(wrapper.find('.board-head').text()).not.toContain('论文式长文')
    wrapper.unmount()
  })

  it('legacy 结果选中：渲染「旧版分析」标注 + BoardAnalysisReport', async () => {
    const legacy = makeRow(7, 'legacy_board_analysis')
    boardResults.value = [legacy]
    selectedBoardResult.value = legacy
    const wrapper = mountPanel()
    await nextTick()
    expect(wrapper.find('.bb-legacy-banner').exists()).toBe(true)
    expect(wrapper.find('.bb-legacy-banner').text()).toContain('旧版分析')
    expect(wrapper.find('.board-report-stub').exists()).toBe(true)
    expect(wrapper.find('.brief-stub').exists()).toBe(false)
    wrapper.unmount()
  })

  it('investigation 结果选中：渲染 BoardInvestigationReport，不冒充简报/旧报告', async () => {
    const inv = makeRow(9, 'board_investigation')
    boardResults.value = [inv]
    selectedBoardResult.value = inv
    const wrapper = mountPanel()
    await nextTick()
    expect(wrapper.find('.investigation-stub').exists()).toBe(true)
    expect(wrapper.find('.brief-stub').exists()).toBe(false)
    expect(wrapper.find('.board-report-stub').exists()).toBe(false)
    wrapper.unmount()
  })

  it('legacy 真实 fixture：完整旧 sectors（thesis/argument/depth）透传到 BoardAnalysisReport，横幅「旧版分析」（6.2 只读兼容）', async () => {
    // 真实 legacy 形状：与旧写链落库 payload 同构（D1 五字段）。
    const legacySectors = {
      scope: 'board',
      form: 'board',
      thesis: '存储涨价不是需求反转，而是产能纪律的重新定价',
      angle: '概念重命名',
      candidates: [
        { thesis: '候选甲：需求反转论', hook: '钩子甲', angle: '周期反转' },
        { thesis: '候选乙：产能纪律论', hook: '钩子乙', angle: '供给约束' },
      ],
      chosen_index: 1,
      reason: '候选乙覆盖泳道更全',
      argument: {
        intro: '开篇：两条泳道价格同向上行但订单未放量。',
        layers: [
          { layer: '表层现象', deep_logic: '价格上行由供给收缩驱动。', basis: '态势卡#1' },
          { layer: '传导机制', deep_logic: '原厂减产传导至现货升水。', basis: 'agent 检索' },
        ],
        boundary: '还不能确认传导是否已闭环。',
        conclusion: { cert: 'medium', judgment: '错位仍在扩大。' },
      },
      depth: {
        system_reframe: '放进全球产能周期系统讲。',
        mechanism_layers: [{ layer: '产能纪律', deep_logic: '寡头协同减产。', basis: '季度报告' }],
        historical_analogy: { case: '2019 下行周期', mechanism: '同类协同', diff: '本次更快' },
        boundary: '数据未覆盖衍生品。',
        evidence_chain: [],
      },
      lane_refs: [{ lane_id: 901, note: '主观察' }],
    }
    const legacy: BoardAnalysisResultRow = {
      id: 7, analysis_scope: 'board', result_kind: 'legacy_board_analysis',
      sectors: legacySectors as unknown as BoardAnalysisResultRow['sectors'],
      tool_calls: [{ tool: 'web_search', args: { query: '产能 明细' }, result_preview: '命中3条' }],
      session_id: 'data_enrichment_board_9_ab12cd34',
      created_at: '2026-08-26T10:00:00Z',
    }
    boardResults.value = [legacy]
    selectedBoardResult.value = legacy
    const wrapper = mountPanel()
    await nextTick()
    // 横幅标注旧版分析
    expect(wrapper.find('.bb-legacy-banner').text()).toContain('旧版分析')
    // 完整 result 原样交给旧组件：sectors 五字段 + tool_calls 不丢
    const report = wrapper.findComponent({ name: 'BoardAnalysisReport' })
    expect(report.exists()).toBe(true)
    expect(report.props('result')).toMatchObject({ id: 7, result_kind: 'legacy_board_analysis' })
    expect((report.props('result') as BoardAnalysisResultRow).sectors).toEqual(legacySectors)
    expect((report.props('result') as BoardAnalysisResultRow).tool_calls).toEqual(legacy.tool_calls)
    // 新 kind 组件不出现
    expect(wrapper.find('.brief-stub').exists()).toBe(false)
    expect(wrapper.find('.investigation-stub').exists()).toBe(false)
    wrapper.unmount()
  })

  it('brief 选中（真实 sectors）：渲染 BoardBriefReport，不进旧组件、无 legacy 横幅', async () => {
    const brief: BoardAnalysisResultRow = {
      id: 12, analysis_scope: 'board', result_kind: 'board_brief',
      sectors: { result_kind: 'board_brief', summary: '两条泳道各有进展', observations: [], relationships: [], uncertainties: [], research_questions: [], lane_refs: [] } as unknown as BoardAnalysisResultRow['sectors'],
      created_at: '2026-09-01T10:00:00Z',
    }
    boardResults.value = [brief]
    selectedBoardResult.value = brief
    const wrapper = mountPanel()
    await nextTick()
    expect(wrapper.find('.brief-stub').exists()).toBe(true)
    expect(wrapper.find('.bb-legacy-banner').exists()).toBe(false)
    expect(wrapper.find('.board-report-stub').exists()).toBe(false)
    expect(wrapper.find('.investigation-stub').exists()).toBe(false)
    wrapper.unmount()
  })

  it('历史下拉带 kind 标签（简报/调查/旧版分析）', async () => {
    boardResults.value = [makeRow(11, 'board_brief'), makeRow(9, 'board_investigation'), makeRow(7, 'legacy_board_analysis')]
    selectedBoardResult.value = boardResults.value[0] ?? null
    const wrapper = mountPanel()
    await nextTick()
    const select = wrapper.find('.board-history')
    expect(select.exists()).toBe(true)
    const text = select.text()
    expect(text).toContain('简报')
    expect(text).toContain('调查')
    expect(text).toContain('旧版分析')
    wrapper.unmount()
  })

  it('在跑任务按 job_kind 区分文案：正在调查', async () => {
    boardAnalysisTriggering.value = true
    activeBoardJob.value = { jobId: 'job-1', jobKind: 'board_investigation' }
    const wrapper = mountPanel()
    await nextTick()
    expect(wrapper.find('.bb-job-tag').text()).toContain('正在调查')
    wrapper.unmount()
  })

  it('在跑任务按 job_kind 区分文案：正在生成简报', async () => {
    boardAnalysisTriggering.value = true
    activeBoardJob.value = { jobId: 'job-2', jobKind: 'board_brief' }
    const wrapper = mountPanel()
    await nextTick()
    expect(wrapper.find('.bb-job-tag').text()).toContain('正在生成简报')
    wrapper.unmount()
  })
})

describe('BoardEnrichmentPanel 调查接线与 lane 下钻（board-level-deep-analysis 5.7/5.8）', () => {
  /** 挂载并展开聚焦区（唯一泳道选择点 + prefill textarea 在折叠区内）。 */
  async function mountWithFocusOpen() {
    const wrapper = mountPanel()
    const toggle = wrapper.findAll('.focus-toggle').find(b => b.text().includes('聚焦分析'))
    await toggle!.trigger('click')
    return wrapper
  }

  it('investigate 事件 → triggerBoardInvestigation(boardId, payload) 接线', async () => {
    const wrapper = mountPanel()
    await nextTick()
    const brief = wrapper.findComponent({ name: 'BoardBriefReport' })
    expect(brief.exists()).toBe(true)
    brief.vm.$emit('investigate', { briefing_result_id: 11, question_id: 'q1', question: '现货放量是需求驱动还是补库存？' })
    await nextTick()
    expect(triggerBoardInvestigation).toHaveBeenCalledTimes(1)
    expect(triggerBoardInvestigation).toHaveBeenCalledWith(7701, {
      briefing_result_id: 11,
      question_id: 'q1',
      question: '现货放量是需求驱动还是补库存？',
    })
    wrapper.unmount()
  })

  it('investigationRunning 透传：调查在跑时简报组件收到 true', async () => {
    boardAnalysisTriggering.value = true
    activeBoardJob.value = { jobId: 'job-1', jobKind: 'board_investigation' }
    const wrapper = mountPanel()
    await nextTick()
    expect(wrapper.findComponent({ name: 'BoardBriefReport' }).props('investigationRunning')).toBe(true)
    wrapper.unmount()
  })

  it('brief 在跑（非调查）：investigationRunning 不误传 true', async () => {
    boardAnalysisTriggering.value = true
    activeBoardJob.value = { jobId: 'job-2', jobKind: 'board_brief' }
    const wrapper = mountPanel()
    await nextTick()
    expect(wrapper.findComponent({ name: 'BoardBriefReport' }).props('investigationRunning')).toBe(false)
    wrapper.unmount()
  })

  it('简报 observation lane 下钻：选中泳道 + 展开聚焦区 + prefill 写入可编辑 textarea（不自动触发）', async () => {
    const wrapper = await mountWithFocusOpen()
    const brief = wrapper.findComponent({ name: 'BoardBriefReport' })
    brief.vm.$emit('drill-lane', { laneId: 102, prefill: '现货侧成交量连续两周放大' })
    await nextTick()
    // 选中对应泳道
    expect(selectedTopicId.value).toBe(102)
    // prefill 写入 textarea（可编辑）
    const lensInput = wrapper.find('.focus-lens-input')
    expect((lensInput.element as HTMLTextAreaElement).value).toBe('现货侧成交量连续两周放大')
    // 下钻不自动触发聚焦分析（用户自己点按钮才发）
    expect(triggerEnrichmentMock).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('investigation evidence lane 下钻：prefill（具体问题+证据说明）写入 textarea，用户修改后触发携改后 lens', async () => {
    vi.stubGlobal('confirm', () => true)
    try {
      const wrapper = await mountWithFocusOpen()
      // 驱动路由：选中一份 investigation 结果
      const inv = makeRow(9, 'board_investigation')
      boardResults.value = [inv]
      selectedBoardResult.value = inv
      await nextTick()
      const invComp = wrapper.findComponent({ name: 'BoardInvestigationReport' })
      expect(invComp.exists()).toBe(true)
      const prefill = '两条泳道是否由同一资金驱动？\n该证据是假设「产业基金同步注入」的反证\n证据摘录：「基金公告原文摘录ABC」'
      invComp.vm.$emit('drill-lane', { laneId: 101, prefill })
      await nextTick()
      const lensInput = wrapper.find('.focus-lens-input')
      expect((lensInput.element as HTMLTextAreaElement).value).toBe(prefill)
      // 用户修改后再触发：payload 携改后的 lens（后端 prefill_lens 精确透传）
      await lensInput.setValue('改后的调查问题：资金同源吗？')
      await wrapper.find('.focus-body .btn-primary').trigger('click')
      await nextTick()
      expect(triggerEnrichmentMock).toHaveBeenCalledWith(101, '改后的调查问题：资金同源吗？')
      wrapper.unmount()
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('幽灵 lane（不在 topics）：notify 提示、不误选/不展开聚焦区', async () => {
    const wrapper = mountPanel()
    await nextTick()
    const brief = wrapper.findComponent({ name: 'BoardBriefReport' })
    brief.vm.$emit('drill-lane', { laneId: 9999, prefill: '幽灵泳道的预填' })
    await nextTick()
    expect(notifyMocks.warn).toHaveBeenCalledTimes(1)
    expect(notifyMocks.warn.mock.calls[0]![0]).toContain('9999')
    // 不误选（保持 101）、不展开、不写入 prefill
    expect(selectedTopicId.value).toBe(101)
    expect(wrapper.find('.focus-body').exists()).toBe(false)
    wrapper.unmount()
  })
})

// ── board QAPanel（6.2：版块报告追问接线，三 kind 均挂）──────────────────
describe('BoardEnrichmentPanel — 版块报告 QAPanel（board-level-deep-analysis 6.2）', () => {
  it('三 kind（brief/legacy/investigation）选中时都渲染 QAPanel，resultId 挂当前报告', async () => {
    for (const kind of ['board_brief', 'legacy_board_analysis', 'board_investigation'] as const) {
      boardResults.value = [makeRow(42, kind)]
      selectedBoardResult.value = boardResults.value[0] ?? null
      const wrapper = mountPanel()
      await nextTick()
      const panel = wrapper.find('.qa-stub.board-qa')
      expect(panel.exists(), `kind=${kind} 应渲染版块 QAPanel`).toBe(true)
      expect(panel.attributes('data-result-id')).toBe('42')
      wrapper.unmount()
    }
  })

  it('无版块报告（selectedBoardResult null）不渲染 QAPanel', async () => {
    const wrapper = mountPanel()
    await nextTick()
    expect(wrapper.find('.qa-stub.board-qa').exists()).toBe(false)
    wrapper.unmount()
  })

  it('ask/sediment 事件 → askBoardQuestion/sedimentBoardAnswer（board 路由），load 事件 → loadBoardQA', async () => {
    boardResults.value = [makeRow(42, 'board_brief')]
    selectedBoardResult.value = boardResults.value[0] ?? null
    const wrapper = mountPanel()
    await nextTick()
    const panel = wrapper.find('.qa-stub.board-qa')

    await panel.find('.qa-load-stub').trigger('click')
    await panel.find('.qa-ask-stub').trigger('click')
    await panel.find('.qa-sediment-stub').trigger('click')

    expect(loadBoardQA).toHaveBeenCalledWith(42)
    expect(askBoardQuestion).toHaveBeenCalledWith('追问问题')
    expect(sedimentBoardAnswer).toHaveBeenCalledWith(5)
    wrapper.unmount()
  })

  it('切历史报告：QAPanel resultId 跟随变更（重拉由 QAPanel 内部 watch 驱动）', async () => {
    boardResults.value = [makeRow(42, 'board_brief'), makeRow(77, 'legacy_board_analysis')]
    selectedBoardResult.value = boardResults.value[0] ?? null
    const wrapper = mountPanel()
    await nextTick()
    const panel = wrapper.find('.qa-stub.board-qa')
    expect(panel.attributes('data-result-id')).toBe('42')

    selectedBoardResult.value = boardResults.value[1] ?? null
    await nextTick()
    expect(panel.attributes('data-result-id')).toBe('77')
    wrapper.unmount()
  })

  it('topic 聚焦区 QAPanel 保持原样（topic refs + loadQA/askQuestion/sedimentAnswer），与 board QA 互不串台', async () => {
    boardResults.value = [makeRow(42, 'board_brief')]
    selectedBoardResult.value = boardResults.value[0] ?? null
    boardQaList.value = [{ id: 1 }]
    const wrapper = mountPanel()
    await nextTick()
    // 展开聚焦区并选中 topic → topic QAPanel 渲染（v-if 门 latestResultId）
    latestResultId.value = 55
    wrapper.find('.focus-toggle').trigger('click')
    await nextTick()
    const topicPanel = wrapper.find('.qa-stub:not(.board-qa)')
    expect(topicPanel.exists()).toBe(true)
    expect(topicPanel.attributes('data-qa-count')).toBe('0') // topic qaList 仍空（board 行不串入）

    await topicPanel.find('.qa-ask-stub').trigger('click')
    await topicPanel.find('.qa-sediment-stub').trigger('click')
    expect(askQuestion).toHaveBeenCalledWith('追问问题')
    expect(sedimentAnswer).toHaveBeenCalledWith(5)
    expect(askBoardQuestion).not.toHaveBeenCalled()
    expect(sedimentBoardAnswer).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
