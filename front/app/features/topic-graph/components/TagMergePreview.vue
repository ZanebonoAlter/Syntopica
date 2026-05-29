<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useTagMergePreviewApi } from '~/api/tagMergePreview'
import type { TagMergeCandidate, MergeSummary } from '~/types/tagMerge'

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
  merged: [summary: MergeSummary]
}>()

const api = useTagMergePreviewApi()

type MergeState = 'idle' | 'scanning' | 'preview' | 'summary'

const state = ref<MergeState>('idle')
const candidates = ref<TagMergeCandidate[]>([])
const expandedIds = ref<number[]>([])
const editingId = ref<number | null>(null)
const editedName = ref('')
const customNames = ref<Map<number, string>>(new Map())
const swappedIds = ref<Set<number>>(new Set())
const mergedIds = ref<number[]>([])
const skippedIds = ref<number[]>([])
const failedIds = ref<number[]>([])
const mergingIds = ref<number[]>([])
const batchMerging = ref(false)
const mergeSummary = ref<MergeSummary | null>(null)
const error = ref<string | null>(null)

const visibleCandidates = computed(() =>
  candidates.value.filter(
    c => !mergedIds.value.includes(c.sourceTagId) && !skippedIds.value.includes(c.sourceTagId),
  ),
)

async function startScan() {
  state.value = 'scanning'
  error.value = null

  try {
    const response = await api.scanMergePreview({
      limit: 50,
      includeArticles: true,
      categoryId: props.scopeCategoryId ?? undefined,
      feedId: props.scopeFeedId ?? undefined,
    })
    if (response.success && response.data) {
      candidates.value = response.data.candidates
      state.value = 'preview'
    } else {
      error.value = response.error || '扫描失败'
      state.value = 'preview'
      candidates.value = []
    }
  } catch (err) {
    console.error('Failed to scan merge preview:', err)
    error.value = err instanceof Error ? err.message : '扫描失败'
    state.value = 'preview'
    candidates.value = []
  }
}

watch(() => props.visible, (isVisible) => {
  if (isVisible && state.value === 'idle') {
    void startScan()
  }
}, { immediate: true })

function toggleExpand(id: number) {
  const idx = expandedIds.value.indexOf(id)
  if (idx >= 0) {
    expandedIds.value.splice(idx, 1)
  } else {
    expandedIds.value.push(id)
  }
}

function startEdit(candidate: TagMergeCandidate) {
  editingId.value = candidate.sourceTagId
  editedName.value = customNames.value.get(candidate.sourceTagId) || getDisplayName(candidate)
}

function saveEdit(candidate: TagMergeCandidate) {
  if (editedName.value.trim()) {
    customNames.value.set(candidate.sourceTagId, editedName.value.trim())
  }
  editingId.value = null
}

function cancelEdit() {
  editingId.value = null
}

function skipCandidate(id: number) {
  skippedIds.value.push(id)
}

function toggleSwap(sourceTagId: number) {
  if (swappedIds.value.has(sourceTagId)) {
    swappedIds.value.delete(sourceTagId)
  } else {
    swappedIds.value.add(sourceTagId)
  }
}

async function mergeSingle(candidate: TagMergeCandidate) {
  mergingIds.value.push(candidate.sourceTagId)

  const isSwapped = swappedIds.value.has(candidate.sourceTagId)
  const actualSourceId = isSwapped ? candidate.targetTagId : candidate.sourceTagId
  const actualTargetId = isSwapped ? candidate.sourceTagId : candidate.targetTagId
  const newName = customNames.value.get(candidate.sourceTagId)
    || (isSwapped ? candidate.sourceLabel : candidate.targetLabel)

  try {
    const response = await api.mergeTagsWithCustomName({
      sourceTagId: actualSourceId,
      targetTagId: actualTargetId,
      newName,
    })

    if (response.success) {
      mergedIds.value.push(candidate.sourceTagId)
    } else {
      failedIds.value.push(candidate.sourceTagId)
    }
  } catch (err) {
    console.error('Failed to merge:', err)
    failedIds.value.push(candidate.sourceTagId)
  } finally {
    const idx = mergingIds.value.indexOf(candidate.sourceTagId)
    if (idx >= 0) mergingIds.value.splice(idx, 1)
  }
}

