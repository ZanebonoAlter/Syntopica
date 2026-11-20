/**
 * 右轨测试（矩阵阶段 G5 / test-cases RS 组；console-rails-live-signals 后按新顺序与新块重写）。
 *
 *  - RS-1 区块顺序：体检 → 当日日报 → 话题追踪 → 构成候选 → 来源
 *  - RS-2 体检一行状态条（评语首句不再常驻，进详情）；「最新快照」口径归详情（右轨不重复）
 *  - RS-3 构成候选 top3（0/2/5）与真实清单数
 *  - RS-4 追踪管理纯板块作用域；persistentTopicId 只作泳道定位
 *  - RS-5 来源独立 7/30/90 窗 + 未取数不补 0 + 「独立于时间视窗」标注
 *  - RS-6 无全局治理六项（反向断言）
 *  - RS-7 体检/来源失败局部降级、互不阻塞
 *  - RS-8 「当日日报」块：跟随选中日、点评层不重复日报标题、泳道层线索数与事件上抛
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { BoardHealthSnapshotDTO } from '~/api/semanticBoards'
import { normalizeRailHealth } from '~/api/normalizers/consoleRail'
import type { TopicWatch } from '~/api/topicWatches'
import type { BoardSourceBreakdown } from '~/types'
import type { NowBriefModel } from './nowBrief'

const { apiMock } = vi.hoisted(() => ({
  apiMock: { getBoardSourceBreakdown: vi.fn() },
}))

vi.mock('~/api/semanticBoards', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/semanticBoards')>()
  return { ...original, useSemanticBoardsApi: () => apiMock }
})
vi.mock('@iconify/vue', () => ({ Icon: { name: 'Icon', template: '<span class="icon-stub" />' } }))

import BoardContextRail from './BoardContextRail.vue'

const mounted: Array<{ unmount: () => void }> = []

function snapshot(candidates: number, overrides: Partial<BoardHealthSnapshotDTO> = {}): BoardHealthSnapshotDTO {
  return {
    id: 1,
    semantic_board_id: 1974,
    checked_at: '2026-10-23T23:00:00+08:00',
    metrics: {
      board: { matched_tags: 128, reason_dist: {}, ghost_matches: 9, vector_missing: 2 },
      labels: [],
    },
    uncaptured_candidates: Array.from({ length: candidates }, (_, index) => ({
      aux_id: index + 1,
      label: `候选标签 ${index + 1}`,
      member_tags: 20 - index,
      member_articles: 30 - index,
    })),
    llm_score: 85,
    llm_comment: '核心标签贴合度良好，可考虑补两条高频辅助标签。第二句不该出现在首句预览里。',
    created_at: '',
    ...overrides,
  }
}

function health(candidates: number, overrides: Partial<BoardHealthSnapshotDTO> = {}) {
  return normalizeRailHealth(snapshot(candidates, overrides))
}

function watches(): TopicWatch[] {
  return [
    {
      id: '7',
      semanticBoardId: '1974',
      label: '美伊谈判',
      type: 'sentence_topic',
      persistentTopicId: '1204',
      status: 'active',
      createdAt: '',
      updatedAt: '',
    },
    {
      id: '8',
      semanticBoardId: '1974',
      label: '胡塞航运',
      type: 'keyword',
      status: 'paused',
      createdAt: '',
      updatedAt: '',
    },
  ]
}

function breakdown(total = 65): BoardSourceBreakdown {
  return {
    total_articles: total,
    source_count: 4,
    sources: [
      { feed_id: 11, title: '半岛电视台', articles: 30, share: 30 / total, feed_articles: 60, feed_hit_rate: 0.4 },
      { feed_id: 12, title: '伊朗媒体', articles: 20, share: 20 / total, feed_articles: 40, feed_hit_rate: 0.3 },
      { feed_id: 13, title: '沙特通讯社', articles: 10, share: 10 / total, feed_articles: 50, feed_hit_rate: 0.2 },
      { feed_id: 14, title: '美国新闻', articles: 5, share: 5 / total, feed_articles: 25, feed_hit_rate: 0.1 },
    ],
  }
}

/** 「当日日报」块视图模型（真实读取链组装的产物；本测试只管渲染与事件）。 */
function nowModel(overrides: Partial<NowBriefModel> = {}): NowBriefModel {
  return {
    date: '2026-11-18',
    status: 'ready',
    error: null,
    hasReport: true,
    reviews: [
      { title: '伊朗袭击巴林美军基地', reason: '多家媒体报道了袭击细节。', articleCount: 3, clusterId: 42 },
      { title: '伊朗被曝研究新型导弹', reason: '媒体称其推进相关研究。', articleCount: null, clusterId: null },
    ],
    lanes: [
      {
        topicId: '1204',
        label: '沙特与也门胡塞武装冲突',
        color: 'var(--color-secondary)',
        clueCount: 4,
        sentence: '胡塞武装指控沙特袭击致平民死亡并威胁报复',
      },
      {
        topicId: '1203',
        label: '美伊博弈升级',
        color: 'var(--color-success)',
        clueCount: null,
        sentence: '美军与伊朗革命卫队在霍尔木兹海峡一带互有攻防',
      },
    ],
    ...overrides,
  }
}

