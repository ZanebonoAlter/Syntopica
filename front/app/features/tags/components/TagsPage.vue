<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { Icon } from '@iconify/vue'
import { useBoardConceptsApi } from '~/api/boardConcepts'
import { useHierarchyConfigApi } from '~/api/hierarchyConfig'
import { useArticlesApi } from '~/api/articles'
import { useWebSocketRebuild } from '~/composables/useWebSocketRebuild'
import type { SectorItem, SectorDiff } from '~/api/boardConcepts'
import type { Article } from '~/types'
import TagHierarchy from '~/features/topic-graph/components/TagHierarchy.vue'
import SectorList from '~/features/tags/components/SectorList.vue'
import AddSectorDialog from '~/features/tags/components/AddSectorDialog.vue'
import SectorApprovalPanel from '~/features/tags/components/SectorApprovalPanel.vue'
import TemplateSettingsDialog from '~/features/tags/components/TemplateSettingsDialog.vue'
import PendingChangePanel from '~/features/tags/components/PendingChangePanel.vue'
import type { HierarchyClosureStatus } from '~/api/hierarchyConfig'

const api = useBoardConceptsApi()
const hierarchyApi = useHierarchyConfigApi()
const articlesApi = useArticlesApi()
const { status: rebuildStatus, total: rebuildTotal, processed: rebuildProcessed, currentTag, errorMessage: rebuildError, reset: resetRebuild } = useWebSocketRebuild()

const categories = [
  { key: 'event', label: '事件' },
  { key: 'person', label: '人物' },
  { key: 'keyword', label: '关键词' },
] as const

const selectedCategory = ref('event')
const sectors = ref<SectorItem[]>([])
const selectedSectorId = ref<number | null>(null)
const sectorsLoading = ref(false)
const sectorsError = ref<string | null>(null)
const hierarchyRefreshKey = ref(0)
const closureStatus = ref<HierarchyClosureStatus | null>(null)
const closureError = ref<string | null>(null)

const showAddDialog = ref(false)
const showRegenerateDialog = ref(false)
const showTemplateDialog = ref(false)
const showPendingPanel = ref(false)
const regenerateLoading = ref(false)
const regenerateError = ref<string | null>(null)
const sectorDiff = ref<SectorDiff | null>(null)

const pendingCount = ref(0)
const rebuildNotification = ref<'completed' | 'failed' | null>(null)
let pendingTimer: ReturnType<typeof setInterval> | null = null
let rebuildDismissTimer: ReturnType<typeof setTimeout> | null = null

const timelineArticles = ref<Article[]>([])
const timelineLoading = ref(false)
const timelineVisible = computed(() => selectedSectorId.value !== null)

const rebuildProgressPercent = computed(() => {
  if (rebuildTotal.value === 0) return 0
  return Math.round((rebuildProcessed.value / rebuildTotal.value) * 100)
})

const rebuildBarVisible = computed(() => {
  return rebuildStatus.value === 'processing' || rebuildNotification.value !== null
})

const closureBlockerEntries = computed(() => {
  if (!closureStatus.value) return []
  return Object.entries(closureStatus.value.blocker_counts || {})
    .filter(([, count]) => count > 0)
    .map(([key, count]) => ({ key, label: blockerLabel(key), count }))
})

async function loadSectors() {
  sectorsLoading.value = true
  sectorsError.value = null
  const res = await api.getSectors(selectedCategory.value)
  if (res.success && res.data) {
    sectors.value = res.data
  } else {
    sectorsError.value = res.error || '加载板块失败'
  }
  sectorsLoading.value = false
}

async function loadPendingCount() {
  const res = await hierarchyApi.getPending('pending')
  if (res.success && res.data) {
    pendingCount.value = res.data.length
  }
}

async function loadClosureStatus() {
  closureError.value = null
  const res = await hierarchyApi.getClosureStatus(selectedCategory.value)
  if (res.success && res.data) {
    closureStatus.value = res.data
  } else {
    closureError.value = res.error || '加载闭环状态失败'
  }
}

function blockerLabel(blocker?: string): string {
  switch (blocker) {
    case 'rebuild_running': return '重建运行中'
    case 'no_active_sector': return '缺少板块'
    case 'pending_changes': return '存在待确认变更'
    case 'no_matching_sector': return '标签未匹配板块'
    default: return '可继续闭环'
  }
}

