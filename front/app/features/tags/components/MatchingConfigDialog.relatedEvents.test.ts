import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import MatchingConfigDialog from './MatchingConfigDialog.vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import AppInput from '~/components/ui/AppInput.vue'
import type { MatchingConfig } from '~/api/semanticBoards'

// 用例来源：openspec/changes/enable-story-cross-day-continuation/test-cases.md
// 故事 S2（乾链路 1/2 + 变体：边界 0/1、空值、NaN、范围外、保存失败保留输入）。
// 契约：matching-config 新增 related_event_similarity_threshold（默认 0.86，[0,1]），
// 非法值不发 PUT；保存失败保留输入并就地提示；弹窗迁到标准 size="lg"。

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
    ...overrides,
  }
}

/**
 * 弹窗按真实用法打开：先隐藏挂载、再置为可见——表单在 visible/config 的 watch
 * 里从接口配置回填（挂载时就是可见的会绕过该路径，测不到真实回填）。
 */
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

function thresholdInput(wrapper: Awaited<ReturnType<typeof mountDialog>>) {
  return wrapper.find('[data-testid="mc-related-threshold"] input')
}

function saveButton(wrapper: Awaited<ReturnType<typeof mountDialog>>) {
  return wrapper.findAll('button').find(button => button.text() === '保存')
}

describe('MatchingConfigDialog — 相关事件阈值（S2）', () => {
  it('加载配置后显示已保存值（不是固定默认值）', async () => {
    const w = await mountDialog({ config: config({ related_event_similarity_threshold: 0.9 }) })
    expect((thresholdInput(w).element as HTMLInputElement).value).toBe('0.9')
    expect(w.find('[data-testid="mc-related-events"]').text()).toContain('不合并事件')
  })

  it('保存 0.86 → emit save 携带阈值，其余匹配参数原样不变', async () => {
    const w = await mountDialog()
    expect((thresholdInput(w).element as HTMLInputElement).value).toBe('0.86')

    await saveButton(w)!.trigger('click')

    const emitted = w.emitted('save')
    expect(emitted).toHaveLength(1)
    const payload = emitted![0]![0] as Partial<MatchingConfig>
    expect(payload.related_event_similarity_threshold).toBe(0.86)
    expect(payload.semantic_board_match_sim_threshold).toBe(0.72)
    expect(payload.semantic_board_match_max_boards).toBe(3)
  })

  it('调高阈值后保存 → 新值随 save 上送（不重算向量，仅参数）', async () => {
    const w = await mountDialog()
    await thresholdInput(w).setValue('0.95')
    await saveButton(w)!.trigger('click')
    expect((w.emitted('save')![0]![0] as Partial<MatchingConfig>).related_event_similarity_threshold).toBe(0.95)
  })

  it('边界 0 与 1 可保存', async () => {
    for (const value of ['0', '1']) {
      const w = await mountDialog()
      await thresholdInput(w).setValue(value)
      await saveButton(w)!.trigger('click')
      expect((w.emitted('save')![0]![0] as Partial<MatchingConfig>).related_event_similarity_threshold).toBe(Number(value))
    }
  })

  it.each([
    ['空值', '', '请填写 0 到 1 之间的数值'],
    ['范围外', '1.5', '取值范围 0 ~ 1'],
    ['负数', '-0.1', '取值范围 0 ~ 1'],
  ])('%s 非法输入 → 不发 save 且就地报错', async (_name, value, message) => {
    const w = await mountDialog()
    await thresholdInput(w).setValue(value)
    await saveButton(w)!.trigger('click')

    expect(w.emitted('save')).toBeUndefined()
    expect(w.text()).toContain(message)
    expect(saveButton(w)!.attributes('disabled')).toBeDefined()
  })

  it('保存失败 → 显示错误并保留已输入值（不谎称已保存）', async () => {
    const w = await mountDialog({ saveError: '保存失败，请重试' })
    await thresholdInput(w).setValue('0.9')
    await w.setProps({ saveError: '保存失败，请重试' })

    expect(w.find('[data-testid="mc-save-error"]').text()).toContain('保存失败，请重试')
    expect((thresholdInput(w).element as HTMLInputElement).value).toBe('0.9')
    expect(w.emitted('save')).toBeUndefined()
  })

  it('保存中禁用重复提交', async () => {
    const w = await mountDialog({ saving: true })
    expect(saveButton(w)!.attributes('disabled')).toBeDefined()
  })

  it('弹窗使用标准 size="lg" 档位（不再自由 width）', async () => {
    const w = await mountDialog()
    const dialog = w.find('.app-dialog')
    expect(dialog.exists()).toBe(true)
    expect(dialog.classes()).toContain('is-sized')
    expect((dialog.element as HTMLElement).getAttribute('style') || '').toContain('--dialog-w: 760px')
  })
})
