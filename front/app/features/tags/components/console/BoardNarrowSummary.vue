<script setup lang="ts">
/**
 * 窄屏泳道摘要（rework-workshop-into-radar-and-desk 矩阵阶段 G5 + 8C G3）。
 *
 * <768px 不横滚整张矩阵：保留**全部可选泳道**（活跃 + 休眠折叠）、选中态、14 日分区
 * 迷你条，并把就地下读交给下方同一个阅读槽（同一摘要/编年/其他话题实现）。
 * 口径：迷你条 = 当日分区数（不是桶峰值）；未知/断档不补 0。
 * 「本板块上下文」进入同一个单级宽工作面，不在手机另造第二套内容。
 *
 * 8C G3（同源，不是第二套日期导航）：
 *  - 报日条 = **同一连续 `visibleAxis`**，只有 `reportDates` 内日期是按钮；非报日保留为
 *    非交互文本（不压轴、不可聚焦）。选中日变化只滚**日期条自身 `scrollLeft`**（见
 *    `BoardNarrowDayStrip`，禁止 `scrollIntoView` 把祖先/整页拉回上方）。
 *  - 组顺序：正式行 → 当日其他话题（summary + 自身行）→ 休眠泳道（summary + 自身行）；
 *    两个组独立折叠（`othersOpen` / `expandedDormant` 全由 G4 持有，本组件不另持状态）；
 *    0 休眠仍保留 summary（0 / 本窗无，禁用展开）。
 *  - 0 正式泳道不再只剩空态：报日条 + 其他组 + 下方同一阅读槽（G4 挂载），旧空态降为行内说明。
 *  - 新 props/事件全部可选：G4 尚未接线时保持旧窄屏结构（泳道卡 + 迷你条 + 休眠入口）。
 *
 * G3 拆分（S1，内聚行为）：报日条/泳道行列表/组 summary/其他组拆到同目录子组件，父组件只保留
 * 编排（可见行、迷你条统一分档、组顺序与空/错/加载态）；DOM、testid、props/emits 与交互不变。
 */
import { computed } from 'vue'
import type { ObservedDayNode } from '~/features/tags/composables/useOtherTopicObservation'
import { Icon } from '@iconify/vue'
import type { LaneDayResolver, ObservationLaneRow } from './observationLanes'
import { narrowDaySectionCount } from './laneConsoleView'
import type { OtherTopicRow, OtherTopicsView } from './dayOtherTopics'
import BoardNarrowDayStrip from './BoardNarrowDayStrip.vue'
import BoardNarrowGroupToggle from './BoardNarrowGroupToggle.vue'
import BoardNarrowLaneList from './BoardNarrowLaneList.vue'
import BoardNarrowOtherGroup from './BoardNarrowOtherGroup.vue'

const props = defineProps<{
  boardLabel: string
  rows: ObservationLaneRow[]
  dormantRows: ObservationLaneRow[]
  axis: string[]
  observedDays?: ObservedDayNode[]
  today: string
  sourceDays: number
  selectedLaneId: string | null
  resolveDay: LaneDayResolver
  expandedDormant: boolean
  loading: boolean
  error: string | null
  emptyState: 'no-reports' | 'no-lanes' | null
  /** G1 真实报日集合（可选；省略 = 未接线）。null = 已接线但索引未就绪/失败。 */
  reportDates?: ReadonlySet<string> | null
  /** 壳层唯一选中日（可选；省略 = 未接线，只影响报日条高亮与点行携带日期）。 */
  selectedDate?: string | null
  indexLoading?: boolean
  indexError?: string | null
  /** G1 buildOtherTopics 视图（可选；省略 = 未接线，保持旧窄屏结构）。 */
  otherTopics?: OtherTopicsView | null
  /** 其他组展开态（G4 持有；省略 = 收起）。 */
  othersOpen?: boolean
  selectedOtherSectionId?: string | null
}>()

const emit = defineEmits<{
  selectLane: [topicId: string]
  toggleDormant: []
  retry: []
  generate: []
  gotoChronicle: []
  openContext: []
  /** 点一个有日报的日期（写同一 `selectedDate`，与桌面列头/日格同源）。 */
  selectDayHeader: [date: string]
  selectObservedDay: [node: ObservedDayNode]
  /** 其他组展开/收起（与休眠组互不串联）。 */
  toggleOthers: []
  /** 选其他行（`sectionId` 是当日局部身份；下方阅读槽由 G4 接线）。 */
  selectOther: [payload: { sectionId: string, date: string, topicId?: string | null }]
}>()

