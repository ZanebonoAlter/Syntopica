<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { computed } from 'vue'

type TopicAnalysisType = 'event' | 'person' | 'keyword'

type TopicAnalysisData = Record<string, unknown>

interface TimelineItem {
  date: string
  title: string
  summary?: string
  source_articles?: Array<{
    id?: number | string
    title?: string
    link?: string
  }>
}

interface PersonAppearance {
  date?: string
  scene?: string
  quote?: string
  article_title?: string
  article_link?: string
}

interface TrendPoint {
  date: string
  count: number
}

interface Props {
  analysisType: TopicAnalysisType
  data: TopicAnalysisData | null
  loading?: boolean
  error?: string | null
  analysisStatus?: 'idle' | 'pending' | 'processing' | 'completed' | 'failed' | 'missing'
  analysisProgress?: number
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
  error: null,
  analysisStatus: 'idle',
  analysisProgress: 0,
})

const emit = defineEmits<{
  retry: []
}>()

const timeline = computed(() => {
  const value = props.data?.timeline
  return Array.isArray(value) ? value as TimelineItem[] : []
})

const personProfile = computed(() => {
  const value = props.data?.profile
  if (!value || typeof value !== 'object') return null
  return value as Record<string, string>
})

const appearances = computed(() => {
  const value = props.data?.appearances
  return Array.isArray(value) ? value as PersonAppearance[] : []
})

const trendData = computed(() => {
  const value = props.data?.trend_data
  return Array.isArray(value) ? value as TrendPoint[] : []
})

const trendMax = computed(() => Math.max(...trendData.value.map(point => point.count), 1))

const relatedTopics = computed(() => {
  const value = props.data?.related_topics
  return Array.isArray(value) ? value as string[] : []
})

const cooccurrence = computed(() => {
  const value = props.data?.cooccurrence ?? props.data?.co_occurrence
  return Array.isArray(value) ? value as Array<{ keyword: string; score?: number }> : []
})

const contexts = computed(() => {
  const value = props.data?.contexts ?? props.data?.context_examples
  return Array.isArray(value) ? value as Array<{ text: string; source?: string }> : []
})
</script>

