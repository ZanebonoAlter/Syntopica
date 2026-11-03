/**
 * ComposeSidebar — 就地编排态右侧候选侧边栏测试（inline-compose-lane 切片②）.
 *
 * 纯展示组件，只测 props→DOM/emit 接线（composable 逻辑由 D1 覆盖）：
 *  - 无候选时候选区隐藏
 *  - 正在连续命中组：activatable 置顶高亮、确认启用 disabled 当 !activatable
 *  - 已中断组：标「近期未命中」、不渲染确认启用按钮、视觉弱化
 *  - 点确认启用 emit("activate", topicId)；点采纳 emit("adopt", item)
 *  - 搜索框输入 emit("update:queryText")；searchError 显提示条
 *  - 相似 section 推荐：两组皆空隐藏、点击 emit("recommend", id)、active 项显来源
 *  - 已中断组默认折叠（标题计数 + aria-expanded），点标题展开
 */
import { describe, expect, it } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import ComposeSidebar from './ComposeSidebar.vue'
import AppButton from '~/components/ui/AppButton.vue'
import type { BoardTopicListItem } from '~/api/dailyReports'
import type { RecommendedSection, SidebarCandidateItem } from '~/features/tags/composables/useInlineCompose'

function makeTopic(over: Partial<BoardTopicListItem> = {}): BoardTopicListItem {
  return {
    id: 1,
    semantic_board_id: 1,
    label: '美伊博弈',
    description: '',
    status: 'candidate',
    first_seen_date: '2026-01-01',
    last_seen_date: '2026-06-01',
    hit_count: 3,
    consecutive_hits: 3,
    section_count: 4,
    color: '#c2410c',
    can_activate: true,
    ...over,
  }
}

function makeItem(
  topicOver: Partial<BoardTopicListItem> = {},
  itemOver: Partial<Omit<SidebarCandidateItem, 'topic'>> = {},
): SidebarCandidateItem {
  const topic = makeTopic(topicOver)
  return {
    topic,
    activatable: topic.can_activate,
    brokenStreak: topic.consecutive_hits === 0,
    ...itemOver,
  }
}

function mountSidebar(over: Record<string, unknown> = {}) {
  return mount(ComposeSidebar, {
    props: {
      items: [],
      queryText: '',
      searchError: null,
      searching: false,
      recommendations: { unassigned: [], active: [] },
      recommendationTitle: '与你已选最相近',
      ...over,
    },
  })
}

describe('ComposeSidebar — 无候选', () => {
  it('items 为空时候选区整体隐藏', () => {
    const wrapper = mountSidebar({ items: [] })
    expect(wrapper.find('.cs-candidates').exists()).toBe(false)
  })
})

describe('ComposeSidebar — 正在连续命中组', () => {
  it('activatable 候选置顶高亮渲染', () => {
    const activatable = makeItem({ id: 10, label: '美伊博弈', consecutive_hits: 3, can_activate: true })
    const wrapper = mountSidebar({ items: [activatable] })
    const card = wrapper.find('.cs-card.is-activatable')
    expect(card.exists()).toBe(true)
    expect(card.text()).toContain('美伊博弈')
    expect(card.text()).toContain('连续 3 天')
    expect(card.text()).toContain('含 4 条 section')
  })

  it('!activatable 候选「确认启用」禁用且提示条件', () => {
    const notYet = makeItem({ id: 11, label: '油价波动', consecutive_hits: 1, can_activate: false })
    const wrapper = mountSidebar({ items: [notYet] })
    const card = wrapper.find('.cs-card')
    const confirmBtn = card.findAllComponents(AppButton).find((b: VueWrapper) => b.text().includes('确认启用'))
    expect(confirmBtn).toBeTruthy()
    expect(confirmBtn!.props('disabled')).toBe(true)
    expect(card.text()).toContain('需先满足连续多天出现条件')
  })

  it('点确认启用 emit("activate", topicId)', async () => {
    const item = makeItem({ id: 20, can_activate: true })
    const wrapper = mountSidebar({ items: [item] })
    const card = wrapper.find('.cs-card')
    const confirmBtn = card.findAllComponents(AppButton).find((b: VueWrapper) => b.text().includes('确认启用'))
    await confirmBtn!.trigger('click')
    expect(wrapper.emitted('activate')).toBeTruthy()
    expect(wrapper.emitted('activate')![0]).toEqual([20])
  })
})

