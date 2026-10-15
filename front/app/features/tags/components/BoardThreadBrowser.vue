<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { Icon } from '@iconify/vue'
import { useDailyReportsApi, type SectionTimelineNode, type SectionRelation, type DailyReportThread, type BoardTopicListItem, type TopicKeyEventMarker } from '~/api/dailyReports'
import { useTopicKeyEvents } from '../composables/useTopicKeyEvents'
import { useArticlesApi } from '~/api/articles'
import { fullComponentHighlight } from '~/utils/graphHighlight'
import { filterFocusNodes, isDragMove, buildFocusMeta } from './topicFocus'
import ComposeInlineToolbar from './ComposeInlineToolbar.vue'
import ComposeSidebar from './ComposeSidebar.vue'
import { TIER_LABEL } from './composeReport'
import { useInlineCompose, type MoveOutItem, type NodeTierInfo, type SidebarCandidateItem } from '../composables/useInlineCompose'

const props = defineProps<{
  boardId: number
  /** 外部聚焦入口（话题态势卡片/气泡点击）：非空时进入 focus 专注视图聚焦该话题。 */
  focusTopicId?: number | null
}>()

const {
  getBoardSectionTimeline,
  getDailyReportDetail,
  updateTopic,
  deleteTopic,
  mergeTopics,
  backfillPersistentTopics,
  listBoardTopics,
} = useDailyReportsApi()
const { getArticle } = useArticlesApi()
const { theme } = useTheme()

const days = ref(14)
const loading = ref(false)
const sections = ref<SectionTimelineNode[]>([])
const relations = ref<SectionRelation[]>([])
const selectedNode = ref<SectionTimelineNode | null>(null)
const hoveredId = ref<number | null>(null)
// 视图模式：timeline=匈牙利相似度 DAG（默认）；lanes=按话题分泳道（identity 驱动）；
// focus=从某条泳道进入单话题专注视图（复用 lanes 数据，仅重投影单话题节点）；
// 编排态为 lanes 上的 composeMode 布尔叠加态（不再单独占一个 viewMode，见下方编排态块）。
const viewMode = ref<'timeline' | 'lanes' | 'focus'>('timeline')
// focus 视图锁定的话题 id（由点泳道进入）；lanes/timeline 下保持 null。
const focusedTopicId = ref<number | null>(null)

// --- Popup thread/article state ---
const popupThreads = ref<DailyReportThread[]>([])
const popupThreadsLoading = ref(false)
const expandedThreadId = ref<number | null>(null)
const threadArticles = ref<Map<number, { id: number, title: string }[]>>(new Map())
const threadArticlesLoading = ref(false)

// --- Constants（不随缩放变化：标签列宽）---
// 泳道视图参数
const LANE_LABEL_W = 180 // 左侧话题标签列宽（容结标签 + hover 操作菜单，避免重叠）

// --- Status styling ---
// Colors reference Layer 2 theme tokens (defined in main.css as
// --color-thread-status-*), so they adapt to editorial/dark themes.
const statusColorMap: Record<string, string> = {
  emerging: 'var(--color-thread-status-emerging)',
  continuing: 'var(--color-thread-status-continuing)',
  split: 'var(--color-thread-status-split)',
  merge: 'var(--color-thread-status-merge)',
  ending: 'var(--color-thread-status-ending)',
}

const statusLabels: Record<string, string> = {
  emerging: '新兴',
  continuing: '持续',
  split: '分化',
  merge: '合并',
  ending: '结束',
}

function statusFill(status: string): string {
  return statusColorMap[status] || 'var(--color-thread-status-ending)'
}

// --- Detective wall entry: only on large screens with WebGL support ---
const viewportWidth = ref(typeof window !== 'undefined' ? window.innerWidth : 1280)
const showDetectiveEntry = computed(() => {
  if (viewportWidth.value < 768) return false
  try {
    const c = document.createElement('canvas')
    return !!(c.getContext('webgl2') || c.getContext('webgl'))
  } catch {
    return false
  }
})
// Named handler so it can be removed on unmount (avoid leaked listeners).
function onViewportResize() { viewportWidth.value = window.innerWidth }
onMounted(() => window.addEventListener('resize', onViewportResize))
onUnmounted(() => window.removeEventListener('resize', onViewportResize))

// --- SVG 缩放（滚轮：鼠标在图上直接滚轮 = 缩放，不用 Ctrl，避免和浏览器缩放冲突）---
// 缩放语义：只拉开布局间距（行高 / 列宽 / 节点间距随滚轮线性变化），
// 字体与节点半径保持不变。这样放大→节点之间越来越宽松，文字不会因等比放大
// 而盖住相邻泳道（原 transform:scale 等比缩放「放大了还是挤」的问题根除）。
const svgScrollRef = ref<HTMLDivElement | null>(null)
const zoomScale = ref(1)
const MIN_SCALE = 0.6
const MAX_SCALE = 2.4
const ZOOM_STEP = 0.15
const zoomPercent = computed(() => Math.round(zoomScale.value * 100))

// 间距类布局参数随缩放线性变化（基础值已加大：行高 52→76、节点间距 24→34，
// 保证 100% 时也不挤）。节点半径 / 字号 / 标签列宽不随缩放（见常量区）。
const COL_W = computed(() => 148 * zoomScale.value)
const ROW_H = computed(() => 60 * zoomScale.value)
const PAD = computed(() => 32 * zoomScale.value)
const LANE_BASE = computed(() => 76 * zoomScale.value)
const LANE_NODE_GAP = computed(() => 34 * zoomScale.value)
// 节点 label 截断阈值随列宽放大→可容纳更多字，缩小设下限防过短。
const LABEL_MAX = computed(() => Math.max(4, Math.round(10 * zoomScale.value)))
// 字体 / 节点半径缩放系数：增长慢于间距 zoomScale（系数 0.5）——放大时字与节点变大、
// 但间距增长更快→不会盖邻道。fontScale=1+(zoom-1)*0.5：zoom2→1.5, zoom2.4→1.7, zoom0.6→0.8。
const fontScale = computed(() => 1 + (zoomScale.value - 1) * 0.5)
const NODE_R = computed(() => 7 * fontScale.value)

// 缩放时保持视口中心在内容里的相对比例（内容尺寸随 zoomScale 变化）。
async function setZoom(nextScale: number) {
  const el = svgScrollRef.value
  const maxX = el && el.scrollWidth > el.clientWidth ? el.scrollWidth - el.clientWidth : 0
  const maxY = el && el.scrollHeight > el.clientHeight ? el.scrollHeight - el.clientHeight : 0
  const ratioX = maxX > 0 ? el!.scrollLeft / maxX : 0
  const ratioY = maxY > 0 ? el!.scrollTop / maxY : 0
  zoomScale.value = nextScale
  await nextTick()
  if (el) {
    const nx = el.scrollWidth > el.clientWidth ? el.scrollWidth - el.clientWidth : 0
    const ny = el.scrollHeight > el.clientHeight ? el.scrollHeight - el.clientHeight : 0
    el.scrollLeft = ratioX * nx
    el.scrollTop = ratioY * ny
  }
}

function zoomIn() {
  void setZoom(Math.min(MAX_SCALE, +(zoomScale.value + ZOOM_STEP).toFixed(2)))
}
function zoomOut() {
  void setZoom(Math.max(MIN_SCALE, +(zoomScale.value - ZOOM_STEP).toFixed(2)))
}

function resetZoom() {
  void setZoom(1)
}

// 鼠标在图上滚轮 = 缩放。需 passive:false 才能 preventDefault 阻止页面滚动，
// 故手动 addEventListener（template @wheel 在部分浏览器被当 passive）。
function onWheelZoom(e: WheelEvent) {
  e.preventDefault()
  const delta = e.deltaY < 0 ? ZOOM_STEP : -ZOOM_STEP
  const next = Math.min(MAX_SCALE, Math.max(MIN_SCALE, +(zoomScale.value + delta).toFixed(2)))
  if (next === zoomScale.value) return
  void setZoom(next)
}
// svgScrollRef 随 viewMode 切换重新挂载，watch 动态绑/解 wheel 监听。
watch(svgScrollRef, (el, oldEl) => {
  if (oldEl) oldEl.removeEventListener('wheel', onWheelZoom)
  if (el) el.addEventListener('wheel', onWheelZoom, { passive: false })
}, { immediate: true })

// 编排态右侧候选侧边栏折叠态（收起成窄条，让出空间给泳道）。
const composeSidebarCollapsed = ref(false)

// Theme-aware SVG colors
const svgGridColor = computed(() => theme.value === 'dark' ? 'rgba(255,255,255,0.04)' : 'rgba(26,26,26,0.06)')
const svgEdgeColor = computed(() => theme.value === 'dark' ? 'rgba(255,255,255,' : 'rgba(26,26,26,')
const svgNodeStroke = computed(() => theme.value === 'dark' ? 'rgba(255,255,255,0.15)' : 'rgba(26,26,26,0.15)')
const svgHighlightColor = computed(() => theme.value === 'dark' ? 'rgba(255,255,255,0.65)' : 'rgba(26,26,26,0.65)')
// 泳道相关主题色（本次新增）
const svgLaneLabelColor = computed(() => theme.value === 'dark' ? 'rgba(226,232,240,0.88)' : 'rgba(26,26,26,0.85)')
const svgLaneStripeColor = computed(() => theme.value === 'dark' ? 'rgba(255,255,255,0.025)' : 'rgba(26,26,26,0.03)')

function getEdgeColor(distance: number, highlighted: boolean): string {
  if (highlighted) return svgHighlightColor.value
  return `${svgEdgeColor.value}${edgeOpacity(distance)})`
}

// --- Date helpers ---

function formatDateShort(dateStr: string): string {
  const d = new Date(dateStr)
  return `${d.getMonth() + 1}/${d.getDate()}`
}

function truncateLabel(label: string): string {
  const max = LABEL_MAX.value
  return label.length > max ? label.slice(0, max) + '…' : label
}


// --- Hover highlight graph ---

const visibleRelations = computed(() => relations.value.filter((r) => {
  const isIdentity = r.relation_type === 'identity'
  return viewMode.value === 'lanes' ? isIdentity : !isIdentity
}))

const highlightedNodeIds = computed(() => {
  if (hoveredId.value === null) return new Set<number>()
  return fullComponentHighlight(
    hoveredId.value,
    visibleRelations.value.map(r => ({ source: r.from_id, target: r.to_id })),
  )
})

function isEdgeHighlighted(r: { fromId: number; toId: number }): boolean {
  if (hoveredId.value === null) return false
  return highlightedNodeIds.value.has(r.fromId) && highlightedNodeIds.value.has(r.toId)
}

function isNodeHighlighted(nodeId: number): boolean {
  return hoveredId.value !== null && highlightedNodeIds.value.has(nodeId)
}

// --- Simple timeline layout ---

const sortedDates = computed<string[]>(() => {
  const dates = new Set(sections.value.map(s => s.period_date.slice(0, 10)))
  return [...dates].sort()
})

const dateIndex = computed(() => {
  const map = new Map<string, number>()
  sortedDates.value.forEach((d, i) => map.set(d, i))
  return map
})

const maxRows = computed(() => {
  const counts = new Map<string, number>()
  for (const s of sections.value) {
    const d = s.period_date.slice(0, 10)
    counts.set(d, (counts.get(d) ?? 0) + 1)
  }
  let max = 0
  for (const c of counts.values()) max = Math.max(max, c)
  return max
})

// --- 泳道视图：按话题归类（topics/unassignedCount 来自下方话题管理块） ---
interface LaneRow { key: string; label: string; color: string; status: string; topicId: number | null }
const laneRows = computed<LaneRow[]>(() => {
  const rows: LaneRow[] = topics.value.filter(t => t.status === 'active').map(t => ({
    key: `topic-${t.id}`, label: t.label, color: t.color, status: t.status, topicId: t.id,
  }))
  if (nonActiveSectionCount.value > 0) {
    rows.push({ key: 'unassigned', label: '待确认 / 未分类', color: '#64748b', status: 'none', topicId: null })
  }
  return rows
})
const laneIndexByKey = computed(() => {
  const m = new Map<string, number>()
  laneRows.value.forEach((r, i) => m.set(r.key, i))
  return m
})
const sectionById = computed(() => {
  const m = new Map<number, SectionTimelineNode>()
  for (const s of sections.value) m.set(s.id, s)
  return m
})
function sectionLaneKey(s: SectionTimelineNode): string {
  if (s.persistent_topic?.status === 'active') return `topic-${s.persistent_topic.id}`
  return 'unassigned'
}

interface PositionedSection {
  data: SectionTimelineNode
  cx: number
  cy: number
}

// 泳道自适应高度：每条泳道高 = 基础行高 + 该泳道内单天最大节点数 × 节点间距。
// 固定高度会导致同天多节点的泳道互相覆盖（节点+标签溢出），动态高度让每条
// 泳道都够容结自己的内容，间隔充裕。LANE_BASE/LANE_NODE_GAP 随 zoomScale 变化（见缩放区）。
const laneLayout = computed(() => {
  const lanes = laneRows.value
  const laneH = new Array(lanes.length).fill(LANE_BASE.value)
  const subMax = new Map<string, number>()
  for (const s of sections.value) {
    const date = s.period_date.slice(0, 10)
    const li = laneIndexByKey.value.get(sectionLaneKey(s)) ?? 0
    const k = `${li}:${date}`
    const n = (subMax.get(k) ?? 0) + 1
    subMax.set(k, n)
    // 单节点只需 LANE_BASE；每多一个节点 +节点间距（含节点半径与标签余量）。
    const need = LANE_BASE.value + (n - 1) * LANE_NODE_GAP.value
    if (need > laneH[li]!) laneH[li] = need
  }
  const laneY: number[] = []
  let acc = 0
  for (const h of laneH) {
    laneY.push(acc)
    acc += h
  }
  return { laneH, laneY, subMax }
})

