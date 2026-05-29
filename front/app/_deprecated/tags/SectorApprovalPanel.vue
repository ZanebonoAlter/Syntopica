<script setup lang="ts">
import { computed, ref } from 'vue'
import { Icon } from '@iconify/vue'
import type { SectorDiff, SectorDiffExecutionItemResult, SectorDiffExecutionResult } from '~/api/boardConcepts'
import { useBoardConceptsApi } from '~/api/boardConcepts'

interface SuggestionItem {
  id: string
  type: 'keep' | 'add' | 'merge' | 'split'
  name: string
  description: string
  affectedCount: number
  sourceIds: number[]
}

interface ExecutionResult {
  sectorCounts: { created: number; merged: number; split: number }
  affectedTags: number
  success: number
  failed: number
  failedReasons: string[]
  items: SectorDiffExecutionItemResult[]
}

const props = defineProps<{
  diff: SectorDiff
  loading: boolean
  category: string
}>()

const emit = defineEmits<{
  done: []
  cancel: []
}>()

const suggestions = computed((): SuggestionItem[] => {
  const items: SuggestionItem[] = []
  const keepItems = props.diff.keep || []
  const addItems = props.diff.add || []
  const mergeItems = props.diff.merge || []
  const splitItems = props.diff.split || []

  keepItems.forEach((item, i) => {
    items.push({
      id: `keep-${i}`,
      type: 'keep',
      name: item.name,
      description: '',
      affectedCount: 0,
      sourceIds: [item.id],
    })
  })

  addItems.forEach((item, i) => {
    items.push({
      id: `add-${i}`,
      type: 'add',
      name: item.name,
      description: item.description || '',
      affectedCount: 0,
      sourceIds: [],
    })
  })

  mergeItems.forEach((item, i) => {
    items.push({
      id: `merge-${i}`,
      type: 'merge',
      name: item.name,
      description: `合并 ${item.source_ids.length} 个源 → 目标 #${item.target_id}`,
      affectedCount: item.source_ids.length,
      sourceIds: item.source_ids,
    })
  })

  splitItems.forEach((item, i) => {
    items.push({
      id: `split-${i}`,
      type: 'split',
      name: `源 #${item.source_id}`,
      description: `拆分为 ${item.new_items.length} 个板块: ${item.new_items.map((ni) => ni.name).join('、')}`,
      affectedCount: item.new_items.length,
      sourceIds: [item.source_id],
    })
  })

  return items
})

const totalAffectedTags = computed(() => props.diff.affected_tag_count)

const acceptedIds = ref<Set<string>>(new Set())
const rejectedIds = ref<Set<string>>(new Set())

function toggleAccept(id: string) {
  if (rejectedIds.value.has(id)) {
    rejectedIds.value.delete(id)
  }
  if (acceptedIds.value.has(id)) {
    acceptedIds.value.delete(id)
  } else {
    acceptedIds.value.add(id)
  }
  rejectedIds.value = new Set(rejectedIds.value)
  acceptedIds.value = new Set(acceptedIds.value)
}

function toggleReject(id: string) {
  if (acceptedIds.value.has(id)) {
    acceptedIds.value.delete(id)
  }
  if (rejectedIds.value.has(id)) {
    rejectedIds.value.delete(id)
  } else {
    rejectedIds.value.add(id)
  }
  acceptedIds.value = new Set(acceptedIds.value)
  rejectedIds.value = new Set(rejectedIds.value)
}

function summarizeExecution(result: SectorDiffExecutionResult): ExecutionResult {
  const successful = result.results.filter((item) => item.status === 'success')
  return {
    sectorCounts: {
      created: successful
        .filter((item) => item.operation === 'add')
        .reduce((sum, item) => sum + (item.created_ids?.length || 0), 0),
      merged: successful.filter((item) => item.operation === 'merge').length,
      split: successful.filter((item) => item.operation === 'split').length,
    },
    affectedTags: result.affected_tag_count,
    success: result.success_count,
    failed: result.failed_count,
    failedReasons: result.results
      .filter((item) => item.status === 'failed')
      .map((item) => `${typeLabel(item.operation)} ${item.name || item.source_id || item.target_id || ''}: ${item.error || '执行失败'}`),
    items: result.results,
  }
}

