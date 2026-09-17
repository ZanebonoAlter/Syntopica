<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { RssFeed } from '~/types'
import ArticleStatusMenu, { type ArticleStatusMenuProps } from './ArticleStatusMenu.vue'
import '~/components/article/ArticleContent.css'

defineOptions({ inheritAttrs: false })

interface Props {
  feed: RssFeed | null
  articleTitle: string
  articleFavorite: boolean
  viewMode: 'preview' | 'iframe'
  hasPrev: boolean
  hasNext: boolean
  showBackButton: boolean
  showNavButtons: boolean
  /** 状态图标 + ⋯ 菜单数据（ArticleContentView 从 useArticleContentView 直取，不经 PreviewPanel 转发，design D3） */
  statusMenu?: ArticleStatusMenuProps | null
}

withDefaults(defineProps<Props>(), {
  showBackButton: false,
  showNavButtons: false,
  statusMenu: null,
})

const emit = defineEmits<{
  'toggle-favorite': []
  'toggle-view-mode': []
  'toggle-fullscreen': []
  'navigate-prev': []
  'navigate-next': []
  'open-original': []
  'manual-firecrawl': []
  'manual-summary': []
  'manual-tagging': []
  'set-content-source': [source: string]
}>()
</script>

<template>
  <header class="article-header">
    <div class="header-left">
      <button
        v-if="showBackButton"
        class="flex items-center gap-1 rounded-lg p-2 text-[var(--color-text-secondary)] transition-all duration-200 hover:bg-[var(--color-bg-hover)] hover:text-[var(--color-text-primary)]"
        @click="emit('toggle-fullscreen')"
      >
        <Icon icon="mdi:arrow-left" width="20" height="20" />
        <span class="text-sm">退出全屏</span>
      </button>
      <!-- feed 徽章降噪：图标保留原色，名称改次要文本色（不再用 feed.color 染字） -->
      <div v-if="feed" class="feed-badge">
        <FeedIcon :icon="feed.icon" :color="feed.color" :size="16" />
        <span class="text-sm font-medium text-[var(--color-text-secondary)]">{{ feed.title }}</span>
      </div>
      <span class="article-title">{{ articleTitle }}</span>
    </div>

    <div class="header-actions">
      <template v-if="showNavButtons">
        <button class="action-btn" :class="{ 'opacity-30 cursor-not-allowed': !hasPrev }" :disabled="!hasPrev" title="上一篇文章" @click="emit('navigate-prev')">
          <Icon icon="mdi:chevron-up" width="20" height="20" />
        </button>
        <button class="action-btn" :class="{ 'opacity-30 cursor-not-allowed': !hasNext }" :disabled="!hasNext" title="下一篇文章" @click="emit('navigate-next')">
          <Icon icon="mdi:chevron-down" width="20" height="20" />
        </button>
        <div class="mx-1 h-5 w-px bg-[var(--color-border-subtle)]" />
      </template>

      <button class="action-btn" :title="viewMode === 'preview' ? '切换到内嵌网页' : '切换到内容预览'" @click="emit('toggle-view-mode')">
        <Icon :icon="viewMode === 'preview' ? 'mdi:web' : 'mdi:file-document-outline'" width="20" height="20" />
      </button>

      <button class="action-btn" :class="{ active: articleFavorite }" :title="articleFavorite ? '取消收藏' : '收藏'" @click="emit('toggle-favorite')">
        <Icon :icon="articleFavorite ? 'mdi:star' : 'mdi:star-outline'" width="20" height="20" />
      </button>

      <button class="action-btn" :title="showBackButton ? '退出全屏' : '全屏'" @click="emit('toggle-fullscreen')">
        <Icon :icon="showBackButton ? 'mdi:fullscreen-exit' : 'mdi:fullscreen'" width="20" height="20" />
      </button>

      <button class="action-btn" title="在新窗口打开原文" @click="emit('open-original')">
        <Icon icon="mdi:external-link" width="20" height="20" />
      </button>

      <!-- 处理状态图标 + ⋯ 更多操作菜单（含处理详情浮层） -->
      <ArticleStatusMenu
        v-if="statusMenu"
        v-bind="statusMenu"
        @manual-firecrawl="emit('manual-firecrawl')"
        @manual-summary="emit('manual-summary')"
        @manual-tagging="emit('manual-tagging')"
        @update:active-content-source="emit('set-content-source', $event)"
      />
    </div>
  </header>
</template>
