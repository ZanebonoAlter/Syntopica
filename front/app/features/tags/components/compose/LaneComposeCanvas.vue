<script setup lang="ts">
/**
 * 编排态中央画布 · SVG 点线工作面（rework-workshop-into-radar-and-desk / workshop-lane-compose）。
 *
 * 口径（契约 §2.3 / §2.4）：
 * - **容器真实像素 1:1**：viewBox 宽 = 容器 clientWidth；窗口尺寸变化后重排重绘；
 * - 默认「话题带」呈现：顶部每个候选话题一行标题，成员以细线指向所属话题；
 *   未归属散点不常显标题，读全文靠悬停浮层；话题多于可读行数时明写「另有 N 个候选话题在左侧候选栏」；
 * - **任何标题 / 编号 / 横条不得越出画布边界**（标题避让用 `placeLabels` 的思路，放不下就少画）；
 * - 普通点半径下限 3px（只读参照 2.5px）；**推荐点半径恒严格小于已选点半径**；
 * - 已有 active 泳道**默认隐藏**，仅用户显式开启才显示且只读；
 * - 节点卡：悬停出卡 → 选中后钉在该点旁并跟随 → 卡内「＋选入/移出」真实改成员并当场翻字
 *   → 卡上 × 或再点同一节点 = 取消选中（卡片收起、右栏线索清空）。
 * - **滚轮缩放**（2026-11-01 用户补充）：0.5× – 3×、以光标为锚点、整体等比缩放；
 *   缩放**只影响呈现**（不碰选择 / 候选资格 / 质心 / 相关节点口径），也不重排日期列。
 *
 * 只读边界：active 泳道的点只读，卡内不给选入动作。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  centroidRecommendations,
  clampZoom,
  estimateTextWidth,
  isDragBeyond,
  panScroll,
  placeLabels,
  placeRelatedTitles,
  pointAppearance,
  relatedNodeIds,
  scaledPointRadius,
  truncateText,
  zoomAnchorScroll,
  zoomStep,
  type LaneLabelCandidate,
  LANE_ZOOM_DEFAULT,
} from '~/features/tags/composables/laneComposeLogic'
import type { ComposeCandidate, ComposeCandidateChain, ComposeRelation } from '~/api/persistentTopics'

const props = defineProps<{
  sections: ComposeCandidate[]
  relations: ComposeRelation[]
  chains: ComposeCandidateChain[]
  selectedIds: Set<string>
  focusId: string | null
  matchThreshold: number
  showAssigned: boolean
  dates: string[]
}>()

const emit = defineEmits<{
  'focus': [id: string]
  'clear-focus': []
  'toggle-member': [id: string, checked: boolean]
}>()

/** 兜底画布宽（容器未测量时用；真实渲染以容器 clientWidth 为准）。 */
const FALLBACK_WIDTH = 900
const BAND_TOP = 20
const BAND_LEFT = 12
const BAND_ROW = 24
const MAX_BAND_LANES = 6
const DATE_AXIS_H = 22
const TOP_PAD = 70
const ACTIVE_PAD = 54
/**
 * 节点卡宽度**固定**：位置计算必须用常量，不能用「量出来的宽度」。
 *
 * 原因（2026-11-01 实机 bug）：绝对定位卡片的宽度是 shrink-to-fit，会被
 * 「容器宽 − left」夹住；而 left 又用「量出来的宽度」算 → **反馈环**：
 * 量到 92px → 位置更靠右 → 可用宽更小 → 量到 60px → … → 塌成一条竖条，
 * 文字一个字一行。所以：宽度固定，量出来的只用高度。
 */
const CARD_WIDTH = 300
/** 相关节点标题：字号、最大字数（缩略）、碰撞盒高度、同时最多标几个。 */
const RELATED_TITLE_FONT = 9
const RELATED_TITLE_MAX_CHARS = 10
/**
 * 碰撞盒高度要覆盖 **渲染出来**的行高（CJK 行盒约 1.55em，9px 字号实测 ~14px），
 * 否则画布上看着不压字、实际字形互相叠在一起。
 */
const RELATED_TITLE_BOX_H = 14
/** 同时最多标几个相关节点（按「1 跳优先、其次离质心近」排序取前 N，其余在提示行里说明）。 */
const RELATED_TITLE_MAX = 12

const containerRef = ref<HTMLElement | null>(null)
const containerWidth = ref(0)
const containerHeight = ref(0)
const hoverId = ref<string | null>(null)
const dismissedId = ref<string | null>(null)
const cardEl = ref<HTMLElement | null>(null)
const cardSize = ref({ width: 260, height: 120 })

function measure(): void {
  const el = containerRef.value
  if (!el) return
  containerWidth.value = el.clientWidth || 0
  containerHeight.value = el.clientHeight || 0
}

let resizeObserver: ResizeObserver | null = null

function onWindowResize(): void {
  measure()
}

onMounted(() => {
  measure()
  window.addEventListener('resize', onWindowResize)
  if (typeof ResizeObserver !== 'undefined' && containerRef.value) {
    resizeObserver = new ResizeObserver(() => measure())
    resizeObserver.observe(containerRef.value)
  }
  // 滚轮缩放：必须非 passive，否则 preventDefault 不生效（画布上滚轮会把整页滚走）
  containerRef.value?.addEventListener('wheel', onWheel, { passive: false })
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onWindowResize)
  resizeObserver?.disconnect()
  resizeObserver = null
  containerRef.value?.removeEventListener('wheel', onWheel)
})

defineExpose({ measure })

// ── 数据视图 ───────────────────────────────────────────────────────────────
const sectionById = computed(() => {
  const map = new Map<string, ComposeCandidate>()
  for (const section of props.sections) map.set(section.id, section)
  return map
})

function isReadonly(section: ComposeCandidate): boolean {
  return section.persistentTopic?.status === 'active'
}

/** 画布上要画的节点：默认只画非 active；显式开启参照才补上 active（只读）。 */
const drawable = computed<ComposeCandidate[]>(() =>
  props.sections.filter(section => props.showAssigned || !isReadonly(section)),
)

