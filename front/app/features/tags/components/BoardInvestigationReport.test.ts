import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import BoardInvestigationReport from './BoardInvestigationReport.vue'
import type {
  BoardAnalysisResultRow,
  BoardInvestigationSectors,
} from '~/api/boardEnrichment'

// 本地 iconify 子集在单测环境未注册，真 Icon 会发起网络请求拉 icon 数据——stub 掉
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

/**
 * 调查报告视图（board-level-deep-analysis 5.6 / M10）：
 *  - 首屏有限结论：调查问题 + conclusion（summary/confidence/scope/boundary）+
 *    各 hypothesis assessment；支持证据/反证/gap/证据详情折叠逐步展开
 *  - assessment 五态中文标签；允许 H0 最可信、允许全部 insufficient
 *  - 证据 quote 原地展开（不进首屏）；web/page URL 可点击；lane 证据可下钻
 *  - lane emit {laneId, prefill}：prefill = 具体调查问题 + 证据 quote/lane_note +
 *    反证假设说明（长度受控）；幽灵/非法 lane 不 emit
 *  - 不渲染 argument/depth 或重复连续长文（调查 schema 无此字段，塞了也忽略）
 */

function makeInv(
  sectors: Partial<BoardInvestigationSectors>,
): BoardAnalysisResultRow {
  return {
    id: 77,
    analysis_scope: 'board',
    result_kind: 'board_investigation',
    parent_result_id: 42,
    question_key: 'q-key-1',
    sectors: {
      scope: 'board',
      result_kind: 'board_investigation',
      parent_briefing_id: 42,
      question: { id: 'q1', text: '', source: 'generated' },
      hypotheses: [],
      conclusion: {
        summary: '',
        confidence: 'medium',
        scope: '',
        boundary: '',
      },
      evidence_chain: [],
      lane_refs: [],
      ...sectors,
    } as BoardInvestigationSectors,
    session_id: 'data_enrichment_board_9_inv1',
    created_at: '2026-09-02T10:00:00Z',
  }
}

const fullInv: Partial<BoardInvestigationSectors> = {
  question: {
    id: 'q1',
    text: '两条泳道是否由同一资金驱动？',
    source: 'generated',
  },
  hypotheses: [
    {
      id: 'h1',
      label: '产业基金同步注入两条泳道',
      is_null: false,
      derived_from: ['h1'],
      assessment: 'supported',
      confidence: 'medium',
      scope: '两条泳道的资金侧',
      support_evidence: ['e1'],
      counter_evidence: [],
      gaps: ['资金明细未完全核实'],
    },
    {
      id: 'h0',
      label: '两条泳道变化彼此独立，无可下结论的关联',
      is_null: true,
      assessment: 'plausible',
      confidence: 'low',
      scope: '',
      support_evidence: ['e2'],
      counter_evidence: ['e1'],
      gaps: [],
    },
  ],
  conclusion: {
    summary: '产业基金公告支持 h1，但资金明细未完全核实，暂不下定论。',
    confidence: 'medium',
    scope: '两条泳道的资金侧关联',
    boundary: '资金明细未披露前不能确认同源。',
  },
  evidence_chain: [
    {
      id: 'e1',
      source_type: 'web',
      url: 'https://example.com/fund-notice',
      quote: '基金公告原文摘录ABC',
      institution: '示例研究所',
      date: '2026-08-20',
      supports: ['h1'],
      counters: [],
    },
    {
      id: 'e2',
      source_type: 'lane',
      ref: '901',
      lane_note: '产能与招标详情：招标节奏与基金公告同期',
      supports: ['h0'],
      counters: [],
    },
    {
      // 幽灵泳道：ref 不在本份 lane_refs 白名单内
      id: 'e3',
      source_type: 'lane',
      ref: '9999',
      lane_note: '幽灵泳道说明',
      supports: [],
      counters: [],
    },
  ],
  lane_refs: [
    { lane_id: 901, note: '主泳道' },
    { lane_id: 902, note: '对照泳道' },
  ],
}

