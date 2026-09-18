/**
 * FeedSourceQualityBlock — add-source-board-hit-rate T5 / B6-15..20。
 *
 * 四分解堆叠条宽度比例 + 图例篇数 + 率文案（分子/分母）、板块分布 Top6+其他 N、
 * 空态/样本不足/打标关闭三态文案、失败重试且不阻断设置表单
 * （后者经 FeedDetailEditor 装配层验证：块失败时下方表单仍可交互）。
 */
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import FeedSourceQualityBlock from './FeedSourceQualityBlock.vue'
import FeedDetailEditor from './FeedDetailEditor.vue'
import type { FeedBoardHitStats, RssFeed } from '~/types'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span />' },
}))
vi.mock('~/components/feed/FeedIcon.vue', () => ({
  default: { name: 'FeedIcon', template: '<i />' },
}))

const FEED = { id: 'f1', title: '华尔街见闻-要闻', url: 'https://example.com', category: '', description: '' } as RssFeed

function stats(partial: Partial<FeedBoardHitStats>): FeedBoardHitStats {
  return {
    feed_id: 1,
    title: '华尔街见闻-要闻',
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

function mountBlock(props: Record<string, unknown> = {}) {
  return mount(FeedSourceQualityBlock, {
    props: { feed: FEED, stats: stats({ articles: 1662, in_board: 437 }), ...props },
  })
}

describe('FeedSourceQualityBlock — 三分解与率（B6-15）', () => {
  it('1662/437/739/486/0 → 堆叠条四段宽度比例正确、图例含四数值、率文案 26.3%（437 / 1662）', () => {
    const wrapper = mountBlock({
      stats: stats({
        articles: 1662,
        in_board: 437,
        tagged_no_board: 739,
        untagged_pending: 486,
        untagged_settled: 0,
        hit_rate: 437 / 1662,
      }),
      windowDays: 7,
    })

    const widths = Object.fromEntries(
      wrapper.findAll('.fsq-stackbar-seg').map(seg => [
        seg.attributes('data-segment'),
        Number.parseFloat(seg.attributes('style')!.match(/width:\s*([\d.]+)%/)![1]!),
      ]),
    )
    expect(widths.in_board).toBeCloseTo((437 / 1662) * 100, 5)
    expect(widths.tagged_no_board).toBeCloseTo((739 / 1662) * 100, 5)
    expect(widths.untagged_pending).toBeCloseTo((486 / 1662) * 100, 5)
    expect(widths.untagged_settled).toBeCloseTo(0, 5)

    const legend = wrapper.find('.fsq-legend').text()
    expect(legend).toContain('入板块 437')
    expect(legend).toContain('有标签无板块 739')
    expect(legend).toContain('打标排队中 486')
    expect(legend).toContain('已处理无标签 0')

    const rate = wrapper.find('.fsq-rate').text()
    expect(rate).toContain('26.3%')
    expect(rate).toContain('（437 / 1662）')
  })
})

describe('FeedSourceQualityBlock — 板块分布 chips（B6-16）', () => {
  it('8 个板块 → Top 6 chips（篇数降序）+ 「其他 2 个板块」', () => {
    const boards = [
      { board_id: 1, label: '美国新闻', articles: 142 },
      { board_id: 2, label: '中国国内新闻', articles: 118 },
      { board_id: 3, label: '中东地缘政治与美伊关系', articles: 61 },
      { board_id: 4, label: '俄乌冲突', articles: 48 },
      { board_id: 5, label: '生成式 AI 与大模型厂商', articles: 39 },
      { board_id: 6, label: '台湾新闻', articles: 29 },
      { board_id: 7, label: '美联储与全球经济', articles: 12 },
      { board_id: 8, label: '科技公司与供应链', articles: 5 },
    ]
    const wrapper = mountBlock({
      stats: stats({ articles: 1662, in_board: 437, hit_rate: 437 / 1662, boards }),
    })
    const chips = wrapper.findAll('.fsq-bchip')
    expect(chips).toHaveLength(7)
    expect(chips[0]!.text()).toContain('美国新闻')
    expect(chips[0]!.text()).toContain('142')
    expect(chips[5]!.text()).toContain('台湾新闻')
    expect(chips[6]!.text()).toContain('其他 2 个板块')
  })
})

describe('FeedSourceQualityBlock — 空态 / 样本不足 / 打标关闭（B6-17..19）', () => {
  it('B6-17: 窗口内 0 篇 → 「近 7 天没有新文章」，不渲染堆叠条', () => {
    const wrapper = mountBlock({ stats: stats({ articles: 0, hit_rate: 0 }), windowDays: 7 })
    expect(wrapper.find('[data-testid="fsq-empty"]').text()).toContain('近 7 天没有新文章')
    expect(wrapper.find('.fsq-stackbar').exists()).toBe(false)
  })

  it('B6-18: 窗口内 3 篇 → 「样本不足（3 篇）」，不给百分比', () => {
    const wrapper = mountBlock({ stats: stats({ articles: 3, in_board: 1, hit_rate: 1 / 3 }) })
    const state = wrapper.find('[data-testid="fsq-low-sample"]')
    expect(state.text()).toContain('样本不足（3 篇）')
    expect(state.text()).not.toContain('%')
  })

  it('B6-19: tagging_enabled=false → 「该源已关闭打标，不参与板块归属」，块内不含 0% 率', () => {
    const wrapper = mountBlock({
      stats: stats({ articles: 1662, in_board: 0, hit_rate: 0, tagging_enabled: false }),
    })
    const off = wrapper.find('[data-testid="fsq-off"]')
    expect(off.exists()).toBe(true)
    expect(off.text()).toContain('该源已关闭打标，不参与板块归属')
    expect(wrapper.text()).not.toContain('%')
    expect(wrapper.find('.fsq-stackbar').exists()).toBe(false)
  })
})

describe('FeedSourceQualityBlock — 失败与窗口控件（B6-20 块内 + B6-14 块侧）', () => {
  it('B6-20: 统计失败 → 块内「统计加载失败」+ 重试按钮（点击通知父级重取）', async () => {
    // 注：本机 VTU 2.4.6 + Vue 3.5 组合下 wrapper.emitted() 不记录自定义事件（环境级问题，
    // 与本 change 无关），故用监听器 prop 断言。
    const onRetry = vi.fn()
    const wrapper = mountBlock({ stats: undefined, error: '网络错误', onRetry })
    const err = wrapper.find('[data-testid="fsq-error"]')
    expect(err.text()).toContain('统计加载失败')
    expect(err.text()).toContain('网络错误')

    await wrapper.find('[data-testid="fsq-retry"]').trigger('click')
    expect(onRetry).toHaveBeenCalledTimes(1)

    // 重试期间 → 骨架占位
    await wrapper.setProps({ error: null, loading: true })
    expect(wrapper.find('[data-testid="fsq-loading"]').exists()).toBe(true)
  })

  it('窗口分段控件点击通知父级切窗口（与列表共享状态的事件出口）', async () => {
    const onSetWindow = vi.fn()
    const wrapper = mountBlock({ windowDays: 7, onSetWindow })
    await wrapper.find('.fsq-seg-btn[data-window="90"]').trigger('click')
    expect(onSetWindow).toHaveBeenCalledWith(90)
  })
})

// —— B6-20 装配层：块失败不阻断下方设置表单（FeedDetailEditor 内结构验证）——
function mountEditor(editorProps: Record<string, unknown> = {}) {
  return mount(FeedDetailEditor, {
    props: {
      feed: FEED,
      categories: [],
      refreshOptions: [{ label: '每小时', value: 60 }],
      maxArticlesOptions: [{ label: '100 篇', value: 100 }],
      loading: false,
      ...editorProps,
    },
  })
}

describe('FeedDetailEditor 装配 — 来源质量块位置与失败不阻断（B6-20）', () => {
  const retrySpy = vi.fn()

  it('块位于状态条之后、设置表单之前；统计失败时表单仍可交互', async () => {
    const wrapper = mountEditor({
      stats: undefined,
      statsLoading: false,
      statsError: '后端 500',
      windowDays: 7,
    })

    const root = wrapper.find('.feed-detail')
    const childClasses = root.element.children.length
    expect(childClasses).toBeGreaterThanOrEqual(4)

    // 结构顺序：header → status → 质量块 → form
    const order = Array.from(root.element.children).map(el => el.className)
    expect(order.findIndex(c => c.includes('feed-detail__status'))).toBeLessThan(
      order.findIndex(c => c.includes('fsq-block') || c.includes('feed-source-quality')),
    )
    const blockIndex = order.findIndex(c => c.includes('fsq-block'))
    const formIndex = order.findIndex(c => c.includes('feed-detail__form'))
    expect(blockIndex).toBeGreaterThan(-1)
    expect(formIndex).toBeGreaterThan(blockIndex)

    // 块内失败提示 + 冒泡到编辑器的重试出口
    expect(wrapper.find('[data-testid="fsq-error"]').text()).toContain('统计加载失败')
    const retryFromEditor = mountEditor({
      stats: undefined,
      statsLoading: false,
      statsError: '后端 500',
      windowDays: 7,
      onRetryStats: retrySpy,
    })
    await retryFromEditor.find('[data-testid="fsq-retry"]').trigger('click')
    expect(retrySpy).toHaveBeenCalledTimes(1)

    // 设置表单元素仍在且未被禁用（统计失败不影响其可用性）
    for (const select of wrapper.findAll('select.feed-detail__select')) {
      expect(select.attributes('disabled')).toBeUndefined()
    }
    await wrapper.findAll('select.feed-detail__select')[0]!.setValue('')
    expect(wrapper.find('.feed-detail__form').exists()).toBe(true)
  })
})
