/**
 * TopicNewsBackground 挂载测试（redesign-enrichment-research-workbench 4.3）。
 *
 * 验收锚（spec「话题新闻背景独立上下文」四 Scenario）：
 *  - 数据键 topic+granularity+period 独立读取，切 topic/period 晚响应不串台；
 *  - 保存失败保留草稿（PUT 失败 textarea 内容仍在）；
 *  - 补生成/重新汇总必须先弹确认（取消零生成调用；已有内容文案含覆盖提示）；
 *  - readonly 只读预览无编辑/生成按钮；
 *  - owner 不符（404）错误态可重试。
 *
 * API 全 mock（~/api/boardEnrichment）；确认弹窗走真实 useConfirm 管道 +
 * AppConfirmDialog harness（对齐 VendorCard.test.ts 既有模式）。
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import TopicNewsBackground from './TopicNewsBackground.vue'
import AppConfirmDialog from '~/components/ui/AppConfirmDialog.vue'
import { useConfirm } from '~/composables/useConfirm'
import type { ContextRow } from '~/api/boardEnrichment'

const { listContextsMock, updateContextMock, regenerateContextMock } = vi.hoisted(() => ({
  listContextsMock: vi.fn(),
  updateContextMock: vi.fn(),
  regenerateContextMock: vi.fn(),
}))

vi.mock('~/api/boardEnrichment', () => ({
  useBoardEnrichmentApi: () => ({
    listContexts: listContextsMock,
    getContext: vi.fn(),
    updateContext: updateContextMock,
    regenerateContext: regenerateContextMock,
  }),
}))

function ctxRow(partial: Partial<ContextRow>): ContextRow {
  return {
    id: 1,
    persistent_topic_id: 1,
    granularity: 'week',
    period: '2026-W27',
    content: '',
    as_of_date: '2026-07-05',
    source: 'llm_assisted',
    ...partial,
  }
}

/** 每次新建：组件会行内替换 rows（引用赋值），fixture 必须用例间隔离。 */
function makeWeekRows(): ContextRow[] {
  return [
    ctxRow({ id: 11, period: '2026-W27', content: '周背景W27内容' }),
    ctxRow({ id: 10, period: '2026-W26', content: '周背景W26内容' }),
  ]
}

function baseProps() {
  return { topicId: 1, topicLabel: '半导体设备', semanticBoardId: 5 as number | undefined }
}

/** 直挂组件（多数用例）；需要确认弹窗的用例用 mountWithConfirm harness。 */
function mountView(props: Record<string, unknown> = {}) {
  return mount(TopicNewsBackground, { props: { ...baseProps(), ...props } })
}

/** 组件 + 全局确认弹窗 harness（补生成确认走真实 AppConfirmDialog 管道）。 */
function mountWithConfirm(props: Record<string, unknown> = {}) {
  return mount({
    components: { TopicNewsBackground, AppConfirmDialog },
    data: () => ({ p: { ...baseProps(), ...props } }),
    template: `<div><TopicNewsBackground v-bind="p" /><AppConfirmDialog /></div>`,
  })
}

async function clickBodyButton(text: string) {
  const btn = Array.from(document.body.querySelectorAll('button')).find((b) =>
    b.textContent?.includes(text),
  )
  expect(btn, `body 内应存在「${text}」按钮（确认弹窗 teleport 到 body）`).toBeTruthy()
  btn!.click()
  await flushPromises()
}

beforeEach(() => {
  vi.clearAllMocks()
  listContextsMock.mockResolvedValue({ success: true, data: makeWeekRows() })
  updateContextMock.mockResolvedValue({ success: true, data: ctxRow({}) })
  regenerateContextMock.mockResolvedValue({ success: true, data: ctxRow({}) })
})

// ── 数据键与晚响应（Scenario：从话题直接查看背景 / 背景失效与晚响应）───────────

