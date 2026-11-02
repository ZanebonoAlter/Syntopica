<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { DeskDayNode } from './deskTypes'

const props = withDefaults(defineProps<{
  days: DeskDayNode[]
  currentDate: string
  topics?: { topicId: number, label: string, color?: string }[]
}>(), {
  currentDate: '',
  topics: () => [],
})

const emit = defineEmits<{
  select: [date: string]
  openLane: [topicId: number]
}>()

function pick(day: DeskDayNode): void {
  if (day.hasReport) emit('select', day.date)
}
</script>

<template>
  <aside class="dde" aria-label="日报演进" data-testid="desk-evolution">
    <div class="dde-head">
      <Icon icon="mdi:chart-timeline-variant" width="14" />
      <h2>近 14 天</h2>
    </div>
    <ol class="dde-days">
      <li
        v-for="day in props.days"
        :key="day.date"
        class="dde-day"
        :class="{ 'is-current': day.date === currentDate, 'is-today': day.isToday, 'is-empty': !day.hasReport }"
      >
        <button
          type="button"
          :disabled="!day.hasReport"
          :data-testid="`desk-day-${day.date}`"
          :title="day.hasReport ? `查看 ${day.date} 日报` : `${day.date} 无日报`"
          @click="pick(day)"
        >
          <span class="dde-label">{{ day.label }}</span>
          <span class="dde-dot" aria-hidden="true" />
          <span v-if="day.isToday" class="dde-today">今</span>
        </button>
      </li>
    </ol>
    <p v-if="!props.days.length" class="dde-empty">暂无日报节点</p>

    <section v-if="props.topics?.length" class="dde-topics">
      <h3>本期话题</h3>
      <div v-for="topic in props.topics" :key="topic.topicId" class="dde-topic">
        <span class="dde-topic-dot" :style="{ backgroundColor: topic.color || 'var(--color-accent)' }" />
        <span class="dde-topic-label">{{ topic.label }}</span>
        <button
          type="button"
          class="dde-lane"
          title="在矩阵中查看这条线"
          @click="emit('openLane', topic.topicId)"
        >
          矩阵
        </button>
      </div>
    </section>
  </aside>
</template>

<style scoped>
.dde {
  flex: none;
  padding: 0.85rem 0.8rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-elevated);
  box-shadow: var(--shadow-subtle);
}

.dde-head {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  color: var(--color-text-muted);
}

.dde-head h2 {
  margin: 0;
  font-size: 0.72rem;
  font-weight: 600;
  letter-spacing: 0.08em;
}

.dde-days {
  display: grid;
  gap: 0.12rem;
  margin: 0.55rem 0 0;
  padding: 0;
  list-style: none;
}

.dde-day button {
  display: grid;
  grid-template-columns: 1fr 0.6rem auto;
  align-items: center;
  gap: 0.4rem;
  width: 100%;
  padding: 0.22rem 0.3rem;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 0.68rem;
  text-align: left;
  cursor: pointer;
}

.dde-day button:hover:not(:disabled),
.dde-day button:focus-visible:not(:disabled) {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
  outline: none;
}

.dde-day button:disabled {
  color: var(--color-text-muted);
  opacity: 0.5;
  cursor: default;
}

.dde-label {
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.64rem;
}

.dde-dot {
  width: 0.36rem;
  height: 0.36rem;
  border-radius: 50%;
  background: var(--color-border-strong);
}

.dde-day.is-empty .dde-dot {
  background: transparent;
  border: 1px dashed var(--color-border-strong);
}

.dde-day.is-current button {
  background: var(--color-bg-active);
  color: var(--color-text-primary);
  font-weight: 600;
}

.dde-day.is-current .dde-dot {
  background: var(--color-accent);
}

.dde-day.is-today .dde-label {
  color: var(--color-accent);
}

.dde-today {
  padding: 0 0.2rem;
  border-radius: 3px;
  background: color-mix(in srgb, var(--color-accent) 16%, transparent);
  color: var(--color-accent);
  font-size: 0.58rem;
}

.dde-empty {
  margin: 0.5rem 0 0;
  color: var(--color-text-muted);
  font-size: 0.68rem;
}

.dde-topics {
  margin-top: 0.75rem;
  padding-top: 0.65rem;
  border-top: 1px solid var(--color-border-subtle);
}

.dde-topics h3 {
  margin: 0 0 0.35rem;
  color: var(--color-text-muted);
  font-size: 0.62rem;
  font-weight: 600;
  letter-spacing: 0.1em;
}

.dde-topic {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.18rem 0;
  font-size: 0.68rem;
}

.dde-topic-dot {
  flex: none;
  width: 0.36rem;
  height: 0.36rem;
  border-radius: 50%;
}

.dde-topic-label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  color: var(--color-text-secondary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dde-lane {
  flex: none;
  padding: 0.1rem 0.3rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 3px;
  background: transparent;
  color: var(--color-text-muted);
  font-size: 0.6rem;
  cursor: pointer;
}

.dde-lane:hover,
.dde-lane:focus-visible {
  border-color: var(--color-accent);
  color: var(--color-accent);
  outline: none;
}
</style>
