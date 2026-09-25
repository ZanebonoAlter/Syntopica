import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { mount } from '@vue/test-utils'
import SettingsSectionMarginNotes from './SettingsSectionMarginNotes.vue'
import type { MarginNoteAdminRow } from '~/api/marginNotes'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

// AppPageShell 走真实组件（无外部依赖）；AppDialog 真实组件（Teleport 到 body）。
const pushMock = vi.fn()
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: pushMock }),
}))

const api = vi.hoisted(() => ({
  getBoards: vi.fn(),
  listAnnotations: vi.fn(),
  deleteAnnotation: vi.fn(),
}))

vi.mock('~/api/semanticBoards', () => ({
  useSemanticBoardsApi: () => api,
}))

vi.mock('../composables/useMarginNoteAdmin', () => ({
  useMarginNoteAdmin: () => ({
    rows: rowsRef,
    total: totalRef,
    loading: loadingRef,
    error: errorRef,
    load: loadMock,
    remove: removeMock,
  }),
}))

const rowsRef = { value: [] as MarginNoteAdminRow[] }
const totalRef = { value: 0 }
const loadingRef = { value: false }
const errorRef = { value: '' }
const loadMock = vi.fn()
const removeMock = vi.fn()

function makeRow(over: Partial<MarginNoteAdminRow> = {}): MarginNoteAdminRow {
  return {
    id: 1,
    report_id: 10,
    section_id: 100,
    thread_id: 7,
    quoted_text: '以利率招标方式开展 3000 亿元 7 天期逆回购操作',
    anchor_offset_start: null,
    anchor_offset_end: null,
    created_at: '2026-09-21T13:00:00Z',
    qas: [
      { id: 9, annotation_id: 1, question: '逆回购是什么？', answer: '短期流动性注入。', cited_article_ids: [], cited_web_sources: [], extracted_terms: [{ term: '逆回购' }], created_at: '' },
    ],
    board_id: 1974,
    board_label: '中国宏观',
    period_date: '2026-09-21',
    issue_number: '第 128 期',
    ...over,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  api.getBoards.mockResolvedValue({ success: true, data: { items: [{ id: 1974, label: '中国宏观' }], total: 1 } })
  api.listAnnotations.mockResolvedValue({ success: true, data: { annotations: [], total: 0 } })
  rowsRef.value = []
  totalRef.value = 0
  loadingRef.value = false
  errorRef.value = ''
  loadMock.mockResolvedValue(undefined)
  removeMock.mockResolvedValue(true)
})

function mountSection() {
  return mount(SettingsSectionMarginNotes, { attachTo: document.body })
}

