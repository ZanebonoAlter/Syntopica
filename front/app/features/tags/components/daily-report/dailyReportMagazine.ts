import type {
  DailyReport,
  DailyReportHighlight,
  DailyReportSection,
  SectionRelation,
  SectionTimelineNode,
} from '~/api/dailyReports'

export type QualityZoneKey = 'active' | 'briefs'

export interface QualityZone {
  key: QualityZoneKey
  label: string
  eyebrow: string
  sections: DailyReportSection[]
}

export interface TopicGroup {
  key: string
  topicId?: number
  /** 当天组名（= 组内首个 section 的 cluster_label），做大标题，贴当日内容。 */
  label: string
  /** 规范名（= persistent_topic.label），做跨天锚点；无 persistent_topic 时为 undefined。 */
  canonicalLabel?: string
  color?: string
  sections: DailyReportSection[]
  articleCount: number
  threadCount: number
}

export interface LeadStory {
  title: string
  summary: string
  source: 'highlight' | 'section'
  /** 头条由 section 线索派生时的 section id（lead 批注无 thread 时锚定归属线索用；highlight 派生时无）。 */
  sectionId?: number
}

export interface LifelineDay {
  key: string
  date: string
  sections: SectionTimelineNode[]
}

export interface LifelineEdge {
  key: string
  fromSectionId: number
  toSectionId: number
  fromDayKey: string
  toDayKey: string
  /** 跨过空白自然日（日期跨度 > 1 天）的连线，渲染时弱化 */
  weak: boolean
}

export interface LifelineWindow {
  days: LifelineDay[]
  edges: LifelineEdge[]
}

export type RequestStatus = 'idle' | 'loading' | 'success' | 'error'

export interface RequestCacheEntry<T> {
  status: RequestStatus
  data?: T
  error?: string
}

export function sortDailyReportSections(sections: DailyReportSection[]): DailyReportSection[] {
  return [...sections].sort((a, b) => {
    // 物化板块（watch_*）排分区末尾：算法聚类是主叙事，关注物化是叠加板块。
    const aw = isWatchSection(a) ? 1 : 0
    const bw = isWatchSection(b) ? 1 : 0
    if (aw !== bw) return aw - bw
    if (a.best_tier !== b.best_tier) return a.best_tier - b.best_tier
    return b.avg_score - a.avg_score
  })
}

/** 关注物化板块（watch-materialized-topic）：lane_tier 前缀 watch_ 标记。 */
export function isWatchSection(section: DailyReportSection): boolean {
  return section.lane_tier === 'watch_keyword' || section.lane_tier === 'watch_sentence'
}

export function buildQualityZones(sections: DailyReportSection[]): QualityZone[] {
  const sorted = sortDailyReportSections(sections)
  const active = sorted.filter(section => section.topic_status_at_report === 'active')
  const briefs = sorted.filter(section => section.topic_status_at_report !== 'active')

  return [
    active.length ? { key: 'active' as const, label: '关心的话题', eyebrow: 'Following', sections: active } : null,
    briefs.length ? { key: 'briefs' as const, label: '其他动态', eyebrow: 'Briefs', sections: briefs } : null,
  ].filter((zone): zone is QualityZone => zone !== null)
}

/** 组内按 bucket_index 升序稳定排序；缺省（未分桶/历史数据）按 0 处理。 */
export function sortSectionsByBucketIndex(sections: DailyReportSection[]): DailyReportSection[] {
  return sections
    .map((section, index) => ({ section, index }))
    .sort((a, b) => {
      const ai = a.section.bucket_index ?? 0
      const bi = b.section.bucket_index ?? 0
      return ai === bi ? a.index - b.index : ai - bi
    })
    .map(entry => entry.section)
}

