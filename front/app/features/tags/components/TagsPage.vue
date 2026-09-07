<script setup lang="ts">
import { computed, nextTick, onMounted, provide, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import ThemeToggle from '~/components/ui/ThemeToggle.vue'
import { useOnboarding } from '~/composables/useOnboarding'
import AddSemanticBoardDialog from './AddSemanticBoardDialog.vue'
import BoardCompositionPanel from './BoardCompositionPanel.vue'
import AuxiliaryLabelPool from './AuxiliaryLabelPool.vue'
import CompositeLabelPool from './CompositeLabelPool.vue'
import UpgradeSuggestionPanel from './UpgradeSuggestionPanel.vue'
import BackfillProgress from './BackfillProgress.vue'
import MatchingConfigDialog from './MatchingConfigDialog.vue'
import DailyReportGenerateDialog from './DailyReportGenerateDialog.vue'
import BoardDailyReportTimeline from './BoardDailyReportTimeline.vue'
import BoardThreadBrowser from './BoardThreadBrowser.vue'
import TopicDetectiveWall from './TopicDetectiveWall.client.vue'
import TagMergePreview from './TagMergePreview.vue'
import BoardListSidebar from './BoardListSidebar.vue'
import BoardTimelinePanel from './BoardTimelinePanel.vue'
import BoardEnrichmentPanel from './BoardEnrichmentPanel.vue'
import BoardEditDialog from './BoardEditDialog.vue'
import ArticlePreviewModal from './ArticlePreviewModal.vue'
import WatchManagePanel from './topic-watch/WatchManagePanel.vue'
import { WATCH_PANEL_KEY } from './topic-watch/watchPanelInject'
import { useTopicWatchesApi } from '~/api/topicWatches'
import { useTagsPage } from '~/features/tags/composables/useTagsPage'

// 标签池视图切换（未选板块时）：辅助标签 / 组合标签
const poolTab = ref<'aux' | 'composite'>('aux')

const {
  // Board CRUD
  boards, selectedBoardId, boardsLoading, boardsError,
  compositionLabels, compositionComposites, compositionLoading,
  editingBoard, editLabel, editDescription,
  editEnrichmentEnabled, editRelationAutoDiscoveryEnabled, editWindowDays, editContextLayers,
  editSaving, editError,
  showAddDialog,
  // Timeline
  timelineArticles, timelineLoading, timelineHasMore,
  activeFilterLabelId, filterFeedId, startDate, endDate,
  showDirectionMismatch, timelineSort, quickRange,
  timelineDisplayArticles, feedOptions, selectedTagForDetail,
  // Aux labels
  auxiliaryLabels, auxClusters, auxUnclusteredCount,
  auxLoading, auxSearchQuery, auxStatusFilter, auxPagination,
  // Other state
  contentTab, showUpgradeDialog, showMatchingConfigDialog,
  showGenerateDialog, showMergePreview, showArticlePreview,
  selectedPreviewArticle, previewArticles, loadingPreviewArticle,
  backfillTask,
  upgradeBackfillNotice,
  upgradePersistedSuggestions, upgradePersistedLoading, upgradePersistedGenerating,
  matchingConfig, matchingConfigLoading,
  // Methods - board
  loadBoards, loadComposition, handleSelectBoard,
  openEditBoard, closeEditBoard, handleSaveBoardEdit,
  handleAddBoard, handleDeleteBoard, handleRemoveComposition,
  // Methods - timeline
  handleLoadMore, handleFilterLabel, handleFilterChange,
  handleSortChange, handleDateInputChange, applyQuickRange,
  toggleMatchDetail,
  // Methods - aux
  loadAuxiliaryLabels, loadClusters, handleUpdatePage,
  handleDisableAuxLabel, handleMergeAuxLabel,
  // Methods - other
  handleUpgradeSuggest, loadPersistedSuggestions, handleGenerateUpgradeSuggestions,
  handleDismissUpgradeRow, handleConfirmUpgradeRow,
  handleTriggerBackfill,
  handleOpenMatchingConfig, handleSaveMatchingConfig,
  handleMergeComplete,
  openArticlePreview, closeArticlePreview,
  handleArticleFavorite, handleArticleUpdate,
} = useTagsPage()

const { isTagsFirstRun, startTagsTour } = useOnboarding()
const selectedBoardLabel = computed(() => boards.value.find(board => board.id === selectedBoardId.value)?.label)

// 话题总览 tab：侦探墙全屏入口（BoardThreadBrowser @open-detective-wall 触发）
const showTopicOverviewWall = ref(false)
const topicOverviewWallTopicId = ref<number | undefined>(undefined)
function openTopicOverviewDetectiveWall(topicId?: number) {
  topicOverviewWallTopicId.value = topicId
  showTopicOverviewWall.value = true
}

// 话题态势版图卡片/气泡 click → 切「话题总览」tab 并聚焦该 topic（BoardThreadBrowser focus 视图，不弹侦探墙）
const focusTopicIdInBrowser = ref<number | null>(null)
function handleLandscapeSelectTopic(topicId: number) {
  contentTab.value = 'topic-overview'
  // 先清再设，保证同一话题重复点击也触发 BoardThreadBrowser 的 watch
  focusTopicIdInBrowser.value = null
  nextTick(() => {
    focusTopicIdInBrowser.value = topicId
  })
}

onMounted(() => {
  if (isTagsFirstRun.value) {
    void startTagsTour()
  }
})

// —— 版块级关注管理（watch-keyword-and-quickadd：入口 chip 常驻 tab 栏右端）——
const watchesApi = useTopicWatchesApi()
const watchPanelOpen = ref(false)
const watchCount = ref(0)
function openWatchPanel() {
  watchPanelOpen.value = true
}
provide(WATCH_PANEL_KEY, openWatchPanel)
async function loadWatchCount() {
  if (selectedBoardId.value === null) return
  const res = await watchesApi.listWatches(selectedBoardId.value)
  watchCount.value = res.success && res.data ? res.data.length : 0
}
watch(selectedBoardId, () => { void loadWatchCount() })

/** 跨版块关系跳转（5.3）：切换选中版块；目标不在列表（如已停用）时如实提示。 */
function handleOpenBoard(boardId: number) {
  if (boardId === selectedBoardId.value) return
  selectedBoardId.value = boardId
}
onMounted(() => { void loadWatchCount() })
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
          <h1 class="tags-page-title">叙事工坊</h1>
        </div>
        <div class="flex items-center gap-2">
          <button type="button" class="tags-guide-btn" title="语义板块引导" @click="startTagsTour">
            <Icon icon="mdi:compass-outline" width="18" />
          </button>
          <ThemeToggle />
        </div>
      </div>
    </div>

    <div class="tags-main">
      <BoardListSidebar
        :boards="boards"
        :selected-board-id="selectedBoardId"
        :boards-loading="boardsLoading"
        :boards-error="boardsError"
        @select="handleSelectBoard"
        @delete="handleDeleteBoard"
        @edit="openEditBoard"
        @add-board="showAddDialog = true"
        @upgrade-suggest="handleUpgradeSuggest"
        @trigger-backfill="handleTriggerBackfill"
        @open-merge-preview="showMergePreview = true"
        @open-matching-config="handleOpenMatchingConfig"
        @open-generate="showGenerateDialog = true"
      />

      <main class="tags-content">
        <div v-if="selectedBoardId !== null">
          <div class="tags-content-tabs" data-onboarding="tags-content-tabs">
            <button type="button" class="tags-content-tab" :class="{ 'tags-content-tab--active': contentTab === 'composition' }" @click="contentTab = 'composition'">
              <Icon icon="mdi:view-dashboard-outline" width="14" /> 板块内容
            </button>
            <button type="button" class="tags-content-tab" :class="{ 'tags-content-tab--active': contentTab === 'topic-overview' }" @click="contentTab = 'topic-overview'">
              <Icon icon="mdi:chart-timeline-variant" width="14" /> 话题总览
            </button>
            <button type="button" class="tags-content-tab" :class="{ 'tags-content-tab--active': contentTab === 'daily-reports' }" @click="contentTab = 'daily-reports'">
              <Icon icon="mdi:file-document-outline" width="14" /> 日报
            </button>
            <button type="button" class="tags-content-tab" :class="{ 'tags-content-tab--active': contentTab === 'articles' }" @click="contentTab = 'articles'">
              <Icon icon="mdi:newspaper-variant-outline" width="14" /> 文章
            </button>
            <button type="button" class="tags-content-tab" :class="{ 'tags-content-tab--active': contentTab === 'enrichment' }" @click="contentTab = 'enrichment'">
              <Icon icon="mdi:database-plus-outline" width="14" /> 数据增强
            </button>
            <!-- 关注管理入口：tab 栏右端常驻（watch 是版块级实体，与 tab 同层） -->
            <button
              type="button"
              class="tags-watch-chip tags-watch-chip--nowrap"
              data-testid="watch-panel-chip"
              title="打开关注管理面板"
              @click="openWatchPanel"
            >
              <Icon icon="mdi:eye-outline" width="12" aria-hidden="true" />
              我在追踪 <span class="tags-watch-chip__n">({{ watchCount }})</span>
            </button>
          </div>

          <BoardCompositionPanel
            v-if="contentTab === 'composition'"
            :board-id="selectedBoardId"
            :labels="compositionLabels"
            :composites="compositionComposites"
            :board-label="selectedBoardLabel"
            :loading="compositionLoading"
            @remove="handleRemoveComposition"
            @refresh="() => loadComposition(selectedBoardId!)"
            @select-topic="handleLandscapeSelectTopic"
          />

          <BoardEnrichmentPanel
            v-if="contentTab === 'enrichment'"
            :board-id="selectedBoardId"
            :enrichment-enabled="boards.find((b) => b.id === selectedBoardId)?.enrichment_enabled ?? false"
            @enrichment-toggled="loadBoards()"
            @open-board="handleOpenBoard"
          />

          <BoardDailyReportTimeline
            v-if="contentTab === 'daily-reports'"
            :board-id="selectedBoardId"
            :board-title="selectedBoardLabel"
            :boards="boards"
            @open-article="openArticlePreview"
            @select-board="handleSelectBoard"
          />

          <BoardThreadBrowser
            v-if="contentTab === 'topic-overview'"
            :board-id="selectedBoardId"
            :focus-topic-id="focusTopicIdInBrowser"
            @open-article="openArticlePreview"
            @open-detective-wall="openTopicOverviewDetectiveWall()"
          />

          <TopicDetectiveWall
            v-if="showTopicOverviewWall"
            :board-id="selectedBoardId"
            :initial-topic-id="topicOverviewWallTopicId"
            @close="showTopicOverviewWall = false"
            @open-article="openArticlePreview"
          />

          <BoardTimelinePanel
            v-if="contentTab === 'articles'"
            :timeline-articles="timelineArticles"
            :timeline-loading="timelineLoading"
            :timeline-has-more="timelineHasMore"
            :active-filter-label-id="activeFilterLabelId"
            :composition-labels="compositionLabels"
            :selected-board-id="selectedBoardId"
            :timeline-display-articles="timelineDisplayArticles"
            :feed-options="feedOptions"
            :selected-tag-for-detail="selectedTagForDetail"
            @update:selected-tag-for-detail="selectedTagForDetail = $event"
            :filter-feed-id="filterFeedId"
            :start-date="startDate"
            :end-date="endDate"
            :show-direction-mismatch="showDirectionMismatch"
            :timeline-sort="timelineSort"
            :quick-range="quickRange"
            @load-more="handleLoadMore(selectedBoardId)"
            @filter-label="(id: number | null) => handleFilterLabel(id, selectedBoardId)"
            @filter-change="handleFilterChange(selectedBoardId)"
            @sort-change="(mode: 'quality' | 'time') => handleSortChange(mode, selectedBoardId)"
            @date-input-change="handleDateInputChange(selectedBoardId)"
            @apply-quick-range="(range: 'today' | '3d' | '7d' | '30d') => applyQuickRange(range, selectedBoardId)"
            @open-article-preview="(id: number) => openArticlePreview(id)"
            @toggle-match-detail="(tag) => toggleMatchDetail(tag)"
            @update:filter-feed-id="(id: number | null) => { filterFeedId = id; handleFilterChange(selectedBoardId) }"
            @update:start-date="(v: string) => startDate = v"
            @update:end-date="(v: string) => endDate = v"
            @update:show-direction-mismatch="(v: boolean) => showDirectionMismatch = v"
          />
        </div>

        <div v-else>
          <div class="tags-pool-tabs">
            <button type="button" class="tags-pool-tab" :class="{ 'is-active': poolTab === 'aux' }" @click="poolTab = 'aux'">辅助标签</button>
            <button type="button" class="tags-pool-tab" :class="{ 'is-active': poolTab === 'composite' }" data-testid="pool-tab-composite" @click="poolTab = 'composite'">组合标签</button>
          </div>
          <AuxiliaryLabelPool
            v-if="poolTab === 'aux'"
            :labels="auxiliaryLabels"
            :clusters="auxClusters"
            :unclustered-count="auxUnclusteredCount"
            :loading="auxLoading"
            :search-query="auxSearchQuery"
            :status-filter="auxStatusFilter"
            :pagination="auxPagination"
            @update:search-query="(v: string) => auxSearchQuery = v"
            @update:status-filter="(v: string) => auxStatusFilter = v"
            @update:page="handleUpdatePage"
            @disable="handleDisableAuxLabel"
            @merge="handleMergeAuxLabel"
            @refresh="loadAuxiliaryLabels"
            @select-cluster="() => {}"
          />
          <CompositeLabelPool v-if="poolTab === 'composite'" @changed="loadAuxiliaryLabels()" />
        </div>
      </main>
    </div>

    <div class="tags-bottombar">
      <BackfillProgress :task="backfillTask" />
    </div>

    <AddSemanticBoardDialog :visible="showAddDialog" @confirm="handleAddBoard" @cancel="showAddDialog = false" />
    <WatchManagePanel
      v-if="selectedBoardId !== null"
      v-model="watchPanelOpen"
      :board-id="selectedBoardId"
      @changed="loadWatchCount"
    />
    <BoardEditDialog
      :editing-board="!!editingBoard"
      :edit-label="editLabel"
      :edit-description="editDescription"
      :edit-enrichment-enabled="editEnrichmentEnabled"
      :edit-relation-auto-discovery-enabled="editRelationAutoDiscoveryEnabled"
      :edit-window-days="editWindowDays"
      :edit-context-layers="editContextLayers"
      :edit-saving="editSaving"
      :edit-error="editError"
      @update:edit-label="(v: string) => editLabel = v"
      @update:edit-description="(v: string) => editDescription = v"
      @update:edit-enrichment-enabled="(v: boolean) => editEnrichmentEnabled = v"
      @update:edit-relation-auto-discovery-enabled="(v: boolean) => editRelationAutoDiscoveryEnabled = v"
      @update:edit-window-days="(v: number) => editWindowDays = v"
      @update:edit-context-layers="(v: string[]) => editContextLayers = v"
      @save="handleSaveBoardEdit"
      @close="closeEditBoard"
    />
    <UpgradeSuggestionPanel
      :visible="showUpgradeDialog"
      :backfill-notice="upgradeBackfillNotice"
      :persisted-suggestions="upgradePersistedSuggestions"
      :persisted-loading="upgradePersistedLoading"
      :persisted-generating="upgradePersistedGenerating"
      :boards="boards"
      @load-persisted="loadPersistedSuggestions"
      @generate="handleGenerateUpgradeSuggestions"
      @dismiss-row="handleDismissUpgradeRow"
      @confirm-row="handleConfirmUpgradeRow"
      @cancel="showUpgradeDialog = false"
    />
    <MatchingConfigDialog
      :visible="showMatchingConfigDialog"
      :config="matchingConfig"
      :loading="matchingConfigLoading"
      @save="handleSaveMatchingConfig"
      @cancel="showMatchingConfigDialog = false"
    />
    <DailyReportGenerateDialog :visible="showGenerateDialog" :boards="boards" @cancel="showGenerateDialog = false" />
    <TagMergePreview :visible="showMergePreview" @close="showMergePreview = false" @merged="handleMergeComplete" />
    <ArticlePreviewModal
      :visible="showArticlePreview"
      :selected-preview-article="selectedPreviewArticle"
      :preview-articles="previewArticles"
      :loading-preview-article="loadingPreviewArticle"
      @close="closeArticlePreview"
      @favorite="handleArticleFavorite"
      @article-update="handleArticleUpdate"
      @navigate="(a) => selectedPreviewArticle = a"
    />
  </div>
</template>

<style scoped>
.tags-page { display: flex; flex-direction: column; height: 100vh; background: var(--color-bg-base); color: var(--color-text-primary); }
.tags-topbar { position: sticky; top: 0; z-index: 30; border-bottom: 1px solid var(--color-border-subtle); background: var(--color-bg-elevated); backdrop-filter: blur(16px); }
.tags-topbar-inner { display: flex; align-items: center; justify-content: space-between; padding: 0.75rem 1.5rem; }
.tags-back-btn { display: flex; align-items: center; justify-content: center; width: 28px; height: 28px; border: 1px solid var(--color-border-medium); border-radius: 8px; color: var(--color-text-muted); text-decoration: none; transition: all 0.12s ease; }
.tags-back-btn:hover { border-color: var(--color-border-strong); color: var(--color-text-secondary); background: var(--color-bg-hover); }
.tags-page-title { font-family: serif; font-size: 1.1rem; font-weight: 600; color: var(--color-text-primary); letter-spacing: 0.02em; }
.tags-guide-btn { display: flex; align-items: center; justify-content: center; width: 28px; height: 28px; border: 1px solid var(--color-border-medium); border-radius: 8px; background: none; color: var(--color-text-muted); cursor: pointer; transition: all 0.12s ease; }
.tags-guide-btn:hover { border-color: var(--color-border-strong); color: var(--color-text-secondary); background: var(--color-bg-hover); }
.tags-main { display: flex; flex: 1; min-height: 0; width: 100%; }
.tags-content { flex: 1; min-width: 0; padding: 1.25rem 1.5rem 3.5rem; overflow-y: auto; }
.tags-content-tabs { display: flex; gap: 0.25rem; padding: 0 0 0.75rem; margin-bottom: 1rem; border-bottom: 1px solid var(--color-border-subtle); }
.tags-content-tab { display: flex; align-items: center; gap: 0.35rem; padding: 0.4rem 0.75rem; border: none; border-radius: 8px 8px 0 0; background: none; color: var(--color-text-muted); font-size: 0.75rem; cursor: pointer; transition: all 0.12s ease; position: relative; }
.tags-content-tab:hover { color: var(--color-text-secondary); background: var(--color-bg-hover); }
.tags-content-tab--active { color: var(--color-accent); background: var(--color-accent-subtle); }
.tags-content-tab--active::after { content: ''; position: absolute; bottom: -1px; left: 0; right: 0; height: 2px; background: var(--color-accent); border-radius: 1px; }
/* 关注入口 chip：tab 栏右端常驻（margin-left:auto 推到最右），随版块存在 */
.tags-watch-chip { display: inline-flex; align-items: center; gap: 0.4rem; flex: 0 0 auto; margin-left: auto; padding: 0.35rem 0.75rem; border: 1px solid var(--color-border-medium); border-radius: 999px; background: none; color: var(--color-text-secondary); font-size: 0.7rem; cursor: pointer; transition: all 0.12s ease; }
.tags-watch-chip--nowrap { white-space: nowrap; }
.tags-watch-chip:hover { border-color: var(--color-accent); color: var(--color-accent); }
.tags-watch-chip__n { font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-size: 0.66rem; color: var(--color-text-muted); background: var(--color-bg-hover); border-radius: 8px; padding: 0 0.15rem; }
.tags-pool-tabs { display: flex; gap: 0.25rem; padding: 0 0 0.75rem; margin-bottom: 0.75rem; border-bottom: 1px solid var(--color-border-subtle); }
.tags-pool-tab { display: flex; align-items: center; gap: 0.35rem; padding: 0.4rem 0.75rem; border: none; border-radius: 8px 8px 0 0; background: none; color: var(--color-text-muted); font-size: 0.75rem; cursor: pointer; transition: all 0.12s ease; position: relative; }
.tags-pool-tab:hover { color: var(--color-text-secondary); background: var(--color-bg-hover); }
.tags-pool-tab.is-active { color: var(--color-accent); background: var(--color-accent-subtle); }
.tags-pool-tab.is-active::after { content: ''; position: absolute; bottom: -1px; left: 0; right: 0; height: 2px; background: var(--color-accent); border-radius: 1px; }
.tags-bottombar { position: fixed; bottom: 0; left: 0; right: 0; z-index: 40; display: flex; align-items: center; justify-content: flex-end; gap: 0.75rem; padding: 0.45rem 1.25rem; border-top: 1px solid var(--color-border-subtle); background: var(--color-bg-elevated); backdrop-filter: blur(12px); }
</style>
