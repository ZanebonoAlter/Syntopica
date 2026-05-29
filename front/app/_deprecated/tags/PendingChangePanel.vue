<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { Icon } from '@iconify/vue'
import { useHierarchyConfigApi } from '~/api/hierarchyConfig'
import type { HierarchyPendingChange, PendingChangeApprovalResult } from '~/api/hierarchyConfig'

const props = defineProps<{
  category: string
}>()

const emit = defineEmits<{
  close: []
  countUpdate: [count: number]
}>()

const api = useHierarchyConfigApi()

const pendingList = ref<HierarchyPendingChange[]>([])
const loading = ref(false)
const error = ref('')
const approvingIds = ref<Set<number>>(new Set())
const approvingAll = ref(false)
const toastMessage = ref('')
const toastType = ref<'success' | 'error'>('success')
const approvalResults = ref<PendingChangeApprovalResult[]>([])
let toastTimer: ReturnType<typeof setTimeout> | null = null

const groupedByCategory = computed(() => {
  const groups: Record<string, HierarchyPendingChange[]> = {}
  for (const item of pendingList.value) {
    const cat = item.tag?.category ?? item.change_type
    if (!groups[cat]) groups[cat] = []
    groups[cat].push(item)
  }
  return groups
})

async function loadPending() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.getPending('pending')
    if (res.success && res.data) {
      pendingList.value = res.data
      emit('countUpdate', res.data.length)
    } else {
      error.value = res.error || '加载失败'
    }
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : '网络错误'
  } finally {
    loading.value = false
  }
}

async function approveOne(id: number) {
  approvingIds.value = new Set([...approvingIds.value, id])
  try {
    const res = await api.approvePendingChanges({ ids: [id] })
    if (res.success && res.data) {
      approvalResults.value = res.data.results || []
      if (res.data.failed > 0) {
        showToast(`确认失败: ${res.data.failed} 条`, 'error')
      } else {
        showToast(`已确认 ${res.data.approved} 条变更`, 'success')
      }
      await loadPending()
    } else {
      showToast(res.error || '确认失败', 'error')
    }
  } catch (e: unknown) {
    showToast(e instanceof Error ? e.message : '网络错误', 'error')
  } finally {
    const next = new Set(approvingIds.value)
    next.delete(id)
    approvingIds.value = next
  }
}

async function approveAll() {
  approvingAll.value = true
  try {
    const res = await api.approvePendingChanges({ approve_all: true, category: props.category })
    if (res.success && res.data) {
      const { approved, failed } = res.data
      approvalResults.value = res.data.results || []
      if (failed > 0) {
        showToast(`已确认 ${approved} 条，失败 ${failed} 条`, 'error')
      } else {
        showToast(`已确认 ${approved} 条变更`, 'success')
      }
      await loadPending()
    } else {
      showToast(res.error || '批量确认失败', 'error')
    }
  } catch (e: unknown) {
    showToast(e instanceof Error ? e.message : '网络错误', 'error')
  } finally {
    approvingAll.value = false
  }
}

function resultChangeLabel(result: PendingChangeApprovalResult): string {
  const badge = changeTypeBadge(result.change_type)
  return `${badge.label} #${result.tag_id}`
}

function showToast(msg: string, type: 'success' | 'error') {
  toastMessage.value = msg
  toastType.value = type
  if (toastTimer) clearTimeout(toastTimer)
  toastTimer = setTimeout(() => {
    toastMessage.value = ''
  }, 5000)
}

function changeTypeBadge(type: string): { label: string; cls: string } {
  switch (type) {
    case 'move': return { label: '移动', cls: 'badge--move' }
    case 'reparent': return { label: '重归属', cls: 'badge--reparent' }
    case 'create': return { label: '新建', cls: 'badge--create' }
    case 'delete': return { label: '删除', cls: 'badge--delete' }
    default: return { label: type, cls: 'badge--default' }
  }
}

onMounted(loadPending)
onUnmounted(() => {
  if (toastTimer) clearTimeout(toastTimer)
})
</script>

