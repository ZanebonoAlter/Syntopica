/**
 * 版块体检问题处理区 · 视图模型与纯派生（board-health-triage-advice seam C2 / tasks 3.2-3.3）。
 *
 * 只做**纯函数**：把 C1 归一后的分诊/信号与持久任务状态折成工作面要显示的形状。
 * 组件里不出现病型判定、统计公式或完成判据：
 *  - 分诊类别与信号来自服务端快照（`api/normalizers/boardHealthTriage`）；
 *  - 任务阶段/计数来自持久任务（`api/normalizers/auxiliaryRepair`）；
 *  - 本文件只把二者翻译成中文标签与阶段状态，**不重算任何口径**。
 */
import type { BoardHealthSnapshotDTO } from '~/api/semanticBoards'
import type {
  AuxiliaryRepairJobStatus,
  AuxiliaryRepairJobView,
} from '~/api/normalizers/auxiliaryRepair'
import type {
  BoardHealthMagnetView,
  BoardHealthTriageCategory,
  BoardHealthTriageGuardView,
  BoardHealthTriageItemView,
} from '~/api/normalizers/boardHealthTriage'
import { BOARD_HEALTH_MAGNET_CALIBER_HEALTHY_SUBSET } from '~/api/normalizers/boardHealthTriage'

/** 问题项的处置状态（只反映**真实**已知任务，不臆造进度）。 */
export type BoardHealthIssueStatus =
  /** 待核对（尚无该标签的任务）。 */
  | 'todo'
  /** 任务进行中（pending / embedding / saving / matching）。 */
  | 'running'
  /** 更新完成（匹配范围已处理结束；效果待下一轮体检）。 */
  | 'completed'
  /** 明确失败，等待用户重试（embedding_failed / save_failed / matching_failed）。 */
  | 'failed'
  /** 来源版本冲突（未覆盖数据，需加载最新重新核对）。 */
  | 'conflict'
  /** 标签已停用/不可用（本次未修改描述或向量）。 */
  | 'unavailable'
  /** 状态未知（服务端返回词表外状态；可恢复查询，不当失败/完成）。 */
  | 'unknown'

export const BOARD_HEALTH_ISSUE_STATUS_LABEL: Record<BoardHealthIssueStatus, string> = {
  todo: '待核对',
  running: '执行中',
  completed: '已更新',
  failed: '待重试',
  conflict: '需重新核对',
  unavailable: '标签不可用',
  unknown: '状态未知',
}

/** 工作面索引项（remove / rewrite：需核对项）。 */
export interface BoardHealthIssueItem {
  auxId: string
  label: string
  category: Extract<BoardHealthTriageCategory, 'remove' | 'rewrite'>
  evidence: string
  /** 建议文案（rewrite 才有；空串 = 缺建议，无法执行）。 */
  suggestion: string
  /** 是否可执行（缺建议的 rewrite 不可执行；非 auxiliary 的标签也不可执行）。 */
  actionable: boolean
  /**
   * 不可执行原因（非空 = 本项不可在处理区执行，只读展示证据）。
   * 组合标签（composite）无描述修复入口（服务端修复链只认 auxiliary）。
   */
  notExecutableReason: string | null
  /**
   * 核心主体标记（服务端按版块标题/描述/别名文本判定）。
   * false = 非核心主体，或旧快照缺该字段（按「非核心主体」处理，不臆造）。
   */
  coreSubject: boolean
  /** 该项当前状态（只反映真实任务）。 */
  status: BoardHealthIssueStatus
}

/** 「其余标签」折叠项（keep / insufficient：只读结论）。 */
export interface BoardHealthOtherItem {
  auxId: string
  label: string
  category: Extract<BoardHealthTriageCategory, 'keep' | 'insufficient'>
  evidence: string
  /** 核心主体标记（服务端判定；旧快照缺字段 = false）。 */
  coreSubject: boolean
}

/**
 * 描述修复可执行性（4.1 M1）：服务端描述修复链全程只认 auxiliary 标签——
 * 组合标签（composite）的「重写描述」没有对应入口（context 404），必须显式
 * 标为不可执行并说明原因（证据仍展示，不隐藏、不当它健康）。
 *
 * - auxiliary → 可执行（null = 无阻碍）；
 * - composite → 「组合标签暂不支持描述修复」；
 * - 未知/缺字段（旧快照）→ 「标签类型未知，无法执行描述修复」（不猜类型）。
 */
export function boardHealthRepairUnsupportedReason(labelType: string): string | null {
  if (labelType === 'auxiliary') return null
  if (labelType === 'composite') return '组合标签暂不支持描述修复'
  return '标签类型未知，无法执行描述修复'
}

