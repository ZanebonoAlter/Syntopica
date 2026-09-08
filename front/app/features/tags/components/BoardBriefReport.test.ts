import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import BoardBriefReport from './BoardBriefReport.vue'
import type { BoardAnalysisResultRow, BoardBriefSectors } from '~/api/boardEnrichment'

// 本地 iconify 子集在单测环境未注册，真 Icon 会发起网络请求拉 icon 数据——stub 掉
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

/**
 * 版块简报主视图渲染（board-level-deep-analysis 5.4 / M9）：
 *  - summary / observations / relationships / uncertainties / research_questions / lane_refs
 *  - 无关系 / 0 问题是正常态（直白文案，不是错误）
 *  - lane 引用点击 → drill-lane {laneId, prefill}（prefill = 观察/问题/关系说明）
 *  - 候选问题「深入调查」→ investigate {briefing_result_id, question_id, question}，绝不自动触发
 *  - 自填问题 → investigate {briefing_result_id, question}；空白禁用、>2000 rune 禁用
 */

function makeBrief(sectors: Partial<BoardBriefSectors>): BoardAnalysisResultRow {
  return {
    id: 42,
    analysis_scope: 'board',
    result_kind: 'board_brief',
    sectors: {
      scope: 'board',
      result_kind: 'board_brief',
      summary: '',
      observations: [],
      relationships: [],
      uncertainties: [],
      research_questions: [],
      lane_refs: [],
      ...sectors,
    } as BoardBriefSectors,
    session_id: 'data_enrichment_board_9_ab12cd34',
    created_at: '2026-09-01T10:00:00Z',
  }
}

const fullBrief: Partial<BoardBriefSectors> = {
  summary: '近两周板块内三条泳道热度分化：现货活跃、政策预期降温、外围扰动升温。',
  observations: [
    {
      id: 'o1',
      lane_id: 101,
      statement: '现货侧成交量连续两周放大',
      basis: '新闻记忆（月档）读数',
      as_of_date: '2026-09-01',
    },
    {
      id: 'o2',
      lane_id: 102,
      statement: '政策预期从紧转稳，市场解读降温',
      basis: '新闻记忆（周档）读数',
      as_of_date: '2026-08-30',
    },
  ],
  relationships: [
    {
      lane_ids: [101, 102],
      type: 'divergent',
      explanation: '现货活跃与政策预期降温方向分化，暂无证据表明同一驱动。',
      confidence: 'medium',
      evidence_refs: ['o1', 'o2'],
    },
  ],
  uncertainties: [
    {
      question: '分化会持续还是收敛？',
      why_uncertain: '缺少同期资金流数据',
      needed_evidence: '连续两周的跨泳道资金流序列',
    },
  ],
  research_questions: [
    {
      id: 'q1',
      question: '现货放量是需求驱动还是补库存？',
      rationale: '两种解释对后续价格路径含义相反',
      related_lane_ids: [101],
    },
  ],
  lane_refs: [
    { lane_id: 101, note: '现货端数据源' },
    { lane_id: 102 },
  ],
}