async function batchMerge() {
  batchMerging.value = true

  const remaining = [...visibleCandidates.value]
  for (const candidate of remaining) {
    await mergeSingle(candidate)
  }

  buildSummary()
  state.value = 'summary'
  batchMerging.value = false
}

function buildSummary() {
  const mergedCandidates = candidates.value.filter(c => mergedIds.value.includes(c.sourceTagId))

  mergeSummary.value = {
    mergedCount: mergedIds.value.length,
    skippedCount: skippedIds.value.length,
    failedCount: failedIds.value.length,
    mergedDetails: mergedCandidates.map(c => {
      const isSwapped = swappedIds.value.has(c.sourceTagId)
      return {
        sourceId: isSwapped ? c.targetTagId : c.sourceTagId,
        sourceLabel: isSwapped ? c.targetLabel : c.sourceLabel,
        targetId: isSwapped ? c.sourceTagId : c.targetTagId,
        newLabel: customNames.value.get(c.sourceTagId) || (isSwapped ? c.sourceLabel : c.targetLabel),
      }
    }),
  }
}

function handleClose() {
  if (mergeSummary.value) {
    emit('merged', mergeSummary.value)
  }
  emit('close')

  // Reset state
  state.value = 'idle'
  candidates.value = []
  expandedIds.value = []
  editingId.value = null
  editedName.value = ''
  customNames.value = new Map()
  swappedIds.value = new Set()
  mergedIds.value = []
  skippedIds.value = []
  failedIds.value = []
  mergingIds.value = []
  batchMerging.value = false
  mergeSummary.value = null
  error.value = null
}

function getDisplayName(candidate: TagMergeCandidate) {
  const isSwapped = swappedIds.value.has(candidate.sourceTagId)
  const custom = customNames.value.get(candidate.sourceTagId)
  if (custom) return custom
  return isSwapped ? candidate.sourceLabel : candidate.targetLabel
}

function getDisplaySource(candidate: TagMergeCandidate) {
  const isSwapped = swappedIds.value.has(candidate.sourceTagId)
  return isSwapped ? candidate.targetLabel : candidate.sourceLabel
}

function formatSimilarity(similarity: number) {
  return `${Math.round(similarity * 100)}%`
}
</script>