function resultDetail(item: SectorDiffExecutionItemResult): string {
  if (item.status === 'failed') {
    return item.error || '执行失败'
  }
  const parts: string[] = []
  if (item.created_ids?.length) parts.push(`创建 #${item.created_ids.join('、#')}`)
  if (item.moved_tag_count > 0) parts.push(`移动 ${item.moved_tag_count} 个标签`)
  if (item.affected_tag_count > 0) parts.push(`影响 ${item.affected_tag_count} 个标签`)
  return parts.join(' · ') || '已执行'
}

const acceptedSuggestionIds = computed(() => {
  const accepted = new Set(acceptedIds.value)
  if (accepted.size === 0 && rejectedIds.value.size === 0) {
    return new Set(suggestions.value.map((s) => s.id))
  }
  return accepted
})

const acceptedCount = computed(() => acceptedSuggestionIds.value.size)
const totalSuggestions = computed(() => suggestions.value.length)

const api = useBoardConceptsApi()

async function executeApproval(acceptedIds: string[]) {
  const acceptedSet = new Set(acceptedIds)
  const keep = (props.diff.keep || []).filter((_, i) => acceptedSet.has(`keep-${i}`))
  const add = (props.diff.add || []).filter((_, i) => acceptedSet.has(`add-${i}`))
  const merge = (props.diff.merge || []).filter((_, i) => acceptedSet.has(`merge-${i}`))
  const split = (props.diff.split || []).filter((_, i) => acceptedSet.has(`split-${i}`))
  const filteredDiff: SectorDiff = {
    keep,
    add,
    merge,
    split,
    affected_tag_count: props.diff.affected_tag_count,
  }

  executing.value = true
  execError.value = null
  execProgress.value = { current: 0, total: add.length + merge.length + split.length, label: '准备执行' }

  try {
    const res = await api.confirmRegenerateSectors(props.category, filteredDiff)
    if (res.success && res.data) {
      execResult.value = summarizeExecution(res.data)
      execProgress.value = { current: execProgress.value.total, total: execProgress.value.total, label: '完成' }
    } else {
      execError.value = res.error || '执行失败'
    }
  } catch (e) {
    execError.value = e instanceof Error ? e.message : '执行失败'
  } finally {
    executing.value = false
  }
}

function handleApproveAll() {
  const ids = suggestions.value
    .filter((s) => !rejectedIds.value.has(s.id))
    .map((s) => s.id)
  void executeApproval(ids)
}

function handleConfirm() {
  void executeApproval(Array.from(acceptedSuggestionIds.value))
}

const executing = ref(false)
const execProgress = ref({ current: 0, total: 0, label: '' })
const execResult = ref<ExecutionResult | null>(null)
const execError = ref<string | null>(null)

function handleClose() {
  if (execResult.value) {
    emit('done')
    return
  }
  emit('cancel')
}

function typeIcon(type: string): string {
  switch (type) {
    case 'keep': return 'mdi:check-circle-outline'
    case 'add': return 'mdi:plus-circle-outline'
    case 'merge': return 'mdi:merge'
    case 'split': return 'mdi:call-split'
    default: return 'mdi:help-circle-outline'
  }
}

function typeLabel(type: string): string {
  switch (type) {
    case 'keep': return '保留'
    case 'add': return '新增'
    case 'merge': return '合并'
    case 'split': return '拆分'
    default: return '未知'
  }
}

function typeClass(type: string): string {
  switch (type) {
    case 'keep': return 'card--keep'
    case 'add': return 'card--add'
    case 'merge': return 'card--merge'
    case 'split': return 'card--split'
    default: return ''
  }
}
</script>

