<script setup lang="ts">
/**
 * 泳道修正工作面 · 中央画布宿主（lane-manual-correction-loop 5.2 / 5.5 / 5.9）。
 *
 * 画布纪律（复用既有编排画布口径，纯计算在 `laneComposeLogic`）：
 * - **容器真实像素 1:1**：viewBox 宽 = 容器 `clientWidth`；窗口尺寸变化后重排重绘；
 * - **查看与选入分离**：点节点只改选中态（节点卡跟随），成员数不变；
 * - 拖拽平移不误伤点选（位移超阈值才算拖）；滚轮缩放 0.5×–3×，缩放只影响呈现；
 * - **成员行距跟随节点占高**（fix-lane-row-gap-zoom）：同一天成员按行错开时行距 =
 *   `max(16, 圆直径 + 标签起笔距 + 两行标签 + 4)` = 44px。**不按半径推导**：本画布成员
 *   半径恒为 5.5（`MEMBER_BASE_RADIUS` 覆盖），按半径只得出 16px，而上一个节点的两行
 *   标签（+34.5px）会压在下一个节点的圆上——这才是“同一天多节点拥挤”的真因。
 *   成员区变高时分区下移，画布变高由容器滚动承接（宁滚动不重叠）；
 * - 图面文字 / 编号 / 横条不越出画布边界，标题冲突时宁缺勿叠。
 *
 * 修正模式专属：
 * - **「更早」折叠带**（画布左缘）：当前首义集含窗口外节点时出现 —— 虚线分隔 +「更早 · N」
 *   + 首义虚线圆点纵向堆叠（**非比例折叠**，不按时间距离定位）；上限 6 个 +「+M」溢出；
 *   带内节点可点出同款节点卡、可勾选（与左栏「更早」分组同一份勾选态，双向同步）。
 * - **画布内确认条**：覆盖已有首义时就地确认（不叠弹窗）。
 * - **节点卡写操作**（unify-lane-workbench D8）：聚焦后卡上直接「＋加入本泳道」/
 *   「⊘移出本泳道」/「↩加回」，当场生效（emit 意图，宿主统一走 useLaneCorrection）；
 *   **点画布本体仍只改选中态**（查看与选入分离不破）；active 只读与归属未知节点
 *   不提供写入口；按钮阻止冒泡。
 * - **节点卡标题完整展示**（实机反馈）：标题按需换行、不以省略号截断；
 * - **轻点空白取消聚焦**（实机反馈）：聚焦在场时点画布空白处 emit `clear-focus`（节点卡收起）；
 *   平移松手、落在节点 / 节点卡 / 缩放条 / 折叠带勾选框上的操作不算；
 * - **话题带不画强调色横条**（实机反馈）：成员行上方只留标签与中性基线，与其它分区同形。
 * - **相关高亮**（D6）：焦点的一跳原关系 ∪ 推荐（relatedNodeIds 唯一入口）；
 *   其余节点弱化；取消聚焦恢复默认。
 * - **节点表达统一 pointAppearance**（D7）：成员固定尺寸、候选按**批内相对归一**的接近度、
 *   只读更弱更小；已移出划除 / 首义标记 / 焦点环 / 相关 / 弱化是正交标志。
 *
 * 拖拽改归属（align-lane-correction-workface 第 4 节）：
 * - **落点按分区、不按日期列**（X 轴是报告期事实，拖动不改写 `periodDate`）：
 *   成员区 = 加入 / 加回，成员区之外 = 移出，只读参照区与「更早」折叠带 = 不可落点；
 * - **可拖性逐节点判定**：只读参照 / 折叠带 / 落库中（pendingIds）节点不可拖；
 * - **手势纪律**与编排画布一致：pointer 事件、4px 起拖阈值、拖拽期抑制文本选中、
 *   指针捕获只在判定为拖拽那一刻启用、拖完 250ms 内吞 click、Esc / pointercancel 取消；
 * - **即时落库、无草稿**：松手即 emit 既有 `add` / `remove` / `restore`（同一写路径、
 *   同一份成员集合）；落库中节点半透明且不可再拖（防重入）；
 * - **失败乐观回滚 + 节点内联标错**：分区由 props 派生，写失败即回原分区，
 *   错误以红描边 + `title` / `aria-live` 就地呈现（不弹窗、不中断连续拖拽）。
 */
import { computed, ref, useId, watch } from 'vue'
import {
  canvasContentPoint,
  estimateTextWidth,
  isDragBeyond,
  panScroll,
  pointAppearance,
  relatedNodeIds,
  relativeProximities,
  scaledPointRadius,
  type LanePointAppearance,
  type LaneRelation,
} from '~/features/tags/composables/laneComposeLogic'
import { useLaneCanvasShell } from '~/features/tags/composables/useLaneCanvasShell'
import LaneCanvasStickyRail from '~/features/tags/components/LaneCanvasStickyRail.vue'
import type { LaneCorrectionNode } from '~/features/tags/composables/useLaneCorrection'
import { LANE_FOLD_MAX } from '~/features/tags/composables/useLaneCorrection'

const props = withDefaults(defineProps<{
  /** 泳道名（话题带标题）。 */
  laneLabel: string
  /** 窗口内日期轴（YYYY-MM-DD）。 */
  dates: string[]
  /** 本泳道成员（窗口内）。 */
  memberNodes: LaneCorrectionNode[]
  /** 候选（未选入本泳道）。 */
  candidateNodes: LaneCorrectionNode[]
  /** 其它泳道节点（只读参照）。 */
  readonlyNodes: LaneCorrectionNode[]
  /** 本次移出的节点。 */
  removedNodes: LaneCorrectionNode[]
  /** 窗外首义节点（「更早」折叠带）。 */
  foldNodes: LaneCorrectionNode[]
  /** 已勾选的首义范围节点 id。 */
  anchorSelection: string[]
  /** 画布选中（只改选中态）。 */
  focusId: string | null
  /** 加载中（骨架）。 */
  loading: boolean
  /** 加载失败（行内错误 + 重试）。 */
  error: string | null
  /** 画布无节点可展示时的空态说明（null = 不展示）。 */
  emptyNote: string | null
  /** 画布内确认条是否展开（覆盖已有首义）。 */
  confirmVisible: boolean
  /** 当前首义来源（确认条文案用）。 */
  sourceLabel: string
  /** 质心推荐阈值（候选接近度→半径映射；与右栏同一口径）。 */
  matchThreshold?: number
  /** 窗口内两端可见的原关系（相关高亮共享入口的输入）。 */
  relations?: LaneRelation[]
  /** 正在提交的节点 id（卡片按钮禁用，防重复提交；与左栏/右栏同一份）。 */
  pendingIds?: string[]
  /**
   * **已生效首义**节点 id（当前首义集里的成员）。
   *
   * 与「待提交勾选」（`anchorSelection`）是**两套标记**：左栏勾选但尚未提交的节点
   * 画虚线环；当前首义集里的成员画实线环——两者可区分（D10 / 5.4）。
   */
  anchorIds?: string[]
}>(), {
  matchThreshold: 0.3,
  relations: () => [],
  pendingIds: () => [],
  anchorIds: () => [],
})

const emit = defineEmits<{
  /** 点节点 → 只改选中态。 */
  focus: [id: string]
  /** 点画布空白处 → 取消聚焦（节点卡收起）；平移松手与落在节点上的轻点不算。 */
  'clear-focus': []
  /** 节点卡「＋加入本泳道」（显式写意图，当场生效）。 */
  add: [id: string]
  /** 节点卡「⊘移出本泳道」（直接解除归属，不选去处）。 */
  remove: [id: string]
  /** 节点卡「↩加回」（本次移出的节点归回本泳道）。 */
  restore: [id: string]
  /** 折叠带勾选 → 与左栏「首义范围」同一份选择集。 */
  toggleAnchor: [id: string]
  /** 确认条「继续重设」。 */
  confirmApply: []
  /** 确认条「取消」。 */
  cancelApply: []
  /** 加载失败后的重试。 */
  retry: []
}>()

/** 兜底画布宽（容器未测量时用；真实渲染以容器 clientWidth 为准）。 */
const FALLBACK_WIDTH = 900
/** 左侧标签槽（话题带 / 分区标签的留白）。 */
const LEFT_PAD = 150
/** 「更早」折叠带宽（固定窄列）。 */
const FOLD_W = 68
/** 折叠带与时间轴之间的间隙。 */
const FOLD_GAP = 8
const RIGHT_PAD = 28
/** 话题带基线。 */
const BAND_Y = 44
/**
 * 日期轴区高：刻度文字（基线 14）落在这一条里，列 guide 从它起笔（`y1`）。
 *
 * 纵向吸顶的范围就是 `y ∈ [0, DATE_AXIS_H]`——**分区带不钉顶**（unify-lane-canvas-shell D6：
 * 本画布只有一条泳道，钉顶等于让整块上部常驻，会吃掉候选区可视高）。
 */
const DATE_AXIS_H = 36
/** 成员节点基线。 */
const MEMBER_Y = 86
/**
 * 成员节点半径（本画布固定尺寸，**不随列宽变化、不编码接近度**）。
 *
 * 取 7.5 是布局链联动的结果（D4）：`pointAppearance` 的候选半径上限被
 * `min(cap × 0.72, memberBase − 0.5)` 钉死在成员基准半径之下——成员基准半径
 * 只有 5.5 时，候选再怎么调区间也只能落在 5.0 上下（肉眼无差别）。
 * 抬到 7.5 后候选才有 5.9–7.4 的可见档差，且仍 < 成员（推荐 ≤ 已选）。
 */
const MEMBER_BASE_RADIUS = 7.5
/** 普通点基准半径（缺省公式的输入；只读参照与无估算候选走它）。 */
const BASE_POINT_RADIUS = 4
/**
 * 候选接近度 → 半径 / 深浅（**批内相对归一**后使用）。
 *
 * 半径 5.9–7.4（直径 11.8–14.8）< 成员 7.5；深浅 18%–80%。
 * 与编排画布同款区间，两画布的候选视觉一致。
 */
const RECOMMEND_RADIUS_RANGE = { min: 5.9, max: 7.4 }
const PROXIMITY_STRENGTH_RANGE = { min: 18, max: 80 }
/** 标题起笔距节点下沿的间距（引导线与标签共用）。 */
const LABEL_TOP_GAP = 5
/** 节点卡宽度固定（绝对定位卡片量宽会形成塌缩反馈环）。 */
const CARD_WIDTH = 264
/** 标题每行字数 / 最多行数（两行截断，title 属性给全名）。 */
/** 每行最多多少字（硬上限；实际按列宽收窄，见 labelPerLine）。 */
const LABEL_PER_LINE_MAX = 10
/** 标签字号（px）；须与 `.lcc-label { font-size }` 一致。 */
const LABEL_FONT_SIZE = 11
const LABEL_MAX_LINES = 2
const LABEL_LINE_H = 12
/**
 * 单个节点的纵向总占高：圆直径 + 标签起笔间距 + 标签最多两行。
 * 行距必须≥它，否则上一个节点的标签会压在下一个节点的圆上——
 * 这才是“同一天多节点拥挤”的真因（半径 5.5 直径 11 本身不重叠）。
 */
