<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { RssFeed } from '~/types'
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
}

withDefaults(defineProps<Props>(), {
  showBackButton: false,
  showNavButtons: false,
})

const emit = defineEmits<{
  'toggle-favorite': []
  'toggle-view-mode': []
  'toggle-fullscreen': []
  'navigate-prev': []
  'navigate-next': []
  'open-original': []
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
      <div v-if="feed" class="feed-badge">
        <FeedIcon :icon="feed.icon" :color="feed.color" :size="16" />
        <span class="text-sm font-medium" :style="{ color: feed.color }">{{ feed.title }}</span>
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
    </div>
  </header>
</template>
