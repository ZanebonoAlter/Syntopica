/**
 * 共享描述修复 composable（board-health-triage-advice seam C1 / task 3.1，
 * C1 独立 review 修复版）。
 *
 * 职责边界：只做取数 + 状态机，**不做 UI 布局决定**（组件层 3.2/3.3 消费本状态）。
 *
 * 取数（全部经 api/auxiliaryLabels.ts，ApiClient 是唯一 HTTP 边界）：
 *  - 修复上下文（纯读）：原描述 / 不透明版本 / 直接引用版块 / 活跃标签数 /
 *    最新任务；**重进从 context.latest_task 恢复**已知任务，不凭本地 map 假冒进度。
 *
 * 提交纪律（一次确认 = 一个幂等键 + 一份冻结 payload）：
 *  - 确认后 payload（description + expected_version）与幂等键一起**冻结**，按标签存；
 *  - 超时 / 网络错 / 5xx / 408 等超时取消类 = **结果未知**：重发必须同键同体；
 *  - 改了描述不得复用旧键（拒绝并提示先重发原内容或加载最新版本重新核对）；
 *  - 4xx（不含超时取消类）是服务端**确定拒绝**（未建任务）→ 解冻，可改正后换新键；
 *  - 已知任务在出错时保留，不重置为"新提交"；409 保留草稿并透传服务端 message。
 *
 * 冻结解除的唯一凭据（implementation-contract §3 澄清）：
 *  - **同键 + 同 payload + 归属一致**的任务才证明"本次确认已被受理"；
 *  - 单次 context 查询为空、或只返回**别的幂等键**任务，都**不能**证明原请求
 *    未被受理 → 不解冻、不换键、不把别的任务当本次结果；
 *  - 查询为空/他键任务**不是**撤销授权：未知结果的确认必须继续用同键同体。
 *
 * 轮询纪律（按明确 task id，有界）：
 *  - 正间隔 + 有限预算；终态与 unknown 停止（unknown 标可恢复，**不当后台失败**）；
 *  - 网络错 / 预算耗尽 → 保留已知任务并标"查询暂停·可恢复"，不伪造完成或失败，
 *    不再 POST 建任务；手动恢复只查询同一 job；
 *  - 轮询只发 GET，不写任何东西；卸载/切标签停 timer；迟到响应按 epoch 丢弃。
 *
 * 生命周期：
 *  - 在 Vue 组件 / effect scope 内创建 → 作用域结束**自动 dispose**；
 *  - scope 外创建 → dev 提示并要求**显式 dispose()**；
 *  - `reset()` 只是"清当前选择"，**可复用**（随后 loadContext 仍可用）；
 *    `dispose()` 才是永久结束（此后不再写任何 ref、不再轮询）。
 *  - 停止本地查询**不取消**后台已确认任务（服务端任务照跑）。
 */
import { computed, getCurrentScope, onScopeDispose, ref } from 'vue'
import { useAuxiliaryLabelsApi } from '~/api/auxiliaryLabels'
import {
  canonicalAuxiliaryId,
  isAuxiliaryRepairMutexReleasedStatus,
  isAuxiliaryRepairNewConfirmAllowedStatus,
  isAuxiliaryRepairPollStopStatus,
  isAuxiliaryRepairRetryableStatus,
  normalizeAuxiliaryRepairContext,
  normalizeAuxiliaryRepairJob,
  validateRepairDescription,
  type AuxiliaryRepairContextView,
  type AuxiliaryRepairJobView,
} from '~/api/normalizers/auxiliaryRepair'

const DEFAULT_POLL_INTERVAL_MS = 3000
const DEFAULT_MAX_POLL_ATTEMPTS = 60

/**
 * 超时 / 取消类响应（按 HTTP 状态码判定，**不解析错误文案**）。
 * 这类响应说明"服务端是否已受理"不可知 → 与传输错同等对待（结果未知）。
 */
const TIMEOUT_LIKE_STATUSES: readonly number[] = [408, 499]

