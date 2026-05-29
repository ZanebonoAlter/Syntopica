<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Icon } from '@iconify/vue'
import type { AuxiliaryLabel, AuxiliaryLabelCluster } from '~/api/auxiliaryLabels'

const props = defineProps<{
  labels: AuxiliaryLabel[]
  clusters: AuxiliaryLabelCluster[]
  unclusteredCount: number
  loading: boolean
  searchQuery: string
  statusFilter: string
  pagination: { page: number; pages: number; total: number } | null
}>()

const emit = defineEmits<{
  'update:searchQuery': [v: string]
  'update:statusFilter': [v: string]
  'update:page': [v: number]
  disable: [id: number, label: string]
  merge: [sourceId: number, targetId: number]
  refresh: []
  selectCluster: [index: number]
}>()

const mergeSourceId = ref<number | null>(null)
const activeClusterIndex = ref(-1)
const clusterPage = ref(1)
const clusterPerPage = 15

const displayedLabels = computed(() => {
  if (activeClusterIndex.value < 0 || activeClusterIndex.value >= props.clusters.length) {
    return props.labels
  }
  const cluster = props.clusters[activeClusterIndex.value]
  if (!cluster) return []
  return cluster.labels
})

const totalClusterPages = computed(() => {
  return Math.max(1, Math.ceil(displayedLabels.value.length / clusterPerPage))
})

const paginatedClusterLabels = computed(() => {
  if (activeClusterIndex.value < 0) return props.labels
  const start = (clusterPage.value - 1) * clusterPerPage
  return displayedLabels.value.slice(start, start + clusterPerPage)
})

function selectCluster(index: number) {
  activeClusterIndex.value = index
  clusterPage.value = 1
  mergeSourceId.value = null
  emit('selectCluster', index)
}

function goToClusterPage(p: number) {
  if (p >= 1 && p <= totalClusterPages.value) {
    clusterPage.value = p
  }
}

function goToPage(p: number) {
  if (p >= 1 && props.pagination && p <= props.pagination.pages) {
    emit('update:page', p)
  }
}

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

watch(() => props.searchQuery, () => {
  activeClusterIndex.value = -1
})

function paginationRange(current: number, total: number): (number | 'ellipsis')[] {
  if (total <= 7) {
    return Array.from({ length: total }, (_, i) => i + 1)
  }
  const range: (number | 'ellipsis')[] = []
  if (current <= 4) {
    for (let i = 1; i <= 5; i++) range.push(i)
    range.push('ellipsis')
    range.push(total)
  } else if (current >= total - 3) {
    range.push(1)
    range.push('ellipsis')
    for (let i = total - 4; i <= total; i++) range.push(i)
  } else {
    range.push(1)
    range.push('ellipsis')
    range.push(current - 1)
    range.push(current)
    range.push(current + 1)
    range.push('ellipsis')
    range.push(total)
  }
  return range
}
</script>

