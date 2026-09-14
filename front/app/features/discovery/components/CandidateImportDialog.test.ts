import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import CandidateImportDialog from './CandidateImportDialog.vue'
import type {
  CatalogApplyResult,
  CatalogExportFile,
  CatalogImportPreview,
} from '~/types/discovery'

/**
 * 5.3 导入弹窗（spec C3）：
 * - pick 前端预检：≤2MiB / 可解析 JSON（非 JSON/超限就地报错可换文件，不发请求）；
 * - preview 分类渲染：new/duplicate/conflict/invalid 计数 + 逐条原因；
 * - 确认流转：confirm 携带 {fingerprint, local_revision, 文件本体} → 逐项结果 applied/skipped/failed；
 * - 409 stale_preview：提示并自动以原完整文件重新预览，回预览步不自动写库；
 * - 取消零副作用：confirm 之前无任何写请求；
 * - 重试失败项：failed 子集重新 preview+confirm，已成功项不在子集中。
 */

const previewMock = vi.fn()
const confirmMock = vi.fn()
const loadCandidatesMock = vi.fn()

vi.mock('~/api/discovery', () => ({
  useDiscoveryApi: () => ({
    previewCatalogImport: previewMock,
    confirmCatalogImport: confirmMock,
  }),
}))

vi.mock('~/stores/discovery', async () => {
  const { reactive } = await import('vue')
  return {
    useDiscoveryStore: () => reactive({
      candidatesLoaded: true,
      loadCandidates: loadCandidatesMock,
    }),
  }
})

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

function entry(key: string): CatalogExportFile['entries'][number] {
  return { stable_key: key, kind: 'rss', name: `来源-${key}`, feed_url: `https://${key}.example/feed.xml` }
}

function exportFile(keys: string[]): CatalogExportFile {
  return {
    format: 'syntopica-candidate-catalog',
    version: 1,
    entries: keys.map(entry),
  }
}

function preview(over: Partial<CatalogImportPreview> = {}): CatalogImportPreview {
  return {
    counts: { new: 2, duplicate: 1, conflict: 1, invalid: 1 },
    items: [
      { stable_key: 'k1', kind: 'rss', name: '来源-k1', classification: 'new' },
      { stable_key: 'k2', kind: 'rss', name: '来源-k2', classification: 'new' },
      { stable_key: 'k3', kind: 'rss', name: '来源-k3', classification: 'duplicate', reason: '地址已存在，复用本地记录' },
      { stable_key: 'k4', kind: 'rss', name: '来源-k4', classification: 'conflict', reason: '本地同 key 内容不同，保留本地' },
      { stable_key: 'k5', kind: 'rss', name: '来源-k5', classification: 'invalid', reason: 'feed_url 缺失且非 rsshub' },
    ],
    fingerprint: 'fp-1',
    local_revision: 7,
    ...over,
  }
}

function applyResult(over: Partial<CatalogApplyResult> = {}): CatalogApplyResult {
  return {
    applied: ['k1'],
    skipped_duplicate: ['k3'],
    skipped_conflict: [{ stable_key: 'k4', reason: '保留本地' }],
    failed: [{ stable_key: 'k2', reason: '写入失败' }],
    invalid: [{ stable_key: 'k5', reason: 'feed_url 缺失' }],
    ...over,
  }
}

function dialogEl(): HTMLElement | null {
  return document.querySelector('.app-dialog')
}

function clickBtn(testid: string) {
  ;(dialogEl()!.querySelector(`[data-testid="${testid}"]`) as HTMLElement).click()
}

/** 注入文件并触发 change（happy-dom 无法直接构造 FileList，覆写 files 属性） */
async function pickFile(file: File) {
  const input = dialogEl()!.querySelector('[data-testid="import-file-input"]') as HTMLInputElement
  Object.defineProperty(input, 'files', { value: [file], configurable: true })
  input.dispatchEvent(new Event('change', { bubbles: true }))
  await flushPromises()
}

function jsonFile(data: unknown, name = 'catalog.json'): File {
  return new File([JSON.stringify(data)], name, { type: 'application/json' })
}

beforeEach(() => {
  previewMock.mockReset()
  confirmMock.mockReset()
  loadCandidatesMock.mockReset()
  document.body.innerHTML = ''
})

async function mountDialog() {
  const wrapper = mount(CandidateImportDialog, {
    props: { modelValue: false },
    attachTo: document.body,
  })
  await nextTick()
  await wrapper.setProps({ modelValue: true }) // 触发 watch 重置状态机
  await nextTick()
  return wrapper
}

