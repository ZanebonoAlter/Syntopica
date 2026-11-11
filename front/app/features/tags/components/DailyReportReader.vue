<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, toRef, watch } from 'vue'
import { Icon } from '@iconify/vue'
import TopicDetectiveWall from './TopicDetectiveWall.client.vue'
import DailyReportMasthead from './daily-report/DailyReportMasthead.vue'
import DailyReportSidebar from './daily-report/DailyReportSidebar.vue'
import DailyReportTopicSection from './daily-report/DailyReportTopicSection.vue'
import DailyReportWatchIndex from './daily-report/DailyReportWatchIndex.vue'
import MarginNotesRail from './daily-report/MarginNotesRail.vue'
import SelectionAskBubble from './daily-report/SelectionAskBubble.vue'
import AppSidebarDrawer from '~/components/ui/AppSidebarDrawer.vue'
import { hasOpenOverlay } from '~/components/ui/overlayStack'
import PeelTransition from '~/components/PeelTransition.vue'
import {
  applyMarginNoteHighlights,
} from './daily-report/marginNoteAnchor'
import { useMarginNoteSelection, type MarginNoteAnchorDraft } from '~/features/tags/composables/useMarginNoteSelection'
import { useMarginNotes } from '~/features/tags/composables/useMarginNotes'
import { useMediaQuery } from '~/composables/useMediaQuery'
import {
  buildQualityZones,
  formatMagazineDate,
  groupSectionsByTopic,
  selectLeadStory,
} from './daily-report/dailyReportMagazine'
import { useDailyReportReader } from '~/features/tags/composables/useDailyReportReader'
import { todayInShanghai } from '~/features/tags/composables/useBoardConsole'
import { useNotify } from '~/composables/useNotify'
import { useLaneTrendData } from '~/features/tags/composables/useLaneTrendData'
import type { PeelDirection } from '~/composables/usePeelTransition'
import { useTopicWatchesApi, type TopicWatchHit } from '~/api/topicWatches'
import { useDailyReportsApi } from '~/api/dailyReports'
import type { ActiveWatchSummary, DailyReportWatchHit } from '~/api/dailyReports'

/** 版块切换条所需的最小版块信息（结构兼容 SemanticBoard）。 */
interface BoardOption {
  id: number
  label: string
}

const props = withDefaults(defineProps<{
  boardId: number
  boardTitle?: string
  /** 可就近切换的版块列表（来自 useTagsPage.boards）；缺省时隐藏切换条。 */
  boards?: BoardOption[]
  /** 深链（管理页「跳原日报」）：打开阅读层后直接定位的报告 id。 */
  initialReportId?: number | null
  /** 深链定位的批注 id（定位 + 闪现，MG-5）。 */
  initialAnnotationId?: number | null
  /** `/tags/:boardId/:date` 深链：按日期选日报。 */
  initialDate?: string | null
  /** 列表点追踪命中进入时，定位对应 section。 */
  initialWatchId?: string | null
  /**
   * `?cluster=` 深链（D14）：定位本期正文 DOM 顺序中该聚簇的第一条 thread
   * （命中条件 = thread.tag_ids 含聚簇代表标签 id）；本期正文无对应线索时不滚动、给提示。
   */
  initialClusterId?: number | null
  /** 旧 `?story=` 深链已下线（D14）：解析后忽略，只提示一次，不定位、不报错。 */
  initialLegacyStory?: boolean
  /** desk=路由页内静态四区；overlay=旧列表浮层（阶段④前兼容）。 */
  mode?: 'overlay' | 'desk'
  /** 阅读台今日（Asia/Shanghai 本地日期），用于演进带「今」标记。 */
  today?: string
}>(), {
  boards: () => [],
  initialReportId: null,
  initialAnnotationId: null,
  initialDate: null,
  initialWatchId: null,
  initialClusterId: null,
  initialLegacyStory: false,
  mode: 'overlay',
  today: '',
})

const emit = defineEmits<{
  /** 关闭阅读层（overlay）或返回矩阵（desk，由路由页处理）。 */
  close: []
  openArticle: [articleId: number]
  selectBoard: [boardId: number]
  /** desk 下日期切换：路由页替换 URL。 */
  selectDate: [date: string]
  /** 「在矩阵中查看这条线」。 */
  openLane: [topicId: number]
  openTodayBoard: [payload: { boardId: number, date: string }]
}>()

/** 当前转场方向：切版块→纵向，切日期→横向。须在触发 key 变更前同步写入。 */
const direction = ref<PeelDirection>('horizontal')
/** 动画进行中锁，防越界连点。 */
const animating = ref(false)

const reader = useDailyReportReader(toRef(props, 'boardId'), {
  // 阅读台右栏固定 14 天；旧浮层保持 7 天，沿用「加载更早」增量。
  initialDays: props.mode === 'desk' ? 14 : 7,
})
const { generateDailyReport, getTodayReports } = useDailyReportsApi()
const notify = useNotify()
// 泳道趋势区取数（lane-trend-overview §D4）：板块级 lane-dynamics 页面级缓存（首个泳道展开触发、
// 翻期不重拉）+ contexts 月/年按需缓存；均为只读，不随阅读报告日期变化（趋势=现在）。
const { contextEntries, ensureLaneDynamics, ensureContext, getLaneDynamicsEntry } = useLaneTrendData(toRef(props, 'boardId'))
/**
 * 重新分桶（add-intra-lane-theme-bucketing）：由子组件的二次确认后触发，
 * 成功后仅局部替换该桶组 sections（不重载整份日报，阅读位置不丢）。
 */
async function handleRebucket(sectionId: number) {
  const report = reader.selectedDetail.value
  if (!report) return
  const outcome = await reader.rebucket(report.id, sectionId)
  if (!outcome.ok) {
    notify.error(outcome.error || '重新分桶失败')
    return
  }
  if (outcome.buckets <= 1) notify.warn('没有发现可拆分的主题方向')
  else notify.success(`已重新分桶：${outcome.buckets} 个方向`)
}

