<script setup lang="ts">
/**
 * 泳道逐日节点带（rework-workshop-into-radar-and-desk 矩阵阶段 G3，短窗 ≤31 日形态）。
 *
 * 逐日节点：点径 = min(24, 6+√日合计×5)、节点数字 = 当日线索合计（各主题桶非综合稿 thread 数之和，
 * 不是桶峰值）、线索合计 ≥3 显示首线索标题（截断）。三态纪律：content=有分区可开（已知 0 合法、
 * 全未知显示 '?'）；gap=无分区空心禁用（曲线值 0 但不可开）；unavailable='?' 禁用。
 * 禁用节点真实点击不可达（aria/title 已带说明）；合成事件兜底只提示不 emit。
 */
import { useNotify } from '~/composables/useNotify'
import { evolutionDotSize, type EvolutionNode } from './observationLanes'

const props = defineProps<{
  nodes: EvolutionNode[]
  /** 受控选中日高亮（父持有 selectedDate）。 */
  selectedDate?: string | null
}>()

const emit = defineEmits<{
  /** 选日（仅 content 日；父更新 selectedDate 与下方阅读，不强滚）。 */
  selectDate: [date: string]
}>()

const { warn: notifyWarn } = useNotify()

function dotStyle(node: EvolutionNode): Record<string, string> {
  if (node.kind !== 'content') return {}
  const size = evolutionDotSize(node.total ?? 0)
  return { width: `${size}px`, height: `${size}px` }
}

function clip(text: string, max: number): string {
  return text.length > max ? `${text.slice(0, max)}…` : text
}

function nodeCount(node: EvolutionNode): string {
  if (node.kind === 'gap') return '—'
  return node.total === null ? '?' : String(node.total)
}

function nodeTitle(node: EvolutionNode): string {
  if (node.kind === 'gap') return `${node.date} 无分区`
  if (node.kind === 'unavailable') return `${node.date} 未取到`
  return `${node.date} · 当日 ${node.total ?? '?'} 条线索${node.firstTitle ? ` · ${node.firstTitle}` : ''}`
}

function onNodeClick(node: EvolutionNode): void {
  if (node.kind === 'content') {
    emit('selectDate', node.date)
    return
  }
  if (node.kind === 'gap') notifyWarn(`${node.date} 这一天没有分区`)
  else notifyWarn(`${node.date} 该日历史数据未取到`)
}
</script>

<template>
  <div
    class="oev-ribbon"
    data-testid="lane-evolution-ribbon"
    :style="{ '--oev-cols': String(nodes.length) }"
  >
    <button
      v-for="node in nodes"
      :key="node.date"
      type="button"
      class="oev-node"
      :class="[node.kind, { 'is-today': node.isToday, 'is-selected': node.date === selectedDate }]"
      :disabled="node.kind !== 'content'"
      :data-testid="`evolution-node-${node.date}`"
      :data-date="node.date"
      :title="nodeTitle(node)"
      :aria-label="`${nodeTitle(node)}${node.kind === 'content' ? '，查看该日' : ''}`"
      @click="onNodeClick(node)"
    >
      <span v-if="node.kind !== 'unavailable'" class="oev-dot" :style="dotStyle(node)" />
      <span class="oev-node-count">{{ nodeCount(node) }}</span>
      <span class="oev-node-date">{{ node.isToday ? '今天' : node.date.slice(5) }}</span>
      <span v-if="node.kind === 'content' && node.showLabel && node.firstTitle" class="oev-node-label">
        {{ clip(node.firstTitle, 26) }}
      </span>
    </button>
  </div>
</template>

<style scoped>
.oev-ribbon {
  display: grid;
  grid-template-columns: repeat(var(--oev-cols, 14), minmax(1.7rem, 1fr));
  overflow-x: auto;
  position: relative;
  padding: 0.4rem 0 0.2rem;
  gap: 0.1rem;
}

.oev-ribbon::before {
  content: '';
  position: absolute;
  left: 0.4rem;
  right: 0.4rem;
  top: 1.15rem;
  height: 1px;
  background: var(--color-border-medium);
}

.oev-node {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.22rem;
  min-width: 0;
  padding: 0;
  border: none;
  background: none;
  color: var(--color-text-muted);
  cursor: pointer;
}

.oev-node:disabled {
  cursor: default;
}

.oev-node .oev-dot {
  display: block;
  border-radius: 50%;
  min-height: 6px;
}

.oev-node.content .oev-dot {
  background: var(--color-accent);
}

.oev-node.gap .oev-dot {
  width: 8px;
  height: 8px;
  min-height: 8px;
  background: transparent;
  border: 1px solid var(--color-border-strong);
}

.oev-node.unavailable {
  padding-top: 0.35rem;
}

.oev-node-count {
  font-size: 0.78rem;
  font-variant-numeric: tabular-nums;
  color: var(--color-text-secondary);
  line-height: 1.1;
}

.oev-node.content .oev-node-count {
  color: var(--color-text-primary);
}

.oev-node-date {
  font-size: 0.64rem;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.oev-node.is-today .oev-node-date {
  color: var(--color-accent);
  font-weight: 600;
}

.oev-node.is-selected {
  outline: 1px solid var(--color-accent);
  outline-offset: 2px;
  border-radius: 4px;
}

.oev-node-label {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.64rem;
  color: var(--color-text-muted);
}
</style>
