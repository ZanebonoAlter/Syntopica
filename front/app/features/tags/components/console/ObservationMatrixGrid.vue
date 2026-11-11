<script setup lang="ts">
/**
 * 观察矩阵网格（rework-workshop-into-radar-and-desk 矩阵阶段 G3 + 8C G2）。
 *
 * 泳道 × 日期网格本体：格值 = 当日线索合计（旧响应回退各桶已知线索合计）、
 * 色深 = 该值在本行可见窗内的归一深浅（`clueDepth`，行内归一只；颜色只是冗余强化通道）；
 * 无分区 = '—' 禁用；历史未取到 = '?' 禁用；有分区 0 线索 = '0' 可开；**无行尾合计列**。
 * 名单/行序/休眠由父注入的 rows 携带，本组件只做逐可见日投影（resolveDay）与渲染；
 * >31 列容器内横滚、标签列 sticky。
 *
 * 8C G2 增量（日报日驱动的日期轴 + 当日其他话题组，纯受控）：
 *  - **日期列头**：`reportDates` 三态——省略 = 未接线（全不可点，旧调用零改动）；
 *    `null` = 已接线但索引未就绪/失败（不可点且不冒「无日报」）；`Set` = 真实报日（只有集合内
 *    日期是原生 button，点它 emit `selectDayHeader(date)`，与日格共用壳层同一 `selectedDate`）。
 *  - **组顺序**：正式活跃行 → 当日其他话题组（`DayOtherTopicsGroup`：summary + 自身行）→
 *    休眠 summary + 休眠行。两个 summary 都是纵向全宽、原生 button + `aria-expanded`、
 *    互不串联；休眠 0 行仍保留同级 summary（「本窗无」）且禁用展开，不造行。
 *  - 组行/组状态全由父经 `otherTopics`（G1 模型视图）注入，本组件不重算排除/计数/状态词。
 */
import { computed } from 'vue'
import type { ObservedDayNode } from '~/features/tags/composables/useOtherTopicObservation'
import { Icon } from '@iconify/vue'
import { useNotify } from '~/composables/useNotify'
import DayOtherTopicsGroup from './DayOtherTopicsGroup.vue'
import ObservationMatrixDayHeader from './ObservationMatrixDayHeader.vue'
import ObservationMatrixLaneRow from './ObservationMatrixLaneRow.vue'
import { OTHER_TOPICS_INDEX_LOADING_TEXT, type OtherTopicsView } from './dayOtherTopics'
import {
  clueRowMax,
  matrixCellView,
  projectMatrixCell,
  type LaneDayResolver,
  type MatrixCellView,
  type ObservationLaneRow,
} from './observationLanes'

const props = defineProps<{
  /** 可见观察轴（升序 date-only；显示裁切，行序/休眠不随它重算）。 */
  axis: string[]
  /** 当前其他话题的同 ID 可见窗记录日（壳层 gate 后下发）。 */
  observedDays?: ObservedDayNode[]
  activeRows: ObservationLaneRow[]
  dormantRows: ObservationLaneRow[]
  selectedLaneId: string | null
  selectedDate: string | null
  today: string
  /** 单日内容解析（G1 resolveLaneDay 注入；content/gap/unavailable 三态）。 */
  resolveDay: LaneDayResolver
  expandedDormant: boolean
  /** 真实报日集合：省略 = 未接线；null = 已接线未就绪/失败；Set = 已知报日可选。 */
  reportDates?: ReadonlySet<string> | null
  /** 报日索引读取中（列头提示用）。 */
  indexLoading?: boolean
  /** 报日索引失败（陈旧）文案：只用于提示与列头 title，不一刀禁掉已知报日。 */
  indexError?: string | null
  /** 当日其他话题组视图模型（G1 `buildOtherTopics` 派生；null/省略 = 未接线）。 */
  otherTopics?: OtherTopicsView | null
  /** 其他组展开态（壳层持有；省略 = 收起）。 */
  othersOpen?: boolean
  /** 当前阅读槽选中的其他分区（高亮用）。 */
  selectedOtherSectionId?: string | null
}>()

const emit = defineEmits<{
  selectLane: [topicId: string]
  /** 点有分区格：就地选日（父更新 selectedDate/下方阅读；不弹日期 modal、不强滚）。 */
  selectDate: [payload: { topicId: string, date: string }]
  /** 点报日列头：与日格同一 `selectedDate` 的第三处入口（壳层统一写状态）。 */
  selectDayHeader: [date: string]
  selectObservedDay: [node: ObservedDayNode]
  toggleDormant: []
  toggleOthers: []
  /** 其他组选行/选格同一事件（sectionId 是当日局部身份）。 */
  selectOther: [payload: { sectionId: string, date: string, topicId?: string | null }]
}>()

