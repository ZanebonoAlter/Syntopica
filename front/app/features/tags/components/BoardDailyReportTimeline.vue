<script setup lang="ts">
import { ref, watch, computed, onMounted, onUnmounted } from 'vue'
import { Icon } from '@iconify/vue'
import { useFloating } from '@floating-ui/vue'
import { autoUpdate, offset, shift, flip } from '@floating-ui/dom'
import { useDailyReportsApi, type DailyReportListItem, type DailyReport, type DailyReportThread } from '~/api/dailyReports'
import { useArticlesApi } from '~/api/articles'
import SectionLifecyclePanel from './SectionLifecyclePanel.vue'
import ThreadLineagePanel from './ThreadLineagePanel.vue'
import BoardThreadBrowser from './BoardThreadBrowser.vue'

const props = defineProps<{ boardId: number }>()

const emit = defineEmits<{
  openArticle: [articleId: number]
}>()

const { getBoardDailyReports, getDailyReportDetail } = useDailyReportsApi()
const { getArticle } = useArticlesApi()

const reports = ref<DailyReportListItem[]>([])
const days = ref(7)
const loading = ref(false)
const showModal = ref(false)
const currentDayIndex = ref(-1)
const detailCache = ref<Map<number, DailyReport>>(new Map())
const detailLoading = ref<number | null>(null)

const selectedReport = computed(() => {
  if (currentDayIndex.value < 0 || currentDayIndex.value >= reports.value.length) return null
  return reports.value[currentDayIndex.value]
})

const selectedDetail = computed<DailyReport | null>(() => {
  if (!selectedReport.value) return null
  return detailCache.value.get(selectedReport.value.id) ?? null
})

interface QualityZone {
  label: string
  tier: number
  sections: any[]
  columns: number
}

const qualityZones = computed<QualityZone[]>(() => {
  if (!selectedDetail.value) return []
  const sections = [...(selectedDetail.value.sections || [])].sort((a, b) => {
    if (a.best_tier !== b.best_tier) return a.best_tier - b.best_tier
    return b.avg_score - a.avg_score
  })
  if (sections.length === 0) return []

  const zones: QualityZone[] = []

  // Tier 0-1: Core events (2 columns)
  const core = sections.filter(s => s.best_tier <= 1)
  if (core.length) zones.push({ label: '核心事件', tier: 1, sections: core, columns: 2 })

  // Tier 2: Related events (single column)
  const related = sections.filter(s => s.best_tier === 2)
  if (related.length) zones.push({ label: '相关事件', tier: 2, sections: related, columns: 1 })

  // Tier 3+: Other dynamics (single column)
  const other = sections.filter(s => s.best_tier >= 3)
  if (other.length) zones.push({ label: '其他动态', tier: 3, sections: other, columns: 1 })

  return zones
})

const sectionStatusColor: Record<string, string> = {
  emerging: 'np-section-emerging',
  continuing: 'np-section-continuing',
  ending: 'np-section-ending',
}

const sectionStatusLabel: Record<string, string> = {
  emerging: '新兴',
  continuing: '持续',
  ending: '结束',
}

const reportStatusStyle: Record<string, string> = {
  done: 'bg-green-900/40 text-green-400',
  generating: 'bg-yellow-900/40 text-yellow-400',
  pending: 'bg-gray-800/40 text-gray-400',
  failed: 'bg-red-900/40 text-red-400',
}

const reportStatusLabel: Record<string, string> = {
  done: '完成',
  generating: '生成中',
  pending: '待生成',
  failed: '失败',
}

// Thread article popup state
const threadPopupTrigger = ref<HTMLElement>()
const threadPopupFloating = ref<HTMLElement>()
const threadPopupOpen = ref(false)
const threadPopupArticles = ref<Array<{ id: number; title: string; loading: boolean }>>([])
const threadPopupLoading = ref(false)
let currentOpenThread: DailyReportThread | null = null

