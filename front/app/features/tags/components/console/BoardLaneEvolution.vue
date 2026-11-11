<script setup lang="ts">
/**
 * 选中泳道演进带（rework-workshop-into-radar-and-desk 2.4）。
 *
 * 与矩阵共用同一份 lane-dynamics 响应：按日期渲染节点（大小/数字 = 当日事件线数，
 * ≥3 条事件线显示事件标签），并给出「今天这条线发生的事」「最密集的三天」与
 * 滚动态势句（长句折叠，标注 as_of）。
 *
 * 红线：前端不推断/补造节点——无 section 的日期只画空心节点；事件与日期映射
 * 全部来自后端 timeline。
 */
import { computed, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useNotify } from '~/composables/useNotify'
import type { LaneMatrixRow } from '~/features/tags/composables/useBoardConsole'

const props = defineProps<{
  axis: string[]
  row: LaneMatrixRow | null
  today: string
  windowDays: number
  /** 与矩阵共用加载/错误态（任务 2.5）：loading 骨架、error 局部错误 + 重试。 */
  loading?: boolean
  error?: string | null
  /** 刷新失败但保留旧数据（与矩阵同源布尔标记）。 */
  staleNotice?: boolean
}>()

const emit = defineEmits<{
  openCell: [payload: { topicId: number, date: string }]
  retry: []
}>()

const { warn: notifyWarn } = useNotify()

/** 密集日标签阈值：当日事件线数 ≥3 才显示事件标签（story 口径，add-event-story-heat 5.4）。 */
const DENSE_LABEL_THRESHOLD = 3
/** 长态势句折叠阈值（字符数）。 */
const SUMMARY_FOLD_LENGTH = 80

const summaryExpanded = ref(false)
watch(() => props.row?.topicId, () => {
  summaryExpanded.value = false
})

const sectionsByDate = computed(() => {
  const map = new Map<string, string[]>()
  for (const day of props.row?.lane.timeline ?? []) {
    map.set(day.date, (day.sections ?? []).map((s) => s.label))
  }
  return map
})

interface RibbonNode {
  date: string
  count: number
  clueCount: number
  storyCount: number
  participants: number
  labels: string[]
  isToday: boolean
  sizeClass: string
}

const nodes = computed<RibbonNode[]>(() => props.axis.map((date) => {
  const count = props.row?.counts[date] ?? 0
  const clueCount = props.row?.clueCounts[date] ?? 0
  const storyCount = props.row?.storyCounts[date] ?? 0
  return {
    date,
    count,
    clueCount,
    storyCount,
    participants: props.row?.participantCounts[date] ?? 0,
    labels: sectionsByDate.value.get(date) ?? [],
    isToday: date === props.today,
    sizeClass: storyCount >= DENSE_LABEL_THRESHOLD ? 'is-big' : storyCount > 0 ? 'is-mid' : 'is-hollow',
  }
}))

function clip(text: string, max: number): string {
  return text.length > max ? `${text.slice(0, max)}…` : text
}

const todayNode = computed(() => nodes.value.find((node) => node.isToday) ?? null)

const todayEvents = computed(() => {
  const node = todayNode.value
  if (!node || node.count === 0) return []
  return (props.row?.lane.timeline ?? [])
    .find((day) => day.date === node.date)?.sections.flatMap((s) => s.events) ?? []
})

/** 最密集的三天：事件线数降序，并列取更晚日期（spec 只约束「最密集」，并列口径按此固定）。 */
const densestDays = computed(() => {
  const withData = nodes.value.filter((node) => node.count > 0)
  return [...withData]
    .sort((a, b) => b.storyCount - a.storyCount || (a.date < b.date ? 1 : -1))
    .slice(0, 3)
})

/** 最近一次有分区的日期（今天无数据时的兜底提示，不补造事件）。 */
const lastActiveDate = computed(() => {
  const row = props.row
  if (!row || row.lastActiveIndex < 0) return ''
  return props.axis[row.lastActiveIndex] ?? ''
})

