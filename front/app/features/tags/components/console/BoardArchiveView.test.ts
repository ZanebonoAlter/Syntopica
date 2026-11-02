import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { BoardArticleTag } from '~/api/semanticBoards'
import BoardArchiveView from './BoardArchiveView.vue'
import type { ArchiveArticle } from '../../composables/useBoardArchive'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

const { apiMock, articlesApi } = vi.hoisted(() => ({
  apiMock: {
    getBoardArticles: vi.fn(),
    getMatchingConfig: vi.fn(),
    getMatchDetail: vi.fn(),
    getTagAuxiliaryLabels: vi.fn(),
    getComposition: vi.fn(),
  },
  articlesApi: {
    getArticle: vi.fn(),
  },
}))

vi.mock('~/api/semanticBoards', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/semanticBoards')>()
  return { ...original, useSemanticBoardsApi: () => apiMock }
})

vi.mock('~/api/articles', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/articles')>()
  return { ...original, useArticlesApi: () => articlesApi }
})

function tag(overrides: Partial<BoardArticleTag> = {}): BoardArticleTag {
  return {
    id: 7,
    label: '美伊关系',
    category: 'event',
    description: '伊朗与美国之间的双边关系。',
    match_reason: 'direct_hit',
    score: 0.92,
    downgraded: false,
    direction_mismatch: false,
    ...overrides,
  }
}

function article(id: number, overrides: Partial<ArchiveArticle> = {}): ArchiveArticle {
  return {
    id,
    title: `文章 ${id}`,
    url: '',
    link: `https://example.com/${id}`,
    pub_date: '2026-10-23 08:00:00',
    feed_id: 1,
    feed_name: '测试源',
    filtered_tags: [tag()],
    ...overrides,
  }
}

const ARTICLES: ArchiveArticle[] = [
  article(1, { in_report: true, pub_date: '2026-10-23 08:00:00', weak_evidence: false }),
  article(2, { in_report: false, pub_date: '2026-10-23 09:00:00', weak_evidence: false }),
  article(3, {
    in_report: false,
    pub_date: '2026-10-22 08:00:00',
    weak_evidence: true,
    filtered_tags: [tag({ id: 8, label: '霍尔木兹', match_reason: 'max_sim', score: 0.62, downgraded: true, direction_mismatch: true })],
  }),
  article(4, { in_report: false, pub_date: '2026-10-22 09:00:00', weak_evidence: true, filtered_tags: [] }),
  article(5, {
    in_report: true,
    pub_date: '2026-10-21 08:00:00',
    weak_evidence: false,
    // 后端归档证据序：direct_hit > composite_hit > hit_rate > weighted > max_sim。
    // 首条（direct_hit 0.62）即最强证据，刻意与最高分（weighted 0.91）不同——锁住「信后端序」口径。
    filtered_tags: [
      tag({ id: 9, label: '沙特', match_reason: 'direct_hit', score: 0.62 }),
      tag({ id: 10, label: '霍尔木兹', match_reason: 'hit_rate', score: 0.5 }),
      tag({ id: 11, label: '伊朗', match_reason: 'weighted', score: 0.91 }),
      tag({ id: 12, label: '美国', match_reason: 'max_sim', score: 0.45 }),
    ],
  }),
]

function mountView() {
  return mount(BoardArchiveView, {
    props: { boardId: 1974, asOf: '2026-10-23' },
    attachTo: document.body,
  })
}

async function showFullRange(wrapper: ReturnType<typeof mountView>) {
  await wrapper.find('[data-testid="archive-start-date"] input').setValue('2026-10-18')
  await flushPromises()
}

beforeEach(() => {
  vi.clearAllMocks()
  document.body.innerHTML = ''
  apiMock.getBoardArticles.mockResolvedValue({
    success: true,
    data: ARTICLES,
    pagination: { page: 1, per_page: 100, total: ARTICLES.length, pages: 1 },
  })
  apiMock.getMatchingConfig.mockResolvedValue({
    success: true,
    data: {
      semantic_board_match_sim_threshold: 0.72,
      semantic_board_match_direct_hit_min_overlap: 2,
      semantic_board_match_direct_hit_rate: 0.5,
      semantic_board_match_direct_max_sim: 0.8,
      semantic_board_match_direction_sim_threshold: 0.55,
    },
  })
  apiMock.getMatchDetail.mockResolvedValue({ success: false, error: '无数据' })
  apiMock.getTagAuxiliaryLabels.mockResolvedValue({ success: true, data: { total: 0, items: [] } })
  apiMock.getComposition.mockResolvedValue({ success: true, data: { total: 0, items: [] } })
  articlesApi.getArticle.mockResolvedValue({ success: true, data: { id: 1, tags: [] } })
})

