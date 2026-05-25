<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { Icon } from '@iconify/vue'
import { useSemanticBoardsApi, type SemanticBoard, type AuxiliaryLabelItem, type UpgradeCandidate, type UpgradeCluster, type UpgradeSuggestion, type BackfillTask, type MatchingConfig, type BoardArticle, type BoardArticleTag } from '~/api/semanticBoards'
import { useAuxiliaryLabelsApi, type AuxiliaryLabel, type AuxiliaryLabelCluster } from '~/api/auxiliaryLabels'
import { useArticlesApi } from '~/api/articles'
import { normalizeArticle } from '~/features/articles/utils/normalizeArticle'
import type { Article } from '~/types'
import type { ArticlePayload } from '~/features/articles/utils/normalizeArticle'
import ArticleContentView from '~/features/articles/components/ArticleContentView.vue'
import { useFeedsStore } from '~/stores/feeds'
import SemanticBoardList from './SemanticBoardList.vue'
import AddSemanticBoardDialog from './AddSemanticBoardDialog.vue'
import BoardCompositionPanel from './BoardCompositionPanel.vue'
import BoardNarrativeTimeline from './BoardNarrativeTimeline.vue'
import AuxiliaryLabelPool from './AuxiliaryLabelPool.vue'
import UpgradeSuggestionPanel from './UpgradeSuggestionPanel.vue'
import BackfillProgress from './BackfillProgress.vue'
import MatchingConfigDialog from './MatchingConfigDialog.vue'
import NarrativeGenerateDialog from './NarrativeGenerateDialog.vue'

const sbApi = useSemanticBoardsApi()
const auxApi = useAuxiliaryLabelsApi()
const articlesApi = useArticlesApi()
const feedsStore = useFeedsStore()

const boards = ref<SemanticBoard[]>([])
const selectedBoardId = ref<number | null>(null)
const boardsLoading = ref(false)
const boardsError = ref<string | null>(null)

const compositionLabels = ref<AuxiliaryLabelItem[]>([])
const compositionLoading = ref(false)

const auxiliaryLabels = ref<AuxiliaryLabel[]>([])
const auxClusters = ref<AuxiliaryLabelCluster[]>([])
const auxUnclusteredCount = ref(0)
const auxLoading = ref(false)
const auxSearchQuery = ref('')
const auxStatusFilter = ref('')
const auxPage = ref(1)
const auxPagination = ref<{ page: number; pages: number; total: number } | null>(null)
const auxPerPage = 50

const upgradeCandidates = ref<UpgradeCandidate[]>([])
const upgradeClusters = ref<UpgradeCluster[]>([])
const upgradeSuggestions = ref<UpgradeSuggestion[]>([])
const upgradeLoading = ref(false)
const upgradeSuggesting = ref(false)
const upgradeBackfillNotice = ref(false)

const backfillTask = ref<BackfillTask | null>(null)
let backfillPollTimer: ReturnType<typeof setInterval> | null = null

const matchingConfig = ref<MatchingConfig | null>(null)
const matchingConfigLoading = ref(false)

const contentTab = ref<'composition' | 'narratives' | 'articles'>('composition')
const showAddDialog = ref(false)
const showUpgradeDialog = ref(false)
const showMatchingConfigDialog = ref(false)
const showGenerateDialog = ref(false)

const timelineArticles = ref<BoardArticle[]>([])
const timelineLoading = ref(false)
const timelinePage = ref(1)
const timelineHasMore = ref(false)
const timelinePerPage = 50
const activeFilterLabelId = ref<number | null>(null)
const filterFeedId = ref<number | null>(null)
const startDate = ref<string>('')
const endDate = ref<string>('')
const feedOptions = computed(() => feedsStore.feeds)
const timelineVisible = computed(() => selectedBoardId.value !== null)

const selectedPreviewArticle = ref<Article | null>(null)
const previewArticles = ref<Article[]>([])
const loadingPreviewArticle = ref(false)

async function loadBoards() {
  boardsLoading.value = true
  boardsError.value = null
  const res = await sbApi.getBoards()
  if (res.success && res.data) {
    boards.value = res.data.items
  } else {
    boardsError.value = res.error || '加载板块失败'
  }
  boardsLoading.value = false
}

async function loadComposition(boardId: number) {
  compositionLoading.value = true
  const res = await sbApi.getComposition(boardId)
  if (res.success && res.data) {
    compositionLabels.value = res.data.items
  } else {
    compositionLabels.value = []
  }
  compositionLoading.value = false
}

