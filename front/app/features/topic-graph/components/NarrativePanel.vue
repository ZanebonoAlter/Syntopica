<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { ref, computed, watch } from 'vue'
import { useNarrativeApi, type NarrativeScopeCategory, type BoardTimelineDay, type BoardNarrativeItem, type TagBrief } from '~/api/topicGraph'
import { apiClient } from '~/api/client'
import NarrativeBoardCanvas from './NarrativeBoardCanvas.client.vue'
import NarrativeDetailCard from './NarrativeDetailCard.vue'

interface NarrativeTag {
  id: number
  slug: string
  label: string
  category: string
  kind?: string
}

interface Props {
  date: string
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'select-tag': [tag: NarrativeTag]
}>()

const narrativeApi = useNarrativeApi()
const timelineDaysRange = 7
const loading = ref(false)
const error = ref<string | null>(null)
const expandedIds = ref<Set<number>>(new Set())
const selectedId = ref<number | null>(null)
const hoveredId = ref<number | null>(null)
const triggering = ref(false)
const triggerMessage = ref<string | null>(null)

const scopeMode = ref<'global' | 'category'>('global')
const selectedCategoryId = ref<number | null>(null)
const scopeCategories = ref<NarrativeScopeCategory[]>([])
const scopesLoading = ref(false)

const boardTimelineDays = ref<BoardTimelineDay[]>([])
const expandedBoardIds = ref<Set<number>>(new Set())

interface UnclassifiedTag {
  id: number
  label: string
  description: string
}

const unclassifiedTags = ref<UnclassifiedTag[]>([])
const unclassifiedLoading = ref(false)
const showUnclassified = ref(false)

const statusStyle: Record<string, { label: string; dot: string; ring: string; bg: string; border: string }> = {
  emerging:   { label: '新兴', dot: '#34d399', ring: 'rgba(52,211,153,0.25)',  bg: 'rgba(52,211,153,0.08)',  border: 'rgba(52,211,153,0.35)' },
  continuing: { label: '持续', dot: '#60a5fa', ring: 'rgba(96,165,250,0.25)',  bg: 'rgba(96,165,250,0.08)',  border: 'rgba(96,165,250,0.35)' },
  splitting:  { label: '分化', dot: '#fb923c', ring: 'rgba(251,146,60,0.25)', bg: 'rgba(251,146,60,0.08)', border: 'rgba(251,146,60,0.35)' },
  merging:    { label: '融合', dot: '#c084fc', ring: 'rgba(192,132,252,0.25)', bg: 'rgba(192,132,252,0.08)', border: 'rgba(192,132,252,0.35)' },
  ending:     { label: '终结', dot: '#6b7280', ring: 'rgba(107,114,128,0.25)', bg: 'rgba(107,114,128,0.08)', border: 'rgba(107,114,128,0.35)' },
}

const allBoardNarratives = computed(() => {
  const all: BoardNarrativeItem[] = []
  for (const day of boardTimelineDays.value) {
    for (const board of (day.boards ?? [])) all.push(...(board.narratives ?? []))
  }
  return all
})

interface ExpandedBoardTags {
  boardId: number
  boardName: string
  semanticBoardId?: number
  eventTags: TagBrief[]
}

const expandedBoardsTags = computed(() => {
  const result: ExpandedBoardTags[] = []
  for (const day of boardTimelineDays.value) {
    for (const board of (day.boards ?? [])) {
      if (expandedBoardIds.value.has(board.id)) {
        result.push({
          boardId: board.id,
          boardName: board.name,
          semanticBoardId: board.semantic_board_id,
          eventTags: board.event_tags ?? [],
        })
      }
    }
  }
  return result
})

