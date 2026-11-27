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
import { useDailyReportsApi } from '~/api/dailyReports'
import type { BoardConsoleStageProps } from './consoleStageTypes'
import { useConsoleShellData } from './useConsoleShellData'
import { useConsoleReturnContext, type ReadingSettledState } from './useConsoleReturnContext'
import { useConsoleShellRail } from './useConsoleShellRail'
import { useConsoleRefresh } from './useConsoleRefresh'
import { useConsoleReadingWiring } from './useConsoleReadingWiring'
import { useConsoleShellOverlays } from './useConsoleShellOverlays'
import { buildConsoleStageProps } from './consoleStageBinding'
import { buildCoverageLanes } from './coverageLanes'
import { shiftDate } from './observationRange'
import { mergeLaneDynamicsLanes } from './nowBrief'
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
const { warn: notifyWarn, success: notifySuccess, error: notifyError } = useNotify()
const isNarrow = useIsNarrowViewport()
const boardIdRef = toRef(props, 'boardId')
const dailyApi = useDailyReportsApi()

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
  overviewError,
  railRows,
  railAxisDateList,
  watches,
  watchesLoading,
  watchesError,
  loadBoards,
  loadReportMeta,
  loadWatches,
  boardTopics,
  topicsLoading,
  topicsError,
  loadBoardTopics,
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
  poolWindow,
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
  batchQueryWrite,
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

// ── 窄屏回落：关键事件视图窄屏不渲染（delta spec「窄屏不渲染」）────────────
// 窄屏深链 ?view=events 时若停在事件视图：舞台走窄屏摘要分支（视图本身不渲染），
// 而阅读槽又被 `activeView !== 'events'` 连带隐藏、头部也没有可切的按钮（窄屏不渲染
// 该按钮）→ 用户卡在空舞台、无处可去（review R-8）。
// 故窄屏一律回落矩阵（= 现有窄屏降级形态），并把 URL 里的 view=events 一并去掉
// （URL 与画面保持一致，刷新不会又掉进来）。
watch(isNarrow, (narrow) => {
  if (narrow && activeView.value === 'events') setView('matrix')
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

/**
 * 泳道显示名（修正工作面标题 / 归档弹窗文案用）。
 *
 * 先查 A 档名单（本窗口有分区的泳道）；查不到再回退板块话题清单——覆盖组（窗口内未进
 * 日报）的泳道按定义不在 A 档名单里，但「归档泳道」这类危险操作必须先让用户看清对象。
 * 两处都没有才留空。
 */
function laneLabelOf(topicId: string): string {
  return allLanes.value.find(row => row.topicId === topicId)?.label
    ?? boardTopics.value.find(topic => String(topic.id) === topicId)?.label
    ?? ''
}

/** 修正工作面的泳道名。 */
const correctionLaneLabel = computed(() => (correctionTopicId.value ? laneLabelOf(correctionTopicId.value) : ''))

/** 归档弹窗的泳道名。 */
const archiveLaneLabel = computed(() => (archiveTopicId.value ? laneLabelOf(String(archiveTopicId.value)) : ''))

/** 就地改名后刷新名单（泳道名来自 A 档名单，不刷新会显示旧名）+ 覆盖组清单。 */
async function handleLaneRenamed(): Promise<void> {
  await Promise.all([load(), loadBoardTopics()])
}

/** 归档后关闭弹窗并刷新名单（覆盖组清单一并刷新：归档后该泳道不再是 active）。 */
async function handleLaneArchived(): Promise<void> {
  closeLaneArchive()
  await Promise.all([load(), loadBoardTopics()])
}

/**
 * 新建泳道保存后：既有链（关弹层 + 刷名单 + 选中该泳道）不变，额外刷一次覆盖组清单
 * （新建的泳道当前窗口没分区 → 会出现在「窗口内未进日报」组）。
 */
async function handleComposeSaved(topic?: Parameters<typeof overlays.handleComposeSaved>[0]): Promise<void> {
  await overlays.handleComposeSaved(topic)
  await loadBoardTopics()
}

/** 话题管理（合并/拆分/转正/归档）后：名单与覆盖组清单一并刷新（活跃集合变了）。 */
async function handleTopicsChanged(): Promise<void> {
  await Promise.all([load(), loadBoardTopics()])
}

/**
 * 行头就地重命名（矩阵行头「⋯」→「重命名」）。
 *
 * 复用既有写入 `PATCH /daily-reports/topics/:id`（与修正工作面同一条路径）；成功刷 A 档名单
 * 与覆盖组清单，失败如实报错并返回 false（调用方保留输入框，不伪装成功）。
 */
async function renameLane(topicId: string, label: string): Promise<boolean> {
  const id = Number(topicId)
  if (!Number.isSafeInteger(id) || id <= 0) {
    notifyError('泳道 ID 无效')
    return false
  }
  const trimmed = label.trim()
  if (!trimmed) {
    notifyWarn('泳道名不能为空')
    return false
  }
  const res = await dailyApi.updateTopic(id, { label: trimmed })
  if (!res.success) {
    notifyError(res.error || '泳道名更新失败')
    return false
  }
  notifySuccess('泳道名已更新')
  await Promise.all([load(), loadBoardTopics()])
  return true
}

/** 覆盖组（窗口内未进日报的活跃泳道）：活跃话题 − A 档名单行。 */
const coverageLanes = computed(() => buildCoverageLanes(allLanes.value, boardTopics.value))

/**
 * 「其他话题（未入泳道）」逐日读数（后端 `other_topics`）。
 *
 * 旧后端不下发该字段 → `otherTopicDaysAvailable` 为 false → 矩阵整行不渲染
 * （不显示全「—」的空壳行）。
 */
const otherTopicDays = computed(() => sourceWindow.value?.otherTopics ?? {})
const otherTopicDaysAvailable = computed(() => sourceWindow.value?.otherTopicsAvailable ?? false)

/**
 * 「其他话题（未入泳道）」逐日读数的**档窗起点**。
 *
 * 后端 `other_topics` 只聚合档窗 `[anchor-(days-1), anchor]`；观察范围拖到 365 池区段时，
 * 那些日期没有 entry 是「本次聚合没算」而不是「没有日报」——行组件据此区分文案。
 */
const otherTopicsSince = computed(() => {
  const anchor = sourceWindow.value?.anchorDate
  if (!anchor) return null
  return shiftDate(anchor, -(sourceDays.value - 1))
})

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
  batchQueryWrite,
})

