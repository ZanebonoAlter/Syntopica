<script setup lang="ts">
/**
 * 雷达右栏底部管线状态块（add-radar-pipeline-status D9/D10 + D14）。
 *
 * 五项事实（标签队列 / Embedding 队列 / 生成窗口 / 最新日报落点 / 当日日报状态）
 * + 手动生成入口：点「立即生成」在**状态块内就地展开**板块勾选清单（默认全选、
 * 可全选/全不选、可取消勾选），不弹窗、不跳页（D10）。勾选 = 参与本次生成；
 * 未勾选任何板块时禁止提交。
 *
 * 两个队列量纲不同：**标签队列单位是文章**，**Embedding 队列单位是标签**
 * （embedding_queues 行按 tag_id 排）——各行各带自己的计数与进度条。
 *
 * 状态语义（R-17）：
 * - 对账从未成功 → 各行都呈现「读取中」，**不产出**「今日日报未生成」这类结论；
 * - 对账失败 → 保留旧值并标注「更新于 HH:mm」，不清零、不转错误态；
 * - 总览取数失败 → 日报行呈现「读取失败」，**不判定**「今日日报未生成」；
 * - **双队列**任一有积压（pending + processing > 0）→ 生成按钮禁用 + 缺料说明
 *   （日报自动生成要求 tag_jobs 与 embedding_queues 双队列清空，D14）；
 * - embedding 队列计数未知（旧后端无该段）→ 同样挡住生成，不断言「已清空」；
 * - 双队列均清空 → 生成按钮可用、缺料说明消失；
 * - 清单展开期间队列复堵 → 确认按钮同样禁用（与入口按钮同款判据，
 *   不能绕过「队列未清空禁用生成」门槛）；
 * - 今日已出 → 入口按钮文案改为「重新生成今日日报」，清单内给出替换警示，
 *   确认按钮文案为「确认替换」（同日替换是破坏性动作，design Risks）。
 *
 * 状态块默认**折叠成一行摘要**（matrix-clue-fallback-and-rail-merge D5）：
 * 正常态只占一行「状态词 · 日报 x/y · 队列 n」；折叠摘要行本身呈现异常词与
 * 计数（折叠不藏异常信号）。点摘要行展开五项事实 + 手动生成入口，再点收起。
 * **异常态自动展开**（含挂载时即异常）：生成请求 failed / 任一队列有失败任务 /
 * 已过生成时刻而当日日报未生成 / 队列活跃量 ≥ 明显积压阈值。异常消除回折叠。
 * 展开态的五项事实与生成闸门契约零改动。
 *
 * 视觉：只用 Layer 2 语义 token（--color-*），底色用 color-mix 派生；
 * 数字用 --font-mono，标题用 --font-serif（与右栏既有排版一致）。
 */
import { computed, ref, watch } from 'vue'
import AppButton from '~/components/ui/AppButton.vue'
import { useConfirm } from '~/composables/useConfirm'
import { useRadarPipelineStatus, type RadarStatusBoard } from '~/features/tags/composables/useRadarPipelineStatus'

const props = defineProps<{
  /** 雷达总览载荷的板块行（落点日期与勾选清单的唯一来源；不另发请求）。 */
  rows: RadarStatusBoard[]
  /** 总览取数中。 */
  overviewLoading?: boolean
  /** 总览取数失败（非空即失败）。 */
  overviewError?: string
}>()

const emit = defineEmits<{
  /** 生成终态到达：父层据此刷新雷达总览数据。 */
  (e: 'generated'): void
}>()

const status = useRadarPipelineStatus({
  rows: () => props.rows,
  loading: () => props.overviewLoading ?? false,
  error: () => props.overviewError ?? '',
})

const {
  pollReady,
  updatedAtText,
  lastReconciledAt,
  activeCount,
  completedToday,
  failedCount,
  roundTotal,
  progressPercent,
  queueBlocked,
  embeddingActiveCount,
  embeddingCompletedToday,
  embeddingFailedCount,
  embeddingRoundTotal,
  embeddingProgressPercent,
  embeddingReady,
  embeddingBlocked,
  queuesDrained,
  scheduleTime,
  scheduleDeadline,
  latestReportDate,
  todayReportCount,
  todayReady,
  boardTotal,
  boards,
  overviewLoading,
  overviewError,
  phase,
  progressText,
  result,
  failureMessage,
  generate,
  // 缺口（近 7 天）
  gapsByDate,
  missingDays,
  totalMissing,
  gapLoading,
  gapError,
  gapLoaded,
  reloadGaps,
  // 补填
  backfillPhase,
  backfillProgress,
  backfillResult,
  backfillError,
  backfillDate,
  backfillAll,
  backfillBlocked,
} = status

const { confirm } = useConfirm()

// ── 板块勾选清单（就地展开，不弹窗）──────────────────────────────────────
const pickerOpen = ref(false)
const selected = ref<number[]>([])

