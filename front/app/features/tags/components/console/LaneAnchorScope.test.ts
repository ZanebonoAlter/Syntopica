import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import LaneAnchorScope from './LaneAnchorScope.vue'
import type { LaneCentroidScope, LaneCorrectionNode } from '~/features/tags/composables/useLaneCorrection'

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
      originKind: 'unresolved',
      originTitle: null,
      originDate: null,
      centroid: null,
      ...overrides,
    },
  })
}

/** 质心滚动窗口口径（参与滚动计算的节点）。 */
function centroid(overrides: Partial<LaneCentroidScope> = {}): LaneCentroidScope {
  return {
    windowSize: 30,
    anchorWeight: 0.5,
    sectionIds: [11, 12],
    sectionCount: 2,
    members: [
      { sectionId: 11, title: '滚动节点一', periodDate: '2026-11-10' },
      { sectionId: 12, title: '滚动节点二', periodDate: '2026-11-11' },
    ],
    degraded: false,
    ...overrides,
  }
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

  it('首义读失败（sourceLabel 为空）：不渲染来源行（不拿未读到的值当断言），但保留「读取失败 · 重试」', () => {
    const w = mountScope({ sourceLabel: '', originKind: 'error' })
    // 来源行整行不渲染：没有「当前首义来源：自动/人工」的断言。
    expect(w.find('[data-testid="lane-anchor-source"]').exists()).toBe(false)
    expect(w.text()).not.toContain('当前首义来源')
    expect(w.text()).not.toContain('自动（创建话题时写入）')
    // 不重复报错：只留「读取失败 · 重试」这一行。
    const retry = w.find('[data-testid="lane-anchor-origin-retry"]')
    expect(retry.exists()).toBe(true)
    expect(retry.text()).toContain('读取失败 · 重试')
    // 写入口照旧在场。
    expect(w.find('[data-testid="lane-anchor-apply"]').exists()).toBe(true)
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

// ── 6.3：首义来源与滚动窗口的只读展示（align-lane-correction-workface） ─────────
describe('LaneAnchorScope · 首义来源节点（可考 / 不可考 / 读失败）', () => {
  it('可考：显示来源节点标题 + 日期，且不出现「来源不可考」', () => {
    const w = mountScope({
      originKind: 'resolved',
      originTitle: '人工选定的首义节点',
      originDate: '2026-11-10',
    })
    const origin = w.find('[data-testid="lane-anchor-origin"]').text()
    expect(origin).toContain('人工选定的首义节点')
    expect(origin).toContain('2026-11-10')
    expect(origin).not.toContain('来源不可考')
    expect(w.find('[data-testid="lane-anchor-origin-unresolved"]').exists()).toBe(false)
  })

  it('不可考：显示「创建时写入 · 来源不可考」，且**不显示任何节点条目**', () => {
    const w = mountScope({ originKind: 'unresolved' })
    expect(w.find('[data-testid="lane-anchor-origin-unresolved"]').text()).toContain('创建时写入 · 来源不可考')
    // 不拿「最早成员」等推测节点顶替：来源节点区域不得出现节点标题
    expect(w.find('[data-testid="lane-anchor-origin-node"]').exists()).toBe(false)
    expect(w.find('[data-testid="lane-anchor-origin"]').text()).not.toContain('窗口内成员')
    expect(w.find('[data-testid="lane-anchor-origin-note"]').text()).toContain('定一次首义即可留痕')
  })

  it('读失败：显示「读取失败 · 重试」，点它抛 retry（只读块失败不阻断写入口）', async () => {
    const w = mountScope({ originKind: 'error' })
    const retry = w.find('[data-testid="lane-anchor-origin-retry"]')
    expect(retry.text()).toContain('读取失败 · 重试')
    await retry.trigger('click')
    expect(w.emitted('retry')).toHaveLength(1)
    // 写入口（勾选 / 定首义）仍在
    expect(w.find('[data-testid="lane-anchor-apply"]').exists()).toBe(true)
  })

  it('读取中：显示「读取中…」，不误报不可考', () => {
    const w = mountScope({ originKind: 'loading' })
    expect(w.find('[data-testid="lane-anchor-origin-loading"]').text()).toContain('读取中')
    expect(w.find('[data-testid="lane-anchor-origin-unresolved"]').exists()).toBe(false)
  })
})

describe('LaneAnchorScope · 参与滚动计算的节点（命名隔离 + 退化口径）', () => {
  it('计数与成员计数不同名不同数：「参与滚动计算的节点（30/30）」≠「窗口内 11 · 全部 108」', () => {
    const w = mountScope({
      windowCount: 11,
      totalCount: 108,
      centroid: centroid({ sectionIds: [1, 2, 3], sectionCount: 30 }),
    })
    const badge = w.find('.las-count').text()
    expect(badge).toContain('窗口内 11')
    expect(badge).toContain('全部 108')
    const group = w.find('[data-testid="lane-anchor-centroid-toggle"]').text()
    expect(group).toContain('参与滚动计算的节点（30/30）')
    // 两把尺子不得同名：徽标说「窗口内」，滚动组不得也叫「窗口 N」
    expect(group).not.toContain('窗口内')
  })

  it('默认收起：只显示计数；点开列出参与滚动计算的节点', async () => {
    const w = mountScope({ centroid: centroid() })
    expect(w.find('[data-testid="lane-anchor-centroid-list"]').exists()).toBe(false)
    await w.find('[data-testid="lane-anchor-centroid-toggle"]').trigger('click')
    const list = w.find('[data-testid="lane-anchor-centroid-list"]').text()
    expect(list).toContain('滚动节点一')
    expect(list).toContain('滚动节点二')
  })

  it('degraded：文案是中性退化口径，不得写死成「退回首义向量」', () => {
    const w = mountScope({ centroid: centroid({ degraded: true }) })
    const note = w.find('[data-testid="lane-anchor-centroid-note"]').text()
    expect(note).toContain('质心当前为退化口径（窗口向量不足或首义不可用）')
    expect(note).not.toContain('退回首义向量')
    expect(note).not.toContain('纯滚动均值')
  })

  it('后端未给滚动窗口口径 → 整块不渲染（不造 0、不造窗口）', () => {
    const w = mountScope({ centroid: null })
    expect(w.find('[data-testid="lane-anchor-centroid-group"]').exists()).toBe(false)
  })
})
