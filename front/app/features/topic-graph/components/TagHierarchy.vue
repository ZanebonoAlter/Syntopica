<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { computed, onMounted, ref, watch } from 'vue'
import { useAbstractTagApi } from '~/api/abstractTags'
import { useWatchedTagsApi } from '~/api/watchedTags'
import type { TopicCategory } from '~/api/topicGraph'
import { useOrganizeWebSocket } from '~/features/topic-graph/composables/useOrganizeWebSocket'
import TagHierarchyRow from '~/features/topic-graph/components/TagHierarchyRow.vue'
import type { TagHierarchyNode } from '~/types/topicTag'

const props = defineProps<{
  feedId?: string | null
  categoryId?: string | null
  anchorDate?: string
  selectable?: boolean
  category?: string
  sectorId?: number | null
  hideCategoryTabs?: boolean
}>()

const emit = defineEmits<{
  'select-tag': [slug: string, category: TopicCategory]
}>()

const abstractTagApi = useAbstractTagApi()
const watchedTagsApi = useWatchedTagsApi()

const nodes = ref<TagHierarchyNode[]>([])
const loading = ref(true)
const error = ref<string | null>(null)
const selectedCategory = ref<string>(props.category ?? '')
const showUnclassified = ref(false)
const unplacedTags = ref<TagHierarchyNode[]>([])
const showUnplaced = ref(false)
const searchQuery = ref('')
const initialTimeRange = props.anchorDate ? `custom:${props.anchorDate}:${props.anchorDate}` : ''
const timeRange = ref<string>(initialTimeRange)
const customStartDate = ref(props.anchorDate ?? '')
const customEndDate = ref(props.anchorDate ?? '')
const showCustomRange = ref(false)

watch(() => props.anchorDate, (newDate) => {
  if (newDate) {
    timeRange.value = `custom:${newDate}:${newDate}`
    customStartDate.value = newDate
    customEndDate.value = newDate
  }
})

const watchedTagIds = ref<Set<number>>(new Set())

// Inline editing state
const editingNodeId = ref<number | null>(null)
const editingValue = ref('')
const saving = ref(false)

// Detach confirm state
const confirmingDetach = ref<{ parentId: number; childId: number; childLabel: string } | null>(null)

// Reassign modal state
const reassignModal = ref<{ tagId: number; tagLabel: string } | null>(null)
const reassignCandidates = ref<TagHierarchyNode[]>([])
const reassignLoading = ref(false)
const reassignError = ref<string | null>(null)

const organizing = ref(false)
const organizeResult = ref<{ total_unclassified: number; processed: number } | null>(null)
const organizeWs = useOrganizeWebSocket()

const tagCategories = ['event', 'person', 'keyword'] as const

const categoryLabel = (cat: string) =>
  cat === 'event' ? '事件' : cat === 'person' ? '人物' : '关键词'

function normalizeHierarchyCategory(category: string): TopicCategory {
  if (category === 'event' || category === 'person') return category
  return 'keyword'
}

function matchesSearch(label: string): boolean {
  if (!searchQuery.value) return true
  const q = searchQuery.value.toLowerCase()
  return label.toLowerCase().includes(q)
}

function filterTree(list: TagHierarchyNode[], filterInactive: boolean): TagHierarchyNode[] {
  const result: TagHierarchyNode[] = []
  for (const node of list) {
    if (filterInactive && !node.isActive && node.children.length === 0) continue
    const childMatch = filterTree(node.children, filterInactive)
    const matchesSearchOK = matchesSearch(node.label)
    const hasVisibleChildren = childMatch.length > 0
    if (filterInactive && !node.isActive && !hasVisibleChildren) continue
    if (!matchesSearchOK && !hasVisibleChildren) continue
    result.push({ ...node, children: childMatch })
  }
  return result
}

const filteredNodes = computed(() => {
  const filterInactive = timeRange.value !== ''
  return filterTree(nodes.value, filterInactive)
})

function hasWatchedDescendant(node: TagHierarchyNode): boolean {
  if (watchedTagIds.value.has(node.id)) return true
  return node.children.some(c => hasWatchedDescendant(c))
}