const watchesApi = useTopicWatchesApi()
const watchHitsByReport = ref(new Map<number, TopicWatchHit[]>())
const focusSectionId = ref<number | null>(null)
/** 重点定位请求（D14）：nonce 使重复点击同簇重触发；切 report 清空。 */
const locateRequest = ref<{ clusterId: number, nonce: number } | null>(null)
let locateNonce = 0
const showReader = ref(true)
const showDetectiveWall = ref(false)
const detectiveTopicId = ref<number | undefined>()
let previousBodyOverflow = ''

const qualityZones = computed(() => buildQualityZones(reader.selectedDetail.value?.sections ?? []))
const activeTopics = computed(() => {
  const zone = qualityZones.value.find(item => item.key === 'active')
  return zone ? groupSectionsByTopic(zone) : []
})

/** 当前报告 id：页边注数据层按报告拉取/切换。 */
const selectedReportId = computed(() => reader.selectedDetail.value?.id ?? 0)
const marginNotes = useMarginNotes(selectedReportId)

// —— 页边注交互（daily-report-margin-notes）——
const peelPage = ref<HTMLElement | null>(null)
const railRef = ref<{ focusCard: (annotationId: number, options?: { focusInput?: boolean }) => Promise<boolean> } | null>(null)
const litNoteId = ref<number | null>(null)
const pendingFocusNoteId = ref<number | null>(null)
/** 深链定位挂起标志：报告列表未就绪期间阻止 loading watch 自动选中最新一期。 */
const deepLinkPending = ref(false)
/** 引用文本失配（原文已变更）的批注 id（高亮应用后回写，PR-3）。 */
const changedNoteIds = ref<number[]>([])
/** 窄屏（<1100px，沿用 drm-layout 现有断点）：右抽屉 + 右下浮动入口。 */
const isNotesNarrow = useMediaQuery('(max-width: 1100px)')
const notesDrawerOpen = ref(false)
let litTimer: ReturnType<typeof setTimeout> | null = null

const noteAskStates = computed(() => marginNotes.askStates.value)

function applyNoteHighlights() {
  const root = peelPage.value
  if (!root) return
  const { resolutions } = applyMarginNoteHighlights(root, marginNotes.annotations.value)
  changedNoteIds.value = [...resolutions.entries()]
    .filter(([, resolution]) => resolution === 'changed')
    .map(([id]) => id)
  if (pendingFocusNoteId.value != null) {
    const target = pendingFocusNoteId.value
    // 批注数据未到达前不消费定位请求：report 详情 watcher 会先于 annotations 触发一次
    // applyNoteHighlights（annotations 空 → 无 mark），消费掉就再没人触发定位——
    // 深链/跳原日报会停在报告顶部、卡不点亮、正文不闪现。annotations 到齐后 watcher
    // 会再跑一次，pendingFocusNoteId 仍在，此刻才真正定位。
    if (!marginNotes.annotations.value.some(item => item.id === target)) return
    pendingFocusNoteId.value = null
    void focusAnnotation(target, { focusInput: false })
    // 深链定位（管理页「跳原日报」）：栏内点亮之外，正文同步滚到锚点并闪现
    // （spec「全局批注管理」：打开阅读层并定位到该批注锚点、高亮闪现）。
    jumpToOriginal(target)
  }
}

watch([marginNotes.annotations, () => reader.selectedDetail.value?.id], () => {
  // 等正文（含从数据重建的 DOM）就绪后再落 mark；幂等 + 自愈清理见 marginNoteAnchor
  void nextTick(applyNoteHighlights)
}, { immediate: true })

/** 定位并点亮某卡：窄屏先开抽屉；mark/深链/管理页跳转共用。 */
async function focusAnnotation(annotationId: number, options?: { focusInput?: boolean }) {
  if (isNotesNarrow.value) notesDrawerOpen.value = true
  await nextTick()
  if (litTimer) clearTimeout(litTimer)
  litNoteId.value = null
  await nextTick()
  litNoteId.value = annotationId
  await railRef.value?.focusCard(annotationId, { focusInput: options?.focusInput })
  litTimer = setTimeout(() => { litNoteId.value = null }, 1600)
}

/** 卡「↗ 跳回原文」：滚动到正文 mark 并闪现（引用失配时无 mark，静默降级）。 */
function jumpToOriginal(annotationId: number) {
  const mark = peelPage.value?.querySelector<HTMLElement>(`mark[data-jump="${annotationId}"]`)
  if (!mark) return
  if (typeof mark.scrollIntoView === 'function') mark.scrollIntoView({ behavior: 'smooth', block: 'center' })
  mark.classList.remove('mn-flash')
  mark.classList.add('mn-flash')
  setTimeout(() => mark.classList.remove('mn-flash'), 2400)
}

/** 正文 mark 点击 → 点亮对应卡（窄屏开抽屉定位）；不触发 thread 展开。 */
function handleMarkClick(annotationId: number) {
  void focusAnnotation(annotationId)
}

/** lead 批注的 section 归属：headline 由 section 派生时取该 section，否则 0（后端契约点：lead 批注无 section 归属时 section_id=0，见最终汇报联调点）。 */
const leadSectionId = computed(() => {
  const detail = reader.selectedDetail.value
  if (!detail) return 0
  return selectLeadStory(detail)?.sectionId ?? 0
})

/** 气泡确认 → 落锚：乐观落锚状态机（mark 已由 handleBubbleConfirm 先行包好），成功后聚焦输入。 */
async function handleAnchorConfirm(draft: MarginNoteAnchorDraft) {
  const detail = reader.selectedDetail.value
  if (!detail) return
  const threadId = draft.key === 'lead' ? null : Number(draft.key.slice(2))
  const sectionId = draft.key === 'lead'
    ? leadSectionId.value
    : (detail.sections.flatMap(section => section.threads).find(thread => thread.id === threadId)?.section_id ?? 0)
  const created = await marginNotes.anchor({
    section_id: sectionId,
    thread_id: threadId,
    quoted_text: draft.quotedText,
    anchor_offset_start: draft.start,
    anchor_offset_end: draft.end,
  })
  if (!created) return
  await focusAnnotation(created.id, { focusInput: true })
}

const { bubble: mnBubble, confirmBubble, buildMark } = useMarginNoteSelection({
  active: showReader,
  reportRoot: peelPage,
  onMarkClick: handleMarkClick,
})

