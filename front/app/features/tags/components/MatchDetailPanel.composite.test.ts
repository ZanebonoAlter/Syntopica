import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import MatchDetailPanel from './MatchDetailPanel.vue'
import type { MatchDetailResponse } from '~/api/semanticBoards'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

const getMatchDetailMock = vi.fn()

vi.mock('~/api/semanticBoards', () => ({
  useSemanticBoardsApi: () => ({ getMatchDetail: getMatchDetailMock }),
}))

function baseConfig(): Record<string, number> {
  return {
    sim_threshold: 0.72,
    hit_rate_sim_blend: 0.7,
    min_effective_sample: 3,
    direct_hit_rate: 0.5,
    direct_max_sim: 0.8,
    direct_max_sim_min_hits: 2,
    direct_max_sim_min_hit_rate: 0.3,
    direct_hit_min_overlap: 2,
    direction_sim_threshold: 0.5,
    direct_hit_score_factor: 0.7,
    weight_sim: 0.6,
    weight_density: 0.4,
    weighted_threshold: 0.6,
  }
}

function compositeHitDetail(): MatchDetailResponse {
  return {
    topic_tag_id: 1,
    topic_tag_label: '美债收益率突破',
    semantic_board_id: 5,
    match_reason: 'composite_hit',
    score: 1,
    downgraded: false,
    direction_mismatch: false,
    direction_sim: 0.1,
    effective_min_hits: 2,
    config: baseConfig() as unknown as MatchDetailResponse['config'],
    direct_hit_auxiliaries: [],
    composite_hits: [
      {
        id: 9,
        label: '美债收益率',
        components: [
          { id: 10, label: '收益率', position: 2 },
          { id: 11, label: '美国国债', position: 1 },
        ],
      },
    ],
    tag_auxiliary_count: 0,
    hits: 0,
    hit_rate: 0,
    max_similarity: 0,
    pairs: [],
  } as MatchDetailResponse
}

function mountPanel(detail: MatchDetailResponse) {
  getMatchDetailMock.mockResolvedValue({ success: true, data: detail })
  return mount(MatchDetailPanel, {
    props: { boardId: 5, tag: { id: 1, label: '美债收益率突破' } as never },
    global: { stubs: { teleport: true } },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('MatchDetailPanel — composite_hit 展示（add-composite-labels 6.3）', () => {
  it('徽标显示「组合命中 1.00」', async () => {
    const w = mountPanel(compositeHitDetail())
    await flushPromises()
    const badge = w.find('.match-detail-badge')
    expect(badge.text()).toContain('组合命中')
  })

  it('流程步骤展示组合链（组合名 = 组件 × 组件，按 position 排序）', async () => {
    const w = mountPanel(compositeHitDetail())
    await flushPromises()
    const flowText = w.find('.flow-steps').text()
    expect(flowText).toContain('组合命中')
    expect(flowText).toContain('美债收益率 = 美国国债 × 收益率')
  })

  it('direct_hit 详情展示降级分数公式（score factor）', async () => {
    const detail = {
      ...compositeHitDetail(),
      match_reason: 'direct_hit',
      score: 0.7,
      composite_hits: undefined,
      direction_mismatch: true,
    } as MatchDetailResponse
    const w = mountPanel(detail)
    await flushPromises()
    const badge = w.find('.match-detail-badge')
    expect(badge.text()).toContain('直接命中')
    expect(w.text()).toContain('0.70')
    // 降级分数在流程步骤中说明
    expect(w.text()).toContain('降级分数')
  })
})
