/**
 * 关键事件视图布局纯模型（add-key-event-matrix-view design D4/D10/D11；
 * key-event-day-focus-cards design D1/D3/D6/D13/D14）。
 *
 * 组件只做测量与渲染（可测的逻辑与不可测的 DOM 分离）：
 *  - `computeEventColumnWidth`：列宽 = max(64, 可用宽 / 可见日数)——超宽屏均分填满、
 *    窄屏守 64 下限后由舞台容器横向滚动承接（MUST NOT 压缩列宽塞进一屏）；
 *  - `layoutKeyEventBands`：**双带**归带与分层（key-event-day-focus-cards）——按日期
 *    升序逐组在上/下带各算代价，取代价小的一侧（并列取下带）；下带组与卡片矩形
 *    横向相交时改走上带（卡片是下带的固定障碍区）；同日多条整组放不下时改摆到圆点**左侧**
 *    （组内引线 MUST NOT 穿过组内标签，见 R-15）；两带都找不到可用连续层时显式
 *    标记该行溢出（`overflowed`），MUST NOT 把未校验的层当可用（R-16）；
 *  - `keyEventLeaderPath`：引线折点（竖段自圆点外缘走到标签近缘；圆点 x 落在标签
 *    横向范围内时只画竖段，否则补一段横段接入标签近端）；
 *  - `keyEventDayCardRect`：日聚焦卡片矩形（左缘 = 选中列左缘，靠右放不下时右缘
 *    对齐选中列右缘向左展开；行体窄于卡片宽时钳制在 [0, 行体宽] 内）；
 *  - `keyEventRowHeight` / `keyEventRowHeightWithCards`：标签行高 =
 *    min(246, max(70, 50 + 32 × (上带层数 + 下带层数)))；卡片一侧**不受 246 封顶**，
 *    最终行高取两者较大值；
 *  - `keyEventLabelsCollapsed`：收起判据只看**该行标签层数是否超上限**（4 层，口径 =
 *    上带 + 下带合计），与观察窗天数无关——天数多只是列更宽更稀，不是更挤（design D7/D14）；
 *  - `computeKeyEventRequestDays`：事件批量读的请求窗口天数（MUST NOT 封顶 365，
 *    见函数注释「为什么不能封顶 365」）。
 *
 * 节点（node）始终落在生命线（轴线）上：同日多条事件靠**横向散开**区分（MUST NOT
 * 竖向压到别的层的标签上）。
 *
 * 旧单侧贪心 `layoutEventLabels`（层顶 46px、层距 44px）仍在 `KeyEventTimelineView.vue`
 * 里挂着，等渲染模块切到双带布局后才随之下线。
 */
import type { BoardKeyEventItem } from '~/api/boardKeyEvents'
import { daysBetweenInclusive } from './observationRange'

/** 列宽下限（D10）：低于它标签无处可放，宁可撑宽画布横向滚动。 */
export const KEY_EVENT_MIN_COLUMN_WIDTH = 64

/** 标签距节点右缘的起始偏移（节点半径 + 视觉间隙）。 */
export const KEY_EVENT_LABEL_X_OFFSET = 9

/** 同层相邻标签最小水平间隙。 */
export const KEY_EVENT_LABEL_GAP = 4

/** 层距：层与层的纵向步进（含标签高 ~24px 与层间空隙）。旧单侧贪心布局在用。 */
export const KEY_EVENT_LAYER_STEP = 44

/** 第一层标签 top（轴心下方）。 */
export const KEY_EVENT_LABEL_TOP = 46

/** 节点中心 y：与轴线同高（轴线 top 18px、节点高 18px → 中心 18）。 */
export const KEY_EVENT_NODE_CENTER_Y = 18

/** 节点上沿（= 中心 − 半径 9）：节点骑在轴线上。 */
export const KEY_EVENT_NODE_TOP = KEY_EVENT_NODE_CENTER_Y - 9

/**
 * 同日多节点的横向散开步长（px；节点恒在轴线上，只能横向错开）。
 *
 * 24px 而非 12px：18px 圆点配 12px 步长会让相邻圆点互相压盖 6px（design D13）。
 */
export const KEY_EVENT_NODE_FAN_STEP = 24

/** 无事件行的基础行高（行头：话题名 2 行 + 事件计数）。 */
export const KEY_EVENT_BASE_ROW_HEIGHT = 70

