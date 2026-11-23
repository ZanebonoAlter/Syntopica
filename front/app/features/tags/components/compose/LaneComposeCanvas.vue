<script setup lang="ts">
/**
 * 编排态中央画布 · **行式泳道**工作面（rework-lane-compose-swimlane-canvas）。
 *
 * 纵向语法（D1/D2/D9）：**行 = 泳道**。
 * - 第 0 行 = 本次泳道（accent，落点；恒定，不绑任何候选链）；
 * - 第 1..N 行 = 服务端候选链（按服务端顺序；成员被取空的链**整行消失**）；
 * - 最后一行 = 散料地板（不属于任何链的节点）。
 * 行分组只靠「y 区段 + 行标签 + 1px 分隔线」：**不画横条胶囊、不画 tick 长线**。
 *
 * 手势（D3）：
 * - 从节点 / 行标签按下 = 拖拽对象；从空白按下 = 平移（两者共用 4px 阈值）；
 * - 拖到本次泳道行区间内松手 = 选入；从本次泳道拖到行外 = 移出；拖行标签 = 整组采纳；
 * - 点击节点 = **只**改选中态（钉住节点卡），SHALL NOT 改变成员数；
 * - 拖拽期间屏蔽滚轮缩放与悬停卡，拖拽结束那一下 click 被吞掉。
 *
 * 阈值（D4）只门控**散料**：候选链成员 / 已选 / 只读参照永不因阈值隐藏；
 * 被收起的散料以「另有 N 条未展开 · 展开」常驻，措辞不出现「不相关」。
 *
 * 检索态（D5）：候选链行与地板降透明度，本次泳道行不降级；命中点四角括号浮出，
 * 未命中节点不可点。检索范围由宿主按「未归类」派生后以 `searchHits` 传入。
 *
 * 标题（D6）：所有会画出来的节点都**常显截断标题**（≤10 字宽，**11px**，点下 14px 基线）；
 * 能不能看到只取决于**有没有位置**（已选 > 候选链成员 > 焦点相关 > 其它散料依次落位，
 * 放不下就整条不画）。只读参照梯子（密排 10px 一行）不画标题。
 * 时间列宽有 124px 下限 + 横向滚动，所以常显标题**永不因窗口收窄关档**。
 *
 * 关系表达：**不画任何连线 / 箭头**（母 spec「不生成事件关系边」）；原关系只用
 * 「节点卡的原关系条数」与「相关节点标题档」表达。
 *
 * 横向滚动：内容宽 = 行标签 132 + 天数 × 列宽 + 右留白 24，可超容器宽（不缩字）；
 * 行标签列**横向吸顶**（不透明底 + 提档底色 + 6 级渐隐阴影），日期轴纵向吸顶且随内容横向移动，
 * 但被裁切到标签列右侧（刻度不得侵入吸顶标签列）；吸顶时本次泳道行的标签组画在置顶覆盖层。
 *
 * 只读边界：active 泳道的点只读（不可拖、卡内无选入动作）。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import {
  buildRowIndex,
  centroidRecommendations,
  clampZoom,
  cosineFor,
  estimateTextWidth,
  isDragBeyond,
  layoutRows,
  panScroll,
  placeRelatedTitles,
  pointAppearance,
  proximityOf,
  relatedNodeIds,
  relativeProximities,
  rowMemberY,
  scaledPointRadius,
  thresholdOf,
  titleBudget,
  truncateByWidth,
  TITLE_CHAR_WIDTH,
  truncateText,
  visibleScatter,
  zoomAnchorScroll,
  zoomStep,
  type LaneBox,
  type LaneLabelCandidate,
  type LanePlacedLabel,
  type LaneRowLayout,
  type LaneRowMember,
  type LaneThresholdTier,
  LANE_ROW_GAP,
  LANE_ROW_LABEL_H,
  LANE_ROW_SELF,
  LANE_SLOT_H,
  LANE_MIN_COLUMN,
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
  /** 阈值三档（与右栏「可补充的事件」共用同一状态）。 */
  thresholdTier?: LaneThresholdTier
  /** 检索态是否生效（关键词非空）。 */
  searchActive?: boolean
  /** 检索命中集（= 未归类层里按标题命中的 section id）。 */
  searchHits?: Set<string>
  /** 最近一次移出的成员（非 null 时显示 3 秒撤销入口）。 */
  undoId?: string | null
}>()

const emit = defineEmits<{
  'focus': [id: string]
  'clear-focus': []
  'toggle-member': [id: string, checked: boolean]
  /** 整组采纳（拖候选链行标签 → 本次泳道）：带链 key，走与左栏勾选同一个写入口。 */
  'adopt-chain': [key: string]
  'undo-removal': []
}>()

/** 兜底画布宽（容器未测量时用；真实渲染以容器 clientWidth 为准）。 */
const FALLBACK_WIDTH = 900
const DATE_AXIS_H = 22
/** 日期轴之后的行区起始 y。 */
const ROWS_TOP = DATE_AXIS_H + 10
/** 左侧行标签区宽度（名称 + 计数；成员点从它右侧开始，互不压字）。 */
const LABEL_W = 132
/** 右侧留白（最后一列标题不越界）。 */
const RIGHT_PAD = 24
/** 候选链行上限：超过时在画布底部明写「另有 N 个候选话题在左侧候选栏」。 */
const MAX_CHAIN_ROWS = 10
/** 底部留白：散料地板收起时整段 = 76（1 槽 18px 行）+ 36 = 112px。 */
const CANVAS_BOTTOM_PAD = 36
/** 节点常显标题的字号与相对点的基线偏移（11px 字号，点下 14px）。 */
const NODE_TITLE_FONT = 11
const NODE_TITLE_OFFSET = 14
/**
 * 点尺寸（2026-11-21 提档 + 用户反馈修）：散料直径 11px、推荐 **11.8–14.8px**、已选 15px。
 *
 * 已选 / 散料 / 只读走 `pointAppearance` 的固定覆盖 flag（缺省行为留给修正画布）；
 * 推荐走**显式区间** `RECOMMEND_RADIUS_RANGE`（接近度 0 → 5.9，1 → 7.4），
 * 不再撞旧公式的 6.57–7.0 天花板（只差 0.43px，肉眼等于常量）。
 * 接近度本身走**相对归一**（`relativeProximities`）：阈值只门控列表与散料可见性，
 * 不参与这张视觉映射（绝对除法在距离≈阈值时把整批压成 0.04–0.14）。
 * `BASE_POINT_RADIUS` 只喂缺省公式（未传区间时的兜底路径）。
 */
const BASE_POINT_RADIUS = 7.3
const MEMBER_RADIUS = 7.5
const SCATTER_RADIUS = 5.5
/**
 * 接近度 → 半径 / 深浅（2026-11-21 用户实拍反馈：「颜色 + 大小」被压没了）。
 *
 * 半径 5.9–7.4（直径 11.8–14.8）仍 < 已选 7.5、> 散料 5.5；
 * 深浅 18%–80%（旧档 12%–67%）。只在编排画布传入，修正画布缺省逐位不变。
 */
const RECOMMEND_RADIUS_RANGE = { min: 5.9, max: 7.4 }
const PROXIMITY_STRENGTH_RANGE = { min: 18, max: 80 }
/** 只读参照梯子仍是密排（10px 一行），半径保持提档前的尺寸。 */
const READONLY_RADIUS = 2.75
/** 节点常显标题的碰撞盒（11px 字号一行）。 */
const NODE_TITLE_BOX_H = 14
/** 标题向下错行的行数与步长；错行用尽仍撞上 → 整条不画（宁缺勿叠）。 */
const NODE_TITLE_LINES = 2
const NODE_TITLE_LINE_H = 16
/**
 * 节点卡宽度**固定**：位置计算必须用常量，不能用「量出来的宽度」。
 *
 * 原因（2026-11-01 实机 bug）：绝对定位卡片的宽度是 shrink-to-fit，会被
 * 「容器宽 − left」夹住；而 left 又用「量出来的宽度」算 → **反馈环**：
 * 量到 92px → 位置更靠右 → 可用宽更小 → 量到 60px → … → 塌成一条竖条。
 */
const CARD_WIDTH = 300

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
  // 容器尺寸变化后滚动位置可能被浏览器夹回，吸顶位移要跟着重算
  syncScrollTop()
}

let resizeObserver: ResizeObserver | null = null

function onWindowResize(): void {
  measure()
}

onMounted(() => {
  measure()
  window.addEventListener('resize', onWindowResize)
  window.addEventListener('keydown', onWindowKeydown, true)
  if (typeof ResizeObserver !== 'undefined' && containerRef.value) {
    resizeObserver = new ResizeObserver(() => measure())
    resizeObserver.observe(containerRef.value)
  }
  // 滚轮缩放：必须非 passive，否则 preventDefault 不生效（画布上滚轮会把整页滚走）
  containerRef.value?.addEventListener('wheel', onWheel, { passive: false })
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onWindowResize)
  window.removeEventListener('keydown', onWindowKeydown, true)
  resizeObserver?.disconnect()
  resizeObserver = null
  containerRef.value?.removeEventListener('wheel', onWheel)
})

defineExpose({ measure, debugPointsEpoch: () => pointsEpoch })

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

/** 窗口内两端可见的原关系（只用于相关节点/节点卡计数，**不画连线**）。 */
const visibleRelations = computed(() =>
  props.relations.filter(rel => visibleIds.value.has(rel.fromId) && visibleIds.value.has(rel.toId)),
)

const tier = computed<LaneThresholdTier>(() => props.thresholdTier ?? 'standard')
const threshold = computed(() => thresholdOf(tier.value, props.matchThreshold))
const searchActive = computed(() => Boolean(props.searchActive))
const searchHits = computed<ReadonlySet<string>>(() => props.searchHits ?? new Set<string>())

/** 质心推荐（阈值取当前档位；尺寸/深浅映射与右栏同一批）。 */
const recommendations = computed(() =>
  centroidRecommendations(props.sections, props.selectedIds, threshold.value),
)
const recommendedDistance = computed(() => {
  const map = new Map<string, number>()
  for (const item of recommendations.value.unassigned) map.set(item.section.id, item.distance)
  return map
})

/** 候选链成员（服务端唯一口径；阈值/检索都不得把它们藏起来）。 */
const candidateIds = computed(() => {
  const set = new Set<string>()
  for (const chain of props.chains) for (const id of chain.sectionIds) set.add(String(id))
  return set
})

const readonlyIds = computed(() => {
  const set = new Set<string>()
  for (const section of props.sections) if (isReadonly(section)) set.add(section.id)
  return set
})

// ── 行归属（只从服务端 chains 派生，前端不重算） ─────────────────────────────
const rowIndex = computed(() => buildRowIndex(
  props.chains,
  props.sections.map(section => section.id),
  props.selectedIds,
))

const floorRow = computed(() => rowIndex.value.floorRow)

