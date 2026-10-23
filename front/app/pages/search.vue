<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { nextTick } from 'vue'
import AppPageShell from '~/components/ui/AppPageShell.vue'
import AppInput from '~/components/ui/AppInput.vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppToggle from '~/components/ui/AppToggle.vue'
import ThemeToggle from '~/components/ui/ThemeToggle.vue'
import { ArticleCardView } from '~/features/articles/public'
import ArticlePreviewModal from '~/features/tags/components/ArticlePreviewModal.vue'
import { useSearch } from '~/features/search/composables/useSearch'
import { useArticlesApi } from '~/api/articles'
import { normalizeArticle, type ArticlePayload } from '~/api/normalizers/article'
import type { Article, SearchHit } from '~/types'

/**
 * 站内文章搜索页（add-article-search）。
 * 布局契约（ui-design Layout Contract）：AppPageShell mode="contained"（≤1120px 居中），
 * 全部语义 token，editorial/dark 双主题零特判。
 */
const {
  query, includeArchived, expandEnabled, sortBy, status, loading, loadingMore,
  hits, total, tookMs, semanticSkipped, semanticTags, expandWords,
  errorMessage, hasMore, submit, retry, loadMore, setIncludeArchived, setExpandEnabled, setSortBy,
} = useSearch()

const articlesApi = useArticlesApi()

// ---- 结果分区（review R-5 双命中归属）：扩展区只放扩展独占命中，主路+扩展双命中卡
// 留在主区（chip 已同时展示两类命中理由）——主路命中不因叠加扩展词被整卡挪区 ----
/** 扩展独占命中：hitReasons 含 expand 且不含任何主路（keyword/semantic）理由 */
function isExpandOnlyHit(hit: SearchHit): boolean {
  return hit.hitReasons.some(r => r.type === 'expand')
    && !hit.hitReasons.some(r => r.type === 'keyword' || r.type === 'semantic')
}
const mainResults = computed(() => hits.value.filter(h => !isExpandOnlyHit(h)))
const expandResults = computed(() => hits.value.filter(isExpandOnlyHit))

// ---- 预览弹窗：点卡片开 ArticlePreviewModal（非路由跳转，URL 不变）----
const showPreview = ref(false)
const selectedPreviewArticle = ref<Article | null>(null)
const loadingPreviewArticle = ref(false)
const activeHit = ref<SearchHit | null>(null)
// 关闭回原位：记录打开时列表滚动位置，关闭后恢复（查询 state 不卸载列表）
const savedScrollTop = ref(0)
const scrollContainerRef = ref<HTMLElement | null>(null)

/** 预览正文高亮词：当前命中卡的命中文本（关键词/标签/扩展词去重） */
const highlightTerms = computed(() => {
  const terms: string[] = []
  const seen = new Set<string>()
  for (const reason of activeHit.value?.hitReasons ?? []) {
    if (reason.text && !seen.has(reason.text)) {
      seen.add(reason.text)
      terms.push(reason.text)
    }
  }
  return terms
})

// 列表投影无正文，弹窗需详情接口取全文（id 字符串化发生在 api/search 归一层；
// 此处仅做 API 数字参数的边界还原，与 useTagsPage.openArticlePreview 同款）
async function openPreview(hit: SearchHit): Promise<void> {
  activeHit.value = hit
  savedScrollTop.value = scrollContainerRef.value?.scrollTop ?? 0
  showPreview.value = true
  loadingPreviewArticle.value = true
  try {
    const response = await articlesApi.getArticle(Number(hit.article.id))
    if (!response.success || !response.data) return
    selectedPreviewArticle.value = normalizeArticle(response.data as unknown as ArticlePayload)
  } catch (error) {
    console.error('Failed to open article preview:', error)
  } finally {
    loadingPreviewArticle.value = false
  }
}

function closePreview(): void {
  showPreview.value = false
  activeHit.value = null
  selectedPreviewArticle.value = null
  void nextTick(() => {
    if (scrollContainerRef.value) scrollContainerRef.value.scrollTop = savedScrollTop.value
  })
}

async function handleFavorite(articleId: string): Promise<void> {
  const hit = hits.value.find(h => h.article.id === articleId)
  if (!hit) return
  const next = !(selectedPreviewArticle.value?.id === articleId
    ? selectedPreviewArticle.value.favorite
    : hit.article.favorite)
  const response = await articlesApi.updateArticle(Number(articleId), { favorite: next })
  if (!response.success) return
  hit.article.favorite = next
  if (selectedPreviewArticle.value?.id === articleId) {
    selectedPreviewArticle.value = { ...selectedPreviewArticle.value, favorite: next }
  }
}

