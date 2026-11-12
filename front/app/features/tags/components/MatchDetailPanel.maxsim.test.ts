import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import MatchDetailPanel from './MatchDetailPanel.vue'
import type { MatchDetailResponse } from '~/api/semanticBoards'

/**
 * retire-downgraded-mount：④ max_sim 命中步的文案契约。
 *
 * 降级标记退役后，④ 步骤 SHALL 只给真实三条件数值 + 方向校验结论，
 * SHALL NOT 再出现「⚠ 降级匹配（原阈值 X …）」这类降级语义文案。
 */

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

function maxSimDetail(overrides: Partial<MatchDetailResponse> = {}): MatchDetailResponse {
  return {
    topic_tag_id: 1,
    topic_tag_label: 'SGLang',
    semantic_board_id: 5,
    match_reason: 'max_sim',
    score: 0.85,
    direction_mismatch: false,
    direction_sim: 0.8,
    effective_min_hits: 1,
    config: baseConfig() as unknown as MatchDetailResponse['config'],
    direct_hit_auxiliaries: [],
    tag_auxiliary_count: 1,
    hits: 1,
    hit_rate: 0.333,
    max_similarity: 0.85,
    pairs: [],
    ...overrides,
  } as MatchDetailResponse
}

function mountPanel(detail: MatchDetailResponse) {
  getMatchDetailMock.mockResolvedValue({ success: true, data: detail })
  return mount(MatchDetailPanel, {
    props: { boardId: 5, tag: { id: 1, label: 'SGLang' } as never },
    global: { stubs: { teleport: true } },
  })
}

/** 取 ④ 最高相似度规则这一步的 result 文案。 */
function stepResult(wrapper: ReturnType<typeof mountPanel>, _stepId: string): string {
  const nodes = wrapper.findAll('.flow-node')
  const hit = nodes.find(s => s.text().includes('最高相似度规则'))
  expect(hit, '未找到 ④ 最高相似度规则步骤').toBeTruthy()
  return hit!.find('.flow-node-result').text()
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('MatchDetailPanel — ④ max_sim 命中步（retire-downgraded-mount）', () => {
  it('N=1 放宽门槛命中：给真实三条件数值，且不含降级文案', async () => {
    const w = mountPanel(maxSimDetail())
    await flushPromises()
    const text = stepResult(w, 'max_sim')
    expect(text).toContain('Smax=0.85')
    expect(text).toContain('1≥1命中')
    expect(text).toContain('→ 满足！')
    expect(text).not.toContain('降级')
    expect(text).not.toContain('↓')
  })

  it('方向校验通过：④ 步骤标「方向校验✓」', async () => {
    const w = mountPanel(maxSimDetail({ direction_sim: 0.8 }))
    await flushPromises()
    const text = stepResult(w, 'max_sim')
    expect(text).toContain('方向校验✓')
  })

  it('方向校验未通过：④ 步骤标「⚠方向不符」', async () => {
    const w = mountPanel(maxSimDetail({ direction_sim: 0.2 }))
    await flushPromises()
    const text = stepResult(w, 'max_sim')
    expect(text).toContain('⚠方向不符')
  })

  it('方向数据缺失：④ 步骤标「方向校验跳过（无数据）」', async () => {
    const w = mountPanel(maxSimDetail({ direction_sim: null }))
    await flushPromises()
    const text = stepResult(w, 'max_sim')
    expect(text).toContain('方向校验跳过（无数据）')
  })
})