const visibleIds = computed(() => new Set(props.sections.map(section => section.id)))

/** 窗口内两端可见的原关系（画布只画这批边，不新增、不推断）。 */
const edges = computed(() =>
  props.relations.filter(rel => visibleIds.value.has(rel.fromId) && visibleIds.value.has(rel.toId)),
)

/** 质心推荐（只用于视觉映射与距离展示；active 只读参照不进优先组）。 */
const recommendations = computed(() =>
  centroidRecommendations(props.sections, props.selectedIds, props.matchThreshold),
)
const recommendedDistance = computed(() => {
  const map = new Map<string, number>()
  for (const item of recommendations.value.unassigned) map.set(item.section.id, item.distance)
  return map
})

// ── 相关节点标题（选中后：相关节点在节点下显示标题缩略） ──────────────────
/**
 * **相关节点** = ① 与已选成员**有原关系**（1 跳）的其它节点 ∪ ② **质心较近**的推荐节点
 * ——② 就是右栏「可补充的事件」里那**同一批**（`centroidRecommendations` 的可补充组，
 * 同一套距离与阈值口径，右栏与画布不各算各的）。
 *
 * - ① 只认服务端透出的原关系（`edges` 已过滤到两端可见），不新增、不推断；
 *   1 跳、不做传递闭包（否则整张图被点亮成一片）；
 * - ② 只取**可补充**的那一组（不含 active 只读参照——只读的那 100 多个既不在右栏、
 *   也不该在画布上抢戏）；
 * - 两条都排除已选成员自己（用户要的是「其他」相关节点）。
 */
const relatedIds = computed(() => {
  const out = relatedNodeIds(edges.value, props.selectedIds)
  for (const item of recommendations.value.unassigned) out.add(item.section.id)
  for (const id of props.selectedIds) out.delete(id)
  return out
})

/**
 * 相关节点分两路：
 * - **可画的相关节点**（非只读）：在**节点下方**显示标题缩略（用户 2026-11-01 补充）；
 * - **只读的相关节点**（属于已有 active 泳道）：只在画布底部的提示行里点名
 *   （它们在只读参照区里是 10px 密排的「梯子」，CJK 行高 13px 放不下逐点标题，
 *   硬放就是压字——宁缺勿叠）。
 */
/** 可画的相关节点：按「1 跳条数多 → 离质心近」排序（用于标题优先级与截断）。 */
const relatedPoolRanked = computed(() => {
  const hops = new Map<string, number>()
  for (const rel of edges.value) {
    if (props.selectedIds.has(rel.fromId) && relatedIds.value.has(rel.toId)) {
      hops.set(rel.toId, (hops.get(rel.toId) ?? 0) + 1)
    } else if (props.selectedIds.has(rel.toId) && relatedIds.value.has(rel.fromId)) {
      hops.set(rel.fromId, (hops.get(rel.fromId) ?? 0) + 1)
    }
  }
  const distanceOf = (id: string): number => recommendedDistance.value.get(id) ?? Number.POSITIVE_INFINITY
  return [...relatedIds.value]
    .filter((id) => {
      const section = sectionById.value.get(id)
      return Boolean(section) && !isReadonly(section!)
    })
    .sort((a, b) => (hops.get(b) ?? 0) - (hops.get(a) ?? 0) || distanceOf(a) - distanceOf(b) || a.localeCompare(b))
    .map(id => ({ id, hops: hops.get(id) ?? 0, distance: distanceOf(id) }))
})

/** 可画相关节点的前 N 个（候选；能不能真标出还要看落位）。 */
const relatedPoolTop = computed(() => relatedPoolRanked.value.slice(0, RELATED_TITLE_MAX))

/** 实际参与标名的可画相关节点（落位冲突/列太窄的会被 placeRelatedTitles 跳过）。 */
const relatedPoolIds = computed(() => relatedPoolTop.value.map(r => r.id))

/** 超出上限、更远的相关节点数量（提示行里说明，不静默丢掉）。 */
const relatedPoolOverflow = computed(() => Math.max(0, relatedPoolRanked.value.length - RELATED_TITLE_MAX))

const relatedReadonlySections = computed(() => {
  const out: ComposeCandidate[] = []
  for (const id of relatedIds.value) {
    const section = sectionById.value.get(id)
    if (section && isReadonly(section)) out.push(section)
  }
  return out
})

/**
 * 相关节点标题的落位：只放「节点下方」一个位置，放不下就宁缺勿叠（跳过），
 * 越出画布边界同样跳过；与已选关系条数多的优先拿到标题。
 */
const relatedTitleItems = computed(() => {
  if (!props.selectedIds.size || !relatedPoolIds.value.length) return []
  // 优先级：1 跳条数多、离质心近的先拿到标题（relatedPoolRanked 已按这个排好序）
  const priorityOf = (id: string): number => {
    const rank = relatedPoolRanked.value.findIndex(r => r.id === id)
    return rank === -1 ? 0 : relatedPoolRanked.value.length - rank
  }
  const cands: LaneLabelCandidate[] = []
  for (const id of relatedPoolIds.value) {
    const section = sectionById.value.get(id)
    const point = points.value.get(id)
    if (!section || !point) continue
    const radius = pointRadius(id)
    // 缩略长度按「到画布两侧的空间」算（标题居中放在节点下），不按日期列宽算——
    // 列宽只有 ~20px 时按列算会把标题缩成 2 字，毫无识别力。
    const room = Math.floor((Math.min(point.x, vbWidth.value - point.x) * 2 - 8) / RELATED_TITLE_FONT)
    const maxChars = Math.max(6, Math.min(RELATED_TITLE_MAX_CHARS, room))
    const text = truncateText(section.clusterLabel, maxChars)
    cands.push({
      id,
      x: point.x,
      y: point.y,
      r: radius,
      w: estimateTextWidth(text, RELATED_TITLE_FONT),
      h: RELATED_TITLE_BOX_H,
      priority: priorityOf(id),
      text,
    })
  }
  return placeRelatedTitles(cands, {
    lines: 2,
    lineHeight: 16,
    gap: 0,
    minX: 0,
    maxX: vbWidth.value,
    minY: 0,
    maxY: canvasHeight.value - 2,
  })
    .filter(label => !label.skipped && label.box)
    .map(label => ({
      id: label.id,
      text: label.text,
      x: label.box!.x + label.box!.w / 2,
      y: label.box!.y + label.box!.h,
    }))
})

