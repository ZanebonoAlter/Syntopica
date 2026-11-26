<script setup lang="ts">
/**
 * 当日其他话题单一阅读槽（rework-workshop-into-radar-and-desk 矩阵阶段 8C G3）。
 *
 * 纯受控：日期/分区选择/报告读取全部由 G4 壳层与既有单 store 持有；本组件不自建网络、
 * 不新建第二日报读取器、不改日期/范围/URL、不弹层/不强滚/不抢焦点。
 *
 *  - 正文：所选分区 `cluster_label` 逐字 + 该 section 全量 threads（复用 `LaneChronicleThread`，
 *    单条互斥展开真实 `summary` 逐字，空摘要 = 「暂无摘要记录」）；相关文章按需复用注入
 *    `reading` store 的 ≤20 缓存与单项重试（`LaneRelatedArticles`），**不复制**摘要/相关文章实现。
 *  - display gate：`reportDay.date` 必须等于 `selectedDate`；跨日报日/加载中不把旧日 sections
 *    挂在新日期下。详情已更新缺该分区 → 中性「报告已更新」+ 受控刷新意图，不借别区/别日。
 *  - 受控重试（M1）：失败/缺分区的「重试/刷新」只 emit `retryReport({date,sectionId})`，由 G4
 *    统一编排 `ensureReportIndex({force:true}) → ensureReportDay(date,{force:true})`（同日重建会
 *    换 reportId，先 force 索引才不会反复读旧缓存）。本组件**不**直接用旧 reportId 读详情。
 *  - 动作：打开所选日日报 / 看此日归档（完整日期 emit，latest 主入口仍在壳层头部，不重复）。
 *  - 同归属观察仍复用注入的板块级缓存；本区只保留读取/失败重试/缺 ID 提示。
 *    关联日导航已移至现有日期表头黄点（BoardConsoleStage），不再重复日期药丸。
 *  - `readingSettled`（M2）：等正确 date/section/topic 上下文 + 报告终态 + 观察依赖**真实**
 *    非刷新终态 + DOM 更新/已挂载后才发（见 `useOtherReadingSettled`）；换上下文/卸载后旧排队
 *    不发，同 context 刷新终态可再发一次；不带尚未展开的相关文章、无固定 sleep。
 */
import { computed, ref, watch } from 'vue'
import type { LaneDayReportSection } from '~/api/normalizers/laneDayReading'
import type { LaneDayReadingStore, LaneReportDay } from '~/features/tags/composables/useLaneDayReading'
import type { ObservedDayNode, OtherTopicObservationStore } from '~/features/tags/composables/useOtherTopicObservation'
import LaneChronicleThread from './LaneChronicleThread.vue'
import OtherTopicObservationBand from './OtherTopicObservationBand.vue'
import OtherTopicReadingStates from './OtherTopicReadingStates.vue'
import { useOtherReadingSettledSignal } from './useOtherReadingSettled'

const props = withDefaults(defineProps<{
  /** 壳层唯一选中日。 */
  selectedDate: string | null
  /** 当前其他分区选择（`selectedOther.sectionId`；当日局部身份）。 */
  selectedSectionId: string | null
  /** 既有单 store 发布的报告日模型（渲染前按 date 做 display gate）。 */
  reportDay: LaneReportDay | null
  /** 注入的既有读取 store（相关文章缓存/失败重试；产品路径必须注入）。 */
  reading: LaneDayReadingStore
  /** 注入的板块级只读观察依赖（G4 单实例；本组件只读/触发懒读取）。 */
  observation: OtherTopicObservationStore
  /** 选中其他行的稳定话题身份（G2 row.persistentTopicId；null = 当次未归属）。 */
  topicId?: string | null
}>(), {
  topicId: null,
})

const emit = defineEmits<{
  /** 打开所选日报（完整日期；latest 主入口不在此重复）。 */
  openReport: [payload: { date: string, reportId: string | null }]
  /** 看本板块此日归档。 */
  viewArchive: [date: string]
  /** 文章主点击（G4 复用既有站内阅读）。 */
  openArticle: [articleId: string]
  /**
   * 受控报告重试/刷新意图（G4 必须按序：`ensureReportIndex({force:true})` →
   * `ensureReportDay(date,{force:true})`；payload 只带当前稳定上下文，不带旧 reportId）。
   */
  retryReport: [payload: { date: string, sectionId: string }]
  /** 阅读上下文终态（阅读 + 观察都就绪/终态；供既有 R34 返回恢复）。 */
  readingSettled: [payload: {
    /** 稳定其他分区 context key：`other:<sectionId>`（与泳道 topicId 命名空间区分）。 */
    laneKey: string
    date: string
    status: string
    sectionId: string
    topicId: string | null
  }]
}>()

