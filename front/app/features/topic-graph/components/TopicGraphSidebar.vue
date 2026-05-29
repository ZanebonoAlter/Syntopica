<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useTopicGraphApi, type TopicCategory, type TopicGraphDetailPayload } from '~/api/topicGraph'
import { useAbstractTagApi } from '~/api/abstractTags'
import type { TagHierarchyNode } from '~/types/topicTag'
import type { PendingArticle, TimelineDigestSelection, TimelineAggregationArticle } from '~/types/timeline'
import { normalizeTopicCategory } from '~/features/topic-graph/utils/normalizeTopicCategory'
import KeywordCloud, { type Keyword } from './KeywordCloud.vue'

interface Props {
  detail: TopicGraphDetailPayload | null
  selectedDigest?: TimelineDigestSelection | null
  loading?: boolean
  error?: string | null
  dataState?: string
  selectedKeyword?: string | null
  selectedTagSlug?: string | null
  pendingArticles?: PendingArticle[]
  selectedPendingNode?: boolean
  abstractNodeSlug?: string | null
  abstractNodeLabel?: string | null
  timelineGroupArticles?: TimelineAggregationArticle[]
  timelineGroupKey?: string | null
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
  selectedDigest: null,
  error: null,
  dataState: 'empty',
  selectedKeyword: null,
  selectedTagSlug: null,
  pendingArticles: () => [],
  selectedPendingNode: false,
  abstractNodeSlug: null,
  abstractNodeLabel: null,
  timelineGroupArticles: () => [],
  timelineGroupKey: null,
})

const emit = defineEmits<{
  openArticle: [articleId: number]
  highlightKeyword: [keywordSlug: string | null]
  tagMerged: []
  selectChildTag: [slug: string, label: string]
  selectAbstractTag: [slug: string]
}>()

// Internal selected keyword state (for toggle behavior)
const internalSelectedKeyword = ref<string | null>(null)

// Computed selected keyword (prefer prop, fallback to internal)
const activeKeywordSlug = computed(() => props.selectedKeyword !== undefined ? props.selectedKeyword : internalSelectedKeyword.value)

const deduplicatedArticles = computed(() => {
  if (!props.detail || !props.selectedDigest) return []

  const topicArticleIds = new Set(props.detail.articles.map(article => article.id))
  const matchedIds = new Set(props.selectedDigest.matchedArticleIds)

  // 如果有选中的标签slug，只显示与该标签相关的文章
  const hasSelectedTag = props.selectedTagSlug && props.selectedTagSlug.trim() !== ''

  return props.selectedDigest.articles
    .filter(article => {
      // 如果没有选中的标签，显示所有文章
      if (!hasSelectedTag) return true

      // 如果选中了标签，只显示与当前话题标签匹配的文章
      // matchedIds 包含与当前标签相关的文章ID
      return matchedIds.has(article.id) || topicArticleIds.has(article.id)
    })
    .map(article => ({
      ...article,
      matchedTopic: matchedIds.has(article.id) || topicArticleIds.has(article.id),
      matchedBySummaryOnly: !topicArticleIds.has(article.id),
    }))
    .sort((left, right) => {
      if (left.matchedTopic === right.matchedTopic) return 0
      return left.matchedTopic ? -1 : 1
    })
})

const keywords = computed((): Keyword[] => {
  // Prefer matched articles tags from selected digest
  if (props.selectedDigest?.matchedArticlesTags?.length) {
    const tags = props.selectedDigest.matchedArticlesTags
    const maxCount = Math.max(...tags.map(tag => 1), 1)

    return tags.slice(0, 18).map(tag => ({
      slug: tag.slug,
      label: tag.label,
      count: 1,
      relevance: 0.5,
    }))
  }

  // Fallback to related_tags from topic detail
  if (!props.detail?.related_tags?.length) return []

  const maxCooccurrence = Math.max(...props.detail.related_tags.map(tag => tag.cooccurrence), 1)

  return props.detail.related_tags.slice(0, 18).map(tag => ({
    slug: tag.slug,
    label: tag.label,
    count: tag.cooccurrence,
    relevance: Math.max(tag.cooccurrence / maxCooccurrence, 0.28),
  }))
})

const shouldScrollFeaturedArticles = computed(() => deduplicatedArticles.value.length > 8)