function handleSelectSector(id: number | null) {
  selectedSectorId.value = id
  if (id !== null) {
    void loadTimelineArticles(id)
  } else {
    timelineArticles.value = []
  }
}

async function loadTimelineArticles(sectorId: number) {
  timelineLoading.value = true
  try {
    const res = await articlesApi.getArticles({
      concept_id: sectorId,
      per_page: 50,
      sort_by: 'date',
    })
    if (res.success && res.data) {
      timelineArticles.value = (res.data.items || []) as Article[]
    } else {
      timelineArticles.value = []
    }
  } catch (e) {
    console.error('Failed to load timeline articles:', e)
    timelineArticles.value = []
  } finally {
    timelineLoading.value = false
  }
}

async function refreshSectorState() {
  await loadSectors()
  hierarchyRefreshKey.value++
  await loadPendingCount()
  await loadClosureStatus()
  if (selectedSectorId.value !== null) {
    await loadTimelineArticles(selectedSectorId.value)
  }
}

async function handleAddSector(data: { label: string; description: string }) {
  const res = await api.createSector({
    name: data.label,
    description: data.description || undefined,
    category: selectedCategory.value,
    source: 'manual',
  })
  if (res.success) {
    showAddDialog.value = false
    await nextTick()
    await refreshSectorState()
  } else {
    sectorsError.value = res.error || '添加板块失败'
  }
}

async function handleRegenerate() {
  regenerateLoading.value = true
  regenerateError.value = null
  const res = await api.regenerateSectors(selectedCategory.value)
  if (res.success && res.data) {
    sectorDiff.value = res.data
    showRegenerateDialog.value = true
  } else {
    regenerateError.value = res.error || 'LLM 重新生成失败'
  }
  regenerateLoading.value = false
}

async function handleConfirmDone() {
  showRegenerateDialog.value = false
  sectorDiff.value = null
  await refreshSectorState()
}

async function handleTemplateSaved() {
  showTemplateDialog.value = false
  resetRebuild()
  await refreshSectorState()
}

async function handleDeleteSector(id: number) {
  const sector = sectors.value.find(s => s.id === id)
  if (!sector) return
  if (sector.protected) {
    if (!confirm('此板块受保护，确认删除？')) return
    const res = await api.deleteSector(id, true)
    if (res.success) {
      if (selectedSectorId.value === id) selectedSectorId.value = null
      await refreshSectorState()
    } else {
      sectorsError.value = res.error || '删除失败'
    }
  } else {
    if (!confirm('确认删除此板块？')) return
    const res = await api.deleteSector(id)
    if (res.success) {
      if (selectedSectorId.value === id) selectedSectorId.value = null
      await refreshSectorState()
    } else {
      sectorsError.value = res.error || '删除失败'
    }
  }
}

function dismissRebuildNotification() {
  rebuildNotification.value = null
  if (rebuildDismissTimer) {
    clearTimeout(rebuildDismissTimer)
    rebuildDismissTimer = null
  }
}

watch(rebuildStatus, (newStatus) => {
  if (newStatus === 'completed') {
    rebuildNotification.value = 'completed'
    if (rebuildDismissTimer) clearTimeout(rebuildDismissTimer)
    rebuildDismissTimer = setTimeout(() => {
      rebuildNotification.value = null
    }, 30000)
    void loadPendingCount()
    void refreshSectorState()
  } else if (newStatus === 'failed') {
    rebuildNotification.value = 'failed'
    if (rebuildDismissTimer) clearTimeout(rebuildDismissTimer)
    rebuildDismissTimer = setTimeout(() => {
      rebuildNotification.value = null
    }, 30000)
  }
})

watch(selectedCategory, () => {
  selectedSectorId.value = null
  void loadSectors()
  void loadClosureStatus()
})

onMounted(() => {
  void loadSectors()
  void loadPendingCount()
  void loadClosureStatus()
  pendingTimer = setInterval(() => {
    void loadPendingCount()
  }, 30000)
})

onUnmounted(() => {
  if (pendingTimer) clearInterval(pendingTimer)
  if (rebuildDismissTimer) clearTimeout(rebuildDismissTimer)
})
</script>