function handleArticleUpdate(articleId: string, updates: Partial<Article>): void {
  const hit = hits.value.find(h => h.article.id === articleId)
  if (hit) Object.assign(hit.article, updates)
  if (selectedPreviewArticle.value?.id === articleId) {
    selectedPreviewArticle.value = { ...selectedPreviewArticle.value, ...updates }
  }
}

// 弹窗内上一篇/下一篇：单文章预览（previewArticles 空 → 导航按钮隐藏），切换选中即换正文
function handlePreviewNavigate(article: Article): void {
  selectedPreviewArticle.value = article
}

// 「阅读全文」（add-article-search D1）：关闭弹窗后经 /?article=<id> 深链进首页阅读路径
// （FeedLayoutShell.onMounted 详情直取，归档文章不依赖首页列表成员资格）
function goReadFull(): void {
  const article = selectedPreviewArticle.value
  if (!article) return
  closePreview()
  void navigateTo({ path: '/', query: { article: article.id } })
}
</script>

<template>
  <div class="search-page">
    <div class="search-topbar">
      <div class="search-topbar-inner">
        <div class="flex items-center gap-3">
          <NuxtLink to="/" class="search-back-btn" title="返回首页">
            <Icon icon="mdi:arrow-left" width="16" />
          </NuxtLink>
          <Icon icon="mdi:magnify" width="18" class="text-white/50" />
          <h1 class="search-page-title">文章搜索</h1>
        </div>
        <ThemeToggle />
      </div>
    </div>

    <!-- 滚动容器：弹窗关闭回原位恢复它的 scrollTop -->
    <div ref="scrollContainerRef" class="search-main">
      <AppPageShell mode="contained" as="div">
        <!-- 搜索工具条 -->
        <form class="search-bar" @submit.prevent="submit">
          <AppInput
            v-model="query"
            class="search-input-grow"
            input-id="search-q"
            placeholder="输入关键词，搜全站文章（默认含归档）"
            :disabled="loading"
          />
          <AppButton type="submit" :loading="loading" :disabled="loading">搜索</AppButton>
        </form>

        <div class="search-filters">
          <!-- 排序分段控件（add-search-sort-modes D6）：视觉同 BoardTimelinePanel 的
               tags-sort-toggle，不抽公共组件；所有状态下可见，首查 loading 时 disabled -->
          <div class="search-sort-toggle" role="group" aria-label="结果排序">
            <button
              type="button"
              class="search-sort-btn"
              :class="{ 'search-sort-btn--active': sortBy === 'date' }"
              data-test="search-sort-date"
              :aria-pressed="sortBy === 'date'"
              :disabled="loading"
              @click="setSortBy('date')"
            >
              <Icon icon="mdi:clock-outline" width="13" height="13" /> 最新优先
            </button>
            <button
              type="button"
              class="search-sort-btn"
              :class="{ 'search-sort-btn--active': sortBy === 'relevance' }"
              data-test="search-sort-relevance"
              :aria-pressed="sortBy === 'relevance'"
              :disabled="loading"
              @click="setSortBy('relevance')"
            >
              <Icon icon="mdi:star-outline" width="13" height="13" /> 相关度优先
            </button>
          </div>
          <AppToggle
            :model-value="includeArchived"
            label="含归档文章"
            :disabled="loading"
            @update:model-value="setIncludeArchived"
          />
          <AppToggle
            :model-value="expandEnabled"
            label="LLM 扩展搜索"
            :disabled="loading"
            @update:model-value="setExpandEnabled"
          />
          <span v-if="status === 'success' || status === 'empty'" class="search-summary">
            共 {{ total }} 条结果<template v-if="semanticTags.length"> · 语义命中 {{ semanticTags.length }} 个标签</template><template v-if="tookMs != null"> · {{ (tookMs / 1000).toFixed(2) }}s</template>
          </span>
        </div>

        <!-- 语义降级提示条：不阻塞、不报错，随下次查询自动重试 -->
        <div v-if="semanticSkipped && (status === 'success' || status === 'empty')" class="search-notice" role="status">
          语义匹配暂不可用，已降级为纯关键词结果
        </div>

        <!-- 错误条：保留输入，重试按钮 -->
        <div v-if="status === 'error'" class="search-error" role="alert">
          <span class="search-error-text">{{ errorMessage }}</span>
          <AppButton variant="secondary" size="sm" data-test="search-retry" @click="retry">重试</AppButton>
        </div>

        <!-- 首查骨架屏 ×3（封面 + 双行 shimmer） -->
        <div v-if="status === 'loading'" class="search-skeletons" data-test="search-skeletons">
          <div v-for="i in 3" :key="i" class="skeleton-row">
            <div class="skeleton-cover skeleton-pulse" />
            <div class="skeleton-main">
              <div class="skeleton-line skeleton-pulse" style="width: 78%" />
              <div class="skeleton-line skeleton-pulse" style="width: 45%" />
            </div>
          </div>
        </div>

        <!-- 初始空态（未搜索） -->
        <div v-else-if="status === 'initial'" class="search-empty" data-test="search-initial-empty">
          <Icon icon="mdi:magnify" width="44" height="44" />
          <p class="search-empty-title">输入关键词，搜全站文章</p>
          <p class="search-empty-sub">默认含归档 · 支持语义匹配标签</p>
        </div>

        <!-- 无结果：空态 + 开启 LLM 扩展一键重查 -->
        <div v-else-if="status === 'empty'" class="search-empty" data-test="search-no-results">
          <Icon icon="mdi:magnify" width="44" height="44" />
          <p class="search-empty-title">没有找到「{{ query }}」的相关文章</p>
          <p class="search-empty-sub">试试更短的关键词，或用 LLM 扩展同义词再搜一次</p>
          <AppButton
            v-if="!expandEnabled"
            variant="secondary"
            data-test="enable-expand-retry"
            @click="setExpandEnabled(true)"
          >
            ✨ 开启 LLM 扩展搜索再试
          </AppButton>
        </div>

        <!-- 结果列表：命中 chip + 归档角标（ArticleCardView 新 props） -->
        <template v-else-if="status === 'success'">
          <div class="search-results">
            <ArticleCardView
              v-for="hit in mainResults"
              :key="hit.article.id"
              :article="hit.article"
              :hit-reasons="hit.hitReasons"
              :archived="hit.archived"
              @click="openPreview(hit)"
              @favorite="handleFavorite"
            />
          </div>

          <!-- 扩展结果分隔条：主结果与扩展结果 UI 区隔 + 扩展词回显 -->
          <template v-if="expandResults.length">
            <div class="expand-divider" data-test="expand-divider">
              ✨ 以下 {{ expandResults.length }} 条来自扩展搜索
            </div>
            <div v-if="expandWords.length" class="expand-hint">已用 LLM 将查询扩展为：{{ expandWords.join(' · ') }}</div>
            <div class="search-results">
              <ArticleCardView
                v-for="hit in expandResults"
                :key="hit.article.id"
                :article="hit.article"
                :hit-reasons="hit.hitReasons"
                :archived="hit.archived"
                @click="openPreview(hit)"
                @favorite="handleFavorite"
              />
            </div>
          </template>

          <div class="search-load-more">
            <AppButton
              v-if="hasMore"
              variant="secondary"
              :loading="loadingMore"
              :disabled="loadingMore"
              data-test="search-load-more"
              @click="loadMore"
            >
              加载更多
            </AppButton>
            <span v-else class="search-end-hint">已显示全部 {{ hits.length }} 条</span>
          </div>
        </template>
      </AppPageShell>
    </div>

    <!-- 结果卡片点击预览（非路由跳转，URL 不变） -->
    <ArticlePreviewModal
      :visible="showPreview"
      :selected-preview-article="selectedPreviewArticle"
      :preview-articles="[]"
      :loading-preview-article="loadingPreviewArticle"
      :highlight-terms="highlightTerms"
      @close="closePreview"
      @favorite="handleFavorite"
      @article-update="handleArticleUpdate"
      @navigate="handlePreviewNavigate"
    >
      <!-- 阅读全文（spec「阅读全文跳转」）：详情未就绪时隐藏；图标用子集内既有 mdi:book-open-outline -->
      <template #header-actions>
        <AppButton
          v-if="selectedPreviewArticle"
          variant="secondary"
          size="sm"
          data-test="preview-read-full"
          @click="goReadFull"
        >
          <Icon icon="mdi:book-open-outline" width="14" height="14" />
          阅读全文
        </AppButton>
      </template>
    </ArticlePreviewModal>
  </div>
