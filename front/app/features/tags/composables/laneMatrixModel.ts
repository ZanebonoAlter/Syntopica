/**
 * 矩阵纯模型层（版块态势控制台 1.x + 矩阵阶段 G1 拆层）。
 *
 * 自 useBoardConsole.ts 拆出的纯函数/常量（公共导入路径不变：useBoardConsole
 * 原样 re-export；此处是唯一实现）。无 Vue/Router 依赖，组件与测试同构消费。
 */
import type { LocationQuery, Router } from 'vue-router'
import type { LaneDynamicsLane } from '~/api/laneDynamics'

export type ConsoleView = 'matrix' | 'archive'
export type MatrixWindowKey = '7d' | '14d' | '30d' | 'all'

export interface MatrixWindowOption {
  key: MatrixWindowKey
  /** lane-dynamics 的 days 参数（「全部」按 365 天封顶，避免无界拉取）。 */
  days: number
  label: string
}

export const MATRIX_WINDOW_OPTIONS: MatrixWindowOption[] = [
  { key: '7d', days: 7, label: '7 天' },
  { key: '14d', days: 14, label: '14 天' },
  { key: '30d', days: 30, label: '30 天' },
  { key: 'all', days: 365, label: '近 1 年' },
]

export const DEFAULT_WINDOW_DAYS = 14
/** 休眠阈值：窗口内分区数 ≤ 该值的活跃泳道折叠（spec：默认 2）。 */
export const DORMANT_SECTION_THRESHOLD = 2

const DAY_MS = 86_400_000

/** 今天（Asia/Shanghai 日历日，YYYY-MM-DD）——与后端日报期口径一致，不读浏览器时区。 */
export function todayInShanghai(now: Date = new Date()): string {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts(now)
  const value = (type: string) => parts.find((p) => p.type === type)?.value ?? ''
  return `${value('year')}-${value('month')}-${value('day')}`
}

/** 日期轴：以 endDate 为末日向前取 days 天（含首尾），升序数组。非法输入返回 []。 */
export function buildDayAxis(endDate: string, days: number): string[] {
  const end = Date.parse(`${endDate}T00:00:00Z`)
  if (!Number.isFinite(end)) return []
  const total = Math.max(1, Math.min(3650, Math.floor(days) || 1))
  const axis: string[] = []
  for (let i = total - 1; i >= 0; i -= 1) {
    axis.push(new Date(end - i * DAY_MS).toISOString().slice(0, 10))
  }
  return axis
}

export interface LaneMatrixRow {
  lane: LaneDynamicsLane
  topicId: number
  label: string
  watchLinked: boolean
  /** date → 当日 section 数（只在轴内的日期；矩阵格值与抽屉入口）。 */
  counts: Record<string, number>
  /** date → 当日线索数（热度口径，演进带节点大小/排序用）。 */
  clueCounts: Record<string, number>
  /** 轴内分区总数（行尾合计）。 */
  total: number
  /** 轴内线索总数（泳道热度排序口径）。 */
  clueTotal: number
  /** 轴内最后一次有分区的日期下标；-1 = 窗口内无分区。 */
  lastActiveIndex: number
}

/** 单条泳道在给定轴上的矩阵行（格值 = 当日 section 数；热度另存 clueCounts）。 */
export function buildLaneRow(lane: LaneDynamicsLane, axis: string[]): LaneMatrixRow {
  const axisSet = new Set(axis)
  const counts: Record<string, number> = {}
  const clueCounts: Record<string, number> = {}
  for (const day of lane.timeline ?? []) {
    if (!axisSet.has(day.date)) continue
    const sections = day.sections ?? []
    counts[day.date] = sections.length
    clueCounts[day.date] = sections.reduce((sum, section) => {
      const fallback = (section.events?.length ?? 0) + (section.folded_count ?? 0)
      return sum + (section.thread_count ?? fallback)
    }, 0)
  }
  let total = 0
  let clueTotal = 0
  let lastActiveIndex = -1
  axis.forEach((date, index) => {
    const count = counts[date] ?? 0
    total += count
    clueTotal += clueCounts[date] ?? 0
    if (count > 0) lastActiveIndex = index
  })
  return {
    lane,
    topicId: lane.topic_id,
    label: lane.label,
    watchLinked: lane.watch_linked,
    counts,
    clueCounts,
    total,
    clueTotal,
    lastActiveIndex,
  }
}