const { warn: notifyWarn } = useNotify()

interface RenderEntry {
  row: ObservationLaneRow
  dormant: boolean
  cells: MatrixCellView[]
}

function project(row: ObservationLaneRow, dormant: boolean): RenderEntry {
  // 两遍：先把本行可见窗投影成原始格（拿 clueCount）→ 取行内归一分母 → 再出展示视图（色深行内归一）
  const rawCells = props.axis.map(date => projectMatrixCell(props.resolveDay, row.topicId, date))
  const rowMax = clueRowMax(rawCells.map(cell => (cell.kind === 'content' ? cell.metrics.clueCount : null)))
  return {
    row,
    dormant,
    cells: rawCells.map((cell, index) =>
      matrixCellView(cell, { laneLabel: row.label, date: props.axis[index]!, rowMax }),
    ),
  }
}

/** 正式行（活跃泳道）；休眠行排在两组 summary 之后单独渲染（组序见文件头）。 */
const activeEntries = computed<RenderEntry[]>(() => props.activeRows.map(row => project(row, false)))
const dormantEntries = computed<RenderEntry[]>(() => props.dormantRows.map(row => project(row, true)))

const scrollable = computed(() => props.axis.length > 31)
const gridStyle = computed(() => ({ '--matrix-cols': String(props.axis.length) }))

/** 索引提示：加载中/失败给出内联说明；陈旧但已有旧集合时已知报日照常可点（不一刀禁）。 */
const indexHint = computed<string | null>(() => {
  const dates = props.reportDates
  if (dates === undefined) return null
  if (dates === null) {
    if (props.indexLoading) return OTHER_TOPICS_INDEX_LOADING_TEXT
    if (props.indexError) return '报日索引未取到，日期列头暂不可选'
    return null
  }
  if (props.indexError) return '报日索引刷新失败，日期列头按上次取到的报日可用'
  return null
})

function onCellClick(row: ObservationLaneRow, cell: MatrixCellView, date: string): void {
  if (!cell.openable) {
    // 禁用格真实点击不可达（aria 已带「无分区/未取到」提示）；合成事件兜底只提示不 emit
    if (cell.kind === 'gap') notifyWarn(`${date} 这一天「${row.label}」没有分区`)
    else notifyWarn(`${date} 该日历史数据未取到`)
    return
  }
  emit('selectDate', { topicId: row.topicId, date })
}

/** 行组件只报「点了哪一格」；可开性判定/禁用格提示仍在本层（与拆分前同一出口）。 */
function onSelectCell(payload: { row: ObservationLaneRow, cell: MatrixCellView, date: string }): void {
  onCellClick(payload.row, payload.cell, payload.date)
}
</script>

<template>
  <div class="omx-matrix">
    <div v-if="indexHint" class="omx-index-hint" data-testid="matrix-report-index-hint">
      <span>{{ indexHint }}</span>
    </div>

    <div class="omx-scroll" data-testid="matrix-scroll">
      <div class="omx-grid" :class="{ 'is-scrollable': scrollable }" :style="gridStyle" data-testid="matrix-grid">
        <ObservationMatrixDayHeader
          :axis="axis"
          :observed-days="observedDays"
          :today="today"
          :selected-date="selectedDate"
          :report-dates="reportDates"
          :index-error="indexError"
          @select-day-header="date => emit('selectDayHeader', date)"
          @select-observed-day="node => emit('selectObservedDay', node)"
        />

        <ObservationMatrixLaneRow
          v-for="entry in activeEntries"
          :key="entry.row.topicId"
          :row="entry.row"
          :cells="entry.cells"
          :axis="axis"
          :today="today"
          :selected-lane-id="selectedLaneId"
          :selected-date="selectedDate"
          :dormant="false"
          @select-lane="emit('selectLane', $event)"
          @select-cell="onSelectCell"
        />

        <!-- 当日其他话题组（summary + 自身行；display:contents 参与本网格） -->
        <DayOtherTopicsGroup
          :axis="axis"
          :selected-date="selectedDate"
          :view="otherTopics ?? null"
          :expanded="othersOpen === true"
          :selected-section-id="selectedOtherSectionId ?? null"
          @toggle-others="emit('toggleOthers')"
          @select-other="payload => emit('selectOther', payload)"
        />

        <!-- 休眠泳道组：summary 在自己的行之前；0 行仍保留同级 summary（本窗无，禁用展开） -->
        <div class="omx-group-summary is-dormant" data-testid="dormant-toggle">
          <button
            type="button"
            class="omx-group-btn"
            :aria-expanded="expandedDormant"
            :disabled="dormantRows.length === 0"
            @click="emit('toggleDormant')"
          >
            <Icon :icon="expandedDormant ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="14" />
            <span class="omx-group-title">休眠泳道<span class="omx-group-count"> · {{ dormantRows.length }}</span></span>
            <span class="omx-group-note">
              <template v-if="dormantRows.length === 0">本窗无</template>
              <template v-else>{{ expandedDormant ? '收起' : '展开查看' }}（它们仍挂着追踪，只是这段时间没动）</template>
            </span>
          </button>
        </div>

        <template v-if="expandedDormant && dormantRows.length > 0">
          <ObservationMatrixLaneRow
            v-for="entry in dormantEntries"
            :key="entry.row.topicId"
            :row="entry.row"
            :cells="entry.cells"
            :axis="axis"
            :today="today"
            :selected-lane-id="selectedLaneId"
            :selected-date="selectedDate"
            :dormant="true"
            @select-lane="emit('selectLane', $event)"
            @select-cell="onSelectCell"
          />
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 舞台内滚动：可见日 >31（365 档/拖宽）时列数超宽，仅矩阵自身横滚（布局契约例外） */
.omx-scroll {
  overflow-x: auto;
  overflow-y: hidden;
  padding-bottom: 0.2rem;
}

