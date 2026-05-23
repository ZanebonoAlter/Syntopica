<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { useRefreshPolling } from '~/features/feeds/composables/useRefreshPolling'
import { SIDEBAR_MIN_WIDTH, SIDEBAR_MAX_WIDTH } from '~/utils/constants'
import AppTooltip from '~/components/common/AppTooltip.vue'
import FeedActionMenu from '~/components/feed/FeedActionMenu.vue'
import type { WatchedTag } from '~/api/watchedTags'

interface Props {
  sidebarCollapsed?: boolean
  sidebarWidth?: number
  selectedCategory?: string | null
  selectedFeed?: string | null
  globalUnreadCount?: number
  watchedTags?: WatchedTag[]
  selectedWatchedTagId?: string | null
}

const props = withDefaults(defineProps<Props>(), {
  sidebarCollapsed: false,
  sidebarWidth: 256,
  selectedCategory: null,
  selectedFeed: null,
  globalUnreadCount: 0,
  watchedTags: () => [],
  selectedWatchedTagId: null,
})

const emit = defineEmits<{
  toggleSidebar: []
  'update:sidebarWidth': [value: number]
  categoryClick: [categoryId: string]
  feedClick: [feedId: string]
  favoritesClick: []
  topicGraphClick: []
  allArticlesClick: []
  editCategory: [categoryId: string]
  editFeed: [feedId: string]
  deleteCategory: [categoryId: string, categoryName: string]
  markFeedAsRead: [feedId: string]
  startResizing: [event: MouseEvent]
  stopResizing: []
  watchedTagsClick: []
  watchedTagClick: [tagId: string]
}>()

const apiStore = useApiStore()
const feedsStore = useFeedsStore()
const articlesStore = useArticlesStore()
const isResizing = ref(false)
const { updateSelection } = useRefreshPolling()

const uncategorizedFeeds = computed(() => feedsStore.feeds.filter((f) => !f.category))

function startResizing(event: MouseEvent) {
  isResizing.value = true
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
  document.addEventListener('mousemove', onResizing)
  document.addEventListener('mouseup', stopResizing)
}

function onResizing(event: MouseEvent) {
  if (!isResizing.value) return

  const newWidth = event.clientX
  if (newWidth >= SIDEBAR_MIN_WIDTH && newWidth <= SIDEBAR_MAX_WIDTH) {
    emit('update:sidebarWidth', newWidth)
  }
}

function stopResizing() {
  isResizing.value = false
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
  document.removeEventListener('mousemove', onResizing)
  document.removeEventListener('mouseup', stopResizing)
}

function handleCategoryClick(categoryId: string) {
  updateSelection(categoryId, null)
  emit('categoryClick', categoryId)
}

function handleFeedClick(feedId: string) {
  updateSelection(props.selectedCategory, feedId)
  emit('feedClick', feedId)
}

function handleFavoritesClick() {
  updateSelection('favorites', null)
  emit('favoritesClick')
}

function handleTopicGraphClick() {
  updateSelection('topic-graph', null)
  emit('topicGraphClick')
}

function handleAllArticlesClick() {
  updateSelection(null, null)
  emit('allArticlesClick')
}

async function handleMarkFeedAsRead(feedId: string) {
  const response = await apiStore.markAllAsRead({ feedId })
  if (!response.success) return

  const feed = feedsStore.feeds.find(f => f.id === feedId)
  if (feed && feed.unreadCount) {
    feed.unreadCount = 0
  }
}

import '~/components/layout/AppSidebar.css'

const navigateTo = useNuxtApp().$router ? (path: string) => useNuxtApp().$router.push(path) : () => {}
</script>

<style scoped>
.watched-tags-section {
  padding: 0 0.5rem;
}
.watched-tags-header {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.5rem 0.5rem 0.25rem;
}
.sidebar-item--sm {
  padding: 0.3rem 0.5rem;
  font-size: 0.82rem;
  gap: 0.4rem;
}
.watched-tags-empty {
  padding: 0.5rem 0.5rem 0.75rem;
  border-radius: 10px;
  background: rgba(59, 107, 135, 0.06);
  margin: 0.25rem 0;
}
.watched-tags-go-btn {
  display: inline-block;
  margin-top: 0.35rem;
  padding: 0.2rem 0.6rem;
  border: 1px solid rgba(59, 107, 135, 0.25);
  border-radius: 999px;
  background: none;
  color: var(--color-ink-600);
  font-size: 0.72rem;
  cursor: pointer;
  transition: all 0.12s ease;
}
.watched-tags-go-btn:hover {
  background: rgba(59, 107, 135, 0.08);
  border-color: rgba(59, 107, 135, 0.4);
}
</style>

