import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useMarginNoteAdmin } from './useMarginNoteAdmin'
import type { MarginNoteAdminRow } from '~/api/marginNotes'

const api = vi.hoisted(() => ({
  listAnnotations: vi.fn(),
  deleteAnnotation: vi.fn(),
}))

vi.mock('~/api/marginNotes', () => ({
  useMarginNotesApi: () => api,
}))

function makeRow(over: Partial<MarginNoteAdminRow> = {}): MarginNoteAdminRow {
  return {
    id: 1,
    report_id: 10,
    section_id: 100,
    thread_id: 7,
    quoted_text: '开展逆回购操作',
    anchor_offset_start: null,
    anchor_offset_end: null,
    created_at: '2026-09-22T00:00:00Z',
    qas: [],
    board_id: 1974,
    board_label: '中国宏观',
    period_date: '2026-09-21',
    issue_number: '第 128 期',
    ...over,
  }
}

describe('useMarginNoteAdmin（管理页数据层，MG-1/MG-2/MG-6）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.listAnnotations.mockResolvedValue({ success: true, data: { annotations: [], total: 0 } })
    api.deleteAnnotation.mockResolvedValue({ success: true, data: null })
  })

  it('load 透传 board_id/q/分页参数，回填 rows 与 total', async () => {
    api.listAnnotations.mockResolvedValue({
      success: true,
      data: { annotations: [makeRow()], total: 6 },
    })
    const admin = useMarginNoteAdmin()
    await admin.load({ board_id: 1974, q: '逆回购', page_size: 200, page: 1 })
    expect(api.listAnnotations).toHaveBeenCalledWith({ board_id: 1974, q: '逆回购', page_size: 200, page: 1 })
    expect(admin.rows.value).toHaveLength(1)
    expect(admin.total.value).toBe(6)
  })

  it('加载失败置 error，不残留旧行', async () => {
    api.listAnnotations.mockResolvedValue({ success: false, error: '服务不可用' })
    const admin = useMarginNoteAdmin()
    await admin.load()
    expect(admin.error.value).toBe('服务不可用')
    expect(admin.rows.value).toHaveLength(0)
  })

  it('remove 成功后本地移除行且 total -1（MG-6 行移除 + 计数刷新）', async () => {
    api.listAnnotations.mockResolvedValue({
      success: true,
      data: { annotations: [makeRow({ id: 1 }), makeRow({ id: 2 })], total: 2 },
    })
    const admin = useMarginNoteAdmin()
    await admin.load()
    const ok = await admin.remove(1)
    expect(api.deleteAnnotation).toHaveBeenCalledWith(1)
    expect(ok).toBe(true)
    expect(admin.rows.value.map(row => row.id)).toEqual([2])
    expect(admin.total.value).toBe(1)
  })

  it('remove 失败保留行（删除与日报内同源：失败不影响数据）', async () => {
    api.listAnnotations.mockResolvedValue({
      success: true,
      data: { annotations: [makeRow({ id: 1 })], total: 1 },
    })
    api.deleteAnnotation.mockResolvedValue({ success: false, error: '删除失败' })
    const admin = useMarginNoteAdmin()
    await admin.load()
    const ok = await admin.remove(1)
    expect(ok).toBe(false)
    expect(admin.rows.value).toHaveLength(1)
    expect(admin.total.value).toBe(1)
  })
})
