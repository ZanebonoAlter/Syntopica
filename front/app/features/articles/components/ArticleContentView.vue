<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { marked } from 'marked'
import type { Article, RssFeed } from '~/types'
import ArticleTagList from './ArticleTagList.vue'
import { useArticlesApi } from '~/api/articles'
import { useReadingTracker, useScrollDepthTracker } from '~/features/preferences/composables/useReadingTracker'
import { useContentCompletion, type ContentCompletionStatus } from '~/features/articles/composables/useContentCompletion'
import { useFirecrawlApi } from '~/api/firecrawl'
import { shouldShowArticleDescription } from '~/utils/articleContentGuards'
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

marked.setOptions({ gfm: true, breaks: true })

interface Props {
  article: Article | null
  articles?: Article[]
  onClose?: () => void
  highlightedTagSlugs?: string[]
}

const props = withDefaults(defineProps<Props>(), {
  article: null,
  articles: () => [],
  onClose: () => {},
  highlightedTagSlugs: () => [],
})

const emit = defineEmits<{
  favorite: [id: string]
  navigate: [article: Article]
  articleUpdate: [id: string, updates: Partial<Article>]
}>()

const apiStore = useApiStore()
const feedsStore = useFeedsStore()
const { isAIEnabled } = useAI()
const { $dayjs } = useNuxtApp()
const articlesApi = useArticlesApi()
const watchedTagsApi = useWatchedTagsApi()
const { crawlArticle } = useFirecrawlApi()
const { getCompletionStatus, completeArticle } = useContentCompletion()
const { onResult: onTagResult, onError: onTagError, watchArticle, clearWatch } = useTagWebSocket()

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
const fullscreenContentContainer = ref<HTMLElement>()
const showBackTop = computed(() => scrollTop.value > 300)

const feed = computed(() => {
  const article = props.article
  if (!article) return null
  return feedsStore.feeds.find((item: RssFeed) => item.id === article.feedId) ?? null
})

const currentIndex = computed(() => {
  if (!props.article || !props.articles.length) return -1
  return props.articles.findIndex(item => item.id === props.article?.id)
})

const hasPrev = computed(() => currentIndex.value > 0)
const hasNext = computed(() => currentIndex.value < props.articles.length - 1)

const contentContainer = ref<HTMLElement>()
let lastScrollDepth = 0

const { readingTime, trackEvent, uploadEvents } = useReadingTracker({
  article: computed(() => props.article),
})

useScrollDepthTracker(contentContainer, (depth) => {
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
  const container = isFullscreen.value
    ? fullscreenContentContainer.value
    : contentContainer.value
  container?.scrollTo({ top: 0, behavior: 'smooth' })
}

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

  return showFirecrawlStatus.value
    || showSummaryStatus.value
    || showManualFirecrawlAction.value
    || showManualSummaryAction.value
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

function syncCurrentArticle(updates: Partial<Article>) {
  if (!props.article) return

  Object.assign(props.article, updates)

  const storeArticle = apiStore.articles.find(item => item.id === props.article?.id)
  if (storeArticle) {
    Object.assign(storeArticle, updates)
  }

  emit('articleUpdate', props.article.id, updates)
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
  } catch {
    liveStatus.value = null
  }
}

async function handleManualFirecrawl() {
  if (!props.article || manualFirecrawlLoading.value) return

  manualFirecrawlLoading.value = true
  manualActionError.value = null
  syncCurrentArticle({
    firecrawlStatus: 'processing',
    firecrawlError: undefined,
  })

  try {
    const response = await crawlArticle(Number(props.article.id))
    if (!response.success) {
      throw new Error(response.error || '手动抓取失败')
    }

    syncCurrentArticle({
      firecrawlStatus: response.data?.firecrawl_status === 'completed' ? 'completed' : props.article.firecrawlStatus,
      firecrawlContent: response.data?.firecrawl_content || props.article.firecrawlContent,
      firecrawlError: undefined,
      firecrawlCrawledAt: new Date().toISOString(),
    })

    await loadCompletionStatus(props.article.id)
    manualActionError.value = null
  } catch (error) {
    const message = error instanceof Error ? error.message : '手动抓取失败'
    manualActionError.value = message
    syncCurrentArticle({
      firecrawlStatus: 'failed',
      firecrawlError: message,
    })
  } finally {
    manualFirecrawlLoading.value = false
  }
}

