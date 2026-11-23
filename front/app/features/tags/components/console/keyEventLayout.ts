/**
 * 关键事件视图布局纯模型（add-key-event-matrix-view design D4/D10/D11）。
 *
 * 三个纯函数，组件只做测量与渲染（可测的逻辑与不可测的 DOM 分离）：
 *  - `computeEventColumnWidth`：列宽 = max(64, 可用宽 / 可见日数)——超宽屏均分填满、
 *    窄屏守 64 下限后由舞台容器横向滚动承接（MUST NOT 压缩列宽塞进一屏）；
 *  - `layoutEventLabels`：标签两阶段布局的第二阶段——用 `offsetWidth` 实测宽度做
 *    贪心分层（按日期升序；每层记录已占右边界，放得下就复用、放不下新开一层；
 *    同层相邻标签水平间隙 ≥ 4px；靠近右边界左移贴边；不截断标题）；
 *  - `keyEventRowHeight`：行高 = 无事件基础行高 70px，有标签按层数增高（每层 +44px，
 *    封顶 246px）；
 *  - `keyEventLabelsCollapsed`：收起判据只看**该行标签层数是否超上限**（4 层），
 *    与观察窗天数无关——天数多只是列更宽更稀，不是更挤（design D7）；
 *  - `computeKeyEventRequestDays`：事件批量读的请求窗口天数（MUST NOT 封顶 365，
 *    见函数注释「为什么不能封顶 365」）。
 *
 * 节点（node）始终落在生命线（轴线）上：同日多条事件靠**横向散开**区分（MUST NOT
 * 竖向压到别的层的标签上），标签在各层里向右展开（层顶 46px、层距 44px）。
 */
import type { BoardKeyEventItem } from '~/api/boardKeyEvents'
import { daysBetweenInclusive } from './observationRange'

/** 列宽下限（D10）：低于它标签无处可放，宁可撑宽画布横向滚动。 */
export const KEY_EVENT_MIN_COLUMN_WIDTH = 64

/** 标签距节点右缘的起始偏移（节点半径 + 视觉间隙）。 */
export const KEY_EVENT_LABEL_X_OFFSET = 9

/** 同层相邻标签最小水平间隙。 */
export const KEY_EVENT_LABEL_GAP = 4

/** 层距：层与层的纵向步进（含标签高 ~24px 与层间空隙）。 */
export const KEY_EVENT_LAYER_STEP = 44

/** 第一层标签 top（轴心下方）。 */
export const KEY_EVENT_LABEL_TOP = 46

/** 节点中心 y：与轴线同高（轴线 top 18px、节点高 18px → 中心 18）。 */
export const KEY_EVENT_NODE_CENTER_Y = 18

/** 节点上沿（= 中心 − 半径 9）：节点骑在轴线上。 */
export const KEY_EVENT_NODE_TOP = KEY_EVENT_NODE_CENTER_Y - 9

/** 同日多节点的横向散开步长（px；节点恒在轴线上，只能横向错开）。 */
export const KEY_EVENT_NODE_FAN_STEP = 12

/** 无事件行的基础行高（行头：话题名 2 行 + 事件计数）。 */
export const KEY_EVENT_BASE_ROW_HEIGHT = 70

/** 标签层数上限（design D7）：超过它该行才退化为「节点 + 当日事件计数」。 */
export const KEY_EVENT_MAX_LABEL_LAYERS = 4

/** 行高封顶（= 4 层标签的视觉上限，Layout Contract）：超限行只配一个封顶高度。 */
export const KEY_EVENT_MAX_ROW_HEIGHT = 246

/** 列宽：理想均分，但不得低于下限（下限撑宽画布，由容器横向滚动承接）。 */
export function computeEventColumnWidth(available: number, dayCount: number): number {
  const days = Math.max(1, Math.floor(dayCount) || 1)
  if (!Number.isFinite(available) || available <= 0) return KEY_EVENT_MIN_COLUMN_WIDTH
  return Math.max(KEY_EVENT_MIN_COLUMN_WIDTH, available / days)
}

/**
 * 关键事件批量读的请求窗口天数：覆盖**可见轴起点**。
 *
 * **为什么不能封顶 365**（review R-4，2026-11-23）：后端读端 `days` 的窗口是
 * `[今天−(days−1), 今天]`，而可见轴的左沿受**池边界**约束（`[anchor−364, anchor]`，
 * anchor = 该版块最新 completed 日报日，通常**落后今天** N 天）。此时
 * `axis[0] = anchor−364 = 今天−364−N`，比后端 365 天窗口的起点还早 N 天 →
 * 若把请求天数封顶 365，轴最左 N 天的 confirmed 事件**不在响应里**，视图把它们
 * 渲染成「无确认事件」——静默少事件、无任何提示（轴上凭空少节点）。
 *
 * 池的 365 是**泳道数据**的取数口径，与关键事件读端无关：后端 `days` 接受任意
 * 正整数，故请求窗口按轴跨度如实取，MUST NOT 与池对齐。
 *
 * 为什么不无限大：轴左沿被池边界钳住（`axis[0] ≥ anchor−364`），`anchor ≤ today`，
 * 故 span ≤ 365 + (today − anchor)，是有界的（最坏 = 池跨度 + 日报滞后天数）。
 *
 * @param axis 可见观察轴（升序 date-only；空数组 = 还没有轴）
 * @param today 今天（date-only）
 * @param sourceDays 当前取数档天数（7/14/30/365；下限用）
 */
