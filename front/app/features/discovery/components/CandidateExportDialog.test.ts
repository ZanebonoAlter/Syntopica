import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import CandidateExportDialog from './CandidateExportDialog.vue'
import type { CatalogExportResult } from '~/types/discovery'

/**
 * 5.3 导出弹窗（spec C4 默认安全导出）：
 * - 打开即调 export：预览条数 + 三类排除计数与原因（0 也展示，不隐藏）；
 * - 「导出并下载」用文件本体组 Blob 触发真实下载（createObjectURL + a[download]）；
 * - include_private v1 后端恒 400：前端预拦截——勾选给提示并禁用导出，不发必败请求。
 */

const exportMock = vi.fn()

vi.mock('~/api/discovery', () => ({
  useDiscoveryApi: () => ({
    exportCandidates: exportMock,
  }),
}))

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

function exportResult(over: Partial<CatalogExportResult> = {}): CatalogExportResult {
  return {
    export: {
      format: 'syntopica-candidate-catalog',
      version: 1,
      entries: [
        {
          stable_key: 'rss:https://a.example/feed.xml',
          kind: 'rss',
          name: '来源 A',
          feed_url: 'https://a.example/feed.xml',
        },
        {
          stable_key: 'rsshub:/test/blog/:id',
          kind: 'rsshub',
          name: '来源 B',
          route_namespace: '/test',
          route_path: '/blog/:id',
        },
      ],
    },
    excluded_private: 1,
    excluded_query: 2,
    excluded_sensitive: 0,
    ...over,
  }
}

function dialogEl(): HTMLElement | null {
  return document.querySelector('.app-dialog')
}

function clickBtn(testid: string) {
  ;(dialogEl()!.querySelector(`[data-testid="${testid}"]`) as HTMLElement).click()
}

/** Blob 下载桩：捕获 createObjectURL 入参与 anchor 点击（happy-dom 无真实下载） */
let createdBlobs: Blob[] = []
/** 点击过的 anchor（组件在点击后同步 remove，需在 spy 内捕获） */
let clickedAnchors: HTMLAnchorElement[] = []
const originalCreateObjectURL = URL.createObjectURL
const originalRevokeObjectURL = URL.revokeObjectURL

beforeEach(() => {
  exportMock.mockReset()
  createdBlobs = []
  clickedAnchors = []
  document.body.innerHTML = ''
  // happy-dom 未实现 createObjectURL：直接覆盖并在 afterEach 恢复，不污染其他文件
  ;(URL as unknown as Record<string, unknown>).createObjectURL = vi.fn((blob: Blob) => {
    createdBlobs.push(blob)
    return `blob:mock-${createdBlobs.length}`
  })
  ;(URL as unknown as Record<string, unknown>).revokeObjectURL = vi.fn()
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
    clickedAnchors.push(this)
  })
})

afterEach(() => {
  ;(URL as unknown as Record<string, unknown>).createObjectURL = originalCreateObjectURL
  ;(URL as unknown as Record<string, unknown>).revokeObjectURL = originalRevokeObjectURL
  vi.restoreAllMocks()
})

async function mountDialog() {
  // 生产生命周期：父级先以 false 挂载，用户点击后置 true 触发 watch 加载（mount 即 true 不会触发非 immediate watch）
  const wrapper = mount(CandidateExportDialog, {
    props: { modelValue: false },
    attachTo: document.body,
  })
  await nextTick()
  await wrapper.setProps({ modelValue: true })
  await flushPromises()
  return wrapper
}

