/**
 * 共享描述修复 normalizer（board-health-triage-advice seam C1 / task 3.1）。
 *
 * wire 契约（implementation-contract §1 + 后端 auxrepair.JobView/RepairContext）：
 *  - GET  /auxiliary-labels/:id/description-repair-context   （纯读）
 *  - POST /auxiliary-labels/:id/description-repairs          （202 创建任务）
 *  - GET  /auxiliary-labels/:id/description-repairs/:job_id  （纯读）
 *  - POST /auxiliary-labels/:id/description-repairs/:job_id/retry（明确重试，无参）
 *
 * 归一纪律：
 *  - 数字 ID 在 wire 上保持数字，本模块在 API 边界转成字符串 ID 视图；
 *  - version 是服务端**不透明**令牌：只透传，不解析、不拼接、不本地构造；
 *  - 计数由服务端单源给出：processed（匹配引擎实际成功）/ failed（仍失败）/
 *    not_applicable（失效跳过，**不是成功**）/ remaining（wire 缺 = 未知，
 *    不填 0、不用差值反推）；
 *  - 执行元数据（claim_owner/phase_token/committed_version/committed_at 等私有列）
 *    不进 wire；本模块按白名单逐字段映射，即便 wire 混入也不透出。
 */
import type { ApiResponse } from '~/types'

// ── raw wire（snake_case，勿在前端消费）──────────────────────────────────────

export interface AuxiliaryRepairLabelRaw {
  id: number
  label: string
  description: string
  status: string
  /** 服务端不透明版本令牌。 */
  version: string
}

export interface AuxiliaryRepairBoardRaw {
  id: number
  label: string
  status: string
}

export interface AuxiliaryRepairJobRaw {
  id: number
  auxiliary_label_id: number
  idempotency_key?: string
  expected_version?: string
  old_description?: string
  new_description?: string
  stage?: string
  status?: string
  description_committed?: boolean
  total?: number
  processed?: number
  failed?: number
  not_applicable?: number
  /** 尚待处理项数；旧 wire 缺该字段 = 未知（null），不伪造 0。 */
  remaining?: number | null
  error?: string
  created_at?: string
  updated_at?: string
}

export interface AuxiliaryRepairContextRaw {
  auxiliary_label?: AuxiliaryRepairLabelRaw | null
  direct_boards?: AuxiliaryRepairBoardRaw[] | null
  active_tag_count?: number
  latest_task?: AuxiliaryRepairJobRaw | null
}

// ── 归一视图（camelCase + 字符串 ID）────────────────────────────────────────

/** 任务状态闭集（服务端词表）；未知值落 'unknown'，不猜语义。 */
export type AuxiliaryRepairJobStatus =
  | 'pending'
  | 'embedding'
  | 'saving'
  | 'matching'
  | 'completed'
  | 'embedding_failed'
  | 'save_failed'
  | 'matching_failed'
  | 'conflict'
  | 'unavailable'
  | 'unknown'

/** 任务阶段闭集（pending 可为空串）。 */
export type AuxiliaryRepairStage = '' | 'embedding' | 'saving' | 'matching' | 'unknown'

const JOB_STATUSES: readonly AuxiliaryRepairJobStatus[] = [
  'pending',
  'embedding',
  'saving',
  'matching',
  'completed',
  'embedding_failed',
  'save_failed',
  'matching_failed',
  'conflict',
  'unavailable',
]

const JOB_STAGES: readonly AuxiliaryRepairStage[] = ['', 'embedding', 'saving', 'matching']

/**
 * 轮询停止集：**停止**继续查询任务状态。
 * 含 completed/conflict/unavailable 与三种需用户明确重试的失败态；
 * 另含 unknown（服务端词表外的状态无法解读 → 停止查询并标可恢复，
 * **不得**当成后台失败，也不得当成完成）。
 */
export const AUXILIARY_REPAIR_POLL_STOP_STATUSES: readonly AuxiliaryRepairJobStatus[] = [
  'completed',
  'conflict',
  'unavailable',
  'embedding_failed',
  'save_failed',
  'matching_failed',
  'unknown',
]

/**
 * 后端同标签互斥释放集：只有这些状态后服务端才允许同标签**新建**任务
 * （后端 partial unique index 的终态集合：completed/conflict/unavailable）。
 * 三种失败态**仍持锁** → 只能走 retryTask，不得铸新键新建。
 */
export const AUXILIARY_REPAIR_MUTEX_RELEASED_STATUSES: readonly AuxiliaryRepairJobStatus[] = [
  'completed',
  'conflict',
  'unavailable',
]

/** 需用户明确 retry 的失败态（后台不自动重跑）。 */
export const AUXILIARY_REPAIR_RETRYABLE_STATUSES: readonly AuxiliaryRepairJobStatus[] = [
  'embedding_failed',
  'save_failed',
  'matching_failed',
]

/** 是否应停止轮询（终态 + unknown）。 */
export function isAuxiliaryRepairPollStopStatus(status: AuxiliaryRepairJobStatus): boolean {
  return AUXILIARY_REPAIR_POLL_STOP_STATUSES.includes(status)
}