<template>
  <Teleport to="body">
    <div class="panel-overlay" @click.self="emit('cancel')">
      <div class="panel-card">
        <div class="panel-header">
          <h3 class="panel-title">LLM 板块建议审批</h3>
          <span class="panel-subtitle">{{ suggestions.length }} 条建议 · 影响 {{ totalAffectedTags }} 个标签</span>
          <button type="button" class="panel-close" @click="emit('cancel')">
            <Icon icon="mdi:close" width="18" />
          </button>
        </div>

        <div class="panel-body">
          <!-- Execution result summary -->
          <div v-if="execResult" class="exec-result">
            <div class="exec-result-header">
              <Icon icon="mdi:check-circle-outline" width="16" class="text-green-400/70" />
              <span>执行完成</span>
            </div>
            <div class="exec-result-stats">
              <div class="exec-stat">
                <span class="exec-stat-label">新增板块</span>
                <span class="exec-stat-value">{{ execResult.sectorCounts.created }}</span>
              </div>
              <div class="exec-stat">
                <span class="exec-stat-label">合并板块</span>
                <span class="exec-stat-value">{{ execResult.sectorCounts.merged }}</span>
              </div>
              <div class="exec-stat">
                <span class="exec-stat-label">拆分板块</span>
                <span class="exec-stat-value">{{ execResult.sectorCounts.split }}</span>
              </div>
              <div class="exec-stat">
                <span class="exec-stat-label">受影响标签</span>
                <span class="exec-stat-value">{{ execResult.affectedTags }}</span>
              </div>
              <div class="exec-stat">
                <span class="exec-stat-label">成功</span>
                <span class="exec-stat-value exec-stat-value--success">{{ execResult.success }}</span>
              </div>
              <div class="exec-stat" v-if="execResult.failed > 0">
                <span class="exec-stat-label">失败</span>
                <span class="exec-stat-value exec-stat-value--fail">{{ execResult.failed }}</span>
              </div>
            </div>
            <div
              v-for="(reason, i) in execResult.failedReasons"
              :key="i"
              class="exec-fail-reason"
            >
              <Icon icon="mdi:alert-circle-outline" width="12" class="text-red-400/60" />
              <span>{{ reason }}</span>
            </div>
            <div class="exec-item-list">
              <div
                v-for="(item, i) in execResult.items"
                :key="`${item.operation}-${i}`"
                class="exec-item"
                :class="{ 'exec-item--failed': item.status === 'failed' }"
              >
                <span class="exec-item-type">{{ typeLabel(item.operation) }}</span>
                <span class="exec-item-detail">{{ resultDetail(item) }}</span>
              </div>
            </div>
          </div>

          <!-- Execution error -->
          <div v-if="execError" class="exec-error">
            <Icon icon="mdi:alert-circle-outline" width="14" />
            <span>{{ execError }}</span>
          </div>

          <!-- Execution progress -->
          <div v-if="executing" class="exec-progress">
            <Icon icon="mdi:loading" width="14" class="animate-spin text-blue-400/60" />
            <span>正在执行: {{ execProgress.label }} ({{ execProgress.current }}/{{ execProgress.total }})</span>
            <div class="exec-progress-bar">
              <div
                class="exec-progress-bar-fill"
                :style="{ width: `${execProgress.total > 0 ? (execProgress.current / execProgress.total) * 100 : 0}%` }"
              />
            </div>
          </div>

          <!-- Suggestion cards -->
          <div class="suggestion-cards">
            <div
              v-for="suggestion in suggestions"
              :key="suggestion.id"
              class="suggestion-card"
              :class="[
                typeClass(suggestion.type),
                { 'suggestion-card--accepted': acceptedIds.has(suggestion.id) },
                { 'suggestion-card--rejected': rejectedIds.has(suggestion.id) },
              ]"
            >
              <div class="card-header">
                <div class="card-type">
                  <Icon :icon="typeIcon(suggestion.type)" width="14" />
                  <span>{{ typeLabel(suggestion.type) }}</span>
                </div>
                <div class="card-actions">
                  <button
                    type="button"
                    class="card-action-btn card-action-btn--accept"
                    :class="{ 'card-action-btn--active': acceptedIds.has(suggestion.id) }"
                    @click="toggleAccept(suggestion.id)"
                  >
                    <Icon
                      :icon="acceptedIds.has(suggestion.id) ? 'mdi:check-circle' : 'mdi:check-circle-outline'"
                      width="14"
                    />
                    接受
                  </button>
                  <button
                    type="button"
                    class="card-action-btn card-action-btn--reject"
                    :class="{ 'card-action-btn--active': rejectedIds.has(suggestion.id) }"
                    @click="toggleReject(suggestion.id)"
                  >
                    <Icon
                      :icon="rejectedIds.has(suggestion.id) ? 'mdi:close-circle' : 'mdi:close-circle-outline'"
                      width="14"
                    />
                    拒绝
                  </button>
                </div>
              </div>
              <div class="card-body">
                <p class="card-name">{{ suggestion.name }}</p>
                <p v-if="suggestion.description" class="card-desc">{{ suggestion.description }}</p>
              </div>
            </div>
          </div>

          <!-- Empty state -->
          <div v-if="suggestions.length === 0" class="suggestion-empty">
            <Icon icon="mdi:robot-outline" width="32" class="text-white/15" />
            <p>LLM 未生成任何板块建议</p>
            <p class="text-xs text-white/25 mt-1">尝试手动添加板块或等待更多标签数据</p>
          </div>
        </div>

        <div class="panel-footer">
          <button type="button" class="panel-btn panel-btn--ghost" @click="handleClose">
            {{ execResult ? '关闭' : '取消' }}
          </button>
          <button
            v-if="!execResult"
            type="button"
            class="panel-btn panel-btn--primary"
            :disabled="loading || executing || acceptedSuggestionIds.size === 0"
            @click="handleApproveAll"
          >
            <Icon v-if="loading || executing" icon="mdi:loading" width="14" class="animate-spin mr-1" />
            全部批准 ({{ acceptedCount }}/{{ totalSuggestions }})
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.panel-overlay {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(8, 12, 18, 0.75);
  backdrop-filter: blur(8px);
}

