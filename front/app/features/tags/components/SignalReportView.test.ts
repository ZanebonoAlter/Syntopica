/**
 * SignalReportView + signalReport 纯函数 — 信号解读报告（5.3/5.4，FE-6~10）。
 *
 * FE-6 真实计数：meta 行取数/计算/决策数来自 generation_meta（不信模型自报）
 * FE-7 引用渲染：[[data:c1:o2]]→appendix 查表数值+单位、[[calc:k1]]→代码计算
 *      值+公式/输入/精度、点击展开附录并定位；[[news:]]→候选依据原地展示；
 *      悬空引用显示占位不崩
 * FE-8 图表：refs → SVG；null 断点不补零不连线；0 图合法
 * FE-9 历史标记（事后回顾）+ 返回列表；重新研究 regenerate 显式入口
 * FE-10 无 review：DOM 无任何评审/采纳/通过/驳回按钮徽标与 phase
 *
 * 安全渲染：LLM 文本全部走插值——断言恶意 HTML 不被解析为元素。
 * 可测纯逻辑（引用解析/图表数据变换/状态派生）直接单测 signalReport.ts。
 *
 * 断言方式：back/regenerate 事件经 attrs onXxx spy 捕获（本仓库 vitest 环境
 * 下 wrapper.emitted() 不记录自定义事件——同 FeedDetailEditor.test.ts 既有
 * 红；handler 直调路径不受影响）。
 */
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import SignalReportView from './SignalReportView.vue'
import type { SignalAppendixCall, SignalReportDetail } from '~/api/boardSignals'
import {
  appendixAnchorId,
  buildAppendixCalcRows,
  buildAppendixObsRows,
  buildParagraphViews,
  buildSignalChartData,
  candidateStatusLabel,
  directionLabel,
  findSection,
  isRetrospectiveReport,
  latestObservationPeriod,
  parseSignalReferenceTokens,
  resolveCalcRef,
  resolveDataRef,
  splitLineSegments,
  validateSignalPeriodFormat,
  type SignalChartPoint,
  type TextSegmentView,
} from './signalReport'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

// ── 合成 fixture（与 sample-report.md 同一组数值，非真实行情）──────────────

const calls = [
  {
    call_id: 'c1',
    question: '库存是否确实下降？',
    tool: 'eia_wpsr_table1',
    args: { section: 'stocks' },
    status: 'ok',
    retrieved_at: '2026-09-22T03:00:00Z',
    last_modified: '2026-09-16T12:00:00Z',
    source_sha256: 'abc123',
    observations: [
      { observation_id: 'c1:o1', label: '美国商业原油库存（不含SPR）', period: '2026-09-04', value: 420.0, raw_value: '420.0', unit: 'MMbbl' },
      { observation_id: 'c1:o2', label: '美国商业原油库存（不含SPR）', period: '2026-09-11', value: 419.0, raw_value: '419.0', unit: 'MMbbl' },
      { observation_id: 'c1:o3', label: '美国SPR库存', period: '2026-09-11', value: null, raw_value: '', unit: 'MMbbl', missing_reason: '未公布' },
    ],
  },
  {
    call_id: 'c2',
    question: '是否可由进口减少解释？',
    tool: 'eia_wpsr_table1',
    args: { section: 'supply' },
    status: 'ok',
    observations: [
      { observation_id: 'c2:o1', label: '美国原油进口', period: '2026-09-04', value: 6000, unit: 'Mb/d' },
      { observation_id: 'c2:o2', label: '美国原油进口', period: '2026-09-11', value: 6400, unit: 'Mb/d' },
    ],
  },
]

const calculations = [
  { calc_id: 'k1', op: 'difference', inputs: ['c1:o2', 'c1:o1'], expression: 'c1:o2 − c1:o1', value: '-1', unit: 'MMbbl', precision: 4, status: 'ok', computed_at: '2026-09-22T03:05:00Z' },
  { calc_id: 'k2', op: 'percent_change', inputs: ['c1:o2', 'c1:o1'], expression: '(c1:o2 − c1:o1) / c1:o1 × 100', value: '-0.2381', unit: '%', precision: 4, status: 'ok', computed_at: '2026-09-22T03:05:00Z' },
]