const tagCategoryStyle: Record<string, { dot: string; bg: string; border: string }> = {
  event:   { dot: '#f87171', bg: 'rgba(248,113,113,0.12)',  border: 'rgba(248,113,113,0.3)' },
  person:  { dot: '#60a5fa', bg: 'rgba(96,165,250,0.12)',   border: 'rgba(96,165,250,0.3)' },
  keyword: { dot: '#34d399', bg: 'rgba(52,211,153,0.12)',   border: 'rgba(52,211,153,0.3)' },
}

function onBoardTagClick(tag: TagBrief) {
  handleDetailTagSelect({ id: tag.id, slug: tag.slug, label: tag.label, category: tag.category, kind: tag.kind })
}

const selectedNarrative = computed(() => {
  if (selectedId.value === null) return null
  return allBoardNarratives.value.find(n => n.id === selectedId.value) ?? null
})

const totalCount = computed(() => allBoardNarratives.value.length)

const showTrigger = computed(() => {
  return scopeMode.value === 'global' || selectedCategoryId.value !== null
})

const activeCategoryName = computed(() => {
  if (selectedCategoryId.value === null) return ''
  const cat = scopeCategories.value.find(c => c.category_id === selectedCategoryId.value)
  return cat?.category_name ?? ''
})

function handleDetailTagSelect(tag: NarrativeTag) {
  emit('select-tag', tag)
}

function toggleExpand(id: number) {
  const next = new Set(expandedIds.value)
  if (next.has(id)) next.delete(id); else next.add(id)
  expandedIds.value = next
}

function handleCanvasSelect(id: number) {
  selectedId.value = selectedId.value === id ? null : id
}

function handleCanvasHover(id: number | null) {
  hoveredId.value = id
}

async function loadScopes() {
  scopesLoading.value = true
  try {
    const response = await narrativeApi.getNarrativeScopes(props.date, timelineDaysRange)
    if (response.success && response.data) {
      scopeCategories.value = response.data.categories ?? []
    } else {
      scopeCategories.value = []
    }
  } catch (err) {
    console.error('Failed to load narrative scopes:', err)
    scopeCategories.value = []
  } finally {
    scopesLoading.value = false
  }
}

async function loadBoardTimeline() {
  loading.value = true
  error.value = null
  try {
    const scopeType = scopeMode.value === 'category' && selectedCategoryId.value !== null ? 'feed_category' : undefined
    const categoryId = scopeMode.value === 'category' ? selectedCategoryId.value ?? undefined : undefined
    const response = await narrativeApi.getBoardTimeline(props.date, timelineDaysRange, scopeType, categoryId)
    if (response.success && response.data) {
      boardTimelineDays.value = response.data
    } else {
      boardTimelineDays.value = []
    }
  } catch (err) {
    console.error('Failed to load board timeline:', err)
    error.value = '版块数据加载失败'
    boardTimelineDays.value = []
  } finally {
    loading.value = false
  }
  void loadUnclassifiedTags()
}

async function loadUnclassifiedTags() {
  unclassifiedLoading.value = true
  try {
    const response = await apiClient.get('/narratives/unclassified')
    if (response.success && response.data) {
      unclassifiedTags.value = (response.data as UnclassifiedTag[]) || []
    }
  } catch {
    unclassifiedTags.value = []
  } finally {
    unclassifiedLoading.value = false
  }
  if (unclassifiedTags.value.length > 0) {
    showUnclassified.value = true
  }
}

function switchScope(mode: 'global' | 'category') {
  scopeMode.value = mode
  selectedCategoryId.value = null
  selectedId.value = null
  hoveredId.value = null
  expandedIds.value = new Set()
  showUnclassified.value = false
  if (mode === 'category') {
    void loadScopes()
  }
  void loadBoardTimeline()
}

function selectCategory(catId: number) {
  selectedCategoryId.value = catId
  selectedId.value = null
  expandedIds.value = new Set()
  void loadBoardTimeline()
}

function backToCategoryList() {
  selectedCategoryId.value = null
  selectedId.value = null
  expandedIds.value = new Set()
  boardTimelineDays.value = []
  void loadBoardTimeline()
}