describe('数据键 topic+granularity+period 与 epoch 守卫', () => {
  it('挂载即按 (topicId, gran, semanticBoardId) 读取；切 topic 各自独立读取', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(listContextsMock).toHaveBeenCalledWith(1, 'week', 5)
    expect(wrapper.find('.tnb-content').text()).toContain('周背景W27内容')

    await wrapper.setProps({ topicId: 2 })
    await flushPromises()
    expect(listContextsMock).toHaveBeenLastCalledWith(2, 'week', 5)
    wrapper.unmount()
  })

  it('周期历史翻阅：period select 切换显示对应周期内容（不重复请求）', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('.tnb-period-select').element.getAttribute('value') ?? '').toContain('W27')

    // 默认选中最新（降序首个 = 2026-W27）；切到 2026-W26。
    await wrapper.find('.tnb-period-select').setValue('2026-W26')
    await nextTick()
    expect(wrapper.find('.tnb-content').text()).toContain('周背景W26内容')
    expect(listContextsMock).toHaveBeenCalledTimes(1) // 翻历史不发新请求
    wrapper.unmount()
  })

  it('切周期档（月）重新按 granularity 读取', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.find('.tnb-gran-btn:nth-child(2)').trigger('click')
    await flushPromises()
    expect(listContextsMock).toHaveBeenLastCalledWith(1, 'month', 5)
    wrapper.unmount()
  })

  it('晚响应不串台：旧 topic 的慢响应迟到后丢弃，仍显示新 topic 内容', async () => {
    let resolveSlow!: (v: unknown) => void
    listContextsMock.mockImplementationOnce(
      () => new Promise((resolve) => { resolveSlow = resolve }),
    )
    const wrapper = mountView() // topic 1 → 慢请求在途
    await wrapper.setProps({ topicId: 2 }) // topic 2 → 立即返回
    await flushPromises()
    expect(listContextsMock).toHaveBeenLastCalledWith(2, 'week', 5)

    // 旧 topic 1 的响应迟到：不得覆盖 topic 2 的视图。
    resolveSlow({ success: true, data: [ctxRow({ content: '话题1的旧背景' })] })
    await flushPromises()
    expect(wrapper.find('.tnb-content').text()).not.toContain('话题1的旧背景')
    expect(wrapper.find('.tnb-state--error').exists()).toBe(false)
    wrapper.unmount()
  })
})

// ── 编辑与草稿（Scenario：背景编辑与研究隔离）───────────────────────────────

describe('编辑保存与草稿保留', () => {
  it('保存成功：PUT 带目标话题周期与归属参数，内容更新并退出编辑', async () => {
    updateContextMock.mockResolvedValue({
      success: true,
      data: ctxRow({ id: 11, period: '2026-W27', content: '人工改写内容', source: 'manual' }),
    })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('.tnb-edit-btn').trigger('click')
    const area = wrapper.find('.tnb-textarea')
    expect((area.element as HTMLTextAreaElement).value).toBe('周背景W27内容')
    await area.setValue('人工改写内容')
    await wrapper.find('.tnb-save-btn').trigger('click')
    await flushPromises()

    expect(updateContextMock).toHaveBeenCalledWith(1, 'week', '2026-W27', { content: '人工改写内容' }, 5)
    expect(wrapper.find('.tnb-content').text()).toContain('人工改写内容')
    expect(wrapper.find('.tnb-textarea').exists()).toBe(false)
    wrapper.unmount()
  })

  it('保存失败：错误就地展示，textarea 草稿保留不丢', async () => {
    updateContextMock.mockResolvedValue({ success: false, error: 'owner 校验失败' })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('.tnb-edit-btn').trigger('click')
    await wrapper.find('.tnb-textarea').setValue('未保存的草稿')
    await wrapper.find('.tnb-save-btn').trigger('click')
    await flushPromises()

    expect(wrapper.find('.tnb-error-msg').text()).toContain('owner 校验失败')
    const area = wrapper.find('.tnb-textarea')
    expect(area.exists()).toBe(true) // 仍在编辑态
    expect((area.element as HTMLTextAreaElement).value).toBe('未保存的草稿')
    wrapper.unmount()
  })
})

// ── 补生成必须确认（Scenario：补生成必须明确确认）───────────────────────────