async function handleManualSummary() {
  if (!props.article || manualSummaryLoading.value) return

  manualSummaryLoading.value = true
  manualActionError.value = null
  syncCurrentArticle({
    summaryStatus: 'pending',
    completionError: undefined,
  })

  try {
    await completeArticle(props.article.id, { force: true })
    await loadCompletionStatus(props.article.id)
    manualActionError.value = null
  } catch (error) {
    const message = error instanceof Error ? error.message : '手动总结失败'
    manualActionError.value = message
    syncCurrentArticle({
      summaryStatus: 'failed',
      completionError: message,
    })
  } finally {
    manualSummaryLoading.value = false
  }
}

async function handleManualTagging() {
  if (!props.article || manualTaggingLoading.value) return

  manualTaggingLoading.value = true
  taggingError.value = null

  try {
    const response = await articlesApi.retagArticle(Number(props.article.id))
    if (!response.success || !response.data) {
      throw new Error(response.error || '提交标签任务失败')
    }

    watchArticle(Number(props.article.id))
  } catch (error) {
    const message = error instanceof Error ? error.message : '提交标签任务失败'
    taggingError.value = message
    manualTaggingLoading.value = false
  }
}

async function handleTagWatchToggle(payload: { id: number; slug: string }) {
  if (!props.article?.tags) return
  if (watchPendingIds.value.has(payload.id)) return

  const tag = props.article.tags.find(t => t.id === payload.id)
  if (!tag) return

  const previousWatched = tag.isWatched
  const newWatched = !previousWatched

  tag.isWatched = newWatched
  watchPendingIds.value.add(payload.id)

  try {
    const response = newWatched
      ? await watchedTagsApi.watchTag(payload.id)
      : await watchedTagsApi.unwatchTag(payload.id)
    if (!response.success) {
      throw new Error(response.error || '操作失败')
    }
  } catch {
    tag.isWatched = previousWatched
  } finally {
    watchPendingIds.value.delete(payload.id)
  }
}

onTagResult((articleId: number, tags: ArticleTag[]) => {
  if (!props.article || String(articleId) !== props.article.id) return

  syncCurrentArticle({
    tags,
    tagCount: tags.length,
  })

  manualTaggingLoading.value = false
  taggingError.value = null
})

onTagError((articleId: number, error: string) => {
  if (!props.article || String(articleId) !== props.article.id) return

  taggingError.value = error
  manualTaggingLoading.value = false
})

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

  if (newId) {
    void loadCompletionStatus(newId)
  }
}, { immediate: true })

watch(() => props.article, (newArticle) => {
  if (!newArticle) return

  useHead({
    title: `${newArticle.title} - Syntopica`,
    meta: [
      { name: 'description', content: newArticle.description },
    ],
  })
})

function handleFavorite() {
  if (!props.article) return

  const isFavorite = !props.article.favorite
  emit('favorite', props.article.id)
  trackEvent(isFavorite ? 'favorite' : 'unfavorite', lastScrollDepth, readingTime.value)
  uploadEvents()
}

function openOriginal() {
  if (props.article?.link) {
    window.open(props.article.link, '_blank')
  }
}

function toggleViewMode() {
  viewMode.value = viewMode.value === 'preview' ? 'iframe' : 'preview'
  if (viewMode.value === 'iframe') {
    iframeLoading.value = true
  }
}

function toggleFullscreen() {
  isFullscreen.value = !isFullscreen.value
}

function handleIframeLoad() {
  iframeLoading.value = false
}

function handleIframeError() {
  iframeLoading.value = false
}

function navigatePrev() {
  if (!hasPrev.value || !props.article) return

  const previousArticle = props.articles[currentIndex.value - 1]
  if (previousArticle) {
    emit('navigate', previousArticle)
  }
}