/** 提示行：只读相关节点点名（默认隐藏时额外说明在已有泳道里）。 */
const relatedHintText = computed(() => {
  const list = relatedReadonlySections.value
  if (!list.length) return ''
  const names = list.map(section => truncateText(section.clusterLabel, 10)).join('、')
  const where = props.showAssigned ? '在已有泳道里' : '在已有泳道里（默认隐藏）'
  const parts: string[] = []
  if (list.length) parts.push(`相关节点（只读 ${list.length} 个，${where}）：${names}`)
  if (relatedUnlabeledCount.value > 0) {
    parts.push(`另有 ${relatedUnlabeledCount.value} 个可补充的相关节点未标名（列太窄或超出上限，右栏「可补充的事件」里看全部）`)
  }
  return parts.join('；')
})

/** 实际落到画布上的相关标题 id（落位冲突/越界的会被跳过）。 */
const relatedPlacedIds = computed(() => new Set(relatedTitleItems.value.map(item => item.id)))

/**
 * 没标出名字的可补充相关节点数 = 前 N 个里没落位的 + 超出 N 的更远的。
 * 提示行里如实说明，不静默丢掉（用户可去右栏看全部）。
 */
const relatedUnlabeledCount = computed(() => {
  const notPlaced = relatedPoolTop.value.filter(r => !relatedPlacedIds.value.has(r.id)).length
  return notPlaced + relatedPoolOverflow.value
})

// ── 布局（容器真实像素 1:1） ──────────────────────────────────────────────
const vbWidth = computed(() => (containerWidth.value > 0 ? containerWidth.value : FALLBACK_WIDTH))

const dateIndex = computed(() => new Map(props.dates.map((date, index) => [date, index])))

const bandLanes = computed(() => {
  const lanes = props.chains
    .map(chain => ({
      chain,
      members: chain.sectionIds
        .map(id => sectionById.value.get(id))
        .filter((s): s is ComposeCandidate => Boolean(s)),
    }))
    .filter(lane => lane.members.length > 0)
  return lanes
    .sort((a, b) => b.members.length - a.members.length || a.chain.firstDate.localeCompare(b.chain.firstDate))
    .slice(0, MAX_BAND_LANES)
})

const bandHeight = computed(() => (bandLanes.value.length ? bandLanes.value.length * BAND_ROW + 10 : 0))
const dateAxisY = computed(() => bandHeight.value + DATE_AXIS_H)
const leftPad = computed(() => (props.showAssigned ? 150 : 40))
const span = computed(() => (props.showAssigned ? vbWidth.value - leftPad.value - 24 : vbWidth.value - leftPad.value - 40))
const columnWidth = computed(() => span.value / Math.max(1, props.dates.length))
const baseRadius = computed(() => Math.max(3.5, Math.min(10, columnWidth.value * 0.34)))
const dateTick = computed(() => Math.max(1, Math.ceil(props.dates.length / 14)))

const slotCountByDate = computed(() => {
  const counts = new Map<string, number>()
  for (const section of drawable.value) {
    if (isReadonly(section)) continue
    const date = section.periodDate.slice(0, 10)
    counts.set(date, (counts.get(date) ?? 0) + 1)
  }
  return counts
})
const maxSlots = computed(() => Math.max(1, ...slotCountByDate.value.values()))
const rowGap = computed(() => (props.showAssigned ? 36 : Math.max(36, Math.min(110, 360 / Math.max(1, maxSlots.value - 1)))))
const poolTop = computed(() => dateAxisY.value + (props.showAssigned ? ACTIVE_PAD : TOP_PAD))

function xForDate(date: string): number {
  const index = dateIndex.value.get(date) ?? 0
  return leftPad.value + (index + 0.5) * columnWidth.value
}

/** active 泳道行（有参照时画在分隔线之下，只读）。 */
const activeRows = computed(() => {
  const counts = new Map<string, { label: string, count: number }>()
  for (const section of props.sections) {
    if (!isReadonly(section)) continue
    const topicId = section.persistentTopicId ?? ''
    if (!topicId) continue
    const entry = counts.get(topicId) ?? { label: section.persistentTopic?.label ?? '已有泳道', count: 0 }
    entry.count += 1
    counts.set(topicId, entry)
  }
  return [...counts.entries()]
    .map(([topicId, entry]) => ({ topicId, ...entry }))
    .sort((a, b) => a.label.localeCompare(b.label))
})

const poolBottom = computed(() => poolTop.value + (maxSlots.value - 1) * rowGap.value)
const separatorY = computed(() => poolBottom.value + 40)
const activeTop = computed(() => separatorY.value + 55)
const activeRowOffsets = computed(() => {
  const offsets = new Map<string, number>()
  let cursor = activeTop.value
  for (const row of activeRows.value) {
    offsets.set(row.topicId, cursor)
    cursor += Math.max(52, row.count * 10 + 20)
  }
  return { offsets, cursor }
})

/** 节点坐标（非 active 走日期列；active 走只读行）。 */
const points = computed(() => {
  const map = new Map<string, { x: number, y: number }>()
  const slots = new Map<string, number>()
  const activeSlot = new Map<string, number>()
  const ordered = [...drawable.value]
    .sort((a, b) => a.periodDate.localeCompare(b.periodDate) || Number(a.id) - Number(b.id))
  for (const section of ordered) {
    const date = section.periodDate.slice(0, 10)
    const x = xForDate(date)
    if (isReadonly(section)) {
      const topicId = section.persistentTopicId ?? ''
      const k = activeSlot.get(topicId) ?? 0
      activeSlot.set(topicId, k + 1)
      const base = activeRowOffsets.value.offsets.get(topicId) ?? activeTop.value
      map.set(section.id, { x, y: base + k * 10 })
    } else {
      const k = slots.get(date) ?? 0
      slots.set(date, k + 1)
      map.set(section.id, { x, y: poolTop.value + k * rowGap.value })
    }
  }
  return map
})

