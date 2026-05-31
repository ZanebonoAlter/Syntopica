<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useTagMergePreviewApi } from '~/api/tagMergePreview'
import type { MergeGroup, MergeSuggestion, EvaluateProgress, ScanProgress, LLMVerdict } from '~/types/tagMerge'

interface Props {
  visible: boolean
  scopeCategoryId?: string | null
  scopeFeedId?: string | null
  standalone?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  scopeCategoryId: null,
  scopeFeedId: null,
  standalone: true,
})
const emit = defineEmits<{
  close: []
  merged: []
}>()

const api = useTagMergePreviewApi()

// --- State ---
const loading = ref(false)
const groups = ref<MergeGroup[]>([])
const error = ref<string | null>(null)

// Evaluate
const evaluating = ref(false)
const evalProgress = ref<EvaluateProgress | null>(null)
let evalEs: EventSource | null = null

// Full scan
const scanning = ref(false)
const scanProgress = ref<ScanProgress | null>(null)
let scanEs: EventSource | null = null

// Selection: Set of "target_tag_id:new_tag_id" strings
const selectedKeys = ref<Set<string>>(new Set())
const merging = ref(false)
const mergedCount = ref(0)

// Search (add tag to group)
const searchingGroupId = ref<number | null>(null)
const searchQuery = ref('')
const searchResults = ref<Array<{ id: number; label: string; slug: string; category: string; feed_count: number }>>([])
const searchLoading = ref(false)
let searchTimer: ReturnType<typeof setTimeout> | null = null

// --- Selection helpers ---
function sugKey(targetTagId: number, newTagId: number): string {
  return `${targetTagId}:${newTagId}`
}