<template>
  <div class="pending-panel">
    <div class="pending-panel-header">
      <div class="pending-panel-title-row">
        <h3 class="pending-panel-title">
          <Icon icon="mdi:swap-horizontal" width="15" />
          待确认变更
        </h3>
        <span class="pending-panel-count">{{ pendingList.length }}</span>
      </div>
      <div class="pending-panel-actions">
        <button
          type="button"
          class="pending-action-btn pending-action-btn--primary"
          :disabled="approvingAll || pendingList.length === 0"
          @click="approveAll"
        >
          <Icon v-if="approvingAll" icon="mdi:loading" width="14" class="animate-spin" />
          全部确认
        </button>
        <button type="button" class="pending-close-btn" @click="emit('close')">
          <Icon icon="mdi:close" width="16" />
        </button>
      </div>
    </div>

    <div v-if="toastMessage" class="pending-toast" :class="`pending-toast--${toastType}`">
      <Icon :icon="toastType === 'success' ? 'mdi:check-circle-outline' : 'mdi:alert-circle-outline'" width="14" />
      <span>{{ toastMessage }}</span>
    </div>

    <div v-if="approvalResults.length > 0" class="pending-results">
      <div
        v-for="result in approvalResults"
        :key="result.id"
        class="pending-result-row"
        :class="{ 'pending-result-row--failed': result.status === 'failed' }"
      >
        <span>{{ resultChangeLabel(result) }}</span>
        <span>{{ result.status === 'failed' ? result.reason : '已确认' }}</span>
      </div>
    </div>

    <div v-if="loading" class="pending-loading">
      <Icon icon="mdi:loading" width="18" class="animate-spin" />
      <span>加载中...</span>
    </div>

    <div v-else-if="error" class="pending-error">
      <Icon icon="mdi:alert-circle-outline" width="14" />
      <span>{{ error }}</span>
    </div>

    <div v-else-if="pendingList.length === 0" class="pending-empty">
      <Icon icon="mdi:check-all" width="20" class="text-green-400/40" />
      <span>暂无待确认变更</span>
    </div>

    <div v-else class="pending-list">
      <div v-for="(items, cat) in groupedByCategory" :key="cat" class="pending-group">
        <div class="pending-group-header">{{ cat }}</div>
        <div
          v-for="item in items"
          :key="item.id"
          class="pending-item"
        >
          <div class="pending-item-main">
            <span class="pending-item-label">{{ item.tag_label }}</span>
            <span class="pending-item-badge" :class="changeTypeBadge(item.change_type).cls">
              {{ changeTypeBadge(item.change_type).label }}
            </span>
          </div>
          <div v-if="item.current_parent_label" class="pending-item-parent">
            当前父节点: {{ item.current_parent_label }}
          </div>
          <div class="pending-item-reason">{{ item.reason }}</div>
          <div class="pending-item-footer">
            <span class="pending-item-time">{{ item.created_at }}</span>
            <button
              type="button"
              class="pending-approve-btn"
              :disabled="approvingIds.has(item.id)"
              @click="approveOne(item.id)"
            >
              <Icon v-if="approvingIds.has(item.id)" icon="mdi:loading" width="12" class="animate-spin" />
              确认
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.pending-panel {
  position: fixed;
  bottom: 36px;
  left: 0;
  right: 0;
  z-index: 50;
  max-height: 50vh;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 16px 16px 0 0;
  background: rgba(12, 18, 28, 0.97);
  backdrop-filter: blur(16px);
  display: flex;
  flex-direction: column;
  box-shadow: 0 -8px 40px rgba(0, 0, 0, 0.4);
}

.pending-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.85rem 1.25rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  flex-shrink: 0;
}

.pending-panel-title-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.pending-panel-title {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.85rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.85);
}

.pending-panel-count {
  font-size: 0.6rem;
  color: rgba(255, 255, 255, 0.35);
  padding: 0.1rem 0.45rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
}

.pending-panel-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.pending-action-btn {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 8px;
  background: none;
  font-size: 0.72rem;
  padding: 0.35rem 0.85rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.pending-action-btn--primary {
  border-color: rgba(240, 138, 75, 0.3);
  color: rgba(255, 220, 200, 0.8);
  background: rgba(240, 138, 75, 0.1);
}

.pending-action-btn--primary:hover:not(:disabled) {
  background: rgba(240, 138, 75, 0.18);
  border-color: rgba(240, 138, 75, 0.5);
}

.pending-action-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.pending-close-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 8px;
  background: none;
  color: rgba(255, 255, 255, 0.35);
  cursor: pointer;
  transition: all 0.12s ease;
}

.pending-close-btn:hover {
  background: rgba(255, 255, 255, 0.06);
  color: rgba(255, 255, 255, 0.65);
}

.pending-toast {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.45rem 0.85rem;
  margin: 0.5rem 1.25rem 0;
  border-radius: 8px;
  font-size: 0.72rem;
  animation: toastSlideIn 0.2s ease;
}

