<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Icon } from '@iconify/vue'
import type {
  TopicInfo,
  AIAnalysisStatus,
  AIAnalysisResult,
  TopicCategoryType,
} from '~/types/ai'
import EventAnalysisView from './EventAnalysisView.vue'
import PersonAnalysisView from './PersonAnalysisView.vue'
import KeywordAnalysisView from './KeywordAnalysisView.vue'

interface Props {
  topic: TopicInfo | null
  status: AIAnalysisStatus
  progress?: number
  result?: AIAnalysisResult | null
  error?: string | null
  lastUpdated?: string | null
}

const props = withDefaults(defineProps<Props>(), {
  progress: 0,
  result: null,
  error: null,
  lastUpdated: null,
})

const emit = defineEmits<{
  'request-analysis': [topic: TopicInfo | null]
  'rebuild-analysis': [topic: TopicInfo | null]
  'close': []
  'view-detail': [section: string]
}>()

const isExpanded = ref(true)

const categoryLabels: Record<TopicCategoryType, string> = {
  event: '事件分析',
  person: '人物分析',
  keyword: '关键词分析',
}

const statusText = computed(() => {
  switch (props.status) {
    case 'pending':
      return '等待分析'
    case 'processing':
      return `AI分析中... ${props.progress}%`
    case 'completed':
      return '分析完成'
    case 'failed':
      return '分析失败'
    default:
      return '等待开始'
  }
})

const statusIcon = computed(() => {
  switch (props.status) {
    case 'pending':
      return 'mdi:clock-outline'
    case 'processing':
      return 'mdi:loading'
    case 'completed':
      return 'mdi:check-circle'
    case 'failed':
      return 'mdi:alert-circle'
    default:
      return 'mdi:brain'
  }
})

const formattedLastUpdated = computed(() => {
  if (!props.lastUpdated) return null
  try {
    const date = new Date(props.lastUpdated)
    return date.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return props.lastUpdated
  }
})

function toggleExpand() {
  isExpanded.value = !isExpanded.value
}

function handleStartAnalysis() {
  emit('request-analysis', props.topic)
  isExpanded.value = true
}

function handleRebuild() {
  emit('rebuild-analysis', props.topic)
  isExpanded.value = true
}

function handleViewDetail(section: string) {
  emit('view-detail', section)
}

// Auto-expand when analysis starts
watch(
  () => props.status,
  (newStatus) => {
    if (newStatus === 'processing') {
      isExpanded.value = true
    }
  }
)
</script>