<template>
  <div class="tags-page">
    <!-- Top bar -->
    <div class="tags-topbar">
      <div class="tags-topbar-inner">
        <div class="flex items-center gap-3">
          <NuxtLink to="/" class="tags-back-btn" title="返回首页">
            <Icon icon="mdi:arrow-left" width="16" />
          </NuxtLink>
          <Icon icon="mdi:tag-multiple" width="18" class="text-white/50" />
          <h1 class="tags-page-title">标签管理</h1>
        </div>
        <div class="flex items-center gap-3">
          <div class="tags-category-tabs">
            <button
              v-for="cat in categories"
              :key="cat.key"
              type="button"
              class="tags-cat-btn"
              :class="{ 'tags-cat-btn--active': selectedCategory === cat.key }"
              @click="selectedCategory = cat.key"
            >
              {{ cat.label }}
            </button>
          </div>
          <button
            type="button"
            class="tags-settings-btn"
            title="模板设置"
            @click="showTemplateDialog = true"
          >
            <Icon icon="mdi:cog-outline" width="16" />
          </button>
        </div>
      </div>
    </div>

    <!-- Main content -->
    <div class="tags-main">
      <!-- Left panel: Sector list -->
      <aside class="tags-sidebar">
        <div
          v-if="sectorsError"
          class="tags-sidebar-error"
        >
          <Icon icon="mdi:alert-circle-outline" width="14" />
          <span>{{ sectorsError }}</span>
        </div>
        <div v-if="regenerateError" class="tags-sidebar-error">
          <Icon icon="mdi:alert-circle-outline" width="14" />
          <span>{{ regenerateError }}</span>
        </div>
        <SectorList
          :sectors="sectors"
          :selected-id="selectedSectorId"
          :loading="sectorsLoading"
          @select="handleSelectSector"
          @add="showAddDialog = true"
          @regenerate="handleRegenerate"
          @delete="handleDeleteSector"
        />
        <div v-if="closureStatus" class="tags-closure-card">
          <div class="tags-closure-title">
            <Icon icon="mdi:source-branch-check" width="14" />
            <span>层级闭环</span>
          </div>
          <div class="tags-closure-grid">
            <span>板块</span><strong>{{ closureStatus.active_sector_count }}</strong>
            <span>未归属</span><strong>{{ closureStatus.unplaced_tag_count }}</strong>
            <span>待确认</span><strong>{{ closureStatus.pending_change_count }}</strong>
          </div>
          <div class="tags-closure-blocker" :class="{ 'tags-closure-blocker--ok': !closureStatus.top_blocker }">
            {{ blockerLabel(closureStatus.top_blocker) }}
          </div>
          <div v-if="closureBlockerEntries.length > 0" class="tags-closure-blockers">
            <span
              v-for="item in closureBlockerEntries"
              :key="item.key"
              class="tags-closure-blocker-pill"
            >{{ item.label }} {{ item.count }}</span>
          </div>
        </div>
        <div v-else-if="closureError" class="tags-sidebar-error">
          <Icon icon="mdi:alert-circle-outline" width="14" />
          <span>{{ closureError }}</span>
        </div>
      </aside>

      <!-- Right panel: Hierarchy tree and timeline -->
      <main class="tags-content">
        <TagHierarchy :key="hierarchyRefreshKey" :selectable="false" :category="selectedCategory" :sector-id="selectedSectorId" :hide-category-tabs="true" />

        <!-- Sector article timeline -->
        <div v-if="timelineVisible" class="tags-timeline">
          <div class="tags-timeline-header">
            <Icon icon="mdi:timeline-clock-outline" width="15" class="text-[rgba(240,138,75,0.8)]" />
            <span class="tags-timeline-title">相关文章</span>
            <span v-if="timelineArticles.length" class="tags-timeline-count">{{ timelineArticles.length }} 篇</span>
          </div>

          <div v-if="timelineLoading" class="tags-timeline-loading">
            <div v-for="i in 3" :key="i" class="th-skeleton" />
          </div>

          <div v-else-if="timelineArticles.length === 0" class="tags-timeline-empty">
            <Icon icon="mdi:newspaper-variant-outline" width="28" class="text-white/15" />
            <p>暂无相关文章</p>
          </div>

          <div v-else class="tags-timeline-list">
            <div
              v-for="article in timelineArticles"
              :key="article.id"
              class="tags-timeline-item"
            >
              <div class="tags-timeline-item-date">
                {{ new Date(article.pubDate).toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' }) }}
              </div>
              <div class="tags-timeline-item-content">
                <a
                  :href="article.link"
                  target="_blank"
                  class="tags-timeline-item-title"
                  rel="noopener noreferrer"
                >{{ article.title }}</a>
                <span class="tags-timeline-item-feed">{{ article.feedId }}</span>
              </div>
            </div>
          </div>
        </div>
      </main>
    </div>

    <!-- Bottom bar -->
    <div class="tags-bottombar" :class="{ 'tags-bottombar--rebuild': rebuildBarVisible }">
      <!-- Rebuild progress -->
      <div v-if="rebuildStatus === 'processing'" class="tags-rebuild-progress">
        <div class="tags-rebuild-bar-track">
          <div
            class="tags-rebuild-bar-fill"
            :style="{ width: `${rebuildProgressPercent}%` }"
          />
        </div>
        <div class="tags-rebuild-info">
          <Icon icon="mdi:loading" width="13" class="animate-spin text-blue-400/60" />
          <span>重建中: 已处理 {{ rebuildProcessed }}/{{ rebuildTotal }}</span>
          <span v-if="currentTag" class="tags-rebuild-current">{{ currentTag }}</span>
        </div>
      </div>

      <!-- Rebuild completed notification -->
      <div v-else-if="rebuildNotification === 'completed'" class="tags-rebuild-done">
        <div class="tags-rebuild-done-content">
          <Icon icon="mdi:check-circle-outline" width="14" class="text-green-400/70" />
          <span>重建完成: {{ rebuildProcessed }} 标签已处理</span>
        </div>
        <button type="button" class="tags-rebuild-dismiss" @click="dismissRebuildNotification">
          <Icon icon="mdi:close" width="12" />
        </button>
      </div>

      <!-- Rebuild failed notification -->
      <div v-else-if="rebuildNotification === 'failed'" class="tags-rebuild-done tags-rebuild-done--failed">
        <div class="tags-rebuild-done-content">
          <Icon icon="mdi:alert-circle-outline" width="14" class="text-red-400/70" />
          <span>重建失败: {{ rebuildError }}</span>
        </div>
        <button type="button" class="tags-rebuild-dismiss" @click="dismissRebuildNotification">
          <Icon icon="mdi:close" width="12" />
        </button>
      </div>

      <!-- Pending changes badge -->
      <button
        type="button"
        class="tags-pending-btn"
        :class="{ 'tags-pending-btn--active': showPendingPanel }"
        @click="showPendingPanel = !showPendingPanel"
      >
        <Icon icon="mdi:swap-horizontal" width="14" />
        <span>待确认变更</span>
        <span v-if="pendingCount > 0" class="tags-pending-badge">{{ pendingCount }}</span>
      </button>
    </div>

    <!-- Add Sector Dialog -->
    <AddSectorDialog
      v-if="showAddDialog"
      @confirm="handleAddSector"
      @cancel="showAddDialog = false"
    />

    <!-- Sector Approval Panel -->
    <SectorApprovalPanel
      v-if="showRegenerateDialog && sectorDiff"
      :diff="sectorDiff"
      :loading="regenerateLoading"
      :category="selectedCategory"
      @done="handleConfirmDone"
      @cancel="showRegenerateDialog = false; sectorDiff = null"
    />

    <!-- Template Settings Dialog -->
    <TemplateSettingsDialog
      v-if="showTemplateDialog"
      :category="selectedCategory"
      @saved="handleTemplateSaved"
      @cancel="showTemplateDialog = false"
    />

    <!-- Pending Change Panel -->
    <PendingChangePanel
      v-if="showPendingPanel"
      :category="selectedCategory"
      @close="showPendingPanel = false"
      @count-update="pendingCount = $event"
    />
  </div>
</template>

<style scoped>
.tags-page {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: #080c12;
  color: rgba(255, 255, 255, 0.85);
}

.tags-topbar {
  position: sticky;
  top: 0;
  z-index: 30;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(8, 12, 18, 0.92);
  backdrop-filter: blur(16px);
}

.tags-topbar-inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  max-width: 1440px;
  margin: 0 auto;
  padding: 0.75rem 1.5rem;
}

.tags-back-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 8px;
  color: rgba(255, 255, 255, 0.35);
  text-decoration: none;
  transition: all 0.12s ease;
}

