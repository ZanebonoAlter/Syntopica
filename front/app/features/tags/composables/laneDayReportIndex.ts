/**
 * laneDayReportIndex — 板块报日索引（8C G1）的请求/覆盖/请求序/陈旧状态。
 *
 * 从 `useLaneDayReading` 按内聚职责抽出（code-style §8：大文件按内聚行为拆 composable，
 * 子模块以小 interface 显式依赖），仍是**唯一索引状态所有者**：父 store 创建一次并委托，
 * 产品路径不另建第二份索引/第二个读取器。合同逐条继承：
 *  - 索引仅 1 份/板块：列表 1 次 GET，日期轴、编年、其他话题组、报告优先读取全部复用；
 *  - 覆盖：`reportIndexDaysFor(poolStart, today)`（≥365，停更差额自动扩大）；需求扩大后整体
 *    替换不合并；force/更大在途请求的有效 days 不缩水（R3，生成后 anchor 前移不丢新报日）；
 *  - 请求序：每启动一次列表请求递增 `indexSeq`，旧响应晚到只转交最新请求结果，
 *    **不回写索引/错误**（新报日不得被旧列表缩回）；
 *  - 陈旧语义：失败保留旧索引只标 `indexError`，不把已知报日冒成空集、不当「无日报」；
 *  - 板块代际：`boardEpoch()` 由父 store 持有（切板块/卸载统一递增），与 boardId 双道判定
 *    晚到响应；在途合乘按 (epoch, days) 复用同一次 GET。
 *
 * 不拥有：详情缓存、`current`/`reportDay` 展示、选中日与任何 UI 状态（父 store 与消费方持有）。
 */
import { computed, ref, shallowRef, type ComputedRef, type Ref } from 'vue'
import { reportIndexDaysFor } from '~/api/laneDayReading'
import type { LaneDayReportIndexItem } from '~/api/normalizers/laneDayReading'
import { POOL_DAYS, shiftDate } from '../components/console/observationRange'

/** 当前板块报告列表索引（仅 1 份；days 扩大后整体替换，不反复取同范围）。 */
export interface ReportIndexState {
  days: number
  items: LaneDayReportIndexItem[]
}

/** 索引请求结果：stale=true 表示响应已过期（切板块/卸载），调用方直接放弃本次读取。 */
export interface IndexLoadResult {
  items: LaneDayReportIndexItem[] | null
  error: string | null
  stale: boolean
}

export interface LaneDayReportIndexDeps {
  /** 板块 ID（回写前与请求时的板块比对：晚到不落到新板块）。 */
  boardId: Ref<number>
  /** 今天（YYYY-MM-DD，Asia/Shanghai；报告列表「今日锚」差额计算用）。 */
  today: () => string
  /** 板块数据锚点（YYYY-MM-DD | null；池首 = anchor-(POOL_DAYS-1)，null 回退目标日）。 */
  anchorDate: () => string | null
  /** 日报列表端点（读注入，本模块不碰 HTTP）。 */
  list: (boardId: number, days: number) => Promise<{ success: boolean, data?: LaneDayReportIndexItem[], error?: string }>
  /** 板块代际（父 store 的切板块/卸载递增）；晚到响应不回填索引。 */
  boardEpoch: () => number
}

export interface LaneDayReportIndexReader {
  /** 成功索引（仅 1 份；消费方只读）。 */
  index: Ref<ReportIndexState | null>
  indexLoading: Ref<boolean>
  /** 索引失败/陈旧（保留旧索引），消费方可提示 + 重试；成功刷新后清空。 */
  indexError: Ref<string | null>
  /** 报日集合（真实 period_date；索引未成功前为空集，失败语义看 indexError，不把它当「无日报」）。 */
  reportDates: ComputedRef<ReadonlySet<string>>
  /** 覆盖所需 days 的索引（命中成功缓存则复用，否则取列表；force 绕过成功缓存）。 */
  loadIndex: (days: number, force?: boolean) => Promise<IndexLoadResult>
  /** 报日索引入口（日期轴/窄屏报日条）：覆盖不足才扩大重取，force = 生成完成后强制刷新。 */
  ensureReportIndex: (options?: { force?: boolean }) => Promise<void>
  /** 单日读取所需 days：覆盖池首与目标日更早者（目标日可早于池首）。 */
  requiredIndexDays: (date: string) => number
  /** 诊断：当前成功索引的覆盖天数。 */
  indexDays: () => number
  /** 切板块/卸载：清索引与在途槽（与父 store 的板块代际同步失效）。 */
  reset: () => void
}