function mountRail(overrides: Record<string, unknown> = {}) {
  const wrapper = mount(BoardContextRail, {
    props: {
      boardId: 1974,
      healthSnapshot: health(3),
      healthLoading: false,
      healthError: null,
      healthScore: 85,
      healthTier: 'good',
      healthCheckedAtText: '10/23 23:00',
      healthTrendPolyline: '0,10 110,8 220,2',
      healthTrendSummary: '7 轮体检 · 首轮 65 → 最新 85（+20）',
      watches: watches(),
      watchesLoading: false,
      watchesError: null,
      laneIds: ['1204', '1205'],
      topicCandidateCount: 4,
      now: nowModel(),
      ...overrides,
    },
  })
  mounted.push(wrapper)
  return wrapper
}

beforeEach(() => {
  apiMock.getBoardSourceBreakdown.mockReset()
  apiMock.getBoardSourceBreakdown.mockImplementation((_boardId: number, window: number) => Promise.resolve({
    success: true,
    data: breakdown(window === 30 ? 136 : 65),
  }))
})

afterEach(() => {
  for (const wrapper of mounted.splice(0)) wrapper.unmount()
})

describe('BoardContextRail — RS-1 区块顺序', () => {
  it('DOM 顺序：体检 → 当日日报 → 话题追踪 → 构成候选 → 来源', async () => {
    const wrapper = mountRail()
    await flushPromises()
    const html = wrapper.html()
    const order = ['rail-block-health', 'rail-block-now', 'rail-block-watch', 'rail-block-candidates', 'rail-block-sources']
      .map(id => html.indexOf(`data-testid="${id}"`))
    expect(order.every(index => index >= 0)).toBe(true)
    expect([...order].sort((a, b) => a - b)).toEqual(order)
  })
})

describe('BoardContextRail — RS-2 体检结论常驻', () => {
  it('显示真实评分/时间；评语首句不再常驻（进详情），最新快照口径归详情（右轨不重复）', async () => {
    const wrapper = mountRail()
    await flushPromises()
    expect(wrapper.find('[data-testid="rail-health-score"]').text()).toBe('85')
    const meta = wrapper.find('[data-testid="rail-health-checked-at"]')
    expect(meta.text()).toContain('10/23 23:00')
    expect(meta.text()).not.toContain('最新快照')
    expect(wrapper.find('[data-testid="rail-health"]').text()).not.toContain('非所选历史评分')
    // brief 变体瘦身：评语首句不再常驻（右轨只留一行状态条）。
    expect(wrapper.find('[data-testid="rail-health-excerpt"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="rail-health"]').text()).not.toContain('核心标签贴合度良好')
    // 评语仍可达（按需进体检详情）——入口就是评分徽章本身（不另占一个「评语↗」）。
    const badge = wrapper.find('[data-testid="rail-health-score"]')
    expect(badge.element.tagName).toBe('BUTTON')
    await badge.trigger('click')
    expect(wrapper.emitted('openHealth')).toBeTruthy()
    expect(wrapper.find('[data-testid="rail-health-detail"]').exists()).toBe(false)
  })

  it('无快照=等待首轮体检（不伪 0）；刷新失败保留旧快照标 stale', async () => {
    const empty = mountRail({ healthSnapshot: null, healthScore: null, healthTier: null, healthCheckedAtText: '' })
    await flushPromises()
    expect(empty.find('[data-testid="rail-health-pending"]').text()).toContain('等待首轮体检')
    expect(empty.find('[data-testid="rail-health-score"]').exists()).toBe(false)

    const stale = mountRail({ healthError: '刷新失败', healthStale: true })
    await flushPromises()
    expect(stale.find('[data-testid="rail-health-stale"]').text()).toContain('上次成功快照')
    expect(stale.find('[data-testid="rail-health-score"]').text()).toBe('85')
  })

  it('首错：局部错误 + 重试，入口仍在（不阻塞其它区块）', async () => {
    const wrapper = mountRail({ healthSnapshot: null, healthScore: null, healthError: '体检接口挂了' })
    await flushPromises()
    expect(wrapper.find('[data-testid="rail-health-error"]').text()).toContain('体检接口挂了')
    await wrapper.find('[data-testid="rail-health-retry"]').trigger('click')
    expect(wrapper.emitted('retryHealth')).toHaveLength(1)
  })
})

