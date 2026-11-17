/**
 * 当日线索阅读数据 normalizer（rework-workshop-into-radar-and-desk 矩阵阶段 G4）。
 *
 * 既有日报/文章 API（api/dailyReports.ts、api/articles.ts）的 snake_case 协议**完全不改**；
 * 仅在本域读取面（useLaneDayReading → LaneChronicle）客户端边界归一为 camelCase 读模型
 * 并把数字 ID 字符串化（code-style §9：转换只在 API 层，组件/状态层不散落 raw 映射）。
 *
 * 稳定 ID 链（workshop-board-console「当日线索工作面」/ board-lane-dynamics「稳定 ID 关联」）：
 *   报告列表 period_date → reportId → 报告详情 sections[].id（= lane-dynamics section_id）
 *   → threads[].id → related_article_ids → 文章详情。
 * join 按上述稳定 ID 精确匹配；MUST NOT 按标题文本匹配或跨报告借用（红线）。
 */
import type { DailyReport, DailyReportListItem, DailyReportSection, DailyReportThread } from '../dailyReports'
import { normalizeArticle, type ArticlePayload } from './article'
import type { Article } from '~/types'
import type { WindowLaneDay } from './laneDynamicsWindow'

/** 报告列表索引条目（当前板块 1 份索引内的行）。 */
export interface LaneDayReportIndexItem {
  reportId: string
  periodDate: string
  title: string
  createdAt: string
}

/** 报告详情内的一条线索（thread；summary 逐字，同名不同 threadId 各自保留）。 */
export interface LaneDayThread {
  threadId: string
  sectionId: string
  title: string
  /** thread.summary 原文（空串 = 无摘要记录，合法态）。 */
  summary: string
  confidence: number
  relatedArticleIds: string[]
  fitDistance: number | null
  /** thread 范围：'composite' = 综合稿（不计入线索数）；缺省 = 未知（旧响应）。 */
  scope: string | null
  createdAt: string
}

/** 报告详情内的一个主题桶（section）。 */
export interface LaneDayReportSection {
  sectionId: string
  /** 当日报告 `cluster_index` 原值（行序按它升序）；旧 fixture/未归一数据缺省 = 未知，消费方按 null 处理。 */
  clusterIndex?: number | null
  clusterLabel: string
  /** 泳道内主题分桶方向名（lane_tier 桶标题）；历史 section 为 null。 */
  bucketTitle: string | null
  /** 生成时的话题状态快照**原值**（active | candidate | archived | null）；不猜、不改写历史，
   *  未知/大小写异常由纯模型（dayOtherTopics）降级为「当时状态未记录」。缺省同 null。 */
  topicStatusAtReport?: string | null
  /** 生成时归属的持久话题（字符串 ID）；null/缺省 = 当次未归属（旧数据缺字段）。 */
  persistentTopicId?: string | null
  threads: LaneDayThread[]
}

/** 报告详情读模型（join 输入）。 */
export interface LaneDayReportDetail {
  reportId: string
  boardId: string
  periodDate: string
  sections: LaneDayReportSection[]
}

function normalizeThread(thread: DailyReportThread): LaneDayThread {
  return {
    threadId: String(thread.id),
    sectionId: String(thread.section_id),
    title: thread.title ?? '',
    summary: thread.summary ?? '',
    confidence: thread.confidence ?? 0,
    relatedArticleIds: (thread.related_article_ids ?? []).map(id => String(id)),
    scope: typeof thread.scope === 'string' && thread.scope !== '' ? thread.scope : null,
    fitDistance: typeof thread.fit_distance === 'number' ? thread.fit_distance : null,
    createdAt: thread.created_at ?? '',
  }
}

function normalizeReportSection(section: DailyReportSection): LaneDayReportSection {
  return {
    sectionId: String(section.id),
    clusterIndex: typeof section.cluster_index === 'number' ? section.cluster_index : null,
    clusterLabel: section.cluster_label ?? '',
    bucketTitle: section.bucket_title ?? null,
    topicStatusAtReport: typeof section.topic_status_at_report === 'string' ? section.topic_status_at_report : null,
    persistentTopicId: section.persistent_topic_id == null ? null : String(section.persistent_topic_id),
    threads: (section.threads ?? []).map(normalizeThread),
  }
}

export function normalizeReportIndexItem(item: DailyReportListItem): LaneDayReportIndexItem {
  return {
    reportId: String(item.id),
    periodDate: item.period_date,
    title: item.title ?? '',
    createdAt: item.created_at ?? '',
  }
}

