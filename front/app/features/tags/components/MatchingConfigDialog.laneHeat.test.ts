/**
 * MatchingConfigDialog 候选泳道权重测试（retire-story-adopt-event-metrics 3.4 / D8）。
 *
 * 契约：`lane_heat_candidate_weight`（默认 0.5、域 [0,1] 有限数）接入既有「匹配规则」配置面；
 * 缺项回退默认；非法拒绝且不覆盖旧值（不发 save）；与 `related_event_similarity_threshold`
 * 各自独立校验、互不影响（改一个不动另一个）。
 */
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import MatchingConfigDialog from './MatchingConfigDialog.vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import AppInput from '~/components/ui/AppInput.vue'
import type { MatchingConfig } from '~/api/semanticBoards'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))
vi.mock('~/components/KaTeXRender.vue', () => ({
  default: { name: 'KaTeXRender', props: { latex: String, display: Boolean }, template: '<span class="katex-stub" />' },
}))

function config(overrides: Partial<MatchingConfig> = {}): MatchingConfig {
  return {
    semantic_board_match_sim_threshold: 0.72,
    semantic_board_match_direct_hit_rate: 0.5,
    semantic_board_match_direct_max_sim: 0.8,
    semantic_board_match_direct_max_sim_min_hits: 2,
    semantic_board_match_direct_max_sim_min_hit_rate: 0.3,
    semantic_board_match_min_effective_sample: 3,
    semantic_board_match_hit_rate_sim_blend: 0.7,
    semantic_board_match_weight_sim: 0.6,
    semantic_board_match_weight_density: 0.4,
    semantic_board_match_weighted_threshold: 0.6,
    semantic_board_match_max_boards: 3,
    semantic_board_match_direct_hit_min_overlap: 2,
    semantic_board_match_direction_sim_threshold: 0.5,
    semantic_board_upgrade_ref_count_threshold: 5,
    semantic_board_upgrade_cluster_distance_threshold: 0.35,
    semantic_board_upgrade_cotag_window_days: 30,
    semantic_board_upgrade_cotag_top_n: 10,
    semantic_board_upgrade_cotag_dedupe_sim_threshold: 0.9,
    semantic_board_upgrade_cotag_hard_limit: 20,
    semantic_board_upgrade_cluster_method: 'average_link',
    related_event_similarity_threshold: 0.86,
    lane_heat_candidate_weight: 0.5,
    ...overrides,
  }
}

async function mountDialog(props: Record<string, unknown> = {}) {
  const wrapper = mount(MatchingConfigDialog, {
    props: { visible: false, config: config(), loading: false, ...props },
    global: {
      components: { AppButton, AppDialog, AppInput },
      stubs: { teleport: true },
    },
  })
  await wrapper.setProps({ visible: true })
  return wrapper
}

function weightInput(wrapper: Awaited<ReturnType<typeof mountDialog>>) {
  return wrapper.find('[data-testid="mc-lane-heat-weight"] input')
}

function relatedInput(wrapper: Awaited<ReturnType<typeof mountDialog>>) {
  return wrapper.find('[data-testid="mc-related-threshold"] input')
}

function saveButton(wrapper: Awaited<ReturnType<typeof mountDialog>>) {
  return wrapper.findAll('button').find(button => button.text() === '保存')
}

describe('MatchingConfigDialog — 候选泳道权重（S4）', () => {
  it('加载配置后显示已保存值，并说明只进热度不进生命周期', async () => {
    const w = await mountDialog({ config: config({ lane_heat_candidate_weight: 0.35 }) })
    expect((weightInput(w).element as HTMLInputElement).value).toBe('0.35')
    const block = w.find('[data-testid="mc-lane-heat"]')
    expect(block.text()).toContain('默认 0.5')
    expect(block.text()).toContain('不影响话题生命周期')
  })

  it('缺项（旧后端未下发）回退默认 0.5，不显示空值/undefined', async () => {
    const legacy = config()
    delete (legacy as Partial<MatchingConfig>).lane_heat_candidate_weight
    const w = await mountDialog({ config: legacy })
    expect((weightInput(w).element as HTMLInputElement).value).toBe('0.5')
  })

  it('保存权重 → emit save 携带该键；相关事件阈值与其他匹配参数原样不变', async () => {
    const w = await mountDialog()
    await weightInput(w).setValue('0.8')
    await saveButton(w)!.trigger('click')

    const payload = w.emitted('save')![0]![0] as Partial<MatchingConfig>
    expect(payload.lane_heat_candidate_weight).toBe(0.8)
    expect(payload.related_event_similarity_threshold).toBe(0.86)
    expect(payload.semantic_board_match_sim_threshold).toBe(0.72)
  })

  it('边界 0 与 1 可保存（0 = 候选不计入版块关注度）', async () => {
    for (const value of ['0', '1']) {
      const w = await mountDialog()
      await weightInput(w).setValue(value)
      await saveButton(w)!.trigger('click')
      expect((w.emitted("save")![0]![0] as Partial<MatchingConfig>).lane_heat_candidate_weight).toBe(Number(value))
    }
  })

  it.each([
    ['空值', '', '请填写 0 到 1 之间的数值'],
    ['范围外', '1.5', '取值范围 0 ~ 1'],
    ['负数', '-0.1', '取值范围 0 ~ 1'],
  ])('%s 非法输入 → 不发 save 且就地报错（不覆盖旧值）', async (_name, value, message) => {
    const w = await mountDialog()
    await weightInput(w).setValue(value)
    await saveButton(w)!.trigger('click')

    expect(w.emitted('save')).toBeUndefined()
    expect(w.find('[data-testid="mc-lane-heat"]').text()).toContain(message)
    expect(saveButton(w)!.attributes('disabled')).toBeDefined()
  })

  it('与相关事件阈值互不影响：改一个不动另一个，各自校验', async () => {
    const w = await mountDialog()
    // 改权重不影响阈值字段的显示值
    await weightInput(w).setValue('0.9')
    expect((relatedInput(w).element as HTMLInputElement).value).toBe('0.86')
    // 改阈值不影响权重字段的显示值
    await relatedInput(w).setValue('0.7')
    expect((weightInput(w).element as HTMLInputElement).value).toBe('0.9')
    // 阈值非法但权重合法 → 仍整体拒绝（不部分保存）
    await relatedInput(w).setValue('1.2')
    await saveButton(w)!.trigger('click')
    expect(w.emitted('save')).toBeUndefined()
    // 修回阈值后一次保存同时携带两个键
    await relatedInput(w).setValue('0.7')
    await saveButton(w)!.trigger('click')
    const payload = w.emitted('save')![0]![0] as Partial<MatchingConfig>
    expect(payload.related_event_similarity_threshold).toBe(0.7)
    expect(payload.lane_heat_candidate_weight).toBe(0.9)
  })
})