<template>
  <Teleport to="body" :disabled="!props.standalone">
    <div v-if="visible" :class="props.standalone ? 'tag-merge-overlay' : 'tag-merge-inline'" @click.self="props.standalone ? handleClose() : undefined">
      <div :class="props.standalone ? 'tag-merge-modal' : 'tag-merge-inline__content'">
        <!-- Scanning state -->
        <div v-if="state === 'scanning'" class="tag-merge-loading">
          <Icon icon="mdi:loading" width="32" class="animate-spin text-[rgba(240,138,75,0.9)]" />
          <p class="mt-4 text-sm text-[rgba(255,255,255,0.65)]">正在扫描相似标签...</p>
        </div>

        <!-- Preview state -->
        <template v-else-if="state === 'preview'">
          <header class="tag-merge-header">
            <div>
              <h2 class="text-lg font-semibold text-white">
                标签合并预览
                <span v-if="candidates.length" class="ml-2 text-sm font-normal text-[rgba(255,255,255,0.5)]">
                  ({{ candidates.length }} 对候选)
                </span>
              </h2>
            </div>
            <div class="flex items-center gap-2">
              <button
                v-if="visibleCandidates.length > 0"
                type="button"
                class="tag-merge-action-btn tag-merge-action-btn--primary"
                :disabled="batchMerging"
                @click="batchMerge"
              >
                <Icon icon="mdi:call-merge" width="16" />
                <span>{{ batchMerging ? '合并中...' : '全部合并' }}</span>
              </button>
              <button
                type="button"
                class="tag-merge-close-btn"
                aria-label="关闭"
                @click="handleClose"
              >
                <Icon icon="mdi:close" width="18" />
              </button>
            </div>
          </header>

          <div v-if="error" class="tag-merge-error">
            <Icon icon="mdi:alert-circle-outline" width="16" />
            <span>{{ error }}</span>
          </div>

          <div v-if="visibleCandidates.length === 0 && !error" class="tag-merge-empty">
            <Icon icon="mdi:tag-check-outline" width="32" class="text-[rgba(255,255,255,0.3)]" />
            <p class="mt-3 text-sm text-[rgba(255,255,255,0.5)]">没有发现相似标签</p>
          </div>

          <div v-else class="tag-merge-cards">
            <div
              v-for="candidate in visibleCandidates"
              :key="candidate.sourceTagId"
              class="tag-merge-card"
            >
              <!-- Card header: source → target -->
              <div class="tag-merge-card__header">
                <div class="tag-merge-card__tags">
                  <span class="tag-merge-card__source">{{ getDisplaySource(candidate) }}</span>
                  <button
                    type="button"
                    class="tag-merge-swap-btn"
                    :class="{ 'tag-merge-swap-btn--active': swappedIds.has(candidate.sourceTagId) }"
                    title="交换合并方向"
                    @click="toggleSwap(candidate.sourceTagId)"
                  >
                    <Icon icon="mdi:swap-horizontal" width="16" />
                  </button>
                  <span class="tag-merge-card__target">{{ getDisplayName(candidate) }}</span>
                </div>
                <button
                  type="button"
                  class="tag-merge-icon-btn"
                  aria-label="编辑名称"
                  @click="startEdit(candidate)"
                >
                  <Icon icon="mdi:pencil-outline" width="14" />
                </button>
              </div>

              <!-- Similarity badge + article counts -->
              <div class="tag-merge-card__stats">
                <span class="tag-merge-similarity-badge">
                  <Icon icon="mdi:link-variant" width="12" />
                  {{ formatSimilarity(candidate.similarity) }}
                </span>
                <span class="tag-merge-article-count">
                  {{ candidate.sourceArticles }} → {{ candidate.targetArticles }} 文章
                </span>
              </div>

              <!-- Inline edit -->
              <div v-if="editingId === candidate.sourceTagId" class="tag-merge-card__edit">
                <input
                  v-model="editedName"
                  type="text"
                  class="tag-merge-edit-input"
                  placeholder="合并后名称"
                  @keyup.enter="saveEdit(candidate)"
                  @keyup.escape="cancelEdit"
                />
                <button type="button" class="tag-merge-icon-btn tag-merge-icon-btn--confirm" @click="saveEdit(candidate)">
                  <Icon icon="mdi:check" width="16" />
                </button>
                <button type="button" class="tag-merge-icon-btn" @click="cancelEdit">
                  <Icon icon="mdi:close" width="16" />
                </button>
              </div>

              <!-- Expand toggle -->
              <button
                type="button"
                class="tag-merge-expand-btn"
                @click="toggleExpand(candidate.sourceTagId)"
              >
                <Icon
                  :icon="expandedIds.includes(candidate.sourceTagId) ? 'mdi:chevron-up' : 'mdi:chevron-down'"
                  width="16"
                />
                <span>{{ expandedIds.includes(candidate.sourceTagId) ? '收起文章' : '查看文章' }}</span>
              </button>

              <!-- Article titles -->
              <div v-if="expandedIds.includes(candidate.sourceTagId)" class="tag-merge-articles">
                <div class="tag-merge-articles__col">
                  <p class="tag-merge-articles__label">{{ getDisplaySource(candidate) }}</p>
                  <ul v-if="(swappedIds.has(candidate.sourceTagId) ? candidate.targetArticleTitles : candidate.sourceArticleTitles)?.length" class="tag-merge-articles__list">
                    <li v-for="article in (swappedIds.has(candidate.sourceTagId) ? candidate.targetArticleTitles : candidate.sourceArticleTitles)" :key="article.articleId">
                      {{ article.title }}
                    </li>
                  </ul>
                  <p v-else class="tag-merge-articles__empty">暂无文章</p>
                </div>
                <div class="tag-merge-articles__col">
                  <p class="tag-merge-articles__label">{{ getDisplayName(candidate) }}</p>
                  <ul v-if="(swappedIds.has(candidate.sourceTagId) ? candidate.sourceArticleTitles : candidate.targetArticleTitles)?.length" class="tag-merge-articles__list">
                    <li v-for="article in (swappedIds.has(candidate.sourceTagId) ? candidate.sourceArticleTitles : candidate.targetArticleTitles)" :key="article.articleId">
                      {{ article.title }}
                    </li>
                  </ul>
                  <p v-else class="tag-merge-articles__empty">暂无文章</p>
                </div>
              </div>

              <!-- Card actions -->
              <div class="tag-merge-card__actions">
                <button
                  type="button"
                  class="tag-merge-action-btn tag-merge-action-btn--primary tag-merge-action-btn--sm"
                  :disabled="mergingIds.includes(candidate.sourceTagId)"
                  @click="mergeSingle(candidate)"
                >
                  <Icon v-if="mergingIds.includes(candidate.sourceTagId)" icon="mdi:loading" width="14" class="animate-spin" />
                  <Icon v-else icon="mdi:call-merge" width="14" />
                  <span>{{ mergingIds.includes(candidate.sourceTagId) ? '合并中...' : '合并' }}</span>
                </button>
                <button
                  type="button"
                  class="tag-merge-action-btn tag-merge-action-btn--ghost tag-merge-action-btn--sm"
                  @click="skipCandidate(candidate.sourceTagId)"
                >
                  <Icon icon="mdi:skip-next" width="14" />
                  <span>跳过</span>
                </button>
              </div>
            </div>
          </div>
        </template>

        <!-- Summary state -->
        <template v-else-if="state === 'summary'">
          <header class="tag-merge-header">
            <div class="flex items-center gap-3">
              <Icon icon="mdi:check-circle" width="24" class="text-emerald-400" />
              <h2 class="text-lg font-semibold text-white">合并完成</h2>
            </div>
            <button
              type="button"
              class="tag-merge-close-btn"
              aria-label="关闭"
              @click="handleClose"
            >
              <Icon icon="mdi:close" width="18" />
            </button>
          </header>

          <div v-if="mergeSummary" class="tag-merge-summary">
            <div class="tag-merge-summary__stats">
              <div class="tag-merge-summary__stat tag-merge-summary__stat--success">
                <span class="tag-merge-summary__number">{{ mergeSummary.mergedCount }}</span>
                <span class="tag-merge-summary__label">已合并</span>
              </div>
              <div class="tag-merge-summary__stat tag-merge-summary__stat--skipped">
                <span class="tag-merge-summary__number">{{ mergeSummary.skippedCount }}</span>
                <span class="tag-merge-summary__label">已跳过</span>
              </div>
              <div v-if="mergeSummary.failedCount > 0" class="tag-merge-summary__stat tag-merge-summary__stat--failed">
                <span class="tag-merge-summary__number">{{ mergeSummary.failedCount }}</span>
                <span class="tag-merge-summary__label">失败</span>
              </div>
            </div>

            <div v-if="mergeSummary.mergedDetails.length" class="tag-merge-summary__details">
              <h3 class="tag-merge-summary__title">合并详情</h3>
              <ul class="tag-merge-summary__list">
                <li v-for="detail in mergeSummary.mergedDetails" :key="detail.sourceId" class="tag-merge-summary__item">
                  <span class="text-[rgba(255,255,255,0.55)]">{{ detail.sourceLabel }}</span>
                  <Icon icon="mdi:arrow-right" width="14" class="text-[rgba(240,138,75,0.7)]" />
                  <span class="text-[rgba(255,255,255,0.9)]">{{ detail.newLabel }}</span>
                </li>
              </ul>
            </div>
          </div>

          <div class="tag-merge-summary__footer">
            <button type="button" class="tag-merge-action-btn tag-merge-action-btn--primary" @click="handleClose">
              <Icon icon="mdi:check" width="16" />
              <span>完成</span>
            </button>
          </div>
        </template>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
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

