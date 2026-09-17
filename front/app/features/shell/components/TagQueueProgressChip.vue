<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { useTagQueueProgress } from '~/composables/useTagQueueProgress'

/**
 * 标签队列进度芯片（tag-queue-progress-chip）
 * 可见性唯一判据（白盒 E）：pending+leased>0 || failed>0；空闲 = 组件不渲染（v-if 移除 DOM）
 * 进行中：「分析中 n/total」+ 迷你进度条 + 转圈图标；失败态：红边框红底「n 个失败」
 * 点击跳设置页队列区（复用既有 TagQueuePanel）
 */

const { status, activeCount, visible, isFailedState, roundTotal, progressPercent, ensureStarted, stop } = useTagQueueProgress()

onMounted(() => {
  ensureStarted()
})

onUnmounted(() => {
  stop()
})

function goToQueues() {
  return navigateTo('/settings?section=queues&queue=tag')
}
</script>

<template>
  <button
    v-if="visible"
    class="queue-chip"
    :class="{ 'queue-chip--failed': isFailedState }"
    data-testid="tag-queue-progress-chip"
    title="查看标签队列详情"
    @click="goToQueues"
  >
    <span class="queue-chip__icon" :class="{ 'is-spinning': !isFailedState }">
      <Icon :icon="isFailedState ? 'mdi:alert-circle-outline' : 'mdi:progress-clock'" width="16" height="16" />
    </span>
    <span class="queue-chip__text" data-testid="chip-text">
      <template v-if="isFailedState && activeCount === 0">{{ status.failed }} 个失败</template>
      <template v-else-if="isFailedState">{{ status.failed }} 个失败 · 分析中 {{ status.completedToday }}/{{ roundTotal }}</template>
      <template v-else>分析中 {{ status.completedToday }}/{{ roundTotal }}</template>
    </span>
    <span class="queue-chip__bar" aria-hidden="true">
      <i :style="{ width: `${progressPercent}%` }" />
    </span>
  </button>
</template>

<style scoped>
.queue-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  height: 2.25rem;
  padding: 0 0.75rem;
  margin-right: 0.25rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 1.125rem;
  background: var(--color-bg-hover);
  cursor: pointer;
  font-size: 0.75rem;
  color: var(--color-text-secondary);
  transition: border-color 0.15s ease;
  white-space: nowrap;
}

.queue-chip:hover {
  border-color: var(--color-border-strong);
}

.queue-chip__icon {
  display: inline-flex;
  color: var(--color-info);
}

.queue-chip__icon.is-spinning {
  animation: chip-spin 1.2s linear infinite;
}

@keyframes chip-spin {
  to { transform: rotate(360deg); }
}

.queue-chip--failed {
  border-color: var(--color-error);
  background: var(--color-error-subtle, rgba(196, 47, 60, 0.12));
  color: var(--color-error);
}

.queue-chip--failed .queue-chip__icon {
  color: var(--color-error);
}

.queue-chip--failed .queue-chip__bar > i {
  background: var(--color-error);
}

.queue-chip__text {
  font-variant-numeric: tabular-nums;
}

.queue-chip__bar {
  width: 3.5rem;
  height: 5px;
  background: var(--color-border-subtle);
  border-radius: 3px;
  overflow: hidden;
}

.queue-chip__bar > i {
  display: block;
  height: 100%;
  background: var(--color-info);
  border-radius: 3px;
  transition: width 0.3s ease;
}
</style>
