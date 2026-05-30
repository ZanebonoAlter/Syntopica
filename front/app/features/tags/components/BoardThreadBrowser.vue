<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useDailyReportsApi, type ThreadLineageNode } from '~/api/dailyReports'

const props = defineProps<{ boardId: number }>()

const { getBoardThreadTimeline } = useDailyReportsApi()

const days = ref(14)
const loading = ref(false)
const threads = ref<ThreadLineageNode[]>([])
const selectedNode = ref<ThreadLineageNode | null>(null)

// --- Date columns for the range ---

const dateColumns = computed<string[]>(() => {
  const cols: string[] = []
  const now = new Date()
  for (let i = days.value - 1; i >= 0; i--) {
    const d = new Date(now)
    d.setDate(d.getDate() - i)
    cols.push(formatDateISO(d))
  }
  return cols
})

function formatDateISO(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

function formatDateShort(dateStr: string): string {
  const d = new Date(dateStr)
  return `${d.getMonth() + 1}/${d.getDate()}`
}

// --- Lineage chain building ---

interface LineageChain {
  rootId: number
  title: string
  nodes: ThreadLineageNode[]
}

function buildChains(flatNodes: ThreadLineageNode[]): LineageChain[] {
  if (flatNodes.length === 0) return []

  const nodeMap = new Map<number, ThreadLineageNode>()
  for (const n of flatNodes) {
    nodeMap.set(n.id, n)
  }

  // Build parent -> children map
  const childrenMap = new Map<number, ThreadLineageNode[]>()
  for (const n of flatNodes) {
    if (n.prev_thread_id != null) {
      const list = childrenMap.get(n.prev_thread_id) || []
      list.push(n)
      childrenMap.set(n.prev_thread_id, list)
    }
  }

  // Find roots: nodes whose prev_thread_id is null OR whose prev_thread_id is not in nodeMap
  const visited = new Set<number>()
  const chains: LineageChain[] = []

  // Process nodes with prev_thread_id first to find those referenced by others
  const referencedAsPrev = new Set<number>()
  for (const n of flatNodes) {
    if (n.prev_thread_id != null) {
      referencedAsPrev.add(n.prev_thread_id)
    }
  }

  // Build chains starting from roots
  for (const n of flatNodes) {
    if (visited.has(n.id)) continue

    const isRoot = n.prev_thread_id == null || !nodeMap.has(n.prev_thread_id)
    if (!isRoot) continue

    // BFS/DFS from this root
    const chainNodes: ThreadLineageNode[] = []
    const queue = [n]
    while (queue.length > 0) {
      const current = queue.shift()!
      if (visited.has(current.id)) continue
      visited.add(current.id)
      chainNodes.push(current)
      const kids = childrenMap.get(current.id) || []
      queue.push(...kids)
    }

    // Sort by period_date
    chainNodes.sort((a, b) => a.period_date.localeCompare(b.period_date))

    chains.push({
      rootId: n.id,
      title: chainNodes[0]!.title,
      nodes: chainNodes,
    })
  }

  // Remaining nodes not visited (orphans without root reference)
  for (const n of flatNodes) {
    if (visited.has(n.id)) continue
    visited.add(n.id)
    chains.push({
      rootId: n.id,
      title: n.title,
      nodes: [n],
    })
  }

  // Sort chains: longer chains first, then by earliest date
  chains.sort((a, b) => {
    if (a.nodes.length !== b.nodes.length) return b.nodes.length - a.nodes.length
    return a.nodes[0]!.period_date.localeCompare(b.nodes[0]!.period_date)
  })

  return chains
}

const chains = computed(() => buildChains(threads.value))

// --- Status styling ---

const statusColors: Record<string, string> = {
  emerging: 'bg-green-500',
  continuing: 'bg-blue-500',
  splitting: 'bg-orange-500',
  merging: 'bg-purple-500',
  ending: 'bg-gray-500',
}

const statusLabels: Record<string, string> = {
  emerging: '新兴',
  continuing: '持续',
  splitting: '分裂',
  merging: '合并',
  ending: '结束',
}

// --- Node position helpers ---

function getColumnIndex(dateStr: string): number {
  return dateColumns.value.indexOf(dateStr)
}

function nodeStyle(node: ThreadLineageNode, colWidth: number = 36) {
  const col = getColumnIndex(node.period_date)
  if (col < 0) return { display: 'none' }
  return {
    left: `${col * colWidth + colWidth / 2}px`,
  }
}

// Connector line between consecutive nodes in a chain
function connectorSegments(chain: LineageChain, colWidth: number = 36): Array<{ x1: number; y1: number; x2: number; y2: number }> {
  const segments: Array<{ x1: number; y1: number; x2: number; y2: number }> = []
  const rowHeight = 36
  for (let i = 0; i < chain.nodes.length - 1; i++) {
    const from = chain.nodes[i]!
    const to = chain.nodes[i + 1]!
    const fromCol = getColumnIndex(from.period_date)
    const toCol = getColumnIndex(to.period_date)
    if (fromCol < 0 || toCol < 0) continue
    segments.push({
      x1: fromCol * colWidth + colWidth / 2,
      y1: rowHeight / 2,
      x2: toCol * colWidth + colWidth / 2,
      y2: rowHeight / 2,
    })
  }
  return segments
}

function selectNode(node: ThreadLineageNode) {
  if (selectedNode.value?.id === node.id) {
    selectedNode.value = null
  } else {
    selectedNode.value = node
  }
}

// --- Data loading ---

async function loadData() {
  loading.value = true
  selectedNode.value = null
  try {
    const res = await getBoardThreadTimeline(props.boardId, days.value)
    if (res.success && res.data) {
      threads.value = res.data.threads || []
    } else {
      threads.value = []
    }
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.boardId, days],
  () => { loadData() },
  { immediate: true },
)
</script>

<template>
  <div class="btb-container">
    <!-- Controls -->
    <div class="btb-controls">
      <div class="btb-controls-left">
        <Icon icon="mdi:source-branch" width="15" class="text-white/50" />
        <span class="btb-controls-title">线程时间线</span>
      </div>
      <div class="btb-days-toggle">
        <button
          v-for="d in [7, 14, 30, 60]"
          :key="d"
          class="btb-days-btn"
          :class="{ active: days === d }"
          @click="days = d"
        >
          {{ d }}天
        </button>
      </div>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="btb-loading">
      <div v-for="i in 3" :key="i" class="btb-skeleton" />
    </div>

    <!-- Empty -->
    <div v-else-if="chains.length === 0" class="btb-empty">
      <Icon icon="mdi:source-branch" width="28" class="text-white/15" />
      <p>暂无线程数据</p>
    </div>

    <!-- Gantt chart -->
    <div v-else class="btb-chart">
      <!-- Date headers row -->
      <div class="btb-headers">
        <div class="btb-header-label"></div>
        <div class="btb-header-dates">
          <div
            v-for="date in dateColumns"
            :key="date"
            class="btb-header-cell"
          >
            {{ formatDateShort(date) }}
          </div>
        </div>
      </div>

      <!-- Chain rows -->
      <div class="btb-rows">
        <div
          v-for="chain in chains"
          :key="chain.rootId"
          class="btb-chain-row"
        >
          <div class="btb-chain-label" :title="chain.title">
            {{ chain.title }}
          </div>
          <div class="btb-chain-timeline">
            <!-- Connector SVG -->
            <svg
              class="btb-connectors"
              :viewBox="`0 0 ${dateColumns.length * 36} 36`"
              preserveAspectRatio="none"
            >
              <line
                v-for="(seg, si) in connectorSegments(chain)"
                :key="si"
                :x1="seg.x1"
                :y1="seg.y1"
                :x2="seg.x2"
                :y2="seg.y2"
                stroke="rgba(255,255,255,0.15)"
                stroke-width="1.5"
              />
            </svg>
            <!-- Nodes -->
            <button
              v-for="node in chain.nodes"
              :key="node.id"
              class="btb-node"
              :class="[statusColors[node.status] || 'bg-gray-500', { selected: selectedNode?.id === node.id }]"
              :style="nodeStyle(node)"
              :title="`${node.title} (${statusLabels[node.status] || node.status})`"
              @click="selectNode(node)"
            >
              <span class="btb-node-dot"></span>
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Node detail popup -->
    <Teleport to="body">
      <Transition name="btb-popup">
        <div v-if="selectedNode" class="btb-popup-overlay" @click.self="selectedNode = null">
          <div class="btb-popup">
            <div class="btb-popup-header">
              <span
                class="btb-popup-status"
                :class="statusColors[selectedNode.status] || 'bg-gray-500'"
              >
                {{ statusLabels[selectedNode.status] || selectedNode.status }}
              </span>
              <button class="btb-popup-close" @click="selectedNode = null">
                <Icon icon="mdi:close" width="14" />
              </button>
            </div>
            <div class="btb-popup-title">{{ selectedNode.title }}</div>
            <div v-if="selectedNode.summary" class="btb-popup-summary">{{ selectedNode.summary }}</div>
            <div class="btb-popup-meta">
              <span>{{ formatDateShort(selectedNode.period_date) }}</span>
              <span v-if="selectedNode.cluster_label" class="btb-popup-cluster">{{ selectedNode.cluster_label }}</span>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.btb-container {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  margin-top: 1rem;
  padding: 1rem;
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(255, 255, 255, 0.025);
}

/* Controls */
.btb-controls {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.btb-controls-left {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.btb-controls-title {
  font-size: 0.78rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.7);
}

.btb-days-toggle {
  display: flex;
  gap: 0.25rem;
}

.btb-days-btn {
  padding: 0.2rem 0.55rem;
  font-size: 0.65rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 4px;
  background: transparent;
  color: rgba(255, 255, 255, 0.35);
  cursor: pointer;
  transition: all 0.12s ease;
}

.btb-days-btn:hover {
  color: rgba(255, 255, 255, 0.6);
  border-color: rgba(255, 255, 255, 0.15);
}

.btb-days-btn.active {
  background: rgba(255, 255, 255, 0.1);
  color: rgba(255, 255, 255, 0.85);
  border-color: rgba(255, 255, 255, 0.2);
}

/* Loading */
.btb-loading {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.btb-skeleton {
  height: 36px;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.03);
  animation: btbPulse 1.5s ease-in-out infinite;
}

@keyframes btbPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

/* Empty */
.btb-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.4rem;
  padding: 2.5rem 0;
  color: rgba(255, 255, 255, 0.3);
  font-size: 0.8rem;
}

/* Chart */
.btb-chart {
  display: flex;
  flex-direction: column;
  overflow-x: auto;
}

/* Date headers */
.btb-headers {
  display: flex;
  align-items: flex-end;
  gap: 0;
  padding-bottom: 0.3rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  margin-bottom: 0.25rem;
}

.btb-header-label {
  flex-shrink: 0;
  width: 120px;
}

.btb-header-dates {
  display: flex;
  flex-shrink: 0;
}

.btb-header-cell {
  width: 36px;
  flex-shrink: 0;
  text-align: center;
  font-size: 0.55rem;
  color: rgba(255, 255, 255, 0.25);
  transform: rotate(-35deg);
  transform-origin: center bottom;
  white-space: nowrap;
}

/* Chain rows */
.btb-rows {
  display: flex;
  flex-direction: column;
}

.btb-chain-row {
  display: flex;
  align-items: center;
  gap: 0;
  height: 36px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.03);
  transition: background 0.12s ease;
}

