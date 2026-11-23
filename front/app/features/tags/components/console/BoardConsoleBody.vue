<script setup lang="ts">
/**
 * 控制台三区骨架（rework-workshop-into-radar-and-desk 矩阵阶段 G5）。
 *
 * 只做展示编排与事件转发：顶栏 / 版块栏 / 主舞台 / 上下文右轨；
 * 数据与唯一状态全部由壳层构造后注入（`stage` / `rail` 为纯视图模型）。
 */
import { ref } from 'vue'
import { Icon } from '@iconify/vue'
import type { ConsoleView } from '~/features/tags/composables/useBoardConsole'
import type { BoardConsoleStageProps } from './consoleStageTypes'
import type { ObservationRange, SourceTierDays } from './observationRange'
import type { RailBinding } from './railContext'
import type { RailBoardRow } from './railClueAxis'
import BoardConsoleHeader from './BoardConsoleHeader.vue'
import BoardRail from './BoardRail.vue'
import BoardConsoleStage from './BoardConsoleStage.vue'
import BoardContextRail from './BoardContextRail.vue'

defineProps<{
  boardMissing: boolean
  /** 左轨行视图模型（共同轴重对齐后的逐日线索条码）。 */
  railRows: RailBoardRow[]
  /** 左轨共同轴日期（14 天，oldest→newest；行 title 的逐日明细用）。 */
  railAxisDates: string[]
  selectedBoardId: number
  boardsLoading: boolean
  boardsError: string | null
  /** overview 本次没取到（版块信号未取到；版块列表本身可能正常）。 */
  signalError: string | null
  boardLabel: string
  latestReportDate: string | null
  latestReportTitle: string
  reportMetaUnavailable: boolean
  reportMetaStale: boolean
  refreshing: boolean
  activeView: ConsoleView
  isNarrow: boolean
  stage: BoardConsoleStageProps
  rail: RailBinding
}>()

const emit = defineEmits<{
  setView: [view: ConsoleView]
  openLatestReport: []
  refresh: []
  openResearch: []
  manage: [kind: 'settings' | 'composition' | 'topics']
  newLane: []
  openContext: []
  selectBoard: [boardId: number]
  retryBoards: []
  selectLane: [topicId: string]
  /** 「编辑泳道」→ 泳道修正工作面（lane-manual-correction-loop）。 */
  editLane: [topicId: string]
  /** 「归档泳道」→ 归档弹窗（sm 档 + 二次确认）。 */
  archiveLane: [topicId: string]
  selectCell: [payload: { topicId: string, date: string }]
  selectDate: [date: string]
  toggleDormant: []
  /** 报日列头选日（与日格同一 selectedDate）。 */
  selectDayHeader: [date: string]
  /** 当日其他话题组展开/收起。 */
  toggleOthers: []
  /** 其他组选行/选格。 */
  selectOther: [payload: { sectionId: string, date: string, topicId?: string | null }]
  /** 右轨「当日其他话题」选行（同一落点，但右轨发起要先切回矩阵 / 追踪视图，design D14）。 */
  selectOtherFromRail: [payload: { sectionId: string, date: string, topicId?: string | null }]
  /** 观察带节点（原子切同一 selectedDate + 稳定目标）。 */
  selectObservedDay: [payload: { date: string, sectionId: string, reportId: string, topicId: string }]
  retryStage: []
  /** 阅读槽受控重试（force 索引 → force 当日详情）。 */
  retryReport: [payload: { date: string, sectionId: string }]
  /** 报日索引/当日报告链重试。 */
  retryReportChain: []
  generate: []
  openReportDate: [payload: { date: string, reportId: string | null }]
  viewArchive: [date: string]
  openArticle: [articleId: string]
  previewRange: [range: ObservationRange]
  commitRange: [range: ObservationRange]
  sourceChange: [days: SourceTierDays]
  retryPool: []
  readingSettled: [payload: { laneKey: string, date: string, status: string }]
  retryHealth: []
  openHealth: []
  openCandidates: []
  openSources: []
  openWatchManage: []
  retryWatches: []
  locateLane: [topicId: string]
  openTopics: []
  /** 右轨当日点评 → 跳到该期日报正文的对应聚簇。 */
  openReportCluster: [payload: { date: string, clusterId: number | null }]
  /** 右轨当日日报详情失败 → 受控重试（只重取当日详情）。 */
  retryNow: [date: string]
}>()

const stageRef = ref<HTMLElement | null>(null)

function getStageElement(): HTMLElement | null {
  return stageRef.value
}

defineExpose({ getStageElement })
</script>

