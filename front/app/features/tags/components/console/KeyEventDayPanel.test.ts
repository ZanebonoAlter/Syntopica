import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import KeyEventDayPanel from './KeyEventDayPanel.vue'
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

const byDate = (map: Record<string, BoardKeyEventItem[]>) => new Map(Object.entries(map))

beforeEach(() => {
  vi.clearAllMocks()
})

describe('KeyEventDayPanel — 选中日事件列表', () => {
  it('无选中日：方向提示（不冒充空列表）', () => {
    const wrapper = mount(KeyEventDayPanel, {
      props: { selectedDate: null, eventsByDate: byDate({}), reading: readingStore() },
    })
    expect(wrapper.find('[data-testid="key-event-day-empty-selection"]').text()).toContain('选择日期')
    expect(wrapper.find('[data-testid="key-event-day-title"]').exists()).toBe(false)
  })

  it('选中日无事件：如实说明并指向日报阅读台', () => {
    const wrapper = mount(KeyEventDayPanel, {
      props: { selectedDate: '2026-11-12', eventsByDate: byDate({}), reading: readingStore() },
    })
    expect(wrapper.find('[data-testid="key-event-day-none"]').text()).toContain('没有确认的关键事件')
    expect(wrapper.find('[data-testid="key-event-day-none"]').text()).toContain('日报阅读台')
  })

  it('选中日 3 条事件：逐卡渲染标题与话题快照', () => {
    const wrapper = mount(KeyEventDayPanel, {
      props: {
        selectedDate: '2026-11-12',
        eventsByDate: byDate({ '2026-11-12': [event('a'), event('b'), event('c')] }),
        reading: readingStore(),
      },
    })
    expect(wrapper.find('[data-testid="key-event-day-title"]').text()).toContain('11/12')
    const cards = wrapper.findAll('[data-testid^="key-event-card-"]')
    expect(cards).toHaveLength(3)
    expect(cards[0]!.text()).toContain('事件 a')
    expect(cards[0]!.text()).toContain('AI 模型发布')
  })

  it('摘要手风琴：点标题展开全文、再点收束；其余卡收束（单开）', async () => {
    const wrapper = mount(KeyEventDayPanel, {
      props: {
        selectedDate: '2026-11-12',
        eventsByDate: byDate({ '2026-11-12': [event('a'), event('b')] }),
        reading: readingStore(),
      },
    })
    expect(wrapper.find('.kdp-summary').exists()).toBe(false)
    await wrapper.find('[data-testid="key-event-card-a"]').find('.kdp-card-head').trigger('click')
    expect(wrapper.find('[data-testid="key-event-card-a"]').find('.kdp-summary').text()).toContain('事件 a 的摘要全文')
    expect(wrapper.find('[data-testid="key-event-card-b"]').find('.kdp-summary').exists()).toBe(false)
    await wrapper.find('[data-testid="key-event-card-a"]').find('.kdp-card-head').trigger('click')
    expect(wrapper.find('.kdp-summary').exists()).toBe(false)
  })

  it('关联文章：有 articleIds 的卡显示入口、无文章的卡不显示（MUST NOT 空列表占位）', async () => {
    const wrapper = mount(KeyEventDayPanel, {
      props: {
        selectedDate: '2026-11-12',
        eventsByDate: byDate({
          '2026-11-12': [
            event('with-arts', { articleIds: ['101', '102'] }),
            event('no-arts'),
          ],
        }),
        reading: readingStore(),
      },
    })
    await wrapper.find('[data-testid="key-event-card-with-arts"]').find('.kdp-card-head').trigger('click')
    expect(wrapper.find('[data-testid="key-event-card-with-arts"]').findComponent(LaneRelatedArticles).exists()).toBe(true)

    await wrapper.find('[data-testid="key-event-card-no-arts"]').find('.kdp-card-head').trigger('click')
    expect(wrapper.find('[data-testid="key-event-card-no-arts"]').findComponent(LaneRelatedArticles).exists()).toBe(false)
    expect(wrapper.find('[data-testid="key-event-card-no-arts"]').text()).not.toContain('相关文章')
  })

  it('文章元信息复用壳层 reading（不新建缓存）：展开时按需读 + 点文章转发 open-article', async () => {
    const reading = {
      readRelated: vi.fn(() => ({ status: 'success' as const, meta: { articleId: '101', title: '文章甲', pubDate: '2026-11-12T08:00:00Z', read: false, article: {} } })),
      ensureRelatedArticles: vi.fn(async () => {}),
      retryRelatedArticle: vi.fn(async () => {}),
    } as unknown as LaneDayReadingStore
    const wrapper = mount(KeyEventDayPanel, {
      props: {
        selectedDate: '2026-11-12',
        eventsByDate: byDate({ '2026-11-12': [event('a', { articleIds: ['101'] })] }),
        reading,
      },
    })
    await wrapper.find('[data-testid="key-event-card-a"]').find('.kdp-card-head').trigger('click')
    // LaneRelatedArticles 展开触发按需读
    await wrapper.find('[data-testid="key-event-card-a"]').find('.lra-toggle').trigger('click')
    expect(reading.ensureRelatedArticles).toHaveBeenCalledWith(['101'])
    await wrapper.find('[data-testid="lane-chronicle-article-101"]').trigger('click')
    expect(wrapper.emitted('openArticle')).toEqual([['101']])
  })

  it('同文章跨事件：取数全部走注入的同一 store（不新建第二份缓存）', async () => {
    const reading = {
      readRelated: vi.fn(() => ({ status: 'idle' as const })),
      ensureRelatedArticles: vi.fn(async () => {}),
      retryRelatedArticle: vi.fn(async () => {}),
    } as unknown as LaneDayReadingStore
    const wrapper = mount(KeyEventDayPanel, {
      props: {
        selectedDate: '2026-11-12',
        eventsByDate: byDate({
          '2026-11-12': [event('a', { articleIds: ['101'] }), event('b', { articleIds: ['101'] })],
        }),
        reading,
      },
    })
    await wrapper.find('[data-testid="key-event-card-a"]').find('.kdp-card-head').trigger('click')
    await wrapper.find('[data-testid="key-event-card-a"]').find('.lra-toggle').trigger('click')
    await wrapper.find('[data-testid="key-event-card-b"]').find('.kdp-card-head').trigger('click')
    await wrapper.find('[data-testid="key-event-card-b"]').find('.lra-toggle').trigger('click')
    // 两次展开都只委托给壳层 store（同一文章 ID 由 store 的在途合乘/缓存处理，组件不自建缓存）
    expect(reading.ensureRelatedArticles).toHaveBeenCalledTimes(2)
    expect(reading.ensureRelatedArticles).toHaveBeenNthCalledWith(1, ['101'])
    expect(reading.ensureRelatedArticles).toHaveBeenNthCalledWith(2, ['101'])
  })

  it('打开文章预览不打断本地展开态：emit open-article 后摘要与文章列表仍在', async () => {
    const reading = {
      readRelated: vi.fn(() => ({ status: 'success' as const, meta: { articleId: '101', title: '文章甲', pubDate: '2026-11-12T08:00:00Z', read: false, article: {} } })),
      ensureRelatedArticles: vi.fn(async () => {}),
      retryRelatedArticle: vi.fn(async () => {}),
    } as unknown as LaneDayReadingStore
    const wrapper = mount(KeyEventDayPanel, {
      props: {
        selectedDate: '2026-11-12',
        eventsByDate: byDate({ '2026-11-12': [event('a', { articleIds: ['101'] })] }),
        reading,
      },
    })
    await wrapper.find('[data-testid="key-event-card-a"]').find('.kdp-card-head').trigger('click')
    await wrapper.find('[data-testid="key-event-card-a"]').find('.lra-toggle').trigger('click')
    await wrapper.find('[data-testid="lane-chronicle-article-101"]').trigger('click')
    // 预览由壳层既有弹窗接管；本面板不因预览而收束（关闭后仍回原卡）
    expect(wrapper.emitted('openArticle')).toEqual([['101']])
    expect(wrapper.find('[data-testid="key-event-card-a"]').find('.kdp-summary').exists()).toBe(true)
    expect(wrapper.find('[data-testid="lane-chronicle-article-101"]').exists()).toBe(true)
  })

  it('选中日切换：展开态复位（新日从收束开始，不残留旧卡状态）', async () => {
    const wrapper = mount(KeyEventDayPanel, {
      props: {
        selectedDate: '2026-11-12',
        eventsByDate: byDate({ '2026-11-12': [event('a')], '2026-11-13': [event('b')] }),
        reading: readingStore(),
      },
    })
    await wrapper.find('[data-testid="key-event-card-a"]').find('.kdp-card-head').trigger('click')
    expect(wrapper.find('.kdp-summary').exists()).toBe(true)
    await wrapper.setProps({ selectedDate: '2026-11-13' })
    await flushPromises()
    expect(wrapper.find('.kdp-summary').exists()).toBe(false)
    expect(wrapper.find('[data-testid="key-event-card-b"]').exists()).toBe(true)
  })
})

