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
      label: section.cluster_label,
      canonicalLabel: section.persistent_topic?.label,
      color: section.persistent_topic?.color,
      sections: [section],
      articleCount: section.article_count,
      threadCount: section.threads?.length ?? 0,
    })
  }

  return [...groups.values()]
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
