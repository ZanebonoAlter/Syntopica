/**
 * LegacyResearchHistory — 旧研究历史只读外壳（redesign-enrichment-research-workbench 5.2）。
 *
 * 挂载测试，API 层全 mock（~/api/boardEnrichment → useBoardEnrichmentApi）：
 *  - 列表：四类旧 kind 行渲染；未知 kind 如实显示原始标识不崩；无记录不补 0；
 *  - 详情：按 result_kind 分发既有 reader（断言 mock 子组件 props / 详情 API 入参）；
 *  - 单条坏载荷只降级该条（「原始记录不可解析」+ 原始记录只读入口），列表其余行照常；
 *  - 列表与详情独立错误态、独立重试；
 *  - 分页「加载更多」before_id 游标追加不重置；
 *  - 零 LLM 红线：reader 自带生成/追问/沉淀入口不可见、无任何写 API 调用；
 *  - open-topic 下钻钩子（列表话题 chip + reader lane chip 统一上报）；
 *  - 弹层契约：AppDialog size="xl"（1040 档，不传自由 width）。
 *
 * 断言方式：事件经 attrs onXxx spy 捕获（本仓库 vitest 环境下 wrapper.emitted()
 * 不记录自定义事件——同 SignalReportView.test.ts 既有红）。
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import LegacyResearchHistory from './LegacyResearchHistory.vue'
import BoardBriefReport from './BoardBriefReport.vue'
import type { LegacyHistoryRow } from '~/api/boardEnrichment'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))
vi.mock('~/utils/markdown', () => ({
  renderMarkdown: (s: string) => `<p>${s}</p>`,
}))

const apiMocks = vi.hoisted(() => ({
  listLegacyHistory: vi.fn(),
  getBoardAnalysisResult: vi.fn(),
  getResult: vi.fn(),
  listQA: vi.fn(),
  listBoardQA: vi.fn(),
  listDebates: vi.fn(),
  listReviews: vi.fn(),
}))
vi.mock('~/api/boardEnrichment', () => ({
  useBoardEnrichmentApi: () => apiMocks,
}))

const ok = <T,>(data: T) => ({ success: true, data })
const fail = (error = '后端不可用') => ({ success: false, error })

function row(overrides: Partial<LegacyHistoryRow> = {}): LegacyHistoryRow {
  return {
    id: 11,
    result_kind: 'board_brief',
    analysis_scope: 'board',
    persistent_topic_id: null,
    semantic_board_id: 7,
    created_at: '2026-09-01T10:00:00Z',
    session_id: 'sess-abcdef1234',
    ...overrides,
  }
}

/** 整页 fixture（id 降序，模拟后端 id DESC 游标分页）。 */
function page(count: number, headId: number): LegacyHistoryRow[] {
  return Array.from({ length: count }, (_, i) =>
    row({ id: headId - i, result_kind: 'legacy_board_analysis' }),
  )
}

const stubs = {
  AppDialog: {
    name: 'AppDialog',
    props: ['modelValue', 'title', 'size'],
    template: '<div class="dialog-stub"><slot /></div>',
  },
  BoardBriefReport: {
    name: 'BoardBriefReport',
    props: ['result', 'loading'],
    template: '<div class="brief-stub" />',
  },
  BoardInvestigationReport: {
    name: 'BoardInvestigationReport',
    props: ['result', 'loading'],
    template: '<div class="investigation-stub" />',
  },
  BoardAnalysisReport: {
    name: 'BoardAnalysisReport',
    props: ['result', 'loading'],
    template: '<div class="board-analysis-stub" />',
  },
  CausalAnalysisReport: {
    name: 'CausalAnalysisReport',
    props: ['result', 'topicLabel', 'loading'],
    template: '<div class="causal-stub" />',
  },
  DebateSection: {
    name: 'DebateSection',
    props: ['stage', 'debates'],
    template: '<div class="debate-stub" />',
  },
}