const snapshot = computed(() => props.row?.lane.snapshot ?? null)
const canFoldSummary = computed(() => {
  const snap = snapshot.value
  if (!snap) return false
  return (snap.detail ?? snap.summary ?? '').length > SUMMARY_FOLD_LENGTH
})
const summaryText = computed(() => {
  const snap = snapshot.value
  if (!snap) return ''
  return summaryExpanded.value ? (snap.detail || snap.summary) : snap.summary
})

function openNode(node: RibbonNode): void {
  if (!props.row || node.count === 0) {
    if (props.row) notifyWarn(`${node.date} 这一天「${props.row.label}」没有分区`)
    return
  }
  emit('openCell', { topicId: props.row.topicId, date: node.date })
}
</script>

<template>
  <section class="ev" data-testid="lane-evolution">
    <div v-if="!row && loading" class="ev-state" data-testid="lane-evolution-loading">
      <div class="ev-skeleton-line" />
      <div class="ev-skeleton-line is-short" />
      <div class="ev-skeleton-ribbon" />
    </div>
    <div v-else-if="!row && error" class="ev-state ev-state--error" data-testid="lane-evolution-error">
      <Icon icon="mdi:alert-circle-outline" width="15" />
      <span>演进带加载失败{{ error ? `（${error}）` : '' }}</span>
      <button type="button" class="ev-retry" data-testid="lane-evolution-retry" @click="emit('retry')">重试</button>
    </div>
    <div v-else-if="!row" class="ev-empty" data-testid="lane-evolution-empty">
      选一条泳道，看它怎么走到今天。
    </div>

    <template v-else>
      <div v-if="staleNotice || error" class="ev-stale" data-testid="lane-evolution-stale">
        刷新失败，当前展示上次数据
      </div>
      <header class="ev-head">
        <span class="ev-name" :title="row.label">{{ row.label }}</span>
        <span v-if="row.watchLinked" class="ev-watch">追踪</span>
        <span class="ev-meta">{{ windowDays }} 天 {{ row.storyTotal }} 条事件线 · 今日 {{ row.storyCounts[today] ?? 0 }}</span>
      </header>

      <!-- 态势句：有快照 → summary + as_of（长句折叠）；无快照 → 待结算占位 -->
      <div v-if="snapshot" class="ev-stance" data-testid="lane-evolution-stance">
        <p class="ev-stance-text" :class="{ 'is-clamped': canFoldSummary && !summaryExpanded }">{{ summaryText }}</p>
        <div class="ev-stance-foot">
          <span class="ev-asof" data-testid="lane-evolution-asof">汇总截止 {{ snapshot.as_of }}</span>
          <button
            v-if="canFoldSummary"
            type="button"
            class="ev-summary-toggle"
            data-testid="lane-evolution-summary-toggle"
            @click="summaryExpanded = !summaryExpanded"
          >
            {{ summaryExpanded ? '收起' : '展开全文' }}
          </button>
        </div>
      </div>
      <div v-else class="ev-pending" data-testid="lane-evolution-pending">
        态势待结算 · 时间线照常展示
      </div>

      <!-- 演进带：节点大小 = 当日 section 数；≥3 个分区显示事件标签 -->
      <div
        class="ev-ribbon"
        data-testid="lane-evolution-ribbon"
        :style="{ '--ev-cols': String(axis.length) }"
      >
        <button
          v-for="node in nodes"
          :key="node.date"
          type="button"
          class="ev-node"
          :class="[node.sizeClass, { 'is-today': node.isToday }]"
          :data-testid="`evolution-node-${node.date}`"
          :title="node.count > 0 ? `${node.date} ${node.storyCount} 条事件线 · 最热 ${node.participants} 个来源 · ${node.labels.join(' / ')}` : `${node.date} 无分区`"
          :aria-label="node.count > 0 ? `${node.date} ${node.storyCount} 条事件线 · 最热 ${node.participants} 个来源` : `${node.date} 无分区`"
          @click="openNode(node)"
        >
          <span class="ev-dot" />
          <span class="ev-node-count">{{ node.count > 0 ? node.storyCount : '—' }}</span>
          <span class="ev-node-date">{{ node.isToday ? '今天' : node.date.slice(5) }}</span>
          <span v-if="node.storyCount >= DENSE_LABEL_THRESHOLD && node.labels.length" class="ev-node-label">
            {{ clip(node.labels.join(' / '), 26) }}
          </span>
        </button>
      </div>

      <!-- 今日事件 / 最密集三天 -->
      <div class="ev-grid">
        <div class="ev-col">
          <h4>今天这条线发生的事</h4>
          <div v-if="todayEvents.length" class="ev-events" data-testid="lane-evolution-today-events">
            <div v-for="(event, index) in todayEvents.slice(0, 4)" :key="index" class="ev-event">
              <span class="ev-event-date">{{ today.slice(5) }}</span>
              <span>{{ event }}</span>
            </div>
            <div v-if="todayEvents.length > 4" class="ev-more">另有 {{ todayEvents.length - 4 }} 条</div>
          </div>
          <div v-else class="ev-hint" data-testid="lane-evolution-no-today">
            今天这条线还没有分区。
            <template v-if="lastActiveDate">最近一次 {{ lastActiveDate }}。</template>
          </div>
        </div>

        <div class="ev-col">
          <h4>最密集的三天</h4>
          <div v-if="densestDays.length" class="ev-peaks" data-testid="lane-evolution-peaks">
            <div v-for="day in densestDays" :key="day.date" class="ev-peak">
              <span class="ev-peak-date">{{ day.date.slice(5) }}</span>
              <span class="ev-peak-label" :title="day.labels.join(' / ')">{{ clip(day.labels.join(' / '), 30) }}</span>
              <span class="ev-peak-count">{{ day.storyCount }}</span>
            </div>
          </div>
          <div v-else class="ev-hint">这个窗口没有分区。</div>
        </div>
      </div>
    </template>
  </section>
