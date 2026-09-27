<script setup lang="ts">
import type { MarginNoteBubbleState } from '~/features/tags/composables/useMarginNoteSelection'

/**
 * 划词「问一问」气泡（daily-report-margin-notes design D5 / ui-design 主操作）：
 * 绝对定位于正文根容器（.drm-peel-page）坐标系，浮于选区上方；点击确认落锚。
 * mousedown preventDefault 防止点击气泡时清掉选区（原型同款）。
 */
defineProps<{
  state: MarginNoteBubbleState
}>()

const emit = defineEmits<{
  confirm: []
}>()
</script>

<template>
  <button
    v-if="state.visible"
    type="button"
    class="mn-ask-bubble"
    :style="{ left: `${state.x}px`, top: `${state.y}px` }"
    @mousedown.prevent
    @click="emit('confirm')"
  >
    问一问 ✎
  </button>
</template>

<style scoped>
.mn-ask-bubble {
  position: absolute;
  z-index: 50;
  display: inline-flex;
  align-items: center;
  transform: translate(-50%, calc(-100% - 10px));
  padding: 0.34rem 0.85rem;
  border: 0;
  border-radius: 999px;
  background: var(--color-text-primary);
  color: var(--color-bg-base);
  font-family: system-ui, -apple-system, "PingFang SC", "Microsoft YaHei", sans-serif;
  font-size: 0.78rem;
  letter-spacing: 0.04em;
  white-space: nowrap;
  cursor: pointer;
  box-shadow: var(--shadow-strong);
  user-select: none;
  animation: mnBubbleIn 0.18s ease;
}

.mn-ask-bubble::after {
  content: "";
  position: absolute;
  bottom: -3px;
  left: 50%;
  width: 8px;
  height: 8px;
  background: var(--color-text-primary);
  transform: translateX(-50%) rotate(45deg);
}

.mn-ask-bubble:hover {
  background: var(--color-accent);
  color: #fff;
}

@keyframes mnBubbleIn {
  from { opacity: 0; transform: translate(-50%, calc(-100% - 6px)); }
  to { opacity: 1; transform: translate(-50%, calc(-100% - 10px)); }
}
</style>
