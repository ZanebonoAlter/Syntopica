<script setup lang="ts">
import { Icon } from '@iconify/vue'

interface Props {
  count: number
  isActive?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  isActive: false,
})

const emit = defineEmits<{
  select: []
}>()

function handleSelect() {
  emit('select')
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    handleSelect()
  }
}
</script>

<template>
  <article class="timeline-pending-item">
    <div class="timeline-pending-item__marker">
      <div class="timeline-pending-item__dot" :class="{ 'timeline-pending-item__dot--active': props.isActive }" />
      <div class="timeline-pending-item__line" />
    </div>

    <div class="timeline-pending-item__content">
      <div
        class="timeline-pending-item__body"
        :class="{ 'timeline-pending-item__body--active': props.isActive }"
        role="button"
        tabindex="0"
        @click="handleSelect"
        @keydown="handleKeydown"
      >
        <div class="timeline-pending-item__header">
          <Icon icon="mdi:file-document-edit-outline" width="18" />
          <span class="timeline-pending-item__title">正在整理</span>
          <span class="timeline-pending-item__count">{{ props.count }} 篇文章</span>
        </div>
        <p class="timeline-pending-item__hint">
          已打标签但尚未生成日报的文章，点击查看详情
        </p>
      </div>
    </div>
  </article>
</template>

<style scoped>
.timeline-pending-item {
  display: grid;
  grid-template-columns: 44px minmax(0, 1fr);
  gap: 0.9rem;
  position: relative;
}

.timeline-pending-item__marker {
  display: flex;
  flex-direction: column;
  align-items: center;
  position: relative;
}

.timeline-pending-item__dot {
  width: 12px;
  height: 12px;
  border-radius: 999px;
  background: rgba(240, 138, 75, 0.28);
  border: 2px dashed rgba(240, 138, 75, 0.56);
  flex-shrink: 0;
}

.timeline-pending-item__dot--active {
  background: rgba(240, 138, 75, 0.56);
  box-shadow: 0 0 12px rgba(240, 138, 75, 0.34);
}

.timeline-pending-item__line {
  width: 2px;
  flex: 1;
  min-height: 24px;
  margin-top: 4px;
  background: linear-gradient(180deg, rgba(240, 138, 75, 0.28), rgba(255, 255, 255, 0.03));
}

.timeline-pending-item__content {
  display: grid;
  gap: 0.55rem;
  padding-bottom: 1.4rem;
}

.timeline-pending-item__body {
  display: grid;
  gap: 0.55rem;
  text-align: left;
  border-radius: 1.15rem;
  border: 1px dashed rgba(240, 138, 75, 0.28);
  background: linear-gradient(180deg, rgba(40, 30, 25, 0.88), rgba(25, 18, 14, 0.94));
  padding: 1rem;
  transition: all 0.18s ease;
  cursor: pointer;
}

.timeline-pending-item__body:hover,
.timeline-pending-item__body--active {
  border-color: rgba(240, 138, 75, 0.48);
  background: linear-gradient(180deg, rgba(48, 36, 28, 0.94), rgba(30, 22, 16, 0.98));
}

.timeline-pending-item__header {
  display: flex;
  align-items: center;
  gap: 0.55rem;
  color: rgba(240, 138, 75, 0.92);
}

.timeline-pending-item__title {
  font-size: 0.95rem;
  font-weight: 600;
}

.timeline-pending-item__count {
  margin-left: auto;
  font-size: 0.78rem;
  padding: 0.22rem 0.58rem;
  border-radius: 999px;
  background: rgba(240, 138, 75, 0.15);
  color: rgba(255, 231, 213, 0.88);
}

.timeline-pending-item__hint {
  font-size: 0.82rem;
  line-height: 1.5;
  color: rgba(214, 225, 236, 0.62);
}

@media (max-width: 640px) {
  .timeline-pending-item {
    grid-template-columns: 34px minmax(0, 1fr);
    gap: 0.65rem;
  }

  .timeline-pending-item__body {
    padding: 0.85rem;
  }
}
</style>