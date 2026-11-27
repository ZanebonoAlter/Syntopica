<script setup lang="ts">
/**
 * 观察矩阵网格（rework-workshop-into-radar-and-desk 矩阵阶段 G3 + 8C G2）。
 *
 * 泳道 × 日期网格本体：格值 = 当日线索合计（旧响应回退各桶已知**非综合稿**线索合计，
 * 部分桶不可数时写下界「≥N」不冒充完整值）、
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
 *
 * `lane-matrix-coverage-and-others-row` 增量（仍纯受控）：
 *  - **「其他话题（未入泳道）」整行**：活跃行之后、当日其他话题组之前；逐日格 = 当日未入
 *    泳道分区的非综合稿线索数（与泳道格同口径）。后端旧版本没有该字段时**整行不渲染**
 *    （`otherTopicDaysAvailable` 为 false），不显示全「—」的空壳行。
 *  - **覆盖组「窗口内未进日报」**（`lane-coverage-entry-list` 起）：活跃但当前窗口无分区的
 *    泳道（壳层用既有 topics 清单派生）**不再占矩阵网格行**，改为 `.omx-scroll` 之外、`.omx-matrix`
 *    内的一条折叠条入口（名字常显、默认折叠）+ 点击展开的无格列表（列表项复用
 *    `ObservationMatrixLaneRow` 的 `coverage` 分支，`cells=[]`）；清单读取失败只出一行提示 + 重试
 *    （不冒「没有窗口外泳道」），成功且为空则整组不渲染。展开态是本组件本地状态。
 *  - **行头就地重命名**：`renameLane` 逐层下传（缺省 = 无该入口，菜单不显示「重命名」）。
 */
import { computed, ref } from 'vue'
import type { ObservedDayNode } from '~/features/tags/composables/useOtherTopicObservation'
import { Icon } from '@iconify/vue'
import { useNotify } from '~/composables/useNotify'
import type { WindowOtherTopicDay } from '~/api/normalizers/laneDynamicsWindow'
import DayOtherTopicsGroup from './DayOtherTopicsGroup.vue'
import ObservationMatrixDayHeader from './ObservationMatrixDayHeader.vue'
import ObservationMatrixLaneRow from './ObservationMatrixLaneRow.vue'
import ObservationMatrixOtherRow from './ObservationMatrixOtherRow.vue'
import { OTHER_TOPICS_INDEX_LOADING_TEXT, type OtherTopicsView } from './dayOtherTopics'
import type { CoverageLaneRow } from './coverageLanes'
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
  /**
   * 「其他话题（未入泳道）」逐日读数（date → 未入泳道分区数与非综合稿线索数）。
   * 命名与组视图 `otherTopics` 区分开：那个是「当日」组视图，这个是**逐日**读数。
   */
  otherTopicDays?: Record<string, WindowOtherTopicDay>
  /** 后端是否下发 `other_topics`：false（旧后端）→ 整行不渲染。 */
  otherTopicDaysAvailable?: boolean
  /**
   * 「其他话题」逐日读数的**档窗起点**（YYYY-MM-DD）：后端只聚合 `[anchor-(days-1), anchor]`。
   * 该日之前（观察范围拖到 365 池区段）没有读数是「未聚合」而不是「没有日报」。
   */
  otherTopicsSince?: string | null
  /** 覆盖组行（窗口内未进日报的活跃泳道；壳层用既有 topics 清单派生）。 */
  coverageLanes?: CoverageLaneRow[]
  /** 覆盖清单读取中（不渲染组，不占位、不冒「无覆盖」）。 */
  coverageLoading?: boolean
  /** 覆盖清单读取失败（底部一行提示 + 重试）。 */
  coverageError?: string | null
  /** 就地重命名（缺省 = 无该入口，菜单不显示「重命名」）。 */
  renameLane?: (topicId: string, label: string) => Promise<boolean>
}>()