describe('BoardContextRail — RS-3 构成候选 top3', () => {
  it('5 个候选只列 3 个并显示实际总数', async () => {
    const wrapper = mountRail({ healthSnapshot: health(5) })
    await flushPromises()
    expect(wrapper.find('[data-testid="rail-candidates-count"]').text()).toBe('5')
    expect(wrapper.findAll('[data-testid^="rail-candidate-"]').length).toBe(3)
    expect(wrapper.find('[data-testid="rail-candidate-1"]').text()).toContain('20 tags · 30 篇')
  })

  it('0 个候选=空文案；无快照=等待（不造候选）', async () => {
    const zero = mountRail({ healthSnapshot: health(0) })
    await flushPromises()
    expect(zero.find('[data-testid="rail-candidates-empty"]').exists()).toBe(true)

    const pending = mountRail({ healthSnapshot: null })
    await flushPromises()
    expect(pending.find('[data-testid="rail-candidates-pending"]').exists()).toBe(true)
    expect(pending.findAll('[data-testid^="rail-candidate-"]').length).toBe(0)
  })

  it('构成候选/话题追踪都不再写常驻注脚；入口只留短标识', async () => {
    const wrapper = mountRail()
    await flushPromises()
    // 右轨零 `.bcr-note`：收编确认归实际操作处，激活归详情操作
    expect(wrapper.findAll('.bcr-note')).toHaveLength(0)
    const candidates = wrapper.find('[data-testid="rail-block-candidates"]')
    expect(candidates.find('[data-testid="rail-candidates-detail"]').text()).toBe('查看候选')
    expect(candidates.text()).not.toContain('高频共现')
    expect(candidates.text()).not.toContain('历史归属不自动回填')
    const watch = wrapper.find('[data-testid="rail-block-watch"]')
    expect(watch.find('[data-testid="rail-topic-candidates"]').text()).toBe('话题候选 · 4')
    expect(watch.text()).not.toContain('人工激活')
    expect(watch.text()).not.toContain('仅用于定位泳道')
  })

  it('查看候选 → 打开单级工作面（不在此就地收编）', async () => {
    const wrapper = mountRail()
    await flushPromises()
    await wrapper.find('[data-testid="rail-candidates-detail"]').trigger('click')
    expect(wrapper.emitted('openCandidates')).toHaveLength(1)
  })
})