.tag-merge-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 3rem 1rem;
}

.tag-merge-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  margin-bottom: 1.25rem;
}

.tag-merge-close-btn {
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

.tag-merge-close-btn:hover {
  border-color: rgba(255, 255, 255, 0.2);
  color: rgba(255, 255, 255, 0.85);
}

.tag-merge-error {
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

.tag-merge-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 2.5rem 1rem;
}

.tag-merge-cards {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.tag-merge-card {
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 1rem;
  padding: 1rem;
  background: rgba(255, 255, 255, 0.03);
  transition:
    border-color 0.15s ease,
    box-shadow 0.15s ease;
}

.tag-merge-card:hover {
  border-color: rgba(255, 255, 255, 0.12);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.15);
}

.tag-merge-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.tag-merge-card__tags {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.92rem;
  min-width: 0;
}

.tag-merge-swap-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.25rem;
  height: 1.25rem;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: transparent;
  color: rgba(255, 255, 255, 0.25);
  cursor: pointer;
  flex-shrink: 0;
  transition: all 0.15s ease;
}

.tag-merge-swap-btn:hover {
  border-color: rgba(240, 138, 75, 0.4);
  color: rgba(240, 138, 75, 0.8);
}

.tag-merge-swap-btn--active {
  border-color: rgba(240, 138, 75, 0.5);
  color: rgba(240, 138, 75, 0.9);
  background: rgba(240, 138, 75, 0.1);
}