/** 气泡确认：消费草稿拿回 live range → 包 mark（跨节点降级只落卡）→ 走乐观落锚状态机。 */
function handleBubbleConfirm() {
  const consumed = confirmBubble()
  if (!consumed) return
  const { draft, range } = consumed
  if (peelPage.value && range) buildMark(range, -1) // 临时 id mark，落锚成功后高亮应用循环自愈换真 id
  void handleAnchorConfirm(draft)
}

function handleAskNote(annotationId: number, question: string) {
  void marginNotes.ask(annotationId, question)
}

function handleRetryAsk(annotationId: number) {
  void marginNotes.retryAsk(annotationId)
}

async function handleRemoveNote(annotationId: number) {
  await marginNotes.remove(annotationId)
  // 高亮 watch 自愈：id 不在现存集合的 mark 会被清理，正文高亮同步消失
}

/** 引用 chip → 与 thread 溯源同链路：ensureArticles 取标题后走文章预览。 */
async function openCitedArticle(articleId: number) {
  await reader.ensureArticleTitles([articleId])
  emit('openArticle', articleId)
}

onMounted(() => { void loadTodayBoards() })

onMounted(async () => {
  // 深链：报告 id 优先；其次按日期（/tags/:boardId/:date）；最后自动选最近一期。
  if (props.initialAnnotationId != null) pendingFocusNoteId.value = props.initialAnnotationId
  if (props.initialReportId || props.initialDate) deepLinkPending.value = true
  try {
    if (reader.loading.value) {
      await new Promise<void>(resolve => {
        const stop = watch(reader.loading, (isLoading) => {
          if (!isLoading) { stop(); resolve() }
        })
      })
    }
    if (props.initialReportId) {
      await reader.selectReportById(props.initialReportId)
    } else if (props.initialDate) {
      const date = props.initialDate.slice(0, 10)
      let index = reader.reports.value.findIndex(report => report.period_date.slice(0, 10) === date)
      // 深链日期早于默认 7 天窗口：按天差扩窗重取一次（只读）。
      if (index < 0) {
        const target = new Date(`${date}T12:00:00`).getTime()
        const today = new Date(`${deskToday.value}T12:00:00`).getTime()
        const diff = Math.max(0, Math.ceil((today - target) / 86_400_000))
        if (diff + 1 > reader.days.value) {
          reader.days.value = Math.min(370, diff + 2)
          await reader.loadReports()
          index = reader.reports.value.findIndex(report => report.period_date.slice(0, 10) === date)
        }
      }
      if (index >= 0) await reader.selectReport(index)
    }
  } finally {
    deepLinkPending.value = false
  }
  const detail = reader.selectedDetail.value
  if (detail) {
    await ensureWatchHits(detail.id, detail)
    if (props.initialWatchId) {
      const hit = (watchHitsByReport.value.get(detail.id) ?? []).find(item => item.watchId === String(props.initialWatchId))
      if (hit) locateWatchSection(hit.sectionId)
    }
  }
  await nextTick()
  // 深链定位（`?cluster=` → 日报正文）：复用刊头重点的同一路径；本期正文无该
  // 聚簇线索时不滚动、不跳近似位置，只给「本期正文暂无对应线索」提示。
  // 放在 nextTick 之后：正文分区组件已挂载，定位意图才会被各分区接收。
  if (props.initialClusterId != null && reader.selectedDetail.value) {
    handleLocateHighlight(props.initialClusterId)
  }
  // 旧 `?story=` 深链（D14）：解析后忽略，只提示该定位方式已下线，不定位、不报错。
  if (props.initialLegacyStory) {
    notify.warn('事件线定位已下线，请从「今日重点」进入正文')
  }
})

async function ensureWatchHits(reportId: number, detail?: { activeWatchHits?: DailyReportWatchHit[] }) {
  if (watchHitsByReport.value.has(reportId)) return watchHitsByReport.value.get(reportId) ?? []
  if (detail?.activeWatchHits) {
    const hits = detail.activeWatchHits.map((hit, index): TopicWatchHit => ({
      id: `${reportId}-${hit.watchId}-${hit.sectionId}-${index}`,
      watchId: String(hit.watchId),
      sectionId: String(hit.sectionId),
      reportId: String(reportId),
      periodDate: '',
      reason: hit.reason ?? '',
      watchLabel: hit.label,
      watchType: hit.type,
    }))
    watchHitsByReport.value.set(reportId, hits)
    watchHitsByReport.value = new Map(watchHitsByReport.value)
    return hits
  }

  const response = await watchesApi.getWatchHits(reportId)
  const hits = response.success && response.data ? response.data : []
  watchHitsByReport.value.set(reportId, hits)
  watchHitsByReport.value = new Map(watchHitsByReport.value)
  return hits
}

function locateWatchSection(sectionId: string | number) {
  focusSectionId.value = null
  nextTick(() => {
    focusSectionId.value = Number(sectionId)
  })
}

/** 刊头重点定位（D14）：先按当前 report 正文预判目标（thread.tag_ids 含聚簇代表标签 id）——
 *  点击时目标已消失则原地轻提示、不下发定位（不跳近似位置）；有目标则转发给各正文分区，
 *  由持有目标的分区展开并滚动/聚焦/高亮 2s（reduced-motion 不平滑滚动）。 */
function handleLocateHighlight(clusterId: number) {
  const detail = reader.selectedDetail.value
  const hasTarget = (detail?.sections ?? []).some(section =>
    (section.threads ?? []).some(thread => (thread.tag_ids ?? []).includes(clusterId)),
  )
  if (!hasTarget) {
    notify.warn('本期正文暂无对应线索')
    return
  }
  locateRequest.value = { clusterId, nonce: ++locateNonce }
}

async function closeReader() {
  emit('close')
}

async function selectReport(index: number) {
  focusSectionId.value = null
  locateRequest.value = null
  await reader.selectReport(index)
  const detail = reader.selectedDetail.value
  if (detail) await ensureWatchHits(detail.id, detail)
  scrollReaderTop()
}

