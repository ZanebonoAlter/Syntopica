<script setup lang="ts">
/**
 * 观察矩阵（rework-workshop-into-radar-and-desk 矩阵阶段 G3，矩阵阶段批准版面）。
 *
 * 本组件 = 头部说明（口径说明/数据截止/查看所选日）+ 状态分层
 * （loading 骨架 / 首次错误 / 刷新失败 stale / 无日报 / 有日报无泳道）+ 网格挂载
 * （`ObservationMatrixGrid`：行×日格、休眠折叠、日格三态；格值 = 当日线索合计，
 *  色深 = 该值在本行可见窗内的归一连续深浅（冗余强化通道，数值 + title/aria 仍承载全部信息），
 *  retire-story-adopt-event-metrics D4/D5 + 2026-11-11 看版后修订 tasks 3.5）。
 *
 * 受控契约：数据全由父（G5）经 props 注入，组件不自建网络/日期/选中状态；
 * 点格只转发 selectDate（父就地更新选中日与下方阅读；不开弹层、不强滚——
 * 「查看所选日 ↓」是显式导航）。档位控件已内聚在 G2 ObservationRangeControls，
 * 本组件不渲染第二组。行/格细节见子组件与 observationLanes 模型。
 *
 * 8C G2 增量（日报日驱动的日期轴 + 当日其他话题组，仍纯受控）：
 *  - 0 正式泳道不再用整块 `no-lanes` 空态替换网格：轴与网格照画（0 行），空态降为网格前
 *    的一行内联说明（生成入口义务不变）；初次 loading/A 错误与「已成功空集」分层不变。
 *  - 报日列头可选（`reportDates` 三态见 ObservationMatrixGrid）→ 转发 `selectDayHeader`；
 *    组/阅读选择经 `selectOther` 转发；两个 toggle 事件各自独立（`toggleDormant`/`toggleOthers`）。
 *  - 组状态/行/计数全部由壳层用 G1 `buildOtherTopics` 派生后经 `otherTopics` 下发。
 */
import { computed } from 'vue'
import type { ObservedDayNode } from '~/features/tags/composables/useOtherTopicObservation'
import { Icon } from '@iconify/vue'
import ObservationMatrixGrid from './ObservationMatrixGrid.vue'
import { shiftDate } from './observationRange'
import type { OtherTopicsView } from './dayOtherTopics'
import type { LaneDayResolver, ObservationLaneRow } from './observationLanes'

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
  /** source 档天数（footnote「名单/排序/休眠按 N 日档」）。 */
  sourceDays: number
  /** 数据锚点（YYYY-MM-DD）：档窗起点派生 + 早于今天时提示数据截止。 */
  anchorDate: string | null
  /** 单日内容解析（G1 resolveLaneDay 注入；content/gap/unavailable 三态）。 */
  resolveDay: LaneDayResolver
  expandedDormant: boolean
  loading: boolean
  error: string | null
  staleNotice: boolean
  emptyState: 'no-reports' | 'no-lanes' | null
  dataAsOf?: string | null
  /** 真实报日集合：省略 = 未接线；null = 已接线未就绪/失败；Set = 已知报日列头可选。 */
  reportDates?: ReadonlySet<string> | null
  /** 报日索引读取中/失败（列头提示用；已知报日在陈旧索引下仍可点）。 */
  indexLoading?: boolean
  indexError?: string | null
  /** 当日其他话题组视图模型（G1 派生；省略/null = 壳层尚未接线）。 */
  otherTopics?: OtherTopicsView | null
  /** 其他组展开态（壳层持有；省略 = 收起）。 */
  othersOpen?: boolean
  /** 当前阅读槽选中的其他分区（高亮用）。 */
  selectedOtherSectionId?: string | null
}>()