describe('BoardInvestigationReport — 首屏有限结论', () => {
  it('空 result → 空态提示（引导从简报发起调查，不崩）', () => {
    const w = mount(BoardInvestigationReport, { props: { result: null } })
    expect(w.text()).toContain('还没有调查报告')
  })

  it('loading → 装配中提示', () => {
    const w = mount(BoardInvestigationReport, {
      props: { result: null, loading: true },
    })
    expect(w.text()).toContain('正在装配调查报告')
  })

  it('首屏：调查问题（含来源标签）+ conclusion 四字段 + 各 hypothesis assessment 可见', () => {
    const w = mount(BoardInvestigationReport, {
      props: { result: makeInv(fullInv) },
    })
    const text = w.text()
    // 调查问题 + generated 来源标签 + 候选 id
    expect(text).toContain('两条泳道是否由同一资金驱动？')
    expect(text).toContain('简报候选')
    expect(text).toContain('q1')
    // conclusion 四字段（置信中文）
    expect(text).toContain('产业基金公告支持 h1，但资金明细未完全核实，暂不下定论。')
    expect(text).toContain('两条泳道的资金侧关联')
    expect(text).toContain('资金明细未披露前不能确认同源。')
    expect(text).toContain('中')
    // hypothesis label + assessment
    expect(text).toContain('产业基金同步注入两条泳道')
    expect(text).toContain('证据支持')
    expect(text).toContain('初步可信')
  })

  it('首屏不展开证据细节：quote / lane_note / 机构不在首屏文本中', () => {
    const w = mount(BoardInvestigationReport, {
      props: { result: makeInv(fullInv) },
    })
    const text = w.text()
    expect(text).not.toContain('基金公告原文摘录ABC')
    expect(text).not.toContain('示例研究所')
    expect(text).not.toContain('招标节奏与基金公告同期')
  })

  it('不渲染 argument/depth：sectors 塞入旧字段也忽略（新 kind 绝不进论文式长文）', () => {
    const w = mount(BoardInvestigationReport, {
      props: {
        result: makeInv({
          ...fullInv,
          argument: {
            intro: '旧版论文引言不应出现在调查报告',
            layers: [],
            boundary: '',
            conclusion: { cert: 'high', judgment: '' },
          },
          depth: {
            system_reframe: '旧版系统重定位不应出现',
            mechanism_layers: [],
            historical_analogy: [],
            boundary: '',
            evidence_chain: [],
          },
        } as Partial<BoardInvestigationSectors> & Record<string, unknown>),
      },
    })
    expect(w.text()).not.toContain('旧版论文引言不应出现在调查报告')
    expect(w.text()).not.toContain('旧版系统重定位不应出现')
  })
})

describe('BoardInvestigationReport — hypothesis assessment', () => {
  it('五态标签各渲染正确（supported/plausible/insufficient/weakened/refuted）', () => {
    const five = [
      ['a1', 'supported', '证据支持'],
      ['a2', 'plausible', '初步可信'],
      ['a3', 'insufficient', '证据不足'],
      ['a4', 'weakened', '被削弱'],
      ['a5', 'refuted', '被推翻'],
    ] as const
    const w = mount(BoardInvestigationReport, {
      props: {
        result: makeInv({
          ...fullInv,
          hypotheses: five.map(([id, assessment]) => ({
            id,
            label: `假设${id}`,
            assessment,
            confidence: 'low',
            support_evidence: [],
            counter_evidence: [],
            gaps: [],
          })),
        }),
      },
    })
    for (const label of five.map(f => f[2])) {
      expect(w.text()).toContain(label)
    }
  })

  it('H0 标记「零假设」，允许 H0 最可信（assessment=supported 正常渲染）', () => {
    const w = mount(BoardInvestigationReport, {
      props: {
        result: makeInv({
          ...fullInv,
          hypotheses: [
            {
              id: 'h0',
              label: '变化彼此独立',
              is_null: true,
              assessment: 'supported',
              confidence: 'high',
              support_evidence: ['e2'],
              counter_evidence: [],
              gaps: [],
            },
          ],
        }),
      },
    })
    expect(w.text()).toContain('零假设')
    expect(w.text()).toContain('证据支持')
  })

  it('全部 insufficient 是正常态：无错误样式（无 role=alert）', () => {
    const w = mount(BoardInvestigationReport, {
      props: {
        result: makeInv({
          ...fullInv,
          hypotheses: [
            {
              id: 'h1',
              label: '假设一',
              assessment: 'insufficient',
              confidence: 'low',
              support_evidence: [],
              counter_evidence: [],
              gaps: ['缺数据'],
            },
            {
              id: 'h0',
              label: '零假设',
              is_null: true,
              assessment: 'insufficient',
              confidence: 'low',
              support_evidence: [],
              counter_evidence: [],
              gaps: [],
            },
          ],
        }),
      },
    })
    expect(w.text()).toContain('证据不足')
    expect(w.find('[role="alert"]').exists()).toBe(false)
  })

  it('支持证据 / 反证 / 缺口 分区独立：展开 hypothesis 后三类各自可见', async () => {
    const w = mount(BoardInvestigationReport, {
      props: { result: makeInv(fullInv) },
    })
    const toggle = w.find('[data-test="hyp-toggle-h0"]')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    const body = w.text()
    // 反证引用（e1）与支持引用（e2）分开列出
    expect(body).toContain('反证')
    expect(body).toContain('e1')
    expect(body).toContain('支持证据')
    expect(body).toContain('e2')
    // gap 分区（h1 的 gap 在展开后可见）
    const t1 = w.find('[data-test="hyp-toggle-h1"]')
    await t1.trigger('click')
    expect(w.text()).toContain('资金明细未完全核实')
  })

  it('悬空证据引用不崩：support_evidence 指向不存在的 id 原样显示', async () => {
    const w = mount(BoardInvestigationReport, {
      props: {
        result: makeInv({
          ...fullInv,
          hypotheses: [
            {
              id: 'h1',
              label: '悬空引用假设',
              assessment: 'plausible',
              confidence: 'low',
              support_evidence: ['eX-not-exist'],
              counter_evidence: [],
              gaps: [],
            },
          ],
        }),
      },
    })
    await w.find('[data-test="hyp-toggle-h1"]').trigger('click')
    expect(w.text()).toContain('eX-not-exist')
  })

  it('空 gaps / 空证据引用是正常态：不渲染错误文案', async () => {
    const w = mount(BoardInvestigationReport, {
      props: {
        result: makeInv({
          ...fullInv,
          hypotheses: [
            {
              id: 'h0',
              label: '零假设',
              is_null: true,
              assessment: 'insufficient',
              confidence: 'low',
              support_evidence: [],
              counter_evidence: [],
              gaps: [],
            },
          ],
        }),
      },
    })
    await w.find('[data-test="hyp-toggle-h0"]').trigger('click')
    expect(w.find('[role="alert"]').exists()).toBe(false)
  })
})