<template>
  <aside class="app-sidebar" :style="sidebarCollapsed ? 'width: 48px' : `width: ${sidebarWidth}px`">
    <div v-if="!sidebarCollapsed" class="resize-handle" :class="{ active: isResizing }" @mousedown="startResizing" />

    <div class="sidebar-content">
      <button class="sidebar-item" :class="{ active: !selectedCategory && !selectedFeed }" @click="handleAllArticlesClick">
        <Icon icon="mdi:inbox" width="20" height="20" />
        <span v-if="!sidebarCollapsed" class="flex-1 text-left font-medium">全部文章</span>
        <span v-if="!sidebarCollapsed && globalUnreadCount > 0" class="badge">{{ globalUnreadCount }}</span>
      </button>

      <button class="sidebar-item" :class="{ active: selectedCategory === 'favorites' }" @click="handleFavoritesClick">
        <Icon icon="mdi:star" width="20" height="20" />
        <span v-if="!sidebarCollapsed" class="flex-1 text-left font-medium">收藏夹</span>
        <span v-if="!sidebarCollapsed && articlesStore.favoriteCount > 0" class="badge badge-amber">{{ articlesStore.favoriteCount }}</span>
      </button>

      <button class="sidebar-item" :class="{ active: selectedCategory === 'topic-graph' }" @click="handleTopicGraphClick">
        <Icon icon="mdi:graph-outline" width="20" height="20" class="text-ink-600" />
        <span v-if="!sidebarCollapsed" class="flex-1 text-left font-medium">主题图谱</span>
      </button>

      <button class="sidebar-item" @click="navigateTo('/tags')">
        <Icon icon="mdi:tag-multiple" width="20" height="20" class="text-ink-600" />
        <span v-if="!sidebarCollapsed" class="flex-1 text-left font-medium">标签管理</span>
      </button>

      <div v-if="!sidebarCollapsed" class="divider" />

      <div v-if="!sidebarCollapsed" class="watched-tags-section">
        <div class="watched-tags-header">
          <Icon icon="mdi:heart-outline" width="14" class="text-ink-400" />
          <span class="text-xs text-ink-400 font-medium">关注标签</span>
        </div>

        <template v-if="watchedTags.length > 0">
          <button
            class="sidebar-item sidebar-item--sm"
            :class="{ active: selectedCategory === 'watched-tags' && !selectedWatchedTagId }"
            @click="emit('watchedTagsClick')"
          >
            <Icon icon="mdi:heart-multiple" width="16" height="16" class="text-red-400" />
            <span class="flex-1 text-left">全部关注</span>
          </button>
          <button
            v-for="tag in watchedTags"
            :key="tag.id"
            class="sidebar-item sidebar-item--sm"
            :class="{ active: selectedCategory === 'watched-tags' && selectedWatchedTagId === String(tag.id) }"
            @click="emit('watchedTagClick', String(tag.id))"
          >
            <Icon :icon="tag.isAbstract ? 'mdi:tag-multiple' : 'mdi:tag'" width="16" height="16" :class="tag.isAbstract ? 'text-indigo-500' : 'text-ink-400'" />
            <span class="flex-1 text-left truncate">{{ tag.label }}</span>
          </button>
        </template>

        <div v-else class="watched-tags-empty">
          <p class="text-xs text-ink-500">关注标签可获取个性化文章推送</p>
          <button class="watched-tags-go-btn" @click="navigateTo('/topics')">
            前往关注
          </button>
        </div>
      </div>

      <div v-if="!sidebarCollapsed" class="divider" />

      <div v-if="!sidebarCollapsed" class="categories">
        <div v-for="category in feedsStore.categories" :key="category.id" class="category-group">
          <div class="category-item" :class="{ active: selectedCategory === category.id }">
            <button class="category-btn" :class="{ 'text-ink-700': selectedCategory === category.id }" @click="handleCategoryClick(category.id)">
              <Icon :icon="category.icon" width="18" height="18" />
              <span class="text-sm font-medium">{{ category.name }}</span>
              <span class="count">{{ category.feedCount }}</span>
            </button>
            <div class="category-actions">
              <button class="action-btn" title="编辑分类" @click.stop="$emit('editCategory', category.id)">
                <Icon icon="mdi:pencil" width="15" height="15" class="text-gray-500" />
              </button>
              <button class="action-btn" title="删除分类" @click.stop="$emit('deleteCategory', category.id, category.name)">
                <Icon icon="mdi:delete" width="15" height="15" class="text-gray-500" />
              </button>
            </div>
          </div>

          <div v-if="selectedCategory === category.id" class="feeds-list">
            <div v-for="feed in feedsStore.getFeedsByCategory(category.id)" :key="feed.id" class="feed-item" :class="{ active: selectedFeed === feed.id }">
              <AppTooltip :content="`${feedsStore.unreadCountsByFeed[feed.id] || 0} 篇未读文章`" :disabled="(feedsStore.unreadCountsByFeed[feed.id] || 0) <= 0">
                <span v-if="(feedsStore.unreadCountsByFeed[feed.id] || 0) > 0" class="badge badge-sm">
                  {{ feedsStore.unreadCountsByFeed[feed.id] || 0 }}
                </span>
              </AppTooltip>

              <button class="feed-btn" @click="handleFeedClick(feed.id)">
                <FeedIcon :icon="feed.icon" :feed-id="feed.id" :size="16" />
                <AppTooltip :content="feed.title">
                  <span class="truncate">{{ feed.title }}</span>
                </AppTooltip>
              </button>

              <div class="feed-action-wrapper">
                <FeedActionMenu :feed-id="feed.id" :feed-title="feed.title" @mark-as-read="handleMarkFeedAsRead" @edit="$emit('editFeed', $event)" />
              </div>
            </div>
          </div>
        </div>

        <div v-if="uncategorizedFeeds.length > 0" class="uncategorized">
          <div class="category-item" :class="{ active: selectedCategory === 'uncategorized' }">
            <button class="category-btn" :class="{ 'text-ink-700': selectedCategory === 'uncategorized' }" @click="handleCategoryClick('uncategorized')">
              <Icon icon="mdi:folder-off" width="18" height="18" />
              <span class="text-sm font-medium">未分类</span>
              <span class="count">{{ uncategorizedFeeds.length }}</span>
            </button>
          </div>

          <div v-if="selectedCategory === 'uncategorized'" class="feeds-list">
            <div v-for="feed in uncategorizedFeeds" :key="feed.id" class="feed-item" :class="{ active: selectedFeed === feed.id }">
              <AppTooltip :content="`${feedsStore.unreadCountsByFeed[feed.id] || 0} 篇未读文章`" :disabled="(feedsStore.unreadCountsByFeed[feed.id] || 0) <= 0">
                <span v-if="(feedsStore.unreadCountsByFeed[feed.id] || 0) > 0" class="badge badge-sm">
                  {{ feedsStore.unreadCountsByFeed[feed.id] || 0 }}
                </span>
              </AppTooltip>

              <button class="feed-btn" @click="handleFeedClick(feed.id)">
                <FeedIcon :icon="feed.icon" :feed-id="feed.id" :size="16" />
                <AppTooltip :content="feed.title">
                  <span class="truncate">{{ feed.title }}</span>
                </AppTooltip>
              </button>

              <div class="feed-action-wrapper">
                <FeedActionMenu :feed-id="feed.id" :feed-title="feed.title" @mark-as-read="handleMarkFeedAsRead" @edit="$emit('editFeed', $event)" />
              </div>
            </div>
          </div>
        </div>
      </div>

      <div v-else class="collapsed-view">
        <div v-for="category in feedsStore.categories" :key="category.id">
          <button class="sidebar-item collapsed-item" :class="{ active: selectedCategory === category.id }" :title="category.name" @click="handleCategoryClick(category.id)">
            <Icon :icon="category.icon" width="20" height="20" />
          </button>
        </div>
      </div>
    </div>
  </aside>
</template>