.btb-chain-row:hover {
  background: rgba(255, 255, 255, 0.02);
}

.btb-chain-label {
  flex-shrink: 0;
  width: 120px;
  padding-right: 0.6rem;
  font-size: 0.7rem;
  color: rgba(255, 255, 255, 0.5);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.btb-chain-timeline {
  position: relative;
  flex-shrink: 0;
  width: max-content;
  height: 36px;
}

/* Connectors */
.btb-connectors {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
}

/* Nodes */
.btb-node {
  position: absolute;
  top: 50%;
  transform: translate(-50%, -50%);
  width: 12px;
  height: 12px;
  border-radius: 50%;
  border: none;
  padding: 0;
  cursor: pointer;
  transition: transform 0.1s ease, box-shadow 0.1s ease;
  display: flex;
  align-items: center;
  justify-content: center;
}

.btb-node:hover {
  transform: translate(-50%, -50%) scale(1.4);
  box-shadow: 0 0 8px rgba(255, 255, 255, 0.2);
  z-index: 2;
}

.btb-node.selected {
  transform: translate(-50%, -50%) scale(1.5);
  box-shadow: 0 0 10px rgba(255, 255, 255, 0.3);
  z-index: 3;
}

.btb-node-dot {
  width: 100%;
  height: 100%;
  border-radius: 50%;
}

/* Popup overlay */
.btb-popup-overlay {
  position: fixed;
  inset: 0;
  z-index: 300;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.4);
}