/**
 * 是否允许**显式新确认**（铸新键新建任务）。
 * 只有后端互斥已释放的终态才允许；失败态与 unknown 一律保守禁止。
 */
export function isAuxiliaryRepairNewConfirmAllowedStatus(status: AuxiliaryRepairJobStatus): boolean {
  return AUXILIARY_REPAIR_MUTEX_RELEASED_STATUSES.includes(status)
}

/**
 * 后端同标签互斥是否已释放（= 允许显式新确认的同一集合）。
 * 语义别名：便于在"轮询终态"与"互斥释放"两处分别取用。
 */
export function isAuxiliaryRepairMutexReleasedStatus(status: AuxiliaryRepairJobStatus): boolean {
  return isAuxiliaryRepairNewConfirmAllowedStatus(status)
}

/** 是否可由用户明确重试（只重排同一任务，不新建）。 */
export function isAuxiliaryRepairRetryableStatus(status: AuxiliaryRepairJobStatus): boolean {
  return AUXILIARY_REPAIR_RETRYABLE_STATUSES.includes(status)
}

/**
 * 规范化辅助标签 ID（字符串视图的唯一形态）。
 *
 * 冻结存/查/解除、请求路径、上下文与任务归属校验**必须**用同一 canonical 值，
 * 否则 '01' 与 '1' 会分家（冻结永不解、任务认领错）。
 * 数字串去前导零（不做数字转换：int64 超出 2^53 时 Number 会丢精度）。
 */
export function canonicalAuxiliaryId(value: string | number | null | undefined): string {
  if (value === null || value === undefined) return ''
  const raw = String(value).trim()
  if (!raw) return ''
  if (/^\d+$/.test(raw)) return raw.replace(/^0+(?=\d)/, '')
  return raw
}

export interface AuxiliaryRepairLabelView {
  id: string
  label: string
  description: string
  status: string
  /** 服务端不透明版本令牌：只透传。 */
  version: string
}

export interface AuxiliaryRepairBoardView {
  id: string
  label: string
  status: string
}

export interface AuxiliaryRepairJobView {
  id: string
  auxiliaryLabelId: string
  /** 客户端自己的幂等键（同键同体重发返回同一任务）。 */
  idempotencyKey: string
  /** 来源版本令牌（不透明，只透传）。 */
  expectedVersion: string
  oldDescription: string
  newDescription: string
  stage: AuxiliaryRepairStage
  status: AuxiliaryRepairJobStatus
  /** 描述+向量是否已原子提交（≠ 匹配全部完成）。 */
  descriptionCommitted: boolean
  /** 创建时冻结的范围数量；缺字段/非法 → null（未知，不捏造 0）。 */
  total: number | null
  /** 匹配引擎实际成功数（合法零命中也算成功）；缺字段/非法 → null（未知）。 */
  processed: number | null
  /** 仍处失败态的数量；缺字段/非法 → null（未知）。 */
  failed: number | null
  /** 处理时已失效/删除而跳过的数量（**不是成功**）；旧 wire 缺字段按 0 读。 */
  notApplicable: number
  /** 尚待处理数量；wire 缺该字段时为 null（未知，不展示、不补 0）。 */
  remaining: number | null
  error: string
  createdAt: string
  updatedAt: string
}

export interface AuxiliaryRepairContextView {
  label: AuxiliaryRepairLabelView
  /** 直接引用该标签的版块（共享影响范围）；null = 字段缺失（**未知**，不谎称"没有版块引用"）。 */
  directBoards: AuxiliaryRepairBoardView[] | null
  /** 匹配更新覆盖的活跃话题标签数量；null = 字段缺失（**未知**，不谎称 0 范围）。 */
  activeTagCount: number | null
  /** 最近一条持久任务（null = 该标签暂无任务）。 */
  latestTask: AuxiliaryRepairJobView | null
}

function str(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

/**
 * 非负整数计数；缺失/非法 → null（**未知**）。
 * 不得捏造 0：那会把“未知”说成“零项/零失败”，并据此造出完成进度。
 */
function optionalCount(value: unknown): number | null {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) return null
  return Math.floor(value)
}

/** 兼容计数：缺失/非法 → 0（仅用于 not_applicable 的旧 wire 兼容）。 */
function legacyCount(value: unknown): number {
  return optionalCount(value) ?? 0
}

/** remaining：缺失/非法 → null（未知）；绝不落 0。 */
function remainingCount(value: unknown): number | null {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) return null
  return Math.floor(value)
}

function jobStatus(value: unknown): AuxiliaryRepairJobStatus {
  return typeof value === 'string' && (JOB_STATUSES as readonly string[]).includes(value)
    ? (value as AuxiliaryRepairJobStatus)
    : 'unknown'
}

function jobStage(value: unknown): AuxiliaryRepairStage {
  if (typeof value !== 'string') return 'unknown'
  return (JOB_STAGES as readonly string[]).includes(value) ? (value as AuxiliaryRepairStage) : 'unknown'
}

