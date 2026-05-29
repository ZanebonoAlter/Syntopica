<script setup lang="ts">
import { ref, computed } from 'vue'
import { Icon } from '@iconify/vue'
import type { TopicCategory } from '~/api/topicGraph'

interface TimelineItem {
  date: string
  title: string
  summary: string
  sources: Array<{ articleId: number; title: string }>
}

interface KeyMoment {
  moment: string
}

interface RelatedEntity {
  name: string
  type: string
}

interface PersonProfile {
  name: string
  role: string
  background: string
}

interface PersonAppearance {
  date: string
  context: string
  quote: string
  articleId: number
}

interface TrendPoint {
  date: string
  value: number
}

interface CoOccurrenceItem {
  term: string
  count: number
}

interface AnalysisResult {
  timeline?: TimelineItem[]
  keyMoments?: string[]
  relatedEntities?: RelatedEntity[]
  summary?: string
  profile?: PersonProfile
  appearances?: PersonAppearance[]
  trend?: TrendPoint[]
  relatedTopics?: string[]
  coOccurrence?: CoOccurrenceItem[]
  contextExamples?: string[]
}

interface TopicInfo {
  slug: string
  label: string
  category: TopicCategory
}

interface Props {
  selectedTopic: TopicInfo | null
  analysisType: 'event' | 'person' | 'keyword'
  status: 'idle' | 'loading' | 'completed' | 'error'
  progress: number
  result: AnalysisResult | null
  error: string | null
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'start-analysis': []
  'retry': []
  'open-article': [articleId: number]
}>()

const isExpanded = ref(false)

const categoryLabels: Record<TopicCategory, string> = {
  event: '事件分析',
  person: '人物分析',
  keyword: '关键词分析',
}

const statusText = computed(() => {
  switch (props.status) {
    case 'loading':
      return `AI分析中... ${props.progress}%`
    case 'completed':
      return '分析完成'
    case 'error':
      return '分析失败'
    default:
      return '等待开始'
  }
})

const trendMax = computed(() => {
  if (!props.result?.trend || props.result.trend.length === 0) return 1
  return Math.max(...props.result.trend.map(point => point.value), 1)
})

function toggleExpand() {
  isExpanded.value = !isExpanded.value
}

function handleStartAnalysis() {
  emit('start-analysis')
  isExpanded.value = true
}

function handleRetry() {
  emit('retry')
  isExpanded.value = true
}
</script>