function rowForId(id: string): number {
  return rowIndex.value.rowOf.get(id) ?? floorRow.value
}

/** 散料地板是否已展开（展开 = 不受阈值收起；仍按单日封顶）。 */
const floorExpanded = ref(false)

/**
 * 「无成型候选」空态（ui-design State Matrix `empty（无成型候选）`）：窗口内没有任何
 * 候选链（服务端唯一口径 `chains` 已按窗口过滤）。
 *
 * 用「候选链数为 0」而不是「链行已被取空」——后者是用户把链成员全拖进本次泳道，
 * 不能当作「无成型候选」（否则会弹出错误提示并让全部散料瞬间涌出）。
 */
const noCandidateChains = computed(() => rowIndex.value.chainRows.length === 0)

/**
 * 散料可见集：展开或**无成型候选**时按「全部」档，否则按当前档位。
 *
 * 空链 override 只改画布的呈现层（无质心可言的窗口不该只剩一条空行），
 * 不改 `visibleScatter` 的档位合同，也不连带右栏批次。
 */
const scatterVisibility = computed(() => {
  const effective: LaneThresholdTier = floorExpanded.value || noCandidateChains.value ? 'all' : tier.value
  return visibleScatter(
    props.sections,
    props.selectedIds,
    candidateIds.value,
    readonlyIds.value,
    recommendations.value.center,
    effective,
    props.matchThreshold,
  )
})

/** 画布可见集 = 白名单旁路 ∪ 达标散料 ∪ 检索命中（命中点在真实位置浮出）。 */
const visibleIdSet = computed(() => {
  const set = new Set(scatterVisibility.value.visibleIds)
  if (searchActive.value) {
    for (const id of searchHits.value) {
      if (visibleIds.value.has(id)) set.add(id)
    }
  }
  return set
})

/** 被阈值收起、尚未显示的散料数（折叠计数 = 散料总数 − 当前可见散料数）。 */
const hiddenScatterCount = computed(() => {
  const visible = scatterVisibility.value.visibleScatterIds
  let count = 0
  for (const section of props.sections) {
    const id = section.id
    if (props.selectedIds.has(id) || candidateIds.value.has(id) || readonlyIds.value.has(id)) continue
    if (!visible.has(id)) count += 1
  }
  return count
})

const floorFoldText = computed(() => {
  if (floorExpanded.value) return '收起'
  return hiddenScatterCount.value > 0 ? `低相关散料 ${hiddenScatterCount.value} 条 · 展开` : ''
})

function toggleFloor(): void {
  floorExpanded.value = !floorExpanded.value
}

/**
 * 画布上仍然有成员的候选链行（成员被取空的链整行消失）。
 *
 * 只按「行归属 + 是否被选入」判，不读 `visibleIdSet`——候选链成员本来就走白名单旁路，
 * 且这样不会与 `scatterVisibility` 形成计算属性循环依赖。
 */
const chainViews = computed(() =>
  rowIndex.value.chainRows
    .map(row => ({
      ...row,
      members: row.ids.filter(id => rowForId(id) === row.row),
    }))
    .filter(row => row.members.length > 0),
)

/** 顶部状态提示：「无成型候选」时明写，不静默只留一条空行。 */
const noChainNotice = computed(() => noCandidateChains.value && drawable.value.length > 0)

const renderedChainViews = computed(() => chainViews.value.slice(0, MAX_CHAIN_ROWS))
const moreChainCount = computed(() => Math.max(0, chainViews.value.length - renderedChainViews.value.length))

const rowOrder = computed(() => [
  LANE_ROW_SELF,
  ...renderedChainViews.value.map(row => row.row),
  floorRow.value,
])

/** 已展开的「行:日」（单日封顶后可点开该日全部）。 */
const expandedDays = ref<Set<string>>(new Set())