describe('补生成/重新汇总必须先弹确认', () => {
  it('已有内容：点按钮先弹确认（文案含覆盖+预算提示），取消零生成调用；确认后才调 regenerate', async () => {
    regenerateContextMock.mockResolvedValue({
      success: true,
      data: ctxRow({ id: 11, period: '2026-W27', content: '重新汇总后的内容' }),
    })
    const wrapper = mountWithConfirm()
    await flushPromises()

    await wrapper.find('.tnb-regen-btn').trigger('click')
    await flushPromises()
    expect(regenerateContextMock).not.toHaveBeenCalled() // 确认前零生成调用

    const { state } = useConfirm()
    expect(state.value?.open).toBe(true)
    expect(state.value?.message).toContain('覆盖')
    expect(state.value?.message).toContain('预算')

    // 取消 → 仍不生成。
    await clickBodyButton('取消')
    expect(regenerateContextMock).not.toHaveBeenCalled()
    expect(wrapper.find('.tnb-content').text()).toContain('周背景W27内容') // 旧内容保留

    // 再点 + 确认 → regenerate 调用（带目标周期与归属参数）。
    await wrapper.find('.tnb-regen-btn').trigger('click')
    await flushPromises()
    await clickBodyButton('重新汇总')
    expect(regenerateContextMock).toHaveBeenCalledTimes(1)
    expect(regenerateContextMock).toHaveBeenCalledWith(1, 'week', '2026-W27', 5)
    expect(wrapper.find('.tnb-content').text()).toContain('重新汇总后的内容')
    wrapper.unmount()
  })

  it('无内容（empty 态）：确认文案不含覆盖提示；确认后补生成当前周期', async () => {
    listContextsMock.mockResolvedValue({ success: true, data: [] })
    regenerateContextMock.mockResolvedValue({
      success: true,
      data: ctxRow({ id: 12, period: '2026-W28', content: '新生成内容' }),
    })
    const wrapper = mountWithConfirm()
    await flushPromises()
    expect(wrapper.find('.tnb-state--empty').exists()).toBe(true)

    await wrapper.find('.tnb-regen-btn').trigger('click')
    await flushPromises()
    const { state } = useConfirm()
    expect(state.value?.open).toBe(true)
    expect(state.value?.message).not.toContain('覆盖')

    await clickBodyButton('生成')
    expect(regenerateContextMock).toHaveBeenCalledWith(1, 'week', undefined, 5)
    expect(wrapper.find('.tnb-state--empty').exists()).toBe(false)
    expect(wrapper.find('.tnb-content').text()).toContain('新生成内容')
    wrapper.unmount()
  })

  it('生成失败：旧内容与错误保留，不伪装空态', async () => {
    regenerateContextMock.mockResolvedValue({ success: false, error: '模型调用失败' })
    const wrapper = mountWithConfirm()
    await flushPromises()

    await wrapper.find('.tnb-regen-btn').trigger('click')
    await flushPromises()
    await clickBodyButton('重新汇总')
    await flushPromises()

    expect(wrapper.find('.tnb-error-msg').text()).toContain('模型调用失败')
    expect(wrapper.find('.tnb-content').text()).toContain('周背景W27内容')
    wrapper.unmount()
  })
})

// ── readonly 只读预览（Scenario：从话题直接查看背景——研究入口形态）────────────

describe('readonly 只读预览', () => {
  it('无编辑/生成/保存入口，仅内容 + 周期选择', async () => {
    const wrapper = mountView({ readonly: true })
    await flushPromises()

    expect(wrapper.find('.tnb-content').text()).toContain('周背景W27内容')
    expect(wrapper.find('.tnb-edit-btn').exists()).toBe(false)
    expect(wrapper.find('.tnb-regen-btn').exists()).toBe(false)
    expect(wrapper.find('.tnb-textarea').exists()).toBe(false)
    expect(wrapper.find('.tnb-save-btn').exists()).toBe(false)
    // 周期选择仍可用
    expect(wrapper.find('.tnb-period-select').exists()).toBe(true)
    wrapper.unmount()
  })

  it('readonly 空态：只提示暂无背景，无补生成按钮（没有记忆不自动生成）', async () => {
    listContextsMock.mockResolvedValue({ success: true, data: [] })
    const wrapper = mountView({ readonly: true })
    await flushPromises()

    expect(wrapper.find('.tnb-state--empty').exists()).toBe(true)
    expect(wrapper.find('.tnb-regen-btn').exists()).toBe(false)
    expect(regenerateContextMock).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})

// ── 失效与重试（Scenario：背景失效与晚响应）─────────────────────────────────

describe('owner 不符 404 错误态可重试', () => {
  it('GET 失败（404 owner 校验）→ 错误态 + 重试按钮；重试成功恢复内容', async () => {
    listContextsMock
      .mockResolvedValueOnce({ success: false, error: 'topic not found in board', status: 404 })
      .mockResolvedValueOnce({ success: true, data: makeWeekRows() })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('.tnb-state--error').exists()).toBe(true)
    expect(wrapper.find('.tnb-state--error').text()).toContain('topic not found in board')
    expect(wrapper.find('.tnb-retry-btn').exists()).toBe(true)

    await wrapper.find('.tnb-retry-btn').trigger('click')
    await flushPromises()
    expect(listContextsMock).toHaveBeenCalledTimes(2)
    expect(wrapper.find('.tnb-state--error').exists()).toBe(false)
    expect(wrapper.find('.tnb-content').text()).toContain('周背景W27内容')
    wrapper.unmount()
  })
})
