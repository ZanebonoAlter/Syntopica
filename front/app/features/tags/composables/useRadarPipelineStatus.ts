/**
 * 雷达页管线状态聚合（add-radar-pipeline-status D6/D7/D8/D11）。
 *
 * 把「标签队列积压 → 日报生成窗口 → 当日日报是否已出」这条因果链的数据
 * 收到一处，供右栏底部的状态块渲染：
 *
 * - **标签队列**：读 `usePollBundle` 已分发的 `tag-queue-progress:status`
 *   （pending / processing / completedToday / failed）。**不新建周期性请求**——
 *   本 composable 只 `ensureStarted()` 启动既有单例（幂等），不写 setInterval。
 * - **生成窗口**：读 `poll:schedulers` 里 `daily_report` 条目的 `schedule_time`
 *   与 `schedule_deadline`；旧后端缺 `schedule_deadline` 时降级到默认 23:30
 *   （不留空、不报错，见用例 V4/V13）。
 * - **日报落点 / 当日判定**：雷达总览载荷各板 `report_date` 的最大值；当日判定
 *   用 `report_date === todayKey()`（**本地**日期，不走 `out.Date`、不走 UTC）。
 * - **手动生成**：`generateDailyReport({date, board_ids})` + 既有 WS 事件
 *   `daily_report_progress` / `daily_report_done`（D11：不新增事件类型）。
 *   WS 事件是全局广播，按响应里的 **job_id** 归属本次生成：响应未到时先缓存，
 *   拿到 job_id 后只回放匹配的（并发管线的终态不被误吸收）。
 *   终态计数**直接读**消息自带的 `failed_count` / `empty_count`（三态：成功 /
 *   真失败 / 当日无内容）——**不**用 `total_boards - total_saved` 推算失败数，
 *   那种推算会把「当日无内容」的板块错算成失败（与通知中心口径不一致）。
 *   队列门槛（`pending + processing > 0` / 对账从未成功）在**入口**同样把关：
 *   绕过 UI 直接调用也不会放行（清单展开期间有文章入队时同样生效）。
 * - **近 7 天缺口与补填**（add-daily-report-gap-backfill D8）：缺口清单来自
 *   `GET /daily-reports/gaps`（**一次性请求**：挂载 / 补填终态 / 跨日切换各取
 *   一次，**不新建周期性轮询**，也不塞进雷达总览的轮询载荷）。缺口判定口径在
 *   服务端（与生成 Step1 同谓词），前端只消费 `missing_days` / `total_missing`
 *   与平铺的 `gaps`，**不自行推算**。取数失败只降级缺口区（未知 ≠ 无缺口），
 *   卡片其余五项事实不受影响。补填走 `POST /daily-reports/backfill-gaps`
 *   （异步受理即返回 `job_id`）；逐格进度复用 `daily_report_progress` 但**必须**
 *   按 `run_scope === 'backfill'` + `job_id` 过滤（否则被当日生成的进度串台），
 *   终态走**独立**事件 `daily_report_backfill_done`（`daily_report_done` 仍是
 *   当日生成专用）。
 *
 * R-17：首次对账从未成功时，本层只给「读取中」，**不产出**「今日日报未生成」
 * 这类业务结论；对账失败保留旧值由 `usePollBundle` 保证（不清零、不转错误态）。
 */
import { computed, getCurrentInstance, onMounted, onUnmounted, ref, toValue, watch, type MaybeRefOrGetter } from 'vue'
import { useDailyReportsApi, type DailyReportBackfillRunSummary, type DailyReportBackfillStatus, type DailyReportGap } from '~/api/dailyReports'
import { useEmbeddingQueueProgress } from '~/composables/useEmbeddingQueueProgress'
import { useEventStream } from '~/composables/useEventStream'
import { usePollBundle } from '~/composables/usePollBundle'
import { useTagQueueProgress } from '~/composables/useTagQueueProgress'
import { EVENT_TYPES } from '~/utils/eventTypes'
import { todayKey } from '~/utils/todayKey'