function expandDay(row: number, date: string): void {
  const key = `${row}:${date}`
  const next = new Set(expandedDays.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  expandedDays.value = next
}

/** 行几何：本次泳道行预留 2 槽以便落点；散料地板至少 1 槽。 */
const layout = computed<LaneRowLayout[]>(() => {
  const allowed = new Set(rowOrder.value)
  const membersByRow = new Map<number, LaneRowMember[]>()
  for (const section of drawable.value) {
    const id = section.id
    if (isReadonly(section)) continue
    if (!visibleIdSet.value.has(id)) continue
    const row = rowForId(id)
    if (!allowed.has(row)) continue
    const list = membersByRow.get(row) ?? []
    list.push({ id, date: section.periodDate.slice(0, 10) })
    membersByRow.set(row, list)
  }
  return layoutRows(rowIndex.value.rowOf, props.dates, membersByRow, {
    top: ROWS_TOP,
    rowOrder: rowOrder.value,
    minSlots: new Map([[LANE_ROW_SELF, 2], [floorRow.value, 1]]),
    expanded: expandedDays.value,
  })
})

const layoutByRow = computed(() => {
  const map = new Map<number, LaneRowLayout>()
  for (const row of layout.value) map.set(row.row, row)
  return map
})

const rowsBottom = computed(() => {
  const rows = layout.value
  const last = rows[rows.length - 1]
  return last ? last.top + last.height : ROWS_TOP
})

/** 本次泳道成员覆盖的天数（行标签的「跨度」）。 */
const coveredDays = computed(() => {
  const days = new Set<string>()
  for (const section of props.sections) {
    if (props.selectedIds.has(section.id)) days.add(section.periodDate.slice(0, 10))
  }
  return days.size
})

interface RowOverflowMark {
  date: string
  count: number
  x: number
  y: number
}

interface RowView {
  /** 行号（只读参照梯子为 -1）。 */
  row: number
  kind: 'lane' | 'chain' | 'floor' | 'readonly'
  top: number
  height: number
  slots: number
  label: string
  /** 候选链行对应的服务端链 key（整组拖的写入口）。 */
  chainKey: string
  countText: string
  tally: string
  draggable: boolean
  /** 整组拖时一起进入拖拽的成员（该行未采纳成员）。 */
  labelIds: string[]
  overflow: RowOverflowMark[]
  /**
   * 该行要画的节点。节点归「行」是吸顶的前提：本次泳道行整组（含成员点）
   * 一起平移才能钉住，且它的不透明底必须盖住滚到下面的内容（所以它最后画）。
   */
  nodes: ComposeCandidate[]
}

const ROW_NAME_CHARS = Math.max(4, Math.floor((LABEL_W - 20) / 11.5))

const rowViews = computed<RowView[]>(() => {
  const out: RowView[] = []
  for (const row of layout.value) {
    const kind: RowView['kind'] = row.row === LANE_ROW_SELF
      ? 'lane'
      : row.row === floorRow.value ? 'floor' : 'chain'
    const chain = kind === 'chain' ? renderedChainViews.value.find(item => item.row === row.row) : undefined
    const unselected = chain ? chain.members.filter(id => !props.selectedIds.has(id)) : []
    const overflow: RowOverflowMark[] = []
    for (const [date, count] of row.overflowByDate) {
      overflow.push({
        date,
        count,
        x: xForDate(date),
        y: row.top + LANE_ROW_LABEL_H + row.slots * LANE_SLOT_H + 17,
      })
    }
    out.push({
      row: row.row,
      kind,
      top: row.top,
      height: row.height,
      slots: row.slots,
      chainKey: chain?.key ?? '',
      label: kind === 'lane'
        ? '本次泳道'
        : kind === 'floor'
          ? '散料'
          : truncateText(chain?.label ?? '', ROW_NAME_CHARS),
      countText: kind === 'lane'
        ? `${props.selectedIds.size} 条 · 覆盖 ${coveredDays.value} 天`
        : kind === 'floor'
          ? `${scatterVisibility.value.visibleScatterIds.size} 条可见`
          : `剩余 ${unselected.length} 条`,
      tally: chain
        ? `${chain.ids.filter(id => props.selectedIds.has(id)).length}/${chain.ids.length}`
        : '',
      draggable: Boolean(chain) && unselected.length > 0,
      labelIds: unselected,
      overflow,
      nodes: nodesByRow.value.get(row.row) ?? [],
    })
  }
  // 只读参照梯子（已有泳道）也作为一行参与渲染：节点标记只有一份（见 RowView.nodes）
  if (props.showAssigned) {
    out.push({
      row: -1,
      kind: 'readonly',
      top: 0,
      height: 0,
      slots: 0,
      label: '',
      chainKey: '',
      countText: '',
      tally: '',
      draggable: false,
      labelIds: [],
      overflow: [],
      nodes: nodesByRow.value.get(-1) ?? [],
    })
  }
  return out
})

/**
 * 绘制顺序：只读梯子 → 其它行 → **本次泳道行最后**。
 *
 * 吸顶时本次泳道行带着自己的不透明底整组上移，最后画才能盖住滚到下面的行；
 * 不溢出时没有任何重叠，顺序对视觉无影响（01–03 态不变）。
 */
const paintingRows = computed(() => {
  const lane = rowViews.value.filter(row => row.kind === 'lane')
  const readonly = rowViews.value.filter(row => row.kind === 'readonly')
  const others = rowViews.value.filter(row => row.kind !== 'lane' && row.kind !== 'readonly')
  return [...readonly, ...others, ...lane]
})

// ── 布局（容器真实像素 1:1） ──────────────────────────────────────────────
/**
 * 可视宽（容器 `clientWidth`）。
 *
 * **不得用 `scrollWidth`**：内容宽随 `clientWidth` 反推，读 `scrollWidth` 就是反馈环。
 */
const viewportWidth = computed(() => (containerWidth.value > 0 ? containerWidth.value : FALLBACK_WIDTH))

const dateIndex = computed(() => new Map(props.dates.map((date, index) => [date, index])))

const leftPad = LABEL_W
const columns = computed(() => Math.max(1, props.dates.length))
/** 时间列宽：理想均分但有 52px 下限（不足就把内容撑宽让人横向滚动，不缩字）。 */
const columnWidth = computed(() => Math.max(
  LANE_MIN_COLUMN,
  (viewportWidth.value - leftPad - RIGHT_PAD) / columns.value,
))
/** 内容宽 = 行标签 + 天数 × 列宽 + 右留白（可超可视宽 → 容器横向滚动）。 */
const contentWidth = computed(() => leftPad + columns.value * columnWidth.value + RIGHT_PAD)
/** viewBox 宽（逻辑坐标）：不溢出时恰好等于可视宽，与提档前完全一致。 */
const vbWidth = computed(() => Math.max(viewportWidth.value, contentWidth.value))
const dateTick = computed(() => Math.max(1, Math.ceil(props.dates.length / 14)))
const titleChars = computed(() => titleBudget(columnWidth.value))

function xForDate(date: string): number {
  const index = dateIndex.value.get(date) ?? 0
  return leftPad + (index + 0.5) * columnWidth.value
}

/** active 泳道行（有参照时画在分隔线之下，只读；既有密排梯子）。 */
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

const separatorY = computed(() => rowsBottom.value + 24)
const activeTop = computed(() => separatorY.value + 42)
const activeRowOffsets = computed(() => {
  const offsets = new Map<string, number>()
  let cursor = activeTop.value
  for (const row of activeRows.value) {
    offsets.set(row.topicId, cursor)
    cursor += Math.max(52, row.count * 10 + 20)
  }
  return { offsets, cursor }
})

interface LanePoint {
  x: number
  y: number
  /** 所在行号（只读参照为 -1）。 */
  row: number
}

/** D7：points 只在可见集 / 窗口 / 容器宽度 / 阈值变化时重算；拖拽只动 ghost。 */
let pointsEpoch = 0

/** 节点坐标：泳道行按槽位（同日下叠），只读参照走既有梯子。 */
const points = computed(() => {
  pointsEpoch += 1
  const map = new Map<string, LanePoint>()
  const activeSlots = new Map<string, number>()
  for (const section of drawable.value) {
    if (!isReadonly(section)) continue
    const topicId = section.persistentTopicId ?? ''
    const k = activeSlots.get(topicId) ?? 0
    activeSlots.set(topicId, k + 1)
    const base = activeRowOffsets.value.offsets.get(topicId) ?? activeTop.value
    map.set(section.id, { x: xForDate(section.periodDate.slice(0, 10)), y: base + k * 10, row: -1 })
  }
  for (const row of layout.value) {
    for (const [id, slot] of row.slotOf) {
      const section = sectionById.value.get(id)
      if (!section) continue
      map.set(id, { x: xForDate(section.periodDate.slice(0, 10)), y: rowMemberY(row, slot), row: row.row })
    }
  }
  return map
})

const canvasHeight = computed(() => (props.showAssigned
  ? Math.max(560, activeRowOffsets.value.cursor + 30)
  : Math.max(460, rowsBottom.value + CANVAS_BOTTOM_PAD)))

// ── 吸顶（P-1：本次泳道行钉在画布顶部） ─────────────────────────────────
/**
 * 画布纵向滚动的实际位置（CSS px，含缩放）。
 *
 * 吸顶不是「另一个容器」，而是把**本次泳道行整组**（行标签 + 落点框 + 成员点）
 * 反向平移 `scrollTop / zoom`：它因此永远停在日期轴之下，滚到地板的散料也能拖进来。
 */
const scrollTop = ref(0)
const scrollLeft = ref(0)
const scrollTopLogical = computed(() => (zoom.value > 0 ? scrollTop.value / zoom.value : scrollTop.value))
const scrollLeftLogical = computed(() => (zoom.value > 0 ? scrollLeft.value / zoom.value : scrollLeft.value))

function syncScrollTop(): void {
  const el = containerRef.value
  scrollTop.value = el ? el.scrollTop || 0 : 0
  scrollLeft.value = el ? el.scrollLeft || 0 : 0
}

function onSurfaceScroll(): void {
  syncScrollTop()
}

/** 内容是否真的超出可视高度（决定要不要吸顶；不溢出时行为与旧版完全一致）。 */
const overflowing = computed(() => {
  const viewport = containerHeight.value
  if (!viewport) return false
  return canvasHeight.value * zoom.value > viewport + 1
})

/** 内容是否真的超出可视宽（决定行标签列要不要横向吸顶）。 */
const overflowingX = computed(() => {
  const viewport = containerWidth.value
  if (!viewport) return false
  return vbWidth.value * zoom.value > viewport + 1
})

/** 吸顶行的反向位移（逻辑坐标 = CSS px / zoom；不溢出时不下发 transform）。 */
const pinnedTransform = computed(() => (overflowing.value
  ? `translate(0, ${scrollTopLogical.value})`
  : undefined))

/**
 * 行标签列的反向位移（横向吸顶）：整组（含 row-handle 拖拽热区）平移 `scrollLeft / zoom`。
 *
 * 它挂在行组内部，所以与本次泳道行的纵向位移天然叠加（同一套逻辑位移）。
 */
const rowLabelTransform = computed(() => (overflowingX.value
  ? `translate(${scrollLeftLogical.value}, 0)`
  : undefined))

/** 可视宽（逻辑坐标）：落点框 / 高亮覆盖它，不覆盖整条内容宽。 */
const visibleWidth = computed(() => {
  const viewport = containerWidth.value
  if (!viewport || zoom.value <= 0) return vbWidth.value
  return Math.min(vbWidth.value, viewport / zoom.value)
})
/** 落点框左缘：横向滚动后跟着可视区走（否则滚到中段就只剩上下两条虚线）。 */
const dropzoneX = computed(() => 6 + (overflowingX.value ? scrollLeftLogical.value : 0))
const dropzoneWidth = computed(() => Math.max(0, visibleWidth.value - 12))

/** 吸顶行下缘的层界阴影层数（每层 1px，透明度递减；1px 分隔线另画）。 */
const STICKY_SHADOW_BANDS = 6

// ── 横向裁剪 / 吸顶标签置顶层 ────────────────────────────────────────────
/**
 * 日期轴刻度与列 guide 的裁剪区划（2026-11-21 用户反馈：横向滚动后刻度文字
 * 滑到左侧吸顶标签列上方，与该列的行标签文字重叠）。
 *
 * - 只允许画在标签列右侧的可视区：**左界 = `LABEL_W`（视口坐标）**，右界 = `vbWidth`；
 * - 裁切框活在内容坐标，所以左界要加上 `scrollLeftLogical` 才是「视口 132」；
 * - `y` 方向不裁（整个画布高都在里面），纵向仍由 `pinnedTransform` 吸顶；
 * - `overflowingX` 为假时不下发 `clip-path`，不溢出时渲染与旧版逐像素一致。
 */
const axisClipId = `lcc-axis-clip-${useId()}`
const axisClipPath = computed(() => (overflowingX.value ? `url(#${axisClipId})` : undefined))
const axisClipX = computed(() => LABEL_W + (overflowingX.value ? scrollLeftLogical.value : 0))
const axisClipWidth = computed(() => Math.max(0, vbWidth.value - axisClipX.value))

/** 本次泳道行（置顶标签层要读它的 top/height/label/countText）。 */
const laneRowView = computed(() => rowViews.value.find(row => row.kind === 'lane') ?? null)

/**
 * 吸顶时本次泳道行的**标签组**改画到置顶覆盖层（所有行之后、日期刻度之前）。
 *
 * 行循环里用 `v-if` 跳过它——`[data-role="row-label"]` 全画布只有一份，
 * 不会出现「两份都画、z 序靠文档顺序碰运气」。
 */
const pinnedLaneLabelVisible = computed(() => overflowing.value && laneRowView.value !== null)

// ── 接近程度（相对归一：整块画布统一梯度，阈值不参与） ──────────────────────
/**
 * 本次渲染批次内每个点的质心距离（**阈值不参与**）。
 *
 * 批次 = 真正会画出来的非已选、非只读点（候选链成员 / 散料 / 检索命中都在内）——
 * 只有这样整块画布才能读出远近梯度；已选点恒 7.5、只读参照不参与。
 */
const batchDistances = computed(() => {
  const map = new Map<string, number>()
  const center = recommendations.value.center
  if (!center) return map
  for (const section of renderedNodes.value) {
    if (isReadonly(section) || props.selectedIds.has(section.id)) continue
    const distance = cosineFor(section.embedding, center)
    if (distance === null || !Number.isFinite(distance)) continue
    map.set(section.id, distance)
  }
  return map
})

/** 相对归一后的接近度表（最近 = 1、最远 = 0）；空表 = 无质心，全画布退回 ordinary。 */
const relativeProximity = computed(() => relativeProximities(batchDistances.value))

/** 每个点的视觉参数（半径 / 接近程度；相关/弱化为正交标志）。 */
const appearances = computed(() => {
  const map = new Map<string, ReturnType<typeof pointAppearance>>()
  const proximity = relativeProximity.value
  for (const section of drawable.value) {
    const readonly = isReadonly(section)
    map.set(section.id, pointAppearance(
      readonly ? undefined : recommendedDistance.value.get(section.id),
      props.matchThreshold,
      props.selectedIds.has(section.id),
      readonly,
      BASE_POINT_RADIUS,
      columnWidth.value,
      {
        memberRadius: MEMBER_RADIUS,
        scatterRadius: SCATTER_RADIUS,
        readonlyRadius: READONLY_RADIUS,
        recommendRange: RECOMMEND_RADIUS_RANGE,
        strengthRange: PROXIMITY_STRENGTH_RANGE,
        proximityOverride: proximity.get(section.id),
        focus: props.focusId === section.id,
        related: focusRelatedIds.value?.has(section.id) ?? false,
        dimmed: focusRelatedIds.value !== null && !focusRelatedIds.value.has(section.id),
      },
    ))
  }
  return map
})

// ── 缩放（滚轮放大 / 缩小；拖拽期间屏蔽） ─────────────────────────────────
const zoom = ref(LANE_ZOOM_DEFAULT)
const zoomPercent = computed(() => `${Math.round(zoom.value * 100)}%`)

// 内容高度 / 缩放 / 是否溢出变化后滚动位置可能被浏览器夹回 → 吸顶位移同步一次
// （放在 zoom 之后：watch 会立刻求值 source，提前引用会踩 TDZ）
watch([overflowing, overflowingX, canvasHeight, vbWidth, zoom], () => {
  void nextTick().then(syncScrollTop)
})

function onWheel(event: WheelEvent): void {
  // 拖拽期间不响应滚轮缩放（D3）
  if (dragging.value) return
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

// ── 指针手势：平移 / 单条拖拽 / 整组拖拽（D3） ─────────────────────────────
type GestureKind = 'pan' | 'node' | 'row'

interface Gesture {
  kind: GestureKind
  id: string
  /** 整组拖时的候选链 key（与左栏勾选同一写入口）。 */
  chainKey: string
  row: number
  ids: string[]
  start: { x: number, y: number }
  scroll: { left: number, top: number }
}

/** 平移 / 拖拽后压掉那一下 click 的时刻。 */
let lastDragAt = 0
let gesture: Gesture | null = null
const isPanning = ref(false)
const dragging = ref(false)
const hoverRow = ref<number | null>(null)
const dragGhost = ref<{ x: number, y: number } | null>(null)
const dragLabel = ref('')
const dragCount = ref(0)
/** 被拖对象相对起手点的内容坐标位移：本体跟随光标实时移动（不是只动 ghost）。 */
const dragDelta = ref<{ x: number, y: number } | null>(null)
let dragOrigin: { x: number, y: number } | null = null

function isInteractiveTarget(target: Element | null): boolean {
  return Boolean(target?.closest?.('.lcc-card, .lcc-zoom, .lcc-undo, button, input, select, textarea'))
}

function onSurfacePointerDown(event: PointerEvent): void {
  const target = event.target as Element | null
  if (isInteractiveTarget(target)) return
  if (typeof event.button === 'number' && event.button !== 0) return
  const surface = containerRef.value
  const start = { x: event.clientX, y: event.clientY }
  const scroll = { left: surface?.scrollLeft ?? 0, top: surface?.scrollTop ?? 0 }
  const nodeEl = target?.closest?.('[data-node]') as Element | null
  if (nodeEl) {
    const id = nodeEl.getAttribute('data-node') ?? ''
    const section = sectionById.value.get(id)
    if (!section || isReadonly(section)) return
    if (searchActive.value && !searchHits.value.has(id)) return
    // 阻止浏览器默认行为：否则拖动会顺带选中画布上其它节点的文字（2026-11-23 用户反馈）。
    event.preventDefault()
    gesture = { kind: 'node', id, chainKey: '', row: rowForId(id), ids: [id], start, scroll }
    return
  }
  const rowEl = target?.closest?.('[data-role="row-handle"]') as Element | null
  if (rowEl) {
    const row = Number(rowEl.getAttribute('data-row'))
    const view = rowViews.value.find(item => item.row === row)
    if (!view || !view.draggable || !view.labelIds.length) return
    event.preventDefault()
    gesture = { kind: 'row', id: '', chainKey: view.chainKey, row, ids: [...view.labelIds], start, scroll }
    return
  }
  gesture = { kind: 'pan', id: '', chainKey: '', row: -1, ids: [], start, scroll }
  isPanning.value = false
}

/** 客户端坐标 → 画布内容坐标（解除滚动与缩放）。 */
function contentPointOf(clientX: number, clientY: number): { x: number, y: number } {
  const surface = containerRef.value
  if (!surface) return { x: 0, y: 0 }
  const rect = surface.getBoundingClientRect()
  const z = zoom.value > 0 ? zoom.value : 1
  return {
    x: (clientX - rect.left + surface.scrollLeft) / z,
    y: (clientY - rect.top + surface.scrollTop) / z,
  }
}

/** 画布内容坐标（解除滚动与缩放）。 */
function contentPoint(event: PointerEvent): { x: number, y: number } {
  return contentPointOf(event.clientX, event.clientY)
}

/**
 * 内容坐标 y → 所在行号（仅**非吸顶**部分）。
 *
 * 吸顶时本次泳道行的内容坐标已被上移 `scrollTopLogical`，它由 `rowAtPointer`
 * 单独判定；这里跳过第 0 行，避免「指针在画布上方（已被吸顶行遮住的区域）」误判成落点。
 */
function rowAtContent(y: number): number | null {
  for (const row of layout.value) {
    if (row.row === LANE_ROW_SELF && overflowing.value) continue
    if (y >= row.top && y <= row.top + row.height) return row.row
  }
  return null
}

/**
 * 指针 → 落点行号（**用吸顶行的实际可视位置**）。
 *
 * 换算：`contentY = clientY - 容器顶 + scrollTop`（`contentPoint`）；
 * 吸顶行在内容坐标里的实际区间 = `[row0.top + scrollTop/zoom, row0.top + row0.height + scrollTop/zoom]`
 * ——因为该行整组被反向平移了 `scrollTop/zoom`。等价于「吸顶行屏幕顶边 = 日期轴下缘」。
 */
function rowAtPointer(event: PointerEvent): number | null {
  const point = contentPoint(event)
  if (overflowing.value) {
    const band = layoutByRow.value.get(LANE_ROW_SELF)
    if (band) {
      const top = band.top + scrollTopLogical.value
      if (point.y >= top && point.y <= top + band.height) return LANE_ROW_SELF
    }
  }
  return rowAtContent(point.y)
}

let ghostFrame = 0
let pendingGhost: { x: number, y: number } | null = null

/** ghost 跟随（rAF 节流；首次移动立即落位，避免「拖了没反馈」）。 */
function updateDragPoint(event: PointerEvent): void {
  const point = contentPoint(event)
  hoverRow.value = rowAtPointer(event)
  if (dragOrigin) dragDelta.value = { x: point.x - dragOrigin.x, y: point.y - dragOrigin.y }
  pendingGhost = point
  if (dragGhost.value === null) dragGhost.value = point
  if (ghostFrame) return
  const flush = (): void => {
    ghostFrame = 0
    if (pendingGhost) dragGhost.value = pendingGhost
  }
  if (typeof requestAnimationFrame === 'function') ghostFrame = requestAnimationFrame(flush)
  else dragGhost.value = point
}

function beginObjectDrag(event: PointerEvent): void {
  const active = gesture
  if (!active) return
  dragging.value = true
  // 起点必须取 pointerdown 的位置，不能取第一次 move 的位置——否则首帧位移恒为 0，
  // 用户会看到「拖了但节点不跟手」。
  dragOrigin = contentPointOf(active.start.x, active.start.y)
  dragDelta.value = { x: 0, y: 0 }
  dragCount.value = active.ids.length
  dragLabel.value = active.kind === 'row'
    ? (rowViews.value.find(row => row.row === active.row)?.label ?? '')
    : (sectionById.value.get(active.id)?.clusterLabel ?? '')
  updateDragPoint(event)
}

function onSurfacePointerMove(event: PointerEvent): void {
  if (!gesture) return
  const surface = containerRef.value
  if (!surface) return
  const current = { x: event.clientX, y: event.clientY }
  if (!dragging.value) {
    if (!isDragBeyond(gesture.start, current)) return
    if (gesture.kind === 'pan') {
      isPanning.value = true
      try { surface.setPointerCapture?.(event.pointerId) } catch { /* 某些环境不支持指针捕获 */ }
    } else {
      try { surface.setPointerCapture?.(event.pointerId) } catch { /* 同上 */ }
      beginObjectDrag(event)
    }
  }
  if (gesture.kind === 'pan') {
    const next = panScroll(gesture.start, current, gesture.scroll)
    surface.scrollLeft = next.left
    surface.scrollTop = next.top
    return
  }
  updateDragPoint(event)
}

/** 落点结算：本次泳道 = 选入；行外 = 移出；整组只在本次泳道生效。 */
function applyDrop(active: Gesture, targetRow: number | null): void {
  if (active.kind === 'node') {
    const id = active.id
    const wasMember = props.selectedIds.has(id)
    if (targetRow === LANE_ROW_SELF && !wasMember) {
      emit('toggle-member', id, true)
      return
    }
    if (targetRow !== LANE_ROW_SELF && wasMember) emit('toggle-member', id, false)
    return
  }
  if (active.kind === 'row') {
    if (targetRow !== LANE_ROW_SELF) return
    if (active.chainKey) emit('adopt-chain', active.chainKey)
  }
}

function resetGesture(): void {
  gesture = null
  dragging.value = false
  isPanning.value = false
  dragGhost.value = null
  dragDelta.value = null
  dragOrigin = null
  hoverRow.value = null
  dragLabel.value = ''
  dragCount.value = 0
  pendingGhost = null
}

function onSurfacePointerUp(event?: PointerEvent): void {
  const active = gesture
  if (!active) return
  const wasDragging = dragging.value
  let targetRow: number | null = null
  if (wasDragging && active.kind !== 'pan') {
    if (event && Number.isFinite(event.clientX) && Number.isFinite(event.clientY)) targetRow = rowAtPointer(event)
    else if (dragGhost.value) targetRow = rowAtContent(dragGhost.value.y)
  }
  resetGesture()
  if (active.kind === 'pan') {
    if (wasDragging) lastDragAt = Date.now()
    return
  }
  if (!wasDragging) return
  lastDragAt = Date.now()
  applyDrop(active, targetRow)
}

/** Esc / pointercancel / 拖出窗口：取消拖拽，无副作用。 */
function onSurfacePointerCancel(): void {
  if (!gesture) return
  if (dragging.value || isPanning.value) lastDragAt = Date.now()
  resetGesture()
}

function onWindowKeydown(event: KeyboardEvent): void {
  if (event.key !== 'Escape') return
  if (!dragging.value && !isPanning.value) return
  event.preventDefault()
  // 捕获阶段拦下：拖拽中的 Esc 只取消拖拽，不冒泡到宿主的「退出编排态」
  event.stopPropagation()
  lastDragAt = Date.now()
  resetGesture()
}

/**
 * 点的**逻辑半径**（写进 SVG 的值）。
 *
 * 缩小时按**像素下限**补偿：保证渲染半径（逻辑半径 × zoom）仍 ≥3px（只读参照 2.5px）。
 */
function pointRadius(id: string): number {
  const section = sectionById.value.get(id)
  const readonly = section ? isReadonly(section) : false
  const radius = appearances.value.get(id)?.radius ?? BASE_POINT_RADIUS
  return scaledPointRadius(radius, zoom.value, readonly ? 2.5 : 3)
}

// ── 节点常显截断标题（D6） ─────────────────────────────────────────
/**
 * 常显标题的截断文本：按**可用宽度**截（列宽 − 2px 内边距），最多 10 个字宽。
 *
 * 不用字数口径：`Gemini 4.5` 与 10 个汉字同配额会把前者截成 `Gemini 4 …`，
 * 明明还有半列空白。宽度口径让中英混排都把列宽用满。
 */
function nodeTitleText(section: ComposeCandidate): string {
  const budget = titleChars.value
  if (budget === null) return ''
  const maxWidth = Math.min(columnWidth.value - 2, (budget + 1) * TITLE_CHAR_WIDTH)
  return truncateByWidth(section.clusterLabel, maxWidth, NODE_TITLE_FONT)
}

/**
 * 标题优先级：已选 > 候选链成员 > 与焦点相关 > 其它散料。
 *
 * **所有会画出来的节点都有标题**（含散料），优先级只决定「谁先占位置」——
 * 位置不够时后放的整条不画（宁缺勿叠），而不是按身份决定有没有标题。
 */
function titlePriority(id: string): number {
  if (props.selectedIds.has(id)) return 4
  if (candidateIds.value.has(id)) return 3
  if (focusRelatedIds.value?.has(id)) return 2
  return 1
}

/**
 * 节点标题落位（**全画布唯一一套**）：障碍 = 其它节点的圆 + 已落位标题。
 *
 * 两套旧标题（`persistentTitleBoxes` 常显标题 + 相关节点标题）合并到这里：
 * 一个节点只会得到一个 `<text>`、只占一次位，不会再出现「同一节点两份标题 / 重复占位」。
 * 落不下的整条不画（`skipped`）——不叠字、也不缩到 2 字碎片。
 */
const nodeTitleItems = computed<LanePlacedLabel[]>(() => {
  if (titleChars.value === null) return []
  const circles: LaneBox[] = []
  const cands: LaneLabelCandidate[] = []
  for (const section of renderedNodes.value) {
    const point = points.value.get(section.id)
    if (!point) continue
    const radius = pointRadius(section.id)
    // 所有已画出的节点圆都参与避让（含只读参照），但标题只给非只读节点
    circles.push({ x: point.x - radius, y: point.y - radius, w: radius * 2, h: radius * 2 })
    if (isReadonly(section)) continue
    const text = nodeTitleText(section)
    if (!text) continue
    cands.push({
      id: section.id,
      x: point.x,
      y: point.y,
      r: radius,
      boxTop: point.y + NODE_TITLE_OFFSET - NODE_TITLE_BOX_H,
      w: estimateTextWidth(text, NODE_TITLE_FONT),
      h: NODE_TITLE_BOX_H,
      priority: titlePriority(section.id),
      text,
    })
  }
  return placeRelatedTitles(cands, {
    lines: NODE_TITLE_LINES,
    lineHeight: NODE_TITLE_LINE_H,
    gap: 0,
    minX: 0,
    maxX: vbWidth.value,
    minY: 0,
    maxY: canvasHeight.value - 2,
    obstaclesOf: () => circles,
  }).filter(label => !label.skipped && label.box)
})

/** 节点 id → 已落位的标题（模板只读这张表：标题跟着节点所在行/吸顶一起走）。 */
const nodeTitleById = computed(() => {
  const map = new Map<string, { id: string, text: string, x: number, y: number }>()
  for (const label of nodeTitleItems.value) {
    map.set(label.id, { id: label.id, text: label.text, x: label.box!.x + label.box!.w / 2, y: label.box!.y + label.box!.h })
  }
  return map
})

function nodeTitleOf(id: string): { id: string, text: string, x: number, y: number } | null {
  return nodeTitleById.value.get(id) ?? null
}

// ── 相关节点（与已选相关：1 跳原关系 ∪ 质心较近的推荐） ────────────────────
/**
 * 与已选相关的节点集（只做**标记**，不再单独画一套标题——标题已统一）。
 *
 * 仍作为 `lcc-canvas__related-title` / `lane-related-title-*` 锚点保留：
 * 外层用例依赖它区分「相关节点标题」与普通节点标题；样式已不再单独区分（统一 11px）。
 */
const relatedIds = computed(() => {
  const out = relatedNodeIds(visibleRelations.value, props.selectedIds)
  for (const item of recommendations.value.unassigned) out.add(item.section.id)
  for (const id of props.selectedIds) out.delete(id)
  return out
})

const focusRelatedIds = computed<Set<string> | null>(() => {
  if (!props.focusId) return null
  const recommended = recommendations.value.unassigned.map(item => item.section.id)
  return relatedNodeIds(visibleRelations.value, new Set([props.focusId]), recommended)
})

/*
 * 画布底部原本还有一行「相关节点（只读 N 个…）：名字…；另有 M 个可补充的相关节点未标名…」。
 * 2026-11-23 用户定档：这行说明没必要（只读节点在只读梯子里可见，可补充的在右栏）。
 * 已整段删除，不保留空壳计算属性。
 */

// ── 节点路径（关系边已抟除：不画任何连线，原关系只进节点卡计数与相关标题） ──────

/** 拖拽中：被拖节点本体跟随光标（不是只让 ghost 动）。 */
function draggedTransform(id: string): string | undefined {
  const delta = dragDelta.value
  if (!delta || !gesture || !gesture.ids.includes(id)) return undefined
  return `translate(${delta.x.toFixed(2)} ${delta.y.toFixed(2)})`
}

/** 该节点是否有已确认关键线索（★ 星形主体，不是附加角标）。 */
function hasKeyStar(id: string): boolean {
  return (sectionById.value.get(id)?.confirmedKeyEvents?.length ?? 0) > 0
}

/**
 * 五角星路径：外接半径 = `pointRadius`。
 *
 * 只换形状、不改尺寸语义——星形面积比同半径圆小，因此看起来略小是正确的；
 * 放大外接半径会伪造一个「更接近」的大小信号，违反接近度编码红线。
 */
function starPath(id: string): string {
  const point = points.value.get(id)
  if (!point) return ''
  const radius = pointRadius(id)
  const innerRatio = 0.44
  const pts: string[] = []
  for (let i = 0; i < 10; i++) {
    const angle = -Math.PI / 2 + (i * Math.PI) / 5
    const r = i % 2 === 0 ? radius : radius * innerRatio
    pts.push(`${(point.x + Math.cos(angle) * r).toFixed(2)} ${(point.y + Math.sin(angle) * r).toFixed(2)}`)
  }
  return `M${pts.join(' L')}Z`
}

function checkPath(id: string): string {
  const point = points.value.get(id)
  if (!point) return ''
  const radius = pointRadius(id)
  // 星形节点的内切圆更小：✓ 按 0.32R 缩（圆点 0.48R），避免溢出星形主体。
  const w = Math.min(3, radius * (hasKeyStar(id) ? 0.32 : 0.48))
  return `M${point.x - w} ${point.y} L${point.x - w * 0.25} ${point.y + w * 0.75} L${point.x + w} ${point.y - w * 0.8}`
}

/**
 * 检索命中标记 = **四角括号**（与「已选 = 实心 + ✓」「相关 = 加粗描边」形状不同源，
 * 不复用 accent 实环）。
 */
function hitBracketPath(id: string): string {
  const point = points.value.get(id)
  if (!point) return ''
  const half = Math.max(6, pointRadius(id) + 4.5)
  const arm = 2.3
  const { x, y } = point
  return [
    `M${x - half} ${y - half + arm} L${x - half} ${y - half} L${x - half + arm} ${y - half}`,
    `M${x + half - arm} ${y - half} L${x + half} ${y - half} L${x + half} ${y - half + arm}`,
    `M${x + half} ${y + half - arm} L${x + half} ${y + half} L${x + half - arm} ${y + half}`,
    `M${x - half + arm} ${y + half} L${x - half} ${y + half} L${x - half} ${y + half - arm}`,
  ].join(' ')
}

function isSearchHit(id: string): boolean {
  return searchActive.value && searchHits.value.has(id)
}

/** 检索态下未命中节点不可点（不可选中、不可拖）。 */
function isClickable(section: ComposeCandidate): boolean {
  return !searchActive.value || isSearchHit(section.id)
}

/** 已落槽、真正会画出来的节点（被单日封顶收起的成员留到 +N 展开）。 */
const renderedNodes = computed(() => drawable.value.filter(section => points.value.has(section.id)))

/** 节点按「所在行」分组（只读参照为 -1）：节点标记在模板里只有一份，跟着行走。 */
const nodesByRow = computed(() => {
  const map = new Map<number, ComposeCandidate[]>()
  for (const section of renderedNodes.value) {
    const row = rowOfNode(section.id)
    const list = map.get(row) ?? []
    list.push(section)
    map.set(row, list)
  }
  // 行内**自下而上**画（y 降序）：槽距 18px 比「11px 标题 + 点下 14px」更窄，
  // 这样同列的常显标题压在上一个点上，而不是被下一个点盖掉。
  for (const list of map.values()) {
    list.sort((a, b) => (points.value.get(b.id)?.y ?? 0) - (points.value.get(a.id)?.y ?? 0))
  }
  return map
})

// ── 节点卡 ────────────────────────────────────────────────────────────────
const cardId = computed(() => props.focusId ?? hoverId.value)

const cardSection = computed(() => (cardId.value ? sectionById.value.get(cardId.value) ?? null : null))

/** 节点卡里的已确认关键线索（★ 明细；空数组时整块不渲染）。 */
const cardKeyEvents = computed(() => cardSection.value?.confirmedKeyEvents ?? [])

const cardVisible = computed(() => {
  const section = cardSection.value
  if (!section) return false
  // 检索态下只有命中点能出卡（未命中节点不可点）
  return !searchActive.value || isSearchHit(section.id)
})

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
  // 吸顶行整组被反向平移了 scrollTop/zoom；卡片不在那个 <g> 里，要自己补回这段位移
  const pinnedShift = overflowing.value && rowOfNode(id) === LANE_ROW_SELF ? scrollTopLogical.value : 0
  const screenX = point.x * zoom.value
  const screenY = (point.y + pinnedShift) * zoom.value
  const screenRadius = radius * zoom.value
  // 卡片定位是**内容坐标**（绝对定位在滚动容器里，会跟着内容滚），
  // 所以横向可摆放区间 = 可视区在内容坐标里的区间（含 scrollLeft），不是 [0, clientWidth]。
  const viewLeft = scrollLeft.value
  const viewRight = viewLeft + containerWidth.value
  const right = screenX + screenRadius + 10 + width
  const left = right > viewRight ? screenX - screenRadius - 10 - width : screenX + screenRadius + 10
  const maxTop = Math.max(4, (containerHeight.value || canvasHeight.value) - height - 4)
  const top = Math.max(4, Math.min(maxTop, screenY - height / 2))
  return { left: Math.max(viewLeft + 4, left), top }
})