async function loadAuxiliaryLabels() {
  auxLoading.value = true
  const res = await auxApi.getLabels({ search: auxSearchQuery.value || undefined, status: auxStatusFilter.value || undefined, page: auxPage.value, per_page: auxPerPage })
  if (res.success && res.data) {
    auxiliaryLabels.value = res.data.items
    if (res.pagination) {
      auxPagination.value = { page: res.pagination.page, pages: res.pagination.pages, total: res.pagination.total }
    } else {
      auxPagination.value = null
    }
  } else {
    auxiliaryLabels.value = []
    auxPagination.value = null
  }
  auxLoading.value = false
}

async function loadClusters() {
  const res = await auxApi.getClusters()
  if (res.success && res.data) {
    auxClusters.value = res.data.clusters
    auxUnclusteredCount.value = res.data.unclustered_count
  }
}

function handleUpdatePage(page: number) {
  auxPage.value = page
  void loadAuxiliaryLabels()
}

function handleSelectBoard(id: number | null) {
  selectedBoardId.value = id
  activeFilterLabelId.value = null
  filterFeedId.value = null
  startDate.value = ''
  endDate.value = ''
  contentTab.value = 'composition'
  if (id !== null) {
    void loadComposition(id)
    timelinePage.value = 1
    void loadTimelineArticles(id)
  } else {
    compositionLabels.value = []
    timelineArticles.value = []
  }
}

async function loadTimelineArticles(boardId: number, append = false) {
  timelineLoading.value = true
  try {
    const page = append ? timelinePage.value + 1 : 1
    const params: Record<string, unknown> = { page, per_page: timelinePerPage }
    if (activeFilterLabelId.value !== null) {
      params.auxiliary_label_id = activeFilterLabelId.value
    }
    if (filterFeedId.value) {
      params.feed_id = filterFeedId.value
    }
    if (startDate.value) {
      params.start_date = startDate.value
    }
    if (endDate.value) {
      params.end_date = endDate.value
    }
    const res = await sbApi.getBoardArticles(boardId, params)
    if (res.success && res.data) {
      const newArticles = res.data
      if (append) {
        timelineArticles.value.push(...newArticles)
        timelinePage.value = page
      } else {
        timelineArticles.value = newArticles
        timelinePage.value = 1
      }
      const total = res.pagination?.total ?? 0
      timelineHasMore.value = timelineArticles.value.length < total
    } else {
      if (!append) timelineArticles.value = []
    }
  } catch {
    if (!append) timelineArticles.value = []
  } finally {
    timelineLoading.value = false
  }
}

function handleLoadMore() {
  if (selectedBoardId.value !== null && !timelineLoading.value) {
    void loadTimelineArticles(selectedBoardId.value, true)
  }
}

async function openArticlePreview(articleId: number) {
  loadingPreviewArticle.value = true
  try {
    const response = await articlesApi.getArticle(articleId)
    if (!response.success || !response.data) return
    selectedPreviewArticle.value = normalizeArticle(response.data as unknown as ArticlePayload)
    previewArticles.value = []
  } catch (error) {
    console.error('Failed to open article preview:', error)
  } finally {
    loadingPreviewArticle.value = false
  }
}

function closeArticlePreview() {
  selectedPreviewArticle.value = null
}

async function handleArticleFavorite(articleId: string) {
  const currentFavorite = selectedPreviewArticle.value?.id === articleId
    ? selectedPreviewArticle.value.favorite
    : previewArticles.value.find(a => a.id === articleId)?.favorite

  const response = await articlesApi.updateArticle(Number(articleId), { favorite: !currentFavorite })
  if (!response.success) return

  const target = previewArticles.value.find(article => article.id === articleId)
  if (target) {
    target.favorite = !target.favorite
  }

  if (selectedPreviewArticle.value?.id === articleId) {
    selectedPreviewArticle.value = {
      ...selectedPreviewArticle.value,
      favorite: !selectedPreviewArticle.value.favorite,
    }
  }
}

function handleArticleUpdate(articleId: string, updates: Partial<Article>) {
  const target = previewArticles.value.find(article => article.id === articleId)
  if (target) {
    Object.assign(target, updates)
  }

  if (selectedPreviewArticle.value?.id === articleId) {
    Object.assign(selectedPreviewArticle.value, updates)
  }
}