const topicCategoryLabels: Record<TopicCategory, string> = {
  event: '事件',
  person: '人物',
  keyword: '关键词',
}

const displayTopicCategory = computed<TopicCategory>(() => {
  if (!props.detail) return 'keyword'
  return normalizeTopicCategory(props.detail.topic.category, props.detail.topic.kind)
})

function handleKeywordSelect(keyword: Keyword) {
  if (activeKeywordSlug.value === keyword.slug) {
    internalSelectedKeyword.value = null
    emit('highlightKeyword', null)
  } else {
    internalSelectedKeyword.value = keyword.slug
    emit('highlightKeyword', keyword.slug)
  }
}

watch(() => props.selectedKeyword, (value) => {
  if (value === null) {
    internalSelectedKeyword.value = null
  }
})

const topicGraphApi = useTopicGraphApi()
const showMergeDialog = ref(false)
const mergeSearchQuery = ref('')
const mergeSearchResults = ref<Array<{ id: number; label: string; slug: string; category: string; feed_count: number }>>([])
const mergeSearching = ref(false)
const mergeMerging = ref(false)
const mergeError = ref<string | null>(null)
const mergeSuccess = ref<string | null>(null)
let mergeSearchTimer: ReturnType<typeof setTimeout> | null = null

// Abstract tag detail state
const abstractTagApi = useAbstractTagApi()
const abstractChildren = ref<TagHierarchyNode[]>([])
const abstractLoading = ref(false)

// Load child tags when abstractNodeSlug changes
watch(() => props.abstractNodeSlug, async (slug) => {
  if (!slug) {
    abstractChildren.value = []
    return
  }

  abstractLoading.value = true
  try {
    const res = await abstractTagApi.fetchHierarchy(undefined, undefined, undefined, undefined, undefined)
    if (res.success && res.data) {
      // Find the matching hierarchy node by slug
      const findNodeBySlug = (nodes: TagHierarchyNode[], targetSlug: string): TagHierarchyNode | null => {
        for (const node of nodes) {
          if (node.slug === targetSlug) return node
          const found = findNodeBySlug(node.children, targetSlug)
          if (found) return found
        }
        return null
      }
      const match = findNodeBySlug(res.data.nodes, slug)
      abstractChildren.value = match?.children || []
    }
  } catch {
    abstractChildren.value = []
  } finally {
    abstractLoading.value = false
  }
}, { immediate: true })

function handleChildTagClick(child: TagHierarchyNode) {
  emit('selectChildTag', child.slug, child.label)
}

function handleAbstractTagClick() {
  if (props.abstractNodeSlug) {
    emit('selectAbstractTag', props.abstractNodeSlug)
  }
}

function openMergeDialog() {
  showMergeDialog.value = true
  mergeSearchQuery.value = ''
  mergeSearchResults.value = []
  mergeError.value = null
  mergeSuccess.value = null
}

function closeMergeDialog() {
  showMergeDialog.value = false
}

function onMergeSearchInput() {
  if (mergeSearchTimer) clearTimeout(mergeSearchTimer)
  mergeError.value = null
  if (!mergeSearchQuery.value.trim()) {
    mergeSearchResults.value = []
    return
  }
  mergeSearchTimer = setTimeout(async () => {
    mergeSearching.value = true
    try {
      const res = await topicGraphApi.searchTags(mergeSearchQuery.value, displayTopicCategory.value, 10)
      if (res.success && res.data) {
        const currentId = props.detail?.topic?.id
        mergeSearchResults.value = (res.data as any[]).filter(t => t.id !== currentId)
      }
    } catch {
      mergeError.value = '搜索失败'
    } finally {
      mergeSearching.value = false
    }
  }, 300)
}

async function doMerge(targetTagId: number, targetLabel: string) {
  if (!props.detail?.topic?.id) return
  mergeMerging.value = true
  mergeError.value = null
  try {
    const res = await topicGraphApi.mergeTags(props.detail.topic.id, targetTagId)
    if (res.success) {
      mergeSuccess.value = `已合并到「${targetLabel}」`
      setTimeout(() => {
        closeMergeDialog()
        emit('tagMerged')
      }, 800)
    } else {
      mergeError.value = res.error || '合并失败'
    }
  } catch (e: any) {
    mergeError.value = e?.message || '合并失败'
  } finally {
    mergeMerging.value = false
  }
}
</script>

