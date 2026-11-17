import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import LaneAnchorScope from './LaneAnchorScope.vue'
import type { LaneCorrectionNode } from '~/features/tags/composables/useLaneCorrection'

/**
 * 左栏「首义范围」用例（test-cases 故事 A2 / D2–D5 + A-3 / A-12）。
 *
 * 判据（机械断言）：
 * - 当前首义来源标签：自动（创建话题时写入）/ 人工（选定的 N 个节点）；
 * - 勾选节点 → 按钮计数随勾选数变化（「用选中的 N 个节点定首义」）；
 * - 选择池覆盖本话题**全部成员**（窗口内 + 更早成员）；候选不入池；
 * - 属当前首义集的节点带首义标记；自动首义无节点集 → 无节点列表。
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

const POOL = [
  node('m1', '2026-11-10', '窗口内成员'),
  node('m2', '2026-11-12', '窗口内成员二'),
  node('f1', '2026-10-21', '窗外首义节点', { outsideWindow: true }),
  node('f2', '2026-09-28', '窗外其它成员', { outsideWindow: true }),
]

function mountScope(overrides: Record<string, unknown> = {}) {
  return mount(LaneAnchorScope, {
    props: {
      sourceLabel: '自动（创建话题时写入）',
      manual: false,
      pool: POOL,
      memberIds: ['m1', 'm2'],
      anchorIds: [],
      selection: [],
      applying: false,
      ...overrides,
    },
  })
}

describe('LaneAnchorScope · 计数口径（fix-lane-metric-provenance）', () => {
  it('「全部 M」用成员总数（与左栏同定义），不按选择池去重并集', () => {
    const w = mountScope({ windowCount: 11, totalCount: 108 })
    const count = w.find('.las-count').text()
    expect(count).toContain('窗口内 11')
    expect(count).toContain('全部 108')
    // 选择池只有 4 个节点，但「全部」必须是成员总数 108（不是池长 4）
    expect(count).not.toContain('全部 4')
  })

  it('未接线计数（缺 props）时回退选择池条数（旧调用兼容）', () => {
    const w = mountScope()
    const count = w.find('.las-count').text()
    expect(count).toContain('窗口内 2')
    expect(count).toContain('全部 4')
  })
})

describe('LaneAnchorScope · 当前首义来源', () => {
  it('自动首义：标签为「自动（创建话题时写入）」', () => {
    const w = mountScope()
    expect(w.find('[data-testid="lane-anchor-source"]').text()).toContain('自动（创建话题时写入）')
  })

  it('人工首义：标签带选定节点数', () => {
    const w = mountScope({ manual: true, sourceLabel: '人工（选定的 2 个节点）' })
    expect(w.find('[data-testid="lane-anchor-source"]').text()).toContain('人工（选定的 2 个节点）')
  })
})

describe('LaneAnchorScope · 选择与提交', () => {
  it('勾选节点 → 抛 toggle，按钮计数随勾选数变化', async () => {
    const w = mountScope()
    expect(w.find('[data-testid="lane-anchor-apply"]').text()).toContain('用选中的 0 个节点定首义')
    await w.find('[data-testid="lane-anchor-check-m1"]').trigger('change')
    expect(w.emitted('toggle')).toEqual([['m1']])
  })

  it('已勾选时按钮文案带 N，且点击抛 apply', async () => {
    const w = mountScope({ selection: ['m1', 'm2'] })
    const button = w.find('[data-testid="lane-anchor-apply"]')
    expect(button.text()).toContain('用选中的 2 个节点定首义')
    await button.trigger('click')
    expect(w.emitted('apply')).toHaveLength(1)
  })

  it('没勾选时按钮禁用（不提交空首义）', () => {
    const w = mountScope()
    expect(w.find('[data-testid="lane-anchor-apply"]').attributes('disabled')).toBeDefined()
  })
})

describe('LaneAnchorScope · 更早成员分组（D2 / D4 / D5）', () => {
  it('窗外成员进「更早成员 · N」分组（默认折叠，展开后可见），且与窗口内清单分开', async () => {
    const w = mountScope({ manual: true })
    expect(w.find('[data-testid="lane-anchor-earlier-group"]').text()).toContain('更早成员 · 2')
    // 默认折叠：清单不渲染（97 条堆在那儿会糊一屏左栏）。
    expect(w.find('[data-testid="lane-anchor-earlier-list"]').exists()).toBe(false)
    await w.find('[data-testid="lane-anchor-earlier-toggle"]').trigger('click')
    expect(w.find('[data-testid="lane-anchor-earlier-list"]').text()).toContain('窗外首义节点')
    expect(w.find('[data-testid="lane-anchor-earlier-list"]').text()).toContain('窗外其它成员')
    expect(w.find('[data-testid="lane-anchor-list"]').text()).not.toContain('窗外首义节点')
  })

  it('选择池覆盖全部成员（窗口内 + 更早成员），候选不入池', async () => {
    const w = mountScope({ manual: true })
    await w.find('[data-testid="lane-anchor-earlier-toggle"]').trigger('click')
    const pool = w.find('[data-testid="lane-anchor-list"]').text() + w.find('[data-testid="lane-anchor-earlier-list"]').text()
    expect(pool).toContain('窗口内成员')
    expect(pool).toContain('窗外首义节点')
    expect(pool).toContain('窗外其它成员')
    expect(pool).not.toContain('候选节点')
  })

  it('属当前首义集的节点带首义标记（不再单独分组）', async () => {
    const w = mountScope({ manual: true, anchorIds: ['f1'] })
    await w.find('[data-testid="lane-anchor-earlier-toggle"]').trigger('click')
    const f1 = w.find('[data-testid="lane-anchor-earlier-check-f1"]')
    expect(w.find('[data-testid="lane-anchor-earlier-list"]').text()).toContain('首义')
    expect(f1.exists()).toBe(true)
    // 不属首义集的窗外节点不带首义标。
    expect(w.find('[data-testid="lane-anchor-earlier-check-f2"]').element.parentElement?.textContent).not.toContain('首义')
  })

  it('自动首义无节点集 → 不出现「更早」分组（不虚构节点列表）', () => {
    const w = mountScope({ manual: false, pool: POOL.filter(item => !item.outsideWindow) })
    expect(w.find('[data-testid="lane-anchor-earlier-group"]').exists()).toBe(false)
  })

  it('「更早成员」分组的勾选与画布折叠带同一份选择集', async () => {
    const w = mountScope({ manual: true, selection: ['f1'] })
    await w.find('[data-testid="lane-anchor-earlier-toggle"]').trigger('click')
    expect(w.find('[data-testid="lane-anchor-earlier-check-f1"]').element).toHaveProperty('checked', true)
    await w.find('[data-testid="lane-anchor-earlier-check-f1"]').trigger('change')
    expect(w.emitted('toggle')).toEqual([['f1']])
  })
})
