<script setup lang="ts">
import { Icon } from '@iconify/vue'

type TopicAnalysisType = 'event' | 'person' | 'keyword'

interface Props {
  modelValue: TopicAnalysisType
  counts?: {
    event: number
    person: number
    keyword: number
  }
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'update:modelValue': [value: TopicAnalysisType]
}>()

const tabs: Array<{ type: TopicAnalysisType; label: string; icon: string }> = [
  {
    type: 'event',
    label: '事件',
    icon: 'mdi:calendar-star',
  },
  {
    type: 'person',
    label: '人物',
    icon: 'mdi:account-star',
  },
  {
    type: 'keyword',
    label: '关键词',
    icon: 'mdi:tag',
  },
]

function handleSelect(type: TopicAnalysisType) {
  if (props.modelValue === type) return
  emit('update:modelValue', type)
}
</script>

<template>
  <div class="topic-analysis-tabs" data-testid="topic-analysis-tabs">
    <button
      v-for="tab in tabs"
      :key="tab.type"
      class="tab-button"
      type="button"
      :data-testid="`tab-${tab.type}`"
      :class="{ 'tab-button--active': props.modelValue === tab.type }"
      @click="handleSelect(tab.type)"
    >
      <Icon :icon="tab.icon" width="16" />
      <span>{{ tab.label }}</span>
      <span v-if="props.counts" class="tab-count">{{ props.counts[tab.type] }}</span>
    </button>
  </div>
</template>

<style scoped>
.topic-analysis-tabs {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 0.55rem;
  border-radius: 1rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: linear-gradient(180deg, rgba(17, 26, 36, 0.72), rgba(9, 14, 22, 0.88));
  padding: 0.45rem;
}

.tab-button {
  position: relative;
  display: inline-flex;
  min-height: 2.75rem;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
  border-radius: 0.78rem;
  border: 1px solid transparent;
  background: transparent;
  color: rgba(214, 225, 236, 0.76);
  font-size: 0.88rem;
  font-weight: 600;
  letter-spacing: 0.02em;
  transition:
    color 0.2s ease,
    border-color 0.2s ease,
    background 0.2s ease,
    transform 0.2s ease;
}

.tab-button::after {
  content: '';
  position: absolute;
  left: 18%;
  right: 18%;
  bottom: 0.42rem;
  height: 2px;
  border-radius: 999px;
  background: linear-gradient(90deg, rgba(240, 138, 75, 0.96), rgba(90, 151, 231, 0.9));
  opacity: 0;
  transform: scaleX(0.4);
  transition:
    opacity 0.2s ease,
    transform 0.2s ease;
}

.tab-button:hover,
.tab-button:focus-visible {
  color: rgba(247, 251, 255, 0.95);
  border-color: rgba(154, 187, 225, 0.22);
  background: rgba(255, 255, 255, 0.04);
}

.tab-button:focus-visible {
  outline: 2px solid rgba(240, 138, 75, 0.46);
  outline-offset: 1px;
}

.tab-button--active {
  color: rgba(255, 243, 234, 0.95);
  border-color: rgba(240, 138, 75, 0.28);
  background: linear-gradient(180deg, rgba(240, 138, 75, 0.18), rgba(87, 146, 229, 0.11));
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.08);
}

.tab-button--active::after {
  opacity: 1;
  transform: scaleX(1);
}

.tab-count {
  display: inline-flex;
  min-width: 1.3rem;
  justify-content: center;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.12);
  padding: 0.02rem 0.38rem;
  font-size: 0.7rem;
  line-height: 1.4;
  color: rgba(255, 245, 236, 0.92);
}

@media (max-width: 767px) {
  .topic-analysis-tabs {
    grid-template-columns: 1fr;
  }
}
</style>