const emit = defineEmits<{
  selectLane: [topicId: string]
  /** 「编辑泳道」→ 泳道修正工作面（lane-manual-correction-loop 6.3；画布接线由上层壳层做）。 */
  editLane: [topicId: string]
  /** 「归档泳道」→ 归档弹窗（sm 档 + 二次确认）。 */
  archiveLane: [topicId: string]
  /** 点有分区格：就地选日（父更新 selectedDate/下方阅读；不弹日期 modal、不强滚）。 */
  selectDate: [payload: { topicId: string, date: string }]
  /** 点报日列头：与日格同一 `selectedDate` 的第三处入口（壳层统一写状态）。 */
  selectDayHeader: [date: string]
  selectObservedDay: [node: ObservedDayNode]
  toggleDormant: []
  toggleOthers: []
  /** 其他组选行/选格同一事件（sectionId 是当日局部身份）。 */
  selectOther: [payload: { sectionId: string, date: string, topicId?: string | null }]
  /** 点「其他话题（未入泳道）」有日报的格：切选中日 + 展开当日其他话题组。 */
  selectOthersDay: [date: string]
  /** 覆盖清单读取失败后的重试。 */
  retryCoverage: []
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

/**
 * 覆盖组行（窗口内未进日报的活跃泳道）：列表项只带行头，不投影逐日格。
 *
 * 这些泳道不在 A 档名单里，没有格可读（向右看也只是在重复「这行没数据」），
 * 所以不再经 `project()` 造“—”格；日期与名字都在折叠条 / 列表项里只出现一次。
 */
const coverageEntries = computed<{ row: ObservationLaneRow }[]>(() => (props.coverageLanes ?? []).map(lane => ({
  row: {
    topicId: lane.topicId,
    label: lane.label,
    watchLinked: false,
    sectionTotal: 0,
    clueTotal: 0,
    dormant: false,
    snapshot: null,
  },
})))

/** 覆盖组折叠条的名字清单（只列名字；日期只在展开后的列表项里出现一次）。 */
const coverageSummaryText = computed(() => (props.coverageLanes ?? []).map(lane => lane.label).join('、'))

/** 折叠条的展开态（本地状态：刷新清单后不重置，只此一处消费）。 */
const coverageOpen = ref(false)

/**
 * 「其他话题（未入泳道）」整行是否渲染。
 *
 * 两个条件缺一不可：① 后端确实下发了 `other_topics`（旧后端不渲染）；② 窗口内**确有**读数
 * （无 completed 日报时后端恒发空数组，渲染一整行「—」只是噪声）。
 */
const showOtherTopicRow = computed(() =>
  props.otherTopicDaysAvailable === true && Object.keys(props.otherTopicDays ?? {}).length > 0,
)

/** 覆盖组是否渲染：清单取到了且非空（读取中不渲染，失败只出提示行）。 */
const showCoverageGroup = computed(() =>
  props.coverageLoading !== true && !props.coverageError && (props.coverageLanes?.length ?? 0) > 0,
)

/** 覆盖行的最近命中日期（供列表项的行头 title 与「最近命中」标用）。 */
function coverageLastSeenOf(topicId: string): string | null {
  return (props.coverageLanes ?? []).find(lane => lane.topicId === topicId)?.lastSeenDate ?? null
}

/** 休眠折叠条的泳道名（既有 spec：折叠条显示数量**与**泳道名）。 */
const dormantNamesText = computed(() => props.dormantRows.map(row => row.label).join(' · '))

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
          :rename-lane="renameLane"
          @select-lane="emit('selectLane', $event)"
          @select-cell="onSelectCell"
          @edit-lane="emit('editLane', $event)"
          @archive-lane="emit('archiveLane', $event)"
        />

        <!-- 「其他话题（未入泳道）」整行：活跃行之后、当日其他话题组之前；
             旧后端无 other_topics 字段时整行不渲染（不显示全「—」空壳行）。 -->
        <ObservationMatrixOtherRow
          v-if="showOtherTopicRow"
          :axis="axis"
          :selected-date="selectedDate"
          :today="today"
          :days="otherTopicDays ?? {}"
          :since="otherTopicsSince ?? null"
          @select-others-day="date => emit('selectOthersDay', date)"
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
            <span
              v-if="dormantRows.length > 0"
              class="omx-dormant-names"
              :title="dormantNamesText"
              :aria-label="dormantNamesText"
              data-testid="dormant-names"
            >{{ dormantNamesText }}</span>
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
            :rename-lane="renameLane"
            @select-lane="emit('selectLane', $event)"
            @select-cell="onSelectCell"
            @edit-lane="emit('editLane', $event)"
            @archive-lane="emit('archiveLane', $event)"
          />
        </template>
      </div>
    </div>

    <!-- 覆盖组「窗口内未进日报」：休眠组之后、矩阵网格之外（不随横滚漂移、不受滚动容器裁切）。
         折叠条入口（名字常显、默认折叠）+ 点击展开的无格列表；读取失败只出一行提示 + 重试
         （不把失败渲染成「没有窗口外泳道」）。 -->
    <div v-if="coverageError" class="omx-group-noterow" data-testid="coverage-error-row">
      <span class="omx-group-notetext">窗口外泳道清单未取到</span>
      <button type="button" class="omx-coverage-retry" data-testid="coverage-retry" @click="emit('retryCoverage')">重试</button>
    </div>

    <template v-else-if="showCoverageGroup">
      <div class="omx-group-summary is-coverage" data-testid="coverage-group-summary">
        <button
          type="button"
          class="omx-group-btn"
          :aria-expanded="coverageOpen"
          data-testid="coverage-toggle"
          @click="coverageOpen = !coverageOpen"
        >
          <Icon :icon="coverageOpen ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="14" />
          <span class="omx-group-title">窗口内未进日报<span class="omx-group-count"> · {{ coverageLanes?.length ?? 0 }}</span></span>
          <span
            class="omx-coverage-list"
            :title="coverageSummaryText"
            :aria-label="coverageSummaryText"
            data-testid="coverage-group-lanes"
          >{{ coverageSummaryText }}</span>
          <span class="omx-group-note">{{ coverageOpen ? '收起' : '展开查看' }}</span>
        </button>
      </div>

      <ul v-if="coverageOpen" class="omx-coverage-items" data-testid="coverage-list">
        <li v-for="entry in coverageEntries" :key="entry.row.topicId" class="omx-coverage-item">
          <ObservationMatrixLaneRow
            :row="entry.row"
            :cells="[]"
            :axis="[]"
            :today="today"
            :selected-lane-id="selectedLaneId"
            :selected-date="selectedDate"
            :dormant="false"
            :coverage="true"
            :coverage-last-seen="coverageLastSeenOf(entry.row.topicId)"
            :rename-lane="renameLane"
            @edit-lane="emit('editLane', $event)"
            @archive-lane="emit('archiveLane', $event)"
          />
        </li>
      </ul>
    </template>
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
  /* ≤31：单屏网格，日期两行显示（完整日期仍在 title/aria），不制造容器内横滚。
     标签列宽（2026-11-26 用户反馈泳道名展示不全）：先把日格留到 2.6rem（两位数 + 日期短格式的下限），
     剩下的全给标签列，再卡上下界 7rem / 19rem（约 20 个汉字）。旧值 `minmax(7rem, 10rem)` 是写死上限：
     1920 屏上日格 70px 只放 1–3 个数（一半白用电、池），标签列却卡在 160px。
     实测（1920）：标签列 304px、全泳道名零截断、日格 67.6px；1600：304px / 零截断；1440：262px。 */
  grid-template-columns:
    minmax(7rem, max(10rem, min(19rem, calc(100% - var(--matrix-cols, 14) * 2.6rem))))
    repeat(var(--matrix-cols, 14), minmax(0, 1fr));
  gap: 0.12rem;
  align-items: center;
  min-width: 0;
}

