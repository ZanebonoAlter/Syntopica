<script setup lang="ts">
/**
 * 关键事件视图（add-key-event-matrix-view design D2/D4/D5/D7/D9/D10/D11）。
 *
 * 矩阵骨架的另一个读数：行 = 持久化话题（active，行序/休眠沿用 `buildObservationRows`
 * 产出、经 Stage 注入；候选话题不占行），列 = 同一 `visibleAxis`，格子换成「这段时间
 * 的关键事件」——confirmed 注记按 `source_period_date` 落节点 + 名称标签（生命线）。
 *
 * 只读视图（design D8）：不挂注记写动作（确认/取消/编辑唯一归属日报阅读台）。
 * 时间视窗与矩阵共用（D9）：本组件不维护第二套跨度，轴/档/观察范围全由 Stage 注入。
 * 行集合只含持久化话题（D2）：候选话题不占行，其确认事件在时间轴与当日阅读区**都**不呈现。
 *
 * 布局（keyEventLayout.ts 纯函数 + 两阶段测量）：
 *  - 列宽 = max(64, 可用宽 / 可见日数)，超宽屏均分填满、窄屏撑宽后舞台内横滚；
 *  - 标签先自然宽度渲染（visibility:hidden 保留布局盒），`offsetWidth` 实测后贪心
 *    分层（层距 44px、同层间隙 ≥4px、右缘贴边左移），**不截断标题**；实测对象
 *    必须是真实 DOM 元素（组件 ref 拿到的是公开实例，量不到宽度）；
 *  - 行体显式宽度 = 轴宽：只含绝对定位子元素时宽度会塌成 0，虚线轴（生命线）
 *    跟着撑不开；
 *  - 节点恒在生命线（轴线）上，同日多条事件靠横向散开区分（MUST NOT 竖向压到别层标签）；
 *  - 标签收起只看**该行层数**，不看观察窗天数（design D7）：层数 ≤ 4 照常显示名称，
 *    超上限（行高封顶 246px）的行才退化为节点 + 当日事件计数（名称由下方阅读区承担）；
 *  - 空白处左键拖拽平移（复用 laneComposeLogic 的 isDragBeyond/panScroll，阈值 5px）；
 *  - 行头 sticky left；选中日整列淡底；切换/换日/**窗口变化**时选中日滚入视口。
 *
 * 下方阅读区（KeyEventDayPanel）：选中日全部 confirmed 事件（标题 + 摘要内联展开 +
 * 关联文章 → 站内阅读 ArticlePreviewModal），事件 → 文章 → 阅读闭环（design D12）。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useBoardKeyEvents } from '~/features/tags/composables/useBoardKeyEvents'
import type { LaneDayReadingStore } from '~/features/tags/composables/useLaneDayReading'
import { isDragBeyond, panScroll } from '~/features/tags/composables/laneComposeLogic'
import type { BoardKeyEventItem } from '~/api/boardKeyEvents'
import type { ObservationLaneRow } from './observationLanes'
import KeyEventLabel from './KeyEventLabel.vue'
import KeyEventDayPanel from './KeyEventDayPanel.vue'
import {
  computeEventColumnWidth,
  computeKeyEventRequestDays,
  keyEventLabelsCollapsed,
  keyEventRowHeight,
  layoutEventLabels,
  nodeFanOffset,
  placementLayerCount,
  KEY_EVENT_NODE_TOP,
  type KeyEventLabelPlacement,
} from './keyEventLayout'

/** 行头宽（px；容纳话题名两行 + 事件计数，与矩阵标签列量级一致）。 */
const HEADER_WIDTH = 192

/** 拖拽平移阈值（px；未超阈值视为点选，design D11）。 */
const DRAG_THRESHOLD = 5

const props = defineProps<{
  boardId: number
  /** 版块名（stale 提示点名来源版块用；发布时快照，不随切版块变）。 */
  boardLabel?: string
  /** 可见观察轴（与矩阵同一 visibleAxis；升序 date-only）。 */
  axis: string[]
  activeRows: ObservationLaneRow[]
  dormantRows: ObservationLaneRow[]
  selectedLaneId: string | null
  selectedDate: string | null
  today: string
  sourceDays: number
  expandedDormant: boolean
  /** A 档（行集合来源）状态：骨架/首错/刷新失败沿用矩阵同一份。 */
  loading: boolean
  error: string | null
  staleNotice: boolean
  emptyState: 'no-reports' | 'no-lanes' | null
  /** 壳层共享读取实例（文章元信息按需读；不自建第二份缓存）。 */
  reading: LaneDayReadingStore
}>()

const emit = defineEmits<{
  selectLane: [topicId: string]
  selectDate: [date: string]
  toggleDormant: []
  retry: []
  openArticle: [articleId: string]
}>()

// ── 事件数据源（board 级批量读；行集合仍来自 A 档 props）──────────────────────

