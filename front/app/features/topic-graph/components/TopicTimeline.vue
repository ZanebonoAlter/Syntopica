<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { TopicCategory } from '~/api/topicGraph'
import type { TimelineAggregationGroup, TimelineAggregationMode } from '~/types/timeline'
import TimelineHeader from './TimelineHeader.vue'
import TimelineItem from './TimelineItem.vue'

interface TopicInfo {
  slug: string
  label: string
  category: TopicCategory
  description?: string
}

interface Props {
  selectedTopic: TopicInfo | null
  groups: TimelineAggregationGroup[]
  activeGroupKey?: string | null
  aggregationMode: TimelineAggregationMode
  totalCount: number
}

const props = withDefaults(defineProps<Props>(), {
  activeGroupKey: null,
})

const emit = defineEmits<{
  'open-article': [articleId: number]
  'select-group': [groupKey: string]
  'update:aggregationMode': [mode: TimelineAggregationMode]
}>()

function handleOpenArticle(articleId: number) {
  emit('open-article', articleId)
}

function handleSelectGroup(groupKey: string) {
  emit('select-group', groupKey)
}

function handleModeChange(mode: TimelineAggregationMode) {
  emit('update:aggregationMode', mode)
}
</script>

<template>
  <div class="topic-timeline">
    <TimelineHeader
      :topic="selectedTopic"
      :total-count="totalCount"
      :aggregation-mode="aggregationMode"
      @update:aggregation-mode="handleModeChange"
    />

    <div class="timeline-content">
      <div v-if="!selectedTopic" class="timeline-empty">
        <Icon icon="mdi:cursor-default-click" width="32" />
        <span>请先选择一个题材查看相关文章</span>
      </div>

      <template v-else>
        <div v-if="groups.length === 0" class="timeline-empty">
          <Icon icon="mdi:file-search" width="32" />
          <span>这个题材在当前窗口里还没有关联文章</span>
        </div>

        <div v-else class="timeline-list">
          <TimelineItem
            v-for="(group, index) in groups"
            :key="group.key"
            :group="group"
            :is-first="index === 0"
            :is-last="index === groups.length - 1"
            :is-active="activeGroupKey === group.key"
            :aggregation-mode="aggregationMode"
            @open-article="handleOpenArticle"
            @select="handleSelectGroup"
          />
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.topic-timeline {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  height: 100%;
}

.timeline-content {
  flex: 1;
  min-height: 0;
  padding-right: 0.25rem;
}

.timeline-empty {
  display: flex;
  min-height: 16rem;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.75rem;
  border-radius: 1.2rem;
  border: 1px dashed rgba(255, 255, 255, 0.1);
  background: rgba(255, 255, 255, 0.02);
  padding: 2rem 1rem;
  color: rgba(255, 255, 255, 0.52);
  text-align: center;
}

.timeline-list {
  display: flex;
  flex-direction: column;
  gap: 0.65rem;
}
</style>