const selectedCount = computed(() => selected.value.length)
const generateDisabled = computed(() => !pollReady.value || !queuesDrained.value || phase.value === 'submitting')
/**
 * 确认按钮的禁用判据与入口按钮**同款**（双队列门槛 + 对账就绪）：清单展开
 * 期间可能有新文章/新标签入队，此时「确认生成」必须跟着入口按钮一起变灰，
 * 否则能绕过「队列未清空禁用生成」的门槛。
 */
const confirmDisabled = computed(() => phase.value === 'submitting'
  || selectedCount.value === 0
  || !queuesDrained.value
  || !pollReady.value)

/** 队列行文案：读取中优先（首次对账未成功不产出结论）；失败任务不算积压（D8）。 */
const queueText = computed(() => {
  if (!pollReady.value) return '读取中'
  if (activeCount.value > 0) return `待打标 ${activeCount.value}`
  if (completedToday.value > 0) return '队列已清空'
  return '队列空闲'
})

/**
 * embedding 队列行文案（单位：**标签**，不是文章）。
 * 计数未知（旧后端无该段）→ 「状态未知」，不谎称已清空（R-17）。
 */
const embeddingQueueText = computed(() => {
  if (!pollReady.value) return '读取中'
  if (!embeddingReady.value) return '状态未知'
  if (embeddingActiveCount.value > 0) return `待向量化 ${embeddingActiveCount.value}`
  if (embeddingCompletedToday.value > 0) return '队列已清空'
  return '队列空闲'
})

/** 对账从未成功时不展示窗口时刻（默认时刻不是本站真实配置，不当事实呈现）。 */
const windowReady = computed(() => pollReady.value)

/** 最新日报落点行。 */
const latestText = computed(() => {
  if (!pollReady.value) return '读取中'
  if (overviewLoading.value) return '读取中'
  if (overviewError.value) return '读取失败'
  if (!latestReportDate.value) return '暂无'
  return latestReportDate.value
})

/** 当日日报状态行：已出 / 未生成 / 读取中 / 读取失败。 */
const todayText = computed(() => {
  if (!pollReady.value) return '读取中'
  if (overviewLoading.value) return '读取中'
  if (overviewError.value) return '读取失败'
  if (todayReportCount.value > 0) return `今日日报已出（${todayReportCount.value}/${boardTotal.value} 板块）`
  return '今日日报未生成'
})

/** 按钮不可用的原因说明（缺料说明）：双队列任一有积压 / 对账尚未成功。 */
const shortageText = computed(() => {
  if (!pollReady.value) return '对账读取中：队列状态未知，暂不可生成。'
  const parts: string[] = []
  if (queueBlocked.value) {
    parts.push(`标签队列仍有 ${activeCount.value} 篇待打标：手动生成不等队列，当日才完成打标的文章不会进入当日日报。`)
  }
  if (embeddingBlocked.value) {
    parts.push(embeddingReady.value
      ? `Embedding 队列有 ${embeddingActiveCount.value} 个标签待向量化：泳道分桶会退化，文章可能落进「未归位」桶。`
      : 'Embedding 队列状态未知：无法确认是否已清空，暂不可生成。')
  }
  return parts.join(' ')
})

/**
 * 当前本地时刻是否还在生成时刻之前。
 *
 * 真实时钟（new Date()）不是响应式依赖，页面停驻跨过生成时刻后文案不会
 * 自己变。把「最近一次对账时刻」当刻度读进来：每次对账成功必变（间隔
 * ≥30s），下一次对账即可让文案重算——**不为此引入定时器**（client-poll-budget）。
 */
const isBeforeSchedule = computed(() => {
  const tick = lastReconciledAt.value
  // tick 只作为依赖被读到（tick ≤ now，取 max 不改变比较结果）。
  const now = new Date(Math.max(Date.now(), tick))
  const nowMinutes = now.getHours() * 60 + now.getMinutes()
  const parts = scheduleTime.value.split(':').map(Number)
  const hour = parts[0]
  const minute = parts[1]
  if (hour === undefined || minute === undefined) return false
  if (!Number.isFinite(hour) || !Number.isFinite(minute)) return false
  return nowMinutes < hour * 60 + minute
})

/**
 * 「今日未生成」的缺料原因（ui-design 受影响状态 success 行）：
 * 未到生成时刻 / 已过生成时刻但今日无素材或生成未完成。
 */
const notGeneratedHint = computed(() => {
  if (!pollReady.value) return ''
  if (overviewLoading.value || overviewError.value) return ''
  if (todayReportCount.value > 0) return ''
  return isBeforeSchedule.value
    ? `尚未到生成时刻（${scheduleTime.value}）`
    : `已过生成时刻（${scheduleTime.value}）：今日暂无可用素材或生成未完成`
})

/**
 * 补填入口为何禁用（缺料说明）。
 * Embedding 计数不可得时不能说「队列还没清空」（它可能是已清空、只是未知）——
 * 两种原因分开说，否则用户看到灰按钮却读不到真实原因。
 */
const backfillBlockedHint = computed(() => {
  if (!embeddingReady.value) return '队列状态未知：无法确认是否已清空，清空后才能补。'
  return '队列还没清空：补出来的候选会不全，清空后才能补。'
})