/** 标签层数上限（design D7）：超过它该行才退化为「节点 + 当日事件计数」。 */
export const KEY_EVENT_MAX_LABEL_LAYERS = 4

/** 行高封顶（= 4 层标签的视觉上限，Layout Contract）：超限行只配一个封顶高度。 */
export const KEY_EVENT_MAX_ROW_HEIGHT = 246

/** 标签高（药丸）。 */
export const KEY_EVENT_LABEL_HEIGHT = 24

/** 双带布局的层距（带内相邻两层；双带下层距是 32px 而非旧单侧的 44px）。 */
export const KEY_EVENT_BAND_LAYER_STEP = 32

/** 带间留白（上带 ↔ 轴带、轴带 ↔ 下带）。 */
export const KEY_EVENT_BAND_PADDING = 8

/** 轴带高（生命线居中、圆点骑线）。 */
export const KEY_EVENT_AXIS_BAND_HEIGHT = 18

/** 引线水平接线长度（圆点 → 标签近端）。 */
export const KEY_EVENT_LEADER_ELBOW = 12

/** 日聚焦卡片宽。 */
export const KEY_EVENT_CARD_WIDTH = 280

/** 行高公式的固定偏移（轴带 + 上下带留白）。 */
export const KEY_EVENT_ROW_HEIGHT_OFFSET = 50

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
 * 行高：标签行高 = min(246, max(70, 50 + 32 × (上带层数 + 下带层数)))。
 *
 * 双带下层距是 32px（不再是旧单侧的 44px），故 4 层 = 178px，仍低于 246px 封顶
 * （design D14）；层数更多时钉在封顶高度。
 *
 * @param layerCount 上带 + 下带合计层数
 */
export function keyEventRowHeight(layerCount: number): number {
  const layers = Math.max(0, Math.floor(layerCount) || 0)
  const grown = KEY_EVENT_ROW_HEIGHT_OFFSET + KEY_EVENT_BAND_LAYER_STEP * layers
  return Math.min(KEY_EVENT_MAX_ROW_HEIGHT, Math.max(KEY_EVENT_BASE_ROW_HEIGHT, grown))
}

/**
 * 最终行高 = max(标签行高, 卡片列占位高度)；**卡片一侧不受 246 封顶约束**
 * （≤ 4 张卡片全部展开，行高必须放得下——design D6）。
 *
 * @param labelRowHeight 标签行高（`keyEventRowHeight` 的结果）
 * @param cardStackHeight 卡片列占位高度（轴带下缘 + 8 + 卡片堆叠高 + 8；由调用方算，
 *   因为卡片列的纵向起点取决于上带层数，纯函数拿不到轴带位置）
 */
export function keyEventRowHeightWithCards(labelRowHeight: number, cardStackHeight: number): number {
  return Math.max(labelRowHeight, cardStackHeight)
}

/**
 * 该行是否收起标签（design D7/D14）：**只看这一行的层数**，与观察窗天数无关。
 * 层数口径 = 上带 + 下带合计；合计 ≤ 4 照常显示事件名称（哪怕 55 / 365 天）；
 * 超上限才退化为节点 + 当日计数，名称由该行选中日的卡片承接。
 *
 * @param layerCount 上带 + 下带合计层数
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

// ── 双带布局（key-event-day-focus-cards：引线树标签 + 日聚焦卡片）────────────

/** 归带输入的一条标签（同日组内的一条）。 */
export interface KeyEventBandItem {
  /** 事件标识（布局结果回填键）。 */
  key: string
  /** 圆点中心 x（行体坐标系）。 */
  dotX: number
  /** 标签自然宽（offsetWidth 实测；标题 MUST NOT 截断 → 实测宽就是全宽）。 */
  width: number
}

/** 同日分组（同一天的事件归为一组，占连续 k 层）。 */
export interface KeyEventBandGroup {
  /** 日期（date-only）。 */
  date: string
  /** 组内标签（圆点 x + 自然宽）；数组顺序即组内层序。 */
  items: readonly KeyEventBandItem[]
}