const positionedNodes = computed<PositionedSection[]>(() => {
  if (viewMode.value === 'lanes') {
    const layout = laneLayout.value
    const seen = new Map<string, number>()
    return sections.value.map(s => {
      const date = s.period_date.slice(0, 10)
      const col = dateIndex.value.get(date) ?? 0
      const li = laneIndexByKey.value.get(sectionLaneKey(s)) ?? 0
      const k = `${li}:${date}`
      const total = layout.subMax.get(k) ?? 1
      const idx = seen.get(k) ?? 0
      seen.set(k, idx + 1)
      const subOffset = (idx - (total - 1) / 2) * LANE_NODE_GAP.value
      return {
        data: s,
        cx: col * COL_W.value + PAD.value + COL_W.value / 2 + LANE_LABEL_W,
        // 与背景 rect 同基准（rect y = laneY[li] + PAD），节点 cy 也 + PAD。
        cy: PAD.value + layout.laneY[li]! + layout.laneH[li]! / 2 + subOffset,
      }
    })
  }
  // timeline 模式：同天按出现顺序纵向堆叠。
  const rowCounter = new Map<string, number>()
  return sections.value.map(s => {
    const date = s.period_date.slice(0, 10)
    const col = dateIndex.value.get(date) ?? 0
    const row = rowCounter.get(date) ?? 0
    rowCounter.set(date, row + 1)
    return {
      data: s,
      cx: col * COL_W.value + PAD.value + COL_W.value / 2,
      cy: row * ROW_H.value + PAD.value + ROW_H.value / 2,
    }
  })
})

const posById = computed(() => {
  const map = new Map<number, { cx: number; cy: number }>()
  for (const pn of positionedNodes.value) {
    map.set(pn.data.id, { cx: pn.cx, cy: pn.cy })
  }
  return map
})

interface EdgeLine {
  key: string
  d: string
  fromId: number
  toId: number
  distance: number
  relationType: string  // 'identity'（同话题）| 'similarity'（匈牙利）
  color?: string        // identity 边的话题色
}

/** Map distance to visual weight: strong (<0.15) → 2, medium (<0.25) → 1.2, weak → 0.6 */
function edgeWeight(dist: number): number {
  if (dist < 0.15) return 2
  if (dist < 0.25) return 1.2
  return 0.6
}

/** Map distance to opacity: strong → 0.35, medium → 0.18, weak → 0.08 */
function edgeOpacity(dist: number): number {
  if (dist < 0.15) return 0.35
  if (dist < 0.25) return 0.18
  return 0.08
}

const edgePaths = computed<EdgeLine[]>(() => {
  return visibleRelations.value
    .map((r, i) => {
      const relType = r.relation_type ?? 'similarity'
      const from = posById.value.get(r.from_id)
      const to = posById.value.get(r.to_id)
      if (!from || !to) return { key: `edge-${i}`, d: '', fromId: r.from_id, toId: r.to_id, distance: r.distance, relationType: relType }
      const midX = (from.cx + to.cx) / 2
      let color: string | undefined
      if (relType === 'identity') {
        const fromNode = sectionById.value.get(r.from_id)
        color = fromNode?.persistent_topic?.color
      }
      return {
        key: `edge-${i}`,
        d: `M${from.cx},${from.cy} C${midX},${from.cy} ${midX},${to.cy} ${to.cx},${to.cy}`,
        fromId: r.from_id,
        toId: r.to_id,
        distance: r.distance,
        relationType: relType,
        color,
      }
    }).filter(e => e.d !== '')
})

const svgWidth = computed(() => {
  const base = sortedDates.value.length * COL_W.value + PAD.value * 2
  return viewMode.value === 'lanes' ? base + LANE_LABEL_W : base
})
const svgHeight = computed(() => {
  if (viewMode.value === 'lanes') {
    const total = laneLayout.value.laneH.reduce((a, b) => a + b, 0)
    return total + PAD.value * 2
  }
  return maxRows.value * ROW_H.value + PAD.value * 2
})

interface DateCol {
  date: string
  label: string
  x: number
}

const dateColumns = computed<DateCol[]>(() =>
  sortedDates.value.map((date, i) => ({
    date,
    label: formatDateShort(date),
    x: i * COL_W.value + PAD.value + COL_W.value / 2 + (viewMode.value === 'lanes' ? LANE_LABEL_W : 0),
  })),
)

// --- Node select → load threads ---

async function selectNode(node: SectionTimelineNode) {
  if (selectedNode.value?.id === node.id) {
    selectedNode.value = null
    popupThreads.value = []
    expandedThreadId.value = null
    return
  }
  selectedNode.value = node
  expandedThreadId.value = null
  popupThreads.value = []
  threadArticles.value = new Map()

  popupThreadsLoading.value = true
  try {
    const res = await getDailyReportDetail(node.report_id)
    if (res.success && res.data) {
      const section = res.data.report.sections?.find(s => s.id === node.id)
      popupThreads.value = section?.threads || []
    }
  } finally {
    popupThreadsLoading.value = false
  }
}

async function toggleThreadArticles(thread: DailyReportThread) {
  if (expandedThreadId.value === thread.id) {
    expandedThreadId.value = null
    return
  }
  expandedThreadId.value = thread.id

  if (threadArticles.value.has(thread.id)) return
  if (!thread.related_article_ids?.length) return

  threadArticlesLoading.value = true
  const ids = thread.related_article_ids.slice(0, 10)
  const results = await Promise.allSettled(ids.map(id => getArticle(id)))
  const articles = results.map((r, i) => {
    const aid = ids[i]!
    if (r.status === 'fulfilled' && r.value.success && r.value.data) {
      return { id: aid, title: r.value.data.title || '(无标题)' }
    }
    return { id: aid, title: `文章 #${aid}` }
  })
  threadArticles.value = new Map(threadArticles.value).set(thread.id, articles)
  threadArticlesLoading.value = false
}

const emit = defineEmits<{
  openArticle: [articleId: number]
  openDetectiveWall: []
  /** 来源日报导航（关键注记详情；仅有效来源可定位）。 */
  openDailyReport: [reportId: number]
}>()

// --- 关键事件注记（add-topic-key-event-markers 3.4/3.5）----------------------
// 总览（lanes/timeline DAG）/focus 只读 confirmed：未确认建议不进总览注记层，
// 对应新闻仍是普通节点（共享缓存的 suggested 不泄漏）。挑选统一在日报。
const keyEvents = useTopicKeyEvents()
// 突出关键节点开关：只弱化普通节点展示（opacity），不改坐标/节点集/关系。
const highlightKeyEvents = ref(false)
const selectedMarkerId = ref<number | null>(null)

watch([focusedTopicId, days], ([topicId, d]) => {
  if (topicId != null) void keyEvents.ensureTopicKeyEvents(topicId, d)
  // 切换泳道/时间窗口：重置局部强调与快照选择（迟到响应按 topicId 分键不串）。
  highlightKeyEvents.value = false
  selectedMarkerId.value = null
}, { immediate: true })

/** 全版块 confirmed 注记（lanes 徽标/弱化判定；跨 topic 汇总只读）。 */
const boardConfirmedMarkers = computed<TopicKeyEventMarker[]>(() => {
  // 按 marker.id 去重：同一 marker 可能经多个话题缓存可达（跨入口共享），
  // 汇总重复会计双 ★n 计数。
  const seen = new Set<number>()
  const out: TopicKeyEventMarker[] = []
  for (const topic of topics.value) {
    for (const marker of keyEvents.topicEntry(topic.id).markers) {
      if (seen.has(marker.id)) continue
      seen.add(marker.id)
      out.push(marker)
    }
  }
  return out
})

const boardMarkersBySection = computed(() => {
  const index = new Map<number, TopicKeyEventMarker[]>()
  for (const marker of boardConfirmedMarkers.value) {
    if (marker.source_section_id == null) continue
    const list = index.get(marker.source_section_id)
    if (list) list.push(marker)
    else index.set(marker.source_section_id, [marker])
  }
  return index
})

/** ★n：该来源 section 下的独立 confirmed 注记数（不是整章状态）。 */
/** 取消失败局部反馈（读写 error SHALL 有反馈；取消按钮即恢复重试入口）。 */
function markerErrorMessage(marker: TopicKeyEventMarker): string {
  return keyEvents.markerView(marker.id).error || ''
}

function sectionMarkerCount(sectionId: number): number {
  return boardMarkersBySection.value.get(sectionId)?.length ?? 0
}

/** 该来源 section 的 confirmed 注记（详情/弹窗维护列表）。 */
function sectionMarkers(sectionId: number): TopicKeyEventMarker[] {
  return boardMarkersBySection.value.get(sectionId) ?? []
}

function unmarkMarker(marker: TopicKeyEventMarker) {
  if (marker.persistent_topic_id != null) void keyEvents.actMarker(marker.persistent_topic_id, marker.id, 'unmark')
}

// --- focus 视图关键节点（合并列：真实 section + 失效来源快照，同轴按日期排）---
const focusTopicMarkers = computed<TopicKeyEventMarker[]>(() =>
  focusedTopicId.value == null ? [] : keyEvents.topicEntry(focusedTopicId.value).markers,
)

const focusDetachedMarkers = computed<TopicKeyEventMarker[]>(() => {
  const liveIds = new Set(focusNodes.value.map(n => n.id))
  return focusTopicMarkers.value
    .filter(m => m.source_section_id == null || !liveIds.has(m.source_section_id))
    .sort((a, b) => a.source_period_date < b.source_period_date ? -1 : a.source_period_date > b.source_period_date ? 1 : b.id - a.id)
})

type FocusColumn =
  | { kind: 'section', key: string, date: string, node: SectionTimelineNode }
  | { kind: 'marker', key: string, date: string, marker: TopicKeyEventMarker }

/** 合并列：真实 section 与失效快照同轴（快照不占 section ID、不伪造关系）。 */
const focusColumns = computed<FocusColumn[]>(() => {
  const cols: FocusColumn[] = focusNodes.value.map(n => ({
    kind: 'section', key: `fn-${n.id}`, date: n.period_date, node: n,
  }))
  for (const m of focusDetachedMarkers.value) {
    cols.push({ kind: 'marker', key: `km-${m.id}`, date: m.source_period_date, marker: m })
  }
  cols.sort((a, b) => (a.date < b.date ? -1 : a.date > b.date ? 1 : a.key < b.key ? -1 : 1))
  return cols
})

// --- 编排态（切片③）：手动建泳道，就地叠加在 lanes 上（composeMode 布尔态）---
// composable 负责候选池/勾选/聚合/离群/聚类质量/语义搜索/保存（含移出二次确认）的全部
// 纯逻辑与网络协调；host 只负责接线：节点 checkbox + distance/tier/离群标徽、顶部浮工具条、
// 右侧候选侧边栏、移出确认 AppDialog（requestMoveOutConfirm 经 Promise 桥接）。
const showMoveOutDialog = ref(false)
const pendingMoveOut = ref<MoveOutItem[]>([])
let resolveMoveOut: ((ok: boolean) => void) | null = null
// sidebar 搜索进行中（composable 未暴露 searching 态；host 监听 queryVec/searchError 近似收尾）。
const composeSearching = ref(false)

// boardId 适配 composable 的 Ref<number|string> 入参。
const boardIdRef = computed<number | string>(() => props.boardId)
// host 的 topics 是聚合的 TopicRow（驼峰），composable 需 BoardTopicListItem（蛇形）→ 就地映射。
const candidateTopics = computed<BoardTopicListItem[]>(() =>
  topics.value
    .filter(t => t.status === 'candidate')
    .map(t => ({
      id: t.id,
      semantic_board_id: props.boardId,
      label: t.label,
      description: '',
      status: t.status,
      first_seen_date: t.firstDate,
      last_seen_date: t.lastDate,
      hit_count: t.hitCount,
      consecutive_hits: t.consecutiveHits,
      section_count: t.sectionCount,
      color: t.color,
      can_activate: t.canActivate,
    })),
)

const compose = useInlineCompose({
  boardId: boardIdRef,
  candidateTopics,
  onSaved: async () => { await loadData() },
  requestMoveOutConfirm: async (items: MoveOutItem[]) => {
    pendingMoveOut.value = items
    showMoveOutDialog.value = true
    return await new Promise<boolean>(r => { resolveMoveOut = r })
  },
})
// composeMode 与 composable.active 同步：active=true 即进入编排态（viewMode 保持 lanes 不变）。
const composeMode = ref(false)
watch(() => compose.active.value, (v) => { composeMode.value = v })

// 进入编排态：切到 lanes（就地叠加），composable 置 active 并加载候选池。
function enterCompose() {
  viewMode.value = 'lanes'
  void compose.enter()
}

// 可保存：泳道名非空且有勾选。
const canSave = computed(() => compose.laneName.value.trim() !== '' && compose.selectedIds.value.size > 0)

// 节点编排态信息取值 helper：pool 用 string id，节点 id 是 number → String 对齐。
function nodeInfoFor(secId: number | string): NodeTierInfo | undefined {
  return compose.nodeInfo.value[String(secId)]
}

// 侧边栏「确认启用」：按 topicId 找到 SidebarCandidateItem 走 composable.activate（updateTopic + onSaved）。
function onSidebarActivate(topicId: number) {
  const item: SidebarCandidateItem | undefined = compose.sidebarItems.value.find(i => i.topic.id === topicId)
  if (item) void compose.activate(item)
}

// 侧边栏「相似 section 推荐」点击：直接 toggle 勾选（active 项等同勾走移出，复用现有移出提示/保存二次确认）。
function onSidebarRecommend(sectionId: string) {
  compose.toggle(sectionId)
}

// 语义搜索桥接：侧边栏文本 → composable.runSearch（debounce + embedQuery 冷启动）。
function onComposeQuery(text: string) {
  composeSearching.value = true
  compose.runSearch(text)
}
watch([() => compose.queryVec.value, () => compose.searchError.value], () => {
  composeSearching.value = false
})

// 移出确认对话框按钮（resolve 挂起的 Promise，composable.save 据此继续或中止）。
function confirmMoveOut() {
  showMoveOutDialog.value = false
  resolveMoveOut?.(true)
  resolveMoveOut = null
}
function cancelMoveOut() {
  showMoveOutDialog.value = false
  resolveMoveOut?.(false)
  resolveMoveOut = null
}

// --- Persistent topic management (list drives the lanes view) ---

interface TopicRow {
  id: number
  label: string
  status: string
  color: string
  sectionCount: number
  firstDate: string
  lastDate: string
  hitCount: number
  consecutiveHits: number
  canActivate: boolean
}

