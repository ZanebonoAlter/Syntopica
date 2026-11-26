<script setup lang="ts">
/**
 * 右轨「当日日报」块（console-rails-live-signals design D5/D6）。
 *
 * **跟随矩阵选中日**（唯一日期源；本块内没有任何日期选择器），两层表达：
 *  1. **点评层** = 该日日报的 `highlights`（2–3 条：标题 + 点评正文 + 可选「N 篇来源」）。
 *     MUST NOT 额外显示日报标题 / 摘要——后端写库时 `title = highlights[0].Title`、
 *     `summary = highlights[0].Reason`，重复即重复。
 *  2. **泳道层** = 该日日报里归属正式泳道的分区：泳道名 + 该泳道当日线索数（与矩阵日格
 *     同源同值；取不到显示 `—`，不补 0）+ 一句代表线索。
 *
 * 本组件**不发任何请求**：视图模型由壳层用既有读取链组装（`nowBrief.ts`）。
 * 取数失败只在本块局部降级（提示 + 重试），右轨其余块照常。
 */
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import type { NowBriefModel } from './nowBrief'

const props = defineProps<{
  /** 「当日日报」视图模型（点评层 + 泳道层）。 */
  model: NowBriefModel
}>()

const emit = defineEmits<{
  /** 点泳道行 → 定位矩阵里的该泳道（inline-edit）。 */
  locateLane: [topicId: string]
  /** 点点评标题（可定位时）→ 跳到该期日报正文的对应聚簇。 */
  openReport: [payload: { date: string, clusterId: number | null }]
  /** 详情读取失败 → 受控重试（只重取当日详情，不重建索引）。 */
  retry: []
}>()

/** 块标题所跟日期（MM-DD）。 */
const dateText = computed(() => (props.model.date ? props.model.date.slice(5) : ''))

function openReview(clusterId: number | null): void {
  if (!props.model.date) return
  emit('openReport', { date: props.model.date, clusterId })
}
</script>

<template>
  <section class="rnb" data-testid="rail-now">
    <div class="rnb-head">
      <span class="rnb-title">
        <Icon icon="mdi:newspaper-variant-outline" width="14" />
        当日日报
        <span v-if="dateText" class="rnb-date" data-testid="rail-now-date">· {{ dateText }}</span>
      </span>
    </div>

    <!-- 加载骨架：2 条点评灰条 + 2 条泳道灰条（不新增共享 skeleton 组件） -->
    <div v-if="model.status === 'loading'" class="rnb-skeleton" data-testid="rail-now-loading">
      <span class="rnb-skeleton-line rnb-skeleton-line--wide" />
      <span class="rnb-skeleton-line" />
      <span class="rnb-skeleton-line rnb-skeleton-line--wide" />
      <span class="rnb-skeleton-line" />
    </div>

    <div v-else-if="model.status === 'error'" class="rnb-error" data-testid="rail-now-error">
      <Icon icon="mdi:alert-circle-outline" width="13" />
      <span>{{ model.error ?? '日报详情加载失败' }}</span>
      <button type="button" class="rnb-link" data-testid="rail-now-retry" @click="emit('retry')">重试</button>
    </div>

    <p v-else-if="model.status === 'no-report'" class="rnb-hint" data-testid="rail-now-empty">
      这一天没有日报
    </p>

    <template v-else>
      <ul v-if="model.reviews.length > 0" class="rnb-reviews" data-testid="rail-now-reviews">
        <li
          v-for="(review, index) in model.reviews"
          :key="index"
          class="rnb-review"
          :style="{ '--i': String(index) }"
          :data-testid="`rail-now-review-item-${index}`"
        >
          <button
            v-if="review.title && review.clusterId !== null"
            type="button"
            class="rnb-review-title"
            data-testid="rail-now-review-title"
            title="定位到本期正文中该聚簇的第一条线索"
            @click="openReview(review.clusterId)"
          >{{ review.title }}</button>
          <span v-else-if="review.title" class="rnb-review-title">{{ review.title }}</span>
          <p v-if="review.reason" class="rnb-review-reason">{{ review.reason }}</p>
          <span v-if="review.articleCount !== null" class="rnb-review-meta">
            {{ review.articleCount }} 篇来源
          </span>
        </li>
      </ul>
      <p v-else class="rnb-hint" data-testid="rail-now-no-highlights">本期暂无重点</p>

      <div v-if="model.lanes.length > 0" class="rnb-lane-sub">各泳道</div>
      <ul v-if="model.lanes.length > 0" class="rnb-lanes" data-testid="rail-now-lanes">
        <li
          v-for="(lane, index) in model.lanes"
          :key="lane.topicId"
          class="rnb-lane"
          :style="{ '--i': String(index) }"
          :data-testid="`rail-now-lane-item-${lane.topicId}`"
        >
          <button
            type="button"
            class="rnb-lane-row"
            data-testid="rail-now-lane-row"
            :title="lane.sentence"
            @click="emit('locateLane', lane.topicId)"
          >
            <span class="rnb-lane-dot" :style="{ background: lane.color }" />
            <span class="rnb-lane-name">{{ lane.label }}</span>
            <span
              class="rnb-lane-count"
              :class="{ 'is-unknown': lane.clueCount === null }"
              data-testid="rail-now-lane-count"
            >{{ lane.clueCount === null ? '—' : `${lane.clueCount} 条` }}</span>
            <span class="rnb-lane-sentence">{{ lane.sentence }}</span>
          </button>
        </li>
      </ul>
      <p v-else class="rnb-hint" data-testid="rail-now-no-lanes">当日没有归属泳道的分区</p>
    </template>
  </section>