async function triggerGeneration() {
  triggering.value = true
  triggerMessage.value = null
  try {
    const scopeType = scopeMode.value === 'category' && selectedCategoryId.value !== null ? 'feed_category' : undefined
    const categoryId = scopeMode.value === 'category' ? selectedCategoryId.value ?? undefined : undefined

    const response = await narrativeApi.regenerateNarratives(props.date, scopeType, categoryId)
    if (response.success) {
      triggerMessage.value = '叙事重新整理完成'
      setTimeout(() => { triggerMessage.value = null }, 3000)
      void loadBoardTimeline()
    } else {
      triggerMessage.value = response.error || '重新整理失败'
      setTimeout(() => { triggerMessage.value = null }, 5000)
    }
  } catch (err) {
    console.error('Failed to trigger narrative generation:', err)
    triggerMessage.value = '重新整理失败'
    setTimeout(() => { triggerMessage.value = null }, 5000)
  } finally {
    triggering.value = false
  }
}

watch(() => props.date, () => {
  boardTimelineDays.value = []
  expandedIds.value = new Set()
  selectedId.value = null
  hoveredId.value = null
  scopeCategories.value = []
  selectedCategoryId.value = null
  void loadBoardTimeline()
  if (scopeMode.value === 'category') {
    void loadScopes()
  }
}, { immediate: true })
</script>

