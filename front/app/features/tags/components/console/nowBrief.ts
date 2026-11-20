/**
 * 右轨「当日日报」读模型（console-rails-live-signals design D5/D6）。
 *
 * 纯函数：**不发任何请求**——输入是既有读取链的当日日报详情（报告优先读取 `reportDay`）
 * 与既有 lane-dynamics 泳道清单（矩阵日格同源）。两层表达（第三层「其他话题」由独立组件
 * `RailOtherTopicsBrief.vue` 承载，不在本模型派生）：
 *
 *  1. **点评层** = `report.highlights`（后端 LLM 生成的 2–3 条当日要闻，原序不截断）。
 *     MUST NOT 单独再显示 `report.title` / `report.summary`——后端 `FinalizeReportHighlights`
 *     写库时 `title = highlights[0].Title`、`summary = highlights[0].Reason`，重复即重复。
 *     `article_count` 缺省 → 不显示「N 篇来源」（不补 0；真实 0 是合法值，照实显示）。
 *  2. **泳道层** = 当日日报里归属**正式泳道**的分区：`topic_status_at_report === 'active'`
 *     且有 `persistent_topic`；句子取该分区首条 thread 的 `title`（无 thread 则退回
 *     `cluster_label`），两者皆无 → 该行不出。
 *     - **只保留能在 lane-dynamics 里按 topic_id 找到的泳道**：右栏每个「N 条」SHALL 在
 *       矩阵里有对应格，否则该行不进右栏。
 *     - **线索数取 lane-dynamics 该泳道在选中日的 `clue_count`**（与矩阵日格同源同值，
 *       绝不用含综合稿的日报分区线索数）；取不到 → `null`（显示 `—`，不补 0）。
 *     - **按线索数降序**（`null` 排末位）；并列保持日报 sections 原序（稳定排序）。
 *     - candidate 分区（「其他动态」）不进**泳道层**（不得与泳道行的「该泳道当日线索数」并排成
 *       同一列）；它们由右轨「当日日报」块内的独立子块「当日其他话题」承载
 *       （add-other-topics-to-context-rail：≥1440px 才显示，模型 = `dayOtherTopics.ts` 的
 *       `OtherTopicsView`，单位后缀「条线索」与泳道层的「条」分家）；矩阵下方「当日其他话题」
 *       折叠组仍是主交互面。
 */
import type { LaneDayReportHighlight, LaneDayReportSection } from '~/api/normalizers/laneDayReading'
import { dayCellMetrics, type WindowLane } from '~/api/normalizers/laneDynamicsWindow'

/** 右轨当日日报的读状态（与报告优先读取的 status 同形）。 */
export type NowBriefStatus = 'loading' | 'ready' | 'no-report' | 'error'

/** 一条当日点评（= 日报 highlights 的一条）。 */
export interface NowBriefReview {
  title: string
  reason: string
  /** 来源篇数；null = 缺省（不显示「N 篇来源」，不补 0）。 */
  articleCount: number | null
  /** 聚簇 id（可深链定位日报正文）；null = 不可定位。 */
  clusterId: number | null
}

/** 一条当日泳道（= 日报里归属该正式泳道的分区）。 */
export interface NowBriefLane {
  topicId: string
  label: string
  /** 泳道色点（话题 id 稳定哈希 → 既有主题色；不新增调色板）。 */
  color: string
  /** 当日线索合计（与矩阵日格同源同值）；null = 取不到（显示 `—`，不补 0）。 */
  clueCount: number | null
  /** 一句代表线索（日报 thread 标题；无则分区概述）。 */
  sentence: string
}

/** 「当日日报」块视图模型（组件只渲染）。 */
export interface NowBriefModel {
  /** 所跟日期（= 矩阵选中日；null = 还没选日）。 */
  date: string | null
  status: NowBriefStatus
  error: string | null
  /** 该日是否有日报（status='ready' 时为 true）。 */
  hasReport: boolean
  reviews: NowBriefReview[]
  lanes: NowBriefLane[]
}