.tag-merge-card__source {
  color: rgba(255, 255, 255, 0.55);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tag-merge-card__target {
  color: rgba(99, 179, 237, 0.92);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 500;
}

.tag-merge-card__stats {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-top: 0.5rem;
}

.tag-merge-similarity-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  border-radius: 999px;
  background: rgba(16, 185, 129, 0.18);
  border: 1px solid rgba(16, 185, 129, 0.35);
  padding: 0.15rem 0.55rem;
  font-size: 0.72rem;
  color: rgba(110, 231, 183, 0.92);
}

.tag-merge-article-count {
  font-size: 0.78rem;
  color: rgba(255, 255, 255, 0.45);
}

.tag-merge-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.75rem;
  height: 1.75rem;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: transparent;
  color: rgba(255, 255, 255, 0.45);
  cursor: pointer;
  flex-shrink: 0;
  transition: all 0.15s ease;
}

.tag-merge-icon-btn:hover {
  border-color: rgba(255, 255, 255, 0.25);
  color: rgba(255, 255, 255, 0.8);
}

.tag-merge-icon-btn--confirm {
  color: rgba(110, 231, 183, 0.8);
  border-color: rgba(110, 231, 183, 0.25);
}

.tag-merge-icon-btn--confirm:hover {
  border-color: rgba(110, 231, 183, 0.5);
  color: rgba(110, 231, 183, 1);
}

.tag-merge-card__edit {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-top: 0.6rem;
}

.tag-merge-edit-input {
  flex: 1;
  background: rgba(0, 0, 0, 0.2);
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 0.5rem;
  padding: 0.45rem 0.75rem;
  color: rgba(255, 255, 255, 0.9);
  font-size: 0.85rem;
  outline: none;
  transition: border-color 0.15s ease;
}

.tag-merge-edit-input:focus {
  border-color: rgba(240, 138, 75, 0.5);
}

.tag-merge-edit-input::placeholder {
  color: rgba(255, 255, 255, 0.3);
}

.tag-merge-expand-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  margin-top: 0.6rem;
  border: none;
  background: transparent;
  color: rgba(255, 255, 255, 0.4);
  font-size: 0.78rem;
  cursor: pointer;
  padding: 0.25rem 0;
  transition: color 0.15s ease;
}

.tag-merge-expand-btn:hover {
  color: rgba(255, 255, 255, 0.65);
}

.tag-merge-articles {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.75rem;
  margin-top: 0.6rem;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  padding-top: 0.6rem;
}

.tag-merge-articles__col {
  min-width: 0;
  max-height: 10rem;
  overflow-y: auto;
}