/**
 * 把分诊项拆成「需核对项」与「其余标签」两类。
 *
 * 分流口径：remove / rewrite → 需核对；keep / insufficient → 其余标签。
 * 闸门降级项（服务端已落为 insufficient）因此**不进需核对索引**——不新增分类、
 * 不新增前端过滤逻辑（fix-board-health-magnet-signal D8）。
 */
export function splitTriageItems(
  items: BoardHealthTriageItemView[],
  labelOf: (auxId: string) => string,
  statusOf: (auxId: string) => BoardHealthIssueStatus,
): { issues: BoardHealthIssueItem[]; others: BoardHealthOtherItem[] } {
  const issues: BoardHealthIssueItem[] = []
  const others: BoardHealthOtherItem[] = []
  for (const item of items) {
    const label = labelOf(item.auxId)
    if (item.category === 'remove' || item.category === 'rewrite') {
      // 摘除走既有 DELETE 链（组合标签也可摘）；描述修复只对 auxiliary 可执行。
      const notExecutableReason = item.category === 'rewrite'
        ? boardHealthRepairUnsupportedReason(item.labelType)
        : null
      issues.push({
        auxId: item.auxId,
        label,
        category: item.category,
        evidence: item.evidence,
        suggestion: item.suggestion,
        actionable: item.actionable && !notExecutableReason,
        notExecutableReason,
        coreSubject: item.coreSubject,
        status: statusOf(item.auxId),
      })
    } else {
      others.push({ auxId: item.auxId, label, category: item.category, evidence: item.evidence, coreSubject: item.coreSubject })
    }
  }
  return { issues, others }
}

/** 贴合均值展示（null = 未知，不补 0）。 */
function formatAffinity(value: number | null): string {
  if (value === null || !Number.isFinite(value)) return '未知'
  return value.toFixed(3)
}

/**
 * 磁铁证据对比文案（纯派生；口径与数值都由服务端给）。
 *
 * 分支**按服务端口径标记 `caliber`**（fix-board-health-magnet-signal D12 / R-4），
 * 不再拿 `goodMean === null` 当旧快照判据——`good_mean` 无 `omitempty`，新快照在
 * 健康侧不可算时同样是 null，两者不可区分：
 *
 * - 新口径（`caliber = 'healthy_subset'`）且健康侧可算：
 *   「失败子集 X vs 健康子集 Y」——与磁铁判定口径一致；
 * - 新口径但健康侧不可算（`goodMean === null`）：「健康子集不可算（样本不足或
 *   向量缺失）」+ 失败子集数值，**不回退**全体均值、**不标**旧口径快照；
 * - 旧口径快照（`caliber` 为空）：回退「失败子集 X vs 全体 Y」并标注「旧口径快照」；
 * - `caliber` 为其它非空值（协议外的未来取值）：同样走旧口径分支——保守处理，
 *   宁可标成旧口径，也不冒充新口径结论。
 */
export function boardHealthMagnetComparisonText(magnet: BoardHealthMagnetView | null): string {
  if (!magnet) return '未知（样本不足或向量缺失，本轮未算）'
  const failed = `失败子集 ${formatAffinity(magnet.failedMean)}（${magnet.failedSamples} 项）`
  if (magnet.caliber === BOARD_HEALTH_MAGNET_CALIBER_HEALTHY_SUBSET) {
    if (magnet.goodMean === null) {
      return `${failed} · 健康子集不可算（样本不足或向量缺失）`
    }
    return `${failed} vs 健康子集 ${formatAffinity(magnet.goodMean)}（${magnet.goodSamples} 项）`
  }
  return `${failed} vs 全体 ${formatAffinity(magnet.allMean)}（${magnet.allSamples} 项）· 旧口径快照`
}

/**
 * 磁铁结论文案（纯派生；服务端已判，不重算差值）。
 *
 * - 新口径快照：照搬服务端结论（「差值显著，疑似磁铁污染」/「差值未达阈值」/
 *   「样本不足，不作判断」）；
 * - 旧口径快照（`caliber` 为空）：服务端的 `suspicious` 是**旧公式**
 *   （`all_mean − failed_mean`）算的，语义与新口径**相反**（旧的「suspicious」
 *   其实是「失败比全体低」）→ 结论文案必须带口径标记，不让用户读成新口径结论。
 */
export function boardHealthMagnetVerdictText(magnet: BoardHealthMagnetView | null): string {
  if (!magnet) return ''
  const base = magnet.insufficient
    ? '样本不足，不作判断'
    : magnet.suspicious
      ? '差值显著，疑似磁铁污染'
      : '差值未达阈值'
  if (magnet.caliber === BOARD_HEALTH_MAGNET_CALIBER_HEALTHY_SUBSET) return base
  return `旧口径快照的磁铁结论（口径已反转，勿据此操作）：${base}`
}