/**
 * 单个节点纵向占高（圆顶 → 标签**框**底）。
 * 必须含标签框自带的 4px 内边距（`labels` 里 height = lines*LINE_H + 4），
 * 否则行距会让标签框底与下一圆顶**恰好相切**，`circleHit` 的 >= 判断
 * 进而把同列下一个圆判成相交，把标题整套吞掉（review A-R-1）。
 */
const LABEL_BOX_PAD = 4
const NODE_BLOCK_H = MEMBER_BASE_RADIUS * 2 + LABEL_TOP_GAP + LABEL_MAX_LINES * LABEL_LINE_H + LABEL_BOX_PAD

const cardEl = ref<HTMLElement | null>(null)
const cardSize = ref({ width: CARD_WIDTH, height: 120 })

/**
 * 画布外壳（容器测量 / 缩放 / 滚动同步 / `Esc`）——与编排画布共用同一份实现
 * （unify-lane-canvas-shell D1）。
 *
 * `contentHeight` / `contentWidth` 用 getter 惰性传入：`canvasHeight` / `vbWidth`
 * 声明在本调用之后，计算属性惰性求值因此不踩 TDZ。
 */
const {
  containerRef,
  containerWidth,
  containerHeight,
  measure,
  zoom,
  zoomPercent,
  changeZoom,
  resetZoom,
  syncScroll,
  scrollTopLogical,
  scrollLeftLogical,
  overflowing,
  overflowingX,
} = useLaneCanvasShell({
  contentHeight: () => canvasHeight.value,
  contentWidth: () => vbWidth.value,
  // 拖拽期间屏蔽滚轮缩放（D3-①：编辑侧原本缺这个守卫，本次向编排侧对齐）
  isZoomBlocked: () => dragging.value,
  // Esc 同时取消拖拽与平移（D3-②：编辑侧原本只取消拖拽）
  hasActiveGesture: () => dragging.value || isPanning.value,
  onCancelGesture: () => {
    lastDragAt = Date.now()
    if (isPanning.value) lastPanAt = Date.now()
    resetGesture()
    panStart = null
    isPanning.value = false
  },
})

defineExpose({ measure })

// ── 缩放（只影响呈现，不碰选择 / 成员数 / 首义选择集） ─────────────────────
/**
 * 滚轮 / 锚点换算 / 重置全部由外壳 composable 提供（见上方 `useLaneCanvasShell`）；
 * 本画布只保留 `＋ / － / 重置` 三个控件接的 `changeZoom` / `resetZoom`（由外壳返回）。
 */
// ── 拖拽平移（空白处拖；位移超阈值才算拖，不误伤点选） ─────────────────────
let panStart: { x: number, y: number, left: number, top: number, onNode: boolean } | null = null
let lastPanAt = 0
const isPanning = ref(false)

function onSurfacePointerDown(event: PointerEvent): void {
  const target = event.target as HTMLElement | null
  if (target?.closest?.('.lcc-card, .lcc-zoom, button, .lcc-fold-check')) return
  const surface = containerRef.value
  if (!surface) return
  // 命中可拖节点 → 接管手势为拖拽候选。
  // 不可拖节点（只读参照 / 「更早」折叠带 / 落库中）**不接管拖拽，但也不吞手势**：
  // 按下退化为空白处平移（旧能力：任何非交互目标都可起平移）；
  // 未过阈值松手仍算点击，由 click 处理聚焦（onNodeClick 语义不变）。
  const nodeEl = target?.closest?.('[data-node]') as HTMLElement | null
  if (nodeEl) {
    const id = nodeEl.getAttribute('data-node') ?? ''
    if (isDraggable(id)) {
      gesture = {
        id,
        start: { x: event.clientX, y: event.clientY },
        scroll: { left: surface.scrollLeft, top: surface.scrollTop },
        dragging: false,
      }
      // 阻止默认行为：拖动时不选中画布上的其它文字（同编排画布纪律）。
      event.preventDefault()
      return
    }
  }
  panStart = { x: event.clientX, y: event.clientY, left: surface.scrollLeft, top: surface.scrollTop, onNode: Boolean(nodeEl) }
  isPanning.value = false
}

function onSurfacePointerMove(event: PointerEvent): void {
  // 节点拖拽优先（手势与画布平移互斥）。
  if (gesture) {
    if (!gesture.dragging) {
      if (!isDragBeyond(gesture.start, { x: event.clientX, y: event.clientY })) return
      beginNodeDrag(event)
    }
    updateDragPoint(event)
    return
  }
  if (!panStart) return
  const surface = containerRef.value
  if (!surface) return
  if (!isPanning.value) {
    if (!isDragBeyond(panStart, { x: event.clientX, y: event.clientY })) return
    isPanning.value = true
    try { surface.setPointerCapture?.(event.pointerId) } catch { /* 环境不支持指针捕获 */ }
  }
  const next = panScroll(
    { x: panStart.x, y: panStart.y },
    { x: event.clientX, y: event.clientY },
    { left: panStart.left, top: panStart.top },
  )
  surface.scrollLeft = next.left
  surface.scrollTop = next.top
}

function onSurfacePointerUp(event: PointerEvent): void {
  if (gesture) {
    const active = gesture
    const wasDragging = active.dragging
    const target = wasDragging ? zoneAtPoint(contentPointOf(event)) : 'none'
    resetGesture()
    // 未过 4px 阈值 → 仍算点击（只改选中态，由 click 处理）。
    if (!wasDragging) return
    // 拖完 250ms 内吞 click（防误触节点卡）。
    lastDragAt = Date.now()
    applyDrop(active.id, target)
    return
  }
  if (!panStart) return
  if (!panStart) return
  const wasPanning = isPanning.value
  const startedOnNode = panStart.onNode
  if (wasPanning) lastPanAt = Date.now()
  panStart = null
  isPanning.value = false
  // 空白处轻点（未平移、起手不在节点上）= 取消聚焦：节点卡收起、相关高亮 / 弱化恢复默认。
  // 落在节点上的轻点不算——聚焦与「再点同一节点取消」由节点自己的 click 负责，
  // 这里若先清空，随后的 click 会把它重新聚焦（切换语义坏掉）；平移松手是滚动，不是选择。
  if (!wasPanning && !startedOnNode && props.focusId) emit('clear-focus')
}

/** pointercancel：取消拖拽、**不发任何写请求**。 */
function onSurfacePointerCancel(): void {
  resetGesture()
  panStart = null
  isPanning.value = false
}

// ── 布局（容器真实像素 1:1） ──────────────────────────────────────────────
/** 可视宽度（容器实测；未测量时用回退值）。 */
const viewportWidth = computed(() => (containerWidth.value > 0 ? containerWidth.value : FALLBACK_WIDTH))

interface Point { x: number, y: number }

/** 画布可视高度基线（容器高度优先；不足时给最小可视区）。 */
const baseCanvasHeight = computed(() => Math.max(360, containerHeight.value || 360))

/** 折叠带是否存在（当前首义集含窗口外节点）。 */
const hasFold = computed(() => props.foldNodes.length > 0)

/** 时间轴左缘（有折叠带时整体右移一个折叠带宽）。 */
const axisX0 = computed(() => LEFT_PAD + (hasFold.value ? FOLD_W + FOLD_GAP : 0))

/**
 * 列宽下限：低于它一个标题无处可放（两行×10 字 ≈ 116px），列与列挤成一团。
 * 天数多到均分不够时**宁可把画布撑宽让人横向滚动，也不继续压缩列宽**，
 * 与纵向「宁滚动不重叠」同一原则。
 */
const MIN_COLUMN_WIDTH = 64

/** 列宽：理想均分，但不得低于下限（下限会撑宽画布，由容器横向滚动承接）。 */
const columnWidth = computed(() => {
  const avail = viewportWidth.value - axisX0.value - RIGHT_PAD
  return Math.max(MIN_COLUMN_WIDTH, avail / Math.max(1, props.dates.length))
})

/** 画布内容宽（≥ 视口宽）；列宽被下限撑大时画布变宽，容器横向滚动。 */
const vbWidth = computed(() =>
  Math.max(viewportWidth.value, axisX0.value + columnWidth.value * props.dates.length + RIGHT_PAD))

/**
 * 节点行距：按**节点总占高**（圆 + 两行标签）推导，保证上一个节点的标签底
 * 不会碰到下一个节点的圆顶（留 4px 间隙）。三个区共用同一行距。
 *
 * 不按半径推导——半径最大值仅 5.5，按半径只会得出 16，标签仍然会压（原缺陷）；
 * 也不随列宽变——半径不随列宽变，行距跟着变只会让窄窗更密。
 */
const nodeRowGap = computed(() => Math.max(16, NODE_BLOCK_H + 4))

/** 同一天在同一区内的最大堆叠数（纯计数，不依赖 zones → 无循环）。 */
const maxSlots = computed(() => {
  const countOf = (nodes: readonly LaneCorrectionNode[]): number => {
    const perDay = new Map<string, number>()
    for (const node of nodes) perDay.set(node.date, (perDay.get(node.date) ?? 0) + 1)
    return perDay.size === 0 ? 0 : Math.max(...perDay.values())
  }
  return {
    member: countOf(props.memberNodes),
    // 候选与已移出共用候选区坐标系，堆叠数必须合起来算（否则两者首个节点会重合）
    candidate: countOf([...props.candidateNodes, ...props.removedNodes]),
    readonly: countOf(props.readonlyNodes),
  }
})

/** 每个区需要的高度：至少容纳该区同日最大堆叠（下限 60 保持旧观感）。 */
const zoneHeights = computed(() => ({
  zone1: Math.max(60, maxSlots.value.candidate * nodeRowGap.value),
  zone2: Math.max(60, maxSlots.value.readonly * nodeRowGap.value),
}))

/**
 * 成员区底部：最后一个成员圆心 + NODE_BLOCK_H。
 *
 * 注：NODE_BLOCK_H（40）的语义是「圆顶→标签底」，加在圆心上会多算 5.5px
 * （半个圆）。这是**保守的高估**，不会反过来造成重叠；保留单一常量是为了
 * 让行距与区域高度同源，避免两个近似值各自漂移。
 */