/**
 * 归一化持久任务。按白名单逐字段映射（私有执行元数据天然被丢弃）。
 * raw 为空 → null（不造任务状态）。
 */
export function normalizeAuxiliaryRepairJob(raw: AuxiliaryRepairJobRaw | null | undefined): AuxiliaryRepairJobView | null {
  if (!raw || typeof raw !== 'object') return null
  if (typeof raw.id !== 'number' || !Number.isFinite(raw.id)) return null
  return {
    id: canonicalAuxiliaryId(raw.id),
    auxiliaryLabelId: canonicalAuxiliaryId(raw.auxiliary_label_id),
    idempotencyKey: str(raw.idempotency_key),
    expectedVersion: str(raw.expected_version),
    oldDescription: str(raw.old_description),
    newDescription: str(raw.new_description),
    stage: jobStage(raw.stage),
    status: jobStatus(raw.status),
    descriptionCommitted: raw.description_committed === true,
    total: optionalCount(raw.total),
    processed: optionalCount(raw.processed),
    failed: optionalCount(raw.failed),
    notApplicable: legacyCount(raw.not_applicable),
    remaining: remainingCount(raw.remaining),
    error: str(raw.error),
    createdAt: str(raw.created_at),
    updatedAt: str(raw.updated_at),
  }
}

/**
 * 归一化修复上下文（纯读载荷）。
 * 缺 auxiliary_label → null（无可用上下文；上层报"上下文不可用"，不造版本）。
 */
export function normalizeAuxiliaryRepairContext(raw: AuxiliaryRepairContextRaw | null | undefined): AuxiliaryRepairContextView | null {
  if (!raw || typeof raw !== 'object') return null
  const label = raw.auxiliary_label
  if (!label || typeof label !== 'object') return null

  const boards = Array.isArray(raw.direct_boards)
    ? raw.direct_boards.map((board) => ({
        id: canonicalAuxiliaryId(board?.id),
        label: str(board?.label),
        status: str(board?.status),
      }))
    : null
  return {
    label: {
      id: canonicalAuxiliaryId(label.id),
      label: str(label.label),
      description: str(label.description),
      status: str(label.status),
      version: str(label.version),
    },
    directBoards: boards,
    activeTagCount: optionalCount(raw.active_tag_count),
    latestTask: normalizeAuxiliaryRepairJob(raw.latest_task ?? null),
  }
}

/** 客户端分支响应：成功 = 归一视图；失败 = 原样透传（error/status/data）。 */
export type AuxiliaryRepairContextResponse =
  | { success: true; data: AuxiliaryRepairContextView; message?: string }
  | { success: false; error?: string; status?: number; message?: string; data?: unknown }

/** 客户端分支响应（任务）。 */
export type AuxiliaryRepairJobResponse =
  | { success: true; data: AuxiliaryRepairJobView; message?: string }
  | { success: false; error?: string; status?: number; message?: string; data?: unknown }

/** 描述三规则的字符上限（Unicode 码点，与后端 MaxRepairDescriptionRunes 同值）。 */
export const REPAIR_DESCRIPTION_MAX_RUNES = 500

/**
 * 描述三规则（与后端 ValidateRepairDescription 同一闭集，不加第四条）：
 * trim 后非空、≤500 码点、不得**恰好**等于标签名（大小写/分隔符变体仍合法）。
 * 返回规范化（trim 后）描述，或错误文案（null = 通过）。
 */
export function validateRepairDescription(
  label: string,
  description: string,
): { ok: true; value: string } | { ok: false; error: string } {
  const trimmed = description.trim()
  if (!trimmed) return { ok: false, error: '描述不能为空' }
  if ([...trimmed].length > REPAIR_DESCRIPTION_MAX_RUNES) {
    return { ok: false, error: `描述不能超过 ${REPAIR_DESCRIPTION_MAX_RUNES} 个字符` }
  }
  if (trimmed === label) return { ok: false, error: '描述不能只重复标签名' }
  return { ok: true, value: trimmed }
}

/** context 分支接线：raw 响应 → 归一读模型；失败原样透传。 */
export function mapAuxiliaryRepairContextResponse(res: ApiResponse<AuxiliaryRepairContextRaw>): AuxiliaryRepairContextResponse {
  if (res.success && res.data) {
    const data = normalizeAuxiliaryRepairContext(res.data)
    if (data) return { success: true, data, message: res.message }
    return { success: false, error: '修复上下文不可用', status: res.status, message: res.message }
  }
  return { success: false, error: res.error, status: res.status, message: res.message, data: res.data }
}

/** task 分支接线：raw 响应 → 归一读模型；失败原样透传。 */
export function mapAuxiliaryRepairJobResponse(res: ApiResponse<AuxiliaryRepairJobRaw>): AuxiliaryRepairJobResponse {
  if (res.success && res.data) {
    const data = normalizeAuxiliaryRepairJob(res.data)
    if (data) return { success: true, data, message: res.message }
    return { success: false, error: '修复任务响应不可用', status: res.status, message: res.message }
  }
  return { success: false, error: res.error, status: res.status, message: res.message, data: res.data }
}
