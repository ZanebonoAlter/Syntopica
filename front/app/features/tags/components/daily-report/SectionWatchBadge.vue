<script setup lang="ts">
import { computed } from 'vue'

/**
 * 关注物化板块徽标（watch-materialized-topic；watch-materialize-llm-adjudication
 * 加 watch 名装饰）。
 *
 * lane_tier = watch_keyword（关键字物化，临时板块）或 watch_sentence（一句话
 * 物化，持久话题线）。纯展示：小圆点 + 可选 watch 名小字（watch_label 由日报
 * 详情读路径透出，LLM 当日标题之外始终可识别追踪源），色彩走主题 token，
 * 与 SectionTierBadge 的视觉语言一致（色点、无数字）。
 */
const props = defineProps<{ laneTier: string, watchLabel?: string | null }>()

const META: Record<string, { color: string, label: string }> = {
  watch_keyword: { color: 'var(--color-tag-keyword, #b45309)', label: '关键字物化板块' },
  watch_sentence: { color: 'var(--color-accent, #2563eb)', label: '一句话物化话题' },
}

const meta = computed(() => META[props.laneTier] ?? META.watch_keyword!)

/** 装饰文案：『关键字物化板块 · harness』（watch_label 缺失时退化为纯类型标签）。 */
const decoration = computed(() => {
  const name = props.watchLabel?.trim()
  return name ? `${meta.value.label} · ${name}` : meta.value.label
})
</script>

<template>
  <span
    class="section-watch-badge"
    :data-lane-tier="laneTier"
    :style="{ color: meta.color }"
    :title="decoration"
    role="img"
    :aria-label="decoration"
  >
    <span class="section-watch-badge__dot" :style="{ backgroundColor: meta.color }" />
    <span v-if="watchLabel?.trim()" class="section-watch-badge__name">{{ watchLabel.trim() }}</span>
  </span>
</template>

<style scoped>
.section-watch-badge {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  gap: 0.25rem;
  vertical-align: middle;
}

.section-watch-badge__dot {
  display: inline-block;
  width: 0.5rem;
  height: 0.5rem;
  border-radius: 2px; /* 方形角点：与圆形 tier 徽标形成家族区分 */
}

.section-watch-badge__name {
  font-size: 0.75rem;
  line-height: 1;
  opacity: 0.85; /* 小装饰：弱于板块标题，强于悬浮说明 */
}
</style>
