<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useDailyReportsApi, type ThreadLineageNode } from '~/api/dailyReports'

const props = defineProps<{
  threadId: number
  visible: boolean
}>()

defineEmits<{
  close: []
}>()

const { getThreadLineage } = useDailyReportsApi()

const chain = ref<ThreadLineageNode[]>([])
const loading = ref(false)
const error = ref(false)

const statusColor: Record<string, string> = {
  emerging: 'bg-emerald-500/20 text-emerald-400 ring-emerald-500/40',
  continuing: 'bg-blue-500/20 text-blue-400 ring-blue-500/40',
  splitting: 'bg-orange-500/20 text-orange-400 ring-orange-500/40',
  merging: 'bg-purple-500/20 text-purple-400 ring-purple-500/40',
  ending: 'bg-gray-500/20 text-gray-400 ring-gray-500/40',
}

const dotColor: Record<string, string> = {
  emerging: 'bg-emerald-400',
  continuing: 'bg-blue-400',
  splitting: 'bg-orange-400',
  merging: 'bg-purple-400',
  ending: 'bg-gray-400',
}

const statusLabel: Record<string, string> = {
  emerging: '新兴',
  continuing: '持续',
  splitting: '分裂',
  merging: '合并',
  ending: '结束',
}

const weekDays = ['日', '一', '二', '三', '四', '五', '六']

function formatDate(dateStr: string): string {
  const d = new Date(dateStr)
  const month = d.getMonth() + 1
  const day = d.getDate()
  const weekDay = weekDays[d.getDay()]
  return `${month}月${day}日 周${weekDay}`
}

function truncate(text: string, maxLen: number): string {
  if (!text) return ''
  return text.length > maxLen ? text.slice(0, maxLen) + '...' : text
}

let fetchId = 0

async function fetchLineage() {
  loading.value = true
  error.value = false
  const currentFetch = ++fetchId
  try {
    const res = await getThreadLineage(props.threadId)
    if (currentFetch !== fetchId) return
    if (res.success && res.data) {
      chain.value = res.data.chain || []
    } else {
      error.value = true
    }
  } catch {
    if (currentFetch !== fetchId) return
    error.value = true
  } finally {
    if (currentFetch === fetchId) {
      loading.value = false
    }
  }
}

watch(
  () => [props.visible, props.threadId] as const,
  ([vis]) => {
    if (vis) {
      fetchLineage()
    }
  },
)

const firstNode = computed(() => chain.value[0])

const isStandalone = (nodes: ThreadLineageNode[]): boolean => {
  if (nodes.length <= 1) return true
  return nodes.every(n => n.prev_thread_id === null) && nodes.length === 1
}
</script>

<template>
  <Transition name="lineage-panel">
    <div v-if="visible" class="lp-panel">
      <div class="lp-header">
        <span class="lp-title">线程血统</span>
        <button type="button" class="lp-close" @click="$emit('close')">✕</button>
      </div>

      <div v-if="loading" class="lp-body">
        <div v-for="i in 3" :key="i" class="lp-skeleton-node">
          <div class="lp-skeleton-dot" />
          <div class="lp-skeleton-lines">
            <div class="lp-skeleton-line lp-skeleton-short" />
            <div class="lp-skeleton-line lp-skeleton-long" />
            <div class="lp-skeleton-line lp-skeleton-medium" />
          </div>
        </div>
      </div>

      <div v-else-if="error" class="lp-body">
        <div class="lp-error">加载失败</div>
      </div>

      <div v-else class="lp-body lp-timeline">
        <!-- Standalone thread -->
        <template v-if="isStandalone(chain)">
          <div class="lp-standalone">
            <span class="lp-standalone-dot" />
            <span class="lp-standalone-label">独立线程</span>
          </div>
          <div v-if="firstNode" class="lp-node">
            <div class="lp-node-dot" :class="dotColor[firstNode.status] || 'bg-gray-400'" />
            <div class="lp-node-content">
              <div class="lp-node-date">{{ formatDate(firstNode.period_date) }}</div>
              <span class="lp-node-status" :class="statusColor[firstNode.status] || ''">
                {{ statusLabel[firstNode.status] || firstNode.status }}
              </span>
              <div class="lp-node-title">{{ firstNode.title }}</div>
              <div v-if="firstNode.summary" class="lp-node-summary">{{ truncate(firstNode.summary, 80) }}</div>
            </div>
          </div>
        </template>

        <!-- Lineage chain -->
        <template v-else>
          <div v-for="(node, i) in chain" :key="node.id" class="lp-node-wrapper">
            <div class="lp-node" :class="{ 'lp-node-current': node.id === threadId }">
              <div class="lp-node-dot" :class="dotColor[node.status] || 'bg-gray-400'" />
              <div class="lp-node-content">
                <div class="lp-node-date">{{ formatDate(node.period_date) }}</div>
                <span class="lp-node-status" :class="statusColor[node.status] || ''">
                  {{ statusLabel[node.status] || node.status }}
                </span>
                <div class="lp-node-title">{{ node.title }}</div>
                <div v-if="node.summary" class="lp-node-summary">{{ truncate(node.summary, 80) }}</div>
              </div>
            </div>
            <div v-if="i < chain.length - 1" class="lp-connector" />
          </div>
        </template>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.lp-panel {
  position: absolute;
  top: 0;
  right: 0;
  width: 320px;
  height: 100%;
  background: #111827;
  border-left: 1px solid rgba(255, 255, 255, 0.08);
  display: flex;
  flex-direction: column;
  z-index: 10;
}

