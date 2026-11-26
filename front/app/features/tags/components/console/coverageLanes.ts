/**
 * 覆盖组派生（change `lane-matrix-coverage-and-others-row` 4.1）：**窗口内未进日报**的活跃泳道。
 *
 * 根因：`lane-dynamics` 只为「窗口内有锚定 section」的话题生成行（沉寂不展示），
 * 于是「新建但当前窗口没分区」的活跃泳道在矩阵里整行缺席（连行头 ⋯ 都点不到）。
 * 本模块用**既有** `GET /semantic-boards/:id/topics` 清单补齐这类泳道：
 * `status === 'active'` 且不在 A 档名单（rows）里的 persistent topic。
 *
 * 口径边界（spec）：
 *  - 不进 A 档排序 / 休眠判定 / 右轨名单（这里只产出矩阵底部独立组的行）；
 *  - `last_seen_date` 缺失如实写 null（展示层写「无记录」），不拿今天冒充；
 *  - 不造数、不改话题状态机。
 */
import type { BoardTopicListItem } from '~/api/dailyReports'
import { compareTopicId, type ObservationLaneRow } from './observationLanes'

/** 覆盖组单行（矩阵底部「窗口内未进日报」组）。 */
export interface CoverageLaneRow {
  /** 话题 ID（字符串，与矩阵行 `topicId` 同一形态）。 */
  topicId: string
  /** 泳道名（topics 清单原样）。 */
  label: string
  /** 最近一次出现日期（YYYY-MM-DD）；null = 清单里没有该日期（缺失即无记录）。 */
  lastSeenDate: string | null
}

/** `last_seen_date` 归一为 date-only：`/semantic-boards/:id/topics` 的 Go 时间字段
 *  实测是 RFC3339（如 `2026-11-09T00:00:00Z`），直接展示会把时分秒带进矩阵。
 *  非日期形态（未知/缺省）→ null（展示层写「无记录」，不造日期）。 */
function normalizeDateOnly(raw: string | null | undefined): string | null {
  const value = (raw ?? '').trim()
  return /^\d{4}-\d{2}-\d{2}/.test(value) ? value.slice(0, 10) : null
}

/**
 * 覆盖组行 = 活跃话题 − A 档名单行。
 *
 * 排序：最近一次出现日期**降序**（最近有动静的排前面；无记录的沉底）→
 * 泳道名升序 → 话题 ID 升序（数字 ID 按数值，避免 10 排在 9 之后）。
 */
export function buildCoverageLanes(
  rows: readonly ObservationLaneRow[],
  topics: readonly BoardTopicListItem[],
): CoverageLaneRow[] {
  const present = new Set(rows.map(row => row.topicId))
  const lanes: CoverageLaneRow[] = []
  for (const topic of topics) {
    if (topic.status !== 'active') continue
    const topicId = String(topic.id)
    if (present.has(topicId)) continue
    lanes.push({
      topicId,
      label: topic.label,
      lastSeenDate: normalizeDateOnly(topic.last_seen_date),
    })
  }
  return lanes.sort((a, b) => {
    if (a.lastSeenDate !== b.lastSeenDate) {
      // 无记录（null）沉底：有日期的按降序，无日期的排最后。
      if (a.lastSeenDate === null) return 1
      if (b.lastSeenDate === null) return -1
      return a.lastSeenDate < b.lastSeenDate ? 1 : -1
    }
    if (a.label !== b.label) return a.label < b.label ? -1 : 1
    return compareTopicId(a.topicId, b.topicId)
  })
}