.pending-toast--success {
  background: rgba(74, 222, 128, 0.1);
  border: 1px solid rgba(74, 222, 128, 0.2);
  color: rgba(134, 239, 172, 0.85);
}

.pending-toast--error {
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.2);
  color: rgba(252, 165, 165, 0.85);
}

.pending-results {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  margin: 0.5rem 1.25rem 0;
}

.pending-result-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 0.35rem 0.55rem;
  border-radius: 7px;
  border: 1px solid rgba(74, 222, 128, 0.12);
  background: rgba(74, 222, 128, 0.05);
  color: rgba(210, 255, 225, 0.75);
  font-size: 0.68rem;
}

.pending-result-row--failed {
  border-color: rgba(239, 68, 68, 0.18);
  background: rgba(239, 68, 68, 0.06);
  color: rgba(252, 165, 165, 0.82);
}

@keyframes toastSlideIn {
  from { opacity: 0; transform: translateY(-4px); }
  to { opacity: 1; transform: translateY(0); }
}

.pending-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  padding: 2rem 0;
  color: rgba(255, 255, 255, 0.35);
  font-size: 0.78rem;
}

.pending-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.6rem 0.85rem;
  margin: 0.75rem 1.25rem;
  border-radius: 10px;
  border: 1px solid rgba(240, 138, 75, 0.25);
  background: rgba(240, 138, 75, 0.08);
  color: rgba(255, 200, 180, 0.85);
  font-size: 0.72rem;
}

.pending-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  padding: 2rem 0;
  color: rgba(255, 255, 255, 0.3);
  font-size: 0.78rem;
}

.pending-list {
  flex: 1;
  overflow-y: auto;
  padding: 0.5rem 1.25rem 1rem;
}

.pending-group {
  margin-bottom: 0.75rem;
}

.pending-group-header {
  font-size: 0.65rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: rgba(255, 255, 255, 0.3);
  padding: 0.25rem 0;
  margin-bottom: 0.35rem;
}

.pending-item {
  padding: 0.55rem 0.65rem;
  border: 1px solid rgba(255, 255, 255, 0.04);
  border-radius: 8px;
  margin-bottom: 0.3rem;
  background: rgba(0, 0, 0, 0.12);
  transition: border-color 0.12s ease;
}

.pending-item:hover {
  border-color: rgba(255, 255, 255, 0.08);
}

.pending-item-main {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.pending-item-label {
  font-size: 0.8rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.8);
}

.pending-item-badge {
  font-size: 0.6rem;
  padding: 0.1rem 0.4rem;
  border-radius: 999px;
  font-weight: 500;
  flex-shrink: 0;
}

.badge--move {
  background: rgba(147, 197, 253, 0.1);
  border: 1px solid rgba(147, 197, 253, 0.2);
  color: rgba(147, 197, 253, 0.75);
}

.badge--reparent {
  background: rgba(196, 181, 253, 0.1);
  border: 1px solid rgba(196, 181, 253, 0.2);
  color: rgba(196, 181, 253, 0.75);
}

.badge--create {
  background: rgba(74, 222, 128, 0.1);
  border: 1px solid rgba(74, 222, 128, 0.2);
  color: rgba(134, 239, 172, 0.75);
}

.badge--delete {
  background: rgba(252, 165, 165, 0.1);
  border: 1px solid rgba(252, 165, 165, 0.2);
  color: rgba(252, 165, 165, 0.75);
}

.badge--default {
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.1);
  color: rgba(255, 255, 255, 0.5);
}

.pending-item-parent {
  font-size: 0.68rem;
  color: rgba(255, 255, 255, 0.35);
  margin-top: 0.2rem;
}

.pending-item-reason {
  font-size: 0.7rem;
  color: rgba(255, 255, 255, 0.45);
  margin-top: 0.2rem;
  line-height: 1.4;
}

.pending-item-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 0.35rem;
}

.pending-item-time {
  font-size: 0.6rem;
  color: rgba(255, 255, 255, 0.2);
}

.pending-approve-btn {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  border: 1px solid rgba(240, 138, 75, 0.2);
  border-radius: 6px;
  background: rgba(240, 138, 75, 0.06);
  color: rgba(255, 220, 200, 0.7);
  font-size: 0.65rem;
  padding: 0.2rem 0.6rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.pending-approve-btn:hover:not(:disabled) {
  background: rgba(240, 138, 75, 0.14);
  border-color: rgba(240, 138, 75, 0.4);
  color: rgba(255, 220, 200, 0.9);
}

.pending-approve-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
</style>
