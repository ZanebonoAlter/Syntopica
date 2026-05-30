<script setup lang="ts">
import { ref } from 'vue'
import { Icon } from '@iconify/vue'
import { useSemanticBoardsApi, type AuxiliaryLabelItem } from '~/api/semanticBoards'
import AuxiliaryLabelPicker from './AuxiliaryLabelPicker.vue'

const props = defineProps<{
  boardId: number
  labels: AuxiliaryLabelItem[]
  loading: boolean
}>()

const emit = defineEmits<{
  remove: [auxiliaryLabelId: number]
  refresh: []
}>()

const api = useSemanticBoardsApi()
const showPicker = ref(false)
const pendingIds = ref<number[]>([])
const adding = ref(false)
const notice = ref('')

function handleRemove(id: number, label: string) {
  if (!confirm(`从板块中移除辅助标签 "${label}"？\n注意：不会自动回填历史数据。`)) return
  emit('remove', id)
}

async function handleConfirmAdd() {
  if (pendingIds.value.length === 0) {
    showPicker.value = false
    return
  }
  adding.value = true
  try {
    for (const id of pendingIds.value) {
      const res = await api.addComposition(props.boardId, id)
      if (!res.success) {
        console.error('Failed to add composition:', res.error)
      }
    }
    showPicker.value = false
    pendingIds.value = []
    notice.value = '已添加构成标签。历史标签归属不会自动回填，可手动触发 board 回填。'
    emit('refresh')
  } finally {
    adding.value = false
  }
}
</script>

<template>
  <div class="bcp-panel">
    <div class="bcp-header">
      <Icon icon="mdi:puzzle-outline" width="15" class="text-white/50" />
      <span class="bcp-title">构成标签</span>
      <span class="bcp-count">{{ labels.length }}</span>
      <div class="bcp-spacer" />
      <button type="button" class="bcp-add-btn" @click="showPicker = !showPicker">
        <Icon :icon="showPicker ? 'mdi:close' : 'mdi:plus'" width="14" />
        {{ showPicker ? '取消' : '添加' }}
      </button>
    </div>

    <div v-if="notice" class="bcp-notice">
      <Icon icon="mdi:information-outline" width="14" />
      <span>{{ notice }}</span>
    </div>

    <!-- Add auxiliary picker -->
    <div v-if="showPicker" class="bcp-picker">
      <AuxiliaryLabelPicker
        mode="edit"
        :board-id="boardId"
        :selected-ids="pendingIds"
        @update:selected-ids="pendingIds = $event"
      />
      <div v-if="pendingIds.length > 0" class="bcp-confirm">
        <button type="button" class="bcp-confirm-btn" :disabled="adding" @click="handleConfirmAdd">
          <Icon icon="mdi:check" width="14" />
          {{ adding ? '添加中...' : `确认添加 ${pendingIds.length} 个标签` }}
        </button>
      </div>
    </div>

    <div v-if="loading" class="bcp-loading">
      <div v-for="i in 3" :key="i" class="bcp-skeleton-chip" />
    </div>

    <div v-else-if="labels.length === 0 && !showPicker" class="bcp-empty">
      <Icon icon="mdi:tag-off-outline" width="20" class="text-white/15" />
      <span>暂无构成标签</span>
      <button type="button" class="bcp-empty-add" @click="showPicker = true">点击添加</button>
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

.bcp-spacer {
  flex: 1;
}

.bcp-add-btn {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.2rem 0.5rem;
  border-radius: 6px;
  border: 1px solid rgba(99, 179, 237, 0.2);
  background: rgba(99, 179, 237, 0.06);
  color: rgba(147, 197, 253, 0.7);
  font-size: 0.68rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.bcp-add-btn:hover {
  background: rgba(99, 179, 237, 0.12);
  border-color: rgba(99, 179, 237, 0.35);
  color: rgba(147, 197, 253, 0.9);
}

.bcp-notice {
  display: flex;
  align-items: flex-start;
  gap: 0.35rem;
  padding: 0.45rem 0.6rem;
  border-radius: 8px;
  border: 1px solid rgba(99, 179, 237, 0.16);
  background: rgba(99, 179, 237, 0.06);
  color: rgba(191, 219, 254, 0.78);
  font-size: 0.68rem;
  line-height: 1.5;
}

.bcp-picker {
  padding: 0.75rem;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.05);
  background: rgba(0, 0, 0, 0.15);
}

.bcp-confirm {
  display: flex;
  justify-content: flex-end;
  padding-top: 0.4rem;
}

.bcp-confirm-btn {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.35rem 0.8rem;
  border-radius: 8px;
  border: 1px solid rgba(240, 138, 75, 0.35);
  background: rgba(240, 138, 75, 0.1);
  color: rgba(255, 220, 200, 0.85);
  font-size: 0.72rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.bcp-confirm-btn:hover:not(:disabled) {
  background: rgba(240, 138, 75, 0.18);
  border-color: rgba(240, 138, 75, 0.5);
}

.bcp-confirm-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
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

.bcp-empty-add {
  font-size: 0.68rem;
  padding: 0.15rem 0.4rem;
  border-radius: 6px;
  border: 1px solid rgba(99, 179, 237, 0.2);
  background: none;
  color: rgba(147, 197, 253, 0.6);
  cursor: pointer;
  transition: all 0.1s;
}

.bcp-empty-add:hover {
  background: rgba(99, 179, 237, 0.08);
  color: rgba(147, 197, 253, 0.9);
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