async function shiftReport(offset: number) {
  focusSectionId.value = null
  locateRequest.value = null
  await reader.shiftReport(offset)
  const detail = reader.selectedDetail.value
  if (detail) await ensureWatchHits(detail.id, detail)
  scrollReaderTop()
}

function scrollReaderTop(): void {
  readerRoot.value?.scrollTo({ top: 0, behavior: 'smooth' })
}

/** 切日期（横向翻页）：在 key 变更前同步写入方向，加动画锁。 */
function shiftReportPeel(offset: number) {
  if (animating.value) return
  direction.value = 'horizontal'
  animating.value = true
  void shiftReport(offset)
}

/** 侧栏点击某天（横向翻页）。 */
function selectReportPeel(index: number) {
  if (animating.value) return
  direction.value = 'horizontal'
  animating.value = true
  void selectReport(index)
}

/** 就近切换版块（纵向翻页）：在 boardId 变更前同步写入方向。 */
function handleSwitchBoard(id: number) {
  if (animating.value || id === props.boardId) return
  direction.value = 'vertical'
  animating.value = true
  emit('selectBoard', id)
}

/** Peel 转场结束：释放动画锁。 */
function onPeelEnd() {
  animating.value = false
}

const todayReportRows = ref<{ boardId: number, date: string }[]>([])

const todayReportBoards = computed(() => todayReportRows.value.map(row => ({
  id: row.boardId,
  label: props.boards.find(board => board.id === row.boardId)?.label ?? `版块 #${row.boardId}`,
  date: row.date,
})))

async function loadTodayBoards(): Promise<void> {
  const response = await getTodayReports()
  if (!response?.success || !response.data) return
  todayReportRows.value = response.data.reports.map(row => ({
    boardId: row.semantic_board_id,
    date: row.period_date.slice(0, 10),
  }))
}

const generating = ref(false)

async function generateForDesk(): Promise<void> {
  if (generating.value) return
  const target = props.initialDate || deskToday.value
  generating.value = true
  try {
    const response = await generateDailyReport({ date: target, board_id: props.boardId })
    if (!response.success) {
      notify.error(response.error || '日报生成失败')
      return
    }
    notify.success(`已触发 ${target} 日报生成`)
    await reader.loadReports()
  } finally {
    generating.value = false
  }
}

async function openLatestReport(): Promise<void> {
  if (reader.reports.value.length > 0) await selectReportPeel(0)
}

async function loadHistorical(reportIds: number[]) {
  await Promise.all(reportIds.map(reportId => reader.ensureHistoricalDetail(reportId)))
}

function scrollTo(target: string) {
  document.getElementById(target)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

function openDetectiveWall(topicId?: number) {
  detectiveTopicId.value = topicId
  showDetectiveWall.value = true
}

function handleKeydown(event: KeyboardEvent) {
  if (!showReader.value) return
  if (event.key === 'Escape') {
    // 窄屏页边注抽屉开着时，Esc 只该关抽屉（AppSidebarDrawer 自己的 Esc 处理器），
    // 否则同一击键连阅读层一起关、丢失阅读位置。
    if (notesDrawerOpen.value || hasOpenOverlay()) return
    event.preventDefault()
    void closeReader()
  } else if (props.mode !== 'desk' && (event.key === 'ArrowDown' || event.key === 'ArrowRight')) {
    event.preventDefault()
    shiftReportPeel(1)
  } else if (props.mode !== 'desk' && (event.key === 'ArrowUp' || event.key === 'ArrowLeft')) {
    event.preventDefault()
    shiftReportPeel(-1)
  }
}

watch(showReader, (open) => {
  if (open) {
    if (props.mode === 'overlay') {
      previousBodyOverflow = document.body.style.overflow
      document.body.style.overflow = 'hidden'
    }
    document.addEventListener('keydown', handleKeydown)
  } else {
    document.body.style.overflow = previousBodyOverflow
    document.removeEventListener('keydown', handleKeydown)
  }
}, { immediate: true })

watch(() => props.boardId, () => {
  // 不关闭 reader：版块切换发生在 reader 内部（就近切换条），需保持开启以播放纵向翻页转场。
  // reader 打开时 modal 覆盖侧栏，外部切换不可能发生；reader 关闭时此项为 no-op。
  showDetectiveWall.value = false
  detectiveTopicId.value = undefined
})

// 版块切换后（reader 开着）日报列表重载完成时自动选中第一天，触发纵向翻页进入转场。
// 深链定位挂起期间跳过自动选中（否则会抢先选最新一期，随后再跳目标造成闪跳）。
watch(reader.loading, async (isLoading) => {
  if (isLoading) return
  if (deepLinkPending.value) return
  if (!showReader.value || reader.currentDayIndex.value >= 0) return
  if (reader.reports.value.length > 0) {
    await reader.selectReport(0)
  } else {
    // 新版块无日报：无进入转场，兏底释放动画锁
    animating.value = false
  }
})

// desk：日期切换回写 URL；优先进/退期不改变当前阅读位置。
watch(() => reader.selectedReport.value?.period_date, (date) => {
  if (props.mode === 'desk' && date) emit('selectDate', date.slice(0, 10))
})

const deskToday = computed(() => props.today || todayInShanghai())
const readerRoot = ref<HTMLElement | null>(null)

onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown)
  document.body.style.overflow = previousBodyOverflow
  if (litTimer) clearTimeout(litTimer)
})
</script>