export function computeKeyEventRequestDays(axis: readonly string[], today: string, sourceDays: number): number {
  const floor = Math.max(1, Math.floor(sourceDays) || 1)
  if (axis.length === 0) return floor
  const span = daysBetweenInclusive(axis[0]!, today)
  return Math.max(1, Math.max(floor, span))
}

/** 待布局标签（第一阶段产物：节点 x + 实测自然宽度）。 */
export interface KeyEventLabelBox {
  /** 事件标识（布局结果回填键）。 */
  key: string
  /** 节点中心 x（列内像素坐标，行体坐标系）。 */
  x: number
  /** 标签自然宽度（offsetWidth 实测；happy-dom 等无布局环境为 0）。 */
  width: number
}

/** 布局结果：绝对定位（行体坐标系）。 */
export interface KeyEventLabelPlacement {
  key: string
  left: number
  top: number
  /** 所在层序（0 起）。 */
  layer: number
}

/**
 * 贪心分层（D4）：按节点 x 升序逐条放置；每层记录已占右边界，标签左缘 ≥ 层右边界
 * + 4px 才复用该层，否则新开一层。靠近右边界（left + width > containerWidth）时
 * 左移贴边（left = max(0, containerWidth - width)），贴边后同样参与层占用判定。
 */
export function layoutEventLabels(
  items: readonly KeyEventLabelBox[],
  containerWidth: number,
): KeyEventLabelPlacement[] {
  const layerRights: number[] = []
  const out: KeyEventLabelPlacement[] = []
  const sorted = [...items].sort((a, b) => a.x - b.x || (a.key < b.key ? -1 : a.key > b.key ? 1 : 0))
  for (const item of sorted) {
    const width = Math.max(0, item.width)
    let left = item.x + KEY_EVENT_LABEL_X_OFFSET
    if (containerWidth > 0 && left + width > containerWidth) {
      left = Math.max(0, containerWidth - width)
    }
    let layer = layerRights.findIndex(right => left >= right + KEY_EVENT_LABEL_GAP)
    if (layer === -1) {
      layerRights.push(left + width)
      layer = layerRights.length - 1
    } else {
      layerRights[layer] = left + width
    }
    out.push({ key: item.key, left, top: KEY_EVENT_LABEL_TOP + layer * KEY_EVENT_LAYER_STEP, layer })
  }
  return out
}

/**
 * 行高：无标签走基础行高 70px；有标签按层数增高（每层 +44px），封顶 246px。
 * 层顶 46px 起、每层 44px，故 L 层需要 46 + 44L 的高度（下限取基础行高）。
 */
export function keyEventRowHeight(layerCount: number): number {
  if (layerCount <= 0) return KEY_EVENT_BASE_ROW_HEIGHT
  const grown = KEY_EVENT_LABEL_TOP + layerCount * KEY_EVENT_LAYER_STEP
  return Math.min(KEY_EVENT_MAX_ROW_HEIGHT, Math.max(KEY_EVENT_BASE_ROW_HEIGHT, grown))
}

/**
 * 该行是否收起标签（design D7）：**只看这一行的层数**，与观察窗天数无关。
 * 层数 ≤ 4 照常显示事件名称（哪怕 55 / 365 天）；超上限才退化为节点 + 当日计数，
 * 名称由下方选中日阅读区承担。
 */
export function keyEventLabelsCollapsed(layerCount: number): boolean {
  return layerCount > KEY_EVENT_MAX_LABEL_LAYERS
}

/**
 * 同日多节点的横向散开偏移：第 i 个（共 count 个）相对列中心的偏移
 * =(i − (count−1)/2) × step。count ≤ 1 → 不偏移。
 * 节点恒在轴线上（MUST NOT 竖向下压到别层的标签上），同日多条靠横向错开区分。
 */
export function nodeFanOffset(index: number, count: number, step: number = KEY_EVENT_NODE_FAN_STEP): number {
  if (count <= 1) return 0
  return (index - (count - 1) / 2) * step
}

/** 行内标签层数（布局结果派生）。 */
export function placementLayerCount(placements: readonly KeyEventLabelPlacement[]): number {
  let max = -1
  for (const p of placements) if (p.layer > max) max = p.layer
  return max + 1
}

/** 按日聚合的单行视图模型（行体渲染入口）：axis 内逐日事件（窗外事件过滤）。 */
export interface KeyEventDayGroup {
  date: string
  events: BoardKeyEventItem[]
}

/** 一行的逐日事件投影：只保留 axis 内的日期（窗外事件不渲染、不计数）。 */
export function projectKeyEventDays(
  eventsByDate: ReadonlyMap<string, BoardKeyEventItem[]>,
  axis: readonly string[],
): KeyEventDayGroup[] {
  const groups: KeyEventDayGroup[] = []
  for (const date of axis) {
    const events = eventsByDate.get(date)
    groups.push({ date, events: events ? [...events] : [] })
  }
  return groups
}
