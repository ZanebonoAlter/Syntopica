<script setup lang="ts">
/**
 * 版块态势矩阵（rework-workshop-into-radar-and-desk 2.1/2.2/2.5）。
 *
 * 泳道 × 日期网格：格值 = 当日 section 数（不是 thread 数）；空格 = 断档（不显示 0）；
 * 行尾 = 窗口内线索总数（排序键，title 里保留分区数）、今天列高亮；窗口内分区数 ≤2 的活跃泳道折叠进「休眠」组（纯前端展示规则）。
 *
 * 只读红线：格内与泳道行不提供归档/合并/转正等生命周期写操作；
 * 点格只 emit（抽屉由宿主开），空格给提示。
 */
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import { useNotify } from '~/composables/useNotify'
import type { LaneMatrixRow } from '~/features/tags/composables/useBoardConsole'

const props = defineProps<{
  boardId: number
  axis: string[]
  activeRows: LaneMatrixRow[]
  dormantRows: LaneMatrixRow[]
  selectedLaneId: number | null
  today: string
  windowDays: number
  expandedDormant: boolean
  loading: boolean
  error: string | null
  staleNotice: boolean
  emptyState: 'no-reports' | 'no-lanes' | null
  /** 数据实际截止日（后端报告锚点）；早于今天时提示，避免把「没有新日报」看成矩阵坏了。 */
  dataAsOf?: string | null
}>()

const emit = defineEmits<{
  selectLane: [topicId: number]
  openCell: [payload: { topicId: number, date: string }]
  retry: []
  toggleDormant: []
  generate: []
}>()

const { warn: notifyWarn } = useNotify()

const columnLabel = computed(() =>
  props.axis.map((date) => (date === props.today ? '今天' : date.slice(5))),
)

const gridStyle = computed(() => ({ '--matrix-cols': String(props.axis.length) }))

function isToday(date: string): boolean {
  return date === props.today
}

function cellCount(row: LaneMatrixRow, date: string): number {
  return row.counts[date] ?? 0
}

/** 密度分档：1/2/3/≥4（≥4 同档）。 */
function cellBucket(count: number): string {
  if (count <= 0) return ''
  if (count >= 4) return 'is-b4'
  return `is-b${count}`
}

function onCellClick(row: LaneMatrixRow, date: string): void {
  const count = cellCount(row, date)
  if (count <= 0) {
    notifyWarn(`${date} 这一天「${row.label}」没有分区`)
    return
  }
  emit('openCell', { topicId: row.topicId, date })
}

const showSkeleton = computed(() => props.loading && props.activeRows.length === 0 && props.dormantRows.length === 0)
const showError = computed(() => !props.loading && props.error !== null && !showSkeleton.value && props.emptyState === null && props.activeRows.length === 0 && props.dormantRows.length === 0)
</script>

