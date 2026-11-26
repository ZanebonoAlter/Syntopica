/**
 * useLaneDayReading — 当日线索阅读状态（rework-workshop-into-radar-and-desk 矩阵阶段 G4 + 8C G1）。
 *
 * 本文件是**组合根**：创建一次板块级读取状态并委托给四个内聚 helper（8C.4S 结构规范包按
 * code-style §8 拆分，公共导出/返回键/时序不变）：
 *  - `laneDayReportIndex`：报日索引请求/覆盖/请求序/陈旧状态（唯一索引状态所有者）；
 *  - `laneDayCurrentReading`：编年当前日读取（laneSeq + 上下文隔离 + stale 保留）；
 *  - `laneDayReportDay`：报告优先读取（reportSeq + date display gate）；
 *  - `laneDayRelatedArticles`：相关文章条目状态（跨板块晚到不发布）。
 * 本文件自身只持有：fetcher 注入、板块代际（`boardEpoch`，所有晚到判定的统一失效源）、
 * 两份有界缓存（详情 ≤14 / 文章 ≤20，简单 Map FIFO）、切板块与卸载的统一复位。
 *
 * 阶段合同（§3.1「当日编年」+ §2.2 P4 有界缓存 + 8C 报日索引）主干：
 *  - 本板块读缓存：报告列表索引仅 1 份（lookback 随停更差额扩大后整体替换，见
 *    api/laneDayReading.reportIndexDaysFor——锚定今天的列表端点与锚定 anchor 的 lane 池
 *    不同源，死写 365 会漏池首报告）；报告详情 ≤14、文章详情 ≤20。
 *  - **双 epoch**：lane 读取与报告优先读取各持序号，互不取消；切板块/卸载统一复位与清缓存，
 *    晚到响应按 epoch/板块/缓存失效代丢弃。
 *  - 显示 gate：`current` 带 laneKey/date/sectionKey、`reportDay` 带 date，消费方渲染前必须与
 *    自身上下文比对；跨上下文先清显示，同上下文刷新失败才 stale 保留旧内容。
 *  - 失败不入成功缓存；不伪造摘要、不借别日、不触发生成/回填；加载标题不标已读、不动
 *    reading_behaviors。
 *
 * 不拥有：sourceDays/观察范围/选中泳道/选中日期（G5 统一持有；本 composable 只跟随组件
 * 传入的 date/day 受控读取，上一/下一有内容日由组件 emit 日期）。当前读对象独立持有
 * （current 强引用完整模型），不因 FIFO 逐出消失/回顶（CA-8）。
 */
import { getCurrentScope, onScopeDispose, watch, type Ref } from 'vue'
import { useLaneDayReadingApi } from '~/api/laneDayReading'
import type { LaneDayArticleMeta, LaneDayReportDetail, LaneDayReportIndexItem } from '~/api/normalizers/laneDayReading'
import { createLaneDayCurrentReading } from './laneDayCurrentReading'
import { createLaneDayRelatedArticles } from './laneDayRelatedArticles'
import { createLaneDayReportDay } from './laneDayReportDay'
import { createLaneDayReportIndex } from './laneDayReportIndex'
import { createBoundedRequestCache } from './laneDayReadingCache'

/** 日报详情缓存上限（CA-3/4/6：14 份 FIFO、失败不占位）。 */
export const REPORT_DETAIL_CACHE_LIMIT = 14
/** 相关文章缓存上限（CA-5：20 份 FIFO、命中不请求）。 */
export const ARTICLE_CACHE_LIMIT = 20

export interface LaneDayReadingFetchers {
  list: (boardId: number, days: number) => Promise<{ success: boolean, data?: LaneDayReportIndexItem[], error?: string }>
  detail: (reportId: string) => Promise<{ success: boolean, data?: LaneDayReportDetail, error?: string }>
  article: (articleId: string) => Promise<{ success: boolean, data?: LaneDayArticleMeta, error?: string }>
}

export interface UseLaneDayReadingDeps {
  boardId: Ref<number>
  /** 今天（YYYY-MM-DD，Asia/Shanghai；报告列表「今日锚」差额计算用）。 */
  today: () => string
  /** 板块数据锚点（YYYY-MM-DD | null；池首 = anchor-(POOL_DAYS-1)，null 回退目标日）。 */
  anchorDate: () => string | null
  fetchers?: LaneDayReadingFetchers
}