export function groupSectionsByTopic(zone: QualityZone): TopicGroup[] {
  const groups = new Map<string, TopicGroup>()

  for (const section of zone.sections) {
    const topicId = section.persistent_topic?.id
    const key = topicId != null ? `topic-${topicId}` : `section-${section.id}`
    const existing = groups.get(key)
    if (existing) {
      existing.sections.push(section)
      existing.articleCount += section.article_count
      existing.threadCount += section.threads?.length ?? 0
      continue
    }

    groups.set(key, {
      key,
      topicId,
      // 组名仍是当天第一条 section 的标题（父级泳道卡）。
      label: section.cluster_label,
      canonicalLabel: section.persistent_topic?.label,
      color: section.persistent_topic?.color,
      sections: [section],
      articleCount: section.article_count,
      threadCount: section.threads?.length ?? 0,
    })
  }

  // 泳道内主题分桶（add-intra-lane-theme-bucketing）：同一话题同一天可能有多条
  // 兄弟 section，组内按 bucket_index 升序稳定排序（同序保持原始相对顺序）。
  for (const group of groups.values()) {
    group.sections = sortSectionsByBucketIndex(group.sections)
  }

  return [...groups.values()]
}

// ── surface-story-heat-in-report-top：重点依据与正文定位的展示派生 ──

/** 重点条目的展示证据（全部可缺省：旧数据/标签回退不显示数字/徽标/按钮，不伪造）。 */
export interface HighlightEvidence {
  /** 存活事件线 id；null = 旧标签回退重点（无定位/进展/来源快照）。 */
  storyId: number | null
  /** 来源数快照；null = 缺失不显示（真实 0 是合法值，显示「0 个来源」）。 */
  sourceCount: number | null
  /** 可读热度提示（48h 热度/截止/不完整）；null = 无热度快照。 */
  heatHint: string | null
  /** 昨日重点再次入选（服务端派生）。 */
  isProgress: boolean
  /** 定位状态：'none' = 无 story_id（静态文案）；'ready' = 正文有目标可定位；
 *  'missing' = 有 id 但本期正文无对应线程（提示不跳转，不猜 tag→story）。 */
  locate: 'none' | 'ready' | 'missing'
}

/** 正文定位目标判定：范围限当前 report 的 body 线程（后端已过滤 display_tier 并归一存活 id）。 */
export function reportHasStoryThread(report: Pick<DailyReport, 'sections'>, storyId: number): boolean {
  return (report.sections ?? []).some(section =>
    (section.threads ?? []).some(thread => thread.story_id === storyId),
  )
}

/** 单条重点的展示证据：快照字段缺失一律降级，不因缺字段整体崩或显示占位符。 */
export function highlightEvidence(
  highlight: DailyReportHighlight,
  report: Pick<DailyReport, 'sections'>,
): HighlightEvidence {
  const storyId = typeof highlight.story_id === 'number' ? highlight.story_id : null
  const sourceCount = typeof highlight.participant_count === 'number' ? highlight.participant_count : null
  const heat = typeof highlight.story_heat === 'number' ? highlight.story_heat : null
  const heatHint = heat !== null
    ? [
        `48h 热度 ${heat.toFixed(1)}`,
        typeof highlight.heat_as_of === 'string' && highlight.heat_as_of ? `截止 ${highlight.heat_as_of.slice(0, 10)}` : '',
        highlight.heat_incomplete === true ? '热度证据不全' : '',
      ].filter(Boolean).join('，')
    : null
  return {
    storyId,
    sourceCount,
    heatHint,
    isProgress: highlight.is_progress === true,
    locate: storyId === null ? 'none' : reportHasStoryThread(report, storyId) ? 'ready' : 'missing',
  }
}

export function selectLeadStory(report: DailyReport): LeadStory | null {
  const firstHighlight = report.highlights?.[0] as DailyReportHighlight | undefined
  if (firstHighlight?.title) {
    return {
      title: firstHighlight.title,
      summary: firstHighlight.reason || report.summary,
      source: 'highlight',
    }
  }

  const section = sortDailyReportSections(report.sections || [])[0]
  if (!section) return null
  const firstThread = section.threads?.[0]
  return {
    title: section.cluster_label,
    summary: firstThread?.summary || firstThread?.title || report.summary,
    source: 'section',
    sectionId: section.id,
  }
}