// 话题清单由 section-timeline 的 persistent_topic 字段聚合，无需额外端点。
const topics = computed<TopicRow[]>(() => {
  const map = new Map<number, TopicRow>()
  for (const s of sections.value) {
    const t = s.persistent_topic
    if (!t) continue
    const row = map.get(t.id) ?? {
      id: t.id, label: t.label, status: t.status, color: t.color,
      sectionCount: 0, firstDate: s.period_date.slice(0, 10), lastDate: s.period_date.slice(0, 10),
      hitCount: t.hit_count ?? 0, consecutiveHits: t.consecutive_hits, canActivate: t.can_activate,
    }
    row.sectionCount++
    const d = s.period_date.slice(0, 10)
    if (d < row.firstDate) row.firstDate = d
    if (d > row.lastDate) row.lastDate = d
    // status 以最新一天的为准（timeline 已按时间排序）。
    row.status = t.status
    row.consecutiveHits = t.consecutive_hits
    row.hitCount = t.hit_count ?? 0
    row.canActivate = t.can_activate
    map.set(t.id, row)
  }
  return [...map.values()].sort((a, b) => {
    // 归档沉底；其余按 section 数降序。
    const aArchived = a.status === 'archived' ? 1 : 0
    const bArchived = b.status === 'archived' ? 1 : 0
    if (aArchived !== bArchived) return aArchived - bArchived
    return b.sectionCount - a.sectionCount
  })
})

// lanes/timeline 总览激活时为本版块全部话题拉取 confirmed（fire-and-forget，
// 各 topic 独立缓存键；失败局部提示，不阻塞图渲染）。放在 topics 声明之后：
// immediate watch 在 setup 期同步触发，早于声明会 TDZ。依赖含 topics 签名：
// 时间线异步到达后话题集合从空变非空也要补拉。
const boardTopicIdsSig = computed(() => topics.value.map(t => t.id).join(','))
watch([() => props.boardId, viewMode, days, boardTopicIdsSig], ([boardIdVal, mode]) => {
  if (boardIdVal == null || mode === 'focus') return
  for (const topic of topics.value) {
    void keyEvents.ensureTopicKeyEvents(topic.id, days.value)
  }
}, { immediate: true })

const topicStats = computed(() => {
  let active = 0, candidate = 0, archived = 0
  for (const t of topics.value) {
    if (t.status === 'active') active++
    else if (t.status === 'candidate') candidate++
    else if (t.status === 'archived') archived++
  }
  return { active, candidate, archived, total: topics.value.length }
})

const topicStatusLabel: Record<string, string> = { candidate: '候选', active: '活跃', archived: '已归档' }
const unassignedCount = computed(() => sections.value.filter(s => !s.persistent_topic_id).length)
const nonActiveSectionCount = computed(() => sections.value.filter(s => s.persistent_topic?.status !== 'active').length)

// --- Data loading ---

async function loadData() {
  loading.value = true
  selectedNode.value = null
  try {
    const res = await getBoardSectionTimeline(props.boardId, days.value)
    if (res.success && res.data) {
      sections.value = res.data.sections || []
      relations.value = res.data.relations || []
    } else {
      sections.value = []
      relations.value = []
    }
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.boardId, days.value],
  () => { loadData() },
  { immediate: true },
)

// --- Focus view (slice C′)：重投影单话题节点到专注布局（复用 lanes 数据）---
const focusScrollRef = ref<HTMLDivElement | null>(null)
// focus 列宽（节点间距），独立于 lanes 的 COL_W，给单话题更多呼吸空间。
const FOCUS_COL_W = 96
// 拖拽 vs 点击阈值：超过该位移视为拖拽，吞掉后续 click（避免误触展开）。
const FOCUS_DRAG_THRESHOLD = 3

const focusNodes = computed<SectionTimelineNode[]>(() =>
  focusedTopicId.value == null ? [] : filterFocusNodes(sections.value, focusedTopicId.value),
)
const focusMeta = computed(() => buildFocusMeta(focusNodes.value))
const focusTimelineWidth = computed(() => focusColumns.value.length * FOCUS_COL_W + 32)
const focusMarkerCount = computed(() => focusTopicMarkers.value.length)
const focusTopicLabel = computed(() => {
  const id = focusedTopicId.value
  if (id == null) return ''
  return topics.value.find(t => t.id === id)?.label ?? `话题 #${id}`
})
const focusStatusLabel = computed(() => {
  const id = focusedTopicId.value
  if (id == null) return ''
  const t = topics.value.find(x => x.id === id)
  if (!t) return ''
  return t.status === 'active' ? 'Active · 持续演进' : (topicStatusLabel[t.status] ?? t.status)
})

function enterFocus(topicId: number) {
  focusedTopicId.value = topicId
  selectedNode.value = null
  selectedMarkerId.value = null
  popupThreads.value = []
  expandedThreadId.value = null
  viewMode.value = 'focus'
}

/** 点快照节点：原位详情（选择不标记、不研究、无假文章链接）。 */
const selectedMarker = computed<TopicKeyEventMarker | null>(() => {
  const id = selectedMarkerId.value
  if (id == null) return null
  return focusTopicMarkers.value.find(m => m.id === id) ?? null
})

function selectFocusMarker(marker: TopicKeyEventMarker) {
  selectedNode.value = null
  selectedMarkerId.value = marker.id
}

// 外部聚焦入口：TagsPage 从话题态势卡片/气泡点击切入本 tab 时聚焦该话题（focus 视图）。
// sections 未加载完时先记下 id，数据就绪后 focusNodes computed 自动补全视图。
watch(
  () => props.focusTopicId,
  (id) => {
    if (id == null) return
    enterFocus(id)
  },
  { immediate: true },
)
function exitFocus() {
  viewMode.value = 'lanes'
}
// 只有具名话题泳道（active topic）可进入专注视图；unassigned 不进。
function onLaneClick(lane: LaneRow) {
  if (suppressNextSvgClick) { suppressNextSvgClick = false; return }
  if (lane.topicId == null) return
  enterFocus(lane.topicId)
}

// 拖拽平移：pointer 事件区分 click vs drag（超阈值吞掉后续 click）。
const dragState = ref({ down: false, startX: 0, startScroll: 0, moved: false })
let suppressNextFocusClick = false
function onPointerDown(e: PointerEvent) {
  const el = focusScrollRef.value
  if (!el) return
  // 点在节点上时不启动拖拽，让节点 click 优先。
  if ((e.target as Element)?.closest?.('.btb-focus-node')) return
  dragState.value = { down: true, startX: e.clientX, startScroll: el.scrollLeft, moved: false }
  el.setPointerCapture?.(e.pointerId)
}
function onPointerMove(e: PointerEvent) {
  if (!dragState.value.down) return
  const dx = e.clientX - dragState.value.startX
  if (isDragMove(dx, FOCUS_DRAG_THRESHOLD)) dragState.value.moved = true
  const el = focusScrollRef.value
  if (el) el.scrollLeft = dragState.value.startScroll - dx
}
function endDrag() {
  if (dragState.value.down && dragState.value.moved) suppressNextFocusClick = true
  dragState.value.down = false
  dragState.value.moved = false
}
function onPointerUp() { endDrag() }
function onPointerCancel() { endDrag() }

// timeline/lanes 画布拖拽平移：pointer 改 scrollLeft/scrollTop，超阈值吞掉
// 节点/泳道 click，避免误触选中或误进 focus（与 focus 的 dragState 互斥，两视图不同时显示）。
const svgDragState = ref({ down: false, startX: 0, startY: 0, startScrollX: 0, startScrollY: 0, moved: false })
let suppressNextSvgClick = false
function onSvgPointerDown(e: PointerEvent) {
  const el = svgScrollRef.value
  if (!el) return
  // 点在节点 / 泳道 hover 操作菜单上时不启动拖拽，让其 click 优先。
  if ((e.target as Element)?.closest?.('.btb-dag-node, .btb-lane-op')) return
  svgDragState.value = { down: true, startX: e.clientX, startY: e.clientY, startScrollX: el.scrollLeft, startScrollY: el.scrollTop, moved: false }
  el.setPointerCapture?.(e.pointerId)
  // 拖拽平移期间禁用全局文本选择，避免框选文字干扰拖动（参考 AppSidebarView 做法）。
  document.body.style.userSelect = 'none'
}
function onSvgPointerMove(e: PointerEvent) {
  if (!svgDragState.value.down) return
  const dx = e.clientX - svgDragState.value.startX
  const dy = e.clientY - svgDragState.value.startY
  if (isDragMove(dx, FOCUS_DRAG_THRESHOLD) || isDragMove(dy, FOCUS_DRAG_THRESHOLD)) svgDragState.value.moved = true
  const el = svgScrollRef.value
  if (el) {
    el.scrollLeft = svgDragState.value.startScrollX - dx
    el.scrollTop = svgDragState.value.startScrollY - dy
  }
}
function endSvgDrag() {
  if (svgDragState.value.down && svgDragState.value.moved) suppressNextSvgClick = true
  svgDragState.value.down = false
  svgDragState.value.moved = false
  document.body.style.userSelect = ''
}
async function onSvgNodeClick(node: SectionTimelineNode) {
  if (suppressNextSvgClick) { suppressNextSvgClick = false; return }
  await selectNode(node)
}
async function onFocusNodeClick(node: SectionTimelineNode) {
  // 拖拽刚结束 → 吞掉这次 click，避免误触就地展开。
  if (suppressNextFocusClick) { suppressNextFocusClick = false; return }
  await selectNode(node)
}

// --- 工具条话题操作（回刷 / 合并）+ 泳道 hover 操作（重命名 / 归档·恢复 / 删除）---
// 原 TopicManageDialog 能力迁入工作台；二次确认统一走 AppDialog（禁 window.*）。
const opsError = ref<string | null>(null)
const busyTopicId = ref<number | null>(null)
const backfilling = ref(false)

// 泳道 hover 操作菜单的纵向锚点（泳道竖向中心）。
function laneOpsY(li: number): number {
  return (laneLayout.value.laneY[li] ?? 0) + PAD.value + (laneLayout.value.laneH[li] ?? LANE_BASE.value) / 2
}

// 泳道标题按可用宽度截断，避免长标题延伸到 hover 操作按钮区（重命名/归档/删除）造成重叠。
// 估算 10px 字号下宽度：中文/全角≈10px，其余≈5.5px。
function laneTextWidth(s: string): number {
  let w = 0
  for (const ch of s) {
    w += /[\u4e00-\u9fff\u3000-\u303f\uff00-\uffef]/.test(ch) ? 10 : 5.5
  }
  return w
}
function laneLabel(label: string): string {
  // 可用宽度 = 标签列宽 - 按钮组偏移(LANE_LABEL_W-68) - 标题起点(24) - 间距(6)
  const maxW = LANE_LABEL_W - 68 - 24 - 6
  if (laneTextWidth(label) <= maxW) return label
  let s = label
  while (s.length > 0 && laneTextWidth(s + '…') > maxW) {
    s = s.slice(0, -1)
  }
  return s + '…'
}

// 重命名
const renameTarget = ref<TopicRow | null>(null)
const renameLabel = ref('')
function startRename(lane: LaneRow) {
  const row = topics.value.find(t => t.id === lane.topicId) ?? null
  renameTarget.value = row
  renameLabel.value = lane.label
}
async function confirmRename() {
  const t = renameTarget.value
  if (!t) return
  const label = renameLabel.value.trim()
  if (!label || label === t.label) { renameTarget.value = null; return }
  busyTopicId.value = t.id
  const res = await updateTopic(t.id, { label })
  busyTopicId.value = null
  if (res.success) {
    renameTarget.value = null
    await loadData()
  } else {
    opsError.value = res.error || '重命名失败'
  }
}

// 归档 / 恢复（active→archived；archived→active）
const statusTarget = ref<TopicRow | null>(null)
const statusNext = ref<'archived' | 'active'>('archived')
function startStatusToggle(lane: LaneRow) {
  const row = topics.value.find(t => t.id === lane.topicId) ?? null
  if (!row) return
  statusTarget.value = row
  statusNext.value = row.status === 'archived' ? 'active' : 'archived'
}
async function confirmStatusToggle() {
  const t = statusTarget.value
  if (!t) return
  busyTopicId.value = t.id
  const res = await updateTopic(t.id, { status: statusNext.value })
  busyTopicId.value = null
  if (res.success) {
    statusTarget.value = null
    await loadData()
  } else {
    opsError.value = res.error || (statusNext.value === 'archived' ? '归档失败' : '恢复失败')
  }
}

// 删除（输入名称二次确认，硬删除不可恢复）
const deleteTarget = ref<TopicRow | null>(null)
const deleteConfirmText = ref('')
const deleteCanConfirm = computed(() => {
  const t = deleteTarget.value
  return !!t && deleteConfirmText.value.trim() === t.label.trim()
})
function startDelete(lane: LaneRow) {
  const row = topics.value.find(t => t.id === lane.topicId) ?? null
  if (!row) return
  deleteTarget.value = row
  deleteConfirmText.value = ''
}
async function confirmDelete() {
  const t = deleteTarget.value
  if (!t || !deleteCanConfirm.value) return
  busyTopicId.value = t.id
  const res = await deleteTopic(t.id)
  busyTopicId.value = null
  if (res.success) {
    deleteTarget.value = null
    deleteConfirmText.value = ''
    await loadData()
  } else {
    opsError.value = res.error || '删除失败'
  }
}

// 合并预览：拉取全量话题（含已归档），选源 + 目标，调 merge API。
const mergeOpen = ref(false)
const mergeAllTopics = ref<BoardTopicListItem[]>([])
const mergeLoading = ref(false)
const mergeSourceId = ref<number | null>(null)
const mergeTargetId = ref<number | null>(null)
const mergeCandidates = computed(() =>
  mergeAllTopics.value.filter(t => t.id !== mergeSourceId.value && t.status !== 'archived'),
)
async function startMerge() {
  mergeOpen.value = true
  mergeSourceId.value = null
  mergeTargetId.value = null
  mergeLoading.value = true
  const res = await listBoardTopics(props.boardId)
  mergeLoading.value = false
  if (res.success && res.data) {
    mergeAllTopics.value = res.data.topics ?? []
  } else {
    mergeAllTopics.value = []
    opsError.value = res.error || '加载话题失败'
  }
}
async function confirmMerge() {
  const sId = mergeSourceId.value
  const tId = mergeTargetId.value
  if (sId == null || tId == null) return
  busyTopicId.value = sId
  const res = await mergeTopics(tId, [sId])
  busyTopicId.value = null
  if (res.success) {
    mergeOpen.value = false
    mergeSourceId.value = null
    mergeTargetId.value = null
    await loadData()
  } else {
    opsError.value = res.error || '合并失败'
  }
}