const showQueueProgress = computed(() => pollReady.value && roundTotal.value > 0)
const showEmbeddingProgress = computed(() => pollReady.value && embeddingReady.value && embeddingRoundTotal.value > 0)

// ── 折叠状态机（D5）─────────────────────────────────────────────────────
/** 明显积压阈值（UI attention 常量，不是业务闸门）：生成闸门仍是「任一活跃即禁用」，
 *  与此阈值无关——展开与禁用是两把尺子。 */
const RADAR_QUEUE_ATTENTION = 100

/** 异常态（任一为真）→ 强制展开。未到生成时刻的未出日报不算异常。 */
// 「过点未出日报」这一支在取数窗口内保持上一次已知判定：加载中不是「异常消除」
// 而是「未知」——避免挂载/重试/生成后的 load() 引起「收起→展开」抖动（review R-3）。
const overdueMissing = ref(false)
watch(
  [pollReady, overviewLoading, overviewError, todayReady, isBeforeSchedule],
  ([ready, loading, error, today, beforeSchedule]) => {
    if (!ready || loading || error) return
    overdueMissing.value = !today && !beforeSchedule
  },
  { immediate: true },
)

const abnormal = computed(() => {
  if (phase.value === 'failed') return true
  if (failedCount.value > 0 || embeddingFailedCount.value > 0) return true
  if (overdueMissing.value) return true
  return activeCount.value >= RADAR_QUEUE_ATTENTION || embeddingActiveCount.value >= RADAR_QUEUE_ATTENTION
})

// 自动态与用户手动态解耦（review R-3）：manualCollapsed = null 时跟随 autoCollapsed；
// 用户手动展开/收起后**粘滞**（正常态不自动收回），直到用户自己再切一次。
const autoCollapsed = ref(true)
const manualCollapsed = ref<boolean | null>(null)
const collapsed = computed(() => manualCollapsed.value ?? autoCollapsed.value)
// 异常即展开（含挂载时已异常）；异常消除回折叠。
// 只有**真实变迁**（异常态实际变了）才动折叠态：取数窗口内「过点未出日报」这一支
// 由 overdueMissing 粘滞，abnormal 不会在窗口内翻转，故这里不会收起再展开抖动。
// 真实变迁时手动意图作废、交还控制权给自动态——否则「用户手动收起 + 真实异常出现」
// 不会自动展开（spec「异常态 SHALL 自动展开」），「异常消除后 SHALL 回到折叠摘要」
// 也永远轮不到。
watch(abnormal, (isAbnormal) => {
  const nextCollapsed = !isAbnormal
  if (nextCollapsed === autoCollapsed.value) return
  autoCollapsed.value = nextCollapsed
  manualCollapsed.value = null
}, { immediate: true })

function toggleCollapsed(): void {
  manualCollapsed.value = !collapsed.value
}

/**
 * 失败任务条数（折叠摘要也要看得见计数——折叠 SHALL NOT 藏异常信号，design R4 / review R-4）。
 *
 * 两条队列**量纲不同**（标签队列计文章、Embedding 队列计标签），故按分段写法
 * `2+1` 呈现，MUST NOT 相加成一个数（与摘要队列段同款约定，review R-6）。
 */
const failedCountText = computed(() => {
  const parts: string[] = []
  if (failedCount.value > 0) parts.push(String(failedCount.value))
  if (embeddingFailedCount.value > 0) parts.push(String(embeddingFailedCount.value))
  return parts.join('+')
})

/** 失败条数的量纲提示（两条队列单位不同：文章 vs 标签；不相加）。 */
const summaryFailedTitle = '失败条数 = 标签队列失败文章数 + Embedding 队列失败标签数（两者量纲不同，不相加）'

/** 折叠摘要的状态词：读取中优先（R-17：不把未知当业务结论），其次异常词，最后正常。
 *  失败态带条数：折叠不得藏异常计数（design R4 / review R-4）。 */
const summaryState = computed(() => {
  if (!pollReady.value) return '读取中'
  if (failedCount.value > 0 || embeddingFailedCount.value > 0) {
    return `队列有失败 ${failedCountText.value}`
  }
  if (phase.value === 'failed') return '生成失败'
  if (overviewLoading.value) return '读取中'
  if (overviewError.value) return '读取失败'
  if (!todayReady.value && !isBeforeSchedule.value) return '日报未生成'
  if (activeCount.value >= RADAR_QUEUE_ATTENTION || embeddingActiveCount.value >= RADAR_QUEUE_ATTENTION) return '队列积压'
  return '管线正常'
})

/** 摘要日报段：今日已出 x/y；未出分清「未到点」与「已过点」；未知不判未生成。 */
const summaryReport = computed(() => {
  if (!pollReady.value) return '日报 —'
  if (overviewLoading.value) return '日报 读取中'
  if (overviewError.value) return '日报 读取失败'
  if (todayReportCount.value > 0) return `日报 ${todayReportCount.value}/${boardTotal.value}`
  return '日报 0/' + String(boardTotal.value)
})