export function normalizeReportDetail(report: DailyReport): LaneDayReportDetail {
  return {
    reportId: String(report.id),
    boardId: String(report.semantic_board_id),
    periodDate: report.period_date,
    sections: (report.sections ?? []).map(normalizeReportSection),
  }
}

/** 相关文章元信息（真实文章详情的字段子集；不造来源名——/articles/:id 只带 feed_id）。 */
export interface LaneDayArticleMeta {
  articleId: string
  title: string
  pubDate: string
  read: boolean
  /** G5 站内阅读复用的完整归一 payload（同一次读取，不二次请求）。 */
  article: Article
}

export function normalizeArticleMeta(payload: ArticlePayload): LaneDayArticleMeta {
  const article = normalizeArticle(payload)
  return {
    articleId: article.id,
    title: article.title || '(无标题)',
    pubDate: article.pubDate,
    read: article.read ?? false,
    article,
  }
}

// ── 当日 join（纯函数：base 日内容 × 报告详情 → 编年视图模型） ───────────────

/** 单桶 join 结果：matched = 详情命中该 section_id（全量 threads）；否则降级 base 标题。 */
export interface LaneDaySectionView {
  sectionId: string
  label: string
  matched: boolean
  bucketTitle: string | null
  /** matched 时为该桶全量线索（含 0 条，合法）；未命中为空。 */
  threads: LaneDayThread[]
  /** base（lane-dynamics 聚合）已返回的 ≤N 条标题；仅未命中时用于降级展示。 */
  fallbackTitles: string[]
  foldedCount: number
}

/** 当日阅读整体状态（列表/详情失败与「无报告」「分区缺失」分层，见 useLaneDayReading）。 */
export type LaneDayJoinStatus = 'loading' | 'ready' | 'degraded' | 'no-report' | 'error'

export interface LaneDayReadingModel {
  status: LaneDayJoinStatus
  /** status='error' 时的错误文案（列表/详情失败各带来源）。 */
  error: string | null
  reportId: string | null
  sections: LaneDaySectionView[]
  /** 降级桶数（未命中的 base 桶）。 */
  missingSectionCount: number
  /** 降级桶被后端截断未载入的标题数合计（「另有 K 条折叠未读取」）。 */
  foldedTotal: number
}

/** join：请求桶顺序保持 base 日序；详情按 sectionId 精确命中（缺 thread_count 的旧桶不参与判断）。 */
export function buildLaneDaySections(
  day: WindowLaneDay,
  detail: LaneDayReportDetail | null,
): LaneDaySectionView[] {
  const detailBySectionId = new Map(detail?.sections.map(section => [section.sectionId, section]) ?? [])
  return day.sections.map((base) => {
    const matched = detailBySectionId.get(base.sectionId)
    return {
      sectionId: base.sectionId,
      label: base.label,
      matched: matched != null,
      bucketTitle: matched?.bucketTitle ?? null,
      threads: matched?.threads ?? [],
      fallbackTitles: base.eventTitles,
      foldedCount: base.foldedCount,
    }
  })
}

/**
 * 组装整体读模型（纯函数）：
 *  - `detail` 提供时自动推导 status（全部命中 = ready；≥1 桶未命中 = degraded）；
 *  - 无 detail（no-report/error/loading 路径）传显式 status，sections 仍给 base 降级视图，
 *    不借别日、不伪造摘要。
 */
export function buildLaneDayJoinedModel(
  day: WindowLaneDay,
  options: { reportId: string, detail: LaneDayReportDetail },
): LaneDayReadingModel {
  const sections = buildLaneDaySections(day, options.detail)
  const counts = countDegraded(sections)
  return {
    status: counts.missingSectionCount === 0 ? 'ready' : 'degraded',
    error: null,
    reportId: options.reportId,
    sections,
    ...counts,
  }
}

export function buildLaneDayFallbackModel(
  day: WindowLaneDay,
  options: { status: 'no-report' | 'error' | 'loading', error?: string | null, reportId?: string | null },
): LaneDayReadingModel {
  const sections = buildLaneDaySections(day, null)
  return {
    status: options.status,
    error: options.error ?? null,
    reportId: options.reportId ?? null,
    sections,
    ...countDegraded(sections),
  }
}

function countDegraded(sections: LaneDaySectionView[]): { missingSectionCount: number, foldedTotal: number } {
  let missingSectionCount = 0
  let foldedTotal = 0
  for (const section of sections) {
    if (!section.matched) {
      missingSectionCount += 1
      foldedTotal += section.foldedCount
    }
  }
  return { missingSectionCount, foldedTotal }
}