.panel-card {
  width: min(600px, 92%);
  max-height: 85vh;
  border-radius: 1.25rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(17, 27, 38, 0.98);
  padding: 1.5rem;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
}

.panel-header {
  display: flex;
  align-items: center;
  gap: 0.65rem;
  margin-bottom: 1rem;
  flex-shrink: 0;
}

.panel-title {
  font-size: 0.95rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.9);
}

.panel-subtitle {
  font-size: 0.7rem;
  color: rgba(255, 255, 255, 0.35);
  flex: 1;
}

.panel-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 8px;
  background: none;
  color: rgba(255, 255, 255, 0.4);
  cursor: pointer;
  transition: all 0.12s ease;
}

.panel-close:hover {
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.7);
}

.panel-body {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding-right: 0.25rem;
}

/* Execution results */
.exec-result {
  padding: 1rem;
  border-radius: 10px;
  border: 1px solid rgba(74, 222, 128, 0.12);
  background: rgba(74, 222, 128, 0.06);
}

.exec-result-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-bottom: 0.75rem;
  font-size: 0.85rem;
  color: rgba(134, 239, 172, 0.9);
  font-weight: 500;
}

.exec-result-stats {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.exec-stat {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.25rem 0.6rem;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.05);
}

.exec-stat-label {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.4);
}

.exec-stat-value {
  font-size: 0.75rem;
  color: rgba(255, 255, 255, 0.8);
  font-weight: 500;
}

.exec-stat-value--success {
  color: rgba(134, 239, 172, 0.9);
}

.exec-stat-value--fail {
  color: rgba(252, 165, 165, 0.9);
}

.exec-fail-reason {
  display: flex;
  align-items: flex-start;
  gap: 0.35rem;
  margin-top: 0.5rem;
  font-size: 0.7rem;
  color: rgba(252, 165, 165, 0.7);
}

.exec-item-list {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  margin-top: 0.75rem;
}

.exec-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.4rem 0.55rem;
  border-radius: 7px;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(134, 239, 172, 0.1);
}

.exec-item--failed {
  border-color: rgba(252, 165, 165, 0.16);
  background: rgba(239, 68, 68, 0.06);
}

.exec-item-type {
  flex-shrink: 0;
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.45);
}

.exec-item-detail {
  text-align: right;
  font-size: 0.68rem;
  color: rgba(255, 255, 255, 0.65);
}

.exec-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.6rem 0.8rem;
  border-radius: 10px;
  border: 1px solid rgba(240, 138, 75, 0.25);
  background: rgba(240, 138, 75, 0.08);
  color: rgba(255, 200, 180, 0.85);
  font-size: 0.75rem;
}

