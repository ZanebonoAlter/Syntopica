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
 * - **相关高亮**（D6）：焦点的一跳原关系 ∪ 推荐（relatedNodeIds 唯一入口）；
 *   其余节点弱化；取消聚焦恢复默认。
 * - **节点表达统一 pointAppearance**（D7）：成员固定尺寸、候选按接近度、只读更弱更小；
 *   已移出划除 / 首义虚线 / 焦点环 / 相关 / 弱化是正交标志。图例「候选大小=接近度」。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  clampZoom,
  estimateTextWidth,
  isDragBeyond,
  panScroll,
  pointAppearance,
  relatedNodeIds,
  scaledPointRadius,
  zoomAnchorScroll,
  zoomStep,
  type LanePointAppearance,
  type LaneRelation,
  LANE_ZOOM_DEFAULT,
} from '~/features/tags/composables/laneComposeLogic'
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
}>(), {
  matchThreshold: 0.3,
  relations: () => [],
  pendingIds: () => [],
})

const emit = defineEmits<{
  /** 点节点 → 只改选中态。 */
  focus: [id: string]
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
/** 成员节点基线。 */
const MEMBER_Y = 86
/** 成员节点半径（本画布固定小尺寸，不随列宽变化）。 */
const MEMBER_BASE_RADIUS = 5.5
/** 标题起笔距节点下沿的间距（引导线与标签共用）。 */
const LABEL_TOP_GAP = 5
/** 节点卡宽度固定（绝对定位卡片量宽会形成塌缩反馈环）。 */
const CARD_WIDTH = 264
/** 标题每行字数 / 最多行数（两行截断，title 属性给全名）。 */
const LABEL_PER_LINE = 10
const LABEL_MAX_LINES = 2
const LABEL_LINE_H = 12
/**
 * 单个节点的纵向总占高：圆直径 + 标签起笔间距 + 标签最多两行。
 * 行距必须≥它，否则上一个节点的标签会压在下一个节点的圆上——
 * 这才是“同一天多节点拥挤”的真因（半径 5.5 直径 11 本身不重叠）。
 */
const NODE_BLOCK_H = MEMBER_BASE_RADIUS * 2 + LABEL_TOP_GAP + LABEL_MAX_LINES * LABEL_LINE_H

const containerRef = ref<HTMLElement | null>(null)
const containerWidth = ref(0)
const containerHeight = ref(0)
const cardEl = ref<HTMLElement | null>(null)
const cardSize = ref({ width: CARD_WIDTH, height: 120 })

const zoom = ref(LANE_ZOOM_DEFAULT)
const zoomPercent = computed(() => `${Math.round(zoom.value * 100)}%`)

function measure(): void {
  const el = containerRef.value
  if (!el) return
  containerWidth.value = el.clientWidth || 0
  containerHeight.value = el.clientHeight || 0
}

let resizeObserver: ResizeObserver | null = null

onMounted(() => {
  measure()
  window.addEventListener('resize', onWindowResize)
  if (typeof ResizeObserver !== 'undefined' && containerRef.value) {
    resizeObserver = new ResizeObserver(() => measure())
    resizeObserver.observe(containerRef.value)
  }
  containerRef.value?.addEventListener('wheel', onWheel, { passive: false })
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onWindowResize)
  resizeObserver?.disconnect()
  resizeObserver = null
  containerRef.value?.removeEventListener('wheel', onWheel)
})

function onWindowResize(): void {
  measure()
}

defineExpose({ measure })

// ── 缩放（只影响呈现，不碰选择 / 成员数 / 首义选择集） ─────────────────────
function onWheel(event: WheelEvent): void {
  event.preventDefault()
  applyZoom(zoomStep(zoom.value, event.deltaY), event.clientX, event.clientY)
}

