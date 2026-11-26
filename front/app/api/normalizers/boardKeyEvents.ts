/**
 * 版块级关键事件批量读 normalizer（add-key-event-matrix-view 2.1）。
 *
 * 后端 `GET /semantic-boards/:id/key-events?days=N`（design D3）返回窗口内全部
 * confirmed 注记的扁平列表（snake_case DTO）；仅在 API 边界归一为 camelCase 读模型：
 *  - 数字 ID 字符串化（code-style §9：组件/状态层不出现 raw 数字 ID）；
 *  - `source_period_date` 截为 date-only（后端 date 列以 UTC 零点序列化，slice 即日历日）；
 *  - `article_ids_snapshot` null → 空数组（「确认时未冻结文章」与「0 篇」同态渲染）。
 *
 * 端点只透 confirmed（后端契约），normalizer 不再按 state 过滤也不改写快照字段
 * （topic_label_snapshot 等确认时冻结，MUST NOT 反查实时名）。
 */

/** 后端 marker DTO（snake_case 透传层；只声明本视图消费的字段）。 */
export interface RawBoardKeyEventMarker {
  id: number
  persistent_topic_id?: number | null
  source_period_date: string
  topic_label_snapshot: string
  title_snapshot: string
  summary_snapshot: string
  article_ids_snapshot?: number[] | null
  state: string
}

/** 归一后的单条关键事件。 */
export interface BoardKeyEventItem {
  id: string
  /** 所属持久化话题（字符串 ID）；null = 无话题归属（不进话题索引，由视图「其他」兜底行承接）。 */
  topicId: string | null
  topicLabelSnapshot: string
  /** 来源日报日（date-only YYYY-MM-DD）。 */
  sourcePeriodDate: string
  titleSnapshot: string
  summarySnapshot: string
  /** 确认时冻结的关联文章 ID（字符串化；空数组 = 未冻结）。 */
  articleIds: string[]
}

function toDateOnly(raw: string): string {
  return typeof raw === 'string' ? raw.slice(0, 10) : ''
}

export function normalizeBoardKeyEvent(raw: RawBoardKeyEventMarker): BoardKeyEventItem {
  return {
    id: String(raw.id),
    topicId: raw.persistent_topic_id == null ? null : String(raw.persistent_topic_id),
    topicLabelSnapshot: raw.topic_label_snapshot ?? '',
    sourcePeriodDate: toDateOnly(raw.source_period_date),
    titleSnapshot: raw.title_snapshot ?? '',
    summarySnapshot: raw.summary_snapshot ?? '',
    articleIds: Array.isArray(raw.article_ids_snapshot)
      ? raw.article_ids_snapshot.map(id => String(id))
      : [],
  }
}

export function normalizeBoardKeyEvents(raws: readonly RawBoardKeyEventMarker[] | null | undefined): BoardKeyEventItem[] {
  if (!Array.isArray(raws)) return []
  return raws.map(normalizeBoardKeyEvent)
}
