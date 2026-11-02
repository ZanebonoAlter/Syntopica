/**
 * 归档数据源（rework-workshop-into-radar-and-desk task 4.x，2026-10-25 产品复审后重做）。
 *
 * 产品定位：回答「指定日期有哪些文章被判定属于本版块，以及为什么」。
 *  - 不再按话题/标签分组：文章按日期平铺，一行看标题 + 全部命中标签；
 *  - 日期是主筛选：默认只看数据锚点当天，也支持任意起止日期；
 *  - `in_report` / `weak_evidence` 由后端派生，前端只读，不复制口径。
 *
 * 8G.3b（2026-10-31 归档重做）新增：
 *  - 组内排序两档（quality 后端质量序 / time 发布时间），切换需重拉（两种后端序）；
 *  - 分组走 Map 聚合（quality 序下 pub_date 交错，逐条相邻分组会碎组）。
 */
import { computed, onMounted, ref, watch, type Ref } from 'vue'
import { useSemanticBoardsApi } from '~/api/semanticBoards'
import type { BoardArticle } from '~/api/semanticBoards'

export type ArchiveFilter = 'all' | 'not_in_report' | 'weak'

/**
 * 组内排序模式：
 *  - quality：后端 sort=quality（最佳匹配层级 → 分数），组内保持后端原序；
 *  - time：发布时间倒序（同刻用 id 倒序兜底）。
 * 组间永远日期倒序（不跨日期排序是归档的既有红线）。
 */
export type ArchiveSortMode = 'quality' | 'time'

export interface ArchiveArticle extends BoardArticle {
  in_report?: boolean
  weak_evidence?: boolean
}

export interface ArchiveStats {
  total: number
  inReport: number
  notInReport: number
  weak: number
}

/** 规则语义（spec: 匹配证据展示的一句话解释）。 */
export const RULE_META: Record<string, { label: string; semantics: string }> = {
  direct_hit: {
    label: '直接命中',
    semantics: '标签的辅助标签与版块构成对得上，证据强，通常可以放心采信。',
  },
  hit_rate: {
    label: '命中率',
    semantics: '与版块构成标签的命中比例达标，中等证据，建议扫一眼原文再采信。',
  },
  max_sim: {
    label: '相似度',
    semantics: '与某个构成标签向量最相似，弱证据，容易把沾边的稿子带进来。',
  },
  weighted: {
    label: '综合',
    semantics: '相似度与标签密度加权达标，证据偏弱，边界稿建议人工核对。',
  },
  composite_hit: {
    label: '组合命中',
    semantics: '与版块组合标签的多个组件共同命中，证据强。',
  },
}

export function ruleMeta(rule: string): { label: string; semantics: string } {
  return RULE_META[rule] ?? { label: rule || '未打标', semantics: '没有可解释的匹配规则，按弱证据看待。' }
}

export function articleDate(article: ArchiveArticle): string {
  return (article.pub_date ?? '').slice(0, 10)
}