.tags-back-btn:hover {
  border-color: rgba(255, 255, 255, 0.2);
  color: rgba(255, 255, 255, 0.6);
  background: rgba(255, 255, 255, 0.04);
}

.tags-page-title {
  font-family: serif;
  font-size: 1.1rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.9);
  letter-spacing: 0.02em;
}

.tags-category-tabs {
  display: flex;
  gap: 0.375rem;
}

.tags-cat-btn {
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 999px;
  background: none;
  color: rgba(255, 255, 255, 0.45);
  font-size: 0.75rem;
  padding: 0.3rem 0.9rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.tags-cat-btn:hover {
  border-color: rgba(255, 255, 255, 0.2);
  color: rgba(255, 255, 255, 0.75);
}

.tags-cat-btn--active {
  border-color: rgba(240, 138, 75, 0.5);
  background: rgba(240, 138, 75, 0.12);
  color: rgba(255, 220, 200, 0.9);
}

.tags-settings-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 8px;
  background: none;
  color: rgba(255, 255, 255, 0.35);
  cursor: pointer;
  transition: all 0.12s ease;
}

.tags-settings-btn:hover {
  border-color: rgba(255, 255, 255, 0.2);
  color: rgba(255, 255, 255, 0.6);
  background: rgba(255, 255, 255, 0.04);
}

