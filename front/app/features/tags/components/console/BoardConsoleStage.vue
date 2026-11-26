<script setup lang="ts">
/**
 * 主舞台（rework-workshop-into-radar-and-desk 矩阵阶段 G5）。
 *
 * 纯受控展示/交互转发：观察窗控件 → 矩阵 → 选中泳道演进 → 当日编年；
 * 或归档视图。壳层持有全部状态，本组件不自建日期/网络/选中态。
 * 窄屏只换列表形态（BoardNarrowSummary），下方编年仍是同一个 LaneChronicle。
 */
import { computed, ref } from 'vue'
import type { ObservedDayNode } from '~/features/tags/composables/useOtherTopicObservation'
import type { ObservationRange, SourceTierDays } from './observationRange'
import type { BoardConsoleStageProps } from './consoleStageTypes'
import ObservationRangeControls from './ObservationRangeControls.vue'
import { POOL_DAYS } from './observationRange'
import BoardNarrowSummary from './BoardNarrowSummary.vue'
import ObservationMatrix from './ObservationMatrix.vue'
import KeyEventTimelineView from './KeyEventTimelineView.vue'
import ObservationLaneEvolution from './ObservationLaneEvolution.vue'
import LaneChronicle from './LaneChronicle.vue'
import OtherTopicReading from './OtherTopicReading.vue'
import BoardArchiveView from './BoardArchiveView.vue'

const emit = defineEmits<{
  selectLane: [topicId: string]
  /** 「编辑泳道」→ 泳道修正工作面（lane-manual-correction-loop 6.3；画布接线由壳层做）。 */
  editLane: [topicId: string]
  /** 「归档泳道」→ 归档弹窗（sm 档 + 二次确认）。 */
  archiveLane: [topicId: string]
  selectCell: [payload: { topicId: string, date: string }]
  selectDate: [date: string]
  toggleDormant: []
  /** 报日列头（与日格共用同一 selectedDate；壳层统一写状态）。 */
  selectDayHeader: [date: string]
  /** 当日其他话题组展开/收起（与休眠组独立）。 */
  toggleOthers: []
  /** 其他组选行/选格（sectionId 是当日局部身份）。 */
  selectOther: [payload: { sectionId: string, date: string, topicId?: string | null }]
  /** 点「其他话题（未入泳道）」有日报的格：切选中日 + 展开当日其他话题组。 */
  selectOthersDay: [date: string]
  /** 覆盖清单（窗口外泳道）读取失败后的重试。 */
  retryCoverage: []
  /** 观察带真实记录日节点（原子切同一 selectedDate + 稳定目标）。 */
  selectObservedDay: [payload: { date: string, sectionId: string, reportId: string, topicId: string }]
  retry: []
  /** 阅读槽受控重试：force 索引 → force 当日详情（不借旧 reportId）。 */
  retryReport: [payload: { date: string, sectionId: string }]
  /** 报日索引/当日报告链重试（G2 组提示无独立按钮，这里给最小正确入口）。 */
  retryReportChain: []
  generate: []
  openContext: []
  openReport: [payload: { date: string, reportId: string | null }]
  viewArchive: [date: string]
  openArticle: [articleId: string]
  previewRange: [range: ObservationRange]
  commitRange: [range: ObservationRange]
  sourceChange: [days: SourceTierDays]
  retryPool: []
  readingSettled: [payload: { laneKey: string, date: string, status: string }]
}>()

const props = defineProps<BoardConsoleStageProps>()

const chronicleRef = ref<HTMLElement | null>(null)

/** 其他分区阅读中抑制泳道高亮（底层 laneId 保留供 URL/切回，不出现双重选中）。 */
const laneHighlightKey = computed(() => (props.selectedOtherSectionId === null ? props.selectedLaneKey : null))

/** 选中其他分区才显示关联日；切换期间不借旧观察目标或窗外记录。 */
const observedDays = computed(() => {
  const view = props.otherObservation.view.value
  if (!props.selectedOtherSectionId || !props.selectedOtherTopicId
    || view.topicId !== props.selectedOtherTopicId || view.status !== 'ready') return []
  const axis = new Set(props.visibleAxis)
  return view.nodes.filter(node => axis.has(node.date))
})

function selectObservedDay(node: ObservedDayNode): void {
  if (!props.selectedOtherTopicId || !observedDays.value.includes(node)) return
  emit('selectObservedDay', {
    date: node.date, sectionId: node.sectionId, reportId: node.reportId, topicId: props.selectedOtherTopicId,
  })
}

/** 当日报告读取失败（display gate：只认当前选中日）。 */
const reportDayError = computed(() => {
  const day = props.reportDay
  if (!day || day.date !== props.selectedDate || day.status !== 'error') return null
  return day.error
})

/** 报日索引/当日报告链失败文案（重试入口在下方，不把重试只接旧 A load）。 */
const reportChainError = computed(() => {
  if (reportDayError.value) return `当日报告读取失败（${reportDayError.value}）`
  if (props.indexError) return `报日索引读取失败（${props.indexError}）`
  return null
})