.lp-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.7rem 1rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  flex-shrink: 0;
}

.lp-title {
  font-size: 0.85rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.8);
}

.lp-close {
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
  font-size: 0.85rem;
  transition: all 0.12s ease;
}

.lp-close:hover {
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.7);
}

.lp-body {
  flex: 1;
  overflow-y: auto;
  padding: 1rem;
}

.lp-timeline {
  display: flex;
  flex-direction: column;
  gap: 0;
}

/* Standalone thread */
.lp-standalone {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0;
  margin-bottom: 0.5rem;
}

.lp-standalone-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.3);
}

.lp-standalone-label {
  font-size: 0.72rem;
  color: rgba(255, 255, 255, 0.35);
}

/* Node wrapper (node + connector) */
.lp-node-wrapper {
  display: flex;
  flex-direction: column;
}

/* Single node */
.lp-node {
  display: flex;
  gap: 0.75rem;
  padding: 0.6rem 0;
  position: relative;
}

.lp-node-current {
  background: rgba(255, 255, 255, 0.04);
  border-radius: 6px;
  padding: 0.6rem 0.6rem;
  margin: 0 -0.6rem;
}

/* Timeline dot */
.lp-node-dot {
  flex-shrink: 0;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  margin-top: 4px;
  position: relative;
  z-index: 1;
}

.lp-node-current .lp-node-dot {
  box-shadow: 0 0 0 3px rgba(255, 255, 255, 0.1);
}

/* Connector line */
.lp-connector {
  width: 2px;
  height: 16px;
  background: rgba(75, 85, 99, 0.6);
  margin-left: 4px;
}

/* Node content */
.lp-node-content {
  flex: 1;
  min-width: 0;
}

.lp-node-date {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
  margin-bottom: 0.2rem;
}

.lp-node-status {
  display: inline-block;
  font-size: 0.58rem;
  padding: 0.06rem 0.35rem;
  border-radius: 3px;
  font-weight: 500;
  line-height: 1.5;
  margin-bottom: 0.3rem;
}

.lp-node-title {
  font-size: 0.82rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.75);
  line-height: 1.4;
}

.lp-node-current .lp-node-title {
  color: rgba(255, 255, 255, 0.95);
}

.lp-node-summary {
  font-size: 0.72rem;
  color: rgba(255, 255, 255, 0.35);
  line-height: 1.5;
  margin-top: 0.2rem;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* Error state */
.lp-error {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 2rem 0;
  font-size: 0.8rem;
  color: rgba(255, 255, 255, 0.3);
}

/* Loading skeleton */
.lp-skeleton-node {
  display: flex;
  gap: 0.75rem;
  padding: 0.6rem 0;
}

.lp-skeleton-dot {
  flex-shrink: 0;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.08);
  margin-top: 4px;
}

.lp-skeleton-lines {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  flex: 1;
}

.lp-skeleton-line {
  height: 10px;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.06);
  animation: lpPulse 1.5s ease-in-out infinite;
}

.lp-skeleton-short { width: 40%; }
.lp-skeleton-long { width: 85%; }
.lp-skeleton-medium { width: 60%; }

@keyframes lpPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

/* Slide transition */
.lineage-panel-enter-active {
  transition: transform 250ms ease-out, opacity 250ms ease-out;
}

.lineage-panel-leave-active {
  transition: transform 200ms ease-in, opacity 200ms ease-in;
}

.lineage-panel-enter-from {
  transform: translateX(100%);
  opacity: 0;
}

.lineage-panel-leave-to {
  transform: translateX(100%);
  opacity: 0;
}
</style>