/**
 * 请求窗口：覆盖可见轴起点（观察范围可自由回看，滚动窗口 days 只能取「最近 N 天」，
 * 故 days = max(sourceDays, today − axis.start + 1)）。
 *
 * **MUST NOT 封顶 365**（review R-4）：轴左沿受池边界约束（`[anchor−364, anchor]`，
 * anchor 通常落后今天 N 天），封顶 365 会让轴最左 N 天的事件**静默不返回**（被当成
 * 「无确认事件」渲染）。后端 `days` 接受任意正整数，与池的 365 无关；公式与
 * 「为什么不会无限大」见 `keyEventLayout.ts:computeKeyEventRequestDays`。
 * 窗口外事件由渲染层按 axis 过滤（projectKeyEventDays），多取不渲染。
 */
const requestDays = computed(() => computeKeyEventRequestDays(props.axis, props.today, props.sourceDays))

const boardIdRef = computed(() => props.boardId)
const boardLabelRef = computed(() => props.boardLabel ?? '')
const daysRef = computed(() => requestDays.value)
const {
  eventIndex,
  eventsByDate,
  loading: eventsLoading,
  error: eventsError,
  staleNotice: eventsStale,
  contentSource: eventsSource,
  retry: retryEvents,
} = useBoardKeyEvents({ boardId: boardIdRef, days: daysRef, boardLabel: boardLabelRef })

/**
 * stale 提示点名来源版块（review R-7）：保留的旧数据可能来自**另一个版块**，
 * 只说「之前的数据」会让用户误以为是当前版块的数据。
 */
const staleSourceText = computed(() => {
  const source = eventsSource.value
  if (!source) return '之前的数据'
  const name = source.boardLabel.trim() || `版块 #${source.boardId}`
  return source.boardId === props.boardId
    ? `版块「${name}」之前的数据`
    : `版块「${name}」之前的数据（不是当前版块的数据）`
})

/** 轴集合（窗外日期过滤）。 */
const axisSet = computed(() => new Set(props.axis))

/**
 * 行集合话题 ID（持久化话题：active 行 + 休眠折叠行）。
 * 候选话题不占行（design D2），其**确认事件也不在本视图呈现**（delta spec
 * 「候选话题不占行」+ design 风险项「候选话题事件不可见」）——当日阅读区同样按此
 * 过滤，避免出现“时间轴上没有、阅读区里有”的孤儿事件。
 */
const rowTopicIds = computed(() => new Set([
  ...props.activeRows.map(row => row.topicId),
  ...props.dormantRows.map(row => row.topicId),
]))

/** 当日事件（仅持久化话题的注记；候选/无归属话题的注记不呈现）。 */
const dayEventsByDate = computed(() => {
  const out = new Map<string, BoardKeyEventItem[]>()
  for (const [date, events] of eventsByDate.value) {
    const kept = events.filter(event => event.topicId != null && rowTopicIds.value.has(event.topicId))
    if (kept.length > 0) out.set(date, kept)
  }
  return out
})

/** 行 → 轴内逐日事件（含行头计数与最近事件日）。 */
interface RowProjection {
  row: ObservationLaneRow
  /** 轴内事件总数（行头「N 事件」）。 */
  eventCount: number
  /** 轴内最近事件日（date-only；null = 无）。 */
  lastEventDate: string | null
  /** 轴内日期 → 事件列表（节点与标签渲染源）。 */
  byDate: Map<string, BoardKeyEventItem[]>
}

function projectRow(row: ObservationLaneRow): RowProjection {
  const byDate = new Map<string, BoardKeyEventItem[]>()
  let count = 0
  let last: string | null = null
  const topicDays = eventIndex.value.get(row.topicId)
  if (topicDays) {
    for (const [date, events] of topicDays) {
      if (!axisSet.value.has(date)) continue
      byDate.set(date, events)
      count += events.length
      if (last === null || date > last) last = date
    }
  }
  return { row, eventCount: count, lastEventDate: last, byDate }
}

const activeProjections = computed(() => props.activeRows.map(projectRow))
const dormantProjections = computed(() => props.dormantRows.map(projectRow))
const renderedDormant = computed(() => (props.expandedDormant ? dormantProjections.value : []))

/** 窗口内 confirmed 事件总数（空态判定用；含休眠行）。 */
const windowEventCount = computed(() =>
  [...activeProjections.value, ...dormantProjections.value].reduce((sum, p) => sum + p.eventCount, 0))

// ── 尺寸与两阶段标签布局（keyEventLayout 纯函数 + offsetWidth 实测）──────────

const scrollRef = ref<HTMLElement | null>(null)
const containerWidth = ref(0)
let resizeObserver: ResizeObserver | null = null
/** 已挂 observer 的容器（换容器才重挂，避免重复观察）。 */
let observedSurface: HTMLElement | null = null

/**
 * 舞台容器实测宽：clientWidth 优先（不含滚动条）；为 0（未布局 / 祖先隐藏）时
 * 退回几何盒，再退回父容器宽度。**绝不返回 0 宽度**——0 会把列宽钉死在
 * 64px 下限（实机验收 P2-①：2560 首屏右侧留白 ~830px）。
 */
