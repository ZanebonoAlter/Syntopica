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
import AddSemanticBoardDialog from './AddSemanticBoardDialog.vue'
import BoardCompositionPanel from './BoardCompositionPanel.vue'
import AuxiliaryLabelPool from './AuxiliaryLabelPool.vue'
import UpgradeSuggestionPanel from './UpgradeSuggestionPanel.vue'
import BackfillProgress from './BackfillProgress.vue'
import MatchingConfigDialog from './MatchingConfigDialog.vue'
import NarrativeGenerateDialog from './NarrativeGenerateDialog.vue'
import BoardDailyReportTimeline from './BoardDailyReportTimeline.vue'
import MatchDetailPanel from './MatchDetailPanel.vue'

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

const contentTab = ref<'composition' | 'daily-reports' | 'articles'>('composition')
const showAddDialog = ref(false)
const showUpgradeDialog = ref(false)
const showMatchingConfigDialog = ref(false)
const showGenerateDialog = ref(false)
const editingBoard = ref<SemanticBoard | null>(null)
const editLabel = ref('')
const editDescription = ref('')
const editSaving = ref(false)
const editError = ref<string | null>(null)

const timelineArticles = ref<BoardArticle[]>([])
const timelineLoading = ref(false)
const timelinePage = ref(1)
const timelineHasMore = ref(false)
const timelinePerPage = 50
const activeFilterLabelId = ref<number | null>(null)
const filterFeedId = ref<number | null>(null)
const startDate = ref<string>('')
const endDate = ref<string>('')
const showDirectionMismatch = ref(false)
const feedOptions = computed(() => feedsStore.feeds)
const timelineVisible = computed(() => selectedBoardId.value !== null)
const selectedTagForDetail = ref<BoardArticleTag | null>(null)
const timelineDisplayArticles = computed(() => timelineArticles.value.map((article) => {
  if (showDirectionMismatch.value) return article
  return {
    ...article,
    filtered_tags: (article.filtered_tags || []).filter(tag => !tag.direction_mismatch),
  }
}))

const quickRange = ref<'today' | '3d' | '7d' | '30d' | null>('today')

function getDateStr(d: Date): string {
  return d.toISOString().slice(0, 10)
}

function applyQuickRange(range: 'today' | '3d' | '7d' | '30d') {
  quickRange.value = range
  const now = new Date()
  endDate.value = getDateStr(now)
  const start = new Date()
  if (range === '3d') {
    start.setDate(start.getDate() - 2)
  } else if (range === '7d') {
    start.setDate(start.getDate() - 6)
  } else if (range === '30d') {
    start.setDate(start.getDate() - 29)
  }
  startDate.value = getDateStr(start)
  handleFilterChange()
}