function toggleSelect(targetTagId: number, newTagId: number) {
  const key = sugKey(targetTagId, newTagId)
  const next = new Set(selectedKeys.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  selectedKeys.value = next
}

function isSugSelected(targetTagId: number, newTagId: number): boolean {
  return selectedKeys.value.has(sugKey(targetTagId, newTagId))
}

function selectAllInGroup(group: MergeGroup) {
  const next = new Set(selectedKeys.value)
  for (const sug of group.suggestions) {
    next.add(sugKey(group.target_tag_id, sug.new_tag_id))
  }
  selectedKeys.value = next
}

function deselectAllInGroup(group: MergeGroup) {
  const next = new Set(selectedKeys.value)
  for (const sug of group.suggestions) {
    next.delete(sugKey(group.target_tag_id, sug.new_tag_id))
  }
  selectedKeys.value = next
}

function isGroupAllSelected(group: MergeGroup): boolean {
  return group.suggestions.length > 0 && group.suggestions.every(s => isSugSelected(group.target_tag_id, s.new_tag_id))
}

function selectAllMergeable() {
  const next = new Set(selectedKeys.value)
  for (const group of groups.value) {
    for (const sug of group.suggestions) {
      const verdict = parseVerdict(sug.llm_verdict)
      if (!verdict || verdict.should_merge) {
        next.add(sugKey(group.target_tag_id, sug.new_tag_id))
      }
    }
  }
  selectedKeys.value = next
}

function clearSelection() {
  selectedKeys.value = new Set()
}

const selectedCount = computed(() => selectedKeys.value.size)

// --- Load groups ---
async function loadGroups() {
  loading.value = true
  error.value = null
  try {
    const response = await api.loadMergeGroups({ limit: 200 })
    if (response.success && response.data) {
      groups.value = response.data.groups || []
    } else {
      error.value = response.error || '加载失败'
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

// --- Evaluate ---
async function triggerEvaluate() {
  evaluating.value = true
  evalProgress.value = null
  error.value = null

  const response = await api.triggerEvaluate()
  if (!response.success) {
    error.value = response.error || '评估已在进行中'
    evaluating.value = false
    return
  }

  evalEs = api.createEvaluateEventSource((progress: EvaluateProgress) => {
    evalProgress.value = progress
    if (progress.status === 'done' || progress.status === 'error') {
      evalEs?.close()
      evalEs = null
      evaluating.value = false
      if (progress.status === 'done') {
        void loadGroups()
      }
    }
  })
}

function cancelEvaluate() {
  evalEs?.close()
  evalEs = null
  evaluating.value = false
  evalProgress.value = null
}

// --- Full scan ---
async function triggerFullScan() {
  scanning.value = true
  scanProgress.value = null
  error.value = null

  const response = await api.triggerFullScan()
  if (!response.success) {
    error.value = response.error || '扫描已在进行中'
    scanning.value = false
    return
  }

  scanEs = api.createScanEventSource((progress: ScanProgress) => {
    scanProgress.value = progress
    if (progress.status === 'done' || progress.status === 'error') {
      scanEs?.close()
      scanEs = null
      scanning.value = false
      if (progress.status === 'done') {
        void loadGroups()
      }
    }
  })
}

function cancelScan() {
  scanEs?.close()
  scanEs = null
  scanning.value = false
  scanProgress.value = null
}

// --- Batch merge selected ---
async function mergeSelected() {
  if (selectedKeys.value.size === 0) return
  merging.value = true
  let count = 0

  for (const key of selectedKeys.value) {
    const [targetStr, newStr] = key.split(':')
    const targetTagId = Number(targetStr)
    const newTagId = Number(newStr)

    // Find the group and suggestion
    const group = groups.value.find(g => g.target_tag_id === targetTagId)
    const sug = group?.suggestions.find(s => s.new_tag_id === newTagId)
    if (!group || !sug) continue

    const verdict = parseVerdict(sug.llm_verdict)
    const newName = verdict?.suggested_name || group.target_label

    try {
      await api.mergeTagsWithCustomName({
        sourceTagId: newTagId,
        targetTagId,
        newName,
      })
      count++
    } catch (err) {
      console.error(`Failed to merge ${sug.new_label} → ${group.target_label}:`, err)
    }
  }

  mergedCount.value += count
  selectedKeys.value = new Set()

  // Reload to refresh
  void loadGroups()
  merging.value = false
}

// --- Dismiss suggestion ---
async function removeSuggestion(sug: MergeSuggestion, group: MergeGroup) {
  try {
    await api.dismissSuggestion(sug.new_tag_id, group.target_tag_id)
    group.suggestions = group.suggestions.filter(s => s.id !== sug.id)
    if (group.suggestions.length === 0) {
      groups.value = groups.value.filter(g => g.target_tag_id !== group.target_tag_id)
    }
  } catch (err) {
    console.error('Failed to remove suggestion:', err)
  }
}

// --- Search tags to add to group ---
function openSearch(target_tag_id: number) {
  searchingGroupId.value = target_tag_id
  searchQuery.value = ''
  searchResults.value = []
}

function closeSearch() {
  searchingGroupId.value = null
  searchQuery.value = ''
  searchResults.value = []
}

async function onSearchInput() {
  if (searchTimer) clearTimeout(searchTimer)
  if (searchQuery.value.trim().length < 1) {
    searchResults.value = []
    return
  }
  searchTimer = setTimeout(async () => {
    searchLoading.value = true
    try {
      const response = await api.searchTags(searchQuery.value)
      if (response.success && response.data) {
        const group = groups.value.find(g => g.target_tag_id === searchingGroupId.value)
        const existingIds = new Set([
          searchingGroupId.value,
          ...(group?.suggestions.map(s => s.new_tag_id) || []),
        ])
        searchResults.value = (response.data as Array<{ id: number; label: string; slug: string; category: string; feed_count: number }>)
          .filter(t => !existingIds.has(t.id))
      }
    } catch (err) {
      console.error('Search failed:', err)
    } finally {
      searchLoading.value = false
    }
  }, 300)
}

async function addTagToGroup(tagId: number) {
  if (!searchingGroupId.value) return
  try {
    await api.addToGroup(searchingGroupId.value, tagId)
    closeSearch()
    void loadGroups()
  } catch (err) {
    console.error('Add to group failed:', err)
  }
}

// --- Helpers ---
function parseVerdict(raw: string | null): LLMVerdict | null {
  if (!raw) return null
  try {
    return JSON.parse(raw)
  } catch {
    return null
  }
}

function formatSimilarity(similarity: number) {
  return `${Math.round(similarity * 100)}%`
}

// --- Lifecycle ---
watch(() => props.visible, (isVisible) => {
  if (isVisible) {
    void loadGroups()
  }
}, { immediate: true })

function handleClose() {
  if (mergedCount.value > 0) {
    emit('merged')
  }
  emit('close')
  groups.value = []
  selectedKeys.value = new Set()
  mergedCount.value = 0
  error.value = null
}
</script>

<template>
  <Teleport to="body" :disabled="!props.standalone">
    <div v-if="visible" :class="props.standalone ? 'tag-merge-overlay' : 'tag-merge-inline'" @click.self="props.standalone ? handleClose() : undefined">
      <div :class="props.standalone ? 'tag-merge-modal' : 'tag-merge-inline__content'">
        <!-- Header -->
        <header class="tm-header">
          <div>
            <h2 class="text-lg font-semibold text-white">
              标签合并预览
              <span v-if="groups.length" class="ml-2 text-sm font-normal text-[rgba(255,255,255,0.5)]">
                ({{ groups.length }} 组)
              </span>
            </h2>
          </div>
          <div class="flex items-center gap-2">
            <button
              v-if="!evaluating"
              type="button"
              class="tm-btn tm-btn--accent"
              :disabled="groups.length === 0 || scanning"
              @click="triggerEvaluate"
            >
              <Icon icon="mdi:robot-outline" width="16" />
              <span>AI 评估</span>
            </button>
            <button
              v-if="!scanning"
              type="button"
              class="tm-btn"
              @click="triggerFullScan"
            >
              <Icon icon="mdi:radar" width="16" />
              <span>全量扫描</span>
            </button>
            <button type="button" class="tm-close-btn" aria-label="关闭" @click="handleClose">
              <Icon icon="mdi:close" width="18" />
            </button>
          </div>
        </header>

        <!-- Batch action bar -->
        <div v-if="selectedCount > 0" class="tm-batch-bar">
          <span class="tm-batch-bar__count">已选 {{ selectedCount }} 项</span>
          <div class="flex items-center gap-2">
            <button type="button" class="tm-btn tm-btn--sm" @click="selectAllMergeable">
              <Icon icon="mdi:check-all" width="14" />
              <span>全选可合并</span>
            </button>
            <button type="button" class="tm-btn tm-btn--sm" @click="clearSelection">
              <span>清空</span>
            </button>
            <button
              type="button"
              class="tm-btn tm-btn--primary tm-btn--sm"
              :disabled="merging"
              @click="mergeSelected"
            >
              <Icon v-if="merging" icon="mdi:loading" width="14" class="animate-spin" />
              <Icon v-else icon="mdi:call-merge" width="14" />
              <span>合并选中 ({{ selectedCount }})</span>
            </button>
          </div>
        </div>

        <!-- Evaluate progress -->
        <div v-if="evaluating && evalProgress" class="tm-progress">
          <div class="tm-progress__bar">
            <div
              class="tm-progress__fill"
              :style="{ width: `${evalProgress.total_groups ? (evalProgress.completed / evalProgress.total_groups * 100) : 0}%` }"
            />
          </div>
          <div class="tm-progress__info">
            <span>{{ evalProgress.completed }}/{{ evalProgress.total_groups }} 组</span>
            <span v-if="evalProgress.current_target">正在评估「{{ evalProgress.current_target }}」</span>
          </div>
          <button type="button" class="tm-btn tm-btn--sm" @click="cancelEvaluate">
            <Icon icon="mdi:close" width="14" />
          </button>
        </div>

        <!-- Scan progress -->
        <div v-if="scanning && scanProgress" class="tm-progress">
          <div class="tm-progress__bar">
            <div
              class="tm-progress__fill"
              :style="{ width: `${scanProgress.total ? (scanProgress.scanned / scanProgress.total * 100) : 0}%` }"
            />
          </div>
          <div class="tm-progress__info">
            <span>{{ scanProgress.scanned }}/{{ scanProgress.total }} 标签</span>
            <span>发现 {{ scanProgress.new_suggestions }} 个新建议</span>
          </div>
          <button type="button" class="tm-btn tm-btn--sm" @click="cancelScan">
            <Icon icon="mdi:close" width="14" />
          </button>
        </div>

        <!-- Error -->
        <div v-if="error" class="tm-error">
          <Icon icon="mdi:alert-circle-outline" width="16" />
          <span>{{ error }}</span>
        </div>

        <!-- Loading -->
        <div v-if="loading" class="tm-loading">
          <Icon icon="mdi:loading" width="32" class="animate-spin text-[rgba(240,138,75,0.9)]" />
          <p class="mt-4 text-sm text-[rgba(255,255,255,0.65)]">加载中...</p>
        </div>

        <!-- Empty -->
        <div v-else-if="groups.length === 0 && !error" class="tm-empty">
          <Icon icon="mdi:tag-check-outline" width="32" class="text-[rgba(255,255,255,0.3)]" />
          <p class="mt-3 text-sm text-[rgba(255,255,255,0.5)]">没有发现相似标签</p>
          <p class="mt-1 text-xs text-[rgba(255,255,255,0.3)]">试试「全量扫描」查找更多相似对</p>
        </div>

        <!-- Groups -->
        <div v-else class="tm-groups">
          <div v-for="group in groups" :key="group.target_tag_id" class="tm-group">
            <!-- Group header: target tag -->
            <div class="tm-group__header">
              <div class="tm-group__target">
                <button
                  type="button"
                  class="tm-checkbox"
                  :class="{ 'tm-checkbox--checked': isGroupAllSelected(group) }"
                  @click="isGroupAllSelected(group) ? deselectAllInGroup(group) : selectAllInGroup(group)"
                >
                  <Icon v-if="isGroupAllSelected(group)" icon="mdi:check" width="14" />
                </button>
                <span class="tm-group__target-name">{{ group.target_label }}</span>
                <span class="tm-group__target-meta">{{ group.target_articles }} 篇文章</span>
              </div>
              <div class="flex items-center gap-2">
                <button
                  type="button"
                  class="tm-btn tm-btn--sm"
                  @click="openSearch(group.target_tag_id)"
                >
                  <Icon icon="mdi:plus" width="14" />
                  <span>添加标签</span>
                </button>
              </div>
            </div>

            <!-- Search overlay for this group -->
            <div v-if="searchingGroupId === group.target_tag_id" class="tm-search">
              <div class="tm-search__input-row">
                <input
                  v-model="searchQuery"
                  type="text"
                  class="tm-search__input"
                  placeholder="搜索标签..."
                  autofocus
                  @input="onSearchInput"
                  @keyup.escape="closeSearch"
                />
                <button type="button" class="tm-btn tm-btn--sm" @click="closeSearch">
                  <Icon icon="mdi:close" width="14" />
                </button>
              </div>
              <div v-if="searchLoading" class="tm-search__loading">搜索中...</div>
              <div v-else-if="searchResults.length" class="tm-search__results">
                <button
                  v-for="tag in searchResults"
                  :key="tag.id"
                  type="button"
                  class="tm-search__item"
                  @click="addTagToGroup(tag.id)"
                >
                  <span class="tm-search__item-label">{{ tag.label }}</span>
                  <span class="tm-search__item-meta">{{ tag.feed_count }} 篇</span>
                </button>
              </div>
              <div v-else-if="searchQuery.length >= 1" class="tm-search__empty">无结果</div>
            </div>

            <!-- Suggestions list -->
            <div class="tm-suggestions">
              <div
                v-for="sug in group.suggestions"
                :key="sug.id"
                class="tm-suggestion"
                :class="{ 'tm-suggestion--selected': isSugSelected(group.target_tag_id, sug.new_tag_id) }"
                @click="toggleSelect(group.target_tag_id, sug.new_tag_id)"
              >
                <div class="tm-suggestion__main">
                  <button
                    type="button"
                    class="tm-checkbox tm-checkbox--sm"
                    :class="{ 'tm-checkbox--checked': isSugSelected(group.target_tag_id, sug.new_tag_id) }"
                    @click.stop="toggleSelect(group.target_tag_id, sug.new_tag_id)"
                  >
                    <Icon v-if="isSugSelected(group.target_tag_id, sug.new_tag_id)" icon="mdi:check" width="12" />
                  </button>
                  <span class="tm-suggestion__label">{{ sug.new_label }}</span>
                  <span class="tm-suggestion__similarity">{{ formatSimilarity(sug.similarity) }}</span>
                  <span class="tm-suggestion__articles">{{ sug.new_articles }} 篇</span>
                </div>
                <!-- LLM verdict -->
                <div v-if="parseVerdict(sug.llm_verdict)" class="tm-suggestion__verdict">
                  <span
                    class="tm-suggestion__badge"
                    :class="parseVerdict(sug.llm_verdict)!.should_merge ? 'tm-suggestion__badge--yes' : 'tm-suggestion__badge--no'"
                  >
                    {{ parseVerdict(sug.llm_verdict)!.should_merge ? '建议合并' : '不建议' }}
                  </span>
                  <span class="tm-suggestion__arrow">→</span>
                  <span class="tm-suggestion__name">{{ parseVerdict(sug.llm_verdict)!.suggested_name }}</span>
                </div>
                <div v-if="parseVerdict(sug.llm_verdict)?.reason" class="tm-suggestion__reason">
                  {{ parseVerdict(sug.llm_verdict)!.reason }}
                </div>
                <button
                  type="button"
                  class="tm-suggestion__remove"
                  @click.stop="removeSuggestion(sug, group)"
                >
                  <Icon icon="mdi:close" width="12" />
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
/* --- Layout --- */
.tag-merge-overlay {
  position: fixed;
  inset: 0;
  z-index: 78;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1rem;
  background: rgba(8, 12, 18, 0.7);
  backdrop-filter: blur(10px);
}
.tag-merge-modal {
  width: min(48rem, 100%);
  max-height: calc(100vh - 2rem);
  overflow-y: auto;
  border-radius: 1.75rem;
  background: linear-gradient(180deg, rgba(17, 27, 38, 0.98), rgba(9, 15, 23, 1));
  box-shadow: 0 30px 100px rgba(0, 0, 0, 0.32);
  padding: 1.5rem;
}
.tag-merge-inline {
  width: 100%;
}
.tag-merge-inline__content {
  width: 100%;
  max-height: 60vh;
  overflow-y: auto;
  border-radius: 1rem;
  background: linear-gradient(180deg, rgba(17, 27, 38, 0.98), rgba(9, 15, 23, 1));
  border: 1px solid rgba(255, 255, 255, 0.08);
  padding: 1.5rem;
}

/* --- Header --- */
.tm-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  margin-bottom: 1rem;
}
.tm-close-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 2.75rem;
  min-width: 2.75rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 999px;
  background: transparent;
  color: rgba(255, 255, 255, 0.5);
  cursor: pointer;
  transition: all 0.15s ease;
}
.tm-close-btn:hover {
  border-color: rgba(255, 255, 255, 0.2);
  color: rgba(255, 255, 255, 0.85);
}

/* --- Batch bar --- */
.tm-batch-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.6rem 1rem;
  margin-bottom: 0.75rem;
  border-radius: 0.75rem;
  background: rgba(99, 179, 237, 0.1);
  border: 1px solid rgba(99, 179, 237, 0.25);
}
.tm-batch-bar__count {
  font-size: 0.85rem;
  color: rgba(99, 179, 237, 0.9);
  font-weight: 500;
}

/* --- Buttons --- */
.tm-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.35rem;
  border-radius: 999px;
  font-size: 0.82rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
  border: 1px solid rgba(255, 255, 255, 0.12);
  background: transparent;
  color: rgba(255, 255, 255, 0.6);
  padding: 0.5rem 1.1rem;
}
.tm-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.tm-btn:hover:not(:disabled) {
  border-color: rgba(255, 255, 255, 0.25);
  color: rgba(255, 255, 255, 0.85);
}
.tm-btn--primary {
  background: linear-gradient(135deg, rgba(240, 138, 75, 0.85), rgba(220, 110, 55, 0.9));
  color: rgba(255, 245, 235, 0.95);
  border-color: rgba(240, 138, 75, 0.4);
}
.tm-btn--primary:hover:not(:disabled) {
  background: linear-gradient(135deg, rgba(240, 138, 75, 1), rgba(220, 110, 55, 1));
  box-shadow: 0 6px 20px rgba(240, 138, 75, 0.25);
}
.tm-btn--accent {
  border-color: rgba(99, 179, 237, 0.3);
  color: rgba(99, 179, 237, 0.9);
}
.tm-btn--accent:hover:not(:disabled) {
  border-color: rgba(99, 179, 237, 0.5);
  color: rgba(99, 179, 237, 1);
  background: rgba(99, 179, 237, 0.08);
}
.tm-btn--sm {
  padding: 0.3rem 0.75rem;
  font-size: 0.78rem;
  min-height: 1.75rem;
}