describe('CandidateImportDialog — pick 前端预检（不发请求）', () => {
  it('超 2MiB：就地报错可换文件，不发 preview 请求', async () => {
    previewMock.mockResolvedValue({ success: true, data: preview() })
    const wrapper = await mountDialog()
    const big = new File([new Uint8Array(3 * 1024 * 1024)], 'big.json', { type: 'application/json' })
    await pickFile(big)
    const err = dialogEl()!.querySelector('[data-testid="import-pick-error"]')!
    expect(err.textContent).toContain('2 MiB')
    expect(previewMock).not.toHaveBeenCalled()

    // 换合法文件可继续（同一弹窗内就地下一步）
    await pickFile(jsonFile(exportFile(['k1', 'k2', 'k3', 'k4', 'k5'])))
    expect(previewMock).toHaveBeenCalledTimes(1)
    expect(dialogEl()!.querySelector('[data-testid="import-counts"]')).not.toBeNull()
    wrapper.unmount()
  })

  it('非 JSON 文件：就地报错，不发请求', async () => {
    const wrapper = await mountDialog()
    await pickFile(new File(['{ not json'], 'bad.json', { type: 'application/json' }))
    const err = dialogEl()!.querySelector('[data-testid="import-pick-error"]')!
    expect(err.textContent).toContain('不是有效的 JSON')
    expect(previewMock).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('结构缺 entries：就地报错（文件级 400 同道换文件）', async () => {
    const wrapper = await mountDialog()
    await pickFile(jsonFile({ format: 'syntopica-candidate-catalog', version: 1 }))
    expect(dialogEl()!.querySelector('[data-testid="import-pick-error"]')!.textContent)
      .toContain('entries')
    expect(previewMock).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})

describe('CandidateImportDialog — preview 分类渲染（C3）', () => {
  it('四类计数 + 逐条原因（invalid 原因就地展示）', async () => {
    previewMock.mockResolvedValue({ success: true, data: preview() })
    const wrapper = await mountDialog()
    await pickFile(jsonFile(exportFile(['k1', 'k2', 'k3', 'k4', 'k5'])))
    const counts = dialogEl()!.querySelector('[data-testid="import-counts"]')!
    expect(counts.textContent).toContain('新增 2')
    expect(counts.textContent).toContain('重复 1')
    expect(counts.textContent).toContain('冲突 1')
    expect(counts.textContent).toContain('无效 1')
    const items = dialogEl()!.querySelector('[data-testid="import-preview-items"]')!
    expect(items.textContent).toContain('地址已存在')
    expect(items.textContent).toContain('保留本地')
    expect(dialogEl()!.querySelector('[data-testid="import-item-reason-4"]')!.textContent)
      .toContain('feed_url 缺失')
    wrapper.unmount()
  })

  it('无新增条目：确认按钮禁用（只有 new 才可应用）', async () => {
    previewMock.mockResolvedValue({
      success: true,
      data: preview({
        counts: { new: 0, duplicate: 1, conflict: 0, invalid: 0 },
        items: [{ stable_key: 'k3', kind: 'rss', name: '来源-k3', classification: 'duplicate', reason: '已存在' }],
      }),
    })
    const wrapper = await mountDialog()
    await pickFile(jsonFile(exportFile(['k3'])))
    const btn = dialogEl()!.querySelector('[data-testid="import-confirm"]') as HTMLButtonElement
    expect(btn.disabled).toBe(true)
    expect(btn.textContent).toContain('没有可应用的新增条目')
    wrapper.unmount()
  })
})

describe('CandidateImportDialog — 确认流转与结果（C3）', () => {
  it('确认携带 fingerprint/local_revision/文件本体；结果逐组渲染 + 静默刷新候选库', async () => {
    const file = exportFile(['k1', 'k2', 'k3', 'k4', 'k5'])
    previewMock.mockResolvedValue({ success: true, data: preview() })
    confirmMock.mockResolvedValue({ success: true, data: applyResult() })
    const wrapper = await mountDialog()
    await pickFile(jsonFile(file))
    clickBtn('import-confirm')
    await flushPromises()

    expect(confirmMock).toHaveBeenCalledTimes(1)
    expect(confirmMock).toHaveBeenCalledWith({
      fingerprint: 'fp-1',
      localRevision: 7,
      file,
    })
    // 导入改变了候选库：刷新列表但不停留在结果页
    expect(loadCandidatesMock).toHaveBeenCalledTimes(1)
    const lead = dialogEl()!.querySelector('[data-testid="import-result-lead"]')!
    expect(lead.textContent).toContain('应用 1 条')
    expect(lead.textContent).toContain('失败 1 条')
    expect(dialogEl()!.querySelector('[data-testid="import-result-applied"]')!.textContent)
      .toContain('k1')
    expect(dialogEl()!.querySelector('[data-testid="import-result-failed"]')!.textContent)
      .toContain('写入失败')
    wrapper.unmount()
  })

  it('确认非 409 失败：错误就地展示停留在预览步', async () => {
    previewMock.mockResolvedValue({ success: true, data: preview() })
    confirmMock.mockResolvedValue({ success: false, error: '写入暂不可用' })
    const wrapper = await mountDialog()
    await pickFile(jsonFile(exportFile(['k1', 'k2', 'k3', 'k4', 'k5'])))
    clickBtn('import-confirm')
    await flushPromises()
    expect(dialogEl()!.querySelector('[data-testid="import-confirm-error"]')!.textContent)
      .toContain('写入暂不可用')
    expect(dialogEl()!.querySelector('[data-testid="import-counts"]')).not.toBeNull()
    wrapper.unmount()
  })
})

describe('CandidateImportDialog — 409 stale_preview（C3）', () => {
  it('confirm 409：提示「已变化」并以原完整文件自动重新预览，不自动写库', async () => {
    const file = exportFile(['k1', 'k2', 'k3', 'k4', 'k5'])
    previewMock.mockResolvedValue({ success: true, data: preview() })
    confirmMock.mockResolvedValueOnce({ success: false, status: 409, error: '预览已过期' })
    const wrapper = await mountDialog()
    await pickFile(jsonFile(file))
    clickBtn('import-confirm')
    await flushPromises()

    // 回到预览步 + stale 提示；以原完整文件重新预览（非失败子集）
    expect(dialogEl()!.querySelector('[data-testid="import-stale-notice"]')!.textContent)
      .toContain('发生了变化')
    expect(dialogEl()!.querySelector('[data-testid="import-counts"]')).not.toBeNull()
    expect(previewMock).toHaveBeenCalledTimes(2)
    expect(previewMock).toHaveBeenLastCalledWith(file)
    // 只确认过一次；重新预览后不自动再次 confirm（用户重新核对）
    expect(confirmMock).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
})

describe('CandidateImportDialog — 取消零副作用（C3）', () => {
  it('预览后直接退出：confirm 从未被调用（预览零写库，任何退出方式零副作用）', async () => {
    previewMock.mockResolvedValue({ success: true, data: preview() })
    const wrapper = await mountDialog()
    await pickFile(jsonFile(exportFile(['k1', 'k2', 'k3', 'k4', 'k5'])))
    // preview 步 footer 无取消按钮（换文件/确认），退出走弹窗关闭 ✕
    ;(dialogEl()!.querySelector('.app-dialog__close') as HTMLElement).click()
    await nextTick()
    expect(confirmMock).not.toHaveBeenCalled()
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([false])
    wrapper.unmount()
  })
})

describe('CandidateImportDialog — 重试失败项（C3）', () => {
  it('重试子集只含失败条目：已成功项不重复提交', async () => {
    const file = exportFile(['k1', 'k2'])
    previewMock.mockResolvedValue({
      success: true,
      data: preview({
        counts: { new: 2, duplicate: 0, conflict: 0, invalid: 0 },
        items: [
          { stable_key: 'k1', kind: 'rss', name: '来源-k1', classification: 'new' },
          { stable_key: 'k2', kind: 'rss', name: '来源-k2', classification: 'new' },
        ],
      }),
    })
    confirmMock.mockResolvedValue({
      success: true,
      data: applyResult({
        applied: ['k1'],
        skipped_duplicate: [],
        skipped_conflict: [],
        failed: [{ stable_key: 'k2', reason: '写入失败' }],
        invalid: [],
      }),
    })
    const wrapper = await mountDialog()
    await pickFile(jsonFile(file))
    clickBtn('import-confirm')
    await flushPromises()

    clickBtn('import-retry-failed')
    await flushPromises()
    // 重试范围提示出现；重新 preview 收到的文件只含 k2（k1 已成功不在子集）
    expect(dialogEl()!.querySelector('[data-testid="import-retry-note"]')).not.toBeNull()
    expect(previewMock).toHaveBeenCalledTimes(2)
    const retryFile = previewMock.mock.calls[1]![0] as CatalogExportFile
    expect(retryFile.entries.map(e => e.stable_key)).toEqual(['k2'])
    expect(retryFile.format).toBe('syntopica-candidate-catalog')
    wrapper.unmount()
  })
})