describe('BoardBriefReport', () => {
  it('空 result → 空态提示（引导生成第一份简报，不崩）', () => {
    const w = mount(BoardBriefReport, { props: { result: null } })
    expect(w.text()).toContain('还没有简报')
    expect(w.text()).toContain('生成简报')
  })

  it('loading → 装配中提示', () => {
    const w = mount(BoardBriefReport, { props: { result: null, loading: true } })
    expect(w.text()).toContain('正在装配版块简报')
  })

  it('完整简报：summary / 观察 / 关系 / 不确定项 / 研究问题 / 泳道引用全渲染', () => {
    const w = mount(BoardBriefReport, { props: { result: makeBrief(fullBrief) } })
    const text = w.text()
    // 概览
    expect(text).toContain('近两周板块内三条泳道热度分化')
    // 观察（statement + basis + 截止日）
    expect(text).toContain('现货侧成交量连续两周放大')
    expect(text).toContain('新闻记忆（月档）读数')
    expect(text).toContain('2026-09-01')
    // 关系（类型标签 + 置信 + 解释 + 依据引用）
    expect(text).toContain('方向分化')
    expect(text).toContain('中')
    expect(text).toContain('现货活跃与政策预期降温方向分化')
    // 不确定项
    expect(text).toContain('分化会持续还是收敛？')
    expect(text).toContain('连续两周的跨泳道资金流序列')
    // 研究问题 + 深入调查按钮
    expect(text).toContain('现货放量是需求驱动还是补库存？')
    expect(text).toContain('两种解释对后续价格路径含义相反')
    expect(w.findAll('button').some(b => b.text().includes('深入调查'))).toBe(true)
    // 泳道引用
    expect(text).toContain('泳道引用')
    expect(text).toContain('现货端数据源')
  })

  it('无关系是正常态：直白文案，不渲染错误样式', () => {
    const w = mount(BoardBriefReport, {
      props: { result: makeBrief({ relationships: [] }) },
    })
    expect(w.text()).toContain('当前未发现需要合并解释的关系')
    // 正常态文案不带错误色彩（无 role=alert）
    expect(w.find('[role="alert"]').exists()).toBe(false)
  })

  it('0 问题是正常态：直白文案，自填入口仍可用', () => {
    const w = mount(BoardBriefReport, {
      props: { result: makeBrief({ research_questions: [] }) },
    })
    expect(w.text()).toContain('当前没有值得展开调查的问题')
    // 自填输入 + 按钮仍在（空问题时禁用）
    const input = w.find('textarea')
    expect(input.exists()).toBe(true)
    const btn = w.findAll('button').find(b => b.text().includes('深入调查'))
    expect(btn).toBeDefined()
    expect(btn!.attributes('disabled')).toBeDefined()
  })

  it('观察 lane 引用点击 → drill-lane {laneId, prefill=statement}', async () => {
    const w = mount(BoardBriefReport, { props: { result: makeBrief(fullBrief) } })
    const chip = w.find('[data-test="obs-lane-101"]')
    expect(chip.exists()).toBe(true)
    await chip.trigger('click')
    expect(w.emitted('drill-lane')?.[0]?.[0]).toEqual({
      laneId: 101,
      prefill: '现货侧成交量连续两周放大',
    })
  })

  it('关系 lane 引用点击 → drill-lane {laneId, prefill=explanation}', async () => {
    const w = mount(BoardBriefReport, { props: { result: makeBrief(fullBrief) } })
    const chip = w.find('[data-test="rel-lane-102"]')
    expect(chip.exists()).toBe(true)
    await chip.trigger('click')
    expect(w.emitted('drill-lane')?.[0]?.[0]).toEqual({
      laneId: 102,
      prefill: '现货活跃与政策预期降温方向分化，暂无证据表明同一驱动。',
    })
  })

  it('lane_refs 清单 chip 点击 → drill-lane {laneId, prefill=note}', async () => {
    const w = mount(BoardBriefReport, { props: { result: makeBrief(fullBrief) } })
    const chip = w.find('[data-test="lane-ref-101"]')
    expect(chip.exists()).toBe(true)
    await chip.trigger('click')
    expect(w.emitted('drill-lane')?.[0]?.[0]).toEqual({
      laneId: 101,
      prefill: '现货端数据源',
    })
  })

  it('无 note 的 lane_ref → prefill 空串', async () => {
    const w = mount(BoardBriefReport, { props: { result: makeBrief(fullBrief) } })
    const chip = w.find('[data-test="lane-ref-102"]')
    await chip.trigger('click')
    expect(w.emitted('drill-lane')?.[0]?.[0]).toEqual({ laneId: 102, prefill: '' })
  })

  it('研究问题 lane 引用点击 → drill-lane {laneId, prefill=question}', async () => {
    const w = mount(BoardBriefReport, { props: { result: makeBrief(fullBrief) } })
    const chip = w.find('[data-test="q-lane-101"]')
    expect(chip.exists()).toBe(true)
    await chip.trigger('click')
    expect(w.emitted('drill-lane')?.[0]?.[0]).toEqual({
      laneId: 101,
      prefill: '现货放量是需求驱动还是补库存？',
    })
  })

  it('候选问题「深入调查」→ investigate {briefing_result_id, question_id, question}（不自动触发）', async () => {
    const w = mount(BoardBriefReport, { props: { result: makeBrief(fullBrief) } })
    const btn = w.findAll('[data-test="q-investigate"]').find(b => !b.attributes('disabled'))
    expect(btn).toBeDefined()
    await btn!.trigger('click')
    expect(w.emitted('investigate')).toHaveLength(1)
    expect(w.emitted('investigate')?.[0]?.[0]).toEqual({
      briefing_result_id: 42,
      question_id: 'q1',
      question: '现货放量是需求驱动还是补库存？',
    })
  })

  it('自填问题：合法文本 → investigate {briefing_result_id, question}（无 question_id）', async () => {
    const w = mount(BoardBriefReport, { props: { result: makeBrief(fullBrief) } })
    await w.find('textarea').setValue('  外围扰动会不会传导到现货？  ')
    const btn = w.find('[data-test="custom-investigate"]')
    await btn.trigger('click')
    expect(w.emitted('investigate')?.[0]?.[0]).toEqual({
      briefing_result_id: 42,
      question: '外围扰动会不会传导到现货？',
    })
  })

  it('自填问题：纯空白禁用按钮，点击不 emit', async () => {
    const w = mount(BoardBriefReport, { props: { result: makeBrief(fullBrief) } })
    await w.find('textarea').setValue('   \n\t  ')
    const btn = w.find('[data-test="custom-investigate"]')
    expect(btn.attributes('disabled')).toBeDefined()
    await btn.trigger('click')
    expect(w.emitted('investigate')).toBeUndefined()
  })

  it('自填问题：超过 2000 rune 禁用并提示', async () => {
    const w = mount(BoardBriefReport, { props: { result: makeBrief(fullBrief) } })
    await w.find('textarea').setValue('长'.repeat(2001))
    const btn = w.find('[data-test="custom-investigate"]')
    expect(btn.attributes('disabled')).toBeDefined()
    expect(w.text()).toContain('2001/2000')
    await btn.trigger('click')
    expect(w.emitted('investigate')).toBeUndefined()
  })

  it('自填问题：正好 2000 rune 合法（边界）', async () => {
    const w = mount(BoardBriefReport, { props: { result: makeBrief(fullBrief) } })
    await w.find('textarea').setValue('长'.repeat(2000))
    const btn = w.find('[data-test="custom-investigate"]')
    expect(btn.attributes('disabled')).toBeUndefined()
    await btn.trigger('click')
    expect(w.emitted('investigate')).toHaveLength(1)
  })

  it('all_sparse：素材不足诚实提示，不出研究问题区空壳', () => {
    const w = mount(BoardBriefReport, {
      props: {
        result: makeBrief({
          all_sparse: true,
          summary: '各泳道素材都太稀薄，本轮没有可观察事实。',
          observations: [],
          relationships: [],
          research_questions: [],
        }),
      },
    })
    expect(w.text()).toContain('素材不足')
    expect(w.text()).toContain('各泳道素材都太稀薄')
    expect(w.text()).toContain('当前没有值得展开调查的问题')
  })

  it('机械降级（degraded）：展示降级原因，观察照常渲染', () => {
    const w = mount(BoardBriefReport, {
      props: {
        result: makeBrief({
          degraded: true,
          degraded_why: 'LLM 解析失败两次，已机械降级为只读观察',
          observations: fullBrief.observations,
          relationships: [],
        }),
      },
    })
    expect(w.text()).toContain('LLM 解析失败两次，已机械降级为只读观察')
    expect(w.text()).toContain('现货侧成交量连续两周放大')
  })
})

