<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { Article } from '~/types'
import { useNuxtApp } from '#app'
import { useAI } from '~/composables/useAI'
import { useArticleContentView } from '~/features/articles/composables/useArticleContentView'
import type { ArticleStatusMenuProps } from './ArticleStatusMenu.vue'
import ArticleContentToolbar from './ArticleContentToolbar.vue'
import ArticleContentPreviewPanel from './ArticleContentPreviewPanel.vue'
import ArticleIframeView from './ArticleIframeView.vue'
import '~/components/article/ArticleContent.css'

const { $dayjs } = useNuxtApp()
const { isAIEnabled } = useAI()

interface Props {
  article: Article | null
  articles?: Article[]
  highlightedTagSlugs?: string[]
}

const props = withDefaults(defineProps<Props>(), {
  article: null,
  articles: () => [],
  highlightedTagSlugs: () => [],
})

const emit = defineEmits<{
  favorite: [id: string]
  navigate: [article: Article]
  articleUpdate: [id: string, updates: Partial<Article>]
}>()

const {
  viewMode, iframeLoading, isFullscreen,
  selectedContentSource,
  manualFirecrawlLoading, manualSummaryLoading, manualTaggingLoading,
  manualActionError, taggingError,
  scrollProgress, scrollTop, contentContainer, fullscreenContentContainer,
  showBackTop,
  feed, hasPrev, hasNext, mergedArticle, currentIndex,
  actionBusy,
  manualFirecrawlLabel, manualSummaryLabel, manualTaggingLabel,
  showContentSourceToggle, displayContent, showDescription,
  detailLines, renderedStoredSummary,
  activeContentSource,
  onContentScroll, scrollToTop,
  handleManualFirecrawl, handleManualSummary, handleManualTagging,
  handleTagWatchToggle, openOriginal, toggleViewMode, toggleFullscreen,
  handleIframeLoad, handleIframeError,
  readingTime, trackEvent, uploadEvents, lastScrollDepth,
} = useArticleContentView(props)

// 上一篇/下一篇：composable 里的 navigatePrev/Next 是历史空转（选中职责在 shell），
// 此处按当前索引真实 emit('navigate')，由 FeedLayoutShell 的 handleArticleClick 接住
function handleNavigatePrev() {
  const list = props.articles
  if (!hasPrev.value || !props.article || !list?.length) return
  const prev = list[currentIndex.value - 1]
  if (prev) emit('navigate', prev)
}

function handleNavigateNext() {
  const list = props.articles
  if (!hasNext.value || !props.article || !list?.length) return
  const next = list[currentIndex.value + 1]
  if (next) emit('navigate', next)
}

function handleFavorite() {
  if (!props.article) return
  emit('favorite', props.article.id)
  trackEvent(props.article.favorite ? 'unfavorite' : 'favorite', lastScrollDepth, readingTime.value)
  uploadEvents()
}

const hasMultipleArticles = computed(() => (props.articles?.length ?? 0) > 1)

function setContentSource(source: string) {
  selectedContentSource.value = source as any
}

/** 工具栏状态/菜单数据（design D3：从 composable 直取，不经 PreviewPanel 转发） */
const statusMenuProps = computed<ArticleStatusMenuProps>(() => ({
  article: mergedArticle.value,
  showManualFirecrawlAction: Boolean(feed.value?.firecrawlEnabled),
  showManualSummaryAction: Boolean(feed.value?.articleSummaryEnabled),
  showManualTaggingAction: Boolean(isAIEnabled.value),
  actionBusy: Boolean(actionBusy.value),
  manualFirecrawlLoading: Boolean(manualFirecrawlLoading.value),
  manualSummaryLoading: Boolean(manualSummaryLoading.value),
  manualTaggingLoading: Boolean(manualTaggingLoading.value),
  manualFirecrawlLabel: manualFirecrawlLabel.value,
  manualSummaryLabel: manualSummaryLabel.value,
  manualTaggingLabel: manualTaggingLabel.value,
  manualActionError: manualActionError.value,
  taggingError: taggingError.value,
  detailLines: detailLines.value,
  showContentSourceToggle: Boolean(showContentSourceToggle.value),
  activeContentSource: (activeContentSource.value ?? 'original') as string,
}))

const previewProps = computed(() => ({
  article: props.article,
  highlightedTagSlugs: props.highlightedTagSlugs,
  aiEnabled: Boolean(isAIEnabled.value),
  kickerLabel: feed.value?.title ?? '',
  renderedStoredSummary: renderedStoredSummary.value,
  manualTaggingLoading: Boolean(manualTaggingLoading.value),
  manualTaggingLabel: manualTaggingLabel.value,
  taggingError: taggingError.value,
  actionBusy: Boolean(actionBusy.value),
  showDescription: Boolean(showDescription.value),
  displayContent: displayContent.value,
  articleImageUrl: props.article?.imageUrl ?? null,
  articlePubDate: props.article?.pubDate ?? '',
  articleAuthor: props.article?.author ?? null,
  articleRead: Boolean(props.article?.read),
  articleTitleFull: props.article?.title ?? '',
  handleManualTagging: handleManualTagging,
  handleTagWatchToggle: handleTagWatchToggle,
  openOriginal: openOriginal,
}))
</script>

