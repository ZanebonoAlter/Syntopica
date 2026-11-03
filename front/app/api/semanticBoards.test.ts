import { beforeEach, describe, expect, it, vi } from 'vitest'

const { postMock, getMock } = vi.hoisted(() => ({
  postMock: vi.fn(),
  getMock: vi.fn(),
}))

// Keep all real methods (incl. prototype buildQueryParams); only override the HTTP verbs
// on the singleton instance — spreading would drop non-own prototype methods.
vi.mock('./client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./client')>()
  actual.apiClient.get = getMock
  actual.apiClient.post = postMock
  return { apiClient: actual.apiClient }
})

import { useBoardRadarApi, useSemanticBoardsApi } from './semanticBoards'

describe('useSemanticBoardsApi - persisted upgrade suggestions', () => {
  beforeEach(() => {
    postMock.mockReset()
    getMock.mockReset()
    vi.restoreAllMocks()
  })

  it('getUpgradeSuggestions: no params → GET /upgrade-suggestions without query', async () => {
    const expected = { success: true, data: { suggestions: [] } }
    getMock.mockResolvedValue(expected)
    const api = useSemanticBoardsApi()
    const res = await api.getUpgradeSuggestions()
    expect(getMock).toHaveBeenCalledWith('/semantic-boards/upgrade-suggestions')
    expect(res).toEqual(expected)
  })

  it('getUpgradeSuggestions: status+decision → query string', async () => {
    getMock.mockResolvedValue({ success: true, data: { suggestions: [] } })
    const api = useSemanticBoardsApi()
    await api.getUpgradeSuggestions({ status: 'pending', decision: 'merge_into_existing' })
    expect(getMock).toHaveBeenCalledWith('/semantic-boards/upgrade-suggestions?status=pending&decision=merge_into_existing')
  })

  it('getUpgradeSuggestions: decision=watch → observation pool query', async () => {
    getMock.mockResolvedValue({ success: true, data: { suggestions: [] } })
    const api = useSemanticBoardsApi()
    await api.getUpgradeSuggestions({ decision: 'watch' })
    expect(getMock).toHaveBeenCalledWith('/semantic-boards/upgrade-suggestions?decision=watch')
  })

  it('dismissUpgradeSuggestion: with reason → POST body { reason }', async () => {
    const expected = { success: true, data: { id: 5, status: 'dismissed' } }
    postMock.mockResolvedValue(expected)
    const api = useSemanticBoardsApi()
    const res = await api.dismissUpgradeSuggestion(5, '不合适')
    expect(postMock).toHaveBeenCalledWith('/semantic-boards/upgrade-suggestions/5/dismiss', { reason: '不合适' })
    expect(res).toEqual(expected)
  })

  it('dismissUpgradeSuggestion: no reason → POST body undefined', async () => {
    postMock.mockResolvedValue({ success: true, data: { id: 7, status: 'dismissed' } })
    const api = useSemanticBoardsApi()
    await api.dismissUpgradeSuggestion(7)
    expect(postMock).toHaveBeenCalledWith('/semantic-boards/upgrade-suggestions/7/dismiss', undefined)
  })

  it('generateUpgradeSuggestions → POST /generate with four-grid params', async () => {
    const expected = { success: true, data: { inserted: 3, skipped: 1, cooldown_blocked: 2 } }
    postMock.mockResolvedValue(expected)
    const api = useSemanticBoardsApi()
    const res = await api.generateUpgradeSuggestions({ direction: 'expand', source: 'composite', target_board_id: 42 })
    expect(postMock).toHaveBeenCalledWith('/semantic-boards/upgrade-suggestions/generate?direction=expand&source=composite&target_board_id=42')
    expect(res).toEqual(expected)
  })

  it('executeUpgrade: carries suggestion_id for confirm linkage', async () => {
    postMock.mockResolvedValue({ success: true, data: { semantic_board_id: 9, auxiliary_label_ids: [1, 2] } })
    const api = useSemanticBoardsApi()
    await api.executeUpgrade({
      decision: 'merge_into_existing',
      target_board_id: 3,
      auxiliary_label_ids: [1, 2],
      suggestion_id: 10,
    })
    expect(postMock).toHaveBeenCalledWith('/semantic-boards/upgrade-execute', {
      decision: 'merge_into_existing',
      target_board_id: 3,
      auxiliary_label_ids: [1, 2],
      suggestion_id: 10,
    })
  })
})

