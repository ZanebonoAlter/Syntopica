import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import BoardSettingsForm from './BoardSettingsForm.vue'
import type { SemanticBoard } from '~/api/semanticBoards'

/**
 * 版块设置页签（3.2）：字段回填 / 保存载荷 / 空名校验 / 档位至少保留一个。
 */
const { updateBoardMock, deleteBoardMock } = vi.hoisted(() => ({ updateBoardMock: vi.fn(), deleteBoardMock: vi.fn() }))
const notifyMocks = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock('~/api/semanticBoards', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/semanticBoards')>()
  return { ...original, useSemanticBoardsApi: () => ({ updateBoard: updateBoardMock, deleteBoard: deleteBoardMock }) }
})
vi.mock('~/composables/useNotify', () => ({ useNotify: () => notifyMocks }))
vi.mock('@iconify/vue', () => ({ Icon: { name: 'Icon', template: '<span />' } }))

const BOARD: SemanticBoard = {
  id: 1974,
  label: '中东地缘政治与美伊关系',
  slug: 'middle-east',
  aliases: [],
  ref_count: 0,
  tag_count: 120,
  description: '美伊与地区代理冲突',
  display_order: 0,
  source: 'manual',
  status: 'active',
  protected: false,
  enrichment_enabled: true,
  relation_auto_discovery_enabled: false,
  window_days: 14,
  context_layers: ['week', 'month'],
  created_at: '',
  updated_at: '',
}

const mounted: Array<ReturnType<typeof mount>> = []

function mountForm(board: SemanticBoard | null = BOARD) {
  const onSaved = vi.fn()
  const wrapper = mount(BoardSettingsForm, {
    props: { board, onSaved },
    attachTo: document.body,
  })
  mounted.push(wrapper)
  return { wrapper, onSaved }
}

function shadowInput(testid: string): HTMLInputElement {
  return document.body.querySelector(`[data-testid="${testid}"] input`) as HTMLInputElement
}

afterEach(() => {
  while (mounted.length) mounted.pop()!.unmount()
  document.body.innerHTML = ''
})

beforeEach(() => {
  updateBoardMock.mockReset()
  updateBoardMock.mockResolvedValue({ success: true, data: { id: 1974 } })
  deleteBoardMock.mockReset()
  deleteBoardMock.mockResolvedValue({ success: true, data: { id: 1974 } })
  notifyMocks.success.mockReset()
  notifyMocks.error.mockReset()
})