/** 全部泳道行：热度=轴内线索总数降序（分桶数只做次级），并列按 topic_id 升序。 */
export function buildLaneRows(lanes: LaneDynamicsLane[], axis: string[]): LaneMatrixRow[] {
  return lanes
    .map((lane) => buildLaneRow(lane, axis))
    .sort((a, b) => b.clueTotal - a.clueTotal || b.total - a.total || a.topicId - b.topicId)
}

/** 休眠分组：total ≤ threshold 归休眠（阈值含等号），其余留主矩阵。 */
export function splitDormantRows(
  rows: LaneMatrixRow[],
  threshold: number = DORMANT_SECTION_THRESHOLD,
): { active: LaneMatrixRow[], dormant: LaneMatrixRow[] } {
  const active: LaneMatrixRow[] = []
  const dormant: LaneMatrixRow[] = []
  for (const row of rows) {
    if (row.total <= threshold) dormant.push(row)
    else active.push(row)
  }
  return { active, dormant }
}

/** 解析 `?lane=`：正整数字符串 → topic id；其余（空串/空白/负数/非数字/数组）→ null。 */
export function parseLaneParam(raw: unknown): number | null {
  const value = Array.isArray(raw) ? raw[0] : raw
  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  if (!/^\d+$/.test(trimmed)) return null
  const id = Number(trimmed)
  return Number.isSafeInteger(id) && id > 0 ? id : null
}

/**
 * 选中泳道解析：
 *  ① query lane 命中现有泳道 → 用它；
 *  ② 否则取窗口内线索数最多的泳道（rows 已按 clueTotal 降序，并列 topic_id 升序）；
 *  ③ 无泳道 → null。
 */
export function resolveLaneSelection(rows: LaneMatrixRow[], rawLane: unknown): number | null {
  const requested = parseLaneParam(rawLane)
  if (requested !== null && rows.some((row) => row.topicId === requested)) return requested
  return rows.length > 0 ? rows[0]!.topicId : null
}

/** 窗口天数归一：只接受选项内值，其余回落默认 14。 */
export function normalizeWindowDays(raw: unknown): number {
  const value = typeof raw === 'string' ? Number(raw) : raw
  if (typeof value !== 'number' || !Number.isFinite(value)) return DEFAULT_WINDOW_DAYS
  const option = MATRIX_WINDOW_OPTIONS.find((item) => item.days === Math.floor(value))
  return option ? option.days : DEFAULT_WINDOW_DAYS
}

export interface LegacyBoardDeepLink {
  boardId: number
  /** 移除 `board` 后的其余 query（report/annotation 原样保留）。 */
  query: Record<string, unknown>
}

/**
 * 旧深链 `/tags?board=<id>&...` → 新版块路由参数。
 * 非法 board（缺失/空串/0/负数/非整数）返回 null（不重定向）。
 */
export function parseBoardDeepLink(query: Record<string, unknown> | undefined | null): LegacyBoardDeepLink | null {
  if (!query) return null
  const raw = Array.isArray(query.board) ? query.board[0] : query.board
  if (typeof raw !== 'string') return null
  const trimmed = raw.trim()
  if (!/^\d+$/.test(trimmed)) return null
  const boardId = Number(trimmed)
  if (!Number.isSafeInteger(boardId) || boardId <= 0) return null
  const rest: Record<string, unknown> = { ...query }
  delete rest.board
  return { boardId, query: rest }
}

/** 深链是否携带日报定位（阶段①保留旧页承载；阶段②映射到 `/tags/:boardId/:date`）。 */
export function hasReportDeepLink(query: Record<string, unknown> | undefined | null): boolean {
  if (!query) return false
  const raw = query.report
  const value = Array.isArray(raw) ? raw[0] : raw
  return value != null && String(value).trim() !== ''
}

export interface ConsoleRouteLike {
  query: LocationQuery
}

/** 只依赖 replace：组件传 `router`，测试传 `{ replace: vi.fn() }`。 */
export type ConsoleRouterLike = Pick<Router, 'replace'>
