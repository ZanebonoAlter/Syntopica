<script setup lang="ts">
import { ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useDailyReportsApi, type DailyReportListItem, type DailyReport, type DailyReportSection, type DailyReportThread } from '~/api/dailyReports'

const props = defineProps<{ boardId: number }>()

const { getBoardDailyReports, getDailyReportDetail } = useDailyReportsApi()

const reports = ref<DailyReportListItem[]>([])
const days = ref(7)
const loading = ref(false)
const expandedId = ref<number | null>(null)
const detailCache = ref<Map<number, DailyReport>>(new Map())
const detailLoading = ref<number | null>(null)

const threadStatusColor: Record<string, string> = {
  emerging: 'bg-green-900/40 text-green-400',
  continuing: 'bg-blue-900/40 text-blue-400',
  splitting: 'bg-orange-900/40 text-orange-400',
  merging: 'bg-purple-900/40 text-purple-400',
  ending: 'bg-gray-800/40 text-gray-400',
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

async function toggleExpand(report: DailyReportListItem) {
  if (expandedId.value === report.id) {
    expandedId.value = null
    return
  }
  expandedId.value = report.id

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

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' })
}

watch(() => props.boardId, () => {
  days.value = 7
  expandedId.value = null
  detailCache.value = new Map()
  loadReports()
}, { immediate: true })
</script>

<template>
  <div class="drt-panel">
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
        v-for="r in reports"
        :key="r.id"
        class="drt-card"
        :class="{ 'drt-card--expanded': expandedId === r.id }"
        @click="toggleExpand(r)"
      >
        <div class="drt-card-top">
          <span class="drt-status" :class="reportStatusStyle[r.status] || 'bg-gray-800/40 text-gray-400'">
            {{ reportStatusLabel[r.status] || r.status }}
          </span>
          <span class="drt-date">{{ formatDate(r.period_date) }}</span>
          <span class="drt-meta">{{ r.cluster_count }} 聚类 · {{ r.article_count }} 篇</span>
        </div>
        <div class="drt-card-title">{{ r.title }}</div>
        <div class="drt-card-summary">{{ r.summary }}</div>

        <!-- Loading detail -->
        <div v-if="detailLoading === r.id" class="drt-detail-loading">
          <div class="drt-skeleton-sm" />
          <div class="drt-skeleton-sm" />
        </div>

        <!-- Expanded detail -->
        <div v-if="expandedId === r.id && detailCache.has(r.id)" class="drt-expanded" @click.stop>
          <template v-if="detailCache.get(r.id) === undefined">
            <!-- should not happen -->
          </template>
          <template v-else>
            <!-- Highlights -->
            <div v-if="detailCache.get(r.id)!.highlights?.length" class="drt-section">
              <div class="drt-section-title">
                <Icon icon="mdi:star-outline" width="12" class="text-yellow-400/60" />
                今日重点
              </div>
              <div
                v-for="(h, hi) in detailCache.get(r.id)!.highlights"
                :key="hi"
                class="drt-highlight"
              >
                <span class="drt-highlight-title">{{ h.title }}</span>
                <span class="drt-highlight-reason">{{ h.reason }}</span>
              </div>
            </div>

            <!-- Dynamics -->
            <div v-if="detailCache.get(r.id)!.dynamics" class="drt-section">
              <div class="drt-section-title">
                <Icon icon="mdi:trending-up" width="12" class="text-blue-400/60" />
                板块动态
              </div>
              <p class="drt-dynamics">{{ detailCache.get(r.id)!.dynamics }}</p>
            </div>

            <!-- Clustered threads -->
            <div v-if="detailCache.get(r.id)!.sections?.length" class="drt-section">
              <div class="drt-section-title">
                <Icon icon="mdi:source-branch" width="12" class="text-purple-400/60" />
                叙事线索
              </div>
              <div
                v-for="section in detailCache.get(r.id)!.sections"
                :key="section.id"
                class="drt-cluster"
              >
                <div class="drt-cluster-header">
                  <span class="drt-cluster-label">{{ section.cluster_label }}</span>
                  <span class="drt-cluster-count">{{ section.article_count }} 篇</span>
                </div>
                <div v-if="section.threads?.length" class="drt-threads">
                  <div
                    v-for="(thread, ti) in section.threads"
                    :key="ti"
                    class="drt-thread"
                  >
                    <span class="drt-thread-status" :class="threadStatusColor[thread.status] || 'bg-gray-800/40 text-gray-400'">
                      {{ threadStatusLabel[thread.status] || thread.status }}
                    </span>
                    <div class="drt-thread-body">
                      <span class="drt-thread-title">{{ thread.title }}</span>
                      <span class="drt-thread-summary">{{ thread.summary }}</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </template>
        </div>
      </div>
    </div>

    <div v-if="reports.length > 0" class="drt-more">
      <button type="button" class="drt-more-btn" @click="loadMore">
        加载更早
      </button>
    </div>
  </div>
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

.drt-skeleton-sm {
  height: 24px;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.03);
  animation: drtPulse 1.5s ease-in-out infinite;
  margin-top: 0.3rem;
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
  gap: 0.4rem;
}