function handleDateInputChange() {
  quickRange.value = null
  handleFilterChange()
}

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
  selectedTagForDetail.value = null
  activeFilterLabelId.value = null
  filterFeedId.value = null
  quickRange.value = 'today'
  const now = new Date()
  startDate.value = getDateStr(now)
  endDate.value = getDateStr(now)
  contentTab.value = 'articles'
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
    if (showDirectionMismatch.value) {
      params.show_direction_mismatch = true
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

function sourceIcon(source: string): string {
  switch (source) {
    case 'manual': return 'mdi:lock'
    case 'llm_extract': return 'mdi:robot'
    default: return 'mdi:lightning-bolt'
  }
}

function sourceTitle(source: string): string {
  switch (source) {
    case 'manual': return '手动创建'
    case 'llm_extract': return 'LLM 生成'
    default: return '自动生成'
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
  selectedTagForDetail.value = null
  timelinePage.value = 1
  if (selectedBoardId.value !== null) {
    void loadTimelineArticles(selectedBoardId.value)
  }
}

function handleFilterChange() {
  selectedTagForDetail.value = null
  timelinePage.value = 1
  if (selectedBoardId.value !== null) {
    void loadTimelineArticles(selectedBoardId.value)
  }
}

function toggleMatchDetail(tag: BoardArticleTag) {
  selectedTagForDetail.value = selectedTagForDetail.value?.id === tag.id ? null : tag
}

function isSelectedDetailTag(tag: BoardArticleTag): boolean {
  return selectedTagForDetail.value?.id === tag.id
}

function matchReasonColor(reason: string, downgraded?: boolean): string {
  const colors: Record<string, string> = {
    direct_hit: '#22c55e',
    hit_rate: '#3b82f6',
    max_sim: '#f59e0b',
    weighted: '#94a3b8',
  }
  const color = colors[reason] || '#94a3b8'
  return downgraded ? color + '80' : color
}

function matchInfoLabel(tag: BoardArticleTag): string {
  const labels: Record<string, string> = {
    direct_hit: '直接命中',
    hit_rate: '命中率',
    max_sim: '相似度',
    weighted: '综合',
  }
  return `${labels[tag.match_reason] || tag.match_reason} ${tag.score.toFixed(2)}${tag.downgraded ? '↓' : ''}`
}

function strongestMatch(tags: BoardArticleTag[]): BoardArticleTag | null {
  if (!tags?.length) return null
  const [first, ...rest] = tags
  if (!first) return null
  return rest.reduce((best, t) => t.score > best.score ? t : best, first)
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

function openEditBoard(board: SemanticBoard) {
  editingBoard.value = board
  editLabel.value = board.label
  editDescription.value = board.description || ''
  editError.value = null
}

function closeEditBoard() {
  if (editSaving.value) return
  editingBoard.value = null
  editLabel.value = ''
  editDescription.value = ''
  editError.value = null
}

async function handleSaveBoardEdit() {
  const board = editingBoard.value
  const label = editLabel.value.trim()
  if (!board || !label) return

  editSaving.value = true
  editError.value = null
  try {
    const res = await sbApi.updateBoard(board.id, {
      label,
      description: editDescription.value.trim(),
    })
    if (res.success) {
      await loadBoards()
      editSaving.value = false
      closeEditBoard()
    } else {
      editError.value = res.error || '保存失败'
    }
  } finally {
    editSaving.value = false
  }
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
        <div class="sb-list">
          <div class="sb-list-header">
            <span class="sb-list-title">语义板块</span>
            <span class="sb-list-count">{{ boards.length }}</span>
          </div>

          <div
            class="sb-item"
            :class="{ 'sb-item--active': selectedBoardId === null }"
            @click="handleSelectBoard(null)"
          >
            <Icon icon="mdi:view-grid" width="14" class="sb-item-icon" />
            <span class="sb-item-label">全部</span>
            <span class="sb-item-badge">{{ boards.reduce((s, x) => s + x.tag_count, 0) }}</span>
          </div>

          <div v-if="boardsLoading" class="sb-loading">
            <div v-for="i in 3" :key="i" class="sb-skeleton" />
          </div>

          <div v-else-if="boards.length === 0" class="sb-empty">
            <Icon icon="mdi:folder-outline" width="24" class="text-white/15" />
            <p>暂无板块</p>
          </div>

          <div v-else class="sb-items">
            <div
              v-for="board in boards"
              :key="board.id"
              class="sb-item"
              :class="{
                'sb-item--active': selectedBoardId === board.id,
                'sb-item--protected': board.protected,
              }"
              @click="handleSelectBoard(board.id)"
            >
              <Icon
                :icon="sourceIcon(board.source)"
                width="13"
                class="sb-source-icon"
                :title="sourceTitle(board.source)"
              />
              <span class="sb-item-label">{{ board.label }}</span>
              <span v-if="board.tag_count > 0" class="sb-item-badge">{{ board.tag_count }}</span>
              <button
                type="button"
                class="sb-icon-btn sb-edit-btn"
                title="编辑板块"
                @click.stop="openEditBoard(board)"
              >
                <Icon icon="mdi:pencil" width="12" />
              </button>
              <button
                type="button"
                class="sb-icon-btn sb-delete-btn"
                title="删除板块"
                @click.stop="handleDeleteBoard(board.id)"
              >
                <Icon icon="mdi:close" width="12" />
              </button>
            </div>
          </div>

          <div class="sb-actions">
            <button type="button" class="sb-action-btn sb-action-btn--primary" @click="showAddDialog = true">
              <Icon icon="mdi:plus" width="14" />
              添加板块
            </button>
            <button type="button" class="sb-action-btn sb-action-btn--secondary" @click="handleUpgradeSuggest">
              <Icon icon="mdi:auto-fix" width="14" />
              升级建议
            </button>
            <button type="button" class="sb-action-btn sb-action-btn--secondary" @click="handleTriggerBackfill">
              <Icon icon="mdi:backup-restore" width="14" />
              匹配回填
            </button>
            <button type="button" class="sb-action-btn sb-action-btn--ghost" @click="handleOpenMatchingConfig">
              <Icon icon="mdi:tune" width="14" />
              匹配参数
            </button>
            <button type="button" class="sb-action-btn sb-action-btn--ghost" @click="showGenerateDialog = true">
              <Icon icon="mdi:auto-fix" width="14" />
              整理叙事
            </button>
          </div>
        </div>
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
            <button type="button" class="tags-content-tab" :class="{ 'tags-content-tab--active': contentTab === 'daily-reports' }" @click="contentTab = 'daily-reports'">
              <Icon icon="mdi:file-document-outline" width="14" />
              日报
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

          <BoardDailyReportTimeline v-if="contentTab === 'daily-reports'" :board-id="selectedBoardId" @open-article="openArticlePreview" />

          <!-- Article timeline -->
          <div v-if="contentTab === 'articles'" class="tags-articles-layout">
            <div class="tags-timeline">
              <div class="tags-timeline-header">
              <Icon icon="mdi:timeline-clock-outline" width="15" class="text-[rgba(240,138,75,0.8)]" />
              <span class="tags-timeline-title">相关文章</span>
              <span v-if="timelineArticles.length" class="tags-timeline-count">{{ timelineArticles.length }} 篇</span>
              <label class="tags-direction-toggle">
                <input v-model="showDirectionMismatch" type="checkbox" @change="handleFilterChange()" />
                显示方向不符
              </label>
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
              <div class="tags-quick-range">
                <button
                  v-for="opt in [
                    { key: 'today', label: '今天' },
                    { key: '3d', label: '3天' },
                    { key: '7d', label: '7天' },
                    { key: '30d', label: '30天' },
                  ]"
                  :key="opt.key"
                  type="button"
                  class="tags-filter-chip"
                  :class="{ 'tags-filter-chip--active': quickRange === opt.key }"
                  @click="applyQuickRange(opt.key as 'today' | '3d' | '7d' | '30d')"
                >
                  {{ opt.label }}
                </button>
              </div>
              <select v-model="filterFeedId" class="tags-filter-select" @change="handleFilterChange()">
                <option :value="null">全部来源</option>
                <option v-for="feed in feedOptions" :key="feed.id" :value="Number(feed.id)">{{ feed.title }}</option>
              </select>
              <input type="date" v-model="startDate" class="tags-filter-date" @change="handleDateInputChange()" />
              <input type="date" v-model="endDate" class="tags-filter-date" @change="handleDateInputChange()" />
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
                v-for="article in timelineDisplayArticles"
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
                <div class="tags-timeline-item-body">
                  <div class="tags-timeline-item-content">
                    <span class="tags-timeline-item-title">{{ article.title }}</span>
                    <div v-if="article.filtered_tags?.length" class="tags-timeline-item-tags">
                      <button
                        v-for="tag in article.filtered_tags"
                        :key="tag.id"
                        type="button"
                        class="tags-timeline-tag-chip"
                        :class="{
                          'tags-timeline-tag-chip--selected': isSelectedDetailTag(tag),
                          'tags-timeline-tag-chip--direction-mismatch': tag.direction_mismatch,
                        }"
                        :style="{ borderColor: matchReasonColor(tag.match_reason, tag.downgraded) }"
                        :title="matchInfoLabel(tag)"
                        @click.stop="toggleMatchDetail(tag)"
                      >
                        {{ tag.label }} {{ tag.score.toFixed(2) }}{{ tag.downgraded ? '↓' : '' }}{{ tag.direction_mismatch ? '⊘' : '' }}
                      </button>
                    </div>
                  </div>
                  <span
                    v-if="strongestMatch(article.filtered_tags)"
                    class="tags-timeline-item-match-info"
                    :style="{ color: matchReasonColor(strongestMatch(article.filtered_tags)!.match_reason) }"
                  >
                    {{ matchInfoLabel(strongestMatch(article.filtered_tags)!) }}
                  </span>
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
            <Transition name="match-detail-panel">
              <MatchDetailPanel
                v-if="selectedTagForDetail && selectedBoardId !== null"
                :board-id="selectedBoardId"
                :tag="selectedTagForDetail"
                class="tags-match-detail-panel"
                @close="selectedTagForDetail = null"
              />
            </Transition>
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

    <!-- Edit Board Dialog -->
    <Teleport to="body">
      <div v-if="editingBoard" class="board-edit-overlay" @click.self="closeEditBoard">
        <form class="board-edit-card" @submit.prevent="handleSaveBoardEdit">
          <div class="board-edit-header">
            <h3 class="board-edit-title">编辑板块</h3>
            <button type="button" class="board-edit-close" :disabled="editSaving" @click="closeEditBoard">
              <Icon icon="mdi:close" width="18" />
            </button>
          </div>
          <div class="board-edit-body">
            <label class="board-edit-field">
              <span class="board-edit-label">名称 <span class="board-edit-required">*</span></span>
              <input v-model="editLabel" type="text" class="board-edit-input" placeholder="板块名称" maxlength="100" autofocus />
            </label>
            <label class="board-edit-field">
              <span class="board-edit-label">描述</span>
              <textarea v-model="editDescription" class="board-edit-textarea" placeholder="可选描述" maxlength="500" rows="4" />
            </label>
            <p v-if="editError" class="board-edit-error">{{ editError }}</p>
          </div>
          <div class="board-edit-footer">
            <button type="button" class="board-edit-btn board-edit-btn--ghost" :disabled="editSaving" @click="closeEditBoard">取消</button>
            <button type="submit" class="board-edit-btn board-edit-btn--primary" :disabled="editSaving || !editLabel.trim()">
              {{ editSaving ? '保存中...' : '保存' }}
            </button>
          </div>
        </form>
      </div>
    </Teleport>

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
  max-width: min(1800px, 95vw);
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
  max-width: min(1800px, 95vw);
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

.sb-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.sb-list-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 0.25rem;
  margin-bottom: 0.25rem;
}

