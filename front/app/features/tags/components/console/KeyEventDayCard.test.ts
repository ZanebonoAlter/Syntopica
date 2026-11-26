import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import KeyEventDayCard from './KeyEventDayCard.vue'
import LaneRelatedArticles from './LaneRelatedArticles.vue'
import type { BoardKeyEventItem } from '~/api/boardKeyEvents'
import type { LaneDayReadingStore } from '~/features/tags/composables/useLaneDayReading'

function event(id: string, overrides: Partial<BoardKeyEventItem> = {}): BoardKeyEventItem {
  return {
    id, topicId: '93', topicLabelSnapshot: 'AI 模型发布',
    sourcePeriodDate: '2026-11-12', titleSnapshot: `事件 ${id}`,
    summarySnapshot: `事件 ${id} 的摘要全文。`, articleIds: [], ...overrides,
  }
}

/** 文章按需读三项的测试桩（其余成员不参与本组件，用 unknown 断言补齐）。 */
function readingStore(): LaneDayReadingStore {
  return {
    readRelated: vi.fn(() => ({ status: 'idle' as const })),
    ensureRelatedArticles: vi.fn(async () => {}),
    retryRelatedArticle: vi.fn(async () => {}),
  } as unknown as LaneDayReadingStore
}

/** 文章已就绪的读取桩（点文章 → emit openArticle 用）。 */
function loadedReadingStore(): LaneDayReadingStore {
  return {
    readRelated: vi.fn(() => ({
      status: 'success' as const,
      meta: { articleId: '101', title: '文章甲', pubDate: '2026-11-12T08:00:00Z', read: false, article: {} },
    })),
    ensureRelatedArticles: vi.fn(async () => {}),
    retryRelatedArticle: vi.fn(async () => {}),
  } as unknown as LaneDayReadingStore
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('KeyEventDayCard — 结构与内容', () => {
  it('首行：日期（加粗、事件色）+ 所属话题（弱化、同一行）', () => {
    const wrapper = mount(KeyEventDayCard, {
      props: { event: event('a'), reading: readingStore() },
    })
    const date = wrapper.find('[data-testid="key-event-day-card-date-a"]')
    expect(date.text()).toBe('11/12 周四')
    expect(wrapper.find('[data-testid="key-event-day-card-topic-a"]').text()).toBe('AI 模型发布')
    // 同一行：日期与话题同属 .kdc-meta
    expect(date.element.closest('.kdc-meta')).not.toBeNull()
  })

  it('标题完整渲染（长标题逐字可读，不省略）', () => {
    const longTitle = 'GPT-6.2 Astra与Sol双模型发布，分层定位专业与日常场景，并同步开放企业级推理通道'
    const wrapper = mount(KeyEventDayCard, {
      props: { event: event('a', { titleSnapshot: longTitle }), reading: readingStore() },
    })
    expect(wrapper.find('[data-testid="key-event-day-card-title-a"]').text()).toBe(longTitle)
  })

  it('标题样式不得用省略号截断（源码契约，happy-dom 不解析 scoped CSS 改读源码）', async () => {
    const { readFileSync } = await import('node:fs')
    const { resolve } = await import('node:path')
    const source = readFileSync(resolve(process.cwd(), 'app/features/tags/components/console/KeyEventDayCard.vue'), 'utf-8')
    const start = source.indexOf('\n.kdc-title {')
    expect(start, '未找到 .kdc-title 规则块').toBeGreaterThan(-1)
    const block = source.slice(start, source.indexOf('}', start))
    expect(block, '标题 MUST NOT 用 text-overflow: ellipsis').not.toContain('text-overflow')
    expect(block, '标题 MUST NOT 单行 nowrap').not.toContain('white-space: nowrap')
    expect(block, '标题 MUST 允许换行').toContain('white-space: normal')
  })

  it('卡片内不出现注记写入口与教学语（只读视图）', () => {
    const wrapper = mount(KeyEventDayCard, {
      props: { event: event('a', { articleIds: ['101'] }), reading: readingStore() },
    })
    const text = wrapper.text()
    for (const banned of ['确认', '取消', '编辑', 'suggested', 'dismissed']) {
      expect(text, `卡片内不得出现「${banned}」`).not.toContain(banned)
    }
  })
})

describe('KeyEventDayCard — 展开控件（button + aria-expanded，逐卡独立）', () => {
  it('摘要区是 button + aria-expanded：点开就地展开全文、再点收起', async () => {
    const wrapper = mount(KeyEventDayCard, {
      props: { event: event('a'), reading: readingStore() },
    })
    const toggle = wrapper.find('[data-testid="key-event-day-card-summary-a"]')
    expect(toggle.element.tagName).toBe('BUTTON')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('.kdc-summary-btn').classes()).not.toContain('is-expanded')

    await toggle.trigger('click')
    expect(wrapper.find('[data-testid="key-event-day-card-summary-a"]').attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('.kdc-summary-btn').classes()).toContain('is-expanded')
    // 展开后是全文（不是残句）
    expect(wrapper.find('.kdc-summary').text()).toBe('事件 a 的摘要全文。')

    await wrapper.find('[data-testid="key-event-day-card-summary-a"]').trigger('click')
    expect(wrapper.find('[data-testid="key-event-day-card-summary-a"]').attributes('aria-expanded')).toBe('false')
  })

  it('关联文章入口是 button + aria-expanded；无文章不显示入口', async () => {
    const withArts = mount(KeyEventDayCard, {
      props: { event: event('with-arts', { articleIds: ['101', '102'] }), reading: readingStore() },
    })
    const toggle = withArts.find('[data-testid="key-event-day-card-articles-with-arts"]')
    expect(toggle.element.tagName).toBe('BUTTON')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(toggle.text()).toContain('关联文章 2 篇')

    const noArts = mount(KeyEventDayCard, {
      props: { event: event('no-arts'), reading: readingStore() },
    })
    expect(noArts.find('[data-testid="key-event-day-card-articles-no-arts"]').exists()).toBe(false)
    expect(noArts.text()).not.toContain('关联文章')
  })

  it('展开后按钮文案切「收起文章」（ui-design「按钮变『收起文章』」），篇数留在 aria-label', async () => {
    const wrapper = mount(KeyEventDayCard, {
      props: { event: event('a', { articleIds: ['101', '102'] }), reading: readingStore() },
    })
    const toggle = wrapper.find('[data-testid="key-event-day-card-articles-a"]')
    expect(toggle.text(), '收起前：关联文章 N 篇').toContain('关联文章 2 篇')

    await toggle.trigger('click')
    const expanded = wrapper.find('[data-testid="key-event-day-card-articles-a"]')
    expect(expanded.attributes('aria-expanded')).toBe('true')
    expect(expanded.text(), '展开后 MUST 切「收起文章」').toContain('收起文章')
    expect(expanded.text(), '展开后不再显示「关联文章 N 篇」').not.toContain('关联文章 2 篇')
    // 篇数信息保留在 aria-label（可见文案是「收起文章」）
    expect(expanded.attributes('aria-label')).toContain('收起文章')
    expect(expanded.attributes('aria-label')).toContain('2 篇')

    await expanded.trigger('click')
    const collapsed = wrapper.find('[data-testid="key-event-day-card-articles-a"]')
    expect(collapsed.text(), '再点收起 → 文案回到「关联文章 N 篇」').toContain('关联文章 2 篇')
  })

  it('逐卡独立展开：展开 A 卡不影响 B 卡', async () => {
    const wrapper = mount(KeyEventDayCard, {
      props: { event: event('a'), reading: readingStore() },
    })
    const other = mount(KeyEventDayCard, {
      props: { event: event('b'), reading: readingStore() },
    })
    await wrapper.find('[data-testid="key-event-day-card-summary-a"]').trigger('click')
    expect(wrapper.find('[data-testid="key-event-day-card-summary-a"]').attributes('aria-expanded')).toBe('true')
    expect(other.find('[data-testid="key-event-day-card-summary-b"]').attributes('aria-expanded')).toBe('false')
  })
})