/** 卡片矩形（下带的固定障碍区）。 */
export interface KeyEventCardRect {
  /** 左缘。 */
  left: number
  /** 右缘。 */
  right: number
  /**
   * 矩形是否被钳制过（行体窄于卡片宽 / 选中列越出行体右缘）：钳制后矩形只覆盖
   * 行体的一部分，实际卡片仍按 280px 渲染，溢出部分由舞台容器内部横向滚动承接。
   */
  clamped?: boolean
}

/** 标签归属带（上带 = 生命线上方，下带 = 生命线下方）。 */
export type KeyEventBand = 'up' | 'down'

/** 一条标签的归带结果。 */
export interface KeyEventBandPlacement {
  /** 事件标识。 */
  key: string
  /** 归属带。 */
  band: KeyEventBand
  /** 带内层序（0 起）。 */
  layer: number
  /** 标签左缘（组内统一）。 */
  left: number
  /** 标签自然宽。 */
  width: number
  /** 圆点中心 x。 */
  dotX: number
  /**
   * 该标签的引线是否仍会穿过**同组其它标签**（几何上无解时才为 true）。
   *
   * 正常路径恒为 false：同日多条的组要么整体在圆点右侧、要么整体在圆点左侧（单条组贴右缘时圆点落在自身范围内，引线退化为竖线段，不穿过任何标签），组内竖直引线
   * 都落在组标签的横向范围之外（delta spec：组内引线 MUST NOT 穿过组内标签）。
   */
  crossesGroupLabel?: boolean
}

/** 归带结果。 */
export interface KeyEventBandLayout {
  /** 逐条标签的归属与位置（按日期升序）。 */
  placements: KeyEventBandPlacement[]
  /** 上带层数。 */
  upLayers: number
  /** 下带层数。 */
  downLayers: number
  /**
   * 该行是否溢出：有组在两带都找不到可用的连续 k 层（review R-16）。
   * 调用方据此收起该行，MUST NOT 把未校验的层当可用。
   */
  overflowed?: boolean
  /** 溢出时没放下的标签 key（诊断用）。 */
  unplacedKeys?: string[]
}

/** 带内一层：已占右边界 + 该层已有标签（算覆盖条数用）。 */
interface KeyEventBandLayer {
  /** 已占右边界。 */
  right: number
  /** 该层已有标签（近端 + 宽）。 */
  labels: { left: number; width: number }[]
}

/**
 * 分层搜索上限（带内层数）：搜索到它仍找不到可用连续层时，判定该行溢出
 * （review R-16 —— 搜索到上限就返回的层**没被校验过**，MUST NOT 当成可用层）。
 */
export const KEY_EVENT_MAX_LAYER_SEARCH = 400

/** 单侧评估结果。 */
interface KeyEventBandSideResult {
  /** 起始层。 */
  startLayer: number
  /** 代价。 */
  cost: number
  /** 是否找到**真正可用**的连续 k 层（false = 搜索到上限仍冲突）。 */
  fits: boolean
}

/**
 * 单侧评估：起始层（第一个容得下整组的连续 k 层）+ 代价。
 *
 * 代价 = 100 ×（带内横向范围盖住本组任一圆点 x 的既有标签**条数**）
 *      +  10 ×（起始层 + k）
 *      +   3 ×（该带已用层数）
 *
 * 覆盖条数按「标签」计（不按层聚合）：同一条标签只能被计一次。
 */
function evalBandSide(
  layers: readonly (KeyEventBandLayer | null)[],
  left: number,
  group: KeyEventBandGroup,
): KeyEventBandSideResult {
  const k = group.items.length
  let startLayer = 0
  let fits = false
  for (;;) {
    let ok = true
    for (let i = 0; i < k; i += 1) {
      const layer = layers[startLayer + i]
      if (layer && layer.right + KEY_EVENT_LABEL_GAP > left) { ok = false; break }
    }
    if (ok) { fits = true; break }
    startLayer += 1
    if (startLayer > KEY_EVENT_MAX_LAYER_SEARCH) break
  }
  let covering = 0
  for (const layer of layers) {
    if (!layer) continue
    for (const label of layer.labels) {
      const right = label.left + label.width
      if (group.items.some(item => item.dotX >= label.left && item.dotX <= right)) covering += 1
    }
  }
  const used = layers.filter(layer => layer !== null).length
  return { startLayer, cost: 100 * covering + 10 * (startLayer + k) + 3 * used, fits }
}

