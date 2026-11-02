/**
 * 右轨测试（矩阵阶段 G5 / test-cases RS 组；继承 S1-16/17/19 后按批准版面重写）。
 *
 *  - RS-1 区块顺序：体检 → 构成候选 → 话题追踪 → 来源
 *  - RS-2 评语首句两行预览 + 「最新快照 · 非所选历史评分」
 *  - RS-3 构成候选 top3（0/2/5）与真实清单数
 *  - RS-4 追踪管理纯板块作用域；persistentTopicId 只作泳道定位
 *  - RS-5 来源独立 7/30/90 窗 + 未取数不补 0 + 「独立统计」标注
 *  - RS-6 无全局治理六项（反向断言）
 *  - RS-7 体检/来源失败局部降级、互不阻塞
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { BoardHealthSnapshotDTO } from '~/api/semanticBoards'
import { normalizeRailHealth } from '~/api/normalizers/consoleRail'
import type { TopicWatch } from '~/api/topicWatches'
import type { BoardSourceBreakdown } from '~/types'

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
  it('DOM 顺序：体检 → 构成候选 → 话题追踪 → 来源', async () => {
    const wrapper = mountRail()
    await flushPromises()
    const html = wrapper.html()
    const order = ['rail-block-health', 'rail-block-candidates', 'rail-block-watch', 'rail-block-sources']
      .map(id => html.indexOf(`data-testid="${id}"`))
    expect(order.every(index => index >= 0)).toBe(true)
    expect([...order].sort((a, b) => a - b)).toEqual(order)
  })
})

describe('BoardContextRail — RS-2 体检结论常驻', () => {
  it('显示真实评分/时间/评语首句 + 最新快照标注（不随历史视窗）', async () => {
    const wrapper = mountRail()
    await flushPromises()
    expect(wrapper.find('[data-testid="rail-health-score"]').text()).toBe('85')
    expect(wrapper.find('[data-testid="rail-health-checked-at"]').text()).toContain('10/23 23:00')
    expect(wrapper.find('[data-testid="rail-health-checked-at"]').text()).toContain('最新快照 · 非所选历史评分')
    const excerpt = wrapper.find('[data-testid="rail-health-excerpt"]').text()
    expect(excerpt).toContain('核心标签贴合度良好')
    expect(excerpt).not.toContain('第二句')
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

  it('查看候选与依据 → 打开单级工作面（不在此就地收编）', async () => {
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

  it('watch 加载失败局部重试；话题候选单独计数（人工激活，不自动转正）', async () => {
    const wrapper = mountRail({ watches: [], watchesError: '关注接口挂了' })
    await flushPromises()
    expect(wrapper.find('[data-testid="rail-watch-error"]').text()).toContain('关注接口挂了')
    await wrapper.find('[data-testid="rail-watch-retry"]').trigger('click')
    expect(wrapper.emitted('retryWatches')).toHaveLength(1)

    const candidates = wrapper.find('[data-testid="rail-topic-candidates"]')
    expect(candidates.text()).toContain('话题候选 · 4')
    expect(candidates.text()).toContain('人工激活')
    await candidates.trigger('click')
    expect(wrapper.emitted('openTopics')).toHaveLength(1)
  })
})

describe('BoardContextRail — RS-5 来源独立窗', () => {
  it('默认 7 日；切 30 日独立请求并刷新总量；「独立统计」标注在场', async () => {
    const wrapper = mountRail()
    await flushPromises()
    expect(apiMock.getBoardSourceBreakdown).toHaveBeenLastCalledWith(1974, 7)
    expect(wrapper.find('[data-testid="rail-sources-totals"]').text()).toContain('65')
    expect(wrapper.find('[data-testid="rail-sources-scope"]').text()).toContain('独立统计，不随时间视窗')

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