describe('BoardContextRail — RS-4 话题追踪（纯板块作用域）', () => {
  it('真实名称 + active/paused；管理入口只发 openWatchManage（不带泳道参数）', async () => {
    const wrapper = mountRail()
    await flushPromises()
    expect(wrapper.find('[data-testid="rail-watches"]').text()).toContain('美伊谈判')
    expect(wrapper.find('[data-testid="rail-watches"]').text()).toContain('关注中')
    expect(wrapper.find('[data-testid="rail-watches"]').text()).toContain('已暂停')

    await wrapper.find('[data-testid="rail-watch-manage"]').trigger('click')
    expect(wrapper.emitted('openWatchManage')).toEqual([[]])
  })

  it('persistentTopicId 命中名单才给定位链接；未关联/不在名单只显示名称', async () => {
    const wrapper = mountRail()
    await flushPromises()
    const locate = wrapper.find('[data-testid="rail-watch-locate-7"]')
    expect(locate.exists()).toBe(true)
    await locate.trigger('click')
    expect(wrapper.emitted('locateLane')).toEqual([['1204']])

    // watch 8 无 persistentTopicId；watch 9 的 ID 不在当前名单 → 不给链接
    const noLink = mountRail({
      watches: [
        { ...watches()[1]! },
        { ...watches()[0]!, id: '9', label: '已归档话题', persistentTopicId: '9999' },
      ],
    })
    await flushPromises()
    expect(noLink.find('[data-testid="rail-watch-locate-8"]').exists()).toBe(false)
    expect(noLink.find('[data-testid="rail-watch-locate-9"]').exists()).toBe(false)
  })

  it('watch 加载失败局部重试；话题候选只报数量（人工激活入口在详情）', async () => {
    const wrapper = mountRail({ watches: [], watchesError: '关注接口挂了' })
    await flushPromises()
    expect(wrapper.find('[data-testid="rail-watch-error"]').text()).toContain('关注接口挂了')
    await wrapper.find('[data-testid="rail-watch-retry"]').trigger('click')
    expect(wrapper.emitted('retryWatches')).toHaveLength(1)

    const candidates = wrapper.find('[data-testid="rail-topic-candidates"]')
    expect(candidates.text()).toContain('话题候选 · 4')
    expect(candidates.text()).not.toContain('人工激活')
    await candidates.trigger('click')
    expect(wrapper.emitted('openTopics')).toHaveLength(1)
  })
})

describe('BoardContextRail — RS-5 来源独立窗', () => {
  it('默认 7 日；切 30 日独立请求并刷新总量；来源只标「独立于时间视窗」（不重复 N 日）', async () => {
    const wrapper = mountRail()
    await flushPromises()
    expect(apiMock.getBoardSourceBreakdown).toHaveBeenLastCalledWith(1974, 7)
    expect(wrapper.find('[data-testid="rail-sources-totals"]').text()).toContain('65')
    expect(wrapper.find('[data-testid="rail-sources-scope"]').text()).toBe('独立于时间视窗')

    await wrapper.find('[data-testid="rail-source-window-30"]').trigger('click')
    await flushPromises()
    expect(apiMock.getBoardSourceBreakdown).toHaveBeenLastCalledWith(1974, 30)
    expect(wrapper.find('[data-testid="rail-sources-totals"]').text()).toContain('136')
  })

  it('贡献条按份额分段；top3 + 其余来源计数；未取数不补 0', async () => {
    const wrapper = mountRail()
    await flushPromises()
    const segments = wrapper.findAll('[data-testid="rail-sources-composition"] .rsb-segment')
    expect(segments).toHaveLength(4)
    expect(segments[0]!.attributes('title')).toContain('半岛电视台')
    expect(wrapper.findAll('[data-testid^="rail-source-row-"]').length).toBe(3)
    expect(wrapper.find('[data-testid="rail-sources"]').text()).toContain('另有 1 个来源')

    apiMock.getBoardSourceBreakdown.mockClear()
    apiMock.getBoardSourceBreakdown.mockResolvedValue({ success: false, error: '来源统计挂了' })
    const failed = mountRail()
    await flushPromises()
    expect(failed.find('[data-testid="rail-sources-error"]').text()).toContain('来源统计挂了')
    const before = apiMock.getBoardSourceBreakdown.mock.calls.length
    await failed.find('[data-testid="rail-sources-retry"]').trigger('click')
    await flushPromises()
    expect(apiMock.getBoardSourceBreakdown.mock.calls.length).toBe(before + 1)
  })

  it('0 篇归入：明确空态，不画分布', async () => {
    apiMock.getBoardSourceBreakdown.mockResolvedValue({
      success: true,
      data: { total_articles: 0, source_count: 0, sources: [] },
    })
    const wrapper = mountRail()
    await flushPromises()
    expect(wrapper.find('[data-testid="rail-sources-empty"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="rail-sources-composition"]').exists()).toBe(false)
  })

  it('来源明细入口 → 单级工作面 sources', async () => {
    const wrapper = mountRail()
    await flushPromises()
    await wrapper.find('[data-testid="rail-sources-detail"]').trigger('click')
    expect(wrapper.emitted('openSources')).toHaveLength(1)
  })
})

