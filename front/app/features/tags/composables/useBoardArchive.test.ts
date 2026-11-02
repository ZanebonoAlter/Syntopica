import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import type { BoardArticleTag } from '~/api/semanticBoards'
import {
  archiveStats,
  filterArchiveArticles,
  groupArchiveArticles,
  isWeakArticle,
  normalizeArchiveRange,
  ruleMeta,
  sortArchiveArticles,
  useBoardArchive,
  type ArchiveArticle,
} from './useBoardArchive'

const { apiMock } = vi.hoisted(() => ({
  apiMock: { getBoardArticles: vi.fn() },
}))

vi.mock('~/api/semanticBoards', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/semanticBoards')>()
  return { ...original, useSemanticBoardsApi: () => apiMock }
})

function tag(overrides: Partial<BoardArticleTag> = {}): BoardArticleTag {
  return {
    id: 7,
    label: '美伊关系',
    category: 'event',
    match_reason: 'direct_hit',
    score: 0.9,
    downgraded: false,
    direction_mismatch: false,
    ...overrides,
  }
}

function article(
  id: number,
  overrides: Partial<ArchiveArticle> & { tags?: BoardArticleTag[] } = {},
): ArchiveArticle {
  const { tags, ...rest } = overrides
  return {
    id,
    title: `文章 ${id}`,
    url: '',
    link: `https://example.com/${id}`,
    pub_date: '2026-10-20 08:00:00',
    feed_id: 1,
    feed_name: '测试源',
    filtered_tags: tags ?? [tag()],
    ...rest,
  }
}

describe('归档日期与文章顺序', () => {
  it('日期归一：空/非法回退今天，start > end 自动交换', () => {
    const fallback = normalizeArchiveRange('', '')
    expect(fallback.start).toBe(fallback.end)
    expect(fallback.start).toMatch(/^\d{4}-\d{2}-\d{2}$/)
    expect(normalizeArchiveRange('2026-10-24', '2026-10-20')).toEqual({ start: '2026-10-20', end: '2026-10-24' })
  })

  it('文章按完整发布时间倒序，同日不按入库 id 打乱', () => {
    const sorted = sortArchiveArticles([
      article(1, { pub_date: '2026-10-21 09:02:00' }),
      article(3, { pub_date: '2026-10-21 23:20:00' }),
      article(2, { pub_date: '2026-10-21 20:19:00' }),
      article(4, { pub_date: '2026-10-21 20:19:00' }),
    ])
    expect(sorted.map((item) => item.id)).toEqual([3, 4, 2, 1])
  })
})

describe('归档日期分组（Map 聚合）', () => {
  it('交错日期不碎组：按日期聚合，组间日期倒序', () => {
    const groups = groupArchiveArticles([
      article(1, { pub_date: '2026-10-21 09:00:00' }),
      article(2, { pub_date: '2026-10-20 09:00:00' }),
      article(3, { pub_date: '2026-10-21 20:00:00' }),
      article(4, { pub_date: '2026-10-19 09:00:00' }),
      article(5, { pub_date: '2026-10-21 12:00:00' }),
    ])
    expect(groups.map((group) => group.date)).toEqual(['2026-10-21', '2026-10-20', '2026-10-19'])
    expect(groups.map((group) => group.articles.map((item) => item.id))).toEqual([[1, 3, 5], [2], [4]])
  })

  it('quality 模式组内保持后端原序（前端不重排）', () => {
    const groups = groupArchiveArticles([
      article(1, { pub_date: '2026-10-21 23:00:00' }),
      article(2, { pub_date: '2026-10-21 09:00:00' }),
      article(3, { pub_date: '2026-10-21 15:00:00' }),
    ], 'quality')
    expect(groups.map((group) => group.articles.map((item) => item.id))).toEqual([[1, 2, 3]])
  })

  it('time 模式组内发布时间倒序，同刻 id 倒序兜底', () => {
    const groups = groupArchiveArticles([
      article(1, { pub_date: '2026-10-21 09:00:00' }),
      article(3, { pub_date: '2026-10-21 20:00:00' }),
      article(2, { pub_date: '2026-10-21 20:00:00' }),
      article(4, { pub_date: '2026-10-21 12:00:00' }),
    ], 'time')
    expect(groups.map((group) => group.articles.map((item) => item.id))).toEqual([[3, 2, 4, 1]])
  })
})