</template>

<style scoped>
.ev {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  padding: 0.9rem 1rem 1rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 12px;
  background: color-mix(in srgb, var(--color-bg-elevated) 70%, transparent);
}

.ev-state {
  padding: 0.9rem 0;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  color: var(--color-text-muted);
  font-size: 0.75rem;
}

.ev-state--error {
  flex-direction: row;
  align-items: center;
  gap: 0.5rem;
}

.ev-retry {
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  padding: 0.15rem 0.45rem;
  background: none;
  color: var(--color-accent);
  font-size: 0.7rem;
  cursor: pointer;
}

.ev-skeleton-line {
  height: 0.9rem;
  border-radius: 6px;
  background: var(--color-bg-hover);
}

.ev-skeleton-line.is-short {
  width: 45%;
}

.ev-skeleton-ribbon {
  height: 2.2rem;
  border-radius: 8px;
  background: var(--color-bg-hover);
  margin-top: 0.3rem;
}

.ev-stale {
  padding: 0.35rem 0.5rem;
  border-radius: 6px;
  background: var(--color-warning-subtle, var(--color-bg-hover));
  color: var(--color-text-secondary);
  font-size: 0.7rem;
}

.ev-empty {
  padding: 0.6rem 0.2rem;
  font-size: 0.8rem;
  color: var(--color-text-muted);
}

.ev-head {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  min-width: 0;
}