describe('BoardInvestigationReport — 证据台账', () => {
  it('证据展开：quote/institution/date 渲染，web URL 是可点击链接', async () => {
    const w = mount(BoardInvestigationReport, {
      props: { result: makeInv(fullInv) },
    })
    const toggle = w.find('[data-test="ev-toggle-e1"]')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    await toggle.trigger('click')
    expect(w.text()).toContain('基金公告原文摘录ABC')
    expect(w.text()).toContain('示例研究所')
    expect(w.text()).toContain('2026-08-20')
    const link = w.find('.bi-ev a[href="https://example.com/fund-notice"]')
    expect(link.exists()).toBe(true)
    expect(link.attributes('target')).toBe('_blank')
    expect(link.attributes('rel')).toContain('noopener')
  })

  it('page 原文页证据同样可展开 + URL 可点击', async () => {
    const w = mount(BoardInvestigationReport, {
      props: {
        result: makeInv({
          ...fullInv,
          evidence_chain: [
            {
              id: 'p1',
              source_type: 'page',
              url: 'https://example.org/report.pdf',
              quote: '报告原文逐字摘录',
              institution: '官方统计',
              date: '2026-07-01',
              supports: [],
              counters: ['h1'],
            },
          ],
        }),
      },
    })
    await w.find('[data-test="ev-toggle-p1"]').trigger('click')
    expect(w.text()).toContain('报告原文逐字摘录')
    expect(w.find('.bi-ev a[href="https://example.org/report.pdf"]').exists()).toBe(true)
  })

  it('空证据链是正常态：准确中性文案（没有通过核验、可展示的证据），不归因采材失败，不渲染错误', () => {
    const w = mount(BoardInvestigationReport, {
      props: { result: makeInv({ ...fullInv, evidence_chain: [] }) },
    })
    expect(w.text()).toContain('没有通过核验、可展示的证据')
    expect(w.text()).not.toContain('没有采到可用材料')
    expect(w.find('[role="alert"]').exists()).toBe(false)
  })
})