const canvasHeight = computed(() => (props.showAssigned
  ? Math.max(600, activeRowOffsets.value.cursor + 30)
  : Math.max(430, poolBottom.value + 70)))

/** 每个点的视觉参数（半径 / 接近程度）。 */
const appearances = computed(() => {
  const map = new Map<string, ReturnType<typeof pointAppearance>>()
  for (const section of drawable.value) {
    const readonly = isReadonly(section)
    map.set(section.id, pointAppearance(
      readonly ? undefined : recommendedDistance.value.get(section.id),
      props.matchThreshold,
      props.selectedIds.has(section.id),
      readonly,
      baseRadius.value,
      columnWidth.value,
    ))
  }
  return map
})

// ── 缩放（2026-11-01 用户补充：滚轮放大 / 缩小） ───────────────────────────
/**
 * 缩放**只影响呈现**：不碰选择、候选资格、质心计算、相关节点口径、候选链口径。
 *
 * - 滚轮缩放（普通滚轮与 Ctrl+滚轮同等可用），范围 0.5× – 3×，步进约 1.1；
 * - 以**光标位置为锚点**：缩放后光标下的内容尽量不动（靠滚动位置换算）；
 * - **不重排日期列**：逻辑坐标（viewBox）不变，等比缩放靠 SVG 元素尺寸实现；
 * - 画布上滚轮**不把整页滚走**：preventDefault → 监听器必须 `{ passive: false }`。
 */
const zoom = ref(LANE_ZOOM_DEFAULT)
const zoomPercent = computed(() => `${Math.round(zoom.value * 100)}%`)

function onWheel(event: WheelEvent): void {
  event.preventDefault()
  const clientX = Number.isFinite(event.clientX) ? event.clientX : undefined
  const clientY = Number.isFinite(event.clientY) ? event.clientY : undefined
  applyZoom(zoomStep(zoom.value, event.deltaY), clientX, clientY)
}

/** 应用缩放：改比例 + 按光标锚点换算滚动位置（等 SVG 尺寸更新后再落滚动，避免被旧范围夹掉）。 */
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
  const nextScrollLeft = zoomAnchorScroll(anchorX, surface.scrollLeft, from, to)
  const nextScrollTop = zoomAnchorScroll(anchorY, surface.scrollTop, from, to)
  zoom.value = to
  void nextTick().then(() => {
    surface.scrollLeft = nextScrollLeft
    surface.scrollTop = nextScrollTop
  })
}

/** 重置到 100%：比例与滚动位置一起回到初始态。 */
function resetZoom(): void {
  zoom.value = LANE_ZOOM_DEFAULT
  const surface = containerRef.value
  if (!surface) return
  void nextTick().then(() => {
    surface.scrollLeft = 0
    surface.scrollTop = 0
  })
}

// ── 拖拽平移（按住左键拖画布） ─────────────────────────────────────────
/** 拖拽起点：指针屏幕坐标 + 那一刻的滚动位置。 */
let panStart: { x: number, y: number, left: number, top: number } | null = null
/** 最近一次平移结束的时刻（用来压掉拖拽后的那一下 click）。 */
let lastPanAt = 0
const isPanning = ref(false)

function onSurfacePointerDown(event: PointerEvent): void {
  // 节点卡与控件不参与平移（卡上的按钮要照常点）
  const target = event.target as HTMLElement | null
  if (target?.closest?.('.lcc-card, .lcc-zoom, button')) return
  const surface = containerRef.value
  if (!surface) return
  panStart = { x: event.clientX, y: event.clientY, left: surface.scrollLeft, top: surface.scrollTop }
  isPanning.value = false
  // **不**在这里 setPointerCapture：那会把随后的 pointerup / click 改派到画布，
  // 节点自己的点击（选中、节点卡）就收不到了。指针捕获放到「真的拖起来了」那一刻。
}