.ev-name {
  font-family: var(--font-serif, Georgia, serif);
  font-size: 1.05rem;
  color: var(--color-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ev-watch {
  font-size: 0.6rem;
  color: var(--color-info);
  border: 1px solid color-mix(in srgb, var(--color-info) 30%, transparent);
  border-radius: 2px;
  padding: 0 0.22rem;
}

.ev-meta {
  margin-left: auto;
  font-size: 0.7rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.ev-stance {
  padding: 0.5rem 0.65rem;
  border-left: 3px solid var(--color-accent);
  border-radius: 0 8px 8px 0;
  background: var(--color-accent-subtle);
}

.ev-stance-text {
  margin: 0;
  font-size: 0.8rem;
  line-height: 1.6;
  color: var(--color-text-primary);
  overflow-wrap: break-word;
}

.ev-stance-text.is-clamped {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.ev-stance-foot {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  margin-top: 0.25rem;
}

.ev-asof {
  font-size: 0.66rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.ev-summary-toggle {
  margin-left: auto;
  border: none;
  background: none;
  color: var(--color-accent);
  font-size: 0.7rem;
  cursor: pointer;
  padding: 0;
}

.ev-summary-toggle:hover {
  text-decoration: underline;
}

.ev-pending {
  padding: 0.45rem 0.6rem;
  border-radius: 8px;
  background: var(--color-bg-sunken);
  font-size: 0.74rem;
  color: var(--color-text-muted);
}

.ev-ribbon {
  display: grid;
  grid-template-columns: repeat(var(--ev-cols, 14), minmax(1.4rem, 1fr));
  overflow-x: auto;
  position: relative;
  padding: 0.4rem 0 0.2rem;
  gap: 0.1rem;
}

.ev-ribbon::before {
  content: '';
  position: absolute;
  left: 0.4rem;
  right: 0.4rem;
  top: 0.85rem;
  height: 1px;
  background: var(--color-border-medium);
}

.ev-node {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.25rem;
  min-width: 0;
  padding: 0;
  border: none;
  background: none;
  color: var(--color-text-muted);
  cursor: pointer;
}

.ev-dot {
  width: 0.5rem;
  height: 0.5rem;
  border-radius: 50%;
  margin-top: 0.3rem;
  background: var(--color-bg-sunken);
  border: 1px solid var(--color-border-strong);
  position: relative;
  z-index: 1;
}

.ev-node.is-mid .ev-dot {
  background: color-mix(in srgb, var(--color-accent) 45%, transparent);
  border-color: color-mix(in srgb, var(--color-accent) 55%, transparent);
}

.ev-node.is-big .ev-dot {
  width: 0.72rem;
  height: 0.72rem;
  margin-top: 0.19rem;
  background: var(--color-accent);
  border-color: var(--color-accent);
}

.ev-node.is-today .ev-dot {
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--color-accent) 15%, transparent);
}

.ev-node-date {
  font-size: 0.62rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.ev-node.is-today .ev-node-date {
  color: var(--color-accent);
}

.ev-node-label {
  max-width: 100%;
  font-size: 0.66rem;
  line-height: 1.4;
  text-align: center;
  color: var(--color-text-muted);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  overflow-wrap: anywhere;
}

.ev-node.is-big .ev-node-label {
  color: var(--color-text-secondary);
}

.ev-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.3fr) minmax(0, 1fr);
  gap: 1.2rem;
  padding-top: 0.7rem;
  border-top: 1px dashed var(--color-border-subtle);
}

.ev-col {
  min-width: 0;
}

.ev-col h4 {
  margin: 0 0 0.45rem;
  font-size: 0.66rem;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: var(--color-text-muted);
  font-weight: 500;
}

.ev-events,
.ev-peaks {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
}

.ev-event {
  display: flex;
  gap: 0.55rem;
  font-size: 0.8rem;
  line-height: 1.5;
  color: var(--color-text-primary);
  overflow-wrap: break-word;
}

.ev-event-date,
.ev-peak-date {
  flex: 0 0 auto;
  font-size: 0.66rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
  padding-top: 0.14rem;
}

.ev-peak {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  font-size: 0.78rem;
  color: var(--color-text-secondary);
  min-width: 0;
}

.ev-peak-label {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ev-peak-count {
  margin-left: auto;
  font-size: 0.7rem;
  color: var(--color-accent);
  font-variant-numeric: tabular-nums;
}

.ev-hint,
.ev-more {
  font-size: 0.74rem;
  color: var(--color-text-muted);
}

@media (max-width: 767.98px) {
  .ev-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .ev-ribbon {
    grid-template-columns: repeat(var(--ev-cols, 14), minmax(1.1rem, 1fr));
  }

  .ev-head {
    flex-wrap: wrap;
  }

  .ev-meta {
    margin-left: 0;
    width: 100%;
  }
}
</style>