<template>
  <div class="alp-panel">
    <div class="alp-header">
      <Icon icon="mdi:tag-multiple-outline" width="15" class="text-white/50" />
      <span class="alp-title">辅助标签池</span>
      <span class="alp-count">{{ pagination?.total ?? labels.length }}</span>
    </div>

    <!-- Cluster tabs -->
    <div v-if="clusters.length > 0" class="alp-clusters">
      <button
        type="button"
        class="alp-cluster-chip"
        :class="{ 'alp-cluster-chip--active': activeClusterIndex === -1 }"
        @click="selectCluster(-1)"
      >
        全部
        <span class="alp-cluster-count">{{ pagination?.total ?? '' }}</span>
      </button>
      <button
        v-for="(cluster, ci) in clusters"
        :key="ci"
        type="button"
        class="alp-cluster-chip"
        :class="{ 'alp-cluster-chip--active': activeClusterIndex === ci }"
        @click="selectCluster(ci)"
      >
        {{ cluster.label }}
        <span class="alp-cluster-count">{{ cluster.size }}</span>
      </button>
    </div>

    <!-- Filters -->
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

    <!-- Loading -->
    <div v-if="loading" class="alp-loading">
      <div v-for="i in 4" :key="i" class="alp-skeleton-row" />
    </div>

    <!-- Empty -->
    <div v-else-if="displayedLabels.length === 0" class="alp-empty">
      <Icon icon="mdi:tag-off-outline" width="24" class="text-white/15" />
      <span>暂无辅助标签</span>
    </div>

    <!-- Label list (cluster view) -->
    <div v-else-if="activeClusterIndex >= 0" class="alp-list">
      <div
        v-for="label in paginatedClusterLabels"
        :key="label.id"
        class="alp-row"
        :class="{ 'alp-row--disabled': 'status' in label && (label as any).status === 'disabled', 'alp-row--merge-target': mergeSourceId !== null && mergeSourceId !== label.id }"
      >
        <div class="alp-row-main">
          <span class="alp-row-label">{{ label.label }}</span>
          <span class="alp-row-ref">{{ label.ref_count }} 引用</span>
        </div>
        <div class="alp-row-actions">
          <template v-if="mergeSourceId === null">
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

      <!-- Cluster pagination -->
      <div v-if="totalClusterPages > 1" class="alp-page-nav alp-page-nav--bottom">
        <button type="button" class="alp-page-btn" :disabled="clusterPage <= 1" @click="goToClusterPage(clusterPage - 1)">
          <Icon icon="mdi:chevron-left" width="14" />
        </button>
        <template v-for="p in paginationRange(clusterPage, totalClusterPages)" :key="p">
          <span v-if="p === 'ellipsis'" class="alp-page-ellipsis">…</span>
          <button
            v-else
            type="button"
            class="alp-page-btn"
            :class="{ 'alp-page-btn--active': p === clusterPage }"
            @click="goToClusterPage(p as number)"
          >
            {{ p }}
          </button>
        </template>
        <button type="button" class="alp-page-btn" :disabled="clusterPage >= totalClusterPages" @click="goToClusterPage(clusterPage + 1)">
          <Icon icon="mdi:chevron-right" width="14" />
        </button>
      </div>

      <div v-if="mergeSourceId !== null" class="alp-merge-hint">
        <span>选择目标标签进行合并</span>
        <button type="button" class="alp-row-btn" @click="cancelMerge">取消</button>
      </div>
    </div>

    <!-- Label list (All view) -->
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

      <!-- Server pagination bottom -->
      <div v-if="pagination && pagination.pages > 1" class="alp-page-nav alp-page-nav--bottom">
        <button type="button" class="alp-page-btn" :disabled="pagination.page <= 1" @click="goToPage(pagination.page - 1)">
          <Icon icon="mdi:chevron-left" width="14" />
        </button>
        <template v-for="p in paginationRange(pagination.page, pagination.pages)" :key="p">
          <span v-if="p === 'ellipsis'" class="alp-page-ellipsis">…</span>
          <button
            v-else
            type="button"
            class="alp-page-btn"
            :class="{ 'alp-page-btn--active': p === pagination.page }"
            @click="goToPage(p as number)"
          >
            {{ p }}
          </button>
        </template>
        <button type="button" class="alp-page-btn" :disabled="pagination.page >= pagination.pages" @click="goToPage(pagination.page + 1)">
          <Icon icon="mdi:chevron-right" width="14" />
        </button>
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

.alp-clusters {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  padding-bottom: 0.5rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.alp-cluster-chip {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.25rem 0.55rem;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: none;
  color: rgba(255, 255, 255, 0.5);
  font-size: 0.7rem;
  cursor: pointer;
  transition: all 0.12s ease;
  white-space: nowrap;
}

.alp-cluster-chip:hover {
  border-color: rgba(255, 255, 255, 0.18);
  color: rgba(255, 255, 255, 0.7);
  background: rgba(255, 255, 255, 0.03);
}

.alp-cluster-chip--active {
  border-color: rgba(240, 138, 75, 0.45);
  color: rgba(255, 220, 200, 0.85);
  background: rgba(240, 138, 75, 0.1);
}

.alp-cluster-count {
  font-size: 0.6rem;
  opacity: 0.6;
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

.alp-page-nav {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.25rem;
  padding: 0.25rem 0;
}

.alp-page-nav--bottom {
  padding-top: 0.5rem;
  border-top: 1px solid rgba(255, 255, 255, 0.05);
  margin-top: 0.25rem;
}

.alp-page-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 26px;
  height: 26px;
  padding: 0 0.3rem;
  border-radius: 6px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: none;
  color: rgba(255, 255, 255, 0.45);
  font-size: 0.7rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.alp-page-btn:hover:not(:disabled) {
  border-color: rgba(255, 255, 255, 0.18);
  color: rgba(255, 255, 255, 0.7);
  background: rgba(255, 255, 255, 0.03);
}

.alp-page-btn--active {
  border-color: rgba(240, 138, 75, 0.45);
  color: rgba(255, 220, 200, 0.85);
  background: rgba(240, 138, 75, 0.1);
}

.alp-page-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.alp-page-ellipsis {
  color: rgba(255, 255, 255, 0.3);
  font-size: 0.7rem;
  padding: 0 0.15rem;
}
</style>
