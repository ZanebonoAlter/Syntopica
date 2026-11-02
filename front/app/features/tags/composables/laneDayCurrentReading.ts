/**
 * laneDayCurrentReading — 编年「当前日」读取状态（G4 当日编年 + 8C 跨上下文隔离）。
 *
 * 从 `useLaneDayReading` 按内聚职责抽出（code-style §8），由父 store 创建一次并委托；
 * 与报告优先读取共用同一份报日索引与详情缓存（不另建读取器/缓存）。合同逐条继承：
 *  - **受控读取**：laneKey/date 由组件从 props 派生，sectionKey 由当日 sections 派生后内部持有；
 *    laneKey/date/sectionKey 任一变化即跨上下文，先清显示数据再加载（不把旧日摘要挂在新日期标题下）；
 *    仅同上下文刷新失败保留旧数据标 stale。
 *  - **独立 epoch**（`laneSeq`）：切板块/离开上下文/卸载后晚到响应丢弃，不取消报告优先读取。
 *  - 分层：索引失败/无报日/详情失败都降级为 `error`/`no-report` 模型（`buildLaneDayFallbackModel`），
 *    分区缺失按桶降级 base 标题；不伪造摘要、不借别日、不触发生成/回填。
 *  - `suspendDisplay` 供组件离开可读上下文时失效在途显示（R24），不动成功缓存。
 *
 * 不拥有：索引状态（`laneDayReportIndex`）、详情/文章缓存本体（父 store 注入）、选中日与 UI。
 */
import { ref, shallowRef, type Ref, type ShallowRef } from 'vue'
import type { WindowLaneDay } from '~/api/normalizers/laneDynamicsWindow'
import {
  buildLaneDayFallbackModel,
  buildLaneDayJoinedModel,
  type LaneDayReadingModel,
  type LaneDayReportDetail,
} from '~/api/normalizers/laneDayReading'
import type { BoundedRequestCache } from './laneDayReadingCache'
import type { IndexLoadResult } from './laneDayReportIndex'

/** 读取上下文（board 由父 store 持有；laneKey/date/sectionKey 任一变化即跨上下文）。
 *  sectionKey 从当日 sections 派生（逗号串），调用方无法错配；同日换泳道/分区集变化都隔离显示。 */
export interface LaneDayReadingContext {
  laneKey: string
  date: string
  sectionKey: string
}

export type LaneDayCurrentReading = LaneDayReadingModel & LaneDayReadingContext

export interface LaneDayCurrentReadingDeps {
  /** 板块 ID（晚到响应与请求时的板块比对）。 */
  boardId: () => number
  /** 同一份报日索引（父 store 注入，不新建第二份）。 */
  loadIndex: (days: number, force?: boolean) => Promise<IndexLoadResult>
  /** 单日所需索引覆盖天数（索引模块计算）。 */
  requiredIndexDays: (date: string) => number
  /** 同一份详情缓存（与报告优先读取共用键/容量/在途合乘）。 */
  detailCache: BoundedRequestCache<string, LaneDayReportDetail>
}

export interface LaneDayCurrentReadingReader {
  /** 当前日读取态（跨上下文先置 null——不把旧日数据挂在新日期标题下）。 */
  current: ShallowRef<LaneDayCurrentReading | null>
  loading: Ref<boolean>
  error: Ref<string | null>
  /** 同上下文刷新失败：保留旧数据 + stale 标注（跨上下文失败不冒旧内容）。 */
  staleNotice: Ref<boolean>
  /**
   * 当前日读取入口（受控：laneKey/date 由组件从 props 派生，sectionKey 从 day 内部派生）。
   * force = 强制重读（索引 + 详情都绕过成功缓存；「报告已更新/分区不匹配」刷新路径）。
   */
  ensureDayReading: (input: { laneKey: string, date: string }, day: WindowLaneDay, options?: { force?: boolean }) => Promise<void>
  /** 离开可读上下文：失效在途显示（不动成功缓存）。 */
  suspendDisplay: () => void
  /** 切板块/卸载：清显示与上下文（在途响应按序号丢弃）。 */
  reset: () => void
}

