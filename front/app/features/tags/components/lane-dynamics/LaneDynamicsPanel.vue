<script setup lang="ts">
/**
 * 泳道动态容器（overview-lane-dynamics ui-design §2-§4 + tasks 3.2/3.4）。
 *
 * 职责：
 *  - 拉取 GET /semantic-boards/:id/lane-dynamics（单请求聚合，只读）。
 *  - 状态矩阵（ui-design §4）：loading 骨架（卡片 shimmer 占位）/
 *    error 内联错误条 + 重试（保留旧数据时顶部刷新提示条）/
 *    空态两分支（无日报 → 「生成日报」引导；有日报无泳道 → 文案）/
 *    success：卡片网格 + 候选栏。
 *  - 候选栏（tasks 3.4）：只读紧凑列表，无候选不渲染；点击跳话题总览 focus。
 *  - 空态生成日报：迁移自原态势版图（generateDailyReport +
 *    useDailyReportProgress WS 进度，完成后自动刷新）。
 *
 * 卡片/候选栏 click → 向上 emit selectTopic，由 TagsPage 切「话题总览」tab 并聚焦。
 */
import { computed, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useLaneDynamicsApi, type LaneDynamicsResponse } from '~/api/laneDynamics'
import { useDailyReportsApi } from '~/api/dailyReports'
import { useDailyReportProgress } from '~/composables/useDailyReportProgress'
import { useNotify } from '~/composables/useNotify'
import LaneDynamicsCard from './LaneDynamicsCard.vue'

const props = defineProps<{
  boardId: number
}>()

const emit = defineEmits<{
  selectTopic: [topicId: number]
  generated: []
}>()

const api = useLaneDynamicsApi()
const reportsApi = useDailyReportsApi()
const { success: notifySuccess, error: notifyError } = useNotify()
const { progress, done, totalSaved, reset } = useDailyReportProgress()

const DEFAULT_DAYS = 14

const data = ref<LaneDynamicsResponse | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)
/** 刷新失败但已保留旧数据 → 顶部刷新提示条（ui-design 状态矩阵 error 行）。 */
const staleNotice = ref(false)

const windowDays = computed(() => data.value?.window_days ?? DEFAULT_DAYS)
const lanes = computed(() => data.value?.lanes ?? [])
const candidates = computed(() => data.value?.candidates ?? [])

/** 空态分支①：板块无日报（has_reports=false，tasks 2.2 响应契约）。 */
const isNoReports = computed(() => data.value !== null && !data.value.has_reports)
/** 空态分支②：有日报但无活跃泳道。 */
const isNoLanes = computed(() => data.value !== null && data.value.has_reports && lanes.value.length === 0)

async function load(boardId: number) {
  loading.value = true
  error.value = null
  staleNotice.value = false
  const res = await api.getLaneDynamics(boardId, DEFAULT_DAYS)
  if (res.success && res.data) {
    data.value = res.data
  } else {
    error.value = res.error || '加载泳道动态失败'
    // 保留旧数据：顶部刷新提示条，不清空已渲染内容（ui-design §4）。
    if (data.value) staleNotice.value = true
  }
  loading.value = false
}

void load(props.boardId)
watch(() => props.boardId, (id) => {
  data.value = null
  void load(id)
})

// ── 空态：生成日报（迁移自原态势版图，复用既有端点 + WS 进度） ──────
const generating = ref(false)
const progressEntries = computed(() => Array.from(progress.value.values()))

async function handleGenerate() {
  if (generating.value) return
  generating.value = true
  reset()
  const today = new Date().toISOString().slice(0, 10)
  const res = await reportsApi.generateDailyReport({ date: today, board_id: props.boardId })
  if (!res.success || !res.data) {
    notifyError(res.error || '触发生成失败')
    generating.value = false
    return
  }
  // job 已触发，WS 进度由 useDailyReportProgress 持续写入；done 置 true 后重载。
}

// done 由 WS daily_report_done 置位 → 重载泳道动态 + 通知（spec「生成后刷新」）
watch(done, (isDone) => {
  if (!isDone) return
  generating.value = false
  notifySuccess(`日报已生成（共 ${totalSaved.value} 篇）`)
  emit('generated')
  void load(props.boardId)
})

function onSelectTopic(id: number) {
  emit('selectTopic', id)
}
</script>

