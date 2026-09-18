<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Icon } from '@iconify/vue'
import { useGlobalSettings } from '~/composables/useGlobalSettings'
import { useApiStore } from '~/stores/api'
import FeedMasterList from './FeedMasterList.vue'
import FeedDetailEditor from './FeedDetailEditor.vue'
import { useFeedSourceQuality } from '~/features/settings/composables/useFeedSourceQuality'

const apiStore = useApiStore()
const {
  collapsedCategories, loading, error, success,
  feedsByCategory, categories, refreshOptions, maxArticlesOptions,
  updateFeedSetting, refreshFeed, createCategoryAndAssign, deleteFeed,
} = useGlobalSettings()

// 来源质量聚合（add-source-board-hit-rate §4.5）：窗口状态提升于此，
// 列表工具栏与详情块共享同一实例（spec：任一处切换两处同步）。
const {
  windowDays: statsWindowDays,
  statsByFeed,
  loading: statsLoading,
  error: statsError,
  setWindow: setStatsWindow,
  retry: retryStats,
  load: loadStats,
} = useFeedSourceQuality()

const selectedFeedId = ref<string | undefined>()

// 全量 feeds 与统计并行拉取，任一失败不阻塞另一个（tasks 4.5）
onMounted(() => {
  apiStore.fetchFeeds({ per_page: 10000 })
  loadStats()
})

const allFeeds = computed(() =>
  Object.values(feedsByCategory.value).flat()
)

const selectedFeed = computed(() =>
  allFeeds.value.find(f => f.id === selectedFeedId.value)
)

/** 选中源在当前窗口下的统计（无数据时详情块显示骨架/错误态，不阻塞表单） */
const selectedFeedStats = computed(() =>
  selectedFeed.value ? statsByFeed.value[String(selectedFeed.value.id)] : undefined
)

function onCreateCategory(name: string) {
  if (!selectedFeed.value) return
  createCategoryAndAssign(selectedFeed.value.id, name)
}

async function onDeleteFeed() {
  if (!selectedFeed.value) return
  const ok = await deleteFeed(
    selectedFeed.value.id,
    selectedFeed.value.title,
    selectedFeed.value.articleCount,
  )
  if (ok) selectedFeedId.value = undefined
}

// 订阅源管理入口（增/导入/导出）——聚合在设置页（slim-header-feed-actions）
const showAddFeedDialog = ref(false)
const showAddCategoryDialog = ref(false)
const showImportDialog = ref(false)

async function handleExportOpml() {
  try {
    const blob = await apiStore.exportOpml()
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `feeds-export-${new Date().toISOString().split('T')[0]}.opml`
    a.click()
    window.URL.revokeObjectURL(url)
  } catch {
    error.value = '导出失败'
  }
}

function reloadFeeds() {
  apiStore.fetchFeeds({ per_page: 10000 })
}
</script>

