<script setup lang="ts">
import { computed, ref } from 'vue'
import { Icon } from '@iconify/vue'
import type { TimelineAggregationGroup, TimelineAggregationArticle } from '~/types/timeline'

interface Props {
  group: TimelineAggregationGroup
  isFirst: boolean
  isLast: boolean
  isActive?: boolean
  aggregationMode: 'day' | 'hour'
}

const props = defineProps<Props>()

const emit = defineEmits<{
  select: [groupKey: string]
  openArticle: [articleId: number]
}>()

const isExpanded = ref(false)

const formattedLabel = computed(() => {
  if (props.aggregationMode === 'hour') {
    const start = props.group.startDate
    const end = props.group.endDate
    const fmt = (d: Date) => `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
    return `${fmt(start)} - ${fmt(end)}`
  }
  return new Intl.DateTimeFormat('zh-CN', {
    month: 'short',
    day: 'numeric',
    weekday: 'short',
  }).format(props.group.startDate)
})

const articleCount = computed(() => props.group.articles.length)

function toggleExpand() {
  isExpanded.value = !isExpanded.value
}

function handleSelect() {
  emit('select', props.group.key)
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    handleSelect()
  }
}

function openArticle(article: TimelineAggregationArticle) {
  emit('openArticle', Number(article.id))
}
</script>

<template>
  <article class="timeline-item" :class="{ 'timeline-item--first': isFirst, 'timeline-item--last': isLast }">
    <div class="timeline-item__marker">
      <div class="timeline-item__dot" />
      <div v-if="!isLast" class="timeline-item__line" />
    </div>

    <div class="timeline-item__content">
      <div class="timeline-item__header">
        <span class="timeline-item__label">{{ formattedLabel }}</span>
        <span class="timeline-item__count-badge">{{ articleCount }} 篇</span>
      </div>

      <div
        class="timeline-item__body"
        :class="{ 'timeline-item__body--active': props.isActive }"
        role="button"
        tabindex="0"
        @click="handleSelect"
        @keydown="handleKeydown"
      >
        <div class="timeline-item__info-row">
          <span class="timeline-item__info-text">{{ articleCount }} 篇文章</span>
          <span class="timeline-item__info-range">{{ formattedLabel }}</span>
        </div>

        <div v-if="isExpanded" class="timeline-item__articles">
          <button
            v-for="article in group.articles"
            :key="article.id"
            type="button"
            class="timeline-item__article-btn"
            @click.stop="openArticle(article)"
          >
            <span class="timeline-item__article-title">{{ article.title }}</span>
            <span class="timeline-item__article-feed">{{ article.feedName }}</span>
          </button>
        </div>
      </div>

      <div class="timeline-item__footer">
        <button
          type="button"
          class="timeline-item__expand"
          @click="toggleExpand"
        >
          <Icon :icon="isExpanded ? 'mdi:chevron-up' : 'mdi:chevron-down'" width="16" />
          <span>{{ isExpanded ? '收起' : '展开文章' }}</span>
        </button>
      </div>
    </div>
  </article>
</template>

<style scoped>
.timeline-item {
  display: grid;
  grid-template-columns: 44px minmax(0, 1fr);
  gap: 0.9rem;
  position: relative;
}

.timeline-item--first .timeline-item__dot {
  background: linear-gradient(135deg, rgba(240, 138, 75, 0.94), rgba(255, 182, 123, 0.98));
  box-shadow: 0 0 12px rgba(240, 138, 75, 0.34);
}

.timeline-item__marker {
  display: flex;
  flex-direction: column;
  align-items: center;
  position: relative;
}

.timeline-item__dot {
  width: 12px;
  height: 12px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.28);
  border: 2px solid rgba(255, 255, 255, 0.16);
  flex-shrink: 0;
}

.timeline-item__line {
  width: 2px;
  flex: 1;
  min-height: 24px;
  margin-top: 4px;
  background: linear-gradient(180deg, rgba(240, 138, 75, 0.28), rgba(255, 255, 255, 0.03));
}

.timeline-item__content {
  display: grid;
  gap: 0.55rem;
  padding-bottom: 1.4rem;
}

.timeline-item--last .timeline-item__content {
  padding-bottom: 0;
}

.timeline-item__header {
  display: flex;
  align-items: center;
  gap: 0.55rem;
  flex-wrap: wrap;
}

.timeline-item__label {
  font-size: 0.75rem;
  font-weight: 600;
  color: rgba(240, 138, 75, 0.92);
  letter-spacing: 0.04em;
}

.timeline-item__count-badge {
  margin-left: auto;
  padding: 0.22rem 0.58rem;
  border-radius: 999px;
  border: 1px solid rgba(240, 138, 75, 0.2);
  background: rgba(240, 138, 75, 0.08);
  font-size: 0.72rem;
  font-weight: 600;
  color: rgba(240, 138, 75, 0.88);
}

.timeline-item__body {
  display: grid;
  gap: 0.72rem;
  text-align: left;
  border-radius: 1.15rem;
  border: 1px solid rgba(141, 173, 214, 0.14);
  background: linear-gradient(180deg, rgba(20, 30, 42, 0.88), rgba(10, 16, 24, 0.94));
  padding: 1rem;
  box-shadow: 0 16px 36px rgba(0, 0, 0, 0.18);
}

.timeline-item__body--active {
  border-color: rgba(240, 138, 75, 0.34);
  background: linear-gradient(180deg, rgba(28, 39, 53, 0.94), rgba(13, 20, 29, 0.98));
  box-shadow: 0 20px 44px rgba(0, 0, 0, 0.24);
}

.timeline-item__info-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  flex-wrap: wrap;
}

.timeline-item__info-text {
  font-size: 0.72rem;
  letter-spacing: 0.12em;
  color: rgba(241, 223, 208, 0.76);
}

.timeline-item__info-range {
  font-size: 0.72rem;
  color: rgba(171, 194, 217, 0.74);
}

.timeline-item__articles {
  display: flex;
  flex-wrap: wrap;
  gap: 0.45rem;
}

.timeline-item__article-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  border-radius: 999px;
  border: 1px solid rgba(240, 138, 75, 0.2);
  background: rgba(255, 255, 255, 0.04);
  padding: 0.34rem 0.68rem;
  color: rgba(255, 231, 213, 0.88);
  font-size: 0.73rem;
  transition: all 0.18s ease;
}

.timeline-item__article-btn:hover,
.timeline-item__article-btn:focus-visible {
  border-color: rgba(240, 138, 75, 0.4);
  background: rgba(240, 138, 75, 0.1);
}

.timeline-item__article-title {
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.timeline-item__article-feed {
  color: rgba(214, 225, 235, 0.5);
  font-size: 0.68rem;
}

.timeline-item__footer {
  display: flex;
  justify-content: flex-end;
  gap: 0.45rem;
  flex-wrap: wrap;
}

.timeline-item__expand {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  border-radius: 999px;
  padding: 0.28rem 0.62rem;
  color: rgba(214, 225, 236, 0.68);
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.08);
  font-size: 0.74rem;
}

.timeline-item__expand:hover,
.timeline-item__expand:focus-visible {
  color: rgba(248, 252, 255, 0.92);
  border-color: rgba(240, 138, 75, 0.28);
}

@media (max-width: 640px) {
  .timeline-item {
    grid-template-columns: 34px minmax(0, 1fr);
    gap: 0.65rem;
  }

  .timeline-item__body {
    padding: 0.85rem;
  }

  .timeline-item__count-badge {
    margin-left: 0;
  }

  .timeline-item__article-title {
    max-width: 140px;
  }
}
</style>