<template>
  <section class="ldp" data-testid="lane-dynamics-panel">
    <header class="ldp-head">
      <Icon icon="mdi:chart-timeline-variant" width="15" class="ldp-head-icon" />
      <span class="ldp-title">泳道动态</span>
      <span class="ldp-window">近{{ windowDays }}天 · 随每日日报结算</span>
      <div class="ldp-spacer" />
      <button type="button" class="ldp-refresh" title="刷新" data-testid="lane-refresh" @click="load(boardId)">
        <Icon icon="mdi:refresh" width="13" />
      </button>
    </header>

    <!-- 刷新失败但保留旧数据：顶部刷新提示条（状态矩阵 error 行） -->
    <div v-if="staleNotice && error" class="ldp-stale" data-testid="lane-stale-notice">
      <Icon icon="mdi:alert-circle-outline" width="14" />
      <span>刷新失败（{{ error }}），当前显示的是之前的数据</span>
      <button type="button" class="ldp-stale-retry" @click="load(boardId)">重试</button>
    </div>

    <!-- loading：骨架（卡片 shimmer 占位，不用转圈盖层） -->
    <div v-if="loading" class="ldp-grid" data-testid="lane-skeleton">
      <div v-for="i in 4" :key="i" class="ldp-skeleton" />
    </div>

    <!-- error（无旧数据）：内联错误条 + 重试 -->
    <div v-else-if="error && !data" class="ldp-error" data-testid="lane-error">
      <Icon icon="mdi:alert-circle-outline" width="16" />
      <span>{{ error }}</span>
      <button type="button" class="ldp-retry" data-testid="lane-retry" @click="load(boardId)">重试</button>
    </div>

    <!-- 空态①：板块无日报 → 「生成日报」引导（迁移 WS 进度模式） -->
    <div v-else-if="isNoReports" class="ldp-empty" data-testid="lane-empty-no-reports">
      <Icon icon="mdi:file-document-outline" width="22" class="ldp-empty-icon" />
      <p class="ldp-empty-text">该板块还没有日报，泳道动态基于每日日报结算。</p>
      <p class="ldp-empty-hint">生成第一份日报后，这里会出现各条泳道最近发生的事。</p>
      <button
        type="button"
        class="ldp-empty-btn"
        data-testid="lane-generate-btn"
        :disabled="generating"
        @click="handleGenerate"
      >
        <Icon icon="mdi:play" width="13" />
        {{ generating ? '生成中…' : '生成日报' }}
      </button>
      <!-- WS 进度（useDailyReportProgress，mirror 旧态势版图空态） -->
      <div v-if="generating && progressEntries.length" class="ldp-progress">
        <div
          v-for="entry in progressEntries"
          :key="entry.board_id"
          class="ldp-progress-row"
        >
          <span class="ldp-progress-board">{{ entry.board_name }}</span>
          <span
            class="ldp-progress-status"
            :class="{
              'ldp-progress-status--generating': entry.status === 'generating',
              'ldp-progress-status--completed': entry.status === 'completed',
              'ldp-progress-status--failed': entry.status === 'failed',
            }"
          >
            <template v-if="entry.status === 'waiting'">等待中</template>
            <template v-else-if="entry.status === 'generating'">生成中 {{ entry.progress }}</template>
            <template v-else-if="entry.status === 'completed'">完成 ({{ entry.saved }})</template>
            <template v-else-if="entry.status === 'failed'">失败</template>
            <template v-else>{{ entry.status }}</template>
          </span>
        </div>
      </div>
    </div>

    <!-- 空态②：有日报但无活跃泳道 -->
    <div v-else-if="isNoLanes" class="ldp-empty" data-testid="lane-empty-no-lanes">
      <Icon icon="mdi:tag-off-outline" width="22" class="ldp-empty-icon" />
      <p class="ldp-empty-text">暂无活跃泳道——在日报里孵化话题后出现。</p>
    </div>

    <!-- 正常态：卡片网格（按 section_count_14d 降序，后端已排） -->
    <template v-else-if="data">
      <div v-if="lanes.length" class="ldp-grid">
        <LaneDynamicsCard
          v-for="lane in lanes"
          :key="lane.topic_id"
          :lane="lane"
          :window-days="windowDays"
          @select="onSelectTopic"
        />
      </div>

      <!-- 候选栏（tasks 3.4）：只读紧凑列表，无候选不渲染 -->
      <div v-if="candidates.length" class="ldp-cand" data-testid="lane-candidate-bar">
        <div class="ldp-cand-title">候选泳道 · 达到激活门槛，值得注意</div>
        <div
          v-for="cand in candidates"
          :key="cand.topic_id"
          class="ldp-cand-row"
          role="button"
          tabindex="0"
          :data-testid="`lane-candidate-${cand.topic_id}`"
          @click="onSelectTopic(cand.topic_id)"
          @keydown.enter.prevent="onSelectTopic(cand.topic_id)"
          @keydown.space.prevent="onSelectTopic(cand.topic_id)"
        >
          <span class="ldp-cand-name">{{ cand.label }}</span>
          <span class="ldp-cand-hint">{{ cand.recent_hint }}</span>
          <span class="ldp-cand-date">{{ cand.last_seen_date }}</span>
        </div>
      </div>
    </template>
  </section>
</template>