/* >31：契约允许仅矩阵容器内横滚；标签列 sticky，日期保留单行短格式。
   此档容器是 `min-width: max-content`（宽不定的百分比会退化成 auto），故只给固定 rem 上限。 */
.omx-grid.is-scrollable {
  grid-template-columns: minmax(9rem, 13rem) repeat(var(--matrix-cols, 14), minmax(2.1rem, 1fr));
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
  flex: 0 0 auto;
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

.omx-group-noterow,
.omx-grid :deep(.omx-group-noterow) {
  grid-column: 1 / -1;
  padding: 0.35rem 0.6rem;
  border-bottom: 1px dotted var(--color-border-subtle);
  font-size: 0.72rem;
  color: var(--color-text-muted);
}

/* 覆盖组（窗口内未进日报）：折叠条里的泳道名清单（超长截断，title/aria 保留全文）。 */
.omx-coverage-list {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.68rem;
  color: var(--color-text-muted);
}

/* 覆盖列表（窗口内未进日报）：普通列表而不是网格（没有日期格）。
   左缩进对齐折叠条标题：按钮内边距 0.6rem + chevron 14px + gap 0.4rem。 */
.omx-coverage-items {
  list-style: none;
  margin: 0;
  padding: 0 0 0 1.85rem;
}

.omx-coverage-item + .omx-coverage-item {
  border-top: 1px dotted var(--color-border-subtle);
}

/* 行头的 sticky / 手型光标是给网格准备的：列表里没有横滚，也不可选行 → 就地改回静态。 */
.omx-coverage-item :deep(.omx-lane) {
  position: static;
  cursor: default;
}

/* 休眠折叠条里的泳道名（既有 spec：折叠条显示数量与泳道名）。 */
.omx-dormant-names {
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.68rem;
  color: var(--color-text-muted);
}

.omx-coverage-retry {
  flex: 0 0 auto;
  margin-left: 0.4rem;
  padding: 0 0.3rem;
  border: 1px solid currentColor;
  border-radius: 4px;
  background: none;
  color: var(--color-accent);
  font-size: 0.68rem;
  cursor: pointer;
}
</style>
