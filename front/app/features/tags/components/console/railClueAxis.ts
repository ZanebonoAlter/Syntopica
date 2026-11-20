/**
 * 左轨逐日线索条码读模型（console-rails-live-signals design D2/D3/D4）。
 *
 * 纯函数：版块列表 × overview 行 → 左轨行视图模型。**不新增网络请求**——输入全部来自
 * 既有 `GET /semantic-boards`（行序 / tag_count）与 `GET /semantic-boards/overview`
 * （逐日线索轴 / 版块锚点日 / 全局锚点）。
 *
 * 共同轴（关键正确性要求，design D2）：后端 `radarAxis(anchor)` 按**每个版块自己的
 * 锚点日**生成 14 天窗口（`backend-go/internal/topicgraph/repository/board_radar_repository.go`），
 * 13 行的「最后一格」各自代表不同日期，直接并列即假比较。本模块按**全局锚点日**
 * （overview 响应 `date`）把每行重对齐到同一条 14 天轴 `[date-13 … date]`：
 *
 *   offset  = daysBetween(globalDate, board.anchor_date)   // 该版块落后全局多少天
 *   位置     = i - offset                                   // board.clue_days[i] 在共同轴上的位置
 *
 * 位置落在 `[0,14)` 之外 = **未取到**（null → 斜纹占位），MUST NOT 补 0。于是
 * 「停更」（末段斜纹）与「安静」（真实 0 → 空槽）在视觉上天然分开，无需额外文字标注。
 *
 * 行序保持 `GET /semantic-boards` 的返回序（display_order），MUST NOT 因活跃度重排；
 * 活跃度差异只用**行整体强度**（opacity 0.42–1，按 14 天总量分档）表达。
 * 颜色归一复用 `observationLanes.ts` 的 `clueDepth` / `clueMix`（行内峰值归一，
 * 与矩阵日格、雷达小矩阵同一套色阶），本模块不引入第二套色阶。
 */
import type { RadarBoard } from '~/api/normalizers/boardRadar'
import type { SemanticBoard } from '~/api/semanticBoards'
import { clueDepth, clueMix } from './observationLanes'
import { daysBetweenInclusive, shiftDate } from './observationRange'

/** 共同轴固定 14 天（不随顶部观察档位变化——跨版块可比性依赖固定刻度）。 */
export const RAIL_AXIS_DAYS = 14

/** 行整体强度下限（安静行压暗到这个比例；上限 1 = 最活跃行不压暗）。 */
export const RAIL_INTENSITY_MIN = 0.42
export const RAIL_INTENSITY_MAX = 1

/** 左轨一行视图模型（组件只渲染，不自己算口径）。 */
export interface RailBoardRow {
  boardId: number
  label: string
  /** 共同轴 14 格（oldest→newest）；null = 该位未取到（斜纹占位）。 */
  cells: (number | null)[]
  /** 行内峰值（色阶归一分母）；0 = 整行无已知值。 */
  peak: number
  /** 行整体强度系数（0.42–1）：按 14 天总量分档压暗安静行。 */
  intensity: number
  /** 标签挂载数（治理数据；只进 title，不占行内数字位）。 */
  tagCount: number | null
  /** 该版块自身锚点日（滞后标记；只进 title / 可访问文本）。 */
  anchorDate: string | null
}

function isDateString(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false
  const ms = Date.parse(`${value}T00:00:00Z`)
  return Number.isFinite(ms) && new Date(ms).toISOString().slice(0, 10) === value
}

/** 相差天数（later - earlier）；日期非法 → null（不猜、不补 0）。 */
function dayOffset(later: string, earlier: string): number | null {
  if (!isDateString(later) || !isDateString(earlier)) return null
  return daysBetweenInclusive(earlier, later) - 1
}

/** 共同轴日期（oldest→newest，14 天）；全局锚点非法 → 空数组（不造日期）。 */
export function railAxisDates(globalDate: string): string[] {
  if (!isDateString(globalDate)) return []
  return Array.from({ length: RAIL_AXIS_DAYS }, (_, index) => shiftDate(globalDate, index - (RAIL_AXIS_DAYS - 1)))
}

/** 行内已知格合计（未取到位不计）。 */
export function railRowTotal(cells: readonly (number | null)[]): number {
  let total = 0
  for (const cell of cells) if (typeof cell === 'number' && Number.isFinite(cell)) total += cell
  return total
}

