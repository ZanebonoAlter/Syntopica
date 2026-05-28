<script setup lang="ts">
import { ref, watch, computed, onUnmounted } from 'vue'
import { Icon } from '@iconify/vue'
import { useDailyReportsApi, type DailyReportListItem, type DailyReport } from '~/api/dailyReports'

const props = defineProps<{ boardId: number }>()

const { getBoardDailyReports, getDailyReportDetail } = useDailyReportsApi()

const reports = ref<DailyReportListItem[]>([])
const days = ref(7)
const loading = ref(false)
const showModal = ref(false)
const currentDayIndex = ref(-1)
const detailCache = ref<Map<number, DailyReport>>(new Map())
const detailLoading = ref<number | null>(null)
const currentPage = ref(1)
const flipDirection = ref<'left' | 'right'>('right')

const selectedReport = computed(() => {
  if (currentDayIndex.value < 0 || currentDayIndex.value >= reports.value.length) return null
  return reports.value[currentDayIndex.value]
})

const selectedDetail = computed<DailyReport | null>(() => {
  if (!selectedReport.value) return null
  return detailCache.value.get(selectedReport.value.id) ?? null
})

type NewspaperPage =
  | { type: 'overview', highlights: any[], dynamics: string | null, sections: any[] }
  | { type: 'content', sections: any[] }

const PAGE1_CAPACITY = 4
const PAGE_N_CAPACITY = 5

const pages = computed<NewspaperPage[]>(() => {
  if (!selectedDetail.value) return []
  const result: NewspaperPage[] = []
  const detail = selectedDetail.value

  const sortedSections = [...(detail.sections || [])].sort((a, b) => {
    if (a.best_tier !== b.best_tier) return a.best_tier - b.best_tier
    return b.avg_score - a.avg_score
  })

  // Page 1: overview + first N sections
  const page1Sections = sortedSections.slice(0, PAGE1_CAPACITY)
  result.push({
    type: 'overview',
    highlights: detail.highlights || [],
    dynamics: detail.dynamics || null,
    sections: page1Sections,
  })

  // Remaining pages
  let idx = PAGE1_CAPACITY
  while (idx < sortedSections.length) {
    const end = Math.min(idx + PAGE_N_CAPACITY, sortedSections.length)
    result.push({
      type: 'content',
      sections: sortedSections.slice(idx, end),
    })
    idx = end
  }

  return result
})

const totalPages = computed(() => pages.value.length)

const currentPg = computed(() => pages.value[currentPage.value - 1] as NewspaperPage | undefined)

const threadStatusColor: Record<string, string> = {
  emerging: 'np-status-emerging',
  continuing: 'np-status-continuing',
  splitting: 'np-status-splitting',
  merging: 'np-status-merging',
  ending: 'np-status-ending',
}

