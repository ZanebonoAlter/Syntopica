/**
 * useConsoleSelection 测试（8C G1；RDY 组的壳层投影）。
 *
 * 只验证「纯模型校正结果如何进入唯一 selectedDate」与「reportDates 接线是否可选」：
 *  - 未接线（旧调用）：保持既有 CH-1 口径，不因新增依赖改变默认选日；
 *  - 接线：索引未就绪时不清、到达后按 RDY-1/2 校正；切泳道保留合法报日；范围排出回窗内最新报日。
 */
import { describe, expect, it } from 'vitest'
import { computed, effectScope, nextTick, ref, type Ref } from 'vue'
import type { LaneDynamicsResponse } from '~/api/laneDynamics'
import {
  normalizeLaneDynamicsWindow,
  type LaneDynamicsWindowData,
  type WindowLaneDay,
} from '~/api/normalizers/laneDynamicsWindow'
import { buildDayAxis } from '~/features/tags/composables/laneMatrixModel'
import { useConsoleSelection } from './useConsoleSelection'
import type { LaneDayResolver } from './observationLanes'

const ANCHOR = '2026-10-24'
const AXIS = buildDayAxis(ANCHOR, 14)

function sectionRaw(id: number): Record<string, unknown> {
  return { section_id: id, label: `桶${id}`, events: [`事件${id}`] }
}

function laneRaw(topicId: number, label: string, timeline: { date: string, sections: Record<string, unknown>[] }[]): Record<string, unknown> {
  return { topic_id: topicId, label, watch_linked: false, section_count_14d: 0, snapshot: null, timeline }
}

function normalize(lanes: Record<string, unknown>[]): LaneDynamicsWindowData {
  return normalizeLaneDynamicsWindow({
    window_days: 14,
    has_reports: true,
    lanes,
    candidates: [],
  } as unknown as LaneDynamicsResponse)
}

function resolverFor(source: LaneDynamicsWindowData): LaneDayResolver {
  const index = new Map(source.lanes.map(lane => [lane.topicId, new Map(lane.timeline.map(day => [day.date, day]))]))
  return (topicId, date) => {
    const day = index.get(topicId)?.get(date)
    return day ? { kind: 'content', source: 'tier', day } : { kind: 'gap' }
  }
}

const REPORT_DATES = new Set(['2026-10-22', '2026-10-23', '2026-10-24'])

function harness(options: {
  source?: LaneDynamicsWindowData | null
  selectedLaneId?: number | null
  reportDates?: Ref<ReadonlySet<string> | null>
  verifiedDay?: Ref<string | null>
  /** 365 池边界（非 null = 曲线轴存在）；配合 poolDays 模拟池已就绪。 */
  poolBounds?: { start: string, end: string } | null
  /** 池已就绪时的历史日：topicId → date → 该日桶。 */
  poolDays?: Record<string, Record<string, WindowLaneDay>>
  /** true = 池未就绪（档窗外一律 unavailable，模拟 365 池未取到）。 */
  poolUnavailable?: boolean
} = {}) {
  const initial = options.source === undefined ? normalize([]) : options.source
  const sourceRef = ref<LaneDynamicsWindowData | null>(initial)
  const tierResolver = resolverFor(initial ?? normalize([]))
  const poolDays = options.poolDays ?? {}
  /** 池已就绪时：池内历史日 → content/pool；其余走档（A）口径。 */
  const poolAwareResolver: LaneDayResolver = (topicId, date) => {
    const pooled = poolDays[topicId]?.[date]
    if (pooled) return { kind: 'content', source: 'pool', day: pooled }
    if (options.poolUnavailable) {
      const inTier = (initial ?? normalize([])).lanes
        .some(lane => lane.topicId === topicId && lane.timeline.some(day => day.date === date))
      if (!inTier) return { kind: 'unavailable' }
    }
    return tierResolver(topicId, date)
  }
  let resolver = poolAwareResolver
  const selectedLaneId = ref<number | null>(options.selectedLaneId ?? null)
  const visibleAxis = ref<string[]>([...AXIS])
  const scope = effectScope()
  const selection = scope.run(() => useConsoleSelection({
    sourceWindow: sourceRef,
    axis: computed(() => AXIS),
    visibleAxis: computed(() => visibleAxis.value),
    selectedLaneId,
    resolveLaneDay: (topicId, date) => resolver(topicId, date),
    poolBounds: computed(() => options.poolBounds ?? null),
    ...(options.reportDates === undefined ? {} : { reportDates: options.reportDates }),
    ...(options.verifiedDay === undefined ? {} : { verifiedDay: options.verifiedDay }),
  }))!
  /** 模拟 A 通道到达（壳层同 tick 更新 sourceWindow + 自动选中泳道）。 */
  function deliverSource(next: LaneDynamicsWindowData, laneId: number | null = null): void {
    resolver = resolverFor(next)
    sourceRef.value = next
    selectedLaneId.value = laneId
  }
  return { selection, selectedLaneId, visibleAxis, sourceRef, scope, deliverSource }
}

