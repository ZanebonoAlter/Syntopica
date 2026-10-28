import { computed, ref, watch, type Ref } from 'vue'
import { useDailyReportsApi } from '~/api/dailyReports'
import { useArticlesApi } from '~/api/articles'
import { createRequestCache, type RequestCacheEntry } from '~/features/tags/components/daily-report/dailyReportMagazine'
import { useTopicKeyEvents } from '~/features/tags/composables/useTopicKeyEvents'
import type {
  DailyReport,
  DailyReportListItem,
  DailyReportSection,
  SectionRelation,
  SectionTimelineNode,
} from '~/api/dailyReports'

export interface TopicLifelineData {
  sections: SectionTimelineNode[]
  relations: SectionRelation[]
}

export interface ArticleTitle {
  id: number
  title: string
}

function responseError(message: string): Error {
  return new Error(message)
}

/** 重新分桶结果（add-intra-lane-theme-bucketing）：buckets=1 表示未拆动（未达阈值/拆不动）。 */
export interface RebucketOutcome {
  ok: boolean
  buckets: number
  error?: string
}

export interface UseDailyReportReaderOptions {
  /** 首屏载入的日报天数；阅读台右栏是 14 天口径，默认 7 保持旧浮层行为。 */
  initialDays?: number
}

export function useDailyReportReader(boardId: Ref<number>, options: UseDailyReportReaderOptions = {}) {
  const { getBoardDailyReports, getDailyReportDetail, getTopicLifeline, rebucketSection } = useDailyReportsApi()
  const { getArticle } = useArticlesApi()
  // 关键事件共享缓存（add-topic-key-event-markers）：日报读端与总览共用；
  // 读取随详情加载触发，只读不触发推荐（spec：打开页面只读）。
  const keyEvents = useTopicKeyEvents()

  const initialDays = Math.max(7, Math.floor(options.initialDays ?? 7))
  const reports = ref<DailyReportListItem[]>([])
  const days = ref(initialDays)
  const loading = ref(false)
  const currentDayIndex = ref(-1)
  const detailCache = ref(new Map<number, DailyReport>())
  const detailLoading = ref<number | null>(null)
  const detailError = ref('')
  const lifelineEntries = ref(new Map<number, RequestCacheEntry<TopicLifelineData>>())
  const articleEntries = ref(new Map<number, RequestCacheEntry<ArticleTitle>>())
  /** 正在进行重分桶的 section id（按钮置灰 + 原位 spinner）。 */
  const rebucketSectionId = ref<number | null>(null)
  /** 重分桶后失效的旧锚点 → 该组首条 section（深链兼容；只影响本会话内存）。 */
  const sectionAliases = ref(new Map<number, number>())

  const lifelineCache = createRequestCache<number, TopicLifelineData>(async (topicId) => {
    const response = await getTopicLifeline(topicId)
    if (!response.success || !response.data) throw responseError('话题生命线加载失败')
    return response.data
  }, entries => { lifelineEntries.value = entries })

  const articleCache = createRequestCache<number, ArticleTitle>(async (articleId) => {
    const response = await getArticle(articleId)
    if (!response.success || !response.data) throw responseError(`文章 #${articleId} 加载失败`)
    return { id: articleId, title: response.data.title || '(无标题)' }
  }, entries => { articleEntries.value = entries })

  const selectedReport = computed(() => {
    if (currentDayIndex.value < 0 || currentDayIndex.value >= reports.value.length) return null
    return reports.value[currentDayIndex.value]
  })

  const selectedDetail = computed(() => {
    const report = selectedReport.value
    return report ? detailCache.value.get(report.id) ?? null : null
  })

  async function loadReports() {
    loading.value = true
    try {
      const response = await getBoardDailyReports(boardId.value, { days: days.value })
      reports.value = response.success && response.data ? response.data.reports || [] : []
    } finally {
      loading.value = false
    }
  }

  async function ensureDetail(reportId: number) {
    if (detailCache.value.has(reportId)) return detailCache.value.get(reportId)
    detailLoading.value = reportId
    detailError.value = ''
    try {
      const response = await getDailyReportDetail(reportId)
      if (!response.success || !response.data) {
        detailError.value = '日报详情加载失败'
        return undefined
      }
      detailCache.value.set(reportId, response.data.report)
      detailCache.value = new Map(detailCache.value)
      // 只读拉取当期关键事件注记（不触发推荐；历史详情同理）。
      void keyEvents.ensureReportKeyEvents(reportId)
      return response.data.report
    } finally {
      detailLoading.value = null
    }
  }

  // 静默加载历史日报详情：写入 detailCache 供历史节点展示，
  // 但不触碰 detailLoading / detailError，避免遮蔽当前正在阅读的日报。
  async function ensureHistoricalDetail(reportId: number) {
    if (detailCache.value.has(reportId)) return detailCache.value.get(reportId)
    const response = await getDailyReportDetail(reportId)
    if (!response.success || !response.data) return undefined
    detailCache.value.set(reportId, response.data.report)
    detailCache.value = new Map(detailCache.value)
    void keyEvents.ensureReportKeyEvents(reportId)
    return response.data.report
  }

  async function selectReport(index: number) {
    const report = reports.value[index]
    if (!report) return undefined
    currentDayIndex.value = index
    return ensureDetail(report.id)
  }

  function getReportDetail(reportId: number): DailyReport | undefined {
    return detailCache.value.get(reportId)
  }

  async function selectReportById(reportId: number) {
    const index = reports.value.findIndex(report => report.id === reportId)
    if (index < 0) return undefined
    return selectReport(index)
  }

  async function shiftReport(offset: number) {
    const next = currentDayIndex.value + offset
    if (next < 0 || next >= reports.value.length) return undefined
    return selectReport(next)
  }

  async function loadMore() {
    days.value += 7
    await loadReports()
  }

  function getLifeline(topicId: number): RequestCacheEntry<TopicLifelineData> {
    return lifelineEntries.value.get(topicId) ?? { status: 'idle' }
  }

  async function ensureLifeline(topicId: number, retry = false) {
    return lifelineCache.load(topicId, retry)
  }

  function getArticleTitle(articleId: number): RequestCacheEntry<ArticleTitle> {
    return articleEntries.value.get(articleId) ?? { status: 'idle' }
  }

  async function ensureArticleTitles(articleIds: number[], retry = false) {
    const uniqueIds = [...new Set(articleIds)]
    await Promise.all(uniqueIds.map(articleId => articleCache.load(articleId, retry)))
  }

  /**
   * 对某 section 所在的桶组重跑泳道内主题分桶。
   * 成功时**局部替换**该组 sections（不动其余分区与滚动位置）；失败时不改数据。
   * 重分桶会删建 section 行：被删掉的旧 id 记入 sectionAliases，深链回退到组头。
   */
  async function rebucket(reportId: number, sectionId: number): Promise<RebucketOutcome> {
    if (rebucketSectionId.value != null) return { ok: false, buckets: 0, error: '正在重新分桶' }
    rebucketSectionId.value = sectionId
    try {
      const response = await rebucketSection(reportId, sectionId)
      if (!response.success || !response.data) {
        return { ok: false, buckets: 0, error: response.error || '重新分桶失败' }
      }
      const { buckets, sections } = response.data
      const report = detailCache.value.get(reportId)
      if (report && buckets > 1) {
        const target = report.sections.find(section => section.id === sectionId)
        const clusterIndex = target?.cluster_index
        const groupSections = clusterIndex == null
          ? []
          : report.sections.filter(section => section.cluster_index === clusterIndex)
        const oldIds = groupSections.map(section => section.id)
        const headId = sections[0]?.id
        const nextSections: DailyReportSection[] = [
          ...report.sections.filter(section => clusterIndex == null || section.cluster_index !== clusterIndex),
          ...sections,
        ]
        if (headId != null) {
          const aliases = new Map(sectionAliases.value)
          for (const oldId of oldIds) {
            if (!sections.some(section => section.id === oldId)) aliases.set(oldId, headId)
          }
          sectionAliases.value = aliases
        }
        detailCache.value.set(reportId, { ...report, sections: nextSections })
        detailCache.value = new Map(detailCache.value)
      }
      return { ok: true, buckets }
    } finally {
      rebucketSectionId.value = null
    }
  }

  /** 当前选中日报详情失败后的显式重试（B-6：错误条必须可恢复）。 */
  async function retryDetail() {
    const report = selectedReport.value
    if (!report) return undefined
    detailError.value = ''
    return ensureDetail(report.id)
  }

  async function retryArticle(articleId: number) {
    return articleCache.load(articleId, true)
  }

  function clearBoardState() {
    days.value = initialDays
    reports.value = []
    currentDayIndex.value = -1
    detailCache.value = new Map()
    detailLoading.value = null
    detailError.value = ''
    lifelineCache.clear()
    articleCache.clear()
    rebucketSectionId.value = null
    sectionAliases.value = new Map()
  }

  watch(boardId, async () => {
    clearBoardState()
    await loadReports()
  }, { immediate: true })

  return {
    reports,
    days,
    loading,
    currentDayIndex,
    detailLoading,
    detailError,
    lifelineEntries,
    articleEntries,
    selectedReport,
    selectedDetail,
    detailCache,
    keyEvents,
    rebucketSectionId,
    sectionAliases,
    rebucket,
    loadReports,
    ensureDetail,
    ensureHistoricalDetail,
    getReportDetail,
    selectReport,
    selectReportById,
    shiftReport,
    loadMore,
    getLifeline,
    ensureLifeline,
    getArticleTitle,
    ensureArticleTitles,
    retryDetail,
    retryArticle,
  }
}