.sb-list-title {
  font-size: 0.7rem;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: rgba(255, 255, 255, 0.4);
}

.sb-list-count {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
  padding: 0.1rem 0.45rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
}

.sb-item {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.45rem 0.6rem;
  border-radius: 10px;
  cursor: pointer;
  transition: all 0.12s ease;
  position: relative;
}

.sb-item:hover {
  background: rgba(255, 255, 255, 0.04);
}

.sb-item--active {
  background: rgba(240, 138, 75, 0.1);
  border: 1px solid rgba(240, 138, 75, 0.2);
}

.sb-item--protected .sb-source-icon {
  color: rgba(240, 138, 75, 0.6);
}

.sb-item-icon,
.sb-source-icon {
  color: rgba(255, 255, 255, 0.3);
  flex-shrink: 0;
}

.sb-item-label {
  flex: 1;
  font-size: 0.8rem;
  color: rgba(255, 255, 255, 0.75);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sb-item--active .sb-item-label {
  color: rgba(255, 220, 200, 0.9);
}

.sb-item-badge {
  font-size: 0.6rem;
  color: rgba(255, 255, 255, 0.35);
  padding: 0.1rem 0.4rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
  flex-shrink: 0;
}

.sb-icon-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border: none;
  border-radius: 6px;
  background: none;
  color: rgba(255, 255, 255, 0.15);
  cursor: pointer;
  opacity: 0;
  transition: all 0.12s ease;
  flex-shrink: 0;
}