.drt-card {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  padding: 0.65rem 0.75rem;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(255, 255, 255, 0.02);
  cursor: pointer;
  transition: all 0.12s ease;
}

.drt-card:hover {
  background: rgba(255, 255, 255, 0.04);
  border-color: rgba(255, 255, 255, 0.1);
}

.drt-card--expanded {
  background: rgba(255, 255, 255, 0.04);
  border-color: rgba(255, 255, 255, 0.12);
}

.drt-card-top {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.drt-status {
  font-size: 0.6rem;
  padding: 0.1rem 0.4rem;
  border-radius: 4px;
  font-weight: 500;
  line-height: 1.4;
}

.drt-date {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.35);
}

.drt-meta {
  font-size: 0.6rem;
  color: rgba(255, 255, 255, 0.25);
  margin-left: auto;
}

.drt-card-title {
  font-size: 0.78rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.8);
  line-height: 1.4;
}

.drt-card-summary {
  font-size: 0.7rem;
  color: rgba(255, 255, 255, 0.45);
  line-height: 1.5;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.drt-detail-loading {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  margin-top: 0.5rem;
  padding-top: 0.5rem;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}

.drt-expanded {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin-top: 0.5rem;
  padding-top: 0.5rem;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}

.drt-section {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.drt-section-title {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  font-size: 0.7rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.55);
  margin-bottom: 0.15rem;
}

.drt-highlight {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  padding: 0.35rem 0.5rem;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid rgba(255, 255, 255, 0.04);
}

.drt-highlight-title {
  font-size: 0.72rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.75);
}

.drt-highlight-reason {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.4);
  line-height: 1.4;
}

.drt-dynamics {
  font-size: 0.68rem;
  color: rgba(255, 255, 255, 0.5);
  line-height: 1.6;
}

.drt-cluster {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  padding: 0.4rem 0.5rem;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.015);
  border: 1px solid rgba(255, 255, 255, 0.04);
}

.drt-cluster-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.drt-cluster-label {
  font-size: 0.72rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.65);
}

.drt-cluster-count {
  font-size: 0.6rem;
  color: rgba(255, 255, 255, 0.25);
}

.drt-threads {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  margin-top: 0.2rem;
}

.drt-thread {
  display: flex;
  align-items: flex-start;
  gap: 0.35rem;
  padding: 0.25rem 0;
}

.drt-thread-status {
  flex-shrink: 0;
  font-size: 0.58rem;
  padding: 0.08rem 0.35rem;
  border-radius: 3px;
  font-weight: 500;
  line-height: 1.4;
  margin-top: 0.05rem;
}

.drt-thread-body {
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
  min-width: 0;
}

.drt-thread-title {
  font-size: 0.7rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.7);
}

.drt-thread-summary {
  font-size: 0.63rem;
  color: rgba(255, 255, 255, 0.4);
  line-height: 1.4;
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
</style>