/** 摘要队列段：两条队列量纲不同（标签队列计**文章**、Embedding 队列计**标签**），
 *  按 design D5 分段呈现 `队列 40+3`，MUST NOT 相加成一个数（matrix-clue R-3）。
 *  embedding 未就绪时不把未知当 0，标签队列自身计数照常给出。 */
const summaryQueue = computed(() => {
  if (!pollReady.value) return '队列 —'
  if (!embeddingReady.value) return `队列 ${activeCount.value} + 标签 —`
  const embedding = embeddingActiveCount.value > 0 ? `+${embeddingActiveCount.value}` : ''
  return `队列 ${activeCount.value}${embedding}`
})

/** 摘要队列段的量纲提示（两条队列单位不同：文章 vs 标签）。 */
const summaryQueueTitle = '标签队列待打标文章数 + Embedding 队列待向量化标签数（两者量纲不同，不相加）'

/** 摘要行是否处于异常态（用于着色）。 */
const summaryAbnormal = computed(() => abnormal.value && pollReady.value)

function openPicker(): void {
  selected.value = boards.value.map(board => board.boardId)
  pickerOpen.value = true
}

function closePicker(): void {
  pickerOpen.value = false
}

function selectAll(): void {
  selected.value = boards.value.map(board => board.boardId)
}

function selectNone(): void {
  selected.value = []
}

function toggleBoard(boardId: number): void {
  selected.value = selected.value.includes(boardId)
    ? selected.value.filter(id => id !== boardId)
    : [...selected.value, boardId]
}

function confirmGenerate(): void {
  if (confirmDisabled.value) return
  void submitGenerate()
}

/**
 * 发起生成（今日已出时先走一次**危险样式**的二次确认）。
 *
 * 今日已出（todayReady）= 同日替换 = 破坏性动作：既有日报的章节与线索会被
 * 重写，页边注与人工锚定可能错位（design D9）。先弹全局确认弹窗，取消则
 * **不发任何请求**；首次生成（非 todayReady）不弹。
 */
async function submitGenerate(): Promise<void> {
  if (todayReady.value) {
    const ok = await confirm({
      title: '重新生成今日日报？',
      message: '既有日报的章节与线索会被重写，页边注与人工锚定可能错位。',
      confirmText: '重新生成',
      cancelText: '取消',
      danger: true,
    })
    if (!ok) return
  }
  await generate(selected.value)
}

// ── 缺口折叠区（近 7 天）────────────────────────────────────────────────
// 就地展开的折叠区（与板块勾选清单同款）：不弹窗、不跳页。默认折叠，
// 行头带「N 天 / M 篇」；无缺口时行头呈「近 7 天无缺口」且不出按钮。
const gapOpen = ref(false)

/** 缺口日期的短格式（MM-DD）。 */
function formatGapDate(date: string): string {
  return date.length >= 10 ? date.slice(5, 10) : date
}

/**
 * 补填进行中（受理中 / 运行中）：两个补填入口同时禁用，不许重复触发。
 */
const backfillBusy = computed(() => backfillPhase.value === 'submitting' || backfillPhase.value === 'running')

/**
 * 缺口行头文案。
 *
 * 读取中 / 读取失败优先（未知 ≠ 无缺口）；补填进行中转为进度（补齐中 n/m）。
 */
const gapHeadText = computed(() => {
  if (backfillBusy.value) {
    const { filled, total } = backfillProgress.value
    return total > 0 ? `近 7 天缺口 补齐中 ${filled}/${total}` : '近 7 天缺口 补齐中…'
  }
  if (gapError.value) return '近 7 天缺口 读取失败'
  if (gapLoading.value || !gapLoaded.value) return '近 7 天缺口 读取中'
  if (totalMissing.value === 0) return '近 7 天无缺口'
  return `近 7 天缺口 ${missingDays.value} 天 / ${totalMissing.value} 篇`
})

/** 补填进度行（带当前报告日）。 */
const backfillProgressText = computed(() => {
  const { filled, total, date } = backfillProgress.value
  const dateText = date ? `${formatGapDate(date)} · ` : ''
  return `补齐中 ${dateText}${filled}/${total}`
})

/** 补填终态文案（四态：完成 / 部分完成 / 额度用尽 / 失败）。 */
const backfillResultText = computed(() => {
  const r = backfillResult.value
  if (!r) return ''
  const head = r.status === 'success'
    ? '补齐完成'
    : r.status === 'partial'
      ? '部分完成'
      : r.status === 'budget_exhausted'
        ? '额度用尽，已留档待续'
        : '补齐失败'
  return `${head} · 补出 ${r.filled} 篇 · 失败 ${r.failed} 篇 · 剩余 ${r.remaining} 篇`
})

function toggleGap(): void {
  gapOpen.value = !gapOpen.value
}

function onBackfillDate(date: string): void {
  void backfillDate(date)
}

function onBackfillAll(): void {
  void backfillAll()
}

watch(phase, (next, prev) => {
  if (next === 'done' && prev !== 'done') emit('generated')
})

// 补填终态：缺口清单变了 → 父层据此刷新雷达总览（缺口取数由 composable 自己重取）
watch(backfillPhase, (next, prev) => {
  if (next === 'done' && prev !== 'done') emit('generated')
})
</script>