/** 单一阅读槽空态（0 正式泳道也指向报日独立 + 当日其他话题，不是无路可走）。 */
const readingHint = computed(() => (props.emptyState === 'no-lanes'
  ? '还没有活跃泳道：点报日列头选日报日，再展开「当日其他话题」读当日分区。'
  : '先选一个日期（报日列头或矩阵格），下方显示当日线索；「当日其他话题」可读未进泳道的分区。'))

function scrollToChronicle(): void {
  chronicleRef.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
</script>

<template>
  <BoardArchiveView
    v-if="activeView === 'archive'"
    :board-id="boardId"
    :initial-date="archiveInitialDate ?? undefined"
    :labels="compositionLabels"
    @open-article="id => emit('openArticle', String(id))"
  />

  <template v-else>
    <ObservationRangeControls
      :bounds="poolBounds"
      :range="observation"
      :source-days="sourceDays"
      :series="poolSeries"
      :loading="sourceDays === 365 ? loading : poolLoading"
      :notice="rangeNoticeText"
      @preview="range => emit('previewRange', range)"
      @commit="range => emit('commitRange', range)"
      @source-change="days => emit('sourceChange', days)"
    />

    <!-- 报日索引/当日报告链失败：最小正确重试入口（force 索引 → force 当日报告，不只刷 A） -->
    <div
      v-if="reportChainError"
      class="stage-pool"
      data-testid="matrix-report-chain-notice"
    >
      <span>{{ reportChainError }}</span>
      <button
        type="button"
        data-testid="matrix-report-chain-retry"
        :disabled="indexLoading"
        @click="emit('retryReportChain')"
      >{{ indexLoading ? '重试中…' : '重试报日读取' }}</button>
    </div>

    <!-- 历史池未取到/陈旧：无条件提示（池是首屏背景曲线的数据源，不再等观察范围跨出档窗才提示）；
         365 档即池（档/池共用同一响应）没有独立池可失败，不适用本提示。 -->
    <div
      v-if="sourceDays !== POOL_DAYS && (poolError || poolStaleNotice)"
      class="stage-pool"
      data-testid="matrix-pool-notice"
    >
      <span>
        <template v-if="poolStaleNotice">365 日历史池刷新失败，当前保留上次成功内容</template>
        <template v-else>365 日历史池未取到，历史日显示 ?（不改名单/范围）</template>
        <template v-if="poolError">（{{ poolError }}）</template>
      </span>
      <button type="button" data-testid="matrix-pool-retry" :disabled="poolLoading" @click="emit('retryPool')">{{ poolLoading ? '重试中…' : '重试历史池' }}</button>
    </div>

    <BoardNarrowSummary
      v-if="isNarrow"
      :board-label="boardLabel"
      :rows="allLanes"
      :dormant-rows="dormantLanes"
      :axis="visibleAxis"
      :observed-days="observedDays"
      :today="today"
      :source-days="sourceDays"
      :selected-lane-id="laneHighlightKey"
      :resolve-day="resolveLaneDay"
      :expanded-dormant="expandedDormant"
      :loading="loading"
      :error="error"
      :empty-state="emptyState"
      :report-dates="reportDates"
      :selected-date="selectedDate"
      :index-loading="indexLoading"
      :index-error="indexError"
      :other-topics="otherTopics"
      :others-open="othersOpen"
      :selected-other-section-id="selectedOtherSectionId"
      @select-lane="id => emit('selectLane', id)"
      @toggle-dormant="emit('toggleDormant')"
      @retry="emit('retry')"
      @generate="emit('generate')"
      @goto-chronicle="scrollToChronicle"
      @open-context="emit('openContext')"
      @select-day-header="date => emit('selectDayHeader', date)"
      @select-observed-day="selectObservedDay"
      @toggle-others="emit('toggleOthers')"
      @select-other="payload => emit('selectOther', payload)"
    />

    <template v-else>
      <!-- 关键事件视图（add-key-event-matrix-view）：矩阵骨架的另一个读数，自带当日事件阅读区；
           时间视窗/选中态/行序全部与矩阵共用（D9/D2），候选话题不占行。 -->
      <KeyEventTimelineView
        v-if="activeView === 'events'"
        :board-id="boardId"
        :board-label="boardLabel"
        :axis="visibleAxis"
        :active-rows="activeLanes"
        :dormant-rows="dormantLanes"
        :selected-lane-id="laneHighlightKey"
        :selected-date="selectedDate"
        :today="today"
        :source-days="sourceDays"
        :expanded-dormant="expandedDormant"
        :loading="loading"
        :error="error"
        :stale-notice="staleNotice"
        :empty-state="emptyState"
        :reading="reading"
        @select-lane="(id: string) => emit('selectLane', id)"
        @select-date="(date: string) => emit('selectDate', date)"
        @toggle-dormant="emit('toggleDormant')"
        @retry="emit('retry')"
        @open-article="(id: string) => emit('openArticle', id)"
      />

      <template v-else>
      <ObservationMatrix
        :axis="visibleAxis"
        :observed-days="observedDays"
        :active-rows="activeLanes"
        :dormant-rows="dormantLanes"
        :selected-lane-id="laneHighlightKey"
        :selected-date="selectedDate"
        :today="today"
        :source-days="sourceDays"
        :anchor-date="anchorDate"
        :resolve-day="resolveLaneDay"
        :expanded-dormant="expandedDormant"
        :loading="loading"
        :error="error"
        :stale-notice="staleNotice"
        :empty-state="emptyState"
        :data-as-of="anchorDate"
        :report-dates="reportDates"
        :index-loading="indexLoading"
        :index-error="indexError"
        :other-topics="otherTopics"
        :others-open="othersOpen"
        :selected-other-section-id="selectedOtherSectionId"
        :other-topic-days="otherTopicDays"
        :other-topic-days-available="otherTopicDaysAvailable"
        :other-topics-since="otherTopicsSince ?? null"
        :coverage-lanes="coverageLanes"
        :coverage-loading="coverageLoading"
        :coverage-error="coverageError"
        :rename-lane="renameLane"
        @select-lane="id => emit('selectLane', id)"
        @edit-lane="id => emit('editLane', id)"
        @archive-lane="id => emit('archiveLane', id)"
        @select-date="payload => emit('selectCell', payload)"
        @select-day-header="date => emit('selectDayHeader', date)"
        @select-observed-day="selectObservedDay"
        @select-other="payload => emit('selectOther', payload)"
        @select-others-day="date => emit('selectOthersDay', date)"
        @retry-coverage="emit('retryCoverage')"
        @toggle-dormant="emit('toggleDormant')"
        @toggle-others="emit('toggleOthers')"
        @retry="emit('retry')"
        @goto-selected="scrollToChronicle"
        @generate="emit('generate')"
      />

      <!-- 其他分区阅读中隐藏旧 lane 演进：避免两个演进/阅读面叠 -->
      <ObservationLaneEvolution
        v-if="selectedOtherSectionId === null && allLanes.length > 0"
        :lane="selectedLaneRow"
        :axis="visibleAxis"
        :today="today"
        :selected-date="selectedDate"
        :resolve-day="resolveLaneDay"
        :source-days="sourceDays"
        :loading="loading"
        :error="error"
        :stale-notice="staleNotice"
        @select-date="date => emit('selectDate', date)"
        @retry="emit('retry')"
      />
      </template>
    </template>

    <!-- 阅读槽：事件视图自带当日事件阅读区，故矩阵/追踪/归档视图下才渲染；
         窄屏（isNarrow）例外：窄屏不渲染事件视图，阅读槽不得被连带隐藏
         （否则窄屏深链 ?view=events 会留下一个空舞台，review R-8）。 -->
    <div v-if="activeView !== 'events' || isNarrow" ref="chronicleRef" class="stage-chronicle">
      <!-- 单一阅读槽：其他分区 → OtherTopicReading（含观察带）；否则原 lane 编年；都没有给方向提示 -->
      <OtherTopicReading
        v-if="selectedOtherSectionId"
        :selected-date="selectedDate"
        :selected-section-id="selectedOtherSectionId"
        :topic-id="selectedOtherTopicId"
        :report-day="reportDay"
        :reading="reading"
        :observation="otherObservation"
        @open-report="payload => emit('openReport', payload)"
        @view-archive="date => emit('viewArchive', date)"
        @open-article="id => emit('openArticle', id)"
        @retry-report="payload => emit('retryReport', payload)"
        @reading-settled="payload => emit('readingSettled', payload)"
      />
      <LaneChronicle
        v-else-if="selectedLaneRow"
        :board-id="boardId"
        :lane="selectedLaneRow"
        :reading="reading"
        :selected-date="selectedDate"
        :axis="visibleAxis"
        :today="today"
        :anchor-date="anchorDate"
        :resolve-day="resolveLaneDay"
        @select-date="date => emit('selectDate', date)"
        @open-report="payload => emit('openReport', payload)"
        @view-archive="date => emit('viewArchive', date)"
        @open-article="id => emit('openArticle', id)"
        @reading-settled="payload => emit('readingSettled', payload)"
      />
      <p v-else class="stage-hint" data-testid="console-reading-empty">{{ readingHint }}</p>
    </div>
  </template>
</template>

<style scoped>
.stage-pool {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.6rem;
  padding: 0.45rem 0.65rem;
  border: 1px solid var(--color-warning-subtle);
  border-radius: 6px;
  background: var(--color-warning-subtle);
  color: var(--color-warning);
  font-size: 0.72rem;
  line-height: 1.5;
}

.stage-pool button {
  flex: 0 0 auto;
  opacity: 1;
  border: 1px solid currentColor;
  border-radius: 5px;
  background: none;
  color: inherit;
  font-size: 0.7rem;
  padding: 0.15rem 0.45rem;
  cursor: pointer;
}

.stage-pool button:disabled {
  opacity: 0.55;
  cursor: wait;
}

.stage-chronicle {
  min-width: 0;
  scroll-margin-top: 0.8rem;
}

.stage-hint {
  margin: 0;
  padding: 0.8rem 0.9rem;
  border: 1px dashed var(--color-border-medium);
  border-radius: 10px;
  color: var(--color-text-muted);
  font-size: 0.78rem;
  line-height: 1.7;
}
</style>