const rowsFixture: LegacyHistoryRow[] = [
  row({ id: 11, result_kind: 'board_brief' }),
  row({
    id: 12,
    result_kind: 'topic_analysis',
    analysis_scope: 'topic',
    persistent_topic_id: 33,
    semantic_board_id: null,
  }),
  row({ id: 13, result_kind: 'legacy_board_analysis' }),
  row({ id: 14, result_kind: 'board_investigation' }),
  row({ id: 15, result_kind: 'weird_kind' }),
]

const briefSectors = {
  result_kind: 'board_brief',
  summary: '旧简报概览',
  observations: [{ id: 'O1', statement: '库存下降', lane_id: 5 }],
  relationships: [],
  uncertainties: [],
  research_questions: [{ id: 'Q1', question: '为什么下降？' }],
  lane_refs: [],
}

async function flush(w: VueWrapper) {
  await flushPromises()
  await nextTick()
  await nextTick()
  return w
}

async function mountHistory(
  extra: { attrs?: Record<string, unknown>; stubsOverride?: Record<string, unknown> } = {},
) {
  const wrapper = mount(LegacyResearchHistory, {
    props: { boardId: 7, open: true },
    attrs: extra.attrs,
    global: {
      stubs: { ...stubs, ...(extra.stubsOverride ?? {}) },
    },
  })
  return flush(wrapper)
}

beforeEach(() => {
  vi.clearAllMocks()
  apiMocks.listLegacyHistory.mockResolvedValue(ok(rowsFixture))
  apiMocks.getBoardAnalysisResult.mockImplementation((_boardId: number, id: number) =>
    Promise.resolve(
      ok({
        id,
        analysis_scope: 'board',
        result_kind:
          id === 14 ? 'board_investigation'
          : id === 13 ? 'legacy_board_analysis'
          : 'board_brief',
        sectors:
          id === 14
            ? { result_kind: 'board_investigation', question: { text: '调查问题' } }
            : id === 13
              ? { form: 'event_chain', thesis: '旧论文命题', argument: { intro: 'x' } }
              : briefSectors,
      }),
    ),
  )
  apiMocks.getResult.mockResolvedValue(
    ok({
      id: 12,
      persistent_topic_id: 33,
      evolution_assessment: '',
      sectors: { form: 'event_chain', lens: '供给', analysis: { fact_layer: [] } },
      causal_chain: null,
      tool_calls: null,
      input_snapshot: null,
      session_id: 'sess-abcdef1234',
      created_at: '2026-09-01T10:00:00Z',
    }),
  )
  apiMocks.listQA.mockResolvedValue(ok([]))
  apiMocks.listBoardQA.mockResolvedValue(ok([]))
  apiMocks.listDebates.mockResolvedValue(ok([]))
  apiMocks.listReviews.mockResolvedValue(ok([]))
})

