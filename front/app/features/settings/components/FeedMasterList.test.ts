/**
 * FeedMasterList — add-source-board-hit-rate T5 / B6-09..14。
 *
 * 排序在分类分组内生效（不打乱分类结构）、只看低命中筛选（≥5 篇且 <30% 且打标开启、
 * 空分类隐藏、无结果可复位）、列表 meta 行与行尾 pill 接入、窗口切换双向同步
 * （FeedMasterList 与 FeedSourceQualityBlock 挂同一 useFeedSourceQuality 实例的 harness）。
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, onMounted } from 'vue'
import FeedMasterList from './FeedMasterList.vue'
import FeedSourceQualityBlock from './FeedSourceQualityBlock.vue'
import { useFeedSourceQuality } from '../composables/useFeedSourceQuality'
import type { FeedBoardHitStats, RssFeed } from '~/types'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span />' },
}))
vi.mock('~/components/feed/FeedIcon.vue', () => ({
  default: { name: 'FeedIcon', template: '<i />' },
}))

const { getBoardHitStatsMock } = vi.hoisted(() => ({
  getBoardHitStatsMock: vi.fn(),
}))
vi.mock('~/api/feeds', () => ({
  useFeedsApi: () => ({ getBoardHitStats: getBoardHitStatsMock }),
}))

// —— fixture：两大分类 + 一个筛后为空的分类；含 0 篇源与打标关闭源 ——
function stats(partial: Partial<FeedBoardHitStats>): FeedBoardHitStats {
  return {
    feed_id: 0,
    title: '',
    tagging_enabled: true,
    articles: 0,
    in_board: 0,
    tagged_no_board: 0,
    untagged_pending: 0,
    untagged_settled: 0,
    hit_rate: 0,
    boards: [],
    ...partial,
  }
}

const STATS: Record<string, FeedBoardHitStats> = {
  f1: stats({ feed_id: 1, articles: 1662, in_board: 437, hit_rate: 437 / 1662, tagged_no_board: 739, untagged_pending: 486 }),
  f2: stats({ feed_id: 2, articles: 499, in_board: 160, hit_rate: 160 / 499, tagged_no_board: 268, untagged_settled: 64 }),
  f3: stats({ feed_id: 3, articles: 67, in_board: 56, hit_rate: 56 / 67, untagged_settled: 4 }),
  f4: stats({ feed_id: 4, articles: 162, in_board: 71, hit_rate: 71 / 162, untagged_settled: 92 }),
  f5: stats({ feed_id: 5, articles: 1, in_board: 0, hit_rate: 0, tagging_enabled: false, untagged_settled: 1 }),
  f6: stats({ feed_id: 6, articles: 14, in_board: 1, hit_rate: 1 / 14, untagged_settled: 13 }),
  f7: stats({ feed_id: 7, articles: 2, in_board: 0, hit_rate: 0, untagged_settled: 2 }),
  f8: stats({ feed_id: 8, articles: 0, in_board: 0, hit_rate: 0, boards: [] }),
}

function feed(id: string, title: string): RssFeed {
  return { id, title, url: `https://example.com/${id}`, category: '', description: '' } as RssFeed
}

const FEEDS_BY_CATEGORY: Record<string, RssFeed[]> = {
  新闻资讯: [feed('f1', '华尔街见闻-要闻'), feed('f2', '资讯_凤凰网'), feed('f3', '华尔街见闻-最热文章')],
  技术社区: [feed('f4', 'V2EX-技术'), feed('f5', 'HuggingFace-Blog'), feed('f6', 'HackerNews'), feed('f8', '慢源')],
  独家内容: [feed('f7', '冷门源')],
}

function mountList(props: Record<string, unknown> = {}) {
  return mount(FeedMasterList, {
    props: {
      feedsByCategory: FEEDS_BY_CATEGORY,
      collapsedCategories: {},
      statsByFeed: STATS,
      statsLoading: false,
      windowDays: 7,
      ...props,
    },
  })
}

type SeqEntry = { type: 'cat' | 'item'; title: string }

function structure(wrapper: ReturnType<typeof mountList>): SeqEntry[] {
  return wrapper
    .findAll('.feed-master__category, .feed-master__item')
    .map((node) =>
      node.classes().includes('feed-master__category')
        ? { type: 'cat' as const, title: node.find('.feed-master__category-name').text() }
        : { type: 'item' as const, title: node.find('.feed-master__item-title').text() },
    )
}

function itemsUnder(wrapper: ReturnType<typeof mountList>, category: string): string[] {
  const seq = structure(wrapper)
  const start = seq.findIndex(s => s.type === 'cat' && s.title === category)
  if (start === -1) return []
  const out: string[] = []
  for (let i = start + 1; i < seq.length; i++) {
    const entry = seq[i]!
    if (entry.type !== 'item') break
    out.push(entry.title)
  }
  return out
}

async function setSort(wrapper: ReturnType<typeof mountList>, mode: string) {
  await wrapper.find('[data-testid="feed-sort-select"]').setValue(mode)
}

beforeEach(() => {
  getBoardHitStatsMock.mockReset()
})

describe('FeedMasterList — 分类内排序（B6-09..11）', () => {
  it('B6-09: 入板块率升序 → 分类结构不变、分类内率升序、0 篇源排末尾', async () => {
    const wrapper = mountList()
    await setSort(wrapper, 'hit_rate_asc')

    const cats = structure(wrapper).filter(s => s.type === 'cat').map(s => s.title)
    expect(cats).toEqual(['新闻资讯', '技术社区', '独家内容'])

    expect(itemsUnder(wrapper, '新闻资讯')).toEqual([
      '华尔街见闻-要闻',
      '资讯_凤凰网',
      '华尔街见闻-最热文章',
    ])
    // f5 打标关闭 rate=0 排前；f8 窗口内 0 篇（无样本）排在样本充足项之后
    expect(itemsUnder(wrapper, '技术社区')).toEqual([
      'HuggingFace-Blog',
      'HackerNews',
      'V2EX-技术',
      '慢源',
    ])
  })

  it('B6-10: 篇数降序 → 分类内按窗口篇数降序', async () => {
    const wrapper = mountList()
    await setSort(wrapper, 'articles_desc')
    expect(itemsUnder(wrapper, '新闻资讯')).toEqual([
      '华尔街见闻-要闻',
      '资讯_凤凰网',
      '华尔街见闻-最热文章',
    ])
    expect(itemsUnder(wrapper, '技术社区')).toEqual([
      'V2EX-技术',
      'HackerNews',
      'HuggingFace-Blog',
      '慢源',
    ])
  })

  it('B6-11: 杂音量降序（articles − in_board）→ 分类内按杂音量降序', async () => {
    const wrapper = mountList()
    await setSort(wrapper, 'noise_desc')
    expect(itemsUnder(wrapper, '新闻资讯')).toEqual([
      '华尔街见闻-要闻',
      '资讯_凤凰网',
      '华尔街见闻-最热文章',
    ])
    expect(itemsUnder(wrapper, '技术社区')).toEqual([
      'V2EX-技术',
      'HackerNews',
      'HuggingFace-Blog',
      '慢源',
    ])
  })

  it('默认模式 = 分类内标题序', async () => {
    const wrapper = mountList()
    const titles = FEEDS_BY_CATEGORY['新闻资讯']!.map(f => f.title)
    const expected = [...titles].sort((a, b) => a.localeCompare(b, 'zh-Hans-CN'))
    expect(itemsUnder(wrapper, '新闻资讯')).toEqual(expected)
    // 分组结构不被排序打乱
    expect(structure(wrapper).filter(s => s.type === 'cat').map(s => s.title)).toEqual([
      '新闻资讯',
      '技术社区',
      '独家内容',
    ])
  })
})

describe('FeedMasterList — 只看低命中筛选（B6-12..13）', () => {
  it('B6-12: 仅保留 ≥5 篇且 <30% 且打标开启的源；空分类隐藏', async () => {
    const wrapper = mountList()
    await wrapper.find('[data-testid="feed-low-hit-filter"]').trigger('click')

    // f1（1662 篇 26%）与 f6（14 篇 7%）保留；f2/f3/f4 率 ≥30%、f7 <5 篇、f8 0 篇、f5 打标关闭均被筛除
    expect(itemsUnder(wrapper, '新闻资讯')).toEqual(['华尔街见闻-要闻'])
    expect(itemsUnder(wrapper, '技术社区')).toEqual(['HackerNews'])
    // 「独家内容」分类筛后为空 → 自动隐藏
    expect(itemsUnder(wrapper, '独家内容')).toEqual([])
    expect(structure(wrapper).some(s => s.type === 'cat' && s.title === '独家内容')).toBe(false)
  })

  it('B6-12: 打标关闭的源不被判为杂音（不出现在筛选结果中）', async () => {
    const wrapper = mountList()
    await wrapper.find('[data-testid="feed-low-hit-filter"]').trigger('click')
    const allItems = structure(wrapper).filter(s => s.type === 'item').map(s => s.title)
    expect(allItems).not.toContain('HuggingFace-Blog')
  })

  it('B6-13: 筛选无匹配 → 空结果提示 + 复位入口，复位后恢复全量', async () => {
    const noNoiseStats = Object.fromEntries(
      Object.entries(STATS).map(([k, s]) => [k, { ...s, hit_rate: Math.max(s.hit_rate, 0.5) }]),
    )
    const wrapper = mountList({ statsByFeed: noNoiseStats })
    await wrapper.find('[data-testid="feed-low-hit-filter"]').trigger('click')

    const empty = wrapper.find('[data-testid="feed-filter-empty"]')
    expect(empty.exists()).toBe(true)
    expect(empty.text()).toContain('没有符合筛选条件的订阅源')

    await wrapper.find('[data-testid="feed-filter-reset"]').trigger('click')
    expect(wrapper.find('[data-testid="feed-filter-empty"]').exists()).toBe(false)
    expect(itemsUnder(wrapper, '新闻资讯')).toHaveLength(3)
    expect(itemsUnder(wrapper, '技术社区')).toHaveLength(4)
  })
})

describe('FeedMasterList — 列表项接入与窗口（B6-01/02/07 列表侧 + B6-14）', () => {
  it('meta 行「N 天内 X 篇 · 入板块 Y」+ 行尾 pill 三档；0 篇源只显示「N 天内 0 篇」', () => {
    const wrapper = mountList()
    const items = wrapper.findAll('.feed-master__item')
    const f1 = items.find(i => i.find('.feed-master__item-title').text() === '华尔街见闻-要闻')!
    expect(f1.find('.feed-master__item-meta').text()).toContain('7 天内 1662 篇 · 入板块 437')
    // pill 用 DOM 锚断言（DOMWrapper 上不做组件查询）
    const pill1 = f1.find('.feed-source-pill')
    expect(pill1.text()).toBe('26%')
    expect(pill1.classes()).toContain('feed-source-pill--low')

    const f3 = items.find(i => i.find('.feed-master__item-title').text() === '华尔街见闻-最热文章')!
    expect(f3.find('.feed-master__item-meta').text()).toContain('7 天内 67 篇 · 入板块 56')
    expect(f3.find('.feed-source-pill').classes()).toContain('feed-source-pill--high')

    const f8 = items.find(i => i.find('.feed-master__item-title').text() === '慢源')!
    expect(f8.find('.feed-master__item-meta').text()).toContain('7 天内 0 篇')
    expect(f8.find('.feed-source-pill').text()).toBe('无新文')
  })

  it('统计加载中：pill 全部「—」，meta 行沿用既有状态文案（不显示不可信数字）', () => {
    const wrapper = mountList({ statsLoading: true })
    for (const pill of wrapper.findAll('.feed-source-pill')) {
      expect(pill.text()).toBe('—')
      expect(pill.classes()).toContain('feed-source-pill--pending')
    }
    const f1 = wrapper.findAll('.feed-master__item')[0]!
    expect(f1.find('.feed-master__item-meta').text()).not.toContain('天内')
  })

  it('B6-14: 点列表窗口分段 → 通知父级切窗口(30)；windowDays=30 时 meta 行同步为 30 天', async () => {
    // 注：本机 VTU 2.4.6 + Vue 3.5 组合下 wrapper.emitted() 不记录自定义事件（环境级问题，
    // 既有 emitted 断言型测试同样红，与本 change 无关），故用监听器 prop 断言。
    const onSetWindow = vi.fn()
    const wrapper = mountList({ onSetWindow })
    await wrapper.find('.feed-master__seg-btn[data-window="30"]').trigger('click')
    expect(onSetWindow).toHaveBeenCalledWith(30)

    await wrapper.setProps({ windowDays: 30 })
    const f1 = wrapper.findAll('.feed-master__item')[0]!
    expect(f1.find('.feed-master__item-meta').text()).toContain('30 天内 1662 篇 · 入板块 437')
  })
})

// —— B6-14（跨组件同步）：列表工具栏与详情块挂同一 useFeedSourceQuality 实例 ——
const HARNESS_FEEDS: Record<string, RssFeed[]> = { 新闻资讯: [feed('f1', '华尔街见闻-要闻')] }

const SyncHarness = defineComponent({
  components: { FeedMasterList, FeedSourceQualityBlock },
  setup() {
    const q = useFeedSourceQuality()
    onMounted(() => { void q.load() })
    return {
      q,
      harnessFeeds: HARNESS_FEEDS,
      harnessFeed: feed('f1', '华尔街见闻-要闻'),
    }
  },
  template: `
    <div>
      <FeedMasterList
        :feeds-by-category="harnessFeeds"
        :collapsed-categories="{}"
        :stats-by-feed="q.statsByFeed.value"
        :stats-loading="q.loading.value"
        :window-days="q.windowDays.value"
        @set-window="q.setWindow"
      />
      <FeedSourceQualityBlock
        :feed="harnessFeed"
        :stats="q.statsByFeed.value['f1']"
        :loading="q.loading.value"
        :error="q.error.value"
        :window-days="q.windowDays.value"
        @set-window="q.setWindow"
        @retry="q.retry"
      />
    </div>
  `,
})

describe('FeedMasterList ↔ FeedSourceQualityBlock 窗口双向同步（B6-14）', () => {
  it('列表切 30 天 → 详情块窗口标签同步为 30 天；详情侧切回 7 → 列表同步', async () => {
    getBoardHitStatsMock.mockResolvedValue({
      success: true,
      data: { items: [STATS.f1] },
    })
    const wrapper = mount(SyncHarness)
    await flushPromises()

    const block = wrapper.findComponent(FeedSourceQualityBlock)
    expect(wrapper.find('.feed-master__seg-btn--on').attributes('data-window')).toBe('7')
    expect(block.find('.fsq-seg-btn--on').attributes('data-window')).toBe('7')

    // 列表侧切 30 → 详情块同步 30（同一窗口状态源）
    await wrapper.find('.feed-master__seg-btn[data-window="30"]').trigger('click')
    await flushPromises()
    expect(getBoardHitStatsMock).toHaveBeenLastCalledWith(30)
    expect(wrapper.find('.feed-master__seg-btn--on').attributes('data-window')).toBe('30')
    expect(block.find('.fsq-seg-btn--on').attributes('data-window')).toBe('30')

    // 详情侧切回 7 → 列表同步（spec：任一处切换两处一致）
    await block.find('.fsq-seg-btn[data-window="7"]').trigger('click')
    await flushPromises()
    expect(getBoardHitStatsMock).toHaveBeenLastCalledWith(7)
    expect(wrapper.find('.feed-master__seg-btn--on').attributes('data-window')).toBe('7')
    expect(block.find('.fsq-seg-btn--on').attributes('data-window')).toBe('7')
  })
})
