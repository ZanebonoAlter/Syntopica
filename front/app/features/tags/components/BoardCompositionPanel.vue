<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { AuxiliaryLabelItem } from '~/api/semanticBoards'

const props = defineProps<{
  boardId: number
  labels: AuxiliaryLabelItem[]
  loading: boolean
}>()

const emit = defineEmits<{
  remove: [auxiliaryLabelId: number]
  refresh: []
}>()

function handleRemove(id: number, label: string) {
  if (!confirm(`从板块中移除辅助标签 "${label}"？\n注意：不会自动回填历史数据。`)) return
  emit('remove', id)
}
</script>

<template>
  <div class="bcp-panel">
    <div class="bcp-header">
      <Icon icon="mdi:puzzle-outline" width="15" class="text-white/50" />
      <span class="bcp-title">构成标签</span>
      <span class="bcp-count">{{ labels.length }}</span>
    </div>

    <div v-if="loading" class="bcp-loading">
      <div v-for="i in 3" :key="i" class="bcp-skeleton-chip" />
    </div>

    <div v-else-if="labels.length === 0" class="bcp-empty">
      <Icon icon="mdi:tag-off-outline" width="20" class="text-white/15" />
      <span>暂无构成标签</span>
    </div>

    <div v-else class="bcp-chips">
      <div
        v-for="label in labels"
        :key="label.id"
        class="bcp-chip"
        :class="{ 'bcp-chip--disabled': label.status === 'disabled' }"
      >
        <span class="bcp-chip-label">{{ label.label }}</span>
        <span v-if="label.ref_count > 0" class="bcp-chip-ref">{{ label.ref_count }}</span>
        <button
          type="button"
          class="bcp-chip-remove"
          title="移除"
          @click="handleRemove(label.id, label.label)"
        >
          <Icon icon="mdi:close" width="10" />
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.bcp-panel {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  padding: 1rem;
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(255, 255, 255, 0.025);
}

.bcp-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.bcp-title {
  font-size: 0.78rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.7);
}

.bcp-count {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
  padding: 0.05rem 0.4rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
}

.bcp-loading {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
}

.bcp-skeleton-chip {
  width: 60px;
  height: 26px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.03);
  animation: bcpPulse 1.5s ease-in-out infinite;
}

@keyframes bcpPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

.bcp-empty {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 1rem 0;
  color: rgba(255, 255, 255, 0.25);
  font-size: 0.75rem;
}

.bcp-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
}

.bcp-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.25rem 0.5rem;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(255, 255, 255, 0.05);
  font-size: 0.72rem;
  color: rgba(255, 255, 255, 0.75);
  transition: all 0.12s ease;
}

.bcp-chip:hover {
  background: rgba(255, 255, 255, 0.08);
}

.bcp-chip--disabled {
  opacity: 0.5;
  border-style: dashed;
}

.bcp-chip-ref {
  font-size: 0.6rem;
  color: rgba(255, 255, 255, 0.35);
  padding: 0 0.25rem;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.06);
}

.bcp-chip-remove {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  border: none;
  border-radius: 4px;
  background: none;
  color: rgba(255, 255, 255, 0.2);
  cursor: pointer;
  opacity: 0;
  transition: all 0.12s ease;
}

.bcp-chip:hover .bcp-chip-remove {
  opacity: 1;
}

.bcp-chip-remove:hover {
  color: rgba(252, 165, 165, 0.9);
  background: rgba(239, 68, 68, 0.12);
}
</style>
