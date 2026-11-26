/**
 * 今日雷达总览状态（rework-workshop-into-radar-and-desk 8A/R-18 拆分；
 * retire-story-adopt-event-metrics 3.2 换当天线索口径）。
 *
 * 取数（单次 GET /semantic-boards/overview）+ 派生（关注分档 / 当天线索最多的泳道
 * 清单段标记 / 报头文案）+ 去向导航（进版块 / 进阅读台 / 进 ?lane= 控制台）。
 * 数据边界：useBoardRadarApi 已在 API 层归一 camelCase/字符串 ID，本层零映射；
 * 首次取数失败（从未成功拿到数据）不得把初始化 0 呈现为业务结论（R-17），
 * 有旧数据时刷新失败保留旧值由调用方叠加错误条。
 *
 * 事件线退役（D10）：`warming` / `topStories` / `crossDayBuilding` 派生整体删除——
 * 右栏清单由后端归并/排序/截断（上限 8），前端只读 `top_lanes` 与三个诚实标记；
 * 旧后端缺该段时 `topLanes = null`，右栏回退版块级日报标题列表。
 *
 * 「没取到 ≠ 0」（fix-radar-overview-false-zero）：端点超时未完成或泳道动态计算失败的
 * 版块行带 `dataUnavailable=true`，其计数类字段是 `null`（未知）。这些行**不参与**
 * 关注度分档、不计入「N 个板块值得关注」，当日线索合计与热点数在存在未取到版块时
 * 归为未知（`null`）——报头不得拿 0 冒充「今天没有」。
 *
 * 滞后版块沉底（fix-radar-overview-false-zero 追加，2026-11-19 用户决定）：本版块自己的
 * 锚点日 ≠ 全局锚点日的行归入「滞后」段——数据是齐的，只是这个版块的「今天」更早。
 * 关注分档阈值按当日分布标定，旧数据不可比 → 滞后行**不进 hot/warm/cold**、不占前排
 * 位次，逐行沉底到列表最底部（仍可见，保留「数据至 MM/DD」中性徽标）。判定复用
 * `isLaggingAnchor`（锚点日缺失 = 没有自己的锚点日 → 不算滞后，照旧按常温处理）。
 */
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useBoardRadarApi, type RadarBoard, type RadarTopLanes } from '~/api/semanticBoards'
import { RADAR_HOT_ATTENTION, RADAR_WARM_ATTENTION, isLaggingAnchor, miniAxis, shortDate } from '~/features/tags/components/console/boardRadarHelpers'

/**
 * 报头副注形态：
 * hot=有热点 / no-hot=有关注无热点 / empty=成功空集 / failed=首次取数失败 /
 * partial=本次有版块没取到（全部或部分未取到）。
 */
export type RadarSubKind = 'hot' | 'no-hot' | 'empty' | 'failed' | 'partial'