describe('BoardArchiveView — 日期范围与分组', () => {
  it('默认只查数据锚点当天，并按日期分组展示文章', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenCalledWith(1974, expect.objectContaining({
      start_date: '2026-10-23',
      end_date: '2026-10-23',
      sort: 'quality',
    }))
    expect(wrapper.find('[data-testid="archive-stat-total"]').text()).toContain('5')
    expect(wrapper.find('[data-testid="archive-window"]').text()).toContain('2026-10-23')

    await showFullRange(wrapper)
    expect(apiMock.getBoardArticles).toHaveBeenLastCalledWith(1974, expect.objectContaining({
      start_date: '2026-10-18',
      end_date: '2026-10-23',
    }))
    const heads = wrapper.findAll('.ar-day-date').map(node => node.text())
    expect(heads).toEqual(['2026-10-23', '2026-10-22', '2026-10-21'])
  })

  it('快捷范围使用自由日期起止', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.find('[data-testid="archive-preset-7"]').trigger('click')
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenLastCalledWith(1974, expect.objectContaining({
      start_date: '2026-10-17',
      end_date: '2026-10-23',
    }))
  })

  it('组头显示日期、篇数与弱证据计数', async () => {
    const wrapper = mountView()
    await flushPromises()
    await showFullRange(wrapper)
    const dayHead = wrapper.findAll('.ar-day-head')[1]
    expect(dayHead?.text()).toContain('2026-10-22')
    expect(dayHead?.text()).toContain('2 篇')
    expect(dayHead?.text()).toContain('弱证据 2')
  })

  it('deck 行给出范围 / 总数 / 已进比例 / 未进 / 弱证据交叉口径（一句话可读）', async () => {
    const wrapper = mountView()
    await flushPromises()
    const deck = wrapper.find('[data-testid="archive-window"]')
    expect(deck.text()).toContain('2026-10-23')
    expect(wrapper.find('[data-testid="archive-stat-total"]').text()).toContain('5 篇')
    // 已进 2 / 5 = 40%，比例条与百分比同时给出
    expect(wrapper.find('[data-testid="archive-stat-in"]').text()).toContain('2')
    expect(wrapper.find('[data-testid="archive-stat-in-pct"]').text()).toBe('40%')
    expect(wrapper.find('[data-testid="archive-stat-out"]').text()).toContain('3')
    // 弱证据不是第四个桶：同一格里明说可与入报交叉
    expect(wrapper.find('[data-testid="archive-stat-weak"]').text()).toContain('2')
    expect(wrapper.find('[data-testid="archive-stat-weak"]').text()).toContain('交叉')
    // 口径两句直接可见，不靠 hover
    expect(deck.text()).toContain('已入 = 出现在范围内任一日报分区线索')
    expect(deck.text()).toContain('无命中标签的文章不会进入日报')

    await showFullRange(wrapper)
    expect(wrapper.find('[data-testid="archive-window"]').text()).toContain('2026-10-18 → 2026-10-23')
  })
})