<template>
  <Teleport to="body" :disabled="mode === 'desk'">
    <Transition name="drm-reader-transition">
      <div
        v-if="showReader"
        class="drm-overlay"
        :class="{ 'is-desk': mode === 'desk' }"
        :role="mode === 'overlay' ? 'dialog' : 'region'"
        :aria-modal="mode === 'overlay' ? 'true' : undefined"
        aria-label="日报详情"
      >
        <article ref="readerRoot" class="drm-reader" :class="{ 'is-desk': mode === 'desk' }">
          <nav class="drm-toolbar" aria-label="日报日期导航">
            <button
              type="button"
              :disabled="reader.currentDayIndex.value >= reader.reports.value.length - 1"
              @click="shiftReportPeel(1)"
            >
              <Icon icon="mdi:chevron-left" width="18" />
              较早一期
            </button>
            <span>{{ reader.selectedReport.value ? formatMagazineDate(reader.selectedReport.value.period_date) : '日报' }}</span>
            <button
              type="button"
              :disabled="reader.currentDayIndex.value <= 0"
              @click="shiftReportPeel(-1)"
            >
              较新一期
              <Icon icon="mdi:chevron-right" width="18" />
            </button>
            <button
              type="button"
              class="drm-toolbar__close"
              :aria-label="mode === 'desk' ? '返回版块矩阵' : '关闭日报'"
              @click="closeReader"
            >
              <Icon :icon="mode === 'desk' ? 'mdi:arrow-left' : 'mdi:close'" width="20" />
              <span v-if="mode === 'desk'" class="drm-toolbar__back-label">矩阵</span>
            </button>
            <NuxtLink v-if="mode === 'desk'" to="/" class="drm-toolbar__home" aria-label="返回主页" data-testid="desk-home">
              <Icon icon="mdi:home-outline" width="18" />
              <span class="drm-toolbar__home-label">主页</span>
            </NuxtLink>
          </nav>

          <div v-if="reader.detailLoading.value !== null" class="drm-reader__loading" aria-live="polite">
            <span v-for="index in 3" :key="index" />
          </div>
          <div
            v-else-if="mode === 'desk' && !reader.loading.value && !reader.selectedDetail.value"
            class="drm-desk-empty"
            data-testid="desk-empty"
          >
            <Icon icon="mdi:file-document-outline" width="30" />
            <p>这一天还没有日报。</p>
            <small>{{ props.initialDate || deskToday }} · 可以现在触发生成，或先看最近一期。</small>
            <div class="drm-desk-empty__actions">
              <button type="button" :disabled="generating" data-testid="desk-empty-generate" @click="generateForDesk">
                {{ generating ? '正在触发…' : '生成这一天' }}
              </button>
              <button
                v-if="reader.reports.value.length"
                type="button"
                class="is-ghost"
                data-testid="desk-empty-latest"
                @click="openLatestReport"
              >
                看最近一期
              </button>
            </div>
          </div>

          <!-- Peel 转场容器：始终挂载（reader 开启时），内部文章按 selectedDetail 显隐 + :key 触发方向化翻页 -->
          <PeelTransition :direction="direction" class="drm-peel-host" @end="onPeelEnd">
            <div ref="peelPage" v-if="reader.selectedDetail.value" :key="reader.selectedDetail.value.id" class="drm-peel-page">
              <DailyReportMasthead
                :report="reader.selectedDetail.value"
                :board-title="boardTitle"
                @locate="handleLocateHighlight"
              />
              <div class="drm-layout">
                <DailyReportSidebar
                  :zones="qualityZones"
                  :active-topics="activeTopics"
                  :reports="reader.reports.value"
                  :current-index="reader.currentDayIndex.value"
                  :boards="boards"
                  :board-id="boardId"
                  :today-boards="todayReportBoards"
                  :can-load-more="reader.reports.value.length > 0"
                  @scroll-to="scrollTo"
                  @select-report="selectReportPeel"
                  @load-more="reader.loadMore()"
                  @select-board="handleSwitchBoard"
                  @select-today="emit('openTodayBoard', $event)"
                />
                <main class="drm-content">
                  <DailyReportWatchIndex
                    :hits="watchHitsByReport.get(reader.selectedDetail.value.id) ?? []"
                    :sections="reader.selectedDetail.value.sections"
                    @locate="locateWatchSection"
                  />
                  <DailyReportTopicSection
                    v-for="zone in qualityZones"
                    :key="`${boardId}-${zone.key}`"
                    :zone="zone"
                    :report-date="reader.selectedDetail.value.period_date"
                    :lifeline-entries="reader.lifelineEntries.value"
                    :article-entries="reader.articleEntries.value"
                    :report-details="reader.detailCache.value"
                    :lane-dynamics-entry="getLaneDynamicsEntry()"
                    :context-entries="contextEntries"
                    :focus-section-id="focusSectionId"
                    :locate-thread="locateRequest"
                    :rebucket-section-id="reader.rebucketSectionId.value"
                    :section-aliases="reader.sectionAliases.value"
                    @rebucket="handleRebucket"
                    @ensure-lifeline="reader.ensureLifeline"
                    @ensure-lane-dynamics="ensureLaneDynamics"
                    @ensure-context="ensureContext"
                    @ensure-articles="reader.ensureArticleTitles"
                    @retry-article="reader.retryArticle"
                    @load-historical="loadHistorical"
                    @open-article="emit('openArticle', $event)"
                    @open-detective="openDetectiveWall"
                  />
                  <section v-if="reader.selectedDetail.value.dynamics" class="drm-dynamics">
                    <span>Board Dynamics</span>
                    <h2>板块动态</h2>
                    <p>{{ reader.selectedDetail.value.dynamics }}</p>
                  </section>
                </main>
                <!-- 页边注栏（桌面：第三列 sticky；窄屏 <1100px 收为抽屉） -->
                <aside v-if="!isNotesNarrow" class="drm-notes-rail">
                  <div class="drm-notes-rail__scroller">
                    <MarginNotesRail
                      ref="railRef"
                      :notes="marginNotes.annotations.value"
                      :loading="marginNotes.loading.value"
                      :load-error="marginNotes.loadError.value"
                      :anchor-error="marginNotes.anchorError.value"
                      :changed-ids="changedNoteIds"
                      :lit-id="litNoteId"
                      :ask-states="noteAskStates"
                      @retry-load="marginNotes.load(true)"
                      @jump="jumpToOriginal"
                      @remove="handleRemoveNote"
                      @ask="handleAskNote"
                      @retry-ask="handleRetryAsk"
                      @open-cited="(_annotationId, articleId) => openCitedArticle(articleId)"
                    />
                  </div>
                </aside>
              </div>
              <footer class="drm-colophon" aria-label="本期完">
                <span class="drm-colophon__ornament" aria-hidden="true">◆</span>
                <em>本期脉络由 Syntopica 整理</em>
                <span class="drm-colophon__date">{{ reader.selectedDetail.value ? formatMagazineDate(reader.selectedDetail.value.period_date) : '' }}</span>
              </footer>

              <!-- 划词「问一问」气泡：绝对定位于正文根容器坐标系 -->
              <SelectionAskBubble :state="mnBubble" @confirm="handleBubbleConfirm" />

              <!-- 窄屏（<1100px）：页边注收为右侧抽屉 + 右下浮动入口 -->
              <AppSidebarDrawer :open="notesDrawerOpen" side="right" @close="notesDrawerOpen = false">
                <div v-if="isNotesNarrow" class="drm-notes-drawer">
                  <MarginNotesRail
                    ref="railRef"
                    :notes="marginNotes.annotations.value"
                    :loading="marginNotes.loading.value"
                    :load-error="marginNotes.loadError.value"
                    :anchor-error="marginNotes.anchorError.value"
                    :changed-ids="changedNoteIds"
                    :lit-id="litNoteId"
                    :ask-states="noteAskStates"
                    @retry-load="marginNotes.load(true)"
                    @jump="jumpToOriginal"
                    @remove="handleRemoveNote"
                    @ask="handleAskNote"
                    @retry-ask="handleRetryAsk"
                    @open-cited="(_annotationId, articleId) => openCitedArticle(articleId)"
                  />
                </div>
              </AppSidebarDrawer>
              <button
                v-if="isNotesNarrow && marginNotes.annotations.value.length"
                type="button"
                class="drm-notes-fab"
                data-testid="mn-fab"
                @click="notesDrawerOpen = true"
              >
                <Icon icon="mdi:notebook-outline" width="14" />
                页边注
                <span class="drm-notes-fab__count">{{ marginNotes.annotations.value.length }}</span>
              </button>
            </div>
            <div v-else-if="reader.detailError.value" class="drm-peel-page" data-testid="detail-error-page">
              <div class="drm-layout">
                <DailyReportSidebar
                  :zones="qualityZones"
                  :active-topics="activeTopics"
                  :reports="reader.reports.value"
                  :current-index="reader.currentDayIndex.value"
                  :boards="boards"
                  :board-id="boardId"
                  :today-boards="todayReportBoards"
                  :can-load-more="reader.reports.value.length > 0"
                  @scroll-to="scrollTo"
                  @select-report="selectReportPeel"
                  @load-more="reader.loadMore()"
                  @select-board="handleSwitchBoard"
                  @select-today="emit('openTodayBoard', $event)"
                />
                <main class="drm-content">
                  <div class="drm-reader__error" role="alert">
                    <span>{{ reader.detailError.value }}</span>
                    <button type="button" data-testid="detail-retry" @click="reader.retryDetail()">重试</button>
                  </div>
                </main>

              </div>
            </div>
          </PeelTransition>
        </article>
      </div>
    </Transition>
  </Teleport>

  <TopicDetectiveWall
    v-if="showDetectiveWall"
    :board-id="boardId"
    :initial-topic-id="detectiveTopicId"
    @close="showDetectiveWall = false"
    @open-article="emit('openArticle', $event)"
  />