export function useBoardRadarOverview() {
  const router = useRouter()
  const api = useBoardRadarApi()

  const rows = ref<RadarBoard[]>([])
  const date = ref('')
  const loading = ref(true)
  const error = ref('')
  /** 泳道清单段（null = 旧后端响应，无该段 → 回退版块级日报列表）。 */
  const topLanes = ref<RadarTopLanes | null>(null)

  async function load(): Promise<void> {
    loading.value = true
    error.value = ''
    const res = await api.getOverview()
    // 判别联合按 success 分支：成功成员 data 必填；异常「success 无 data」已在
    // mapRadarOverviewResponse 收敛为 success:false，仍落 error 路径。
    if (res.success) {
      rows.value = res.data.boards
      date.value = res.data.date
      // 缺段（旧后端）→ null，回退版块级列表。
      topLanes.value = res.data.topLanes ?? null
    } else {
      // 失败不清空 rows：有旧数据时保留可读（R-17/State Matrix「失败留旧数据」）
      error.value = res.error || '雷达加载失败'
    }
    loading.value = false
  }

  /**
   * 本次没取到的版块行（端点超时未完成 / 泳道动态计算失败）：计数未知（null）。
   * 它们不参与关注度分档、不计入「N 个板块值得关注」。
   */
  const unavailableRows = computed(() => rows.value.filter(row => row.dataUnavailable))
  /** 取到数据的版块行（与「未取到」严格区分）。 */
  const knownRows = computed(() => rows.value.filter(row => !row.dataUnavailable))
  /** 未取到的版块数。 */
  const unavailableCount = computed(() => unavailableRows.value.length)
  /** 本次数据不完整（至少一个版块没取到）。 */
  const partial = computed(() => unavailableCount.value > 0)
  /** 全部版块都没取到（有行且逐行未取到）。 */
  const allUnavailable = computed(() => rows.value.length > 0 && unavailableCount.value === rows.value.length)

  /**
   * 滞后版块行（本版块锚点日 ≠ 全局锚点日）：不进关注度分档、不占前排位次，
   * 逐行沉底到列表最底部（仍可见）。锚点日缺失的行不算滞后。
   */
  const laggingRows = computed(() => knownRows.value.filter(row => isLaggingAnchor(row.anchorDate, date.value)))
  /** 滞后版块数。 */
  const laggingCount = computed(() => laggingRows.value.length)
  /** 数据新鲜、锚点日与全局一致的已知行（关注度分档只对这批做）。 */
  const freshRows = computed(() => knownRows.value.filter(row => !isLaggingAnchor(row.anchorDate, date.value)))

  /** 分档只对新鲜已知行做；关注度未知（null）的行不进任何一档——不拿 0 冒充常温。 */
  const hotRows = computed(() => freshRows.value.filter(row => row.attention !== null && row.attention >= RADAR_HOT_ATTENTION))
  const warmRows = computed(() => freshRows.value.filter(row => row.attention !== null && row.attention >= RADAR_WARM_ATTENTION && row.attention < RADAR_HOT_ATTENTION))
  // 关注度未知但没标 data_unavailable 的行（协议上不可达，防御性兜底）也归常温：
  // 行内全部 null-safe 显示「—」，总比从四段全部漏出、静默消失要好。
  const coldRows = computed(() => freshRows.value.filter(row => row.attention === null || row.attention < RADAR_WARM_ATTENTION))
  const visibleRows = computed(() => [...hotRows.value, ...warmRows.value])
  /** 主列表行序（前端排序口径）：关注（热点→关注）→ 常温 → 滞后 → 未取到。 */
  const storyRows = computed(() => [...visibleRows.value, ...coldRows.value, ...laggingRows.value, ...unavailableRows.value])

  /** 旧响应回退（无泳道清单段）：版块级日报标题列表；未取到的版块没有日报标题。 */
  const todayRead = computed(() => [...rows.value]
    .filter(row => row.reportTitle && !row.dataUnavailable)
    .sort((a, b) => (b.attention ?? -1) - (a.attention ?? -1))
    .slice(0, 5))

  /** 泳道清单可用（新版后端带该段）。跨版块归并与排序全在后端完成。 */
  const hasLaneFeed = computed(() => topLanes.value !== null)
  /** 「当天线索最多的泳道」清单（≤ 8，服务端截断）。 */
  const laneItems = computed(() => topLanes.value?.lanes ?? [])
  /** 全局锚点日（右栏滞后判定的基准）：清单段自带，缺段回退总览日期。 */
  const laneAnchorDate = computed(() => topLanes.value?.anchorDate || date.value)
  /** 排序不完整（部分版块延迟或降级）。 */
  const partialLanes = computed(() => topLanes.value?.partial === true)
  /** 降级版块数（超时或泳道数据计算失败）——右栏提示带上数量。 */
  const degradedBoardCount = computed(() => topLanes.value?.degradedBoardIds.length ?? 0)
  /** 滞后版块数（各条目按自己的锚点日计——已知口径偏差如实呈现）。 */
  const laggingBoardCount = computed(() => topLanes.value?.laggingBoardIds.length ?? 0)
  /**
   * 全版块当日线索合计（报头统计用）；任一行缺该字段（旧后端）或存在未取到版块
   * → null（未知，不造 0，也不拿未取到的行当 0 计入）。
   */
  const todayClueTotal = computed<number | null>(() => {
    if (unavailableCount.value > 0) return null
    let total = 0
    // 只合「今日」的行：滞后版块的线索数是它自己那个更早的锚点日的数，
    // 混进来就是旧数据污染今日口径（与分档、排序同一把尺子）。
    for (const row of freshRows.value) {
      if (row.todayClues === null) return null
      total += row.todayClues
    }
    return total
  })

  /** 达到热点阈值的版块数；存在未取到版块时为未知（null，不据此断言「没有热点」）。 */
  const hotCount = computed<number | null>(() => (unavailableCount.value > 0 ? null : hotRows.value.length))

  /** 首次取数即失败：错误且从未成功拿到数据。 */
  const firstLoadFailed = computed(() => Boolean(error.value) && rows.value.length === 0)

  const subKind = computed<RadarSubKind>(() => {
    if (firstLoadFailed.value) return 'failed'
    // 全部未取到与部分未取到同属「本次数据不完整」一档。
    if (unavailableCount.value > 0) return 'partial'
    if (hotRows.value.length) return 'hot'
    if (visibleRows.value.length) return 'no-hot'
    return 'empty'
  })

  const dateText = computed(() => {
    if (!date.value) return ''
    const d = new Date(`${date.value}T12:00:00`)
    return new Intl.DateTimeFormat('zh-CN', { month: 'long', day: 'numeric', weekday: 'short' }).format(d)
  })

  /** 报头大标题：首次失败给中性「今日雷达」；未取到不给「0 个板块值得关注」这类结论。 */
  const headline = computed(() => {
    if (firstLoadFailed.value) return '今日雷达'
    if (allUnavailable.value) return '本次数据未取全，请重试'
    if (unavailableCount.value > 0) {
      // 已经一个关注版块都没剩（或本来就没）时，再说「0 个板块值得关注」
      // 就是从没取到的数据里推结论——直接说没取全（review R-6）。
      if (visibleRows.value.length === 0) return '本次数据未取全，请重试'
      return `${visibleRows.value.length} 个板块值得关注（${unavailableCount.value} 个版块未取到）`
    }
    const hot = hotRows.value.length
    return `${visibleRows.value.length} 个板块值得关注${hot ? `，其中 ${hot} 个达到热点阈值` : ''}`
  })

  /** 小矩阵表头日期区间：全局日期前 13 天 → 全局日期。 */
  const miniRange = computed(() => {
    if (!date.value) return ''
    return `${shortDate(miniAxis(date.value, 14)[0])}—${shortDate(date.value)}`
  })

  function openBoard(boardId: string): void {
    if (!boardId) return
    void router.push(`/tags/${boardId}`)
  }

  function openReport(row: RadarBoard): void {
    const dateKey = row.reportDate?.slice(0, 10)
    if (!dateKey) {
      openBoard(row.boardId)
      return
    }
    void router.push(`/tags/${row.boardId}/${dateKey}`)
  }

  /** 小矩阵/右栏泳道 → 既有控制台 `?lane=` 选中泳道。 */
  function openLane(payload: { boardId: string, topicId: string }): void {
    if (!payload.boardId || !payload.topicId) return
    void router.push({ path: `/tags/${payload.boardId}`, query: { lane: payload.topicId } })
  }

  return {
    rows,
    date,
    loading,
    error,
    topLanes,
    unavailableRows,
    knownRows,
    unavailableCount,
    partial,
    allUnavailable,
    laggingRows,
    laggingCount,
    freshRows,
    hotRows,
    warmRows,
    coldRows,
    visibleRows,
    storyRows,
    todayRead,
    hasLaneFeed,
    laneItems,
    laneAnchorDate,
    partialLanes,
    degradedBoardCount,
    laggingBoardCount,
    todayClueTotal,
    hotCount,
    firstLoadFailed,
    subKind,
    dateText,
    headline,
    miniRange,
    load,
    openBoard,
    openReport,
    openLane,
  }
}