describe('KeyEventDayCard — 关联文章（复用既有组件与同一份 reading）', () => {
  it('点文字链展开文章列表；不出现第二个内部开关（无双开关）', async () => {
    const reading = readingStore()
    const wrapper = mount(KeyEventDayCard, {
      props: { event: event('a', { articleIds: ['101'] }), reading },
    })
    expect(wrapper.findComponent(LaneRelatedArticles).exists()).toBe(false)

    await wrapper.find('[data-testid="key-event-day-card-articles-a"]').trigger('click')
    expect(wrapper.findComponent(LaneRelatedArticles).exists()).toBe(true)
    // 卡片自带入口 → 既有组件的内部开关不渲染
    expect(wrapper.find('.lra-toggle').exists()).toBe(false)
    // 文章按需读走注入的同一份 store（不新建第二份缓存）
    expect(reading.ensureRelatedArticles).toHaveBeenCalledWith(['101'])
  })

  it('点文章 → emit openArticle（走既有站内阅读链路）', async () => {
    const wrapper = mount(KeyEventDayCard, {
      props: { event: event('a', { articleIds: ['101'] }), reading: loadedReadingStore() },
    })
    await wrapper.find('[data-testid="key-event-day-card-articles-a"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-testid="lane-chronicle-article-101"]').trigger('click')
    expect(wrapper.emitted('openArticle')).toEqual([['101']])
  })

  it('文章列表真实渲染（标题 · 日期），不是空占位', async () => {
    const wrapper = mount(KeyEventDayCard, {
      props: { event: event('a', { articleIds: ['101'] }), reading: loadedReadingStore() },
    })
    await wrapper.find('[data-testid="key-event-day-card-articles-a"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="lane-chronicle-article-101"]').text()).toContain('文章甲')
    expect(wrapper.find('[data-testid="lane-chronicle-article-101"]').text()).toContain('2026-11-12')
  })
})