</template>

<style scoped>
.drt-panel {
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
  margin-top: 1rem;
  padding: 1rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 12px;
  background: var(--color-bg-hover);
}

.drt-header,
.drt-heading,
.drt-summary-card__top {
  display: flex;
  align-items: center;
}

.drt-header {
  justify-content: space-between;
  gap: 1rem;
}

.drt-heading {
  gap: 0.45rem;
  color: var(--color-text-secondary);
}

.drt-heading h2 {
  margin: 0;
  font-size: 0.8rem;
  font-weight: 600;
}

.drt-count {
  padding: 0.1rem 0.45rem;
  border-radius: 999px;
  background: var(--color-bg-active);
  color: var(--color-text-muted);
  font-size: 0.65rem;
}

.drt-browser-toggle,
.drt-more-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.35rem;
  padding: 0.45rem 0.7rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  background: var(--color-bg-elevated);
  color: var(--color-text-secondary);
  font-size: 0.7rem;
  cursor: pointer;
}

.drt-list {
  display: grid;
  gap: 0.65rem;
}

.drt-summary-card {
  display: grid;
  gap: 0.45rem;
  width: 100%;
  padding: 0.9rem 1rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-elevated);
  box-shadow: var(--shadow-subtle);
  color: var(--color-text-primary);
  text-align: left;
  cursor: pointer;
  animation: drtEnter 280ms ease both;
}

.drt-summary-card:hover,
.drt-summary-card:focus-visible {
  border-color: var(--color-border-strong);
  box-shadow: var(--shadow-medium);
  outline: none;
}

.drt-summary-card__top {
  justify-content: space-between;
  color: var(--color-text-muted);
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.65rem;
}

.drt-summary-card strong {
  font-family: "Noto Serif SC", serif;
  font-size: 0.84rem;
  line-height: 1.5;
}

.drt-summary-card small {
  color: var(--color-text-muted);
  font-size: 0.66rem;
}

.drt-summary-card__open {
  display: grid;
  gap: 0.45rem;
}

.drt-watch-preview {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.35rem;
  padding-top: 0.1rem;
}

.drt-watch-preview__tag,
.drt-watch-preview__more {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
  min-width: 0;
  max-width: 14rem;
  padding: 0.2rem 0.45rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 999px;
  background: var(--color-bg-hover);
  color: var(--color-text-secondary);
  font-size: 0.66rem;
  line-height: 1.3;
  text-overflow: ellipsis;
  white-space: nowrap;
  overflow: hidden;
}

.drt-watch-preview__tag {
  cursor: pointer;
}

.drt-watch-preview__tag:hover,
.drt-watch-preview__tag:focus-visible {
  border-color: var(--color-accent);
  color: var(--color-text-primary);
  outline: none;
}

.drt-watch-preview__tag[data-type="keyword"] > span {
  color: var(--color-text-muted);
}

.drt-watch-preview__tag[data-type="label"] > span {
  color: var(--color-accent);
}

.drt-watch-preview__more {
  border-style: dashed;
  color: var(--color-text-muted);
}

.drt-status[data-status="done"] { color: var(--color-success); }
.drt-status[data-status="generating"],
.drt-status[data-status="pending"] { color: var(--color-warning); }
.drt-status[data-status="failed"] { color: var(--color-error); }