describe('BoardSettingsForm', () => {
  it('字段按版块回填（名称/窗口/档位/增强开关）', () => {
    mountForm()
    expect(shadowInput('gov-settings-label').value).toBe('中东地缘政治与美伊关系')
    expect(shadowInput('gov-settings-window').value).toBe('14')
    expect(document.body.querySelector('[data-testid="gov-settings-layer-week"]')!.className).toContain('is-on')
    expect(document.body.querySelector('[data-testid="gov-settings-layer-year"]')!.className).not.toContain('is-on')
    expect(document.body.textContent).toContain('今日雷达（总览）的版块治理')
    expect(document.body.textContent).not.toContain('版块页顶部「更多」')
    expect(document.body.textContent).not.toContain('治理右轨「版块治理」')
  })

  it('保存：载荷含名称/窗口/档位/增强/关系开关，成功后上报 saved', async () => {
    const { onSaved } = mountForm()
    shadowInput('gov-settings-label').value = '中东局势'
    shadowInput('gov-settings-label').dispatchEvent(new Event('input'))
    await flushPromises()

    ;(document.body.querySelector('[data-testid="gov-settings-save"]') as HTMLButtonElement).click()
    await flushPromises()

    expect(updateBoardMock).toHaveBeenCalledWith(1974, expect.objectContaining({
      label: '中东局势',
      window_days: 14,
      context_layers: ['week', 'month'],
      enrichment_enabled: true,
      relation_auto_discovery_enabled: false,
    }))
    expect(notifyMocks.success).toHaveBeenCalled()
    expect(onSaved).toHaveBeenCalledTimes(1)
  })

  it('空名称 → 本地拦截，不发请求', async () => {
    mountForm()
    shadowInput('gov-settings-label').value = '   '
    shadowInput('gov-settings-label').dispatchEvent(new Event('input'))
    await flushPromises()
    ;(document.body.querySelector('[data-testid="gov-settings-save"]') as HTMLButtonElement).click()
    await flushPromises()

    expect(updateBoardMock).not.toHaveBeenCalled()
    // 空名让保存按钮直接禁用（本地拦截），不会进入 save()
    expect((document.body.querySelector('[data-testid="gov-settings-save"]') as HTMLButtonElement).disabled).toBe(true)
  })

  it('档位全部取消 → 保存禁用且提示至少保留一个', async () => {
    mountForm()
    for (const key of ['week', 'month']) {
      (document.body.querySelector(`[data-testid="gov-settings-layer-${key}"]`) as HTMLButtonElement).click()
    }
    await flushPromises()
    const save = document.body.querySelector('[data-testid="gov-settings-save"]') as HTMLButtonElement
    expect(save.disabled).toBe(true)
  })

  it('窗口清空/越界 → 行内错误 + 保存禁用（不发请求）', async () => {
    mountForm()
    const input = shadowInput('gov-settings-window')
    input.value = ''
    input.dispatchEvent(new Event('input'))
    await flushPromises()
    expect(document.body.querySelector('[data-testid="gov-settings-window-error"]')).not.toBeNull()
    expect((document.body.querySelector('[data-testid="gov-settings-save"]') as HTMLButtonElement).disabled).toBe(true)

    input.value = '500'
    input.dispatchEvent(new Event('input'))
    await flushPromises()
    expect(updateBoardMock).not.toHaveBeenCalled()
  })

  it('档位固定 week/month/year/all 顺序输出（不随点击顺序漂移）', async () => {
    mountForm()
    // 先开 all 再开 year（点击顺序与档位顺序相反），输出仍按固定顺序
    ;(document.body.querySelector('[data-testid="gov-settings-layer-all"]') as HTMLButtonElement).click()
    await flushPromises()
    ;(document.body.querySelector('[data-testid="gov-settings-layer-year"]') as HTMLButtonElement).click()
    await flushPromises()
    ;(document.body.querySelector('[data-testid="gov-settings-save"]') as HTMLButtonElement).click()
    await flushPromises()
    // 初始 week/month，点开 all 后仍按固定档位顺序输出（year 未选不补）
    expect(updateBoardMock).toHaveBeenCalledWith(1974, expect.objectContaining({
      context_layers: ['week', 'month', 'year', 'all'],
    }))
  })

  it('数据增强关闭时关系自动发现禁用（依赖联动）', () => {
    mountForm({ ...BOARD, enrichment_enabled: false })
    const toggle = document.body.querySelector('[data-testid="gov-settings-relation"]') as HTMLElement
    expect(toggle.className).toContain('is-disabled')
  })

  it('保存失败 → 展示错误且不触发 saved', async () => {
    updateBoardMock.mockResolvedValueOnce({ success: false, error: '后端 500' })
    const { onSaved } = mountForm()
    await flushPromises()
    ;(document.body.querySelector('[data-testid="gov-settings-save"]') as HTMLButtonElement).click()
    await flushPromises()
    expect(document.body.querySelector('[data-testid="gov-settings-error"]')!.textContent).toContain('后端 500')
    expect(onSaved).not.toHaveBeenCalled()
  })

  it('删除版块：二次确认后调用 API 并上报 deleted', async () => {
    const onDeleted = vi.fn()
    const wrapper = mount(BoardSettingsForm, { props: { board: BOARD, onDeleted }, attachTo: document.body })
    mounted.push(wrapper)
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    ;(document.body.querySelector('[data-testid="gov-settings-delete"]') as HTMLButtonElement).click()
    await flushPromises()
    expect(confirmSpy).toHaveBeenCalled()
    expect(deleteBoardMock).toHaveBeenCalledWith(1974)
    expect(onDeleted).toHaveBeenCalledTimes(1)
    confirmSpy.mockRestore()
  })
})