.sb-item:hover .sb-icon-btn {
  opacity: 1;
}

.sb-edit-btn:hover {
  color: rgba(147, 197, 253, 0.9);
  background: rgba(59, 130, 246, 0.12);
}

.sb-delete-btn:hover {
  color: rgba(252, 165, 165, 0.9);
  background: rgba(239, 68, 68, 0.12);
}

.sb-loading {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.sb-skeleton {
  height: 32px;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.03);
  animation: sbPulse 1.5s ease-in-out infinite;
}

@keyframes sbPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

.sb-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.4rem;
  padding: 2rem 0;
  color: rgba(255, 255, 255, 0.3);
  font-size: 0.75rem;
}

.sb-items {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.sb-actions {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  margin-top: 0.75rem;
  padding-top: 0.75rem;
  border-top: 1px solid rgba(255, 255, 255, 0.05);
}

.sb-action-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
  width: 100%;
  padding: 0.5rem;
  border-radius: 10px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: none;
  font-size: 0.75rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.sb-action-btn--primary {
  color: rgba(255, 220, 200, 0.7);
  border-color: rgba(240, 138, 75, 0.2);
}

.sb-action-btn--primary:hover {
  background: rgba(240, 138, 75, 0.1);
  border-color: rgba(240, 138, 75, 0.35);
  color: rgba(255, 220, 200, 0.9);
}

.sb-action-btn--secondary {
  color: rgba(147, 197, 253, 0.6);
  border-color: rgba(99, 179, 237, 0.2);
}

.sb-action-btn--secondary:hover {
  background: rgba(99, 179, 237, 0.08);
  border-color: rgba(99, 179, 237, 0.35);
  color: rgba(147, 197, 253, 0.9);
}

.sb-action-btn--ghost {
  color: rgba(255, 255, 255, 0.4);
  border-color: rgba(255, 255, 255, 0.06);
}

.sb-action-btn--ghost:hover {
  background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.7);
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

.tags-articles-layout {
  display: flex;
  align-items: flex-start;
  gap: 1rem;
  min-width: 0;
}

.tags-articles-layout .tags-timeline {
  flex: 1;
  min-width: 0;
}

.tags-match-detail-panel {
  width: 320px;
  flex-shrink: 0;
  position: sticky;
  top: 1rem;
  align-self: flex-start;
  max-height: calc(100vh - 6rem);
  overflow-y: auto;
}

.tags-timeline {
  margin-top: 2rem;
  padding-top: 1.5rem;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}

.tags-articles-layout .tags-timeline {
  margin-top: 0;
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

.tags-direction-toggle {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  margin-left: auto;
  font-size: 0.7rem;
  color: rgba(255, 255, 255, 0.38);
  cursor: pointer;
}

.tags-direction-toggle input {
  cursor: pointer;
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

.tags-timeline-item-body {
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;
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
  font-family: inherit;
  border-radius: 4px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(255, 255, 255, 0.06);
  color: rgba(255, 255, 255, 0.5);
  cursor: pointer;
  transition: background 0.12s ease, box-shadow 0.12s ease;
}

.tags-timeline-tag-chip:hover {
  background: rgba(255, 255, 255, 0.1);
}

.tags-timeline-tag-chip--selected {
  box-shadow: 0 0 0 2px rgba(240, 138, 75, 0.35);
  background: rgba(240, 138, 75, 0.12);
}

.tags-timeline-tag-chip--direction-mismatch {
  border-style: dashed;
  opacity: 0.65;
}

.match-detail-panel-enter-active,
.match-detail-panel-leave-active {
  transition: opacity 0.16s ease, transform 0.16s ease;
}

.match-detail-panel-enter-from,
.match-detail-panel-leave-to {
  opacity: 0;
  transform: translateX(12px);
}

.tags-timeline-item-match-info {
  flex-shrink: 0;
  font-size: 0.62rem;
  font-weight: 500;
  white-space: nowrap;
  margin-left: auto;
  padding-left: 0.5rem;
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

.tags-quick-range {
  display: flex;
  gap: 0.25rem;
  margin-right: 0.5rem;
  padding-right: 0.5rem;
  border-right: 1px solid rgba(255, 255, 255, 0.08);
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

.board-edit-overlay {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(8, 12, 18, 0.75);
  backdrop-filter: blur(8px);
}

.board-edit-card {
  width: min(480px, 90%);
  display: flex;
  flex-direction: column;
  border-radius: 1.25rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(17, 27, 38, 0.98);
  padding: 1.5rem;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
}

.board-edit-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 1.25rem;
}

.board-edit-title {
  font-size: 0.95rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.9);
}

.board-edit-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 8px;
  background: none;
  color: rgba(255, 255, 255, 0.4);
  cursor: pointer;
  transition: all 0.12s ease;
}

.board-edit-close:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.7);
}