.btb-popup {
  background: #1e1e2e;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  padding: 1rem 1.2rem;
  max-width: 400px;
  width: 90vw;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
}

.btb-popup-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.5rem;
}

.btb-popup-status {
  display: inline-block;
  font-size: 0.6rem;
  font-weight: 500;
  padding: 0.12rem 0.4rem;
  border-radius: 3px;
  color: white;
}

.btb-popup-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  border-radius: 4px;
  background: transparent;
  color: rgba(255, 255, 255, 0.4);
  cursor: pointer;
  transition: color 0.12s ease;
}

.btb-popup-close:hover {
  color: rgba(255, 255, 255, 0.8);
}

.btb-popup-title {
  font-size: 0.9rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.85);
  line-height: 1.4;
  margin-bottom: 0.3rem;
}

.btb-popup-summary {
  font-size: 0.78rem;
  color: rgba(255, 255, 255, 0.5);
  line-height: 1.6;
  margin-bottom: 0.5rem;
}

.btb-popup-meta {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
}

.btb-popup-cluster {
  padding: 0.08rem 0.35rem;
  border-radius: 3px;
  background: rgba(255, 255, 255, 0.06);
}

/* Popup animation */
.btb-popup-enter-active {
  transition: opacity 150ms ease-out;
}
.btb-popup-enter-active .btb-popup {
  transition: opacity 200ms ease-out, transform 200ms ease-out;
}
.btb-popup-leave-active {
  transition: opacity 120ms ease-in;
}
.btb-popup-leave-active .btb-popup {
  transition: opacity 120ms ease-in, transform 120ms ease-in;
}
.btb-popup-enter-from {
  opacity: 0;
}
.btb-popup-enter-from .btb-popup {
  opacity: 0;
  transform: scale(0.95) translateY(4px);
}
.btb-popup-leave-to {
  opacity: 0;
}
.btb-popup-leave-to .btb-popup {
  opacity: 0;
  transform: scale(0.95) translateY(4px);
}
</style>