.tags-main {
  display: flex;
  flex: 1;
  min-height: 0;
  max-width: 1440px;
  width: 100%;
  margin: 0 auto;
}

.tags-sidebar {
  width: 260px;
  flex-shrink: 0;
  border-right: 1px solid rgba(255, 255, 255, 0.05);
  padding: 1rem;
  overflow-y: auto;
}

.tags-sidebar-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.6rem 0.8rem;
  margin-bottom: 0.75rem;
  border-radius: 10px;
  border: 1px solid rgba(240, 138, 75, 0.25);
  background: rgba(240, 138, 75, 0.08);
  color: rgba(255, 200, 180, 0.85);
  font-size: 0.75rem;
}

.tags-closure-card {
  margin-top: 0.75rem;
  padding: 0.8rem;
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.07);
  background: rgba(255, 255, 255, 0.035);
}

.tags-closure-title {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-bottom: 0.65rem;
  color: rgba(255, 255, 255, 0.72);
  font-size: 0.76rem;
  font-weight: 600;
}

.tags-closure-grid {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 0.3rem 0.75rem;
  color: rgba(255, 255, 255, 0.42);
  font-size: 0.68rem;
}

.tags-closure-grid strong {
  color: rgba(255, 255, 255, 0.78);
  font-weight: 600;
}

.tags-closure-blocker {
  margin-top: 0.65rem;
  padding: 0.35rem 0.5rem;
  border-radius: 8px;
  background: rgba(240, 138, 75, 0.1);
  color: rgba(255, 204, 173, 0.86);
  font-size: 0.68rem;
}

.tags-closure-blocker--ok {
  background: rgba(74, 222, 128, 0.08);
  color: rgba(187, 247, 208, 0.82);
}

.tags-closure-blockers {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  margin-top: 0.5rem;
}

.tags-closure-blocker-pill {
  padding: 0.2rem 0.4rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
  color: rgba(255, 255, 255, 0.52);
  font-size: 0.62rem;
}

.tags-content {
  flex: 1;
  min-width: 0;
  padding: 1.25rem 1.5rem 3.5rem;
  overflow-y: auto;
}

.tags-bottombar {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  z-index: 40;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 0.75rem;
  padding: 0.45rem 1.25rem;
  border-top: 1px solid rgba(255, 255, 255, 0.04);
  background: rgba(8, 12, 18, 0.88);
  backdrop-filter: blur(12px);
  transition: all 0.2s ease;
}

