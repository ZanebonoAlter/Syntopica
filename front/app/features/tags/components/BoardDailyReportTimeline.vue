<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, toRef, watch } from 'vue'
import { Icon } from '@iconify/vue'
import BoardThreadBrowser from './BoardThreadBrowser.vue'
import TopicDetectiveWall from './TopicDetectiveWall.client.vue'
import DailyReportMasthead from './daily-report/DailyReportMasthead.vue'
import DailyReportSidebar from './daily-report/DailyReportSidebar.vue'
import DailyReportTopicSection from './daily-report/DailyReportTopicSection.vue'
import DailyReportWatchIndex from './daily-report/DailyReportWatchIndex.vue'
import MarginNotesRail from './daily-report/MarginNotesRail.vue'
import SelectionAskBubble from './daily-report/SelectionAskBubble.vue'
import AppSidebarDrawer from '~/components/ui/AppSidebarDrawer.vue'
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
import { useLaneTrendData } from '~/features/tags/composables/useLaneTrendData'
import type { PeelDirection } from '~/composables/usePeelTransition'
import { useTopicWatchesApi, type TopicWatchHit } from '~/api/topicWatches'
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
}>(), {
  boards: () => [],
  initialReportId: null,
  initialAnnotationId: null,
})

const emit = defineEmits<{
  openArticle: [articleId: number]
  /** 就近切换版块（透传给 TagsPage 调 handleSelectBoard）。 */
  selectBoard: [boardId: number]
}>()

/** 当前转场方向：切版块→纵向，切日期→横向。须在触发 key 变更前同步写入。 */
const direction = ref<PeelDirection>('horizontal')
/** 动画进行中锁，防越界连点。 */
const animating = ref(false)

const reader = useDailyReportReader(toRef(props, 'boardId'))
// 泳道趋势区取数（lane-trend-overview §D4）：板块级 lane-dynamics 页面级缓存（首个泳道展开触发、
// 翻期不重拉）+ contexts 月/年按需缓存；均为只读，不随阅读报告日期变化（趋势=现在）。
const { contextEntries, ensureLaneDynamics, ensureContext, getLaneDynamicsEntry } = useLaneTrendData(toRef(props, 'boardId'))
const watchesApi = useTopicWatchesApi()
const watchHitsByReport = ref(new Map<number, TopicWatchHit[]>())
const focusSectionId = ref<number | null>(null)
const showReader = ref(false)
const showThreadBrowser = ref(false)
const showDetectiveWall = ref(false)
const detectiveTopicId = ref<number | undefined>()
const lastTrigger = ref<HTMLElement | null>(null)
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

onMounted(async () => {
  // 深链（管理页「跳原日报」，MG-5）：开阅读层 → 定位报告 → 定位批注闪现
  if (!props.initialReportId) return
  lastTrigger.value = null
  showReader.value = true
  await reader.selectReportById(props.initialReportId)
  const detail = reader.selectedDetail.value
  if (detail) await ensureWatchHits(detail.id, detail)
  if (props.initialAnnotationId != null) pendingFocusNoteId.value = props.initialAnnotationId
})

const reportStatusLabel: Record<string, string> = {
  done: '完成',
  generating: '生成中',
  pending: '待生成',
  failed: '失败',
}

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

async function openReader(event: MouseEvent, index: number) {
  lastTrigger.value = event.currentTarget as HTMLElement
  showReader.value = true
  focusSectionId.value = null
  await reader.selectReport(index)
  const detail = reader.selectedDetail.value
  if (detail) await ensureWatchHits(detail.id, detail)
}

function openReaderFromKeyboard(event: KeyboardEvent, index: number) {
  void openReader(event as unknown as MouseEvent, index)
}

async function openWatchPreview(event: MouseEvent, index: number, summary: ActiveWatchSummary) {
  await openReader(event, index)
  const report = reader.selectedReport.value
  if (!report) return
  const hit = (watchHitsByReport.value.get(report.id) ?? []).find(item => item.watchId === String(summary.watchId))
  if (hit) locateWatchSection(hit.sectionId)
}