function dateKey(date: string): string {
  return date.slice(0, 10)
}

function addUtcDays(key: string, amount: number): string {
  const value = new Date(`${key}T12:00:00Z`)
  value.setUTCDate(value.getUTCDate() + amount)
  return value.toISOString().slice(0, 10)
}

export function buildLifelineWindow(
  sections: SectionTimelineNode[],
  relations: SectionRelation[],
  reportDate: string,
  dayCount = 7,
): LifelineWindow {
  const endKey = dateKey(reportDate)
  const dayKeys = Array.from({ length: dayCount }, (_, index) => addUtcDays(endKey, index - dayCount + 1))
  const dayKeySet = new Set(dayKeys)
  const dayIndex = new Map(dayKeys.map((key, index) => [key, index]))
  const sectionDayById = new Map<number, string>()
  const grouped = new Map(dayKeys.map(key => [key, [] as SectionTimelineNode[]]))

  for (const section of sections) {
    const key = dateKey(section.period_date)
    if (!dayKeySet.has(key)) continue
    grouped.get(key)?.push(section)
    sectionDayById.set(section.id, key)
  }

  const edges = relations
    .filter(relation => relation.relation_type === 'identity')
    .flatMap((relation) => {
      const fromDayKey = sectionDayById.get(relation.from_id)
      const toDayKey = sectionDayById.get(relation.to_id)
      if (!fromDayKey || !toDayKey || fromDayKey === toDayKey) return []
      const span = Math.abs((dayIndex.get(toDayKey) ?? 0) - (dayIndex.get(fromDayKey) ?? 0))
      return [{
        key: `${relation.from_id}-${relation.to_id}`,
        fromSectionId: relation.from_id,
        toSectionId: relation.to_id,
        fromDayKey,
        toDayKey,
        weak: span > 1,
      }]
    })

  return {
    days: dayKeys.map(key => ({ key, date: key, sections: grouped.get(key) ?? [] })),
    edges,
  }
}

export function buildBezierPath(x1: number, y1: number, x2: number, y2: number): string {
  const midX = (x1 + x2) / 2
  return `M${x1},${y1} C${midX},${y1} ${midX},${y2} ${x2},${y2}`
}

export function formatMagazineDate(date: string): string {
  const value = new Date(date)
  const weekdays = ['日', '一', '二', '三', '四', '五', '六']
  return `${value.getMonth() + 1} 月 ${value.getDate()} 日 · 周${weekdays[value.getDay()]}`
}

export function createRequestCache<K, T>(
  loader: (key: K) => Promise<T>,
  onChange?: (entries: Map<K, RequestCacheEntry<T>>) => void,
) {
  const entries = new Map<K, RequestCacheEntry<T>>()
  const pending = new Map<K, Promise<T | undefined>>()

  function publish(key: K, entry: RequestCacheEntry<T>) {
    entries.set(key, entry)
    onChange?.(new Map(entries))
  }

  async function load(key: K, force = false): Promise<T | undefined> {
    const current = entries.get(key)
    if (!force && current?.status === 'success') return current.data
    if (!force && current?.status === 'error') return undefined
    const existing = pending.get(key)
    if (existing) return existing

    publish(key, { status: 'loading', data: current?.data })
    const request = loader(key)
      .then((data) => {
        publish(key, { status: 'success', data })
        return data
      })
      .catch((error: unknown) => {
        publish(key, {
          status: 'error',
          data: current?.data,
          error: error instanceof Error ? error.message : String(error),
        })
        return undefined
      })
      .finally(() => pending.delete(key))

    pending.set(key, request)
    return request
  }

  function clear() {
    entries.clear()
    pending.clear()
    onChange?.(new Map())
  }

  return {
    get: (key: K): RequestCacheEntry<T> => entries.get(key) ?? { status: 'idle' },
    load,
    clear,
  }
}