/** 行整体强度：0.42 + 0.58 × (行总量 / 全库最大行总量)；形状不变，只压暗安静行。 */
export function railRowIntensity(total: number, maxTotal: number): number {
  const ratio = maxTotal > 0 ? total / maxTotal : 0
  const bounded = ratio < 0 ? 0 : ratio > 1 ? 1 : ratio
  return RAIL_INTENSITY_MIN + (RAIL_INTENSITY_MAX - RAIL_INTENSITY_MIN) * bounded
}

/** 单格色阶混入比例（行内峰值归一；复用矩阵同一套 `clueDepth` / `clueMix`）。 */
export function railCellMix(cell: number | null, peak: number): string {
  if (cell === null) return ''
  return clueMix(clueDepth(cell, peak))
}

function buildRow(board: SemanticBoard, overview: RadarBoard | undefined, globalDate: string): RailBoardRow {
  const cells: (number | null)[] = Array.from({ length: RAIL_AXIS_DAYS }, () => null)
  const clueDays = overview?.clueDays ?? null
  const anchorDate = overview?.anchorDate ?? null
  /**
   * 未取到 / 不可用 = 任一成立：
   *  - 本次没取到该版块（`dataUnavailable`）；
   *  - 后端没下发 `clue_days` 或空轴；
   *  - **轴长不等于 14**：共同轴对齐依赖「下标 ↔ 日期」严格对应，长度不符一律不猜（review R-7：
   *    归一化层若把非法项 filter 掉，下标会整体前移、静默错日期还显示成有值）；
   *  - **缺 `anchor_date`**：无法确定 `clue_days[i]` 属于哪天，就不能断言它落在共同轴哪一格
   *    （review R-6：此前兜底 offset=0 等于静默把值贴到错误日期）。
   *    后端当前只在「无泳道锚定 → 全 0 轴」时出现该组合，但那是后端不变量，前端不得依赖。
   */
  const usable = overview !== undefined
    && overview.dataUnavailable === false
    && clueDays !== null
    && clueDays.length === RAIL_AXIS_DAYS
    && anchorDate !== null
  if (usable && clueDays && anchorDate) {
    const offset = dayOffset(globalDate, anchorDate)
    if (offset !== null) {
      for (let index = 0; index < clueDays.length; index++) {
        const position = index - offset
        if (position < 0 || position >= RAIL_AXIS_DAYS) continue
        const value = clueDays[index]
        cells[position] = typeof value === 'number' && Number.isFinite(value) ? value : null
      }
    }
  }
  let peak = 0
  for (const cell of cells) {
    if (typeof cell === 'number' && Number.isFinite(cell) && cell > peak) peak = cell
  }
  return {
    boardId: board.id,
    label: board.label,
    cells,
    peak,
    intensity: RAIL_INTENSITY_MIN,
    tagCount: typeof board.tag_count === 'number' ? board.tag_count : null,
    anchorDate,
  }
}

/**
 * 组装左轨行：按 `boards` 顺序 join overview 行（**不**按 overview 的关注度序），
 * 共同轴重对齐 + 行内峰值 + 跨行强度分档。
 */
export function buildRailBoardRows(
  boards: readonly SemanticBoard[],
  overviewRows: readonly RadarBoard[],
  globalDate: string,
): RailBoardRow[] {
  const byBoardId = new Map<string, RadarBoard>()
  for (const row of overviewRows) byBoardId.set(row.boardId, row)

  const rows = boards.map(board => buildRow(board, byBoardId.get(String(board.id)), globalDate))
  let maxTotal = 0
  for (const row of rows) {
    const total = railRowTotal(row.cells)
    if (total > maxTotal) maxTotal = total
  }
  return rows.map(row => ({ ...row, intensity: railRowIntensity(railRowTotal(row.cells), maxTotal) }))
}

/**
 * 行 title / aria：名称 + 标签挂载数 + 版块锚点日 + 逐日明细。
 * 治理数据退到 title（行内不占数字位）；未取到的位置写「未取到」而不是 0。
 */
export function railRowTitle(row: RailBoardRow, dates: readonly string[]): string {
  const parts: string[] = [row.label]
  if (row.tagCount !== null) parts.push(`标签挂载 ${row.tagCount}`)
  // 无锚点 ≠ 本次未取到（review R-5）：前者是「该版块没有日报锚点」，后者是「这次没取到」。
  if (row.anchorDate !== null) parts.push(`数据截至 ${row.anchorDate}`)
  else parts.push('该版块无日报锚点')
  const detail = row.cells.map((cell, index) => {
    const date = dates[index] ?? `第 ${index + 1} 天`
    return `${date.slice(5)}：${cell === null ? '未取到' : cell}`
  })
  parts.push(detail.join(' · '))
  return parts.join(' · ')
}