.tag-merge-articles__label {
  font-size: 0.72rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.5);
  text-transform: uppercase;
  letter-spacing: 0.08em;
  margin-bottom: 0.35rem;
}

.tag-merge-articles__list {
  list-style: none;
  padding: 0;
  margin: 0;
}

.tag-merge-articles__list li {
  font-size: 0.78rem;
  color: rgba(255, 255, 255, 0.65);
  padding: 0.2rem 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tag-merge-articles__empty {
  font-size: 0.78rem;
  color: rgba(255, 255, 255, 0.3);
}

.tag-merge-card__actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 0.75rem;
}

.tag-merge-action-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.35rem;
  border-radius: 999px;
  font-size: 0.82rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
  border: 1px solid transparent;
}

.tag-merge-action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.tag-merge-action-btn--primary {
  background: linear-gradient(135deg, rgba(240, 138, 75, 0.85), rgba(220, 110, 55, 0.9));
  color: rgba(255, 245, 235, 0.95);
  padding: 0.5rem 1.1rem;
  border-color: rgba(240, 138, 75, 0.4);
}

.tag-merge-action-btn--primary:hover:not(:disabled) {
  background: linear-gradient(135deg, rgba(240, 138, 75, 1), rgba(220, 110, 55, 1));
  box-shadow: 0 6px 20px rgba(240, 138, 75, 0.25);
}

.tag-merge-action-btn--ghost {
  background: transparent;
  color: rgba(255, 255, 255, 0.5);
  padding: 0.5rem 1.1rem;
  border: 1px solid rgba(255, 255, 255, 0.12);
}

.tag-merge-action-btn--ghost:hover:not(:disabled) {
  border-color: rgba(255, 255, 255, 0.25);
  color: rgba(255, 255, 255, 0.75);
}

.tag-merge-action-btn--sm {
  padding: 0.35rem 0.85rem;
  font-size: 0.78rem;
}

.tag-merge-summary {
  margin-top: 0.5rem;
}

.tag-merge-summary__stats {
  display: flex;
  gap: 1rem;
  margin-bottom: 1.5rem;
}

.tag-merge-summary__stat {
  display: flex;
  flex-direction: column;
  align-items: center;
  border-radius: 0.75rem;
  padding: 1rem 1.5rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  min-width: 5rem;
}

.tag-merge-summary__stat--success {
  background: rgba(16, 185, 129, 0.1);
  border-color: rgba(16, 185, 129, 0.25);
}

.tag-merge-summary__stat--skipped {
  background: rgba(245, 158, 11, 0.1);
  border-color: rgba(245, 158, 11, 0.25);
}

.tag-merge-summary__stat--failed {
  background: rgba(239, 68, 68, 0.1);
  border-color: rgba(239, 68, 68, 0.25);
}

.tag-merge-summary__number {
  font-size: 1.5rem;
  font-weight: 700;
  color: rgba(255, 255, 255, 0.9);
}

.tag-merge-summary__stat--success .tag-merge-summary__number {
  color: rgba(110, 231, 183, 0.95);
}

.tag-merge-summary__stat--skipped .tag-merge-summary__number {
  color: rgba(252, 211, 77, 0.9);
}

.tag-merge-summary__stat--failed .tag-merge-summary__number {
  color: rgba(248, 113, 113, 0.95);
}

.tag-merge-summary__label {
  margin-top: 0.25rem;
  font-size: 0.75rem;
  color: rgba(255, 255, 255, 0.5);
}

.tag-merge-summary__details {
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  padding-top: 1rem;
}

.tag-merge-summary__title {
  font-size: 0.82rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.65);
  margin-bottom: 0.6rem;
}

.tag-merge-summary__list {
  list-style: none;
  padding: 0;
  margin: 0;
}

.tag-merge-summary__item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.45rem 0;
  font-size: 0.82rem;
}

.tag-merge-summary__footer {
  display: flex;
  justify-content: flex-end;
  margin-top: 1.5rem;
  padding-top: 1rem;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
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
</style>