// 回刷历史话题归属（原弹窗能力，迁入工具条）
async function runBackfill() {
  if (backfilling.value) return
  backfilling.value = true
  opsError.value = null
  const res = await backfillPersistentTopics(props.boardId)
  backfilling.value = false
  if (res.success) {
    // 后端异步重建，稍后刷新。
    setTimeout(() => { loadData() }, 4000)
  } else {
    opsError.value = res.error || '回刷失败'
  }
}

// 切换视图模式时清空选中/悬停，避免坐标体系变化后的错位高亮。
watch(viewMode, () => {
  selectedNode.value = null
  hoveredId.value = null
  popupThreads.value = []
  if (viewMode.value !== 'focus') focusedTopicId.value = null
})
</script>

<template>
  <div class="btb-container">
    <!-- Controls -->
    <div v-if="!composeMode" class="btb-controls">
      <div class="btb-controls-left">
        <Icon icon="mdi:source-branch" width="15" class="text-white/50" />
        <span class="btb-controls-title">话题总览</span>
      </div>
      <div class="btb-controls-actions">
        <div class="btb-days-toggle">
        <button
          v-for="opt in [{ v: 7, label: '7天' }, { v: 14, label: '14天' }, { v: 30, label: '30天' }, { v: 0, label: '全部' }]"
          :key="opt.v"
          class="btb-days-btn"
          :class="{ active: days === opt.v }"
          :title="opt.v === 0 ? '全部历史' : `最近 ${opt.v} 天`"
          @click="days = opt.v"
        >
          {{ opt.label }}
        </button>
        </div>
        <div class="btb-view-toggle">
        <button
          class="btb-view-btn"
          :class="{ active: viewMode === 'timeline' }"
          title="匈牙利相似度关系（默认）"
          @click="viewMode = 'timeline'"
        >
          <Icon icon="mdi:chart-timeline-variant" width="13" />
          <span>时间线</span>
        </button>
        <button
          class="btb-view-btn"
          :class="{ active: viewMode === 'lanes' }"
          title="按话题归类，同话题连续性实线连接"
          @click="viewMode = 'lanes'"
        >
          <Icon icon="mdi:view-stream-outline" width="13" />
          <span>话题泳道</span>
        </button>
        </div>
        <button
        v-if="showDetectiveEntry"
        class="btb-detective-btn"
        title="进入 3D 侦探墙"
        @click="emit('openDetectiveWall')"
      >
        <Icon icon="mdi:magnify-scan" width="16" />
        <span>侦探墙</span>
        </button>
        <button
          class="btb-tool-btn"
          title="回刷历史话题归属（原话题管理能力）"
          :disabled="backfilling"
          @click="runBackfill"
        >
          <Icon icon="mdi:database-refresh-outline" width="14" />
          <span>{{ backfilling ? '回刷中…' : '回刷归属' }}</span>
        </button>
        <button
          class="btb-tool-btn"
          title="合并预览（选源 + 目标话题）"
          @click="startMerge"
        >
          <Icon icon="mdi:merge" width="14" />
          <span>合并预览</span>
        </button>
        <button
          class="btb-tool-btn btb-tool-btn--primary"
          title="新建泳道（进入编排态）"
          @click="enterCompose"
        >
          <Icon icon="mdi:plus" width="14" />
          <span>新建泳道</span>
        </button>
      </div>
    </div>

    <!-- Ops error banner -->
    <div v-if="opsError" class="btb-ops-error" role="alert">
      <Icon icon="mdi:alert-circle-outline" width="14" />
      <span>{{ opsError }}</span>
      <button class="btb-ops-error-close" @click="opsError = null">✕</button>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="btb-loading">
      <div v-for="i in 3" :key="i" class="btb-skeleton" />
    </div>

    <!-- Empty -->
    <div v-else-if="sections.length === 0" class="btb-empty">
      <Icon icon="mdi:source-branch" width="28" class="text-white/15" />
      <p>暂无话题数据</p>
    </div>

    <!-- Focus view (single topic deep read) -->
    <div v-else-if="viewMode === 'focus'" class="btb-focus">
      <!-- 空话题降级：该话题在当前时间窗口内无节点，不报错。
           但确认快照独立于真实 section（spec：section 空但 markers 非空仍可显示，
           不被 empty 提前隐藏、不伪造 section）。 -->
      <div v-if="focusMeta.empty && focusDetachedMarkers.length === 0" class="btb-focus-empty">
        <Icon icon="mdi:bookmark-off-outline" width="22" />
        <p>该话题在当前时间窗口内暂无新动态</p>
        <button class="btb-focus-back" @click="exitFocus">
          <Icon icon="mdi:arrow-left" width="13" />
          返回总览
        </button>
      </div>
      <template v-else>
        <!-- sticky 标题：话题名 + 状态徽章 + 元信息（动态数/跨度/最近日期）-->
        <div class="btb-focus-head">
          <div class="btb-focus-head__top">
            <button class="btb-focus-back" @click="exitFocus">
              <Icon icon="mdi:arrow-left" width="13" />
              返回总览
            </button>
            <span v-if="focusStatusLabel" class="btb-focus-status">
              <span class="btb-focus-status__dot" />
              {{ focusStatusLabel }}
            </span>
          </div>
          <h2 class="btb-focus-title">{{ focusTopicLabel }}</h2>
          <div class="btb-focus-meta">
            <span><b>{{ focusMeta.count }}</b> 个动态</span>
            <span>跨度 <b>{{ focusMeta.spanDays }} 天</b></span>
            <span>最近 <b>{{ focusMeta.lastDate ? formatDateShort(focusMeta.lastDate) : '' }}</b></span>
            <span v-if="focusMarkerCount > 0"><b>{{ focusMarkerCount }}</b> 个关键节点</span>
            <button
              type="button"
              class="btb-ke-toggle"
              :aria-pressed="highlightKeyEvents"
              title="突出关键节点：弱化普通动态的显示，不移除背景、不改变时间位置"
              @click="highlightKeyEvents = !highlightKeyEvents"
            >
              <Icon icon="mdi:star-four-points" width="13" />
              {{ highlightKeyEvents ? '取消突出' : '突出关键节点' }}
            </button>
          </div>
        </div>
        <!-- 突出模式下零确认节点：显式提示（spec：当前窗口无确认节点 SHALL 提示）。 -->
        <p v-if="highlightKeyEvents && focusMarkerCount === 0" class="btb-ke-empty-hint" role="status">
          当前窗口内没有已确认关键节点——突出模式不改变内容；再点一次开关可恢复普通显示。
        </p>
        <!-- 横向时间轴（按住拖拽平移，区分 click/drag）-->
        <div
          ref="focusScrollRef"
          class="btb-focus-timeline-wrap"
          :class="{ 'btb-focus-timeline-wrap--dragging': dragState.down && dragState.moved }"
          @pointerdown="onPointerDown"
          @pointermove="onPointerMove"
          @pointerup="onPointerUp"
          @pointercancel="onPointerCancel"
        >
          <div class="btb-focus-timeline" :style="{ width: focusTimelineWidth + 'px' }">
            <div
              v-for="col in focusColumns"
              :key="col.key"
              class="btb-focus-col"
              :class="{
                'btb-focus-col--dimmed': highlightKeyEvents && col.kind === 'section' && sectionMarkerCount(col.node.id) === 0,
              }"
            >
              <div class="btb-focus-col__date"><b>{{ formatDateShort(col.date) }}</b></div>
              <!-- 真实 section 节点：原状态/坐标/交互不变；★n = 该来源独立 confirmed 注记数 -->
              <template v-if="col.kind === 'section'">
                <div
                  class="btb-focus-node"
                  :data-section="col.node.id"
                  :class="{ 'btb-focus-node--selected': selectedNode?.id === col.node.id }"
                  :title="col.node.cluster_label"
                  @click="onFocusNodeClick(col.node)"
                  @keydown.enter.prevent="onFocusNodeClick(col.node)"
                >
                  <span
                    v-if="sectionMarkerCount(col.node.id) > 0"
                    class="btb-focus-node__ke"
                    :title="`该来源 ${sectionMarkerCount(col.node.id)} 条已确认关键线索`"
                  >★{{ sectionMarkerCount(col.node.id) }}</span>
                  <div class="btb-focus-node__inner" />
                </div>
                <div class="btb-focus-node-label" :title="col.node.cluster_label">{{ truncateLabel(col.node.cluster_label) }}</div>
              </template>
              <!-- 失效来源快照：同轴独立注记（不占 section ID、不伪造关系/文章链接） -->
              <template v-else>
                <div
                  class="btb-focus-node btb-focus-node--snapshot"
                  :data-marker="col.marker.id"
                  :class="{ 'btb-focus-node--selected': selectedMarkerId === col.marker.id }"
                  :title="col.marker.title_snapshot"
                  role="button"
                  tabindex="0"
                  @click="selectFocusMarker(col.marker)"
                  @keydown.enter.prevent="selectFocusMarker(col.marker)"
                >
                  <span class="btb-focus-node__ke" title="来源已失效的确认快照">★</span>
                  <div class="btb-focus-node__inner" />
                </div>
                <div class="btb-focus-node-label btb-focus-node-label--snapshot" :title="col.marker.title_snapshot">
                  {{ truncateLabel(col.marker.title_snapshot) }}
                </div>
              </template>
            </div>
          </div>
        </div>
        <!-- 失效快照原位详情：标题/摘要/来源日期/确认时间 + 取消维护；不造文章链接 -->
        <div v-if="selectedMarker && !selectedNode" class="btb-focus-inline" data-testid="focus-marker-detail">
          <div class="btb-focus-inline__head">
            <span class="btb-focus-inline__date">{{ selectedMarker.source_period_date.slice(0, 10) }}</span>
            <span class="btb-focus-inline__title">{{ selectedMarker.title_snapshot }}</span>
            <span class="btb-focus-inline__meta">来源已失效 · 确认于 {{ selectedMarker.confirmed_at?.slice(0, 10) ?? '' }}</span>
          </div>
          <div class="btb-ke-detail" data-testid="focus-ke-list">
            <div class="btb-ke-detail__row">
              <Icon icon="mdi:star-check" width="13" aria-hidden="true" />
              <div class="btb-ke-detail__main">
                <span class="btb-ke-detail__title">{{ selectedMarker.title_snapshot }}</span>
                <span v-if="selectedMarker.summary_snapshot" class="btb-ke-detail__summary">{{ selectedMarker.summary_snapshot }}</span>
              </div>
              <div class="btb-ke-detail__actions">
                <button
                  v-if="selectedMarker.source_report_id != null"
                  type="button"
                  class="btb-ke-detail__link"
                  @click="emit('openDailyReport', selectedMarker.source_report_id!)"
                >来源日报 {{ selectedMarker.source_period_date.slice(0, 10) }}</button>
                <button
                  type="button"
                  class="btb-ke-detail__unmark"
                  :disabled="keyEvents.markerView(selectedMarker.id).pending != null"
                  @click="unmarkMarker(selectedMarker)"
                >取消</button>
              </div>
              <span v-if="markerErrorMessage(selectedMarker)" class="btb-ke-detail__error" role="alert">{{ markerErrorMessage(selectedMarker) }}（可重试取消）</span>
            </div>
          </div>
          <p class="btb-ke-detail__hint">原线索已被重生成替换，以上为确认时快照；不提供原文链接。</p>
        </div>
        <!-- 就地展开 thread（accordion，focus 模式 SHALL NOT 弹 popup overlay）-->
        <div v-if="selectedNode" class="btb-focus-inline">
          <div class="btb-focus-inline__head">
            <span class="btb-focus-inline__date">{{ formatDateShort(selectedNode.period_date) }}</span>
            <span class="btb-focus-inline__title">{{ selectedNode.cluster_label }}</span>
            <span class="btb-focus-inline__meta">{{ selectedNode.article_count }} 篇 · {{ selectedNode.thread_count }} 线索</span>
          </div>
          <!-- confirmed 注记维护：仅取消纠错 + 来源日报导航；无新挑选、无 AI 理由 -->
          <div v-if="sectionMarkers(selectedNode.id).length > 0" class="btb-ke-detail" data-testid="focus-ke-list">
            <div
              v-for="marker in sectionMarkers(selectedNode.id)"
              :key="marker.id"
              class="btb-ke-detail__row"
            >
              <Icon icon="mdi:star-check" width="13" aria-hidden="true" />
              <div class="btb-ke-detail__main">
                <span class="btb-ke-detail__title">{{ marker.title_snapshot }}</span>
                <span v-if="marker.summary_snapshot" class="btb-ke-detail__summary">{{ marker.summary_snapshot }}</span>
              </div>
              <div class="btb-ke-detail__actions">
                <button
                  v-if="marker.source_report_id != null"
                  type="button"
                  class="btb-ke-detail__link"
                  @click="emit('openDailyReport', marker.source_report_id!)"
                >来源日报 {{ marker.source_period_date.slice(0, 10) }}</button>
                <button
                  type="button"
                  class="btb-ke-detail__unmark"
                  :disabled="keyEvents.markerView(marker.id).pending != null"
                  @click="unmarkMarker(marker)"
                >取消</button>
              </div>
              <span v-if="markerErrorMessage(marker)" class="btb-ke-detail__error" role="alert">{{ markerErrorMessage(marker) }}（可重试取消）</span>
            </div>
          </div>
          <div class="btb-threads">
            <div v-if="popupThreadsLoading" class="btb-threads-loading">
              <div v-for="i in 2" :key="i" class="btb-thread-skeleton" />
            </div>
            <div v-else-if="popupThreads.length === 0" class="btb-threads-empty">无关联线索</div>
            <div
              v-else
              v-for="thread in popupThreads"
              :key="thread.id"
              class="btb-thread"
              :class="{ 'btb-thread--expanded': expandedThreadId === thread.id }"
            >
              <div class="btb-thread-header" @click="toggleThreadArticles(thread)">
                <Icon icon="mdi:chevron-right" width="14" class="btb-thread-arrow" />
                <span class="btb-thread-title">{{ thread.title }}</span>
                <span class="btb-thread-count">{{ thread.related_article_ids?.length || 0 }}篇</span>
              </div>
              <div v-if="thread.summary" class="btb-thread-summary">{{ thread.summary }}</div>
              <Transition name="btb-slide">
                <div v-if="expandedThreadId === thread.id" class="btb-articles">
                  <div v-if="threadArticlesLoading" class="btb-articles-loading">加载中…</div>
                  <template v-else>
                    <div
                      v-for="art in (threadArticles.get(thread.id) || [])"
                      :key="art.id"
                      class="btb-article"
                      @click="emit('openArticle', art.id)"
                    >
                      <Icon icon="mdi:file-document-outline" width="12" class="btb-article-icon" />
                      <span class="btb-article-title">{{ art.title }}</span>
                      <Icon icon="mdi:eye-outline" width="10" class="btb-article-external" />
                    </div>
                    <div v-if="(thread.related_article_ids?.length ?? 0) > 10" class="btb-articles-more">
                      还有 {{ (thread.related_article_ids?.length ?? 0) - 10 }} 篇…
                    </div>
                  </template>
                </div>
              </Transition>
            </div>
          </div>
        </div>
      </template>
    </div>

    <!-- Timeline -->
    <div v-else class="btb-chart">
      <!-- 编排态浮层：顶部工具条 + 右侧候选侧边栏，position:absolute 浮于 lanes SVG 之上 -->
      <template v-if="composeMode">
        <div class="btb-compose-toolbar-anchor">
          <ComposeInlineToolbar
            :lane-name="compose.laneName.value"
            :member-count="compose.quality.value.memberCount"
            :mean-distance="compose.quality.value.meanDistance"
            :outlier-count="compose.quality.value.outlierCount"
            :unassigned-count="compose.counts.value.unassigned"
            :move-out-count="compose.counts.value.moveOut"
            :saving="compose.saving.value"
            :can-save="canSave"
            @update:lane-name="compose.laneName.value = $event"
            @save="compose.save()"
            @cancel="compose.cancel()"
          />
        </div>
        <div
          class="btb-compose-sidebar-anchor"
          :class="{ 'is-collapsed': composeSidebarCollapsed }"
        >
          <button
            class="btb-compose-sidebar-toggle"
            :title="composeSidebarCollapsed ? '展开候选栏' : '收起候选栏'"
            @click="composeSidebarCollapsed = !composeSidebarCollapsed"
          >
            <Icon :icon="composeSidebarCollapsed ? 'mdi:chevron-left' : 'mdi:chevron-right'" width="18" />
          </button>
          <ComposeSidebar
            v-show="!composeSidebarCollapsed"
            :items="compose.sidebarItems.value"
            :query-text="compose.queryText.value"
            :search-error="compose.searchError.value"
            :searching="composeSearching"
            :recommendations="compose.recommendations.value"
            :recommendation-title="compose.recommendationTitle.value"
            @update:query-text="onComposeQuery"
            @activate="onSidebarActivate"
            @adopt="compose.adopt"
            @recommend="onSidebarRecommend"
          />
        </div>
      </template>
      <!-- SVG canvas: 间距随 zoomScale 变化（svgWidth/Height 已含缩放），无需 transform spacer。 -->
      <div
        ref="svgScrollRef"
        class="btb-svg-scroll"
        :class="{ 'btb-svg-scroll--dragging': svgDragState.down && svgDragState.moved }"
        :style="composeMode ? { paddingRight: (composeSidebarCollapsed ? 52 : 324) + 'px' } : undefined"
        @pointerdown="onSvgPointerDown"
        @pointermove="onSvgPointerMove"
        @pointerup="endSvgDrag"
        @pointercancel="endSvgDrag"
      >
        <svg
          :width="svgWidth"
          :height="svgHeight"
          class="btb-svg"
          :style="{ '--fs': fontScale }"
        >
          <!-- Date labels live inside the same pan/zoom coordinate system. -->
          <text
            v-for="col in dateColumns"
            :key="'date-' + col.date"
            :x="col.x"
            :y="12"
            text-anchor="middle"
            class="btb-date-label"
          >{{ col.label }}</text>

          <!-- Grid lines -->
          <line
            v-for="col in dateColumns"
            :key="'grid-' + col.date"
            :x1="col.x"
            :y1="0"
            :x2="col.x"
            :y2="svgHeight"
            :stroke="svgGridColor"
            stroke-width="1"
          />

          <!-- Lane backgrounds + labels (lanes mode only) -->
          <template v-if="viewMode === 'lanes'">
            <g
              v-for="(lane, li) in laneRows"
              :key="'lane-' + lane.key"
              class="btb-lane-row"
              :class="{ 'btb-lane-row--enterable': lane.topicId != null, 'btb-lane-row--dimmed': composeMode && lane.topicId != null }"
              :data-topic="lane.topicId ?? ''"
              @click="onLaneClick(lane)"
            >
              <rect
                :x="0"
                :y="(laneLayout.laneY[li] ?? 0) + PAD"
                :width="svgWidth"
                :height="laneLayout.laneH[li] ?? LANE_BASE"
                :fill="li % 2 === 0 ? svgLaneStripeColor : 'transparent'"
              />
              <circle
                :cx="14"
                :cy="(laneLayout.laneY[li] ?? 0) + PAD + (laneLayout.laneH[li] ?? LANE_BASE) / 2"
                :r="5"
                :fill="lane.color"
              />
              <text
                :x="24"
                :y="(laneLayout.laneY[li] ?? 0) + PAD + (laneLayout.laneH[li] ?? LANE_BASE) / 2 + 4"
                class="btb-lane-label"
                :fill="svgLaneLabelColor"
              >{{ laneLabel(lane.label) }}<title>{{ lane.label }}</title></text>
              <!-- 泳道 hover 操作菜单（重命名 / 归档·恢复 / 删除）；source=manual 与 auto 一视同仁 -->
              <g
                v-if="lane.topicId != null"
                class="btb-lane-ops"
                :data-topic="lane.topicId"
                :transform="`translate(${LANE_LABEL_W - 68}, ${laneOpsY(li) - 9})`"
                @click.stop
              >
                <g class="btb-lane-op" role="button" aria-label="重命名" @click.stop="startRename(lane)">
                  <title>重命名</title>
                  <rect class="btb-lane-op-hit" width="20" height="18" rx="3" />
                  <path class="btb-lane-op-ico" d="M3 14l1.2-3.2 6.3-6.3 2 2-6.3 6.3L3 14z M10.5 6.5l2 2" />
                </g>
                <g class="btb-lane-op" transform="translate(23 0)" role="button"
                   :aria-label="lane.status === 'archived' ? '恢复' : '归档'" @click.stop="startStatusToggle(lane)">
                  <title>{{ lane.status === 'archived' ? '恢复' : '归档' }}</title>
                  <rect class="btb-lane-op-hit" width="20" height="18" rx="3" />
                  <path class="btb-lane-op-ico" d="M3 5h14v3H3z M5.5 8.5v6.5h9V8.5 M9 8.5v6.5" />
                </g>
                <g class="btb-lane-op btb-lane-op--danger" transform="translate(46 0)" role="button"
                   aria-label="删除" @click.stop="startDelete(lane)">
                  <title>删除</title>
                  <rect class="btb-lane-op-hit" width="20" height="18" rx="3" />
                  <path class="btb-lane-op-ico" d="M5 6.5h10l-.8 8H5.8L5 6.5z M8 4.5h4 M4 6.5h12" />
                </g>
              </g>
              <!-- unassigned 泳道头部「新建泳道」入口（编排态主战场入口，非编排态显示） -->
              <g
                v-if="!composeMode && lane.key === 'unassigned'"
                class="btb-lane-op btb-lane-compose-entry"
                role="button"
                aria-label="新建泳道"
                :transform="`translate(${LANE_LABEL_W - 24}, ${laneOpsY(li) - 9})`"
                @click.stop="enterCompose"
              >
                <title>新建泳道（进入就地编排）</title>
                <rect class="btb-lane-op-hit btb-lane-compose-entry__hit" width="20" height="18" rx="3" />
                <path class="btb-lane-op-ico" d="M4 9h12M10 3v12" />
              </g>
            </g>
          </template>

          <!-- Edges -->
          <path
            v-for="edge in edgePaths"
            :key="edge.key"
            :d="edge.d"
            fill="none"
            :stroke="viewMode === 'lanes' && edge.relationType === 'identity' ? (edge.color || '#60a5fa') : getEdgeColor(edge.distance, isEdgeHighlighted(edge))"
            :stroke-width="viewMode === 'lanes' && edge.relationType === 'identity' ? 2 : (isEdgeHighlighted(edge) ? 2.5 : edgeWeight(edge.distance))"
            :stroke-dasharray="viewMode === 'lanes' && edge.relationType !== 'identity' ? '4 3' : undefined"
            :opacity="viewMode === 'lanes' && edge.relationType === 'identity' ? (isEdgeHighlighted(edge) ? 0.95 : 0.55) : (viewMode === 'lanes' ? 0.5 : 1)"
          >
            <title>{{ edge.relationType === 'identity' ? '同话题延续' : '相似度' }} · 距离: {{ edge.distance.toFixed(3) }}</title>
          </path>

          <!-- Nodes -->
          <g
            v-for="pn in positionedNodes"
            :key="'node-' + pn.data.id"
            class="btb-dag-node"
            :class="{
              'btb-dag-node--ending': pn.data.status === 'ending',
              'btb-dag-node--manual': pn.data.topic_match_confidence === 'manual',
              'btb-dag-node--selected': selectedNode?.id === pn.data.id,
              'btb-dag-node--lineage': isNodeHighlighted(pn.data.id),
              'btb-dag-node--dimmed': hoveredId !== null && !isNodeHighlighted(pn.data.id),
              'btb-dag-node--compose-on': composeMode && compose.isSelected(String(pn.data.id)),
              'btb-dag-node--compose-outlier': composeMode && nodeInfoFor(pn.data.id)?.isOutlier === true,
            }"
            @click="onSvgNodeClick(pn.data)"
            @mouseenter="hoveredId = pn.data.id"
            @mouseleave="hoveredId = null"
          >
            <title>{{ pn.data.cluster_label }}（{{ pn.data.period_date.slice(0, 10) }}）{{ pn.data.persistent_topic ? '\n话题：' + pn.data.persistent_topic.label : '' }}{{ pn.data.topic_match_confidence === 'manual' ? '\n人工归属' : '' }}{{ pn.data.article_count ? '\n' + pn.data.article_count + ' 篇 · ' + pn.data.thread_count + ' 线索' : '' }}{{ sectionMarkerCount(pn.data.id) ? '\n★ ' + sectionMarkerCount(pn.data.id) + ' 条已确认关键线索' : '' }}</title>
            <!-- ★n：lanes 总览的独立 confirmed 注记徽标（不进 timeline DAG；
                 同轴贴节点上方，不改状态色/坐标/关系）。 -->
            <text
              v-if="viewMode === 'lanes' && sectionMarkerCount(pn.data.id) > 0"
              :x="pn.cx"
              :y="pn.cy - NODE_R - 5"
              text-anchor="middle"
              class="btb-node-ke-badge"
              :class="{ 'btb-node-ke-badge--hot': highlightKeyEvents }"
            >★{{ sectionMarkerCount(pn.data.id) }}</text>
            <template v-if="pn.data.topic_match_confidence === 'manual'">
              <!-- manual confidence：双环描边，独立样式，不套算法三态（task 3.9）-->
              <circle
                :cx="pn.cx"
                :cy="pn.cy"
                :r="selectedNode?.id === pn.data.id ? NODE_R + 2 : NODE_R"
                class="btb-manual-ring"
              />
              <circle
                :cx="pn.cx"
                :cy="pn.cy"
                :r="(selectedNode?.id === pn.data.id ? NODE_R + 2 : NODE_R) - 4"
                class="btb-manual-core"
              />
            </template>
            <circle
              v-else
              :cx="pn.cx"
              :cy="pn.cy"
              :r="selectedNode?.id === pn.data.id ? NODE_R + 2 : NODE_R"
              :fill="statusFill(pn.data.status)"
              :stroke-dasharray="pn.data.status === 'ending' ? '3 2' : undefined"
              :stroke="svgNodeStroke"
              :stroke-width="selectedNode?.id === pn.data.id ? 2 : 1"
            />
            <text
              :x="pn.cx"
              :y="pn.cy + NODE_R + 12"
              text-anchor="middle"
              class="btb-node-label"
              :fill="statusFill(pn.data.status)"
            >
              {{ truncateLabel(pn.data.cluster_label) }}
            </text>
            <!-- 编排态：就地 checkbox（左上角），勾选进入新泳道 -->
            <g
              v-if="composeMode"
              class="btb-compose-check"
              :class="{ 'btb-compose-check--on': compose.isSelected(String(pn.data.id)) }"
              @click.stop="compose.toggle(String(pn.data.id))"
            >
              <title>{{ compose.isSelected(String(pn.data.id)) ? '取消勾选' : '勾选进入新泳道' }}</title>
              <rect
                :x="pn.cx - 13"
                :y="pn.cy - 13"
                width="12"
                height="12"
                rx="2"
                class="btb-compose-check__box"
              />
              <path
                v-if="compose.isSelected(String(pn.data.id))"
                :d="`M${pn.cx - 10},${pn.cy - 7} l2.4,2.6 l4.6,-5`"
                class="btb-compose-check__tick"
              />
            </g>
            <!-- 编排态：distance/tier 标徽（节点右侧，tier 着色） -->
            <g
              v-if="composeMode && nodeInfoFor(pn.data.id)"
              class="btb-compose-badge"
              :class="'btb-compose-badge--' + nodeInfoFor(pn.data.id)!.tier"
            >
              <text :x="pn.cx + NODE_R + 4" :y="pn.cy - 1" class="btb-compose-badge__tier">
                {{ TIER_LABEL[nodeInfoFor(pn.data.id)!.tier] }}
              </text>
              <text :x="pn.cx + NODE_R + 4" :y="pn.cy + 9" class="btb-compose-badge__dist">
                {{ nodeInfoFor(pn.data.id)!.distance.toFixed(2) }}
              </text>
            </g>
            <!-- 编排态：勾走 active section → 移出提示（节点下方，警示色） -->
            <text
              v-if="composeMode && nodeInfoFor(pn.data.id)?.moveOut && nodeInfoFor(pn.data.id)?.selected"
              :x="pn.cx"
              :y="pn.cy + NODE_R + 24"
              text-anchor="middle"
              class="btb-compose-moveout"
            >将从【{{ nodeInfoFor(pn.data.id)?.originLabel }}】移出</text>
          </g>
        </svg>
      </div>
      <!-- 缩放控制条 -->
      <div class="btb-zoom-bar">
        <button class="btb-zoom-btn" :disabled="zoomScale <= MIN_SCALE" title="缩小" @click="zoomOut">
          <Icon icon="mdi:minus" width="14" />
        </button>
        <button class="btb-zoom-label" :disabled="zoomScale === 1" title="重置为 100%" @click="resetZoom">{{ zoomPercent }}%</button>
        <button class="btb-zoom-btn" :disabled="zoomScale >= MAX_SCALE" title="放大" @click="zoomIn">
          <Icon icon="mdi:plus" width="14" />
        </button>
      </div>
    </div>

    <!-- 编排态：保存前移出二次确认（composable.requestMoveOutConfirm → Promise 桥接） -->
    <AppDialog
      :model-value="showMoveOutDialog"
      title="确认移出"
      :close-on-overlay="false"
      @update:model-value="(v) => { if (!v) cancelMoveOut() }"
    >
      <p class="btb-moveout-body">
        以下 {{ pendingMoveOut.length }} 条 section 将从原泳道移出、归入新泳道「<b>{{ compose.laneName.value || '未命名' }}</b>」（单值覆盖，原话题内容保留）：
      </p>
      <ul class="btb-moveout-list">
        <li v-for="(it, i) in pendingMoveOut" :key="i">{{ it.label }}（来自：{{ it.origin }}）</li>
      </ul>
      <template #footer>
        <AppButton variant="secondary" @click="cancelMoveOut">取消</AppButton>
        <AppButton variant="primary" :loading="compose.saving.value" @click="confirmMoveOut">确认移出并保存</AppButton>
      </template>
    </AppDialog>
  </div>

  <!-- Node detail popup -->
  <Teleport to="body">
    <Transition name="btb-popup">
      <div v-if="selectedNode && viewMode !== 'focus'" class="btb-popup-overlay" @click.self="selectedNode = null">
        <div class="btb-popup">
          <div class="btb-popup-header">
            <span
              class="btb-popup-status"
              :style="{ background: statusFill(selectedNode.status) }"
            >
              {{ statusLabels[selectedNode.status] || selectedNode.status }}
            </span>
            <button class="btb-popup-close" @click="selectedNode = null">
              <Icon icon="mdi:close" width="14" />
            </button>
          </div>
          <div class="btb-popup-title">{{ selectedNode.cluster_label }}</div>
          <div class="btb-popup-meta">
            <span>{{ formatDateShort(selectedNode.period_date) }}</span>
            <span>{{ selectedNode.article_count }} 篇</span>
            <span>{{ selectedNode.thread_count }} 条线索</span>
          </div>
          <!-- confirmed 注记维护（总览只读 confirmed：取消纠错 + 来源日报导航） -->
          <div v-if="sectionMarkers(selectedNode.id).length > 0" class="btb-ke-detail btb-ke-detail--popup" data-testid="popup-ke-list">
            <div
              v-for="marker in sectionMarkers(selectedNode.id)"
              :key="marker.id"
              class="btb-ke-detail__row"
            >
              <Icon icon="mdi:star-check" width="13" aria-hidden="true" />
              <div class="btb-ke-detail__main">
                <span class="btb-ke-detail__title">{{ marker.title_snapshot }}</span>
                <span v-if="marker.summary_snapshot" class="btb-ke-detail__summary">{{ marker.summary_snapshot }}</span>
              </div>
              <div class="btb-ke-detail__actions">
                <button
                  v-if="marker.source_report_id != null"
                  type="button"
                  class="btb-ke-detail__link"
                  @click="emit('openDailyReport', marker.source_report_id!)"
                >来源日报</button>
                <button
                  type="button"
                  class="btb-ke-detail__unmark"
                  :disabled="keyEvents.markerView(marker.id).pending != null"
                  @click="unmarkMarker(marker)"
                >取消</button>
              </div>
              <span v-if="markerErrorMessage(marker)" class="btb-ke-detail__error" role="alert">{{ markerErrorMessage(marker) }}（可重试取消）</span>
            </div>
          </div>

          <!-- Threads -->
          <div class="btb-threads">
            <div v-if="popupThreadsLoading" class="btb-threads-loading">
              <div v-for="i in 2" :key="i" class="btb-thread-skeleton" />
            </div>
            <div v-else-if="popupThreads.length === 0" class="btb-threads-empty">
              无关联线索
            </div>
            <div
              v-else
              v-for="thread in popupThreads"
              :key="thread.id"
              class="btb-thread"
              :class="{ 'btb-thread--expanded': expandedThreadId === thread.id }"
            >
              <div class="btb-thread-header" @click="toggleThreadArticles(thread)">
                <Icon icon="mdi:chevron-right" width="14" class="btb-thread-arrow" />
                <span class="btb-thread-title">{{ thread.title }}</span>
                <span class="btb-thread-count">{{ thread.related_article_ids?.length || 0 }}篇</span>
              </div>
              <div v-if="thread.summary" class="btb-thread-summary">{{ thread.summary }}</div>

              <!-- Articles -->
              <Transition name="btb-slide">
                <div v-if="expandedThreadId === thread.id" class="btb-articles">
                  <div v-if="threadArticlesLoading" class="btb-articles-loading">加载中…</div>
                  <template v-else>
                    <div
                      v-for="art in (threadArticles.get(thread.id) || [])"
                      :key="art.id"
                      class="btb-article"
                      @click="emit('openArticle', art.id)"
                    >
                      <Icon icon="mdi:file-document-outline" width="12" class="btb-article-icon" />
                      <span class="btb-article-title">{{ art.title }}</span>
                      <Icon icon="mdi:eye-outline" width="10" class="btb-article-external" />
                    </div>
                    <div
                      v-if="thread.related_article_ids?.length > 10"
                      class="btb-articles-more"
                    >
                      还有 {{ thread.related_article_ids.length - 10 }} 篇…
                    </div>
                  </template>
                </div>
              </Transition>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>

  <!-- 泳道操作确认弹窗（迁自 TopicManageDialog；二次确认统一走 AppDialog，禁 window.*） -->
  <!-- 重命名 -->
  <AppDialog
    :model-value="renameTarget !== null"
    title="重命名话题"
    width="440px"
    @update:model-value="(v) => { if (!v) renameTarget = null }"
  >
    <div class="btb-field">
      <label class="btb-field-label">话题名称</label>
      <AppInput v-model="renameLabel" type="text" placeholder="输入新名称" />
    </div>
    <template #footer>
      <AppButton variant="secondary" @click="renameTarget = null">取消</AppButton>
      <AppButton variant="primary" :loading="busyTopicId === renameTarget?.id" @click="confirmRename">保存</AppButton>
    </template>
  </AppDialog>

  <!-- 归档 / 恢复 -->
  <AppDialog
    :model-value="statusTarget !== null"
    :title="statusNext === 'archived' ? '归档话题' : '恢复话题'"
    width="420px"
    @update:model-value="(v) => { if (!v) statusTarget = null }"
  >
    <p class="btb-confirm-body">
      {{ statusNext === 'archived'
        ? `确定归档「${statusTarget?.label}」？归档后不再参与新归属，可在此恢复。`
        : `确定恢复「${statusTarget?.label}」为活跃话题？恢复后将重新参与话题泳道与新归属。` }}
    </p>
    <template #footer>
      <AppButton variant="secondary" @click="statusTarget = null">取消</AppButton>
      <AppButton variant="primary" :loading="busyTopicId === statusTarget?.id" @click="confirmStatusToggle">
        {{ statusNext === 'archived' ? '归档' : '恢复' }}
      </AppButton>
    </template>
  </AppDialog>

  <!-- 删除（输入名称二次确认） -->
  <AppDialog
    :model-value="deleteTarget !== null"
    title="删除话题"
    width="440px"
    @update:model-value="(v) => { if (!v) deleteTarget = null }"
  >
    <div class="btb-field">
      <div class="btb-danger-banner">
        <Icon icon="mdi:alert" width="18" />
        <span>硬删除不可恢复。话题「{{ deleteTarget?.label }}」下的 {{ deleteTarget?.sectionCount }} 条 section 将解除归属但保留内容。</span>
      </div>
      <label class="btb-field-label">输入话题名称「{{ deleteTarget?.label }}」以确认</label>
      <AppInput v-model="deleteConfirmText" type="text" placeholder="话题名称" />
    </div>
    <template #footer>
      <AppButton variant="secondary" @click="deleteTarget = null">取消</AppButton>
      <AppButton variant="danger" :disabled="!deleteCanConfirm" :loading="busyTopicId === deleteTarget?.id" @click="confirmDelete">删除</AppButton>
    </template>
  </AppDialog>

  <!-- 合并预览 -->
  <AppDialog
    :model-value="mergeOpen"
    title="合并话题"
    width="480px"
    @update:model-value="(v) => { mergeOpen = v; if (!v) { mergeSourceId = null; mergeTargetId = null } }"
  >
    <p class="btb-confirm-body">选择源话题（将被归档、其 section 改指目标）与目标话题（保留）。</p>
    <div class="btb-merge-cols">
      <div class="btb-merge-col">
        <div class="btb-merge-col__t">源话题</div>
        <div v-if="mergeLoading" class="btb-merge-empty">加载中…</div>
        <button
          v-for="t in mergeAllTopics"
          v-else
          :key="'ms-' + t.id"
          type="button"
          class="btb-merge-item"
          :class="{ 'btb-merge-item--sel': mergeSourceId === t.id }"
          @click="mergeSourceId = t.id"
        >
          <span class="btb-merge-dot" :style="{ background: t.color }" />
          <span class="btb-merge-label">{{ t.label }}</span>
          <span class="btb-merge-count">{{ t.section_count }} 条</span>
        </button>
      </div>
      <div class="btb-merge-col">
        <div class="btb-merge-col__t">目标话题</div>
        <div v-if="mergeLoading" class="btb-merge-empty">加载中…</div>
        <button
          v-for="t in mergeCandidates"
          v-else
          :key="'mt-' + t.id"
          type="button"
          class="btb-merge-item"
          :class="{ 'btb-merge-item--sel': mergeTargetId === t.id }"
          @click="mergeTargetId = t.id"
        >
          <span class="btb-merge-dot" :style="{ background: t.color }" />
          <span class="btb-merge-label">{{ t.label }}</span>
          <span class="btb-merge-count">{{ t.section_count }} 条</span>
        </button>
        <div v-if="!mergeLoading && mergeCandidates.length === 0" class="btb-merge-empty">无其他可合并话题</div>
      </div>
    </div>
    <template #footer>
      <AppButton variant="secondary" @click="mergeOpen = false">取消</AppButton>
      <AppButton
        variant="primary"
        :disabled="mergeSourceId === null || mergeTargetId === null"
        :loading="busyTopicId === mergeSourceId"
        @click="confirmMerge"
      >确认合并</AppButton>
    </template>
  </AppDialog>
