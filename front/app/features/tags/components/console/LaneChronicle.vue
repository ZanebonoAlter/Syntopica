<script setup lang="ts">
/**
 * LaneChronicle — 当日编年工作面（rework-workshop-into-radar-and-desk 矩阵阶段 G4）。
 *
 * 承接 SectionDrawer 退役路径的「当日线索」义务（阶段合同 §3.1）：
 *  - 数据 = A 名单内所选泳道 selectedDate 的当日 sections（G1 resolveLaneDay content）
 *    经稳定 ID 链（period_date → reportId → section_id → thread.id）join 日报详情的全量
 *    threads（>5 条可读）；标题默认收束、单条就地互斥展开真实 summary。
 *  - 相关文章按需（LaneRelatedArticles）；文章主点击 emit open-article 给 G5 站内阅读。
 *  - 上一/下一有内容日就地切换（跳过无分区日，仅 emit 日期；不 router/scroll/modal）。
 *  - 保留旧真实动作：「打开所选日日报」（完整日期，latest 主入口在壳层，不重复）与
 *    「看本板块此日归档」（头部子组件 LaneChronicleHeader）。
 *  - 只读：无生命周期/移出/补录写动作（CH-10）；读取层状态见 useLaneDayReading。
 *
 * 受控契约：lane/selectedDate/axis/resolveDay 全部 props 注入；不拥有选中态。
 */
import { computed, ref, watch } from 'vue'
import { useLaneDayReading, type LaneDayReadingStore } from '~/features/tags/composables/useLaneDayReading'
import type { LaneDayResolution } from '~/features/tags/composables/useObservationWindow'
import type { LaneDayResolver, ObservationLaneRow } from './observationLanes'
import LaneChronicleBanners from './LaneChronicleBanners.vue'
import LaneChronicleHeader from './LaneChronicleHeader.vue'
import LaneChronicleThread from './LaneChronicleThread.vue'
import { useReadingSettledSignal } from './useReadingSettledSignal'
import { useLaneChronicleMetrics } from './laneChronicleMetrics'

const props = withDefaults(defineProps<{
  boardId: number
  lane: ObservationLaneRow | null
  /** 受控选中日（父持有；与矩阵/演进共用一份）。 */
  selectedDate: string | null
  /** 可见观察轴（升序；上一/下一有内容日在此范围内走）。 */
  axis: string[]
  today: string
  /** 板块数据锚点（报告索引 lookback 覆盖计算；null 时回退目标日）。 */
  anchorDate: string | null
  resolveDay: LaneDayResolver
  /**
   * 壳层共享读取 store（8C G4b）：产品路径必须注入同一实例（报日索引 1 份、详情/文章同一
   * 缓存）。省略时保持旧自建用法（旧调用/单组件测试），不创建第二实例后丢掉。
   */
  reading?: LaneDayReadingStore
}>(), {
  lane: null,
  selectedDate: null,
  anchorDate: null,
})

const emit = defineEmits<{
  /** 上一/下一有内容日导航（仅 emit 日期，不强滚不开浮层）。 */
  selectDate: [date: string]
  /** 打开所选日日报（G5 走既有阅读台路由，日期完整携带）。 */
  openReport: [payload: { date: string, reportId: string | null }]
  /** 看本板块此日归档（G5 切归档视图并保存返回上下文）。 */
  viewArchive: [date: string]
  /** 文章主点击（G5 复用站内原文阅读，不加预览导语中间层）。 */
  openArticle: [articleId: string]
  /** 当前阅读上下文就绪/终态（R34 返回深滚动据此开始布局恢复）。 */
  readingSettled: [payload: { laneKey: string, date: string, status: string }]
}>()

const reading = props.reading ?? createOwnReading()

/** 未注入时的旧自建用法（组件自持 store；产品路径不走这里）。 */
function createOwnReading(): LaneDayReadingStore {
  return useLaneDayReading({
    boardId: computed(() => props.boardId),
    today: () => props.today,
    anchorDate: () => props.anchorDate,
  })
}

const loading = computed(() => reading.loading.value)
const staleNotice = computed(() => reading.staleNotice.value)
const errorText = computed(() => reading.error.value)

const resolution = computed<LaneDayResolution | null>(() => {
  if (!props.lane || !props.selectedDate) return null
  return props.resolveDay(props.lane.topicId, props.selectedDate)
})

const currentDay = computed(() => (resolution.value?.kind === 'content' ? resolution.value.day : null))
const { metricsSummary, metricsDetail } = useLaneChronicleMetrics(currentDay)

/** 当日 sections 指纹（上下文 gate 用；与 reading.current.sectionKey 同口径）。 */
const daySectionKey = computed(() => currentDay.value?.sections.map(section => section.sectionId).join(',') ?? null)

/**
 * 显示 gate（R24）：reading.current 只有在 board/lane/date/sections 与当前 props 上下文
 * 完全一致时才进入页头/横幅/正文/动作；离开可读上下文（gap/unavailable/未选日/换无分区
 * 泳道）或在途旧响应晚到都不能重新启用旧报告或渲染旧日横幅。
 */
