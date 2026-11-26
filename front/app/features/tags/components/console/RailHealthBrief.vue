<script setup lang="ts">
/**
 * 右轨「版块体检」结论卡（rework-workshop-into-radar-and-desk 矩阵阶段 G5；
 * console-rails-live-signals 把 brief 瘦身为一行状态条）。
 *
 * 两种形态（同一真实数据源，壳层用 useBoardHealth 只取一次）：
 *  - `brief`（右轨常驻）：**一行状态条** = 评分徽章 + 上次体检时间 + 刷新 + 评语入口；
 *    **不常驻评语首句**（评语在 `full` 变体 / 体检详情里仍完整可见，信息不丢）；
 *    「最新快照」口径属详情，右轨不重复（≤ simplify-workshop-matrix-copy）。
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

const scoreText = computed(() => (props.score === null ? null : Math.round(props.score)))
</script>

<template>
  <section class="rhb" data-testid="rail-health">
    <!-- brief：右轨常驻的一行状态条（徽章 + 体检时间 + 刷新 + 评语入口） -->
    <template v-if="variant === 'brief'">
      <div class="rhb-head">
        <span class="rhb-title">
          <Icon icon="mdi:heart-pulse" width="14" />
          版块体检
        </span>
        <button
          v-if="snapshot"
          type="button"
          class="rhb-badge rhb-badge--btn"
          :class="tier ? `rhb-badge--${tier}` : 'rhb-badge--pending'"
          :data-testid="scoreText !== null ? 'rail-health-score' : 'rail-health-badge'"
          title="查看完整评语与依据"
          @click="emit('openDetail')"
        >{{ scoreText !== null ? scoreText : '待评分' }}</button>
        <span v-else class="rhb-badge rhb-badge--pending" data-testid="rail-health-pending">等待首轮体检</span>

        <span class="rhb-meta">
          <span v-if="loading && !snapshot" class="rhb-meta-text" data-testid="rail-health-loading">
            体检数据加载中…
          </span>
          <span v-else-if="error && !snapshot" class="rhb-error" data-testid="rail-health-error">
            <Icon icon="mdi:alert-circle-outline" width="13" />
            <span>{{ error }}</span>
          </span>
          <span v-else-if="checkedAtText" class="rhb-meta-text" data-testid="rail-health-checked-at">
            上次体检 {{ checkedAtText }}
          </span>
          <button
            v-if="error && !snapshot"
            type="button"
            class="rhb-link"
            data-testid="rail-health-retry"
            @click="emit('retry')"
          >重试</button>
          <button
            v-if="snapshot && !loading"
            type="button"
            class="rhb-link rhb-refresh rhb-icon-btn"
            data-testid="rail-health-refresh"
            title="刷新体检快照"
            aria-label="刷新体检快照"
            @click="emit('retry')"
          ><Icon icon="mdi:refresh" width="12" /></button>
        </span>
      </div>

      <p v-if="stale && error" class="rhb-stale" data-testid="rail-health-stale">
        刷新失败（{{ error }}），下面是上次成功快照
        <button type="button" class="rhb-link" data-testid="rail-health-stale-retry" @click="emit('retry')">重试</button>
      </p>
    </template>

    <!-- full：单级宽工作面（完整评语 / 依据 / 趋势） -->
    <template v-else>
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
  flex: 0 0 auto;
  font-size: 0.74rem;
  font-weight: 600;
  color: var(--color-text-secondary);
  white-space: nowrap;
}

.rhb-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  min-width: 2.1rem;
  height: 1.25rem;
  padding: 0 0.35rem;
  border-radius: 999px;
  border: 1px solid transparent;
  font-size: 0.68rem;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

/* 徽章即评语入口（有快照时可点进详情）：只补可点击反馈，不改分档配色。 */
.rhb-badge--btn {
  cursor: pointer;
  font-family: inherit;
  transition: filter 0.15s ease;
}

.rhb-badge--btn:hover {
  filter: brightness(1.15);
}

.rhb-badge--btn:focus-visible {
  outline: 2px solid var(--color-accent);
  outline-offset: 1px;
}

/* 刷新为图标钮：右轨 288px 下要给「上次体检 MM/DD HH:mm」留完整宽度。 */
.rhb-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 1.1rem;
  height: 1.1rem;
  padding: 0;
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
  align-items: baseline;
  flex: 1 1 auto;
  min-width: 0;
  gap: 0.4rem;
  font-size: 0.68rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.rhb-meta-text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rhb-latest {
  padding-left: 0.4rem;
  border-left: 1px solid var(--color-border-subtle);
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

.rhb-link {
  flex: 0 0 auto;
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  padding: 0.1rem 0.4rem;
  background: none;
  color: var(--color-accent);
  font-size: 0.68rem;
  cursor: pointer;
  white-space: nowrap;
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
  gap: 0.3rem;
  min-width: 0;
  font-size: 0.7rem;
  color: var(--color-error);
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
