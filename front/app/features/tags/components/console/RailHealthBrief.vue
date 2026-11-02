<script setup lang="ts">
/**
 * 右轨「版块体检」结论卡（rework-workshop-into-radar-and-desk 矩阵阶段 G5）。
 *
 * 两种形态（同一真实数据源，壳层用 useBoardHealth 只取一次）：
 *  - `brief`（右轨常驻）：真实 llm_score 徽章 + 体检时间 + 评语首句两行预览 +
 *    「最新快照 · 非所选历史评分」标注 + 完整依据入口；
 *  - `full`（单级宽工作面）：完整评语、挂载/幽灵/向量计数、评分趋势（真实 ≥2 点才画）。
 *
 * 诚实边界：无快照=等待首轮体检（不补 0、不套别的版块）；刷新失败但保留旧快照时标 stale；
 * 不以雷达关注度冒充健康；幽灵匹配含义沿 flow（direction_mismatch，不计日报、前端默认隐藏）。
 */
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import type { RailHealthSnapshotView } from '~/api/normalizers/consoleRail'

const props = withDefaults(defineProps<{
  snapshot: RailHealthSnapshotView | null
  loading: boolean
  error: string | null
  /** 刷新失败但仍展示上次成功快照（不等于「无数据」）。 */
  stale?: boolean
  score: number | null
  tier: 'good' | 'mid' | 'low' | null
  checkedAtText: string
  trendPolyline: string
  trendSummary: string
  variant?: 'brief' | 'full'
}>(), {
  stale: false,
  variant: 'brief',
})

const emit = defineEmits<{
  retry: []
  openDetail: []
}>()

/** 评语首句（原型口径：首个句末标点；无标点取全句）——只截首句，不改写原文。 */
const firstSentence = computed(() => {
  const comment = props.snapshot?.comment ?? ''
  if (!comment) return ''
  return comment.match(/^[\s\S]*?[。！？]/)?.[0]?.trim() || comment.trim()
})

const scoreText = computed(() => (props.score === null ? null : Math.round(props.score)))
</script>

<template>
  <section class="rhb" data-testid="rail-health">
    <div class="rhb-head">
      <span class="rhb-title">
        <Icon icon="mdi:heart-pulse" width="14" />
        版块体检
      </span>
      <span
        v-if="snapshot && scoreText !== null"
        class="rhb-badge"
        :class="tier ? `rhb-badge--${tier}` : ''"
        data-testid="rail-health-score"
      >{{ scoreText }}</span>
      <span v-else-if="snapshot" class="rhb-badge rhb-badge--pending">待评分</span>
      <span v-else class="rhb-badge rhb-badge--pending" data-testid="rail-health-pending">等待首轮体检</span>
    </div>

    <div v-if="loading && !snapshot" class="rhb-state" data-testid="rail-health-loading">体检数据加载中…</div>
    <div v-else-if="error && !snapshot" class="rhb-error" data-testid="rail-health-error">
      <Icon icon="mdi:alert-circle-outline" width="13" />
      <span>{{ error }}</span>
      <button type="button" class="rhb-link" data-testid="rail-health-retry" @click="emit('retry')">重试</button>
    </div>
    <div v-else-if="!snapshot" class="rhb-state" data-testid="rail-health-empty">
      等待首轮体检——体检在每日日报生成后自动执行，届时此处展示评分与构成质量。
    </div>

    <template v-else>
      <p v-if="stale && error" class="rhb-stale" data-testid="rail-health-stale">
        刷新失败（{{ error }}），下面是上次成功快照
        <button type="button" class="rhb-link" data-testid="rail-health-stale-retry" @click="emit('retry')">重试</button>
      </p>

      <p class="rhb-meta" data-testid="rail-health-checked-at">
        <span v-if="checkedAtText">上次体检 {{ checkedAtText }}</span>
        <span class="rhb-latest">最新快照 · 非所选历史评分</span>
        <button type="button" class="rhb-link rhb-refresh" data-testid="rail-health-refresh" @click="emit('retry')">刷新</button>
      </p>

      <template v-if="variant === 'brief'">
        <p class="rhb-excerpt" data-testid="rail-health-excerpt">
          {{ firstSentence || '本轮体检未带评语。' }}
        </p>
        <button type="button" class="rhb-entry" data-testid="rail-health-detail" @click="emit('openDetail')">
          完整评语与依据
          <Icon icon="mdi:chevron-right" width="13" />
        </button>
      </template>

      <template v-else>
        <blockquote v-if="snapshot.comment" class="rhb-comment" data-testid="rail-health-comment">
          {{ snapshot.comment }}
        </blockquote>
        <p v-else class="rhb-state">本轮体检未带评语。</p>

        <div class="rhb-chips" data-testid="rail-health-chips">
          <span class="rhb-chip">挂载 tags <b>{{ snapshot.matchedTags }}</b></span>
          <span class="rhb-chip" :class="{ 'rhb-chip--warn': snapshot.ghostMatches > 0 }">
            幽灵匹配 <b>{{ snapshot.ghostMatches }}</b>
          </span>
          <span class="rhb-chip">向量缺失 <b>{{ snapshot.vectorMissing }}</b></span>
        </div>

        <div class="rhb-trend">
          <span class="rhb-trend-label">评分趋势 · 按体检轮次</span>
          <svg
            v-if="trendPolyline"
            class="rhb-trend-svg"
            viewBox="0 0 220 44"
            preserveAspectRatio="none"
            role="img"
            :aria-label="trendSummary"
            data-testid="rail-health-trend"
          >
            <polyline :points="trendPolyline" fill="none" stroke="var(--color-accent)" stroke-width="2" />
          </svg>
          <p v-else class="rhb-state">评分点不足，暂无趋势线</p>
          <p class="rhb-trend-summary">{{ trendSummary }}</p>
        </div>

        <p class="rhb-note">
          最新体检快照，不随左侧时间视窗回溯为历史评分；幽灵匹配 = direction_mismatch 挂载计数（不计日报、前端默认隐藏）。
        </p>
      </template>
    </template>
  </section>