<style scoped>
.ldp {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.ldp-head {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.ldp-head-icon {
  color: var(--color-text-muted);
}

.ldp-title {
  font-size: 0.92rem;
  font-weight: 700;
  color: var(--color-text-primary);
}

.ldp-window {
  font-size: 0.66rem;
  color: var(--color-text-muted);
}

.ldp-spacer {
  flex: 1;
}

.ldp-refresh {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 6px;
  border: 1px solid var(--color-border-medium);
  background: var(--color-bg-sunken);
  color: var(--color-text-muted);
  cursor: pointer;
  transition: all 0.12s ease;
}

.ldp-refresh:hover {
  color: var(--color-text-secondary);
  border-color: var(--color-border-strong);
  background: var(--color-bg-hover);
}

/* 顶部刷新提示条（保留旧数据时） */
.ldp-stale {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.45rem 0.7rem;
  border-radius: 8px;
  border: 1px solid var(--color-warning);
  background: var(--color-warning-subtle);
  color: var(--color-text-secondary);
  font-size: 0.7rem;
}

.ldp-stale-retry {
  margin-left: auto;
  font-size: 0.68rem;
  padding: 0.15rem 0.5rem;
  border-radius: 6px;
  border: 1px solid var(--color-border-medium);
  background: var(--color-bg-sunken);
  color: var(--color-text-secondary);
  cursor: pointer;
}

.ldp-stale-retry:hover {
  background: var(--color-bg-hover);
}

/* 卡片网格（ui-design §5：minmax(360px,1fr) 自适应） */
.ldp-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  gap: 1rem;
}

.ldp-skeleton {
  height: 220px;
  border-radius: 10px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-hover);
  animation: ldpPulse 1.5s ease-in-out infinite;
}

@keyframes ldpPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

/* 内联错误条 */
.ldp-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.75rem;
  border-radius: 10px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-elevated);
  color: var(--color-text-muted);
  font-size: 0.72rem;
}

.ldp-retry {
  margin-left: auto;
  font-size: 0.68rem;
  padding: 0.15rem 0.5rem;
  border-radius: 6px;
  border: 1px solid var(--color-border-medium);
  background: var(--color-bg-sunken);
  color: var(--color-text-secondary);
  cursor: pointer;
}

.ldp-retry:hover {
  background: var(--color-bg-hover);
}

/* 空态引导框（ui-design §5 + 原型②） */
.ldp-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  padding: 2.2rem 1.4rem;
  border-radius: 10px;
  border: 1px dashed var(--color-border-medium);
  background: var(--color-bg-elevated);
  text-align: center;
}

.ldp-empty-icon {
  color: var(--color-text-muted);
  opacity: 0.5;
}

.ldp-empty-text {
  margin: 0;
  font-size: 0.82rem;
  font-weight: 600;
  color: var(--color-text-primary);
}

.ldp-empty-hint {
  margin: 0;
  font-size: 0.72rem;
  color: var(--color-text-secondary);
}

.ldp-empty-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  margin-top: 0.4rem;
  padding: 0.4rem 1.2rem;
  border-radius: 8px;
  border: 1px solid var(--color-accent);
  background: var(--color-accent-subtle);
  color: var(--color-accent);
  font-size: 0.78rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.12s ease;
}

.ldp-empty-btn:hover:not(:disabled) {
  background: var(--color-accent);
  color: var(--color-text-inverted);
}

.ldp-empty-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* WS 进度（mirror 旧态势版图空态 / DailyReportGenerateDialog 风格） */
.ldp-progress {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  width: 100%;
  max-width: 320px;
  margin-top: 0.35rem;
}

.ldp-progress-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.15rem 0;
}

.ldp-progress-board {
  font-size: 0.68rem;
  color: var(--color-text-secondary);
}

.ldp-progress-status {
  font-size: 0.62rem;
  color: var(--color-text-muted);
}

.ldp-progress-status--generating { color: var(--color-warning); }
.ldp-progress-status--completed { color: var(--color-success); }
.ldp-progress-status--failed { color: var(--color-error); }

/* 候选栏：单列紧凑列表（不做卡片），虚线分隔 */
.ldp-cand {
  margin-top: 0.5rem;
  border-top: 1px dashed var(--color-border-medium);
  padding-top: 0.7rem;
}

.ldp-cand-title {
  font-size: 0.68rem;
  color: var(--color-text-muted);
  margin-bottom: 0.45rem;
}

.ldp-cand-row {
  display: flex;
  align-items: baseline;
  gap: 0.6rem;
  padding: 0.28rem 0.5rem;
  border-radius: 6px;
  cursor: pointer;
}

.ldp-cand-row:hover {
  background: var(--color-bg-hover);
}

.ldp-cand-name {
  flex-shrink: 0;
  font-size: 0.78rem;
  font-weight: 600;
  color: var(--color-text-primary);
}

.ldp-cand-hint {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.72rem;
  color: var(--color-text-secondary);
}

.ldp-cand-date {
  flex-shrink: 0;
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.66rem;
  color: var(--color-text-muted);
}
</style>
