import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import BoardQuickEditDialog from './BoardQuickEditDialog.vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import type { SemanticBoard } from '~/api/semanticBoards'

/**
 * 版块轻编辑弹窗（lane-manual-correction-loop 6.2）：
 * 尺寸档 `md`（560px）／四字段齐备（名称 / 描述 / 排序 / 受保护）／保存走既有 `updateBoard`。
 */
const { getBoardsMock, updateBoardMock } = vi.hoisted(() => ({
  getBoardsMock: vi.fn(),
  updateBoardMock: vi.fn(),
}))
const notifyMocks = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock('~/api/semanticBoards', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/semanticBoards')>()
  return {
    ...original,
    useSemanticBoardsApi: () => ({ getBoards: getBoardsMock, updateBoard: updateBoardMock }),
  }
})
vi.mock('~/composables/useNotify', () => ({ useNotify: () => notifyMocks }))
vi.mock('@iconify/vue', () => ({ Icon: { name: 'Icon', template: '<span />' } }))

const BOARD: SemanticBoard = {
  id: 1974,
  label: 'AI 前沿',
  slug: 'ai-frontier',
  aliases: [],
  ref_count: 0,
  tag_count: 12,
  description: '模型与工具链',
  display_order: 3,
  source: 'manual',
  status: 'active',
  protected: false,
  enrichment_enabled: true,
  relation_auto_discovery_enabled: false,
  window_days: 14,
  context_layers: ['week'],
  created_at: '',
  updated_at: '',
}

let wrapper: VueWrapper | null = null

function mountDialog(boardId: number | null = 1974) {
  wrapper = mount(BoardQuickEditDialog, {
    props: { modelValue: true, boardId },
    attachTo: document.body,
  })
  return wrapper
}

/** AppInput 的 testid 落在 wrapper div 上，取内部 input。 */
function inputOf(testid: string): HTMLInputElement {
  return document.body.querySelector(`[data-testid="${testid}"] input`) as HTMLInputElement
}

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

beforeEach(() => {
  getBoardsMock.mockReset()
  getBoardsMock.mockResolvedValue({ success: true, data: { items: [BOARD], total: 1 } })
  updateBoardMock.mockReset()
  updateBoardMock.mockResolvedValue({ success: true, data: { id: 1974 } })
  notifyMocks.success.mockReset()
  notifyMocks.error.mockReset()
})

describe('BoardQuickEditDialog', () => {
  it('尺寸档为 md（560px），不走自由宽度', () => {
    mountDialog()
    expect(wrapper!.findComponent(AppDialog).props('size')).toBe('md')
    const dialog = document.body.querySelector('.app-dialog') as HTMLElement
    expect(dialog.className).toContain('is-sized')
    expect(dialog.getAttribute('style')).toContain('560px')
  })

  it('四字段齐备：名称 / 描述 / 排序 / 受保护', async () => {
    mountDialog()
    await flushPromises()
    const text = document.body.textContent ?? ''
    expect(text).toContain('名称')
    expect(text).toContain('描述')
    expect(text).toContain('排序')
    expect(text).toContain('受保护')
    expect(document.body.querySelector('[data-testid="bqed-label"]')).not.toBeNull()
    expect(document.body.querySelector('[data-testid="bqed-description"]')).not.toBeNull()
    expect(document.body.querySelector('[data-testid="bqed-display_order"]')).not.toBeNull()
    expect(document.body.querySelector('[data-testid="bqed-protected"]')).not.toBeNull()
  })

  it('按版块回填四字段', async () => {
    mountDialog()
    await flushPromises()
    expect(inputOf('bqed-label').value).toBe('AI 前沿')
    expect((document.body.querySelector('[data-testid="bqed-description"]') as HTMLTextAreaElement).value).toBe('模型与工具链')
    expect(inputOf('bqed-display_order').value).toBe('3')
  })

  it('保存：载荷含名称/描述/排序/受保护，成功后提示并上报 saved', async () => {
    const onSaved = vi.fn()
    wrapper = mount(BoardQuickEditDialog, {
      props: { modelValue: true, boardId: 1974, onSaved },
      attachTo: document.body,
    })
    await flushPromises()

    const label = inputOf('bqed-label')
    label.value = 'AI 前沿 2'
    label.dispatchEvent(new Event('input'))
    await flushPromises()

    ;(document.body.querySelector('[data-testid="bqed-save"]') as HTMLButtonElement).click()
    await flushPromises()

    expect(updateBoardMock).toHaveBeenCalledWith(1974, {
      label: 'AI 前沿 2',
      description: '模型与工具链',
      display_order: 3,
      protected: false,
    })
    expect(notifyMocks.success).toHaveBeenCalledWith('版块已更新')
    expect(onSaved).toHaveBeenCalledTimes(1)
  })

  it('名称清空 → 本地拦截，不发请求', async () => {
    mountDialog()
    await flushPromises()
    const label = inputOf('bqed-label')
    label.value = '   '
    label.dispatchEvent(new Event('input'))
    await flushPromises()
    ;(document.body.querySelector('[data-testid="bqed-save"]') as HTMLButtonElement).click()
    await flushPromises()

    expect(updateBoardMock).not.toHaveBeenCalled()
    expect(document.body.querySelector('[data-testid="bqed-error"]')!.textContent).toContain('版块名称不能为空')
  })

  it('受保护开关可切换并进载荷', async () => {
    mountDialog()
    await flushPromises()
    ;(document.body.querySelector('[data-testid="bqed-protected"] .app-toggle__track') as HTMLElement).click()
    await flushPromises()
    ;(document.body.querySelector('[data-testid="bqed-save"]') as HTMLButtonElement).click()
    await flushPromises()

    expect(updateBoardMock).toHaveBeenCalledWith(1974, expect.objectContaining({ protected: true }))
  })

  it('版块不存在 → 行内错误，不发请求', async () => {
    getBoardsMock.mockResolvedValue({ success: true, data: { items: [], total: 0 } })
    mountDialog()
    await flushPromises()
    expect(document.body.querySelector('[data-testid="bqed-error"]')!.textContent).toContain('版块不存在')
    expect(updateBoardMock).not.toHaveBeenCalled()
  })
})