/** 同日组的横向摆放结果。 */
interface KeyEventGroupPlacement {
  /** 组内标签统一的左缘。 */
  left: number
  /** 组内最宽标签的宽（组的横向占位）。 */
  width: number
}

/**
 * 同日组的横向摆放（review R-15）。
 *
 * 首选「圆点右侧」：组内标签近端统一对齐「该日最右圆点 + 12px」（design D3）。
 * 同日多条组右边放不下时改为「圆点左侧」：组右缘 = 该日**最左**圆点 − 12px。
 *
 * **为什么不用「左移贴右缘」**（原来 left = max(0, bodyWidth − 组宽)）：贴边会把整组
 * 挪到圆点**左边**，于是圆点落进组标签的横向范围里，下层标签的竖直引线必然从上层
 * 标签身上穿过（行宽 1200 / 圆点 1150·1174 / 标签宽 300 → 竖线在 x=1174 穿过第 0 层
 * 标签）。挪到圆点左侧后，组内所有竖直引线都落在标签横向范围**之外**，
 * 「组内引线 MUST NOT 穿过组内标签」成立。
 *
 * 例外（几何无解）：组宽 > 「最左圆点 − 12px」且右边又放不下时（需要极宽标签 +
 * 极大同日扇出），只能贴右缘——此时组内引线仍可能穿过组内标签，由调用方按
 * `crossesGroupLabel` 标记收起该行。
 *
 * **同日单条组（k = 1）改摆右缘、不做左侧镜像（R-32）**：一条标签不存在「组内其它
 * 标签」，圆点落在它自己的横向范围里时引线退化为一段竖线（`keyEventLeaderPath` 的
 * inside 分支，design 已定），因此不需要「圆点落在组标签范围外」这条不变式。贴右缘
 * 恰好仍紧邻圆点（贴边只在 `最右圆点 + 12 + 组宽 > 行体宽` 时触发，即圆点离右缘不足
 * 一个标签宽），而左侧镜像会把它推到中部拥挤区、几乎必然新开一层。实测（30 天窗口、
 * 11 条真实标题、64px 列宽）：全部镜像 = 上 2 + 下 3 = 5 层 → 整行收起成 ★N；单条组
 * 贴右缘 = 上 2 + 下 2 = 4 层 → 事件名可见（与已批准原型同口径，原型此处即贴右缘）。
 */
function placeBandGroup(group: KeyEventBandGroup, bodyWidth: number): KeyEventGroupPlacement {
  const width = group.items.reduce((acc, item) => Math.max(acc, Math.max(0, item.width)), 0)
  const maxDotX = group.items.reduce((acc, item) => Math.max(acc, item.dotX), -Infinity)
  const minDotX = group.items.reduce((acc, item) => Math.min(acc, item.dotX), Infinity)
  const rightLeft = maxDotX + KEY_EVENT_LEADER_ELBOW
  if (rightLeft + width <= bodyWidth) return { left: rightLeft, width }
  if (group.items.length > 1) {
    const altLeft = minDotX - KEY_EVENT_LEADER_ELBOW - width
    if (altLeft >= 0) return { left: altLeft, width }
  }
  return { left: Math.max(0, bodyWidth - width), width }
}

/**
 * 双带归带与分层（design D3 / ui-design Layout Contract）。
 *
 * 按日期升序逐组处理：同一天的 k 条标签归为一组、占连续 k 层，组内近端统一对齐
 * 「该日最右圆点 + 12px」（同日多条呈树形分叉）；**同日多条整组放不下时改摆到圆点左侧**
 * （组右缘 = 最左圆点 − 12px），MUST NOT 让组内引线穿过组内标签（review R-15，
 * 理由见 `placeBandGroup`）。上/下带各算代价，取代价小的一侧，**并列取下带**；
 * 下带是卡片列的固定障碍区，组的横向范围与卡片矩形相交时该组改走上带。
 *
 * 两带都找不到可用连续 k 层时（分层搜索到上限，review R-16）该组**不落位**，
 * 并把该行标记为 `overflowed`——MUST NOT 把未校验的层当可用。
 *
 * @param groups 同日分组（各组自带圆点 x 与标签自然宽）
 * @param bodyWidth 行体宽（标签右缘的边界）
 * @param cardRect 日聚焦卡片矩形；无卡片时传 null
 * @param selectedDate 选中日（可选）：该日不出药丸（由卡片承接，design D3）；
 *   不传则不跳过任何分组
 */
