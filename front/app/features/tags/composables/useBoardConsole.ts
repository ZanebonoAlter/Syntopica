/**
 * 版块态势控制台状态源（rework-workshop-into-radar-and-desk 1.2/1.3/1.4/2.1/2.2 + 矩阵阶段 G1）。
 *
 * 职责：
 *  - 拉取 `GET /semantic-boards/:id/lane-dynamics?days=N`（只读，复用既有聚合）。
 *  - 派生矩阵：日期轴（按 Asia/Shanghai 的今天回溯 N 天）、每条泳道逐日分区数、
 *    行尾合计、休眠分组（窗口内 ≤2 个分区 → 前端展示规则，不改话题状态）。
 *  - 选中泳道：`?lane=<topicId>` ↔ 状态双向同步；非法/缺失回退「窗口内线索数最多」的泳道。
 *  - 切版块清场：抽屉、休眠展开、视图选择全部复位（1.4）。
 *
 * 矩阵阶段 G1（观察窗 A 路线，design D11）：
 *  - A 通道 = 当前档（sourceDays）响应，是名单/行序/休眠/候选的唯一基准；
 *    请求经 laneDynamicsChannels（4 档有界缓存/在途去重/epoch 隔离）与
 *    laneDynamicsWindow façade（camelCase/字符串 ID 归一，legacy 原样携带）。
 *  - 观察范围/B 池/query 同步在 useObservationWindow（相邻状态机）；
 *    任何日期动作不切档，明确切档只经 setSourceDays（OX-1/OX-7）。
 *  - anchorDate = lane-dynamics 响应的 completed 锚点日（anchor_date）
 *    ?? 注入的报告锚点（deps.reportAnchor，壳层日报列表，G5 接线）
 *    ?? A 数据最新日；与 today 分清（0 泳道版块仍有日报锚点）。
 *
 * 纯函数全部导出（axis / row / dormant / 深链解析），组件只做渲染。
 */
import { computed, getCurrentScope, onScopeDispose, ref, watch, type Ref } from 'vue'
import type { LocationQuery } from 'vue-router'
import { useLaneDynamicsWindowApi } from '~/api/laneDynamicsWindow'
import type { LaneDynamicsWindowData } from '~/api/normalizers/laneDynamicsWindow'
import type { LaneDynamicsResponse } from '~/api/laneDynamics'
import { createLaneDynamicsChannelStore } from './laneDynamicsChannels'
import { useObservationWindow } from './useObservationWindow'
import { POOL_DAYS, daysBetweenInclusive } from '../components/console/observationRange'

// ── 纯模型层拆至 laneMatrixModel.ts（G1 拆层，公共导入路径不变） ──
export {
  MATRIX_WINDOW_OPTIONS,
  DEFAULT_WINDOW_DAYS,
  DORMANT_SECTION_THRESHOLD,
  todayInShanghai,
  buildDayAxis,
  buildLaneRow,
  buildLaneRows,
  splitDormantRows,
  parseLaneParam,
  resolveLaneSelection,
  normalizeWindowDays,
  parseBoardDeepLink,
  hasReportDeepLink,
} from './laneMatrixModel'
export type {
  ConsoleView,
  MatrixWindowKey,
  MatrixWindowOption,
  LaneMatrixRow,
  LegacyBoardDeepLink,
  ConsoleRouteLike,
  ConsoleRouterLike,
} from './laneMatrixModel'
import {
  DEFAULT_WINDOW_DAYS,
  buildDayAxis,
  buildLaneRows,
  normalizeWindowDays,
  parseLaneParam,
  resolveLaneSelection,
  splitDormantRows,
  todayInShanghai,
  type ConsoleRouteLike,
  type ConsoleRouterLike,
  type ConsoleView,
} from './laneMatrixModel'

export interface BoardConsoleDeps {
  /** 响应式 route getter（组件传 `() => route`；测试传静态对象）。 */
  route?: () => ConsoleRouteLike | undefined
  router?: () => ConsoleRouterLike | undefined
  /** 可注入的「现在」（测试固定 today）。 */
  now?: () => Date
  /** 可注入的报告锚点（壳层日报列表最新 period_date；G5 接线。OX-7：anchor 与 today/泳道日分清）。 */
  reportAnchor?: () => string | null | undefined
}

