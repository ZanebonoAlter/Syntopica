import { ref, onMounted, onUnmounted } from 'vue'
import { useSemanticBoardsApi, type UpgradeGenerateParams, type UpgradeSuggestionRow, type BackfillTask, type MatchingConfig } from '~/api/semanticBoards'
import { useArticlesApi } from '~/api/articles'
import { normalizeArticle, type ArticlePayload } from '~/api/normalizers/article'
import { useArticleFavoriteToggle } from '~/features/articles/composables/useArticleFavoriteToggle'
import type { Article } from '~/types'
import { useBoardCRUD } from './useBoardCRUD'
import { useAuxiliaryLabels } from './useAuxiliaryLabels'

/**
 * 雷达报头「更多」菜单与站内原文预览的共享状态源（rework-workshop-into-radar-and-desk 8.4：
 * 旧五 tab 宿主 TagsPage 已退役删除，本 composable 只保留治理动作 + 文章预览；
 * timeline/五 tab 状态随宿主一并清理）。
 */
export function useTagsPage() {
  const boardCRUD = useBoardCRUD()
  const auxLabels = useAuxiliaryLabels()

  const sbApi = useSemanticBoardsApi()
  const articlesApi = useArticlesApi()
  const { toggleFavorite } = useArticleFavoriteToggle()

  // Dialog state
  const showUpgradeDialog = ref(false)
  const showMatchingConfigDialog = ref(false)
  const showMergePreview = ref(false)
  const showArticlePreview = ref(false)

  // Article preview
  const selectedPreviewArticle = ref<Article | null>(null)
  const previewArticles = ref<Article[]>([])
  const loadingPreviewArticle = ref(false)

  // Upgrade
  const upgradeBackfillNotice = ref(false)

  // Persisted upgrade suggestions (主数据源，§6.1/6.2)
  const upgradePersistedSuggestions = ref<UpgradeSuggestionRow[]>([])
  const upgradePersistedLoading = ref(false)
  const upgradePersistedGenerating = ref(false)
  const upgradePersistedFilter = ref('')

  // Backfill
  const backfillTask = ref<BackfillTask | null>(null)
  let backfillPollTimer: ReturnType<typeof setInterval> | null = null

  // Matching config
  const matchingConfig = ref<MatchingConfig | null>(null)
  const matchingConfigLoading = ref(false)

  // ---- Article preview ----
  async function openArticlePreview(articleId: number) {
    loadingPreviewArticle.value = true
    showArticlePreview.value = true
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
    showArticlePreview.value = false
    selectedPreviewArticle.value = null
  }

  /** 收藏：乐观翻转（星标即时反馈）+ 失败回滚（此前等接口返回才翻转，点击显得没反应） */
  async function handleArticleFavorite(articleId: string) {
    await toggleFavorite(articleId, {
      get: () => selectedPreviewArticle.value,
      apply: (article: Article) => { selectedPreviewArticle.value = article },
    })
  }

  function handleArticleUpdate(articleId: string, updates: Partial<Article>) {
    if (selectedPreviewArticle.value?.id === articleId) {
      Object.assign(selectedPreviewArticle.value, updates)
    }
  }

  // ---- Upgrade ----
  /** 打开升级建议面板（旧内存探索链路已退役：候选/簇/内存建议不再预加载）。 */
  function handleUpgradeSuggest() {
    showUpgradeDialog.value = true
  }

  // ---- Persisted suggestions (主数据源，§6.1/6.2) ----
  async function loadPersistedSuggestions(decision: string = '') {
    upgradePersistedLoading.value = true
    upgradePersistedFilter.value = decision
    const res = await sbApi.getUpgradeSuggestions(decision ? { decision } : undefined)
    if (res.success && res.data) {
      upgradePersistedSuggestions.value = res.data.suggestions
    }
    upgradePersistedLoading.value = false
  }

  /** 四格生成入口（方向×来源+锁定版块）：生成失败时回传错误供面板行内提示。 */
  async function handleGenerateUpgradeSuggestions(params: UpgradeGenerateParams): Promise<{ ok: boolean; error?: string }> {
    upgradePersistedGenerating.value = true
    const res = await sbApi.generateUpgradeSuggestions(params)
    upgradePersistedGenerating.value = false
    if (res.success) {
      await loadPersistedSuggestions(upgradePersistedFilter.value)
      return { ok: true }
    }
    return { ok: false, error: res.error }
  }

  async function handleDismissUpgradeRow(id: number) {
    const res = await sbApi.dismissUpgradeSuggestion(id)
    if (res.success) {
      upgradePersistedSuggestions.value = upgradePersistedSuggestions.value.filter((r) => r.id !== id)
    }
  }

  // 持久化建议确认执行：携带 row.id 作为 suggestion_id，后端事务内置 confirmed 联动 (§6.4)
  async function handleConfirmUpgradeRow(row: UpgradeSuggestionRow) {
    if (row.decision !== 'create_new' && row.decision !== 'merge_into_existing' && row.decision !== 'compose') return
    const res = await sbApi.executeUpgrade({
      decision: row.decision,
      board_label: row.board_label,
      description: row.description,
      target_board_id: row.target_board_id,
      auxiliary_label_ids: row.auxiliary_label_ids,
      suggestion_id: row.id,
    })
    if (res.success) {
      upgradePersistedSuggestions.value = upgradePersistedSuggestions.value.filter((r) => r.id !== row.id)
      upgradeBackfillNotice.value = true
      void boardCRUD.loadBoards()
    }
  }

  // ---- Backfill ----
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

  // ---- Matching config ----
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
    if (res.success) showMatchingConfigDialog.value = false
  }

  // ---- Aux label actions ----
  function handleMergeComplete() {
    void auxLabels.loadAuxiliaryLabels()
    void boardCRUD.loadBoards()
  }

  // ---- Lifecycle ----
  onMounted(() => {
    void boardCRUD.loadBoards()
    void auxLabels.loadAuxiliaryLabels()
    void auxLabels.loadClusters()
  })

  onUnmounted(() => {
    if (backfillPollTimer) clearInterval(backfillPollTimer)
  })

  return {
    // Board CRUD（只读与列表：治理弹窗刷新用；编辑/增删操作由新壳各自的表单承担）
    boards: boardCRUD.boards,
    selectedBoardId: boardCRUD.selectedBoardId,
    boardsLoading: boardCRUD.boardsLoading,
    boardsError: boardCRUD.boardsError,
    loadBoards: boardCRUD.loadBoards,

    // Auxiliary labels
    auxiliaryLabels: auxLabels.auxiliaryLabels,
    auxClusters: auxLabels.auxClusters,
    auxUnclusteredCount: auxLabels.auxUnclusteredCount,
    auxLoading: auxLabels.auxLoading,
    auxSearchQuery: auxLabels.auxSearchQuery,
    auxStatusFilter: auxLabels.auxStatusFilter,
    auxPagination: auxLabels.auxPagination,
    loadAuxiliaryLabels: auxLabels.loadAuxiliaryLabels,
    loadClusters: auxLabels.loadClusters,
    handleUpdatePage: auxLabels.handleUpdatePage,
    handleDisableAuxLabel: auxLabels.handleDisableAuxLabel,
    handleMergeAuxLabel: auxLabels.handleMergeAuxLabel,

    // Additional state
    showUpgradeDialog, showMatchingConfigDialog, showMergePreview, showArticlePreview,

    // Article preview
    selectedPreviewArticle, previewArticles, loadingPreviewArticle,
    openArticlePreview, closeArticlePreview,
    handleArticleFavorite, handleArticleUpdate,

    // Upgrade
    upgradeBackfillNotice,
    handleUpgradeSuggest,
    upgradePersistedSuggestions, upgradePersistedLoading, upgradePersistedGenerating,
    loadPersistedSuggestions, handleGenerateUpgradeSuggestions,
    handleDismissUpgradeRow, handleConfirmUpgradeRow,

    // Backfill
    backfillTask, handleTriggerBackfill,

    // Matching config
    matchingConfig, matchingConfigLoading,
    handleOpenMatchingConfig, handleSaveMatchingConfig,

    // Composite methods
    handleMergeComplete,
  }
}