function measureSurfaceWidth(surface: HTMLElement | null): number {
  if (!surface) return 0
  const byClient = surface.clientWidth
  if (byClient > 0) return byClient
  const rect = typeof surface.getBoundingClientRect === 'function' ? surface.getBoundingClientRect() : null
  const byRect = rect ? rect.width : 0
  if (byRect > 0) return byRect
  const parent = surface.parentElement
  return parent ? parent.clientWidth : 0
}

/** 实测宽落盘（只接受正数）。 */
function applySurfaceWidth(surface: HTMLElement | null): void {
  const measured = measureSurfaceWidth(surface)
  if (measured > 0) containerWidth.value = measured
}

/**
 * 舞台滚动容器的 ref 回调：**容器可能晚于组件出现**（A 档骨架态 → 数据到位后才
 * 挂上），挂上即测量并挂 ResizeObserver。若只在 onMounted 测一次，骨架态下
 * scrollRef 为 null → 既没测到宽也没挂 observer → containerWidth 恒 0 → 列宽
 * 钉在 64px 下限（实机验收 P2-①）。
 *
 * 容器**重建**时（切档骨架态把 `.kev-scroll` 整块卸载、数据到位再挂新的）还要把
 * 选中日拉回视口：滚动位置随旧容器一起丢失，新容器从 scrollLeft=0 起；而重建前后
 * 轴签名与列宽都没变，watch 不再二次触发 → 滚入白做（实机验收 A-8：切 30 天后
 * 选中日在舞台右沿之外）。nextTick 等容器进 DOM、宽可测。
 */
function setScrollRef(el: unknown): void {
  const node = typeof HTMLElement !== 'undefined' && el instanceof HTMLElement ? el : null
  scrollRef.value = node
  if (!node) {
    resizeObserver?.disconnect()
    resizeObserver = null
    observedSurface = null
    return
  }
  if (observedSurface === node) return
  resizeObserver?.disconnect()
  resizeObserver = null
  observedSurface = node
  applySurfaceWidth(node)
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(entries => {
      for (const entry of entries) {
        const target = entry.target as HTMLElement
        const measured = Math.max(entry.contentRect.width, target.clientWidth, measureSurfaceWidth(target))
        if (measured > 0) containerWidth.value = measured
      }
    })
    resizeObserver.observe(node)
  }
  // 容器重建 → 重新把选中日滚入视口（旧容器的滚动位置已随卸载丢失）。
  void nextTick(() => {
    scrollSelectedDateIntoView()
  })
}

const columnWidth = computed(() => computeEventColumnWidth(
  Math.max(0, containerWidth.value - HEADER_WIDTH),
  props.axis.length,
))
const bodyWidth = computed(() => columnWidth.value * props.axis.length)
/** 画布内容宽（≥ 容器宽；列宽被下限撑大时容器横滚）。 */
const innerMinWidth = computed(() => `max(100%, ${HEADER_WIDTH + bodyWidth.value}px)`)

/** 节点中心 x（列中心）。 */
function nodeX(index: number): number {
  return index * columnWidth.value + columnWidth.value / 2
}

/** 同日多节点的横向散开偏移：节点恒在轴线上，同日多条只能横向错开（MUST NOT 竖向压到别层标签）。 */
function nodeFan(eventIndex: number, count: number): number {
  return nodeFanOffset(eventIndex, count)
}

/**
 * 标签 DOM 引用（第一阶段测量源；key = 事件 id）。
 *
 * 组件 ref 回调拿到的是**组件公开实例**（ComponentPublicInstance），不是 DOM 元素；
 * 直接对它量 `offsetWidth` 恒为 undefined → 宽度 0 → 贪心分层退化成「全部第 0 层」
 * → 同一天多个标签互相重叠（实机验收 P1-①）。这里解析出真实根元素
 * （`$el`；KeyEventLabel 的根节点就是标签本体）再交给测量。
 */
const labelEls = new Map<string, HTMLElement>()

/** ref 目标 → 真实 DOM 元素（非 DOM 目标返回 null）。 */
function resolveLabelElement(el: unknown): HTMLElement | null {
  if (!el || typeof HTMLElement === 'undefined') return null
  if (el instanceof HTMLElement) return el
  const inner = (el as { $el?: unknown }).$el
  return inner instanceof HTMLElement ? inner : null
}

function setLabelRef(key: string, el: unknown): void {
  const node = resolveLabelElement(el)
  if (node) labelEls.set(key, node)
  else labelEls.delete(key)
}

/** 标签实测宽（offsetWidth 优先；为 0 时退回几何盒）。 */
function measureLabelWidth(node: HTMLElement | null): number {
  if (!node) return 0
  const byOffset = node.offsetWidth
  if (byOffset > 0) return byOffset
  const rect = typeof node.getBoundingClientRect === 'function' ? node.getBoundingClientRect() : null
  return rect ? rect.width : 0
}