export function useBoardConsole(boardId: Ref<number>, deps: BoardConsoleDeps = {}) {
  const windowApi = useLaneDynamicsWindowApi()
  /** lane-dynamics 通道：4 档有界缓存 + 在途去重 + 板块 epoch（G1，P4 裁决）。 */
  const channelStore = createLaneDynamicsChannelStore((board, days) => windowApi.loadLaneDynamics(board, days))
  if (getCurrentScope()) onScopeDispose(() => channelStore.dispose())

  const data = ref<LaneDynamicsResponse | null>(null)
  /** A 通道归一数据（camelCase/字符串 ID；观察窗/池只读消费平面）。 */
  const sourceWindow = ref<LaneDynamicsWindowData | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)
  /** 刷新失败但保留旧数据 → 顶部刷新提示条（状态矩阵 error 行）。 */
  const staleNotice = ref(false)

  const windowDays = ref(DEFAULT_WINDOW_DAYS)
  const selectedLaneId = ref<number | null>(null)
  const expandedDormant = ref(false)
  const activeView = ref<ConsoleView>('matrix')

  /** A 通道 epoch：切档/切板块/刷新使旧响应作废（OX-10）。 */
  let sourceSeq = 0

  function queryLane(): unknown {
    return deps.route?.()?.query?.lane
  }

  function writeLaneQuery(topicId: number | null): void {
    const router = deps.router?.()
    const route = deps.route?.()
    if (!router || !route) return
    const query: LocationQuery = { ...route.query }
    if (topicId === null) {
      delete query.lane
    } else {
      query.lane = String(topicId)
    }
    router.replace({ query })
  }

  const today = computed(() => todayInShanghai(deps.now?.() ?? new Date()))

  /** 响应里最新的有分区日期（= 后端报告锚点候选）；早于今天时矩阵给出「数据截止」提示。 */
  const latestDataDate = computed<string | null>(() => {
    let latest = ''
    for (const lane of data.value?.lanes ?? []) {
      for (const day of lane.timeline ?? []) {
        if (day.date > latest) latest = day.date
      }
    }
    return latest || null
  })

  /** 数据锚点：注入的报告锚点优先（0 泳道版块仍有日报），否则取 A 数据最新日；与 today 分清。 */
  const reportAnchor = computed(() => {
    const raw = deps.reportAnchor?.()
    return typeof raw === 'string' && raw.trim() !== '' ? raw.trim().slice(0, 10) : null
  })
  /**
   * 窗口锚点（末日）：**后端下发的 completed 锚点日优先**——lane-dynamics 的
   * `anchor_date` = 该版块最新 completed 日报日，与泳道工作面（compose-candidates）
   * 的窗口末日同源同值（fix-lane-metric-provenance：两个窗口末日必须一致，不得
   * 相差一天）。
   *
   * 回退链：后端 anchor_date（显式字段，新后端恒有）→ 壳层注入的报告锚点（旧后端
   * / 0 泳道版块仍要有锚点）→ A 响应最新日。读的是**原始响应**的 anchor_date
   * （而非归一层的回退值），否则旧后端下「归一回退的最新日」会盖掉壳层注入锚点。
   */
  const sourceAnchorDate = computed<string | null>(() => {
    const raw = data.value?.anchor_date
    return typeof raw === 'string' && raw.trim() !== '' ? raw.trim().slice(0, 10) : null
  })
  const anchorDate = computed<string | null>(() =>
    sourceAnchorDate.value ?? reportAnchor.value ?? latestDataDate.value,
  )

  // 轴终点跟数据锚点一致：今天无日报时不再多画一个空「今天」列，
  // 同时保证 14 天矩阵与后端 14 天窗口逐日对齐。
  const axis = computed(() => buildDayAxis(anchorDate.value ?? today.value, windowDays.value))

  // ── 观察窗（G1）：范围/B 池/query 同步（useObservationWindow） ──
  const windowState = useObservationWindow({
    boardId,
    sourceDays: windowDays,
    anchorDate: () => anchorDate.value,
    sourceWindow: () => sourceWindow.value,
    loadPool: (options) => channelStore.load(boardId.value, POOL_DAYS, options),
    onTierChange: (days) => setSourceDays(days, { fromUrl: true }),
    route: deps.route,
    router: deps.router,
  })

  /** 观察轴（显示裁切）：范围内含两端逐日；未就绪时回退档全轴（G3 消费）。 */
  const visibleAxis = computed(() => {
    const range = windowState.observation.value
    if (!range) return axis.value
    return buildDayAxis(range.end, daysBetweenInclusive(range.start, range.end))
  })
  /** 档语义别名（G3+ 消费；与 windowDays 同源）。 */
  const sourceDays = computed(() => windowDays.value)

  const rows = computed(() => buildLaneRows(data.value?.lanes ?? [], axis.value))
  const activeRows = computed(() => splitDormantRows(rows.value).active)
  const dormantRows = computed(() => splitDormantRows(rows.value).dormant)
  const selectedLane = computed(
    () => rows.value.find((row) => row.topicId === selectedLaneId.value) ?? null,
  )
  const candidates = computed(() => data.value?.candidates ?? [])

  const emptyState = computed<'no-reports' | 'no-lanes' | null>(() => {
    if (!data.value) return null
    if (!data.value.has_reports) return 'no-reports'
    return data.value.lanes.length === 0 ? 'no-lanes' : null
  })

  /** 数据到位后校正选中泳道：query 优先，非法/缺失回退线索数最多者。 */
  function syncSelectionFromQuery(): void {
    const next = resolveLaneSelection(rows.value, queryLane())
    selectedLaneId.value = next
    ensureSelectedVisible()
  }

  /** 选中休眠泳道时自动展开休眠组（不静默丢失选择）。 */
  function ensureSelectedVisible(): void {
    if (selectedLaneId.value === null) return
    if (dormantRows.value.some((row) => row.topicId === selectedLaneId.value)) {
      expandedDormant.value = true
    }
  }

  /** A 通道取数：force=true 绕过缓存（刷新/重试语义）；force=false 命中缓存不重发（切档语义）。 */
  async function loadSource(options: { force: boolean }): Promise<void> {
    const seq = ++sourceSeq
    loading.value = true
    error.value = null
    staleNotice.value = false
    const res = await channelStore.load(boardId.value, windowDays.value, options)
    if (seq !== sourceSeq) return
    loading.value = false
    if (res.ok && res.payload) {
      data.value = res.payload.legacy
      sourceWindow.value = res.payload.window
      syncSelectionFromQuery()
      windowState.handleSourceLoaded()
    } else {
      error.value = res.error || '加载泳道动态失败'
      // 保留旧数据：顶部提示条，不清空已渲染内容（ui-design 状态矩阵 error 行）。
      if (data.value) staleNotice.value = true
    }
  }

  /** 刷新/重试入口（沿旧 load 语义：绕过缓存重新请求；在途仍去重）。 */
  async function load(): Promise<void> {
    await loadSource({ force: true })
  }

  /** 选中泳道（G3 字符串 topicId / legacy 数字均可；内部统一折算为 legacy 数字状态）。 */
  function selectLane(topicId: number | string, options: { writeQuery?: boolean } = {}): void {
    const numeric = typeof topicId === 'string' ? Number(topicId) : topicId
    if (!Number.isSafeInteger(numeric) || numeric <= 0) return
    if (!rows.value.some((row) => row.topicId === numeric)) return
    selectedLaneId.value = numeric
    ensureSelectedVisible()
    if (options.writeQuery !== false) writeLaneQuery(numeric)
  }

  function toggleDormant(): void {
    expandedDormant.value = !expandedDormant.value
  }

  /** 明确切档（OX-7：仅档按钮/URL days 走此路径；日期动作不触达）：重算名单与该档默认观察范围。 */
  function setSourceDays(days: number, options: { fromUrl?: boolean } = {}): void {
    const normalized = normalizeWindowDays(days)
    if (normalized === windowDays.value) return
    windowDays.value = normalized
    // 外部 URL days 变更时 URL 已带 days，保留（若有）显式 start/end；按钮/返回恢复则删旧范围。
    windowState.onTierSwitch(normalized, options.fromUrl
      ? { writeQuery: false, restoreQuery: true }
      : undefined)
    // 窗口变化后旧轴不再适用：清空旧数据走骨架，避免轴/格错配。
    data.value = null
    sourceWindow.value = null
    selectedLaneId.value = null
    void loadSource({ force: false })
  }

  /** 旧导出兼容（壳层 select 等未迁移消费者；G5 换接 setSourceDays）。 */
  function setWindow(days: number): void {
    setSourceDays(days)
  }

  function setView(view: ConsoleView): void {
    activeView.value = view
  }

  // 初始档：URL days 合法则直接用（首次请求即该档）；非法回默认并提示（变体 3：不发非法请求）
  const initialDays = windowState.readDaysQuery()
  if (initialDays.days !== null) {
    windowDays.value = initialDays.days
  } else if (initialDays.invalidRaw !== null) {
    windowState.daysNotice.value = `days 参数不合法（${initialDays.invalidRaw}），已回退 ${DEFAULT_WINDOW_DAYS} 日档`
  }

  watch(boardId, (_next, prev) => {
    if (prev !== undefined) {
      channelStore.invalidateBoard(prev)
      sourceSeq += 1
      data.value = null
      sourceWindow.value = null
      selectedLaneId.value = null
      expandedDormant.value = false
      activeView.value = 'matrix'
      windowDays.value = windowState.readDaysQuery().days ?? DEFAULT_WINDOW_DAYS
      windowState.resetForBoard()
    }
    void load()
  }, { immediate: true })

  // 外部（前进/后退、手动改地址）改 ?lane= 时同步；自身写入时值相同 → 幂等。
  watch(
    () => parseLaneParam(queryLane()),
    (requested) => {
      if (requested === null) {
        if (rows.value.length > 0 && selectedLaneId.value === null) syncSelectionFromQuery()
        return
      }
      if (rows.value.some((row) => row.topicId === requested)) {
        if (selectedLaneId.value !== requested) {
          selectedLaneId.value = requested
          ensureSelectedVisible()
        }
      } else if (requested !== selectedLaneId.value) {
        selectedLaneId.value = resolveLaneSelection(rows.value, requested)
      }
    },
  )

  return {
    data,
    sourceWindow,
    loading,
    error,
    staleNotice,
    windowDays,
    sourceDays,
    anchorDate,
    visibleAxis,
    today,
    axis,
    rows,
    activeRows,
    dormantRows,
    selectedLaneId,
    selectedLane,
    expandedDormant,
    activeView,
    candidates,
    latestDataDate,
    emptyState,
    // 观察窗（G1，G2–G5 消费）：范围/池/校验提示/单日内容解析/query 同步
    ...windowState,
    load,
    selectLane,
    toggleDormant,
    setSourceDays,
    setWindow,
    setView,
    writeLaneQuery,
  }
}