function makeReport(overrides: {
  title?: string
  charts?: unknown[]
  stopReason?: string
  analysisMode?: string
  candidateId?: number
  danglingRef?: boolean
} = {}): SignalReportDetail {
  const text = overrides.danglingRef
    ? '库存变化 [[data:c9:o9]] 无法核对。'
    : '仓库里的原油比前一周少了 [[data:c1:o2]]，而净进口 [[calc:k1]] 还在增加，变化率 [[calc:k2]]。另有新闻说需求改善 [[news:31]]。'
  return {
    id: 501,
    analysis_scope: 'board',
    result_kind: 'signal_report',
    semantic_board_id: 9,
    granularity: 'month',
    period: '2026-09',
    source_signal_id: 31,
    sectors: {
      schema_version: 2,
      signal_snapshot: {
        candidate_id: overrides.candidateId ?? 31,
        discovery_id: 8,
        semantic_board_id: 9,
        granularity: 'month',
        period: '2026-09',
        signal: '库存下降信号',
        why_it_matters: '值得查原因',
        research_question: '原油为什么少了？',
        evidence_refs: ['31'],
        score: 8,
        rationale: '异动与报道同向',
        analysis_mode: overrides.analysisMode ?? 'retrospective',
        cutoff: '2026-09-22T03:00:00Z',
      },
      report: {
        title: overrides.title ?? '库存少了，但还不能认定油价会涨',
        sections: [
          { kind: 'thesis', text: '先别因为仓库里的原油少了，就认定油价要涨。' },
          { kind: 'facts', text: `发生了这些事：${overrides.danglingRef ? '库存变化 [[data:c9:o9]] 无法核对。' : text}` },
          { kind: 'causal', text: '一种可能是炼厂拿走了更多原油。但现在缺少回答这个问题的数据。' },
          {
            kind: 'implication',
            text: '先看这种变化能不能持续，不急着下结论。',
            verdict: '未来一个月，先看变化能不能持续。',
            direction: 'conditional',
            horizon: '未来一个月',
            trigger_condition: '如果后面几周库存继续减少，解释更站得住脚。',
            self_doubt: '只看了相邻两周，可能把一次偶然变化当成了趋势。',
          },
        ],
        charts: (overrides.charts ?? [
          { chart_id: 'ch1', kind: 'line', claim: '仓库里的油少了一点', refs: ['c1:o1', 'c1:o2'] },
        ]) as never[],
      },
      appendix: { calls, calculations, gaps: [{ reason: '两期样本不证明趋势' }, { tool: 'eia_wpsr_table1', reason: '无炼厂投料数据' }] },
      generation_meta: {
        session_id: 'board_signal_report_31_ab12cd34',
        attempts: 1,
        retries: 0,
        decisions: 17,
        source_calls: 2,
        calculation_calls: 4,
        stop_reason: overrides.stopReason ?? 'finished',
        analysis_mode: overrides.analysisMode ?? 'retrospective',
        cutoff: '2026-09-22T03:00:00Z',
      },
    },
    session_id: 'board_signal_report_31_ab12cd34',
    created_at: '2026-09-22T03:10:00Z',
  }
}

function mountView(report: SignalReportDetail | null, opts: { loading?: boolean; error?: string | null; regenerating?: boolean } = {}) {
  const handlers = { onBack: vi.fn(), onRegenerate: vi.fn() }
  const wrapper = mount(SignalReportView, {
    props: {
      report,
      loading: opts.loading ?? false,
      error: opts.error ?? null,
      regenerating: opts.regenerating ?? false,
      regeneratePhase: 'research',
    },
    attrs: handlers,
  })
  return { wrapper, handlers }
}

// ── 纯函数：引用解析与 appendix 查表 ─────────────────────────────────────