// Section lifecycle panel state
const lifecycleSectionId = ref<number | null>(null)
const lifecycleVisible = ref(false)

// Thread lineage panel state
const lineageThreadId = ref<number | null>(null)
const lineageVisible = ref(false)

// Section expand state
const expandedSections = ref<Set<number>>(new Set())

// Thread browser toggle
const showThreadBrowser = ref(false)

const { floatingStyles: threadPopupStyles } = useFloating(threadPopupTrigger, threadPopupFloating, {
  placement: 'right-start',
  middleware: [offset(8), shift({ padding: 16 }), flip()],
  whileElementsMounted: autoUpdate,
})

const hasMoreArticles = computed(() => {
  if (!currentOpenThread) return false
  return threadPopupArticles.value.length < (currentOpenThread.related_article_ids?.length || 0)
})

async function loadReports() {
  loading.value = true
  try {
    const res = await getBoardDailyReports(props.boardId, { days: days.value })
    if (res.success && res.data) {
      reports.value = res.data.reports || []
    } else {
      reports.value = []
    }
  } finally {
    loading.value = false
  }
}

async function openNewspaper(index: number) {
  const report = reports.value[index]
  if (!report) return
  currentDayIndex.value = index
  showModal.value = true
  if (detailCache.value.has(report.id)) return
  detailLoading.value = report.id
  try {
    const res = await getDailyReportDetail(report.id)
    if (res.success && res.data) {
      detailCache.value.set(report.id, res.data.report)
      detailCache.value = new Map(detailCache.value)
    }
  } finally {
    detailLoading.value = null
  }
}

function openSectionLifecycle(section: any) {
  lifecycleSectionId.value = section.id
  lifecycleVisible.value = true
}

function closeSectionLifecycle() {
  lifecycleVisible.value = false
  lifecycleSectionId.value = null
}

function openThreadLineage(thread: DailyReportThread) {
  lineageThreadId.value = thread.id
  lineageVisible.value = true
}

function closeThreadLineage() {
  lineageVisible.value = false
  lineageThreadId.value = null
}

function toggleSectionExpand(clusterIndex: number) {
  const next = new Set(expandedSections.value)
  if (next.has(clusterIndex)) {
    next.delete(clusterIndex)
  } else {
    next.add(clusterIndex)
  }
  expandedSections.value = next
}

function navigateToSectionReport(node: { report_id: number }) {
  const idx = reports.value.findIndex(r => r.id === node.report_id)
  if (idx >= 0 && idx !== currentDayIndex.value) {
    currentDayIndex.value = idx
    loadDetailForCurrentDay()
  }
}

function closeNewspaper() {
  showModal.value = false
  closeThreadPopup()
  closeSectionLifecycle()
  closeThreadLineage()
}

function prevDay() {
  if (currentDayIndex.value > 0) {
    currentDayIndex.value--
    loadDetailForCurrentDay()
    closeSectionLifecycle()
  }
}

function nextDay() {
  if (currentDayIndex.value < reports.value.length - 1) {
    currentDayIndex.value++
    loadDetailForCurrentDay()
    closeSectionLifecycle()
  }
}

async function loadDetailForCurrentDay() {
  const report = reports.value[currentDayIndex.value]
  if (!report) return
  if (detailCache.value.has(report.id)) return
  detailLoading.value = report.id
  try {
    const res = await getDailyReportDetail(report.id)
    if (res.success && res.data) {
      detailCache.value.set(report.id, res.data.report)
      detailCache.value = new Map(detailCache.value)
    }
  } finally {
    detailLoading.value = null
  }
}

function loadMore() {
  days.value += 7
  loadReports()
}

const weekDays = ['日', '一', '二', '三', '四', '五', '六']

function formatDateForSummary(dateStr: string): string {
  const d = new Date(dateStr)
  const month = d.getMonth() + 1
  const day = d.getDate()
  const weekDay = weekDays[d.getDay()]
  return `${month}.${day}  星期${weekDay}`
}

