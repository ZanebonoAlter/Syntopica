<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import FeedIcon from '~/components/feed/FeedIcon.vue'
import FeedSourceQualityBlock from './FeedSourceQualityBlock.vue'
import type { FeedBoardHitStats, RssFeed, Category } from '~/types'
import type { StatsWindowDays } from '../utils/sourceQuality'

const props = withDefaults(defineProps<{
  feed: RssFeed
  categories: Category[]
  refreshOptions: { label: string; value: number }[]
  maxArticlesOptions: { label: string; value: number }[]
  loading: boolean
  /** 来源质量块（add-source-board-hit-rate §4.4）：统计/窗口/错误由 SettingsSectionFeeds 提升持有 */
  stats?: FeedBoardHitStats
  statsLoading?: boolean
  statsError?: string | null
  windowDays?: StatsWindowDays
}>(), {
  stats: undefined,
  statsLoading: false,
  statsError: null,
  windowDays: 7,
})

const emit = defineEmits<{
  'update-feed': [feedId: string, setting: 'refresh_interval' | 'max_articles' | 'tagging_enabled' | 'firecrawl_enabled' | 'completion_on_refresh' | 'category_id', value: number | boolean | null]
  'refresh-feed': [feedId: string]
  'create-category': [name: string]
  'delete-feed': [feedId: string]
  'set-window': [days: StatsWindowDays]
  'retry-stats': []
}>()

// ---- Category management ----
const NEW_CATEGORY_VALUE = '__new__'
const selectedCategory = ref(props.feed.category)
const showCreateCategory = ref(false)
const newCategoryName = ref('')

watch(() => props.feed.category, (v) => { selectedCategory.value = v })

function onCategoryChange() {
  if (selectedCategory.value === NEW_CATEGORY_VALUE) {
    showCreateCategory.value = true
    return
  }
  const categoryId = selectedCategory.value === '' ? null : Number(selectedCategory.value)
  emit('update-feed', props.feed.id, 'category_id', categoryId)
}

function submitNewCategory() {
  const name = newCategoryName.value.trim()
  if (!name) return
  emit('create-category', name)
  showCreateCategory.value = false
  newCategoryName.value = ''
}

function cancelCreateCategory() {
  showCreateCategory.value = false
  newCategoryName.value = ''
  selectedCategory.value = props.feed.category
}

function formatInterval(minutes: number): string {
  const opt = props.refreshOptions.find(o => o.value === minutes)
  return opt?.label || `${minutes} 分钟`
}

function formatMaxArticles(count: number): string {
  if (count <= 0 || count >= 9999) return '无限制'
  const opt = props.maxArticlesOptions.find(o => o.value === count)
  return opt?.label || `${count} 篇`
}

function getIntervalColor(minutes: number): string {
  if (minutes === 0) return 'var(--color-text-muted)'
  if (minutes <= 30) return 'var(--color-success)'
  if (minutes <= 120) return 'var(--color-link)'
  return 'var(--color-text-secondary)'
}

function formatStatus(feed: RssFeed): string {
  if (feed.refreshStatus === 'refreshing') return '刷新中…'
  if (feed.refreshStatus === 'error') return feed.refreshError || '刷新失败'
  if (feed.lastRefreshAt) {
    const d = new Date(feed.lastRefreshAt)
    return d.toLocaleString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
  }
  return '未刷新'
}
</script>