// ── 标题不截断（review R-3：层数超限的行轴上已无名称，阅读区是唯一兜底）─────

describe('KeyEventDayPanel — 标题完整可读（不省略号截断）', () => {
  it('长标题逐字渲染（不被裁成残句）', () => {
    const longTitle = 'GPT-6.2 Astra与Sol双模型发布，分层定位专业与日常场景，并同步开放企业级推理通道'
    const wrapper = mount(KeyEventDayPanel, {
      props: {
        selectedDate: '2026-11-12',
        eventsByDate: byDate({ '2026-11-12': [event('a', { titleSnapshot: longTitle })] }),
        reading: readingStore(),
      },
    })
    expect(wrapper.find('.kdp-card-title').text()).toBe(longTitle)
  })

  it('标题样式不得用省略号截断（源码契约，happy-dom 不解析 scoped CSS 改读源码）', async () => {
    const { readFileSync } = await import('node:fs')
    const { resolve } = await import('node:path')
    const source = readFileSync(resolve(process.cwd(), 'app/features/tags/components/console/KeyEventDayPanel.vue'), 'utf-8')
    // 只看标题**自己的**规则块（文件里还有 hover 态的 `.kdp-card-head:hover .kdp-card-title`）
    const start = source.indexOf('\n.kdp-card-title {')
    expect(start, '未找到 .kdp-card-title 规则块').toBeGreaterThan(-1)
    const block = source.slice(start, source.indexOf('}', start))
    expect(block, '标题 MUST NOT 用 text-overflow: ellipsis').not.toContain('text-overflow')
    expect(block, '标题 MUST NOT 单行 nowrap').not.toContain('white-space: nowrap')
    expect(block, '标题 MUST 允许换行').toContain('white-space: normal')
  })
})
