import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import BoardRelationPanel from './BoardRelationPanel.vue'
import type { BoardRelationDetail, BoardRelationRow } from '~/api/boardEnrichment'

// iconify stub（同 BoardBriefReport.test.ts）
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

/**
 * 跨版块关系建议面板（add-evidence-backed-cross-board-relations 6.2）：
 *  - 加载态 / 空态 / 错误态三形态互不冒充；错误态可重试
 *  - 生命周期分组（proposed/unresolved/confirmed/dismissed）+ 状态过滤 emit
 *  - 行展开详情：映射（有/无目标）、支持证据/反证分区、run 审计
 *  - confirm / re-resolve emit；dismiss 必填理由（空/纯空白禁用）
 *  - 超长 claim 折叠 + 展开
 *  - 动作在途反馈（确认中…/重解析中…）
 */

function makeRelation(id: number, overrides: Partial<BoardRelationRow> = {}): BoardRelationRow {
  return {
    id,
    source_board_id: 7701,
    target_board_id: 7702,
    target_concept: '日债收益率',
    relation_type: 'causal',
    claim: `关系主张 ${id}`,
    verification_verdict: 'supported',
    quality_grade: 'medium',
    status: 'proposed',
    ...overrides,
  }
}

function mountPanel(relations: BoardRelationRow[], extra: Record<string, unknown> = {}) {
  return mount(BoardRelationPanel, {
    props: {
      relations,
      detail: null,
      ...extra,
    },
  })
}

describe('BoardRelationPanel — 三形态', () => {
  it('加载态：加载中文案，不渲染列表', () => {
    const w = mountPanel([], { loading: true })
    expect(w.find('[data-test="relation-loading"]').exists()).toBe(true)
    expect(w.find('[data-test="relation-empty"]').exists()).toBe(false)
  })

  it('空态：直白引导文案（非错误）', () => {
    const w = mountPanel([])
    expect(w.find('[data-test="relation-empty"]').exists()).toBe(true)
    expect(w.find('[data-test="relation-error"]').exists()).toBe(false)
  })

  it('错误态：显示 error 文案 + 重试按钮（不冒充空态）', async () => {
    const w = mountPanel([], { error: '数据库罢工' })
    expect(w.find('[data-test="relation-error"]').exists()).toBe(true)
    expect(w.text()).toContain('数据库罢工')
    expect(w.find('[data-test="relation-empty"]').exists()).toBe(false)
    await w.find('[data-test="relation-retry"]').trigger('click')
    expect(w.emitted('reload')).toBeTruthy()
  })

  it('正在发现关联：显示运行中提示条', () => {
    const w = mountPanel([], { activeSource: { sourceKind: 'observation', sourceKey: 'o1' } })
    expect(w.find('[data-test="relation-discovery-running"]').exists()).toBe(true)
    expect(w.text()).toContain('o1')
  })
})

