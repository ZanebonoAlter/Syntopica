import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import BoardArchiveDialog from './BoardArchiveDialog.vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import type { SemanticBoard } from '~/api/semanticBoards'

/**
 * 版块归档弹窗（lane-manual-correction-loop 6.5）：
 * 尺寸档 `md`（560px）／**手打版块名**才放行归档／受保护版块禁用归档并说明原因。
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

function board(overrides: Partial<SemanticBoard> = {}): SemanticBoard {
  return {
    id: 1974,
    label: 'AI 前沿',
    slug: 'ai-frontier',
    aliases: [],
    ref_count: 0,
    tag_count: 12,
    description: '',
    display_order: 0,
    source: 'manual',
    status: 'active',
    protected: false,
    enrichment_enabled: false,
    relation_auto_discovery_enabled: false,
    window_days: 14,
    context_layers: ['week'],
    created_at: '',
    updated_at: '',
    ...overrides,
  }
}

let wrapper: VueWrapper | null = null

async function mountDialog(target: SemanticBoard = board()) {
  getBoardsMock.mockResolvedValue({ success: true, data: { items: [target], total: 1 } })
  wrapper = mount(BoardArchiveDialog, {
    props: { modelValue: true, boardId: 1974 },
    attachTo: document.body,
  })
  await flushPromises()
  return wrapper
}

function confirmInput(): HTMLInputElement {
  return document.body.querySelector('[data-testid="bad-confirm-input"] input') as HTMLInputElement
}

function confirmButton(): HTMLButtonElement {
  return document.body.querySelector('[data-testid="bad-confirm"]') as HTMLButtonElement
}

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

beforeEach(() => {
  getBoardsMock.mockReset()
  updateBoardMock.mockReset()
  updateBoardMock.mockResolvedValue({ success: true, data: { id: 1974 } })
  notifyMocks.success.mockReset()
  notifyMocks.error.mockReset()
})

describe('BoardArchiveDialog', () => {
  it('尺寸档为 md（560px）', async () => {
    await mountDialog()
    expect(wrapper!.findComponent(AppDialog).props('size')).toBe('md')
    const dialog = document.body.querySelector('.app-dialog') as HTMLElement
    expect(dialog.getAttribute('style')).toContain('560px')
  })

  it('未逐字输入版块名 → 归档按钮禁用，不发请求', async () => {
    await mountDialog()
    expect(confirmButton().disabled).toBe(true)

    const input = confirmInput()
    input.value = 'AI'
    input.dispatchEvent(new Event('input'))
    await flushPromises()
    expect(confirmButton().disabled).toBe(true)

    confirmButton().click()
    await flushPromises()
    expect(updateBoardMock).not.toHaveBeenCalled()
  })

  it('逐字输入版块名 → 归档落 status=disabled，并上报 archived', async () => {
    const onArchived = vi.fn()
    getBoardsMock.mockResolvedValue({ success: true, data: { items: [board()], total: 1 } })
    wrapper = mount(BoardArchiveDialog, {
      props: { modelValue: true, boardId: 1974, onArchived },
      attachTo: document.body,
    })
    await flushPromises()

    const input = confirmInput()
    input.value = 'AI 前沿'
    input.dispatchEvent(new Event('input'))
    await flushPromises()
    expect(confirmButton().disabled).toBe(false)

    confirmButton().click()
    await flushPromises()

    expect(updateBoardMock).toHaveBeenCalledWith(1974, { status: 'disabled' })
    expect(notifyMocks.success).toHaveBeenCalledWith('已归档')
    expect(onArchived).toHaveBeenCalledTimes(1)
  })

  it('受保护版块 → 禁用归档并说明原因', async () => {
    await mountDialog(board({ protected: true }))
    expect(document.body.querySelector('[data-testid="bad-protected"]')!.textContent).toContain('受保护')

    const input = confirmInput()
    input.value = 'AI 前沿'
    input.dispatchEvent(new Event('input'))
    await flushPromises()
    expect(confirmButton().disabled).toBe(true)

    confirmButton().click()
    await flushPromises()
    expect(updateBoardMock).not.toHaveBeenCalled()
  })

  it('归档失败 → 行内错误，不上报 archived', async () => {
    updateBoardMock.mockResolvedValue({ success: false, error: '后端 500' })
    const onArchived = vi.fn()
    getBoardsMock.mockResolvedValue({ success: true, data: { items: [board()], total: 1 } })
    wrapper = mount(BoardArchiveDialog, {
      props: { modelValue: true, boardId: 1974, onArchived },
      attachTo: document.body,
    })
    await flushPromises()

    const input = confirmInput()
    input.value = 'AI 前沿'
    input.dispatchEvent(new Event('input'))
    await flushPromises()
    confirmButton().click()
    await flushPromises()

    expect(document.body.querySelector('[data-testid="bad-error"]')!.textContent).toContain('后端 500')
    expect(onArchived).not.toHaveBeenCalled()
  })
})