.tags-bottombar--rebuild {
  justify-content: space-between;
}

.tags-rebuild-progress {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex: 1;
  max-width: 400px;
}

.tags-rebuild-bar-track {
  flex: 1;
  height: 3px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
  overflow: hidden;
}

.tags-rebuild-bar-fill {
  height: 100%;
  border-radius: 999px;
  background: linear-gradient(90deg, rgba(99, 179, 237, 0.7), rgba(147, 197, 253, 0.9));
  transition: width 0.3s ease;
}

.tags-rebuild-info {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.68rem;
  color: rgba(255, 255, 255, 0.45);
  white-space: nowrap;
}

.tags-rebuild-current {
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  color: rgba(255, 255, 255, 0.25);
}

.tags-rebuild-done {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.3rem 0.65rem;
  border-radius: 8px;
  background: rgba(74, 222, 128, 0.06);
  border: 1px solid rgba(74, 222, 128, 0.12);
  animation: rebuildSlideIn 0.25s ease;
}

.tags-rebuild-done--failed {
  background: rgba(239, 68, 68, 0.06);
  border-color: rgba(239, 68, 68, 0.12);
}

.tags-rebuild-done-content {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.7rem;
  color: rgba(134, 239, 172, 0.8);
}

.tags-rebuild-done--failed .tags-rebuild-done-content {
  color: rgba(252, 165, 165, 0.8);
}

.tags-rebuild-dismiss {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border: none;
  border-radius: 4px;
  background: none;
  color: rgba(255, 255, 255, 0.25);
  cursor: pointer;
  transition: all 0.12s ease;
}

.tags-rebuild-dismiss:hover {
  background: rgba(255, 255, 255, 0.06);
  color: rgba(255, 255, 255, 0.5);
}

@keyframes rebuildSlideIn {
  from { opacity: 0; transform: translateY(4px); }
  to { opacity: 1; transform: translateY(0); }
}

.tags-pending-btn {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 8px;
  background: none;
  color: rgba(255, 255, 255, 0.4);
  font-size: 0.7rem;
  padding: 0.3rem 0.7rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.tags-pending-btn:hover {
  border-color: rgba(255, 255, 255, 0.15);
  color: rgba(255, 255, 255, 0.6);
}

.tags-pending-btn--active {
  border-color: rgba(240, 138, 75, 0.3);
  color: rgba(255, 220, 200, 0.8);
  background: rgba(240, 138, 75, 0.06);
}

.tags-pending-badge {
  font-size: 0.58rem;
  font-weight: 600;
  padding: 0.05rem 0.35rem;
  border-radius: 999px;
  background: rgba(240, 138, 75, 0.2);
  color: rgba(255, 220, 200, 0.9);
  line-height: 1.2;
}

/* Sector timeline */
.tags-timeline {
  margin-top: 2rem;
  padding-top: 1.5rem;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}

.tags-timeline-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-bottom: 0.75rem;
}

.tags-timeline-title {
  font-family: serif;
  font-size: 0.9rem;
  color: rgba(255, 255, 255, 0.8);
}

.tags-timeline-count {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
  padding: 0.1rem 0.45rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
}

.tags-timeline-loading {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.tags-timeline-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.4rem;
  padding: 2.5rem 0;
  color: rgba(255, 255, 255, 0.3);
  font-size: 0.8rem;
}

.tags-timeline-list {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.tags-timeline-item {
  display: flex;
  gap: 0.75rem;
  padding: 0.5rem 0.65rem;
  border-radius: 8px;
  transition: background 0.12s ease;
}

.tags-timeline-item:hover {
  background: rgba(255, 255, 255, 0.03);
}

.tags-timeline-item-date {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
  white-space: nowrap;
  padding-top: 0.15rem;
  min-width: 2.5rem;
}

.tags-timeline-item-content {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  min-width: 0;
}

.tags-timeline-item-title {
  font-size: 0.8rem;
  color: rgba(255, 255, 255, 0.7);
  text-decoration: none;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition: color 0.12s ease;
}

.tags-timeline-item-title:hover {
  color: rgba(255, 220, 200, 0.9);
}

.tags-timeline-item-feed {
  font-size: 0.62rem;
  color: rgba(255, 255, 255, 0.25);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