function truncateText(text: string, maxLen: number): string {
  if (!text) return ''
  return text.length > maxLen ? text.slice(0, maxLen) + '...' : text
}

async function openThreadArticles(event: MouseEvent, thread: DailyReportThread) {
  threadPopupTrigger.value = event.currentTarget as HTMLElement
  threadPopupOpen.value = true
  threadPopupLoading.value = true
  threadPopupArticles.value = []
  currentOpenThread = thread

  const ids = thread.related_article_ids || []
  const firstBatch = ids.slice(0, 5)

  if (firstBatch.length === 0) {
    threadPopupLoading.value = false
    return
  }

  const results = await Promise.allSettled(
    firstBatch.map(id => getArticle(id))
  )

  threadPopupArticles.value = results.map((r, i) => {
    const articleId = firstBatch[i]!
    if (r.status === 'fulfilled' && r.value.success && r.value.data) {
      return { id: articleId, title: r.value.data.title || '(无标题)', loading: false }
    }
    return { id: articleId, title: `文章 #${articleId}`, loading: false }
  })
  threadPopupLoading.value = false
}

async function loadMoreThreadArticles() {
  if (!currentOpenThread) return
  const ids = currentOpenThread.related_article_ids || []
  const start = threadPopupArticles.value.length
  const nextBatch = ids.slice(start, start + 5)
  if (nextBatch.length === 0) return

  // Add loading placeholders
  nextBatch.forEach(id => {
    threadPopupArticles.value.push({ id, title: '加载中...', loading: true })
  })

  const results = await Promise.allSettled(
    nextBatch.map(id => getArticle(id))
  )

  // Replace placeholders with actual data
  results.forEach((r, i) => {
    const idx = start + i
    if (r.status === 'fulfilled' && r.value.success && r.value.data) {
      const articleId = nextBatch[i]!
      threadPopupArticles.value[idx] = { id: articleId, title: r.value.data.title || '(无标题)', loading: false }
    } else {
      const articleId = nextBatch[i]!
      threadPopupArticles.value[idx] = { id: articleId, title: `文章 #${articleId}`, loading: false }
    }
  })
}

function closeThreadPopup() {
  threadPopupOpen.value = false
  threadPopupArticles.value = []
  currentOpenThread = null
}

function handleArticleClick(articleId: number) {
  emit('openArticle', articleId)
  closeThreadPopup()
}

function handleThreadPopupOutsideClick(event: MouseEvent) {
  if (threadPopupOpen.value && threadPopupTrigger.value && threadPopupFloating.value) {
    const target = event.target as Node
    if (!threadPopupTrigger.value.contains(target) && !threadPopupFloating.value.contains(target)) {
      closeThreadPopup()
    }
  }
}

function handleKeydown(e: KeyboardEvent) {
  if (!showModal.value) return
  switch (e.key) {
    case 'ArrowUp': e.preventDefault(); prevDay(); break
    case 'ArrowDown': e.preventDefault(); nextDay(); break
    case 'Escape': closeNewspaper(); break
  }
}

watch(showModal, (val) => {
  if (val) {
    document.addEventListener('keydown', handleKeydown)
    document.body.style.overflow = 'hidden'
  } else {
    document.removeEventListener('keydown', handleKeydown)
    document.body.style.overflow = ''
  }
})

onMounted(() => {
  document.addEventListener('click', handleThreadPopupOutsideClick)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown)
  document.removeEventListener('click', handleThreadPopupOutsideClick)
  document.body.style.overflow = ''
})

watch(() => props.boardId, () => {
  days.value = 7
  showModal.value = false
  currentDayIndex.value = -1
  detailCache.value = new Map()
  showThreadBrowser.value = false
  loadReports()
}, { immediate: true })
</script>