<template>
  <section class="narrative-panel">
    <div class="narrative-panel__header">
      <div>
        <p class="narrative-panel__eyebrow">叙事脉络</p>
        <h3 class="narrative-panel__title">
          <template v-if="scopeMode === 'category' && selectedCategoryId !== null">{{ activeCategoryName }} · {{ date }} 叙事脉络</template>
          <template v-else>话题演化时间线</template>
        </h3>
      </div>
      <div class="narrative-panel__actions">
        <span v-if="totalCount" class="narrative-panel__count">{{ totalCount }} 条叙事</span>
        <button
          v-if="showTrigger"
          type="button"
          class="narrative-panel__trigger"
          :disabled="triggering"
          @click="triggerGeneration"
        >
          <Icon v-if="triggering" icon="mdi:loading" width="14" class="animate-spin" />
          <Icon v-else icon="mdi:auto-fix" width="14" />
          {{ triggering ? '整理中...' : '重新整理' }}
        </button>
      </div>
    </div>

    <div v-if="triggerMessage" class="narrative-panel__msg">
      {{ triggerMessage }}
    </div>

    <template v-if="boardTimelineDays.length > 0">
      <div v-if="selectedCategoryId === null" class="narrative-panel__scope-switcher">
        <button
          type="button"
          class="narrative-panel__scope-btn"
          :class="{ 'narrative-panel__scope-btn--active': scopeMode === 'global' }"
          @click="switchScope('global')"
        >
          全局版块
        </button>
        <button
          type="button"
          class="narrative-panel__scope-btn"
          :class="{ 'narrative-panel__scope-btn--active': scopeMode === 'category' }"
          @click="switchScope('category')"
        >
          分类版块
        </button>
      </div>
      <button
        v-else
        type="button"
        class="narrative-panel__back"
        @click="backToCategoryList"
      >
        <Icon icon="mdi:chevron-left" width="16" />
        <span>全部版块</span>
      </button>
    </template>

    <template v-if="scopeMode === 'category' && selectedCategoryId === null && boardTimelineDays.length > 0">
      <div v-if="scopeCategories.length > 0" class="narrative-panel__cat-list">
        <button
          v-for="cat in scopeCategories"
          :key="cat.category_id"
          type="button"
          class="narrative-panel__cat-card"
          @click="selectCategory(cat.category_id)"
        >
          <div class="narrative-panel__cat-icon" :style="{ background: cat.category_color + '22', color: cat.category_color }">
            <Icon :icon="cat.category_icon || 'mdi:folder'" width="18" />
          </div>
          <div class="narrative-panel__cat-info">
            <span class="narrative-panel__cat-name">{{ cat.category_name }}</span>
          </div>
          <span class="narrative-panel__cat-badge">{{ cat.board_count }}</span>
          <Icon icon="mdi:chevron-right" width="16" class="narrative-panel__cat-arrow" />
        </button>
      </div>
      <div v-else class="narrative-panel__centered">
        <Icon icon="mdi:text-box-search-outline" width="28" class="text-white/20" />
        <p>暂无分类版块</p>
        <p class="narrative-panel__empty-hint">请先完成一次叙事整理</p>
      </div>
    </template>

    <template v-if="scopeMode === 'global' || (scopeMode === 'category' && selectedCategoryId !== null)">
      <div v-if="loading" class="narrative-panel__centered">
        <Icon icon="mdi:loading" width="20" class="animate-spin text-white/40" />
        <span>正在加载版块数据...</span>
      </div>

      <div v-else-if="boardTimelineDays.length === 0" class="narrative-panel__centered">
        <Icon icon="mdi:text-box-search-outline" width="28" class="text-white/20" />
        <p>近 {{ timelineDaysRange }} 天暂无版块数据</p>
        <p class="narrative-panel__empty-hint">版块会在叙事整理过程中自动生成</p>
      </div>

      <div v-else class="narrative-panel__body">
        <ClientOnly>
            <NarrativeBoardCanvas
              :days="boardTimelineDays"
              :selected-id="selectedId"
              :expanded-board-ids="expandedBoardIds"
              @select="handleCanvasSelect"
              @hover="handleCanvasHover"
              @board-toggle="expandedBoardIds = $event"
            />

          <Transition name="detail-slide">
            <NarrativeDetailCard
              v-if="selectedNarrative"
              :narrative="selectedNarrative"
              :expanded="expandedIds.has(selectedNarrative.id)"
              :status-style="statusStyle"
              @select-tag="handleDetailTagSelect"
              @toggle-expand="toggleExpand(selectedNarrative.id)"
              @close="selectedId = null"
            />
          </Transition>
        </ClientOnly>

        <div v-if="expandedBoardsTags.length > 0" class="narrative-panel__board-tags">
          <div
            v-for="bt in expandedBoardsTags"
            :key="bt.boardId"
            class="board-tags-group"
          >
            <div class="board-tags-group__header">
              <span class="board-tags-group__name">{{ bt.boardName }}</span>
              <span v-if="bt.semanticBoardId" class="board-tags-group__source">
                <Icon icon="mdi:puzzle-outline" width="11" />
                语义板块 #{{ bt.semanticBoardId }}
              </span>
              <span class="board-tags-group__count">{{ bt.eventTags.length }} 个标签</span>
            </div>
            <div class="board-tags-group__chips">
              <button
                v-for="tag in bt.eventTags"
                :key="`e-${tag.id}`"
                type="button"
                class="board-tag-chip"
                :style="{
                  '--dot': (tagCategoryStyle[tag.category] ?? tagCategoryStyle.keyword!).dot,
                  '--bg': (tagCategoryStyle[tag.category] ?? tagCategoryStyle.keyword!).bg,
                  '--border': (tagCategoryStyle[tag.category] ?? tagCategoryStyle.keyword!).border,
                }"
                @click="onBoardTagClick(tag)"
              >
                <span class="board-tag-chip__dot" />
                <span class="board-tag-chip__label">{{ tag.label }}</span>
              </button>

            </div>
          </div>
        </div>

        <div v-if="unclassifiedTags.length > 0" class="narrative-panel__unclassified">
          <div class="unclassified-header" role="button" tabindex="0" @click="showUnclassified = !showUnclassified" @keydown.enter="showUnclassified = !showUnclassified" @keydown.space.prevent="showUnclassified = !showUnclassified">
            <Icon :icon="showUnclassified ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="16" class="text-white/40" />
            <Icon icon="mdi:tag-off-outline" width="18" class="text-white/40" />
            <span>未归类标签 ({{ unclassifiedTags.length }})</span>
          </div>
          <div v-if="showUnclassified" class="unclassified-list">
            <div v-for="tag in unclassifiedTags" :key="tag.id" class="unclassified-tag-item">
              <div class="unclassified-tag-info">
                <span class="unclassified-tag-label">{{ tag.label }}</span>
                <span v-if="tag.description" class="unclassified-tag-desc">{{ tag.description }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>
  </section>
</template>

<style scoped>
.narrative-panel {
  display: grid;
  gap: 1rem;
}

.narrative-panel__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
}

