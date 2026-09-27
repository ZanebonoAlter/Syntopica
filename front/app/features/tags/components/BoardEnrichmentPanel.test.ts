/**
 * BoardEnrichmentPanel — 信号解读工作台信息架构（board-signal-reports 5.2 重排）.
 *
 * 主视图 = 候选信号列表 + 已生成报告列表（发现 → 逐条深入研究 → 直接阅读，
 * 无评审）；报告阅读走 SignalReportView（reader≤760）。旧简报/调查/旧版分析
 * 视图与数据源绑定管理入口从工作台退役（FE-11：不在 DOM）；聚焦分析折叠区
 * （唯一泳道选择点）与新闻背景折叠区（周期筛选 + 叙事内联编辑）保留。
 *
 * 断言方式：事件经 sig mock 的 vi.fn 捕获（本仓库 vitest 环境下
 * wrapper.emitted() 不记录自定义事件——同 FeedDetailEditor.test.ts 既有红）。
 * 子组件用 vi.mock 模块级替换（稳定，不依赖 stub 名称匹配）。
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { ref, nextTick } from 'vue'
import BoardEnrichmentPanel from './BoardEnrichmentPanel.vue'

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
const syncBoardAnalysisStatus = vi.fn().mockResolvedValue(undefined)
const activateBoardContext = vi.fn()
const triggerEnrichmentMock = vi.fn().mockResolvedValue(true)

// notify 需可断言：hoist 捕获
const notifyMocks = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warn: vi.fn(),
}))

// topic 档 QA（聚焦区保留）
const latestResultId = ref<number | null>(null)
const qaList = ref<unknown[]>([])
const askQuestion = vi.fn().mockResolvedValue(true)
const sedimentAnswer = vi.fn().mockResolvedValue(true)

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
    // data sources（绑定管理退役，bootstrap 保留拉取）
    loadDataSources,
    // stock debates
    debates: ref([]), debateTriggering: ref(false), debateError: ref(''), debateStage: ref(''),
    loadDebates: vi.fn(), triggerDebate: vi.fn(),
    // qa（聚焦区）
    qaList, qaLoading: ref(false), qaError: ref(''), latestAnswer: ref(null),
    loadQA: vi.fn(), askQuestion, sedimentAnswer,
    // board-level analysis（旧视图退役，bootstrap 保留加载/同步）
    loadBoardAnalysisResults, syncBoardAnalysisStatus, syncTopicAnalysisStatus, activateBoardContext,
    // workbench UI（新闻背景周期筛选器）
    selectedGran: ref('week'), selectedPeriodIdx: ref(0), periodList: ref<string[]>([]),
    currentContext: ref(null), setGran: vi.fn(), shiftPeriod: vi.fn(), selectPeriod: vi.fn(),
    // misc
    loadAllTopicTables,
  }),
}))

// ── 信号工作台 composable mock：可写 refs 驱动视图 + 可断言 spies ─────────
const signal = {
  granularity: ref<'month' | 'year'>('month'),
  period: ref('2026-09'),
  candidates: ref<unknown[]>([]),
  candidatesLoading: ref(false),
  candidatesError: ref<string | null>(null),
  reports: ref<unknown[]>([]),
  reportsLoading: ref(false),
  reportsError: ref<string | null>(null),
  discoveryRunning: ref(false),
  discoveryPhase: ref<string | null>(null),
  discoveryError: ref<string | null>(null),
  researchCandidateId: ref<number | null>(null),
  researchPhase: ref<string | null>(null),
  researchError: ref<string | null>(null),
  researchProgressLabel: ref('正在研究 · 决策上限 40 轮 · 可离开页面'),
  activeReportId: ref<number | null>(null),
  activeReport: ref<object | null>(null),
  activeReportLoading: ref(false),
  activeReportError: ref<string | null>(null),
  setBoard: vi.fn(),
  loadCandidates: vi.fn().mockResolvedValue(undefined),
  loadReports: vi.fn().mockResolvedValue(undefined),
  loadPeriod: vi.fn().mockResolvedValue(undefined),
  changePeriod: vi.fn().mockResolvedValue(true),
  discover: vi.fn().mockResolvedValue(true),
  research: vi.fn().mockResolvedValue(true),
  openReport: vi.fn().mockResolvedValue(undefined),
  closeReport: vi.fn(),
}

vi.mock('~/features/tags/composables/useSignalWorkbench', () => ({
  useSignalWorkbench: () => signal,
}))

// 信号子组件：模块级替换（stub 名称匹配在本环境不可靠）
vi.mock('./SignalCandidateList.vue', () => ({
  default: {
    name: 'SignalCandidateList',
    props: ['candidates', 'loading', 'error', 'granularity', 'period', 'discoveryRunning', 'discoveryPhase', 'discoveryError', 'researchCandidateId', 'researchPhase', 'researchError', 'researchProgressLabel'],
    emits: ['commit', 'discover', 'research', 'open-report'],
    template: `<div class="signal-candidates-stub">
      <button class="sc-commit" @click="$emit('commit', { granularity: 'month', period: '2026-08' })" />
      <button class="sc-discover" @click="$emit('discover', { granularity: 'month', period: '2026-09' })" />
      <button class="sc-research" @click="$emit('research', 31, { regenerate: false })" />
      <button class="sc-regenerate" @click="$emit('research', 31, { regenerate: true })" />
      <button class="sc-open" @click="$emit('open-report', 501)" />
    </div>`,
  },
}))
vi.mock('./SignalReportList.vue', () => ({
  default: {
    name: 'SignalReportList',
    props: ['reports', 'loading', 'error'],
    emits: ['open'],
    template: '<div class="signal-reports-stub"><button class="sr-open" @click="$emit(\'open\', 501)" /></div>',
  },
}))
vi.mock('./SignalReportView.vue', () => ({
  default: {
    name: 'SignalReportView',
    props: ['report', 'loading', 'error', 'regenerating', 'regeneratePhase'],
    emits: ['back', 'regenerate'],
    template: `<div class="signal-report-view-stub">
      <button class="srv-back" @click="$emit('back')" />
      <button class="srv-regen" @click="$emit('regenerate', 31)" />
    </div>`,
  },
}))
vi.mock('~/composables/useNotify', () => ({
  useNotify: () => notifyMocks,
}))
vi.mock('~/utils/markdown', () => ({
  renderMarkdown: (s: string) => s,
}))

const stubs = {
  CausalAnalysisReport: { name: 'CausalAnalysisReport', template: '<div class="causal-stub" />' },
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

function mountPanel(): VueWrapper {
  return mount(BoardEnrichmentPanel, { props: { boardId: 7701 }, global: { stubs } })
}

beforeEach(() => {
  vi.clearAllMocks()
  // 重置模块级可变 ref（测试间不得顺序耦合）
  selectedTopicId.value = 101
  signal.granularity.value = 'month'
  signal.period.value = '2026-09'
  signal.candidates.value = []
  signal.reports.value = []
  signal.discoveryRunning.value = false
  signal.researchCandidateId.value = null
  signal.activeReportId.value = null
  signal.activeReport.value = null
})

describe('BoardEnrichmentPanel 工作台收口（信号解读主视图）', () => {
  it('主视图 = 候选信号列表 + 已生成报告列表（信号工作台头部：刷新入口）', () => {
    const wrapper = mountPanel()
    expect(wrapper.find('.signal-candidates-stub').exists()).toBe(true)
    expect(wrapper.find('.signal-reports-stub').exists()).toBe(true)
    const boardHead = wrapper.find('.board-head')
    expect(boardHead.exists()).toBe(true)
    expect(boardHead.text()).toContain('信号解读')
    const refreshBtn = boardHead.find('button[title*="刷新"]')
    expect(refreshBtn.exists()).toBe(true)
    wrapper.unmount()
  })

  it('FE-11：旧产出视图不在 DOM——无简报/调查/旧版分析/跨版块关系/版块报告追问挂载', () => {
    const wrapper = mountPanel()
    expect(wrapper.find('.brief-stub').exists()).toBe(false)
    expect(wrapper.find('.investigation-stub').exists()).toBe(false)
    expect(wrapper.find('.board-report-stub').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('生成简报')
    expect(wrapper.text()).not.toContain('旧版分析')
    // 绑定管理入口退役：无数据源 chips / 绑定按钮 / 绑定弹窗
    expect(wrapper.find('.src-chip').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('+ 绑定')
    expect(wrapper.text()).not.toContain('绑定数据源')
    wrapper.unmount()
  })

  it('FE-11：新闻背景折叠区保留——展开后周期筛选器与粒度切换可用', async () => {
    const wrapper = mountPanel()
    const newsToggle = wrapper.findAll('.focus-toggle').find(b => b.text().includes('新闻背景'))
    expect(newsToggle).toBeDefined()
    expect(wrapper.find('.period-picker').exists()).toBe(false)
    await newsToggle!.trigger('click')
    expect(wrapper.find('.period-picker').exists()).toBe(true)
    expect(wrapper.find('.gran-select').exists()).toBe(true)
    wrapper.unmount()
  })

  it('聚焦分析折叠区保留：泳道下拉唯一（新闻背景的泳道选择点）', async () => {
    const wrapper = mountPanel()
    const focusToggle = wrapper.findAll('.focus-toggle').find(b => b.text().includes('聚焦分析'))
    expect(focusToggle).toBeDefined()
    await focusToggle!.trigger('click')
    const laneSelects = wrapper.findAll('select').filter(s => s.text().includes('选择泳道…'))
    expect(laneSelects.length).toBe(1)
    wrapper.unmount()
  })

  it('bootstrap 在挂载时拉取板块数据（含信号候选/报告列表）', () => {
    const wrapper = mountPanel()
    expect(loadTopics).toHaveBeenCalledWith(7701)
    expect(loadBoardAnalysisResults).toHaveBeenCalledWith(7701)
    expect(signal.setBoard).toHaveBeenCalledWith(7701)
    expect(signal.loadPeriod).toHaveBeenCalledWith(7701)
    wrapper.unmount()
  })

  it('bootstrap：挂载与切板块都先激活 board 视图上下文（activateBoardContext(boardId) 先于加载）', async () => {
    const wrapper = mountPanel()
    expect(activateBoardContext).toHaveBeenCalledTimes(1)
    expect(activateBoardContext).toHaveBeenCalledWith(7701)
    const actOrder = activateBoardContext.mock.invocationCallOrder[0] ?? 0
    const loadOrder = loadTopics.mock.invocationCallOrder[0] ?? 0
    expect(actOrder).toBeLessThan(loadOrder)

    await wrapper.setProps({ boardId: 8802 })
    expect(activateBoardContext).toHaveBeenCalledTimes(2)
    expect(loadTopics).toHaveBeenLastCalledWith(8802)
    // 切板块：信号视图重置 + 新板块列表加载
    expect(signal.setBoard).toHaveBeenLastCalledWith(8802)
    expect(signal.loadPeriod).toHaveBeenLastCalledWith(8802)
    wrapper.unmount()
  })

  it('bootstrap：loadTopics 确定 selectedTopicId 后对当前 topic 调 syncTopicAnalysisStatus（重进恢复接线）；无 topic 不调', async () => {
    const wrapper = mountPanel()
    await new Promise(r => setTimeout(r, 0))
    expect(syncTopicAnalysisStatus).toHaveBeenCalledTimes(1)
    expect(syncTopicAnalysisStatus).toHaveBeenCalledWith(101)
    const loadOrder = loadTopics.mock.invocationCallOrder[0] ?? 0
    const syncOrder = syncTopicAnalysisStatus.mock.invocationCallOrder[0] ?? 0
    expect(syncOrder).toBeGreaterThan(loadOrder)

    selectedTopicId.value = null
    await wrapper.setProps({ boardId: 8802 })
    await new Promise(r => setTimeout(r, 0))
    expect(syncTopicAnalysisStatus).toHaveBeenCalledTimes(1)
    expect(loadAllTopicTables).toHaveBeenCalledTimes(1)
    expect(loadTopics).toHaveBeenLastCalledWith(8802)
    wrapper.unmount()
  })

  it('bootstrap 顺序契约（Critical 修复）：loadAllTopicTables 先于 syncTopicAnalysisStatus', async () => {
    const wrapper = mountPanel()
    await new Promise(r => setTimeout(r, 0))
    expect(loadAllTopicTables).toHaveBeenCalledWith(101)
    expect(syncTopicAnalysisStatus).toHaveBeenCalledWith(101)
    const loadAllOrder = loadAllTopicTables.mock.invocationCallOrder[0] ?? 0
    const syncOrder = syncTopicAnalysisStatus.mock.invocationCallOrder[0] ?? 0
    expect(syncOrder).toBeGreaterThan(loadAllOrder)
    expect(loadAllTopicTables).toHaveBeenCalledTimes(1)
    expect(syncTopicAnalysisStatus).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
})

describe('BoardEnrichmentPanel 信号事件接线（FE-1/FE-2/5.4）', () => {
  it('候选列表 commit → changePeriod（只重拉列表，不触发发现）', async () => {
    const wrapper = mountPanel()
    await nextTick()
    await wrapper.find('.sc-commit').trigger('click')
    expect(signal.changePeriod).toHaveBeenCalledWith(7701, 'month', '2026-08')
    expect(signal.discover).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('FE-1：候选列表 discover → 同周期直接 discover（不重复 changePeriod）；无 research 串发', async () => {
    const wrapper = mountPanel()
    await nextTick()
    await wrapper.find('.sc-discover').trigger('click')
    expect(signal.discover).toHaveBeenCalledTimes(1)
    expect(signal.discover).toHaveBeenCalledWith(7701)
    expect(signal.changePeriod).not.toHaveBeenCalled() // 周期未变，无需重拉
    expect(signal.research).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('FE-1：不同周期的 discover → 先 changePeriod 对齐再发现', async () => {
    const wrapper = mountPanel()
    await nextTick()
    signal.period.value = '2026-07'
    await nextTick()
    await wrapper.find('.sc-discover').trigger('click')
    expect(signal.changePeriod).toHaveBeenCalledWith(7701, 'month', '2026-09')
    expect(signal.discover).toHaveBeenCalledTimes(1)
    expect(signal.research).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('FE-2：深入分析 → research(boardId, 候选id, regenerate:false)（仅被点一条）', async () => {
    const wrapper = mountPanel()
    await nextTick()
    await wrapper.find('.sc-research').trigger('click')
    expect(signal.research).toHaveBeenCalledTimes(1)
    expect(signal.research).toHaveBeenCalledWith(7701, 31, { regenerate: false })
    wrapper.unmount()
  })

  it('5.4：显式重新研究（regenerate=true）需 confirm 预算提示，确认后才发起', async () => {
    vi.stubGlobal('confirm', () => true)
    try {
      const wrapper = mountPanel()
      await nextTick()
      await wrapper.find('.sc-regenerate').trigger('click')
      expect(signal.research).toHaveBeenCalledWith(7701, 31, { regenerate: true })
      wrapper.unmount()
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('5.4：重新研究 confirm 取消 → 不发起（显式操作门禁）', async () => {
    vi.stubGlobal('confirm', () => false)
    try {
      const wrapper = mountPanel()
      await nextTick()
      await wrapper.find('.sc-regenerate').trigger('click')
      expect(signal.research).not.toHaveBeenCalled()
      wrapper.unmount()
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('open-report → openReport(boardId, resultId)（200 already_reported/阅读按钮共用入口）', async () => {
    const wrapper = mountPanel()
    await nextTick()
    await wrapper.find('.sc-open').trigger('click')
    expect(signal.openReport).toHaveBeenCalledWith(7701, 501)
    // 候选列表/报告列表隐藏（阅读视图激活）
    signal.activeReportId.value = 501
    await nextTick()
    expect(wrapper.find('.signal-report-view-stub').exists()).toBe(true)
    expect(wrapper.find('.signal-candidates-stub').exists()).toBe(false)
    wrapper.unmount()
  })

  it('报告阅读视图 back → closeReport（周期/任务状态保留在 composable）', async () => {
    signal.activeReportId.value = 501
    signal.activeReport.value = { id: 501 }
    const wrapper = mountPanel()
    await nextTick()
    await wrapper.find('.srv-back').trigger('click')
    expect(signal.closeReport).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('报告页重新研究 → handleSignalResearch(regenerate:true)（confirm 后发起）', async () => {
    vi.stubGlobal('confirm', () => true)
    try {
      signal.activeReportId.value = 501
      signal.activeReport.value = { id: 501 }
      const wrapper = mountPanel()
      await nextTick()
      await wrapper.find('.srv-regen').trigger('click')
      expect(signal.research).toHaveBeenCalledWith(7701, 31, { regenerate: true })
      wrapper.unmount()
    } finally {
      vi.unstubAllGlobals()
    }
  })
})

describe('BoardEnrichmentPanel 聚焦分析折叠区（单泳道入口保留）', () => {
  it('聚焦分析触发：不自动触发（用户点击才发），lens 透传', async () => {
    vi.stubGlobal('confirm', () => true)
    try {
      const wrapper = mountPanel()
      const focusToggle = wrapper.findAll('.focus-toggle').find(b => b.text().includes('聚焦分析'))
      await focusToggle!.trigger('click')
      expect(triggerEnrichmentMock).not.toHaveBeenCalled()
      await wrapper.find('.focus-body .btn-primary').trigger('click')
      expect(triggerEnrichmentMock).toHaveBeenCalledWith(101, undefined)
      wrapper.unmount()
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('topic 聚焦区 QAPanel 保持原样（topic refs + ask/sediment 接线）', async () => {
    const wrapper = mountPanel()
    latestResultId.value = 55
    const focusToggle = wrapper.findAll('.focus-toggle').find(b => b.text().includes('聚焦分析'))
    await focusToggle!.trigger('click')
    await nextTick()
    const topicPanel = wrapper.findComponent({ name: 'QAPanel' })
    expect(topicPanel.exists()).toBe(true)
    expect(topicPanel.props('resultId')).toBe(55)
    topicPanel.vm.$emit('ask', '追问问题')
    topicPanel.vm.$emit('sediment', 5)
    await nextTick()
    expect(askQuestion).toHaveBeenCalledWith('追问问题')
    expect(sedimentAnswer).toHaveBeenCalledWith(5)
    wrapper.unmount()
  })
})