<template>
  <div class="ai-analysis-panel" :class="`status-${status}`">
    <!-- Panel Header -->
    <div class="panel-header" @click="toggleExpand">
      <div class="header-left">
        <div class="header-icon">
          <Icon :icon="statusIcon" :class="{ 'animate-spin': status === 'processing' }" width="20" />
        </div>
        <div class="header-content">
          <h3 class="header-title">
            <span v-if="topic">{{ topic.label }}</span>
            <span v-else>AI深度分析</span>
          </h3>
          <p class="header-status" :class="`status--${status}`">
            {{ statusText }}
          </p>
        </div>
      </div>
      <div class="header-right">
        <span v-if="topic" class="topic-category" :class="`category--${topic.category}`">
          {{ categoryLabels[topic.category] }}
        </span>

        <button
          v-if="status === 'idle' || status === 'pending'"
          type="button"
          class="btn-action btn-start"
          @click.stop="handleStartAnalysis"
        >
          <Icon icon="mdi:play" width="16" />
          <span>开始分析</span>
        </button>

        <button
          v-if="status === 'completed' || status === 'failed'"
          type="button"
          class="btn-action btn-rebuild"
          @click.stop="handleRebuild"
        >
          <Icon icon="mdi:refresh" width="16" />
          <span>重新分析</span>
        </button>

        <button
          v-if="status === 'completed' || status === 'failed'"
          type="button"
          class="btn-toggle"
          :class="{ 'is-expanded': isExpanded }"
          @click.stop="toggleExpand"
        >
          <Icon :icon="isExpanded ? 'mdi:chevron-up' : 'mdi:chevron-down'" width="20" />
        </button>

        <button type="button" class="btn-close" @click.stop="emit('close')">
          <Icon icon="mdi:close" width="18" />
        </button>
      </div>
    </div>

    <!-- Progress Bar -->
    <div v-if="status === 'processing'" class="progress-bar">
      <div class="progress-fill" :style="{ width: `${Math.min(Math.max(progress, 0), 100)}%` }" />
    </div>

    <!-- Panel Content -->
    <div v-show="isExpanded" class="panel-content">
      <!-- Processing State -->
      <div v-if="status === 'processing'" class="state-processing">
        <div class="processing-animation">
          <Icon icon="mdi:brain" class="brain-icon" width="48" />
          <div class="pulse-ring" />
        </div>
        <p class="processing-text">AI正在分析相关日报内容，提取关键信息...</p>
        <p class="processing-hint">分析时间取决于内容数量，请耐心等待</p>
      </div>

      <!-- Failed State -->
      <div v-else-if="status === 'failed'" class="state-failed">
        <Icon icon="mdi:alert-circle-outline" class="error-icon" width="32" />
        <h4>分析失败</h4>
        <p class="error-message">{{ error || '未知错误' }}</p>
        <button type="button" class="btn-retry" @click="handleRebuild">
          <Icon icon="mdi:refresh" width="16" />
          <span>重新分析</span>
        </button>
      </div>

      <!-- Idle/Pending State -->
      <div v-else-if="status === 'idle' || status === 'pending'" class="state-idle">
        <Icon icon="mdi:brain" class="idle-icon" width="48" />
        <h4>等待分析</h4>
        <p>点击"开始分析"按钮，AI将深度分析该题材的相关内容</p>
        <button type="button" class="btn-start-large" @click="handleStartAnalysis">
          <Icon icon="mdi:play" width="20" />
          <span>开始分析</span>
        </button>
      </div>

      <!-- Completed State -->
      <div v-else-if="status === 'completed' && result" class="state-completed">
        <!-- Event Analysis -->
        <EventAnalysisView
          v-if="result.type === 'event' && result.eventAnalysis"
          :data="result.eventAnalysis"
          @view-detail="handleViewDetail"
        />

        <!-- Person Analysis -->
        <PersonAnalysisView
          v-else-if="result.type === 'person' && result.personAnalysis"
          :data="result.personAnalysis"
          @view-detail="handleViewDetail"
        />

        <!-- Keyword Analysis -->
        <KeywordAnalysisView
          v-else-if="result.type === 'keyword' && result.keywordAnalysis"
          :data="result.keywordAnalysis"
          @view-detail="handleViewDetail"
        />

        <!-- Metadata -->
        <div v-if="result.metadata" class="analysis-metadata">
          <div class="metadata-item">
            <Icon icon="mdi:clock-outline" width="14" />
            <span>分析时间: {{ result.metadata.analysisTime }}</span>
          </div>
          <div class="metadata-item">
            <Icon icon="mdi:chip" width="14" />
            <span>模型版本: {{ result.metadata.modelVersion }}</span>
          </div>
          <div v-if="formattedLastUpdated" class="metadata-item">
            <Icon icon="mdi:calendar" width="14" />
            <span>更新于: {{ formattedLastUpdated }}</span>
          </div>
        </div>
      </div>
    </div>
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

.ai-analysis-panel.status-pending {
  border-color: rgba(255, 255, 255, 0.1);
}

.ai-analysis-panel.status-processing {
  border-color: rgba(240, 138, 75, 0.3);
}

.ai-analysis-panel.status-completed {
  border-color: rgba(16, 185, 129, 0.3);
}

.ai-analysis-panel.status-failed {
  border-color: rgba(239, 68, 68, 0.3);
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 1rem 1.25rem;
  cursor: pointer;
  transition: background 0.15s ease;
}