</template>

<style scoped>
.rnb {
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
  min-width: 0;
}

.rnb-head {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
  min-width: 0;
}

.rnb-title {
  display: inline-flex;
  align-items: baseline;
  gap: 0.3rem;
  font-size: 0.74rem;
  font-weight: 600;
  color: var(--color-text-secondary);
  white-space: nowrap;
}

.rnb-date {
  font-size: 0.7rem;
  font-weight: 500;
  color: var(--color-accent);
  font-variant-numeric: tabular-nums;
}

/* 点评行：错峰淡入（不做位移动画）。 */
.rnb-review,
.rnb-lane {
  opacity: 0;
  animation: rnb-fade-in 220ms ease forwards;
  animation-delay: calc(var(--i, 0) * 50ms);
}

@keyframes rnb-fade-in {
  to { opacity: 1; }
}

.rnb-reviews,
.rnb-lanes {
  display: flex;
  flex-direction: column;
  gap: 0.55rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.rnb-review {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  min-width: 0;
}

.rnb-review-title {
  border: 0;
  padding: 0;
  background: none;
  font: inherit;
  font-size: 0.86rem;
  line-height: 1.4;
  text-align: left;
  color: var(--color-text-primary);
}

button.rnb-review-title {
  cursor: pointer;
}

button.rnb-review-title:hover {
  color: var(--color-accent);
}

.rnb-review-reason {
  margin: 0;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  font-size: 0.72rem;
  line-height: 1.55;
  color: var(--color-text-muted);
}

.rnb-review-meta {
  font-size: 0.66rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.rnb-lane-sub {
  margin-top: 0.2rem;
  padding-top: 0.5rem;
  border-top: 1px solid var(--color-border-subtle);
  font-size: 0.66rem;
  letter-spacing: 0.1em;
  color: var(--color-text-muted);
}

.rnb-lane-row {
  display: grid;
  grid-template-columns: 6px minmax(0, 1fr) auto;
  gap: 0.1rem 0.45rem;
  align-items: baseline;
  width: 100%;
  min-width: 0;
  padding: 0;
  border: 0;
  background: none;
  text-align: left;
  cursor: pointer;
}

.rnb-lane-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  align-self: center;
  grid-row: 1;
  grid-column: 1;
}

.rnb-lane-name {
  grid-row: 1;
  grid-column: 2;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.76rem;
  color: var(--color-text-primary);
}

.rnb-lane-row:hover .rnb-lane-name {
  color: var(--color-accent);
}

.rnb-lane-count {
  grid-row: 1;
  grid-column: 3;
  font-size: 0.68rem;
  color: var(--color-accent);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.rnb-lane-count.is-unknown {
  color: var(--color-text-muted);
}

.rnb-lane-sentence {
  grid-row: 2;
  grid-column: 2 / -1;
  min-width: 0;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  font-size: 0.68rem;
  line-height: 1.5;
  color: var(--color-text-muted);
}

.rnb-hint {
  margin: 0;
  font-size: 0.7rem;
  line-height: 1.6;
  color: var(--color-text-muted);
}

.rnb-error {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.7rem;
  color: var(--color-error);
}

.rnb-link {
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  padding: 0.1rem 0.4rem;
  background: none;
  color: var(--color-accent);
  font-size: 0.68rem;
  cursor: pointer;
}

.rnb-skeleton {
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
  padding: 0.2rem 0;
}

.rnb-skeleton-line {
  height: 0.8rem;
  border-radius: 4px;
  background: var(--color-bg-hover);
  animation: rnb-pulse 1.4s ease-in-out infinite;
}

.rnb-skeleton-line--wide {
  height: 1rem;
  width: 80%;
}

@keyframes rnb-pulse {
  0%, 100% { opacity: 0.55; }
  50% { opacity: 1; }
}

@media (prefers-reduced-motion: reduce) {
  .rnb-review,
  .rnb-lane {
    animation: none;
    opacity: 1;
  }

  .rnb-skeleton-line {
    animation: none;
  }
}
</style>