<template>
  <aside
    class="topic-sidebar rounded-[34px] px-5 py-5 md:px-6 md:py-6"
    data-testid="topic-graph-sidebar"
    :data-state="props.dataState"
  >
    <div v-if="props.loading" class="topic-sidebar__empty">正在展开话题脉络...</div>
    <div v-else-if="props.error" class="topic-sidebar__empty">{{ props.error }}</div>
    <div v-else-if="!props.detail" class="topic-sidebar__empty">点一个节点，右侧就会展开这类题材的近期总结、历史轨迹和外部入口。</div>
    <div v-else class="topic-sidebar__content">
      <!-- Current topic header -->
      <section class="space-y-3">
        <p class="topic-sidebar__eyebrow">当前焦点</p>
        <div class="flex flex-wrap items-center gap-3">
          <h2 class="font-serif text-3xl text-[var(--topic-ink-strong)]">{{ props.detail.topic.label }}</h2>
          <span class="topic-pill" :class="`topic-pill--${displayTopicCategory}`">
            {{ topicCategoryLabels[displayTopicCategory] }}
          </span>
          <button
            class="topic-merge-btn"
            type="button"
            title="合并到其他标签"
            @click="openMergeDialog"
          >
            <Icon icon="mdi:merge" class="text-base" />
          </button>
        </div>
        <p class="text-sm text-[var(--topic-ink-medium)]">
          {{ props.selectedPendingNode ? '待整理文章列表' : (props.selectedDigest ? '当前日报来源文章' : '先从下方选择一条日报') }}
        </p>
      </section>

      <!-- Pending Articles Section -->
      <section v-if="props.selectedPendingNode" class="topic-panel topic-panel--featured rounded-[28px] p-4 md:p-5">
        <div class="flex items-center justify-between gap-3">
          <div>
            <p class="topic-sidebar__eyebrow">待整理文章</p>
            <p class="topic-related-card__context mt-2">已打标签但尚未生成日报</p>
          </div>
          <span class="topic-summary__count">{{ props.pendingArticles.length }} 条</span>
        </div>
        <div
          v-if="props.pendingArticles.length"
          class="topic-sidebar__news-scroll mt-4"
          data-testid="topic-graph-pending-articles"
        >
          <div class="grid gap-3">
            <button
              v-for="article in props.pendingArticles"
              :key="article.id"
              class="topic-related-card"
              type="button"
              data-testid="sidebar-pending-article"
              :data-article-id="String(article.id)"
              @click="emit('openArticle', article.id)"
            >
              <p class="topic-related-card__meta">{{ article.feedName || '来源文章' }}</p>
              <h3 class="topic-related-card__title">{{ article.title }}</h3>
              <p class="topic-related-card__context">待整理文章</p>
            </button>
          </div>
        </div>
        <div v-else class="topic-sidebar__empty topic-sidebar__empty--soft">当前没有待整理的文章。</div>
      </section>

      <!-- Related Articles (deduplicated) -->
      <section v-if="!props.selectedPendingNode" class="topic-panel topic-panel--featured rounded-[28px] p-4 md:p-5">
        <div class="flex items-center justify-between gap-3">
          <div>
            <p class="topic-sidebar__eyebrow">{{ props.timelineGroupKey ? '时间线文章' : '日报文章' }}</p>
            <p v-if="props.timelineGroupKey && props.timelineGroupArticles.length" class="topic-related-card__context mt-2">
              该时间段共 {{ props.timelineGroupArticles.length }} 篇关联文章
            </p>
            <p v-else-if="props.selectedDigest" class="topic-related-card__context mt-2">{{ props.selectedDigest.title }}</p>
          </div>
          <span class="topic-summary__count">{{ props.timelineGroupKey ? props.timelineGroupArticles.length : deduplicatedArticles.length }} 条</span>
        </div>

        <!-- Timeline group articles -->
        <template v-if="props.timelineGroupKey && props.timelineGroupArticles.length">
          <div
            class="topic-sidebar__news-scroll mt-4"
            :class="{ 'topic-sidebar__news-scroll--bounded': props.timelineGroupArticles.length > 8 }"
          >
            <div class="grid gap-3">
              <button
                v-for="article in props.timelineGroupArticles"
                :key="article.id"
                class="topic-related-card"
                type="button"
                @click="emit('openArticle', Number(article.id))"
              >
                <p class="topic-related-card__meta">{{ article.feedName || '来源文章' }}</p>
                <h3 class="topic-related-card__title">{{ article.title }}</h3>
                <p class="topic-related-card__context">{{ article.pubDate ? new Date(article.pubDate).toLocaleString('zh-CN') : '日期待补' }}</p>
              </button>
            </div>
          </div>
          <div v-if="!props.timelineGroupArticles.length" class="topic-sidebar__empty topic-sidebar__empty--soft">
            该时间段暂无关联文章
          </div>
        </template>
        <!-- Deduplicated digest articles -->
        <template v-else-if="!props.timelineGroupKey">
        <div
          v-if="deduplicatedArticles.length"
          class="topic-sidebar__news-scroll mt-4"
          :class="{ 'topic-sidebar__news-scroll--bounded': shouldScrollFeaturedArticles }"
          data-testid="topic-graph-related-articles"
        >
          <div class="grid gap-3">
            <button
              v-for="article in deduplicatedArticles"
              :key="article.id"
              class="topic-related-card"
              type="button"
              data-testid="sidebar-article"
              :data-article-id="String(article.id)"
              @click="emit('openArticle', article.id)"
            >
              <p class="topic-related-card__meta">{{ article.feedName || props.selectedDigest?.feedName || '来源文章' }}</p>
              <h3 class="topic-related-card__title">{{ article.title }}</h3>
              <p class="topic-related-card__context">来自：{{ props.selectedDigest?.title || '当前日报' }}</p>
              <p class="topic-related-card__note" :class="{ 'topic-related-card__note--soft': article.matchedBySummaryOnly }">
                {{ article.matchedBySummaryOnly ? '命中日报关键词，article 本身暂未打上当前 topic 标签' : '命中当前 topic/article 标签' }}
              </p>
            </button>
          </div>
        </div>
        <div v-else class="topic-sidebar__empty topic-sidebar__empty--soft">点击下方日报后，这里只展示该日报里命中当前主题的文章。</div>
        </template>
      </section>

      <!-- Keyword Cloud (Related Topics) -->
      <section v-if="keywords.length > 0" class="topic-panel rounded-[26px] p-4">
        <p class="topic-sidebar__eyebrow">相关主题</p>
        <div class="mt-4">
          <KeywordCloud
            :keywords="keywords"
            :selected-keyword="activeKeywordSlug"
            @select="handleKeywordSelect"
          />
          <p class="keywords-hint">
            点击标签，只高亮当前标签节点和它的一跳邻居
          </p>
        </div>
      </section>

      <!-- Abstract Tag Detail Panel -->
      <section v-if="props.abstractNodeSlug && abstractChildren.length > 0" class="topic-panel rounded-[26px] p-4">
        <p class="topic-sidebar__eyebrow">抽象标签详情</p>

        <!-- Child Tags -->
        <div class="mt-3">
          <h4 class="text-xs font-medium text-[var(--topic-ink-soft)]">子标签</h4>
          <div class="mt-2 grid gap-1.5">
            <!-- Abstract tag itself (returns to parent) -->
            <button
              type="button"
              class="flex items-center gap-2 rounded-xl px-2.5 py-1.5 text-left transition-all hover:border-[rgba(240,138,75,0.2)]"
              :class="'bg-[rgba(10,16,23,0.56)] border border-[rgba(240,138,75,0.3)]'"
              @click="handleAbstractTagClick"
            >
              <span class="w-2 h-2 rounded-full shrink-0 bg-[rgba(240,138,75,0.8)]"></span>
              <span class="text-sm text-[var(--topic-ink-strong)] truncate">{{ props.abstractNodeLabel || '全部' }}</span>
              <span class="ml-auto text-xs text-[rgba(240,138,75,0.7)] shrink-0">全部</span>
            </button>
            <!-- Child tags -->
            <button
                v-for="child in abstractChildren"
                :key="child.slug"
                type="button"
                class="flex items-center gap-2 rounded-xl px-2.5 py-1.5 text-left transition-all hover:border-[rgba(240,138,75,0.2)]"
                :class="'bg-[rgba(10,16,23,0.56)] border border-[var(--topic-border)]'"
                @click="handleChildTagClick(child)"
              >
              <span class="w-2 h-2 rounded-full shrink-0" :class="`bg-[#6366f1]`"></span>
              <span class="text-sm text-[var(--topic-ink-strong)] truncate">{{ child.label }}</span>
              <span class="ml-auto text-xs text-[var(--topic-ink-soft)] shrink-0">{{ child.articleCount }} 篇</span>
            </button>
          </div>
        </div>
      </section>
    </div>

    <!-- Merge Dialog -->
    <Teleport to="body">
      <div v-if="showMergeDialog" class="merge-overlay" @click.self="closeMergeDialog">
        <div class="merge-dialog">
          <div class="flex items-center justify-between gap-3 mb-4">
            <h3 class="text-lg font-semibold text-gray-100">
              合并「{{ props.detail?.topic?.label }}」
            </h3>
            <button type="button" class="merge-close-btn" @click="closeMergeDialog">
              <Icon icon="mdi:close" />
            </button>
          </div>
          <p class="text-sm text-gray-400 mb-3">
            搜索要合并到的目标标签。合并后当前标签的所有文章将迁移到目标标签。
          </p>
          <input
            v-model="mergeSearchQuery"
            type="text"
            class="merge-input"
            placeholder="搜索标签..."
            @input="onMergeSearchInput"
          />
          <div v-if="mergeSearching" class="merge-status">搜索中...</div>
          <div v-else-if="mergeError" class="merge-status merge-status--error">{{ mergeError }}</div>
          <div v-else-if="mergeSuccess" class="merge-status merge-status--success">{{ mergeSuccess }}</div>
          <div v-else-if="mergeSearchResults.length" class="merge-results">
            <button
              v-for="tag in mergeSearchResults"
              :key="tag.id"
              type="button"
              class="merge-result-item"
              :disabled="mergeMerging"
              @click="doMerge(tag.id, tag.label)"
            >
              <span class="merge-result-label">{{ tag.label }}</span>
              <span class="merge-result-meta">{{ tag.category }} · {{ tag.feed_count }} feeds</span>
            </button>
          </div>
          <div v-else-if="mergeSearchQuery.trim()" class="merge-status">无匹配结果</div>
        </div>
      </div>
    </Teleport>
  </aside>