/** 迷你条只取所示区间最后 14 日；超过 14 日不横滚。 */
const miniAxis = computed(() => props.axis.slice(-14))
const activeRows = computed(() => props.rows.filter(row => !row.dormant))
const visibleRows = computed(() => (props.expandedDormant
  ? [...activeRows.value, ...props.dormantRows]
  : [...activeRows.value]))

/** 迷你条分档上限按可见行统一算（活跃 + 展开的休眠），避免同屏两把尺。 */
const maxSections = computed(() => {
  let max = 1
  for (const row of visibleRows.value) {
    for (const date of miniAxis.value) {
      const count = narrowDaySectionCount(props.resolveDay, row.topicId, date)
      if (count !== null && count > max) max = count
    }
  }
  return max
})

function sectionsOf(row: ObservationLaneRow, date: string): number | null {
  return narrowDaySectionCount(props.resolveDay, row.topicId, date)
}

function bucket(value: number | null): string {
  if (value === null) return 'is-unknown'
  if (value <= 0) return ''
  const ratio = value / maxSections.value
  if (ratio > 0.75) return 'is-b4'
  if (ratio > 0.5) return 'is-b3'
  if (ratio > 0.25) return 'is-b2'
  return 'is-b1'
}

function cellTitle(row: ObservationLaneRow, date: string): string {
  const count = sectionsOf(row, date)
  if (count === null) return `${date} 无分区/未取到`
  return `${date} ${count} 个分区`
}

interface NarrowLaneCell {
  date: string
  bucket: string
  title: string
}

function cellsOf(row: ObservationLaneRow): NarrowLaneCell[] {
  return miniAxis.value.map(date => ({
    date,
    bucket: bucket(sectionsOf(row, date)),
    title: cellTitle(row, date),
  }))
}

function modelsOf(rows: ObservationLaneRow[]): { row: ObservationLaneRow, cells: NarrowLaneCell[] }[] {
  return rows.map(row => ({ row, cells: cellsOf(row) }))
}

const activeModels = computed(() => modelsOf(activeRows.value))
const dormantModels = computed(() => modelsOf(props.dormantRows))

/** 报日条接线：任一新 prop 出现即渲染（省略则保持旧窄屏结构）。 */
const stripWired = computed(() => props.reportDates !== undefined
  || props.indexLoading === true
  || props.indexError != null)

const dormantExpanded = computed(() => props.expandedDormant && props.dormantRows.length > 0)
const dormantNote = computed(() => {
  if (props.dormantRows.length === 0) return '本窗无'
  return props.expandedDormant ? '收起' : '展开'
})

function selectDayHeader(date: string): void {
  emit('selectDayHeader', date)
}

function selectOther(row: OtherTopicRow): void {
  const date = props.selectedDate ?? null
  if (date === null) return
  emit('selectOther', { sectionId: row.sectionId, date, topicId: row.persistentTopicId })
}

function toggleDormant(): void {
  if (props.dormantRows.length === 0) return
  emit('toggleDormant')
}
</script>