</template>

<style scoped>
.btb-container {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  /* 工作台化：去除悬浮卡片 chrome（修悬浮留白），擑满父容器宽高 */
  margin-top: 0;
  padding: 0.6rem 0.75rem;
  border-radius: 0;
  border: 0;
  background: transparent;
}

/* Controls */
.btb-controls {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 0.5rem;
}

.btb-controls-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-left: auto;
}

.btb-controls-left {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.btb-controls-title {
  font-size: 0.78rem;
  font-weight: 600;
  color: var(--color-text-secondary);
}

.btb-days-toggle {
  display: flex;
  gap: 0.25rem;
}

.btb-days-btn {
  padding: 0.2rem 0.55rem;
  font-size: 0.65rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: all 0.12s ease;
}

.btb-days-btn:hover {
  color: var(--color-text-secondary);
  border-color: var(--color-border-strong);
}

.btb-days-btn.active {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
  border-color: var(--color-border-strong);
}

/* Detective wall entry button */
.btb-detective-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.25rem 0.6rem;
  font-size: 0.65rem;
  border: 1px solid #DC2626;
  border-radius: 4px;
  background: rgba(220, 38, 38, 0.1);
  color: #DC2626;
  cursor: pointer;
  transition: all 0.12s ease;
}

.btb-detective-btn:hover {
  background: #DC2626;
  color: #fff;
}

