import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import BoardEditDialog from './BoardEditDialog.vue'

// iconify / AppToggle / AppInput stub（表单交互足够：toggle 点击语义走 stub 内按钮）
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

/**
 * 版块编辑弹窗 — 跨版块关系自动发现开关
 * （add-evidence-backed-cross-board-relations 7.3）：
 *  - 旧配置缺 relation_auto_discovery_enabled → 显示关闭
 *  - 开关 emit update:edit-relation-auto-discovery-enabled
 *  - 数据增强关闭时开关禁用（联动门）
 *  - 说明文案含「只生成待裁决建议、绝不自动确认」纪律
 */

const ToggleStub = {
  name: 'AppToggle',
  props: { modelValue: { type: Boolean, default: false }, disabled: { type: Boolean, default: false } },
  emits: ['update:modelValue'],
  template: `<button type="button" class="toggle-stub" :disabled="disabled" data-test="toggle" @click="$emit('update:modelValue', !modelValue)" />`,
}

function mountDialog(extra: Record<string, unknown> = {}) {
  return mount(BoardEditDialog, {
    props: {
      editingBoard: true,
      editLabel: '板块',
      editDescription: '',
      editEnrichmentEnabled: true,
      editRelationAutoDiscoveryEnabled: false,
      editWindowDays: 14,
      editContextLayers: [],
      editSaving: false,
      editError: null,
      ...extra,
    },
    global: { stubs: { AppToggle: ToggleStub, AppInput: true, teleport: true } },
  })
}

describe('BoardEditDialog — 自动发现开关（7.3）', () => {
  it('旧配置缺字段（false）→ 开关关闭态展示', () => {
    const w = mountDialog({ editRelationAutoDiscoveryEnabled: false })
    const row = w.find('[data-test="relation-auto-toggle-row"]')
    expect(row.exists()).toBe(true)
    // 第二个 toggle（enrichment 是第一个）
    const toggles = w.findAllComponents(ToggleStub)
    expect(toggles.length).toBeGreaterThanOrEqual(2)
    expect(toggles[1]?.props('modelValue') as boolean).toBe(false)
  })

  it('开关交互 → emit update:edit-relation-auto-discovery-enabled', async () => {
    const w = mountDialog({ editRelationAutoDiscoveryEnabled: false })
    const toggles = w.findAllComponents(ToggleStub)
    await (toggles[1]?.vm.$el as HTMLElement).click()
    expect(w.emitted('update:edit-relation-auto-discovery-enabled')).toEqual([[true]])
  })

  it('数据增强关闭 → 自动发现开关禁用且视觉弱化', () => {
    const w = mountDialog({ editEnrichmentEnabled: false, editRelationAutoDiscoveryEnabled: true })
    const row = w.find('[data-test="relation-auto-toggle-row"]')
    expect(row.classes()).toContain('disabled')
    const toggles = w.findAllComponents(ToggleStub)
    expect(toggles[1]?.props('disabled') as boolean).toBe(true)
    // 展示值也被钳制为关（editEnrichmentEnabled=false 联动）
    expect(toggles[1]?.props('modelValue') as boolean).toBe(false)
  })

  it('说明文案含「只生成待裁决建议、绝不自动确认」纪律', () => {
    const w = mountDialog()
    const row = w.find('[data-test="relation-auto-toggle-row"]')
    expect(row.text()).toContain('只生成待裁决建议')
    expect(row.text()).toContain('绝不自动确认')
    expect(row.text()).toContain('默认关闭')
  })
})