const membersBottom = computed(() => {
  if (maxSlots.value.member === 0) return MEMBER_Y
  return MEMBER_Y + (maxSlots.value.member - 1) * nodeRowGap.value + NODE_BLOCK_H
})

/**
 * 画布高度：成员区 + 两个分区各自需要的堆叠高度 + 间隔与留白。
 * 节点再多也不压缩行距，超出部分由容器纵向滚动承接（宁滚动不重叠）。
 */
const canvasHeight = computed(() => {
  const needed = membersBottom.value + 48 + 30 + zoneHeights.value.zone1 + 46 + 30 + zoneHeights.value.zone2 + 56
  return Math.max(baseCanvasHeight.value, needed)
})

/** 画布分区（候选区 / 只读参照）：起点跟随成员区底部，高度按各自堆叠需求。 */
const zones = computed(() => {
  const top = Math.max(156, membersBottom.value + 48)
  const holder = Math.max(60, (canvasHeight.value - 56 - top - 92) / 2)
  // 有余量时两区均分剩余空间（保持旧观感），不足时用各自的需求高度（不压缩）。
  const z1 = Math.max(zoneHeights.value.zone1, holder)
  const z2 = Math.max(zoneHeights.value.zone2, holder)
  return {
    zone1Line: top,
    zone1Top: top + 30,
    zone1H: z1,
    zone2Line: top + z1 + 46,
    zone2Top: top + z1 + 46 + 30,
    zone2H: z2,
  }
})

const dateIndex = computed(() => new Map(props.dates.map((date, index) => [date, index])))

function xForDate(date: string): number {
  const index = dateIndex.value.get(date) ?? 0
  return axisX0.value + (index + 0.5) * columnWidth.value
}

/** 节点坐标：成员在基线上按列错行，候选在候选区，只读参照在只读区，移出节点在候选区。 */
const points = computed(() => {
  const map = new Map<string, Point>()
  const memberSlot = new Map<string, number>()
  const candidateSlot = new Map<string, number>()
  const readonlySlot = new Map<string, number>()
  const z = zones.value

  for (const node of props.memberNodes) {
    const key = node.date
    const slot = memberSlot.get(key) ?? 0
    memberSlot.set(key, slot + 1)
    map.set(node.id, { x: xForDate(node.date), y: MEMBER_Y + slot * nodeRowGap.value })
  }
  for (const node of props.candidateNodes) {
    const key = node.date
    const slot = candidateSlot.get(key) ?? 0
    candidateSlot.set(key, slot + 1)
    map.set(node.id, { x: xForDate(node.date), y: z.zone1Top + slot * nodeRowGap.value })
  }
  // 已移出节点与候选节点共用同一区坐标系与同一个堆叠池（否则两者首个节点会重合）。
  for (const node of props.removedNodes) {
    const key = node.date
    const slot = candidateSlot.get(key) ?? 0
    candidateSlot.set(key, slot + 1)
    map.set(node.id, { x: xForDate(node.date), y: z.zone1Top + slot * nodeRowGap.value })
  }
  for (const node of props.readonlyNodes) {
    const key = node.date
    const slot = readonlySlot.get(key) ?? 0
    readonlySlot.set(key, slot + 1)
    map.set(node.id, { x: xForDate(node.date), y: z.zone2Top + slot * nodeRowGap.value })
  }
  return map
})

/** 折叠带内节点（窗外首义）：纵向堆叠，**非比例**（不按时间距离定位）。 */
const foldPoints = computed(() => {
  const map = new Map<string, Point>()
  const count = props.foldNodes.length
  if (!count) return map
  const top = 62
  const limit = Math.max(top + 12, zones.value.zone1Line - 28)
  const gap = count > 1 ? Math.min(26, (limit - top) / (count - 1)) : 0
  const cx = LEFT_PAD + FOLD_W / 2
  props.foldNodes.forEach((node, index) => {
    map.set(node.id, { x: cx, y: top + index * gap })
  })
  return map
})

/** 全部节点位置（含折叠带）。 */
const allPoints = computed(() => {
  const map = new Map(points.value)
  for (const [id, point] of foldPoints.value) map.set(id, point)
  return map
})

// ── 吸顶（横向钉标签列 / 纵向只钉日期轴） ───────────────────────────────
/**
 * 吸顶标签列的**纵轴锚定**：覆盖区恒等于容器的可视高度，而不是内容坐标里的一段。
 *
 * 原因（2026-11-25 实机反馈）：底色如果按内容坐标从 0 盖到 `zone2Line + 30`，
 * 纵向滚动后这块底会**跟着内容上移**，底部露出一截没底的空带、中间留一道硬接缝，
 * 看上去像一个「错位的色块」而不是一列钉住的标签栏。
 *
 * 与日期轴吸顶带同一个算法，但**符号相反**：日期轴那组自带纵向 `translate(0, S)`，
 * 两者抵消后需要 `y = -S`；标签列这组只做**横向**位移、没有纵向 translate，
 * 所以直接取 `y = +S`（内容坐标里已经随滚动上移了 S，再回补 S 就落在视口顶）。
 */
const stickyRailAnchor = computed(() => {
  const top = scrollTopLogical.value
  const viewport = containerHeight.value > 0 ? containerHeight.value / (zoom.value || 1) : canvasHeight.value
  return { y: top, height: viewport }
})

/**
 * 日期轴的纵向吸顶位移（逻辑坐标 = `scrollTop / zoom`）；不溢出时**不下发** transform。
 *
 * 与编排画布同一条口径：只有日期轴钉顶，列 guide 留在内容层（纵向随内容走）。
 */
const dateAxisTransform = computed(() => (overflowing.value
  ? `translate(0, ${scrollTopLogical.value})`
  : undefined))

/**
 * 日期轴 / 列 guide 的横向裁切框：只允许画在吸顶标签列右侧的可视区。
 *
 * - 左界 = 标签列右缘 `axisX0`（视口坐标），右界 = `vbWidth`；
 * - 裁切框活在内容坐标，所以左界要加 `scrollLeftLogical` 才是「视口 `axisX0`」；
 * - `overflowingX` 为假时不下发 `clip-path`（不溢出时渲染与无吸顶实现逐像素一致）。
 */
const axisClipId = `lcc-axis-clip-${useId()}`
const axisClipPath = computed(() => (overflowingX.value ? `url(#${axisClipId})` : undefined))
const axisClipX = computed(() => axisX0.value + (overflowingX.value ? scrollLeftLogical.value : 0))
const axisClipWidth = computed(() => Math.max(0, vbWidth.value - axisClipX.value))

// ── 拖拽改归属（第 4 节：落点按**分区**，不按日期列） ─────────────────────────
/**
 * 落点分区。**X 轴是报告期事实**——拖动不得改写节点的 `periodDate`，
 * 因此落点只判「成员区 / 候选区（含已移出）/ 只读参照区」三条横向带。
 */
type DropZone = 'member' | 'candidate' | 'readonly' | 'fold' | 'none'

/** 节点当前所属分区（判定「拖出成员区 = 移出」与落库结果）。 */
type NodeZone = 'member' | 'candidate' | 'removed' | 'readonly' | 'fold'

/** 拖拽手势（与画布平移互斥：命中可拖节点即接管手势）。 */
interface DragGesture {
  id: string
  /** 指针起点（视口坐标）。 */
  start: { x: number, y: number }
  /** 起手时的滚动位置。 */
  scroll: { left: number, top: number }
  /** 是否已过 4px 阈值（过了才算拖）。 */
  dragging: boolean
}

/**
 * 落库意图（松手即记；用于失败回滚与节点内联标错）。
 *
 * `to` = 期望落到的分区；写成功后 props 会按该分区变化 → 意图消解为成功；
 * 写失败则分区不变 → 节点留在原分区（回滚）+ 内联标错。
 */
interface DropIntent {
  from: NodeZone
  to: NodeZone
  /** 是否观察到过「落库中」（pendingIds 含该 id）——没观察到就不能判失败。 */
  sawPending: boolean
}

const dragging = ref(false)
const dragGhost = ref<{ x: number, y: number } | null>(null)
const dragNodeId = ref<string | null>(null)
const hoverZone = ref<DropZone>('none')
/** 节点内联错误（写失败：不弹窗，就地红描边 + 文案）。 */
const nodeErrors = ref<Record<string, string>>({})
/** 最近一次失败文案（屏幕阅读器播报区）。 */
const lastErrorMessage = ref('')

let gesture: DragGesture | null = null
/** 拖拽意图表（写成功后消解）。 */
const dropIntents = new Map<string, DropIntent>()
/** 拖完 250ms 内吞 click（防误触节点卡）。 */
let lastDragAt = 0

/** 成员区上缘（话题带）。 */
const memberZoneTop = BAND_Y
/** 成员区下缘（成员节点总占高的底部）。 */
const memberZoneBottom = computed(() => membersBottom.value)
/** 候选区（含已移出节点）纵向范围。 */
const candidateZoneTop = computed(() => zones.value.zone1Top)
const candidateZoneBottom = computed(() => zones.value.zone1Top + zones.value.zone1H)
/** 只读参照区纵向范围。 */
const readonlyZoneTop = computed(() => zones.value.zone2Top)
const readonlyZoneBottom = computed(() => zones.value.zone2Top + zones.value.zone2H)

/** 节点当前分区。 */
function zoneOfNode(id: string): NodeZone {
  if (props.foldNodes.some(item => item.id === id)) return 'fold'
  if (isMember(id)) return 'member'
  if (isRemovedNode(id)) return 'removed'
  if (isReadonlyNode(id)) return 'readonly'
  return 'candidate'
}

/**
 * 可拖性**逐节点**判定：只读参照区与「更早」折叠带节点不可拖；
 * 落库中（`pendingIds`）也不可拖（防重入）。
 */
function isDraggable(id: string): boolean {
  if (props.pendingIds.includes(id)) return false
  const zone = zoneOfNode(id)
  return zone === 'member' || zone === 'candidate' || zone === 'removed'
}

/** 指针 → 内容坐标（解除滚动与缩放；两画布共用同一换算）。 */
function contentPointOf(event: PointerEvent): { x: number, y: number } {
  const surface = containerRef.value
  if (!surface) return { x: 0, y: 0 }
  return canvasContentPoint(
    event.clientX,
    event.clientY,
    surface.getBoundingClientRect(),
    { left: surface.scrollLeft, top: surface.scrollTop },
    zoom.value,
  )
}