<template>
  <div class="drt-panel">
    <!-- LIST VIEW (always shown; modal is separate via Teleport) -->
    <div>
      <div class="drt-header">
        <Icon icon="mdi:file-document-outline" width="15" class="text-white/50" />
        <span class="drt-title">板块日报</span>
        <span v-if="reports.length" class="drt-count">{{ reports.length }}</span>
        <button type="button" class="drt-browser-toggle" @click="showThreadBrowser = !showThreadBrowser">
          <Icon :icon="showThreadBrowser ? 'mdi:file-document-outline' : 'mdi:chart-timeline-variant'" width="14" />
          <span>{{ showThreadBrowser ? '日报列表' : '话题总览' }}</span>
        </button>
      </div>

      <!-- Thread browser view -->
      <BoardThreadBrowser v-if="showThreadBrowser" :board-id="boardId" />

      <!-- Report list view -->
      <template v-else>
      <div v-if="loading" class="drt-loading">
        <div v-for="i in 2" :key="i" class="drt-skeleton" />
      </div>

      <div v-else-if="reports.length === 0" class="drt-empty">
        <Icon icon="mdi:file-document-outline" width="28" class="text-white/15" />
        <p>暂无日报</p>
      </div>

      <div v-else class="drt-list">
        <div
          v-for="(r, idx) in reports"
          :key="r.id"
          class="drt-summary-card"
          :style="{ animationDelay: `${idx * 50}ms` }"
          @click="openNewspaper(idx)"
        >
          <div class="drt-summary-top">
            <span class="drt-summary-date">{{ formatDateForSummary(r.period_date) }}</span>
            <span class="drt-summary-status" :class="reportStatusStyle[r.status] || 'bg-gray-800/40 text-gray-400'">
              {{ reportStatusLabel[r.status] || r.status }}
            </span>
          </div>
          <div class="drt-summary-text">{{ truncateText(r.summary || r.title, 30) }}</div>
          <div class="drt-summary-meta">{{ r.article_count }} 篇 · {{ r.cluster_count }} 聚类</div>
        </div>
      </div>

      <div v-if="reports.length > 0" class="drt-more">
        <button type="button" class="drt-more-btn" @click="loadMore">加载更早</button>
      </div>
      </template>
    </div>
  </div>

  <!-- FULL-SCREEN NEWSPAPER MODAL -->
  <Teleport to="body">
    <Transition name="np-modal">
      <div v-if="showModal" class="np-overlay" @click.self="closeNewspaper">
        <!-- Paper panel -->
        <div class="np-paper">
          <!-- Top bar -->
          <div class="np-topbar">
            <button type="button" class="np-topbar-arrow" :disabled="currentDayIndex <= 0" @click="prevDay">
              <Icon icon="mdi:chevron-up" width="16" />
              <span>上一天</span>
            </button>
            <div class="np-topbar-date">
              <span v-if="selectedReport" class="np-topbar-date-text">{{ formatDateForSummary(selectedReport.period_date) }}</span>
            </div>
            <button type="button" class="np-topbar-arrow" :disabled="currentDayIndex >= reports.length - 1" @click="nextDay">
              <span>下一天</span>
              <Icon icon="mdi:chevron-down" width="16" />
            </button>
            <button type="button" class="np-close" @click="closeNewspaper">
              <Icon icon="mdi:close" width="18" />
            </button>
          </div>

          <!-- Paper content area -->
          <div class="np-content">
            <div v-if="detailLoading" class="np-loading">
              <div v-for="i in 3" :key="i" class="np-skeleton" />
            </div>
            <div v-else-if="selectedDetail" class="np-page">
              <!-- Header: date -->
              <div class="np-date-big">
                {{ selectedReport ? formatDateForSummary(selectedReport.period_date) : '' }}
              </div>

              <!-- Highlights -->
              <template v-if="selectedDetail.highlights?.length">
                <div class="np-section-label">今日重点</div>
                <div class="np-divider"></div>
                <div v-for="(h, i) in selectedDetail.highlights" :key="i" class="np-highlight">
                  <div class="np-highlight-title">{{ h.title }}</div>
                  <div class="np-highlight-reason">{{ h.reason }}</div>
                </div>
              </template>

              <!-- Quality zones -->
              <template v-for="(zone, zi) in qualityZones" :key="zi">
                <div class="np-divider"></div>
                <div class="np-zone-header">
                  <span class="np-zone-label">{{ zone.label }}</span>
                  <span class="np-zone-count">{{ zone.sections.length }} 个聚类</span>
                </div>
                <div class="np-cluster-grid" :style="{ gridTemplateColumns: zone.columns === 2 ? 'repeat(2, 1fr)' : '1fr' }">
                  <div v-for="section in zone.sections" :key="section.cluster_index" class="np-cluster-card">
                    <div class="np-cluster-card-header" @click.stop="openSectionLifecycle(section)">
                      <div class="np-cluster-card-header-left">
                        <span class="np-cluster-card-name">{{ section.cluster_label }}</span>
                        <span v-if="section.status" class="np-section-status" :class="sectionStatusColor[section.status] || ''">
                          {{ sectionStatusLabel[section.status] || section.status }}
                        </span>
                      </div>
                      <div class="np-cluster-card-header-right">
                        <span class="np-cluster-card-count">{{ section.article_count }}篇</span>
                      </div>
                    </div>
                    <!-- Threads: collapsed by default -->
                    <div class="np-cluster-card-threads-toggle" @click.stop="toggleSectionExpand(section.cluster_index)">
                      <span>{{ section.threads?.length || 0 }} 条线索 ▸</span>
                    </div>
                    <div v-if="expandedSections.has(section.cluster_index)" class="np-cluster-card-threads">
                      <div v-for="(thread, ti) in section.threads" :key="ti" class="np-thread-item">
                        <div class="np-thread-body">
                          <div class="np-thread-title">{{ thread.title }}</div>
                          <div v-if="thread.summary" class="np-thread-summary">{{ thread.summary }}</div>
                        </div>
                        <div class="np-thread-actions">
                          <Icon icon="mdi:sitemap-outline" width="14" class="np-thread-lineage-icon" title="查看线程血统" @click.stop="openThreadLineage(thread)" />
                          <Icon icon="mdi:file-document-multiple-outline" width="14" class="np-thread-articles-icon" @click.stop="openThreadArticles($event, thread)" />
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </template>
            </div>
          <SectionLifecyclePanel
            v-if="lifecycleSectionId !== null"
            :section-id="lifecycleSectionId"
            :visible="lifecycleVisible"
            @close="closeSectionLifecycle"
            @navigate="navigateToSectionReport"
          />
          <ThreadLineagePanel
            v-if="lineageThreadId !== null"
            :thread-id="lineageThreadId"
            :visible="lineageVisible"
            @close="closeThreadLineage"
          />
          </div>
        </div>
      </div>
    </Transition>

    <!-- Thread article popup -->
    <div
      v-if="threadPopupOpen"
      ref="threadPopupFloating"
      class="np-thread-popup"
      :style="threadPopupStyles"
    >
      <div class="np-thread-popup-header">相关文章</div>
      <div v-if="threadPopupLoading" class="np-thread-popup-loading">加载中...</div>
      <div v-else class="np-thread-popup-list">
        <button
          v-for="article in threadPopupArticles"
          :key="article.id"
          class="np-thread-popup-item"
          :disabled="article.loading"
          @click.stop="handleArticleClick(article.id)"
        >
          <Icon icon="mdi:file-document-outline" width="14" />
          <span>{{ article.title }}</span>
        </button>
        <button
          v-if="hasMoreArticles"
          class="np-thread-popup-more"
          @click.stop="loadMoreThreadArticles"
        >
          加载更多...
        </button>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.drt-panel {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  margin-top: 1rem;
  padding: 1rem;
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(255, 255, 255, 0.025);
}

