<script setup lang="ts">
/** 关联日提示浮到 body，避免被矩阵/窄屏日期条的横滚容器裁掉。 */
import { ref } from 'vue'
import { useFloating } from '@floating-ui/vue'
import { offset, shift, autoUpdate } from '@floating-ui/dom'
import type { ObservedDayNode } from '~/features/tags/composables/useOtherTopicObservation'

defineProps<{ node: ObservedDayNode, visible: boolean }>()

const marker = ref<HTMLElement | null>(null)
const tooltip = ref<HTMLElement | null>(null)
const { floatingStyles } = useFloating(marker, tooltip, {
  strategy: 'fixed',
  placement: 'bottom',
  middleware: [offset(8), shift({ padding: 8 })],
  whileElementsMounted: autoUpdate,
})
</script>

<template>
  <span ref="marker" class="date-association-dot" data-testid="date-association-marker" aria-hidden="true" />
  <Teleport to="body">
    <span v-if="visible" ref="tooltip" class="date-association-title" :style="floatingStyles" role="tooltip">
      <span>{{ node.date }} · 同归属记录</span>
      <span v-for="(title, index) in node.sectionTitles" :key="index">{{ title }}</span>
    </span>
  </Teleport>
</template>

<style scoped>
.date-association-dot {
  position: absolute;
  top: 0.1rem;
  right: 0.1rem;
  display: inline-block;
  width: 0.35rem;
  height: 0.35rem;
  border-radius: 50%;
  background: var(--color-warning);
  vertical-align: middle;
}

.date-association-title {
  position: fixed;
  z-index: 1000;
  width: max-content;
  max-width: min(20rem, 70vw);
  padding: 0.4rem 0.55rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  background: var(--color-bg-elevated);
  color: var(--color-text-primary);
  font-size: 0.72rem;
  font-weight: 400;
  line-height: 1.5;
  white-space: normal;
  text-align: left;
  pointer-events: none;
}

.date-association-title span {
  display: block;
}
</style>
