<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useSemanticBoardsApi, type SuggestedAuxiliaryLabel } from '~/api/semanticBoards'

const props = defineProps<{
  /** 'create' = use suggest-auxiliaries with label; 'edit' = use suggest-auxiliaries-for-board with boardId */
  mode: 'create' | 'edit'
  boardId?: number
  initialLabel?: string
  initialDescription?: string
  /** IDs already selected (for pre-populating checkboxes) */
  selectedIds: number[]
}>()

const emit = defineEmits<{
  'update:selectedIds': [ids: number[]]
}>()

const api = useSemanticBoardsApi()

const label = ref(props.initialLabel ?? '')
const description = ref(props.initialDescription ?? '')
const search = ref('')
const page = ref(1)
const pageSize = 20
const loading = ref(false)
const items = ref<SuggestedAuxiliaryLabel[]>([])
const total = ref(0)

const internalSelected = ref(new Set(props.selectedIds))

watch(() => props.selectedIds, (ids) => {
  internalSelected.value = new Set(ids)
})

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

async function fetchSuggestions() {
  loading.value = true
  try {
    const baseParams = { search: search.value || undefined, page: page.value, page_size: pageSize }
    const res = props.mode === 'edit' && props.boardId
      ? await api.suggestAuxiliariesForBoard(props.boardId, baseParams)
      : await api.suggestAuxiliaries({
          label: label.value,
          description: description.value || undefined,
          exclude_board_id: props.mode === 'create' ? undefined : undefined,
          ...baseParams,
        })
    if (res.success && res.data) {
      items.value = res.data.items
      total.value = res.data.total
    }
  } finally {
    loading.value = false
  }
}

function toggleItem(id: number) {
  const next = new Set(internalSelected.value)
  if (next.has(id)) {
    next.delete(id)
  } else {
    next.add(id)
  }
  internalSelected.value = next
  emit('update:selectedIds', [...next])
}

function similarityPercent(sim: number): string {
  return `${Math.round(sim * 100)}%`
}

watch([label, search], () => { page.value = 1 })
</script>

<template>
  <div class="alp">
    <!-- Query inputs (create mode) -->
    <div v-if="mode === 'create'" class="alp-query">
      <input
        v-model="label"
        type="text"
        class="alp-input"
        placeholder="输入板块名称用于推荐..."
        maxlength="100"
      />
      <input
        v-model="description"
        type="text"
        class="alp-input"
        placeholder="可选描述"
        maxlength="500"
      />
    </div>

    <!-- Search + fetch -->
    <div class="alp-toolbar">
      <input
        v-model="search"
        type="text"
        class="alp-input alp-input--sm"
        placeholder="搜索辅助标签..."
        @keyup.enter="fetchSuggestions"
      />
      <button
        type="button"
        class="alp-fetch-btn"
        :disabled="(mode === 'create' && !label.trim()) || loading"
        @click="fetchSuggestions"
      >
        <Icon icon="mdi:magnify" width="14" />
        推荐
      </button>
    </div>

    <!-- Selected chips -->
    <div v-if="internalSelected.size > 0" class="alp-selected">
      <span class="alp-selected-label">已选 {{ internalSelected.size }} 个：</span>
      <div class="alp-chips">
        <span
          v-for="item in items.filter(i => internalSelected.has(i.id))"
          :key="item.id"
          class="alp-chip"
        >
          {{ item.label }}
          <button type="button" class="alp-chip-x" @click="toggleItem(item.id)">
            <Icon icon="mdi:close" width="10" />
          </button>
        </span>
      </div>
    </div>

    <!-- Results list -->
    <div v-if="loading" class="alp-loading">
      <div v-for="i in 3" :key="i" class="alp-skeleton" />
    </div>

    <div v-else-if="items.length === 0" class="alp-empty">
      <Icon icon="mdi:tag-search-outline" width="20" class="text-white/15" />
      <span>{{ label || boardId ? '点击推荐按钮获取建议' : '请先输入板块名称' }}</span>
    </div>

    <div v-else class="alp-list">
      <label
        v-for="item in items"
        :key="item.id"
        class="alp-item"
        :class="{ 'alp-item--selected': internalSelected.has(item.id) }"
      >
        <input
          type="checkbox"
          class="alp-checkbox"
          :checked="internalSelected.has(item.id)"
          @change="toggleItem(item.id)"
        />
        <span class="alp-item-label">{{ item.label }}</span>
        <span class="alp-item-sim">{{ similarityPercent(item.similarity) }}</span>
        <span v-if="item.ref_count > 0" class="alp-item-ref">{{ item.ref_count }}</span>
      </label>
    </div>

    <!-- Pagination -->
    <div v-if="total > pageSize" class="alp-pagination">
      <button
        type="button"
        class="alp-page-btn"
        :disabled="page <= 1"
        @click="page--; fetchSuggestions()"
      >
        <Icon icon="mdi:chevron-left" width="16" />
      </button>
      <span class="alp-page-info">{{ page }} / {{ totalPages }}</span>
      <button
        type="button"
        class="alp-page-btn"
        :disabled="page >= totalPages"
        @click="page++; fetchSuggestions()"
      >
        <Icon icon="mdi:chevron-right" width="16" />
      </button>
    </div>
  </div>
