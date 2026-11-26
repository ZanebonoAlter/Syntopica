/**
 * 观察窗状态机（rework-workshop-into-radar-and-desk 矩阵阶段 G1，design D11 A 路线）。
 *
 * 职责（由 useBoardConsole 组合，状态引用对 G2–G5 透出）：
 *  - 观察范围 `observation`：纯显示裁切区间；**任何日期动作都不改 sourceDays/base**
 *    （OX-1：拖/输/缩放至恰 7/14/30/365 日也不切档；切档只能经 onTierChange 的明确动作）。
 *  - B 通道（365 池）：A 数据到位 + 锚点已知即**预取**（首屏顶部时间视窗图要整条 365 池曲线，
 *    不能只画当前档窗）；观察范围跨出档窗时另有 needsPool 触发（同一条 ensurePool，幂等）。
 *    仅补 A 同 ID 历史；A 失败 B 成功不冒充（名单/排序/休眠/候选永远在 A 侧计算）。
 *  - URL query 同步：`days` 仅合法四档（外部变更为明确切档；非法给提示不请求）；
 *    `start/end` 严格校验（缺项/格式/顺序/池外四类文案），非法忽略该项且原视窗保持；
 *    无关 query 键（lane/report/annotation…）原样保留。
 *  - 失败分层：A 成功 B 失败 → A 可读、历史未取到（unavailable，不补 0）、范围不动、
 *    可单独重试 B；B 刷新失败保留旧池（poolStaleNotice）。
 */
import { computed, ref, watch, type Ref } from 'vue'
import type { LocationQuery } from 'vue-router'
import type { LaneDynamicsWindowData, WindowLaneDay } from '~/api/normalizers/laneDynamicsWindow'
import type { LaneChannelLoadResult } from './laneDynamicsChannels'
import {
  POOL_DAYS,
  daysBetweenInclusive,
  defaultRangeForTier,
  parseTierDaysValue,
  sameRange,
  shiftDate,
  validateObservationRange,
  type DateBounds,
  type ObservationRange,
} from '../components/console/observationRange'

export interface ObservationRouteLike {
  query?: LocationQuery
}

export interface ObservationRouterLike {
  replace: (to: { query: LocationQuery }) => unknown
}

/** 单日内容解析：content=有分区（已知 0 线索合法）/ gap=断档（该日无分区）/ unavailable=历史未取到。 */
export type LaneDayResolution =
  | { kind: 'content', source: 'tier' | 'pool', day: WindowLaneDay }
  | { kind: 'gap' }
  | { kind: 'unavailable' }

export interface ObservationWindowDeps {
  boardId: Ref<number>
  /** 档（A 通道 days）；本模块只读，切档经 onTierChange 回调。 */
  sourceDays: Ref<number>
  /** 数据锚点（YYYY-MM-DD | null）：报告锚点 ?? A 数据最新日（useBoardConsole 组合）。 */
  anchorDate: () => string | null
  /** A 通道归一数据（legacy `data` 的 camelCase 平面）。 */
  sourceWindow: () => LaneDynamicsWindowData | null
  /** B 通道取数（365 池；channel store 接线，epoch 由 store 与本模块双层守卫）。 */
  loadPool: (options?: { force?: boolean }) => Promise<LaneChannelLoadResult>
  /** 外部 URL `days` 合法变更 → useBoardConsole 明确切档。 */
  onTierChange: (days: number) => void
  route?: () => ObservationRouteLike | undefined
  router?: () => ObservationRouterLike | undefined
}

