<script setup lang="ts">
/**
 * 治理信号区（rework-workshop-into-radar-and-desk task 0.3 / 3.1）：
 * 体检评分 + 分档色 + 趋势迷你条 + 趋势摘要 + 挂载/幽灵/向量缺失 + 未收编候选 topN（含收编）。
 *
 * 从 `BoardHealthPanel` 拆出的独立可渲染区块：只读展示 + 收编动作外抛
 * （`capture` 由宿主调 `useBoardHealth().capture`，保证面板/右轨同一份逻辑）。
 * 数据 props 全部由 `useBoardHealth` 派生，组件内不算口径。
 */
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import type { BoardHealthCandidate, BoardHealthSnapshotDTO } from '~/api/semanticBoards'
import AppButton from '~/components/ui/AppButton.vue'
import AppSectionHeader from '~/components/ui/AppSectionHeader.vue'

const props = withDefaults(defineProps<{
  snapshot: BoardHealthSnapshotDTO | null
  loading: boolean
  error: string | null
  score: number | null
  tier: 'good' | 'mid' | 'low' | null
  checkedAtText: string
  trendPolyline: string
  trendSummary: string
  capturingId?: number | null
  /** 候选展示条数（右轨 3；宿主需要全量时传更大值）。 */
  candidateLimit?: number
  /** 是否渲染候选区（面板把候选全量交给明细区，避免重复）。 */
  showCandidates?: boolean
  /** 是否显示「查看全部治理」入口（右轨 true；面板自身即全量）。 */
  showDetailEntry?: boolean
}>(), {
  capturingId: null,
  candidateLimit: 3,
  showCandidates: true,
  showDetailEntry: false,
})

const emit = defineEmits<{
  retry: []
  capture: [candidate: BoardHealthCandidate]
  openDetail: []
}>()

const boardMetrics = computed(() => props.snapshot?.metrics.board)
const candidates = computed(() => {
  const list = props.snapshot?.uncaptured_candidates ?? []
  return list.slice(0, props.candidateLimit)
})

function candidateDisabled(auxId: number): boolean {
  return props.capturingId !== null
}

function onCapture(c: BoardHealthCandidate) {
  if (candidateDisabled(c.aux_id)) return
  emit('capture', c)
}
</script>

<template>
  <section class="bhp" data-testid="board-health-signal">
    <div class="bhp-header">
      <Icon icon="mdi:heart-pulse" width="15" class="bhp-header-icon" />
      <span class="bhp-title">版块体检</span>
      <span
        v-if="snapshot?.llm_score != null"
        class="bhp-badge"
        :class="tier ? `bhp-badge--${tier}` : ''"
        data-testid="board-health-score"
      >{{ Math.round(snapshot.llm_score) }}</span>
      <span v-else-if="snapshot" class="bhp-badge bhp-badge--pending">待评分</span>
      <span v-else class="bhp-badge bhp-badge--pending">等待首轮体检</span>
      <span v-if="checkedAtText" class="bhp-checked-at">上次体检 {{ checkedAtText }}</span>
    </div>

    <!-- 体检区：loading / error / empty / success 四态独立于构成管理 -->
    <div v-if="loading" class="bhp-health-loading" data-testid="board-health-loading">
      体检数据加载中…
    </div>
    <div v-else-if="error" class="bhp-health-error" data-testid="board-health-error">
      <Icon icon="mdi:alert-circle-outline" width="14" />
      <span>{{ error }}</span>
      <button type="button" class="bhp-retry" @click="emit('retry')">重试</button>
    </div>
    <div v-else-if="!snapshot" class="bhp-health-empty" data-testid="board-health-empty">
      等待首轮体检——体检在每日日报生成后自动执行，届时此处展示评分、构成质量与未收编候选。
    </div>
    <template v-else>
      <div v-if="snapshot.llm_comment" class="bhp-comment" data-testid="board-health-comment">
        <Icon icon="mdi:comment-quote-outline" width="14" />
        <span>{{ snapshot.llm_comment }}</span>
      </div>

      <div class="bhp-chips" data-testid="board-health-chips">
        <span class="bhp-chip">挂载 tags <b>{{ boardMetrics?.matched_tags ?? 0 }}</b></span>
        <span class="bhp-chip" :class="{ 'bhp-chip--warn': (boardMetrics?.ghost_matches ?? 0) > 0 }">
          幽灵匹配 <b>{{ boardMetrics?.ghost_matches ?? 0 }}</b>
        </span>
        <span class="bhp-chip">向量缺失 <b>{{ boardMetrics?.vector_missing ?? 0 }}</b></span>
      </div>

      <!-- 趋势迷你条：<2 个评分点显示文案，不画假线 -->
      <div class="bhp-trend-block">
        <AppSectionHeader title="评分趋势" icon-name="mdi:chart-line" />
        <svg v-if="trendPolyline" class="bhp-trend" viewBox="0 0 220 44" preserveAspectRatio="none" data-testid="board-health-trend">
          <polyline :points="trendPolyline" fill="none" stroke="var(--color-accent)" stroke-width="2" />
        </svg>
        <div v-else class="bhp-trend-empty">评分点不足，暂无趋势线</div>
        <div class="bhp-trend-summary">{{ trendSummary }}</div>
      </div>

      <div v-if="showCandidates" class="bhp-candidates">
        <AppSectionHeader
          title="未收编候选"
          icon-name="mdi:tag-plus-outline"
          description="成员 tags 高频携带、但不在版块构成中的辅助标签"
        />
        <div v-if="candidates.length === 0" class="bhp-cand-empty">暂无未收编候选</div>
        <div v-else class="bhp-cand-list" data-testid="board-health-candidates">
          <div
            v-for="c in candidates"
            :key="c.aux_id"
            class="bhp-cand-item"
            :data-testid="`board-health-candidate-${c.aux_id}`"
          >
            <div class="bhp-cand-main">
              <span class="bhp-cand-label">{{ c.label }}</span>
              <span class="bhp-cand-evidence">{{ c.member_tags }} tags · {{ c.member_articles }} 篇</span>
            </div>
            <AppButton
              variant="secondary"
              size="sm"
              :loading="capturingId === c.aux_id"
              :disabled="candidateDisabled(c.aux_id)"
              :data-testid="`board-health-capture-${c.aux_id}`"
              @click="onCapture(c)"
            >
              收编
            </AppButton>
          </div>
        </div>
      </div>

      <button
        v-if="showDetailEntry"
        type="button"
        class="bhp-detail-entry"
        @click="emit('openDetail')"
      >
        查看全部治理
        <Icon icon="mdi:chevron-right" width="14" />
      </button>
    </template>
  </section>
