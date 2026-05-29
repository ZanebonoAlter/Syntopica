<script setup lang="ts">
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import type { KeywordAnalysis, TrendPoint } from '~/types/ai'

interface Props {
  data: KeywordAnalysis
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'view-detail': [section: string]
}>()

const trendMax = computed(() => {
  if (!props.data.trendData || props.data.trendData.length === 0) return 1
  return Math.max(...props.data.trendData.map((point: TrendPoint) => point.value), 1)
})

function handleViewArticle(articleId: number) {
  emit('view-detail', `article-${articleId}`)
}
</script>

<template>
  <div class="keyword-analysis-view">
    <!-- Summary Section -->
    <section v-if="data.summary" class="analysis-section">
      <h5 class="section-title">
        <Icon icon="mdi:text-box-outline" width="16" />
        <span>分析摘要</span>
      </h5>
      <p class="summary-text">{{ data.summary }}</p>
    </section>

    <!-- Trend Section -->
    <section v-if="data.trendData && data.trendData.length" class="analysis-section">
      <h5 class="section-title">
        <Icon icon="mdi:chart-line" width="16" />
        <span>趋势分析</span>
      </h5>
      <div class="trend-chart">
        <div class="trend-bars">
          <div
            v-for="(point, index) in data.trendData"
            :key="`trend-${index}`"
            class="trend-bar"
            :style="{ height: `${(point.value / trendMax) * 100}%` }"
            :title="`${point.date}: ${point.value}`"
          />
        </div>
        <div class="trend-labels">
          <span>{{ data.trendData[0]?.date }}</span>
          <span>{{ data.trendData[data.trendData.length - 1]?.date }}</span>
        </div>
      </div>
    </section>

    <!-- Related Topics Section -->
    <section v-if="data.relatedTopics && data.relatedTopics.length" class="analysis-section">
      <h5 class="section-title">
        <Icon icon="mdi:tag-multiple-outline" width="16" />
        <span>相关主题</span>
      </h5>
      <div class="topic-tags">
        <span
          v-for="topic in data.relatedTopics"
          :key="topic.slug"
          class="topic-tag"
          :title="`相关度: ${(topic.score * 100).toFixed(1)}%`"
        >
          {{ topic.label }}
        </span>
      </div>
    </section>

    <!-- Co-occurrence Section -->
    <section v-if="data.coOccurrence && data.coOccurrence.length" class="analysis-section">
      <h5 class="section-title">
        <Icon icon="mdi:link-variant" width="16" />
        <span>共现分析</span>
      </h5>
      <ul class="co-occurrence-list">
        <li
          v-for="item in data.coOccurrence"
          :key="item.term"
        >
          <span class="co-occurrence__term">{{ item.term }}</span>
          <span class="co-occurrence__count">{{ item.count }}次</span>
        </li>
      </ul>
    </section>

    <!-- Context Examples Section -->
    <section v-if="data.contextExamples && data.contextExamples.length" class="analysis-section">
      <h5 class="section-title">
        <Icon icon="mdi:format-quote-open" width="16" />
        <span>上下文示例</span>
      </h5>
      <div class="context-examples">
        <blockquote
          v-for="(example, index) in data.contextExamples"
          :key="`context-${index}`"
          class="context-quote"
        >
          <p class="context-text">{{ example.text }}</p>
          <footer v-if="example.source" class="context-source">
            <Icon icon="mdi:source-branch" width="12" />
            <span>{{ example.source }}</span>
          </footer>
        </blockquote>
      </div>
    </section>
  </div>
</template>

<style scoped>
.keyword-analysis-view {
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
  color: rgba(245, 158, 11, 0.8);
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

/* Trend Chart */
.trend-chart {
  border-radius: 0.75rem;
  border: 1px solid rgba(145, 178, 218, 0.2);
  background: rgba(10, 15, 23, 0.7);
  padding: 1rem;
}

.trend-bars {
  display: flex;
  align-items: flex-end;
  gap: 0.25rem;
  height: 4rem;
}

.trend-bar {
  flex: 1;
  border-radius: 0.25rem 0.25rem 0 0;
  background: linear-gradient(180deg, rgba(245, 158, 11, 0.95), rgba(90, 151, 231, 0.92));
  min-height: 0.25rem;
  transition: height 0.3s ease;
}

.trend-labels {
  display: flex;
  justify-content: space-between;
  margin-top: 0.5rem;
  font-size: 0.7rem;
  color: rgba(203, 219, 234, 0.82);
}

/* Related Topics */
.topic-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
}

.topic-tag {
  border-radius: 999px;
  border: 1px solid rgba(145, 178, 218, 0.22);
  padding: 0.3rem 0.65rem;
  font-size: 0.75rem;
  color: rgba(233, 243, 253, 0.88);
  cursor: default;
  transition: all 0.15s ease;
}

.topic-tag:hover {
  border-color: rgba(245, 158, 11, 0.4);
  background: rgba(245, 158, 11, 0.1);
}

/* Co-occurrence */
.co-occurrence-list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.co-occurrence-list li {
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-radius: 0.6rem;
  border: 1px solid rgba(145, 178, 218, 0.18);
  background: rgba(10, 15, 23, 0.7);
  padding: 0.5rem 0.75rem;
}

.co-occurrence__term {
  font-size: 0.85rem;
  color: rgba(219, 231, 243, 0.86);
}

.co-occurrence__count {
  font-size: 0.75rem;
  color: rgba(245, 158, 11, 0.9);
}

/* Context Examples */
.context-examples {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.context-quote {
  margin: 0;
  padding: 0.75rem;
  border-radius: 0.6rem;
  border: 1px solid rgba(145, 178, 218, 0.18);
  background: rgba(10, 15, 23, 0.7);
}

.context-text {
  font-size: 0.85rem;
  line-height: 1.55;
  color: rgba(219, 231, 243, 0.86);
  margin: 0;
}

.context-source {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  margin-top: 0.5rem;
  font-size: 0.75rem;
  color: rgba(170, 192, 214, 0.74);
}

.context-source svg {
  color: rgba(170, 192, 214, 0.6);
}

@media (max-width: 640px) {
  .trend-bars {
    height: 3rem;
  }
}
</style>