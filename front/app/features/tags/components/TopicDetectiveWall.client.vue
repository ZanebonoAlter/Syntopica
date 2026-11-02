<script setup lang="ts">
/**
 * TopicDetectiveWall — full-screen 3D detective photo wall container.
 *
 * Owns the canvas + CSS2D overlay DOM, initializes the Three.js scene stack
 * (TopicWallScene / DirectorCamera / InteractionLayer), fetches board data, and
 * renders the 2D overlays (board selector, days range, detail panel, chapter
 * transition DOM). 2D overlays use motion-v; 3D animation uses gsap.
 *
 * @see design.md §Architecture Overview
 */
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useDailyReportsApi } from '~/api/dailyReports'
import type { SectionTimelineNode, SectionRelation, DailyReportThread } from '~/api/dailyReports'
import { useArticlesApi } from '~/api/articles'
import { TopicWallScene } from './detective-wall/TopicWallScene'
import { DirectorCamera } from './detective-wall/DirectorCamera'
import { InteractionLayer } from './detective-wall/InteractionLayer'
import { WallCameraControls } from './detective-wall/WallCameraControls'
import { SUPPORTED_DAYS } from './detective-wall/types'
import { latestDayX, LANES_PER_BATCH } from './detective-wall/utils'

type WallSection = 'timeline' | 'lifeline' | 'lifecycle'

const props = defineProps<{
  boardId: number
  initialTopicId?: number
}>()
const emit = defineEmits<{
  close: []
  openArticle: [articleId: number]
}>()

const { getBoardSectionTimeline, getDailyReportDetail, getSectionLifecycle, getTopicLifeline } = useDailyReportsApi()
const { getArticle } = useArticlesApi()

const canvasRef = ref<HTMLCanvasElement | null>(null)
const css2dContainerRef = ref<HTMLDivElement | null>(null)

const loading = ref(true)
const error = ref(false)
const days = ref<7 | 14 | 30 | 60>(7)
const sections = ref<SectionTimelineNode[]>([])
const relations = ref<SectionRelation[]>([])

// Detail panel state (plain Vue overlay, not CSS2D).
const showDetailPanel = ref(false)
const focusedNode = ref<SectionTimelineNode | null>(null)
const lifelineNodes = ref<SectionTimelineNode[]>([])
const lifelineRelations = ref<SectionRelation[]>([])
// Per-lifeline-node thread/article expansion (spec §面板操作 "查看详细线索").
const expandedNodeId = ref<number | null>(null)
const nodeThreads = ref<Map<number, DailyReportThread[]>>(new Map())
const nodeThreadsLoading = ref<number | null>(null)
// Full lifecycle mode (spec §面板操作 查看完整生命周期).
const lifecycleActive = ref(false)
const lifecycleLoading = ref(false)
const lifecycleOriginNode = ref<SectionTimelineNode | null>(null)
// 视图模式：timeline=仅匈牙利相似度（默认，隐藏 identity）；lanes=仅话题 identity
// （同话题延续实线）。镜像旧版 2D 话题浏览器的语义。
const viewMode = ref<'timeline' | 'lanes'>('timeline')

// 话题泳道分批：话题超过 LANES_PER_BATCH 时按批翻页，每批从桌面上方往上排，
// 避免赛道堆太高把卡片压到桌面以下。laneBatch 是当前批次索引（0 起）。
const laneBatch = ref(0)

// 当前数据的话题总数（含未分类赛道），用于计算批次数与翻页边界。
const laneCount = computed(() => {
  const keys = new Set<string>()
  for (const s of sections.value) {
    keys.add(s.persistent_topic ? `t${s.persistent_topic.id}` : 'unassigned')
  }
  return keys.size
})
const laneBatchCount = computed(() => Math.max(1, Math.ceil(laneCount.value / LANES_PER_BATCH)))
// 当前批展示的话题标签（用于 UI 提示），按出现顺序取本批区间。
const laneBatchLabels = computed(() => {
  const order: string[] = []
  const seen = new Set<string>()
  for (const s of sections.value) {
    const k = s.persistent_topic ? `t${s.persistent_topic.id}` : 'unassigned'
    if (!seen.has(k)) {
      seen.add(k)
      order.push(s.persistent_topic?.label ?? '未分类')
    }
  }
  const start = laneBatch.value * LANES_PER_BATCH
  return order.slice(start, start + LANES_PER_BATCH)
})

// --- WebGL detection ---
function hasWebGL(): boolean {
  try {
    const c = document.createElement('canvas')
    return !!(c.getContext('webgl2') || c.getContext('webgl'))
  } catch {
    return false
  }
}
const webglOk = hasWebGL()

let scene: TopicWallScene | null = null
let directorCamera: DirectorCamera | null = null
let interaction: InteractionLayer | null = null
let cameraControls: WallCameraControls | null = null
let resizeObserver: ResizeObserver | null = null

onMounted(async () => {
  if (!webglOk || !canvasRef.value || !css2dContainerRef.value) {
    error.value = true
    return
  }

  scene = new TopicWallScene(canvasRef.value, css2dContainerRef.value)
  directorCamera = new DirectorCamera(scene.camera)
  interaction = new InteractionLayer(scene, directorCamera, canvasRef.value, {
    onCardHover: () => { /* tooltip handled by CSS2D in scene */ },
    onCardClick: (card) => {
      focusedNode.value = card.data
    },
    onStringClick: () => { /* handled internally via re-BFS */ },
    onBackgroundClick: () => {
      if (lifecycleActive.value) {
        // Lifecycle mode: background click exits back to the timeline.
        exitLifecycle()
        return
      }
      showDetailPanel.value = false
      focusedNode.value = null
      lifelineNodes.value = []
      lifelineRelations.value = []
    },
    onLifelineReady: (nodes, edges, start) => {
      lifelineNodes.value = nodes
      lifelineRelations.value = edges
      focusedNode.value = start
      showDetailPanel.value = true
    },
  })
  interaction.enable()

  // Orbit-style pan + zoom (2.5D, rotation disabled). Coordinates with
  // DirectorCamera via hooks (disable during transitions, sync target).
  cameraControls = new WallCameraControls(
    scene.camera,
    canvasRef.value,
    directorCamera,
    {
      onInteractStart: () => interaction?.setHoverSuspended(true),
      onInteractEnd: () => interaction?.setHoverSuspended(false),
    },
  )
  scene.addFrameCallback(() => cameraControls?.update())

  scene.startRenderLoop()

  resizeObserver = new ResizeObserver(() => {
    if (!scene || !canvasRef.value) return
    scene.onResize(canvasRef.value.clientWidth, canvasRef.value.clientHeight)
  })
  resizeObserver.observe(canvasRef.value)

  // ESC: close detail panel (or exit lifecycle) first, then exit the wall.
  window.addEventListener('keydown', onKeyDown)

  await loadBoardData()
  if (props.initialTopicId != null) {
    await enterTopicLifeline(props.initialTopicId)
  }
})