.drt-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.drt-title {
  font-size: 0.78rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.7);
}

.drt-count {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
  padding: 0.05rem 0.4rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
}

.drt-loading {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.drt-skeleton {
  height: 72px;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.03);
  animation: drtPulse 1.5s ease-in-out infinite;
}

@keyframes drtPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

.drt-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.4rem;
  padding: 2.5rem 0;
  color: rgba(255, 255, 255, 0.3);
  font-size: 0.8rem;
}

.drt-list {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}

/* Summary cards */
.drt-summary-card {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  padding: 0.5rem 0.6rem;
  border-radius: 6px;
  cursor: pointer;
  transition: background 0.12s ease;
  animation: drtFadeIn 200ms ease-out both;
}

@keyframes drtFadeIn {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.drt-summary-card:hover {
  background: rgba(255, 255, 255, 0.03);
}

.drt-summary-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.drt-summary-date {
  font-size: 0.7rem;
  color: rgba(255, 255, 255, 0.3);
}

.drt-summary-status {
  font-size: 0.6rem;
  padding: 0.1rem 0.4rem;
  border-radius: 4px;
  font-weight: 500;
  line-height: 1.4;
}

.drt-summary-text {
  font-size: 0.8rem;
  color: rgba(255, 255, 255, 0.6);
  line-height: 1.4;
}

.drt-summary-meta {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.2);
}