.drt-loading,
.drm-reader__loading {
  display: grid;
  gap: 0.65rem;
}

.drt-skeleton,
.drm-reader__loading span {
  height: 4.5rem;
  border-radius: 8px;
  background: var(--color-bg-active);
  animation: drtPulse 1.3s ease-in-out infinite;
}

.drt-empty {
  display: grid;
  place-items: center;
  gap: 0.35rem;
  padding: 2.5rem 1rem;
  color: var(--color-text-muted);
  text-align: center;
}

.drt-empty p { margin: 0; }
.drt-empty small { max-width: 19rem; line-height: 1.6; }
.drt-more-btn { align-self: center; }

.drm-overlay {
  position: fixed;
  inset: 0;
  z-index: 9000;
  background: var(--color-bg-base);
}

.drm-reader {
  position: relative;
  width: 100%;
  height: 100%;
  overflow-y: auto;
  background: var(--color-bg-base);
  background-image: radial-gradient(ellipse at 30% 0%, color-mix(in srgb, var(--color-accent) 10%, transparent), transparent 60%);
  color: var(--color-text-primary);
  scrollbar-color: var(--color-border-strong) transparent;
  scrollbar-width: thin;
}

.drm-reader::before {
  content: none;
}

.drm-reader > * {
  position: relative;
  z-index: 1;
}

.drm-toolbar {
  position: sticky;
  top: 0;
  z-index: 20;
  display: grid;
  grid-template-columns: auto 1fr auto auto auto;
  align-items: center;
  min-height: 3.5rem;
  padding: 0 1rem;
  border-bottom: 1px solid var(--color-border-medium);
  background: var(--color-bg-elevated);
  box-shadow: var(--shadow-subtle);
}

.drm-toolbar button {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  min-height: 2.25rem;
  border: 0;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 0.72rem;
  cursor: pointer;
}

.drm-toolbar button:disabled {
  opacity: 0.3;
  cursor: default;
}

.drm-toolbar button:focus-visible {
  outline: 2px solid var(--color-input-focus);
  outline-offset: 2px;
}

.drm-toolbar > span {
  color: var(--color-text-muted);
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.65rem;
  text-align: center;
}

.drm-toolbar__close {
  justify-content: center;
  width: 2.5rem;
  margin-left: 0.5rem;
  border-left: 1px solid var(--color-border-medium) !important;
}

.drm-reader.is-desk .drm-toolbar__close {
  width: auto;
  min-width: 2.5rem;
  padding: 0 0.55rem;
  gap: 0.2rem;
  white-space: nowrap;
}

.drm-toolbar__home {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.2rem;
  min-height: 2.25rem;
  margin-left: 0.5rem;
  padding: 0 0.45rem;
  border-left: 1px solid var(--color-border-medium);
  color: var(--color-text-secondary);
  font-size: 0.72rem;
  text-decoration: none;
  white-space: nowrap;
}

.drm-toolbar__home:hover,
.drm-toolbar__home:focus-visible {
  color: var(--color-accent);
  outline: none;
}

.drm-peel-host {
  position: relative;
  perspective: 1400px;
}

.drm-peel-page {
  position: relative;
  z-index: 1;
  backface-visibility: hidden;
}

.drm-layout {
  display: grid;
  grid-template-columns: 14rem minmax(0, 1fr) clamp(15rem, 17vw, 17rem);
  gap: clamp(2rem, 3vw, 2.75rem);
  width: 100%;
  margin: 0 auto;
  padding: clamp(2rem, 5vw, 5rem) clamp(1rem, 4vw, 4rem) 6rem;
}

/* 页边注栏（第三列）：sticky + 栏内滚动，随阅读层滚动常驻 */
/* 页边注栏（桌面：第三列）：视口内垂直居中 + 内容自适应高度，不顶格、不被阅读层工具条压住。
   外层 sticky 铺满「工具条之下 → 视口底部」做定位与居中（内容短 → 卡片堆整体居中留白）；
   内层才是滚动容器（内容长 → 内层滚动），避开 flex/grid 居中 + overflow 同时用导致的
   顶部内容被截断且滚不到（经典 safe-center 坑，`safe` 关键字跨浏览器支持不稳）。 */
.drm-notes-rail {
  --drm-notes-top: 4.25rem; /* 工具条 min-height 3.5rem + 呼吸 */
  --drm-notes-bottom: 0.75rem;
  position: sticky;
  top: var(--drm-notes-top);
  align-self: start;
  display: flex;
  flex-direction: column;
  justify-content: center;
  height: calc(100vh - var(--drm-notes-top) - var(--drm-notes-bottom));
  min-width: 0;
}

/* 自适应高度：随内容伸缩，超限才栏内滚动（滚动条只在需要时出现） */
.drm-notes-rail__scroller {
  max-height: 100%;
  min-height: 0;
  overflow-y: auto;
  padding-right: 0.2rem;
  scrollbar-width: thin;
  scrollbar-color: var(--color-border-strong) transparent;
}

/* 批注高亮 mark：由锚定器动态注入，不带 scoped 属性，用 :global 声明（主题 token 双主题跟随） */
:global(mark.mn-highlight) {
  background: linear-gradient(transparent 55%, color-mix(in srgb, var(--color-accent) 18%, transparent) 55%);
  color: inherit;
  padding: 0 0.08em;
  cursor: pointer;
  border-bottom: 1px dashed color-mix(in srgb, var(--color-accent) 55%, transparent);
  transition: background 0.25s;
}

:global(mark.mn-highlight:hover) {
  background: linear-gradient(transparent 40%, color-mix(in srgb, var(--color-accent) 32%, transparent) 40%);
}

:global(mark.mn-highlight::after) {
  content: "❧";
  font-size: 0.62em;
  color: var(--color-accent);
  vertical-align: super;
  margin-left: 0.12em;
  opacity: 0.8;
}

@keyframes mnFlash {
  0%, 100% { background: linear-gradient(transparent 40%, color-mix(in srgb, var(--color-accent) 32%, transparent) 40%); }
  50% { background: linear-gradient(transparent 30%, color-mix(in srgb, var(--color-accent) 50%, transparent) 30%); }
}