function onKeyDown(e: KeyboardEvent) {
  if (e.key !== 'Escape') return
  if (lifecycleActive.value) {
    exitLifecycle()
  } else if (showDetailPanel.value) {
    closePanel()
  } else {
    close()
  }
}

async function loadBoardData() {
  if (!scene || !interaction) return
  loading.value = true
  error.value = false
  try {
    const res = await getBoardSectionTimeline(props.boardId, days.value)
    if (res.success && res.data) {
      sections.value = res.data.sections
      relations.value = res.data.relations
      const dates = res.data.sections.map(s => s.period_date.slice(0, 10)).sort()
      const end = dates[dates.length - 1] ?? new Date().toISOString().slice(0, 10)
      const endDate = new Date(end)
      endDate.setDate(endDate.getDate() - (days.value - 1))
      const start = endDate.toISOString().slice(0, 10)
      const dateRange = { start, end }
      scene.loadBoardData(res.data.sections, res.data.relations, dateRange, days.value, viewMode.value, laneBatch.value)
      applyEdgeFilter()
      cameraControls?.setBounds(scene.getCameraBounds())
      interaction.setData(res.data.sections, res.data.relations, dateRange, days.value)
      // Snap camera to today focus.
      const tx = latestDayX(res.data.sections)
      directorCamera?.snapTo(directorCamera.todayFocus(tx))
    } else {
      error.value = true
    }
  } catch {
    error.value = true
  } finally {
    loading.value = false
  }
}

watch(() => days.value, () => {
  interaction?.setTimeRange(days.value)
  loadBoardData()
})

// 按视图模式显隐边：timeline 隐藏 identity（只留匈牙利），lanes 隐藏 similarity
// （只留话题连续性）。通过 line.visible 开关，零重建开销。生命周期模式重建场景后
// 也调用，以保持一致。
function applyEdgeFilter() {
  if (!scene) return
  const showIdentity = viewMode.value === 'lanes'
  scene.redStrings.setVisibleByRelationType('identity', showIdentity)
  scene.redStrings.setVisibleByRelationType('similarity', !showIdentity)
}
function setViewMode(mode: 'timeline' | 'lanes') {
  if (viewMode.value === mode || lifecycleActive.value || !scene || !interaction) return
  viewMode.value = mode
  laneBatch.value = 0 // 切换视图模式时回到首批
  // 话题泳道模式需重布局卡片（按话题分赛道），不能只切边；用当前缓存数据重建场景。
  rebuildFromCache()
}

// 用当前缓存的 sections/relations 重建场景（视图模式/批次切换时复用）。
// 不重新请求数据，避免闪烁。
function rebuildFromCache() {
  if (!scene || !interaction) return
  const dates = sections.value.map(s => s.period_date.slice(0, 10)).sort()
  const end = dates[dates.length - 1] ?? new Date().toISOString().slice(0, 10)
  const endDate = new Date(end)
  endDate.setDate(endDate.getDate() - (days.value - 1))
  const start = endDate.toISOString().slice(0, 10)
  const dateRange = { start, end }
  scene.loadBoardData(sections.value, relations.value, dateRange, days.value, viewMode.value, laneBatch.value)
  cameraControls?.setBounds(scene.getCameraBounds())
  interaction.setData(sections.value, relations.value, dateRange, days.value)
  applyEdgeFilter()
}

// 话题泳道翻页：切批次后重建场景。timeline 模式下不触发。
function setLaneBatch(batch: number) {
  if (viewMode.value !== 'lanes' || lifecycleActive.value) return
  if (batch < 0 || batch >= laneBatchCount.value || batch === laneBatch.value) return
  laneBatch.value = batch
  rebuildFromCache()
}

function setDays(d: 7 | 14 | 30 | 60) {
  if (lifecycleActive.value) return
  days.value = d
}

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeyDown)
  resizeObserver?.disconnect()
  interaction?.dispose()
  cameraControls?.dispose()
  scene?.dispose()
  scene = null
})

function close() {
  emit('close')
}

/** Close just the detail panel (stay in 3D). Mirrors InteractionLayer.resetToOverview. */
function closePanel() {
  interaction?.resetToOverview()
  showDetailPanel.value = false
  focusedNode.value = null
  lifelineNodes.value = []
  lifelineRelations.value = []
}

function openArticle(id: number) {
  emit('openArticle', id)
}

// --- §3.3: lifeline node "查看详细线索" (spec §面板操作) ---
// Clicking a lifeline node expands its threads + articles, fetched via
// getDailyReportDetail(report_id). Section id disambiguates the thread set
// inside that report (SectionTimelineNode carries report_id + section id).
async function toggleNodeThreads(node: SectionTimelineNode) {
  // Collapse if already expanded.
  if (expandedNodeId.value === node.id) {
    expandedNodeId.value = null
    return
  }
  expandedNodeId.value = node.id

  // Cached: a single section may appear on the lifeline once.
  if (nodeThreads.value.has(node.id)) return

  nodeThreadsLoading.value = node.id
  try {
    const res = await getDailyReportDetail(node.report_id)
    if (res.success && res.data) {
      // Match the section inside the report by section id (node.id).
      const section = res.data.report.sections?.find(s => s.id === node.id)
      nodeThreads.value = new Map(nodeThreads.value).set(node.id, section?.threads || [])
    }
  } finally {
    nodeThreadsLoading.value = null
  }
}

// --- §线索下钻:点击 thread 展开其文章列表,再点击具体文章才跳转 ---
// (之前直接取 related_article_ids[0],多篇文章时无法选择。)
const expandedThreadId = ref<number | null>(null)
const threadArticles = ref<Map<number, { id: number; title: string }[]>>(new Map())
const threadArticlesLoading = ref<number | null>(null)

