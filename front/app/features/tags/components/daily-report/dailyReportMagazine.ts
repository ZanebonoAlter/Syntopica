import type {
  DailyReport,
  DailyReportHighlight,
  DailyReportSection,
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

// ── 重点依据与正文定位的展示派生（retire-story-adopt-event-metrics 3.3）──

/** 刊头重点的定位意图：`clusterId` = 聚簇代表标签 id（`?cluster=` 与标题点击）；
 *  `threadId` = 展开区具体线索（精确定位，null/缺省 = 该聚簇正文 DOM 首条）。 */
export interface LocateRequest {
  clusterId: number | null
  threadId: number | null
}

/** 重点条目的展示证据（全部可缺省：旧数据/标签回退不显示数字/口径/按钮，不伪造）。 */
export interface HighlightEvidence {
  /** 聚簇代表 event 标签 id；null = 旧标签回退重点（无定位/无聚簇快照）。 */
  clusterId: number | null
  /** 簇内文章数快照；null = 缺失不显示（真实 0 是合法值，显示「0 篇来源」）。 */
  articleCount: number | null
  /** 聚簇口径（中文）；null = 缺失不显示。 */
  clusterMode: string | null
  /** 数据截至日（YYYY-MM-DD）；null = 缺失不显示。 */
  dataAsOf: string | null
  /** 研究时点（可读文本）；null = 缺失不显示。 */
  researchAt: string | null
  /** 定位状态：'none' = 无 cluster_id（静态文案）；'ready' = 正文有目标可定位；
   *  'missing' = 有 id 但本期正文无对应线程（提示不跳转，不猜近似位置）。 */
  locate: 'none' | 'ready' | 'missing'
}

/** 正文定位目标判定（D14）：范围限当前 report 的 body 线程，命中条件 = 线程 `tag_ids`
 *  含聚簇代表标签 id（DOM 顺序第一条由正文分区在渲染时决定）。 */
export function reportHasClusterThread(report: Pick<DailyReport, 'sections'>, clusterId: number): boolean {
  return (report.sections ?? []).some(section =>
    (section.threads ?? []).some(thread => (thread.tag_ids ?? []).includes(clusterId)),
  )
}

/** 簇内标签全集：`highlight.tag_ids`（服务端给的聚簇本期标签）优先；
 *  缺失时回退代表标签 `cluster_id`（代表标签必属簇内，属回退而非编造）。 */
export function clusterTagIds(highlight: DailyReportHighlight): number[] {
  const tags = (highlight.tag_ids ?? []).filter(id => typeof id === 'number')
  if (tags.length) return tags
  return typeof highlight.cluster_id === 'number' ? [highlight.cluster_id] : []
}

/** 展开区的簇内线索行（纯前端现算，不改后端契约、不依赖快照字段）。 */
export interface ClusterThreadRow {
  threadId: number
  title: string
  sectionId: number
  /** 所属话题名（当天 section 标题，缺省回退组名 = 首个 section 标题）。 */
  sectionLabel: string
  /** 命中聚簇代表标签（cluster_id）的线索：排最前并标注「代表线索」。 */
  representative: boolean
}

/** 今日重点可展开的簇内全部线索：按正文分区渲染序（= DOM 序，与定位语义一致）
 *  取 thread `tag_ids` 与簇内标签集有交集的全部线索，代表线索置顶、组内保持 DOM 序。
 *  无交集返回空数组（调用方显示「本期正文未找到对应线索」，不硬造线索）。 */
export function collectClusterThreads(
  highlight: DailyReportHighlight,
  report: Pick<DailyReport, 'sections'>,
): ClusterThreadRow[] {
  const tagSet = new Set(clusterTagIds(highlight))
  if (tagSet.size === 0) return []
  const clusterId = typeof highlight.cluster_id === 'number' ? highlight.cluster_id : null
  const rows: ClusterThreadRow[] = []
  // 走与 DailyReportTopicSection 相同的分区/分组派生：zone 顺序 + 话题组顺序 + 桶序 = DOM 序。
  for (const zone of buildQualityZones(report.sections ?? [])) {
    for (const group of groupSectionsByTopic(zone)) {
      for (const section of group.sections) {
        for (const thread of section.threads ?? []) {
          const threadTags = thread.tag_ids ?? []
          if (!threadTags.some(id => tagSet.has(id))) continue
          rows.push({
            threadId: thread.id,
            title: thread.title,
            sectionId: section.id,
            sectionLabel: section.cluster_label || group.label,
            representative: clusterId !== null && threadTags.includes(clusterId),
          })
        }
      }
    }
  }
  return [...rows.filter(row => row.representative), ...rows.filter(row => !row.representative)]
}

/** 研究时点（RFC3339）→ 北京时间可读文本（`MM/DD HH:mm`）；非法值返回 null（省略不伪造）。 */
export function formatResearchAt(value: string | null | undefined): string | null {
  if (!value) return null
  const parsed = new Date(value)
  if (!Number.isFinite(parsed.getTime())) return null
  const parts = new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).formatToParts(parsed)
  const partValue = (type: string) => parts.find(part => part.type === type)?.value ?? ''
  return `${partValue('month')}/${partValue('day')} ${partValue('hour')}:${partValue('minute')}`
}

/** 单条重点的展示证据：快照字段缺失一律省略（不显示占位符、不伪造 0）。 */
export function highlightEvidence(
  highlight: DailyReportHighlight,
  report: Pick<DailyReport, 'sections'>,
): HighlightEvidence {
  const clusterId = typeof highlight.cluster_id === 'number' ? highlight.cluster_id : null
  return {
    clusterId,
    articleCount: typeof highlight.article_count === 'number' ? highlight.article_count : null,
    clusterMode: typeof highlight.cluster_mode === 'string' && highlight.cluster_mode ? highlight.cluster_mode : null,
    dataAsOf: typeof highlight.data_as_of === 'string' && highlight.data_as_of ? highlight.data_as_of.slice(0, 10) : null,
    researchAt: formatResearchAt(highlight.research_at),
    locate: clusterId === null ? 'none' : reportHasClusterThread(report, clusterId) ? 'ready' : 'missing',
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
