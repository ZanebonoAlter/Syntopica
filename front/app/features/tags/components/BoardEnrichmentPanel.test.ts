/**
 * BoardEnrichmentPanel — 研究工作台信息架构（redesign-enrichment-research-workbench 4.1）。
 *
 * 主从（AppPageShell split）布局：左栏 = 候选索引（SignalCandidateList stub），
 * 右栏 = 详情（未选提示 → 候选详情 → SignalReportView 报告阅读）。退役区块
 * （聚焦分析折叠区/新闻背景折叠区/DebateSection/底部报告列表）不在 DOM；
 * 旧 API 后端保留兼容，面板不挂生成/追问/沉淀/辩论控件。
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

const selectedTopicId = ref<number | null>(101)
const loadTopics = vi.fn().mockResolvedValue(undefined)
const loadDataSources = vi.fn().mockResolvedValue(undefined)
const loadBoardAnalysisResults = vi.fn().mockResolvedValue(undefined)
const loadAllTopicTables = vi.fn().mockResolvedValue(undefined)
const syncTopicAnalysisStatus = vi.fn().mockResolvedValue(undefined)
const syncBoardAnalysisStatus = vi.fn().mockResolvedValue(undefined)
const activateBoardContext = vi.fn()

// notify 需可断言：hoist 捕获
const notifyMocks = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warn: vi.fn(),
}))

vi.mock('~/features/tags/composables/useBoardEnrichment', () => ({
  // 面板只解构 bootstrap 最小集合（4.1 退役三区块后其余能力不再消费）
  useBoardEnrichment: () => ({
    selectedTopicId, loadTopics,
    loadDataSources,
    loadBoardAnalysisResults, syncBoardAnalysisStatus, syncTopicAnalysisStatus, activateBoardContext,
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
  candidateVisibility: ref<'visible' | 'ignored' | 'all'>('visible'),
  discoveryRunning: ref(false),
  discoveryPhase: ref<string | null>(null),
  discoveryError: ref<string | null>(null),
  researchCandidateId: ref<number | null>(null),
  researchPhase: ref<string | null>(null),
  researchError: ref<string | null>(null),
  researchProgressLabel: ref('正在研究 · 决策上限 40 轮 · 可离开页面'),
  researchProgress: ref(null),
  researchTaskPeriod: ref(''),
  researchTaskGranularity: ref<'month' | 'year'>('month'),
  activeReportId: ref<number | null>(null),
  activeReport: ref<object | null>(null),
  activeReportLoading: ref(false),
  activeReportError: ref<string | null>(null),
  activeCandidateId: ref<number | null>(null),
  activeCandidate: ref<object | null>(null),
  activeCandidateLoading: ref(false),
  activeCandidateError: ref<string | null>(null),
  setBoard: vi.fn(),
  loadCandidates: vi.fn().mockResolvedValue(undefined),
  loadReports: vi.fn().mockResolvedValue(undefined),
  loadPeriod: vi.fn().mockResolvedValue(undefined),
  changePeriod: vi.fn().mockResolvedValue(true),
  discover: vi.fn().mockResolvedValue(true),
  research: vi.fn().mockResolvedValue(true),
  openReport: vi.fn().mockResolvedValue(undefined),
  closeReport: vi.fn(),
  openCandidate: vi.fn().mockResolvedValue(undefined),
  closeCandidate: vi.fn(),
  ignoreCandidate: vi.fn().mockResolvedValue(true),
  restoreCandidate: vi.fn().mockResolvedValue(true),
  setCandidateVisibility: vi.fn().mockResolvedValue(undefined),
  candidateView: ref<'all' | 'pending' | 'readable'>('all'),
  selectCandidateView: vi.fn().mockResolvedValue(undefined),
  returnToResearchTask: vi.fn().mockResolvedValue(true),
}

vi.mock('~/features/tags/composables/useSignalWorkbench', () => ({
  useSignalWorkbench: () => signal,
}))

// 信号子组件：模块级替换（stub 名称匹配在本环境不可靠）
vi.mock('./SignalCandidateList.vue', () => ({
  default: {
    name: 'SignalCandidateList',
    props: ['candidates', 'loading', 'error', 'granularity', 'period', 'discoveryRunning', 'discoveryPhase', 'discoveryError', 'researchCandidateId', 'researchPhase', 'researchError', 'researchProgressLabel', 'researchProgress', 'reports', 'visibility'],
    emits: ['commit', 'discover', 'research', 'open-report', 'open-progress', 'select', 'ignore', 'restore', 'set-visibility', 'select-view'],
    template: `<div class="signal-candidates-stub">
      <button class="sc-commit" @click="$emit('commit', { granularity: 'month', period: '2026-08' })" />
      <button class="sc-discover" @click="$emit('discover', { granularity: 'month', period: '2026-09' })" />
      <button class="sc-research" @click="$emit('research', 31, { regenerate: false })" />
      <button class="sc-regenerate" @click="$emit('research', 31, { regenerate: true })" />
      <button class="sc-open" @click="$emit('open-report', 501)" />
      <button class="sc-select" @click="$emit('select', 31)" />
      <button class="sc-ignore" @click="$emit('ignore', 31)" />
      <button class="sc-restore" @click="$emit('restore', 31)" />
      <button class="sc-visibility" @click="$emit('set-visibility', 'ignored')" />
      <button class="sc-select-view" @click="$emit('select-view', { visibility: 'ignored', view: 'all' })" />
    </div>`,
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

const stubs = {
  AppButton: { name: 'AppButton', template: '<button class="app-btn-stub"><slot /></button>' },
}

function mountPanel(attrs: Record<string, unknown> = {}): VueWrapper {
  return mount(BoardEnrichmentPanel, { props: { boardId: 7701 }, attrs, global: { stubs } })
}

function makeCandidateDetail(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 31,
    signal: '仓库里的原油少了，真的说明大家用油更多了吗？',
    why_it_matters: '库存少了可能是用得更多，也可能只是进货或调货的时间变了。',
    research_question: '原油为什么少了？这是不是一个值得关注的涨价信号？',
    evidence_refs: ['31', '32'],
    related_topics: [{ topic_id: 101, label: '原油供需', frozen_label: '原油供需', frozen_status: 'active', section_ids: [] }],
    related_gaps: ['仅确认一条新闻切片，跨源一致性未验证'],
    ignored_at: null,
    status: 'pending',
    latest_result_id: null,
    period: '2026-09',
    discovery_created_at: '2026-09-22T08:30:00Z',
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  // 重置模块级可变 ref（测试间不得顺序耦合）
  selectedTopicId.value = 101
  signal.granularity.value = 'month'
  signal.period.value = '2026-09'
  signal.candidates.value = []
  signal.reports.value = []
  signal.candidateVisibility.value = 'visible'
  signal.discoveryRunning.value = false
  signal.researchCandidateId.value = null
  signal.researchTaskPeriod.value = ''
  signal.activeReportId.value = null
  signal.activeReport.value = null
  signal.activeCandidateId.value = null
  signal.activeCandidate.value = null
  signal.activeCandidateLoading.value = false
  signal.activeCandidateError.value = null
})

describe('BoardEnrichmentPanel 工作台信息架构（split 主从）', () => {
  it('主视图 = 左索引 + 右详情（标题「研究」+ 刷新入口）；报告不再在列表底部重复排列', () => {
    const wrapper = mountPanel()
    expect(wrapper.find('.signal-candidates-stub').exists()).toBe(true)
    // split 主从壳：aside 索引 + main 详情
    expect(wrapper.find('[data-shell-mode="split"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workbench-detail"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workbench-detail-empty"]').exists()).toBe(true)
    // 底部报告列表退役：报告归到候选详情（IA：不把同一报告在列表底部重复排列）
    expect(wrapper.find('.signal-reports-stub').exists()).toBe(false)
    const boardHead = wrapper.find('.board-head')
    expect(boardHead.exists()).toBe(true)
    expect(boardHead.text()).toContain('研究')
    const refreshBtn = boardHead.find('button[title*="刷新"]')
    expect(refreshBtn.exists()).toBe(true)
    wrapper.unmount()
  })

  it('FE-11：旧产出视图不在 DOM——无简报/调查/旧版分析/数据源绑定挂载', () => {
    const wrapper = mountPanel()
    expect(wrapper.find('.brief-stub').exists()).toBe(false)
    expect(wrapper.find('.investigation-stub').exists()).toBe(false)
    expect(wrapper.find('.board-report-stub').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('生成简报')
    expect(wrapper.text()).not.toContain('旧版分析')
    // 绑定管理入口退役：无数据源 chips / 绑定按钮
    expect(wrapper.find('.src-chip').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('+ 绑定')
    expect(wrapper.text()).not.toContain('绑定数据源')
    wrapper.unmount()
  })

  it('S17：退役区块不渲染——无聚焦分析折叠区/泳道选择器/新闻背景折叠区/金融辩论', () => {
    const wrapper = mountPanel()
    // 聚焦分析折叠区（单泳道下钻 + 泳道下拉 + 追问）整体退役
    expect(wrapper.text()).not.toContain('聚焦分析')
    expect(wrapper.text()).not.toContain('选择泳道')
    expect(wrapper.findAll('select').filter((s) => s.text().includes('选择泳道…')).length).toBe(0)
    // 新闻背景折叠区退役：话题背景在话题上下文（本面板无该入口）
    expect(wrapper.text()).not.toContain('新闻背景')
    expect(wrapper.text()).not.toContain('新闻记忆')
    expect(wrapper.find('.period-picker').exists()).toBe(false)
    // 金融辩论退役
    expect(wrapper.text()).not.toContain('FinGenius')
    expect(wrapper.find('.debate-stub').exists()).toBe(false)
    expect(wrapper.find('.debate-fold').exists()).toBe(false)
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
  it('候选索引 commit → changePeriod（只重拉列表，不触发发现）', async () => {
    const wrapper = mountPanel()
    await nextTick()
    await wrapper.find('.sc-commit').trigger('click')
    expect(signal.changePeriod).toHaveBeenCalledWith(7701, 'month', '2026-08')
    expect(signal.discover).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('FE-1：候选索引 discover → 同周期直接 discover（不重复 changePeriod）；无 research 串发', async () => {
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

  it('忽略/恢复/已忽略视图切换接线：emit → composable（disposition 由状态层裁决）', async () => {
    const wrapper = mountPanel()
    await nextTick()
    await wrapper.find('.sc-ignore').trigger('click')
    expect(signal.ignoreCandidate).toHaveBeenCalledWith(7701, 31)
    await wrapper.find('.sc-restore').trigger('click')
    expect(signal.restoreCandidate).toHaveBeenCalledWith(7701, 31)
    await wrapper.find('.sc-visibility').trigger('click')
    expect(signal.setCandidateVisibility).toHaveBeenCalledWith(7701, 'ignored')
    expect(signal.research).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('R-17 接线锚：select-view emit → selectCandidateView（监听器曾因处理函数嵌套静默丢失）', async () => {
    const wrapper = mountPanel()
    await nextTick()
    await wrapper.find('.sc-select-view').trigger('click')
    expect(signal.selectCandidateView).toHaveBeenCalledTimes(1)
    expect(signal.selectCandidateView).toHaveBeenCalledWith(7701, { visibility: 'ignored', view: 'all' })
    wrapper.unmount()
  })

  it('open-report → openReport(boardId, resultId)（200 already_reported/阅读按钮共用入口）', async () => {
    const wrapper = mountPanel()
    await nextTick()
    await wrapper.find('.sc-open').trigger('click')
    expect(signal.openReport).toHaveBeenCalledWith(7701, 501)
    // 阅读视图激活：详情栏切到报告，split 索引保留（主从布局不整页替换）
    signal.activeReportId.value = 501
    await nextTick()
    expect(wrapper.find('.signal-report-view-stub').exists()).toBe(true)
    expect(wrapper.find('.signal-candidates-stub').exists()).toBe(true)
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

describe('BoardEnrichmentPanel 详情栏（索引↔详情联动）', () => {
  it('索引选中候选 → openCandidate；正在读报告时先收起报告（所选即所显）', async () => {
    const wrapper = mountPanel()
    await nextTick()
    await wrapper.find('.sc-select').trigger('click')
    expect(signal.openCandidate).toHaveBeenCalledWith(7701, 31)

    signal.activeReportId.value = 501
    await nextTick()
    await wrapper.find('.sc-select').trigger('click')
    expect(signal.closeReport).toHaveBeenCalledTimes(1)
    expect(signal.openCandidate).toHaveBeenLastCalledWith(7701, 31)
    wrapper.unmount()
  })

  it('详情加载中/失败：独立状态不伪装空态；失败可重试', async () => {
    signal.activeCandidateId.value = 31
    signal.activeCandidateLoading.value = true
    const wrapper = mountPanel()
    await nextTick()
    expect(wrapper.find('[data-testid="workbench-detail-loading"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workbench-detail-empty"]').exists()).toBe(false)

    signal.activeCandidateLoading.value = false
    signal.activeCandidateError.value = '候选详情加载失败'
    await nextTick()
    const err = wrapper.find('[data-testid="workbench-detail-error"]')
    expect(err.exists()).toBe(true)
    expect(err.attributes('role')).toBe('alert')
    expect(err.text()).toContain('候选详情加载失败')
    await wrapper.find('[data-testid="workbench-detail-retry"]').trigger('click')
    expect(signal.openCandidate).toHaveBeenLastCalledWith(7701, 31)
    wrapper.unmount()
  })

  it('未读报告详情：问题/为什么值得查/依据/相关话题/缺口逐项可读', async () => {
    signal.activeCandidateId.value = 31
    signal.activeCandidate.value = makeCandidateDetail()
    const wrapper = mountPanel()
    await nextTick()
    const detail = wrapper.find('[data-testid="workbench-candidate-detail"]')
    expect(detail.exists()).toBe(true)
    expect(detail.text()).toContain('原油为什么少了')
    expect(detail.text()).toContain('为什么值得查')
    expect(detail.text()).toContain('进货或调货')
    expect(detail.text()).toContain('新闻切片 #31')
    expect(detail.text()).toContain('原油供需')
    expect(detail.text()).toContain('缺口：仅确认一条新闻切片')
    wrapper.unmount()
  })

  it('相关话题 chip 整体可点 → 开背景预览（2026-10-16 用户反馈；键盘可达）', async () => {
    signal.activeCandidateId.value = 31
    signal.activeCandidate.value = makeCandidateDetail()
    const wrapper = mountPanel()
    await nextTick()
    // 直接点 chip 本身（非内部小按钮）→ 预览弹层开且带话题名（AppDialog Teleport 到 body）
    await wrapper.find('[data-testid="related-topic-chip"]').trigger('click')
    await nextTick()
    expect(document.body.innerText).toContain('背景预览 · 原油供需')
    // 关闭（走 AppDialog update:modelValue 契约）后，键盘 Enter 也能重开（可达性锚）
    wrapper.findComponent({ name: 'AppDialog' }).vm.$emit('update:modelValue', false)
    await nextTick()
    await wrapper.find('[data-testid="related-topic-chip"]').trigger('keydown.enter')
    await nextTick()
    expect(document.body.innerText).toContain('背景预览 · 原油供需')
    wrapper.unmount()
  })

  it('详情开始研究：pending 未忽略且无任务才可点，点击只 research 被选候选', async () => {
    signal.activeCandidateId.value = 31
    signal.activeCandidate.value = makeCandidateDetail()
    const wrapper = mountPanel()
    await nextTick()
    const btn = wrapper.find('[data-testid="signal-detail-research"]')
    expect((btn.element as HTMLButtonElement).disabled).toBe(false)
    await btn.trigger('click')
    expect(signal.research).toHaveBeenCalledTimes(1)
    expect(signal.research).toHaveBeenCalledWith(7701, 31, { regenerate: false })
    wrapper.unmount()
  })

  it('详情动作互斥：发现/研究进行中忽略与开始研究禁用并提示', async () => {
    signal.activeCandidateId.value = 31
    signal.activeCandidate.value = makeCandidateDetail()
    signal.discoveryRunning.value = true
    const wrapper = mountPanel()
    await nextTick()
    expect((wrapper.find('[data-testid="signal-detail-ignore"]').element as HTMLButtonElement).disabled).toBe(true)
    expect((wrapper.find('[data-testid="signal-detail-research"]').element as HTMLButtonElement).disabled).toBe(true)
    expect(wrapper.text()).toContain('忽略与新增研究暂不可用')
    wrapper.unmount()
  })

  it('详情忽略/恢复按钮按 ignored_at 切换：未忽略→忽略，已忽略→恢复（动作只经 emit）', async () => {
    signal.activeCandidateId.value = 31
    signal.activeCandidate.value = makeCandidateDetail({ ignored_at: null })
    const wrapper = mountPanel()
    await nextTick()
    const ignoreBtn = wrapper.find('[data-testid="signal-detail-ignore"]')
    expect(ignoreBtn.text()).toContain('忽略')
    await ignoreBtn.trigger('click')
    expect(signal.ignoreCandidate).toHaveBeenCalledWith(7701, 31)

    signal.activeCandidate.value = makeCandidateDetail({ ignored_at: '2026-09-23T00:00:00Z' })
    await nextTick()
    const restoreBtn = wrapper.find('[data-testid="signal-detail-ignore"]')
    expect(restoreBtn.text()).toContain('恢复')
    await restoreBtn.trigger('click')
    expect(signal.restoreCandidate).toHaveBeenCalledWith(7701, 31)
    wrapper.unmount()
  })

  it('已有报告：详情「阅读报告」打开最新版本；版本切换按 source_signal_id 归属逐版本打开', async () => {
    signal.activeCandidateId.value = 31
    signal.activeCandidate.value = makeCandidateDetail({
      status: 'reported',
      latest_result_id: 501,
    })
    signal.reports.value = [
      { id: 503, source_signal_id: 31, created_at: '2026-10-01T00:00:00Z' },
      { id: 501, source_signal_id: 31, created_at: '2026-09-20T00:00:00Z' },
      { id: 502, source_signal_id: 32, created_at: '2026-09-21T00:00:00Z' }, // 其他候选，不计入
    ]
    const wrapper = mountPanel()
    await nextTick()
    await wrapper.find('[data-testid="signal-detail-read"]').trigger('click')
    expect(signal.openReport).toHaveBeenLastCalledWith(7701, 503) // 版本列表首项=最新

    const versionBtns = wrapper.findAll('[data-testid="signal-version-btn"]')
    expect(versionBtns).toHaveLength(2)
    expect(wrapper.text()).toContain('报告版本（2）')
    await versionBtns[1]!.trigger('click')
    expect(signal.openReport).toHaveBeenLastCalledWith(7701, 501)
    wrapper.unmount()
  })

  it('历史研究入口：索引底部按钮打开内嵌 LegacyResearchHistory 弹层（5.2）', async () => {
    const wrapper = mountPanel({})
    await nextTick()
    const btn = wrapper.find('[data-testid="open-legacy-history"]')
    expect(btn.exists()).toBe(true)
    expect(btn.text()).toContain('历史研究')
    expect(wrapper.findComponent({ name: 'LegacyResearchHistory' }).exists()).toBe(true)
    await btn.trigger('click')
    await nextTick()
    expect(wrapper.findComponent({ name: 'LegacyResearchHistory' }).props('open')).toBe(true)
    wrapper.unmount()
  })
})

describe('BoardEnrichmentPanel 任务可找回（D6 返回任务入口）', () => {
  it('研究任务在跑且任务周期≠当前周期：头部显示「返回任务」，点击切回任务周期', async () => {
    signal.researchCandidateId.value = 31
    signal.researchTaskPeriod.value = '2026-08'
    signal.researchTaskGranularity.value = 'month'
    const wrapper = mountPanel()
    await nextTick()
    const btn = wrapper.find('[data-testid="signal-return-task"]')
    expect(btn.exists()).toBe(true)
    expect(btn.text()).toContain('返回任务')
    await btn.trigger('click')
    expect(signal.returnToResearchTask).toHaveBeenCalledWith(7701)
    wrapper.unmount()
  })

  it('任务周期=当前周期（或无任务在跑）：不显示返回任务入口', async () => {
    signal.researchCandidateId.value = 31
    signal.researchTaskPeriod.value = '2026-09'
    signal.researchTaskGranularity.value = 'month'
    const wrapper = mountPanel()
    await nextTick()
    expect(wrapper.find('[data-testid="signal-return-task"]').exists()).toBe(false)

    signal.researchCandidateId.value = null
    signal.researchTaskPeriod.value = '2026-08'
    await nextTick()
    expect(wrapper.find('[data-testid="signal-return-task"]').exists()).toBe(false)
    wrapper.unmount()
  })
})

// ── question-v1 研究简报（add-question-driven-news-research 4.2）─────────────

describe('BoardEnrichmentPanel question-v1 研究简报（4.2）', () => {
  it('新协议详情：还不清楚什么/准备先查什么 逐项可读；研究提示说明新流程', async () => {
    signal.activeCandidateId.value = 31
    signal.activeCandidate.value = makeCandidateDetail({
      research_contract: 'question-v1',
      research_brief: {
        unknown: '进口减少到底是需求走弱还是调仓节奏',
        verification: '先核对当周进出口与炼厂投料口径',
        source_ids: ['n1'],
      },
    })
    const wrapper = mountPanel()
    await nextTick()
    const detail = wrapper.find('[data-testid="workbench-candidate-detail"]')
    expect(detail.find('[data-testid="detail-brief-unknown"]').text()).toContain('需求走弱还是调仓节奏')
    expect(detail.find('[data-testid="detail-brief-verification"]').text()).toContain('进出口与炼厂投料口径')
    expect(wrapper.find('[data-testid="detail-research-hint-question"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="detail-research-hint-legacy"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('旧候选无 brief：不渲染简报块；旧协议提示引导重新发现', async () => {
    signal.activeCandidateId.value = 31
    signal.activeCandidate.value = makeCandidateDetail()
    const wrapper = mountPanel()
    await nextTick()
    expect(wrapper.find('[data-testid="detail-brief-unknown"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="detail-brief-verification"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="detail-research-hint-legacy"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="detail-research-hint-question"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