/**
 * 状态块消费的**板块行最小结构**——只包含四项事实与勾选清单实际用到的字段。
 *
 * 上游雷达总览行携带更多字段（`todayClues` / `reportTitle` / `clueDays` / `lanes` …），
 * 它在结构上可直接传进来；这里收窄声明的目的是让状态块只依赖自己真正需要的
 * 契约，而不是捆住整个雷达读模型。
 */
export interface RadarStatusBoard {
  /** 板块 ID（雷达读模型为字符串 ID；生成请求边界再转数字）。 */
  boardId: string | number
  label: string
  /** 该板块最新日报日期（`YYYY-MM-DD`；无日报为空串）。 */
  reportDate: string
  /** 该板块当日线索合计（0 = 今日无线索，勾选清单给提示；null = 旧后端未知）。 */
  todayClues: number | null
}

/** 兜底生成时刻（后端 `daily_report_time` 默认值；旧后端不返回时的降级）。 */
export const DEFAULT_SCHEDULE_TIME = '21:00'
/** 兜底强制生成时刻（后端 `daily_report_deadline` 默认值；旧后端不返回时的降级）。 */
export const DEFAULT_SCHEDULE_DEADLINE = '23:30'

/** 手动生成按钮的状态机（白盒表：idle / blocked / picker-open / submitting / done / failed）。 */
export type RadarGeneratePhase = 'idle' | 'submitting' | 'done' | 'failed'

/** 状态块展示用的板块条目（勾选清单 = 参与本次生成的板块集合）。 */
export interface RadarPipelineBoard {
  /** 数字板块 ID（API 边界要求数字 ID；读模型里的字符串 ID 在此归一）。 */
  boardId: number
  label: string
  /** 当日线索合计：0 表示该板今天还没有料（生成出来是 0 篇，不算失败）；null = 未知。 */
  todayClues: number | null
  reportDate: string
}

/** 生成终态结果（成功板块数 / 失败板块数 / 当日无内容板块数）。 */
export interface RadarGenerateResult {
  successBoards: number
  failedBoards: number
  /**
   * 当日无内容的板块数：既不是成功也不是失败（**不**计入失败数）。
   * 后端旧版本不带该字段时为 0（不可知 ≠ 有）。
   */
  emptyBoards: number
}

/** 缺口窗口天数（与后端缺口查询 / 「一次补齐」的默认窗口同口径）。 */
export const GAP_WINDOW_DAYS = 7

/** 补填状态机（白盒表：idle / submitting / running / done / failed）。 */
export type RadarBackfillPhase = 'idle' | 'submitting' | 'running' | 'done' | 'failed'

/** 缺口日里的一个缺报板块。 */
export interface RadarGapBoard {
  boardId: number
  /** 板块名（服务端可能给空串 → 回退 `Board #N`）。 */
  label: string
}

/** 某一天的缺口（该日缺报的板块集合）。 */
export interface RadarGapDay {
  /** 报告日（YYYY-MM-DD）。 */
  date: string
  boards: RadarGapBoard[]
}

/** 补填进度（已补数 / 总数 / 当前报告日）。 */
export interface RadarBackfillProgress {
  filled: number
  total: number
  /** 当前正在补的报告日（未收到进度消息时为空串）。 */
  date: string
}

/** 补填终态结果（服务端四态：success / partial / failed / budget_exhausted）。 */
export interface RadarBackfillResult {
  status: DailyReportBackfillStatus
  filled: number
  failed: number
  remaining: number
  windowStart: string
  windowEnd: string
}

export interface RadarPipelineStatusOptions {
  /** 雷达总览载荷的板块行（落点日期与勾选清单都来自这里；不另发请求）。 */
  rows?: MaybeRefOrGetter<RadarStatusBoard[]>
  /** 总览取数中（局部 loading：队列行照常、日报行「读取中」）。 */
  loading?: MaybeRefOrGetter<boolean>
  /** 总览取数失败（非空即失败；此时**不判定**「今日日报未生成」）。 */
  error?: MaybeRefOrGetter<string>
}