/** 当日日报详情输入（报告优先读取发布的当日模型）。 */
export interface NowBriefReport {
  date: string
  status: NowBriefStatus
  error: string | null
  sections: LaneDayReportSection[]
  /** 当日重点（旧读取链不带该字段 = 未取到，不造重点）。 */
  highlights?: readonly LaneDayReportHighlight[]
}

function emptyModel(date: string | null, status: NowBriefStatus, error: string | null): NowBriefModel {
  return { date, status, error, hasReport: false, reviews: [], lanes: [] }
}

/** 点评层：highlights 原序（不截断条数）；标题与正文皆空的条目不占位。 */
function buildReviews(highlights: readonly LaneDayReportHighlight[]): NowBriefReview[] {
  const reviews: NowBriefReview[] = []
  for (const highlight of highlights) {
    const title = (highlight.title ?? '').trim()
    const reason = (highlight.reason ?? '').trim()
    if (!title && !reason) continue
    reviews.push({
      title,
      reason,
      articleCount: typeof highlight.articleCount === 'number' && Number.isFinite(highlight.articleCount)
        ? highlight.articleCount
        : null,
      clusterId: typeof highlight.clusterId === 'number' && Number.isFinite(highlight.clusterId)
        ? highlight.clusterId
        : null,
    })
  }
  return reviews
}

/**
 * 泳道色点：**优先用后端下发的稳定色**（`sections[].persistent_topic.color`，由后端
 * `PersistentTopicColor(topic_id)` 派生，同一话题在日报阅读台/侧栏/侦探墙都同色）。
 * 只有旧数据缺该字段时才回退到本地哈希——回退色仅保证「话题 → 颜色」稳定，不保证与全站一致，
 * 故命名与注释明确其为 fallback（review R-1）。
 */
const LANE_FALLBACK_COLORS = [
  'var(--color-accent)',
  'var(--color-secondary)',
  'var(--color-tertiary)',
  'var(--color-success)',
  'var(--color-info)',
]

/** 后端未下发色时的回退色（稳定但不保证与全站一致）；有后端色就 MUST NOT 走这里。 */
export function laneTopicColor(topicId: string): string {
  let hash = 0
  for (let index = 0; index < topicId.length; index++) {
    hash = (hash * 31 + topicId.charCodeAt(index)) >>> 0
  }
  return LANE_FALLBACK_COLORS[hash % LANE_FALLBACK_COLORS.length]!
}

/** 分区代表句：首条 thread 的标题；无 thread（或标题空）退回分区概述；皆无 → 空串。 */
function sectionSentence(section: LaneDayReportSection): string {
  const threadTitle = (section.threads[0]?.title ?? '').trim()
  if (threadTitle) return threadTitle
  return (section.clusterLabel ?? '').trim()
}

/** 该泳道在给定日期的当日线索合计（与矩阵日格同源同值）；取不到 → null。 */
function clueCountAt(lane: WindowLane, date: string | null): number | null {
  if (!date) return null
  const day = lane.timeline.find(item => item.date === date)
  if (!day) return null
  return dayCellMetrics(day).clueCount
}

/** 排序键：线索数降序（null 末位）；并列按日报原序（显式给次序键，不依赖引擎排序稳定性）。 */
function compareLanes(a: NowBriefLane, b: NowBriefLane, aOrder: number, bOrder: number): number {
  if (a.clueCount === null && b.clueCount === null) return aOrder - bOrder
  if (a.clueCount === null) return 1
  if (b.clueCount === null) return -1
  if (a.clueCount !== b.clueCount) return b.clueCount - a.clueCount
  return aOrder - bOrder
}