export function layoutKeyEventBands(
  groups: readonly KeyEventBandGroup[],
  bodyWidth: number,
  cardRect: KeyEventCardRect | null,
  selectedDate?: string | null,
): KeyEventBandLayout {
  const up: (KeyEventBandLayer | null)[] = []
  const down: (KeyEventBandLayer | null)[] = []
  const placements: KeyEventBandPlacement[] = []
  const unplacedKeys: string[] = []
  let overflowed = false
  const ordered = [...groups].sort((a, b) => (a.date < b.date ? -1 : a.date > b.date ? 1 : 0))

  for (const group of ordered) {
    if (selectedDate != null && group.date === selectedDate) continue
    if (group.items.length === 0) continue
    const placed = placeBandGroup(group, bodyWidth)
    const left = placed.left
    const groupWidth = placed.width

    const upResult = evalBandSide(up, left, group)
    const downBlocked = cardRect !== null && left < cardRect.right && left + groupWidth > cardRect.left
    const downResult = downBlocked ? null : evalBandSide(down, left, group)

    let band: KeyEventBand
    let result: KeyEventBandSideResult
    if (upResult.fits && downResult !== null && downResult.fits) {
      if (downResult.cost <= upResult.cost) { band = 'down'; result = downResult }
      else { band = 'up'; result = upResult }
    } else if (upResult.fits) {
      band = 'up'; result = upResult
    } else if (downResult !== null && downResult.fits) {
      band = 'down'; result = downResult
    } else {
      // 两带都找不到可用的连续 k 层：该组不落位，显式标记溢出（review R-16）
      overflowed = true
      for (const item of group.items) unplacedKeys.push(item.key)
      continue
    }

    const target = band === 'up' ? up : down
    group.items.forEach((item, index) => {
      const layer = result.startLayer + index
      const width = Math.max(0, item.width)
      const placement: KeyEventBandPlacement = {
        key: item.key, band, layer, left, width, dotX: item.dotX,
      }
      // 组内引线穿过组内其它标签（几何无解的兜底情形）：只可能发生在「圆点落在组标签
      // 横向范围内」且该标签不是组内最内层时（更外层的标签挡在内层标签之前）
      const crosses = group.items.slice(0, index).some(other =>
        item.dotX >= left && item.dotX <= left + Math.max(0, other.width))
      if (crosses) placement.crossesGroupLabel = true
      placements.push(placement)
      while (target.length <= layer) target.push(null)
      const prev = target[layer]
      const right = left + width
      target[layer] = {
        right: prev ? Math.max(prev.right, right) : right,
        labels: (prev ? prev.labels : []).concat([{ left, width }]),
      }
    })
  }

  const upLayers = up.reduce((acc, layer, index) => (layer ? Math.max(acc, index + 1) : acc), 0)
  const downLayers = down.reduce((acc, layer, index) => (layer ? Math.max(acc, index + 1) : acc), 0)
  return { placements, upLayers, downLayers, overflowed, unplacedKeys }
}

/** 引线输入（行体坐标系）。 */
export interface KeyEventLeaderInput {
  /** 圆点中心 x。 */
  dotX: number
  /** 竖段起点 y：圆点外缘（上带 = 圆点顶缘，下带 = 圆点底缘）。 */
  dotY: number
  /** 标签左缘（近端）。 */
  labelLeft: number
  /** 标签顶边 y。 */
  labelTop: number
  /** 标签自然宽。 */
  labelWidth: number
  /** 标签高（默认 `KEY_EVENT_LABEL_HEIGHT`）。 */
  labelHeight?: number
  /** 标签在生命线的哪一侧：决定竖段贴标签上缘还是下缘。 */
  band: KeyEventBand
}

/** 引线的一段（竖段或横段）。 */
export interface KeyEventLeaderSegment {
  x1: number
  y1: number
  x2: number
  y2: number
}

/** 引线折点几何（竖段 + 可选横段）。 */
export interface KeyEventLeaderPath {
  /** 竖段：自圆点外缘走到标签近缘。 */
  vertical: KeyEventLeaderSegment
  /** 横段：自竖段末端接入标签近端；圆点 x 落在标签横向范围内时为 null（只画竖段）。 */
  horizontal: KeyEventLeaderSegment | null
  /** SVG path 的 d。 */
  d: string
}