<template>
  <div class="ai-analysis-panel" :class="`ai-analysis-panel--${status}`">
    <!-- 头部信息 -->
    <div class="panel-header" @click="toggleExpand">
      <div class="header-icon">
        <Icon icon="mdi:brain" width="20" />
      </div>
      <div class="header-content">
        <h4 class="header-title">
          {{ selectedTopic ? `${selectedTopic.label} - ${categoryLabels[analysisType]}` : 'AI深度分析' }}
        </h4>
        <p class="header-status" :class="`status--${status}`">
          <Icon v-if="status === 'loading'" icon="mdi:loading" class="animate-spin" width="14" />
          <Icon v-else-if="status === 'completed'" icon="mdi:check-circle" width="14" />
          <Icon v-else-if="status === 'error'" icon="mdi:alert-circle" width="14" />
          {{ statusText }}
        </p>
      </div>
      <div class="header-actions">
        <button
          v-if="status === 'idle'"
          type="button"
          class="btn-start"
          @click.stop="handleStartAnalysis"
        >
          <Icon icon="mdi:play" width="16" />
          <span>开始分析</span>
        </button>
        <button
          v-else-if="status === 'error'"
          type="button"
          class="btn-retry"
          @click.stop="handleRetry"
        >
          <Icon icon="mdi:refresh" width="16" />
          <span>重试</span>
        </button>
        <button
          v-if="status === 'completed' || status === 'error'"
          type="button"
          class="btn-toggle"
          :class="{ 'is-expanded': isExpanded }"
          @click.stop="toggleExpand"
        >
          <Icon :icon="isExpanded ? 'mdi:chevron-up' : 'mdi:chevron-down'" width="20" />
        </button>
      </div>
    </div>

    <!-- 进度条 -->
    <div v-if="status === 'loading'" class="progress-bar">
      <div class="progress-bar__fill" :style="{ width: `${Math.min(Math.max(progress, 0), 100)}%` }" />
    </div>

    <!-- 分析结果内容 -->
    <template v-if="result">
      <div v-show="isExpanded" class="panel-content">
        <!-- 分析摘要 -->
        <div v-if="result.summary" class="content-section">
          <h5 class="section-title">
            <Icon icon="mdi:text-box" width="16" />
            <span>分析摘要</span>
          </h5>
          <p class="section-content">{{ result.summary }}</p>
        </div>

        <!-- 事件分析 -->
        <div v-if="analysisType === 'event' && result.timeline" class="content-section">
          <h5 class="section-title">
            <Icon icon="mdi:timeline" width="16" />
            <span>事件脉络</span>
          </h5>
          <div class="timeline-list">
            <div
              v-for="(item, index) in result.timeline"
              :key="`timeline-${index}`"
              class="timeline-item"
            >
              <div class="timeline-item__date">{{ item.date }}</div>
              <div class="timeline-item__card">
                <h6 class="timeline-item__title">{{ item.title }}</h6>
                <p class="timeline-item__summary">{{ item.summary }}</p>
                <div v-if="item.sources?.length" class="timeline-item__sources">
                  <button
                    v-for="source in item.sources"
                    :key="source.articleId"
                    type="button"
                    class="source-link"
                    @click="emit('open-article', source.articleId)"
                  >
                    {{ source.title }}
                  </button>
                </div>
              </div>
            </div>
          </div>

          <!-- 关键节点 -->
          <div v-if="result.keyMoments?.length" class="key-moments">
            <h6 class="subsection-title">关键节点</h6>
            <ul class="key-moments__list">
              <li v-for="(moment, index) in result.keyMoments" :key="`moment-${index}`">
                {{ moment }}
              </li>
            </ul>
          </div>

          <!-- 相关实体 -->
          <div v-if="result.relatedEntities?.length" class="related-entities">
            <h6 class="subsection-title">相关实体</h6>
            <div class="entity-list">
              <span
                v-for="entity in result.relatedEntities"
                :key="entity.name"
                class="entity-tag"
                :class="`entity-tag--${entity.type}`"
              >
                {{ entity.name }}
              </span>
            </div>
          </div>
        </div>

        <!-- 人物分析 -->
        <div v-if="analysisType === 'person'" class="content-section">
          <!-- 人物档案 -->
          <div v-if="result.profile" class="person-profile">
            <h5 class="section-title">
              <Icon icon="mdi:account" width="16" />
              <span>人物档案</span>
            </h5>
            <div class="profile-card">
              <h6 class="profile-card__name">{{ result.profile.name }}</h6>
              <p class="profile-card__role">{{ result.profile.role }}</p>
              <p class="profile-card__background">{{ result.profile.background }}</p>
            </div>
          </div>

          <!-- 出现记录 -->
          <div v-if="result.appearances?.length" class="appearances">
            <h5 class="section-title">
              <Icon icon="mdi:timeline-check" width="16" />
              <span>出现记录</span>
            </h5>
            <div class="appearance-list">
              <div
                v-for="(appearance, index) in result.appearances"
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
                    @click="emit('open-article', appearance.articleId)"
                  >
                    查看原文
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 关键词分析 -->
        <div v-if="analysisType === 'keyword'" class="content-section">
          <!-- 趋势图 -->
          <div v-if="result.trend?.length" class="keyword-trend">
            <h5 class="section-title">
              <Icon icon="mdi:chart-line" width="16" />
              <span>趋势分析</span>
            </h5>
            <div class="trend-chart">
              <div class="trend-bars">
                <div
                  v-for="(point, index) in result.trend"
                  :key="`trend-${index}`"
                  class="trend-bar"
                  :style="{ height: `${(point.value / trendMax) * 100}%` }"
                  :title="`${point.date}: ${point.value}`"
                />
              </div>
              <div class="trend-labels">
                <span>{{ result.trend[0]?.date }}</span>
                <span>{{ result.trend[result.trend.length - 1]?.date }}</span>
              </div>
            </div>
          </div>

          <!-- 相关主题 -->
          <div v-if="result.relatedTopics?.length" class="related-topics">
            <h5 class="section-title">
              <Icon icon="mdi:tag" width="16" />
              <span>相关主题</span>
            </h5>
            <div class="topic-tags">
              <span
                v-for="topic in result.relatedTopics"
                :key="topic"
                class="topic-tag"
              >
                {{ topic }}
              </span>
            </div>
          </div>

          <!-- 共现分析 -->
          <div v-if="result.coOccurrence?.length" class="co-occurrence">
            <h5 class="section-title">
              <Icon icon="mdi:link-variant" width="16" />
              <span>共现分析</span>
            </h5>
            <ul class="co-occurrence-list">
              <li
                v-for="item in result.coOccurrence"
                :key="item.term"
              >
                <span class="co-occurrence__term">{{ item.term }}</span>
                <span class="co-occurrence__count">{{ item.count }}次</span>
              </li>
            </ul>
          </div>

          <!-- 上下文示例 -->
          <div v-if="result.contextExamples?.length" class="context-examples">
            <h5 class="section-title">
              <Icon icon="mdi:format-quote-open" width="16" />
              <span>上下文示例</span>
            </h5>
            <blockquote
              v-for="(example, index) in result.contextExamples"
              :key="`context-${index}`"
              class="context-quote"
            >
              {{ example }}
            </blockquote>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.ai-analysis-panel {
  border-radius: 1.25rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: linear-gradient(180deg, rgba(20, 30, 42, 0.9), rgba(12, 18, 26, 0.95));
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.2);
  overflow: hidden;
}