describe('LegacyResearchHistory 列表', () => {
  it('四类 kind 行渲染中文名 + 日期 + session 短标识 + 话题归属；未知 kind 如实显示原始标识不崩', async () => {
    const wrapper = await mountHistory()

    const text = wrapper.find('[data-test="legacy-list"]').text()
    expect(text).toContain('简报 #11')
    expect(text).toContain('话题分析 #12')
    expect(text).toContain('旧版分析 #13')
    expect(text).toContain('调查 #14')
    // 未知 kind：显示原始标识 + 不可解析标记，不崩
    expect(text).toContain('weird_kind #15')
    expect(wrapper.find('[data-test="legacy-row-15"]').text()).toContain('不可解析')
    // 行元数据
    expect(wrapper.find('[data-test="legacy-row-11"]').text()).toContain('2026-09-01')
    expect(wrapper.find('[data-test="legacy-row-11"]').text()).toContain('sess-abc')
    expect(wrapper.find('[data-test="legacy-topic-link-33"]').exists()).toBe(true)
    // 无假计数、不补 0：首页未满时无「加载更多」
    expect(wrapper.find('[data-test="legacy-load-more"]').exists()).toBe(false)
  })

  it('无记录空态直说，不补 0 不造假占位', async () => {
    apiMocks.listLegacyHistory.mockResolvedValue(ok([]))
    const wrapper = await mountHistory()
    expect(wrapper.find('[data-test="legacy-empty"]').text()).toContain('暂无旧版研究记录')
    expect(wrapper.find('[data-test="legacy-list"]').text()).not.toContain('0 条')
  })

  it('列表加载失败独立报错可重试，与详情互不影响', async () => {
    apiMocks.listLegacyHistory.mockResolvedValueOnce(fail('库连不上'))
    const wrapper = await mountHistory()
    const errBox = wrapper.find('[data-test="legacy-list-error"]')
    expect(errBox.exists()).toBe(true)
    expect(errBox.text()).toContain('库连不上')

    apiMocks.listLegacyHistory.mockResolvedValue(ok(rowsFixture))
    await wrapper.find('[data-test="legacy-list-retry"]').trigger('click')
    await flush(wrapper)
    expect(wrapper.find('[data-test="legacy-row-11"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="legacy-list-error"]').exists()).toBe(false)
  })
})

describe('LegacyResearchHistory 详情分发', () => {
  it('board_brief 行 → getBoardAnalysisResult + BoardBriefReport（正确 result props）', async () => {
    const wrapper = await mountHistory()
    await wrapper.find('[data-test="legacy-row-11"]').trigger('click')
    await flush(wrapper)

    expect(apiMocks.getBoardAnalysisResult).toHaveBeenCalledWith(7, 11)
    const brief = wrapper.findComponent({ name: 'BoardBriefReport' })
    expect(brief.exists()).toBe(true)
    expect((brief.props('result') as { id: number }).id).toBe(11)
    expect(wrapper.findComponent({ name: 'CausalAnalysisReport' }).exists()).toBe(false)
  })

  it('topic_analysis 行 → getResult(topicId, id) + CausalAnalysisReport，其余 board reader 不挂', async () => {
    const wrapper = await mountHistory()
    await wrapper.find('[data-test="legacy-row-12"]').trigger('click')
    await flush(wrapper)

    expect(apiMocks.getResult).toHaveBeenCalledWith(33, 12)
    const causal = wrapper.findComponent({ name: 'CausalAnalysisReport' })
    expect(causal.exists()).toBe(true)
    expect(causal.props('topicLabel')).toBe('话题 #33')
    expect(wrapper.findComponent({ name: 'BoardBriefReport' }).exists()).toBe(false)
  })

  it('legacy_board_analysis / board_investigation 行 → BoardAnalysisReport / BoardInvestigationReport', async () => {
    const wrapper = await mountHistory()
    await wrapper.find('[data-test="legacy-row-13"]').trigger('click')
    await flush(wrapper)
    expect(wrapper.findComponent({ name: 'BoardAnalysisReport' }).exists()).toBe(true)

    await wrapper.find('[data-test="legacy-row-14"]').trigger('click')
    await flush(wrapper)
    expect(wrapper.findComponent({ name: 'BoardInvestigationReport' }).exists()).toBe(true)
    expect(wrapper.findComponent({ name: 'BoardAnalysisReport' }).exists()).toBe(false)
  })

  it('未知 kind 行：不调详情 API，降级显示 + 原始记录入口；列表其余行照常可点开', async () => {
    const wrapper = await mountHistory()
    await wrapper.find('[data-test="legacy-row-15"]').trigger('click')
    await flush(wrapper)

    expect(apiMocks.getBoardAnalysisResult).not.toHaveBeenCalled()
    expect(apiMocks.getResult).not.toHaveBeenCalled()
    const deg = wrapper.find('[data-test="legacy-degraded"]')
    expect(deg.exists()).toBe(true)
    expect(deg.text()).toContain('原始记录不可解析')
    // 原始记录只读入口：未知 kind 原文可见
    expect(wrapper.find('[data-test="legacy-raw"]').text()).toContain('weird_kind')

    // 其余行照常
    await wrapper.find('[data-test="legacy-row-11"]').trigger('click')
    await flush(wrapper)
    expect(wrapper.findComponent({ name: 'BoardBriefReport' }).exists()).toBe(true)
  })

  it('坏载荷（sectors 缺失）只降级该条并给原始记录，列表其余行可点开', async () => {
    // Once：只污染 row-11 本条，其余行走 beforeEach 的正常 mock
    apiMocks.getBoardAnalysisResult.mockResolvedValueOnce(
      ok({ id: 11, analysis_scope: 'board', result_kind: 'board_brief', sectors: null }),
    )
    const wrapper = await mountHistory()
    await wrapper.find('[data-test="legacy-row-11"]').trigger('click')
    await flush(wrapper)

    const deg = wrapper.find('[data-test="legacy-degraded"]')
    expect(deg.exists()).toBe(true)
    expect(deg.text()).toContain('原始记录不可解析')
    expect(wrapper.find('[data-test="legacy-raw"]').text()).toContain('"id": 11')
    expect(wrapper.findComponent({ name: 'BoardBriefReport' }).exists()).toBe(false)

    await wrapper.find('[data-test="legacy-row-14"]').trigger('click')
    await flush(wrapper)
    expect(wrapper.findComponent({ name: 'BoardInvestigationReport' }).exists()).toBe(true)
  })

  it('详情请求失败独立报错可重试，列表不动', async () => {
    apiMocks.getResult.mockResolvedValue(fail('详情炸了'))
    const wrapper = await mountHistory()
    await wrapper.find('[data-test="legacy-row-12"]').trigger('click')
    await flush(wrapper)

    const errBox = wrapper.find('[data-test="legacy-detail-error"]')
    expect(errBox.exists()).toBe(true)
    expect(errBox.text()).toContain('详情炸了')
    expect(wrapper.find('[data-test="legacy-list-error"]').exists()).toBe(false)

    apiMocks.getResult.mockResolvedValue(
      ok({
        id: 12,
        sectors: { form: 'sparse', lens: '', analysis: {} },
        created_at: '2026-09-01T10:00:00Z',
      }),
    )
    await wrapper.find('[data-test="legacy-detail-retry"]').trigger('click')
    await flush(wrapper)
    expect(wrapper.find('[data-test="legacy-detail-error"]').exists()).toBe(false)
    expect(wrapper.findComponent({ name: 'CausalAnalysisReport' }).exists()).toBe(true)
  })
})