/** 闸门占比展示（只格式化服务端给的占比，前端不重算）。 */
function formatGuardRatio(value: number): string {
  return `${(value * 100).toFixed(0)}%`
}

/**
 * 闸门降级警示文案（纯派生）。
 *
 * 占比 / 阈值 / 原因一律取服务端 `triage_guard` 的值——前端不自己除法、不自己判超限。
 * 未降级（`degraded !== true`）或无闸门记录 → null（不显示警示条）。
 */
export function boardHealthTriageGuardNotice(guard: BoardHealthTriageGuardView | null): string | null {
  if (!guard || !guard.degraded) return null
  const parts: string[] = ['本轮信号可能不可用、不得按此批量操作。']
  if (guard.ratio !== null && guard.threshold !== null) {
    parts.push(`可执行占比 ${formatGuardRatio(guard.ratio)} 超过阈值 ${formatGuardRatio(guard.threshold)}。`)
  } else if (guard.ratio !== null) {
    parts.push(`可执行占比 ${formatGuardRatio(guard.ratio)}。`)
  }
  const reason = guard.reason.trim()
  if (reason) parts.push(reason)
  return parts.join('')
}

/** 任务状态 → 工作面处置状态。 */
export function boardHealthIssueStatus(status: AuxiliaryRepairJobStatus): BoardHealthIssueStatus {
  switch (status) {
    case 'pending':
    case 'embedding':
    case 'saving':
    case 'matching':
      return 'running'
    case 'completed':
      return 'completed'
    case 'embedding_failed':
    case 'save_failed':
    case 'matching_failed':
      return 'failed'
    case 'conflict':
      return 'conflict'
    case 'unavailable':
      return 'unavailable'
    default:
      return 'unknown'
  }
}

/** 三阶段展示状态（真实阶段由服务端 status/stage 决定，不自造）。 */
export type BoardHealthStageState = 'done' | 'current' | 'error' | 'pending'

export interface BoardHealthStageView {
  key: 'embedding' | 'saving' | 'matching'
  label: string
  state: BoardHealthStageState
}

const STAGE_LABELS: Record<'embedding' | 'saving' | 'matching', string> = {
  embedding: '生成新的语义向量',
  saving: '保存描述与向量（同一事务）',
  matching: '更新版块匹配',
}

/**
 * 三阶段状态序列。
 *
 * 只按服务端 `status`/`stage` 推导：
 *  - pending → 三阶段均未开始；
 *  - embedding / saving / matching → 对应阶段进行中，其前阶段已完成；
 *  - completed → 三阶段全部完成；
 *  - embedding_failed / save_failed / matching_failed → 对应阶段失败；
 *  - conflict / unavailable → 停在保存阶段（未覆盖数据）；
 *  - unknown → 全部未知（不当完成、不当失败）。
 */
export function boardHealthStageViews(task: AuxiliaryRepairJobView | null): BoardHealthStageView[] {
  const order: Array<'embedding' | 'saving' | 'matching'> = ['embedding', 'saving', 'matching']
  if (!task) return []
  const status = task.status
  if (status === 'unknown') {
    // 词表外状态：不当完成也不当失败，三阶段都未标。
    return order.map((key) => ({ key, label: STAGE_LABELS[key], state: 'pending' as BoardHealthStageState }))
  }
  if (status === 'completed') {
    return order.map((key) => ({ key, label: STAGE_LABELS[key], state: 'done' as BoardHealthStageState }))
  }
  const failedIndex = status === 'embedding_failed' ? 0 : status === 'save_failed' ? 1 : status === 'matching_failed' ? 2 : -1
  const currentIndex = status === 'embedding' ? 0 : status === 'saving' ? 1 : status === 'matching' ? 2 : -1
  // conflict / unavailable：停在保存阶段（未覆盖数据）。
  const conflictIndex = status === 'conflict' || status === 'unavailable' ? 1 : -1

  return order.map((key, index) => {
    let state: BoardHealthStageState = 'pending'
    if (failedIndex >= 0) {
      if (index < failedIndex) state = 'done'
      else if (index === failedIndex) state = 'error'
    } else if (currentIndex >= 0) {
      if (index < currentIndex) state = 'done'
      else if (index === currentIndex) state = 'current'
    } else if (conflictIndex >= 0) {
      if (index < conflictIndex) state = 'done'
      else if (index === conflictIndex) state = 'error'
    }
    return { key, label: STAGE_LABELS[key], state }
  })
}

/** 计数展示：null = 未知（不补 0、不伪造完成进度）。 */
export function formatRepairCounter(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '未知'
  return String(Math.floor(value))
}