function applyZoom(next: number, clientX?: number, clientY?: number): void {
  const to = clampZoom(next)
  const from = zoom.value
  if (to === from) return
  const surface = containerRef.value
  if (!surface) {
    zoom.value = to
    return
  }
  const rect = surface.getBoundingClientRect()
  const anchorX = (clientX ?? rect.left + surface.clientWidth / 2) - rect.left
  const anchorY = (clientY ?? rect.top + surface.clientHeight / 2) - rect.top
  const nextLeft = zoomAnchorScroll(anchorX, surface.scrollLeft, from, to)
  const nextTop = zoomAnchorScroll(anchorY, surface.scrollTop, from, to)
  zoom.value = to
  void nextTick().then(() => {
    surface.scrollLeft = nextLeft
    surface.scrollTop = nextTop
  })
}

function changeZoom(delta: number): void {
  applyZoom(zoom.value * delta)
}

function resetZoom(): void {
  zoom.value = LANE_ZOOM_DEFAULT
  const surface = containerRef.value
  if (!surface) return
  void nextTick().then(() => {
    surface.scrollLeft = 0
    surface.scrollTop = 0
  })
}

// ── 拖拽平移（空白处拖；位移超阈值才算拖，不误伤点选） ─────────────────────
let panStart: { x: number, y: number, left: number, top: number } | null = null
let lastPanAt = 0
const isPanning = ref(false)

function onSurfacePointerDown(event: PointerEvent): void {
  const target = event.target as HTMLElement | null
  if (target?.closest?.('.lcc-card, .lcc-zoom, button, .lcc-fold-check')) return
  const surface = containerRef.value
  if (!surface) return
  panStart = { x: event.clientX, y: event.clientY, left: surface.scrollLeft, top: surface.scrollTop }
  isPanning.value = false
}