type ReadingState = 'idle' | 'pending' | 'ready' | 'no-report' | 'error' | 'missing-section'

/** display gate：只有报告日期与所选日一致时才允许渲染 sections。 */
const gatedDay = computed<LaneReportDay | null>(() => {
  const day = props.reportDay
  const date = props.selectedDate
  if (day === null || date === null || props.selectedSectionId === null) return null
  return day.date === date ? day : null
})

/** 当前选中分区（稳定 sectionId 精确命中；找不到 = 报告已更新，不借标题补）。 */
const section = computed<LaneDayReportSection | null>(() => {
  const day = gatedDay.value
  if (day === null || day.status !== 'ready') return null
  return day.sections.find(item => item.sectionId === props.selectedSectionId) ?? null
})

const threads = computed(() => section.value?.threads ?? [])

const state = computed<ReadingState>(() => {
  if (props.selectedDate === null || props.selectedSectionId === null) return 'idle'
  const day = gatedDay.value
  if (day === null) return 'pending'
  if (day.status === 'loading') return 'pending'
  if (day.status === 'no-report') return 'no-report'
  if (day.status === 'error') return 'error'
  return section.value !== null ? 'ready' : 'missing-section'
})

const readingTitle = computed(() => {
  switch (state.value) {
    case 'pending': return '正在读取当日报告…'
    case 'no-report': return '所选日没有日报'
    case 'error': return '当日报告读取失败'
    case 'missing-section': return '当日报告已更新，未找到该分区'
    default: return '先选一条其他话题'
  }
})

/** 展开中的线索（单条互斥；切日/切分区收束）。 */
const expandedThreadId = ref<string | null>(null)
watch(
  () => [props.selectedDate, props.selectedSectionId] as const,
  () => { expandedThreadId.value = null },
)

function toggleThread(threadId: string): void {
  expandedThreadId.value = expandedThreadId.value === threadId ? null : threadId
}

// ── 同归属跨日观察（只读依赖，懒读取） ─────────────────────────────────────

/** 观察目标：优先 G2 行快照 ID，其次当前 section 的历史归属 ID；日期未 gate 时不观察。 */
const observationTopicId = computed<string | null>(() => {
  if (gatedDay.value === null || props.selectedSectionId === null) return null
  if (props.topicId !== null && props.topicId !== '') return props.topicId
  return section.value?.persistentTopicId ?? null
})

// 懒读取：选中其他行且有可靠 ID 才取；已有板块级成功数据时换话题只本地投影（不重复 GET）。
watch(observationTopicId, (id) => {
  void props.observation.ensureObservation(id)
}, { immediate: true })

const observeView = computed(() => props.observation.view.value)
const observeNodes = computed<ObservedDayNode[]>(() => (
  observeView.value.status === 'ready' ? observeView.value.nodes : []
))

function retryObservation(): void {
  void props.observation.retry()
}

// ── 既有阅读动作（只读 GET；不本地改日期/URL） ─────────────────────────────

/** 失败重试/缺分区刷新统一走受控意图：父级负责索引 force + 当日详情 force 的顺序。 */
function requestReportRetry(): void {
  const date = props.selectedDate
  const sectionId = props.selectedSectionId
  if (date === null || sectionId === null) return
  emit('retryReport', { date, sectionId })
}

function openReport(): void {
  const day = gatedDay.value
  const date = props.selectedDate
  if (day === null || date === null) return
  emit('openReport', { date, reportId: day.reportId })
}

function viewArchive(): void {
  if (props.selectedDate !== null) emit('viewArchive', props.selectedDate)
}

/** 正文状态块（加载/无日报/失败/缺分区；ready 与 idle 不进块）。 */
const showStateBlock = computed(() => state.value === 'pending'
  || state.value === 'no-report'
  || state.value === 'error'
  || state.value === 'missing-section')

