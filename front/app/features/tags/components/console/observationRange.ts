/**
 * 观察窗纯函数（rework-workshop-into-radar-and-desk 矩阵阶段 G1，design D11「观察窗 A 路线」）。
 *
 * 职责边界：
 *  - 日期严格校验（YYYY-MM-DD 真实日历日）、池边界钳制、平移/缩放/键盘端点调整、
 *    档默认区间派生、URL days 档值解析。
 *  - **本模块没有任何「从日期推导档位」的 API**：观察范围无论恰好 7/14/30/365 日，
 *    sourceDays 只能经 useBoardConsole 的明确切档动作变更（OX-1）。
 *  - 日期为 date-only 字符串，全部按 UTC 数值日做算术（与 Asia/Shanghai 日历日一致，
 *    不读宿主时区）；错误不静默改期、不补 0，由调用方保留原视窗并展示 issue 文案。
 */
export const SOURCE_TIER_DAYS = [7, 14, 30, 365] as const
export type SourceTierDays = 7 | 14 | 30 | 365
export const DEFAULT_SOURCE_DAYS = 14
/** 365 日池档（B 通道与 365 档共用同一次请求，OX-8）。 */
export const POOL_DAYS = 365

export interface ObservationRange {
  start: string
  end: string
}

/** 真实可取数边界（含两端）：[池首, 池尾]。 */
export interface DateBounds {
  start: string
  end: string
}

export type RangeIssueCode = 'missing' | 'format' | 'order' | 'out-of-pool'

export interface RangeIssue {
  code: RangeIssueCode
  message: string
}

export type ObservationRangeResult
  = { ok: true, range: ObservationRange }
  | { ok: false, issue: RangeIssue }

const DAY_MS = 86_400_000

/** 严格日期串：四位年-两位月-两位日 + 真实日历日（2026-02-30 拒绝）。 */
export function parseStrictDate(raw: string): string | null {
  if (typeof raw !== 'string') return null
  const trimmed = raw.trim()
  if (!/^\d{4}-\d{2}-\d{2}$/.test(trimmed)) return null
  const ms = Date.parse(`${trimmed}T00:00:00Z`)
  if (!Number.isFinite(ms)) return null
  return new Date(ms).toISOString().slice(0, 10) === trimmed ? trimmed : null
}

function dateToIndex(date: string): number {
  return Math.round(Date.parse(`${date}T00:00:00Z`) / DAY_MS)
}

function indexToDate(index: number): string {
  return new Date(index * DAY_MS).toISOString().slice(0, 10)
}

/** 日期平移（delta 可负）；调用方保证入参合法。 */
export function shiftDate(date: string, deltaDays: number): string {
  return indexToDate(dateToIndex(date) + deltaDays)
}

/** 含两端的天数宽度（end - start + 1）。 */
export function daysBetweenInclusive(start: string, end: string): number {
  return dateToIndex(end) - dateToIndex(start) + 1
}

/** 档默认观察区间：[anchor-days+1, anchor]（含两端；days ≥ 1）。 */
export function defaultRangeForTier(anchor: string, days: number): ObservationRange {
  const width = Math.max(1, Math.floor(days) || 1)
  return { start: shiftDate(anchor, -(width - 1)), end: anchor }
}

export function sameRange(a: ObservationRange | null | undefined, b: ObservationRange | null | undefined): boolean {
  return !!a && !!b && a.start === b.start && a.end === b.end
}

/**
 * 校验观察范围输入（OX-3 四类：缺项 / 格式 / 顺序 / 池外）。
 * 空串与纯空白视为缺项；任一类失败即整体失败（范围是不可拆分的一个状态）。
 */
