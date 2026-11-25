import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { BoardArticleTag } from '~/api/semanticBoards'
import BoardArchiveView from './BoardArchiveView.vue'
import { RULE_COPY } from './archiveMatchFlow'
import type { ArchiveArticle } from '../../composables/useBoardArchive'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

/** 固定「日历今天」，避免用例随真实日期漂移（默认日期 = 今天是本 change 的契约）。 */
const { clock } = vi.hoisted(() => ({ clock: { today: '2026-11-18' } }))

vi.mock('~/features/tags/composables/useBoardConsole', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/features/tags/composables/useBoardConsole')>()
  return { ...original, todayInShanghai: () => clock.today }
})

const { apiMock, articlesApi } = vi.hoisted(() => ({
  apiMock: {
    getBoardArticles: vi.fn(),
    getBoardArticleDailyCounts: vi.fn(),
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
    filtered_tags: [tag({ id: 8, label: '霍尔木兹', match_reason: 'max_sim', score: 0.62, direction_mismatch: true })],
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

/** 计数端点载荷（稀疏日计数 + 最早/最新命中日）。 */
const COUNTS = {
  success: true,
  data: {
    earliest_date: '2026-01-01',
    latest_date: '2026-11-18',
    days: [
      { date: '2026-10-20', count: 3 },
      { date: '2026-10-23', count: 2 },
      { date: '2026-11-18', count: 1 },
    ],
  },
}

/**
 * 每个用例用独立版块 id：按日计数有模块级 60s 缓存（同 id 会命中缓存不再请求），
 * 独立 id 才能断言「计数只请求一次」。
 */
let boardSeq = 0

function mountView(props: Record<string, unknown> = {}) {
  boardSeq += 1
  const boardId = 1974 + boardSeq
  const wrapper = mount(BoardArchiveView, {
    props: { boardId, ...props },
    attachTo: document.body,
  })
  return { wrapper, boardId }
}

/** 日期表单提交一段起止（复用 ObservationRangeControls 内的 RangeDateForm）。 */
async function submitDateRange(wrapper: ReturnType<typeof mountView>['wrapper'], start: string, end: string) {
  await wrapper.find('#orc-start').setValue(start)
  await wrapper.find('#orc-end').setValue(end)
  await wrapper.find('form').trigger('submit')
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
  apiMock.getBoardArticleDailyCounts.mockResolvedValue(COUNTS)
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

describe('BoardArchiveView — 默认日期与时间导航块', () => {
  it('默认查日历今天（旧版最新日报日不再作默认）', async () => {
    const { wrapper, boardId } = mountView()
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenCalledWith(boardId, expect.objectContaining({
      start_date: '2026-11-18',
      end_date: '2026-11-18',
      sort: 'quality',
    }))
    expect(wrapper.find('[data-testid="archive-window"]').text()).toContain('2026-11-18')
  })

  it('initialDate 优先于今天', async () => {
    const { wrapper, boardId } = mountView({ initialDate: '2026-10-24' })
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenCalledWith(boardId, expect.objectContaining({
      start_date: '2026-10-24',
      end_date: '2026-10-24',
    }))
    expect(wrapper.find('[data-testid="archive-window"]').text()).toContain('2026-10-24')
  })

  it('快捷 chip 与工具行日期输入已移除（时间导航块承担日期筛选）', async () => {
    const { wrapper } = mountView()
    await flushPromises()
    expect(wrapper.find('[data-testid="archive-preset-1"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="archive-preset-7"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="archive-start-date"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="archive-end-date"]').exists()).toBe(false)
    // 时间导航块在（日期表单 + 主图 + 导航条）
    expect(wrapper.find('[data-testid="archive-time-nav"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="observation-range-controls"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="archive-history-strip"]').exists()).toBe(true)
  })

  it('曲线口径固定：计数只请求一次，且不带任何筛选参数', async () => {
    const { boardId } = mountView()
    await flushPromises()
    expect(apiMock.getBoardArticleDailyCounts).toHaveBeenCalledTimes(1)
    expect(apiMock.getBoardArticleDailyCounts).toHaveBeenCalledWith(boardId)
  })

  it('切筛选 / 切排序都不重拉计数（曲线口径不随筛选变化）', async () => {
    const { wrapper } = mountView()
    await flushPromises()
    // 筛选 chip 是行内筛选（前端过滤已取回的列表），不发请求
    await wrapper.find('[data-testid="archive-filter-not_in_report"]').trigger('click')
    await flushPromises()
    expect(apiMock.getBoardArticleDailyCounts).toHaveBeenCalledTimes(1)
    expect(apiMock.getBoardArticles).toHaveBeenCalledTimes(1)
    // 切排序是服务端序：列表重拉，计数仍然不重拉
    await wrapper.get('[data-testid="archive-sort"]').setValue('time')
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenCalledTimes(2)
    expect(apiMock.getBoardArticleDailyCounts).toHaveBeenCalledTimes(1)
  })
})

describe('BoardArchiveView — 刷选与导航条', () => {
  it('主图 preview 不重拉列表，commit 才按新区间重拉一次', async () => {
    const { wrapper, boardId } = mountView()
    await flushPromises()
    const chart = wrapper.findComponent({ name: 'ObservationRangeControls' })

    chart.vm.$emit('preview', { start: '2026-11-10', end: '2026-11-15' })
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenCalledTimes(1)

    chart.vm.$emit('commit', { start: '2026-11-10', end: '2026-11-15' })
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenCalledTimes(2)
    expect(apiMock.getBoardArticles).toHaveBeenLastCalledWith(boardId, expect.objectContaining({
      start_date: '2026-11-10',
      end_date: '2026-11-15',
    }))
  })

  it('日期表单提交视窗外区间：视窗跟随，列表按提交区间请求', async () => {
    const { wrapper, boardId } = mountView()
    await flushPromises()
    await submitDateRange(wrapper, '2026-10-18', '2026-10-23')
    expect(apiMock.getBoardArticles).toHaveBeenLastCalledWith(boardId, expect.objectContaining({
      start_date: '2026-10-18',
      end_date: '2026-10-23',
    }))
    expect(wrapper.find('[data-testid="archive-window"]').text()).toContain('2026-10-18 → 2026-10-23')
  })

  it('导航条平移：已提交区间同步平移同偏移，列表按平移后区间请求', async () => {
    const { wrapper, boardId } = mountView()
    await flushPromises()
    const strip = wrapper.findComponent({ name: 'ArchiveHistoryStrip' })
    // 视窗整体后退 30 天（[2026-07-22, 2026-10-20]）
    strip.vm.$emit('viewport-commit', { start: '2026-07-22', end: '2026-10-20' })
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenLastCalledWith(boardId, expect.objectContaining({
      start_date: '2026-10-19',
      end_date: '2026-10-19',
    }))
  })

  it('导航条拖动中不请求列表，commit 才按平移后区间请求一次', async () => {
    const { wrapper, boardId } = mountView()
    await flushPromises()
    const strip = wrapper.findComponent({ name: 'ArchiveHistoryStrip' })
    strip.vm.$emit('viewport-preview', { start: '2026-07-22', end: '2026-10-20' })
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenCalledTimes(1)
    strip.vm.$emit('viewport-commit', { start: '2026-07-22', end: '2026-10-20' })
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenCalledTimes(2)
    // 回归：delta 必须相对拖动基线算（视窗已随 preview 移动，用当前视窗算会得到 0 → 区间不动）
    expect(apiMock.getBoardArticles).toHaveBeenLastCalledWith(boardId, expect.objectContaining({
      start_date: '2026-10-19',
      end_date: '2026-10-19',
    }))
  })

  it('导航条 Esc 取消：回到拖动前区间，不重拉列表', async () => {
    const { wrapper, boardId } = mountView()
    await flushPromises()
    const strip = wrapper.findComponent({ name: 'ArchiveHistoryStrip' })
    strip.vm.$emit('viewport-preview', { start: '2026-07-22', end: '2026-10-20' })
    await flushPromises()
    strip.vm.$emit('viewport-cancel')
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenCalledTimes(1)
    // 取消后视窗回到拖动前：主图 bounds 仍以今天为右端
    const chart = wrapper.findComponent({ name: 'ObservationRangeControls' })
    expect(chart.props('bounds')).toMatchObject({ end: '2026-11-18' })
    expect(apiMock.getBoardArticles).toHaveBeenLastCalledWith(boardId, expect.objectContaining({
      start_date: '2026-11-18',
      end_date: '2026-11-18',
    }))
  })

  it('同值 commit 不重复拉列表', async () => {
    const { wrapper } = mountView()
    await flushPromises()
    const chart = wrapper.findComponent({ name: 'ObservationRangeControls' })
    chart.vm.$emit('commit', { start: '2026-11-18', end: '2026-11-18' })
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenCalledTimes(1)
  })

  it('计数迟到把视窗收窄时，已提交区间被钳回视窗内（不变量 committedRange ⊆ viewport）', async () => {
    let resolveCounts: ((value: unknown) => void) | null = null
    apiMock.getBoardArticleDailyCounts.mockReturnValueOnce(new Promise((resolve) => {
      resolveCounts = resolve
    }))
    const { wrapper } = mountView()
    await flushPromises()

    // 计数未回：兜底视窗 [today-89, today]，先提交一段更早的区间。
    await submitDateRange(wrapper, '2026-10-18', '2026-10-23')

    // 计数到达且最早命中日 = 2026-11-10 → 视窗收窄，旧区间漏到视窗外。
    resolveCounts!({ success: true, data: { earliest_date: '2026-11-10', latest_date: '2026-11-18', days: [] } })
    await flushPromises()

    const chart = wrapper.findComponent({ name: 'ObservationRangeControls' })
    const bounds = chart.props('bounds') as { start: string, end: string }
    const range = chart.props('range') as { start: string, end: string }
    expect(bounds.start).toBe('2026-11-10')
    expect(range.start >= bounds.start).toBe(true)
    expect(range.end <= bounds.end).toBe(true)
  })
})

describe('BoardArchiveView — 日期范围与分组', () => {
  it('按日期分组展示文章', async () => {
    const { wrapper } = mountView()
    await flushPromises()
    expect(wrapper.find('[data-testid="archive-stat-total"]').text()).toContain('5')

    await submitDateRange(wrapper, '2026-10-18', '2026-10-23')
    const heads = wrapper.findAll('.ar-day-date').map(node => node.text())
    expect(heads).toEqual(['2026-10-23', '2026-10-22', '2026-10-21'])
  })

  it('组头显示日期、篇数与弱证据计数', async () => {
    const { wrapper } = mountView()
    await flushPromises()
    await submitDateRange(wrapper, '2026-10-18', '2026-10-23')
    const dayHead = wrapper.findAll('.ar-day-head')[1]
    expect(dayHead?.text()).toContain('2026-10-22')
    expect(dayHead?.text()).toContain('2 篇')
    expect(dayHead?.text()).toContain('弱证据 2')
  })

  it('deck 行给出范围 / 总数 / 已进比例 / 未进 / 弱证据交叉口径（一句话可读）', async () => {
    const { wrapper } = mountView()
    await flushPromises()
    const deck = wrapper.find('[data-testid="archive-window"]')
    expect(deck.text()).toContain('2026-11-18')
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

    await submitDateRange(wrapper, '2026-10-18', '2026-10-23')
    expect(wrapper.find('[data-testid="archive-window"]').text()).toContain('2026-10-18 → 2026-10-23')
  })
})

describe('BoardArchiveView — 行渲染（四列 / 标签带 / 入板 chip）', () => {
  it('行内按规则色显示标签 chip、分数、最强匹配与状态', async () => {
    const { wrapper } = mountView()
    await flushPromises()
    await submitDateRange(wrapper, '2026-10-18', '2026-10-23')
    const row = wrapper.find('[data-testid="archive-row-5"]')
    expect(row.find('[data-testid="archive-tag-5-9"]').text()).toContain('沙特')
    expect(row.find('[data-testid="archive-tag-5-9"]').text()).toContain('0.62')
    // chip 只露标签名 + 分数（规则用颜色编码 + hover 提示表达）
    expect(row.find('[data-testid="archive-tag-5-10"]').text()).toContain('霍尔木兹')
    expect(row.find('[data-testid="archive-tag-5-10"]').text()).toContain('0.50')
    expect(row.find('[data-testid="archive-tag-5-10"]').attributes('title')).toContain('命中率')
    expect(row.find('[data-testid="archive-tag-5-10"]').attributes('style')).toContain('border-color')
    // 降级样式已随标记退役（2026-11-12）；方向不符虚线边保留
    const mismatch = wrapper.find('[data-testid="archive-tag-3-8"]')
    expect(mismatch.classes()).not.toContain('is-deg')
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
    const { wrapper } = mountView()
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
    const { wrapper } = mountView()
    await flushPromises()
    await submitDateRange(wrapper, '2026-10-18', '2026-10-23')
    const more = wrapper.find('[data-testid="archive-tag-more-5"]')
    expect(more.text()).toContain('+1')
    expect(wrapper.find('[data-testid="archive-tag-5-12"]').exists()).toBe(false)
    await more.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="archive-tag-5-12"]').exists()).toBe(true)
  })

  it('有命中标签的行在标签带末尾挂「入板」chip（步号 + 规则短名）', async () => {
    const { wrapper } = mountView()
    await flushPromises()
    await submitDateRange(wrapper, '2026-10-18', '2026-10-23')
    const flowChip = wrapper.find('[data-testid="archive-flow-chip-5"]')
    expect(flowChip.text()).toContain('入板')
    expect(flowChip.text()).toContain('①')
    expect(flowChip.text()).toContain('精确')
    // 无标签文章不挂入板 chip
    expect(wrapper.find('[data-testid="archive-flow-chip-4"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="archive-row-4"]').text()).toContain('无标签')
  })

  it('无标签口径：无标签灰 chip 与统计带都说明「不会进入日报」', async () => {
    const { wrapper } = mountView()
    await flushPromises()
    await submitDateRange(wrapper, '2026-10-18', '2026-10-23')
    expect(wrapper.find('[data-testid="archive-row-4"] .ar-tag.is-empty').attributes('title')).toContain('不会进入日报')
    expect(wrapper.find('[data-testid="archive-window"]').text()).toContain('无命中标签的文章不会进入日报')
  })

  it('疑似 AI 生成标记追加在标签带末尾：不占入板步号、未判定不渲染', async () => {
    apiMock.getBoardArticles.mockResolvedValue({
      success: true,
      data: [
        article(7, { aiDetectionStatus: 'suspect' }),
        article(8, { aiDetectionStatus: 'unknown' }),
        article(9, { aiDetectionStatus: 'not_ai' }),
      ],
      pagination: { page: 1, per_page: 100, total: 3, pages: 1 },
    })
    const { wrapper } = mountView()
    await flushPromises()
    await submitDateRange(wrapper, '2026-10-18', '2026-10-23')

    const row = wrapper.find('[data-testid="archive-row-7"]')
    const chip = row.find('[data-testid="archive-suspect-chip-7"]')
    expect(chip.exists()).toBe(true)
    expect(chip.text()).toBe('疑似 AI 生成')
    // 标签带末尾：入板 chip 仍在前（步号语义不被占用），疑似标记是行内最后一个 chip
    expect(row.find('[data-testid="archive-flow-chip-7"]').text()).toContain('入板')
    const bandItems = row.findAll('.ar-row-tags > *')
    expect(bandItems.at(-1)?.attributes('data-testid')).toBe('archive-suspect-chip-7')
    // 中性色：不注入标签规则色边框（规则色只表达匹配规则）
    expect(chip.classes()).toContain('ar-suspect-chip')
    expect(chip.attributes('style') ?? '').not.toContain('border-color')
    // 未判定 / 已确认非 AI：不渲染标记、不占位
    expect(wrapper.find('[data-testid="archive-row-8"]').find('.ar-suspect-chip').exists()).toBe(false)
    expect(wrapper.find('[data-testid="archive-row-9"]').find('.ar-suspect-chip').exists()).toBe(false)
  })
})

describe('BoardArchiveView — 交互契约', () => {
  it('行主点击 emit open-article（不开自己的弹窗）', async () => {
    const { wrapper } = mountView()
    await flushPromises()
    await submitDateRange(wrapper, '2026-10-18', '2026-10-23')
    await wrapper.find('[data-testid="archive-row-5"]').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('open-article')).toEqual([[5]])
  })

  it('点标签 chip 打开标签卡 popover，且不触发行点击', async () => {
    const { wrapper } = mountView()
    await flushPromises()
    await submitDateRange(wrapper, '2026-10-18', '2026-10-23')
    await wrapper.find('[data-testid="archive-tag-5-9"]').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('open-article')).toBeUndefined()
    const pop = document.body.querySelector('[data-testid="archive-match-popover"]')
    expect(pop).not.toBeNull()
    expect(pop!.textContent).toContain('沙特')
    expect(pop!.textContent).toContain('标签') // 类别 chip / 描述区
  })

  it('点入板 chip 打开入板过程 popover', async () => {
    const { wrapper } = mountView()
    await flushPromises()
    await submitDateRange(wrapper, '2026-10-18', '2026-10-23')
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
    const { wrapper } = mountView()
    await flushPromises()
    await submitDateRange(wrapper, '2026-10-18', '2026-10-23')
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
    const { wrapper } = mountView()
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
    const { wrapper } = mountView()
    await flushPromises()
    const sort = wrapper.get<HTMLSelectElement>('[data-testid="archive-sort"]')
    expect(sort.element.value).toBe('quality')
    await sort.setValue('time')
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenLastCalledWith(expect.anything(), expect.objectContaining({ sort: 'time' }))
    expect(sort.element.value).toBe('time')
  })

  it('收起更多筛选仍可移除来源，清空筛选保留排序', async () => {
    const { wrapper } = mountView()
    await flushPromises()
    const more = wrapper.get('[data-testid="archive-filter-open"]')
    expect(more.attributes('aria-expanded')).toBe('false')
    await more.trigger('click')
    const feed = wrapper.get<HTMLSelectElement>('[data-testid="archive-feed-filter"]')
    const option = feed.element.options[1]!
    await feed.setValue(option.value)
    await wrapper.get('[data-testid="archive-filter-not_in_report"]').trigger('click')
    await wrapper.get('[data-testid="archive-sort"]').setValue('time')
    await more.trigger('click')
    await flushPromises()
    expect(more.attributes('aria-expanded')).toBe('false')
    expect(wrapper.get('button[aria-label^="移除筛选：来源"]').text()).toContain(option.text)
    expect(wrapper.get('[data-testid="archive-list-count"]').text()).toContain('显示 3')
    await wrapper.get('[data-testid="archive-filter-reset"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('button[aria-label^="移除筛选：来源"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="archive-filter-all"]').attributes('aria-pressed')).toBe('true')
    expect(apiMock.getBoardArticles.mock.lastCall?.[1]).not.toHaveProperty('feed_id')
    expect(wrapper.get<HTMLSelectElement>('[data-testid="archive-sort"]').element.value).toBe('time')
  })
})

describe('BoardArchiveView — 匹配流程说明', () => {
  it('「匹配流程」弹窗给出六步通用说明与真实阈值，步骤名/问法与 popover 同源 RULE_COPY', async () => {
    const { wrapper } = mountView()
    await flushPromises()
    await wrapper.find('[data-testid="archive-flow-open"]').trigger('click')
    await flushPromises()
    const dialog = document.body.querySelector('.app-dialog')
    expect(dialog).not.toBeNull()
    const text = dialog!.textContent ?? ''
    // 步骤名 / 编号 / 问法全部来自单一常量源（同源断言取常量值，不硬编码）
    for (const rule of RULE_COPY) {
      expect(text).toContain(`${rule.no} ${rule.name}`)
      expect(text).toContain(rule.question)
    }
    // 弹窗只讲规则与阈值，不给单次判定结果
    expect(text).toContain('交集 ≥ 2')
    expect(text).toContain('方向校验')
    expect(text).toContain('0.55')
    expect(text).not.toContain('未走到')
    expect(text).not.toContain('→ 满足')
  })
})

describe('BoardArchiveView — 空态与错误', () => {
  it('空态说明当前日期没有文章', async () => {
    apiMock.getBoardArticles.mockResolvedValue({ success: true, data: [], pagination: { pages: 1 } })
    const { wrapper } = mountView()
    await flushPromises()
    expect(wrapper.find('[data-testid="archive-empty"]').text()).toContain('2026-11-18 没有匹配到本版块的文章')
    // 0 篇：比例条留空、百分比显示占位，不出 NaN%
    expect(wrapper.find('[data-testid="archive-stat-in-pct"]').text()).toBe('—')
  })

  it('错误态：显示错误 + 重试', async () => {
    apiMock.getBoardArticles
      .mockResolvedValueOnce({ success: false, error: '归档挂了' })
      .mockResolvedValueOnce({ success: true, data: ARTICLES, pagination: { pages: 1 } })
    const { wrapper } = mountView()
    await flushPromises()
    expect(wrapper.find('[data-testid="archive-error"]').text()).toContain('归档挂了')
    await wrapper.find('[data-testid="archive-retry"]').trigger('click')
    await flushPromises()
    expect(apiMock.getBoardArticles).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="archive-days"]').exists()).toBe(true)
  })

  it('计数失败不阻塞列表：内联错误 + 重试入口，列表照常渲染', async () => {
    apiMock.getBoardArticleDailyCounts.mockResolvedValue({ success: false, error: '计数挂了' })
    const { wrapper } = mountView()
    await flushPromises()
    // 列表照常请求并渲染
    expect(wrapper.find('[data-testid="archive-days"]').exists()).toBe(true)
    expect(apiMock.getBoardArticles).toHaveBeenCalledTimes(1)
    // 曲线侧给内联错误 + 重试
    expect(wrapper.find('[data-testid="archive-time-retry"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="observation-range-feedback"]').text()).toContain('时间曲线加载失败')
  })
})