describe('LegacyResearchHistory 只读红线与子产物', () => {
  it('真实 BoardBriefReport 挂载：生成/追问/沉淀/跨版块入口不可见，无写控件', async () => {
    const wrapper = await mountHistory({
      stubsOverride: { BoardBriefReport }, // 唯一用真实 reader 的用例
    })
    await wrapper.find('[data-test="legacy-row-11"]').trigger('click')
    await flush(wrapper)

    // reader 确实渲染了内容（说明下面的隐藏断言有对象）
    expect(wrapper.find('.bb-report').exists()).toBe(true)
    // 生成/追问/自填入口被内联 display:none 隐藏（hideWriteControls；不依赖
    // isVisible——happy-dom 下样式表未注入、computed display 不可靠）
    const hiddenInline = (el: Element | null): boolean => {
      let cur: Element | null = el
      while (cur) {
        if ((cur as HTMLElement).style?.display === 'none') return true
        cur = cur.parentElement
      }
      return false
    }
    for (const sel of [
      '[data-test="q-investigate"]',
      '[data-test="custom-investigate"]',
      '[data-test="obs-discover-O1"]',
      '.bb-custom',
      'textarea',
    ]) {
      const el = wrapper.find(sel)
      expect(el.exists(), `应已渲染：${sel}`).toBe(true)
      expect(hiddenInline(el.element), `应为内联隐藏：${sel}`).toBe(true)
    }
    // 全组件无可用生成/辩论触发按钮：要么不存在，要么已内联隐藏
    const btns = wrapper.findAll('button')
    for (const banned of ['深入调查', '发现关联', '开始辩论', '沉淀']) {
      const offending = btns.filter((b) => b.text().includes(banned))
      expect(
        offending.every((b) => hiddenInline(b.element)),
        `「${banned}」按钮必须不存在或已隐藏`,
      ).toBe(true)
    }
    // 零 LLM：本外壳只读，不调任何触发类 API（mock 表里根本没有触发函数，
    // 这里锁 listLegacyHistory 之外无调用入口）
    expect(apiMocks.listLegacyHistory).toHaveBeenCalledTimes(1)
  })

  it('子产物只读展示：QA 静态列表（无追问输入），辩论以 result 态展示，无按钮', async () => {
    apiMocks.listBoardQA.mockResolvedValue(
      ok([
        {
          id: 1,
          topic_enrichment_result_id: 11,
          question: '为什么看库存？',
          answer: '因为**数据**这么说',
          tool_calls: null,
          source: 'manual',
          sedimented: true,
          created_at: '2026-09-02T08:00:00Z',
        },
      ]),
    )
    const wrapper = await mountHistory()
    await wrapper.find('[data-test="legacy-row-11"]').trigger('click')
    await flush(wrapper)

    const qa = wrapper.find('[data-test="legacy-qa"]')
    expect(qa.exists()).toBe(true)
    expect(qa.text()).toContain('问：为什么看库存？')
    expect(qa.text()).toContain('因为**数据**这么说')
    // 只读：无追问输入框/提交按钮，无沉淀按钮
    expect(qa.find('textarea').exists()).toBe(false)
    expect(qa.find('input').exists()).toBe(false)
    // 辩论：空数据不出区块（不造假占位）
    expect(wrapper.find('[data-test="legacy-debates"]').exists()).toBe(false)
    expect(wrapper.findComponent({ name: 'DebateSection' }).exists()).toBe(false)
  })

  it('旧辩论数据存在时 DebateSection 以 result 态只读展示（无触发/重试）', async () => {
    apiMocks.listDebates.mockResolvedValue(
      ok([
        {
          id: 9,
          topic_enrichment_result_id: 12,
          sector: '原油',
          code: 'PASHA',
          name: '原油ETF',
          verdict: 'flat',
          agents: [],
          votes: { up: 1, flat: 2, down: 0 },
        },
      ]),
    )
    const wrapper = await mountHistory()
    await wrapper.find('[data-test="legacy-row-12"]').trigger('click')
    await flush(wrapper)

    const debate = wrapper.findComponent({ name: 'DebateSection' })
    expect(debate.exists()).toBe(true)
    expect(debate.props('stage')).toBe('result')
    expect((debate.props('debates') as unknown[]).length).toBe(1)
    const btnTexts = wrapper.findAll('button').map((b) => b.text())
    expect(btnTexts.some((t) => t.includes('开始辩论'))).toBe(false)
  })

  it('子产物加载失败只出提示，不阻塞主报告', async () => {
    apiMocks.listBoardQA.mockRejectedValue(new Error('qa 炸了'))
    const wrapper = await mountHistory()
    await wrapper.find('[data-test="legacy-row-11"]').trigger('click')
    await flush(wrapper)

    expect(wrapper.findComponent({ name: 'BoardBriefReport' }).exists()).toBe(true)
    expect(wrapper.find('[data-test="legacy-sub-error"]').text()).toContain('加载失败')
    expect(wrapper.find('[data-test="legacy-qa"]').exists()).toBe(false)
  })
})