/** 泳道层：active 分区 ∩ lane-dynamics 正式泳道；按线索数降序。 */
function buildLanes(
  sections: readonly LaneDayReportSection[],
  date: string | null,
  lanes: readonly WindowLane[],
): NowBriefLane[] {
  const laneByTopic = new Map(lanes.map(lane => [lane.topicId, lane]))
  const picked: Array<{ lane: NowBriefLane, order: number }> = []
  const seen = new Set<string>()
  let order = 0
  for (const section of sections) {
    const topicId = section.persistentTopicId
    if (!topicId) continue
    if (section.topicStatusAtReport !== 'active') continue
    if (seen.has(topicId)) continue
    // 不在正式泳道名单里的分区不进右栏（保证右栏每个「N 条」在矩阵里有对应格）。
    const lane = laneByTopic.get(topicId)
    if (!lane) continue
    const sentence = sectionSentence(section)
    if (!sentence) continue
    seen.add(topicId)
    picked.push({
      lane: {
        topicId,
        label: lane.label,
        color: section.persistentTopicColor ?? laneTopicColor(topicId),
        clueCount: clueCountAt(lane, date),
        sentence,
      },
      order: order++,
    })
  }
  picked.sort((a, b) => compareLanes(a.lane, b.lane, a.order, b.order))
  return picked.map(item => item.lane)
}

/**
 * 组装「当日日报」视图模型（纯函数）：
 *  - `report` 为 null = 当日详情未到（loading）；status 分层直接透传（右轨局部降级）。
 *  - `selectedDate` 是**唯一日期源**（矩阵选中日）；详情日期与之不一致时按未到处理。
 */
export function buildNowBriefModel(
  report: NowBriefReport | null,
  selectedDate: string | null,
  laneDynamicsLanes: readonly WindowLane[],
): NowBriefModel {
  if (!report) return emptyModel(selectedDate, 'loading', null)
  const date = selectedDate ?? report.date
  /**
   * **日期闸门**（spec「当日日报跟随矩阵选中日」「选中日无日报」MUST NOT 显示邻日内容）：
   * `selectedDate` 是唯一日期源；详情日期与它不一致 = 该日详情还没到（可能仍握着上一日的那份），
   * 按未到处理，MUST NOT 把上一日日报挂到今日标题下。闸门放在纯函数里以便单测覆盖
   * （review R-4：此前只在 useConsoleRail 的 computed 里，无测试）。
   */
  if (selectedDate !== null && report.date !== selectedDate) {
    return emptyModel(selectedDate, 'loading', null)
  }
  if (report.status === 'loading') return emptyModel(date, 'loading', null)
  if (report.status === 'error') {
    return { ...emptyModel(date, 'error', report.error ?? '日报详情加载失败'), error: report.error ?? '日报详情加载失败' }
  }
  if (report.status === 'no-report') return emptyModel(date, 'no-report', null)
  return {
    date,
    status: 'ready',
    error: null,
    hasReport: true,
    reviews: buildReviews(report.highlights ?? []),
    lanes: buildLanes(report.sections, date, laneDynamicsLanes),
  }
}

/**
 * A 档 + 365 池按 topicId 合并（池只补 A 同 ID 的更早日期，与矩阵读链同源）。
 * 池里 A 档没有的 topic 不并入（矩阵名单基准始终是 A 档）。
 */
export function mergeLaneDynamicsLanes(
  tierLanes: readonly WindowLane[],
  poolLanes: readonly WindowLane[],
): WindowLane[] {
  if (poolLanes.length === 0) return [...tierLanes]
  const poolByTopic = new Map(poolLanes.map(lane => [lane.topicId, lane]))
  return tierLanes.map((lane) => {
    const pool = poolByTopic.get(lane.topicId)
    if (!pool) return lane
    const seen = new Set(lane.timeline.map(day => day.date))
    const extra = pool.timeline.filter(day => !seen.has(day.date))
    if (extra.length === 0) return lane
    return { ...lane, timeline: [...lane.timeline, ...extra] }
  })
}