export function createLaneDayCurrentReading(deps: LaneDayCurrentReadingDeps): LaneDayCurrentReadingReader {
  const current = shallowRef<LaneDayCurrentReading | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const staleNotice = ref(false)

  /** lane 读取 epoch：与报告优先读取独立，互不取消；切板块/卸载递增。 */
  let laneSeq = 0
  let currentContext: LaneDayReadingContext | null = null

  async function ensureDayReading(input: { laneKey: string, date: string }, day: WindowLaneDay, options: { force?: boolean } = {}): Promise<void> {
    const requestSeq = ++laneSeq
    const boardId = deps.boardId()
    const context: LaneDayReadingContext = {
      laneKey: input.laneKey,
      date: input.date,
      sectionKey: day.sections.map(section => section.sectionId).join(','),
    }
    const contextChanged = currentContext === null
      || currentContext.laneKey !== context.laneKey
      || currentContext.date !== context.date
      || currentContext.sectionKey !== context.sectionKey
    const previous = currentContext !== null && !contextChanged ? current.value : null
    currentContext = context
    if (contextChanged) {
      // 跨上下文先清显示数据（骨架），不把旧日摘要挂在新日期标题下
      current.value = null
      staleNotice.value = false
      error.value = null
    } else if (options.force) {
      staleNotice.value = false
    }
    loading.value = true
    error.value = null

    // 1) 索引：已缓存覆盖所需范围则复用（不反复取同范围）；否则按差额扩大后整体替换
    const indexResult = await deps.loadIndex(deps.requiredIndexDays(context.date), Boolean(options.force))
    if (requestSeq !== laneSeq) return
    if (deps.boardId() !== boardId) return
    if (indexResult.stale) return
    const items = indexResult.items
    if (items === null) {
      finish(requestSeq, context, day, buildLaneDayFallbackModel(day, { status: 'error', error: indexResult.error || '日报列表加载失败' }), previous)
      return
    }

    // 2) 精确 period_date 命中（不做日期邻近推断）
    const matched = items.find(item => item.periodDate === context.date) ?? null
    if (!matched) {
      // 索引陈旧（刷新失败）时不能断言「无日报」
      const model = indexResult.error
        ? buildLaneDayFallbackModel(day, { status: 'error', error: indexResult.error })
        : buildLaneDayFallbackModel(day, { status: 'no-report' })
      finish(requestSeq, context, day, model, previous)
      return
    }

    // 3) 详情（有界缓存 + 在途去重）→ 稳定 sectionId join
    const detailRes = await deps.detailCache.load(matched.reportId, { force: options.force })
    if (requestSeq !== laneSeq) return
    if (!detailRes.ok || !detailRes.data) {
      finish(requestSeq, context, day, buildLaneDayFallbackModel(day, {
        status: 'error',
        error: detailRes.error || '日报详情加载失败',
        reportId: matched.reportId,
      }), previous)
      return
    }
    finish(requestSeq, context, day, buildLaneDayJoinedModel(day, { reportId: matched.reportId, detail: detailRes.data }), previous)
  }

  function finish(
    requestSeq: number,
    context: LaneDayReadingContext,
    day: WindowLaneDay,
    model: LaneDayReadingModel,
    previous: LaneDayCurrentReading | null,
  ): void {
    if (requestSeq !== laneSeq) return
    loading.value = false
    const hasJoinedContent = previous !== null && previous.sections.some(section => section.matched)
    if (model.status === 'error' && hasJoinedContent) {
      // 同上下文刷新失败：保留旧数据可读 + stale（不整体替换为空降级）
      staleNotice.value = true
      error.value = model.error
      return
    }
    error.value = model.status === 'error' ? model.error : null
    staleNotice.value = false
    current.value = { ...model, laneKey: context.laneKey, date: context.date, sectionKey: context.sectionKey }
  }

  /** 离开可读上下文（gap/unavailable/未选日/换无分区泳道）：失效在途显示（R24），不动成功缓存。 */
  function suspendDisplay(): void {
    laneSeq += 1
    loading.value = false
    current.value = null
    staleNotice.value = false
    error.value = null
  }

  function reset(): void {
    laneSeq += 1
    currentContext = null
    current.value = null
    loading.value = false
    error.value = null
    staleNotice.value = false
  }

  return { current, loading, error, staleNotice, ensureDayReading, suspendDisplay, reset }
}
