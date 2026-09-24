import { ref, computed, watch, nextTick } from 'vue'
import { marked } from 'marked'
import type { Article, RssFeed } from '~/types'
import { useArticlesApi } from '~/api/articles'
import { useReadingTracker, useScrollDepthTracker } from '~/features/preferences/public'
import { useContentCompletion, type ContentCompletionStatus } from '~/features/articles/composables/useContentCompletion'
import { useFirecrawlApi } from '~/api/firecrawl'
import { shouldShowArticleDescription } from '~/utils/articleContentGuards'
import { proxyImagesInHtml } from '~/utils/imageProxy'
import {
  getArticleContentSources,
  resolveArticleContentBySource,
  type ArticleContentSource,
} from '~/utils/articleContentSource'
import { useTagWebSocket } from '~/features/articles/composables/useTagWebSocket'
import type { ArticleTag } from '~/types/article'
import { useWatchedTagsApi } from '~/api/watchedTags'
import {
  getFirecrawlStatusMeta,
  getStatusToneClasses,
  getSummaryStatusMeta,
  shouldShowFirecrawlStatus,
  shouldShowSummaryStatus,
} from '~/features/articles/composables/useArticleProcessingStatus'
import { useFeedsStore } from '~/stores/feeds'
import { useArticlesStore } from '~/stores/articles'
import { useAI } from '~/composables/useAI'

marked.setOptions({ gfm: true, breaks: true })