describe('SettingsSectionMarginNotes（全局批注管理，MG-1~MG-7）', () => {
  it('MG-1：挂载即拉全量列表（含筛选参数），行内展示版块/日期/期号/术语/划词/问答折叠', async () => {
    rowsRef.value = [makeRow()]
    totalRef.value = 6
    const wrapper = mountSection()
    await flushPromises()
    expect(loadMock).toHaveBeenCalledWith(expect.objectContaining({ page_size: 200, page: 1 }))
    expect(wrapper.find('[data-testid="mg-total"]').text()).toBe('6')
    const row = wrapper.find('[data-testid="mg-list"] [data-annotation-id="1"]')
    expect(row.exists()).toBe(true)
    expect(row.find('[data-testid="mg-row-meta"]').text()).toContain('中国宏观')
    expect(row.find('[data-testid="mg-row-meta"]').text()).toContain('2026-09-21')
    expect(row.find('[data-testid="mg-row-meta"]').text()).toContain('第 128 期')
    expect(row.text()).toContain('逆回购')
    expect(row.text()).toContain('开展 3000 亿元')
    expect(row.find('[data-testid="mg-row-qa"]').exists()).toBe(true)
    // 问答折叠：details 默认关闭，展开后可见 QA 内容
    expect(row.find('[data-testid="mg-row-qa"]').attributes('open')).toBeUndefined()
    await row.find('[data-testid="mg-row-qa"] summary').trigger('click')
    expect(row.find('[data-testid="mg-row-qa"]').attributes('open')).toBeDefined()
  })

  it('MG-1：仅标记行（无问答）显示「尚无提问」态', () => {
    rowsRef.value = [makeRow({ id: 3, qas: [] })]
    const wrapper = mountSection()
    expect(wrapper.find('[data-testid="mg-row-noqa"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="mg-row-noqa"]').text()).toContain('仅标记')
  })

  it('MG-2：版块筛选与关键词输入触发 reload（即时过滤），未知组合清空恢复', async () => {
    const wrapper = mountSection()
    await flushPromises()
    loadMock.mockClear()

    await wrapper.find('[data-testid="mg-board-filter"]').setValue('1974')
    expect(loadMock).toHaveBeenLastCalledWith(expect.objectContaining({ board_id: 1974 }))

    await wrapper.find('input[type="search"]').setValue('逆回购')
    expect(loadMock).toHaveBeenLastCalledWith(expect.objectContaining({ q: '逆回购' }))

    // 清空筛选恢复全量
    await wrapper.find('[data-testid="mg-board-filter"]').setValue('')
    await wrapper.find('input[type="search"]').setValue('')
    expect(loadMock).toHaveBeenLastCalledWith(expect.objectContaining({ board_id: undefined, q: undefined }))
  })

  it('MG-3：空态提示区分无数据与筛选无匹配，清筛选恢复', async () => {
    rowsRef.value = []
    errorRef.value = ''
    const wrapper = mountSection()
    // 无数据：暂无批注
    expect(wrapper.find('[data-testid="mg-empty"]').text()).toBe('暂无批注')
    // 有筛选但无匹配：提示清筛选恢复
    await wrapper.find('input[type="search"]').setValue('不存在的词')
    expect(wrapper.find('[data-testid="mg-empty"]').text()).toContain('没有匹配的批注')
  })

  it('MG-6：删除走同源确认弹窗（连带问答），确认后 remove(id)', async () => {
    rowsRef.value = [makeRow({ qas: [makeRow().qas[0]!, { ...makeRow().qas[0]!, id: 10 }] })]
    const wrapper = mountSection()
    await flushPromises()

    await wrapper.find('[data-testid="mg-delete"]').trigger('click')
    expect(removeMock).not.toHaveBeenCalled()
    const confirmText = document.querySelector('[data-testid="mg-confirm-text"]')
    expect(confirmText?.textContent).toContain('连带删除')
    expect(confirmText?.textContent).toContain('2')

    const confirmBtn = document.querySelector('[data-testid="mg-confirm-delete"]') as HTMLButtonElement | null
    expect(confirmBtn).toBeTruthy()
    confirmBtn?.click()
    await flushPromises()
    expect(removeMock).toHaveBeenCalledWith(1)
    wrapper.unmount()
    document.body.innerHTML = ''
  })

  it('MG-6：取消删除不发 remove', async () => {
    rowsRef.value = [makeRow()]
    const wrapper = mountSection()
    await flushPromises()
    await wrapper.find('[data-testid="mg-delete"]').trigger('click')
    const cancelBtn = document.querySelector('[data-testid="mg-cancel-delete"]') as HTMLButtonElement | null
    cancelBtn?.click()
    await flushPromises()
    expect(removeMock).not.toHaveBeenCalled()
    wrapper.unmount()
    document.body.innerHTML = ''
  })

  it('MG-5：跳原日报深链携带 board/report/annotation 三个 query', async () => {
    rowsRef.value = [makeRow({ id: 7, report_id: 128, board_id: 1974 })]
    const wrapper = mountSection()
    await flushPromises()
    await wrapper.find('[data-testid="mg-jump"]').trigger('click')
    expect(pushMock).toHaveBeenCalledWith({
      path: '/tags',
      query: { board: '1974', report: '128', annotation: '7' },
    })
  })

  it('加载失败显示错误行', () => {
    errorRef.value = '批注列表加载失败'
    const wrapper = mountSection()
    expect(wrapper.find('[data-testid="mg-error"]').exists()).toBe(true)
  })

  it('加载中显示骨架', () => {
    loadingRef.value = true
    const wrapper = mountSection()
    expect(wrapper.find('[data-testid="mg-loading"]').exists()).toBe(true)
  })
})