/** 落点分区判定（**只看纵向分区带**；日期列不参与）。 */
function zoneAtPoint(point: { x: number, y: number }): DropZone {
  // 左缘「更早」折叠带不是落点（带内节点本就不可拖）。
  if (hasFold.value && point.x >= LEFT_PAD && point.x <= LEFT_PAD + FOLD_W) return 'fold'
  if (point.y >= memberZoneTop && point.y <= memberZoneBottom.value) return 'member'
  if (point.y >= candidateZoneTop.value && point.y <= candidateZoneBottom.value) return 'candidate'
  if (point.y >= readonlyZoneTop.value && point.y <= readonlyZoneBottom.value) return 'readonly'
  return 'none'
}

/**
 * 落点 → 写动作（null = 不发起任何写操作）。
 *
 * - 成员区：候选 → 加入、已移出 → 加回；已在成员区 → 无动作；
 * - 成员区之外（候选区 / 画布空白）：成员 → 移出；
 * - 只读参照区与折叠带：**不可落点**（不发起任何写操作）。
 */
function dropAction(id: string, target: DropZone): 'add' | 'remove' | 'restore' | null {
  const zone = zoneOfNode(id)
  if (target === 'readonly' || target === 'fold') return null
  if (target === 'member') {
    if (zone === 'candidate') return 'add'
    if (zone === 'removed') return 'restore'
    return null
  }
  if (zone === 'member') return 'remove'
  return null
}

/** 落点分区的视觉类（可落点高亮 / 不可落点弱化）。 */
function dropZoneClass(zone: DropZone): Record<string, boolean> {
  if (!dragging.value) return {}
  const id = dragNodeId.value
  const valid = id ? dropAction(id, zone) !== null : false
  if (hoverZone.value === zone) return valid ? { 'is-drop-target': true } : { 'is-drop-other': true }
  // 只读参照区永不可落点 → 拖拽期间弱化（与落点高亮明确不同）。
  if (zone === 'readonly') return { 'is-drop-other': true }
  return {}
}

/** 节点内联错误写入 / 清除。 */
function setNodeError(id: string, message: string): void {
  nodeErrors.value = { ...nodeErrors.value, [id]: message }
  lastErrorMessage.value = message
}

function clearNodeError(id: string): void {
  if (!nodeErrors.value[id]) return
  const next = { ...nodeErrors.value }
  delete next[id]
  nodeErrors.value = next
}

/**
 * 落库意图消解：写成功 → 分区已按 props 归位；写失败 → 分区未变 → 内联标错。
 *
 * 节点分区**完全由 props 派生**（不乐观改写本地成员集），所以「失败回滚」是
 * 天然的：写失败不改 props，节点就留在原分区。落库期间节点半透明（pendingIds），
 * 计数不跳变（与 ui-design「loading（拖拽落库）」一致）。
 */
function resolveDropIntents(): void {
  if (!dropIntents.size) return
  for (const [id, intent] of [...dropIntents.entries()]) {
    if (zoneOfNode(id) === intent.to) {
      dropIntents.delete(id)
      clearNodeError(id)
      continue
    }
    if (props.pendingIds.includes(id)) {
      intent.sawPending = true
      continue
    }
    if (intent.sawPending) {
      dropIntents.delete(id)
      setNodeError(id, '落库失败，节点已回到原分区（可重试）')
    }
  }
}

watch(
  [() => props.pendingIds, () => props.memberNodes, () => props.candidateNodes, () => props.removedNodes],
  () => resolveDropIntents(),
)

/** 起拖：过 4px 阈值才启动（未过阈值的仍算点击，只改选中态）。 */
function beginNodeDrag(event: PointerEvent): void {
  if (!gesture) return
  gesture.dragging = true
  dragging.value = true
  dragNodeId.value = gesture.id
  try { containerRef.value?.setPointerCapture?.(event.pointerId) } catch { /* 环境不支持指针捕获 */ }
  updateDragPoint(event)
}

/** ghost 跟随 + 落点分区跟踪。 */
function updateDragPoint(event: PointerEvent): void {
  const point = contentPointOf(event)
  dragGhost.value = point
  hoverZone.value = zoneAtPoint(point)
}

/** 拖拽结算：松手即落库（无草稿 / 无应用 / 无撤销）。 */
function applyDrop(id: string, target: DropZone): void {
  const action = dropAction(id, target)
  if (!action) return
  clearNodeError(id)
  dropIntents.set(id, { from: zoneOfNode(id), to: action === 'remove' ? 'removed' : 'member', sawPending: false })
  if (action === 'add') emit('add', id)
  else if (action === 'remove') emit('remove', id)
  else emit('restore', id)
}

/** 复位拖拽态（Esc / pointercancel / 落点后）。 */
function resetGesture(): void {
  gesture = null
  dragging.value = false
  dragGhost.value = null
  dragNodeId.value = null
  hoverZone.value = 'none'
}

// Esc 捕获在 `useLaneCanvasShell` 里（两份画布共用一份）；手势取消回调见上方 options。

// ── 节点表达（unify-lane-workbench D7：pointAppearance 唯一入口） ──────────
function isMember(id: string): boolean {
  return props.memberNodes.some(item => item.id === id)
}
function isRemovedNode(id: string): boolean {
  return props.removedNodes.some(item => item.id === id)
}

function isReadonlyNode(id: string): boolean {
  return props.readonlyNodes.some(item => item.id === id)
}

// ── 相关高亮（D6：焦点的一跳原关系 ∪ 推荐，relatedNodeIds 唯一入口） ─────────
const visibleIds = computed(() => new Set([...props.memberNodes, ...props.candidateNodes, ...props.removedNodes, ...props.readonlyNodes].map(node => node.id)))

const edges = computed(() =>
  props.relations.filter(rel => visibleIds.value.has(String(rel?.fromId ?? '')) && visibleIds.value.has(String(rel?.toId ?? ''))),
)

/** 焦点的相关集合（null = 无焦点 → 默认呈现，不弱化）。 */
const focusRelatedIds = computed<Set<string> | null>(() => {
  if (!props.focusId) return null
  const recommended = props.candidateNodes.map(node => node.id)
  return relatedNodeIds(edges.value, new Set([props.focusId]), recommended)
})

/**
 * 候选桶（候选 + 本次已移出）的**批内相对接近度**（D3 / 5.1）。
 *
 * - 输入 = 候选桶里**估算口径**的接近度（1 − 质心距离）换算回距离；
 * - 归一用 `relativeProximities`（批内 min-max：最近 = 1、最远 = 0，单点/全等距取 0.5）；
 * - **成员与只读参照不进批次**：成员接近度恒为归属快照原值、尺寸固定，不编码接近度；
 * - 无估算口径的节点不进批次（不伪造接近度，维持普通外观）。
 */
const candidateProximity = computed(() => {
  const distances = new Map<string, number>()
  for (const node of [...props.candidateNodes, ...props.removedNodes]) {
    if (node.proximityKind !== 'estimate' || node.proximity === null) continue
    const distance = 1 - node.proximity
    if (!Number.isFinite(distance)) continue
    distances.set(node.id, distance)
  }
  return relativeProximities(distances)
})

/** 统一节点表达：成员固定尺寸、候选按批内相对接近度、只读更弱更小；正交标志叠加。 */
function appearanceOf(node: LaneCorrectionNode): LanePointAppearance {
  const member = isMember(node.id)
  const readonlyFlag = isReadonlyNode(node.id)
  const removedFlag = isRemovedNode(node.id)
  // 候选接近度是估算（1 − 质心距离）；成员/只读不读接近度（快照不驱动大小）。
  const estimate = !member && !readonlyFlag && node.proximityKind === 'estimate' && node.proximity !== null
  const distance = estimate ? 1 - node.proximity! : null
  // 候选桶用**批内相对归一**；成员与只读参照不传 override（尺寸固定）。
  const override = estimate ? candidateProximity.value.get(node.id) : undefined
  const related = focusRelatedIds.value?.has(node.id) ?? false
  return pointAppearance(distance, props.matchThreshold, member, readonlyFlag, BASE_POINT_RADIUS, Math.max(8, columnWidth.value), {
    removed: removedFlag,
    anchor: isAnchorEffective(node.id) || isAnchorChecked(node.id) || props.foldNodes.some(item => item.id === node.id),
    focus: props.focusId === node.id,
    related,
    dimmed: focusRelatedIds.value !== null && !related,
    memberRadius: MEMBER_BASE_RADIUS,
    recommendRange: RECOMMEND_RADIUS_RANGE,
    strengthRange: PROXIMITY_STRENGTH_RANGE,
    proximityOverride: override,
  })
}

/** 统一节点表达表（渲染只读这张表，不在模板里重复算）。 */
const appearances = computed(() => {
  const map = new Map<string, LanePointAppearance>()
  for (const node of [...props.readonlyNodes, ...props.candidateNodes, ...props.removedNodes, ...props.memberNodes]) {
    map.set(node.id, appearanceOf(node))
  }
  return map
})

/** 节点半径（统一 pointAppearance；成员固定、候选接近度、只读弱小）。 */
function radiusOf(node: LaneCorrectionNode): number {
  return appearances.value.get(node.id)?.radius ?? 4
}

/** 逻辑半径（缩小时按像素下限补偿，渲染半径仍 ≥3px）。 */
function logicalRadius(node: LaneCorrectionNode): number {
  return scaledPointRadius(radiusOf(node), zoom.value, 3)
}

/** 成员勾号（已选入 = 实心 + 勾，与「大小=接近度」是两套独立视觉）。 */
function checkPath(node: LaneCorrectionNode): string {
  const point = allPoints.value.get(node.id)
  if (!point) return ''
  const r = radiusOf(node)
  return `M${point.x - r * 0.45} ${point.y} L${point.x - r * 0.1} ${point.y + r * 0.38} L${point.x + r * 0.5} ${point.y - r * 0.38}`
}

/** 移出标记（⊘）。 */
function crossPath(node: LaneCorrectionNode): string {
  const point = allPoints.value.get(node.id)
  if (!point) return ''
  const r = radiusOf(node)
  return `M${point.x - r * 0.5} ${point.y - r * 0.5} L${point.x + r * 0.5} ${point.y + r * 0.5} M${point.x + r * 0.5} ${point.y - r * 0.5} L${point.x - r * 0.5} ${point.y + r * 0.5}`
}

// ── 标题（两行截断 + 冲突宁缺勿叠） ───────────────────────────────────────
function wrapTitle(text: string, perLine: number): string[] {
  const out: string[] = []
  const source = String(text ?? '')
  for (let i = 0; i < source.length && out.length < LABEL_MAX_LINES; i += perLine) {
    out.push(source.slice(i, i + perLine))
  }
  if (source.length > perLine * LABEL_MAX_LINES) {
    out[LABEL_MAX_LINES - 1] = `${out[LABEL_MAX_LINES - 1]!.slice(0, Math.max(1, perLine - 1))}…`
  }
  return out
}