export type {
  LaneDayCurrentReading,
  LaneDayReadingContext,
} from './laneDayCurrentReading'
export type {
  LaneReportDay,
  ReportDayErrorSource,
  ReportDayStatus,
} from './laneDayReportDay'
export type { ReportIndexState } from './laneDayReportIndex'
export type { RelatedArticleEntry } from './laneDayRelatedArticles'

export function useLaneDayReading(deps: UseLaneDayReadingDeps) {
  const defaultApi = useLaneDayReadingApi()
  const fetchers: LaneDayReadingFetchers = deps.fetchers ?? {
    list: defaultApi.loadReportIndex,
    detail: defaultApi.loadReportDetail,
    article: defaultApi.loadArticleMeta,
  }

  /** 板块代际：晚到的索引/详情/文章响应即使在 resetForBoard 之后也不回填状态。 */
  let boardEpoch = 0

  const reportCache = createBoundedRequestCache<string, LaneDayReportDetail>(REPORT_DETAIL_CACHE_LIMIT, async (reportId) => {
    const res = await fetchers.detail(reportId)
    if (!res.success || !res.data) throw new Error(res.error || '日报详情加载失败')
    return res.data
  })
  const articleCache = createBoundedRequestCache<string, LaneDayArticleMeta>(ARTICLE_CACHE_LIMIT, async (articleId) => {
    const res = await fetchers.article(articleId)
    if (!res.success || !res.data) throw new Error(res.error || `文章 #${articleId} 加载失败`)
    return res.data
  })

  const index = createLaneDayReportIndex({
    boardId: deps.boardId,
    today: deps.today,
    anchorDate: deps.anchorDate,
    list: fetchers.list,
    boardEpoch: () => boardEpoch,
  })
  const currentReading = createLaneDayCurrentReading({
    boardId: () => deps.boardId.value,
    loadIndex: index.loadIndex,
    requiredIndexDays: index.requiredIndexDays,
    detailCache: reportCache,
  })
  const reportDay = createLaneDayReportDay({
    boardId: () => deps.boardId.value,
    loadIndex: index.loadIndex,
    requiredIndexDays: index.requiredIndexDays,
    detailCache: reportCache,
  })
  const related = createLaneDayRelatedArticles({
    boardId: () => deps.boardId.value,
    boardEpoch: () => boardEpoch,
    articleCache,
  })

  // ── 生命周期：切板块失效 / 卸载释放（CA-7） ────────────────────────────────

  /** 统一复位：递增板块代际 → 各读取器清自身状态 → 清缓存（顺序即失效序，勿拆散）。 */
  function resetForBoard(): void {
    boardEpoch += 1
    index.reset()
    currentReading.reset()
    reportDay.reset()
    related.reset()
    reportCache.clear()
  }

  watch(deps.boardId, () => {
    resetForBoard()
  })

  if (getCurrentScope()) {
    onScopeDispose(() => {
      resetForBoard()
    })
  }

  return {
    // 当前日读取（渲染入口；消费方必须按 laneKey/date/sectionKey 与自身上下文比对后再用，见 LaneChronicle display gate）
    current: currentReading.current,
    loading: currentReading.loading,
    error: currentReading.error,
    staleNotice: currentReading.staleNotice,
    indexLoading: index.indexLoading,
    ensureDayReading: currentReading.ensureDayReading,
    suspendDisplay: currentReading.suspendDisplay,
    // 报日索引（日期轴/窄屏报日条/报告优先读取共用 1 份）
    reportDates: index.reportDates,
    indexError: index.indexError,
    ensureReportIndex: index.ensureReportIndex,
    // 报告优先读取（报告日模型必须按 date 与调用方所选日比对后再渲染）
    reportDay: reportDay.reportDay,
    reportStale: reportDay.reportStale,
    ensureReportDay: reportDay.ensureReportDay,
    // 相关文章（按需）
    relatedEntries: related.relatedEntries,
    readRelated: related.readRelated,
    ensureRelatedArticles: related.ensureRelatedArticles,
    retryRelatedArticle: related.retryRelatedArticle,
    getArticlePayload: related.getArticlePayload,
    // 生命周期/诊断
    resetForBoard,
    reportCacheCount: () => reportCache.size(),
    articleCacheCount: related.cacheSize,
    indexDays: index.indexDays,
  }
}

export type LaneDayReadingStore = ReturnType<typeof useLaneDayReading>