/** Toggle a thread's article list open/closed, fetching titles lazily. */
async function toggleThreadArticles(thread: DailyReportThread) {
  if (expandedThreadId.value === thread.id) {
    expandedThreadId.value = null
    return
  }
  expandedThreadId.value = thread.id
  // Already fetched for this thread → keep cached list.
  if (threadArticles.value.has(thread.id)) return

  const ids = thread.related_article_ids ?? []
  if (ids.length === 0) return

  threadArticlesLoading.value = thread.id
  try {
    // Fetch titles for up to 10 articles (与旧版 2D 浏览器同款上限).
    const batch = ids.slice(0, 10)
    const results = await Promise.allSettled(batch.map(id => getArticle(id)))
    const articles = results.map((r, i) => {
      const aid = batch[i]!
      if (r.status === 'fulfilled' && r.value.success && r.value.data) {
        return { id: aid, title: r.value.data.title || '(无标题)' }
      }
      return { id: aid, title: `文章 #${aid}` }
    })
    threadArticles.value = new Map(threadArticles.value).set(thread.id, articles)
  } finally {
    threadArticlesLoading.value = null
  }
}

// --- §完整生命周期 (spec §面板操作 查看完整生命周期) ---
// Status → 中文标签 (panel + tooltip 共用).
const STATUS_LABELS: Record<string, string> = {
  emerging: '新兴',
  continuing: '持续',
  split: '分化',
  merge: '合并',
  ending: '结束',
}
function statusLabel(status: string): string {
  return STATUS_LABELS[status] ?? status
}

// Aggregate stats across the current lifeline/lifecycle node set.
const lifelineSummary = computed(() => {
  const nodes = lifelineNodes.value
  let articles = 0
  let threads = 0
  const statusCounts = new Map<string, number>()
  for (const n of nodes) {
    articles += n.article_count
    threads += n.thread_count
    statusCounts.set(n.status, (statusCounts.get(n.status) ?? 0) + 1)
  }
  return { articles, threads, statusCounts }
})

const activeWallSection = computed<WallSection>(() => {
  if (lifecycleActive.value) return 'lifecycle'
  if (showDetailPanel.value && lifelineNodes.value.length > 0) return 'lifeline'
  return 'timeline'
})

const wallSections: Array<{ key: WallSection; label: string; icon: string }> = [
  { key: 'timeline', label: '主墙', icon: 'mdi:view-dashboard-outline' },
  { key: 'lifeline', label: '生命线', icon: 'mdi:vector-polyline' },
  { key: 'lifecycle', label: '生命周期', icon: 'mdi:timeline-clock-outline' },
]

const orderedLifelineNodes = computed(() => [...lifelineNodes.value].sort(compareNodesByTime))
const previousCaseNodes = computed(() => directionalCaseCandidates('previous'))
const nextCaseNodes = computed(() => directionalCaseCandidates('next'))

function nodeTime(node: SectionTimelineNode): number {
  return new Date(node.period_date).getTime()
}

function compareNodesByTime(a: SectionTimelineNode, b: SectionTimelineNode): number {
  const diff = nodeTime(a) - nodeTime(b)
  return diff !== 0 ? diff : a.id - b.id
}

function isConnectedToFocused(node: SectionTimelineNode, focusedId: number): boolean {
  return lifelineRelations.value.some(r =>
    (r.from_id === focusedId && r.to_id === node.id)
    || (r.to_id === focusedId && r.from_id === node.id),
  )
}

function directionalCaseCandidates(direction: 'previous' | 'next'): SectionTimelineNode[] {
  const focused = focusedNode.value
  if (!focused) return []
  const ordered = orderedLifelineNodes.value
  const currentTime = nodeTime(focused)
  const related = ordered.filter((node) => {
    if (node.id === focused.id || !isConnectedToFocused(node, focused.id)) return false
    return direction === 'next' ? nodeTime(node) > currentTime : nodeTime(node) < currentTime
  })
  if (related.length > 0) {
    const sorted = direction === 'next' ? related : related.reverse()
    const nearestTime = nodeTime(sorted[0]!)
    return sorted.filter(node => nodeTime(node) === nearestTime)
  }

  const currentIndex = ordered.findIndex(node => node.id === focused.id)
  const fallback = ordered[currentIndex + (direction === 'next' ? 1 : -1)]
  return fallback ? [fallback] : []
}

function selectCaseNode(node: SectionTimelineNode) {
  focusedNode.value = node
  showDetailPanel.value = true
  interaction?.focusNode(node.id)
}

function selectAndToggleNodeThreads(node: SectionTimelineNode) {
  selectCaseNode(node)
  toggleNodeThreads(node)
}

async function switchWallSection(section: WallSection) {
  if (section === 'timeline') {
    if (lifecycleActive.value) {
      await exitLifecycle()
    } else {
      closePanel()
    }
    return
  }
  if (section === 'lifeline') {
    if (lifecycleActive.value) {
      const origin = lifecycleOriginNode.value
      await exitLifecycle(origin)
      return
    }
    if (focusedNode.value && lifelineNodes.value.length > 0) {
      showDetailPanel.value = true
      selectCaseNode(focusedNode.value)
    }
    return
  }
  if (focusedNode.value && !lifecycleActive.value) {
    await enterLifecycle(focusedNode.value.id)
  }
}

// Enter: fetch the topic's full evolution (no day limit), rebuild the scene
// with only that line, disable fog, move camera to lifecycleFull.
async function enterLifecycle(sectionId: number) {
  if (!interaction || lifecycleActive.value) return
  lifecycleLoading.value = true
  try {
    const res = await getSectionLifecycle(sectionId)
    if (!res.success || !res.data) return
    const lcSections = res.data.sections
    const lcRelations = res.data.relations
    // Derive a date window spanning the lifecycle's own range.
    const dates = lcSections.map(s => s.period_date.slice(0, 10)).sort()
    const dateRange = {
      start: dates[0] ?? '',
      end: dates[dates.length - 1] ?? '',
    }
    lifecycleOriginNode.value = focusedNode.value ?? sections.value.find(s => s.id === sectionId) ?? null
    interaction.enterLifecycle(lcSections, lcRelations, dateRange)
    cameraControls?.setBounds(scene?.getCameraBounds() ?? null)
    lifecycleActive.value = true
    // Panel reflects the lifecycle node set.
    focusedNode.value = lcSections.find(s => s.id === sectionId) ?? lcSections[0] ?? focusedNode.value
    lifelineNodes.value = lcSections
    lifelineRelations.value = lcRelations
    showDetailPanel.value = true
  } finally {
    lifecycleLoading.value = false
  }
}

// Exit: re-enable fog for the timeline window, reload the timeline data, and
// return the camera to the today-focus shot.
async function exitLifecycle(restoreNode: SectionTimelineNode | null = null) {
  if (!interaction || !lifecycleActive.value) return
  interaction.exitLifecycle()
  lifecycleActive.value = false
  lifecycleOriginNode.value = null
  focusedNode.value = null
  lifelineNodes.value = []
  lifelineRelations.value = []
  showDetailPanel.value = false
  // Reload the timeline (restores cards + fog for the current days window).
  await loadBoardData()
  if (restoreNode) {
    focusedNode.value = sections.value.find(s => s.id === restoreNode.id) ?? restoreNode
    showDetailPanel.value = true
    interaction.focusNode(restoreNode.id)
  }
}