/** 每行布局结果：topicId → { placements, layerCount }。 */
const rowLayouts = ref(new Map<string, { placements: Map<string, KeyEventLabelPlacement>, layers: number }>())
/** 测量阶段标志（布局未完成时标签隐藏，防宽度抖动）。 */
const measuring = ref(true)

/**
 * 标签实测宽度缓存（key = 事件 id）。
 *
 * 收起是按**行**判定的，超限行的标签不再渲染（只剩节点 + ★计数），下一轮重排时现场已量
 * 不到它；只看现场 DOM 会退化成 0 宽 → 1 层 → 又判定没超限而重新展开，收起/展开来回
 * 抖。用上轮实测值兜底，层数判定就只由真实文本宽度驱动。
 */
const measuredLabelWidths = new Map<string, number>()

/** 渲染中的标签清单（含行归属；布局输入）。 */
interface PendingLabel {
  key: string
  topicId: string
  x: number
}

const pendingLabels = computed<PendingLabel[]>(() => {
  const out: PendingLabel[] = []
  props.axis.forEach((date, index) => {
    for (const projection of [...activeProjections.value, ...renderedDormant.value]) {
      const events = projection.byDate.get(date)
      if (!events) continue
      events.forEach((event, evIdx) => {
        out.push({
          key: event.id,
          topicId: projection.row.topicId,
          x: nodeX(index) + nodeFan(evIdx, events.length),
        })
      })
    }
  })
  return out
})

/** 标签宽度：现场实测优先（并回写缓存），收起行（标签未渲染）用上轮实测值。 */
function labelWidthOf(key: string): number {
  const live = measureLabelWidth(labelEls.get(key) ?? null)
  if (live > 0) {
    measuredLabelWidths.set(key, live)
    return live
  }
  return measuredLabelWidths.get(key) ?? 0
}

/** 第二阶段：实测宽度 → 贪心分层（行内独立布局；行高随之确定）。 */
async function runLabelLayout(): Promise<void> {
  if (pendingLabels.value.length === 0) {
    rowLayouts.value = new Map()
    measuring.value = false
    return
  }
  measuring.value = true
  await nextTick()
  const byTopic = new Map<string, { placements: Map<string, KeyEventLabelPlacement>, layers: number }>()
  const topics = new Set(pendingLabels.value.map(item => item.topicId))
  for (const topicId of topics) {
    const items = pendingLabels.value
      .filter(item => item.topicId === topicId)
      .map(item => ({ key: item.key, x: item.x, width: labelWidthOf(item.key) }))
    const placements = layoutEventLabels(items, bodyWidth.value)
    const map = new Map<string, KeyEventLabelPlacement>()
    for (const p of placements) map.set(p.key, p)
    byTopic.set(topicId, { placements: map, layers: placementLayerCount(placements) })
  }
  rowLayouts.value = byTopic
  measuring.value = false
}

watch([pendingLabels, bodyWidth], () => { void runLabelLayout() }, { deep: false })

function rowHeight(topicId: string): number {
  return keyEventRowHeight(rowLayouts.value.get(topicId)?.layers ?? 0)
}

/** 该行标签层数是否超上限（超了才退化，design D7）；未测量时为 false = 乐观先显示标签。 */
function rowCollapsed(topicId: string): boolean {
  return keyEventLabelsCollapsed(rowLayouts.value.get(topicId)?.layers ?? 0)
}

function placementOf(topicId: string, key: string): KeyEventLabelPlacement | null {
  return rowLayouts.value.get(topicId)?.placements.get(key) ?? null
}

/** 节点 top：节点恒在生命线（轴线）上；收起行没有标签，同日多条竖向叠放避让 ★ 计数。 */
function nodeTop(topicId: string, eventIndex: number): number {
  if (rowCollapsed(topicId)) return KEY_EVENT_NODE_TOP + eventIndex * 20
  return KEY_EVENT_NODE_TOP
}

/** 节点样式：top + 横向散开（translateX 覆盖居中位移）。 */
function nodeStyle(topicId: string, eventIndex: number, count: number): Record<string, string> {
  return {
    top: `${nodeTop(topicId, eventIndex)}px`,
    transform: `translateX(calc(-50% + ${nodeFan(eventIndex, count)}px))`,
  }
}

// ── 拖拽平移（空白处左键拖；位移 >5px 才算拖，不误伤点选；design D11）─────────

let panStart: { x: number, y: number, left: number, top: number } | null = null
const isPanning = ref(false)

function onSurfacePointerDown(event: PointerEvent): void {
  const target = event.target as HTMLElement | null
  if (target?.closest?.('button, .kev-label, .kev-row-head')) return
  const surface = scrollRef.value
  if (!surface) return
  panStart = { x: event.clientX, y: event.clientY, left: surface.scrollLeft, top: surface.scrollTop }
  isPanning.value = false
}

