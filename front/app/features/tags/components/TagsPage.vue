<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { Icon } from '@iconify/vue'
import { useSemanticBoardsApi, type SemanticBoard, type AuxiliaryLabelItem, type UpgradeCandidate, type UpgradeCluster, type UpgradeSuggestion, type BackfillTask, type MatchingConfig } from '~/api/semanticBoards'
import { useAuxiliaryLabelsApi, type AuxiliaryLabel } from '~/api/auxiliaryLabels'
import { useArticlesApi } from '~/api/articles'
import type { Article } from '~/types'
import SemanticBoardList from './SemanticBoardList.vue'
import AddSemanticBoardDialog from './AddSemanticBoardDialog.vue'
import BoardCompositionPanel from './BoardCompositionPanel.vue'
import AuxiliaryLabelPool from './AuxiliaryLabelPool.vue'
import UpgradeSuggestionPanel from './UpgradeSuggestionPanel.vue'
import BackfillProgress from './BackfillProgress.vue'
import MatchingConfigDialog from './MatchingConfigDialog.vue'

const sbApi = useSemanticBoardsApi()
const auxApi = useAuxiliaryLabelsApi()
const articlesApi = useArticlesApi()

const boards = ref<SemanticBoard[]>([])
const selectedBoardId = ref<number | null>(null)
const boardsLoading = ref(false)
const boardsError = ref<string | null>(null)

const compositionLabels = ref<AuxiliaryLabelItem[]>([])
const compositionLoading = ref(false)

const auxiliaryLabels = ref<AuxiliaryLabel[]>([])
const auxLoading = ref(false)
const auxSearchQuery = ref('')
const auxStatusFilter = ref('')

const upgradeCandidates = ref<UpgradeCandidate[]>([])
const upgradeClusters = ref<UpgradeCluster[]>([])
const upgradeSuggestions = ref<UpgradeSuggestion[]>([])
const upgradeLoading = ref(false)
const upgradeSuggesting = ref(false)

const backfillTask = ref<BackfillTask | null>(null)
let backfillPollTimer: ReturnType<typeof setInterval> | null = null

const matchingConfig = ref<MatchingConfig | null>(null)
const matchingConfigLoading = ref(false)

const showAddDialog = ref(false)
const showUpgradeDialog = ref(false)
const showMatchingConfigDialog = ref(false)

const timelineArticles = ref<Article[]>([])
const timelineLoading = ref(false)
const timelineVisible = computed(() => selectedBoardId.value !== null)

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
  const res = await auxApi.getLabels({ search: auxSearchQuery.value || undefined, status: auxStatusFilter.value || undefined })
  if (res.success && res.data) {
    auxiliaryLabels.value = res.data.items
  } else {
    auxiliaryLabels.value = []
  }
  auxLoading.value = false
}

function handleSelectBoard(id: number | null) {
  selectedBoardId.value = id
  if (id !== null) {
    void loadComposition(id)
    void loadTimelineArticles(id)
  } else {
    compositionLabels.value = []
    timelineArticles.value = []
  }
}

async function loadTimelineArticles(boardId: number) {
  timelineLoading.value = true
  try {
    const res = await articlesApi.getArticles({ concept_id: boardId, per_page: 50, sort_by: 'date' })
    if (res.success && res.data) {
      timelineArticles.value = (res.data.items || []) as Article[]
    } else {
      timelineArticles.value = []
    }
  } catch {
    timelineArticles.value = []
  } finally {
    timelineLoading.value = false
  }
}

function handleAddBoard(data: { label: string; description: string; display_order: number; protected: boolean }) {
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
  const res = await sbApi.getUpgradeCandidates()
  if (res.success && res.data) {
    upgradeCandidates.value = res.data.candidates
    upgradeClusters.value = res.data.clusters
  }
  upgradeLoading.value = false
}

async function handleSuggestUpgrade() {
  upgradeSuggesting.value = true
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
    void loadBoards()
    showUpgradeDialog.value = false
  }
}

async function handleTriggerBackfill() {
  const res = await sbApi.triggerBackfill({ mode: 'full' })
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

watch(auxSearchQuery, () => { void loadAuxiliaryLabels() })
watch(auxStatusFilter, () => { void loadAuxiliaryLabels() })

onMounted(() => {
  void loadBoards()
  void loadAuxiliaryLabels()
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
          @delete="handleDeleteBoard"
        />
      </aside>

      <!-- Right panel -->
      <main class="tags-content">
        <div v-if="selectedBoardId !== null">
          <BoardCompositionPanel
            :board-id="selectedBoardId"
            :labels="compositionLabels"
            :loading="compositionLoading"
            @remove="handleRemoveComposition"
            @refresh="() => loadComposition(selectedBoardId!)"
          />

          <!-- Article timeline -->
          <div class="tags-timeline">
            <div class="tags-timeline-header">
              <Icon icon="mdi:timeline-clock-outline" width="15" class="text-[rgba(240,138,75,0.8)]" />
              <span class="tags-timeline-title">相关文章</span>
              <span v-if="timelineArticles.length" class="tags-timeline-count">{{ timelineArticles.length }} 篇</span>
            </div>

            <div v-if="timelineLoading" class="tags-timeline-loading">
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
              >
                <div class="tags-timeline-item-date">
                  {{ new Date(article.pubDate).toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' }) }}
                </div>
                <div class="tags-timeline-item-content">
                  <a
                    :href="article.link"
                    target="_blank"
                    class="tags-timeline-item-title"
                    rel="noopener noreferrer"
                  >{{ article.title }}</a>
                  <span class="tags-timeline-item-feed">{{ article.feedId }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div v-else>
          <AuxiliaryLabelPool
            :labels="auxiliaryLabels"
            :loading="auxLoading"
            :search-query="auxSearchQuery"
            :status-filter="auxStatusFilter"
            @update:search-query="auxSearchQuery = $event"
            @update:status-filter="auxStatusFilter = $event"
            @disable="handleDisableAuxLabel"
            @merge="handleMergeAuxLabel"
            @refresh="loadAuxiliaryLabels"
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
  gap: 0.75rem;
  padding: 0.5rem 0.65rem;
  border-radius: 8px;
  transition: background 0.12s ease;
}

.tags-timeline-item:hover {
  background: rgba(255, 255, 255, 0.03);
}

.tags-timeline-item-date {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
  white-space: nowrap;
  padding-top: 0.15rem;
  min-width: 2.5rem;
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
  text-decoration: none;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition: color 0.12s ease;
}

.tags-timeline-item-title:hover {
  color: rgba(255, 220, 200, 0.9);
}

.tags-timeline-item-feed {
  font-size: 0.62rem;
  color: rgba(255, 255, 255, 0.25);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
