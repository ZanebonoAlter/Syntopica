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
import { useBoardRadarApi, type RadarBoard } from '~/api/semanticBoards'
import { RADAR_HOT_ATTENTION, RADAR_WARM_ATTENTION, miniAxis, shortDate } from '~/features/tags/components/console/boardRadarHelpers'

export interface RadarWarmingItem {
  row: RadarBoard
  ratio: number
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

  async function load(): Promise<void> {
    loading.value = true
    error.value = ''
    const res = await api.getOverview()
    // 判别联合按 success 分支：成功成员 data 必填；异常「success 无 data」已在
    // mapRadarOverviewResponse 收敛为 success:false，仍落 error 路径。
    if (res.success) {
      rows.value = res.data.boards
      date.value = res.data.date
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

  const todayRead = computed(() => [...rows.value]
    .filter(row => row.reportTitle)
    .sort((a, b) => b.attention - a.attention)
    .slice(0, 5))

  const warming = computed(() => rows.value
    .map((row) => {
      const baseline = row.last3Sections / 3
      const ratio = baseline > 0 ? row.todaySections / baseline : 0
      return { row, ratio }
    })
    .filter(item => item.row.todaySections >= 3 && item.ratio >= 2)
    .sort((a, b) => b.ratio - a.ratio))

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
    : `${visibleRows.value.length} 个板块值得关注，${warming.value.length ? `${warming.value.length} 处出现升温` : '暂未出现升温'}`)

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

  return {
    rows,
    date,
    loading,
    error,
    hotRows,
    warmRows,
    coldRows,
    visibleRows,
    todayRead,
    warming,
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
