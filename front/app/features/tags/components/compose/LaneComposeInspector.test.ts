import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import LaneComposeInspector from './LaneComposeInspector.vue'
import type { ComposeCandidate } from '~/api/persistentTopics'
import type { LaneRecommendations } from '~/features/tags/composables/laneComposeLogic'

/**
 * 右栏检视栏用例（rework-lane-compose-swimlane-canvas 5.1 / 5.4）。
 *
 * 判据：
 * - 检索范围 = **未归类**（宿主传入的 `unclassifiedIds`），候选链成员永不进检索结果；
 * - ①②③ 三段是恒定骨架，空态 ③ 默认「按标题找」、编辑中/已确认默认「已选事件」；
 * - 检索态 = ③ 切到按标题找（不是另一套右栏），口径行写「未归类 · 命中 N 条」。
 */

function candidate(id: string, periodDate: string, clusterLabel: string, topicId?: string, status?: string): ComposeCandidate {
  return {
    id,
    reportId: '1',
    periodDate,
    clusterLabel,
    embedding: [1, 0],
    persistentTopicId: topicId,
    persistentTopic: topicId ? { id: topicId, label: '候选链甲', status: status ?? 'candidate' } : undefined,
  }
}

const SECTIONS: ComposeCandidate[] = [
  candidate('1', '2026-11-06', '链成员甲', 'T'),
  candidate('2', '2026-11-07', '链成员乙', 'T'),
  candidate('4', '2026-11-08', '未归类事件'),
  candidate('6', '2026-11-09', '归档来源事件', 'A', 'archived'),
]
const UNCLASSIFIED = new Set(['4', '6'])

const RECS: LaneRecommendations<ComposeCandidate> = {
  center: [1, 0],
  unassigned: [{ section: SECTIONS[2]!, distance: 0.1 }],
  active: [],
}

function mountInspector(overrides: Record<string, unknown> = {}) {
  return mount(LaneComposeInspector, {
    props: {
      focusSection: null,
      recommendations: RECS,
      selectedSections: [],
      selectedIds: new Set<string>(),
      sections: SECTIONS,
      unclassifiedIds: UNCLASSIFIED,
      query: '',
      showAssigned: false,
      ...overrides,
    },
    attachTo: document.body,
  })
}

describe('LaneComposeInspector — 三段恒定骨架（5.4 / A-14）', () => {
  it('空态：①②③ 都渲染，③ 默认落「按标题找」', async () => {
    const w = mountInspector()
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-focus"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-compose-suggestions"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-compose-tab-find"]').attributes('aria-selected')).toBe('true')
    expect(w.find('[data-testid="lane-compose-find-input"]').exists()).toBe(true)
  })

  it('编辑中 / 已确认：③ 默认落「已选事件」，成员逐条带真实复选框', async () => {
    const w = mountInspector({
      selectedSections: [SECTIONS[0]!, SECTIONS[1]!],
      selectedIds: new Set(['1', '2']),
    })
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-tab-chosen"]').attributes('aria-selected')).toBe('true')
    expect(w.findAll('[data-testid^="lane-chosen-row-"]')).toHaveLength(2)
    const checkbox = w.find('[data-testid="lane-chosen-check-2"]').element as HTMLInputElement
    expect(checkbox.checked).toBe(true)
  })

  it('检索态只切 ③ 内容：①② 仍在场，不出现第二套右栏', async () => {
    const w = mountInspector({ selectedSections: [SECTIONS[0]!], selectedIds: new Set(['1']), query: '未归类' })
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-focus"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-compose-suggestions"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-compose-panel-find"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-compose-panel-chosen"]').exists()).toBe(true)
  })

  it('取消已选勾选即移出本次（不询问、不自动补回）', async () => {
    const w = mountInspector({
      selectedSections: [SECTIONS[0]!, SECTIONS[1]!],
      selectedIds: new Set(['1', '2']),
    })
    await flushPromises()
    await (w.find('[data-testid="lane-chosen-check-2"]').element as HTMLInputElement).click()
    expect(w.emitted('remove')?.[0]).toEqual(['2'])
  })
})

describe('LaneComposeInspector — 检索范围 = 未归类（5.1 / A-7b）', () => {
  it('候选链成员不进检索结果：只出现在链成员标题里的词 → 命中 0', async () => {
    const w = mountInspector({ query: '链成员' })
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-find-scope"]').text()).toContain('命中 0 条')
    expect(w.findAll('[data-testid^="lane-find-"]')).toHaveLength(0)
  })

  it('未归类节点可命中：口令行写「未归类 · 命中 N 条」', async () => {
    const w = mountInspector({ query: '事件' })
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-find-scope"]').text()).toContain('未归类 · 命中 2 条')
    const ids = w.findAll('[data-testid^="lane-find-"]').map(row => row.attributes('data-testid'))
    expect(ids).toContain('lane-find-4')
    expect(ids).toContain('lane-find-6')
    expect(ids.some(id => id === 'lane-find-1')).toBe(false)
  })

  it('检索范围不是 persistentTopicId == null：挂着 candidate 话题但未成链的节点仍在范围内', async () => {
    const detached = candidate('9', '2026-11-10', '未成链的候选话题成员', 'T9')
    const w = mountInspector({
      sections: [...SECTIONS, detached],
      unclassifiedIds: new Set([...UNCLASSIFIED, '9']),
      query: '未成链',
    })
    await flushPromises()
    const rows = w.findAll('[data-testid^="lane-find-"]')
      .map(row => row.attributes('data-testid') ?? '')
      .filter(id => !id.startsWith('lane-find-add-'))
    expect(rows).toEqual(['lane-find-9'])
  })

  it('输入关键词只 emit（由宿主持有，画布命中集与右栏同源）', async () => {
    const w = mountInspector()
    await flushPromises()
    await w.find('[data-testid="lane-compose-find-input"]').setValue('归档')
    expect(w.emitted('update:query')?.[0]).toEqual(['归档'])
  })

  it('检索范围口径行：无关键词时给范围大小（未归类 · N 条可补）', async () => {
    const w = mountInspector()
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-find-scope"]').text()).toContain('未归类 · 2 条可补')
  })
})

