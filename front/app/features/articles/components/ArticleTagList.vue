<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { computed } from 'vue'
import type { ArticleTag } from '~/types'

interface Props {
  tags?: ArticleTag[]
  highlightedSlugs?: string[]
  compact?: boolean
  grouped?: boolean
  maxVisible?: number
  showArticleCount?: boolean
  showWatch?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  tags: () => [],
  highlightedSlugs: () => [],
  compact: false,
  grouped: false,
  maxVisible: 6,
  showArticleCount: true,
  showWatch: false,
})

const emit = defineEmits<{
  watchToggle: [payload: { id: number; slug: string }]
}>()

const categoryMeta: Record<string, { icon: string; label: string }> = {
  event: { icon: 'mdi:calendar-star', label: '事件' },
  person: { icon: 'mdi:account', label: '人物' },
  keyword: { icon: 'mdi:tag', label: '关键词' },
}

const sortedTags = computed(() => [...props.tags].sort((left, right) => {
  const leftCount = left.articleCount ?? 0
  const rightCount = right.articleCount ?? 0
  if (leftCount === rightCount) {
    return left.label.localeCompare(right.label, 'zh-CN')
  }

  return rightCount - leftCount
}))

const visibleTags = computed(() => {
  if (!props.compact) return sortedTags.value
  return sortedTags.value.slice(0, props.maxVisible)
})

const hiddenCount = computed(() => Math.max(sortedTags.value.length - visibleTags.value.length, 0))

function isHighlighted(tag: ArticleTag) {
  return props.highlightedSlugs.includes(tag.slug)
}

function resolveIcon(tag: ArticleTag) {
  return tag.icon || categoryMeta[tag.category]?.icon || 'mdi:tag'
}

function formatCount(tag: ArticleTag) {
  if (!props.showArticleCount || !tag.articleCount || tag.articleCount < 2) return ''
  return `${tag.articleCount}`
}

function handleWatchClick(tag: ArticleTag) {
  if (tag.id == null) return
  emit('watchToggle', { id: tag.id, slug: tag.slug })
}
</script>

<template>
  <div v-if="visibleTags.length" class="article-tag-list" :class="{ 'article-tag-list--compact': compact }">
    <span
      v-for="tag in visibleTags"
      :key="tag.slug"
      class="article-tag"
      :class="[`article-tag--${tag.category || 'keyword'}`, { 'article-tag--highlighted': isHighlighted(tag) }]"
      :data-tag-slug="tag.slug"
    >
      <Icon :icon="resolveIcon(tag)" width="12" />
      <span>{{ tag.label }}</span>
      <span v-if="formatCount(tag)" class="article-tag__count">{{ formatCount(tag) }}</span>
      <button
        v-if="showWatch && tag.id != null"
        class="article-tag__watch"
        :title="tag.isWatched ? '取消关注' : '关注标签'"
        @click.stop="handleWatchClick(tag)"
      >
        <Icon :icon="tag.isWatched ? 'mdi:heart' : 'mdi:heart-outline'" width="13" />
      </button>
    </span>

    <span v-if="hiddenCount" class="article-tag article-tag--more">+{{ hiddenCount }}</span>
  </div>
</template>

<style scoped>
.article-tag-list {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.article-tag-list--compact {
  gap: 0.4rem;
}

.article-tag {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  min-height: 1.9rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.72);
  padding: 0.25rem 0.72rem;
  color: var(--color-ink-medium);
  font-size: 0.76rem;
  font-weight: 700;
  line-height: 1;
}

.article-tag--event {
  background: rgba(245, 158, 11, 0.12);
  color: #9a5c00;
}

.article-tag--person {
  background: rgba(16, 185, 129, 0.12);
  color: #0d7a56;
}

.article-tag--keyword {
  background: rgba(59, 107, 135, 0.12);
  color: #234d66;
}

.article-tag--highlighted {
  border-color: rgba(193, 47, 47, 0.38);
  background: rgba(193, 47, 47, 0.12);
  color: var(--color-print-red-700);
  box-shadow: inset 0 0 0 1px rgba(193, 47, 47, 0.08);
}

.article-tag__count {
  border-left: 1px solid currentColor;
  padding-left: 0.35rem;
  opacity: 0.75;
}

.article-tag__watch {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  cursor: pointer;
  padding: 0;
  margin-left: -0.1rem;
  color: inherit;
  opacity: 0.75;
  transition: opacity 0.15s;
}

.article-tag__watch:hover {
  opacity: 1;
  color: #c12f2f;
}

.article-tag--more {
  background: rgba(18, 24, 30, 0.06);
  color: var(--color-ink-light);
}
</style>