</template>

<style scoped>
.bhp {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.bhp-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.bhp-header-icon {
  color: var(--color-text-muted);
}

.bhp-title {
  font-size: 0.78rem;
  font-weight: 600;
  color: var(--color-text-secondary);
}

.bhp-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 34px;
  height: 20px;
  padding: 0 8px;
  border-radius: 10px;
  border: 1px solid transparent;
  font-size: 11px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.bhp-badge--good {
  color: var(--color-success);
  background: var(--color-success-subtle);
  border-color: var(--color-success-subtle);
}

.bhp-badge--mid {
  color: var(--color-warning);
  background: var(--color-warning-subtle);
  border-color: var(--color-warning-subtle);
}

.bhp-badge--low {
  color: var(--color-error);
  background: var(--color-error-subtle);
  border-color: var(--color-error-subtle);
}

.bhp-badge--pending {
  color: var(--color-text-muted);
  background: var(--color-bg-sunken);
  border-color: var(--color-border-subtle);
  font-weight: 500;
}

.bhp-checked-at {
  font-size: 11px;
  color: var(--color-text-muted);
}

.bhp-health-loading {
  padding: 0.5rem 0.6rem;
  border-radius: 8px;
  background: var(--color-bg-sunken);
  font-size: 0.74rem;
  color: var(--color-text-muted);
}

.bhp-health-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.5rem 0.6rem;
  border-radius: 8px;
  border: 1px solid var(--color-error-subtle);
  background: var(--color-error-subtle);
  color: var(--color-error);
  font-size: 0.74rem;
}

.bhp-retry {
  margin-left: auto;
  border: 1px solid var(--color-error);
  background: none;
  color: var(--color-error);
  border-radius: 6px;
  padding: 0.1rem 0.5rem;
  font-size: 0.7rem;
  cursor: pointer;
}

.bhp-health-empty {
  padding: 0.6rem 0.7rem;
  border-radius: 8px;
  background: var(--color-bg-sunken);
  border: 1px dashed var(--color-border-medium);
  font-size: 0.74rem;
  color: var(--color-text-muted);
  line-height: 1.6;
}

.bhp-comment {
  display: flex;
  align-items: flex-start;
  gap: 0.35rem;
  padding: 0.45rem 0.6rem;
  border-radius: 8px;
  border-left: 3px solid var(--color-accent);
  background: var(--color-accent-subtle);
  color: var(--color-text-primary);
  font-size: 0.74rem;
  line-height: 1.5;
}

.bhp-comment span {
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.bhp-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
}

.bhp-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.18rem 0.5rem;
  border-radius: 8px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-base);
  font-size: 0.7rem;
  color: var(--color-text-muted);
}

.bhp-chip b {
  color: var(--color-text-primary);
  font-variant-numeric: tabular-nums;
  font-weight: 600;
}

.bhp-chip--warn {
  border-color: var(--color-warning-subtle);
}

.bhp-chip--warn b {
  color: var(--color-warning);
}

.bhp-trend-block {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.bhp-trend {
  width: 100%;
  height: 44px;
}

.bhp-trend-empty,
.bhp-trend-summary,
.bhp-cand-empty {
  font-size: 0.72rem;
  color: var(--color-text-muted);
}

.bhp-trend-summary {
  font-variant-numeric: tabular-nums;
}

.bhp-candidates {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.bhp-cand-list {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

.bhp-cand-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.35rem 0.6rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-base);
}

.bhp-cand-main {
  flex: 1;
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  min-width: 0;
}

.bhp-cand-label {
  font-size: 0.76rem;
  font-weight: 600;
  color: var(--color-text-primary);
}

.bhp-cand-evidence {
  font-size: 0.68rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.bhp-detail-entry {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.4rem;
  padding: 0.35rem 0.5rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: none;
  color: var(--color-text-secondary);
  font-size: 0.72rem;
  cursor: pointer;
}

.bhp-detail-entry:hover {
  border-color: var(--color-border-medium);
  color: var(--color-text-primary);
  background: var(--color-bg-hover);
}
</style>