.drt-more {
  text-align: center;
}

.drt-more-btn {
  font-size: 0.68rem;
  padding: 0.2rem 0;
  border: none;
  background: none;
  color: rgba(255, 255, 255, 0.3);
  cursor: pointer;
  transition: color 0.12s ease;
}

.drt-more-btn:hover {
  color: rgba(255, 255, 255, 0.55);
}

.drt-browser-toggle {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  margin-left: auto;
  padding: 0.15rem 0.5rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 4px;
  background: none;
  color: rgba(255, 255, 255, 0.4);
  font-size: 0.68rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.drt-browser-toggle:hover {
  background: rgba(255, 255, 255, 0.05);
  color: rgba(255, 255, 255, 0.7);
}

/* === Newspaper Modal === */
.np-overlay {
  position: fixed;
  inset: 0;
  z-index: 200;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.7);
}

.np-paper {
  position: relative;
  display: flex;
  flex-direction: column;
  width: min(1100px, 92vw);
  max-height: 92vh;
  border-radius: 4px;
  background: #f4eed7;
  /* Subtle noise texture via SVG data URI */
  background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='noise'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23noise)' opacity='0.03'/%3E%3C/svg%3E");
  /* Inset vignette for aged paper look */
  box-shadow:
    inset 0 0 80px rgba(180, 160, 120, 0.3),
    0 20px 60px rgba(0, 0, 0, 0.5);
  overflow: hidden;
}

.np-topbar {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.6rem 1.2rem;
  border-bottom: 1px solid rgba(0, 0, 0, 0.1);
  flex-shrink: 0;
}

.np-topbar-arrow {
  display: flex;
  align-items: center;
  gap: 0.2rem;
  padding: 0.25rem 0.5rem;
  border: 1px solid rgba(0, 0, 0, 0.1);
  border-radius: 4px;
  background: none;
  color: rgba(0, 0, 0, 0.5);
  font-size: 0.72rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.np-topbar-arrow:hover:not(:disabled) {
  background: rgba(0, 0, 0, 0.05);
  color: rgba(0, 0, 0, 0.75);
}

.np-topbar-arrow:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.np-topbar-date {
  flex: 1;
  text-align: center;
}

.np-topbar-date-text {
  font-family: 'Noto Serif SC', serif;
  font-size: 0.88rem;
  font-weight: 600;
  color: rgba(0, 0, 0, 0.7);
}

.np-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 1px solid rgba(0, 0, 0, 0.1);
  border-radius: 4px;
  background: none;
  color: rgba(0, 0, 0, 0.4);
  cursor: pointer;
  transition: all 0.15s ease;
}

.np-close:hover {
  background: rgba(0, 0, 0, 0.05);
  color: rgba(0, 0, 0, 0.7);
}

.np-content {
  flex: 1;
  overflow-y: auto;
  padding: 1.5rem 2rem;
}