describe('CandidateExportDialog — 默认安全导出预览（C4）', () => {
  it('打开即调 export：渲染条数与三类排除计数及原因（0 也展示）', async () => {
    exportMock.mockResolvedValue({ success: true, data: exportResult() })
    const wrapper = await mountDialog()
    expect(exportMock).toHaveBeenCalledTimes(1)
    expect(exportMock).toHaveBeenCalledWith()
    expect(dialogEl()!.querySelector('[data-testid="export-summary"]')!.textContent)
      .toContain('2')
    // 三类排除逐条计数 + 原因（excluded_sensitive=0 也出现，明确「没有排除」而非隐藏）
    const privateRow = dialogEl()!.querySelector('[data-testid="export-excluded-private"]')!
    expect(privateRow.textContent).toContain('私有来源')
    expect(privateRow.textContent).toContain('1 条')
    const queryRow = dialogEl()!.querySelector('[data-testid="export-excluded-query"]')!
    expect(queryRow.textContent).toContain('地址含查询串')
    expect(queryRow.textContent).toContain('2 条')
    expect(queryRow.textContent).toContain('token')
    const sensitiveRow = dialogEl()!.querySelector('[data-testid="export-excluded-sensitive"]')!
    expect(sensitiveRow.textContent).toContain('敏感地址')
    expect(sensitiveRow.textContent).toContain('0 条')
    // 范围说明：文件仅目录配置，不含订阅状态/阅读记录/兴趣画像
    expect(dialogEl()!.querySelector('[data-testid="export-scope-note"]')!.textContent)
      .toContain('不含订阅状态')
    wrapper.unmount()
  })

  it('导出预览失败：错误态就地展示，可重试', async () => {
    exportMock
      .mockResolvedValueOnce({ success: false, error: '服务暂不可用' })
      .mockResolvedValueOnce({ success: true, data: exportResult() })
    const wrapper = await mountDialog()
    expect(dialogEl()!.querySelector('[data-testid="export-error"]')!.textContent)
      .toContain('服务暂不可用')
    clickBtn('export-retry')
    await flushPromises()
    expect(exportMock).toHaveBeenCalledTimes(2)
    expect(dialogEl()!.querySelector('[data-testid="export-summary"]')).not.toBeNull()
    wrapper.unmount()
  })
})

describe('CandidateExportDialog — Blob 下载', () => {
  it('点「导出并下载」：createObjectURL + a[download=文件名] 点击触发下载', async () => {
    exportMock.mockResolvedValue({ success: true, data: exportResult() })
    const wrapper = await mountDialog()
    clickBtn('export-confirm')
    await nextTick()
    expect(createdBlobs).toHaveLength(1)
    expect(createdBlobs[0]!.type).toBe('application/json')
    // anchor：download 文件名 syntopica-candidates-<日期>.json，且真的点了
    expect(clickedAnchors).toHaveLength(1)
    expect(clickedAnchors[0]!.download).toMatch(/^syntopica-candidates-\d{4}-\d{2}-\d{2}\.json$/)
    expect(dialogEl()!.querySelector('[data-testid="export-downloaded"]')).not.toBeNull()
    wrapper.unmount()
  })
})

describe('CandidateExportDialog — include_private 前端预拦截（C4：v1 不支持私有导出）', () => {
  it('勾选开关：提示出现 + 导出禁用 + 不再发 export 请求（绝不发必败请求）', async () => {
    exportMock.mockResolvedValue({ success: true, data: exportResult() })
    const wrapper = await mountDialog()
    expect(exportMock).toHaveBeenCalledTimes(1)

    const track = dialogEl()!.querySelector('[data-testid="export-include-private"] .app-toggle__track') as HTMLElement
    track.click()
    await nextTick()
    const notice = dialogEl()!.querySelector('[data-testid="export-private-notice"]')!
    expect(notice.textContent).toContain('暂不支持')
    const confirm = dialogEl()!.querySelector('[data-testid="export-confirm"]') as HTMLButtonElement
    expect(confirm.disabled).toBe(true)

    // 禁用状态下点击不触发下载、不产生新请求
    confirm.click()
    await nextTick()
    expect(createdBlobs).toHaveLength(0)
    expect(exportMock).toHaveBeenCalledTimes(1)

    // 关闭开关恢复可导出
    ;(dialogEl()!.querySelector('[data-testid="export-include-private"] .app-toggle__track') as HTMLElement).click()
    await nextTick()
    expect((dialogEl()!.querySelector('[data-testid="export-confirm"]') as HTMLButtonElement).disabled).toBe(false)
    wrapper.unmount()
  })
})
