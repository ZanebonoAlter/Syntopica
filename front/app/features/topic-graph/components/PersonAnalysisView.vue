<script setup lang="ts">
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import type { PersonAnalysis, TrendPoint } from '~/types/ai'

interface Props {
  data: PersonAnalysis
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'view-detail': [section: string]
}>()

const trendMax = computed(() => {
  if (!props.data.trend || props.data.trend.length === 0) return 1
  return Math.max(...props.data.trend.map((point: TrendPoint) => point.value), 1)
})

function handleViewArticle(articleId: number) {
  emit('view-detail', `article-${articleId}`)
}
</script>

<template>
  <div class="person-analysis-view">
    <!-- Summary Section -->
    <section v-if="data.summary" class="analysis-section">
      <h5 class="section-title">
        <Icon icon="mdi:text-box-outline" width="16" />
        <span>分析摘要</span>
      </h5>
      <p class="summary-text">{{ data.summary }}</p>
    </section>

    <!-- Profile Section -->
    <section v-if="data.profile" class="analysis-section">
      <h5 class="section-title">
        <Icon icon="mdi:account-outline" width="16" />
        <span>人物档案</span>
      </h5>
      <div class="profile-card">
        <h6 class="profile-name">{{ data.profile.name }}</h6>
        <p class="profile-role">{{ data.profile.role }}</p>
        <p class="profile-background">{{ data.profile.background }}</p>
      </div>
    </section>

    <!-- Appearances Section -->
    <section v-if="data.appearances && data.appearances.length" class="analysis-section">
      <h5 class="section-title">
        <Icon icon="mdi:timeline-check-outline" width="16" />
        <span>出现记录</span>
      </h5>
      <div class="appearance-list">
        <article
          v-for="(appearance, index) in data.appearances"
          :key="`appearance-${index}`"
          class="appearance-item"
        >
          <div class="appearance-item__date">{{ appearance.date }}</div>
          <div class="appearance-item__content">
            <p class="appearance-item__context">{{ appearance.context }}</p>
            <blockquote v-if="appearance.quote" class="appearance-item__quote">
              "{{ appearance.quote }}"
            </blockquote>
            <button
              type="button"
              class="appearance-item__link"
              @click="handleViewArticle(appearance.articleId)"
            >
              查看原文
            </button>
          </div>
        </article>
      </div>
    </section>

    <!-- Trend Section -->
    <section v-if="data.trend && data.trend.length" class="analysis-section">
      <h5 class="section-title">
        <Icon icon="mdi:chart-line" width="16" />
        <span>出现趋势</span>
      </h5>
      <div class="trend-chart">
        <div class="trend-bars">
          <div
            v-for="(point, index) in data.trend"
            :key="`trend-${index}`"
            class="trend-bar"
            :style="{ height: `${(point.value / trendMax) * 100}%` }"
            :title="`${point.date}: ${point.value}`"
          />
        </div>
        <div class="trend-labels">
          <span>{{ data.trend[0]?.date }}</span>
          <span>{{ data.trend[data.trend.length - 1]?.date }}</span>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.person-analysis-view {
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
  color: rgba(16, 185, 129, 0.8);
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

/* Profile Card */
.profile-card {
  border-radius: 0.75rem;
  border: 1px solid rgba(145, 178, 218, 0.2);
  background: rgba(10, 15, 24, 0.72);
  padding: 0.85rem;
}

.profile-name {
  font-size: 1rem;
  font-weight: 600;
  color: rgba(244, 250, 255, 0.94);
  margin: 0 0 0.35rem;
}

.profile-role {
  font-size: 0.8rem;
  color: rgba(16, 185, 129, 0.9);
  margin: 0 0 0.5rem;
}

.profile-background {
  font-size: 0.85rem;
  line-height: 1.55;
  color: rgba(201, 216, 232, 0.8);
  margin: 0;
}

/* Appearances */
.appearance-list {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.appearance-item {
  display: flex;
  gap: 0.75rem;
  border-radius: 0.75rem;
  border: 1px solid rgba(145, 178, 218, 0.18);
  background: rgba(9, 14, 22, 0.72);
  padding: 0.75rem;
}

.appearance-item__date {
  font-size: 0.7rem;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: rgba(176, 197, 218, 0.72);
  white-space: nowrap;
  min-width: 4.5rem;
}

.appearance-item__content {
  flex: 1;
  min-width: 0;
}

.appearance-item__context {
  font-size: 0.85rem;
  line-height: 1.55;
  color: rgba(214, 227, 239, 0.86);
  margin: 0;
}

.appearance-item__quote {
  margin: 0.5rem 0;
  padding-left: 0.75rem;
  border-left: 2px solid rgba(16, 185, 129, 0.4);
  font-size: 0.85rem;
  font-style: italic;
  color: rgba(110, 231, 183, 0.92);
}

.appearance-item__link {
  background: none;
  border: none;
  padding: 0;
  font-size: 0.8rem;
  color: rgba(16, 185, 129, 0.95);
  cursor: pointer;
  text-decoration: underline;
  text-underline-offset: 2px;
}

.appearance-item__link:hover {
  color: rgba(110, 231, 183, 1);
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
  background: linear-gradient(180deg, rgba(16, 185, 129, 0.95), rgba(90, 151, 231, 0.92));
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

@media (max-width: 640px) {
  .appearance-item {
    flex-direction: column;
    gap: 0.5rem;
  }

  .appearance-item__date {
    min-width: auto;
  }
}
</style>