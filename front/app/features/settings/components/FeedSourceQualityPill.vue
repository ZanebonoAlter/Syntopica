<script setup lang="ts">
/**
 * FeedSourceQualityPill — 列表行入板块率指标 pill（add-source-board-hit-rate §4.2）。
 *
 * 三档配色（spec：≥60% 高 / 30–60% 中 / <30% 低）+ 三特殊态
 * （样本少 <5 篇 / 无新文 0 篇 / 打标关闭，后三者不显示百分比）
 * + pending「—」占位（固定 min-width，加载与失败态不抖动布局）。
 * 配色取既有语义 token，圆角/字号沿用既有 tag pill 规范。
 */
import { computed } from 'vue'
import { pillStateLabel, resolvePillState } from '../utils/sourceQuality'

const props = withDefaults(defineProps<{
  /** 窗口内文章总量（上下文样本数）；undefined = 无数据（加载中/失败/未覆盖） */
  articles?: number
  /** 入板块率（0-1） */
  hitRate?: number
  /** 打标关闭（false → 灰底「打标关闭」，不显示 0%）；undefined 视为开启 */
  taggingEnabled?: boolean
  /** 统计聚合请求进行中 → 「—」占位 */
  loading?: boolean
}>(), {
  articles: undefined,
  hitRate: undefined,
  taggingEnabled: undefined,
  loading: false,
})

const state = computed(() => resolvePillState({
  articles: props.articles,
  hit_rate: props.hitRate,
  tagging_enabled: props.taggingEnabled,
}, props.loading))
const label = computed(() => pillStateLabel(state.value, props.hitRate))
</script>

<template>
  <span
    class="feed-source-pill"
    :class="`feed-source-pill--${state}`"
    :data-state="state"
    :title="state === 'off' ? '该源已关闭打标，不参与板块归属' : undefined"
  >{{ label }}</span>
</template>

<style scoped>
.feed-source-pill {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  height: 18px;
  min-width: 44px;
  padding: 0 7px;
  border-radius: 9px;
  border: 1px solid transparent;
  font-size: 10px;
  font-weight: 600;
  line-height: 1;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
  flex: none;
}

.feed-source-pill--high {
  color: var(--color-success);
  background: var(--color-success-subtle);
  border-color: rgba(61, 138, 74, 0.25);
}

.feed-source-pill--mid {
  color: var(--color-text-secondary);
  background: var(--color-bg-sunken);
  border-color: var(--color-border-medium);
}

.feed-source-pill--low {
  color: var(--color-warning);
  background: var(--color-warning-subtle);
  border-color: rgba(196, 136, 60, 0.3);
}

.feed-source-pill--off,
.feed-source-pill--pending,
.feed-source-pill--empty,
.feed-source-pill--low-sample {
  color: var(--color-text-muted);
  background: var(--color-bg-sunken);
  border-color: var(--color-border-subtle);
}
</style>