/** 本地 HH:mm（对账更新时刻标注）。 */
function formatClock(timestamp: number): string {
  if (!timestamp) return ''
  const d = new Date(timestamp)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

export function useRadarPipelineStatus(options: RadarPipelineStatusOptions = {}) {
  // 队列事实直接读 usePollBundle 分发的全局 state（键与既有消费点一致），
  // 口径（activeCount / roundTotal / progressPercent）复用 useTagQueueProgress。
  const queue = useTagQueueProgress()
  const poll = usePollBundle()
  // 常驻对账由既有单例承载（幂等）：只启动，不新建轮询、不新建端点请求。
  poll.ensureStarted()

  const rows = computed<RadarStatusBoard[]>(() => toValue(options.rows) ?? [])
  const overviewLoading = computed(() => toValue(options.loading) ?? false)
  const overviewError = computed(() => toValue(options.error) ?? '')

  // ── 标签队列 ──────────────────────────────────────────────────────────
  const pending = computed(() => queue.status.value.pending)
  const processing = computed(() => queue.status.value.processing)
  /** 活跃待打标量 = pending + processing（与 useTagQueueProgress.activeCount 同口径）。 */
  const activeCount = computed(() => queue.activeCount.value)
  const completedToday = computed(() => queue.status.value.completedToday)
  const failedCount = computed(() => queue.status.value.failed)
  /** 本轮总量 = 活跃量 + 今日已完成（进度分母）。 */
  const roundTotal = computed(() => queue.roundTotal.value)
  const progressPercent = computed(() => queue.progressPercent.value)
  /** 队列空闲：既无活跃量也无今日完成（成功空集下的真实结论）。 */
  const queueIdle = computed(() => activeCount.value === 0 && completedToday.value === 0)
  /** 门槛：队列有积压 → 手动生成禁用（failed 不计入，D8）。 */
  const queueBlocked = computed(() => activeCount.value > 0)

  // ── embedding 队列（第二环；单位是标签，不是文章）─────────────────────
  const embedding = useEmbeddingQueueProgress()
  /** 活跃待向量化量 = pending + processing（与标签队列同口径）。 */
  const embeddingActiveCount = computed(() => embedding.activeCount.value)
  const embeddingCompletedToday = computed(() => embedding.status.value.completedToday)
  const embeddingFailedCount = computed(() => embedding.status.value.failed)
  /** 本轮总量 = 活跃量 + 今日已完成（进度分母）。 */
  const embeddingRoundTotal = computed(() => embedding.roundTotal.value)
  const embeddingProgressPercent = computed(() => embedding.progressPercent.value)
  /**
   * embedding 这一环是否挡住生成：**段缺失（未知）或有积压**。
   * 未知也挡住——R-17：没有数据不产出「已清空」结论。
   */
  const embeddingBlocked = computed(() => !embedding.ready.value || embedding.activeCount.value > 0)
  /** embedding 队列计数已就绪（对账成功且响应带 embedding_queue 段）。 */
  const embeddingReady = computed(() => embedding.ready.value)
  /**
   * 双队列门槛（D14）：日报自动生成要求 tag_jobs 与 embedding_queues **双队列**
   * 清空（lane 分桶依赖 tag embedding 到质心的距离，embedding 未清空会把文章
   * 落进 unmatched 桶）。手动生成沿用同款判据——状态块说的「队列已清空」
   * 必须是真话，否则用户会误以为「定时任务应该已经出报错了」。
   */
  const queuesDrained = computed(() => !queueBlocked.value && !embeddingBlocked.value)

  /**
   * 补填入口门槛（review 复查）：补填与自动生成同款双队列判据——队列没排空时
   * 补出来的候选不全，服务端会 409 `tag_queue_not_empty` / `embedding_queue_not_empty`。
   * 入口同步禁用，避免「看着能点、点了报错」；典型触发场景（停机/故障恢复后
   * 想补缺）恰好就是队列最可能堆着的时候。
   */
  const backfillBlocked = computed(() => !queuesDrained.value)

  // ── 对账状态（R-17 的判据来源）────────────────────────────────────────
  /** 对账至少成功过一次（0 = 从未成功 → 只呈现「读取中」）。 */
  const pollReady = computed(() => queue.lastReconciledAt.value > 0)
  /** 最近一次成功对账时刻（失败时保留旧值并标注此时刻）。 */
  const updatedAtText = computed(() => formatClock(queue.lastReconciledAt.value))
  /**
   * 最近一次成功对账的**原始时间戳**：上层用它当「时钟刻度」——真实时钟
   * （new Date()）不是响应式依赖，页面停驻跨过生成时刻后文案不会自己变；
   * 对账成功必使该值变化，于是下一次对账即可让文案重算（不引入定时器，
   * 守 client-poll-budget）。
   */
  const lastReconciledAt = computed(() => queue.lastReconciledAt.value)

  // ── 日报生成窗口 ──────────────────────────────────────────────────────
  const dailyScheduler = computed(() => poll.schedulers.value.find(s => s.name === 'daily_report'))
  /** 生成时刻（缺省降级 21:00）。 */
  const scheduleTime = computed(() => dailyScheduler.value?.schedule_time || DEFAULT_SCHEDULE_TIME)
  /** 兜底强制生成时刻（旧后端缺 schedule_deadline → 23:30，V4/V13）。 */
  const scheduleDeadline = computed(() => dailyScheduler.value?.schedule_deadline || DEFAULT_SCHEDULE_DEADLINE)

  // ── 日报落点 / 当日判定 ───────────────────────────────────────────────
  /** 各板最新日报日期中的最大值（空 = 暂无日报）。 */
  const latestReportDate = computed(() => rows.value.reduce(
    (max, row) => (row.reportDate && row.reportDate > max ? row.reportDate : max),
    '',
  ))
  /** 勾选清单用的板块条目（数字 ID；ID 不可解析的板块剔除）。 */
  const boards = computed<RadarPipelineBoard[]>(() => rows.value
    .map((row) => ({
      boardId: Number(row.boardId),
      label: row.label,
      todayClues: row.todayClues,
      reportDate: row.reportDate,
    }))
    .filter((row) => Number.isFinite(row.boardId)))
  const boardTotal = computed(() => boards.value.length)
  /** 当日已出板块数（report_date === 本地今天；D7）。 */
  const todayReportCount = computed(() => boards.value.filter(row => row.reportDate === todayKey()).length)
  /** 今日已出（至少一个板块的 report_date == 本地今天）。 */
  const todayReady = computed(() => todayReportCount.value > 0)

  // ── 手动生成 ──────────────────────────────────────────────────────────
  const phase = ref<RadarGeneratePhase>('idle')
  const progressText = ref('')
  const result = ref<RadarGenerateResult | null>(null)
  const failureMessage = ref('')
  /** 每板最近一次状态（completed / failed），用于终态的成功/失败计数。 */
  const boardStatuses = ref<Map<number, string>>(new Map())

  // ── 缺口（近 7 天）─────────────────────────────────────────────────────
  // 缺口取数是**一次性请求**：挂载 / 补填终态 / 跨日切换各取一次——
  // 不新建周期性轮询（状态块既有约束），不塞进雷达总览的轮询载荷。
  const gaps = ref<DailyReportGap[]>([])
  const gapLoading = ref(false)
  const gapError = ref('')
  /** 成功取到过缺口（false = 未知；未知不得渲染成「无缺口」）。 */
  const gapLoaded = ref(false)
  const gapMissingDays = ref(0)
  const gapTotalMissing = ref(0)
  /** 实际窗口天数（服务端按保留窗口收窄；「一次补齐」复用同一口径）。 */
  const gapWindowDays = ref(GAP_WINDOW_DAYS)
  /** 最近一次补填运行摘要（服务端留档；无运行记录时为 null）。 */
  const lastRun = ref<DailyReportBackfillRunSummary | null>(null)

  /**
   * 缺口按日分组（日期升序）。
   *
   * 今日**不在缺口区呈现**（今日状态由既有「当日日报状态」行负责）——服务端
   * 窗口本就不含今天，这里再挡一层（口径漂移时也不把今天列成缺口）。
   * 板块名缺失时回退 `Board #N`（不编造板块名）。
   */
  const gapsByDate = computed<RadarGapDay[]>(() => {
    const today = todayKey()
    const byDate = new Map<string, RadarGapBoard[]>()
    for (const gap of gaps.value) {
      if (!gap.date || gap.date === today) continue
      const boards = byDate.get(gap.date) ?? []
      boards.push({
        boardId: Number(gap.board_id),
        label: gap.board_label?.trim() || `Board #${gap.board_id}`,
      })
      byDate.set(gap.date, boards)
    }
    return [...byDate.entries()]
      .sort((a, b) => (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0))
      .map(([date, boards]) => ({ date, boards }))
  })
  /** 缺口天数（服务端计数；前端不自行推算）。 */
  const missingDays = computed(() => gapMissingDays.value)
  /** 缺口篇数（服务端计数）。 */
  const totalMissing = computed(() => gapTotalMissing.value)

  /**
   * 取一次缺口清单。
   *
   * 失败只降级缺口区：保留「未知」（错误文案 + 重试入口），**不**影响卡片其余
   * 五项事实，也**不**把失败渲染成「无缺口」。
   */
  async function reloadGaps(): Promise<void> {
    gapLoading.value = true
    gapError.value = ''
    const res = await useDailyReportsApi().getDailyReportGaps({ days: GAP_WINDOW_DAYS })
    gapLoading.value = false
    if (!res.success || !res.data) {
      gapError.value = res.error || res.message || '读取失败'
      return
    }
    gaps.value = res.data.gaps ?? []
    gapMissingDays.value = res.data.missing_days ?? 0
    gapTotalMissing.value = res.data.total_missing ?? 0
    gapWindowDays.value = res.data.days ?? GAP_WINDOW_DAYS
    lastRun.value = res.data.last_run ?? null
    gapLoaded.value = true
  }

  // ── 补填（异步：受理即返回，进度与终态走 WS）──────────────────────────
  const backfillPhase = ref<RadarBackfillPhase>('idle')
  const backfillProgress = ref<RadarBackfillProgress>({ filled: 0, total: 0, date: '' })
  const backfillResult = ref<RadarBackfillResult | null>(null)
  const backfillError = ref('')
  /** 本次补填的 job_id（受理响应到达后落定；空 = 尚未受理）。 */
  const backfillJobId = ref('')
  /** 受理响应到达前缓存的补填进度消息（待 job_id 比对后回放/丢弃）。 */
  const bufferedBackfillProgress: Array<Record<string, unknown>> = []
  // 终态消息也可能先于受理响应到达（快失败）：同样先缓存，拿到 job_id 后按 id 回放。
  // 不缓冲会让 UI 永久卡在 running（review R-4）。
  const bufferedBackfillDone: Array<Record<string, unknown>> = []

  /**
   * 补填进度消息：`saved` = 已补数，`progress` = "3/7"（已补/总数），
   * `date` = 本格对应的报告日。只渲染服务端计数，不自行推算。
   */
  function applyBackfillProgress(msg: Record<string, unknown>): void {
    const filled = countOf(msg, 'saved') ?? 0
    const progressText = typeof msg.progress === 'string' ? msg.progress : ''
    const total = Number(progressText.split('/')[1] ?? '')
    backfillProgress.value = {
      filled,
      total: Number.isFinite(total) ? total : 0,
      date: typeof msg.date === 'string' ? msg.date : '',
    }
  }

  /**
   * 发起一次补填（异步受理）。
   *
   * `date` = 只补那天；`days` = 一次补齐（窗口与缺口清单同口径）。
   * 进行中（submitting / running）不重复受理——服务端闸门也会拦，但前端不
   * 把 409 当常态。
   */
  async function triggerBackfill(params: { date?: string; days?: number }): Promise<void> {
    if (backfillPhase.value === 'submitting' || backfillPhase.value === 'running') return
    backfillPhase.value = 'submitting'
    backfillError.value = ''
    backfillResult.value = null
    backfillProgress.value = { filled: 0, total: 0, date: '' }
    backfillJobId.value = ''
    bufferedBackfillProgress.length = 0
    bufferedBackfillDone.length = 0
    const res = await useDailyReportsApi().triggerGapBackfill(params)
    if (!res.success) {
      bufferedBackfillDone.length = 0
      backfillPhase.value = 'failed'
      backfillError.value = res.error || res.message || '补填提交失败，请稍后再试。'
      return
    }
    backfillJobId.value = res.data?.job_id ?? ''
    backfillPhase.value = 'running'
    // 回放受理响应到达前缓存的进度消息：只有 job_id 匹配的才算本次的。
    const buffered = bufferedBackfillProgress.splice(0, bufferedBackfillProgress.length)
    for (const msg of buffered) {
      if (backfillJobId.value && messageJobId(msg) !== backfillJobId.value) continue
      applyBackfillProgress(msg)
    }
    // 回放受理响应到达前缓存的**终态**（与进度同理，按 job_id 匹配）。
    const bufferedDone = bufferedBackfillDone.splice(0, bufferedBackfillDone.length)
    for (const msg of bufferedDone) {
      if (backfillJobId.value && messageJobId(msg) !== backfillJobId.value) continue
      onBackfillDone(msg)
    }
  }

  /** 补某一天（只覆盖该日；不为该日之外的缺口生成日报）。 */
  async function backfillDate(date: string): Promise<void> {
    await triggerBackfill({ date })
  }

  /** 一次补齐窗口内全部缺口（窗口与缺口清单同口径）。 */
  async function backfillAll(): Promise<void> {
    await triggerBackfill({ days: gapWindowDays.value })
  }

  /**
   * 补填终态（独立事件 `daily_report_backfill_done`）。
   *
   * 只认本 job 的消息（job_id 匹配）——别的补填 / 当日生成终态一律忽略。
   * 终态后重取一次缺口（补填改变了缺口清单）。
   */
  function onBackfillDone(msg: Record<string, unknown>): void {
    if (!backfillJobId.value) {
      if (backfillPhase.value === 'submitting') bufferedBackfillDone.push(msg)
      return
    }
    if (messageJobId(msg) !== backfillJobId.value) return
    const status = (typeof msg.status === 'string' ? msg.status : 'failed') as DailyReportBackfillStatus
    backfillResult.value = {
      status,
      filled: countOf(msg, 'filled') ?? 0,
      failed: countOf(msg, 'failed') ?? 0,
      remaining: countOf(msg, 'remaining') ?? 0,
      windowStart: typeof msg.window_start === 'string' ? msg.window_start : '',
      windowEnd: typeof msg.window_end === 'string' ? msg.window_end : '',
    }
    backfillPhase.value = 'done'
    backfillProgress.value = { filled: 0, total: 0, date: '' }
    void reloadGaps()
  }

  // ── WS 消息归属（job_id）──────────────────────────────────────────────
  // `daily_report_progress` / `daily_report_done` 是**全局广播**：请求在途
  // （响应还没到）时可能恰好收到并发管线的终态消息，误吸收会把别人的结果
  // 当成本次的。因此按 job_id 归属：响应到达前先缓存，拿到 job_id 后只回放
  // 匹配的；不匹配的消息一律忽略。
  /** 本次生成的 job_id（空 = 后端未返回，按旧口径接收任意消息）。 */
  let currentJobId = ''
  /** 响应是否已到达（未到达时收到的 WS 消息先缓存）。 */
  let responseArrived = false
  /** 响应到达前缓存的 WS 消息（可能属于并发管线，待 job_id 比对后回放/丢弃）。 */
  const bufferedMessages: Array<{ type: 'progress' | 'done'; msg: Record<string, unknown> }> = []

  /** 消息里的 job_id（缺字段 = 空串）。 */
  function messageJobId(msg: Record<string, unknown>): string {
    return typeof msg.job_id === 'string' ? msg.job_id : ''
  }

  function applyProgress(msg: Record<string, unknown>): void {
    if (phase.value !== 'submitting') return
    const boardId = Number(msg.board_id ?? 0)
    const status = typeof msg.status === 'string' ? msg.status : ''
    if (!Number.isFinite(boardId)) return
    const next = new Map(boardStatuses.value)
    next.set(boardId, status)
    boardStatuses.value = next
    progressText.value = typeof msg.progress === 'string' ? msg.progress : ''
  }

  /**
   * 终态消息里的计数（缺字段 = null = 消息不带该字段）。
   */
  function countOf(msg: Record<string, unknown>, key: string): number | null {
    const raw = msg[key]
    if (raw === undefined || raw === null) return null
    const value = Number(raw)
    return Number.isFinite(value) ? value : null
  }

  function applyDone(msg: Record<string, unknown>): void {
    if (phase.value !== 'submitting') return
    const statuses = [...boardStatuses.value.values()]
    const completed = statuses.filter(s => s === 'completed').length
    const progressFailed = statuses.filter(s => s === 'failed').length
    const total = countOf(msg, 'total_boards') ?? 0
    const saved = countOf(msg, 'total_saved')
    const failed = countOf(msg, 'failed_count')
    const empty = countOf(msg, 'empty_count')

    // 成功数：终态消息自带 total_saved（缺失时回落逐板进度消息的完成数）。
    const successBoards = saved ?? (statuses.length ? completed : total)

    let failedBoards: number
    if (failed !== null) {
      // 新协议：终态消息自带三态计数（成功 / 真失败 / 当日无内容），直接读，
      // **不**推算。「当日无内容」的板计入 empty_count——既不是成功也不是失败；
      // 用 total_boards - total_saved 推算会把它错算成失败（与通知中心口径不一致）。
      failedBoards = failed
    } else if (statuses.length) {
      // 旧后端回落 1：消息不带 failed_count → 用逐板 daily_report_progress 里
      // status === 'failed' 的计数（旧后端的进度消息同样区分完成/失败，
      // 可还原真失败数，且不受「当日无内容」干扰）。
      failedBoards = progressFailed
    } else {
      // 旧后端回落 2：进度消息全丢（消息丢失）→ 只能用旧口径
      // total_boards - total_saved 推算（此时「当日无内容」与「失败」不可分）。
      failedBoards = Math.max(total - (saved ?? 0), 0)
    }

    result.value = { successBoards, failedBoards, emptyBoards: empty ?? 0 }
    progressText.value = ''
    phase.value = 'done'
  }

  /** 收到 WS 消息：先按 job_id 归属，再决定处理还是忽略。 */
  function routeMessage(type: 'progress' | 'done', msg: Record<string, unknown>): void {
    if (phase.value !== 'submitting') return
    if (!responseArrived) {
      // 响应未到：先缓存（可能是并发管线的消息），拿到 job_id 后再回放
      bufferedMessages.push({ type, msg })
      return
    }
    if (currentJobId && messageJobId(msg) !== currentJobId) return
    if (type === 'progress') applyProgress(msg)
    else applyDone(msg)
  }

  function onProgress(msg: Record<string, unknown>): void {
    // 补填进度与当日生成进度**复用同一事件类型**：先按 run_scope 分流，
    // 再按 job_id 归属——否则会被当日生成的进度串台（补填进度报「3/7」时
    // 当日生成可能正在跑）。
    if (msg.run_scope === 'backfill') {
      if (!backfillJobId.value) {
        // 受理响应未到：先缓存（可能是并发补填），拿到 job_id 后再回放。
        if (backfillPhase.value === 'submitting') bufferedBackfillProgress.push(msg)
        return
      }
      if (messageJobId(msg) !== backfillJobId.value) return
      applyBackfillProgress(msg)
      return
    }
    routeMessage('progress', msg)
  }

  function onDone(msg: Record<string, unknown>): void {
    routeMessage('done', msg)
  }

  const stream = useEventStream()
  const unsubProgress = stream.on<Record<string, unknown>>(EVENT_TYPES.DAILY_REPORT_PROGRESS, onProgress)
  const unsubDone = stream.on<Record<string, unknown>>(EVENT_TYPES.DAILY_REPORT_DONE, onDone)
  const unsubBackfillDone = stream.on<Record<string, unknown>>(EVENT_TYPES.DAILY_REPORT_BACKFILL_DONE, onBackfillDone)
  if (getCurrentInstance()) {
    // 挂载取一次缺口（一次性请求；不新建周期性轮询）。
    onMounted(() => { void reloadGaps() })
    onUnmounted(() => {
      unsubProgress()
      unsubDone()
      unsubBackfillDone()
    })
  }

  /**
   * 跨日切换时重取一次缺口（窗口口径是「近 N 天」，跨日后清单变了）。
   *
   * 跨日判据用**最近一次对账时刻**作刻度（每次对账成功必变）——不为此引入
   * 定时器（client-poll-budget），当日判定仍用本地日 `todayKey()`。
   */
  let gapFetchedDay = todayKey()
  watch(lastReconciledAt, () => {
    const today = todayKey()
    if (today === gapFetchedDay) return
    gapFetchedDay = today
    void reloadGaps()
  })

  /**
   * 发起一次手动生成（作用域 = 勾选的板块集合）。
   * 失败（409 互斥 / 400 参数非法）不静默：恢复按钮可用并留下失败文案。
   */
  async function generate(boardIds: number[]): Promise<void> {
    if (phase.value === 'submitting') return
    if (!boardIds.length) return
    // 队列门槛与入口按钮同款判据（D8/D14）：对账从未成功或**双队列**任一有
    // 积压时不生成。UI 已禁用按钮，这里再挡一层——绕过 UI 直接调用也不放行
    // （清单展开期间有新文章/新标签入队时，入口按钮会变灰但确认按钮此前仍可点）。
    if (!pollReady.value || !queuesDrained.value) return
    phase.value = 'submitting'
    result.value = null
    failureMessage.value = ''
    progressText.value = ''
    boardStatuses.value = new Map()
    currentJobId = ''
    responseArrived = false
    bufferedMessages.length = 0
    const res = await useDailyReportsApi().generateDailyReport({
      date: todayKey(),
      board_ids: boardIds,
    })
    if (!res.success) {
      phase.value = 'failed'
      failureMessage.value = res.status === 409
        ? '日报生成正在执行中，请稍后再试。'
        : (res.error || res.message || '日报生成失败，请稍后再试。')
      return
    }
    // 记下本次 job_id：WS 消息按 job_id 归属，不吸收并发管线的消息。
    currentJobId = res.data?.job_id ?? ''
    responseArrived = true
    // 回放响应到达前缓存的消息：只有 job_id 匹配的才算本次的。
    const buffered = bufferedMessages.splice(0, bufferedMessages.length)
    for (const item of buffered) {
      if (currentJobId && messageJobId(item.msg) !== currentJobId) continue
      if (item.type === 'progress') applyProgress(item.msg)
      else applyDone(item.msg)
    }
  }

  return {
    // 队列
    pending,
    processing,
    activeCount,
    completedToday,
    failedCount,
    roundTotal,
    progressPercent,
    queueIdle,
    queueBlocked,
    // embedding 队列（第二环）
    embeddingActiveCount,
    embeddingCompletedToday,
    embeddingFailedCount,
    embeddingRoundTotal,
    embeddingProgressPercent,
    embeddingReady,
    embeddingBlocked,
    queuesDrained,
    // 对账
    pollReady,
    updatedAtText,
    lastReconciledAt,
    // 窗口
    scheduleTime,
    scheduleDeadline,
    // 日报
    latestReportDate,
    todayReportCount,
    todayReady,
    boardTotal,
    boards,
    overviewLoading,
    overviewError,
    // 生成
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
    lastRun,
    reloadGaps,
    // 补填
    backfillPhase,
    backfillProgress,
    backfillResult,
    backfillError,
    backfillDate,
    backfillAll,
    backfillBlocked,
  }
}