function handleFilterLabel(labelId: number | null) {
  activeFilterLabelId.value = labelId
  timelinePage.value = 1
  if (selectedBoardId.value !== null) {
    void loadTimelineArticles(selectedBoardId.value)
  }
}

function handleFilterChange() {
  timelinePage.value = 1
  if (selectedBoardId.value !== null) {
    void loadTimelineArticles(selectedBoardId.value)
  }
}

function matchInfoTooltip(tag: BoardArticleTag): string {
  const reasonMap: Record<string, string> = {
    direct_hit: '直接命中',
    hit_rate: '命中率',
    max_sim: '相似度',
    weighted: '综合',
  }
  const reason = reasonMap[tag.match_reason] || tag.match_reason
  return `${reason} · ${tag.score.toFixed(2)}`
}

function handleAddBoard(data: { label: string; description: string; display_order: number; protected: boolean; auxiliary_labels?: number[] }) {
  sbApi.createBoard(data).then((res) => {
    if (res.success) {
      showAddDialog.value = false
      void nextTick().then(() => loadBoards())
    } else {
      boardsError.value = res.error || '添加失败'
    }
  })
}

function handleDeleteBoard(id: number) {
  const board = boards.value.find(b => b.id === id)
  if (!board) return
  const msg = board.protected ? `此板块受保护，确认删除？` : `确认删除板块"${board.label}"？`
  if (!confirm(msg)) return
  sbApi.deleteBoard(id).then((res) => {
    if (res.success) {
      if (selectedBoardId.value === id) selectedBoardId.value = null
      void loadBoards()
    } else {
      boardsError.value = res.error || '删除失败'
    }
  })
}

function handleRemoveComposition(auxiliaryLabelId: number) {
  if (selectedBoardId.value === null) return
  sbApi.removeFromComposition(selectedBoardId.value, auxiliaryLabelId).then(() => {
    void loadComposition(selectedBoardId.value!)
  })
}

async function handleUpgradeSuggest() {
  upgradeLoading.value = true
  showUpgradeDialog.value = true
  upgradeBackfillNotice.value = false
  const res = await sbApi.getUpgradeCandidates()
  if (res.success && res.data) {
    upgradeCandidates.value = res.data.candidates
    upgradeClusters.value = res.data.clusters
  }
  upgradeLoading.value = false
}

async function handleSuggestUpgrade() {
  upgradeSuggesting.value = true
  upgradeBackfillNotice.value = false
  const res = await sbApi.suggestUpgrade()
  if (res.success && res.data) {
    upgradeSuggestions.value = res.data.suggestions
  }
  upgradeSuggesting.value = false
}

async function handleExecuteUpgrade(suggestion: UpgradeSuggestion) {
  if (suggestion.decision === 'skip') return
  const res = await sbApi.executeUpgrade({
    decision: suggestion.decision,
    board_label: suggestion.board_label,
    description: suggestion.description,
    target_board_id: suggestion.target_board_id,
    auxiliary_label_ids: suggestion.auxiliary_label_ids,
  })
  if (res.success) {
    upgradeSuggestions.value = upgradeSuggestions.value.filter(item => item !== suggestion)
    upgradeBackfillNotice.value = true
    void loadBoards()
  }
}

async function handleTriggerBackfill() {
  const res = await sbApi.triggerBackfill({ mode: 'all' })
  if (res.success && res.data) {
    backfillTask.value = res.data
    startBackfillPolling(res.data.id)
  }
}

function startBackfillPolling(id: string) {
  if (backfillPollTimer) clearInterval(backfillPollTimer)
  backfillPollTimer = setInterval(async () => {
    const res = await sbApi.getBackfillStatus(id)
    if (res.success && res.data) {
      backfillTask.value = res.data
      if (res.data.status === 'completed' || res.data.status === 'failed') {
        if (backfillPollTimer) clearInterval(backfillPollTimer)
        backfillPollTimer = null
      }
    }
  }, 2000)
}

async function handleOpenMatchingConfig() {
  matchingConfigLoading.value = true
  showMatchingConfigDialog.value = true
  const res = await sbApi.getMatchingConfig()
  if (res.success && res.data) {
    matchingConfig.value = res.data
  }
  matchingConfigLoading.value = false
}

async function handleSaveMatchingConfig(data: Partial<MatchingConfig>) {
  const res = await sbApi.updateMatchingConfig(data)
  if (res.success) {
    showMatchingConfigDialog.value = false
  }
}