describe('BoardInvestigationReport — lane 下钻', () => {
  it('lane 证据下钻 → drill-lane {laneId, prefill}：prefill 含调查问题 + lane_note + 反证假设说明', async () => {
    // e2 是 h0 的支持证据、同时是 h1 的……构造反证场景：让 e2 counters 指向 h1
    const w = mount(BoardInvestigationReport, {
      props: {
        result: makeInv({
          ...fullInv,
          evidence_chain: [
            {
              ...fullInv.evidence_chain![1]!,
              counters: ['h1'],
            },
          ],
        }),
      },
    })
    await w.find('[data-test="ev-toggle-e2"]').trigger('click')
    const chip = w.find('[data-test="ev-lane-901"]')
    expect(chip.exists()).toBe(true)
    await chip.trigger('click')
    const emitted = w.emitted('drill-lane')
    expect(emitted).toHaveLength(1)
    const payload = emitted![0]![0] as { laneId: number; prefill: string }
    expect(payload.laneId).toBe(901)
    // 具体调查问题
    expect(payload.prefill).toContain('两条泳道是否由同一资金驱动？')
    // lane_note（可用信息组合）
    expect(payload.prefill).toContain('招标节奏与基金公告同期')
    // 反证假设说明（被该证据反对的假设 label + 评估）
    expect(payload.prefill).toContain('产业基金同步注入两条泳道')
    // 不是抽象 lens / 结论复述
    expect(payload.prefill).not.toContain('产业基金公告支持 h1')
  })

  it('quote 参与组合：lane 证据带 quote 时 prefill 含摘录片段', async () => {
    const w = mount(BoardInvestigationReport, {
      props: {
        result: makeInv({
          ...fullInv,
          evidence_chain: [
            {
              id: 'e9',
              source_type: 'lane',
              ref: '902',
              quote: '泳道内部事实逐字摘录XYZ',
              lane_note: '',
              supports: [],
              counters: [],
            },
          ],
        }),
      },
    })
    await w.find('[data-test="ev-toggle-e9"]').trigger('click')
    await w.find('[data-test="ev-lane-902"]').trigger('click')
    const payload = w.emitted('drill-lane')![0]![0] as { laneId: number; prefill: string }
    expect(payload.laneId).toBe(902)
    expect(payload.prefill).toContain('泳道内部事实逐字摘录XYZ')
  })

  it('幽灵 lane（ref 不在 lane_refs）不渲染下钻入口、不 emit', async () => {
    const w = mount(BoardInvestigationReport, {
      props: { result: makeInv(fullInv) },
    })
    await w.find('[data-test="ev-toggle-e3"]').trigger('click')
    // 幽灵泳道证据展开后没有可点的 lane chip
    expect(w.find('[data-test="ev-lane-9999"]').exists()).toBe(false)
    expect(w.emitted('drill-lane')).toBeUndefined()
  })

  it('非法 lane ref（非数字）不 emit', async () => {
    const w = mount(BoardInvestigationReport, {
      props: {
        result: makeInv({
          ...fullInv,
          evidence_chain: [
            { id: 'ebad', source_type: 'lane', ref: 'not-a-number', supports: [], counters: [] },
          ],
        }),
      },
    })
    await w.find('[data-test="ev-toggle-ebad"]').trigger('click')
    expect(w.find('[data-test^="ev-lane-"]').exists()).toBe(false)
    expect(w.emitted('drill-lane')).toBeUndefined()
  })

  it('prefill 长度受控：超长 quote/假设说明截断（≤400 rune）', async () => {
    const w = mount(BoardInvestigationReport, {
      props: {
        result: makeInv({
          ...fullInv,
          hypotheses: [
            {
              id: 'h1',
              label: '超'.repeat(600),
              assessment: 'weakened',
              confidence: 'low',
              support_evidence: [],
              counter_evidence: [],
              gaps: [],
            },
          ],
          evidence_chain: [
            {
              id: 'elong',
              source_type: 'lane',
              ref: '901',
              quote: '长'.repeat(600),
              lane_note: '注'.repeat(300),
              supports: [],
              counters: ['h1'],
            },
          ],
        }),
      },
    })
    await w.find('[data-test="ev-toggle-elong"]').trigger('click')
    await w.find('[data-test="ev-lane-901"]').trigger('click')
    const payload = w.emitted('drill-lane')![0]![0] as { laneId: number; prefill: string }
    expect(Array.from(payload.prefill).length).toBeLessThanOrEqual(400)
    expect(payload.prefill).toContain('两条泳道是否由同一资金驱动？')
  })
})

describe('BoardInvestigationReport — a11y 与杂项', () => {
  it('来源标签：custom 自填问题显示「自填」', () => {
    const w = mount(BoardInvestigationReport, {
      props: {
        result: makeInv({
          ...fullInv,
          question: { text: '自填的问题', source: 'custom' },
        }),
      },
    })
    expect(w.text()).toContain('自填')
    expect(w.text()).toContain('自填的问题')
  })

  it('method_refs 留痕以文本呈现（审计可见，不冒充证据）', () => {
    const w = mount(BoardInvestigationReport, {
      props: {
        result: makeInv({
          ...fullInv,
          method_refs: [{ id: 11, title: '内部看美国·对比检验', content_hash: 'ab12' }],
        }),
      },
    })
    expect(w.text()).toContain('内部看美国·对比检验')
  })

  it('父简报溯源：显示 parent_briefing_id', () => {
    const w = mount(BoardInvestigationReport, {
      props: { result: makeInv(fullInv) },
    })
    expect(w.text()).toContain('42')
  })
})