describe('BoardContextRail — RS-6 无全局治理入口', () => {
  it('反向断言：不含新建版块/扩充/回填/规则/合并/标签池按钮', async () => {
    const wrapper = mountRail()
    await flushPromises()
    expect(wrapper.find('[data-testid="gov-advanced"]').exists()).toBe(false)
    const text = wrapper.text()
    for (const label of ['新建版块', '扩充版块', '匹配回填', '匹配规则', '标签合并', '标签池']) {
      expect(text).not.toContain(label)
    }
  })
})

describe('BoardContextRail — RS-7 失败互不阻塞', () => {
  it('体检失败不影响来源与追踪区块渲染', async () => {
    const wrapper = mountRail({ healthSnapshot: null, healthError: '体检挂了', healthScore: null })
    await flushPromises()
    expect(wrapper.find('[data-testid="rail-health-error"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="rail-watches"]').text()).toContain('美伊谈判')
    expect(wrapper.find('[data-testid="rail-sources-totals"]').text()).toContain('65')
  })
})

describe('BoardContextRail — RS-8 「当日日报」块', () => {
  it('块标题写明所跟日期（MM-DD）；右轨内无任何日期选择器', async () => {
    const wrapper = mountRail()
    await flushPromises()
    expect(wrapper.find('[data-testid="rail-now-date"]').text()).toContain('11-18')
    const nowBlock = wrapper.find('[data-testid="rail-block-now"]')
    expect(nowBlock.find('input[type="date"]').exists()).toBe(false)
    expect(nowBlock.find('select').exists()).toBe(false)
  })

  it('点评层：标题 + 点评正文 + 「N 篇来源」；缺省篇数不显示（不补 0）', async () => {
    const wrapper = mountRail()
    await flushPromises()
    const reviews = wrapper.findAll('[data-testid^="rail-now-review-item-"]')
    expect(reviews).toHaveLength(2)
    expect(reviews[0]!.text()).toContain('伊朗袭击巴林美军基地')
    expect(reviews[0]!.text()).toContain('多家媒体报道了袭击细节。')
    expect(reviews[0]!.text()).toContain('3 篇来源')
    // 第二条没有 article_count → 不显示「N 篇来源」（不补 0）
    expect(reviews[1]!.text()).not.toContain('篇来源')
  })

  it('泳道层：泳道名 + 当日线索数 + 一句线索；线索数取不到显示「—」', async () => {
    const wrapper = mountRail()
    await flushPromises()
    const lanes = wrapper.findAll('[data-testid^="rail-now-lane-item-"]')
    expect(lanes).toHaveLength(2)
    expect(lanes[0]!.text()).toContain('沙特与也门胡塞武装冲突')
    expect(lanes[0]!.text()).toContain('4 条')
    expect(lanes[0]!.text()).toContain('胡塞武装指控沙特袭击致平民死亡并威胁报复')
    expect(lanes[1]!.text()).toContain('—')
    expect(lanes[1]!.text()).not.toContain('0 条')
  })

  it('点击泳道行 → 定位泳道；点评标题（有聚簇）→ 定位日报正文', async () => {
    const wrapper = mountRail()
    await flushPromises()
    await wrapper.findAll('[data-testid="rail-now-lane-row"]')[0]!.trigger('click')
    expect(wrapper.emitted('locateLane')).toEqual([['1204']])

    await wrapper.findAll('[data-testid="rail-now-review-title"]')[0]!.trigger('click')
    expect(wrapper.emitted('openReport')).toEqual([[{ date: '2026-11-18', clusterId: 42 }]])
  })

  it('选中日无日报 → 诚实空态（不显示邻近日期内容）', async () => {
    const wrapper = mountRail({ now: nowModel({ status: 'no-report', hasReport: false, reviews: [], lanes: [] }) })
    await flushPromises()
    expect(wrapper.find('[data-testid="rail-now-empty"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid^="rail-now-review-item-"]')).toHaveLength(0)
  })

  it('详情读取失败 → 局部降级（提示 + 重试），其余区块照常', async () => {
    const wrapper = mountRail({ now: nowModel({ status: 'error', error: '日报详情加载失败', hasReport: false }) })
    await flushPromises()
    expect(wrapper.find('[data-testid="rail-now-error"]').text()).toContain('日报详情加载失败')
    await wrapper.find('[data-testid="rail-now-retry"]').trigger('click')
    expect(wrapper.emitted('retryNow')).toEqual([['2026-11-18']])
    expect(wrapper.find('[data-testid="rail-watches"]').text()).toContain('美伊谈判')
  })
})