describe('LegacyResearchHistory 分页与弹层契约', () => {
  it('「加载更多」按 before_id 游标追加不重置；追完后按钮消失', async () => {
    apiMocks.listLegacyHistory
      .mockResolvedValueOnce(ok(page(20, 100)))
      .mockResolvedValueOnce(ok(page(5, 80)))
    const wrapper = await mountHistory()
    expect(wrapper.findAll('.lh-row').length).toBe(20)
    expect(wrapper.find('[data-test="legacy-row-100"]').exists()).toBe(true)

    await wrapper.find('[data-test="legacy-load-more"]').trigger('click')
    await flush(wrapper)

    // 游标 = 首页最后一条 id
    expect(apiMocks.listLegacyHistory).toHaveBeenLastCalledWith(7, { limit: 20, beforeId: 81 })
    expect(wrapper.findAll('.lh-row').length).toBe(25)
    expect(wrapper.find('[data-test="legacy-row-100"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="legacy-row-76"]').exists()).toBe(true)
    // 不足整页 → 没有下一页，按钮消失
    expect(wrapper.find('[data-test="legacy-load-more"]').exists()).toBe(false)
  })

  it('加载更多失败独立报错可重试，已加载行不丢', async () => {
    apiMocks.listLegacyHistory
      .mockResolvedValueOnce(ok(page(20, 100)))
      .mockResolvedValueOnce(fail('翻页炸了'))
    const wrapper = await mountHistory()
    await wrapper.find('[data-test="legacy-load-more"]').trigger('click')
    await flush(wrapper)

    expect(wrapper.find('[data-test="legacy-more-error"]').text()).toContain('翻页炸了')
    expect(wrapper.findAll('.lh-row').length).toBe(20)

    apiMocks.listLegacyHistory.mockResolvedValueOnce(ok(page(3, 80)))
    await wrapper.find('[data-test="legacy-more-retry"]').trigger('click')
    await flush(wrapper)
    expect(wrapper.findAll('.lh-row').length).toBe(23)
  })

  it('弹层契约：AppDialog size=xl（1040 档），不传自由 width', async () => {
    const wrapper = await mountHistory()
    const dialog = wrapper.findComponent({ name: 'AppDialog' })
    expect(dialog.exists()).toBe(true)
    expect(dialog.props('size')).toBe('xl')
    expect(dialog.props('modelValue')).toBe(true)
    // 源码不出现自由 width（layout.md 弹窗档红线）
    expect(Object.keys(dialog.props())).not.toContain('width')
  })

  it('open-topic 下钻钩子：列表话题 chip 与 reader lane chip 统一上报', async () => {
    const onOpenTopic = vi.fn()
    const wrapper = await mountHistory({
      attrs: { onOpenTopic },
      stubsOverride: { BoardBriefReport }, // lane chip 由真实 reader 渲染
    })
    await wrapper.find('[data-test="legacy-topic-link-33"]').trigger('click')
    expect(onOpenTopic).toHaveBeenCalledWith(33)

    onOpenTopic.mockClear()
    await wrapper.find('[data-test="legacy-row-11"]').trigger('click')
    await flush(wrapper)
    // 真实 reader 的观察 lane chip：drill-lane → open-topic
    const laneChip = wrapper.find('[data-test="obs-lane-5"]')
    expect(laneChip.exists()).toBe(true)
    await laneChip.trigger('click')
    expect(onOpenTopic).toHaveBeenCalledTimes(1)
    expect(onOpenTopic).toHaveBeenCalledWith(5)
  })

  it('open=false 挂载不拉列表；切 open=true 触发加载', async () => {
    const wrapper = mount(LegacyResearchHistory, {
      props: { boardId: 7, open: false },
      global: { stubs },
    })
    await flush(wrapper)
    expect(apiMocks.listLegacyHistory).not.toHaveBeenCalled()

    await wrapper.setProps({ open: true })
    await flush(wrapper)
    expect(apiMocks.listLegacyHistory).toHaveBeenCalledTimes(1)
  })
})
