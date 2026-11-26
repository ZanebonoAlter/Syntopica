/**
 * 同归属跨日观察（section-timeline）只读 API 边界（rework-workshop-into-radar-and-desk 矩阵阶段 8C G3）。
 *
 * 只读复用既有端点（后端零改动、零新参数、零新接口）：
 *   GET /semantic-boards/:id/section-timeline?days=N  → { sections, relations }
 * 在本边界归一为 camelCase/字符串 ID 读模型，**不改** `api/dailyReports.ts` 的既有类型与消费者。
 *
 * 覆盖承诺（按后端 handler/仓储实核，不靠返回记录的最小/最大日期反推）：
 *  - `days <= 0` ⇒ 仓储以 100000 天窗口查询 = **板块全部历史**（处理器显式把 `days=0` 透传为 <=0）；
 *  - `days > 90` ⇒ 后端**静默钳到 90 天**，窗口锚定该板块 `MAX(period_date)`（completed）；
 *  - 省略 days ⇒ 处理器默认 0（等同全部历史）。
 * 观察带需要覆盖现有 365 日观察池，正数 days 会被钳到 90 天而**静默少覆盖**，因此产品路径固定
 * 请求 `SECTION_TIMELINE_ALL_DAYS = 0`（全部历史），覆盖信息只描述请求口径（`'all'`），绝不按
 * 返回记录日期反推「已覆盖」或把窗外记录当成 0。
 *
 * 历史口径（红线）：section-timeline 节点自带 `period_date` / `cluster_label` / `thread_count` /
 * `persistent_topic_id` 是**生成时**的真实分区记录；节点附带的 nested `persistent_topic`
 * （label/status/hit_count/can_activate）是**当前**话题属性——本边界刻意不映射，避免把当前资格
 * 冒成历史快照。
 *
 * G3 收口（M-API）：产品读取**不开放窗口 days**（签名只有 `boardId`，恒定请求 0=全部历史），
 * 调用方无法传 7 天窗口却拿到 `coverage:'all'` 的少覆盖数据；返回值直接用统一 `ApiResponse<T>`
 * 形状（alias，不另造一套包装），不改 `api/dailyReports.ts` 的既有类型与消费者。
 */
import { useDailyReportsApi } from './dailyReports'
import type { ApiResponse } from '~/types'

/** 端点「全部历史」days 值（<=0 ⇒ 不按天过滤）。产品观察固定使用它覆盖 365 日池。 */
export const SECTION_TIMELINE_ALL_DAYS = 0

/** section-timeline 单节点的最小运行时形状（只取观察需要的字段；其余字段忽略）。 */
export interface SectionTimelinePayloadNode {
  id?: number | string | null
  report_id?: number | string | null
  /** 后端 `time.Time` 序列化为 RFC3339（如 `2026-10-22T00:00:00Z`）；也接受纯日期串。 */
  period_date?: string | null
  cluster_label?: string | null
  /** 分区 thread 数；缺失/非法 = 未知（null），不补 0。 */
  thread_count?: number | null
  /** 生成时归属的持久话题 ID；null/缺省 = 当次未归属。 */
  persistent_topic_id?: number | string | null
  /** 端点附带的**当前**话题 brief——刻意不映射（不是历史快照）。 */
  persistent_topic?: unknown
}

export interface SectionTimelinePayload {
  sections?: readonly SectionTimelinePayloadNode[] | null
  relations?: readonly unknown[] | null
}

/** 观察记录日模型（单条历史分区；同日同归属多分区各自一条，保持接口原序）。 */
export interface ObservationSectionRecord {
  sectionId: string
  reportId: string
  /** 归一后的 `YYYY-MM-DD`（空串记录已在前置过滤掉）。 */
  date: string
  /** 生成时的分区原题（历史快照，逐字；不取 nested 当前话题 label）。 */
  clusterLabel: string
  /** 分区 thread 数；null = 未知（任一缺失日峰值即未知）。 */
  threadCount: number | null
  /** 生成时归属 ID（字符串）；null = 当次未归属（无可靠跨日身份）。 */
  persistentTopicId: string | null
}

export interface ObservationTimelineData {
  records: ObservationSectionRecord[]
  /** 覆盖口径：'all' = 本次请求就是板块全部历史（不按记录反推覆盖）。 */
  coverage: 'all'
}

export type ObservationResponse<T> = ApiResponse<T>

function normalizeId(value: number | string | null | undefined): string {
  if (value === null || value === undefined) return ''
  return String(value)
}

function normalizePeriodDate(value: string | null | undefined): string {
  if (typeof value !== 'string') return ''
  return value.slice(0, 10)
}

function normalizeThreadCount(value: number | null | undefined): number | null {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) return null
  return value
}

/**
 * 归一 section-timeline：数字 ID → 字符串、snake_case → camelCase、日期取日历日。
 * 缺稳定分区 ID 或日期的记录被排除（无法定位记录日），其余原序保留。
 */
export function normalizeSectionTimeline(payload: SectionTimelinePayload | null | undefined): ObservationTimelineData {
  const nodes = payload?.sections ?? []
  const records: ObservationSectionRecord[] = []
  for (const node of nodes) {
    const sectionId = normalizeId(node.id)
    const date = normalizePeriodDate(node.period_date)
    if (sectionId === '' || date === '') continue
    records.push({
      sectionId,
      reportId: normalizeId(node.report_id),
      date,
      clusterLabel: node.cluster_label ?? '',
      threadCount: normalizeThreadCount(node.thread_count),
      persistentTopicId: node.persistent_topic_id == null ? null : String(node.persistent_topic_id),
    })
  }
  return { records, coverage: 'all' }
}

export function useOtherTopicObservationApi() {
  const reports = useDailyReportsApi()

  /**
   * 板块 section-timeline（只读）。恒定 `days=0`（全部历史）：正数会被后端静默钳到 90 天，
   * 覆盖不了 365 观察池；因此本签名**不接受**窗口 days（避免少覆盖却标 `coverage:'all'`）。
   */
  async function loadBoardSectionTimeline(
    boardId: number,
  ): Promise<ObservationResponse<ObservationTimelineData>> {
    const res: ApiResponse<SectionTimelinePayload> = await reports.getBoardSectionTimeline(boardId, SECTION_TIMELINE_ALL_DAYS)
    if (res.success && res.data) {
      return { success: true, data: normalizeSectionTimeline(res.data), message: res.message }
    }
    return { success: false, error: res.error, message: res.message, status: res.status }
  }

  return { loadBoardSectionTimeline }
}