describe('BoardArchiveView — 行渲染（四列 / 标签带 / 入板 chip）', () => {
  it('行内按规则色显示标签 chip、分数、最强匹配与状态', async () => {
    const wrapper = mountView()
    await flushPromises()
    await showFullRange(wrapper)
    const row = wrapper.find('[data-testid="archive-row-5"]')
    expect(row.find('[data-testid="archive-tag-5-9"]').text()).toContain('沙特')
    expect(row.find('[data-testid="archive-tag-5-9"]').text()).toContain('0.62')
    // chip 只露标签名 + 分数（规则用颜色编码 + hover 提示表达）
    expect(row.find('[data-testid="archive-tag-5-10"]').text()).toContain('霍尔木兹')
    expect(row.find('[data-testid="archive-tag-5-10"]').text()).toContain('0.50')
    expect(row.find('[data-testid="archive-tag-5-10"]').attributes('title')).toContain('命中率')
    expect(row.find('[data-testid="archive-tag-5-10"]').attributes('style')).toContain('border-color')
    // 降级标签半透明、方向不符虚线边
    const mismatch = wrapper.find('[data-testid="archive-tag-3-8"]')
    expect(mismatch.classes()).toContain('is-deg')
    expect(mismatch.classes()).toContain('is-mismatch')
    expect(mismatch.text()).toContain('⊘')
    // 最强匹配列：规则名 + 分数（规则色），取后端组内首条（filtered_tags[0]）
    const strongest = row.find('[data-testid="archive-strongest"]').text()
    expect(strongest).toContain('直接命中')
    expect(strongest).toContain('0.62')
    // 最高分的 weighted 0.91 不是最强证据（后端层级序说了算）
    expect(strongest).not.toContain('综合')
    // 状态列
    expect(row.find('[data-testid="archive-status"]').text()).toContain('已入')
    expect(wrapper.find('[data-testid="archive-row-2"]').find('[data-testid="archive-status"]').text()).toContain('未入日报')
    expect(wrapper.find('[data-testid="archive-row-3"]').find('[data-testid="archive-status"]').text()).toContain('弱证据')
  })

  it('最强匹配与入板步号取后端组内首条，不按最高分推导', async () => {
    apiMock.getBoardArticles.mockResolvedValue({
      success: true,
      data: [article(6, {
        filtered_tags: [
          tag({ id: 20, label: '直接命中标签', match_reason: 'direct_hit', score: 0.62 }),
          tag({ id: 21, label: '加权标签', match_reason: 'weighted', score: 0.91 }),
        ],
      })],
      pagination: { page: 1, per_page: 100, total: 1, pages: 1 },
    })
    const wrapper = mountView()
    await flushPromises()
    const strongest = wrapper.find('[data-testid="archive-strongest"]').text()
    expect(strongest).toContain('直接命中')
    expect(strongest).toContain('0.62')
    expect(strongest).not.toContain('综合')
    // 入板步号跟着最强判定（① 精确），不跟着最高分标签（⑤ 加权）
    const flowChip = wrapper.find('[data-testid="archive-flow-chip-6"]')
    expect(flowChip.text()).toContain('①')
    expect(flowChip.text()).toContain('精确')
    expect(flowChip.text()).not.toContain('⑤')
  })

  it('标签 chip 超过 3 个折叠为「+N」，点开列全', async () => {
    const wrapper = mountView()
    await flushPromises()
    await showFullRange(wrapper)
    const more = wrapper.find('[data-testid="archive-tag-more-5"]')
    expect(more.text()).toContain('+1')
    expect(wrapper.find('[data-testid="archive-tag-5-12"]').exists()).toBe(false)
    await more.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="archive-tag-5-12"]').exists()).toBe(true)
  })

  it('有命中标签的行在标签带末尾挂「入板」chip（步号 + 规则短名）', async () => {
    const wrapper = mountView()
    await flushPromises()
    await showFullRange(wrapper)
    const flowChip = wrapper.find('[data-testid="archive-flow-chip-5"]')
    expect(flowChip.text()).toContain('入板')
    expect(flowChip.text()).toContain('①')
    expect(flowChip.text()).toContain('精确')
    // 无标签文章不挂入板 chip
    expect(wrapper.find('[data-testid="archive-flow-chip-4"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="archive-row-4"]').text()).toContain('无标签')
  })

  it('无标签口径：无标签灰 chip 与统计带都说明「不会进入日报」', async () => {
    const wrapper = mountView()
    await flushPromises()
    await showFullRange(wrapper)
    expect(wrapper.find('[data-testid="archive-row-4"] .ar-tag.is-empty').attributes('title')).toContain('不会进入日报')
    expect(wrapper.find('[data-testid="archive-window"]').text()).toContain('无命中标签的文章不会进入日报')
  })
})

describe('BoardArchiveView — 交互契约', () => {
  it('行主点击 emit open-article（不开自己的弹窗）', async () => {
    const wrapper = mountView()
    await flushPromises()
    await showFullRange(wrapper)
    await wrapper.find('[data-testid="archive-row-5"]').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('open-article')).toEqual([[5]])
  })

  it('点标签 chip 打开标签卡 popover，且不触发行点击', async () => {
    const wrapper = mountView()
    await flushPromises()
    await showFullRange(wrapper)
    await wrapper.find('[data-testid="archive-tag-5-9"]').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('open-article')).toBeUndefined()
    const pop = document.body.querySelector('[data-testid="archive-match-popover"]')
    expect(pop).not.toBeNull()
    expect(pop!.textContent).toContain('沙特')
    expect(pop!.textContent).toContain('标签') // 类别 chip / 描述区
  })

  it('点入板 chip 打开入板过程 popover', async () => {
    const wrapper = mountView()
    await flushPromises()
    await showFullRange(wrapper)
    await wrapper.find('[data-testid="archive-flow-chip-5"]').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('open-article')).toBeUndefined()
    const pop = document.body.querySelector('[data-testid="archive-match-popover"]')
    expect(pop).not.toBeNull()
    expect(pop!.textContent).toContain('本文入板过程')
  })
})