function handleDisableAuxLabel(id: number) {
  auxApi.disableLabel(id).then(() => void loadAuxiliaryLabels())
}

function handleMergeAuxLabel(sourceId: number, targetId: number) {
  auxApi.mergeAlias(sourceId, targetId).then(() => void loadAuxiliaryLabels())
}

watch(auxSearchQuery, () => { auxPage.value = 1; void loadAuxiliaryLabels() })
watch(auxStatusFilter, () => { auxPage.value = 1; void loadAuxiliaryLabels() })

onMounted(() => {
  void loadBoards()
  void loadAuxiliaryLabels()
  void loadClusters()
})

onUnmounted(() => {
  if (backfillPollTimer) clearInterval(backfillPollTimer)
})
</script>

<template>
  <div class="tags-page">
    <!-- Top bar -->
    <div class="tags-topbar">
      <div class="tags-topbar-inner">
        <div class="flex items-center gap-3">
          <NuxtLink to="/" class="tags-back-btn" title="返回首页">
            <Icon icon="mdi:arrow-left" width="16" />
          </NuxtLink>
          <Icon icon="mdi:view-grid" width="18" class="text-white/50" />
          <h1 class="tags-page-title">语义板块管理</h1>
        </div>
      </div>
    </div>

    <!-- Main content -->
    <div class="tags-main">
      <!-- Left panel: Board list -->
      <aside class="tags-sidebar">
        <div v-if="boardsError" class="tags-sidebar-error">
          <Icon icon="mdi:alert-circle-outline" width="14" />
          <span>{{ boardsError }}</span>
        </div>
        <SemanticBoardList
          :boards="boards"
          :selected-id="selectedBoardId"
          :loading="boardsLoading"
          :search-query="''"
          @select="handleSelectBoard"
          @add="showAddDialog = true"
          @upgrade="handleUpgradeSuggest"
          @backfill="handleTriggerBackfill"
          @config="handleOpenMatchingConfig"
          @generate="showGenerateDialog = true"
          @delete="handleDeleteBoard"
        />
      </aside>

      <!-- Right panel -->
      <main class="tags-content">
        <div v-if="selectedBoardId !== null">
          <!-- Content tabs -->
          <div class="tags-content-tabs">
            <button type="button" class="tags-content-tab" :class="{ 'tags-content-tab--active': contentTab === 'composition' }" @click="contentTab = 'composition'">
              <Icon icon="mdi:view-dashboard-outline" width="14" />
              板块内容
            </button>
            <button type="button" class="tags-content-tab" :class="{ 'tags-content-tab--active': contentTab === 'narratives' }" @click="contentTab = 'narratives'">
              <Icon icon="mdi:timeline-text-outline" width="14" />
              叙事
            </button>
            <button type="button" class="tags-content-tab" :class="{ 'tags-content-tab--active': contentTab === 'articles' }" @click="contentTab = 'articles'">
              <Icon icon="mdi:newspaper-variant-outline" width="14" />
              文章
            </button>
          </div>

          <BoardCompositionPanel
            v-if="contentTab === 'composition'"
            :board-id="selectedBoardId"
            :labels="compositionLabels"
            :loading="compositionLoading"
            @remove="handleRemoveComposition"
            @refresh="() => loadComposition(selectedBoardId!)"
          />

          <!-- Board Narrative Timeline -->
          <BoardNarrativeTimeline v-if="contentTab === 'narratives'" :board-id="selectedBoardId" />

          <!-- Article timeline -->
          <div v-if="contentTab === 'articles'" class="tags-timeline">
            <div class="tags-timeline-header">
              <Icon icon="mdi:timeline-clock-outline" width="15" class="text-[rgba(240,138,75,0.8)]" />
              <span class="tags-timeline-title">相关文章</span>
              <span v-if="timelineArticles.length" class="tags-timeline-count">{{ timelineArticles.length }} 篇</span>
            </div>

            <!-- Filter chips -->
            <div v-if="compositionLabels.length > 0 || selectedBoardId !== null" class="tags-filter-chips">
              <button
                v-if="compositionLabels.length > 0"
                type="button"
                class="tags-filter-chip"
                :class="{ 'tags-filter-chip--active': activeFilterLabelId === null }"
                @click="handleFilterLabel(null)"
              >
                全部
              </button>
              <button
                v-for="label in compositionLabels"
                :key="label.id"
                type="button"
                class="tags-filter-chip"
                :class="{ 'tags-filter-chip--active': activeFilterLabelId === label.id }"
                @click="handleFilterLabel(label.id)"
              >
                {{ label.label }}
              </button>
              <select v-model="filterFeedId" class="tags-filter-select" @change="handleFilterChange()">
                <option :value="null">全部来源</option>
                <option v-for="feed in feedOptions" :key="feed.id" :value="Number(feed.id)">{{ feed.title }}</option>
              </select>
              <input type="date" v-model="startDate" class="tags-filter-date" @change="handleFilterChange()" />
              <input type="date" v-model="endDate" class="tags-filter-date" @change="handleFilterChange()" />
            </div>

            <div v-if="timelineLoading && timelineArticles.length === 0" class="tags-timeline-loading">
              <div v-for="i in 3" :key="i" class="th-skeleton" />
            </div>

            <div v-else-if="timelineArticles.length === 0" class="tags-timeline-empty">
              <Icon icon="mdi:newspaper-variant-outline" width="28" class="text-white/15" />
              <p>暂无相关文章</p>
            </div>

            <div v-else class="tags-timeline-list">
              <div
                v-for="article in timelineArticles"
                :key="article.id"
                class="tags-timeline-item"
                @click="openArticlePreview(article.id)"
              >
                <div class="tags-timeline-item-meta">
                  <span class="tags-timeline-item-date">
                    {{ new Date(article.pub_date).toLocaleString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' }) }}
                  </span>
                  <span v-if="article.feed_name" class="tags-timeline-item-feed-name">{{ article.feed_name }}</span>
                </div>
                <div class="tags-timeline-item-content">
                  <span class="tags-timeline-item-title">{{ article.title }}</span>
                  <div v-if="article.filtered_tags?.length" class="tags-timeline-item-tags">
                    <span
                      v-for="tag in article.filtered_tags"
                      :key="tag.id"
                      class="tags-timeline-tag-chip"
                      :title="matchInfoTooltip(tag)"
                    >
                      {{ tag.label }}
                    </span>
                  </div>
                </div>
              </div>
            </div>
            <button
              v-if="timelineHasMore"
              type="button"
              class="tags-timeline-more"
              :disabled="timelineLoading"
              @click="handleLoadMore"
            >
              <template v-if="timelineLoading">加载中...</template>
              <template v-else>加载更多</template>
            </button>
          </div>
        </div>

        <div v-else>
          <AuxiliaryLabelPool
            :labels="auxiliaryLabels"
            :clusters="auxClusters"
            :unclustered-count="auxUnclusteredCount"
            :loading="auxLoading"
            :search-query="auxSearchQuery"
            :status-filter="auxStatusFilter"
            :pagination="auxPagination"
            @update:search-query="auxSearchQuery = $event"
            @update:status-filter="auxStatusFilter = $event"
            @update:page="handleUpdatePage"
            @disable="handleDisableAuxLabel"
            @merge="handleMergeAuxLabel"
            @refresh="loadAuxiliaryLabels"
            @select-cluster="() => {}"
          />
        </div>
      </main>
    </div>

    <!-- Bottom bar -->
    <div class="tags-bottombar">
      <BackfillProgress :task="backfillTask" />
    </div>

    <!-- Add Board Dialog -->
    <AddSemanticBoardDialog
      :visible="showAddDialog"
      @confirm="handleAddBoard"
      @cancel="showAddDialog = false"
    />

    <!-- Upgrade Suggestion Panel -->
    <UpgradeSuggestionPanel
      :visible="showUpgradeDialog"
      :candidates="upgradeCandidates"
      :clusters="upgradeClusters"
      :suggestions="upgradeSuggestions"
      :loading="upgradeLoading"
      :suggesting="upgradeSuggesting"
      :backfill-notice="upgradeBackfillNotice"
      @suggest="handleSuggestUpgrade"
      @execute="handleExecuteUpgrade"
      @cancel="showUpgradeDialog = false"
    />

    <!-- Matching Config Dialog -->
    <MatchingConfigDialog
      :visible="showMatchingConfigDialog"
      :config="matchingConfig"
      :loading="matchingConfigLoading"
      @save="handleSaveMatchingConfig"
      @cancel="showMatchingConfigDialog = false"
    />

    <!-- Narrative Generate Dialog -->
    <NarrativeGenerateDialog
      :visible="showGenerateDialog"
      :boards="boards"
      @cancel="showGenerateDialog = false"
    />

    <Teleport to="body">
      <div
        v-if="selectedPreviewArticle"
        class="tags-article-modal"
        @click.self="closeArticlePreview"
      >
        <div class="tags-article-modal__panel">
          <header class="tags-article-modal__header">
            <p class="truncate text-sm text-ink-medium">
              {{ loadingPreviewArticle ? '正在准备文章预览...' : '文章预览' }}
            </p>
            <button
              class="btn-ghost min-h-11 min-w-11 px-0"
              type="button"
              aria-label="关闭文章弹窗"
              @click="closeArticlePreview"
            >
              <Icon icon="mdi:close" width="18" />
            </button>
          </header>
          <div class="tags-article-modal__body">
            <ArticleContentView
              :article="selectedPreviewArticle"
              :articles="previewArticles"
              @navigate="selectedPreviewArticle = $event"
              @favorite="handleArticleFavorite"
              @article-update="handleArticleUpdate"
            />
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.tags-page {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: #080c12;
  color: rgba(255, 255, 255, 0.85);
}

.tags-topbar {
  position: sticky;
  top: 0;
  z-index: 30;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(8, 12, 18, 0.92);
  backdrop-filter: blur(16px);
}

.tags-topbar-inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  max-width: 1440px;
  margin: 0 auto;
  padding: 0.75rem 1.5rem;
}

.tags-back-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 8px;
  color: rgba(255, 255, 255, 0.35);
  text-decoration: none;
  transition: all 0.12s ease;
}

