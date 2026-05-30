<script setup lang="ts">
import { ref, watch } from 'vue'
import { useDailyReportsApi, type SectionLifecycleNode } from '~/api/dailyReports'

const props = defineProps<{
  sectionId: number
  visible: boolean
}>()

const emit = defineEmits<{
  close: []
  navigate: [node: SectionLifecycleNode]
}>()

const { getSectionLifecycle } = useDailyReportsApi()

const chain = ref<SectionLifecycleNode[]>([])
const loading = ref(false)
const error = ref(false)

const statusColor: Record<string, string> = {
  emerging: 'bg-emerald-500/20 text-emerald-400 ring-emerald-500/40',
  continuing: 'bg-blue-500/20 text-blue-400 ring-blue-500/40',
  ending: 'bg-gray-500/20 text-gray-400 ring-gray-500/40',
}

const dotColor: Record<string, string> = {
  emerging: 'bg-emerald-400',
  continuing: 'bg-blue-400',
  ending: 'bg-gray-400',
}

const statusLabel: Record<string, string> = {
  emerging: '新兴',
  continuing: '持续',
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

let fetchId = 0

async function fetchLifecycle() {
  loading.value = true
  error.value = false
  const currentFetch = ++fetchId
  try {
    const res = await getSectionLifecycle(props.sectionId)
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
  () => [props.visible, props.sectionId] as const,
  ([vis]) => {
    if (vis) {
      fetchLifecycle()
    }
  },
  { immediate: true },
)

function handleNodeClick(node: SectionLifecycleNode) {
  emit('navigate', node)
}
</script>

<template>
  <Transition name="slp-panel">
    <div v-if="visible" class="slp-panel">
      <div class="slp-header">
        <span class="slp-title">话题生命周期</span>
        <button type="button" class="slp-close" @click="$emit('close')">&#10005;</button>
      </div>

      <div v-if="loading" class="slp-body">
        <div v-for="i in 3" :key="i" class="slp-skeleton-node">
          <div class="slp-skeleton-dot" />
          <div class="slp-skeleton-lines">
            <div class="slp-skeleton-line slp-skeleton-short" />
            <div class="slp-skeleton-line slp-skeleton-long" />
          </div>
        </div>
      </div>

      <div v-else-if="error" class="slp-body">
        <div class="slp-error">加载失败</div>
      </div>

      <div v-else class="slp-body slp-timeline">
        <div v-for="(node, i) in chain" :key="node.id" class="slp-node-wrapper">
          <div
            class="slp-node"
            :class="{ 'slp-node-current': node.id === sectionId }"
            @click="handleNodeClick(node)"
          >
            <div class="slp-node-dot" :class="dotColor[node.status] || 'bg-gray-400'" />
            <div class="slp-node-content">
              <div class="slp-node-date">{{ formatDate(node.period_date) }}</div>
              <div class="slp-node-title-row">
                <span class="slp-node-name">{{ node.cluster_label }}</span>
                <span class="slp-node-status" :class="statusColor[node.status] || ''">
                  {{ statusLabel[node.status] || node.status }}
                </span>
              </div>
              <div class="slp-node-meta">
                <span>{{ node.article_count }} 篇</span>
                <span>{{ node.thread_count }} 条线索</span>
              </div>
            </div>
          </div>
          <div v-if="i < chain.length - 1" class="slp-connector" />
        </div>

        <div v-if="chain.length === 0" class="slp-empty">独立话题</div>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.slp-panel {
  position: fixed;
  top: 0;
  right: 0;
  width: 320px;
  height: 100vh;
  background: #111827;
  border-left: 1px solid rgba(255, 255, 255, 0.08);
  display: flex;
  flex-direction: column;
  z-index: 250;
  box-shadow: -8px 0 32px rgba(0, 0, 0, 0.4);
}

.slp-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.7rem 1rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  flex-shrink: 0;
}

.slp-title {
  font-size: 0.85rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.8);
}

.slp-close {
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

.slp-close:hover {
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.7);
}

.slp-body {
  flex: 1;
  overflow-y: auto;
  padding: 1rem;
}

.slp-timeline {
  display: flex;
  flex-direction: column;
  gap: 0;
}

/* Node wrapper */
.slp-node-wrapper {
  display: flex;
  flex-direction: column;
}

/* Single node */
.slp-node {
  display: flex;
  gap: 0.75rem;
  padding: 0.6rem 0.4rem;
  position: relative;
  cursor: pointer;
  border-radius: 6px;
  transition: background 0.12s ease;
}

.slp-node:hover {
  background: rgba(255, 255, 255, 0.04);
}

.slp-node-current {
  background: rgba(255, 255, 255, 0.06);
}

/* Timeline dot */
.slp-node-dot {
  flex-shrink: 0;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  margin-top: 4px;
  position: relative;
  z-index: 1;
}

.slp-node-current .slp-node-dot {
  box-shadow: 0 0 0 3px rgba(255, 255, 255, 0.1);
}

/* Connector line */
.slp-connector {
  width: 2px;
  height: 16px;
  background: rgba(75, 85, 99, 0.6);
  margin-left: 8.4px;
}

/* Node content */
.slp-node-content {
  flex: 1;
  min-width: 0;
}

.slp-node-date {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
  margin-bottom: 0.2rem;
}

.slp-node-title-row {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-bottom: 0.2rem;
}

.slp-node-name {
  font-size: 0.82rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.75);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.slp-node-current .slp-node-name {
  color: rgba(255, 255, 255, 0.95);
}

.slp-node-status {
  display: inline-block;
  font-size: 0.58rem;
  padding: 0.06rem 0.35rem;
  border-radius: 3px;
  font-weight: 500;
  line-height: 1.5;
  flex-shrink: 0;
}

.slp-node-meta {
  display: flex;
  gap: 0.5rem;
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
}

/* Empty */
.slp-empty {
  text-align: center;
  padding: 1.5rem 0;
  font-size: 0.75rem;
  color: rgba(255, 255, 255, 0.3);
}

/* Error state */
.slp-error {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 2rem 0;
  font-size: 0.8rem;
  color: rgba(255, 255, 255, 0.3);
}

/* Loading skeleton */
.slp-skeleton-node {
  display: flex;
  gap: 0.75rem;
  padding: 0.6rem 0;
}

.slp-skeleton-dot {
  flex-shrink: 0;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.08);
  margin-top: 4px;
}

.slp-skeleton-lines {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  flex: 1;
}

.slp-skeleton-line {
  height: 10px;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.06);
  animation: slpPulse 1.5s ease-in-out infinite;
}

.slp-skeleton-short { width: 40%; }
.slp-skeleton-long { width: 85%; }

@keyframes slpPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

/* Slide transition */
.slp-panel-enter-active {
  transition: transform 250ms ease-out, opacity 250ms ease-out;
}

.slp-panel-leave-active {
  transition: transform 200ms ease-in, opacity 200ms ease-in;
}

.slp-panel-enter-from {
  transform: translateX(100%);
  opacity: 0;
}

.slp-panel-leave-to {
  transform: translateX(100%);
  opacity: 0;
}
</style>