<template>
  <section class="bns" data-testid="board-narrow-summary">
    <header class="bns-head">
      <div>
        <p class="bns-kicker">泳道摘要 · 窄屏</p>
        <h2 :title="boardLabel">{{ boardLabel }}</h2>
      </div>
      <span class="bns-scope">名单/排序/休眠按 {{ sourceDays }} 日档</span>
    </header>

    <BoardNarrowDayStrip
      v-if="stripWired"
      :axis="axis"
      :observed-days="observedDays"
      :report-dates="reportDates"
      :selected-date="selectedDate"
      :index-loading="indexLoading"
      :index-error="indexError"
      @select-day="selectDayHeader"
      @select-observed-day="node => emit('selectObservedDay', node)"
    />

    <div v-if="loading && rows.length === 0" class="bns-skeleton" aria-label="加载中">
      <div v-for="i in 4" :key="i" />
    </div>

    <div v-else-if="error && rows.length === 0" class="bns-error" role="alert">
      <span>{{ error }}</span>
      <button type="button" data-testid="narrow-retry" @click="emit('retry')">重试</button>
    </div>

    <div v-else-if="emptyState === 'no-reports'" class="bns-empty">
      <p>该版块还没有日报，泳道动态基于每日日报结算。</p>
      <button type="button" data-testid="narrow-generate" @click="emit('generate')">生成第一份日报</button>
    </div>

    <template v-else>
      <p v-if="rows.length === 0 && emptyState === 'no-lanes'" class="bns-inline" data-testid="narrow-no-lanes-note">
        0 正式泳道——当日内容见下方「当日其他话题」。
      </p>
      <p v-else-if="rows.length === 0" class="bns-inline">当前观察范围没有泳道数据。</p>
      <p v-else-if="activeRows.length === 0" class="bns-inline">0 正式泳道 · 当日内容见下方「当日其他话题」。</p>

      <BoardNarrowLaneList
        :rows="activeModels"
        :cols="miniAxis.length"
        :selected-lane-id="selectedLaneId"
        testid="narrow-active-lanes"
        @select-lane="id => emit('selectLane', id)"
        @goto-chronicle="emit('gotoChronicle')"
      />

      <BoardNarrowOtherGroup
        v-if="otherTopics !== undefined"
        :view="otherTopics ?? null"
        :open="othersOpen === true"
        :selected-other-section-id="selectedOtherSectionId ?? null"
        @toggle="emit('toggleOthers')"
        @select="selectOther"
      />

      <section class="bns-group" data-testid="narrow-dormant-group">
        <BoardNarrowGroupToggle
          title="休眠泳道"
          :count="String(dormantRows.length)"
          :note="dormantNote"
          :expanded="dormantExpanded"
          :disabled="dormantRows.length === 0"
          testid="narrow-dormant-toggle"
          @toggle="toggleDormant"
        />

        <BoardNarrowLaneList
          v-if="expandedDormant && dormantRows.length > 0"
          :rows="dormantModels"
          :cols="miniAxis.length"
          :selected-lane-id="selectedLaneId"
          testid="narrow-dormant-lanes"
          dormant
          @select-lane="id => emit('selectLane', id)"
          @goto-chronicle="emit('gotoChronicle')"
        />
      </section>
    </template>

    <div class="bns-actions">
      <button type="button" class="bns-context" data-testid="narrow-open-context" @click="emit('openContext')">
        <Icon icon="mdi:radar" width="14" />
        本板块上下文 ↗
      </button>
    </div>
  </section>
</template>

<style scoped>
.bns {
  display: flex;
  flex-direction: column;
  gap: 0.7rem;
  min-width: 0;
}

.bns-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 0.8rem;
  padding-bottom: 0.6rem;
  border-bottom: 1px solid var(--color-border-medium);
}

.bns-kicker {
  margin: 0 0 0.2rem;
  color: var(--color-text-muted);
  font-size: 0.58rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.bns-head h2 {
  margin: 0;
  max-width: 15rem;
  overflow: hidden;
  color: var(--color-text-primary);
  font-family: var(--font-serif, serif);
  font-size: 1.05rem;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bns-scope {
  flex: 0 0 auto;
  color: var(--color-text-muted);
  font-size: 0.62rem;
  line-height: 1.5;
  text-align: right;
}

.bns-skeleton {
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
}

.bns-skeleton div {
  height: 3.4rem;
  border-radius: 5px;
  background: var(--color-bg-hover);
  animation: bns-pulse 1.1s ease-in-out infinite alternate;
}

@keyframes bns-pulse {
  from { opacity: 0.55; }
  to { opacity: 1; }
}

.bns-error,
.bns-empty {
  padding: 1rem 0.2rem;
  color: var(--color-text-muted);
  font-size: 0.75rem;
}

.bns-error {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  color: var(--color-error);
}

.bns-error button,
.bns-empty button {
  border: 0;
  background: none;
  color: var(--color-accent);
  font: inherit;
  text-decoration: underline;
  cursor: pointer;
}

.bns-empty p {
  margin: 0 0 0.5rem;
}

.bns-inline {
  margin: 0;
  padding: 0.3rem 0.1rem;
  color: var(--color-text-muted);
  font-size: 0.72rem;
  line-height: 1.6;
}

/* ── 折叠组容器（休眠组；其他组容器样式随其组件） ── */
.bns-group {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  border-top: 1px solid var(--color-border-subtle, var(--color-border-medium));
  padding-top: 0.35rem;
  min-width: 0;
}

.bns-actions {
  display: flex;
  justify-content: flex-start;
  padding-top: 0.4rem;
}

.bns-context {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.35rem 0.6rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  background: none;
  color: var(--color-text-secondary);
  font-size: 0.72rem;
  cursor: pointer;
}

.bns-context:hover {
  color: var(--color-accent);
  border-color: var(--color-accent);
}
</style>
