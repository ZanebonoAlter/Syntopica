<script setup lang="ts">
/**
 * 雷达总览治理宿主（rework-workshop-into-radar-and-desk 8A/R-18 拆分）。
 *
 * 单点持有 useTagsPage 全局治理状态，承接报头「更多」菜单五项
 * （扩充版块/匹配回填/匹配规则/标签合并/标签池）与全部真实 dialogs、回填进度、
 * 标签池两页签；新建版块流程（确认→创建→成功回程）也在此完成，创建成功
 * emit board-created 由视图重拉总览并进入新版块（顺序与旧实现一致）。
 * 旧治理 dialog 的既有接口零改动，不扩到 semanticBoards 其它 API。
 */
import { ref } from 'vue'
import AddSemanticBoardDialog from '~/features/tags/components/AddSemanticBoardDialog.vue'
import UpgradeSuggestionPanel from '~/features/tags/components/UpgradeSuggestionPanel.vue'
import MatchingConfigDialog from '~/features/tags/components/MatchingConfigDialog.vue'
import TagMergePreview from '~/features/tags/components/TagMergePreview.vue'
import BackfillProgress from '~/features/tags/components/BackfillProgress.vue'
import AuxiliaryLabelPool from '~/features/tags/components/AuxiliaryLabelPool.vue'
import CompositeLabelPool from '~/features/tags/components/CompositeLabelPool.vue'
import { useTagsPage } from '~/features/tags/composables/useTagsPage'
import { useSemanticBoardsApi } from '~/api/semanticBoards'
import { useNotify } from '~/composables/useNotify'

export type RadarAdvancedKind = 'upgrade' | 'backfill' | 'matching' | 'merge' | 'labels'

const props = defineProps<{
  addBoardVisible: boolean
}>()

const emit = defineEmits<{
  (e: 'update:addBoardVisible', value: boolean): void
  (e: 'board-created', boardId: number): void
}>()

const boardsApi = useSemanticBoardsApi()
const { error: notifyError, success: notifySuccess } = useNotify()

const creatingBoard = ref(false)
const labelsOpen = ref(false)
const poolTab = ref<'aux' | 'composite'>('aux')

const {
  boards: govBoards,
  auxiliaryLabels,
  auxClusters,
  auxUnclusteredCount,
  auxLoading,
  auxSearchQuery,
  auxStatusFilter,
  auxPagination,
  upgradeBackfillNotice,
  upgradePersistedSuggestions,
  upgradePersistedLoading,
  upgradePersistedGenerating,
  showUpgradeDialog,
  showMatchingConfigDialog,
  showMergePreview,
  matchingConfig,
  matchingConfigLoading,
  matchingConfigSaving,
  matchingConfigError,
  backfillTask,
  handleUpgradeSuggest,
  loadPersistedSuggestions,
  handleGenerateUpgradeSuggestions,
  handleDismissUpgradeRow,
  handleConfirmUpgradeRow,
  handleTriggerBackfill,
  handleOpenMatchingConfig,
  handleSaveMatchingConfig,
  handleMergeComplete,
  loadAuxiliaryLabels,
  loadClusters,
  handleUpdatePage,
  handleDisableAuxLabel,
  handleMergeAuxLabel,
} = useTagsPage()

function openAdvanced(kind: RadarAdvancedKind): void {
  if (kind === 'upgrade') handleUpgradeSuggest()
  else if (kind === 'backfill') {
    if (!window.confirm('匹配回填会重算全库所有版块的匹配结果，可能耗时较长。确认继续？')) return
    void handleTriggerBackfill()
  }
  else if (kind === 'matching') void handleOpenMatchingConfig()
  else if (kind === 'merge') showMergePreview.value = true
  else {
    labelsOpen.value = true
    void loadAuxiliaryLabels()
    void loadClusters()
  }
}

defineExpose({ openAdvanced, creatingBoard })

async function handleAddBoard(data: {
  label: string
  description: string
  display_order: number
  protected: boolean
  auxiliary_labels: number[]
}): Promise<void> {
  if (creatingBoard.value) return
  creatingBoard.value = true
  try {
    const res = await boardsApi.createBoard(data)
    if (res.success && res.data) {
      emit('update:addBoardVisible', false)
      notifySuccess(`版块「${data.label}」已创建`)
      emit('board-created', res.data.id)
    } else {
      notifyError(res.error || '创建版块失败')
    }
  } finally {
    creatingBoard.value = false
  }
}
</script>

<template>
  <UpgradeSuggestionPanel
    :visible="showUpgradeDialog"
    :backfill-notice="upgradeBackfillNotice"
    :persisted-suggestions="upgradePersistedSuggestions"
    :persisted-loading="upgradePersistedLoading"
    :persisted-generating="upgradePersistedGenerating"
    :boards="govBoards"
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
    :saving="matchingConfigSaving"
    :save-error="matchingConfigError"
    @save="handleSaveMatchingConfig"
    @cancel="showMatchingConfigDialog = false"
  />
  <TagMergePreview :visible="showMergePreview" @close="showMergePreview = false" @merged="handleMergeComplete" />
  <div v-if="backfillTask" class="rd-backfill" data-testid="radar-backfill-progress">
    <BackfillProgress :task="backfillTask" />
  </div>
  <AppDialog v-model="labelsOpen" title="标签池" width="min(92vw, 72rem)">
    <div class="rd-pool-tabs" role="tablist" aria-label="标签池类型">
      <button type="button" :class="{ 'is-on': poolTab === 'aux' }" @click="poolTab = 'aux'">辅助标签</button>
      <button type="button" :class="{ 'is-on': poolTab === 'composite' }" @click="poolTab = 'composite'">组合标签</button>
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
    <CompositeLabelPool v-else @changed="loadAuxiliaryLabels" />
  </AppDialog>
  <AddSemanticBoardDialog
    :visible="addBoardVisible"
    @confirm="handleAddBoard"
    @cancel="emit('update:addBoardVisible', false)"
  />
</template>

<style scoped>
.rd-backfill {
  position: fixed;
  right: 1rem;
  bottom: 1rem;
  z-index: 60;
  min-width: 18rem;
  padding: 0.5rem 0.7rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  background: var(--color-dialog-bg);
  box-shadow: var(--shadow-strong);
}

.rd-pool-tabs {
  display: flex;
  gap: 0.35rem;
  margin-bottom: 0.75rem;
}

.rd-pool-tabs button {
  padding: 0.28rem 0.65rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 999px;
  background: transparent;
  color: var(--color-text-muted);
  font-size: 0.72rem;
  cursor: pointer;
}

.rd-pool-tabs button.is-on {
  border-color: var(--color-accent);
  background: var(--color-accent-subtle);
  color: var(--color-accent);
}
</style>
