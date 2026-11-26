<script setup lang="ts">
/**
 * 右轨「来源构成」结论卡（rework-workshop-into-radar-and-desk 矩阵阶段 G5）。
 *
 * 口径唯一实现仍是 `useBoardSources`（sourcestats 读链，本组件只渲染、不复制公式）：
 * 独立滚动统计窗 7/30/90（**不随左侧观察日期/档位重算**），贡献条 = 各来源占本板块份额。
 * 未取到（加载失败/空响应）如实标未取数，不补 0、不套用别的窗口；全量表格与 feed_hit_rate
 * 口径在单级宽工作面（≥1040px）里按需查看，不在右轨重复教学。
 */
import { computed, toRef } from 'vue'
import { Icon } from '@iconify/vue'
import { normalizeRailSourceBreakdown } from '~/api/normalizers/consoleRail'
import { useBoardSources } from '../../composables/useBoardSources'

const props = withDefaults(defineProps<{
  boardId: number
  /** brief=右轨 top3；full=单级宽工作面（列全部来源 + 口径说明）。 */
  variant?: 'brief' | 'full'
}>(), {
  variant: 'brief',
})

const emit = defineEmits<{
  openDetail: []
}>()

const {
  STATS_WINDOWS,
  windowDays,
  breakdown,
  loading,
  error,
  retry,
} = useBoardSources(toRef(props, 'boardId'), { window: 7 })

/** API 边界归一：组件只消费 camelCase/字符串 feedId，不散落 raw DTO 字段。 */
const view = computed(() => normalizeRailSourceBreakdown(breakdown.value))
const sources = computed(() => {
  const list = [...(view.value?.sources ?? [])]
  return list.sort((a, b) => b.articles - a.articles)
})
const totalArticles = computed(() => view.value?.totalArticles ?? 0)
const sourceCount = computed(() => view.value?.sourceCount ?? 0)

function shareText(share: number): string {
  const pct = Math.round(share * 100)
  return share > 0 && pct === 0 ? '<1%' : `${pct}%`
}

const TOP_LIMIT = 3
</script>

<template>
  <section class="rsb" data-testid="rail-sources">
    <div class="rsb-head">
      <span class="rsb-title">
        <Icon icon="mdi:source-branch" width="14" />
        来源构成
      </span>
      <div class="rsb-windows" role="group" aria-label="来源独立滚动统计窗">
        <button
          v-for="w in STATS_WINDOWS"
          :key="w"
          type="button"
          class="rsb-window"
          :class="{ 'is-on': windowDays === w }"
          :aria-pressed="windowDays === w"
          :data-testid="`rail-source-window-${w}`"
          @click="windowDays = w"
        >{{ w }}日</button>
      </div>
    </div>

    <p class="rsb-caption" data-testid="rail-sources-scope">独立于时间视窗</p>

    <div v-if="loading" class="rsb-state" data-testid="rail-sources-loading">来源统计加载中…</div>
    <div v-else-if="error" class="rsb-error" data-testid="rail-sources-error">
      <Icon icon="mdi:alert-circle-outline" width="13" />
      <span>{{ error }}</span>
      <button type="button" class="rsb-link" data-testid="rail-sources-retry" @click="retry">重试</button>
    </div>
    <div v-else-if="!breakdown" class="rsb-state" data-testid="rail-sources-unavailable">
      本板块此档来源未取数——可切换窗口或稍后重试；不补 0。
    </div>
    <div v-else-if="totalArticles === 0" class="rsb-state" data-testid="rail-sources-empty">
      近 {{ windowDays }} 天没有文章归入本版块。
    </div>

    <template v-else>
      <div
        class="rsb-composition"
        role="img"
        :aria-label="`本板块来源贡献，共 ${totalArticles} 篇`"
        data-testid="rail-sources-composition"
      >
        <span
          v-for="(source, index) in sources"
          :key="source.feedId"
          class="rsb-segment"
          :class="`rsb-segment--${index % 4}`"
          :style="{ width: `${Math.max(0, Math.min(100, source.share * 100))}%` }"
          :title="`${source.title} · ${source.articles} 篇`"
        />
      </div>
      <p class="rsb-totals" data-testid="rail-sources-totals">
        <b>{{ totalArticles }}</b> 篇入本板块 · <b>{{ sourceCount }}</b> 个来源
      </p>

      <div class="rsb-rows">
        <div
          v-for="source in sources.slice(0, variant === 'full' ? sources.length : TOP_LIMIT)"
          :key="source.feedId"
          class="rsb-row"
          :data-testid="`rail-source-row-${source.feedId}`"
        >
          <span class="rsb-row-title" :title="source.title">{{ source.title }}</span>
          <span class="rsb-row-count">{{ source.articles }} 篇</span>
          <span class="rsb-row-share">{{ shareText(source.share) }}</span>
        </div>
      </div>

      <template v-if="variant === 'brief' && sources.length > TOP_LIMIT">
        <p class="rsb-more">另有 {{ sources.length - TOP_LIMIT }} 个来源</p>
      </template>

      <button type="button" class="rsb-entry" data-testid="rail-sources-detail" @click="emit('openDetail')">
        来源明细与口径
        <Icon icon="mdi:chevron-right" width="13" />
      </button>
    </template>
  </section>