/* §话题管理最小版 */
.btb-detective-btn--active {
  background: rgba(96, 165, 250, 0.18);
  border-color: #60a5fa;
  color: #93c5fd;
}
.btb-topics {
  margin: 0 0 0.75rem;
  padding: 0.75rem;
  border: 1px solid rgba(96, 165, 250, 0.22);
  border-radius: 8px;
  background: rgba(15, 23, 42, 0.45);
}
.btb-topics-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  margin-bottom: 0.6rem;
  flex-wrap: wrap;
}
.btb-topics-stats {
  display: flex;
  gap: 0.85rem;
  font-size: 0.72rem;
  color: rgba(226, 232, 240, 0.7);
}
.btb-topics-stats b {
  color: #e2e8f0;
  font-weight: 600;
}
.btb-topics-warn {
  color: #fbbf24;
}
.btb-topics-backfill {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.3rem 0.7rem;
  font-size: 0.7rem;
  border: 1px solid #60a5fa;
  border-radius: 4px;
  background: rgba(96, 165, 250, 0.12);
  color: #93c5fd;
  cursor: pointer;
  transition: all 0.12s ease;
}
.btb-topics-backfill:hover:not(:disabled) {
  background: rgba(96, 165, 250, 0.28);
}
.btb-topics-backfill:disabled {
  opacity: 0.55;
  cursor: wait;
}
.btb-topics-empty {
  padding: 1rem 0.5rem;
  font-size: 0.72rem;
  text-align: center;
  color: rgba(226, 232, 240, 0.45);
}
.btb-topics-list {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  max-height: 320px;
  overflow-y: auto;
}
.btb-topic-row {
  display: flex;
  align-items: center;
  gap: 0.55rem;
  padding: 0.45rem 0.55rem;
  border-radius: 6px;
  background: rgba(30, 41, 59, 0.5);
}
.btb-topic-row--archived {
  opacity: 0.5;
}
.btb-topic-color {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}
.btb-topic-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.btb-topic-label {
  font-size: 0.8rem;
  font-weight: 600;
  color: #e2e8f0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.btb-topic-meta {
  font-size: 0.65rem;
  color: rgba(148, 163, 184, 0.8);
}
.btb-topic-ops {
  display: flex;
  gap: 0.25rem;
  flex-shrink: 0;
}
.btb-topic-op {
  padding: 0.2rem 0.45rem;
  font-size: 0.65rem;
  border: 1px solid rgba(148, 163, 184, 0.3);
  border-radius: 4px;
  background: transparent;
  color: rgba(203, 213, 225, 0.85);
  cursor: pointer;
  transition: all 0.12s ease;
}
.btb-topic-op:hover:not(:disabled) {
  border-color: #60a5fa;
  color: #93c5fd;
}
.btb-topic-op:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.btb-topic-op--confirm:not(:disabled) {
  border-color: rgba(52, 211, 153, 0.6);
  color: #6ee7b7;
}