<template>
  <article class="topic-analysis-panel rounded-[24px] p-4 md:p-5" data-testid="analysis-panel">
    <div
      v-if="props.analysisStatus === 'pending' || props.analysisStatus === 'processing'"
      class="panel-state panel-state--loading panel-state--stack"
      data-testid="analysis-status"
    >
      <Icon icon="mdi:loading" class="panel-loading-icon" width="20" />
      <span>AI 正在分析中...</span>
      <div class="analysis-progress">
        <div class="analysis-progress__fill" :style="{ width: `${Math.min(Math.max(props.analysisProgress, 0), 100)}%` }" />
      </div>
    </div>

    <div v-else-if="props.analysisStatus === 'failed'" class="panel-state panel-state--error panel-state--stack">
      <div class="flex items-center gap-2">
        <Icon icon="mdi:alert-circle-outline" width="18" />
        <span>{{ props.error || '分析失败' }}</span>
      </div>
      <button class="panel-retry" type="button" @click="emit('retry')">重试分析</button>
    </div>

    <div v-else-if="props.loading" class="panel-state panel-state--loading">
      <Icon icon="mdi:loading" class="panel-loading-icon" width="20" />
      <span>正在分析...</span>
    </div>

    <div v-else-if="props.error && !props.data" class="panel-state panel-state--error">
      <Icon icon="mdi:alert-circle-outline" width="18" />
      <span>{{ props.error }}</span>
    </div>

    <div v-else-if="!props.data" class="panel-state panel-state--empty">
      <Icon icon="mdi:file-search-outline" width="18" />
      <span>暂无分析数据</span>
    </div>

    <div v-else-if="props.analysisType === 'event'" class="panel-content panel-content--event">
      <header class="panel-header">
        <h3>事件时间线</h3>
      </header>

      <div v-if="timeline.length" class="event-timeline">
        <div v-for="(item, index) in timeline" :key="`${item.date}-${index}`" class="event-timeline__item">
          <div class="event-timeline__date">{{ item.date || '未知日期' }}</div>
          <div class="event-timeline__card">
            <h4>{{ item.title || '未命名事件' }}</h4>
            <p v-if="item.summary">{{ item.summary }}</p>

            <div v-if="item.source_articles?.length" class="event-timeline__sources">
              <a
                v-for="(source, sourceIndex) in item.source_articles"
                :key="source.id || source.link || sourceIndex"
                :href="source.link"
                target="_blank"
                rel="noopener noreferrer"
                class="event-timeline__source"
              >
                {{ source.title || `来源 ${sourceIndex + 1}` }}
              </a>
            </div>
          </div>
        </div>
      </div>
      <p v-else class="panel-hint">暂无可展示的时间线节点。</p>
    </div>

    <div v-else-if="props.analysisType === 'person'" class="panel-content panel-content--person">
      <header class="panel-header">
        <h3>人物档案</h3>
      </header>

      <section v-if="personProfile" class="person-profile">
        <div v-for="(value, key) in personProfile" :key="key" class="person-profile__item">
          <dt>{{ key }}</dt>
          <dd>{{ value || '暂无' }}</dd>
        </div>
      </section>

      <section>
        <h4 class="panel-subtitle">出现记录</h4>
        <div v-if="appearances.length" class="person-appearances">
          <article v-for="(item, index) in appearances" :key="`${item.date}-${index}`" class="person-appearances__item">
            <div class="person-appearances__meta">
              <span>{{ item.date || '未知日期' }}</span>
              <span v-if="item.scene">{{ item.scene }}</span>
            </div>
            <p v-if="item.quote" class="person-appearances__quote">{{ item.quote }}</p>
            <a
              v-if="item.article_link"
              :href="item.article_link"
              target="_blank"
              rel="noopener noreferrer"
              class="person-appearances__article"
            >
              {{ item.article_title || '查看相关文章' }}
            </a>
          </article>
        </div>
        <p v-else class="panel-hint">暂无人物出现记录。</p>
      </section>
    </div>

    <div v-else class="panel-content panel-content--keyword">
      <header class="panel-header">
        <h3>关键词趋势</h3>
      </header>

      <section>
        <h4 class="panel-subtitle">趋势</h4>
        <div v-if="trendData.length" class="keyword-trend">
          <div v-for="(point, index) in trendData" :key="`${point.date}-${index}`" class="keyword-trend__item">
            <span class="keyword-trend__date">{{ point.date }}</span>
            <div class="keyword-trend__bar-track">
              <span
                class="keyword-trend__bar"
                :style="{ width: `${Math.max((point.count / trendMax) * 100, 8)}%` }"
              />
            </div>
            <span class="keyword-trend__value">{{ point.count }}</span>
          </div>
        </div>
        <p v-else class="panel-hint">暂无趋势数据。</p>
      </section>

      <section>
        <h4 class="panel-subtitle">相关主题</h4>
        <div v-if="relatedTopics.length" class="keyword-pills">
          <span v-for="(topic, index) in relatedTopics" :key="`${topic}-${index}`" class="keyword-pill">{{ topic }}</span>
        </div>
        <p v-else class="panel-hint">暂无相关主题。</p>
      </section>

      <section>
        <h4 class="panel-subtitle">共现分析</h4>
        <div v-if="cooccurrence.length" class="keyword-cooccurrence">
          <article v-for="(item, index) in cooccurrence" :key="`${item.keyword}-${index}`" class="keyword-cooccurrence__item">
            <span>{{ item.keyword }}</span>
            <span v-if="item.score !== undefined">{{ item.score.toFixed(2) }}</span>
          </article>
        </div>
        <p v-else class="panel-hint">暂无共现分析结果。</p>
      </section>

      <section>
        <h4 class="panel-subtitle">上下文示例</h4>
        <div v-if="contexts.length" class="keyword-contexts">
          <article v-for="(item, index) in contexts" :key="`${item.text}-${index}`" class="keyword-contexts__item">
            <p>{{ item.text }}</p>
            <p v-if="item.source" class="keyword-contexts__source">{{ item.source }}</p>
          </article>
        </div>
        <p v-else class="panel-hint">暂无上下文示例。</p>
      </section>
    </div>
  </article>