/** 顶部一行「处理进展」摘要（只反映当前项的真实任务）。 */
export function boardHealthProgressText(task: AuxiliaryRepairJobView | null): string {
  if (!task) return ''
  const processed = formatRepairCounter(task.processed)
  const total = formatRepairCounter(task.total)
  switch (task.status) {
    case 'completed':
      return '更新完成，效果待下一轮体检验证'
    case 'embedding':
      return '正在生成新的语义向量'
    case 'saving':
      return '正在保存描述与向量'
    case 'matching':
      return `正在更新版块匹配（已处理 ${processed}/${total}）`
    case 'embedding_failed':
      return '生成新向量失败，旧描述与旧向量未改变'
    case 'save_failed':
      return '保存描述与向量失败，本次未提交'
    case 'matching_failed':
      return task.failed === null || task.failed === undefined
        ? '描述已保存，但有匹配更新失败项（数量未知）'
        : `描述已保存，但有 ${formatRepairCounter(task.failed)} 项匹配失败`
    case 'conflict':
      return '来源版本已变化，本次未覆盖数据'
    case 'unavailable':
      return '标签已不可用，本次未修改描述或向量'
    case 'pending':
      return '任务已排队，等待开始'
    default:
      return '任务状态未知，可稍后恢复查询'
  }
}


/** 工作面整体状态（旧快照/空/降级/异常一律不猜诊断）。 */
export type BoardHealthWorkspaceState =
  | 'loading'
  | 'error'
  /** 资源不存在/无权（404/403）：不是"刷新失败"，旧清单可能已失效。 */
  | 'gone'
  | 'no-snapshot'
  | 'old-snapshot'
  | 'degraded'
  | 'no-triage'
  | 'normal'

/**
 * 判定工作面状态。
 *
 * 顺序按后端真 wire（backend-go boardhealth/run.go）：
 *  - 降级轮 = LLM 裁决失败：`llm_score` 与 `llm_comment` **均为 NULL**（且 `metrics.triage` 为 JSON null）
 *    → 独立形态，**不能**推断当前标签健康；
 *  - 「历史快照」只留给**有评分无 triage**；
 *  - `triage` 有条目 → 正常；`triage` 为空数组 → 本轮没有分诊清单。
 */
export function boardHealthWorkspaceState(input: {
  loading: boolean
  error: string | null
  /** 最近一次失败的 HTTP 状态码（null = 无失败或网络失败无状态码）。 */
  errorStatus?: number | null
  snapshot: BoardHealthSnapshotDTO | null
  /** 分诊字段是否存在（false = 缺 triage 字段）。 */
  triageAvailable: boolean
  /** 分诊条目数。 */
  triageCount: number
}): BoardHealthWorkspaceState {
  if (input.loading) return 'loading'
  // 404/403：版块可能已删除/无权 —— 不是"刷新失败"，旧清单不得当作仍可操作。
  if (input.errorStatus === 404 || input.errorStatus === 403) return 'gone'
  // 读取失败且没有旧快照 → 错误态；有旧快照时按 stale 保留上次快照（网络/5xx）。
  if (input.error && !input.snapshot) return 'error'
  const snapshot = input.snapshot
  if (!snapshot) return 'no-snapshot'
  if (snapshot.llm_score === null && snapshot.llm_comment === null) return 'degraded'
  if (!input.triageAvailable) return 'old-snapshot'
  if (input.triageCount === 0) return 'no-triage'
  return 'normal'
}

/** 各状态形态的标题与说明文案（不猜诊断、不说全部健康）。 */
export function boardHealthWorkspaceStateCopy(state: BoardHealthWorkspaceState): { title: string; text: string } {
  switch (state) {
    case 'loading':
      return { title: '体检数据加载中…', text: '分诊随快照整体读取，不单独请求。' }
    case 'error':
      return { title: '暂时无法读取体检', text: '请重试；不会因读取失败创建修复任务。' }
    case 'gone':
      return {
        title: '这块内容可能已经不存在',
        text: '体检数据读取被拒绝或版块已删除（404/403）。请返回板块列表重新选择；此处不再展示可操作的清单。',
      }
    case 'no-snapshot':
      return { title: '等待首轮体检', text: '体检在每日日报生成后自动执行，届时这里会列出需要核对的问题。' }
    case 'old-snapshot':
      return {
        title: '历史快照',
        text: '这份快照没有分诊字段，按旧形态展示评语与统计——不据现有统计臆造问题。',
      }
    case 'degraded':
      return {
        title: '本轮未生成评分与分诊',
        text: '统计与信号已保留，但本轮没有评分与分诊；不能据此推断当前标签健康。',
      }
    case 'no-triage':
      return { title: '本轮没有分诊清单', text: '不按统计臆造问题。评分、评语与原统计仍保留。' }
    default:
      return { title: '', text: '' }
  }
}