<template>
  <section class="mx" data-testid="board-matrix">
    <div class="mx-head">
      <span class="mx-kicker">泳道 × 日期</span>
      <span class="mx-hint">格内数字 = 当天该泳道的分区数；空白 = 断档；窗口内 ≤2 个分区的泳道折叠进「休眠」</span>
      <span v-if="dataAsOf && dataAsOf < today" class="mx-asof" data-testid="matrix-data-asof">
        数据截止 {{ dataAsOf }}（之后没有新日报）
      </span>
    </div>

    <!-- 刷新失败但保留旧数据：顶部提示条（状态矩阵 error 行） -->
    <div v-if="staleNotice && error" class="mx-stale" data-testid="matrix-stale-notice">
      <Icon icon="mdi:alert-circle-outline" width="14" />
      <span>刷新失败（{{ error }}），当前显示的是之前的数据</span>
      <button type="button" class="mx-link" @click="emit('retry')">重试</button>
    </div>

    <!-- loading：网格骨架 -->
    <div v-if="showSkeleton" class="mx-skeleton" data-testid="matrix-skeleton">
      <div v-for="i in 5" :key="i" class="mx-skeleton-row" />
    </div>

    <!-- error（无旧数据）：内联错误条 + 重试 -->
    <div v-else-if="showError" class="mx-error" data-testid="matrix-error">
      <Icon icon="mdi:alert-circle-outline" width="16" />
      <span>{{ error }}</span>
      <button type="button" class="mx-link" data-testid="matrix-retry" @click="emit('retry')">重试</button>
    </div>

    <!-- 空态两分支（spec：无日报引导生成 / 有日报无泳道文案） -->
    <div v-else-if="emptyState === 'no-reports'" class="mx-empty" data-testid="matrix-empty-no-reports">
      <Icon icon="mdi:file-document-outline" width="22" class="mx-empty-icon" />
      <p class="mx-empty-text">该版块还没有日报，泳道动态基于每日日报结算。</p>
      <p class="mx-empty-hint">生成第一份日报后，这里会出现各条泳道最近发生的事。</p>
      <button
        type="button"
        class="mx-empty-action"
        data-testid="matrix-empty-generate"
        @click="emit('generate')"
      >去生成第一份日报</button>
    </div>
    <div v-else-if="emptyState === 'no-lanes'" class="mx-empty" data-testid="matrix-empty-no-lanes">
      <Icon icon="mdi:tag-off-outline" width="22" class="mx-empty-icon" />
      <p class="mx-empty-text">暂无活跃泳道——在日报里孵化话题后出现。</p>
    </div>

    <div v-else class="mx-scroll">
      <div class="mx-grid" :style="gridStyle" data-testid="matrix-grid">
        <div class="mx-corner" />
        <div
          v-for="(label, index) in columnLabel"
          :key="axis[index]"
          class="mx-dayhead"
          :class="{ 'is-today': isToday(axis[index]!) }"
        >{{ label }}</div>
        <div class="mx-sumhead">{{ windowDays }}天 · 线索</div>

        <template v-for="row in activeRows" :key="row.topicId">
          <button
            type="button"
            class="mx-lane"
            :class="{ 'is-selected': row.topicId === selectedLaneId }"
            :title="row.label"
            :aria-pressed="row.topicId === selectedLaneId"
            :data-testid="`matrix-lane-${row.topicId}`"
            @click="emit('selectLane', row.topicId)"
          >
            <span class="mx-lane-name">{{ row.label }}</span>
            <span v-if="row.watchLinked" class="mx-lane-watch">追踪</span>
          </button>
          <button
            v-for="date in axis"
            :key="`${row.topicId}-${date}`"
            type="button"
            class="mx-cell"
            :class="[cellBucket(cellCount(row, date)), { 'is-today': isToday(date), 'is-empty': cellCount(row, date) === 0 }]"
            :data-testid="`matrix-cell-${row.topicId}-${date}`"
            :aria-label="`${row.label} ${date} ${cellCount(row, date) || '无分区'}`"
            @click="onCellClick(row, date)"
          >{{ cellCount(row, date) || '' }}</button>
          <div
            class="mx-sum"
            :data-testid="`matrix-total-${row.topicId}`"
            :title="`${row.total} 个分区 · ${row.clueTotal} 条线索（行序按线索数）`"
          >{{ row.clueTotal }}</div>
        </template>

        <div v-if="dormantRows.length > 0" class="mx-dormant" data-testid="dormant-toggle">
          <button type="button" class="mx-dormant-btn" @click="emit('toggleDormant')">
            <Icon :icon="expandedDormant ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="14" />
            <span>休眠泳道 <b>{{ dormantRows.length }}</b></span>
            <span class="mx-dormant-names">{{ dormantRows.map((row) => row.label).join(' · ') }}</span>
            <span class="mx-dormant-tip">{{ expandedDormant ? '收起' : '展开查看' }}（它们仍挂着追踪，只是这段时间没动）</span>
          </button>
        </div>

        <template v-if="expandedDormant">
          <template v-for="row in dormantRows" :key="row.topicId">
            <button
              type="button"
              class="mx-lane is-dormant"
              :class="{ 'is-selected': row.topicId === selectedLaneId }"
              :title="row.label"
              :data-testid="`matrix-lane-${row.topicId}`"
              @click="emit('selectLane', row.topicId)"
            >
              <span class="mx-lane-name">{{ row.label }}</span>
              <span class="mx-lane-dormant-tag">休眠</span>
            </button>
            <button
              v-for="date in axis"
              :key="`${row.topicId}-${date}`"
              type="button"
              class="mx-cell is-dormant"
              :class="[cellBucket(cellCount(row, date)), { 'is-today': isToday(date), 'is-empty': cellCount(row, date) === 0 }]"
              :data-testid="`matrix-cell-${row.topicId}-${date}`"
              :aria-label="`${row.label} ${date} ${cellCount(row, date) || '无分区'}`"
              @click="onCellClick(row, date)"
            >{{ cellCount(row, date) || '' }}</button>
            <div
              class="mx-sum is-dormant"
              :data-testid="`matrix-total-${row.topicId}`"
              :title="`${row.total} 个分区 · ${row.clueTotal} 条线索`"
            >{{ row.clueTotal }}</div>
          </template>
        </template>
      </div>
    </div>
  </section>
