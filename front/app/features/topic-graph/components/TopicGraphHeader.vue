<script setup lang="ts">
import { Icon } from '@iconify/vue'
import type { TopicGraphType } from '~/api/topicGraph'

interface Props {
  selectedType: TopicGraphType
  selectedDate: string
  loading?: boolean
  heroLabel: string
  heroSubline: string
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
})

const emit = defineEmits<{
  'update:type': [value: TopicGraphType]
  'update:date': [value: string]
  refresh: []
}>()

const typeOptions: TopicGraphType[] = ['daily', 'weekly']

function updateDate(event: Event) {
  emit('update:date', (event.target as HTMLInputElement).value)
}
</script>

<template>
  <header class="topic-hero">
    <div class="flex flex-col gap-5">
      <div class="space-y-2">
        <p class="text-[0.65rem] uppercase tracking-[0.34em] text-white/50">Topic Field</p>
        <h1 class="font-serif text-2xl text-white md:text-3xl">{{ heroLabel }}</h1>
        <p class="text-xs leading-5 text-white/60">{{ heroSubline }}</p>
      </div>

      <div class="topic-toolbar">
        <div class="topic-toolbar__switcher" role="tablist" aria-label="主题图谱窗口切换">
          <button
            v-for="type in typeOptions"
            :key="type"
            type="button"
            class="topic-toolbar__switch"
            :class="{ 'topic-toolbar__switch--active': props.selectedType === type }"
            @click="emit('update:type', type)"
          >
            {{ type === 'daily' ? '日报图谱' : '周报图谱' }}
          </button>
        </div>

        <label class="topic-toolbar__date">
          <span class="topic-toolbar__eyebrow">时间锚点</span>
          <input class="topic-toolbar__input" :value="props.selectedDate" type="date" @input="updateDate">
        </label>

        <button class="topic-toolbar__button" type="button" :disabled="props.loading" @click="emit('refresh')">
          {{ props.loading ? '图谱载入中...' : '刷新图谱' }}
        </button>

        <NuxtLink to="/" class="topic-toolbar__home-button" data-testid="return-home-button">
          <Icon icon="mdi:home-variant-outline" width="16" />
          返回首页
        </NuxtLink>
      </div>
    </div>
  </header>
</template>

<style scoped>
.topic-hero {
  /* Removed the detached card styling, now it blends into the rail */
  position: relative;
}

.topic-toolbar {
  display: grid;
  gap: 0.75rem;
  padding-top: 1rem;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}

.topic-toolbar__eyebrow {
  font-size: 0.65rem;
  letter-spacing: 0.24em;
  text-transform: uppercase;
  color: rgba(255, 255, 255, 0.4);
}

.topic-toolbar__switcher {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.35rem;
  background: rgba(0, 0, 0, 0.2);
  padding: 0.25rem;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.04);
}

.topic-toolbar__switch,
.topic-toolbar__button,
.topic-toolbar__input {
  min-height: 2.5rem;
  border-radius: 999px;
  font-size: 0.8rem;
}

.topic-toolbar__switch {
  color: rgba(255, 255, 255, 0.6);
  transition: all 0.2s ease;
}

.topic-toolbar__switch--active {
  background: rgba(255, 255, 255, 0.1);
  color: white;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);
}

.topic-toolbar__date {
  display: grid;
  gap: 0.4rem;
}

.topic-toolbar__input {
  width: 100%;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(0, 0, 0, 0.15);
  color: white;
  padding: 0 1rem;
}

.topic-toolbar__input::-webkit-calendar-picker-indicator {
  filter: invert(1) opacity(0.6);
}

.topic-toolbar__button {
  border: 1px solid rgba(240, 138, 75, 0.3);
  background: rgba(240, 138, 75, 0.15);
  color: rgba(255, 233, 220, 0.9);
  font-weight: 600;
  transition: all 0.2s ease;
}

.topic-toolbar__button:hover:not(:disabled) {
  background: rgba(240, 138, 75, 0.25);
  border-color: rgba(240, 138, 75, 0.5);
}

.topic-toolbar__button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.topic-toolbar__home-button {
  display: inline-flex;
  align-items: center;
  justify-content: flex-start;
  gap: 0.45rem;
  width: fit-content;
  min-height: 2.35rem;
  border-radius: 999px;
  border: 1px solid rgba(255, 255, 255, 0.14);
  background: rgba(255, 255, 255, 0.04);
  color: rgba(255, 255, 255, 0.82);
  padding: 0 0.9rem;
  font-size: 0.78rem;
  text-decoration: none;
  transition:
    background 0.2s ease,
    border-color 0.2s ease,
    color 0.2s ease,
    transform 0.2s ease;
}

.topic-toolbar__home-button:hover,
.topic-toolbar__home-button:focus-visible {
  transform: translateY(-1px);
  border-color: rgba(240, 138, 75, 0.4);
  background: rgba(240, 138, 75, 0.12);
  color: rgba(255, 235, 223, 0.95);
}
</style>