.narrative-panel__eyebrow {
  font-size: 0.72rem;
  letter-spacing: 0.24em;
  text-transform: uppercase;
  color: rgba(186, 206, 226, 0.72);
}

.narrative-panel__title {
  margin-top: 0.35rem;
  font-size: 0.95rem;
  line-height: 1.55;
  color: rgba(241, 247, 252, 0.9);
}

.narrative-panel__actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-shrink: 0;
}

.narrative-panel__count {
  padding: 0.2rem 0.6rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
  color: rgba(255, 255, 255, 0.5);
  font-size: 0.72rem;
}

.narrative-panel__trigger {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.3rem 0.7rem;
  border-radius: 999px;
  border: 1px solid rgba(240, 138, 75, 0.3);
  background: rgba(240, 138, 75, 0.08);
  color: rgba(240, 180, 140, 0.9);
  font-size: 0.72rem;
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}

.narrative-panel__trigger:hover:not(:disabled) {
  border-color: rgba(240, 138, 75, 0.5);
  background: rgba(240, 138, 75, 0.15);
}

.narrative-panel__trigger:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.narrative-panel__msg {
  padding: 0.45rem 0.8rem;
  border-radius: 10px;
  border: 1px solid rgba(240, 138, 75, 0.2);
  background: rgba(240, 138, 75, 0.06);
  color: rgba(240, 180, 140, 0.8);
  font-size: 0.78rem;
  text-align: center;
}

.narrative-panel__centered {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.6rem;
  padding: 2.5rem 1rem;
  border-radius: 16px;
  border: 1px dashed rgba(186, 206, 226, 0.2);
  background: rgba(255, 255, 255, 0.02);
  color: rgba(186, 206, 226, 0.55);
  font-size: 0.85rem;
  text-align: center;
}

.narrative-panel__centered--error {
  border-color: rgba(240, 138, 75, 0.25);
  color: rgba(240, 180, 140, 0.7);
}

.narrative-panel__empty-hint {
  font-size: 0.75rem;
  color: rgba(186, 206, 226, 0.35);
}

.narrative-panel__body {
  margin-top: 1.25rem;
}

.narrative-panel__board-tags {
  margin-top: 0.75rem;
  display: grid;
  gap: 0.75rem;
}

.board-tags-group {
  padding: 0.7rem 0.85rem;
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(255, 255, 255, 0.03);
}

.board-tags-group__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.5rem;
}

.board-tags-group__name {
  font-size: 0.78rem;
  color: rgba(241, 247, 252, 0.7);
  font-weight: 500;
}

.board-tags-group__count {
  font-size: 0.68rem;
  color: rgba(186, 206, 226, 0.45);
}

.board-tags-group__source {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
  font-size: 0.65rem;
  color: rgba(240, 138, 75, 0.7);
  padding: 0.1rem 0.4rem;
  border-radius: 6px;
  background: rgba(240, 138, 75, 0.08);
  border: 1px solid rgba(240, 138, 75, 0.15);
}

.board-tags-group__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
}

