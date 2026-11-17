<script setup lang="ts">
/**
 * 版块态势控制台壳（rework-workshop-into-radar-and-desk 矩阵阶段 G5 收口；8C G4b 接线拆分）。
 *
 * 三区固定视口栅格（≥1440px，layout.md「已登记例外」）：
 *   版块栏（BoardRail） / 主舞台（BoardConsoleStage） / 上下文右轨（BoardContextRail）。
 * 顶栏（BoardConsoleHeader）：面包屑 + 视图 seg（矩阵/追踪｜归档｜研究工作台）+
 * 唯一最新日报主入口 + 板块管理 + 新建泳道。
 *
 * 状态唯一性：sourceDays / 观察范围 / 选中泳道落在 `useBoardConsole`（G1）；
 * 选中日与名单投影在 `useConsoleSelection`（经 `useConsoleReadingWiring` 注入同一份报日集合）；
 * 报日索引/当日详情/文章缓存与观察依赖是**壳层单份实例**（`useConsoleReading`），编年与
 * 其他话题阅读槽只注入消费，不自建第二份读取器。名单/排序/休眠只由 A 档全轴派生；
 * 365 池只补 A 同 ID 历史，不参与名单/候选。最新日报元信息来自独立 report 锚
 * （useConsoleShellData，0 active 泳道版块同样可见）。
 *
 * 本文件只做「取状态 → 装配 → 转发」：弹层编排在 `useConsoleShellOverlays`，阅读接线在
 * `useConsoleReadingWiring`，舞台视图模型装配在 `consoleStageBinding`（code-style §8）。
 */
import { computed, ref, toRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useNotify } from '~/composables/useNotify'
import { useIsNarrowViewport } from '~/composables/useMediaQuery'
import { useBoardConsole } from '~/features/tags/composables/useBoardConsole'
import type { BoardConsoleStageProps } from './consoleStageTypes'
import { useConsoleShellData } from './useConsoleShellData'
import { useConsoleReturnContext, type ReadingSettledState } from './useConsoleReturnContext'
import { useConsoleShellRail } from './useConsoleShellRail'
import { useConsoleRefresh } from './useConsoleRefresh'
import { useConsoleReadingWiring } from './useConsoleReadingWiring'
import { useConsoleShellOverlays } from './useConsoleShellOverlays'
import { buildConsoleStageProps } from './consoleStageBinding'
import { useConsoleShellActions } from './useConsoleShellActions'
import BoardConsoleBody from './BoardConsoleBody.vue'
import BoardWideWorkface from './BoardWideWorkface.vue'
import BoardComposeOverlay from './BoardComposeOverlay.vue'
import LaneArchiveDialog from '~/components/dialog/LaneArchiveDialog.vue'
import DailyReportGenerateDialog from '../DailyReportGenerateDialog.vue'
import TopicManageDialog from '~/components/dialog/TopicManageDialog.vue'
import WatchManagePanel from '../topic-watch/WatchManagePanel.vue'
import ArticlePreviewModal from '../ArticlePreviewModal.vue'

const props = defineProps<{
  boardId: number
}>()

const route = useRoute()
const router = useRouter()
const { warn: notifyWarn } = useNotify()
const isNarrow = useIsNarrowViewport()
const boardIdRef = toRef(props, 'boardId')

const {
  boards,
  boardsLoading,
  boardsError,
  currentBoard,
  boardLabel,
  boardMissing,
  latestReportDate,
  latestReportTitle,
  reportMetaUnavailable,
  reportMetaStale,
  reportMetaLoading,
  watches,
  watchesLoading,
  watchesError,
  loadBoards,
  loadReportMeta,
  loadWatches,
} = useConsoleShellData(boardIdRef)

// ── 控制台状态（G1 唯一状态源；reportAnchor 接独立日报元信息） ──────────────
const consoleState = useBoardConsole(boardIdRef, {
  route: () => route,
  router: () => router,
  reportAnchor: () => latestReportDate.value,
})

const {
  loading,
  error,
  staleNotice,
  today,
  axis,
  sourceWindow,
  poolBounds,
  poolLoading,
  poolError,
  poolStaleNotice,
  retryPool,
  sourceDays,
  anchorDate,
  visibleAxis,
  selectedLaneId,
  expandedDormant,
  activeView,
  emptyState,
  observation,
  rangeNotice,
  daysNotice,
  resolveLaneDay,
  load,
  selectLane,
  toggleDormant,
  setView,
  setSourceDays,
  applyObservationRange,
  syncDefaultRangeToAnchor,
} = consoleState

