<script setup lang="ts">
import { Icon } from '@iconify/vue'
import dayjs from 'dayjs'
import { computed, ref } from 'vue'
import type { Article } from '~/types'
import ArticleTagList from '~/features/articles/components/ArticleTagList.vue'
import { useMermaidRender } from '~/composables/useMermaidRender'
import { proxiedImageUrl } from '~/utils/imageProxy'
import { toPlainText } from '~/utils/htmlSanitize'
import { AI_SUSPECT_LABEL, isAiSuspect } from '~/utils/aiDetection'
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

// mermaid 图渲染（render-mermaid-diagrams）：宿主 = 阅读列整体，覆盖 AI 整理稿与正文两个 v-html 区
const readingColRef = ref<HTMLElement | null>(null)
useMermaidRender(readingColRef, () => `${props.renderedStoredSummary ?? ''}|${props.displayContent ?? ''}`)

// 导语长度上限：与后端摘要清洗上限（≤300 字符）同口径，导语段不呈现更长的内容
const LEDE_MAX_RUNES = 300

// 导语纯文本（fix-reader-lede-blank-space）：
// 详情 description 优先、为空回退列表 excerpt，两者都先去标签取纯文本。
// 第三方 HTML 不得经 v-html 直出——部分源的摘要/正文是整篇文章的 HTML，
// 根节点带 opacity:0，不可见却照样占位（实测单篇导语段高 8046px 空白），
// 且 <img onerror=...> 一类事件属性会被真实执行。
const ledePlainText = computed(() => {
  const description = toPlainText(props.article?.description ?? '', LEDE_MAX_RUNES)
  if (description) return description
  return toPlainText(props.article?.excerpt ?? '', LEDE_MAX_RUNES)
})

// 疑似 AI 生成标记（add-ai-generated-article-detection）：静态胶囊，不复用带操作能力的
// ArticleTagList（后者绑 topic-tag 且带 watch 按钮）；仅 suspect 渲染，不占位。
const showSuspectChip = computed(() => isAiSuspect(props.article?.aiDetectionStatus))
</script>

<template>
  <div ref="readingColRef" class="reading-col">
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
      <span v-if="showSuspectChip" class="suspect-chip">{{ AI_SUSPECT_LABEL }}</span>
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
    <!-- 一律纯文本渲染：清空后的空串不渲染、不留占位块 -->
    <div v-if="showDescription && ledePlainText" class="lede" v-text="ledePlainText" />

    <!-- Image -->
    <div v-if="articleImageUrl" class="article-image">
      <img :src="proxiedImageUrl(articleImageUrl)" :alt="articleTitleFull" class="w-full">
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

<style scoped>
/* 疑似 AI 生成胶囊（add-ai-generated-article-detection）：几何照抄 .article-tag
   （ArticleTagList 的 scoped 样式不外溢，故此处自行声明）；
   底色用 --color-bg-elevated 而非硬编码 rgba，dark 主题自适应。 */
.suspect-chip {
  display: inline-flex;
  align-items: center;
  min-height: 1.9rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 999px;
  background: var(--color-bg-elevated);
  padding: 0.25rem 0.72rem;
  color: var(--color-text-secondary);
  font-size: 0.76rem;
  font-weight: 700;
  line-height: 1;
}
</style>