/**
 * 每行可放几个字：按**列宽**算，保证标签宽度不跨列。
 *
 * 这是「标签压邻列节点」的真正根治手段——只做 label↔圆 碰撞检测的话，
 * 标签要么被大面积丢弃（宁缺勿叠过度），要么仍然跨列压字。让标签宽度
 * 永远 ≤ 列宽，它就不可能碰到别的列；列宽被撑大（天数少 / 放大）时
 * 每行字数自然变多，信息量随之恢复。
 */
const labelPerLine = computed(() => {
  const usable = columnWidth.value - 6 // 留 3px/侧 内边距
  // 下限 2 是防御性兜底：columnWidth 恒 ≥ MIN_COLUMN_WIDTH(64)，实际最少得 5，此分支不可达。
  return Math.max(2, Math.min(LABEL_PER_LINE_MAX, Math.floor(usable / LABEL_FONT_SIZE)))
})

interface PlacedLabel {
  id: string
  lines: string[]
  x: number
  y: number
}

/**
 * 标题落位：节点下方两行；与**已放标签**或**其它节点圆**冲突则跳过（宁缺勿叠）。
 *
 * 为什么要防 label↔圆：列宽（尤其是 30 天以上档）往往小于标签宽度，一个标签
 * 会横跨 4–5 列。若只防 label↔label，标签就会直接压在邻列节点的圆上——
 * 这正是「看起来一直很挤」的主因（纵向行距只能解决同一天多成员的堆叠）。
 * 装不下就跳过，列宽被撑大或用户放大后标签自然恢复。
 */
const labels = computed<PlacedLabel[]>(() => {
  const placed: { x1: number, x2: number, y1: number, y2: number }[] = []
  const out: PlacedLabel[] = []
  const all = [...props.memberNodes, ...props.candidateNodes, ...props.removedNodes, ...props.readonlyNodes]
  /** 全部节点圆（含只读/候选），标签只允许压在**自己的**圆下方。 */
  const circles = all
    .map(node => {
      const point = allPoints.value.get(node.id)
      return point ? { id: node.id, x: point.x, y: point.y, r: radiusOf(node) } : null
    })
    .filter((item): item is { id: string, x: number, y: number, r: number } => item !== null)
  const ordered = all.sort((a, b) => {
    const pa = allPoints.value.get(a.id)
    const pb = allPoints.value.get(b.id)
    return (pa?.y ?? 0) - (pb?.y ?? 0) || a.id.localeCompare(b.id)
  })
  for (const node of ordered) {
    const point = allPoints.value.get(node.id)
    if (!point) continue
    const lines = wrapTitle(node.title, labelPerLine.value)
    if (!lines.length) continue
    const width = estimateTextWidth(lines[0] ?? '', LABEL_FONT_SIZE) + 6
    const height = lines.length * LABEL_LINE_H + LABEL_BOX_PAD
    const box = {
      x1: point.x - width / 2,
      x2: point.x + width / 2,
      y1: point.y + radiusOf(node) + LABEL_TOP_GAP,
      y2: point.y + radiusOf(node) + LABEL_TOP_GAP + height,
    }
    if (box.x1 < 0 || box.x2 > vbWidth.value) continue
    if (box.y2 > canvasHeight.value - 8) continue
    const hit = placed.some(other => !(box.x2 < other.x1 || box.x1 > other.x2 || box.y2 < other.y1 || box.y1 > other.y2))
    if (hit) continue
    // 严格相交才算冲突（相切 0px 不压字，避免边界值把正常标签判掉）
    const circleHit = circles.some(c =>
      c.id !== node.id
      && box.x2 > c.x - c.r && box.x1 < c.x + c.r
      && box.y2 > c.y - c.r && box.y1 < c.y + c.r)
    if (circleHit) continue
    placed.push(box)
    out.push({ id: node.id, lines, x: point.x, y: box.y1 })
  }
  return out
})

// ── 节点卡（跟随选中节点；卡上带显式写操作按钮） ─────────────────────────
const cardNode = computed<LaneCorrectionNode | null>(() => {
  const id = props.focusId
  if (!id) return null
  const all = [...props.memberNodes, ...props.candidateNodes, ...props.removedNodes, ...props.readonlyNodes, ...props.foldNodes]
  return all.find(node => node.id === id) ?? null
})

/** 节点卡状态结论（身份，不做教学解释）。 */
const cardStatus = computed(() => {
  const node = cardNode.value
  if (!node) return ''
  if (isMember(node.id)) return '本泳道成员'
  if (isRemovedNode(node.id)) return '已移出本泳道'
  if (isReadonlyNode(node.id)) return '属其它泳道 · 只读'
  if (props.foldNodes.some(item => item.id === node.id)) return '首义节点 · 更早'
  return '未选入本泳道'
})

/**
 * 卡片写操作种类（D8）：成员→移出；候选→加入；已移出→加回；
 * 只读参照 / 折叠带 / 归属未知 → 无写入口（active 不改挂、未知不假定可写）。
 */
const cardWriteKind = computed<'add' | 'remove' | 'restore' | null>(() => {
  const node = cardNode.value
  if (!node) return null
  if (props.foldNodes.some(item => item.id === node.id)) return null
  if (isMember(node.id)) return 'remove'
  if (isRemovedNode(node.id)) return 'restore'
  if (props.candidateNodes.some(item => item.id === node.id)) return 'add'
  return null
})

const cardPending = computed(() => (cardNode.value ? props.pendingIds.includes(cardNode.value.id) : false))

/** 卡片接近度（两把尺子不同名不同形：快照 / 估算）。 */
const cardProximityText = computed(() => {
  const node = cardNode.value
  if (!node || node.proximity === null) return ''
  return node.proximityKind === 'estimate'
    ? `估算接近度 ${node.proximity.toFixed(2)}`
    : `接近度 ${node.proximity.toFixed(2)}（快照）`
})

const cardPosition = computed(() => {
  const node = cardNode.value
  if (!node) return null
  const point = allPoints.value.get(node.id)
  if (!point) return null
  const width = CARD_WIDTH
  const height = cardSize.value.height
  const screenX = point.x * zoom.value
  const screenY = point.y * zoom.value
  const inFold = props.foldNodes.some(item => item.id === node.id)
  let left = inFold ? screenX + 24 : screenX + 16
  let top = inFold ? screenY + 20 : screenY - 12
  if (!inFold && top < 40) top = 40
  if (left + width > containerWidth.value - 8) left = Math.max(8, screenX - width - 16)
  const maxTop = Math.max(4, (containerHeight.value || canvasHeight.value) - height - 8)
  top = Math.max(4, Math.min(maxTop, top))
  return { left, top }
})

watch(() => props.focusId, () => {
  void Promise.resolve().then(() => {
    const el = cardEl.value
    if (el) cardSize.value = { width: CARD_WIDTH, height: el.offsetHeight || cardSize.value.height }
  })
})

// ── 交互 ──────────────────────────────────────────────────────────────────
function onNodeClick(node: LaneCorrectionNode): void {
  if (Date.now() - lastPanAt < 250) return
  // 拖完 250ms 内吞 click（防误触节点卡）。
  if (Date.now() - lastDragAt < 250) return
  emit('focus', node.id)
}

/** 待提交勾选（左栏首义面板勾选、尚未提交）。 */
function isAnchorChecked(id: string): boolean {
  return props.anchorSelection.includes(id)
}

/** 已生效首义（当前首义集里的成员）——与待提交勾选各有独立可辨标记。 */
function isAnchorEffective(id: string): boolean {
  return props.anchorIds.includes(id)
}

/** 折叠带溢出标记（超出上限的节点数）。 */
const foldOverflow = computed(() => Math.max(0, props.foldNodes.length - LANE_FOLD_MAX))

/** 折叠带展示的节点（上限 6 个）。 */
const foldShown = computed(() => props.foldNodes.slice(0, LANE_FOLD_MAX))
</script>