describe('signalReport 纯函数 — 引用解析（FE-7，6.T3）', () => {
  it('parseSignalReferenceTokens：data/calc/news 三形态切分，混排保序', () => {
    const segs = parseSignalReferenceTokens('库存 [[data:c1:o2]] 与净进口 [[calc:k1]]，新闻 [[news:31]] 结尾')
    expect(segs.map(s => s.kind)).toEqual(['text', 'ref', 'text', 'ref', 'text', 'ref', 'text'])
    expect(segs[1]).toEqual({ kind: 'ref', token: { type: 'data', ref: 'c1:o2' } })
    expect(segs[3]).toEqual({ kind: 'ref', token: { type: 'calc', ref: 'k1' } })
    expect(segs[5]).toEqual({ kind: 'ref', token: { type: 'news', ref: '31' } })
  })

  it('parseSignalReferenceTokens：无 token 返回单文本段', () => {
    expect(parseSignalReferenceTokens('纯文本')).toEqual([{ kind: 'text', text: '纯文本' }])
  })

  it('resolveDataRef：raw_value 源精度优先；null 值保留缺失标记绝不转 0', () => {
    const v = resolveDataRef('c1:o2', calls)
    expect(v?.display).toBe('419.0') // 源精度（Number 会丢 .0）
    expect(v?.unit).toBe('MMbbl')
    const missing = resolveDataRef('c1:o3', calls)
    expect(missing?.display).toBe('缺失')
    expect(missing?.missing).toBe(true)
  })

  it('resolveDataRef：悬空引用返回 null（渲染层占位不崩）', () => {
    expect(resolveDataRef('c9:o9', calls)).toBeNull()
  })

  it('resolveCalcRef：ok 计算返回规范值+公式/输入/精度；missing/rejected 带原因', () => {
    const k1 = resolveCalcRef('k1', calculations)
    expect(k1?.display).toBe('-1')
    expect(k1?.unit).toBe('MMbbl')
    expect(k1?.expression).toContain('c1:o2 − c1:o1')
    expect(k1?.precision).toBe(4)
    const bad = resolveCalcRef('k9', [
      { ...calculations[0]!, calc_id: 'k9', status: 'missing', value: undefined, reason: '输入含缺失期' },
    ])
    expect(bad?.display).toBe('缺失')
    expect(bad?.notOkReason).toBe('输入含缺失期')
    expect(resolveCalcRef('nope', calculations)).toBeNull()
  })

  it('buildParagraphViews：正文拆段并解析引用；悬空 data 引用 view=null、news 不可点击', () => {
    const paras = buildParagraphViews(
      '第一段 [[data:c1:o2]]。\n\n第二段悬空 [[data:c9:o9]] 与新闻 [[news:31]]。',
      calls,
      calculations,
    )
    expect(paras).toHaveLength(2)
    const p2 = paras[1]!
    const refSegs = p2.filter((s): s is Extract<TextSegmentView, { kind: 'ref' }> => s.kind === 'ref')
    expect(refSegs.some(s => s.view === null)).toBe(true) // 悬空占位
    const news = refSegs.find(s => s.view?.token.type === 'news')
    expect(news?.view?.clickable).toBe(false)
    expect(news?.view?.label).toBe('新闻依据 #31')
  })

  it('appendixAnchorId：冒号/特殊字符清洗为安全 DOM id', () => {
    expect(appendixAnchorId('c1:o2')).toBe('signal-app-c1-o2')
  })
})

describe('signalReport 纯函数 — 图表数据变换（FE-8，6.T3）', () => {
  it('buildSignalChartData：折线点按期间升序、null 断点保留不补零', () => {
    const data = buildSignalChartData(
      { chart_id: 'ch1', kind: 'line', claim: '库存', refs: ['c1:o2', 'c1:o1', 'c1:o3'] },
      calls,
      calculations,
    )
    expect(data.points.map(p => p.period)).toEqual(['2026-09-04', '2026-09-11', '2026-09-11'])
    expect(data.points[2]!.value).toBeNull() // 缺失保留 null，不补 0
    expect(data.unit).toBe('MMbbl')
    expect(data.mixed).toBe(false)
  })

  it('buildSignalChartData：计算点 origin=calc；悬空 ref 跳过', () => {
    const data = buildSignalChartData(
      { chart_id: 'ch2', kind: 'comparison', claim: '净进口', refs: ['k1', 'c9:o9'] },
      calls,
      calculations,
    )
    expect(data.points).toHaveLength(1)
    expect(data.points[0]!.origin).toBe('calc')
    expect(data.points[0]!.display).toBe('-1')
  })

  it('splitLineSegments：null 切段——不跨缺失连线；全 null 得空段', () => {
    const pts: SignalChartPoint[] = [
      { ref: 'a', period: '1', value: 1, display: '1', unit: '', origin: 'data', seriesLabel: '' },
      { ref: 'b', period: '2', value: null, display: '缺失', unit: '', origin: 'data', seriesLabel: '' },
      { ref: 'c', period: '3', value: 3, display: '3', unit: '', origin: 'data', seriesLabel: '' },
    ]
    const segs = splitLineSegments(pts)
    expect(segs).toHaveLength(2)
    expect(segs[0]).toHaveLength(1)
    expect(segs[1]).toHaveLength(1)
    expect(splitLineSegments([{ ...pts[0]!, value: null }])).toHaveLength(0)
  })
})

