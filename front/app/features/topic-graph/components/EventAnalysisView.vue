<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { EventAnalysis, EventTimelineItem, RelatedEntity } from '~/types/ai'

interface Props {
  data: EventAnalysis
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'view-detail': [section: string]
}>()

function handleViewSource(articleId: number) {
  emit('view-detail', `article-${articleId}`)
}

function getEntityTypeLabel(type: RelatedEntity['type']): string {
  const labels: Record<string, string> = {
    person: '人物',
    organization: '组织',
    location: '地点',
    concept: '概念',
  }
  return labels[type] || type
}

function getEntityTypeClass(type: RelatedEntity['type']): string {
  const classes: Record<string, string> = {
    person: 'entity-tag--person',
    organization: 'entity-tag--organization',
    location: 'entity-tag--location',
    concept: 'entity-tag--concept',
  }
  return classes[type] || ''
}
</script>

<template>
  <div class="event-analysis-view">
    <!-- Summary Section -->
    <section v-if="data.summary" class="analysis-section">
      <h5 class="section-title">
        <Icon icon="mdi:text-box-outline" width="16" />
        <span>分析摘要</span>
      </h5>
      <p class="summary-text">{{ data.summary }}</p>
    </section>

    <!-- Timeline Section -->
    <section v-if="data.timeline && data.timeline.length" class="analysis-section">
      <h5 class="section-title">
        <Icon icon="mdi:timeline-clock-outline" width="16" />
        <span>事件脉络</span>
      </h5>
      <div class="timeline-list">
        <div
          v-for="(item, index) in data.timeline"
          :key="`timeline-${index}`"
          class="timeline-item"
        >
          <div class="timeline-item__date">{{ item.date }}</div>
          <div class="timeline-item__card">
            <h6 class="timeline-item__title">{{ item.title }}</h6>
            <p class="timeline-item__summary">{{ item.summary }}</p>
            <div v-if="item.sources && item.sources.length" class="timeline-item__sources">
              <button
                v-for="source in item.sources"
                :key="source.articleId"
                type="button"
                class="source-link"
                @click="handleViewSource(source.articleId)"
              >
                {{ source.title }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- Key Moments Section -->
    <section v-if="data.keyMoments && data.keyMoments.length" class="analysis-section">
      <h5 class="section-title">
        <Icon icon="mdi:star-outline" width="16" />
        <span>关键节点</span>
      </h5>
      <ul class="key-moments-list">
        <li v-for="(moment, index) in data.keyMoments" :key="`moment-${index}`">
          {{ moment }}
        </li>
      </ul>
    </section>

    <!-- Related Entities Section -->
    <section v-if="data.relatedEntities && data.relatedEntities.length" class="analysis-section">
      <h5 class="section-title">
        <Icon icon="mdi:link-variant" width="16" />
        <span>相关实体</span>
      </h5>
      <div class="entity-list">
        <span
          v-for="entity in data.relatedEntities"
          :key="entity.name"
          class="entity-tag"
          :class="getEntityTypeClass(entity.type)"
          :title="getEntityTypeLabel(entity.type)"
        >
          {{ entity.name }}
        </span>
      </div>
    </section>
  </div>
</template>

<style scoped>
.event-analysis-view {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.analysis-section {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.9rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.9);
  margin: 0;
}

.section-title svg {
  color: rgba(240, 138, 75, 0.8);
}

.summary-text {
  font-size: 0.85rem;
  line-height: 1.6;
  color: rgba(255, 255, 255, 0.7);
  margin: 0;
  padding: 0.75rem;
  border-radius: 0.75rem;
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid rgba(255, 255, 255, 0.06);
}

/* Timeline Styles */
.timeline-list {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.timeline-list::before {
  content: '';
  position: absolute;
  left: 0.5rem;
  top: 0.5rem;
  bottom: 0.5rem;
  width: 1px;
  background: linear-gradient(180deg, rgba(240, 138, 75, 0.5), rgba(90, 151, 231, 0.14), rgba(90, 151, 231, 0));
}

.timeline-item {
  position: relative;
  padding-left: 1.5rem;
}

.timeline-item::before {
  content: '';
  position: absolute;
  left: 0.25rem;
  top: 0.35rem;
  width: 0.5rem;
  height: 0.5rem;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.5);
  background: linear-gradient(180deg, rgba(240, 138, 75, 0.95), rgba(90, 151, 231, 0.92));
}

.timeline-item__date {
  font-size: 0.7rem;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: rgba(181, 199, 218, 0.72);
  margin-bottom: 0.35rem;
}

.timeline-item__card {
  border-radius: 0.75rem;
  border: 1px solid rgba(145, 178, 218, 0.2);
  background: rgba(9, 15, 23, 0.75);
  padding: 0.75rem;
}

.timeline-item__title {
  font-size: 0.85rem;
  font-weight: 600;
  color: rgba(246, 251, 255, 0.96);
  margin: 0 0 0.35rem;
}

.timeline-item__summary {
  font-size: 0.8rem;
  line-height: 1.55;
  color: rgba(201, 216, 232, 0.8);
  margin: 0;
}

.timeline-item__sources {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
  margin-top: 0.5rem;
}

.source-link {
  border-radius: 999px;
  border: 1px solid rgba(240, 138, 75, 0.24);
  background: rgba(255, 255, 255, 0.04);
  padding: 0.2rem 0.5rem;
  font-size: 0.7rem;
  color: rgba(255, 231, 213, 0.86);
  cursor: pointer;
  transition: all 0.15s ease;
}

.source-link:hover {
  border-color: rgba(240, 138, 75, 0.4);
  background: rgba(240, 138, 75, 0.1);
}

/* Key Moments */
.key-moments-list {
  margin: 0;
  padding-left: 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.key-moments-list li {
  font-size: 0.8rem;
  color: rgba(255, 255, 255, 0.75);
  line-height: 1.5;
}

/* Related Entities */
.entity-list {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
}

.entity-tag {
  border-radius: 999px;
  padding: 0.25rem 0.6rem;
  font-size: 0.75rem;
  font-weight: 500;
}

.entity-tag--person {
  background: rgba(16, 185, 129, 0.2);
  border: 1px solid rgba(16, 185, 129, 0.4);
  color: rgba(110, 231, 183, 0.9);
}

.entity-tag--organization {
  background: rgba(99, 102, 241, 0.2);
  border: 1px solid rgba(99, 102, 241, 0.4);
  color: rgba(165, 180, 252, 0.9);
}

.entity-tag--location {
  background: rgba(245, 158, 11, 0.2);
  border: 1px solid rgba(245, 158, 11, 0.4);
  color: rgba(252, 211, 77, 0.9);
}

.entity-tag--concept {
  background: rgba(139, 92, 246, 0.2);
  border: 1px solid rgba(139, 92, 246, 0.4);
  color: rgba(196, 181, 253, 0.9);
}

@media (max-width: 640px) {
  .timeline-item__card {
    padding: 0.6rem;
  }

  .timeline-item__title {
    font-size: 0.8rem;
  }

  .timeline-item__summary {
    font-size: 0.75rem;
  }
}
</style>