</template>

<style scoped>
.topic-sidebar {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
  position: relative;
  overflow: hidden;
  --topic-ink-strong: rgba(248, 251, 255, 0.96);
  --topic-ink-medium: rgba(210, 221, 232, 0.82);
  --topic-ink-soft: rgba(148, 168, 188, 0.7);
  --topic-border: rgba(123, 154, 192, 0.18);
  --topic-border-strong: rgba(123, 154, 192, 0.28);
  --topic-card: linear-gradient(180deg, rgba(20, 30, 42, 0.86), rgba(11, 17, 25, 0.92));
  --topic-card-raised: linear-gradient(180deg, rgba(25, 37, 50, 0.94), rgba(13, 21, 30, 0.96));
  --topic-chip: rgba(12, 19, 28, 0.82);
  background:
    radial-gradient(circle at 16% 14%, rgba(240, 138, 75, 0.18), transparent 26%),
    radial-gradient(circle at 82% 10%, rgba(74, 129, 219, 0.16), transparent 24%),
    linear-gradient(180deg, rgba(17, 28, 39, 0.96), rgba(8, 14, 22, 0.98));
  border: 1px solid rgba(153, 187, 227, 0.18);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.05),
    0 28px 90px rgba(2, 6, 12, 0.4);
}

.topic-sidebar::before {
  content: '';
  position: absolute;
  inset: 0;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.05), transparent 18%),
    linear-gradient(90deg, rgba(255, 255, 255, 0.03), transparent 28%);
  pointer-events: none;
}