<template>
  <div v-if="boardMissing" class="console-missing" data-testid="console-board-missing">
    <Icon icon="mdi:alert-circle-outline" width="28" />
    <p>版块不存在或已归档。</p>
    <NuxtLink to="/tags">返回今日雷达</NuxtLink>
  </div>
  <div v-else class="console" data-testid="board-console">
    <BoardConsoleHeader
    :board-label="boardLabel"
    :active-view="activeView"
    :latest-report-date="latestReportDate"
    :latest-report-title="latestReportTitle"
    :report-meta-unavailable="reportMetaUnavailable"
    :report-meta-stale="reportMetaStale"
    :refreshing="refreshing"
    :is-narrow="isNarrow"
    @set-view="view => emit('setView', view)"
    @open-report="emit('openLatestReport')"
    @refresh="emit('refresh')"
    @open-research="emit('openResearch')"
    @manage="kind => emit('manage', kind)"
    @new-lane="emit('newLane')"
    @open-context="emit('openContext')"
  />

  <div class="console-main">
    <BoardRail
      class="console-boards"
      :rows="railRows"
      :axis-dates="railAxisDates"
      :selected-board-id="selectedBoardId"
      :loading="boardsLoading"
      :error="boardsError"
      :signal-error="signalError"
      @select="id => emit('selectBoard', id)"
      @retry="emit('retryBoards')"
    />

    <main ref="stageRef" class="console-stage">
      <BoardConsoleStage
        v-bind="stage"
        @select-lane="id => emit('selectLane', id)"
        @edit-lane="id => emit('editLane', id)"
        @archive-lane="id => emit('archiveLane', id)"
        @select-cell="payload => emit('selectCell', payload)"
        @select-date="date => emit('selectDate', date)"
        @select-day-header="date => emit('selectDayHeader', date)"
        @select-other="payload => emit('selectOther', payload)"
        @select-observed-day="payload => emit('selectObservedDay', payload)"
        @toggle-dormant="emit('toggleDormant')"
        @toggle-others="emit('toggleOthers')"
        @retry="emit('retryStage')"
        @retry-report="payload => emit('retryReport', payload)"
        @retry-report-chain="emit('retryReportChain')"
        @generate="emit('generate')"
        @open-context="emit('openContext')"
        @open-report="payload => emit('openReportDate', payload)"
        @view-archive="date => emit('viewArchive', date)"
        @open-article="id => emit('openArticle', id)"
        @preview-range="range => emit('previewRange', range)"
        @commit-range="range => emit('commitRange', range)"
        @source-change="days => emit('sourceChange', days)"
        @retry-pool="emit('retryPool')"
        @reading-settled="payload => emit('readingSettled', payload)"
      />
    </main>

    <aside class="console-gov" aria-label="本板块上下文">
      <BoardContextRail
        v-bind="rail"
        @retry-health="emit('retryHealth')"
        @open-health="emit('openHealth')"
        @open-candidates="emit('openCandidates')"
        @open-sources="emit('openSources')"
        @open-watch-manage="emit('openWatchManage')"
        @retry-watches="emit('retryWatches')"
        @locate-lane="id => emit('locateLane', id)"
        @open-topics="emit('openTopics')"
        @open-report="emit('openReportCluster', $event)"
        @retry-now="emit('retryNow', $event)"
        @select-other="emit('selectOtherFromRail', $event)"
      />
    </aside>
  </div>
  </div>
</template>

<style scoped>
.console-missing {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.6rem;
  height: 100vh;
  height: 100dvh;
  color: var(--color-text-muted);
  font-size: 0.9rem;
}

.console-missing a {
  color: var(--color-accent);
  font-size: 0.8rem;
}

.console {
  height: 100vh;
  height: 100dvh;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--color-bg-base);
  color: var(--color-text-primary);
}

.console-main {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: 17rem minmax(0, 1fr) 21rem;
}

.console-boards {
  min-width: 0;
}

/* 1440 下右轨压缩到 18rem；宽屏保持 21rem（workspace 全宽，无 120rem 上限） */
@media (max-width: 1535.98px) and (min-width: 1440px) {
  .console-main {
    grid-template-columns: 17rem minmax(0, 1fr) 18rem;
  }
}

.console-stage {
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
  padding: 0.9rem 1.1rem 1.6rem;
}

.console-gov {
  min-width: 0;
  min-height: 0;
  /* 右轨内部自己滚（满高自适应：内容超高时整栏滚动，不悬在栏底）。 */
  overflow: hidden;
  border-left: 1px solid var(--color-border-subtle);
  padding: 0.9rem 1rem 1.6rem;
  display: flex;
  flex-direction: column;
  gap: 0.7rem;
}

/* 1440 以下：回落 workspace（页面滚动，不锁视口）；右轨压缩 */
@media (max-width: 1439.98px) {
  .console {
    height: auto;
    min-height: 100dvh;
    overflow: visible;
  }

  .console-main {
    grid-template-columns: 15rem minmax(0, 1fr) 18rem;
    min-height: 70vh;
  }

  .console-stage,
  .console-gov {
    overflow-y: visible;
  }
}

/* 768–1199：右轨收进单级宽工作面，避免两根常驻轨把舞台夹窄 */
@media (max-width: 1199.98px) and (min-width: 768px) {
  .console-main {
    grid-template-columns: 15rem minmax(0, 1fr);
  }

  .console-gov {
    display: none;
  }
}

/* 窄屏(<768)：单栏线性化——版块栏与右轨不常驻，泳道摘要 + 下方编年 + 上下文入口 */
@media (max-width: 767.98px) {
  .console-main {
    grid-template-columns: minmax(0, 1fr);
    min-height: 0;
  }

  .console-boards,
  .console-gov {
    display: none;
  }

  .console-stage {
    padding: 0.7rem 0.8rem 4rem;
    overflow-y: auto;
  }
}
</style>
