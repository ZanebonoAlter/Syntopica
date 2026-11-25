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
 * 行集合只含持久化话题（D2）：候选话题不占行，其确认事件**不呈现**（时间轴与卡片列都没有）。
 *
 * 布局（keyEventLayout.ts 纯函数 + 两阶段测量；key-event-day-focus-cards 双带）：
 *  - 列宽 = max(64, 可用宽 / 可见日数)，超宽屏均分填满、窄屏撑宽后舞台内横滚；
 *  - 标签先自然宽度渲染（visibility:hidden 保留布局盒），`offsetWidth` 实测后归带
 *    分层（双带：上带 / 轴带 / 下带；层距 32px、同层间隙 ≥4px、贴右缘整组左移），
 *    **不截断标题**；实测对象必须是真实 DOM 元素（组件 ref 拿到的是公开实例，量不到宽度）；
 *  - 行体三段：上带（La 层）· 轴带（18px，生命线居中、圆点骑线）· 下带（Lb 层）；
 *    轴带随上带层数下移（axisTop = 8（顶部留白）+ 32 × La + 8（带间留白），
 *    生命线 = axisTop + 9）；**收起行按 La = Lb = 0**（标签已隐藏 → 轴带贴行顶，
 *    节点 MUST NOT 跑出本行）；
 *    标签行高 = min(246, max(70, 50 + 32 × (La + Lb)))；
 *  - 每条标签一根引线（单层 SVG 覆盖层，画在标签之下）：自圆点外缘垂直走到标签近缘，
 *    圆点 x 落在标签横向范围内时只画竖段；描边走 --color-event；
 *  - 行体显式宽度 = 轴宽：只含绝对定位子元素时宽度会塌成 0，虚线轴（生命线）
 *    跟着撑不开；
 *  - 节点恒在生命线（轴线）上，同日多条事件**横向**散开区分（MUST NOT 竖向压到别层
 *    标签）；步长 = min(24, (列宽 − 节点宽) / (k − 1))，同日节点恒在本列内；一列装不下
 *    （k × 节点宽 + (k−1) × 间隙 > 列宽）时该日降级为**单节点 + 当日计数**；
 *    **收起行每天只出一个节点**（承载「11.05 ★N」日期前缀 + 计数盒）——逐事件扇开会让
 *    带日期的计数盒越出本列、侵入邻列（review R-30）；
 *    节点描边与文字用 --color-event，选中日节点实心填充改用 --color-accent
 *    （琥珀 = 事件、红 = 选中，同一颜色 MUST NOT 表达两义）；
 *  - 行头内容按生命线对齐（内容块中心 = 生命线 + translateY(-50%)），不随行高居中；
 *  - 标签收起只看**该行层数**（口径 = 上带 + 下带合计），不看观察窗天数（design D7/D14）：
 *    层数 ≤ 4 照常显示名称，超上限的行才退化为节点 + 当日事件计数；
 *    另含两条几何无解的收起路径：归带溢出 / 组内引线穿组内标签（未校验的层
 *    MUST NOT 当可用 → 收起）；
 *  - 空白处左键拖拽平移（复用 laneComposeLogic 的 isDragBeyond/panScroll，阈值 5px）；
 *  - 行头 sticky left；选中日整列淡底；切换/换日/**窗口变化**时选中日滚入视口；
 *  - 悬停强调是视图内瞬态（design D11）：加粗该事件的引线 / 节点 / 轴头日期，
 *    MUST NOT 改选中态。
 *
 * 日聚焦卡片列（key-event-day-focus-cards 3.2/3.3）：选中日在**所有**有该日事件的
 * 泳道行内就地展开卡片（`KeyEventDayCard.vue`）；卡片列放下带（顶 = 轴带下缘 + 8、
 * 左缘 = 卡片矩形左缘、宽固定 280px），卡片矩形参与归带判定（相交则该组改上带）；
 * 卡片堆叠高实测（一次性读 offsetHeight 求和）→ 行高取 max(标签行高, 卡片列占位高)，
 * 卡片一侧**不受 246px 封顶约束**；> 4 张固定高度内部滚动 + 标「还有 N 条」。
 *
 * 视图头与行头读数（tasks 4.1–4.3；review R-28：读数三态，**未知 ≠ 0 条**）：
 * 视图头「所属日期」下拉（选项 = 当前轴全部日期，选中即 `emit('selectDate')` 写入
 * 与矩阵**同一个** selectedDate，本视图不存第二份）+ 选中日读数（全部泳道在该日的
 * 确认事件数）；行头补「当日 k 条」（该泳道在选中日的确认事件数）。
 * 读取中 / 首读失败 / 旧快照一律**不显示 0**（视图头说「读取中」/「读取失败」/
 * 「旧快照」，行头不渲染）；只有成功响应覆盖该日才显示「0 条」；旧快照有数据时
 * 数字带「（旧快照）」标注。选中日全泳道 0 条时轴上方一行
 * 「MM/DD 没有确认的关键事件」（MUST NOT 用它代替视图头的读数；整窗无事件时**不叠**
 * 这一行，review R-29）。
 *
 * 休眠泳道选中日自动展开（design D9 / tasks 5.2）：视图内**本地**派生状态
 * `autoDormantDate`（MUST NOT 写壳层共用的 `expandedDormant`——那会让矩阵视图的
 * 休眠组跟着弹开）——选中日在休眠行有确认事件时自动展开当天有事件的那几行；
 * 用户点折叠按钮即清空（**用户显式折叠优先**，直到选中日再变）。
 * 折叠按钮分两条路径（review R-27）：**本地自动展开态**点「收起」只清本地状态、
 * **MUST NOT emit `toggleDormant`**（发了会把壳层共用值翻成 true → 全部休眠行弹开
 * 且矩阵视图被同步打开）；只有壳层 `expandedDormant` 为真时才 emit。
 *
 * 下方「当日关键事件」阅读区已退役（tasks 5.1）：其能力（标题 / 摘要 / 关联文章 →
 * 站内阅读）由泳道内的日聚焦卡片承接。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { groupUnassignedEvents, useBoardKeyEvents } from '~/features/tags/composables/useBoardKeyEvents'
import type { LaneDayReadingStore } from '~/features/tags/composables/useLaneDayReading'
import { isDragBeyond, panScroll } from '~/features/tags/composables/laneComposeLogic'
import type { BoardKeyEventItem } from '~/api/boardKeyEvents'
import type { ObservationLaneRow } from './observationLanes'
import KeyEventLabel from './KeyEventLabel.vue'
import KeyEventDayCard from './KeyEventDayCard.vue'
import {
  computeEventColumnWidth,
  computeKeyEventRequestDays,
  keyEventDayCardRect,
  keyEventLabelsCollapsed,
  keyEventLeaderPath,
  keyEventRowHeight,
  keyEventRowHeightWithCards,
  layoutKeyEventBands,
  nodeFanOffset,
  KEY_EVENT_AXIS_BAND_HEIGHT,
  KEY_EVENT_BAND_LAYER_STEP,
  KEY_EVENT_BAND_PADDING,
  KEY_EVENT_CARD_WIDTH,
  KEY_EVENT_LABEL_HEIGHT,
  KEY_EVENT_NODE_FAN_STEP,
  type KeyEventBandGroup,
  type KeyEventBandPlacement,
  type KeyEventCardRect,
} from './keyEventLayout'

/**
 * 行头宽（px；容纳话题名两行 + 事件计数，与矩阵标签列量级一致）。
 *
 * 同时是列宽公式里的行头占位（`(舞台宽 − 192) / 可见日数`）与行头列的固定宽：
 * 行头列在滚动区外（R-33），两处必须同值，否则列与行头会错位。
 */
const HEADER_WIDTH = 192

/** 拖拽平移阈值（px；未超阈值视为点选，design D11）。 */
const DRAG_THRESHOLD = 5

/**
 * 轴带顶相对行顶的留白（px）。
 *
 * 轴带纵向位置随**上带层数**下移：axisTop = 顶部留白 8 + 32 × La + 带间留白 8；
 * 生命线 = axisTop + 9（轴带 18px 居中），圆点中心骑在生命线上。
 *
 * 与行高公式自洽：`50 + 32 × (La + Lb)` 里的 50 = 顶部留白 8 + 带间留白 8
 * + 轴带 18 + 带间留白 8 + 底部留白 8——少算这 8px 带间留白，行底就会多出 8px 死白。
 */
const AXIS_TOP_OFFSET = 8 + KEY_EVENT_BAND_PADDING

/** 圆点半径（px；18px 圆点）：节点上沿 = 生命线 − 半径。 */
const NODE_RADIUS = 9

/** 节点直径（px；.kev-node 的 min-width / height）。 */
const NODE_SIZE = NODE_RADIUS * 2

/** 同日相邻节点的最小水平间隙（px）。 */
const NODE_MIN_GAP = 4

/**
 * 卡片列容量上限（design D6）：超过 4 张 → 固定高度内部滚动 + 标「还有 N 条」；
 * ≤ 4 张全部展开且该行**不设行高封顶**（卡片行高不受 246px 标签封顶约束）。
 */
const CARD_LIMIT = 4

/** 卡片之间的间距（px；堆叠高 = 实测高之和 + 间距 × (张数 − 1)）。 */
const CARD_GAP = 8