export function validateObservationRange(
  startRaw: unknown,
  endRaw: unknown,
  bounds: DateBounds,
): ObservationRangeResult {
  const startValue = typeof startRaw === 'string' ? startRaw.trim() : ''
  const endValue = typeof endRaw === 'string' ? endRaw.trim() : ''
  if (!startValue || !endValue) {
    return { ok: false, issue: { code: 'missing', message: '请填写起止日期' } }
  }
  const start = parseStrictDate(startValue)
  const end = parseStrictDate(endValue)
  if (!start || !end) {
    return { ok: false, issue: { code: 'format', message: '日期须为 YYYY-MM-DD 格式（如 2026-10-06）' } }
  }
  if (start > end) {
    return { ok: false, issue: { code: 'order', message: '起始日期不能晚于截止日期' } }
  }
  if (start < bounds.start || end > bounds.end) {
    return {
      ok: false,
      issue: { code: 'out-of-pool', message: `日期超出可取数范围（${bounds.start} ~ ${bounds.end}）` },
    }
  }
  return { ok: true, range: { start, end } }
}

/** 把范围钳制进边界（保宽度；宽度超池时取整池）。 */
export function clampRangeToBounds(range: ObservationRange, bounds: DateBounds): ObservationRange {
  const width = Math.min(daysBetweenInclusive(range.start, range.end), daysBetweenInclusive(bounds.start, bounds.end))
  let startIndex = Math.max(dateToIndex(range.start), dateToIndex(bounds.start))
  const overflow = startIndex + width - 1 - dateToIndex(bounds.end)
  if (overflow > 0) startIndex -= overflow
  return { start: indexToDate(startIndex), end: indexToDate(startIndex + width - 1) }
}

/** 视窗平移（OX-5）：整体位移 delta 日，钳制在池内、宽度保持 ≥1。 */
export function shiftRange(range: ObservationRange, deltaDays: number, bounds: DateBounds): ObservationRange {
  return clampRangeToBounds(
    { start: shiftDate(range.start, deltaDays), end: shiftDate(range.end, deltaDays) },
    bounds,
  )
}

/** 视窗缩放（OX-5）：宽度 ×factor（0.5/2 等），围绕中心，钳制 [1, 池宽]。 */
export function zoomRange(range: ObservationRange, factor: number, bounds: DateBounds): ObservationRange {
  const width = daysBetweenInclusive(range.start, range.end)
  const maxWidth = daysBetweenInclusive(bounds.start, bounds.end)
  const target = Math.max(1, Math.min(Math.round(width * factor), maxWidth))
  const center = Math.round((dateToIndex(range.start) + dateToIndex(range.end)) / 2)
  const startIndex = center - Math.floor((target - 1) / 2)
  return clampRangeToBounds({ start: indexToDate(startIndex), end: indexToDate(startIndex + target - 1) }, bounds)
}

/**
 * 键盘端点逐日/逐周调整（OX-6）：只动一端，钳制在池内且不越过对端。
 * delta 为负表示向历史方向（← / Shift+←）。
 */
export function stepRangeEdge(
  range: ObservationRange,
  edge: 'start' | 'end',
  deltaDays: number,
  bounds: DateBounds,
): ObservationRange {
  if (edge === 'start') {
    const next = Math.min(Math.max(dateToIndex(range.start) + deltaDays, dateToIndex(bounds.start)), dateToIndex(range.end))
    return { start: indexToDate(next), end: range.end }
  }
  const next = Math.max(Math.min(dateToIndex(range.end) + deltaDays, dateToIndex(bounds.end)), dateToIndex(range.start))
  return { start: range.start, end: indexToDate(next) }
}

/** 键盘 Home/End（OX-6）：把聚焦端送到池首/池尾，不越过对端。 */
export function edgeToBound(
  range: ObservationRange,
  edge: 'start' | 'end',
  position: 'start' | 'end',
  bounds: DateBounds,
): ObservationRange {
  const delta = dateToIndex(bounds[position]) - dateToIndex(range[edge])
  return stepRangeEdge(range, edge, delta, bounds)
}

/** URL days 档值解析：仅接受 7/14/30/365 的纯数字串，其余（含 8/abc/0/-1）为 null。 */
export function parseTierDaysValue(raw: string): SourceTierDays | null {
  const value = typeof raw === 'string' ? raw.trim() : ''
  if (!/^\d+$/.test(value)) return null
  const days = Number(value)
  return (SOURCE_TIER_DAYS as readonly number[]).includes(days) ? days as SourceTierDays : null
}