/* §视图切换 + 泳道 */
.btb-view-toggle {
  display: inline-flex;
  gap: 2px;
  padding: 2px;
  border: 1px solid rgba(148, 163, 184, 0.25);
  border-radius: 5px;
  background: rgba(15, 23, 42, 0.4);
}
.btb-view-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.2rem 0.5rem;
  font-size: 0.65rem;
  color: rgba(203, 213, 225, 0.65);
  background: transparent;
  border: none;
  border-radius: 3px;
  cursor: pointer;
  transition: all 0.12s ease;
}
.btb-view-btn:hover { color: #e2e8f0; }
.btb-view-btn.active {
  background: rgba(96, 165, 250, 0.18);
  color: #93c5fd;
}
.btb-lane-label {
  font-size: calc(10px * var(--fs, 1));
  font-weight: 600;
}

/* Loading */
.btb-loading {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.btb-skeleton {
  height: 36px;
  border-radius: 6px;
  background: var(--color-bg-hover);
  animation: btbPulse 1.5s ease-in-out infinite;
}

@keyframes btbPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

/* Empty */
.btb-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.4rem;
  padding: 2.5rem 0;
  color: var(--color-text-muted);
  font-size: 0.8rem;
}

/* Chart */
.btb-chart {
  display: flex;
  flex-direction: column;
  position: relative;
}

.btb-date-label {
  font-size: calc(0.6rem * var(--fs, 1));
  fill: var(--color-text-muted);
  pointer-events: none;
}

/* SVG scroll container（缩放后两向可滚动） */
.btb-svg-scroll {
  overflow: auto;
  position: relative;
  cursor: grab;
  user-select: none;
  /* 占满工作台主体高度：接近 content 高度；泳道多时纵向滚动 */
  height: calc(100vh - 200px);
  min-height: 320px;
  max-height: none;
}
.btb-svg-scroll--dragging { cursor: grabbing; }

.btb-svg-zoom {
  position: relative;
}

.btb-svg {
  display: block;
  transform-origin: 0 0;
}

/* 缩放控制条（右下角浮动，主题适配） */
.btb-zoom-bar {
  position: absolute;
  bottom: 0.4rem;
  right: 0.5rem;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 2px;
  background: var(--color-bg-overlay);
  border: 1px solid var(--color-border-medium);
  border-radius: 5px;
  z-index: 2;
}
.btb-zoom-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  padding: 0;
  color: var(--color-text-secondary);
  background: transparent;
  border: none;
  border-radius: 3px;
  cursor: pointer;
  transition: background 0.12s ease, color 0.12s ease;
}
.btb-zoom-btn:hover:not(:disabled) {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
}
.btb-zoom-btn:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}
.btb-zoom-label {
  min-width: 42px;
  padding: 0 0.3rem;
  font-size: 0.65rem;
  font-variant-numeric: tabular-nums;
  color: var(--color-text-secondary);
  background: transparent;
  border: none;
  border-radius: 3px;
  cursor: pointer;
}
.btb-zoom-label:hover:not(:disabled) {
  color: var(--color-text-primary);
}
.btb-zoom-label:disabled {
  cursor: default;
  color: var(--color-text-muted);
}

/* Node label */
.btb-node-label {
  font-size: calc(10px * var(--fs, 1));
  font-weight: 500;
  pointer-events: none;
  opacity: 0.82;
  paint-order: stroke;
  stroke: var(--color-bg-hover);
  stroke-width: 3px;
  stroke-linejoin: round;
  transition: opacity 0.12s ease;
}

@media (max-width: 900px) {
  .btb-controls {
    align-items: flex-start;
  }
  .btb-controls-actions {
    gap: 0.35rem;
  }
}

/* DAG nodes */
.btb-dag-node {
  cursor: pointer;
  transition: opacity 0.15s ease;
}

.btb-dag-node circle {
  transition: r 0.1s ease, filter 0.1s ease, stroke-width 0.1s ease;
}

.btb-dag-node:hover circle {
  filter: drop-shadow(0 0 5px rgba(255, 255, 255, 0.3));
  r: 9;
}

.btb-dag-node:hover .btb-node-label {
  opacity: 1;
}

.btb-dag-node--lineage circle {
  filter: drop-shadow(0 0 8px rgba(255, 255, 255, 0.45));
  stroke: rgba(255, 255, 255, 0.5) !important;
  stroke-width: 2 !important;
  r: 9;
}

.btb-dag-node--lineage .btb-node-label {
  opacity: 1;
}

.btb-dag-node--dimmed {
  opacity: 0.15;
  filter: saturate(0.3) brightness(0.6);
}

.btb-dag-node--dimmed .btb-node-label {
  opacity: 0.3;
}

.btb-dag-node--ending {
  opacity: 0.45;
}

.btb-dag-node--ending circle {
  stroke: #9ca3af;
}

.btb-dag-node--ending.btb-dag-node--lineage {
  opacity: 0.8;
}

.btb-dag-node--selected circle {
  filter: drop-shadow(0 0 6px rgba(255, 255, 255, 0.35));
  stroke: rgba(255, 255, 255, 0.5);
}

/* manual confidence 节点：双环描边（task 3.9），独立于算法三态（实心/虚线/空心）。
   hover 显示"人工归属"由 <title> 提供；跟随双主题用语义 token。 */
.btb-dag-node--manual .btb-manual-ring {
  fill: var(--color-bg-base);
  stroke: var(--color-accent);
  stroke-width: 2.5;
}
.btb-dag-node--manual .btb-manual-core {
  fill: var(--color-accent);
  stroke: none;
}

/* Popup overlay */
.btb-popup-overlay {
  position: fixed;
  inset: 0;
  z-index: 200;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--color-bg-overlay);
}

.btb-popup {
  background: var(--color-bg-elevated);
  /* Teleport 到 body 的 popup 不在页面容器继承链内，body 无主题文字色
     （UA 默认黑），dark 下会黑底黑字——必须在容器上显式接主题文字色。 */
  color: var(--color-text-primary);
  border: 1px solid var(--color-border-medium);
  border-radius: 10px;
  padding: 1rem 1.2rem;
  max-width: 420px;
  width: 90vw;
  max-height: 80vh;
  overflow-y: auto;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
}

.btb-popup-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.5rem;
}

.btb-popup-status {
  display: inline-block;
  font-size: 0.6rem;
  font-weight: 500;
  padding: 0.12rem 0.4rem;
  border-radius: 3px;
  /* 状态胶囊文字对比对象是 status 填充色而非页面背景：
     editorial 填充偏深→inverted 为近白；dark 填充提亮→inverted 为深色，双主题可读。 */
  color: var(--color-text-inverted);
}

.btb-popup-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: color 0.12s ease;
}

.btb-popup-close:hover {
  color: var(--color-text-secondary);
}

.btb-popup-title {
  font-size: 0.9rem;
  font-weight: 600;
  color: var(--color-text-primary);
  line-height: 1.4;
  margin-bottom: 0.3rem;
}

.btb-popup-meta {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.65rem;
  color: var(--color-text-muted);
}

/* Threads section */
.btb-threads {
  margin-top: 0.7rem;
  border-top: 1px solid var(--color-border-subtle);
  padding-top: 0.6rem;
}

.btb-threads-loading,
.btb-threads-empty {
  font-size: 0.7rem;
  color: var(--color-text-muted);
  text-align: center;
  padding: 0.5rem 0;
}

.btb-thread-skeleton {
  height: 28px;
  border-radius: 4px;
  background: var(--color-bg-hover);
  margin-bottom: 0.3rem;
  animation: btbPulse 1.5s ease-in-out infinite;
}

.btb-thread {
  margin-bottom: 0.35rem;
  border-radius: 6px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-hover);
  overflow: hidden;
}

.btb-thread--expanded {
  border-color: var(--color-border-medium);
}

.btb-thread-header {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.4rem 0.5rem;
  cursor: pointer;
  transition: background 0.1s ease;
}

.btb-thread-header:hover {
  background: var(--color-bg-hover);
}

.btb-thread-arrow {
  color: var(--color-text-muted);
  transition: transform 0.15s ease;
  flex-shrink: 0;
}

.btb-thread--expanded .btb-thread-arrow {
  transform: rotate(90deg);
}

.btb-thread-title {
  flex: 1;
  font-size: 0.72rem;
  color: var(--color-text-secondary);
  line-height: 1.3;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.btb-thread-count {
  font-size: 0.6rem;
  color: var(--color-text-muted);
  flex-shrink: 0;
}

.btb-thread-summary {
  font-size: 0.65rem;
  color: var(--color-text-muted);
  line-height: 1.4;
  padding: 0 0.5rem 0.3rem 1.4rem;
}

/* Articles */
.btb-articles {
  border-top: 1px solid var(--color-border-subtle);
  padding: 0.3rem 0.5rem 0.4rem;
}

.btb-articles-loading {
  font-size: 0.65rem;
  color: var(--color-text-muted);
  padding: 0.2rem 0;
}

.btb-article {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.25rem 0.3rem;
  border-radius: 3px;
  cursor: pointer;
  transition: background 0.1s ease;
}

.btb-article:hover {
  background: var(--color-bg-hover);
}

.btb-article:hover .btb-article-title {
  color: var(--color-text-primary);
}

.btb-article-icon {
  color: var(--color-text-muted);
  flex-shrink: 0;
}

.btb-article-title {
  flex: 1;
  font-size: 0.65rem;
  color: var(--color-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.btb-article:hover .btb-article-title {
  color: var(--color-text-primary);
}

.btb-article-external {
  color: var(--color-text-muted);
  flex-shrink: 0;
}

.btb-articles-more {
  font-size: 0.6rem;
  color: var(--color-text-muted);
  padding: 0.15rem 0.3rem;
}

/* Slide animation for articles */
.btb-slide-enter-active {
  transition: max-height 150ms ease-out, opacity 150ms ease-out;
}
.btb-slide-leave-active {
  transition: max-height 100ms ease-in, opacity 100ms ease-in;
}
.btb-slide-enter-from,
.btb-slide-leave-to {
  max-height: 0;
  opacity: 0;
  overflow: hidden;
}

/* Popup animation */
.btb-popup-enter-active {
  transition: opacity 150ms ease-out;
}
.btb-popup-enter-active .btb-popup {
  transition: opacity 200ms ease-out, transform 200ms ease-out;
}
.btb-popup-leave-active {
  transition: opacity 120ms ease-in;
}
.btb-popup-leave-active .btb-popup {
  transition: opacity 120ms ease-in, transform 120ms ease-in;
}
.btb-popup-enter-from {
  opacity: 0;
}
.btb-popup-enter-from .btb-popup {
  opacity: 0;
  transform: scale(0.95) translateY(4px);
}
.btb-popup-leave-to {
  opacity: 0;
}
.btb-popup-leave-to .btb-popup {
  opacity: 0;
  transform: scale(0.95) translateY(4px);
}

/* ===== Focus view (slice C′) — single-topic deep read — all semantic tokens ===== */
.btb-focus {
  display: flex;
  flex-direction: column;
  gap: 0.8rem;
  padding: 0.5rem 0 1rem;
}
.btb-focus-head {
  position: sticky;
  top: 0;
  z-index: 5;
  background: var(--color-bg-base);
  border-bottom: 1px solid var(--color-border-medium);
  padding: 0.8rem 1rem;
}
.btb-focus-head__top {
  display: flex;
  align-items: center;
  gap: 0.6rem;
}
.btb-focus-back {
  appearance: none;
  border: 1px solid var(--color-border-medium);
  background: var(--color-bg-elevated);
  color: var(--color-text-secondary);
  padding: 0.32rem 0.7rem;
  border-radius: 6px;
  font-size: 0.74rem;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  font-family: inherit;
}
.btb-focus-back:hover {
  border-color: var(--color-accent);
  color: var(--color-accent);
}
.btb-focus-status {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.66rem;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--color-accent);
  padding: 0.18rem 0.5rem;
  border: 1px solid var(--color-accent);
  border-radius: 10px;
  background: var(--color-accent-subtle);
}
.btb-focus-status__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--color-accent);
}
.btb-focus-title {
  margin: 0.7rem 0 0;
  font-size: 1.6rem;
  font-weight: 800;
  line-height: 1.2;
  color: var(--color-text-primary);
}
.btb-focus-meta {
  margin-top: 0.4rem;
  display: flex;
  flex-wrap: wrap;
  gap: 1.1rem;
  color: var(--color-text-muted);
  font-size: 0.72rem;
}
.btb-focus-meta b {
  color: var(--color-text-secondary);
  font-size: 0.82rem;
}
.btb-focus-timeline-wrap {
  position: relative;
  overflow-x: auto;
  overflow-y: hidden;
  cursor: grab;
  user-select: none;
  border-bottom: 1px solid var(--color-border-subtle);
  background: var(--color-bg-hover);
}
.btb-focus-timeline-wrap--dragging {
  cursor: grabbing;
}
.btb-focus-timeline {
  display: flex;
  align-items: flex-start;
  padding: 0.8rem 1rem;
}
.btb-focus-col {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 96px;
  flex: 0 0 96px;
  position: relative;
}
.btb-focus-col__date {
  font-size: 0.72rem;
  color: var(--color-text-muted);
  padding-bottom: 0.7rem;
}
.btb-focus-col__date b {
  display: block;
  color: var(--color-text-secondary);
  font-size: 0.95rem;
}
/* 话题延续主线（竖向，穿过节点）*/
.btb-focus-col::after {
  content: '';
  position: absolute;
  left: 50%;
  top: 3.4rem;
  bottom: 0.4rem;
  width: 2px;
  transform: translateX(-50%);
  background: linear-gradient(180deg, var(--color-accent), transparent);
  opacity: 0.45;
}
.btb-focus-node {
  position: relative;
  z-index: 1;
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: var(--color-bg-base);
  border: 3px solid var(--color-accent);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: transform 0.15s, background 0.15s;
}
.btb-focus-node:hover {
  transform: scale(1.08);
}
.btb-focus-node__inner {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--color-accent);
  opacity: 0.85;
}
.btb-focus-node--selected {
  background: var(--color-accent);
  box-shadow: 0 0 0 4px var(--color-accent-subtle);
}
.btb-focus-node--selected .btb-focus-node__inner {
  background: var(--color-bg-base);
  opacity: 1;
}
.btb-focus-node-label {
  margin-top: 0.5rem;
  font-size: 0.72rem;
  line-height: 1.35;
  color: var(--color-text-secondary);
  max-width: 92px;
  text-align: center;
}
.btb-focus-node--selected ~ .btb-focus-node-label {
  color: var(--color-accent);
  font-weight: 700;
}
.btb-focus-inline {
  padding: 0.8rem 1rem;
  background: var(--color-bg-elevated);
  color: var(--color-text-primary);
  border: 1px solid var(--color-border-subtle);
  border-radius: 10px;
}
.btb-focus-inline__head {
  display: flex;
  align-items: baseline;
  gap: 0.8rem;
  padding-bottom: 0.6rem;
  border-bottom: 1px solid var(--color-border-subtle);
  margin-bottom: 0.6rem;
}
.btb-focus-inline__date {
  color: var(--color-text-muted);
  font-size: 0.74rem;
}
.btb-focus-inline__title {
  color: var(--color-text-primary);
  font-weight: 700;
}
.btb-focus-inline__meta {
  margin-left: auto;
  color: var(--color-text-muted);
  font-size: 0.72rem;
}
.btb-focus-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.6rem;
  padding: 2rem 1rem;
  color: var(--color-text-muted);
  text-align: center;
}
.btb-focus-empty p {
  margin: 0;
}
.btb-lane-row--enterable {
  cursor: pointer;
}