function todayLocal(): string {
  const now = new Date()
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`
}

/** 单篇弱证据：优先后端派生；旧响应回退到规则判断。 */
export function isWeakArticle(article: ArchiveArticle): boolean {
  if (typeof article.weak_evidence === 'boolean') return article.weak_evidence
  const tags = article.filtered_tags ?? []
  if (tags.length === 0) return true
  // 与后端 ApplyWeakEvidenceFilter 对齐：没有「强匹配」才算弱（任一强即不弱）。
  return !tags.some((tag) => tag.match_reason !== 'max_sim' && !tag.downgraded)
}

export function filterArchiveArticles(articles: ArchiveArticle[], filter: ArchiveFilter): ArchiveArticle[] {
  if (filter === 'not_in_report') return articles.filter((article) => !article.in_report)
  if (filter === 'weak') return articles.filter((article) => isWeakArticle(article))
  return articles
}

export function archiveStats(articles: ArchiveArticle[]): ArchiveStats {
  let inReport = 0
  let weak = 0
  for (const article of articles) {
    if (article.in_report) inReport++
    if (isWeakArticle(article)) weak++
  }
  return {
    total: articles.length,
    inReport,
    notInReport: articles.length - inReport,
    weak,
  }
}

/** 归档窗口（保留给旧调用方）：以 asOf 为右端、向前 days 天（含端点）。 */
export function archiveWindowRange(asOf: string | null, days: number): { start: string; end: string } {
  const end = asOf && /^\d{4}-\d{2}-\d{2}$/.test(asOf) ? asOf : todayLocal()
  const base = new Date(`${end}T00:00:00Z`)
  const span = Math.max(1, Math.floor(days)) - 1
  base.setUTCDate(base.getUTCDate() - span)
  return { start: base.toISOString().slice(0, 10), end }
}

/** 起止日期归一：非法/空回退今天；start > end 时交换，保证 API 窗口合法。 */
export function normalizeArchiveRange(start: string, end: string): { start: string; end: string } {
  const valid = (value: string) => /^\d{4}-\d{2}-\d{2}$/.test(value)
  const fallback = todayLocal()
  const safeEnd = valid(end) ? end : fallback
  const safeStart = valid(start) ? start : safeEnd
  return safeStart <= safeEnd ? { start: safeStart, end: safeEnd } : { start: safeEnd, end: safeStart }
}

/** 文章平铺排序：完整发布时间倒序；发布时间相同才用 id 倒序兜底。 */
export function sortArchiveArticles(articles: ArchiveArticle[]): ArchiveArticle[] {
  return [...articles].sort((a, b) => {
    const aTime = a.pub_date ?? ''
    const bTime = b.pub_date ?? ''
    if (aTime !== bTime) return bTime.localeCompare(aTime)
    return b.id - a.id
  })
}

/**
 * 按日期分组（Map 聚合，不按相邻日期切段）。
 *
 * 关键：quality 序下后端返回的 pub_date 是交错的（强证据聚在一起、弱证据散落），
 * 逐条相邻分组会把同一天碎成几十个组（原型期实测 163 篇碎成 85 组）。
 * 因此必须先按日期收集进 Map，再组间日期倒序输出。
 *  - quality：组内保持后端原序（BestTier/分数口径由后端定，前端不复制）；
 *  - time：组内发布时间倒序，同刻 id 倒序兜底。
 */
export function groupArchiveArticles(
  articles: ArchiveArticle[],
  sortMode: ArchiveSortMode = 'quality',
): Array<{ date: string; articles: ArchiveArticle[] }> {
  const byDate = new Map<string, ArchiveArticle[]>()
  for (const article of articles) {
    const date = articleDate(article)
    const bucket = byDate.get(date)
    if (bucket) bucket.push(article)
    else byDate.set(date, [article])
  }
  const groups = [...byDate.entries()]
    .map(([date, items]) => ({ date, articles: items }))
    .sort((a, b) => b.date.localeCompare(a.date))
  if (sortMode === 'time') {
    for (const group of groups) {
      group.articles.sort((a, b) => {
        const aTime = a.pub_date ?? ''
        const bTime = b.pub_date ?? ''
        if (aTime !== bTime) return bTime.localeCompare(aTime)
        return b.id - a.id
      })
    }
  }
  return groups
}

const ARCHIVE_PER_PAGE = 100
const ARCHIVE_MAX_PAGES = 20

export function useBoardArchive(
  boardId: Ref<number>,
  startDate: Ref<string>,
  endDate: Ref<string>,
) {
  const api = useSemanticBoardsApi()
  const articles = ref<ArchiveArticle[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const filter = ref<ArchiveFilter>('all')
  const sortMode = ref<ArchiveSortMode>('quality')
  const feedId = ref<number | null>(null)
  const labelId = ref<number | null>(null)
  const showDirectionMismatch = ref(false)
  const feedOptions = ref<Array<{ id: number, name: string }>>([])
  const truncated = ref(false)

  let requestSeq = 0

  const range = computed(() => normalizeArchiveRange(startDate.value, endDate.value))

  /** 切换组内排序：quality/time 是两种后端序，切换需重拉（watch 触发）。 */
  function setSortMode(mode: ArchiveSortMode): void {
    sortMode.value = mode
  }

  async function load(): Promise<void> {
    const seq = ++requestSeq
    loading.value = true
    error.value = null
    truncated.value = false
    const collected: ArchiveArticle[] = []
    let failed = false
    for (let page = 1; page <= ARCHIVE_MAX_PAGES; page++) {
      const res = await api.getBoardArticles(boardId.value, {
        start_date: range.value.start,
        end_date: range.value.end,
        ...(feedId.value ? { feed_id: feedId.value } : {}),
        ...(labelId.value ? { auxiliary_label_id: labelId.value } : {}),
        ...(showDirectionMismatch.value ? { show_direction_mismatch: 'true' } : {}),
        page,
        per_page: ARCHIVE_PER_PAGE,
        sort: sortMode.value,
      })
      if (seq !== requestSeq) return
      if (!res?.success) {
        failed = true
        error.value = res?.error || '归档加载失败'
        break
      }
      collected.push(...((res.data ?? []) as ArchiveArticle[]))
      const pages = res.pagination?.pages ?? 1
      if (page >= pages) break
      if (page === ARCHIVE_MAX_PAGES) truncated.value = true
    }
    if (seq !== requestSeq) return
    if (!failed) {
      // 保留后端返回原序：quality 序就是后端质量序（前端不重排）。
      articles.value = collected
      if (feedId.value === null) {
        const seen = new Map<number, string>()
        for (const article of collected) {
          if (!seen.has(article.feed_id)) seen.set(article.feed_id, article.feed_name)
        }
        feedOptions.value = [...seen.entries()]
          .map(([id, name]) => ({ id, name }))
          .sort((a, b) => a.name.localeCompare(b.name))
      }
    }
    loading.value = false
  }

  /** 当前排序模式下的文章顺序：quality=后端原序；time=发布时间倒序。 */
  const orderedArticles = computed(() => (
    sortMode.value === 'quality' ? articles.value : sortArchiveArticles(articles.value)
  ))
  const sortedArticles = computed(() => orderedArticles.value)
  const visibleArticles = computed(() => filterArchiveArticles(orderedArticles.value, filter.value))
  /** 日期分组（Map 聚合）：组间日期倒序，组内按 sortMode。 */
  const groupedArticles = computed(() => groupArchiveArticles(visibleArticles.value, sortMode.value))
  const stats = computed(() => archiveStats(articles.value))

  watch([boardId, range, feedId, labelId, showDirectionMismatch, sortMode], () => { void load() })
  onMounted(() => { void load() })

  return {
    articles,
    sortedArticles,
    visibleArticles,
    groupedArticles,
    stats,
    filter,
    sortMode,
    setSortMode,
    feedId,
    feedOptions,
    labelId,
    showDirectionMismatch,
    loading,
    error,
    truncated,
    range,
    reload: load,
  }
}