<template>
  <section class="rs-block" aria-label="管线状态" data-testid="radar-status-block">
    <div class="rs-rule-title">
      <h2>管线状态</h2>
      <span class="rs-rule-tag">PIPELINE</span>
    </div>

    <!-- 折叠摘要行（常驻头部）：正常态就只有这一行；展开后它仍在顶端作为状态句 +
         折叠入口，异常词与计数始终可见（折叠不藏异常信号）。 -->
    <button
      type="button"
      class="rs-summary"
      :class="{ 'is-abnormal': summaryAbnormal }"
      data-testid="radar-status-summary"
      :aria-expanded="!collapsed"
      @click="toggleCollapsed"
    >
      <span
        class="rs-summary-state"
        :title="failedCount > 0 || embeddingFailedCount > 0 ? summaryFailedTitle : undefined"
      >{{ summaryState }}</span>
      <span class="rs-summary-sep">·</span>
      <span data-testid="radar-status-summary-report">{{ summaryReport }}</span>
      <span class="rs-summary-sep">·</span>
      <span data-testid="radar-status-summary-queue" :title="summaryQueueTitle">{{ summaryQueue }}</span>
      <span class="rs-summary-chevron" aria-hidden="true">{{ collapsed ? '▾' : '▴' }}</span>
    </button>

    <template v-if="!collapsed">
    <div class="rs-facts">
      <div class="rs-fact">
        <span class="rs-key">标签队列：</span>
        <span class="rs-val" data-testid="radar-status-queue">{{ queueText }}</span>
      </div>
      <div class="rs-fact">
        <span class="rs-key">Embedding 队列：</span>
        <span class="rs-val" data-testid="radar-status-embedding-queue">{{ embeddingQueueText }}</span>
      </div>
      <div class="rs-fact">
        <span class="rs-key">生成窗口：</span>
        <span class="rs-val">
          <template v-if="windowReady">
            <span data-testid="radar-status-schedule-time">生成 {{ scheduleTime }}</span>
            <span class="rs-sep">·</span>
            <span data-testid="radar-status-schedule-deadline">兜底 {{ scheduleDeadline }}</span>
          </template>
          <span v-else data-testid="radar-status-window-loading">读取中</span>
        </span>
      </div>
      <div class="rs-fact">
        <span class="rs-key">最新日报：</span>
        <span class="rs-val" data-testid="radar-status-latest">{{ latestText }}</span>
      </div>
      <div class="rs-fact">
        <span class="rs-key">今日状态：</span>
        <span class="rs-val" data-testid="radar-status-today">{{ todayText }}</span>
      </div>
    </div>

    <div v-if="showQueueProgress" class="rs-progress" data-testid="radar-status-queue-progress">
      <div class="rs-bar" role="img" :aria-label="`今日完成 ${completedToday} / ${roundTotal}`">
        <span class="rs-bar-fill" :style="{ width: `${progressPercent}%` }" />
      </div>
      <span class="rs-progress-text">今日完成 {{ completedToday }} / {{ roundTotal }}</span>
    </div>

    <div v-if="showEmbeddingProgress" class="rs-progress" data-testid="radar-status-embedding-progress">
      <div class="rs-bar" role="img" :aria-label="`今日完成 ${embeddingCompletedToday} / ${embeddingRoundTotal} 个标签`">
        <span class="rs-bar-fill" :style="{ width: `${embeddingProgressPercent}%` }" />
      </div>
      <span class="rs-progress-text">今日完成 {{ embeddingCompletedToday }} / {{ embeddingRoundTotal }} 个标签</span>
    </div>

    <p v-if="failedCount > 0" class="rs-failed" data-testid="radar-status-queue-failed">
      <span class="rs-dot" aria-hidden="true" />{{ failedCount }} 个失败
    </p>

    <p v-if="embeddingFailedCount > 0" class="rs-failed" data-testid="radar-status-embedding-failed">
      <span class="rs-dot" aria-hidden="true" />Embedding 队列 {{ embeddingFailedCount }} 个失败
    </p>

    <p v-if="pollReady" class="rs-updated" data-testid="radar-status-updated">更新于 {{ updatedAtText }}</p>

    <p v-if="shortageText" class="rs-shortage" data-testid="radar-status-shortage">{{ shortageText }}</p>

    <p v-if="notGeneratedHint" class="rs-hint" data-testid="radar-status-not-generated-hint">{{ notGeneratedHint }}</p>

    <p v-if="phase === 'submitting'" class="rs-running" data-testid="radar-status-progress">
      生成中 {{ progressText || '…' }}
    </p>
    <p v-if="phase === 'done' && result" class="rs-result" data-testid="radar-status-result">
      成功 {{ result.successBoards }} 板块 · 失败 {{ result.failedBoards }} 板块
    </p>
    <p v-if="phase === 'failed' && failureMessage" class="rs-failed" data-testid="radar-status-error">
      <span class="rs-dot" aria-hidden="true" />{{ failureMessage }}
    </p>

    <div class="rs-actions">
      <AppButton
        v-if="!pickerOpen"
        size="sm"
        data-testid="radar-status-generate"
        :disabled="generateDisabled"
        @click="openPicker"
      >
        {{ todayReady ? '重新生成今日日报' : '立即生成今日日报' }}
      </AppButton>
      <button
        v-if="!pickerOpen"
        class="rs-collapse"
        type="button"
        data-testid="radar-status-collapse"
        @click="toggleCollapsed"
      >
        收起
      </button>
    </div>

    <div v-if="pickerOpen" class="rs-picker" data-testid="radar-status-picker">
      <div class="rs-picker-head">
        <span class="rs-picker-title">生成范围</span>
        <span class="rs-picker-count" data-testid="radar-status-selected-count">
          参与 {{ selectedCount }} / {{ boardTotal }}
        </span>
      </div>

      <div class="rs-picker-toggles">
        <button type="button" class="rs-toggle" data-testid="radar-status-select-all" @click="selectAll">全选</button>
        <button type="button" class="rs-toggle" data-testid="radar-status-select-none" @click="selectNone">全不选</button>
      </div>

      <ul v-if="boards.length" class="rs-picker-list">
        <li v-for="board in boards" :key="board.boardId" class="rs-picker-item">
          <label class="rs-board">
            <input
              type="checkbox"
              :checked="selected.includes(board.boardId)"
              :data-testid="`radar-status-board-${board.boardId}`"
              @change="toggleBoard(board.boardId)"
            >
            <span class="rs-board-label">{{ board.label }}</span>
            <span v-if="board.todayClues === 0" class="rs-board-empty">今日无线索</span>
          </label>
        </li>
      </ul>
      <p v-else class="rs-picker-empty" data-testid="radar-status-picker-empty">暂无可参与的板块</p>

      <p v-if="todayReady" class="rs-replace-warning" data-testid="radar-status-replace-warning">
        将替换今天的日报（标题、章节、线索全部重写）
      </p>

      <div class="rs-picker-actions">
        <AppButton
          size="sm"
          data-testid="radar-status-confirm"
          :disabled="confirmDisabled"
          :loading="phase === 'submitting'"
          @click="confirmGenerate"
        >
          {{ todayReady ? '确认替换' : '确认生成' }}
        </AppButton>
        <button type="button" class="rs-toggle" data-testid="radar-status-cancel" @click="closePicker">取消</button>
      </div>
    </div>

    <!-- 近 7 天缺口（add-daily-report-gap-backfill）：就地展开的折叠区（默认
         折叠）——不弹窗、不跳页。取数失败只降级本区，不影响其余五项事实；
         取数未完成时呈「读取中」，未知不得渲染成「无缺口」。 -->
    <div class="rs-gap" data-testid="radar-gap">
      <button
        type="button"
        class="rs-gap-head"
        data-testid="radar-gap-head"
        :aria-expanded="gapOpen"
        @click="toggleGap"
      >
        <span class="rs-gap-title" data-testid="radar-gap-summary">{{ gapHeadText }}</span>
        <span class="rs-gap-chevron" aria-hidden="true">{{ gapOpen ? '▴' : '▾' }}</span>
      </button>

      <div v-if="gapError" class="rs-gap-error" data-testid="radar-gap-error">
        <span>读取失败</span>
        <button type="button" class="rs-toggle" data-testid="radar-gap-retry" @click="reloadGaps">重试</button>
      </div>

      <p v-if="backfillBusy" class="rs-running" data-testid="radar-gap-progress">{{ backfillProgressText }}</p>
      <p v-if="backfillPhase === 'done' && backfillResult" class="rs-result" data-testid="radar-gap-result">{{ backfillResultText }}</p>
      <p v-if="backfillPhase === 'failed' && backfillError" class="rs-failed" data-testid="radar-gap-submit-error">
        <span class="rs-dot" aria-hidden="true" />{{ backfillError }}
      </p>

      <div v-if="gapOpen" class="rs-gap-body" data-testid="radar-gap-body">
        <ul v-if="gapsByDate.length" class="rs-gap-list" data-testid="radar-gap-list">
          <li
            v-for="day in gapsByDate"
            :key="day.date"
            class="rs-gap-row"
            :data-testid="`radar-gap-day-${day.date}`"
          >
            <span class="rs-gap-date">{{ formatGapDate(day.date) }}</span>
            <span class="rs-gap-count">{{ day.boards.length }} 个板块缺</span>
            <AppButton
              size="sm"
              variant="ghost"
              class="rs-gap-btn rs-gap-act"
              :data-testid="`radar-gap-backfill-${day.date}`"
              :disabled="backfillBusy || backfillBlocked"
              @click="onBackfillDate(day.date)"
            >
              补这天
            </AppButton>
          </li>
        </ul>
        <p v-else class="rs-gap-empty" data-testid="radar-gap-empty">窗口内没有缺报板块</p>

        <div v-if="gapsByDate.length" class="rs-gap-actions">
          <AppButton
            size="sm"
            variant="secondary"
            class="rs-gap-act"
            data-testid="radar-gap-backfill-all"
            :disabled="backfillBusy || backfillBlocked"
            @click="onBackfillAll"
          >
            一次补齐 {{ totalMissing }} 篇
          </AppButton>
        </div>

        <p v-if="backfillBlocked" class="rs-hint rs-gap-blocked" data-testid="radar-gap-queue-blocked">
          {{ backfillBlockedHint }}
        </p>
      </div>
    </div>
    </template>
  </section>
