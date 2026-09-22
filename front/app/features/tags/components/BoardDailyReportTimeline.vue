<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, toRef, watch } from 'vue'
import { Icon } from '@iconify/vue'
import BoardThreadBrowser from './BoardThreadBrowser.vue'
import TopicDetectiveWall from './TopicDetectiveWall.client.vue'
import DailyReportMasthead from './daily-report/DailyReportMasthead.vue'
import DailyReportSidebar from './daily-report/DailyReportSidebar.vue'
import DailyReportTopicSection from './daily-report/DailyReportTopicSection.vue'
import DailyReportWatchIndex from './daily-report/DailyReportWatchIndex.vue'
import PeelTransition from '~/components/PeelTransition.vue'
import {
  buildQualityZones,
  formatMagazineDate,
  groupSectionsByTopic,
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
}>(), {
  boards: () => [],
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
            <div v-if="reader.selectedDetail.value" :key="reader.selectedDetail.value.id" class="drm-peel-page">
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
              </div>
              <footer class="drm-colophon" aria-label="本期完">
                <span class="drm-colophon__ornament" aria-hidden="true">◆</span>
                <em>本期脉络由 Syntopica 整理</em>
                <span class="drm-colophon__date">{{ reader.selectedDetail.value ? formatMagazineDate(reader.selectedDetail.value.period_date) : '' }}</span>
              </footer>
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
  grid-template-columns: 14rem minmax(0, 1fr);
  gap: clamp(2rem, 3vw, 2.75rem);
  width: 100%;
  margin: 0 auto;
  padding: clamp(2rem, 5vw, 5rem) clamp(1rem, 4vw, 4rem) 6rem;
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
