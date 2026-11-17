import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import LaneMemberList from './LaneMemberList.vue'
import type { LaneCorrectionNode } from '~/features/tags/composables/useLaneCorrection'

/**
 * 左栏「本泳道成员 / 更早成员 / 已移出（本次）」用例（test-cases 故事 B1 / B5 + A-2 / A-7）。
 *
 * unify-lane-workbench 3.2 收窄：关联推荐已迁出至右栏 `LaneRecommendationList.vue`
 * （推荐断言迁 RN 测试；本文件反向断言左栏无推荐分组——两侧内容不重复）。
 *
 * 判据（机械断言）：
 * - 取消勾选成员 → 抛 remove（**直接移除，不弹任何选择器**）；
 * - 节点移出后进「已移出（本次）」分组，点「加回来」抛 restore；
 * - 成员行标「首义」（该节点 ∈ 当前首义集）；
 * - 成员接近度 = 归属快照口径（估算口径在右栏 RN，不同名不同形）；
 * - 数据全量、展示分层（5.12）：徽标「窗口 N · 全部 M」；「更早成员 · N」折叠
 *   分组按月分堆、组内滚动、点开节点卡；无全量口径时回退「N 条」。
 */

function node(
  id: string,
  date: string,
  title: string,
  extra: Partial<LaneCorrectionNode> = {},
): LaneCorrectionNode {
  return {
    id,
    date,
    title,
    topicId: '7',
    topicLabel: 'AI 前沿',
    proximity: null,
    // 接近度来源：默认归属快照（关联推荐行为 estimate）。
    proximityKind: 'snapshot',
    outsideWindow: false,
    ...extra,
  }
}

const MEMBERS = [
  node('m1', '2026-11-10', '成员一', { proximity: 0.93 }),
  node('m2', '2026-11-12', '成员二', { proximity: 0.81 }),
]

/** 更早成员（窗口外；跨两个月 → 按月分堆）。 */
const EARLY_MEMBERS = [
  node('e1', '2026-10-21', '十月成员', { outsideWindow: true, proximity: 0.72 }),
  node('e2', '2026-10-03', '十月初成员', { outsideWindow: true }),
  node('e3', '2026-09-28', '九月成员', { outsideWindow: true }),
]

const REMOVED = [node('m4', '2026-11-11', '误入节点')]

function mountList(overrides: Record<string, unknown> = {}) {
  return mount(LaneMemberList, {
    props: {
      members: MEMBERS,
      removed: REMOVED,
      anchorIds: [],
      pendingIds: [],
      ...overrides,
    },
  })
}

describe('LaneMemberList · 移除（故事 B1）', () => {
  it('取消勾选成员 → 抛 remove，且不出现任何选择器/弹窗', async () => {
    const w = mountList()
    await w.find('[data-testid="lane-left-check-m1"]').trigger('change')
    expect(w.emitted('remove')).toEqual([['m1']])
    expect(w.find('.app-dialog').exists()).toBe(false)
    expect(w.find('select').exists()).toBe(false)
  })

  it('取消勾选是破坏性动作：与「取消选中」在视觉与提示上区分', () => {
    const w = mountList()
    const check = w.find('[data-testid="lane-left-check-m1"]')
    expect(check.classes()).toContain('lml-check--destructive')
    expect(check.attributes('title')).toBe('取消勾选即移出本泳道')
  })

  it('成员计数与清单同源', () => {
    const w = mountList()
    expect(w.find('[data-testid="lane-members-count"]').text()).toBe('2 条')
    expect(w.find('[data-testid="lane-left-members"]').findAll('li')).toHaveLength(2)
  })

  it('首义节点在成员行标「首义」', () => {
    const w = mountList({ anchorIds: ['m2'] })
    expect(w.find('[data-testid="lane-left-member-m2"]').text()).toContain('首义')
    expect(w.find('[data-testid="lane-left-member-m1"]').text()).not.toContain('首义')
  })
})

describe('LaneMemberList · 计数与接近度口径（fix-lane-metric-provenance）', () => {
  it('徽标「窗口 N · 全部 M」：M = 成员总数（与首义面板同定义）', () => {
    const w = mountList({ totalCount: 108 })
    expect(w.find('[data-testid="lane-members-count"]').text()).toBe('窗口 2 · 全部 108')
  })

  it('成员接近度 = 归属快照口径（估算口径在右栏 RN，不同名不同形）', () => {
    const w = mountList()
    const memberProx = w.find('[data-testid="lane-left-prox"]').text()
    expect(memberProx).toContain('接近度 0.93')
    expect(memberProx).toContain('快照')
    expect(memberProx).not.toContain('估算')
  })
})