describe('useConsoleSelection — 默认选日（未接线 reportDates 的旧口径）', () => {
  it('无泳道且无报日依赖：保持旧行为（null，不凭空造日期）', async () => {
    const h = harness()
    h.selection.selectedDate.value = null
    await nextTick()
    expect(h.selection.selectedDate.value).toBeNull()
    h.scope.stop()
  })

  it('有泳道且无报日依赖：默认最后有分区日（既有 CH-1）', async () => {
    const source = normalize([
      laneRaw(1204, '主线', [
        { date: '2026-10-20', sections: [sectionRaw(1)] },
        { date: '2026-10-22', sections: [sectionRaw(2)] },
      ]),
    ])
    const h = harness({ source })
    h.selectedLaneId.value = 1204
    await nextTick()
    expect(h.selection.selectedDate.value).toBe('2026-10-22')
    h.scope.stop()
  })
})

describe('useConsoleSelection — 报日依赖到达后再校正（RDY）', () => {
  it('RDY-7→RDY-1：索引未就绪暂 null，到达后取窗内最新报日', async () => {
    const reportDates = ref<ReadonlySet<string> | null>(null)
    const h = harness({ reportDates })
    await nextTick()
    expect(h.selection.selectedDate.value).toBeNull()
    reportDates.value = REPORT_DATES
    await nextTick()
    expect(h.selection.selectedDate.value).toBe('2026-10-24')
    h.scope.stop()
  })

  it('A 名单未到达：不先把「0 泳道」当事实选中报日；名单到达后按有泳道校正', async () => {
    const source = normalize([
      laneRaw(1204, '主线', [{ date: '2026-10-20', sections: [sectionRaw(1)] }]),
    ])
    const h = harness({ source: null, reportDates: ref<ReadonlySet<string> | null>(REPORT_DATES) })
    await nextTick()
    expect(h.selection.selectedDate.value).toBeNull()
    // A 到达：壳层同 tick 自动选中最前泳道
    h.deliverSource(source, 1204)
    await nextTick()
    expect(h.selection.selectedDate.value).toBe('2026-10-20')
    h.scope.stop()
  })

  it('RDY-2：有泳道时默认仍是最后内容日，不被报日抢走', async () => {
    const source = normalize([
      laneRaw(1204, '主线', [{ date: '2026-10-20', sections: [sectionRaw(1)] }]),
    ])
    const h = harness({ source, selectedLaneId: 1204, reportDates: ref<ReadonlySet<string> | null>(REPORT_DATES) })
    await nextTick()
    expect(h.selection.selectedDate.value).toBe('2026-10-20')
    h.scope.stop()
  })

  it('RDY-10：切泳道保留当前报日（不搬回内容日）', async () => {
    const source = normalize([
      laneRaw(1204, '主线', [{ date: '2026-10-20', sections: [sectionRaw(1)] }]),
      laneRaw(1205, '辅线', [{ date: '2026-10-21', sections: [sectionRaw(2)] }]),
    ])
    const h = harness({ source, selectedLaneId: 1204, reportDates: ref<ReadonlySet<string> | null>(REPORT_DATES) })
    await nextTick()
    expect(h.selection.selectedDate.value).toBe('2026-10-20')
    h.selection.selectedDate.value = '2026-10-24'
    await nextTick()
    h.selectedLaneId.value = 1205
    await nextTick()
    expect(h.selection.selectedDate.value).toBe('2026-10-24')
    h.scope.stop()
  })

  it('RDY-5/6：范围排出回新窗最新报日；窗内无报日为 null', async () => {
    const reportDates = ref<ReadonlySet<string> | null>(REPORT_DATES)
    const h = harness({ reportDates })
    await nextTick()
    expect(h.selection.selectedDate.value).toBe('2026-10-24')
    // 拖到不含 10/24 的窗：回新窗内最新报日 10/23
    h.visibleAxis.value = ['2026-10-15', '2026-10-16', '2026-10-22', '2026-10-23']
    await nextTick()
    expect(h.selection.selectedDate.value).toBe('2026-10-23')
    // 再拖到窗内无报日：null（不跳 today、不跳 active 日）
    reportDates.value = new Set(['2026-10-01'])
    await nextTick()
    expect(h.selection.selectedDate.value).toBeNull()
    h.scope.stop()
  })

  it('索引失败（null）：不把已选报日搬走', async () => {
    const reportDates = ref<ReadonlySet<string> | null>(REPORT_DATES)
    const h = harness({ reportDates })
    await nextTick()
    expect(h.selection.selectedDate.value).toBe('2026-10-24')
    reportDates.value = null
    await nextTick()
    expect(h.selection.selectedDate.value).toBe('2026-10-24')
    h.scope.stop()
  })
})