<template>
  <section class="lcc-center" data-testid="lane-correction-center">
    <!-- 画布内确认条（覆盖已有首义时就地确认，不叠弹窗） -->
    <div v-if="confirmVisible" class="lcc-confirm" data-testid="lane-anchor-confirm">
      <p class="lcc-confirm__text">
        当前首义来源：<b>{{ sourceLabel }}</b>。重设后判定基准会改变，继续？
      </p>
      <div class="lcc-confirm__actions">
        <button type="button" class="lcc-confirm__btn" data-testid="lane-anchor-confirm-cancel" @click="emit('cancelApply')">取消</button>
        <button type="button" class="lcc-confirm__btn lcc-confirm__btn--primary" data-testid="lane-anchor-confirm-ok" @click="emit('confirmApply')">继续重设</button>
      </div>
    </div>

    <div
      ref="containerRef"
      class="lcc-surface"
      :class="{ 'is-panning': isPanning, 'is-dragging': dragging }"
      data-testid="lane-correction-canvas"
      aria-label="本泳道成员与候选节点画布"
      @scroll="syncScroll"
      @pointerdown="onSurfacePointerDown"
      @pointermove="onSurfacePointerMove"
      @pointerup="onSurfacePointerUp"
      @pointercancel="onSurfacePointerCancel"
    >
      <!-- 加载骨架 -->
      <svg
        v-if="loading"
        class="lcc-svg"
        :viewBox="`0 0 ${vbWidth} ${canvasHeight}`"
        :width="vbWidth * zoom"
        :height="canvasHeight * zoom"
        data-testid="lane-correction-canvas-svg"
        role="group"
        aria-label="画布加载中"
      >
        <rect class="lcc-skel" :x="axisX0" y="36" rx="6" :width="Math.max(40, vbWidth - axisX0 - RIGHT_PAD)" height="14" />
        <rect class="lcc-skel lcc-skel--2" x="24" y="72" rx="6" width="150" height="9" />
        <rect class="lcc-skel lcc-skel--2" x="24" y="96" rx="6" width="120" height="9" />
        <rect class="lcc-skel lcc-skel--2" x="24" y="120" rx="6" width="140" height="9" />
      </svg>

      <!-- 加载失败：行内错误 + 重试（不呈现业务空态） -->
      <div v-else-if="error" class="lcc-error" role="alert" data-testid="lane-correction-error">
        <b>加载失败</b>
        <span>{{ error }}</span>
        <button type="button" class="lcc-confirm__btn" data-testid="lane-correction-retry" @click="emit('retry')">重试</button>
      </div>

      <svg
        v-else
        class="lcc-svg"
        :viewBox="`0 0 ${vbWidth} ${canvasHeight}`"
        :width="vbWidth * zoom"
        :height="canvasHeight * zoom"
        data-testid="lane-correction-canvas-svg"
        role="group"
        aria-label="本泳道成员、候选与只读参照节点"
      >
        <!--
          日期轴 / 列 guide 的横向裁切框：只允许画在吸顶标签列右侧的可视区
          （左界视口坐标 = 标签列右缘 `axisX0`，右界 = `vbWidth`）；不横向溢出时整段不上场。
        -->
        <defs>
          <clipPath :id="axisClipId" data-role="axis-clip">
            <rect :x="axisClipX" y="0" :width="axisClipWidth" :height="canvasHeight" />
          </clipPath>
        </defs>

        <!-- 日期列 guide（画在最底，不遮任何内容；留在内容层：纵向不吸顶、横向随内容走） -->
        <g class="lcc-axis" data-role="column-guides" :clip-path="axisClipPath">
          <template v-for="(date, index) in dates" :key="date">
            <line
              class="lcc-guide"
              :x1="axisX0 + (index + 0.5) * columnWidth"
              :x2="axisX0 + (index + 0.5) * columnWidth"
              :y1="DATE_AXIS_H"
              :y2="canvasHeight - 16"
            />
          </template>
        </g>

        <!-- 话题带（本泳道一条）：中性基线留在内容层，标签进吸顶列（见本 svg 末尾） -->
        <g class="lcc-band">
          <line class="lcc-track" :x1="axisX0" :x2="vbWidth - RIGHT_PAD" :y1="BAND_Y + 6" :y2="BAND_Y + 6" />
        </g>

        <!-- 分区线（内容层，起点 = 标签列右缘）；分区名进吸顶列 -->
        <g class="lcc-zones">
          <line class="lcc-zone" :x1="axisX0" :x2="vbWidth - RIGHT_PAD" :y1="zones.zone1Line" :y2="zones.zone1Line" />
          <line class="lcc-zone" :x1="axisX0" :x2="vbWidth - RIGHT_PAD" :y1="zones.zone2Line" :y2="zones.zone2Line" />
        </g>

        <!-- 落点分区（拖拽时高亮可落点分区、弱化不可落点区；平时不显现） -->
        <g class="lcc-dropzones">
          <rect
            class="lcc-canvas__dropzone"
            :class="dropZoneClass('member')"
            data-testid="lane-dropzone-member"
            :x="axisX0"
            :y="memberZoneTop"
            :width="Math.max(0, vbWidth - axisX0 - RIGHT_PAD)"
            :height="Math.max(0, memberZoneBottom - memberZoneTop)"
            rx="6"
          />
          <rect
            class="lcc-canvas__dropzone"
            :class="dropZoneClass('candidate')"
            data-testid="lane-dropzone-candidate"
            :x="axisX0"
            :y="candidateZoneTop"
            :width="Math.max(0, vbWidth - axisX0 - RIGHT_PAD)"
            :height="Math.max(0, candidateZoneBottom - candidateZoneTop)"
            rx="6"
          />
          <rect
            class="lcc-canvas__dropzone"
            :class="dropZoneClass('readonly')"
            data-testid="lane-dropzone-readonly"
            :x="axisX0"
            :y="readonlyZoneTop"
            :width="Math.max(0, vbWidth - axisX0 - RIGHT_PAD)"
            :height="Math.max(0, readonlyZoneBottom - readonlyZoneTop)"
            rx="6"
          />
        </g>

        <!-- 节点（先只读参照，再候选与移出，成员在上层；表达统一 pointAppearance） -->
        <g
          v-for="node in [...readonlyNodes, ...candidateNodes, ...removedNodes, ...memberNodes]"
          :key="node.id"
          class="lcc-node"
          :class="{
            'is-member': isMember(node.id),
            'is-removed': isRemovedNode(node.id),
            'is-readonly': isReadonlyNode(node.id),
            'is-focus': focusId === node.id,
            'is-related': appearances.get(node.id)?.related,
            'is-dimmed': appearances.get(node.id)?.dimmed,
            'is-draggable': isDraggable(node.id),
            'is-locked': !isDraggable(node.id),
            'is-pending': pendingIds.includes(node.id),
            'is-error': Boolean(nodeErrors[node.id]),
          }"
          :data-node="node.id"
          :data-testid="`lane-node-${node.id}`"
          :data-proximity="appearances.get(node.id)?.proximity ?? ''"
          :style="`--proximity-fill: ${Math.round(appearances.get(node.id)?.strength ?? 0)}%`"
          tabindex="0"
          role="button"
          :aria-label="`${node.date} · ${node.title}${nodeErrors[node.id] ? ' · ' + nodeErrors[node.id] : ''}`"
          @click="onNodeClick(node)"
        >
          <title>{{ node.title }} · {{ node.date }}{{ nodeErrors[node.id] ? ` · ${nodeErrors[node.id]}` : '' }}</title>
          <circle class="lcc-hit" :cx="allPoints.get(node.id)?.x" :cy="allPoints.get(node.id)?.y" :r="Math.max(9, radiusOf(node) + 5)" />
          <!-- 已生效首义：实线环（与待提交勾选的虚线环可区分） -->
          <circle
            v-if="isAnchorEffective(node.id)"
            class="lcc-anchor-ring lcc-anchor-ring--effective"
            :cx="allPoints.get(node.id)?.x"
            :cy="allPoints.get(node.id)?.y"
            :r="radiusOf(node) + 7"
          />
          <!-- 待提交勾选：虚线环 -->
          <circle
            v-if="isAnchorChecked(node.id)"
            class="lcc-anchor-ring lcc-anchor-ring--pending"
            :cx="allPoints.get(node.id)?.x"
            :cy="allPoints.get(node.id)?.y"
            :r="radiusOf(node) + 7"
          />
          <circle
            class="lcc-point"
            :class="{ 'is-member': isMember(node.id) }"
            :cx="allPoints.get(node.id)?.x"
            :cy="allPoints.get(node.id)?.y"
            :r="logicalRadius(node)"
          />
          <path
            v-if="isMember(node.id)"
            class="lcc-check"
            :d="checkPath(node)"
          />
          <path
            v-if="isRemovedNode(node.id)"
            class="lcc-cross"
            :d="crossPath(node)"
          />
          <circle
            v-if="focusId === node.id"
            class="lcc-focus-ring"
            :cx="allPoints.get(node.id)?.x"
            :cy="allPoints.get(node.id)?.y"
            :r="radiusOf(node) + 4"
          />
        </g>

        <!-- 节点标题（冲突宁缺勿叠） -->
        <g class="lcc-labels">
          <template v-for="label in labels" :key="`label-${label.id}`">
            <text
              v-for="(line, lineIndex) in label.lines"
              :key="`label-${label.id}-${lineIndex}`"
              class="lcc-label"
              :x="label.x"
              :y="label.y + 10 + lineIndex * LABEL_LINE_H"
              text-anchor="middle"
              paint-order="stroke"
              stroke-width="3"
              stroke-linejoin="round"
            >{{ line }}</text>
          </template>
        </g>

        <!--
          横向吸顶标签列（unify-lane-canvas-shell D5）：话题带两行标签 + 两个分区名
          + 「更早」折叠带（含首义范围勾选框）。

          - 宽度取 `axisX0`（150 ↔ 226，随折叠带存在与否），**不取 `LEFT_PAD`**：折叠带里的
            勾选框是可点操作入口，随横滚消失即退化；
          - slot 内图元一律按**内容坐标**书写（`x = 14` / `LEFT_PAD` 起），横向位移由
            `LaneCanvasStickyRail` 外层整组一次性给足（`offset = scrollLeftLogical`）——
            图元自身再补一次位移就会偏移一个滚动量（双重平移）；
          - 画在节点与标题之后：不透明底要盖住滑到标签列下方的点与标题；
          - 不横向溢出时不下发 transform、不画底与阴影（渲染与无吸顶实现一致）。
        -->
        <LaneCanvasStickyRail
          :width="axisX0"
          :overflowing="overflowingX"
          :y="stickyRailAnchor.y"
          :height="stickyRailAnchor.height"
          :offset="scrollLeftLogical"
          tint
          divider
        >
          <text class="lcc-band-title" x="14" :y="BAND_Y + 2">本泳道话题带</text>
          <text class="lcc-band-sub" x="14" :y="BAND_Y + 18">{{ laneLabel }}</text>
          <text class="lcc-zone-label" x="14" :y="zones.zone1Line + 14">候选区</text>
          <text class="lcc-zone-label" x="14" :y="zones.zone1Line + 26">未选入本泳道</text>
          <text class="lcc-zone-label" x="14" :y="zones.zone2Line + 14">只读参照</text>
          <text class="lcc-zone-label" x="14" :y="zones.zone2Line + 26">属其它泳道</text>

          <!-- 「更早」折叠带（窗口外首义节点；非比例折叠）也钉在列内：勾选框是可点入口 -->
          <g v-if="hasFold" class="lcc-fold" data-testid="lane-fold-band">
            <rect class="lcc-fold-bg" :x="LEFT_PAD" y="4" :width="FOLD_W" :height="Math.max(40, zones.zone1Line - 20)" rx="7" />
            <line class="lcc-fold-sep" :x1="LEFT_PAD + FOLD_W + FOLD_GAP / 2" y1="2" :x2="LEFT_PAD + FOLD_W + FOLD_GAP / 2" :y2="zones.zone1Line - 10" />
            <text class="lcc-fold-label" :x="LEFT_PAD + FOLD_W / 2" y="15" text-anchor="middle">更早 · {{ foldNodes.length }}</text>
            <text class="lcc-fold-break" :x="LEFT_PAD + FOLD_W / 2" y="34" text-anchor="middle">⋯</text>
            <g
              v-for="node in foldShown"
              :key="`fold-${node.id}`"
              class="lcc-fold-node"
              :data-node="node.id"
              :data-testid="`lane-fold-node-${node.id}`"
              tabindex="0"
              role="button"
              :aria-label="`更早 · ${node.title}`"
              @click="onNodeClick(node)"
            >
              <title>{{ node.title }} · {{ node.date }} · 首义来源节点</title>
              <circle class="lcc-hit" :cx="allPoints.get(node.id)?.x" :cy="allPoints.get(node.id)?.y" r="11" />
              <circle class="lcc-point is-anchor" :cx="allPoints.get(node.id)?.x" :cy="allPoints.get(node.id)?.y" r="3.5" />
              <circle class="lcc-fold-ring" :cx="allPoints.get(node.id)?.x" :cy="allPoints.get(node.id)?.y" r="8" />
            </g>
            <g
              v-for="node in foldShown"
              :key="`fold-check-${node.id}`"
              class="lcc-fold-check"
              :class="{ 'is-on': isAnchorChecked(node.id) }"
              :data-testid="`lane-fold-check-${node.id}`"
              role="checkbox"
              :aria-checked="isAnchorChecked(node.id)"
              :aria-label="`首义范围：${node.title}`"
              @click.stop="emit('toggleAnchor', node.id)"
            >
              <rect class="lcc-hit" :x="(allPoints.get(node.id)?.x ?? 0) + 10" :y="(allPoints.get(node.id)?.y ?? 0) - 7" width="14" height="14" />
              <rect class="lcc-fold-box" :x="(allPoints.get(node.id)?.x ?? 0) + 12" :y="(allPoints.get(node.id)?.y ?? 0) - 5" width="10" height="10" rx="2" />
              <path
                v-if="isAnchorChecked(node.id)"
                class="lcc-fold-tick"
                :d="`M${(allPoints.get(node.id)?.x ?? 0) + 13.8} ${allPoints.get(node.id)?.y ?? 0} L${(allPoints.get(node.id)?.x ?? 0) + 16.3} ${(allPoints.get(node.id)?.y ?? 0) + 2.6} L${(allPoints.get(node.id)?.x ?? 0) + 20.4} ${(allPoints.get(node.id)?.y ?? 0) - 2.8}`"
              />
            </g>
            <text v-if="foldOverflow > 0" class="lcc-fold-more" :x="LEFT_PAD + FOLD_W / 2" :y="(foldPoints.get(foldShown[foldShown.length - 1]?.id ?? '')?.y ?? 62) + 22" text-anchor="middle">
              +{{ foldOverflow }}
            </text>
          </g>
        </LaneCanvasStickyRail>

        <!--
          日期轴刻度：钉在画布顶部（纵向吸顶），但**不横向吸顶**——刻度必需跟日期列一起
          横向移动，否则就对不上列；画在所有内容之后，并裁切到标签列右侧（`axisClipPath`），
          否则横滚后刻度会滑到吸顶标签列上方与该列文字重叠。

          渐变吸顶带（与编排画布的 `sticky-bg` / `sticky-tint` / `sticky-edge` 同款三件套）：
          刻度文字自己只有 10px 细字，纵滚 40–60px 后话题带与分区线就会滑进 `y ∈ [0, 36]`
          的刻度区，没有不透明底时日期数字直接压在节点圆 / 标题 / 分区线上（不可读）。
          - 底从 `axisX0` 起（**不从 0**）：左侧吸顶标签列在文档顺序里画在这层之前，
            盖上去会把「本泳道话题带」那块的上半截吃掉；
          - `y = -scrollTopLogical` + `height = scrollTopLogical + DATE_AXIS_H`：即
            **在视口坐标里恒盖 `y ∈ [0, DATE_AXIS_H]`**（整组已被上移了 scrollTop）；
          - 三者同受 `overflowing` 门控：不溢出时一个不画，渲染与无吸顶实现一致。
        -->
        <g
          class="lcc-axis-ticks"
          data-role="date-axis"
          :transform="dateAxisTransform"
          :clip-path="axisClipPath"
        >
          <rect
            v-if="overflowing"
            class="lcc-date-axis-bg"
            data-role="date-axis-bg"
            :x="axisX0"
            :y="-scrollTopLogical"
            :width="Math.max(0, vbWidth - axisX0)"
            :height="scrollTopLogical + DATE_AXIS_H"
          />
          <rect
            v-if="overflowing"
            class="lcc-date-axis-tint"
            data-role="date-axis-tint"
            :x="axisX0"
            :y="-scrollTopLogical"
            :width="Math.max(0, vbWidth - axisX0)"
            :height="scrollTopLogical + DATE_AXIS_H"
          />
          <line
            v-if="overflowing"
            class="lcc-date-axis-edge"
            data-role="date-axis-edge"
            :x1="axisX0"
            :x2="vbWidth"
            :y1="DATE_AXIS_H"
            :y2="DATE_AXIS_H"
          />
          <template v-for="(date, index) in dates" :key="`tick-${date}`">
            <text
              v-if="index % Math.max(1, Math.ceil(54 / columnWidth)) === 0"
              class="lcc-text"
              :x="axisX0 + (index + 0.5) * columnWidth"
              y="14"
              text-anchor="middle"
            >{{ date.slice(5) }}</text>
          </template>
        </g>
      </svg>

      <!-- 拖拽 ghost：放大 1.6× 跟随（与编排画布同款视觉语言） -->
      <div
        v-if="dragGhost"
        class="lcc-drag-ghost"
        data-role="drag-ghost"
        :style="{ left: `${dragGhost.x * zoom}px`, top: `${dragGhost.y * zoom}px` }"
      />

      <!-- 节点内联失败播报（不弹窗；错误同时以红描边 + title 呈现在节点上） -->
      <p class="lcc-live" aria-live="assertive" data-testid="lane-node-error-live">{{ lastErrorMessage }}</p>

      <!-- 画布无节点可展示 -->
      <p v-if="!loading && !error && emptyNote" class="lcc-note" data-testid="lane-correction-empty">
        {{ emptyNote }}
      </p>

      <!-- 节点卡：跟随选中节点；卡上显式写操作（加入/移出/加回，当场生效） -->
      <div
        v-if="cardNode"
        ref="cardEl"
        class="lcc-card"
        :style="cardPosition ? { left: `${cardPosition.left}px`, top: `${cardPosition.top}px` } : undefined"
        data-testid="lane-node-card"
      >
        <strong class="lcc-card__title">{{ cardNode.title }}</strong>
        <span class="lcc-card__meta">
          {{ cardNode.date }}<template v-if="cardProximityText"> · {{ cardProximityText }}</template>
        </span>
        <span class="lcc-card__meta">{{ cardStatus }}</span>
        <span class="lcc-card__meta">{{ isAnchorChecked(cardNode.id) ? '首义来源节点' : '非首义来源节点' }}</span>
        <div v-if="cardWriteKind" class="lcc-card__actions">
          <button
            v-if="cardWriteKind === 'add'"
            type="button"
            class="lcc-card__btn lcc-card__btn--add"
            :disabled="cardPending"
            data-testid="lane-card-add"
            @click.stop="emit('add', cardNode.id)"
          >＋ 加入本泳道</button>
          <button
            v-else-if="cardWriteKind === 'remove'"
            type="button"
            class="lcc-card__btn lcc-card__btn--remove"
            :disabled="cardPending"
            data-testid="lane-card-remove"
            @click.stop="emit('remove', cardNode.id)"
          >⊘ 移出本泳道</button>
          <button
            v-else-if="cardWriteKind === 'restore'"
            type="button"
            class="lcc-card__btn lcc-card__btn--add"
            :disabled="cardPending"
            data-testid="lane-card-restore"
            @click.stop="emit('restore', cardNode.id)"
          >↩ 加回本泳道</button>
        </div>
      </div>
    </div>

    <!-- 图例：尺寸图例（小圆—大圆）+ 两种首义标记 + 相关/弱化为正交标志 -->
    <p class="lcc-legend" aria-label="图例">
      <span><i class="lcc-lg lcc-lg--member" />成员</span>
      <span class="lcc-lg-scale"><i class="lcc-lg lcc-lg--scale-sm" /><i class="lcc-lg lcc-lg--scale-lg" />候选大小=接近度</span>
      <span><i class="lcc-lg lcc-lg--anchor-effective" />已生效首义</span>
      <span><i class="lcc-lg lcc-lg--anchor-pending" />待提交勾选</span>
      <span><i class="lcc-lg lcc-lg--out">⊘</i>已移出</span>
      <span><i class="lcc-lg lcc-lg--related" />相关</span>
    </p>

    <!-- 缩放控件 -->
    <div class="lcc-zoom" data-testid="lane-correction-zoom">
      <button type="button" aria-label="缩小" data-testid="lane-zoom-out" @click="changeZoom(0.9)">－</button>
      <span class="lcc-zoom__readout" data-testid="lane-zoom-readout">{{ zoomPercent }}</span>
      <button type="button" aria-label="放大" data-testid="lane-zoom-in" @click="changeZoom(1.1)">＋</button>
      <button type="button" data-testid="lane-zoom-reset" @click="resetZoom">重置</button>
    </div>
  </section>