/**
 * 引线折点（design D4）：竖段自圆点外缘走到标签近缘（上带 = 标签下缘、下带 = 标签上缘，
 * 标签遮线不遮字）；圆点 x 落在标签横向范围内时**只画竖段**，否则竖段走到标签垂直
 * 中线，再横段接入标签近端（近端对齐「圆点 + 12px」时横段恰为 12px）。
 *
 * 标签可能在圆点**左侧**（同日多条整组放不下时改摆到圆点左侧，review R-15）——此时近端是
 * 标签的**右缘**，横段自圆点向左接入；两种朝向由 dotX 与 labelLeft / labelRight 的
 * 相对位置自动判定，调用方 MUST NOT 假定近端恒为左缘。
 */
export function keyEventLeaderPath(input: KeyEventLeaderInput): KeyEventLeaderPath {
  const height = input.labelHeight ?? KEY_EVENT_LABEL_HEIGHT
  const width = Math.max(0, input.labelWidth)
  const labelRight = input.labelLeft + width
  const centerY = input.labelTop + height / 2
  const nearY = input.band === 'up' ? input.labelTop + height : input.labelTop
  const inside = input.dotX >= input.labelLeft && input.dotX <= labelRight
  const vertical: KeyEventLeaderSegment = {
    x1: input.dotX,
    y1: input.dotY,
    x2: input.dotX,
    y2: inside ? nearY : centerY,
  }
  if (inside) {
    return {
      vertical,
      horizontal: null,
      d: `M ${input.dotX} ${input.dotY} L ${input.dotX} ${vertical.y2}`,
    }
  }
  const endX = input.dotX < input.labelLeft ? input.labelLeft : labelRight
  const horizontal: KeyEventLeaderSegment = { x1: input.dotX, y1: centerY, x2: endX, y2: centerY }
  return {
    vertical,
    horizontal,
    d: `M ${input.dotX} ${input.dotY} L ${input.dotX} ${centerY} L ${endX} ${centerY}`,
  }
}

/**
 * 日聚焦卡片矩形（design D6）：左缘 = 选中列左缘；`left + 280 > bodyWidth` 时改为
 * 右缘对齐选中列右缘向左展开（MUST NOT 伸出舞台）。
 *
 * **不变式**（review R-18）：返回值恒满足 `0 <= left` 且 `right <= max(bodyWidth, 280)`。
 * 为什么这样定：卡片矩形同时是「下带标签的避让障碍区」，它越出行体就会让下带标签
 * 误判为「不与卡片相交」；反之，行体窄于卡片宽（< 280px）时卡片**装不下**，此时把
 * 矩形钳制在 [0, bodyWidth] 内（右缘不再对齐选中列，宽度收缩到行体宽），
 * 实际卡片仍按 280px 渲染——溢出部分由**舞台容器内部横向滚动**承接（与「列宽下限
 * 64px + 舞台内横滚」同一口径），并由 `clamped` 标记告知调用方。
 *
 * @param selectedIndex 选中列在可见轴上的序号（< 0 = 无选中列 → 返回 null）
 * @param columnWidth 列宽
 * @param bodyWidth 行体宽
 */
export function keyEventDayCardRect(
  selectedIndex: number,
  columnWidth: number,
  bodyWidth: number,
): KeyEventCardRect | null {
  if (!Number.isFinite(selectedIndex) || selectedIndex < 0) return null
  const body = Math.max(0, bodyWidth)
  const columnLeft = selectedIndex * columnWidth
  const columnRight = columnLeft + columnWidth
  let left = columnLeft
  if (left + KEY_EVENT_CARD_WIDTH > body) left = Math.max(0, columnRight - KEY_EVENT_CARD_WIDTH)
  // 钳制进 [0, body]：选中列越出行体右缘、或行体本身窄于卡片宽时都不越界
  let clamped = false
  if (left + KEY_EVENT_CARD_WIDTH > body) {
    left = Math.max(0, Math.min(left, body - Math.min(KEY_EVENT_CARD_WIDTH, body)))
    clamped = true
  }
  const width = Math.min(KEY_EVENT_CARD_WIDTH, body - left)
  const rect: KeyEventCardRect = { left, right: left + width }
  if (clamped) rect.clamped = true
  return rect
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