</template>

<style scoped>
.alp {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.alp-query {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.alp-toolbar {
  display: flex;
  gap: 0.4rem;
}

.alp-input {
  flex: 1;
  width: 100%;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 8px;
  background: rgba(0, 0, 0, 0.2);
  color: rgba(255, 255, 255, 0.88);
  font-size: 0.78rem;
  padding: 0.4rem 0.65rem;
  outline: none;
  transition: border-color 0.12s ease;
  box-sizing: border-box;
}

.alp-input::placeholder {
  color: rgba(255, 255, 255, 0.2);
}

.alp-input:focus {
  border-color: rgba(240, 138, 75, 0.4);
}

.alp-input--sm {
  font-size: 0.72rem;
  padding: 0.3rem 0.5rem;
}

.alp-fetch-btn {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.3rem 0.7rem;
  border-radius: 8px;
  border: 1px solid rgba(99, 179, 237, 0.25);
  background: rgba(99, 179, 237, 0.08);
  color: rgba(147, 197, 253, 0.8);
  font-size: 0.72rem;
  cursor: pointer;
  transition: all 0.12s ease;
  white-space: nowrap;
}

.alp-fetch-btn:hover:not(:disabled) {
  background: rgba(99, 179, 237, 0.15);
  border-color: rgba(99, 179, 237, 0.4);
}

.alp-fetch-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.alp-selected {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

.alp-selected-label {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.4);
}

.alp-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.3rem;
}

.alp-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.15rem 0.4rem;
  border-radius: 6px;
  border: 1px solid rgba(240, 138, 75, 0.25);
  background: rgba(240, 138, 75, 0.08);
  font-size: 0.68rem;
  color: rgba(255, 220, 200, 0.85);
}

.alp-chip-x {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 12px;
  height: 12px;
  border: none;
  border-radius: 3px;
  background: none;
  color: rgba(255, 255, 255, 0.3);
  cursor: pointer;
  transition: color 0.1s;
}

.alp-chip-x:hover {
  color: rgba(252, 165, 165, 0.9);
}

.alp-loading {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.alp-skeleton {
  height: 28px;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.03);
  animation: alpPulse 1.5s ease-in-out infinite;
}

@keyframes alpPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

.alp-empty {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 1rem 0;
  color: rgba(255, 255, 255, 0.25);
  font-size: 0.72rem;
}

.alp-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 260px;
  overflow-y: auto;
}

.alp-item {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.35rem 0.5rem;
  border-radius: 6px;
  cursor: pointer;
  transition: background 0.1s;
}

.alp-item:hover {
  background: rgba(255, 255, 255, 0.04);
}

.alp-item--selected {
  background: rgba(240, 138, 75, 0.08);
  border: 1px solid rgba(240, 138, 75, 0.15);
}

.alp-checkbox {
  width: 14px;
  height: 14px;
  accent-color: rgba(240, 138, 75, 0.8);
  flex-shrink: 0;
}

.alp-item-label {
  flex: 1;
  font-size: 0.75rem;
  color: rgba(255, 255, 255, 0.75);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.alp-item--selected .alp-item-label {
  color: rgba(255, 220, 200, 0.9);
}

.alp-item-sim {
  font-size: 0.62rem;
  color: rgba(147, 197, 253, 0.6);
  min-width: 32px;
  text-align: right;
}

.alp-item-ref {
  font-size: 0.58rem;
  color: rgba(255, 255, 255, 0.3);
  padding: 0 0.25rem;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.05);
}

.alp-pagination {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  padding-top: 0.4rem;
}

.alp-page-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 6px;
  background: none;
  color: rgba(255, 255, 255, 0.5);
  cursor: pointer;
  transition: all 0.1s;
}

.alp-page-btn:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.06);
  color: rgba(255, 255, 255, 0.8);
}

.alp-page-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.alp-page-info {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.4);
}
</style>