describe('ComposeSidebar — 已中断·近期未命中组', () => {
  it('brokenStreak 候选标「近期未命中」且不渲染确认启用按钮', () => {
    const broken = makeItem({ id: 30, label: '断连续', consecutive_hits: 0, can_activate: false })
    const wrapper = mountSidebar({ items: [broken] })
    const brokenGroup = wrapper.find('.cs-group--broken')
    expect(brokenGroup.exists()).toBe(true)
    expect(brokenGroup.text()).toContain('近期未命中')
    // 不应出现「连续 0 天」，也不应出现「确认启用」按钮
    expect(brokenGroup.text()).not.toContain('连续 0 天')
    const confirmBtn = brokenGroup.findAllComponents(AppButton)
      .find((b: VueWrapper) => b.text().includes('确认启用'))
    expect(confirmBtn).toBeUndefined()
  })
})

describe('ComposeSidebar — 采纳', () => {
  it('点采纳 emit("adopt", item)', async () => {
    const item = makeItem({ id: 40, label: '半导体管制', can_activate: true })
    const wrapper = mountSidebar({ items: [item] })
    const card = wrapper.find('.cs-card')
    const adoptBtn = card.findAllComponents(AppButton).find((b: VueWrapper) => b.text().includes('采纳'))
    await adoptBtn!.trigger('click')
    expect(wrapper.emitted('adopt')).toBeTruthy()
    expect(wrapper.emitted('adopt')![0]).toEqual([item])
  })
})

describe('ComposeSidebar — 搜索框', () => {
  it('输入搜索文本 emit("update:queryText")', async () => {
    const wrapper = mountSidebar({ queryText: '' })
    await wrapper.find('input').setValue('半导体出口管制')
    const evt = wrapper.emitted('update:queryText')
    expect(evt).toBeTruthy()
    expect(evt![0]).toEqual(['半导体出口管制'])
  })

  it('searchError 非空时显示降级提示条', () => {
    const wrapper = mountSidebar({ searchError: '搜索向量生成失败' })
    const hint = wrapper.find('.cs-search__error')
    expect(hint.exists()).toBe(true)
    expect(hint.text()).toContain('搜索向量生成失败')
  })

  it('searching 时显示搜索中态', () => {
    const wrapper = mountSidebar({ searching: true })
    expect(wrapper.find('.cs-search__loading').exists()).toBe(true)
  })
})

function makeRec(over: Partial<RecommendedSection> = {}): RecommendedSection {
  return {
    id: 'r1',
    clusterLabel: '相似条目',
    originLabel: null,
    distance: 0.12,
    ...over,
  }
}

describe('ComposeSidebar — 相似 section 推荐', () => {
  it('两组皆空时推荐区不渲染', () => {
    const wrapper = mountSidebar()
    expect(wrapper.find('.cs-recommend').exists()).toBe(false)
  })

  it('unassigned 推荐渲染 label+distance，点击 emit("recommend", id)', async () => {
    const rec = makeRec({ id: 'u9', clusterLabel: '半导体管制', distance: 0.18 })
    const wrapper = mountSidebar({ recommendations: { unassigned: [rec], active: [] } })
    const items = wrapper.findAll('.cs-rec')
    expect(items).toHaveLength(1)
    expect(items[0]!.text()).toContain('半导体管制')
    expect(items[0]!.text()).toContain('0.18')
    await items[0]!.trigger('click')
    expect(wrapper.emitted('recommend')).toEqual([['u9']])
  })

  it('active 推荐显示「从「泳道」移出」来源标签', () => {
    const rec = makeRec({ id: 'm3', clusterLabel: '中东战报C', originLabel: '中东局势', distance: 0.1 })
    const wrapper = mountSidebar({ recommendations: { unassigned: [], active: [rec] } })
    expect(wrapper.find('.cs-rec').text()).toContain('从「中东局势」移出')
  })
})

describe('ComposeSidebar — 已中断组折叠', () => {
  const broken = makeItem({ id: 9, consecutive_hits: 0, can_activate: false, label: '断连续' })
  const main = makeItem({ id: 10, consecutive_hits: 3, can_activate: true, label: '连续命中' })

  it('有主组时默认折叠：标题带计数、aria-expanded=false、内容隐藏', () => {
    const wrapper = mountSidebar({ items: [main, broken] })
    const title = wrapper.find('.cs-group__title--btn')
    expect(title.exists()).toBe(true)
    expect(title.text()).toContain('已中断·近期未命中（1）')
    expect(title.attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('.cs-card.is-broken').exists()).toBe(false)
  })

  it('点标题展开 → aria-expanded=true、内容可见', async () => {
    const wrapper = mountSidebar({ items: [main, broken] })
    const title = wrapper.find('.cs-group__title--btn')
    await title.trigger('click')
    expect(title.attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('.cs-card.is-broken').exists()).toBe(true)
  })

  it('只有已中断候选时默认展开，避免真实数据下侧栏空白', () => {
    const wrapper = mountSidebar({ items: [broken] })
    const title = wrapper.find('.cs-group__title--btn')
    expect(title.attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('.cs-card.is-broken').exists()).toBe(true)
  })
})