/* Execution progress */
.exec-progress {
  padding: 0.75rem 0.8rem;
  border-radius: 10px;
  border: 1px solid rgba(99, 179, 237, 0.12);
  background: rgba(99, 179, 237, 0.06);
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  font-size: 0.72rem;
  color: rgba(147, 197, 253, 0.8);
}

.exec-progress-bar {
  height: 3px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
  overflow: hidden;
}

.exec-progress-bar-fill {
  height: 100%;
  border-radius: 999px;
  background: linear-gradient(90deg, rgba(99, 179, 237, 0.7), rgba(147, 197, 253, 0.9));
  transition: width 0.3s ease;
}

/* Suggestion cards */
.suggestion-cards {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.suggestion-card {
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 10px;
  padding: 0.75rem;
  background: rgba(0, 0, 0, 0.12);
  transition: all 0.12s ease;
}

.suggestion-card--accepted {
  border-color: rgba(74, 222, 128, 0.25);
  background: rgba(74, 222, 128, 0.06);
}

.suggestion-card--rejected {
  opacity: 0.4;
  border-color: rgba(255, 255, 255, 0.04);
}

.card--keep { border-left: 3px solid rgba(74, 222, 128, 0.5); }
.card--add { border-left: 3px solid rgba(96, 165, 250, 0.5); }
.card--merge { border-left: 3px solid rgba(168, 85, 247, 0.5); }
.card--split { border-left: 3px solid rgba(250, 204, 21, 0.5); }

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.5rem;
}

.card-type {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.65rem;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: rgba(255, 255, 255, 0.4);
}

.card--keep .card-type { color: rgba(134, 239, 172, 0.6); }
.card--add .card-type { color: rgba(147, 197, 253, 0.6); }
.card--merge .card-type { color: rgba(196, 181, 253, 0.6); }
.card--split .card-type { color: rgba(252, 211, 77, 0.6); }

.card-actions {
  display: flex;
  gap: 0.35rem;
}

.card-action-btn {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 6px;
  background: none;
  font-size: 0.65rem;
  padding: 0.2rem 0.45rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.card-action-btn--accept {
  color: rgba(134, 239, 172, 0.5);
}

.card-action-btn--accept:hover {
  border-color: rgba(74, 222, 128, 0.3);
  color: rgba(134, 239, 172, 0.8);
}

.card-action-btn--accept.card-action-btn--active {
  border-color: rgba(74, 222, 128, 0.4);
  background: rgba(74, 222, 128, 0.15);
  color: rgba(134, 239, 172, 0.95);
}

.card-action-btn--reject {
  color: rgba(252, 165, 165, 0.4);
}

.card-action-btn--reject:hover {
  border-color: rgba(239, 68, 68, 0.3);
  color: rgba(252, 165, 165, 0.7);
}

.card-action-btn--reject.card-action-btn--active {
  border-color: rgba(239, 68, 68, 0.35);
  background: rgba(239, 68, 68, 0.12);
  color: rgba(252, 165, 165, 0.9);
}

.card-body {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.card-name {
  font-size: 0.82rem;
  color: rgba(255, 255, 255, 0.8);
  font-weight: 500;
}

.card-desc {
  font-size: 0.68rem;
  color: rgba(255, 255, 255, 0.35);
}

.suggestion-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.4rem;
  padding: 3rem 0;
  color: rgba(255, 255, 255, 0.3);
  font-size: 0.8rem;
}

.panel-footer {
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
  margin-top: 1rem;
  flex-shrink: 0;
}

.panel-btn {
  display: flex;
  align-items: center;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  background: none;
  color: rgba(255, 255, 255, 0.7);
  font-size: 0.82rem;
  padding: 0.45rem 1.1rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.panel-btn--ghost:hover {
  background: rgba(255, 255, 255, 0.06);
}

.panel-btn--primary {
  border-color: rgba(240, 138, 75, 0.4);
  color: rgba(255, 220, 200, 0.9);
  background: rgba(240, 138, 75, 0.12);
}

.panel-btn--primary:hover:not(:disabled) {
  background: rgba(240, 138, 75, 0.2);
  border-color: rgba(240, 138, 75, 0.6);
}

.panel-btn--primary:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
</style>