describe('signalReport 纯函数 — 状态/周期/回顾（FE-9，6.T3）', () => {
  it('candidateStatusLabel / directionLabel / validateSignalPeriodFormat', () => {
    expect(candidateStatusLabel('pending')).toBe('待研究')
    expect(candidateStatusLabel('researching')).toBe('研究中')
    expect(candidateStatusLabel('reported')).toBe('已有报告')
    expect(directionLabel('conditional')).toBe('看条件')
    expect(directionLabel('up')).toBe('向上')
    expect(validateSignalPeriodFormat('month', '2026-09')).toBe(true)
    expect(validateSignalPeriodFormat('month', '2026-13')).toBe(false)
    expect(validateSignalPeriodFormat('year', '2026')).toBe(true)
    expect(validateSignalPeriodFormat('year', '2026-09')).toBe(false)
  })

  it('isRetrospectiveReport：generation_meta 或 snapshot 任一 retrospective 即标回顾', () => {
    expect(isRetrospectiveReport(makeReport().sectors)).toBe(true)
    const current = makeReport({ analysisMode: 'current' })
    expect(isRetrospectiveReport(current.sectors)).toBe(false)
  })

  it('findSection：按 kind 取段；缺段返回 null 不崩', () => {
    const payload = makeReport().sectors
    expect(findSection(payload, 'thesis')?.text).toContain('先别')
    expect(findSection({ report: { title: '', sections: [], charts: [] } } as never, 'facts')).toBeNull()
  })

  it('附录表行构建：观测全集（含未引用行）+ 缺失标记 + 计算输入/公式', () => {
    const obsRows = buildAppendixObsRows(calls)
    expect(obsRows).toHaveLength(5) // 3+2 全集，不做正文引用过滤（50 点不截口径的渲染半边）
    expect(obsRows.find(r => r.ref === 'c1:o3')?.display).toBe('缺失')
    const calcRows = buildAppendixCalcRows(calculations)
    expect(calcRows[0]!.inputs).toEqual(['c1:o2', 'c1:o1'])
    expect(calcRows[0]!.display).toBe('-1')
  })
})

// ── 组件渲染（FE-6~10）───────────────────────────────────────────────────