export interface AuxiliaryDescriptionRepairOptions {
  /** 轮询间隔（毫秒，必须为正；非法值回落默认 3000）。 */
  pollIntervalMs?: number
  /** 轮询预算（最多查询次数；非法值回落默认 60）。 */
  maxPollAttempts?: number
  /** 幂等键生成器（默认 crypto.randomUUID；测试可注入）。 */
  createIdempotencyKey?: () => string
}

/** 提交/重试结果（错误只暴露给上层一次处理，composable 不 toast）。 */
export interface AuxiliaryRepairActionResult {
  ok: boolean
  error?: string
  /** true = 结果未知（网络错/5xx/超时类）：需同键同体重发，不得换键重建。 */
  uncertain?: boolean
  /** true = 409 冲突：保留草稿，UI 需加载最新版重新核对。 */
  conflict?: boolean
  task?: AuxiliaryRepairJobView
}

/** 冻结的确认（一次确认仅一键；同键同体重发返回同一任务）。 */
export interface FrozenRepairSubmit {
  auxId: string
  idempotencyKey: string
  description: string
  expectedVersion: string
  /** 该确认的结果未知（超时/网络错/5xx）：必须同键同体重发。 */
  uncertain: boolean
}

function defaultKeyFactory(): string {
  const cryptoRef = globalThis.crypto
  if (cryptoRef && typeof cryptoRef.randomUUID === 'function') return cryptoRef.randomUUID()
  return `repair-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
}

/** dev 环境判定（不依赖 vite/client 类型，vitest 下同样为 true）。 */
function isDevEnv(): boolean {
  const meta = import.meta as ImportMeta & { env?: { DEV?: boolean } }
  return meta.env?.DEV === true
}

/**
 * 该 HTTP 状态是否为"服务端确定拒绝"（未建任务 → 可解冻换新键）。
 * - 传输错（无 status）→ 未知；
 * - 5xx → 未知（服务端可能已落库）；
 * - 408/499 等超时取消类 → 未知；
 * - 其余 4xx → 确定拒绝。
 */
function isDefiniteRejection(status: number | undefined): boolean {
  if (typeof status !== 'number') return false
  if (TIMEOUT_LIKE_STATUSES.includes(status)) return false
  return status >= 400 && status < 500
}

export function useAuxiliaryDescriptionRepair(options: AuxiliaryDescriptionRepairOptions = {}) {
  const api = useAuxiliaryLabelsApi()

  const pollIntervalMs =
    typeof options.pollIntervalMs === 'number' && Number.isFinite(options.pollIntervalMs) && options.pollIntervalMs > 0
      ? options.pollIntervalMs
      : DEFAULT_POLL_INTERVAL_MS
  const maxPollAttempts =
    typeof options.maxPollAttempts === 'number' && Number.isFinite(options.maxPollAttempts) && options.maxPollAttempts > 0
      ? Math.floor(options.maxPollAttempts)
      : DEFAULT_MAX_POLL_ATTEMPTS
  const createKey = options.createIdempotencyKey ?? defaultKeyFactory

  // ── 状态 ──────────────────────────────────────────────────────────────────
  const context = ref<AuxiliaryRepairContextView | null>(null)
  const contextLoading = ref(false)
  const contextError = ref<string | null>(null)

  /** 当前已知任务（提交成功后 / 从 context.latest_task 恢复 / 轮询刷新）。 */
  const task = ref<AuxiliaryRepairJobView | null>(null)
  const taskError = ref<string | null>(null)

  const submitting = ref(false)
  const retrying = ref(false)
  const submitError = ref<string | null>(null)
  /** 409 冲突：草稿保留，UI 需加载最新版重新核对（message 用服务端原文）。 */
  const conflict = ref(false)
  /** 用户正在编辑的新描述（草稿）。 */
  const draft = ref('')

  const polling = ref(false)
  /** 查询暂停（网络错 / 预算耗尽 / 状态不可解读）：可手动恢复，只查同一 job。 */
  const pollPaused = ref(false)

  // ── 内部身份守卫（防迟到响应串台）────────────────────────────────────────
  /** 生命周期：dispose 后不再写任何 ref、不再轮询。 */
  let disposed = false
  /** 上下文世代：切标签/重置/释放时自增，迟到响应直接丢弃。 */
  let contextEpoch = 0
  /** 轮询世代：停轮询/换任务时自增。 */
  let pollGen = 0
  let pollTimer: ReturnType<typeof setTimeout> | null = null
  let pollAttempts = 0
  let pollTarget: { auxId: string; taskId: string } | null = null
  /** 当前上下文所属标签（canonical 字符串 ID）。 */
  const currentAuxId = ref<string | null>(null)
  /** 按标签冻结的确认（同标签一次确认仅一键）。 */
  const frozenSubmits = new Map<string, FrozenRepairSubmit>()
  /** 冻结变更计数：Map 非响应式，靠它让冻结视图失效。 */
  const frozenVersion = ref(0)
  /** 提交序号：迟到的旧提交响应不得清掉新提交的状态。 */
  let submitSeq = 0

  function setFrozen(frozen: FrozenRepairSubmit) {
    frozenSubmits.set(frozen.auxId, frozen)
    frozenVersion.value++
  }

  function clearFrozen(auxId: string) {
    if (frozenSubmits.delete(auxId)) frozenVersion.value++
  }

  /** 标记该标签的冻结为"结果未知"（保留键与 payload，等同键重发）。 */
  function markFrozenUncertain(auxId: string) {
    const frozen = frozenSubmits.get(auxId)
    if (!frozen || frozen.uncertain) return
    frozenSubmits.set(auxId, { ...frozen, uncertain: true })
    frozenVersion.value++
  }

  const frozenSubmit = computed<FrozenRepairSubmit | null>(() => {
    // 读 frozenVersion 建立依赖（Map 本身非响应式）。
    void frozenVersion.value
    return currentAuxId.value === null ? null : frozenSubmits.get(currentAuxId.value) ?? null
  })

  /** 当前标签的确认结果是否未知（由冻结记录派生，reset 与冻结状态恒一致）。 */
  const submitUncertain = computed(() => frozenSubmit.value?.uncertain === true)

  function clearPollTimer() {
    if (pollTimer) {
      clearTimeout(pollTimer)
      pollTimer = null
    }
  }

  /** 停轮询（切标签/终态/卸载/手动停止）：世代自增使在途响应失效。 */
  function stopPolling() {
    pollGen++
    pollTarget = null
    pollAttempts = 0
    clearPollTimer()
    polling.value = false
  }

  function scheduleNextPoll() {
    if (disposed) return
    clearPollTimer()
    pollTimer = setTimeout(() => {
      void pollOnce()
    }, pollIntervalMs)
  }

  /**
   * 启动按 task id 的有界轮询（只 GET）。
   * 已有轮询先停：同一时刻只跟一个任务。
   */
  function startPolling(auxId: string, taskId: string) {
    if (disposed) return
    stopPolling()
    pollTarget = { auxId, taskId }
    pollAttempts = 0
    polling.value = true
    pollPaused.value = false
    scheduleNextPoll()
  }

  async function pollOnce() {
    const target = pollTarget
    if (!target || disposed) return
    const gen = pollGen
    pollAttempts += 1

    const res = await api.getDescriptionRepairJob(target.auxId, target.taskId)
    // 迟到守卫：释放/停轮询/切标签/换任务后返回的响应一律丢弃（不写状态、不排下一发）。
    if (disposed || gen !== pollGen || pollTarget !== target) return

    if (!res.success || !res.data) {
      // 查询失败 ≠ 后台失败：保留已知任务，暂停查询（可恢复），不伪造状态。
      pollPaused.value = true
      taskError.value = res.error || '任务状态查询失败，可稍后恢复查询'
      stopPolling()
      return
    }
    const job = normalizeAuxiliaryRepairJob(res.data)
    if (!job) {
      pollPaused.value = true
      taskError.value = '任务状态响应不可用，可稍后恢复查询'
      stopPolling()
      return
    }

    task.value = job
    taskError.value = null

    if (job.status === 'unknown') {
      // 词表外状态：无法解读 → 停止轮询并标可恢复；
      // 既不当后台失败、也不当完成（等用户重新核对）。
      pollPaused.value = true
      taskError.value = '任务状态未知，可稍后恢复查询'
      stopPolling()
      return
    }
    if (isAuxiliaryRepairPollStopStatus(job.status)) {
      stopPolling()
      return
    }
    if (pollAttempts >= maxPollAttempts) {
      // 预算耗尽：保留已知任务、标可恢复，不伪造后台失败，也不再次 POST。
      pollPaused.value = true
      taskError.value = '任务状态查询已达预算上限，可手动恢复查询'
      stopPolling()
      return
    }
    scheduleNextPoll()
  }

  /**
   * 装载修复上下文（纯读，零写入）。
   *
   * 冻结解除纪律（implementation-contract §3）：
   *  - 只有**同键 + 同 payload + 归属一致**的任务才证明本次确认已被受理 → 采纳并解冻；
   *  - 无冻结时采纳服务端最新任务（重新进入恢复）；
   *  - 查询为空 / 他键任务 → **不解冻**，也不把别的任务当本次结果。
   */
  async function loadContext(auxId: string): Promise<AuxiliaryRepairContextView | null> {
    if (disposed) return null
    const requestedId = canonicalAuxiliaryId(auxId)
    if (!requestedId) {
      contextError.value = '缺少标签 ID'
      return null
    }
    const epoch = ++contextEpoch
    // 切标签：停旧轮询（迟到响应由 epoch 守卫丢弃）。
    stopPolling()
    if (currentAuxId.value !== null && currentAuxId.value !== requestedId) {
      // 切标签：清当前视图（草稿/任务/错误）；各标签的冻结记录**保留**（未知意图不丢）。
      draft.value = ''
      task.value = null
      taskError.value = null
      submitError.value = null
      conflict.value = false
    }
    currentAuxId.value = requestedId

    contextLoading.value = true
    contextError.value = null
    const res = await api.getDescriptionRepairContext(requestedId)
    if (disposed || epoch !== contextEpoch) return null // 迟到响应不串台
    contextLoading.value = false

    if (!res.success || !res.data) {
      context.value = null
      contextError.value = res.error || '修复上下文加载失败'
      return null
    }
    const view = normalizeAuxiliaryRepairContext(res.data)
    if (!view) {
      context.value = null
      contextError.value = '修复上下文不可用'
      return null
    }
    // 归属校验：服务端返回的标签必须就是请求的标签（防串台/认领错任务）。
    if (canonicalAuxiliaryId(view.label.id) !== requestedId) {
      context.value = null
      contextError.value = '修复上下文与请求标签不一致'
      return null
    }
    context.value = view
    // 重新核对完成：清掉上次的冲突/错误标记（**草稿保留**，用户输入不丢）。
    conflict.value = false
    submitError.value = null

    const frozen = frozenSubmits.get(requestedId)
    const latest = view.latestTask
    const latestBelongs = latest !== null && canonicalAuxiliaryId(latest.auxiliaryLabelId) === requestedId

    if (latestBelongs && latest) {
      if (!frozen) {
        // 无冻结 = 重新进入恢复：采纳服务端最新任务。
        task.value = latest
        taskError.value = null
        if (!isAuxiliaryRepairPollStopStatus(latest.status)) {
          startPolling(requestedId, latest.id)
        }
      } else if (isSameConfirmation(latest, frozen)) {
        // 同键 + 同 payload → 本次确认已被受理：采纳并解冻。
        task.value = latest
        taskError.value = null
        clearFrozen(requestedId)
        if (!isAuxiliaryRepairPollStopStatus(latest.status)) {
          startPolling(requestedId, latest.id)
        }
      } else {
        // 他键/异 payload 的任务 ≠ 本次确认的结果 → 不采纳、不解冻。
        task.value = null
        taskError.value = '上次提交结果未知，服务端返回的是另一个任务；请重发原内容以确认'
      }
    } else if (!frozen) {
      // 服务端无任务且无冻结：确实没有任务。
      task.value = null
    }
    // 有冻结 + 服务端无任务 → **不解冻**（查询为空不是撤销授权）。
    return view
  }

  /** 该任务是否就是被冻结的那次确认（同键 + 同 payload + 同标签）。 */
  function isSameConfirmation(job: AuxiliaryRepairJobView, frozen: FrozenRepairSubmit): boolean {
    return (
      job.idempotencyKey !== '' &&
      job.idempotencyKey === frozen.idempotencyKey &&
      job.newDescription === frozen.description &&
      job.expectedVersion === frozen.expectedVersion &&
      canonicalAuxiliaryId(job.auxiliaryLabelId) === canonicalAuxiliaryId(frozen.auxId)
    )
  }

  /** 更新草稿（用户编辑的新描述）。不改已冻结的 payload。 */
  function setDraft(value: string) {
    draft.value = value
  }

  /**
   * 提交确认（一次确认 = 一个幂等键 + 一份冻结 payload）。
   * - 已有冻结 → **同键同体**重发（结果未知）；改了内容则拒绝（不换键重建）。
   * - 无冻结 → 显式新确认：后端同标签互斥未释放时不得新建（失败态只能 retry）。
   * - 4xx（不含超时取消类）= 确定拒绝 → 解冻；超时/网络错/5xx = 结果未知 → 保留冻结。
   */
  async function submitRepair(): Promise<AuxiliaryRepairActionResult> {
    if (disposed) return { ok: false, error: '已释放，无法提交' }
    if (submitting.value) return { ok: false, error: '提交中，请稍候' }
    if (retrying.value) return { ok: false, error: '重试中，请稍候' }
    const ctx = context.value
    if (!ctx) return { ok: false, error: '尚未加载修复上下文' }
    if (ctx.label.status !== 'active') return { ok: false, error: '该标签已停用，不能执行修复' }
    if (!ctx.label.version) return { ok: false, error: '缺少实时版本，请重新加载上下文' }

    const auxId = canonicalAuxiliaryId(ctx.label.id)
    const frozen = frozenSubmits.get(auxId)

    let payload: { description: string; expectedVersion: string; idempotencyKey: string }
    if (frozen) {
      // 结果未知：只能同键同体重发（草稿为空则直接重发冻结内容）。
      const draftValue = draft.value.trim()
      if (draftValue && draftValue !== frozen.description) {
        return {
          ok: false,
          uncertain: true,
          error: '提交结果尚未确认，请先重发原确认内容，待后台确认后再修改。',
        }
      }
      payload = {
        description: frozen.description,
        expectedVersion: frozen.expectedVersion,
        idempotencyKey: frozen.idempotencyKey,
      }
    } else {
      // 显式新确认：后端同标签互斥未释放时不得新建（失败态只能走 retryTask）。
      if (task.value) {
        if (isAuxiliaryRepairRetryableStatus(task.value.status)) {
          return { ok: false, error: '该标签的修复任务处于失败待重试状态，请使用重试而不是新建' }
        }
        if (!isAuxiliaryRepairNewConfirmAllowedStatus(task.value.status)) {
          return { ok: false, error: '该标签已有进行中的修复任务' }
        }
      }
      const check = validateRepairDescription(ctx.label.label, draft.value)
      if (!check.ok) return { ok: false, error: check.error }
      payload = {
        description: check.value,
        expectedVersion: ctx.label.version,
        idempotencyKey: createKey(),
      }
      setFrozen({
        auxId,
        idempotencyKey: payload.idempotencyKey,
        description: payload.description,
        expectedVersion: payload.expectedVersion,
        uncertain: false,
      })
    }

    return postRepair(auxId, payload)
  }

  /**
   * 重发**原确认内容**（显式动作，不是新意图）：
   *  - 严格用冻结的幂等键 + 冻结的 payload（用户当前草稿不参与、也不被清空）；
   *  - 与新确认共用同一条发送管线（互斥 / epoch / 生命周期守卫一致）；
   *  - 结果确定后才解冻；来源仍是服务端，不自动新建任务。
   */
  async function resendFrozen(): Promise<AuxiliaryRepairActionResult> {
    if (disposed) return { ok: false, error: '已释放，无法重发' }
    if (submitting.value) return { ok: false, error: '提交中，请稍候' }
    if (retrying.value) return { ok: false, error: '重试中，请稍候' }
    const auxId = currentAuxId.value
    if (!auxId) return { ok: false, error: '尚未加载修复上下文' }
    const frozen = frozenSubmits.get(auxId)
    if (!frozen) return { ok: false, error: '没有待确认的提交' }
    return postRepair(auxId, {
      description: frozen.description,
      expectedVersion: frozen.expectedVersion,
      idempotencyKey: frozen.idempotencyKey,
    })
  }

  /**
   * 内部发送管线：新确认（submitRepair）与重发原确认（resendFrozen）**共用**。
   * 互斥（submitting）、epoch / 生命周期守卫、结果判定与解冻规则全在这一处，
   * 不复制第二套提交逻辑。
   */
  async function postRepair(
    auxId: string,
    payload: { description: string; expectedVersion: string; idempotencyKey: string },
  ): Promise<AuxiliaryRepairActionResult> {
    submitting.value = true
    submitError.value = null
    conflict.value = false
    const epoch = contextEpoch
    const seq = ++submitSeq

    const res = await api.createDescriptionRepair(auxId, {
      description: payload.description,
      expected_version: payload.expectedVersion,
      idempotency_key: payload.idempotencyKey,
    })
    if (seq !== submitSeq) {
      // 已被更新的提交取代：不碰状态（不得清掉新提交的 submitting）。
      return { ok: false, error: '本次提交已被新的提交取代' }
    }
    if (disposed) {
      // 已释放：不写任何 ref（冻结记录保持原样，等同键重发）。
      return { ok: false, uncertain: true, error: '已释放，本次提交结果未采纳' }
    }
    submitting.value = false
    if (epoch !== contextEpoch) {
      // 迟到响应（切标签/重置）：只维护该标签的冻结记录，**不污染当前视图的全局 ref**。
      if (res.success && res.data) clearFrozen(auxId)
      else if (isDefiniteRejection(res.status)) clearFrozen(auxId)
      else markFrozenUncertain(auxId)
      return { ok: false, uncertain: true, error: '标签已切换，本次提交结果未采纳' }
    }

    if (res.success && res.data) {
      const job = normalizeAuxiliaryRepairJob(res.data)
      if (job && canonicalAuxiliaryId(job.auxiliaryLabelId) === auxId) {
        clearFrozen(auxId)
        task.value = job
        taskError.value = null
        if (!isAuxiliaryRepairPollStopStatus(job.status)) {
          startPolling(auxId, job.id)
        }
        return { ok: true, task: job }
      }
      // 响应不可解析/归属不符：不当成功，按"结果未知"处理（保留冻结，可同键重发）。
      markFrozenUncertain(auxId)
      submitError.value = '修复任务响应不可用，请重发确认（同一幂等键）'
      return { ok: false, uncertain: true, error: submitError.value }
    }

    const message = res.error || '提交失败'
    submitError.value = message
    const status = res.status

    if (status === 409) {
      // 冲突（版本变化 / 同键异载荷 / 已有任务 / 标签停用……）：服务端确定拒绝；
      // 保留草稿，message 用服务端原文（不统一写成"版本变化"）。
      conflict.value = true
      clearFrozen(auxId)
      return { ok: false, conflict: true, error: message }
    }
    if (isDefiniteRejection(status)) {
      // 4xx 确定拒绝（未建任务）→ 改正后可用新键重提。
      clearFrozen(auxId)
      return { ok: false, error: message }
    }
    // 网络错（无 status）/ 5xx / 超时取消类：结果未知 → 保留冻结，重发同键同体。
    markFrozenUncertain(auxId)
    return { ok: false, uncertain: true, error: message }
  }

  /**
   * 明确重试已提交的任务：只打 retry 端点（无 body 参数），**不新建任务**、
   * 不改 expected_version / new_description / 冻结范围。
   */
  async function retryTask(): Promise<AuxiliaryRepairActionResult> {
    if (disposed) return { ok: false, error: '已释放，无法重试' }
    if (retrying.value) return { ok: false, error: '重试中，请稍候' }
    if (submitting.value) return { ok: false, error: '提交中，请稍候' }
    const current = task.value
    if (!current) return { ok: false, error: '没有可重试的任务' }
    if (!isAuxiliaryRepairRetryableStatus(current.status)) {
      return { ok: false, error: '当前状态不能重试' }
    }
    const auxId = canonicalAuxiliaryId(currentAuxId.value ?? context.value?.label.id ?? current.auxiliaryLabelId)
    if (!auxId) return { ok: false, error: '尚未加载修复上下文' }

    const epoch = contextEpoch
    retrying.value = true
    const res = await api.retryDescriptionRepair(auxId, current.id)
    if (disposed) {
      return { ok: false, error: '已释放，本次重试结果未采纳' }
    }
    retrying.value = false
    if (epoch !== contextEpoch) return { ok: false, error: '标签已切换，本次重试结果未采纳' }

    if (!res.success || !res.data) {
      taskError.value = res.error || '重试失败'
      return { ok: false, error: taskError.value }
    }
    const job = normalizeAuxiliaryRepairJob(res.data)
    if (!job) {
      taskError.value = '重试响应不可用'
      return { ok: false, error: taskError.value }
    }
    task.value = job
    taskError.value = null
    if (!isAuxiliaryRepairPollStopStatus(job.status)) {
      startPolling(auxId, job.id)
    }
    return { ok: true, task: job }
  }

  /** 手动恢复查询：只查同一 job（不 POST、不新建）。 */
  function resumePolling() {
    if (disposed) return
    const current = task.value
    if (!current) return
    const auxId = canonicalAuxiliaryId(currentAuxId.value ?? context.value?.label.id ?? current.auxiliaryLabelId)
    if (!auxId) return
    // 真实终态（互斥已释放）不再恢复；unknown/失败态可重新查询。
    if (isAuxiliaryRepairMutexReleasedStatus(current.status)) return
    startPolling(auxId, current.id)
  }

  /**
   * 清空当前选择（离开处理区）：停轮询、丢草稿与已知任务。
   * **不是** dispose：随后 loadContext 仍可用；各标签的冻结记录保留
   * （未知意图不因离开而消失，切回该标签可恢复）。
   */
  function reset() {
    if (disposed) return
    contextEpoch++
    stopPolling()
    currentAuxId.value = null
    context.value = null
    task.value = null
    contextError.value = null
    taskError.value = null
    submitError.value = null
    conflict.value = false
    submitting.value = false
    retrying.value = false
    pollPaused.value = false
    draft.value = ''
  }

  /**
   * 永久结束本 composable：停轮询、使所有在途响应失效，此后不再写任何 ref。
   * 只停**本地查询**，不取消服务端已确认的任务。
   */
  function dispose() {
    if (disposed) return
    disposed = true
    contextEpoch++
    stopPolling()
  }

  const scope = getCurrentScope()
  if (scope) {
    // 组件卸载 / effectScope.stop() → 自动释放。
    onScopeDispose(() => {
      dispose()
    })
  } else if (isDevEnv()) {
    // scope 外创建：没有自动回收点 → 明确要求显式 dispose。
    console.warn(
      '[useAuxiliaryDescriptionRepair] 未在 Vue 作用域（组件 / effectScope）内创建：'
        + '轮询定时器不会自动回收，请在不再使用时显式调用 dispose()。',
    )
  }

  return {
    // 状态
    context,
    contextLoading,
    contextError,
    task,
    taskError,
    submitting,
    retrying,
    submitError,
    conflict,
    submitUncertain,
    draft,
    polling,
    pollPaused,
    frozenSubmit,
    // 动作
    loadContext,
    setDraft,
    submitRepair,
    resendFrozen,
    retryTask,
    resumePolling,
    stopPolling,
    reset,
    dispose,
  }
}
