/**
 * 控制台弹层/浮层编排（8C G4b 从 `BoardConsoleShell` 按内聚行为抽出）。
 *
 * 只做「打开/关闭 + 保存后回调」：生成对话框、新建泳道编排、话题管理、追踪管理、
 * 站内原文阅读（复用既有 `ArticlePreviewModal` 与 `useTagsPage`，不加导语中间层）。
 * 不拥有矩阵读取/选中/返回上下文；切板块统一由 `resetOnBoardChange()` 复位。
 */
import { nextTick, ref, type ComputedRef } from 'vue'
import { useTagsPage } from '~/features/tags/composables/useTagsPage'
import type { ManualTopicResult } from '~/api/persistentTopics'
import { useConsoleWorkface } from './useConsoleWorkface'

export interface ConsoleShellOverlaysDeps {
  isNarrow: ComputedRef<boolean>
  /** 窄屏无横向空间时的新建泳道提示（沿用既有 useNotify）。 */
  notifyWarn: (message: string) => void
  /** 重取 A 档（新建泳道保存后）。 */
  load: () => Promise<void>
  selectLane: (topicId: string) => void
  loadWatches: () => void | Promise<void>
}

export function useConsoleShellOverlays(deps: ConsoleShellOverlaysDeps) {
  // 单级宽工作面（右轨依据/管理入口）与浮层共用一套开关/焦点归还。
  const { open: workfaceOpen, tab: workfaceTab, openWorkface, closeWorkface, setVisible: handleWorkfaceVisible } = useConsoleWorkface()

  const archiveInitialDate = ref<string | null>(null)
  const generateOpen = ref(false)
  const composeOpen = ref(false)
  const topicManageOpen = ref(false)
  const watchOpen = ref(false)
  /** 泳道修正工作面（修正模式）的目标泳道 id；null = 未打开。 */
  const correctionTopicId = ref<string | null>(null)
  /** 泳道归档弹窗的目标泳道 id；null = 未打开。 */
  const archiveTopicId = ref<number | null>(null)

  // ── 站内原文阅读（复用既有 ArticlePreviewModal，输入框焦点回落） ───────────
  const {
    showArticlePreview,
    selectedPreviewArticle,
    previewArticles,
    loadingPreviewArticle,
    openArticlePreview,
    closeArticlePreview,
    handleArticleFavorite,
    handleArticleUpdate,
  } = useTagsPage()
  let articleFocus: HTMLElement | null = null

  function handleOpenArticle(articleId: number | string): void {
    const id = Number(articleId)
    if (!Number.isSafeInteger(id) || id <= 0) return
    articleFocus = typeof document !== 'undefined' ? document.activeElement as HTMLElement | null : null
    void openArticlePreview(id)
  }

  function handleCloseArticle(): void {
    closeArticlePreview()
    const target = articleFocus
    articleFocus = null
    void nextTick(() => target?.focus?.({ preventScroll: true }))
  }

  // ── 生成 / 新建泳道 / 话题与追踪管理 ─────────────────────────────────────
  function handleNewLane(): void {
    if (deps.isNarrow.value) {
      deps.notifyWarn('新建泳道需要宽屏或横屏')
      return
    }
    composeOpen.value = true
  }

  async function handleComposeSaved(topic?: ManualTopicResult): Promise<void> {
    composeOpen.value = false
    await deps.load()
    if (topic?.id) deps.selectLane(String(topic.id))
  }

  function openTopicManage(): void {
    closeWorkface()
    topicManageOpen.value = true
  }

  /**
   * 进泳道修正工作面（矩阵行头「⋯」/ 泳道管理列表的「编辑」入口）。
   * 窄屏禁用画布：只提示宽屏/横屏，不画残缺两栏（契约：窄屏 SHALL NOT 画残缺工作面）。
   */
  function openLaneCorrection(topicId: string): void {
    if (deps.isNarrow.value) {
      deps.notifyWarn('泳道修正需要宽屏或横屏')
      return
    }
    correctionTopicId.value = topicId
  }

  function closeLaneCorrection(): void {
    correctionTopicId.value = null
  }

  /** 进泳道归档弹窗（sm 档 + 二次确认）。 */
  function openLaneArchive(topicId: string): void {
    const id = Number(topicId)
    if (!Number.isSafeInteger(id) || id <= 0) return
    archiveTopicId.value = id
  }

  function closeLaneArchive(): void {
    archiveTopicId.value = null
  }

  function openWatchManage(): void {
    closeWorkface()
    watchOpen.value = true
  }

  function handleManage(kind: 'settings' | 'composition' | 'topics'): void {
    if (kind === 'topics') topicManageOpen.value = true
    else if (kind === 'settings') openWorkface('settings')
    else openWorkface('quality')
  }

  function openContext(): void {
    openWorkface('context')
  }

  /** 切板块：所有浮层关闭、归档初始日清空（生成/编排不跨板块继承）。 */
  function resetOnBoardChange(): void {
    topicManageOpen.value = false
    watchOpen.value = false
    composeOpen.value = false
    generateOpen.value = false
    archiveInitialDate.value = null
    correctionTopicId.value = null
    archiveTopicId.value = null
  }

  return {
    workfaceOpen,
    workfaceTab,
    openWorkface,
    handleWorkfaceVisible,
    handleManage,
    openContext,
    closeWorkface,
    archiveInitialDate,
    generateOpen,
    composeOpen,
    topicManageOpen,
    watchOpen,
    correctionTopicId,
    archiveTopicId,
    openLaneCorrection,
    closeLaneCorrection,
    openLaneArchive,
    closeLaneArchive,
    showArticlePreview,
    selectedPreviewArticle,
    previewArticles,
    loadingPreviewArticle,
    handleNewLane,
    handleComposeSaved,
    openTopicManage,
    openWatchManage,
    handleOpenArticle,
    handleCloseArticle,
    handleArticleFavorite,
    handleArticleUpdate,
    resetOnBoardChange,
  }
}
