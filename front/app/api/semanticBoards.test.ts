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

import { useSemanticBoardsApi } from './semanticBoards'

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

describe('useSemanticBoardsApi - topic landscape', () => {
  beforeEach(() => {
    getMock.mockReset()
    vi.restoreAllMocks()
  })

  it('getTopicLandscape: no days → GET without query', async () => {
    const expected = { success: true, data: { topics: [], vitality: { days: 30, article_count: 0, section_count: 0, active_topic_count: 0, feed_active: null, trend: [] } } }
    getMock.mockResolvedValue(expected)
    const api = useSemanticBoardsApi()
    const res = await api.getTopicLandscape(12)
    expect(getMock).toHaveBeenCalledWith('/semantic-boards/12/topic-landscape')
    expect(res).toEqual(expected)
  })

  it('getTopicLandscape: with days → GET ?days=N', async () => {
    getMock.mockResolvedValue({ success: true, data: { topics: [], vitality: { days: 30, article_count: 0, section_count: 0, active_topic_count: 0, feed_active: null, trend: [] } } })
    const api = useSemanticBoardsApi()
    await api.getTopicLandscape(12, 30)
    expect(getMock).toHaveBeenCalledWith('/semantic-boards/12/topic-landscape?days=30')
  })
})