// --- §9.4/9.6: persistent-topic lifeline + management ---

// Topic management lives in <TopicManageDialog>; this just toggles it open
// from the detail panel.
const showTopicManage = ref(false)

// Topic status labels differ from section status (emerging/continuing…).
const TOPIC_STATUS_LABELS: Record<string, string> = {
  candidate: '候选',
  active: '活跃',
  archived: '已归档',
}
function topicStatusLabel(status: string): string {
  return TOPIC_STATUS_LABELS[status] ?? status
}

// §9.4 Topic lifeline: fetch every section of one persistent topic (no day
// limit) and enter lifecycle mode with it. Mirrors enterLifecycle but sources
// data from getTopicLifeline, so the evolution line survives cluster-label drift.
async function enterTopicLifeline(topicId: number) {
  if (!interaction || lifecycleActive.value) return
  lifecycleLoading.value = true
  try {
    const res = await getTopicLifeline(topicId)
    if (!res.success || !res.data) return
    const lcSections = res.data.sections
    const lcRelations = res.data.relations
    const dates = lcSections.map(s => s.period_date.slice(0, 10)).sort()
    const dateRange = {
      start: dates[0] ?? '',
      end: dates[dates.length - 1] ?? '',
    }
    lifecycleOriginNode.value = focusedNode.value
    interaction.enterLifecycle(lcSections, lcRelations, dateRange)
    cameraControls?.setBounds(scene?.getCameraBounds() ?? null)
    lifecycleActive.value = true
    focusedNode.value = lcSections[0] ?? focusedNode.value
    lifelineNodes.value = lcSections
    lifelineRelations.value = lcRelations
    showDetailPanel.value = true
  } finally {
    lifecycleLoading.value = false
  }
}
</script>

