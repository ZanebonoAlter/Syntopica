<script setup lang="ts">
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import type { BackfillTask } from '~/api/semanticBoards'

const props = defineProps<{
  task: BackfillTask | null
}>()

const percent = computed(() => {
  if (!props.task || props.task.total === 0) return 0
  return Math.round((props.task.processed / props.task.total) * 100)
})

const isRunning = computed(() => props.task?.status === 'pending' || props.task?.status === 'running')
</script>

<template>
  <div v-if="task" class="bf-progress">
    <div class="bf-bar-track">
      <div class="bf-bar-fill" :style="{ width: `${percent}%` }" />
    </div>
    <div class="bf-info">
      <Icon v-if="isRunning" icon="mdi:loading" width="13" class="animate-spin text-blue-400/60" />
      <Icon v-else-if="task.status === 'completed'" icon="mdi:check-circle-outline" width="13" class="text-green-400/70" />
      <Icon v-else icon="mdi:alert-circle-outline" width="13" class="text-red-400/70" />
      <span>回填: {{ task.processed }}/{{ task.total }}</span>
      <span v-if="task.failed > 0" class="bf-failed">失败 {{ task.failed }}</span>
    </div>
  </div>
</template>

<style scoped>
.bf-progress {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex: 1;
  max-width: 400px;
}

.bf-bar-track {
  flex: 1;
  height: 3px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
  overflow: hidden;
}

.bf-bar-fill {
  height: 100%;
  border-radius: 999px;
  background: linear-gradient(90deg, rgba(99, 179, 237, 0.7), rgba(147, 197, 253, 0.9));
  transition: width 0.3s ease;
}

.bf-info {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.68rem;
  color: rgba(255, 255, 255, 0.45);
  white-space: nowrap;
}

.bf-failed {
  color: rgba(252, 165, 165, 0.8);
}
</style>