.topic-sidebar::after {
  content: '';
  position: absolute;
  inset: 1rem auto 1rem 1rem;
  width: 1px;
  background: linear-gradient(180deg, rgba(240, 138, 75, 0.4), rgba(115, 150, 198, 0.08) 42%, rgba(115, 150, 198, 0));
  pointer-events: none;
}

.topic-sidebar__content {
  position: relative;
  z-index: 1;
  display: grid;
  min-height: 0;
  flex: 1;
  gap: 1.5rem;
  grid-template-rows: auto minmax(0, 1fr) auto;
}

.topic-sidebar__content > section:first-child {
  padding-left: 0.75rem;
}

.topic-sidebar__empty--soft {
  border-style: solid;
  background: rgba(10, 16, 23, 0.56);
}

.topic-sidebar__eyebrow {
  font-size: 0.72rem;
  letter-spacing: 0.24em;
  text-transform: uppercase;
  color: var(--topic-ink-soft);
}

.topic-sidebar__empty {
  position: relative;
  z-index: 1;
  border-radius: 1.6rem;
  border: 1px dashed rgba(153, 187, 227, 0.2);
  background: rgba(9, 15, 23, 0.5);
  padding: 1.2rem;
  color: var(--topic-ink-medium);
}

.topic-panel {
  position: relative;
  overflow: hidden;
  border: 1px solid var(--topic-border);
  background: var(--topic-card);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.04),
    0 22px 60px rgba(2, 6, 12, 0.24);
  backdrop-filter: blur(16px);
}