<template>
  <div class="tdw-root">
    <canvas ref="canvasRef" class="tdw-canvas" />
    <div ref="css2dContainerRef" class="tdw-css2d" />

    <!-- Top bar: close + days range -->
    <div class="tdw-topbar">
      <button class="tdw-btn" @click="close">
        <Icon icon="mdi:arrow-left" width="18" />
        <span>返回</span>
      </button>
      <div class="tdw-section-switch">
        <button
          v-for="section in wallSections"
          :key="section.key"
          class="tdw-section-btn"
          :class="{ active: activeWallSection === section.key }"
          :disabled="
            (section.key === 'lifeline' && lifelineNodes.length === 0)
              || (section.key === 'lifecycle' && (!focusedNode || lifecycleLoading))
          "
          @click="switchWallSection(section.key)"
        >
          <Icon :icon="section.icon" width="14" />
          <span>{{ section.label }}</span>
        </button>
      </div>
      <div class="tdw-days-toggle">
        <button
          v-for="d in SUPPORTED_DAYS"
          :key="d"
          class="tdw-days-btn"
          :class="{ active: days === d }"
          :disabled="lifecycleActive"
          @click="setDays(d)"
        >
          {{ d }}天
        </button>
      </div>
      <div class="tdw-view-toggle">
        <button
          class="tdw-view-btn"
          :class="{ active: viewMode === 'timeline' }"
          :disabled="lifecycleActive"
          title="仅匈牙利相似度关系（默认）"
          @click="setViewMode('timeline')"
        >
          <Icon icon="mdi:chart-timeline-variant" width="13" />
          <span>时间线</span>
        </button>
        <button
          class="tdw-view-btn"
          :class="{ active: viewMode === 'lanes' }"
          :disabled="lifecycleActive"
          title="仅话题连续性（同话题实线）"
          @click="setViewMode('lanes')"
        >
          <Icon icon="mdi:view-stream-outline" width="13" />
          <span>话题泳道</span>
        </button>
      </div>
    </div>

    <!-- 话题泳道分批翻页：仅 lanes 模式且话题超过单批容量时显示。样式复用
         --color-* 主题变量，自动适配日夜双主题。 -->
    <div
      v-if="viewMode === 'lanes' && !lifecycleActive && laneBatchCount > 1"
      class="tdw-lane-pager"
    >
      <button
        class="tdw-lane-pager-btn"
        :disabled="laneBatch === 0"
        title="上一批话题"
        @click="setLaneBatch(laneBatch - 1)"
      >
        <Icon icon="mdi:chevron-left" width="16" />
      </button>
      <div class="tdw-lane-pager-info">
        <span class="tdw-lane-pager-title">话题批次 {{ laneBatch + 1 }}/{{ laneBatchCount }}</span>
        <span class="tdw-lane-pager-tags">
          {{ laneBatchLabels.join(' · ') }}
        </span>
      </div>
      <button
        class="tdw-lane-pager-btn"
        :disabled="laneBatch >= laneBatchCount - 1"
        title="下一批话题"
        @click="setLaneBatch(laneBatch + 1)"
      >
        <Icon icon="mdi:chevron-right" width="16" />
      </button>
    </div>

    <!-- Detail panel (plain Vue overlay, fixed position) -->
    <Transition name="tdw-panel">
      <div v-if="showDetailPanel && focusedNode" class="tdw-detail-panel">
        <div class="tdw-detail-header">
          <Icon icon="mdi:folder" width="16" />
          <span>案件编号 #{{ focusedNode.id }}</span>
        </div>
        <div class="tdw-case-nav">
          <button
            class="tdw-case-nav-btn"
            :disabled="previousCaseNodes.length === 0"
            @click="previousCaseNodes[0] && selectCaseNode(previousCaseNodes[0])"
          >
            <Icon icon="mdi:chevron-left" width="15" />
            <span>上一个</span>
          </button>
          <div class="tdw-case-next">
            <button
              v-if="nextCaseNodes.length <= 1"
              class="tdw-case-nav-btn"
              :disabled="nextCaseNodes.length === 0"
              @click="nextCaseNodes[0] && selectCaseNode(nextCaseNodes[0])"
            >
              <span>下一个</span>
              <Icon icon="mdi:chevron-right" width="15" />
            </button>
            <div v-else class="tdw-branch-picker">
              <span class="tdw-branch-label">下一个分化</span>
              <button
                v-for="node in nextCaseNodes"
                :key="node.id"
                class="tdw-branch-btn"
                :title="node.cluster_label"
                @click="selectCaseNode(node)"
              >
                #{{ node.id }}
              </button>
            </div>
          </div>
        </div>
        <div class="tdw-detail-title">{{ focusedNode.cluster_label }}</div>
        <div class="tdw-detail-meta">
          <span>{{ focusedNode.article_count }}篇 · {{ focusedNode.thread_count }}线索</span>
          <span class="tdw-detail-status">{{ statusLabel(focusedNode.status) }}</span>
        </div>
        <!-- §9.4/9.6: persistent-topic lifeline + management -->
        <div v-if="focusedNode.persistent_topic" class="tdw-topic-bar">
          <div class="tdw-topic-info">
            <span class="tdw-topic-color" :style="{ background: focusedNode.persistent_topic.color }" />
            <span class="tdw-topic-label">{{ focusedNode.persistent_topic.label }}</span>
            <span class="tdw-topic-status">{{ topicStatusLabel(focusedNode.persistent_topic.status) }}</span>
          </div>
          <div class="tdw-topic-actions">
            <button
              v-if="!lifecycleActive"
              class="tdw-topic-btn"
              :disabled="lifecycleLoading"
              @click="focusedNode.persistent_topic && enterTopicLifeline(focusedNode.persistent_topic.id)"
            >
              <Icon icon="mdi:sitemap-outline" width="13" />
              <span>{{ lifecycleLoading ? '加载中…' : '话题生命线' }}</span>
            </button>
            <button class="tdw-topic-btn" @click="showTopicManage = true">
              <Icon icon="mdi:folder-cog-outline" width="13" />
              <span>话题管理</span>
            </button>
          </div>
        </div>
        <!-- Lifeline/lifecycle aggregate summary -->
        <div v-if="lifelineNodes.length > 0" class="tdw-detail-summary">
          <span>共 {{ lifelineSummary.articles }}篇 · {{ lifelineSummary.threads }}线索</span>
          <span
            v-for="[status, count] in lifelineSummary.statusCounts"
            :key="status"
            class="tdw-summary-chip"
          >
            <span class="tdw-summary-dot" :class="`tdw-status-${status}`" />
            {{ statusLabel(status) }} {{ count }}
          </span>
        </div>
        <div v-if="lifelineNodes.length > 0" class="tdw-detail-lifeline">
          <div class="tdw-detail-section-label">
            {{ lifecycleActive ? '完整生命周期' : '生命线' }} ({{ lifelineNodes.length }}节点)
          </div>
          <div class="tdw-lifeline-list">
            <div
              v-for="n in lifelineNodes"
              :key="n.id"
              class="tdw-lifeline-node"
            >
              <div
                class="tdw-lifeline-item"
                :class="{ 'tdw-lifeline-item--expanded': expandedNodeId === n.id }"
                @click="selectAndToggleNodeThreads(n)"
              >
                <span class="tdw-lifeline-date">{{ n.period_date.slice(0, 10) }}</span>
                <span class="tdw-lifeline-label">{{ n.cluster_label }}</span>
                <Icon icon="mdi:chevron-right" width="12" class="tdw-lifeline-arrow" />
              </div>

              <!-- Expanded threads for this lifeline node (spec §面板操作 查看详细线索) -->
              <div v-if="expandedNodeId === n.id" class="tdw-lifeline-threads">
                <div v-if="nodeThreadsLoading === n.id" class="tdw-lifeline-threads-loading">
                  加载中…
                </div>
                <template v-else>
                  <div
                    v-for="thread in (nodeThreads.get(n.id) || [])"
                    :key="thread.id"
                    class="tdw-lifeline-thread-wrap"
                  >
                    <div
                      class="tdw-lifeline-thread"
                      :class="{ 'tdw-lifeline-thread--expanded': expandedThreadId === thread.id }"
                      @click="toggleThreadArticles(thread)"
                    >
                      <Icon icon="mdi:chevron-right" width="11" class="tdw-lifeline-thread-arrow" />
                      <span class="tdw-lifeline-thread-title">{{ thread.title }}</span>
                      <span v-if="thread.related_article_ids?.length" class="tdw-lifeline-thread-count">
                        {{ thread.related_article_ids.length }}篇
                      </span>
                    </div>

                    <!-- Article list (click an article to open its preview). -->
                    <div v-if="expandedThreadId === thread.id" class="tdw-thread-articles">
                      <div v-if="threadArticlesLoading === thread.id" class="tdw-thread-articles-loading">
                        加载中…
                      </div>
                      <template v-else>
                        <div
                          v-for="art in (threadArticles.get(thread.id) || [])"
                          :key="art.id"
                          class="tdw-thread-article"
                          @click="openArticle(art.id)"
                        >
                          <Icon icon="mdi:file-document-outline" width="10" class="tdw-thread-article-icon" />
                          <span class="tdw-thread-article-title">{{ art.title }}</span>
                          <Icon icon="mdi:open-in-new" width="9" class="tdw-thread-article-external" />
                        </div>
                        <div
                          v-if="thread.related_article_ids && thread.related_article_ids.length > 10"
                          class="tdw-thread-articles-more"
                        >
                          还有 {{ thread.related_article_ids.length - 10 }} 篇…
                        </div>
                        <div
                          v-if="(threadArticles.get(thread.id) || []).length === 0"
                          class="tdw-thread-articles-empty"
                        >
                          无关联文章
                        </div>
                      </template>
                    </div>
                  </div>
                  <div
                    v-if="(nodeThreads.get(n.id) || []).length === 0"
                    class="tdw-lifeline-threads-empty"
                  >
                    无关联线索
                  </div>
                </template>
              </div>
            </div>
          </div>
        </div>
        <!-- Lifecycle entry (only in BFS lifeline mode, not already in lifecycle). -->
        <button
          v-if="!lifecycleActive"
          class="tdw-btn tdw-btn-lifecycle"
          :disabled="lifecycleLoading"
          @click="enterLifecycle(focusedNode.id)"
        >
          <Icon icon="mdi:timeline-clock-outline" width="14" />
          <span>{{ lifecycleLoading ? '加载中…' : '查看完整生命周期' }}</span>
        </button>
        <button class="tdw-btn tdw-btn-back" @click="lifecycleActive ? exitLifecycle() : closePanel()">
          {{ lifecycleActive ? '返回主墙' : '关闭面板' }}
        </button>
      </div>
    </Transition>

    <!-- Loading / error states -->
    <div v-if="loading" class="tdw-loading">载入中…</div>
    <div v-else-if="error" class="tdw-error">加载失败，请重试</div>

    <!-- Unified topic management dialog (replaces native alert/prompt/confirm) -->
    <TopicManageDialog
      v-model="showTopicManage"
      :board-id="boardId"
      @changed="loadBoardData"
    />
  </div>
</template>

