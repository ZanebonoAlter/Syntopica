/**
 * BoardConsoleShell — 共享描述修复宿主接线（board-health-triage-advice seam C3 / tasks 3.4）。
 *
 * 接线链（真实既有链，不新写删除算法）：
 *   工作面 `remove-composition`（canonical 字符串 ID）
 *     → 壳层 `handleRemoveComposition` → Number(auxId)
 *     → 既有 `removeFromComposition(boardId, auxId)`（DELETE /semantic-boards/:id/composition/:auxId）
 *     → 成功才回填 `removedAuxIds`（工作面据此显示「已摘除」）；失败不谎称已摘除。
 *
 * 失败通知由既有链发一次（`useBoardComposition.removeLabel` 内 notifyError），壳层不叠加 toast。
 */
import { describe, expect, it, beforeEach } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import {
  installShellTestHooks,
  mountShell,
  removeFromComposition,
  getComposition,
} from './boardConsoleShellTestHarness'

installShellTestHooks()

describe('BoardConsoleShell — 摘除接线（工作面 → 既有 DELETE 链 → 回填已摘除）', () => {
  beforeEach(() => {
    removeFromComposition.mockReset().mockResolvedValue({ success: true, data: { board_id: 1974, auxiliary_label_id: 77 } })
    getComposition.mockResolvedValue({ success: true, data: { items: [], composites: [], total: 0 } })
  })

  it('工作面发出摘除 → 走既有 DELETE 链（数字 ID）→ 成功后回填 removedAuxIds', async () => {
    const wrapper = mountShell(1974)
    await wrapper.find('[data-testid="workface-remove"]').trigger('click')
    await flushPromises()

    expect(removeFromComposition).toHaveBeenCalledWith(1974, 77)
    expect(wrapper.find('[data-testid="workface-stub"]').attributes('data-removed-count')).toBe('1')
  })

  it('摘除失败：不回填 removedAuxIds（不谎称已摘除）', async () => {
    removeFromComposition.mockResolvedValue({ success: false, error: '移除失败' })
    const wrapper = mountShell(1974)
    await wrapper.find('[data-testid="workface-remove"]').trigger('click')
    await flushPromises()

    expect(removeFromComposition).toHaveBeenCalledWith(1974, 77)
    expect(wrapper.find('[data-testid="workface-stub"]').attributes('data-removed-count')).toBe('0')
  })
})
