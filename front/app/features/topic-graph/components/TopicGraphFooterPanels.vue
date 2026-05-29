<script setup lang="ts">
import { computed } from 'vue'
import type { TopicGraphDetailPayload } from '~/api/topicGraph'
import { normalizeTopicCategory } from '~/features/topic-graph/utils/normalizeTopicCategory'

interface Props {
  detail: TopicGraphDetailPayload | null
}

const props = defineProps<Props>()

const panelTitle = computed(() => {
  if (!props.detail) return '分析看板'
  const category = normalizeTopicCategory(props.detail.topic.category, props.detail.topic.kind)
  switch (category) {
    case 'person':
      return '人物分析'
    case 'keyword':
      return '关键词分析'
    default:
      return '事件分析'
  }
})
</script>

<template>
  <section class="topic-footer-grid" data-testid="topic-graph-footer">
    <div class="topic-footer-intro">
      <p class="topic-footer-intro__eyebrow">{{ panelTitle }}</p>
      <h3 class="topic-footer-intro__title">分析看板入口开发中，敬请期待</h3>
    </div>

    <div class="topic-footer-placeholder">
      <p class="topic-footer-placeholder__text">
        后续将提供独立的主题分析看板，支持事件脉络、人物关系、关键词云等深度探索。
      </p>
    </div>
  </section>
</template>

<style scoped>
.topic-footer-grid {
  display: grid;
  gap: 0.9rem;
}

.topic-footer-intro {
  display: grid;
  gap: 0.28rem;
  border-left: 2px solid rgba(240, 138, 75, 0.42);
  padding-left: 0.9rem;
}

.topic-footer-intro__eyebrow {
  font-size: 0.72rem;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: rgba(186, 206, 226, 0.72);
}

.topic-footer-intro__title {
  font-size: 0.95rem;
  line-height: 1.55;
  color: rgba(241, 247, 252, 0.9);
}

.topic-footer-placeholder {
  padding: 1.5rem;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.03);
  border: 1px dashed rgba(186, 206, 226, 0.24);
}

.topic-footer-placeholder__text {
  margin: 0;
  font-size: 0.85rem;
  line-height: 1.6;
  color: rgba(186, 206, 226, 0.64);
  text-align: center;
}
</style>