<style scoped>
.tdw-root {
  position: fixed;
  inset: 0;
  background:
    radial-gradient(circle at 20% 18%, rgba(235, 203, 139, 0.12), transparent 28%),
    radial-gradient(circle at 72% 60%, rgba(124, 45, 18, 0.16), transparent 34%),
    linear-gradient(135deg, #080B0E 0%, #17100C 48%, #050709 100%);
  z-index: 50;
  overflow: hidden;
}
.tdw-root::before,
.tdw-root::after {
  position: absolute;
  inset: 0;
  pointer-events: none;
  content: "";
}
.tdw-root::before {
  z-index: 1;
  opacity: 0.28;
  background-image:
    url("data:image/svg+xml,%3Csvg viewBox='0 0 360 520' xmlns='http://www.w3.org/2000/svg'%3E%3Cpath d='M58 438 C126 360 96 300 178 244 C252 193 214 119 302 72' fill='none' stroke='%23EBCB8B' stroke-opacity='.5' stroke-width='3' stroke-dasharray='10 12'/%3E%3Cg fill='%23EBCB8B' fill-opacity='.58' stroke='%23140D0B' stroke-opacity='.55' stroke-width='5'%3E%3Ccircle cx='58' cy='438' r='13'/%3E%3Ccircle cx='112' cy='344' r='10'/%3E%3Ccircle cx='178' cy='244' r='15'/%3E%3Ccircle cx='242' cy='164' r='10'/%3E%3Ccircle cx='302' cy='72' r='14'/%3E%3C/g%3E%3Cg fill='none' stroke='%23FFF4D6' stroke-opacity='.22' stroke-width='1.5'%3E%3Ccircle cx='178' cy='244' r='34'/%3E%3Ccircle cx='302' cy='72' r='30'/%3E%3C/g%3E%3C/svg%3E"),
    linear-gradient(rgba(255, 244, 214, 0.028) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 244, 214, 0.018) 1px, transparent 1px),
    url("data:image/svg+xml,%3Csvg viewBox='0 0 180 180' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.88' numOctaves='4' stitchTiles='stitch'/%3E%3CfeColorMatrix type='saturate' values='0'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)' opacity='0.34'/%3E%3C/svg%3E");
  background-repeat: no-repeat, repeat, repeat, repeat;
  background-position: calc(100% - 92px) 48%, 0 0, 0 0, 0 0;
  background-size: 360px 520px, 42px 42px, 42px 42px, 180px 180px;
  mix-blend-mode: screen;
}
.tdw-root::after {
  z-index: 2;
  background:
    radial-gradient(circle at 50% 42%, transparent 0 64%, rgba(0, 0, 0, 0.28) 100%),
    linear-gradient(90deg, rgba(0, 0, 0, 0.26), transparent 18%, transparent 86%, rgba(0, 0, 0, 0.24));
}
.tdw-canvas {
  position: relative;
  z-index: 0;
  display: block;
  width: 100%;
  height: 100%;
}
.tdw-css2d {
  position: absolute;
  inset: 0;
  z-index: 3;
  pointer-events: none;
}
.tdw-topbar {
  position: absolute;
  top: 1rem;
  left: 1rem;
  right: 1rem;
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: 0.75rem;
  align-items: center;
  z-index: 4;
  pointer-events: none;
}
.tdw-topbar > * { pointer-events: auto; }
.tdw-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.4rem 0.75rem;
  background: rgba(255, 255, 255, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.15);
  border-radius: 0.4rem;
  color: rgba(255, 255, 255, 0.85);
  font-size: 0.85rem;
  cursor: pointer;
}
.tdw-btn:hover { background: rgba(255, 255, 255, 0.14); }
.tdw-section-switch {
  justify-self: center;
  display: inline-flex;
  gap: 0.25rem;
  min-width: 0;
  max-width: 100%;
  padding: 0.22rem;
  background: rgba(5, 7, 9, 0.66);
  border: 1px solid rgba(255, 244, 214, 0.12);
  border-radius: 0.45rem;
  box-shadow: 0 12px 28px rgba(0, 0, 0, 0.26);
}
.tdw-section-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  min-height: 2rem;
  padding: 0.28rem 0.62rem;
  border: 0;
  border-radius: 0.32rem;
  background: transparent;
  color: rgba(255, 251, 235, 0.66);
  font-size: 0.78rem;
  cursor: pointer;
}
.tdw-section-btn.active {
  background: rgba(220, 38, 38, 0.92);
  color: #fff;
}
.tdw-section-btn:disabled {
  opacity: 0.38;
  cursor: not-allowed;
}
.tdw-days-toggle {
  display: flex;
  gap: 0.25rem;
  background: rgba(0, 0, 0, 0.4);
  border-radius: 0.4rem;
  padding: 0.2rem;
}
.tdw-days-btn {
  padding: 0.3rem 0.6rem;
  border: none;
  background: transparent;
  color: rgba(255, 255, 255, 0.6);
  border-radius: 0.3rem;
  cursor: pointer;
  font-size: 0.8rem;
}
.tdw-days-btn.active {
  background: #DC2626;
  color: #fff;
}
.tdw-days-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

/* 视图切换（时间线 / 话题泳道）——与 days-toggle 同款深色浮层 */
.tdw-view-toggle {
  display: flex;
  gap: 0.25rem;
  background: rgba(0, 0, 0, 0.4);
  border-radius: 0.4rem;
  padding: 0.2rem;
}
.tdw-view-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.3rem 0.55rem;
  border: none;
  background: transparent;
  color: rgba(255, 255, 255, 0.6);
  border-radius: 0.3rem;
  cursor: pointer;
  font-size: 0.78rem;
}
.tdw-view-btn:hover:not(:disabled) {
  color: rgba(255, 255, 255, 0.9);
}
.tdw-view-btn.active {
  background: rgba(96, 165, 250, 0.25);
  color: #93c5fd;
}
.tdw-view-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

/* 话题泳道分批翻页浮层（左下角）——与 view-toggle 同款深色半透明 HUD。
   叠在 canvas 上，不随应用主题变（与 topbar/days-toggle 视觉一致）。 */
