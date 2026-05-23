<script setup lang="ts">
import { Icon } from '@iconify/vue'

import type { NarrativeItem, BoardNarrativeItem } from '~/api/topicGraph'

interface NarrativeTag {
  id: number
  slug: string
  label: string
  category: string
  kind?: string
}

interface Props {
  narrative: NarrativeItem | BoardNarrativeItem
  expanded: boolean
  statusStyle: Record<string, { label: string; dot: string; ring: string; bg: string; border: string }>
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'select-tag': [tag: NarrativeTag]
  'toggle-expand': []
  'close': []
}>()
</script>

<template>
  <div class="narrative-detail">
    <div class="narrative-detail__head">
      <h4 class="narrative-detail__title">{{ narrative.title }}</h4>
      <span
        class="narrative-detail__status"
        :style="{
          color: statusStyle[narrative.status]?.dot,
          background: statusStyle[narrative.status]?.bg,
          borderColor: statusStyle[narrative.status]?.border,
        }"
      >
        {{ statusStyle[narrative.status]?.label }}
      </span>
      <button type="button" class="narrative-detail__close" @click.stop="emit('close')">
        <Icon icon="mdi:close" width="14" />
      </button>
    </div>

    <div class="narrative-detail__summary">
      <p v-if="!expanded && narrative.summary.length > 240" class="narrative-detail__text">
        {{ narrative.summary.slice(0, 240) }}...
      </p>
      <p v-else class="narrative-detail__text">{{ narrative.summary }}</p>
      <button
        v-if="narrative.summary.length > 240"
        type="button"
        class="narrative-detail__expand"
        @click="emit('toggle-expand')"
      >
        {{ expanded ? '收起' : '展开全文' }}
      </button>
    </div>

    <div v-if="narrative.related_tags.length" class="narrative-detail__tags">
      <button
        v-for="tag in narrative.related_tags"
        :key="tag.id"
        type="button"
        class="narrative-detail__tag"
        
        @click="emit('select-tag', tag)"
      >
        {{ tag.label }}
      </button>
    </div>

    <div class="narrative-detail__meta">
      <span v-if="narrative.generation > 0" class="narrative-detail__meta-item">
        <Icon icon="mdi:source-branch" width="12" />
        第 {{ narrative.generation }} 代
      </span>
      <span class="narrative-detail__meta-item">{{ narrative.period_date }}</span>
      <span v-if="narrative.parent_ids.length" class="narrative-detail__meta-item">
        <Icon icon="mdi:arrow-left-top" width="12" />
        继承 {{ narrative.parent_ids.length }} 条
      </span>
      <span v-if="narrative.child_ids.length" class="narrative-detail__meta-item">
        <Icon icon="mdi:arrow-right-bottom" width="12" />
        衍生 {{ narrative.child_ids.length }} 条
      </span>
    </div>
  </div>
</template>

<style scoped>
.narrative-detail {
  margin-top: 0.75rem;
  border-radius: 14px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: linear-gradient(180deg, rgba(20, 29, 40, 0.96), rgba(12, 18, 26, 0.98));
  padding: 1rem 1.1rem;
  backdrop-filter: blur(14px);
  box-shadow: 0 10px 40px rgba(0, 0, 0, 0.45);
}

.narrative-detail__head {
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;
}

.narrative-detail__title {
  flex: 1;
  font-size: 0.9rem;
  font-weight: 600;
  line-height: 1.5;
  color: rgba(241, 247, 252, 0.92);
}

.narrative-detail__status {
  flex-shrink: 0;
  padding: 0.15rem 0.5rem;
  border-radius: 999px;
  border: 1px solid;
  font-size: 0.66rem;
  font-weight: 500;
}

.narrative-detail__close {
  flex-shrink: 0;
  border: none;
  background: none;
  color: rgba(255, 255, 255, 0.35);
  cursor: pointer;
  padding: 0.15rem;
  transition: color 0.15s ease;
}

.narrative-detail__close:hover {
  color: rgba(255, 255, 255, 0.7);
}

.narrative-detail__summary {
  margin-top: 0.6rem;
}

.narrative-detail__text {
  font-size: 0.82rem;
  line-height: 1.7;
  color: rgba(186, 206, 226, 0.72);
}

.narrative-detail__expand {
  display: inline-flex;
  align-items: center;
  margin-top: 0.2rem;
  border: none;
  background: none;
  color: rgba(240, 138, 75, 0.72);
  font-size: 0.75rem;
  cursor: pointer;
  padding: 0;
  transition: color 0.15s ease;
}

.narrative-detail__expand:hover {
  color: rgba(240, 138, 75, 1);
}

.narrative-detail__tags {
  display: flex;
  flex-wrap: wrap;
  gap: 0.3rem;
  margin-top: 0.6rem;
}

.narrative-detail__tag {
  padding: 0.15rem 0.45rem;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.6);
  font-size: 0.68rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.narrative-detail__tag:hover {
  border-color: rgba(240, 138, 75, 0.35);
  color: rgba(255, 220, 200, 0.9);
  background: rgba(240, 138, 75, 0.1);
}

.narrative-detail__meta {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-top: 0.6rem;
  font-size: 0.7rem;
  color: rgba(255, 255, 255, 0.35);
}

.narrative-detail__meta-item {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
}
</style>
