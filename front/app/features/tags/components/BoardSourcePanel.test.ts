/**
 * BoardSourcePanel — add-source-board-hit-rate T5 / B6-21..25。
 *
 * 组件层：竞态防护（旧板块请求不覆盖新选中板块）、汇总行/5 列来源表/只读无写动作、
 * 空态引导切窗口、排序纯前端重排、失败重试自愈。
 * 装配层（旧 TagsPage 宿主已随 8.4 退役删除）：新宿主断言由
 * `console/BoardGovernanceDrawer.test.ts`（来源 tab 渲染）与
 * `console/BoardWideWorkface.test.ts` 承接。
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import BoardSourcePanel from './BoardSourcePanel.vue'
import type { BoardSourceBreakdown } from '~/types'

const { getBoardSourceBreakdownMock } = vi.hoisted(() => ({
  getBoardSourceBreakdownMock: vi.fn(),
}))
vi.mock('~/api/semanticBoards', () => ({
  useSemanticBoardsApi: () => ({ getBoardSourceBreakdown: getBoardSourceBreakdownMock }),
}))
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span />' },
}))

function src(
  feedId: number,
  title: string,
  articles: number,
  feedArticles: number,
  feedHitRate: number,
  total: number,
): BoardSourceBreakdown['sources'][number] {
  return {
    feed_id: feedId,
    title,
    articles,
    share: articles / total,
    feed_articles: feedArticles,
    feed_hit_rate: feedHitRate,
  }
}

function resp(
  total: number,
  sources: BoardSourceBreakdown['sources'],
): { success: true; data: BoardSourceBreakdown } {
  return { success: true, data: { total_articles: total, source_count: sources.length, sources } }
}

const BOARD_5_SOURCES = [
  src(1, '华尔街见闻-要闻', 220, 1662, 0.263, 437),
  src(2, '资讯_凤凰网', 130, 499, 0.84, 437),
  src(3, '华尔街见闻-最热文章', 87, 67, 0.07, 437),
]

function mountPanel(boardId = 5) {
  return mount(BoardSourcePanel, { props: { boardId } })
}

function rowTitles(wrapper: ReturnType<typeof mountPanel>): string[] {
  return wrapper.findAll('[data-testid="board-source-row"]').map(row =>
    row.find('.bsp-source-title').text(),
  )
}

beforeEach(() => {
  getBoardSourceBreakdownMock.mockReset()
})

describe('BoardSourcePanel — 来源表渲染（B6-22）', () => {
  it('汇总行含本板块篇数/来源数/最大来源；表格 5 列齐全（34/12/22/16/16）；行内无写动作', async () => {
    getBoardSourceBreakdownMock.mockResolvedValue(resp(437, BOARD_5_SOURCES))
    const wrapper = mountPanel()
    await flushPromises()

    // 端点 URL 与默认窗口
    expect(getBoardSourceBreakdownMock).toHaveBeenCalledWith(5, 7)

    const summary = wrapper.find('.bsp-header').text()
    expect(summary).toContain('本板块')
    expect(summary).toContain('437')
    expect(summary).toContain('来源')
    expect(summary).toContain('3 个')
    expect(summary).toContain('最大来源')
    expect(summary).toContain('华尔街见闻-要闻')
    expect(summary).toContain('（50%）')

    // 表头 5 列，百分比列宽（happy-dom 序列化 inline style 带尾分号，统一剥掉再比）
    const headers = wrapper.findAll('thead th')
    expect(headers).toHaveLength(5)
    expect(headers.map(h => (h.attributes('style') ?? '').replace(/;\s*$/, ''))).toEqual([
      'width: 34%',
      'width: 12%',
      'width: 22%',
      'width: 16%',
      'width: 16%',
    ])

    // 行内容：篇数 / 占比条+% / 该源入板块率 pill / 该源窗口内总量
    const firstRow = wrapper.find('[data-testid="board-source-row"]')
    const cells = firstRow.findAll('td')
    expect(cells[1]!.text()).toBe('220')
    expect(cells[2]!.find('.bsp-share-fill').attributes('style')).toMatch(/width: 50\.34\d*%/)
    expect(cells[2]!.text()).toContain('50%')
    expect(cells[3]!.text()).toBe('26%')
    expect(cells[3]!.find('.feed-source-pill').classes()).toContain('feed-source-pill--low')
    expect(cells[4]!.text()).toBe('1662')

    // 只读硬约定：行内无任何按钮/链接动作
    expect(wrapper.find('tbody button').exists()).toBe(false)
    expect(wrapper.find('tbody a').exists()).toBe(false)

    // 口径脚注（多板块各计一次 + 只读说明）
    const footnote = wrapper.find('.bsp-footnote').text()
    expect(footnote).toContain('同一篇文章命中多个板块时在每个板块各计一次')
    expect(footnote).toContain('只读面板')
  })
})

describe('BoardSourcePanel — 排序（B6-24）', () => {
  it('切「按该源入板块率 ↑」→ 行按 feed_hit_rate 升序（纯前端重排，不重发请求）', async () => {
    getBoardSourceBreakdownMock.mockResolvedValue(resp(437, BOARD_5_SOURCES))
    const wrapper = mountPanel()
    await flushPromises()
    expect(rowTitles(wrapper)).toEqual(['华尔街见闻-要闻', '资讯_凤凰网', '华尔街见闻-最热文章'])

    await wrapper.find('[data-testid="board-source-sort"]').setValue('hit_rate')
    await nextTick()
    // 率升序：最热文章 0.07 → 要闻 0.263 → 凤凰网 0.84；请求不重发（纯前端重排）
    expect(rowTitles(wrapper)).toEqual(['华尔街见闻-最热文章', '华尔街见闻-要闻', '资讯_凤凰网'])
    expect(getBoardSourceBreakdownMock).toHaveBeenCalledTimes(1)
  })
})

describe('BoardSourcePanel — 空态与切窗口（B6-23）', () => {
  it('窗口内 0 篇 → 「近 7 天没有文章归入本板块」+ 切窗口提示；切 30 天重取并恢复', async () => {
    getBoardSourceBreakdownMock.mockResolvedValue(resp(0, []))
    const wrapper = mountPanel()
    await flushPromises()

    const empty = wrapper.find('[data-testid="board-source-empty"]')
    expect(empty.text()).toContain('近 7 天没有文章归入本板块')
    expect(empty.text()).toContain('30 天')
    expect(empty.text()).toContain('90 天')
    expect(wrapper.find('.bsp-table').exists()).toBe(false)

    // 空态引导：切 30 天 → 按新窗口重取
    getBoardSourceBreakdownMock.mockResolvedValue(resp(88, [src(1, '华尔街见闻-要闻', 88, 1662, 0.263, 88)]))
    await wrapper.find('.bsp-seg-btn[data-window="30"]').trigger('click')
    await flushPromises()
    expect(getBoardSourceBreakdownMock).toHaveBeenLastCalledWith(5, 30)
    expect(wrapper.find('[data-testid="board-source-empty"]').exists()).toBe(false)
    expect(rowTitles(wrapper)).toEqual(['华尔街见闻-要闻'])
  })
})

describe('BoardSourcePanel — 竞态防护（B6-21）', () => {
  it('快速切换板块：旧板块的迟到响应不得覆盖新选中板块的面板', async () => {
    let resolveBoard5!: (v: unknown) => void
    getBoardSourceBreakdownMock.mockImplementation((boardId: number) => {
      if (boardId === 5) {
        return new Promise((resolve) => { resolveBoard5 = resolve })
      }
      return Promise.resolve(resp(10, [src(9, '板块8独有源', 10, 20, 0.5, 10)]))
    })

    const wrapper = mountPanel(5)
    await nextTick() // loading 置位发生在 onMounted（首帧渲染后），DOM 更新需等一个 tick
    expect(wrapper.find('[data-testid="board-source-loading"]').exists()).toBe(true)

    // 板块 5 请求未返回时切到板块 8 → 板块 8 数据先渲染
    await wrapper.setProps({ boardId: 8 })
    await flushPromises()
    expect(getBoardSourceBreakdownMock).toHaveBeenNthCalledWith(2, 8, 7)
    expect(rowTitles(wrapper)).toEqual(['板块8独有源'])

    // 板块 5 的迟到响应到达 → 丢弃，不覆盖
    resolveBoard5(resp(437, BOARD_5_SOURCES))
    await flushPromises()
    expect(rowTitles(wrapper)).toEqual(['板块8独有源'])
    expect(wrapper.text()).not.toContain('华尔街见闻-要闻')
    expect(wrapper.find('[data-testid="board-source-loading"]').exists()).toBe(false)
  })
})

describe('BoardSourcePanel — 失败重试（B6-25）', () => {
  it('失败 → 块内「统计加载失败」+ 重试；重试成功后恢复表格；重挂载（切 tab 回来）重新拉取', async () => {
    getBoardSourceBreakdownMock
      .mockResolvedValueOnce({ success: false, error: '后端 500' })
      .mockResolvedValueOnce(resp(437, BOARD_5_SOURCES))
      .mockResolvedValueOnce(resp(437, BOARD_5_SOURCES))

    const wrapper = mountPanel()
    await flushPromises()
    const err = wrapper.find('[data-testid="board-source-error"]')
    expect(err.text()).toContain('统计加载失败')
    expect(wrapper.find('.bsp-table').exists()).toBe(false)

    await wrapper.find('[data-testid="board-source-retry"]').trigger('click')
    await flushPromises()
    expect(getBoardSourceBreakdownMock).toHaveBeenCalledTimes(2)
    expect(rowTitles(wrapper)).toEqual(['华尔街见闻-要闻', '资讯_凤凰网', '华尔街见闻-最热文章'])

    // 切其它 tab（卸载）→ 切回「板块内容」（重挂载）→ 重新拉取且状态正常
    wrapper.unmount()
    const remounted = mountPanel()
    await flushPromises()
    expect(getBoardSourceBreakdownMock).toHaveBeenCalledTimes(3)
    expect(remounted.find('[data-testid="board-source-error"]').exists()).toBe(false)
    expect(remounted.find('[data-testid="board-source-row"]')).toBeTruthy()
  })
})