export function createLaneDayReportIndex(deps: LaneDayReportIndexDeps): LaneDayReportIndexReader {
  const index = shallowRef<ReportIndexState | null>(null)
  const indexLoading = ref(false)
  const indexError = ref<string | null>(null)
  const reportDates = computed<ReadonlySet<string>>(
    () => new Set((index.value?.items ?? []).map(item => item.periodDate)),
  )

  /** 索引在途合乘（列表 1 份/板块：日期轴与编年首读共用同一次 GET）。 */
  let indexInflight: { epoch: number, seq: number, days: number, promise: Promise<IndexLoadResult> } | null = null
  /** 索引请求版本：每启动一次列表请求递增；旧响应晚到只准转交最新结果，不准回写状态。 */
  let indexSeq = 0

  /** 池首日（anchor-(POOL_DAYS-1)）；无板块锚点回退今天（只能按最小下限取）。 */
  function poolStartDate(): string {
    const anchor = deps.anchorDate()
    return anchor ? shiftDate(anchor, -(POOL_DAYS - 1)) : deps.today()
  }

  /** 索引 days：覆盖池首→今天的全部日历日（≥365；停更差额自动扩大，不把板块截止当服务器 today）。 */
  function reportIndexDays(): number {
    return reportIndexDaysFor(poolStartDate(), deps.today())
  }

  /** 单日读取所需 days：覆盖池首与目标日更早者（目标日可早于池首）。 */
  function requiredIndexDays(date: string): number {
    const poolStart = poolStartDate()
    return reportIndexDaysFor(poolStart < date ? poolStart : date, deps.today())
  }

  async function runIndexRequest(boardId: number, epoch: number, seq: number, days: number): Promise<IndexLoadResult> {
    const res = await deps.list(boardId, days)
    if (epoch !== deps.boardEpoch() || deps.boardId.value !== boardId) return { items: null, error: null, stale: true }
    if (seq !== indexSeq) {
      // 已被更新的列表请求超越：旧响应不写索引/错误（force 的新报日不得被旧列表缩回），
      // 也不能直接丢弃——把最新请求的结果转交给仍有效的 lane/report 消费者，避免永停在 loading。
      const latest = indexInflight
      if (latest !== null && latest.epoch === epoch && latest.seq > seq) return latest.promise
      return { items: index.value?.items ?? null, error: indexError.value, stale: false }
    }
    if (!res.success || !res.data) {
      // 失败保留旧索引（只标陈旧），不把已知报日冒成空集
      indexError.value = res.error || '日报列表加载失败'
      return { items: null, error: indexError.value, stale: false }
    }
    // 已有更大覆盖时不因旧请求回写到更小范围
    if ((index.value?.days ?? 0) <= days) index.value = { days, items: res.data }
    indexError.value = null
    return { items: index.value?.items ?? res.data, error: null, stale: false }
  }

  function requestIndex(days: number, force: boolean): Promise<IndexLoadResult> {
    const epoch = deps.boardEpoch()
    const shared = indexInflight
    if (!force && shared !== null && shared.epoch === epoch && shared.days >= days) return shared.promise
    // 覆盖收缩防护（R3）：新请求的有效 days 不得小于当前成功索引或同板块在途请求的覆盖。
    // 生成完成后 anchor 前移会让 reportIndexDays 从 369 降到 365；若真按 365 发请求，
    // runIndexRequest 的「已有更大覆盖不缩回」比较会把含新报日的新鲜列表丢掉 → 返回旧数据。
    // 取较大覆盖重取完整列表（整体替换，不合并、不猜测旧数据），新报日可见且覆盖不缩水。
    const effectiveDays = Math.max(
      days,
      index.value?.days ?? 0,
      shared !== null && shared.epoch === epoch ? shared.days : 0,
    )
    const boardId = deps.boardId.value
    const seq = ++indexSeq
    indexLoading.value = true
    const entry: { epoch: number, seq: number, days: number, promise: Promise<IndexLoadResult> } = {
      epoch,
      seq,
      days: effectiveDays,
      promise: Promise.resolve<IndexLoadResult>({ items: null, error: null, stale: true }),
    }
    entry.promise = runIndexRequest(boardId, epoch, seq, effectiveDays).finally(() => {
      if (indexInflight === entry) {
        indexInflight = null
        indexLoading.value = false
      }
    })
    indexInflight = entry
    return entry.promise
  }

  /** 索引：已有成功缓存且覆盖所需范围则复用；否则取列表（force 绕过成功缓存）；失败不清旧索引。 */
  async function loadIndex(days: number, force = false): Promise<IndexLoadResult> {
    if (!force && index.value !== null && index.value.days >= days) return { items: index.value.items, error: null, stale: false }
    return requestIndex(days, force)
  }

  /** 报日索引入口（日期轴/窄屏报日条）：覆盖不足才扩大重取，force = 生成完成后强制刷新。 */
  async function ensureReportIndex(options: { force?: boolean } = {}): Promise<void> {
    await loadIndex(reportIndexDays(), Boolean(options.force))
  }

  function reset(): void {
    indexInflight = null
    index.value = null
    indexLoading.value = false
    indexError.value = null
  }

  return {
    index,
    indexLoading,
    indexError,
    reportDates,
    loadIndex,
    ensureReportIndex,
    requiredIndexDays,
    indexDays: () => index.value?.days ?? 0,
    reset,
  }
}