describe('SignalReportView 渲染（FE-6/7/9/10）', () => {
  it('FE-6：meta 展示真实取数/计算/决策计数（generation_meta，非模型自报）', () => {
    const { wrapper } = mountView(makeReport())
    expect(wrapper.text()).toContain('取数 2 次')
    expect(wrapper.text()).toContain('计算 4 次')
    expect(wrapper.text()).toContain('决策 17/40 轮')
    wrapper.unmount()
  })

  it('FE-7：[[data]]/[[calc]] 渲染为 appendix 数值+单位的引用按钮（模型文本里的 token 不裸露）', () => {
    const { wrapper } = mountView(makeReport())
    const refs = wrapper.findAll('button.sr-ref')
    const labels = refs.map(r => r.text())
    expect(labels).toContain('419.0 MMbbl')
    expect(labels).toContain('-1 MMbbl')
    expect(labels).toContain('-0.2381 %')
    // token 原文不出现在正文
    expect(wrapper.text()).not.toContain('[[data:c1:o2]]')
    // news 引用：非按钮原地展示
    expect(wrapper.find('.sr-ref--news').exists()).toBe(true)
    expect(wrapper.find('.sr-ref--news').text()).toBe('新闻依据 #31')
    wrapper.unmount()
  })

  it('FE-7：点击引用展开附录并高亮对应行（data 观测行 / calc 计算行）', async () => {
    const { wrapper } = mountView(makeReport())
    expect(wrapper.find('[data-testid="signal-appendix"]').attributes('open')).toBeUndefined()
    const ref = wrapper.findAll('button.sr-ref').find(b => b.text() === '419.0 MMbbl')!
    await ref.trigger('click')
    await nextTick()
    const appendix = wrapper.find('[data-testid="signal-appendix"]')
    expect(appendix.attributes('open')).toBeDefined()
    const row = wrapper.find(`#${appendixAnchorId('c1:o2')}`)
    expect(row.exists()).toBe(true)
    expect(row.classes()).toContain('sr-row-target')
    // 附录观测表有全集 + 计算表有公式/输入/精度
    expect(appendix.text()).toContain('代码计算账本')
    expect(appendix.text()).toContain('c1:o2 − c1:o1')
    wrapper.unmount()
  })

  it('FE-7：悬空引用显示占位且不崩（后端已校验，前端兜底）', () => {
    const { wrapper } = mountView(makeReport({ danglingRef: true }))
    expect(wrapper.find('.sr-ref--missing').exists()).toBe(true)
    expect(wrapper.find('.sr-ref--missing').text()).toBe('数据引用缺失')
    wrapper.unmount()
  })

  it('FE-8：单图渲染 svg+折线 path+figcaption 单位与来源标记', () => {
    const { wrapper } = mountView(makeReport())
    const charts = wrapper.find('[data-testid="signal-charts"]')
    expect(charts.findAll('svg')).toHaveLength(1)
    expect(charts.find('path.sr-chart-line').exists()).toBe(true)
    expect(charts.find('figcaption').text()).toContain('单位：MMbbl')
    wrapper.unmount()
  })

  it('FE-8：0 图合法——不渲染图表区块；budget_exhausted 显示缺口提示', () => {
    const { wrapper } = mountView(makeReport({ charts: [], stopReason: 'budget_exhausted' }))
    expect(wrapper.find('[data-testid="signal-charts"]').exists()).toBe(false)
    const notice = wrapper.find('[data-testid="signal-budget-notice"]')
    expect(notice.exists()).toBe(true)
    expect(notice.text()).toContain('预算已用尽')
    expect(notice.text()).toContain('不代表已验证所有问题')
    wrapper.unmount()
  })

  it('FE-9：历史报告标「事后回顾 · 本次数据版本」；返回按钮 emit back', async () => {
    const { wrapper, handlers } = mountView(makeReport())
    expect(wrapper.find('[data-testid="signal-retrospective"]').text()).toContain('事后回顾')
    await wrapper.find('[data-testid="signal-report-back"]').trigger('click')
    expect(handlers.onBack).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('FE-9/5.4：重新研究显式入口 emit regenerate(candidate_id)（regenerate=true 由父级确认）', async () => {
    const { wrapper, handlers } = mountView(makeReport({ candidateId: 31 }))
    const btn = wrapper.find('[data-testid="signal-report-regenerate"]')
    expect(btn.text()).toContain('重新研究')
    await btn.trigger('click')
    expect(handlers.onRegenerate).toHaveBeenCalledTimes(1)
    expect(handlers.onRegenerate).toHaveBeenCalledWith(31)
    wrapper.unmount()
  })

  it('FE-10：全程无 review/judge/采纳/通过/驳回按钮、徽标或 phase（DOM 断言）', () => {
    const { wrapper } = mountView(makeReport())
    const text = wrapper.text()
    for (const banned of ['judge', '审核通过', '驳回', '采纳', '待审批', 'approved']) {
      expect(text.toLowerCase()).not.toContain(banned.toLowerCase())
    }
    // 控件面只有：返回 / 重新研究 / 数据引用 —— 无任何评审类操作按钮
    const buttonTexts = wrapper.findAll('button').map(b => b.text())
    expect(buttonTexts).toEqual(expect.not.arrayContaining(['通过', '驳回', '采纳', '评审通过', '审核']))
    expect(wrapper.find('[class*="review"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid*="review"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('implication 结构化字段展示：verdict/direction/horizon/trigger_condition/self_doubt', () => {
    const { wrapper } = mountView(makeReport())
    expect(wrapper.find('[data-testid="signal-implication"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="signal-direction"]').text()).toBe('看条件')
    expect(wrapper.find('[data-testid="signal-trigger"]').text()).toContain('库存继续减少')
    expect(wrapper.find('[data-testid="signal-self-doubt"]').text()).toContain('偶然变化')
    wrapper.text().includes('未来一个月')
    wrapper.unmount()
  })

  it('安全渲染：LLM 文本中的 HTML 不被解析为元素（插值而非 v-html）', () => {
    const report = makeReport()
    report.sectors.report.sections[0]!.text = '<img src=x onerror=alert(1)>先说结论'
    const { wrapper } = mountView(report)
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.text()).toContain('<img src=x onerror=alert(1)>先说结论')
    wrapper.unmount()
  })

  it('错误态不伪装无数据：detail error 显示错误块（FE-12 详情半边）', () => {
    const { wrapper } = mountView(null, { error: '报告加载失败' })
    expect(wrapper.find('[data-testid="signal-report-error"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="signal-report-error"]').text()).toContain('并非报告不存在')
    wrapper.unmount()
  })

  it('重新研究进行中：内联进度显示，regenerate 按钮禁用', async () => {
    const { wrapper } = mountView(makeReport(), { regenerating: true })
    expect(wrapper.find('[data-testid="signal-regen-progress"]').text()).toContain('重新研究进行中')
    expect((wrapper.find('[data-testid="signal-report-regenerate"]').element as HTMLButtonElement).disabled).toBe(true)
    wrapper.unmount()
  })
})

// ── as-of 机械行（5.5：数据截至 · 研究时点；数据只取已入库 payload）─────────

describe('报告 as-of 机械行（5.5）', () => {
  it('latestObservationPeriod：混杂形态（YYYY-MM-DD/YYYY-MM/YYYY/YYYYMM）规范化取全局最大，WDI year 键回退', () => {
    const mixed: SignalAppendixCall[] = [
      {
        call_id: 'c1', question: 'q', tool: 'eia_wpsr_table1', status: 'ok',
        observations: [
          { observation_id: 'c1:o1', period: '2026-07-15', value: 1 },
          { observation_id: 'c1:o2', period: '2026-06', value: 2 },
        ],
      },
      {
        call_id: 'c2', question: 'q', tool: 'wb_wdi', status: 'ok',
        observations: [{ observation_id: 'c2:o1', year: '2024', value: 3 }], // WDI 无 period 键
      },
      {
        call_id: 'c3', question: 'q', tool: 'un_comtrade_trade', status: 'ok',
        observations: [
          { observation_id: 'c3:o1', period: '202601', value: 4 }, // 字符串序大于 "2026-07-15"，规范化序小于
          { observation_id: 'c3:o2', period: '2026-07', value: 5 },
        ],
      },
    ]
    expect(latestObservationPeriod(mixed)).toBe('2026-07-15')
    // 同月不同日：日破平局（EIA 周度期）
    const intraMonth: SignalAppendixCall[] = [
      {
        call_id: 'c1', question: 'q', tool: 'eia_wpsr_table1', status: 'ok',
        observations: [
          { observation_id: 'c1:o1', period: '2026-09-04', value: 1 },
          { observation_id: 'c1:o2', period: '2026-09-11', value: 2 },
        ],
      },
    ]
    expect(latestObservationPeriod(intraMonth)).toBe('2026-09-11')
  })

  it('latestObservationPeriod：无观测/期间不可解析 → null（渲染层如实写「无可用数据期」）', () => {
    expect(latestObservationPeriod([])).toBeNull()
    const failed: SignalAppendixCall[] = [
      { call_id: 'c1', question: 'q', tool: 'jodi_oil_primary', status: 'error', observations: [] },
    ]
    expect(latestObservationPeriod(failed)).toBeNull()
    const unparseable: SignalAppendixCall[] = [
      { call_id: 'c1', question: 'q', tool: 'jodi_oil_primary', status: 'ok', observations: [{ observation_id: 'c1:o1', period: '未知期' }] },
    ]
    expect(latestObservationPeriod(unparseable)).toBeNull()
  })

  it('标题下渲染 as-of 行：数据截至=观测全局最大期 · 研究时点=generation_meta.cutoff 日期', () => {
    const { wrapper } = mountView(makeReport())
    const asof = wrapper.find('[data-testid="signal-asof"]')
    expect(asof.exists()).toBe(true)
    expect(asof.text()).toBe('数据截至 2026-09-11 · 研究时点 2026-09-22')
    wrapper.unmount()
  })

  it('无任何观测期 → 如实显示「无可用数据期」，研究时点照常', () => {
    const report = makeReport()
    report.sectors.appendix.calls = []
    const { wrapper } = mountView(report)
    const asof = wrapper.find('[data-testid="signal-asof"]')
    expect(asof.text()).toBe('数据截至 无可用数据期 · 研究时点 2026-09-22')
    wrapper.unmount()
  })
})
