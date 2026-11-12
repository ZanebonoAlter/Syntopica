<script setup lang="ts">
import { computed } from 'vue'
import { matchReasonColor, matchInfoLabel } from '~/utils/matchQuality'
import { topicAnchorLabel } from '~/utils/topicAnchor'
import type { DailyReportQualityEntry } from '~/api/dailyReports'

/**
 * Quality probe panel for a daily-report section. Renders two lineage bands:
 *   1. (top) topic-anchor line — System 2 (section ↔ persistent topic):
 *      topic name + cosine distance + chinese tightness label.
 *   2. (below) per-tag quality_breakdown chips — System 1 (tag ↔ board).
 *
 * The anchor line renders only when anchor data is present
 * (confidence ∈ {anchor_hit, auto_new} and a finite positive distance);
 * otherwise it is omitted and the probe falls back to the per-tag list or the
 * "无质量明细" placeholder. Pure display: the parent reveals it on hover/focus.
 */
const props = defineProps<{
  breakdown?: DailyReportQualityEntry[] | null
  topicLabel?: string
  topicDistance?: number
  topicConfidence?: string
}>()

const showAnchor = computed(
  () =>
    (props.topicConfidence === 'anchor_hit' || props.topicConfidence === 'auto_new')
    && typeof props.topicDistance === 'number'
    && Number.isFinite(props.topicDistance)
    && props.topicDistance > 0,
)
const anchorLine = computed(() => {
  if (!showAnchor.value) return null
  const d = props.topicDistance as number
  return {
    label: props.topicLabel || '未命名话题',
    distance: d.toFixed(2),
    tier: topicAnchorLabel(d, props.topicConfidence),
  }
})
const hasBreakdown = computed(() => !!props.breakdown && props.breakdown.length > 0)
</script>

<template>
  <div class="section-quality-explore">
    <p v-if="anchorLine" class="section-quality-explore__anchor">
      🔗 话题锚定 · <span class="section-quality-explore__anchor-label">{{ anchorLine.label }}</span>
      · 距离 {{ anchorLine.distance }} · {{ anchorLine.tier }}
    </p>
    <ul v-if="hasBreakdown" class="section-quality-explore__list">
      <li
        v-for="entry in breakdown"
        :key="entry.tag_id"
        class="section-quality-explore__chip"
        :data-tag-id="entry.tag_id"
        :style="{ borderColor: matchReasonColor(entry.match_reason) }"
      >
        <span class="section-quality-explore__name">{{ entry.label }}</span>
        <span class="section-quality-explore__meta">{{ matchInfoLabel(entry) }}</span>
      </li>
    </ul>
    <p v-else-if="!anchorLine" class="section-quality-explore__empty">无质量明细</p>
  </div>
</template>

<style scoped>
.section-quality-explore {
  min-width: 14rem;
}

.section-quality-explore__list {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.section-quality-explore__chip {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.2rem 0.45rem;
  border: 1px solid;
  border-radius: 4px;
  background: var(--color-bg-hover);
  font-size: 0.68rem;
  line-height: 1.4;
}

.section-quality-explore__name {
  color: var(--color-text-secondary);
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.section-quality-explore__meta {
  flex-shrink: 0;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.section-quality-explore__empty {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 0.68rem;
  font-style: italic;
}

.section-quality-explore__anchor {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 0.25rem;
  margin: 0 0 0.4rem;
  padding-bottom: 0.35rem;
  border-bottom: 1px solid var(--color-border-medium);
  color: var(--color-text-secondary);
  font-size: 0.7rem;
  line-height: 1.4;
  font-variant-numeric: tabular-nums;
}

.section-quality-explore__anchor-label {
  color: var(--color-text-primary);
  font-weight: 500;
}
</style>
