import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useMarginNotesApi } from './marginNotes'

const apiClient = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  delete: vi.fn(),
  buildQueryParams: (params: unknown) => new URLSearchParams(params as Record<string, string>).toString(),
}))

vi.mock('./client', () => ({
  apiClient,
}))

describe('useMarginNotesApi（契约归一化）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('QA terms 字段归一化：后端 terms[]（含字符串条目）→ extracted_terms 对象数组；cited 非数组兜底 []', async () => {
    apiClient.get.mockResolvedValue({
      success: true,
      data: {
        annotations: [{
          id: 1, report_id: 10, section_id: 100, thread_id: 7,
          quoted_text: 'q', anchor_offset_start: null, anchor_offset_end: null,
          created_at: '', qas: [],
        }, {
          id: 2, report_id: 10, section_id: 100, thread_id: null,
          quoted_text: 'q2', anchor_offset_start: null, anchor_offset_end: null,
          created_at: '', qas: [{
            id: 9, annotation_id: 2, question: 'q?', answer: 'a',
            cited_article_ids: 'not-an-array',
            terms: ['逆回购', { term: 'DR007', is_new: true }, ''],
            created_at: '',
          }],
        }],
      },
    })
    const { getReportAnnotations } = useMarginNotesApi()
    const response = await getReportAnnotations(10)
    const secondQa = response.data!.annotations[1]!.qas[0]!
    expect(secondQa.cited_article_ids).toEqual([])
    expect(secondQa.extracted_terms).toEqual([{ term: '逆回购' }, { term: 'DR007', is_new: true }])
  })

  it('D7 网络来源归一化：缺字段/旧轮次 → []；非法条目剔除、保序保留 {title,url}', async () => {
    apiClient.get.mockResolvedValue({
      success: true,
      data: {
        annotations: [{
          id: 1, report_id: 10, section_id: 100, thread_id: 7,
          quoted_text: 'q', anchor_offset_start: null, anchor_offset_end: null,
          created_at: '',
          qas: [
            { id: 9, annotation_id: 1, question: 'q?', answer: 'a', cited_article_ids: [], created_at: '' },
            {
              id: 10, annotation_id: 1, question: 'q2?', answer: 'a2',
              cited_article_ids: [], created_at: '',
              cited_web_sources: [
                { title: '逆回购百科', url: 'https://a.example.com/x' },
                { url: 'https://b.example.com/y' },
                { title: '缺 url 应剔除' },
                'not-an-object',
              ],
            },
          ],
        }],
      },
    })
    const { getReportAnnotations } = useMarginNotesApi()
    const response = await getReportAnnotations(10)
    const qas = response.data!.annotations[0]!.qas
    expect(qas[0]!.cited_web_sources).toEqual([])
    expect(qas[1]!.cited_web_sources).toEqual([
      { title: '逆回购百科', url: 'https://a.example.com/x' },
      { title: '', url: 'https://b.example.com/y' },
    ])
  })

  it('提问响应走同一归一化（terms 优先、extracted_terms 兼容）', async () => {
    apiClient.post.mockResolvedValue({
      success: true,
      data: {
        qa: {
          id: 9, annotation_id: 1, question: 'q', answer: 'a',
          cited_article_ids: [1, 2],
          extracted_terms: [{ term: '缩表' }],
          created_at: '',
        },
      },
    })
    const { askAnnotation } = useMarginNotesApi()
    const response = await askAnnotation(1, 'q?')
    expect(apiClient.post).toHaveBeenCalledWith('/annotations/1/questions', { question: 'q?' })
    expect(response.data!.qa.extracted_terms).toEqual([{ term: '缩表' }])
  })

  it('落锚响应：POST 返回 qa 关联未装载（qas:null）→ 归一为 []，不炸卡片渲染', async () => {
    apiClient.post.mockResolvedValue({
      success: true,
      data: {
        annotation: {
          id: 3, report_id: 10, section_id: 100, thread_id: 7,
          quoted_text: 'q', anchor_offset_start: 0, anchor_offset_end: 1,
          created_at: '', qas: null,
        },
      },
    })
    const { anchorAnnotation } = useMarginNotesApi()
    const response = await anchorAnnotation(10, { section_id: 100, thread_id: 7, quoted_text: 'q', anchor_offset_start: 0, anchor_offset_end: 1 })
    expect(response.data!.annotation.qas).toEqual([])
  })

  it('提问响应缺 qa 载荷 → 归一为失败响应（行内重试路径），不抛错卡 pending', async () => {
    apiClient.post.mockResolvedValue({
      success: true,
      data: { answer: 'a', cited_article_ids: [1], terms: ['逆回购'] },
    })
    const { askAnnotation } = useMarginNotesApi()
    const response = await askAnnotation(1, 'q?')
    expect(response.success).toBe(false)
    expect(response.data).toBeUndefined()
  })

  it('跨报告列表：qas 同一化归一化（库里 extracted_terms 是字符串数组）', async () => {
    apiClient.get.mockResolvedValue({
      success: true,
      data: {
        total: 1,
        annotations: [{
          id: 1, report_id: 2, section_id: 3, thread_id: null,
          quoted_text: 'q', anchor_offset_start: null, anchor_offset_end: null, created_at: '',
          board_id: 1974, board_label: '中国宏观', terms: ['逆回购'],
          qas: [{ id: 9, annotation_id: 1, question: 'q?', answer: 'a', cited_article_ids: [1], extracted_terms: ['逆回购'], created_at: '' }],
        }],
      },
    })
    const { listAnnotations } = useMarginNotesApi()
    const response = await listAnnotations({ board_id: 1974 })
    expect(response.data!.annotations[0]!.qas[0]!.extracted_terms).toEqual([{ term: '逆回购' }])
  })

  it('端点路径：列表/删除/落锚（delete 与 list 不改写载荷）', async () => {
    apiClient.delete.mockResolvedValue({ success: true, data: null })
    apiClient.get.mockResolvedValue({ success: true, data: { annotations: [{ id: 1, report_id: 2, section_id: 3, thread_id: null, quoted_text: '', anchor_offset_start: null, anchor_offset_end: null, created_at: '', qas: [] }], total: 1 } })
    const api = useMarginNotesApi()
    await api.deleteAnnotation(7)
    expect(apiClient.delete).toHaveBeenCalledWith('/annotations/7')
    await api.listAnnotations({ board_id: 1974, q: '逆回购', page_size: 50, page: 2 })
    expect(apiClient.get).toHaveBeenCalledWith('/annotations?board_id=1974&q=%E9%80%86%E5%9B%9E%E8%B4%AD&page_size=50&page=2')
    await api.anchorAnnotation(10, { section_id: 3, thread_id: null, quoted_text: 'x', anchor_offset_start: 0, anchor_offset_end: 1 })
    expect(apiClient.post).toHaveBeenCalledWith('/daily-reports/10/annotations', { section_id: 3, thread_id: null, quoted_text: 'x', anchor_offset_start: 0, anchor_offset_end: 1 })
  })
})
