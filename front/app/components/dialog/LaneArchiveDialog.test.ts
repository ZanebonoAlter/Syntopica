import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import LaneArchiveDialog from './LaneArchiveDialog.vue'
import AppConfirmDialog from '~/components/ui/AppConfirmDialog.vue'
import { useConfirm } from '~/composables/useConfirm'

/**
 * 泳道归档弹窗（lane-manual-correction-loop 6.1）：
 * 尺寸档 `sm`（420px）／提示含「不再吸附新内容」／`useConfirm()` 二次确认后才落库。
 *
 * 二次确认走全局 `useConfirm` 管道（渲染端是 `AppConfirmDialog`），测试里同挂一份渲染端，
 * 点击其确认按钮即等价用户确认。
 */
const { updateTopicMock } = vi.hoisted(() => ({ updateTopicMock: vi.fn() }))
const notifyMocks = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock('~/api/dailyReports', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/dailyReports')>()
  return { ...original, useDailyReportsApi: () => ({ updateTopic: updateTopicMock }) }
})
vi.mock('~/composables/useNotify', () => ({ useNotify: () => notifyMocks }))

let wrapper: VueWrapper | null = null
let confirmWrapper: VueWrapper | null = null

function mountDialog() {
  wrapper = mount(LaneArchiveDialog, {
    props: { modelValue: true, topicId: 42, laneLabel: 'Claude Code 工具链' },
    attachTo: document.body,
  })
  confirmWrapper = mount(AppConfirmDialog, { attachTo: document.body })
  return wrapper
}

/** 全局确认弹窗（AppConfirmDialog）的确认按钮——按文案定位，避开宿主弹窗的同名按钮。 */
function confirmButton(): HTMLButtonElement {
  const dialog = [...document.body.querySelectorAll('.app-dialog')]
    .find(el => el.querySelector('.confirm-dialog__message'))
  const btn = dialog?.querySelector('.app-dialog__footer button:last-child') as HTMLButtonElement | undefined
  if (!btn) throw new Error('未找到确认按钮')
  return btn
}

afterEach(() => {
  const { state, settle } = useConfirm()
  if (state.value?.open) settle(state.value.id, false)
  wrapper?.unmount()
  wrapper = null
  confirmWrapper?.unmount()
  confirmWrapper = null
  document.body.innerHTML = ''
})

beforeEach(() => {
  updateTopicMock.mockReset()
  updateTopicMock.mockResolvedValue({ success: true, data: { id: 42 } })
  notifyMocks.success.mockReset()
  notifyMocks.error.mockReset()
})

describe('LaneArchiveDialog', () => {
  it('尺寸档为 sm（420px），不走自由宽度', () => {
    mountDialog()
    const dialog = document.body.querySelector('.app-dialog') as HTMLElement
    expect(dialog.className).toContain('is-sized')
    expect(dialog.getAttribute('style')).toContain('420px')
  })

  it('提示含「不再吸附新内容」，且不含改名入口', () => {
    mountDialog()
    const text = document.body.textContent ?? ''
    expect(text).toContain('不再吸附新内容')
    expect(text).not.toContain('重命名')
  })

  it('二次确认后落 status=archived，并上报 archived', async () => {
    const onArchived = vi.fn()
    wrapper = mount(LaneArchiveDialog, {
      props: { modelValue: true, topicId: 42, laneLabel: 'Claude Code 工具链', onArchived },
      attachTo: document.body,
    })
    confirmWrapper = mount(AppConfirmDialog, { attachTo: document.body })

    ;(document.body.querySelector('[data-testid="lane-archive-confirm"]') as HTMLButtonElement).click()
    await flushPromises()

    // 确认前不发请求
    expect(updateTopicMock).not.toHaveBeenCalled()

    confirmButton().click()
    await flushPromises()

    expect(updateTopicMock).toHaveBeenCalledWith(42, { status: 'archived' })
    expect(notifyMocks.success).toHaveBeenCalledWith('已归档，该泳道不再吸附新内容')
    expect(onArchived).toHaveBeenCalledTimes(1)
  })

  it('二次确认取消 → 不发请求', async () => {
    mountDialog()
    ;(document.body.querySelector('[data-testid="lane-archive-confirm"]') as HTMLButtonElement).click()
    await flushPromises()

    const { state, settle } = useConfirm()
    expect(state.value?.open).toBe(true)
    expect(state.value?.message).toContain('不再吸附新内容')
    settle(state.value!.id, false)
    await flushPromises()

    expect(updateTopicMock).not.toHaveBeenCalled()
  })

  it('归档失败 → 行内错误 + 错误提示，不上报 archived', async () => {
    updateTopicMock.mockResolvedValue({ success: false, error: '后端 500' })
    const onArchived = vi.fn()
    wrapper = mount(LaneArchiveDialog, {
      props: { modelValue: true, topicId: 42, laneLabel: 'Claude Code 工具链', onArchived },
      attachTo: document.body,
    })
    confirmWrapper = mount(AppConfirmDialog, { attachTo: document.body })

    ;(document.body.querySelector('[data-testid="lane-archive-confirm"]') as HTMLButtonElement).click()
    await flushPromises()
    confirmButton().click()
    await flushPromises()

    expect(document.body.querySelector('[data-testid="lane-archive-error"]')!.textContent).toContain('后端 500')
    expect(notifyMocks.error).toHaveBeenCalled()
    expect(onArchived).not.toHaveBeenCalled()
  })
})