const emit = defineEmits<{
  selectLane: [topicId: string]
  /** 「编辑泳道」→ 泳道修正工作面（lane-manual-correction-loop 6.3）。 */
  editLane: [topicId: string]
  /** 「归档泳道」→ 归档弹窗。 */
  archiveLane: [topicId: string]
  selectDate: [payload: { topicId: string, date: string }]
  /** 点报日列头（与日格共用同一 selectedDate；壳层统一写状态）。 */
  selectDayHeader: [date: string]
  selectObservedDay: [node: ObservedDayNode]
  /** 其他组选行/选格（sectionId 是当日局部身份）。 */
  selectOther: [payload: { sectionId: string, date: string, topicId?: string | null }]
  toggleDormant: []
  toggleOthers: []
  retry: []
  /** 「查看所选日 ↓」显式导航交父（滚动到下方编年由父决定）。 */
  gotoSelected: []
  generate: []
}>()
/** 档窗起点：可见轴跨出档窗的日期 = 365 池补内容区段（footnote 标注，不改名单）。 */
const usesPoolHistory = computed(() => {
  const anchor = props.anchorDate
  if (!anchor || props.axis.length === 0) return false
  return props.axis[0]! < shiftDate(anchor, -(props.sourceDays - 1))
})

const showSkeleton = computed(() => props.loading && props.activeRows.length === 0 && props.dormantRows.length === 0)
const showError = computed(
  () =>
    !props.loading &&
    props.error !== null &&
    props.emptyState === null &&
    props.activeRows.length === 0 &&
    props.dormantRows.length === 0,
)
</script>

<template>
  <section class="omx" data-testid="board-matrix">
    <div class="omx-head">
      <span class="omx-kicker">泳道 × 日期</span>
      <span class="omx-hint" data-testid="matrix-legend">数字 = 当日线索合计（该泳道当天各主题桶的非综合稿线索数之和）· '—' 无分区 · '?' 未取到</span>
      <span class="omx-note" data-testid="matrix-tier-note">
        名单/排序/休眠按 {{ sourceDays }} 日档<span v-if="usesPoolHistory"> · 历史由 365 日池补充（不改名单）</span>
      </span>
      <span v-if="dataAsOf && dataAsOf < today" class="omx-asof" data-testid="matrix-data-asof">
        数据截止 {{ dataAsOf }}（之后没有新日报）
      </span>
      <button
        v-if="selectedDate"
        type="button"
        class="omx-goto"
        data-testid="matrix-goto-selected"
        :title="`跳到下方阅读 ${selectedDate}`"
        @click="emit('gotoSelected')"
      >查看所选日 ↓</button>
    </div>

    <!-- 常驻口径说明（fix-lane-metric-provenance）：数字 = 当日线索合计 / 综合稿不计入。
         只读文本行，不是入口；复用 muted 小字样式，不做强调色、不可交互。 -->
    <p class="omx-basis" data-testid="matrix-basis-note">
      数字 = 当日线索合计（各主题桶非综合稿线索数之和；综合稿不计入，避免一条稿给多条泳道各灌热度）
    </p>

    <!-- 刷新失败但保留旧数据：顶部提示条（状态矩阵 error 行） -->
    <div v-if="staleNotice && error" class="omx-stale" data-testid="matrix-stale-notice">
      <Icon icon="mdi:alert-circle-outline" width="14" />
      <span>刷新失败（{{ error }}），当前显示的是之前的数据</span>
      <button type="button" class="omx-link" @click="emit('retry')">重试</button>
    </div>

    <!-- loading：网格骨架 -->
    <div v-if="showSkeleton" class="omx-skeleton" data-testid="matrix-skeleton">
      <div v-for="i in 5" :key="i" class="omx-skeleton-row" />
    </div>

    <!-- error（无旧数据）：内联错误条 + 重试 -->
    <div v-else-if="showError" class="omx-error" data-testid="matrix-error">
      <Icon icon="mdi:alert-circle-outline" width="16" />
      <span>{{ error }}</span>
      <button type="button" class="omx-link" data-testid="matrix-retry" @click="emit('retry')">重试</button>
    </div>

    <!-- 空态降为网格前的一行内联说明：轴与网格照画（0 行），不再用整块空态替换网格；
         生成入口义务不变；初次 loading/A 错误已在上面分支分层，不误列「无正式」 -->
    <template v-else>
      <div v-if="emptyState === 'no-reports'" class="omx-inline" data-testid="matrix-empty-no-reports">
        <span class="omx-inline-text">该版块还没有日报，泳道动态基于每日日报结算；生成第一份日报后这里会出现各条泳道最近发生的事。</span>
        <button
          type="button"
          class="omx-inline-action"
          data-testid="matrix-empty-generate"
          @click="emit('generate')"
        >去生成第一份日报</button>
      </div>
      <div v-else-if="emptyState === 'no-lanes'" class="omx-inline" data-testid="matrix-empty-no-lanes">
        <span class="omx-inline-text">
          <b>0 正式泳道</b> · {{ othersOpen ? '已展开当日其他话题' : '可展开当日其他话题' }}
        </span>
      </div>

      <!-- 网格：行 × 日格（格值峰值/三态/报日列头/组顺序/容器内横滚）在 ObservationMatrixGrid -->
      <ObservationMatrixGrid
        :axis="axis"
        :observed-days="observedDays"
        :active-rows="activeRows"
        :dormant-rows="dormantRows"
        :selected-lane-id="selectedLaneId"
        :selected-date="selectedDate"
        :today="today"
        :resolve-day="resolveDay"
        :expanded-dormant="expandedDormant"
        :report-dates="reportDates"
        :index-loading="indexLoading === true"
        :index-error="indexError ?? null"
        :other-topics="otherTopics ?? null"
        :others-open="othersOpen === true"
        :selected-other-section-id="selectedOtherSectionId ?? null"
        @select-lane="id => emit('selectLane', id)"
        @edit-lane="id => emit('editLane', id)"
        @archive-lane="id => emit('archiveLane', id)"
        @select-date="payload => emit('selectDate', payload)"
        @select-day-header="date => emit('selectDayHeader', date)"
        @select-observed-day="node => emit('selectObservedDay', node)"
        @select-other="payload => emit('selectOther', payload)"
        @toggle-dormant="emit('toggleDormant')"
        @toggle-others="emit('toggleOthers')"
      />
    </template>
  </section>