function onSurfacePointerMove(event: PointerEvent): void {
  if (!panStart) return
  const surface = scrollRef.value
  if (!surface) return
  if (!isPanning.value) {
    if (!isDragBeyond(panStart, { x: event.clientX, y: event.clientY }, DRAG_THRESHOLD)) return
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
  panStart = null
  isPanning.value = false
}

// ── 选中日滚入视口（切换/换日/窗口变化时；design D10）─────────────────────────

function scrollSelectedDateIntoView(): void {
  const surface = scrollRef.value
  if (!surface || !props.selectedDate) return
  const index = props.axis.indexOf(props.selectedDate)
  if (index === -1) return
  const target = HEADER_WIDTH + index * columnWidth.value + columnWidth.value / 2
  surface.scrollTo({ left: Math.max(0, target - surface.clientWidth / 2), behavior: 'smooth' })
}

/**
 * 轴签名（长度 + 首尾）：观察窗切换（14 天 → 30 天）时轴变了而选中日没变，
 * 只 watch selectedDate 不会把选中日拉回视口（实机验收 P2-②）。用签名而非数组
 * 引用，避免父组件每次渲染换新数组导致反复滚动（抖动）。
 */
const axisSignature = computed(() => {
  const axis = props.axis
  return `${axis.length}:${axis[0] ?? ''}:${axis[axis.length - 1] ?? ''}`
})

// 选中日 / 轴（窗口变化）/ 列宽（容器变化）三者任一变化都把选中日拉回视口。
watch([() => props.selectedDate, axisSignature, columnWidth], () => {
  scrollSelectedDateIntoView()
})

// ── 生命周期：容器测量 + 卸载清理 ──────────────────────────────────────────

onMounted(async () => {
  // 容器若在挂载后才出现（骨架态），setScrollRef 会补测并补挂 observer。
  await nextTick()
  applySurfaceWidth(scrollRef.value)
  void runLabelLayout().then(() => scrollSelectedDateIntoView())
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  resizeObserver = null
  observedSurface = null
  panStart = null
})

// ── 状态分层（A 档骨架/首错沿用矩阵；事件层三态独立）────────────────────────

const showSkeleton = computed(() => props.loading && props.activeRows.length === 0 && props.dormantRows.length === 0)
const showLaneError = computed(() =>
  !props.loading && props.error !== null && props.emptyState === null
  && props.activeRows.length === 0 && props.dormantRows.length === 0)
const eventsFirstError = computed(() =>
  !eventsLoading.value && eventsError.value !== null && eventIndex.value.size === 0)

function shortDate(date: string): string {
  return date.slice(5).replace('-', '/')
}
</script>

<template>
  <section class="kev" data-testid="key-event-view">
    <div class="kev-head">
      <span class="kev-kicker">话题 × 关键事件</span>
      <span class="kev-hint" data-testid="key-event-legend">节点 = 人工确认的关键事件（在日报阅读台标记）· 只读视图</span>
      <button
        v-if="selectedDate"
        type="button"
        class="kev-goto"
        data-testid="key-event-goto-selected"
        :title="`查看下方 ${selectedDate} 的事件`"
        @click="scrollSelectedDateIntoView()"
      >所选日居中</button>
    </div>

    <!-- 事件层刷新失败但保留旧内容（stale；MUST NOT 冒称 0 条） -->
    <div v-if="eventsStale && eventsError" class="kev-stale" data-testid="key-event-stale-notice">
      <Icon icon="mdi:alert-circle-outline" width="14" />
      <span>事件刷新失败（{{ eventsError }}），当前显示的是{{ staleSourceText }}</span>
      <button type="button" class="kev-link" data-testid="key-event-stale-retry" @click="retryEvents">重试</button>
    </div>

    <!-- A 档骨架（行集合未到） -->
    <div v-if="showSkeleton" class="kev-skeleton" data-testid="key-event-skeleton">
      <div v-for="i in 5" :key="i" class="kev-skeleton-row" />
    </div>

    <!-- A 档首错（行集合都取不到；事件无从谈起） -->
    <div v-else-if="showLaneError" class="kev-error" data-testid="key-event-lane-error">
      <Icon icon="mdi:alert-circle-outline" width="16" />
      <span>{{ error }}</span>
      <button type="button" class="kev-link" data-testid="key-event-lane-retry" @click="emit('retry')">重试</button>
    </div>

    <!-- 事件层首错（行集合正常但事件批量读失败；MUST NOT 用空轴冒充失败） -->
    <div v-else-if="eventsFirstError" class="kev-error" data-testid="key-event-error">
      <Icon icon="mdi:alert-circle-outline" width="16" />
      <span>关键事件读取失败（{{ eventsError }}）</span>
      <button type="button" class="kev-link" data-testid="key-event-retry" @click="retryEvents">
        {{ eventsLoading ? '重试中…' : '重试' }}
      </button>
    </div>

    <template v-else>
      <!-- A 档 stale：沿用矩阵口径（保留内容 + 提示） -->
      <div v-if="staleNotice && error" class="kev-stale" data-testid="key-event-lane-stale">
        <Icon icon="mdi:alert-circle-outline" width="14" />
        <span>泳道数据刷新失败（{{ error }}），行序保持上次内容</span>
        <button type="button" class="kev-link" @click="emit('retry')">重试</button>
      </div>

      <!-- 空态分层：无日报 / 无持久化话题 / 0 条 confirmed（行与轴照常渲染） -->
      <div v-if="emptyState === 'no-reports'" class="kev-inline" data-testid="key-event-empty-no-reports">
        <span class="kev-inline-text">该版块还没有日报；生成第一份日报并在阅读台标记关键事件后，这里会出现它们的时间线。</span>
      </div>
      <div v-else-if="emptyState === 'no-lanes'" class="kev-inline" data-testid="key-event-empty-no-lanes">
        <span class="kev-inline-text">该版块暂无持久化话题</span>
      </div>
      <div v-else-if="windowEventCount === 0 && !eventsLoading" class="kev-inline" data-testid="key-event-empty-window">
        <span class="kev-inline-text">这段时间没有确认的关键事件 · 在日报阅读台标记</span>
      </div>

      <!-- 事件层加载中（行集合已到）：轴照画 + 轻提示，不清空 -->
      <div v-if="eventsLoading && windowEventCount === 0" class="kev-loading" data-testid="key-event-loading">
        正在读取关键事件…
      </div>

      <div
        :ref="setScrollRef"
        class="kev-scroll"
        :class="{ 'is-panning': isPanning }"
        data-testid="key-event-scroll"
        @pointerdown="onSurfacePointerDown"
        @pointermove="onSurfacePointerMove"
        @pointerup="onSurfacePointerUp"
        @pointercancel="onSurfacePointerUp"
      >
        <div class="kev-inner" :style="{ minWidth: innerMinWidth }" data-testid="key-event-grid">
          <!-- 日期轴头（与矩阵同一 visibleAxis 逐列对齐；行头位置留白） -->
          <div class="kev-day-heads" :style="{ '--kev-col': `${columnWidth}px` }">
            <div class="kev-head-spacer" />
            <div
              v-for="(date, index) in axis"
              :key="date"
              class="kev-day"
              :class="{ 'is-today': date === today, 'is-selected': date === selectedDate }"
              :data-testid="`key-event-day-${date}`"
              :title="date"
            >
              <span class="kev-day-text">{{ shortDate(date) }}</span>
            </div>
          </div>

          <!-- 活跃行 -->
          <div
            v-for="projection in activeProjections"
            :key="projection.row.topicId"
            class="kev-row"
            :data-testid="`key-event-row-${projection.row.topicId}`"
          >
            <button
              type="button"
              class="kev-row-head"
              :class="{ 'is-selected': projection.row.topicId === selectedLaneId }"
              @click="emit('selectLane', projection.row.topicId)"
            >
              <span class="kev-row-name">{{ projection.row.label }}</span>
              <span class="kev-row-meta">
                {{ projection.eventCount > 0 ? `${projection.eventCount} 事件${projection.lastEventDate ? ` · 最近 ${shortDate(projection.lastEventDate)}` : ''}` : '无确认事件' }}
              </span>
            </button>
            <div class="kev-row-body" :style="{ height: `${rowHeight(projection.row.topicId)}px`, width: `${bodyWidth}px` }">
              <div class="kev-axis-line" aria-hidden="true" />
              <template v-for="(date, index) in axis" :key="date">
                <div
                  class="kev-cell"
                  :class="{ 'is-col-selected': date === selectedDate }"
                  :style="{ left: `${index * columnWidth}px`, width: `${columnWidth}px` }"
                >
                  <button
                    v-for="(ev, evIdx) in projection.byDate.get(date) ?? []"
                    :key="ev.id"
                    type="button"
                    class="kev-node"
                    :data-testid="`key-event-node-${projection.row.topicId}-${date}`"
                    :data-date="date"
                    :style="nodeStyle(projection.row.topicId, evIdx, projection.byDate.get(date)!.length)"
                    :title="`${date} · ${ev.titleSnapshot}`"
                    @click.stop="emit('selectDate', date)"
                  >
                    <template v-if="rowCollapsed(projection.row.topicId) && evIdx === 0">★{{ projection.byDate.get(date)!.length }}</template>
                  </button>
                </div>
              </template>
              <template v-if="!rowCollapsed(projection.row.topicId)">
                <KeyEventLabel
                  v-for="event in axis.flatMap(date => projection.byDate.get(date) ?? [])"
                  :key="event.id"
                  :ref="el => setLabelRef(event.id, el)"
                  :event="event"
                  :left="placementOf(projection.row.topicId, event.id)?.left ?? nodeX(axis.indexOf(event.sourcePeriodDate))"
                  :top="placementOf(projection.row.topicId, event.id)?.top ?? 46"
                  :measuring="measuring || placementOf(projection.row.topicId, event.id) === null"
                  @select="date => emit('selectDate', date)"
                />
              </template>
            </div>
          </div>

          <!-- 休眠组（沿用矩阵休眠判定与折叠交互；0 行保留同级 summary） -->
          <div class="kev-group-summary" data-testid="key-event-dormant-toggle">
            <button
              type="button"
              class="kev-group-btn"
              :aria-expanded="expandedDormant"
              :disabled="dormantRows.length === 0"
              @click="emit('toggleDormant')"
            >
              <Icon :icon="expandedDormant ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="14" />
              <span class="kev-group-title">休眠泳道<span class="kev-group-count"> · {{ dormantRows.length }}</span></span>
              <span class="kev-group-note">
                <template v-if="dormantRows.length === 0">本窗无</template>
                <template v-else>{{ expandedDormant ? '收起' : '展开查看' }}</template>
              </span>
            </button>
          </div>

          <div
            v-for="projection in renderedDormant"
            :key="projection.row.topicId"
            class="kev-row is-dormant"
            :data-testid="`key-event-row-${projection.row.topicId}`"
          >
            <button
              type="button"
              class="kev-row-head"
              :class="{ 'is-selected': projection.row.topicId === selectedLaneId }"
              @click="emit('selectLane', projection.row.topicId)"
            >
              <span class="kev-row-name">{{ projection.row.label }}</span>
              <span class="kev-row-meta">
                {{ projection.eventCount > 0 ? `${projection.eventCount} 事件${projection.lastEventDate ? ` · 最近 ${shortDate(projection.lastEventDate)}` : ''}` : '无确认事件' }}
              </span>
            </button>
            <div class="kev-row-body" :style="{ height: `${rowHeight(projection.row.topicId)}px`, width: `${bodyWidth}px` }">
              <div class="kev-axis-line" aria-hidden="true" />
              <template v-for="(date, index) in axis" :key="date">
                <div
                  class="kev-cell"
                  :class="{ 'is-col-selected': date === selectedDate }"
                  :style="{ left: `${index * columnWidth}px`, width: `${columnWidth}px` }"
                >
                  <button
                    v-for="(ev, evIdx) in projection.byDate.get(date) ?? []"
                    :key="ev.id"
                    type="button"
                    class="kev-node"
                    :data-testid="`key-event-node-${projection.row.topicId}-${date}`"
                    :data-date="date"
                    :style="nodeStyle(projection.row.topicId, evIdx, projection.byDate.get(date)!.length)"
                    :title="`${date} · ${ev.titleSnapshot}`"
                    @click.stop="emit('selectDate', date)"
                  >
                    <template v-if="rowCollapsed(projection.row.topicId) && evIdx === 0">★{{ projection.byDate.get(date)!.length }}</template>
                  </button>
                </div>
              </template>
              <template v-if="!rowCollapsed(projection.row.topicId)">
                <KeyEventLabel
                  v-for="event in axis.flatMap(date => projection.byDate.get(date) ?? [])"
                  :key="event.id"
                  :ref="el => setLabelRef(event.id, el)"
                  :event="event"
                  :left="placementOf(projection.row.topicId, event.id)?.left ?? nodeX(axis.indexOf(event.sourcePeriodDate))"
                  :top="placementOf(projection.row.topicId, event.id)?.top ?? 46"
                  :measuring="measuring || placementOf(projection.row.topicId, event.id) === null"
                  @select="date => emit('selectDate', date)"
                />
              </template>
            </div>
          </div>
        </div>
      </div>

      <!-- 下方阅读区：当日关键事件列表（选中日驱动；事件 → 文章 → 阅读闭环 D12） -->
      <KeyEventDayPanel
        :selected-date="selectedDate"
        :events-by-date="dayEventsByDate"
        :reading="reading"
        @select-date="date => emit('selectDate', date)"
        @open-article="id => emit('openArticle', id)"
      />
    </template>
  </section>
</template>

<style scoped>
.kev {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  min-height: 0;
}

.kev-head {
  display: flex;
  align-items: baseline;
  gap: 0.6rem;
  min-width: 0;
}

.kev-kicker {
  font-size: 0.78rem;
  font-weight: 600;
  color: var(--color-text-primary);
}

.kev-hint {
  font-size: 0.7rem;
  color: var(--color-text-muted);
}

.kev-goto {
  margin-left: auto;
  border: 1px solid var(--color-border-medium);
  border-radius: 5px;
  background: none;
  color: var(--color-text-secondary);
  font-size: 0.7rem;
  padding: 0.15rem 0.45rem;
  cursor: pointer;
}

.kev-goto:hover {
  color: var(--color-accent);
  border-color: var(--color-accent);
}

.kev-stale,
.kev-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.45rem 0.65rem;
  font-size: 0.72rem;
  line-height: 1.5;
}