/* --- Checkbox --- */
.tm-checkbox {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.25rem;
  height: 1.25rem;
  border-radius: 0.3rem;
  border: 1.5px solid rgba(255, 255, 255, 0.25);
  background: transparent;
  color: rgba(255, 255, 255, 0.6);
  cursor: pointer;
  flex-shrink: 0;
  transition: all 0.15s ease;
}
.tm-checkbox--sm {
  width: 1rem;
  height: 1rem;
  border-radius: 0.25rem;
}
.tm-checkbox--checked {
  border-color: rgba(99, 179, 237, 0.7);
  background: rgba(99, 179, 237, 0.2);
  color: rgba(99, 179, 237, 0.95);
}
.tm-checkbox:hover {
  border-color: rgba(255, 255, 255, 0.45);
}

/* --- Progress --- */
.tm-progress {
  padding: 12px 16px;
  margin-bottom: 12px;
  background: rgba(255, 255, 255, 0.05);
  border-radius: 8px;
  display: flex;
  align-items: center;
  gap: 12px;
}
.tm-progress__bar {
  flex: 1;
  height: 4px;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 2px;
  overflow: hidden;
}
.tm-progress__fill {
  height: 100%;
  background: rgba(240, 138, 75, 0.9);
  transition: width 0.3s ease;
}
.tm-progress__info {
  display: flex;
  gap: 12px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
  white-space: nowrap;
}