describe('LaneMemberList · 已移出与加回（故事 B5）', () => {
  it('本次移出的节点在「已移出（本次）」分组里', () => {
    const w = mountList()
    expect(w.find('[data-testid="lane-removed-group"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-removed-list"]').text()).toContain('误入节点')
  })

  it('点「加回来」→ 抛 restore（不弹任何选择器）', async () => {
    const w = mountList()
    await w.find('[data-testid="lane-restore-m4"]').trigger('click')
    expect(w.emitted('restore')).toEqual([['m4']])
    expect(w.find('.app-dialog').exists()).toBe(false)
  })

  it('没有移出节点时不渲染该分组', () => {
    const w = mountList({ removed: [] })
    expect(w.find('[data-testid="lane-removed-group"]').exists()).toBe(false)
  })
})

describe('LaneMemberList · 左栏无推荐（unify-lane-workbench 3.2 / 三栏内容不重复）', () => {
  it('左栏 SHALL NOT 出现关联推荐分组（推荐已迁右栏，两侧不重复）', () => {
    const w = mountList({ earlyMembers: EARLY_MEMBERS, totalCount: 5 })
    expect(w.find('[data-testid="lane-recommend-panel"]').exists()).toBe(false)
    expect(w.find('[data-testid="lane-recommend-list"]').exists()).toBe(false)
    expect(w.text()).not.toContain('关联推荐')
    expect(w.text()).not.toContain('估算接近度')
    expect(w.emitted('add')).toBeUndefined()
  })
})

describe('LaneMemberList · 数据全量、展示分层（5.12 / 故事 D6）', () => {
  it('计数徽标为「窗口 N · 全部 M」（M = 窗口成员 + 更早成员）', () => {
    const w = mountList({ earlyMembers: EARLY_MEMBERS, totalCount: 5 })
    expect(w.find('[data-testid="lane-members-count"]').text()).toBe('窗口 2 · 全部 5')
  })

  it('后端未提供全量口径 → 回退只显示窗口数（旧后端兼容）', () => {
    const w = mountList()
    expect(w.find('[data-testid="lane-members-count"]').text()).toBe('2 条')
  })

  it('「更早成员 · N」分组默认折叠（折叠态不占高）', () => {
    const w = mountList({ earlyMembers: EARLY_MEMBERS, totalCount: 5 })
    expect(w.find('[data-testid="lane-early-group"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-early-count"]').text()).toContain('3')
    expect(w.find('[data-testid="lane-early-month-2026-10"]').exists()).toBe(false)
  })

  it('展开后按月分堆，每条显示日期 + 标题', async () => {
    const w = mountList({ earlyMembers: EARLY_MEMBERS, totalCount: 5 })
    await w.find('[data-testid="lane-early-toggle"]').trigger('click')

    const october = w.find('[data-testid="lane-early-month-2026-10"]')
    expect(october.exists()).toBe(true)
    expect(october.text()).toContain('10月')
    expect(october.text()).toContain('十月成员')
    expect(october.text()).toContain('十月初成员')

    const september = w.find('[data-testid="lane-early-month-2026-09"]')
    expect(september.exists()).toBe(true)
    expect(september.text()).toContain('九月成员')
  })

  it('点更早成员 → 打开节点卡（标题/日期/接近度/身份标，无画布定位）', async () => {
    const w = mountList({ earlyMembers: EARLY_MEMBERS, totalCount: 5 })
    await w.find('[data-testid="lane-early-toggle"]').trigger('click')
    expect(w.find('[data-testid="lane-early-card-e1"]').exists()).toBe(false)

    await w.find('[data-testid="lane-early-peek-e1"]').trigger('click')
    const card = w.find('[data-testid="lane-early-card-e1"]')
    expect(card.exists()).toBe(true)
    expect(card.text()).toContain('十月成员')
    expect(card.text()).toContain('2026-10-21')
    expect(card.text()).toContain('接近度 0.72')
    expect(card.text()).toContain('更早成员')
    // 更早成员不在画布上：卡片里没有「定位」动作。
    expect(card.find('[data-testid^="lane-left-peek-"]').exists()).toBe(false)
    expect(w.emitted('focus')).toBeUndefined()
  })

  it('没有更早成员 → 不渲染该分组', () => {
    const w = mountList({ earlyMembers: [], totalCount: 2 })
    expect(w.find('[data-testid="lane-early-group"]').exists()).toBe(false)
  })
})