</template>

<style scoped>
.mx {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  min-height: 0;
}

.mx-head {
  display: flex;
  align-items: baseline;
  gap: 0.8rem;
  flex-wrap: wrap;
}

.mx-kicker {
  font-size: 0.68rem;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: var(--color-text-muted);
}

.mx-hint {
  font-size: 0.74rem;
  color: var(--color-text-muted);
  opacity: 0.8;
}

.mx-asof {
  margin-left: auto;
  font-size: 0.7rem;
  color: var(--color-warning);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.mx-stale {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.4rem 0.6rem;
  border: 1px solid var(--color-warning-subtle);
  border-radius: 8px;
  background: var(--color-warning-subtle);
  color: var(--color-warning);
  font-size: 0.74rem;
}

.mx-link {
  margin-left: auto;
  border: none;
  background: none;
  color: var(--color-accent);
  font-size: 0.72rem;
  cursor: pointer;
  padding: 0;
}

.mx-link:hover {
  text-decoration: underline;
}

.mx-skeleton {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  padding: 0.4rem 0;
}

.mx-skeleton-row {
  height: 2.9rem;
  border-radius: 6px;
  background: var(--color-bg-hover);
  animation: mx-pulse 1.4s ease-in-out infinite;
}

@keyframes mx-pulse {
  0%, 100% { opacity: 0.55; }
  50% { opacity: 1; }
}

.mx-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.6rem 0.7rem;
  border: 1px solid var(--color-error-subtle);
  border-radius: 8px;
  background: var(--color-error-subtle);
  color: var(--color-error);
  font-size: 0.78rem;
}

.mx-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.35rem;
  padding: 2rem 1rem;
  border: 1px dashed var(--color-border-medium);
  border-radius: 10px;
  text-align: center;
}

.mx-empty-icon {
  color: var(--color-text-muted);
}

.mx-empty-text {
  margin: 0;
  font-size: 0.86rem;
  color: var(--color-text-secondary);
}

.mx-empty-action {
  margin-top: 0.4rem;
  padding: 0.3rem 0.7rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 7px;
  color: var(--color-accent);
  font-size: 0.74rem;
  text-decoration: none;
}

.mx-empty-action:hover {
  background: var(--color-accent-subtle);
}

.mx-empty-hint {
  margin: 0;
  font-size: 0.74rem;
  color: var(--color-text-muted);
}

/* 舞台内滚动：窗口 30/全部 时列数超宽，仅矩阵自身横滚（布局契约例外） */
.mx-scroll {
  overflow-x: auto;
  overflow-y: hidden;
  padding-bottom: 0.2rem;
}

.mx-grid {
  display: grid;
  grid-template-columns: minmax(9rem, 12rem) repeat(var(--matrix-cols, 14), minmax(2.1rem, 1fr)) 2.8rem;
  gap: 0.24rem;
  align-items: center;
  min-width: max-content;
}

.mx-corner {
  position: sticky;
  left: 0;
  z-index: 3;
  height: 1.4rem;
  background: var(--color-bg-base);
}

