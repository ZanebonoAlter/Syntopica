/**
 * 版块级关键事件数据源（add-key-event-matrix-view 2.2/2.3，design D3/D5）。
 *
 * 消费既有 `GET /semantic-boards/:id/key-events?days=N`（只读批量端点），为矩阵级
 * 关键事件视图一次取全观察窗内的 confirmed 注记：
 *  - 按 board + days 分键缓存（同窗往返不重发；切档/切板块重取）；
 *  - loading / error / stale 三态：刷新失败保留旧内容并标 stale，MUST NOT 冒称 0 条；
 *    stale 时保留的是**旧版块**的数据，故发布时快照来源版块（id + 名），
 *    提示条点名它，避免用户以为旧数据就是当前版块的事件（review R-7）；
 *  - 晚到隔离：请求序号（seq）不匹配的响应不发布（切板块/切档后旧响应不得污染新状态）。
 *
 * 纯函数 `buildKeyEventIndex`（topicId → date → events）供视图按行/按日取事件，
 * 同日多事件保持接口序；无话题归属 / 未命中行集合的事件**不进话题索引**（视图层
 * `groupUnassignedEvents` 会把它们收进「其他」兜底行，MUST NOT 在索引层丢弃语义）。
 */
import { computed, ref, watch, type Ref } from 'vue'
import { useBoardKeyEventsApi } from '~/api/boardKeyEvents'
import type { BoardKeyEventItem } from '~/api/boardKeyEvents'

/** 缓存键：board + days（同键往返不重发）。 */
function cacheKey(boardId: number, days: number): string {
  return `${boardId}:${days}`
}

/** 事件索引：topicId → date → 该日事件列表（接口序）。未知话题（null）丢弃。 */
export type BoardKeyEventIndex = Map<string, Map<string, BoardKeyEventItem[]>>

/**
 * 纯函数：扁平事件列表 → topicId + date 二级索引（tasks 2.3）。
 * 同日多事件顺序按接口序；`topicId == null` 的事件不进索引。
 */
export function buildKeyEventIndex(events: readonly BoardKeyEventItem[]): BoardKeyEventIndex {
  const index: BoardKeyEventIndex = new Map()
  for (const event of events) {
    if (event.topicId == null) continue
    let byDate = index.get(event.topicId)
    if (!byDate) {
      byDate = new Map()
      index.set(event.topicId, byDate)
    }
    const list = byDate.get(event.sourcePeriodDate)
    if (list) list.push(event)
    else byDate.set(event.sourcePeriodDate, [event])
  }
  return index
}

/**
 * 纯函数：未归入给定话题集合的事件 → 按来源日期分组（「其他」兜底行数据源）。
 *
 * 收录两类：`topicId == null`（话题已删除 / 历史无归属）与 `topicId` 不在
 * `assignedTopicIds`（未关注 candidate、archived、窗口内无分区而未成行的话题）。
 * 同日顺序按接口序（与 `buildKeyEventIndex` 同口径）；空结果返回空 Map。
 */
export function groupUnassignedEvents(
  events: readonly BoardKeyEventItem[],
  assignedTopicIds: ReadonlySet<string>,
): Map<string, BoardKeyEventItem[]> {
  const byDate = new Map<string, BoardKeyEventItem[]>()
  for (const event of events) {
    if (event.topicId != null && assignedTopicIds.has(event.topicId)) continue
    const list = byDate.get(event.sourcePeriodDate)
    if (list) list.push(event)
    else byDate.set(event.sourcePeriodDate, [event])
  }
  return byDate
}

export interface UseBoardKeyEventsDeps {
  boardId: Ref<number>
  /** 观察窗天数（正整数；与矩阵共用 sourceDays，不维护第二套跨度）。 */
  days: Ref<number>
  /**
   * 版块名（可选）：stale 提示要**点名这批数据属于哪个版块**，发布时快照一次
   * （切版块后当前版块名会变，快照才不会指错）。
   */
  boardLabel?: Ref<string>
}

/** 当前展示内容的来源版块（stale 时点名用；null = 还没有内容）。 */
export interface KeyEventContentSource {
  boardId: number
  boardLabel: string
}

export function useBoardKeyEvents(deps: UseBoardKeyEventsDeps) {
  const api = useBoardKeyEventsApi()

  const events = ref<BoardKeyEventItem[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  /** 刷新失败但保留旧内容（顶部提示条；MUST NOT 冒称 0 条）。 */
  const staleNotice = ref(false)
  /**
   * 当前展示内容的来源版块（发布时快照）：刷新/切版块失败后保留的是**旧版块**的数据，
   * 提示条必须点名它属于哪个版块，否则用户会以为旧数据就是当前版块的事件（review R-7）。
   */
  const contentSource = ref<KeyEventContentSource | null>(null)

  /** 有界缓存（board+days → 已发布事件；上限 8，FIFO 逐出）。 */
  const cache = new Map<string, BoardKeyEventItem[]>()
  const CACHE_LIMIT = 8

  /** 晚到隔离：切 board/days/重试使旧响应作废。 */
  let seq = 0

  async function load(options: { force?: boolean } = {}): Promise<void> {
    const boardId = deps.boardId.value
    const days = deps.days.value
    const key = cacheKey(boardId, days)
    if (!options.force && cache.has(key)) {
      events.value = cache.get(key)!
      contentSource.value = { boardId, boardLabel: deps.boardLabel?.value ?? '' }
      error.value = null
      staleNotice.value = false
      return
    }
    const current = ++seq
    loading.value = true
    error.value = null
    staleNotice.value = false
    const res = await api.getBoardKeyEvents(boardId, days)
    if (current !== seq) return
    loading.value = false
    if (res.success && res.data) {
      cache.set(key, res.data.events)
      if (cache.size > CACHE_LIMIT) {
        cache.delete(cache.keys().next().value as string)
      }
      events.value = res.data.events
      // 发布时快照来源版块（切版块后当前版块名会变；stale 提示要指向**这批数据**的版块）。
      contentSource.value = { boardId, boardLabel: deps.boardLabel?.value ?? '' }
    } else {
      // 失败：保留旧内容 + stale 标记；首次失败（无旧内容）走 error 全量态。
      error.value = res.error || res.message || '关键事件读取失败'
      if (events.value.length > 0 || cache.has(key)) staleNotice.value = true
    }
  }

  /** 重试入口（绕过缓存重发；在途去重由 seq 保证只发布最新）。 */
  function retry(): void {
    void load({ force: true })
  }

  watch([deps.boardId, deps.days], () => {
    void load()
  }, { immediate: true })

  const eventIndex = computed(() => buildKeyEventIndex(events.value))

  /** 选中日全部事件（跨话题；接口序）。 */
  const eventsByDate = computed(() => {
    const byDate = new Map<string, BoardKeyEventItem[]>()
    for (const event of events.value) {
      const list = byDate.get(event.sourcePeriodDate)
      if (list) list.push(event)
      else byDate.set(event.sourcePeriodDate, [event])
    }
    return byDate
  })

  return {
    events,
    eventIndex,
    eventsByDate,
    loading,
    error,
    staleNotice,
    /** 当前展示内容的来源版块（stale 提示点名用）。 */
    contentSource,
    load,
    retry,
  }
}