const threadStatusLabel: Record<string, string> = {
  emerging: '新兴',
  continuing: '持续',
  splitting: '分裂',
  merging: '合并',
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
  currentPage.value = 1
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

function closeNewspaper() {
  showModal.value = false
}

function nextPage() {
  if (currentPage.value < totalPages.value) {
    flipDirection.value = 'right'
    currentPage.value++
  }
}

function prevPage() {
  if (currentPage.value > 1) {
    flipDirection.value = 'left'
    currentPage.value--
  }
}

function prevDay() {
  if (currentDayIndex.value > 0) {
    currentDayIndex.value--
    currentPage.value = 1
    loadDetailForCurrentDay()
  }
}

function nextDay() {
  if (currentDayIndex.value < reports.value.length - 1) {
    currentDayIndex.value++
    currentPage.value = 1
    loadDetailForCurrentDay()
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

function handleKeydown(e: KeyboardEvent) {
  if (!showModal.value) return
  switch (e.key) {
    case 'ArrowLeft': e.preventDefault(); prevPage(); break
    case 'ArrowRight': e.preventDefault(); nextPage(); break
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

onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown)
  document.body.style.overflow = ''
})

watch(() => props.boardId, () => {
  days.value = 7
  showModal.value = false
  currentDayIndex.value = -1
  detailCache.value = new Map()
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
      </div>

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
    </div>
  </div>

  <!-- FULL-SCREEN NEWSPAPER MODAL -->
  <Teleport to="body">
    <Transition name="np-modal">
      <div v-if="showModal" class="np-overlay" @click.self="closeNewspaper">
        <!-- Left edge button -->
        <button
          type="button"
          class="np-edge-btn np-edge-left"
          :disabled="currentPage <= 1"
          @click="prevPage"
        >
          <Icon icon="mdi:chevron-left" width="28" />
        </button>

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
            <Transition :name="flipDirection === 'right' ? 'np-page-right' : 'np-page-left'" mode="out-in">
              <div :key="currentPage" class="np-page">
                <!-- Overview page (page 1): highlights + dynamics + cluster grid -->
                <template v-if="currentPg?.type === 'overview'">
                  <div class="np-date-big">
                    {{ selectedReport ? formatDateForSummary(selectedReport.period_date) : '' }}
                  </div>
                  <div v-if="currentPg.highlights?.length">
                    <div class="np-section-label">今日重点</div>
                    <div class="np-divider"></div>
                    <div v-for="(h, i) in currentPg.highlights" :key="i" class="np-highlight">
                      <div class="np-highlight-title">{{ h.title }}</div>
                      <div class="np-highlight-reason">{{ h.reason }}</div>
                    </div>
                  </div>
                  <template v-if="currentPg.dynamics">
                    <div class="np-divider"></div>
                    <div class="np-section-label">板块动态</div>
                    <p class="np-dynamics">{{ currentPg.dynamics }}</p>
                  </template>

                  <!-- Cluster grid on overview page -->
                  <template v-if="currentPg.sections?.length">
                    <div class="np-divider"></div>
                    <div class="np-cluster-grid">
                      <div v-for="section in currentPg.sections" :key="section.cluster_index" class="np-cluster-card">
                        <div class="np-cluster-card-header">
                          <span class="np-cluster-card-name">{{ section.cluster_label }}</span>
                          <span class="np-cluster-card-count">{{ section.article_count }}篇</span>
                        </div>
                        <div class="np-cluster-card-threads">
                          <template v-for="(thread, ti) in section.threads?.slice(0, 3)" :key="ti">
                            <div class="np-thread-compact">
                              <span class="np-thread-compact-title">{{ thread.title }}</span>
                              <span :class="['np-thread-compact-status', threadStatusColor[thread.status] || '']">{{ threadStatusLabel[thread.status] || thread.status }}</span>
                            </div>
                          </template>
                        </div>
                      </div>
                    </div>
                  </template>
                </template>

                <!-- Content page: hotspot + cluster grid -->
                <template v-else-if="currentPg?.type === 'content'">
                  <div class="np-date-big">
                    {{ selectedReport ? formatDateForSummary(selectedReport.period_date) : '' }}
                  </div>

                  <!-- Hotspot: top section summary -->
                  <template v-if="currentPg.sections.length > 0">
                    <div class="np-section-label">本页热点</div>
                    <div class="np-hotspot">
                      <div class="np-hotspot-name">{{ currentPg.sections[0].cluster_label }}</div>
                      <p class="np-hotspot-summary">{{ currentPg.sections[0].threads?.[0]?.summary || '' }}</p>
                    </div>
                  </template>

                  <!-- Cluster grid -->
                  <div class="np-cluster-grid">
                    <div v-for="section in currentPg.sections" :key="section.cluster_index" class="np-cluster-card">
                      <div class="np-cluster-card-header">
                        <span class="np-cluster-card-name">{{ section.cluster_label }}</span>
                        <span class="np-cluster-card-count">{{ section.article_count }}篇</span>
                      </div>
                      <div class="np-cluster-card-threads">
                        <template v-for="(thread, ti) in section.threads?.slice(0, 3)" :key="ti">
                          <div class="np-thread-compact">
                            <span class="np-thread-compact-title">{{ thread.title }}</span>
                            <span :class="['np-thread-compact-status', threadStatusColor[thread.status] || '']">{{ threadStatusLabel[thread.status] || thread.status }}</span>
                          </div>
                        </template>
                      </div>
                    </div>
                  </div>
                </template>
              </div>
            </Transition>
          </div>

          <!-- Page number -->
          <div v-if="totalPages > 1" class="np-pagenum">{{ currentPage }} / {{ totalPages }}</div>
        </div>

        <!-- Right edge button -->
        <button
          type="button"
          class="np-edge-btn np-edge-right"
          :disabled="currentPage >= totalPages"
          @click="nextPage"
        >
          <Icon icon="mdi:chevron-right" width="28" />
        </button>
      </div>
    </Transition>
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

/* === Newspaper Modal === */
.np-overlay {
  position: fixed;
  inset: 0;
  z-index: 200;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  background: rgba(0, 0, 0, 0.7);
}

.np-paper {
  position: relative;
  display: flex;
  flex-direction: column;
  width: min(800px, 85vw);
  max-height: 90vh;
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

.np-divider-subtle {
  height: 0;
  border-top: 1px dashed rgba(0, 0, 0, 0.08);
  margin: 0.4rem 0;
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

.np-dynamics {
  font-size: 0.82rem;
  color: rgba(0, 0, 0, 0.5);
  line-height: 1.7;
}

.np-cluster-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-bottom: 0.3rem;
}

.np-cluster-label {
  font-family: 'Noto Serif SC', serif;
  font-size: 0.92rem;
  font-weight: 500;
  color: rgba(0, 0, 0, 0.8);
}

.np-cluster-count {
  font-size: 0.65rem;
  color: rgba(0, 0, 0, 0.3);
}

.np-threads {
  display: flex;
  flex-direction: column;
}

.np-thread {
  display: flex;
  align-items: flex-start;
  gap: 0.4rem;
  padding: 0.3rem 0;
}

.np-thread-status {
  flex-shrink: 0;
  font-size: 0.58rem;
  padding: 0.08rem 0.35rem;
  border-radius: 3px;
  font-weight: 500;
  line-height: 1.4;
  margin-top: 0.05rem;
}

.np-thread-body {
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
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
}

.np-pagenum {
  text-align: center;
  padding: 0.5rem;
  font-size: 0.7rem;
  color: rgba(0, 0, 0, 0.25);
  flex-shrink: 0;
}

/* Edge buttons */
.np-edge-btn {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 80px;
  border: none;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.5);
  cursor: pointer;
  transition: all 0.15s ease;
  z-index: 201;
}

.np-edge-btn:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.15);
  color: rgba(255, 255, 255, 0.8);
}

.np-edge-btn:disabled {
  opacity: 0.2;
  cursor: not-allowed;
}

.np-edge-left {
  left: max(1rem, calc((100vw - 800px) / 2 - 56px));
}

.np-edge-right {
  right: max(1rem, calc((100vw - 800px) / 2 - 56px));
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

/* Page flip animation */
.np-page-right-enter-active,
.np-page-right-leave-active,
.np-page-left-enter-active,
.np-page-left-leave-active {
  transition: opacity 250ms ease-out, transform 250ms ease-out;
  position: absolute;
  width: 100%;
}

.np-page-right-enter-from {
  opacity: 0;
  transform: translateX(40px);
}
.np-page-right-leave-to {
  opacity: 0;
  transform: translateX(-40px);
}
.np-page-left-enter-from {
  opacity: 0;
  transform: translateX(-40px);
}
.np-page-left-leave-to {
  opacity: 0;
  transform: translateX(40px);
}

/* Cluster card grid */
.np-cluster-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 0.75rem;
  margin-top: 0.75rem;
}

.np-cluster-card {
  background: rgba(255, 255, 255, 0.6);
  border: 1px solid rgba(0, 0, 0, 0.1);
  border-radius: 4px;
  padding: 0.6rem;
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

.np-thread-compact {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  font-size: 0.8rem;
  gap: 0.3rem;
}

.np-thread-compact-title {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: rgba(0, 0, 0, 0.65);
}

.np-thread-compact-status {
  flex-shrink: 0;
  font-size: 0.58rem;
  padding: 0.08rem 0.35rem;
  border-radius: 3px;
  font-weight: 500;
  line-height: 1.4;
}

/* Hotspot block */
.np-hotspot {
  background: rgba(255, 255, 255, 0.8);
  border-left: 3px solid rgba(139, 69, 19, 0.6);
  padding: 0.5rem 0.75rem;
  margin-bottom: 0.75rem;
}

.np-hotspot-name {
  font-family: 'Noto Serif SC', serif;
  font-weight: 600;
  font-size: 0.9rem;
  color: rgba(0, 0, 0, 0.75);
}

.np-hotspot-summary {
  font-size: 0.8rem;
  color: rgba(0, 0, 0, 0.45);
  margin-top: 0.25rem;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* Thread status colors for light paper background */
.np-status-emerging { background: rgba(34, 197, 94, 0.15); color: rgba(22, 101, 52, 0.8); }
.np-status-continuing { background: rgba(59, 130, 246, 0.15); color: rgba(30, 64, 175, 0.8); }
.np-status-splitting { background: rgba(249, 115, 22, 0.15); color: rgba(154, 52, 18, 0.8); }
.np-status-merging { background: rgba(168, 85, 247, 0.15); color: rgba(107, 33, 168, 0.8); }
.np-status-ending { background: rgba(107, 114, 128, 0.15); color: rgba(55, 65, 81, 0.8); }
</style>