</template>

<style scoped>
.rs-block {
  flex: 0 0 auto;
  padding: 0.85rem 0.9rem 0.9rem;
  border: 1px solid color-mix(in srgb, var(--color-accent) 20%, transparent);
  border-top: 2px solid var(--color-accent);
  border-radius: 6px;
  background: color-mix(in srgb, var(--color-accent) 6%, transparent);
  color: var(--color-text-primary);
}

.rs-rule-title {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.75rem;
  margin-bottom: 0.5rem;
}

.rs-block h2 {
  margin: 0;
  color: var(--color-accent);
  font-family: var(--font-serif, Georgia, serif);
  font-size: 0.95rem;
  font-weight: 650;
  letter-spacing: 0.02em;
}

.rs-rule-tag {
  color: var(--color-text-muted);
  font-size: 0.58rem;
  letter-spacing: 0.16em;
}

.rs-facts {
  display: flex;
  flex-direction: column;
  gap: 0.22rem;
  min-width: 0;
}

/* 折叠摘要行（D5 默认态）：一行占位，整行可点开/收起。异常时状态词用 accent。 */
.rs-summary {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 0.3rem;
  width: 100%;
  padding: 0.45rem 0;
  border: 0;
  border-bottom: 1px solid var(--color-border-subtle);
  background: none;
  color: var(--color-text-muted);
  font: inherit;
  font-size: 0.66rem;
  text-align: left;
  cursor: pointer;
}