/* --- States --- */
.tm-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 3rem 1rem;
}
.tm-error {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  border-radius: 0.75rem;
  border: 1px solid rgba(240, 138, 75, 0.28);
  background: rgba(240, 138, 75, 0.1);
  padding: 0.75rem 1rem;
  color: rgba(255, 220, 200, 0.9);
  font-size: 0.85rem;
  margin-bottom: 1rem;
}
.tm-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 2.5rem 1rem;
}

/* --- Groups --- */
.tm-groups {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}
.tm-group {
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 1rem;
  padding: 1rem;
  background: rgba(255, 255, 255, 0.02);
  transition: border-color 0.15s ease;
}
.tm-group:hover {
  border-color: rgba(255, 255, 255, 0.12);
}
.tm-group__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  margin-bottom: 0.5rem;
}
.tm-group__target {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.tm-group__target-name {
  font-size: 1rem;
  font-weight: 600;
  color: rgba(99, 179, 237, 0.92);
}
.tm-group__target-meta {
  font-size: 0.78rem;
  color: rgba(255, 255, 255, 0.4);
}

/* --- Suggestions --- */
.tm-suggestions {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}
.tm-suggestion {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  padding: 0.5rem 0.6rem;
  border-radius: 0.5rem;
  cursor: pointer;
  transition: background 0.15s ease;
  position: relative;
}
.tm-suggestion:hover {
  background: rgba(255, 255, 255, 0.04);
}
.tm-suggestion--selected {
  background: rgba(99, 179, 237, 0.06);
  border-left: 2px solid rgba(99, 179, 237, 0.5);
  padding-left: calc(0.6rem - 2px);
}
.tm-suggestion__main {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 0;
}
.tm-suggestion__label {
  color: rgba(255, 255, 255, 0.7);
  font-size: 0.88rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tm-suggestion__similarity {
  font-size: 0.72rem;
  border-radius: 999px;
  background: rgba(16, 185, 129, 0.18);
  border: 1px solid rgba(16, 185, 129, 0.35);
  padding: 0.1rem 0.45rem;
  color: rgba(110, 231, 183, 0.92);
  flex-shrink: 0;
}
.tm-suggestion__articles {
  font-size: 0.75rem;
  color: rgba(255, 255, 255, 0.35);
  flex-shrink: 0;
}
.tm-suggestion__verdict {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.78rem;
  padding-left: 1.5rem;
}
.tm-suggestion__badge {
  font-size: 0.68rem;
  border-radius: 999px;
  padding: 0.1rem 0.5rem;
  flex-shrink: 0;
}
.tm-suggestion__badge--yes {
  background: rgba(16, 185, 129, 0.15);
  color: rgba(110, 231, 183, 0.9);
  border: 1px solid rgba(16, 185, 129, 0.3);
}
.tm-suggestion__badge--no {
  background: rgba(248, 113, 113, 0.12);
  color: rgba(248, 113, 113, 0.8);
  border: 1px solid rgba(248, 113, 113, 0.25);
}
.tm-suggestion__arrow {
  color: rgba(240, 138, 75, 0.7);
}
.tm-suggestion__name {
  color: rgba(255, 255, 255, 0.85);
  font-weight: 500;
}
.tm-suggestion__reason {
  font-size: 0.78rem;
  color: rgba(255, 255, 255, 0.45);
  padding-left: 1.5rem;
  line-height: 1.4;
  white-space: normal;
}
.tm-suggestion__remove {
  position: absolute;
  top: 0.5rem;
  right: 0.4rem;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.5rem;
  height: 1.5rem;
  border-radius: 999px;
  border: none;
  background: transparent;
  color: rgba(255, 255, 255, 0.2);
  cursor: pointer;
  flex-shrink: 0;
  transition: all 0.15s ease;
  opacity: 0;
}
.tm-suggestion:hover .tm-suggestion__remove {
  opacity: 1;
}
.tm-suggestion__remove:hover {
  color: rgba(248, 113, 113, 0.9);
  background: rgba(248, 113, 113, 0.1);
}

/* --- Search --- */
.tm-search {
  margin-bottom: 0.5rem;
  padding: 0.6rem;
  border: 1px solid rgba(99, 179, 237, 0.2);
  border-radius: 0.5rem;
  background: rgba(99, 179, 237, 0.04);
}
.tm-search__input-row {
  display: flex;
  gap: 0.4rem;
  margin-bottom: 0.4rem;
}
.tm-search__input {
  flex: 1;
  background: rgba(0, 0, 0, 0.2);
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 0.4rem;
  padding: 0.35rem 0.6rem;
  color: rgba(255, 255, 255, 0.9);
  font-size: 0.82rem;
  outline: none;
  transition: border-color 0.15s ease;
}
.tm-search__input:focus {
  border-color: rgba(99, 179, 237, 0.5);
}
.tm-search__input::placeholder {
  color: rgba(255, 255, 255, 0.3);
}
.tm-search__results {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  max-height: 10rem;
  overflow-y: auto;
}
.tm-search__item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.35rem 0.6rem;
  border-radius: 0.3rem;
  border: none;
  background: transparent;
  color: rgba(255, 255, 255, 0.7);
  font-size: 0.82rem;
  cursor: pointer;
  width: 100%;
  text-align: left;
  transition: all 0.15s ease;
}
.tm-search__item:hover {
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.95);
}
.tm-search__item-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tm-search__item-meta {
  font-size: 0.72rem;
  color: rgba(255, 255, 255, 0.35);
  flex-shrink: 0;
}
.tm-search__loading,
.tm-search__empty {
  padding: 0.4rem;
  font-size: 0.78rem;
  color: rgba(255, 255, 255, 0.35);
}
</style>