describe('useBoardRadarApi - Radar 客户端边界归一（8A/R-18）', () => {
  beforeEach(() => {
    getMock.mockReset()
    postMock.mockReset()
    vi.restoreAllMocks()
  })

  it('getOverview: 真实 raw snake_case 响应 → camelCase/字符串 ID 读模型', async () => {
    // raw 形态对齐 GET /semantic-boards/overview 后端契约（snake_case + 数字 ID）
    getMock.mockResolvedValue({
      success: true,
      data: {
        date: '2026-10-24',
        boards: [
          {
            board_id: 1974,
            label: '中东地缘政治与美伊关系',
            tag_count: 144,
            report_date: '2026-10-24',
            report_title: '伊朗警告打击海湾海底电缆',
            report_article_count: 14,
            report_event_count: 13,
            anchor_date: '2026-10-22',
            today_sections: 8,
            last3_sections: 13,
            prev3_sections: 10,
            attention: 26,
            data_delayed: true,
            days: Array.from({ length: 14 }, () => 1),
            lanes: [
              { topic_id: 1204, label: '美伊博弈', section_count_14d: 15, thread_count_14d: 40, counts: Array.from({ length: 14 }, () => 1) },
            ],
          },
          {
            board_id: 50413,
            label: '影视作品',
            tag_count: 0,
            report_date: '2026-10-24',
            report_title: '本周剧集与行业动向',
            report_article_count: 3,
            report_event_count: 2,
            today_sections: 0,
            last3_sections: 0,
            prev3_sections: 0,
            attention: 0,
            data_delayed: false,
            days: [],
            lanes: [],
          },
        ],
      },
    })

    const res = await useBoardRadarApi().getOverview()
    expect(getMock).toHaveBeenCalledWith('/semantic-boards/overview')
    expect(res.success).toBe(true)
    // 判别联合显式 guard：expect(res.success) 不构成 TS 窄化，窄化后才可取归一化读模型字段
    if (!res.success) throw new Error('预期成功响应')
    const [board, coldBoard] = res.data.boards
    expect(board?.boardId).toBe('1974')
    expect(board?.todaySections).toBe(8)
    expect(board?.last3Sections).toBe(13)
    expect(board?.anchorDate).toBe('2026-10-22')
    expect(board?.dataDelayed).toBe(true)
    expect(board?.lanes[0]?.topicId).toBe('1204')
    expect(board?.lanes[0]?.sectionCount14d).toBe(15)
    // 0 值/无锚日版块（影视形态）仍归一为合法读模型
    expect(coldBoard?.boardId).toBe('50413')
    expect(coldBoard?.anchorDate).toBeUndefined()
    expect(coldBoard?.lanes).toEqual([])
  })

  it('getOverview: 失败响应原样透传，不做归一', async () => {
    getMock.mockResolvedValue({ success: false, error: '接口挂了' })
    const res = await useBoardRadarApi().getOverview()
    expect(res).toEqual({ success: false, error: '接口挂了' })
    expect(res.data).toBeUndefined()
  })
})

describe('useSemanticBoardsApi - 标签辅助标签全集（归档层级视图）', () => {
  beforeEach(() => {
    getMock.mockReset()
    postMock.mockReset()
    vi.restoreAllMocks()
  })

  it('getTagAuxiliaryLabels → GET /tags/:id/auxiliary-labels', async () => {
    const expected = { success: true, data: { items: [{ id: 31, label: '沙特' }], total: 1 } }
    getMock.mockResolvedValue(expected)
    const api = useSemanticBoardsApi()
    const res = await api.getTagAuxiliaryLabels(238319)
    expect(getMock).toHaveBeenCalledWith('/tags/238319/auxiliary-labels')
    expect(res).toEqual(expected)
  })
})
