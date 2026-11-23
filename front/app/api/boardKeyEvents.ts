/**
 * 版块级关键事件批量读 API（add-key-event-matrix-view 2.1，design D3）。
 *
 * - GET /semantic-boards/:id/key-events?days=N：一次取全观察窗内全部 confirmed
 *   注记（扁平列表，前端按 topicId + date 聚合）；只读、无写动作。
 * - days 严格正整数（默认 14；0/负数/非数字 → 400），与话题级读端
 *   days=0=全部 的旧契约不同（delta spec「非法参数」Scenario）。
 * - 无匹配事件返回成功 + 空数组（MUST NOT 报错、MUST NOT 用建议态补足）。
 */
import { apiClient } from './client'
import type { ApiResponse } from '~/types'
import { normalizeBoardKeyEvents, type BoardKeyEventItem, type RawBoardKeyEventMarker } from './normalizers/boardKeyEvents'

export interface BoardKeyEventsPayload {
  events: BoardKeyEventItem[]
}

export type { BoardKeyEventItem, RawBoardKeyEventMarker } from './normalizers/boardKeyEvents'

function mapBoardKeyEventsResponse(res: ApiResponse<{ markers: RawBoardKeyEventMarker[] }>): ApiResponse<BoardKeyEventsPayload> | { success: false, error?: string, message?: string, status?: number } {
  if (res.success && res.data) {
    return { success: true, data: { events: normalizeBoardKeyEvents(res.data.markers) } }
  }
  return { success: false, error: res.error, message: res.message, status: res.status }
}

export function useBoardKeyEventsApi() {
  /** 版块级关键事件取数（只读）。days 必须为正整数（矩阵观察窗档）。 */
  async function getBoardKeyEvents(boardId: number, days: number): Promise<ReturnType<typeof mapBoardKeyEventsResponse>> {
    return mapBoardKeyEventsResponse(
      await apiClient.get<{ markers: RawBoardKeyEventMarker[] }>(`/semantic-boards/${boardId}/key-events?days=${days}`),
    )
  }
  return { getBoardKeyEvents }
}
