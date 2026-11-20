/**
 * 版块控制台壳数据（rework-workshop-into-radar-and-desk 矩阵阶段 G5）。
 *
 * 只负责三份壳级只读数据，不参与矩阵/观察窗状态：
 *  - 版块列表（左栏/面包屑/缺省态）；
 *  - 最新日报独立元信息：优先 `/semantic-boards/overview` 的版块 report_date（0 active
 *    泳道版块也有真实锚），失败才退回 days=365 日报列表（锚今天，仅兜底）；
 *  - 当前板块关注记录（真实 boardId 作用域；WatchManagePanel 仍是唯一写入口）。
 *
 * console-rails-live-signals：overview 响应里**本来就被丢弃**的版块行（逐日线索轴 /
 * 锚点日）在此提升为共享状态，供左轨条码读模型消费——**零新增请求**（同一次
 * `getOverview()` 调用；overview 失败不阻塞版块列表与矩阵，左轨退「全部未取到」形态）。
 */
import { computed, onMounted, ref, watch, type Ref } from 'vue'
import {
  useBoardRadarApi,
  useSemanticBoardsApi,
  type SemanticBoard,
} from '~/api/semanticBoards'
import type { RadarBoard } from '~/api/normalizers/boardRadar'
import { useDailyReportsApi } from '~/api/dailyReports'
import { useTopicWatchesApi, type TopicWatch } from '~/api/topicWatches'
import { buildRailBoardRows, railAxisDates } from './railClueAxis'