.rs-summary:hover {
  color: var(--color-text-secondary);
}

.rs-summary:focus-visible {
  outline: 1px solid var(--color-accent);
  outline-offset: 2px;
}

.rs-summary-state {
  color: var(--color-text-secondary);
  font-weight: 600;
}

.rs-summary.is-abnormal .rs-summary-state {
  color: var(--color-accent);
}

.rs-summary-sep {
  color: var(--color-border-medium);
}

.rs-summary-chevron {
  margin-left: auto;
  color: var(--color-text-muted);
}

.rs-fact {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
  min-width: 0;
  font-size: 0.72rem;
  line-height: 1.5;
}

.rs-key {
  flex: 0 0 auto;
  color: var(--color-text-muted);
  white-space: nowrap;
}

.rs-val {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 0.3rem;
  min-width: 0;
  color: var(--color-text-primary);
  font-family: var(--font-mono, monospace);
  overflow-wrap: anywhere;
}

.rs-sep {
  color: var(--color-text-muted);
}

/* 今日完成进度条 */
.rs-progress {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 0.45rem;
}

.rs-bar {
  flex: 1 1 auto;
  min-width: 0;
  height: 4px;
  border-radius: 2px;
  background: var(--color-bg-sunken);
  overflow: hidden;
}

.rs-bar-fill {
  display: block;
  height: 100%;
  background: var(--color-accent);
}

.rs-progress-text {
  margin: 0;
  color: var(--color-text-secondary);
  font-family: var(--font-mono, monospace);
  font-size: 0.66rem;
}

/* 状态标记：左侧醒目位（interaction-conventions 第 1 条） */
.rs-failed {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  margin: 0.4rem 0 0;
  color: var(--color-error);
  font-size: 0.68rem;
}

.rs-dot {
  flex: 0 0 auto;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--color-error);
}

.rs-updated {
  margin: 0.3rem 0 0;
  color: var(--color-text-muted);
  font-size: 0.62rem;
}

/* 缺料说明：状态说明行，不是动作（interaction-conventions 第 2 条） */
.rs-shortage {
  margin: 0.4rem 0 0;
  color: var(--color-warning);
  font-size: 0.66rem;
  line-height: 1.6;
}

.rs-hint {
  margin: 0.35rem 0 0;
  color: var(--color-text-muted);
  font-size: 0.64rem;
  line-height: 1.6;
}

.rs-result {
  margin: 0.4rem 0 0;
  color: var(--color-text-secondary);
  font-size: 0.68rem;
}

.rs-running {
  margin: 0.4rem 0 0;
  color: var(--color-text-secondary);
  font-family: var(--font-mono, monospace);
  font-size: 0.68rem;
}

.rs-actions {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  margin-top: 0.6rem;
}

/* 展开态的收起入口（折叠后由摘要行承接开合）。 */
.rs-collapse {
  margin-left: auto;
  border: 0;
  background: none;
  color: var(--color-text-muted);
  font: inherit;
  font-size: 0.64rem;
  text-decoration: underline;
  cursor: pointer;
}

.rs-collapse:hover {
  color: var(--color-accent);
}

/* 勾选清单：就地展开 */
.rs-picker {
  margin-top: 0.6rem;
  padding: 0.6rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  background: var(--color-bg-base);
}

.rs-picker-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.5rem;
}