.tdw-lane-pager {
  position: absolute;
  left: 1rem;
  bottom: 1rem;
  z-index: 4;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  max-width: min(560px, calc(100vw - 2rem));
  padding: 0.4rem 0.55rem;
  background: rgba(0, 0, 0, 0.55);
  border: 1px solid rgba(255, 244, 214, 0.12);
  border-radius: 0.45rem;
  box-shadow: 0 12px 28px rgba(0, 0, 0, 0.26);
  backdrop-filter: blur(6px);
}
.tdw-lane-pager-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  width: 1.9rem;
  height: 1.9rem;
  padding: 0;
  border: none;
  border-radius: 0.3rem;
  background: transparent;
  color: rgba(255, 255, 255, 0.7);
  cursor: pointer;
  transition: background 0.12s, color 0.12s;
}
.tdw-lane-pager-btn:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.12);
  color: rgba(255, 255, 255, 0.95);
}
.tdw-lane-pager-btn:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}
.tdw-lane-pager-info {
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
  min-width: 0;
}
.tdw-lane-pager-title {
  font-size: 0.72rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.9);
  white-space: nowrap;
}
.tdw-lane-pager-tags {
  font-size: 0.66rem;
  color: rgba(255, 255, 255, 0.55);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Detail panel — right-side case drawer */
.tdw-detail-panel {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  z-index: 4;
  width: min(520px, calc(100vw - 2rem));
  max-height: none;
  overflow-y: auto;
  background: var(--color-dialog-bg);
  border: 1px solid var(--color-border-medium);
  border-right: 0;
  border-radius: 0.55rem 0 0 0.55rem;
  padding: 1rem 1.1rem 1.25rem;
  color: var(--color-text-primary);
  box-shadow: -18px 0 42px rgba(0, 0, 0, 0.32), var(--shadow-strong);
  font-family: 'JetBrains Mono', 'Courier New', monospace;
}
.tdw-detail-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.75rem;
  color: var(--color-text-secondary);
  border-bottom: 1px solid var(--color-dialog-divider);
  padding-bottom: 0.5rem;
  margin-bottom: 0.5rem;
}
.tdw-case-nav {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.35fr);
  gap: 0.45rem;
  margin-bottom: 0.65rem;
}
.tdw-case-next {
  min-width: 0;
}
.tdw-case-nav-btn,
.tdw-branch-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.25rem;
  min-height: 2rem;
  width: 100%;
  border: 1px solid var(--color-border-medium);
  border-radius: 0.35rem;
  background: var(--color-bg-hover);
  color: var(--color-text-secondary);
  font-size: 0.72rem;
  cursor: pointer;
}
.tdw-case-nav-btn:hover:not(:disabled),
.tdw-branch-btn:hover {
  background: var(--color-accent-subtle);
  border-color: var(--color-accent);
}
.tdw-case-nav-btn:disabled {
  opacity: 0.42;
  cursor: not-allowed;
}
.tdw-branch-picker {
  display: grid;
  grid-template-columns: 1fr;
  gap: 0.25rem;
}
.tdw-branch-label {
  font-size: 0.62rem;
  color: var(--color-accent);
}
.tdw-branch-btn {
  width: auto;
  min-height: 1.65rem;
}
.tdw-detail-title {
  font-size: 1rem;
  font-weight: 600;
  line-height: 1.35;
  margin-bottom: 0.4rem;
}
.tdw-detail-panel .tdw-btn {
  background: var(--color-bg-hover);
  border-color: var(--color-border-medium);
  color: var(--color-text-secondary);
}
.tdw-detail-panel .tdw-btn:hover {
  background: var(--color-accent-subtle);
  border-color: var(--color-accent);
  color: var(--color-accent);
}
.tdw-detail-meta {
  display: flex;
  justify-content: space-between;
  font-size: 0.75rem;
  color: var(--color-text-secondary);
  margin-bottom: 0.75rem;
}
.tdw-detail-status {
  color: var(--color-accent);
  font-weight: 600;
}
/* Lifeline/lifecycle aggregate summary */
.tdw-detail-summary {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.68rem;
  color: var(--color-text-secondary);
  padding: 0.35rem 0;
  border-top: 1px dashed var(--color-border-medium);
  border-bottom: 1px dashed var(--color-border-medium);
  margin-bottom: 0.5rem;
}
.tdw-summary-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
}
.tdw-summary-dot {
  display: inline-block;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #9ca3af;
}
/* Status colors mirror STYLE.statusColors (types.ts). */
.tdw-status-emerging { background: #16a34a; }
.tdw-status-continuing { background: #2563eb; }
.tdw-status-split { background: #ea580c; }
.tdw-status-merge { background: #9333ea; }
.tdw-status-ending { background: #9ca3af; }
.tdw-detail-section-label {
  font-size: 0.7rem;
  color: var(--color-text-secondary);
  margin-bottom: 0.3rem;
}
.tdw-lifeline-node {
  border-bottom: 1px dotted var(--color-border-subtle);
}
.tdw-lifeline-item {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.25rem 0;
  font-size: 0.75rem;
  cursor: pointer;
}
.tdw-lifeline-item:hover { background: var(--color-accent-subtle); }
.tdw-lifeline-item--expanded .tdw-lifeline-arrow { transform: rotate(90deg); }
.tdw-lifeline-arrow {
  margin-left: auto;
  color: var(--color-text-muted);
  transition: transform 0.12s ease;
  flex-shrink: 0;
}
.tdw-lifeline-date { color: var(--color-secondary); min-width: 5.5rem; }
.tdw-lifeline-label {
  flex: 1;
  min-width: 0;
  line-height: 1.25;
  overflow-wrap: anywhere;
}
.tdw-lifeline-threads {
  padding: 0.2rem 0 0.3rem 1rem;
}
.tdw-lifeline-threads-loading,
.tdw-lifeline-threads-empty {
  font-size: 0.7rem;
  color: var(--color-text-secondary);
  padding: 0.15rem 0;
}
.tdw-lifeline-thread {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.2rem 0.25rem;
  border-radius: 3px;
  cursor: pointer;
  transition: background 0.1s ease;
}
.tdw-lifeline-thread:hover { background: var(--color-accent-subtle); }
.tdw-lifeline-thread-arrow {
  color: var(--color-text-muted);
  flex-shrink: 0;
  transition: transform 0.12s ease;
}
.tdw-lifeline-thread--expanded .tdw-lifeline-thread-arrow { transform: rotate(90deg); }
.tdw-lifeline-thread-title {
  flex: 1;
  font-size: 0.72rem;
  color: var(--color-text-primary);
  min-width: 0;
  line-height: 1.25;
  overflow-wrap: anywhere;
}
.tdw-lifeline-thread-count {
  font-size: 0.62rem;
  color: var(--color-text-muted);
  flex-shrink: 0;
}
/* Expanded article list under a thread */
.tdw-thread-articles {
  padding: 0.15rem 0 0.25rem 1.1rem;
}
.tdw-thread-articles-loading,
.tdw-thread-articles-empty {
  font-size: 0.65rem;
  color: var(--color-text-secondary);
  padding: 0.15rem 0;
}
.tdw-thread-article {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.18rem 0.25rem;
  border-radius: 3px;
  cursor: pointer;
  transition: background 0.1s ease;
}
.tdw-thread-article:hover { background: var(--color-accent-subtle); }
.tdw-thread-article-icon { color: var(--color-text-muted); flex-shrink: 0; }
.tdw-thread-article-title {
  flex: 1;
  font-size: 0.66rem;
  color: var(--color-text-secondary);
  min-width: 0;
  line-height: 1.25;
  overflow-wrap: anywhere;
}
.tdw-thread-article:hover .tdw-thread-article-title { color: var(--color-accent); }
.tdw-thread-article-external { color: var(--color-text-muted); flex-shrink: 0; }
.tdw-thread-articles-more {
  font-size: 0.6rem;
  color: var(--color-text-muted);
  padding: 0.1rem 0.25rem;
}
.tdw-btn-back { margin-top: 0.75rem; width: 100%; justify-content: center; }
.tdw-btn-lifecycle {
  margin-top: 0.5rem;
  width: 100%;
  justify-content: center;
  border-color: var(--color-accent);
  color: var(--color-accent);
}
.tdw-btn-lifecycle:hover:not(:disabled) {
  background: var(--color-accent-subtle);
}
.tdw-btn-lifecycle:disabled {
  opacity: 0.6;
  cursor: wait;
}
/* §9.4/9.6 persistent-topic lifeline + management bar */
.tdw-topic-bar {
  margin-top: 0.5rem;
  padding: 0.5rem 0.6rem;
  border: 1px solid rgba(235, 203, 139, 0.22);
  border-radius: 6px;
  background: rgba(235, 203, 139, 0.05);
}
.tdw-topic-info {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-bottom: 0.4rem;
}
.tdw-topic-color {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}
.tdw-topic-label {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  font-weight: 600;
  color: rgba(245, 230, 200, 0.92);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tdw-topic-status {
  flex-shrink: 0;
  padding: 1px 6px;
  font-size: 11px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.08);
  color: rgba(235, 203, 139, 0.7);
}
.tdw-topic-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.3rem;
}
.tdw-topic-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.25rem 0.5rem;
  font-size: 12px;
  color: rgba(235, 203, 139, 0.75);
  background: transparent;
  border: 1px solid rgba(235, 203, 139, 0.28);
  border-radius: 4px;
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}
.tdw-topic-btn:hover:not(:disabled) {
  background: rgba(235, 203, 139, 0.12);
  color: rgba(245, 230, 200, 0.95);
}
.tdw-topic-btn:disabled {
  opacity: 0.6;
  cursor: wait;
}
.tdw-merge-picker {
  margin-top: 0.4rem;
  padding-top: 0.4rem;
  border-top: 1px solid rgba(235, 203, 139, 0.15);
}
.tdw-merge-hint {
  margin-bottom: 0.3rem;
  font-size: 11px;
  color: rgba(235, 203, 139, 0.55);
}
.tdw-merge-candidate {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  width: 100%;
  margin-bottom: 0.2rem;
  padding: 0.3rem 0.4rem;
  font-size: 12px;
  color: rgba(245, 230, 200, 0.9);
  text-align: left;
  background: transparent;
  border: 1px solid rgba(235, 203, 139, 0.18);
  border-radius: 4px;
  cursor: pointer;
}
.tdw-merge-candidate:hover {
  background: rgba(235, 203, 139, 0.12);
}
.tdw-merge-empty {
  padding: 0.3rem 0;
  font-size: 11px;
  color: rgba(235, 203, 139, 0.55);
}
/* Scrollable lifeline node list (was slice(0,10); now unlimited + scrolls). */
.tdw-lifeline-list {
  max-height: 40vh;
  overflow-y: auto;
}

/* motion-v-style transition (Vue <Transition>) */
.tdw-panel-enter-active, .tdw-panel-leave-active {
  transition: transform 0.3s ease, opacity 0.3s ease;
}
.tdw-panel-enter-from, .tdw-panel-leave-to {
  transform: translateX(100%);
  opacity: 0;
}
.tdw-panel-enter-to, .tdw-panel-leave-from {
  transform: translateX(0);
  opacity: 1;
}

@media (max-width: 900px) {
  .tdw-topbar {
    grid-template-columns: 1fr;
    align-items: stretch;
  }
  .tdw-section-switch,
  .tdw-days-toggle,
  .tdw-view-toggle {
    justify-self: stretch;
    overflow-x: auto;
  }
  .tdw-detail-panel {
    width: min(460px, 100vw);
    border-radius: 0;
  }
  .tdw-lane-pager {
    left: 0.5rem;
    right: 0.5rem;
    bottom: 0.5rem;
    max-width: none;
  }
}

/* Chapter transition DOM removed (no BoardSelector entry yet — see HANDOFF §3.2).
   ChapterTransition.ts is retained for future board-switch support. */

.tdw-loading, .tdw-error {
  position: absolute;
  top: 50%;
  left: 50%;
  z-index: 4;
  transform: translate(-50%, -50%);
  color: rgba(255, 255, 255, 0.7);
  font-size: 1rem;
}
</style>

<!-- Global (non-scoped) styles for CSS2DRenderer-injected tooltip elements.
     CSS2DRenderer appends its DOM outside this component's scoped scope, into
     .tdw-css2d, so these rules must not be scoped. -->
<style>
.tdw-card-tooltip {
  pointer-events: auto;
  background:
    linear-gradient(180deg, rgba(255, 251, 235, 0.98), rgba(239, 221, 180, 0.96));
  border: 1px solid rgba(73, 42, 15, 0.32);
  border-radius: 0.22rem;
  padding: 0.3rem 0.55rem 0.28rem;
  font-family: 'JetBrains Mono', 'Courier New', monospace;
  font-size: 0.72rem;
  color: #1A1A1A;
  white-space: nowrap;
  box-shadow:
    0 8px 18px rgba(0, 0, 0, 0.45),
    inset 0 0 0 1px rgba(255, 255, 255, 0.24);
  transform: translate(-50%, -100%);
  cursor: pointer;
  z-index: 2;
}
.tdw-card-tooltip::before {
  content: '';
  position: absolute;
  left: 0.45rem;
  right: 0.45rem;
  top: -0.16rem;
  height: 0.28rem;
  background: rgba(231, 185, 107, 0.78);
  border-radius: 1px;
  transform: rotate(-1.5deg);
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.18);
}
.tdw-card-tooltip-label {
  font-weight: 600;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  letter-spacing: 0;
}
.tdw-card-tooltip-status {
  font-size: 0.62rem;
  color: #7C2D12;
  margin-top: 0.1rem;
}
</style>