</template>

<style scoped>
.topic-analysis-panel {
  border: 1px solid rgba(255, 255, 255, 0.08);
  background:
    radial-gradient(circle at 14% 18%, rgba(240, 138, 75, 0.11), transparent 30%),
    linear-gradient(180deg, rgba(14, 22, 31, 0.9), rgba(8, 13, 20, 0.96));
  box-shadow: 0 20px 52px rgba(3, 8, 14, 0.28);
}

.panel-state {
  display: inline-flex;
  min-height: 5rem;
  align-items: center;
  gap: 0.55rem;
  border-radius: 0.95rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  padding: 0.85rem 1rem;
  color: rgba(221, 231, 241, 0.86);
}

.panel-state--stack {
  width: 100%;
  display: grid;
  gap: 0.55rem;
}

.panel-state--error {
  border-color: rgba(240, 138, 75, 0.28);
  color: rgba(255, 221, 206, 0.96);
}

.panel-loading-icon {
  animation: panel-spin 1s linear infinite;
}

.analysis-progress {
  width: 100%;
  height: 0.45rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.08);
  overflow: hidden;
}

.analysis-progress__fill {
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, rgba(240, 138, 75, 0.95), rgba(90, 151, 231, 0.92));
  transition: width 0.25s ease;
}

.panel-retry {
  justify-self: start;
  border-radius: 0.7rem;
  border: 1px solid rgba(240, 138, 75, 0.38);
  background: rgba(240, 138, 75, 0.16);
  color: rgba(255, 234, 221, 0.95);
  font-size: 0.78rem;
  letter-spacing: 0.04em;
  padding: 0.38rem 0.7rem;
}

@keyframes panel-spin {
  from {
    transform: rotate(0deg);
  }

  to {
    transform: rotate(360deg);
  }
}

.panel-content {
  display: grid;
  gap: 1rem;
}

.panel-header h3 {
  font-size: 1.15rem;
  font-weight: 650;
  color: rgba(246, 251, 255, 0.95);
}

.panel-subtitle {
  margin-bottom: 0.55rem;
  font-size: 0.76rem;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: rgba(174, 194, 216, 0.72);
}

.panel-hint {
  border-radius: 0.9rem;
  border: 1px dashed rgba(255, 255, 255, 0.16);
  padding: 0.75rem 0.9rem;
  color: rgba(189, 204, 220, 0.72);
  font-size: 0.88rem;
}

.event-timeline {
  position: relative;
  display: grid;
  gap: 0.9rem;
}

.event-timeline::before {
  content: '';
  position: absolute;
  left: 0.65rem;
  top: 0.5rem;
  bottom: 0.5rem;
  width: 1px;
  background: linear-gradient(180deg, rgba(240, 138, 75, 0.5), rgba(90, 151, 231, 0.14), rgba(90, 151, 231, 0));
}

.event-timeline__item {
  position: relative;
  display: grid;
  gap: 0.45rem;
  padding-left: 1.75rem;
}

.event-timeline__item::before {
  content: '';
  position: absolute;
  left: 0.35rem;
  top: 0.34rem;
  width: 0.62rem;
  height: 0.62rem;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.5);
  background: linear-gradient(180deg, rgba(240, 138, 75, 0.95), rgba(90, 151, 231, 0.92));
}

.event-timeline__date {
  font-size: 0.72rem;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: rgba(181, 199, 218, 0.72);
}

.event-timeline__card {
  border-radius: 0.95rem;
  border: 1px solid rgba(145, 178, 218, 0.2);
  background: rgba(9, 15, 23, 0.75);
  padding: 0.85rem 0.95rem;
}