.panel-header:hover {
  background: rgba(255, 255, 255, 0.02);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex: 1;
  min-width: 0;
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

.status--pending {
  color: rgba(255, 255, 255, 0.6);
}

.status--processing {
  color: rgba(240, 138, 75, 0.9);
}

.status--completed {
  color: rgba(16, 185, 129, 0.9);
}

.status--failed {
  color: rgba(239, 68, 68, 0.9);
}

.header-right {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.topic-category {
  border-radius: 999px;
  padding: 0.25rem 0.65rem;
  font-size: 0.75rem;
  font-weight: 500;
}

.category--event {
  background: rgba(99, 102, 241, 0.2);
  border: 1px solid rgba(99, 102, 241, 0.4);
  color: rgba(165, 180, 252, 0.9);
}

.category--person {
  background: rgba(16, 185, 129, 0.2);
  border: 1px solid rgba(16, 185, 129, 0.4);
  color: rgba(110, 231, 183, 0.9);
}

.category--keyword {
  background: rgba(245, 158, 11, 0.2);
  border: 1px solid rgba(245, 158, 11, 0.4);
  color: rgba(252, 211, 77, 0.9);
}

.btn-action {
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

.btn-action:hover {
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

.btn-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 2rem;
  height: 2rem;
  border-radius: 0.5rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: transparent;
  color: rgba(255, 255, 255, 0.5);
  cursor: pointer;
  transition: all 0.2s ease;
}

.btn-close:hover {
  border-color: rgba(239, 68, 68, 0.4);
  background: rgba(239, 68, 68, 0.1);
  color: rgba(239, 68, 68, 0.9);
}

.progress-bar {
  height: 0.25rem;
  background: rgba(255, 255, 255, 0.08);
}

.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, rgba(240, 138, 75, 0.95), rgba(90, 151, 231, 0.92));
  transition: width 0.25s ease;
}

.panel-content {
  padding: 0 1.25rem 1.25rem;
}

/* Processing State */
.state-processing {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 2rem 1rem;
  text-align: center;
}

.processing-animation {
  position: relative;
  width: 80px;
  height: 80px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.brain-icon {
  color: rgba(240, 138, 75, 0.9);
  z-index: 1;
}

.pulse-ring {
  position: absolute;
  width: 100%;
  height: 100%;
  border-radius: 50%;
  border: 2px solid rgba(240, 138, 75, 0.4);
  animation: pulse 2s ease-out infinite;
}

@keyframes pulse {
  0% {
    transform: scale(0.8);
    opacity: 1;
  }
  100% {
    transform: scale(1.4);
    opacity: 0;
  }
}

.processing-text {
  margin-top: 1rem;
  font-size: 0.9rem;
  color: rgba(255, 255, 255, 0.8);
}

.processing-hint {
  margin-top: 0.5rem;
  font-size: 0.75rem;
  color: rgba(255, 255, 255, 0.5);
}

/* Failed State */
.state-failed {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 2rem 1rem;
  text-align: center;
}

.error-icon {
  color: rgba(239, 68, 68, 0.9);
}

.state-failed h4 {
  margin: 0.75rem 0 0.5rem;
  font-size: 1rem;
  color: rgba(255, 255, 255, 0.9);
}

.error-message {
  font-size: 0.85rem;
  color: rgba(255, 255, 255, 0.6);
  margin: 0 0 1rem;
}

.btn-retry {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.8rem;
  padding: 0.5rem 1rem;
  border-radius: 999px;
  border: 1px solid rgba(240, 138, 75, 0.4);
  background: linear-gradient(135deg, rgba(240, 138, 75, 0.2), rgba(255, 160, 100, 0.15));
  color: rgba(255, 200, 150, 0.95);
  cursor: pointer;
  transition: all 0.2s ease;
}

.btn-retry:hover {
  border-color: rgba(240, 138, 75, 0.6);
  background: linear-gradient(135deg, rgba(240, 138, 75, 0.3), rgba(255, 160, 100, 0.25));
}

/* Idle State */
.state-idle {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 2rem 1rem;
  text-align: center;
}

.idle-icon {
  color: rgba(255, 255, 255, 0.3);
}

.state-idle h4 {
  margin: 0.75rem 0 0.5rem;
  font-size: 1rem;
  color: rgba(255, 255, 255, 0.8);
}

.state-idle p {
  font-size: 0.85rem;
  color: rgba(255, 255, 255, 0.5);
  margin: 0 0 1rem;
}

.btn-start-large {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.9rem;
  padding: 0.75rem 1.5rem;
  border-radius: 999px;
  border: 1px solid rgba(240, 138, 75, 0.4);
  background: linear-gradient(135deg, rgba(240, 138, 75, 0.2), rgba(255, 160, 100, 0.15));
  color: rgba(255, 200, 150, 0.95);
  cursor: pointer;
  transition: all 0.2s ease;
}

.btn-start-large:hover {
  border-color: rgba(240, 138, 75, 0.6);
  background: linear-gradient(135deg, rgba(240, 138, 75, 0.3), rgba(255, 160, 100, 0.25));
  transform: translateY(-1px);
}

/* Completed State */
.state-completed {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

/* Metadata */
.analysis-metadata {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  padding-top: 1rem;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}

.metadata-item {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.75rem;
  color: rgba(255, 255, 255, 0.5);
}

.metadata-item svg {
  color: rgba(255, 255, 255, 0.4);
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

  .topic-category {
    display: none;
  }

  .btn-action span {
    display: none;
  }

  .btn-action {
    padding: 0.5rem;
  }
}
</style>