// ── 阅读终态信号（报告终态 + 观察真实终态 + DOM 已挂载/更新；不等相关文章） ────

const settledReportStatus = computed<string | null>(() => {
  if (gatedDay.value === null) return null
  switch (state.value) {
    case 'ready':
    case 'missing-section':
    case 'no-report':
    case 'error':
      return state.value
    default:
      return null
  }
})

useOtherReadingSettledSignal({
  selectedDate: () => props.selectedDate,
  selectedSectionId: () => props.selectedSectionId,
  reportStatus: () => settledReportStatus.value,
  observationStatus: () => observeView.value.status,
  observationRefreshing: () => observeView.value.refreshing,
  topicId: () => observationTopicId.value,
}, payload => emit('readingSettled', payload))
</script>

<template>
  <section v-if="selectedDate && selectedSectionId" class="otr" data-testid="other-topic-reading">
    <header class="otr-head">
      <div class="otr-head-main">
        <p class="otr-kicker">
          当日其他话题 ·
          <time :datetime="selectedDate">{{ selectedDate }}</time>
        </p>
        <h3 v-if="section" class="otr-title" :title="section.clusterLabel">{{ section.clusterLabel }}</h3>
        <h3 v-else class="otr-title otr-title--pending" data-testid="other-reading-state-title">{{ readingTitle }}</h3>
      </div>
      <div v-if="state === 'ready'" class="otr-actions">
        <button type="button" data-testid="other-reading-open-report" @click="openReport">
          打开 {{ selectedDate }} 日报 ↗
        </button>
        <button type="button" data-testid="other-reading-archive" @click="viewArchive">看此日归档</button>
      </div>
    </header>

    <!-- 同归属跨日观察带（独立只读依赖：失败/缺身份不阻塞正文） -->
    <OtherTopicObservationBand
      v-if="gatedDay"
      :status="observeView.status"
      :stale="observeView.stale"
      :error="observeView.error"
      :nodes="observeNodes"
      @retry="retryObservation"
    />

    <!-- 阅读正文状态 -->
    <OtherTopicReadingStates
      v-if="showStateBlock"
      :state="state"
      :title="readingTitle"
      :error-text="gatedDay?.error ?? null"
      @retry-report="requestReportRetry"
    />
    <template v-else-if="state === 'ready' && section">
      <ol v-if="threads.length > 0" class="otr-threads" data-testid="other-reading-threads">
        <li v-for="thread in threads" :key="thread.threadId">
          <LaneChronicleThread
            :thread="thread"
            :expanded="expandedThreadId === thread.threadId"
            :reading="reading"
            @toggle="toggleThread"
            @open-article="id => emit('openArticle', id)"
          />
        </li>
      </ol>
      <p v-else class="otr-empty" data-testid="other-reading-empty">该分区当日 0 条线索（合法结果）。</p>
    </template>
    <p v-else class="otr-empty">先在「当日其他话题」里选一条分区。</p>
  </section>
</template>

<style scoped>
.otr {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  padding: 0.9rem 1rem 1rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 12px;
  background: color-mix(in srgb, var(--color-bg-elevated) 70%, transparent);
  min-width: 0;
}

.otr-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.8rem;
  flex-wrap: wrap;
  padding-bottom: 0.55rem;
  border-bottom: 1px solid var(--color-border-subtle);
}

.otr-head-main {
  display: flex;
  flex-direction: column;
  gap: 0.18rem;
  min-width: 0;
  flex: 1 1 16rem;
}

.otr-kicker {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 0.62rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.otr-title {
  margin: 0;
  color: var(--color-text-primary);
  font-size: 0.88rem;
  font-weight: 600;
  line-height: 1.4;
  overflow-wrap: break-word;
}

.otr-title--pending {
  color: var(--color-text-secondary);
  font-size: 0.9rem;
}

.otr-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.otr-actions button {
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  background: none;
  color: var(--color-accent);
  font-size: 0.7rem;
  padding: 0.2rem 0.5rem;
  cursor: pointer;
}

.otr-actions button:hover {
  border-color: var(--color-accent);
}

.otr-threads {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.otr-empty {
  margin: 0;
  font-size: 0.78rem;
  line-height: 1.65;
  color: var(--color-text-muted);
  overflow-wrap: break-word;
}
</style>