.board-edit-body {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.board-edit-field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.board-edit-label {
  font-size: 0.72rem;
  color: rgba(255, 255, 255, 0.5);
  letter-spacing: 0.02em;
}

.board-edit-required,
.board-edit-error {
  color: rgba(240, 138, 75, 0.8);
}

.board-edit-input,
.board-edit-textarea {
  width: 100%;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  background: rgba(0, 0, 0, 0.25);
  color: rgba(255, 255, 255, 0.88);
  font-size: 0.82rem;
  padding: 0.55rem 0.85rem;
  outline: none;
  transition: border-color 0.12s ease;
  box-sizing: border-box;
}

.board-edit-textarea {
  resize: vertical;
}

.board-edit-input::placeholder,
.board-edit-textarea::placeholder {
  color: rgba(255, 255, 255, 0.2);
}

.board-edit-input:focus,
.board-edit-textarea:focus {
  border-color: rgba(240, 138, 75, 0.45);
}

.board-edit-error {
  font-size: 0.72rem;
}

.board-edit-footer {
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
  margin-top: 1.25rem;
}

.board-edit-btn {
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  background: none;
  color: rgba(255, 255, 255, 0.7);
  font-size: 0.82rem;
  padding: 0.45rem 1.1rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.board-edit-btn--ghost:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.06);
}

.board-edit-btn--primary {
  border-color: rgba(240, 138, 75, 0.4);
  color: rgba(255, 220, 200, 0.9);
  background: rgba(240, 138, 75, 0.12);
}

.board-edit-btn--primary:hover:not(:disabled) {
  background: rgba(240, 138, 75, 0.2);
  border-color: rgba(240, 138, 75, 0.6);
}

.board-edit-btn:disabled,
.board-edit-close:disabled {
  opacity: 0.4;
  cursor: not-allowed;
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