watch(cardId, () => {
  void Promise.resolve().then(() => {
    const el = cardEl.value
    // 只量高度：宽度固定（见 CARD_WIDTH 的注释），量出来的宽度会把卡片喂进塌缩反馈环
    if (el) cardSize.value = { width: CARD_WIDTH, height: el.offsetHeight || cardSize.value.height }
  })
})

function onNodeEnter(section: ComposeCandidate): void {
  if (dragging.value) return
  if (searchActive.value && !isSearchHit(section.id)) return
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
  // 刚拖完的那一下 click 不算「点节点」（否则拖一下就把节点选中了）
  if (Date.now() - lastDragAt < 250) return
  if (!isClickable(section)) return
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

/** 行标签 hover：提亮该行全部成员（整组拖动的可拖提示）。 */
function onRowHandleEnter(row: number): void {
  if (dragging.value) return
  hoverRow.value = row
}

function onRowHandleLeave(): void {
  if (dragging.value) return
  hoverRow.value = null
}

function rowOfNode(id: string): number {
  return points.value.get(id)?.row ?? -1
}

/** 撤销入口（3 秒内有效；超时由 composable 清空）。 */
const undoSection = computed(() => (props.undoId ? sectionById.value.get(props.undoId) ?? null : null))

// 外部取消聚焦（父组件的「取消聚焦」按钮）→ 卡片整体收起；之后悬停别的点仍可临时读全文
watch(() => props.focusId, (now, prev) => {
  if (!now && prev) hoverId.value = null
})
</script>

<template>
  <section class="lcc-canvas" data-testid="lane-compose-canvas-panel" aria-label="事件关系画布">
    <!-- 无成型候选：明写状态，不静默只留一条空行（地板全部散料常驻） -->
    <p v-if="noChainNotice" class="lcc-canvas__notice" data-testid="lane-compose-no-chain-notice">
      这个范围没有成型候选
    </p>
    <div
      ref="containerRef"
      class="lcc-canvas__surface"
      :class="{ 'is-panning': isPanning, 'is-dragging': dragging }"
      data-testid="lane-compose-canvas"
      @pointerdown="onSurfacePointerDown"
      @pointermove="onSurfacePointerMove"
      @pointerup="onSurfacePointerUp"
      @pointercancel="onSurfacePointerCancel"
      @scroll="onSurfaceScroll"
    >
      <svg
        :viewBox="`0 0 ${vbWidth} ${canvasHeight}`"
        :width="vbWidth * zoom"
        :height="canvasHeight * zoom"
        data-testid="lane-compose-canvas-svg"
        role="group"
        aria-label="泳道行与事件节点，已选点带勾，拖节点或行标签到本次泳道即选入"
      >
        <!--
          日期轴 / 列 guide 的裁切框：只允许画在横向吸顶标签列右侧的可视区
          （左界视口 132，右界 vbWidth）；不横向溢出时整段不上场。
        -->
        <defs>
          <clipPath :id="axisClipId" data-role="axis-clip">
            <rect :x="axisClipX" y="0" :width="axisClipWidth" :height="canvasHeight" />
          </clipPath>
        </defs>

        <!-- 日期列 guide（画在最底，不遮任何内容；留在内容层：纵向不吸顶、横向随内容走） -->
        <g class="lcc-canvas__axis" data-role="column-guides" :clip-path="axisClipPath">
          <template v-for="(date, index) in dates" :key="date">
            <line
              v-if="index % dateTick === 0 || index === dates.length - 1"
              class="lcc-canvas__guide"
              :x1="leftPad + (index + 0.5) * columnWidth"
              :x2="leftPad + (index + 0.5) * columnWidth"
              :y1="DATE_AXIS_H + 12"
              :y2="canvasHeight - 12"
            />
          </template>
        </g>

        <!-- 不画事件关系边（identity 与 similarity 都不画）：原关系只进节点卡计数与相关标题 -->

        <!-- 行（含只读参照梯子）＋该行节点：本次泳道行最后画，吸顶时才能盖住滚动内容 -->
        <g class="lcc-canvas__rows">
          <g
            v-for="row in paintingRows"
            :key="row.row"
            class="lcc-canvas__row"
            :class="{
              'is-lane-row': row.kind === 'lane',
              'is-chain-row': row.kind === 'chain',
              'is-floor-row': row.kind === 'floor',
              'is-search-bg': searchActive && row.kind !== 'lane' && row.kind !== 'readonly',
              'is-drop-target': dragging && hoverRow === row.row && row.kind === 'lane',
              'is-drop-other': dragging && hoverRow === row.row && row.kind !== 'lane' && row.kind !== 'readonly',
            }"
            :data-row="row.row"
            :data-row-kind="row.kind"
            :data-role="row.kind === 'lane' && overflowing ? 'sticky-row' : undefined"
            :transform="row.kind === 'lane' ? pinnedTransform : undefined"
          >

            <g class="lcc-canvas__row-body">
              <!--
                吸顶行不透明底：从画布顶（含日期轴区）盖到本行下缘，滚到下面的行整片消失；
                日期轴刻度在行之后单独画（见模板末尾），因此仍浮在底色之上。
                不溢出时不画（01–03 态与原渲染一致）。

                起点取 `-scrollTopLogical`：吸顶行整组被下移了 scrollTop，底色的几何起点
                必须是「视口顶」而不是内容 0——否则横向/纵向滚动混合时会在顶部留出
                一整条未遮挡的缝，滚上来的行标签就叠在吸顶文字上（用户实拍反馈）。
              -->
              <rect
                v-if="row.kind === 'lane' && overflowing"
                class="lcc-canvas__sticky-bg"
                data-role="sticky-bg"
                x="0"
                :y="-scrollTopLogical"
                :width="vbWidth"
                :height="scrollTopLogical + row.top + row.height"
              />
              <!--
                吸顶带底色提一档（用户反馈：原来与画布底色同色，没有视觉分界）。
                **不透明**：先铺上面那层 bg-base 再叠 elevated——elevated 在深色主题是
                0.98 alpha，直接当唯一底色会透出滚动内容。
              -->
              <rect
                v-if="row.kind === 'lane' && overflowing"
                class="lcc-canvas__sticky-tint"
                data-role="sticky-tint"
                x="0"
                :y="-scrollTopLogical"
                :width="vbWidth"
                :height="scrollTopLogical + row.top + row.height"
              />
              <rect
                v-if="row.kind === 'lane'"
                class="lcc-canvas__dropzone"
                :x="dropzoneX"
                :y="row.top + 4"
                :width="dropzoneWidth"
                :height="Math.max(0, row.height - LANE_ROW_GAP - 1)"
                rx="6"
              />
              <text
                v-for="mark in row.overflow"
                :key="`${row.row}-${mark.date}`"
                class="lcc-canvas__day-more"
                data-role="day-more"
                :data-date="mark.date"
                :x="mark.x"
                :y="mark.y"
                text-anchor="middle"
                @click="expandDay(row.row, mark.date)"
              >+{{ mark.count }}</text>
            </g>

            <!-- 节点标记：全画布唯一一份，挂在所属行里（吸顶行整组带走成员点） -->
            <g
              v-for="section in row.nodes"
              :key="section.id"
              class="lcc-canvas__node"
              :class="{
                'is-selected': selectedIds.has(section.id),
                'is-readonly': isReadonly(section),
                'is-focus': focusId === section.id,
                'is-recommended': recommendedDistance.has(section.id),
                'is-related': appearances.get(section.id)?.related,
                'is-dimmed': appearances.get(section.id)?.dimmed,
                'is-row-hover': hoverRow !== null && !dragging && rowOfNode(section.id) === hoverRow && !selectedIds.has(section.id),
                'is-search-hit': isSearchHit(section.id),
                'is-search-bg': searchActive && !isSearchHit(section.id) && rowOfNode(section.id) !== 0,
                'is-search-muted': searchActive && !isSearchHit(section.id),
              }"
              :data-node="section.id"
              :data-row="rowOfNode(section.id)"
              :transform="draggedTransform(section.id)"
              :data-proximity="appearances.get(section.id)?.proximity ?? ''"
              :data-distance="recommendedDistance.get(section.id) ?? ''"
              :style="`--proximity-fill: ${appearances.get(section.id)?.strength ?? 0}%`"
              :tabindex="isClickable(section) ? 0 : -1"
              role="button"
              :aria-label="`${isReadonly(section) ? '已有泳道只读' : selectedIds.has(section.id) ? '已选' : '未选'} · ${section.periodDate.slice(0, 10)} · ${section.clusterLabel}${hasKeyStar(section.id) ? ` · 已确认关键线索 ${section.confirmedKeyEvents?.length ?? 0} 条` : ''}`"
              :data-testid="`lane-node-${section.id}`"
              @mouseenter="onNodeEnter(section)"
              @mouseleave="onNodeLeave(section)"
              @click="onNodeClick(section)"
              @keydown.enter.prevent="onNodeClick(section)"
              @keydown.space.prevent="onNodeClick(section)"
            >
              <title>{{ section.periodDate.slice(0, 10) }} · {{ section.clusterLabel }}</title>
              <circle
                class="lcc-canvas__hit"
                :cx="points.get(section.id)?.x"
                :cy="points.get(section.id)?.y"
                :r="Math.max(8, pointRadius(section.id) + 4)"
              />
              <path
                v-if="hasKeyStar(section.id)"
                class="lcc-canvas__point"
                data-role="key-star"
                :d="starPath(section.id)"
              />
              <circle
                v-else
                class="lcc-canvas__point"
                :cx="points.get(section.id)?.x"
                :cy="points.get(section.id)?.y"
                :r="pointRadius(section.id)"
              />
              <path
                v-if="selectedIds.has(section.id)"
                class="lcc-canvas__check"
                :class="{ 'is-star': hasKeyStar(section.id) }"
                :d="checkPath(section.id)"
              />
              <circle
                class="lcc-canvas__focus-ring"
                :cx="points.get(section.id)?.x"
                :cy="points.get(section.id)?.y"
                :r="pointRadius(section.id) + 4"
              />
              <path
                v-if="isSearchHit(section.id)"
                class="lcc-canvas__hit-bracket"
                data-role="search-hit"
                :d="hitBracketPath(section.id)"
              />
              <!--
                常显截断标题（**全画布唯一一套**）：位置来自 `nodeTitleById`（已避让过其它节点圆与
                其它标题）；画在节点组里才能跟着所在行/吸顶一起移动，放不下就没有这一条。
                `lcc-canvas__related-title` / `lane-related-title-*` 只是「与已选相关」的旧锚点
                （标题本体已合并成一套，样式不再区分）。
              -->
              <text
                v-if="nodeTitleOf(section.id)"
                class="lcc-canvas__node-title"
                :class="{ 'lcc-canvas__related-title': relatedIds.has(section.id) }"
                data-role="node-title"
                :data-testid="relatedIds.has(section.id) ? `lane-related-title-${section.id}` : undefined"
                :x="nodeTitleOf(section.id)?.x ?? 0"
                :y="nodeTitleOf(section.id)?.y ?? 0"
                text-anchor="middle"
              >{{ nodeTitleOf(section.id)?.text }}</text>
            </g>

            <!--
              行标签层：画在节点**之后**——横向吸顶时标签底色要盖住滑到下面的点与标题；
              整组（含 row-handle 热区）反向平移 scrollLeft/zoom，不溢出时不下发 transform。
            -->
            <template v-if="row.kind === 'readonly'">
              <!-- 只读梯子的泳道名也在标签列：横向吸顶后仍钉在最左 -->
              <g class="lcc-canvas__row-label" data-role="row-label-group" :transform="rowLabelTransform">
                <rect
                  v-if="overflowingX"
                  class="lcc-canvas__label-bg"
                  data-role="row-label-bg"
                  x="0"
                  :y="separatorY"
                  :width="LABEL_W"
                  :height="Math.max(0, activeRowOffsets.cursor - separatorY)"
                />
                <!-- 标签列底色提一档（与吸顶带同一档，两条吸顶轴看起来是一块「固定框」） -->
                <rect
                  v-if="overflowingX"
                  class="lcc-canvas__sticky-tint"
                  data-role="row-label-tint"
                  x="0"
                  :y="separatorY"
                  :width="LABEL_W"
                  :height="Math.max(0, activeRowOffsets.cursor - separatorY)"
                />
                <text x="10" :y="separatorY + 24">已有泳道 · 只读</text>
                <text
                  v-for="active in activeRows"
                  :key="active.topicId"
                  class="lcc-canvas__lane-label"
                  :x="10"
                  :y="(activeRowOffsets.offsets.get(active.topicId) ?? activeTop) - 2"
                >{{ truncateText(active.label, 10) }}</text>
                <!-- 标签列右缘渐隐阴影（与其它行同款 6 级） -->
                <template v-if="overflowingX">
                  <rect
                    v-for="band in STICKY_SHADOW_BANDS"
                    :key="`lane-label-shadow-${band}`"
                    class="lcc-canvas__label-shadow"
                    :data-role="band === 1 ? 'row-label-shadow' : undefined"
                    :x="LABEL_W + band - 1"
                    :y="separatorY"
                    width="1"
                    :height="Math.max(0, activeRowOffsets.cursor - separatorY)"
                    :opacity="0.28 - (band - 1) * 0.045"
                  />
                </template>
              </g>
              <!-- 层界分隔线画在标签底色之上（否则标签列里看不到行界） -->
              <line class="lcc-canvas__separator" x1="10" :x2="vbWidth - 10" :y1="separatorY" :y2="separatorY" />
            </template>

            <template v-else>
              <!--
                行标签列（横向吸顶）：整组（含 row-handle 拖拽热区）反向平移 scrollLeft/zoom；
                不溢出时不下发 transform（01–03 态与原渲染一致）。
                吸顶时本次泳道行的标签组改画到置顶覆盖层（见行组之后）——这里 `v-if` 跳过，
                保证 `[data-role="row-label"]` 全画布只有一份。
              -->
              <g
                v-if="!(row.kind === 'lane' && overflowing)"
                class="lcc-canvas__row-label"
                data-role="row-label-group"
                :transform="rowLabelTransform"
              >
                <rect
                  v-if="overflowingX"
                  class="lcc-canvas__label-bg"
                  data-role="row-label-bg"
                  x="0"
                  :y="row.top"
                  :width="LABEL_W"
                  :height="row.height"
                />
                <!-- 标签列底色提一档（与吸顶带同一档） -->
                <rect
                  v-if="overflowingX"
                  class="lcc-canvas__sticky-tint"
                  data-role="row-label-tint"
                  x="0"
                  :y="row.top"
                  :width="LABEL_W"
                  :height="row.height"
                />
                <rect
                  v-if="row.draggable"
                  class="lcc-canvas__row-handle"
                  data-role="row-handle"
                  :data-row="row.row"
                  x="4"
                  :y="row.top + 2"
                  :width="LABEL_W - 10"
                  :height="LANE_ROW_LABEL_H - 4"
                  @pointerenter="onRowHandleEnter(row.row)"
                  @pointerleave="onRowHandleLeave()"
                />
                <text
                  class="lcc-canvas__row-name"
                  :class="{ 'is-lane-name': row.kind === 'lane' }"
                  data-role="row-label"
                  x="10"
                  :y="row.top + 18"
                >{{ row.label }}</text>
                <text class="lcc-canvas__row-count" x="10" :y="row.top + 29">{{ row.countText }}</text>
                <!-- 计数也留在标签列（跟随横向吸顶；原来在内容右缘，滚一下就不见了） -->
                <text
                  v-if="row.tally"
                  class="lcc-canvas__row-tally"
                  :x="LABEL_W - 8"
                  :y="row.top + 29"
                  text-anchor="end"
                >{{ row.tally }}</text>
                <!-- 标签列右缘渐隐阴影（横向同款 6 级；与纵向吸顶同一套画法） -->
                <template v-if="overflowingX">
                  <rect
                    v-for="band in STICKY_SHADOW_BANDS"
                    :key="`label-shadow-${band}`"
                    class="lcc-canvas__label-shadow"
                    :data-role="band === 1 ? 'row-label-shadow' : undefined"
                    :x="LABEL_W + band - 1"
                    :y="row.top"
                    width="1"
                    :height="row.height"
                    :opacity="0.28 - (band - 1) * 0.045"
                  />
                </template>
              </g>
              <!-- 行分隔线画在标签底色之上（否则标签列里看不到行界） -->
              <line
                class="lcc-canvas__row-separator"
                data-role="row-separator"
                x1="6"
                :x2="vbWidth - 6"
                :y1="row.top"
                :y2="row.top"
              />
            </template>

            <!-- 吸顶层界：1px 分隔 + 渐隐阴影（钉在行下缘，跟着一起平移） -->
            <template v-if="row.kind === 'lane' && overflowing">
              <rect
                v-for="band in STICKY_SHADOW_BANDS"
                :key="`sticky-shadow-${band}`"
                class="lcc-canvas__sticky-shadow"
                :data-role="band === 1 ? 'sticky-shadow' : undefined"
                x="0"
                :y="row.top + row.height + band - 1"
                :width="vbWidth"
                height="1"
                :opacity="0.28 - (band - 1) * 0.045"
              />
              <!-- 带下缘的 1px 硬分隔（画在渐隐之后，边界才看得见） -->
              <line
                class="lcc-canvas__sticky-edge"
                data-role="sticky-edge"
                x1="0"
                :x2="vbWidth"
                :y1="row.top + row.height"
                :y2="row.top + row.height"
              />
            </template>
          </g>
        </g>

        <!--
          本次泳道行的**标签组置顶层**（用户反馈：吸顶带文字被刻度/滚动内容压住）。

          - 位置：所有行之后、日期刻度之前——标签底色/文字在内容之上，但刻度仍浮在最顶；
          - z 序不靠文档顺序碰运气：行循环里 `v-if` 跳过本次泳道行的标签组，这里画唯一一份；
          - 外层拿 `pinnedTransform`（纵向吸顶）、内层仍拿 `rowLabelTransform`（横向吸顶），
            两个位移各管一轴，不重复；
          - 节点点仍在行循环里（本次泳道行已最后画），不随标签层搬。
        -->
        <g
          v-if="pinnedLaneLabelVisible && laneRowView"
          class="lcc-canvas__lane-label-layer"
          data-role="lane-label-overlay"
          :transform="pinnedTransform"
        >
          <g class="lcc-canvas__row-label" data-role="row-label-group" :transform="rowLabelTransform">
            <rect
              v-if="overflowingX"
              class="lcc-canvas__label-bg"
              data-role="row-label-bg"
              x="0"
              :y="laneRowView.top"
              :width="LABEL_W"
              :height="laneRowView.height"
            />
            <rect
              v-if="overflowingX"
              class="lcc-canvas__sticky-tint"
              data-role="row-label-tint"
              x="0"
              :y="laneRowView.top"
              :width="LABEL_W"
              :height="laneRowView.height"
            />
            <!-- 整组拖拽热区在标签层里（本次泳道行不可整组拖，`draggable` 为假时不渲染） -->
            <rect
              v-if="laneRowView.draggable"
              class="lcc-canvas__row-handle"
              data-role="row-handle"
              :data-row="laneRowView.row"
              x="4"
              :y="laneRowView.top + 2"
              :width="LABEL_W - 10"
              :height="LANE_ROW_LABEL_H - 4"
              @pointerenter="onRowHandleEnter(laneRowView.row)"
              @pointerleave="onRowHandleLeave()"
            />
            <text
              class="lcc-canvas__row-name"
              :class="{ 'is-lane-name': laneRowView.kind === 'lane' }"
              data-role="row-label"
              x="10"
              :y="laneRowView.top + 18"
            >{{ laneRowView.label }}</text>
            <text class="lcc-canvas__row-count" x="10" :y="laneRowView.top + 29">{{ laneRowView.countText }}</text>
            <text
              v-if="laneRowView.tally"
              class="lcc-canvas__row-tally"
              :x="LABEL_W - 8"
              :y="laneRowView.top + 29"
              text-anchor="end"
            >{{ laneRowView.tally }}</text>
            <template v-if="overflowingX">
              <rect
                v-for="band in STICKY_SHADOW_BANDS"
                :key="`lane-label-shadow-${band}`"
                class="lcc-canvas__label-shadow"
                :data-role="band === 1 ? 'row-label-shadow' : undefined"
                :x="LABEL_W + band - 1"
                :y="laneRowView.top"
                width="1"
                :height="laneRowView.height"
                :opacity="0.28 - (band - 1) * 0.045"
              />
            </template>
          </g>
        </g>

        <!-- 行标签列与内容区的竖向分隔（横向吸顶时钉在标签列右缘；画在标签层之上才不糊） -->
        <line
          v-if="overflowingX"
          class="lcc-canvas__label-divider"
          data-role="label-divider"
          :transform="rowLabelTransform"
          :x1="LABEL_W"
          :x2="LABEL_W"
          y1="0"
          :y2="canvasHeight"
        />

        <!-- 散料地板的折叠计数（可展开；措辞只用「另有 N 条未展开」） -->
        <!-- 横向吸顶：折叠计数是行级控件，滚出左缘就点不到了，故与行标签同一位移 -->
        <text
          v-if="floorFoldText"
          class="lcc-canvas__floor-fold"
          data-role="floor-fold"
          data-testid="lane-floor-fold"
          :transform="rowLabelTransform"
          :x="2"
          :y="(layoutByRow.get(floorRow)?.top ?? rowsBottom) + (layoutByRow.get(floorRow)?.height ?? 0) - 9"
          text-anchor="start"
          @click="toggleFloor()"
        >{{ floorFoldText }}</text>

        <!-- 候选链行超出上限：不静默不画 -->
        <text
          v-if="moreChainCount > 0"
          class="lcc-canvas__row-more"
          data-role="row-more"
          :transform="rowLabelTransform"
          :x="10"
          :y="rowsBottom + 18"
        >另有 {{ moreChainCount }} 个候选话题在左侧候选栏</text>

        <!-- 节点标题已与常显标题合并成一套（画在各节点组里，跟着行/吸顶移动），此处不再画第二套 -->

        <!--
          日期轴刻度：钉在画布顶部（与本次泳道行共用同一个横向无关的纵向位移），
          **不横向吸顶**——时间刻度必须跟着日期列一起横向移动，否则就对不上列。
          横向裁切到标签列右侧（左界视口 132）：否则横向滚动后刻度会滑到吸顶标签列上方。
        -->
        <g
          class="lcc-canvas__ticks"
          data-role="date-axis"
          :transform="pinnedTransform"
          :clip-path="axisClipPath"
        >
          <template v-for="(date, index) in dates" :key="`tick-${date}`">
            <text
              v-if="index % dateTick === 0 || index === dates.length - 1"
              :x="leftPad + (index + 0.5) * columnWidth"
              :y="DATE_AXIS_H"
              text-anchor="middle"
            >{{ date.slice(5) }}</text>
          </template>
        </g>
      </svg>

      <!-- 拖拽 ghost：放大 1.6× 跟随（整组拖时带成员数） -->
      <div
        v-if="dragGhost"
        class="lcc-drag-ghost"
        data-role="drag-ghost"
        :class="{ 'is-group': dragCount > 1 }"
        :style="{ left: `${dragGhost.x * zoom}px`, top: `${dragGhost.y * zoom}px` }"
        :title="dragLabel"
      >
        <span v-if="dragCount > 1" class="lcc-drag-ghost__count">{{ dragCount }}</span>
      </div>

      <!-- 节点卡：悬停出卡，选中后钉在该点旁并跟随 -->
      <p v-if="!renderedNodes.length" class="lcc-canvas__empty" data-testid="lane-compose-canvas-empty">
        这个日期范围内没有可画的事件。换个范围，或在右栏「按标题找」里补。
      </p>
      <div
        v-if="cardVisible && cardSection"
        ref="cardEl"
        class="lcc-card"
        :class="{ 'is-pinned': focusId !== null }"
        :style="cardPosition ? { left: `${cardPosition.left}px`, top: `${cardPosition.top}px` } : undefined"
        data-testid="lane-node-card"
      >
        <button type="button" class="lcc-card__close" aria-label="关闭节点卡" data-testid="lane-node-card-close" @click="closeCard">×</button>
        <strong class="lcc-card__title">{{ cardSection.clusterLabel }}</strong>
        <span class="lcc-card__meta">
          {{ cardSection.periodDate.slice(0, 10) }}<template v-if="cardReadonly"> · 已有泳道只读</template><template v-if="cardDistance !== null"> · 估算接近度 {{ proximityOf(cardDistance)?.toFixed(2) }}</template>
        </span>
        <span v-if="cardKeyEvents.length" class="lcc-card__key" data-role="key-events">
          <template v-if="cardKeyEvents.length === 1">★ 已确认关键线索：{{ cardKeyEvents[0]!.title }}</template>
          <template v-else>
            ★ 已确认关键线索 {{ cardKeyEvents.length }} 条
            <template v-for="event in cardKeyEvents" :key="event.markerId">
              <br>· {{ event.title }}
            </template>
          </template>
        </span>
        <span v-if="cardArchivedOrigin" class="lcc-card__origin">来自已归档泳道：{{ cardArchivedOrigin }}</span>
        <span class="lcc-card__meta">{{ cardRelationCount ? `${cardRelationCount} 条原关系` : '暂无原关系，可人工选入' }}</span>
        <button
          v-if="!cardReadonly"
          type="button"
          class="lcc-card__action"
          data-testid="lane-node-card-action"
          @click="toggleCardMember"
        >{{ cardPicked ? '移出本次泳道' : '＋ 选入本次泳道' }}</button>
      </div>
    </div>

    <!-- 移出撤销（3 秒；一次限时操作，不是通用撤销栈） -->
    <p v-if="undoSection" class="lcc-undo" data-testid="lane-compose-undo">
      已移出「{{ truncateText(undoSection.clusterLabel, 12) }}」
      <button type="button" class="lcc-undo__btn" data-testid="lane-compose-undo-btn" @click="emit('undo-removal')">撤销</button>
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
  /* 拖拽平移：空白处可拖（节点与行标签是拖拽对象） */
  cursor: grab;
  /* 水平不留内边距：clientWidth 即内容宽（画布 1:1 的前提） */
  padding: 0;
  /* 滚动条位置固定：缩放时滚动条出现 / 消失不会挤动容器宽度（日期列不漂移） */
  scrollbar-gutter: stable;
}