:global(mark.mn-highlight.mn-flash) {
  animation: mnFlash 1.2s ease 2;
}

/* 窄屏右下浮动入口（specs 窄屏抽屉形态：<1100px 且有批注时出现） */
.drm-notes-fab {
  position: fixed;
  right: 1rem;
  bottom: 1.2rem;
  z-index: 30;
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.55rem 1rem;
  border: 0;
  border-radius: 999px;
  background: var(--color-text-primary);
  color: var(--color-bg-base);
  font-size: 0.78rem;
  font-weight: 600;
  letter-spacing: 0.08em;
  cursor: pointer;
  box-shadow: var(--shadow-strong);
}

.drm-notes-fab__count {
  padding: 0 0.4rem;
  border-radius: 999px;
  background: var(--color-accent);
  color: #fff;
  font-size: 0.66rem;
}

.drm-notes-drawer {
  height: 100%;
  overflow-y: auto;
  padding: 0.25rem;
}

.drm-content {
  min-width: 0;
}

.drm-dynamics {
  padding: 2rem 0;
  border-top: 3px double var(--color-border-strong);
  font-family: "Noto Serif SC", serif;
  animation: drmInkFade 0.7s cubic-bezier(0.2, 0.7, 0.3, 1) both;
  animation-delay: 0.26s;
}

.drm-colophon {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  max-width: 82rem;
  margin: 0 auto;
  padding: 1.5rem clamp(1rem, 4vw, 4rem) 3.5rem;
  border-top: 3px double var(--color-border-strong);
  color: var(--color-text-muted);
  font-style: italic;
  font-size: 0.78rem;
  animation: drmInkFade 0.7s cubic-bezier(0.2, 0.7, 0.3, 1) both;
  animation-delay: 0.32s;
}

.drm-colophon__ornament {
  color: var(--color-accent);
  font-size: 1rem;
  font-style: normal;
}

.drm-colophon__date {
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.68rem;
  font-style: normal;
}

.drm-dynamics > span {
  color: var(--color-accent);
  font-size: 0.7rem;
  font-style: italic;
  letter-spacing: 0.12em;
}

.drm-dynamics h2 {
  margin: 0.35rem 0 0.75rem;
  font-size: 1.8rem;
}

.drm-dynamics p {
  margin: 0;
  color: var(--color-text-secondary);
  line-height: 1.9;
  white-space: pre-line;
}

.drm-reader__loading {
  max-width: 60rem;
  margin: 8rem auto;
  padding: 0 2rem;
}

.drm-reader__error {
  margin: 8rem auto;
  color: var(--color-error);
  text-align: center;
}

.drm-reader-transition-enter-active,
.drm-reader-transition-leave-active {
  transition: opacity 180ms ease;
}

.drm-reader-transition-enter-from,
.drm-reader-transition-leave-to {
  opacity: 0;
}

@keyframes drtPulse {
  0%, 100% { opacity: 0.45; }
  50% { opacity: 0.85; }
}

@keyframes drtEnter {
  from { opacity: 0; transform: translateY(5px); }
  to { opacity: 1; transform: translateY(0); }
}

@keyframes drmInkFade {
  from { opacity: 0; transform: translateY(10px); }
  to { opacity: 1; transform: translateY(0); }
}

@media (max-width: 1100px) {
  .drm-layout {
    grid-template-columns: 1fr;
    gap: 1rem;
  }

  .drm-notes-rail {
    display: none;
  }
}

@media (max-width: 720px) {
  .drm-toolbar {
    grid-template-columns: auto 1fr auto auto;
    padding: 0 0.5rem;
  }

  .drm-toolbar > span {
    display: none;
  }

  .drm-toolbar > button:not(.drm-toolbar__close) {
    justify-content: center;
    width: 2.5rem;
    font-size: 0;
  }

  .drm-toolbar__close {
    grid-column: 3;
    grid-row: 1;
  }

  .drm-toolbar__home {
    grid-column: 4;
    grid-row: 1;
    width: 2.5rem;
    margin-left: 0;
    padding: 0;
  }

  .drm-toolbar__home-label {
    display: none;
  }

  .drm-layout {
    padding-inline: 0.8rem;
  }

  .drm-colophon {
    flex-direction: column;
    gap: 0.5rem;
    text-align: center;
  }
}

@media (prefers-reduced-motion: reduce) {
  .drt-summary-card,
  .drt-skeleton,
  .drm-reader__loading span,
  .drm-dynamics,
  .drm-colophon,
  .drm-reader-transition-enter-active,
  .drm-reader-transition-leave-active {
    animation: none;
    transition: none;
  }
}

/* ── 阅读台（/tags/:boardId/:date）：沿用旧版阅读层三栏布局 ──────────────
   2026-10-27 用户复审：恢复「大刊头 + 今日重点 + 左目录 + 宽屏正文 + 右页边注」，
   不再用窄 40rem 两栏 + 页边注抽屉。 */
.drm-overlay.is-desk {
  position: relative;
  inset: auto;
  z-index: auto;
  height: 100%;
  min-height: 0;
}

.drm-toolbar__back-label {
  font-size: 0.66rem;
  letter-spacing: 0.08em;
}

.drm-desk-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.55rem;
  padding: 5rem 1rem;
  color: var(--color-text-muted);
  text-align: center;
}

.drm-desk-empty p {
  margin: 0;
  color: var(--color-text-primary);
  font-family: var(--font-serif);
  font-size: 1.1rem;
}

.drm-desk-empty small {
  font-size: 0.72rem;
}

.drm-desk-empty__actions {
  display: flex;
  gap: 0.5rem;
  margin-top: 0.4rem;
}

.drm-desk-empty__actions button {
  padding: 0.45rem 0.8rem;
  border: 1px solid var(--color-accent);
  border-radius: 5px;
  background: var(--color-accent);
  color: #fff;
  font-size: 0.75rem;
  cursor: pointer;
}

.drm-desk-empty__actions button.is-ghost {
  background: transparent;
  color: var(--color-accent);
}

.drm-desk-empty__actions button:disabled {
  opacity: 0.6;
  cursor: default;
}
</style>
