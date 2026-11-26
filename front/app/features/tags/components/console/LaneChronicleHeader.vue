<script setup lang="ts">
/**
 * LaneChronicleHeader — 编年头部（rework-workshop-into-radar-and-desk 矩阵阶段 G4）。
 *
 * 纯展示受控块：泳道 kicker + 日期与「今天」徽标 + 上一/下一有内容日导航（仅 emit 日期，
 * 边界禁用带明确 title）+ 简短真实合计（R26：默认只显当日合计，未知标 ?/≥已知部分；
 * 桶峰值/桶数/口径进 <details> 键盘可展开计数说明，默认折叠不删口径）+
 * 两个保留动作（打开所选日日报——完整日期、latest 主入口在壳层不重复；看本板块此日归档）。
 */
import { Icon } from '@iconify/vue'

defineProps<{
  laneLabel: string
  selectedDate: string | null
  today: string
  prevDate: string | null
  nextDate: string | null
  metricsSummary: string | null
  metricsDetail: string | null
  canOpenReport: boolean
  openReportTitle: string
}>()

const emit = defineEmits<{
  selectDate: [date: string]
  openReport: []
  viewArchive: []
}>()
</script>

<template>
  <header class="lch-head">
    <div class="lch-kicker">
      <span class="lch-kicker-system">当日线索</span>
      <span class="lch-kicker-sep" aria-hidden="true">·</span>
      <span class="lch-lane-label" :title="laneLabel">{{ laneLabel }}</span>
    </div>
    <div class="lch-title-row">
      <h3 class="lch-date" :title="selectedDate ?? undefined">
        {{ selectedDate ?? '未选日期' }}<span v-if="selectedDate && selectedDate === today" class="lch-today">今天</span>
      </h3>
      <nav class="lch-nav" aria-label="前后有内容日">
        <button
          type="button" class="lch-nav-btn" data-testid="lane-chronicle-prev"
          :disabled="prevDate === null"
          :title="prevDate ? `上一有内容日 ${prevDate}` : '已到最早有内容日'"
          @click="prevDate && emit('selectDate', prevDate)"
        >
          <Icon icon="mdi:chevron-left" width="14" />{{ prevDate ? prevDate.slice(5) : '最早' }}
        </button>
        <button
          type="button" class="lch-nav-btn" data-testid="lane-chronicle-next"
          :disabled="nextDate === null"
          :title="nextDate ? `下一有内容日 ${nextDate}` : '已到最新有内容日'"
          @click="nextDate && emit('selectDate', nextDate)"
        >
          {{ nextDate ? nextDate.slice(5) : '最新' }}<Icon icon="mdi:chevron-right" width="14" />
        </button>
      </nav>
    </div>
    <div class="lch-meta-row">
      <span v-if="metricsSummary" class="lch-metrics" data-testid="lane-chronicle-metrics">{{ metricsSummary }}</span>
      <details v-if="metricsDetail" class="lch-metrics-details" data-testid="lane-chronicle-metrics-details">
        <summary class="lch-metrics-summary">计数口径</summary>
        <p class="lch-metrics-detail">{{ metricsDetail }}</p>
      </details>
      <span class="lch-actions">
        <button
          type="button" class="lch-action lch-action--primary" data-testid="lane-chronicle-open-report"
          :disabled="!canOpenReport" :title="openReportTitle" @click="emit('openReport')"
        >
          <Icon icon="mdi:file-document-outline" width="14" />
          打开 {{ selectedDate ? selectedDate.slice(5) : '—' }} 日报
        </button>
        <button
          type="button" class="lch-action" data-testid="lane-chronicle-archive"
          :title="`在归档视图核对本板块 ${selectedDate ?? ''} 文章`" @click="emit('viewArchive')"
        >
          看本板块此日归档
        </button>
      </span>
    </div>
  </header>
</template>

<style scoped>
.lch-head {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.lch-kicker {
  display: flex;
  align-items: baseline;
  gap: 0.28rem;
  min-width: 0;
  font-size: 0.66rem;
  color: var(--color-text-muted);
}

/* 仅系统 kicker 用大字距/大写；业务泳道名保持 API 原始大小写 */
.lch-kicker-system {
  letter-spacing: 0.14em;
  text-transform: uppercase;
  flex: 0 0 auto;
}

.lch-lane-label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-secondary);
}

.lch-title-row {
  display: flex;
  align-items: center;
  gap: 0.6rem;
}

.lch-date {
  margin: 0;
  font-family: var(--font-serif, Georgia, serif);
  font-size: 1.05rem;
  font-weight: 600;
  color: var(--color-text-primary);
  font-variant-numeric: tabular-nums;
}

.lch-today {
  margin-left: 0.4rem;
  font-size: 0.62rem;
  color: var(--color-accent);
  border: 1px solid color-mix(in srgb, var(--color-accent) 40%, transparent);
  border-radius: 3px;
  padding: 0 0.25rem;
  vertical-align: middle;
}

.lch-nav {
  margin-left: auto;
  display: flex;
  gap: 0.3rem;
}

.lch-nav-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.1rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  background: none;
  color: var(--color-text-secondary);
  font-size: 0.7rem;
  padding: 0.18rem 0.4rem;
  font-variant-numeric: tabular-nums;
  cursor: pointer;
}

.lch-nav-btn:disabled {
  opacity: 0.45;
  cursor: default;
}

.lch-meta-row {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  flex-wrap: wrap;
}

.lch-metrics {
  font-size: 0.7rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.lch-metrics-details {
  font-size: 0.7rem;
  color: var(--color-text-muted);
}

.lch-metrics-summary {
  cursor: pointer;
  user-select: none;
  color: var(--color-text-muted);
  list-style: none;
}

.lch-metrics-summary::marker {
  content: '';
}

.lch-metrics-summary::before {
  content: '▸';
  margin-right: 0.2rem;
  display: inline-block;
  color: var(--color-text-muted);
}

.lch-metrics-details[open] .lch-metrics-summary::before {
  transform: rotate(90deg);
}

.lch-metrics-detail {
  margin: 0.15rem 0 0;
  color: var(--color-text-secondary);
  font-variant-numeric: tabular-nums;
}

.lch-actions {
  margin-left: auto;
  display: flex;
  gap: 0.4rem;
}

.lch-action {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  background: none;
  color: var(--color-text-secondary);
  font-size: 0.72rem;
  padding: 0.25rem 0.55rem;
  cursor: pointer;
}

.lch-action:disabled {
  opacity: 0.45;
  cursor: default;
}

.lch-action--primary {
  color: var(--color-accent);
  border-color: color-mix(in srgb, var(--color-accent) 45%, transparent);
}
</style>