export function useObservationWindow(deps: ObservationWindowDeps) {
  const observation = ref<ObservationRange | null>(null)
  /** URL 已反映的已提交范围（null = URL 无 start/end）；预览（writeQuery:false）不触碰（R-22）。 */
  const committedRange = ref<ObservationRange | null>(null)
  /** start/end 校验问题文案（原视窗保持不变时展示）。 */
  const rangeNotice = ref<string | null>(null)
  /** 观察范围是否已被用户显式设置（URL 恢复/拖选/输入/切档重置；G5 元信息晚到不覆盖）。 */
  const rangeTouched = ref(false)
  /** URL days 非法提示（归一回默认档/当前档，不发非法请求）。 */
  const daysNotice = ref<string | null>(null)

  const poolWindow = ref<LaneDynamicsWindowData | null>(null)
  const poolLoading = ref(false)
  const poolError = ref<string | null>(null)
  /** 池刷新失败但保留旧池数据。 */
  const poolStaleNotice = ref(false)

  /** B 通道 epoch：切板块/复位时使在途池响应作废。 */
  let poolSeq = 0
  /** R35：明确切档后忽略 router 尚未提交的旧 start/end（缓存秒回时尤其重要）。 */
  let suppressQueryRestore = false

  function readQueryValue(key: string): string | undefined {
    const raw = deps.route?.()?.query?.[key]
    const value = Array.isArray(raw) ? raw[0] : raw
    return typeof value === 'string' ? value : undefined
  }

  function writeQuery(update: (query: LocationQuery) => void): void {
    const router = deps.router?.()
    const route = deps.route?.()
    if (!router || !route) return
    const query: LocationQuery = { ...route.query }
    update(query)
    router.replace({ query })
  }

  /** 明确切档写 query：days=档值；范围将重置为该档默认 → 同步清掉 start/end 深链。 */
  function writeTierQuery(days: number): void {
    writeQuery((query) => {
      query.days = String(days)
      delete query.start
      delete query.end
    })
  }

  /** 观察范围提交写 query（拖选过程可用 writeQuery:false 静默更新，提交时再落 URL）。 */
  function writeRangeQuery(range: ObservationRange): void {
    writeQuery((query) => {
      query.start = range.start
      query.end = range.end
    })
  }

  /** 池边界：真实 365 日窗口 [anchor-364, anchor]（与后端 days=365 同锚同窗，含两端）。 */
  const poolBounds = computed<DateBounds | null>(() => {
    const anchor = deps.anchorDate()
    return anchor ? { start: shiftDate(anchor, -(POOL_DAYS - 1)), end: anchor } : null
  })

  /** 观察范围是否跨出档窗（决定是否需要 B 池补历史）。 */
  const needsPool = computed(() => {
    const anchor = deps.anchorDate()
    const range = observation.value
    if (!anchor || !range) return false
    if (deps.sourceDays.value === POOL_DAYS) return false
    return range.start < shiftDate(anchor, -(deps.sourceDays.value - 1))
  })

  async function ensurePool(options: { force?: boolean } = {}): Promise<void> {
    // 365 档即池：档/池共用 A 响应（OX-8），不发第二次请求
    if (deps.sourceDays.value === POOL_DAYS) return
    // 在途防重：手动重试也不叠发（避免重试风暴）
    if (poolLoading.value) return
    if (!options.force && (poolWindow.value !== null || poolError.value !== null)) return
    const seq = ++poolSeq
    poolLoading.value = true
    // 不清旧错/旧陈旧提示：成功才清，失败保留可读旧池并继续标陈旧（R29）
    const res = await deps.loadPool({ force: options.force })
    if (seq !== poolSeq) return
    poolLoading.value = false
    if (res.ok && res.payload) {
      poolWindow.value = res.payload.window
      poolError.value = null
      poolStaleNotice.value = false
    } else {
      poolError.value = res.error || '加载 365 日池失败'
      if (poolWindow.value !== null) poolStaleNotice.value = true
    }
  }

  /** B 单独重试（A 保持可读时使用；失败不占成功缓存，重试确实再发）。 */
  function retryPool(): Promise<void> {
    return ensurePool({ force: true })
  }

  // 观察跨出档窗 → 自动按需拉池（幂等：在途/已就绪/已失败未重试时不重复发）
  watch(needsPool, (needed) => {
    if (needed) void ensurePool()
  })

  // 池预取（首屏完整背景曲线）：顶部时间视窗图按 spec 是「365 池内已返回泳道的日线索合计」，
  // 池未取到时曲线只剩当前档窗那几天（用户视角=「首屏永远只有 14 天，拖一下才出历史」）。
  // 故 A 数据到位且锚点已知即拉池（design D11「要完整背景曲线可早调 ensurePool()」）；
  // 幂等由 ensurePool 自身守卫兜底（缓存命中/在途去重/失败不自动重发），不产生请求风暴。
  // 365 档即池（档/池共用同一响应）时 ensurePool 直接 no-op，不发第二次请求。
  watch(
    () => [deps.anchorDate(), deps.sourceWindow()] as const,
    ([anchor, window]) => {
      if (!anchor || !window) return
      void ensurePool()
    },
  )

  /** 档/池双索引：topicId(String) → date → 该日全部主题桶。 */
  const tierDayIndex = computed(() => buildDayIndex(deps.sourceWindow()))
  const poolDayIndex = computed(() => buildDayIndex(poolWindow.value))

  function buildDayIndex(window: LaneDynamicsWindowData | null): Map<string, Map<string, WindowLaneDay>> {
    const index = new Map<string, Map<string, WindowLaneDay>>()
    for (const lane of window?.lanes ?? []) {
      const days = new Map<string, WindowLaneDay>()
      for (const day of lane.timeline) days.set(day.date, day)
      index.set(lane.topicId, days)
    }
    return index
  }

  /**
   * 单日内容解析（G3/G4 消费）：
   *  - 档窗内（date ≥ anchor-days+1）：A 响应权威 → content / gap（断档）。
   *  - 档窗外（历史区段）：池未取到（未拉/失败/在途）→ unavailable（不补 0）；
   *    池就绪 → content（仅 A 名单同 ID 能查到行；B 独有 ID 不进名单，自然无行可渲染）。
   */
  function resolveLaneDay(topicId: string, date: string): LaneDayResolution {
    const anchor = deps.anchorDate()
    const tierWindow = deps.sourceWindow()
    if (anchor && tierWindow) {
      const tierStart = shiftDate(anchor, -(deps.sourceDays.value - 1))
      if (date >= tierStart) {
        const day = tierDayIndex.value.get(topicId)?.get(date)
        return day ? { kind: 'content', source: 'tier', day } : { kind: 'gap' }
      }
    }
    if (deps.sourceDays.value === POOL_DAYS) return { kind: 'unavailable' }
    if (poolWindow.value === null) return { kind: 'unavailable' }
    const day = poolDayIndex.value.get(topicId)?.get(date)
    return day ? { kind: 'content', source: 'pool', day } : { kind: 'gap' }
  }

  /** A 成功/切档后调用：范围为空时初始化该档默认区间，并做一次 URL start/end 恢复。 */
  function handleSourceLoaded(): void {
    const anchor = deps.anchorDate()
    if (!anchor) {
      observation.value = null
      return
    }
    if (observation.value === null) {
      observation.value = defaultRangeForTier(anchor, deps.sourceDays.value)
      if (suppressQueryRestore) {
        suppressQueryRestore = false
      } else {
        restoreRangeFromQuery()
      }
    }
  }

  function restoreRangeFromQuery(): void {
    const start = readQueryValue('start')
    const end = readQueryValue('end')
    if (start === undefined && end === undefined) return
    const bounds = poolBounds.value
    if (!bounds) return
    const result = validateObservationRange(start, end, bounds)
    if (result.ok) {
      observation.value = result.range
      // 深链恢复：URL 已反映该范围，与 committedRange 对齐（后续同值提交幂等）
      committedRange.value = result.range
      rangeTouched.value = true
      rangeNotice.value = null
    } else {
      // 非法项忽略 + 明确文案；原视窗保持（默认档区间不动，不静默截期/补 0）
      rangeNotice.value = result.issue.message
    }
  }

  /**
   * 应用观察范围（G2 入口）：严格校验，失败只给文案不改原范围/已提交范围。
   * 预览（writeQuery:false）：只更新显示，不写 URL 不动 committedRange；
   * 提交（默认）：与 URL 已反映范围（committedRange）不同才写 query——
   * 预览后同值松手恰写一次（R-22），提交后重复同值幂等不写。
   * 非法输入：observation 与 committedRange 均保持不变。
   */
  function applyObservationRange(start: string, end: string, options: { writeQuery?: boolean } = {}): boolean {
    const bounds = poolBounds.value
    if (!bounds) {
      rangeNotice.value = '数据尚未就绪，暂不能设置观察范围'
      return false
    }
    const result = validateObservationRange(start, end, bounds)
    if (!result.ok) {
      rangeNotice.value = result.issue.message
      return false
    }
    observation.value = result.range
    rangeNotice.value = null
    rangeTouched.value = true
    suppressQueryRestore = false
    if (options.writeQuery !== false && !sameRange(committedRange.value, result.range)) {
      writeRangeQuery(result.range)
      committedRange.value = result.range
    }
    return true
  }

  /**
   * 明确切档（useBoardConsole.setSourceDays 调用）：范围重置 + 目标档默认区间。
   * 用户按钮：写 days 并删除 start/end，且不回头读 router 尚未提交的旧 start/end；
   * 外部 URL days 变更：URL 已有 days，保留显式 start/end 供源加载后恢复。
   */
  function onTierSwitch(days: number, options: { writeQuery?: boolean, restoreQuery?: boolean } = {}): void {
    daysNotice.value = null
    rangeNotice.value = null
    observation.value = null
    committedRange.value = null
    rangeTouched.value = false
    suppressQueryRestore = options.restoreQuery !== true
    // 365 档即池（档/池共用同一响应，OX-8）：没有独立的池可失败 → 清掉池失败/陈旧态，
    // 否则提示条会误述「历史池未取到」且重试按钮变死键（review R-1）。
    if (days === POOL_DAYS) {
      poolError.value = null
      poolStaleNotice.value = false
    }
    if (options.writeQuery !== false) writeTierQuery(days)
  }

  /** 切板块清场（useBoardConsole watch(boardId) 调用）：范围/提示/池状态全部复位。 */
  function resetForBoard(): void {
    poolSeq += 1
    observation.value = null
    committedRange.value = null
    rangeTouched.value = false
    suppressQueryRestore = false
    rangeNotice.value = null
    daysNotice.value = null
    poolWindow.value = null
    poolLoading.value = false
    poolError.value = null
    poolStaleNotice.value = false
  }

  /**
   * G5：报告锚点元信息异步晚到后，若用户尚未显式设置范围且 URL 未带 start/end，
   * 用新锚点重算档默认区间。已拖选/输入/URL 显式范围一律不覆盖（D11 用户选择优先）。
   */
  function syncDefaultRangeToAnchor(): void {
    if (rangeTouched.value) return
    if (readQueryValue('start') !== undefined || readQueryValue('end') !== undefined) return
    const anchor = deps.anchorDate()
    if (!anchor) return
    observation.value = defaultRangeForTier(anchor, deps.sourceDays.value)
  }

  /** URL days 读取（初始化用）：合法档值 / null；非法时附原始串供提示。 */
  function readDaysQuery(): { days: number | null, invalidRaw: string | null } {
    const raw = readQueryValue('days')
    if (raw === undefined) return { days: null, invalidRaw: null }
    const days = parseTierDaysValue(raw)
    return days === null ? { days: null, invalidRaw: raw } : { days, invalidRaw: null }
  }

  // 外部 URL days 变更 = 明确切档（OX-7）；非法 → 保留当前合法档并告知实际档（R-23），不发非法请求
  watch(() => readQueryValue('days') ?? null, (raw) => {
    if (raw === null) return
    const days = parseTierDaysValue(raw)
    if (days === null) {
      daysNotice.value = `days 参数不合法（${raw}），保持 ${deps.sourceDays.value} 日档`
      return
    }
    daysNotice.value = null
    if (days !== deps.sourceDays.value) deps.onTierChange(days)
  })

  // 外部 URL start/end 变更：整体校验恢复；自身写入触发的同值回读幂等跳过
  watch(() => `${readQueryValue('start') ?? ''}~${readQueryValue('end') ?? ''}`, (next, prev) => {
    if (next === prev || next === '~') return
    restoreRangeFromQuery()
  })

  return {
    observation,
    committedRange,
    rangeNotice,
    daysNotice,
    poolBounds,
    poolWindow,
    poolLoading,
    poolError,
    poolStaleNotice,
    needsPool,
    ensurePool,
    retryPool,
    resolveLaneDay,
    handleSourceLoaded,
    syncDefaultRangeToAnchor,
    applyObservationRange,
    onTierSwitch,
    resetForBoard,
    readDaysQuery,
    /** 观察轴宽度（含两端；G3 可直接复用 buildDayAxis(end, width)）。 */
    observationWidth: computed(() => {
      const range = observation.value
      return range ? daysBetweenInclusive(range.start, range.end) : 0
    }),
  }
}