.np-loading {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.np-skeleton {
  height: 48px;
  border-radius: 6px;
  background: rgba(0, 0, 0, 0.04);
  animation: npPulse 1.5s ease-in-out infinite;
}

@keyframes npPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

/* Page content */
.np-page {
  color: rgba(0, 0, 0, 0.55);
}

.np-date-big {
  font-family: 'Noto Serif SC', serif;
  font-size: 2.2rem;
  font-weight: 300;
  color: rgba(0, 0, 0, 0.2);
  margin-bottom: 1.2rem;
  letter-spacing: 0.05em;
}

.np-section-label {
  font-family: 'Noto Serif SC', serif;
  font-size: 0.88rem;
  font-weight: 600;
  color: rgba(0, 0, 0, 0.6);
  margin-bottom: 0.4rem;
}

.np-divider {
  height: 1px;
  background: rgba(0, 0, 0, 0.12);
  margin: 0.75rem 0;
}

.np-highlight {
  margin-bottom: 0.75rem;
}

.np-highlight-title {
  font-family: 'Noto Serif SC', serif;
  font-size: 1.3rem;
  font-weight: 500;
  color: rgba(0, 0, 0, 0.9);
  line-height: 1.4;
  margin-bottom: 0.15rem;
}

.np-highlight-reason {
  font-size: 0.82rem;
  color: rgba(0, 0, 0, 0.5);
  line-height: 1.65;
}

/* Zone header */
.np-zone-header {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  margin-bottom: 0.5rem;
}

.np-zone-label {
  font-family: 'Noto Serif SC', serif;
  font-size: 1rem;
  font-weight: 600;
  color: rgba(0, 0, 0, 0.7);
}

.np-zone-count {
  font-size: 0.7rem;
  color: rgba(0, 0, 0, 0.3);
}

/* Cluster card grid */
.np-cluster-grid {
  display: grid;
  gap: 0.75rem;
  /* grid-template-columns set via inline style */
}

.np-cluster-card {
  background: rgba(255, 255, 255, 0.6);
  border: 1px solid rgba(0, 0, 0, 0.1);
  border-radius: 4px;
  padding: 0.75rem;
}

.np-cluster-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.4rem;
  border-bottom: 1px solid rgba(0, 0, 0, 0.08);
  padding-bottom: 0.3rem;
}

.np-cluster-card-name {
  font-family: 'Noto Serif SC', serif;
  font-weight: 600;
  font-size: 0.85rem;
  color: rgba(0, 0, 0, 0.75);
}

.np-cluster-card-count {
  font-size: 0.75rem;
  color: rgba(139, 115, 85, 0.7);
}

.np-cluster-card-threads {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

/* Thread item */
.np-thread-item {
  display: flex;
  align-items: flex-start;
  gap: 0.4rem;
  padding: 0.35rem 0;
  cursor: pointer;
  border-radius: 3px;
  transition: background 0.1s ease;
}

.np-thread-item:hover {
  background: rgba(0, 0, 0, 0.04);
}

.np-thread-status {
  flex-shrink: 0;
  font-size: 0.58rem;
  padding: 0.08rem 0.35rem;
  border-radius: 3px;
  font-weight: 500;
  line-height: 1.4;
  margin-top: 0.1rem;
}

.np-thread-body {
  flex: 1;
  min-width: 0;
}

.np-thread-title {
  font-size: 0.82rem;
  font-weight: 500;
  color: rgba(0, 0, 0, 0.75);
  line-height: 1.4;
}

.np-thread-summary {
  font-size: 0.72rem;
  color: rgba(0, 0, 0, 0.4);
  line-height: 1.5;
  margin-top: 0.15rem;
}

.np-thread-actions {
  display: flex;
  align-items: flex-start;
  gap: 0.3rem;
}

.np-thread-lineage-icon {
  color: rgba(0, 0, 0, 0.15);
  margin-top: 0.15rem;
  cursor: pointer;
  transition: color 0.12s ease;
}

.np-thread-lineage-icon:hover {
  color: rgba(0, 0, 0, 0.5);
}

.np-thread-articles-icon {
  flex-shrink: 0;
  color: rgba(0, 0, 0, 0.2);
  margin-top: 0.15rem;
}

/* Thread popup */
.np-thread-popup {
  z-index: 9999;
  background: white;
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.15), 0 2px 4px rgba(0, 0, 0, 0.1);
  border: 1px solid rgba(0, 0, 0, 0.1);
  min-width: 280px;
  max-width: 400px;
  max-height: 320px;
  overflow-y: auto;
  padding: 4px;
}