const display = computed(() => {
  const model = reading.current.value
  const sectionKey = daySectionKey.value
  if (!model || !props.lane || !props.selectedDate || sectionKey === null) return null
  if (model.laneKey !== props.lane.topicId || model.date !== props.selectedDate) return null
  if (model.sectionKey !== sectionKey) return null
  return model
})

/** 可读日期序列（观察轴内有分区的日期，升序；gap/unavailable 都跳过）。 */
const contentDates = computed<string[]>(() => {
  if (!props.lane) return []
  return props.axis.filter(date => props.resolveDay(props.lane!.topicId, date).kind === 'content')
})

const prevDate = computed(() => {
  const selected = props.selectedDate
  if (!selected) return null
  const earlier = contentDates.value.filter(date => date < selected)
  return earlier.length > 0 ? earlier[earlier.length - 1]! : null
})

const nextDate = computed(() => {
  const selected = props.selectedDate
  if (!selected) return null
  const later = contentDates.value.filter(date => date > selected)
  return later.length > 0 ? later[0]! : null
})

/** 展开中的 thread（单条互斥；切日/泳道收束）。 */
const expandedThreadId = ref<string | null>(null)

watch(
  () => [props.lane?.topicId ?? null, props.selectedDate] as const,
  () => {
    expandedThreadId.value = null
  },
)

// 受控读取：lane/date/sections/解析结果变化（含池晚到 unavailable→content）即读；
// 离开可读上下文（next=null）先失效在途显示（R24，不动成功缓存），不把旧日内容挂在新日期标题下。
watch(
  () => {
    const day = currentDay.value
    const topicId = props.lane?.topicId ?? null
    return day && topicId
      ? { boardId: props.boardId, topicId, date: day.date, sections: daySectionKey.value, source: resolution.value?.kind }
      : null
  },
  (next) => {
    if (!next) {
      reading.suspendDisplay()
      return
    }
    const day = currentDay.value
    if (!day) return
    void reading.ensureDayReading({ laneKey: next.topicId, date: next.date }, day)
  },
  { immediate: true },
)

const state = computed(() => display.value?.status ?? 'idle')

useReadingSettledSignal({
  lane: () => props.lane,
  selectedDate: () => props.selectedDate,
  resolution: () => resolution.value,
  display: () => display.value,
  loading: () => loading.value,
}, payload => emit('readingSettled', payload))

/** 页脚口径：N 桶 · M 条标题（含降级 base 标题）· 另有 K 条折叠未读取。 */
const footerCounts = computed(() => {
  const model = display.value
  if (!model) return null
  let titles = 0
  for (const section of model.sections) {
    titles += section.matched ? section.threads.length : section.fallbackTitles.length
  }
  return { buckets: model.sections.length, titles, folded: model.foldedTotal }
})

const canOpenReport = computed(() => display.value?.reportId != null)
const openReportTitle = computed(() => {
  if (canOpenReport.value) return `打开 ${props.selectedDate} 日报`
  if (!props.lane || !props.selectedDate) return '尚未选择日期'
  const kind = resolution.value?.kind
  if (kind === 'gap') return '这一天这条泳道没有分区'
  if (kind === 'unavailable') return '该日历史数据未取到'
  if (state.value === 'no-report') return '未匹配到当日日报'
  if (state.value === 'error') return '日报线索加载失败，稍后重试'
  return '线索加载中…'
})


function retry(): void {
  const topicId = props.lane?.topicId
  const day = currentDay.value
  if (!topicId || !day || !props.selectedDate) return
  void reading.ensureDayReading({ laneKey: topicId, date: props.selectedDate }, day)
}

/** 强制重读（索引+详情绕过成功缓存；「报告已更新/分区不匹配」刷新路径）。 */
function forceRefresh(): void {
  const topicId = props.lane?.topicId
  const day = currentDay.value
  if (!topicId || !day || !props.selectedDate) return
  void reading.ensureDayReading({ laneKey: topicId, date: props.selectedDate }, day, { force: true })
}

function onOpenReport(): void {
  // 动作同样检查上下文（R24）：只有显示中的模型才允许发出，旧 reportId 不得配新日期
  const model = display.value
  if (!model || model.date !== props.selectedDate) return
  emit('openReport', { date: model.date, reportId: model.reportId })
}
</script>

