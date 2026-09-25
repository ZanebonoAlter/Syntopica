import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { ref } from 'vue'
import { useMarginNotes } from './useMarginNotes'
import type { MarginNoteAnnotation } from '~/api/marginNotes'

const api = vi.hoisted(() => ({
  getReportAnnotations: vi.fn(),
  anchorAnnotation: vi.fn(),
  askAnnotation: vi.fn(),
  deleteAnnotation: vi.fn(),
}))

vi.mock('~/api/marginNotes', () => ({
  useMarginNotesApi: () => api,
}))

function makeAnnotation(over: Partial<MarginNoteAnnotation> = {}): MarginNoteAnnotation {
  return {
    id: 1,
    report_id: 10,
    section_id: 100,
    thread_id: 7,
    quoted_text: '开展逆回购操作',
    anchor_offset_start: 6,
    anchor_offset_end: 12,
    created_at: '2026-09-22T00:00:00Z',
    qas: [],
    ...over,
  }
}

describe('useMarginNotes', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.getReportAnnotations.mockResolvedValue({ success: true, data: { annotations: [] } })
    api.anchorAnnotation.mockResolvedValue({ success: true, data: { annotation: makeAnnotation({ id: 5 }) } })
    api.askAnnotation.mockResolvedValue({
      success: true,
      data: { qa: { id: 9, annotation_id: 5, question: 'Q', answer: 'A', cited_article_ids: [1, 2], extracted_terms: [{ term: '逆回购', is_new: true }], created_at: '2026-09-22T01:00:00Z' } },
    })
    api.deleteAnnotation.mockResolvedValue({ success: true, data: null })
  })

  it('按 reportId 拉取批注；切报告自动重拉且旧响应不串场', async () => {
    const reportId = ref(10)
    const notes = useMarginNotes(reportId)
    await flushPromises()
    expect(api.getReportAnnotations).toHaveBeenCalledWith(10)
    expect(notes.loadedReportId.value).toBe(10)

    api.getReportAnnotations.mockResolvedValue({
      success: true,
      data: { annotations: [makeAnnotation({ id: 2, report_id: 11 })] },
    })
    reportId.value = 11
    await flushPromises()
    expect(api.getReportAnnotations).toHaveBeenCalledWith(11)
    expect(notes.annotations.value.map(item => item.id)).toEqual([2])
  })

  it('拉取失败置 loadError 且整栏可重试（State Matrix：进入报告加载失败）', async () => {
    api.getReportAnnotations.mockResolvedValue({ success: false, error: '服务不可用' })
    const notes = useMarginNotes(ref(10))
    await flushPromises()
    expect(notes.loadError.value).toBe('服务不可用')

    api.getReportAnnotations.mockResolvedValue({ success: true, data: { annotations: [makeAnnotation()] } })
    await notes.load(true)
    expect(notes.loadError.value).toBe('')
    expect(notes.annotations.value.length).toBe(1)
  })

  it('落锚：乐观插入临时行，成功后替换为服务端 id', async () => {
    const notes = useMarginNotes(ref(10))
    await flushPromises()

    const created = await notes.anchor({
      section_id: 100,
      thread_id: 7,
      quoted_text: '开展逆回购操作',
      anchor_offset_start: 6,
      anchor_offset_end: 12,
    })
    expect(api.anchorAnnotation).toHaveBeenCalledWith(10, {
      section_id: 100,
      thread_id: 7,
      quoted_text: '开展逆回购操作',
      anchor_offset_start: 6,
      anchor_offset_end: 12,
    })
    expect(created?.id).toBe(5)
    expect(notes.annotations.value.map(item => item.id)).toEqual([5])
  })

  it('落锚失败：临时行回滚不残留', async () => {
    api.anchorAnnotation.mockResolvedValue({ success: false, error: '网络错误' })
    const notes = useMarginNotes(ref(10))
    await flushPromises()
    const created = await notes.anchor({ section_id: 1, thread_id: null, quoted_text: 'x', anchor_offset_start: 0, anchor_offset_end: 1 })
    expect(created).toBeNull()
    expect(notes.annotations.value).toHaveLength(0)
    expect(notes.anchorError.value).toBe('网络错误')
  })

  it('提问：成功追加 QA 轮；历史轮保留', async () => {
    api.getReportAnnotations.mockResolvedValue({
      success: true,
      data: { annotations: [makeAnnotation({ id: 5, qas: [{ id: 1, annotation_id: 5, question: '首轮', answer: '答1', cited_article_ids: [], cited_web_sources: [], extracted_terms: [], created_at: '' }] })] },
    })
    const notes = useMarginNotes(ref(10))
    await flushPromises()

    const ok = await notes.ask(5, '那到期不续做会怎样')
    expect(ok).toBe(true)
    const qas = notes.annotations.value[0]!.qas
    expect(qas).toHaveLength(2)
    expect(qas[0]!.question).toBe('首轮')
    // 追加轮以服务端返回的 qa 为准
    expect(qas[1]!.question).toBe('Q')
    expect(notes.askState(5).pendingQuestion).toBeNull()
  })

  it('提问失败：问题文本保留在 failedQuestion，重试复用不丢（AV-1）', async () => {
    api.askAnnotation
      .mockResolvedValueOnce({ success: false, error: '模型超时' })
      .mockResolvedValueOnce({ success: true, data: { qa: { id: 9, annotation_id: 5, question: '逆回购是什么', answer: 'A', cited_article_ids: [], cited_web_sources: [], extracted_terms: [], created_at: '' } } })
    const notes = useMarginNotes(ref(10))
    await flushPromises()
    await notes.anchor({ section_id: 100, thread_id: 7, quoted_text: 'q', anchor_offset_start: 0, anchor_offset_end: 1 })
    await flushPromises()

    const first = await notes.ask(5, '逆回购是什么')
    expect(first).toBe(false)
    expect(notes.askState(5).error).toBe('模型超时')
    expect(notes.askState(5).failedQuestion).toBe('逆回购是什么')

    await notes.retryAsk(5)
    // 重试发出的是保留的问题文本，而非空串
    expect(api.askAnnotation).toHaveBeenLastCalledWith(5, '逆回购是什么')
    expect(notes.askState(5).error).toBeNull()
    expect(notes.annotations.value[0]!.qas).toHaveLength(1)
  })

  it('提问 loading 中重复提交被锁定，只发一次请求（ID-2）', async () => {
    let resolveAsk: (value: unknown) => void = () => {}
    api.askAnnotation.mockReturnValue(new Promise((resolve) => { resolveAsk = resolve }))
    const notes = useMarginNotes(ref(10))
    await flushPromises()
    await notes.anchor({ section_id: 100, thread_id: 7, quoted_text: 'q', anchor_offset_start: 0, anchor_offset_end: 1 })
    await flushPromises()

    const first = notes.ask(5, '问题')
    const second = notes.ask(5, '问题')
    expect(await second).toBe(false)
    resolveAsk({ success: true, data: { qa: { id: 9, annotation_id: 5, question: '问题', answer: 'A', cited_article_ids: [], cited_web_sources: [], extracted_terms: [], created_at: '' } } })
    expect(await first).toBe(true)
    expect(api.askAnnotation).toHaveBeenCalledTimes(1)
  })

  it('删除：确认（调用方职责）后请求并移除本地行', async () => {
    api.getReportAnnotations.mockResolvedValue({
      success: true,
      data: { annotations: [makeAnnotation({ id: 5 }), makeAnnotation({ id: 6 })] },
    })
    const notes = useMarginNotes(ref(10))
    await flushPromises()

    const ok = await notes.remove(5)
    expect(api.deleteAnnotation).toHaveBeenCalledWith(5)
    expect(ok).toBe(true)
    expect(notes.annotations.value.map(item => item.id)).toEqual([6])
  })

  it('annotationsByThread：按 thread 分组索引供正文高亮反查', async () => {
    api.getReportAnnotations.mockResolvedValue({
      success: true,
      data: { annotations: [makeAnnotation({ id: 1, thread_id: 7 }), makeAnnotation({ id: 2, thread_id: null })] },
    })
    const notes = useMarginNotes(ref(10))
    await flushPromises()
    expect(notes.annotationsByThread.value.get(7)?.map(item => item.id)).toEqual([1])
    expect(notes.annotationsByThread.value.has(0)).toBe(false)
  })
})