</template>

<style scoped>
.search-page {
  display: flex;
  flex-direction: column;
  height: 100vh;
  height: 100dvh;
  background: var(--color-bg-base);
  color: var(--color-text-primary);
}

.search-topbar {
  flex-shrink: 0;
  background: var(--color-bg-elevated);
  border-bottom: 1px solid var(--color-border-subtle);
}

.search-topbar-inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 20px;
}

.search-back-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 8px;
  color: var(--color-text-secondary);
  transition: background 0.15s, color 0.15s;
}

.search-back-btn:hover {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
}

.search-page-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--color-text-primary);
}

.search-main {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 24px 0 48px;
}

/* 搜索工具条 */
.search-bar {
  display: flex;
  gap: 10px;
}

.search-input-grow {
  flex: 1;
}

.search-input-grow :deep(.app-input) {
  padding: 10px 14px;
  font-size: 15px;
}

/* 过滤行：含归档 / LLM 扩展 / 结果摘要；窄屏折行堆叠 */
.search-filters {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px 18px;
  margin-top: 14px;
}

/* 排序分段按钮：复用 BoardTimelinePanel 分段形态（accent token），窄屏折行不溢出 */
.search-sort-toggle {
  display: inline-flex;
  flex: 0 0 auto;
  border: 1px solid var(--color-border-medium);
  border-radius: 8px;
  overflow: hidden;
}