async function closeReader() {
  showReader.value = false
  await nextTick()
  lastTrigger.value?.focus()
}

async function selectReport(index: number) {
  focusSectionId.value = null
  await reader.selectReport(index)
  const detail = reader.selectedDetail.value
  if (detail) await ensureWatchHits(detail.id, detail)
  document.querySelector('.drm-reader')?.scrollTo({ top: 0, behavior: 'smooth' })
}

async function shiftReport(offset: number) {
  focusSectionId.value = null
  await reader.shiftReport(offset)
  const detail = reader.selectedDetail.value
  if (detail) await ensureWatchHits(detail.id, detail)
  document.querySelector('.drm-reader')?.scrollTo({ top: 0, behavior: 'smooth' })
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
    if (notesDrawerOpen.value) return
    event.preventDefault()
    void closeReader()
  } else if (event.key === 'ArrowDown' || event.key === 'ArrowRight') {
    event.preventDefault()
    shiftReportPeel(1)
  } else if (event.key === 'ArrowUp' || event.key === 'ArrowLeft') {
    event.preventDefault()
    shiftReportPeel(-1)
  }
}

watch(showReader, (open) => {
  if (open) {
    previousBodyOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    document.addEventListener('keydown', handleKeydown)
  } else {
    document.body.style.overflow = previousBodyOverflow
    document.removeEventListener('keydown', handleKeydown)
  }
})

watch(() => props.boardId, () => {
  // 不关闭 reader：版块切换发生在 reader 内部（就近切换条），需保持开启以播放纵向翻页转场。
  // reader 打开时 modal 覆盖侧栏，外部切换不可能发生；reader 关闭时此项为 no-op。
  showThreadBrowser.value = false
  showDetectiveWall.value = false
  detectiveTopicId.value = undefined
})

// 版块切换后（reader 开着）日报列表重载完成时自动选中第一天，触发纵向翻页进入转场。
watch(reader.loading, async (isLoading) => {
  if (isLoading) return
  if (!showReader.value || reader.currentDayIndex.value >= 0) return
  if (reader.reports.value.length > 0) {
    await reader.selectReport(0)
  } else {
    // 新版块无日报：无进入转场，兏底释放动画锁
    animating.value = false
  }
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown)
  document.body.style.overflow = previousBodyOverflow
  if (litTimer) clearTimeout(litTimer)
})
</script>