/* ===== 工作台工具条按钮（回刷 / 合并 / 新建，全语义 token）===== */
.btb-tool-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.28rem 0.6rem;
  font-size: 0.7rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  background: var(--color-bg-elevated);
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: background 0.12s ease, color 0.12s ease, border-color 0.12s ease;
}
.btb-tool-btn:hover:not(:disabled) {
  border-color: var(--color-border-strong);
  color: var(--color-text-primary);
  background: var(--color-bg-hover);
}
.btb-tool-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.btb-tool-btn--primary {
  background: var(--color-accent);
  border-color: var(--color-accent);
  color: var(--color-text-inverted);
  font-weight: 600;
}
.btb-tool-btn--primary:hover:not(:disabled) {
  background: var(--color-accent-hover);
  border-color: var(--color-accent-hover);
  color: var(--color-text-inverted);
}

/* 操作错误条 */
.btb-ops-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.4rem 0.7rem;
  font-size: 0.72rem;
  color: var(--color-error);
  background: var(--color-bg-active);
  border: 1px solid var(--color-error);
  border-radius: 6px;
}
.btb-ops-error-close {
  margin-left: auto;
  border: 0;
  background: transparent;
  color: inherit;
  cursor: pointer;
  font-size: 0.8rem;
}

/* 泳道 hover 操作菜单（SVG 内，hover 显现；默认隐藏不占交互） */
.btb-lane-ops {
  opacity: 0;
  transition: opacity 0.12s ease;
  pointer-events: none;
}
.btb-lane-row:hover .btb-lane-ops {
  opacity: 1;
  pointer-events: auto;
}
.btb-lane-op {
  cursor: pointer;
}
.btb-lane-op-hit {
  fill: transparent;
}
.btb-lane-op:hover .btb-lane-op-hit {
  fill: var(--color-bg-hover);
}
.btb-lane-op-ico {
  fill: none;
  stroke: var(--color-text-muted);
  stroke-width: 1.4;
  stroke-linecap: round;
  stroke-linejoin: round;
  pointer-events: none;
}
.btb-lane-op:hover .btb-lane-op-ico {
  stroke: var(--color-text-primary);
}
.btb-lane-op--danger:hover .btb-lane-op-ico {
  stroke: var(--color-error);
}

/* 弹窗内表单/确认样式（复用 TopicManageDialog 视觉约定，全语义 token） */
.btb-field {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}
.btb-field-label {
  font-size: 0.78rem;
  font-weight: 600;
  color: var(--color-text-secondary);
}
.btb-confirm-body {
  font-size: 0.84rem;
  color: var(--color-text-primary);
  line-height: 1.5;
  margin: 0;
}
.btb-danger-banner {
  display: flex;
  gap: 0.5rem;
  padding: 0.55rem 0.7rem;
  background: var(--color-bg-active);
  border: 1px solid var(--color-error);
  border-radius: 8px;
  color: var(--color-error);
  font-size: 0.76rem;
  line-height: 1.5;
}

/* 合并预览双栏 */
.btb-merge-cols {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.75rem;
  margin-top: 0.5rem;
}
.btb-merge-col {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  max-height: 40vh;
  overflow-y: auto;
}
.btb-merge-col__t {
  font-size: 0.72rem;
  font-weight: 600;
  color: var(--color-text-secondary);
  padding-bottom: 0.25rem;
}
.btb-merge-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.4rem 0.55rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 6px;
  background: transparent;
  cursor: pointer;
  text-align: left;
}
.btb-merge-item:hover {
  background: var(--color-bg-hover);
}
.btb-merge-item--sel {
  border-color: var(--color-accent);
  background: var(--color-accent-subtle);
}
.btb-merge-dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  flex-shrink: 0;
}
.btb-merge-label {
  flex: 1;
  font-size: 0.78rem;
  color: var(--color-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.btb-merge-count {
  font-size: 0.66rem;
  color: var(--color-text-muted);
}
.btb-merge-empty {
  padding: 0.75rem;
  text-align: center;
  font-size: 0.74rem;
  color: var(--color-text-muted);
}

/* ===== 编排态浮层（顶部工具条 + 右侧候选侧边栏，position:absolute 浮于 lanes SVG 之上）===== */
.btb-compose-toolbar-anchor {
  position: absolute;
  top: 10px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 30;
  max-width: calc(100% - 340px);
}
.btb-compose-sidebar-anchor {
  position: absolute;
  top: 78px;
  right: 12px;
  width: 300px;
  max-height: calc(100% - 96px);
  overflow: auto;
  z-index: 30;
  display: flex;
  flex-direction: column;
  align-items: flex-end;   /* 展开态：toggle 贴右 */
}
.btb-compose-sidebar-anchor.is-collapsed {
  width: 40px;
  overflow: visible;
  align-items: center;     /* 折叠态：toggle 居中，窄条整体可见 */
}
.btb-compose-sidebar-toggle {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  margin: 6px;
  border: 1px solid var(--color-border-medium);
  border-radius: 999px;
  background: var(--color-bg-elevated);
  color: var(--color-text-secondary);
  cursor: pointer;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.15);
}
.btb-compose-sidebar-toggle:hover {
  color: var(--color-text-primary);
  border-color: var(--color-accent);
}

/* 编排态：active 泳道淑显保留为背景参照（unassigned 不淑显，主战场） */
.btb-lane-row--dimmed {
  opacity: 0.3;
  transition: opacity 0.2s ease;
}

/* 编排态节点：勾选/离群状态 */
.btb-dag-node--compose-outlier circle:not(.btb-manual-ring):not(.btb-manual-core) {
  stroke: var(--color-warning);
}

/* 编排态：就地 checkbox（节点左上角） */
.btb-compose-check { cursor: pointer; }
.btb-compose-check__box {
  fill: var(--color-bg-elevated);
  stroke: var(--color-accent);
  stroke-width: 1.4;
}
.btb-compose-check--on .btb-compose-check__box {
  fill: var(--color-accent);
}
.btb-compose-check__tick {
  fill: none;
  stroke: var(--color-text-inverted);
  stroke-width: 1.6;
  stroke-linecap: round;
  stroke-linejoin: round;
}

/* 编排态：distance/tier 标徽（节点右侧） */
.btb-compose-badge { pointer-events: none; }
.btb-compose-badge__tier {
  font-size: 8px;
  font-weight: 600;
  dominant-baseline: middle;
}
.btb-compose-badge__dist {
  font-size: 7.5px;
  font-family: ui-monospace, monospace;
  fill: var(--color-text-muted);
  dominant-baseline: middle;
}
.btb-compose-badge--good .btb-compose-badge__tier { fill: var(--color-success); }
.btb-compose-badge--boundary .btb-compose-badge__tier { fill: var(--color-accent); }
.btb-compose-badge--outlier .btb-compose-badge__tier,
.btb-compose-badge--far .btb-compose-badge__tier { fill: var(--color-warning); }

/* 编排态：勾走 active section 的移出提示（节点下方） */
.btb-compose-moveout {
  font-size: 8px;
  fill: var(--color-warning);
  pointer-events: none;
}

/* unassigned 泳道头部「新建泳道」入口 */
.btb-lane-compose-entry { cursor: pointer; }
.btb-lane-compose-entry__hit {
  fill: var(--color-accent-subtle);
  stroke: var(--color-accent);
  stroke-width: 0.8;
}
.btb-lane-compose-entry:hover .btb-lane-compose-entry__hit {
  fill: var(--color-accent);
}
.btb-lane-compose-entry__text {
  font-size: 9px;
  font-weight: 600;
  fill: var(--color-accent);
  pointer-events: none;
}
.btb-lane-compose-entry:hover .btb-lane-compose-entry__text {
  fill: var(--color-text-inverted);
}

/* 移出二次确认对话框内容 */
.btb-moveout-body {
  margin: 0 0 0.6rem;
  font-size: 0.84rem;
  color: var(--color-text-primary);
  line-height: 1.5;
}
.btb-moveout-body b { color: var(--color-accent); }
.btb-moveout-list {
  margin: 0;
  padding-left: 1.2rem;
  font-size: 0.78rem;
  color: var(--color-text-secondary);
  line-height: 1.7;
}

/* ── 关键事件注记（add-topic-key-event-markers）──────────────────────────── */
.btb-ke-toggle {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 10px;
  border-radius: 999px;
  border: 1px solid var(--color-border-medium, rgba(127, 127, 127, 0.35));
  background: transparent;
  color: inherit;
  font-size: 0.72rem;
  cursor: pointer;
  white-space: nowrap;
}

.btb-ke-toggle[aria-pressed='true'] {
  background: var(--color-accent, #d94a4a);
  border-color: var(--color-accent, #d94a4a);
  /* accent 填充在 dark 下提亮（橙），近白字对比不够——用主题反色（editorial 近白 / dark 深色）。 */
  color: var(--color-text-inverted);
}

/* 突出模式：普通节点仅弱化透明度（不移除、不改坐标）；快照/关键节点保持。 */
.btb-focus-col--dimmed {
  opacity: 0.32;
  transition: opacity 0.15s ease;
}

.btb-focus-col--dimmed .btb-focus-node,
.btb-focus-col--dimmed .btb-focus-node-label {
  cursor: pointer; /* 普通点仍可点（背景可达） */
}

.btb-focus-node {
  position: relative;
}

.btb-focus-node__ke {
  position: absolute;
  top: -10px;
  right: -8px;
  z-index: 2;
  min-width: 20px;
  padding: 0 4px;
  border-radius: 999px;
  background: var(--color-accent, #d94a4a);
  color: var(--color-text-inverted);
  font-size: 0.66rem;
  font-weight: 700;
  line-height: 14px;
  text-align: center;
  pointer-events: none;
}

.btb-focus-node--snapshot {
  outline: 2px dashed var(--color-accent, #d94a4a);
  outline-offset: 2px;
}

.btb-focus-node--snapshot .btb-focus-node__inner {
  opacity: 0.55;
}

.btb-focus-node-label--snapshot {
  font-style: italic;
  opacity: 0.85;
}

.btb-node-ke-badge {
  fill: var(--color-accent, #d94a4a);
  font-size: 10px;
  font-weight: 700;
  pointer-events: none;
}

.btb-ke-detail {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px 10px;
  margin: 8px 0;
  border: 1px solid var(--color-border-medium, rgba(127, 127, 127, 0.35));
  border-radius: 10px;
  background: var(--color-bg-elevated, rgba(127, 127, 127, 0.06));
  /* 容器接主题文字色：focus inline 与 Teleport popup 两条挂载链都不依赖外层继承。 */
  color: var(--color-text-primary);
}

.btb-ke-detail__row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  min-width: 0;
}

/* 原型 .event-title 形态：标题列（serif 加粗可换行）+ 摘要，动作列独立在右，不挤标题。 */
.btb-ke-detail__main {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.btb-ke-detail__title {
  font-family: "Noto Serif SC", serif;
  font-weight: 700;
  font-size: 0.84rem;
  line-height: 1.5;
  overflow-wrap: anywhere;
}

.btb-ke-detail__summary {
  margin: 0;
  color: var(--color-text-muted, #9a8f81);
  font-size: 0.75rem;
  line-height: 1.55;
  overflow-wrap: anywhere;
}

.btb-ke-detail__actions {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 6px;
  padding-top: 1px;
}

.btb-ke-detail__link,
.btb-ke-detail__unmark {
  flex: 0 0 auto;
  padding: 2px 8px;
  border-radius: 7px;
  border: 1px solid var(--color-border-medium, rgba(127, 127, 127, 0.35));
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 0.72rem;
  cursor: pointer;
  white-space: nowrap;
}

.btb-ke-detail__link:hover {
  color: var(--color-text-primary);
  border-color: var(--color-border-strong, rgba(127, 127, 127, 0.5));
}

.btb-ke-detail__unmark {
  border-color: var(--color-accent, #d94a4a);
  color: var(--color-accent, #d94a4a);
}

.btb-ke-detail__unmark:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btb-ke-detail__error {
  flex: 1 0 100%;
  color: var(--color-accent, #d94a4a);
  font-size: 0.72rem;
  overflow-wrap: anywhere;
}

.btb-ke-empty-hint {
  margin: 4px 0 0;
  color: var(--color-text-muted, #9a8f81);
  font-size: 0.74rem;
}

.btb-ke-detail__hint {
  margin: 2px 0 0;
  color: var(--color-text-muted, #9a8f81);
  font-size: 0.72rem;
}
</style>