function navigateNext() {
  if (!hasNext.value || !props.article) return

  const nextArticle = props.articles[currentIndex.value + 1]
  if (nextArticle) {
    emit('navigate', nextArticle)
  }
}

const aiEnabled = isAIEnabled

function renderMarkdown(content?: string | null) {
  if (!content) return ''
  return marked.parse(content) as string
}

const renderedStoredSummary = computed(() => renderMarkdown(mergedArticle.value?.aiContentSummary))

const contentSources = computed(() => getArticleContentSources({
  firecrawlContent: mergedArticle.value?.firecrawlContent,
  content: mergedArticle.value?.content,
}))

const availableContentSources = computed(() => contentSources.value.available)

const activeContentSource = computed<ArticleContentSource | null>(() => {
  if (availableContentSources.value.includes(selectedContentSource.value)) {
    return selectedContentSource.value
  }

  return contentSources.value.defaultSource
})

const showContentSourceToggle = computed(() => availableContentSources.value.length > 1)

const displayContent = computed(() => {
  if (!mergedArticle.value) return ''

  const resolvedContent = resolveArticleContentBySource(contentSources.value, activeContentSource.value ?? undefined)
  if (!resolvedContent) {
    return ''
  }

  let content: string
  if (activeContentSource.value === 'firecrawl') {
    content = renderMarkdown(resolvedContent)
  } else {
    content = resolvedContent
  }

  return content.replace(/<img[^>]*src=["']Base64-Image-Removed["'][^>]*\/?>/gi, '')
})

const showDescription = computed(() => {
  if (!mergedArticle.value) return false

  return shouldShowArticleDescription(mergedArticle.value.description, displayContent.value)
})

const detailLines = computed(() => {
  if (!mergedArticle.value) return []

  const lines: string[] = []
  if (mergedArticle.value.firecrawlCrawledAt) {
    lines.push(`抓取时间：${$dayjs(mergedArticle.value.firecrawlCrawledAt).format('YYYY-MM-DD HH:mm')}`)
  }
  if (mergedArticle.value.summaryGeneratedAt) {
    lines.push(`总结时间：${$dayjs(mergedArticle.value.summaryGeneratedAt).format('YYYY-MM-DD HH:mm')}`)
  }
  if ((mergedArticle.value.completionAttempts ?? 0) > 0) {
    lines.push(`总结尝试：${mergedArticle.value.completionAttempts} 次`)
  }
  return lines
})

import '~/components/article/ArticleContent.css'
</script>

<template>
  <div v-if="!article" class="h-full flex items-center justify-center bg-white">
    <div class="text-center">
      <div>
        <img src="/favicon.png" alt="Syntopica" width="360" height="360" class="mx-auto mb-4" />
      </div>
      <h3 class="mb-2 text-xl font-semibold text-gray-700">选择一篇文章开始阅读</h3>
      <p class="text-gray-500">点击左侧文章列表查看内容</p>
    </div>
  </div>

  <div v-else-if="!isFullscreen" class="article-content h-full flex flex-col relative">
    <header class="article-header">
      <div class="header-left">
        <div v-if="feed" class="feed-badge">
          <Icon :icon="feed.icon || 'mdi:rss'" :style="{ color: feed.color }" width="16" height="16" />
          <span class="text-sm font-medium" :style="{ color: feed.color }">{{ feed.title }}</span>
        </div>
        <span class="article-title">{{ article.title }}</span>
      </div>

      <div class="header-actions">
        <template v-if="articles.length > 1">
          <button class="action-btn" :class="{ 'opacity-30 cursor-not-allowed': !hasPrev }" :disabled="!hasPrev" title="上一篇文章" @click="navigatePrev">
            <Icon icon="mdi:chevron-up" width="20" height="20" />
          </button>
          <button class="action-btn" :class="{ 'opacity-30 cursor-not-allowed': !hasNext }" :disabled="!hasNext" title="下一篇文章" @click="navigateNext">
            <Icon icon="mdi:chevron-down" width="20" height="20" />
          </button>
          <div class="mx-1 h-5 w-px bg-ink-200" />
        </template>

        <button class="action-btn" :title="viewMode === 'preview' ? '切换到内嵌网页' : '切换到内容预览'" @click="toggleViewMode">
          <Icon :icon="viewMode === 'preview' ? 'mdi:web' : 'mdi:file-document-outline'" width="20" height="20" />
        </button>

        <button class="action-btn" :class="{ active: article.favorite }" :title="article.favorite ? '取消收藏' : '收藏'" @click="handleFavorite">
          <Icon :icon="article.favorite ? 'mdi:star' : 'mdi:star-outline'" width="20" height="20" />
        </button>

        <button class="action-btn" title="全屏" @click="toggleFullscreen">
          <Icon icon="mdi:fullscreen" width="20" height="20" />
        </button>

        <button class="action-btn" title="在新窗口打开原文" @click="openOriginal">
          <Icon icon="mdi:external-link" width="20" height="20" />
        </button>
      </div>
    </header>

    <div v-if="viewMode === 'preview'" class="reading-progress-bar">
      <div class="reading-progress-bar__fill" :style="{ width: `${scrollProgress}%` }" />
    </div>

    <div v-if="viewMode === 'preview'" ref="contentContainer" class="preview-mode flex-1 overflow-y-auto" @scroll="onContentScroll">
      <div v-if="showProcessingPanel && mergedArticle && firecrawlMeta && summaryMeta" class="mb-6 rounded-2xl border border-ink-200 bg-white/80 p-4 shadow-subtle">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="flex flex-wrap items-center gap-2">
            <span v-if="showFirecrawlStatus" class="inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs font-semibold" :class="getStatusToneClasses(firecrawlMeta.tone)">
              <Icon :icon="firecrawlMeta.icon" width="14" height="14" :class="{ 'animate-spin': mergedArticle.firecrawlStatus === 'processing' }" />
              {{ firecrawlMeta.label }}
            </span>
            <span v-if="showSummaryStatus" class="inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs font-semibold" :class="getStatusToneClasses(summaryMeta.tone)">
              <Icon :icon="summaryMeta.icon" width="14" height="14" :class="{ 'animate-spin': mergedArticle.summaryStatus === 'pending' }" />
              {{ summaryMeta.label }}
            </span>
          </div>

          <div class="flex flex-wrap items-center gap-2">
            <button
              v-if="showManualFirecrawlAction"
              class="inline-flex items-center gap-1.5 rounded-full border border-ink-200 bg-white px-3 py-1.5 text-xs font-semibold text-ink-700 transition hover:border-ink-300 hover:text-ink-900 disabled:cursor-not-allowed disabled:opacity-50"
              :disabled="actionBusy"
              @click="handleManualFirecrawl"
            >
              <Icon icon="mdi:web-sync" width="14" height="14" :class="{ 'animate-spin': manualFirecrawlLoading }" />
              {{ manualFirecrawlLabel }}
            </button>
            <button
              v-if="showManualSummaryAction"
              class="inline-flex items-center gap-1.5 rounded-full border border-ink-200 bg-white px-3 py-1.5 text-xs font-semibold text-ink-700 transition hover:border-ink-300 hover:text-ink-900 disabled:cursor-not-allowed disabled:opacity-50"
              :disabled="actionBusy"
              @click="handleManualSummary"
            >
              <Icon icon="mdi:brain" width="14" height="14" :class="{ 'animate-spin': manualSummaryLoading }" />
              {{ manualSummaryLabel }}
            </button>
          </div>
        </div>

        <div v-if="detailLines.length" class="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-ink-medium">
          <span v-for="line in detailLines" :key="line">{{ line }}</span>
        </div>

        <div v-if="manualActionError || mergedArticle.firecrawlError || mergedArticle.completionError" class="mt-3 rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
          {{ manualActionError || mergedArticle.completionError || mergedArticle.firecrawlError }}
        </div>
      </div>

      <div v-if="showContentSourceToggle" class="mb-6 flex items-center gap-2 rounded-2xl border border-ink-200 bg-white/80 p-2 shadow-subtle">
        <button
          class="rounded-xl px-3 py-2 text-xs font-semibold transition"
          :class="activeContentSource === 'original' ? 'bg-ink-900 text-white' : 'text-ink-600 hover:bg-ink-100'"
          @click="selectedContentSource = 'original'"
        >
          原始内容
        </button>
        <button
          class="rounded-xl px-3 py-2 text-xs font-semibold transition"
          :class="activeContentSource === 'firecrawl' ? 'bg-ink-900 text-white' : 'text-ink-600 hover:bg-ink-100'"
          @click="selectedContentSource = 'firecrawl'"
        >
          Firecrawl 全文
        </button>
      </div>

      <div v-if="mergedArticle?.aiContentSummary" class="mb-6 rounded-2xl border border-ink-200 bg-white/80 p-5 shadow-subtle">
        <div class="mb-3 flex items-center gap-2 text-ink-700">
          <Icon icon="mdi:brain" width="18" height="18" />
          <span class="text-sm font-semibold">AI 整理稿</span>
        </div>
        <ArticleTagList
          v-if="article.tags?.length"
          class="mb-3"
          :tags="article.tags"
          :highlighted-slugs="highlightedTagSlugs"
          compact
          :show-article-count="false"
          show-watch
          @watch-toggle="handleTagWatchToggle"
        />
        <div class="summary-surface">
          <div class="markdown-body markdown-summary" v-html="renderedStoredSummary" />
        </div>
      </div>

      <div class="article-meta">
        <span>{{ $dayjs(article.pubDate).format('YYYY年MM月DD日 HH:mm') }}</span>
        <span v-if="article.author">作者：{{ article.author }}</span>
        <span v-if="article.read" class="read-badge">
          <Icon icon="mdi:check-circle" width="14" height="14" />
          已读
        </span>
      </div>

      <h1 class="article-title-full">{{ article.title }}</h1>

      <div class="mb-4 flex flex-wrap items-center gap-2">
        <ArticleTagList
          v-if="article.tags?.length"
          :tags="article.tags"
          :highlighted-slugs="highlightedTagSlugs"
          compact
          show-watch
          @watch-toggle="handleTagWatchToggle"
        />
        <span v-if="manualTaggingLoading" class="inline-flex items-center gap-1 text-xs text-ink-medium">
          <Icon icon="mdi:loading" width="14" height="14" class="animate-spin" />
          正在生成标签...
        </span>
        <button
          v-else-if="aiEnabled"
          class="inline-flex items-center gap-1 rounded-full border border-dashed border-ink-200 bg-white/80 px-2.5 py-1 text-xs font-medium text-ink-medium transition hover:border-ink-300 hover:text-ink-900 disabled:cursor-not-allowed disabled:opacity-50"
          :disabled="actionBusy"
          @click="handleManualTagging"
        >
          <Icon icon="mdi:tag-plus-outline" width="14" height="14" />
          {{ manualTaggingLabel }}
        </button>
      </div>

      <div v-if="taggingError" class="mb-2 rounded-lg border border-rose-200 bg-rose-50 px-3 py-1.5 text-xs text-rose-700">
        {{ taggingError }}
      </div>

      <div v-if="showDescription" class="article-description">
        <div v-html="article.description" />
      </div>

      <div v-if="article.imageUrl" class="article-image">
        <img :src="article.imageUrl" :alt="article.title" class="w-full">
      </div>

<div class="article-body">
        <div v-if="displayContent" class="markdown-body markdown-article" v-html="displayContent" />
        <div v-else class="empty-content">
          <button class="btn btn-primary mt-4" @click="openOriginal">前往原文阅读</button>
        </div>
      </div>
    </div>

    <div v-else class="iframe-mode flex-1 relative">
      <div v-if="iframeLoading" class="iframe-loading">
        <div class="text-center">
          <Icon icon="mdi:loading" width="48" height="48" class="animate-spin text-blue-600 mx-auto mb-4" />
          <p class="text-gray-600">正在加载网页...</p>
        </div>
      </div>

      <iframe
        v-if="article.link"
        :src="article.link"
        class="w-full h-full border-0"
        title="Article Content"
        sandbox="allow-same-origin allow-scripts allow-forms allow-popups"
        @load="handleIframeLoad"
        @error="handleIframeError"
      />

      <div v-else class="iframe-error">
        <div class="text-center text-gray-400">
          <Icon icon="mdi:web-off" width="64" height="64" class="mb-4 mx-auto" />
          <p>无法加载网页</p>
          <button class="btn btn-primary mt-4" @click="openOriginal">在新窗口打开</button>
        </div>
      </div>
    </div>

    <button
      v-if="viewMode === 'preview'"
      class="back-to-top-btn"
      :class="{ 'back-to-top-btn--visible': showBackTop }"
      @click="scrollToTop"
    >
      <Icon icon="mdi:chevron-up" width="20" height="20" />
    </button>
  </div>

  <Teleport v-else to="body">
    <div class="fullscreen-article fixed inset-0 z-[100] bg-white flex flex-col">
      <header class="article-header">
        <div class="header-left">
          <button class="flex items-center gap-1 rounded-lg p-2 text-ink-medium transition-all duration-200 hover:bg-ink-50 hover:text-ink-dark" @click="toggleFullscreen">
            <Icon icon="mdi:arrow-left" width="20" height="20" />
            <span class="text-sm">退出全屏</span>
          </button>
          <div v-if="feed" class="feed-badge">
            <Icon :icon="feed.icon || 'mdi:rss'" :style="{ color: feed.color }" width="16" height="16" />
            <span class="text-sm font-medium" :style="{ color: feed.color }">{{ feed.title }}</span>
          </div>
        </div>

        <div class="header-actions">
          <template v-if="articles.length > 1">
            <button class="action-btn" :class="{ 'opacity-30 cursor-not-allowed': !hasPrev }" :disabled="!hasPrev" title="上一篇文章" @click="navigatePrev">
              <Icon icon="mdi:chevron-up" width="20" height="20" />
            </button>
            <button class="action-btn" :class="{ 'opacity-30 cursor-not-allowed': !hasNext }" :disabled="!hasNext" title="下一篇文章" @click="navigateNext">
              <Icon icon="mdi:chevron-down" width="20" height="20" />
            </button>
            <div class="mx-1 h-5 w-px bg-ink-200" />
          </template>

          <button class="action-btn" :title="viewMode === 'preview' ? '切换到内嵌网页' : '切换到内容预览'" @click="toggleViewMode">
            <Icon :icon="viewMode === 'preview' ? 'mdi:web' : 'mdi:file-document-outline'" width="20" height="20" />
          </button>

          <button class="action-btn" :class="{ active: article.favorite }" :title="article.favorite ? '取消收藏' : '收藏'" @click="handleFavorite">
            <Icon :icon="article.favorite ? 'mdi:star' : 'mdi:star-outline'" width="20" height="20" />
          </button>

          <button class="action-btn" title="退出全屏" @click="toggleFullscreen">
            <Icon icon="mdi:fullscreen-exit" width="20" height="20" />
          </button>

          <button class="action-btn" title="在新窗口打开原文" @click="openOriginal">
            <Icon icon="mdi:external-link" width="20" height="20" />
          </button>
        </div>
      </header>

      <div v-if="viewMode === 'preview'" class="reading-progress-bar">
        <div class="reading-progress-bar__fill" :style="{ width: `${scrollProgress}%` }" />
      </div>

      <div v-if="viewMode === 'preview'" ref="fullscreenContentContainer" class="preview-mode flex-1 overflow-y-auto" @scroll="onContentScroll">
        <div v-if="showProcessingPanel && mergedArticle && firecrawlMeta && summaryMeta" class="mb-6 rounded-2xl border border-ink-200 bg-white/80 p-4 shadow-subtle">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div class="flex flex-wrap items-center gap-2">
              <span v-if="showFirecrawlStatus" class="inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs font-semibold" :class="getStatusToneClasses(firecrawlMeta.tone)">
                <Icon :icon="firecrawlMeta.icon" width="14" height="14" :class="{ 'animate-spin': mergedArticle.firecrawlStatus === 'processing' }" />
                {{ firecrawlMeta.label }}
              </span>
              <span v-if="showSummaryStatus" class="inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs font-semibold" :class="getStatusToneClasses(summaryMeta.tone)">
                <Icon :icon="summaryMeta.icon" width="14" height="14" :class="{ 'animate-spin': mergedArticle.summaryStatus === 'pending' }" />
                {{ summaryMeta.label }}
              </span>
            </div>

            <div class="flex flex-wrap items-center gap-2">
              <button
                v-if="showManualFirecrawlAction"
                class="inline-flex items-center gap-1.5 rounded-full border border-ink-200 bg-white px-3 py-1.5 text-xs font-semibold text-ink-700 transition hover:border-ink-300 hover:text-ink-900 disabled:cursor-not-allowed disabled:opacity-50"
                :disabled="actionBusy"
                @click="handleManualFirecrawl"
              >
                <Icon icon="mdi:web-sync" width="14" height="14" :class="{ 'animate-spin': manualFirecrawlLoading }" />
                {{ manualFirecrawlLabel }}
              </button>
              <button
                v-if="showManualSummaryAction"
                class="inline-flex items-center gap-1.5 rounded-full border border-ink-200 bg-white px-3 py-1.5 text-xs font-semibold text-ink-700 transition hover:border-ink-300 hover:text-ink-900 disabled:cursor-not-allowed disabled:opacity-50"
                :disabled="actionBusy"
                @click="handleManualSummary"
              >
                <Icon icon="mdi:brain" width="14" height="14" :class="{ 'animate-spin': manualSummaryLoading }" />
                {{ manualSummaryLabel }}
              </button>
            </div>
          </div>

          <div v-if="detailLines.length" class="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-ink-medium">
            <span v-for="line in detailLines" :key="line">{{ line }}</span>
          </div>

          <div v-if="manualActionError || mergedArticle.firecrawlError || mergedArticle.completionError" class="mt-3 rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
            {{ manualActionError || mergedArticle.completionError || mergedArticle.firecrawlError }}
          </div>
        </div>

        <div v-if="showContentSourceToggle" class="mb-6 flex items-center gap-2 rounded-2xl border border-ink-200 bg-white/80 p-2 shadow-subtle">
          <button
            class="rounded-xl px-3 py-2 text-xs font-semibold transition"
            :class="activeContentSource === 'original' ? 'bg-ink-900 text-white' : 'text-ink-600 hover:bg-ink-100'"
            @click="selectedContentSource = 'original'"
          >
            原始内容
          </button>
          <button
            class="rounded-xl px-3 py-2 text-xs font-semibold transition"
            :class="activeContentSource === 'firecrawl' ? 'bg-ink-900 text-white' : 'text-ink-600 hover:bg-ink-100'"
            @click="selectedContentSource = 'firecrawl'"
          >
            Firecrawl 全文
          </button>
        </div>

        <div v-if="mergedArticle?.aiContentSummary" class="mb-6 rounded-2xl border border-ink-200 bg-white/80 p-5 shadow-subtle">
          <div class="mb-3 flex items-center gap-2 text-ink-700">
            <Icon icon="mdi:brain" width="18" height="18" />
            <span class="text-sm font-semibold">AI 整理稿</span>
          </div>
          <ArticleTagList
            v-if="article.tags?.length"
            class="mb-3"
            :tags="article.tags"
            :highlighted-slugs="highlightedTagSlugs"
            compact
            :show-article-count="false"
            show-watch
            @watch-toggle="handleTagWatchToggle"
          />
          <div class="summary-surface">
          <div class="markdown-body markdown-summary" v-html="renderedStoredSummary" />
        </div>
        </div>

        <div class="article-meta">
          <span>{{ $dayjs(article.pubDate).format('YYYY年MM月DD日 HH:mm') }}</span>
          <span v-if="article.author">作者：{{ article.author }}</span>
          <span v-if="article.read" class="read-badge">
            <Icon icon="mdi:check-circle" width="14" height="14" />
            已读
          </span>
        </div>

        <h1 class="article-title-full">{{ article.title }}</h1>

        <div class="mb-4 flex flex-wrap items-center gap-2">
          <ArticleTagList
            v-if="article.tags?.length"
            :tags="article.tags"
            :highlighted-slugs="highlightedTagSlugs"
            compact
            show-watch
            @watch-toggle="handleTagWatchToggle"
          />
          <span v-if="manualTaggingLoading" class="inline-flex items-center gap-1 text-xs text-ink-medium">
            <Icon icon="mdi:loading" width="14" height="14" class="animate-spin" />
            正在生成标签...
          </span>
          <button
            v-else-if="aiEnabled"
            class="inline-flex items-center gap-1 rounded-full border border-dashed border-ink-200 bg-white/80 px-2.5 py-1 text-xs font-medium text-ink-medium transition hover:border-ink-300 hover:text-ink-900 disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="actionBusy"
            @click="handleManualTagging"
          >
            <Icon icon="mdi:tag-plus-outline" width="14" height="14" />
            {{ manualTaggingLabel }}
          </button>
        </div>

        <div v-if="taggingError" class="mb-2 rounded-lg border border-rose-200 bg-rose-50 px-3 py-1.5 text-xs text-rose-700">
          {{ taggingError }}
        </div>

        <div v-if="showDescription" class="article-description">
          <div v-html="article.description" />
        </div>

        <div v-if="article.imageUrl" class="article-image">
          <img :src="article.imageUrl" :alt="article.title" class="w-full">
        </div>

<div class="article-body">
          <div v-if="displayContent" class="markdown-body markdown-article" v-html="displayContent" />
          <div v-else class="empty-content">
            <button class="btn btn-primary mt-4" @click="openOriginal">前往原文阅读</button>
          </div>
        </div>
      </div>

      <div v-else class="iframe-mode flex-1 relative">
        <div v-if="iframeLoading" class="iframe-loading">
          <div class="text-center">
            <Icon icon="mdi:loading" width="48" height="48" class="animate-spin text-blue-600 mx-auto mb-4" />
            <p class="text-gray-600">正在加载网页...</p>
          </div>
        </div>

        <iframe
          v-if="article.link"
          :src="article.link"
          class="w-full h-full border-0"
          title="Article Content"
          sandbox="allow-same-origin allow-scripts allow-forms allow-popups"
          @load="handleIframeLoad"
          @error="handleIframeError"
        />

        <div v-else class="iframe-error">
          <div class="text-center text-gray-400">
            <Icon icon="mdi:web-off" width="64" height="64" class="mb-4 mx-auto" />
            <p>无法加载网页</p>
            <button class="btn btn-primary mt-4" @click="openOriginal">在新窗口打开</button>
          </div>
        </div>
      </div>

      <button
        v-if="viewMode === 'preview'"
        class="back-to-top-btn"
        :class="{ 'back-to-top-btn--visible': showBackTop }"
        @click="scrollToTop"
      >
        <Icon icon="mdi:chevron-up" width="20" height="20" />
      </button>
    </div>
  </Teleport>
</template>

<style scoped>
.fullscreen-article {
  background: white;
}

.fullscreen-article .article-header {
  flex-shrink: 0;
}

.fullscreen-article .preview-mode {
  flex: 1;
  overflow-y: auto;
}

.fullscreen-article .iframe-mode {
  flex: 1;
}

.reading-progress-bar {
  height: 2px;
  background: rgba(26, 26, 26, 0.06);
  flex-shrink: 0;
}

.reading-progress-bar__fill {
  height: 100%;
  background: var(--color-ink-500);
  transition: width 0.15s ease-out;
  border-radius: 0 1px 1px 0;
}

.back-to-top-btn {
  position: absolute;
  bottom: 24px;
  right: 24px;
  width: 40px;
  height: 40px;
  border-radius: 0.75rem;
  background: var(--color-ink-500);
  color: white;
  border: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: var(--shadow-medium);
  opacity: 0;
  transform: translateY(8px);
  pointer-events: none;
  transition: opacity 0.25s ease, transform 0.25s ease, background-color 0.2s ease;
  z-index: 10;
}

.back-to-top-btn:hover {
  background: var(--color-ink-600);
  box-shadow: var(--shadow-strong);
}

.back-to-top-btn--visible {
  opacity: 1;
  transform: translateY(0);
  pointer-events: auto;
}
</style>


