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
 */
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useBoardRadarApi, type RadarBoard, type RadarTopLanes } from '~/api/semanticBoards'
import { RADAR_HOT_ATTENTION, RADAR_WARM_ATTENTION, miniAxis, shortDate } from '~/features/tags/components/console/boardRadarHelpers'

/** 报头副注形态：hot=有热点 / no-hot=有关注无热点 / empty=成功空集 / failed=首次取数失败。 */
export type RadarSubKind = 'hot' | 'no-hot' | 'empty' | 'failed'

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

  const hotRows = computed(() => rows.value.filter(row => row.attention >= RADAR_HOT_ATTENTION))
  const warmRows = computed(() => rows.value.filter(row => row.attention >= RADAR_WARM_ATTENTION && row.attention < RADAR_HOT_ATTENTION))
  const coldRows = computed(() => rows.value.filter(row => row.attention < RADAR_WARM_ATTENTION))
  const visibleRows = computed(() => [...hotRows.value, ...warmRows.value])

  /** 旧响应回退（无泳道清单段）：版块级日报标题列表。 */
  const todayRead = computed(() => [...rows.value]
    .filter(row => row.reportTitle)
    .sort((a, b) => b.attention - a.attention)
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
  /** 全版块当日线索合计（报头统计用）；任一行缺该字段（旧后端）→ null（未知，不造 0）。 */
  const todayClueTotal = computed<number | null>(() => {
    let total = 0
    for (const row of rows.value) {
      if (row.todayClues === null) return null
      total += row.todayClues
    }
    return total
  })

  /** 首次取数即失败：错误且从未成功拿到数据。 */
  const firstLoadFailed = computed(() => Boolean(error.value) && rows.value.length === 0)

  const subKind = computed<RadarSubKind>(() => {
    if (firstLoadFailed.value) return 'failed'
    if (hotRows.value.length) return 'hot'
    if (visibleRows.value.length) return 'no-hot'
    return 'empty'
  })

  const dateText = computed(() => {
    if (!date.value) return ''
    const d = new Date(`${date.value}T12:00:00`)
    return new Intl.DateTimeFormat('zh-CN', { month: 'long', day: 'numeric', weekday: 'short' }).format(d)
  })

  /** 报头大标题：首次失败给中性「今日雷达」，不把 0 说成业务结论。 */
  const headline = computed(() => {
    if (firstLoadFailed.value) return '今日雷达'
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
    hotRows,
    warmRows,
    coldRows,
    visibleRows,
    todayRead,
    hasLaneFeed,
    laneItems,
    laneAnchorDate,
    partialLanes,
    degradedBoardCount,
    laggingBoardCount,
    todayClueTotal,
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