<template>
  <div class="feed-detail">
    <!-- Feed header -->
    <div class="feed-detail__header">
      <div
        class="feed-detail__icon"
        :style="{ backgroundColor: (feed.color || '#999') + '15' }"
      >
        <FeedIcon :icon="feed.icon" :feed-id="feed.id" :color="feed.color" :size="24" />
      </div>
      <div class="feed-detail__title-group">
        <h3 class="feed-detail__title">{{ feed.title }}</h3>
        <p class="feed-detail__url">{{ feed.url }}</p>
      </div>
      <div class="feed-detail__header-actions">
        <button
          class="feed-detail__refresh-btn"
          title="立即刷新"
          :disabled="loading"
          @click="emit('refresh-feed', feed.id)"
        >
          <Icon
            :icon="loading ? 'mdi:loading' : 'mdi:refresh'"
            :class="{ 'animate-spin': loading }"
            width="16"
            height="16"
          />
          刷新
        </button>
        <button
          class="feed-detail__delete-btn"
          title="删除订阅源"
          :disabled="loading"
          @click="emit('delete-feed', feed.id)"
        >
          <Icon icon="mdi:trash-can-outline" width="18" height="18" />
        </button>
      </div>
    </div>

    <!-- Status bar -->
    <div class="feed-detail__status">
      <span class="feed-detail__status-item" :style="{ color: getIntervalColor(feed.refreshInterval || 0) }">
        <Icon icon="mdi:clock" width="14" height="14" />
        {{ formatInterval(feed.refreshInterval || 0) }}
      </span>
      <span class="feed-detail__status-item">
        <Icon icon="mdi:file-document-multiple" width="14" height="14" />
        {{ formatMaxArticles(feed.maxArticles ?? 100) }}
      </span>
      <span class="feed-detail__status-item">
        <Icon icon="mdi:file-document" width="14" height="14" />
        {{ feed.articleCount }} 篇
      </span>
      <span class="feed-detail__status-item" :class="{ 'feed-detail__status-item--error': feed.refreshStatus === 'error' }">
        <Icon icon="mdi:update" width="14" height="14" />
        {{ formatStatus(feed) }}
      </span>
    </div>

    <!-- 来源质量块（只读观测，位置契约：状态条之后、设置表单之前；失败不阻断下方表单） -->
    <FeedSourceQualityBlock
      :feed="feed"
      :stats="stats"
      :loading="statsLoading"
      :error="statsError"
      :window-days="windowDays"
      @set-window="emit('set-window', $event)"
      @retry="emit('retry-stats')"
    />

    <!-- Settings form -->
    <div class="feed-detail__form">
      <!-- Category selector -->
      <div class="feed-detail__field feed-detail__field--category">
        <label class="feed-detail__label">分类</label>
        <select
          v-model="selectedCategory"
          class="feed-detail__select"
          @change="onCategoryChange"
        >
          <option value="">无分类</option>
          <option v-for="c in categories" :key="c.id" :value="c.id">{{ c.name }}</option>
          <option :value="NEW_CATEGORY_VALUE">＋ 新建分类…</option>
        </select>
        <div v-if="showCreateCategory" class="feed-detail__new-category">
          <input
            v-model="newCategoryName"
            class="feed-detail__input"
            placeholder="输入分类名称"
            @keyup.enter="submitNewCategory"
          />
          <button class="feed-detail__mini-btn feed-detail__mini-btn--primary" @click="submitNewCategory">确认</button>
          <button class="feed-detail__mini-btn" @click="cancelCreateCategory">取消</button>
        </div>
      </div>

      <div class="feed-detail__row">
        <div class="feed-detail__field">
          <label class="feed-detail__label">自动刷新</label>
          <select
            :value="feed.refreshInterval"
            class="feed-detail__select"
            @change="emit('update-feed', feed.id, 'refresh_interval', Number(($event.target as HTMLSelectElement).value))"
          >
            <option v-for="option in refreshOptions" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
        </div>
        <div class="feed-detail__field">
          <label class="feed-detail__label">最大文章数</label>
          <select
            :value="feed.maxArticles"
            class="feed-detail__select"
            @change="emit('update-feed', feed.id, 'max_articles', Number(($event.target as HTMLSelectElement).value))"
          >
            <option v-for="option in maxArticlesOptions" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
          <p class="feed-detail__field-hint">超出后旧文归档保留原文，不再出现在列表中</p>
        </div>
      </div>

      <div class="feed-detail__toggles">
        <div class="feed-detail__toggle-row">
          <div class="feed-detail__toggle-label">
            <Icon icon="mdi:web" width="16" height="16" style="color: var(--color-link)" />
            <div>
              <span class="feed-detail__toggle-name">Firecrawl 全文抓取</span>
              <span class="feed-detail__toggle-desc">使用 Firecrawl 获取完整文章内容</span>
            </div>
          </div>
          <AppToggle
            :model-value="!!feed.firecrawlEnabled"
            @update:model-value="emit('update-feed', feed.id, 'firecrawl_enabled', $event)"
          />
        </div>

        <div class="feed-detail__toggle-row">
          <div class="feed-detail__toggle-label">
            <Icon icon="mdi:tag-multiple" width="16" height="16" style="color: var(--color-secondary)" />
            <div>
              <span class="feed-detail__toggle-name">AI 打标签</span>
              <span class="feed-detail__toggle-desc">自动为文章生成主题标签</span>
            </div>
          </div>
          <AppToggle
            :model-value="!!feed.taggingEnabled"
            @update:model-value="emit('update-feed', feed.id, 'tagging_enabled', $event)"
          />
        </div>

        <div class="feed-detail__toggle-row">
          <div class="feed-detail__toggle-label">
            <Icon icon="mdi:text-box-plus" width="16" height="16" style="color: var(--color-success)" />
            <div>
              <span class="feed-detail__toggle-name">内容补全</span>
              <span class="feed-detail__toggle-desc">刷新时自动补全文章正文</span>
            </div>
          </div>
          <AppToggle
            :model-value="!!feed.completionOnRefresh"
            @update:model-value="emit('update-feed', feed.id, 'completion_on_refresh', $event)"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.feed-detail {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.feed-detail__header {
  display: flex;
  align-items: center;
  gap: 12px;
}

.feed-detail__icon {
  width: 44px;
  height: 44px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.feed-detail__title-group {
  flex: 1;
  min-width: 0;
}

.feed-detail__title {
  font-size: 16px;
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.feed-detail__url {
  font-size: 12px;
  color: var(--color-text-muted);
  margin: 2px 0 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.feed-detail__refresh-btn {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 12px;
  border: 1px solid var(--color-border-medium);
  border-radius: 8px;
  background: var(--color-bg-elevated);
  color: var(--color-text-secondary);
  font-size: 13px;
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
  flex-shrink: 0;
}

.feed-detail__refresh-btn:hover {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
}

.feed-detail__refresh-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.feed-detail__header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.feed-detail__delete-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border: 1px solid var(--color-border-medium);
  border-radius: 8px;
  background: var(--color-bg-elevated);
  color: var(--color-error);
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s;
}

.feed-detail__delete-btn:hover:not(:disabled) {
  background: rgba(248, 113, 113, 0.1);
  border-color: var(--color-error);
}

.feed-detail__delete-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.feed-detail__field--category {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.feed-detail__new-category {
  display: flex;
  align-items: center;
  gap: 6px;
}

.feed-detail__input {
  flex: 1;
  min-width: 0;
  padding: 7px 10px;
  font-size: 13px;
  border: 1px solid var(--color-input-border);
  border-radius: 8px;
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  outline: none;
  transition: border-color 0.15s;
}

.feed-detail__input:focus {
  border-color: var(--color-input-focus);
}

.feed-detail__mini-btn {
  padding: 6px 12px;
  font-size: 12px;
  border: 1px solid var(--color-border-medium);
  border-radius: 8px;
  background: var(--color-bg-elevated);
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: background 0.15s;
  flex-shrink: 0;
}

.feed-detail__mini-btn:hover {
  background: var(--color-bg-hover);
}

.feed-detail__mini-btn--primary {
  background: var(--color-link);
  border-color: var(--color-link);
  color: #fff;
}

.feed-detail__mini-btn--primary:hover {
  opacity: 0.9;
  background: var(--color-link);
}

/* Status bar */
.feed-detail__status {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  padding: 10px 14px;
  border-radius: 8px;
  background: var(--color-bg-sunken);
}

.feed-detail__status-item {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--color-text-secondary);
}

.feed-detail__status-item--error {
  color: var(--color-error);
}

/* Form */
.feed-detail__form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.feed-detail__row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.feed-detail__field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.feed-detail__label {
  font-size: 12px;
  font-weight: 500;
  color: var(--color-text-secondary);
}

.feed-detail__field-hint {
  font-size: 12px;
  color: var(--color-text-secondary);
  opacity: 0.8;
}

.feed-detail__select {
  padding: 7px 10px;
  font-size: 13px;
  border: 1px solid var(--color-input-border);
  border-radius: 8px;
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  outline: none;
  transition: border-color 0.15s;
}

.feed-detail__select:focus {
  border-color: var(--color-input-focus);
}

/* Toggles */
.feed-detail__toggles {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--color-border-subtle);
  border-radius: 10px;
  overflow: hidden;
}

.feed-detail__toggle-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.feed-detail__toggle-row:last-child {
  border-bottom: none;
}

.feed-detail__toggle-label {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.feed-detail__toggle-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text-primary);
  display: block;
}

.feed-detail__toggle-desc {
  font-size: 11px;
  color: var(--color-text-muted);
  display: block;
}

/* Responsive */
@media (max-width: 600px) {
  .feed-detail__row {
    grid-template-columns: 1fr;
  }
}
</style>