// ── 阅读接线（G4a 单份读取 + 选中日投影 + 当日其他话题 + 单一阅读槽） ────────
const wiring = useConsoleReadingWiring({
  boardId: boardIdRef,
  today: computed(() => today.value),
  anchorDate: computed(() => anchorDate.value),
  axis,
  visibleAxis,
  sourceWindow,
  selectedLaneId,
  resolveLaneDay,
  poolBounds,
  selectLane,
})

const { selection } = wiring
const { laneGroups, allLanes, selectedLaneRow, selectedLaneKey, selectedDate, poolSeries } = selection

const rangeNoticeText = computed(() => rangeNotice.value ?? daysNotice.value)

// ── 报告元信息晚到：不改用户已选范围，只补默认区间 ─────────────────────────
watch(anchorDate, () => {
  syncDefaultRangeToAnchor()
}, { immediate: true })

const bodyRef = ref<{ getStageElement: () => HTMLElement | null } | null>(null)
const readingSettled = ref<ReadingSettledState | null>(null)

// 阅读上下文一变就作废旧终态信号（旧 lane/other 信号不得兑现滚动；新组件挂载后自行再发）。
watch([selectedDate, selectedLaneKey, wiring.selectedOtherSectionId], () => {
  readingSettled.value = null
})

// ── 返回上下文（阅读台/研究工作台往返 + 内部归档同实例往返；URL 显式值优先） ──
const returnContext = useConsoleReturnContext({
  boardId: boardIdRef,
  sourceWindow,
  sourceDays,
  allLanes,
  selectedLaneKey,
  selectedDate,
  observation,
  stageRef: () => bodyRef.value?.getStageElement() ?? null,
  route: () => route,
  router: () => router,
  selectLane,
  setSourceDays,
  applyObservationRange,
  readingSettled,
  selectedOther: wiring.selectedOther,
  othersOpen: wiring.othersOpen,
})

// ── 刷新链（元信息 → A → 阅读附属：报日索引/当日报告/观察失效/lane 编年重建） ──
const { busy: consoleRefreshing, refresh: refreshConsole } = useConsoleRefresh({
  loadReportMeta,
  loadSource: load,
  reportMetaLoading,
  after: () => wiring.refreshAfterRebuild(),
})

// ── 弹层编排（生成/新建泳道/话题管理/追踪管理/站内原文阅读） ────────────────
const overlays = useConsoleShellOverlays({
  // 下游 deps 声明 ComputedRef：普通 Ref 包一层只读 computed 对齐（取值/时序不变，写法同下方 wiring.today）。
  isNarrow: computed(() => isNarrow.value),
  notifyWarn,
  load,
  selectLane,
  loadWatches,
})

const {
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
  workfaceOpen,
  workfaceTab,
  openWorkface,
  handleWorkfaceVisible,
  handleManage,
  openContext,
  closeWorkface,
} = overlays

/** 泳道显示名（修正工作面标题 / 归档弹窗文案用；名单里没有该泳道时留空）。 */
function laneLabelOf(topicId: string): string {
  return allLanes.value.find(row => row.topicId === topicId)?.label ?? ''
}

/** 修正工作面的泳道名。 */
const correctionLaneLabel = computed(() => (correctionTopicId.value ? laneLabelOf(correctionTopicId.value) : ''))

/** 归档弹窗的泳道名。 */
const archiveLaneLabel = computed(() => (archiveTopicId.value ? laneLabelOf(String(archiveTopicId.value)) : ''))

/** 就地改名后刷新名单（泳道名来自 A 档名单，不刷新会显示旧名）。 */
async function handleLaneRenamed(): Promise<void> {
  await load()
}

/** 归档后关闭弹窗并刷新名单。 */
async function handleLaneArchived(): Promise<void> {
  closeLaneArchive()
  await load()
}

// ── 共享描述修复：摘除反馈（board-health-triage-advice seam C3 / tasks 3.4）──
// 只有宿主**真实**摘除成功才回填；失败不谎称已摘除（工作面据此显示状态）。
const removedAuxIds = ref<string[]>([])

/**
 * 工作面发起的摘除 → 既有 remove 链（removeFromComposition → 本地重取 → 体检重取）。
 * 失败由既有链通知一次（不叠加 toast）；成功后回填 removedAuxIds。
 */
