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
 *
 * R-17：首次对账从未成功时，本层只给「读取中」，**不产出**「今日日报未生成」
 * 这类业务结论；对账失败保留旧值由 `usePollBundle` 保证（不清零、不转错误态）。
 */
import { computed, getCurrentInstance, onUnmounted, ref, toValue, type MaybeRefOrGetter } from 'vue'
import { useDailyReportsApi } from '~/api/dailyReports'
import { useEmbeddingQueueProgress } from '~/composables/useEmbeddingQueueProgress'
import { useEventStream } from '~/composables/useEventStream'
import { usePollBundle } from '~/composables/usePollBundle'
import { useTagQueueProgress } from '~/composables/useTagQueueProgress'
import { EVENT_TYPES } from '~/utils/eventTypes'
import { todayKey } from '~/utils/todayKey'

/**
 * 状态块消费的**板块行最小结构**——只包含四项事实与勾选清单实际用到的字段。
 *
 * 上游雷达总览行携带更多字段（`todaySections` / `reportTitle` / `days` / `lanes` …），
 * 它在结构上可直接传进来；这里收窄声明的目的是让状态块只依赖自己真正需要的
 * 契约，而不是捆住整个雷达读模型。
 */
export interface RadarStatusBoard {
  /** 板块 ID（雷达读模型为字符串 ID；生成请求边界再转数字）。 */
  boardId: string | number
  label: string
  /** 该板块最新日报日期（`YYYY-MM-DD`；无日报为空串）。 */
  reportDate: string
  /** 该板块当日报纸分区数（0 = 今日无分区，勾选清单给提示）。 */
  todaySections: number
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
  /** 当日分区数：0 表示该板今天还没有料（生成出来是 0 篇，不算失败）。 */
  todaySections: number
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
      todaySections: row.todaySections,
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
    routeMessage('progress', msg)
  }

  function onDone(msg: Record<string, unknown>): void {
    routeMessage('done', msg)
  }

  const stream = useEventStream()
  const unsubProgress = stream.on<Record<string, unknown>>(EVENT_TYPES.DAILY_REPORT_PROGRESS, onProgress)
  const unsubDone = stream.on<Record<string, unknown>>(EVENT_TYPES.DAILY_REPORT_DONE, onDone)
  if (getCurrentInstance()) {
    onUnmounted(() => {
      unsubProgress()
      unsubDone()
    })
  }

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
  }
}