function onSurfacePointerMove(event: PointerEvent): void {
  if (!panStart) return
  const surface = containerRef.value
  if (!surface) return
  if (!isPanning.value) {
    if (!isDragBeyond(panStart, { x: event.clientX, y: event.clientY })) return
    // 真的拖起来了：这时才捕获指针（拖出画布也要继续跟）
    isPanning.value = true
    try { surface.setPointerCapture?.(event.pointerId) } catch { /* 某些环境不支持指针捕获，忽略 */ }
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

/**
 * 点的**逻辑半径**（写进 SVG 的值）。
 *
 * 缩小时按**像素下限**补偿：保证渲染半径（逻辑半径 × zoom）仍 ≥3px（只读参照 2.5px）——
 * 不许把点缩到看不见。100% 时原样返回（不改既有视觉）。
 */
function pointRadius(id: string): number {
  const section = sectionById.value.get(id)
  const readonly = section ? isReadonly(section) : false
  const radius = appearances.value.get(id)?.radius ?? baseRadius.value
  return scaledPointRadius(radius, zoom.value, readonly ? 2.5 : 3)
}

/** 节点编号（只读参照不编号；≤45 天才编号）。 */
const numberById = computed(() => {
  const map = new Map<string, string>()
  const pool = drawable.value.filter(section => !isReadonly(section))
  ;[...pool]
    .sort((a, b) => b.periodDate.localeCompare(a.periodDate) || Number(a.id) - Number(b.id))
    .forEach((section, index) => map.set(section.id, String(index + 1).padStart(2, '0')))
  return map
})

const laneNodeIds = computed(() => {
  const ids = new Set<string>()
  for (const chain of props.chains) for (const id of chain.sectionIds) ids.add(id)
  return ids
})

// ── 话题带 ────────────────────────────────────────────────────────────────
interface BandRow {
  key: string
  label: string
  text: string
  /** 标题文本框的左边界（画布像素）。 */
  textLeft: number
  barX: number
  barW: number
  rowY: number
  ticks: { x: number, y: number }[]
}

const bandRows = computed<BandRow[]>(() =>
  bandLanes.value.map((lane, index) => {
    const ps = lane.members
      .map(member => points.value.get(member.id))
      .filter((p): p is { x: number, y: number } => Boolean(p))
    const rowY = BAND_TOP + index * BAND_ROW
    if (ps.length === 0) {
      return { key: lane.chain.key, label: lane.chain.topicLabel, text: '', textLeft: BAND_LEFT, barX: BAND_LEFT, barW: 0, rowY, ticks: [] }
    }
    const xs = ps.map(p => p.x)
    const first = Math.min(...xs)
    const last = Math.max(...xs)
    const barW = Math.max(10, last - first + 10)
    const roomLeft = first - 14
    const fullWidth = estimateTextWidth(lane.chain.topicLabel, 11.5)
    let textLeft = first + 8
    let text = lane.chain.topicLabel
    if (roomLeft > fullWidth) {
      // 左侧放得下：标题贴在横条左侧（右对齐）
      textLeft = roomLeft - fullWidth
    } else if (roomLeft > 80) {
      // 左侧不够：截断后仍贴在横条左侧
      text = truncateText(lane.chain.topicLabel, Math.max(6, Math.floor(roomLeft / 11.5)))
      textLeft = roomLeft - estimateTextWidth(text, 11.5)
    } else {
      // 左侧放不下：标题收进自己的横条并截断（不越出画布）
      text = truncateText(lane.chain.topicLabel, Math.max(6, Math.floor((barW - 16) / 11.5)))
    }
    const radius = pointRadius(lane.members[0]!.id)
    const ticks = ps.map(p => ({ x: p.x, y: p.y - radius }))
    return { key: lane.chain.key, label: lane.chain.topicLabel, text, textLeft, barX: first - 5, barW, rowY, ticks }
  }),
)

/**
 * 话题带标题的避让：走 `placeLabels`（原型口径）——
 * 四个位置都撞上或越出画布就放弃这条标题（宁缺勿叠），并保证不越出右/下边界。
 */
const bandTitles = computed(() => {
  const candidates: LaneLabelCandidate[] = []
  for (const row of bandRows.value) {
    if (!row.text) continue
    const w = estimateTextWidth(row.text, 12)
    candidates.push({
      id: row.key,
      // 以文本框右边缘为参考点：placeLabels 的 'left' 落位还原到既定左边界
      x: row.textLeft + w + 5,
      y: row.rowY + 0.4,
      r: 0,
      w,
      h: 12,
      priority: -row.rowY,
      text: row.text,
    })
  }
  return placeLabels(candidates, {
    orders: ['left'],
    minX: 0,
    maxX: vbWidth.value,
    minY: 0,
    maxY: bandHeight.value + 12,
  })
})

const bandTitleByKey = computed(() => {
  const map = new Map<string, { x: number, y: number }>()
  for (const placed of bandTitles.value) {
    if (placed.skipped || !placed.box) continue
    map.set(placed.id, { x: placed.box.x, y: placed.box.y })
  }
  return map
})

const moreBandCount = computed(() => Math.max(0, props.chains.length - bandLanes.value.length))

// ── 边与节点路径 ──────────────────────────────────────────────────────────
function edgePath(rel: ComposeRelation): string {
  const a = points.value.get(rel.fromId)
  const b = points.value.get(rel.toId)
  if (!a || !b) return ''
  const bend = (rel.relationType === 'identity' ? 0 : 10) + (a.x === b.x ? 16 : 0)
  return `M${a.x} ${a.y} Q${(a.x + b.x) / 2 + bend} ${(a.y + b.y) / 2 - bend} ${b.x} ${b.y}`
}

function checkPath(id: string): string {
  const point = points.value.get(id)
  if (!point) return ''
  const radius = pointRadius(id)
  const w = Math.min(3, radius * 0.48)
  return `M${point.x - w} ${point.y} L${point.x - w * 0.25} ${point.y + w * 0.75} L${point.x + w} ${point.y - w * 0.8}`
}

function showNumber(section: ComposeCandidate): boolean {
  if (isReadonly(section)) return false
  if (props.dates.length > 45) return false
  return laneNodeIds.value.has(section.id)
    || props.selectedIds.has(section.id)
    || props.focusId === section.id
    || (props.dates.length <= 14 && drawable.value.length <= 20)
}

interface NumberBox {
  x: number
  y: number
  anchor: 'start' | 'end' | 'middle'
}

function numberBox(section: ComposeCandidate): NumberBox {
  const point = points.value.get(section.id)
  const radius = pointRadius(section.id)
  if (!point) return { x: 0, y: 0, anchor: 'middle' }
  if (props.dates.length > 14) return { x: point.x, y: point.y - 13, anchor: 'middle' }
  // 贴右边界时翻到点的左边（不越出画布）
  if (point.x + radius + 18 > vbWidth.value) return { x: point.x - radius - 6, y: point.y + 4, anchor: 'end' }
  return { x: point.x + radius + 6, y: point.y + 4, anchor: 'start' }
}

// ── 节点卡 ────────────────────────────────────────────────────────────────
const cardId = computed(() => props.focusId ?? hoverId.value)

const cardSection = computed(() => (cardId.value ? sectionById.value.get(cardId.value) ?? null : null))

const cardDistance = computed(() => {
  const id = cardId.value
  if (!id) return null
  return recommendedDistance.value.get(id) ?? null
})

const cardRelationCount = computed(() => {
  const id = cardId.value
  if (!id) return 0
  let count = 0
  for (const rel of props.relations) {
    if (rel.fromId === id && visibleIds.value.has(rel.toId)) count += 1
    else if (rel.toId === id && visibleIds.value.has(rel.fromId)) count += 1
  }
  return count
})

const cardReadonly = computed(() => (cardSection.value ? isReadonly(cardSection.value) : false))
const cardPicked = computed(() => (cardId.value ? props.selectedIds.has(cardId.value) : false))
const cardArchivedOrigin = computed(() => {
  const section = cardSection.value
  if (!section || section.persistentTopic?.status !== 'archived') return ''
  return section.persistentTopic.label ?? ''
})

const cardPosition = computed(() => {
  const id = cardId.value
  if (!id) return null
  const point = points.value.get(id)
  if (!point) return null
  const radius = pointRadius(id)
  const width = CARD_WIDTH
  const height = cardSize.value.height
  // 卡片是 HTML 浮层：定位用的是**画布像素**，所以必须把缩放比例算进去（否则与节点错位）
  const screenX = point.x * zoom.value
  const screenY = point.y * zoom.value
  const screenRadius = radius * zoom.value
  const right = screenX + screenRadius + 10 + width
  const left = right > containerWidth.value ? screenX - screenRadius - 10 - width : screenX + screenRadius + 10
  const maxTop = Math.max(4, (containerHeight.value || canvasHeight.value) - height - 4)
  const top = Math.max(4, Math.min(maxTop, screenY - height / 2))
  return { left: Math.max(4, left), top }
})

watch(cardId, () => {
  void Promise.resolve().then(() => {
    const el = cardEl.value
    // 只量高度：宽度固定（见 CARD_WIDTH 的注释），量出来的宽度会把卡片喂进塌缩反馈环
    if (el) cardSize.value = { width: CARD_WIDTH, height: el.offsetHeight || cardSize.value.height }
  })
})

function onNodeEnter(section: ComposeCandidate): void {
  if (props.focusId) return
  if (dismissedId.value === section.id) return
  dismissedId.value = null
  hoverId.value = section.id
}

function onNodeLeave(section: ComposeCandidate): void {
  if (dismissedId.value === section.id) dismissedId.value = null
  hoverId.value = null
}

function onNodeClick(section: ComposeCandidate): void {
  // 刚拖完画布的那一下 click 不算「点节点」（否则拖一下就把节点选中了）
  if (Date.now() - lastPanAt < 250) return
  // 再点同一节点 = 取消选中（卡片收起、右栏线索清空）
  if (props.focusId === section.id) dismissedId.value = section.id
  emit('focus', section.id)
}

function closeCard(): void {
  const id = cardId.value
  if (id) dismissedId.value = id
  emit('clear-focus')
}

function toggleCardMember(): void {
  const id = cardId.value
  if (!id || cardReadonly.value) return
  emit('toggle-member', id, !cardPicked.value)
}

// 外部取消聚焦（父组件的「取消聚焦」按钮）→ 卡片整体收起；之后悬停别的点仍可临时读全文
watch(() => props.focusId, (now, prev) => {
  if (!now && prev) hoverId.value = null
})
</script>

<template>
  <section class="lcc-canvas" data-testid="lane-compose-canvas-panel" aria-label="事件关系画布">
    <div
      ref="containerRef"
      class="lcc-canvas__surface"
      :class="{ 'is-panning': isPanning }"
      data-testid="lane-compose-canvas"
      @pointerdown="onSurfacePointerDown"
      @pointermove="onSurfacePointerMove"
      @pointerup="onSurfacePointerUp"
      @pointercancel="onSurfacePointerUp"
    >
      <svg
        :viewBox="`0 0 ${vbWidth} ${canvasHeight}`"
        :width="vbWidth * zoom"
        :height="canvasHeight * zoom"
        data-testid="lane-compose-canvas-svg"
        role="group"
        aria-label="当前可见事件与原关系，已选点带勾，推荐越近越大越深"
      >
        <!-- 日期轴 -->
        <g class="lcc-canvas__axis">
          <template v-for="(date, index) in dates" :key="date">
            <text
              v-if="index % dateTick === 0 || index === dates.length - 1"
              :x="leftPad + (index + 0.5) * columnWidth"
              :y="dateAxisY"
              text-anchor="middle"
            >{{ date.slice(5) }}</text>
            <line
              v-if="index % dateTick === 0 || index === dates.length - 1"
              class="lcc-canvas__guide"
              :x1="leftPad + (index + 0.5) * columnWidth"
              :x2="leftPad + (index + 0.5) * columnWidth"
              :y1="dateAxisY + 12"
              :y2="canvasHeight - 20"
            />
          </template>
        </g>

        <!-- 已有泳道（只读参照，默认隐藏） -->
        <g v-if="showAssigned" class="lcc-canvas__assigned">
          <line class="lcc-canvas__separator" x1="10" :x2="vbWidth - 10" :y1="separatorY" :y2="separatorY" />
          <text x="10" :y="separatorY + 24">已有泳道 · 只读</text>
          <template v-for="row in activeRows" :key="row.topicId">
            <text
              class="lcc-canvas__lane-label"
              :x="10"
              :y="(activeRowOffsets.offsets.get(row.topicId) ?? activeTop) - 2"
            >{{ truncateText(row.label, 10) }}</text>
          </template>
        </g>

        <!-- 原关系边（两端可见才画） -->
        <g class="lcc-canvas__edges">
          <path
            v-for="rel in edges"
            :key="`${rel.fromId}-${rel.toId}-${rel.relationType}`"
            class="lcc-canvas__edge"
            :class="rel.relationType === 'identity' ? 'is-identity' : 'is-similarity'"
            :d="edgePath(rel)"
          />
        </g>

        <!-- 话题带（默认呈现：每候选话题一行标题，成员细线指向所属话题） -->
        <g class="lcc-canvas__band">
          <g v-for="row in bandRows" :key="row.key">
            <title>{{ row.label }}</title>
            <rect class="lcc-canvas__band-bar" :x="row.barX" :y="row.rowY - 6" :width="row.barW" height="12" rx="6" />
            <text
              v-if="bandTitleByKey.get(row.key)"
              class="lcc-canvas__band-title"
              :x="bandTitleByKey.get(row.key)!.x"
              :y="bandTitleByKey.get(row.key)!.y + 9.6"
              text-anchor="start"
            >{{ row.text }}</text>
            <line
              v-for="tick in row.ticks"
              :key="`${row.key}-${tick.x}`"
              class="lcc-canvas__band-tick"
              :x1="tick.x"
              :y1="row.rowY + 7"
              :x2="tick.x"
              :y2="tick.y"
            />
          </g>
          <text v-if="moreBandCount > 0" class="lcc-canvas__band-more" :x="BAND_LEFT" :y="BAND_TOP + (bandRows.length + 1) * BAND_ROW - 4">
            另有 {{ moreBandCount }} 个候选话题在左侧候选栏
          </text>
        </g>

        <!-- 相关节点标题（选中后，与已选有原关系的节点在节点下显示标题缩略） -->
        <g class="lcc-canvas__related">
          <text
            v-for="item in relatedTitleItems"
            :key="item.id"
            class="lcc-canvas__related-title"
            :x="item.x"
            :y="item.y"
            text-anchor="middle"
            :data-testid="`lane-related-title-${item.id}`"
          >{{ item.text }}</text>
        </g>

        <!-- 节点 -->
        <g
          v-for="section in drawable"
          :key="section.id"
          class="lcc-canvas__node"
          :class="{
            'is-selected': selectedIds.has(section.id),
            'is-readonly': isReadonly(section),
            'is-focus': focusId === section.id,
            'is-recommended': recommendedDistance.has(section.id),
          }"
          :data-node="section.id"
          :data-proximity="appearances.get(section.id)?.proximity ?? ''"
          :data-distance="recommendedDistance.get(section.id) ?? ''"
          :style="`--proximity-fill: ${appearances.get(section.id)?.strength ?? 0}%`"
          tabindex="0"
          role="button"
          :aria-label="`${isReadonly(section) ? '已有泳道只读' : selectedIds.has(section.id) ? '已选' : '未选'} · ${section.periodDate.slice(0, 10)} · ${section.clusterLabel}`"
          :data-testid="`lane-node-${section.id}`"
          @mouseenter="onNodeEnter(section)"
          @mouseleave="onNodeLeave(section)"
          @click="onNodeClick(section)"
        >
          <title>{{ section.periodDate.slice(0, 10) }} · {{ section.clusterLabel }}</title>
          <circle
            class="lcc-canvas__hit"
            :cx="points.get(section.id)?.x"
            :cy="points.get(section.id)?.y"
            :r="Math.max(7, pointRadius(section.id) + 4)"
          />
          <circle
            class="lcc-canvas__point"
            :cx="points.get(section.id)?.x"
            :cy="points.get(section.id)?.y"
            :r="pointRadius(section.id)"
          />
          <path v-if="selectedIds.has(section.id)" class="lcc-canvas__check" :d="checkPath(section.id)" />
          <circle
            class="lcc-canvas__focus-ring"
            :cx="points.get(section.id)?.x"
            :cy="points.get(section.id)?.y"
            :r="pointRadius(section.id) + 4"
          />
          <text
            v-if="showNumber(section)"
            class="lcc-canvas__number"
            :x="numberBox(section).x"
            :y="numberBox(section).y"
            :text-anchor="numberBox(section).anchor"
          >{{ numberById.get(section.id) }}</text>
        </g>
      </svg>

      <!-- 节点卡：悬停出卡，选中后钉在该点旁并跟随 -->
      <p v-if="!drawable.length" class="lcc-canvas__empty" data-testid="lane-compose-canvas-empty">
        这个日期范围内没有可画的事件。换个范围，或在右栏「按标题找」里补。
      </p>
      <div
        v-if="cardSection"
        ref="cardEl"
        class="lcc-card"
        :class="{ 'is-pinned': focusId !== null }"
        :style="cardPosition ? { left: `${cardPosition.left}px`, top: `${cardPosition.top}px` } : undefined"
        data-testid="lane-node-card"
      >
        <button type="button" class="lcc-card__close" aria-label="关闭节点卡" data-testid="lane-node-card-close" @click="closeCard">×</button>
        <strong class="lcc-card__title">{{ cardSection.clusterLabel }}</strong>
        <span class="lcc-card__meta">
          {{ cardSection.periodDate.slice(0, 10) }}<template v-if="cardReadonly"> · 已有泳道只读</template><template v-if="cardDistance !== null"> · 距离 {{ cardDistance.toFixed(3) }}</template>
        </span>
        <span v-if="cardArchivedOrigin" class="lcc-card__origin">来自已归档泳道：{{ cardArchivedOrigin }}</span>
        <span class="lcc-card__meta">{{ cardRelationCount ? `${cardRelationCount} 条原关系，画布上已连出` : '暂无原关系，可人工选入' }}</span>
        <button
          v-if="!cardReadonly"
          type="button"
          class="lcc-card__action"
          data-testid="lane-node-card-action"
          @click="toggleCardMember"
        >{{ cardPicked ? '移出本次泳道' : '＋ 选入本次泳道' }}</button>
      </div>
    </div>

    <!-- 相关节点提示行（面板层：不随画布滚动/缩放漂走） -->
    <p v-if="relatedHintText" class="lcc-canvas__related-hint" data-testid="lane-related-hint">
      {{ relatedHintText }}
    </p>

    <!-- 缩放读数与重置（右下角；样式沿用 --color-* token，不新开浮层档位） -->
    <div class="lcc-zoom" data-testid="lane-compose-zoom">
      <span class="lcc-zoom__readout" data-testid="lane-compose-zoom-readout">{{ zoomPercent }}</span>
      <button type="button" class="lcc-zoom__reset" data-testid="lane-compose-zoom-reset" @click="resetZoom">重置</button>
    </div>
  </section>
</template>

<style scoped>
.lcc-canvas {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  /* 缩放控件锚在面板右下角 */
  position: relative;
  background: var(--color-bg-base);
}

.lcc-canvas__surface {
  position: relative;
  flex: 1;
  min-height: 0;
  overflow: auto;
  /* 拖拽平移：空白处可拖（节点本身仍是 pointer） */
  cursor: grab;
  /* 水平不留内边距：clientWidth 即内容宽（画布 1:1 的前提） */
  padding: 0;
  /* 滚动条位置固定：缩放时滚动条出现 / 消失不会挤动容器宽度（日期列不漂移） */
  scrollbar-gutter: stable;
}

.lcc-canvas__surface.is-panning {
  cursor: grabbing;
  user-select: none;
}

.lcc-canvas__surface svg {
  display: block;
}

.lcc-canvas__empty {
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

.lcc-canvas__surface text {
  fill: var(--color-text-muted);
  font-size: 11px;
}

.lcc-canvas__guide {
  stroke: var(--color-border-medium);
  stroke-width: 0.65;
  stroke-dasharray: 2 6;
}

.lcc-canvas__separator {
  stroke: var(--color-border-medium);
  stroke-width: 1;
}

.lcc-canvas__lane-label {
  fill: var(--color-text-muted);
  font-size: 11px;
}

.lcc-canvas__edge {
  fill: none;
  stroke: var(--color-text-muted);
  stroke-width: 1.15;
  opacity: 0.8;
  pointer-events: none;
}

.lcc-canvas__edge.is-similarity {
  stroke-dasharray: 4 4;
}

.lcc-canvas__band-bar {
  fill: var(--color-accent-subtle);
  stroke: var(--color-accent);
  stroke-width: 1;
}

.lcc-canvas__band-title {
  fill: var(--color-text-primary);
  font-size: 12px;
  font-weight: 600;
}

.lcc-canvas__band-tick {
  stroke: var(--color-accent);
  stroke-width: 0.8;
  opacity: 0.35;
  stroke-dasharray: 2 3;
}

.lcc-canvas__band-more {
  fill: var(--color-text-muted);
  font-size: 10px;
}

.lcc-canvas__node {
  cursor: pointer;
}

.lcc-canvas__hit {
  fill: transparent;
  stroke: none;
}

.lcc-canvas__point {
  fill: var(--color-bg-base);
  stroke: var(--color-text-muted);
  stroke-width: 1.5;
}

.lcc-canvas__node.is-readonly .lcc-canvas__point {
  stroke-width: 1.2;
}

.lcc-canvas__node.is-recommended:not(.is-selected) .lcc-canvas__point {
  fill: color-mix(in srgb, var(--color-accent) var(--proximity-fill), var(--color-bg-base));
  stroke: var(--color-accent);
  stroke-width: 1.3;
}

.lcc-canvas__node.is-selected .lcc-canvas__point {
  fill: var(--color-accent);
  stroke: var(--color-accent);
}

.lcc-canvas__check {
  fill: none;
  stroke: var(--color-bg-base);
  stroke-width: 1.6;
  stroke-linecap: round;
  stroke-linejoin: round;
  pointer-events: none;
}

.lcc-canvas__focus-ring {
  fill: none;
  stroke: var(--color-text-primary);
  stroke-width: 1.5;
  opacity: 0;
  pointer-events: none;
}

.lcc-canvas__node.is-focus .lcc-canvas__focus-ring {
  opacity: 1;
}

.lcc-canvas__number {
  fill: var(--color-accent);
  font-family: monospace;
  font-size: 11px;
  pointer-events: none;
}

.lcc-canvas__related-title {
  fill: var(--color-text-secondary);
  font-size: 9px;
  pointer-events: none;
}

.lcc-canvas__related-hint {
  position: absolute;
  left: 12px;
  bottom: 8px;
  /* 右端给缩放控件（读数 + 重置）让位，两边不压字 */
  max-width: calc(100% - 150px);
  margin: 0;
  padding: 4px 8px;
  border-radius: 3px;
  background: color-mix(in srgb, var(--color-bg-base) 88%, transparent);
  font-size: 10px;
  line-height: 1.6;
  color: var(--color-text-muted);
  pointer-events: none;
}

.lcc-card {
  position: absolute;
  z-index: 5;
  /* 宽度固定：绝对定位 + shrink-to-fit 会让「量出来的宽度 → 位置 → 可用宽」塌成竖条 */
  width: 300px;
  /* 高度封顶：内容超高时卡内滚动，不把画布撑出超长竖条 */
  max-height: calc(100% - 12px);
  overflow-y: auto;
  padding: 10px 12px;
  border-radius: 4px;
  background: var(--color-text-primary);
  color: var(--color-bg-base);
  box-shadow: 0 6px 18px rgba(0, 0, 0, 0.22);
  pointer-events: auto;
}

.lcc-card__title {
  display: block;
  /* 给右上角的 × 让位：文字多时标题不能压到关闭按钮下面 */
  padding-right: 18px;
  font-size: 13px;
  line-height: 1.6;
  overflow-wrap: anywhere;
}

.lcc-card__meta,
.lcc-card__origin {
  display: block;
  font-size: 11px;
  opacity: 0.85;
  margin-top: 4px;
  overflow-wrap: anywhere;
}

.lcc-card__action {
  margin-top: 9px;
  font-size: 11px;
  padding: 4px 10px;
  border: 1px solid currentColor;
  background: transparent;
  color: inherit;
  border-radius: 2px;
  cursor: pointer;
}

.lcc-card__close {
  position: absolute;
  top: 2px;
  right: 6px;
  border: 0;
  background: transparent;
  color: inherit;
  opacity: 0.7;
  font-size: 15px;
  line-height: 1;
  padding: 2px 5px;
  cursor: pointer;
}

/* 缩放读数与重置（画布右下角；不手写 z-index——本控件不是浮层） */
.lcc-zoom {
  position: absolute;
  right: 12px;
  bottom: 12px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 3px 6px;
  border: 1px solid var(--color-border-medium);
  border-radius: 3px;
  background: var(--color-bg-base);
  color: var(--color-text-muted);
  font-size: 11px;
  line-height: 1.6;
}

.lcc-zoom__readout {
  min-width: 36px;
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.lcc-zoom__reset {
  padding: 1px 6px;
  border: 1px solid var(--color-border-medium);
  border-radius: 2px;
  background: transparent;
  color: inherit;
  font-size: 11px;
  cursor: pointer;
}
</style>
