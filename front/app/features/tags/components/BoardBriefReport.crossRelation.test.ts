import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import BoardBriefReport from './BoardBriefReport.vue'
import type { BoardAnalysisResultRow, BoardBriefCrossRelation, BoardBriefSectors } from '~/api/boardEnrichment'

// 本地 iconify 子集在单测环境未注册，stub 掉（同 BoardBriefReport.test.ts）
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

/**
 * 已确认跨版块关系分区（add-evidence-backed-cross-board-relations 5.3）：
 *  - 有效关系：方向/类型/质量徽标 + claim + 证据外链 + 版块跳转 emit
 *  - 无关系（含旧简报缺字段）：分区不渲染，不是错误
 *  - 过期/未确认：由后端排除，前端只信任机械字段（不在前端再过滤）
 *  - 超长 claim：折叠 + 展开全文
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

const crossOutgoing: BoardBriefCrossRelation = {
  relation_id: 11,
  other_board_id: 9902,
  direction: 'outgoing',
  relation_type: 'causal',
  claim: '日债收益率走高经避险资金传导',
  quality_grade: 'medium',
  confirmed_at: '2026-08-31',
  expires_at: '2026-10-01',
  evidence_url: 'https://example.com/r1',
  evidence_quote: '原始网页摘录：十年期日债收益率触及新高',
}

const crossIncoming: BoardBriefCrossRelation = {
  relation_id: 12,
  other_board_id: 9901,
  direction: 'incoming',
  relation_type: 'common_driver',
  claim: '与中东局势共享同一避险驱动',
  quality_grade: 'high',
}

describe('BoardBriefReport — 已确认跨版块关系分区（5.3）', () => {
  it('有效关系：分区渲染方向/类型/质量徽标、claim、确认/有效期、证据外链、摘录', () => {
    const w = mount(BoardBriefReport, {
      props: { result: makeBrief({ cross_board_relations: [crossOutgoing, crossIncoming] }) },
    })
    expect(w.find('[data-test="cross-relations-section"]').exists()).toBe(true)
    expect(w.find('[data-test="cross-relation-11"]').exists()).toBe(true)
    expect(w.find('[data-test="cross-relation-12"]').exists()).toBe(true)
    expect(w.find('[data-test="cross-dir-11"]').text()).toBe('传出')
    expect(w.find('[data-test="cross-dir-12"]').text()).toBe('传入')
    expect(w.find('[data-test="cross-relation-11"]').text()).toContain('因果传导')
    expect(w.find('[data-test="cross-relation-11"]').text()).toContain('证据中等')
    expect(w.find('[data-test="cross-relation-12"]').text()).toContain('证据充分')
    expect(w.find('[data-test="cross-claim-11"]').text()).toContain('日债收益率走高经避险资金传导')
    expect(w.find('[data-test="cross-relation-11"]').text()).toContain('确认于 2026-08-31')
    expect(w.find('[data-test="cross-relation-11"]').text()).toContain('有效期至 2026-10-01')
    expect(w.find('[data-test="cross-relation-11"]').text()).toContain('原始网页摘录：十年期日债收益率触及新高')

    const evidence = w.find<HTMLAnchorElement>('[data-test="cross-evidence-11"]')
    expect(evidence.exists()).toBe(true)
    expect(evidence.attributes('href')).toBe('https://example.com/r1')
    expect(evidence.attributes('target')).toBe('_blank')
    // incoming 无证据 URL：不渲染外链
    expect(w.find('[data-test="cross-evidence-12"]').exists()).toBe(false)
  })

  it('版块跳转按钮点击 → open-board {boardId}', async () => {
    const w = mount(BoardBriefReport, {
      props: { result: makeBrief({ cross_board_relations: [crossOutgoing] }) },
    })
    await w.find('[data-test="cross-open-board-9902"]').trigger('click')
    expect(w.emitted('open-board')).toEqual([[9902]])
  })

  it('无关系：分区不渲染（正常态，非错误）', () => {
    const w = mount(BoardBriefReport, { props: { result: makeBrief({}) } })
    expect(w.find('[data-test="cross-relations-section"]').exists()).toBe(false)
  })

  it('旧简报缺 cross_board_relations 字段 → 降级为空，分区不渲染不崩', () => {
    const legacy = makeBrief({})
    delete (legacy.sectors as Partial<BoardBriefSectors>).cross_board_relations
    const w = mount(BoardBriefReport, { props: { result: legacy } })
    expect(w.find('[data-test="cross-relations-section"]').exists()).toBe(false)
    // 主结构照常
    expect(w.text()).toContain('跨泳道关系')
  })

  it('过期关系由后端排除：前端不过滤，只渲染机械字段（空数组=无关系）', () => {
    // 契约：后端 LoadActive 只返回未过期；前端收到什么渲染什么（空=无）
    const w = mount(BoardBriefReport, { props: { result: makeBrief({ cross_board_relations: [] }) } })
    expect(w.find('[data-test="cross-relations-section"]').exists()).toBe(false)
  })

  it('超长 claim：默认截断 + 展开/收起', async () => {
    const longClaim = '超'.repeat(200)
    const rel: BoardBriefCrossRelation = { ...crossOutgoing, relation_id: 13, claim: longClaim }
    const w = mount(BoardBriefReport, { props: { result: makeBrief({ cross_board_relations: [rel] }) } })

    const claimEl = w.find('[data-test="cross-claim-13"]')
    expect(claimEl.text().length).toBeLessThan(200)
    expect(claimEl.text().endsWith('…')).toBe(true)

    await w.find('[data-test="cross-toggle-13"]').trigger('click')
    expect(w.find('[data-test="cross-claim-13"]').text()).toBe(longClaim)
    expect(w.find('[data-test="cross-claim-13"]').text()).not.toContain('…')

    await w.find('[data-test="cross-toggle-13"]').trigger('click')
    expect(w.find('[data-test="cross-claim-13"]').text().length).toBeLessThan(200)
  })

  it('短 claim：无展开按钮', () => {
    const w = mount(BoardBriefReport, { props: { result: makeBrief({ cross_board_relations: [crossOutgoing] }) } })
    expect(w.find('[data-test="cross-toggle-11"]').exists()).toBe(false)
  })

  it('未知类型/未知方向：原样兜底不崩', () => {
    const rel: BoardBriefCrossRelation = {
      ...crossOutgoing,
      relation_id: 14,
      relation_type: 'exotic_future_type',
      direction: 'sideways',
      quality_grade: 'none',
    }
    const w = mount(BoardBriefReport, { props: { result: makeBrief({ cross_board_relations: [rel] }) } })
    const row = w.find('[data-test="cross-relation-14"]')
    expect(row.exists()).toBe(true)
    expect(row.text()).toContain('exotic_future_type')
    expect(row.text()).toContain('sideways')
    expect(row.text()).toContain('未分级')
  })
})