async function handleRemoveComposition(auxiliaryLabelId: string): Promise<void> {
  const ok = await removeComposition(Number(auxiliaryLabelId))
  if (!ok) return
  if (!removedAuxIds.value.includes(auxiliaryLabelId)) {
    removedAuxIds.value = [...removedAuxIds.value, auxiliaryLabelId]
  }
}

// ── 壳层动作（视图/归档往返/报告/研究/版块/重试/定位）在 useConsoleShellActions ──
const {
  handleSetView,
  handleViewArchive,
  openLatestReport,
  openResearch,
  selectBoard,
  handleRetryStage,
  handleRetryReportChain,
  handleRetryOtherReport,
  locateLaneFromRail,
} = useConsoleShellActions({
  boardId: props.boardId,
  // 同上：动作 deps 声明 ComputedRef，包装只读 computed 不改取值。
  activeView: computed(() => activeView.value),
  latestReportDate: computed(() => latestReportDate.value),
  archiveInitialDate,
  setView,
  router,
  returnContext,
  reloadSource: load,
  retryReportIndex: wiring.retryReportIndex,
  refreshAfterRebuild: wiring.refreshAfterRebuild,
  retryReport: wiring.retryReport,
  selectLaneFromStage: wiring.selectLaneFromStage,
  closeWorkface,
})

// ── 体检 / 构成 / 右轨上下文（同一份真实数据；装配在 useConsoleShellRail） ──
const {
  railContext,
  railBinding,
  removeComposition,
  captureHealth,
  healthSnapshot,
  healthCapturingId,
  loadHealth,
  compositionLabels,
  compositionComposites,
  compositionLoading,
  compositionError,
  loadComposition,
  archiveLabels,
} = useConsoleShellRail({
  boardId: boardIdRef,
  sourceWindow,
  allLanes,
  watches,
  watchesLoading,
  watchesError,
})

const stageBinding = computed<BoardConsoleStageProps>(() => buildConsoleStageProps({
  boardId: props.boardId,
  boardLabel: boardLabel.value,
  activeView: activeView.value,
  isNarrow: isNarrow.value,
  emptyState: emptyState.value,
  latestReportDate: latestReportDate.value,
  anchorDate: anchorDate.value,
  loading: loading.value,
  error: error.value,
  staleNotice: staleNotice.value,
  today: today.value,
  sourceDays: sourceDays.value,
  observation: observation.value,
  poolBounds: poolBounds.value,
  poolSeries: poolSeries.value,
  poolLoading: poolLoading.value,
  poolError: poolError.value,
  poolStaleNotice: poolStaleNotice.value,
  rangeNoticeText: rangeNoticeText.value,
  visibleAxis: visibleAxis.value,
  lanes: laneGroups.value,
  selectedLaneKey: selectedLaneKey.value,
  selectedDate: selectedDate.value,
  selectedLaneRow: selectedLaneRow.value,
  expandedDormant: expandedDormant.value,
  resolveLaneDay,
  compositionLabels: archiveLabels.value,
  archiveInitialDate: archiveInitialDate.value,
  reading: wiring.stageProps.value,
}))

async function handleBoardSaved(): Promise<void> {
  await loadBoards()
}

function handleBoardDeleted(): void {
  closeWorkface()
  void router.push('/tags')
}

// ── 版块切换清场：阅读终态/选中日/浮层一并复位（读取缓存由 store 自行换板块清空） ──
watch(boardIdRef, () => {
  readingSettled.value = null
  selectedDate.value = null
  removedAuxIds.value = []
  closeWorkface()
  overlays.resetOnBoardChange()
})

</script>