const props = defineProps<{
  boardId: number
  /** 版块名（stale 提示点名来源版块用；发布时快照，不随切版块变）。 */
  boardLabel?: string
  /** 可见观察轴（与矩阵同一 visibleAxis；升序 date-only）。 */
  axis: string[]
  activeRows: ObservationLaneRow[]
  dormantRows: ObservationLaneRow[]
  /**
   * 父组件仍传入（壳层与矩阵共用同一份选中态），但本视图**不再据此渲染**任何选中视觉：
   * 行头已改为不可点的只读列（2026-11-24 用户反馈「选中没有联动功能」）。
   */
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
  /**
   * 本视图行头不再可点（2026-11-24 用户反馈：无联动下游，假选中易误解）；
   * 声明保留仅为父组件 `@select-lane` 绑定的契约兼容，实现里 MUST NOT 再 emit。
   */
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
  events,
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
 * 行集合话题 ID（只含持久化话题：active 行 + 休眠折叠行）。
 *
 * 候选/归档/无分区/无归属话题不占行（spec「候选话题不占行」），其确认事件不进
 * 这些行的投影——而是由下方「其他」兜底行统一承接，MUST NOT 丢弃。
 */
const rowTopicIds = computed(() => new Set([
  ...props.activeRows.map(row => row.topicId),
  ...props.dormantRows.map(row => row.topicId),
]))

/** 「其他」兜底行伪 topicId（仅本视图内部使用；MUST NOT 写入壳层 selectedLane）。 */
const OTHER_ROW_TOPIC_ID = '__other__'

/** 「其他」兜底行的跨版块门槛：展示中的事件必须真属于当前版块。 */
const eventsBoardMatches = computed(() => eventsSource.value?.boardId === props.boardId)

/** 「其他」行事件：未归属任何已显示话题行的 confirmed 注记（按日期分组，轴内）。 */
const otherEventsByDate = computed(() => {
  const out = new Map<string, BoardKeyEventItem[]>()
  // 跨版块隔离：stale 保留的事件属于旧版块时，既不能进正式行，也不能被兜底行收进来
  // （events 无 board 字段，来源以 contentSource 为准；首次加载未发布时同样为空）。
  if (!eventsBoardMatches.value) return out
  for (const [date, list] of groupUnassignedEvents(events.value, rowTopicIds.value)) {
    if (!axisSet.value.has(date)) continue
    out.set(date, list)
  }
  return out
})

/** 当日事件（全量：持久化话题行 + 「其他」行；接口序，轴内过滤）。 */
const dayEventsByDate = computed(() => {
  const out = new Map<string, BoardKeyEventItem[]>()
  if (!eventsBoardMatches.value) return out
  for (const [date, list] of eventsByDate.value) {
    if (!axisSet.value.has(date) || list.length === 0) continue
    out.set(date, list)
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

// ── 休眠泳道选中日自动展开（design D9；视图内本地派生，不写壳层共用状态）──────

/**
 * 自动展开日（视图内**本地**状态）。
 *
 * 规则（design D9）：选中日在休眠行里有确认事件 → 自动展开当天有事件的那几行；
 * 用户点休眠组折叠按钮 → 清空（**用户显式折叠优先**，直到选中日再变）。
 *
 * MUST NOT 写 `useBoardConsole.ts` 的共用 `expandedDormant`——那是矩阵与关键事件视图
 * 共用的状态，写进去会让矩阵视图的休眠组跟着弹开（design D9 取舍）。
 */
const autoDormantDate = ref<string | null>(null)

/** 用户显式折叠（优先于自动展开；选中日再变才解除）。 */
const dormantCollapsedByUser = ref(false)

/** 休眠行在给定日期有确认事件（自动展开的判据）。 */
function dormantHasEventsOn(date: string | null): boolean {
  if (!date) return false
  return dormantProjections.value.some(projection => (projection.byDate.get(date)?.length ?? 0) > 0)
}

/** 同步自动展开日（按当前选中日 + 休眠行事件）。 */
function syncAutoDormant(): void {
  const date = props.selectedDate
  autoDormantDate.value = dormantHasEventsOn(date) ? date : null
}

/**
 * 休眠行有事件的日期集合签名（数据到位晚于选中日时也能自动展开）。
 *
 * 只用日期集合（不含条数）：同一批日期的条数变化不该反复触发展开同步。
 */
const dormantDaySignature = computed(() => {
  const days = new Set<string>()
  for (const projection of dormantProjections.value) {
    for (const [date, events] of projection.byDate) {
      if (events.length > 0) days.add(date)
    }
  }
  return `${props.selectedDate ?? ''}|${[...days].sort().join(',')}`
})

// 选中日变化 → 解除「用户显式折叠」并重新同步（design D9 规则①）。
watch(() => props.selectedDate, () => {
  dormantCollapsedByUser.value = false
  syncAutoDormant()
})

// 休眠事件数据到位晚于选中日 → 同样自动展开；用户显式折叠过则不打扰。
watch(dormantDaySignature, () => {
  if (dormantCollapsedByUser.value) return
  syncAutoDormant()
}, { immediate: true })

/**
 * 渲染的休眠行：壳层展开 → 全部休眠行；否则只取「自动展开日在当日有事件」的行。
 */
const renderedDormant = computed(() => {
  if (props.expandedDormant) return dormantProjections.value
  const date = autoDormantDate.value
  if (!date) return []
  return dormantProjections.value.filter(projection => (projection.byDate.get(date)?.length ?? 0) > 0)
})

/**
 * 休眠组折叠按钮（review R-27：**两条路径分开**，MUST NOT 污染矩阵共用状态）。
 *
 * - **壳层已展开**（`props.expandedDormant === true`）：点「收起」= 真收起全部休眠行
 *   → `emit('toggleDormant')`（壳层把共用值翻回 false，矩阵视图同步收起）。
 * - **本地自动展开**（壳层未展开，只是本视图命中选中日自动展开那几行）：点「收起」
 *   只清本地状态 + 锁到换日为止，**MUST NOT emit** —— emit 会把壳层共用值翻成
 *   true，全部休眠行弹开，且**矩阵视图被同步打开**（本视图的操作 MUST NOT 改
 *   矩阵视图表现）。
 * - 两者都没展开：点「展开查看」→ 交壳层展开（共用状态的唯一入口）。
 */
function onToggleDormant(): void {
  if (!dormantExpanded.value) {
    // 展开路径：交壳层展开全部休眠行。
    emit('toggleDormant')
    return
  }
  dormantCollapsedByUser.value = true
  autoDormantDate.value = null
  if (props.expandedDormant) emit('toggleDormant')
}

/** 休眠行当前是否可见（壳层展开 **或** 自动展开命中了行；组标题 / 箭头按它算）。 */
const dormantExpanded = computed(() => props.expandedDormant || renderedDormant.value.length > 0)

/**
 * 「其他」兜底行投影（spec「关键事件『其他』兜底行」）。
 *
 * 收容所有不在行集合里的事件：未关注 candidate / archived / 专题已删除（null）/
 * 窗口内无分区未成行的话题。轴内 0 条 → null（**有事件才渲染**，不造空行）。
 */
const otherProjection = computed<RowProjection | null>(() => {
  const byDate = new Map<string, BoardKeyEventItem[]>()
  let count = 0
  let last: string | null = null
  for (const [date, list] of otherEventsByDate.value) {
    if (list.length === 0) continue
    byDate.set(date, list)
    count += list.length
    if (last === null || date > last) last = date
  }
  if (count === 0) return null
  return {
    row: {
      topicId: OTHER_ROW_TOPIC_ID,
      label: '其他',
      watchLinked: false,
      sectionTotal: 0,
      clueTotal: 0,
      dormant: false,
      snapshot: null,
    },
    eventCount: count,
    lastEventDate: last,
    byDate,
  }
})

const otherProjections = computed(() => (otherProjection.value ? [otherProjection.value] : []))

/**
 * 可见行投影（渲染与布局的唯一行序列）：活跃行 → 可见休眠行 → 「其他」。
 *
 * 兜底行复用正式行的全套布局（双带标签 / 引线 / 卡片列 / 行高测算），
 * 不得单独实现一套（design D3）。
 */
const visibleProjections = computed(() => [
  ...activeProjections.value,
  ...renderedDormant.value,
  ...otherProjections.value,
])

/** 窗口内 confirmed 事件总数（空态判定用；含休眠行与「其他」行）。 */
const windowEventCount = computed(() =>
  [...activeProjections.value, ...dormantProjections.value, ...otherProjections.value]
    .reduce((sum, p) => sum + p.eventCount, 0))

// ── 视图头 / 行头读数（tasks 4.1–4.3）────────────────────────────────────────

/** topicId → 行投影（活跃 + 休眠 + 其他；行头「当日 k 条」用）。 */
const projectionByTopicId = computed(() => {
  const map = new Map<string, RowProjection>()
  for (const projection of activeProjections.value) map.set(projection.row.topicId, projection)
  for (const projection of dormantProjections.value) map.set(projection.row.topicId, projection)
  if (otherProjection.value) map.set(otherProjection.value.row.topicId, otherProjection.value)
  return map
})

/**
 * 选中日在**全部泳道**的确认事件数（视图头「MM/DD · n 条」）。
 *
 * 口径 = 全量 confirmed（持久化话题行 + 「其他」行），选中日无值时为 0。
 * **注意**：0 只在「读数状态 = ready」时才是有效零值（见 `dayCountKind`）。
 */
const selectedDayCount = computed(() => {
  const date = props.selectedDate
  if (!date) return 0
  return dayEventsByDate.value.get(date)?.length ?? 0
})

/**
 * 事件读数三态（review R-28：**未知 ≠ 0 条**）。
 *
 * - `loading`：事件仍在读 → 任何条数都未知（MUST NOT 冒称「0 条」）；
 * - `error`：首读失败（无旧内容可显示）→ 未知；
 * - `stale`：刷新失败但保留旧内容 → 旧内容里**有**该日事件才认条数（并标注
 *   「旧快照」），没有则仍属未知（旧快照不覆盖该日，不能说「0 条」）；
 * - `ready`：成功响应覆盖整个观察窗 → 条数有效（**含 0**）。
 */
type DayCountKind = 'loading' | 'error' | 'stale' | 'ready'

const dayCountKind = computed<DayCountKind>(() => {
  if (eventsLoading.value) return 'loading'
  if (eventsError.value !== null) return eventsStale.value ? 'stale' : 'error'
  if (eventsStale.value) return 'stale'
  return 'ready'
})

/**
 * 视图头读数文案（review R-28）。
 *
 * - 未知（读取中 / 首读失败 / 旧快照不含该日）：**MUST NOT** 显示「0 条」，
 *   改说「读取中」/「读取失败」/「旧快照」；
 * - 有效零值（成功响应覆盖该日、确无事件）：「MM/DD · 0 条」；
 * - 陈旧但有旧数据：「MM/DD · n 条（旧快照）」——数字不得当作当前事实。
 */
const selectedDayCountText = computed(() => {
  const date = props.selectedDate
  if (!date) return ''
  const md = shortDate(date)
  const kind = dayCountKind.value
  if (kind === 'loading') return `${md} · 读取中`
  if (kind === 'error') return `${md} · 读取失败`
  if (kind === 'stale') {
    return selectedDayCount.value > 0 ? `${md} · ${selectedDayCount.value} 条（旧快照）` : `${md} · 旧快照`
  }
  return `${md} · ${selectedDayCount.value} 条`
})

/** 选中日在**休眠行**里的确认事件数（休眠组标题「当日 M 条」）。 */
const dormantDayCount = computed(() => {
  const date = props.selectedDate
  if (!date) return 0
  return dormantProjections.value.reduce(
    (sum, projection) => sum + (projection.byDate.get(date)?.length ?? 0),
    0,
  )
})

/**
 * 行头「当日 k 条」文案（topicId → 文案；**未知态整行不渲染**，review R-28）。
 *
 * - 读取中 / 首读失败 → 未知 → **不渲染**该读数（MUST NOT 冒称「当日 0 条」）；
 * - 陈旧态且该行旧数据里没有该日事件 → 仍属未知 → 不渲染；
 * - 有效零值（成功响应覆盖该日、该行确无事件）→ 「当日 0 条」。
 */
const rowDayCountTexts = computed(() => {
  const out = new Map<string, string>()
  const date = props.selectedDate
  if (!date) return out
  const kind = dayCountKind.value
  if (kind === 'loading' || kind === 'error') return out
  for (const [topicId, projection] of projectionByTopicId.value) {
    const count = projection.byDate.get(date)?.length ?? 0
    if (kind === 'stale') {
      if (count <= 0) continue
      out.set(topicId, `当日 ${count} 条（旧快照）`)
      continue
    }
    out.set(topicId, `当日 ${count} 条`)
  }
  return out
})

/**
 * 「选中日无事件」说明行（tasks 4.3）。
 *
 * 条件 = 当日全泳道确认事件总数 = 0；读取中 / 读取失败时**不出现**（MUST NOT 冒称
 * 0 条），且无日报 / 无持久化话题时已有各自的空态，不叠另一行「当日无事件」。
 * 该行是补充说明，MUST NOT 代替视图头的「MM/DD · 0 条」读数。
 *
 * **两态互斥**（review R-29）：整窗无事件时由整窗空态说明，不再叠加本行；只有
 * 「窗内其它日有事件、选中日为零」才显示本行。读数未知（读取中 / 失败 / 旧快照
 * 不含该日）时也不显示（MUST NOT 断言「没有事件」）。
 */
const showSelectedDayEmpty = computed(() =>
  props.selectedDate !== null
  && props.emptyState === null
  && windowEventCount.value > 0
  && dayCountKind.value === 'ready'
  && selectedDayCount.value === 0)

/** 说明行文案：「MM/DD 没有确认的关键事件」。 */
const selectedDayEmptyText = computed(() => {
  const date = props.selectedDate
  return date ? `${shortDate(date)} 没有确认的关键事件` : ''
})

/** 周几（date-only → 中文周几；按 UTC 解析避免时区偏移）。 */
const WEEKDAYS = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']

/** 下拉选项文案：「MM/DD 周X」（与卡片首行同一口径）。 */
function dateOptionLabel(date: string): string {
  const parsed = new Date(`${date}T00:00:00Z`)
  const weekday = Number.isNaN(parsed.getTime()) ? '' : (WEEKDAYS[parsed.getUTCDay()] ?? '')
  const md = shortDate(date)
  return weekday ? `${md} ${weekday}` : md
}

/**
 * 下拉选日 → 写入与矩阵**同一个** selectedDate（视图不存第二份选中态）。
 *
 * 选项只有当前轴内的日期（无「不聚焦」项：`reconcileSelectedDate` 对 null 会回落
 * 到最近报日，`emit('selectDate')` 也只收 string）。
 */
function onDateSelect(event: Event): void {
  const value = (event.target as HTMLSelectElement | null)?.value ?? ''
  if (!value) return
  emit('selectDate', value)
}

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
 *
 * **传的是舞台（`.kev-stage`），不是滚动区（R-33）**：行头列在滚动区外，列宽基准
 * 仍是「整舞台宽 − 行头宽」；若改量滚动区，列宽会因去掉行头而变窄。
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

/** 布局宽基准：舞台（滚动区的父级；无父级时退回自身）。 */
function layoutSurface(node: HTMLElement | null): HTMLElement | null {
  return node?.parentElement ?? node
}

/** 实测宽落盘（只接受正数）。 */
function applySurfaceWidth(surface: HTMLElement | null): void {
  const measured = measureSurfaceWidth(layoutSurface(surface))
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
        const measured = Math.max(entry.contentRect.width, target.clientWidth, measureSurfaceWidth(layoutSurface(target)))
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
/** 画布内容宽（≥ 容器宽；列宽被下限撑大时容器横滚）。行头列在滚动区外，不计入本宽。 */
const innerMinWidth = computed(() => `max(100%, ${bodyWidth.value}px)`)

/** 节点中心 x（列中心）。 */
function nodeX(index: number): number {
  return index * columnWidth.value + columnWidth.value / 2
}

/**
 * 同日多节点的横向散开偏移：节点恒在轴线上，同日多条只能横向错开（MUST NOT 竖向压到别层标签）。
 *
 * 步长按**列宽**钳制（review R-22）：step = min(24, (列宽 − 节点宽) / (k − 1))——
 * 同日节点恒在本列内，64px 窄列下相邻两天的节点不再互压。
 */
function nodeFan(eventIndex: number, count: number): number {
  return nodeFanOffset(eventIndex, count, fanStep(count))
}

/**
 * 同日节点的横向散开步长（px）：24px 上限，且 MUST 把 k 个节点钳在本列内。
 *
 * 24px 固定步长在 64px 列宽下会让相邻两天的最近两个节点圆心只距 16px < 节点宽 18px
 * （互相压盖，点选归属取决于绘制顺序 —— review R-22）。
 */
function fanStep(count: number): number {
  if (count <= 1) return KEY_EVENT_NODE_FAN_STEP
  const usable = Math.max(0, columnWidth.value - NODE_SIZE)
  return Math.min(KEY_EVENT_NODE_FAN_STEP, usable / Math.max(1, count - 1))
}

/**
 * 该日能否在本列内放下 k 个节点（k × 节点宽 + (k−1) × 间隙 ≤ 列宽）。
 *
 * 放不下 → 该日**降级为单个节点 + 当日计数**（review R-22）：MUST NOT 压缩列宽
 * （64px 是下限，超宽由舞台内横滚承接）、MUST NOT 恢复竖向叠放（节点恒在生命线上）。
 */
function dayFitsNodes(count: number): boolean {
  if (count <= 1) return true
  return count * NODE_SIZE + (count - 1) * NODE_MIN_GAP <= columnWidth.value
}

// ── 日聚焦卡片列（key-event-day-focus-cards 3.2 / 3.3）────────────────────────

/**
 * 卡片矩形（**整视图共用一份**：左缘只取决于选中列与列宽，与行无关）。
 *
 * 由 `keyEventDayCardRect` 给出（design D6）：左缘 = 选中列左缘；右边放不下时改为
 * 右缘对齐选中列右缘向左展开（MUST NOT 伸出舞台）；行体窄于卡片宽时钳制在
 * [0, bodyWidth] 内并置 `clamped`——**卡片本体仍按 280px 渲染**，溢出部分由舞台
 * 容器内部横向滚动承接（与「列宽下限 64px + 舞台内横滚」同一口径）。
 */
const cardRect = computed(() => {
  const date = props.selectedDate
  if (!date) return null
  const index = props.axis.indexOf(date)
  if (index < 0) return null
  return keyEventDayCardRect(index, columnWidth.value, bodyWidth.value)
})

/** 空列表常量（避免模板里每次渲染新建数组）。 */
const NO_CARD_EVENTS: BoardKeyEventItem[] = []

/** 各行在选中日的卡片事件（topicId → 事件列表；该日无事件的行不进表）。 */
const rowCardEventMap = computed(() => {
  const map = new Map<string, BoardKeyEventItem[]>()
  const date = props.selectedDate
  if (!date) return map
  for (const projection of visibleProjections.value) {
    const events = projection.byDate.get(date)
    if (events && events.length > 0) map.set(projection.row.topicId, events)
  }
  return map
})

/** 该行在选中日的卡片事件（该日无事件 → 该行无卡片列）。 */
function rowCardEvents(topicId: string): BoardKeyEventItem[] {
  return rowCardEventMap.value.get(topicId) ?? NO_CARD_EVENTS
}

/**
 * 该行的卡片矩形（**按行**传入；该行在选中日没有事件 → null）。
 *
 * 卡片矩形是下带的固定障碍区：只有**真有卡片列**的行才需要避让。没有卡片列的行若也
 * 拿同一张矩形去挡，会把它的下带标签无谓赶到上带（层数虚增、提前触发收起）
 * —— review R-23。
 */
function rowCardRect(topicId: string): KeyEventCardRect | null {
  if (rowCardEvents(topicId).length === 0) return null
  return cardRect.value
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
function resolveElement(el: unknown): HTMLElement | null {
  if (!el || typeof HTMLElement === 'undefined') return null
  if (el instanceof HTMLElement) return el
  const inner = (el as { $el?: unknown }).$el
  return inner instanceof HTMLElement ? inner : null
}

function setLabelRef(key: string, el: unknown): void {
  const node = resolveElement(el)
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

/** 每行双带布局结果：标签归属带 / 层序 / 坐标 + 上下带层数。 */
interface RowBandLayout {
  /** 事件 id → 归带结果（左缘 / 宽 / 圆点 x / 带 / 层序）。 */
  placements: Map<string, KeyEventBandPlacement>
  /** 上带层数（轴带随它下移）。 */
  upLayers: number
  /** 下带层数。 */
  downLayers: number
  /** 归带溢出（两带都放不下该组；MUST NOT 把未校验的层当可用 → 该行收起）。 */
  overflowed: boolean
  /** 组内引线穿过组内标签（几何无解兜底；该行收起）。 */
  crossesGroupLabel: boolean
}

/** 每行布局结果：topicId → 双带布局。 */
const rowLayouts = ref(new Map<string, RowBandLayout>())
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

/**
 * 待布局分组（第一阶段产物：行 → 同日组 → 圆点 x）。
 *
 * 同日多条归为一组：组内占连续层、近端统一对齐「该日最右圆点 + 12px」→ 树形分叉
 * （design D3）。标签自然宽在第二阶段回填。
 */
const pendingGroups = computed<Map<string, KeyEventBandGroup[]>>(() => {
  const byTopic = new Map<string, KeyEventBandGroup[]>()
  for (const projection of visibleProjections.value) {
    const groups: KeyEventBandGroup[] = []
    props.axis.forEach((date, index) => {
      const events = projection.byDate.get(date)
      if (!events || events.length === 0) return
      groups.push({
        date,
        items: events.map((event, evIdx) => ({
          key: event.id,
          // 一列装不下 → 该日降级为单节点（全部事件的引线同源于这一个点）：
          // 散开偏移恒 0，MUST NOT 把节点挤出本列（MUST NOT 竖向叠放）。
          dotX: dayFitsNodes(events.length)
            ? nodeX(index) + nodeFan(evIdx, events.length)
            : nodeX(index),
          width: 0,
        })),
      })
    })
    byTopic.set(projection.row.topicId, groups)
  }
  return byTopic
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

/**
 * 第二阶段：实测宽度 → 归带分层（双带；行内独立布局；行高随之确定）。
 *
 * 选中日不出药丸（design D3：当日名称由日聚焦卡片承接）→ 把选中日交给归带函数跳过
 * 该日分组。**卡片矩形作为第 3 参传入**，但**按行**：该行在选中日没有事件时传 null
 * （review R-23）——没有卡片列的行不该被空矩形赶到上带。
 */
async function runLabelLayout(): Promise<void> {
  const groups = pendingGroups.value
  if (groups.size === 0) {
    rowLayouts.value = new Map()
    measuring.value = false
    await measureCardStacks()
    return
  }
  measuring.value = true
  await nextTick()
  const byTopic = new Map<string, RowBandLayout>()
  for (const [topicId, rowGroups] of groups) {
    const measured = rowGroups.map(group => ({
      date: group.date,
      items: group.items.map(item => ({ ...item, width: labelWidthOf(item.key) })),
    }))
    const layout = layoutKeyEventBands(measured, bodyWidth.value, rowCardRect(topicId), props.selectedDate)
    const placements = new Map<string, KeyEventBandPlacement>()
    for (const placement of layout.placements) placements.set(placement.key, placement)
    byTopic.set(topicId, {
      placements,
      upLayers: layout.upLayers,
      downLayers: layout.downLayers,
      overflowed: layout.overflowed === true,
      crossesGroupLabel: layout.placements.some(placement => placement.crossesGroupLabel === true),
    })
  }
  rowLayouts.value = byTopic
  measuring.value = false
  // 卡片已渲染（卡片列的位置只取决于选中列与上带层数，与标签实测宽无关）→ 量卡片高。
  await measureCardStacks()
}

// 布局输入变化才重排：分组（事件 / 轴 / 列宽）、行体宽、选中日（该日不出药丸）、卡片矩形。
watch([pendingGroups, bodyWidth, cardRect, () => props.selectedDate], () => { void runLabelLayout() }, { deep: false })

// ── 卡片列高度测量（design D6：渲染完成后**一次性**读 offsetHeight 求和）──────

/**
 * 卡片列实测堆叠高（topicId → px）。
 *
 * 口径 = 前 `min(4, n)` 张卡片的 `offsetHeight` 之和 + 间距 × (张数 − 1)；n > 4 时
 * 该值即滚动容器的固定高度（超出部分内部滚动）。**一次性测量**，MUST NOT 逐卡挂
 * `ResizeObserver`（design D5/D6 同取舍：观察者成本高于一次批量测量）。
 */
const cardStackHeights = ref(new Map<string, number>())

/** 已渲染的卡片元素（topicId → eventId → 元素）；测量时按行内事件序取前 4 张。 */
const cardEls = new Map<string, Map<string, HTMLElement>>()

function setCardRef(topicId: string, eventId: string, el: unknown): void {
  const node = resolveElement(el)
  let byEvent = cardEls.get(topicId)
  if (!node) {
    byEvent?.delete(eventId)
    if (byEvent && byEvent.size === 0) cardEls.delete(topicId)
    return
  }
  if (!byEvent) {
    byEvent = new Map()
    cardEls.set(topicId, byEvent)
  }
  byEvent.set(eventId, node)
}

/** 元素实测高（offsetHeight 优先；为 0 时退回几何盒）。 */
function measureElementHeight(node: HTMLElement | null): number {
  if (!node) return 0
  const byOffset = node.offsetHeight
  if (byOffset > 0) return byOffset
  const rect = typeof node.getBoundingClientRect === 'function' ? node.getBoundingClientRect() : null
  return rect ? rect.height : 0
}

/** 读一轮卡片堆叠高（不写回）：前 `min(4, n)` 张卡的实测高之和 + 间距 × (张数 − 1)。 */
function readCardStacks(): Map<string, number> {
  const next = new Map<string, number>()
  for (const [topicId, byEvent] of cardEls) {
    const events = rowCardEvents(topicId)
    let sum = 0
    let counted = 0
    for (const ev of events) {
      if (counted >= CARD_LIMIT) break
      const el = byEvent.get(ev.id)
      if (!el) continue
      sum += measureElementHeight(el)
      counted += 1
    }
    if (counted === 0) continue
    next.set(topicId, sum + (counted - 1) * CARD_GAP)
  }
  return next
}

/** 两份堆叠高是否一致。 */
function sameCardStacks(a: Map<string, number>, b: Map<string, number>): boolean {
  if (a.size !== b.size) return false
  for (const [topicId, height] of a) {
    if (b.get(topicId) !== height) return false
  }
  return true
}

/**
 * 重新量一次卡片堆叠高（值未变则不写回，避免无谓重渲染）。
 *
 * **逐轮重测（azure R-31）**：一轮测 → 写回 → 下一轮再测，最多两轮。第二轮兜住
 * 「卡片高度又被上一轮的写回带动、晚一轮才落地」的情形（摘要全文 / 关联文章行），
 * 使行高在**同一轮交互**内就取到最终实测值，不留「下一行标签压住卡片按钮」的缝。
 * 不引入 `ResizeObserver`（design D6 取舍不变）。
 */
async function measureCardStacks(): Promise<void> {
  let written = cardStackHeights.value
  for (let pass = 0; pass < 2; pass += 1) {
    await nextTick()
    const measured = readCardStacks()
    if (sameCardStacks(measured, written)) return
    cardStackHeights.value = measured
    written = measured
  }
}

/**
 * 卡片列尺寸观察者：卡片自身变高（展开摘要 / 文章行到位 / 字体回流）**任何原因**
 * 都要让行高跟上 —— 不能只靠「点击」这一个入口（用户反馈：点击卡片展开后行高没自适应，
 * 卡片溢出压住下一行标签）。
 *
 * 只挂**每行一个**（观察 `.kev-cards` 容器，不逐卡挂），成本与行数同阶；`ResizeObserver`
 * 在 SSR / happy-dom 下可能不存在，缺失时静默退回「点击 + 两轮重测」路径。
 */
let cardObserver: ResizeObserver | null = null
const observedCardColumns = new Map<string, Element>()

function setCardColumnRef(topicId: string, el: unknown): void {
  const node = resolveElement(el)
  const prev = observedCardColumns.get(topicId)
  if (prev && prev !== node) {
    cardObserver?.unobserve(prev)
    observedCardColumns.delete(topicId)
  }
  if (!node) return
  observedCardColumns.set(topicId, node)
  cardObserver?.observe(node)
}

/** 建观察者（挂载时一次），并补挂已登记的卡片列。 */
function installCardObserver(): void {
  if (typeof ResizeObserver === 'undefined') return
  cardObserver = new ResizeObserver(() => { void measureCardStacks() })
  for (const el of observedCardColumns.values()) cardObserver.observe(el)
}

/**
 * 卡片内容变化后重测（摘要就地展开 / 文章到位 → 卡片自身变高 → 行高跟上）。
 *
 * 触发点：卡片列内的任何点击（展开 / 收起）+ 关联文章条目状态变化。两者都只
 * “再量一次”，不引入观察者（design D6）。
 */
let cardMeasureScheduled = false
function scheduleCardMeasure(): void {
  if (cardMeasureScheduled) return
  cardMeasureScheduled = true
  void nextTick(() => {
    cardMeasureScheduled = false
    void measureCardStacks()
  })
}

/** 卡片内关联文章条目状态（文章到位 → 卡片变高 → 重测；随 store 变更而变）。 */
const relatedStatusVersion = computed(() => {
  const parts: string[] = []
  for (const projection of visibleProjections.value) {
    for (const ev of rowCardEvents(projection.row.topicId)) {
      for (const articleId of ev.articleIds) {
        parts.push(`${articleId}:${props.reading.readRelated(articleId).status}`)
      }
    }
  }
  return parts.join('|')
})

watch(relatedStatusVersion, () => { void measureCardStacks() })

/** 该行标签层数（上带 + 下带合计；收起判据与行高口径一致）。 */
function rowLayerCount(topicId: string): number {
  const layout = rowLayouts.value.get(topicId)
  return (layout?.upLayers ?? 0) + (layout?.downLayers ?? 0)
}

/**
 * 行高 = max(标签行高, 卡片列占位高度)（design D6）。
 *
 * 标签行高仍按层数公式 `min(246, max(70, 50 + 32 × 层数))`（4 层 = 178px）；
 * **卡片一侧不受 246px 封顶约束**——≤ 4 张卡片必须全部展开，行高放不下就长高
 * （MUST NOT 截断卡片）。
 */
function rowHeight(topicId: string): number {
  return keyEventRowHeightWithCards(
    keyEventRowHeight(rowLayerCount(topicId)),
    cardColumnHeight(topicId),
  )
}

/**
 * 卡片列定位样式：左缘 = 卡片矩形左缘；顶 = 轴带下缘 + 8（放下带）；宽固定 280px。
 *
 * **卡片宽度不随 `clamped` 收缩**：行体窄于 280px 时矩形被钳制，卡片仍按 280px
 * 渲染，溢出部分由舞台容器内部横向滚动承接（design D6）。
 */
function cardColumnStyle(topicId: string): Record<string, string> {
  const rect = cardRect.value
  if (!rect) return {}
  return {
    left: `${rect.left}px`,
    top: `${rowAxisBottom(topicId) + KEY_EVENT_BAND_PADDING}px`,
    width: `${KEY_EVENT_CARD_WIDTH}px`,
  }
}

/** 卡片列滚动容器的内联样式：超过 4 张 → 固定高度（内部滚动）。 */
function cardScrollStyle(topicId: string): Record<string, string> {
  if (rowCardEvents(topicId).length <= CARD_LIMIT) return {}
  const stack = cardStackHeights.value.get(topicId) ?? 0
  if (stack <= 0) return {}
  return { height: `${stack}px` }
}

/**
 * 该行标签层数是否超上限（超了才退化，design D7/D14）；未测量时为 0 层 = 不收起
 * （首帧乐观显示标签，避免测量前误判成「超限」）。
 *
 * **另含两条几何无解的收起路径**（M2 遗留接线，review R-15/R-16）：
 *  - 归带溢出（两带都找不到可用的连续层）：未校验的层 MUST NOT 当可用 → 收起；
 *  - 组内引线穿过组内标签（几何无解兜底）：收起（节点 + ★N，不画标签与引线）。
 */
function rowCollapsed(topicId: string): boolean {
  const layout = rowLayouts.value.get(topicId)
  if (!layout) return false
  if (layout.overflowed || layout.crossesGroupLabel) return true
  return keyEventLabelsCollapsed(layout.upLayers + layout.downLayers)
}

/**
 * 卡片列占位高度（**从行顶起**，design D6）：轴带下缘 + 8 + 卡片堆叠高 + 8。
 *
 * 卡片列的纵向起点 = 轴带下缘 + 8（放下带），故从行顶算起要先加上轴带位置。
 */
function cardColumnHeight(topicId: string): number {
  // 不再以 `cardRect` 是否存在为前提：矩形只决定卡片列的横向位置，与行高无关；
  // 若在矩形算出前就跳过，行高会停在标签高度、卡片直接压到下一行（用户反馈）。
  if (rowCardEvents(topicId).length === 0) return 0
  const stack = cardStackHeights.value.get(topicId) ?? 0
  return rowAxisBottom(topicId) + KEY_EVENT_BAND_PADDING + stack + KEY_EVENT_BAND_PADDING
}

/**
 * 上带层数（**收起行按 0 计**，review R-21）。
 *
 * 收起行的标签已经隐藏（只剩节点 + 计数），若仍按被隐藏的上带层数定位轴带，节点会
 * 跑到本行之外（行高封顶 246px，节点顶却在 16 + 32 × 8 = 272px）。收起时轴带贴行顶
 * （La = Lb = 0），生命线、节点与卡片列高度同用一个位置。
 */
function rowUpLayers(topicId: string): number {
  if (rowCollapsed(topicId)) return 0
  return rowLayouts.value.get(topicId)?.upLayers ?? 0
}

/** 轴带顶（行内 y）：随上带层数下移（axisTop = 8 + 8 + 32 × La）。 */
function rowAxisTop(topicId: string): number {
  return AXIS_TOP_OFFSET + KEY_EVENT_BAND_LAYER_STEP * rowUpLayers(topicId)
}

/** 生命线 y（轴带居中）：圆点中心恒骑在它上面。 */
function rowLifelineY(topicId: string): number {
  return rowAxisTop(topicId) + KEY_EVENT_AXIS_BAND_HEIGHT / 2
}

/** 轴带下缘 y。 */
function rowAxisBottom(topicId: string): number {
  return rowAxisTop(topicId) + KEY_EVENT_AXIS_BAND_HEIGHT
}

/** 标签上缘（上带贴轴带上、下带贴轴带下；层距 32px、带间留白 8px）。 */
function labelTop(topicId: string, placement: KeyEventBandPlacement): number {
  return placement.band === 'up'
    ? rowAxisTop(topicId) - KEY_EVENT_BAND_PADDING - KEY_EVENT_BAND_LAYER_STEP * placement.layer - KEY_EVENT_LABEL_HEIGHT
    : rowAxisBottom(topicId) + KEY_EVENT_BAND_PADDING + KEY_EVENT_BAND_LAYER_STEP * placement.layer
}

/** 行的标签渲染模型（双带定位）。 */
interface RowLabelView {
  event: BoardKeyEventItem
  /** 归带结果（null = 还没出布局结果 → 标签隐藏）。 */
  placement: KeyEventBandPlacement | null
  left: number
  top: number
}

/**
 * 行的标签渲染模型：双带坐标（左缘 = 归带结果；上缘按带与层序算）。
 *
 * 测量阶段全部标签先渲染（visibility:hidden 保留布局盒）供 offsetWidth 实测；
 * 布局出来后没有归属的（选中日由日聚焦卡片承接）不再渲染。
 */
function rowLabelViews(topicId: string, projection: RowProjection): RowLabelView[] {
  const layout = rowLayouts.value.get(topicId)
  const views: RowLabelView[] = []
  props.axis.forEach((date, index) => {
    const events = projection.byDate.get(date) ?? []
    events.forEach((event, evIdx) => {
      const placement = layout?.placements.get(event.id) ?? null
      views.push({
        event,
        placement,
        left: placement?.left ?? nodeX(index) + nodeFan(evIdx, events.length),
        top: placement ? labelTop(topicId, placement) : 0,
      })
    })
  })
  return views.filter(view => measuring.value || view.placement !== null)
}

/** 一根引线（SVG path 的 d + 归属事件）。 */
interface RowLeader {
  key: string
  d: string
}

/**
 * 行的引线几何：每条标签恰好一根（起点 = 该事件圆点 x）。
 *
 * 竖段自圆点外缘（上带 = 圆点顶缘、下带 = 圆点底缘）走到标签近缘；圆点 x 落在标签
 * 横向范围内时只画竖段，否则补一段横段接入标签近端（design D4）。
 */
function rowLeaders(topicId: string): RowLeader[] {
  // 收起行没有标签（只剩节点 + ★计数）→ 不画引线（不然是连向空处的线）。
  if (rowCollapsed(topicId)) return []
  const layout = rowLayouts.value.get(topicId)
  if (!layout) return []
  const lifelineY = rowLifelineY(topicId)
  const leaders: RowLeader[] = []
  for (const placement of layout.placements.values()) {
    const path = keyEventLeaderPath({
      dotX: placement.dotX,
      dotY: placement.band === 'up' ? lifelineY - NODE_RADIUS : lifelineY + NODE_RADIUS,
      labelLeft: placement.left,
      labelTop: labelTop(topicId, placement),
      labelWidth: placement.width,
      band: placement.band,
    })
    leaders.push({ key: placement.key, d: path.d })
  }
  return leaders
}

/**
 * 节点样式：骑在生命线上（top = 生命线 − 半径）+ 同日多条横向散开。
 *
 * 收起行同样骑在生命线上（MUST NOT 竖向叠放下压，delta spec「关键事件节点与名称」）。
 */
function nodeStyle(topicId: string, eventIndex: number, count: number): Record<string, string> {
  return {
    top: `${rowLifelineY(topicId) - NODE_RADIUS}px`,
    transform: `translateX(calc(-50% + ${nodeFan(eventIndex, count)}px))`,
  }
}

/** 一行某日的节点渲染视图（review R-22：装不下 → 单节点 + 当日计数）。 */
interface RowNodeView {
  event: BoardKeyEventItem
  /** 散开序号（降级为单节点时恒 0）。 */
  index: number
  /** 同日渲染节点数（散开步长按它算）。 */
  count: number
  /** 当日事件总数（★N 用）。 */
  total: number
  /** 是否显示「★N」。 */
  showCount: boolean
}

/**
 * 一行某日的节点列表。
 *
 * - **收起行**：标签已撤下，**每天只出一个节点**（承载该日日期前缀 + 计数「11.05 ★N」，
 *   review R-30 / 2026-11-24 用户反馈）——逐事件扇开时，带日期的计数盒（盒宽 > 18px，
 *   只有 min-width 无上限）会越出本列、侵入邻列；单节点居中后盒宽（~50px）
 *   仍 ≤ 64px 列宽下限，跨列不相交。
 * - **一列装不下**（k × 节点宽 + (k−1) × 间隙 > 列宽）：该日**降级为单个节点 +
 *   当日计数**，同日其余事件的引线同源于这一个点（MUST NOT 压缩列宽、
 *   MUST NOT 竖向叠放）。
 */
function rowNodes(projection: RowProjection, date: string): RowNodeView[] {
  const events = projection.byDate.get(date) ?? []
  if (events.length === 0) return []
  const total = events.length
  const collapsed = rowCollapsed(projection.row.topicId)
  const degraded = !dayFitsNodes(total)
  // 收起行与「一列装不下」都只出一个节点；其余行保持「同日扇开」。
  const shown = (collapsed || degraded) ? events.slice(0, 1) : events
  const count = shown.length
  return shown.map((event, index) => ({
    event,
    index,
    count,
    total,
    showCount: (collapsed || degraded) && index === 0,
  }))
}

// ── 悬停强调（design D11）：视图内瞬态，MUST NOT 改选中态 ────────────────────

/** 悬停的事件 id（瞬态，不落持久状态）。 */
const hoveredEventId = ref<string | null>(null)

/** 悬停事件所在日期（轴头日期强调用；由全量事件反查，含无归属事件）。 */
const hoveredDate = computed(() => {
  const id = hoveredEventId.value
  if (!id) return null
  for (const [date, events] of eventsByDate.value) {
    if (events.some(event => event.id === id)) return date
  }
  return null
})

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
  // 滚动区坐标系以行体左缘为 0（行头列在滚动区外，R-33）——不再叠加 HEADER_WIDTH。
  const target = index * columnWidth.value + columnWidth.value / 2
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
  installCardObserver()
  await nextTick()
  applySurfaceWidth(scrollRef.value)
  void runLabelLayout().then(() => scrollSelectedDateIntoView())
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  resizeObserver = null
  observedSurface = null
  cardObserver?.disconnect()
  cardObserver = null
  observedCardColumns.clear()
  panStart = null
})

// ── 状态分层（A 档骨架/首错沿用矩阵；事件层三态独立）────────────────────────

const showSkeleton = computed(() => props.loading && props.activeRows.length === 0 && props.dormantRows.length === 0)
const showLaneError = computed(() =>
  !props.loading && props.error !== null && props.emptyState === null
  && props.activeRows.length === 0 && props.dormantRows.length === 0)
const eventsFirstError = computed(() =>
  !eventsLoading.value && eventsError.value !== null && events.value.length === 0)

function shortDate(date: string): string {
  return date.slice(5).replace('-', '/')
}

/** 收起行节点日期前缀（用户要求形如 11.05，点号分隔）。 */
function dayDot(date: string): string {
  return date.slice(5).replace('-', '.')
}
</script>

<template>
  <section class="kev" data-testid="key-event-view">
    <div class="kev-head">
      <span class="kev-kicker">话题 × 关键事件</span>
      <span class="kev-hint" data-testid="key-event-legend">节点 = 人工确认的关键事件（在日报阅读台标记）· 只读视图</span>
      <!-- 所属日期下拉（tasks 4.1）：选项 = 当前轴全部日期；写入与矩阵同一个 selectedDate -->
      <label class="kev-select">
        <span>所属日期</span>
        <select
          aria-label="所属日期"
          data-testid="key-event-date-select"
          :value="selectedDate ?? ''"
          @change="onDateSelect"
        >
          <option v-for="date in axis" :key="date" :value="date">{{ dateOptionLabel(date) }}</option>
        </select>
      </label>
      <!-- 选中日在全部泳道的当日条数（tasks 4.1；MUST NOT 被「无事件说明行」代替）。
           读数三态（review R-28）：读取中 / 读取失败 / 旧快照一律不冒称「0 条」。 -->
      <span v-if="selectedDate" class="kev-head-count" data-testid="key-event-selected-count">
        {{ selectedDayCountText }}
      </span>
      <button
        v-if="selectedDate"
        type="button"
        class="kev-goto"
        data-testid="key-event-goto-selected"
        title="所选日居中"
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

      <!-- 选中日无事件说明（tasks 4.3；条件 = 当日全泳道确认事件总数 = 0）。
           MUST NOT 代替视图头的「MM/DD · 0 条」读数（两者同时在）。
           放在舞台之外：横向滚动只动列，提示行不跟着横移（R-33）。 -->
      <p
        v-if="showSelectedDayEmpty"
        class="kev-day-empty"
        data-testid="key-event-selected-day-empty"
      >
        {{ selectedDayEmptyText }}
      </p>

      <!-- 行头列 + 横向滚动区（R-33）：行头 MUST NOT 留在滚动容器内 ——
           行头 sticky 在滚动容器里会把左缘 192px 的内容（轴头日期 / 标签 / 引线）
           用不透明白底盖住（实机上 1440×900 横滚后可见标签左半截被盖）。
           现在行头单独成列（不横滚），滚动区从行头右缘起算，两者逐行等高：
           行头单元与行体同用 rowHeight()、轴头行与休眠组标题用等高占位块。 -->
      <div class="kev-stage" data-testid="key-event-stage">
        <div class="kev-head-col" data-testid="key-event-head-col">
          <div class="kev-head-axis-spacer" aria-hidden="true" />
          <div
            v-for="projection in activeProjections"
            :key="projection.row.topicId"
            class="kev-head-cell"
            :style="{ height: `${rowHeight(projection.row.topicId)}px` }"
          >
            <div
              class="kev-row-head"
              :data-testid="`key-event-rowhead-${projection.row.topicId}`"
            >
              <!-- 行头内容按生命线对齐（review R-25）：内容块中心 = 生命线，transform 拉回自身半高 -->
              <span
                class="kev-row-head-inner"
                :style="{ top: `${rowLifelineY(projection.row.topicId)}px` }"
              >
                <span class="kev-row-name">{{ projection.row.label }}</span>
                <span class="kev-row-meta">
                  {{ projection.eventCount > 0 ? `${projection.eventCount} 事件${projection.lastEventDate ? ` · 最近 ${shortDate(projection.lastEventDate)}` : ''}` : '无确认事件' }}
                </span>
                <!-- 当日在该泳道的条数（tasks 4.2）；读数未知时不渲染（review R-28） -->
                <span
                  v-if="rowDayCountTexts.has(projection.row.topicId)"
                  class="kev-row-day"
                  :data-testid="`key-event-rowday-${projection.row.topicId}`"
                >{{ rowDayCountTexts.get(projection.row.topicId) }}</span>
              </span>
            </div>
          </div>
          <!-- 休眠组标题占位：与滚动区里的 `.kev-group-summary` 等高（同一套 inline-flex 盒）。 -->
          <div class="kev-head-summary-spacer" aria-hidden="true">
            <span class="kev-group-btn">
              <span class="kev-group-title">休眠泳道<span class="kev-group-count"> · {{ dormantRows.length }}</span><span v-if="dormantDayCount > 0" class="kev-group-count"> · 当日 {{ dormantDayCount }} 条</span></span>
              <span class="kev-group-note">{{ dormantExpanded ? '收起' : '展开查看' }}</span>
            </span>
          </div>
          <div
            v-for="projection in renderedDormant"
            :key="projection.row.topicId"
            class="kev-head-cell is-dormant"
            :style="{ height: `${rowHeight(projection.row.topicId)}px` }"
          >
            <div
              class="kev-row-head"
              :data-testid="`key-event-rowhead-${projection.row.topicId}`"
            >
              <span
                class="kev-row-head-inner"
                :style="{ top: `${rowLifelineY(projection.row.topicId)}px` }"
              >
                <span class="kev-row-name">{{ projection.row.label }}</span>
                <span class="kev-row-meta">
                  {{ projection.eventCount > 0 ? `${projection.eventCount} 事件${projection.lastEventDate ? ` · 最近 ${shortDate(projection.lastEventDate)}` : ''}` : '无确认事件' }}
                </span>
                <span
                  v-if="rowDayCountTexts.has(projection.row.topicId)"
                  class="kev-row-day"
                  :data-testid="`key-event-rowday-${projection.row.topicId}`"
                >{{ rowDayCountTexts.get(projection.row.topicId) }}</span>
              </span>
            </div>
          </div>
          <!-- 「其他」兜底行头（spec「关键事件『其他』兜底行」）：不绑定点击，MUST NOT 写 selectedLane -->
          <div
            v-for="projection in otherProjections"
            :key="projection.row.topicId"
            class="kev-head-cell is-other"
            :style="{ height: `${rowHeight(projection.row.topicId)}px` }"
          >
            <div
              class="kev-row-head"
              :data-testid="`key-event-rowhead-${projection.row.topicId}`"
            >
              <span
                class="kev-row-head-inner"
                :style="{ top: `${rowLifelineY(projection.row.topicId)}px` }"
              >
                <span class="kev-row-name">其他</span>
                <span class="kev-row-meta">
                  {{ `${projection.eventCount} 事件${projection.lastEventDate ? ` · 最近 ${shortDate(projection.lastEventDate)}` : ''}` }}
                </span>
                <span
                  v-if="rowDayCountTexts.has(projection.row.topicId)"
                  class="kev-row-day"
                  :data-testid="`key-event-rowday-${projection.row.topicId}`"
                >{{ rowDayCountTexts.get(projection.row.topicId) }}</span>
              </span>
            </div>
          </div>
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
          <!-- 日期轴头（与矩阵同一 visibleAxis 逐列对齐；列头即可选日入口，R-34） -->
          <div class="kev-day-heads" :style="{ '--kev-col': `${columnWidth}px` }">
            <button
              v-for="(date, index) in axis"
              :key="date"
              type="button"
              class="kev-day"
              :class="{ 'is-today': date === today, 'is-selected': date === selectedDate, 'is-hovered': date === hoveredDate }"
              :data-testid="`key-event-day-${date}`"
              :title="date"
              :aria-pressed="date === selectedDate"
              @click="emit('selectDate', date)"
            >
              <span class="kev-day-text">{{ shortDate(date) }}</span>
            </button>
          </div>

          <!-- 活跃行（行头在 `.kev-head-col`，与这里的行体逐行等高） -->
          <div
            v-for="projection in activeProjections"
            :key="projection.row.topicId"
            class="kev-row"
            :data-testid="`key-event-row-${projection.row.topicId}`"
          >
            <div class="kev-row-body" :style="{ height: `${rowHeight(projection.row.topicId)}px`, width: `${bodyWidth}px` }">
              <!-- 生命线（轴带居中；随上带层数下移） -->
              <div class="kev-axis-line" :style="{ top: `${rowLifelineY(projection.row.topicId)}px` }" aria-hidden="true" />
              <!-- 引线覆盖层：每条标签恰有一根 path（画在标签之下，遮线不遮字） -->
              <svg
                v-if="rowLeaders(projection.row.topicId).length > 0"
                class="kev-leads"
                :class="{ 'is-measuring': measuring }"
                :width="bodyWidth"
                :height="rowHeight(projection.row.topicId)"
                aria-hidden="true"
              >
                <path
                  v-for="lead in rowLeaders(projection.row.topicId)"
                  :key="lead.key"
                  class="kev-lead"
                  :class="{ 'is-hovered': hoveredEventId === lead.key }"
                  :d="lead.d"
                  :data-ev="lead.key"
                />
              </svg>
              <template v-for="(date, index) in axis" :key="date">
                <div
                  class="kev-cell"
                  :class="{ 'is-col-selected': date === selectedDate }"
                  :style="{ left: `${index * columnWidth}px`, width: `${columnWidth}px` }"
                >
                  <button
                    v-for="node in rowNodes(projection, date)"
                    :key="node.event.id"
                    type="button"
                    class="kev-node"
                    :class="{ 'is-selected': date === selectedDate, 'is-hovered': hoveredEventId === node.event.id, 'has-count': node.showCount }"
                    :data-testid="`key-event-node-${projection.row.topicId}-${date}`"
                    :data-date="date"
                    :style="nodeStyle(projection.row.topicId, node.index, node.count)"
                    :title="`${date} · ${node.event.titleSnapshot}`"
                    @click.stop="emit('selectDate', date)"
                    @mouseenter="hoveredEventId = node.event.id"
                    @mouseleave="hoveredEventId = null"
                  >
                    <template v-if="node.showCount">{{ dayDot(date) }} ★{{ node.total }}</template>
                  </button>
                </div>
              </template>
              <template v-if="!rowCollapsed(projection.row.topicId)">
                <KeyEventLabel
                  v-for="view in rowLabelViews(projection.row.topicId, projection)"
                  :key="view.event.id"
                  :ref="el => setLabelRef(view.event.id, el)"
                  :event="view.event"
                  :left="view.left"
                  :top="view.top"
                  :measuring="measuring || view.placement === null"
                  @select="date => emit('selectDate', date)"
                  @mouseenter="hoveredEventId = view.event.id"
                  @mouseleave="hoveredEventId = null"
                />
              </template>
              <!-- 日聚焦卡片列（选中日在**所有**有该日事件的泳道行内展开） -->
              <div
                v-if="cardRect && rowCardEvents(projection.row.topicId).length > 0"
                class="kev-cards"
                :data-testid="`key-event-cards-${projection.row.topicId}`"
                :style="cardColumnStyle(projection.row.topicId)"
                :ref="el => setCardColumnRef(projection.row.topicId, el)"
                @click.capture="scheduleCardMeasure"
                @transitionend.passive="scheduleCardMeasure"
              >
                <div
                  class="kev-cards-scroll scroll-thin"
                  :class="{ 'is-scrollable': rowCardEvents(projection.row.topicId).length > CARD_LIMIT }"
                  :style="cardScrollStyle(projection.row.topicId)"
                  :data-testid="`key-event-cards-scroll-${projection.row.topicId}`"
                >
                  <KeyEventDayCard
                    v-for="ev in rowCardEvents(projection.row.topicId)"
                    :key="ev.id"
                    :ref="el => setCardRef(projection.row.topicId, ev.id, el)"
                    :event="ev"
                    :reading="reading"
                    @open-article="id => emit('openArticle', id)"
                  />
                  <p
                    v-if="rowCardEvents(projection.row.topicId).length > CARD_LIMIT"
                    class="kev-cards-more"
                    :data-testid="`key-event-cards-more-${projection.row.topicId}`"
                  >
                    还有 {{ rowCardEvents(projection.row.topicId).length - CARD_LIMIT }} 条
                  </p>
                </div>
              </div>
            </div>
          </div>

          <!-- 休眠组（沿用矩阵休眠判定与折叠交互；0 行保留同级 summary） -->
          <div class="kev-group-summary" data-testid="key-event-dormant-toggle">
            <button
              type="button"
              class="kev-group-btn"
              :aria-expanded="dormantExpanded"
              :disabled="dormantRows.length === 0"
              @click="onToggleDormant()"
            >
              <Icon :icon="dormantExpanded ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="14" />
              <span class="kev-group-title">休眠泳道<span class="kev-group-count"> · {{ dormantRows.length }}</span><span v-if="dormantDayCount > 0" class="kev-group-count"> · 当日 {{ dormantDayCount }} 条</span></span>
              <span class="kev-group-note">
                <template v-if="dormantRows.length === 0">本窗无</template>
                <template v-else>{{ dormantExpanded ? '收起' : '展开查看' }}</template>
              </span>
            </button>
          </div>

          <div
            v-for="projection in renderedDormant"
            :key="projection.row.topicId"
            class="kev-row is-dormant"
            :data-testid="`key-event-row-${projection.row.topicId}`"
          >
            <div class="kev-row-body" :style="{ height: `${rowHeight(projection.row.topicId)}px`, width: `${bodyWidth}px` }">
              <!-- 生命线（轴带居中；随上带层数下移） -->
              <div class="kev-axis-line" :style="{ top: `${rowLifelineY(projection.row.topicId)}px` }" aria-hidden="true" />
              <!-- 引线覆盖层：每条标签恰有一根 path（画在标签之下，遮线不遮字） -->
              <svg
                v-if="rowLeaders(projection.row.topicId).length > 0"
                class="kev-leads"
                :class="{ 'is-measuring': measuring }"
                :width="bodyWidth"
                :height="rowHeight(projection.row.topicId)"
                aria-hidden="true"
              >
                <path
                  v-for="lead in rowLeaders(projection.row.topicId)"
                  :key="lead.key"
                  class="kev-lead"
                  :class="{ 'is-hovered': hoveredEventId === lead.key }"
                  :d="lead.d"
                  :data-ev="lead.key"
                />
              </svg>
              <template v-for="(date, index) in axis" :key="date">
                <div
                  class="kev-cell"
                  :class="{ 'is-col-selected': date === selectedDate }"
                  :style="{ left: `${index * columnWidth}px`, width: `${columnWidth}px` }"
                >
                  <button
                    v-for="node in rowNodes(projection, date)"
                    :key="node.event.id"
                    type="button"
                    class="kev-node"
                    :class="{ 'is-selected': date === selectedDate, 'is-hovered': hoveredEventId === node.event.id, 'has-count': node.showCount }"
                    :data-testid="`key-event-node-${projection.row.topicId}-${date}`"
                    :data-date="date"
                    :style="nodeStyle(projection.row.topicId, node.index, node.count)"
                    :title="`${date} · ${node.event.titleSnapshot}`"
                    @click.stop="emit('selectDate', date)"
                    @mouseenter="hoveredEventId = node.event.id"
                    @mouseleave="hoveredEventId = null"
                  >
                    <template v-if="node.showCount">{{ dayDot(date) }} ★{{ node.total }}</template>
                  </button>
                </div>
              </template>
              <template v-if="!rowCollapsed(projection.row.topicId)">
                <KeyEventLabel
                  v-for="view in rowLabelViews(projection.row.topicId, projection)"
                  :key="view.event.id"
                  :ref="el => setLabelRef(view.event.id, el)"
                  :event="view.event"
                  :left="view.left"
                  :top="view.top"
                  :measuring="measuring || view.placement === null"
                  @select="date => emit('selectDate', date)"
                  @mouseenter="hoveredEventId = view.event.id"
                  @mouseleave="hoveredEventId = null"
                />
              </template>
              <!-- 日聚焦卡片列（选中日在**所有**有该日事件的泳道行内展开） -->
              <div
                v-if="cardRect && rowCardEvents(projection.row.topicId).length > 0"
                class="kev-cards"
                :data-testid="`key-event-cards-${projection.row.topicId}`"
                :style="cardColumnStyle(projection.row.topicId)"
                :ref="el => setCardColumnRef(projection.row.topicId, el)"
                @click.capture="scheduleCardMeasure"
                @transitionend.passive="scheduleCardMeasure"
              >
                <div
                  class="kev-cards-scroll scroll-thin"
                  :class="{ 'is-scrollable': rowCardEvents(projection.row.topicId).length > CARD_LIMIT }"
                  :style="cardScrollStyle(projection.row.topicId)"
                  :data-testid="`key-event-cards-scroll-${projection.row.topicId}`"
                >
                  <KeyEventDayCard
                    v-for="ev in rowCardEvents(projection.row.topicId)"
                    :key="ev.id"
                    :ref="el => setCardRef(projection.row.topicId, ev.id, el)"
                    :event="ev"
                    :reading="reading"
                    @open-article="id => emit('openArticle', id)"
                  />
                  <p
                    v-if="rowCardEvents(projection.row.topicId).length > CARD_LIMIT"
                    class="kev-cards-more"
                    :data-testid="`key-event-cards-more-${projection.row.topicId}`"
                  >
                    还有 {{ rowCardEvents(projection.row.topicId).length - CARD_LIMIT }} 条
                  </p>
                </div>
              </div>
            </div>
          </div>
          <!-- 「其他」兜底行（spec「关键事件『其他』兜底行」）：排在休眠组之后、独立常显、不参与折叠 -->
          <div
            v-for="projection in otherProjections"
            :key="projection.row.topicId"
            class="kev-row is-other"
            :data-testid="`key-event-row-${projection.row.topicId}`"
          >
            <div class="kev-row-body" :style="{ height: `${rowHeight(projection.row.topicId)}px`, width: `${bodyWidth}px` }">
              <!-- 生命线（轴带居中；随上带层数下移） -->
              <div class="kev-axis-line" :style="{ top: `${rowLifelineY(projection.row.topicId)}px` }" aria-hidden="true" />
              <!-- 引线覆盖层：每条标签恰有一根 path（画在标签之下，遮线不遮字） -->
              <svg
                v-if="rowLeaders(projection.row.topicId).length > 0"
                class="kev-leads"
                :class="{ 'is-measuring': measuring }"
                :width="bodyWidth"
                :height="rowHeight(projection.row.topicId)"
                aria-hidden="true"
              >
                <path
                  v-for="lead in rowLeaders(projection.row.topicId)"
                  :key="lead.key"
                  class="kev-lead"
                  :class="{ 'is-hovered': hoveredEventId === lead.key }"
                  :d="lead.d"
                  :data-ev="lead.key"
                />
              </svg>
              <template v-for="(date, index) in axis" :key="date">
                <div
                  class="kev-cell"
                  :class="{ 'is-col-selected': date === selectedDate }"
                  :style="{ left: `${index * columnWidth}px`, width: `${columnWidth}px` }"
                >
                  <button
                    v-for="node in rowNodes(projection, date)"
                    :key="node.event.id"
                    type="button"
                    class="kev-node"
                    :class="{ 'is-selected': date === selectedDate, 'is-hovered': hoveredEventId === node.event.id, 'has-count': node.showCount }"
                    :data-testid="`key-event-node-${projection.row.topicId}-${date}`"
                    :data-date="date"
                    :style="nodeStyle(projection.row.topicId, node.index, node.count)"
                    :title="`${date} · ${node.event.titleSnapshot}`"
                    @click.stop="emit('selectDate', date)"
                    @mouseenter="hoveredEventId = node.event.id"
                    @mouseleave="hoveredEventId = null"
                  >
                    <template v-if="node.showCount">{{ dayDot(date) }} ★{{ node.total }}</template>
                  </button>
                </div>
              </template>
              <template v-if="!rowCollapsed(projection.row.topicId)">
                <KeyEventLabel
                  v-for="view in rowLabelViews(projection.row.topicId, projection)"
                  :key="view.event.id"
                  :ref="el => setLabelRef(view.event.id, el)"
                  :event="view.event"
                  :left="view.left"
                  :top="view.top"
                  :measuring="measuring || view.placement === null"
                  @select="date => emit('selectDate', date)"
                  @mouseenter="hoveredEventId = view.event.id"
                  @mouseleave="hoveredEventId = null"
                />
              </template>
              <!-- 日聚焦卡片列（选中日在**所有**有该日事件的泳道行内展开） -->
              <div
                v-if="cardRect && rowCardEvents(projection.row.topicId).length > 0"
                class="kev-cards"
                :data-testid="`key-event-cards-${projection.row.topicId}`"
                :style="cardColumnStyle(projection.row.topicId)"
                :ref="el => setCardColumnRef(projection.row.topicId, el)"
                @click.capture="scheduleCardMeasure"
                @transitionend.passive="scheduleCardMeasure"
              >
                <div
                  class="kev-cards-scroll scroll-thin"
                  :class="{ 'is-scrollable': rowCardEvents(projection.row.topicId).length > CARD_LIMIT }"
                  :style="cardScrollStyle(projection.row.topicId)"
                  :data-testid="`key-event-cards-scroll-${projection.row.topicId}`"
                >
                  <KeyEventDayCard
                    v-for="ev in rowCardEvents(projection.row.topicId)"
                    :key="ev.id"
                    :ref="el => setCardRef(projection.row.topicId, ev.id, el)"
                    :event="ev"
                    :reading="reading"
                    @open-article="id => emit('openArticle', id)"
                  />
                  <p
                    v-if="rowCardEvents(projection.row.topicId).length > CARD_LIMIT"
                    class="kev-cards-more"
                    :data-testid="`key-event-cards-more-${projection.row.topicId}`"
                  >
                    还有 {{ rowCardEvents(projection.row.topicId).length - CARD_LIMIT }} 条
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
        </div>
      </div>
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
  flex-wrap: wrap;
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

/* 所属日期下拉（tasks 4.1）：沿用 BoardArchiveView `.ar-select` 口径
   （同一套 token：--color-border-medium / --color-input-bg / --color-text-primary）。 */
.kev-select {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: var(--color-text-muted);
  font-size: 0.74rem;
}

.kev-select select {
  flex: 0 0 auto;
  max-width: 12rem;
  padding: 0.25rem 0.35rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  font: inherit;
  font-size: 0.76rem;
  cursor: pointer;
}

/* 键盘可见焦点（MUST NOT 只靠 hover 反馈）。 */
.kev-select select:focus-visible {
  outline: 2px solid var(--color-accent);
  outline-offset: 1px;
}

/* 选中日读数：「MM/DD · n 条」（视图头；tasks 4.1）。 */
.kev-head-count {
  color: var(--color-text-secondary);
  font-size: 0.72rem;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
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

/* 选中日无事件说明行（tasks 4.3）：贴在轴上方，左侧留出行头宽度与轴对齐。 */
.kev-day-empty {
  margin: 0;
  padding: 0.15rem 0 0.15rem 192px;
  color: var(--color-text-muted);
  font-size: 0.72rem;
  line-height: 1.5;
}

/* 舞台内横滚：列宽被 64px 下限撑大时只滚本容器（布局契约例外）。
   行头不在本容器内（R-33）：滚动区从行头右缘起算，故左缘不会有内容被行头盖住。 */
.kev-scroll {
  flex: 1 1 auto;
  min-width: 0;
  overflow-x: auto;
  overflow-y: hidden;
  padding-bottom: 0.2rem;
  touch-action: pan-x pan-y;
}

.kev-scroll.is-panning {
  cursor: grabbing;
  user-select: none;
}

/* 舞台：行头列（不横滚）+ 横向滚动区（R-33）。 */
.kev-stage {
  /* 轴头行高（行头列占位与滚动区里的 `.kev-day-heads` 同用 → 两边逐行对齐）。 */
  --kev-axis-head: 1.4rem;
  display: flex;
  align-items: flex-start;
  gap: 0;
  min-width: 0;
}

/* 行头列：宽度 = 列宽公式里的行头占位（HEADER_WIDTH = 192px）；不参与横向滚动。
   MUST NOT 把它放回滚动容器内做 sticky —— 那会让左缘 192px 的轴头 / 标签 / 引线
   被不透明白底盖住（azure R-33）。 */
.kev-head-col {
  flex: 0 0 auto;
  width: 192px;
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

/* 轴头行占位：与 `.kev-day-heads` 等高（两边同用一个固定行高）。 */
.kev-head-axis-spacer {
  flex: 0 0 auto;
  height: var(--kev-axis-head);
  border-bottom: 1px solid transparent;
}

/* 行头单元：高度 = 行高（与滚动区里同序的行体逐行等高）。 */
.kev-head-cell {
  flex: 0 0 auto;
  position: relative;
}

/* 休眠组标题占位：与 `.kev-group-summary` 等高（同一套 inline-flex 盒，仅隐藏）。 */
.kev-head-summary-spacer {
  display: flex;
  visibility: hidden;
  pointer-events: none;
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
  height: var(--kev-axis-head);
}

/* 轴头日期（R-34）：可选日入口 —— 原生 button（Enter / Space 直开即生效）。 */
.kev-day {
  flex: 0 0 auto;
  width: var(--kev-col, 64px);
  padding: 0 0 0.2rem;
  border: none;
  border-bottom: 1px solid var(--color-border-subtle);
  background: none;
  color: var(--color-text-muted);
  font: inherit;
  font-size: 0.66rem;
  font-variant-numeric: tabular-nums;
  text-align: center;
  white-space: nowrap;
  cursor: pointer;
}

.kev-day:focus-visible {
  /* 用 outline 而不是 box-shadow：选中日的 box-shadow 是下边框强调条，两者会打架。 */
  outline: 2px solid var(--color-accent);
  outline-offset: -2px;
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

/* 悬停强调（design D11）：该事件的轴头日期加重（不改选中色）。 */
.kev-day.is-hovered {
  color: var(--color-text-primary);
  font-weight: 600;
}

.kev-row {
  min-width: 0;
}

/* 休眠行：弱化只落在**行头文字**（次要信息）。
   MUST NOT 用整行 opacity —— 那会把事件色 #8f5a17 合成到 2.23:1（低于正文级 4.5:1，
   也低于非文字级 3:1，review R-24）；卡片 / 节点 / 引线一律保持满对比度。 */
.kev-head-cell.is-dormant .kev-row-name {
  color: var(--color-text-muted);
}

/* 「其他」兜底行：与休眠行同一套弱化口径（只弱化行头文字，不碰节点 / 标签 / 卡片）。 */
.kev-head-cell.is-other .kev-row-name {
  color: var(--color-text-muted);
}

.kev-row-head {
  position: relative;
  display: block;
  width: 100%;
  height: 100%;
  padding: 0.3rem 16px 0.3rem 0;
  /* 行头是只读列（2026-11-24 用户反馈：本视图无联动，选中易误解）——
     不可点，故用默认光标、不做悬停反馈。border-left 留 2px 透明是为了
     与改前（button 版）的行头文字左缘逐像素对齐。 */
  border: none;
  border-left: 2px solid transparent;
  background: var(--color-bg-base);
  color: var(--color-text-secondary);
  text-align: left;
  cursor: default;
}

/* 行头内容按生命线对齐（review R-25）：内容块中心 = 生命线，transform 拉回自身半高。
   不再整行垂直居中 —— 居中会让行头文字与生命线错位（行高 290px、上带 0 层时
   行头中心 ~145px、生命线只在 25px）。 */
.kev-row-head-inner {
  position: absolute;
  left: 0;
  right: 16px;
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  transform: translateY(-50%);
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

/* 行头「当日 k 条」（tasks 4.2）：次要读数，与事件计数同一层级。 */
.kev-row-day {
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

/* 生命线：轴带居中（top 由行内生命线位置注入，随上带层数下移）。
   颜色用 --color-text-muted：引线树的主干从这条线长出来，--color-border-medium
   对比度仅 1.67:1 承载不了（tasks 2.5）。 */
.kev-axis-line {
  position: absolute;
  left: 0;
  right: 0;
  border-top: 1px dashed var(--color-text-muted);
}

/* 引线覆盖层：每行一个 SVG，逐标签一条 path（design D4）。
   z-index 0 → 画在标签（z-index 1）之下：标签遮线不遮字；行头（z-index 2）仍在上。 */
.kev-leads {
  position: absolute;
  top: 0;
  left: 0;
  z-index: 0;
  overflow: visible;
  pointer-events: none;
}

/* 测量阶段（标签隐藏、坐标未定）不画线，避免一闪。 */
.kev-leads.is-measuring {
  visibility: hidden;
}

.kev-lead {
  fill: none;
  stroke: var(--color-event);
  stroke-width: 1.5;
  stroke-linecap: round;
  stroke-linejoin: round;
}

/* 悬停强调（design D11）：该事件的引线加重。 */
.kev-lead.is-hovered {
  stroke-width: 2.5;
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
  left: 50%;
  transform: translateX(-50%);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  padding: 0 2px;
  /* 节点描边与文字走事件语义色（design D12：琥珀 = 事件，红 = 选中）。 */
  border: 1px solid var(--color-event);
  border-radius: 50%;
  background: var(--color-bg-elevated);
  color: var(--color-event);
  font-size: 0.6rem;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  cursor: pointer;
}

/* 收起行节点（「11.05 ★N」，2026-11-24 用户反馈）：盒要装日期前缀，
   宽列下也是胶囊而不是椭圆；MUST NOT 换行、MUST NOT 省略号截断 ——
   64px 宽列下靠更小字号 + 更紧内边距守「盒宽 ≤ 列宽」（单节点恒居中，
   故 64px 列里两侧各还剩 ~7px，不进邻列）。 */
.kev-node.has-count {
  padding: 0 3px;
  border-radius: 9px;
  font-size: 0.55rem;
  white-space: nowrap;
}

/* 选中日的节点呈实心（与未选中日区分；三重冗余之一）。
   实心填充走**强调色**（红 = 我选中的那天）：事件色（琥珀）只保留在描边 / 引线 /
   卡片首行日期 —— 同一颜色 MUST NOT 表达两义（review R-20）。 */
.kev-node.is-selected {
  background: var(--color-accent);
  color: var(--color-bg-elevated);
}

.kev-node:hover,
.kev-node:focus-visible {
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-event) 35%, transparent);
  outline: none;
}

/* 悬停强调（design D11）：该事件的节点加重。 */
.kev-node.is-hovered {
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-event) 45%, transparent);
}

.kev-group-summary {
  display: flex;
}

.kev-group-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  /* 单行：与行头列的等高占位块（`.kev-head-summary-spacer`）逐行等高，不得换行。 */
  white-space: nowrap;
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

/* 日聚焦卡片列：行内绝对定位（放下带：顶 = 轴带下缘 + 8；左缘 = 卡片矩形左缘）。
   z-index 1 → 画在引线覆盖层（z-index 0）之上；行头（z-index 2）仍在其上。 */
.kev-cards {
  position: absolute;
  z-index: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}

/* 卡片列滚动容器：> 4 张时固定高度内部滚动（滚动条走全局 .scroll-thin）。 */
.kev-cards-scroll {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
}

.kev-cards-scroll.is-scrollable {
  overflow-y: auto;
  overflow-x: hidden;
}

/* 未展示条数：贴在滚动容器底部（sticky），滚动时也看得见。 */
.kev-cards-more {
  position: sticky;
  bottom: 0;
  margin: 0;
  padding: 0.15rem 0.1rem 0.1rem;
  background: var(--color-bg-elevated);
  color: var(--color-text-muted);
  font-size: 0.66rem;
  line-height: 1.4;
  text-align: right;
}
</style>
