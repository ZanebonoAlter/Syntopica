/**
 * 今日雷达总览状态（rework-workshop-into-radar-and-desk 8A/R-18 拆分）。
 *
 * 取数（单次 GET /semantic-boards/overview）+ 派生（关注分档 / 今日该读 /
 * 升温 / 报头文案）+ 去向导航（进版块 / 进阅读台 / 进 ?lane= 控制台）。
 * 数据边界：useBoardRadarApi 已在 API 层归一 camelCase/字符串 ID，本层零映射；
 * 首次取数失败（从未成功拿到数据）不得把初始化 0 呈现为业务结论（R-17），
 * 有旧数据时刷新失败保留旧值由调用方叠加错误条。
 */
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useBoardRadarApi, type RadarBoard, type RadarStoryFeed, type RadarStoryItem } from '~/api/semanticBoards'
import { RADAR_HOT_ATTENTION, RADAR_WARM_ATTENTION, hasHeatSignal, miniAxis, shortDate } from '~/features/tags/components/console/boardRadarHelpers'

export interface RadarWarmingItem {
  row: RadarBoard
  ratio: number
  /** 展示用：当日事件线数（无 heat 回退当日分区数，旧响应兼容）。 */
  todayCount: number
  /** 展示用：近 3 日事件线数日均（无 heat 回退分区数口径；热度倍数另由 ratio 表达）。 */
  baselineAvg: number
}

/** 报头副注形态：hot=有热点 / no-hot=有关注无热点 / empty=成功空集 / failed=首次取数失败。 */
export type RadarSubKind = 'hot' | 'no-hot' | 'empty' | 'failed'

export function useBoardRadarOverview() {
  const router = useRouter()
  const api = useBoardRadarApi()

  const rows = ref<RadarBoard[]>([])
  const date = ref('')
  const loading = ref(true)
  const error = ref('')
  /** 事件线清单段（null = 旧后端响应，无该段 → 回退版块级列表）。 */
  const stories = ref<RadarStoryFeed | null>(null)

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
      stories.value = res.data.stories ?? null
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

  /** 旧响应回退（无事件线清单段）：版块级日报标题列表 + 版块级升温清单。 */
  const todayRead = computed(() => [...rows.value]
    .filter(row => row.reportTitle)
    .sort((a, b) => b.attention - a.attention)
    .slice(0, 5))

  /** 升温（workshop-radar 契约）：当日事件线热度 ≥ 近 3 日日均热度 2 倍且当日事件线数 ≥3；
   *  无 heat 字段（旧响应）回退旧 sections 口径（当日分区 ≥3 且 ≥ 近 3 日均值 2 倍）。 */
  const warming = computed<RadarWarmingItem[]>(() => rows.value
    .map((row) => {
      const heat = hasHeatSignal(row)
      const todayValue = heat ? row.todayHeat : row.todaySections
      const heatBaseline = (heat ? row.last3Heat : row.last3Sections) / 3
      const ratio = heatBaseline > 0 ? todayValue / heatBaseline : 0
      return {
        row,
        ratio,
        todayCount: heat ? row.todayStories : row.todaySections,
        baselineAvg: (heat ? row.last3Stories : row.last3Sections) / 3,
      }
    })
    .filter(item => item.todayCount >= 3 && item.ratio >= 2)
    .sort((a, b) => b.ratio - a.ratio))

  /**
   * 跨天延续脚注判据（matrix-clue-fallback-and-rail-merge D3）：服务端
   * `cross_day_story_count == 0` → 根本不存在跨天延续的事件线 → 出一行脚注
   * 「跨天延续建设中」。> 0（有跨天线但没升温）与 null（旧响应缺字段、无法判定）
   * 都不出脚注——有徽标已表达升温与否，无字段不得断言成因。
   */
  const crossDayBuilding = computed(() => stories.value?.crossDayStoryCount === 0)

  /** 事件线清单可用（新版后端带该段）。跨版块归并与排序全在后端完成。 */
  const hasStoryFeed = computed(() => stories.value !== null)
  /** 「今日该读」单清单：跨版块全局 Top N 事件线（≤ 8，行内徽标承载分档）。 */
  const topStories = computed<RadarStoryItem[]>(() => stories.value?.topStories ?? [])
  /** 未回填版块数（事件身份未回填 → 未纳入事件线排序）。 */
  const unbackfilledBoardCount = computed(() => stories.value?.unbackfilledBoardCount ?? 0)
  /** 排序不完整（部分版块降级）。 */
  const partialStories = computed(() => stories.value?.partial === true)
  /** 降级版块数（超时或事件线计算失败）——右栏提示带上数量。 */
  const degradedBoardCount = computed(() => stories.value?.degradedBoardIds.length ?? 0)
  /** 滞后版块数（各条目按自己的锚点日计——已知口径偏差如实呈现）。 */
  const laggingBoardCount = computed(() => stories.value?.laggingBoardIds.length ?? 0)
  /** 报头升温计数：新版用服务端全量计数（旧响应回退版块级清单长度）。 */
  const warmingCount = computed(() => stories.value ? stories.value.warmingCount : warming.value.length)

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
  const headline = computed(() => firstLoadFailed.value
    ? '今日雷达'
    : `${visibleRows.value.length} 个板块值得关注，${warmingCount.value ? `${warmingCount.value} 处出现升温` : '暂未出现升温'}`)

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

  /** 小矩阵泳道 → 既有控制台 `?lane=` 选中泳道。 */
  function openLane(payload: { boardId: string, topicId: string }): void {
    void router.push({ path: `/tags/${payload.boardId}`, query: { lane: payload.topicId } })
  }

  /** 事件线深链：跳该事件线落点的日报并定位正文（`?story=`）。
   *  无落点报告日期的条目不可点（调用方不渲染为按钮）。 */
  function openStory(item: RadarStoryItem): void {
    const reportDate = item.reportDate?.slice(0, 10)
    if (!item.boardId || !reportDate || !item.storyId) return
    void router.push({
      path: `/tags/${item.boardId}/${reportDate}`,
      query: { story: item.storyId },
    })
  }

  return {
    rows,
    date,
    loading,
    error,
    stories,
    hotRows,
    warmRows,
    coldRows,
    visibleRows,
    todayRead,
    warming,
    hasStoryFeed,
    topStories,
    crossDayBuilding,
    unbackfilledBoardCount,
    partialStories,
    degradedBoardCount,
    laggingBoardCount,
    warmingCount,
    firstLoadFailed,
    subKind,
    dateText,
    headline,
    miniRange,
    load,
    openBoard,
    openReport,
    openLane,
    openStory,
  }
}