</template>

<style scoped>
.lcc-center {
  position: relative;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.lcc-confirm {
  flex: none;
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  padding: 8px 12px;
  border-bottom: 1px solid var(--color-border-medium);
  background: var(--color-bg-base);
}

.lcc-confirm__text {
  margin: 0;
  font-size: 12px;
  color: var(--color-text-secondary);
}

.lcc-confirm__text b {
  color: var(--color-text-primary);
}

.lcc-confirm__actions {
  margin-left: auto;
  display: flex;
  gap: 8px;
}

.lcc-confirm__btn {
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-primary);
  font-size: 12px;
  padding: 3px 10px;
  cursor: pointer;
}

.lcc-confirm__btn--primary {
  border-color: var(--color-accent);
  background: var(--color-accent-subtle);
}

.lcc-surface {
  position: relative;
  flex: 1;
  min-height: 0;
  overflow: auto;
  cursor: grab;
  padding: 0;
  scrollbar-gutter: stable;
}

.lcc-surface.is-panning {
  cursor: grabbing;
  user-select: none;
}

/* 拖拽期间：抑制文本选中 + 光标变 grabbing（落点高亮见 .lcc-canvas__dropzone）。 */
.lcc-surface.is-dragging {
  cursor: grabbing;
  user-select: none;
}

.lcc-svg {
  display: block;
}

.lcc-text,
.lcc-guide,
.lcc-zone,
.lcc-track {
  pointer-events: none;
}

.lcc-text {
  fill: var(--color-text-muted);
  font-size: 10px;
}

.lcc-guide {
  stroke: var(--color-border-medium);
  stroke-width: 0.65;
  stroke-dasharray: 2 6;
}

.lcc-zone {
  stroke: var(--color-border-medium);
  stroke-width: 1;
}

.lcc-zone-label {
  fill: var(--color-text-muted);
  font-size: 10px;
}

.lcc-track {
  stroke: var(--color-border-medium);
  stroke-width: 1;
}

.lcc-band-title {
  fill: var(--color-text-primary);
  font-size: 12px;
  font-weight: 600;
}

.lcc-band-sub {
  fill: var(--color-text-muted);
  font-size: 11px;
}

/*
  日期轴吸顶带（与编排画布 `sticky-bg` / `sticky-tint` / `sticky-edge` 同款）。
  不透明底先把滑到顶部的节点与标题整片盖住，提档色才叠得上去。
*/
.lcc-date-axis-bg {
  fill: var(--color-bg-base);
}

