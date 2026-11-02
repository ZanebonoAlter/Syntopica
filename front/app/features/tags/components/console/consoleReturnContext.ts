/**
 * 控制台自有返回上下文（rework-workshop-into-radar-and-desk 矩阵阶段 G5）。
 *
 * 用途：去「研究工作台」（真实 `/tags/:boardId/research`，外部页面不受本 change 控制、
 * 返回时只 push `/tags/:boardId`）再回来时，矩阵自己保存并恢复
 * 泳道 / 选中日 / 观察范围（days/start/end）/ 滚动位（ui-design：返回上下文由矩阵自身保存）。
 *
 * 硬边界：
 *  - 只存本板块、只存矩阵视图状态；**不存** `report`/`annotation` 等一次性深链参数
 *    （避免返回时盲重放旧日报/批注导致循环）。
 *  - 一次性：读到即删（`take`），不长期滞留。
 *  - URL 显式值优先：路由已带 `lane/days/start/end` 时不使用缓存（`hasExplicitViewQuery`）。
 *  - sessionStorage 不可用（隐私模式/服务端）时静默降级为无缓存，不抛错、不影响主流程。
 */
import { parseStrictDate, parseTierDaysValue } from './observationRange'

/** 需恢复的矩阵视图状态（全部可选：只回填真实拿到的项）。 */
export interface ConsoleReturnContext {
  laneId: string | null
  date: string | null
  days: number | null
  start: string | null
  end: string | null
  scrollTop: number
  /**
   * 8C G4a：稳定「其他分区」阅读槽（`selectedOther`）。**不是 lane 身份**——不写 URL 四参、
   * 不参与 `laneId` 恢复，只在同一 board/报日下恢复下方阅读面板。缺省 = 旧 payload/未选其他行。
   */
  other?: ConsoleOtherSelection
  /** 8C G4a：其他组展开态（仅在 `other` 存在时保存；恢复阅读槽所需，不单独持久化用户折叠偏好）。 */
  othersOpen?: boolean
}

/** 其他分区阅读槽的稳定身份（sectionId/date 必需；reportId/topicId 从真实详情/行取得，可为空）。 */
export interface ConsoleOtherSelection {
  sectionId: string
  date: string
  reportId: string | null
  topicId: string | null
}

/** 其他分区阅读 key 命名空间（与 G3 `useOtherReadingSettled` 的 `laneKey` 同口径）。 */
export const OTHER_READING_KEY_PREFIX = 'other:'

/** `other:<sectionId>`：与泳道 topicId 阅读 key 区分，禁止塞进 `laneId`/URL `lane`。 */
export function otherReadingKey(sectionId: string): string {
  return `${OTHER_READING_KEY_PREFIX}${sectionId}`
}

const STORAGE_PREFIX = 'syntopica:board-console:return:'

type StorageLike = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>

function defaultStorage(): StorageLike | null {
  try {
    if (typeof window === 'undefined' || !window.sessionStorage) return null
    return window.sessionStorage
  } catch {
    return null
  }
}

function storageKey(boardId: number): string {
  return `${STORAGE_PREFIX}${boardId}`
}

/** 保存返回上下文（覆盖写；存储不可用/序列化失败静默放弃）。 */
export function saveReturnContext(boardId: number, context: ConsoleReturnContext, storage: StorageLike | null = defaultStorage()): void {
  if (!storage) return
  try {
    storage.setItem(storageKey(boardId), JSON.stringify(context))
  } catch {
    // 配额满/隐私模式：不影响矩阵本身
  }
}

/** 一次性取出返回上下文（读到即删；非法/缺失返回 null）。 */
export function takeReturnContext(boardId: number, storage: StorageLike | null = defaultStorage()): ConsoleReturnContext | null {
  if (!storage) return null
  let raw: string | null
  try {
    raw = storage.getItem(storageKey(boardId))
    if (raw !== null) storage.removeItem(storageKey(boardId))
  } catch {
    return null
  }
  if (raw === null) return null
  try {
    return sanitizeReturnContext(JSON.parse(raw))
  } catch {
    return null
  }
}

/**
 * URL 是否已表达视图状态（`lane/days/start/end` 任一非空）。
 * 显式合法值优先于返回缓存（G1：URL 是档/范围/选中的唯一事实源）。
 */
export function hasExplicitViewQuery(query: Record<string, unknown> | undefined): boolean {
  if (!query) return false
  for (const key of ['lane', 'days', 'start', 'end'] as const) {
    const raw = query[key]
    const value = Array.isArray(raw) ? raw[0] : raw
    if (typeof value === 'string' && value.trim() !== '') return true
  }
  return false
}

function idString(raw: unknown): string | null {
  return typeof raw === 'string' && /^\d+$/.test(raw) ? raw : null
}

/**
 * 其他分区阅读槽反序列化：sectionId/日期必须合法且与报告日一致，否则整项丢弃（旧 payload 无此项）。
 * reportId/topicId 非法只置 null（不猜、不按标题补）。
 */
function sanitizeOtherSelection(raw: unknown, date: string | null): ConsoleOtherSelection | null {
  if (date === null || typeof raw !== 'object' || raw === null) return null
  const value = raw as Record<string, unknown>
  const sectionId = idString(value.sectionId)
  const otherDate = typeof value.date === 'string' ? parseStrictDate(value.date) : null
  if (sectionId === null || otherDate === null || otherDate !== date) return null
  return { sectionId, date: otherDate, reportId: idString(value.reportId), topicId: idString(value.topicId) }
}

/** 反序列化收敛：只接受合法日期/四档天数/正数滚动位，其余置 null（不补造）。 */
export function sanitizeReturnContext(raw: unknown): ConsoleReturnContext | null {
  if (typeof raw !== 'object' || raw === null) return null
  const value = raw as Record<string, unknown>
  const laneRaw = value.laneId
  const laneId = typeof laneRaw === 'string' && /^\d+$/.test(laneRaw) ? laneRaw : null
  const date = typeof value.date === 'string' ? parseStrictDate(value.date) : null
  const days = typeof value.days === 'string' || typeof value.days === 'number'
    ? parseTierDaysValue(String(value.days))
    : null
  const start = typeof value.start === 'string' ? parseStrictDate(value.start) : null
  const end = typeof value.end === 'string' ? parseStrictDate(value.end) : null
  // 范围成对才可用（缺一端按无范围处理，交由 G1 默认区间）
  const hasRange = start !== null && end !== null && start <= end
  const scrollTop = typeof value.scrollTop === 'number' && Number.isFinite(value.scrollTop) && value.scrollTop > 0
    ? Math.round(value.scrollTop)
    : 0
  const other = sanitizeOtherSelection(value.other, date)
  if (laneId === null && date === null && days === null && !hasRange && scrollTop === 0 && other === null) return null
  return {
    laneId,
    date,
    days,
    start: hasRange ? start : null,
    end: hasRange ? end : null,
    scrollTop,
    // 旧六字段 payload 保持原形态（不新增空 other/othersOpen 键）；其他分区仅在真实存在时出现。
    ...(other === null
      ? {}
      : {
          other,
          othersOpen: typeof value.othersOpen === 'boolean' ? value.othersOpen : false,
        }),
  }
}