describe('LaneComposeInspector — ① 只叙述（不放选入按钮）', () => {
  it('当前事件线索段没有按钮；标题/日期/来源都在', async () => {
    const w = mountInspector({ focusSection: SECTIONS[3]! })
    await flushPromises()
    const focus = w.find('[data-testid="lane-compose-focus"]')
    expect(focus.text()).toContain('归档来源事件')
    expect(focus.text()).toContain('2026-11-09')
    expect(focus.text()).toContain('已归档')
    expect(focus.find('button').exists()).toBe(false)
  })
})

describe('LaneComposeInspector — 右栏② ★（add-lane-compose-topic-event-tags）', () => {
  it('可补充事件条目带确认注记 → 日期行首 ★；无标记不显示', () => {
    const withKey = {
      ...RECS,
      unassigned: [
        { section: { ...SECTIONS[2]!, confirmedKeyEvents: [{ markerId: 'm1', title: '降息预期升温' }] }, distance: 0.1 },
        { section: { ...SECTIONS[2]!, id: '8', clusterLabel: '另一条', confirmedKeyEvents: [] }, distance: 0.2 },
      ],
    }
    const w = mountInspector({ recommendations: withKey })
    expect(w.findAll('.lci__key-star')).toHaveLength(1)
    expect(w.find('[data-testid="lane-suggest-4"]').text()).toContain('★')
    expect(w.find('[data-testid="lane-suggest-8"]').text()).not.toContain('★')
  })
})

describe('LaneComposeInspector — 退出检索（A-7 收口）', () => {
  it('「清空」→ 关键词归零（宿主据此退出检索态）', async () => {
    const w = mountInspector({ query: '归档' })
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-find-scope"]').text()).toContain('命中 1 条')
    await w.find('[data-testid="lane-compose-find-clear"]').trigger('click')
    expect(w.emitted('update:query')?.[0]).toEqual([''])
  })

  it('关键词由非空回到空：③ 退回「已选事件」（有已选时）', async () => {
    const w = mountInspector({ selectedSections: [SECTIONS[0]!], selectedIds: new Set(['1']), query: '归档' })
    await flushPromises()
    await w.setProps({ query: '' })
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-tab-chosen"]').attributes('aria-selected')).toBe('true')
  })
})

describe('LaneComposeInspector — 界面文字只承担三类（P-2）', () => {
  const EMPTY_RECS: LaneRecommendations<ComposeCandidate> = { center: null, unassigned: [], active: [] }

  it('无已选成员时 ② 只写状态结论「无」，不出现机制科普/教学语', async () => {
    const w = mountInspector({ recommendations: EMPTY_RECS })
    await flushPromises()
    const suggest = w.find('[data-testid="lane-compose-suggestions"]')
    expect(suggest.find('.lci__empty').text()).toBe('无')
    expect(suggest.text()).not.toContain('质心')
    expect(suggest.text()).not.toContain('选入候选后')
  })

  it('有已选但零推荐：保留「状态结论 + 下一步动作」（允许范围内，不改成「无」）', async () => {
    const w = mountInspector({
      recommendations: EMPTY_RECS,
      selectedSections: [SECTIONS[0]!],
      selectedIds: new Set(['1']),
    })
    await flushPromises()
    const suggest = w.find('[data-testid="lane-compose-suggestions"]')
    expect(suggest.find('.lci__empty').text()).toContain('当前没有符合阈值的补充推荐')
    expect(suggest.text()).not.toContain('质心')
  })

  it('① 不再承载「在节点卡里直接点」这类界面内教学语', async () => {
    const w = mountInspector({ focusSection: SECTIONS[0]! })
    await flushPromises()
    const focus = w.find('[data-testid="lane-compose-focus"]')
    expect(focus.text()).toContain('候选话题')
    expect(focus.text()).not.toContain('节点卡里直接点')
  })

  it('① 只读参照只叙述归属事实，不解释跨泳道策略', async () => {
    const active = candidate('5', '2026-11-11', '已有泳道成员', 'A', 'active')
    const w = mountInspector({ focusSection: active })
    await flushPromises()
    const text = w.find('[data-testid="lane-compose-focus"]').text()
    expect(text).toContain('只读参照')
    expect(text).not.toContain('另行确认')
  })
})