/*
  提档底色：必须叠在上面那层 bg-base 之上——elevated 在深色主题带 alpha，
  单独用它当唯一底色会透出滚动内容。
*/
.lcc-date-axis-tint {
  fill: var(--color-bg-elevated);
}

/* 带下缘 1px 硬分隔（与固定列右缘阴影一起表达层界） */
.lcc-date-axis-edge {
  stroke: var(--color-border-medium);
  stroke-width: 1;
}

.lcc-node {
  cursor: pointer;
}

/* 可拖节点：光标 grab；不可拖（只读参照 / 更早折叠带 / 落库中）：default。 */
.lcc-node.is-draggable {
  cursor: grab;
}

.lcc-node.is-locked {
  cursor: default;
}

/* 落库中（pendingIds）：半透明 + 不可再次拖拽（防重入）。 */
.lcc-node.is-pending .lcc-point {
  opacity: 0.45;
}

/* 落库失败：节点内联红描边（不弹窗）。 */
.lcc-node.is-error .lcc-point {
  stroke: var(--color-error);
  stroke-width: 2;
}

.lcc-node.is-error .lcc-check {
  stroke: var(--color-error);
}

.lcc-hit {
  fill: transparent;
  stroke: none;
}

.lcc-point {
  fill: color-mix(in srgb, var(--color-accent) var(--proximity-fill, 0%), transparent);
  stroke: var(--color-accent);
  stroke-width: 1;
}

.lcc-point.is-member {
  fill: var(--color-accent);
}

.lcc-point.is-anchor {
  fill: transparent;
  stroke: var(--color-accent);
  stroke-width: 1.2;
  stroke-dasharray: 3 3;
}

.lcc-check {
  fill: none;
  stroke: var(--color-bg-base);
  stroke-width: 1.4;
  pointer-events: none;
}

.lcc-cross {
  fill: none;
  stroke: var(--color-text-muted);
  stroke-width: 1.2;
  pointer-events: none;
}

.lcc-anchor-ring {
  fill: none;
  stroke: var(--color-accent);
  stroke-width: 1.2;
  stroke-dasharray: 3 3;
  opacity: 0.9;
  pointer-events: none;
}

/* 已生效首义 = 实线环；待提交勾选 = 虚线环（两套标记可区分，D10）。 */
.lcc-anchor-ring--effective {
  stroke-width: 1.8;
  stroke-dasharray: none;
  opacity: 1;
}

/* 取消选中 = 细圈；与「取消勾选 = 移除」（左栏破坏性描边）明确区分。 */
.lcc-focus-ring {
  fill: none;
  stroke: var(--color-text-primary);
  stroke-width: 1;
  pointer-events: none;
}

/* 标题描边光晕（D-1）：节点标记圆（成员勾号圆）压住标题时，文字靠描边保持可读。
   描边色取画布底色 —— `paint-order: stroke`（先描边后填充）让描边只露在字形外侧，
   形成一圈光晕；标题一个不少、落位算法不动。 */
.lcc-label {
  fill: var(--color-text-muted);
  font-size: 11px;
  pointer-events: none;
  stroke: var(--color-bg-base);
}

.lcc-fold-bg {
  fill: var(--color-bg-base);
  stroke: var(--color-border-medium);
  stroke-width: 1;
  stroke-dasharray: 4 4;
}

.lcc-fold-sep {
  stroke: var(--color-border-medium);
  stroke-width: 1;
  stroke-dasharray: 3 4;
}

.lcc-fold-label {
  fill: var(--color-text-primary);
  font-size: 11px;
  font-weight: 600;
}

.lcc-fold-break,
.lcc-fold-more {
  fill: var(--color-text-muted);
  font-size: 11px;
}

.lcc-fold-ring {
  fill: none;
  stroke: var(--color-accent);
  stroke-width: 1.2;
  stroke-dasharray: 3 3;
  opacity: 0.9;
  pointer-events: none;
}

.lcc-fold-box {
  fill: var(--color-bg-base);
  stroke: var(--color-accent);
  stroke-width: 1;
}

.lcc-fold-check.is-on .lcc-fold-box {
  fill: var(--color-accent);
}

.lcc-fold-tick {
  fill: none;
  stroke: var(--color-bg-base);
  stroke-width: 1.4;
  pointer-events: none;
}

.lcc-note {
  position: absolute;
  top: 40%;
  left: 0;
  right: 0;
  margin: 0;
  text-align: center;
  color: var(--color-text-muted);
  font-size: 12px;
  line-height: 1.9;
}

.lcc-error {
  position: absolute;
  top: 24px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  padding: 10px 14px;
  border: 1px solid var(--color-error);
  border-radius: 6px;
  background: var(--color-bg-base);
  color: var(--color-text-primary);
  font-size: 12px;
}

.lcc-card {
  position: absolute;
  width: 264px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 10px;
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  background: var(--color-bg-base);
  box-shadow: 0 6px 18px rgb(0 0 0 / 18%);
  /* 卡体接住自己的点击：点卡不算「点空白画布」（不取消聚焦），长标题也可选中复制；
     画布空白处轻点才取消聚焦（见 onSurfacePointerUp）。 */
  pointer-events: auto;
}

.lcc-card__actions {
  display: flex;
  gap: 6px;
  margin-top: 4px;
}

.lcc-card__btn {
  flex: 1;
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-primary);
  font-size: 12px;
  padding: 3px 8px;
  cursor: pointer;
}

.lcc-card__btn--add {
  border-color: var(--color-accent);
  color: var(--color-accent);
}

/* 移出与「取消聚焦」视觉区分：实心错误色。 */
.lcc-card__btn--remove {
  border-color: var(--color-error);
  background: var(--color-error);
  color: var(--color-bg-base);
  font-weight: 600;
}

.lcc-card__btn:disabled {
  opacity: 0.55;
  cursor: wait;
}

/* 相关高亮 / 弱化（焦点在场时；取消聚焦恢复默认）。 */
.lcc-node.is-related .lcc-point {
  stroke: var(--color-accent);
  stroke-width: 2;
}

.lcc-node.is-related .lcc-label {
  fill: var(--color-text-primary);
}

.lcc-node.is-dimmed {
  opacity: 0.35;
}

/* 标题完整展示：长标题换行、不以省略号截断（卡高随内容实测，见 cardSize）。 */
.lcc-card__title {
  font-size: 12px;
  line-height: 1.45;
  color: var(--color-text-primary);
  white-space: normal;
  overflow-wrap: anywhere;
}

.lcc-card__meta {
  font-size: 11px;
  color: var(--color-text-muted);
}

/* 落点分区：平时不显现，拖拽时高亮可落点分区、弱化不可落点区
   （与编排画布同款 .lcc-canvas__dropzone 视觉语言；scoped 样式各自维护一份）。 */
.lcc-canvas__dropzone {
  fill: color-mix(in srgb, var(--color-accent) 6%, transparent);
  stroke: color-mix(in srgb, var(--color-accent) 62%, transparent);
  stroke-width: 1;
  stroke-dasharray: 5 4;
  pointer-events: none;
  opacity: 0;
}

.lcc-surface.is-dragging .lcc-canvas__dropzone {
  opacity: 1;
}

.lcc-surface.is-dragging .lcc-canvas__dropzone.is-drop-target {
  fill: color-mix(in srgb, var(--color-accent) 14%, transparent);
  stroke-width: 1.5;
  opacity: 1;
}

.lcc-surface.is-dragging .lcc-canvas__dropzone.is-drop-other {
  opacity: 0.3;
}

/* 拖拽 ghost：放大 1.6× 跟随（与编排画布同款）。 */
.lcc-drag-ghost {
  position: absolute;
  width: 12px;
  height: 12px;
  margin: -6px 0 0 -6px;
  border-radius: 50%;
  border: 1.5px solid var(--color-accent);
  background: color-mix(in srgb, var(--color-accent) 30%, var(--color-bg-base));
  transform: scale(1.6);
  opacity: 0.9;
  pointer-events: none;
  z-index: 4;
}

/* 落库失败播报区（屏幕阅读器；视觉错误在节点上）。 */
.lcc-live {
  position: absolute;
  width: 1px;
  height: 1px;
  margin: -1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
  border: 0;
}

.lcc-legend {
  position: absolute;
  left: 12px;
  bottom: 10px;
  display: flex;
  gap: 12px;
  margin: 0;
  padding: 4px 8px;
  border: 1px solid var(--color-border-subtle);
  border-radius: 4px;
  background: var(--color-bg-base);
  font-size: 10px;
  color: var(--color-text-secondary);
}

.lcc-lg {
  display: inline-block;
  width: 10px;
  height: 10px;
  margin-right: 4px;
  vertical-align: -1px;
}

.lcc-lg--member {
  border-radius: 50%;
  background: var(--color-accent);
}

/* 尺寸图例：小圆—大圆（候选大小 = 批内相对接近度）。 */
.lcc-lg-scale {
  display: inline-flex;
  align-items: center;
  gap: 3px;
}

.lcc-lg--scale-sm {
  width: 5px;
  height: 5px;
  margin-right: 0;
  border-radius: 50%;
  border: 1px solid var(--color-accent);
}

.lcc-lg--scale-lg {
  width: 11px;
  height: 11px;
  border-radius: 50%;
  border: 1px solid var(--color-accent);
  background: color-mix(in srgb, var(--color-accent) 60%, transparent);
}

/* 两种首义标记：已生效 = 实线环；待提交勾选 = 虚线环。 */
.lcc-lg--anchor-effective {
  border-radius: 50%;
  border: 1.8px solid var(--color-accent);
}

.lcc-lg--anchor-pending {
  border-radius: 50%;
  border: 1px dashed var(--color-accent);
}

.lcc-lg--related {
  border-radius: 50%;
  border: 2px solid var(--color-accent);
}

.lcc-zoom {
  position: absolute;
  right: 12px;
  bottom: 10px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 3px 8px;
  border: 1px solid var(--color-border-subtle);
  border-radius: 4px;
  background: var(--color-bg-base);
  font-size: 11px;
  color: var(--color-text-secondary);
}

.lcc-zoom button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 26px;
  height: 26px;
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  background: transparent;
  color: inherit;
  font-size: 17px;
  line-height: 1;
  padding: 0 4px;
  cursor: pointer;
}

.lcc-zoom button:hover {
  border-color: var(--color-accent);
  color: var(--color-accent);
}

.lcc-zoom__readout {
  min-width: 36px;
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.lcc-skel {
  fill: var(--color-border-subtle);
}

.lcc-skel--2 {
  fill: var(--color-border-medium);
  opacity: 0.5;
}
</style>