.omx-index-hint {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  margin-bottom: 0.3rem;
  font-size: 0.7rem;
  color: var(--color-text-muted);
}

.omx-grid {
  display: grid;
  /* ≤31：单屏网格，日期两行显示（完整日期仍在 title/aria），不制造容器内横滚 */
  grid-template-columns: minmax(7rem, 10rem) repeat(var(--matrix-cols, 14), minmax(0, 1fr));
  gap: 0.12rem;
  align-items: center;
  min-width: 0;
}

/* >31：契约允许仅矩阵容器内横滚；标签列 sticky，日期保留单行短格式 */
.omx-grid.is-scrollable {
  grid-template-columns: minmax(9rem, 12rem) repeat(var(--matrix-cols, 14), minmax(2.1rem, 1fr));
  gap: 0.24rem;
  min-width: max-content;
}

/* ── 组 summary（当日其他话题 / 休眠泳道同款：纵向全宽、原生 button + aria-expanded） ── */
.omx-group-summary,
.omx-grid :deep(.omx-group-summary) {
  grid-column: 1 / -1;
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  min-width: 0;
  margin-top: 0.35rem;
}

.omx-group-btn,
.omx-grid :deep(.omx-group-btn) {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
  flex: 1 1 auto;
  min-width: 0;
  padding: 0.42rem 0.6rem;
  border: 1px dashed var(--color-border-medium);
  border-radius: 8px;
  background: none;
  color: var(--color-text-muted);
  font-size: 0.76rem;
  text-align: left;
  cursor: pointer;
}

.omx-group-btn:hover:not(:disabled),
.omx-grid :deep(.omx-group-btn:hover:not(:disabled)) {
  color: var(--color-text-secondary);
  border-color: var(--color-border-strong);
}

.omx-group-btn[aria-expanded='true'],
.omx-grid :deep(.omx-group-btn[aria-expanded='true']) {
  border-style: solid;
  border-color: var(--color-border-strong);
  background: var(--color-bg-elevated);
}

.omx-group-btn:disabled,
.omx-grid :deep(.omx-group-btn:disabled) {
  cursor: default;
  opacity: 0.85;
}

.omx-group-title,
.omx-grid :deep(.omx-group-title) {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-secondary);
  font-weight: 600;
}

.omx-group-count,
.omx-grid :deep(.omx-group-count) {
  color: var(--color-text-muted);
  font-weight: 400;
  font-variant-numeric: tabular-nums;
}

.omx-group-note,
.omx-group-extra,
.omx-grid :deep(.omx-group-note),
.omx-grid :deep(.omx-group-extra) {
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.68rem;
  color: var(--color-text-muted);
}

.omx-group-note,
.omx-grid :deep(.omx-group-note) {
  margin-left: auto;
}

.omx-group-extra,
.omx-grid :deep(.omx-group-extra) {
  flex: 0 0 auto;
  max-width: 40%;
}

.omx-grid :deep(.omx-group-noterow) {
  grid-column: 1 / -1;
  padding: 0.35rem 0.6rem;
  border-bottom: 1px dotted var(--color-border-subtle);
  font-size: 0.72rem;
  color: var(--color-text-muted);
}
</style>