// ── 体检 / 构成 / 右轨上下文（同一份真实数据；装配在 useConsoleShellRail） ──
/** lane-dynamics 泳道清单（A 档 + 365 池合并）：右轨泳道线索数与矩阵日格同源同值。 */
const laneDynamicsLanes = computed(() => mergeLaneDynamicsLanes(
  sourceWindow.value?.lanes ?? [],
  poolWindow.value?.lanes ?? [],
))

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
  reportDay: computed(() => wiring.stageProps.value.reportDay),
  selectedDate,
  laneDynamicsLanes,
  // 「当日其他话题」组视图模型：壳层已算好（矩阵同源），右轨只消费（零新增请求）。
  otherTopics: wiring.otherTopics,
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
  otherTopicDays: otherTopicDays.value,
  otherTopicDaysAvailable: otherTopicDaysAvailable.value,
  otherTopicsSince: otherTopicsSince.value,
  coverageLanes: coverageLanes.value,
  coverageLoading: topicsLoading.value,
  coverageError: topicsError.value,
  renameLane,
  reading: wiring.stageProps.value,
}))

async function handleBoardSaved(): Promise<void> {
  await loadBoards()
}

/** 左轨重试：版块列表与 overview 信号（同一批请求，零新增）。 */
async function handleRetryRail(): Promise<void> {
  await Promise.all([loadBoards(), loadReportMeta()])
}

/** 右轨「当日日报」点评标题 → 跳到该期日报正文的对应聚簇（inline-edit）。 */
function handleOpenReportCluster(payload: { date: string, clusterId: number | null }): void {
  returnContext.openReport(payload.date, payload.clusterId)
}

/** 右轨「当日日报」详情失败 → 只重取当日详情（不重建索引、不改选中日）。 */
async function handleRetryNow(date: string): Promise<void> {
  await wiring.retryReportDay(date)
}

/**
 * 「当日其他话题」选行（矩阵折叠组 / 右轨子块同一入口）：选中该分区并由主区既有阅读槽
 * 承接——右轨只发事件，不自己改选择态、不另开浮层。开着工作面时先关工作面
 * （同「定位泳道」：正文在幕后的阅读槽里）。
 */
function handleSelectOther(payload: { sectionId: string, date: string, topicId?: string | null }): void {
  closeWorkface()
  wiring.selectOther(payload)
}

/**
 * 右轨「当日其他话题」选行：与舞台内同名动作同一落点；**右轨发起**且当前在关键事件
 * 视图时要先切回矩阵 / 追踪视图（关键事件视图没有其他话题组，阅读槽也不在那里；
 * 不切就等于选了个寂寞）。
 * 判据在发起处区分：舞台内选中仍留在当前视图；矩阵 / 归档视图里的右轨选中**不切视图**
 * （design D14 / delta spec「右轨选中切回矩阵视图」；review R-1：归档视图里的右轨点击
 * 不得静默把用户切走）。
 */
function handleSelectOtherFromRail(payload: { sectionId: string, date: string, topicId?: string | null }): void {
  // 与「话题追踪」同纪律：同一动作里的 query 写入合并成一次 replace（实机验收 A-34）。
  // 本路径本身只写一次（不写 lane），合并是为口径一致 + 防后续新增写入再起竞争。
  batchQueryWrite(() => {
    if (activeView.value === 'events') handleSetView('matrix')
    handleSelectOther(payload)
  })
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
      :rail-rows="railRows"
      :rail-axis-dates="railAxisDateList"
      :signal-error="overviewError"
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
      @retry-boards="handleRetryRail"
      @select-lane="wiring.selectLaneFromStage"
      @edit-lane="overlays.openLaneCorrection"
      @archive-lane="overlays.openLaneArchive"
      @select-cell="wiring.selectCellFromStage"
      @select-date="wiring.selectDateFromStage"
      @select-day-header="wiring.selectDayHeader"
      @select-other="handleSelectOther"
      @select-others-day="wiring.selectOthersDayFromStage"
      @retry-coverage="loadBoardTopics"
      @select-other-from-rail="handleSelectOtherFromRail"
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
      @open-report-cluster="handleOpenReportCluster"
      @retry-now="handleRetryNow"
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
      @select-other="handleSelectOtherFromRail"
      @retry-now="handleRetryNow"
      @open-watch-manage="overlays.openWatchManage"
      @open-topics="overlays.openTopicManage"
    />

    <BoardComposeOverlay
      v-if="composeOpen"
      :board-id="boardId"
      :board-label="boardLabel"
      @saved="handleComposeSaved"
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
      @changed="handleTopicsChanged"
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