.search-sort-btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 5px 10px;
  font-size: 12.5px;
  color: var(--color-text-muted);
  background: transparent;
  border: none;
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}

.search-sort-btn--active {
  color: var(--color-accent);
  background: var(--color-accent-subtle);
}

.search-sort-btn:hover:not(.search-sort-btn--active):not(:disabled) {
  color: var(--color-text-secondary);
}

.search-sort-btn:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.search-summary {
  margin-left: auto;
  font-size: 12.5px;
  color: var(--color-text-muted);
}

/* 语义降级黄色提示条（不阻塞） */
.search-notice {
  margin-top: 14px;
  padding: 8px 14px;
  border-radius: 8px;
  font-size: 13px;
  background: var(--color-warning-subtle);
  border: 1px solid color-mix(in srgb, var(--color-warning) 32%, transparent);
  color: var(--color-text-primary);
}

/* 错误条 + 重试 */
.search-error {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 14px;
  padding: 8px 14px;
  border-radius: 8px;
  font-size: 13px;
  background: color-mix(in srgb, var(--color-error) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--color-error) 30%, transparent);
  color: var(--color-error);
}

.search-error-text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 首查骨架屏 ×3 */
.skeleton-row {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 14px 0;
  border-bottom: 1px solid var(--color-border-subtle);
}

.skeleton-cover {
  flex: 0 0 68px;
  width: 68px;
  height: 68px;
  border-radius: 8px;
}

.skeleton-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.skeleton-line {
  height: 14px;
  border-radius: 6px;
}

.skeleton-pulse {
  background: var(--color-bg-sunken);
  animation: skeleton-pulse 1.4s ease-in-out infinite;
}

@keyframes skeleton-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.45; }
}

/* 空态（初始/无结果共用版式） */
.search-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 72px 16px 48px;
  text-align: center;
  color: var(--color-text-muted);
}

.search-empty-title {
  margin: 10px 0 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--color-text-secondary);
}

.search-empty-sub {
  margin: 0 0 10px;
  font-size: 13px;
}

/* 结果列表 */
.search-results {
  margin-top: 10px;
}

.search-load-more {
  display: flex;
  justify-content: center;
  padding: 18px 0 6px;
}

.search-end-hint {
  font-size: 12.5px;
  color: var(--color-text-muted);
}

/* 扩展结果分隔条：warning 虚线 + 扩展词回显 */
.expand-divider {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 18px 0 2px;
  font-size: 12px;
  color: var(--color-warning);
}

.expand-divider::before,
.expand-divider::after {
  content: '';
  flex: 1;
  border-top: 1px dashed color-mix(in srgb, var(--color-warning) 45%, transparent);
}

.expand-hint {
  text-align: center;
  font-size: 11.5px;
  color: var(--color-text-muted);
  margin: 2px 0 0;
}

/* 窄屏降级（断点统一 767.98px；≥768px 布局零变化） */
@media (max-width: 767.98px) {
  .search-main {
    padding: 16px 0 32px;
  }

  .search-filters {
    flex-direction: column;
    align-items: flex-start;
  }

  .search-summary {
    margin-left: 0;
  }
}
</style>