export function useArticleContentView(props: {
  article: Article | null
  articles?: Article[]
}) {
  const feedsStore = useFeedsStore()
  const { isAIEnabled } = useAI()
  const articlesApi = useArticlesApi()
  const watchedTagsApi = useWatchedTagsApi()
  const { crawlArticle } = useFirecrawlApi()
  const { getCompletionStatus, completeArticle } = useContentCompletion()
  const { onResult: onTagResult, onError: onTagError, watchArticle, clearWatch } = useTagWebSocket()

  // ---- State ----
  const viewMode = ref<'preview' | 'iframe'>('preview')
  const iframeLoading = ref(true)
  const isFullscreen = ref(false)
  const liveStatus = ref<ContentCompletionStatus | null>(null)
  const selectedContentSource = ref<ArticleContentSource>('firecrawl')
  const manualFirecrawlLoading = ref(false)
  const manualSummaryLoading = ref(false)
  const manualTaggingLoading = ref(false)
  const manualActionError = ref<string | null>(null)
  const taggingError = ref<string | null>(null)
  const watchPendingIds = ref(new Set<number>())
  const scrollProgress = ref(0)
  const scrollTop = ref(0)
  const contentContainer = ref<HTMLElement>()
  const fullscreenContentContainer = ref<HTMLElement>()

  // ---- Computed ----
  const currentIndex = computed(() => {
    if (!props.article || !props.articles?.length) return -1
    return props.articles!.findIndex(item => item.id === props.article!.id)
  })
  const hasPrev = computed(() => currentIndex.value > 0)
  const hasNext = computed(() => currentIndex.value < (props.articles?.length ?? 0) - 1)
  const showBackTop = computed(() => scrollTop.value > 300)

  const feed = computed(() => {
    if (!props.article) return null
    return feedsStore.feeds.find((item: RssFeed) => item.id === props.article!.feedId) ?? null
  })

  // ---- Content tracking ----
  let lastScrollDepth = 0

  const { readingTime, trackEvent, uploadEvents } = useReadingTracker({
    article: computed(() => props.article),
  })

  useScrollDepthTracker(contentContainer, (depth: number) => {
    if (depth > lastScrollDepth && Math.abs(depth - lastScrollDepth) >= 10) {
      lastScrollDepth = depth
      trackEvent('scroll', depth, readingTime.value)
    }
  })

  function onContentScroll(e: Event) {
    const el = e.target as HTMLElement
    scrollTop.value = el.scrollTop
    const maxScroll = el.scrollHeight - el.clientHeight
    scrollProgress.value = maxScroll > 0 ? Math.round((el.scrollTop / maxScroll) * 100) : 0
  }

  function scrollToTop() {
    const container = isFullscreen.value ? fullscreenContentContainer.value : contentContainer.value
    container?.scrollTo({ top: 0, behavior: 'smooth' })
  }

  // ---- Merged article with live status ----
  const mergedArticle = computed<Article | null>(() => {
    if (!props.article) return null
    return {
      ...props.article,
      summaryStatus: liveStatus.value?.summaryStatus ?? props.article.summaryStatus,
      completionAttempts: liveStatus.value?.attempts ?? props.article.completionAttempts,
      completionError: liveStatus.value?.error ?? props.article.completionError,
      summaryGeneratedAt: liveStatus.value?.summaryGeneratedAt ?? props.article.summaryGeneratedAt,
      aiContentSummary: liveStatus.value?.aiContentSummary ?? props.article.aiContentSummary,
      firecrawlContent: liveStatus.value?.firecrawlContent ?? props.article.firecrawlContent,
      firecrawlStatus: liveStatus.value?.firecrawlStatus ?? props.article.firecrawlStatus,
      firecrawlError: liveStatus.value?.firecrawlError ?? props.article.firecrawlError,
    }
  })

  const firecrawlMeta = computed(() => mergedArticle.value ? getFirecrawlStatusMeta(mergedArticle.value) : null)
  const summaryMeta = computed(() => mergedArticle.value ? getSummaryStatusMeta(mergedArticle.value) : null)
  const showFirecrawlStatus = computed(() => mergedArticle.value ? shouldShowFirecrawlStatus(mergedArticle.value, feed.value) : false)
  const showSummaryStatus = computed(() => mergedArticle.value ? shouldShowSummaryStatus(mergedArticle.value, feed.value) : false)
  const showManualFirecrawlAction = computed(() => feed.value?.firecrawlEnabled === true)
  const showManualSummaryAction = computed(() => feed.value?.articleSummaryEnabled === true)
  const showProcessingPanel = computed(() => {
    if (!mergedArticle.value) return false
    return showFirecrawlStatus.value || showSummaryStatus.value
      || showManualFirecrawlAction.value || showManualSummaryAction.value
      || detailLines.value.length > 0
      || Boolean(manualActionError.value || mergedArticle.value.firecrawlError || mergedArticle.value.completionError)
  })
  const actionBusy = computed(() => manualFirecrawlLoading.value || manualSummaryLoading.value)

  const manualFirecrawlLabel = computed(() => {
    if (manualFirecrawlLoading.value) return '抓取中...'
    return mergedArticle.value?.firecrawlStatus === 'completed' ? '重新抓取全文' : '手动抓取全文'
  })
  const manualSummaryLabel = computed(() => {
    if (manualSummaryLoading.value) return '总结中...'
    return mergedArticle.value?.aiContentSummary ? '重新生成总结' : '手动生成总结'
  })
  const manualTaggingLabel = computed(() => {
    if (manualTaggingLoading.value) return '打标签中...'
    return (mergedArticle.value?.tagCount ?? 0) > 0 ? '重新打标签' : '手动打标签'
  })

  // ---- Content source ----
  const contentSources = computed(() => getArticleContentSources({
    firecrawlContent: mergedArticle.value?.firecrawlContent,
    content: mergedArticle.value?.content,
  }))
  const availableContentSources = computed(() => contentSources.value.available)
  const activeContentSource = computed<ArticleContentSource | null>(() => {
    if (availableContentSources.value.includes(selectedContentSource.value)) return selectedContentSource.value
    return contentSources.value.defaultSource
  })
  const showContentSourceToggle = computed(() => availableContentSources.value.length > 1)

  const displayContent = computed(() => {
    if (!mergedArticle.value) return ''
    const resolvedContent = resolveArticleContentBySource(contentSources.value, activeContentSource.value ?? undefined)
    if (!resolvedContent) return ''
    let content: string
    if (activeContentSource.value === 'firecrawl') {
      content = marked.parse(resolvedContent) as string
    } else {
      content = resolvedContent
    }
    return proxyImagesInHtml(content.replace(/<img[^>]*src=["']Base64-Image-Removed["'][^>]*\/?>/gi, ''))
  })

  // 导语取值优先级（slim-article-list-payload）：详情 description 优先，
  // 详情未就绪时回退列表窄投影的 excerpt，两者都空则为空串。
  const ledeText = computed(() => {
    const article = mergedArticle.value
    if (!article) return ''
    if (article.description.trim()) return article.description
    if (article.excerpt?.trim()) return article.excerpt
    return ''
  })

  const showDescription = computed(() => {
    if (!mergedArticle.value) return false
    return shouldShowArticleDescription(ledeText.value, displayContent.value)
  })

  // ---- Detail lines ----
  const detailLines = computed(() => {
    if (!mergedArticle.value) return []
    const lines: string[] = []
    if (mergedArticle.value.firecrawlCrawledAt) {
      lines.push(`抓取时间：${mergedArticle.value.firecrawlCrawledAt.slice(0, 16).replace('T', ' ')}`)
    }
    if (mergedArticle.value.summaryGeneratedAt) {
      lines.push(`总结时间：${mergedArticle.value.summaryGeneratedAt.slice(0, 16).replace('T', ' ')}`)
    }
    if ((mergedArticle.value.completionAttempts ?? 0) > 0) {
      lines.push(`总结尝试：${mergedArticle.value.completionAttempts} 次`)
    }
    return lines
  })

  const renderedStoredSummary = computed(() => {
    const content = mergedArticle.value?.aiContentSummary
    if (!content) return ''
    return marked.parse(content) as string
  })

  // ---- Sync helpers ----
  function syncCurrentArticle(updates: Partial<Article>) {
    if (!props.article) return
    Object.assign(props.article, updates)
    const storeArticle = useArticlesStore().articles.find(item => item.id === props.article!.id)
    if (storeArticle) Object.assign(storeArticle, updates)
  }

  function applyLiveStatusToArticle(status: ContentCompletionStatus | null) {
    if (!props.article || !status) return
    syncCurrentArticle({
      summaryStatus: status.summaryStatus,
      completionAttempts: status.attempts,
      completionError: status.error ?? undefined,
      summaryGeneratedAt: status.summaryGeneratedAt ?? undefined,
      aiContentSummary: status.aiContentSummary ?? props.article.aiContentSummary,
      firecrawlContent: status.firecrawlContent ?? props.article.firecrawlContent,
      firecrawlStatus: status.firecrawlStatus ?? props.article.firecrawlStatus,
      firecrawlError: status.firecrawlError ?? props.article.firecrawlError,
    })
  }

  async function loadCompletionStatus(articleId: string) {
    try {
      const status = await getCompletionStatus(articleId)
      liveStatus.value = status
      applyLiveStatusToArticle(status)
    } catch { liveStatus.value = null }
  }

  // ---- Manual actions ----
  async function handleManualFirecrawl() {
    if (!props.article || manualFirecrawlLoading.value) return
    manualFirecrawlLoading.value = true
    manualActionError.value = null
    syncCurrentArticle({ firecrawlStatus: 'processing', firecrawlError: undefined })
    try {
      const response = await crawlArticle(Number(props.article.id))
      if (!response.success) throw new Error(response.error || '手动抓取失败')
      syncCurrentArticle({
        firecrawlStatus: response.data?.firecrawl_status === 'completed' ? 'completed' : props.article.firecrawlStatus,
        firecrawlContent: response.data?.firecrawl_content || props.article.firecrawlContent,
        firecrawlError: undefined,
        firecrawlCrawledAt: new Date().toISOString(),
      })
      await loadCompletionStatus(props.article.id)
    } catch (error) {
      const message = error instanceof Error ? error.message : '手动抓取失败'
      manualActionError.value = message
      syncCurrentArticle({ firecrawlStatus: 'failed', firecrawlError: message })
    } finally { manualFirecrawlLoading.value = false }
  }

  async function handleManualSummary() {
    if (!props.article || manualSummaryLoading.value) return
    manualSummaryLoading.value = true
    manualActionError.value = null
    syncCurrentArticle({ summaryStatus: 'pending', completionError: undefined })
    try {
      await completeArticle(props.article.id, { force: true })
      await loadCompletionStatus(props.article.id)
    } catch (error) {
      const message = error instanceof Error ? error.message : '手动总结失败'
      manualActionError.value = message
      syncCurrentArticle({ summaryStatus: 'failed', completionError: message })
    } finally { manualSummaryLoading.value = false }
  }

  async function handleManualTagging() {
    if (!props.article || manualTaggingLoading.value) return
    manualTaggingLoading.value = true
    taggingError.value = null
    try {
      const response = await articlesApi.retagArticle(Number(props.article.id))
      if (!response.success || !response.data) throw new Error(response.error || '提交标签任务失败')
      watchArticle(Number(props.article.id))
    } catch (error) {
      const message = error instanceof Error ? error.message : '提交标签任务失败'
      taggingError.value = message
      manualTaggingLoading.value = false
    }
  }

  async function handleTagWatchToggle(payload: { id: number; slug: string }) {
    if (!props.article?.tags || watchPendingIds.value.has(payload.id)) return
    const tag = props.article.tags.find(t => t.id === payload.id)
    if (!tag) return
    const previous = tag.isWatched
    const newWatched = !previous
    tag.isWatched = newWatched
    watchPendingIds.value.add(payload.id)
    try {
      const response = newWatched
        ? await watchedTagsApi.watchTag(payload.id)
        : await watchedTagsApi.unwatchTag(payload.id)
      if (!response.success) throw new Error(response.error || '操作失败')
    } catch { tag.isWatched = previous }
    finally { watchPendingIds.value.delete(payload.id) }
  }

  // ---- Navigation ----
  function handleFavorite() {
    if (!props.article) return
    trackEvent(props.article.favorite ? 'unfavorite' : 'favorite', lastScrollDepth, readingTime.value)
    uploadEvents()
  }

  function openOriginal() {
    if (props.article?.link) window.open(props.article.link, '_blank')
  }

  function toggleViewMode() {
    viewMode.value = viewMode.value === 'preview' ? 'iframe' : 'preview'
    if (viewMode.value === 'iframe') iframeLoading.value = true
  }

  function toggleFullscreen() { isFullscreen.value = !isFullscreen.value }
  function handleIframeLoad() { iframeLoading.value = false }
  function handleIframeError() { iframeLoading.value = false }

  function navigatePrev() {
    if (!hasPrev.value || !props.article || !props.articles?.length) return
    const prev = props.articles[currentIndex.value - 1]
    if (prev) { /* navigate is handled by emit in component */ }
  }

  function navigateNext() {
    if (!hasNext.value || !props.article || !props.articles?.length) return
    const next = props.articles[currentIndex.value + 1]
    if (next) { /* navigate is handled by emit in component */ }
  }

  // ---- Watchers & Setup ----
  watch(() => props.article?.id, (newId, oldId) => {
    if (newId === oldId) return
    iframeLoading.value = true
    viewMode.value = 'preview'
    liveStatus.value = null
    manualActionError.value = null
    taggingError.value = null
    manualFirecrawlLoading.value = false
    manualSummaryLoading.value = false
    manualTaggingLoading.value = false
    lastScrollDepth = 0
    scrollProgress.value = 0
    scrollTop.value = 0
    clearWatch()
    selectedContentSource.value = getArticleContentSources({
      firecrawlContent: props.article?.firecrawlContent,
      content: props.article?.content,
    }).defaultSource ?? 'firecrawl'
    nextTick(() => {
      contentContainer.value?.scrollTo({ top: 0 })
      fullscreenContentContainer.value?.scrollTo({ top: 0 })
    })
    if (newId) void loadCompletionStatus(newId)
  }, { immediate: true })

  // Tag webSocket events
  onTagResult((articleId: number, tags: ArticleTag[]) => {
    if (!props.article || String(articleId) !== props.article.id) return
    syncCurrentArticle({ tags, tagCount: tags.length })
    manualTaggingLoading.value = false
    taggingError.value = null
  })

  onTagError((articleId: number, error: string) => {
    if (!props.article || String(articleId) !== props.article.id) return
    taggingError.value = error
    manualTaggingLoading.value = false
  })

  return {
    // State
    viewMode, iframeLoading, isFullscreen, liveStatus,
    selectedContentSource, manualFirecrawlLoading,
    manualSummaryLoading, manualTaggingLoading,
    manualActionError, taggingError, watchPendingIds,
    scrollProgress, scrollTop, contentContainer, fullscreenContentContainer,
    showBackTop,

    // Computed
    feed, currentIndex, hasPrev, hasNext,
    mergedArticle, firecrawlMeta, summaryMeta,
    showFirecrawlStatus, showSummaryStatus,
    showManualFirecrawlAction, showManualSummaryAction,
    showProcessingPanel, actionBusy,
    manualFirecrawlLabel, manualSummaryLabel, manualTaggingLabel,
    contentSources, availableContentSources, activeContentSource,
    showContentSourceToggle, displayContent, showDescription,
    detailLines, renderedStoredSummary,

    // Event handlers
    onContentScroll, scrollToTop,
    handleManualFirecrawl, handleManualSummary, handleManualTagging,
    handleTagWatchToggle, handleFavorite,
    openOriginal, toggleViewMode, toggleFullscreen,
    handleIframeLoad, handleIframeError,
    navigatePrev, navigateNext,
    readingTime, trackEvent, uploadEvents, lastScrollDepth,
  }
}