.mx-dayhead {
  font-size: 0.66rem;
  color: var(--color-text-muted);
  text-align: center;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.mx-dayhead.is-today {
  color: var(--color-accent);
  font-weight: 600;
}

.mx-sumhead {
  font-size: 0.66rem;
  color: var(--color-text-muted);
  text-align: right;
  padding-right: 0.3rem;
  white-space: nowrap;
}

.mx-lane {
  position: sticky;
  left: 0;
  z-index: 2;
  background: var(--color-bg-base);
  display: flex;
  align-items: center;
  gap: 0.35rem;
  min-width: 0;
  padding: 0.25rem 0.4rem 0.25rem 0;
  border: none;
  color: var(--color-text-secondary);
  text-align: left;
  cursor: pointer;
}

.mx-lane-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.86rem;
}

.mx-lane:hover .mx-lane-name {
  color: var(--color-text-primary);
}

.mx-lane.is-selected .mx-lane-name {
  color: var(--color-text-primary);
  font-weight: 600;
}

.mx-lane.is-selected::before {
  content: '';
  width: 2px;
  height: 1rem;
  border-radius: 1px;
  background: var(--color-accent);
  margin-left: -0.5rem;
  flex: 0 0 auto;
}

.mx-lane-watch,
.mx-lane-dormant-tag {
  flex: 0 0 auto;
  font-size: 0.58rem;
  padding: 0 0.22rem;
  border-radius: 2px;
  border: 1px solid var(--color-border-medium);
  color: var(--color-text-muted);
  white-space: nowrap;
}

.mx-lane-watch {
  color: var(--color-info);
  border-color: color-mix(in srgb, var(--color-info) 30%, transparent);
}

.mx-cell {
  height: 2.9rem;
  border: none;
  border-radius: 3px;
  display: grid;
  place-items: center;
  font-size: 0.74rem;
  font-variant-numeric: tabular-nums;
  color: var(--color-text-primary);
  background: color-mix(in srgb, var(--color-text-primary) 4%, transparent);
  cursor: pointer;
  transition: transform 0.1s ease, box-shadow 0.1s ease;
}

.mx-cell:hover {
  /* 不用 scale：横向滚动容器会裁掉上下 2–3px；用描边强调 */
  box-shadow: inset 0 0 0 1px var(--color-accent);
  z-index: 2;
}

.mx-cell.is-empty {
  background: transparent;
}

.mx-cell.is-b1 {
  background: color-mix(in srgb, var(--color-accent) 13%, transparent);
  color: var(--color-text-secondary);
}

.mx-cell.is-b2 {
  background: color-mix(in srgb, var(--color-accent) 26%, transparent);
}

.mx-cell.is-b3 {
  background: color-mix(in srgb, var(--color-accent) 44%, transparent);
}

.mx-cell.is-b4 {
  background: color-mix(in srgb, var(--color-accent) 66%, transparent);
  /* 不覆盖前景色：反白色在 66% accent 上双主题对比度均 <3:1 */
  font-weight: 600;
}

.mx-cell.is-today {
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-accent) 45%, transparent);
}

.mx-cell.is-dormant {
  opacity: 0.55;
}

.mx-lane.is-dormant {
  opacity: 0.55;
}

.mx-sum {
  text-align: right;
  font-size: 0.78rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
  padding-right: 0.3rem;
}

.mx-sum.is-dormant {
  opacity: 0.55;
}

.mx-dormant {
  grid-column: 1 / -1;
  margin-top: 0.35rem;
}

.mx-dormant-btn {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  width: 100%;
  padding: 0.45rem 0.6rem;
  border: 1px dashed var(--color-border-medium);
  border-radius: 8px;
  background: none;
  color: var(--color-text-muted);
  font-size: 0.76rem;
  cursor: pointer;
  text-align: left;
}

.mx-dormant-btn:hover {
  color: var(--color-text-secondary);
  border-color: var(--color-border-strong);
}

.mx-dormant-btn b {
  color: var(--color-text-secondary);
  font-variant-numeric: tabular-nums;
}

.mx-dormant-names {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  opacity: 0.85;
}

.mx-dormant-tip {
  margin-left: auto;
  flex: 0 0 auto;
  font-size: 0.68rem;
  opacity: 0.75;
}
</style>