describe('BoardRelationPanel — 列表与详情', () => {
  it('生命周期分组：proposed / unresolved / confirmed / dismissed 各归其组', () => {
    const w = mountPanel([
      makeRelation(1),
      makeRelation(2, { status: 'unresolved', target_board_id: null }),
      makeRelation(3, { status: 'confirmed' }),
      makeRelation(4, { status: 'dismissed', dismiss_reason: '噪音' }),
    ])
    expect(w.find('[data-test="relation-group-proposed"]').exists()).toBe(true)
    expect(w.find('[data-test="relation-group-unresolved"]').exists()).toBe(true)
    expect(w.find('[data-test="relation-group-confirmed"]').exists()).toBe(true)
    expect(w.find('[data-test="relation-group-dismissed"]').exists()).toBe(true)
    // 目标未定的行展示外部概念
    expect(w.find('[data-test="relation-target-2"]').text()).toContain('日债收益率')
    // 驳回理由可见
    expect(w.find('[data-test="relation-row-4"]').text()).toContain('驳回理由：噪音')
  })

  it('状态过滤 change → reload emit 带 status', async () => {
    const w = mountPanel([makeRelation(1)])
    await w.find('[data-test="relation-status-filter"]').setValue('confirmed')
    expect(w.emitted('reload')).toEqual([['confirmed']])
  })

  it('行展开 → open-detail emit；再点收起', async () => {
    const w = mountPanel([makeRelation(1)])
    await w.find('[data-test="relation-expand-1"]').trigger('click')
    expect(w.emitted('open-detail')).toEqual([[1]])
    expect(w.find('[data-test="relation-detail-1"]').exists()).toBe(true)
    await w.find('[data-test="relation-expand-1"]').trigger('click')
    expect(w.find('[data-test="relation-detail-1"]').exists()).toBe(false)
  })

  it('详情渲染：映射（有目标）/ 支持证据 / 反证 / run 审计', async () => {
    const detail: BoardRelationDetail = {
      ...makeRelation(1),
      mechanism: '避险资金从债市流向黄金',
      mapping_snapshot: { candidates: [{ concept: '日债', score: 0.7 }] },
      evidence: [
        { url: 'https://example.com/a', title: '报道A', institution: '路透', date: '2026-08-30', quote: '原文摘录A', use: 'support' },
      ],
      counterevidence: [
        { url: 'https://example.com/b', title: '反证B', quote: '相反方向的报道' },
      ],
      run: { id: 9, status: 'succeeded', trigger_kind: 'auto', source_kind: 'observation', source_key: 'o1' },
    }
    const w = mountPanel([makeRelation(1)], { detail })
    await w.find('[data-test="relation-expand-1"]').trigger('click')
    expect(w.find('[data-test="relation-detail-1"]').exists()).toBe(true)
    expect(w.text()).toContain('避险资金从债市流向黄金')
    expect(w.text()).toContain('版块 #7702')
    expect(w.text()).toContain('路透')
    expect(w.text()).toContain('反证B')
    const runRow = w.find('[data-test="relation-run"]')
    expect(runRow.text()).toContain('run #9')
    expect(runRow.text()).toContain('自动')
    // 支持证据外链
    const links = w.findAll('a.brp-evi-link')
    expect(links).toHaveLength(2)
    expect(links[0]?.attributes('href')).toBe('https://example.com/a')
  })

  it('详情无目标（unresolved）：映射如实显示未解析', async () => {
    const detail: BoardRelationDetail = { ...makeRelation(2, { status: 'unresolved', target_board_id: null }) }
    const w = mountPanel([makeRelation(2, { status: 'unresolved', target_board_id: null })], { detail })
    await w.find('[data-test="relation-expand-2"]').trigger('click')
    expect(w.text()).toContain('未解析')
  })

  it('详情加载中：显示加载文案', async () => {
    const w = mountPanel([makeRelation(1)], { detailLoading: true })
    await w.find('[data-test="relation-expand-1"]').trigger('click')
    expect(w.text()).toContain('详情加载中')
  })

  it('超长 claim：折叠 + 展开/收起', async () => {
    const longClaim = '超'.repeat(200)
    const w = mountPanel([makeRelation(1, { claim: longClaim })])
    expect(w.find('[data-test="relation-claim-1"]').text().length).toBeLessThan(200)
    await w.find('[data-test="relation-claim-toggle-1"]').trigger('click')
    expect(w.find('[data-test="relation-claim-1"]').text()).toContain('超'.repeat(200))
    await w.find('[data-test="relation-claim-toggle-1"]').trigger('click')
    expect(w.find('[data-test="relation-claim-1"]').text().length).toBeLessThan(200)
  })
})

describe('BoardRelationPanel — 裁决动作', () => {
  it('proposed：确认按钮 emit confirm；在途显示「确认中…」且禁用', async () => {
    const w = mountPanel([makeRelation(1)])
    await w.find('[data-test="relation-confirm-1"]').trigger('click')
    expect(w.emitted('confirm')).toEqual([[1]])

    const w2 = mountPanel([makeRelation(1)], { confirmingId: 1 })
    const btn = w2.find('[data-test="relation-confirm-1"]')
    expect(btn.attributes('disabled')).toBeDefined()
    expect(btn.text()).toContain('确认中')
  })

  it('dismiss：行内理由输入；空/纯空白禁用提交；合法理由 emit', async () => {
    const w = mountPanel([makeRelation(1)])
    // 打开理由输入框
    await w.find('[data-test="relation-dismiss-1"]').trigger('click')
    expect(w.find('[data-test="relation-dismiss-box-1"]').exists()).toBe(true)

    const submit = w.find('[data-test="relation-dismiss-submit-1"]')
    expect(submit.attributes('disabled')).toBeDefined() // 空

    await w.find('[data-test="relation-dismiss-input-1"]').setValue('   ')
    expect(submit.attributes('disabled')).toBeDefined() // 纯空白

    await w.find('[data-test="relation-dismiss-input-1"]').setValue('证据站不住')
    expect(submit.attributes('disabled')).toBeUndefined()
    await submit.trigger('click')
    expect(w.emitted('dismiss')).toEqual([[1, '证据站不住']])
  })

  it('dismiss 在途：提交按钮显示「提交中…」并禁用', () => {
    const w = mountPanel([makeRelation(1)], { dismissingId: 1 })
    expect(w.find('[data-test="relation-dismiss-1"]').attributes('disabled')).toBeDefined()
  })

  it('unresolved：重找目标按钮 emit re-resolve；在途反馈', async () => {
    const w = mountPanel([makeRelation(2, { status: 'unresolved', target_board_id: null })])
    await w.find('[data-test="relation-reresolve-2"]').trigger('click')
    expect(w.emitted('re-resolve')).toEqual([[2]])

    const w2 = mountPanel([makeRelation(2, { status: 'unresolved', target_board_id: null })], { reResolvingId: 2 })
    const btn = w2.find('[data-test="relation-reresolve-2"]')
    expect(btn.attributes('disabled')).toBeDefined()
    expect(btn.text()).toContain('重解析中')
  })

  it('confirmed 行：无确认/驳回按钮（终态只读）', () => {
    const w = mountPanel([makeRelation(3, { status: 'confirmed' })])
    expect(w.find('[data-test="relation-confirm-3"]').exists()).toBe(false)
    expect(w.find('[data-test="relation-dismiss-3"]').exists()).toBe(false)
  })
})