function sortNodesByActivity(list: TagHierarchyNode[]): TagHierarchyNode[] {
  return [...list].sort((a, b) => {
    const aWatched = hasWatchedDescendant(a)
    const bWatched = hasWatchedDescendant(b)
    if (aWatched !== bWatched) return aWatched ? -1 : 1
    const aActive = a.isActive !== false
    const bActive = b.isActive !== false
    if (aActive !== bActive) return aActive ? -1 : 1
    const aScore = a.qualityScore ?? 0
    const bScore = b.qualityScore ?? 0
    if (bScore !== aScore) return bScore - aScore
    return (b.feedCount ?? 0) - (a.feedCount ?? 0)
  }).map(node => ({
    ...node,
    children: sortNodesByActivity(node.children),
  }))
}

const sortedNodes = computed(() => sortNodesByActivity(filteredNodes.value))

const totalCount = computed(() => {
  let count = 0
  function walk(list: TagHierarchyNode[]) {
    for (const node of list) {
      count++
      if (node.children.length > 0) walk(node.children)
    }
  }
  walk(sortedNodes.value)
  return count
})

async function loadHierarchy() {
  loading.value = true
  error.value = null
  try {
    const apiCategory = selectedCategory.value || undefined
    const conceptId = props.sectorId ?? undefined
    const response = await abstractTagApi.fetchHierarchy(
      apiCategory,
      props.feedId || undefined,
      props.categoryId || undefined,
      showUnclassified.value,
      timeRange.value || undefined,
      conceptId,
    )
    if (response.success && response.data) {
      nodes.value = response.data.nodes ?? []
    } else {
      error.value = response.error || '加载层级数据失败'
    }
    const unplacedRes = await abstractTagApi.fetchHierarchy(
      apiCategory,
      props.feedId || undefined,
      props.categoryId || undefined,
      true,
      timeRange.value || undefined,
    )
    if (unplacedRes.success && unplacedRes.data) {
      const placedIds = new Set<number>()
      function collectPlaced(list: TagHierarchyNode[]) {
        for (const node of list) {
          placedIds.add(node.id)
          collectPlaced(node.children)
        }
      }
      collectPlaced(nodes.value)
      unplacedTags.value = (unplacedRes.data.nodes ?? []).filter(n => !placedIds.has(n.id))
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function loadWatchedTags() {
  try {
    const res = await watchedTagsApi.listWatchedTags()
    if (res.success && res.data) {
      watchedTagIds.value = new Set((res.data as any[]).map((t: any) => t.id))
    }
  } catch (e) {
    console.error('Failed to load watched tags:', e)
  }
}

async function toggleWatch(node: TagHierarchyNode) {
  const wasWatched = watchedTagIds.value.has(node.id)
  if (wasWatched) {
    watchedTagIds.value.delete(node.id)
    watchedTagIds.value = new Set(watchedTagIds.value)
    const res = await watchedTagsApi.unwatchTag(node.id)
    if (!res.success) {
      watchedTagIds.value = new Set([...watchedTagIds.value, node.id])
      console.error('Failed to unwatch tag:', res.error)
    }
  } else {
    watchedTagIds.value = new Set([...watchedTagIds.value, node.id])
    const res = await watchedTagsApi.watchTag(node.id)
    if (!res.success) {
      watchedTagIds.value.delete(node.id)
      watchedTagIds.value = new Set(watchedTagIds.value)
      console.error('Failed to watch tag:', res.error)
    }
  }
}

function startEdit(node: TagHierarchyNode) {
  editingNodeId.value = node.id
  editingValue.value = node.label
}

function cancelEdit() {
  editingNodeId.value = null
  editingValue.value = ''
}

async function confirmEdit() {
  if (!editingNodeId.value) return
  const name = editingValue.value.trim()
  if (!name || name.length > 160) return

  saving.value = true
  try {
    const response = await abstractTagApi.updateAbstractName(editingNodeId.value, name)
    if (response.success) {
      await loadHierarchy()
    } else {
      error.value = response.error || '更新失败'
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : '更新失败'
  } finally {
    saving.value = false
    editingNodeId.value = null
    editingValue.value = ''
  }
}

function requestDetach(node: TagHierarchyNode) {
  // Find parent ID from the tree
  const parentId = findParentId(nodes.value, node.id)
  if (parentId) {
    confirmingDetach.value = { parentId, childId: node.id, childLabel: node.label }
  }
}

function collectAbstractTags(tree: TagHierarchyNode[], excludeId: number): TagHierarchyNode[] {
  const result: TagHierarchyNode[] = []
  for (const node of tree) {
    if (node.id === excludeId) continue
    if (node.children.length > 0) {
      result.push(node)
    }
    result.push(...collectAbstractTags(node.children, excludeId))
  }
  return result
}

function requestReassign(node: TagHierarchyNode) {
  reassignCandidates.value = collectAbstractTags(nodes.value, node.id)
  reassignModal.value = { tagId: node.id, tagLabel: node.label }
  reassignError.value = null
}

function cancelReassign() {
  reassignModal.value = null
  reassignError.value = null
}

async function confirmReassign(newParentId: number) {
  if (!reassignModal.value) return
  reassignLoading.value = true
  reassignError.value = null
  try {
    const response = await abstractTagApi.reassignTag(reassignModal.value.tagId, newParentId)
    if (response.success) {
      reassignModal.value = null
      await loadHierarchy()
    } else {
      reassignError.value = response.error || '归类失败'
    }
  } catch (e) {
    reassignError.value = e instanceof Error ? e.message : '归类失败'
  } finally {
    reassignLoading.value = false
  }
}

function applyCustomRange() {
  if (customStartDate.value && customEndDate.value) {
    timeRange.value = `custom:${customStartDate.value}:${customEndDate.value}`
  }
}

function findParentId(tree: TagHierarchyNode[], targetId: number, parentId: number | null = null): number | null {
  for (const node of tree) {
    if (node.id === targetId) return parentId
    if (node.children.length > 0) {
      const found = findParentId(node.children, targetId, node.id)
      if (found !== null) return found
    }
  }
  return null
}

function cancelDetach() {
  confirmingDetach.value = null
}

async function confirmDetach() {
  if (!confirmingDetach.value) return
  const { parentId, childId } = confirmingDetach.value
  try {
    const response = await abstractTagApi.detachChild(parentId, childId)
    if (response.success) {
      await loadHierarchy()
    } else {
      error.value = response.error || '分离失败'
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : '分离失败'
  } finally {
    confirmingDetach.value = null
  }
}

function handleUpdateEditingValue(val: string) {
  editingValue.value = val
}

function handleSelectNode(node: TagHierarchyNode) {
  if (props.selectable === false) return
  emit('select-tag', node.slug, normalizeHierarchyCategory(node.category))
}

async function organizeUnclassified() {
  organizing.value = true
  organizeResult.value = null
  error.value = null
  organizeWs.reset()
  try {
    const apiCategory = selectedCategory.value || undefined
    const response = await abstractTagApi.organizeTags(apiCategory)
    if (!response.success) {
      error.value = response.error || '整理失败'
      organizing.value = false
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : '整理失败'
    organizing.value = false
  }
}

watch(() => organizeWs.status.value, (newStatus) => {
  if (newStatus === 'completed') {
    organizing.value = false
    organizeResult.value = {
      total_unclassified: organizeWs.totalUnclassified.value,
      processed: organizeWs.processed.value,
    }
    if (organizeWs.processed.value > 0) {
      showUnclassified.value = true
      void loadHierarchy()
    }
  }
})

onMounted(() => {
  void loadHierarchy()
  void loadWatchedTags()
})

watch(() => [props.feedId, props.categoryId] as const, () => {
  void loadHierarchy()
})

watch(() => props.category, (newCat) => {
  if (props.hideCategoryTabs) {
    if (newCat !== selectedCategory.value) {
      selectedCategory.value = newCat ?? ''
      void loadHierarchy()
    }
  } else if (newCat !== undefined && newCat !== selectedCategory.value) {
    selectedCategory.value = newCat
    void loadHierarchy()
  }
})

watch(timeRange, (newVal) => {
  if (!newVal.startsWith('custom:')) {
    showCustomRange.value = false
  }
  void loadHierarchy()
})

watch(() => props.sectorId, () => {
  void loadHierarchy()
})
</script>

<template>
  <div class="tag-hierarchy">
    <!-- Header with category filter -->
    <div class="flex items-center justify-between gap-3 mb-2">
      <div class="flex items-center gap-2">
        <div>
          <h3 class="font-serif text-lg text-white">标签层级</h3>
          <p class="text-xs text-white/50 mt-0.5">
            {{ totalCount }} 个标签
          </p>
        </div>
      </div>
      <div class="flex items-center gap-2 flex-shrink-0">
        <button
          type="button"
          class="th-category-btn th-organize-btn"
          :disabled="organizing"
          @click="void organizeUnclassified()"
        >
          <Icon :icon="organizing ? 'mdi:loading' : 'mdi:auto-fix'" width="12" class="mr-1" :class="{ 'animate-spin': organizing }" />
          {{ organizing ? '整理中...' : '整理标签' }}
        </button>
        <div v-if="organizing && organizeWs.totalUnclassified.value > 0" class="th-organize-result">
          <Icon icon="mdi:loading" width="12" class="animate-spin text-blue-400" />
          <span>{{ organizeWs.processed.value }} / {{ organizeWs.totalUnclassified.value }}</span>
        </div>
        <div v-else-if="organizeResult && !organizing" class="th-organize-result">
          <Icon icon="mdi:check-circle-outline" width="12" class="text-green-400" />
          <span>{{ organizeResult.processed }} 组已归类</span>
        </div>
      </div>
    </div>

    <!-- Category tabs + search -->
    <div class="flex items-center gap-2 mb-3">
      <div v-if="!hideCategoryTabs" class="flex gap-1.5 flex-shrink-0">
        <button
          type="button"
          class="th-category-btn"
          :class="{ 'th-category-btn--active': !selectedCategory }"
          @click="selectedCategory = ''; void loadHierarchy()"
        >
          全部
        </button>
        <button
          v-for="cat in tagCategories"
          :key="cat"
          type="button"
          class="th-category-btn"
          :class="{ 'th-category-btn--active': selectedCategory === cat }"
          @click="selectedCategory = cat; void loadHierarchy()"
        >
          {{ categoryLabel(cat) }}
        </button>
      </div>
      <div class="th-search-wrap flex-1 min-w-0">
        <Icon icon="mdi:magnify" width="14" class="th-search-icon" />
        <input
          v-model="searchQuery"
          type="text"
          class="th-search-input"
          placeholder="搜索标签..."
        />
        <button
          v-if="searchQuery"
          type="button"
          class="th-search-clear"
          @click="searchQuery = ''"
        >
          <Icon icon="mdi:close" width="12" />
        </button>
      </div>
    </div>

    <!-- Time filter -->
    <div class="flex items-center gap-1.5 mb-3 flex-wrap">
      <span class="text-xs text-white/40 mr-0.5">时间</span>
      <button
        type="button"
        class="th-category-btn"
        :class="{ 'th-category-btn--active': timeRange === '' }"
        @click="timeRange = ''"
      >
        全部
      </button>
      <button
        type="button"
        class="th-category-btn"
        :class="{ 'th-category-btn--active': timeRange === '1d' }"
        @click="timeRange = '1d'"
      >
        今天
      </button>
      <button
        type="button"
        class="th-category-btn"
        :class="{ 'th-category-btn--active': timeRange === '7d' }"
        @click="timeRange = '7d'"
      >
        7天
      </button>
      <button
        type="button"
        class="th-category-btn"
        :class="{ 'th-category-btn--active': timeRange === '30d' }"
        @click="timeRange = '30d'"
      >
        30天
      </button>
      <button
        type="button"
        class="th-category-btn"
        :class="{ 'th-category-btn--active': showCustomRange }"
        @click="showCustomRange = !showCustomRange"
      >
        <Icon icon="mdi:calendar-range" width="12" class="mr-1" />
        自定义
      </button>
    </div>
    <div v-if="showCustomRange" class="flex items-center gap-2 mb-3 th-custom-range">
      <input v-model="customStartDate" type="date" class="th-date-input" />
      <span class="text-white/30 text-xs">至</span>
      <input v-model="customEndDate" type="date" class="th-date-input" />
      <button type="button" class="th-category-btn th-category-btn--active" @click="applyCustomRange">确定</button>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="space-y-3">
      <div v-for="i in 3" :key="i" class="th-skeleton" />
    </div>

    <!-- Error -->
    <div v-else-if="error" class="th-error">
      <Icon icon="mdi:alert-circle-outline" width="16" />
      <span>{{ error }}</span>
    </div>

    <!-- Empty -->
    <div v-else-if="sortedNodes.length === 0 && props.sectorId" class="th-empty">
      <Icon icon="mdi:filter-variant-remove" width="32" class="text-white/20" />
      <p>该板块暂无标签</p>
      <p class="text-xs text-white/30 mt-1">此板块下没有匹配的标签层级关系</p>
    </div>

    <!-- Empty: no data -->
    <div v-else-if="sortedNodes.length === 0" class="th-empty">
      <Icon icon="mdi:file-tree-outline" width="32" class="text-white/20" />
      <p>暂无标签层级关系</p>
      <p class="text-xs text-white/30 mt-1">当新文章入库时，中间相似度标签将自动提取抽象概念</p>
    </div>

    <!-- Tree -->
    <div v-else class="th-tree">
      <TagHierarchyRow
        v-for="node in sortedNodes"
        :key="node.id"
        :node="node"
        :depth="0"
        :editing-id="editingNodeId"
        :saving="saving"
        :watched-tag-ids="watchedTagIds"
        @start-edit="startEdit"
        @cancel-edit="cancelEdit"
        @confirm-edit="void confirmEdit()"
        @detach="requestDetach"
        @reassign="requestReassign"
        @select="handleSelectNode"
        @update:editing-value="handleUpdateEditingValue"
        @toggle-watch="toggleWatch"
      />
    </div>

    <!-- Unplaced tags section -->
    <div v-if="unplacedTags.length > 0" class="th-unplaced-section">
      <button
        type="button"
        class="th-unplaced-toggle"
        @click="showUnplaced = !showUnplaced"
      >
        <Icon :icon="showUnplaced ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="16" />
        <span class="th-unplaced-label">未归属</span>
        <span class="th-unplaced-count">{{ unplacedTags.length }} 个标签</span>
      </button>
      <div v-if="showUnplaced" class="th-unplaced-list">
        <TagHierarchyRow
          v-for="tag in unplacedTags"
          :key="tag.id"
          :node="tag"
          :depth="1"
          :editing-id="editingNodeId"
          :saving="saving"
          :watched-tag-ids="watchedTagIds"
          @start-edit="startEdit"
          @cancel-edit="cancelEdit"
          @confirm-edit="void confirmEdit()"
          @detach="requestDetach"
          @reassign="requestReassign"
          @select="handleSelectNode"
          @update:editing-value="handleUpdateEditingValue"
          @toggle-watch="toggleWatch"
        />
      </div>
    </div>

    <!-- Detach confirm dialog -->
    <Teleport to="body">
      <div v-if="confirmingDetach" class="th-confirm-overlay" @click.self="cancelDetach">
        <div class="th-confirm-dialog">
          <p class="text-sm text-white/80">确认将 <strong class="text-white">{{ confirmingDetach.childLabel }}</strong> 从抽象标签中分离？</p>
          <div class="flex gap-2 mt-4 justify-end">
            <button type="button" class="th-btn th-btn--ghost" @click="cancelDetach">取消</button>
            <button type="button" class="th-btn th-btn--danger" @click="void confirmDetach()">分离</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Reassign modal -->
    <Teleport to="body">
      <div v-if="reassignModal" class="th-confirm-overlay" @click.self="cancelReassign">
        <div class="th-confirm-dialog" style="width: min(480px, 90%);">
          <p class="text-sm text-white/60 mb-1">归类标签</p>
          <p class="text-base text-white font-medium mb-3">{{ reassignModal.tagLabel }}</p>

          <div v-if="reassignError" class="th-error mb-3">
            <Icon icon="mdi:alert-circle-outline" width="14" />
            <span>{{ reassignError }}</span>
          </div>

          <p class="text-xs text-white/40 mb-2">选择目标抽象标签：</p>

          <div v-if="reassignCandidates.length === 0" class="text-xs text-white/30 py-4 text-center">
            暂无可选的抽象标签
          </div>

          <div v-else class="th-reassign-list">
            <button
              v-for="candidate in reassignCandidates"
              :key="candidate.id"
              type="button"
              class="th-reassign-item"
              :disabled="reassignLoading"
              @click="void confirmReassign(candidate.id)"
            >
              <span class="th-reassign-item-label">{{ candidate.label }}</span>
              <span class="th-reassign-item-cat">{{ candidate.category === 'event' ? '事件' : candidate.category === 'person' ? '人物' : '关键词' }}</span>
              <span v-if="candidate.feedCount > 0" class="th-reassign-item-count">{{ candidate.feedCount }}</span>
            </button>
          </div>

          <div class="flex gap-2 mt-4 justify-end">
            <button type="button" class="th-btn th-btn--ghost" @click="cancelReassign">取消</button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.tag-hierarchy { min-height: 200px; }

.th-skeleton {
  height: 36px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.04);
  animation: pulse 1.5s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

.th-error {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.75rem 1rem;
  border-radius: 14px;
  border: 1px solid rgba(240, 138, 75, 0.3);
  background: rgba(240, 138, 75, 0.1);
  color: rgba(255, 220, 200, 0.88);
  font-size: 0.82rem;
}

.th-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  padding: 3rem 1rem;
  color: rgba(255, 255, 255, 0.4);
  font-size: 0.85rem;
}

.th-tree { display: flex; flex-direction: column; gap: 1px; }

.th-category-btn {
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 999px;
  background: none;
  color: rgba(255, 255, 255, 0.5);
  font-size: 0.7rem;
  padding: 0.2rem 0.65rem;
  cursor: pointer;
  transition: all 0.12s ease;
}
.th-category-btn:hover { border-color: rgba(255, 255, 255, 0.25); color: rgba(255, 255, 255, 0.8); }
.th-category-btn--active { border-color: rgba(240, 138, 75, 0.5); background: rgba(240, 138, 75, 0.12); color: rgba(255, 220, 200, 0.9); }

.th-organize-btn {
  border-color: rgba(99, 179, 237, 0.3);
  color: rgba(147, 197, 253, 0.8);
}
.th-organize-btn:hover:not(:disabled) {
  border-color: rgba(99, 179, 237, 0.5);
  background: rgba(99, 179, 237, 0.1);
  color: rgba(191, 219, 254, 0.95);
}
.th-organize-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.th-organize-result {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.68rem;
  color: rgba(134, 239, 172, 0.8);
  animation: fadeIn 0.3s ease;
}

@keyframes fadeIn {
  from { opacity: 0; transform: translateY(-4px); }
  to { opacity: 1; transform: translateY(0); }
}

.th-search-wrap {
  position: relative;
  display: flex;
  align-items: center;
}
.th-search-icon {
  position: absolute;
  left: 8px;
  color: rgba(255, 255, 255, 0.3);
  pointer-events: none;
}
.th-search-input {
  width: 100%;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.85);
  font-size: 0.72rem;
  padding: 0.25rem 1.5rem 0.25rem 1.75rem;
  outline: none;
  transition: border-color 0.12s ease;
}
.th-search-input::placeholder { color: rgba(255, 255, 255, 0.25); }
.th-search-input:focus { border-color: rgba(240, 138, 75, 0.4); }
.th-search-clear {
  position: absolute;
  right: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border: none;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.4);
  cursor: pointer;
}
.th-search-clear:hover { background: rgba(255, 255, 255, 0.15); color: rgba(255, 255, 255, 0.7); }

.th-date-input {
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.85);
  font-size: 0.72rem;
  padding: 0.25rem 0.5rem;
  outline: none;
}
.th-date-input:focus { border-color: rgba(240, 138, 75, 0.4); }
.th-custom-range { margin-top: -0.25rem; }

.th-confirm-overlay {
  position: fixed;
  inset: 0;
  z-index: 90;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(8, 12, 18, 0.7);
  backdrop-filter: blur(8px);
}

.th-confirm-dialog {
  width: min(400px, 90%);
  border-radius: 1.25rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(17, 27, 38, 0.98);
  padding: 1.5rem;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.4);
}

.th-btn {
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  background: none;
  color: rgba(255, 255, 255, 0.7);
  font-size: 0.82rem;
  padding: 0.4rem 1rem;
  cursor: pointer;
  transition: all 0.12s ease;
}
.th-btn--ghost:hover { background: rgba(255, 255, 255, 0.06); }
.th-btn--danger { border-color: rgba(240, 138, 75, 0.4); color: rgba(255, 200, 180, 0.9); }
.th-btn--danger:hover { background: rgba(240, 138, 75, 0.15); }

.th-reassign-list {
  max-height: 280px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.th-reassign-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.02);
  color: rgba(255, 255, 255, 0.75);
  font-size: 0.82rem;
  cursor: pointer;
  text-align: left;
  transition: all 0.12s ease;
}
.th-reassign-item:hover {
  border-color: rgba(99, 102, 241, 0.4);
  background: rgba(99, 102, 241, 0.08);
  color: white;
}
.th-reassign-item:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.th-reassign-item-label { flex: 1; }
.th-reassign-item-cat {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.35);
  padding: 0.1rem 0.4rem;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.08);
}
.th-reassign-item-count {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
}

.th-unplaced-section {
  margin-top: 1rem;
  border-top: 1px dashed rgba(255, 255, 255, 0.1);
  padding-top: 0.5rem;
}
.th-unplaced-toggle {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  width: 100%;
  padding: 0.5rem 0.75rem;
  border: none;
  border-radius: 10px;
  background: none;
  color: rgba(255, 255, 255, 0.5);
  font-size: 0.8rem;
  cursor: pointer;
  transition: background 0.2s;
}
.th-unplaced-toggle:hover { background: rgba(255, 255, 255, 0.04); }
.th-unplaced-label { font-weight: 500; color: rgba(255, 255, 255, 0.7); }
.th-unplaced-count { font-size: 0.75rem; }
.th-unplaced-list { padding-left: 0.5rem; }
</style>