describe('useConsoleSelection — 已验证记录日保护（R41）', () => {
  it('索引缺该日但它是已验证记录日：不被普通回落搬走；保护撤销后回落', async () => {
    const reportDates = ref<ReadonlySet<string> | null>(REPORT_DATES)
    const verifiedDay = ref<string | null>(null)
    const h = harness({ reportDates, verifiedDay })
    await nextTick()
    expect(h.selection.selectedDate.value).toBe('2026-10-24')

    // 无保护：索引缺 10/21 → 按普通口径回最新报日 10/24
    h.selection.selectedDate.value = '2026-10-22'
    await nextTick()
    expect(h.selection.selectedDate.value).toBe('2026-10-22')
    h.selection.selectedDate.value = '2026-10-21'
    await nextTick()
    expect(h.selection.selectedDate.value).toBe('2026-10-24')

    // 有保护（当前观察目标日）：同一动作不再搬走
    verifiedDay.value = '2026-10-21'
    h.selection.selectedDate.value = '2026-10-22'
    await nextTick()
    h.selection.selectedDate.value = '2026-10-21'
    await nextTick()
    expect(h.selection.selectedDate.value).toBe('2026-10-21')

    // 保护撤销（清场/换选）：回到普通回落
    verifiedDay.value = null
    h.selection.selectedDate.value = '2026-10-22'
    await nextTick()
    h.selection.selectedDate.value = '2026-10-21'
    await nextTick()
    expect(h.selection.selectedDate.value).toBe('2026-10-24')
    h.scope.stop()
  })
})

describe('useConsoleSelection — 池曲线 series（顶部时间视窗背景曲线）', () => {
  /** 池内历史日的一个桶（线索数 6）。 */
  function poolDay(date: string): WindowLaneDay {
    return {
      date,
      sections: [{
        sectionId: '1001',
        label: '历史桶',
        threadCount: 6,
        fallbackClueCount: 6,
        foldedCount: 0,
        eventTitles: ['历史事件'],
      }],
    }
  }

  it('池已就绪：档窗外历史日在 series 里有值（曲线不再只剩当前档那几天）', () => {
    const source = normalize([
      laneRaw(10, '主线 A', [{ date: '2026-10-23', sections: [sectionRaw(1)] }]),
    ])
    const h = harness({
      source,
      poolBounds: { start: '2025-10-24', end: '2026-10-23' },
      poolDays: { '10': { '2026-06-15': poolDay('2026-06-15') } },
    })
    const series = h.selection.poolSeries.value
    expect(series).not.toBeNull()
    // 档窗外历史日：曲线有真实值（此前池未取到时这里是 unknown/null → 曲线只剩档窗那几天）
    expect(series?.find(point => point.date === '2026-06-15')?.value).toBe(6)
    // 档窗内当日：仍按 A 口径
    expect(series?.find(point => point.date === '2026-10-23')?.value).toBe(1)
    // 池已取到的断档日 = 已知 0（不是「未取到」的 null）
    expect(series?.find(point => point.date === '2026-01-01')?.value).toBe(0)
    h.scope.stop()
  })

  it('池未就绪：档窗外历史日是 unknown（null 不补 0），不伪造曲线', () => {
    const source = normalize([
      laneRaw(10, '主线 A', [{ date: '2026-10-23', sections: [sectionRaw(1)] }]),
    ])
    const h = harness({
      source,
      poolBounds: { start: '2025-10-24', end: '2026-10-23' },
      poolUnavailable: true,
    })
    const series = h.selection.poolSeries.value
    expect(series?.find(point => point.date === '2026-06-15')?.value).toBeNull()
    h.scope.stop()
  })
})