describe('BoardBriefReport — 调查在跑态（board-level-deep-analysis 5.7 接线）', () => {
  it('investigationRunning：问题按钮/自填提交禁用并显示「正在调查」，点击不 emit', async () => {
    const w = mount(BoardBriefReport, {
      props: { result: makeBrief(fullBrief), investigationRunning: true },
    })
    expect(w.text()).toContain('正在调查')
    const qBtn = w.find('[data-test="q-investigate"]')
    expect(qBtn.attributes('disabled')).toBeDefined()
    await qBtn.trigger('click')
    // 自填合法文本但 running：提交仍禁用
    await w.find('textarea').setValue('外围扰动会不会传导到现货？')
    const customBtn = w.find('[data-test="custom-investigate"]')
    expect(customBtn.attributes('disabled')).toBeDefined()
    await customBtn.trigger('click')
    expect(w.emitted('investigate')).toBeUndefined()
  })

  it('investigationRunning=false（默认）：按钮照常可用', async () => {
    const w = mount(BoardBriefReport, { props: { result: makeBrief(fullBrief) } })
    expect(w.text()).not.toContain('正在调查')
    const qBtn = w.find('[data-test="q-investigate"]')
    expect(qBtn.attributes('disabled')).toBeUndefined()
    await qBtn.trigger('click')
    expect(w.emitted('investigate')).toHaveLength(1)
  })
})
