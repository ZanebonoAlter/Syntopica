<script setup lang="ts">
import { ref } from 'vue'
import { Icon } from '@iconify/vue'
import type { AuxiliaryLabel } from '~/api/auxiliaryLabels'

const props = defineProps<{
  labels: AuxiliaryLabel[]
  loading: boolean
  searchQuery: string
  statusFilter: string
}>()

const emit = defineEmits<{
  'update:searchQuery': [v: string]
  'update:statusFilter': [v: string]
  disable: [id: number, label: string]
  merge: [sourceId: number, targetId: number]
  refresh: []
}>()

const mergeSourceId = ref<number | null>(null)

function handleDisable(id: number, label: string) {
  if (!confirm(`禁用辅助标签 "${label}"？\n禁用后不再参与 board 匹配和升级候选。`)) return
  emit('disable', id, label)
}

function startMerge(sourceId: number) {
  mergeSourceId.value = sourceId
}

function cancelMerge() {
  mergeSourceId.value = null
}

function confirmMerge(targetId: number, targetLabel: string) {
  if (mergeSourceId.value === null) return
  if (!confirm(`将标签合并为 "${targetLabel}" 的 alias？`)) return
  emit('merge', mergeSourceId.value, targetId)
  mergeSourceId.value = null
}
</script>

<template>
  <div class="alp-panel">
    <div class="alp-header">
      <Icon icon="mdi:tag-multiple-outline" width="15" class="text-white/50" />
      <span class="alp-title">辅助标签池</span>
      <span class="alp-count">{{ labels.length }}</span>
    </div>

    <div class="alp-filters">
      <input
        :value="searchQuery"
        type="text"
        class="alp-search"
        placeholder="搜索标签..."
        @input="emit('update:searchQuery', ($event.target as HTMLInputElement).value)"
      />
      <select
        :value="statusFilter"
        class="alp-select"
        @change="emit('update:statusFilter', ($event.target as HTMLSelectElement).value)"
      >
        <option value="">全部状态</option>
        <option value="active">活跃</option>
        <option value="disabled">已禁用</option>
      </select>
    </div>

    <div v-if="loading" class="alp-loading">
      <div v-for="i in 4" :key="i" class="alp-skeleton-row" />
    </div>

    <div v-else-if="labels.length === 0" class="alp-empty">
      <Icon icon="mdi:tag-off-outline" width="24" class="text-white/15" />
      <span>暂无辅助标签</span>
    </div>

    <div v-else class="alp-list">
      <div
        v-for="label in labels"
        :key="label.id"
        class="alp-row"
        :class="{ 'alp-row--disabled': label.status === 'disabled', 'alp-row--merge-target': mergeSourceId !== null && mergeSourceId !== label.id }"
      >
        <div class="alp-row-main">
          <span class="alp-row-label">{{ label.label }}</span>
          <span v-if="label.aliases.length" class="alp-row-aliases">aka {{ label.aliases.join(', ') }}</span>
          <span class="alp-row-ref">{{ label.ref_count }} 引用</span>
        </div>
        <div class="alp-row-actions">
          <template v-if="mergeSourceId === null">
            <button
              v-if="label.status === 'active'"
              type="button"
              class="alp-row-btn"
              title="禁用"
              @click="handleDisable(label.id, label.label)"
            >
              <Icon icon="mdi:eye-off-outline" width="12" />
            </button>
            <button
              type="button"
              class="alp-row-btn"
              title="合并为其他标签的 alias"
              @click="startMerge(label.id)"
            >
              <Icon icon="mdi:merge" width="12" />
            </button>
          </template>
          <template v-else-if="mergeSourceId !== label.id">
            <button
              type="button"
              class="alp-row-btn alp-row-btn--primary"
              @click="confirmMerge(label.id, label.label)"
            >
              合并到此
            </button>
          </template>
          <span v-else class="alp-row-self">源标签</span>
        </div>
      </div>
      <div v-if="mergeSourceId !== null" class="alp-merge-hint">
        <span>选择目标标签进行合并</span>
        <button type="button" class="alp-row-btn" @click="cancelMerge">取消</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.alp-panel {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.alp-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.alp-title {
  font-size: 0.78rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.7);
}

.alp-count {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
  padding: 0.05rem 0.4rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
}

.alp-filters {
  display: flex;
  gap: 0.5rem;
}

.alp-search {
  flex: 1;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 10px;
  background: rgba(0, 0, 0, 0.2);
  color: rgba(255, 255, 255, 0.8);
  font-size: 0.75rem;
  padding: 0.4rem 0.7rem;
  outline: none;
}

.alp-search::placeholder {
  color: rgba(255, 255, 255, 0.25);
}

.alp-search:focus {
  border-color: rgba(240, 138, 75, 0.4);
}

.alp-select {
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 10px;
  background: rgba(0, 0, 0, 0.2);
  color: rgba(255, 255, 255, 0.7);
  font-size: 0.75rem;
  padding: 0.4rem 0.6rem;
  outline: none;
}

.alp-loading {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.alp-skeleton-row {
  height: 36px;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.03);
  animation: alpPulse 1.5s ease-in-out infinite;
}

@keyframes alpPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

.alp-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.4rem;
  padding: 2rem 0;
  color: rgba(255, 255, 255, 0.25);
  font-size: 0.75rem;
}

.alp-list {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.alp-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.5rem 0.6rem;
  border-radius: 10px;
  transition: all 0.12s ease;
}

.alp-row:hover {
  background: rgba(255, 255, 255, 0.04);
}

.alp-row--disabled {
  opacity: 0.5;
}

.alp-row--merge-target {
  cursor: pointer;
}

.alp-row-main {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  min-width: 0;
  flex-wrap: wrap;
}

.alp-row-label {
  font-size: 0.78rem;
  color: rgba(255, 255, 255, 0.8);
}

.alp-row-aliases {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.35);
}

.alp-row-ref {
  font-size: 0.6rem;
  color: rgba(255, 255, 255, 0.3);
  padding: 0.05rem 0.35rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
}

.alp-row-actions {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  flex-shrink: 0;
}

.alp-row-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.2rem;
  padding: 0.25rem 0.4rem;
  border-radius: 6px;
  border: none;
  background: none;
  color: rgba(255, 255, 255, 0.35);
  font-size: 0.65rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.alp-row-btn:hover {
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.7);
}

.alp-row-btn--primary {
  color: rgba(240, 138, 75, 0.8);
}

.alp-row-btn--primary:hover {
  background: rgba(240, 138, 75, 0.12);
}

.alp-row-self {
  font-size: 0.6rem;
  color: rgba(255, 255, 255, 0.25);
  padding: 0.2rem 0.4rem;
}

.alp-merge-hint {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.5rem 0.6rem;
  border-radius: 10px;
  background: rgba(240, 138, 75, 0.08);
  border: 1px solid rgba(240, 138, 75, 0.15);
  font-size: 0.72rem;
  color: rgba(255, 220, 200, 0.8);
}
</style>
