/**
 * laneDayReportDay — 「报告优先」当日读取（8C G1）：只要日期，不依赖选中 lane / lane-dynamics 当日桶。
 *
 * 从 `useLaneDayReading` 按内聚职责抽出（code-style §8），由父 store 创建一次并委托；
 * 与编年共用同一份报日索引与同一个详情缓存（不另建读取器/缓存）。合同逐条继承：
 *  - 索引 → 精确 `period_date` 命中（不做日期邻近推断）→ 详情（同一 `reportCache`）→ 发布
 *    带 `date/status/error/errorSource/reportId/sections` 的 `reportDay`；
 *  - 0 个 section 是合法 `ready`；无报日 / 索引失败 / 详情失败分层（`errorSource`），互不冒充；
 *  - **独立 epoch**：`reportSeq` 与 lane 读取互不取消；晚到响应只按自身序号丢弃；
 *  - **显示 gate**：消费方必须比对 `reportDay.date` 与自身所选日；跨日/首读先清显示
 *    （不把旧日 sections 挂在新日期标题下），同日刷新失败才 stale 保留旧内容。
 *
 * 不拥有：索引状态（`laneDayReportIndex`）、详情缓存本体（父 store 注入）、选中日与 UI。
 */
import { ref, shallowRef, type Ref, type ShallowRef } from 'vue'
import type {
  LaneDayReportDetail,
  LaneDayReportHighlight,
  LaneDayReportSection,
} from '~/api/normalizers/laneDayReading'
import type { BoundedRequestCache, BoundedCacheLoadResult } from './laneDayReadingCache'
import type { IndexLoadResult } from './laneDayReportIndex'

/** 报告优先读取的分层状态：loading / ready（0 分区合法）/ 无日报 / 读取失败。 */
export type ReportDayStatus = 'loading' | 'ready' | 'no-report' | 'error'

/** 失败来源分层：索引失败说明报日集合可能缺项（不得断言无日报），详情失败只影响当日。 */
export type ReportDayErrorSource = 'index' | 'detail' | null

/** 报告优先读取发布的当日模型（消费方渲染前必须比对 `date` 与自身所选日）。 */
export interface LaneReportDay {
  date: string
  status: ReportDayStatus
  error: string | null
  errorSource: ReportDayErrorSource
  reportId: string | null
  /** 该日报详情全量 sections（当日原序；0 个是合法结论，不补造）。 */
  sections: LaneDayReportSection[]
  /** 该日报的当日重点（2–3 条；右轨「当日日报」点评层；缺 highlights 的旧响应 = 未取到）。
   *  可选：旧读取链的模型不带该字段，消费方按空数组处理（不造重点）。 */
  highlights?: LaneDayReportHighlight[]
}

export interface LaneDayReportDayDeps {
  /** 板块 ID（晚到响应与请求时的板块比对）。 */
  boardId: () => number
  /** 同一份报日索引（父 store 注入，不新建第二份）。 */
  loadIndex: (days: number, force?: boolean) => Promise<IndexLoadResult>
  /** 单日所需索引覆盖天数（索引模块计算）。 */
  requiredIndexDays: (date: string) => number
  /** 同一份详情缓存（与编年共用键/容量/在途合乘）。 */
  detailCache: BoundedRequestCache<string, LaneDayReportDetail>
}

export interface LaneDayReportDayReader {
  /** 报告优先读取的当日模型（不依赖选中 lane；display gate = date 比对）。 */
  reportDay: ShallowRef<LaneReportDay | null>
  /** 同日报日刷新失败：保留旧内容 + stale 标注。 */
  reportStale: Ref<boolean>
  /** 报告优先读取入口（只需日期）。force = 绕过详情成功缓存（报告重建/刷新路径）。 */
  ensureReportDay: (date: string, options?: { force?: boolean }) => Promise<void>
  /** 切板块/卸载：清显示与在途序号。 */
  reset: () => void
}

export function createLaneDayReportDay(deps: LaneDayReportDayDeps): LaneDayReportDayReader {
  const reportDay = shallowRef<LaneReportDay | null>(null)
  const reportStale = ref(false)
  /** 报告优先读取 epoch：与 laneSeq 独立，互不取消。 */
  let reportSeq = 0

  /**
   * 报告优先读取入口（只需日期）：索引 → 精确 period_date → 详情（同一 reportCache）
   * → 发布 `reportDay`。0 个 section 为合法 ready；无报日/索引失败/详情失败分别分层，不互相冒充。
   * force = 绕过详情成功缓存（报告重建/刷新路径）。
   */
  async function ensureReportDay(date: string, options: { force?: boolean } = {}): Promise<void> {
    const requestSeq = ++reportSeq
    const boardId = deps.boardId()
    const previous = reportDay.value
    const sameDate = previous !== null && previous.date === date
    // 跨日/首读先清显示：不把旧日 sections 挂在新日期标题下（同日刷新保留旧内容至终态）
    reportDay.value = sameDate
      ? { ...previous, status: 'loading', error: null, errorSource: null }
      : { date, status: 'loading', error: null, errorSource: null, reportId: null, sections: [] }
    if (options.force || !sameDate) reportStale.value = false

    const indexResult = await deps.loadIndex(deps.requiredIndexDays(date))
    if (requestSeq !== reportSeq) return
    if (deps.boardId() !== boardId || indexResult.stale) return
    const base: LaneReportDay = {
      date,
      status: 'loading',
      error: null,
      errorSource: null,
      reportId: null,
      sections: [],
    }
    const items = indexResult.items
    if (items === null) {
      finish(requestSeq, { ...base, status: 'error', error: indexResult.error || '日报列表加载失败', errorSource: 'index' }, previous)
      return
    }
    const matched = items.find(item => item.periodDate === date) ?? null
    if (!matched) {
      if (indexResult.error) {
        // 索引陈旧：不得把「索引里没看到」当作「当日无日报」
        finish(requestSeq, { ...base, status: 'error', error: indexResult.error, errorSource: 'index' }, previous)
        return
      }
      finish(requestSeq, { ...base, status: 'no-report' }, previous)
      return
    }
    const detailRes: BoundedCacheLoadResult<LaneDayReportDetail> = await deps.detailCache.load(matched.reportId, { force: options.force })
    if (requestSeq !== reportSeq) return
    if (!detailRes.ok || !detailRes.data) {
      finish(requestSeq, {
        ...base,
        status: 'error',
        error: detailRes.error || '日报详情加载失败',
        errorSource: 'detail',
        reportId: matched.reportId,
      }, previous)
      return
    }
    finish(requestSeq, {
      date,
      status: 'ready',
      error: null,
      errorSource: null,
      reportId: matched.reportId,
      sections: detailRes.data.sections,
      highlights: detailRes.data.highlights,
    }, previous)
  }

  function finish(requestSeq: number, next: LaneReportDay, previous: LaneReportDay | null): void {
    if (requestSeq !== reportSeq) return
    if (next.status === 'error' && previous !== null && previous.date === next.date && previous.status === 'ready') {
      // 同日报日刷新失败：保留旧内容可读 + stale（不整体替换为空）
      reportDay.value = { ...previous, error: next.error, errorSource: next.errorSource }
      reportStale.value = true
      return
    }
    reportStale.value = false
    reportDay.value = next
  }

  function reset(): void {
    reportSeq += 1
    reportDay.value = null
    reportStale.value = false
  }

  return { reportDay, reportStale, ensureReportDay, reset }
}