.rs-picker-title {
  color: var(--color-text-primary);
  font-family: var(--font-serif, Georgia, serif);
  font-size: 0.8rem;
}

.rs-picker-count {
  color: var(--color-text-muted);
  font-family: var(--font-mono, monospace);
  font-size: 0.64rem;
}

.rs-picker-toggles {
  display: flex;
  gap: 0.6rem;
  margin-top: 0.35rem;
}

.rs-toggle {
  padding: 0;
  border: 0;
  background: none;
  color: var(--color-accent);
  font: inherit;
  font-size: 0.66rem;
  text-decoration: underline;
  cursor: pointer;
}

.rs-picker-list {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  margin: 0.45rem 0 0;
  padding: 0;
  list-style: none;
}

.rs-picker-item {
  min-width: 0;
}

.rs-board {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
  min-width: 0;
  font-size: 0.7rem;
}

.rs-board input {
  flex: 0 0 auto;
  accent-color: var(--color-accent);
}

.rs-board-label {
  min-width: 0;
  color: var(--color-text-secondary);
  overflow-wrap: anywhere;
}

.rs-board-empty {
  flex: 0 0 auto;
  color: var(--color-text-muted);
  font-size: 0.62rem;
}

.rs-picker-empty {
  margin: 0.45rem 0 0;
  color: var(--color-text-muted);
  font-size: 0.68rem;
}

/* 重生成警示：同日替换是破坏性动作，确认按钮附近必须写明后果 */
.rs-replace-warning {
  margin: 0.45rem 0 0;
  color: var(--color-warning);
  font-size: 0.66rem;
  line-height: 1.6;
}

.rs-picker-actions {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  margin-top: 0.6rem;
}

/* 窄屏：事实行改单列；清单单列由 flex-direction: column 保证 */
@media (max-width: 767.98px) {
  .rs-fact {
    flex-direction: column;
    align-items: flex-start;
    gap: 0.1rem;
  }
}

/* 近 7 天缺口：就地展开的折叠区（与勾选清单同族的容器） */
.rs-gap {
  margin-top: 0.6rem;
  padding-top: 0.5rem;
  border-top: 1px solid var(--color-border-subtle);
}

.rs-gap-head {
  display: flex;
  align-items: baseline;
  gap: 0.3rem;
  width: 100%;
  padding: 0.2rem 0;
  border: 0;
  background: none;
  color: var(--color-text-secondary);
  font: inherit;
  font-size: 0.7rem;
  text-align: left;
  cursor: pointer;
}

.rs-gap-head:hover {
  color: var(--color-accent);
}

.rs-gap-head:focus-visible {
  outline: 1px solid var(--color-accent);
  outline-offset: 2px;
}

.rs-gap-title {
  min-width: 0;
  font-family: var(--font-mono, monospace);
  overflow-wrap: anywhere;
}

.rs-gap-chevron {
  margin-left: auto;
  color: var(--color-text-muted);
}

.rs-gap-error {
  display: flex;
  align-items: baseline;
  gap: 0.35rem;
  margin: 0.3rem 0 0;
  color: var(--color-error);
  font-size: 0.68rem;
}

.rs-gap-body {
  margin-top: 0.3rem;
  /* 不画嵌套面：缺口区是卡片内的一组数据行，不是子卡片。
     自带 background 会在双主题下与卡片的 accent 半透明底打架——浅色主题
     一块暖白、深色主题一块纯黑，看着像外来模块。分隔靠行间的细线。 */
}

.rs-gap-list {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}

.rs-gap-row {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 0.4rem;
  min-width: 0;
  padding: 0.3rem 0;
  border-top: 1px dashed var(--color-border-subtle);
  font-size: 0.68rem;
  line-height: 1.6;
}

.rs-gap-list > .rs-gap-row:first-child {
  border-top: 0;
}

.rs-gap-date {
  flex: 0 0 auto;
  color: var(--color-text-primary);
  font-family: var(--font-mono, monospace);
}

.rs-gap-count {
  flex: 0 0 auto;
  color: var(--color-text-muted);
}

.rs-gap-btn {
  margin-left: auto;
}

/* 禁用的行级 ghost 按钮：全局 .4 在这行小字上几乎看不见（有效对比度 ≈1.8:1），
   上调到 .55 —— 仍明确是禁用态，但保留可读性。同一区域的「一次补齐」也走 .55，
   否则相邻两个禁用按钮一个亮一个喑，看着像只有一个是禁用的。 */
.rs-gap-act:disabled {
  opacity: 0.55;
}

/* 缺料说明是「按钮为什么灰了」的唯一出口：不能沿用 .rs-hint 的 10.24px + muted
   （≈3.0:1），提到 11.2px + secondary（≈5.99:1 / dark ≈7.16:1）。 */
.rs-gap-blocked {
  font-size: 0.7rem;
  color: var(--color-text-secondary);
}

.rs-gap-empty {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 0.68rem;
}

.rs-gap-actions {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  margin-top: 0.15rem;
  padding-top: 0.5rem;
  border-top: 1px solid var(--color-border-subtle);
}
</style>