.tags-back-btn:hover {
  border-color: rgba(255, 255, 255, 0.2);
  color: rgba(255, 255, 255, 0.6);
  background: rgba(255, 255, 255, 0.04);
}

.tags-page-title {
  font-family: serif;
  font-size: 1.1rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.9);
  letter-spacing: 0.02em;
}

.tags-main {
  display: flex;
  flex: 1;
  min-height: 0;
  max-width: 1440px;
  width: 100%;
  margin: 0 auto;
}

.tags-sidebar {
  width: 260px;
  flex-shrink: 0;
  border-right: 1px solid rgba(255, 255, 255, 0.05);
  padding: 1rem;
  overflow-y: auto;
}

.tags-sidebar-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.6rem 0.8rem;
  margin-bottom: 0.75rem;
  border-radius: 10px;
  border: 1px solid rgba(240, 138, 75, 0.25);
  background: rgba(240, 138, 75, 0.08);
  color: rgba(255, 200, 180, 0.85);
  font-size: 0.75rem;
}

.tags-content {
  flex: 1;
  min-width: 0;
  padding: 1.25rem 1.5rem 3.5rem;
  overflow-y: auto;
}

.tags-content-tabs {
  display: flex;
  gap: 0.25rem;
  padding: 0 0 0.75rem;
  margin-bottom: 1rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.tags-content-tab {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.4rem 0.75rem;
  border: none;
  border-radius: 8px 8px 0 0;
  background: none;
  color: rgba(255, 255, 255, 0.4);
  font-size: 0.75rem;
  cursor: pointer;
  transition: all 0.12s ease;
  position: relative;
}

.tags-content-tab:hover {
  color: rgba(255, 255, 255, 0.65);
  background: rgba(255, 255, 255, 0.03);
}

.tags-content-tab--active {
  color: rgba(255, 220, 200, 0.85);
  background: rgba(240, 138, 75, 0.08);
}

.tags-content-tab--active::after {
  content: '';
  position: absolute;
  bottom: -1px;
  left: 0;
  right: 0;
  height: 2px;
  background: rgba(240, 138, 75, 0.6);
  border-radius: 1px;
}

.tags-bottombar {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  z-index: 40;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 0.75rem;
  padding: 0.45rem 1.25rem;
  border-top: 1px solid rgba(255, 255, 255, 0.04);
  background: rgba(8, 12, 18, 0.88);
  backdrop-filter: blur(12px);
  transition: all 0.2s ease;
}

.tags-timeline {
  margin-top: 2rem;
  padding-top: 1.5rem;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}

.tags-timeline-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-bottom: 0.75rem;
}