.board-tag-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.2rem 0.55rem;
  border-radius: 6px;
  border: 1px solid var(--border, rgba(255,255,255,0.12));
  background: var(--bg, rgba(255,255,255,0.06));
  cursor: pointer;
  transition: all 0.12s ease;
  font-size: 0.7rem;
  color: rgba(241, 247, 252, 0.8);
}

.board-tag-chip:hover {
  background: var(--bg, rgba(255,255,255,0.12));
  border-color: var(--dot, rgba(255,255,255,0.3));
  transform: translateY(-1px);
}

.board-tag-chip__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--dot, rgba(255,255,255,0.5));
  flex-shrink: 0;
}

.board-tag-chip__icon {
  color: rgba(186, 206, 226, 0.5);
  flex-shrink: 0;
}

.board-tag-chip__label {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 10em;
}

.narrative-panel__unclassified {
  margin-top: 1rem;
  padding: 0.8rem;
  border-radius: 12px;
  border: 1px dashed rgba(186, 206, 226, 0.15);
  background: rgba(255, 255, 255, 0.02);
}

.unclassified-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-bottom: 0.6rem;
  font-size: 0.78rem;
  color: rgba(186, 206, 226, 0.55);
  cursor: pointer;
  user-select: none;
}

.unclassified-header:hover {
  color: rgba(241, 247, 252, 0.75);
}

.unclassified-list {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
}

.unclassified-tag-item {
  padding: 0.25rem 0.6rem;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(255, 255, 255, 0.03);
}

.unclassified-tag-label {
  font-size: 0.72rem;
  color: rgba(241, 247, 252, 0.7);
}

.unclassified-tag-desc {
  display: block;
  font-size: 0.65rem;
  color: rgba(186, 206, 226, 0.4);
  margin-top: 0.15rem;
}

.narrative-panel__scope-switcher {
  display: flex;
  gap: 0.25rem;
  padding: 0.2rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.narrative-panel__scope-btn {
  flex: 1;
  padding: 0.3rem 0.9rem;
  border-radius: 999px;
  border: none;
  background: none;
  color: rgba(186, 206, 226, 0.55);
  font-size: 0.75rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.narrative-panel__scope-btn--active {
  background: rgba(255, 255, 255, 0.08);
  color: rgba(241, 247, 252, 0.9);
}

.narrative-panel__scope-btn:hover:not(.narrative-panel__scope-btn--active) {
  color: rgba(186, 206, 226, 0.8);
}

.narrative-panel__back {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
  padding: 0.25rem 0.5rem;
  border: none;
  background: none;
  color: rgba(186, 206, 226, 0.55);
  font-size: 0.75rem;
  cursor: pointer;
  transition: color 0.15s ease;
}

.narrative-panel__back:hover {
  color: rgba(241, 247, 252, 0.9);
}

.narrative-panel__cat-list {
  display: grid;
  gap: 0.5rem;
}

.narrative-panel__cat-card {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.65rem 0.85rem;
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(255, 255, 255, 0.02);
  cursor: pointer;
  transition: all 0.15s ease;
  text-align: left;
}

.narrative-panel__cat-card:hover {
  border-color: rgba(255, 255, 255, 0.15);
  background: rgba(255, 255, 255, 0.04);
}

.narrative-panel__cat-icon {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.narrative-panel__cat-info {
  flex: 1;
  min-width: 0;
}

.narrative-panel__cat-name {
  font-size: 0.82rem;
  color: rgba(241, 247, 252, 0.85);
}

.narrative-panel__cat-badge {
  padding: 0.15rem 0.5rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
  color: rgba(186, 206, 226, 0.6);
  font-size: 0.7rem;
  font-variant-numeric: tabular-nums;
}

.narrative-panel__cat-arrow {
  color: rgba(186, 206, 226, 0.3);
  flex-shrink: 0;
}

.detail-slide-enter-active,
.detail-slide-leave-active {
  transition: all 0.22s cubic-bezier(0.22, 1, 0.36, 1);
}

.detail-slide-enter-from,
.detail-slide-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>