<template>
  <section class="drt-panel" aria-labelledby="daily-report-panel-title">
    <header class="drt-header">
      <div class="drt-heading">
        <Icon icon="mdi:newspaper-variant-outline" width="16" />
        <h2 id="daily-report-panel-title">板块日报</h2>
        <span v-if="reader.reports.value.length" class="drt-count">{{ reader.reports.value.length }}</span>
      </div>
      <button type="button" class="drt-browser-toggle" @click="showThreadBrowser = !showThreadBrowser">
        <Icon :icon="showThreadBrowser ? 'mdi:newspaper-variant-outline' : 'mdi:chart-timeline-variant'" width="15" />
        {{ showThreadBrowser ? '日报列表' : '话题总览' }}
      </button>
    </header>

    <BoardThreadBrowser
      v-if="showThreadBrowser"
      :board-id="boardId"
      @open-article="emit('openArticle', $event)"
      @open-detective-wall="openDetectiveWall()"
    />

    <template v-else>
      <div v-if="reader.loading.value" class="drt-loading" aria-live="polite">
        <div v-for="index in 2" :key="index" class="drt-skeleton" />
      </div>
      <div v-else-if="!reader.reports.value.length" class="drt-empty">
        <Icon icon="mdi:file-document-outline" width="28" />
        <p>日报需要积累数据</p>
        <small>系统会按板块聚合每日热点，数据积累后日报会自动生成。</small>
      </div>
      <div v-else class="drt-list">
        <article
          v-for="(report, index) in reader.reports.value"
          :key="report.id"
          class="drt-summary-card"
          role="button"
          tabindex="0"
          :style="{ animationDelay: `${index * 50}ms` }"
          @click="openReader($event, index)"
          @keydown.enter="openReaderFromKeyboard($event, index)"
          @keydown.space.prevent="openReaderFromKeyboard($event, index)"
        >
          <div class="drt-summary-card__open">
            <span class="drt-summary-card__top">
              <span>{{ formatMagazineDate(report.period_date) }}</span>
              <span class="drt-status" :data-status="report.status">{{ reportStatusLabel[report.status] || report.status }}</span>
            </span>
            <strong>{{ report.summary || report.title }}</strong>
            <small>{{ report.article_count }} 篇 · {{ report.cluster_count }} 话题</small>
          </div>
          <div
            v-if="report.activeWatchSummaries?.length"
            class="drt-watch-preview"
            data-testid="watch-preview"
            aria-label="本期追踪命中"
          >
            <button
              v-for="summary in report.activeWatchSummaries.slice(0, 2)"
              :key="String(summary.watchId)"
              type="button"
              class="drt-watch-preview__tag"
              :data-type="summary.type"
              @click.stop="openWatchPreview($event, index, summary)"
            >
              <span aria-hidden="true">{{ summary.type === 'keyword' ? '#' : '✦' }}</span>
              {{ summary.label }}
            </button>
            <span v-if="report.activeWatchSummaries.length > 2" class="drt-watch-preview__more">
              +{{ report.activeWatchSummaries.length - 2 }}
            </span>
          </div>
        </article>
      </div>
      <button v-if="reader.reports.value.length" type="button" class="drt-more-btn" @click="reader.loadMore">
        加载更早
      </button>
    </template>
  </section>

  <Teleport to="body">
    <Transition name="drm-reader-transition">
      <div
        v-if="showReader"
        class="drm-overlay"
        role="dialog"
        aria-modal="true"
        aria-label="日报详情"
      >
        <article class="drm-reader">
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
            <button type="button" class="drm-toolbar__close" aria-label="关闭日报" @click="closeReader">
              <Icon icon="mdi:close" width="20" />
            </button>
          </nav>

          <div v-if="reader.detailLoading.value !== null" class="drm-reader__loading" aria-live="polite">
            <span v-for="index in 3" :key="index" />
          </div>
          <div v-else-if="reader.detailError.value" class="drm-reader__error" role="alert">
            {{ reader.detailError.value }}
          </div>

          <!-- Peel 转场容器：始终挂载（reader 开启时），内部文章按 selectedDetail 显隐 + :key 触发方向化翻页 -->
          <PeelTransition :direction="direction" class="drm-peel-host" @end="onPeelEnd">
            <div ref="peelPage" v-if="reader.selectedDetail.value" :key="reader.selectedDetail.value.id" class="drm-peel-page">
              <DailyReportMasthead :report="reader.selectedDetail.value" :board-title="boardTitle" />
              <div class="drm-layout">
                <DailyReportSidebar
                  :zones="qualityZones"
                  :active-topics="activeTopics"
                  :reports="reader.reports.value"
                  :current-index="reader.currentDayIndex.value"
                  :boards="boards"
                  :board-id="boardId"
                  @scroll-to="scrollTo"
                  @select-report="selectReportPeel"
                  @select-board="handleSwitchBoard"
                  @open-topic-overview="showThreadBrowser = true; closeReader()"
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
                <!-- 页边注栏（桌面：第三列 sticky 视口内居中 + 内容自适应高度；窄屏收为抽屉） -->
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
  grid-template-columns: auto 1fr auto auto;
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
    grid-template-columns: auto 1fr auto;
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
</style>