<template>
  <BoardConsoleBody
      ref="bodyRef"
      :board-missing="boardMissing"
      :boards="boards"
      :selected-board-id="boardId"
      :boards-loading="boardsLoading"
      :boards-error="boardsError"
      :board-label="boardLabel"
      :latest-report-date="latestReportDate"
      :latest-report-title="latestReportTitle"
      :report-meta-unavailable="reportMetaUnavailable"
      :report-meta-stale="reportMetaStale"
      :refreshing="consoleRefreshing"
      :active-view="activeView"
      :is-narrow="isNarrow"
      :stage="stageBinding"
      :rail="railBinding"
      @set-view="handleSetView"
      @open-latest-report="openLatestReport"
      @refresh="refreshConsole"
      @open-research="openResearch"
      @manage="handleManage"
      @new-lane="overlays.handleNewLane"
      @open-context="openContext"
      @select-board="selectBoard"
      @retry-boards="loadBoards"
      @select-lane="wiring.selectLaneFromStage"
      @edit-lane="overlays.openLaneCorrection"
      @archive-lane="overlays.openLaneArchive"
      @select-cell="wiring.selectCellFromStage"
      @select-date="wiring.selectDateFromStage"
      @select-day-header="wiring.selectDayHeader"
      @select-other="wiring.selectOther"
      @toggle-others="wiring.toggleOthers"
      @select-observed-day="wiring.selectObservedDay"
      @toggle-dormant="toggleDormant"
      @retry-stage="handleRetryStage"
      @retry-report="handleRetryOtherReport"
      @retry-report-chain="handleRetryReportChain"
      @generate="generateOpen = true"
      @open-report-date="payload => returnContext.openReport(payload.date)"
      @view-archive="handleViewArchive"
      @open-article="overlays.handleOpenArticle"
      @preview-range="range => applyObservationRange(range.start, range.end, { writeQuery: false })"
      @commit-range="range => applyObservationRange(range.start, range.end)"
      @source-change="days => setSourceDays(days)"
      @retry-pool="retryPool"
      @reading-settled="readingSettled = $event"
      @retry-health="loadHealth"
      @open-health="openWorkface('health')"
      @open-candidates="openWorkface('candidates')"
      @open-sources="openWorkface('sources')"
      @open-watch-manage="overlays.openWatchManage"
      @retry-watches="loadWatches"
      @locate-lane="locateLaneFromRail"
      @open-topics="overlays.openTopicManage"
    />

    <BoardWideWorkface
      :open="workfaceOpen"
      :board-id="boardId"
      :board-label="boardLabel"
      :board="currentBoard"
      :tab="workfaceTab"
      :rail="railContext"
      :labels="compositionLabels"
      :composites="compositionComposites"
      :composition-loading="compositionLoading"
      :composition-error="compositionError"
      :snapshot="healthSnapshot"
      :capturing-id="healthCapturingId"
      :removed-aux-ids="removedAuxIds"
      @update:open="handleWorkfaceVisible"
      @update:tab="workfaceTab = $event"
      @remove="removeComposition"
      @remove-composition="handleRemoveComposition"
      @capture="captureHealth"
      @refresh="loadComposition"
      @saved="handleBoardSaved"
      @deleted="handleBoardDeleted"
      @retry-health="loadHealth"
      @retry-watches="loadWatches"
      @locate-lane="locateLaneFromRail"
      @open-watch-manage="overlays.openWatchManage"
      @open-topics="overlays.openTopicManage"
    />

    <BoardComposeOverlay
      v-if="composeOpen"
      :board-id="boardId"
      :board-label="boardLabel"
      @saved="overlays.handleComposeSaved"
      @close="composeOpen = false"
    />

    <!-- 泳道修正工作面（修正模式：两栏全屏画布；窄屏由壳层禁用入口） -->
    <BoardComposeOverlay
      v-if="correctionTopicId"
      mode="correction"
      :board-id="boardId"
      :board-label="boardLabel"
      :topic-id="Number(correctionTopicId)"
      :lane-label="correctionLaneLabel"
      @renamed="handleLaneRenamed"
      @close="closeLaneCorrection"
    />

    <!-- 泳道归档（sm 档 + 二次确认） -->
    <LaneArchiveDialog
      v-if="archiveTopicId"
      :model-value="archiveTopicId !== null"
      :topic-id="archiveTopicId"
      :lane-label="archiveLaneLabel"
      @update:model-value="value => { if (!value) closeLaneArchive() }"
      @archived="handleLaneArchived"
    />

    <DailyReportGenerateDialog
      :visible="generateOpen"
      :boards="boards"
      @completed="refreshConsole"
      @cancel="generateOpen = false; refreshConsole()"
    />

    <TopicManageDialog
      v-model="topicManageOpen"
      :board-id="boardId"
      @changed="load()"
      @edit-lane="topicId => openLaneCorrection(String(topicId))"
    />

    <WatchManagePanel
      v-model="watchOpen"
      :board-id="boardId"
      @changed="loadWatches"
    />

    <ArticlePreviewModal
      :visible="showArticlePreview"
      :dialog-semantics="true"
      :selected-preview-article="selectedPreviewArticle"
      :preview-articles="previewArticles"
      :loading-preview-article="loadingPreviewArticle"
      @close="overlays.handleCloseArticle"
      @favorite="overlays.handleArticleFavorite"
      @article-update="overlays.handleArticleUpdate"
      @navigate="(article) => selectedPreviewArticle = article"
    />
</template>