<template>
  <div class="feeds-section">
    <div v-if="success" class="msg msg--success">{{ success }}</div>
    <div v-if="error" class="msg msg--error">{{ error }}</div>

    <div class="feeds-toolbar">
      <button class="feeds-toolbar__btn" @click="showAddFeedDialog = true">
        <Icon icon="mdi:plus" width="16" height="16" />
        <span>添加订阅源</span>
      </button>
      <button class="feeds-toolbar__btn" @click="showAddCategoryDialog = true">
        <Icon icon="mdi:folder-plus" width="16" height="16" />
        <span>添加分类</span>
      </button>
      <button class="feeds-toolbar__btn" @click="showImportDialog = true">
        <Icon icon="mdi:import" width="16" height="16" />
        <span>导入</span>
      </button>
      <button class="feeds-toolbar__btn" @click="handleExportOpml">
        <Icon icon="mdi:export" width="16" height="16" />
        <span>导出</span>
      </button>
    </div>

    <div class="feeds-layout">
      <!-- Master list (left) -->
      <div class="feeds-master">
        <FeedMasterList
          v-model:selected-feed-id="selectedFeedId"
          :feeds-by-category="feedsByCategory"
          :collapsed-categories="collapsedCategories"
          :stats-by-feed="statsByFeed"
          :stats-loading="statsLoading"
          :window-days="statsWindowDays"
          @set-window="setStatsWindow"
          @toggle-collapse="collapsedCategories[$event] = !collapsedCategories[$event]"
        />
      </div>

      <!-- Detail editor (right) -->
      <div class="feeds-detail">
        <FeedDetailEditor
          v-if="selectedFeed"
          :key="selectedFeed.id"
          :feed="selectedFeed"
          :categories="categories"
          :refresh-options="refreshOptions"
          :max-articles-options="maxArticlesOptions"
          :loading="loading"
          :stats="selectedFeedStats"
          :stats-loading="statsLoading"
          :stats-error="statsError"
          :window-days="statsWindowDays"
          @set-window="setStatsWindow"
          @retry-stats="retryStats"
          @update-feed="updateFeedSetting"
          @refresh-feed="refreshFeed"
          @create-category="onCreateCategory"
          @delete-feed="onDeleteFeed"
        />

        <!-- Empty state -->
        <div v-else class="feeds-detail__empty">
          <Icon icon="mdi:rss" width="48" height="48" style="color: var(--color-text-muted); opacity: 0.4" />
          <p class="feeds-detail__empty-title">选择一个订阅源</p>
          <p class="feeds-detail__empty-desc">从左侧列表中选择一个订阅源来查看和编辑配置</p>
        </div>
      </div>
    </div>

    <AddFeedDialog
      v-if="showAddFeedDialog"
      @close="showAddFeedDialog = false"
      @added="reloadFeeds"
    />
    <AddCategoryDialog
      v-if="showAddCategoryDialog"
      @close="showAddCategoryDialog = false"
      @added="reloadFeeds"
    />
    <ImportOpmlDialog
      v-if="showImportDialog"
      @close="showImportDialog = false"
      @imported="reloadFeeds"
    />
  </div>
</template>

<style scoped>
.feeds-section {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.feeds-layout {
  display: flex;
  flex: 1;
  min-height: 0;
  border: 1px solid var(--color-border-subtle);
  border-radius: 12px;
  overflow: hidden;
  background: var(--color-bg-elevated);
}

.feeds-master {
  /* ui-design Layout Contract：master 栏 width: 320px; min-width: 280px */
  width: 320px;
  min-width: 280px;
  flex-shrink: 0;
  border-right: 1px solid var(--color-border-subtle);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.feeds-detail {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  padding: 20px;
}

.feeds-detail__empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 100%;
  min-height: 200px;
}

.feeds-detail__empty-title {
  font-size: 15px;
  font-weight: 500;
  color: var(--color-text-secondary);
  margin: 0;
}

.feeds-detail__empty-desc {
  font-size: 13px;
  color: var(--color-text-muted);
  margin: 0;
  text-align: center;
}

.msg {
  padding: 0.75rem;
  border-radius: 8px;
  font-size: 0.875rem;
  margin-bottom: 12px;
}

.msg--success {
  background: rgba(74, 222, 128, 0.1);
  border: 1px solid rgba(74, 222, 128, 0.2);
  color: #4ade80;
}

.msg--error {
  background: rgba(248, 113, 113, 0.1);
  border: 1px solid rgba(248, 113, 113, 0.2);
  color: #f87171;
}

.feeds-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
}

.feeds-toolbar__btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-elevated);
  color: var(--color-text-primary);
  font-size: 13px;
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s;
}

.feeds-toolbar__btn:hover {
  background: var(--color-bg-hover);
  border-color: var(--color-border-medium);
}

/* Responsive: stack on narrow */
@media (max-width: 768px) {
  .feeds-layout {
    flex-direction: column;
  }

  .feeds-master {
    width: 100%;
    max-height: 40vh;
    border-right: none;
    border-bottom: 1px solid var(--color-border-subtle);
  }
}
</style>