describe('BoardArchiveView — 筛选与排序', () => {
  it('未纳入日报 / 弱证据筛选按单篇生效', async () => {
    const wrapper = mountView()
    await flushPromises()
    await showFullRange(wrapper)
    await wrapper.find('[data-testid="archive-filter-not_in_report"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.ar-row')).toHaveLength(3)
    await wrapper.find('[data-testid="archive-filter-weak"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.ar-row')).toHaveLength(2)
    expect(wrapper.find('[data-testid="archive-row-3"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="archive-row-4"]').exists()).toBe(true)
  })

  it('筛选 chip 只做动作，计数只在 deck（chip 用 title 带计数，不重复占排版）', async () => {
    const wrapper = mountView()
    await flushPromises()
    const notInReport = wrapper.find('[data-testid="archive-filter-not_in_report"]')
    const weak = wrapper.find('[data-testid="archive-filter-weak"]')
    expect(notInReport.text()).toBe('未纳入日报')
    expect(weak.text()).toBe('弱证据')
    expect(notInReport.attributes('title')).toContain('3')
    expect(weak.attributes('title')).toContain('2')
    // 同一批数字在 deck 上只出现一次
    expect(wrapper.find('[data-testid="archive-stat-out"]').text()).toContain('3')
    expect(wrapper.find('[data-testid="archive-stat-weak"]').text()).toContain('2')
  })

  it('切换组内排序为时间：重拉并按 sort=time 请求', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('[data-testid="archive-sort-quality"]').attributes('aria-pressed')).toBe('true')
    await wrapper.find('[data-testid="archive-sort-time"]').trigger('click')
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenLastCalledWith(1974, expect.objectContaining({ sort: 'time' }))
    expect(wrapper.find('[data-testid="archive-sort-time"]').attributes('aria-pressed')).toBe('true')
  })

  it('「筛选」覆盖面收来源 / 构成标签 / 方向不符', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.find('[data-testid="archive-filter-open"]').trigger('click')
    await flushPromises()
    const dialog = document.body.querySelector('.app-dialog')
    expect(dialog).not.toBeNull()
    expect(dialog!.textContent).toContain('来源')
    expect(dialog!.textContent).toContain('构成标签')
    expect(dialog!.textContent).toContain('显示方向不符标签')
  })
})

describe('BoardArchiveView — 匹配流程说明', () => {
  it('「匹配流程」弹窗给出六步通用说明与真实阈值', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.find('[data-testid="archive-flow-open"]').trigger('click')
    await flushPromises()
    const dialog = document.body.querySelector('.app-dialog')
    expect(dialog).not.toBeNull()
    const text = dialog!.textContent ?? ''
    expect(text).toContain('⓪ 组合命中')
    expect(text).toContain('① 精确匹配')
    expect(text).toContain('② 算余弦相似度')
    expect(text).toContain('③ 命中率规则')
    expect(text).toContain('④ 最高相似度规则')
    expect(text).toContain('⑤ 综合加权')
    expect(text).toContain('交集 ≥ 2')
    expect(text).toContain('方向校验')
    expect(text).toContain('0.55')
  })
})

describe('BoardArchiveView — 空态与错误', () => {
  it('空态说明当前日期没有文章', async () => {
    apiMock.getBoardArticles.mockResolvedValue({ success: true, data: [], pagination: { pages: 1 } })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('[data-testid="archive-empty"]').text()).toContain('2026-10-23 没有匹配到本版块的文章')
    // 0 篇：比例条留空、百分比显示占位，不出 NaN%
    expect(wrapper.find('[data-testid="archive-stat-in-pct"]').text()).toBe('—')
  })

  it('错误态：显示错误 + 重试', async () => {
    apiMock.getBoardArticles
      .mockResolvedValueOnce({ success: false, error: '归档挂了' })
      .mockResolvedValueOnce({ success: true, data: ARTICLES, pagination: { pages: 1 } })
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('[data-testid="archive-error"]').text()).toContain('归档挂了')
    await wrapper.find('[data-testid="archive-retry"]').trigger('click')
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="archive-days"]').exists()).toBe(true)
  })
})
