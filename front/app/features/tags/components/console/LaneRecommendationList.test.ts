import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import LaneRecommendationList from './LaneRecommendationList.vue'
import type { LaneCorrectionNode } from '~/features/tags/composables/useLaneCorrection'

/**
 * 右栏「关联推荐」用例（unify-lane-workbench 3.1 / 9.4）。
 *
 * 推荐行断言从 `LaneMemberList.test.ts` 迁入（左栏收窄后推荐只在右栏）：
 * - 「＋ 加入」抛 add、「定位」抛 focus（与节点卡同一语义、同一份状态）；
 * - 估算接近度口径（与成员快照不同名不同形）；
 * - 来源泳道名（改挂来源要看得见；无来源不显装饰性标签）；
 * - pending 同步禁用、空态、错误态、长标题省略号 + title 全文。
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
    topicId: '',
    topicLabel: '',
    proximity: null,
    proximityKind: 'estimate',
    outsideWindow: false,
    ...extra,
  }
}

const RECOMMENDATIONS = [
  node('c1', '2026-11-11', '候选节点', { proximity: 0.62 }),
  node('c2', '2026-11-12', '动态泳道节点', { topicId: '9', topicLabel: '工具链动态', proximity: 0.58 }),
]

function mountList(overrides: Record<string, unknown> = {}) {
  return mount(LaneRecommendationList, {
    props: {
      recommendations: RECOMMENDATIONS,
      pendingIds: [],
      ...overrides,
    },
  })
}

describe('LaneRecommendationList · 推荐行（迁移自 LaneMemberList）', () => {
  it('点「＋ 加入」→ 抛 add（与节点卡加入同一语义）', async () => {
    const w = mountList()
    await w.find('[data-testid="lane-recommend-add-c1"]').trigger('click')
    expect(w.emitted('add')).toEqual([['c1']])
  })

  it('点「定位」→ 抛 focus（只改选中态，不写归属）', async () => {
    const w = mountList()
    await w.find('[data-testid="lane-recommend-peek-c1"]').trigger('click')
    expect(w.emitted('focus')).toEqual([['c1']])
  })

  it('估算接近度口径：接近度不可算时不显示数字（不伪造）', () => {
    const w = mountList({ recommendations: [node('c3', '2026-11-11', '无距离候选')] })
    expect(w.find('[data-testid="lane-recommend-prox-c3"]').exists()).toBe(false)
    expect(w.find('[data-testid="lane-recommend-list"]').text()).not.toContain('接近度')
  })

  it('接近度展示为「估算接近度」（与成员快照不同名不同形）', () => {
    const w = mountList()
    expect(w.find('[data-testid="lane-recommend-prox-c1"]').text()).toBe('估算接近度 0.62')
    expect(w.text()).not.toContain('快照')
  })

  it('没有推荐时给本块自己的空态', () => {
    const w = mountList({ recommendations: [] })
    expect(w.find('[data-testid="lane-recommend-empty"]').text()).toContain('没有新的关联推荐')
  })
})

describe('LaneRecommendationList · 来源与四态（B14 / I7 / B5）', () => {
  it('来源泳道名可见（改挂来源要看得见）；无来源不显装饰性标签', () => {
    const w = mountList()
    const row = w.find('[data-testid="lane-recommend-c2"]')
    expect(row.text()).toContain('来自 工具链动态')
    // 无归属行不显示「来自」标签
    expect(w.find('[data-testid="lane-recommend-c1"]').find('[data-testid="lane-recommend-source"]').exists()).toBe(false)
  })

  it('pending 同步禁用：同节点按钮 disabled（防重复提交）', () => {
    const w = mountList({ pendingIds: ['c1'] })
    expect((w.find('[data-testid="lane-recommend-add-c1"]').element as HTMLButtonElement).disabled).toBe(true)
    expect((w.find('[data-testid="lane-recommend-add-c2"]').element as HTMLButtonElement).disabled).toBe(false)
  })

  it('错误态：行内错误展示（保持旧数据不清空）', () => {
    const w = mountList({ error: '候选取数失败' })
    expect(w.find('[data-testid="lane-recommend-error"]').text()).toContain('候选取数失败')
    expect(w.find('[data-testid="lane-recommend-list"]').exists()).toBe(false)
  })

  it('长标题：CSS 省略号形态（title 属性给全文；按钮不被挤走）', () => {
    const longTitle = '这是一个非常长非常长的候选节点标题'.repeat(6)
    const w = mountList({ recommendations: [node('c9', '2026-11-11', longTitle, { proximity: 0.5 })] })
    const name = w.find('[data-testid="lane-recommend-c9"]').find('.lrl-name')
    expect(name.attributes('title')).toBe(longTitle) // 全文可达
    // 截断是 CSS 形态（overflow ellipsis，视觉验收归浏览器）；DOM 不吞文本
    expect(name.text()).toBe(longTitle)
    expect(w.find('[data-testid="lane-recommend-add-c9"]').exists()).toBe(true)
  })

  it('计数与推荐行同源（待选 · N）', () => {
    const w = mountList()
    expect(w.find('[data-testid="lane-recommend-count"]').text()).toBe('待选 · 2')
  })
})