.np-thread-popup-header {
  font-size: 0.75rem;
  font-weight: 600;
  color: rgba(0, 0, 0, 0.5);
  padding: 6px 10px 4px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.06);
  margin-bottom: 2px;
}

.np-thread-popup-loading {
  padding: 12px;
  text-align: center;
  font-size: 0.8rem;
  color: rgba(0, 0, 0, 0.4);
}

.np-thread-popup-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  width: 100%;
  padding: 8px 10px;
  border: none;
  background: transparent;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.8rem;
  color: rgba(0, 0, 0, 0.7);
  text-align: left;
  line-height: 1.4;
  transition: background 0.1s ease;
}

.np-thread-popup-item:hover:not(:disabled) {
  background: rgba(0, 0, 0, 0.04);
}

.np-thread-popup-item:disabled {
  opacity: 0.5;
  cursor: wait;
}

.np-thread-popup-item svg {
  flex-shrink: 0;
  color: rgba(0, 0, 0, 0.3);
  margin-top: 2px;
}

.np-thread-popup-more {
  display: block;
  width: 100%;
  padding: 6px 10px;
  border: none;
  background: transparent;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.75rem;
  color: rgba(0, 0, 0, 0.4);
  text-align: center;
  transition: background 0.1s ease;
}

.np-thread-popup-more:hover {
  background: rgba(0, 0, 0, 0.04);
  color: rgba(0, 0, 0, 0.6);
}

/* Modal open/close animation */
.np-modal-enter-active {
  transition: opacity 200ms ease-out;
}
.np-modal-enter-active .np-paper {
  transition: opacity 300ms ease-out, transform 300ms ease-out;
}
.np-modal-leave-active {
  transition: opacity 200ms ease-in;
}
.np-modal-leave-active .np-paper {
  transition: opacity 200ms ease-in, transform 200ms ease-in;
}
.np-modal-enter-from {
  opacity: 0;
}
.np-modal-enter-from .np-paper {
  opacity: 0;
  transform: scale(0.95);
}
.np-modal-leave-to {
  opacity: 0;
}
.np-modal-leave-to .np-paper {
  opacity: 0;
  transform: scale(0.95);
}

/* Section status colors for light paper background */
.np-section-emerging { background: rgba(34, 197, 94, 0.15); color: rgba(22, 101, 52, 0.8); }
.np-section-continuing { background: rgba(59, 130, 246, 0.15); color: rgba(30, 64, 175, 0.8); }
.np-section-ending { background: rgba(107, 114, 128, 0.15); color: rgba(55, 65, 81, 0.8); }

.np-section-status {
  font-size: 0.58rem;
  padding: 0.08rem 0.35rem;
  border-radius: 3px;
  font-weight: 500;
  line-height: 1.4;
}

.np-cluster-card-header-left {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.np-cluster-card-header-right {
  display: flex;
  align-items: center;
  gap: 0.3rem;
}

.np-cluster-card-header {
  cursor: pointer;
}

.np-cluster-card-threads-toggle {
  font-size: 0.72rem;
  color: rgba(0, 0, 0, 0.35);
  padding: 0.25rem 0;
  cursor: pointer;
  transition: color 0.1s ease;
}

.np-cluster-card-threads-toggle:hover {
  color: rgba(0, 0, 0, 0.6);
}
</style>
