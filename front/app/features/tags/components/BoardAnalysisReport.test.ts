import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import BoardAnalysisReport from './BoardAnalysisReport.vue'
import type { BoardAnalysisResultRow } from '~/api/boardEnrichment'

/**
 * 版块级分析报告渲染（board-level-deep-analysis 4.3 / M8）：
 *  - scope=board 五字段的论文式渲染（命题/论证层/深度层/证据链/泳道引用）
 *  - sparse 档诚实降级（不渲染论证骨架，显示素材不足提示）
 *  - lane 引用点击 → drill-lane payload（laneId + 预填 lens）
 *  - 旧格式（无 argument/depth）降级不崩
 */

function makeResult(sectors: Record<string, unknown>): BoardAnalysisResultRow {
  return {
    id: 42,
    analysis_scope: 'board',
    sectors: { scope: 'board', form: 'board', thesis: '', candidates: [], chosen_index: 0, lane_refs: [], ...sectors } as BoardAnalysisResultRow['sectors'],
    session_id: 'data_enrichment_board_9_ab12cd34',
    created_at: '2026-08-26T10:00:00Z',
  }
}

const fullSectors = {
  thesis: '美联储降息路径与板块内三条泳道的传导错位',
  angle: '供需错配',
  candidates: [
    { thesis: '候选甲', hook: '钩子甲', angle: '切角甲' },
    { thesis: '候选乙', hook: '钩子乙', angle: '切角乙' },
  ],
  chosen_index: 1,
  reason: '候选乙覆盖泳道更全',
  argument: {
    intro: '开篇定调：从近两周的异常升维到结构命题。',
    layers: [
      { layer: '表层现象', deep_logic: '三条泳道同向异速。', basis: '态势卡#1' },
      { layer: '传导机制', deep_logic: '利率预期先行，现货滞后。', basis: 'agent 检索' },
    ],
    boundary: '还不能确认传导是否已闭环。',
    conclusion: { cert: 'medium', judgment: '错位仍在扩大，维持观察。' },
  },
  depth: {
    system_reframe: '放进全球美元流动性大系统讲。',
    mechanism_layers: [{ layer: '机制甲', deep_logic: '深层逻辑甲', basis: '依据甲' }],
    historical_analogy: [{ case: '2019 降息周期', mechanism: '同类错位', diff: '本次更快' }],
    regime_shift: null,
    boundary: '数据未覆盖衍生品市场。',
    evidence_chain: [
      { source_type: 'web', url: 'https://example.com/report', quote: '原文摘录一句', institution: 'BIS', date: '2026-08', kind: 'quote' },
      { source_type: 'lane', ref: '7', quote: '泳道七贡献了现货数据', lane_note: '现货端' },
    ],
  },
  lane_refs: [{ lane_id: 7, note: '现货端数据源' }],
}

describe('BoardAnalysisReport', () => {
  it('空 result → 空态提示（不崩）', () => {
    const w = mount(BoardAnalysisReport, { props: { result: null } })
    expect(w.text()).toContain('还没有版块级分析报告')
  })

  it('loading → 装配中提示', () => {
    const w = mount(BoardAnalysisReport, { props: { result: null, loading: true } })
    expect(w.text()).toContain('正在装配版块分析报告')
  })

  it('完整报告：五字段全部渲染（命题/论证层/深度层/证据链/泳道引用）', () => {
    const w = mount(BoardAnalysisReport, { props: { result: makeResult(fullSectors) } })
    const text = w.text()
    // 命题 + 切角 + 候选
    expect(text).toContain('美联储降息路径与板块内三条泳道的传导错位')
    expect(text).toContain('切角：供需错配')
    expect(text).toContain('候选甲')
    expect(text).toContain('候选乙')
    // 论证骨架（层级递进）
    expect(text).toContain('开篇定调')
    expect(text).toContain('表层现象')
    expect(text).toContain('传导机制')
    expect(text).toContain('还不能确认传导是否已闭环')
    expect(text).toContain('错位仍在扩大')
    // 深度层
    expect(text).toContain('全球美元流动性大系统')
    expect(text).toContain('2019 降息周期')
    expect(text).toContain('机制层拆解')
    // 证据链 + 泳道引用
    expect(text).toContain('证据链（2 条）')
    expect(text).toContain('原文摘录一句')
    expect(text).toContain('泳道引用（1 条）')
    // 选中候选高亮标记存在
    expect(w.find('.ba-cand.chosen').exists()).toBe(true)
    // 确定性分级
    expect(text).toContain('确定性 · 中')
  })

  it('sparse 档：诚实降级，不渲染论证骨架', () => {
    const w = mount(BoardAnalysisReport, {
      props: { result: makeResult({ form: 'sparse', thesis: '素材不足的骨架命题' }) },
    })
    const text = w.text()
    expect(text).toContain('素材不足')
    expect(text).toContain('骨架命题')
    expect(text).not.toContain('机制层拆解')
    expect(text).not.toContain('证据链')
  })

  it('旧格式（无 argument/depth）：降级提示不崩', () => {
    const w = mount(BoardAnalysisReport, {
      props: { result: makeResult({ argument: undefined, depth: undefined }) },
    })
    expect(w.text()).toContain('旧格式')
  })

  it('lane 点击 → drill-lane payload（laneId + 预填 lens）', async () => {
    const w = mount(BoardAnalysisReport, { props: { result: makeResult(fullSectors) } })
    // 泳道引用清单 chip
    const chip = w.find('.ba-lane-chip')
    expect(chip.exists()).toBe(true)
    await chip.trigger('click')
    expect(w.emitted('drill-lane')?.[0]?.[0]).toEqual({ laneId: 7, lens: '现货端数据源' })
    // 证据链里的 lane 证据也可点
    const laneEv = w.find('button.ba-ev-drill')
    expect(laneEv.exists()).toBe(true)
    await laneEv.trigger('click')
    const events = w.emitted('drill-lane')
    expect(events).toHaveLength(2)
    expect(events?.[1]?.[0]).toMatchObject({ laneId: 7 })
  })

  it('web 证据渲染可点击链接（url + institution + kind 标签）', () => {
    const w = mount(BoardAnalysisReport, { props: { result: makeResult(fullSectors) } })
    const link = w.find('a.ba-ev-quote')
    expect(link.attributes('href')).toBe('https://example.com/report')
    expect(link.text()).toContain('原文摘录一句')
    expect(w.text()).toContain('BIS')
    expect(w.text()).toContain('原文摘录')
  })
})