<template>
  <section v-if="lane" class="lch" data-testid="lane-chronicle">
    <LaneChronicleHeader
      :lane-label="lane.label"
      :selected-date="selectedDate"
      :today="today"
      :prev-date="prevDate"
      :next-date="nextDate"
      :metrics-summary="metricsSummary"
      :metrics-detail="metricsDetail"
      :can-open-report="canOpenReport"
      :open-report-title="openReportTitle"
      @select-date="date => emit('selectDate', date)"
      @open-report="onOpenReport"
      @view-archive="selectedDate && emit('viewArchive', selectedDate)"
    />

    <!-- 状态横幅（分层：stale/error/no-report/degraded；子组件内同受 display gate） -->
    <LaneChronicleBanners
      :display="display"
      :stale-notice="staleNotice"
      :error-text="errorText"
      @retry="retry"
      @refresh="forceRefresh"
    />

    <!-- 日内容三态 -->
    <div v-if="!selectedDate" class="lch-hint">尚未选择日期。</div>
    <div v-else-if="resolution?.kind === 'gap'" class="lch-hint" data-testid="lane-chronicle-gap">这一天这条泳道没有分区。</div>
    <div v-else-if="resolution?.kind === 'unavailable'" class="lch-hint" data-testid="lane-chronicle-unavailable">该日历史数据未取到（365 日池未加载或加载失败）。</div>
    <div v-else-if="loading && !display" class="lch-skeleton" data-testid="lane-chronicle-loading">
      <div class="lch-skeleton-line" />
      <div class="lch-skeleton-line is-short" />
      <div class="lch-skeleton-line" />
    </div>

    <!-- 当日桶与线索（join 结果；降级桶显示 base 标题 + 折叠提示） -->
    <div v-else-if="display" class="lch-body">
      <article
        v-for="section in display.sections" :key="section.sectionId"
        class="lch-section" :data-testid="`lane-chronicle-section-${section.sectionId}`"
      >
        <h4 class="lch-section-label" :title="section.label">
          {{ section.bucketTitle || section.label }}
          <span v-if="!section.matched" class="lch-section-flag" title="该桶完整线索未读取（报告可能已更新）">未读取全文</span>
        </h4>
        <template v-if="section.matched">
          <LaneChronicleThread
            v-for="thread in section.threads" :key="thread.threadId"
            :thread="thread"
            :expanded="expandedThreadId === thread.threadId"
            :reading="reading"
            @toggle="id => (expandedThreadId = expandedThreadId === id ? null : id)"
            @open-article="id => emit('openArticle', id)"
          />
          <p v-if="section.threads.length === 0" class="lch-hint">当日 0 条线索（合法结果）。</p>
        </template>
        <template v-else>
          <ul class="lch-fallback">
            <li v-for="(title, index) in section.fallbackTitles" :key="index" class="lch-fallback-item">
              <span class="lch-fallback-mark" aria-hidden="true">·</span>{{ title }}
            </li>
          </ul>
          <p v-if="section.fallbackTitles.length === 0" class="lch-hint">这一桶聚合也没有返回标题。</p>
          <p v-if="section.foldedCount > 0" class="lch-folded">另有 {{ section.foldedCount }} 条折叠未读取</p>
        </template>
      </article>
      <footer v-if="footerCounts" class="lch-footer" data-testid="lane-chronicle-footer">
        已返回 {{ footerCounts.buckets }} 桶 · {{ footerCounts.titles }} 条标题<template v-if="footerCounts.folded > 0"> · 另有 {{ footerCounts.folded }} 条折叠</template>
      </footer>
    </div>
  </section>
</template>

<style scoped>
.lch {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  padding: 0.9rem 1rem 1rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 12px;
  background: color-mix(in srgb, var(--color-bg-elevated) 70%, transparent);
}

.lch-hint {
  font-size: 0.78rem;
  color: var(--color-text-muted);
  padding: 0.2rem 0;
}

.lch-skeleton {
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
}

.lch-skeleton-line {
  height: 0.85rem;
  border-radius: 6px;
  background: var(--color-bg-hover);
}

.lch-skeleton-line.is-short {
  width: 55%;
}

.lch-body {
  display: flex;
  flex-direction: column;
  gap: 0.8rem;
  border-top: 1px solid var(--color-border-subtle);
  padding-top: 0.7rem;
}

.lch-section {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.lch-section-label {
  margin: 0;
  font-size: 0.88rem;
  font-weight: 600;
  color: var(--color-text-primary);
  overflow-wrap: break-word;
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
}

.lch-section-flag {
  font-size: 0.62rem;
  font-weight: 400;
  color: var(--color-text-muted);
  border: 1px dashed var(--color-border-subtle);
  border-radius: 3px;
  padding: 0 0.2rem;
  white-space: nowrap;
}

.lch-fallback {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.lch-fallback-item {
  display: flex;
  gap: 0.45rem;
  font-size: 0.8rem;
  line-height: 1.55;
  color: var(--color-text-secondary);
  overflow-wrap: break-word;
}

.lch-fallback-mark {
  color: var(--color-text-muted);
  flex: 0 0 auto;
}

.lch-folded {
  margin: 0.1rem 0 0;
  font-size: 0.7rem;
  color: var(--color-text-muted);
}

.lch-footer {
  border-top: 1px dashed var(--color-border-subtle);
  padding-top: 0.45rem;
  font-size: 0.68rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}
</style>