function onSurfacePointerMove(event: PointerEvent): void {
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

function onSurfacePointerUp(): void {
  if (!panStart) return
  if (isPanning.value) lastPanAt = Date.now()
  panStart = null
  isPanning.value = false
}

// ── 布局（容器真实像素 1:1） ──────────────────────────────────────────────
const vbWidth = computed(() => (containerWidth.value > 0 ? containerWidth.value : FALLBACK_WIDTH))

interface Point { x: number, y: number }

/** 画布可视高度基线（容器高度优先；不足时给最小可视区）。 */
const baseCanvasHeight = computed(() => Math.max(360, containerHeight.value || 360))

/** 折叠带是否存在（当前首义集含窗口外节点）。 */
const hasFold = computed(() => props.foldNodes.length > 0)

/** 时间轴左缘（有折叠带时整体右移一个折叠带宽）。 */
const axisX0 = computed(() => LEFT_PAD + (hasFold.value ? FOLD_W + FOLD_GAP : 0))

const columnWidth = computed(() => Math.max(8, (vbWidth.value - axisX0.value - RIGHT_PAD) / Math.max(1, props.dates.length)))

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

/** 统一节点表达：成员固定尺寸、候选按接近度、只读更弱更小；正交标志叠加。 */
function appearanceOf(node: LaneCorrectionNode): LanePointAppearance {
  const member = isMember(node.id)
  const readonlyFlag = isReadonlyNode(node.id)
  const removedFlag = isRemovedNode(node.id)
  // 候选接近度是估算（1 − 质心距离）；成员/只读不读接近度（快照不驱动大小）。
  const distance = !member && !readonlyFlag && node.proximityKind === 'estimate' && node.proximity !== null
    ? 1 - node.proximity
    : null
  const related = focusRelatedIds.value?.has(node.id) ?? false
  return pointAppearance(distance, props.matchThreshold, member, readonlyFlag, 4, Math.max(8, columnWidth.value), {
    removed: removedFlag,
    anchor: isAnchorChecked(node.id) || props.foldNodes.some(item => item.id === node.id),
    focus: props.focusId === node.id,
    related,
    dimmed: focusRelatedIds.value !== null && !related,
    memberRadius: MEMBER_BASE_RADIUS,
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
function wrapTitle(text: string): string[] {
  const out: string[] = []
  const source = String(text ?? '')
  for (let i = 0; i < source.length && out.length < LABEL_MAX_LINES; i += LABEL_PER_LINE) {
    out.push(source.slice(i, i + LABEL_PER_LINE))
  }
  if (source.length > LABEL_PER_LINE * LABEL_MAX_LINES) {
    out[LABEL_MAX_LINES - 1] = `${out[LABEL_MAX_LINES - 1]!.slice(0, LABEL_PER_LINE - 1)}…`
  }
  return out
}

interface PlacedLabel {
  id: string
  lines: string[]
  x: number
  y: number
}

/**
 * 标题落位：节点下方两行；与**已放标签**冲突则跳过（宁缺勿叠）。
 *
 * 已知边界（review R-5，独立问题未纳入本 change）：本实现只防 label↔label，
 * 不防 label↔节点圆、也不防 label↔节点卡（`.lcc-card` 是不透明 HTML 浮层）。
 * 列宽小于标签宽度时（如 30 天档列宽 ≈24px、标签框 ≈116px），标签仍可能
 * 横向压到邻列节点上。横向避让需另行 design，见 change
 * `fix-lane-row-gap-zoom` 的 review.md R-5。
 */
const labels = computed<PlacedLabel[]>(() => {
  const placed: { x1: number, x2: number, y1: number, y2: number }[] = []
  const out: PlacedLabel[] = []
  const all = [...props.memberNodes, ...props.candidateNodes, ...props.removedNodes, ...props.readonlyNodes]
  const ordered = all.sort((a, b) => {
    const pa = allPoints.value.get(a.id)
    const pb = allPoints.value.get(b.id)
    return (pa?.y ?? 0) - (pb?.y ?? 0) || a.id.localeCompare(b.id)
  })
  for (const node of ordered) {
    const point = allPoints.value.get(node.id)
    if (!point) continue
    const lines = wrapTitle(node.title)
    if (!lines.length) continue
    const width = estimateTextWidth(lines[0] ?? '', 11) + 6
    const height = lines.length * LABEL_LINE_H + 4
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
  emit('focus', node.id)
}

function isAnchorChecked(id: string): boolean {
  return props.anchorSelection.includes(id)
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
      :class="{ 'is-panning': isPanning }"
      data-testid="lane-correction-canvas"
      aria-label="本泳道成员与候选节点画布"
      @pointerdown="onSurfacePointerDown"
      @pointermove="onSurfacePointerMove"
      @pointerup="onSurfacePointerUp"
      @pointercancel="onSurfacePointerUp"
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
        <!-- 日期轴 -->
        <g class="lcc-axis">
          <template v-for="(date, index) in dates" :key="date">
            <line
              class="lcc-guide"
              :x1="axisX0 + (index + 0.5) * columnWidth"
              :x2="axisX0 + (index + 0.5) * columnWidth"
              y1="36"
              :y2="canvasHeight - 16"
            />
            <text
              v-if="index % Math.max(1, Math.ceil(54 / columnWidth)) === 0"
              class="lcc-text"
              :x="axisX0 + (index + 0.5) * columnWidth"
              y="14"
              text-anchor="middle"
            >{{ date.slice(5) }}</text>
          </template>
        </g>

        <!-- 话题带（本泳道一条）+ 活跃段 -->
        <g class="lcc-band">
          <text class="lcc-band-title" x="14" :y="BAND_Y + 2">本泳道话题带</text>
          <text class="lcc-band-sub" x="14" :y="BAND_Y + 18">{{ laneLabel }}</text>
          <line class="lcc-track" :x1="axisX0" :x2="vbWidth - RIGHT_PAD" :y1="BAND_Y + 6" :y2="BAND_Y + 6" />
          <rect
            class="lcc-band-bar"
            :x="axisX0"
            :y="BAND_Y"
            :width="Math.max(28, vbWidth - axisX0 - RIGHT_PAD)"
            height="12"
            rx="6"
          />
          <line
            v-for="node in memberNodes"
            :key="`tick-${node.id}`"
            class="lcc-band-tick"
            :x1="allPoints.get(node.id)?.x"
            :x2="allPoints.get(node.id)?.x"
            :y1="BAND_Y + 12"
            :y2="allPoints.get(node.id)?.y"
          />
        </g>

        <!-- 分区线与分区标签 -->
        <g class="lcc-zones">
          <line class="lcc-zone" :x1="axisX0" :x2="vbWidth - RIGHT_PAD" :y1="zones.zone1Line" :y2="zones.zone1Line" />
          <text class="lcc-zone-label" x="14" :y="zones.zone1Line + 14">候选区</text>
          <text class="lcc-zone-label" x="14" :y="zones.zone1Line + 26">未选入本泳道</text>
          <line class="lcc-zone" :x1="axisX0" :x2="vbWidth - RIGHT_PAD" :y1="zones.zone2Line" :y2="zones.zone2Line" />
          <text class="lcc-zone-label" x="14" :y="zones.zone2Line + 14">只读参照</text>
          <text class="lcc-zone-label" x="14" :y="zones.zone2Line + 26">属其它泳道</text>
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
          }"
          :data-node="node.id"
          :data-testid="`lane-node-${node.id}`"
          :data-proximity="appearances.get(node.id)?.proximity ?? ''"
          :style="`--proximity-fill: ${Math.round(appearances.get(node.id)?.strength ?? 0)}%`"
          tabindex="0"
          role="button"
          :aria-label="`${node.date} · ${node.title}`"
          @click="onNodeClick(node)"
        >
          <title>{{ node.title }} · {{ node.date }}</title>
          <circle class="lcc-hit" :cx="allPoints.get(node.id)?.x" :cy="allPoints.get(node.id)?.y" :r="Math.max(9, radiusOf(node) + 5)" />
          <circle
            v-if="isAnchorChecked(node.id)"
            class="lcc-anchor-ring"
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

        <!-- 「更早」折叠带（窗口外首义节点；非比例折叠） -->
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
      </svg>

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

    <!-- 图例：候选大小=接近度（成员固定尺寸，不编码接近度）；相关/弱化为正交标志 -->
    <p class="lcc-legend" aria-label="图例">
      <span><i class="lcc-lg lcc-lg--member" />成员</span>
      <span><i class="lcc-lg lcc-lg--near" />候选大小=接近度</span>
      <span><i class="lcc-lg lcc-lg--anchor" />虚线=首义</span>
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

.lcc-band-bar {
  fill: var(--color-accent-subtle);
  stroke: var(--color-accent);
  stroke-width: 1;
}

.lcc-band-tick {
  stroke: var(--color-accent);
  stroke-width: 0.8;
  opacity: 0.4;
  stroke-dasharray: 2 3;
}

.lcc-node {
  cursor: pointer;
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
  /* 卡体仍穿透点击（不挡画布）；写操作按钮自己接事件。 */
  pointer-events: none;
}

.lcc-card__actions {
  display: flex;
  gap: 6px;
  margin-top: 4px;
  pointer-events: auto;
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

.lcc-card__title {
  font-size: 12px;
  color: var(--color-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.lcc-card__meta {
  font-size: 11px;
  color: var(--color-text-muted);
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

.lcc-lg--near {
  border-radius: 50%;
  border: 1px solid var(--color-accent);
  background: color-mix(in srgb, var(--color-accent) 40%, transparent);
}

.lcc-lg--anchor {
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
  border: 1px solid var(--color-border-medium);
  border-radius: 3px;
  background: transparent;
  color: inherit;
  font-size: 11px;
  padding: 0 6px;
  cursor: pointer;
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