/* 顶部状态提示（只出现在「无成型候选」空态；占流不遮日期轴） */
.lcc-canvas__notice {
  flex: none;
  margin: 0;
  padding: 5px 12px;
  border-bottom: 1px solid var(--color-border-subtle);
  color: var(--color-text-muted);
  font-size: 11px;
  line-height: 1.6;
}

.lcc-canvas__surface.is-panning {
  cursor: grabbing;
  user-select: none;
}

.lcc-canvas__surface.is-dragging {
  cursor: grabbing;
  /* 拖对象时也不得选中画布文本（仅 is-panning 不够：拖节点不走平移分支） */
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

.lcc-canvas__axis text,
.lcc-canvas__ticks text {
  font-size: 9px;
}

/* 吸顶行：不透明底（盖住滚到下面的行）+ 下缘 1px 分隔与渐隐阴影 */
.lcc-canvas__sticky-bg {
  fill: var(--color-bg-base);
}

/*
  吸顶带 / 标签列的**提档底色**（与画布底色 bg-base 区分开，用户反馈：原来同色，没有分界）。
  必须不透明：叠在上面那层 bg-base 之上，elevated 的 alpha 不会透出滚动内容。
*/
.lcc-canvas__sticky-tint {
  fill: var(--color-bg-elevated);
}

/* 带下缘硬分隔与标签列/内容区竖向分隔（都是 1px，靠 border-medium 而不是阴影） */
.lcc-canvas__sticky-edge,
.lcc-canvas__label-divider {
  stroke: var(--color-border-medium);
  stroke-width: 1;
}

.lcc-canvas__sticky-shadow {
  fill: var(--color-bg-base);
}

/* 行标签列横向吸顶：不透明底（盖住从标签下方滑过的点）+ 右缘 6 级渐隐 */
.lcc-canvas__label-bg,
.lcc-canvas__label-shadow {
  fill: var(--color-bg-base);
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

/* 行分隔线：行分组的唯一横向笔画（无横条胶囊） */.lcc-canvas__row-separator {
  stroke: var(--color-border-subtle);
  stroke-width: 1;
}

.lcc-canvas__row-name {
  fill: var(--color-text-secondary);
  font-size: 11px;
}

.lcc-canvas__row-name.is-lane-name {
  fill: var(--color-accent);
  font-weight: 600;
}

.lcc-canvas__row-count {
  fill: var(--color-text-muted);
  font-size: 9px;
  font-family: monospace;
}

.lcc-canvas__row-tally {
  fill: var(--color-text-muted);
  font-size: 9.5px;
  font-family: monospace;
}

.lcc-canvas__row-handle {
  fill: transparent;
  cursor: grab;
}

.lcc-canvas__row:hover .lcc-canvas__row-name,
.lcc-canvas__row:hover .lcc-canvas__row-count {
  fill: var(--color-text-primary);
}

.lcc-canvas__dropzone {
  fill: color-mix(in srgb, var(--color-accent) 6%, transparent);
  stroke: color-mix(in srgb, var(--color-accent) 62%, transparent);
  stroke-width: 1;
  stroke-dasharray: 5 4;
  pointer-events: none;
}

.lcc-canvas__row.is-drop-target .lcc-canvas__dropzone {
  fill: color-mix(in srgb, var(--color-accent) 14%, transparent);
  stroke-width: 1.5;
}

.lcc-canvas__day-more {
  fill: var(--color-text-muted);
  font-size: 9px;
  font-family: monospace;
  cursor: pointer;
}

.lcc-canvas__day-more:hover {
  fill: var(--color-accent);
}

.lcc-canvas__floor-fold {
  fill: var(--color-text-muted);
  font-size: 10px;
  cursor: pointer;
}

.lcc-canvas__floor-fold:hover {
  fill: var(--color-accent);
}

.lcc-canvas__row-more {
  fill: var(--color-text-muted);
  font-size: 10px;
}

/* 检索态：候选链行与地板降为背景，本次泳道行不降级 */
.lcc-canvas__row.is-search-bg {
  opacity: 0.2;
}

.lcc-canvas__node {
  cursor: pointer;
}

.lcc-canvas__hit {
  fill: transparent;
  stroke: none;
}

.lcc-canvas__point {
  /*
   * 深浅 = 接近程度（相对归一后的 0..1 → 18%–80%）：无接近度时 0% = bg-base，外观不变。
   * 阈值不参与这条映射——超阈值的候选链成员/散料也在同一条梯度上。
   */
  fill: color-mix(in srgb, var(--color-accent) var(--proximity-fill, 0%), var(--color-bg-base));
  stroke: var(--color-text-muted);
  stroke-width: 1.4;
}

.lcc-canvas__node.is-readonly .lcc-canvas__point {
  stroke-width: 1.2;
}

/* 描边 = 是否在「可补充的事件」批次里（阈值口径）：与右栏同一批，不各算各的 */
.lcc-canvas__node.is-recommended:not(.is-selected) .lcc-canvas__point {
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
  stroke-width: 1.5;
  stroke-linecap: round;
  stroke-linejoin: round;
  pointer-events: none;
}

/* 星形节点里的 ✓ 更细（内切圆小，粗线会糊成一团） */
.lcc-canvas__check.is-star {
  stroke-width: 1.1;
}

/* ★ 星形主体：只让星角圆润，不改 fill/stroke（沿用 .lcc-canvas__point 的各态规则） */
.lcc-canvas__point[data-role="key-star"] {
  stroke-linejoin: round;
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

/* 键盘通道：focus-visible 环建在圆外（与「已选 = 实心 + ✓」不冲突） */
.lcc-canvas__node:focus-visible {
  outline: none;
}

.lcc-canvas__node:focus-visible .lcc-canvas__focus-ring {
  opacity: 1;
  stroke: var(--color-accent);
  stroke-width: 2;
}

/* 相关高亮 / 弱化（焦点在场时；取消聚焦恢复默认，不残留）。 */
.lcc-canvas__node.is-related .lcc-canvas__point {
  stroke: var(--color-accent);
  stroke-width: 2;
}

.lcc-canvas__node.is-dimmed {
  opacity: 0.35;
}

/* 行标签 hover：提亮该行全部成员（整组拖动的可拖提示） */
.lcc-canvas__node.is-row-hover .lcc-canvas__point {
  fill: color-mix(in srgb, var(--color-accent) 18%, var(--color-bg-base));
  stroke: var(--color-accent);
}

/* 检索态：候选链层降为背景；未命中节点不可点 */
.lcc-canvas__node.is-search-bg {
  opacity: 0.2;
}

.lcc-canvas__node.is-search-muted {
  pointer-events: none;
}

/* 检索命中 = 四角括号（形状与「已选 = 实心 + ✓」「相关 = 加粗描边」不同源） */
.lcc-canvas__hit-bracket {
  fill: none;
  stroke: var(--color-accent);
  stroke-width: 1.1;
  stroke-linecap: square;
  pointer-events: none;
}

.lcc-canvas__node-title {
  fill: var(--color-text-secondary);
  font-size: 11px;
  pointer-events: none;
}

/* 相关节点标题：与常显标题同一套（同一个 <text>），样式不再区分（旧 9px 档已删） */
.lcc-canvas__node.is-search-hit .lcc-canvas__node-title {
  fill: var(--color-accent);
  font-weight: 600;
}

/* 拖拽 ghost：放大 1.6× 跟随；整组拖带成员数 */
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

.lcc-drag-ghost.is-group {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  margin: -11px 0 0 -11px;
}

.lcc-drag-ghost__count {
  font-size: 10px;
  font-weight: 600;
  color: var(--color-accent);
}

.lcc-undo {
  position: absolute;
  right: 12px;
  bottom: 44px;
  margin: 0;
  padding: 4px 8px;
  border: 1px solid var(--color-border-medium);
  border-radius: 3px;
  background: var(--color-bg-base);
  color: var(--color-text-muted);
  font-size: 10px;
  line-height: 1.6;
}

.lcc-undo__btn {
  margin-left: 4px;
  border: 0;
  background: transparent;
  color: var(--color-accent);
  font-size: 10px;
  cursor: pointer;
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

.lcc-card__key {
  display: block;
  font-size: 11px;
  margin-top: 4px;
  line-height: 1.6;
  color: var(--color-accent);
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
