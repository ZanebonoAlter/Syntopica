/**
 * 当日线索阅读 API façade（rework-workshop-into-radar-and-desk 矩阵阶段 G4）。
 *
 * 只读消费既有日报列表/详情与文章详情端点（后端零改动、零新参数、零新接口）：
 *  - GET /semantic-boards/:id/daily-reports?days=N（api/dailyReports.getBoardDailyReports）
 *  - GET /daily-reports/:id（api/dailyReports.getDailyReportDetail）
 *  - GET /articles/:id（api/articles.getArticle）
 * 在本域边界归一为 camelCase/字符串 ID 读模型；失败错误体逐字段透传，不改 shared
 * ApiResponse、不改既有共享 API 与旧消费者。
 *
 * **报告索引 lookback 覆盖（关键新核实，G4 brief）**：ListReports 以
 * `NormalizeReportDate(time.Now())` 为锚（今日、服务器日历日）回溯 days-1 天（含端点），
 * days<=0 默认 7、**无上限**；而 lane-dynamics 365 池按板块 MAX(period_date)（anchor）
 * 回溯。停更板块 anchor 早于今天时，死写 days=365 会漏掉池最早几日的真实报告。
 * `reportIndexDaysFor` 按 UTC+8 实际日历日与 anchor 差额计算足够 lookback：
 * days = max(365, daysBetweenInclusive(requiredStart, today))，覆盖池首/目标日下界。
 * 不新增 from-to 参数、不把 G1 的 7/14/30/365 四档校验套到报告列表。
 */
import { useDailyReportsApi, type DailyReport, type DailyReportListItem } from './dailyReports'
import { useArticlesApi } from './articles'
import {
  normalizeArticleMeta,
  normalizeReportDetail,
  normalizeReportIndexItem,
  type LaneDayArticleMeta,
  type LaneDayReportDetail,
  type LaneDayReportIndexItem,
} from './normalizers/laneDayReading'
import type { ApiResponse, Article } from '~/types'
import type { ArticlePayload } from './normalizers/article'

const DAY_MS = 86_400_000
/** 池宽下限：lane-dynamics 365 池同宽（POOL_DAYS 口径；本地复算不引 feature 层）。 */
export const REPORT_INDEX_MIN_DAYS = 365

function dateIndex(date: string): number {
  return Math.round(Date.parse(`${date}T00:00:00Z`) / DAY_MS)
}

/** 含两端天数（end-start+1）；与 features observationRange.daysBetweenInclusive 同口径（api 层不自引 feature）。 */
function daysBetweenInclusive(start: string, end: string): number {
  return dateIndex(end) - dateIndex(start) + 1
}

/**
 * 报告列表 days 参数：覆盖 requiredStart（池首 = anchor-364 或目标日更早者）到今天的
 * 全部日历日；365 下限保证与池同宽。requiredStart 晚于 today（异常输入）回退下限。
 */
export function reportIndexDaysFor(requiredStart: string, today: string): number {
  const span = daysBetweenInclusive(requiredStart, today)
  if (!Number.isFinite(span) || span < 1) return REPORT_INDEX_MIN_DAYS
  return Math.max(REPORT_INDEX_MIN_DAYS, span)
}

export type LaneDayReadingResponse<T>
  = { success: true, data: T, message?: string }
  | { success: false, message?: string, error?: string, status?: number }

function mapListResponse(res: ApiResponse<{ reports: DailyReportListItem[] }>): LaneDayReadingResponse<LaneDayReportIndexItem[]> {
  if (res.success && res.data) {
    return { success: true, data: (res.data.reports ?? []).map(normalizeReportIndexItem), message: res.message }
  }
  return { success: false, error: res.error, message: res.message, status: res.status }
}

function mapDetailResponse(res: ApiResponse<{ report: DailyReport }>): LaneDayReadingResponse<LaneDayReportDetail> {
  if (res.success && res.data?.report) {
    return { success: true, data: normalizeReportDetail(res.data.report), message: res.message }
  }
  return { success: false, error: res.error, message: res.message, status: res.status }
}

function mapArticleResponse(res: ApiResponse<Article>): LaneDayReadingResponse<LaneDayArticleMeta> {
  if (res.success && res.data) {
    return { success: true, data: normalizeArticleMeta(res.data as unknown as ArticlePayload), message: res.message }
  }
  return { success: false, error: res.error, message: res.message, status: res.status }
}

export function useLaneDayReadingApi() {
  const reportsApi = useDailyReportsApi()
  const articlesApi = useArticlesApi()

  /** 报告列表索引（只读）：days 见 reportIndexDaysFor，端点自身无上限。 */
  async function loadReportIndex(boardId: number, days: number): Promise<LaneDayReadingResponse<LaneDayReportIndexItem[]>> {
    return mapListResponse(await reportsApi.getBoardDailyReports(boardId, { days }))
  }

  /** 报告详情（只读）：全量 sections/threads（解决聚合前 5 条截断）。 */
  async function loadReportDetail(reportId: string): Promise<LaneDayReadingResponse<LaneDayReportDetail>> {
    return mapDetailResponse(await reportsApi.getDailyReportDetail(Number(reportId)))
  }

  /** 文章详情（只读）：相关文章元信息 + G5 复用 payload；数字转换收在本边界。 */
  async function loadArticleMeta(articleId: string): Promise<LaneDayReadingResponse<LaneDayArticleMeta>> {
    return mapArticleResponse(await articlesApi.getArticle(Number(articleId)))
  }

  return { loadReportIndex, loadReportDetail, loadArticleMeta }
}