.ai-analysis-panel--loading {
  border-color: rgba(240, 138, 75, 0.3);
}

.ai-analysis-panel--completed {
  border-color: rgba(16, 185, 129, 0.3);
}

.ai-analysis-panel--error {
  border-color: rgba(239, 68, 68, 0.3);
}

.panel-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 1rem 1.25rem;
  cursor: pointer;
  transition: background 0.15s ease;
}

.panel-header:hover {
  background: rgba(255, 255, 255, 0.02);
}

.header-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 2.5rem;
  height: 2.5rem;
  border-radius: 0.75rem;
  background: linear-gradient(135deg, rgba(240, 138, 75, 0.2), rgba(255, 160, 100, 0.15));
  color: rgba(255, 200, 150, 0.95);
}

.header-content {
  flex: 1;
  min-width: 0;
}

.header-title {
  font-size: 0.95rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.95);
  margin: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.header-status {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.75rem;
  color: rgba(255, 255, 255, 0.5);
  margin: 0.25rem 0 0;
}

.status--loading {
  color: rgba(240, 138, 75, 0.9);
}

.status--completed {
  color: rgba(16, 185, 129, 0.9);
}

.status--error {
  color: rgba(239, 68, 68, 0.9);
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.btn-start,
.btn-retry {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.8rem;
  padding: 0.5rem 0.9rem;
  border-radius: 999px;
  border: 1px solid rgba(240, 138, 75, 0.4);
  background: linear-gradient(135deg, rgba(240, 138, 75, 0.2), rgba(255, 160, 100, 0.15));
  color: rgba(255, 200, 150, 0.95);
  cursor: pointer;
  transition: all 0.2s ease;
}

.btn-start:hover,
.btn-retry:hover {
  border-color: rgba(240, 138, 75, 0.6);
  background: linear-gradient(135deg, rgba(240, 138, 75, 0.3), rgba(255, 160, 100, 0.25));
  transform: translateY(-1px);
}

.btn-toggle {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 2rem;
  height: 2rem;
  border-radius: 0.5rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(255, 255, 255, 0.05);
  color: rgba(255, 255, 255, 0.6);
  cursor: pointer;
  transition: all 0.2s ease;
}

.btn-toggle:hover {
  border-color: rgba(255, 255, 255, 0.2);
  color: rgba(255, 255, 255, 0.8);
}

.btn-toggle.is-expanded {
  background: rgba(255, 255, 255, 0.1);
}

.progress-bar {
  height: 0.25rem;
  background: rgba(255, 255, 255, 0.08);
}

.progress-bar__fill {
  height: 100%;
  background: linear-gradient(90deg, rgba(240, 138, 75, 0.95), rgba(90, 151, 231, 0.92));
  transition: width 0.25s ease;
}

.panel-content {
  padding: 0 1.25rem 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.content-section {
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

.section-content {
  font-size: 0.85rem;
  line-height: 1.6;
  color: rgba(255, 255, 255, 0.7);
  margin: 0;
}

/* Timeline styles */
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

/* Key moments */
.key-moments {
  margin-top: 0.5rem;
}

.subsection-title {
  font-size: 0.75rem;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: rgba(174, 194, 216, 0.72);
  margin: 0 0 0.5rem;
}

.key-moments__list {
  margin: 0;
  padding-left: 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.key-moments__list li {
  font-size: 0.8rem;
  color: rgba(255, 255, 255, 0.75);
  line-height: 1.5;
}

/* Related entities */
.related-entities {
  margin-top: 0.5rem;
}

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

/* Person profile */
.person-profile {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.profile-card {
  border-radius: 0.75rem;
  border: 1px solid rgba(145, 178, 218, 0.2);
  background: rgba(10, 15, 24, 0.72);
  padding: 0.85rem;
}

.profile-card__name {
  font-size: 1rem;
  font-weight: 600;
  color: rgba(244, 250, 255, 0.94);
  margin: 0 0 0.35rem;
}

.profile-card__role {
  font-size: 0.8rem;
  color: rgba(240, 138, 75, 0.9);
  margin: 0 0 0.5rem;
}

.profile-card__background {
  font-size: 0.85rem;
  line-height: 1.55;
  color: rgba(201, 216, 232, 0.8);
  margin: 0;
}

/* Appearances */
.appearances {
  margin-top: 0.75rem;
}

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
  border-left: 2px solid rgba(240, 138, 75, 0.4);
  font-size: 0.85rem;
  font-style: italic;
  color: rgba(255, 227, 206, 0.92);
}

.appearance-item__link {
  background: none;
  border: none;
  padding: 0;
  font-size: 0.8rem;
  color: rgba(255, 200, 150, 0.95);
  cursor: pointer;
  text-decoration: underline;
  text-underline-offset: 2px;
}

.appearance-item__link:hover {
  color: rgba(255, 220, 180, 1);
}

/* Keyword trend */
.keyword-trend {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

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
  background: linear-gradient(180deg, rgba(240, 138, 75, 0.95), rgba(90, 151, 231, 0.92));
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

/* Related topics */
.related-topics {
  margin-top: 0.75rem;
}

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
}

/* Co-occurrence */
.co-occurrence {
  margin-top: 0.75rem;
}

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
  color: rgba(240, 138, 75, 0.9);
}

/* Context examples */
.context-examples {
  margin-top: 0.75rem;
}

.context-quote {
  margin: 0.5rem 0;
  padding: 0.75rem;
  border-radius: 0.6rem;
  border: 1px solid rgba(145, 178, 218, 0.18);
  background: rgba(10, 15, 23, 0.7);
  font-size: 0.85rem;
  line-height: 1.55;
  color: rgba(219, 231, 243, 0.86);
}

.context-quote + .context-quote {
  margin-top: 0.5rem;
}

@media (max-width: 640px) {
  .panel-header {
    padding: 0.85rem 1rem;
  }

  .header-title {
    font-size: 0.85rem;
  }

  .panel-content {
    padding: 0 1rem 1rem;
  }

  .appearance-item {
    flex-direction: column;
    gap: 0.5rem;
  }
}
</style>