</template>

<style scoped>
.rsb {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  min-width: 0;
}

.rsb-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.4rem;
  flex-wrap: wrap;
}

.rsb-title {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  font-size: 0.78rem;
  font-weight: 600;
  color: var(--color-text-secondary);
  white-space: nowrap;
}

.rsb-windows {
  display: flex;
  gap: 0.15rem;
}

.rsb-window {
  padding: 0.1rem 0.35rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 6px;
  background: none;
  color: var(--color-text-muted);
  font-size: 0.66rem;
  cursor: pointer;
}

.rsb-window.is-on {
  border-color: var(--color-accent);
  color: var(--color-accent);
  background: var(--color-accent-subtle);
}

.rsb-caption {
  margin: 0;
  font-size: 0.66rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.rsb-composition {
  display: flex;
  width: 100%;
  height: 0.4rem;
  margin-top: 0.15rem;
  border-radius: 999px;
  overflow: hidden;
  background: var(--color-bg-hover);
}

.rsb-segment {
  display: block;
  height: 100%;
}

.rsb-segment--0 { background: var(--color-accent); }
.rsb-segment--1 { background: var(--color-info, var(--color-accent)); opacity: 0.72; }
.rsb-segment--2 { background: var(--color-warning); }
.rsb-segment--3 { background: var(--color-text-muted); }

.rsb-totals {
  margin: 0;
  font-size: 0.7rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.rsb-totals b {
  color: var(--color-text-primary);
}

.rsb-rows {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
}

.rsb-row {
  display: flex;
  align-items: baseline;
  gap: 0.35rem;
  font-size: 0.7rem;
  line-height: 1.5;
  min-width: 0;
}

.rsb-row-title {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-secondary);
}

.rsb-row-count,
.rsb-row-share {
  flex: 0 0 auto;
  font-variant-numeric: tabular-nums;
  color: var(--color-text-muted);
}

.rsb-more {
  margin: 0;
  font-size: 0.66rem;
  color: var(--color-text-muted);
}

.rsb-entry {
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

.rsb-entry:hover,
.rsb-entry:focus-visible {
  color: var(--color-accent);
  outline: none;
}

.rsb-state {
  margin: 0;
  font-size: 0.72rem;
  line-height: 1.6;
  color: var(--color-text-muted);
}

.rsb-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.72rem;
  color: var(--color-error);
}

.rsb-link {
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  padding: 0.1rem 0.4rem;
  background: none;
  color: var(--color-accent);
  font-size: 0.68rem;
  cursor: pointer;
}
</style>