describe('BoardInvestigationReport — 跨版块泳道引用（2.4）', () => {
  const crossInv: Partial<BoardInvestigationSectors> = {
    question: { id: 'q1', text: '外部驱动是否作用于本板块？', source: 'custom' },
    hypotheses: [],
    conclusion: { summary: 's', confidence: 'medium', scope: '', boundary: '' },
    evidence_chain: [
      {
        id: 'e1',
        source_type: 'lane',
        ref: '601',
        note: '对方板块泳道事实',
        supports: [],
        counters: [],
      },
      {
        id: 'e2',
        source_type: 'lane',
        ref: '901',
        note: '本板块泳道事实',
        supports: [],
        counters: [],
      },
    ],
    lane_refs: [
      { lane_id: 601, board_id: 9902, note: '经动态授权引用' },
      { lane_id: 901, note: '本板块泳道' },
    ],
  }

  it('引用泳道清单：跨版块标注版块号并可跳转，本版块泳道正常展示', async () => {
    const w = mount(BoardInvestigationReport, { props: { result: makeInv(crossInv) } })
    expect(w.find('[data-test="inv-lane-refs"]').exists()).toBe(true)
    const cross = w.find('[data-test="inv-laneref-601"]')
    expect(cross.text()).toContain('版块 #9902')
    expect(cross.text()).toContain('泳道 #601')
    expect(cross.text()).toContain('经动态授权引用')
    const local = w.find('[data-test="inv-laneref-901"]')
    expect(local.text()).toContain('泳道 #901')
    expect(local.text()).not.toContain('版块 #')

    await w.find('[data-test="inv-laneref-open-601"]').trigger('click')
    expect(w.emitted('open-board')).toEqual([[9902]])
  })

  it('证据 chip：跨版块泳道显示「版块 #N · 泳道 #M」且点击跳对方版块；本板块照旧 drill-lane', async () => {
    const w = mount(BoardInvestigationReport, { props: { result: makeInv(crossInv) } })
    // 证据台账默认折叠：先展开两条证据
    await w.find('[data-test="ev-toggle-e1"]').trigger('click')
    await w.find('[data-test="ev-toggle-e2"]').trigger('click')
    const crossChip = w.find('[data-test="ev-lane-601"]')
    expect(crossChip.text()).toContain('版块 #9902')
    expect(crossChip.classes()).toContain('bi-lane-cross')
    await crossChip.trigger('click')
    expect(w.emitted('open-board')).toEqual([[9902]])
    expect(w.emitted('drill-lane')).toBeFalsy()

    const localChip = w.find('[data-test="ev-lane-901"]')
    expect(localChip.text()).toContain('下钻核查')
    expect(localChip.classes()).not.toContain('bi-lane-cross')
    await localChip.trigger('click')
    expect(w.emitted('drill-lane')).toBeTruthy()
    expect(w.emitted('open-board')).toHaveLength(1)
  })

  it('旧报告 lane_refs 无 board_id：清单正常渲染，本版块语义不受影响', async () => {
    const legacy: Partial<BoardInvestigationSectors> = {
      ...crossInv,
      lane_refs: [
        { lane_id: 601, note: '旧数据无版块归属' },
        { lane_id: 901 },
      ],
    }
    const w = mount(BoardInvestigationReport, { props: { result: makeInv(legacy) } })
    expect(w.find('[data-test="inv-lane-refs"]').exists()).toBe(true)
    expect(w.find('[data-test="inv-laneref-601"]').text()).toContain('旧数据无版块归属')
    // 无跨版块标记 → 证据 chip 仍是本板块下钻语义
    await w.find('[data-test="ev-toggle-e1"]').trigger('click')
    await w.find('[data-test="ev-lane-601"]').trigger('click')
    expect(w.emitted('drill-lane')).toBeTruthy()
    expect(w.emitted('open-board')).toBeFalsy()
  })

  it('空 lane_refs：清单区不渲染（正常态）', () => {
    const w = mount(BoardInvestigationReport, { props: { result: makeInv({}) } })
    expect(w.find('[data-test="inv-lane-refs"]').exists()).toBe(false)
  })
})