</template>

<style scoped>
.rhb {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  min-width: 0;
}

.rhb-head {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  min-width: 0;
}

.rhb-title {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  font-size: 0.78rem;
  font-weight: 600;
  color: var(--color-text-secondary);
  white-space: nowrap;
}

.rhb-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 2.1rem;
  height: 1.25rem;
  padding: 0 0.35rem;
  border-radius: 999px;
  border: 1px solid transparent;
  font-size: 0.68rem;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.rhb-badge--good {
  color: var(--color-success);
  background: var(--color-success-subtle);
  border-color: var(--color-success-subtle);
}

.rhb-badge--mid {
  color: var(--color-warning);
  background: var(--color-warning-subtle);
  border-color: var(--color-warning-subtle);
}

.rhb-badge--low {
  color: var(--color-error);
  background: var(--color-error-subtle);
  border-color: var(--color-error-subtle);
}

.rhb-badge--pending {
  color: var(--color-text-muted);
  background: var(--color-bg-sunken);
  border-color: var(--color-border-subtle);
  font-weight: 500;
  white-space: nowrap;
}

.rhb-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 0.4rem;
  margin: 0;
  font-size: 0.68rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.rhb-latest {
  padding-left: 0.4rem;
  border-left: 1px solid var(--color-border-subtle);
}

.rhb-excerpt {
  margin: 0;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  font-size: 0.76rem;
  line-height: 1.6;
  color: var(--color-text-primary);
}

.rhb-comment {
  margin: 0;
  padding: 0.45rem 0.6rem;
  border-left: 3px solid var(--color-accent);
  background: var(--color-accent-subtle);
  color: var(--color-text-primary);
  font-size: 0.78rem;
  line-height: 1.7;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.rhb-entry {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.4rem;
  width: 100%;
  padding: 0.32rem 0;
  border: 0;
  border-top: 1px solid var(--color-border-subtle);
  background: none;
  color: var(--color-text-secondary);
  font-size: 0.72rem;
  text-align: left;
  cursor: pointer;
}

.rhb-entry:hover,
.rhb-entry:focus-visible {
  color: var(--color-accent);
  outline: none;
}

.rhb-state {
  margin: 0;
  font-size: 0.72rem;
  line-height: 1.6;
  color: var(--color-text-muted);
}

.rhb-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.72rem;
  color: var(--color-error);
}

.rhb-link {
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  padding: 0.1rem 0.4rem;
  background: none;
  color: var(--color-accent);
  font-size: 0.68rem;
  cursor: pointer;
}

.rhb-stale {
  margin: 0;
  font-size: 0.68rem;
  color: var(--color-warning);
}

.rhb-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  margin-top: 0.2rem;
}

.rhb-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.16rem 0.45rem;
  border-radius: 6px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-base);
  font-size: 0.68rem;
  color: var(--color-text-muted);
}

.rhb-chip b {
  color: var(--color-text-primary);
  font-variant-numeric: tabular-nums;
}

.rhb-chip--warn b {
  color: var(--color-warning);
}

.rhb-trend {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  margin-top: 0.3rem;
}

.rhb-trend-label {
  font-size: 0.68rem;
  color: var(--color-text-muted);
}

.rhb-trend-svg {
  width: 100%;
  height: 44px;
}

.rhb-trend-summary {
  margin: 0;
  font-size: 0.7rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.rhb-note {
  margin: 0.3rem 0 0;
  font-size: 0.66rem;
  line-height: 1.6;
  color: var(--color-text-muted);
}
</style>
