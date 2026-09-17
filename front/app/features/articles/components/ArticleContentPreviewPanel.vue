<script setup lang="ts">
import { Icon } from '@iconify/vue'
import dayjs from 'dayjs'
import type { Article } from '~/types'
import ArticleTagList from '~/features/articles/components/ArticleTagList.vue'
import '~/components/article/ArticleContent.css'

defineOptions({ inheritAttrs: false })

// 处理状态横幅/手动按钮行/内容源切换卡已迁入 ArticleStatusMenu（redesign-reading-pane design D3/D4），
// 本面板只保留阅读信息架构：kicker → 元信息 → 标题 → 标签 → 导语 → AI 整理稿 → 正文。
const props = withDefaults(defineProps<{
  article: Article | null
  highlightedTagSlugs: string[]
  aiEnabled: boolean
  /** 标题上方红色 kicker 小标文案（feed 名/栏目名） */
  kickerLabel: string
  renderedStoredSummary: string
  manualTaggingLoading: boolean
  manualTaggingLabel: string
  taggingError: string | null
  actionBusy: boolean
  showDescription: boolean
  displayContent: string
  articleImageUrl: string | null
  articlePubDate: string
  articleAuthor: string | null
  articleRead: boolean
  articleTitleFull: string

  handleManualTagging: () => void
  handleTagWatchToggle: (payload: { id: number; slug: string }) => void
  openOriginal: () => void
}>(), {
  article: null,
  highlightedTagSlugs: () => [],
  aiEnabled: false,
  kickerLabel: '',
  renderedStoredSummary: '',
  manualTaggingLoading: false,
  manualTaggingLabel: '',
  taggingError: null,
  actionBusy: false,
  showDescription: false,
  displayContent: '',
  articleImageUrl: null,
  articlePubDate: '',
  articleAuthor: null,
  articleRead: false,
  articleTitleFull: '',
  handleManualTagging: () => {},
  handleTagWatchToggle: () => {},
  openOriginal: () => {},
})
</script>

<template>
  <div class="reading-col">
    <!-- Kicker：红色小标 + 细线（编辑签名元素） -->
    <div class="kicker">
      <span class="kicker-label">{{ kickerLabel }}</span>
      <span class="kicker-rule" />
    </div>

    <!-- Meta -->
    <div class="article-meta">
      <span>{{ dayjs(articlePubDate).format('YYYY年MM月DD日 HH:mm') }}</span>
      <span v-if="articleAuthor">作者：{{ articleAuthor }}</span>
      <span v-if="articleRead" class="read-badge">
        <Icon icon="mdi:check-circle" width="14" height="14" />
        已读
      </span>
    </div>

    <!-- Title -->
    <h1 class="article-title-full">{{ articleTitleFull }}</h1>

    <!-- Tags & Manual Tagging -->
    <div class="tag-row">
      <ArticleTagList
        v-if="article?.tags?.length"
        :tags="article.tags"
        :highlighted-slugs="highlightedTagSlugs"
        compact
        show-watch
        @watch-toggle="handleTagWatchToggle"
      />
      <span v-if="manualTaggingLoading" class="inline-flex items-center gap-1 text-xs text-[var(--color-text-secondary)]">
        <Icon icon="mdi:loading" width="14" height="14" class="animate-spin" />
        正在生成标签...
      </span>
      <button
        v-else-if="aiEnabled"
        class="inline-flex items-center gap-1 rounded-full border border-dashed border-[var(--color-border-subtle)] px-2.5 py-1 text-xs font-medium text-[var(--color-text-secondary)] transition hover:border-[var(--color-border-medium)] hover:text-[var(--color-text-primary)] disabled:cursor-not-allowed disabled:opacity-50"
        style="background: var(--color-bg-elevated)"
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

    <!-- 导语：description 有实质内容时，无边框浅色段（guard 收紧后无占位块） -->
    <div v-if="showDescription" class="lede">
      <div v-html="article?.description" />
    </div>

    <!-- Image -->
    <div v-if="articleImageUrl" class="article-image">
      <img :src="articleImageUrl" :alt="articleTitleFull" class="w-full">
    </div>

    <!-- AI 整理稿：无卡框小节（无摘要时整块不渲染） -->
    <div v-if="renderedStoredSummary" class="ai-block">
      <div class="ai-head">
        <Icon icon="mdi:brain" width="13" height="13" />
        <span>AI 整理稿</span>
      </div>
      <ArticleTagList
        v-if="article?.tags?.length"
        class="mb-3"
        :tags="article.tags"
        :highlighted-slugs="highlightedTagSlugs"
        compact
        :show-article-count="false"
        show-watch
        @watch-toggle="handleTagWatchToggle"
      />
      <div class="markdown-body markdown-summary" v-html="renderedStoredSummary" />
    </div>

    <!-- 红色短粗线转场 -->
    <hr class="section-sep">

    <!-- Article Body -->
    <div class="article-body">
      <div v-if="displayContent" class="markdown-body markdown-article" v-html="displayContent" />
      <div v-else class="empty-content">
        <AppButton variant="primary" class="mt-4" @click="openOriginal">前往原文阅读</AppButton>
      </div>
    </div>
  </div>
</template>