.tags-timeline-title {
  font-family: serif;
  font-size: 0.9rem;
  color: rgba(255, 255, 255, 0.8);
}

.tags-timeline-count {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
  padding: 0.1rem 0.45rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
}

.tags-timeline-loading {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.th-skeleton {
  height: 36px;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.03);
  animation: thPulse 1.5s ease-in-out infinite;
}

@keyframes thPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

.tags-timeline-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.4rem;
  padding: 2.5rem 0;
  color: rgba(255, 255, 255, 0.3);
  font-size: 0.8rem;
}

.tags-timeline-list {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.tags-timeline-item {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  padding: 0.5rem 0.65rem;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.12s ease;
}

.tags-timeline-item:hover {
  background: rgba(255, 255, 255, 0.03);
}

.tags-timeline-item-meta {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.tags-timeline-item-date {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
  white-space: nowrap;
}

.tags-timeline-item-feed-name {
  font-size: 0.62rem;
  color: rgba(255, 255, 255, 0.25);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tags-timeline-item-content {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  min-width: 0;
}

.tags-timeline-item-title {
  font-size: 0.8rem;
  color: rgba(255, 255, 255, 0.7);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition: color 0.12s ease;
}

.tags-timeline-item-title:hover {
  color: rgba(255, 220, 200, 0.9);
}

.tags-timeline-item-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem;
  margin-top: 0.15rem;
}

.tags-timeline-tag-chip {
  display: inline-flex;
  align-items: center;
  padding: 0.1rem 0.4rem;
  font-size: 0.62rem;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.06);
  color: rgba(255, 255, 255, 0.5);
  cursor: default;
  transition: background 0.12s ease;
}

.tags-timeline-tag-chip:hover {
  background: rgba(255, 255, 255, 0.1);
}

.tags-filter-select {
  padding: 0.2rem 0.4rem;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.45);
  font-size: 0.7rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.tags-filter-select:hover {
  border-color: rgba(255, 255, 255, 0.18);
  color: rgba(255, 255, 255, 0.65);
}

.tags-filter-select option {
  background: #1a1f2a;
  color: rgba(255, 255, 255, 0.85);
}

.tags-filter-date {
  padding: 0.2rem 0.4rem;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.45);
  font-size: 0.7rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.tags-filter-date:hover {
  border-color: rgba(255, 255, 255, 0.18);
  color: rgba(255, 255, 255, 0.65);
}

.tags-filter-date::-webkit-calendar-picker-indicator {
  filter: invert(0.5);
}

.tags-timeline-more {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  margin-top: 0.75rem;
  padding: 0.5rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 10px;
  background: none;
  color: rgba(255, 255, 255, 0.4);
  font-size: 0.75rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.tags-timeline-more:hover {
  border-color: rgba(255, 255, 255, 0.15);
  color: rgba(255, 255, 255, 0.6);
  background: rgba(255, 255, 255, 0.03);
}

.tags-timeline-more:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.tags-timeline-loading-more {
  margin-top: 0.5rem;
}

.tags-filter-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  margin-bottom: 0.75rem;
  padding-bottom: 0.75rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.tags-filter-chip {
  padding: 0.2rem 0.55rem;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: none;
  color: rgba(255, 255, 255, 0.45);
  font-size: 0.7rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.tags-filter-chip:hover {
  border-color: rgba(255, 255, 255, 0.18);
  color: rgba(255, 255, 255, 0.65);
  background: rgba(255, 255, 255, 0.03);
}

.tags-filter-chip--active {
  border-color: rgba(240, 138, 75, 0.45);
  color: rgba(255, 220, 200, 0.85);
  background: rgba(240, 138, 75, 0.1);
}

.tags-article-modal {
  position: fixed;
  inset: 0;
  z-index: 80;
  display: flex;
  align-items: stretch;
  justify-content: center;
  background: rgba(8, 12, 18, 0.7);
  padding: 1rem;
  backdrop-filter: blur(10px);
}

.tags-article-modal__panel {
  display: flex;
  height: calc(100vh - 2rem);
  width: min(1500px, 100%);
  flex-direction: column;
  overflow: hidden;
  border-radius: 1.75rem;
  background: rgba(255, 252, 248, 0.98);
  box-shadow: 0 30px 100px rgba(0, 0, 0, 0.28);
}

.tags-article-modal__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  border-bottom: 1px solid rgba(20, 33, 44, 0.08);
  padding: 1rem 1.25rem;
}

.tags-article-modal__body {
  min-height: 0;
  flex: 1;
}
</style>