export function useConsoleShellData(boardId: Ref<number>) {
  const sbApi = useSemanticBoardsApi()
  const radarApi = useBoardRadarApi()
  const dailyApi = useDailyReportsApi()
  const watchesApi = useTopicWatchesApi()

  const boards = ref<SemanticBoard[]>([])
  const boardsLoading = ref(false)
  const boardsError = ref<string | null>(null)

  const latestReportDate = ref<string | null>(null)
  const latestReportTitle = ref('')
  /** overview 的版块行（逐日线索轴 / 锚点日）；左轨条码读模型的输入。 */
  const overviewRows = ref<RadarBoard[]>([])
  /** overview 的全局锚点日（共同轴末日）。 */
  const overviewGlobalDate = ref('')
  /** overview 本次没取到（左轨退「全部未取到」形态，不阻塞版块列表与矩阵）。 */
  const overviewError = ref<string | null>(null)
  /** overview + 兜底列表均失败且从未成功：不代表「从无历史日报」。 */
  const reportMetaUnavailable = ref(false)
  /** 刷新失败但保留同板块最后一次成功元信息。 */
  const reportMetaStale = ref(false)
  const reportMetaLoading = ref(false)
  let reportSeq = 0

  const watches = ref<TopicWatch[]>([])
  const watchesLoading = ref(false)
  const watchesError = ref<string | null>(null)
  let watchSeq = 0

  async function loadBoards(): Promise<void> {
    boardsLoading.value = true
    boardsError.value = null
    const res = await sbApi.getBoards()
    if (res.success && res.data) boards.value = res.data.items
    else boardsError.value = res.error || '版块列表加载失败'
    boardsLoading.value = false
  }

  function resetReportMeta(): void {
    latestReportDate.value = null
    latestReportTitle.value = ''
    reportMetaUnavailable.value = false
    reportMetaStale.value = false
  }

  /**
   * 读取独立 report 元信息：overview 优先、有界列表兜底。
   * 同板块刷新失败保留最后成功值并标 stale；从未成功才置 unavailable（不把有限列表空当历史不存在）。
   */
  async function loadReportMeta(): Promise<void> {
    const seq = ++reportSeq
    reportMetaLoading.value = true
    const hadValue = latestReportDate.value !== null
    try {
      const overview = await radarApi.getOverview()
      if (seq !== reportSeq) return

      let nextDate = ''
      let nextTitle = ''
      if (overview.success) {
        // overview 行提升为共享状态（左轨条码）：同一次请求，零新增。
        overviewRows.value = overview.data.boards
        overviewGlobalDate.value = overview.data.date
        overviewError.value = null
        const row = overview.data.boards.find(item => item.boardId === String(boardId.value))
        nextDate = row?.reportDate ? row.reportDate.slice(0, 10) : ''
        nextTitle = row?.reportTitle ?? ''
      } else {
        // 本次没取到 → 左轨整列未取到（不拿旧值冒充本次信号）。
        overviewRows.value = []
        overviewGlobalDate.value = ''
        overviewError.value = overview.error || '版块信号未取到'
      }

      let fallbackTried = false
      let fallbackSuccess = false
      if (!nextDate) {
        const fallback = await dailyApi.getBoardDailyReports(boardId.value, { days: 365 })
        if (seq !== reportSeq) return
        fallbackTried = true
        fallbackSuccess = fallback.success
        const latest = fallback.data?.reports?.[0]
        if (fallback.success && latest) {
          nextDate = latest.period_date.slice(0, 10)
          nextTitle = latest.title || ''
        }
      }

      if (nextDate) {
        latestReportDate.value = nextDate
        latestReportTitle.value = nextTitle
        reportMetaUnavailable.value = false
        reportMetaStale.value = false
        return
      }

      // 两源都成功且确实为空 → 清空；任一失败/歧义 → 保留旧值并标陈旧。
      const allKnownEmpty = overview.success && (!fallbackTried || fallbackSuccess)
      if (allKnownEmpty) {
        latestReportDate.value = null
        latestReportTitle.value = ''
        reportMetaUnavailable.value = false
        reportMetaStale.value = false
        return
      }
      if (hadValue) {
        reportMetaStale.value = true
        reportMetaUnavailable.value = false
      } else {
        latestReportDate.value = null
        latestReportTitle.value = ''
        reportMetaUnavailable.value = true
        reportMetaStale.value = false
      }
    } catch {
      if (seq !== reportSeq) return
      // 异常路径同样算「本次没取到」：左轨退全未取到形态（不拿旧值冒充本次信号）。
      overviewRows.value = []
      overviewGlobalDate.value = ''
      overviewError.value = '版块信号未取到'
      if (hadValue) {
        reportMetaStale.value = true
        reportMetaUnavailable.value = false
      } else {
        latestReportDate.value = null
        latestReportTitle.value = ''
        reportMetaUnavailable.value = true
        reportMetaStale.value = false
      }
    } finally {
      if (seq === reportSeq) reportMetaLoading.value = false
    }
  }

  async function loadWatches(): Promise<void> {
    const seq = ++watchSeq
    watchesLoading.value = true
    watchesError.value = null
    const res = await watchesApi.listWatches(boardId.value)
    if (seq !== watchSeq) return
    if (res.success && res.data) watches.value = res.data
    else {
      watches.value = []
      watchesError.value = res.error || '关注记录加载失败'
    }
    watchesLoading.value = false
  }

  onMounted(() => {
    void loadBoards()
    void loadReportMeta()
    void loadWatches()
  })

  watch(boardId, () => {
    resetReportMeta()
    void loadReportMeta()
    void loadWatches()
  })

  const currentBoard = computed(() => boards.value.find(board => board.id === boardId.value) ?? null)
  const boardLabel = computed(() => currentBoard.value?.label ?? `版块 #${boardId.value}`)
  const boardMissing = computed(
    () => !boardsLoading.value && !boardsError.value && boards.value.length > 0 && currentBoard.value === null,
  )

  /** 左轨行视图模型（共同轴重对齐 + 峰值/强度；行序 = 版块列表序）。 */
  const railRows = computed(() => buildRailBoardRows(boards.value, overviewRows.value, overviewGlobalDate.value))
  /** 左轨共同轴日期（14 天，oldest→newest）。 */
  const railAxisDateList = computed(() => railAxisDates(overviewGlobalDate.value))

  return {
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
    overviewRows,
    overviewGlobalDate,
    overviewError,
    railRows,
    railAxisDateList,
    watches,
    watchesLoading,
    watchesError,
    loadBoards,
    loadReportMeta,
    loadWatches,
  }
}