.event-timeline__card h4 {
  color: rgba(246, 251, 255, 0.96);
  font-size: 0.97rem;
  font-weight: 620;
}

.event-timeline__card p {
  margin-top: 0.42rem;
  color: rgba(201, 216, 232, 0.8);
  font-size: 0.9rem;
  line-height: 1.6;
}

.event-timeline__sources {
  margin-top: 0.7rem;
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.event-timeline__source {
  border-radius: 999px;
  border: 1px solid rgba(240, 138, 75, 0.24);
  background: rgba(255, 255, 255, 0.04);
  padding: 0.25rem 0.62rem;
  font-size: 0.75rem;
  color: rgba(255, 231, 213, 0.86);
}

.person-profile {
  display: grid;
  gap: 0.6rem;
  border-radius: 0.95rem;
  border: 1px solid rgba(145, 178, 218, 0.2);
  background: rgba(10, 15, 24, 0.72);
  padding: 0.8rem 0.9rem;
}

.person-profile__item {
  display: grid;
  gap: 0.2rem;
}

.person-profile__item dt {
  font-size: 0.72rem;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: rgba(163, 187, 211, 0.7);
}

.person-profile__item dd {
  color: rgba(244, 250, 255, 0.94);
  font-size: 0.9rem;
  line-height: 1.55;
}

.person-appearances {
  display: grid;
  gap: 0.6rem;
}

.person-appearances__item {
  border-radius: 0.9rem;
  border: 1px solid rgba(145, 178, 218, 0.18);
  background: rgba(9, 14, 22, 0.72);
  padding: 0.75rem 0.85rem;
}

.person-appearances__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 0.55rem;
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.14em;
  color: rgba(176, 197, 218, 0.72);
}

.person-appearances__quote {
  margin-top: 0.45rem;
  color: rgba(214, 227, 239, 0.86);
  font-size: 0.9rem;
  line-height: 1.55;
}

.person-appearances__article {
  margin-top: 0.52rem;
  display: inline-flex;
  color: rgba(255, 227, 206, 0.92);
  font-size: 0.82rem;
}

.keyword-trend {
  display: grid;
  gap: 0.52rem;
}

.keyword-trend__item {
  display: grid;
  grid-template-columns: 5.3rem minmax(0, 1fr) 2.2rem;
  align-items: center;
  gap: 0.55rem;
}

.keyword-trend__date,
.keyword-trend__value {
  font-size: 0.78rem;
  color: rgba(203, 219, 234, 0.82);
}

.keyword-trend__bar-track {
  height: 0.46rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.08);
  overflow: hidden;
}

.keyword-trend__bar {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, rgba(240, 138, 75, 0.95), rgba(89, 151, 231, 0.92));
}

.keyword-pills {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.keyword-pill {
  border-radius: 999px;
  border: 1px solid rgba(145, 178, 218, 0.22);
  padding: 0.32rem 0.66rem;
  color: rgba(233, 243, 253, 0.88);
  font-size: 0.78rem;
}

.keyword-cooccurrence,
.keyword-contexts {
  display: grid;
  gap: 0.52rem;
}

.keyword-cooccurrence__item,
.keyword-contexts__item {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.8rem;
  border-radius: 0.85rem;
  border: 1px solid rgba(145, 178, 218, 0.18);
  background: rgba(10, 15, 23, 0.7);
  padding: 0.65rem 0.8rem;
  color: rgba(219, 231, 243, 0.86);
}

.keyword-contexts__item {
  display: grid;
  justify-content: start;
}

.keyword-contexts__item p {
  line-height: 1.55;
  font-size: 0.88rem;
}

.keyword-contexts__source {
  margin-top: 0.42rem;
  font-size: 0.74rem;
  text-transform: uppercase;
  letter-spacing: 0.12em;
  color: rgba(170, 192, 214, 0.74);
}

@media (max-width: 767px) {
  .keyword-trend__item {
    grid-template-columns: 4.6rem minmax(0, 1fr) 2rem;
  }
}
</style>