<template>
  <div v-if="!article" class="h-full flex items-center justify-center" style="background: var(--color-bg-base)">
    <div class="text-center">
      <div>
        <img src="/favicon.png" alt="Syntopica" width="360" height="360" class="mx-auto mb-4" />
      </div>
      <h3 class="mb-2 text-xl font-semibold" style="color: var(--color-text-primary)">选择一篇文章开始阅读</h3>
      <p style="color: var(--color-text-secondary)">点击左侧文章列表查看内容</p>
    </div>
  </div>

  <!-- Normal mode -->
  <div v-else-if="!isFullscreen" class="article-content h-full flex flex-col relative">
    <ArticleContentToolbar
      :feed="feed" :view-mode="viewMode" :has-prev="hasPrev" :has-next="hasNext"
      :article-title="article.title ?? ''" :article-favorite="article.favorite ?? false"
      :show-back-button="false" :show-nav-buttons="hasMultipleArticles"
      :status-menu="statusMenuProps"
      @toggle-favorite="handleFavorite" @toggle-view-mode="toggleViewMode"
      @toggle-fullscreen="toggleFullscreen" @navigate-prev="handleNavigatePrev"
      @navigate-next="handleNavigateNext" @open-original="openOriginal"
      @manual-firecrawl="handleManualFirecrawl" @manual-summary="handleManualSummary"
      @manual-tagging="handleManualTagging" @set-content-source="setContentSource"
    />

    <div v-if="viewMode === 'preview'" class="reading-progress-bar">
      <div class="reading-progress-bar__fill" :style="{ width: `${scrollProgress}%` }" />
    </div>

    <div v-if="viewMode === 'preview'" ref="contentContainer"
      class="preview-mode flex-1 overflow-y-auto" @scroll="onContentScroll">
      <ArticleContentPreviewPanel v-bind="previewProps" />
    </div>

    <ArticleIframeView v-else :src="article.link ?? null"
      :loading="iframeLoading" @iframe-load="handleIframeLoad"
      @iframe-error="handleIframeError" @open-original="openOriginal" />

    <button v-if="viewMode === 'preview'"
      class="back-to-top-btn" :class="{ 'back-to-top-btn--visible': showBackTop }"
      @click="scrollToTop">
      <Icon icon="mdi:chevron-up" width="20" height="20" />
    </button>
  </div>

  <!-- Fullscreen mode -->
  <Teleport v-else to="body">
    <div class="fullscreen-article fixed inset-0 z-[100] flex flex-col" style="background: var(--color-bg-base)">
      <ArticleContentToolbar
        :feed="feed"
        :article-title="article.title ?? ''" :article-favorite="article.favorite ?? false"
        :view-mode="viewMode" :has-prev="hasPrev" :has-next="hasNext"
        :show-back-button="true" :show-nav-buttons="hasMultipleArticles"
        :status-menu="statusMenuProps"
        @toggle-favorite="handleFavorite" @toggle-view-mode="toggleViewMode"
        @toggle-fullscreen="toggleFullscreen" @navigate-prev="handleNavigatePrev"
        @navigate-next="handleNavigateNext" @open-original="openOriginal"
        @manual-firecrawl="handleManualFirecrawl" @manual-summary="handleManualSummary"
        @manual-tagging="handleManualTagging" @set-content-source="setContentSource"
      />

      <div v-if="viewMode === 'preview'" class="reading-progress-bar">
        <div class="reading-progress-bar__fill" :style="{ width: `${scrollProgress}%` }" />
      </div>

      <div v-if="viewMode === 'preview'" ref="fullscreenContentContainer"
        class="preview-mode flex-1 overflow-y-auto" @scroll="onContentScroll">
        <ArticleContentPreviewPanel v-bind="previewProps" />
      </div>

      <ArticleIframeView v-else :src="article.link ?? null"
        :loading="iframeLoading" @iframe-load="handleIframeLoad"
        @iframe-error="handleIframeError" @open-original="openOriginal" />

      <button v-if="viewMode === 'preview'"
        class="back-to-top-btn" :class="{ 'back-to-top-btn--visible': showBackTop }"
        @click="scrollToTop">
        <Icon icon="mdi:chevron-up" width="20" height="20" />
      </button>
    </div>
  </Teleport>
</template>