.topic-panel::before {
  content: '';
  position: absolute;
  inset: 0 auto auto 0;
  width: 100%;
  height: 1px;
  background: linear-gradient(90deg, rgba(240, 138, 75, 0.44), rgba(120, 167, 230, 0.14), rgba(255, 255, 255, 0));
  pointer-events: none;
}

.topic-panel--featured {
  display: flex;
  min-height: 0;
  flex-direction: column;
  overflow: hidden;
  background: var(--topic-card-raised);
}

.topic-sidebar__news-scroll {
  min-height: 0;
  flex: 1;
  padding-right: 0.25rem;
}

.topic-sidebar__news-scroll--bounded {
  overflow-y: auto;
  max-height: 75vh;
}

.topic-sidebar__news-scroll--bounded::-webkit-scrollbar {
  width: 0.45rem;
}

.topic-sidebar__news-scroll--bounded::-webkit-scrollbar-track {
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.05);
}

.topic-sidebar__news-scroll--bounded::-webkit-scrollbar-thumb {
  border-radius: 999px;
  background: rgba(240, 138, 75, 0.28);
}

.topic-pill {
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(22, 29, 39, 0.88), rgba(11, 17, 24, 0.96));
  padding: 0.45rem 0.85rem;
  font-size: 0.78rem;
  font-weight: 600;
  color: rgba(248, 251, 255, 0.9);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
}

.topic-pill--event {
  border: 1px solid rgba(245, 158, 11, 0.32);
}

.topic-pill--person {
  border: 1px solid rgba(16, 185, 129, 0.32);
}

.topic-pill--keyword {
  border: 1px solid rgba(99, 102, 241, 0.32);
}

.topic-related-card {
  position: relative;
  width: 100%;
  text-align: left;
  display: block;
  border-radius: 1.25rem;
  border: 1px solid var(--topic-border);
  background: linear-gradient(180deg, rgba(18, 27, 38, 0.96), rgba(10, 16, 24, 0.98));
  padding: 1rem 1rem 1.05rem;
  text-decoration: none;
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.04),
    0 16px 40px rgba(3, 8, 14, 0.28);
  transition:
    transform 0.22s ease,
    border-color 0.22s ease,
    background 0.22s ease,
    box-shadow 0.22s ease;
}

.topic-related-card::before {
  content: '';
  position: absolute;
  inset: 0 auto 0 0;
  width: 3px;
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(240, 138, 75, 0.9), rgba(92, 143, 226, 0.52));
  opacity: 0.7;
}

.topic-related-card__meta,
.topic-related-card__context {
  font-size: 0.74rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--topic-ink-soft);
}

.topic-related-card__title {
  margin-top: 0.45rem;
  font-size: 1rem;
  font-weight: 700;
  line-height: 1.45;
  color: var(--topic-ink-strong);
}

.topic-related-card__context {
  margin-top: 0.65rem;
}

.topic-related-card__note {
  margin-top: 0.55rem;
  font-size: 0.78rem;
  line-height: 1.5;
  color: rgba(255, 227, 203, 0.86);
}

.topic-related-card__note--soft {
  color: rgba(173, 193, 214, 0.72);
}

.topic-related-card {
  cursor: pointer;
}