</template>

<style scoped>
.omx {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  min-height: 0;
}

.omx-head {
  display: flex;
  align-items: baseline;
  gap: 0.8rem;
  flex-wrap: wrap;
}

.omx-kicker {
  font-size: 0.68rem;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: var(--color-text-muted);
}

.omx-hint {
  font-size: 0.74rem;
  color: var(--color-text-muted);
  opacity: 0.8;
}

.omx-hint.is-warn {
  color: var(--color-warning);
  opacity: 1;
}

/* 常驻口径说明行：muted 小字、不可交互（与图例同档，不用强调色）。 */
.omx-basis {
  margin: 0;
  font-size: 0.7rem;
  color: var(--color-text-muted);
  opacity: 0.8;
}

.omx-note {
  font-size: 0.7rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.omx-asof {
  margin-left: auto;
  font-size: 0.7rem;
  color: var(--color-warning);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.omx-goto {
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  padding: 0.1rem 0.45rem;
  background: none;
  color: var(--color-accent);
  font-size: 0.7rem;
  cursor: pointer;
  white-space: nowrap;
}

.omx-goto:hover {
  background: var(--color-accent-subtle);
}

.omx-stale {
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

.omx-link {
  margin-left: auto;
  border: none;
  background: none;
  color: var(--color-accent);
  font-size: 0.72rem;
  cursor: pointer;
  padding: 0;
}

.omx-link:hover {
  text-decoration: underline;
}

.omx-skeleton {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  padding: 0.4rem 0;
}

.omx-skeleton-row {
  height: 2.9rem;
  border-radius: 6px;
  background: var(--color-bg-hover);
  animation: omx-pulse 1.4s ease-in-out infinite;
}

@keyframes omx-pulse {
  0%, 100% { opacity: 0.55; }
  50% { opacity: 1; }
}

.omx-error {
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

.omx-inline {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  flex-wrap: wrap;
  padding: 0.4rem 0.6rem;
  border-left: 3px solid var(--color-border-medium);
  border-radius: 0 6px 6px 0;
  background: var(--color-bg-elevated);
  font-size: 0.76rem;
  color: var(--color-text-muted);
}

.omx-inline-text {
  min-width: 0;
}

.omx-inline-text b {
  color: var(--color-text-secondary);
  font-weight: 600;
}

.omx-inline-action {
  margin-left: auto;
  padding: 0.15rem 0.5rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  background: none;
  color: var(--color-accent);
  font-size: 0.72rem;
  cursor: pointer;
  white-space: nowrap;
}

.omx-inline-action:hover {
  background: var(--color-accent-subtle);
}
</style>