.kev-stale {
  border: 1px solid var(--color-warning-subtle);
  border-radius: 6px;
  background: var(--color-warning-subtle);
  color: var(--color-warning);
}

.kev-error {
  border: 1px solid color-mix(in srgb, var(--color-danger, var(--color-accent)) 40%, var(--color-bg-base));
  border-radius: 6px;
  background: color-mix(in srgb, var(--color-danger, var(--color-accent)) 7%, var(--color-bg-base));
  color: var(--color-danger, var(--color-accent));
}

.kev-link {
  flex: 0 0 auto;
  margin-left: auto;
  border: 1px solid currentColor;
  border-radius: 5px;
  background: none;
  color: inherit;
  font-size: 0.7rem;
  padding: 0.15rem 0.45rem;
  cursor: pointer;
}

.kev-link:disabled {
  opacity: 0.55;
  cursor: wait;
}

.kev-skeleton {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.kev-skeleton-row {
  height: 3rem;
  border-radius: 8px;
  background: color-mix(in srgb, var(--color-text-primary) 5%, var(--color-bg-base));
}

.kev-inline,
.kev-loading {
  padding: 0.5rem 0.65rem;
  border: 1px dashed var(--color-border-medium);
  border-radius: 8px;
  font-size: 0.75rem;
  color: var(--color-text-muted);
  line-height: 1.6;
}

.kev-loading {
  border-style: solid;
  border-color: var(--color-border-subtle);
}

/* 舞台内横滚：列宽被 64px 下限撑大时只滚本容器（布局契约例外），行头 sticky。 */
.kev-scroll {
  overflow-x: auto;
  overflow-y: hidden;
  padding-bottom: 0.2rem;
  touch-action: pan-x pan-y;
}

.kev-scroll.is-panning {
  cursor: grabbing;
  user-select: none;
}

.kev-inner {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

.kev-day-heads {
  display: flex;
  align-items: flex-end;
  gap: 0;
}

.kev-head-spacer {
  flex: 0 0 auto;
  width: 192px;
}

.kev-day {
  flex: 0 0 auto;
  width: var(--kev-col, 64px);
  padding-bottom: 0.2rem;
  text-align: center;
  border-bottom: 1px solid var(--color-border-subtle);
  color: var(--color-text-muted);
  font-size: 0.66rem;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.kev-day.is-today {
  color: var(--color-accent);
  font-weight: 600;
}

.kev-day.is-selected {
  color: var(--color-text-primary);
  font-weight: 600;
  box-shadow: inset 0 -2px 0 var(--color-accent);
}

.kev-row {
  display: flex;
  align-items: stretch;
  min-width: 0;
}

.kev-row.is-dormant {
  opacity: 0.55;
}

.kev-row-head {
  position: sticky;
  left: 0;
  z-index: 2;
  flex: 0 0 auto;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 0.15rem;
  width: 192px;
  padding: 0.3rem 16px 0.3rem 0;
  border: none;
  border-left: 2px solid transparent;
  background: var(--color-bg-base);
  color: var(--color-text-secondary);
  text-align: left;
  cursor: pointer;
}

.kev-row-head:hover .kev-row-name,
.kev-row-head.is-selected .kev-row-name {
  color: var(--color-text-primary);
}

.kev-row-head.is-selected {
  border-left-color: var(--color-accent);
}

.kev-row-head.is-selected .kev-row-name {
  font-weight: 600;
}

.kev-row-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.84rem;
  color: var(--color-text-primary);
}

.kev-row-meta {
  font-size: 0.64rem;
  color: var(--color-text-muted);
  white-space: nowrap;
}

/* 行体：显式宽度 = 轴宽（列宽 × 可见日数）。只含绝对定位子元素时 flex 宽会塌成 0，
   虚线轴（left:0;right:0）就撑不开（实机验收 P1-②：生命线不可见）。 */
.kev-row-body {
  position: relative;
  flex: 0 0 auto;
  min-height: 70px;
}

.kev-axis-line {
  position: absolute;
  top: 18px;
  left: 0;
  right: 0;
  border-top: 1px dashed var(--color-border-medium);
}

.kev-cell {
  position: absolute;
  top: 0;
  bottom: 0;
}

.kev-cell.is-col-selected::after {
  content: '';
  position: absolute;
  inset: 0;
  background: color-mix(in srgb, var(--color-accent) 7%, transparent);
  pointer-events: none;
}

.kev-node {
  position: absolute;
  top: 9px;
  left: 50%;
  transform: translateX(-50%);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  padding: 0 2px;
  border: 1px solid var(--color-accent);
  border-radius: 50%;
  background: var(--color-bg-elevated);
  color: var(--color-accent);
  font-size: 0.6rem;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  cursor: pointer;
}

.kev-node:hover,
.kev-node:focus-visible {
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-accent) 35%, transparent);
  outline: none;
}

.kev-group-summary {
  display: flex;
}

.kev-group-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.3rem 0;
  border: none;
  background: none;
  color: var(--color-text-muted);
  font-size: 0.72rem;
  cursor: pointer;
}

.kev-group-btn:disabled {
  cursor: default;
  opacity: 0.7;
}

.kev-group-title {
  color: var(--color-text-secondary);
  font-weight: 600;
}

.kev-group-count,
.kev-group-note {
  color: var(--color-text-muted);
}
</style>
