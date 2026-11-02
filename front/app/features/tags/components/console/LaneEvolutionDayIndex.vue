<script setup lang="ts">
/**
 * 泳道长窗逐日索引（rework-workshop-into-radar-and-desk 矩阵阶段 G3，>31 日形态的折叠索引）。
 *
 * 默认折叠的 <details> 逐日清单：content 日 = 可点行（选日交父），gap/unavailable =
 * 静态行（'— 无分区' / '? 未取到'，不可点）。合计与首线索标题逐字来自节点投影，
 * 不补 0、不编造标题。
 */
import type { EvolutionNode } from './observationLanes'

defineProps<{
  nodes: EvolutionNode[]
}>()

const emit = defineEmits<{
  /** 选日（仅 content 日）。 */
  selectDate: [date: string]
}>()

function nodeCount(node: EvolutionNode): string {
  return node.total === null ? '?' : String(node.total)
}

function rowTitle(node: EvolutionNode): string {
  if (node.kind === 'gap') return `${node.date} 无分区`
  if (node.kind === 'unavailable') return `${node.date} 未取到`
  return `${node.date} · 当日合计 ${node.total ?? '?'} 条${node.firstTitle ? ` · ${node.firstTitle}` : ''}`
}
</script>

<template>
  <details class="oei" data-testid="lane-evolution-day-index">
    <summary>展开逐日索引（{{ nodes.length }} 日）</summary>
    <ul class="oei-list">
      <li v-for="node in nodes" :key="node.date" :class="node.kind">
        <button
          v-if="node.kind === 'content'"
          type="button"
          class="oei-day"
          :data-testid="`evolution-index-day-${node.date}`"
          :title="rowTitle(node)"
          @click="emit('selectDate', node.date)"
        >
          <span class="oei-date">{{ node.date }}</span>
          <span class="oei-count">{{ nodeCount(node) }} 条</span>
          <span class="oei-title">{{ node.firstTitle || '（无标题线索）' }}</span>
        </button>
        <span v-else class="oei-day is-static" :title="rowTitle(node)">
          <span class="oei-date">{{ node.date }}</span>
          <span class="oei-count">{{ node.kind === 'gap' ? '— 无分区' : '? 未取到' }}</span>
        </span>
      </li>
    </ul>
  </details>
</template>

<style scoped>
.oei {
  border: 1px dashed var(--color-border-medium);
  border-radius: 8px;
  padding: 0.4rem 0.6rem;
}

.oei summary {
  font-size: 0.74rem;
  color: var(--color-text-muted);
  cursor: pointer;
}

.oei summary:hover {
  color: var(--color-text-secondary);
}

.oei-list {
  list-style: none;
  margin: 0.4rem 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  max-height: 16rem;
  overflow-y: auto;
}

.oei-day {
  display: flex;
  align-items: baseline;
  gap: 0.6rem;
  width: 100%;
  border: none;
  background: none;
  padding: 0.18rem 0.2rem;
  border-radius: 5px;
  color: var(--color-text-secondary);
  font-size: 0.74rem;
  text-align: left;
  cursor: pointer;
  min-width: 0;
}

.oei-day:not(.is-static):hover {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
}

.oei-day.is-static {
  cursor: default;
  color: var(--color-text-muted);
}

.oei-date {
  flex: 0 0 auto;
  font-variant-numeric: tabular-nums;
}

.oei-count {
  flex: 0 0 auto;
  font-variant-numeric: tabular-nums;
}

.oei-title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1 1 auto;
  opacity: 0.85;
}
</style>