describe('筛选、统计与弱证据', () => {
  const articles = [
    article(1, { tags: [tag({ id: 7 })], in_report: true }),
    article(2, { tags: [tag({ id: 7, match_reason: 'max_sim' })], in_report: false }),
    article(3, { tags: [], in_report: true }),
    article(4, { tags: [tag({ id: 9, label: '强证据别的组' })], in_report: true }),
  ]

  it('未纳入日报只看文章本身，不裁同组其它文章', () => {
    expect(filterArchiveArticles(articles, 'not_in_report').map((item) => item.id)).toEqual([2])
  })

  it('弱证据筛选按单篇判定（无标签 / max_sim / 降级）', () => {
    expect(filterArchiveArticles(articles, 'weak').map((item) => item.id)).toEqual([2, 3])
  })

  it('统计口径：总数 / 已入 / 未入 / 弱证据', () => {
    expect(archiveStats(articles)).toEqual({
      total: 4,
      inReport: 3,
      notInReport: 1,
      weak: 2,
    })
  })

  it('弱证据回退：无字段时按标签判断', () => {
    expect(isWeakArticle(article(1, { tags: [] }))).toBe(true)
    expect(isWeakArticle(article(2, { tags: [tag({ downgraded: true })] }))).toBe(true)
    expect(isWeakArticle(article(3, { tags: [tag()] }))).toBe(false)
    expect(isWeakArticle(article(4, { tags: [tag({ match_reason: 'weighted' })], weak_evidence: false }))).toBe(false)
    expect(isWeakArticle(article(5, { tags: [tag(), tag({ match_reason: 'max_sim' })] }))).toBe(false)
  })
})

describe('useBoardArchive — 按日期取数', () => {
  beforeEach(() => {
    apiMock.getBoardArticles.mockReset()
  })

  function mountHost(start = '2026-10-23', end = '2026-10-23') {
    const Host = defineComponent({
      setup() {
        const state = useBoardArchive(ref(1974), ref(start), ref(end))
        return { state }
      },
      template: '<div>{{ state.stats.value.total }}</div>',
    })
    return mount(Host)
  }

  it('只请求指定日期并分页拉全', async () => {
    apiMock.getBoardArticles
      .mockResolvedValueOnce({ success: true, data: [article(1, { in_report: true })], pagination: { pages: 2, total: 2 } })
      .mockResolvedValueOnce({ success: true, data: [article(2, { in_report: false })], pagination: { pages: 2, total: 2 } })
    const wrapper = mountHost()
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenCalledTimes(2)
    expect(apiMock.getBoardArticles).toHaveBeenNthCalledWith(1, 1974, {
      start_date: '2026-10-23',
      end_date: '2026-10-23',
      page: 1,
      per_page: 100,
      sort: 'quality',
    })
    expect(wrapper.text()).toBe('2')
  })

  it('默认质量序取数，切换排序重拉并带对应 sort 参数', async () => {
    apiMock.getBoardArticles.mockResolvedValue({ success: true, data: [article(1)], pagination: { pages: 1 } })
    const wrapper = mountHost()
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenCalledTimes(1)
    expect(apiMock.getBoardArticles).toHaveBeenNthCalledWith(1, 1974, expect.objectContaining({ sort: 'quality' }))

    wrapper.vm.state.setSortMode('time')
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenCalledTimes(2)
    expect(apiMock.getBoardArticles).toHaveBeenLastCalledWith(1974, expect.objectContaining({ sort: 'time' }))
    expect(wrapper.vm.state.sortMode.value).toBe('time')
  })

  it('分组暴露：groupedArticles 按日期聚合且组内随排序模式', async () => {
    apiMock.getBoardArticles.mockResolvedValue({
      success: true,
      data: [
        article(1, { pub_date: '2026-10-23 09:00:00' }),
        article(2, { pub_date: '2026-10-22 09:00:00' }),
        article(3, { pub_date: '2026-10-23 20:00:00' }),
      ],
      pagination: { pages: 1 },
    })
    const wrapper = mountHost()
    await flushPromises()
    const groups = wrapper.vm.state.groupedArticles.value
    expect(groups.map((group) => group.date)).toEqual(['2026-10-23', '2026-10-22'])
    expect(groups.map((group) => group.articles.map((item) => item.id))).toEqual([[1, 3], [2]])
  })

  it('后端乱序输入 → quality 模式组内相对序保持不变（前端不重排）', async () => {
    apiMock.getBoardArticles.mockResolvedValue({
      success: true,
      data: [
        article(3, { pub_date: '2026-10-23 08:00:00' }),
        article(1, { pub_date: '2026-10-23 09:00:00' }),
        article(2, { pub_date: '2026-10-23 15:00:00' }),
      ],
      pagination: { pages: 1 },
    })
    const wrapper = mountHost()
    await flushPromises()
    const groups = wrapper.vm.state.groupedArticles.value
    expect(groups).toHaveLength(1)
    expect(groups[0]?.articles.map((item) => item.id)).toEqual([3, 1, 2])
  })

  it('失败：保留错误与旧数据，不静默清空', async () => {
    apiMock.getBoardArticles
      .mockResolvedValueOnce({ success: true, data: [article(1)], pagination: { pages: 1 } })
      .mockResolvedValueOnce({ success: false, error: '后端挂了' })
    const wrapper = mountHost()
    await flushPromises()
    await wrapper.vm.state.reload()
    await flushPromises()
    expect(wrapper.vm.state.error.value).toBe('后端挂了')
    expect(wrapper.vm.state.articles.value).toHaveLength(1)
  })

  it('规则语义表覆盖五类匹配 + 未知回退', () => {
    expect(ruleMeta('max_sim').semantics).toContain('弱证据')
    expect(ruleMeta('direct_hit').label).toBe('直接命中')
    expect(ruleMeta('weighted').label).toBe('综合')
    expect(ruleMeta('unknown').semantics.length).toBeGreaterThan(0)
  })
})