.topic-related-card:hover,
.topic-related-card:focus-visible {
  transform: translateY(-2px);
  border-color: rgba(240, 138, 75, 0.36);
  background: linear-gradient(180deg, rgba(24, 35, 48, 0.98), rgba(12, 19, 28, 1));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.05),
    0 24px 48px rgba(3, 8, 14, 0.36);
}

.topic-related-card:focus-visible {
  outline: 2px solid rgba(240, 138, 75, 0.45);
  outline-offset: 2px;
}

@media (max-width: 1279px) {
  .topic-sidebar__content {
    display: flex;
    flex-direction: column;
  }

  .topic-panel--featured,
  .topic-sidebar__news-scroll {
    min-height: auto;
    flex: none;
    overflow: visible;
  }
}

.topic-summary__count {
  border-radius: 999px;
  border: 1px solid rgba(240, 138, 75, 0.2);
  background: var(--topic-chip);
  padding: 0.32rem 0.7rem;
  font-size: 0.75rem;
  color: rgba(255, 228, 209, 0.88);
}

.keywords-hint {
  font-size: 0.72rem;
  color: rgba(255, 255, 255, 0.4);
  text-align: center;
  margin-top: 0.75rem;
}

.topic-merge-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2rem;
  height: 2rem;
  border-radius: 999px;
  border: 1px solid rgba(141, 173, 214, 0.2);
  background: rgba(14, 21, 30, 0.7);
  color: rgba(241, 246, 250, 0.7);
  cursor: pointer;
  transition: all 0.2s ease;
}

.topic-merge-btn:hover {
  border-color: rgba(240, 138, 75, 0.4);
  color: rgba(240, 138, 75, 0.9);
  background: rgba(240, 138, 75, 0.1);
}
</style>

<style>
.merge-overlay {
  position: fixed;
  inset: 0;
  z-index: 9999;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(2, 6, 12, 0.75);
  backdrop-filter: blur(8px);
}

.merge-dialog {
  width: 420px;
  max-width: 92vw;
  max-height: 80vh;
  border-radius: 1.5rem;
  border: 1px solid rgba(200, 210, 225, 0.2);
  background: linear-gradient(180deg, #1a2536, #0e1520);
  box-shadow: 0 32px 80px rgba(2, 6, 12, 0.6);
  padding: 1.5rem;
  overflow-y: auto;
}

.merge-close-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2rem;
  height: 2rem;
  border-radius: 999px;
  border: 1px solid rgba(200, 210, 225, 0.15);
  background: transparent;
  color: #9ca3af;
  cursor: pointer;
  font-size: 1.1rem;
}

.merge-close-btn:hover {
  color: #e5e7eb;
  border-color: rgba(200, 210, 225, 0.3);
}

.merge-input {
  width: 100%;
  padding: 0.6rem 1rem;
  border-radius: 0.75rem;
  border: 1px solid rgba(200, 210, 225, 0.2);
  background: rgba(10, 16, 23, 0.8);
  color: #f1f5f9;
  font-size: 0.9rem;
  outline: none;
  transition: border-color 0.2s ease;
}

.merge-input:focus {
  border-color: rgba(240, 138, 75, 0.5);
}

.merge-input::placeholder {
  color: rgba(173, 193, 214, 0.4);
}

.merge-status {
  margin-top: 0.75rem;
  font-size: 0.82rem;
  color: #9ca3af;
}

.merge-status--error {
  color: #f87171;
}

.merge-status--success {
  color: #4ade80;
}

.merge-results {
  margin-top: 0.75rem;
  display: grid;
  gap: 0.5rem;
}

.merge-result-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
  padding: 0.6rem 0.9rem;
  border-radius: 0.75rem;
  border: 1px solid rgba(200, 210, 225, 0.12);
  background: rgba(14, 21, 30, 0.6);
  color: #f1f5f9;
  text-align: left;
  cursor: pointer;
  transition: all 0.15s ease;
}

.merge-result-item:hover:not(:disabled) {
  border-color: rgba(240, 138, 75, 0.4);
  background: rgba(240, 138, 75, 0.1);
}

.merge-result-item:disabled {
  opacity: 0.5;
  cursor: wait;
}

.merge-result-label {
  font-weight: 600;
  font-size: 0.9rem;
  color: #e2e8f0;
}

.merge-result-meta {
  font-size: 0.75rem;
  color: #94a3b8;
}
</style>
