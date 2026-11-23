<script setup lang="ts">
/**
 * 关键事件单条标签（add-key-event-matrix-view design D4）。
 *
 * 受控展示：位置（left/top）由父（KeyEventTimelineView 两阶段布局结果）注入；
 * 本体 `white-space: nowrap` 自然宽度渲染、**不加 text-overflow**（标签不截断标题），
 * 宽度由父在第一阶段用 `offsetWidth` 实测后交给贪心分层。点击转发 select（选日）。
 * 颜色全部走 `--color-*` 语义变量（双主题，D13）。
 */
import type { BoardKeyEventItem } from '~/api/boardKeyEvents'

const props = defineProps<{
  event: BoardKeyEventItem
  left: number
  top: number
  /** 定位未完成（第一阶段测量中）时隐藏本体，避免宽度抖动。 */
  measuring?: boolean
}>()

const emit = defineEmits<{
  select: [date: string]
}>()
</script>

<template>
  <button
    type="button"
    class="kev-label"
    :class="{ 'is-measuring': measuring }"
    :style="{ left: `${props.left}px`, top: `${props.top}px` }"
    :data-testid="`key-event-label-${event.id}`"
    :title="`${event.sourcePeriodDate} · ${event.titleSnapshot}`"
    @click.stop="emit('select', event.sourcePeriodDate)"
  >
    <span class="kev-label-dot" aria-hidden="true" />
    <span class="kev-label-text">{{ event.titleSnapshot }}</span>
  </button>
</template>

<style scoped>
.kev-label {
  position: absolute;
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  max-width: none;
  height: 1.5rem;
  padding: 0 0.45rem 0 0.3rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 999px;
  background: var(--color-bg-elevated);
  color: var(--color-text-secondary);
  font-size: 0.74rem;
  line-height: 1;
  white-space: nowrap;
  cursor: pointer;
}

/* 第一阶段（测量）不可见但占自然宽度：visibility 保留布局盒子，offsetWidth 可测。 */
.kev-label.is-measuring {
  visibility: hidden;
}

.kev-label:hover,
.kev-label:focus-visible {
  border-color: var(--color-accent);
  color: var(--color-text-primary);
  outline: none;
  box-shadow: 0 0 0 1px var(--color-accent);
}

.kev-label-dot {
  flex: 0 0 auto;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--color-accent);
}

.kev-label-text {
  /* 自然宽度：不换行、不省略（spec「标签不截断」）。 */
  white-space: nowrap;
}
</style>
